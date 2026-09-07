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
func (idx *IVFIndex) Save(w io.Writer) error {
	return gob.NewEncoder(w).Encode(idx)
}

// SaveFile writes idx to the given path, creating or truncating it.
func (idx *IVFIndex) SaveFile(path string) error {
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

// LoadIVF reads an IVFIndex previously written by Save.
func LoadIVF(r io.Reader) (*IVFIndex, error) {
	var idx IVFIndex
	if err := gob.NewDecoder(r).Decode(&idx); err != nil {
		return nil, err
	}
	return &idx, nil
}

// LoadIVFFile reads an IVFIndex from the given path.
func LoadIVFFile(path string) (*IVFIndex, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	return LoadIVF(bufio.NewReaderSize(f, ioBufferSize))
}

// Save writes pi to w using gob. See IVFIndex.Save for why gob.
func (pi *PartitionedIndex) Save(w io.Writer) error {
	return gob.NewEncoder(w).Encode(pi)
}

// SaveFile writes pi to the given path, creating or truncating it.
func (pi *PartitionedIndex) SaveFile(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	bw := bufio.NewWriterSize(f, ioBufferSize)
	if saveErr := pi.Save(bw); saveErr != nil {
		return saveErr
	}
	return bw.Flush()
}

// LoadPartitioned reads a PartitionedIndex previously written by Save.
func LoadPartitioned(r io.Reader) (*PartitionedIndex, error) {
	var pi PartitionedIndex
	if err := gob.NewDecoder(r).Decode(&pi); err != nil {
		return nil, err
	}
	return &pi, nil
}

// LoadPartitionedFile reads a PartitionedIndex from the given path.
func LoadPartitionedFile(path string) (*PartitionedIndex, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	return LoadPartitioned(bufio.NewReaderSize(f, ioBufferSize))
}
