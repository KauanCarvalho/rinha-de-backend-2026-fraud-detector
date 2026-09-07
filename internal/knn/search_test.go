package knn_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/knn"
)

func TestSearch_MatchesBruteForce(t *testing.T) {
	t.Parallel()

	const n = 5000
	const k = 5
	vectors, labels := randomDataset(n, 42)
	idx := knn.Build(vectors, labels, knn.DefaultLeafSize)
	require.Equal(t, n, idx.Len())

	for q := range 25 {
		query := randomQuery(int64(1000 + q))

		got := idx.Search(query, k, 0)
		want := knn.BruteForceSearch(vectors, labels, query, k)

		gotDists := distances(got)
		wantDists := distances(want)
		assert.Equalf(t, wantDists, gotDists, "query %d: k-d tree distances must match brute force", q)
		assert.Equalf(t, knn.FraudCount(want), knn.FraudCount(got), "query %d: fraud count must match brute force", q)
	}
}

func distances(neighbors []knn.Neighbor) []int64 {
	ds := make([]int64, len(neighbors))
	for i, n := range neighbors {
		ds[i] = n.Dist
	}
	return ds
}

func TestSearch_BudgetAlwaysReturnsGreedyLeaf(t *testing.T) {
	t.Parallel()

	vectors, labels := randomDataset(2000, 7)
	idx := knn.Build(vectors, labels, knn.DefaultLeafSize)
	query := randomQuery(999)

	got := idx.Search(query, 5, 1)
	require.Len(t, got, 5)
	for _, n := range got {
		assert.NotEqual(t, int64(math.MaxInt64), n.Dist, "greedy leaf must always populate real neighbors")
	}
}

func TestSearch_TighterBudgetNeverBeatsUnbounded(t *testing.T) {
	t.Parallel()

	vectors, labels := randomDataset(4000, 11)
	idx := knn.Build(vectors, labels, knn.DefaultLeafSize)

	for q := range 10 {
		query := randomQuery(int64(2000 + q))
		exact := idx.Search(query, 5, 0)
		budgeted := idx.Search(query, 5, 2)

		assert.LessOrEqual(t, exact[len(exact)-1].Dist, budgeted[len(budgeted)-1].Dist)
	}
}

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
