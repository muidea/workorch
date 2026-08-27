package orchestrator

import (
	"fmt"
	"strings"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/requestcontent"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// runtimeCandidateRequestIdentity projects the identity for one physical
// LLM call from sealed authority only. It is deliberately not a state
// transition: C2a makes the EventHub contract total before C4 changes the
// request-session drive ordering. C4 must persist exactly this identity in the
// request-session ledger before sending the call.
func runtimeCandidateRequestIdentity(
	aggregate runtime.RuntimeAggregate,
	stablePrefixHash string,
	candidateContext runtimecontract.RuntimeCandidateContext,
	manifest runtimecontract.RuntimeCandidateContractManifest,
	previousFailure *runtimecontract.RuntimeCandidateFailure,
	previousCorrection *runtimecontract.RuntimeCandidateCorrectionDirective,
	sequence int,
	correctionInput *llmmodels.RuntimeCandidateCorrectionInput,
) (logicalRequestID, physicalCallID string, err error) {
	if aggregate.Facts.WorkspaceID == "" || aggregate.Facts.RunID == "" || strings.TrimSpace(stablePrefixHash) == "" || sequence <= 0 {
		return "", "", fmt.Errorf("runtime candidate request identity requires scope, stable prefix and sequence")
	}
	transaction := aggregate.Decision.AuthorityTransaction()
	if err := runtimecontract.ValidateAuthorityTransaction(aggregate.Decision, transaction); err != nil {
		return "", "", err
	}
	if err := runtimecontract.ValidateRuntimeCandidateContractManifestForDecision(manifest, aggregate.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromContext(candidateContext)); err != nil {
		return "", "", err
	}
	observation, err := requestcontent.ProjectRuntimeCandidateCorrectionObservation(previousFailure, previousCorrection)
	if err != nil {
		return "", "", err
	}
	correctionHash := correctionInput.StableRequestCorrectionHash(observation)
	logicalRequestID = runtimecontract.StableRuntimeCandidateRequestSessionID(
		runtimecontract.RuntimeProviderDocumentOperations,
		aggregate.Facts.WorkspaceID,
		aggregate.Facts.RunID,
		transaction,
		stablePrefixHash,
		manifest.Hash,
		correctionHash,
	)
	physicalCallID = runtimecontract.StableRuntimeCandidatePhysicalCallID(logicalRequestID, sequence)
	if err := (runtimecontract.RuntimeCandidateRequestIdentity{LogicalRequestID: logicalRequestID, Sequence: sequence, PhysicalCallID: physicalCallID}).Validate(); err != nil {
		return "", "", err
	}
	return logicalRequestID, physicalCallID, nil
}
