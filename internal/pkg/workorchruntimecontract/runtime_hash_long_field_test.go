package workorchruntimecontract

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"hash"
	"strings"
	"testing"
)

// Frozen prior call-local writer, including one complete allocation for every
// field exceeding its short buffer. The bytes, not write boundaries, are identity.
type referenceLongFieldWriter struct {
	hash.Hash
	buffer [128]byte
}

func (w *referenceLongFieldWriter) writeString(value string) {
	if len(value) > len(w.buffer)-8 {
		encoded := make([]byte, 8+len(value))
		binary.BigEndian.PutUint64(encoded[:8], uint64(len(value)))
		copy(encoded[8:], value)
		w.Write(encoded)
		return
	}
	encoded := w.buffer[:8+len(value)]
	binary.BigEndian.PutUint64(encoded[:8], uint64(len(value)))
	copy(encoded[8:], value)
	w.Write(encoded)
}

func assertLongFieldHash(t testing.TB, values []string) {
	t.Helper()
	got := newRuntimeHashWriter()
	want := &referenceLongFieldWriter{Hash: sha256.New()}
	for range 2 {
		for _, value := range values {
			got.writeString(value)
			want.writeString(value)
			if !bytes.Equal(got.Sum(nil), want.Sum(nil)) {
				t.Fatal("chunked hash changed exact length-prefixed bytes")
			}
		}
		got.Write([]byte("raw\x00"))
		want.Write([]byte("raw\x00"))
	}
	got.Reset()
	want.Reset()
	for _, value := range values {
		got.writeString(value)
		want.writeString(value)
	}
	if !bytes.Equal(got.Sum(nil), want.Sum(nil)) {
		t.Fatal("chunk scratch retained prior hash contents across reset")
	}
}

func TestRuntimeHashLongFieldCompleteEncoding(t *testing.T) {
	for _, size := range []int{120, 121, 4095, 4096, 4097, 8191, 8192, 8193, 12279, 12280, 12281, 16384, 98304} {
		value := strings.Repeat("原\x00\xff\x80\n", size/7+1)[:size]
		assertLongFieldHash(t, []string{"", value, "short", value + "tail", ""})
	}
}

func FuzzRuntimeHashLongFieldCompleteEncoding(f *testing.F) {
	for _, size := range []uint16{0, 120, 121, 8191, 8192, 8193, 12280, 16384} {
		f.Add("原\x00\xff\r\n", size)
	}
	f.Fuzz(func(t *testing.T, raw string, size uint16) {
		if len(raw) > 256 {
			t.Skip()
		}
		if raw == "" {
			raw = "\x00"
		}
		long := strings.Repeat(raw, int(size)/len(raw)+1)[:int(size)]
		assertLongFieldHash(t, []string{raw, long, "short", long})
	})
}

func BenchmarkRuntimeHashLongFields(b *testing.B) {
	value := strings.Repeat("原始长文字\x00\n", 16384)
	for _, chunked := range []bool{false, true} {
		name := "complete-allocation"
		if chunked {
			name = "call-local-chunks"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if chunked {
					w := newRuntimeHashWriter()
					for range 4 {
						w.writeString(value)
					}
					w.Sum(nil)
				} else {
					w := &referenceLongFieldWriter{Hash: sha256.New()}
					for range 4 {
						w.writeString(value)
					}
					w.Sum(nil)
				}
			}
		})
	}
}
