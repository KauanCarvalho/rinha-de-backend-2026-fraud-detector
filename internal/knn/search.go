package knn

import "math"

// BruteForceSearch scans every vector and returns the k nearest neighbors.
// It exists to give IVFIndex.Search a correctness oracle in tests (a full
// nprobe search over an IVF index is mathematically equivalent to this,
// since every vector belongs to exactly one cluster) and is not meant for
// production use over the full 3M-vector reference dataset.
func BruteForceSearch(vectors []QVector, labels []Label, query QVector, k int) []Neighbor {
	r := newResult(k)
	for i, v := range vectors {
		r.tryInsert(sqDist(v, query), labels[i])
	}
	return r.neighbors
}

// result keeps the k best (smallest-distance) neighbors seen so far, sorted
// ascending. k is expected to be tiny (5, in this challenge), so a plain
// insertion-sorted slice beats the bookkeeping of a heap.
type result struct {
	neighbors []Neighbor
}

func newResult(k int) *result {
	ns := make([]Neighbor, k)
	for i := range ns {
		ns[i].Dist = math.MaxInt64
	}
	return &result{neighbors: ns}
}

func (r *result) worst() int64 { return r.neighbors[len(r.neighbors)-1].Dist }

func (r *result) tryInsert(dist int64, label Label) {
	if dist >= r.worst() {
		return
	}
	i := len(r.neighbors) - 1
	for i > 0 && r.neighbors[i-1].Dist > dist {
		r.neighbors[i] = r.neighbors[i-1]
		i--
	}
	r.neighbors[i] = Neighbor{Dist: dist, Label: label}
}
