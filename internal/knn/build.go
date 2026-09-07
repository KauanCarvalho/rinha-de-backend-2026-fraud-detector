package knn

import "math"

// DefaultLeafSize is the number of points a leaf holds before the tree
// stops splitting further. Larger leaves shrink the tree (faster build,
// less memory for node metadata) at the cost of scanning more points per
// leaf visit.
const DefaultLeafSize = 32

// Build constructs a k-d tree over vectors/labels (same length, paired by
// index). Splits are chosen on the widest dimension of each node's subset,
// using quickselect to find the median in linear time — full sorting at
// every level would cost O(n log^2 n) and become noticeable at 3M points.
//
// Build reorders vectors/labels internally; it does not mutate the slices
// passed in.
func Build(vectors []QVector, labels []Label, leafSize int) *Index {
	if leafSize <= 0 {
		leafSize = DefaultLeafSize
	}

	order := make([]int32, len(vectors))
	for i := range order {
		order[i] = int32(i)
	}

	b := &builder{
		vectors:  vectors,
		labels:   labels,
		leafSize: leafSize,
	}
	root := b.build(order)

	outVectors := make([]QVector, len(b.finalOrder))
	outLabels := make([]Label, len(b.finalOrder))
	for i, origIdx := range b.finalOrder {
		outVectors[i] = vectors[origIdx]
		outLabels[i] = labels[origIdx]
	}

	return &Index{
		Nodes:   b.nodes,
		Vectors: outVectors,
		Labels:  outLabels,
		Root:    root,
	}
}

type builder struct {
	vectors    []QVector
	labels     []Label
	leafSize   int
	nodes      []node
	finalOrder []int32
}

func (b *builder) build(indices []int32) int32 {
	nodeIdx := int32(len(b.nodes))
	b.nodes = append(b.nodes, node{})

	if len(indices) <= b.leafSize {
		start := int32(len(b.finalOrder))
		b.finalOrder = append(b.finalOrder, indices...)
		b.nodes[nodeIdx] = node{Left: -1, Right: -1, Start: start, Len: int32(len(indices))}
		return nodeIdx
	}

	axis := b.widestAxis(indices)
	mid := len(indices) / 2 //nolint:mnd // splitting a slice in half, not a magic number
	selectMedian(indices, mid, func(i, j int32) bool {
		return b.vectors[i][axis] < b.vectors[j][axis]
	})
	splitValue := b.vectors[indices[mid]][axis]

	left := b.build(indices[:mid])
	right := b.build(indices[mid:])

	b.nodes[nodeIdx] = node{Left: left, Right: right, SplitDim: int32(axis), SplitValue: splitValue}
	return nodeIdx
}

// widestAxis returns the dimension with the largest value range across the
// given subset, which tends to produce more balanced, better-pruning
// splits than a fixed round-robin axis.
func (b *builder) widestAxis(indices []int32) int {
	var lo, hi [Dim]int16
	for d := range lo {
		lo[d] = math.MaxInt16
		hi[d] = math.MinInt16
	}
	for _, idx := range indices {
		v := b.vectors[idx]
		for d := range Dim {
			if v[d] < lo[d] {
				lo[d] = v[d]
			}
			if v[d] > hi[d] {
				hi[d] = v[d]
			}
		}
	}

	best, bestWidth := 0, int32(math.MinInt32)
	for d := range Dim {
		width := int32(hi[d]) - int32(lo[d])
		if width > bestWidth {
			bestWidth = width
			best = d
		}
	}
	return best
}

// selectMedian performs a Hoare-partition quickselect, reordering indices
// in place so that indices[:mid] holds the mid smallest elements under
// less(a, b) (unordered among themselves) and indices[mid:] holds the rest.
// This is the linear-time alternative to sorting the whole slice just to
// find one split point.
func selectMedian(indices []int32, mid int, less func(a, b int32) bool) {
	lo, hi := 0, len(indices)-1
	for lo < hi {
		pivot := indices[(lo+hi)/2]
		i, j := lo, hi
		for i <= j {
			for less(indices[i], pivot) {
				i++
			}
			for less(pivot, indices[j]) {
				j--
			}
			if i <= j {
				indices[i], indices[j] = indices[j], indices[i]
				i++
				j--
			}
		}
		switch {
		case mid <= j:
			hi = j
		case mid >= i:
			lo = i
		default:
			return
		}
	}
}
