package events

import (
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/startuppreparation"
	"github.com/muidea/workorch/internal/pkg/common"
)

const TopicWorkspaceExactConfirmedStartupQuery = "/workspace/activation-preparation/exact-startup/query"
const TopicWorkspacePreparationStateQuery = "/workspace/activation-preparation/state/query"

// ExpectedWorkspaceRevision is mandatory: a stale caller must explicitly read
// the new owner revision, not silently substitute another sealed input.
type QueryExactConfirmedStartupSnapshotCommand struct {
	WorkspaceID               common.WorkspaceID
	ExpectedWorkspaceRevision uint64
}

func (c QueryExactConfirmedStartupSnapshotCommand) Validate() error {
	if c.WorkspaceID == "" || c.ExpectedWorkspaceRevision == 0 {
		return common.ErrInvalidArgument
	}
	return nil
}

type QueryExactConfirmedStartupSnapshotResult struct {
	Snapshot startuppreparation.ExactConfirmedStartupSnapshot
}

func (r QueryExactConfirmedStartupSnapshotResult) ValidateFor(c QueryExactConfirmedStartupSnapshotCommand) error {
	if c.Validate() != nil || r.Snapshot.WorkspaceID != c.WorkspaceID || r.Snapshot.WorkspaceRevision != c.ExpectedWorkspaceRevision {
		return common.ErrConflict
	}
	return r.Snapshot.Validate()
}

type QueryWorkspacePreparationStateCommand struct {
	WorkspaceID common.WorkspaceID
}

// This is only a persisted-operation projection. No current no-Run or
// external-resource claim is inferred from ActiveRunID or absence of records.
type QueryWorkspacePreparationStateResult struct {
	WorkspaceID             common.WorkspaceID
	WorkspaceRevision       uint64
	ActiveRunID             common.RunID
	State                   string
	CurrentValidityReason   string
	CurrentValidity         string
	ResourceInitializations []PreparationInitializationView
	Record                  *workspace.WorkspacePreparationRecord
	SkillHubProjectIssue    string
}

func (r QueryWorkspacePreparationStateResult) ValidateFor(c QueryWorkspacePreparationStateCommand) error {
	if !validPreparationValidityReason(r.CurrentValidity, r.CurrentValidityReason) {
		return common.ErrConflict
	}
	if c.WorkspaceID == "" || r.WorkspaceID != c.WorkspaceID || r.WorkspaceRevision == 0 {
		return common.ErrConflict
	}
	if len(r.ResourceInitializations) > workspace.PreparationInitializationLimit {
		return common.ErrConflict
	}
	seen := map[string]bool{}
	for _, v := range r.ResourceInitializations {
		if v.Validate(r.WorkspaceRevision) != nil || seen[v.RequestID] {
			return common.ErrConflict
		}
		seen[v.RequestID] = true
	}
	if r.Record == nil {
		if r.State != "not_checked" || r.CurrentValidity != "not_checked" || r.SkillHubProjectIssue != "" {
			return common.ErrConflict
		}
		return nil
	}
	if r.Record.Validate(c.WorkspaceID) != nil {
		return common.ErrConflict
	}
	last := r.Record.Attempts[len(r.Record.Attempts)-1]
	if r.SkillHubProjectIssue != "" && (r.SkillHubProjectIssue != "project_registration_required" || last.State != workspace.WorkspacePreparationBlocked || last.SkillHubEffectState != workspace.PreparationSkillHubUnresolved) {
		return common.ErrConflict
	}
	if last.CommittedWorkspaceRevision > r.WorkspaceRevision {
		return common.ErrConflict
	}
	expected := string(last.State)
	for _, c := range r.Record.Completions {
		if c.RequestID == last.RequestID {
			expected = "completed"
		}
	}
	if r.State != expected || r.CurrentValidity != "not_evaluated" && r.CurrentValidity != "stale" && !(expected == "completed" && r.CurrentValidity == "observed") {
		return common.ErrConflict
	}
	return nil
}

// Reasons describe current readback only; they never modify historical checks.
func validPreparationValidityReason(validity, reason string) bool {
	switch reason {
	case "":
		return true // older read-only projections have no reason
	case "file_epoch_unavailable", "file_epoch_timeout", "file_epoch_cancelled":
		return validity == "not_evaluated"
	case "binding_changed", "attempt_outside_window", "completion_outside_window", "file_observation_stale", "provider_cache_expired", "owner_observation_stale", "file_epoch_changed_or_expired":
		return validity == "stale"
	default:
		return false
	}
}
