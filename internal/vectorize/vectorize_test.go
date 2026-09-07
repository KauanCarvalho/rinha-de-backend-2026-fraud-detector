package vectorize_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/domain"
	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/vectorize"
)

func mustParseTime(t *testing.T, s string) time.Time {
	t.Helper()

	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("invalid time %q: %v", s, err)
	}

	return ts
}

// TestVectorize_DetectionRulesExamples reproduces, dimension by dimension,
// the two full flow examples documented in docs/en/DETECTION_RULES.md (a
// legitimate transaction and a fraudulent one). These are golden test cases:
// any drift in the formulas must be caught here.
func TestVectorize_DetectionRulesExamples(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		req  domain.FraudScoreRequest
		want vectorize.Vector
	}{
		{
			name: "legitimate transaction, known merchant, no history",
			req: domain.FraudScoreRequest{
				ID: "tx-1329056812",
				Transaction: domain.TransactionInfo{
					Amount:       41.12,
					Installments: 2,
					RequestedAt:  mustParseTime(t, "2026-03-11T18:45:53Z"),
				},
				Customer: domain.CustomerInfo{
					AvgAmount:      82.24,
					TxCount24h:     3,
					KnownMerchants: []string{"MERC-003", "MERC-016"},
				},
				Merchant: domain.MerchantInfo{
					ID:        "MERC-016",
					MCC:       "5411",
					AvgAmount: 60.25,
				},
				Terminal: domain.TerminalInfo{
					IsOnline:    false,
					CardPresent: true,
					KmFromHome:  29.23,
				},
				LastTransaction: nil,
			},
			want: vectorize.Vector{0.0041, 0.1667, 0.05, 0.7826, 0.3333, -1, -1, 0.0292, 0.15, 0, 1, 0, 0.15, 0.006},
		},
		{
			name: "fraudulent transaction, unknown merchant, no history",
			req: domain.FraudScoreRequest{
				ID: "tx-3330991687",
				Transaction: domain.TransactionInfo{
					Amount:       9505.97,
					Installments: 10,
					RequestedAt:  mustParseTime(t, "2026-03-14T05:15:12Z"),
				},
				Customer: domain.CustomerInfo{
					AvgAmount:      81.28,
					TxCount24h:     20,
					KnownMerchants: []string{"MERC-008", "MERC-007", "MERC-005"},
				},
				Merchant: domain.MerchantInfo{
					ID:        "MERC-068",
					MCC:       "7802",
					AvgAmount: 54.86,
				},
				Terminal: domain.TerminalInfo{
					IsOnline:    false,
					CardPresent: true,
					KmFromHome:  952.27,
				},
				LastTransaction: nil,
			},
			want: vectorize.Vector{0.9506, 0.8333, 1.0, 0.2174, 0.8333, -1, -1, 0.9523, 1.0, 0, 1, 1, 0.75, 0.0055},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := vectorize.Vectorize(tt.req)
			for i := range got {
				assert.InDeltaf(t, tt.want[i], got[i], 1e-3, "dimension %d", i)
			}
		})
	}
}

func TestVectorize_EdgeCases(t *testing.T) {
	t.Parallel()

	baseReq := func() domain.FraudScoreRequest {
		return domain.FraudScoreRequest{
			Transaction: domain.TransactionInfo{
				Amount:       100,
				Installments: 1,
				RequestedAt:  mustParseTime(t, "2026-01-05T10:00:00Z"), // a Monday
			},
			Customer: domain.CustomerInfo{
				AvgAmount:      100,
				TxCount24h:     1,
				KnownMerchants: []string{"MERC-001"},
			},
			Merchant: domain.MerchantInfo{ID: "MERC-001", MCC: "5411", AvgAmount: 100},
			Terminal: domain.TerminalInfo{KmFromHome: 1},
		}
	}

	t.Run("last_transaction nil uses -1 sentinel", func(t *testing.T) {
		t.Parallel()

		req := baseReq()
		req.LastTransaction = nil

		got := vectorize.Vectorize(req)

		assert.Equal(t, -1.0, got[vectorize.DimMinutesSinceLastTx])
		assert.Equal(t, -1.0, got[vectorize.DimKmFromLastTx])
	})

	t.Run("last_transaction present computes normalized values", func(t *testing.T) {
		t.Parallel()

		req := baseReq()
		req.LastTransaction = &domain.LastTransaction{
			Timestamp:     mustParseTime(t, "2026-01-05T09:00:00Z"),
			KmFromCurrent: 500,
		}

		got := vectorize.Vectorize(req)

		assert.InDelta(t, 60.0/1440.0, got[vectorize.DimMinutesSinceLastTx], 1e-9)
		assert.InDelta(t, 500.0/1000.0, got[vectorize.DimKmFromLastTx], 1e-9)
	})

	t.Run("unknown mcc falls back to default risk 0.5", func(t *testing.T) {
		t.Parallel()

		req := baseReq()
		req.Merchant.MCC = "9999"

		got := vectorize.Vectorize(req)

		// 0.5 is the documented contract value (docs/en/DATASET.md: "MCC not
		// listed? 0.5 can be used as the default"), asserted as a literal
		// rather than against the package's own internal constant so this
		// test would still catch a regression if that constant drifted.
		assert.Equal(t, 0.5, got[vectorize.DimMCCRisk])
	})

	t.Run("known mcc uses risk table", func(t *testing.T) {
		t.Parallel()

		req := baseReq()
		req.Merchant.MCC = "7995" // 0.85 per resources/mcc_risk.json

		got := vectorize.Vectorize(req)

		assert.Equal(t, 0.85, got[vectorize.DimMCCRisk])
	})

	t.Run("merchant not in known_merchants is flagged unknown", func(t *testing.T) {
		t.Parallel()

		req := baseReq()
		req.Merchant.ID = "MERC-999"

		got := vectorize.Vectorize(req)

		assert.Equal(t, 1.0, got[vectorize.DimUnknownMerchant])
	})

	t.Run("merchant in known_merchants is not flagged", func(t *testing.T) {
		t.Parallel()

		req := baseReq()

		got := vectorize.Vectorize(req)

		assert.Equal(t, 0.0, got[vectorize.DimUnknownMerchant])
	})

	t.Run("values above ceiling are clamped to 1.0", func(t *testing.T) {
		t.Parallel()

		req := baseReq()
		req.Transaction.Amount = 999_999 // way above max_amount=10000
		req.Customer.TxCount24h = 500    // way above max_tx_count_24h=20
		req.Terminal.KmFromHome = 999_999

		got := vectorize.Vectorize(req)

		assert.Equal(t, 1.0, got[vectorize.DimAmount])
		assert.Equal(t, 1.0, got[vectorize.DimTxCount24h])
		assert.Equal(t, 1.0, got[vectorize.DimKmFromHome])
	})

	t.Run("day of week maps monday=0 sunday=6", func(t *testing.T) {
		t.Parallel()

		req := baseReq()
		req.Transaction.RequestedAt = mustParseTime(t, "2026-01-04T00:00:00Z") // a Sunday

		got := vectorize.Vectorize(req)

		assert.InDelta(t, 1.0, got[vectorize.DimDayOfWeek], 1e-9) // sunday is weekday index 6, and 6/6 == 1.0
	})

	t.Run("zero customer avg_amount with positive amount is treated as max risk", func(t *testing.T) {
		t.Parallel()

		req := baseReq()
		req.Customer.AvgAmount = 0

		got := vectorize.Vectorize(req)

		assert.Equal(t, 1.0, got[vectorize.DimAmountVsAvg])
	})

	t.Run("zero customer avg_amount and zero amount is treated as no risk", func(t *testing.T) {
		t.Parallel()

		req := baseReq()
		req.Customer.AvgAmount = 0
		req.Transaction.Amount = 0

		got := vectorize.Vectorize(req)

		assert.Equal(t, 0.0, got[vectorize.DimAmountVsAvg])
	})
}
