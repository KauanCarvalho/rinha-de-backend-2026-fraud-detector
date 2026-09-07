package scoring_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/scoring"
)

func TestFraudScore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		fraudCount int
		want       float64
	}{
		{"no frauds among neighbors (legit example)", 0, 0.0},
		{"all frauds among neighbors (fraud example)", 5, 1.0},
		{"three of five", 3, 0.6},
		{"one of five", 1, 0.2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, scoring.FraudScore(tt.fraudCount))
		})
	}
}

func TestApproved(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		fraudScore float64
		want       bool
	}{
		{"zero score is approved", 0.0, true},
		{"just below threshold is approved", 0.599, true},
		{"exactly at threshold is denied", 0.6, false},
		{"above threshold is denied", 0.8, false},
		{"max score is denied", 1.0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, scoring.Approved(tt.fraudScore))
		})
	}
}
