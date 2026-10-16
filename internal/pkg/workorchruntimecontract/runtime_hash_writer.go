package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/binary"
	"hash"
)

// 缓冲仅属于一次 hash 编码；不持有业务对象，不跨调用复用校验结果。
// 超出短字段边界仍完整写入原始 bytes，不截断或改写正文。
type runtimeHashWriter struct {
	hash.Hash
	buffer     [128]byte
	longBuffer *[4096]byte
}

func newRuntimeHashWriter() *runtimeHashWriter {
	return &runtimeHashWriter{Hash: sha256.New()}
}

func (w *runtimeHashWriter) writeString(value string) {
	if len(value) >= 8192 {
		// Complete length-prefixed bytes, streamed through call-local scratch.
		// Keep the ordinary short/medium encoding path unchanged; no raw value
		// or validation product is cached or shared across hash calls.
		if w.longBuffer == nil {
			w.longBuffer = new([4096]byte)
		}
		binary.BigEndian.PutUint64(w.longBuffer[:8], uint64(len(value)))
		first := copy(w.longBuffer[8:], value)
		_, _ = w.Write(w.longBuffer[:8+first])
		value = value[first:]
		for len(value) != 0 {
			n := copy(w.longBuffer[:], value)
			_, _ = w.Write(w.longBuffer[:n])
			value = value[n:]
		}
		return
	}
	if len(value) > len(w.buffer)-8 {
		encoded := make([]byte, 8+len(value))
		binary.BigEndian.PutUint64(encoded[:8], uint64(len(value)))
		copy(encoded[8:], value)
		_, _ = w.Write(encoded)
		return
	}
	encoded := w.buffer[:8+len(value)]
	binary.BigEndian.PutUint64(encoded[:8], uint64(len(value)))
	copy(encoded[8:], value)
	_, _ = w.Write(encoded)
}

func (w *runtimeHashWriter) writeCount(count uint64) {
	binary.BigEndian.PutUint64(w.buffer[:8], count)
	_, _ = w.Write(w.buffer[:8])
}
