package orchestrator

import (
	"fmt"
	"strings"

	runtimev2 "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtimev2"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// runtimeV2CandidateRequestIdentity projects the identity for one physical
// LLM call from sealed authority only. It is deliberately not a state
// transition: C2a makes the EventHub contract total before C4 changes the
// request-session drive ordering. C4 must persist exactly this identity in the
// request-session ledger before sending the call.
func runtimeV2CandidateRequestIdentity(
	aggregate runtimev2.RuntimeAggregate,
	stablePrefixHash string,
	candidateContext runtimecontract.RuntimeCandidateContext,
	manifest runtimecontract.RuntimeCandidateContractManifest,
	previousFailure *runtimecontract.RuntimeCandidateFailure,
	sequence int,
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
	correctionHash := runtimecontract.StableRuntimeCandidateRequestCorrectionHash(previousFailure)
	logicalRequestID = runtimecontract.StableRuntimeCandidateRequestSessionID(
		aggregate.Facts.WorkspaceID,
		aggregate.Facts.RunID,
		transaction,
		stablePrefixHash,
		manifest.Hash,
		correctionHash,
	)
	physicalCallID = runtimecontract.StableRuntimeCandidatePhysicalCallID(logicalRequestID, sequence)
	if err := runtimecontract.ValidateRuntimeCandidateRequestIdentity(logicalRequestID, sequence, physicalCallID); err != nil {
		return "", "", err
	}
	return logicalRequestID, physicalCallID, nil
}
