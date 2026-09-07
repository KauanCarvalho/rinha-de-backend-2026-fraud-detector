package knn_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/knn"
	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/vectorize"
)

func TestDimMatchesVectorizeDim(t *testing.T) {
	t.Parallel()

	assert.Equal(t, vectorize.Dim, knn.Dim)
}

func TestQuantize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   knn.Vector
		want knn.QVector
	}{
		{
			name: "zeros",
			in:   knn.Vector{},
			want: knn.QVector{},
		},
		{
			name: "sentinel -1 values",
			in:   knn.Vector{0, 0, 0, 0, 0, -1, -1, 0, 0, 0, 0, 0, 0, 0},
			want: knn.QVector{0, 0, 0, 0, 0, -10000, -10000, 0, 0, 0, 0, 0, 0, 0},
		},
		{
			name: "max 1.0 values",
			in:   knn.Vector{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
			want: knn.QVector{
				10000,
				10000,
				10000,
				10000,
				10000,
				10000,
				10000,
				10000,
				10000,
				10000,
				10000,
				10000,
				10000,
				10000,
			},
		},
		{
			name: "rounds to nearest",
			in:   knn.Vector{0.15},
			want: knn.QVector{1500},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, knn.Quantize(tt.in))
		})
	}
}

func TestFraudCount(t *testing.T) {
	t.Parallel()

	neighbors := []knn.Neighbor{
		{Dist: 1, Label: knn.LabelFraud},
		{Dist: 2, Label: knn.LabelLegit},
		{Dist: 3, Label: knn.LabelFraud},
		{Dist: 4, Label: knn.LabelLegit},
		{Dist: 5, Label: knn.LabelFraud},
	}

	assert.Equal(t, 3, knn.FraudCount(neighbors))
}
