package knn_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/knn"
)

func TestSaveLoad_RoundTrip(t *testing.T) {
	t.Parallel()

	vectors, labels := randomDataset(1000, 5)
	idx := knn.Build(vectors, labels, knn.DefaultLeafSize)

	var buf bytes.Buffer
	require.NoError(t, idx.Save(&buf))

	loaded, err := knn.Load(&buf)
	require.NoError(t, err)
	require.Equal(t, idx.Len(), loaded.Len())

	for q := range 5 {
		query := randomQuery(int64(3000 + q))
		want := idx.Search(query, 5, 0)
		got := loaded.Search(query, 5, 0)
		assert.Equal(t, want, got)
	}
}

func TestSaveFileLoadFile_RoundTrip(t *testing.T) {
	t.Parallel()

	vectors, labels := randomDataset(500, 9)
	idx := knn.Build(vectors, labels, knn.DefaultLeafSize)

	path := t.TempDir() + "/index.bin"
	require.NoError(t, idx.SaveFile(path))

	loaded, err := knn.LoadFile(path)
	require.NoError(t, err)
	require.Equal(t, idx.Len(), loaded.Len())

	query := randomQuery(4242)
	assert.Equal(t, idx.Search(query, 5, 0), loaded.Search(query, 5, 0))
}

func TestLoadFile_MissingFile(t *testing.T) {
	t.Parallel()

	_, err := knn.LoadFile(t.TempDir() + "/does-not-exist.bin")

	assert.Error(t, err)
}
