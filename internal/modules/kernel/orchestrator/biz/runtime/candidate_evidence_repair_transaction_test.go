package runtime

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestConfirmedRepairMultiOperationReceiptChainSurvivesRestartAndDuplicate(t *testing.T) {
	fixture := newConfirmedRepairMultiOperationFixture(t)
	coordinator, err := NewDispatchCoordinator(fixture.repository)
	if err != nil {
		t.Fatal(err)
	}

	first := fixture.acceptedEvidence(t, 0, "tree-v1", "tree-v2")
	attachConfirmedRepairReceiptForTest(t, &fixture.repository.aggregate, &first, "tree-v1", "tree-v2")
	partial, err := coordinator.commitEvidence(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	if partial.DispatchCompleted || partial.Aggregate.ActiveDispatch == nil ||
		len(partial.Aggregate.ActiveDispatch.CompletedOperationIDs) != 1 ||
		partial.Aggregate.Facts.Resources.FailureTarget != nil || partial.Aggregate.Facts.RepairDirective != nil ||
		len(partial.Aggregate.PrerequisiteInvalidations) != 1 || len(partial.Aggregate.Facts.Prerequisites.Pending) == 0 {
		t.Fatalf("first all-of receipt did not persist the directive-free sealed transaction: %+v", partial.Aggregate)
	}
	if err := partial.Aggregate.Validate(); err != nil {
		t.Fatalf("partial all-of repair did not survive fresh-load invariant: %v", err)
	}

	partialRevision := partial.Aggregate.StateRevision
	restarted, err := NewDispatchCoordinator(fixture.repository)
	if err != nil {
		t.Fatal(err)
	}
	duplicateFirst, err := restarted.commitEvidence(context.Background(), first)
	if err != nil || duplicateFirst.Disposition != CommitDispositionDuplicate || duplicateFirst.Aggregate.StateRevision != partialRevision {
		t.Fatalf("first all-of receipt duplicate changed handoff: result=%+v err=%v", duplicateFirst, err)
	}

	second := fixture.acceptedEvidence(t, 1, "tree-v2", "tree-v3")
	attachConfirmedRepairReceiptForTest(t, &fixture.repository.aggregate, &second, "tree-v2", "tree-v3")
	settled, err := restarted.commitEvidence(context.Background(), second)
	if err != nil {
		t.Fatal(err)
	}
	if !settled.DispatchCompleted || settled.Aggregate.ActiveDispatch != nil ||
		settled.Aggregate.Facts.Resources.FailureTarget != nil || settled.Aggregate.Facts.RepairDirective == nil ||
		settled.Aggregate.Facts.RepairDirective.RequiredState != runtimecontract.ConvergenceStateNeedValidation ||
		settled.Aggregate.Facts.RepairDirective.CurrentFailure != nil || len(settled.Aggregate.Facts.RepairDirective.TargetRefs) != 0 ||
		settled.Aggregate.Facts.RepairDirective.Closure == nil || settled.Aggregate.Facts.RepairDirective.Closure.ID != fixture.closureID ||
		settled.Aggregate.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite ||
		len(settled.Aggregate.PrerequisiteInvalidations) != 1 {
		t.Fatalf("complete all-of repair did not close into fresh validation: %+v", settled.Aggregate)
	}
	if err := settled.Aggregate.Validate(); err != nil {
		t.Fatalf("settled all-of repair did not survive fresh load: %v", err)
	}
	for _, test := range []struct {
		name      string
		mutate    func(*RuntimeAggregate)
		wantError string
	}{
		{
			name: "missing continuation",
			mutate: func(value *RuntimeAggregate) {
				value.Facts.RepairDirective = nil
			},
			wantError: "does not exactly match its completed transaction",
		},
		{
			name: "restored closure targets",
			mutate: func(value *RuntimeAggregate) {
				value.Facts.RepairDirective.TargetRefs = append([]string(nil), value.Facts.RepairDirective.Closure.GroundedTargetRefs...)
			},
			wantError: "does not exactly match its completed transaction",
		},
		{
			name: "renamed continuation",
			mutate: func(value *RuntimeAggregate) {
				value.Facts.RepairDirective.ID = "repair-generic-fresh-validation"
			},
			wantError: "does not exactly match its completed transaction",
		},
		{
			name: "missing confirmation reverse proof",
			mutate: func(value *RuntimeAggregate) {
				value.RepairSurfaceConfirmations = nil
			},
			wantError: "has no unique exact owner confirmation",
		},
	} {
		t.Run("fresh-load "+test.name, func(t *testing.T) {
			tampered := cloneRuntimeAggregateForMCPProvenanceTest(t, settled.Aggregate)
			test.mutate(&tampered)
			if err := tampered.Validate(); err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("settled File continuation tamper error = %v", err)
			}
		})
	}

	// Evidence revision is global chronology, not a mutable payload detail. Keep
	// the execution's fact ID/hash projection coordinated with the tampered fact
	// so this case proves both recovery gates reject revision drift itself: the
	// aggregate-wide ledger and the confirmed-owner receipt-chain reverse proof.
	// Do not couple the assertion to which validator reports first or to its
	// diagnostic wording.
	t.Run("fresh-load completed evidence revision drift", func(t *testing.T) {
		tampered := cloneRuntimeAggregateForMCPProvenanceTest(t, settled.Aggregate)
		dispatch := &tampered.CompletedDispatches[len(tampered.CompletedDispatches)-1]
		mutated := false
		for index := range tampered.Facts.Evidence.Accepted {
			fact := &tampered.Facts.Evidence.Accepted[index]
			if fact.DispatchID != dispatch.ID {
				continue
			}
			fact.Revision = "runtime-evidence-99999999999999999999"
			for executionIndex := range dispatch.OperationExecutions {
				execution := &dispatch.OperationExecutions[executionIndex]
				if execution.OperationID != fact.OperationID {
					continue
				}
				execution.EvidenceFactID = fact.ID
				execution.EvidenceFactHash = runtimecontract.StableRuntimeEvidenceFactHash(*fact)
				mutated = true
			}
			break
		}
		if !mutated {
			t.Fatal("settled repair-owner transaction has no final execution evidence to drift")
		}
		if err := tampered.Validate(); err == nil {
			t.Fatal("fresh-load aggregate accepted a coordinated completed evidence revision drift")
		}
		if err := validateConfirmedRepairFreshValidationContinuation(tampered); err == nil {
			t.Fatal("confirmed File continuation accepted a coordinated completed evidence revision drift")
		}
	})

	settledRevision := settled.Aggregate.StateRevision
	restartedAfterSettlement, err := NewDispatchCoordinator(fixture.repository)
	if err != nil {
		t.Fatal(err)
	}
	duplicateSecond, err := restartedAfterSettlement.commitEvidence(context.Background(), second)
	if err != nil || duplicateSecond.Disposition != CommitDispositionDuplicate || duplicateSecond.Aggregate.StateRevision != settledRevision ||
		duplicateSecond.Aggregate.Facts.Resources.FailureTarget != nil || duplicateSecond.Aggregate.Facts.RepairDirective == nil ||
		duplicateSecond.Aggregate.Facts.RepairDirective.Closure == nil || duplicateSecond.Aggregate.Facts.RepairDirective.Closure.ID != fixture.closureID {
		t.Fatalf("final all-of receipt duplicate changed validation closure: result=%+v err=%v", duplicateSecond, err)
	}
}

func TestConfirmedRepairExactTargetSubsetSealsAndCompletesSelectedOperations(t *testing.T) {
	surfaceTargets := []string{
		"internal/config/config.go",
		"internal/config/defaults.go",
		"internal/lsp/client.go",
		"internal/lsp/references.go",
		"internal/lsp/server.go",
	}
	selectedTargets := []string{"internal/lsp/client.go", "internal/lsp/references.go"}
	fixture := newConfirmedRepairSubsetFixture(t, surfaceTargets, selectedTargets)
	dispatch := fixture.repository.aggregate.ActiveDispatch
	if dispatch == nil || dispatch.RepairCommitment == nil {
		t.Fatal("confirmed owner repair subset has no sealed dispatch commitment")
	}
	actualTargets, ownerModuleID, err := confirmedRepairDispatchTargets(*dispatch)
	if err != nil {
		t.Fatal(err)
	}
	if ownerModuleID != runtimecontract.RuntimeModuleFile || !sameRuntimeTargetRefs(actualTargets, selectedTargets) {
		t.Fatalf("sealed repair targets = %q owner=%q, want selected subset %q", actualTargets, ownerModuleID, selectedTargets)
	}
	var confirmedTargets []string
	for _, record := range fixture.repository.aggregate.RepairSurfaceConfirmations {
		if record.IdentitySignature == dispatch.RepairCommitment.ConfirmationIdentityHash {
			confirmedTargets = record.Confirmation.ConfirmedTargetRefs
		}
	}
	if !sameRuntimeTargetRefs(fixture.repository.aggregate.Decision.RequiredOutput.TargetRefs, surfaceTargets) ||
		!sameRuntimeTargetRefs(confirmedTargets, surfaceTargets) ||
		!sameRuntimeTargetRefs(dispatch.RepairCommitment.Closure.GroundedTargetRefs, surfaceTargets) {
		t.Fatalf("repair surface drifted: decision=%q confirmation=%q closure=%q want=%q",
			fixture.repository.aggregate.Decision.RequiredOutput.TargetRefs, confirmedTargets,
			dispatch.RepairCommitment.Closure.GroundedTargetRefs, surfaceTargets)
	}
	required, err := confirmedRepairCommitmentRequiredByRejectedObservation(fixture.repository.aggregate, *dispatch)
	if err != nil || !required {
		t.Fatalf("confirmed repair subset lost marker-independent commitment requirement: required=%v err=%v", required, err)
	}
	if err := fixture.repository.aggregate.Validate(); err != nil {
		t.Fatalf("confirmed repair subset did not survive initial fresh load: %v", err)
	}
	narrowedDecision := cloneRuntimeAggregateForMCPProvenanceTest(t, fixture.repository.aggregate)
	narrowedDecision.Decision.RequiredOutput.TargetRefs = append([]string(nil), selectedTargets...)
	if _, _, err := resolveConfirmedRepairTransaction(narrowedDecision, *narrowedDecision.ActiveDispatch); err == nil ||
		!strings.Contains(err.Error(), "commitment does not match the current sealed Decision") {
		t.Fatalf("confirmed repair closure was narrowed to the candidate subset: %v", err)
	}
	missingCommitment := cloneRuntimeAggregateForMCPProvenanceTest(t, fixture.repository.aggregate)
	missingCommitment.ActiveDispatch.RepairCommitment = nil
	missingCommitment.ActiveDispatch.RepairCommitmentHash = ""
	if err := validateConfirmedRepairMandatoryCommitments(missingCommitment); err == nil ||
		!strings.Contains(err.Error(), "omitted its mandatory rejected-validation commitment") {
		t.Fatalf("confirmed repair subset escaped marker-independent commitment detection: %v", err)
	}

	coordinator, err := NewDispatchCoordinator(fixture.repository)
	if err != nil {
		t.Fatal(err)
	}
	first := fixture.acceptedEvidence(t, 0, "tree-v1", "tree-v2")
	attachConfirmedRepairReceiptForTest(t, &fixture.repository.aggregate, &first, "tree-v1", "tree-v2")
	partial, err := coordinator.commitEvidence(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	if partial.DispatchCompleted || partial.Aggregate.ActiveDispatch == nil || len(partial.Aggregate.ActiveDispatch.CompletedOperationIDs) != 1 {
		t.Fatalf("first selected repair operation did not remain partial: %+v", partial.Aggregate.ActiveDispatch)
	}
	if err := partial.Aggregate.Validate(); err != nil {
		t.Fatalf("partial selected repair subset did not survive restart: %v", err)
	}

	restarted, err := NewDispatchCoordinator(fixture.repository)
	if err != nil {
		t.Fatal(err)
	}
	second := fixture.acceptedEvidence(t, 1, "tree-v2", "tree-v3")
	attachConfirmedRepairReceiptForTest(t, &fixture.repository.aggregate, &second, "tree-v2", "tree-v3")
	settled, err := restarted.commitEvidence(context.Background(), second)
	if err != nil {
		t.Fatal(err)
	}
	assertConfirmedRepairFreshValidationForTest(t, settled.Aggregate, fixture.closureID)
	if settled.Aggregate.Facts.RepairDirective == nil || settled.Aggregate.Facts.RepairDirective.Closure == nil ||
		!sameRuntimeTargetRefs(settled.Aggregate.Facts.RepairDirective.Closure.GroundedTargetRefs, surfaceTargets) {
		t.Fatalf("fresh-validation continuation lost the full confirmed repair surface: %+v", settled.Aggregate.Facts.RepairDirective)
	}
	if err := settled.Aggregate.Validate(); err != nil {
		t.Fatalf("completed selected repair subset did not survive fresh load: %v", err)
	}
}

func TestConfirmedRepairValidationOnlyBindingManifestRetainsSourceLineage(t *testing.T) {
	ctx := context.Background()
	authority := newConfirmedRepairValidationOnlyAuthorityFixture(t, []string{"main.go"})
	confirmed := authority.aggregate
	bindingID := confirmed.Facts.Focus.CoverageEvidenceBindings[0].ID

	pending, err := authority.controller.BeginCandidateAttemptForTest(
		ctx, confirmed.Facts.WorkspaceID, confirmed.Facts.RunID, 0, "confirmed-repair-validation-source-binding",
	)
	if err != nil {
		t.Fatalf("begin confirmed repair candidate: %v", err)
	}
	if len(pending.CandidateRequestSessions) == 0 {
		t.Fatal("confirmed repair candidate has no persisted request session")
	}
	session := pending.CandidateRequestSessions[len(pending.CandidateRequestSessions)-1]
	if session.CandidateManifest == nil || session.CandidateManifest.RepairPurpose != runtimecontract.RuntimeDispatchRepairPurposeConfirmedValidation {
		t.Fatalf("confirmed repair request manifest = %+v", session.CandidateManifest)
	}
	manifest := session.CandidateManifest.Normalized()
	if len(manifest.Operations) != 1 ||
		!reflect.DeepEqual(manifest.Operations[0].RequiredOperationFields, []string{"bindingRefs", "kind", "parameters", "targetRefs"}) {
		t.Fatalf("confirmed repair operation manifest = %+v", manifest.Operations)
	}
	for _, authorization := range manifest.Operations[0].BindingAuthorizations {
		if !reflect.DeepEqual(authorization.AllowedBindingRefs, []string{bindingID}) {
			t.Fatalf("confirmed repair binding authorization = %+v, want %q", authorization, bindingID)
		}
	}
	withoutBinding := []byte(`{"operations":[{"kind":"file/write","targetRefs":["main.go"],"parameters":[{"name":"content","string":"updated"}]}]}`)
	if _, err := runtimecontract.DecodeRuntimeCandidatePayloadWithManifest(manifest, withoutBinding); err == nil || !strings.Contains(err.Error(), "bindingRefs") {
		t.Fatalf("confirmed repair request accepted operation without source binding: %v", err)
	}

	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		Kind: runtimecontract.StepKindFileWrite, BindingRefs: []string{bindingID}, TargetRefs: []string{"main.go"},
		Parameters: runtimeFileWriteParameters("updated"),
	}}}}}
	if err := RecordFixtureCandidateFeasibilities(
		ctx, authority.controller, confirmed.Facts.WorkspaceID, confirmed.Facts.RunID, 0, candidate,
	); err != nil {
		t.Fatalf("record confirmed repair feasibility: %v", err)
	}
	sealed, err := authority.controller.AdmitAndSealCandidateAttemptForTest(
		ctx, confirmed.Facts.WorkspaceID, confirmed.Facts.RunID, 0, candidate,
	)
	if err != nil || !sealed.Admission.Accepted() || sealed.Aggregate.ActiveDispatch == nil ||
		sealed.Aggregate.ActiveDispatch.RepairCommitment == nil {
		t.Fatalf("confirmed repair admission = %+v err=%v", sealed, err)
	}
	required, err := confirmedRepairCommitmentRequiredByRejectedObservation(sealed.Aggregate, *sealed.Aggregate.ActiveDispatch)
	if err != nil || !required {
		t.Fatalf("confirmed repair source binding was not retained: required=%v err=%v", required, err)
	}
	if err := sealed.Aggregate.Validate(); err != nil {
		t.Fatalf("confirmed repair with validation-only source binding did not survive fresh load: %v", err)
	}
}

func TestConfirmedRepairCommitmentFreshLoadRejectsDeletionAndCoordinatedTamper(t *testing.T) {
	fixture := newConfirmedRepairMultiOperationFixture(t)
	coordinator, err := NewDispatchCoordinator(fixture.repository)
	if err != nil {
		t.Fatal(err)
	}
	first := fixture.acceptedEvidence(t, 0, "tree-v1", "tree-v2")
	attachConfirmedRepairReceiptForTest(t, &fixture.repository.aggregate, &first, "tree-v1", "tree-v2")
	if _, err := coordinator.commitEvidence(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	second := fixture.acceptedEvidence(t, 1, "tree-v2", "tree-v3")
	attachConfirmedRepairReceiptForTest(t, &fixture.repository.aggregate, &second, "tree-v2", "tree-v3")
	settled, err := coordinator.commitEvidence(context.Background(), second)
	if err != nil {
		t.Fatal(err)
	}
	if err := settled.Aggregate.Validate(); err != nil {
		t.Fatalf("settled confirmed owner repair baseline: %v", err)
	}

	dispatchIndex := -1
	for index := range settled.Aggregate.CompletedDispatches {
		if settled.Aggregate.CompletedDispatches[index].RepairCommitment != nil {
			dispatchIndex = index
		}
	}
	if dispatchIndex < 0 {
		t.Fatal("settled confirmed owner repair has no committed repair-purpose dispatch")
	}
	dispatchID := settled.Aggregate.CompletedDispatches[dispatchIndex].ID
	attemptIndex := -1
	sessionIndex := -1
	for index := range settled.Aggregate.CandidateAttempts {
		attempt := settled.Aggregate.CandidateAttempts[index]
		if attempt.State != runtimecontract.RuntimeCandidateAttemptStateAccepted || attempt.DispatchID != dispatchID {
			continue
		}
		attemptIndex = index
		for candidateSessionIndex := range settled.Aggregate.CandidateRequestSessions {
			if settled.Aggregate.CandidateRequestSessions[candidateSessionIndex].LogicalRequestID == attempt.LogicalRequestID {
				sessionIndex = candidateSessionIndex
			}
		}
	}
	if attemptIndex < 0 || sessionIndex < 0 {
		t.Fatalf("settled confirmed owner repair has no accepted pre-seal lineage: attempt=%d session=%d", attemptIndex, sessionIndex)
	}

	tests := []struct {
		name      string
		mutate    func(*RuntimeAggregate)
		precheck  func(*testing.T, *RuntimeAggregate)
		wantError string
	}{
		{
			name: "dispatch commitment deleted",
			mutate: func(value *RuntimeAggregate) {
				dispatch := &value.CompletedDispatches[dispatchIndex]
				dispatch.RepairCommitment = nil
				dispatch.RepairCommitmentHash = ""
				dispatch.DispatchAuthorityHash = runtimecontract.StableRuntimeDispatchAuthorityHash(sealedDispatchToRuntimeRecord(*dispatch))
				value.CandidateAttempts[attemptIndex].DispatchAuthorityHash = dispatch.DispatchAuthorityHash
			},
			wantError: "no exact sealed dispatch target-authorization projection",
		},
		{
			name: "confirmation deleted",
			mutate: func(value *RuntimeAggregate) {
				value.RepairSurfaceConfirmations = nil
			},
			wantError: "has no unique exact owner confirmation",
		},
		{
			name: "fresh validation directive deleted",
			mutate: func(value *RuntimeAggregate) {
				value.Facts.RepairDirective = nil
			},
			wantError: "does not exactly match its completed transaction",
		},
		{
			name: "confirmation and directive deleted",
			mutate: func(value *RuntimeAggregate) {
				value.RepairSurfaceConfirmations = nil
				value.Facts.RepairDirective = nil
			},
			wantError: "has no unique exact owner confirmation",
		},
		{
			name: "all commitment copies confirmation and directive deleted",
			mutate: func(value *RuntimeAggregate) {
				dispatch := &value.CompletedDispatches[dispatchIndex]
				dispatch.RepairCommitment = nil
				dispatch.RepairCommitmentHash = ""
				dispatch.DispatchAuthorityHash = runtimecontract.StableRuntimeDispatchAuthorityHash(sealedDispatchToRuntimeRecord(*dispatch))
				attempt := &value.CandidateAttempts[attemptIndex]
				attempt.AdmissionRepairCommitment = nil
				attempt.RepairCommitmentHash = ""
				attempt.DispatchAuthorityHash = dispatch.DispatchAuthorityHash
				session := &value.CandidateRequestSessions[sessionIndex]
				session.RepairCommitment = nil
				session.RepairCommitmentHash = ""
				value.RepairSurfaceConfirmations = nil
				value.Facts.RepairDirective = nil
			},
			wantError: "omitted its manifest-required repair commitment",
		},
		{
			name: "commitment marker copies confirmation and directive coordinated deletion",
			mutate: func(value *RuntimeAggregate) {
				dispatch := &value.CompletedDispatches[dispatchIndex]
				dispatch.RepairCommitment = nil
				dispatch.RepairCommitmentHash = ""
				attempt := &value.CandidateAttempts[attemptIndex]
				attempt.AdmissionRepairCommitment = nil
				attempt.RepairCommitmentHash = ""
				session := &value.CandidateRequestSessions[sessionIndex]
				session.RepairCommitment = nil
				session.RepairCommitmentHash = ""
				manifest := session.CandidateManifest.Clone()
				manifest.RepairPurpose = ""
				manifest.RepairClosureID = ""
				manifest = manifest.Normalized()
				manifest.Hash = runtimecontract.StableRuntimeCandidateContractManifestHash(manifest)
				session.CandidateManifest = &manifest
				session.ManifestHash = manifest.Hash
				session.LogicalRequestID = runtimecontract.StableRuntimeCandidateRequestSessionID(
					session.DocumentKind, value.Facts.WorkspaceID, value.Facts.RunID, session.Transaction,
					session.StablePrefixHash, session.ManifestHash, session.CorrectionHash,
				)
				if session.ExpectedDocument != nil {
					expected := session.ExpectedDocument.Clone()
					expected.ManifestHash = manifest.Hash
					expected.LogicalRequestID = session.LogicalRequestID
					expected.Hash = runtimecontract.StableRuntimeExpectedDocumentHash(expected)
					session.ExpectedDocument = &expected
					if session.LastDocumentFraming != nil {
						framing := *session.LastDocumentFraming
						framing.ExpectedDocumentHash = expected.Hash
						session.LastDocumentFraming = &framing
					}
				}
				session.LastPhysicalCallID = runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, 1)
				attempt.ManifestHash = manifest.Hash
				attempt.LogicalRequestID = session.LogicalRequestID
				dispatch.CandidateManifestHash = manifest.Hash
				dispatch.DispatchAuthorityHash = runtimecontract.StableRuntimeDispatchAuthorityHash(sealedDispatchToRuntimeRecord(*dispatch))
				attempt.DispatchAuthorityHash = dispatch.DispatchAuthorityHash
				value.RepairSurfaceConfirmations = nil
				value.Facts.RepairDirective = nil
			},
			precheck: func(t *testing.T, value *RuntimeAggregate) {
				t.Helper()
				if err := validateConfirmedRepairMandatoryCommitments(*value); err == nil ||
					!strings.Contains(err.Error(), "omitted its mandatory rejected-validation commitment") {
					t.Fatalf("marker-independent mandatory commitment detector error = %v", err)
				}
			},
			// Full aggregate validation may reject the coordinated manifest rewrite
			// even earlier because manifest-derived operation IDs also change. The
			// precheck above independently proves that deleting the marker cannot
			// make the rejected-validation repair purpose disappear.
			wantError: "candidate/manifest mismatch",
		},
		{
			name: "all commitment copies coordinate source hash drift",
			mutate: func(value *RuntimeAggregate) {
				commitment := value.CompletedDispatches[dispatchIndex].RepairCommitment.Clone()
				commitment.SourceEvidenceFactHash = strings.Repeat("f", 64)
				commitmentHash := runtimecontract.StableRuntimeDispatchRepairCommitmentHash(commitment)
				dispatch := &value.CompletedDispatches[dispatchIndex]
				dispatch.RepairCommitment = &commitment
				dispatch.RepairCommitmentHash = commitmentHash
				dispatch.DispatchAuthorityHash = runtimecontract.StableRuntimeDispatchAuthorityHash(sealedDispatchToRuntimeRecord(*dispatch))
				attempt := &value.CandidateAttempts[attemptIndex]
				attemptCommitment := commitment.Clone()
				attempt.AdmissionRepairCommitment = &attemptCommitment
				attempt.RepairCommitmentHash = commitmentHash
				attempt.DispatchAuthorityHash = dispatch.DispatchAuthorityHash
				sessionCommitment := commitment.Clone()
				session := &value.CandidateRequestSessions[sessionIndex]
				session.RepairCommitment = &sessionCommitment
				session.RepairCommitmentHash = commitmentHash
			},
			wantError: "commitment source lineage mismatch",
		},
		{
			name: "dispatch commitment raw order laundering",
			mutate: func(value *RuntimeAggregate) {
				dispatch := &value.CompletedDispatches[dispatchIndex]
				commitment := *dispatch.RepairCommitment
				commitment.Closure.CoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), commitment.Closure.CoverageWorkItemIDs...)
				commitment.Closure.RequiredEvidenceKinds = append([]string(nil), commitment.Closure.RequiredEvidenceKinds...)
				commitment.Closure.GroundedTargetRefs = append([]string(nil), commitment.Closure.GroundedTargetRefs...)
				for left, right := 0, len(commitment.Closure.RequiredEvidenceKinds)-1; left < right; left, right = left+1, right-1 {
					commitment.Closure.RequiredEvidenceKinds[left], commitment.Closure.RequiredEvidenceKinds[right] =
						commitment.Closure.RequiredEvidenceKinds[right], commitment.Closure.RequiredEvidenceKinds[left]
				}
				dispatch.RepairCommitment = &commitment
				dispatch.RepairCommitmentHash = runtimecontract.StableRuntimeDispatchRepairCommitmentHash(commitment)
				dispatch.DispatchAuthorityHash = runtimecontract.StableRuntimeDispatchAuthorityHash(sealedDispatchToRuntimeRecord(*dispatch))
				value.CandidateAttempts[attemptIndex].DispatchAuthorityHash = dispatch.DispatchAuthorityHash
			},
			wantError: "repair commitment projection is not raw-canonical",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tampered := cloneRuntimeAggregateForMCPProvenanceTest(t, settled.Aggregate)
			test.mutate(&tampered)
			if test.precheck != nil {
				test.precheck(t, &tampered)
			}
			if err := tampered.Validate(); err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("confirmed owner repair commitment tamper error = %v", err)
			}
		})
	}
}

func TestAcceptedConfirmedRepairManifestLineageRejectsMissingDuplicateAndBodyDrift(t *testing.T) {
	fixture := newConfirmedRepairSingleOperationFixture(t)
	aggregate := cloneRuntimeAggregateForMCPProvenanceTest(t, fixture.repository.aggregate)
	if aggregate.ActiveDispatch == nil {
		t.Fatal("confirmed owner repair manifest fixture has no active dispatch")
	}
	activeRecord := sealedDispatchToRuntimeRecord(*aggregate.ActiveDispatch)
	completedRecords := sealedDispatchesToRuntimeRecords(aggregate.CompletedDispatches)
	if err := runtimecontract.ValidateRuntimeAcceptedCandidateManifestLineage(
		&activeRecord, completedRecords, aggregate.CandidateAttempts, aggregate.CandidateRequestSessions,
	); err != nil {
		t.Fatalf("confirmed owner repair manifest baseline: %v", err)
	}

	logicalRequestID := ""
	for _, attempt := range aggregate.CandidateAttempts {
		if attempt.State == runtimecontract.RuntimeCandidateAttemptStateAccepted && attempt.DispatchID == activeRecord.ID {
			logicalRequestID = attempt.LogicalRequestID
		}
	}
	if logicalRequestID == "" {
		t.Fatal("confirmed owner repair manifest fixture has no accepted request identity")
	}
	sessionIndex := -1
	for index := range aggregate.CandidateRequestSessions {
		if aggregate.CandidateRequestSessions[index].LogicalRequestID == logicalRequestID {
			sessionIndex = index
		}
	}
	if sessionIndex < 0 || aggregate.CandidateRequestSessions[sessionIndex].CandidateManifest == nil {
		t.Fatal("confirmed owner repair manifest fixture has no persisted body")
	}

	for _, test := range []struct {
		name      string
		mutate    func(*RuntimeAggregate)
		wantError string
	}{
		{
			name: "missing session",
			mutate: func(value *RuntimeAggregate) {
				kept := value.CandidateRequestSessions[:0]
				for _, session := range value.CandidateRequestSessions {
					if session.LogicalRequestID != logicalRequestID {
						kept = append(kept, session)
					}
				}
				value.CandidateRequestSessions = kept
			},
			wantError: "has no exact pre-seal operations manifest",
		},
		{
			name: "duplicate session",
			mutate: func(value *RuntimeAggregate) {
				value.CandidateRequestSessions = append(value.CandidateRequestSessions, value.CandidateRequestSessions[sessionIndex])
			},
			wantError: "has no exact pre-seal operations manifest",
		},
		{
			name: "missing manifest body",
			mutate: func(value *RuntimeAggregate) {
				value.CandidateRequestSessions[sessionIndex].CandidateManifest = nil
			},
			wantError: "has no exact pre-seal operations manifest",
		},
		{
			name: "canonical body drift",
			mutate: func(value *RuntimeAggregate) {
				manifest := value.CandidateRequestSessions[sessionIndex].CandidateManifest.Clone()
				manifest.RepairClosureID = "repair-closure-coordinated-drift"
				manifest.Hash = runtimecontract.StableRuntimeCandidateContractManifestHash(manifest)
				value.CandidateRequestSessions[sessionIndex].CandidateManifest = &manifest
			},
			wantError: "manifest body/hash mismatch",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			tampered := cloneRuntimeAggregateForMCPProvenanceTest(t, aggregate)
			test.mutate(&tampered)
			freshActive := sealedDispatchToRuntimeRecord(*tampered.ActiveDispatch)
			if err := runtimecontract.ValidateRuntimeAcceptedCandidateManifestLineage(
				&freshActive, sealedDispatchesToRuntimeRecords(tampered.CompletedDispatches), tampered.CandidateAttempts, tampered.CandidateRequestSessions,
			); err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("accepted manifest lineage tamper error = %v", err)
			}
		})
	}
}

func TestConfirmedRepairSingleOperationGovernanceContinueSurvivesRestartAndDuplicate(t *testing.T) {
	fixture := newConfirmedRepairSingleOperationFixture(t)
	governance := advanceConfirmedRepairOperationGovernanceForTest(
		t, fixture.repository, 0, "single", runtimecontract.RuntimeGovernanceResolutionOutcomeContinue,
	)
	if governance.Aggregate.ActiveDispatch == nil {
		t.Fatal("single-operation File repair governance continuation has no resume dispatch")
	}
	controller, err := NewRuntimeController(fixture.repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := controller.candidateDispatch.markGovernanceResumeExecutionStarted(
		context.Background(), governance.Aggregate.Facts.WorkspaceID, governance.Aggregate.Facts.RunID, governance.operationID,
	); err != nil {
		t.Fatalf("start single-operation File repair governance resume: %v", err)
	}
	evidence := fixture.acceptedEvidence(t, 0, "tree-v1", "tree-v2")
	attachConfirmedRepairReceiptForTest(t, &fixture.repository.aggregate, &evidence, "tree-v1", "tree-v2")
	coordinator, err := NewDispatchCoordinator(fixture.repository)
	if err != nil {
		t.Fatal(err)
	}
	settled, err := coordinator.commitEvidence(context.Background(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	assertConfirmedRepairFreshValidationForTest(t, settled.Aggregate, fixture.closureID)
	if err := settled.Aggregate.Validate(); err != nil {
		t.Fatalf("single-operation governed File repair did not survive fresh load: %v", err)
	}

	restarted, err := NewRuntimeController(fixture.repository)
	if err != nil {
		t.Fatal(err)
	}
	duplicateResolution, err := restarted.commitGovernanceResolution(context.Background(), governance.resolution)
	if err != nil || duplicateResolution.Disposition != CommitDispositionDuplicate ||
		duplicateResolution.Aggregate.StateRevision != settled.Aggregate.StateRevision {
		t.Fatalf("single-operation governance resolution replay changed settled repair: result=%+v err=%v", duplicateResolution, err)
	}
	restartedCoordinator, err := NewDispatchCoordinator(fixture.repository)
	if err != nil {
		t.Fatal(err)
	}
	duplicateEvidence, err := restartedCoordinator.commitEvidence(context.Background(), evidence)
	if err != nil || duplicateEvidence.Disposition != CommitDispositionDuplicate ||
		duplicateEvidence.Aggregate.StateRevision != settled.Aggregate.StateRevision {
		t.Fatalf("single-operation governed receipt replay changed settled repair: result=%+v err=%v", duplicateEvidence, err)
	}
}

func TestConfirmedRepairMultiOperationGovernanceContinueOrdersAcceptedAndResolutionEvidence(t *testing.T) {
	fixture := newConfirmedRepairMultiOperationFixture(t)
	coordinator, err := NewDispatchCoordinator(fixture.repository)
	if err != nil {
		t.Fatal(err)
	}
	first := fixture.acceptedEvidence(t, 0, "tree-v1", "tree-v2")
	attachConfirmedRepairReceiptForTest(t, &fixture.repository.aggregate, &first, "tree-v1", "tree-v2")
	partial, err := coordinator.commitEvidence(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	if err := partial.Aggregate.Validate(); err != nil {
		t.Fatalf("pre-governance partial File repair did not survive fresh load: %v", err)
	}
	acceptedAttemptCount := len(partial.Aggregate.CandidateAttempts)

	governance := advanceConfirmedRepairOperationGovernanceForTest(
		t, fixture.repository, 1, "multi-second", runtimecontract.RuntimeGovernanceResolutionOutcomeContinue,
	)
	if governance.Aggregate.ActiveDispatch == nil {
		t.Fatal("multi-operation File repair governance continuation has no resume dispatch")
	}
	resume := governance.Aggregate.ActiveDispatch
	var source *SealedDispatch
	for _, binding := range governance.Aggregate.OperationGovernanceBindings {
		binding = binding.Normalized()
		if binding.OperationID != governance.operationID || binding.Grant == nil || binding.Grant.ResumeDispatchID != resume.ID {
			continue
		}
		for index := range governance.Aggregate.CompletedDispatches {
			if governance.Aggregate.CompletedDispatches[index].ID == binding.OperationDispatchID {
				source = &governance.Aggregate.CompletedDispatches[index]
			}
		}
	}
	if source == nil {
		t.Fatal("multi-operation File repair governance resume has no exact source dispatch")
	}
	if len(governance.Aggregate.CandidateAttempts) != acceptedAttemptCount ||
		!reflect.DeepEqual(resume.Candidate, source.Candidate) || resume.CandidateHash != source.CandidateHash ||
		resume.CandidateManifestHash != source.CandidateManifestHash ||
		!runtimecontract.RuntimeCoverageEvidenceBindingProjectionsEqual(resume.CoverageBindings, source.CoverageBindings) ||
		!runtimecontract.RuntimeDispatchOperationTargetAuthorizationProjectionsEqual(resume.OperationTargetAuthorizations, source.OperationTargetAuthorizations) ||
		!runtimecontract.RuntimeValidationRecipeProjectionsEqual(resume.ValidationRecipes, source.ValidationRecipes) ||
		governance.Aggregate.Decision.State != runtimecontract.ConvergenceStateNeedStateChange ||
		resume.Transaction != governance.Aggregate.Decision.AuthorityTransaction() ||
		governance.Aggregate.Facts.Resources.FailureTarget != nil || governance.Aggregate.Facts.RepairDirective != nil {
		t.Fatalf("File governance continuation selected or restored authority outside its source seal: source=%+v resume=%+v decision=%+v", source, resume, governance.Aggregate.Decision)
	}
	clonedFirst, found := operationExecutionRecord(governance.Aggregate.ActiveDispatch.OperationExecutions, first.OperationID)
	if !found || clonedFirst.State != runtimecontract.OperationExecutionEvidenceCommitted ||
		!containsString(governance.Aggregate.ActiveDispatch.CompletedOperationIDs, first.OperationID) {
		t.Fatalf("governance resume lost the first completed File member: %+v", governance.Aggregate.ActiveDispatch)
	}
	// Exercise the crash boundary immediately after the governance-resolution
	// CAS. Recovery must load the reducer-produced decision and resume dispatch;
	// it must not need the invocation-local owner event or reconstruct a mutable
	// FailureTarget/RepairDirective.
	fresh := governanceResumeAggregateJSONRoundTrip(t, governance.Aggregate)
	if err := fresh.Validate(); err != nil {
		t.Fatalf("fresh-load governance continuation: %v", err)
	}
	fixture.repository.aggregate = fresh
	controller, err := NewRuntimeController(fixture.repository)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := controller.Load(context.Background(), fresh.Facts.WorkspaceID, fresh.Facts.RunID)
	if err != nil || loaded.ActiveDispatch == nil || loaded.Decision.State != runtimecontract.ConvergenceStateNeedStateChange ||
		loaded.ActiveDispatch.Transaction != loaded.Decision.AuthorityTransaction() ||
		loaded.Facts.Resources.FailureTarget != nil || loaded.Facts.RepairDirective != nil {
		t.Fatalf("restarted governance continuation lost reducer authority: aggregate=%+v err=%v", loaded, err)
	}
	if _, _, err := controller.candidateDispatch.markGovernanceResumeExecutionStarted(
		context.Background(), governance.Aggregate.Facts.WorkspaceID, governance.Aggregate.Facts.RunID, governance.operationID,
	); err != nil {
		t.Fatalf("start second File repair governance resume: %v", err)
	}
	second := fixture.acceptedEvidence(t, 1, "tree-v2", "tree-v3")
	attachConfirmedRepairReceiptForTest(t, &fixture.repository.aggregate, &second, "tree-v2", "tree-v3")
	restartedCoordinator, err := NewDispatchCoordinator(fixture.repository)
	if err != nil {
		t.Fatal(err)
	}
	settled, err := restartedCoordinator.commitEvidence(context.Background(), second)
	if err != nil {
		t.Fatal(err)
	}
	assertConfirmedRepairFreshValidationForTest(t, settled.Aggregate, fixture.closureID)
	if err := settled.Aggregate.Validate(); err != nil {
		t.Fatalf("multi-operation governed File repair did not survive fresh load: %v", err)
	}

	tampered := cloneRuntimeAggregateForMCPProvenanceTest(t, settled.Aggregate)
	foundGovernanceEvidence := false
	for index := range tampered.Facts.Evidence.Accepted {
		fact := &tampered.Facts.Evidence.Accepted[index]
		if fact.Kind != runtimecontract.EvidenceKindGovernance || fact.DispatchID != governance.resolution.DispatchID {
			continue
		}
		fact.Revision = "runtime-evidence-99999999999999999999"
		foundGovernanceEvidence = true
	}
	if !foundGovernanceEvidence {
		t.Fatal("governed File repair fixture has no accepted governance evidence")
	}
	if err := validateConfirmedRepairFreshValidationContinuation(tampered); err == nil ||
		!strings.Contains(err.Error(), "evidence revision gap has no exact governance-resolution lineage") {
		t.Fatalf("coordinated governance revision drift error = %v", err)
	}

	restarted, err := NewRuntimeController(fixture.repository)
	if err != nil {
		t.Fatal(err)
	}
	duplicateResolution, err := restarted.commitGovernanceResolution(context.Background(), governance.resolution)
	if err != nil || duplicateResolution.Disposition != CommitDispositionDuplicate ||
		duplicateResolution.Aggregate.StateRevision != settled.Aggregate.StateRevision {
		t.Fatalf("multi-operation governance resolution replay changed settled repair: result=%+v err=%v", duplicateResolution, err)
	}
	duplicateEvidence, err := restartedCoordinator.commitEvidence(context.Background(), second)
	if err != nil || duplicateEvidence.Disposition != CommitDispositionDuplicate ||
		duplicateEvidence.Aggregate.StateRevision != settled.Aggregate.StateRevision {
		t.Fatalf("multi-operation governed receipt replay changed settled repair: result=%+v err=%v", duplicateEvidence, err)
	}
}

func TestReduceRuntimeStateWithDirectiveConsumesGovernanceContinuationOwnerEvent(t *testing.T) {
	fixture := newConfirmedRepairMultiOperationFixture(t)
	coordinator, err := NewDispatchCoordinator(fixture.repository)
	if err != nil {
		t.Fatal(err)
	}
	first := fixture.acceptedEvidence(t, 0, "tree-v1", "tree-v2")
	attachConfirmedRepairReceiptForTest(t, &fixture.repository.aggregate, &first, "tree-v1", "tree-v2")
	if _, err := coordinator.commitEvidence(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	governance := advanceConfirmedRepairOperationGovernanceForTest(
		t, fixture.repository, 1, "sole-reducer", runtimecontract.RuntimeGovernanceResolutionOutcomeContinue,
	)

	var source SealedDispatch
	var binding runtimecontract.RuntimeOperationGovernanceBinding
	found := false
	for _, rawBinding := range governance.Aggregate.OperationGovernanceBindings {
		candidate := rawBinding.Clone().Normalized()
		if candidate.OperationID != governance.operationID || candidate.Grant == nil {
			continue
		}
		binding = candidate
		for _, completed := range governance.Aggregate.CompletedDispatches {
			if completed.ID == binding.OperationDispatchID {
				source = completed
				found = true
			}
		}
	}
	if !found {
		t.Fatal("governance continuation fixture has no exact source dispatch/binding")
	}
	// ResumeDispatchID is the output of the same CAS and is intentionally absent
	// from the reducer input. The input contains only the accepted owner result,
	// source seal and pre-resume granted fence.
	binding.Grant.ResumeDispatchID = ""
	event := &runtimeOwnerInternalReductionEvent{
		Kind:                 RuntimeInternalEventGovernanceResolutionObserved,
		GovernanceResolution: governance.resolution,
		SourceDispatch:       source,
		OperationBinding:     binding,
	}
	facts, decision, err := reduceRuntimeStateWithDirectiveAndOwnerEvent(
		governance.Aggregate.Facts, nil, event, governance.Aggregate.CoverageProofLineage,
	)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != runtimecontract.ConvergenceStateNeedStateChange ||
		runtimecontract.StableRequiredOutputHash(decision.RequiredOutput) != runtimecontract.StableRequiredOutputHash(governance.Aggregate.Decision.RequiredOutput) ||
		facts.Resources.FailureTarget != nil || facts.RepairDirective != nil {
		t.Fatalf("owner event did not reduce through canonical continuation: decision=%+v facts=%+v", decision, facts)
	}
	_, ordinary, err := ReduceRuntimeStateWithDirective(
		governance.Aggregate.Facts, nil, governance.Aggregate.CoverageProofLineage)

	if err != nil {
		t.Fatal(err)
	}
	if ordinary.State == decision.State &&
		runtimecontract.StableRequiredOutputHash(ordinary.RequiredOutput) == runtimecontract.StableRequiredOutputHash(decision.RequiredOutput) {
		t.Fatalf("sealed continuation was reconstructed without its owner-internal event: %+v", ordinary)
	}
}

func TestConfirmedRepairGovernanceFailCannotCreateFreshValidationAuthority(t *testing.T) {
	fixture := newConfirmedRepairSingleOperationFixture(t)
	governance := advanceConfirmedRepairOperationGovernanceForTest(
		t, fixture.repository, 0, "fail", runtimecontract.RuntimeGovernanceResolutionOutcomeFail,
	)
	if governance.Aggregate.ActiveDispatch != nil || governance.Aggregate.Facts.RepairDirective != nil ||
		governance.Aggregate.Facts.Failure == nil ||
		governance.Aggregate.Facts.Failure.Class != runtimecontract.FailureClassGovernanceRejected {
		t.Fatalf("failed File governance resolution created repair continuation authority: %+v", governance.Aggregate)
	}
	completed, err := completedConfirmedRepairTransactions(governance.Aggregate)
	if err != nil {
		t.Fatalf("classify failed File governance transaction: %v", err)
	}
	if len(completed) != 0 {
		t.Fatalf("failed File governance transaction was promoted as completed repair: %+v", completed)
	}
	if err := governance.Aggregate.Validate(); err != nil {
		t.Fatalf("failed File governance resolution did not survive fresh load: %v", err)
	}

	restarted, err := NewRuntimeController(fixture.repository)
	if err != nil {
		t.Fatal(err)
	}
	revision := governance.Aggregate.StateRevision
	duplicate, err := restarted.commitGovernanceResolution(context.Background(), governance.resolution)
	if err != nil || duplicate.Disposition != CommitDispositionDuplicate || duplicate.Aggregate.StateRevision != revision ||
		duplicate.Aggregate.Facts.RepairDirective != nil {
		t.Fatalf("failed File governance resolution replay created continuation: result=%+v err=%v", duplicate, err)
	}
}

func TestConfirmedRepairMultiOperationRejectedMemberRetiresTransientAuthority(t *testing.T) {
	fixture := newConfirmedRepairMultiOperationFixture(t)
	coordinator, err := NewDispatchCoordinator(fixture.repository)
	if err != nil {
		t.Fatal(err)
	}
	first := fixture.acceptedEvidence(t, 0, "tree-v1", "tree-v2")
	attachConfirmedRepairReceiptForTest(t, &fixture.repository.aggregate, &first, "tree-v1", "tree-v2")
	partial, err := coordinator.commitEvidence(context.Background(), first)
	if err != nil || partial.Aggregate.Facts.Resources.FailureTarget != nil || partial.Aggregate.Facts.RepairDirective != nil {
		t.Fatalf("first repair member retained superseded mutable authority: result=%+v err=%v", partial, err)
	}

	dispatch := *fixture.repository.aggregate.ActiveDispatch
	ordered, err := orderedCandidateOperations(dispatch.Candidate)
	if err != nil {
		t.Fatal(err)
	}
	var operation runtimecontract.RuntimeOperation
	for _, candidate := range ordered {
		if !containsString(dispatch.CompletedOperationIDs, candidate.ID) {
			operation = candidate.Normalized()
			break
		}
	}
	if operation.ID == "" {
		t.Fatal("partial File repair dispatch has no uncompleted member to reject")
	}
	rejected := ExecutionEvidence{
		EventID:     "runtime-operation|" + dispatch.ID + "|" + operation.ID,
		WorkspaceID: fixture.repository.aggregate.Facts.WorkspaceID, RunID: fixture.repository.aggregate.Facts.RunID,
		DispatchID: dispatch.ID, AuthoritySnapshotID: dispatch.Transaction.AuthoritySnapshotID,
		OperationID: operation.ID, Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusRejected,
		SourceRef: "workorch://runtime-operations/" + dispatch.ID + "/" + operation.ID,
		Failure: &runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassExecutionFailure, Code: "repair_member_rejected",
			Signature: "repair-member-rejected", Message: "File owner rejected the second repair member",
		},
	}
	attachRuntimeTestOwnerReceipt(t, &fixture.repository.aggregate, &rejected)
	completed, err := coordinator.commitEvidence(context.Background(), rejected)
	if err != nil {
		t.Fatal(err)
	}
	if !completed.DispatchCompleted || completed.Aggregate.ActiveDispatch != nil {
		t.Fatalf("rejected all-of member did not close dispatch: %+v", completed.Aggregate)
	}
	if target := completed.Aggregate.Facts.Resources.FailureTarget; target != nil && target.TargetObservation != nil &&
		target.TargetObservation.ObservedRevision == "tree-v1" {
		t.Fatalf("rejected all-of member retained confirmed-revision FailureTarget: %+v", target)
	}
	if directive := completed.Aggregate.Facts.RepairDirective; directive != nil && directive.Closure != nil &&
		directive.Closure.ID == fixture.closureID && directive.RequiredState == runtimecontract.ConvergenceStateNeedStateChange {
		t.Fatalf("rejected all-of member retained stale repair directive: %+v", directive)
	}
	if err := completed.Aggregate.Validate(); err != nil {
		t.Fatalf("rejected all-of completion did not survive fresh load: %v", err)
	}
}

func TestConfirmedRepairMultiOperationRejectsSecondReceiptFromOriginalRevision(t *testing.T) {
	fixture := newConfirmedRepairMultiOperationFixture(t)
	coordinator, err := NewDispatchCoordinator(fixture.repository)
	if err != nil {
		t.Fatal(err)
	}
	first := fixture.acceptedEvidence(t, 0, "tree-v1", "tree-v2")
	attachConfirmedRepairReceiptForTest(t, &fixture.repository.aggregate, &first, "tree-v1", "tree-v2")
	if _, err := coordinator.commitEvidence(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	second := fixture.acceptedEvidence(t, 1, "tree-v1", "tree-v3")
	attachConfirmedRepairReceiptForTest(t, &fixture.repository.aggregate, &second, "tree-v1", "tree-v3")
	if _, err := coordinator.commitEvidence(context.Background(), second); err == nil ||
		!strings.Contains(err.Error(), "does not continue the completed receipt chain") {
		t.Fatalf("second receipt reusing original confirmation revision error = %v", err)
	}
}

func TestPendingConfirmedRepairTransactionDoesNotMatchFreshValidationPartialDispatch(t *testing.T) {
	observation := runtimecontract.RuntimeValidationFailureObservation{
		WorkspaceID: "ws-1", RunID: "run-1", AuthoritySnapshotID: "authority-validation",
		DispatchID: "dispatch-validation-source", OperationID: "validate-source",
		FailureSignature: "validation-failure", TargetRole: "implementation",
		DiagnosticClass:     runtimecontract.ValidationDiagnosticBuildFailed,
		CandidateTargetRefs: []string{"main.go"}, ValidationScopeTargetRefs: []string{"main.go"}, DiagnosticHash: strings.Repeat("5", 64),
	}
	closure := runtimecontract.RuntimeRepairClosure{ID: "closure-validation", RequireValidationAfterStateChange: true}
	failure := runtimecontract.FailureFact{Signature: observation.FailureSignature, ValidationFailureObservation: &observation}
	directive := runtimecontract.RuntimeRepairDirective{
		Trigger: runtimecontract.RuntimeRepairTriggerRejectedValidation, RequiredState: runtimecontract.ConvergenceStateNeedValidation,
		CurrentFailure: &failure, Closure: &closure,
	}
	aggregate := RuntimeAggregate{
		Facts: runtimecontract.RuntimeFacts{RepairDirective: &directive},
		Decision: runtimecontract.RuntimeDecision{
			State:          runtimecontract.ConvergenceStateNeedValidation,
			RequiredOutput: runtimecontract.RequiredOutputContract{RepairClosureID: closure.ID},
		},
		ActiveDispatch: &SealedDispatch{
			State: DispatchStateSealed,
			Candidate: runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{
				{Operation: runtimecontract.RuntimeOperation{ID: "validate-1"}},
				{Operation: runtimecontract.RuntimeOperation{ID: "validate-2"}},
			}}},
			CompletedOperationIDs: []string{"validate-1"},
		},
	}
	if err := validatePendingConfirmedRepairTransaction(aggregate); err != nil {
		t.Fatalf("fresh-validation partial dispatch was classified as confirmed owner repair transaction: %v", err)
	}
}

func TestRefreshExhaustedConfirmedRepairKeepsMutableAuthorityRetired(t *testing.T) {
	fixture := newConfirmedRepairMultiOperationFixture(t)
	coordinator, err := NewDispatchCoordinator(fixture.repository)
	if err != nil {
		t.Fatal(err)
	}
	first := fixture.acceptedEvidence(t, 0, "tree-v1", "tree-v2")
	attachConfirmedRepairReceiptForTest(t, &fixture.repository.aggregate, &first, "tree-v1", "tree-v2")
	partial, err := coordinator.commitEvidence(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	transaction, found, err := resolveConfirmedRepairTransaction(partial.Aggregate, *partial.Aggregate.ActiveDispatch)
	if err != nil || !found || transaction == nil {
		t.Fatalf("partial repair has no exact immutable transaction: transaction=%+v found=%v err=%v", transaction, found, err)
	}
	invalidated := cloneRuntimeAggregateForMCPProvenanceTest(t, partial.Aggregate)
	invalidated.Facts.Resources.FailureTarget = nil
	invalidated.Facts.RepairDirective = nil
	invalidated.Facts.Failure = &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassPrerequisiteRefreshExhausted,
		Code:  "prerequisite_refresh_exhausted", Signature: "refresh-exhausted", Message: "refresh budget exhausted",
	}
	dispatchInvalidated := evidenceDispatchInvalidated(invalidated, ExecutionEvidence{Status: ExecutionEvidenceStatusAccepted}, true)
	if !dispatchInvalidated {
		t.Fatal("refresh-exhausted receipt was not classified as dispatch invalidation")
	}
	if invalidated.Facts.Resources.FailureTarget != nil || invalidated.Facts.RepairDirective != nil {
		t.Fatalf("refresh-exhausted dispatch retained stale repair authority: %+v", invalidated.Facts)
	}
}

func TestPendingConfirmedRepairTransactionRejectsBrokenReceiptEvidenceAndRevisionChain(t *testing.T) {
	fixture := newConfirmedRepairMultiOperationFixture(t)
	coordinator, err := NewDispatchCoordinator(fixture.repository)
	if err != nil {
		t.Fatal(err)
	}
	first := fixture.acceptedEvidence(t, 0, "tree-v1", "tree-v2")
	attachConfirmedRepairReceiptForTest(t, &fixture.repository.aggregate, &first, "tree-v1", "tree-v2")
	partial, err := coordinator.commitEvidence(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		mutate    func(*RuntimeAggregate)
		wantError string
	}{
		{
			name: "missing processed event",
			mutate: func(value *RuntimeAggregate) {
				operationID := value.ActiveDispatch.CompletedOperationIDs[0]
				eventID := ""
				for _, fact := range value.Facts.Evidence.Accepted {
					if fact.DispatchID == value.ActiveDispatch.ID && fact.OperationID == operationID {
						eventID = string(fact.ID)
					}
				}
				kept := make([]string, 0, len(value.ProcessedEventIDs))
				for _, candidate := range value.ProcessedEventIDs {
					if candidate != eventID {
						kept = append(kept, candidate)
					}
				}
				value.ProcessedEventIDs = kept
			},
			wantError: "receipt-derived accepted evidence fact and processed-event fence",
		},
		{
			name: "drifted accepted payload",
			mutate: func(value *RuntimeAggregate) {
				operationID := value.ActiveDispatch.CompletedOperationIDs[0]
				var fact *runtimecontract.EvidenceFact
				for index := range value.Facts.Evidence.Accepted {
					candidate := &value.Facts.Evidence.Accepted[index]
					if candidate.DispatchID == value.ActiveDispatch.ID && candidate.OperationID == operationID {
						fact = candidate
						break
					}
				}
				if fact == nil {
					t.Fatal("pending confirmed owner repair fixture has no accepted fact for its completed member")
				}
				fact.ContentHash = "forged-evidence-payload"
				for index := range value.ActiveDispatch.OperationExecutions {
					execution := &value.ActiveDispatch.OperationExecutions[index]
					if execution.OperationID == fact.OperationID {
						execution.EvidenceFactHash = runtimecontract.StableRuntimeEvidenceFactHash(*fact)
					}
				}
			},
			wantError: "accepted evidence does not match its exact receipt",
		},
		{
			name: "partial evidence revision drift",
			mutate: func(value *RuntimeAggregate) {
				operationID := value.ActiveDispatch.CompletedOperationIDs[0]
				for index := range value.Facts.Evidence.Accepted {
					if value.Facts.Evidence.Accepted[index].DispatchID == value.ActiveDispatch.ID &&
						value.Facts.Evidence.Accepted[index].OperationID == operationID {
						fact := &value.Facts.Evidence.Accepted[index]
						fact.Revision = "runtime-evidence-99999999999999999999"
						for executionIndex := range value.ActiveDispatch.OperationExecutions {
							execution := &value.ActiveDispatch.OperationExecutions[executionIndex]
							if execution.OperationID == operationID {
								execution.EvidenceFactHash = runtimecontract.StableRuntimeEvidenceFactHash(*fact)
							}
						}
					}
				}
			},
			wantError: "contiguous dispatch chain",
		},
		{
			name: "receipt chain detached from confirmation",
			mutate: func(value *RuntimeAggregate) {
				execution, _ := operationExecutionRecord(value.ActiveDispatch.OperationExecutions, value.ActiveDispatch.CompletedOperationIDs[0])
				oldFactID := execution.EvidenceFactID
				for index := range value.OperationReceipts {
					if value.OperationReceipts[index].ExecutionKey != execution.ExecutionKey || value.OperationReceipts[index].Fence != execution.Fence {
						continue
					}
					for changeIndex := range value.OperationReceipts[index].ResourceChanges {
						change := &value.OperationReceipts[index].ResourceChanges[changeIndex]
						if change.Domain == runtimecontract.RuntimeResourceWorkspaceTree {
							change.BeforeRevision = "tree-detached"
						}
					}
					receipt := value.OperationReceipts[index].Normalized()
					value.OperationReceipts[index] = receipt
					hash := runtimecontract.StableRuntimeOperationReceiptHash(receipt)
					newFactID := common.EvidenceRecordID("runtime-receipt|" + hash)
					for factIndex := range value.Facts.Evidence.Accepted {
						fact := &value.Facts.Evidence.Accepted[factIndex]
						if fact.ID != oldFactID {
							continue
						}
						fact.ID = newFactID
					}
					for processedIndex := range value.ProcessedEventIDs {
						if value.ProcessedEventIDs[processedIndex] == string(oldFactID) {
							value.ProcessedEventIDs[processedIndex] = string(newFactID)
						}
					}
					for executionIndex := range value.ActiveDispatch.OperationExecutions {
						if value.ActiveDispatch.OperationExecutions[executionIndex].OperationID == execution.OperationID {
							persisted := &value.ActiveDispatch.OperationExecutions[executionIndex]
							persisted.OwnerReceiptHash = hash
							persisted.EvidenceFactID = newFactID
							for _, fact := range value.Facts.Evidence.Accepted {
								if fact.ID == newFactID {
									persisted.EvidenceFactHash = runtimecontract.StableRuntimeEvidenceFactHash(fact)
								}
							}
						}
					}
				}
			},
			wantError: "workspace revision chain",
		},
		{
			name: "coordinated source identity rewrite",
			mutate: func(value *RuntimeAggregate) {
				observation := value.RepairSurfaceConfirmations[len(value.RepairSurfaceConfirmations)-1].Confirmation.Observation
				for index := range value.Facts.Evidence.Rejected {
					fact := &value.Facts.Evidence.Rejected[index]
					if fact.DispatchID != observation.DispatchID || fact.OperationID != observation.OperationID {
						continue
					}
					oldID := string(fact.ID)
					fact.ID = "forged-source-event"
					fact.SourceRef = "workorch://forged/source"
					for processedIndex := range value.ProcessedEventIDs {
						if value.ProcessedEventIDs[processedIndex] == oldID {
							value.ProcessedEventIDs[processedIndex] = string(fact.ID)
						}
					}
				}
			},
			wantError: "canonical persisted receipt-recovery identity",
		},
		{
			name: "coordinated live source identity rewrite",
			mutate: func(value *RuntimeAggregate) {
				observation := value.RepairSurfaceConfirmations[len(value.RepairSurfaceConfirmations)-1].Confirmation.Observation
				for index := range value.Facts.Evidence.Rejected {
					fact := &value.Facts.Evidence.Rejected[index]
					if fact.DispatchID != observation.DispatchID || fact.OperationID != observation.OperationID {
						continue
					}
					oldID := string(fact.ID)
					fact.ID = common.EvidenceRecordID("runtime-operation|" + observation.DispatchID + "|" + observation.OperationID)
					fact.SourceRef = "workorch://runtime-operations/" + observation.DispatchID + "/" + observation.OperationID
					for processedIndex := range value.ProcessedEventIDs {
						if value.ProcessedEventIDs[processedIndex] == oldID {
							value.ProcessedEventIDs[processedIndex] = string(fact.ID)
						}
					}
					for dispatchIndex := range value.CompletedDispatches {
						dispatch := &value.CompletedDispatches[dispatchIndex]
						if dispatch.ID != observation.DispatchID {
							continue
						}
						for executionIndex := range dispatch.OperationExecutions {
							execution := &dispatch.OperationExecutions[executionIndex]
							if execution.OperationID == observation.OperationID {
								execution.EvidenceFactID = fact.ID
								execution.EvidenceFactHash = runtimecontract.StableRuntimeEvidenceFactHash(*fact)
							}
						}
					}
				}
			},
			wantError: "receipt-recovery identity",
		},
		{
			name: "source subject drift",
			mutate: func(value *RuntimeAggregate) {
				observation := value.RepairSurfaceConfirmations[len(value.RepairSurfaceConfirmations)-1].Confirmation.Observation
				for index := range value.Facts.Evidence.Rejected {
					if value.Facts.Evidence.Rejected[index].DispatchID == observation.DispatchID && value.Facts.Evidence.Rejected[index].OperationID == observation.OperationID {
						value.Facts.Evidence.Rejected[index].SubjectRef = "another-operation"
					}
				}
			},
			wantError: "scope/revision does not match",
		},
		{
			name: "source evidence reason drift",
			mutate: func(value *RuntimeAggregate) {
				observation := value.RepairSurfaceConfirmations[len(value.RepairSurfaceConfirmations)-1].Confirmation.Observation
				for index := range value.Facts.Evidence.Rejected {
					if value.Facts.Evidence.Rejected[index].DispatchID == observation.DispatchID && value.Facts.Evidence.Rejected[index].OperationID == observation.OperationID {
						value.Facts.Evidence.Rejected[index].EvidenceReason = "forged reason"
					}
				}
			},
			wantError: "does not match its exact validation-owner receipt",
		},
		{
			name: "source revision drift",
			mutate: func(value *RuntimeAggregate) {
				observation := value.RepairSurfaceConfirmations[len(value.RepairSurfaceConfirmations)-1].Confirmation.Observation
				for index := range value.Facts.Evidence.Rejected {
					if value.Facts.Evidence.Rejected[index].DispatchID == observation.DispatchID && value.Facts.Evidence.Rejected[index].OperationID == observation.OperationID {
						value.Facts.Evidence.Rejected[index].Revision = "runtime-evidence-99999999999999999999"
					}
				}
			},
			wantError: "commitment source lineage mismatch",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tampered := cloneRuntimeAggregateForMCPProvenanceTest(t, partial.Aggregate)
			test.mutate(&tampered)
			if err := validatePendingConfirmedRepairTransaction(tampered); err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("fresh-load tamper error = %v", err)
			}
		})
	}
}

func TestPendingConfirmedRepairTransactionRejectsMutableAuthorityAndFreshnessDrift(t *testing.T) {
	fixture := newConfirmedRepairMultiOperationFixture(t)
	coordinator, err := NewDispatchCoordinator(fixture.repository)
	if err != nil {
		t.Fatal(err)
	}
	first := fixture.acceptedEvidence(t, 0, "tree-v1", "tree-v2")
	attachConfirmedRepairReceiptForTest(t, &fixture.repository.aggregate, &first, "tree-v1", "tree-v2")
	partial, err := coordinator.commitEvidence(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	transaction, found, err := resolveConfirmedRepairTransaction(partial.Aggregate, *partial.Aggregate.ActiveDispatch)
	if err != nil || !found {
		t.Fatalf("resolve partial transaction: found=%v err=%v", found, err)
	}

	tests := []struct {
		name      string
		mutate    func(*RuntimeAggregate)
		wantError string
	}{
		{
			name: "restored failure target",
			mutate: func(value *RuntimeAggregate) {
				observation := runtimecontract.RuntimeFailureTargetObservation{
					OwnerModuleID:    transaction.confirmation.OwnerModuleID,
					SubjectRef:       transaction.confirmation.SubjectRef,
					TargetRole:       transaction.confirmation.Observation.TargetRole,
					ObservedRevision: transaction.confirmation.ObservedRevision,
					TargetRefs:       append([]string(nil), transaction.confirmation.ConfirmedTargetRefs...),
				}.Normalized()
				value.Facts.Resources.FailureTarget = &runtimecontract.FailureTarget{
					Signature:         transaction.confirmation.Observation.FailureSignature,
					TargetRefs:        append([]string(nil), transaction.confirmation.ConfirmedTargetRefs...),
					TargetObservation: &observation,
				}
			},
			wantError: "restored superseded mutable authority",
		},
		{
			name: "restored failure target without confirmation ledger",
			mutate: func(value *RuntimeAggregate) {
				value.RepairSurfaceConfirmations = nil
				observation := runtimecontract.RuntimeFailureTargetObservation{
					OwnerModuleID:    transaction.confirmation.OwnerModuleID,
					SubjectRef:       transaction.confirmation.SubjectRef,
					TargetRole:       transaction.confirmation.Observation.TargetRole,
					ObservedRevision: transaction.confirmation.ObservedRevision,
					TargetRefs:       append([]string(nil), transaction.confirmation.ConfirmedTargetRefs...),
				}.Normalized()
				value.Facts.Resources.FailureTarget = &runtimecontract.FailureTarget{
					Signature:         transaction.confirmation.Observation.FailureSignature,
					TargetRefs:        append([]string(nil), transaction.confirmation.ConfirmedTargetRefs...),
					TargetObservation: &observation,
				}
			},
			wantError: "has no unique exact owner confirmation",
		},
		{
			name: "unrelated directive cannot hide missing confirmation lineage",
			mutate: func(value *RuntimeAggregate) {
				value.RepairSurfaceConfirmations = nil
				directive := transaction.stateChangeDirective(value.Facts, value.Decision)
				directive.ID = "unrelated-generic-repair"
				directive.CurrentFailure = nil
				value.Facts.RepairDirective = &directive
			},
			wantError: "has no unique exact owner confirmation",
		},
		{
			name: "unrelated failure target cannot hide missing confirmation lineage",
			mutate: func(value *RuntimeAggregate) {
				value.RepairSurfaceConfirmations = nil
				value.Facts.Resources.FailureTarget = &runtimecontract.FailureTarget{
					Signature: "unrelated-failure", TargetRefs: []string{"unrelated.go"},
				}
			},
			wantError: "has no unique exact owner confirmation",
		},
		{
			name: "restored state change directive",
			mutate: func(value *RuntimeAggregate) {
				directive := transaction.stateChangeDirective(value.Facts, value.Decision)
				value.Facts.RepairDirective = &directive
			},
			wantError: "restored superseded mutable authority",
		},
		{
			name: "extra pending prerequisite",
			mutate: func(value *RuntimeAggregate) {
				value.Facts.Prerequisites.Pending = append(value.Facts.Prerequisites.Pending, runtimecontract.RuntimePrerequisite{
					ID: "forged-pending", Kind: runtimecontract.PrerequisiteKindManifest, OwnerModuleID: runtimecontract.RuntimeModuleFile,
				})
			},
			wantError: "exactly bind current pending prerequisites",
		},
		{
			name: "missing pending prerequisites",
			mutate: func(value *RuntimeAggregate) {
				value.Facts.Prerequisites.Pending = nil
			},
			wantError: "exactly bind current pending prerequisites",
		},
		{
			name: "restored invalidated observation",
			mutate: func(value *RuntimeAggregate) {
				invalidation := value.PrerequisiteInvalidations[len(value.PrerequisiteInvalidations)-1].Normalized()
				value.Facts.Resources.PrerequisiteObservations = append(value.Facts.Resources.PrerequisiteObservations,
					runtimecontract.RuntimePrerequisiteObservation{
						PrerequisiteID: invalidation.PrerequisiteIDs[0], ObservationID: invalidation.ObservationIDs[0],
					})
			},
			wantError: "retains an invalidated owner observation",
		},
		{
			name: "missing dispatch invalidation",
			mutate: func(value *RuntimeAggregate) {
				kept := value.PrerequisiteInvalidations[:0]
				for _, invalidation := range value.PrerequisiteInvalidations {
					if invalidation.DispatchID != value.ActiveDispatch.ID {
						kept = append(kept, invalidation)
					}
				}
				value.PrerequisiteInvalidations = kept
			},
			wantError: "exactly bind current pending prerequisites",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tampered := cloneRuntimeAggregateForMCPProvenanceTest(t, partial.Aggregate)
			test.mutate(&tampered)
			if err := validatePendingConfirmedRepairTransaction(tampered); err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("pending File repair tamper error = %v", err)
			}
		})
	}
}

type confirmedRepairMultiOperationFixture struct {
	repository *durableRuntimeStateRepository
	closureID  string
}

type confirmedRepairGovernanceTestResult struct {
	Aggregate   RuntimeAggregate
	operationID string
	resolution  runtimecontract.RuntimeGovernanceResolution
}

func advanceConfirmedRepairOperationGovernanceForTest(
	t *testing.T,
	repository *durableRuntimeStateRepository,
	operationIndex int,
	suffix string,
	outcome runtimecontract.RuntimeGovernanceResolutionOutcome,
) confirmedRepairGovernanceTestResult {
	t.Helper()
	if repository == nil || repository.aggregate.ActiveDispatch == nil {
		t.Fatal("confirmed owner repair governance fixture has no active dispatch")
	}
	ctx := context.Background()
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	operations, err := orderedCandidateOperations(repository.aggregate.ActiveDispatch.Candidate)
	if err != nil || operationIndex < 0 || operationIndex >= len(operations) {
		t.Fatalf("confirmed owner repair governance operation index %d: %v", operationIndex, err)
	}
	operation := operations[operationIndex].Normalized()
	claimed, err := controller.claimOperationExecution(ctx, repository.aggregate.Facts.WorkspaceID, repository.aggregate.Facts.RunID, operation.ID)
	if err != nil {
		t.Fatalf("claim confirmed owner repair operation for governance: %v", err)
	}
	execution, found := operationExecutionRecord(claimed.ActiveDispatch.OperationExecutions, operation.ID)
	if !found || execution.State != runtimecontract.OperationExecutionClaimed {
		t.Fatalf("confirmed owner repair governance execution was not claimed: %+v", execution)
	}
	targetRef := operation.TargetRefs[0]
	requirement := runtimecontract.RuntimeGovernanceRequirement{
		Kind: runtimecontract.GovernanceApproval, SubjectRef: targetRef,
		Reason: "confirmed owner repair requires explicit owner approval", RequestedAction: operation.Kind, RiskLevel: "high",
		AuthorizationContext: workorcheventpayload.New(workorcheventpayload.StringField("targetRef", targetRef)),
	}
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: execution.ExecutionKey, OperationID: execution.OperationID,
		Outcome:             runtimecontract.RuntimeOperationOutcomeGovernanceRequired,
		ObservedEffectScope: runtimecontract.RuntimeEffectGovernanceOnly, Governance: &requirement,
	}
	sourceDispatchID := claimed.ActiveDispatch.ID
	_, persistedReceipt, err := controller.commitOperationReceipt(
		ctx, claimed.Facts.WorkspaceID, claimed.Facts.RunID, sourceDispatchID, execution.Fence, runtimecontract.RuntimeModuleFile, receipt,
	)
	if err != nil {
		t.Fatalf("commit confirmed owner repair governance receipt: %v", err)
	}
	governanceRequired, err := controller.candidateDispatch.recordOperationGovernanceRequired(
		ctx, claimed.Facts.WorkspaceID, claimed.Facts.RunID, operation.ID,
		RuntimeOperationExecutionResult{
			EventID: "confirmed-repair-governance-" + suffix, OperationID: operation.ID,
			Receipt: persistedReceipt, Governance: &requirement,
		},
	)
	if err != nil {
		t.Fatalf("record confirmed owner repair governance requirement: %v", err)
	}
	if governanceRequired.ActiveDispatch != nil || governanceRequired.Decision.State != runtimecontract.ConvergenceStateNeedGovernance ||
		governanceRequired.Facts.Failure == nil || governanceRequired.Facts.Failure.GovernanceDependency == nil {
		t.Fatalf("confirmed owner repair did not enter exact governance wait: %+v", governanceRequired)
	}
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(
		governanceRequired.Decision,
		runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(governanceRequired.Facts.Resources),
	)
	if err != nil {
		t.Fatalf("build confirmed owner repair governance manifest: %v", err)
	}
	dependency := governanceRequired.Facts.Failure.GovernanceDependency.Clone()
	governanceCandidate := runtimecontract.RuntimeCandidate{Governance: &runtimecontract.GovernanceCandidate{ExternalDependency: dependency}}
	governed, err := controller.sealGovernanceDispatch(
		ctx, governanceRequired.Facts.WorkspaceID, governanceRequired.Facts.RunID, manifest,
		governanceCandidate,
	)
	if err != nil {
		t.Fatalf("seal confirmed owner repair governance dispatch: %v", err)
	}
	approvalID := common.ApprovalID("approval-confirmed-repair-" + suffix)
	bound, err := controller.bindGovernanceRequest(
		ctx, governed.Facts.WorkspaceID, governed.Facts.RunID,
		runtimecontract.RuntimeGovernanceRequestRecord{
			ApprovalID: approvalID, AuthoritySnapshotID: governed.ActiveDispatch.Transaction.AuthoritySnapshotID,
			DispatchID: governed.ActiveDispatch.ID, DependencyKind: dependency.Kind, SubjectRef: dependency.SubjectRef,
			RequestedAction: dependency.RequestedAction, RiskLevel: dependency.RiskLevel,
			AuthorizationHash: runtimecontract.StableRuntimeAuthorizationContextHash(dependency.AuthorizationContext),
		},
	)
	if err != nil {
		t.Fatalf("bind confirmed owner repair governance request: %v", err)
	}
	if len(bound.OperationGovernanceBindings) == 0 {
		t.Fatal("confirmed owner repair governance request has no operation binding")
	}
	resolution := runtimecontract.RuntimeGovernanceResolution{
		ResolutionID: common.ResolutionID("resolution-confirmed-repair-" + suffix),
		ApprovalID:   approvalID, WorkspaceID: governed.Facts.WorkspaceID, RunID: governed.Facts.RunID,
		AuthoritySnapshotID: governed.ActiveDispatch.Transaction.AuthoritySnapshotID, DispatchID: governed.ActiveDispatch.ID,
		DependencyKind: dependency.Kind, SubjectRef: dependency.SubjectRef, Outcome: outcome,
		Summary: "confirmed owner repair governance " + string(outcome), EvidenceRefs: []string{"approval://confirmed-repair/" + suffix},
	}
	committed, err := controller.commitGovernanceResolution(ctx, resolution)
	if err != nil {
		t.Fatalf("commit confirmed owner repair governance resolution: %v", err)
	}
	if committed.Disposition != CommitDispositionCommitted {
		t.Fatalf("confirmed owner repair governance resolution disposition = %q", committed.Disposition)
	}
	return confirmedRepairGovernanceTestResult{
		Aggregate: committed.Aggregate, operationID: operation.ID, resolution: resolution,
	}
}

func assertConfirmedRepairFreshValidationForTest(t *testing.T, aggregate RuntimeAggregate, closureID string) {
	t.Helper()
	if aggregate.ActiveDispatch != nil || aggregate.Facts.Resources.FailureTarget != nil || aggregate.Facts.RepairDirective == nil ||
		aggregate.Facts.RepairDirective.RequiredState != runtimecontract.ConvergenceStateNeedValidation ||
		aggregate.Facts.RepairDirective.CurrentFailure != nil || len(aggregate.Facts.RepairDirective.TargetRefs) != 0 ||
		aggregate.Facts.RepairDirective.Closure == nil || aggregate.Facts.RepairDirective.Closure.ID != closureID {
		t.Fatalf("confirmed owner repair did not close into target-free fresh validation: %+v", aggregate)
	}
}

func newConfirmedRepairMultiOperationFixture(t *testing.T) confirmedRepairMultiOperationFixture {
	t.Helper()
	return newConfirmedRepairFixture(t, []string{"main.go", "other.go"})
}

func newConfirmedRepairSingleOperationFixture(t *testing.T) confirmedRepairMultiOperationFixture {
	t.Helper()
	return newConfirmedRepairFixture(t, []string{"main.go"})
}

type confirmedRepairAuthorityFixture struct {
	repository *durableRuntimeStateRepository
	controller *RuntimeController
	aggregate  RuntimeAggregate
	closureID  string
	targets    []string
}

func newConfirmedRepairAuthorityFixture(t *testing.T, targets []string) confirmedRepairAuthorityFixture {
	return newConfirmedRepairAuthorityFixtureWithBindingProofs(t, targets, false)
}

func newConfirmedRepairValidationOnlyAuthorityFixture(t *testing.T, targets []string) confirmedRepairAuthorityFixture {
	return newConfirmedRepairAuthorityFixtureWithBindingProofs(t, targets, true)
}

func newConfirmedRepairAuthorityFixtureWithBindingProofs(t *testing.T, targets []string, validationOnly bool) confirmedRepairAuthorityFixture {
	t.Helper()
	ctx := context.Background()
	targets = normalizeRuntimeTargetRefs(targets)
	if len(targets) == 0 {
		t.Fatal("confirmed owner repair fixture requires at least one target")
	}
	var aggregate RuntimeAggregate
	if validationOnly {
		_, _, aggregate = sealMultiTargetExactValidationOnlyBindingForTest(t, targets, "implementation")
		if len(aggregate.Facts.DefinitionFocuses) != 1 {
			t.Fatalf("validation-only confirmed repair fixture Definition focuses = %+v", aggregate.Facts.DefinitionFocuses)
		}
		convergence := runtimeValidationFailureRepairConvergenceForTest(2)
		aggregate.Facts.DefinitionFocuses[0].ValidationFailureRepairConvergence = &convergence
		aggregate.Facts.DefinitionFocuses = runtimecontract.NormalizeRuntimeDefinitionFocuses(aggregate.Facts.DefinitionFocuses)
	} else {
		_, _, aggregate = sealMultiTargetExactValidationBindingForTest(t, targets, "implementation")
	}
	aggregate.Facts.Capabilities.GovernanceKinds = []string{runtimecontract.RuntimeBlockKindApprovalRequest}
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	coordinator, err := NewDispatchCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	bindingID := aggregate.Facts.Focus.CoverageEvidenceBindings[0].ID
	if !validationOnly {
		stateChange := multiTargetExactCandidate(bindingID, targets...)
		sealedStateChange := sealCandidateForTest(t, repository, stateChange)
		for index, operation := range candidateRuntimeOperations(sealedStateChange.ActiveDispatch.Candidate) {
			evidence := ExecutionEvidence{
				EventID:     fmt.Sprintf("confirmed-repair-baseline-state-%d", index+1),
				WorkspaceID: sealedStateChange.Facts.WorkspaceID, RunID: sealedStateChange.Facts.RunID,
				DispatchID: sealedStateChange.ActiveDispatch.ID, AuthoritySnapshotID: sealedStateChange.ActiveDispatch.Transaction.AuthoritySnapshotID,
				OperationID: operation.ID, Kind: runtimecontract.EvidenceKindStateChange,
				Status: ExecutionEvidenceStatusAccepted, SourceRef: "confirmed-repair/baseline-state/" + operation.ID,
			}
			attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &evidence)
			if _, err := coordinator.commitEvidence(ctx, evidence); err != nil {
				t.Fatalf("commit confirmed owner repair baseline state change %q: %v", operation.ID, err)
			}
		}
	}
	aggregate = repository.aggregate
	if aggregate.Decision.State != runtimecontract.ConvergenceStateNeedValidation || aggregate.ActiveDispatch != nil {
		t.Fatalf("confirmed owner repair baseline did not require validation: %+v", aggregate)
	}
	aggregate.Facts.Repair = runtimecontract.RuntimeRepairLedger{FocusID: aggregate.Facts.Focus.ID, SliceID: aggregate.Facts.Focus.SliceID}
	if err := aggregate.Validate(); err != nil {
		t.Fatalf("reset confirmed owner repair validation cycle: %v", err)
	}
	repository.aggregate = aggregate
	validation := validationCandidate()
	validation.Validation.Operations[0].Operation.CoverageWorkItemIDs = nil
	validation.Validation.Operations[0].Operation.BindingRefs = []string{bindingID}
	validation.Validation.Operations[0].Operation.TargetRefs = append([]string(nil), targets...)
	sealed := sealCandidateForTest(t, repository, validation)
	operationID := runtimeTestCandidateOperationID(t, sealed.ActiveDispatch.Candidate, 0)
	if sealed.ActiveDispatch.FocusTargetRole == "" {
		t.Fatal("confirmed owner repair validation dispatch omitted the sealed focus target role")
	}
	observation := runtimecontract.RuntimeValidationFailureObservation{
		WorkspaceID: sealed.Facts.WorkspaceID, RunID: sealed.Facts.RunID,
		AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		DispatchID:          sealed.ActiveDispatch.ID, OperationID: operationID,
		FailureSignature: "multi-build-failure", TargetRole: sealed.ActiveDispatch.FocusTargetRole,
		DiagnosticClass:     runtimecontract.ValidationDiagnosticBuildFailed,
		CandidateTargetRefs: append([]string(nil), targets...), ValidationScopeTargetRefs: append([]string(nil), targets...),
		DiagnosticHash: strings.Repeat("4", 64),
	}
	rejected := runtimeRejectedValidationEvidenceForTest(t, sealed, orchestratorevents.RuntimeOperationEventID(sealed.ActiveDispatch.ID, operationID), observation.DiagnosticClass)
	rejected.SourceRef = orchestratorevents.RuntimeOperationSourceRef(sealed.ActiveDispatch.ID, operationID)
	rejected.Failure.Signature = observation.FailureSignature
	rejected.Failure.ValidationDiagnostic.TargetRefs = append([]string(nil), targets...)
	rejected.Failure.ValidationFailureObservation = &observation
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &rejected)
	committed, err := coordinator.commitEvidence(ctx, rejected)
	if err != nil {
		t.Fatalf("commit multi-target validation failure: %v", err)
	}
	confirmation := runtimecontract.RuntimeRepairSurfaceConfirmation{
		Observation: observation, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable",
		Disposition: runtimecontract.RuntimeRepairSurfaceConfirmed, ConfirmedTargetRefs: append([]string(nil), targets...), ObservedRevision: "tree-v1",
	}
	confirmed, err := controller.recordRepairSurfaceConfirmation(ctx, committed.Aggregate.Facts.WorkspaceID, committed.Aggregate.Facts.RunID, confirmation)
	if err != nil {
		t.Fatalf("confirm multi-target repair surface: %v", err)
	}
	confirmed = satisfyConfirmedRepairTargetSnapshotsForTest(t, ctx, controller, confirmed, "tree-v1")
	if confirmed.Decision.State != runtimecontract.ConvergenceStateNeedStateChange || confirmed.Facts.RepairDirective == nil ||
		confirmed.Facts.RepairDirective.Closure == nil || confirmed.Facts.Resources.FailureTarget == nil {
		t.Fatalf("multi-target confirmation did not establish repair authority: %+v", confirmed)
	}
	return confirmedRepairAuthorityFixture{
		repository: repository,
		controller: controller,
		aggregate:  confirmed,
		closureID:  confirmed.Facts.RepairDirective.Closure.ID,
		targets:    append([]string(nil), targets...),
	}
}

func newConfirmedRepairFixture(t *testing.T, targets []string) confirmedRepairMultiOperationFixture {
	t.Helper()
	return newConfirmedRepairSubsetFixture(t, targets, targets)
}

func newConfirmedRepairSubsetFixture(t *testing.T, surfaceTargets, selectedTargets []string) confirmedRepairMultiOperationFixture {
	t.Helper()
	ctx := context.Background()
	authority := newConfirmedRepairAuthorityFixture(t, surfaceTargets)
	surfaceTargets = authority.targets
	selectedTargets = normalizeRuntimeTargetRefs(selectedTargets)
	if !runtimeTargetRefsWithin(selectedTargets, surfaceTargets) {
		t.Fatalf("confirmed owner repair selected targets %q are outside surface %q", selectedTargets, surfaceTargets)
	}
	repository := authority.repository
	controller := authority.controller
	confirmed := authority.aggregate
	closureID := authority.closureID
	bindingID := confirmed.Facts.Focus.CoverageEvidenceBindings[0].ID
	operations := make([]runtimecontract.StateChangingOperation, 0, len(selectedTargets))
	previousID := ""
	for index, targetRef := range selectedTargets {
		operation := runtimecontract.RuntimeOperation{
			ID: fmt.Sprintf("repair-%d", index+1), Kind: runtimecontract.StepKindFileWrite,
			BindingRefs: []string{bindingID}, TargetRefs: []string{targetRef}, Parameters: runtimeFileWriteParameters("updated " + targetRef),
		}
		if previousID != "" {
			operation.DependsOn = []string{previousID}
		}
		operations = append(operations, runtimecontract.StateChangingOperation{Operation: operation})
		previousID = operation.ID
	}
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: operations}}
	if _, err := controller.BeginCandidateAttemptForTest(
		ctx, confirmed.Facts.WorkspaceID, confirmed.Facts.RunID, 0, "confirmed-repair-production-admission",
	); err != nil {
		t.Fatalf("begin confirmed owner repair production admission: %v", err)
	}
	if err := RecordFixtureCandidateFeasibilities(
		ctx, controller, confirmed.Facts.WorkspaceID, confirmed.Facts.RunID, 0, candidate,
	); err != nil {
		t.Fatalf("record confirmed owner repair owner feasibility: %v", err)
	}
	admissionResult, err := controller.AdmitAndSealCandidateAttemptForTest(
		ctx, confirmed.Facts.WorkspaceID, confirmed.Facts.RunID, 0, candidate,
	)
	if err != nil || !admissionResult.Admission.Accepted() || admissionResult.Aggregate.ActiveDispatch == nil {
		t.Fatalf("seal confirmed owner repair production admission: result=%+v err=%v", admissionResult, err)
	}
	_, acceptedAttempt := runtimeTestAcceptedAttemptForActiveDispatch(t, admissionResult.Aggregate)
	var requestSession *runtimecontract.RuntimeCandidateRequestSession
	for index := range admissionResult.Aggregate.CandidateRequestSessions {
		if admissionResult.Aggregate.CandidateRequestSessions[index].LogicalRequestID == acceptedAttempt.LogicalRequestID {
			requestSession = &admissionResult.Aggregate.CandidateRequestSessions[index]
		}
	}
	if requestSession == nil || requestSession.RepairCommitment == nil || acceptedAttempt.AdmissionRepairCommitment == nil ||
		admissionResult.Aggregate.ActiveDispatch.RepairCommitment == nil ||
		!runtimecontract.SameRuntimeDispatchRepairCommitment(requestSession.RepairCommitment, acceptedAttempt.AdmissionRepairCommitment) ||
		!runtimecontract.SameRuntimeDispatchRepairCommitment(requestSession.RepairCommitment, admissionResult.Aggregate.ActiveDispatch.RepairCommitment) {
		t.Fatalf("confirmed owner repair production admission did not persist exact pre-seal commitment: session=%+v attempt=%+v dispatch=%+v", requestSession, acceptedAttempt, admissionResult.Aggregate.ActiveDispatch)
	}
	return confirmedRepairMultiOperationFixture{repository: repository, closureID: closureID}
}

func satisfyConfirmedRepairTargetSnapshotsForTest(
	t *testing.T,
	ctx context.Context,
	controller *RuntimeController,
	current RuntimeAggregate,
	treeRevision string,
) RuntimeAggregate {
	t.Helper()
	for current.Decision.State == runtimecontract.ConvergenceStateNeedPrerequisite {
		var prerequisite runtimecontract.RuntimePrerequisite
		for _, pending := range current.Facts.Prerequisites.Pending {
			if pending.Kind == runtimecontract.PrerequisiteKindTargetSnapshot && pending.OwnerModuleID == runtimecontract.RuntimeModuleFile {
				prerequisite = pending
				break
			}
		}
		if prerequisite.ID == "" {
			t.Fatalf("unexpected confirmed owner repair prerequisites: %+v", current.Facts.Prerequisites.Pending)
		}
		snapshot := runtimecontract.TargetSnapshot{
			TargetRef: prerequisite.SubjectRef, Content: "before " + prerequisite.SubjectRef,
			ContentHash: "content-v1/" + prerequisite.SubjectRef, ContentState: runtimecontract.SnapshotContentStateComplete,
		}
		observation := runtimecontract.RuntimePrerequisiteObservation{
			PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: prerequisite.OwnerModuleID,
			ContentHash: snapshot.ContentHash,
			Dependencies: []runtimecontract.RuntimeFreshnessDependency{
				{Domain: runtimecontract.RuntimeResourceWorkspaceTarget, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: snapshot.TargetRef, Revision: snapshot.ContentHash},
				{Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable", Revision: treeRevision},
			},
		}
		observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
		var err error
		current, err = controller.commitPrerequisite(ctx, current.Facts.WorkspaceID, current.Facts.RunID, PrerequisiteResult{
			PrerequisiteID: prerequisite.ID, TargetSnapshot: &snapshot, Observation: &observation,
		})
		if err != nil {
			t.Fatalf("commit confirmed owner repair target snapshot: %v", err)
		}
	}
	return current
}

func (f confirmedRepairMultiOperationFixture) acceptedEvidence(t *testing.T, index int, beforeTree, afterTree string) ExecutionEvidence {
	t.Helper()
	if f.repository == nil || f.repository.aggregate.ActiveDispatch == nil {
		t.Fatal("multi-operation repair fixture has no active dispatch")
	}
	dispatch := *f.repository.aggregate.ActiveDispatch
	operations, err := orderedCandidateOperations(dispatch.Candidate)
	if err != nil || index < 0 || index >= len(operations) {
		t.Fatalf("multi-operation repair fixture operation index %d: %v", index, err)
	}
	operation := operations[index]
	return ExecutionEvidence{
		EventID:     "runtime-operation|" + dispatch.ID + "|" + operation.ID,
		WorkspaceID: f.repository.aggregate.Facts.WorkspaceID, RunID: f.repository.aggregate.Facts.RunID,
		DispatchID: dispatch.ID, AuthoritySnapshotID: dispatch.Transaction.AuthoritySnapshotID,
		OperationID: operation.ID, Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
		SourceRef: "workorch://runtime-operations/" + dispatch.ID + "/" + operation.ID,
		Evidence: runtimecontract.RuntimeOperationEvidence{
			Summary: fmt.Sprintf("accepted repair %s -> %s", beforeTree, afterTree), Quality: common.EvidenceQualityValid,
		},
	}
}

func attachConfirmedRepairReceiptForTest(
	t *testing.T,
	aggregate *RuntimeAggregate,
	evidence *ExecutionEvidence,
	beforeTree string,
	afterTree string,
) {
	t.Helper()
	attachRuntimeTestOwnerReceipt(t, aggregate, evidence)
	operation, found := candidateOperationByID(aggregate.ActiveDispatch.Candidate, evidence.OperationID)
	if !found || len(operation.TargetRefs) != 1 || evidence.Receipt == nil {
		t.Fatalf("confirmed owner repair receipt fixture operation = %+v receipt=%+v", operation, evidence.Receipt)
	}
	receipt := evidence.Receipt.Normalized()
	receipt.BeforeHash = "content-before/" + operation.ID
	receipt.AfterHash = "content-after/" + operation.ID
	receipt.ResourceChanges = []runtimecontract.RuntimeResourceChange{
		{Domain: runtimecontract.RuntimeResourceWorkspaceTarget, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: operation.TargetRefs[0], BeforeRevision: receipt.BeforeHash, AfterRevision: receipt.AfterHash},
		{Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable", BeforeRevision: beforeTree, AfterRevision: afterTree},
	}
	receipt = receipt.Normalized()
	hash := runtimecontract.StableRuntimeOperationReceiptHash(receipt)
	for index := range aggregate.OperationReceipts {
		if aggregate.OperationReceipts[index].ExecutionKey == receipt.ExecutionKey && aggregate.OperationReceipts[index].Fence == receipt.Fence {
			aggregate.OperationReceipts[index] = receipt
		}
	}
	for index := range aggregate.ActiveDispatch.OperationExecutions {
		if aggregate.ActiveDispatch.OperationExecutions[index].OperationID == evidence.OperationID {
			aggregate.ActiveDispatch.OperationExecutions[index].OwnerReceiptHash = hash
		}
	}
	evidence.Receipt = &receipt
	evidence.OwnerReceiptRef = "runtime-receipt/" + receipt.ExecutionKey
	evidence.OwnerReceiptHash = hash
}
