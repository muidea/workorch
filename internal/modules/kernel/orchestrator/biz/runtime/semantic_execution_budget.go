package runtime

import (
	"fmt"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// SemanticExecutionBudget reads the exact pre-submission owner snapshot. It
// cannot reconstruct an older budget from a newer aggregate revision.
func SemanticExecutionBudget(a RuntimeAggregate, c runtimecontract.RuntimeSemanticPlanRequestContext, m runtimecontract.RuntimeSemanticPlanManifest) (runtimecontract.RuntimeSemanticExecutionBudget, error) {
	invalid := runtimecontract.RuntimeSemanticExecutionBudget{}
	if err := a.Validate(); err != nil {
		return invalid, err
	}
	if c.WorkspaceID != a.Facts.WorkspaceID || c.RunID != a.Facts.RunID || c.Definition.Revision != a.Definition.Revision || c.Definition.StartupInputHash != a.Definition.StartupInputHash ||
		c.DefinitionSeed.Hash != a.Facts.DefinitionSeed.Hash || a.SemanticPlanning.Manifest == nil || m.Hash != a.SemanticPlanning.Manifest.Hash ||
		c.CoverageProgress.SourceStateRevision != a.StateRevision || c.CoverageProgress.EvidenceRevision != a.Facts.EvidenceRevision ||
		!runtimecontract.SameRuntimeExecutionLimits(c.Activation.ExecutionContract.ExecutionLimits, a.Facts.Activation.ExecutionContract.ExecutionLimits) {
		return invalid, fmt.Errorf("semantic execution budget aggregate scope or source mismatch")
	}
	if err := m.Validate(); err != nil {
		return invalid, err
	}
	if err := c.CoverageProgress.Validate(c, m); err != nil {
		return invalid, err
	}
	view, err := runtimecontract.BuildRuntimeExecutionBudgetView(a.Facts.WorkspaceID, a.Facts.RunID, a.StateRevision, a.ExecutionBudget, "", "", runtimecontract.RuntimeExecutionBudgetSourceUsageFromFacts(a.Facts))
	if err != nil {
		return invalid, err
	}
	durable, err := runtimecontract.RuntimeDurableDispatchUsage(sealedDispatchRecord(a.ActiveDispatch), sealedDispatchesToRuntimeRecords(a.CompletedDispatches), a.CandidateAttempts)
	if err != nil {
		return invalid, err
	}
	return runtimecontract.BuildRuntimeSemanticExecutionBudget(c, m, view, runtimecontract.RuntimeSemanticProofBatchUsage(a.SemanticPlanning), durable)
}
