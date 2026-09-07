package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/detector"
	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/domain"
)

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
		var req domain.FraudScoreRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		resp := det.Evaluate(req)
		writeJSON(w, http.StatusOK, resp)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
