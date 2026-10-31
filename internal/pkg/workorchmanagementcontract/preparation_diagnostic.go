package workorchmanagementcontract

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"strings"
	"time"
)

type PreparationFinalizationView struct {
	Code  string `json:"code"`
	Owner string `json:"owner"`
	Stage string `json:"stage"`
}
type PreparationObservationView struct {
	Phase          string     `json:"phase"`
	Owner          string     `json:"owner"`
	Stage          string     `json:"stage"`
	Code           string     `json:"code"`
	StartedAt      time.Time  `json:"startedAt"`
	CompletedAt    time.Time  `json:"completedAt"`
	Deadline       *time.Time `json:"deadline,omitempty"`
	Attempted      bool       `json:"attempted"`
	ErrorByteCount int        `json:"errorByteCount"`
	ErrorSHA256    string     `json:"errorSha256,omitempty"`
	NotSavedReason string     `json:"notSavedReason,omitempty"`
}

func validatePreparationDiagnosticViews(a PreparationAttemptView) error {
	if len(a.Observations) > 24 || (len(a.Observations) > 0 || a.Finalization != nil) && a.State != "blocked" {
		return common.ErrConflict
	}
	if f := a.Finalization; f != nil {
		if a.Assessment == nil || a.Assessment.Outcome != "checks_passed" || !preparationViewIdentity(f.Owner) || !preparationViewIdentity(f.Stage) {
			return common.ErrConflict
		}
		switch f.Code {
		case "evidence_invalid", "original_source_rejected", "bootstrap_rejected", "checks_incomplete", "owner_observation_incomplete", "owner_observation_changed", "owner_observation_expired", "file_epoch_unavailable", "file_observation_changed_or_expired":
		default:
			return common.ErrConflict
		}
	}
	seen := map[string]bool{}
	for _, d := range a.Observations {
		key := d.Phase + "/" + d.Owner
		if seen[key] || d.Phase != "initial" && d.Phase != "final" || !preparationViewIdentity(d.Owner) || !preparationViewIdentity(d.Stage) || d.StartedAt.IsZero() || d.CompletedAt.Before(d.StartedAt) || d.StartedAt.Before(a.StartedAt) || a.CompletedAt == nil || d.CompletedAt.After(*a.CompletedAt) || d.ErrorByteCount < 0 {
			return common.ErrConflict
		}
		seen[key] = true
		switch d.Code {
		case "observed", "deadline_exceeded", "cancelled", "owner_rejected", "invalid_result", "source_drift", "incomplete", "budget_exhausted_before_call":
		default:
			return common.ErrConflict
		}
		if d.Code == "observed" && (!d.Attempted || d.ErrorByteCount != 0 || d.ErrorSHA256 != "") || d.Code == "budget_exhausted_before_call" && (d.Attempted || d.Deadline == nil || d.StartedAt.Before(*d.Deadline)) {
			return common.ErrConflict
		}
		if d.ErrorByteCount == 0 && (d.ErrorSHA256 != "" && d.ErrorSHA256 != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" || d.NotSavedReason != "") || d.ErrorByteCount > 0 && (len(d.ErrorSHA256) != 64 || strings.Trim(d.ErrorSHA256, "0123456789abcdef") != "") {
			return common.ErrConflict
		}
		if d.ErrorByteCount > 16*1024 {
			if d.NotSavedReason != "error_exceeds_storage_limit" {
				return common.ErrConflict
			}
		} else if d.NotSavedReason != "" {
			return common.ErrConflict
		}
	}
	return nil
}
