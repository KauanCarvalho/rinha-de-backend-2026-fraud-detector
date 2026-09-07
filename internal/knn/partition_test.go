package knn_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/knn"
	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/vectorize"
)

// taggedVector builds a QVector with the given tag bits set on the real
// vectorize dimensions Tag reads, and zero everywhere else. hasLastTx=false
// sets the sentinel (matching a null last_transaction), not a literal -1
// value, mirroring what Quantize would actually produce.
func taggedVector(hasLastTx, isOnline, cardPresent, unknownMerchant bool) knn.QVector {
	var q knn.QVector
	if !hasLastTx {
		q[vectorize.DimMinutesSinceLastTx] = -knn.Scale
	}
	if isOnline {
		q[vectorize.DimIsOnline] = knn.Scale
	}
	if cardPresent {
		q[vectorize.DimCardPresent] = knn.Scale
	}
	if unknownMerchant {
		q[vectorize.DimUnknownMerchant] = knn.Scale
	}
	return q
}

func TestTag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                                           string
		hasLastTx, isOnline, cardPresent, unknownMerch bool
		want                                           uint8
	}{
		{"all false", true, false, false, false, 0},
		{"no last tx only", false, false, false, false, 1},
		{"online only", true, true, false, false, 2},
		{"card present only", true, false, true, false, 4},
		{"unknown merchant only", true, false, false, true, 8},
		{"all true except has-last-tx", false, true, true, true, 15},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			q := taggedVector(tt.hasLastTx, tt.isOnline, tt.cardPresent, tt.unknownMerch)
			assert.Equal(t, tt.want, knn.Tag(q))
		})
	}
}

func TestBuildPartitioned_RoutesVectorsToTheirOwnTag(t *testing.T) {
	t.Parallel()

	tagA := taggedVector(true, false, false, false) // tag 0
	tagB := taggedVector(true, true, false, false)  // tag 2
	vectors := []knn.QVector{tagA, tagA, tagA, tagB, tagB}
	labels := []knn.Label{knn.LabelLegit, knn.LabelLegit, knn.LabelFraud, knn.LabelFraud, knn.LabelFraud}

	pi := knn.BuildPartitioned(vectors, labels, knn.DefaultLeafSize)

	assert.Equal(t, 3, pi.Partitions[0].Len())
	assert.Equal(t, 2, pi.Partitions[2].Len())
	assert.Equal(t, 5, pi.Len())

	for tag := range knn.NumPartitions {
		if tag == 0 || tag == 2 {
			continue
		}
		assert.Zerof(t, pi.Partitions[tag].Len(), "tag %d should have no partition", tag)
	}
}

func TestPartitionedIndex_Search_MatchesBruteForceWithinPartition(t *testing.T) {
	t.Parallel()

	base := taggedVector(true, false, true, false) // a fixed tag shared by every vector below
	vectors, labels := randomDataset(300, 11)
	for i := range vectors {
		// Overwrite the tag dimensions only, keeping the rest random, so
		// every vector shares base's tag while distances still vary.
		vectors[i][vectorize.DimMinutesSinceLastTx] = base[vectorize.DimMinutesSinceLastTx]
		vectors[i][vectorize.DimIsOnline] = base[vectorize.DimIsOnline]
		vectors[i][vectorize.DimCardPresent] = base[vectorize.DimCardPresent]
		vectors[i][vectorize.DimUnknownMerchant] = base[vectorize.DimUnknownMerchant]
	}

	pi := knn.BuildPartitioned(vectors, labels, knn.DefaultLeafSize)

	for q := range 5 {
		query := randomQuery(int64(7000 + q))
		query[vectorize.DimMinutesSinceLastTx] = base[vectorize.DimMinutesSinceLastTx]
		query[vectorize.DimIsOnline] = base[vectorize.DimIsOnline]
		query[vectorize.DimCardPresent] = base[vectorize.DimCardPresent]
		query[vectorize.DimUnknownMerchant] = base[vectorize.DimUnknownMerchant]

		want := knn.BruteForceSearch(vectors, labels, query, 5)
		got := pi.Search(query, 5, 0)
		assert.Equal(t, want, got)
	}
}

func TestPartitionedIndex_Search_FallsBackWhenTargetPartitionIsEmpty(t *testing.T) {
	t.Parallel()

	onlyTag := taggedVector(true, false, false, false) // tag 0
	vectors, labels := randomDataset(200, 13)
	for i := range vectors {
		vectors[i][vectorize.DimMinutesSinceLastTx] = onlyTag[vectorize.DimMinutesSinceLastTx]
		vectors[i][vectorize.DimIsOnline] = onlyTag[vectorize.DimIsOnline]
		vectors[i][vectorize.DimCardPresent] = onlyTag[vectorize.DimCardPresent]
		vectors[i][vectorize.DimUnknownMerchant] = onlyTag[vectorize.DimUnknownMerchant]
	}

	pi := knn.BuildPartitioned(vectors, labels, knn.DefaultLeafSize)
	require.Positive(t, pi.Partitions[0].Len())

	// Query tagged 15 (every bit set) -- guaranteed empty, since every
	// reference vector above was forced to tag 0.
	query := randomQuery(99)
	other := taggedVector(false, true, true, true) // tag 15
	query[vectorize.DimMinutesSinceLastTx] = other[vectorize.DimMinutesSinceLastTx]
	query[vectorize.DimIsOnline] = other[vectorize.DimIsOnline]
	query[vectorize.DimCardPresent] = other[vectorize.DimCardPresent]
	query[vectorize.DimUnknownMerchant] = other[vectorize.DimUnknownMerchant]
	require.Equal(t, uint8(15), knn.Tag(query))
	require.Zero(t, pi.Partitions[15].Len())

	want := knn.BruteForceSearch(vectors, labels, query, 5)
	got := pi.Search(query, 5, 0)
	assert.Equal(t, want, got)
}

func TestPartitionedIndex_SaveFileLoadFile_RoundTrip(t *testing.T) {
	t.Parallel()

	vectors, labels := randomDataset(500, 21)
	pi := knn.BuildPartitioned(vectors, labels, knn.DefaultLeafSize)

	path := t.TempDir() + "/partitioned.bin"
	require.NoError(t, pi.SaveFile(path))

	loaded, err := knn.LoadPartitionedFile(path)
	require.NoError(t, err)
	require.Equal(t, pi.Len(), loaded.Len())

	for q := range 5 {
		query := randomQuery(int64(8000 + q))
		assert.Equal(t, pi.Search(query, 5, 0), loaded.Search(query, 5, 0))
	}
}

func TestPartitionedIndex_SaveLoad_RoundTrip(t *testing.T) {
	t.Parallel()

	vectors, labels := randomDataset(300, 23)
	pi := knn.BuildPartitioned(vectors, labels, knn.DefaultLeafSize)

	var buf bytes.Buffer
	require.NoError(t, pi.Save(&buf))

	loaded, err := knn.LoadPartitioned(&buf)
	require.NoError(t, err)
	require.Equal(t, pi.Len(), loaded.Len())
}
