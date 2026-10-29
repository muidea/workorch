package workspace

import (
	"reflect"

	"github.com/muidea/workorch/internal/pkg/common"
)

// Persistence backstop for v2 only. v1 trace fixtures remain readable; they
// cannot be upgraded into a production operation. All v2 mutations require
// CAS, immutable original identities and append-only completed history.
func ValidatePreparationRecordTransition(current, next *Workspace, cas bool) error {
	if err := validatePreparationCompletionTransition(current, next, cas); err != nil {
		return err
	}
	if err := validatePreparationInitializationTransition(current, next, cas); err != nil {
		return err
	}
	if next == nil {
		return common.ErrInvalidArgument
	}
	if err := validatePreparationEvidenceTransition(current, next, cas); err != nil {
		return err
	}
	var before *WorkspacePreparationRecord
	if current != nil {
		before = current.StartupPreparation
	}
	after := next.StartupPreparation
	if (before == nil || before.Version != WorkspacePreparationWorkflowVersion) && (after == nil || after.Version != WorkspacePreparationWorkflowVersion) {
		return nil
	}
	if current == nil || current.ID != next.ID || after == nil || after.Version != WorkspacePreparationWorkflowVersion || after.Validate(next.ID) != nil || before != nil && before.Validate(current.ID) != nil {
		return common.ErrConflict
	}
	if reflect.DeepEqual(before, after) {
		return nil
	}
	if !cas || !reflect.DeepEqual(current.PreparationFence, next.PreparationFence) || current.PreparationFence == nil || current.PreparationFence.Validate(current.ID) != nil || current.Revision == ^uint64(0) {
		return common.ErrConflict
	}
	length := 0
	if before != nil {
		if before.Version != WorkspacePreparationWorkflowVersion {
			return common.ErrConflict
		}
		length = len(before.Attempts)
	}
	if len(after.Attempts) == length+1 {
		if length > 0 {
			last := before.Attempts[length-1]
			if (last.State != WorkspacePreparationBlocked && last.State != WorkspacePreparationFailed) || !reflect.DeepEqual(before.Attempts, after.Attempts[:length]) {
				return common.ErrConflict
			}
		}
		a := after.Attempts[length]
		if a.State == WorkspacePreparationAccepted && a.ExpectedWorkspaceRevision == current.Revision && a.CommittedWorkspaceRevision == current.Revision+1 && a.CreationEpoch == current.PreparationFence.CreationEpoch && a.PreparationEpoch == current.PreparationFence.PreparationEpoch {
			return nil
		}
		return common.ErrConflict
	}
	if length == 0 || len(after.Attempts) != length || !reflect.DeepEqual(before.Attempts[:length-1], after.Attempts[:length-1]) {
		return common.ErrConflict
	}
	a, b := before.Attempts[length-1], after.Attempts[length-1]
	if (a.State != WorkspacePreparationAccepted && a.State != WorkspacePreparationUnknown) || b.State != WorkspacePreparationBlocked || b.CommittedWorkspaceRevision != current.Revision+1 || len(a.Checks) != len(b.Checks) {
		return common.ErrConflict
	}
	// Compare the entire original value after removing only the explicitly
	// permitted transition fields. No hidden replacement of binding/epochs.
	b.State, b.CompletedAt, b.CommittedWorkspaceRevision = a.State, a.CompletedAt, a.CommittedWorkspaceRevision
	b.DirectoryEvidenceHash = a.DirectoryEvidenceHash
	b.ResourceEvidenceHash = a.ResourceEvidenceHash
	b.ManagedConfigEvidenceRef = a.ManagedConfigEvidenceRef
	b.SoulEvidenceRef = a.SoulEvidenceRef
	b.SkillRegistryEvidenceRef = a.SkillRegistryEvidenceRef
	b.SkillHubEffectState = a.SkillHubEffectState
	b.SkillHubInstalledState = a.SkillHubInstalledState
	b.SkillSelectionState = a.SkillSelectionState
	b.SkillContextState = a.SkillContextState
	b.SkillReferenceState = a.SkillReferenceState
	b.SkillStatusState = a.SkillStatusState
	b.ProviderCacheEvidenceRef = a.ProviderCacheEvidenceRef
	b.RunHistoryEvidenceRef = a.RunHistoryEvidenceRef
	b.NoRunEvaluationVersion = a.NoRunEvaluationVersion
	b.MemoryInventoryEvidenceRef = a.MemoryInventoryEvidenceRef
	b.MCPRegistryEvidenceRef = a.MCPRegistryEvidenceRef
	b.SealEvidenceRef = a.SealEvidenceRef
	b.Assessment = a.Assessment
	b.DefinitionCompilation = a.DefinitionCompilation
	b.ExecutionInputVersion = a.ExecutionInputVersion
	b.EmptySelectionEvaluationVersion = a.EmptySelectionEvaluationVersion
	b.Checks = append([]WorkspacePreparationCheckResult(nil), b.Checks...)
	for i := range b.Checks {
		b.Checks[i].State, b.Checks[i].Reason = a.Checks[i].State, a.Checks[i].Reason
	}
	if !reflect.DeepEqual(a, b) {
		return common.ErrConflict
	}
	return nil
}
