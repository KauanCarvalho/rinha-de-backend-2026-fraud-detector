package knn_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/knn"
)

func TestBruteForceSearch_ReturnsKNearestSortedByDistance(t *testing.T) {
	t.Parallel()

	var origin knn.QVector // all zeros

	near := origin
	near[0] = 10

	mid := origin
	mid[0] = 100

	far := origin
	far[0] = 1000

	vectors := []knn.QVector{far, near, mid}
	labels := []knn.Label{knn.LabelLegit, knn.LabelFraud, knn.LabelLegit}

	got := knn.BruteForceSearch(vectors, labels, origin, 2)

	require.Len(t, got, 2)
	assert.Equal(t, int64(100), got[0].Dist) // near: 10^2
	assert.Equal(t, knn.LabelFraud, got[0].Label)
	assert.Equal(t, int64(10000), got[1].Dist) // mid: 100^2
	assert.Equal(t, knn.LabelLegit, got[1].Label)
}
