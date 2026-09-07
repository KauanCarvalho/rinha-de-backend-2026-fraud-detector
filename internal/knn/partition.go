package knn

// Partition-tag dimension indices. These mirror vectorize's
// DimMinutesSinceLastTx, DimIsOnline, DimCardPresent, and
// DimUnknownMerchant (see internal/vectorize) — duplicated here, not
// imported, to keep this package decoupled from vectorize (see the package
// doc comment). TestTag in partition_test.go builds its inputs using the
// real vectorize.Dim* constants, so a drift between the two would fail
// that test rather than silently tagging the wrong dimensions.
const (
	tagDimHasLastTx       = 5
	tagDimIsOnline        = 9
	tagDimCardPresent     = 10
	tagDimUnknownMerchant = 11
)

// Bit positions within the tag returned by Tag.
const (
	tagBitHasNoLastTx     = 0
	tagBitIsOnline        = 1
	tagBitCardPresent     = 2
	tagBitUnknownMerchant = 3
)

// NumPartitions is the number of buckets Tag can return (2^4: one bit each
// for has-last-transaction, is-online, card-present, unknown-merchant).
const NumPartitions = 16

// Tag derives a coarse partition key from q's already boolean-valued
// dimensions (card_present, is_online, unknown_merchant) plus whether
// last_transaction was present at all (the sentinel on
// DimMinutesSinceLastTx). Each of these dimensions is exactly 0 or Scale
// pre-mismatch, or the noHistory sentinel, so a mismatch on any one of them
// alone contributes roughly Scale² to a squared-distance sum — typically
// larger than the combined contribution of every continuous dimension
// being merely "close" — which is why two vectors with different tags are
// rarely each other's true nearest neighbors. Partitioning by this tag
// before searching trades a small, empirically-measured amount of recall
// for a roughly NumPartitions-fold reduction in search space per query.
// See RESULTS.md for the measured trade-off.
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

// PartitionedIndex holds one IVFIndex per Tag value, built from only the
// reference vectors sharing that tag. A zero-value IVFIndex (Len() == 0)
// means no reference vector ever had that tag. Partitions is a fixed-size
// array of values rather than pointers so that gob — which rejects nil
// pointers inside arrays — can serialize an index with unpopulated
// partitions.
type PartitionedIndex struct {
	Partitions [NumPartitions]IVFIndex
}

// BuildPartitioned splits vectors and labels into NumPartitions buckets by
// Tag and builds one IVFIndex per non-empty bucket.
func BuildPartitioned(vectors []QVector, labels []Label) *PartitionedIndex {
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
		pi.Partitions[t] = *BuildIVF(bucketVectors[t], bucketLabels[t])
	}
	return pi
}

// Len returns the total number of vectors across every partition.
func (pi *PartitionedIndex) Len() int {
	n := 0
	for i := range pi.Partitions {
		n += pi.Partitions[i].Len()
	}
	return n
}

// Search routes query to the IVFIndex for its Tag and searches only that
// partition (with the given nprobe — see IVFIndex.Search). If that
// partition is empty — a tag combination never seen in the reference set —
// it falls back to searching every non-empty partition and merging the
// results, so Search never returns worse candidates than a real
// (unpartitioned) index would for a genuinely novel tag combination.
func (pi *PartitionedIndex) Search(query QVector, k, nprobe int) []Neighbor {
	if p := &pi.Partitions[Tag(query)]; p.Len() > 0 {
		return p.Search(query, k, nprobe)
	}

	r := newResult(k)
	for i := range pi.Partitions {
		if pi.Partitions[i].Len() == 0 {
			continue
		}
		for _, n := range pi.Partitions[i].Search(query, k, nprobe) {
			r.tryInsert(n.Dist, n.Label)
		}
	}
	return r.neighbors
}
