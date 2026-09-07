// Package knn implements an in-process IVF (Inverted-File) approximate
// nearest-neighbor index over int16-quantized fixed-size vectors, further
// split into coarse partitions by a few cheap categorical dimensions (see
// Tag) before IVF search even runs.
//
// It knows nothing about fraud detection: callers are responsible for
// picking k and interpreting the returned labels (see FraudCount for the
// one convenience built on top of Label).
package knn

import "math"

// Dim is the number of dimensions in an indexed vector. It must match
// vectorize.Dim — the two packages are kept decoupled on purpose, so this
// is asserted by TestDimMatchesVectorizeDim in index_test.go instead of a
// shared import.
const Dim = 14

// Scale converts a normalized float64 in [-1.0, 1.0] to the int16 domain
// used for storage and distance calculations. The challenge's vectors live
// in [0.0, 1.0] with a -1 sentinel (see docs/en/DETECTION_RULES.md), so a
// scale of 10000 keeps ample headroom inside int16's [-32768, 32767] range.
const Scale = 10000

// Label classifies a reference vector.
type Label uint8

// The two possible Label values.
const (
	LabelLegit Label = 0
	LabelFraud Label = 1
)

// Vector is an unquantized, unscaled feature vector.
type Vector [Dim]float64

// QVector is the int16-quantized, scaled form of a Vector used throughout
// the index for compact storage and fast integer distance math.
type QVector [Dim]int16

// Quantize scales and rounds v into its QVector form.
func Quantize(v Vector) QVector {
	var q QVector
	for i, x := range v {
		scaled := math.Round(x * Scale)
		switch {
		case scaled > math.MaxInt16:
			scaled = math.MaxInt16
		case scaled < math.MinInt16:
			scaled = math.MinInt16
		}
		q[i] = int16(scaled)
	}
	return q
}

// Neighbor is one result of a Search or BruteForceSearch call.
type Neighbor struct {
	Dist  int64
	Label Label
}

// FraudCount returns how many of the given neighbors are labeled as fraud.
func FraudCount(neighbors []Neighbor) int {
	c := 0
	for _, n := range neighbors {
		if n.Label == LabelFraud {
			c++
		}
	}
	return c
}

func sqDist(a, b QVector) int64 {
	var sum int64
	for i := range Dim {
		d := int64(a[i]) - int64(b[i])
		sum += d * d
	}
	return sum
}
