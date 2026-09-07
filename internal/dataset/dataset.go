// Package dataset parses the reference dataset format documented in
// docs/en/DATASET.md: a JSON array of {"vector": [14 floats], "label":
// "fraud"|"legit"} records.
package dataset

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/knn"
)

// record matches one entry of the reference dataset.
type record struct {
	Vector [knn.Dim]float64 `json:"vector"`
	Label  string           `json:"label"`
}

// LoadReferences streams a JSON array of reference records from r,
// quantizing each one as it is decoded so the full decompressed payload
// (~100-300MB for the official dataset) is never held as a single decoded
// blob. onProgress, if non-nil, is called after every record with the
// running count so far — useful for logging progress on large inputs.
func LoadReferences(r io.Reader, expectedCount int, onProgress func(count int)) ([]knn.QVector, []knn.Label, error) {
	dec := json.NewDecoder(r)
	if _, err := dec.Token(); err != nil { // consume opening '['
		return nil, nil, fmt.Errorf("read opening token: %w", err)
	}

	vectors := make([]knn.QVector, 0, expectedCount)
	labels := make([]knn.Label, 0, expectedCount)

	for dec.More() {
		var rec record
		if err := dec.Decode(&rec); err != nil {
			return nil, nil, fmt.Errorf("decode record %d: %w", len(vectors), err)
		}

		label, err := parseLabel(rec.Label)
		if err != nil {
			return nil, nil, fmt.Errorf("record %d: %w", len(vectors), err)
		}

		vectors = append(vectors, knn.Quantize(knn.Vector(rec.Vector)))
		labels = append(labels, label)

		if onProgress != nil {
			onProgress(len(vectors))
		}
	}

	if _, err := dec.Token(); err != nil { // consume closing ']'
		return nil, nil, fmt.Errorf("read closing token: %w", err)
	}

	return vectors, labels, nil
}

// LoadReferencesFile opens path as plain JSON (e.g.
// resources/example-references.json) and loads it via LoadReferences.
func LoadReferencesFile(
	path string,
	expectedCount int,
	onProgress func(count int),
) ([]knn.QVector, []knn.Label, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = f.Close() }()

	return LoadReferences(f, expectedCount, onProgress)
}

// LoadReferencesGzipFile opens path as a gzip-compressed JSON file (the
// official resources/references.json.gz) and loads it via LoadReferences.
func LoadReferencesGzipFile(
	path string,
	expectedCount int,
	onProgress func(count int),
) ([]knn.QVector, []knn.Label, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = f.Close() }()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, nil, fmt.Errorf("open gzip stream: %w", err)
	}
	defer func() { _ = gz.Close() }()

	return LoadReferences(gz, expectedCount, onProgress)
}

func parseLabel(s string) (knn.Label, error) {
	switch s {
	case "fraud":
		return knn.LabelFraud, nil
	case "legit":
		return knn.LabelLegit, nil
	default:
		return 0, fmt.Errorf("unknown label %q", s)
	}
}
