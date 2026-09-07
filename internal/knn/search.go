package knn

import "math"

// Search returns the k nearest neighbors to query, sorted by ascending
// distance.
//
// The initial greedy descent to a leaf (cost O(log n)) always happens in
// full, regardless of budget, so Search never returns fewer candidates than
// a single leaf holds. maxExtraLeaves then bounds how many additional
// leaves the branch-and-bound backtracking is allowed to scan before giving
// up and returning the best candidates found so far; pass 0 for unbounded
// (exact) search.
//
// This budget exists because k-d trees lose most of their pruning power as
// dimensionality grows — 14 dimensions is already past the point where
// exact search reliably beats scanning a large fraction of the tree — so
// bounding the backtracking is what keeps p99 latency predictable under
// that curse of dimensionality, at the cost of a small amount of recall.
// This is the standard bounded/"best-effort" k-d tree search technique.
func (idx *Index) Search(query QVector, k int, maxExtraLeaves int) []Neighbor {
	r := newResult(k)
	extraLeaves := 0
	idx.search(idx.Root, query, r, &extraLeaves, maxExtraLeaves, true)
	return r.neighbors
}

func (idx *Index) search(nodeIdx int32, query QVector, r *result, extraLeaves *int, budget int, onGreedyPath bool) {
	if nodeIdx < 0 {
		return
	}

	n := &idx.Nodes[nodeIdx]
	if n.isLeaf() {
		if !onGreedyPath {
			*extraLeaves++
		}
		for i := n.Start; i < n.Start+n.Len; i++ {
			r.tryInsert(sqDist(idx.Vectors[i], query), idx.Labels[i])
		}
		return
	}

	diff := int64(query[n.SplitDim]) - int64(n.SplitValue)
	near, far := n.Left, n.Right
	if diff > 0 {
		near, far = n.Right, n.Left
	}

	idx.search(near, query, r, extraLeaves, budget, onGreedyPath)

	// The far subtree can only contain a closer point than what we already
	// have if the query is nearer to the split plane than our current
	// worst-of-k candidate — otherwise every point over there is provably
	// farther away on this dimension alone.
	planeDist := diff * diff
	if (!r.full() || planeDist < r.worst()) && (budget <= 0 || *extraLeaves < budget) {
		idx.search(far, query, r, extraLeaves, budget, false)
	}
}

// BruteForceSearch scans every vector and returns the k nearest neighbors.
// It exists to give Search a correctness oracle in tests and is not meant
// for production use over the full 3M-vector reference dataset.
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

func (r *result) full() bool { return r.worst() != math.MaxInt64 }

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
