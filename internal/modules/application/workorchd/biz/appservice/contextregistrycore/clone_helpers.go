package contextregistrycore

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

func GenerateContextID(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err == nil {
		if prefix == "" {
			return hex.EncodeToString(raw[:])
		}
		return prefix + "-" + hex.EncodeToString(raw[:])
	}
	if prefix == "" {
		return fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	}
	return fmt.Sprintf("%s-%d", prefix, time.Now().UTC().UnixNano())
}

func CloneStrings(input []string) []string {
	if input == nil {
		return nil
	}
	return append([]string(nil), input...)
}

func RedactStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		if value == "" {
			out[key] = ""
			continue
		}
		out[key] = "redacted"
	}
	return out
}

func CloneMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func CloneFloats(input []float64) []float64 {
	if input == nil {
		return nil
	}
	return append([]float64(nil), input...)
}

func CloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	ret := make(map[string]string, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func cloneStringMap(input map[string]string) map[string]string {
	return CloneStringMap(input)
}
