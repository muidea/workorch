package workspace

import "github.com/muidea/workorch/internal/pkg/common"

const PreparationDefinitionCompilerVersion = "workorch-preparation-definition-compiler/v1"

// A bounded summary of pure sealed-source compilation, not a bootstrap,
// currentness lease, M3 or authorization. Complete inputs remain in the seal
// archive. Repository adapters independently recompile them on write/read.
type PreparationDefinitionCompilation struct {
	CompilerVersion, SnapshotHash, ManifestHash                       string
	SourceInventoryHash, ProfileContractsHash                         string
	SourceCount, RequirementCount, ObligationCount, VerificationCount int
}

func (c PreparationDefinitionCompilation) ValidateFor(a WorkspacePreparationAttempt) error {
	if a.State != WorkspacePreparationBlocked || a.EvaluationVersion != PreparationCheckEvaluationVersion || a.Assessment == nil || a.SealEvidenceRef == "" || c.CompilerVersion != PreparationDefinitionCompilerVersion || c.SnapshotHash != a.Binding.SnapshotHash || c.ManifestHash != a.Binding.ManifestHash || !boundedPreparationIdentity(c.SourceInventoryHash) || !boundedPreparationIdentity(c.ProfileContractsHash) || c.SourceCount < 2 || c.SourceCount > 4096 || c.RequirementCount < 1 || c.RequirementCount > 4096 || c.ObligationCount < 1 || c.ObligationCount > 4096 || c.VerificationCount < 1 || c.VerificationCount > 4096 {
		return common.ErrConflict
	}
	for _, check := range a.Checks {
		if check.Reason == "binding_changed" || check.Reason == "original_check_interrupted" {
			return common.ErrConflict
		}
	}
	return nil
}

// Return the complete validated original source, never a caller-supplied hash
// or a successor Definition. The independent pure compiler must still validate
// the snapshot and its complete all_of semantics.
func PreparationDefinitionCompilationSource(w *Workspace, a WorkspacePreparationAttempt) (PreparationSealSnapshot, error) {
	if w == nil || a.Assessment == nil || a.Assessment.ValidateFor(a.Binding.ManifestHash, a.Checks) != nil {
		return PreparationSealSnapshot{}, common.ErrConflict
	}
	var source *WorkspacePreparationSealEvidence
	for i := range w.PreparationSealEvidence {
		e := &w.PreparationSealEvidence[i]
		if e.Ref == a.SealEvidenceRef {
			if source != nil || e.validateFor(w, a) != nil {
				return PreparationSealSnapshot{}, common.ErrConflict
			}
			source = e
		}
	}
	if source == nil {
		return PreparationSealSnapshot{}, common.ErrConflict
	}
	for _, check := range a.Checks {
		if check.Reason == "binding_changed" || check.Reason == "original_check_interrupted" || (check.ID == "exact_seal" || check.ID == "persistence_integrity") && check.State != "passed" {
			return PreparationSealSnapshot{}, common.ErrConflict
		}
	}
	s := source.Before.Clone()
	s.WorkspaceRevision-- // original manifest preimage, not the observation revision
	s.SnapshotHash = s.Hash()
	if s.SnapshotHash != a.Binding.SnapshotHash {
		return PreparationSealSnapshot{}, common.ErrConflict
	}
	return s, nil
}
