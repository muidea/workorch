package runtime

import runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

// compileRuntimeSemanticAcceptedBatchBindings is the SM-2 sealing adapter
// from one accepted proof batch to binding authority. Invariant source gates
// only recompile pure preimages and cannot seal authority. RequestLogicalID is
// the immutable batch-generation identity; the planning ledger's current
// logical request is only a provider request-session cursor and may point at
// an attempt-1 correction.
func compileRuntimeSemanticAcceptedBatchBindings(
	manifest runtimecontract.RuntimeSemanticPlanManifest,
	batch runtimecontract.RuntimeSemanticPlanBatch,
	document runtimecontract.RuntimeSemanticPlanDocument,
	feasibilities []runtimecontract.RuntimeSemanticProofFeasibility,
) ([]runtimecontract.CoverageFocus, runtimecontract.RuntimeCWIReconciliationContract, error) {
	return runtimecontract.CompileRuntimeSemanticAcceptedBatchBindings(manifest, batch, document, feasibilities)
}
