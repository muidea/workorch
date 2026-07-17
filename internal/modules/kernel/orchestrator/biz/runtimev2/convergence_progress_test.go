package runtimev2

import (
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeConvergenceProgressSignatureIgnoresRetryBookkeeping(t *testing.T) {
	facts := controllerFacts()
	decision, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	left := RuntimeAggregate{StateRevision: 1, Definition: controllerDefinition(), Facts: facts, Decision: decision}
	right := left
	right.StateRevision = 99
	right.Decision.AuthoritySnapshot.Attempt = 98
	right.CandidateAttempts = []runtimecontract.RuntimeCandidateAttemptRecord{{
		AuthorityContractID: runtimecontract.StableAuthorityContractID(facts.WorkspaceID, facts.RunID, decision.AuthorityTransaction()),
		Transaction:         decision.AuthorityTransaction(), Attempt: 0, StablePrefixHash: "stable-prefix",
		State:   runtimecontract.RuntimeCandidateAttemptStateFailed,
		Failure: &runtimecontract.RuntimeCandidateFailure{Class: runtimecontract.RuntimeCandidateFailureClassSchema, Reason: "invalid schema"},
	}}
	if RuntimeConvergenceProgressSignature(left) != RuntimeConvergenceProgressSignature(right) {
		t.Fatal("retry bookkeeping changed convergence progress signature")
	}
}

func TestRuntimeConvergenceProgressSignatureTracksEvidenceAndDispatch(t *testing.T) {
	facts := controllerFacts()
	decision, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	base := RuntimeAggregate{StateRevision: 1, Definition: controllerDefinition(), Facts: facts, Decision: decision}
	withEvidence := base
	withEvidence.Facts.EvidenceRevision = "evidence-v2-00000000000000000001"
	if RuntimeConvergenceProgressSignature(base) == RuntimeConvergenceProgressSignature(withEvidence) {
		t.Fatal("evidence progress did not change convergence signature")
	}
	withDispatch := base
	withDispatch.ActiveDispatch = &SealedDispatch{CandidateHash: "candidate-1", State: DispatchStateSealed}
	if RuntimeConvergenceProgressSignature(base) == RuntimeConvergenceProgressSignature(withDispatch) {
		t.Fatal("sealed dispatch did not change convergence signature")
	}
}
