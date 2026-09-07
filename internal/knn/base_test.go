// Package knn_test's base_test.go holds helpers shared across this
// package's other test files (build_test.go, search_test.go,
// serialize_test.go) instead of duplicating them or piling every test into
// one generic file.
package knn_test

import (
	"math/rand"

	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/knn"
)

// randomDataset builds n random quantized vectors with a random legit/fraud
// label, using a fixed seed for reproducibility.
func randomDataset(n int, seed int64) ([]knn.QVector, []knn.Label) {
	rng := rand.New(rand.NewSource(seed))
	vectors := make([]knn.QVector, n)
	labels := make([]knn.Label, n)

	for i := range n {
		for d := range knn.Dim {
			vectors[i][d] = int16(rng.Intn(20001) - 10000) // [-10000, 10000]
		}
		if rng.Intn(2) == 0 {
			labels[i] = knn.LabelLegit
		} else {
			labels[i] = knn.LabelFraud
		}
	}
	return vectors, labels
}

func randomQuery(seed int64) knn.QVector {
	rng := rand.New(rand.NewSource(seed))
	var q knn.QVector
	for d := range knn.Dim {
		q[d] = int16(rng.Intn(20001) - 10000)
	}
	return q
}
