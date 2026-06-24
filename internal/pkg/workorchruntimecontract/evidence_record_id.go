package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

func StableEvidenceRecordID(raw string, fallback string, suffix string) common.EvidenceRecordID {
	value := strings.TrimSpace(raw)
	base := safeEvidenceRecordIDPart(value)
	if base == "" {
		base = safeEvidenceRecordIDPart(fallback)
	}
	if base == "" {
		base = "evidence"
	}
	if len(base) > 48 {
		base = strings.Trim(base[:48], "-")
	}
	sum := sha256.Sum256([]byte(value))
	digest := hex.EncodeToString(sum[:])
	if len(digest) > 12 {
		digest = digest[:12]
	}
	suffix = safeEvidenceRecordIDPart(suffix)
	if suffix == "" {
		suffix = "record"
	}
	return common.EvidenceRecordID(base + "-" + digest + "-" + suffix)
}

func safeEvidenceRecordIDPart(value string) string {
	value = strings.TrimSpace(value)
	value = strings.NewReplacer("/", "-", "\\", "-", ":", "-", " ", "-", "_", "-").Replace(value)
	value = strings.Trim(value, "-")
	for strings.Contains(value, "--") {
		value = strings.ReplaceAll(value, "--", "-")
	}
	return value
}
