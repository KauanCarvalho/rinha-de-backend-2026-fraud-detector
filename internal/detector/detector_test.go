package detector_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/detector"
	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/domain"
	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/knn"
	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/vectorize"
)

func sampleRequest() domain.FraudScoreRequest {
	return domain.FraudScoreRequest{
		ID: "tx-test-1",
		Transaction: domain.TransactionInfo{
			Amount:       41.12,
			Installments: 2,
			RequestedAt:  time.Date(2026, 3, 11, 18, 45, 53, 0, time.UTC),
		},
		Customer: domain.CustomerInfo{
			AvgAmount:      82.24,
			TxCount24h:     3,
			KnownMerchants: []string{"MERC-003", "MERC-016"},
		},
		Merchant: domain.MerchantInfo{ID: "MERC-016", MCC: "5411", AvgAmount: 60.25},
		Terminal: domain.TerminalInfo{IsOnline: false, CardPresent: true, KmFromHome: 29.23},
	}
}

func buildIndexAroundQuery(t *testing.T, req domain.FraudScoreRequest, fraudNear, legitNear int) *knn.Index {
	t.Helper()

	query := knn.Quantize(knn.Vector(vectorize.Vectorize(req)))

	var vectors []knn.QVector
	var labels []knn.Label

	for range fraudNear {
		vectors = append(vectors, query)
		labels = append(labels, knn.LabelFraud)
	}
	for range legitNear {
		vectors = append(vectors, query)
		labels = append(labels, knn.LabelLegit)
	}

	var far knn.QVector
	for d := range far {
		far[d] = 32000 // outside the [-10000, 10000] domain any real vectorize() output can reach
	}
	for range 50 {
		vectors = append(vectors, far)
		labels = append(labels, knn.LabelLegit)
	}

	return knn.Build(vectors, labels, knn.DefaultLeafSize)
}

func TestDetector_Evaluate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		fraudNear    int
		legitNear    int
		wantScore    float64
		wantApproved bool
	}{
		{"all five nearest are fraud", 5, 0, 1.0, false},
		{"all five nearest are legit", 0, 5, 0.0, true},
		{"three of five nearest are fraud (at threshold)", 3, 2, 0.6, false},
		{"two of five nearest are fraud (below threshold)", 2, 3, 0.4, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := sampleRequest()
			idx := buildIndexAroundQuery(t, req, tt.fraudNear, tt.legitNear)
			d := detector.New(idx, 0)

			got := d.Evaluate(req)

			require.Equal(t, tt.wantScore, got.FraudScore)
			assert.Equal(t, tt.wantApproved, got.Approved)
		})
	}
}
