package workspace

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"reflect"
	"time"
)

const PreparationCompletionVersion = "workorch-preparation-completion/v1"

// A historical all-of + bootstrap + original no-Run transaction. It never
// grants execution. Currentness is projected independently and can expire.
type WorkspacePreparationCompletion struct {
	Version, CompletionID, RequestID, PayloadHash string
	CreationEpoch                                 string
	PreparationEpoch                              uint64
	Binding                                       WorkspacePreparationBinding
	BootstrapHash                                 string
	CompletedAt, ExpiresAt                        time.Time
	CommittedWorkspaceRevision                    uint64
}

func (c WorkspacePreparationCompletion) ValidateFor(a WorkspacePreparationAttempt) error {
	if c.Version != PreparationCompletionVersion || !boundedPreparationIdentity(c.CompletionID) || !boundedPreparationIdentity(c.BootstrapHash) || c.RequestID != a.RequestID || c.PayloadHash != a.PayloadHash || c.Binding != a.Binding || c.CreationEpoch != a.CreationEpoch || c.PreparationEpoch != 0 || a.PreparationEpoch != 0 || c.CommittedWorkspaceRevision != a.CommittedWorkspaceRevision || !c.CompletedAt.Equal(a.CompletedAt) || c.ExpiresAt.Sub(c.CompletedAt) != 30*time.Second || a.State != WorkspacePreparationBlocked || a.NoRunEvaluationVersion != PreparationNoRunFinalVersion || a.Assessment == nil || a.Assessment.Outcome != "checks_passed" || a.DefinitionCompilation == nil || a.OwnerRevalidationVersion != PreparationOwnerRevalidationVersion {
		return common.ErrConflict
	}
	return nil
}
func validatePreparationCompletions(r WorkspacePreparationRecord) error {
	if len(r.Completions) > len(r.Attempts) || r.Version != WorkspacePreparationWorkflowVersion && len(r.Completions) != 0 {
		return common.ErrConflict
	}
	seen := map[string]bool{}
	last := uint64(0)
	for _, c := range r.Completions {
		found := false
		for _, a := range r.Attempts {
			if a.RequestID == c.RequestID {
				found = c.ValidateFor(a) == nil
			}
		}
		if !found || seen[c.RequestID] || c.CommittedWorkspaceRevision <= last {
			return common.ErrConflict
		}
		seen[c.RequestID] = true
		last = c.CommittedWorkspaceRevision
	}
	return nil
}
func validatePreparationCompletionTransition(current, next *Workspace, cas bool) error {
	var before, after []WorkspacePreparationCompletion
	if current != nil && current.StartupPreparation != nil {
		before = current.StartupPreparation.Completions
	}
	if next != nil && next.StartupPreparation != nil {
		after = next.StartupPreparation.Completions
	}
	if reflect.DeepEqual(before, after) {
		return nil
	}
	if current == nil || !cas || len(after) != len(before)+1 || len(before) > 0 && !reflect.DeepEqual(before, after[:len(before)]) || current.StartupPreparation == nil || next.StartupPreparation == nil {
		return common.ErrConflict
	}
	a := current.StartupPreparation.Attempts[len(current.StartupPreparation.Attempts)-1]
	b := next.StartupPreparation.Attempts[len(next.StartupPreparation.Attempts)-1]
	c := after[len(after)-1]
	if a.State != WorkspacePreparationAccepted || b.RequestID != a.RequestID || c.ValidateFor(b) != nil || current.Revision != a.ExpectedWorkspaceRevision+1 || c.CommittedWorkspaceRevision != current.Revision+1 || next.ActiveRunID != "" || !reflect.DeepEqual(current.PreparationFence, next.PreparationFence) {
		return common.ErrConflict
	}
	return nil
}
