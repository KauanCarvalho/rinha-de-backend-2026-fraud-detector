package knn

const (
	tagDimHasLastTx       = 5
	tagDimIsOnline        = 9
	tagDimCardPresent     = 10
	tagDimUnknownMerchant = 11
)

const (
	tagBitHasNoLastTx     = 0
	tagBitIsOnline        = 1
	tagBitCardPresent     = 2
	tagBitUnknownMerchant = 3
)

const NumPartitions = 16

func Tag(q QVector) uint8 {
	var tag uint8
	if q[tagDimHasLastTx] < 0 {
		tag |= 1 << tagBitHasNoLastTx
	}
	if q[tagDimIsOnline] != 0 {
		tag |= 1 << tagBitIsOnline
	}
	if q[tagDimCardPresent] != 0 {
		tag |= 1 << tagBitCardPresent
	}
	if q[tagDimUnknownMerchant] != 0 {
		tag |= 1 << tagBitUnknownMerchant
	}
	return tag
}

type PartitionedIndex struct {
	Partitions [NumPartitions]Index
}

func BuildPartitioned(vectors []QVector, labels []Label, leafSize int) *PartitionedIndex {
	var bucketVectors [NumPartitions][]QVector
	var bucketLabels [NumPartitions][]Label
	for i, v := range vectors {
		t := Tag(v)
		bucketVectors[t] = append(bucketVectors[t], v)
		bucketLabels[t] = append(bucketLabels[t], labels[i])
	}

	pi := &PartitionedIndex{}
	for t := range pi.Partitions {
		if len(bucketVectors[t]) == 0 {
			continue
		}
		pi.Partitions[t] = *Build(bucketVectors[t], bucketLabels[t], leafSize)
	}
	return pi
}

func (pi *PartitionedIndex) Len() int {
	n := 0
	for i := range pi.Partitions {
		n += pi.Partitions[i].Len()
	}
	return n
}

func (pi *PartitionedIndex) Search(query QVector, k, maxExtraLeaves int) []Neighbor {
	if p := &pi.Partitions[Tag(query)]; p.Len() > 0 {
		return p.Search(query, k, maxExtraLeaves)
	}

	r := newResult(k)
	for i := range pi.Partitions {
		if pi.Partitions[i].Len() == 0 {
			continue
		}
		for _, n := range pi.Partitions[i].Search(query, k, maxExtraLeaves) {
			r.tryInsert(n.Dist, n.Label)
		}
	}
	return r.neighbors
}
