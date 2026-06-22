package runtimepath

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const DefaultMaxSegmentLen = 160

// SafePath returns a readable, filesystem-safe relative path while preserving
// the structure expressed by "/" or "\" in the source identifier.
func SafePath(value string) string {
	parts := SafeSegments(value)
	if len(parts) == 0 {
		return "empty"
	}
	return filepath.Join(parts...)
}

func SafePathWithMax(value string, maxSegmentLen int) string {
	parts := SafeSegmentsWithMax(value, maxSegmentLen)
	if len(parts) == 0 {
		return "empty"
	}
	return filepath.Join(parts...)
}

func SafeSegments(value string) []string {
	return safeSegments(value, 0)
}

func SafeSegmentsWithMax(value string, maxSegmentLen int) []string {
	return safeSegments(value, maxSegmentLen)
}

func safeSegments(value string, maxSegmentLen int) []string {
	parts := strings.FieldsFunc(strings.TrimSpace(value), func(r rune) bool {
		return r == '/' || r == '\\'
	})
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		name := SafeSegment(part)
		if maxSegmentLen > 0 {
			name = TruncateSafeSegment(name, maxSegmentLen)
		}
		if name != "" {
			cleaned = append(cleaned, name)
		}
	}
	return cleaned
}

func SafeSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "." || value == ".." {
		return ""
	}
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		keep := r >= 'a' && r <= 'z' ||
			r >= 'A' && r <= 'Z' ||
			r >= '0' && r <= '9' ||
			r == '-' || r == '_' || r == '.'
		if keep {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	ret := strings.Trim(b.String(), ".-")
	if ret == "" || ret == "." || ret == ".." {
		return "item"
	}
	return ret
}

func TruncateSafeSegment(segment string, maxLen int) string {
	if maxLen <= 0 || len(segment) <= maxLen {
		return segment
	}
	sum := sha256.Sum256([]byte(segment))
	hash := fmt.Sprintf("%x", sum[:])[:16]
	if maxLen <= len(hash)+1 {
		return hash[:maxLen]
	}
	prefixLen := maxLen - len(hash) - 1
	if prefixLen < 16 {
		prefixLen = 16
	}
	if prefixLen+len(hash)+1 > maxLen {
		prefixLen = maxLen - len(hash) - 1
	}
	if prefixLen >= len(segment) {
		return segment
	}
	prefix := strings.TrimRight(segment[:prefixLen], ".-")
	if prefix == "" {
		prefix = segment[:prefixLen]
	}
	return prefix + "-" + hash
}

// SequenceScopedPath returns "<scope>/<NNN-label>" under scopeRoot.
//
// The returned path is relative to scopeRoot and uses an existing matching
// directory when present, so repeated calls for the same item keep stable paths.
// The sequence is derived by scanning scopeRoot/scopeID, allowing service
// restarts to continue from the current max sequence.
func SequenceScopedPath(scopeRoot string, scopeID string, itemID string) string {
	scopeSafe := TruncateSafeSegment(SafeSegment(scopeID), DefaultMaxSegmentLen)
	if scopeSafe == "" {
		scopeSafe = "scope"
	}
	label := sequenceLabel(scopeSafe, itemID)
	scopeDir := filepath.Join(scopeRoot, scopeSafe)
	itemDir := sequencePrefixedDir(scopeDir, label)
	return filepath.ToSlash(filepath.Join(scopeSafe, itemDir))
}

// SequenceScopedRoot returns the filesystem directory for a relative
// SequenceScopedPath result.
func SequenceScopedRoot(scopeRoot string, relativePath string) string {
	return filepath.Join(scopeRoot, filepath.FromSlash(strings.Trim(strings.TrimSpace(relativePath), "/")))
}

func sequenceLabel(scopeSafe string, itemID string) string {
	parts := SafeSegmentsWithMax(itemID, DefaultMaxSegmentLen)
	if len(parts) > 0 && parts[0] == scopeSafe {
		parts = parts[1:]
	}
	if len(parts) == 0 {
		return "empty"
	}
	return TruncateSafeSegment(SafeSegment(strings.Join(parts, "-")), DefaultMaxSegmentLen-len("000-"))
}

func sequencePrefixedDir(scopeDir string, label string) string {
	label = TruncateSafeSegment(SafeSegment(label), DefaultMaxSegmentLen-len("000-"))
	if label == "" {
		label = "empty"
	}
	if existing := existingSequencePrefixedDir(scopeDir, label); existing != "" {
		return existing
	}
	return fmt.Sprintf("%03d-%s", nextSequence(scopeDir), label)
}

func existingSequencePrefixedDir(scopeDir string, label string) string {
	entries, err := os.ReadDir(scopeDir)
	if err != nil {
		return ""
	}
	suffix := "-" + label
	best := ""
	bestSeq := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		seq, ok := SequenceFromDirName(name)
		if !ok || !strings.HasSuffix(name, suffix) {
			continue
		}
		if best == "" || seq < bestSeq {
			best = name
			bestSeq = seq
		}
	}
	return best
}

func nextSequence(scopeDir string) int {
	entries, err := os.ReadDir(scopeDir)
	if err != nil {
		return 1
	}
	maxSeq := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		seq, ok := SequenceFromDirName(entry.Name())
		if ok && seq > maxSeq {
			maxSeq = seq
		}
	}
	return maxSeq + 1
}

func SequenceFromDirName(name string) (int, bool) {
	if len(name) < 5 || name[3] != '-' {
		return 0, false
	}
	for idx := 0; idx < 3; idx++ {
		if name[idx] < '0' || name[idx] > '9' {
			return 0, false
		}
	}
	seq, err := strconv.Atoi(name[:3])
	return seq, err == nil
}
