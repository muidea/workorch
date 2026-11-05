package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/binary"
	"hash"
)

// Scratch belongs to one encoding only. Small length-prefixed fields share a
// digest write; every original byte is still written in the original order.
// No source object, digest or validation result is reused across hash calls.
type runtimeHashWriter struct {
	digest     hash.Hash
	buffer     [128]byte
	used       int
	longBuffer *[4096]byte
}

func newRuntimeHashWriter() *runtimeHashWriter { return &runtimeHashWriter{digest: sha256.New()} }

func (w *runtimeHashWriter) flush() {
	if w.used != 0 {
		_, _ = w.digest.Write(w.buffer[:w.used])
		w.used = 0
	}
}

func (w *runtimeHashWriter) Write(value []byte) (int, error) {
	n := len(value)
	if w.used != 0 {
		copied := copy(w.buffer[w.used:], value)
		w.used += copied
		value = value[copied:]
		if w.used == len(w.buffer) {
			w.flush()
		}
	}
	if len(value) >= len(w.buffer) {
		_, _ = w.digest.Write(value)
	} else {
		w.used += copy(w.buffer[w.used:], value)
	}
	return n, nil
}

// Flushing does not consume the digest. Repeated Sum and subsequent Write
// retain hash.Hash semantics, including a caller-supplied destination prefix.
func (w *runtimeHashWriter) Sum(dst []byte) []byte { w.flush(); return w.digest.Sum(dst) }
func (w *runtimeHashWriter) Reset()                { w.used = 0; w.digest.Reset() }
func (w *runtimeHashWriter) Size() int             { return w.digest.Size() }
func (w *runtimeHashWriter) BlockSize() int        { return w.digest.BlockSize() }

func (w *runtimeHashWriter) writeString(value string) {
	w.writeCount(uint64(len(value)))
	if len(value) >= 8192 {
		w.flush()
		if w.longBuffer == nil {
			w.longBuffer = new([4096]byte)
		}
		for len(value) != 0 {
			n := copy(w.longBuffer[:], value)
			_, _ = w.digest.Write(w.longBuffer[:n])
			value = value[n:]
		}
		return
	}
	for len(value) != 0 {
		n := copy(w.buffer[w.used:], value)
		w.used += n
		value = value[n:]
		if w.used == len(w.buffer) {
			w.flush()
		}
	}
}

func (w *runtimeHashWriter) writeCount(count uint64) {
	if len(w.buffer)-w.used < 8 {
		w.flush()
	}
	binary.BigEndian.PutUint64(w.buffer[w.used:w.used+8], count)
	w.used += 8
}
