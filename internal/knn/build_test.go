package knn_test

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/knn"
)

func TestBuild_EmptyLeafSizeFallsBackToDefault(t *testing.T) {
	t.Parallel()

	vectors, labels := randomDataset(100, 3)
	idx := knn.Build(vectors, labels, 0)

	assert.Equal(t, 100, idx.Len())
}

func TestBuild_VariousLeafSizes_MatchesBruteForce(t *testing.T) {
	t.Parallel()

	const n = 2000
	const k = 5
	vectors, labels := randomDataset(n, 17)
	query := randomQuery(123)
	want := knn.BruteForceSearch(vectors, labels, query, k)

	for _, leafSize := range []int{1, 2, 5, 32, 500} {
		t.Run("leafSize="+strconv.Itoa(leafSize), func(t *testing.T) {
			t.Parallel()

			idx := knn.Build(vectors, labels, leafSize)
			require.Equal(t, n, idx.Len())

			got := idx.Search(query, k, 0)
			assert.Equal(t, want, got)
		})
	}
}
