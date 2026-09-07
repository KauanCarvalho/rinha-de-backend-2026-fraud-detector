package knn

import (
	"bufio"
	"encoding/gob"
	"io"
	"os"
)

// ioBufferSize sizes the buffered reader/writer used around the multi-MB
// gob stream, cutting down on syscalls versus [os.File]'s own unbuffered I/O.
const ioBufferSize = 4 << 20 // 4MB

// Save writes idx to w using gob. gob was chosen over a hand-rolled binary
// format on purpose: the reference index is built once (at Docker image
// build time, see cmd/indexbuilder) and loaded once per process start, so
// its encode/decode speed is not on the hot path — plain stdlib
// serialization is less code to maintain and to get wrong than a manual
// byte-offset format.
func (idx *Index) Save(w io.Writer) error {
	return gob.NewEncoder(w).Encode(idx)
}

// SaveFile writes idx to the given path, creating or truncating it.
func (idx *Index) SaveFile(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	bw := bufio.NewWriterSize(f, ioBufferSize)
	if saveErr := idx.Save(bw); saveErr != nil {
		return saveErr
	}
	return bw.Flush()
}

// Load reads an Index previously written by Save.
func Load(r io.Reader) (*Index, error) {
	var idx Index
	if err := gob.NewDecoder(r).Decode(&idx); err != nil {
		return nil, err
	}
	return &idx, nil
}

// LoadFile reads an Index from the given path.
func LoadFile(path string) (*Index, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	return Load(bufio.NewReaderSize(f, ioBufferSize))
}
