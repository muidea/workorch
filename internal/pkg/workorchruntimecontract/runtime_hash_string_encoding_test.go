package workorchruntimecontract

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"strings"
	"testing"
)

func runtimeWriteHashStringReference(w hashStringWriter, value string) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	_, _ = w.Write(size[:])
	_, _ = w.Write([]byte(value))
}

func TestRuntimeHashStringEncodingPreservesExactLengthPrefixedBytes(t *testing.T) {
	for _, value := range []string{"", "identity", " \t原判据\r\n ", "A\x00B", string([]byte{0xff, 0x80, 0}), strings.Repeat("原", 32768)} {
		var got, want bytes.Buffer
		writeHashString(&got, value)
		runtimeWriteHashStringReference(&want, value)
		if !bytes.Equal(got.Bytes(), want.Bytes()) || len(got.Bytes()) != 8+len(value) || binary.BigEndian.Uint64(got.Bytes()[:8]) != uint64(len(value)) {
			t.Fatal("长度前缀、非 UTF-8 或原文字节发生漂移")
		}
		actual, reference := sha256.New(), sha256.New()
		for range 2 {
			writeHashString(actual, value)
			runtimeWriteHashStringReference(reference, value)
		}
		if !bytes.Equal(actual.Sum(nil), reference.Sum(nil)) {
			t.Fatal("重复 hash 编码改变既有 identity")
		}
	}
	for _, values := range [][]string{nil, {}, {"A", "B"}, {"AB", ""}, {" 原文\n ", "声明"}} {
		var got, want bytes.Buffer
		writeHashStrings(&got, values)
		var count [8]byte
		binary.BigEndian.PutUint64(count[:], uint64(len(values)))
		want.Write(count[:])
		for _, value := range values {
			runtimeWriteHashStringReference(&want, value)
		}
		if !bytes.Equal(got.Bytes(), want.Bytes()) {
			t.Fatal("集合 cardinality 或成员编码改变")
		}
	}
}

func FuzzRuntimeHashStringEncoding(f *testing.F) {
	for _, value := range []string{"", "原判据\r\n", "A\x00B", string([]byte{0xff, 0x80})} {
		f.Add(value)
	}
	f.Fuzz(func(t *testing.T, value string) {
		var got, want bytes.Buffer
		writeHashString(&got, value)
		runtimeWriteHashStringReference(&want, value)
		if !bytes.Equal(got.Bytes(), want.Bytes()) {
			t.Fatal("原编码不等价")
		}
	})
}

func BenchmarkRuntimeHashStringEncoding(b *testing.B) {
	for _, test := range []struct {
		name  string
		write func(hashStringWriter, string)
	}{{"reference", runtimeWriteHashStringReference}, {"single_write", writeHashString}} {
		b.Run(test.name, func(b *testing.B) {
			h := sha256.New()
			b.ReportAllocs()
			for b.Loop() {
				test.write(h, "workorch-source-obligation-native-check-identity")
			}
		})
	}
}
