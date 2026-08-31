package runtime

import (
	"fmt"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// compileRuntimeSemanticAcceptedBatchBindings is the only production adapter
// from one accepted proof batch to SM-2 binding authority. RequestLogicalID is
// the immutable batch-generation identity; the planning ledger's current
// logical request is only a provider request-session cursor and may point at
// an attempt-1 correction.
func compileRuntimeSemanticAcceptedBatchBindings(
	manifest runtimecontract.RuntimeSemanticPlanManifest,
	batch runtimecontract.RuntimeSemanticPlanBatch,
	document runtimecontract.RuntimeSemanticPlanDocument,
	feasibilities []runtimecontract.RuntimeSemanticProofFeasibility,
) ([]runtimecontract.CoverageFocus, runtimecontract.RuntimeCWIReconciliationContract, error) {
	manifest = manifest.Normalized()
	batch = batch.Normalized()
	document = document.Normalized()
	if err := batch.ValidateAgainstManifest(manifest); err != nil {
		return nil, runtimecontract.RuntimeCWIReconciliationContract{}, fmt.Errorf("runtime semantic binding batch: %w", err)
	}
	documentHash := runtimecontract.StableRuntimeSemanticPlanDocumentHash(document)
	if batch.State != runtimecontract.RuntimeSemanticPlanBatchAccepted || batch.DocumentHash != documentHash ||
		document.BatchID != batch.BatchID || document.BatchSequence != batch.Sequence {
		return nil, runtimecontract.RuntimeCWIReconciliationContract{}, fmt.Errorf("runtime semantic binding requires the exact accepted batch document")
	}
	return runtimecontract.CompileRuntimeSemanticBindings(manifest, batch.RequestLogicalID, document, feasibilities)
}
