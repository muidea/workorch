package workorchmanagementcontract

import "github.com/muidea/workorch/internal/pkg/common"

// Metadata only: no original definitions, resource bytes, bootstrap or permit.
type PreparationDefinitionCompilationView struct {
	CompilerVersion      string `json:"compilerVersion"`
	SnapshotHash         string `json:"snapshotHash"`
	ManifestHash         string `json:"manifestHash"`
	SourceInventoryHash  string `json:"sourceInventoryHash"`
	ProfileContractsHash string `json:"profileContractsHash"`
	SourceCount          int    `json:"sourceCount"`
	RequirementCount     int    `json:"requirementCount"`
	ObligationCount      int    `json:"obligationCount"`
	VerificationCount    int    `json:"verificationCount"`
}

func (c PreparationDefinitionCompilationView) ValidateFor(a PreparationAttemptView) error {
	if a.State != "blocked" || a.EvaluationVersion != "workorch-preparation-check-evaluation/v1" || a.Assessment == nil || a.SealEvidenceRef == "" || c.CompilerVersion != "workorch-preparation-definition-compiler/v1" || c.SnapshotHash != a.Binding.SnapshotHash || c.ManifestHash != a.Binding.ManifestHash || !preparationViewIdentity(c.SourceInventoryHash) || !preparationViewIdentity(c.ProfileContractsHash) || c.SourceCount < 2 || c.SourceCount > 4096 || c.RequirementCount < 1 || c.RequirementCount > 4096 || c.ObligationCount < 1 || c.ObligationCount > 4096 || c.VerificationCount < 1 || c.VerificationCount > 4096 {
		return common.ErrConflict
	}
	for _, check := range a.Checks {
		if check.Reason == "binding_changed" || check.Reason == "original_check_interrupted" {
			return common.ErrConflict
		}
	}
	return nil
}
