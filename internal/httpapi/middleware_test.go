package httpapi

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecoverMiddleware_CatchesPanic(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	panicking := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("boom")
	})
	handler := recoverMiddleware(logger, panicking)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	assert.NotPanics(t, func() { handler.ServeHTTP(rec, req) })
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestRecoverMiddleware_PassesThroughWithoutPanic(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	handler := recoverMiddleware(logger, ok)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusTeapot, rec.Code)
}

func TestLoggingMiddleware_LogsRequestLine(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	handler := loggingMiddleware(logger, ok)

	req := httptest.NewRequest(http.MethodGet, "/whatever", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	var logLine map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &logLine))
	assert.Equal(t, "request", logLine["msg"])
	assert.Equal(t, http.MethodGet, logLine["method"])
	assert.Equal(t, "/whatever", logLine["path"])
	assert.Equal(t, float64(http.StatusTeapot), logLine["status"])
	assert.NotEmpty(t, logLine["request_id"])
	assert.Contains(t, logLine, "duration_ms")
}

func TestLoggingMiddleware_LogsStatusEvenAfterRecoveredPanic(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	panicking := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("boom")
	})
	// Mirrors the composition order used in NewHandler: logging must wrap
	// recover, so a panic still produces a request log line.
	handler := loggingMiddleware(logger, recoverMiddleware(logger, panicking))

	req := httptest.NewRequest(http.MethodGet, "/panics", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	var logLine map[string]any
	require.NoError(t, json.Unmarshal(bytes.Split(buf.Bytes(), []byte("\n"))[1], &logLine))
	assert.Equal(t, "request", logLine["msg"])
	assert.Equal(t, float64(http.StatusInternalServerError), logLine["status"])
}
