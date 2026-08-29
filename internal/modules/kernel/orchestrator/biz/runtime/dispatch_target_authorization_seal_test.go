package runtime

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestCandidateAdmissionPersistsIndependentManagedValidationRecipeProjection(t *testing.T) {
	aggregate := validationAggregate(t)
	repository := &controllerRepository{aggregate: &aggregate}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.BeginCandidateAttemptForTest(
		context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, "stable-prefix",
	); err != nil {
		t.Fatal(err)
	}
	if err := RecordFixtureCandidateFeasibilities(
		context.Background(), controller, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, validationCandidate(),
	); err != nil {
		t.Fatal(err)
	}
	sealed, err := controller.AdmitAndSealCandidateAttemptForTest(
		context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, validationCandidate(),
	)
	if err != nil || !sealed.Admission.Accepted() || sealed.Aggregate.ActiveDispatch == nil {
		t.Fatalf("managed-validation admission = %+v err=%v", sealed, err)
	}
	_, attempt := runtimeTestAcceptedAttemptForActiveDispatch(t, sealed.Aggregate)
	if !runtimecontract.RuntimeValidationRecipeProjectionsEqual(
		attempt.AdmissionValidationRecipes,
		sealed.Aggregate.ActiveDispatch.ValidationRecipes,
	) || attempt.ValidationRecipeHash != runtimecontract.StableRuntimeValidationRecipeSnapshotHash(attempt.AdmissionValidationRecipes) {
		t.Fatalf("managed-validation admission lost independent recipe projection: attempt=%+v dispatch=%+v", attempt, sealed.Aggregate.ActiveDispatch)
	}
	saves := repository.saves
	duplicate, err := controller.AdmitAndSealCandidateAttemptForTest(
		context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, validationCandidate(),
	)
	if err != nil || !duplicate.Admission.Accepted() || duplicate.Aggregate.StateRevision != sealed.Aggregate.StateRevision || repository.saves != saves {
		t.Fatalf("managed-validation duplicate admission = %+v err=%v saves=%d", duplicate, err, repository.saves)
	}
}

func TestSealDispatchWithManifestRetainsMultiTargetExactSingletonAuthorization(t *testing.T) {
	aggregate := multiTargetExactDispatchAggregate(t)
	manifest, candidate := multiTargetExactDispatchManifestAndCandidate(t, aggregate)
	feasibilities := runtimeTestCurrentCandidateFeasibilities(t, aggregate, manifest, candidate)

	dispatch, err := SealDispatchWithManifestAndOwnerAuthority(
		aggregate.Decision,
		candidate,
		runtimecontract.RuntimeActivationForFacts(aggregate.Facts),
		manifest,
		feasibilities,
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
	feasibilities := runtimeTestCurrentCandidateFeasibilities(t, aggregate, manifest, candidate)

	_, err := SealDispatchWithManifestAndOwnerAuthority(
		aggregate.Decision,
		candidate,
		runtimecontract.RuntimeActivationForFacts(aggregate.Facts),
		manifest,
		feasibilities,
		aggregate.Facts.Resources.OperationAdmissibilities,
		aggregate.Facts.Resources.PrerequisiteObservations,
	)
	if err == nil || !strings.Contains(err.Error(), "target") {
		t.Fatalf("final SM-3 target drift error = %v", err)
	}
}

func TestRuntimeAggregateFreshLoadRejectsCoordinatedDispatchProjectionAndSealRewrite(t *testing.T) {
	repository := &durableRuntimeStateRepository{aggregate: validationRepairSurfaceAggregateForTest(t, true)}
	sealed := sealCandidateForTest(t, repository, validationCandidate())
	if sealed.ActiveDispatch == nil || len(sealed.ActiveDispatch.OperationTargetAuthorizations) != 1 {
		t.Fatalf("accepted validation admission fixture = dispatch=%+v attempts=%+v", sealed.ActiveDispatch, sealed.CandidateAttempts)
	}
	attemptIndex, accepted := runtimeTestAcceptedAttemptForActiveDispatch(t, sealed)
	if err := sealed.Validate(); err != nil {
		t.Fatalf("accepted exact-scope baseline: %v", err)
	}
	authorization := accepted.AdmissionTargetAuthorizations[0].Target.Normalized()
	if authorization.Policy != runtimecontract.RuntimeOperationTargetPolicyExactTarget || len(authorization.ExactTargetRefs) == 0 {
		t.Fatalf("accepted validation admission scope = %+v", authorization)
	}
	if !runtimecontract.RuntimeDispatchOperationTargetAuthorizationProjectionsEqual(
		accepted.AdmissionTargetAuthorizations,
		sealed.ActiveDispatch.OperationTargetAuthorizations,
	) {
		t.Fatalf("accepted admission/dispatch target projections differ: attempt=%+v dispatch=%+v", accepted.AdmissionTargetAuthorizations, sealed.ActiveDispatch.OperationTargetAuthorizations)
	}
	wantCommitment := runtimecontract.StableRuntimeDispatchOperationTargetAuthorizationsHash(accepted.AdmissionTargetAuthorizations)
	if accepted.TargetAuthorizationHash != wantCommitment {
		t.Fatalf("accepted attempt target commitment = %q, want %q", accepted.TargetAuthorizationHash, wantCommitment)
	}
	if !acceptedCandidateAdmissionDuplicate(sealed, accepted.Attempt, accepted.ManifestHash, accepted.LogicalRequestID, sealed.ActiveDispatch.Candidate) {
		t.Fatal("exact accepted admission was not recognized as duplicate")
	}

	tampered := sealed
	tampered.CandidateAttempts = make([]runtimecontract.RuntimeCandidateAttemptRecord, len(sealed.CandidateAttempts))
	for idx := range sealed.CandidateAttempts {
		tampered.CandidateAttempts[idx] = sealed.CandidateAttempts[idx].Clone()
	}
	tampered.ActiveDispatch.OperationTargetAuthorizations = runtimecontract.CloneRuntimeDispatchOperationTargetAuthorizations(
		sealed.ActiveDispatch.OperationTargetAuthorizations,
	)
	tampered.ActiveDispatch.OperationTargetAuthorizations[0].Target.ExactTargetRefs = append(
		tampered.ActiveDispatch.OperationTargetAuthorizations[0].Target.ExactTargetRefs,
		"support.go",
	)
	tampered.ActiveDispatch.DispatchAuthorityHash = runtimecontract.StableRuntimeDispatchAuthorityHash(
		sealedDispatchToRuntimeRecord(*tampered.ActiveDispatch),
	)
	// Reproduce the old coordinated-rehash gap: the attempt projection, dispatch
	// projection and dispatch self-seal all move together. Only the complete
	// manifest body persisted by the earlier request-session CAS remains an
	// independent pre-seal authority source.
	tampered.CandidateAttempts[attemptIndex].AdmissionTargetAuthorizations = runtimecontract.CloneRuntimeDispatchOperationTargetAuthorizations(
		tampered.ActiveDispatch.OperationTargetAuthorizations,
	)
	tampered.CandidateAttempts[attemptIndex].TargetAuthorizationHash = runtimecontract.StableRuntimeDispatchOperationTargetAuthorizationsHash(
		tampered.ActiveDispatch.OperationTargetAuthorizations,
	)
	tampered.CandidateAttempts[attemptIndex].DispatchAuthorityHash = tampered.ActiveDispatch.DispatchAuthorityHash
	if acceptedCandidateAdmissionDuplicate(tampered, accepted.Attempt, accepted.ManifestHash, accepted.LogicalRequestID, tampered.ActiveDispatch.Candidate) {
		t.Fatal("coordinated dispatch projection/seal rewrite was recognized as an exact accepted duplicate")
	}

	payload, err := json.Marshal(tampered)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded RuntimeAggregate
	if err := json.Unmarshal(payload, &reloaded); err != nil {
		t.Fatal(err)
	}
	if err := reloaded.Validate(); err == nil || !strings.Contains(err.Error(), "projections do not match pre-seal manifest") {
		t.Fatalf("coordinated dispatch projection/seal rewrite fresh-load error = %v", err)
	}
}

func TestRuntimeAggregateFreshLoadRejectsCoordinatedDispatchRecipeAndSealRewrite(t *testing.T) {
	repository := &durableRuntimeStateRepository{aggregate: validationRepairSurfaceAggregateForTest(t, true)}
	sealed := sealCandidateForTest(t, repository, validationCandidate())
	if sealed.ActiveDispatch == nil || len(sealed.ActiveDispatch.ValidationRecipes) != 1 {
		t.Fatalf("accepted managed-validation admission fixture = dispatch=%+v attempts=%+v", sealed.ActiveDispatch, sealed.CandidateAttempts)
	}
	attemptIndex, attempt := runtimeTestAcceptedAttemptForActiveDispatch(t, sealed)
	if !runtimecontract.RuntimeValidationRecipeProjectionsEqual(attempt.AdmissionValidationRecipes, sealed.ActiveDispatch.ValidationRecipes) {
		t.Fatalf("accepted admission/dispatch recipe projections differ: attempt=%+v dispatch=%+v", attempt.AdmissionValidationRecipes, sealed.ActiveDispatch.ValidationRecipes)
	}
	wantCommitment := runtimecontract.StableRuntimeValidationRecipeSnapshotHash(attempt.AdmissionValidationRecipes)
	if attempt.ValidationRecipeHash != wantCommitment {
		t.Fatalf("accepted attempt recipe commitment = %q, want %q", attempt.ValidationRecipeHash, wantCommitment)
	}
	if !acceptedCandidateAdmissionDuplicate(sealed, attempt.Attempt, attempt.ManifestHash, attempt.LogicalRequestID, sealed.ActiveDispatch.Candidate) {
		t.Fatal("exact managed-validation admission was not recognized as duplicate")
	}

	tampered := sealed
	dispatch := *sealed.ActiveDispatch
	dispatch.ValidationRecipes = runtimecontract.CloneRuntimeValidationRecipes(sealed.ActiveDispatch.ValidationRecipes)
	dispatch.ValidationRecipes[0].Args = []string{"test", "./coordinated-rewrite/..."}
	dispatch.DispatchAuthorityHash = runtimecontract.StableRuntimeDispatchAuthorityHash(
		sealedDispatchToRuntimeRecord(dispatch),
	)
	tampered.ActiveDispatch = &dispatch
	tampered.CandidateAttempts = make([]runtimecontract.RuntimeCandidateAttemptRecord, len(sealed.CandidateAttempts))
	for idx := range sealed.CandidateAttempts {
		tampered.CandidateAttempts[idx] = sealed.CandidateAttempts[idx].Clone()
	}
	// Move both copied projections and the dispatch self-seal together. The
	// earlier request-session manifest body is the remaining independent source.
	tampered.CandidateAttempts[attemptIndex].AdmissionValidationRecipes = runtimecontract.CloneRuntimeValidationRecipes(dispatch.ValidationRecipes)
	tampered.CandidateAttempts[attemptIndex].ValidationRecipeHash = runtimecontract.StableRuntimeValidationRecipeSnapshotHash(dispatch.ValidationRecipes)
	tampered.CandidateAttempts[attemptIndex].DispatchAuthorityHash = dispatch.DispatchAuthorityHash
	if acceptedCandidateAdmissionDuplicate(tampered, attempt.Attempt, attempt.ManifestHash, attempt.LogicalRequestID, dispatch.Candidate) {
		t.Fatal("coordinated dispatch recipe/seal rewrite was recognized as an exact accepted duplicate")
	}

	payload, err := json.Marshal(tampered)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded RuntimeAggregate
	if err := json.Unmarshal(payload, &reloaded); err != nil {
		t.Fatal(err)
	}
	if err := reloaded.Validate(); err == nil || !strings.Contains(err.Error(), "projections do not match pre-seal manifest") {
		t.Fatalf("coordinated dispatch recipe/seal rewrite fresh-load error = %v", err)
	}
}

func TestRuntimeAggregateFreshLoadRejectsCoordinatedDispatchCoverageBindingAndSealRewrite(t *testing.T) {
	repository := &durableRuntimeStateRepository{aggregate: validationRepairSurfaceAggregateForTest(t, true)}
	sealed := sealCandidateForTest(t, repository, validationCandidate())
	if sealed.ActiveDispatch == nil || len(sealed.ActiveDispatch.CoverageBindings) == 0 {
		t.Fatalf("accepted coverage-binding admission fixture = dispatch=%+v attempts=%+v", sealed.ActiveDispatch, sealed.CandidateAttempts)
	}
	attemptIndex, attempt := runtimeTestAcceptedAttemptForActiveDispatch(t, sealed)
	if !runtimecontract.RuntimeCoverageEvidenceBindingProjectionsEqual(attempt.AdmissionCoverageBindings, sealed.ActiveDispatch.CoverageBindings) {
		t.Fatalf("accepted admission/dispatch coverage projections differ: attempt=%+v dispatch=%+v", attempt.AdmissionCoverageBindings, sealed.ActiveDispatch.CoverageBindings)
	}

	tampered := sealed
	dispatch := *sealed.ActiveDispatch
	dispatch.CoverageBindings = runtimecontract.CloneCoverageEvidenceBindings(sealed.ActiveDispatch.CoverageBindings)
	dispatch.CoverageBindings[0].DeclarationRef = "test/coordinated-preseal-rewrite"
	dispatch.CoverageBindings = runtimecontract.NormalizeCoverageEvidenceBindings(dispatch.CoverageBindings)
	dispatch.CoverageBindingHash = runtimecontract.StableRuntimeCoverageEvidenceBindingSnapshotHash(dispatch.CoverageBindings)
	dispatch.DispatchAuthorityHash = runtimecontract.StableRuntimeDispatchAuthorityHash(sealedDispatchToRuntimeRecord(dispatch))
	tampered.ActiveDispatch = &dispatch
	tampered.CandidateAttempts = make([]runtimecontract.RuntimeCandidateAttemptRecord, len(sealed.CandidateAttempts))
	for idx := range sealed.CandidateAttempts {
		tampered.CandidateAttempts[idx] = sealed.CandidateAttempts[idx].Clone()
	}
	tampered.CandidateAttempts[attemptIndex].AdmissionCoverageBindings = runtimecontract.CloneCoverageEvidenceBindings(dispatch.CoverageBindings)
	tampered.CandidateAttempts[attemptIndex].AdmissionCoverageBindingHash = runtimecontract.StableRuntimeCoverageEvidenceBindingSnapshotHash(dispatch.CoverageBindings)
	tampered.CandidateAttempts[attemptIndex].DispatchAuthorityHash = dispatch.DispatchAuthorityHash
	if acceptedCandidateAdmissionDuplicate(tampered, attempt.Attempt, attempt.ManifestHash, attempt.LogicalRequestID, dispatch.Candidate) {
		t.Fatal("coordinated coverage-binding projection/seal rewrite was recognized as an exact accepted duplicate")
	}
	tamperedRecord := sealedDispatchToRuntimeRecord(*tampered.ActiveDispatch)
	if err := runtimecontract.ValidateRuntimeAcceptedCandidateManifestLineage(
		&tamperedRecord, sealedDispatchesToRuntimeRecords(tampered.CompletedDispatches), tampered.CandidateAttempts, tampered.CandidateRequestSessions,
	); err == nil || !strings.Contains(err.Error(), "projections do not match pre-seal manifest") {
		t.Fatalf("coordinated coverage-binding rewrite candidate-manifest lineage error = %v", err)
	}

	payload, err := json.Marshal(tampered)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded RuntimeAggregate
	if err := json.Unmarshal(payload, &reloaded); err != nil {
		t.Fatal(err)
	}
	if err := reloaded.Validate(); err == nil ||
		(!strings.Contains(err.Error(), "semantic binding preimage violation") &&
			!strings.Contains(err.Error(), "projections do not match pre-seal manifest")) {
		t.Fatalf("coordinated coverage-binding projection/seal rewrite fresh-load error = %v", err)
	}
}

func runtimeTestAcceptedAttemptForActiveDispatch(t *testing.T, aggregate RuntimeAggregate) (int, runtimecontract.RuntimeCandidateAttemptRecord) {
	t.Helper()
	if aggregate.ActiveDispatch == nil {
		t.Fatal("accepted-attempt fixture has no active dispatch")
	}
	matched := -1
	for index, attempt := range aggregate.CandidateAttempts {
		if attempt.State != runtimecontract.RuntimeCandidateAttemptStateAccepted || attempt.DispatchID != aggregate.ActiveDispatch.ID {
			continue
		}
		if matched >= 0 {
			t.Fatalf("active dispatch %q has duplicate accepted attempts", aggregate.ActiveDispatch.ID)
		}
		matched = index
	}
	if matched < 0 {
		t.Fatalf("active dispatch %q has no accepted attempt", aggregate.ActiveDispatch.ID)
	}
	return matched, aggregate.CandidateAttempts[matched].Clone()
}

func multiTargetExactDispatchAggregate(t *testing.T) RuntimeAggregate {
	t.Helper()
	targets := []string{"go.mod", "main.go", "semantic_test.go"}
	return stateChangeAggregateWithPreparation(t, func(aggregate *RuntimeAggregate) {
		aggregate.Facts.Focus.TargetRefs = append([]string(nil), targets...)
		catalog := runtimecontract.RuntimeOperationAdmissibility{
			PrerequisiteID: "operation-admissibility-file-multi-target", OwnerModuleID: runtimecontract.RuntimeModuleFile,
			OperationKinds: []string{runtimecontract.RuntimeBlockKindPatchApply},
			OperationVariants: []runtimecontract.RuntimeOperationAdmissibilityVariant{{
				Kind: runtimecontract.RuntimeBlockKindPatchApply, VariantID: "file_write",
				AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExactTarget},
			}},
			TargetRefs: append([]string(nil), targets...), ObservedRevision: "file-multi-target-revision-1",
		}
		catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
		prerequisite := runtimecontract.RuntimePrerequisite{
			ID: catalog.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
			Scope: runtimecontract.RuntimePrerequisiteScopeGlobal, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindPatchApply},
		}
		observation := runtimecontract.RuntimePrerequisiteObservation{
			PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: prerequisite.OwnerModuleID,
			ContentHash: catalog.SnapshotID,
			Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
				Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
				SubjectRef: "workspace_deliverable", Revision: catalog.ObservedRevision,
			}},
		}
		observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
		aggregate.Facts.Prerequisites.Satisfied = append(aggregate.Facts.Prerequisites.Satisfied, prerequisite)
		aggregate.Facts.Resources.OperationAdmissibilities = append(aggregate.Facts.Resources.OperationAdmissibilities, catalog)
		aggregate.Facts.Resources.PrerequisiteObservations = append(aggregate.Facts.Resources.PrerequisiteObservations, observation)
	})
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
