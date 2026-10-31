package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"github.com/muidea/workorch/internal/pkg/common"
	"slices"
	"time"
)

// Errors remain original bytes, including invalid UTF-8. JSON encodes []byte
// reversibly. This restricted owner value is never returned in the public DTO.
const PreparationObservationErrorLimit = 16 * 1024
const PreparationObservationLimit = 24

type PreparationObservationDiagnostic struct {
	Phase                  string
	Owner                  string
	Stage                  string
	StartedAt, CompletedAt time.Time
	Deadline               time.Time
	Attempted              bool
	Code                   string
	ErrorBytes             []byte `json:",omitempty"`
	ErrorByteCount         int
	ErrorSHA256            string `json:",omitempty"`
	NotSavedReason         string `json:",omitempty"`
}

func ClonePreparationObservations(items []PreparationObservationDiagnostic) []PreparationObservationDiagnostic {
	out := slices.Clone(items)
	for i := range out {
		out[i].ErrorBytes = slices.Clone(out[i].ErrorBytes)
	}
	return out
}

func (d PreparationObservationDiagnostic) Validate() error {
	if d.Phase != "initial" && d.Phase != "final" || !boundedPreparationIdentity(d.Owner) || !boundedPreparationIdentity(d.Stage) || d.StartedAt.IsZero() || d.CompletedAt.Before(d.StartedAt) {
		return common.ErrConflict
	}
	switch d.Code {
	case "observed", "deadline_exceeded", "cancelled", "owner_rejected", "invalid_result", "source_drift", "incomplete", "budget_exhausted_before_call":
	default:
		return common.ErrConflict
	}
	if d.Code == "budget_exhausted_before_call" && (d.Attempted || d.Deadline.IsZero() || d.StartedAt.Before(d.Deadline)) || d.Code == "observed" && (!d.Attempted || d.ErrorByteCount != 0 || d.ErrorSHA256 != "") {
		return common.ErrConflict
	}
	if d.ErrorByteCount == 0 {
		if len(d.ErrorBytes) != 0 || d.ErrorSHA256 != "" && d.ErrorSHA256 != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" || d.NotSavedReason != "" {
			return common.ErrConflict
		}
		return nil
	}
	if d.ErrorByteCount < 0 || len(d.ErrorSHA256) != 64 {
		return common.ErrConflict
	}
	for _, c := range d.ErrorSHA256 {
		if c < '0' || c > '9' && c < 'a' || c > 'f' {
			return common.ErrConflict
		}
	}
	if d.ErrorByteCount > PreparationObservationErrorLimit {
		if len(d.ErrorBytes) != 0 || d.NotSavedReason != "error_exceeds_storage_limit" {
			return common.ErrConflict
		}
	} else {
		digest := sha256.Sum256(d.ErrorBytes)
		if len(d.ErrorBytes) != d.ErrorByteCount || d.NotSavedReason != "" || hex.EncodeToString(digest[:]) != d.ErrorSHA256 {
			return common.ErrConflict
		}
	}
	return nil
}

func ValidatePreparationObservations(items []PreparationObservationDiagnostic, start, end time.Time) error {
	if len(items) > PreparationObservationLimit {
		return common.ErrConflict
	}
	seen := map[string]bool{}
	for _, d := range items {
		key := d.Phase + "/" + d.Owner
		if seen[key] || d.Validate() != nil || !start.IsZero() && d.StartedAt.Before(start) || !end.IsZero() && d.CompletedAt.After(end) {
			return common.ErrConflict
		}
		seen[key] = true
	}
	return nil
}

// Negative diagnostic only. A completion still needs independent recompilation
// of all original evidence; this value cannot authorize execution or readiness.
type PreparationFinalization struct{ Code, Owner, Stage string }

func (f PreparationFinalization) Validate() error {
	switch f.Code {
	case "evidence_invalid", "original_source_rejected", "bootstrap_rejected", "checks_incomplete", "owner_observation_incomplete", "owner_observation_changed", "owner_observation_expired", "file_epoch_unavailable", "file_observation_changed_or_expired":
	default:
		return common.ErrConflict
	}
	if !boundedPreparationIdentity(f.Owner) || !boundedPreparationIdentity(f.Stage) {
		return common.ErrConflict
	}
	return nil
}
