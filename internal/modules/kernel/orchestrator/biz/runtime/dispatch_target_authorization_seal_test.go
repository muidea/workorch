package runtime

import (
	"reflect"
	"strings"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestSealDispatchWithManifestRetainsMultiTargetExactSingletonAuthorization(t *testing.T) {
	aggregate := multiTargetExactDispatchAggregate(t)
	manifest, candidate := multiTargetExactDispatchManifestAndCandidate(t, aggregate)

	dispatch, err := SealDispatchWithManifestAndOwnerAuthority(
		aggregate.Decision,
		candidate,
		runtimecontract.RuntimeActivationForFacts(aggregate.Facts),
		manifest,
		nil,
		aggregate.Facts.Resources.OperationAdmissibilities,
		aggregate.Facts.Resources.PrerequisiteObservations,
	)
	if err != nil {
		t.Fatalf("final SM-3 seal: %v", err)
	}
	if dispatch.CandidateManifestHash != manifest.Hash || len(dispatch.OperationTargetAuthorizations) != 1 {
		t.Fatalf("sealed manifest target authority = hash=%q snapshots=%+v", dispatch.CandidateManifestHash, dispatch.OperationTargetAuthorizations)
	}
	snapshot := dispatch.OperationTargetAuthorizations[0]
	operation := dispatch.Candidate.StateChange.Operations[0].Operation.Normalized()
	if snapshot.OperationID != operation.ID || snapshot.OperationHash != runtimecontract.StableRuntimeOperationHash(operation) || !snapshot.RequireTargets ||
		snapshot.Target.Policy != runtimecontract.RuntimeOperationTargetPolicyExactTarget ||
		!reflect.DeepEqual(snapshot.Target.ExactTargetRefs, []string{"go.mod", "main.go", "semantic_test.go"}) ||
		!reflect.DeepEqual(operation.TargetRefs, []string{"main.go"}) {
		t.Fatalf("multi-target singleton target snapshot = operation=%+v snapshot=%+v", operation, snapshot)
	}
	if err := dispatch.ValidateWithActivation(aggregate.Decision, runtimecontract.RuntimeActivationForFacts(aggregate.Facts)); err != nil {
		t.Fatalf("sealed multi-target singleton dispatch is not self-contained: %v", err)
	}
}

func TestSealDispatchWithManifestRejectsTargetDriftAtFinalBoundary(t *testing.T) {
	aggregate := multiTargetExactDispatchAggregate(t)
	manifest, candidate := multiTargetExactDispatchManifestAndCandidate(t, aggregate)
	candidate.StateChange.Operations[0].Operation.TargetRefs = []string{"outside.go"}
	var deriveErr error
	candidate, deriveErr = runtimecontract.DeriveRuntimeCandidateOperationAuthority(manifest, candidate)
	if deriveErr != nil {
		t.Fatalf("derive drifted candidate identity: %v", deriveErr)
	}

	_, err := SealDispatchWithManifestAndOwnerAuthority(
		aggregate.Decision,
		candidate,
		runtimecontract.RuntimeActivationForFacts(aggregate.Facts),
		manifest,
		nil,
		aggregate.Facts.Resources.OperationAdmissibilities,
		aggregate.Facts.Resources.PrerequisiteObservations,
	)
	if err == nil || !strings.Contains(err.Error(), "target") {
		t.Fatalf("final SM-3 target drift error = %v", err)
	}
}

func multiTargetExactDispatchAggregate(t *testing.T) RuntimeAggregate {
	t.Helper()
	facts := baseDispatchFacts()
	facts.Focus.TargetRefs = []string{"go.mod", "main.go", "semantic_test.go"}
	aggregate := RuntimeAggregate{StateRevision: 1, Definition: dispatchDefinition(), Facts: facts}
	runtimeTestEnsureCurrentFocusBindings(t, &aggregate)
	return aggregate
}

func multiTargetExactDispatchManifestAndCandidate(t *testing.T, aggregate RuntimeAggregate) (runtimecontract.RuntimeCandidateContractManifest, runtimecontract.RuntimeCandidate) {
	t.Helper()
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(
		aggregate.Decision,
		runtimecontract.RuntimeCandidateManifestOwnerFactsFromContext(
			runtimecontract.RuntimeCandidateContextForDecision(aggregate.Facts, aggregate.Decision, nil),
		),
	)
	if err != nil {
		t.Fatalf("compile candidate manifest: %v", err)
	}
	candidate := stateChangeCandidate()
	candidate.StateChange.Operations[0].Operation.TargetRefs = []string{"main.go"}
	candidate = runtimeTestBindCandidateToCurrentFocus(t, aggregate, candidate)
	candidate, err = runtimecontract.DeriveRuntimeCandidateOperationAuthority(manifest, candidate)
	if err != nil {
		t.Fatalf("derive candidate operation authority: %v", err)
	}
	if err := runtimecontract.ValidateRuntimeCandidateAgainstManifest(manifest, candidate); err != nil {
		t.Fatalf("admit manifest-bound candidate: %v", err)
	}
	return manifest, candidate
}
