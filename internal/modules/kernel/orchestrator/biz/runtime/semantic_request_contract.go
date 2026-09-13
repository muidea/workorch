package runtime

import (
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// SemanticExpectedDocument projects the current persisted request intent. Both
// request rendering and SM-3 admission use this read-only projection; callers
// cannot select a different batch, correction attempt, or output budget.
func SemanticExpectedDocument(aggregate RuntimeAggregate) (runtimecontract.RuntimeExpectedDocumentContract, error) {
	ledger := aggregate.SemanticPlanning
	if ledger.Manifest == nil {
		return runtimecontract.RuntimeExpectedDocumentContract{}, fmt.Errorf("%w: semantic expected document requires a manifest", common.ErrConflict)
	}
	execution := aggregate.Facts.Activation.ExecutionContract.Normalized()
	expected := runtimecontract.RuntimeExpectedDocumentContract{
		SchemaVersion:      runtimecontract.RuntimeExpectedDocumentSchemaCurrent,
		DocumentKind:       runtimecontract.RuntimeProviderDocumentSemanticPlan,
		PlanStep:           runtimecontract.RuntimeSemanticPlanStepCoverageOrder,
		DefinitionSeedHash: aggregate.Facts.DefinitionSeed.Hash,
		ManifestHash:       ledger.Manifest.Hash, LogicalRequestID: ledger.LogicalRequestID,
		StablePrefixHash: ledger.StablePrefixHash, CorrectionAttempt: ledger.Attempt,
		MaxRecoveryAttempts: execution.CandidateDocumentRecovery.MaxAttempts,
	}
	if aggregate.Facts.Resources.ProviderCeiling != nil {
		expected.MaxOutputTokens = aggregate.Facts.Resources.ProviderCeiling.EffectiveMaxOutputTokens
	}
	if expected.MaxRecoveryAttempts <= 0 {
		expected.MaxRecoveryAttempts = 1
	}
	if ledger.ActiveBatchID != "" {
		found := false
		for _, batch := range ledger.Batches {
			if batch.BatchID != ledger.ActiveBatchID {
				continue
			}
			if found {
				return runtimecontract.RuntimeExpectedDocumentContract{}, fmt.Errorf("%w: ambiguous semantic active batch", common.ErrConflict)
			}
			found = true
			expected.PlanStep = runtimecontract.RuntimeSemanticPlanStepProofBatch
			expected.BatchID, expected.BatchSequence = batch.BatchID, batch.Sequence
			expected.BatchCoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), batch.CoverageWorkItemIDs...)
			expected.CoverageOrderHash = batch.CoverageOrderHash
		}
		if !found {
			return runtimecontract.RuntimeExpectedDocumentContract{}, fmt.Errorf("%w: semantic expected document has no persisted active batch", common.ErrConflict)
		}
	}
	expected.Hash = runtimecontract.StableRuntimeExpectedDocumentHash(expected)
	return expected, nil
}

// SemanticRequestIdentityHash identifies the request within its sealed intent;
// it does not create or advance the semantic planning ledger.
func SemanticRequestIdentityHash(ledger runtimecontract.RuntimeSemanticPlanningLedger) (string, error) {
	if ledger.CorrectionDirective != nil {
		return ledger.CorrectionDirective.Hash, nil
	}
	if ledger.ActiveBatchID == "" {
		return "", nil
	}
	for _, batch := range ledger.Batches {
		if batch.BatchID == ledger.ActiveBatchID {
			return fmt.Sprintf("semantic-proof-batch-%d", batch.Sequence), nil
		}
	}
	return "", fmt.Errorf("%w: semantic request identity has no persisted active batch", common.ErrConflict)
}

func validateSemanticRequestIntent(aggregate RuntimeAggregate, session runtimecontract.RuntimeCandidateRequestSession) error {
	expected, err := SemanticExpectedDocument(aggregate)
	if err != nil {
		return err
	}
	identityHash, err := SemanticRequestIdentityHash(aggregate.SemanticPlanning)
	if err != nil {
		return err
	}
	if err := expected.Validate(); err != nil {
		return fmt.Errorf("%w: current semantic expected document is invalid", common.ErrConflict)
	}
	if session.LogicalRequestID != expected.LogicalRequestID || session.ManifestHash != expected.ManifestHash ||
		session.StablePrefixHash != expected.StablePrefixHash || session.CorrectionHash != identityHash ||
		!sameRuntimeExpectedDocument(session.ExpectedDocument, &expected) {
		return fmt.Errorf("%w: semantic request session does not match current request intent", common.ErrConflict)
	}
	return nil
}
