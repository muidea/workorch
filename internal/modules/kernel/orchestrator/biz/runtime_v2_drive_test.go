package orchestrator

// Owner Biz drive tests.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	decisionmodule "github.com/muidea/workorch/internal/modules/blocks/decision"
	filemodule "github.com/muidea/workorch/internal/modules/blocks/file"
	gitmodule "github.com/muidea/workorch/internal/modules/blocks/git"
	gitbiz "github.com/muidea/workorch/internal/modules/blocks/git/biz"
	llmmodule "github.com/muidea/workorch/internal/modules/blocks/llm"
	resultmodule "github.com/muidea/workorch/internal/modules/blocks/result"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	runmodule "github.com/muidea/workorch/internal/modules/blocks/run"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	shellmodule "github.com/muidea/workorch/internal/modules/blocks/shell"
	shellbiz "github.com/muidea/workorch/internal/modules/blocks/shell/biz"
	workspacemodule "github.com/muidea/workorch/internal/modules/blocks/workspace"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runtimev2 "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtimev2"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	filerepository "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type panicAfterOwnerReceiptExecutor struct {
	delegate eventRuntimeV2OperationExecutor
}

// panicAfterOwnerReceiptCommitErrorExecutor models process loss after a
// capability owner applied its durable effect but before the owner receipt CAS
// was committed. The nested EventHub error is intentionally not converted into
// an outcome_unknown transition: the restart must use only the sealed proof.
type panicAfterOwnerReceiptCommitErrorExecutor struct {
	delegate eventRuntimeV2OperationExecutor
}

// countingRuntimeV2OperationExecutor keeps the P5-C process-restart fixture
// honest: both the initial governance-only attempt and the granted resume go
// through the real EventHub/File owner, while duplicate or stale governance
// resolutions must never invoke an additional owner operation.
type countingRuntimeV2OperationExecutor struct {
	delegate eventRuntimeV2OperationExecutor
	requests []runtimev2.RuntimeOperationExecutionRequest
}

func (e *countingRuntimeV2OperationExecutor) ExecuteRuntimeOperation(ctx context.Context, request runtimev2.RuntimeOperationExecutionRequest) (runtimev2.RuntimeOperationExecutionResult, error) {
	e.requests = append(e.requests, request)
	return e.delegate.ExecuteRuntimeOperation(ctx, request)
}

type failOnceResultRepository struct {
	repository.ResultRepository
	remainingFailures int
}

type failOnceCASRuntimeV2Repository struct {
	delegate          runtimev2.RuntimeStateRepository
	remainingFailures int
}

// runtimeV2FocusAdvanceReceiptExecutor stands in for an already admissible
// owner receipt while exercising the SM-2 evidence/coverage/focus CAS. Durable
// effect replay belongs to P5; this fixture proves the next authority cursor
// is atomic once that receipt has been persisted.
type runtimeV2FocusAdvanceReceiptExecutor struct {
	owner *runtimev2.RuntimeController
	calls int
}

func (e *runtimeV2FocusAdvanceReceiptExecutor) ExecuteRuntimeOperation(ctx context.Context, request runtimev2.RuntimeOperationExecutionRequest) (runtimev2.RuntimeOperationExecutionResult, error) {
	e.calls++
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: request.Execution.ExecutionKey, OperationID: request.Operation.ID,
		Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: request.Execution.ExpectedEffectScope,
		Changed:  runtimecontract.RuntimeEffectIsDurable(request.Execution.ExpectedEffectScope),
		Evidence: runtimecontract.RuntimeOperationEvidence{Summary: "focus-one receipt committed"},
	}
	_, committed, err := e.owner.CommitOperationReceipt(ctx, request.WorkspaceID, request.RunID, request.DispatchID, request.Execution.Fence, receipt)
	if err != nil {
		return runtimev2.RuntimeOperationExecutionResult{}, err
	}
	return runtimev2.RuntimeOperationExecutionResult{
		EventID: "focus-advance-evidence-" + request.Operation.ID, OperationID: request.Operation.ID,
		EvidenceKind: request.EvidenceKind, Status: runtimev2.ExecutionEvidenceStatusAccepted,
		SourceRef: "runtime://focus-advance/" + request.Operation.ID, Evidence: committed.Evidence, Receipt: committed,
	}, nil
}

type runtimeV2FailIfOperationExecuted struct{ calls int }

func (e *runtimeV2FailIfOperationExecuted) ExecuteRuntimeOperation(context.Context, runtimev2.RuntimeOperationExecutionRequest) (runtimev2.RuntimeOperationExecutionResult, error) {
	e.calls++
	return runtimev2.RuntimeOperationExecutionResult{}, errors.New("persisted receipt must commit evidence without re-executing the operation")
}

// inMemoryRuntimeV2StateRepository keeps the candidate/stale mainline test
// independent of the Run owner. Production recovery is covered by the
// file-backed fixtures; this direct repository only proves the controller CAS
// transition owned by Orchestrator.
type inMemoryRuntimeV2StateRepository struct {
	aggregate *runtimev2.RuntimeAggregate
}

func (r *inMemoryRuntimeV2StateRepository) Load(_ context.Context, workspaceID common.WorkspaceID, runID common.RunID) (runtimev2.RuntimeAggregate, error) {
	if r == nil || r.aggregate == nil || r.aggregate.Facts.WorkspaceID != workspaceID || r.aggregate.Facts.RunID != runID {
		return runtimev2.RuntimeAggregate{}, common.ErrNotFound
	}
	// Mirror the Run owner's record conversion: callers must never mutate the
	// persisted aggregate through a shared slice/map backing array before their
	// CAS succeeds. This matters for crash-boundary tests such as request
	// content written before the session-fragment CAS.
	return runtimeAggregateFromRecord(runtimeStateRecordFromAggregate(*r.aggregate)), nil
}

func (r *inMemoryRuntimeV2StateRepository) Save(_ context.Context, aggregate runtimev2.RuntimeAggregate, expectedRevision uint64) error {
	if r == nil {
		return common.ErrInvalidArgument
	}
	if r.aggregate != nil && r.aggregate.StateRevision != expectedRevision {
		return common.ErrConflict
	}
	copy := runtimeAggregateFromRecord(runtimeStateRecordFromAggregate(aggregate))
	r.aggregate = &copy
	return nil
}

type runtimeV2SaveCrashRepository struct {
	delegate runtimev2.RuntimeStateRepository
	match    func(runtimev2.RuntimeAggregate) bool
	after    bool
	fired    bool
	marker   string
}

func (r *runtimeV2SaveCrashRepository) Load(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (runtimev2.RuntimeAggregate, error) {
	return r.delegate.Load(ctx, workspaceID, runID)
}

func (r *runtimeV2SaveCrashRepository) Save(ctx context.Context, aggregate runtimev2.RuntimeAggregate, expectedRevision uint64) error {
	matched := !r.fired && r.match != nil && r.match(aggregate)
	if matched && !r.after {
		r.fired = true
		panic(r.marker)
	}
	if err := r.delegate.Save(ctx, aggregate, expectedRevision); err != nil {
		return err
	}
	if matched && r.after {
		r.fired = true
		panic(r.marker)
	}
	return nil
}

func (r *failOnceCASRuntimeV2Repository) Load(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (runtimev2.RuntimeAggregate, error) {
	return r.delegate.Load(ctx, workspaceID, runID)
}

func (r *failOnceCASRuntimeV2Repository) Save(ctx context.Context, aggregate runtimev2.RuntimeAggregate, expectedRevision uint64) error {
	if expectedRevision > 0 && r.remainingFailures > 0 {
		r.remainingFailures--
		return common.ErrConflict
	}
	return r.delegate.Save(ctx, aggregate, expectedRevision)
}

func (r *failOnceResultRepository) Create(ctx context.Context, item *resultmodel.Result) error {
	if r.remainingFailures > 0 {
		r.remainingFailures--
		return errors.New("injected terminal result delivery failure")
	}
	return r.ResultRepository.Create(ctx, item)
}

func (e panicAfterOwnerReceiptExecutor) ExecuteRuntimeOperation(ctx context.Context, request runtimev2.RuntimeOperationExecutionRequest) (runtimev2.RuntimeOperationExecutionResult, error) {
	_, err := e.delegate.ExecuteRuntimeOperation(ctx, request)
	if err != nil {
		return runtimev2.RuntimeOperationExecutionResult{}, err
	}
	panic("runtime-v2-test-crash-after-owner-receipt")
}

func (e panicAfterOwnerReceiptCommitErrorExecutor) ExecuteRuntimeOperation(ctx context.Context, request runtimev2.RuntimeOperationExecutionRequest) (runtimev2.RuntimeOperationExecutionResult, error) {
	result, err := e.delegate.ExecuteRuntimeOperation(ctx, request)
	if err != nil {
		panic("runtime-v2-test-crash-after-owner-receipt-commit-error")
	}
	return result, nil
}

func TestDriveRuntimeV2FileBackedRunExecutesDurableFileOwnerAndRestarts(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	root := t.TempDir()
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	bootstrap := runtimeV2Bootstrap()
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2",
		DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, State: common.RunStateActive,
	}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	startRuntimeV2ReceiptTestOwner(t, hub)

	deliverable := filepath.Join(root, "workspace")
	internalDir := filepath.Join(root, "internal")
	if err := os.MkdirAll(deliverable, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(internalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deliverable, "target.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{
			ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir,
		}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })

	fileOwner := filemodule.New()
	if err := fileOwner.Configure(filemodule.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := fileOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fileOwner.Stop(ctx) })
	candidateCount := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		candidateCount++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
			ID: "write-1", Kind: runtimecontract.StepKindFileEdit,
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"},
			Parameters: runtimeV2FileEditParameters("target.txt"),
		}}}}}
		result.Set(runtimeV2CandidateResultForTest(t, command, candidate), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })

	module := newRuntimeV2DriveTestModule(t, hub)
	driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, module.handleRuntimeV2Drive)
	command := orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"}
	completed, err := DriveRuntimeV2(ctx, hub, command)
	if err != nil {
		t.Fatal(err)
	}
	content, readErr := os.ReadFile(filepath.Join(deliverable, "target.txt"))
	if readErr != nil || string(content) != "new\n" {
		t.Fatalf("durable file content = %q err=%v", string(content), readErr)
	}
	persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.RuntimeState == nil || persisted.RuntimeState.SchemaVersion != runtimecontract.RuntimeStateSchemaVersionCurrent || len(persisted.RuntimeState.OperationReceipts) != 1 {
		t.Fatalf("file-backed runtime state = %+v err=%v", persisted, err)
	}
	receipt := persisted.RuntimeState.OperationReceipts[0]
	if receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || receipt.ObservedEffectScope != runtimecontract.RuntimeEffectWorkspaceDurable || !receipt.Changed || receipt.BeforeHash == receipt.AfterHash {
		t.Fatalf("durable owner receipt = %+v", receipt)
	}

	driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2)
	restarted := newRuntimeV2DriveTestModule(t, hub)
	driveObserver = magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, restarted.handleRuntimeV2Drive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2) })
	replayed, err := DriveRuntimeV2(ctx, hub, command)
	if err != nil || replayed.StateRevision != completed.StateRevision || candidateCount != 1 {
		t.Fatalf("file-backed restart replay=%+v complete=%+v candidates=%d err=%v", replayed, completed, candidateCount, err)
	}
}

// TestRuntimeV2FileBackedFocusScopedSelectionsStayRetiredAcrossDurableWrites
// is the deployed failure regression: each bootstrap focus receives a File
// owner selection, each durable write changes workspace_tree, and the next
// focus must not be overwritten by a freshness reacquisition for a completed
// focus. The provider observes the selection carried by the sealed current
// authority on every candidate request.
func TestRuntimeV2FileBackedFocusScopedSelectionsStayRetiredAcrossDurableWrites(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	root := t.TempDir()
	deliverable, internalDir := filepath.Join(root, "workspace"), filepath.Join(root, "internal")
	if err := os.MkdirAll(filepath.Join(deliverable, "internal", "lsp"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(internalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	targets := map[string]string{
		"focus-module":  "go.mod",
		"focus-manager": "internal/lsp/manager.go",
		"focus-test":    "internal/lsp/manager_test.go",
	}
	for _, target := range targets {
		if err := os.WriteFile(filepath.Join(deliverable, target), []byte("before\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	bootstrap := runtimeV2FocusScopedSelectionBootstrap()
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
		DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, State: common.RunStateActive,
	}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	startRuntimeV2ReceiptTestOwner(t, hub)

	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{
			ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir,
		}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	fileOwner := filemodule.New()
	if err := fileOwner.Configure(filemodule.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := fileOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fileOwner.Stop(ctx) })

	providerCalls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		providerCalls++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		focusID := command.Decision.Focus.ID
		target, known := targets[focusID]
		if !known {
			t.Errorf("unexpected focus %q", focusID)
			return
		}
		selection := command.CandidateContext.Resources.TargetSelection
		if selection == nil || selection.PrerequisiteID == "" || selection.Disposition != runtimecontract.RuntimeTargetSelectionSelected ||
			len(selection.TargetRefs) != 1 || selection.TargetRefs[0] != target {
			t.Errorf("candidate %s received non-current target selection %+v", focusID, selection)
			return
		}
		coverageID := command.Decision.Focus.CoverageWorkItemIDs[0]
		response := orchestratorevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, LogicalRequestID: command.LogicalRequestID, RequestSequence: command.RequestSequence, PhysicalCallID: command.PhysicalCallID}
		var candidate runtimecontract.RuntimeCandidate
		switch command.Decision.Action {
		case runtimecontract.RuntimeActionProduceStateChange:
			oldText, newText := "before\n", "after-"+focusID+"\n"
			candidate = runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
				ID: "edit-" + focusID, Kind: runtimecontract.StepKindFileEdit,
				CoverageWorkItemIDs: []common.CoverageWorkItemID{coverageID}, TargetRefs: []string{target},
				Parameters: []runtimecontract.OperationParameter{{Name: "oldText", String: &oldText}, {Name: "newText", String: &newText}},
			}}}}}
		case runtimecontract.RuntimeActionProduceValidation:
			candidate = runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: runtimecontract.RuntimeOperation{
				ID: "read-" + focusID, Kind: runtimecontract.StepKindFileRead,
				CoverageWorkItemIDs: []common.CoverageWorkItemID{coverageID}, TargetRefs: []string{target},
				Parameters: nil,
			}}}}}
		case runtimecontract.RuntimeActionProduceAssessment:
			refs := make([]string, 0, 2)
			for _, evidence := range command.CandidateContext.Evidence.Accepted {
				for _, evidenceCoverageID := range evidence.CoverageWorkItemIDs {
					if evidenceCoverageID == coverageID {
						refs = append(refs, string(evidence.ID))
						break
					}
				}
			}
			candidate = runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{
				FocusID: focusID, CoverageWorkItemIDs: []common.CoverageWorkItemID{coverageID},
				Disposition: runtimecontract.AssessmentDispositionSatisfied, Summary: focusID + " evidence satisfied",
				Items: []runtimecontract.RuntimeAssessmentItemFact{{CoverageWorkItemID: coverageID, Satisfied: true, EvidenceRefs: refs}}, EvidenceRefs: refs,
			}}
		default:
			t.Errorf("unexpected action %q for %s", command.Decision.Action, focusID)
			return
		}
		response.DocumentContent = runtimeV2CandidateDocumentForTest(t, command.CandidateManifest, candidate)
		result.Set(response, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })

	module := newRuntimeV2GroundedTargetSequenceModule(t, hub, runtimeV2StateRepository{hub: hub})
	driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, module.handleRuntimeV2Drive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2) })
	completed, err := DriveRuntimeV2(ctx, hub, orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"})
	if err != nil || completed.Disposition != orchestratorevents.RuntimeV2DriveDispositionTerminal || completed.Action != runtimecontract.RuntimeActionComplete {
		t.Fatalf("focus-scoped sequence completion=%+v err=%v", completed, err)
	}
	if providerCalls != 9 {
		t.Fatalf("provider calls=%d, want three candidates for each of three focuses", providerCalls)
	}
	persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.RuntimeState == nil {
		t.Fatalf("persisted focus-scoped aggregate=%+v err=%v", persisted, err)
	}
	state := persisted.RuntimeState
	if state.Facts.Focus.ID != "focus-test" || state.Facts.Resources.TargetSelection == nil ||
		len(state.Facts.Resources.TargetSelection.TargetRefs) != 1 || state.Facts.Resources.TargetSelection.TargetRefs[0] != targets["focus-test"] {
		t.Fatalf("current target authority=%+v", state.Facts)
	}
	retiredFocuses := map[string]bool{}
	activeIDs := map[string]bool{}
	for _, prerequisite := range append(append([]runtimecontract.RuntimePrerequisite(nil), state.Facts.Prerequisites.Pending...), state.Facts.Prerequisites.Satisfied...) {
		activeIDs[prerequisite.ID] = true
	}
	for _, prerequisite := range state.Facts.Prerequisites.Retired {
		if prerequisite.Kind == runtimecontract.PrerequisiteKindTargetSelection {
			retiredFocuses[prerequisite.FocusID] = true
			if activeIDs[prerequisite.ID] {
				t.Fatalf("retired selection re-entered active prerequisite graph: %+v", prerequisite)
			}
		}
	}
	if !retiredFocuses["focus-module"] || !retiredFocuses["focus-manager"] {
		t.Fatalf("completed focus selections were not retained as audit-only history: %+v", state.Facts.Prerequisites.Retired)
	}
	for _, observation := range state.Facts.Resources.RetiredPrerequisiteObservations {
		if observation.Kind == runtimecontract.PrerequisiteKindTargetSelection {
			if activeIDs[observation.PrerequisiteID] {
				t.Fatalf("retired selection observation re-entered active freshness: %+v", observation)
			}
		}
	}
	for focusID, target := range targets {
		content, readErr := os.ReadFile(filepath.Join(deliverable, target))
		if readErr != nil || string(content) != "after-"+focusID+"\n" {
			t.Fatalf("%s durable write content=%q err=%v", focusID, content, readErr)
		}
	}
}

// R3-3 / H5-C: each subtest interrupts the same owner-grounded sequence both
// immediately before and immediately after one Run-owner CAS. A fresh
// Orchestrator Biz then has no choice but to load that file-backed aggregate
// and continue through the regular typed EventHub owners. Target selection is
// intentionally the File owner's bounded complete workspace scope; neither
// the fixture nor the Orchestrator synthesizes an implementation path from a
// path hint.
func TestRuntimeV2FileBackedGroundedTargetStateSequenceRestartsAtCommittedBoundaries(t *testing.T) {
	boundaries := []struct {
		name  string
		match func(runtimev2.RuntimeAggregate) bool
	}{
		{
			name: "target_selection",
			match: func(aggregate runtimev2.RuntimeAggregate) bool {
				return aggregate.Facts.Resources.TargetSelection != nil
			},
		},
		{
			name: "candidate_proposal",
			match: func(aggregate runtimev2.RuntimeAggregate) bool {
				for _, attempt := range aggregate.CandidateAttempts {
					if attempt.Proposal != nil && aggregate.ActiveDispatch == nil {
						return true
					}
				}
				return false
			},
		},
		{
			name: "receipt_and_evidence",
			match: func(aggregate runtimev2.RuntimeAggregate) bool {
				return len(aggregate.OperationReceipts) == 1 && len(aggregate.Facts.Evidence.Accepted) == 1
			},
		},
		{
			name: "assessment_terminal",
			match: func(aggregate runtimev2.RuntimeAggregate) bool {
				return aggregate.Decision.Action == runtimecontract.RuntimeActionComplete && aggregate.Decision.Terminal != nil
			},
		},
	}
	for _, tc := range boundaries {
		for _, after := range []bool{false, true} {
			phase := "before_commit"
			if after {
				phase = "after_commit_response_loss"
			}
			t.Run(tc.name+"_"+phase, func(t *testing.T) {
				ctx := context.Background()
				hub := magicevent.NewHub(32)
				t.Cleanup(func() { hub.Terminate(ctx) })
				candidateRequestContents := memory.NewArtifactContinuationRepository()
				root := t.TempDir()
				deliverable, internalDir := filepath.Join(root, "workspace"), filepath.Join(root, "internal")
				if err := os.MkdirAll(deliverable, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(internalDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(deliverable, "implementation.txt"), []byte("before\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(deliverable, "support.txt"), []byte("support\n"), 0o644); err != nil {
					t.Fatal(err)
				}

				bootstrap := runtimeV2GroundedTargetSequenceBootstrap()
				bootstrap.Focuses[0].TargetSelectorRef = "file/workspace_deliverable"
				runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
				workspaces := filerepository.NewWorkspaceRepository(filepath.Join(root, "workspaces.json"))
				results := filerepository.NewResultRepository(filepath.Join(root, "results.json"))
				if err := runs.Create(ctx, &runmodel.Run{
					ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
					DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, State: common.RunStateActive,
				}); err != nil {
					t.Fatal(err)
				}
				if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-v2", State: common.WorkspaceStateActive, ActiveRunID: "run-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}); err != nil {
					t.Fatal(err)
				}
				runOwner := runmodule.New()
				if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
					t.Fatal(err)
				}
				if err := runOwner.Start(); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = runOwner.Stop(ctx) })
				workspaceOwner := workspacemodule.New()
				if err := workspaceOwner.Configure(workspacemodule.Dependencies{
					Workspaces: workspaces, Goals: memory.NewGoalRepository(), DoDs: memory.NewDoDRepository(),
					Templates: memory.NewWorkspaceTemplateRepository(), EventHub: hub,
				}); err != nil {
					t.Fatal(err)
				}
				if err := workspaceOwner.Start(); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = workspaceOwner.Stop(ctx) })
				resultOwner := resultmodule.New()
				if err := resultOwner.Configure(resultmodule.Dependencies{Results: results, Artifacts: memory.NewArtifactRepository(), EventHub: hub}); err != nil {
					t.Fatal(err)
				}
				if err := resultOwner.Start(); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = resultOwner.Stop(ctx) })
				fileOwner := filemodule.New()
				if err := fileOwner.Configure(filemodule.Dependencies{EventHub: hub}); err != nil {
					t.Fatal(err)
				}
				if err := fileOwner.Start(); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = fileOwner.Stop(ctx) })
				startRuntimeV2ReceiptTestOwner(t, hub)

				providerCalls := 0
				llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
				llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
					providerCalls++
					command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
					candidate, err := runtimeV2GroundedTargetSequenceCandidate(command)
					if err != nil {
						t.Errorf("grounded sequence candidate: %v", err)
						result.Set(orchestratorevents.RequestRuntimeCandidateResult{
							Transaction: command.Transaction, LogicalRequestID: command.LogicalRequestID, RequestSequence: command.RequestSequence, PhysicalCallID: command.PhysicalCallID,
							Failure: &runtimecontract.RuntimeCandidateFailure{Class: runtimecontract.RuntimeCandidateFailureClassInternal, Reason: err.Error()},
						}, nil)
						return
					}
					result.Set(runtimeV2CandidateResultForTest(t, command, candidate), nil)
				})
				t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })

				crashMarker := "grounded-sequence-" + tc.name + "-" + phase
				crashRepository := &runtimeV2SaveCrashRepository{
					delegate: runtimeV2StateRepository{hub: hub}, match: tc.match, after: after, marker: crashMarker,
				}
				crashingModule := newRuntimeV2GroundedTargetSequenceModule(t, hub, crashRepository, candidateRequestContents)
				func() {
					defer func() {
						if recovered := recover(); recovered != crashMarker {
							t.Fatalf("unexpected crash marker: %v", recovered)
						}
					}()
					if err := crashingModule.scheduleRuntimeV2Drive(ctx, "ws-v2", "run-v2", crashMarker); err != nil {
						t.Fatalf("drive before crash: %v", err)
					}
					t.Fatalf("expected crash %s %s CAS", phase, tc.name)
				}()
				if !crashRepository.fired {
					t.Fatalf("crash boundary %s was not reached", tc.name)
				}
				crashed, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
				if err != nil || crashed.RuntimeState == nil {
					t.Fatalf("crashed aggregate missing from Run owner: run=%+v err=%v", crashed, err)
				}
				crashedAggregate := runtimeAggregateFromRecord(*crashed.RuntimeState)
				if after && !tc.match(crashedAggregate) {
					t.Fatalf("committed %s boundary is absent after response loss: %+v", tc.name, crashedAggregate)
				}
				if !after && tc.match(crashedAggregate) {
					t.Fatalf("uncommitted %s boundary leaked into restarted aggregate: %+v", tc.name, crashedAggregate)
				}

				restarted := newRuntimeV2GroundedTargetSequenceModule(t, hub, runtimeV2StateRepository{hub: hub}, candidateRequestContents)
				if err := restarted.scheduleRuntimeV2Drive(ctx, "ws-v2", "run-v2", "restart:"+tc.name+":"+phase); err != nil {
					t.Fatalf("restart drive: %v", err)
				}
				persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
				if err != nil || persisted.RuntimeState == nil || persisted.State != common.RunStateCompleted {
					t.Fatalf("restart terminal run=%+v state=%+v err=%v", persisted, persisted.RuntimeState, err)
				}
				state := persisted.RuntimeState
				if state.Decision.Action != runtimecontract.RuntimeActionComplete || state.Decision.Terminal == nil ||
					state.Facts.Resources.TargetSelection == nil || state.Facts.Resources.TargetSelection.Disposition != runtimecontract.RuntimeTargetSelectionScope ||
					len(state.Facts.Resources.TargetSnapshots) != 0 ||
					len(state.OperationReceipts) != 2 || len(state.Facts.Evidence.Accepted) != 2 || len(state.Facts.Assessments) != 1 {
					t.Fatalf("restart state sequence did not converge exactly once: %+v", state)
				}
				// C4 persists the complete provider document before C5 allocates or
				// proposes an attempt. A proposal-CAS crash therefore reloads the
				// same document_complete session and must not ask the provider to
				// regenerate the candidate. Other boundaries have the same or a
				// stronger persisted fact to consume.
				wantProviderCalls := 3
				if providerCalls != wantProviderCalls {
					t.Fatalf("provider calls=%d, want %d", providerCalls, wantProviderCalls)
				}
				if content, err := os.ReadFile(filepath.Join(deliverable, "implementation.txt")); err != nil || string(content) != "after\n" {
					t.Fatalf("target durable effect=%q err=%v", content, err)
				}
				items, err := results.ListByRun(ctx, "run-v2")
				if err != nil || len(items) != 1 || items[0].State != common.ResultStateAccepted {
					t.Fatalf("terminal projection Result=%+v err=%v", items, err)
				}
				workspace, err := workspaces.Get(ctx, "ws-v2")
				if err != nil || workspace.ActiveRunID != "" {
					t.Fatalf("terminal projection Workspace=%+v err=%v", workspace, err)
				}
			})
		}
	}
}

// TestRuntimeV2FileBackedAmbiguousTargetSelectionFailsWithoutProvider keeps
// target grounding on the production typed-owner path. A profile may declare
// File's neutral unique-entry selector, but a multi-file workspace is an
// explicit ambiguous observation, not permission for the Orchestrator, LLM,
// or validation entry to choose an implementation target.
func TestRuntimeV2FileBackedAmbiguousTargetSelectionFailsWithoutProvider(t *testing.T) {
	t.Run("state_change", func(t *testing.T) {
		testRuntimeV2FileBackedUnresolvedTargetSelectionFailsWithoutProvider(t, runtimeV2GroundedTargetSequenceBootstrap(), runtimecontract.RuntimeTargetSelectionAmbiguous, map[string]string{"validation-entry.go": "package fixture\n", "implementation.go": "package fixture\n"})
	})
	t.Run("direct_validation", func(t *testing.T) {
		testRuntimeV2FileBackedUnresolvedTargetSelectionFailsWithoutProvider(t, runtimeV2GroundedTargetValidationBootstrap(), runtimecontract.RuntimeTargetSelectionAmbiguous, map[string]string{"validation-entry.go": "package fixture\n", "implementation.go": "package fixture\n"})
	})
}

// The legacy unique-entry selector reports an empty directory as absent. The
// production workspace-deliverable selector has a separate owner-grounded
// creation path below; neither path requests TUI/Web target input.
func TestRuntimeV2FileBackedAbsentTargetSelectionFailsWithoutProvider(t *testing.T) {
	testRuntimeV2FileBackedUnresolvedTargetSelectionFailsWithoutProvider(t, runtimeV2GroundedTargetSequenceBootstrap(), runtimecontract.RuntimeTargetSelectionAbsent, nil)
}

func TestRuntimeV2FileBackedWorkspaceDeliverableWithoutProfileArtifactFailsWithoutProvider(t *testing.T) {
	bootstrap := runtimeV2GroundedTargetSequenceBootstrap()
	bootstrap.Focuses[0].TargetSelectorRef = "file/workspace_deliverable"
	bootstrap.Focuses[0].ProspectiveTargetRef = ""
	testRuntimeV2FileBackedUnresolvedTargetSelectionFailsWithoutProvider(t, bootstrap, runtimecontract.RuntimeTargetSelectionAbsent, nil)
}

func TestRuntimeV2FileBackedEmptyWorkspaceCreationSelectionContinuesThroughCandidate(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	root := t.TempDir()
	deliverable, internalDir := filepath.Join(root, "workspace"), filepath.Join(root, "internal")
	if err := os.MkdirAll(deliverable, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(internalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	bootstrap := runtimeV2GroundedTargetSequenceBootstrap()
	bootstrap.Definition.CapabilityContract = []string{runtimecontract.StepKindFileWrite, runtimecontract.StepKindFileRead}
	bootstrap.Focuses[0].TargetSelectorRef = "file/workspace_deliverable"
	bootstrap.Focuses[0].ProspectiveTargetRef = "implementation.txt"
	bootstrap.Focuses[0].AcceptedStepKinds = []string{runtimecontract.StepKindFileWrite, runtimecontract.StepKindFileRead}
	bootstrap.Requirements[0].RequiredCapabilities = []string{runtimecontract.StepKindFileWrite}
	bootstrap.Requirements = append(bootstrap.Requirements, runtimecontract.RuntimeRequirement{
		Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		RequiredCapabilities: []string{runtimecontract.StepKindFileWrite, runtimecontract.StepKindFileRead}, Reason: "empty workspace File creation inventory",
	})
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID, DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, State: common.RunStateActive}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	startRuntimeV2ReceiptTestOwner(t, hub)
	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	fileOwner := filemodule.New()
	if err := fileOwner.Configure(filemodule.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := fileOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fileOwner.Stop(ctx) })
	candidateCalls := 0
	initialSelection := runtimecontract.RuntimeTargetSelectionDisposition("")
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		candidateCalls++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		if candidateCalls == 1 && command.CandidateContext.Resources.TargetSelection != nil {
			initialSelection = command.CandidateContext.Resources.TargetSelection.Disposition
		}
		result.Set(runtimeV2CandidateResultForTest(t, command, runtimeV2EmptyWorkspaceCreationCandidate(command)), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	module := newRuntimeV2GroundedTargetSequenceModule(t, hub, runtimeV2StateRepository{hub: hub})
	driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, module.handleRuntimeV2Drive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2) })

	completed, err := DriveRuntimeV2(ctx, hub, orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"})
	if err != nil || completed.Disposition != orchestratorevents.RuntimeV2DriveDispositionTerminal || completed.Action != runtimecontract.RuntimeActionComplete || candidateCalls != 3 {
		t.Fatalf("empty workspace creation completion=%+v candidateCalls=%d err=%v", completed, candidateCalls, err)
	}
	content, err := os.ReadFile(filepath.Join(deliverable, "implementation.txt"))
	if err != nil || string(content) != "created\n" {
		t.Fatalf("empty workspace creation file=%q err=%v", string(content), err)
	}
	persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.RuntimeState == nil || initialSelection != runtimecontract.RuntimeTargetSelectionCreate ||
		persisted.RuntimeState.Facts.Resources.TargetSelection == nil || persisted.RuntimeState.Facts.Resources.TargetSelection.Disposition != runtimecontract.RuntimeTargetSelectionSelected ||
		len(persisted.RuntimeState.Facts.Resources.TargetSnapshots) != 1 || len(persisted.RuntimeState.OperationReceipts) != 2 {
		t.Fatalf("empty workspace creation authority=%+v err=%v", persisted, err)
	}
}

func TestRuntimeV2FileBackedEmptyWorkspaceCreationReceiptCrashRestartDoesNotReplayCreate(t *testing.T) {
	for _, after := range []bool{false, true} {
		phase := "before_receipt_evidence_commit"
		if after {
			phase = "after_receipt_evidence_commit_response_loss"
		}
		t.Run(phase, func(t *testing.T) {
			ctx := context.Background()
			hub := magicevent.NewHub(32)
			t.Cleanup(func() { hub.Terminate(ctx) })
			root := t.TempDir()
			deliverable, internalDir := filepath.Join(root, "workspace"), filepath.Join(root, "internal")
			if err := os.MkdirAll(deliverable, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(internalDir, 0o755); err != nil {
				t.Fatal(err)
			}
			bootstrap := runtimeV2GroundedTargetSequenceBootstrap()
			bootstrap.Definition.CapabilityContract = []string{runtimecontract.StepKindFileWrite, runtimecontract.StepKindFileRead}
			bootstrap.Focuses[0].TargetSelectorRef = "file/workspace_deliverable"
			bootstrap.Focuses[0].ProspectiveTargetRef = "implementation.txt"
			bootstrap.Focuses[0].AcceptedStepKinds = []string{runtimecontract.StepKindFileWrite, runtimecontract.StepKindFileRead}
			bootstrap.Requirements[0].RequiredCapabilities = []string{runtimecontract.StepKindFileWrite}
			bootstrap.Requirements = append(bootstrap.Requirements, runtimecontract.RuntimeRequirement{
				Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleFile,
				RequiredCapabilities: []string{runtimecontract.StepKindFileWrite, runtimecontract.StepKindFileRead}, Reason: "empty workspace File creation inventory",
			})
			runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
			workspaces := filerepository.NewWorkspaceRepository(filepath.Join(root, "workspaces.json"))
			results := filerepository.NewResultRepository(filepath.Join(root, "results.json"))
			if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID, DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, State: common.RunStateActive}); err != nil {
				t.Fatal(err)
			}
			if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-v2", State: common.WorkspaceStateActive, ActiveRunID: "run-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}); err != nil {
				t.Fatal(err)
			}
			runOwner := runmodule.New()
			if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
				t.Fatal(err)
			}
			if err := runOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = runOwner.Stop(ctx) })
			workspaceOwner := workspacemodule.New()
			if err := workspaceOwner.Configure(workspacemodule.Dependencies{
				Workspaces: workspaces, Goals: memory.NewGoalRepository(), DoDs: memory.NewDoDRepository(),
				Templates: memory.NewWorkspaceTemplateRepository(), EventHub: hub,
			}); err != nil {
				t.Fatal(err)
			}
			if err := workspaceOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = workspaceOwner.Stop(ctx) })
			resultOwner := resultmodule.New()
			if err := resultOwner.Configure(resultmodule.Dependencies{Results: results, Artifacts: memory.NewArtifactRepository(), EventHub: hub}); err != nil {
				t.Fatal(err)
			}
			if err := resultOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = resultOwner.Stop(ctx) })
			startRuntimeV2ReceiptTestOwner(t, hub)
			fileOwner := filemodule.New()
			if err := fileOwner.Configure(filemodule.Dependencies{EventHub: hub}); err != nil {
				t.Fatal(err)
			}
			if err := fileOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = fileOwner.Stop(ctx) })
			candidateCalls := 0
			llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
			llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
				candidateCalls++
				command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
				result.Set(runtimeV2CandidateResultForTest(t, command, runtimeV2EmptyWorkspaceCreationCandidate(command)), nil)
			})
			t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })

			marker := "empty-workspace-create-receipt-" + phase
			crashRepository := &runtimeV2SaveCrashRepository{
				delegate: runtimeV2StateRepository{hub: hub}, after: after, marker: marker,
				match: func(aggregate runtimev2.RuntimeAggregate) bool {
					return len(aggregate.OperationReceipts) == 1 && len(aggregate.Facts.Evidence.Accepted) == 1
				},
			}
			crashing := newRuntimeV2GroundedTargetSequenceModule(t, hub, crashRepository)
			func() {
				defer func() {
					if recovered := recover(); recovered != marker {
						t.Fatalf("unexpected empty-workspace receipt crash marker: %v", recovered)
					}
				}()
				if err := crashing.scheduleRuntimeV2Drive(ctx, "ws-v2", "run-v2", marker); err != nil {
					t.Fatalf("drive before empty-workspace receipt crash: %v", err)
				}
				t.Fatal("expected empty-workspace receipt crash")
			}()
			if !crashRepository.fired {
				t.Fatal("empty-workspace receipt crash point was not reached")
			}
			created, err := os.ReadFile(filepath.Join(deliverable, "implementation.txt"))
			if err != nil || string(created) != "created\n" {
				t.Fatalf("empty-workspace first create content=%q err=%v", string(created), err)
			}

			restarted := newRuntimeV2GroundedTargetSequenceModule(t, hub, runtimeV2StateRepository{hub: hub})
			if err := restarted.scheduleRuntimeV2Drive(ctx, "ws-v2", "run-v2", "restart:"+phase); err != nil {
				t.Fatalf("restart after empty-workspace receipt crash: %v", err)
			}
			persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil || persisted.RuntimeState == nil || persisted.State != common.RunStateCompleted ||
				persisted.RuntimeState.Decision.Action != runtimecontract.RuntimeActionComplete ||
				persisted.RuntimeState.Facts.Resources.TargetSelection == nil || persisted.RuntimeState.Facts.Resources.TargetSelection.Disposition != runtimecontract.RuntimeTargetSelectionSelected ||
				len(persisted.RuntimeState.Facts.Resources.TargetSnapshots) != 1 || len(persisted.RuntimeState.OperationReceipts) != 2 ||
				len(persisted.RuntimeState.Facts.Evidence.Accepted) != 2 || candidateCalls != 3 {
				t.Fatalf("empty-workspace receipt restart authority=%+v candidateCalls=%d err=%v", persisted, candidateCalls, err)
			}
		})
	}
}

// TestRuntimeV2FileBackedBoundedWorkspaceScopeAllowsNewDeliverable verifies
// the owner-chain counterpart to empty-workspace creation. A complete bounded
// scope is authority for File to assess a concrete new workspace-relative
// target, not an Orchestrator permission to synthesize one. The durable
// receipt must then invalidate the old scope and require a fresh complete
// scope before validation and assessment can converge. Scope contents remain
// File feasibility inventory; they must never be read as concrete snapshots.
func TestRuntimeV2FileBackedBoundedWorkspaceScopeAllowsNewDeliverable(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	root := t.TempDir()
	deliverable, internalDir := filepath.Join(root, "workspace"), filepath.Join(root, "internal")
	if err := os.MkdirAll(deliverable, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(internalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Keep the candidate-context budget at four items while the File owner
	// observes five workspace entries.  Target selection is an owner-grounded
	// feasibility fact, so it must retain the complete bounded scope rather
	// than be downgraded to ambiguous by the candidate context budget.
	for target, content := range map[string]string{
		"implementation.txt": "before\n", "support.txt": "support\n", "go.mod": "module fixture\n",
		"internal/bridge.go": "package fixture\n", "internal/bridge_test.go": "package fixture\n",
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(deliverable, target)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(deliverable, target), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	bootstrap := runtimeV2GroundedTargetSequenceBootstrap()
	bootstrap.ConvergencePolicy.MaxCandidateContextItems = 4
	bootstrap.Definition.CapabilityContract = []string{runtimecontract.StepKindFileWrite, runtimecontract.StepKindFileRead}
	bootstrap.Focuses[0].TargetSelectorRef = "file/workspace_deliverable"
	bootstrap.Focuses[0].AcceptedStepKinds = []string{runtimecontract.StepKindFileWrite, runtimecontract.StepKindFileRead}
	bootstrap.Requirements[0].RequiredCapabilities = []string{runtimecontract.StepKindFileWrite}
	bootstrap.Requirements = append(bootstrap.Requirements, runtimecontract.RuntimeRequirement{
		Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		RequiredCapabilities: []string{runtimecontract.StepKindFileWrite, runtimecontract.StepKindFileRead}, Reason: "bounded workspace File creation inventory",
	})
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID, DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, State: common.RunStateActive}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	startRuntimeV2ReceiptTestOwner(t, hub)
	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	fileOwner := filemodule.New()
	if err := fileOwner.Configure(filemodule.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := fileOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fileOwner.Stop(ctx) })
	candidateCalls := 0
	stateChangeCalls := 0
	initialSelection := runtimecontract.RuntimeTargetSelectionDisposition("")
	initialSelectionCount := 0
	initialSnapshots := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		candidateCalls++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		if candidateCalls == 1 && command.CandidateContext.Resources.TargetSelection != nil {
			initialSelection = command.CandidateContext.Resources.TargetSelection.Disposition
			initialSelectionCount = command.CandidateContext.Resources.TargetSelection.CandidateCount
			initialSnapshots = len(command.CandidateContext.Resources.TargetSnapshots)
		}
		if command.Decision.Action == runtimecontract.RuntimeActionProduceStateChange {
			stateChangeCalls++
			if stateChangeCalls == 1 {
				// A valid File write to unchanged content produces an owner no-op
				// receipt.  Scope must remain feasibility inventory through this
				// repair, not become the concrete directive target that blocks the
				// next File proposal below.
				result.Set(runtimeV2CandidateResultForTest(t, command, runtimeV2WorkspaceCreationCandidate(command, "implementation.txt", "before\n")), nil)
				return
			}
		}
		result.Set(runtimeV2CandidateResultForTest(t, command, runtimeV2WorkspaceCreationCandidate(command, "new-deliverable.txt", "created\n")), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	module := newRuntimeV2GroundedTargetSequenceModule(t, hub, runtimeV2StateRepository{hub: hub})
	driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, module.handleRuntimeV2Drive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2) })

	completed, err := DriveRuntimeV2(ctx, hub, orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"})
	if err != nil || completed.Disposition != orchestratorevents.RuntimeV2DriveDispositionTerminal || completed.Action != runtimecontract.RuntimeActionComplete || candidateCalls != 4 || stateChangeCalls != 2 {
		t.Fatalf("bounded scope new-deliverable completion=%+v candidateCalls=%d stateChangeCalls=%d err=%v", completed, candidateCalls, stateChangeCalls, err)
	}
	content, err := os.ReadFile(filepath.Join(deliverable, "new-deliverable.txt"))
	if err != nil || string(content) != "created\n" {
		t.Fatalf("bounded scope new-deliverable content=%q err=%v", string(content), err)
	}
	persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.RuntimeState == nil || initialSelection != runtimecontract.RuntimeTargetSelectionScope || initialSelectionCount != 5 || initialSnapshots != 0 ||
		persisted.RuntimeState.Facts.Resources.TargetSelection == nil || persisted.RuntimeState.Facts.Resources.TargetSelection.Disposition != runtimecontract.RuntimeTargetSelectionScope ||
		persisted.RuntimeState.Facts.Resources.TargetSelection.CandidateCount != 6 || len(persisted.RuntimeState.Facts.Resources.TargetSnapshots) != 0 ||
		len(persisted.RuntimeState.OperationReceipts) != 3 || len(persisted.RuntimeState.Facts.Evidence.Accepted) != 2 || len(persisted.RuntimeState.Facts.Evidence.Rejected) != 1 ||
		persisted.RuntimeState.Facts.Resources.FailureTarget != nil || (persisted.RuntimeState.Facts.RepairDirective != nil && len(persisted.RuntimeState.Facts.RepairDirective.TargetRefs) != 0) {
		t.Fatalf("bounded scope new-deliverable authority=%+v err=%v", persisted, err)
	}
}

// TestRuntimeV2FileBackedWorkspaceScopeExecutesTargetBoundShellOperation
// proves the P6 scope exception through the actual File and Shell owners. The
// shell command may modify go.sum because File observed it in the sealed scope;
// it does not receive authority to create a new path or to operate outside that
// inventory. P5's Shell receipt crash/restart matrix remains the replay proof
// for the resulting sealed operation.
func TestRuntimeV2FileBackedWorkspaceScopeExecutesTargetBoundShellOperation(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	root := t.TempDir()
	deliverable, internalDir := filepath.Join(root, "workspace"), filepath.Join(root, "internal")
	if err := os.MkdirAll(deliverable, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(internalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for target, content := range map[string]string{"go.mod": "module fixture\n", "go.sum": "before\n"} {
		if err := os.WriteFile(filepath.Join(deliverable, target), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	bootstrap := runtimeV2GroundedTargetSequenceBootstrap()
	bootstrap.Definition.CapabilityContract = []string{runtimecontract.StepKindShellCommand, runtimecontract.StepKindFileRead}
	bootstrap.Focuses[0].TargetSelectorRef = "file/workspace_deliverable"
	bootstrap.Focuses[0].AcceptedStepKinds = []string{runtimecontract.StepKindShellCommand, runtimecontract.StepKindFileRead}
	bootstrap.Requirements = []runtimecontract.RuntimeRequirement{
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleShell, CapabilityClass: runtimecontract.RuntimeCapabilityClassStateChange, RequiredCapabilities: []string{runtimecontract.StepKindShellCommand}, Reason: "Shell state-change capability"},
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleFile, CapabilityClass: runtimecontract.RuntimeCapabilityClassValidation, RequiredCapabilities: []string{runtimecontract.StepKindFileRead}, Reason: "File validation capability"},
		{Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleShell, RequiredCapabilities: []string{runtimecontract.StepKindShellCommand}, Reason: "Shell owner inventory"},
		{Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleFile, RequiredCapabilities: []string{runtimecontract.StepKindFileRead}, Reason: "File validation inventory"},
	}
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID, DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, State: common.RunStateActive}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	startRuntimeV2ReceiptTestOwner(t, hub)
	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	fileOwner := filemodule.New()
	if err := fileOwner.Configure(filemodule.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := fileOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fileOwner.Stop(ctx) })
	shellOwner := shellmodule.New()
	if err := shellOwner.Configure(shellmodule.Dependencies{EventHub: hub, RuntimeAdapter: shellbiz.NewRuntimeAdapter()}); err != nil {
		t.Fatal(err)
	}
	if err := shellOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = shellOwner.Stop(ctx) })
	candidateCalls := 0
	candidateActions := make([]runtimecontract.RuntimeAction, 0, 4)
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		candidateCalls++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		candidateActions = append(candidateActions, command.Decision.Action)
		switch command.Decision.Action {
		case runtimecontract.RuntimeActionProduceStateChange:
			commandName, target, script := "sh", "go.sum", "printf 'updated\\n' > go.sum"
			stateChanging := true
			candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
				ID: "tidy-go-sum", Kind: runtimecontract.StepKindShellCommand, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{target},
				Parameters: []runtimecontract.OperationParameter{{Name: "command", String: &commandName}, {Name: "args", StringList: []string{"-c", script}}, {Name: "stateChanging", Boolean: &stateChanging}},
			}}}}}
			result.Set(runtimeV2CandidateResultForTest(t, command, candidate), nil)
		case runtimecontract.RuntimeActionProduceValidation:
			target := "go.sum"
			candidate := runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: runtimecontract.RuntimeOperation{
				ID: "read-go-sum", Kind: runtimecontract.StepKindFileRead, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{target},
			}}}}}
			result.Set(runtimeV2CandidateResultForTest(t, command, candidate), nil)
		case runtimecontract.RuntimeActionProduceAssessment:
			refs := make([]string, 0, len(command.CandidateContext.Evidence.Accepted))
			for _, evidence := range command.CandidateContext.Evidence.Accepted {
				refs = append(refs, string(evidence.ID))
			}
			candidate := runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{FocusID: "focus-v2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, Disposition: runtimecontract.AssessmentDispositionSatisfied, Summary: "target-bound shell mutation validated", Items: []runtimecontract.RuntimeAssessmentItemFact{{CoverageWorkItemID: "cwi-v2", Satisfied: true, EvidenceRefs: refs}}, EvidenceRefs: refs}}
			result.Set(runtimeV2CandidateResultForTest(t, command, candidate), nil)
		default:
			t.Fatalf("unexpected action %q", command.Decision.Action)
		}
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	module := newRuntimeV2GroundedTargetSequenceModule(t, hub, runtimeV2StateRepository{hub: hub})
	driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, module.handleRuntimeV2Drive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2) })

	completed, err := DriveRuntimeV2(ctx, hub, orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"})
	// The Shell receipt changes a File-observed target. SM-2 therefore retires
	// the dependent File validation inventory before assessment; it must not
	// replay the Shell operation. One pre-refresh validation request may be
	// invalidated without an owner receipt, so the proof counts receipts rather
	// than treating request count as a side-effect count.
	if err != nil || completed.Disposition != orchestratorevents.RuntimeV2DriveDispositionTerminal || completed.Action != runtimecontract.RuntimeActionComplete || candidateCalls != 4 {
		t.Fatalf("target-bound Shell completion=%+v candidateCalls=%d actions=%v err=%v", completed, candidateCalls, candidateActions, err)
	}
	content, err := os.ReadFile(filepath.Join(deliverable, "go.sum"))
	if err != nil || string(content) != "updated\n" {
		t.Fatalf("target-bound Shell content=%q err=%v", string(content), err)
	}
	persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.RuntimeState == nil || persisted.RuntimeState.Decision.Action != runtimecontract.RuntimeActionComplete ||
		persisted.RuntimeState.Facts.Resources.TargetSelection == nil || persisted.RuntimeState.Facts.Resources.TargetSelection.Disposition != runtimecontract.RuntimeTargetSelectionScope ||
		len(persisted.RuntimeState.OperationReceipts) != 2 || len(persisted.RuntimeState.Facts.Evidence.Accepted) != 2 {
		state := persisted.RuntimeState
		if state == nil {
			t.Fatalf("target-bound Shell runtime state=%+v err=%v", persisted, err)
		}
		t.Fatalf("target-bound Shell state=%s selection=%+v receipts=%d accepted=%d err=%v", state.Decision.Action, state.Facts.Resources.TargetSelection, len(state.OperationReceipts), len(state.Facts.Evidence.Accepted), err)
	}
	stateChangeCalls := 0
	for _, action := range candidateActions {
		if action == runtimecontract.RuntimeActionProduceStateChange {
			stateChangeCalls++
		}
	}
	if stateChangeCalls != 1 {
		t.Fatalf("target-bound Shell mutation was replayed: candidate actions=%v", candidateActions)
	}
}

func runtimeV2EmptyWorkspaceCreationCandidate(command orchestratorevents.RequestRuntimeCandidateCommand) runtimecontract.RuntimeCandidate {
	target := "implementation.txt"
	if selection := command.CandidateContext.Resources.TargetSelection; selection != nil && selection.Disposition == runtimecontract.RuntimeTargetSelectionCreate && selection.ProspectiveTargetRef != "" {
		target = selection.ProspectiveTargetRef
	}
	return runtimeV2WorkspaceCreationCandidate(command, target, "created\n")
}

func runtimeV2WorkspaceCreationCandidate(command orchestratorevents.RequestRuntimeCandidateCommand, target, content string) runtimecontract.RuntimeCandidate {
	switch command.Decision.Action {
	case runtimecontract.RuntimeActionProduceStateChange:
		return runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
			ID: "create-deliverable", Kind: runtimecontract.StepKindFileWrite, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{target},
			Parameters: []runtimecontract.OperationParameter{{Name: "content", String: &content}},
		}}}}}
	case runtimecontract.RuntimeActionProduceValidation:
		return runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: runtimecontract.RuntimeOperation{
			ID: "read-created-deliverable", Kind: runtimecontract.StepKindFileRead, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{target},
		}}}}}
	case runtimecontract.RuntimeActionProduceAssessment:
		refs := make([]string, 0, len(command.CandidateContext.Evidence.Accepted))
		for _, evidence := range command.CandidateContext.Evidence.Accepted {
			refs = append(refs, string(evidence.ID))
		}
		return runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{FocusID: "focus-v2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, Disposition: runtimecontract.AssessmentDispositionSatisfied, Summary: "created workspace deliverable validated", Items: []runtimecontract.RuntimeAssessmentItemFact{{CoverageWorkItemID: "cwi-v2", Satisfied: true, EvidenceRefs: refs}}, EvidenceRefs: refs}}
	default:
		return runtimecontract.RuntimeCandidate{}
	}
}

func testRuntimeV2FileBackedUnresolvedTargetSelectionFailsWithoutProvider(t *testing.T, bootstrap runtimecontract.RuntimeBootstrap, disposition runtimecontract.RuntimeTargetSelectionDisposition, files map[string]string) {
	t.Helper()
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	root := t.TempDir()
	deliverable, internalDir := filepath.Join(root, "workspace"), filepath.Join(root, "internal")
	if err := os.MkdirAll(deliverable, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(internalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(deliverable, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
		DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, State: common.RunStateActive,
	}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	fileOwner := filemodule.New()
	if err := fileOwner.Configure(filemodule.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := fileOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fileOwner.Stop(ctx) })

	providerCalls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		providerCalls++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, LogicalRequestID: command.LogicalRequestID, RequestSequence: command.RequestSequence, PhysicalCallID: command.PhysicalCallID, Failure: &runtimecontract.RuntimeCandidateFailure{
			Class: runtimecontract.RuntimeCandidateFailureClassInternal, Reason: "provider must not be called for unresolved target selection",
		}}, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	governanceRequests := 0
	governanceObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleDecision, hub)
	governanceObserver.Subscribe(orchestratorevents.TopicOrchestratorOpenRuntimeGovernanceRequest, func(_ magicevent.Event, result magicevent.Result) {
		governanceRequests++
		result.Set(nil, nil)
	})
	t.Cleanup(func() {
		governanceObserver.Unsubscribe(orchestratorevents.TopicOrchestratorOpenRuntimeGovernanceRequest)
	})
	module := newRuntimeV2GroundedTargetSequenceModule(t, hub, runtimeV2StateRepository{hub: hub})
	driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, module.handleRuntimeV2Drive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2) })

	completed, err := DriveRuntimeV2(ctx, hub, orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"})
	if err != nil || completed.Disposition != orchestratorevents.RuntimeV2DriveDispositionTerminal || completed.Action != runtimecontract.RuntimeActionFail || completed.Terminal == nil || completed.Terminal.Code != "target_grounding_unresolved" {
		t.Fatalf("unresolved owner target result=%+v err=%v", completed, err)
	}
	if providerCalls != 0 {
		t.Fatalf("unresolved target selection called provider %d times", providerCalls)
	}
	if governanceRequests != 0 {
		t.Fatalf("unresolved target selection opened %d governance requests", governanceRequests)
	}
	persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.RuntimeState == nil || persisted.RuntimeState.ActiveDispatch != nil || persisted.RuntimeState.Decision.Action != runtimecontract.RuntimeActionFail || persisted.RuntimeState.Decision.Terminal == nil ||
		persisted.RuntimeState.Facts.Resources.TargetSelection == nil || persisted.RuntimeState.Facts.Resources.TargetSelection.Disposition != disposition ||
		persisted.RuntimeState.Facts.Resources.TargetSelection.CandidateCount != len(files) || len(persisted.RuntimeState.Facts.Resources.TargetSnapshots) != 0 ||
		len(persisted.RuntimeState.OperationReceipts) != 0 || len(persisted.RuntimeState.Facts.Evidence.Accepted) != 0 {
		t.Fatalf("unresolved target selection persisted wrong authority state=%+v err=%v", persisted, err)
	}
}

func runtimeV2GroundedTargetSequenceBootstrap() runtimecontract.RuntimeBootstrap {
	bootstrap := runtimeV2Bootstrap()
	bootstrap.Definition.CapabilityContract = []string{runtimecontract.StepKindFileEdit, runtimecontract.StepKindFileRead}
	bootstrap.Focuses = []runtimecontract.CoverageFocus{{
		ID: "focus-v2", SliceID: "slice-v2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation},
		TargetRole:            "deliverable", TargetDiscoverySources: []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest},
		TargetSelectorRef: "file/unique_workspace_entry", RequireTargetGrounding: true, RequireAssessment: true,
		AcceptedStepKinds: []string{runtimecontract.StepKindFileEdit, runtimecontract.StepKindFileRead},
	}}
	bootstrap.Slices[0].RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}
	bootstrap.Requirements = []runtimecontract.RuntimeRequirement{
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleFile, CapabilityClass: runtimecontract.RuntimeCapabilityClassStateChange, RequiredCapabilities: []string{runtimecontract.StepKindFileEdit}},
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleFile, CapabilityClass: runtimecontract.RuntimeCapabilityClassValidation, RequiredCapabilities: []string{runtimecontract.StepKindFileRead}},
	}
	return bootstrap
}

func runtimeV2FocusScopedSelectionBootstrap() runtimecontract.RuntimeBootstrap {
	bootstrap := runtimeV2Bootstrap()
	bootstrap.Definition.CapabilityContract = []string{runtimecontract.StepKindFileEdit, runtimecontract.StepKindFileRead}
	bootstrap.Focuses = []runtimecontract.CoverageFocus{
		{
			ID: "focus-module", SliceID: "slice-module", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-module"},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation},
			TargetRole:            "module_definition", TargetDiscoverySources: []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest},
			TargetSelectorRef: "file/profile_declared_candidates", TargetSelectorOwnerRef: "profile-bootstrap", TargetSelectorSubjectRef: "module",
			TargetSelectorCandidateRefs: []string{"go.mod"}, RequireTargetGrounding: true, RequireAssessment: true,
			AcceptedStepKinds: []string{runtimecontract.StepKindFileEdit, runtimecontract.StepKindFileRead},
		},
		{
			ID: "focus-manager", SliceID: "slice-manager", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-manager"},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation},
			TargetRole:            "implementation", TargetDiscoverySources: []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest},
			TargetSelectorRef: "file/profile_declared_candidates", TargetSelectorOwnerRef: "profile-bootstrap", TargetSelectorSubjectRef: "manager",
			TargetSelectorCandidateRefs: []string{"internal/lsp/manager.go"}, RequireTargetGrounding: true, RequireAssessment: true,
			AcceptedStepKinds: []string{runtimecontract.StepKindFileEdit, runtimecontract.StepKindFileRead},
		},
		{
			ID: "focus-test", SliceID: "slice-test", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-test"},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation},
			TargetRole:            "verification", TargetDiscoverySources: []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest},
			TargetSelectorRef: "file/profile_declared_candidates", TargetSelectorOwnerRef: "profile-bootstrap", TargetSelectorSubjectRef: "manager-test",
			TargetSelectorCandidateRefs: []string{"internal/lsp/manager_test.go"}, RequireTargetGrounding: true, RequireAssessment: true,
			AcceptedStepKinds: []string{runtimecontract.StepKindFileEdit, runtimecontract.StepKindFileRead},
		},
	}
	bootstrap.Slices = []runtimecontract.RuntimeAcceptanceSlice{
		{ID: "slice-module", Title: "module", Scope: "bootstrap", Priority: 1, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-module"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}, CompletionPolicy: runtimecontract.RuntimeAcceptanceSliceCompletionAll, ResultPolicy: runtimecontract.RuntimeAcceptanceSliceResultAllowed},
		{ID: "slice-manager", Title: "manager", Scope: "bootstrap", Priority: 2, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-manager"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}, CompletionPolicy: runtimecontract.RuntimeAcceptanceSliceCompletionAll, ResultPolicy: runtimecontract.RuntimeAcceptanceSliceResultAllowed},
		{ID: "slice-test", Title: "test", Scope: "bootstrap", Priority: 3, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-test"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}, CompletionPolicy: runtimecontract.RuntimeAcceptanceSliceCompletionAll, ResultPolicy: runtimecontract.RuntimeAcceptanceSliceResultAllowed},
	}
	bootstrap.CoverageItems = []runtimecontract.RuntimeCoverageItem{
		{ID: "cwi-module", SliceID: "slice-module"},
		{ID: "cwi-manager", SliceID: "slice-manager"},
		{ID: "cwi-test", SliceID: "slice-test"},
	}
	bootstrap.Requirements = []runtimecontract.RuntimeRequirement{
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleFile, CapabilityClass: runtimecontract.RuntimeCapabilityClassStateChange, RequiredCapabilities: []string{runtimecontract.StepKindFileEdit}},
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleFile, CapabilityClass: runtimecontract.RuntimeCapabilityClassValidation, RequiredCapabilities: []string{runtimecontract.StepKindFileRead}},
	}
	return bootstrap
}

func runtimeV2GroundedTargetValidationBootstrap() runtimecontract.RuntimeBootstrap {
	bootstrap := runtimeV2GroundedTargetSequenceBootstrap()
	bootstrap.Focuses[0].RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	bootstrap.Focuses[0].RequireDirectEvidence = true
	bootstrap.Slices[0].RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	return bootstrap
}

func runtimeV2GroundedTargetSequenceCandidate(command orchestratorevents.RequestRuntimeCandidateCommand) (runtimecontract.RuntimeCandidate, error) {
	target := "implementation.txt"
	switch command.Decision.Action {
	case runtimecontract.RuntimeActionProduceStateChange:
		oldText, newText := "before\n", "after\n"
		return runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
			ID: "grounded-edit", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{target},
			Parameters: []runtimecontract.OperationParameter{{Name: "oldText", String: &oldText}, {Name: "newText", String: &newText}},
		}}}}}, nil
	case runtimecontract.RuntimeActionProduceValidation:
		return runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: runtimecontract.RuntimeOperation{
			ID: "grounded-read", Kind: runtimecontract.StepKindFileRead, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{target},
			Parameters: nil,
		}}}}}, nil
	case runtimecontract.RuntimeActionProduceAssessment:
		refs := make([]string, 0, len(command.CandidateContext.Evidence.Accepted))
		for _, evidence := range command.CandidateContext.Evidence.Accepted {
			refs = append(refs, string(evidence.ID))
		}
		return runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{
			FocusID: "focus-v2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, Disposition: runtimecontract.AssessmentDispositionSatisfied,
			Summary: "grounded target evidence satisfied",
			Items:   []runtimecontract.RuntimeAssessmentItemFact{{CoverageWorkItemID: "cwi-v2", Satisfied: true, EvidenceRefs: refs}}, EvidenceRefs: refs,
		}}, nil
	default:
		return runtimecontract.RuntimeCandidate{}, fmt.Errorf("unexpected grounded sequence action %q", command.Decision.Action)
	}
}

func newRuntimeV2GroundedTargetSequenceModule(t *testing.T, hub magicevent.Hub, runtimeRepository runtimev2.RuntimeStateRepository, contents ...repository.ArtifactContinuationRepository) *Biz {
	t.Helper()
	ensureRuntimeCandidatePreflightTestResponder(hub)
	candidateRequestContents := runtimeV2CandidateRequestContentTestStore(hub)
	if len(contents) > 0 && contents[0] != nil {
		candidateRequestContents = contents[0]
	}
	module := New()
	if err := module.Configure(Dependencies{
		ToolCalls: memory.NewToolCallRepository(), Patches: memory.NewPatchRepository(),
		ValidationResults: memory.NewValidationResultRepository(), EvidenceRecords: memory.NewEvidenceRecordRepository(),
		CoverageWorkItems: memory.NewCoverageWorkItemRepository(), ArtifactContinuations: candidateRequestContents,
		CommandHub: hub,
	}); err != nil {
		t.Fatal(err)
	}
	if err := module.bizPtr.ConfigureRuntimeV2Repository(runtimeRepository); err != nil {
		t.Fatal(err)
	}
	return module
}

func TestRuntimeV2FileBackedGitCapabilityStateSequenceMatrix(t *testing.T) {
	testCases := []struct {
		name               string
		prepare            func(*testing.T, string)
		expectedCandidates [][]string
		expectedFinalRefs  []string
		expectedCount      int
	}{
		{
			name: "repository_absent",
			expectedCandidates: [][]string{
				{"create-commit", "repo-init", "stage-changes"},
				{"create-commit", "stage-changes"},
				{"create-commit"},
			},
			expectedCount: 3,
			prepare: func(t *testing.T, deliverable string) {
				t.Helper()
				writeRuntimeV2GitFixtureFile(t, deliverable)
			},
		},
		{
			name: "initialized_unstaged",
			expectedCandidates: [][]string{
				{"create-commit", "stage-changes"},
				{"create-commit"},
			},
			expectedCount: 2,
			prepare: func(t *testing.T, deliverable string) {
				t.Helper()
				runRuntimeV2Git(t, deliverable, "init", "-q")
				runRuntimeV2Git(t, deliverable, "config", "user.name", "WorkOrch Test")
				runRuntimeV2Git(t, deliverable, "config", "user.email", "workorch@example.test")
				writeRuntimeV2GitFixtureFile(t, deliverable)
			},
		},
		{
			name: "initialized_staged", expectedCandidates: [][]string{{"create-commit"}}, expectedCount: 1,
			prepare: func(t *testing.T, deliverable string) {
				t.Helper()
				runRuntimeV2Git(t, deliverable, "init", "-q")
				runRuntimeV2Git(t, deliverable, "config", "user.name", "WorkOrch Test")
				runRuntimeV2Git(t, deliverable, "config", "user.email", "workorch@example.test")
				writeRuntimeV2GitFixtureFile(t, deliverable)
				runRuntimeV2Git(t, deliverable, "add", "target.txt")
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			hub := magicevent.NewHub(32)
			t.Cleanup(func() { hub.Terminate(ctx) })
			root := t.TempDir()
			deliverable := filepath.Join(root, "workspace")
			internalDir := filepath.Join(root, "internal")
			if err := os.MkdirAll(deliverable, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(internalDir, 0o755); err != nil {
				t.Fatal(err)
			}
			testCase.prepare(t, deliverable)

			bootstrap := runtimeV2GitCapabilityStateBootstrap()
			bootstrap.Requirements = append(bootstrap.Requirements, runtimecontract.RuntimeRequirement{
				Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleGit,
				RequiredCapabilities: []string{runtimecontract.StepKindGitWrite}, Reason: "git owner inventory",
			})
			runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
			if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2", DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, State: common.RunStateActive}); err != nil {
				t.Fatal(err)
			}
			runOwner := runmodule.New()
			if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
				t.Fatal(err)
			}
			if err := runOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = runOwner.Stop(ctx) })
			startRuntimeV2ReceiptTestOwner(t, hub)

			workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
			workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
				result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
			})
			t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
			startRuntimeV2FileManifestOwner(t, ctx, hub)

			receipts, err := gitbiz.NewCapabilityStateReceiptStore(filepath.Join(root, "git-owner-data"))
			if err != nil {
				t.Fatal(err)
			}
			gitOwner := gitmodule.New()
			if err := gitOwner.Configure(gitmodule.Dependencies{EventHub: hub, RuntimeAdapter: gitbiz.NewRuntimeAdapter(true), CapabilityStateReceipts: receipts}); err != nil {
				t.Fatal(err)
			}
			if err := gitOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = gitOwner.Stop(ctx) })

			candidateCalls := 0
			candidateRefsMatch := true
			llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
			llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
				candidateCalls++
				command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
				refs := requiredOperationRefs(command.Decision.RequiredOutput.RequiredOperationSequence)
				if candidateCalls > len(testCase.expectedCandidates) || !equalStrings(refs, testCase.expectedCandidates[candidateCalls-1]) {
					candidateRefsMatch = false
				}
				result.Set(runtimeV2CandidateResultForTest(t, command, runtimeV2GitStateCandidate(refs)), nil)
			})
			t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })

			module := newRuntimeV2DriveTestModule(t, hub)
			driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
			driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, module.handleRuntimeV2Drive)
			t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2) })
			completed, err := DriveRuntimeV2(ctx, hub, orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"})
			if err != nil {
				t.Fatal(err)
			}
			if completed.Action != runtimecontract.RuntimeActionComplete || candidateCalls != len(testCase.expectedCandidates) || !candidateRefsMatch {
				t.Fatalf("completed=%+v candidateCalls=%d candidateRefsMatch=%t", completed, candidateCalls, candidateRefsMatch)
			}
			persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil || persisted.RuntimeState == nil || len(persisted.RuntimeState.OperationReceipts) != testCase.expectedCount {
				t.Fatalf("persisted=%+v err=%v", persisted, err)
			}
			if len(persisted.RuntimeState.Facts.Resources.CapabilityStates) != 1 || !equalStrings(requiredOperationRefs(persisted.RuntimeState.Facts.Resources.CapabilityStates[0].RequiredOperationSequence), testCase.expectedFinalRefs) {
				t.Fatalf("capability state = %+v", persisted.RuntimeState.Facts.Resources.CapabilityStates)
			}
			if len(persisted.RuntimeState.Facts.Resources.OperationAdmissibilities) != 1 ||
				persisted.RuntimeState.Facts.Resources.OperationAdmissibilities[0].AcquisitionBinding == "" ||
				persisted.RuntimeState.Facts.Resources.OperationAdmissibilities[0].AcquisitionBinding != persisted.RuntimeState.Facts.Resources.CapabilityStates[0].AcquisitionBinding {
				t.Fatalf("Git capability/admissibility binding = states=%+v inventories=%+v", persisted.RuntimeState.Facts.Resources.CapabilityStates, persisted.RuntimeState.Facts.Resources.OperationAdmissibilities)
			}
			if output := runtimeV2GitOutput(t, deliverable, "rev-list", "--count", "HEAD"); strings.TrimSpace(output) != "1" {
				t.Fatalf("commit count = %q, want 1", output)
			}
		})
	}
}

func TestRuntimeV2FileBackedGitRestartAfterInitEvidenceCommitContinuesSequence(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	root := t.TempDir()
	deliverable, internalDir := filepath.Join(root, "workspace"), filepath.Join(root, "internal")
	if err := os.MkdirAll(deliverable, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(internalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeRuntimeV2GitFixtureFile(t, deliverable)

	bootstrap := runtimeV2GitCapabilityStateBootstrap()
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2", DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, State: common.RunStateActive}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	startRuntimeV2ReceiptTestOwner(t, hub)
	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	startRuntimeV2FileManifestOwner(t, ctx, hub)
	receipts, err := gitbiz.NewCapabilityStateReceiptStore(filepath.Join(root, "git-owner-data"))
	if err != nil {
		t.Fatal(err)
	}
	gitOwner := gitmodule.New()
	if err := gitOwner.Configure(gitmodule.Dependencies{EventHub: hub, RuntimeAdapter: gitbiz.NewRuntimeAdapter(true), CapabilityStateReceipts: receipts}); err != nil {
		t.Fatal(err)
	}
	if err := gitOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = gitOwner.Stop(ctx) })

	module := newRuntimeV2DriveTestModule(t, hub)
	run, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err := module.bizPtr.InitializeRuntimeV2FromRun(ctx, run)
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err = module.prepareRuntimeV2Prerequisites(ctx, aggregate)
	if err != nil {
		t.Fatal(err)
	}
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(aggregate.Definition, aggregate.Facts.Activation.ExecutionContract)
	if _, err := module.bizPtr.BeginRuntimeV2CandidateAttemptForTest(ctx, "ws-v2", "run-v2", 0, prefix); err != nil {
		t.Fatal(err)
	}
	if _, err := module.bizPtr.AcceptRuntimeV2CandidateAttempt(ctx, "ws-v2", "run-v2", 0, runtimeV2GitStateCandidate([]string{"create-commit", "repo-init", "stage-changes"})); err != nil {
		t.Fatal(err)
	}
	baseRepository := runtimeV2StateRepository{hub: hub}
	crashRepository := &runtimeV2SaveCrashRepository{
		delegate: baseRepository, after: true, marker: "git-init-evidence",
		match: func(current runtimev2.RuntimeAggregate) bool {
			if current.ActiveDispatch != nil || current.EvidenceSequence != 1 || len(current.OperationReceipts) != 1 ||
				current.Decision.Action != runtimecontract.RuntimeActionAcquirePrerequisite || len(current.CompletedDispatches) != 1 ||
				current.CompletedDispatches[0].State != runtimev2.DispatchStateInvalidated || len(current.CompletedDispatches[0].CompletedOperationIDs) != 1 {
				return false
			}
			return current.CompletedDispatches[0].CompletedOperationIDs[0] == "git-repo-init"
		},
	}
	crashingController, err := runtimev2.NewRuntimeController(crashRepository)
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "git-init-evidence" {
				t.Fatalf("unexpected crash marker: %v", recovered)
			}
		}()
		_, _ = crashingController.ExecuteActiveDispatch(ctx, "ws-v2", "run-v2", eventRuntimeV2OperationExecutor{hub: hub})
		t.Fatal("expected injected crash")
	}()
	crashed, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || crashed.RuntimeState == nil || len(crashed.RuntimeState.OperationReceipts) != 1 {
		t.Fatalf("crashed run=%+v err=%v", crashed, err)
	}
	initReceipt := crashed.RuntimeState.OperationReceipts[0]
	if initReceipt.OperationID != "git-repo-init" {
		t.Fatalf("first receipt=%+v", initReceipt)
	}

	candidateCalls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		candidateCalls++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		result.Set(runtimeV2CandidateResultForTest(t, command, runtimeV2GitStateCandidate(requiredOperationRefs(command.Decision.RequiredOutput.RequiredOperationSequence))), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	restarted := newRuntimeV2DriveTestModule(t, hub)
	driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, restarted.handleRuntimeV2Drive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2) })
	completed, err := DriveRuntimeV2(ctx, hub, orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"})
	if err != nil || completed.Action != runtimecontract.RuntimeActionComplete || candidateCalls != 2 {
		t.Fatalf("restart completed=%+v candidateCalls=%d err=%v", completed, candidateCalls, err)
	}
	persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.RuntimeState == nil || len(persisted.RuntimeState.OperationReceipts) != 3 {
		t.Fatalf("persisted=%+v err=%v", persisted, err)
	}
	countInit := 0
	for _, receipt := range persisted.RuntimeState.OperationReceipts {
		if receipt.OperationID == "git-repo-init" {
			countInit++
			if receipt.ExecutionKey != initReceipt.ExecutionKey || receipt.Fence != initReceipt.Fence {
				t.Fatalf("init receipt was replaced: before=%+v after=%+v", initReceipt, receipt)
			}
		}
	}
	if countInit != 1 || strings.TrimSpace(runtimeV2GitOutput(t, deliverable, "rev-list", "--count", "HEAD")) != "1" {
		t.Fatalf("init count=%d", countInit)
	}
}

func TestRuntimeV2FileBackedGitQueryReceiptCrashBeforePrerequisiteCommitReusesSnapshot(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	root := t.TempDir()
	deliverable, internalDir := filepath.Join(root, "workspace"), filepath.Join(root, "internal")
	if err := os.MkdirAll(deliverable, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(internalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeRuntimeV2GitFixtureFile(t, deliverable)
	bootstrap := runtimeV2GitCapabilityStateBootstrap()
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2", DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, State: common.RunStateActive}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	startRuntimeV2ReceiptTestOwner(t, hub)
	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	startRuntimeV2FileManifestOwner(t, ctx, hub)
	storeRoot := filepath.Join(root, "git-owner-data")
	store, err := gitbiz.NewCapabilityStateReceiptStore(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	gitOwner := gitmodule.New()
	if err := gitOwner.Configure(gitmodule.Dependencies{EventHub: hub, RuntimeAdapter: gitbiz.NewRuntimeAdapter(true), CapabilityStateReceipts: store}); err != nil {
		t.Fatal(err)
	}
	if err := gitOwner.Start(); err != nil {
		t.Fatal(err)
	}

	baseRepository := runtimeV2StateRepository{hub: hub}
	crashRepository := &runtimeV2SaveCrashRepository{
		delegate: baseRepository, marker: "git-query-before-prerequisite-commit",
		match: func(current runtimev2.RuntimeAggregate) bool {
			return len(current.Facts.Resources.CapabilityStates) == 1 && current.Decision.Action != runtimecontract.RuntimeActionAcquirePrerequisite
		},
	}
	crashingBiz := orchestratorbiz.NewRuntimeOrchestrator()
	if err := crashingBiz.ConfigureRuntimeV2Repository(crashRepository); err != nil {
		t.Fatal(err)
	}
	crashingModule := newTestBiz(crashingBiz, hub, nil)
	run, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err := crashingBiz.InitializeRuntimeV2FromRun(ctx, run)
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "git-query-before-prerequisite-commit" {
				t.Fatalf("unexpected crash marker: %v", recovered)
			}
		}()
		_, _ = crashingModule.prepareRuntimeV2Prerequisites(ctx, aggregate)
		t.Fatal("expected injected crash")
	}()
	if !crashRepository.fired {
		t.Fatal("query/prerequisite crash point was not reached")
	}
	crashed, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || crashed.RuntimeState == nil || len(crashed.RuntimeState.Facts.Resources.CapabilityStates) != 0 {
		t.Fatalf("crashed run=%+v err=%v", crashed, err)
	}
	var prerequisiteID string
	for _, prerequisite := range crashed.RuntimeState.Facts.Prerequisites.Pending {
		if prerequisite.Kind == runtimecontract.PrerequisiteKindCapabilityState {
			prerequisiteID = prerequisite.ID
			break
		}
	}
	if prerequisiteID == "" {
		t.Fatalf("missing persisted capability-state prerequisite: %+v", crashed.RuntimeState.Facts.Prerequisites)
	}
	acquisitionKey := stableRuntimeV2CapabilityStateAcquisitionKey("ws-v2", "run-v2", prerequisiteID, 0)
	queried, found, err := store.Load("ws-v2", acquisitionKey)
	if err != nil || !found {
		t.Fatalf("durable query receipt found=%t err=%v", found, err)
	}
	// A controller-only restart would retain the old EventHub's observer graph.
	// Tear down that entire graph and rebuild every process-local participant
	// from the file-backed Run and Git receipt stores instead.
	if err := gitOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	hub.Terminate(ctx)

	restartedHub := magicevent.NewHub(32)
	t.Cleanup(func() { restartedHub.Terminate(ctx) })
	restartedRuns := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	restartedRunOwner := runmodule.New()
	if err := restartedRunOwner.Configure(runmodule.Dependencies{Runs: restartedRuns, EventHub: restartedHub}); err != nil {
		t.Fatal(err)
	}
	if err := restartedRunOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedRunOwner.Stop(ctx) })
	startRuntimeV2ReceiptTestOwner(t, restartedHub)
	restartedWorkspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, restartedHub)
	restartedWorkspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
	})
	t.Cleanup(func() { restartedWorkspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	startRuntimeV2FileManifestOwner(t, ctx, restartedHub)
	reopenedStore, err := gitbiz.NewCapabilityStateReceiptStore(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	restartedGitOwner := gitmodule.New()
	if err := restartedGitOwner.Configure(gitmodule.Dependencies{EventHub: restartedHub, RuntimeAdapter: gitbiz.NewRuntimeAdapter(true), CapabilityStateReceipts: reopenedStore}); err != nil {
		t.Fatal(err)
	}
	if err := restartedGitOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedGitOwner.Stop(ctx) })

	candidateCalls := 0
	firstCandidateReusedQueryReceipt := false
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, restartedHub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		candidateCalls++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		result.Set(runtimeV2CandidateResultForTest(t, command, runtimeV2GitStateCandidate(requiredOperationRefs(command.Decision.RequiredOutput.RequiredOperationSequence))), nil)
		if candidateCalls == 1 && (len(command.CandidateContext.Resources.CapabilityStates) != 1 ||
			command.CandidateContext.Resources.CapabilityStates[0].SnapshotID != queried.SnapshotID ||
			command.CandidateContext.Resources.CapabilityStates[0].AcquisitionKey != queried.AcquisitionKey ||
			command.CandidateContext.Resources.CapabilityStates[0].QueryReceiptRef != queried.QueryReceiptRef) {
			t.Errorf("restart did not reuse durable query receipt in first candidate: %+v", command.CandidateContext.Resources.CapabilityStates)
			return
		}
		if candidateCalls == 1 {
			firstCandidateReusedQueryReceipt = true
		}
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	restarted := newRuntimeV2DriveTestModule(t, restartedHub)
	driveObserver := magicevent.NewSimpleObserver(ModuleID, restartedHub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, restarted.handleRuntimeV2Drive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2) })
	completed, err := DriveRuntimeV2(ctx, restartedHub, orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"})
	if err != nil || completed.Action != runtimecontract.RuntimeActionComplete || candidateCalls != 3 || !firstCandidateReusedQueryReceipt {
		t.Fatalf("restart completed=%+v candidateCalls=%d err=%v", completed, candidateCalls, err)
	}
	persisted, err := restartedRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.RuntimeState == nil || len(persisted.RuntimeState.Facts.Resources.CapabilityStates) != 1 {
		t.Fatalf("persisted=%+v err=%v", persisted, err)
	}
	if len(persisted.RuntimeState.OperationReceipts) != 3 {
		t.Fatalf("restart did not complete refreshed Git sequence: receipts=%+v", persisted.RuntimeState.OperationReceipts)
	}
}

// Manifest acquisition is a File-owner prerequisite, but the receipt is
// authoritative only after the Run aggregate CAS. A response loss after that
// CAS must leave the next process with the committed manifest and only the
// remaining prerequisites; it must not depend on the old File observer.
func TestRuntimeV2FileBackedManifestCommitResponseLossRestartReusesCommittedManifest(t *testing.T) {
	runRuntimeV2FileBackedManifestCommitCrashRestart(t, true)
}

func TestRuntimeV2FileBackedManifestCommitBeforeCASRestartReacquiresManifest(t *testing.T) {
	runRuntimeV2FileBackedManifestCommitCrashRestart(t, false)
}

func runRuntimeV2FileBackedManifestCommitCrashRestart(t *testing.T, crashAfterCAS bool) {
	ctx := context.Background()
	root := t.TempDir()
	deliverable := filepath.Join(root, "workspace")
	if err := os.MkdirAll(deliverable, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deliverable, "target.txt"), []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runsPath := filepath.Join(root, "runs.json")
	bootstrap := runtimeV2Bootstrap()
	bootstrap.Requirements = append([]runtimecontract.RuntimeRequirement{
		{Kind: runtimecontract.PrerequisiteKindManifest, OwnerModuleID: runtimecontract.RuntimeModuleFile, Reason: "file owner manifest"},
	}, bootstrap.Requirements...)
	runs := filerepository.NewRunRepository(runsPath)
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2", DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, State: common.RunStateActive,
	}); err != nil {
		t.Fatal(err)
	}

	startProcess := func(t *testing.T) (magicevent.Hub, *filerepository.RunRepository, *runmodule.Module, *filemodule.Module, magicevent.SimpleObserver) {
		t.Helper()
		hub := magicevent.NewHub(16)
		processRuns := filerepository.NewRunRepository(runsPath)
		runOwner := runmodule.New()
		if err := runOwner.Configure(runmodule.Dependencies{Runs: processRuns, EventHub: hub}); err != nil {
			hub.Terminate(ctx)
			t.Fatal(err)
		}
		if err := runOwner.Start(); err != nil {
			hub.Terminate(ctx)
			t.Fatal(err)
		}
		workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
		workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
			result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable}}, nil)
		})
		fileOwner := filemodule.New()
		if err := fileOwner.Configure(filemodule.Dependencies{EventHub: hub}); err != nil {
			workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
			_ = runOwner.Stop(ctx)
			hub.Terminate(ctx)
			t.Fatal(err)
		}
		if err := fileOwner.Start(); err != nil {
			workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
			_ = runOwner.Stop(ctx)
			hub.Terminate(ctx)
			t.Fatal(err)
		}
		return hub, processRuns, runOwner, fileOwner, workspaceObserver
	}
	stopProcess := func(hub magicevent.Hub, runOwner *runmodule.Module, fileOwner *filemodule.Module, workspaceObserver magicevent.SimpleObserver) {
		if fileOwner != nil {
			_ = fileOwner.Stop(ctx)
		}
		if workspaceObserver != nil {
			workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
		}
		if runOwner != nil {
			_ = runOwner.Stop(ctx)
		}
		if hub != nil {
			hub.Terminate(ctx)
		}
	}

	hub, processRuns, runOwner, fileOwner, workspaceObserver := startProcess(t)
	defer stopProcess(hub, runOwner, fileOwner, workspaceObserver)
	baseRepository := runtimeV2StateRepository{hub: hub}
	crashMarker := "manifest-commit-before-cas"
	if crashAfterCAS {
		crashMarker = "manifest-response-loss"
	}
	crashRepository := &runtimeV2SaveCrashRepository{
		delegate: baseRepository, after: crashAfterCAS, marker: crashMarker,
		match: func(current runtimev2.RuntimeAggregate) bool {
			return current.Facts.Resources.Manifest != nil && current.Facts.Resources.Manifest.Revision != ""
		},
	}
	crashingBiz := orchestratorbiz.NewRuntimeOrchestrator()
	if err := crashingBiz.ConfigureRuntimeV2Repository(crashRepository); err != nil {
		t.Fatal(err)
	}
	crashingModule := newTestBiz(crashingBiz, hub, nil)
	run, err := processRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err := crashingBiz.InitializeRuntimeV2FromRun(ctx, run)
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != crashMarker {
				t.Fatalf("unexpected manifest response-loss marker: %v", recovered)
			}
		}()
		_, _ = crashingModule.prepareRuntimeV2Prerequisites(ctx, aggregate)
		t.Fatal("expected manifest response loss")
	}()
	if !crashRepository.fired {
		t.Fatal("manifest response-loss CAS point was not reached")
	}
	crashed, err := processRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || crashed.RuntimeState == nil {
		t.Fatalf("manifest response-loss aggregate=%+v err=%v", crashed, err)
	}
	var committedManifest *runtimecontract.WorkspaceManifest
	if crashAfterCAS {
		if crashed.RuntimeState.Facts.Resources.Manifest == nil || crashed.RuntimeState.Facts.Resources.Manifest.Revision == "" {
			t.Fatalf("after-CAS manifest was not committed: %+v", crashed.RuntimeState)
		}
		value := *crashed.RuntimeState.Facts.Resources.Manifest
		committedManifest = &value
	} else if crashed.RuntimeState.Facts.Resources.Manifest != nil {
		t.Fatalf("before-CAS manifest leaked into persisted aggregate: %+v", crashed.RuntimeState)
	}
	foundPendingManifest := false
	for _, pending := range crashed.RuntimeState.Facts.Prerequisites.Pending {
		foundPendingManifest = foundPendingManifest || pending.Kind == runtimecontract.PrerequisiteKindManifest
	}
	if crashAfterCAS && foundPendingManifest {
		t.Fatalf("committed manifest remained pending: %+v", crashed.RuntimeState.Facts.Prerequisites)
	}
	if !crashAfterCAS && !foundPendingManifest {
		t.Fatalf("before-CAS manifest was removed from pending prerequisites: %+v", crashed.RuntimeState.Facts.Prerequisites)
	}

	// Destroy every process-local participant before recovery. Only the file
	// repositories and workspace directory remain.
	stopProcess(hub, runOwner, fileOwner, workspaceObserver)
	hub, processRuns, runOwner, fileOwner, workspaceObserver = startProcess(t)
	defer stopProcess(hub, runOwner, fileOwner, workspaceObserver)
	restarted := newRuntimeV2DriveTestModule(t, hub)
	loaded, err := restarted.bizPtr.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := restarted.prepareRuntimeV2Prerequisites(ctx, loaded)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Facts.Resources.Manifest == nil || resumed.Facts.Resources.Manifest.Revision == "" ||
		len(resumed.Facts.Prerequisites.Pending) != 0 || resumed.Decision.Action != runtimecontract.RuntimeActionProduceStateChange ||
		len(resumed.CandidateAttempts) != 0 || resumed.ActiveDispatch != nil {
		t.Fatalf("restart did not reuse committed manifest without entering candidate path: %+v", resumed)
	}
	if committedManifest != nil && resumed.Facts.Resources.Manifest.Revision != committedManifest.Revision {
		t.Fatalf("restart did not reuse committed manifest: got=%+v want=%+v", resumed.Facts.Resources.Manifest, committedManifest)
	}
}

// Provider ceiling is the only prerequisite acquired from the LLM owner before
// any candidate request exists.  Its query result may become authority only
// through the Run-owner CAS: a process loss before that CAS needs exactly one
// new LLM owner query, while a loss after it must reuse the same sealed ceiling
// and observation without requesting a candidate or provider completion.
func TestRuntimeV2FileBackedProviderCeilingCommitCrashRestart(t *testing.T) {
	for _, crashAfterCAS := range []bool{false, true} {
		phase := "before_cas_reacquires_provider_ceiling"
		if crashAfterCAS {
			phase = "after_cas_reuses_provider_ceiling"
		}
		t.Run(phase, func(t *testing.T) {
			ctx := context.Background()
			useTestLLMConfig(t, 128000, 8192)
			root := t.TempDir()
			deliverable := filepath.Join(root, "workspace")
			if err := os.MkdirAll(deliverable, 0o755); err != nil {
				t.Fatal(err)
			}
			runsPath := filepath.Join(root, "runs.json")
			bootstrap := runtimeV2Bootstrap()
			bootstrap.Activation.ProviderTarget = runtimecontract.ProviderModelTarget{ProviderID: "custom-test", ModelID: "model-1"}
			bootstrap.Requirements = []runtimecontract.RuntimeRequirement{{
				Kind: runtimecontract.PrerequisiteKindProviderCeiling, OwnerModuleID: llmmodule.ModuleID,
				MinimumBytes: 32 * 1024, Reason: "sealed provider ceiling",
			}}
			if err := filerepository.NewRunRepository(runsPath).Create(ctx, &runmodel.Run{
				ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
				DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, State: common.RunStateActive,
			}); err != nil {
				t.Fatal(err)
			}

			startProcess := func(t *testing.T) (magicevent.Hub, *filerepository.RunRepository, *runmodule.Module, *llmmodule.Module, magicevent.SimpleObserver, magicevent.SimpleObserver, *int) {
				t.Helper()
				// The module must resolve configuration from a freshly opened source on
				// every simulated process start; no DefaultLLMTarget or process-global
				// test configuration is reused as prerequisite authority.
				if err := workorchconfig.Close(); err != nil {
					t.Fatal(err)
				}
				hub := magicevent.NewHub(16)
				processRuns := filerepository.NewRunRepository(runsPath)
				runOwner := runmodule.New()
				if err := runOwner.Configure(runmodule.Dependencies{Runs: processRuns, EventHub: hub}); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := runOwner.Start(); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
				workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
					result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable}}, nil)
				})
				llmOwner := llmmodule.New()
				if err := llmOwner.Configure(llmmodule.Dependencies{EventHub: hub}); err != nil {
					workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
					_ = runOwner.Stop(ctx)
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := llmOwner.Start(); err != nil {
					workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
					_ = runOwner.Stop(ctx)
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				ceilingCalls := new(int)
				// The spy is deliberately not a responder. LLM Biz remains the only
				// production consumer of the typed acquire command.
				ceilingObserver := magicevent.NewSimpleObserverWithMatchID("provider-ceiling-query-spy", "/runtime/#", hub)
				ceilingObserver.Subscribe(orchestratorevents.TopicOrchestratorAcquireProviderCeiling, func(event magicevent.Event, _ magicevent.Result) {
					if _, ok := event.Data().(orchestratorevents.AcquireProviderCeilingCommand); !ok {
						t.Errorf("provider ceiling spy received %T", event.Data())
						return
					}
					*ceilingCalls++
				})
				return hub, processRuns, runOwner, llmOwner, workspaceObserver, ceilingObserver, ceilingCalls
			}
			stopProcess := func(hub magicevent.Hub, runOwner *runmodule.Module, llmOwner *llmmodule.Module, workspaceObserver, ceilingObserver magicevent.SimpleObserver) {
				if ceilingObserver != nil {
					ceilingObserver.Unsubscribe(orchestratorevents.TopicOrchestratorAcquireProviderCeiling)
				}
				if llmOwner != nil {
					_ = llmOwner.Stop(ctx)
				}
				if workspaceObserver != nil {
					workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
				}
				if runOwner != nil {
					_ = runOwner.Stop(ctx)
				}
				if hub != nil {
					hub.Terminate(ctx)
				}
			}

			hub, processRuns, runOwner, llmOwner, workspaceObserver, ceilingObserver, ceilingCalls := startProcess(t)
			baseRepository := runtimeV2StateRepository{hub: hub}
			marker := "provider-ceiling-before-cas"
			if crashAfterCAS {
				marker = "provider-ceiling-after-cas"
			}
			crashRepository := &runtimeV2SaveCrashRepository{
				delegate: baseRepository, after: crashAfterCAS, marker: marker,
				match: func(current runtimev2.RuntimeAggregate) bool {
					return current.Facts.Resources.ProviderCeiling != nil && current.Facts.Resources.ProviderCeiling.ConfigurationRevision != ""
				},
			}
			crashingBiz := orchestratorbiz.NewRuntimeOrchestrator()
			if err := crashingBiz.ConfigureRuntimeV2Repository(crashRepository); err != nil {
				t.Fatal(err)
			}
			crashingModule := newTestBiz(crashingBiz, hub, nil)
			run, err := processRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			aggregate, err := crashingBiz.InitializeRuntimeV2FromRun(ctx, run)
			if err != nil {
				t.Fatal(err)
			}
			func() {
				defer func() {
					if recovered := recover(); recovered != marker {
						t.Fatalf("unexpected provider ceiling crash marker: %v", recovered)
					}
				}()
				_, _ = crashingModule.prepareRuntimeV2Prerequisites(ctx, aggregate)
				t.Fatal("expected provider ceiling crash")
			}()
			if !crashRepository.fired || *ceilingCalls != 1 {
				t.Fatalf("provider ceiling crash fired=%t calls=%d", crashRepository.fired, *ceilingCalls)
			}
			crashed, err := processRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil || crashed.RuntimeState == nil {
				t.Fatalf("provider ceiling crashed state=%+v err=%v", crashed, err)
			}
			var committedCeiling *runtimecontract.ProviderCeiling
			var committedObservation *runtimecontract.RuntimePrerequisiteObservation
			if crashAfterCAS {
				if crashed.RuntimeState.Facts.Resources.ProviderCeiling == nil || len(crashed.RuntimeState.Facts.Resources.PrerequisiteObservations) != 1 {
					t.Fatalf("after-CAS provider ceiling was not persisted: %+v", crashed.RuntimeState)
				}
				ceiling := *crashed.RuntimeState.Facts.Resources.ProviderCeiling
				committedCeiling = &ceiling
				observation := crashed.RuntimeState.Facts.Resources.PrerequisiteObservations[0]
				committedObservation = &observation
			} else if crashed.RuntimeState.Facts.Resources.ProviderCeiling != nil || len(crashed.RuntimeState.Facts.Resources.PrerequisiteObservations) != 0 {
				t.Fatalf("before-CAS provider ceiling leaked into persisted aggregate: %+v", crashed.RuntimeState)
			}
			if len(crashed.RuntimeState.CandidateRequestSessions) != 0 || len(crashed.RuntimeState.CandidateAttempts) != 0 || crashed.RuntimeState.ActiveDispatch != nil ||
				len(crashed.RuntimeState.CompletedDispatches) != 0 || len(crashed.RuntimeState.OperationReceipts) != 0 || len(crashed.RuntimeState.GovernanceRequests) != 0 {
				t.Fatalf("provider ceiling acquisition entered a candidate or side-effect path: %+v", crashed.RuntimeState)
			}

			// A restart destroys every process-local actor, including the LLM module
			// and the configuration cache it used. Only file-backed Run state and the
			// temporary workspace/config source remain.
			stopProcess(hub, runOwner, llmOwner, workspaceObserver, ceilingObserver)
			hub, processRuns, runOwner, llmOwner, workspaceObserver, ceilingObserver, restartedCalls := startProcess(t)
			defer stopProcess(hub, runOwner, llmOwner, workspaceObserver, ceilingObserver)
			restarted := newRuntimeV2DriveTestModule(t, hub)
			loaded, err := restarted.bizPtr.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			resumed, err := restarted.prepareRuntimeV2Prerequisites(ctx, loaded)
			if err != nil {
				t.Fatal(err)
			}
			// This fixture intentionally requests only a ceiling. Once it is
			// committed the reducer may reject the otherwise capability-less test
			// definition; that terminal is orthogonal to acquisition. The required
			// property here is that no candidate or side effect was introduced.
			if resumed.Facts.Resources.ProviderCeiling == nil || len(resumed.Facts.Prerequisites.Pending) != 0 ||
				len(resumed.CandidateRequestSessions) != 0 ||
				len(resumed.CandidateAttempts) != 0 || resumed.ActiveDispatch != nil || len(resumed.OperationReceipts) != 0 || len(resumed.GovernanceRequests) != 0 {
				t.Fatalf("provider ceiling restart entered an unexpected runtime path: %+v", resumed)
			}
			if !crashAfterCAS && *restartedCalls != 1 {
				t.Fatalf("before-CAS restart LLM ceiling queries=%d, want 1", *restartedCalls)
			}
			if crashAfterCAS && *restartedCalls != 0 {
				t.Fatalf("after-CAS restart re-queried committed LLM ceiling: calls=%d", *restartedCalls)
			}
			if committedCeiling != nil {
				got := resumed.Facts.Resources.ProviderCeiling
				if got.Target != committedCeiling.Target || got.CapabilityRevision != committedCeiling.CapabilityRevision ||
					got.ConfigurationRevision != committedCeiling.ConfigurationRevision || got.MaxBytes != committedCeiling.MaxBytes ||
					got.ContextWindowTokens != committedCeiling.ContextWindowTokens || got.EffectiveMaxOutputTokens != committedCeiling.EffectiveMaxOutputTokens {
					t.Fatalf("restart did not reuse committed provider ceiling: got=%+v want=%+v", got, committedCeiling)
				}
				observation, ok := runtimecontract.RuntimePrerequisiteObservationForID(resumed.Facts.Resources.PrerequisiteObservations, committedObservation.PrerequisiteID)
				if !ok || observation.ObservationID != committedObservation.ObservationID || observation.ContentHash != committedObservation.ContentHash ||
					len(observation.Dependencies) != 1 || len(committedObservation.Dependencies) != 1 || observation.Dependencies[0] != committedObservation.Dependencies[0] {
					t.Fatalf("restart did not reuse committed provider ceiling observation: got=%+v want=%+v", observation, committedObservation)
				}
			}
		})
	}
}

// Capability prerequisites are dynamically addressed to their declared owner.
// Verify the File owner path through a real module rather than a test
// responder: an uncommitted result is queried again by the next process, while
// a committed capability set is reused without a second owner query or any
// candidate/dispatch work.
func TestRuntimeV2FileBackedFileCapabilityCommitCrashRestart(t *testing.T) {
	for _, crashAfterCAS := range []bool{false, true} {
		phase := "before_cas_reacquires_file_capability"
		if crashAfterCAS {
			phase = "after_cas_reuses_file_capability"
		}
		t.Run(phase, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			deliverable := filepath.Join(root, "workspace")
			if err := os.MkdirAll(deliverable, 0o755); err != nil {
				t.Fatal(err)
			}
			runsPath := filepath.Join(root, "runs.json")
			bootstrap := runtimeV2Bootstrap()
			bootstrap.Requirements = []runtimecontract.RuntimeRequirement{{
				Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: filemodule.ModuleID,
				CapabilityClass:      runtimecontract.RuntimeCapabilityClassStateChange,
				RequiredCapabilities: []string{runtimecontract.StepKindFileEdit}, Reason: "sealed file capability",
			}}
			if err := filerepository.NewRunRepository(runsPath).Create(ctx, &runmodel.Run{
				ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
				DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, State: common.RunStateActive,
			}); err != nil {
				t.Fatal(err)
			}

			startProcess := func(t *testing.T) (magicevent.Hub, *filerepository.RunRepository, *runmodule.Module, *filemodule.Module, magicevent.SimpleObserver, magicevent.SimpleObserver, *int) {
				t.Helper()
				hub := magicevent.NewHub(16)
				processRuns := filerepository.NewRunRepository(runsPath)
				runOwner := runmodule.New()
				if err := runOwner.Configure(runmodule.Dependencies{Runs: processRuns, EventHub: hub}); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := runOwner.Start(); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
				workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
					result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable}}, nil)
				})
				fileOwner := filemodule.New()
				if err := fileOwner.Configure(filemodule.Dependencies{EventHub: hub}); err != nil {
					workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
					_ = runOwner.Stop(ctx)
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := fileOwner.Start(); err != nil {
					workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
					_ = runOwner.Stop(ctx)
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				capabilityCalls := new(int)
				capabilityObserver := magicevent.NewSimpleObserverWithMatchID("file-capability-query-spy", "blocks/#", hub)
				capabilityObserver.Subscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeCapabilities, func(event magicevent.Event, _ magicevent.Result) {
					command, ok := event.Data().(orchestratorevents.AcquireRuntimeCapabilitiesCommand)
					if !ok || command.ModuleID != filemodule.ModuleID {
						t.Errorf("file capability spy received %T / %+v", event.Data(), event.Data())
						return
					}
					*capabilityCalls++
				})
				return hub, processRuns, runOwner, fileOwner, workspaceObserver, capabilityObserver, capabilityCalls
			}
			stopProcess := func(hub magicevent.Hub, runOwner *runmodule.Module, fileOwner *filemodule.Module, workspaceObserver, capabilityObserver magicevent.SimpleObserver) {
				if capabilityObserver != nil {
					capabilityObserver.Unsubscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeCapabilities)
				}
				if fileOwner != nil {
					_ = fileOwner.Stop(ctx)
				}
				if workspaceObserver != nil {
					workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
				}
				if runOwner != nil {
					_ = runOwner.Stop(ctx)
				}
				if hub != nil {
					hub.Terminate(ctx)
				}
			}

			hub, processRuns, runOwner, fileOwner, workspaceObserver, capabilityObserver, capabilityCalls := startProcess(t)
			baseRepository := runtimeV2StateRepository{hub: hub}
			marker := "file-capability-before-cas"
			if crashAfterCAS {
				marker = "file-capability-after-cas"
			}
			crashRepository := &runtimeV2SaveCrashRepository{
				delegate: baseRepository, after: crashAfterCAS, marker: marker,
				match: func(current runtimev2.RuntimeAggregate) bool {
					return len(current.Facts.Capabilities.StateChangeKinds) == 1 && current.Facts.Capabilities.StateChangeKinds[0] == runtimecontract.StepKindFileEdit
				},
			}
			crashingBiz := orchestratorbiz.NewRuntimeOrchestrator()
			if err := crashingBiz.ConfigureRuntimeV2Repository(crashRepository); err != nil {
				t.Fatal(err)
			}
			crashingModule := newTestBiz(crashingBiz, hub, nil)
			run, err := processRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			aggregate, err := crashingBiz.InitializeRuntimeV2FromRun(ctx, run)
			if err != nil {
				t.Fatal(err)
			}
			func() {
				defer func() {
					if recovered := recover(); recovered != marker {
						t.Fatalf("unexpected file capability crash marker: %v", recovered)
					}
				}()
				_, _ = crashingModule.prepareRuntimeV2Prerequisites(ctx, aggregate)
				t.Fatal("expected file capability crash")
			}()
			if !crashRepository.fired || *capabilityCalls != 1 {
				t.Fatalf("file capability crash fired=%t calls=%d", crashRepository.fired, *capabilityCalls)
			}
			crashed, err := processRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil || crashed.RuntimeState == nil {
				t.Fatalf("file capability crashed state=%+v err=%v", crashed, err)
			}
			if crashAfterCAS {
				if len(crashed.RuntimeState.Facts.Capabilities.StateChangeKinds) != 1 || crashed.RuntimeState.Facts.Capabilities.StateChangeKinds[0] != runtimecontract.StepKindFileEdit {
					t.Fatalf("after-CAS file capability was not persisted: %+v", crashed.RuntimeState.Facts.Capabilities)
				}
			} else if len(crashed.RuntimeState.Facts.Capabilities.StateChangeKinds) != 0 {
				t.Fatalf("before-CAS file capability leaked into persisted aggregate: %+v", crashed.RuntimeState.Facts.Capabilities)
			}
			if len(crashed.RuntimeState.CandidateRequestSessions) != 0 || len(crashed.RuntimeState.CandidateAttempts) != 0 || crashed.RuntimeState.ActiveDispatch != nil ||
				len(crashed.RuntimeState.CompletedDispatches) != 0 || len(crashed.RuntimeState.OperationReceipts) != 0 || len(crashed.RuntimeState.GovernanceRequests) != 0 {
				t.Fatalf("file capability acquisition entered a candidate or side-effect path: %+v", crashed.RuntimeState)
			}

			stopProcess(hub, runOwner, fileOwner, workspaceObserver, capabilityObserver)
			hub, processRuns, runOwner, fileOwner, workspaceObserver, capabilityObserver, restartedCalls := startProcess(t)
			defer stopProcess(hub, runOwner, fileOwner, workspaceObserver, capabilityObserver)
			restarted := newRuntimeV2DriveTestModule(t, hub)
			loaded, err := restarted.bizPtr.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			resumed, err := restarted.prepareRuntimeV2Prerequisites(ctx, loaded)
			if err != nil {
				t.Fatal(err)
			}
			if len(resumed.Facts.Capabilities.StateChangeKinds) != 1 || resumed.Facts.Capabilities.StateChangeKinds[0] != runtimecontract.StepKindFileEdit ||
				len(resumed.Facts.Prerequisites.Pending) != 0 || resumed.Decision.Action != runtimecontract.RuntimeActionProduceStateChange ||
				len(resumed.CandidateRequestSessions) != 0 || len(resumed.CandidateAttempts) != 0 || resumed.ActiveDispatch != nil ||
				len(resumed.OperationReceipts) != 0 || len(resumed.GovernanceRequests) != 0 {
				t.Fatalf("file capability restart entered an unexpected runtime path: %+v", resumed)
			}
			if !crashAfterCAS && *restartedCalls != 1 {
				t.Fatalf("before-CAS restart File capability queries=%d, want 1", *restartedCalls)
			}
			if crashAfterCAS && *restartedCalls != 0 {
				t.Fatalf("after-CAS restart re-queried committed File capability: calls=%d", *restartedCalls)
			}
		})
	}
}

// Git owns a distinct capability receipt implementation from File. Cover both
// Run-owner CAS sides with rebuilt Hub/Run/Git actors so an uncommitted Git
// result is queried again, while a committed one is never rediscovered by the
// restarted process.
func TestRuntimeV2FileBackedGitCapabilityCommitCrashRestart(t *testing.T) {
	for _, crashAfterCAS := range []bool{false, true} {
		phase := "before_cas_reacquires_git_capability"
		if crashAfterCAS {
			phase = "after_cas_reuses_git_capability"
		}
		t.Run(phase, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			deliverable := filepath.Join(root, "workspace")
			internalDirectory := filepath.Join(root, "internal")
			if err := os.MkdirAll(deliverable, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(internalDirectory, 0o755); err != nil {
				t.Fatal(err)
			}
			runsPath := filepath.Join(root, "runs.json")
			bootstrap := runtimeV2Bootstrap()
			bootstrap.Requirements = []runtimecontract.RuntimeRequirement{{
				Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: gitmodule.ModuleID,
				CapabilityClass: runtimecontract.RuntimeCapabilityClassStateChange, RequiredCapabilities: []string{gitmodule.StepKindWrite}, Reason: "sealed git capability",
			}}
			if err := filerepository.NewRunRepository(runsPath).Create(ctx, &runmodel.Run{
				ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
				DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, State: common.RunStateActive,
			}); err != nil {
				t.Fatal(err)
			}

			startProcess := func(t *testing.T) (magicevent.Hub, *filerepository.RunRepository, *runmodule.Module, *gitmodule.Module, magicevent.SimpleObserver, magicevent.SimpleObserver, *int) {
				t.Helper()
				hub := magicevent.NewHub(16)
				processRuns := filerepository.NewRunRepository(runsPath)
				runOwner := runmodule.New()
				if err := runOwner.Configure(runmodule.Dependencies{Runs: processRuns, EventHub: hub}); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := runOwner.Start(); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
				workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
					result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDirectory}}, nil)
				})
				store, err := gitbiz.NewCapabilityStateReceiptStore(filepath.Join(root, "git-capability-receipts"))
				if err != nil {
					workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
					_ = runOwner.Stop(ctx)
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				gitOwner := gitmodule.New()
				if err := gitOwner.Configure(gitmodule.Dependencies{EventHub: hub, RuntimeAdapter: gitbiz.NewRuntimeAdapter(true), CapabilityStateReceipts: store}); err != nil {
					workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
					_ = runOwner.Stop(ctx)
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := gitOwner.Start(); err != nil {
					workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
					_ = runOwner.Stop(ctx)
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				calls := new(int)
				spy := magicevent.NewSimpleObserverWithMatchID("git-capability-query-spy", "blocks/#", hub)
				spy.Subscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeCapabilities, func(event magicevent.Event, _ magicevent.Result) {
					command, ok := event.Data().(orchestratorevents.AcquireRuntimeCapabilitiesCommand)
					if !ok || command.ModuleID != gitmodule.ModuleID || command.CapabilityClass != runtimecontract.RuntimeCapabilityClassStateChange || len(command.RequiredKinds) != 1 || command.RequiredKinds[0] != gitmodule.StepKindWrite {
						t.Errorf("git capability spy received %T / %+v", event.Data(), event.Data())
						return
					}
					*calls++
				})
				return hub, processRuns, runOwner, gitOwner, workspaceObserver, spy, calls
			}
			stopProcess := func(hub magicevent.Hub, runOwner *runmodule.Module, gitOwner *gitmodule.Module, workspaceObserver, spy magicevent.SimpleObserver) {
				if spy != nil {
					spy.Unsubscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeCapabilities)
				}
				if gitOwner != nil {
					_ = gitOwner.Stop(ctx)
				}
				if workspaceObserver != nil {
					workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
				}
				if runOwner != nil {
					_ = runOwner.Stop(ctx)
				}
				if hub != nil {
					hub.Terminate(ctx)
				}
			}

			hub, processRuns, runOwner, gitOwner, workspaceObserver, spy, calls := startProcess(t)
			baseRepository := runtimeV2StateRepository{hub: hub}
			marker := "git-capability-before-cas"
			if crashAfterCAS {
				marker = "git-capability-after-cas"
			}
			crashRepository := &runtimeV2SaveCrashRepository{delegate: baseRepository, after: crashAfterCAS, marker: marker, match: func(current runtimev2.RuntimeAggregate) bool {
				return len(current.Facts.Capabilities.StateChangeKinds) == 1 && current.Facts.Capabilities.StateChangeKinds[0] == gitmodule.StepKindWrite
			}}
			crashingOwner := orchestratorbiz.NewRuntimeOrchestrator()
			if err := crashingOwner.ConfigureRuntimeV2Repository(crashRepository); err != nil {
				t.Fatal(err)
			}
			crashing := newTestBiz(crashingOwner, hub, nil)
			run, err := processRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			aggregate, err := crashingOwner.InitializeRuntimeV2FromRun(ctx, run)
			if err != nil {
				t.Fatal(err)
			}
			func() {
				defer func() {
					if recovered := recover(); recovered != marker {
						t.Fatalf("unexpected git capability crash marker: %v", recovered)
					}
				}()
				_, _ = crashing.prepareRuntimeV2Prerequisites(ctx, aggregate)
				t.Fatal("expected git capability crash")
			}()
			if !crashRepository.fired || *calls != 1 {
				t.Fatalf("git capability crash fired=%t calls=%d", crashRepository.fired, *calls)
			}
			crashed, err := processRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil || crashed.RuntimeState == nil {
				t.Fatalf("git capability crashed state=%+v err=%v", crashed, err)
			}
			if crashAfterCAS {
				if len(crashed.RuntimeState.Facts.Capabilities.StateChangeKinds) != 1 || crashed.RuntimeState.Facts.Capabilities.StateChangeKinds[0] != gitmodule.StepKindWrite {
					t.Fatalf("after-CAS git capability was not persisted: %+v", crashed.RuntimeState.Facts.Capabilities)
				}
			} else if len(crashed.RuntimeState.Facts.Capabilities.StateChangeKinds) != 0 {
				t.Fatalf("before-CAS git capability leaked into persisted aggregate: %+v", crashed.RuntimeState.Facts.Capabilities)
			}
			if len(crashed.RuntimeState.CandidateRequestSessions) != 0 || len(crashed.RuntimeState.CandidateAttempts) != 0 || crashed.RuntimeState.ActiveDispatch != nil ||
				len(crashed.RuntimeState.CompletedDispatches) != 0 || len(crashed.RuntimeState.OperationReceipts) != 0 || len(crashed.RuntimeState.GovernanceRequests) != 0 {
				t.Fatalf("git capability acquisition entered a candidate or side-effect path: %+v", crashed.RuntimeState)
			}
			stopProcess(hub, runOwner, gitOwner, workspaceObserver, spy)

			hub, processRuns, runOwner, gitOwner, workspaceObserver, spy, restartedCalls := startProcess(t)
			defer stopProcess(hub, runOwner, gitOwner, workspaceObserver, spy)
			restarted := newRuntimeV2DriveTestModule(t, hub)
			loaded, err := restarted.bizPtr.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			resumed, err := restarted.prepareRuntimeV2Prerequisites(ctx, loaded)
			if err != nil {
				t.Fatal(err)
			}
			if len(resumed.Facts.Capabilities.StateChangeKinds) != 1 || resumed.Facts.Capabilities.StateChangeKinds[0] != gitmodule.StepKindWrite ||
				len(resumed.Facts.Prerequisites.Pending) != 0 || resumed.Decision.Action != runtimecontract.RuntimeActionProduceStateChange ||
				len(resumed.CandidateRequestSessions) != 0 || len(resumed.CandidateAttempts) != 0 || resumed.ActiveDispatch != nil ||
				len(resumed.OperationReceipts) != 0 || len(resumed.GovernanceRequests) != 0 {
				t.Fatalf("git capability restart entered an unexpected runtime path: %+v", resumed)
			}
			if !crashAfterCAS && *restartedCalls != 1 {
				t.Fatalf("before-CAS restart Git capability queries=%d, want 1", *restartedCalls)
			}
			if crashAfterCAS && *restartedCalls != 0 {
				t.Fatalf("after-CAS restart re-queried committed Git capability: calls=%d", *restartedCalls)
			}
		})
	}
}

// Shell shares the generic capability receipt constructor with File and Git,
// but it is a separately wired runtime owner and serves a different capability
// class. Exercise the real Shell receiver across both Run-owner CAS boundaries
// so its EventHub subscription cannot silently drift from the shared contract.
func TestRuntimeV2FileBackedShellCapabilityCommitCrashRestart(t *testing.T) {
	for _, crashAfterCAS := range []bool{false, true} {
		phase := "before_cas_reacquires_shell_capability"
		if crashAfterCAS {
			phase = "after_cas_reuses_shell_capability"
		}
		t.Run(phase, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			deliverable := filepath.Join(root, "workspace")
			internalDirectory := filepath.Join(root, "internal")
			if err := os.MkdirAll(deliverable, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(internalDirectory, 0o755); err != nil {
				t.Fatal(err)
			}
			runsPath := filepath.Join(root, "runs.json")
			bootstrap := runtimeV2Bootstrap()
			bootstrap.Requirements = []runtimecontract.RuntimeRequirement{{
				Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: shellmodule.ModuleID,
				CapabilityClass: runtimecontract.RuntimeCapabilityClassValidation, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, Reason: "sealed shell validation capability",
			}}
			if err := filerepository.NewRunRepository(runsPath).Create(ctx, &runmodel.Run{
				ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
				DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, State: common.RunStateActive,
			}); err != nil {
				t.Fatal(err)
			}

			startProcess := func(t *testing.T) (magicevent.Hub, *filerepository.RunRepository, *runmodule.Module, *shellmodule.Module, magicevent.SimpleObserver, magicevent.SimpleObserver, *int) {
				t.Helper()
				hub := magicevent.NewHub(16)
				processRuns := filerepository.NewRunRepository(runsPath)
				runOwner := runmodule.New()
				if err := runOwner.Configure(runmodule.Dependencies{Runs: processRuns, EventHub: hub}); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := runOwner.Start(); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
				workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
					result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDirectory}}, nil)
				})
				shellOwner := shellmodule.New()
				if err := shellOwner.Configure(shellmodule.Dependencies{EventHub: hub, RuntimeAdapter: shellbiz.NewRuntimeAdapter()}); err != nil {
					workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
					_ = runOwner.Stop(ctx)
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := shellOwner.Start(); err != nil {
					workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
					_ = runOwner.Stop(ctx)
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				calls := new(int)
				spy := magicevent.NewSimpleObserverWithMatchID("shell-capability-query-spy", "blocks/#", hub)
				spy.Subscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeCapabilities, func(event magicevent.Event, _ magicevent.Result) {
					command, ok := event.Data().(orchestratorevents.AcquireRuntimeCapabilitiesCommand)
					if !ok || command.ModuleID != shellmodule.ModuleID || command.CapabilityClass != runtimecontract.RuntimeCapabilityClassValidation || len(command.RequiredKinds) != 1 || command.RequiredKinds[0] != runtimecontract.RuntimeBlockKindManagedValidationSession {
						t.Errorf("shell capability spy received %T / %+v", event.Data(), event.Data())
						return
					}
					*calls++
				})
				return hub, processRuns, runOwner, shellOwner, workspaceObserver, spy, calls
			}
			stopProcess := func(hub magicevent.Hub, runOwner *runmodule.Module, shellOwner *shellmodule.Module, workspaceObserver, spy magicevent.SimpleObserver) {
				if spy != nil {
					spy.Unsubscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeCapabilities)
				}
				if shellOwner != nil {
					_ = shellOwner.Stop(ctx)
				}
				if workspaceObserver != nil {
					workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
				}
				if runOwner != nil {
					_ = runOwner.Stop(ctx)
				}
				if hub != nil {
					hub.Terminate(ctx)
				}
			}

			hub, processRuns, runOwner, shellOwner, workspaceObserver, spy, calls := startProcess(t)
			baseRepository := runtimeV2StateRepository{hub: hub}
			marker := "shell-capability-before-cas"
			if crashAfterCAS {
				marker = "shell-capability-after-cas"
			}
			crashRepository := &runtimeV2SaveCrashRepository{delegate: baseRepository, after: crashAfterCAS, marker: marker, match: func(current runtimev2.RuntimeAggregate) bool {
				return len(current.Facts.Capabilities.ValidationKinds) == 1 && current.Facts.Capabilities.ValidationKinds[0] == runtimecontract.RuntimeBlockKindManagedValidationSession
			}}
			crashingOwner := orchestratorbiz.NewRuntimeOrchestrator()
			if err := crashingOwner.ConfigureRuntimeV2Repository(crashRepository); err != nil {
				t.Fatal(err)
			}
			crashing := newTestBiz(crashingOwner, hub, nil)
			run, err := processRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			aggregate, err := crashingOwner.InitializeRuntimeV2FromRun(ctx, run)
			if err != nil {
				t.Fatal(err)
			}
			func() {
				defer func() {
					if recovered := recover(); recovered != marker {
						t.Fatalf("unexpected shell capability crash marker: %v", recovered)
					}
				}()
				_, _ = crashing.prepareRuntimeV2Prerequisites(ctx, aggregate)
				t.Fatal("expected shell capability crash")
			}()
			if !crashRepository.fired || *calls != 1 {
				t.Fatalf("shell capability crash fired=%t calls=%d", crashRepository.fired, *calls)
			}
			crashed, err := processRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil || crashed.RuntimeState == nil {
				t.Fatalf("shell capability crashed state=%+v err=%v", crashed, err)
			}
			if crashAfterCAS {
				if len(crashed.RuntimeState.Facts.Capabilities.ValidationKinds) != 1 || crashed.RuntimeState.Facts.Capabilities.ValidationKinds[0] != runtimecontract.RuntimeBlockKindManagedValidationSession {
					t.Fatalf("after-CAS shell capability was not persisted: %+v", crashed.RuntimeState.Facts.Capabilities)
				}
			} else if len(crashed.RuntimeState.Facts.Capabilities.ValidationKinds) != 0 {
				t.Fatalf("before-CAS shell capability leaked into persisted aggregate: %+v", crashed.RuntimeState.Facts.Capabilities)
			}
			if len(crashed.RuntimeState.CandidateRequestSessions) != 0 || len(crashed.RuntimeState.CandidateAttempts) != 0 || crashed.RuntimeState.ActiveDispatch != nil ||
				len(crashed.RuntimeState.CompletedDispatches) != 0 || len(crashed.RuntimeState.OperationReceipts) != 0 || len(crashed.RuntimeState.GovernanceRequests) != 0 {
				t.Fatalf("shell capability acquisition entered a candidate or side-effect path: %+v", crashed.RuntimeState)
			}
			stopProcess(hub, runOwner, shellOwner, workspaceObserver, spy)

			hub, processRuns, runOwner, shellOwner, workspaceObserver, spy, restartedCalls := startProcess(t)
			defer stopProcess(hub, runOwner, shellOwner, workspaceObserver, spy)
			restarted := newRuntimeV2DriveTestModule(t, hub)
			loaded, err := restarted.bizPtr.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			resumed, err := restarted.prepareRuntimeV2Prerequisites(ctx, loaded)
			if err != nil {
				t.Fatal(err)
			}
			// This fixture intentionally has no state-change capability. The
			// reducer therefore terminalizes after acquisition; the capability
			// receiver boundary remains candidate-free on either CAS side.
			if len(resumed.Facts.Capabilities.ValidationKinds) != 1 || resumed.Facts.Capabilities.ValidationKinds[0] != runtimecontract.RuntimeBlockKindManagedValidationSession ||
				len(resumed.Facts.Prerequisites.Pending) != 0 || resumed.Decision.Action != runtimecontract.RuntimeActionFail ||
				len(resumed.CandidateRequestSessions) != 0 || len(resumed.CandidateAttempts) != 0 || resumed.ActiveDispatch != nil ||
				len(resumed.OperationReceipts) != 0 || len(resumed.GovernanceRequests) != 0 {
				t.Fatalf("shell capability restart entered an unexpected runtime path: %+v", resumed)
			}
			if !crashAfterCAS && *restartedCalls != 1 {
				t.Fatalf("before-CAS restart Shell capability queries=%d, want 1", *restartedCalls)
			}
			if crashAfterCAS && *restartedCalls != 0 {
				t.Fatalf("after-CAS restart re-queried committed Shell capability: calls=%d", *restartedCalls)
			}
		})
	}
}

// Validation scratch is a local prerequisite, but its filesystem resource is
// still subject to the same authority boundary as owner receipts.  The path is
// derived under Workspace.InternalDirectory; an uncommitted path is never
// authority after restart, while a committed one is reused without touching a
// provider, candidate or operation path.
func TestRuntimeV2FileBackedValidationScratchCommitCrashRestart(t *testing.T) {
	for _, crashAfterCAS := range []bool{false, true} {
		phase := "before_cas_reacquires_validation_scratch"
		if crashAfterCAS {
			phase = "after_cas_reuses_validation_scratch"
		}
		t.Run(phase, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			deliverable := filepath.Join(root, "workspace")
			internalDirectory := filepath.Join(root, "internal")
			if err := os.MkdirAll(deliverable, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(internalDirectory, 0o755); err != nil {
				t.Fatal(err)
			}
			runsPath := filepath.Join(root, "runs.json")
			bootstrap := runtimeV2Bootstrap()
			bootstrap.Requirements = []runtimecontract.RuntimeRequirement{
				{Kind: runtimecontract.PrerequisiteKindValidationScratch, OwnerModuleID: runtimecontract.RuntimeModuleOrchestrator, Reason: "workspace scoped validation scratch"},
				{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleFile,
					CapabilityClass: runtimecontract.RuntimeCapabilityClassStateChange, RequiredCapabilities: []string{runtimecontract.StepKindFileEdit}, Reason: "file capability keeps the authority nonterminal"},
			}
			if err := filerepository.NewRunRepository(runsPath).Create(ctx, &runmodel.Run{
				ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
				DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, State: common.RunStateActive,
			}); err != nil {
				t.Fatal(err)
			}

			startProcess := func(t *testing.T) (magicevent.Hub, *filerepository.RunRepository, *runmodule.Module, magicevent.SimpleObserver) {
				t.Helper()
				hub := magicevent.NewHub(16)
				processRuns := filerepository.NewRunRepository(runsPath)
				runOwner := runmodule.New()
				if err := runOwner.Configure(runmodule.Dependencies{Runs: processRuns, EventHub: hub}); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := runOwner.Start(); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
				workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
					result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{
						ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDirectory,
					}}, nil)
				})
				startRuntimeV2FileManifestOwner(t, ctx, hub)
				return hub, processRuns, runOwner, workspaceObserver
			}
			stopProcess := func(hub magicevent.Hub, runOwner *runmodule.Module, workspaceObserver magicevent.SimpleObserver) {
				if workspaceObserver != nil {
					workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
				}
				if runOwner != nil {
					_ = runOwner.Stop(ctx)
				}
				if hub != nil {
					hub.Terminate(ctx)
				}
			}

			hub, processRuns, runOwner, workspaceObserver := startProcess(t)
			baseRepository := runtimeV2StateRepository{hub: hub}
			marker := "validation-scratch-before-cas"
			if crashAfterCAS {
				marker = "validation-scratch-after-cas"
			}
			crashRepository := &runtimeV2SaveCrashRepository{
				delegate: baseRepository, after: crashAfterCAS, marker: marker,
				match: func(current runtimev2.RuntimeAggregate) bool {
					return current.Facts.Resources.ValidationScratch != nil && current.Facts.Resources.ValidationScratch.Reference != ""
				},
			}
			crashingBiz := orchestratorbiz.NewRuntimeOrchestrator()
			if err := crashingBiz.ConfigureRuntimeV2Repository(crashRepository); err != nil {
				t.Fatal(err)
			}
			crashingModule := newTestBiz(crashingBiz, hub, nil)
			run, err := processRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			aggregate, err := crashingBiz.InitializeRuntimeV2FromRun(ctx, run)
			if err != nil {
				t.Fatal(err)
			}
			func() {
				defer func() {
					if recovered := recover(); recovered != marker {
						t.Fatalf("unexpected validation scratch crash marker: %v", recovered)
					}
				}()
				_, _ = crashingModule.prepareRuntimeV2Prerequisites(ctx, aggregate)
				t.Fatal("expected validation scratch crash")
			}()
			if !crashRepository.fired {
				t.Fatal("validation scratch CAS crash point was not reached")
			}
			crashed, err := processRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil || crashed.RuntimeState == nil {
				t.Fatalf("validation scratch crashed state=%+v err=%v", crashed, err)
			}
			var committedScratch *runtimecontract.ValidationScratch
			if crashAfterCAS {
				if crashed.RuntimeState.Facts.Resources.ValidationScratch == nil {
					t.Fatalf("after-CAS validation scratch was not persisted: %+v", crashed.RuntimeState)
				}
				scratch := *crashed.RuntimeState.Facts.Resources.ValidationScratch
				committedScratch = &scratch
			} else if crashed.RuntimeState.Facts.Resources.ValidationScratch != nil {
				t.Fatalf("before-CAS validation scratch leaked into persisted aggregate: %+v", crashed.RuntimeState)
			}
			if len(crashed.RuntimeState.CandidateRequestSessions) != 0 || len(crashed.RuntimeState.CandidateAttempts) != 0 || crashed.RuntimeState.ActiveDispatch != nil ||
				len(crashed.RuntimeState.CompletedDispatches) != 0 || len(crashed.RuntimeState.OperationReceipts) != 0 || len(crashed.RuntimeState.GovernanceRequests) != 0 {
				t.Fatalf("validation scratch acquisition entered a candidate or side-effect path: %+v", crashed.RuntimeState)
			}

			stopProcess(hub, runOwner, workspaceObserver)
			hub, processRuns, runOwner, workspaceObserver = startProcess(t)
			defer stopProcess(hub, runOwner, workspaceObserver)
			restarted := newRuntimeV2DriveTestModule(t, hub)
			loaded, err := restarted.bizPtr.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			resumed, err := restarted.prepareRuntimeV2Prerequisites(ctx, loaded)
			if err != nil {
				t.Fatal(err)
			}
			if resumed.Facts.Resources.ValidationScratch == nil || len(resumed.Facts.Prerequisites.Pending) != 0 ||
				len(resumed.CandidateRequestSessions) != 0 || len(resumed.CandidateAttempts) != 0 || resumed.ActiveDispatch != nil ||
				len(resumed.OperationReceipts) != 0 || len(resumed.GovernanceRequests) != 0 {
				t.Fatalf("validation scratch restart entered an unexpected runtime path: %+v", resumed)
			}
			if !strings.HasPrefix(resumed.Facts.Resources.ValidationScratch.Reference, filepath.Join(internalDirectory, "validation-scratch")) {
				t.Fatalf("validation scratch escaped workspace internal directory: %+v", resumed.Facts.Resources.ValidationScratch)
			}
			if info, statErr := os.Stat(resumed.Facts.Resources.ValidationScratch.Reference); statErr != nil || !info.IsDir() {
				t.Fatalf("validation scratch directory = %v, info=%+v", statErr, info)
			}
			if committedScratch != nil && (resumed.Facts.Resources.ValidationScratch.Reference != committedScratch.Reference ||
				resumed.Facts.Resources.ValidationScratch.Revision != committedScratch.Revision) {
				t.Fatalf("restart did not reuse committed validation scratch: got=%+v want=%+v", resumed.Facts.Resources.ValidationScratch, committedScratch)
			}
		})
	}
}

// A committed scratch can disappear independently of any candidate or owner
// operation. That loss is a read-only SM-2 stale transition: both sides of the
// invalidation CAS must restart into exactly the scratch acquisition path, not
// a provider request, candidate attempt, governance ticket or durable effect.
func TestRuntimeV2FileBackedValidationScratchStaleInvalidationCrashRestart(t *testing.T) {
	runRuntimeV2FileBackedValidationScratchStaleInvalidationCrashRestart(t, 0)
}

// The read-only stale ledger is independent from owner receipt progress. Its
// exhausted edge must persist a terminal fact after a complete process reload;
// it may not create a provider request, candidate, dispatch or ticket.
func TestRuntimeV2FileBackedValidationScratchStaleRefreshExhaustionIsTerminal(t *testing.T) {
	runRuntimeV2FileBackedValidationScratchStaleInvalidationCrashRestart(t, 1)
}

func runRuntimeV2FileBackedValidationScratchStaleInvalidationCrashRestart(t *testing.T, maxStaleRefreshes int) {
	for _, crashAfterCAS := range []bool{false, true} {
		phase := "before_cas_keeps_committed_scratch_until_stale_replayed"
		if crashAfterCAS {
			phase = "after_cas_reacquires_only_scratch"
		}
		t.Run(phase, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			deliverable := filepath.Join(root, "workspace")
			internalDirectory := filepath.Join(root, "internal")
			if err := os.MkdirAll(deliverable, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(internalDirectory, 0o755); err != nil {
				t.Fatal(err)
			}
			runsPath := filepath.Join(root, "runs.json")
			bootstrap := runtimeV2Bootstrap()
			bootstrap.Requirements = []runtimecontract.RuntimeRequirement{
				{
					Kind: runtimecontract.PrerequisiteKindValidationScratch, OwnerModuleID: runtimecontract.RuntimeModuleOrchestrator,
					Reason: "workspace scoped validation scratch",
				},
				{
					Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleFile,
					CapabilityClass:      runtimecontract.RuntimeCapabilityClassStateChange,
					RequiredCapabilities: []string{runtimecontract.StepKindFileEdit},
				},
			}
			if maxStaleRefreshes > 0 {
				bootstrap.RecoveryPolicy.MaxPrerequisiteRefreshes = maxStaleRefreshes
			}
			if err := filerepository.NewRunRepository(runsPath).Create(ctx, &runmodel.Run{
				ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
				DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, State: common.RunStateActive,
			}); err != nil {
				t.Fatal(err)
			}

			startProcess := func(t *testing.T) (magicevent.Hub, *filerepository.RunRepository, *runmodule.Module, magicevent.SimpleObserver) {
				t.Helper()
				hub := magicevent.NewHub(16)
				processRuns := filerepository.NewRunRepository(runsPath)
				runOwner := runmodule.New()
				if err := runOwner.Configure(runmodule.Dependencies{Runs: processRuns, EventHub: hub}); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := runOwner.Start(); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
				workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
					result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{
						ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDirectory,
					}}, nil)
				})
				startRuntimeV2FileManifestOwner(t, ctx, hub)
				return hub, processRuns, runOwner, workspaceObserver
			}
			stopProcess := func(hub magicevent.Hub, runOwner *runmodule.Module, workspaceObserver magicevent.SimpleObserver) {
				if workspaceObserver != nil {
					workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
				}
				if runOwner != nil {
					_ = runOwner.Stop(ctx)
				}
				if hub != nil {
					hub.Terminate(ctx)
				}
			}

			// Process one obtains and commits the scoped resource through the normal
			// local prerequisite path, then exits completely before resource loss.
			hub, processRuns, runOwner, workspaceObserver := startProcess(t)
			first := newRuntimeV2DriveTestModule(t, hub)
			run, err := processRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			aggregate, err := first.bizPtr.InitializeRuntimeV2FromRun(ctx, run)
			if err != nil {
				t.Fatal(err)
			}
			aggregate, err = first.prepareRuntimeV2Prerequisites(ctx, aggregate)
			if err != nil || aggregate.Facts.Resources.ValidationScratch == nil {
				t.Fatalf("initial scratch acquisition aggregate=%+v err=%v", aggregate, err)
			}
			committedScratch := *aggregate.Facts.Resources.ValidationScratch
			if err := os.RemoveAll(committedScratch.Reference); err != nil {
				t.Fatal(err)
			}
			stopProcess(hub, runOwner, workspaceObserver)

			// Process two sees the deleted resource. Fault only the exact
			// read-only invalidation write, before or after its Run-owner CAS.
			hub, processRuns, runOwner, workspaceObserver = startProcess(t)
			baseRepository := runtimeV2StateRepository{hub: hub}
			marker := "validation-scratch-stale-before-cas"
			if crashAfterCAS {
				marker = "validation-scratch-stale-after-cas"
			}
			crashRepository := &runtimeV2SaveCrashRepository{
				delegate: baseRepository, after: crashAfterCAS, marker: marker,
				match: func(current runtimev2.RuntimeAggregate) bool {
					return current.Facts.Resources.ValidationScratch == nil && current.Facts.Recovery.PrerequisiteRefreshes == 1 &&
						len(current.Facts.Prerequisites.Pending) == 1 && current.Facts.Prerequisites.Pending[0].Kind == runtimecontract.PrerequisiteKindValidationScratch
				},
			}
			crashingOwner := orchestratorbiz.NewRuntimeOrchestrator()
			if err := crashingOwner.ConfigureRuntimeV2Repository(crashRepository); err != nil {
				t.Fatal(err)
			}
			crashing := newTestBiz(crashingOwner, hub, nil)
			loaded, err := crashing.bizPtr.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			func() {
				defer func() {
					if recovered := recover(); recovered != marker {
						t.Fatalf("unexpected validation scratch stale crash marker: %v", recovered)
					}
				}()
				_, _ = crashing.driveRuntimeV2Decision(ctx, loaded, "stable-prefix-test")
				t.Fatal("expected validation scratch stale invalidation crash")
			}()
			if !crashRepository.fired {
				t.Fatal("validation scratch stale CAS crash point was not reached")
			}
			crashed, err := processRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil || crashed.RuntimeState == nil {
				t.Fatalf("stale crash state=%+v err=%v", crashed, err)
			}
			if crashAfterCAS {
				if crashed.RuntimeState.Facts.Resources.ValidationScratch != nil || crashed.RuntimeState.Facts.Recovery.PrerequisiteRefreshes != 1 {
					t.Fatalf("after-CAS stale invalidation was not durable: %+v", crashed.RuntimeState)
				}
			} else if crashed.RuntimeState.Facts.Resources.ValidationScratch == nil || crashed.RuntimeState.Facts.Recovery.PrerequisiteRefreshes != 0 {
				t.Fatalf("before-CAS stale invalidation leaked: %+v", crashed.RuntimeState)
			}
			if len(crashed.RuntimeState.CandidateRequestSessions) != 0 || len(crashed.RuntimeState.CandidateAttempts) != 0 || crashed.RuntimeState.ActiveDispatch != nil ||
				len(crashed.RuntimeState.GovernanceRequests) != 0 || len(crashed.RuntimeState.OperationReceipts) != 0 {
				t.Fatalf("scratch stale invalidation entered candidate or side-effect path: %+v", crashed.RuntimeState)
			}
			stopProcess(hub, runOwner, workspaceObserver)

			// Process three is entirely new. If process two crashed before CAS it
			// performs the one stale retirement; if after CAS it merely reacquires
			// the scoped scratch. Either way no other prerequisite or candidate work
			// is allowed.
			hub, processRuns, runOwner, workspaceObserver = startProcess(t)
			defer func() { stopProcess(hub, runOwner, workspaceObserver) }()
			restarted := newRuntimeV2DriveTestModule(t, hub)
			loaded, err = restarted.bizPtr.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			if !crashAfterCAS {
				loaded, err = restarted.driveRuntimeV2Decision(ctx, loaded, "stable-prefix-test")
				if err != nil {
					t.Fatal(err)
				}
			}
			resumed, err := restarted.prepareRuntimeV2Prerequisites(ctx, loaded)
			if err != nil {
				t.Fatal(err)
			}
			if resumed.Facts.Resources.ValidationScratch == nil || resumed.Facts.Resources.ValidationScratch.Reference != committedScratch.Reference ||
				resumed.Facts.Resources.ValidationScratch.Revision == committedScratch.Revision || len(resumed.Facts.Prerequisites.Pending) != 0 ||
				resumed.Facts.Recovery.PrerequisiteRefreshes != 1 || len(resumed.CandidateRequestSessions) != 0 || len(resumed.CandidateAttempts) != 0 ||
				resumed.ActiveDispatch != nil || len(resumed.GovernanceRequests) != 0 || len(resumed.OperationReceipts) != 0 {
				t.Fatalf("stale restart did not reacquire only scratch: %+v", resumed)
			}
			if info, statErr := os.Stat(resumed.Facts.Resources.ValidationScratch.Reference); statErr != nil || !info.IsDir() {
				t.Fatalf("reacquired scratch directory = %v, info=%+v", statErr, info)
			}
			if maxStaleRefreshes > 0 {
				if err := os.RemoveAll(resumed.Facts.Resources.ValidationScratch.Reference); err != nil {
					t.Fatal(err)
				}
				marker := "validation-scratch-exhaustion-before-cas"
				if crashAfterCAS {
					marker = "validation-scratch-exhaustion-after-cas"
				}
				crashRepository := &runtimeV2SaveCrashRepository{
					delegate: runtimeV2StateRepository{hub: hub}, after: crashAfterCAS, marker: marker,
					match: func(current runtimev2.RuntimeAggregate) bool {
						return current.Decision.Action == runtimecontract.RuntimeActionFail && current.Facts.Failure != nil &&
							current.Facts.Failure.Code == "prerequisite_refresh_exhausted" &&
							current.Facts.Recovery.PrerequisiteRefreshes == maxStaleRefreshes
					},
				}
				crashingOwner := orchestratorbiz.NewRuntimeOrchestrator()
				if err := crashingOwner.ConfigureRuntimeV2Repository(crashRepository); err != nil {
					t.Fatal(err)
				}
				crashing := newTestBiz(crashingOwner, hub, nil)
				func() {
					defer func() {
						if recovered := recover(); recovered != marker {
							t.Fatalf("unexpected read-only exhaustion crash marker: %v", recovered)
						}
					}()
					_, _ = crashing.driveRuntimeV2Decision(ctx, resumed, "stable-prefix-test")
					t.Fatal("expected read-only exhaustion terminal CAS crash")
				}()
				if !crashRepository.fired {
					t.Fatal("read-only exhaustion terminal CAS point was not reached")
				}
				crashed, err := processRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
				if err != nil || crashed.RuntimeState == nil {
					t.Fatalf("read-only exhaustion crash state=%+v err=%v", crashed, err)
				}
				if crashAfterCAS {
					if crashed.RuntimeState.Decision.Action != runtimecontract.RuntimeActionFail || crashed.RuntimeState.Facts.Failure == nil ||
						crashed.RuntimeState.Facts.Failure.Code != "prerequisite_refresh_exhausted" || crashed.RuntimeState.Facts.Resources.ValidationScratch != nil ||
						crashed.RuntimeState.Facts.Recovery.PrerequisiteRefreshes != maxStaleRefreshes {
						t.Fatalf("after-CAS read-only exhaustion was not terminal: %+v", crashed.RuntimeState)
					}
				} else if crashed.RuntimeState.Decision.Action == runtimecontract.RuntimeActionFail || crashed.RuntimeState.Facts.Resources.ValidationScratch == nil ||
					crashed.RuntimeState.Facts.Recovery.PrerequisiteRefreshes != maxStaleRefreshes {
					t.Fatalf("before-CAS read-only exhaustion leaked terminal authority: %+v", crashed.RuntimeState)
				}

				// The next process observes either the committed terminal or the
				// original stale resource and converges once. It never requests a
				// candidate or touches the already exhausted ledger again.
				stopProcess(hub, runOwner, workspaceObserver)
				hub, runOwner, workspaceObserver = nil, nil, nil
				terminalHub, terminalRuns, terminalRunOwner, terminalWorkspaceObserver := startProcess(t)
				defer stopProcess(terminalHub, terminalRunOwner, terminalWorkspaceObserver)
				terminalBiz := newRuntimeV2DriveTestModule(t, terminalHub)
				terminalResult, terminalErr := terminalBiz.driveRuntimeV2Locked(ctx, orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"})
				if terminalErr != nil || terminalResult.Action != runtimecontract.RuntimeActionFail {
					t.Fatalf("read-only exhaustion fresh terminal reload=%+v err=%v", terminalResult, terminalErr)
				}
				persisted, loadErr := terminalRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
				if loadErr != nil || persisted.RuntimeState == nil || persisted.RuntimeState.Decision.Action != runtimecontract.RuntimeActionFail ||
					persisted.RuntimeState.Facts.Failure == nil || persisted.RuntimeState.Facts.Failure.Code != "prerequisite_refresh_exhausted" ||
					persisted.RuntimeState.Facts.Recovery.PrerequisiteRefreshes != maxStaleRefreshes ||
					len(persisted.RuntimeState.CandidateRequestSessions) != 0 || len(persisted.RuntimeState.CandidateAttempts) != 0 ||
					persisted.RuntimeState.ActiveDispatch != nil || len(persisted.RuntimeState.GovernanceRequests) != 0 || len(persisted.RuntimeState.OperationReceipts) != 0 {
					t.Fatalf("read-only exhaustion persisted terminal=%+v err=%v", persisted.RuntimeState, loadErr)
				}
			}
		})
	}
}

// FailureTarget has no external responder, but its local mapping remains a
// persisted SM-2 prerequisite. Rebuild every process actor around the
// Run-owner file store to prove an uncommitted mapping is derived again while
// an after-CAS response loss is reused without another authority transition.
func TestRuntimeV2FileBackedFailureTargetCommitCrashRestart(t *testing.T) {
	for _, crashAfterCAS := range []bool{false, true} {
		phase := "before_cas_rederives_local_mapping"
		if crashAfterCAS {
			phase = "after_cas_reuses_local_mapping"
		}
		t.Run(phase, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			deliverable := filepath.Join(root, "workspace")
			if err := os.MkdirAll(deliverable, 0o755); err != nil {
				t.Fatal(err)
			}
			runsPath := filepath.Join(root, "runs.json")
			bootstrap := runtimeV2Bootstrap()
			if err := filerepository.NewRunRepository(runsPath).Create(ctx, &runmodel.Run{
				ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
				DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, State: common.RunStateActive,
			}); err != nil {
				t.Fatal(err)
			}

			startProcess := func(t *testing.T) (magicevent.Hub, *filerepository.RunRepository, *runmodule.Module, magicevent.SimpleObserver) {
				t.Helper()
				hub := magicevent.NewHub(16)
				processRuns := filerepository.NewRunRepository(runsPath)
				runOwner := runmodule.New()
				if err := runOwner.Configure(runmodule.Dependencies{Runs: processRuns, EventHub: hub}); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := runOwner.Start(); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
				workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
					result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable}}, nil)
				})
				return hub, processRuns, runOwner, workspaceObserver
			}
			stopProcess := func(hub magicevent.Hub, runOwner *runmodule.Module, workspaceObserver magicevent.SimpleObserver) {
				if workspaceObserver != nil {
					workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
				}
				if runOwner != nil {
					_ = runOwner.Stop(ctx)
				}
				if hub != nil {
					hub.Terminate(ctx)
				}
			}

			hub, processRuns, runOwner, workspaceObserver := startProcess(t)
			baseRepository := runtimeV2StateRepository{hub: hub}
			marker := "failure-target-before-cas"
			if crashAfterCAS {
				marker = "failure-target-after-cas"
			}
			crashRepository := &runtimeV2SaveCrashRepository{
				delegate: baseRepository, after: crashAfterCAS, marker: marker,
				match: func(current runtimev2.RuntimeAggregate) bool {
					target := current.Facts.Resources.FailureTarget
					return target != nil && target.Signature == "failure-target-v1" && len(target.TargetRefs) == 1 && target.TargetRefs[0] == "implementation.go"
				},
			}
			crashingOwner := orchestratorbiz.NewRuntimeOrchestrator()
			if err := crashingOwner.ConfigureRuntimeV2Repository(crashRepository); err != nil {
				t.Fatal(err)
			}
			crashing := newTestBiz(crashingOwner, hub, nil)
			facts := runtimeV2Aggregate(t, 1).Facts
			facts.Failure = &runtimecontract.FailureFact{Signature: "failure-target-v1", TargetRefs: []string{"implementation.go"}}
			aggregate, err := crashing.bizPtr.InitializeRuntimeV2(ctx, facts, runtimeV2Definition(), []runtimev2.RuntimeRequirement{{
				Kind: runtimecontract.PrerequisiteKindFailureTarget, OwnerModuleID: runtimecontract.RuntimeModuleOrchestrator,
				Reason: "typed failure repair surface",
			}})
			if err != nil {
				t.Fatal(err)
			}
			if aggregate.Decision.Action != runtimecontract.RuntimeActionAcquirePrerequisite {
				t.Fatalf("failure-target bootstrap action=%s, aggregate=%+v", aggregate.Decision.Action, aggregate)
			}
			initialRevision := aggregate.StateRevision
			func() {
				defer func() {
					if recovered := recover(); recovered != marker {
						t.Fatalf("unexpected failure-target crash marker: %v", recovered)
					}
				}()
				_, _ = crashing.prepareRuntimeV2Prerequisites(ctx, aggregate)
				t.Fatal("expected failure-target commit crash")
			}()
			if !crashRepository.fired {
				t.Fatal("failure-target CAS crash point was not reached")
			}
			crashed, err := processRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil || crashed.RuntimeState == nil {
				t.Fatalf("failure-target crash state=%+v err=%v", crashed, err)
			}
			if crashAfterCAS {
				if crashed.RuntimeState.Facts.Resources.FailureTarget == nil || crashed.RuntimeState.Facts.Resources.FailureTarget.Signature != "failure-target-v1" {
					t.Fatalf("after-CAS failure target was not persisted: %+v", crashed.RuntimeState)
				}
			} else if crashed.RuntimeState.Facts.Resources.FailureTarget != nil {
				t.Fatalf("before-CAS failure target leaked into persisted aggregate: %+v", crashed.RuntimeState)
			}
			if len(crashed.RuntimeState.Facts.Resources.PrerequisiteObservations) != 0 || len(crashed.RuntimeState.CandidateRequestSessions) != 0 ||
				len(crashed.RuntimeState.CandidateAttempts) != 0 || crashed.RuntimeState.ActiveDispatch != nil || len(crashed.RuntimeState.GovernanceRequests) != 0 ||
				len(crashed.RuntimeState.OperationReceipts) != 0 {
				t.Fatalf("failure-target acquisition entered an external path: %+v", crashed.RuntimeState)
			}
			stopProcess(hub, runOwner, workspaceObserver)

			hub, _, runOwner, workspaceObserver = startProcess(t)
			defer stopProcess(hub, runOwner, workspaceObserver)
			restarted := newRuntimeV2DriveTestModule(t, hub)
			loaded, err := restarted.bizPtr.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			resumed, err := restarted.prepareRuntimeV2Prerequisites(ctx, loaded)
			if err != nil {
				t.Fatal(err)
			}
			if resumed.Facts.Resources.FailureTarget == nil || resumed.Facts.Resources.FailureTarget.Signature != "failure-target-v1" ||
				len(resumed.Facts.Resources.FailureTarget.TargetRefs) != 1 || resumed.Facts.Resources.FailureTarget.TargetRefs[0] != "implementation.go" ||
				len(resumed.Facts.Prerequisites.Pending) != 0 || len(resumed.Facts.Resources.PrerequisiteObservations) != 0 ||
				len(resumed.CandidateRequestSessions) != 0 || len(resumed.CandidateAttempts) != 0 || resumed.ActiveDispatch != nil ||
				len(resumed.GovernanceRequests) != 0 || len(resumed.OperationReceipts) != 0 {
				t.Fatalf("failure-target restart entered unexpected path: %+v", resumed)
			}
			if crashAfterCAS && resumed.StateRevision != crashed.RuntimeState.StateRevision {
				t.Fatalf("after-CAS restart rederived committed local mapping: got revision=%d want=%d", resumed.StateRevision, crashed.RuntimeState.StateRevision)
			}
			if !crashAfterCAS && resumed.StateRevision != initialRevision+1 {
				t.Fatalf("before-CAS restart did not commit exactly one local mapping: got revision=%d want=%d", resumed.StateRevision, initialRevision+1)
			}
		})
	}
}

// A creation selection is a File-owned prerequisite.  The controller-only
// state-repository test proves its local CAS behavior, but it cannot prove
// that a new process re-queries an uncommitted File observation exactly once
// or that it does not query a committed one.  Exercise both boundaries with
// new Run/File/Workspace/EventHub participants and the production File owner
// behind the typed prerequisite contract.
func TestRuntimeV2FileBackedCreationSelectionCommitCrashRestart(t *testing.T) {
	for _, crashAfterCAS := range []bool{false, true} {
		phase := "before_cas_reacquires_file_selection"
		if crashAfterCAS {
			phase = "after_cas_reuses_file_selection"
		}
		t.Run(phase, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			deliverable := filepath.Join(root, "workspace")
			if err := os.MkdirAll(deliverable, 0o755); err != nil {
				t.Fatal(err)
			}
			runsPath := filepath.Join(root, "runs.json")
			bootstrap := runtimeV2GroundedTargetSequenceBootstrap()
			bootstrap.Definition.CapabilityContract = []string{runtimecontract.StepKindFileEdit}
			bootstrap.Requirements = nil
			bootstrap.Focuses[0].TargetSelectorRef = "file/workspace_deliverable"
			bootstrap.Focuses[0].ProspectiveTargetRef = "initial-deliverable.txt"
			if err := filerepository.NewRunRepository(runsPath).Create(ctx, &runmodel.Run{
				ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
				DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, State: common.RunStateActive,
			}); err != nil {
				t.Fatal(err)
			}

			startProcess := func(t *testing.T) (magicevent.Hub, *filerepository.RunRepository, *runmodule.Module, *filemodule.Module, magicevent.SimpleObserver, magicevent.SimpleObserver, *int) {
				t.Helper()
				hub := magicevent.NewHub(16)
				processRuns := filerepository.NewRunRepository(runsPath)
				runOwner := runmodule.New()
				if err := runOwner.Configure(runmodule.Dependencies{Runs: processRuns, EventHub: hub}); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := runOwner.Start(); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
				workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
					result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable}}, nil)
				})
				fileOwner := filemodule.New()
				if err := fileOwner.Configure(filemodule.Dependencies{EventHub: hub}); err != nil {
					workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
					_ = runOwner.Stop(ctx)
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := fileOwner.Start(); err != nil {
					workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
					_ = runOwner.Stop(ctx)
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				selectionCalls := new(int)
				// The spy does not set a result.  File's production Biz remains the
				// sole typed responder; the extra observer only proves whether a new
				// process queried the File owner at all.
				selectionObserver := magicevent.NewSimpleObserverWithMatchID("creation-selection-query-spy", "/runtime/#", hub)
				selectionObserver.Subscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeTargetSelection, func(event magicevent.Event, _ magicevent.Result) {
					if _, ok := event.Data().(orchestratorevents.AcquireRuntimeTargetSelectionCommand); !ok {
						t.Errorf("creation selection spy received %T", event.Data())
						return
					}
					*selectionCalls++
				})
				return hub, processRuns, runOwner, fileOwner, workspaceObserver, selectionObserver, selectionCalls
			}
			stopProcess := func(hub magicevent.Hub, runOwner *runmodule.Module, fileOwner *filemodule.Module, workspaceObserver, selectionObserver magicevent.SimpleObserver) {
				if selectionObserver != nil {
					selectionObserver.Unsubscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeTargetSelection)
				}
				if fileOwner != nil {
					_ = fileOwner.Stop(ctx)
				}
				if workspaceObserver != nil {
					workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
				}
				if runOwner != nil {
					_ = runOwner.Stop(ctx)
				}
				if hub != nil {
					hub.Terminate(ctx)
				}
			}

			hub, processRuns, runOwner, fileOwner, workspaceObserver, selectionObserver, selectionCalls := startProcess(t)
			baseRepository := runtimeV2StateRepository{hub: hub}
			marker := "creation-selection-before-cas"
			if crashAfterCAS {
				marker = "creation-selection-after-cas"
			}
			crashRepository := &runtimeV2SaveCrashRepository{
				delegate: baseRepository, after: crashAfterCAS, marker: marker,
				match: func(current runtimev2.RuntimeAggregate) bool {
					selection := current.Facts.Resources.TargetSelection
					return selection != nil && selection.Disposition == runtimecontract.RuntimeTargetSelectionCreate && selection.ProspectiveTargetRef == "initial-deliverable.txt"
				},
			}
			crashingBiz := orchestratorbiz.NewRuntimeOrchestrator()
			if err := crashingBiz.ConfigureRuntimeV2Repository(crashRepository); err != nil {
				t.Fatal(err)
			}
			crashingModule := newTestBiz(crashingBiz, hub, nil)
			run, err := processRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			aggregate, err := crashingBiz.InitializeRuntimeV2FromRun(ctx, run)
			if err != nil {
				t.Fatal(err)
			}
			func() {
				defer func() {
					if recovered := recover(); recovered != marker {
						t.Fatalf("unexpected creation selection crash marker: %v", recovered)
					}
				}()
				_, _ = crashingModule.prepareRuntimeV2Prerequisites(ctx, aggregate)
				t.Fatal("expected creation selection crash")
			}()
			if !crashRepository.fired || *selectionCalls != 1 {
				t.Fatalf("creation selection crash fired=%t calls=%d", crashRepository.fired, *selectionCalls)
			}
			crashed, err := processRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil || crashed.RuntimeState == nil {
				t.Fatalf("crashed creation selection state=%+v err=%v", crashed, err)
			}
			if crashAfterCAS {
				if crashed.RuntimeState.Facts.Resources.TargetSelection == nil || crashed.RuntimeState.Facts.Resources.TargetSelection.Disposition != runtimecontract.RuntimeTargetSelectionCreate {
					t.Fatalf("after-CAS creation selection was not persisted: %+v", crashed.RuntimeState)
				}
			} else if crashed.RuntimeState.Facts.Resources.TargetSelection != nil {
				t.Fatalf("before-CAS creation selection leaked into persisted state: %+v", crashed.RuntimeState)
			}

			// A real restart owns none of the first process's observers.  Reopen the
			// file-backed Run store and reconstruct every EventHub participant.
			stopProcess(hub, runOwner, fileOwner, workspaceObserver, selectionObserver)
			hub, processRuns, runOwner, fileOwner, workspaceObserver, selectionObserver, restartedSelectionCalls := startProcess(t)
			defer stopProcess(hub, runOwner, fileOwner, workspaceObserver, selectionObserver)
			restartedBiz := orchestratorbiz.NewRuntimeOrchestrator()
			if err := restartedBiz.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
				t.Fatal(err)
			}
			restartedModule := newTestBiz(restartedBiz, hub, nil)
			loaded, err := restartedBiz.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			resumed, err := restartedModule.prepareRuntimeV2Prerequisites(ctx, loaded)
			if err != nil {
				t.Fatal(err)
			}
			if resumed.Facts.Resources.TargetSelection == nil || resumed.Facts.Resources.TargetSelection.Disposition != runtimecontract.RuntimeTargetSelectionCreate ||
				resumed.Facts.Resources.TargetSelection.SelectorRef != "file/workspace_deliverable" || resumed.Facts.Resources.TargetSelection.ProspectiveTargetRef != "initial-deliverable.txt" ||
				len(resumed.Facts.Resources.PrerequisiteObservations) != 2 || len(resumed.CandidateAttempts) != 0 || resumed.ActiveDispatch != nil {
				t.Fatalf("restart creation selection aggregate=%+v", resumed)
			}
			if want := 1; !crashAfterCAS && *restartedSelectionCalls != want {
				t.Fatalf("before-CAS restart File selection calls=%d, want %d", *restartedSelectionCalls, want)
			} else if crashAfterCAS && *restartedSelectionCalls != 0 {
				t.Fatalf("after-CAS restart re-queried committed File selection: calls=%d", *restartedSelectionCalls)
			}
		})
	}
}

// Target snapshots are a separate File-owner prerequisite from selection. A
// File-backed restart may only observe the pending snapshot (and re-read it)
// or the exact committed snapshot; it must never turn an old in-memory read
// into target authority or request a candidate while that fact is unresolved.
func TestRuntimeV2FileBackedTargetSnapshotCommitCrashRestart(t *testing.T) {
	for _, crashAfterCAS := range []bool{false, true} {
		phase := "before_cas_reacquires_file_snapshot"
		if crashAfterCAS {
			phase = "after_cas_reuses_file_snapshot"
		}
		t.Run(phase, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			deliverable := filepath.Join(root, "workspace")
			if err := os.MkdirAll(deliverable, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(deliverable, "target.txt"), []byte("before\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			runsPath := filepath.Join(root, "runs.json")
			bootstrap := runtimeV2Bootstrap()
			bootstrap.Focuses[0].RequireTargetGrounding = true
			bootstrap.Focuses[0].TargetRole = "deliverable"
			if err := filerepository.NewRunRepository(runsPath).Create(ctx, &runmodel.Run{
				ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
				DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, State: common.RunStateActive,
			}); err != nil {
				t.Fatal(err)
			}

			startProcess := func(t *testing.T) (magicevent.Hub, *filerepository.RunRepository, *runmodule.Module, *filemodule.Module, magicevent.SimpleObserver, magicevent.SimpleObserver, *int) {
				t.Helper()
				hub := magicevent.NewHub(16)
				processRuns := filerepository.NewRunRepository(runsPath)
				runOwner := runmodule.New()
				if err := runOwner.Configure(runmodule.Dependencies{Runs: processRuns, EventHub: hub}); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := runOwner.Start(); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
				workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
					result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable}}, nil)
				})
				fileOwner := filemodule.New()
				if err := fileOwner.Configure(filemodule.Dependencies{EventHub: hub}); err != nil {
					workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
					_ = runOwner.Stop(ctx)
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := fileOwner.Start(); err != nil {
					workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
					_ = runOwner.Stop(ctx)
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				snapshotCalls := new(int)
				snapshotObserver := magicevent.NewSimpleObserverWithMatchID("target-snapshot-query-spy", "/runtime/#", hub)
				snapshotObserver.Subscribe(orchestratorevents.TopicOrchestratorAcquireTargetSnapshot, func(event magicevent.Event, _ magicevent.Result) {
					if _, ok := event.Data().(orchestratorevents.AcquireTargetSnapshotCommand); !ok {
						t.Errorf("target snapshot spy received %T", event.Data())
						return
					}
					*snapshotCalls++
				})
				return hub, processRuns, runOwner, fileOwner, workspaceObserver, snapshotObserver, snapshotCalls
			}
			stopProcess := func(hub magicevent.Hub, runOwner *runmodule.Module, fileOwner *filemodule.Module, workspaceObserver, snapshotObserver magicevent.SimpleObserver) {
				if snapshotObserver != nil {
					snapshotObserver.Unsubscribe(orchestratorevents.TopicOrchestratorAcquireTargetSnapshot)
				}
				if fileOwner != nil {
					_ = fileOwner.Stop(ctx)
				}
				if workspaceObserver != nil {
					workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
				}
				if runOwner != nil {
					_ = runOwner.Stop(ctx)
				}
				if hub != nil {
					hub.Terminate(ctx)
				}
			}

			hub, processRuns, runOwner, fileOwner, workspaceObserver, snapshotObserver, snapshotCalls := startProcess(t)
			marker := "target-snapshot-before-cas"
			if crashAfterCAS {
				marker = "target-snapshot-after-cas"
			}
			crashRepository := &runtimeV2SaveCrashRepository{
				delegate: runtimeV2StateRepository{hub: hub}, after: crashAfterCAS, marker: marker,
				match: func(current runtimev2.RuntimeAggregate) bool {
					return len(current.Facts.Resources.TargetSnapshots) == 1 && current.Facts.Resources.TargetSnapshots[0].TargetRef == "target.txt"
				},
			}
			crashingBiz := orchestratorbiz.NewRuntimeOrchestrator()
			if err := crashingBiz.ConfigureRuntimeV2Repository(crashRepository); err != nil {
				t.Fatal(err)
			}
			crashingModule := newTestBiz(crashingBiz, hub, nil)
			run, err := processRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			aggregate, err := crashingBiz.InitializeRuntimeV2FromRun(ctx, run)
			if err != nil {
				t.Fatal(err)
			}
			func() {
				defer func() {
					if recovered := recover(); recovered != marker {
						t.Fatalf("unexpected target snapshot crash marker: %v", recovered)
					}
				}()
				_, _ = crashingModule.prepareRuntimeV2Prerequisites(ctx, aggregate)
				t.Fatal("expected target snapshot crash")
			}()
			if !crashRepository.fired || *snapshotCalls != 1 {
				t.Fatalf("target snapshot crash fired=%t calls=%d", crashRepository.fired, *snapshotCalls)
			}
			crashed, err := processRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil || crashed.RuntimeState == nil {
				t.Fatalf("crashed target snapshot state=%+v err=%v", crashed, err)
			}
			if crashAfterCAS {
				if len(crashed.RuntimeState.Facts.Resources.TargetSnapshots) != 1 || crashed.RuntimeState.Facts.Resources.TargetSnapshots[0].TargetRef != "target.txt" {
					t.Fatalf("after-CAS target snapshot was not persisted: %+v", crashed.RuntimeState)
				}
			} else if len(crashed.RuntimeState.Facts.Resources.TargetSnapshots) != 0 {
				t.Fatalf("before-CAS target snapshot leaked into persisted state: %+v", crashed.RuntimeState)
			}

			stopProcess(hub, runOwner, fileOwner, workspaceObserver, snapshotObserver)
			hub, processRuns, runOwner, fileOwner, workspaceObserver, snapshotObserver, restartedSnapshotCalls := startProcess(t)
			defer stopProcess(hub, runOwner, fileOwner, workspaceObserver, snapshotObserver)
			restartedBiz := orchestratorbiz.NewRuntimeOrchestrator()
			if err := restartedBiz.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
				t.Fatal(err)
			}
			restartedModule := newTestBiz(restartedBiz, hub, nil)
			loaded, err := restartedBiz.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			resumed, err := restartedModule.prepareRuntimeV2Prerequisites(ctx, loaded)
			if err != nil {
				t.Fatal(err)
			}
			if len(resumed.Facts.Resources.TargetSnapshots) != 1 || resumed.Facts.Resources.TargetSnapshots[0].TargetRef != "target.txt" ||
				!resumed.Facts.Resources.TargetSnapshots[0].Complete() || len(resumed.Facts.Resources.PrerequisiteObservations) != 2 ||
				len(resumed.CandidateAttempts) != 0 || resumed.ActiveDispatch != nil {
				t.Fatalf("restart target snapshot aggregate=%+v", resumed)
			}
			if !crashAfterCAS && *restartedSnapshotCalls != 1 {
				t.Fatalf("before-CAS restart File snapshot calls=%d, want 1", *restartedSnapshotCalls)
			} else if crashAfterCAS && *restartedSnapshotCalls != 0 {
				t.Fatalf("after-CAS restart re-queried committed File snapshot: calls=%d", *restartedSnapshotCalls)
			}

			// A delayed duplicate owner response is not a second acquisition.  Model
			// it with the real File-owner query after the fresh process has already
			// proved that the committed snapshot requires no query.  SM-2 must reject
			// the replay before CAS and leave every persisted fact/ledger untouched.
			var committedPrerequisite runtimecontract.RuntimePrerequisite
			for _, prerequisite := range resumed.Facts.Prerequisites.Satisfied {
				if prerequisite.Kind == runtimecontract.PrerequisiteKindTargetSnapshot && prerequisite.SubjectRef == "target.txt" {
					committedPrerequisite = prerequisite
					break
				}
			}
			if committedPrerequisite.ID == "" {
				t.Fatalf("restart did not retain satisfied target snapshot prerequisite: %+v", resumed.Facts.Prerequisites)
			}
			duplicateCallsBefore := *restartedSnapshotCalls
			duplicateResponse, err := AcquireTargetSnapshotPrerequisite(ctx, hub, orchestratorevents.AcquireTargetSnapshotCommand{
				WorkspaceID: "ws-v2", RunID: "run-v2", AuthoritySnapshotID: resumed.Decision.AuthoritySnapshot.ID,
				PrerequisiteID: committedPrerequisite.ID, WorkspaceDeliverableDirectory: deliverable,
				TargetRef: "target.txt", MaxBytes: runtimeV2DefaultTargetSnapshotBytes,
			})
			if err != nil {
				t.Fatalf("delayed duplicate File target snapshot query: %v", err)
			}
			if *restartedSnapshotCalls != duplicateCallsBefore+1 {
				t.Fatalf("delayed duplicate File target snapshot calls=%d, want %d", *restartedSnapshotCalls, duplicateCallsBefore+1)
			}
			if _, err := restartedBiz.CommitRuntimeV2Prerequisite(ctx, "ws-v2", "run-v2", runtimev2.PrerequisiteResult{
				PrerequisiteID: duplicateResponse.PrerequisiteID, TargetSnapshot: &duplicateResponse.Snapshot, Observation: duplicateResponse.Observation,
			}); err == nil {
				t.Fatal("delayed duplicate File target snapshot unexpectedly mutated committed aggregate")
			}
			final, err := restartedBiz.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			if final.StateRevision != resumed.StateRevision || len(final.Facts.Resources.TargetSnapshots) != 1 ||
				final.Facts.Resources.TargetSnapshots[0] != resumed.Facts.Resources.TargetSnapshots[0] ||
				len(final.Facts.Resources.PrerequisiteObservations) != len(resumed.Facts.Resources.PrerequisiteObservations) ||
				len(final.PrerequisiteInvalidations) != len(resumed.PrerequisiteInvalidations) ||
				final.Facts.Recovery != resumed.Facts.Recovery || len(final.CandidateAttempts) != 0 || final.ActiveDispatch != nil {
				t.Fatalf("delayed duplicate File target snapshot changed persisted aggregate: before=%+v after=%+v", resumed, final)
			}

			// A typed read-only stale receipt must retire just this File snapshot.
			// Persist a request-session marker to exercise the session-first stale
			// route (without allocating an attempt), then reconstruct a third process
			// and prove the new File owner read is the sole replacement authority.
			if err := os.WriteFile(filepath.Join(deliverable, "target.txt"), []byte("after\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			candidateContext := runtimecontract.RuntimeCandidateContextForDecision(final.Facts, final.Decision, nil)
			manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(final.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(candidateContext.Resources))
			if err != nil {
				t.Fatal(err)
			}
			prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(final.Definition, final.Facts.Activation.ExecutionContract)
			_, session, err := restartedBiz.EnsureRuntimeV2CandidateRequestSession(ctx, "ws-v2", "run-v2", prefix, manifest.Hash, "")
			if err != nil {
				t.Fatal(err)
			}
			physicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, session.NextSequence)
			if _, err := restartedBiz.MarkRuntimeV2CandidateRequestSubmitted(ctx, "ws-v2", "run-v2", session.LogicalRequestID, session.NextSequence, physicalCallID); err != nil {
				t.Fatal(err)
			}
			var snapshotObservation runtimecontract.RuntimePrerequisiteObservation
			for _, observation := range final.Facts.Resources.PrerequisiteObservations {
				if observation.PrerequisiteID == committedPrerequisite.ID {
					snapshotObservation = observation
					break
				}
			}
			if snapshotObservation.ObservationID == "" {
				t.Fatalf("target snapshot observation missing from persisted aggregate: %+v", final.Facts.Resources.PrerequisiteObservations)
			}
			stale := runtimecontract.RuntimePrerequisiteStale{
				PrerequisiteID: committedPrerequisite.ID, ObservationID: snapshotObservation.ObservationID,
				Domain: runtimecontract.RuntimeResourceWorkspaceTarget, OwnerModuleID: runtimecontract.RuntimeModuleFile,
				SubjectRef: "target.txt", ExpectedRevision: final.Facts.Resources.TargetSnapshots[0].ContentHash,
				ObservedRevision: "target-v2", Reason: "File owner observed a newer target revision",
			}
			invalidated, err := restartedBiz.InvalidateRuntimeV2CandidateRequestSessionForPrerequisiteStale(ctx, "ws-v2", "run-v2", session.LogicalRequestID, stale)
			if err != nil {
				t.Fatal(err)
			}
			if invalidated.Decision.Action != runtimecontract.RuntimeActionAcquirePrerequisite || len(invalidated.Facts.Resources.TargetSnapshots) != 0 ||
				len(invalidated.Facts.Resources.PrerequisiteObservations) != 1 || len(invalidated.Facts.Prerequisites.Pending) != 1 ||
				invalidated.Facts.Prerequisites.Pending[0].ID != committedPrerequisite.ID || invalidated.Facts.Recovery.PrerequisiteRefreshes != 1 ||
				invalidated.Facts.Recovery.PrerequisiteResourceChangeRefreshes != 0 || len(invalidated.CandidateAttempts) != 0 || invalidated.ActiveDispatch != nil ||
				len(invalidated.CandidateRequestSessions) != 1 || invalidated.CandidateRequestSessions[0].State != runtimecontract.RuntimeCandidateRequestSessionTerminal ||
				invalidated.CandidateRequestSessions[0].LastOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomePrerequisiteStale {
				t.Fatalf("target snapshot stale did not isolate owner fact: %+v", invalidated)
			}

			stopProcess(hub, runOwner, fileOwner, workspaceObserver, snapshotObserver)
			hub, processRuns, runOwner, fileOwner, workspaceObserver, snapshotObserver, staleReacquireCalls := startProcess(t)
			defer stopProcess(hub, runOwner, fileOwner, workspaceObserver, snapshotObserver)
			thirdBiz := orchestratorbiz.NewRuntimeOrchestrator()
			if err := thirdBiz.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
				t.Fatal(err)
			}
			thirdModule := newTestBiz(thirdBiz, hub, nil)
			loadedAfterStale, err := thirdBiz.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			reacquired, err := thirdModule.prepareRuntimeV2Prerequisites(ctx, loadedAfterStale)
			if err != nil {
				t.Fatal(err)
			}
			if *staleReacquireCalls != 1 || len(reacquired.Facts.Resources.TargetSnapshots) != 1 ||
				reacquired.Facts.Resources.TargetSnapshots[0].Content != "after\n" || len(reacquired.Facts.Resources.PrerequisiteObservations) != 2 ||
				reacquired.Facts.Recovery.PrerequisiteRefreshes != 1 ||
				reacquired.Facts.Recovery.PrerequisiteResourceChangeRefreshes != 0 ||
				len(reacquired.CandidateAttempts) != 0 || reacquired.ActiveDispatch != nil {
				t.Fatalf("fresh-process target snapshot stale reacquisition=%+v calls=%d", reacquired, *staleReacquireCalls)
			}
			if _, err := processRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// File operation admissibility is an owner observation, not an implicit
// Orchestrator allow-list. Its receipt must therefore have the same Run-CAS
// recovery boundary as manifest, selection, and snapshot facts.
func TestRuntimeV2FileBackedFileAdmissibilityCommitCrashRestart(t *testing.T) {
	for _, crashAfterCAS := range []bool{false, true} {
		phase := "before_cas_reacquires_file_inventory"
		if crashAfterCAS {
			phase = "after_cas_reuses_file_inventory"
		}
		t.Run(phase, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			deliverable := filepath.Join(root, "workspace")
			if err := os.MkdirAll(deliverable, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(deliverable, "target.txt"), []byte("before\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			runsPath := filepath.Join(root, "runs.json")
			bootstrap := runtimeV2Bootstrap()
			bootstrap.Requirements = []runtimecontract.RuntimeRequirement{
				{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleFile,
					CapabilityClass: runtimecontract.RuntimeCapabilityClassStateChange, RequiredCapabilities: []string{runtimecontract.StepKindFileEdit}, Reason: "File capability"},
				{Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleFile,
					RequiredCapabilities: []string{runtimecontract.StepKindFileEdit}, Reason: "File owner inventory"},
			}
			if err := filerepository.NewRunRepository(runsPath).Create(ctx, &runmodel.Run{
				ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
				DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, State: common.RunStateActive,
			}); err != nil {
				t.Fatal(err)
			}

			startProcess := func(t *testing.T) (magicevent.Hub, *filerepository.RunRepository, *runmodule.Module, *filemodule.Module, magicevent.SimpleObserver, magicevent.SimpleObserver, *int) {
				t.Helper()
				hub := magicevent.NewHub(16)
				processRuns := filerepository.NewRunRepository(runsPath)
				runOwner := runmodule.New()
				if err := runOwner.Configure(runmodule.Dependencies{Runs: processRuns, EventHub: hub}); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := runOwner.Start(); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
				workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
					result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable}}, nil)
				})
				fileOwner := filemodule.New()
				if err := fileOwner.Configure(filemodule.Dependencies{EventHub: hub}); err != nil {
					workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
					_ = runOwner.Stop(ctx)
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := fileOwner.Start(); err != nil {
					workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
					_ = runOwner.Stop(ctx)
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				inventoryCalls := new(int)
				// Admissibility uses the canonical owner module ID as its EventHub
				// destination (unlike /runtime/prerequisite snapshot commands). Use
				// a distinct matching pattern so this observer cannot replace File
				// Biz's real `blocks/file` responder through observer-ID de-dup.
				inventoryObserver := magicevent.NewSimpleObserverWithMatchID("file-admissibility-query-spy", "blocks/#", hub)
				inventoryObserver.Subscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeOperationAdmissibility, func(event magicevent.Event, _ magicevent.Result) {
					if _, ok := event.Data().(orchestratorevents.AcquireRuntimeOperationAdmissibilityCommand); !ok {
						t.Errorf("File admissibility spy received %T", event.Data())
						return
					}
					*inventoryCalls++
				})
				return hub, processRuns, runOwner, fileOwner, workspaceObserver, inventoryObserver, inventoryCalls
			}
			stopProcess := func(hub magicevent.Hub, runOwner *runmodule.Module, fileOwner *filemodule.Module, workspaceObserver, inventoryObserver magicevent.SimpleObserver) {
				if inventoryObserver != nil {
					inventoryObserver.Unsubscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeOperationAdmissibility)
				}
				if fileOwner != nil {
					_ = fileOwner.Stop(ctx)
				}
				if workspaceObserver != nil {
					workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
				}
				if runOwner != nil {
					_ = runOwner.Stop(ctx)
				}
				if hub != nil {
					hub.Terminate(ctx)
				}
			}

			hub, processRuns, runOwner, fileOwner, workspaceObserver, inventoryObserver, inventoryCalls := startProcess(t)
			marker := "file-admissibility-before-cas"
			if crashAfterCAS {
				marker = "file-admissibility-after-cas"
			}
			crashRepository := &runtimeV2SaveCrashRepository{
				delegate: runtimeV2StateRepository{hub: hub}, after: crashAfterCAS, marker: marker,
				match: func(current runtimev2.RuntimeAggregate) bool {
					return len(current.Facts.Resources.OperationAdmissibilities) == 1 && current.Facts.Resources.OperationAdmissibilities[0].OwnerModuleID == runtimecontract.RuntimeModuleFile
				},
			}
			crashingBiz := orchestratorbiz.NewRuntimeOrchestrator()
			if err := crashingBiz.ConfigureRuntimeV2Repository(crashRepository); err != nil {
				t.Fatal(err)
			}
			crashingModule := newTestBiz(crashingBiz, hub, nil)
			run, err := processRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			aggregate, err := crashingBiz.InitializeRuntimeV2FromRun(ctx, run)
			if err != nil {
				t.Fatal(err)
			}
			func() {
				defer func() {
					if recovered := recover(); recovered != marker {
						t.Fatalf("unexpected File admissibility crash marker: %v", recovered)
					}
				}()
				_, _ = crashingModule.prepareRuntimeV2Prerequisites(ctx, aggregate)
				t.Fatal("expected File admissibility crash")
			}()
			if !crashRepository.fired || *inventoryCalls != 1 {
				t.Fatalf("File admissibility crash fired=%t calls=%d", crashRepository.fired, *inventoryCalls)
			}
			crashed, err := processRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil || crashed.RuntimeState == nil {
				t.Fatalf("crashed File admissibility state=%+v err=%v", crashed, err)
			}
			if crashAfterCAS {
				if len(crashed.RuntimeState.Facts.Resources.OperationAdmissibilities) != 1 || crashed.RuntimeState.Facts.Resources.OperationAdmissibilities[0].OwnerModuleID != runtimecontract.RuntimeModuleFile ||
					len(crashed.RuntimeState.Facts.Resources.PrerequisiteObservations) != 2 {
					t.Fatalf("after-CAS File inventory was not persisted: %+v", crashed.RuntimeState)
				}
			} else if len(crashed.RuntimeState.Facts.Resources.OperationAdmissibilities) != 0 || len(crashed.RuntimeState.Facts.Resources.PrerequisiteObservations) != 1 {
				t.Fatalf("before-CAS File inventory leaked into persisted state: %+v", crashed.RuntimeState)
			}

			stopProcess(hub, runOwner, fileOwner, workspaceObserver, inventoryObserver)
			hub, processRuns, runOwner, fileOwner, workspaceObserver, inventoryObserver, restartedInventoryCalls := startProcess(t)
			defer stopProcess(hub, runOwner, fileOwner, workspaceObserver, inventoryObserver)
			restartedBiz := orchestratorbiz.NewRuntimeOrchestrator()
			if err := restartedBiz.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
				t.Fatal(err)
			}
			restartedModule := newTestBiz(restartedBiz, hub, nil)
			loaded, err := restartedBiz.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			resumed, err := restartedModule.prepareRuntimeV2Prerequisites(ctx, loaded)
			if err != nil {
				t.Fatal(err)
			}
			if len(resumed.Facts.Resources.OperationAdmissibilities) != 1 || resumed.Facts.Resources.OperationAdmissibilities[0].OwnerModuleID != runtimecontract.RuntimeModuleFile ||
				resumed.Facts.Resources.OperationAdmissibilities[0].SnapshotID == "" || resumed.Facts.Resources.OperationAdmissibilities[0].ObservedRevision == "" ||
				len(resumed.Facts.Resources.PrerequisiteObservations) != 2 || len(resumed.CandidateRequestSessions) != 0 || len(resumed.CandidateAttempts) != 0 ||
				resumed.ActiveDispatch != nil || len(resumed.CompletedDispatches) != 0 || len(resumed.GovernanceRequests) != 0 || len(resumed.OperationReceipts) != 0 ||
				len(resumed.Facts.Evidence.Accepted) != 0 || len(resumed.Facts.Evidence.Rejected) != 0 {
				t.Fatalf("restart File admissibility aggregate=%+v", resumed)
			}
			if !crashAfterCAS && *restartedInventoryCalls != 1 {
				t.Fatalf("before-CAS restart File inventory calls=%d, want 1", *restartedInventoryCalls)
			} else if crashAfterCAS && *restartedInventoryCalls != 0 {
				t.Fatalf("after-CAS restart re-queried committed File inventory: calls=%d", *restartedInventoryCalls)
			}
			if crashAfterCAS {
				persistedInventory := crashed.RuntimeState.Facts.Resources.OperationAdmissibilities[0]
				persistedObservation, found := runtimecontract.RuntimePrerequisiteObservationForID(crashed.RuntimeState.Facts.Resources.PrerequisiteObservations, persistedInventory.PrerequisiteID)
				if !found {
					t.Fatalf("after-CAS File inventory observation is missing: %+v", crashed.RuntimeState.Facts.Resources.PrerequisiteObservations)
				}
				resumedInventory := resumed.Facts.Resources.OperationAdmissibilities[0]
				resumedObservation, found := runtimecontract.RuntimePrerequisiteObservationForID(resumed.Facts.Resources.PrerequisiteObservations, resumedInventory.PrerequisiteID)
				if !found {
					t.Fatalf("restarted File inventory observation is missing: %+v", resumed.Facts.Resources.PrerequisiteObservations)
				}
				if resumedInventory.SnapshotID != persistedInventory.SnapshotID || resumedInventory.ObservedRevision != persistedInventory.ObservedRevision ||
					resumedInventory.AcquisitionBinding != persistedInventory.AcquisitionBinding || resumedObservation.ObservationID != persistedObservation.ObservationID ||
					resumedObservation.ContentHash != persistedObservation.ContentHash {
					t.Fatalf("after-CAS restart did not reuse the exact File owner inventory: persisted=%+v resumed=%+v", persistedInventory, resumedInventory)
				}
			}

			// A delayed inventory response cannot become a second admissibility
			// grant. The real File owner may answer, but SM-2 must reject the
			// already-satisfied prerequisite without a CAS mutation.
			inventory := resumed.Facts.Resources.OperationAdmissibilities[0]
			var committedPrerequisite runtimecontract.RuntimePrerequisite
			for _, prerequisite := range resumed.Facts.Prerequisites.Satisfied {
				if prerequisite.ID == inventory.PrerequisiteID {
					committedPrerequisite = prerequisite
					break
				}
			}
			if committedPrerequisite.ID == "" {
				t.Fatalf("restart did not retain satisfied File inventory prerequisite: %+v", resumed.Facts.Prerequisites)
			}
			duplicateCallsBefore := *restartedInventoryCalls
			duplicateResponse, err := AcquireRuntimeOperationAdmissibilityPrerequisite(ctx, hub, orchestratorevents.AcquireRuntimeOperationAdmissibilityCommand{
				WorkspaceID: "ws-v2", RunID: "run-v2", AuthoritySnapshotID: resumed.Decision.AuthoritySnapshot.ID,
				PrerequisiteID: committedPrerequisite.ID, OwnerModuleID: runtimecontract.RuntimeModuleFile,
				WorkspaceDeliverableDirectory: deliverable, RequiredKinds: committedPrerequisite.RequiredCapabilities,
				RequiredTargetRefs: runtimev2.GroundedRepairTargets(resumed.Facts), MaxItems: runtimecontract.DefaultRuntimeOwnerInventoryBudget().MaxItems,
			})
			if err != nil {
				t.Fatalf("delayed duplicate File inventory query: %v", err)
			}
			if *restartedInventoryCalls != duplicateCallsBefore+1 {
				t.Fatalf("delayed duplicate File inventory calls=%d, want %d", *restartedInventoryCalls, duplicateCallsBefore+1)
			}
			if _, err := restartedBiz.CommitRuntimeV2Prerequisite(ctx, "ws-v2", "run-v2", runtimev2.PrerequisiteResult{
				PrerequisiteID: duplicateResponse.PrerequisiteID, OperationAdmissibility: &duplicateResponse.Admissibility, Observation: duplicateResponse.Observation,
			}); err == nil {
				t.Fatal("delayed duplicate File inventory unexpectedly mutated committed aggregate")
			}
			final, err := restartedBiz.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			if final.StateRevision != resumed.StateRevision || len(final.Facts.Resources.OperationAdmissibilities) != 1 ||
				final.Facts.Resources.OperationAdmissibilities[0].SnapshotID != inventory.SnapshotID ||
				len(final.Facts.Resources.PrerequisiteObservations) != len(resumed.Facts.Resources.PrerequisiteObservations) ||
				len(final.PrerequisiteInvalidations) != len(resumed.PrerequisiteInvalidations) || final.Facts.Recovery != resumed.Facts.Recovery ||
				len(final.CandidateRequestSessions) != 0 || len(final.CandidateAttempts) != 0 || final.ActiveDispatch != nil {
				t.Fatalf("delayed duplicate File inventory changed persisted aggregate: before=%+v after=%+v", resumed, final)
			}

			// A File dependency revision that changes after the owner observation is
			// a read-only stale path. It must retire this inventory alone and, after
			// a fresh process rebuild, obtain exactly one replacement from File.
			if err := os.WriteFile(filepath.Join(deliverable, "target.txt"), []byte("after\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			candidateContext := runtimecontract.RuntimeCandidateContextForDecision(final.Facts, final.Decision, nil)
			manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(final.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(candidateContext.Resources))
			if err != nil {
				t.Fatal(err)
			}
			prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(final.Definition, final.Facts.Activation.ExecutionContract)
			_, session, err := restartedBiz.EnsureRuntimeV2CandidateRequestSession(ctx, "ws-v2", "run-v2", prefix, manifest.Hash, "")
			if err != nil {
				t.Fatal(err)
			}
			physicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, session.NextSequence)
			if _, err := restartedBiz.MarkRuntimeV2CandidateRequestSubmitted(ctx, "ws-v2", "run-v2", session.LogicalRequestID, session.NextSequence, physicalCallID); err != nil {
				t.Fatal(err)
			}
			inventoryObservation, found := runtimecontract.RuntimePrerequisiteObservationForID(final.Facts.Resources.PrerequisiteObservations, inventory.PrerequisiteID)
			if !found || len(inventoryObservation.Dependencies) == 0 {
				t.Fatalf("File inventory observation has no freshness dependency: %+v", final.Facts.Resources.PrerequisiteObservations)
			}
			dependency := inventoryObservation.Dependencies[0].Normalized()
			stale := runtimecontract.RuntimePrerequisiteStale{
				PrerequisiteID: inventory.PrerequisiteID, ObservationID: inventoryObservation.ObservationID,
				Domain: dependency.Domain, OwnerModuleID: dependency.OwnerModuleID, SubjectRef: dependency.SubjectRef,
				ExpectedRevision: dependency.Revision, ObservedRevision: "file-inventory-v2", Reason: "File owner observed a newer inventory dependency",
			}
			invalidated, err := restartedBiz.InvalidateRuntimeV2CandidateRequestSessionForPrerequisiteStale(ctx, "ws-v2", "run-v2", session.LogicalRequestID, stale)
			if err != nil {
				t.Fatal(err)
			}
			if invalidated.Decision.Action != runtimecontract.RuntimeActionAcquirePrerequisite || len(invalidated.Facts.Resources.OperationAdmissibilities) != 0 ||
				len(invalidated.Facts.Resources.PrerequisiteObservations) != 1 || len(invalidated.Facts.Prerequisites.Pending) != 1 ||
				invalidated.Facts.Prerequisites.Pending[0].ID != inventory.PrerequisiteID || invalidated.Facts.Recovery.PrerequisiteRefreshes != 1 ||
				invalidated.Facts.Recovery.PrerequisiteResourceChangeRefreshes != 0 || len(invalidated.CandidateAttempts) != 0 || invalidated.ActiveDispatch != nil ||
				len(invalidated.CandidateRequestSessions) != 1 || invalidated.CandidateRequestSessions[0].State != runtimecontract.RuntimeCandidateRequestSessionTerminal ||
				invalidated.CandidateRequestSessions[0].LastOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomePrerequisiteStale {
				t.Fatalf("File inventory stale did not isolate owner fact: %+v", invalidated)
			}

			stopProcess(hub, runOwner, fileOwner, workspaceObserver, inventoryObserver)
			hub, processRuns, runOwner, fileOwner, workspaceObserver, inventoryObserver, staleReacquireCalls := startProcess(t)
			defer stopProcess(hub, runOwner, fileOwner, workspaceObserver, inventoryObserver)
			thirdBiz := orchestratorbiz.NewRuntimeOrchestrator()
			if err := thirdBiz.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
				t.Fatal(err)
			}
			thirdModule := newTestBiz(thirdBiz, hub, nil)
			loadedAfterStale, err := thirdBiz.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			reacquired, err := thirdModule.prepareRuntimeV2Prerequisites(ctx, loadedAfterStale)
			if err != nil {
				t.Fatal(err)
			}
			if *staleReacquireCalls != 1 || len(reacquired.Facts.Resources.OperationAdmissibilities) != 1 ||
				reacquired.Facts.Resources.OperationAdmissibilities[0].ObservedRevision == inventory.ObservedRevision ||
				len(reacquired.Facts.Resources.PrerequisiteObservations) != 2 || reacquired.Facts.Recovery.PrerequisiteRefreshes != 1 ||
				reacquired.Facts.Recovery.PrerequisiteResourceChangeRefreshes != 0 || len(reacquired.CandidateAttempts) != 0 || reacquired.ActiveDispatch != nil {
				t.Fatalf("fresh-process File inventory stale reacquisition=%+v calls=%d", reacquired, *staleReacquireCalls)
			}
			if _, err := processRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRuntimeV2FileBackedGitAdmissibilityCommitCrashRestart(t *testing.T) {
	for _, crashAfterCAS := range []bool{false, true} {
		phase := "before_cas_reacquires_git_inventory"
		if crashAfterCAS {
			phase = "after_cas_reuses_git_inventory"
		}
		t.Run(phase, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			deliverable, internalDir := filepath.Join(root, "workspace"), filepath.Join(root, "internal")
			if err := os.MkdirAll(deliverable, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(internalDir, 0o755); err != nil {
				t.Fatal(err)
			}
			writeRuntimeV2GitFixtureFile(t, deliverable)
			runsPath := filepath.Join(root, "runs.json")
			receiptRoot := filepath.Join(root, "git-owner-data")
			bootstrap := runtimeV2GitCapabilityStateBootstrap()
			bootstrap.Requirements = append(bootstrap.Requirements, runtimecontract.RuntimeRequirement{
				Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleGit,
				RequiredCapabilities: []string{runtimecontract.StepKindGitWrite}, Reason: "git owner inventory",
			})
			if err := filerepository.NewRunRepository(runsPath).Create(ctx, &runmodel.Run{
				ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
				DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, State: common.RunStateActive,
			}); err != nil {
				t.Fatal(err)
			}

			startProcess := func(t *testing.T) (magicevent.Hub, *filerepository.RunRepository, *runmodule.Module, *filemodule.Module, *gitmodule.Module, magicevent.SimpleObserver, magicevent.SimpleObserver, *int) {
				t.Helper()
				hub := magicevent.NewHub(32)
				processRuns := filerepository.NewRunRepository(runsPath)
				runOwner := runmodule.New()
				if err := runOwner.Configure(runmodule.Dependencies{Runs: processRuns, EventHub: hub}); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := runOwner.Start(); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
				workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
					result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
				})
				fileOwner := filemodule.New()
				if err := fileOwner.Configure(filemodule.Dependencies{EventHub: hub}); err != nil {
					workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
					_ = runOwner.Stop(ctx)
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := fileOwner.Start(); err != nil {
					workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
					_ = runOwner.Stop(ctx)
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				receipts, err := gitbiz.NewCapabilityStateReceiptStore(receiptRoot)
				if err != nil {
					_ = fileOwner.Stop(ctx)
					workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
					_ = runOwner.Stop(ctx)
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				gitOwner := gitmodule.New()
				if err := gitOwner.Configure(gitmodule.Dependencies{EventHub: hub, RuntimeAdapter: gitbiz.NewRuntimeAdapter(true), CapabilityStateReceipts: receipts}); err != nil {
					_ = fileOwner.Stop(ctx)
					workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
					_ = runOwner.Stop(ctx)
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := gitOwner.Start(); err != nil {
					_ = fileOwner.Stop(ctx)
					workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
					_ = runOwner.Stop(ctx)
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				inventoryCalls := new(int)
				inventoryObserver := magicevent.NewSimpleObserverWithMatchID("git-admissibility-query-spy", "blocks/#", hub)
				inventoryObserver.Subscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeOperationAdmissibility, func(event magicevent.Event, _ magicevent.Result) {
					command, ok := event.Data().(orchestratorevents.AcquireRuntimeOperationAdmissibilityCommand)
					if !ok || command.OwnerModuleID != runtimecontract.RuntimeModuleGit {
						t.Errorf("Git admissibility spy received %#v", event.Data())
						return
					}
					*inventoryCalls++
				})
				return hub, processRuns, runOwner, fileOwner, gitOwner, workspaceObserver, inventoryObserver, inventoryCalls
			}
			stopProcess := func(hub magicevent.Hub, runOwner *runmodule.Module, fileOwner *filemodule.Module, gitOwner *gitmodule.Module, workspaceObserver, inventoryObserver magicevent.SimpleObserver) {
				if inventoryObserver != nil {
					inventoryObserver.Unsubscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeOperationAdmissibility)
				}
				if gitOwner != nil {
					_ = gitOwner.Stop(ctx)
				}
				if fileOwner != nil {
					_ = fileOwner.Stop(ctx)
				}
				if workspaceObserver != nil {
					workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
				}
				if runOwner != nil {
					_ = runOwner.Stop(ctx)
				}
				if hub != nil {
					hub.Terminate(ctx)
				}
			}

			hub, processRuns, runOwner, fileOwner, gitOwner, workspaceObserver, inventoryObserver, inventoryCalls := startProcess(t)
			marker := "git-admissibility-before-cas"
			if crashAfterCAS {
				marker = "git-admissibility-after-cas"
			}
			crashRepository := &runtimeV2SaveCrashRepository{
				delegate: runtimeV2StateRepository{hub: hub}, after: crashAfterCAS, marker: marker,
				match: func(current runtimev2.RuntimeAggregate) bool {
					return len(current.Facts.Resources.OperationAdmissibilities) == 1 && current.Facts.Resources.OperationAdmissibilities[0].OwnerModuleID == runtimecontract.RuntimeModuleGit
				},
			}
			crashingBiz := orchestratorbiz.NewRuntimeOrchestrator()
			if err := crashingBiz.ConfigureRuntimeV2Repository(crashRepository); err != nil {
				t.Fatal(err)
			}
			crashingModule := newTestBiz(crashingBiz, hub, nil)
			run, err := processRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			aggregate, err := crashingBiz.InitializeRuntimeV2FromRun(ctx, run)
			if err != nil {
				t.Fatal(err)
			}
			func() {
				defer func() {
					if recovered := recover(); recovered != marker {
						t.Fatalf("unexpected Git admissibility crash marker: %v", recovered)
					}
				}()
				_, _ = crashingModule.prepareRuntimeV2Prerequisites(ctx, aggregate)
				t.Fatal("expected Git admissibility crash")
			}()
			if !crashRepository.fired || *inventoryCalls != 1 {
				t.Fatalf("Git admissibility crash fired=%t calls=%d", crashRepository.fired, *inventoryCalls)
			}
			crashed, err := processRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil || crashed.RuntimeState == nil {
				t.Fatalf("crashed Git admissibility state=%+v err=%v", crashed, err)
			}
			if crashAfterCAS {
				if len(crashed.RuntimeState.Facts.Resources.OperationAdmissibilities) != 1 || len(crashed.RuntimeState.Facts.Resources.CapabilityStates) != 1 ||
					len(crashed.RuntimeState.Facts.Resources.PrerequisiteObservations) != 3 {
					t.Fatalf("after-CAS Git inventory was not persisted: %+v", crashed.RuntimeState)
				}
			} else if len(crashed.RuntimeState.Facts.Resources.OperationAdmissibilities) != 0 || len(crashed.RuntimeState.Facts.Resources.CapabilityStates) != 1 ||
				len(crashed.RuntimeState.Facts.Resources.PrerequisiteObservations) != 2 {
				t.Fatalf("before-CAS Git inventory leaked into persisted state: %+v", crashed.RuntimeState)
			}

			stopProcess(hub, runOwner, fileOwner, gitOwner, workspaceObserver, inventoryObserver)
			hub, processRuns, runOwner, fileOwner, gitOwner, workspaceObserver, inventoryObserver, restartedInventoryCalls := startProcess(t)
			defer stopProcess(hub, runOwner, fileOwner, gitOwner, workspaceObserver, inventoryObserver)
			restartedBiz := orchestratorbiz.NewRuntimeOrchestrator()
			if err := restartedBiz.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
				t.Fatal(err)
			}
			restartedModule := newTestBiz(restartedBiz, hub, nil)
			loaded, err := restartedBiz.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			resumed, err := restartedModule.prepareRuntimeV2Prerequisites(ctx, loaded)
			if err != nil {
				t.Fatal(err)
			}
			if len(resumed.Facts.Resources.OperationAdmissibilities) != 1 || len(resumed.Facts.Resources.CapabilityStates) != 1 ||
				resumed.Facts.Resources.OperationAdmissibilities[0].OwnerModuleID != runtimecontract.RuntimeModuleGit ||
				resumed.Facts.Resources.OperationAdmissibilities[0].AcquisitionBinding == "" ||
				resumed.Facts.Resources.OperationAdmissibilities[0].AcquisitionBinding != resumed.Facts.Resources.CapabilityStates[0].AcquisitionBinding ||
				len(resumed.Facts.Resources.PrerequisiteObservations) != 3 || len(resumed.CandidateRequestSessions) != 0 || len(resumed.CandidateAttempts) != 0 ||
				resumed.ActiveDispatch != nil || len(resumed.CompletedDispatches) != 0 || len(resumed.GovernanceRequests) != 0 || len(resumed.OperationReceipts) != 0 ||
				len(resumed.Facts.Evidence.Accepted) != 0 || len(resumed.Facts.Evidence.Rejected) != 0 {
				t.Fatalf("restart Git admissibility aggregate=%+v", resumed)
			}
			if !crashAfterCAS && *restartedInventoryCalls != 1 {
				t.Fatalf("before-CAS restart Git inventory calls=%d, want 1", *restartedInventoryCalls)
			} else if crashAfterCAS && *restartedInventoryCalls != 0 {
				t.Fatalf("after-CAS restart re-queried committed Git inventory: calls=%d", *restartedInventoryCalls)
			}
			if crashAfterCAS {
				persistedInventory := crashed.RuntimeState.Facts.Resources.OperationAdmissibilities[0]
				persistedObservation, found := runtimecontract.RuntimePrerequisiteObservationForID(crashed.RuntimeState.Facts.Resources.PrerequisiteObservations, persistedInventory.PrerequisiteID)
				if !found {
					t.Fatalf("after-CAS Git inventory observation is missing: %+v", crashed.RuntimeState.Facts.Resources.PrerequisiteObservations)
				}
				resumedInventory := resumed.Facts.Resources.OperationAdmissibilities[0]
				resumedObservation, found := runtimecontract.RuntimePrerequisiteObservationForID(resumed.Facts.Resources.PrerequisiteObservations, resumedInventory.PrerequisiteID)
				if !found || resumedInventory.SnapshotID != persistedInventory.SnapshotID || resumedInventory.ObservedRevision != persistedInventory.ObservedRevision ||
					resumedInventory.AcquisitionBinding != persistedInventory.AcquisitionBinding || resumedObservation.ObservationID != persistedObservation.ObservationID ||
					resumedObservation.ContentHash != persistedObservation.ContentHash {
					t.Fatalf("after-CAS restart did not reuse exact Git inventory: persisted=%+v resumed=%+v", persistedInventory, resumedInventory)
				}
			}

			inventory := resumed.Facts.Resources.OperationAdmissibilities[0]
			var committedPrerequisite runtimecontract.RuntimePrerequisite
			for _, prerequisite := range resumed.Facts.Prerequisites.Satisfied {
				if prerequisite.ID == inventory.PrerequisiteID {
					committedPrerequisite = prerequisite
					break
				}
			}
			if committedPrerequisite.ID == "" {
				t.Fatalf("restart did not retain satisfied Git inventory prerequisite: %+v", resumed.Facts.Prerequisites)
			}
			duplicateCallsBefore := *restartedInventoryCalls
			duplicateResponse, err := AcquireRuntimeOperationAdmissibilityPrerequisite(ctx, hub, orchestratorevents.AcquireRuntimeOperationAdmissibilityCommand{
				WorkspaceID: "ws-v2", RunID: "run-v2", AuthoritySnapshotID: resumed.Decision.AuthoritySnapshot.ID,
				PrerequisiteID: committedPrerequisite.ID, OwnerModuleID: runtimecontract.RuntimeModuleGit,
				WorkspaceDeliverableDirectory: deliverable, RequiredKinds: committedPrerequisite.RequiredCapabilities,
				CapabilityStateBinding: inventory.AcquisitionBinding, MaxItems: runtimecontract.DefaultRuntimeOwnerInventoryBudget().MaxItems,
			})
			if err != nil {
				t.Fatalf("delayed duplicate Git inventory query: %v", err)
			}
			if *restartedInventoryCalls != duplicateCallsBefore+1 {
				t.Fatalf("delayed duplicate Git inventory calls=%d, want %d", *restartedInventoryCalls, duplicateCallsBefore+1)
			}
			if _, err := restartedBiz.CommitRuntimeV2Prerequisite(ctx, "ws-v2", "run-v2", runtimev2.PrerequisiteResult{
				PrerequisiteID: duplicateResponse.PrerequisiteID, OperationAdmissibility: &duplicateResponse.Admissibility, Observation: duplicateResponse.Observation,
			}); err == nil {
				t.Fatal("delayed duplicate Git inventory unexpectedly mutated committed aggregate")
			}
			final, err := restartedBiz.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			if final.StateRevision != resumed.StateRevision || len(final.Facts.Resources.OperationAdmissibilities) != 1 ||
				final.Facts.Resources.OperationAdmissibilities[0].SnapshotID != inventory.SnapshotID || len(final.Facts.Resources.PrerequisiteObservations) != len(resumed.Facts.Resources.PrerequisiteObservations) ||
				len(final.PrerequisiteInvalidations) != len(resumed.PrerequisiteInvalidations) || final.Facts.Recovery != resumed.Facts.Recovery ||
				len(final.CandidateRequestSessions) != 0 || len(final.CandidateAttempts) != 0 || final.ActiveDispatch != nil {
				t.Fatalf("delayed duplicate Git inventory changed persisted aggregate: before=%+v after=%+v", resumed, final)
			}

			candidateContext := runtimecontract.RuntimeCandidateContextForDecision(final.Facts, final.Decision, nil)
			manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(final.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(candidateContext.Resources))
			if err != nil {
				t.Fatal(err)
			}
			prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(final.Definition, final.Facts.Activation.ExecutionContract)
			_, session, err := restartedBiz.EnsureRuntimeV2CandidateRequestSession(ctx, "ws-v2", "run-v2", prefix, manifest.Hash, "")
			if err != nil {
				t.Fatal(err)
			}
			physicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, session.NextSequence)
			if _, err := restartedBiz.MarkRuntimeV2CandidateRequestSubmitted(ctx, "ws-v2", "run-v2", session.LogicalRequestID, session.NextSequence, physicalCallID); err != nil {
				t.Fatal(err)
			}
			inventoryObservation, found := runtimecontract.RuntimePrerequisiteObservationForID(final.Facts.Resources.PrerequisiteObservations, inventory.PrerequisiteID)
			if !found {
				t.Fatalf("Git inventory observation is missing: %+v", final.Facts.Resources.PrerequisiteObservations)
			}
			var repositoryDependency runtimecontract.RuntimeFreshnessDependency
			for _, dependency := range inventoryObservation.Dependencies {
				if dependency.Domain == runtimecontract.RuntimeResourceRepositoryState {
					repositoryDependency = dependency.Normalized()
					break
				}
			}
			if repositoryDependency.Revision == "" {
				t.Fatalf("Git inventory has no repository-state dependency: %+v", inventoryObservation)
			}
			stale := runtimecontract.RuntimePrerequisiteStale{
				PrerequisiteID: inventory.PrerequisiteID, ObservationID: inventoryObservation.ObservationID,
				Domain: repositoryDependency.Domain, OwnerModuleID: repositoryDependency.OwnerModuleID, SubjectRef: repositoryDependency.SubjectRef,
				ExpectedRevision: repositoryDependency.Revision, ObservedRevision: "repository-state-after-read-only-stale",
				Reason: "Git owner observed a newer repository state",
			}
			invalidated, err := restartedBiz.InvalidateRuntimeV2CandidateRequestSessionForPrerequisiteStale(ctx, "ws-v2", "run-v2", session.LogicalRequestID, stale)
			if err != nil {
				t.Fatal(err)
			}
			if invalidated.Decision.Action != runtimecontract.RuntimeActionAcquirePrerequisite || len(invalidated.Facts.Resources.OperationAdmissibilities) != 0 ||
				len(invalidated.Facts.Resources.CapabilityStates) != 0 || len(invalidated.Facts.Resources.PrerequisiteObservations) != 1 ||
				len(invalidated.Facts.Prerequisites.Pending) != 2 || invalidated.Facts.Recovery.PrerequisiteRefreshes != 1 ||
				invalidated.Facts.Recovery.PrerequisiteResourceChangeRefreshes != 0 || len(invalidated.CandidateAttempts) != 0 || invalidated.ActiveDispatch != nil ||
				len(invalidated.CandidateRequestSessions) != 1 || invalidated.CandidateRequestSessions[0].State != runtimecontract.RuntimeCandidateRequestSessionTerminal ||
				invalidated.CandidateRequestSessions[0].LastOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomePrerequisiteStale {
				t.Fatalf("Git inventory stale did not retire the bound repository-state chain: %+v", invalidated)
			}
			// The owner stale receipt represents a real repository revision change.
			// Make that change before the fresh process re-acquires the bound state;
			// otherwise a correct owner would return the same old revision and SM-2
			// must reject it through the persisted invalidation fence.
			runRuntimeV2Git(t, deliverable, "init", "-q")
			runRuntimeV2Git(t, deliverable, "config", "user.name", "WorkOrch Test")
			runRuntimeV2Git(t, deliverable, "config", "user.email", "workorch@example.test")

			stopProcess(hub, runOwner, fileOwner, gitOwner, workspaceObserver, inventoryObserver)
			hub, processRuns, runOwner, fileOwner, gitOwner, workspaceObserver, inventoryObserver, staleReacquireCalls := startProcess(t)
			defer stopProcess(hub, runOwner, fileOwner, gitOwner, workspaceObserver, inventoryObserver)
			thirdBiz := orchestratorbiz.NewRuntimeOrchestrator()
			if err := thirdBiz.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
				t.Fatal(err)
			}
			thirdModule := newTestBiz(thirdBiz, hub, nil)
			loadedAfterStale, err := thirdBiz.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			reacquired, err := thirdModule.prepareRuntimeV2Prerequisites(ctx, loadedAfterStale)
			if err != nil {
				t.Fatal(err)
			}
			if *staleReacquireCalls != 1 || len(reacquired.Facts.Resources.OperationAdmissibilities) != 1 || len(reacquired.Facts.Resources.CapabilityStates) != 1 ||
				reacquired.Facts.Resources.OperationAdmissibilities[0].AcquisitionBinding != reacquired.Facts.Resources.CapabilityStates[0].AcquisitionBinding ||
				len(reacquired.Facts.Resources.PrerequisiteObservations) != 3 || reacquired.Facts.Recovery.PrerequisiteRefreshes != 1 ||
				reacquired.Facts.Recovery.PrerequisiteResourceChangeRefreshes != 0 || len(reacquired.CandidateAttempts) != 0 || reacquired.ActiveDispatch != nil {
				t.Fatalf("fresh-process Git stale reacquisition=%+v calls=%d", reacquired, *staleReacquireCalls)
			}
			if _, err := processRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// Shell operation admissibility is a read-only owner inventory. Its durable
// boundary is the Run aggregate CAS, not the Shell Biz instance that produced
// it. This fixture deliberately rebuilds the Run/File/Shell owners and the
// EventHub after each injected loss so it cannot reuse an in-memory receipt.
func TestRuntimeV2FileBackedShellAdmissibilityCommitCrashRestart(t *testing.T) {
	for _, crashAfterCAS := range []bool{false, true} {
		phase := "before_cas_reacquires_shell_inventory"
		if crashAfterCAS {
			phase = "after_cas_reuses_shell_inventory"
		}
		t.Run(phase, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			deliverable := filepath.Join(root, "workspace")
			if err := os.MkdirAll(deliverable, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(deliverable, "target.txt"), []byte("before\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			runsPath := filepath.Join(root, "runs.json")
			bootstrap := runtimeV2Bootstrap()
			bootstrap.Definition.CapabilityContract = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
			bootstrap.Focuses[0].RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
			bootstrap.Focuses[0].AcceptedStepKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
			bootstrap.Slices[0].RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
			bootstrap.Requirements = []runtimecontract.RuntimeRequirement{
				{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleShell,
					CapabilityClass: runtimecontract.RuntimeCapabilityClassValidation, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, Reason: "Shell capability"},
				{Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleShell,
					RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, Reason: "Shell owner inventory"},
			}
			if err := filerepository.NewRunRepository(runsPath).Create(ctx, &runmodel.Run{
				ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
				DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, State: common.RunStateActive,
			}); err != nil {
				t.Fatal(err)
			}

			shellTimeout := ""
			startProcess := func(t *testing.T) (magicevent.Hub, *filerepository.RunRepository, *runmodule.Module, *filemodule.Module, *shellmodule.Module, magicevent.SimpleObserver, magicevent.SimpleObserver, *int) {
				t.Helper()
				hub := magicevent.NewHub(32)
				processRuns := filerepository.NewRunRepository(runsPath)
				runOwner := runmodule.New()
				if err := runOwner.Configure(runmodule.Dependencies{Runs: processRuns, EventHub: hub}); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := runOwner.Start(); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
				workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
					result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable}}, nil)
				})
				fileOwner := filemodule.New()
				if err := fileOwner.Configure(filemodule.Dependencies{EventHub: hub}); err != nil {
					workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
					_ = runOwner.Stop(ctx)
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := fileOwner.Start(); err != nil {
					workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
					_ = runOwner.Stop(ctx)
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				adapter, err := shellbiz.NewRuntimeAdapterFromTimeoutString(shellTimeout)
				if err != nil {
					_ = fileOwner.Stop(ctx)
					workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
					_ = runOwner.Stop(ctx)
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				shellOwner := shellmodule.New()
				if err := shellOwner.Configure(shellmodule.Dependencies{EventHub: hub, RuntimeAdapter: adapter}); err != nil {
					_ = fileOwner.Stop(ctx)
					workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
					_ = runOwner.Stop(ctx)
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := shellOwner.Start(); err != nil {
					_ = fileOwner.Stop(ctx)
					workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
					_ = runOwner.Stop(ctx)
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				inventoryCalls := new(int)
				inventoryObserver := magicevent.NewSimpleObserverWithMatchID("shell-admissibility-query-spy", "blocks/#", hub)
				inventoryObserver.Subscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeOperationAdmissibility, func(event magicevent.Event, _ magicevent.Result) {
					command, ok := event.Data().(orchestratorevents.AcquireRuntimeOperationAdmissibilityCommand)
					if !ok || command.OwnerModuleID != runtimecontract.RuntimeModuleShell {
						t.Errorf("Shell admissibility spy received %#v", event.Data())
						return
					}
					*inventoryCalls++
				})
				return hub, processRuns, runOwner, fileOwner, shellOwner, workspaceObserver, inventoryObserver, inventoryCalls
			}
			stopProcess := func(hub magicevent.Hub, runOwner *runmodule.Module, fileOwner *filemodule.Module, shellOwner *shellmodule.Module, workspaceObserver, inventoryObserver magicevent.SimpleObserver) {
				if inventoryObserver != nil {
					inventoryObserver.Unsubscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeOperationAdmissibility)
				}
				if shellOwner != nil {
					_ = shellOwner.Stop(ctx)
				}
				if fileOwner != nil {
					_ = fileOwner.Stop(ctx)
				}
				if workspaceObserver != nil {
					workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
				}
				if runOwner != nil {
					_ = runOwner.Stop(ctx)
				}
				if hub != nil {
					hub.Terminate(ctx)
				}
			}

			hub, processRuns, runOwner, fileOwner, shellOwner, workspaceObserver, inventoryObserver, inventoryCalls := startProcess(t)
			marker := "shell-admissibility-before-cas"
			if crashAfterCAS {
				marker = "shell-admissibility-after-cas"
			}
			crashRepository := &runtimeV2SaveCrashRepository{
				delegate: runtimeV2StateRepository{hub: hub}, after: crashAfterCAS, marker: marker,
				match: func(current runtimev2.RuntimeAggregate) bool {
					return len(current.Facts.Resources.OperationAdmissibilities) == 1 && current.Facts.Resources.OperationAdmissibilities[0].OwnerModuleID == runtimecontract.RuntimeModuleShell
				},
			}
			crashingBiz := orchestratorbiz.NewRuntimeOrchestrator()
			if err := crashingBiz.ConfigureRuntimeV2Repository(crashRepository); err != nil {
				t.Fatal(err)
			}
			crashingModule := newTestBiz(crashingBiz, hub, nil)
			run, err := processRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			aggregate, err := crashingBiz.InitializeRuntimeV2FromRun(ctx, run)
			if err != nil {
				t.Fatal(err)
			}
			func() {
				defer func() {
					if recovered := recover(); recovered != marker {
						t.Fatalf("unexpected Shell admissibility crash marker: %v", recovered)
					}
				}()
				_, _ = crashingModule.prepareRuntimeV2Prerequisites(ctx, aggregate)
				t.Fatal("expected Shell admissibility crash")
			}()
			if !crashRepository.fired || *inventoryCalls != 1 {
				t.Fatalf("Shell admissibility crash fired=%t calls=%d", crashRepository.fired, *inventoryCalls)
			}
			crashed, err := processRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil || crashed.RuntimeState == nil {
				t.Fatalf("crashed Shell admissibility state=%+v err=%v", crashed, err)
			}
			if crashAfterCAS {
				if len(crashed.RuntimeState.Facts.Resources.OperationAdmissibilities) != 1 || len(crashed.RuntimeState.Facts.Resources.PrerequisiteObservations) != 2 {
					t.Fatalf("after-CAS Shell inventory was not persisted: %+v", crashed.RuntimeState)
				}
			} else if len(crashed.RuntimeState.Facts.Resources.OperationAdmissibilities) != 0 || len(crashed.RuntimeState.Facts.Resources.PrerequisiteObservations) != 1 {
				t.Fatalf("before-CAS Shell inventory leaked into persisted state: %+v", crashed.RuntimeState)
			}

			stopProcess(hub, runOwner, fileOwner, shellOwner, workspaceObserver, inventoryObserver)
			hub, processRuns, runOwner, fileOwner, shellOwner, workspaceObserver, inventoryObserver, restartedInventoryCalls := startProcess(t)
			defer stopProcess(hub, runOwner, fileOwner, shellOwner, workspaceObserver, inventoryObserver)
			restartedBiz := orchestratorbiz.NewRuntimeOrchestrator()
			if err := restartedBiz.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
				t.Fatal(err)
			}
			restartedModule := newTestBiz(restartedBiz, hub, nil)
			loaded, err := restartedBiz.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			resumed, err := restartedModule.prepareRuntimeV2Prerequisites(ctx, loaded)
			if err != nil {
				t.Fatal(err)
			}
			if len(resumed.Facts.Resources.OperationAdmissibilities) != 1 || resumed.Facts.Resources.OperationAdmissibilities[0].OwnerModuleID != runtimecontract.RuntimeModuleShell ||
				resumed.Facts.Resources.OperationAdmissibilities[0].SnapshotID == "" || resumed.Facts.Resources.OperationAdmissibilities[0].ObservedRevision == "" ||
				len(resumed.Facts.Resources.PrerequisiteObservations) != 2 || len(resumed.CandidateRequestSessions) != 0 || len(resumed.CandidateAttempts) != 0 ||
				resumed.ActiveDispatch != nil || len(resumed.CompletedDispatches) != 0 || len(resumed.GovernanceRequests) != 0 || len(resumed.OperationReceipts) != 0 ||
				len(resumed.Facts.Evidence.Accepted) != 0 || len(resumed.Facts.Evidence.Rejected) != 0 {
				t.Fatalf("restart Shell admissibility aggregate=%+v", resumed)
			}
			if !crashAfterCAS && *restartedInventoryCalls != 1 {
				t.Fatalf("before-CAS restart Shell inventory calls=%d, want 1", *restartedInventoryCalls)
			} else if crashAfterCAS && *restartedInventoryCalls != 0 {
				t.Fatalf("after-CAS restart re-queried committed Shell inventory: calls=%d", *restartedInventoryCalls)
			}
			inventory := resumed.Facts.Resources.OperationAdmissibilities[0]
			inventoryObservation, found := runtimecontract.RuntimePrerequisiteObservationForID(resumed.Facts.Resources.PrerequisiteObservations, inventory.PrerequisiteID)
			if !found {
				t.Fatalf("restart Shell inventory observation is missing: %+v", resumed.Facts.Resources.PrerequisiteObservations)
			}
			if crashAfterCAS {
				persistedInventory := crashed.RuntimeState.Facts.Resources.OperationAdmissibilities[0]
				persistedObservation, found := runtimecontract.RuntimePrerequisiteObservationForID(crashed.RuntimeState.Facts.Resources.PrerequisiteObservations, persistedInventory.PrerequisiteID)
				if !found || inventory.SnapshotID != persistedInventory.SnapshotID || inventory.ObservedRevision != persistedInventory.ObservedRevision ||
					inventory.AcquisitionBinding != persistedInventory.AcquisitionBinding || inventoryObservation.ObservationID != persistedObservation.ObservationID ||
					inventoryObservation.ContentHash != persistedObservation.ContentHash {
					t.Fatalf("after-CAS restart did not reuse exact Shell inventory: persisted=%+v resumed=%+v", persistedInventory, inventory)
				}
			}

			var committedPrerequisite runtimecontract.RuntimePrerequisite
			for _, prerequisite := range resumed.Facts.Prerequisites.Satisfied {
				if prerequisite.ID == inventory.PrerequisiteID {
					committedPrerequisite = prerequisite
					break
				}
			}
			if committedPrerequisite.ID == "" {
				t.Fatalf("restart did not retain satisfied Shell inventory prerequisite: %+v", resumed.Facts.Prerequisites)
			}
			duplicateCallsBefore := *restartedInventoryCalls
			duplicateResponse, err := AcquireRuntimeOperationAdmissibilityPrerequisite(ctx, hub, orchestratorevents.AcquireRuntimeOperationAdmissibilityCommand{
				WorkspaceID: "ws-v2", RunID: "run-v2", AuthoritySnapshotID: resumed.Decision.AuthoritySnapshot.ID,
				PrerequisiteID: committedPrerequisite.ID, OwnerModuleID: runtimecontract.RuntimeModuleShell,
				WorkspaceDeliverableDirectory: deliverable, RequiredKinds: committedPrerequisite.RequiredCapabilities,
				RequiredTargetRefs: runtimev2.GroundedRepairTargets(resumed.Facts), MaxItems: runtimecontract.DefaultRuntimeOwnerInventoryBudget().MaxItems,
			})
			if err != nil {
				t.Fatalf("delayed duplicate Shell inventory query: %v", err)
			}
			if *restartedInventoryCalls != duplicateCallsBefore+1 {
				t.Fatalf("delayed duplicate Shell inventory calls=%d, want %d", *restartedInventoryCalls, duplicateCallsBefore+1)
			}
			if _, err := restartedBiz.CommitRuntimeV2Prerequisite(ctx, "ws-v2", "run-v2", runtimev2.PrerequisiteResult{
				PrerequisiteID: duplicateResponse.PrerequisiteID, OperationAdmissibility: &duplicateResponse.Admissibility, Observation: duplicateResponse.Observation,
			}); err == nil {
				t.Fatal("delayed duplicate Shell inventory unexpectedly mutated committed aggregate")
			}
			final, err := restartedBiz.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			if final.StateRevision != resumed.StateRevision || len(final.Facts.Resources.OperationAdmissibilities) != 1 ||
				final.Facts.Resources.OperationAdmissibilities[0].SnapshotID != inventory.SnapshotID || len(final.Facts.Resources.PrerequisiteObservations) != len(resumed.Facts.Resources.PrerequisiteObservations) ||
				len(final.PrerequisiteInvalidations) != len(resumed.PrerequisiteInvalidations) || final.Facts.Recovery != resumed.Facts.Recovery ||
				len(final.CandidateRequestSessions) != 0 || len(final.CandidateAttempts) != 0 || final.ActiveDispatch != nil {
				t.Fatalf("delayed duplicate Shell inventory changed persisted aggregate: before=%+v after=%+v", resumed, final)
			}

			var targetDependency runtimecontract.RuntimeFreshnessDependency
			for _, dependency := range inventoryObservation.Dependencies {
				if dependency.Domain == runtimecontract.RuntimeResourceWorkspaceTarget && dependency.OwnerModuleID == runtimecontract.RuntimeModuleFile && dependency.SubjectRef == "target.txt" {
					targetDependency = dependency.Normalized()
					break
				}
			}
			if targetDependency.Revision == "" {
				t.Fatalf("Shell inventory observation has no File-owned target dependency: %+v", inventoryObservation)
			}
			if err := os.WriteFile(filepath.Join(deliverable, "target.txt"), []byte("after\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			candidateContext := runtimecontract.RuntimeCandidateContextForDecision(final.Facts, final.Decision, nil)
			manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(final.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(candidateContext.Resources))
			if err != nil {
				t.Fatal(err)
			}
			prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(final.Definition, final.Facts.Activation.ExecutionContract)
			_, session, err := restartedBiz.EnsureRuntimeV2CandidateRequestSession(ctx, "ws-v2", "run-v2", prefix, manifest.Hash, "")
			if err != nil {
				t.Fatal(err)
			}
			physicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, session.NextSequence)
			if _, err := restartedBiz.MarkRuntimeV2CandidateRequestSubmitted(ctx, "ws-v2", "run-v2", session.LogicalRequestID, session.NextSequence, physicalCallID); err != nil {
				t.Fatal(err)
			}
			stale := runtimecontract.RuntimePrerequisiteStale{
				PrerequisiteID: inventory.PrerequisiteID, ObservationID: inventoryObservation.ObservationID,
				Domain: targetDependency.Domain, OwnerModuleID: targetDependency.OwnerModuleID, SubjectRef: targetDependency.SubjectRef,
				ExpectedRevision: targetDependency.Revision, ObservedRevision: "target-after-read-only-stale", Reason: "File owner observed a newer validation target revision",
			}
			invalidated, err := restartedBiz.InvalidateRuntimeV2CandidateRequestSessionForPrerequisiteStale(ctx, "ws-v2", "run-v2", session.LogicalRequestID, stale)
			if err != nil {
				t.Fatal(err)
			}
			if invalidated.Decision.Action != runtimecontract.RuntimeActionAcquirePrerequisite || len(invalidated.Facts.Resources.OperationAdmissibilities) != 0 ||
				len(invalidated.Facts.Resources.PrerequisiteObservations) != 1 || len(invalidated.Facts.Prerequisites.Pending) != 1 ||
				invalidated.Facts.Prerequisites.Pending[0].ID != inventory.PrerequisiteID || invalidated.Facts.Recovery.PrerequisiteRefreshes != 1 ||
				invalidated.Facts.Recovery.PrerequisiteResourceChangeRefreshes != 0 || len(invalidated.CandidateAttempts) != 0 || invalidated.ActiveDispatch != nil ||
				len(invalidated.CandidateRequestSessions) != 1 || invalidated.CandidateRequestSessions[0].State != runtimecontract.RuntimeCandidateRequestSessionTerminal ||
				invalidated.CandidateRequestSessions[0].LastOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomePrerequisiteStale {
				t.Fatalf("Shell target stale did not isolate Shell inventory: %+v", invalidated)
			}

			stopProcess(hub, runOwner, fileOwner, shellOwner, workspaceObserver, inventoryObserver)
			hub, processRuns, runOwner, fileOwner, shellOwner, workspaceObserver, inventoryObserver, staleReacquireCalls := startProcess(t)
			defer stopProcess(hub, runOwner, fileOwner, shellOwner, workspaceObserver, inventoryObserver)
			thirdBiz := orchestratorbiz.NewRuntimeOrchestrator()
			if err := thirdBiz.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
				t.Fatal(err)
			}
			thirdModule := newTestBiz(thirdBiz, hub, nil)
			loadedAfterStale, err := thirdBiz.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			reacquired, err := thirdModule.prepareRuntimeV2Prerequisites(ctx, loadedAfterStale)
			if err != nil {
				t.Fatal(err)
			}
			reacquiredInventory := reacquired.Facts.Resources.OperationAdmissibilities[0]
			reacquiredObservation, found := runtimecontract.RuntimePrerequisiteObservationForID(reacquired.Facts.Resources.PrerequisiteObservations, reacquiredInventory.PrerequisiteID)
			if !found || *staleReacquireCalls != 1 || len(reacquired.Facts.Resources.OperationAdmissibilities) != 1 ||
				len(reacquired.Facts.Resources.PrerequisiteObservations) != 2 || reacquired.Facts.Recovery.PrerequisiteRefreshes != 1 ||
				reacquired.Facts.Recovery.PrerequisiteResourceChangeRefreshes != 0 || reacquiredObservation.ObservationID == inventoryObservation.ObservationID ||
				len(reacquired.CandidateAttempts) != 0 || reacquired.ActiveDispatch != nil {
				t.Fatalf("fresh-process Shell inventory stale reacquisition=%+v calls=%d", reacquired, *staleReacquireCalls)
			}
			var policyDependency runtimecontract.RuntimeFreshnessDependency
			for _, dependency := range reacquiredObservation.Dependencies {
				if dependency.Domain == runtimecontract.RuntimeResourceOwnerCatalog && dependency.OwnerModuleID == runtimecontract.RuntimeModuleShell && dependency.SubjectRef == "shell_runtime_policy" {
					policyDependency = dependency.Normalized()
					break
				}
			}
			if policyDependency.Revision == "" {
				t.Fatalf("Shell inventory observation has no Shell policy dependency: %+v", reacquiredObservation)
			}
			candidateContext = runtimecontract.RuntimeCandidateContextForDecision(reacquired.Facts, reacquired.Decision, nil)
			manifest, err = runtimecontract.BuildRuntimeCandidateContractManifest(reacquired.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(candidateContext.Resources))
			if err != nil {
				t.Fatal(err)
			}
			prefix = runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(reacquired.Definition, reacquired.Facts.Activation.ExecutionContract)
			_, policySession, err := thirdBiz.EnsureRuntimeV2CandidateRequestSession(ctx, "ws-v2", "run-v2", prefix, manifest.Hash, "")
			if err != nil {
				t.Fatal(err)
			}
			policyPhysicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(policySession.LogicalRequestID, policySession.NextSequence)
			if _, err := thirdBiz.MarkRuntimeV2CandidateRequestSubmitted(ctx, "ws-v2", "run-v2", policySession.LogicalRequestID, policySession.NextSequence, policyPhysicalCallID); err != nil {
				t.Fatal(err)
			}
			policyStale := runtimecontract.RuntimePrerequisiteStale{
				PrerequisiteID: reacquiredInventory.PrerequisiteID, ObservationID: reacquiredObservation.ObservationID,
				Domain: policyDependency.Domain, OwnerModuleID: policyDependency.OwnerModuleID, SubjectRef: policyDependency.SubjectRef,
				ExpectedRevision: policyDependency.Revision, ObservedRevision: "shell-policy-changed", Reason: "Shell runtime policy changed",
			}
			policyInvalidated, err := thirdBiz.InvalidateRuntimeV2CandidateRequestSessionForPrerequisiteStale(ctx, "ws-v2", "run-v2", policySession.LogicalRequestID, policyStale)
			if err != nil {
				t.Fatal(err)
			}
			if policyInvalidated.Decision.Action != runtimecontract.RuntimeActionAcquirePrerequisite || len(policyInvalidated.Facts.Resources.OperationAdmissibilities) != 0 ||
				len(policyInvalidated.Facts.Resources.PrerequisiteObservations) != 1 || len(policyInvalidated.Facts.Prerequisites.Pending) != 1 ||
				policyInvalidated.Facts.Prerequisites.Pending[0].ID != reacquiredInventory.PrerequisiteID || policyInvalidated.Facts.Recovery.PrerequisiteRefreshes != 2 ||
				policyInvalidated.Facts.Recovery.PrerequisiteResourceChangeRefreshes != 0 || len(policyInvalidated.CandidateAttempts) != 0 || policyInvalidated.ActiveDispatch != nil {
				t.Fatalf("Shell policy stale did not isolate Shell inventory: %+v", policyInvalidated)
			}

			// A changed timeout is a different Shell policy revision. It is supplied
			// only to the newly constructed owner after the stale CAS; no policy
			// value is written by SM-2 or inherited from the prior process.
			stopProcess(hub, runOwner, fileOwner, shellOwner, workspaceObserver, inventoryObserver)
			shellTimeout = "90s"
			hub, processRuns, runOwner, fileOwner, shellOwner, workspaceObserver, inventoryObserver, policyReacquireCalls := startProcess(t)
			defer stopProcess(hub, runOwner, fileOwner, shellOwner, workspaceObserver, inventoryObserver)
			fourthBiz := orchestratorbiz.NewRuntimeOrchestrator()
			if err := fourthBiz.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
				t.Fatal(err)
			}
			fourthModule := newTestBiz(fourthBiz, hub, nil)
			loadedAfterPolicyStale, err := fourthBiz.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			reacquiredPolicy, err := fourthModule.prepareRuntimeV2Prerequisites(ctx, loadedAfterPolicyStale)
			if err != nil {
				t.Fatal(err)
			}
			policyInventory := reacquiredPolicy.Facts.Resources.OperationAdmissibilities[0]
			policyObservation, found := runtimecontract.RuntimePrerequisiteObservationForID(reacquiredPolicy.Facts.Resources.PrerequisiteObservations, policyInventory.PrerequisiteID)
			if !found || *policyReacquireCalls != 1 || len(reacquiredPolicy.Facts.Resources.OperationAdmissibilities) != 1 ||
				len(reacquiredPolicy.Facts.Resources.PrerequisiteObservations) != 2 || reacquiredPolicy.Facts.Recovery.PrerequisiteRefreshes != 2 ||
				reacquiredPolicy.Facts.Recovery.PrerequisiteResourceChangeRefreshes != 0 || policyInventory.ObservedRevision == reacquiredInventory.ObservedRevision ||
				policyObservation.ObservationID == reacquiredObservation.ObservationID || len(reacquiredPolicy.CandidateAttempts) != 0 || reacquiredPolicy.ActiveDispatch != nil {
				t.Fatalf("fresh-process Shell policy stale reacquisition=%+v calls=%d", reacquiredPolicy, *policyReacquireCalls)
			}
			if _, err := processRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestRuntimeV2FileBackedAdmissibilityThreeOwnerMergeRestart proves the SM-2
// merge boundary with the three production inventory owners. Each inventory is
// committed independently, then every process-local object is destroyed before
// the next owner runs. The final restart must therefore recover only the three
// persisted owner facts, never an in-memory receipt or candidate side effect.
func TestRuntimeV2FileBackedAdmissibilityThreeOwnerMergeRestart(t *testing.T) {
	runRuntimeV2FileBackedAdmissibilityThreeOwnerRestart(t, "")
}

func TestRuntimeV2FileBackedAdmissibilityThreeOwnerStaleRestart(t *testing.T) {
	for _, staleCase := range []string{"file_target", "git_repository", "shell_policy"} {
		t.Run(staleCase, func(t *testing.T) {
			runRuntimeV2FileBackedAdmissibilityThreeOwnerRestart(t, staleCase)
		})
	}
}

func runRuntimeV2FileBackedAdmissibilityThreeOwnerRestart(t *testing.T, staleCase string) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	ctx := context.Background()
	root := t.TempDir()
	shellTimeout := shellbiz.DefaultCommandTimeout
	deliverable, internalDir := filepath.Join(root, "workspace"), filepath.Join(root, "internal")
	if err := os.MkdirAll(deliverable, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(internalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deliverable, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runsPath, receiptRoot := filepath.Join(root, "runs.json"), filepath.Join(root, "git-owner-data")
	bootstrap := runtimeV2FileShellGitBootstrap()
	if err := filerepository.NewRunRepository(runsPath).Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
		DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, State: common.RunStateActive,
	}); err != nil {
		t.Fatal(err)
	}

	type process struct {
		hub          magicevent.Hub
		runs         *filerepository.RunRepository
		runOwner     *runmodule.Module
		fileOwner    *filemodule.Module
		gitOwner     *gitmodule.Module
		shellOwner   *shellmodule.Module
		workspaceObs magicevent.SimpleObserver
		inventoryObs magicevent.SimpleObserver
		queries      map[string]int
	}
	startProcess := func(t *testing.T) *process {
		t.Helper()
		value := &process{hub: magicevent.NewHub(48), runs: filerepository.NewRunRepository(runsPath), queries: map[string]int{}}
		value.runOwner = runmodule.New()
		if err := value.runOwner.Configure(runmodule.Dependencies{Runs: value.runs, EventHub: value.hub}); err != nil {
			value.hub.Terminate(ctx)
			t.Fatal(err)
		}
		if err := value.runOwner.Start(); err != nil {
			value.hub.Terminate(ctx)
			t.Fatal(err)
		}
		value.workspaceObs = magicevent.NewSimpleObserver(workspaceevents.CommandDestination, value.hub)
		value.workspaceObs.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
			result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
		})
		value.fileOwner = filemodule.New()
		if err := value.fileOwner.Configure(filemodule.Dependencies{EventHub: value.hub}); err != nil {
			value.workspaceObs.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
			_ = value.runOwner.Stop(ctx)
			value.hub.Terminate(ctx)
			t.Fatal(err)
		}
		if err := value.fileOwner.Start(); err != nil {
			value.workspaceObs.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
			_ = value.runOwner.Stop(ctx)
			value.hub.Terminate(ctx)
			t.Fatal(err)
		}
		receipts, err := gitbiz.NewCapabilityStateReceiptStore(receiptRoot)
		if err != nil {
			t.Fatal(err)
		}
		value.gitOwner = gitmodule.New()
		if err := value.gitOwner.Configure(gitmodule.Dependencies{EventHub: value.hub, RuntimeAdapter: gitbiz.NewRuntimeAdapter(true), CapabilityStateReceipts: receipts}); err != nil {
			t.Fatal(err)
		}
		if err := value.gitOwner.Start(); err != nil {
			t.Fatal(err)
		}
		value.shellOwner = shellmodule.New()
		if err := value.shellOwner.Configure(shellmodule.Dependencies{EventHub: value.hub, RuntimeAdapter: shellbiz.NewRuntimeAdapterWithTimeout(shellTimeout)}); err != nil {
			t.Fatal(err)
		}
		if err := value.shellOwner.Start(); err != nil {
			t.Fatal(err)
		}
		value.inventoryObs = magicevent.NewSimpleObserverWithMatchID("three-owner-admissibility-query-spy", "blocks/#", value.hub)
		value.inventoryObs.Subscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeOperationAdmissibility, func(event magicevent.Event, _ magicevent.Result) {
			command, ok := event.Data().(orchestratorevents.AcquireRuntimeOperationAdmissibilityCommand)
			if !ok {
				t.Errorf("three-owner admissibility spy received %T", event.Data())
				return
			}
			value.queries[command.OwnerModuleID]++
		})
		return value
	}
	stopProcess := func(value *process) {
		if value == nil {
			return
		}
		if value.inventoryObs != nil {
			value.inventoryObs.Unsubscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeOperationAdmissibility)
		}
		if value.shellOwner != nil {
			_ = value.shellOwner.Stop(ctx)
		}
		if value.gitOwner != nil {
			_ = value.gitOwner.Stop(ctx)
		}
		if value.fileOwner != nil {
			_ = value.fileOwner.Stop(ctx)
		}
		if value.workspaceObs != nil {
			value.workspaceObs.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
		}
		if value.runOwner != nil {
			_ = value.runOwner.Stop(ctx)
		}
		if value.hub != nil {
			value.hub.Terminate(ctx)
		}
	}

	value := startProcess(t)
	defer func() { stopProcess(value) }()
	newBiz := func(hub magicevent.Hub) (*RuntimeOrchestrator, *Biz) {
		owner := NewRuntimeOrchestrator()
		if err := owner.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
			t.Fatal(err)
		}
		return owner, newTestBiz(owner, hub, nil)
	}
	biz, module := newBiz(value.hub)
	run, err := value.runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err := biz.InitializeRuntimeV2FromRun(ctx, run)
	if err != nil {
		t.Fatal(err)
	}

	// Establish capabilities, manifest and the Git repository-state binding
	// first. They are prerequisites for the three owner inventories but are not
	// the subject of this merge proof.
	for {
		var next *runtimecontract.RuntimePrerequisite
		for _, prerequisite := range runtimeV2OrderedPrerequisites(aggregate.Facts.Prerequisites.Pending) {
			if prerequisite.Kind != runtimecontract.PrerequisiteKindOperationAdmissibility {
				candidate := prerequisite
				next = &candidate
				break
			}
		}
		if next == nil {
			break
		}
		result, err := module.resolveRuntimeV2Prerequisite(ctx, aggregate, deliverable, root, *next)
		if err != nil {
			t.Fatal(err)
		}
		aggregate, err = biz.CommitRuntimeV2Prerequisite(ctx, "ws-v2", "run-v2", result)
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(aggregate.Facts.Resources.OperationAdmissibilities) != 0 || len(aggregate.Facts.Resources.CapabilityStates) != 1 {
		t.Fatalf("three-owner fixture did not establish prerequisites before inventories: %+v", aggregate.Facts)
	}

	findPending := func(current runtimev2.RuntimeAggregate, owner string) runtimecontract.RuntimePrerequisite {
		for _, prerequisite := range current.Facts.Prerequisites.Pending {
			if prerequisite.Kind == runtimecontract.PrerequisiteKindOperationAdmissibility && prerequisite.OwnerModuleID == owner {
				return prerequisite
			}
		}
		t.Fatalf("missing pending admissibility prerequisite for %s: %+v", owner, current.Facts.Prerequisites.Pending)
		return runtimecontract.RuntimePrerequisite{}
	}
	owners := []string{runtimecontract.RuntimeModuleFile, runtimecontract.RuntimeModuleGit, runtimecontract.RuntimeModuleShell}
	for index, owner := range owners {
		prerequisite := findPending(aggregate, owner)
		result, err := module.resolveRuntimeV2Prerequisite(ctx, aggregate, deliverable, root, prerequisite)
		if err != nil {
			t.Fatalf("resolve %s inventory: %v", owner, err)
		}
		aggregate, err = biz.CommitRuntimeV2Prerequisite(ctx, "ws-v2", "run-v2", result)
		if err != nil {
			t.Fatalf("commit %s inventory: %v", owner, err)
		}
		if value.queries[owner] != 1 || len(aggregate.Facts.Resources.OperationAdmissibilities) != index+1 ||
			len(aggregate.CandidateRequestSessions) != 0 || len(aggregate.CandidateAttempts) != 0 || aggregate.ActiveDispatch != nil ||
			len(aggregate.GovernanceRequests) != 0 || len(aggregate.OperationReceipts) != 0 {
			t.Fatalf("%s inventory commit leaked another state machine: aggregate=%+v queries=%v", owner, aggregate, value.queries)
		}
		stopProcess(value)
		value = startProcess(t)
		biz, module = newBiz(value.hub)
		aggregate, err = biz.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
		if err != nil {
			t.Fatal(err)
		}
		if len(aggregate.Facts.Resources.OperationAdmissibilities) != index+1 || value.queries[owner] != 0 {
			t.Fatalf("restart after %s inventory did not reuse committed merge: aggregate=%+v queries=%v", owner, aggregate, value.queries)
		}
	}

	if len(aggregate.Facts.Resources.OperationAdmissibilities) != len(owners) || len(aggregate.Facts.Resources.PrerequisiteObservations) < len(owners) ||
		len(aggregate.CandidateRequestSessions) != 0 || len(aggregate.CandidateAttempts) != 0 || aggregate.ActiveDispatch != nil ||
		len(aggregate.GovernanceRequests) != 0 || len(aggregate.OperationReceipts) != 0 {
		t.Fatalf("three-owner restart merge did not preserve only prerequisites: %+v", aggregate)
	}
	for _, owner := range owners {
		var inventory runtimecontract.RuntimeOperationAdmissibility
		var prerequisite runtimecontract.RuntimePrerequisite
		for _, candidate := range aggregate.Facts.Resources.OperationAdmissibilities {
			if candidate.OwnerModuleID == owner {
				inventory = candidate
				break
			}
		}
		for _, candidate := range aggregate.Facts.Prerequisites.Satisfied {
			if candidate.ID == inventory.PrerequisiteID {
				prerequisite = candidate
				break
			}
		}
		if inventory.PrerequisiteID == "" || prerequisite.ID == "" {
			t.Fatalf("three-owner merge lost %s inventory/prerequisite: %+v", owner, aggregate.Facts)
		}
		beforeRevision := aggregate.StateRevision
		response, err := AcquireRuntimeOperationAdmissibilityPrerequisite(ctx, value.hub, orchestratorevents.AcquireRuntimeOperationAdmissibilityCommand{
			WorkspaceID: "ws-v2", RunID: "run-v2", AuthoritySnapshotID: aggregate.Decision.AuthoritySnapshot.ID,
			PrerequisiteID: prerequisite.ID, OwnerModuleID: owner, WorkspaceDeliverableDirectory: deliverable,
			RequiredKinds: prerequisite.RequiredCapabilities, RequiredTargetRefs: runtimev2.GroundedRepairTargets(aggregate.Facts),
			CapabilityStateBinding: inventory.AcquisitionBinding, MaxItems: runtimecontract.DefaultRuntimeOwnerInventoryBudget().MaxItems,
		})
		if err != nil {
			t.Fatalf("delayed duplicate %s inventory query: %v", owner, err)
		}
		if _, err := biz.CommitRuntimeV2Prerequisite(ctx, "ws-v2", "run-v2", runtimev2.PrerequisiteResult{
			PrerequisiteID: response.PrerequisiteID, OperationAdmissibility: &response.Admissibility, Observation: response.Observation,
		}); err == nil {
			t.Fatalf("delayed duplicate %s inventory unexpectedly committed", owner)
		}
		aggregate, err = biz.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
		if err != nil || aggregate.StateRevision != beforeRevision || len(aggregate.Facts.Resources.OperationAdmissibilities) != len(owners) ||
			len(aggregate.CandidateRequestSessions) != 0 || len(aggregate.CandidateAttempts) != 0 || aggregate.ActiveDispatch != nil {
			t.Fatalf("duplicate %s inventory mutated merged aggregate: aggregate=%+v err=%v", owner, aggregate, err)
		}
	}
	if staleCase == "" {
		return
	}

	findObservation := func(current runtimev2.RuntimeAggregate, prerequisiteID string) runtimecontract.RuntimePrerequisiteObservation {
		for _, observation := range current.Facts.Resources.PrerequisiteObservations {
			if observation.PrerequisiteID == prerequisiteID {
				return observation
			}
		}
		t.Fatalf("missing current observation for %s: %+v", prerequisiteID, current.Facts.Resources.PrerequisiteObservations)
		return runtimecontract.RuntimePrerequisiteObservation{}
	}
	findInventory := func(current runtimev2.RuntimeAggregate, owner string) runtimecontract.RuntimeOperationAdmissibility {
		for _, inventory := range current.Facts.Resources.OperationAdmissibilities {
			if inventory.OwnerModuleID == owner {
				return inventory
			}
		}
		t.Fatalf("missing current inventory for %s: %+v", owner, current.Facts.Resources.OperationAdmissibilities)
		return runtimecontract.RuntimeOperationAdmissibility{}
	}
	findDependency := func(observation runtimecontract.RuntimePrerequisiteObservation, domain runtimecontract.RuntimeResourceDomain, owner string) runtimecontract.RuntimeFreshnessDependency {
		for _, dependency := range observation.Dependencies {
			if dependency.Domain == domain && dependency.OwnerModuleID == owner {
				return dependency
			}
		}
		t.Fatalf("missing %s/%s dependency in observation %+v", domain, owner, observation)
		return runtimecontract.RuntimeFreshnessDependency{}
	}
	var stale runtimecontract.RuntimePrerequisiteStale
	var wantRetired []string
	switch staleCase {
	case "file_target":
		observation := findObservation(aggregate, findInventory(aggregate, runtimecontract.RuntimeModuleFile).PrerequisiteID)
		dependency := findDependency(observation, runtimecontract.RuntimeResourceWorkspaceTarget, runtimecontract.RuntimeModuleFile)
		if err := os.WriteFile(filepath.Join(deliverable, dependency.SubjectRef), []byte("package main\n// changed before prerequisite refresh\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		stale = runtimecontract.RuntimePrerequisiteStale{PrerequisiteID: observation.PrerequisiteID, ObservationID: observation.ObservationID, Domain: dependency.Domain, OwnerModuleID: dependency.OwnerModuleID, SubjectRef: dependency.SubjectRef, ExpectedRevision: dependency.Revision, ObservedRevision: "file-target-changed", Reason: "file target changed"}
		wantRetired = []string{runtimecontract.RuntimeModuleFile, runtimecontract.RuntimeModuleShell}
	case "git_repository":
		observation := findObservation(aggregate, findInventory(aggregate, runtimecontract.RuntimeModuleGit).PrerequisiteID)
		dependency := findDependency(observation, runtimecontract.RuntimeResourceRepositoryState, runtimecontract.RuntimeModuleGit)
		runRuntimeV2Git(t, deliverable, "init", "-q")
		runRuntimeV2Git(t, deliverable, "config", "user.name", "WorkOrch Test")
		runRuntimeV2Git(t, deliverable, "config", "user.email", "workorch@example.test")
		stale = runtimecontract.RuntimePrerequisiteStale{PrerequisiteID: observation.PrerequisiteID, ObservationID: observation.ObservationID, Domain: dependency.Domain, OwnerModuleID: dependency.OwnerModuleID, SubjectRef: dependency.SubjectRef, ExpectedRevision: dependency.Revision, ObservedRevision: "repository-changed", Reason: "repository state changed"}
		wantRetired = []string{runtimecontract.RuntimeModuleGit}
	case "shell_policy":
		observation := findObservation(aggregate, findInventory(aggregate, runtimecontract.RuntimeModuleShell).PrerequisiteID)
		dependency := findDependency(observation, runtimecontract.RuntimeResourceOwnerCatalog, runtimecontract.RuntimeModuleShell)
		shellTimeout = 30 * time.Second
		stale = runtimecontract.RuntimePrerequisiteStale{PrerequisiteID: observation.PrerequisiteID, ObservationID: observation.ObservationID, Domain: dependency.Domain, OwnerModuleID: dependency.OwnerModuleID, SubjectRef: dependency.SubjectRef, ExpectedRevision: dependency.Revision, ObservedRevision: "shell-policy-changed", Reason: "shell policy changed"}
		wantRetired = []string{runtimecontract.RuntimeModuleShell}
	default:
		t.Fatalf("unknown stale case %q", staleCase)
	}
	type ownerFact struct {
		snapshotID         string
		acquisitionBinding string
		observationID      string
	}
	beforeFacts := make(map[string]ownerFact, len(owners))
	for _, owner := range owners {
		inventory := findInventory(aggregate, owner)
		observation := findObservation(aggregate, inventory.PrerequisiteID)
		beforeFacts[owner] = ownerFact{snapshotID: inventory.SnapshotID, acquisitionBinding: inventory.AcquisitionBinding, observationID: observation.ObservationID}
	}
	beforeRevision := aggregate.StateRevision
	invalidated, err := biz.InvalidateRuntimeV2PrerequisitesForReadOnlyStale(ctx, "ws-v2", "run-v2", stale)
	if err != nil || invalidated.StateRevision != beforeRevision+1 || invalidated.Facts.Recovery.PrerequisiteRefreshes != 1 ||
		len(invalidated.CandidateRequestSessions) != 0 || len(invalidated.CandidateAttempts) != 0 || invalidated.ActiveDispatch != nil ||
		len(invalidated.GovernanceRequests) != 0 || len(invalidated.OperationReceipts) != 0 {
		t.Fatalf("%s stale invalidation did not stay in SM-2: aggregate=%+v err=%v", staleCase, invalidated, err)
	}
	for _, owner := range owners {
		found := false
		for _, inventory := range invalidated.Facts.Resources.OperationAdmissibilities {
			if inventory.OwnerModuleID == owner {
				found = true
			}
		}
		want := true
		for _, retired := range wantRetired {
			if owner == retired {
				want = false
			}
		}
		if found != want {
			t.Fatalf("%s stale inventory ownership changed incorrectly for %s: inventories=%+v", staleCase, owner, invalidated.Facts.Resources.OperationAdmissibilities)
		}
	}
	stopProcess(value)
	value = startProcess(t)
	biz, module = newBiz(value.hub)
	aggregate, err = biz.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	for _, owner := range owners {
		if value.queries[owner] != 0 {
			t.Fatalf("%s stale restart unexpectedly queried %s before re-drive: %v", staleCase, owner, value.queries)
		}
	}
	for {
		var next *runtimecontract.RuntimePrerequisite
		for _, prerequisite := range runtimeV2OrderedPrerequisites(aggregate.Facts.Prerequisites.Pending) {
			if prerequisite.Kind != runtimecontract.PrerequisiteKindOperationAdmissibility {
				candidate := prerequisite
				next = &candidate
				break
			}
		}
		if next == nil {
			break
		}
		result, err := module.resolveRuntimeV2Prerequisite(ctx, aggregate, deliverable, root, *next)
		if err != nil {
			t.Fatalf("%s stale reacquire dependency %s: %v", staleCase, next.ID, err)
		}
		aggregate, err = biz.CommitRuntimeV2Prerequisite(ctx, "ws-v2", "run-v2", result)
		if err != nil {
			t.Fatalf("%s stale commit dependency %s: %v", staleCase, next.ID, err)
		}
	}
	for _, owner := range wantRetired {
		prerequisite := findPending(aggregate, owner)
		result, err := module.resolveRuntimeV2Prerequisite(ctx, aggregate, deliverable, root, prerequisite)
		if err != nil {
			t.Fatalf("%s stale reacquire %s: %v", staleCase, owner, err)
		}
		aggregate, err = biz.CommitRuntimeV2Prerequisite(ctx, "ws-v2", "run-v2", result)
		if err != nil {
			t.Fatalf("%s stale commit %s: %v", staleCase, owner, err)
		}
	}
	for _, owner := range owners {
		wantQueries := 0
		for _, retired := range wantRetired {
			if owner == retired {
				wantQueries = 1
			}
		}
		if value.queries[owner] != wantQueries {
			t.Fatalf("%s stale restart query count for %s=%d want %d", staleCase, owner, value.queries[owner], wantQueries)
		}
	}
	if aggregate.Facts.Recovery.PrerequisiteRefreshes != 1 || len(aggregate.CandidateRequestSessions) != 0 ||
		len(aggregate.CandidateAttempts) != 0 || aggregate.ActiveDispatch != nil || len(aggregate.GovernanceRequests) != 0 ||
		len(aggregate.OperationReceipts) != 0 {
		t.Fatalf("%s stale re-acquisition leaked another state machine: %+v", staleCase, aggregate)
	}
	for _, owner := range owners {
		retired := false
		for _, candidate := range wantRetired {
			retired = retired || owner == candidate
		}
		if retired {
			continue
		}
		inventory := findInventory(aggregate, owner)
		observation := findObservation(aggregate, inventory.PrerequisiteID)
		got := ownerFact{snapshotID: inventory.SnapshotID, acquisitionBinding: inventory.AcquisitionBinding, observationID: observation.ObservationID}
		if got != beforeFacts[owner] {
			t.Fatalf("%s stale changed unaffected %s owner fact: got=%+v want=%+v", staleCase, owner, got, beforeFacts[owner])
		}
	}
	beforeDuplicate := aggregate.StateRevision
	duplicate, err := biz.InvalidateRuntimeV2PrerequisitesForReadOnlyStale(ctx, "ws-v2", "run-v2", stale)
	if err != nil || duplicate.StateRevision != beforeDuplicate || duplicate.Facts.Recovery != aggregate.Facts.Recovery ||
		len(duplicate.PrerequisiteInvalidations) != len(aggregate.PrerequisiteInvalidations) {
		t.Fatalf("%s stale duplicate was not a no-op: duplicate=%+v err=%v", staleCase, duplicate, err)
	}
}

func TestRuntimeV2FileBackedGitSnapshotCommitResponseLossRestartReusesCommittedSnapshot(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	root := t.TempDir()
	deliverable, internalDir := filepath.Join(root, "workspace"), filepath.Join(root, "internal")
	if err := os.MkdirAll(deliverable, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(internalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeRuntimeV2GitFixtureFile(t, deliverable)
	bootstrap := runtimeV2GitCapabilityStateBootstrap()
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2", DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, State: common.RunStateActive}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	startRuntimeV2ReceiptTestOwner(t, hub)
	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	startRuntimeV2FileManifestOwner(t, ctx, hub)
	storeRoot := filepath.Join(root, "git-owner-data")
	store, err := gitbiz.NewCapabilityStateReceiptStore(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	gitOwner := gitmodule.New()
	if err := gitOwner.Configure(gitmodule.Dependencies{EventHub: hub, RuntimeAdapter: gitbiz.NewRuntimeAdapter(true), CapabilityStateReceipts: store}); err != nil {
		t.Fatal(err)
	}
	if err := gitOwner.Start(); err != nil {
		t.Fatal(err)
	}

	// This is the durable boundary under test: all prerequisite snapshots have
	// committed, while no candidate attempt/dispatch has been persisted yet.
	// The panic models a lost caller response after the Run CAS, so recovery
	// cannot rely on an in-process aggregate or owner responder.
	initializer := newRuntimeV2DriveTestModule(t, hub)
	run, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err := initializer.bizPtr.InitializeRuntimeV2FromRun(ctx, run)
	if err != nil {
		t.Fatal(err)
	}
	baseRepository := runtimeV2StateRepository{hub: hub}
	crashRepository := &runtimeV2SaveCrashRepository{
		delegate: baseRepository, after: true, marker: "git-snapshot-response-loss",
		match: func(current runtimev2.RuntimeAggregate) bool {
			return current.Decision.Action == runtimecontract.RuntimeActionProduceStateChange &&
				len(current.Facts.Resources.CapabilityStates) == 1 && len(current.CandidateAttempts) == 0 && current.ActiveDispatch == nil
		},
	}
	crashingBiz := orchestratorbiz.NewRuntimeOrchestrator()
	if err := crashingBiz.ConfigureRuntimeV2Repository(crashRepository); err != nil {
		t.Fatal(err)
	}
	crashingModule := newTestBiz(crashingBiz, hub, nil)
	func() {
		defer func() {
			if recovered := recover(); recovered != "git-snapshot-response-loss" {
				t.Fatalf("unexpected snapshot response-loss marker: %v", recovered)
			}
		}()
		_, _ = crashingModule.prepareRuntimeV2Prerequisites(ctx, aggregate)
		t.Fatal("expected injected snapshot response loss")
	}()
	if !crashRepository.fired {
		t.Fatal("snapshot response-loss CAS point was not reached")
	}
	committedRun, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || committedRun.RuntimeState == nil || committedRun.RuntimeState.Decision.Action != runtimecontract.RuntimeActionProduceStateChange ||
		len(committedRun.RuntimeState.Facts.Resources.CapabilityStates) != 1 || len(committedRun.RuntimeState.CandidateAttempts) != 0 || committedRun.RuntimeState.ActiveDispatch != nil {
		t.Fatalf("snapshot/candidate response-loss boundary run=%+v err=%v", committedRun, err)
	}
	committedSnapshot := committedRun.RuntimeState.Facts.Resources.CapabilityStates[0]
	if queried, found, err := store.Load("ws-v2", committedSnapshot.AcquisitionKey); err != nil || !found || queried.SnapshotID != committedSnapshot.SnapshotID {
		t.Fatalf("committed snapshot has no durable owner query receipt: found=%t queried=%+v err=%v", found, queried, err)
	}

	// Simulate a complete process loss after the Run aggregate CAS. Rebuild the
	// Run adapter, EventHub and every prerequisite owner from durable stores;
	// no prerequisite query may be scheduled because this snapshot is already
	// authoritative.
	if err := gitOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	hub.Terminate(ctx)
	restartedHub := magicevent.NewHub(32)
	t.Cleanup(func() { restartedHub.Terminate(ctx) })
	restartedRuns := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	restartedRunOwner := runmodule.New()
	if err := restartedRunOwner.Configure(runmodule.Dependencies{Runs: restartedRuns, EventHub: restartedHub}); err != nil {
		t.Fatal(err)
	}
	if err := restartedRunOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedRunOwner.Stop(ctx) })
	startRuntimeV2ReceiptTestOwner(t, restartedHub)
	restartedWorkspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, restartedHub)
	restartedWorkspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
	})
	t.Cleanup(func() { restartedWorkspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	startRuntimeV2FileManifestOwner(t, ctx, restartedHub)
	reopenedStore, err := gitbiz.NewCapabilityStateReceiptStore(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	restartedGitOwner := gitmodule.New()
	if err := restartedGitOwner.Configure(gitmodule.Dependencies{EventHub: restartedHub, RuntimeAdapter: gitbiz.NewRuntimeAdapter(true), CapabilityStateReceipts: reopenedStore}); err != nil {
		t.Fatal(err)
	}
	if err := restartedGitOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedGitOwner.Stop(ctx) })
	candidateCalls := 0
	firstCandidateReusedSnapshot := false
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, restartedHub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		candidateCalls++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		result.Set(runtimeV2CandidateResultForTest(t, command, runtimeV2GitStateCandidate(requiredOperationRefs(command.Decision.RequiredOutput.RequiredOperationSequence))), nil)
		if candidateCalls == 1 && (len(command.CandidateContext.Resources.CapabilityStates) != 1 || command.CandidateContext.Resources.CapabilityStates[0].SnapshotID != committedSnapshot.SnapshotID ||
			command.CandidateContext.Resources.CapabilityStates[0].AcquisitionKey != committedSnapshot.AcquisitionKey) {
			t.Errorf("candidate did not reuse committed capability snapshot: %+v", command.CandidateContext.Resources.CapabilityStates)
			return
		}
		if candidateCalls == 1 {
			firstCandidateReusedSnapshot = true
		}
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	driveObserver := magicevent.NewSimpleObserver(ModuleID, restartedHub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, newRuntimeV2DriveTestModule(t, restartedHub).handleRuntimeV2Drive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2) })
	completed, err := DriveRuntimeV2(ctx, restartedHub, orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"})
	if err != nil || completed.Action != runtimecontract.RuntimeActionComplete || candidateCalls != 3 || !firstCandidateReusedSnapshot {
		t.Fatalf("restart after snapshot commit completed=%+v candidateCalls=%d err=%v", completed, candidateCalls, err)
	}
	persisted, err := restartedRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.RuntimeState == nil || len(persisted.RuntimeState.Facts.Resources.CapabilityStates) != 1 || len(persisted.RuntimeState.OperationReceipts) != 3 {
		t.Fatalf("persisted state=%+v err=%v", persisted, err)
	}
}

func TestDriveRuntimeV2FileBackedFreshFileShellGitAcceptance(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	ctx := context.Background()
	hub := magicevent.NewHub(64)
	t.Cleanup(func() { hub.Terminate(ctx) })
	root := t.TempDir()
	deliverable, internalDir := filepath.Join(root, "workspace"), filepath.Join(root, "internal")
	if err := os.MkdirAll(deliverable, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(internalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	workspaces := filerepository.NewWorkspaceRepository(filepath.Join(root, "workspaces.json"))
	results := filerepository.NewResultRepository(filepath.Join(root, "results.json"))
	artifacts := filerepository.NewArtifactRepository(filepath.Join(root, "artifacts.json"))
	bootstrap := runtimeV2FileShellGitBootstrap()
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2",
		DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, State: common.RunStateActive,
	}); err != nil {
		t.Fatal(err)
	}
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-v2", State: common.WorkspaceStateActive, ActiveRunID: "run-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	startRuntimeV2ReceiptTestOwner(t, hub)
	workspaceOwner := workspacemodule.New()
	if err := workspaceOwner.Configure(workspacemodule.Dependencies{Workspaces: workspaces, Goals: memory.NewGoalRepository(), DoDs: memory.NewDoDRepository(), Templates: memory.NewWorkspaceTemplateRepository(), EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := workspaceOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = workspaceOwner.Stop(ctx) })
	fileOwner := filemodule.New()
	if err := fileOwner.Configure(filemodule.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := fileOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fileOwner.Stop(ctx) })
	shellOwner := shellmodule.New()
	if err := shellOwner.Configure(shellmodule.Dependencies{EventHub: hub, RuntimeAdapter: shellbiz.NewRuntimeAdapter()}); err != nil {
		t.Fatal(err)
	}
	if err := shellOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = shellOwner.Stop(ctx) })
	gitReceipts, err := gitbiz.NewCapabilityStateReceiptStore(filepath.Join(root, "git-owner-data"))
	if err != nil {
		t.Fatal(err)
	}
	gitOwner := gitmodule.New()
	if err := gitOwner.Configure(gitmodule.Dependencies{EventHub: hub, RuntimeAdapter: gitbiz.NewRuntimeAdapter(true), CapabilityStateReceipts: gitReceipts}); err != nil {
		t.Fatal(err)
	}
	if err := gitOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = gitOwner.Stop(ctx) })
	resultOwner := resultmodule.New()
	if err := resultOwner.Configure(resultmodule.Dependencies{Results: results, Artifacts: artifacts, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := resultOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resultOwner.Stop(ctx) })

	candidateAttempts := map[string][]int{}
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		focusID, action := command.Decision.Focus.ID, command.Decision.Action
		candidateAttempts[focusID+"/"+string(action)] = append(candidateAttempts[focusID+"/"+string(action)], command.CandidateAttempt)
		candidate, candidateErr := runtimeV2FileShellGitCandidate(command)
		if candidateErr != nil {
			t.Errorf("candidate for %s/%s: %v", focusID, action, candidateErr)
			failure := runtimecontract.RuntimeCandidateFailure{Class: runtimecontract.RuntimeCandidateFailureClassInternal, Reason: candidateErr.Error()}
			result.Set(orchestratorevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, LogicalRequestID: command.LogicalRequestID, RequestSequence: command.RequestSequence, PhysicalCallID: command.PhysicalCallID, Failure: &failure}, nil)
			return
		}
		result.Set(runtimeV2CandidateResultForTest(t, command, candidate), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	module := newRuntimeV2DriveTestModule(t, hub)
	driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, module.handleRuntimeV2Drive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2) })
	completed, err := DriveRuntimeV2(ctx, hub, orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"})
	if err != nil || completed.Action != runtimecontract.RuntimeActionComplete || completed.Terminal == nil {
		current, loadErr := module.bizPtr.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
		t.Fatalf("file-shell-git drive completion=%+v terminal=%+v aggregate=%+v loadErr=%v err=%v", completed, completed.Terminal, current, loadErr, err)
	}
	if content, readErr := os.ReadFile(filepath.Join(deliverable, "main.go")); readErr != nil || string(content) != "package main\n// edited\n" {
		t.Fatalf("fresh File sequence content=%q err=%v", string(content), readErr)
	}
	t.Logf("file-shell-git candidate attempts: %+v", candidateAttempts)
	if got := strings.TrimSpace(runtimeV2GitOutput(t, deliverable, "rev-list", "--count", "HEAD")); got != "1" {
		t.Fatalf("fresh Git sequence commit count=%q", got)
	}
	persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.RuntimeState == nil {
		t.Fatalf("persisted file-shell-git run=%+v err=%v", persisted, err)
	}
	state := persisted.RuntimeState
	if state.Facts.Recovery.PrerequisiteRefreshes != 0 || state.Facts.Recovery.PrerequisiteResourceChangeRefreshes < 4 || len(state.PrerequisiteInvalidations) < 4 {
		t.Fatalf("durable effects did not trigger persisted freshness re-acquisition: recovery=%+v invalidations=%+v", state.Facts.Recovery, state.PrerequisiteInvalidations)
	}
	if state.Facts.Recovery.ProviderTransportAttempts != 0 || state.Facts.Failure != nil {
		t.Fatalf("freshness path consumed unrelated budget or left failure: recovery=%+v failure=%+v", state.Facts.Recovery, state.Facts.Failure)
	}
	for key, attempts := range candidateAttempts {
		for _, attempt := range attempts {
			if attempt != 0 {
				t.Fatalf("fresh authority transaction %s started candidate attempt %d, want 0", key, attempt)
			}
		}
	}
	if attempts := candidateAttempts["focus-git/"+string(runtimecontract.RuntimeActionProduceStateChange)]; len(attempts) < 3 {
		t.Fatalf("Git init/add/commit did not use refreshed transactions: %+v", candidateAttempts)
	}
	for _, receipt := range state.OperationReceipts {
		if receipt.PrerequisiteStale != nil || receipt.Outcome == runtimecontract.RuntimeOperationOutcomeRejected {
			t.Fatalf("fresh positive path produced stale/rejected receipt: %+v", receipt)
		}
	}
	aggregate, err := module.bizPtr.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	delivery := newTestBiz(orchestratorbiz.NewRuntimeOrchestrator(), hub, nil)
	if err := delivery.deliverRuntimeV2Terminal(ctx, aggregate); err != nil {
		t.Fatal(err)
	}
	resultItem, err := results.Get(ctx, "ws-v2/run-v2/result/dod-assessment")
	if err != nil || resultItem.State != common.ResultStateAccepted {
		t.Fatalf("accepted Result=%+v err=%v", resultItem, err)
	}
	terminalRun, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || terminalRun.State != common.RunStateCompleted {
		t.Fatalf("terminal Run=%+v err=%v", terminalRun, err)
	}
	workspace, err := workspaces.Get(ctx, "ws-v2")
	if err != nil || workspace.ActiveRunID != "" {
		t.Fatalf("terminal Workspace=%+v err=%v", workspace, err)
	}
}

func runtimeV2FileShellGitBootstrap() runtimecontract.RuntimeBootstrap {
	bootstrap := runtimeV2Bootstrap()
	// This fixture deliberately performs five distinct durable effects
	// (create, edit, init, add, commit). Each can invalidate a mutable
	// prerequisite. The activation-sealed resource-change allowance covers this
	// normal progressing path; stale/no-progress recovery remains independent.
	bootstrap.RecoveryPolicy.MaxPrerequisiteResourceChangeRefreshes = 8
	bootstrap.Definition.CapabilityContract = []string{
		runtimecontract.StepKindFileWrite, runtimecontract.StepKindFileEdit,
		runtimecontract.RuntimeBlockKindManagedValidationSession, runtimecontract.StepKindGitWrite,
	}
	bootstrap.Focuses = []runtimecontract.CoverageFocus{
		{ID: "focus-file", SliceID: "slice-file", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-file"}, TargetRefs: []string{"main.go"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}, AcceptedStepKinds: []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindManagedValidationSession}},
		{ID: "focus-edit", SliceID: "slice-edit", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-edit"}, TargetRefs: []string{"main.go"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, AcceptedStepKinds: []string{runtimecontract.StepKindFileEdit}},
		{ID: "focus-git", SliceID: "slice-git", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-git"}, TargetRefs: []string{"main.go"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}, AcceptedStepKinds: []string{runtimecontract.StepKindGitWrite, runtimecontract.RuntimeBlockKindManagedValidationSession}, RequireAssessment: true},
	}
	bootstrap.Slices = []runtimecontract.RuntimeAcceptanceSlice{
		{ID: "slice-file", Title: "file", Scope: "fixture", Priority: 1, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-file"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}, CompletionPolicy: runtimecontract.RuntimeAcceptanceSliceCompletionAll, ResultPolicy: runtimecontract.RuntimeAcceptanceSliceResultAllowed},
		{ID: "slice-edit", Title: "edit", Scope: "fixture", Priority: 1, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-edit"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, CompletionPolicy: runtimecontract.RuntimeAcceptanceSliceCompletionAll, ResultPolicy: runtimecontract.RuntimeAcceptanceSliceResultAllowed},
		{ID: "slice-git", Title: "git", Scope: "fixture", Priority: 1, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-git"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}, CompletionPolicy: runtimecontract.RuntimeAcceptanceSliceCompletionAll, ResultPolicy: runtimecontract.RuntimeAcceptanceSliceResultAllowed},
	}
	bootstrap.CoverageItems = []runtimecontract.RuntimeCoverageItem{{ID: "cwi-file", SliceID: "slice-file"}, {ID: "cwi-edit", SliceID: "slice-edit"}, {ID: "cwi-git", SliceID: "slice-git"}}
	bootstrap.Requirements = []runtimecontract.RuntimeRequirement{
		{Kind: runtimecontract.PrerequisiteKindManifest, OwnerModuleID: runtimecontract.RuntimeModuleFile, Reason: "workspace manifest"},
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleFile, CapabilityClass: runtimecontract.RuntimeCapabilityClassStateChange, RequiredCapabilities: []string{runtimecontract.StepKindFileWrite, runtimecontract.StepKindFileEdit}},
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleShell, CapabilityClass: runtimecontract.RuntimeCapabilityClassValidation, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}},
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleGit, CapabilityClass: runtimecontract.RuntimeCapabilityClassStateChange, RequiredCapabilities: []string{runtimecontract.StepKindGitWrite}},
		{Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleFile, RequiredCapabilities: []string{runtimecontract.StepKindFileWrite, runtimecontract.StepKindFileEdit}, Reason: "file owner inventory"},
		{Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleShell, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, Reason: "shell validation inventory"},
		{Kind: runtimecontract.PrerequisiteKindCapabilityState, OwnerModuleID: runtimecontract.RuntimeModuleGit, CapabilityStateKind: "repository_state", SubjectRef: "workspace_deliverable", Reason: "git sequence"},
		// Every executable capability needs its owner-authored admissibility
		// prerequisite. Git's inventory is bound to the capability-state receipt
		// above and must be invalidated/reacquired after the File tree changes;
		// omitting it would let the fixture generate git/write without its sealed
		// owner inventory, a bootstrap-contract violation rather than freshness.
		{Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleGit, RequiredCapabilities: []string{runtimecontract.StepKindGitWrite}, Reason: "git owner inventory"},
	}
	return bootstrap
}

func runtimeV2FileShellGitCandidate(command orchestratorevents.RequestRuntimeCandidateCommand) (runtimecontract.RuntimeCandidate, error) {
	focus := command.Decision.Focus
	coverageID := focus.CoverageWorkItemIDs[0]
	switch focus.ID {
	case "focus-file":
		switch command.Decision.Action {
		case runtimecontract.RuntimeActionProduceStateChange:
			target, content := "main.go", "package main\n"
			return runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{ID: "file-create", Kind: runtimecontract.StepKindFileWrite, CoverageWorkItemIDs: []common.CoverageWorkItemID{coverageID}, TargetRefs: []string{target}, Parameters: []runtimecontract.OperationParameter{{Name: "content", String: &content}}}}}}}, nil
		case runtimecontract.RuntimeActionProduceValidation:
			return runtimeV2ManagedShellValidationCandidate("validate-created", coverageID, "main.go", "package main"), nil
		}
	case "focus-edit":
		if command.Decision.Action == runtimecontract.RuntimeActionProduceStateChange {
			path, oldText, newText := "main.go", "package main\n", "package main\n// edited\n"
			return runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{ID: "file-edit", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{coverageID}, TargetRefs: []string{path}, Parameters: []runtimecontract.OperationParameter{{Name: "oldText", String: &oldText}, {Name: "newText", String: &newText}}}}}}}, nil
		}
	case "focus-git":
		switch command.Decision.Action {
		case runtimecontract.RuntimeActionProduceStateChange:
			return runtimeV2GitStateCandidateForCoverage(requiredOperationRefs(command.Decision.RequiredOutput.RequiredOperationSequence), coverageID, "main.go"), nil
		case runtimecontract.RuntimeActionProduceValidation:
			return runtimeV2ManagedShellValidationCandidate("validate-committed", coverageID, "main.go", "edited"), nil
		case runtimecontract.RuntimeActionProduceAssessment:
			refs := make([]string, 0)
			for _, evidence := range command.CandidateContext.Evidence.Accepted {
				for _, evidenceCoverageID := range evidence.CoverageWorkItemIDs {
					if evidenceCoverageID == coverageID {
						refs = append(refs, string(evidence.ID))
						break
					}
				}
			}
			return runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{FocusID: focus.ID, CoverageWorkItemIDs: []common.CoverageWorkItemID{coverageID}, Disposition: runtimecontract.AssessmentDispositionSatisfied, Summary: "git sequence validated", Items: []runtimecontract.RuntimeAssessmentItemFact{{CoverageWorkItemID: coverageID, Satisfied: true}}, EvidenceRefs: refs}}, nil
		}
	}
	return runtimecontract.RuntimeCandidate{}, fmt.Errorf("unexpected focus/action %s/%s", focus.ID, command.Decision.Action)
}

func runtimeV2ManagedShellValidationCandidate(id string, coverageID common.CoverageWorkItemID, target, want string) runtimecontract.RuntimeCandidate {
	command, script := "sh", "test -f "+target+" && grep -q '"+want+"' "+target
	return runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: id, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession, CoverageWorkItemIDs: []common.CoverageWorkItemID{coverageID}, TargetRefs: []string{target},
		Parameters: []runtimecontract.OperationParameter{{Name: "command", String: &command}, {Name: "args", StringList: []string{"-c", script}}},
	}}}}}
}

func runtimeV2GitStateCandidateForCoverage(requirementRefs []string, coverageID common.CoverageWorkItemID, target string) runtimecontract.RuntimeCandidate {
	operations := make([]runtimecontract.StateChangingOperation, 0, len(requirementRefs))
	for _, ref := range requirementRefs {
		operation := runtimecontract.RuntimeOperation{ID: "fresh-git-" + ref, Kind: runtimecontract.StepKindGitWrite, RequiredOperationRef: ref, CoverageWorkItemIDs: []common.CoverageWorkItemID{coverageID}, TargetRefs: []string{target}}
		switch ref {
		case "repo-init":
			value := "init"
			operation.Parameters = []runtimecontract.OperationParameter{{Name: "operation", String: &value}}
		case "stage-changes":
			value := "add"
			operation.Parameters = []runtimecontract.OperationParameter{{Name: "operation", String: &value}, {Name: "pathspecs", StringList: []string{target}}}
		case "create-commit":
			value, message := "commit", "fresh runtime v2 fixture"
			operation.Parameters = []runtimecontract.OperationParameter{{Name: "operation", String: &value}, {Name: "message", String: &message}}
		}
		switch ref {
		case "stage-changes":
			if containsRuntimeV2String(requirementRefs, "repo-init") {
				operation.DependsOn = []string{"fresh-git-repo-init"}
			}
		case "create-commit":
			if containsRuntimeV2String(requirementRefs, "stage-changes") {
				operation.DependsOn = []string{"fresh-git-stage-changes"}
			}
		}
		operations = append(operations, runtimecontract.StateChangingOperation{Operation: operation})
	}
	return runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: operations}}
}

func TestRuntimeV2FileBackedGitStaleRefreshCrashRestartDoesNotReplayOldDispatch(t *testing.T) {
	runRuntimeV2FileBackedGitStaleRefreshCrashRestart(t, true)
}

func TestRuntimeV2FileBackedGitStaleRefreshBeforeCASRestartDoesNotLeakInvalidation(t *testing.T) {
	runRuntimeV2FileBackedGitStaleRefreshCrashRestart(t, false)
}

func runRuntimeV2FileBackedGitStaleRefreshCrashRestart(t *testing.T, crashAfterCAS bool) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	root := t.TempDir()
	deliverable, internalDir := filepath.Join(root, "workspace"), filepath.Join(root, "internal")
	if err := os.MkdirAll(deliverable, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(internalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeRuntimeV2GitFixtureFile(t, deliverable)
	bootstrap := runtimeV2GitCapabilityStateBootstrap()
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2", DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, State: common.RunStateActive}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	startRuntimeV2ReceiptTestOwner(t, hub)
	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	startRuntimeV2FileManifestOwner(t, ctx, hub)
	storeRoot := filepath.Join(root, "git-owner-data")
	store, err := gitbiz.NewCapabilityStateReceiptStore(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	gitOwner := gitmodule.New()
	if err := gitOwner.Configure(gitmodule.Dependencies{EventHub: hub, RuntimeAdapter: gitbiz.NewRuntimeAdapter(true), CapabilityStateReceipts: store}); err != nil {
		t.Fatal(err)
	}
	if err := gitOwner.Start(); err != nil {
		t.Fatal(err)
	}

	module := newRuntimeV2DriveTestModule(t, hub)
	run, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err := module.bizPtr.InitializeRuntimeV2FromRun(ctx, run)
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err = module.prepareRuntimeV2Prerequisites(ctx, aggregate)
	if err != nil {
		t.Fatal(err)
	}
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(aggregate.Definition, aggregate.Facts.Activation.ExecutionContract)
	if _, err := module.bizPtr.BeginRuntimeV2CandidateAttemptForTest(ctx, "ws-v2", "run-v2", 0, prefix); err != nil {
		t.Fatal(err)
	}
	if _, err := module.bizPtr.AcceptRuntimeV2CandidateAttempt(ctx, "ws-v2", "run-v2", 0, runtimeV2GitStateCandidate([]string{"create-commit", "repo-init", "stage-changes"})); err != nil {
		t.Fatal(err)
	}

	// External owner state changes after the sealed absent-repository snapshot
	// and before the first operation. The old init must become a typed stale
	// receipt, never a replayable execution failure or an ordinary repair.
	runRuntimeV2Git(t, deliverable, "init", "-q")
	runRuntimeV2Git(t, deliverable, "config", "user.name", "WorkOrch Test")
	runRuntimeV2Git(t, deliverable, "config", "user.email", "workorch@example.test")
	baseRepository := runtimeV2StateRepository{hub: hub}
	crashMarker := "git-stale-refresh-before-cas"
	if crashAfterCAS {
		crashMarker = "git-stale-refresh-after-cas"
	}
	crashRepository := &runtimeV2SaveCrashRepository{
		delegate: baseRepository, after: crashAfterCAS, marker: crashMarker,
		match: func(current runtimev2.RuntimeAggregate) bool {
			return current.ActiveDispatch == nil && current.Decision.Action == runtimecontract.RuntimeActionAcquirePrerequisite &&
				current.Facts.Recovery.PrerequisiteRefreshes == 1 && len(current.Facts.Resources.CapabilityStates) == 0
		},
	}
	crashingController, err := runtimev2.NewRuntimeController(crashRepository)
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != crashMarker {
				t.Fatalf("unexpected stale-refresh crash marker: %v", recovered)
			}
		}()
		_, _ = crashingController.ExecuteActiveDispatch(ctx, "ws-v2", "run-v2", eventRuntimeV2OperationExecutor{hub: hub})
		t.Fatal("expected injected stale-refresh crash")
	}()
	crashed, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || crashed.RuntimeState == nil || crashed.RuntimeState.EvidenceSequence != 0 {
		t.Fatalf("stale refresh crash boundary=%+v err=%v", crashed, err)
	}
	if crashAfterCAS {
		if crashed.RuntimeState.ActiveDispatch != nil || crashed.RuntimeState.Facts.Recovery.PrerequisiteRefreshes != 1 ||
			len(crashed.RuntimeState.Facts.Resources.CapabilityStates) != 0 || len(crashed.RuntimeState.OperationReceipts) != 1 ||
			crashed.RuntimeState.OperationReceipts[0].PrerequisiteStale == nil || len(crashed.RuntimeState.PrerequisiteInvalidations) != 1 {
			t.Fatalf("after-CAS stale invalidation was not committed once: %+v", crashed.RuntimeState)
		}
	} else if crashed.RuntimeState.ActiveDispatch == nil || crashed.RuntimeState.Facts.Recovery.PrerequisiteRefreshes != 0 ||
		len(crashed.RuntimeState.Facts.Resources.CapabilityStates) != 1 || len(crashed.RuntimeState.OperationReceipts) != 1 ||
		crashed.RuntimeState.OperationReceipts[0].PrerequisiteStale == nil || len(crashed.RuntimeState.PrerequisiteInvalidations) != 0 {
		t.Fatalf("before-CAS stale invalidation leaked uncommitted authority: %+v", crashed.RuntimeState)
	}
	if err := gitOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	hub.Terminate(ctx)
	restartedHub := magicevent.NewHub(32)
	t.Cleanup(func() { restartedHub.Terminate(ctx) })
	restartedRuns := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	restartedRunOwner := runmodule.New()
	if err := restartedRunOwner.Configure(runmodule.Dependencies{Runs: restartedRuns, EventHub: restartedHub}); err != nil {
		t.Fatal(err)
	}
	if err := restartedRunOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedRunOwner.Stop(ctx) })
	startRuntimeV2ReceiptTestOwner(t, restartedHub)
	restartedWorkspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, restartedHub)
	restartedWorkspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
	})
	t.Cleanup(func() { restartedWorkspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	startRuntimeV2FileManifestOwner(t, ctx, restartedHub)
	reopenedStore, err := gitbiz.NewCapabilityStateReceiptStore(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	restartedGitOwner := gitmodule.New()
	if err := restartedGitOwner.Configure(gitmodule.Dependencies{EventHub: restartedHub, RuntimeAdapter: gitbiz.NewRuntimeAdapter(true), CapabilityStateReceipts: reopenedStore}); err != nil {
		t.Fatal(err)
	}
	if err := restartedGitOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedGitOwner.Stop(ctx) })
	candidateCalls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, restartedHub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		candidateCalls++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		refs := requiredOperationRefs(command.Decision.RequiredOutput.RequiredOperationSequence)
		result.Set(runtimeV2CandidateResultForTest(t, command, runtimeV2GitStateCandidate(refs)), nil)
		expected := []string{"create-commit", "stage-changes"}
		if candidateCalls == 2 {
			expected = []string{"create-commit"}
		}
		if !equalStrings(refs, expected) {
			t.Errorf("stale refresh used wrong required sequence on candidate %d: %v", candidateCalls, refs)
		}
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	driveObserver := magicevent.NewSimpleObserver(ModuleID, restartedHub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, newRuntimeV2DriveTestModule(t, restartedHub).handleRuntimeV2Drive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2) })
	completed, err := DriveRuntimeV2(ctx, restartedHub, orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"})
	if err != nil || completed.Action != runtimecontract.RuntimeActionComplete || candidateCalls != 2 {
		t.Fatalf("stale refresh restart completed=%+v candidateCalls=%d err=%v", completed, candidateCalls, err)
	}
	persisted, err := restartedRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.RuntimeState == nil || len(persisted.RuntimeState.OperationReceipts) != 3 {
		t.Fatalf("stale refresh restart persisted receipts=%+v err=%v", persisted.RuntimeState.OperationReceipts, err)
	}
	initReceipts := 0
	for _, receipt := range persisted.RuntimeState.OperationReceipts {
		if receipt.OperationID == "git-repo-init" {
			initReceipts++
			if receipt.Outcome != runtimecontract.RuntimeOperationOutcomeRejected || receipt.PrerequisiteStale == nil {
				t.Fatalf("old init was replayed or reclassified: %+v", receipt)
			}
		}
	}
	if initReceipts != 1 || strings.TrimSpace(runtimeV2GitOutput(t, deliverable, "rev-list", "--count", "HEAD")) != "1" {
		t.Fatalf("old init dispatch was replayed, receipts=%d", initReceipts)
	}
}

// An accepted owner receipt that changed a prerequisite resource is not a
// read-only stale refresh. Its invalidation consumes only the separate
// resource-change ledger and must never replay the durable operation that
// produced the receipt, regardless of which side of the invalidation CAS
// crashed.
func TestRuntimeV2FileBackedGitReceiptResourceChangeRefreshCrashRestartDoesNotReplayReceipt(t *testing.T) {
	runRuntimeV2FileBackedGitReceiptResourceChangeRefreshCrashRestart(t, true, 0, false)
}

func TestRuntimeV2FileBackedGitReceiptResourceChangeRefreshBeforeCASRestartUsesCommittedReceipt(t *testing.T) {
	runRuntimeV2FileBackedGitReceiptResourceChangeRefreshCrashRestart(t, false, 0, false)
}

// Once the receipt-resource-change ledger is exhausted, a later accepted
// owner receipt must terminalize in SM-2.  It must not borrow the read-only
// stale budget or submit a further candidate to compensate for the terminal.
func TestRuntimeV2FileBackedGitReceiptResourceChangeRefreshExhaustionIsTerminal(t *testing.T) {
	for _, crashAfterTerminalCAS := range []bool{false, true} {
		phase := "terminal_before_cas"
		if crashAfterTerminalCAS {
			phase = "terminal_after_cas"
		}
		t.Run(phase, func(t *testing.T) {
			runRuntimeV2FileBackedGitReceiptResourceChangeRefreshCrashRestart(t, crashAfterTerminalCAS, 1, true)
		})
	}
}

func runRuntimeV2FileBackedGitReceiptResourceChangeRefreshCrashRestart(t *testing.T, crashAfterCAS bool, maxReceiptRefreshes int, crashTerminal bool) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	root := t.TempDir()
	deliverable, internalDir := filepath.Join(root, "workspace"), filepath.Join(root, "internal")
	if err := os.MkdirAll(deliverable, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(internalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeRuntimeV2GitFixtureFile(t, deliverable)
	bootstrap := runtimeV2GitCapabilityStateBootstrap()
	if maxReceiptRefreshes > 0 {
		bootstrap.RecoveryPolicy.MaxPrerequisiteResourceChangeRefreshes = maxReceiptRefreshes
	}
	runsPath := filepath.Join(root, "runs.json")
	runs := filerepository.NewRunRepository(runsPath)
	if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2", DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, State: common.RunStateActive}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	startRuntimeV2ReceiptTestOwner(t, hub)
	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	startRuntimeV2FileManifestOwner(t, ctx, hub)
	storeRoot := filepath.Join(root, "git-owner-data")
	store, err := gitbiz.NewCapabilityStateReceiptStore(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	gitOwner := gitmodule.New()
	if err := gitOwner.Configure(gitmodule.Dependencies{EventHub: hub, RuntimeAdapter: gitbiz.NewRuntimeAdapter(true), CapabilityStateReceipts: store}); err != nil {
		t.Fatal(err)
	}
	if err := gitOwner.Start(); err != nil {
		t.Fatal(err)
	}

	module := newRuntimeV2DriveTestModule(t, hub)
	run, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err := module.bizPtr.InitializeRuntimeV2FromRun(ctx, run)
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err = module.prepareRuntimeV2Prerequisites(ctx, aggregate)
	if err != nil {
		t.Fatal(err)
	}
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(aggregate.Definition, aggregate.Facts.Activation.ExecutionContract)
	if _, err := module.bizPtr.BeginRuntimeV2CandidateAttemptForTest(ctx, "ws-v2", "run-v2", 0, prefix); err != nil {
		t.Fatal(err)
	}
	if _, err := module.bizPtr.AcceptRuntimeV2CandidateAttempt(ctx, "ws-v2", "run-v2", 0, runtimeV2GitStateCandidate([]string{"create-commit", "repo-init", "stage-changes"})); err != nil {
		t.Fatal(err)
	}

	baseRepository := runtimeV2StateRepository{hub: hub}
	crashMarker := "git-receipt-resource-change-refresh-before-cas"
	if crashAfterCAS {
		crashMarker = "git-receipt-resource-change-refresh-after-cas"
	}
	crashRepository := &runtimeV2SaveCrashRepository{
		delegate: baseRepository, after: crashAfterCAS, marker: crashMarker,
		match: func(current runtimev2.RuntimeAggregate) bool {
			return current.ActiveDispatch == nil && current.Decision.Action == runtimecontract.RuntimeActionAcquirePrerequisite &&
				current.Facts.Recovery.PrerequisiteRefreshes == 0 && current.Facts.Recovery.PrerequisiteResourceChangeRefreshes == 1 &&
				len(current.Facts.Resources.CapabilityStates) == 0 && len(current.OperationReceipts) == 1 && current.EvidenceSequence == 1
		},
	}
	crashingController, err := runtimev2.NewRuntimeController(crashRepository)
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != crashMarker {
				t.Fatalf("unexpected resource-change crash marker: %v", recovered)
			}
		}()
		_, _ = crashingController.ExecuteActiveDispatch(ctx, "ws-v2", "run-v2", eventRuntimeV2OperationExecutor{hub: hub})
		t.Fatal("expected injected receipt resource-change crash")
	}()
	if !crashRepository.fired {
		t.Fatal("resource-change refresh CAS point was not reached")
	}
	crashed, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || crashed.RuntimeState == nil || len(crashed.RuntimeState.OperationReceipts) != 1 ||
		crashed.RuntimeState.OperationReceipts[0].Outcome != runtimecontract.RuntimeOperationOutcomeAccepted {
		t.Fatalf("resource-change crash boundary=%+v err=%v", crashed, err)
	}
	initReceipt := crashed.RuntimeState.OperationReceipts[0]
	if crashAfterCAS {
		if crashed.RuntimeState.ActiveDispatch != nil || crashed.RuntimeState.Facts.Recovery.PrerequisiteRefreshes != 0 ||
			crashed.RuntimeState.Facts.Recovery.PrerequisiteResourceChangeRefreshes != 1 || len(crashed.RuntimeState.Facts.Resources.CapabilityStates) != 0 ||
			crashed.RuntimeState.EvidenceSequence != 1 || len(crashed.RuntimeState.PrerequisiteInvalidations) != 1 ||
			crashed.RuntimeState.PrerequisiteInvalidations[0].ReceiptHash == "" {
			t.Fatalf("after-CAS resource-change invalidation was not committed exactly once: %+v", crashed.RuntimeState)
		}
	} else if crashed.RuntimeState.ActiveDispatch == nil || crashed.RuntimeState.Facts.Recovery.PrerequisiteRefreshes != 0 ||
		crashed.RuntimeState.Facts.Recovery.PrerequisiteResourceChangeRefreshes != 0 || len(crashed.RuntimeState.Facts.Resources.CapabilityStates) != 1 ||
		crashed.RuntimeState.EvidenceSequence != 0 || len(crashed.RuntimeState.PrerequisiteInvalidations) != 0 {
		t.Fatalf("before-CAS resource-change invalidation leaked uncommitted authority: %+v", crashed.RuntimeState)
	}

	// Rebuild the full process. The only surviving inputs are file-backed Run
	// state, workspace files and the Git owner's durable receipt store.
	if err := gitOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	hub.Terminate(ctx)
	restartedHub := magicevent.NewHub(32)
	t.Cleanup(func() { restartedHub.Terminate(ctx) })
	restartedRuns := filerepository.NewRunRepository(runsPath)
	restartedRunOwner := runmodule.New()
	if err := restartedRunOwner.Configure(runmodule.Dependencies{Runs: restartedRuns, EventHub: restartedHub}); err != nil {
		t.Fatal(err)
	}
	if err := restartedRunOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedRunOwner.Stop(ctx) })
	startRuntimeV2ReceiptTestOwner(t, restartedHub)
	restartedWorkspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, restartedHub)
	restartedWorkspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
	})
	t.Cleanup(func() { restartedWorkspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	startRuntimeV2FileManifestOwner(t, ctx, restartedHub)
	reopenedStore, err := gitbiz.NewCapabilityStateReceiptStore(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	restartedGitOwner := gitmodule.New()
	if err := restartedGitOwner.Configure(gitmodule.Dependencies{EventHub: restartedHub, RuntimeAdapter: gitbiz.NewRuntimeAdapter(true), CapabilityStateReceipts: reopenedStore}); err != nil {
		t.Fatal(err)
	}
	if err := restartedGitOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedGitOwner.Stop(ctx) })

	candidateCalls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, restartedHub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		candidateCalls++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		result.Set(runtimeV2CandidateResultForTest(t, command, runtimeV2GitStateCandidate(requiredOperationRefs(command.Decision.RequiredOutput.RequiredOperationSequence))), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	driveObserver := magicevent.NewSimpleObserver(ModuleID, restartedHub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, newRuntimeV2DriveTestModule(t, restartedHub).handleRuntimeV2Drive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2) })
	if maxReceiptRefreshes > 0 && crashTerminal {
		marker := "git-receipt-refresh-exhaustion-terminal-before-cas"
		if crashAfterCAS {
			marker = "git-receipt-refresh-exhaustion-terminal-after-cas"
		}
		crashRepository := &runtimeV2SaveCrashRepository{
			delegate: runtimeV2StateRepository{hub: restartedHub}, after: crashAfterCAS, marker: marker,
			match: func(current runtimev2.RuntimeAggregate) bool {
				return current.Decision.Action == runtimecontract.RuntimeActionFail && current.Facts.Failure != nil &&
					current.Facts.Failure.Code == "prerequisite_receipt_refresh_exhausted" &&
					current.Facts.Recovery.PrerequisiteRefreshes == 0 &&
					current.Facts.Recovery.PrerequisiteResourceChangeRefreshes == maxReceiptRefreshes
			},
		}
		crashingOwner := orchestratorbiz.NewRuntimeOrchestrator()
		if err := crashingOwner.ConfigureRuntimeV2Repository(crashRepository); err != nil {
			t.Fatal(err)
		}
		crashing := newTestBiz(crashingOwner, restartedHub, nil)
		func() {
			defer func() {
				if recovered := recover(); recovered != marker {
					t.Fatalf("unexpected receipt-refresh exhaustion terminal crash marker: %v", recovered)
				}
			}()
			_, _ = crashing.driveRuntimeV2Locked(ctx, orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"})
			t.Fatal("expected receipt-refresh exhaustion terminal CAS crash")
		}()
		if !crashRepository.fired {
			t.Fatal("receipt-refresh exhaustion terminal CAS point was not reached")
		}
		crashed, loadErr := restartedRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
		if loadErr != nil || crashed.RuntimeState == nil {
			t.Fatalf("receipt-refresh exhaustion crash state=%+v err=%v", crashed, loadErr)
		}
		if crashAfterCAS {
			if crashed.RuntimeState.Decision.Action != runtimecontract.RuntimeActionFail || crashed.RuntimeState.Facts.Failure == nil ||
				crashed.RuntimeState.Facts.Failure.Code != "prerequisite_receipt_refresh_exhausted" ||
				crashed.RuntimeState.Facts.Recovery.PrerequisiteRefreshes != 0 ||
				crashed.RuntimeState.Facts.Recovery.PrerequisiteResourceChangeRefreshes != maxReceiptRefreshes {
				t.Fatalf("after-CAS receipt-refresh exhaustion was not terminal: %+v", crashed.RuntimeState)
			}
		} else if crashed.RuntimeState.Decision.Action == runtimecontract.RuntimeActionFail || crashed.RuntimeState.Facts.Failure != nil ||
			crashed.RuntimeState.Facts.Recovery.PrerequisiteRefreshes != 0 ||
			crashed.RuntimeState.Facts.Recovery.PrerequisiteResourceChangeRefreshes != maxReceiptRefreshes {
			t.Fatalf("before-CAS receipt-refresh exhaustion leaked terminal authority: %+v", crashed.RuntimeState)
		}
		llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate)
		driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2)
		if err := restartedGitOwner.Stop(ctx); err != nil {
			t.Fatal(err)
		}
		if err := restartedRunOwner.Stop(ctx); err != nil {
			t.Fatal(err)
		}
		restartedWorkspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
		restartedHub.Terminate(ctx)
		assertRuntimeV2GitReceiptRefreshTerminalReload(t, ctx, runsPath, storeRoot, deliverable, internalDir, maxReceiptRefreshes)
		return
	}
	completed, err := DriveRuntimeV2(ctx, restartedHub, orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"})
	if maxReceiptRefreshes > 0 {
		if err != nil || completed.Action != runtimecontract.RuntimeActionFail || candidateCalls != 1 {
			t.Fatalf("receipt-resource-change exhaustion completed=%+v candidateCalls=%d err=%v", completed, candidateCalls, err)
		}
		persisted, loadErr := restartedRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
		if loadErr != nil || persisted.RuntimeState == nil || persisted.RuntimeState.Facts.Failure == nil ||
			persisted.RuntimeState.Facts.Failure.Code != "prerequisite_receipt_refresh_exhausted" ||
			persisted.RuntimeState.Facts.Recovery.PrerequisiteRefreshes != 0 ||
			persisted.RuntimeState.Facts.Recovery.PrerequisiteResourceChangeRefreshes != maxReceiptRefreshes ||
			len(persisted.RuntimeState.OperationReceipts) != 2 || len(persisted.RuntimeState.CandidateAttempts) != 2 ||
			persisted.RuntimeState.ActiveDispatch != nil || len(persisted.RuntimeState.GovernanceRequests) != 0 {
			t.Fatalf("receipt-resource-change exhaustion persisted=%+v err=%v", persisted.RuntimeState, loadErr)
		}
		// Terminal recovery is delivery-only. Reissuing the same drive command
		// cannot request another candidate or owner operation.
		repeated, repeatErr := DriveRuntimeV2(ctx, restartedHub, orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"})
		if repeatErr != nil || repeated.Action != runtimecontract.RuntimeActionFail || candidateCalls != 1 {
			t.Fatalf("receipt-resource-change terminal re-drive looped: result=%+v candidateCalls=%d err=%v", repeated, candidateCalls, repeatErr)
		}
		return
	}
	if err != nil || completed.Action != runtimecontract.RuntimeActionComplete || candidateCalls != 2 {
		t.Fatalf("resource-change restart completed=%+v candidateCalls=%d err=%v", completed, candidateCalls, err)
	}
	persisted, err := restartedRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.RuntimeState == nil || persisted.RuntimeState.Facts.Recovery.PrerequisiteRefreshes != 0 ||
		persisted.RuntimeState.Facts.Recovery.PrerequisiteResourceChangeRefreshes != 3 {
		t.Fatalf("resource-change restart recovery=%+v err=%v", persisted.RuntimeState.Facts.Recovery, err)
	}
	initReceipts := 0
	for _, receipt := range persisted.RuntimeState.OperationReceipts {
		if receipt.OperationID != "git-repo-init" {
			continue
		}
		initReceipts++
		if receipt.ExecutionKey != initReceipt.ExecutionKey || receipt.Fence != initReceipt.Fence || receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted {
			t.Fatalf("init receipt was replayed or replaced: before=%+v after=%+v", initReceipt, receipt)
		}
	}
	if initReceipts != 1 || len(persisted.RuntimeState.PrerequisiteInvalidations) != 3 || strings.TrimSpace(runtimeV2GitOutput(t, deliverable, "rev-list", "--count", "HEAD")) != "1" {
		t.Fatalf("resource-change restart replayed init or duplicated invalidations: receipts=%d invalidations=%+v", initReceipts, persisted.RuntimeState.PrerequisiteInvalidations)
	}
}

// assertRuntimeV2GitReceiptRefreshTerminalReload starts a fourth, independent
// process after the terminal-write response loss.  It proves that SM-4 only
// delivers the persisted SM-2 terminal; it cannot request a candidate or
// execute the Git owner again.
func assertRuntimeV2GitReceiptRefreshTerminalReload(t *testing.T, ctx context.Context, runsPath, storeRoot, deliverable, internalDir string, maxReceiptRefreshes int) {
	t.Helper()
	hub := magicevent.NewHub(32)
	defer hub.Terminate(ctx)
	runs := filerepository.NewRunRepository(runsPath)
	runOwner := runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	defer runOwner.Stop(ctx)
	startRuntimeV2ReceiptTestOwner(t, hub)
	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
	})
	defer workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
	startRuntimeV2FileManifestOwner(t, ctx, hub)
	store, err := gitbiz.NewCapabilityStateReceiptStore(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	gitOwner := gitmodule.New()
	if err := gitOwner.Configure(gitmodule.Dependencies{EventHub: hub, RuntimeAdapter: gitbiz.NewRuntimeAdapter(true), CapabilityStateReceipts: store}); err != nil {
		t.Fatal(err)
	}
	if err := gitOwner.Start(); err != nil {
		t.Fatal(err)
	}
	defer gitOwner.Stop(ctx)
	candidateCalls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		candidateCalls++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		result.Set(runtimeV2CandidateResultForTest(t, command, runtimeV2GitStateCandidate(requiredOperationRefs(command.Decision.RequiredOutput.RequiredOperationSequence))), nil)
	})
	defer llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate)
	driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, newRuntimeV2DriveTestModule(t, hub).handleRuntimeV2Drive)
	defer driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2)

	completed, err := DriveRuntimeV2(ctx, hub, orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"})
	if err != nil || completed.Action != runtimecontract.RuntimeActionFail || candidateCalls != 0 {
		t.Fatalf("receipt-resource-change fresh terminal recovery=%+v candidateCalls=%d err=%v", completed, candidateCalls, err)
	}
	persisted, loadErr := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if loadErr != nil || persisted.RuntimeState == nil || persisted.RuntimeState.Facts.Failure == nil ||
		persisted.RuntimeState.Facts.Failure.Code != "prerequisite_receipt_refresh_exhausted" ||
		persisted.RuntimeState.Facts.Recovery.PrerequisiteRefreshes != 0 ||
		persisted.RuntimeState.Facts.Recovery.PrerequisiteResourceChangeRefreshes != maxReceiptRefreshes ||
		persisted.RuntimeState.ActiveDispatch != nil || len(persisted.RuntimeState.GovernanceRequests) != 0 {
		t.Fatalf("receipt-resource-change fresh persisted terminal=%+v err=%v", persisted.RuntimeState, loadErr)
	}
}

// A-5 composes the two independent freshness sources in one persisted Run.
// Git's accepted init receipt first retires its external repository-state
// observations.  Only after that committed owner transition do we lose the
// orchestrator-local validation scratch.  The second transition must consume
// the read-only ledger without replaying the already durable Git operation or
// borrowing the receipt-resource-change ledger.
func TestRuntimeV2FileBackedMixedOwnerReceiptThenLocalScratchStaleCrashRestart(t *testing.T) {
	for _, crashAfterCAS := range []bool{false, true} {
		phase := "before_cas_replays_only_local_stale"
		if crashAfterCAS {
			phase = "after_cas_reuses_local_stale"
		}
		t.Run(phase, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			deliverable, internalDirectory := filepath.Join(root, "workspace"), filepath.Join(root, "internal")
			if err := os.MkdirAll(deliverable, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(internalDirectory, 0o755); err != nil {
				t.Fatal(err)
			}
			writeRuntimeV2GitFixtureFile(t, deliverable)
			bootstrap := runtimeV2GitCapabilityStateBootstrap()
			bootstrap.Requirements = append(bootstrap.Requirements, runtimecontract.RuntimeRequirement{
				Kind: runtimecontract.PrerequisiteKindValidationScratch, OwnerModuleID: runtimecontract.RuntimeModuleOrchestrator,
				Reason: "mixed-owner validation scratch",
			})
			runsPath := filepath.Join(root, "runs.json")
			if err := filerepository.NewRunRepository(runsPath).Create(ctx, &runmodel.Run{
				ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
				DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, State: common.RunStateActive,
			}); err != nil {
				t.Fatal(err)
			}
			storeRoot := filepath.Join(root, "git-owner-data")

			type process struct {
				hub               magicevent.Hub
				runs              *filerepository.RunRepository
				runOwner          *runmodule.Module
				gitOwner          *gitmodule.Module
				workspaceObserver magicevent.SimpleObserver
			}
			startProcess := func(t *testing.T) process {
				t.Helper()
				hub := magicevent.NewHub(32)
				runs := filerepository.NewRunRepository(runsPath)
				runOwner := runmodule.New()
				if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := runOwner.Start(); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				startRuntimeV2ReceiptTestOwner(t, hub)
				workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
				workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
					result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{
						ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDirectory,
					}}, nil)
				})
				startRuntimeV2FileManifestOwner(t, ctx, hub)
				store, err := gitbiz.NewCapabilityStateReceiptStore(storeRoot)
				if err != nil {
					workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
					_ = runOwner.Stop(ctx)
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				gitOwner := gitmodule.New()
				if err := gitOwner.Configure(gitmodule.Dependencies{EventHub: hub, RuntimeAdapter: gitbiz.NewRuntimeAdapter(true), CapabilityStateReceipts: store}); err != nil {
					workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
					_ = runOwner.Stop(ctx)
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := gitOwner.Start(); err != nil {
					workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
					_ = runOwner.Stop(ctx)
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				return process{hub: hub, runs: runs, runOwner: runOwner, gitOwner: gitOwner, workspaceObserver: workspaceObserver}
			}
			stopProcess := func(value process) {
				if value.workspaceObserver != nil {
					value.workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
				}
				if value.gitOwner != nil {
					_ = value.gitOwner.Stop(ctx)
				}
				if value.runOwner != nil {
					_ = value.runOwner.Stop(ctx)
				}
				if value.hub != nil {
					value.hub.Terminate(ctx)
				}
			}

			// Process one commits an external owner receipt.  Its state change is
			// the first ledger source and must be durable before the local loss.
			first := startProcess(t)
			owner := newRuntimeV2DriveTestModule(t, first.hub)
			run, err := first.runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			aggregate, err := owner.bizPtr.InitializeRuntimeV2FromRun(ctx, run)
			if err != nil {
				t.Fatal(err)
			}
			aggregate, err = owner.prepareRuntimeV2Prerequisites(ctx, aggregate)
			if err != nil || aggregate.Facts.Resources.ValidationScratch == nil {
				t.Fatalf("initial mixed prerequisites aggregate=%+v err=%v", aggregate, err)
			}
			committedScratch := *aggregate.Facts.Resources.ValidationScratch
			prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(aggregate.Definition, aggregate.Facts.Activation.ExecutionContract)
			if _, err := owner.bizPtr.BeginRuntimeV2CandidateAttemptForTest(ctx, "ws-v2", "run-v2", 0, prefix); err != nil {
				t.Fatal(err)
			}
			if _, err := owner.bizPtr.AcceptRuntimeV2CandidateAttempt(ctx, "ws-v2", "run-v2", 0, runtimeV2GitStateCandidate([]string{"create-commit", "repo-init", "stage-changes"})); err != nil {
				t.Fatal(err)
			}
			controller, err := runtimev2.NewRuntimeController(runtimeV2StateRepository{hub: first.hub})
			if err != nil {
				t.Fatal(err)
			}
			firstReceipt, err := controller.ExecuteActiveDispatch(ctx, "ws-v2", "run-v2", eventRuntimeV2OperationExecutor{hub: first.hub})
			if err != nil {
				t.Fatal(err)
			}
			if firstReceipt.ActiveDispatch != nil || firstReceipt.Facts.Recovery.PrerequisiteRefreshes != 0 ||
				firstReceipt.Facts.Recovery.PrerequisiteResourceChangeRefreshes != 1 || firstReceipt.Facts.Resources.ValidationScratch == nil ||
				firstReceipt.Facts.Resources.ValidationScratch.Revision != committedScratch.Revision || len(firstReceipt.OperationReceipts) != 1 ||
				firstReceipt.OperationReceipts[0].OperationID != "git-repo-init" {
				t.Fatalf("external receipt did not establish the mixed-owner boundary: %+v", firstReceipt)
			}
			if err := os.RemoveAll(committedScratch.Reference); err != nil {
				t.Fatal(err)
			}
			stopProcess(first)

			// Process two sees exactly the persisted external invalidation plus a
			// missing local resource.  Fault only the local-stale CAS transition.
			second := startProcess(t)
			baseRepository := runtimeV2StateRepository{hub: second.hub}
			marker := "mixed-owner-local-stale-before-cas"
			if crashAfterCAS {
				marker = "mixed-owner-local-stale-after-cas"
			}
			crashRepository := &runtimeV2SaveCrashRepository{
				delegate: baseRepository, after: crashAfterCAS, marker: marker,
				match: func(current runtimev2.RuntimeAggregate) bool {
					return current.ActiveDispatch == nil && current.Facts.Resources.ValidationScratch == nil &&
						current.Facts.Recovery.PrerequisiteRefreshes == 1 && current.Facts.Recovery.PrerequisiteResourceChangeRefreshes == 1
				},
			}
			crashingOwner := orchestratorbiz.NewRuntimeOrchestrator()
			if err := crashingOwner.ConfigureRuntimeV2Repository(crashRepository); err != nil {
				t.Fatal(err)
			}
			crashing := newTestBiz(crashingOwner, second.hub, nil)
			loaded, err := crashing.bizPtr.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			func() {
				defer func() {
					if recovered := recover(); recovered != nil {
						if recovered != marker {
							t.Fatalf("unexpected mixed-owner stale marker: %v", recovered)
						}
						return
					}
					current, loadErr := crashing.bizPtr.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
					t.Fatalf("mixed-owner local stale CAS was not reached: loaded=%+v current=%+v err=%v", loaded, current, loadErr)
				}()
				_, _ = crashing.driveRuntimeV2Decision(ctx, loaded, "stable-prefix-test")
			}()
			if !crashRepository.fired {
				t.Fatal("mixed-owner local stale CAS point was not reached")
			}
			crashed, err := second.runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil || crashed.RuntimeState == nil {
				t.Fatalf("mixed-owner stale crash state=%+v err=%v", crashed, err)
			}
			if crashAfterCAS {
				if crashed.RuntimeState.Facts.Resources.ValidationScratch != nil || crashed.RuntimeState.Facts.Recovery.PrerequisiteRefreshes != 1 ||
					crashed.RuntimeState.Facts.Recovery.PrerequisiteResourceChangeRefreshes != 1 {
					t.Fatalf("after-CAS mixed stale did not preserve independent ledgers: %+v", crashed.RuntimeState)
				}
			} else if crashed.RuntimeState.Facts.Resources.ValidationScratch == nil || crashed.RuntimeState.Facts.Recovery.PrerequisiteRefreshes != 0 ||
				crashed.RuntimeState.Facts.Recovery.PrerequisiteResourceChangeRefreshes != 1 {
				t.Fatalf("before-CAS mixed stale leaked local invalidation: %+v", crashed.RuntimeState)
			}
			if len(crashed.RuntimeState.CandidateRequestSessions) != 1 || len(crashed.RuntimeState.CandidateAttempts) != 1 || crashed.RuntimeState.ActiveDispatch != nil ||
				len(crashed.RuntimeState.GovernanceRequests) != 0 || len(crashed.RuntimeState.OperationReceipts) != 1 {
				t.Fatalf("mixed stale replayed a candidate, dispatch, ticket, or operation: %+v", crashed.RuntimeState)
			}
			stopProcess(second)

			// A fresh process either persists the one missing stale transition or
			// merely reacquires it.  In both cases the Git receipt is retained
			// exactly once and no provider/operation path is entered.
			third := startProcess(t)
			defer stopProcess(third)
			restarted := newRuntimeV2DriveTestModule(t, third.hub)
			loaded, err = restarted.bizPtr.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			if !crashAfterCAS {
				loaded, err = restarted.driveRuntimeV2Decision(ctx, loaded, "stable-prefix-test")
				if err != nil {
					t.Fatal(err)
				}
			}
			resumed, err := restarted.prepareRuntimeV2Prerequisites(ctx, loaded)
			if err != nil {
				t.Fatal(err)
			}
			if resumed.Facts.Resources.ValidationScratch == nil || resumed.Facts.Resources.ValidationScratch.Reference != committedScratch.Reference ||
				resumed.Facts.Resources.ValidationScratch.Revision == committedScratch.Revision || resumed.Facts.Recovery.PrerequisiteRefreshes != 1 ||
				resumed.Facts.Recovery.PrerequisiteResourceChangeRefreshes != 1 || len(resumed.Facts.Prerequisites.Pending) != 0 ||
				len(resumed.CandidateRequestSessions) != 1 || len(resumed.CandidateAttempts) != 1 || resumed.ActiveDispatch != nil ||
				len(resumed.GovernanceRequests) != 0 || len(resumed.OperationReceipts) != 1 || resumed.OperationReceipts[0].OperationID != "git-repo-init" {
				t.Fatalf("mixed-owner restart did not converge only prerequisite facts: %+v", resumed)
			}
			if info, statErr := os.Stat(resumed.Facts.Resources.ValidationScratch.Reference); statErr != nil || !info.IsDir() {
				t.Fatalf("reacquired mixed-owner scratch = %v, info=%+v", statErr, info)
			}
		})
	}
}

func runtimeV2GitCapabilityStateBootstrap() runtimecontract.RuntimeBootstrap {
	bootstrap := runtimeV2Bootstrap()
	bootstrap.Definition.CapabilityContract = []string{runtimecontract.StepKindGitWrite}
	bootstrap.Focuses[0].AcceptedStepKinds = []string{runtimecontract.StepKindGitWrite}
	bootstrap.Requirements = []runtimecontract.RuntimeRequirement{
		{Kind: runtimecontract.PrerequisiteKindCapabilityState, OwnerModuleID: runtimecontract.RuntimeModuleGit, CapabilityStateKind: "repository_state", SubjectRef: "workspace_deliverable", Reason: "repository state required"},
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleGit, CapabilityClass: runtimecontract.RuntimeCapabilityClassStateChange, RequiredCapabilities: []string{runtimecontract.StepKindGitWrite}, Reason: "git write capability required"},
	}
	return bootstrap
}

func runtimeV2GitStateCandidate(requirementRefs []string) runtimecontract.RuntimeCandidate {
	operations := make([]runtimecontract.StateChangingOperation, 0, len(requirementRefs))
	for _, ref := range requirementRefs {
		operation := runtimecontract.RuntimeOperation{ID: "git-" + ref, Kind: runtimecontract.StepKindGitWrite, RequiredOperationRef: ref, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}}
		switch ref {
		case "repo-init":
			value := "init"
			operation.Parameters = []runtimecontract.OperationParameter{{Name: "operation", String: &value}}
		case "stage-changes":
			value := "add"
			operation.Parameters = []runtimecontract.OperationParameter{{Name: "operation", String: &value}, {Name: "pathspecs", StringList: []string{"target.txt"}}}
		case "create-commit":
			value, message := "commit", "runtime v2 fixture"
			operation.Parameters = []runtimecontract.OperationParameter{{Name: "operation", String: &value}, {Name: "message", String: &message}}
		}
		switch ref {
		case "stage-changes":
			if containsRuntimeV2String(requirementRefs, "repo-init") {
				operation.DependsOn = []string{"git-repo-init"}
			}
		case "create-commit":
			if containsRuntimeV2String(requirementRefs, "stage-changes") {
				operation.DependsOn = []string{"git-stage-changes"}
			}
		}
		operations = append(operations, runtimecontract.StateChangingOperation{Operation: operation})
	}
	return runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: operations}}
}

func containsRuntimeV2String(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func requiredOperationRefs(values []runtimecontract.RuntimeRequiredOperationVariant) []string {
	ret := make([]string, 0, len(values))
	for _, value := range values {
		ret = append(ret, value.RequirementID)
	}
	return ret
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}
	return true
}

func writeRuntimeV2GitFixtureFile(t *testing.T, deliverable string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(deliverable, "target.txt"), []byte("runtime v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runRuntimeV2Git(t *testing.T, directory string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = directory
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
}

func runtimeV2GitOutput(t *testing.T, directory string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = directory
	output, err := command.Output()
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return string(output)
}

func TestRuntimeV2FileBackedPatchVariantMatrix(t *testing.T) {
	testCases := []struct {
		name      string
		target    string
		initial   string
		variant   string
		candidate func() runtimecontract.RuntimeCandidate
		want      string
	}{
		{
			name: "file_write", target: "written.txt", initial: "old\n", variant: "file_write", want: "written\n",
			candidate: func() runtimecontract.RuntimeCandidate {
				patchType, content, target := "file_write", "written\n", "written.txt"
				return runtimeV2PatchCandidate("patch-file-write", target, []runtimecontract.OperationParameter{{Name: "patchType", String: &patchType}, {Name: "content", String: &content}})
			},
		},
		{
			name: "text_replace", target: "replace.txt", initial: "old\n", variant: "text_replace", want: "new\n",
			candidate: func() runtimecontract.RuntimeCandidate {
				patchType, oldText, newText, target := "text_replace", "old\n", "new\n", "replace.txt"
				return runtimeV2PatchCandidate("patch-text-replace", target, []runtimecontract.OperationParameter{{Name: "patchType", String: &patchType}, {Name: "oldText", String: &oldText}, {Name: "newText", String: &newText}})
			},
		},
		{
			name: "unified_diff", target: "diff.txt", initial: "one\ntwo\nthree\n", variant: "unified_diff", want: "one\nTWO\nthree\n",
			candidate: func() runtimecontract.RuntimeCandidate {
				patchType, target := "unified_diff", "diff.txt"
				diff := "--- a/diff.txt\n+++ b/diff.txt\n@@ -1,3 +1,3 @@\n one\n-two\n+TWO\n three\n"
				return runtimeV2PatchCandidate("patch-unified-diff", target, []runtimecontract.OperationParameter{{Name: "patchType", String: &patchType}, {Name: "diff", String: &diff}})
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			hub := magicevent.NewHub(32)
			t.Cleanup(func() { hub.Terminate(ctx) })
			root := t.TempDir()
			deliverable, internalDir := filepath.Join(root, "workspace"), filepath.Join(root, "internal")
			if err := os.MkdirAll(deliverable, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(internalDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if testCase.initial != "" {
				if err := os.WriteFile(filepath.Join(deliverable, testCase.target), []byte(testCase.initial), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			bootstrap := runtimeV2PatchVariantBootstrap(testCase.variant, testCase.target)
			runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
			if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2", DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, State: common.RunStateActive}); err != nil {
				t.Fatal(err)
			}
			runOwner := runmodule.New()
			if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
				t.Fatal(err)
			}
			if err := runOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = runOwner.Stop(ctx) })
			startRuntimeV2ReceiptTestOwner(t, hub)
			workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
			workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
				result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
			})
			t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
			fileOwner := filemodule.New()
			if err := fileOwner.Configure(filemodule.Dependencies{EventHub: hub}); err != nil {
				t.Fatal(err)
			}
			if err := fileOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = fileOwner.Stop(ctx) })
			llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
			llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
				command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
				result.Set(runtimeV2CandidateResultForTest(t, command, testCase.candidate()), nil)
			})
			t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
			module := newRuntimeV2DriveTestModule(t, hub)
			driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
			driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, module.handleRuntimeV2Drive)
			t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2) })
			completed, err := DriveRuntimeV2(ctx, hub, orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"})
			if err != nil || completed.Action != runtimecontract.RuntimeActionComplete {
				t.Fatalf("completed=%+v err=%v", completed, err)
			}
			content, err := os.ReadFile(filepath.Join(deliverable, testCase.target))
			if err != nil || string(content) != testCase.want {
				t.Fatalf("content=%q err=%v want=%q", content, err, testCase.want)
			}
		})
	}
}

func runtimeV2PatchVariantBootstrap(variant, target string) runtimecontract.RuntimeBootstrap {
	bootstrap := runtimeV2Bootstrap()
	bootstrap.Definition.CapabilityContract = []string{runtimecontract.RuntimeBlockKindPatchApply}
	bootstrap.Focuses[0].AcceptedStepKinds = []string{runtimecontract.RuntimeBlockKindPatchApply}
	bootstrap.Focuses[0].AcceptedOperationVariants = []runtimecontract.RuntimeOperationVariantRef{{Kind: runtimecontract.RuntimeBlockKindPatchApply, VariantID: variant}}
	bootstrap.Focuses[0].TargetRefs = []string{target}
	bootstrap.Requirements = []runtimecontract.RuntimeRequirement{
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleFile, CapabilityClass: runtimecontract.RuntimeCapabilityClassStateChange, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindPatchApply}, Reason: "patch capability required"},
		{Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleFile, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindPatchApply}, Reason: "patch owner inventory required"},
	}
	return bootstrap
}

func runtimeV2PatchCandidate(id, target string, parameters []runtimecontract.OperationParameter) runtimecontract.RuntimeCandidate {
	return runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{ID: id, Kind: runtimecontract.RuntimeBlockKindPatchApply, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{target}, Parameters: parameters}}}}}
}

func TestRuntimeV2FileBackedPatchReplayProofRestartsAfterOwnerReceiptCASFailure(t *testing.T) {
	testCases := []struct {
		name        string
		target      string
		initial     string
		variant     string
		operationID string
		want        string
		candidate   func(string) runtimecontract.RuntimeCandidate
	}{
		{
			name: "file_write", target: "written.txt", initial: "old\n", variant: "file_write", operationID: "patch-file-write", want: "written\n",
			candidate: func(target string) runtimecontract.RuntimeCandidate {
				patchType, content := "file_write", "written\n"
				return runtimeV2PatchCandidate("patch-file-write", target, []runtimecontract.OperationParameter{{Name: "patchType", String: &patchType}, {Name: "content", String: &content}})
			},
		},
		{
			name: "text_replace", target: "written.txt", initial: "old\n", variant: "text_replace", operationID: "patch-text-replace", want: "written\n",
			candidate: func(target string) runtimecontract.RuntimeCandidate {
				patchType, oldText, newText := "text_replace", "old\n", "written\n"
				return runtimeV2PatchCandidate("patch-text-replace", target, []runtimecontract.OperationParameter{{Name: "patchType", String: &patchType}, {Name: "oldText", String: &oldText}, {Name: "newText", String: &newText}})
			},
		},
		{
			name: "unified_diff", target: "written.txt", initial: "old\n", variant: "unified_diff", operationID: "patch-unified-diff", want: "written\n",
			candidate: func(target string) runtimecontract.RuntimeCandidate {
				patchType := "unified_diff"
				diff := "--- a/written.txt\n+++ b/written.txt\n@@ -1,1 +1,1 @@\n-old\n+written\n"
				return runtimeV2PatchCandidate("patch-unified-diff", target, []runtimecontract.OperationParameter{{Name: "patchType", String: &patchType}, {Name: "diff", String: &diff}})
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			testRuntimeV2FileBackedPatchReplayProofRestartsAfterOwnerReceiptCASFailure(t, testCase.target, testCase.initial, testCase.variant, testCase.operationID, testCase.want, testCase.candidate)
		})
	}
}

func testRuntimeV2FileBackedPatchReplayProofRestartsAfterOwnerReceiptCASFailure(t *testing.T, target, initial, variant, operationID, want string, candidateForTarget func(string) runtimecontract.RuntimeCandidate) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	root := t.TempDir()
	deliverable, internalDir := filepath.Join(root, "workspace"), filepath.Join(root, "internal")
	if err := os.MkdirAll(deliverable, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(internalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	bootstrap := runtimeV2PatchVariantBootstrap(variant, target)
	// Make the continuation cross the P4 assessment boundary as well as the
	// P3 owner-receipt boundary. Other patch-variant tests intentionally keep
	// their bootstrap validation-free, so scope this requirement to this
	// receipt-loss recovery sequence.
	bootstrap.Focuses[0].RequireAssessment = true
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2", DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, State: common.RunStateActive}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deliverable, target), []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	fileOwner := filemodule.New()
	if err := fileOwner.Configure(filemodule.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := fileOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fileOwner.Stop(ctx) })
	baseRepository := runtimeV2StateRepository{hub: hub}
	failingRepository := &failWhenRuntimeStateRepository{
		RuntimeStateRepository: baseRepository, err: errors.New("injected owner receipt CAS failure"),
		match: func(aggregate runtimev2.RuntimeAggregate) bool {
			if aggregate.ActiveDispatch == nil || len(aggregate.OperationReceipts) != 1 || aggregate.EvidenceSequence != 0 {
				return false
			}
			execution, found := runtimeV2DriveOperationExecutionRecord(aggregate.ActiveDispatch.OperationExecutions, operationID)
			return found && execution.State == runtimecontract.OperationExecutionOwnerCommitted
		},
	}
	crashingBiz := orchestratorbiz.NewRuntimeOrchestrator()
	if err := crashingBiz.ConfigureRuntimeV2Repository(failingRepository); err != nil {
		t.Fatal(err)
	}
	crashingModule := newTestBiz(crashingBiz, hub, nil)
	receiptObserver := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	receiptObserver.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, crashingModule.handleRuntimeV2OperationReceipt)
	receiptObserver.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, crashingModule.handleRuntimeV2OperationReceipt)

	run, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err := crashingBiz.InitializeRuntimeV2FromRun(ctx, run)
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err = crashingModule.prepareRuntimeV2Prerequisites(ctx, aggregate)
	if err != nil {
		t.Fatal(err)
	}
	candidate := candidateForTarget(target)
	providerCalls := 0
	stateChangeCalls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		providerCalls++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		if command.Decision.Action == runtimecontract.RuntimeActionProduceAssessment {
			evidenceRefs := make([]string, 0, len(command.CandidateContext.Evidence.Accepted))
			for _, evidence := range command.CandidateContext.Evidence.Accepted {
				evidenceRefs = append(evidenceRefs, string(evidence.ID))
			}
			candidate := runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{
				FocusID: "focus-v2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, Disposition: runtimecontract.AssessmentDispositionSatisfied,
				Summary: "replay-proof durable patch assessed", Items: []runtimecontract.RuntimeAssessmentItemFact{{CoverageWorkItemID: "cwi-v2", Satisfied: true, EvidenceRefs: evidenceRefs}}, EvidenceRefs: evidenceRefs,
			}}
			result.Set(runtimeV2CandidateResultForTest(t, command, candidate), nil)
			return
		}
		if command.Decision.Action == runtimecontract.RuntimeActionProduceStateChange {
			stateChangeCalls++
		}
		result.Set(runtimeV2CandidateResultForTest(t, command, candidate), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(aggregate.Definition, aggregate.Facts.Activation.ExecutionContract)
	aggregate, err = crashingModule.requestAndSealRuntimeV2CandidateWithRetry(ctx, aggregate, prefix)
	if err != nil || aggregate.ActiveDispatch == nil {
		t.Fatalf("seal replay-proof dispatch aggregate=%+v err=%v", aggregate, err)
	}
	sealedDispatchID := aggregate.ActiveDispatch.ID
	sealedExecution, found := runtimeV2DriveOperationExecutionRecord(aggregate.ActiveDispatch.OperationExecutions, operationID)
	if !found || sealedExecution.ReplayProof == nil {
		t.Fatalf("patch dispatch has no sealed replay proof: %+v", sealedExecution)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "runtime-v2-test-crash-after-owner-receipt-commit-error" {
				t.Fatalf("unexpected crash marker: %v", recovered)
			}
		}()
		_, _ = crashingBiz.ExecuteRuntimeV2Dispatch(ctx, "ws-v2", "run-v2", panicAfterOwnerReceiptCommitErrorExecutor{delegate: eventRuntimeV2OperationExecutor{hub: hub}})
		t.Fatal("expected injected receipt-CAS crash")
	}()
	if !failingRepository.failed {
		t.Fatal("owner receipt CAS failure point was not reached")
	}
	if content, err := os.ReadFile(filepath.Join(deliverable, target)); err != nil || string(content) != want {
		t.Fatalf("durable side effect before restart content=%q err=%v", content, err)
	}
	crashed, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || crashed.RuntimeState == nil || crashed.RuntimeState.ActiveDispatch == nil || len(crashed.RuntimeState.OperationReceipts) != 0 || crashed.RuntimeState.EvidenceSequence != 0 {
		t.Fatalf("receipt-CAS crash persisted state=%+v err=%v", crashed, err)
	}
	claimed, found := runtimeV2DriveOperationExecutionRecord(crashed.RuntimeState.ActiveDispatch.OperationExecutions, operationID)
	if !found || claimed.State != runtimecontract.OperationExecutionClaimed || claimed.Fence == 0 || claimed.ExecutionKey != sealedExecution.ExecutionKey || claimed.ReplayProof == nil {
		t.Fatalf("receipt-CAS crash lost claimed proof execution=%+v", claimed)
	}

	receiptObserver.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
	receiptObserver.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
	llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate)
	workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
	if err := fileOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	hub.Terminate(ctx)

	// The replay-proof recovery is deliberately a new process. Its owner can
	// execute only with the sealed key/fence/proof persisted before the receipt
	// CAS loss; no old owner cache, responder or provider session survives.
	restartedHub := magicevent.NewHub(32)
	t.Cleanup(func() { restartedHub.Terminate(ctx) })
	restartedRunOwner := runmodule.New()
	if err := restartedRunOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: restartedHub}); err != nil {
		t.Fatal(err)
	}
	if err := restartedRunOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedRunOwner.Stop(ctx) })
	restartedWorkspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, restartedHub)
	restartedWorkspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
	})
	t.Cleanup(func() { restartedWorkspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	restartedFileOwner := filemodule.New()
	if err := restartedFileOwner.Configure(filemodule.Dependencies{EventHub: restartedHub}); err != nil {
		t.Fatal(err)
	}
	if err := restartedFileOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedFileOwner.Stop(ctx) })
	restartedLLMObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, restartedHub)
	restartedLLMObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		providerCalls++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		if command.Decision.Action == runtimecontract.RuntimeActionProduceAssessment {
			evidenceRefs := make([]string, 0, len(command.CandidateContext.Evidence.Accepted))
			for _, evidence := range command.CandidateContext.Evidence.Accepted {
				evidenceRefs = append(evidenceRefs, string(evidence.ID))
			}
			assessment := runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{
				FocusID: "focus-v2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, Disposition: runtimecontract.AssessmentDispositionSatisfied,
				Summary: "replay-proof durable patch assessed", Items: []runtimecontract.RuntimeAssessmentItemFact{{CoverageWorkItemID: "cwi-v2", Satisfied: true, EvidenceRefs: evidenceRefs}}, EvidenceRefs: evidenceRefs,
			}}
			result.Set(runtimeV2CandidateResultForTest(t, command, assessment), nil)
			return
		}
		if command.Decision.Action == runtimecontract.RuntimeActionProduceStateChange {
			stateChangeCalls++
		}
		result.Set(runtimeV2CandidateResultForTest(t, command, candidate), nil)
	})
	t.Cleanup(func() { restartedLLMObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	restartedRepository := runtimeV2StateRepository{hub: restartedHub}
	restartedBiz := orchestratorbiz.NewRuntimeOrchestrator()
	if err := restartedBiz.ConfigureRuntimeV2Repository(restartedRepository); err != nil {
		t.Fatal(err)
	}
	restartedModule := newTestBiz(restartedBiz, restartedHub, nil)
	restartedReceiptObserver := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, restartedHub)
	restartedReceiptObserver.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, restartedModule.handleRuntimeV2OperationReceipt)
	restartedReceiptObserver.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, restartedModule.handleRuntimeV2OperationReceipt)
	t.Cleanup(func() {
		restartedReceiptObserver.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
		restartedReceiptObserver.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
	})
	completed, err := restartedBiz.ExecuteRuntimeV2Dispatch(ctx, "ws-v2", "run-v2", eventRuntimeV2OperationExecutor{hub: restartedHub})
	if err != nil || completed.ActiveDispatch != nil || completed.Decision.Action != runtimecontract.RuntimeActionAcquirePrerequisite || completed.Facts.Recovery.OperationRecoveryAttempts != 1 || len(completed.OperationReceipts) != 1 || len(completed.Facts.Evidence.Accepted) != 1 {
		t.Fatalf("restart did not recover proof-bound receipt exactly once aggregate=%+v err=%v", completed, err)
	}
	receipt := completed.OperationReceipts[0]
	if receipt.ExecutionKey != claimed.ExecutionKey || receipt.Fence != claimed.Fence || receipt.BeforeHash != claimed.ReplayProof.BeforeHash || receipt.AfterHash != claimed.ReplayProof.AfterHash {
		t.Fatalf("restart did not retain sealed execution/receipt delta receipt=%+v claimed=%+v", receipt, claimed)
	}
	// A duplicate owner delivery after process reconstruction must resolve the
	// same persisted key/fence receipt. It must neither reapply the durable
	// patch nor mutate the Run aggregate after the first recovered evidence
	// commit.
	var sealedDispatch runtimev2.SealedDispatch
	for _, value := range completed.CompletedDispatches {
		if value.ID == sealedDispatchID {
			sealedDispatch = value
			break
		}
	}
	if sealedDispatch.ID == "" || sealedDispatch.Candidate.StateChange == nil || len(sealedDispatch.Candidate.StateChange.Operations) != 1 {
		t.Fatalf("recovered dispatch is unavailable for duplicate delivery: %+v", completed.CompletedDispatches)
	}
	// The File owner above was constructed only after the process restart. Its
	// duplicate answer must come from the persisted receipt query, not an old
	// adapter cache.
	duplicate, err := (eventRuntimeV2OperationExecutor{hub: restartedHub}).ExecuteRuntimeOperation(ctx, runtimev2.RuntimeOperationExecutionRequest{
		WorkspaceID: "ws-v2", RunID: "run-v2", DispatchID: sealedDispatch.ID, Transaction: sealedDispatch.Transaction,
		Operation: sealedDispatch.Candidate.StateChange.Operations[0].Operation, Execution: claimed,
		EvidenceKind: runtimecontract.EvidenceKindStateChange, TargetRole: completed.Facts.Focus.TargetRole,
	})
	if err != nil || duplicate.Receipt.ExecutionKey != receipt.ExecutionKey || duplicate.Receipt.Fence != receipt.Fence ||
		duplicate.Receipt.BeforeHash != receipt.BeforeHash || duplicate.Receipt.AfterHash != receipt.AfterHash {
		t.Fatalf("duplicate owner delivery did not reuse persisted receipt result=%+v err=%v", duplicate, err)
	}
	if content, readErr := os.ReadFile(filepath.Join(deliverable, target)); readErr != nil || string(content) != want {
		t.Fatalf("duplicate owner delivery rewrote durable target content=%q err=%v", content, readErr)
	}
	loaded, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || loaded.RuntimeState == nil || loaded.RuntimeState.StateRevision != completed.StateRevision ||
		len(loaded.RuntimeState.OperationReceipts) != 1 || len(loaded.RuntimeState.Facts.Evidence.Accepted) != 1 {
		t.Fatalf("duplicate owner delivery changed persisted aggregate=%+v err=%v", loaded, err)
	}

	// Continue through the normal drive after the reconstructed receipt. This
	// ties P3's owner-receipt loss to P4's evidence/assessment transition: the
	// restart may only consume the original key/fence receipt and must not ask
	// the provider for a second state-changing candidate before terminal
	// authority is committed.
	driveObserver := magicevent.NewSimpleObserver(ModuleID, restartedHub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, restartedModule.handleRuntimeV2Drive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2) })
	terminal, err := DriveRuntimeV2(ctx, restartedHub, orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"})
	if err != nil || terminal.Disposition != orchestratorevents.RuntimeV2DriveDispositionTerminal || terminal.Action != runtimecontract.RuntimeActionComplete || terminal.Terminal == nil {
		t.Fatalf("receipt-loss continuation did not commit terminal authority result=%+v err=%v", terminal, err)
	}
	if providerCalls != 2 || stateChangeCalls != 1 {
		t.Fatalf("receipt-loss continuation provider calls=%d state-change calls=%d, want one initial patch and one assessment", providerCalls, stateChangeCalls)
	}
	terminalAggregate, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || terminalAggregate.RuntimeState == nil || terminalAggregate.RuntimeState.ActiveDispatch != nil ||
		terminalAggregate.RuntimeState.Decision.Action != runtimecontract.RuntimeActionComplete || terminalAggregate.RuntimeState.Decision.Terminal == nil ||
		len(terminalAggregate.RuntimeState.OperationReceipts) != 1 || len(terminalAggregate.RuntimeState.Facts.Evidence.Accepted) != 1 ||
		len(terminalAggregate.RuntimeState.Facts.Assessments) != 1 {
		t.Fatalf("receipt-loss continuation changed P3/P4 identities or missed assessment terminal aggregate=%+v err=%v", terminalAggregate, err)
	}
	finalReceipt := terminalAggregate.RuntimeState.OperationReceipts[0]
	if finalReceipt.ExecutionKey != receipt.ExecutionKey || finalReceipt.Fence != receipt.Fence || finalReceipt.BeforeHash != receipt.BeforeHash || finalReceipt.AfterHash != receipt.AfterHash {
		t.Fatalf("terminal continuation replaced reconstructed receipt before=%+v after=%+v", receipt, finalReceipt)
	}
	// P3 may race with the eventual terminal delivery: a delayed duplicate
	// receipt must remain an idempotent lookup, while a different receipt for
	// the same sealed execution key/fence must be rejected.  In both cases the
	// already committed SM-2 terminal authority is immutable; no new dispatch,
	// evidence, candidate attempt, or provider request may appear.
	duplicateTerminal, duplicateReceipt, err := restartedBiz.CommitRuntimeV2OperationReceipt(
		ctx, "ws-v2", "run-v2", sealedDispatchID, finalReceipt.Fence, finalReceipt,
	)
	if err != nil || duplicateTerminal.StateRevision != terminalAggregate.RuntimeState.StateRevision ||
		!runtimecontract.SameRuntimeOperationReceipt(duplicateReceipt, finalReceipt) {
		t.Fatalf("duplicate receipt after terminal aggregate=%+v receipt=%+v err=%v", duplicateTerminal, duplicateReceipt, err)
	}
	tamperedReceipt := finalReceipt.Clone()
	tamperedReceipt.Evidence.Summary = "tampered delayed receipt"
	if _, _, err := restartedBiz.CommitRuntimeV2OperationReceipt(ctx, "ws-v2", "run-v2", sealedDispatchID, tamperedReceipt.Fence, tamperedReceipt); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("different delayed receipt after terminal err=%v, want conflict", err)
	}
	immutableTerminal, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || immutableTerminal.RuntimeState == nil ||
		immutableTerminal.RuntimeState.StateRevision != terminalAggregate.RuntimeState.StateRevision ||
		immutableTerminal.RuntimeState.Decision.Action != runtimecontract.RuntimeActionComplete ||
		len(immutableTerminal.RuntimeState.OperationReceipts) != 1 || len(immutableTerminal.RuntimeState.Facts.Evidence.Accepted) != 1 ||
		providerCalls != 2 || stateChangeCalls != 1 {
		t.Fatalf("late receipt changed terminal authority=%+v provider=%d stateChanges=%d err=%v", immutableTerminal, providerCalls, stateChangeCalls, err)
	}

	// Carry the recovered P3 receipt through a P5 owner acknowledgement loss.
	// The workspace observer was only the File prerequisite adapter; terminal
	// projection below deliberately uses the production Workspace owner.
	restartedWorkspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
	workspaces := filerepository.NewWorkspaceRepository(filepath.Join(root, "workspaces.json"))
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{
		ID: "ws-v2", State: common.WorkspaceStateActive, ActiveRunID: "run-v2",
		DeliverableDirectory: deliverable, InternalDirectory: internalDir,
	}); err != nil {
		t.Fatal(err)
	}
	workspaceOwner := workspacemodule.New()
	if err := workspaceOwner.Configure(workspacemodule.Dependencies{
		Workspaces: workspaces, Goals: memory.NewGoalRepository(), DoDs: memory.NewDoDRepository(),
		Templates: memory.NewWorkspaceTemplateRepository(), EventHub: restartedHub,
	}); err != nil {
		t.Fatal(err)
	}
	if err := workspaceOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = workspaceOwner.Stop(ctx) })
	deliveryModule := newTestBiz(orchestratorbiz.NewRuntimeOrchestrator(), restartedHub, nil)
	results := filerepository.NewResultRepository(filepath.Join(root, "results.json"))
	interruptedResults := &failAfterResultCreateRepository{ResultRepository: results}
	firstResultOwner := resultmodule.New()
	if err := firstResultOwner.Configure(resultmodule.Dependencies{
		Results: interruptedResults, Artifacts: filerepository.NewArtifactRepository(filepath.Join(root, "artifacts.json")), EventHub: restartedHub,
	}); err != nil {
		t.Fatal(err)
	}
	if err := firstResultOwner.Start(); err != nil {
		t.Fatal(err)
	}
	if err := deliveryModule.deliverRuntimeV2Terminal(ctx, runtimeAggregateFromRecord(*terminalAggregate.RuntimeState)); err == nil {
		t.Fatal("expected terminal Result acknowledgement loss after recovered receipt")
	}
	if !interruptedResults.fired {
		t.Fatal("terminal Result acknowledgement loss was not reached")
	}
	if run, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2"); err != nil || run.State != common.RunStateActive {
		t.Fatalf("P5 projected Run before Result acknowledgement restart: %+v err=%v", run, err)
	}
	if workspace, err := workspaces.Get(ctx, "ws-v2"); err != nil || workspace.ActiveRunID != "run-v2" {
		t.Fatalf("P5 released Workspace before Result acknowledgement restart: %+v err=%v", workspace, err)
	}
	if err := firstResultOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	restartedResultOwner := resultmodule.New()
	if err := restartedResultOwner.Configure(resultmodule.Dependencies{
		Results: results, Artifacts: filerepository.NewArtifactRepository(filepath.Join(root, "artifacts.json")), EventHub: restartedHub,
	}); err != nil {
		t.Fatal(err)
	}
	if err := restartedResultOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedResultOwner.Stop(ctx) })
	persistedTerminal, err := restartedBiz.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
	if err != nil || persistedTerminal.StateRevision != terminalAggregate.RuntimeState.StateRevision || persistedTerminal.Decision.Action != runtimecontract.RuntimeActionComplete {
		t.Fatalf("P3/P5 restart did not retain terminal authority=%+v err=%v", persistedTerminal, err)
	}
	if err := deliveryModule.deliverRuntimeV2Terminal(ctx, persistedTerminal); err != nil {
		t.Fatalf("P3/P5 terminal replay: %v", err)
	}
	if items, err := results.ListByRun(ctx, "run-v2"); err != nil || len(items) != 1 || items[0].State != common.ResultStateAccepted {
		t.Fatalf("P3/P5 Result replay=%+v err=%v", items, err)
	}
	if run, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2"); err != nil || run.State != common.RunStateCompleted {
		t.Fatalf("P3/P5 Run replay=%+v err=%v", run, err)
	}
	if workspace, err := workspaces.Get(ctx, "ws-v2"); err != nil || workspace.ActiveRunID != "" {
		t.Fatalf("P3/P5 Workspace replay=%+v err=%v", workspace, err)
	}
	finalAggregate, err := restartedBiz.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
	if err != nil || finalAggregate.StateRevision != terminalAggregate.RuntimeState.StateRevision ||
		len(finalAggregate.OperationReceipts) != 1 || len(finalAggregate.Facts.Evidence.Accepted) != 1 ||
		providerCalls != 2 || stateChangeCalls != 1 {
		t.Fatalf("P3/P5 replay rewrote authority=%+v provider=%d stateChanges=%d err=%v", finalAggregate, providerCalls, stateChangeCalls, err)
	}
}

func runtimeV2DriveOperationExecutionRecord(values []runtimecontract.RuntimeOperationExecutionRecord, operationID string) (runtimecontract.RuntimeOperationExecutionRecord, bool) {
	for _, value := range values {
		if value.OperationID == operationID {
			return value, true
		}
	}
	return runtimecontract.RuntimeOperationExecutionRecord{}, false
}

func TestRuntimeV2FileBackedPatchVariantRestartAfterOwnerReceipt(t *testing.T) {
	testCases := []struct {
		name      string
		target    string
		initial   string
		variant   string
		candidate func() runtimecontract.RuntimeCandidate
		want      string
	}{
		{
			name: "file_write", target: "written.txt", initial: "old\n", variant: "file_write", want: "written\n",
			candidate: func() runtimecontract.RuntimeCandidate {
				patchType, content, target := "file_write", "written\n", "written.txt"
				return runtimeV2PatchCandidate("patch-file-write", target, []runtimecontract.OperationParameter{{Name: "patchType", String: &patchType}, {Name: "content", String: &content}})
			},
		},
		{
			name: "text_replace", target: "replace.txt", initial: "old\n", variant: "text_replace", want: "new\n",
			candidate: func() runtimecontract.RuntimeCandidate {
				patchType, oldText, newText, target := "text_replace", "old\n", "new\n", "replace.txt"
				return runtimeV2PatchCandidate("patch-text-replace", target, []runtimecontract.OperationParameter{{Name: "patchType", String: &patchType}, {Name: "oldText", String: &oldText}, {Name: "newText", String: &newText}})
			},
		},
		{
			name: "unified_diff", target: "diff.txt", initial: "one\ntwo\nthree\n", variant: "unified_diff", want: "one\nTWO\nthree\n",
			candidate: func() runtimecontract.RuntimeCandidate {
				patchType, target := "unified_diff", "diff.txt"
				diff := "--- a/diff.txt\n+++ b/diff.txt\n@@ -1,3 +1,3 @@\n one\n-two\n+TWO\n three\n"
				return runtimeV2PatchCandidate("patch-unified-diff", target, []runtimecontract.OperationParameter{{Name: "patchType", String: &patchType}, {Name: "diff", String: &diff}})
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			hub := magicevent.NewHub(32)
			t.Cleanup(func() { hub.Terminate(ctx) })
			root := t.TempDir()
			deliverable, internalDir := filepath.Join(root, "workspace"), filepath.Join(root, "internal")
			if err := os.MkdirAll(deliverable, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(internalDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if testCase.initial != "" {
				if err := os.WriteFile(filepath.Join(deliverable, testCase.target), []byte(testCase.initial), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			bootstrap := runtimeV2PatchVariantBootstrap(testCase.variant, testCase.target)
			runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
			if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2", DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, State: common.RunStateActive}); err != nil {
				t.Fatal(err)
			}
			runOwner := runmodule.New()
			if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
				t.Fatal(err)
			}
			if err := runOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = runOwner.Stop(ctx) })
			startRuntimeV2ReceiptTestOwner(t, hub)
			workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
			workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
				result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
			})
			t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
			fileOwner := filemodule.New()
			if err := fileOwner.Configure(filemodule.Dependencies{EventHub: hub}); err != nil {
				t.Fatal(err)
			}
			if err := fileOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = fileOwner.Stop(ctx) })
			candidateCalls := 0
			llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
			llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
				candidateCalls++
				command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
				result.Set(runtimeV2CandidateResultForTest(t, command, testCase.candidate()), nil)
			})
			t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })

			module := newRuntimeV2DriveTestModule(t, hub)
			run, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			aggregate, err := module.bizPtr.InitializeRuntimeV2FromRun(ctx, run)
			if err != nil {
				t.Fatal(err)
			}
			aggregate, err = module.prepareRuntimeV2Prerequisites(ctx, aggregate)
			if err != nil {
				t.Fatal(err)
			}
			prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(aggregate.Definition, aggregate.Facts.Activation.ExecutionContract)
			aggregate, err = module.requestAndSealRuntimeV2CandidateWithRetry(ctx, aggregate, prefix)
			if err != nil || aggregate.ActiveDispatch == nil {
				t.Fatalf("seal patch variant before crash aggregate=%+v err=%v", aggregate, err)
			}
			func() {
				defer func() {
					if recovered := recover(); recovered != "runtime-v2-test-crash-after-owner-receipt" {
						t.Fatalf("unexpected patch variant crash marker: %v", recovered)
					}
				}()
				_, _ = module.bizPtr.ExecuteRuntimeV2Dispatch(ctx, "ws-v2", "run-v2", panicAfterOwnerReceiptExecutor{delegate: eventRuntimeV2OperationExecutor{hub: hub}})
				t.Fatal("expected injected patch variant crash after owner receipt")
			}()
			content, err := os.ReadFile(filepath.Join(deliverable, testCase.target))
			if err != nil || string(content) != testCase.want {
				state, _ := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
				var failure *runtimecontract.FailureFact
				if state.RuntimeState != nil && len(state.RuntimeState.OperationReceipts) > 0 {
					failure = state.RuntimeState.OperationReceipts[0].Failure
				}
				t.Fatalf("side effect before patch restart content=%q err=%v want=%q failure=%+v", content, err, testCase.want, failure)
			}
			crashed, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil || crashed.RuntimeState == nil || crashed.RuntimeState.ActiveDispatch == nil || len(crashed.RuntimeState.OperationReceipts) != 1 || crashed.RuntimeState.EvidenceSequence != 0 {
				t.Fatalf("patch receipt crash boundary=%+v err=%v", crashed, err)
			}
			execution := crashed.RuntimeState.ActiveDispatch.OperationExecutions[0]
			if execution.State != runtimecontract.OperationExecutionOwnerCommitted || execution.Fence == 0 {
				t.Fatalf("patch receipt crash execution=%+v", execution)
			}
			restarted := newRuntimeV2DriveTestModule(t, hub)
			driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
			driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, restarted.handleRuntimeV2Drive)
			t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2) })
			completed, err := DriveRuntimeV2(ctx, hub, orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"})
			if err != nil || completed.Action != runtimecontract.RuntimeActionComplete || candidateCalls != 1 {
				t.Fatalf("patch restart completion=%+v candidateCalls=%d err=%v", completed, candidateCalls, err)
			}
			persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil || persisted.RuntimeState == nil || len(persisted.RuntimeState.OperationReceipts) != 1 || len(persisted.RuntimeState.Facts.Evidence.Accepted) != 1 {
				t.Fatalf("patch restart persisted=%+v err=%v", persisted, err)
			}
			receipt := persisted.RuntimeState.OperationReceipts[0]
			if receipt.ExecutionKey != execution.ExecutionKey || receipt.Fence != execution.Fence || receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted {
				t.Fatalf("patch variant restart replaced receipt: before=%+v after=%+v", execution, receipt)
			}
		})
	}
}

func TestDriveRuntimeV2FileBackedRestartRecoversCrashAfterOwnerReceipt(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	root := t.TempDir()
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	bootstrap := runtimeV2Bootstrap()
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2",
		DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, State: common.RunStateActive,
	}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	startRuntimeV2ReceiptTestOwner(t, hub)

	deliverable := filepath.Join(root, "workspace")
	internalDir := filepath.Join(root, "internal")
	if err := os.MkdirAll(deliverable, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(internalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deliverable, "target.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{
			ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir,
		}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	fileOwner := filemodule.New()
	if err := fileOwner.Configure(filemodule.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := fileOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fileOwner.Stop(ctx) })
	candidateCount := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		candidateCount++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
			ID: "write-1", Kind: runtimecontract.StepKindFileEdit,
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"},
			Parameters: runtimeV2FileEditParameters("target.txt"),
		}}}}}
		result.Set(runtimeV2CandidateResultForTest(t, command, candidate), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })

	module := newRuntimeV2DriveTestModule(t, hub)
	run, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err := module.bizPtr.InitializeRuntimeV2FromRun(ctx, run)
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err = module.prepareRuntimeV2Prerequisites(ctx, aggregate)
	if err != nil {
		t.Fatal(err)
	}
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(aggregate.Definition, aggregate.Facts.Activation.ExecutionContract)
	aggregate, err = module.requestAndSealRuntimeV2CandidateWithRetry(ctx, aggregate, prefix)
	if err != nil || aggregate.ActiveDispatch == nil {
		t.Fatalf("seal before crash aggregate=%+v err=%v", aggregate, err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "runtime-v2-test-crash-after-owner-receipt" {
				t.Fatalf("unexpected crash marker: %v", recovered)
			}
		}()
		_, _ = module.bizPtr.ExecuteRuntimeV2Dispatch(ctx, "ws-v2", "run-v2", panicAfterOwnerReceiptExecutor{delegate: eventRuntimeV2OperationExecutor{hub: hub}})
		t.Fatal("expected injected crash after owner receipt")
	}()

	content, readErr := os.ReadFile(filepath.Join(deliverable, "target.txt"))
	if readErr != nil || string(content) != "new\n" {
		t.Fatalf("side effect before crash content=%q err=%v", string(content), readErr)
	}
	crashedRun, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || crashedRun.RuntimeState == nil || crashedRun.RuntimeState.ActiveDispatch == nil {
		t.Fatalf("crashed persisted run=%+v err=%v", crashedRun, err)
	}
	if len(crashedRun.RuntimeState.OperationReceipts) != 1 || crashedRun.RuntimeState.EvidenceSequence != 0 {
		t.Fatalf("crash boundary receipts=%d evidenceSequence=%d", len(crashedRun.RuntimeState.OperationReceipts), crashedRun.RuntimeState.EvidenceSequence)
	}
	execution := crashedRun.RuntimeState.ActiveDispatch.OperationExecutions[0]
	if execution.State != runtimecontract.OperationExecutionOwnerCommitted || execution.Fence == 0 {
		t.Fatalf("crash boundary execution=%+v", execution)
	}

	// Simulate a fresh process, not merely a new controller. The receipt is
	// durable authority; no previous Hub observer, File owner, Run owner or
	// provider observer is allowed to serve the recovery drive.
	llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate)
	workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
	if err := fileOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	hub.Terminate(ctx)

	restartedHub := magicevent.NewHub(32)
	t.Cleanup(func() { restartedHub.Terminate(ctx) })
	restartedRunOwner := runmodule.New()
	if err := restartedRunOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: restartedHub}); err != nil {
		t.Fatal(err)
	}
	if err := restartedRunOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedRunOwner.Stop(ctx) })
	startRuntimeV2ReceiptTestOwner(t, restartedHub)
	restartedWorkspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, restartedHub)
	restartedWorkspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{
			ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir,
		}}, nil)
	})
	t.Cleanup(func() { restartedWorkspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	restartedFileOwner := filemodule.New()
	if err := restartedFileOwner.Configure(filemodule.Dependencies{EventHub: restartedHub}); err != nil {
		t.Fatal(err)
	}
	if err := restartedFileOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedFileOwner.Stop(ctx) })

	restarted := newRuntimeV2DriveTestModule(t, restartedHub)
	driveObserver := magicevent.NewSimpleObserver(ModuleID, restartedHub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, restarted.handleRuntimeV2Drive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2) })
	completed, err := DriveRuntimeV2(ctx, restartedHub, orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"})
	if err != nil {
		t.Fatal(err)
	}
	if completed.Action != runtimecontract.RuntimeActionComplete || candidateCount != 1 {
		t.Fatalf("restart completion=%+v candidateCount=%d", completed, candidateCount)
	}
	completedRun, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || completedRun.RuntimeState == nil {
		t.Fatalf("completed persisted run=%+v err=%v", completedRun, err)
	}
	if len(completedRun.RuntimeState.OperationReceipts) != 1 || len(completedRun.RuntimeState.Facts.Evidence.Accepted) != 1 || completedRun.RuntimeState.EvidenceSequence != 1 {
		t.Fatalf("restart exactly-once receipts=%d accepted=%d evidenceSequence=%d", len(completedRun.RuntimeState.OperationReceipts), len(completedRun.RuntimeState.Facts.Evidence.Accepted), completedRun.RuntimeState.EvidenceSequence)
	}
	if completedRun.RuntimeState.OperationReceipts[0].ExecutionKey != execution.ExecutionKey || completedRun.RuntimeState.OperationReceipts[0].Fence != execution.Fence {
		t.Fatal(fmt.Sprintf("restart replaced owner receipt: before=%+v after=%+v", execution, completedRun.RuntimeState.OperationReceipts[0]))
	}
}

func TestRuntimeV2FileBackedClaimEvidenceAndDispatchCloseCrashMatrix(t *testing.T) {
	testCases := []struct {
		name       string
		validation bool
		after      bool
		match      func(runtimev2.RuntimeAggregate) bool
	}{
		{
			name: "claim_cas_before", validation: true,
			match: func(aggregate runtimev2.RuntimeAggregate) bool {
				return aggregate.ActiveDispatch != nil && len(aggregate.ActiveDispatch.OperationExecutions) == 1 &&
					aggregate.ActiveDispatch.OperationExecutions[0].State == runtimecontract.OperationExecutionClaimed && len(aggregate.OperationReceipts) == 0
			},
		},
		{
			name: "claim_cas_after_execute_before", validation: true, after: true,
			match: func(aggregate runtimev2.RuntimeAggregate) bool {
				return aggregate.ActiveDispatch != nil && len(aggregate.ActiveDispatch.OperationExecutions) == 1 &&
					aggregate.ActiveDispatch.OperationExecutions[0].State == runtimecontract.OperationExecutionClaimed && len(aggregate.OperationReceipts) == 0
			},
		},
		{
			name: "evidence_and_dispatch_close_cas_before", validation: false,
			match: func(aggregate runtimev2.RuntimeAggregate) bool { return aggregate.EvidenceSequence == 1 },
		},
		{
			name: "evidence_and_dispatch_close_cas_after", validation: false, after: true,
			match: func(aggregate runtimev2.RuntimeAggregate) bool { return aggregate.EvidenceSequence == 1 },
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			hub := magicevent.NewHub(32)
			t.Cleanup(func() { hub.Terminate(ctx) })
			root := t.TempDir()
			runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
			bootstrap := runtimeV2Bootstrap()
			if testCase.validation {
				bootstrap.Definition.CapabilityContract = []string{runtimecontract.StepKindFileRead}
				bootstrap.Focuses[0].RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
				bootstrap.Requirements = []runtimecontract.RuntimeRequirement{{
					Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleFile,
					CapabilityClass: runtimecontract.RuntimeCapabilityClassValidation, RequiredCapabilities: []string{runtimecontract.StepKindFileRead},
				}}
			}
			if err := runs.Create(ctx, &runmodel.Run{
				ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2",
				DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, State: common.RunStateActive,
			}); err != nil {
				t.Fatal(err)
			}
			runOwner := runmodule.New()
			if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
				t.Fatal(err)
			}
			if err := runOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = runOwner.Stop(ctx) })
			startRuntimeV2ReceiptTestOwner(t, hub)

			deliverable := filepath.Join(root, "workspace")
			internalDir := filepath.Join(root, "internal")
			if err := os.MkdirAll(deliverable, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(internalDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(deliverable, "target.txt"), []byte("old\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
			workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
				result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
			})
			t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
			fileOwner := filemodule.New()
			if err := fileOwner.Configure(filemodule.Dependencies{EventHub: hub}); err != nil {
				t.Fatal(err)
			}
			if err := fileOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = fileOwner.Stop(ctx) })

			module := newRuntimeV2DriveTestModule(t, hub)
			run, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			aggregate, err := module.bizPtr.InitializeRuntimeV2FromRun(ctx, run)
			if err != nil {
				t.Fatal(err)
			}
			aggregate, err = module.prepareRuntimeV2Prerequisites(ctx, aggregate)
			if err != nil {
				t.Fatal(err)
			}
			prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(aggregate.Definition, aggregate.Facts.Activation.ExecutionContract)
			if _, err := module.bizPtr.BeginRuntimeV2CandidateAttemptForTest(ctx, "ws-v2", "run-v2", 0, prefix); err != nil {
				t.Fatal(err)
			}
			target := "target.txt"
			candidate := runtimecontract.RuntimeCandidate{}
			if testCase.validation {
				candidate.Validation = &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: runtimecontract.RuntimeOperation{
					ID: "read-1", Kind: runtimecontract.StepKindFileRead, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{target},
					Parameters: nil,
				}}}}
			} else {
				candidate.StateChange = &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
					ID: "edit-1", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{target},
					Parameters: runtimeV2FileEditParameters(target),
				}}}}
			}
			sealed, err := module.bizPtr.AcceptRuntimeV2CandidateAttempt(ctx, "ws-v2", "run-v2", 0, candidate)
			if err != nil || sealed.ActiveDispatch == nil {
				t.Fatalf("sealed aggregate=%+v err=%v", sealed, err)
			}
			baseRepository := runtimeV2StateRepository{hub: hub}
			crashRepository := &runtimeV2SaveCrashRepository{delegate: baseRepository, match: testCase.match, after: testCase.after, marker: testCase.name}
			crashingController, err := runtimev2.NewRuntimeController(crashRepository)
			if err != nil {
				t.Fatal(err)
			}
			func() {
				defer func() {
					if recovered := recover(); recovered != testCase.name {
						t.Fatalf("unexpected crash marker: %v", recovered)
					}
				}()
				_, _ = crashingController.ExecuteActiveDispatch(ctx, "ws-v2", "run-v2", eventRuntimeV2OperationExecutor{hub: hub})
				t.Fatal("expected injected save crash")
			}()
			if !crashRepository.fired {
				t.Fatal("crash point was not reached")
			}

			// Rebuild every process-owned actor before recovery. The validation
			// claim path may replay a safe read only after the fresh owner proves
			// no receipt exists; the durable state-change path must consume its
			// persisted receipt without a second write.
			workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
			if err := fileOwner.Stop(ctx); err != nil {
				t.Fatal(err)
			}
			if err := runOwner.Stop(ctx); err != nil {
				t.Fatal(err)
			}
			hub.Terminate(ctx)

			restartedHub := magicevent.NewHub(32)
			t.Cleanup(func() { restartedHub.Terminate(ctx) })
			restartedRunOwner := runmodule.New()
			if err := restartedRunOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: restartedHub}); err != nil {
				t.Fatal(err)
			}
			if err := restartedRunOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = restartedRunOwner.Stop(ctx) })
			startRuntimeV2ReceiptTestOwner(t, restartedHub)
			restartedWorkspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, restartedHub)
			restartedWorkspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
				result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
			})
			t.Cleanup(func() { restartedWorkspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
			restartedFileOwner := filemodule.New()
			if err := restartedFileOwner.Configure(filemodule.Dependencies{EventHub: restartedHub}); err != nil {
				t.Fatal(err)
			}
			if err := restartedFileOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = restartedFileOwner.Stop(ctx) })
			restartedRepository := runtimeV2StateRepository{hub: restartedHub}
			restarted, err := runtimev2.NewRuntimeController(restartedRepository)
			if err != nil {
				t.Fatal(err)
			}
			loaded, err := restarted.Load(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			if loaded.Decision.Action != runtimecontract.RuntimeActionComplete {
				loaded, err = restarted.ExecuteActiveDispatch(ctx, "ws-v2", "run-v2", eventRuntimeV2OperationExecutor{hub: restartedHub})
				if err != nil {
					t.Fatal(err)
				}
			}
			if loaded.Decision.Action != runtimecontract.RuntimeActionComplete || len(loaded.OperationReceipts) != 1 || len(loaded.Facts.Evidence.Accepted) != 1 || loaded.EvidenceSequence != 1 {
				t.Fatalf("restart exactly-once aggregate=%+v", loaded)
			}
			if testCase.validation && testCase.after && loaded.Facts.Recovery.OperationRecoveryAttempts != 1 {
				t.Fatalf("claimed safe recovery ledger=%+v", loaded.Facts.Recovery)
			}
			content, readErr := os.ReadFile(filepath.Join(deliverable, "target.txt"))
			want := "old\n"
			if !testCase.validation {
				want = "new\n"
			}
			if readErr != nil || string(content) != want {
				t.Fatalf("restart content=%q want=%q err=%v", string(content), want, readErr)
			}
		})
	}
}

// P4-C: an accepted receipt closes coverage and advances the current focus in
// the same SM-2 evidence CAS. Exercise the exact boundary before and after the
// Run-owner save, then reconstruct every actor. The new process may consume
// the persisted receipt, but must never execute it again or leave focus one as
// current authority.
func TestRuntimeV2FileBackedEvidenceCoverageFocusAdvanceCrashRestart(t *testing.T) {
	for _, crashAfterCAS := range []bool{false, true} {
		phase := "before_evidence_focus_cas"
		if crashAfterCAS {
			phase = "after_evidence_focus_cas"
		}
		t.Run(phase, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			runsPath := filepath.Join(root, "runs.json")
			deliverable := filepath.Join(root, "workspace")
			if err := os.MkdirAll(deliverable, 0o755); err != nil {
				t.Fatal(err)
			}
			bootstrap := runtimeV2Bootstrap()
			bootstrap.Focuses = []runtimecontract.CoverageFocus{
				{ID: "focus-1", SliceID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, TargetRefs: []string{"one.txt"}},
				{ID: "focus-2", SliceID: "slice-2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-2"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, TargetRefs: []string{"two.txt"}},
			}
			bootstrap.Slices = []runtimecontract.RuntimeAcceptanceSlice{
				{ID: "slice-1", Title: "one", Scope: "fixture", Priority: 1, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, CompletionPolicy: runtimecontract.RuntimeAcceptanceSliceCompletionAll, ResultPolicy: runtimecontract.RuntimeAcceptanceSliceResultAllowed},
				{ID: "slice-2", Title: "two", Scope: "fixture", Priority: 2, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-2"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, CompletionPolicy: runtimecontract.RuntimeAcceptanceSliceCompletionAll, ResultPolicy: runtimecontract.RuntimeAcceptanceSliceResultAllowed},
			}
			bootstrap.CoverageItems = []runtimecontract.RuntimeCoverageItem{{ID: "cwi-1", SliceID: "slice-1"}, {ID: "cwi-2", SliceID: "slice-2"}}
			if err := filerepository.NewRunRepository(runsPath).Create(ctx, &runmodel.Run{
				ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
				DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, State: common.RunStateActive,
			}); err != nil {
				t.Fatal(err)
			}

			type process struct {
				hub               magicevent.Hub
				runs              *filerepository.RunRepository
				runOwner          *runmodule.Module
				file              *filemodule.Module
				workspaceObserver magicevent.SimpleObserver
			}
			startProcess := func(t *testing.T) process {
				t.Helper()
				hub := magicevent.NewHub(16)
				runs := filerepository.NewRunRepository(runsPath)
				runOwner := runmodule.New()
				if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := runOwner.Start(); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				startRuntimeV2ReceiptTestOwner(t, hub)
				workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
				workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
					result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable}}, nil)
				})
				fileOwner := filemodule.New()
				if err := fileOwner.Configure(filemodule.Dependencies{EventHub: hub}); err != nil {
					workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
					_ = runOwner.Stop(ctx)
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := fileOwner.Start(); err != nil {
					workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
					_ = runOwner.Stop(ctx)
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				return process{hub: hub, runs: runs, runOwner: runOwner, file: fileOwner, workspaceObserver: workspaceObserver}
			}
			stopProcess := func(value process) {
				if value.file != nil {
					_ = value.file.Stop(ctx)
				}
				if value.workspaceObserver != nil {
					value.workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
				}
				if value.runOwner != nil {
					_ = value.runOwner.Stop(ctx)
				}
				if value.hub != nil {
					value.hub.Terminate(ctx)
				}
			}

			first := startProcess(t)
			module := newRuntimeV2DriveTestModule(t, first.hub)
			run, err := first.runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			aggregate, err := module.bizPtr.InitializeRuntimeV2FromRun(ctx, run)
			if err != nil {
				t.Fatal(err)
			}
			aggregate, err = module.prepareRuntimeV2Prerequisites(ctx, aggregate)
			if err != nil {
				t.Fatal(err)
			}
			prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(aggregate.Definition, aggregate.Facts.Activation.ExecutionContract)
			if _, err := module.bizPtr.BeginRuntimeV2CandidateAttemptForTest(ctx, "ws-v2", "run-v2", 0, prefix); err != nil {
				t.Fatal(err)
			}
			target := "one.txt"
			sealed, err := module.bizPtr.AcceptRuntimeV2CandidateAttempt(ctx, "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
				ID: "write-focus-1", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{target}, Parameters: runtimeV2FileEditParameters(target),
			}}}}})
			if err != nil || sealed.ActiveDispatch == nil {
				t.Fatalf("sealed focus-one dispatch=%+v err=%v", sealed, err)
			}

			marker := "evidence-coverage-focus-before-cas"
			if crashAfterCAS {
				marker = "evidence-coverage-focus-after-cas"
			}
			crashRepository := &runtimeV2SaveCrashRepository{
				delegate: runtimeV2StateRepository{hub: first.hub}, after: crashAfterCAS, marker: marker,
				match: func(current runtimev2.RuntimeAggregate) bool {
					return current.EvidenceSequence == 1 && current.Facts.Focus.ID == "focus-2" && current.ActiveDispatch == nil &&
						len(current.Facts.Evidence.Accepted) == 1 && len(current.Facts.CompletedFocuses) == 1 && current.Facts.CompletedFocuses[0].ID == "focus-1"
				},
			}
			crashingController, err := runtimev2.NewRuntimeController(crashRepository)
			if err != nil {
				t.Fatal(err)
			}
			executor := &runtimeV2FocusAdvanceReceiptExecutor{owner: crashingController}
			func() {
				defer func() {
					if recovered := recover(); recovered != marker {
						t.Fatalf("unexpected evidence/focus crash marker: %v", recovered)
					}
				}()
				_, _ = crashingController.ExecuteActiveDispatch(ctx, "ws-v2", "run-v2", executor)
				t.Fatal("expected evidence/coverage/focus CAS crash")
			}()
			if !crashRepository.fired || executor.calls != 1 {
				t.Fatalf("focus-one receipt was not committed exactly once: crash=%t calls=%d", crashRepository.fired, executor.calls)
			}
			crashed, err := first.runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil || crashed.RuntimeState == nil {
				t.Fatalf("evidence/focus crash state=%+v err=%v", crashed, err)
			}
			if crashAfterCAS {
				if crashed.RuntimeState.EvidenceSequence != 1 || crashed.RuntimeState.Facts.Focus.ID != "focus-2" ||
					len(crashed.RuntimeState.Facts.Evidence.Accepted) != 1 || len(crashed.RuntimeState.Facts.CompletedFocuses) != 1 {
					t.Fatalf("after-CAS evidence/focus state was not atomic: %+v", crashed.RuntimeState)
				}
			} else if crashed.RuntimeState.EvidenceSequence != 0 || crashed.RuntimeState.Facts.Focus.ID != "focus-1" ||
				len(crashed.RuntimeState.Facts.Evidence.Accepted) != 0 || len(crashed.RuntimeState.Facts.CompletedFocuses) != 0 ||
				crashed.RuntimeState.ActiveDispatch == nil || len(crashed.RuntimeState.OperationReceipts) != 1 {
				t.Fatalf("before-CAS evidence/focus transition leaked or lost its receipt: %+v", crashed.RuntimeState)
			}
			stopProcess(first)

			second := startProcess(t)
			defer stopProcess(second)
			restarted, err := runtimev2.NewRuntimeController(runtimeV2StateRepository{hub: second.hub})
			if err != nil {
				t.Fatal(err)
			}
			loaded, err := restarted.Load(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			if loaded.ActiveDispatch != nil {
				noExecute := &runtimeV2FailIfOperationExecuted{}
				loaded, err = restarted.ExecuteActiveDispatch(ctx, "ws-v2", "run-v2", noExecute)
				if err != nil {
					t.Fatal(err)
				}
				if noExecute.calls != 0 {
					t.Fatalf("fresh process replayed an owner-committed receipt %d time(s)", noExecute.calls)
				}
			}
			if loaded.EvidenceSequence != 1 || loaded.Facts.Focus.ID != "focus-2" || len(loaded.Facts.RemainingFocuses) != 0 ||
				len(loaded.Facts.CompletedFocuses) != 1 || loaded.Facts.CompletedFocuses[0].ID != "focus-1" ||
				len(loaded.Facts.Evidence.Accepted) != 1 || loaded.ActiveDispatch != nil || len(loaded.OperationReceipts) != 1 {
				t.Fatalf("fresh process did not converge evidence/coverage/focus exactly once: %+v", loaded)
			}
			for _, item := range loaded.Facts.Coverage {
				if item.ID == "cwi-1" && item.State != runtimecontract.RuntimeCoverageItemStateSatisfied {
					t.Fatalf("focus-one coverage was not closed by accepted evidence: %+v", item)
				}
			}
			stateRevision := loaded.StateRevision
			receipt := loaded.OperationReceipts[0]
			persistedEvidence := loaded.Facts.Evidence.Accepted[0]
			evidence := runtimev2.ExecutionEvidence{
				EventID: string(persistedEvidence.ID), WorkspaceID: "ws-v2", RunID: "run-v2",
				DispatchID: sealed.ActiveDispatch.ID, AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
				OperationID: "write-focus-1", Kind: runtimecontract.EvidenceKindStateChange, Status: runtimev2.ExecutionEvidenceStatusAccepted,
				SourceRef: persistedEvidence.SourceRef, Evidence: receipt.Evidence,
				ObservedEffectScope: receipt.ObservedEffectScope, OwnerReceiptRef: "runtime-receipt/" + receipt.ExecutionKey,
				OwnerReceiptHash: runtimecontract.StableRuntimeOperationReceiptHash(receipt), Receipt: &receipt,
			}
			duplicate, err := restarted.CommitEvidence(ctx, evidence)
			if err != nil || duplicate.Disposition != runtimev2.CommitDispositionDuplicate || duplicate.Aggregate.StateRevision != stateRevision {
				t.Fatalf("duplicate evidence changed focused authority: result=%+v err=%v", duplicate, err)
			}
		})
	}
}

// TestRuntimeV2FileBackedOperationGovernanceGrantFreshProcessCrashRestart
// proves the operation-scoped governance hand-off at the production owner
// boundary. It intentionally stops every actor between the grant CAS and the
// resumed write: the new process must recover only the ticket/binding/receipt
// persisted by the Run owner, preserve ExecutionKey, advance the fence once,
// and pass the exact grant back to the File owner.
//
// Affected machines: SM-2 records the governance fact and grant, SM-3 seals
// the original/resume dispatches and executes the fence, and SM-4 reloads the
// aggregate after process loss. SM-1 projection is deliberately out of scope
// here and remains P5-D.
func TestRuntimeV2FileBackedOperationGovernanceGrantFreshProcessCrashRestart(t *testing.T) {
	ctx := context.Background()

	for _, crashAfterCAS := range []bool{false, true} {
		name := "before_grant_cas"
		if crashAfterCAS {
			name = "after_grant_cas_response_loss"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			deliverable := filepath.Join(root, "workspace")
			internalDirectory := filepath.Join(root, "internal")
			if err := os.MkdirAll(deliverable, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(internalDirectory, 0o755); err != nil {
				t.Fatal(err)
			}
			runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
			decisionTickets := filerepository.NewDecisionTicketRepository(filepath.Join(root, "decision-tickets.json"))
			decisionResolutions := filerepository.NewDecisionResolutionRepository(filepath.Join(root, "decision-resolutions.json"))
			bootstrap := runtimeV2Bootstrap()
			bootstrap.Definition.CapabilityContract = []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindDecisionRequest}
			bootstrap.Focuses[0].TargetRefs = []string{".env"}
			bootstrap.Focuses[0].AcceptedStepKinds = []string{runtimecontract.StepKindFileWrite}
			bootstrap.Requirements = []runtimecontract.RuntimeRequirement{
				{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleFile, CapabilityClass: runtimecontract.RuntimeCapabilityClassStateChange, RequiredCapabilities: []string{runtimecontract.StepKindFileWrite}},
				{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleDecision, CapabilityClass: runtimecontract.RuntimeCapabilityClassGovernance, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindDecisionRequest}},
			}
			if err := runs.Create(ctx, &runmodel.Run{
				ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
				DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, State: common.RunStateActive,
			}); err != nil {
				t.Fatal(err)
			}

			type process struct {
				hub           magicevent.Hub
				runOwner      *runmodule.Module
				decisionOwner *decisionmodule.Module
				fileOwner     *filemodule.Module
				workspace     magicevent.SimpleObserver
				receipts      magicevent.SimpleObserver
				module        *Biz
				controller    *runtimev2.RuntimeController
			}
			startProcess := func(t *testing.T) process {
				t.Helper()
				hub := magicevent.NewHub(16)
				runOwner := runmodule.New()
				if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := runOwner.Start(); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				decisionOwner := decisionmodule.New()
				if err := decisionOwner.Configure(decisionmodule.Dependencies{Tickets: decisionTickets, Resolutions: decisionResolutions, EventHub: hub}); err != nil {
					_ = runOwner.Stop(ctx)
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := decisionOwner.Start(); err != nil {
					_ = runOwner.Stop(ctx)
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				workspace := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
				workspace.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
					result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{
						ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDirectory,
					}}, nil)
				})
				fileOwner := filemodule.New()
				if err := fileOwner.Configure(filemodule.Dependencies{EventHub: hub}); err != nil {
					workspace.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
					_ = decisionOwner.Stop(ctx)
					_ = runOwner.Stop(ctx)
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := fileOwner.Start(); err != nil {
					workspace.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
					_ = decisionOwner.Stop(ctx)
					_ = runOwner.Stop(ctx)
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				owner := orchestratorbiz.NewRuntimeOrchestrator()
				if err := owner.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
					_ = fileOwner.Stop(ctx)
					workspace.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
					_ = decisionOwner.Stop(ctx)
					_ = runOwner.Stop(ctx)
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				module := newTestBiz(owner, hub, nil)
				receipts := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
				receipts.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, module.handleRuntimeV2OperationReceipt)
				receipts.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, module.handleRuntimeV2OperationReceipt)
				return process{hub: hub, runOwner: runOwner, decisionOwner: decisionOwner, fileOwner: fileOwner, workspace: workspace, receipts: receipts, module: module, controller: owner.runtimeV2Controller}
			}
			stopProcess := func(value process) {
				if value.receipts != nil {
					value.receipts.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
					value.receipts.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
				}
				if value.fileOwner != nil {
					_ = value.fileOwner.Stop(ctx)
				}
				if value.workspace != nil {
					value.workspace.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
				}
				if value.decisionOwner != nil {
					_ = value.decisionOwner.Stop(ctx)
				}
				if value.runOwner != nil {
					_ = value.runOwner.Stop(ctx)
				}
				if value.hub != nil {
					value.hub.Terminate(ctx)
				}
			}

			first := startProcess(t)
			facts := runtimeV2Aggregate(t, 1).Facts
			facts.Focus.TargetRefs = []string{".env"}
			facts.Focus.AcceptedStepKinds = []string{runtimecontract.StepKindFileWrite}
			facts.Capabilities = runtimecontract.RuntimeCapabilities{
				StateChangeKinds: []string{runtimecontract.StepKindFileWrite},
				GovernanceKinds:  []string{runtimecontract.RuntimeBlockKindDecisionRequest},
			}
			definition := bootstrap.Definition
			initialized, err := first.controller.Initialize(ctx, facts, definition, nil)
			if err != nil || initialized.Decision.Action != runtimecontract.RuntimeActionProduceStateChange {
				stopProcess(first)
				t.Fatalf("governance fixture initialization=%+v err=%v", initialized, err)
			}
			if _, err := beginRuntimeV2CandidateAttemptForTest(ctx, first.controller, "ws-v2", "run-v2", 0, "governance-grant-prefix"); err != nil {
				stopProcess(first)
				t.Fatal(err)
			}
			target, content := ".env", "TOKEN=granted\n"
			candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
				ID: "write-governed", Kind: runtimecontract.StepKindFileWrite, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{target},
				Parameters: []runtimecontract.OperationParameter{{Name: "content", String: &content}},
			}}}}}
			sealed, err := admitAndSealRuntimeV2CandidateAttemptForTest(ctx, first.controller, "ws-v2", "run-v2", 0, candidate)
			if err != nil || sealed.Aggregate.ActiveDispatch == nil {
				stopProcess(first)
				t.Fatalf("governed write seal=%+v err=%v", sealed, err)
			}
			executor := &countingRuntimeV2OperationExecutor{delegate: eventRuntimeV2OperationExecutor{hub: first.hub}}
			waiting, err := first.controller.ExecuteActiveDispatch(ctx, "ws-v2", "run-v2", executor)
			if err != nil || waiting.ActiveDispatch != nil || waiting.Decision.Action != runtimecontract.RuntimeActionRequestGovernance || len(waiting.OperationGovernanceBindings) != 1 || len(executor.requests) != 1 {
				stopProcess(first)
				t.Fatalf("governance-required receipt did not suspend operation: aggregate=%+v calls=%d err=%v", waiting, len(executor.requests), err)
			}
			binding := waiting.OperationGovernanceBindings[0].Normalized()
			if binding.State != runtimecontract.RuntimeOperationGovernanceBindingSuspended || binding.ExecutionKey == "" || binding.Fence == 0 || binding.GovernanceReceiptHash == "" {
				stopProcess(first)
				t.Fatalf("suspended governance binding=%+v", binding)
			}
			manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(waiting.Decision, runtimecontract.RuntimeCandidateManifestOwnerFacts{})
			if err != nil {
				stopProcess(first)
				t.Fatal(err)
			}
			governed, err := first.controller.SealGovernanceDispatch(ctx, "ws-v2", "run-v2", manifest, runtimecontract.RuntimeCandidate{Governance: &runtimecontract.GovernanceCandidate{ExternalDependency: runtimecontract.ExternalDependency{
				Kind: binding.DependencyKind, SubjectRef: binding.SubjectRef, Reason: waiting.Facts.Failure.GovernanceDependency.Reason,
			}}})
			if err != nil || governed.ActiveDispatch == nil {
				stopProcess(first)
				t.Fatalf("governance dispatch seal=%+v err=%v", governed, err)
			}
			bound, err := first.module.prepareRuntimeV2GovernanceRequest(ctx, governed)
			if err != nil || len(bound.GovernanceRequests) != 1 || len(bound.OperationGovernanceBindings) != 1 || bound.OperationGovernanceBindings[0].State != runtimecontract.RuntimeOperationGovernanceBindingTicketBound {
				stopProcess(first)
				t.Fatalf("governance ticket binding=%+v err=%v", bound, err)
			}
			request := bound.GovernanceRequests[0]
			if _, err := decisionTickets.Get(ctx, request.TicketID); err != nil {
				stopProcess(first)
				t.Fatalf("decision owner did not persist governed ticket %q: %v", request.TicketID, err)
			}
			resolution := runtimecontract.RuntimeGovernanceResolution{
				ResolutionID: "governance-grant-resolution", TicketID: request.TicketID, WorkspaceID: "ws-v2", RunID: "run-v2",
				AuthoritySnapshotID: request.AuthoritySnapshotID, DispatchID: request.DispatchID,
				DependencyKind: request.DependencyKind, SubjectRef: request.SubjectRef,
				Outcome: runtimecontract.RuntimeGovernanceResolutionOutcomeContinue, Summary: "persisted approval granted",
			}
			marker := "governance-grant-before-cas"
			if crashAfterCAS {
				marker = "governance-grant-after-cas"
			}
			crashRepository := &runtimeV2SaveCrashRepository{
				delegate: runtimeV2StateRepository{hub: first.hub}, after: crashAfterCAS, marker: marker,
				match: func(aggregate runtimev2.RuntimeAggregate) bool {
					return aggregate.ActiveDispatch != nil && len(aggregate.OperationGovernanceBindings) == 1 &&
						aggregate.OperationGovernanceBindings[0].State == runtimecontract.RuntimeOperationGovernanceBindingGranted
				},
			}
			crashing, err := runtimev2.NewRuntimeController(crashRepository)
			if err != nil {
				stopProcess(first)
				t.Fatal(err)
			}
			func() {
				defer func() {
					if recovered := recover(); recovered != marker {
						stopProcess(first)
						t.Fatalf("unexpected governance grant crash marker: %v", recovered)
					}
				}()
				_, _ = crashing.CommitGovernanceResolution(ctx, resolution)
				t.Fatal("expected governance grant CAS interruption")
			}()
			if !crashRepository.fired {
				stopProcess(first)
				t.Fatal("governance grant CAS interruption was not reached")
			}
			stopProcess(first)

			second := startProcess(t)
			defer stopProcess(second)
			continued, err := second.controller.CommitGovernanceResolution(ctx, resolution)
			if err != nil {
				t.Fatal(err)
			}
			wantDisposition := runtimev2.CommitDispositionCommitted
			if crashAfterCAS {
				wantDisposition = runtimev2.CommitDispositionDuplicate
			}
			if continued.Disposition != wantDisposition || continued.Aggregate.ActiveDispatch == nil || len(continued.Aggregate.OperationGovernanceBindings) != 1 {
				t.Fatalf("fresh-process governance continuation=%+v, want disposition=%s", continued, wantDisposition)
			}
			grantBinding := continued.Aggregate.OperationGovernanceBindings[0].Normalized()
			if grantBinding.State != runtimecontract.RuntimeOperationGovernanceBindingGranted || grantBinding.Grant == nil ||
				grantBinding.ExecutionKey != binding.ExecutionKey || grantBinding.Grant.GovernanceFence != binding.Fence || grantBinding.Grant.ResumeFence != binding.Fence+1 {
				t.Fatalf("persisted governance grant=%+v initial binding=%+v", grantBinding, binding)
			}
			resumeExecution, found := runtimeV2OperationExecution(continued.Aggregate.ActiveDispatch.OperationExecutions, "write-governed")
			if !found || resumeExecution.ExecutionKey != binding.ExecutionKey || resumeExecution.Fence != binding.Fence+1 || resumeExecution.State != runtimecontract.OperationExecutionClaimed {
				t.Fatalf("fresh-process resume execution=%+v binding=%+v", resumeExecution, binding)
			}
			if _, _, err := second.controller.QueryOperationReceipt(ctx, "ws-v2", "run-v2", continued.Aggregate.ActiveDispatch.ID, "write-governed", binding.ExecutionKey, binding.Fence, grantBinding.Grant); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("old fence must not consume the resume grant: %v", err)
			}
			stale := resolution
			stale.ResolutionID = "governance-grant-stale"
			stale.AuthoritySnapshotID = "stale-governance-authority"
			staleCommit, err := second.controller.CommitGovernanceResolution(ctx, stale)
			if err != nil || staleCommit.Disposition != runtimev2.CommitDispositionStale || staleCommit.Aggregate.ActiveDispatch == nil {
				t.Fatalf("stale governance resolution=%+v err=%v", staleCommit, err)
			}
			duplicate, err := second.controller.CommitGovernanceResolution(ctx, resolution)
			if err != nil || duplicate.Disposition != runtimev2.CommitDispositionDuplicate || duplicate.Aggregate.ActiveDispatch == nil {
				t.Fatalf("duplicate governance resolution=%+v err=%v", duplicate, err)
			}

			executor.delegate.hub = second.hub
			completed, err := second.controller.ExecuteActiveDispatch(ctx, "ws-v2", "run-v2", executor)
			if err != nil || completed.ActiveDispatch != nil || len(executor.requests) != 2 {
				t.Fatalf("granted operation execution=%+v calls=%d err=%v", completed, len(executor.requests), err)
			}
			if executor.requests[0].GovernanceGrant != nil || executor.requests[1].GovernanceGrant == nil ||
				executor.requests[1].Execution.ExecutionKey != binding.ExecutionKey || executor.requests[1].Execution.Fence != binding.Fence+1 {
				t.Fatalf("owner execution grant/fence sequence=%+v", executor.requests)
			}
			writtenContent, err := os.ReadFile(filepath.Join(deliverable, ".env"))
			if err != nil || string(writtenContent) != "TOKEN=granted\n" {
				t.Fatalf("granted File owner effect content=%q err=%v", string(writtenContent), err)
			}
			if len(completed.OperationReceipts) != 2 {
				t.Fatalf("governance/resume receipt lineage=%+v", completed.OperationReceipts)
			}
			if receipt, ok := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(completed.OperationReceipts, binding.ExecutionKey, binding.Fence); !ok || receipt.Outcome != runtimecontract.RuntimeOperationOutcomeGovernanceRequired {
				t.Fatalf("governance receipt=%+v found=%t", receipt, ok)
			}
			if receipt, ok := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(completed.OperationReceipts, binding.ExecutionKey, binding.Fence+1); !ok || receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted {
				t.Fatalf("granted resume receipt=%+v found=%t", receipt, ok)
			}
			if _, err := second.controller.CommitGovernanceResolution(ctx, resolution); err != nil || len(executor.requests) != 2 {
				t.Fatalf("post-effect governance duplicate must not replay File owner: calls=%d err=%v", len(executor.requests), err)
			}
		})
	}
}

func runtimeV2OperationExecution(values []runtimecontract.RuntimeOperationExecutionRecord, operationID string) (runtimecontract.RuntimeOperationExecutionRecord, bool) {
	for _, value := range values {
		value = value.Normalized()
		if value.OperationID == operationID {
			return value, true
		}
	}
	return runtimecontract.RuntimeOperationExecutionRecord{}, false
}

func TestDriveRuntimeV2FileBackedTwoFocusStateValidationAssessmentAndRestart(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	root := t.TempDir()
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	workspaces := filerepository.NewWorkspaceRepository(filepath.Join(root, "workspaces.json"))
	results := filerepository.NewResultRepository(filepath.Join(root, "results.json"))
	failingResults := &failOnceResultRepository{ResultRepository: results, remainingFailures: 1}
	artifacts := filerepository.NewArtifactRepository(filepath.Join(root, "artifacts.json"))
	decisionTickets := filerepository.NewDecisionTicketRepository(filepath.Join(root, "decision-tickets.json"))
	decisionResolutions := filerepository.NewDecisionResolutionRepository(filepath.Join(root, "decision-resolutions.json"))
	bootstrap := runtimeV2Bootstrap()
	bootstrap.Definition.CapabilityContract = []string{runtimecontract.StepKindFileEdit, runtimecontract.RuntimeBlockKindPatchApply, runtimecontract.StepKindFileRead, runtimecontract.RuntimeBlockKindDecisionRequest}
	bootstrap.Focuses = []runtimecontract.CoverageFocus{
		{ID: "focus-1", SliceID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}, TargetRefs: []string{"focus-1.txt"}, RequireAssessment: true},
		{ID: "focus-2", SliceID: "slice-2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-2"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}, TargetRefs: []string{".env"}, RequireAssessment: true},
	}
	bootstrap.Slices = []runtimecontract.RuntimeAcceptanceSlice{
		{ID: "slice-1", Title: "Focus 1", Scope: "test", Priority: 1, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}, CompletionPolicy: runtimecontract.RuntimeAcceptanceSliceCompletionAll, ResultPolicy: runtimecontract.RuntimeAcceptanceSliceResultAllowed},
		{ID: "slice-2", Title: "Focus 2", Scope: "test", Priority: 1, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-2"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}, CompletionPolicy: runtimecontract.RuntimeAcceptanceSliceCompletionAll, ResultPolicy: runtimecontract.RuntimeAcceptanceSliceResultAllowed},
	}
	bootstrap.CoverageItems = []runtimecontract.RuntimeCoverageItem{{ID: "cwi-1", SliceID: "slice-1"}, {ID: "cwi-2", SliceID: "slice-2"}}
	bootstrap.Requirements = []runtimecontract.RuntimeRequirement{
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleFile, CapabilityClass: runtimecontract.RuntimeCapabilityClassStateChange, RequiredCapabilities: []string{runtimecontract.StepKindFileEdit, runtimecontract.RuntimeBlockKindPatchApply}},
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleFile, CapabilityClass: runtimecontract.RuntimeCapabilityClassValidation, RequiredCapabilities: []string{runtimecontract.StepKindFileRead}},
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleDecision, CapabilityClass: runtimecontract.RuntimeCapabilityClassGovernance, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindDecisionRequest}},
	}
	deliverable := filepath.Join(root, "workspace")
	internalDir := filepath.Join(root, "internal")
	if err := os.MkdirAll(deliverable, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(internalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2",
		DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, State: common.RunStateActive,
	}); err != nil {
		t.Fatal(err)
	}
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir, State: common.WorkspaceStateActive, ActiveRunID: "run-v2"}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	workspaceOwner := workspacemodule.New()
	if err := workspaceOwner.Configure(workspacemodule.Dependencies{
		Workspaces: workspaces, Goals: memory.NewGoalRepository(), DoDs: memory.NewDoDRepository(),
		Templates: memory.NewWorkspaceTemplateRepository(), EventHub: hub,
	}); err != nil {
		t.Fatal(err)
	}
	if err := workspaceOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = workspaceOwner.Stop(ctx) })
	startRuntimeV2ReceiptTestOwner(t, hub)
	decisionOwner := decisionmodule.New()
	if err := decisionOwner.Configure(decisionmodule.Dependencies{Tickets: decisionTickets, Resolutions: decisionResolutions, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := decisionOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = decisionOwner.Stop(ctx) })

	if err := os.WriteFile(filepath.Join(deliverable, "focus-1.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fileOwner := filemodule.New()
	if err := fileOwner.Configure(filemodule.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := fileOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fileOwner.Stop(ctx) })
	resultOwner := resultmodule.New()
	if err := resultOwner.Configure(resultmodule.Dependencies{Results: failingResults, Artifacts: artifacts, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := resultOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resultOwner.Stop(ctx) })

	candidateCount := 0
	providerTransientInjected := false
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		candidateCount++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		focus := command.Decision.Focus
		coverageID := focus.CoverageWorkItemIDs[0]
		target := focus.TargetRefs[0]
		response := orchestratorevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, LogicalRequestID: command.LogicalRequestID, RequestSequence: command.RequestSequence, PhysicalCallID: command.PhysicalCallID}
		var candidate runtimecontract.RuntimeCandidate
		if focus.ID == "focus-2" && command.Decision.Action == runtimecontract.RuntimeActionProduceStateChange && !providerTransientInjected {
			providerTransientInjected = true
			response.Failure = &runtimecontract.RuntimeCandidateFailure{Class: runtimecontract.RuntimeCandidateFailureClassProviderTransient, Reason: "injected provider transport interruption"}
			result.Set(response, nil)
			return
		}
		switch command.Decision.Action {
		case runtimecontract.RuntimeActionProduceStateChange:
			if focus.ID == "focus-2" {
				patchType, content := "file_write", "TOKEN=granted\n"
				candidate = runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
					ID: "patch-" + focus.ID, Kind: runtimecontract.RuntimeBlockKindPatchApply,
					CoverageWorkItemIDs: []common.CoverageWorkItemID{coverageID}, TargetRefs: []string{target},
					Parameters: []runtimecontract.OperationParameter{{Name: "patchType", String: &patchType}, {Name: "content", String: &content}},
				}}}}}
			} else {
				oldText, newText := "old\n", "new\n"
				candidate = runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
					ID: "edit-" + focus.ID, Kind: runtimecontract.StepKindFileEdit,
					CoverageWorkItemIDs: []common.CoverageWorkItemID{coverageID}, TargetRefs: []string{target},
					Parameters: []runtimecontract.OperationParameter{{Name: "oldText", String: &oldText}, {Name: "newText", String: &newText}},
				}}}}}
			}
		case runtimecontract.RuntimeActionProduceValidation:
			candidate = runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: runtimecontract.RuntimeOperation{
				ID: "read-" + focus.ID, Kind: runtimecontract.StepKindFileRead,
				CoverageWorkItemIDs: []common.CoverageWorkItemID{coverageID}, TargetRefs: []string{target},
				Parameters: nil,
			}}}}}
		case runtimecontract.RuntimeActionProduceAssessment:
			refs := make([]string, 0)
			for _, evidence := range command.CandidateContext.Evidence.Accepted {
				for _, evidenceCoverageID := range evidence.CoverageWorkItemIDs {
					if evidenceCoverageID == coverageID {
						refs = append(refs, string(evidence.ID))
						break
					}
				}
			}
			candidate = runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{
				FocusID: focus.ID, CoverageWorkItemIDs: []common.CoverageWorkItemID{coverageID},
				Disposition: runtimecontract.AssessmentDispositionSatisfied, Summary: focus.ID + " satisfied",
				Items: []runtimecontract.RuntimeAssessmentItemFact{{CoverageWorkItemID: coverageID, Satisfied: true}}, EvidenceRefs: refs,
			}}
		default:
			t.Errorf("unexpected candidate action %s", command.Decision.Action)
			return
		}
		response.DocumentContent = runtimeV2CandidateDocumentForTest(t, command.CandidateManifest, candidate)
		result.Set(response, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })

	moduleBiz := orchestratorbiz.NewRuntimeOrchestrator()
	casRepository := &failOnceCASRuntimeV2Repository{delegate: runtimeV2StateRepository{hub: hub}, remainingFailures: 1}
	if err := moduleBiz.ConfigureRuntimeV2Repository(casRepository); err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(moduleBiz, hub, nil)
	driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, module.handleRuntimeV2Drive)
	command := orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"}
	if err := module.scheduleRuntimeV2Drive(ctx, "ws-v2", "run-v2", "cas-conflict-injection"); err != nil {
		t.Fatal(err)
	}
	casRecovered, err := module.bizPtr.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
	if err != nil || casRecovered.Facts.Recovery.CASReloadAttempts != 1 || casRecovered.Facts.Failure != nil {
		t.Fatalf("CAS recovery aggregate=%+v err=%v", casRecovered, err)
	}
	waiting, err := DriveRuntimeV2(ctx, hub, command)
	if err != nil {
		t.Fatal(err)
	}
	if waiting.Disposition != orchestratorevents.RuntimeV2DriveDispositionGovernedWait || waiting.BlockingTicketID == "" || candidateCount != 5 {
		t.Fatalf("two-focus governance wait=%+v candidates=%d", waiting, candidateCount)
	}
	governed, err := module.bizPtr.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
	if err != nil || governed.ActiveDispatch == nil || len(governed.OperationGovernanceBindings) != 1 {
		t.Fatalf("governed aggregate=%+v err=%v", governed, err)
	}
	binding := governed.OperationGovernanceBindings[0]
	request := governed.GovernanceRequests[len(governed.GovernanceRequests)-1]
	staleResolution := runtimecontract.RuntimeGovernanceResolution{
		ResolutionID: "resolution-focus-2-stale", TicketID: request.TicketID, WorkspaceID: "ws-v2", RunID: "run-v2",
		AuthoritySnapshotID: "stale-governance-authority", DispatchID: request.DispatchID,
		DependencyKind: request.DependencyKind, SubjectRef: request.SubjectRef,
		Outcome: runtimecontract.RuntimeGovernanceResolutionOutcomeContinue, Summary: "stale approval",
	}
	staleCommit, err := module.bizPtr.CommitRuntimeV2GovernanceResolution(ctx, staleResolution)
	if err != nil || staleCommit.Disposition != runtimev2.CommitDispositionStale || staleCommit.Aggregate.ActiveDispatch == nil || len(staleCommit.Aggregate.Diagnostics) == 0 {
		t.Fatalf("stale governance commit=%+v err=%v", staleCommit, err)
	}
	resolution := runtimecontract.RuntimeGovernanceResolution{
		ResolutionID: "resolution-focus-2", TicketID: request.TicketID, WorkspaceID: "ws-v2", RunID: "run-v2",
		AuthoritySnapshotID: request.AuthoritySnapshotID, DispatchID: request.DispatchID,
		DependencyKind: request.DependencyKind, SubjectRef: request.SubjectRef,
		Outcome: runtimecontract.RuntimeGovernanceResolutionOutcomeContinue, Summary: "file patch approved",
	}
	grantCrashRepository := &runtimeV2SaveCrashRepository{
		delegate: runtimeV2StateRepository{hub: hub}, after: true, marker: "governance-grant-cas-after",
		match: func(aggregate runtimev2.RuntimeAggregate) bool {
			return aggregate.ActiveDispatch != nil && len(aggregate.OperationGovernanceBindings) == 1 &&
				aggregate.OperationGovernanceBindings[0].State == runtimecontract.RuntimeOperationGovernanceBindingGranted
		},
	}
	grantCrashBiz := orchestratorbiz.NewRuntimeOrchestrator()
	if err := grantCrashBiz.ConfigureRuntimeV2Repository(grantCrashRepository); err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "governance-grant-cas-after" {
				t.Fatalf("unexpected governance grant crash marker: %v", recovered)
			}
		}()
		_, _ = grantCrashBiz.CommitRuntimeV2GovernanceResolution(ctx, resolution)
		t.Fatal("expected crash after governance grant CAS")
	}()
	if !grantCrashRepository.fired {
		t.Fatal("governance grant crash point was not reached")
	}
	continued, err := module.bizPtr.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
	if err != nil || continued.ActiveDispatch == nil {
		t.Fatalf("persisted governance continuation=%+v err=%v", continued, err)
	}
	resumeExecution := continued.ActiveDispatch.OperationExecutions[0]
	if resumeExecution.ExecutionKey != binding.ExecutionKey || resumeExecution.Fence != binding.Fence+1 || resumeExecution.State != runtimecontract.OperationExecutionClaimed {
		t.Fatalf("governance resume execution=%+v binding=%+v", resumeExecution, binding)
	}
	duplicateContinuation, err := module.bizPtr.CommitRuntimeV2GovernanceResolution(ctx, resolution)
	if err != nil || duplicateContinuation.Disposition != runtimev2.CommitDispositionDuplicate || duplicateContinuation.Aggregate.ActiveDispatch == nil || duplicateContinuation.Aggregate.ActiveDispatch.ID != continued.ActiveDispatch.ID {
		t.Fatalf("duplicate governance continuation=%+v err=%v", duplicateContinuation, err)
	}
	completed, err := DriveRuntimeV2(ctx, hub, command)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Action != runtimecontract.RuntimeActionComplete || completed.Terminal == nil || candidateCount != 7 {
		t.Fatalf("two-focus completion=%+v candidates=%d", completed, candidateCount)
	}
	persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.RuntimeState == nil {
		t.Fatalf("persisted two-focus run=%+v err=%v", persisted, err)
	}
	if len(persisted.RuntimeState.OperationReceipts) != 5 || len(persisted.RuntimeState.Facts.Assessments) != 2 {
		t.Fatalf("two-focus receipts=%d assessments=%d", len(persisted.RuntimeState.OperationReceipts), len(persisted.RuntimeState.Facts.Assessments))
	}
	if persisted.RuntimeState.Facts.Recovery.ProviderTransportAttempts != 1 {
		t.Fatalf("provider transport recovery ledger=%+v", persisted.RuntimeState.Facts.Recovery)
	}
	for _, item := range persisted.RuntimeState.Facts.Coverage {
		if item.State != runtimecontract.RuntimeCoverageItemStateSatisfied {
			t.Fatalf("coverage item not satisfied: %+v", item)
		}
	}
	content, readErr := os.ReadFile(filepath.Join(deliverable, "focus-1.txt"))
	if readErr != nil || string(content) != "new\n" {
		t.Fatalf("focus-1 content=%q err=%v", string(content), readErr)
	}
	content, readErr = os.ReadFile(filepath.Join(deliverable, ".env"))
	if readErr != nil || string(content) != "TOKEN=granted\n" {
		t.Fatalf("focus-2 governed content=%q err=%v", string(content), readErr)
	}
	if _, ok := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(persisted.RuntimeState.OperationReceipts, binding.ExecutionKey, binding.Fence); !ok {
		t.Fatal("governance receipt was not preserved")
	}
	if receipt, ok := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(persisted.RuntimeState.OperationReceipts, binding.ExecutionKey, binding.Fence+1); !ok || receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted {
		t.Fatalf("governance resume receipt=%+v found=%v", receipt, ok)
	}
	driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2)
	restarted := newRuntimeV2DriveTestModule(t, hub)
	driveObserver = magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, restarted.handleRuntimeV2Drive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2) })
	replayed, err := DriveRuntimeV2(ctx, hub, command)
	if err != nil || replayed.StateRevision != completed.StateRevision || candidateCount != 7 {
		t.Fatalf("two-focus restart=%+v completed=%+v candidates=%d err=%v", replayed, completed, candidateCount, err)
	}
	aggregate, err := restarted.bizPtr.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	deliveryBiz := orchestratorbiz.NewRuntimeOrchestrator()
	deliveryModule := newTestBiz(deliveryBiz, hub, nil)
	if err := deliveryModule.deliverRuntimeV2Terminal(ctx, aggregate); err == nil {
		t.Fatal("first terminal delivery must fail before Result persistence")
	}
	if _, err := results.Get(ctx, "ws-v2/run-v2/result/dod-assessment"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("failed terminal delivery unexpectedly persisted Result: %v", err)
	}
	recoveryAggregate, consumed, err := restarted.bizPtr.ConsumeRuntimeV2RecoveryBudget(ctx, "ws-v2", "run-v2", runtimecontract.RuntimeRecoveryBudgetTerminalDelivery)
	if err != nil || !consumed || recoveryAggregate.Facts.Recovery.TerminalDeliveryAttempts != 1 {
		t.Fatalf("terminal delivery recovery budget aggregate=%+v consumed=%v err=%v", recoveryAggregate, consumed, err)
	}
	if err := resultOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	restartedResultOwner := resultmodule.New()
	if err := restartedResultOwner.Configure(resultmodule.Dependencies{Results: results, Artifacts: artifacts, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := restartedResultOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedResultOwner.Stop(ctx) })
	if err := deliveryModule.deliverRuntimeV2Terminal(ctx, recoveryAggregate); err != nil {
		t.Fatal(err)
	}
	resultItem, err := results.Get(ctx, "ws-v2/run-v2/result/dod-assessment")
	if err != nil || resultItem.State != common.ResultStateAccepted || resultItem.RuntimeCloseout == nil || len(resultItem.RuntimeCloseout.SatisfiedCoverageWorkItemIDs) != 2 {
		t.Fatalf("two-focus result=%+v err=%v", resultItem, err)
	}
	terminalRun, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || terminalRun.State != common.RunStateCompleted {
		t.Fatalf("two-focus terminal run=%+v err=%v", terminalRun, err)
	}
	releasedWorkspace, err := workspaces.Get(ctx, "ws-v2")
	if err != nil || releasedWorkspace.ActiveRunID != "" {
		t.Fatalf("two-focus released workspace=%+v err=%v", releasedWorkspace, err)
	}
}

func TestDriveRuntimeV2CompletesTypedPathAndIsRestartIdempotent(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	bootstrap := runtimeV2Bootstrap()
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2",
		DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, State: common.RunStateActive,
	}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	startRuntimeV2ReceiptTestOwner(t, hub)

	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{
			ID: "ws-v2", DeliverableDirectory: t.TempDir(), InternalDirectory: t.TempDir(),
		}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })

	fileObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleFile, hub)
	fileObserver.Subscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeCapabilities, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.AcquireRuntimeCapabilitiesCommand)
		response, err := orchestratorevents.ResolveRuntimeCapabilities(command, runtimecontract.RuntimeModuleFile, runtimecontract.StepKindFileEdit)
		if err != nil {
			t.Errorf("resolve capabilities: %v", err)
			return
		}
		result.Set(response, nil)
	})
	operationCount := 0
	fileObserver.Subscribe(orchestratorevents.TopicOrchestratorExecuteFileEditOperation, func(event magicevent.Event, result magicevent.Result) {
		operationCount++
		command := event.Data().(orchestratorevents.ExecuteFileEditOperationCommand)
		receipt := orchestratorevents.NewRuntimeOperationReceipt(command.Execution, command.OperationID, runtimecontract.RuntimeOperationExecutionStatusAccepted, command.TargetRefs, runtimecontract.RuntimeOperationEvidence{Summary: "状态变更已完成"}, nil, nil, "2026-07-20T00:00:00Z")
		committed, err := commitRuntimeOperationReceiptForTest(event.Context(), hub, runtimecontract.RuntimeModuleFile, command.WorkspaceID, command.RunID, command.DispatchID, command.Execution, receipt)
		if err != nil {
			t.Errorf("commit owner receipt: %v", err)
			return
		}
		result.Set(orchestratorevents.ExecuteFileEditOperationResult{
			Receipt: committed,
			EventID: "event-" + command.OperationID, OperationID: command.OperationID,
			EvidenceKind: command.EvidenceKind, Status: runtimecontract.RuntimeOperationExecutionStatusAccepted,
			SourceRef: "workorch://runtime-operations/" + command.DispatchID + "/" + command.OperationID,
			Evidence:  runtimecontract.RuntimeOperationEvidence{Summary: "状态变更已完成"},
		}, nil)
	})
	t.Cleanup(func() {
		fileObserver.Unsubscribe(orchestratorevents.TopicOrchestratorExecuteFileEditOperation)
		fileObserver.Unsubscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeCapabilities)
	})

	candidateCount := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		candidateCount++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
			ID: "write-1", Kind: runtimecontract.StepKindFileEdit,
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"},
			Parameters: runtimeV2FileEditParameters("target.txt"),
		}}}}}
		result.Set(runtimeV2CandidateResultForTest(t, command, candidate), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })

	module := newRuntimeV2DriveTestModule(t, hub)
	driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, module.handleRuntimeV2Drive)

	command := orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"}
	completed, err := DriveRuntimeV2(ctx, hub, command)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Disposition != orchestratorevents.RuntimeV2DriveDispositionTerminal || completed.Action != runtimecontract.RuntimeActionComplete || completed.Terminal == nil {
		t.Fatalf("completed drive result = %+v", completed)
	}
	if candidateCount != 1 || operationCount != 1 {
		t.Fatalf("candidateCount=%d operationCount=%d, want 1/1", candidateCount, operationCount)
	}

	replayed, err := DriveRuntimeV2(ctx, hub, command)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.StateRevision != completed.StateRevision || replayed.AuthoritySnapshotID != completed.AuthoritySnapshotID || candidateCount != 1 || operationCount != 1 {
		t.Fatalf("replayed=%+v completed=%+v counts=%d/%d", replayed, completed, candidateCount, operationCount)
	}

	driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2)
	restarted := newRuntimeV2DriveTestModule(t, hub)
	driveObserver = magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, restarted.handleRuntimeV2Drive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2) })
	recovered, err := DriveRuntimeV2(ctx, hub, command)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.StateRevision != completed.StateRevision || recovered.AuthoritySnapshotID != completed.AuthoritySnapshotID || candidateCount != 1 || operationCount != 1 {
		t.Fatalf("recovered=%+v completed=%+v counts=%d/%d", recovered, completed, candidateCount, operationCount)
	}
}

func TestDriveRuntimeV2ReturnsPersistedGovernedWait(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	bootstrap := runtimeV2Bootstrap()
	bootstrap.Definition.CapabilityContract = append(bootstrap.Definition.CapabilityContract, runtimecontract.RuntimeBlockKindDecisionRequest)
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2",
		DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, State: common.RunStateActive,
	}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })

	decisionOwner := decisionmodule.New()
	if err := decisionOwner.Configure(decisionmodule.Dependencies{
		Tickets: memory.NewDecisionTicketRepository(), Resolutions: memory.NewDecisionResolutionRepository(), EventHub: hub,
	}); err != nil {
		t.Fatal(err)
	}
	if err := decisionOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = decisionOwner.Stop(ctx) })

	module := newRuntimeV2DriveTestModule(t, hub)
	facts := runtimeV2Aggregate(t, 1).Facts
	facts.Capabilities.GovernanceKinds = []string{runtimecontract.RuntimeBlockKindDecisionRequest}
	facts.Failure = &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassGovernanceDependency, Signature: "external-approval", Message: "需要外部裁决",
		GovernanceDependency: &runtimecontract.ExternalDependency{Kind: "external-approval", SubjectRef: "subject-1", Reason: "需要外部裁决"},
	}
	if _, err := module.bizPtr.InitializeRuntimeV2(ctx, facts, bootstrap.Definition, nil); err != nil {
		t.Fatal(err)
	}

	requestCount := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		requestCount++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		candidate := runtimecontract.RuntimeCandidate{Governance: &runtimecontract.GovernanceCandidate{ExternalDependency: runtimecontract.ExternalDependency{
			Kind: "external-approval", SubjectRef: "subject-1", Reason: "需要外部裁决",
		}}}
		result.Set(runtimeV2CandidateResultForTest(t, command, candidate), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })

	driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, module.handleRuntimeV2Drive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2) })
	command := orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"}
	waiting, err := DriveRuntimeV2(ctx, hub, command)
	if err != nil {
		t.Fatal(err)
	}
	if waiting.Disposition != orchestratorevents.RuntimeV2DriveDispositionGovernedWait || waiting.Action != runtimecontract.RuntimeActionRequestGovernance || waiting.BlockingTicketID == "" || requestCount != 0 {
		t.Fatalf("waiting=%+v requestCount=%d", waiting, requestCount)
	}
	replayed, err := DriveRuntimeV2(ctx, hub, command)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.StateRevision != waiting.StateRevision || replayed.BlockingTicketID != waiting.BlockingTicketID || requestCount != 0 {
		t.Fatalf("replayed=%+v waiting=%+v requestCount=%d", replayed, waiting, requestCount)
	}
}

func TestRuntimeV2ProviderAuthenticationCreatesConfigurationDependency(t *testing.T) {
	facts := runtimecontract.RuntimeFacts{
		Focus:   runtimecontract.CoverageFocus{ID: "focus-1"},
		Failure: &runtimecontract.FailureFact{Class: runtimecontract.FailureClassProviderAuthentication, Message: "invalid provider credential"},
		Resources: runtimecontract.RuntimeResourceFacts{ProviderCeiling: &runtimecontract.ProviderCeiling{
			Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "model-1"},
		}},
	}
	dependency, found := runtimev2.GovernanceDependencyForAggregate(runtimev2.RuntimeAggregate{Facts: facts})
	if !found || dependency.Kind != string(runtimecontract.GovernanceConfiguration) || dependency.SubjectRef != "ai-proxy/model-1" || dependency.Reason != "provider authentication requires configuration" {
		t.Fatalf("configuration dependency = %+v", dependency)
	}
}

func TestRuntimeV2GovernanceDependencyPreservesSuspendedOperationBinding(t *testing.T) {
	aggregate := runtimev2.RuntimeAggregate{
		Facts: runtimecontract.RuntimeFacts{
			Focus: runtimecontract.CoverageFocus{ID: "focus-1"},
			Failure: &runtimecontract.FailureFact{
				Class: runtimecontract.FailureClassGovernanceRequired, Message: "approval required",
				GovernanceDependency: &runtimecontract.ExternalDependency{Kind: string(runtimecontract.GovernanceApproval), SubjectRef: ".env", Reason: "approval required"},
			},
		},
		OperationGovernanceBindings: []runtimecontract.RuntimeOperationGovernanceBinding{{
			ExecutionKey: "exec-1", Fence: 1, DependencyKind: string(runtimecontract.GovernanceApproval), SubjectRef: ".env",
			State: runtimecontract.RuntimeOperationGovernanceBindingSuspended,
		}},
	}
	dependency, found := runtimev2.GovernanceDependencyForAggregate(aggregate)
	if !found || dependency.Kind != string(runtimecontract.GovernanceApproval) || dependency.SubjectRef != ".env" || dependency.Reason != "approval required" {
		t.Fatalf("operation governance dependency = %+v", dependency)
	}
}

func TestRuntimeV2GovernanceDependencyRejectsGenericFailure(t *testing.T) {
	aggregate := runtimev2.RuntimeAggregate{Facts: runtimecontract.RuntimeFacts{
		Focus: runtimecontract.CoverageFocus{ID: "focus-1"},
		Failure: &runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassGovernanceRequired, Message: "runtime requires governance",
		},
	}}
	if dependency, found := runtimev2.GovernanceDependencyForAggregate(aggregate); found || dependency != (runtimecontract.ExternalDependency{}) {
		t.Fatalf("generic governance dependency = %+v found=%v, want none", dependency, found)
	}
}

func TestDriveRuntimeV2CompleteSchemaFailureStopsAfterSameContractRetry(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	bootstrap := runtimeV2Bootstrap()
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2",
		DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, State: common.RunStateActive,
	}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })

	module := newRuntimeV2DriveTestModule(t, hub)
	facts := runtimeV2Aggregate(t, 1).Facts
	facts.Capabilities.GovernanceKinds = []string{runtimecontract.RuntimeBlockKindDecisionRequest}
	// Live-equivalent shape: an accepted durable operation invalidated mutable
	// prerequisite observations, so freshness reacquisition debt remains while a
	// later candidate contract consumes its bounded attempt 0/1 budget.
	facts.FreshnessReacquireCoverage = append([]common.CoverageWorkItemID(nil), facts.Focus.CoverageWorkItemIDs...)
	if _, err := module.bizPtr.InitializeRuntimeV2(ctx, facts, bootstrap.Definition, nil); err != nil {
		t.Fatal(err)
	}

	requestCount := 0
	var requests []orchestratorevents.RequestRuntimeCandidateCommand
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		requestCount++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		requests = append(requests, command)
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{
			Transaction: command.Transaction, LogicalRequestID: command.LogicalRequestID, RequestSequence: command.RequestSequence, PhysicalCallID: command.PhysicalCallID,
			DocumentContent: []byte(`{"unexpected":true}`),
		}, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })

	driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, module.handleRuntimeV2Drive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2) })

	completed, err := DriveRuntimeV2(ctx, hub, orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"})
	if err != nil {
		t.Fatal(err)
	}
	if requestCount != 2 || completed.Disposition != orchestratorevents.RuntimeV2DriveDispositionTerminal ||
		completed.Action != runtimecontract.RuntimeActionFail || completed.Terminal == nil || completed.Terminal.Code != "candidate_retry_exhausted" {
		t.Fatalf("completed=%+v requestCount=%d, want bounded terminal after two requests", completed, requestCount)
	}
	if len(requests) != 2 || requests[0].PreviousFailure != nil || requests[1].PreviousFailure == nil ||
		requests[1].PreviousFailure.Class != runtimecontract.RuntimeCandidateFailureClassSchema || requests[1].PreviousFailure.Code != "candidate_schema_invalid" ||
		requests[1].PreviousFailure.Signature != "candidate-schema-invalid" || requests[1].PreviousFailure.Reason == "" {
		t.Fatalf("candidate retry did not carry the persisted first failure: %+v", requests)
	}
}

func TestRequestAndSealRuntimeV2CandidateContinuesProviderLimitedOutput(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	facts := runtimeV2Aggregate(t, 1).Facts
	facts.Activation.ExecutionContract.CandidateOutputContinuation = runtimecontract.RuntimeCandidateOutputContinuationPolicy{MaxChunks: 2, TailBytes: 12}
	repository := &inMemoryRuntimeV2StateRepository{}
	controller, err := runtimev2.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := controller.Initialize(ctx, facts, runtimeV2Definition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	continuations := memory.NewArtifactContinuationRepository()
	owner := NewRuntimeOrchestratorWithEvidence(RuntimeEvidenceRepositories{ArtifactContinuations: continuations})
	if err := owner.ConfigureRuntimeV2Repository(repository); err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(owner, hub, nil)
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(initialized.Definition, initialized.Facts.Activation.ExecutionContract)
	first := `{"operations":[{"id":"op-1","kind":"file/edit","coverageWorkItemIds":["cwi-v2"],`
	second := `"targetRefs":["target.txt"],"parameters":[{"name":"oldText","string":"old"},{"name":"newText","string":"new"}]}]}`
	calls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		calls++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		response := orchestratorevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, LogicalRequestID: command.LogicalRequestID, RequestSequence: command.RequestSequence, PhysicalCallID: command.PhysicalCallID}
		if command.OutputContinuation == nil {
			response.OutputFragment = &runtimecontract.RuntimeCandidateOutputFragment{Content: []byte(first), ReachedProviderLimit: true}
		} else {
			if command.OutputContinuation.Sequence != 2 || command.OutputContinuation.PreviousChunkTail != first[len(first)-12:] || command.StablePrefixHash != prefix {
				t.Fatalf("continuation command = %+v", command)
			}
			response.OutputFragment = &runtimecontract.RuntimeCandidateOutputFragment{Content: []byte(second)}
		}
		result.Set(response, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	completed, err := module.requestAndSealRuntimeV2CandidateWithRetry(ctx, initialized, prefix)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || completed.ActiveDispatch == nil || len(completed.CandidateAttempts) != 1 || completed.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateAccepted {
		t.Fatalf("candidate output continuation completion = %+v calls=%d", completed, calls)
	}
	if len(completed.CandidateRequestSessions) != 1 || completed.CandidateRequestSessions[0].State != runtimecontract.RuntimeCandidateRequestSessionDocumentComplete || completed.CandidateRequestSessions[0].NextSequence != 3 || len(completed.CandidateRequestSessions[0].Fragments) != 2 {
		t.Fatalf("persisted C4 request session = %+v", completed.CandidateRequestSessions)
	}
	id := runtimecontract.StableRuntimeCandidateRequestContentID(completed.CandidateRequestSessions[0].LogicalRequestID)
	continuation, err := continuations.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	chunks, err := continuations.ListChunks(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if continuation.State != common.ArtifactContinuationStateApplied || !continuation.FinalReceived || len(chunks) != 2 || continuation.AuthorityTransaction != initialized.Decision.AuthorityTransaction() || continuation.ArtifactID != completed.CandidateRequestSessions[0].LogicalRequestID {
		t.Fatalf("persisted candidate output continuation = %+v", continuation)
	}
}

func TestRequestAndSealRuntimeV2CandidateCommitsSessionBeforeProviderAndAttemptAfterDocument(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	facts := runtimeV2Aggregate(t, 1).Facts
	state := &inMemoryRuntimeV2StateRepository{}
	controller, err := runtimev2.NewRuntimeController(state)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := controller.Initialize(ctx, facts, runtimeV2Definition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	owner := NewRuntimeOrchestratorWithEvidence(RuntimeEvidenceRepositories{ArtifactContinuations: memory.NewArtifactContinuationRepository()})
	if err := owner.ConfigureRuntimeV2Repository(state); err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(owner, hub, nil)
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(initialized.Definition, initialized.Facts.Activation.ExecutionContract)
	document := []byte(`{"operations":[{"id":"op-1","kind":"file/edit","coverageWorkItemIds":["cwi-v2"],"targetRefs":["target.txt"],"parameters":[{"name":"oldText","string":"old"},{"name":"newText","string":"new"}]}]}`)
	providerCalls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		providerCalls++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		persisted, loadErr := state.Load(ctx, command.WorkspaceID, command.RunID)
		if loadErr != nil {
			t.Errorf("load session before provider call: %v", loadErr)
		} else if len(persisted.CandidateRequestSessions) != 1 || !persisted.CandidateRequestSessions[0].ProviderSubmitted || persisted.CandidateRequestSessions[0].LastPhysicalCallID != command.PhysicalCallID || len(persisted.CandidateAttempts) != 0 {
			t.Errorf("provider call did not observe committed request session without candidate attempt: %+v", persisted)
		}
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{
			Transaction: command.Transaction, LogicalRequestID: command.LogicalRequestID, RequestSequence: command.RequestSequence, PhysicalCallID: command.PhysicalCallID,
			DocumentContent: append([]byte(nil), document...),
		}, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	completed, err := module.requestAndSealRuntimeV2CandidateWithRetry(ctx, initialized, prefix)
	if err != nil {
		t.Fatal(err)
	}
	if providerCalls != 1 || completed.ActiveDispatch == nil || len(completed.CandidateAttempts) != 1 || completed.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateAccepted || len(completed.CandidateRequestSessions) != 1 || completed.CandidateRequestSessions[0].State != runtimecontract.RuntimeCandidateRequestSessionDocumentComplete {
		t.Fatalf("session-first candidate completion = %+v calls=%d", completed, providerCalls)
	}
}

func TestRequestAndSealRuntimeV2CandidateRetriesTransportWithoutCandidateAttempt(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	facts := runtimeV2Aggregate(t, 1).Facts
	state := &inMemoryRuntimeV2StateRepository{}
	controller, err := runtimev2.NewRuntimeController(state)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := controller.Initialize(ctx, facts, runtimeV2Definition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	owner := NewRuntimeOrchestratorWithEvidence(RuntimeEvidenceRepositories{ArtifactContinuations: memory.NewArtifactContinuationRepository()})
	if err := owner.ConfigureRuntimeV2Repository(state); err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(owner, hub, nil)
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(initialized.Definition, initialized.Facts.Activation.ExecutionContract)
	document := []byte(`{"operations":[{"id":"op-1","kind":"file/edit","coverageWorkItemIds":["cwi-v2"],"targetRefs":["target.txt"],"parameters":[{"name":"oldText","string":"old"},{"name":"newText","string":"new"}]}]}`)
	var calls []orchestratorevents.RequestRuntimeCandidateCommand
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		calls = append(calls, command)
		if len(calls) == 1 {
			persisted, loadErr := state.Load(ctx, command.WorkspaceID, command.RunID)
			if loadErr != nil || len(persisted.CandidateAttempts) != 0 || len(persisted.CandidateRequestSessions) != 1 || !persisted.CandidateRequestSessions[0].ProviderSubmitted {
				t.Errorf("first transport call did not observe prepared/submitted session only: aggregate=%+v err=%v", persisted, loadErr)
			}
			result.Set(orchestratorevents.RequestRuntimeCandidateResult{
				Transaction: command.Transaction, LogicalRequestID: command.LogicalRequestID, RequestSequence: command.RequestSequence, PhysicalCallID: command.PhysicalCallID,
				Failure: &runtimecontract.RuntimeCandidateFailure{Class: runtimecontract.RuntimeCandidateFailureClassProviderTransient, Code: "deadline_exceeded", Signature: "deadline-exceeded", Reason: "provider deadline exceeded"},
			}, nil)
			return
		}
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{
			Transaction: command.Transaction, LogicalRequestID: command.LogicalRequestID, RequestSequence: command.RequestSequence, PhysicalCallID: command.PhysicalCallID,
			DocumentContent: append([]byte(nil), document...),
		}, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	completed, err := module.requestAndSealRuntimeV2CandidateWithRetry(ctx, initialized, prefix)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || calls[0].LogicalRequestID != calls[1].LogicalRequestID || calls[0].RequestSequence != 1 || calls[1].RequestSequence != 1 || calls[0].PhysicalCallID != calls[1].PhysicalCallID || completed.Facts.Recovery.ProviderTransportAttempts != 1 || len(completed.CandidateAttempts) != 1 || completed.ActiveDispatch == nil {
		t.Fatalf("transport retry did not preserve request identity and candidate budget: calls=%+v aggregate=%+v", calls, completed)
	}
}

func TestRequestAndSealRuntimeV2CandidateRestartsFromPersistedOutputChunk(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	facts := runtimeV2Aggregate(t, 1).Facts
	facts.Activation.ExecutionContract.CandidateOutputContinuation = runtimecontract.RuntimeCandidateOutputContinuationPolicy{MaxChunks: 2, TailBytes: 12}
	state := &inMemoryRuntimeV2StateRepository{}
	controller, err := runtimev2.NewRuntimeController(state)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := controller.Initialize(ctx, facts, runtimeV2Definition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	continuations := memory.NewArtifactContinuationRepository()
	owner := NewRuntimeOrchestratorWithEvidence(RuntimeEvidenceRepositories{ArtifactContinuations: continuations})
	if err := owner.ConfigureRuntimeV2Repository(state); err != nil {
		t.Fatal(err)
	}
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(initialized.Definition, initialized.Facts.Activation.ExecutionContract)
	candidateContext := runtimecontract.RuntimeCandidateContextForDecision(initialized.Facts, initialized.Decision, nil)
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(initialized.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(candidateContext.Resources))
	if err != nil {
		t.Fatal(err)
	}
	prepared, session, err := owner.EnsureRuntimeV2CandidateRequestSession(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, prefix, manifest.Hash, "")
	if err != nil {
		t.Fatal(err)
	}
	first := `{"operations":[{"id":"op-1","kind":"file/edit","coverageWorkItemIds":["cwi-v2"],`
	firstPhysicalID := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, 1)
	submitted, err := owner.MarkRuntimeV2CandidateRequestSubmitted(ctx, prepared.Facts.WorkspaceID, prepared.Facts.RunID, session.LogicalRequestID, 1, firstPhysicalID)
	if err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(owner, hub, nil)
	pending, _, err := module.persistAndRecordRuntimeCandidateRequestFragment(ctx, submitted, session.LogicalRequestID, 1, firstPhysicalID, []byte(first), true)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		calls++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		if command.OutputContinuation == nil || command.RequestSequence != 2 || command.OutputContinuation.Sequence != 2 || command.OutputContinuation.PreviousChunkTail != first[len(first)-12:] {
			t.Fatalf("restart continuation command = %+v", command)
		}
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, LogicalRequestID: command.LogicalRequestID, RequestSequence: command.RequestSequence, PhysicalCallID: command.PhysicalCallID, OutputFragment: &runtimecontract.RuntimeCandidateOutputFragment{Content: []byte(`"targetRefs":["target.txt"],"parameters":[{"name":"oldText","string":"old"},{"name":"newText","string":"new"}]}]}`)}}, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	restarted := newTestBiz(owner, hub, nil)
	completed, err := restarted.requestAndSealRuntimeV2CandidateWithRetry(ctx, pending, prefix)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || completed.ActiveDispatch == nil || len(completed.CandidateAttempts) != 1 || completed.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateAccepted || len(completed.CandidateRequestSessions) != 1 || completed.CandidateRequestSessions[0].NextSequence != 3 {
		t.Fatalf("restart continuation completion = %+v calls=%d", completed, calls)
	}
}

// P3-B3: a complete C4 document is durable before C5 allocates attempt 0.
// Recreating the owner, state adapter, and content repository must therefore
// decode/admit that document exactly once without another provider request.
func TestRuntimeV2FileBackedCompleteDocumentRestartAdmitsWithoutProviderReplay(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	root := t.TempDir()
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	bootstrap := runtimeV2Bootstrap()
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
		DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, State: common.RunStateActive,
	}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })

	contentsRoot := filepath.Join(root, "candidate-contents")
	firstOwner := NewRuntimeOrchestratorWithEvidence(RuntimeEvidenceRepositories{
		ArtifactContinuations: filerepository.NewArtifactContinuationRepository(contentsRoot),
	})
	if err := firstOwner.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	initialized, err := firstOwner.InitializeRuntimeV2(ctx, runtimeV2Aggregate(t, 1).Facts, runtimeV2Definition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(initialized.Definition, initialized.Facts.Activation.ExecutionContract)
	candidateContext := runtimecontract.RuntimeCandidateContextForDecision(initialized.Facts, initialized.Decision, nil)
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(initialized.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(candidateContext.Resources))
	if err != nil {
		t.Fatal(err)
	}
	_, session, err := firstOwner.EnsureRuntimeV2CandidateRequestSession(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, prefix, manifest.Hash, "")
	if err != nil {
		t.Fatal(err)
	}
	physicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, session.NextSequence)
	submitted, err := firstOwner.MarkRuntimeV2CandidateRequestSubmitted(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, session.LogicalRequestID, session.NextSequence, physicalCallID)
	if err != nil {
		t.Fatal(err)
	}
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "reloaded-edit", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeV2FileEditParameters("target.txt"),
	}}}}}
	document := runtimeV2CandidateDocumentForTest(t, manifest, candidate)
	committed, _, _, err := newTestBiz(firstOwner, hub, nil).persistAndCompleteRuntimeCandidateRequestDocument(ctx, submitted, session.LogicalRequestID, session.NextSequence, physicalCallID, document)
	if err != nil {
		t.Fatal(err)
	}
	if len(committed.CandidateAttempts) != 0 || committed.ActiveDispatch != nil || committed.CandidateRequestSessions[0].State != runtimecontract.RuntimeCandidateRequestSessionDocumentComplete {
		t.Fatalf("C4 complete boundary did not remain attempt-free: %+v", committed)
	}

	// Simulate process loss: all three persistence/authority objects are
	// reconstructed from their file roots. The LLM responder is deliberately a
	// call counter, because any provider call after a durable complete document
	// is a P3-B3 violation.
	if err := runOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	hub.Terminate(ctx)
	hub = magicevent.NewHub(16)
	runOwner = runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: filerepository.NewRunRepository(filepath.Join(root, "runs.json")), EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	restartedContents := filerepository.NewArtifactContinuationRepository(contentsRoot)
	restartedOwner := NewRuntimeOrchestratorWithEvidence(RuntimeEvidenceRepositories{
		ArtifactContinuations: restartedContents,
	})
	if err := restartedOwner.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	reloaded, err := restartedOwner.LoadRuntimeV2(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	providerCalls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(_ magicevent.Event, _ magicevent.Result) {
		providerCalls++
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	sealed, err := newTestBiz(restartedOwner, hub, nil).requestAndSealRuntimeV2CandidateWithRetry(ctx, reloaded, prefix)
	if err != nil {
		t.Fatal(err)
	}
	if providerCalls != 0 || sealed.ActiveDispatch == nil || len(sealed.CandidateAttempts) != 1 ||
		sealed.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateAccepted ||
		sealed.CandidateAttempts[0].LogicalRequestID != session.LogicalRequestID {
		t.Fatalf("restart reused a complete document incorrectly: providerCalls=%d aggregate=%+v", providerCalls, sealed)
	}
	stored, err := loadCompletedRuntimeCandidateRequestDocument(ctx, restartedContents, sealed, session.LogicalRequestID)
	if err != nil || string(stored) != string(document) {
		t.Fatalf("restarted content repository lost C4 document: content=%q err=%v", stored, err)
	}
}

// P3-B3 prepare boundary: a durable prepared session has no unresolved
// physical call. A restarted owner must reuse that exact logical session and
// make its first provider submission once, rather than allocating an attempt
// before I/O or creating a second session.
func TestRuntimeV2FileBackedPrepareRestartSubmitsOnce(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	root := t.TempDir()
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	bootstrap := runtimeV2Bootstrap()
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
		DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, State: common.RunStateActive,
	}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })

	contentsRoot := filepath.Join(root, "candidate-contents")
	firstOwner := NewRuntimeOrchestratorWithEvidence(RuntimeEvidenceRepositories{
		ArtifactContinuations: filerepository.NewArtifactContinuationRepository(contentsRoot),
	})
	if err := firstOwner.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	initialized, err := firstOwner.InitializeRuntimeV2(ctx, runtimeV2Aggregate(t, 1).Facts, runtimeV2Definition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(initialized.Definition, initialized.Facts.Activation.ExecutionContract)
	candidateContext := runtimecontract.RuntimeCandidateContextForDecision(initialized.Facts, initialized.Decision, nil)
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(initialized.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(candidateContext.Resources))
	if err != nil {
		t.Fatal(err)
	}
	prepared, session, err := firstOwner.EnsureRuntimeV2CandidateRequestSession(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, prefix, manifest.Hash, "")
	if err != nil {
		t.Fatal(err)
	}
	if session.State != runtimecontract.RuntimeCandidateRequestSessionPrepared || session.ProviderSubmitted || len(prepared.CandidateAttempts) != 0 {
		t.Fatalf("prepared C4 boundary = %+v aggregate=%+v", session, prepared)
	}

	// Rebuild both the state controller and opaque-content repository before
	// any physical provider request. The only persisted input is session ID.
	if err := runOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	hub.Terminate(ctx)
	hub = magicevent.NewHub(16)
	runOwner = runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: filerepository.NewRunRepository(filepath.Join(root, "runs.json")), EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	restartedOwner := NewRuntimeOrchestratorWithEvidence(RuntimeEvidenceRepositories{
		ArtifactContinuations: filerepository.NewArtifactContinuationRepository(contentsRoot),
	})
	if err := restartedOwner.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	reloaded, err := restartedOwner.LoadRuntimeV2(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	providerCalls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		providerCalls++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		if command.LogicalRequestID != session.LogicalRequestID || command.RequestSequence != 1 || command.PhysicalCallID != runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, 1) {
			t.Errorf("restart changed prepared request identity: %+v", command)
		}
		candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
			ID: "prepared-restart-edit", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeV2FileEditParameters("target.txt"),
		}}}}}
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{
			Transaction: command.Transaction, LogicalRequestID: command.LogicalRequestID, RequestSequence: command.RequestSequence, PhysicalCallID: command.PhysicalCallID,
			DocumentContent: runtimeV2CandidateDocumentForTest(t, command.CandidateManifest, candidate),
		}, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	sealed, err := newTestBiz(restartedOwner, hub, nil).requestAndSealRuntimeV2CandidateWithRetry(ctx, reloaded, prefix)
	if err != nil {
		t.Fatal(err)
	}
	if providerCalls != 1 || sealed.ActiveDispatch == nil || len(sealed.CandidateAttempts) != 1 ||
		sealed.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateAccepted ||
		len(sealed.CandidateRequestSessions) != 1 || sealed.CandidateRequestSessions[0].LogicalRequestID != session.LogicalRequestID ||
		sealed.CandidateRequestSessions[0].NextSequence != 1 {
		t.Fatalf("prepared restart did not make one first submission: providerCalls=%d aggregate=%+v", providerCalls, sealed)
	}
}

// P3-B3: once the submitted marker is durable but no provider outcome is,
// restart has no safe replay authority. It must terminalize the session as
// provider_submission_unknown without allocating an attempt or making a
// second physical request.
func TestRuntimeV2FileBackedSubmittedCandidateRestartDoesNotReplayProvider(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	root := t.TempDir()
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	bootstrap := runtimeV2Bootstrap()
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
		DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, State: common.RunStateActive,
	}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })

	firstOwner := NewRuntimeOrchestratorWithEvidence(RuntimeEvidenceRepositories{
		ArtifactContinuations: filerepository.NewArtifactContinuationRepository(filepath.Join(root, "candidate-contents")),
	})
	if err := firstOwner.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	initialized, err := firstOwner.InitializeRuntimeV2(ctx, runtimeV2Aggregate(t, 1).Facts, runtimeV2Definition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(initialized.Definition, initialized.Facts.Activation.ExecutionContract)
	context := runtimecontract.RuntimeCandidateContextForDecision(initialized.Facts, initialized.Decision, nil)
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(initialized.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(context.Resources))
	if err != nil {
		t.Fatal(err)
	}
	_, session, err := firstOwner.EnsureRuntimeV2CandidateRequestSession(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, prefix, manifest.Hash, "")
	if err != nil {
		t.Fatal(err)
	}
	physicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, session.NextSequence)
	if _, err := firstOwner.MarkRuntimeV2CandidateRequestSubmitted(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, session.LogicalRequestID, session.NextSequence, physicalCallID); err != nil {
		t.Fatal(err)
	}

	if err := runOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	hub.Terminate(ctx)
	hub = magicevent.NewHub(16)
	runOwner = runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: filerepository.NewRunRepository(filepath.Join(root, "runs.json")), EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	restartedOwner := NewRuntimeOrchestratorWithEvidence(RuntimeEvidenceRepositories{
		ArtifactContinuations: filerepository.NewArtifactContinuationRepository(filepath.Join(root, "candidate-contents")),
	})
	if err := restartedOwner.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	reloaded, err := restartedOwner.LoadRuntimeV2(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	providerCalls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(_ magicevent.Event, _ magicevent.Result) { providerCalls++ })
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	terminal, err := newTestBiz(restartedOwner, hub, nil).requestAndSealRuntimeV2CandidateWithRetry(ctx, reloaded, prefix)
	if err != nil {
		t.Fatal(err)
	}
	if providerCalls != 0 || terminal.Decision.Terminal == nil || terminal.Decision.Terminal.Code != "provider_submission_unknown" ||
		len(terminal.CandidateAttempts) != 0 || terminal.ActiveDispatch != nil || len(terminal.GovernanceRequests) != 0 ||
		len(terminal.CandidateRequestSessions) != 1 || terminal.CandidateRequestSessions[0].State != runtimecontract.RuntimeCandidateRequestSessionTerminal ||
		terminal.CandidateRequestSessions[0].LastOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomeProviderSubmissionUnknown {
		t.Fatalf("submitted restart replayed or changed candidate authority: providerCalls=%d aggregate=%+v", providerCalls, terminal)
	}
}

// P3-B3 stale boundary: even when C4 has durably marked a document complete,
// a typed prerequisite invalidation wins over C5 admission. Restart must keep
// the old document as terminal audit lineage and return to SM-2 acquisition.
func TestRuntimeV2FileBackedStaleCompleteSessionInvalidatesBeforeCandidateReuse(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	root := t.TempDir()
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	bootstrap := runtimeV2Bootstrap()
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
		DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, State: common.RunStateActive,
	}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })

	facts := runtimeV2Aggregate(t, 1).Facts
	ceiling := runtimecontract.ProviderCeiling{
		Target: facts.Activation.ProviderTarget, ContextWindowTokens: 128000, ProviderMaxOutputTokens: 16384,
		EffectiveMaxOutputTokens: 4096, MaxInputTokens: 92928, MaxBytes: 96 * 1024,
		CapabilityRevision: "capability-v1", ConfigurationRevision: "config-v1",
	}
	prerequisite := runtimecontract.RuntimePrerequisite{
		ID: "provider_ceiling:llm", Kind: runtimecontract.PrerequisiteKindProviderCeiling,
		OwnerModuleID: runtimecontract.RuntimeModuleLLM, MinimumBytes: 32 * 1024,
	}
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: prerequisite.OwnerModuleID,
		ContentHash: "provider-ceiling-v1", Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceProviderConfig, OwnerModuleID: runtimecontract.RuntimeModuleLLM,
			SubjectRef: string(ceiling.Target.ProviderID) + "/" + string(ceiling.Target.ModelID), Revision: ceiling.ConfigurationRevision,
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	facts.Resources.ProviderCeiling = &ceiling
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{prerequisite}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{observation}

	contentsRoot := filepath.Join(root, "candidate-contents")
	firstOwner := NewRuntimeOrchestratorWithEvidence(RuntimeEvidenceRepositories{
		ArtifactContinuations: filerepository.NewArtifactContinuationRepository(contentsRoot),
	})
	if err := firstOwner.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	initialized, err := firstOwner.InitializeRuntimeV2(ctx, facts, runtimeV2Definition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(initialized.Definition, initialized.Facts.Activation.ExecutionContract)
	candidateContext := runtimecontract.RuntimeCandidateContextForDecision(initialized.Facts, initialized.Decision, nil)
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(initialized.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(candidateContext.Resources))
	if err != nil {
		t.Fatal(err)
	}
	_, session, err := firstOwner.EnsureRuntimeV2CandidateRequestSession(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, prefix, manifest.Hash, "")
	if err != nil {
		t.Fatal(err)
	}
	physicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, session.NextSequence)
	submitted, err := firstOwner.MarkRuntimeV2CandidateRequestSubmitted(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, session.LogicalRequestID, session.NextSequence, physicalCallID)
	if err != nil {
		t.Fatal(err)
	}
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "stale-complete-edit", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeV2FileEditParameters("target.txt"),
	}}}}}
	completed, _, _, err := newTestBiz(firstOwner, hub, nil).persistAndCompleteRuntimeCandidateRequestDocument(ctx, submitted, session.LogicalRequestID, session.NextSequence, physicalCallID, runtimeV2CandidateDocumentForTest(t, manifest, candidate))
	if err != nil {
		t.Fatal(err)
	}
	if len(completed.CandidateAttempts) != 0 || completed.CandidateRequestSessions[0].State != runtimecontract.RuntimeCandidateRequestSessionDocumentComplete {
		t.Fatalf("stale fixture did not stop at C4 completion: %+v", completed)
	}

	stale := runtimecontract.RuntimePrerequisiteStale{
		PrerequisiteID: prerequisite.ID, ObservationID: observation.ObservationID,
		Domain: runtimecontract.RuntimeResourceProviderConfig, OwnerModuleID: runtimecontract.RuntimeModuleLLM,
		SubjectRef: string(ceiling.Target.ProviderID) + "/" + string(ceiling.Target.ModelID), ExpectedRevision: "config-v1", ObservedRevision: "config-v2",
		Reason: "provider configuration changed",
	}
	invalidated, err := firstOwner.InvalidateRuntimeV2CandidateRequestSessionForPrerequisiteStale(ctx, completed.Facts.WorkspaceID, completed.Facts.RunID, session.LogicalRequestID, stale)
	if err != nil {
		t.Fatal(err)
	}
	if invalidated.Decision.Action != runtimecontract.RuntimeActionAcquirePrerequisite || len(invalidated.CandidateAttempts) != 0 ||
		invalidated.Facts.Resources.ProviderCeiling != nil || len(invalidated.Facts.Prerequisites.Pending) != 1 ||
		len(invalidated.CandidateRequestSessions) != 1 || invalidated.CandidateRequestSessions[0].State != runtimecontract.RuntimeCandidateRequestSessionTerminal ||
		invalidated.CandidateRequestSessions[0].LastOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomePrerequisiteStale {
		t.Fatalf("complete session stale was not invalidated before admission: %+v", invalidated)
	}

	if err := runOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	hub.Terminate(ctx)
	hub = magicevent.NewHub(16)
	runOwner = runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: filerepository.NewRunRepository(filepath.Join(root, "runs.json")), EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	restartedOwner := NewRuntimeOrchestratorWithEvidence(RuntimeEvidenceRepositories{
		ArtifactContinuations: filerepository.NewArtifactContinuationRepository(contentsRoot),
	})
	if err := restartedOwner.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	reloaded, err := restartedOwner.LoadRuntimeV2(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	providerCalls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(_ magicevent.Event, _ magicevent.Result) { providerCalls++ })
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	resumed, err := newTestBiz(restartedOwner, hub, nil).requestAndSealRuntimeV2CandidateWithRetry(ctx, reloaded, prefix)
	if err != nil {
		t.Fatal(err)
	}
	if providerCalls != 0 || resumed.Decision.Action != runtimecontract.RuntimeActionAcquirePrerequisite || len(resumed.CandidateAttempts) != 0 ||
		resumed.ActiveDispatch != nil || len(resumed.GovernanceRequests) != 0 || resumed.Facts.Repair.RepairAttempts != 0 ||
		resumed.Facts.Recovery.ProviderTransportAttempts != 0 {
		t.Fatalf("stale complete session reused candidate content: providerCalls=%d aggregate=%+v", providerCalls, resumed)
	}
}

func TestRequestAndSealRuntimeV2CandidateFailsWhenOutputContinuationBudgetExhausted(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	facts := runtimeV2Aggregate(t, 1).Facts
	facts.Activation.ExecutionContract.CandidateOutputContinuation = runtimecontract.RuntimeCandidateOutputContinuationPolicy{MaxChunks: 1, TailBytes: 8}
	state := &inMemoryRuntimeV2StateRepository{}
	controller, err := runtimev2.NewRuntimeController(state)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := controller.Initialize(ctx, facts, runtimeV2Definition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	continuations := memory.NewArtifactContinuationRepository()
	owner := NewRuntimeOrchestratorWithEvidence(RuntimeEvidenceRepositories{ArtifactContinuations: continuations})
	if err := owner.ConfigureRuntimeV2Repository(state); err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(owner, hub, nil)
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(initialized.Definition, initialized.Facts.Activation.ExecutionContract)
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	calls := 0
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		calls++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		fragment := runtimecontract.RuntimeCandidateOutputFragment{Content: []byte(`{"operations":[`), ReachedProviderLimit: true}
		if command.OutputContinuation != nil {
			fragment.Content = []byte(`{"id":"op-1"`)
		}
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, LogicalRequestID: command.LogicalRequestID, RequestSequence: command.RequestSequence, PhysicalCallID: command.PhysicalCallID, OutputFragment: &fragment}, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	failed, err := module.requestAndSealRuntimeV2CandidateWithRetry(ctx, initialized, prefix)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Decision.Action != runtimecontract.RuntimeActionFail || failed.Decision.Terminal == nil || failed.Decision.Terminal.Code != "candidate_output_continuation_exhausted" {
		t.Fatalf("candidate output continuation exhaustion = %+v", failed.Decision)
	}
	if len(failed.CandidateAttempts) != 0 || failed.ActiveDispatch != nil || failed.Facts.Repair.RepairAttempts != 0 || len(failed.GovernanceRequests) != 0 {
		t.Fatalf("continuation exhaustion must not allocate a candidate/dispatch/repair/governance: %+v", failed)
	}
	if len(failed.CandidateRequestSessions) != 1 {
		t.Fatalf("candidate request sessions = %+v", failed.CandidateRequestSessions)
	}
	session := failed.CandidateRequestSessions[0]
	if session.State != runtimecontract.RuntimeCandidateRequestSessionTerminal ||
		session.LastOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomeContinuationExhausted ||
		session.LastOutcome.Code != "candidate_output_continuation_exhausted" ||
		len(session.Fragments) != 2 {
		t.Fatalf("terminal continuation session = %+v", session)
	}
	id := runtimeV2CandidateRequestContentID(t, initialized, prefix, 0)
	continuation, err := continuations.Get(ctx, id)
	if err != nil || continuation.State != common.ArtifactContinuationStateFailed {
		t.Fatalf("failed continuation = %+v err=%v", continuation, err)
	}
	reloaded, err := state.Load(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	restartedOwner := NewRuntimeOrchestratorWithEvidence(RuntimeEvidenceRepositories{ArtifactContinuations: continuations})
	if err := restartedOwner.ConfigureRuntimeV2Repository(state); err != nil {
		t.Fatal(err)
	}
	restarted := newTestBiz(restartedOwner, hub, nil)
	resumed, err := restarted.requestAndSealRuntimeV2CandidateWithRetry(ctx, reloaded, prefix)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.StateRevision != failed.StateRevision || resumed.Decision.Terminal == nil || calls != 2 {
		t.Fatalf("restart must preserve terminal continuation exhaustion without a provider replay: aggregate=%+v calls=%d", resumed, calls)
	}
}

func TestRequestAndSealRuntimeV2CandidateTerminatesUnknownSubmittedRequestWithoutCandidateAttempt(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	state := &inMemoryRuntimeV2StateRepository{}
	controller, err := runtimev2.NewRuntimeController(state)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := controller.Initialize(ctx, runtimeV2Aggregate(t, 1).Facts, runtimeV2Definition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	owner := orchestratorbiz.NewRuntimeOrchestrator()
	if err := owner.ConfigureRuntimeV2Repository(state); err != nil {
		t.Fatal(err)
	}
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(initialized.Definition, initialized.Facts.Activation.ExecutionContract)
	context := runtimecontract.RuntimeCandidateContextForDecision(initialized.Facts, initialized.Decision, nil)
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(initialized.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(context.Resources))
	if err != nil {
		t.Fatal(err)
	}
	prepared, session, err := owner.EnsureRuntimeV2CandidateRequestSession(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, prefix, manifest.Hash, "")
	if err != nil {
		t.Fatal(err)
	}
	physicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, session.NextSequence)
	if _, err := owner.MarkRuntimeV2CandidateRequestSubmitted(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, session.LogicalRequestID, session.NextSequence, physicalCallID); err != nil {
		t.Fatal(err)
	}
	terminal, err := newTestBiz(owner, hub, nil).requestAndSealRuntimeV2CandidateWithRetry(ctx, prepared, prefix)
	if err != nil {
		t.Fatal(err)
	}
	if terminal.Decision.Action != runtimecontract.RuntimeActionFail || terminal.Decision.Terminal == nil ||
		terminal.Decision.Terminal.Code != "provider_submission_unknown" || len(terminal.CandidateAttempts) != 0 ||
		terminal.ActiveDispatch != nil || terminal.Facts.Repair.RepairAttempts != 0 ||
		terminal.Facts.Recovery.ProviderTransportAttempts != 0 || len(terminal.GovernanceRequests) != 0 {
		t.Fatalf("unknown submitted provider request consumed candidate authority: %+v", terminal)
	}
	if len(terminal.CandidateRequestSessions) != 1 {
		t.Fatalf("candidate request sessions = %+v", terminal.CandidateRequestSessions)
	}
	stored := terminal.CandidateRequestSessions[0]
	if stored.State != runtimecontract.RuntimeCandidateRequestSessionTerminal ||
		stored.LastOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomeProviderSubmissionUnknown ||
		!stored.ProviderSubmitted {
		t.Fatalf("unknown submission session = %+v", stored)
	}
}

func TestRequestAndSealRuntimeV2CandidateFailsCompleteDocumentContentDefectWithoutProviderOrAttempt(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	state := &inMemoryRuntimeV2StateRepository{}
	controller, err := runtimev2.NewRuntimeController(state)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := controller.Initialize(ctx, runtimeV2Aggregate(t, 1).Facts, runtimeV2Definition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	contents := memory.NewArtifactContinuationRepository()
	owner := NewRuntimeOrchestratorWithEvidence(RuntimeEvidenceRepositories{ArtifactContinuations: contents})
	if err := owner.ConfigureRuntimeV2Repository(state); err != nil {
		t.Fatal(err)
	}
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(initialized.Definition, initialized.Facts.Activation.ExecutionContract)
	candidateContext := runtimecontract.RuntimeCandidateContextForDecision(initialized.Facts, initialized.Decision, nil)
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(initialized.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(candidateContext.Resources))
	if err != nil {
		t.Fatal(err)
	}
	prepared, session, err := owner.EnsureRuntimeV2CandidateRequestSession(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, prefix, manifest.Hash, "")
	if err != nil {
		t.Fatal(err)
	}
	physicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, session.NextSequence)
	if _, err := owner.MarkRuntimeV2CandidateRequestSubmitted(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, session.LogicalRequestID, session.NextSequence, physicalCallID); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.CompleteRuntimeV2CandidateRequestDocument(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, session.LogicalRequestID, physicalCallID); err != nil {
		t.Fatal(err)
	}
	providerCalls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(_ magicevent.Event, _ magicevent.Result) {
		providerCalls++
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	terminal, err := newTestBiz(owner, hub, nil).requestAndSealRuntimeV2CandidateWithRetry(ctx, prepared, prefix)
	if err != nil {
		t.Fatal(err)
	}
	if providerCalls != 0 || terminal.Decision.Action != runtimecontract.RuntimeActionFail || terminal.Decision.Terminal == nil ||
		terminal.Decision.Terminal.Code != "runtime_contract_defect" || len(terminal.CandidateAttempts) != 0 ||
		terminal.ActiveDispatch != nil || terminal.Facts.Repair.RepairAttempts != 0 {
		t.Fatalf("complete document content defect must not replay provider or allocate an attempt: %+v", terminal)
	}
}

func TestRequestAndSealRuntimeV2CandidateReacquiresProviderCeilingAfterTypedStale(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	facts := runtimeV2Aggregate(t, 1).Facts
	ceiling := runtimecontract.ProviderCeiling{
		Target:                   runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ContextWindowTokens:      128000,
		ProviderMaxOutputTokens:  16384,
		EffectiveMaxOutputTokens: 4096,
		MaxInputTokens:           92928,
		MaxBytes:                 96 * 1024,
		CapabilityRevision:       "capability-v1",
		ConfigurationRevision:    "config-v1",
	}
	prerequisite := runtimecontract.RuntimePrerequisite{
		ID: "provider_ceiling:llm", Kind: runtimecontract.PrerequisiteKindProviderCeiling,
		OwnerModuleID: runtimecontract.RuntimeModuleLLM, MinimumBytes: 32 * 1024,
	}
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: prerequisite.OwnerModuleID,
		ContentHash: "provider-ceiling-v1",
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceProviderConfig, OwnerModuleID: runtimecontract.RuntimeModuleLLM,
			SubjectRef: "ai-proxy/gpt-4o", Revision: ceiling.ConfigurationRevision,
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	facts.Resources.ProviderCeiling = &ceiling
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{observation}
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{prerequisite}
	repository := &inMemoryRuntimeV2StateRepository{}
	controller, err := runtimev2.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := controller.Initialize(ctx, facts, runtimeV2Definition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	owner := orchestratorbiz.NewRuntimeOrchestrator()
	if err := owner.ConfigureRuntimeV2Repository(repository); err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(owner, hub, nil)
	stale := runtimecontract.RuntimePrerequisiteStale{
		PrerequisiteID: prerequisite.ID, ObservationID: observation.ObservationID,
		Domain: runtimecontract.RuntimeResourceProviderConfig, OwnerModuleID: runtimecontract.RuntimeModuleLLM,
		SubjectRef: "ai-proxy/gpt-4o", ExpectedRevision: "config-v1", ObservedRevision: "config-v2",
		Reason: "provider configuration changed",
	}
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	calls := 0
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		calls++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, LogicalRequestID: command.LogicalRequestID, RequestSequence: command.RequestSequence, PhysicalCallID: command.PhysicalCallID, Stale: &stale}, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	invalidated, err := module.requestAndSealRuntimeV2Candidate(ctx, initialized, "stable-prefix-provider-config", 0)
	if err != nil {
		t.Fatal(err)
	}
	if invalidated.Decision.Action != runtimecontract.RuntimeActionAcquirePrerequisite ||
		len(invalidated.CandidateAttempts) != 0 || invalidated.Facts.Recovery.PrerequisiteRefreshes != 1 ||
		len(invalidated.Facts.Prerequisites.Pending) != 1 || invalidated.Facts.Resources.ProviderCeiling != nil ||
		len(invalidated.Facts.Evidence.Accepted) != 0 || len(invalidated.Facts.Evidence.Rejected) != 0 ||
		invalidated.Facts.Repair.RepairAttempts != 0 || invalidated.Facts.Recovery.ProviderTransportAttempts != 0 ||
		invalidated.ActiveDispatch != nil || len(invalidated.GovernanceRequests) != 0 || calls != 1 {
		t.Fatalf("provider configuration stale did not return to clean prerequisite acquisition: %+v", invalidated)
	}
	if len(invalidated.CandidateRequestSessions) != 1 {
		t.Fatalf("stale candidate request sessions = %+v", invalidated.CandidateRequestSessions)
	}
	terminated := invalidated.CandidateRequestSessions[0]
	if terminated.State != runtimecontract.RuntimeCandidateRequestSessionTerminal ||
		terminated.LastOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomePrerequisiteStale ||
		terminated.ProviderSubmitted {
		t.Fatalf("stale request session was not terminalized without candidate debt: %+v", terminated)
	}
	restarted, err := repository.Load(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if restarted.Decision.Action != runtimecontract.RuntimeActionAcquirePrerequisite || len(restarted.CandidateAttempts) != 0 ||
		len(restarted.CandidateRequestSessions) != 1 || restarted.CandidateRequestSessions[0].LastOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomePrerequisiteStale {
		t.Fatalf("reload did not preserve stale session and prerequisite reacquisition: %+v", restarted)
	}
	refreshedCeiling := ceiling
	refreshedCeiling.ConfigurationRevision = "config-v2"
	refreshedObservation := observation
	refreshedObservation.ContentHash = "provider-ceiling-v2"
	refreshedObservation.Dependencies[0].Revision = refreshedCeiling.ConfigurationRevision
	refreshedObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(refreshedObservation)
	reacquired, err := owner.CommitRuntimeV2Prerequisite(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, runtimev2.PrerequisiteResult{
		PrerequisiteID: prerequisite.ID, ProviderCeiling: &refreshedCeiling, Observation: &refreshedObservation,
	})
	if err != nil {
		t.Fatal(err)
	}
	if reacquired.Decision.Action != runtimecontract.RuntimeActionProduceStateChange || len(reacquired.CandidateAttempts) != 0 {
		t.Fatalf("reacquired provider ceiling did not seal a clean authority: %+v", reacquired)
	}
	withNewSession, nextSession, err := owner.EnsureRuntimeV2CandidateRequestSession(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, "stable-prefix-provider-config", "manifest-provider-config-v2", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(withNewSession.CandidateAttempts) != 0 || nextSession.LogicalRequestID == terminated.LogicalRequestID ||
		nextSession.Transaction.AuthoritySnapshotID == terminated.Transaction.AuthoritySnapshotID ||
		nextSession.State != runtimecontract.RuntimeCandidateRequestSessionPrepared {
		t.Fatalf("reacquired authority did not create a distinct clean request session: previous=%+v next=%+v aggregate=%+v", terminated, nextSession, withNewSession)
	}
}

func startRuntimeV2FileManifestOwner(t *testing.T, ctx context.Context, hub magicevent.Hub) {
	t.Helper()
	fileOwner := filemodule.New()
	if err := fileOwner.Configure(filemodule.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := fileOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fileOwner.Stop(ctx) })
}

func newRuntimeV2DriveTestModule(t *testing.T, hub magicevent.Hub) *Biz {
	t.Helper()
	biz := orchestratorbiz.NewRuntimeOrchestrator()
	if err := biz.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	return newTestBiz(biz, hub, nil)
}

func TestRuntimeV2EnsureCandidateFeasibleBlocksProviderWithoutKinds(t *testing.T) {
	aggregate := runtimev2.RuntimeAggregate{
		Decision: runtimecontract.RuntimeDecision{
			Action: runtimecontract.RuntimeActionProduceValidation,
			RequiredOutput: runtimecontract.RequiredOutputContract{
				Kind:                runtimecontract.RequiredOutputKindValidation,
				AcceptedKinds:       []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
				CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
			},
		},
		Facts: runtimecontract.RuntimeFacts{
			// file/edit is pure durable — kind-level validation feasibility must exclude it.
			Capabilities: runtimecontract.RuntimeCapabilities{ValidationKinds: []string{runtimecontract.StepKindFileEdit}},
		},
	}
	if err := runtimeV2EnsureCandidateFeasible(aggregate); err == nil || !strings.Contains(err.Error(), "no feasible operation kinds") {
		t.Fatalf("expected infeasible validation, got %v", err)
	}
	aggregate.Facts.Capabilities.ValidationKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
	if err := runtimeV2EnsureCandidateFeasible(aggregate); err != nil {
		t.Fatalf("managed validation should be feasible: %v", err)
	}
}

func TestRuntimeV2DecisionPersistsFeasibilityWithoutCallingProvider(t *testing.T) {
	facts := runtimecontract.RuntimeFacts{
		Activation:  runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		WorkspaceID: "ws-v2", RunID: "run-v2", DefinitionRevision: "definition-v2", EvidenceRevision: runtimecontract.InitialRuntimeEvidenceRevision,
		Focus:    runtimecontract.CoverageFocus{ID: "focus-v2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}},
		Coverage: []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-v2", State: runtimecontract.RuntimeCoverageItemStateOpen}},
		// Pure durable kind in ValidationKinds proves the pre-provider feasibility gate.
		// Dual-use shell/command is intentionally feasible at kind level under RVC-R6.
		Capabilities: runtimecontract.RuntimeCapabilities{ValidationKinds: []string{runtimecontract.StepKindFileEdit}},
	}
	decision, err := runtimev2.ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	repository := &runtimeV2DriveRepository{aggregate: runtimev2.RuntimeAggregate{StateRevision: 1, Definition: runtimeV2Definition(), Facts: facts.Normalized(), Decision: decision}}
	biz := orchestratorbiz.NewRuntimeOrchestrator()
	if err := biz.ConfigureRuntimeV2Repository(repository); err != nil {
		t.Fatal(err)
	}
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	providerCalls := 0
	observer := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	observer.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(_ magicevent.Event, _ magicevent.Result) { providerCalls++ })
	t.Cleanup(func() { observer.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	module := newTestBiz(biz, hub, nil)
	next, err := module.driveRuntimeV2Decision(context.Background(), repository.aggregate, "unused-prefix")
	if err != nil {
		t.Fatal(err)
	}
	if providerCalls != 0 || next.Facts.Feasibility == nil || next.Decision.Action != runtimecontract.RuntimeActionFail || next.Decision.Terminal == nil || next.Decision.Terminal.Code != "capability_unavailable" {
		t.Fatalf("feasibility transition = %+v providerCalls=%d", next, providerCalls)
	}
}

func TestRuntimeV2DecisionReconcilesHistoricalAuthorityBeforeProvider(t *testing.T) {
	facts := runtimecontract.RuntimeFacts{
		Activation:         runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		WorkspaceID:        "ws-v2",
		RunID:              "run-v2",
		DefinitionRevision: "definition-v2",
		EvidenceRevision:   runtimecontract.InitialRuntimeEvidenceRevision,
		Focus: runtimecontract.CoverageFocus{
			ID: "focus-manager", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, TargetRole: "implementation",
			TargetDiscoverySources: []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest},
			TargetSelectorRef:      "file/profile_declared_candidates", TargetSelectorOwnerRef: "profile-bootstrap",
			TargetSelectorSubjectRef: "manager", TargetSelectorCandidateRefs: []string{"internal/lsp/manager.go"},
			RequireTargetGrounding: true,
		},
		Coverage:     []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-v2", State: runtimecontract.RuntimeCoverageItemStateOpen}},
		Capabilities: runtimecontract.RuntimeCapabilities{StateChangeKinds: []string{runtimecontract.StepKindFileWrite}},
	}
	preparedFacts, preparedDecision, err := runtimev2.ReduceRuntimeStateWithDirective(facts, nil)
	if err != nil || preparedDecision.Action != runtimecontract.RuntimeActionAcquirePrerequisite {
		t.Fatalf("prepared authority facts=%+v decision=%+v err=%v", preparedFacts, preparedDecision, err)
	}
	for len(preparedFacts.Prerequisites.Pending) > 0 {
		prerequisite := preparedFacts.Prerequisites.Pending[0]
		result := runtimev2.PrerequisiteResult{PrerequisiteID: prerequisite.ID}
		switch prerequisite.Kind {
		case runtimecontract.PrerequisiteKindManifest:
			result.Manifest = &runtimecontract.WorkspaceManifest{Revision: "tree-current", TargetRefs: []string{"internal/lsp/manager.go"}}
		case runtimecontract.PrerequisiteKindTargetSelection:
			selection := runtimecontract.RuntimeTargetSelection{
				PrerequisiteID: prerequisite.ID, OwnerModuleID: prerequisite.OwnerModuleID,
				TargetRole: prerequisite.SubjectRef, SelectorRef: prerequisite.TargetSelectorRef,
				SelectorOwnerRef: prerequisite.TargetSelectorOwnerRef, SelectorSubjectRef: prerequisite.TargetSelectorSubjectRef,
				SelectorCandidateRefs: prerequisite.TargetSelectorCandidateRefs,
				Disposition:           runtimecontract.RuntimeTargetSelectionSelected, TargetRefs: []string{"internal/lsp/manager.go"},
				CandidateTargetRefs: []string{"internal/lsp/manager.go"}, CandidateCount: 1, ObservedRevision: "tree-current",
			}
			selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
			result.TargetSelection = &selection
		case runtimecontract.PrerequisiteKindTargetSnapshot:
			result.TargetSnapshot = &runtimecontract.TargetSnapshot{
				TargetRef: prerequisite.SubjectRef, Content: "package lsp", ContentHash: "manager-current", ContentState: runtimecontract.SnapshotContentStateComplete,
			}
		default:
			t.Fatalf("unexpected current authority prerequisite %+v", prerequisite)
		}
		preparedFacts, err = runtimev2.CommitPrerequisiteResult(preparedFacts, result)
		if err != nil {
			t.Fatalf("commit current authority prerequisite %s: %v", prerequisite.ID, err)
		}
		preparedFacts, preparedDecision, err = runtimev2.ReduceRuntimeStateWithDirective(preparedFacts, &preparedDecision.AuthoritySnapshot)
		if err != nil {
			t.Fatalf("reduce current authority prerequisite %s: %v", prerequisite.ID, err)
		}
	}
	if preparedDecision.Action != runtimecontract.RuntimeActionProduceStateChange {
		t.Fatalf("current target authority did not reach candidate action: %+v", preparedDecision)
	}
	historicalSelection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: "selection-old-go-mod", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		TargetRole: "module_definition", SelectorRef: "file/profile_declared_candidates",
		Disposition: runtimecontract.RuntimeTargetSelectionSelected, TargetRefs: []string{"go.mod"},
		CandidateTargetRefs: []string{"go.mod"}, CandidateCount: 1, ObservedRevision: "tree-old",
	}
	historicalSelection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(historicalSelection)
	historicalPrerequisite := runtimecontract.RuntimePrerequisite{
		ID: historicalSelection.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindTargetSelection,
		Scope: runtimecontract.RuntimePrerequisiteScopeFocus, FocusID: "focus-module", OwnerModuleID: runtimecontract.RuntimeModuleFile,
	}
	historicalObservation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: historicalPrerequisite.ID, Kind: historicalPrerequisite.Kind, OwnerModuleID: historicalPrerequisite.OwnerModuleID,
		ContentHash: historicalSelection.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: "workspace_deliverable", Revision: "tree-old",
		}},
	}
	historicalObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(historicalObservation)
	preparedFacts.Prerequisites.Satisfied = append(preparedFacts.Prerequisites.Satisfied, historicalPrerequisite)
	preparedFacts.Resources.TargetSelection = &historicalSelection
	preparedFacts.Resources.TargetSnapshots = []runtimecontract.TargetSnapshot{{
		TargetRef: "go.mod", ContentHash: "go-mod-old", ContentState: runtimecontract.SnapshotContentStateComplete,
	}}
	preparedFacts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{historicalObservation}
	repository := &runtimeV2DriveRepository{aggregate: runtimev2.RuntimeAggregate{
		StateRevision: 7, Definition: runtimeV2Definition(), Facts: preparedFacts, Decision: preparedDecision,
	}}
	biz := orchestratorbiz.NewRuntimeOrchestrator()
	if err := biz.ConfigureRuntimeV2Repository(repository); err != nil {
		t.Fatal(err)
	}
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	providerCalls := 0
	observer := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	observer.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(_ magicevent.Event, _ magicevent.Result) { providerCalls++ })
	t.Cleanup(func() { observer.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	module := newTestBiz(biz, hub, nil)

	next, err := module.driveRuntimeV2Decision(context.Background(), repository.aggregate, "unused-prefix")
	if err != nil {
		t.Fatal(err)
	}
	if providerCalls != 0 || next.StateRevision != 8 || next.Decision.Action != runtimecontract.RuntimeActionAcquirePrerequisite ||
		len(next.CandidateAttempts) != 0 || next.Facts.Repair.RepairAttempts != 0 ||
		next.Facts.Recovery.PrerequisiteRefreshes != 0 || next.Facts.Recovery.PrerequisiteResourceChangeRefreshes != 0 {
		t.Fatalf("pre-provider authority gate leaked into candidate/recovery path: %+v providerCalls=%d", next, providerCalls)
	}
	if next.Facts.Resources.TargetSelection != nil || len(next.Facts.Prerequisites.Retired) != 1 ||
		next.Facts.Prerequisites.Retired[0].ID != historicalPrerequisite.ID ||
		len(next.Facts.Resources.RetiredPrerequisiteObservations) != 1 ||
		next.Facts.Resources.RetiredPrerequisiteObservations[0].ObservationID != historicalObservation.ObservationID {
		t.Fatalf("historical authority was not retired before provider path: %+v", next.Facts)
	}
	var currentSelectionID string
	for _, prerequisite := range next.Facts.Prerequisites.Pending {
		if prerequisite.Kind == runtimecontract.PrerequisiteKindTargetSelection && prerequisite.FocusID == facts.Focus.ID {
			currentSelectionID = prerequisite.ID
			break
		}
	}
	if currentSelectionID == "" {
		t.Fatalf("current selection was not reacquired: %+v", next.Facts.Prerequisites.Pending)
	}
	for _, prerequisite := range next.Facts.Prerequisites.Satisfied {
		if prerequisite.ID == currentSelectionID {
			t.Fatalf("reacquired selection remains both pending and satisfied: %+v", next.Facts.Prerequisites)
		}
	}
}

type runtimeV2DriveRepository struct {
	aggregate runtimev2.RuntimeAggregate
}

func (r *runtimeV2DriveRepository) Load(_ context.Context, workspaceID common.WorkspaceID, runID common.RunID) (runtimev2.RuntimeAggregate, error) {
	if r == nil || r.aggregate.Facts.WorkspaceID != workspaceID || r.aggregate.Facts.RunID != runID {
		return runtimev2.RuntimeAggregate{}, common.ErrNotFound
	}
	return r.aggregate, nil
}

func (r *runtimeV2DriveRepository) Save(_ context.Context, aggregate runtimev2.RuntimeAggregate, expectedRevision uint64) error {
	if r == nil || r.aggregate.StateRevision != expectedRevision {
		return common.ErrConflict
	}
	r.aggregate = aggregate
	return nil
}
