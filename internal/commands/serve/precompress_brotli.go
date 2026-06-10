package serve

// Brotli encoding for the static precompression table — deliberately isolated in
// its own file behind the encoder seam (see precompress.go). Brotli buys ~15-20%
// over gzip on text and its slow max-quality encode is free here: it runs once at
// startup, never per request. If the andybalholm/brotli dependency ever needs to
// go, deleting this file and the brotli entry in precompressEncoders() removes it
// cleanly — the negotiation/serving path falls back to gzip+identity untouched.

import (
	"bytes"

	"github.com/andybalholm/brotli"
)

// brotliEncode compresses b at maximum quality. ok is false when the result is
// not smaller than the input (tiny files), so the caller skips storing a variant
// that would only cost bytes to serve.
func brotliEncode(b []byte) (compressed []byte, ok bool) {
	var buf bytes.Buffer
	w := brotli.NewWriterLevel(&buf, brotli.BestCompression) // quality 11
	if _, err := w.Write(b); err != nil {
		return nil, false
	}
	if err := w.Close(); err != nil {
		return nil, false
	}
	if buf.Len() >= len(b) {
		return nil, false
	}
	return buf.Bytes(), true
}
