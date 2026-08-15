package web

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

func boundedInt(value int, fallback int, max int) int {
	if value <= 0 {
		value = fallback
	}
	if max > 0 && value > max {
		value = max
	}
	return value
}

func boundedInt64(value int64, fallback int64, max int64) int64 {
	if value <= 0 {
		value = fallback
	}
	if max > 0 && value > max {
		value = max
	}
	return value
}

func minPositive(value int, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func minPositive64(value int64, fallback int64) int64 {
	if value > 0 {
		return value
	}
	return fallback
}

func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	ret := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			ret = append(ret, part)
		}
	}
	return ret
}

func boolValue(raw string) bool {
	return strings.EqualFold(strings.TrimSpace(raw), "true")
}

func stableHash(parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		_, _ = hash.Write([]byte(part))
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func uniqueStrings(input []string) []string {
	seen := map[string]struct{}{}
	ret := make([]string, 0, len(input))
	for _, item := range input {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		ret = append(ret, item)
	}
	return ret
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func cloneStringMap(input map[string]string) map[string]string {
	ret := map[string]string{}
	for key, value := range input {
		ret[key] = value
	}
	return ret
}
