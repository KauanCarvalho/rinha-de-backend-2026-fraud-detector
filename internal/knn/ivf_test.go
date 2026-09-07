package knn_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/knn"
)

func TestBuildIVF_LenMatchesInput(t *testing.T) {
	t.Parallel()

	vectors, labels := randomDataset(1000, 30)
	idx := knn.BuildIVF(vectors, labels)

	assert.Equal(t, len(vectors), idx.Len())
}

func TestIVFIndex_Search_FullProbeMatchesBruteForce(t *testing.T) {
	t.Parallel()

	vectors, labels := randomDataset(2000, 31)
	idx := knn.BuildIVF(vectors, labels)

	for q := range 5 {
		query := randomQuery(int64(9000 + q))

		want := knn.BruteForceSearch(vectors, labels, query, 5)
		// Probing every cluster is equivalent to brute force: every vector
		// belongs to exactly one cluster, so nothing is skipped.
		got := idx.Search(query, 5, len(idx.Centroids))
		assert.Equal(t, want, got)
	}
}

func TestIVFIndex_Search_FewerProbesNeverBeatsFullProbe(t *testing.T) {
	t.Parallel()

	vectors, labels := randomDataset(3000, 32)
	idx := knn.BuildIVF(vectors, labels)

	for q := range 5 {
		query := randomQuery(int64(9500 + q))

		full := idx.Search(query, 5, len(idx.Centroids))
		partial := idx.Search(query, 5, 2)

		assert.LessOrEqual(t, full[len(full)-1].Dist, partial[len(partial)-1].Dist)
	}
}

func TestIVFIndex_Search_ZeroProbesReturnsEmptyResult(t *testing.T) {
	t.Parallel()

	vectors, labels := randomDataset(500, 33)
	idx := knn.BuildIVF(vectors, labels)

	got := idx.Search(randomQuery(9999), 5, 0)
	for _, n := range got {
		assert.Equal(t, knn.Label(0), n.Label)
	}
}

func TestBuildIVF_TinyDataset(t *testing.T) {
	t.Parallel()

	vectors := []knn.QVector{randomQuery(1), randomQuery(2), randomQuery(3)}
	labels := []knn.Label{knn.LabelFraud, knn.LabelLegit, knn.LabelFraud}

	idx := knn.BuildIVF(vectors, labels)

	assert.Equal(t, 3, idx.Len())
	got := idx.Search(vectors[0], 3, len(idx.Centroids))
	assert.Equal(t, knn.BruteForceSearch(vectors, labels, vectors[0], 3), got)
}

func TestIVFIndex_SaveFileLoadFile_RoundTrip(t *testing.T) {
	t.Parallel()

	vectors, labels := randomDataset(1200, 35)
	idx := knn.BuildIVF(vectors, labels)

	path := t.TempDir() + "/ivf.bin"
	require.NoError(t, idx.SaveFile(path))

	loaded, err := knn.LoadIVFFile(path)
	require.NoError(t, err)
	require.Equal(t, idx.Len(), loaded.Len())

	query := randomQuery(10500)
	assert.Equal(t,
		idx.Search(query, 5, len(idx.Centroids)),
		loaded.Search(query, 5, len(loaded.Centroids)),
	)
}

func TestLoadIVFFile_MissingFile(t *testing.T) {
	t.Parallel()

	_, err := knn.LoadIVFFile(t.TempDir() + "/does-not-exist.bin")

	assert.Error(t, err)
}

func TestIVFIndex_SaveLoad_RoundTrip(t *testing.T) {
	t.Parallel()

	vectors, labels := randomDataset(1500, 34)
	idx := knn.BuildIVF(vectors, labels)

	var buf bytes.Buffer
	require.NoError(t, idx.Save(&buf))

	loaded, err := knn.LoadIVF(&buf)
	require.NoError(t, err)
	require.Equal(t, idx.Len(), loaded.Len())

	for q := range 5 {
		query := randomQuery(int64(10000 + q))
		want := idx.Search(query, 5, len(idx.Centroids))
		got := loaded.Search(query, 5, len(loaded.Centroids))
		assert.Equal(t, want, got)
	}
}
