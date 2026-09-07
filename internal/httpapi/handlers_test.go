package httpapi_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/detector"
	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/domain"
	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/httpapi"
	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/knn"
	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/observability"
)

// testHandler builds a real handler backed by a tiny, fully-legit
// reference index, so POST /fraud-score exercises the full
// vectorize -> knn -> scoring pipeline through the HTTP layer.
func testHandler(t *testing.T) http.Handler {
	t.Helper()

	var legit knn.QVector
	vectors := make([]knn.QVector, 10)
	labels := make([]knn.Label, 10)
	for i := range vectors {
		vectors[i] = legit
		labels[i] = knn.LabelLegit
	}
	idx := knn.Build(vectors, labels, knn.DefaultLeafSize)
	det := detector.New(idx, 0)
	logger := observability.NewLogger("error") // keep test output quiet

	return httpapi.NewHandler(det, logger)
}

func TestReadyHandler(t *testing.T) {
	t.Parallel()

	handler := testHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestFraudScoreHandler_ValidRequest(t *testing.T) {
	t.Parallel()

	handler := testHandler(t)

	body := `{
		"id": "tx-1",
		"transaction": {"amount": 41.12, "installments": 2, "requested_at": "2026-03-11T18:45:53Z"},
		"customer": {"avg_amount": 82.24, "tx_count_24h": 3, "known_merchants": ["MERC-016"]},
		"merchant": {"id": "MERC-016", "mcc": "5411", "avg_amount": 60.25},
		"terminal": {"is_online": false, "card_present": true, "km_from_home": 29.23},
		"last_transaction": null
	}`
	req := httptest.NewRequest(http.MethodPost, "/fraud-score", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var resp domain.FraudScoreResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, 0.0, resp.FraudScore)
	assert.True(t, resp.Approved)
}

func TestFraudScoreHandler_InvalidBody(t *testing.T) {
	t.Parallel()

	handler := testHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/fraud-score", bytes.NewBufferString(`not json`))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestFraudScoreHandler_WrongMethod(t *testing.T) {
	t.Parallel()

	handler := testHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/fraud-score", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestUnknownRoute(t *testing.T) {
	t.Parallel()

	handler := testHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/nope", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestFraudScoreHandler_EmptyBody(t *testing.T) {
	t.Parallel()

	handler := testHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/fraud-score", io.NopCloser(bytes.NewReader(nil)))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
