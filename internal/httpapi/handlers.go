package httpapi

import (
	"bytes"
	"encoding/json"
	"math"
	"net/http"
	"sync"

	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/detector"
	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/domain"
	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/scoring"
)

// fraudScoreResponseBodies holds the pre-encoded JSON body (including the
// trailing newline [json.Encoder] would add) for every possible
// /fraud-score response. FraudScore is always fraudCount/scoring.K for a
// fraudCount in [0, scoring.K] (see internal/scoring.FraudScore), so there
// are only scoring.K+1 distinct response bodies that can ever be produced.
// Encoding them once here, at package init, instead of on every request,
// removes encoding/json's reflection-based marshaling from the hot path
// entirely without changing a single byte of what a client receives.
//
//nolint:gochecknoglobals // read-only after init; see comment above
var fraudScoreResponseBodies = buildFraudScoreResponseBodies()

func buildFraudScoreResponseBodies() [scoring.K + 1][]byte {
	var bodies [scoring.K + 1][]byte
	for fraudCount := range bodies {
		score := scoring.FraudScore(fraudCount)
		body, err := json.Marshal(domain.FraudScoreResponse{
			Approved:   scoring.Approved(score),
			FraudScore: score,
		})
		if err != nil {
			panic(err)
		}
		bodies[fraudCount] = append(body, '\n')
	}
	return bodies
}

// requestBodyBufferPool reuses the []byte-backed buffer used to read each
// request body, instead of letting [json.Decoder] allocate a fresh internal
// buffer per request. Buffers are always Reset before use, so nothing from
// a previous request is ever visible to the next one. A [sync.Pool] is
// inherently meant to be shared package- or process-wide — that is its
// entire purpose — so this is not mutable shared state in the sense the
// gochecknoglobals linter otherwise guards against.
//
//nolint:gochecknoglobals // sync.Pool is designed to be a shared global; see comment above
var requestBodyBufferPool = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

// readyHandler backs GET /ready (docs/en/API.md): any 2xx means the
// process has a loaded index and is willing to take traffic. By the time
// the server is registered with the mux (see NewServer), the index has
// already been loaded successfully, so this can unconditionally report OK.
func readyHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// fraudScoreHandler backs POST /fraud-score (docs/en/API.md).
func fraudScoreHandler(det *detector.Detector) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		buf, ok := requestBodyBufferPool.Get().(*bytes.Buffer)
		if !ok {
			panic("requestBodyBufferPool: unexpected type")
		}
		buf.Reset()
		defer requestBodyBufferPool.Put(buf)

		if _, err := buf.ReadFrom(r.Body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		var req domain.FraudScoreRequest
		if err := json.Unmarshal(buf.Bytes(), &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		resp := det.Evaluate(req)
		fraudCount := int(math.Round(resp.FraudScore * float64(scoring.K)))
		writeRawJSON(w, http.StatusOK, fraudScoreResponseBodies[fraudCount])
	}
}

func writeRawJSON(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
