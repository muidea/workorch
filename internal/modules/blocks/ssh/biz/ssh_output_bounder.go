package ssh

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"hash"
)

type limitedOutputBuffer struct {
	limit int
	total int
	buf   bytes.Buffer
	hash  hash.Hash
}

func newLimitedOutputBuffer(limit int) *limitedOutputBuffer {
	if limit <= 0 {
		limit = MaxCommandOutputBytes
	}
	return &limitedOutputBuffer{limit: limit, hash: sha256.New()}
}

func (b *limitedOutputBuffer) Write(p []byte) (int, error) {
	if b == nil {
		return len(p), nil
	}
	written := len(p)
	b.total += len(p)
	if b.hash != nil {
		_, _ = b.hash.Write(p)
	}
	if b.buf.Len() >= b.limit {
		return written, nil
	}
	remaining := b.limit - b.buf.Len()
	if len(p) > remaining {
		p = p[:remaining]
	}
	_, _ = b.buf.Write(p)
	return written, nil
}

func (b *limitedOutputBuffer) String() string {
	if b == nil {
		return ""
	}
	return b.buf.String()
}

func (b *limitedOutputBuffer) Truncated() bool {
	return b != nil && b.total > b.limit
}

func (b *limitedOutputBuffer) TotalBytes() int {
	if b == nil {
		return 0
	}
	return b.total
}

func (b *limitedOutputBuffer) ContentHash() string {
	if b == nil || b.hash == nil {
		return hashSSHBytes(nil)
	}
	return hex.EncodeToString(b.hash.Sum(nil))
}
