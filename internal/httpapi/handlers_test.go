package httpapi_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/detector"
	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/domain"
	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/httpapi"
	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/knn"
	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/observability"
)

const validFraudScoreBody = `{
	"id": "tx-1",
	"transaction": {"amount": 41.12, "installments": 2, "requested_at": "2026-03-11T18:45:53Z"},
	"customer": {"avg_amount": 82.24, "tx_count_24h": 3, "known_merchants": ["MERC-016"]},
	"merchant": {"id": "MERC-016", "mcc": "5411", "avg_amount": 60.25},
	"terminal": {"is_online": false, "card_present": true, "km_from_home": 29.23},
	"last_transaction": null
}`

func testHandlerWithLabel(t *testing.T, label knn.Label) http.Handler {
	t.Helper()

	var point knn.QVector
	vectors := make([]knn.QVector, 10)
	labels := make([]knn.Label, 10)
	for i := range vectors {
		vectors[i] = point
		labels[i] = label
	}
	idx := knn.BuildPartitioned(vectors, labels, knn.DefaultLeafSize)
	det := detector.New(idx, 0)
	logger := observability.NewLogger("error") // keep test output quiet

	return httpapi.NewHandler(det, logger)
}

func testHandler(t *testing.T) http.Handler {
	t.Helper()
	return testHandlerWithLabel(t, knn.LabelLegit)
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

	req := httptest.NewRequest(http.MethodPost, "/fraud-score", bytes.NewBufferString(validFraudScoreBody))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var resp domain.FraudScoreResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, 0.0, resp.FraudScore)
	assert.True(t, resp.Approved)
}

func TestFraudScoreHandler_ResponseBodyIsExactPreRenderedJSON(t *testing.T) {
	t.Parallel()

	handler := testHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/fraud-score", bytes.NewBufferString(validFraudScoreBody))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	expected, err := json.Marshal(domain.FraudScoreResponse{Approved: true, FraudScore: 0.0})
	require.NoError(t, err)
	expected = append(expected, '\n')

	assert.Equal(t, expected, rec.Body.Bytes(),
		"pre-rendered body must be byte-identical to json.Marshal, not just semantically equal")
}

func TestFraudScoreHandler_AllFraudNeighbors(t *testing.T) {
	t.Parallel()

	handler := testHandlerWithLabel(t, knn.LabelFraud)

	req := httptest.NewRequest(http.MethodPost, "/fraud-score", bytes.NewBufferString(validFraudScoreBody))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	got := rec.Body.Bytes()

	var resp domain.FraudScoreResponse
	require.NoError(t, json.Unmarshal(got, &resp))
	assert.Equal(t, 1.0, resp.FraudScore)
	assert.False(t, resp.Approved)

	expected, err := json.Marshal(domain.FraudScoreResponse{Approved: false, FraudScore: 1.0})
	require.NoError(t, err)
	expected = append(expected, '\n')
	assert.Equal(t, expected, got)
}

func TestFraudScoreHandler_ConcurrentRequestsDoNotShareBufferPoolState(t *testing.T) {
	t.Parallel()

	handler := testHandler(t)

	const goroutines = 64
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()

			req := httptest.NewRequest(http.MethodPost, "/fraud-score", bytes.NewBufferString(validFraudScoreBody))
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)

			var resp domain.FraudScoreResponse
			assert.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
			assert.Equal(t, 0.0, resp.FraudScore)
			assert.True(t, resp.Approved)
		}()
	}
	wg.Wait()
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
