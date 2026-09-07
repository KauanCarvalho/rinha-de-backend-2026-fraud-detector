package knn

import (
	"math/rand/v2"
	"runtime"
	"sync"
)

// ivfKMeansIters is the number of Lloyd's-algorithm iterations run when
// clustering a partition. Centroids stabilize well before this in
// practice; a fixed small count keeps build time bounded and predictable.
const ivfKMeansIters = 12

// ivfKMeansSeed seeds k-means' random initial centroid choice. Fixed
// (rather than time-based) so building the same reference dataset twice
// always produces the same index.bin.
const ivfKMeansSeed = 42

// ivfMinClusters and ivfMaxClusters bound the number of clusters an IVF
// partition is built with; ivfClustersPerVectors picks a count inside that
// range roughly proportional to the partition's size, so a partition with
// a thousand vectors doesn't get thousands of near-empty clusters, and one
// with a million vectors doesn't get a single giant one.
const (
	ivfMinClusters       = 32
	ivfMaxClusters       = 512
	ivfClustersPerVector = 1000
)

// IVFIndex is an Inverted-File approximate nearest-neighbor index: the
// reference vectors are grouped into K clusters (by k-means), and a search
// only scans the clusters nearest the query instead of every vector — see
// Search. Vectors and Labels are stored grouped by cluster (all of
// cluster 0's vectors, then cluster 1's, ...) so a cluster's members are a
// contiguous slice, addressed by ClusterOffset.
type IVFIndex struct {
	Centroids     []QVector
	ClusterOffset []int32 // length len(Centroids)+1, in Vectors/Labels units
	Vectors       []QVector
	Labels        []Label
}

// Len returns the number of vectors held by the index.
func (idx *IVFIndex) Len() int { return len(idx.Vectors) }

// ivfClusterCount picks a cluster count for n vectors, proportional to n
// but clamped to [ivfMinClusters, ivfMaxClusters], and never more than n
// itself (a cluster needs at least one member to be useful).
func ivfClusterCount(n int) int {
	k := n / ivfClustersPerVector
	k = max(k, ivfMinClusters)
	k = min(k, ivfMaxClusters)
	k = min(k, n)
	return max(k, 1)
}

// BuildIVF clusters vectors into ivfClusterCount(len(vectors)) groups with
// k-means (Lloyd's algorithm, ivfKMeansIters iterations, parallelized
// nearest-centroid assignment) and returns the resulting IVFIndex.
func BuildIVF(vectors []QVector, labels []Label) *IVFIndex {
	n := len(vectors)
	k := ivfClusterCount(n)

	// Not a security-sensitive random use — this is a deterministic,
	// fixed-seed pick of initial k-means centroids, not a secret.
	//nolint:gosec // math/rand/v2 is intentional here; see comment above
	rng := rand.New(rand.NewPCG(ivfKMeansSeed, ivfKMeansSeed))
	centroids := make([]QVector, k)
	for i, p := range rng.Perm(n)[:k] {
		centroids[i] = vectors[p]
	}

	assign := make([]int32, n)
	for range ivfKMeansIters {
		assignToNearestCentroid(vectors, centroids, assign)
		recomputeCentroids(vectors, assign, centroids)
	}
	assignToNearestCentroid(vectors, centroids, assign)

	return buildFromAssignment(vectors, labels, centroids, assign)
}

// assignToNearestCentroid sets assign[i] to the index of vectors[i]'s
// nearest centroid, parallelized across the vectors.
func assignToNearestCentroid(vectors []QVector, centroids []QVector, assign []int32) {
	workers := min(runtime.NumCPU(), len(vectors))
	workers = max(workers, 1)
	chunk := (len(vectors) + workers - 1) / workers

	var wg sync.WaitGroup
	for w := range workers {
		start := w * chunk
		end := min(start+chunk, len(vectors))
		if start >= end {
			continue
		}
		wg.Add(1)
		go func(start, end int) {
			defer wg.Done()
			for i := start; i < end; i++ {
				best, bestDist := 0, sqDist(vectors[i], centroids[0])
				for c := 1; c < len(centroids); c++ {
					if d := sqDist(vectors[i], centroids[c]); d < bestDist {
						best, bestDist = c, d
					}
				}
				assign[i] = int32(best)
			}
		}(start, end)
	}
	wg.Wait()
}

// recomputeCentroids sets each centroid to the mean of the vectors
// currently assigned to it. A centroid with no vectors assigned keeps its
// previous value rather than becoming undefined.
func recomputeCentroids(vectors []QVector, assign []int32, centroids []QVector) {
	sums := make([][Dim]int64, len(centroids))
	counts := make([]int64, len(centroids))
	for i, v := range vectors {
		c := assign[i]
		counts[c]++
		for d := range Dim {
			sums[c][d] += int64(v[d])
		}
	}
	for c := range centroids {
		if counts[c] == 0 {
			continue
		}
		for d := range Dim {
			centroids[c][d] = int16(sums[c][d] / counts[c])
		}
	}
}

// buildFromAssignment groups vectors and labels by their final cluster
// assignment into the contiguous, cluster-ordered layout IVFIndex expects.
func buildFromAssignment(vectors []QVector, labels []Label, centroids []QVector, assign []int32) *IVFIndex {
	k := len(centroids)
	counts := make([]int32, k)
	for _, c := range assign {
		counts[c]++
	}

	offset := make([]int32, k+1)
	for c := range k {
		offset[c+1] = offset[c] + counts[c]
	}

	outVectors := make([]QVector, len(vectors))
	outLabels := make([]Label, len(vectors))
	cursor := append([]int32(nil), offset[:k]...)
	for i, v := range vectors {
		c := assign[i]
		pos := cursor[c]
		outVectors[pos] = v
		outLabels[pos] = labels[i]
		cursor[c]++
	}

	return &IVFIndex{
		Centroids:     centroids,
		ClusterOffset: offset,
		Vectors:       outVectors,
		Labels:        outLabels,
	}
}

// Search probes the nprobe clusters whose centroid is nearest query and
// returns the k nearest vectors found among them, sorted by ascending
// distance. Unlike the k-d tree's bounded backtracking, IVF's cost per
// query is naturally bounded by nprobe × (average cluster size) — there is
// no separate budget parameter.
func (idx *IVFIndex) Search(query QVector, k, nprobe int) []Neighbor {
	type centroidDist struct {
		cluster int
		dist    int64
	}
	dists := make([]centroidDist, len(idx.Centroids))
	for c, centroid := range idx.Centroids {
		dists[c] = centroidDist{c, sqDist(centroid, query)}
	}

	if nprobe > len(dists) {
		nprobe = len(dists)
	}
	// Partial selection sort for the nprobe smallest — nprobe is always
	// small (single/low-double digits) relative to len(dists), so this
	// beats a full sort.
	for i := range nprobe {
		minIdx := i
		for j := i + 1; j < len(dists); j++ {
			if dists[j].dist < dists[minIdx].dist {
				minIdx = j
			}
		}
		dists[i], dists[minIdx] = dists[minIdx], dists[i]
	}

	r := newResult(k)
	for i := range nprobe {
		c := dists[i].cluster
		for j := idx.ClusterOffset[c]; j < idx.ClusterOffset[c+1]; j++ {
			r.tryInsert(sqDist(idx.Vectors[j], query), idx.Labels[j])
		}
	}
	return r.neighbors
}
