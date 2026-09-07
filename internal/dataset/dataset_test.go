package dataset_test

import (
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/dataset"
	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/knn"
)

const validJSON = `[
  {"vector": [0.01, 0.0833, 0.05, 0.8261, 0.1667, -1, -1, 0.0432, 0.25, 0, 1, 0, 0.2, 0.0416], "label": "legit"},
  {"vector": [0.5796, 0.9167, 1.0, 0.0435, 0, 0.0056, 0.4394, 0.4598, 0.4, 1, 0, 1, 0.85, 0.0032], "label": "fraud"}
]`

func TestLoadReferences_Valid(t *testing.T) {
	t.Parallel()

	vectors, labels, err := dataset.LoadReferences(strings.NewReader(validJSON), 0, nil)

	require.NoError(t, err)
	require.Len(t, vectors, 2)
	require.Len(t, labels, 2)

	assert.Equal(t, knn.LabelLegit, labels[0])
	assert.Equal(t, knn.LabelFraud, labels[1])
	assert.Equal(
		t,
		knn.Quantize(knn.Vector{0.01, 0.0833, 0.05, 0.8261, 0.1667, -1, -1, 0.0432, 0.25, 0, 1, 0, 0.2, 0.0416}),
		vectors[0],
	)
}

func TestLoadReferences_UnknownLabel(t *testing.T) {
	t.Parallel()

	input := `[{"vector": [0,0,0,0,0,0,0,0,0,0,0,0,0,0], "label": "maybe"}]`

	_, _, err := dataset.LoadReferences(strings.NewReader(input), 0, nil)

	assert.ErrorContains(t, err, `unknown label "maybe"`)
}

func TestLoadReferences_MalformedJSON(t *testing.T) {
	t.Parallel()

	_, _, err := dataset.LoadReferences(strings.NewReader(`not json`), 0, nil)

	assert.Error(t, err)
}

func TestLoadReferences_EmptyArray(t *testing.T) {
	t.Parallel()

	vectors, labels, err := dataset.LoadReferences(strings.NewReader(`[]`), 0, nil)

	require.NoError(t, err)
	assert.Empty(t, vectors)
	assert.Empty(t, labels)
}

func TestLoadReferences_ProgressCallback(t *testing.T) {
	t.Parallel()

	var seen []int
	_, _, err := dataset.LoadReferences(strings.NewReader(validJSON), 0, func(count int) {
		seen = append(seen, count)
	})

	require.NoError(t, err)
	assert.Equal(t, []int{1, 2}, seen)
}

func TestLoadReferencesFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "refs.json")
	require.NoError(t, os.WriteFile(path, []byte(validJSON), 0o600))

	vectors, _, err := dataset.LoadReferencesFile(path, 0, nil)

	require.NoError(t, err)
	assert.Len(t, vectors, 2)
}

func TestLoadReferencesGzipFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "refs.json.gz")

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	_, err := gw.Write([]byte(validJSON))
	require.NoError(t, err)
	require.NoError(t, gw.Close())
	require.NoError(t, os.WriteFile(path, buf.Bytes(), 0o600))

	vectors, labels, err := dataset.LoadReferencesGzipFile(path, 0, nil)

	require.NoError(t, err)
	assert.Len(t, vectors, 2)
	assert.Len(t, labels, 2)
}
