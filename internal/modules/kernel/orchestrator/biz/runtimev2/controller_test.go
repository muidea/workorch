package runtimev2

import (
	"context"
	"errors"
	"reflect"
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

func TestRuntimeControllerRestartReusesCommittedOwnerTargetSelection(t *testing.T) {
	ctx := context.Background()
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := controllerFacts()
	facts.Focus.TargetRefs = nil
	facts.Focus.TargetRole = "deliverable"
	facts.Focus.TargetDiscoverySources = []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest}
	facts.Focus.TargetSelectorRef = "file/unique_workspace_entry"
	facts.Focus.RequireTargetGrounding = true

	initialized, err := controller.Initialize(ctx, facts, controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var prerequisite runtimecontract.RuntimePrerequisite
	for _, value := range initialized.Facts.Prerequisites.Pending {
		if value.Kind == runtimecontract.PrerequisiteKindTargetSelection {
			prerequisite = value
			break
		}
	}
	if prerequisite.ID == "" {
		t.Fatalf("initial target-selection prerequisite missing: %+v", initialized.Facts.Prerequisites)
	}
	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: prerequisite.ID, OwnerModuleID: runtimecontract.RuntimeModuleFile, TargetRole: "deliverable", SelectorRef: "file/unique_workspace_entry",
		Disposition: runtimecontract.RuntimeTargetSelectionSelected, TargetRefs: []string{"implementation.go"}, CandidateTargetRefs: []string{"implementation.go"}, CandidateCount: 1,
		ObservedRevision: "tree-v1",
	}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: prerequisite.OwnerModuleID, ContentHash: selection.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable", Revision: "tree-v1"}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	committed, err := controller.CommitPrerequisite(ctx, facts.WorkspaceID, facts.RunID, PrerequisiteResult{PrerequisiteID: prerequisite.ID, TargetSelection: &selection, Observation: &observation})
	if err != nil {
		t.Fatal(err)
	}
	if committed.Facts.Resources.TargetSelection == nil || committed.Facts.Resources.TargetSelection.SnapshotID != selection.SnapshotID {
		t.Fatalf("committed target selection = %+v", committed.Facts.Resources.TargetSelection)
	}
	saves := repository.saves
	restarted, err := controller.Initialize(ctx, facts, controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if repository.saves != saves || restarted.StateRevision != committed.StateRevision || restarted.Facts.Resources.TargetSelection == nil || restarted.Facts.Resources.TargetSelection.SnapshotID != selection.SnapshotID {
		t.Fatalf("restart did not reuse committed selection: restarted=%+v saves=%d", restarted, repository.saves)
	}
	if _, err := controller.CommitPrerequisite(ctx, facts.WorkspaceID, facts.RunID, PrerequisiteResult{PrerequisiteID: prerequisite.ID, TargetSelection: &selection, Observation: &observation}); err == nil || repository.saves != saves {
		t.Fatalf("duplicate target selection changed persisted state: saves=%d err=%v", repository.saves, err)
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

func TestRuntimeControllerAssessmentGapDeltaResetsWindowButRetainsRemainingGap(t *testing.T) {
	ctx := context.Background()
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	facts := controllerFacts()
	facts.Focus.RequireAssessment = true
	facts.Focus.CoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-a", "cwi-b"}
	facts.Coverage = []runtimecontract.RuntimeCoverageItemFact{
		{ID: "cwi-a", State: runtimecontract.RuntimeCoverageItemStateOpen},
		{ID: "cwi-b", State: runtimecontract.RuntimeCoverageItemStateOpen},
	}
	if _, err := controller.Initialize(ctx, facts, controllerDefinition(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.BeginCandidateAttempt(ctx, "ws-v2", "run-v2", 0, "state-change-prefix"); err != nil {
		t.Fatal(err)
	}
	sealed, err := controller.AcceptCandidateAttempt(ctx, "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidate{
		StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
			ID: "write-both", Kind: "file/write", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-a", "cwi-b"},
			TargetRefs: []string{"target.txt"}, Parameters: runtimeV2FileWriteParameters("state"),
		}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	evidence := ExecutionEvidence{
		EventID: "event-state", WorkspaceID: "ws-v2", RunID: "run-v2", DispatchID: sealed.ActiveDispatch.ID,
		AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID, OperationID: "write-both",
		Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	}
	attachRuntimeV2TestOwnerReceipt(t, repository.aggregate, &evidence)
	committed, err := controller.CommitEvidence(ctx, evidence)
	if err != nil || committed.Aggregate.Decision.Action != runtimecontract.RuntimeActionProduceAssessment {
		t.Fatalf("state evidence did not reach assessment: aggregate=%+v err=%v", committed.Aggregate, err)
	}
	// Model a persisted prior repairable gap. The assessment below closes cwi-a
	// with current-run evidence and leaves cwi-b open, so it must reset only the
	// no-progress window, not erase the remaining repair target.
	repository.aggregate.Facts.Repair = runtimecontract.RuntimeRepairLedger{
		FocusID: repository.aggregate.Facts.Focus.ID, SliceID: repository.aggregate.Facts.Focus.SliceID,
		RepairAttempts: 2, ValidationOnlyRetries: 1, NoProgressCount: 2, LastCycleID: "prior-cycle",
		SignatureCounts:           []runtimecontract.RuntimeFailureSignatureCount{{Signature: "prior-gap", Count: 2}},
		NoProgressOperationShapes: []runtimecontract.RuntimeRejectedOperationShape{{Operation: runtimecontract.RuntimeOperation{Kind: runtimecontract.StepKindFileWrite}}},
		ActiveGap:                 &runtimecontract.RuntimeAssessmentGap{CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-a", "cwi-b"}, MissingEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, Reason: "both remain open"},
	}
	if _, err := controller.BeginCandidateAttempt(ctx, "ws-v2", "run-v2", 0, "assessment-prefix"); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.AcceptCandidateAttempt(ctx, "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidate{
		Assessment: &runtimecontract.AssessmentCandidate{
			FocusID: "focus-v2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-a", "cwi-b"},
			Disposition: runtimecontract.AssessmentDispositionRepairableGap, Summary: "one remaining item",
			Items: []runtimecontract.RuntimeAssessmentItemFact{
				{CoverageWorkItemID: "cwi-a", Satisfied: true, EvidenceRefs: []string{"event-state"}},
				{CoverageWorkItemID: "cwi-b", Satisfied: false, Reason: "still open"},
			},
			Gap: &runtimecontract.RuntimeAssessmentGap{CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-b"}, Reason: "one remaining item"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	progressed, err := controller.CommitAssessment(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	ledger := progressed.Facts.Repair
	if ledger.RepairAttempts != 3 || ledger.ValidationOnlyRetries != 0 || ledger.NoProgressCount != 0 || len(ledger.SignatureCounts) != 0 ||
		len(ledger.NoProgressOperationShapes) != 0 || ledger.LastCycleID != "" || ledger.ActiveGap == nil ||
		len(ledger.ActiveGap.CoverageWorkItemIDs) != 1 || ledger.ActiveGap.CoverageWorkItemIDs[0] != "cwi-b" || ledger.LastProgressRevision == "" {
		t.Fatalf("partial assessment delta did not preserve the remaining gap while resetting the window: %+v", ledger)
	}
	restarted, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := restarted.Load(ctx, "ws-v2", "run-v2")
	if err != nil || reloaded.StateRevision != progressed.StateRevision || !reflect.DeepEqual(reloaded.Facts.Repair, ledger) {
		t.Fatalf("restart did not preserve partial assessment progress transition: aggregate=%+v err=%v", reloaded, err)
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

func TestRuntimeControllerCandidateRetryExhaustionClosesAtomicallyWithFreshnessDebt(t *testing.T) {
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.Initialize(context.Background(), controllerFacts(), controllerDefinition(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.BeginCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.FailCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassSchema, Code: "candidate_schema_invalid", Signature: "candidate-schema-invalid", Reason: "unexpected EOF",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.BeginCandidateAttempt(context.Background(), "ws-v2", "run-v2", 1, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	repository.aggregate.Facts.FreshnessReacquireCoverage = append([]common.CoverageWorkItemID(nil), repository.aggregate.Facts.Focus.CoverageWorkItemIDs...)
	savesBeforeFailure := repository.saves

	terminal, err := controller.FailCandidateAttempt(context.Background(), "ws-v2", "run-v2", 1, runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassSchema, Code: "candidate_schema_invalid", Signature: "candidate-schema-invalid", Reason: "unexpected EOF",
	})
	if err != nil {
		t.Fatal(err)
	}
	if repository.saves != savesBeforeFailure+1 {
		t.Fatalf("attempt failure saves = %d, want one atomic save after %d", repository.saves, savesBeforeFailure)
	}
	if terminal.Decision.Action != runtimecontract.RuntimeActionFail || terminal.Decision.Terminal == nil || terminal.Decision.Terminal.Code != "candidate_retry_exhausted" || hasPendingCandidateAttempt(terminal.CandidateAttempts) {
		t.Fatalf("attempt 1 failure did not atomically close terminal authority: %+v", terminal)
	}
	last := terminal.CandidateAttempts[len(terminal.CandidateAttempts)-1]
	if last.Attempt != 1 || last.State != runtimecontract.RuntimeCandidateAttemptStateFailed || last.Failure == nil || last.Failure.Code != "candidate_schema_invalid" {
		t.Fatalf("attempt 1 record = %+v", last)
	}
}

func TestRuntimeControllerTerminalDriveFailureClosesPendingCandidateWithFreshnessDebt(t *testing.T) {
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.Initialize(context.Background(), controllerFacts(), controllerDefinition(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.BeginCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	repository.aggregate.Facts.FreshnessReacquireCoverage = append([]common.CoverageWorkItemID(nil), repository.aggregate.Facts.Focus.CoverageWorkItemIDs...)
	savesBeforeFailure := repository.saves

	terminal, err := controller.RecordFailureFact(context.Background(), "ws-v2", "run-v2", runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "runtime_v2_drive_failed",
		Signature: "runtime-v2-drive-failed", Message: "candidate transaction could not be closed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if repository.saves != savesBeforeFailure+1 {
		t.Fatalf("drive failure saves = %d, want one atomic save after %d", repository.saves, savesBeforeFailure)
	}
	if terminal.Decision.Action != runtimecontract.RuntimeActionFail || terminal.Decision.Terminal == nil || terminal.Decision.Terminal.Code != "runtime_contract_defect" || hasPendingCandidateAttempt(terminal.CandidateAttempts) {
		t.Fatalf("drive failure did not close terminal authority and candidate transaction: %+v", terminal)
	}
	attempt := terminal.CandidateAttempts[len(terminal.CandidateAttempts)-1]
	if attempt.State != runtimecontract.RuntimeCandidateAttemptStateFailed || attempt.Failure == nil || attempt.Failure.Class != runtimecontract.RuntimeCandidateFailureClassInternal || attempt.Failure.Code != "runtime_v2_drive_failed" {
		t.Fatalf("terminal candidate closure = %+v", attempt)
	}
}

func TestRuntimeControllerReopensInvalidatedAttemptZeroWithoutCandidateDebt(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	transaction := aggregate.Decision.AuthorityTransaction()
	stale := runtimecontract.RuntimePrerequisiteStale{
		PrerequisiteID: "operation-admissibility", ObservationID: "observation-v1",
		Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		SubjectRef: "workspace_deliverable", ExpectedRevision: "tree-v1", ObservedRevision: "tree-v2", Reason: "workspace changed",
	}
	signature := runtimecontract.StableRuntimePrerequisiteStaleSignature(stale)
	aggregate.CandidateAttempts = []runtimecontract.RuntimeCandidateAttemptRecord{{
		AuthorityContractID: runtimecontract.StableAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, transaction),
		Transaction:         transaction, Attempt: 0, StablePrefixHash: "stable-prefix", State: runtimecontract.RuntimeCandidateAttemptStateInvalidated,
		Stale: &stale,
	}}
	aggregate.PrerequisiteInvalidations = []runtimecontract.RuntimePrerequisiteInvalidationRecord{{
		Signature: signature, PrerequisiteIDs: []string{stale.PrerequisiteID}, ObservationIDs: []string{stale.ObservationID}, Stale: &stale,
	}}
	repository := &controllerRepository{aggregate: &aggregate}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}

	beforeRevision := aggregate.StateRevision
	if _, err := controller.BeginCandidateAttempt(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, "drifted-prefix"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("stable-prefix drift err=%v, want conflict", err)
	}
	if repository.saves != 0 || repository.aggregate.StateRevision != beforeRevision {
		t.Fatalf("stable-prefix drift mutated aggregate: revision=%d saves=%d", repository.aggregate.StateRevision, repository.saves)
	}
	reopened, err := controller.BeginCandidateAttempt(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, "stable-prefix")
	if err != nil {
		t.Fatal(err)
	}
	if reopened.StateRevision != beforeRevision+1 || len(reopened.CandidateAttempts) != 1 {
		t.Fatalf("invalidated attempt was not atomically reopened: %+v", reopened)
	}
	record := reopened.CandidateAttempts[0]
	if record.Attempt != 0 || record.State != runtimecontract.RuntimeCandidateAttemptStatePending || record.StablePrefixHash != "stable-prefix" ||
		record.Transaction != reopened.Decision.AuthorityTransaction() || record.Proposal != nil || len(record.Feasibilities) != 0 || record.Failure != nil || record.Stale != nil {
		t.Fatalf("reopened attempt = %+v", record)
	}
	if len(reopened.PrerequisiteInvalidations) != 1 || reopened.PrerequisiteInvalidations[0].Signature != signature {
		t.Fatalf("freshness audit was not retained: %+v", reopened.PrerequisiteInvalidations)
	}
	if repository.saves != 1 {
		t.Fatalf("reopen saves=%d, want 1", repository.saves)
	}
	duplicate, err := controller.BeginCandidateAttempt(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, "stable-prefix")
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.StateRevision != reopened.StateRevision || repository.saves != 1 || len(duplicate.CandidateAttempts) != 1 {
		t.Fatalf("duplicate reopen was not idempotent: aggregate=%+v saves=%d", duplicate, repository.saves)
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

func TestFailInfeasibleValidationCandidateRoutesToSealedStateChangeRepair(t *testing.T) {
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := controllerFacts()
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	facts.Capabilities = runtimecontract.RuntimeCapabilities{
		StateChangeKinds: []string{runtimecontract.StepKindFileWrite},
		ValidationKinds:  []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
	}
	facts.Activation.ExecutionContract.ValidationPolicies = []runtimecontract.RuntimeValidationPolicy{{
		ID: "validation", Scope: "workspace_profile", AppliesToEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
		ValidationFailureImpliesStateChange: true,
	}}
	initialized, err := controller.Initialize(context.Background(), facts, controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if initialized.Decision.Action != runtimecontract.RuntimeActionProduceValidation {
		t.Fatalf("initial decision = %+v, want validation", initialized.Decision)
	}
	if _, err := controller.BeginCandidateAttempt(context.Background(), facts.WorkspaceID, facts.RunID, 0, "validation-prefix"); err != nil {
		t.Fatal(err)
	}
	repaired, err := controller.FailCandidateAttempt(context.Background(), facts.WorkspaceID, facts.RunID, 0, runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassInfeasible, Code: "owner_target_missing", Signature: "owner/target/missing", Reason: "validation target is unavailable",
	})
	if err != nil {
		t.Fatal(err)
	}
	if repaired.Decision.Action != runtimecontract.RuntimeActionProduceStateChange || repaired.Decision.RequiredOutput.Kind != runtimecontract.RequiredOutputKindStateChange {
		t.Fatalf("infeasible validation did not route to state-change repair: %+v", repaired.Decision)
	}
	if repaired.Facts.Failure == nil || repaired.Facts.Failure.Class != runtimecontract.FailureClassCandidateInfeasible || repaired.Facts.Failure.Code != "owner_target_missing" {
		t.Fatalf("repair failure fact = %+v", repaired.Facts.Failure)
	}
	if repaired.Facts.Repair.RepairAttempts != 1 || repaired.Facts.Repair.ActiveGap == nil || repaired.Facts.Repair.LastCycleID == "" {
		t.Fatalf("candidate infeasible did not start repair cycle: %+v", repaired.Facts.Repair)
	}
	if got := repaired.Facts.Repair.ActiveGap.MissingEvidenceKinds; len(got) != 1 || got[0] != runtimecontract.EvidenceKindValidation {
		t.Fatalf("candidate infeasible gap evidence = %v", got)
	}
	if repaired.Facts.Repair.ActiveGap.FailureSignature != "owner/target/missing" || len(repaired.Facts.Repair.SignatureCounts) != 1 || repaired.Facts.Repair.SignatureCounts[0].Count != 1 {
		t.Fatalf("candidate infeasible gap signature ledger = %+v", repaired.Facts.Repair)
	}
	if len(repaired.CandidateAttempts) != 1 || repaired.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateFailed {
		t.Fatalf("original validation attempt was not retained as a failed admission record: %+v", repaired.CandidateAttempts)
	}
}

func TestFailInfeasibleValidationCandidateExhaustsSameFocusRepairBudget(t *testing.T) {
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := controllerFacts()
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	facts.Capabilities = runtimecontract.RuntimeCapabilities{
		StateChangeKinds: []string{runtimecontract.StepKindFileWrite},
		ValidationKinds:  []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
	}
	facts.ConvergencePolicy = runtimecontract.RuntimeConvergencePolicy{
		MaxRepairAttemptsPerFocus: 1, MaxValidationOnlyRetriesPerFocus: 1,
		MaxAcceptanceSliceWorkItems: 1, MaxFocuses: 1, MaxCandidateContextItems: 1,
	}
	facts.Activation.ExecutionContract.ValidationPolicies = []runtimecontract.RuntimeValidationPolicy{{
		ID: "validation", Scope: "workspace_profile", AppliesToEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
		ValidationFailureImpliesStateChange: true,
	}}
	if _, err := controller.Initialize(context.Background(), facts, controllerDefinition(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.BeginCandidateAttempt(context.Background(), facts.WorkspaceID, facts.RunID, 0, "validation-prefix"); err != nil {
		t.Fatal(err)
	}
	terminal, err := controller.FailCandidateAttempt(context.Background(), facts.WorkspaceID, facts.RunID, 0, runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassInfeasible, Code: "owner_target_missing", Signature: "owner/target/missing", Reason: "validation target is unavailable",
	})
	if err != nil {
		t.Fatal(err)
	}
	if terminal.Facts.Repair.RepairAttempts != 1 || terminal.Decision.Action != runtimecontract.RuntimeActionFail || terminal.Decision.Terminal == nil || terminal.Decision.Terminal.Code != "no_progress_exhausted" {
		t.Fatalf("candidate infeasible repair budget did not terminal atomically: %+v", terminal)
	}
}

func TestFailInfeasibleValidationCandidateKeepsValidationWhenPolicyAllowsAlternative(t *testing.T) {
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := controllerFacts()
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	facts.Capabilities = runtimecontract.RuntimeCapabilities{
		StateChangeKinds: []string{runtimecontract.StepKindFileWrite},
		ValidationKinds:  []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
	}
	facts.Activation.ExecutionContract.ValidationPolicies = []runtimecontract.RuntimeValidationPolicy{{
		ID: "validation", Scope: "workspace_profile", AppliesToEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
		ValidationFailureImpliesStateChange: false,
	}}
	initialized, err := controller.Initialize(context.Background(), facts, controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if initialized.Decision.Action != runtimecontract.RuntimeActionProduceValidation {
		t.Fatalf("initial decision = %+v, want validation", initialized.Decision)
	}
	if _, err := controller.BeginCandidateAttempt(context.Background(), facts.WorkspaceID, facts.RunID, 0, "validation-prefix"); err != nil {
		t.Fatal(err)
	}
	continued, err := controller.FailCandidateAttempt(context.Background(), facts.WorkspaceID, facts.RunID, 0, runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassInfeasible, Code: "owner_target_missing", Reason: "validation target is unavailable",
	})
	if err != nil {
		t.Fatal(err)
	}
	if continued.Decision.Action != runtimecontract.RuntimeActionProduceValidation || continued.Facts.Failure != nil {
		t.Fatalf("alternative-validation policy changed authority unexpectedly: %+v", continued)
	}
}

func TestFailInfeasibleValidationCommandDiagnosticKeepsSameContractRetry(t *testing.T) {
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := controllerFacts()
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	facts.Capabilities = runtimecontract.RuntimeCapabilities{
		StateChangeKinds: []string{runtimecontract.StepKindFileWrite},
		ValidationKinds:  []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
	}
	facts.Activation.ExecutionContract.ValidationPolicies = []runtimecontract.RuntimeValidationPolicy{{
		ID: "validation", Scope: "workspace_profile", AppliesToEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
		ValidationFailureImpliesStateChange: true,
		ImplementationGapDiagnosticClasses:  []string{runtimecontract.ValidationDiagnosticBuildFailed},
	}}
	initialized, err := controller.Initialize(context.Background(), facts, controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.BeginCandidateAttempt(context.Background(), facts.WorkspaceID, facts.RunID, 0, "validation-prefix"); err != nil {
		t.Fatal(err)
	}
	continued, err := controller.FailCandidateAttempt(context.Background(), facts.WorkspaceID, facts.RunID, 0, runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassInfeasible, Code: "validation_target_missing", Signature: "validation-target-missing", Reason: "package target is unavailable",
		ValidationDiagnostic: &runtimecontract.ValidationFailureDiagnostic{
			SchemaVersion: runtimecontract.ValidationFailureDiagnosticSchemaVersion,
			Class:         runtimecontract.ValidationDiagnosticCommandInvalid, RecommendedEvidenceTargetType: runtimecontract.ValidationEvidenceTargetCommandContract,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if continued.Decision.AuthoritySnapshot.ID != initialized.Decision.AuthoritySnapshot.ID || continued.Decision.Action != runtimecontract.RuntimeActionProduceValidation || continued.Facts.Failure != nil || continued.Facts.Repair.RepairAttempts != 0 {
		t.Fatalf("command diagnostic changed SM-2 repair authority: %+v", continued)
	}
	if len(continued.CandidateAttempts) != 1 || continued.CandidateAttempts[0].Failure == nil || continued.CandidateAttempts[0].Failure.ValidationDiagnostic == nil {
		t.Fatalf("command diagnostic was not retained for same-contract retry: %+v", continued.CandidateAttempts)
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
