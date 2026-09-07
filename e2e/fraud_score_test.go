//go:build e2e

// Package e2e exercises the real, containerized stack (HAProxy + 2 API
// replicas) over the network, the way the challenge's own test harness
// does. It expects the stack to already be running — see `make docker-up`
// — and is excluded from `go test ./...` by the e2e build tag so unit test
// runs stay hermetic and fast.
package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/domain"
)

const baseURL = "http://localhost:9999"

var client = &http.Client{Timeout: 5 * time.Second}

func TestReady(t *testing.T) {
	resp, err := client.Get(baseURL + "/ready")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Less(t, resp.StatusCode, 300)
}

// TestFraudScore_DetectionRulesExamples posts the two fully worked examples
// from docs/en/DETECTION_RULES.md against the real running stack (real
// HAProxy round-robin, real API replicas, real reference index) and checks
// the response matches the documented answer exactly.
func TestFraudScore_DetectionRulesExamples(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		wantApproved bool
		wantScore    float64
	}{
		{
			name: "legitimate transaction",
			body: `{
				"id": "tx-1329056812",
				"transaction": {"amount": 41.12, "installments": 2, "requested_at": "2026-03-11T18:45:53Z"},
				"customer": {"avg_amount": 82.24, "tx_count_24h": 3, "known_merchants": ["MERC-003", "MERC-016"]},
				"merchant": {"id": "MERC-016", "mcc": "5411", "avg_amount": 60.25},
				"terminal": {"is_online": false, "card_present": true, "km_from_home": 29.23},
				"last_transaction": null
			}`,
			wantApproved: true,
			wantScore:    0.0,
		},
		{
			name: "fraudulent transaction",
			body: `{
				"id": "tx-3330991687",
				"transaction": {"amount": 9505.97, "installments": 10, "requested_at": "2026-03-14T05:15:12Z"},
				"customer": {"avg_amount": 81.28, "tx_count_24h": 20, "known_merchants": ["MERC-008", "MERC-007", "MERC-005"]},
				"merchant": {"id": "MERC-068", "mcc": "7802", "avg_amount": 54.86},
				"terminal": {"is_online": false, "card_present": true, "km_from_home": 952.27},
				"last_transaction": null
			}`,
			wantApproved: false,
			wantScore:    1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := postFraudScore(t, []byte(tt.body))

			assert.Equal(t, tt.wantApproved, resp.Approved)
			assert.Equal(t, tt.wantScore, resp.FraudScore)
		})
	}
}

// TestFraudScore_ExamplePayloads sends every payload from the official
// resources/example-payloads.json and checks each response is well-formed:
// a 200 with a boolean `approved` consistent with the `fraud_score`
// threshold. It does not assert specific outcomes (those depend on
// whatever reference dataset the stack was built against), only that the
// API answers correctly-shaped, internally-consistent decisions for
// realistic traffic.
func TestFraudScore_ExamplePayloads(t *testing.T) {
	data, err := os.ReadFile("../resources/example-payloads.json")
	require.NoError(t, err)

	var payloads []json.RawMessage
	require.NoError(t, json.Unmarshal(data, &payloads))
	require.NotEmpty(t, payloads)

	for i, payload := range payloads {
		t.Run(fmt.Sprintf("payload_%d", i), func(t *testing.T) {
			resp := postFraudScore(t, payload)

			assert.GreaterOrEqual(t, resp.FraudScore, 0.0)
			assert.LessOrEqual(t, resp.FraudScore, 1.0)
			assert.Equal(t, resp.FraudScore < 0.6, resp.Approved)
		})
	}
}

func postFraudScore(t *testing.T, body []byte) domain.FraudScoreResponse {
	t.Helper()

	httpResp, err := client.Post(baseURL+"/fraud-score", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer httpResp.Body.Close()
	require.Equal(t, http.StatusOK, httpResp.StatusCode)

	var resp domain.FraudScoreResponse
	require.NoError(t, json.NewDecoder(httpResp.Body).Decode(&resp))
	return resp
}
