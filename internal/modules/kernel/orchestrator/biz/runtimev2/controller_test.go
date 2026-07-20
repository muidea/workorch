package runtimev2

import (
	"context"
	"errors"
	"strings"
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
	if got, want := initialized.Facts.Activation.ExecutionContract.OperationContractRevision, runtimecontract.RuntimeOperationContractRevision(); got != want {
		t.Fatalf("operation contract revision = %q, want %q", got, want)
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

func TestCommitPrerequisiteAdvancesSatisfiedFocusAfterFreshnessReacquisition(t *testing.T) {
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := controllerFacts()
	facts.Focus = runtimecontract.CoverageFocus{
		ID: "focus-satisfied", SliceID: "slice-satisfied", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-satisfied"},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, AcceptedStepKinds: []string{runtimecontract.StepKindFileWrite},
	}
	facts.RemainingFocuses = []runtimecontract.CoverageFocus{{
		ID: "focus-next", SliceID: "slice-next", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-next"},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, AcceptedStepKinds: []string{runtimecontract.StepKindFileWrite},
	}}
	facts.Coverage = []runtimecontract.RuntimeCoverageItemFact{
		{ID: "cwi-satisfied", SliceID: "slice-satisfied", State: runtimecontract.RuntimeCoverageItemStateSatisfied, EvidenceRefs: []string{"accepted-satisfied"}},
		{ID: "cwi-next", SliceID: "slice-next", State: runtimecontract.RuntimeCoverageItemStateOpen},
	}
	facts.Evidence.Accepted = []runtimecontract.EvidenceFact{{
		ID: "accepted-satisfied", Kind: runtimecontract.EvidenceKindStateChange, Revision: facts.EvidenceRevision,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-satisfied"},
	}}
	requirements := []RuntimeRequirement{{Kind: runtimecontract.PrerequisiteKindTargetSnapshot, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "target.txt", Reason: "fresh target"}}
	aggregate, err := controller.Initialize(context.Background(), facts, controllerDefinition(), requirements)
	if err != nil {
		t.Fatal(err)
	}
	if aggregate.Decision.Action != runtimecontract.RuntimeActionAcquirePrerequisite || len(aggregate.Facts.Prerequisites.Pending) != 1 {
		t.Fatalf("initialized aggregate = %+v", aggregate)
	}

	committed, err := controller.CommitPrerequisite(context.Background(), "ws-v2", "run-v2", PrerequisiteResult{
		PrerequisiteID: aggregate.Facts.Prerequisites.Pending[0].ID,
		TargetSnapshot: &runtimecontract.TargetSnapshot{TargetRef: "target.txt", Content: "fresh", ContentHash: "fresh-hash", ContentState: runtimecontract.SnapshotContentStateComplete},
	})
	if err != nil {
		t.Fatal(err)
	}
	if committed.Facts.Focus.ID != "focus-next" || len(committed.Facts.RemainingFocuses) != 0 || committed.Decision.Focus.ID != "focus-next" || committed.Decision.Action != runtimecontract.RuntimeActionProduceStateChange {
		t.Fatalf("fresh prerequisite did not advance satisfied focus: %+v", committed)
	}
}

func TestRuntimeControllerInitializeRejectsPersistedOperationContractRevisionMismatch(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	if _, err := controller.Initialize(context.Background(), controllerFacts(), controllerDefinition(), nil); err != nil {
		t.Fatal(err)
	}
	repository.aggregate.Facts.Activation.ExecutionContract.OperationContractRevision = "operation-contract-obsolete"

	_, err := controller.Initialize(context.Background(), controllerFacts(), controllerDefinition(), nil)
	var mismatch *runtimecontract.RuntimeOperationContractRevisionMismatchError
	if !errors.As(err, &mismatch) || !errors.Is(err, runtimecontract.ErrRuntimeOperationContractRevisionMismatch) {
		t.Fatalf("revision mismatch err = %v", err)
	}
	if mismatch.PersistedRevision != "operation-contract-obsolete" || repository.saves != 1 {
		t.Fatalf("mismatch=%+v saves=%d", mismatch, repository.saves)
	}
}

func TestRuntimeControllerInitializeRejectsPersistedMissingOperationContractRevision(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	if _, err := controller.Initialize(context.Background(), controllerFacts(), controllerDefinition(), nil); err != nil {
		t.Fatal(err)
	}
	repository.aggregate.Facts.Activation.ExecutionContract.OperationContractRevision = ""

	_, err := controller.Initialize(context.Background(), controllerFacts(), controllerDefinition(), nil)
	var mismatch *runtimecontract.RuntimeOperationContractRevisionMismatchError
	if !errors.As(err, &mismatch) || mismatch.PersistedRevision != "" || repository.saves != 1 {
		t.Fatalf("missing revision err=%v mismatch=%+v saves=%d", err, mismatch, repository.saves)
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

func TestRuntimeControllerInitializeAcceptsAdvancedFocusOnRestart(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	facts := controllerFacts()
	definition := controllerDefinition()
	initialized, err := controller.Initialize(context.Background(), facts, definition, nil)
	if err != nil {
		t.Fatal(err)
	}
	repository.aggregate.Facts.Focus.ID = "focus-v2-next"
	repository.aggregate.Decision.Focus.ID = "focus-v2-next"

	restarted, err := controller.Initialize(context.Background(), facts, definition, nil)
	if err != nil {
		t.Fatalf("advanced-focus restart: %v", err)
	}
	if restarted.Facts.Focus.ID != "focus-v2-next" || restarted.StateRevision != initialized.StateRevision {
		t.Fatalf("restart replaced progressed aggregate: %+v", restarted)
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

func TestRuntimeControllerRecordsInfeasibleCandidateAsReducerFact(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	facts := controllerFacts()
	facts.Capabilities.GovernanceKinds = []string{runtimecontract.RuntimeBlockKindDecisionRequest}
	aggregate, err := controller.Initialize(context.Background(), facts, controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	progressed, err := controller.RecordFeasibilityFact(context.Background(), "ws-v2", "run-v2", runtimecontract.RuntimeFeasibilityFact{
		FocusID: aggregate.Facts.Focus.ID, Action: runtimecontract.RuntimeActionProduceStateChange,
		RequiredOperationKinds: []string{"file/write"}, MissingCapabilityKinds: []string{"file/write"}, Reason: "owner capability unavailable",
	})
	if err != nil {
		t.Fatal(err)
	}
	if progressed.Decision.Action != runtimecontract.RuntimeActionRequestGovernance || progressed.Facts.Feasibility == nil || len(progressed.CandidateAttempts) != 0 {
		t.Fatalf("feasibility transition = %+v", progressed)
	}
}

func TestRuntimeControllerRecordsFailureFactBeforeTerminalDecision(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	if _, err := controller.Initialize(context.Background(), controllerFacts(), controllerDefinition(), nil); err != nil {
		t.Fatal(err)
	}
	terminal, err := controller.RecordFailureFact(context.Background(), "ws-v2", "run-v2", runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "runtime_v2_drive_failed",
		Signature: "runtime-v2-drive-failed", Message: "authority transaction mismatch",
	})
	if err != nil {
		t.Fatal(err)
	}
	if terminal.Facts.Failure == nil || terminal.Facts.Failure.Code != "runtime_v2_drive_failed" ||
		terminal.Decision.Action != runtimecontract.RuntimeActionFail || terminal.Decision.Terminal == nil || terminal.Decision.Terminal.Code != "runtime_contract_defect" {
		t.Fatalf("failure fact terminal = %+v", terminal)
	}
	saves := repository.saves
	replayed, err := controller.RecordFailureFact(context.Background(), "ws-v2", "run-v2", runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassRuntimeContractMisalignment, Message: "replayed drive failure",
	})
	if err != nil || repository.saves != saves || replayed.StateRevision != terminal.StateRevision {
		t.Fatalf("terminal failure replay = %+v saves=%d err=%v", replayed, repository.saves, err)
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
		ID: "write-1", Kind: "file/write", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeV2FileWriteParameters("state"),
	}}}}}
	if _, err := controller.BeginCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	sealed, err := controller.AcceptCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, candidate)
	if err != nil {
		t.Fatalf("AcceptCandidateAttempt: %v", err)
	}
	commitEvidence := ExecutionEvidence{
		EventID: "event-1", WorkspaceID: "ws-v2", RunID: "run-v2",
		DispatchID: sealed.ActiveDispatch.ID, AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		OperationID: "write-1", Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	}
	attachRuntimeV2TestOwnerReceipt(t, repository.aggregate, &commitEvidence)
	result, err := controller.CommitEvidence(context.Background(), commitEvidence)
	if err != nil || result.Disposition != CommitDispositionCommitted {
		t.Fatalf("CommitEvidence = %+v err=%v", result, err)
	}
	if result.Aggregate.Decision.Action != runtimecontract.RuntimeActionComplete {
		t.Fatalf("next action = %s", result.Aggregate.Decision.Action)
	}
}

func TestRuntimeControllerRejectsExecutableSealWithoutPersistedOwnerFeasibility(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	facts := controllerFacts()
	inventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "operation_admissibility:file", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.StepKindFileWrite}, TargetRefs: []string{"target.txt"}, ObservedRevision: "inventory-revision",
	}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{inventory}
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{{
		ID: inventory.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: runtimecontract.RuntimeModuleFile, RequiredCapabilities: []string{runtimecontract.StepKindFileWrite},
	}}
	if _, err := controller.Initialize(context.Background(), facts, controllerDefinition(), nil); err != nil {
		t.Fatal(err)
	}
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "write-1", Kind: runtimecontract.StepKindFileWrite, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeV2FileWriteParameters("state"),
	}}}}}
	if _, err := controller.BeginCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.AcceptCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, candidate); err == nil || !strings.Contains(err.Error(), "no current persisted feasibility result") {
		t.Fatalf("sealed executable candidate without owner feasibility: %v", err)
	}
	if _, err := controller.RecordCandidateProposal(context.Background(), "ws-v2", "run-v2", 0, candidate); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.RecordCandidateFeasibility(context.Background(), "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidateFeasibilityRecord{
		OperationID: "write-1", AdmissibilitySnapshotID: inventory.SnapshotID, Feasible: true,
	}); err != nil {
		t.Fatal(err)
	}
	if sealed, err := controller.AcceptCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, candidate); err != nil || sealed.ActiveDispatch == nil {
		t.Fatalf("sealed candidate with persisted owner feasibility = %+v err=%v", sealed, err)
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
		ID: "write-1", Kind: "file/write", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeV2FileWriteParameters("state"),
	}}}}})
	if err != nil {
		t.Fatal(err)
	}
	commitEvidence := ExecutionEvidence{
		EventID: "event-1", WorkspaceID: "ws-v2", RunID: "run-v2", DispatchID: sealed.ActiveDispatch.ID,
		AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID, OperationID: "write-1",
		Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	}
	attachRuntimeV2TestOwnerReceipt(t, repository.aggregate, &commitEvidence)
	evidence, err := controller.CommitEvidence(context.Background(), commitEvidence)
	if err != nil || evidence.Aggregate.Decision.Action != runtimecontract.RuntimeActionProduceAssessment {
		t.Fatalf("assessment authority = %+v err=%v", evidence.Aggregate.Decision, err)
	}
	if _, err := controller.BeginCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, "assessment-prefix"); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.AcceptCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{
		FocusID: "focus-v2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, Disposition: runtimecontract.AssessmentDispositionSatisfied,
		Summary: "current focus is satisfied", EvidenceRefs: []string{"event-1"},
	}}); err != nil {
		t.Fatal(err)
	}
	completed, err := controller.CommitAssessment(context.Background(), "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if completed.Decision.Action != runtimecontract.RuntimeActionComplete || len(completed.Facts.Assessments) != 1 || completed.Facts.Assessments[0].Disposition != runtimecontract.AssessmentDispositionSatisfied {
		t.Fatalf("completed assessment aggregate = %+v", completed)
	}
}

func TestRuntimeControllerRejectsSatisfiedAssessmentItemWithUnknownEvidenceRef(t *testing.T) {
	controller := prepareAssessmentController(t, runtimecontract.AssessmentCandidate{
		FocusID: "focus-v2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
		Disposition: runtimecontract.AssessmentDispositionSatisfied, Summary: "satisfied", EvidenceRefs: []string{"event-1"},
		Items: []runtimecontract.RuntimeAssessmentItemFact{{CoverageWorkItemID: "cwi-v2", Satisfied: true, EvidenceRefs: []string{"unknown-evidence"}}},
	})
	if _, err := controller.CommitAssessment(context.Background(), "ws-v2", "run-v2"); err == nil || !strings.Contains(err.Error(), "not accepted current-focus evidence") {
		t.Fatalf("CommitAssessment unknown item ref err = %v", err)
	}
}

func TestRuntimeControllerSatisfiedAssessmentItemInheritsTopLevelEvidenceRefs(t *testing.T) {
	controller := prepareAssessmentController(t, runtimecontract.AssessmentCandidate{
		FocusID: "focus-v2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
		Disposition: runtimecontract.AssessmentDispositionSatisfied, Summary: "satisfied", EvidenceRefs: []string{"event-1"},
		Items: []runtimecontract.RuntimeAssessmentItemFact{{CoverageWorkItemID: "cwi-v2", Satisfied: true}},
	})
	completed, err := controller.CommitAssessment(context.Background(), "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if completed.Decision.Action != runtimecontract.RuntimeActionComplete || len(completed.Facts.Coverage) != 1 ||
		len(completed.Facts.Coverage[0].EvidenceRefs) != 1 || completed.Facts.Coverage[0].EvidenceRefs[0] != "event-1" {
		t.Fatalf("assessment item evidence inheritance = %+v", completed)
	}
	if len(completed.Facts.Assessments) != 1 || len(completed.Facts.Assessments[0].Items) != 1 ||
		len(completed.Facts.Assessments[0].Items[0].EvidenceRefs) != 1 || completed.Facts.Assessments[0].Items[0].EvidenceRefs[0] != "event-1" {
		t.Fatalf("assessment fact did not persist inherited item evidence: %+v", completed.Facts.Assessments)
	}
}

func prepareAssessmentController(t *testing.T, assessment runtimecontract.AssessmentCandidate) *RuntimeController {
	t.Helper()
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	facts := controllerFacts()
	facts.Focus.RequireAssessment = true
	if _, err := controller.Initialize(context.Background(), facts, controllerDefinition(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.BeginCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, "state-change-prefix"); err != nil {
		t.Fatal(err)
	}
	sealed, err := controller.AcceptCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "write-1", Kind: "file/write", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeV2FileWriteParameters("state"),
	}}}}})
	if err != nil {
		t.Fatal(err)
	}
	commitEvidence := ExecutionEvidence{
		EventID: "event-1", WorkspaceID: "ws-v2", RunID: "run-v2", DispatchID: sealed.ActiveDispatch.ID,
		AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID, OperationID: "write-1",
		Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	}
	attachRuntimeV2TestOwnerReceipt(t, repository.aggregate, &commitEvidence)
	evidence, err := controller.CommitEvidence(context.Background(), commitEvidence)
	if err != nil || evidence.Aggregate.Decision.Action != runtimecontract.RuntimeActionProduceAssessment {
		t.Fatalf("prepare assessment evidence = %+v err=%v", evidence, err)
	}
	if _, err := controller.BeginCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, "assessment-prefix"); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.AcceptCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidate{Assessment: &assessment}); err != nil {
		t.Fatal(err)
	}
	return controller
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

func TestRuntimeControllerCandidateFailureDoesNotResealPreExistingExecutionFailure(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	facts := controllerFacts()
	facts.Failure = &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassExecutionFailure, Code: "base_hash_mismatch",
		Signature: "patch-base-hash", Message: "patch baseHash mismatch",
	}
	facts.Repair = runtimecontract.RuntimeRepairLedger{FocusID: facts.Focus.ID, RepairAttempts: 1}
	initialized, err := controller.Initialize(context.Background(), facts, controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := controller.BeginCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, "stable-prefix")
	if err != nil {
		t.Fatal(err)
	}
	failed, err := controller.FailCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassContract, Reason: "candidate schema does not satisfy required output",
	})
	if err != nil {
		t.Fatal(err)
	}
	if failed.Decision.AuthoritySnapshot != pending.Decision.AuthoritySnapshot || failed.Decision.AuthoritySnapshot != initialized.Decision.AuthoritySnapshot {
		t.Fatalf("attempt 0 failure re-sealed authority: initialized=%+v pending=%+v failed=%+v", initialized.Decision.AuthoritySnapshot, pending.Decision.AuthoritySnapshot, failed.Decision.AuthoritySnapshot)
	}
	if failed.Facts.Failure == nil || failed.Facts.Failure.Signature != "patch-base-hash" {
		t.Fatalf("attempt 0 failure replaced execution failure: %+v", failed.Facts.Failure)
	}
	retried, err := controller.BeginCandidateAttempt(context.Background(), "ws-v2", "run-v2", 1, "stable-prefix")
	if err != nil {
		t.Fatal(err)
	}
	if len(retried.CandidateAttempts) != 2 || retried.CandidateAttempts[0].AuthorityContractID == "" || retried.CandidateAttempts[0].AuthorityContractID != retried.CandidateAttempts[1].AuthorityContractID {
		t.Fatalf("candidate retry did not preserve authority contract identity: %+v", retried.CandidateAttempts)
	}
}

func TestRuntimeControllerRejectsRetryWhenPersistedContractIDIsMissing(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	if _, err := controller.Initialize(context.Background(), controllerFacts(), controllerDefinition(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.BeginCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	failed, err := controller.FailCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassContract, Reason: "invalid candidate",
	})
	if err != nil {
		t.Fatal(err)
	}
	failed.CandidateAttempts[0].AuthorityContractID = ""
	resealed, err := ReduceRuntimeState(failed.Facts, &failed.Decision.AuthoritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	failed.Decision = resealed
	failed.StateRevision++
	repository.aggregate = &failed
	if _, err := controller.BeginCandidateAttempt(context.Background(), "ws-v2", "run-v2", 1, "stable-prefix"); err == nil {
		t.Fatal("candidate retry accepted persisted attempt without authority contract id")
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
		Activation:  runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		WorkspaceID: "ws-v2", RunID: "run-v2", DefinitionRevision: "definition-v2", EvidenceRevision: "evidence-v2-00000000000000000000",
		Focus: runtimecontract.CoverageFocus{
			ID: "focus-v2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, TargetRefs: []string{"target.txt"},
		},
		Coverage:     []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-v2", State: runtimecontract.RuntimeCoverageItemStateOpen}},
		Capabilities: runtimecontract.RuntimeCapabilities{StateChangeKinds: []string{"file/write"}},
	}
}

func controllerDefinition() runtimecontract.RuntimeDefinitionContext {
	return runtimecontract.RuntimeDefinitionContext{
		Revision: "definition-v2", GoalID: "goal-v2", GoalDefinition: "完成目标",
		DoDID: "dod-v2", DoDDefinition: "产生可验证交付物", CapabilityContract: []string{"file/write"},
	}
}
