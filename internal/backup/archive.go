package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"

	"github.com/klauspost/compress/zstd"
)

type ArchiveResult struct {
	RawBytes        int64
	CompressedBytes int64
	Checksum        string
}

type ArchiveWriter struct {
	zw       *zstd.Encoder
	hasher   hash.Hash
	rawCount *byteCounter
	cmpCount *byteCounter
	rawIn    io.Writer
}

func NewArchiveWriter(dst io.Writer) (*ArchiveWriter, error) {
	cmpCount := &byteCounter{}
	zw, err := zstd.NewWriter(io.MultiWriter(dst, cmpCount), zstd.WithEncoderLevel(zstd.SpeedDefault))
	if err != nil {
		return nil, fmt.Errorf("creating zstd writer: %w", err)
	}
	hasher := sha256.New()
	rawCount := &byteCounter{}
	return &ArchiveWriter{
		zw:       zw,
		hasher:   hasher,
		rawCount: rawCount,
		cmpCount: cmpCount,
		rawIn:    io.MultiWriter(zw, hasher, rawCount),
	}, nil
}

func (w *ArchiveWriter) Write(p []byte) (int, error) { return w.rawIn.Write(p) }

func (w *ArchiveWriter) Close() (*ArchiveResult, error) {
	if err := w.zw.Close(); err != nil {
		return nil, fmt.Errorf("closing zstd writer: %w", err)
	}
	return &ArchiveResult{
		RawBytes:        w.rawCount.n,
		CompressedBytes: w.cmpCount.n,
		Checksum:        hex.EncodeToString(w.hasher.Sum(nil)),
	}, nil
}

type ArchiveReader struct {
	zr *zstd.Decoder
}

func NewArchiveReader(src io.Reader) (*ArchiveReader, error) {
	zr, err := zstd.NewReader(src)
	if err != nil {
		return nil, fmt.Errorf("creating zstd reader: %w", err)
	}
	return &ArchiveReader{zr: zr}, nil
}

func (r *ArchiveReader) Read(p []byte) (int, error) { return r.zr.Read(p) }

func (r *ArchiveReader) Close() error {
	r.zr.Close()
	return nil
}

type byteCounter struct{ n int64 }

func (c *byteCounter) Write(p []byte) (int, error) {
	c.n += int64(len(p))
	return len(p), nil
}
