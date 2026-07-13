package runtimev2

import (
	"context"
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type controllerRepository struct {
	aggregate *RuntimeAggregate
	saves     int
}

func (r *controllerRepository) Load(_ context.Context, workspaceID common.WorkspaceID, runID common.RunID) (RuntimeAggregate, error) {
	if r.aggregate == nil {
		return RuntimeAggregate{}, common.ErrNotFound
	}
	if r.aggregate.Facts.WorkspaceID != workspaceID || r.aggregate.Facts.RunID != runID {
		return RuntimeAggregate{}, common.ErrNotFound
	}
	return *r.aggregate, nil
}

func (r *controllerRepository) Save(_ context.Context, aggregate RuntimeAggregate, expectedRevision uint64) error {
	currentRevision := uint64(0)
	if r.aggregate != nil {
		currentRevision = r.aggregate.StateRevision
	}
	if currentRevision != expectedRevision {
		return common.ErrConflict
	}
	value := aggregate
	r.aggregate = &value
	r.saves++
	return nil
}

func TestRuntimeControllerInitializeRunsPreflightReduceAndPersistOnce(t *testing.T) {
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := controllerFacts()
	requirements := []RuntimeRequirement{{Kind: runtimecontract.PrerequisiteKindManifest, Reason: "manifest required"}}
	initialized, err := controller.Initialize(context.Background(), facts, controllerDefinition(), requirements)
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if initialized.StateRevision != 1 || initialized.Decision.Action != runtimecontract.RuntimeActionAcquirePrerequisite {
		t.Fatalf("initialized aggregate = %+v", initialized)
	}
	if len(initialized.Facts.Prerequisites.Pending) != 1 || repository.saves != 1 {
		t.Fatalf("pending=%+v saves=%d", initialized.Facts.Prerequisites.Pending, repository.saves)
	}

	restarted, err := controller.Initialize(context.Background(), facts, controllerDefinition(), requirements)
	if err != nil {
		t.Fatalf("restart Initialize: %v", err)
	}
	if repository.saves != 1 {
		t.Fatalf("restart performed another save: %d", repository.saves)
	}
	if restarted.Decision.AuthoritySnapshot.ID != initialized.Decision.AuthoritySnapshot.ID || restarted.Decision.AuthoritySnapshot.Attempt != 0 {
		t.Fatalf("restart changed authority: before=%+v after=%+v", initialized.Decision.AuthoritySnapshot, restarted.Decision.AuthoritySnapshot)
	}
}

func TestRuntimeControllerInitializeRejectsDefinitionDrift(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	facts := controllerFacts()
	if _, err := controller.Initialize(context.Background(), facts, controllerDefinition(), nil); err != nil {
		t.Fatal(err)
	}
	facts.DefinitionRevision = "definition-v2-changed"
	definition := controllerDefinition()
	definition.Revision = facts.DefinitionRevision
	_, err := controller.Initialize(context.Background(), facts, definition, nil)
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("definition drift err = %v, want conflict", err)
	}
}

func TestRuntimeControllerInitializeRejectsPersistedDefinitionContentDrift(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	facts := controllerFacts()
	definition := controllerDefinition()
	if _, err := controller.Initialize(context.Background(), facts, definition, nil); err != nil {
		t.Fatal(err)
	}
	definition.GoalDefinition = "被替换的目标"
	_, err := controller.Initialize(context.Background(), facts, definition, nil)
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("definition content drift err = %v, want conflict", err)
	}
}

func TestRuntimeControllerCommitsPrerequisiteAndReducesAtomically(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	aggregate, err := controller.Initialize(context.Background(), controllerFacts(), controllerDefinition(), []RuntimeRequirement{{Kind: runtimecontract.PrerequisiteKindManifest}})
	if err != nil {
		t.Fatal(err)
	}
	prerequisiteID := aggregate.Facts.Prerequisites.Pending[0].ID
	progressed, err := controller.CommitPrerequisite(context.Background(), "ws-v2", "run-v2", PrerequisiteResult{
		PrerequisiteID: prerequisiteID,
		Manifest:       &runtimecontract.WorkspaceManifest{Revision: "manifest-v1", TargetRefs: []string{"target.txt"}},
	})
	if err != nil {
		t.Fatalf("CommitPrerequisite: %v", err)
	}
	if progressed.StateRevision != 2 || progressed.Decision.Action != runtimecontract.RuntimeActionProduceStateChange {
		t.Fatalf("progressed aggregate = %+v", progressed)
	}
	if progressed.Decision.AuthoritySnapshot.Attempt != 0 {
		t.Fatalf("prerequisite progress inherited retry attempt %d", progressed.Decision.AuthoritySnapshot.Attempt)
	}
}

func TestRuntimeControllerOwnsDispatchAndEvidenceCommit(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	_, err := controller.Initialize(context.Background(), controllerFacts(), controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "write-1", Kind: "file/write", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"},
	}}}}}
	if _, err := controller.BeginCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	sealed, err := controller.AcceptCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, candidate)
	if err != nil {
		t.Fatalf("AcceptCandidateAttempt: %v", err)
	}
	result, err := controller.CommitEvidence(context.Background(), ExecutionEvidence{
		EventID: "event-1", WorkspaceID: "ws-v2", RunID: "run-v2",
		DispatchID: sealed.ActiveDispatch.ID, AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		OperationID: "write-1", Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	})
	if err != nil || result.Disposition != CommitDispositionCommitted {
		t.Fatalf("CommitEvidence = %+v err=%v", result, err)
	}
	if result.Aggregate.Decision.Action != runtimecontract.RuntimeActionComplete {
		t.Fatalf("next action = %s", result.Aggregate.Decision.Action)
	}
}

func TestRuntimeControllerCommitsTypedAssessmentBeforeCompletion(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	facts := controllerFacts()
	facts.Focus.RequireAssessment = true
	if _, err := controller.Initialize(context.Background(), facts, controllerDefinition(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.BeginCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	sealed, err := controller.AcceptCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "write-1", Kind: "file/write", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
	}}}}})
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := controller.CommitEvidence(context.Background(), ExecutionEvidence{
		EventID: "event-1", WorkspaceID: "ws-v2", RunID: "run-v2", DispatchID: sealed.ActiveDispatch.ID,
		AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID, OperationID: "write-1",
		Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	})
	if err != nil || evidence.Aggregate.Decision.Action != runtimecontract.RuntimeActionProduceAssessment {
		t.Fatalf("assessment authority = %+v err=%v", evidence.Aggregate.Decision, err)
	}
	if _, err := controller.BeginCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, "assessment-prefix"); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.AcceptCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{
		FocusID: "focus-v2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, Satisfied: true,
		Summary: "current focus is satisfied", EvidenceRefs: []string{"event-1"},
	}}); err != nil {
		t.Fatal(err)
	}
	completed, err := controller.CommitAssessment(context.Background(), "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if completed.Decision.Action != runtimecontract.RuntimeActionComplete || len(completed.Facts.Assessments) != 1 || !completed.Facts.Assessments[0].Satisfied {
		t.Fatalf("completed assessment aggregate = %+v", completed)
	}
}

func TestRuntimeControllerCandidateRetryIsPersistedAndLimitedToTwoAttempts(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	facts := controllerFacts()
	facts.Capabilities.GovernanceKinds = []string{"decision/request"}
	if _, err := controller.Initialize(context.Background(), facts, controllerDefinition(), nil); err != nil {
		t.Fatal(err)
	}
	firstPending, err := controller.BeginCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, "stable-prefix")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.BeginCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, "stable-prefix"); err != nil {
		t.Fatalf("pending attempt replay should be idempotent: %v", err)
	}
	if repository.saves != 2 {
		t.Fatalf("pending replay performed another save: %d", repository.saves)
	}
	firstFailed, err := controller.FailCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassSchema, Reason: "invalid candidate schema",
	})
	if err != nil {
		t.Fatal(err)
	}
	if firstFailed.Decision.AuthoritySnapshot.ID != firstPending.Decision.AuthoritySnapshot.ID || firstFailed.Decision.Action != runtimecontract.RuntimeActionProduceStateChange {
		t.Fatalf("attempt 0 failure changed contract: %+v", firstFailed.Decision)
	}
	if _, err := controller.BeginCandidateAttempt(context.Background(), "ws-v2", "run-v2", 1, "different-prefix"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("different-prefix retry err = %v, want conflict", err)
	}
	if _, err := controller.BeginCandidateAttempt(context.Background(), "ws-v2", "run-v2", 1, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	exhausted, err := controller.FailCandidateAttempt(context.Background(), "ws-v2", "run-v2", 1, runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassProvider, Reason: "provider unavailable",
	})
	if err != nil {
		t.Fatal(err)
	}
	if exhausted.Decision.Action != runtimecontract.RuntimeActionFail || exhausted.Decision.Terminal == nil || exhausted.Decision.Terminal.Code != "candidate_retry_exhausted" {
		t.Fatalf("exhausted decision = %+v", exhausted.Decision)
	}
	if _, err := controller.BeginCandidateAttempt(context.Background(), "ws-v2", "run-v2", 1, "stable-prefix"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("third request err = %v, want conflict", err)
	}
}

func TestRuntimeControllerCandidateExhaustionWithGovernanceCapabilityIsTerminal(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	facts := controllerFacts()
	facts.Capabilities.GovernanceKinds = []string{runtimecontract.RuntimeBlockKindDecisionRequest}
	if _, err := controller.Initialize(context.Background(), facts, controllerDefinition(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.BeginCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.FailCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidateFailure{Class: runtimecontract.RuntimeCandidateFailureClassProvider, Reason: "provider unavailable"}); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.BeginCandidateAttempt(context.Background(), "ws-v2", "run-v2", 1, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	terminal, err := controller.FailCandidateAttempt(context.Background(), "ws-v2", "run-v2", 1, runtimecontract.RuntimeCandidateFailure{Class: runtimecontract.RuntimeCandidateFailureClassProvider, Reason: "provider still unavailable"})
	if err != nil {
		t.Fatal(err)
	}
	if terminal.Decision.Action != runtimecontract.RuntimeActionFail || terminal.Decision.Terminal == nil || terminal.Decision.Terminal.Code != "candidate_retry_exhausted" {
		t.Fatalf("terminal decision = %+v", terminal.Decision)
	}
}

func TestRuntimeControllerCandidateExhaustionWithoutGovernanceIsTerminal(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	if _, err := controller.Initialize(context.Background(), controllerFacts(), controllerDefinition(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.BeginCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.FailCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidateFailure{Class: runtimecontract.RuntimeCandidateFailureClassContract, Reason: "invalid operation"}); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.BeginCandidateAttempt(context.Background(), "ws-v2", "run-v2", 1, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	terminal, err := controller.FailCandidateAttempt(context.Background(), "ws-v2", "run-v2", 1, runtimecontract.RuntimeCandidateFailure{Class: runtimecontract.RuntimeCandidateFailureClassContract, Reason: "invalid operation again"})
	if err != nil {
		t.Fatal(err)
	}
	if terminal.Decision.Action != runtimecontract.RuntimeActionFail || terminal.Decision.Terminal == nil || terminal.Decision.Terminal.Code != "candidate_retry_exhausted" {
		t.Fatalf("terminal decision = %+v", terminal.Decision)
	}
}

func TestRuntimeControllerInternalCandidateFailureIsImmediatelyTerminal(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	if _, err := controller.Initialize(context.Background(), controllerFacts(), controllerDefinition(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.BeginCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	terminal, err := controller.FailCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidateFailure{Class: runtimecontract.RuntimeCandidateFailureClassInternal, Reason: "transaction mismatch"})
	if err != nil {
		t.Fatal(err)
	}
	if terminal.Decision.Action != runtimecontract.RuntimeActionFail || terminal.Decision.Terminal == nil || terminal.Decision.Terminal.Code != "runtime_contract_defect" {
		t.Fatalf("terminal decision = %+v", terminal.Decision)
	}
}

func controllerFacts() runtimecontract.RuntimeFacts {
	return runtimecontract.RuntimeFacts{
		WorkspaceID: "ws-v2", RunID: "run-v2", DefinitionRevision: "definition-v2", EvidenceRevision: "evidence-v2-00000000000000000000",
		Focus: runtimecontract.CoverageFocus{
			ID: "focus-v2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, TargetRefs: []string{"target.txt"},
		},
		Capabilities: runtimecontract.RuntimeCapabilities{StateChangeKinds: []string{"file/write"}},
	}
}

func controllerDefinition() runtimecontract.RuntimeDefinitionContext {
	return runtimecontract.RuntimeDefinitionContext{
		Revision: "definition-v2", GoalID: "goal-v2", GoalDefinition: "完成目标",
		DoDID: "dod-v2", DoDDefinition: "产生可验证交付物", CapabilityContract: []string{"file/write"},
	}
}
