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
	buffer [128]byte
}

func newRuntimeHashWriter() *runtimeHashWriter {
	return &runtimeHashWriter{Hash: sha256.New()}
}

func (w *runtimeHashWriter) writeString(value string) {
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
