package orchestrator

// Owner Biz drive tests.

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	approvalmodule "github.com/muidea/workorch/internal/modules/blocks/approval"
	approvalbiz "github.com/muidea/workorch/internal/modules/blocks/approval/biz"
	decisionbiz "github.com/muidea/workorch/internal/modules/blocks/decision/biz"
	filebiz "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	llmbiz "github.com/muidea/workorch/internal/modules/blocks/llm/biz"
	resultbiz "github.com/muidea/workorch/internal/modules/blocks/result/biz"
	runbiz "github.com/muidea/workorch/internal/modules/blocks/run/biz"
	workspacebiz "github.com/muidea/workorch/internal/modules/blocks/workspace/biz"

	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"

	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"

	magicevent "github.com/muidea/magicCommon/event"
	decisionmodule "github.com/muidea/workorch/internal/modules/blocks/decision"
	filemodule "github.com/muidea/workorch/internal/modules/blocks/file"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	gitmodule "github.com/muidea/workorch/internal/modules/blocks/git"
	gitbiz "github.com/muidea/workorch/internal/modules/blocks/git/biz"
	gitevents "github.com/muidea/workorch/internal/modules/blocks/git/pkg/events"
	llmmodule "github.com/muidea/workorch/internal/modules/blocks/llm"
	resultmodule "github.com/muidea/workorch/internal/modules/blocks/result"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	runmodule "github.com/muidea/workorch/internal/modules/blocks/run"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	shellmodule "github.com/muidea/workorch/internal/modules/blocks/shell"
	shellbiz "github.com/muidea/workorch/internal/modules/blocks/shell/biz"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	workspacemodule "github.com/muidea/workorch/internal/modules/blocks/workspace"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/llmaudit"
	"github.com/muidea/workorch/internal/pkg/repository"
	filerepository "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type panicAfterOwnerReceiptExecutor struct {
	delegate eventRuntimeOperationExecutor
}

// panicAfterOwnerReceiptCommitErrorExecutor models process loss after a
// capability owner applied its durable effect but before the owner receipt CAS
// was committed. The nested EventHub error is intentionally not converted into
// an outcome_unknown transition: the restart must use only the sealed proof.
type panicAfterOwnerReceiptCommitErrorExecutor struct {
	delegate eventRuntimeOperationExecutor
}

// countingRuntimeOperationExecutor keeps the P5-C process-restart fixture
// honest: both the initial governance-only attempt and the granted resume go
// through the real EventHub/File owner, while duplicate or stale governance
// resolutions must never invoke an additional owner operation.
type countingRuntimeOperationExecutor struct {
	delegate eventRuntimeOperationExecutor
	requests []runtime.RuntimeOperationExecutionRequest
}

func (e *countingRuntimeOperationExecutor) ExecuteRuntimeOperation(ctx context.Context, request runtime.RuntimeOperationExecutionRequest) (runtime.RuntimeOperationExecutionResult, error) {
	e.requests = append(e.requests, request)
	return e.delegate.ExecuteRuntimeOperation(ctx, request)
}

type failOnceResultRepository struct {
	repository.ResultRepository
	remainingFailures int
}

type failOnceCASRuntimeRepository struct {
	delegate          runtime.RuntimeStateRepository
	remainingFailures int
}

// runtimeFocusAdvanceReceiptExecutor stands in for an already admissible
// owner receipt while exercising the SM-2 evidence/coverage/focus CAS. Durable
// effect replay belongs to P5; this fixture proves the next authority cursor
// is atomic once that receipt has been persisted.
type runtimeFocusAdvanceReceiptExecutor struct {
	owner *runtime.RuntimeController
	calls int
}

func (e *runtimeFocusAdvanceReceiptExecutor) ExecuteRuntimeOperation(ctx context.Context, request runtime.RuntimeOperationExecutionRequest) (runtime.RuntimeOperationExecutionResult, error) {
	e.calls++
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: request.Execution.ExecutionKey, OperationID: request.Operation.ID,
		Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: request.Execution.ExpectedEffectScope,
		Changed:    runtimecontract.RuntimeEffectIsDurable(request.Execution.ExpectedEffectScope),
		TargetRefs: append([]string(nil), request.Operation.TargetRefs...),
		Evidence:   runtimecontract.RuntimeOperationEvidence{Summary: "focus-one receipt committed"},
	}
	_, committed, err := runtimeTestControllerCommitOperationReceipt(e.owner, ctx, request.WorkspaceID, request.RunID, request.DispatchID, request.Execution.Fence, receipt)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	return runtime.RuntimeOperationExecutionResult{
		EventID: "focus-advance-evidence-" + request.Operation.ID, OperationID: request.Operation.ID,
		EvidenceKind: request.EvidenceKind, Status: runtime.ExecutionEvidenceStatusAccepted,
		SourceRef: "runtime://focus-advance/" + request.Operation.ID, Evidence: committed.Evidence, Receipt: committed,
	}, nil
}

type runtimeFailIfOperationExecuted struct{ calls int }

func (e *runtimeFailIfOperationExecuted) ExecuteRuntimeOperation(context.Context, runtime.RuntimeOperationExecutionRequest) (runtime.RuntimeOperationExecutionResult, error) {
	e.calls++
	return runtime.RuntimeOperationExecutionResult{}, errors.New("persisted receipt must commit evidence without re-executing the operation")
}

// inMemoryRuntimeStateRepository keeps the candidate/stale mainline test
// independent of the Run owner. Production recovery is covered by the
// file-backed fixtures; this direct repository only proves the controller CAS
// transition owned by Orchestrator.
type inMemoryRuntimeStateRepository struct {
	aggregate *runtime.RuntimeAggregate
}

func (r *inMemoryRuntimeStateRepository) Load(_ context.Context, workspaceID common.WorkspaceID, runID common.RunID) (runtime.RuntimeAggregate, error) {
	if r == nil || r.aggregate == nil || r.aggregate.Facts.WorkspaceID != workspaceID || r.aggregate.Facts.RunID != runID {
		return runtime.RuntimeAggregate{}, common.ErrNotFound
	}
	// Mirror the Run owner's record conversion: callers must never mutate the
	// persisted aggregate through a shared slice/map backing array before their
	// CAS succeeds. This matters for crash-boundary tests such as request
	// content written before the session-fragment CAS.
	return runtimeAggregateFromRecord(runtimeStateRecordFromAggregate(*r.aggregate)), nil
}

func (r *inMemoryRuntimeStateRepository) Save(_ context.Context, aggregate runtime.RuntimeAggregate, expectedRevision uint64) error {
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

type runtimeSaveCrashRepository struct {
	delegate runtime.RuntimeStateRepository
	match    func(runtime.RuntimeAggregate) bool
	after    bool
	fired    bool
	marker   string
}

func (r *runtimeSaveCrashRepository) Load(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (runtime.RuntimeAggregate, error) {
	return r.delegate.Load(ctx, workspaceID, runID)
}

func (r *runtimeSaveCrashRepository) Save(ctx context.Context, aggregate runtime.RuntimeAggregate, expectedRevision uint64) error {
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

func (r *failOnceCASRuntimeRepository) Load(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (runtime.RuntimeAggregate, error) {
	return r.delegate.Load(ctx, workspaceID, runID)
}

func (r *failOnceCASRuntimeRepository) Save(ctx context.Context, aggregate runtime.RuntimeAggregate, expectedRevision uint64) error {
	if expectedRevision > 0 && r.remainingFailures > 0 {
		r.remainingFailures--
		// Model a real competing aggregate writer: a generic storage conflict
		// with no aggregate revision drift is now an owner-local rebase/transient
		// concern, not permission for SM-4 CAS recovery.
		current, err := r.delegate.Load(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
		if err != nil {
			return err
		}
		competing := current
		competing.StateRevision++
		if err := r.delegate.Save(ctx, competing, current.StateRevision); err != nil {
			return err
		}
		return &runtime.RuntimeCASConflictError{
			WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
			ExpectedRevision: expectedRevision, ObservedRevision: competing.StateRevision,
		}
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

func (e panicAfterOwnerReceiptExecutor) ExecuteRuntimeOperation(ctx context.Context, request runtime.RuntimeOperationExecutionRequest) (runtime.RuntimeOperationExecutionResult, error) {
	_, err := e.delegate.ExecuteRuntimeOperation(ctx, request)
	if err != nil {
		return runtime.RuntimeOperationExecutionResult{}, err
	}
	panic("runtime-test-crash-after-owner-receipt")
}

func (e panicAfterOwnerReceiptCommitErrorExecutor) ExecuteRuntimeOperation(ctx context.Context, request runtime.RuntimeOperationExecutionRequest) (runtime.RuntimeOperationExecutionResult, error) {
	result, err := e.delegate.ExecuteRuntimeOperation(ctx, request)
	if err != nil {
		panic("runtime-test-crash-after-owner-receipt-commit-error")
	}
	return result, nil
}

func TestDriveRuntimeFileBackedRunExecutesDurableFileOwnerAndRestarts(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	root := t.TempDir()
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	bootstrap := runtimeBootstrap()
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2",
		DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive,
	}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	startRuntimeReceiptTestOwner(t, hub)

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
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{
			ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir,
		}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })

	fileOwner := filemodule.New()
	if err := fileOwner.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := fileOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fileOwner.Stop(ctx) })
	shellOwner := shellmodule.New()
	if err := shellOwner.Configure(shellbiz.Dependencies{EventHub: hub, RuntimeAdapter: shellbiz.NewRuntimeAdapter()}); err != nil {
		t.Fatal(err)
	}
	if err := shellOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = shellOwner.Stop(ctx) })
	candidateCount := 0
	startRuntimeProviderCeilingTestOwner(t, hub)
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		candidateCount++
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		var candidate runtimecontract.RuntimeCandidate
		switch command.Decision.State {
		case runtimecontract.ConvergenceStateNeedStateChange:
			candidate = runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
				ID: "write-1", Kind: runtimecontract.StepKindFileEdit,
				CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"},
				Parameters: runtimeFileEditParameters("target.txt"),
			}}}}}
		case runtimecontract.ConvergenceStateNeedAssessment:
			evidenceRefs := make([]string, 0, len(command.CandidateContext.Evidence.Accepted))
			for _, evidence := range command.CandidateContext.Evidence.Accepted {
				for _, coverageID := range evidence.CoverageWorkItemIDs {
					if coverageID == "cwi-v2" {
						evidenceRefs = append(evidenceRefs, string(evidence.ID))
						break
					}
				}
			}
			candidate = runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{
				FocusID: command.Decision.Focus.ID, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
				Disposition: runtimecontract.AssessmentDispositionSatisfied, Summary: "file evidence accepted",
				Items: []runtimecontract.RuntimeAssessmentItemFact{{CoverageWorkItemID: "cwi-v2", Satisfied: true, EvidenceRefs: evidenceRefs}}, EvidenceRefs: evidenceRefs,
			}}
		default:
			t.Errorf("unexpected candidate decision state %q", command.Decision.State)
			return
		}
		result.Set(runtimeCandidateResultForTest(t, command, candidate), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })

	runtimeTestPersistBootstrappedOperationFixture(t, ctx, hub, "ws-v2", "run-v2", bootstrap)
	module := newRuntimeDriveTestModule(t, hub)
	driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntime, module.handleRuntimeDrive)
	command := orchestratorevents.DriveRuntimeCommand{WorkspaceID: "ws-v2", RunID: "run-v2"}
	completed, err := DriveRuntime(ctx, hub, command)
	if err != nil {
		t.Fatal(err)
	}
	content, readErr := os.ReadFile(filepath.Join(deliverable, "target.txt"))
	if readErr != nil || string(content) != "new\n" {
		t.Fatalf("durable file content = %q err=%v", string(content), readErr)
	}
	persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.RuntimeState == nil || len(persisted.RuntimeState.OperationReceipts) != 1 {
		t.Fatalf("file-backed runtime state = %+v err=%v", persisted, err)
	}
	receipt := persisted.RuntimeState.OperationReceipts[0]
	if receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || receipt.ObservedEffectScope != runtimecontract.RuntimeEffectWorkspaceDurable || !receipt.Changed || receipt.BeforeHash == receipt.AfterHash {
		t.Fatalf("durable owner receipt = %+v", receipt)
	}

	driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntime)
	restarted := newRuntimeDriveTestModule(t, hub)
	driveObserver = magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntime, restarted.handleRuntimeDrive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntime) })
	replayed, err := DriveRuntime(ctx, hub, command)
	if err != nil || replayed.StateRevision != completed.StateRevision || candidateCount != 2 {
		t.Fatalf("file-backed restart replay=%+v complete=%+v candidates=%d err=%v", replayed, completed, candidateCount, err)
	}
}

// TestRuntimeFileBackedFocusScopedTargetsStayRetiredAcrossDurableWrites is the
// deployed failure regression: each bootstrap declaration has a bounded File
// selector candidate, SM-2 seals the owner-confirmed exact target, and each
// durable write changes workspace_tree. The next focus must not reactivate a
// completed focus's target snapshot freshness authority.
func TestRuntimeFileBackedFocusScopedTargetsStayRetiredAcrossDurableWrites(t *testing.T) {
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
	targets := map[common.CoverageWorkItemID]string{
		"cwi-module":  "go.mod",
		"cwi-manager": "internal/lsp/manager.go",
		"cwi-test":    "internal/lsp/manager_test.go",
	}
	recipes := map[common.CoverageWorkItemID]string{
		"cwi-module":  "focus-scoped-validation-module",
		"cwi-manager": "focus-scoped-validation-manager",
		"cwi-test":    "focus-scoped-validation-test",
	}
	for _, target := range targets {
		if err := os.WriteFile(filepath.Join(deliverable, target), []byte("before\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	bootstrap := runtimeFocusScopedSelectionBootstrap()
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
		DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive,
	}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	startRuntimeReceiptTestOwner(t, hub)

	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{
			ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir,
		}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	fileOwner := filemodule.New()
	if err := fileOwner.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := fileOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fileOwner.Stop(ctx) })
	shellOwner := shellmodule.New()
	if err := shellOwner.Configure(shellbiz.Dependencies{EventHub: hub, RuntimeAdapter: shellbiz.NewRuntimeAdapter()}); err != nil {
		t.Fatal(err)
	}
	if err := shellOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = shellOwner.Stop(ctx) })

	providerCalls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	providerCeilingObserver := magicevent.NewSimpleObserverWithMatchID(
		runtimecontract.RuntimeModuleLLM+"/prerequisite",
		orchestratorevents.PrerequisiteCommandDestination,
		hub,
	)
	providerCeilingObserver.Subscribe(llmevents.TopicAcquireProviderCeiling, func(event magicevent.Event, result magicevent.Result) {
		command, ok := event.Data().(llmevents.AcquireProviderCeilingCommand)
		if !ok {
			t.Errorf("provider ceiling command = %T", event.Data())
			return
		}
		ceiling := runtimecontract.ProviderCeiling{
			Selection: command.Selection, ContextWindowTokens: 65536, ProviderMaxOutputTokens: 8192,
			EffectiveMaxOutputTokens: 8192, MaxInputTokens: 57344, MaxBytes: 256 * 1024,
			CapabilityRevision: "focus-scoped-test-capability", ConfigurationRevision: "focus-scoped-test-config",
		}
		observation := runtimecontract.RuntimePrerequisiteObservation{
			PrerequisiteID: command.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindProviderCeiling,
			OwnerModuleID: runtimecontract.RuntimeModuleLLM, ContentHash: "focus-scoped-test-provider-ceiling",
			Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
				Domain: runtimecontract.RuntimeResourceProviderConfig, OwnerModuleID: runtimecontract.RuntimeModuleLLM,
				SubjectRef: command.Selection.Target.ProviderID + "/" + command.Selection.Target.ModelID,
				Revision:   ceiling.ConfigurationRevision,
			}},
		}
		observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
		result.Set(llmevents.AcquireProviderCeilingResult{
			WorkspaceID: command.WorkspaceID, RunID: command.RunID, AuthoritySnapshotID: command.AuthoritySnapshotID,
			Ceiling: ceiling, Observation: observation,
		}, nil)
	})
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		if command.DocumentKind == runtimecontract.RuntimeProviderDocumentSemanticPlan {
			document := runtimecontract.RuntimeSemanticPlanDocument{
				SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent,
				ManifestHash:  command.SemanticManifest.Hash, DefinitionSeedHash: command.SemanticManifest.DefinitionSeedHash,
				PlanStep: command.SemanticContext.PlanStep,
			}
			switch command.SemanticContext.PlanStep {
			case runtimecontract.RuntimeSemanticPlanStepCoverageOrder:
				manifestCoverage := make(map[common.CoverageWorkItemID]struct{}, len(command.SemanticManifest.CoverageItems))
				for _, item := range command.SemanticManifest.CoverageItems {
					manifestCoverage[item.ID] = struct{}{}
				}
				for _, coverageID := range []common.CoverageWorkItemID{"cwi-module", "cwi-manager", "cwi-test"} {
					if _, found := manifestCoverage[coverageID]; found {
						document.CoverageOrder = append(document.CoverageOrder, coverageID)
					}
				}
				if len(document.CoverageOrder) != len(manifestCoverage) {
					t.Errorf("semantic manifest has unexpected coverage set %+v", command.SemanticManifest.CoverageItems)
					return
				}
			case runtimecontract.RuntimeSemanticPlanStepProofBatch:
				document.BatchID = command.SemanticContext.BatchID
				document.BatchSequence = command.SemanticContext.BatchSequence
				document.BatchCoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), command.SemanticContext.BatchCoverageWorkItemIDs...)
				for _, coverageID := range command.SemanticContext.BatchCoverageWorkItemIDs {
					target, known := targets[coverageID]
					if !known {
						t.Errorf("semantic batch references unexpected coverage %q", coverageID)
						return
					}
					document.Proposals = append(document.Proposals, runtimecontract.RuntimeSemanticProofProposal{
						CoverageWorkItemID: coverageID, OwnerModuleID: runtimecontract.RuntimeModuleFile,
						OperationKind: runtimecontract.StepKindFileEdit,
						Target: runtimecontract.RuntimeOperationTargetAuthorization{
							Policy: runtimecontract.RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{target},
						},
						RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
						RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
						DeclarationRef:        "focus-scoped-semantic/" + string(coverageID),
					})
					// One canonical CWI may require both a durable change and a
					// validation proof. They are independent owner facts, but SM-2
					// compiles them into one CWI binding before any operation request.
					document.Proposals = append(document.Proposals, runtimecontract.RuntimeSemanticProofProposal{
						CoverageWorkItemID: coverageID, OwnerModuleID: runtimecontract.RuntimeModuleShell,
						OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession,
						Target: runtimecontract.RuntimeOperationTargetAuthorization{
							Policy: runtimecontract.RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{target},
						},
						RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
						RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectEphemeralSession},
						ValidationRecipeID:    recipes[coverageID],
						DeclarationRef:        "focus-scoped-semantic/" + string(coverageID),
					})
				}
			default:
				t.Errorf("unexpected semantic plan step %q", command.SemanticContext.PlanStep)
				return
			}
			payload, err := runtimecontract.EncodeRuntimeSemanticPlanPayloadWithManifest(command.SemanticManifest, document)
			if err != nil {
				t.Errorf("encode semantic plan: %v", err)
				return
			}
			result.Set(runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{
				DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity,
				ExpectedDocumentHash: runtimeExpectedDocumentHash(command.ExpectedDocument),
				DocumentOutcome:      runtimeCompleteDocumentOutcome(command.ExpectedDocument, payload),
			}), nil)
			return
		}
		if command.DocumentKind != runtimecontract.RuntimeProviderDocumentOperations {
			t.Errorf("unexpected provider document kind %q", command.DocumentKind)
			return
		}
		providerCalls++
		focusID := command.Decision.Focus.ID
		coverageID := command.Decision.Focus.CoverageWorkItemIDs[0]
		target, known := targets[coverageID]
		if !known {
			t.Errorf("unexpected coverage %q in focus %q", coverageID, focusID)
			return
		}
		// The semantic proposal seals an exact target. Its candidate authority
		// must include the File owner's matching snapshot. The source selector
		// can remain as immutable replay/audit data, but is not the target-policy
		// authority for this proposal.
		selection := command.CandidateContext.Resources.TargetSelection
		snapshots := command.CandidateContext.Resources.TargetSnapshots
		if len(snapshots) != 1 || snapshots[0].TargetRef != target || !snapshots[0].Complete() {
			t.Errorf("candidate %s received non-current exact target authority selection=%+v snapshots=%+v", focusID, selection, snapshots)
			return
		}
		response := llmevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity, ExpectedDocumentHash: runtimeExpectedDocumentHash(command.ExpectedDocument)}
		var candidate runtimecontract.RuntimeCandidate
		switch command.Decision.State {
		case runtimecontract.ConvergenceStateNeedStateChange:
			oldText, newText := "before\n", "after-"+focusID+"\n"
			candidate = runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
				ID: "edit-" + focusID, Kind: runtimecontract.StepKindFileEdit,
				CoverageWorkItemIDs: []common.CoverageWorkItemID{coverageID}, TargetRefs: []string{target},
				Parameters: []runtimecontract.OperationParameter{{Name: "oldText", String: &oldText}, {Name: "newText", String: &newText}},
			}}}}}
		case runtimecontract.ConvergenceStateNeedValidation:
			candidate = runtimeManagedShellValidationCandidate(
				"validate-"+focusID,
				coverageID,
				target,
				recipes[coverageID],
			)
		case runtimecontract.ConvergenceStateNeedAssessment:
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
			t.Errorf("unexpected action %q for %s", command.Decision.State, focusID)
			return
		}
		document := runtimeCandidateDocumentForTest(t, command.CandidateManifest, candidate)
		response.DocumentOutcome = runtimeCompleteDocumentOutcome(command.ExpectedDocument, document)
		result.Set(runtimeProviderResultWithAuditForTest(command, response), nil)
	})
	t.Cleanup(func() {
		llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate)
		providerCeilingObserver.Unsubscribe(llmevents.TopicAcquireProviderCeiling)
	})

	facts, err := bootstrap.InitialFacts("ws-v2", "run-v2")
	if err != nil {
		t.Fatalf("build fresh runtime facts: %v", err)
	}
	initializer := newRuntimeTestOwner()
	if err := initializer.ConfigureRuntimeRepository(runtimeStateRepository{hub: hub}); err != nil {
		t.Fatalf("configure fresh runtime controller: %v", err)
	}
	initial, err := initializer.InitializeRuntime(ctx, facts, bootstrap.Definition, bootstrap.Requirements)
	if err != nil {
		t.Fatalf("initialize fresh runtime aggregate: %v", err)
	}
	if initial.Facts.Focus.ID == "" || initial.Facts.Focus.TargetSelectorRef != "" || initial.Facts.Focus.TargetSelectorOwnerRef != "" ||
		initial.Facts.Focus.TargetSelectorSubjectRef != "" || len(initial.Facts.Focus.TargetSelectorCandidateRefs) != 0 ||
		len(initial.Facts.Focus.TargetDiscoverySources) != 0 {
		t.Fatalf("pre-semantic queue leaked immutable target-grounding authority: %+v", initial.Facts.Focus)
	}
	if len(initial.Facts.DefinitionFocuses) != 3 {
		t.Fatalf("immutable DefinitionFocus inventory = %+v", initial.Facts.DefinitionFocuses)
	}
	declaration := initial.Facts.DefinitionFocuses[0]
	if declaration.TargetSelectorRef != "file/profile_declared_candidates" || declaration.TargetSelectorOwnerRef != "profile-bootstrap" ||
		declaration.TargetSelectorSubjectRef != "module" ||
		!runtimecontract.SameRuntimeTargetSelectorCandidateRefs(declaration.TargetSelectorCandidateRefs, []string{"go.mod"}) ||
		!runtimeTestStringIncluded(declaration.TargetDiscoverySources, runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest) {
		t.Fatalf("immutable DefinitionFocus lost target-grounding declaration: %+v", declaration)
	}
	module := newRuntimeGroundedTargetSequenceModule(t, hub, runtimeStateRepository{hub: hub})
	driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntime, module.handleRuntimeDrive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntime) })
	completed, err := DriveRuntime(ctx, hub, orchestratorevents.DriveRuntimeCommand{WorkspaceID: "ws-v2", RunID: "run-v2"})
	if err != nil || completed.Terminal == nil || completed.State != runtimecontract.ConvergenceStateComplete {
		t.Fatalf("focus-scoped sequence completion=%+v terminal=%+v err=%v", completed, completed.Terminal, err)
	}
	if providerCalls != 9 {
		t.Fatalf("provider calls=%d, want three candidates for each of three focuses", providerCalls)
	}
	persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.RuntimeState == nil {
		t.Fatalf("persisted focus-scoped aggregate=%+v err=%v", persisted, err)
	}
	state := persisted.RuntimeState
	if len(state.Facts.Focus.CoverageWorkItemIDs) != 1 || state.Facts.Focus.CoverageWorkItemIDs[0] != "cwi-test" ||
		len(state.Facts.Resources.TargetSnapshots) != 1 || state.Facts.Resources.TargetSnapshots[0].TargetRef != targets["cwi-test"] || !state.Facts.Resources.TargetSnapshots[0].Complete() {
		t.Fatalf("current target authority=%+v", state.Facts)
	}
	activeIDs := map[string]bool{}
	for _, prerequisite := range append(append([]runtimecontract.RuntimePrerequisite(nil), state.Facts.Prerequisites.Pending...), state.Facts.Prerequisites.Satisfied...) {
		activeIDs[prerequisite.ID] = true
	}
	retiredSnapshotTargets := map[string]bool{}
	for _, prerequisite := range state.Facts.Prerequisites.Retired {
		if prerequisite.Kind == runtimecontract.PrerequisiteKindTargetSnapshot {
			if activeIDs[prerequisite.ID] {
				t.Fatalf("retired target snapshot re-entered active prerequisite graph: %+v", prerequisite)
			}
			retiredSnapshotTargets[prerequisite.SubjectRef] = true
		}
	}
	if !retiredSnapshotTargets[targets["cwi-module"]] || !retiredSnapshotTargets[targets["cwi-manager"]] {
		t.Fatalf("completed-focus target snapshots were not retained as audit-only history: %+v", state.Facts.Prerequisites.Retired)
	}
	for _, observation := range state.Facts.Resources.RetiredPrerequisiteObservations {
		if observation.Kind == runtimecontract.PrerequisiteKindTargetSnapshot {
			if activeIDs[observation.PrerequisiteID] {
				t.Fatalf("retired target snapshot observation re-entered active freshness: %+v", observation)
			}
		}
	}
	for coverageID, target := range targets {
		content, readErr := os.ReadFile(filepath.Join(deliverable, target))
		if readErr != nil || !strings.HasPrefix(string(content), "after-semantic-focus-") {
			t.Fatalf("%s durable write content=%q err=%v", coverageID, content, readErr)
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
func TestRuntimeFileBackedGroundedTargetStateSequenceRestartsAtCommittedBoundaries(t *testing.T) {
	boundaries := []struct {
		name  string
		match func(runtime.RuntimeAggregate) bool
	}{
		{
			name: "target_selection",
			match: func(aggregate runtime.RuntimeAggregate) bool {
				return aggregate.Facts.Resources.TargetSelection != nil
			},
		},
		{
			name: "candidate_proposal",
			match: func(aggregate runtime.RuntimeAggregate) bool {
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
			match: func(aggregate runtime.RuntimeAggregate) bool {
				return len(aggregate.OperationReceipts) == 1 && len(aggregate.Facts.Evidence.Accepted) == 1
			},
		},
		{
			name: "assessment_terminal",
			match: func(aggregate runtime.RuntimeAggregate) bool {
				return aggregate.Decision.State == runtimecontract.ConvergenceStateComplete && aggregate.Decision.Terminal != nil
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

				bootstrap := runtimeGroundedTargetSequenceBootstrap()
				bootstrap.DefinitionFocuses[0].TargetSelectorRef = "file/workspace_deliverable"
				bootstrap = runtimeTestBootstrapWithReboundBindings(bootstrap)
				runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
				workspaces := filerepository.NewWorkspaceRepository(filepath.Join(root, "workspaces.json"))
				results := filerepository.NewResultRepository(filepath.Join(root, "results.json"))
				if err := runs.Create(ctx, &runmodel.Run{
					ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
					DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive,
				}); err != nil {
					t.Fatal(err)
				}
				if err := workspaces.Create(ctx, runtimeWorkspaceForPersistedRunTest(t, ctx, runs, "ws-v2", "run-v2", deliverable, internalDir)); err != nil {
					t.Fatal(err)
				}
				runOwner := runmodule.New()
				if err := runOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: hub}); err != nil {
					t.Fatal(err)
				}
				if err := runOwner.Start(); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = runOwner.Stop(ctx) })
				workspaceOwner := workspacemodule.New()
				if err := workspaceOwner.Configure(workspacebiz.Dependencies{
					Workspaces: workspaces, DeletionMaterialCleaner: workspaces, Goals: memory.NewGoalRepository(), DoDs: memory.NewDoDRepository(),
					Templates: memory.NewWorkspaceTemplateRepository(), EventHub: hub,
				}); err != nil {
					t.Fatal(err)
				}
				if err := workspaceOwner.Start(); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = workspaceOwner.Stop(ctx) })
				resultOwner := resultmodule.New()
				if err := resultOwner.Configure(resultbiz.Dependencies{OwnerRepositories: testResultOwnerRepositories(results, memory.NewArtifactRepository(), memory.NewResultWorkspaceDeletionRepository()), EventHub: hub}); err != nil {
					t.Fatal(err)
				}
				if err := resultOwner.Start(); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = resultOwner.Stop(ctx) })
				fileOwner := filemodule.New()
				if err := fileOwner.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
					t.Fatal(err)
				}
				if err := fileOwner.Start(); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = fileOwner.Stop(ctx) })
				startRuntimeManagedValidationTestOwner(t, hub)
				startRuntimeReceiptTestOwner(t, hub)

				providerCalls := 0
				llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
				llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
					providerCalls++
					command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
					candidate, err := runtimeGroundedTargetSequenceCandidate(command)
					if err != nil {
						t.Errorf("grounded sequence candidate: %v", err)
						result.Set(runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{
							Transaction: command.Transaction, DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity,
							ExpectedDocumentHash: runtimeExpectedDocumentHash(command.ExpectedDocument),
							Feedback:             runtimeProviderFeedbackForTest(t, command, runtimecontract.RuntimeCandidateFailure{Class: runtimecontract.RuntimeCandidateFailureClassInternal, Reason: err.Error()}),
						}), nil)
						return
					}
					result.Set(runtimeCandidateResultForTest(t, command, candidate), nil)
				})
				t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })

				runtimeTestPersistBootstrappedOperationFixture(t, ctx, hub, "ws-v2", "run-v2", bootstrap)
				crashMarker := "grounded-sequence-" + tc.name + "-" + phase
				crashRepository := &runtimeSaveCrashRepository{
					delegate: runtimeStateRepository{hub: hub}, match: tc.match, after: after, marker: crashMarker,
				}
				crashingModule := newRuntimeGroundedTargetSequenceModule(t, hub, crashRepository, candidateRequestContents)
				func() {
					defer func() {
						if recovered := recover(); recovered != crashMarker {
							t.Fatalf("unexpected crash marker: %v", recovered)
						}
					}()
					if err := crashingModule.scheduleRuntimeDrive(ctx, "ws-v2", "run-v2", crashMarker); err != nil {
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

				restarted := newRuntimeGroundedTargetSequenceModule(t, hub, runtimeStateRepository{hub: hub}, candidateRequestContents)
				if err := restarted.scheduleRuntimeDrive(ctx, "ws-v2", "run-v2", "restart:"+tc.name+":"+phase); err != nil {
					t.Fatalf("restart drive: %v", err)
				}
				persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
				if err != nil || persisted.RuntimeState == nil || persisted.State != common.RunStateCompleted {
					t.Fatalf("restart terminal run=%+v state=%+v err=%v", persisted, persisted.RuntimeState, err)
				}
				state := persisted.RuntimeState
				if state.Decision.State != runtimecontract.ConvergenceStateComplete || state.Decision.Terminal == nil ||
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

// TestRuntimeFileBackedAmbiguousTargetSelectionFailsWithoutProvider keeps
// target grounding on the production typed-owner path. A profile may declare
// File's neutral unique-entry selector, but a multi-file workspace is an
// explicit ambiguous observation, not permission for the Orchestrator, LLM,
// or validation entry to choose an implementation target.
func TestRuntimeFileBackedAmbiguousTargetSelectionFailsWithoutProvider(t *testing.T) {
	t.Run("state_change", func(t *testing.T) {
		testRuntimeFileBackedUnresolvedTargetSelectionFailsWithoutProvider(t, runtimeGroundedTargetSequenceBootstrap(), runtimecontract.RuntimeTargetSelectionAmbiguous, map[string]string{"validation-entry.go": "package fixture\n", "implementation.go": "package fixture\n"})
	})
	t.Run("direct_validation", func(t *testing.T) {
		testRuntimeFileBackedUnresolvedTargetSelectionFailsWithoutProvider(t, runtimeGroundedTargetValidationBootstrap(), runtimecontract.RuntimeTargetSelectionAmbiguous, map[string]string{"validation-entry.go": "package fixture\n", "implementation.go": "package fixture\n"})
	})
}

// The legacy unique-entry selector reports an empty directory as absent. The
// production workspace-deliverable selector has a separate owner-grounded
// creation path below; neither path requests TUI/Web target input.
func TestRuntimeFileBackedAbsentTargetSelectionFailsWithoutProvider(t *testing.T) {
	testRuntimeFileBackedUnresolvedTargetSelectionFailsWithoutProvider(t, runtimeGroundedTargetSequenceBootstrap(), runtimecontract.RuntimeTargetSelectionAbsent, nil)
}

func TestRuntimeFileBackedWorkspaceDeliverableWithoutProfileArtifactFailsWithoutProvider(t *testing.T) {
	bootstrap := runtimeGroundedTargetSequenceBootstrap()
	bootstrap.DefinitionFocuses[0].TargetSelectorRef = "file/workspace_deliverable"
	bootstrap = runtimeTestBootstrapWithReboundBindings(bootstrap)
	testRuntimeFileBackedUnresolvedTargetSelectionFailsWithoutProvider(t, bootstrap, runtimecontract.RuntimeTargetSelectionScope, nil)
}

func TestRuntimeFileBackedEmptyWorkspaceCreationSelectionContinuesThroughCandidate(t *testing.T) {
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
	bootstrap := runtimeGroundedTargetSequenceBootstrap()
	bootstrap.Definition.CapabilityContract = []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindManagedValidationSession}
	bootstrap.DefinitionFocuses[0].TargetSelectorRef = "file/workspace_deliverable"
	bootstrap.Requirements[0].RequiredCapabilities = []string{runtimecontract.StepKindFileWrite}
	runtimeTestDeclareProspectiveWorkspaceTarget(&bootstrap, "implementation.txt")
	bootstrap.Requirements = append(bootstrap.Requirements, runtimecontract.RuntimeRequirement{
		Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		RequiredCapabilities: []string{runtimecontract.StepKindFileWrite}, Reason: "empty workspace File creation inventory",
	})
	bootstrap = runtimeTestBootstrapWithReboundBindings(bootstrap)
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID, DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	startRuntimeReceiptTestOwner(t, hub)
	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	fileOwner := filemodule.New()
	if err := fileOwner.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := fileOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fileOwner.Stop(ctx) })
	startRuntimeManagedValidationTestOwner(t, hub)
	candidateCalls := 0
	initialSelection := runtimecontract.RuntimeTargetSelectionDisposition("")
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		candidateCalls++
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		if candidateCalls == 1 && command.CandidateContext.Resources.TargetSelection != nil {
			initialSelection = command.CandidateContext.Resources.TargetSelection.Disposition
		}
		result.Set(runtimeCandidateResultForTest(t, command, runtimeEmptyWorkspaceCreationCandidate(command)), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })
	runtimeTestPersistBootstrappedOperationFixture(t, ctx, hub, "ws-v2", "run-v2", bootstrap)
	module := newRuntimeGroundedTargetSequenceModule(t, hub, runtimeStateRepository{hub: hub})
	driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntime, module.handleRuntimeDrive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntime) })

	completed, err := DriveRuntime(ctx, hub, orchestratorevents.DriveRuntimeCommand{WorkspaceID: "ws-v2", RunID: "run-v2"})
	if err != nil || completed.Terminal == nil || completed.State != runtimecontract.ConvergenceStateComplete || candidateCalls != 2 {
		t.Fatalf("empty workspace creation completion=%+v terminal=%+v candidateCalls=%d err=%v", completed, completed.Terminal, candidateCalls, err)
	}
	content, err := os.ReadFile(filepath.Join(deliverable, "implementation.txt"))
	if err != nil || string(content) != "created\n" {
		t.Fatalf("empty workspace creation file=%q err=%v", string(content), err)
	}
	persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	// The realized create-only selector is reobserved as unsupported after the
	// File effect. That prevents a second create authorization; the accepted
	// receipt plus owner reconciliation, rather than a rewritten Selected fact,
	// proves the realized target.
	if err != nil || persisted.RuntimeState == nil || initialSelection != runtimecontract.RuntimeTargetSelectionCreate ||
		persisted.RuntimeState.Facts.Resources.TargetSelection == nil || persisted.RuntimeState.Facts.Resources.TargetSelection.Disposition != runtimecontract.RuntimeTargetSelectionUnsupported ||
		len(persisted.RuntimeState.Facts.Resources.TargetSnapshots) != 0 || len(persisted.RuntimeState.OperationReceipts) != 1 {
		if persisted != nil && persisted.RuntimeState != nil {
			state := persisted.RuntimeState
			t.Fatalf("empty workspace creation initialSelection=%s state=%s selection=%+v snapshots=%d receipts=%d err=%v", initialSelection, state.Decision.State, state.Facts.Resources.TargetSelection, len(state.Facts.Resources.TargetSnapshots), len(state.OperationReceipts), err)
		}
		t.Fatalf("empty workspace creation authority=%+v err=%v", persisted, err)
	}
}

func TestRuntimeFileBackedEmptyWorkspaceCreationReceiptCrashRestartDoesNotReplayCreate(t *testing.T) {
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
			bootstrap := runtimeGroundedTargetSequenceBootstrap()
			bootstrap.Definition.CapabilityContract = []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindManagedValidationSession}
			bootstrap.DefinitionFocuses[0].TargetSelectorRef = "file/workspace_deliverable"
			bootstrap.Requirements[0].RequiredCapabilities = []string{runtimecontract.StepKindFileWrite}
			runtimeTestDeclareProspectiveWorkspaceTarget(&bootstrap, "implementation.txt")
			bootstrap.Requirements = append(bootstrap.Requirements, runtimecontract.RuntimeRequirement{
				Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleFile,
				RequiredCapabilities: []string{runtimecontract.StepKindFileWrite}, Reason: "empty workspace File creation inventory",
			})
			bootstrap = runtimeTestBootstrapWithReboundBindings(bootstrap)
			runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
			workspaces := filerepository.NewWorkspaceRepository(filepath.Join(root, "workspaces.json"))
			results := filerepository.NewResultRepository(filepath.Join(root, "results.json"))
			if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID, DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive}); err != nil {
				t.Fatal(err)
			}
			if err := workspaces.Create(ctx, runtimeWorkspaceForPersistedRunTest(t, ctx, runs, "ws-v2", "run-v2", deliverable, internalDir)); err != nil {
				t.Fatal(err)
			}
			runOwner := runmodule.New()
			if err := runOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: hub}); err != nil {
				t.Fatal(err)
			}
			if err := runOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = runOwner.Stop(ctx) })
			workspaceOwner := workspacemodule.New()
			if err := workspaceOwner.Configure(workspacebiz.Dependencies{
				Workspaces: workspaces, DeletionMaterialCleaner: workspaces, Goals: memory.NewGoalRepository(), DoDs: memory.NewDoDRepository(),
				Templates: memory.NewWorkspaceTemplateRepository(), EventHub: hub,
			}); err != nil {
				t.Fatal(err)
			}
			if err := workspaceOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = workspaceOwner.Stop(ctx) })
			resultOwner := resultmodule.New()
			if err := resultOwner.Configure(resultbiz.Dependencies{OwnerRepositories: testResultOwnerRepositories(results, memory.NewArtifactRepository(), memory.NewResultWorkspaceDeletionRepository()), EventHub: hub}); err != nil {
				t.Fatal(err)
			}
			if err := resultOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = resultOwner.Stop(ctx) })
			startRuntimeReceiptTestOwner(t, hub)
			fileOwner := filemodule.New()
			if err := fileOwner.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
				t.Fatal(err)
			}
			if err := fileOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = fileOwner.Stop(ctx) })
			startRuntimeManagedValidationTestOwner(t, hub)
			candidateCalls := 0
			llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
			llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
				candidateCalls++
				command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
				result.Set(runtimeCandidateResultForTest(t, command, runtimeEmptyWorkspaceCreationCandidate(command)), nil)
			})
			t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })

			runtimeTestPersistBootstrappedOperationFixture(t, ctx, hub, "ws-v2", "run-v2", bootstrap)
			marker := "empty-workspace-create-receipt-" + phase
			crashRepository := &runtimeSaveCrashRepository{
				delegate: runtimeStateRepository{hub: hub}, after: after, marker: marker,
				match: func(aggregate runtime.RuntimeAggregate) bool {
					return len(aggregate.OperationReceipts) == 1 && len(aggregate.Facts.Evidence.Accepted) == 1
				},
			}
			crashing := newRuntimeGroundedTargetSequenceModule(t, hub, crashRepository)
			func() {
				defer func() {
					if recovered := recover(); recovered != marker {
						t.Fatalf("unexpected empty-workspace receipt crash marker: %v", recovered)
					}
				}()
				if err := crashing.scheduleRuntimeDrive(ctx, "ws-v2", "run-v2", marker); err != nil {
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

			restarted := newRuntimeGroundedTargetSequenceModule(t, hub, runtimeStateRepository{hub: hub})
			if err := restarted.scheduleRuntimeDrive(ctx, "ws-v2", "run-v2", "restart:"+phase); err != nil {
				t.Fatalf("restart after empty-workspace receipt crash: %v", err)
			}
			persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil || persisted.RuntimeState == nil || persisted.State != common.RunStateCompleted ||
				persisted.RuntimeState.Decision.State != runtimecontract.ConvergenceStateComplete ||
				persisted.RuntimeState.Facts.Resources.TargetSelection == nil || persisted.RuntimeState.Facts.Resources.TargetSelection.Disposition != runtimecontract.RuntimeTargetSelectionUnsupported ||
				len(persisted.RuntimeState.Facts.Resources.TargetSnapshots) != 0 || len(persisted.RuntimeState.OperationReceipts) != 1 ||
				len(persisted.RuntimeState.Facts.Evidence.Accepted) != 1 || candidateCalls != 2 {
				t.Fatalf("empty-workspace receipt restart authority=%+v candidateCalls=%d err=%v", persisted, candidateCalls, err)
			}
		})
	}
}

// TestRuntimeFileBackedBoundedWorkspaceScopeAllowsNewDeliverable verifies
// the owner-chain counterpart to empty-workspace creation. The bounded scope
// remains File feasibility inventory during SM-2; the operations request sees
// only the subsequently sealed prospective operation. Creation authority
// comes from the explicitly sealed target and the File owner's admissible
// variant fact.
func TestRuntimeFileBackedBoundedWorkspaceScopeAllowsNewDeliverable(t *testing.T) {
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
	bootstrap := runtimeGroundedTargetSequenceBootstrap()
	bootstrap.ConvergencePolicy.MaxCandidateContextItems = 4
	bootstrap.Definition.CapabilityContract = []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindManagedValidationSession}
	bootstrap.DefinitionFocuses[0].TargetSelectorRef = "file/workspace_deliverable"
	bootstrap.Requirements[0].RequiredCapabilities = []string{runtimecontract.StepKindFileWrite}
	runtimeTestDeclareProspectiveWorkspaceTarget(&bootstrap, "new-deliverable.txt")
	bootstrap.Requirements = append(bootstrap.Requirements, runtimecontract.RuntimeRequirement{
		Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		RequiredCapabilities: []string{runtimecontract.StepKindFileWrite}, Reason: "bounded workspace File creation inventory",
	})
	bootstrap = runtimeTestBootstrapWithReboundBindings(bootstrap)
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID, DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	startRuntimeReceiptTestOwner(t, hub)
	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	fileOwner := filemodule.New()
	if err := fileOwner.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := fileOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fileOwner.Stop(ctx) })
	startRuntimeManagedValidationTestOwner(t, hub)
	candidateCalls := 0
	initialSelection := runtimecontract.RuntimeTargetSelectionDisposition("")
	initialSelectionCount := 0
	initialSnapshots := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		candidateCalls++
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		if candidateCalls == 1 && command.CandidateContext.Resources.TargetSelection != nil {
			initialSelection = command.CandidateContext.Resources.TargetSelection.Disposition
			initialSelectionCount = command.CandidateContext.Resources.TargetSelection.CandidateCount
			initialSnapshots = len(command.CandidateContext.Resources.TargetSnapshots)
		}
		if command.Decision.State == runtimecontract.ConvergenceStateNeedValidation {
			result.Set(runtimeCandidateResultForTest(t, command, runtimeWorkspaceCreationCandidate(command, "new-deliverable.txt", "created\n")), nil)
			return
		}
		result.Set(runtimeCandidateResultForTest(t, command, runtimeWorkspaceCreationCandidate(command, "new-deliverable.txt", "created\n")), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })
	runtimeTestPersistBootstrappedOperationFixture(t, ctx, hub, "ws-v2", "run-v2", bootstrap)
	module := newRuntimeGroundedTargetSequenceModule(t, hub, runtimeStateRepository{hub: hub})
	driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntime, module.handleRuntimeDrive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntime) })

	completed, err := DriveRuntime(ctx, hub, orchestratorevents.DriveRuntimeCommand{WorkspaceID: "ws-v2", RunID: "run-v2"})
	if err != nil || completed.Terminal == nil || completed.State != runtimecontract.ConvergenceStateComplete || candidateCalls != 2 {
		t.Fatalf("bounded scope new-deliverable completion=%+v terminal=%+v candidateCalls=%d err=%v", completed, completed.Terminal, candidateCalls, err)
	}
	if content, err := os.ReadFile(filepath.Join(deliverable, "new-deliverable.txt")); err != nil || string(content) != "created\n" {
		t.Fatalf("canonical repair did not produce the owner-admitted target: content=%q err=%v", content, err)
	}
	persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.RuntimeState == nil || initialSelection != runtimecontract.RuntimeTargetSelectionCreate || initialSelectionCount != 0 || initialSnapshots != 0 ||
		persisted.RuntimeState.Facts.Resources.TargetSelection == nil || persisted.RuntimeState.Facts.Resources.TargetSelection.Disposition != runtimecontract.RuntimeTargetSelectionUnsupported ||
		persisted.RuntimeState.Facts.Resources.TargetSelection.CandidateCount != 0 || len(persisted.RuntimeState.Facts.Resources.TargetSnapshots) != 0 ||
		len(persisted.RuntimeState.OperationReceipts) != 1 || len(persisted.RuntimeState.Facts.Evidence.Accepted) != 1 || len(persisted.RuntimeState.Facts.Evidence.Rejected) != 0 ||
		persisted.RuntimeState.Facts.Resources.FailureTarget != nil || (persisted.RuntimeState.Facts.RepairDirective != nil && len(persisted.RuntimeState.Facts.RepairDirective.TargetRefs) != 0) {
		if persisted != nil && persisted.RuntimeState != nil {
			state := persisted.RuntimeState
			t.Fatalf("bounded scope new-deliverable initialSelection=%s initialCount=%d initialSnapshots=%d state=%s selection=%+v receipts=%d accepted=%d rejected=%d failureTarget=%+v repair=%+v err=%v", initialSelection, initialSelectionCount, initialSnapshots, state.Decision.State, state.Facts.Resources.TargetSelection, len(state.OperationReceipts), len(state.Facts.Evidence.Accepted), len(state.Facts.Evidence.Rejected), state.Facts.Resources.FailureTarget, state.Facts.RepairDirective, err)
		}
		t.Fatalf("bounded scope new-deliverable authority=%+v err=%v", persisted, err)
	}
}

// TestRuntimeFileBackedWorkspaceScopeExecutesTargetBoundShellOperation
// proves the P6 scope exception through the actual File and Shell owners. The
// shell command may modify go.sum because File observed it in the sealed scope;
// it does not receive authority to create a new path or to operate outside that
// inventory. P5's Shell receipt crash/restart matrix remains the replay proof
// for the resulting sealed operation.
func TestRuntimeFileBackedWorkspaceScopeExecutesTargetBoundShellOperation(t *testing.T) {
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
	bootstrap := runtimeGroundedTargetSequenceBootstrap()
	bootstrap.Definition.CapabilityContract = []string{runtimecontract.StepKindShellCommand, runtimecontract.RuntimeBlockKindManagedValidationSession}
	bootstrap.DefinitionFocuses[0].TargetSelectorRef = "file/workspace_deliverable"
	bootstrap.Requirements = []runtimecontract.RuntimeRequirement{
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleShell, CapabilityClass: runtimecontract.RuntimeCapabilityClassStateChange, RequiredCapabilities: []string{runtimecontract.StepKindShellCommand}, Reason: "Shell state-change capability"},
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleShell, CapabilityClass: runtimecontract.RuntimeCapabilityClassValidation, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, Reason: "Shell validation capability"},
		{Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleShell, RequiredCapabilities: []string{runtimecontract.StepKindShellCommand, runtimecontract.RuntimeBlockKindManagedValidationSession}, Reason: "Shell owner inventory"},
	}
	bootstrap = runtimeTestBootstrapWithReboundBindings(bootstrap)
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID, DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	startRuntimeReceiptTestOwner(t, hub)
	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	fileOwner := filemodule.New()
	if err := fileOwner.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := fileOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fileOwner.Stop(ctx) })
	shellOwner := shellmodule.New()
	if err := shellOwner.Configure(shellbiz.Dependencies{EventHub: hub, RuntimeAdapter: shellbiz.NewRuntimeAdapter()}); err != nil {
		t.Fatal(err)
	}
	if err := shellOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = shellOwner.Stop(ctx) })
	candidateCalls := 0
	candidateActions := make([]runtimecontract.ConvergenceState, 0, 4)
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		candidateCalls++
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		candidateActions = append(candidateActions, command.Decision.State)
		switch command.Decision.State {
		case runtimecontract.ConvergenceStateNeedStateChange:
			commandName, target, script := "sh", "go.sum", "printf 'updated\\n' > go.sum"
			stateChanging := true
			candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
				ID: "tidy-go-sum", Kind: runtimecontract.StepKindShellCommand, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{target},
				Parameters: []runtimecontract.OperationParameter{{Name: "command", String: &commandName}, {Name: "args", StringList: []string{"-c", script}}, {Name: "stateChanging", Boolean: &stateChanging}},
			}}}}}
			result.Set(runtimeCandidateResultForTest(t, command, candidate), nil)
		case runtimecontract.ConvergenceStateNeedValidation:
			result.Set(runtimeCandidateResultForTest(t, command, runtimeManagedShellValidationCandidate("validate-go-sum", "cwi-v2", "go.sum", "grounded-target-validation")), nil)
		case runtimecontract.ConvergenceStateNeedAssessment:
			refs := make([]string, 0, len(command.CandidateContext.Evidence.Accepted))
			for _, evidence := range command.CandidateContext.Evidence.Accepted {
				refs = append(refs, string(evidence.ID))
			}
			candidate := runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{FocusID: command.Decision.Focus.ID, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, Disposition: runtimecontract.AssessmentDispositionSatisfied, Summary: "target-bound shell mutation validated", Items: []runtimecontract.RuntimeAssessmentItemFact{{CoverageWorkItemID: "cwi-v2", Satisfied: true, EvidenceRefs: refs}}, EvidenceRefs: refs}}
			result.Set(runtimeCandidateResultForTest(t, command, candidate), nil)
		default:
			t.Fatalf("unexpected action %q", command.Decision.State)
		}
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })
	runtimeTestPersistBootstrappedOperationFixture(t, ctx, hub, "ws-v2", "run-v2", bootstrap)
	module := newRuntimeGroundedTargetSequenceModule(t, hub, runtimeStateRepository{hub: hub})
	driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntime, module.handleRuntimeDrive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntime) })

	completed, err := DriveRuntime(ctx, hub, orchestratorevents.DriveRuntimeCommand{WorkspaceID: "ws-v2", RunID: "run-v2"})
	// The Shell receipt changes a File-observed target. SM-2 therefore retires
	// the dependent File validation inventory before assessment; it must not
	// replay the Shell operation. One pre-refresh validation request may be
	// invalidated without an owner receipt, so the proof counts receipts rather
	// than treating request count as a side-effect count.
	if err != nil || completed.Terminal == nil || completed.State != runtimecontract.ConvergenceStateComplete || candidateCalls != 3 {
		t.Fatalf("target-bound Shell completion=%+v candidateCalls=%d actions=%v err=%v", completed, candidateCalls, candidateActions, err)
	}
	content, err := os.ReadFile(filepath.Join(deliverable, "go.sum"))
	if err != nil || string(content) != "updated\n" {
		t.Fatalf("target-bound Shell content=%q err=%v", string(content), err)
	}
	persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.RuntimeState == nil || persisted.RuntimeState.Decision.State != runtimecontract.ConvergenceStateComplete ||
		persisted.RuntimeState.Facts.Resources.TargetSelection == nil || persisted.RuntimeState.Facts.Resources.TargetSelection.Disposition != runtimecontract.RuntimeTargetSelectionScope ||
		len(persisted.RuntimeState.OperationReceipts) != 2 || len(persisted.RuntimeState.Facts.Evidence.Accepted) != 2 {
		state := persisted.RuntimeState
		if state == nil {
			t.Fatalf("target-bound Shell runtime state=%+v err=%v", persisted, err)
		}
		t.Fatalf("target-bound Shell state=%s selection=%+v receipts=%d accepted=%d err=%v", state.Decision.State, state.Facts.Resources.TargetSelection, len(state.OperationReceipts), len(state.Facts.Evidence.Accepted), err)
	}
	stateChangeCalls := 0
	for _, action := range candidateActions {
		if action == runtimecontract.ConvergenceStateNeedStateChange {
			stateChangeCalls++
		}
	}
	if stateChangeCalls != 1 {
		t.Fatalf("target-bound Shell mutation was replayed: candidate actions=%v", candidateActions)
	}
}

func runtimeEmptyWorkspaceCreationCandidate(command llmevents.RequestRuntimeCandidateCommand) runtimecontract.RuntimeCandidate {
	target := "implementation.txt"
	if selection := command.CandidateContext.Resources.TargetSelection; selection != nil && selection.Disposition == runtimecontract.RuntimeTargetSelectionCreate && selection.ProspectiveTargetRef != "" {
		target = selection.ProspectiveTargetRef
	}
	return runtimeWorkspaceCreationCandidate(command, target, "created\n")
}

func runtimeWorkspaceCreationCandidate(command llmevents.RequestRuntimeCandidateCommand, target, content string) runtimecontract.RuntimeCandidate {
	switch command.Decision.State {
	case runtimecontract.ConvergenceStateNeedStateChange:
		return runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
			ID: "create-deliverable", Kind: runtimecontract.StepKindFileWrite, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{target},
			Parameters: []runtimecontract.OperationParameter{{Name: "content", String: &content}},
		}}}}}
	case runtimecontract.ConvergenceStateNeedValidation:
		return runtimeManagedShellValidationCandidate("validate-created-deliverable", "cwi-v2", target, "grounded-target-validation")
	case runtimecontract.ConvergenceStateNeedAssessment:
		refs := make([]string, 0, len(command.CandidateContext.Evidence.Accepted))
		for _, evidence := range command.CandidateContext.Evidence.Accepted {
			refs = append(refs, string(evidence.ID))
		}
		return runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{FocusID: command.Decision.Focus.ID, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, Disposition: runtimecontract.AssessmentDispositionSatisfied, Summary: "created workspace deliverable validated", Items: []runtimecontract.RuntimeAssessmentItemFact{{CoverageWorkItemID: "cwi-v2", Satisfied: true, EvidenceRefs: refs}}, EvidenceRefs: refs}}
	default:
		return runtimecontract.RuntimeCandidate{}
	}
}

func runtimeTestDeclareProspectiveWorkspaceTarget(bootstrap *runtimecontract.RuntimeBootstrap, target string) {
	bootstrap.Activation.ExecutionContract.ProfileContracts = []runtimecontract.RuntimeProfileContract{{
		ID: "test-workspace-creation",
		EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{
			ID: "cwi-v2", Scope: "test", TargetRole: "deliverable",
			TargetDiscoverySources: []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest},
			TargetSelectorRef:      "file/workspace_deliverable", TargetSelectorOwnerRef: runtimecontract.RuntimeModuleFile,
			TargetSelectorSubjectRef: "deliverable", TargetSelectorCandidateRefs: []string{target}, RequireTargetGrounding: true,
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
			AcceptedStepKinds:     []string{runtimecontract.StepKindFileWrite},
		}},
	}}
	if len(bootstrap.DefinitionFocuses) != 0 {
		focus := &bootstrap.DefinitionFocuses[0]
		focus.TargetRole = "deliverable"
		focus.TargetDiscoverySources = []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest}
		focus.TargetSelectorRef = "file/workspace_deliverable"
		focus.TargetSelectorOwnerRef = runtimecontract.RuntimeModuleFile
		focus.TargetSelectorSubjectRef = "deliverable"
		focus.TargetSelectorCandidateRefs = []string{target}
		focus.RequireTargetGrounding = true
		focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
	}
	for index := range bootstrap.Slices {
		for _, coverageID := range bootstrap.Slices[index].CoverageWorkItemIDs {
			if coverageID == "cwi-v2" {
				bootstrap.Slices[index].RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
				break
			}
		}
	}
}

func runtimeTestDeclareExistingWorkspaceTarget(bootstrap *runtimecontract.RuntimeBootstrap, target string) {
	bootstrap.Activation.ExecutionContract.ProfileContracts = []runtimecontract.RuntimeProfileContract{{
		ID: "test-existing-workspace-target",
		EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{
			ID: "cwi-v2", Scope: "test", TargetRole: "deliverable", TargetRefs: []string{target},
			RequireTargetGrounding: true,
		}},
	}}
	bootstrap.DefinitionFocuses[0].TargetRole = "deliverable"
	bootstrap.DefinitionFocuses[0].TargetDiscoverySources = []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest}
	bootstrap.DefinitionFocuses[0].TargetSelectorRef = "file/profile_declared_candidates"
	bootstrap.DefinitionFocuses[0].TargetSelectorOwnerRef = "test-profile"
	bootstrap.DefinitionFocuses[0].TargetSelectorSubjectRef = "cwi-v2"
	bootstrap.DefinitionFocuses[0].TargetSelectorCandidateRefs = []string{target}
	bootstrap.DefinitionFocuses[0].RequireTargetGrounding = true
}

func testRuntimeFileBackedUnresolvedTargetSelectionFailsWithoutProvider(t *testing.T, bootstrap runtimecontract.RuntimeBootstrap, disposition runtimecontract.RuntimeTargetSelectionDisposition, files map[string]string) {
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
		DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive,
	}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	fileOwner := filemodule.New()
	if err := fileOwner.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := fileOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fileOwner.Stop(ctx) })
	startRuntimeManagedValidationTestOwner(t, hub)

	providerCalls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		providerCalls++
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		result.Set(runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity, ExpectedDocumentHash: runtimeExpectedDocumentHash(command.ExpectedDocument), Feedback: runtimeProviderFeedbackForTest(t, command, runtimecontract.RuntimeCandidateFailure{
			Class: runtimecontract.RuntimeCandidateFailureClassInternal, Reason: "provider must not be called for unresolved target selection",
		})}), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })
	governanceRequests := 0
	governanceObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleDecision, hub)
	governanceObserver.Subscribe(decisionevents.TopicOpenRuntimeGovernanceRequest, func(_ magicevent.Event, result magicevent.Result) {
		governanceRequests++
		result.Set(nil, nil)
	})
	t.Cleanup(func() {
		governanceObserver.Unsubscribe(decisionevents.TopicOpenRuntimeGovernanceRequest)
	})
	runtimeTestPersistBootstrappedOperationFixture(t, ctx, hub, "ws-v2", "run-v2", bootstrap)
	module := newRuntimeGroundedTargetSequenceModule(t, hub, runtimeStateRepository{hub: hub})
	driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntime, module.handleRuntimeDrive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntime) })

	completed, err := DriveRuntime(ctx, hub, orchestratorevents.DriveRuntimeCommand{WorkspaceID: "ws-v2", RunID: "run-v2"})
	if err != nil || completed.State != runtimecontract.ConvergenceStateFailed || completed.Terminal == nil || completed.Terminal.Code != "target_grounding_unresolved" {
		t.Fatalf("unresolved owner target result=%+v terminal=%+v err=%v", completed, completed.Terminal, err)
	}
	if providerCalls != 0 {
		t.Fatalf("unresolved target selection called provider %d times", providerCalls)
	}
	if governanceRequests != 0 {
		t.Fatalf("unresolved target selection opened %d governance requests", governanceRequests)
	}
	persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.RuntimeState == nil || persisted.RuntimeState.ActiveDispatch != nil || persisted.RuntimeState.Decision.State != runtimecontract.ConvergenceStateFailed || persisted.RuntimeState.Decision.Terminal == nil ||
		persisted.RuntimeState.Facts.Resources.TargetSelection == nil || persisted.RuntimeState.Facts.Resources.TargetSelection.Disposition != disposition ||
		persisted.RuntimeState.Facts.Resources.TargetSelection.CandidateCount != len(files) || len(persisted.RuntimeState.Facts.Resources.TargetSnapshots) != 0 ||
		len(persisted.RuntimeState.OperationReceipts) != 0 || len(persisted.RuntimeState.Facts.Evidence.Accepted) != 0 {
		t.Fatalf("unresolved target selection persisted wrong authority state=%+v err=%v", persisted, err)
	}
}

func runtimeGroundedTargetSequenceBootstrap() runtimecontract.RuntimeBootstrap {
	bootstrap := runtimeBootstrap()
	bootstrap.Definition.CapabilityContract = []string{runtimecontract.StepKindFileEdit, runtimecontract.RuntimeBlockKindManagedValidationSession}
	bootstrap.Activation.EvidencePolicies = []runtimecontract.RuntimeEvidencePolicy{{
		ID: "grounded-target-validation", AppliesTo: runtimecontract.StepKindShellCommand,
		CommandName: "sh", RecipeDeclared: true, RecipeArgs: []string{"-c", "printf 'WORKORCH_CHECK_PASSED=grounded-target-validation\\n'"},
	}}
	bootstrap.DefinitionFocuses = []runtimecontract.RuntimeDefinitionFocus{{
		ID: "focus-v2", SliceID: "slice-v2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation},
		TargetRole:            "deliverable", TargetDiscoverySources: []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest},
		TargetSelectorRef: "file/unique_workspace_entry", TargetSelectorOwnerRef: runtimecontract.RuntimeModuleFile, TargetSelectorSubjectRef: "deliverable",
		RequireTargetGrounding: true, RequireAssessment: true,
	}}
	bootstrap.Slices[0].RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}
	bootstrap.Requirements = []runtimecontract.RuntimeRequirement{
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleFile, CapabilityClass: runtimecontract.RuntimeCapabilityClassStateChange, RequiredCapabilities: []string{runtimecontract.StepKindFileEdit}},
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleShell, CapabilityClass: runtimecontract.RuntimeCapabilityClassValidation, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}},
		{Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleFile, RequiredCapabilities: []string{runtimecontract.StepKindFileEdit}, Reason: "File state-change inventory"},
		{Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleShell, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, Reason: "Shell validation inventory"},
	}
	return runtimeTestBootstrapWithBindings(bootstrap)
}

func runtimeFocusScopedSelectionBootstrap() runtimecontract.RuntimeBootstrap {
	bootstrap := runtimeBootstrap()
	bootstrap.Definition.CapabilityContract = []string{runtimecontract.StepKindFileEdit, runtimecontract.RuntimeBlockKindManagedValidationSession}
	bootstrap.Activation.EvidencePolicies = []runtimecontract.RuntimeEvidencePolicy{
		{ID: "focus-scoped-validation-module", AppliesTo: runtimecontract.StepKindShellCommand, CommandName: "sh", RecipeDeclared: true, RecipeArgs: []string{"-c", "test -f go.mod && printf 'WORKORCH_CHECK_PASSED=focus-scoped-validation-module\\n'"}},
		{ID: "focus-scoped-validation-manager", AppliesTo: runtimecontract.StepKindShellCommand, CommandName: "sh", RecipeDeclared: true, RecipeArgs: []string{"-c", "test -f internal/lsp/manager.go && printf 'WORKORCH_CHECK_PASSED=focus-scoped-validation-manager\\n'"}},
		{ID: "focus-scoped-validation-test", AppliesTo: runtimecontract.StepKindShellCommand, CommandName: "sh", RecipeDeclared: true, RecipeArgs: []string{"-c", "test -f internal/lsp/manager_test.go && printf 'WORKORCH_CHECK_PASSED=focus-scoped-validation-test\\n'"}},
	}
	bootstrap.DefinitionFocuses = []runtimecontract.RuntimeDefinitionFocus{
		{
			ID: "focus-module", SliceID: "slice-module", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-module"},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation},
			TargetRole:            "module_definition", TargetDiscoverySources: []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest},
			TargetSelectorRef: "file/profile_declared_candidates", TargetSelectorOwnerRef: "profile-bootstrap", TargetSelectorSubjectRef: "module",
			TargetSelectorCandidateRefs: []string{"go.mod"}, RequireTargetGrounding: true, RequireAssessment: true,
		},
		{
			ID: "focus-manager", SliceID: "slice-manager", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-manager"},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation},
			TargetRole:            "implementation", TargetDiscoverySources: []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest},
			TargetSelectorRef: "file/profile_declared_candidates", TargetSelectorOwnerRef: "profile-bootstrap", TargetSelectorSubjectRef: "manager",
			TargetSelectorCandidateRefs: []string{"internal/lsp/manager.go"}, RequireTargetGrounding: true, RequireAssessment: true,
		},
		{
			ID: "focus-test", SliceID: "slice-test", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-test"},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation},
			TargetRole:            "verification", TargetDiscoverySources: []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest},
			TargetSelectorRef: "file/profile_declared_candidates", TargetSelectorOwnerRef: "profile-bootstrap", TargetSelectorSubjectRef: "manager-test",
			TargetSelectorCandidateRefs: []string{"internal/lsp/manager_test.go"}, RequireTargetGrounding: true, RequireAssessment: true,
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
		{Kind: runtimecontract.PrerequisiteKindProviderCeiling, OwnerModuleID: runtimecontract.RuntimeModuleLLM, MinimumBytes: 64 * 1024, Reason: "semantic planning provider ceiling"},
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleFile, CapabilityClass: runtimecontract.RuntimeCapabilityClassStateChange, RequiredCapabilities: []string{runtimecontract.StepKindFileEdit}},
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleShell, CapabilityClass: runtimecontract.RuntimeCapabilityClassValidation, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}},
		{Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleFile, RequiredCapabilities: []string{runtimecontract.StepKindFileEdit}, Reason: "file owner inventory"},
		{Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleShell, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, Reason: "shell validation inventory"},
	}
	return runtimeTestBootstrapWithBindings(bootstrap)
}

func runtimeGroundedTargetValidationBootstrap() runtimecontract.RuntimeBootstrap {
	bootstrap := runtimeGroundedTargetSequenceBootstrap()
	bootstrap.DefinitionFocuses[0].RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	bootstrap.DefinitionFocuses[0].RequireDirectEvidence = true
	bootstrap.Slices[0].RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	return runtimeTestBootstrapWithBindings(bootstrap)
}

func runtimeGroundedTargetSequenceCandidate(command llmevents.RequestRuntimeCandidateCommand) (runtimecontract.RuntimeCandidate, error) {
	target := "implementation.txt"
	switch command.Decision.State {
	case runtimecontract.ConvergenceStateNeedStateChange:
		oldText, newText := "before\n", "after\n"
		return runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
			ID: "grounded-edit", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{target},
			Parameters: []runtimecontract.OperationParameter{{Name: "oldText", String: &oldText}, {Name: "newText", String: &newText}},
		}}}}}, nil
	case runtimecontract.ConvergenceStateNeedValidation:
		return runtimeManagedShellValidationCandidate("grounded-validate", "cwi-v2", target, "grounded-target-validation"), nil
	case runtimecontract.ConvergenceStateNeedAssessment:
		refs := make([]string, 0, len(command.CandidateContext.Evidence.Accepted))
		for _, evidence := range command.CandidateContext.Evidence.Accepted {
			refs = append(refs, string(evidence.ID))
		}
		return runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{
			FocusID: command.Decision.Focus.ID, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, Disposition: runtimecontract.AssessmentDispositionSatisfied,
			Summary: "grounded target evidence satisfied",
			Items:   []runtimecontract.RuntimeAssessmentItemFact{{CoverageWorkItemID: "cwi-v2", Satisfied: true, EvidenceRefs: refs}}, EvidenceRefs: refs,
		}}, nil
	default:
		return runtimecontract.RuntimeCandidate{}, fmt.Errorf("unexpected grounded sequence action %q", command.Decision.State)
	}
}

func newRuntimeGroundedTargetSequenceModule(t *testing.T, hub magicevent.Hub, runtimeRepository runtime.RuntimeStateRepository, contents ...repository.ArtifactContinuationRepository) *Biz {
	t.Helper()
	ensureRuntimeCandidatePreflightTestResponder(hub)
	candidateRequestContents := runtimeCandidateRequestContentTestStore(hub)
	if len(contents) > 0 && contents[0] != nil {
		candidateRequestContents = contents[0]
	}
	module := New()
	if err := module.Configure(Dependencies{
		Evidence:   memory.NewOrchestratorOwnerRepositorySet(),
		CommandHub: runtimeTestHubWithRunRetirementFallback(hub),
	}); err != nil {
		t.Fatal(err)
	}
	module.evidence.ArtifactContinuations = candidateRequestContents
	if err := module.ConfigureRuntimeRepository(runtimeRepository); err != nil {
		t.Fatal(err)
	}
	return module
}

func TestRuntimeFileBackedGitCapabilityStateSequenceMatrix(t *testing.T) {
	testCases := []struct {
		name               string
		prepare            func(*testing.T, string)
		expectedCandidates [][]string
		expectedFinalRefs  []string
		expectedCount      int
	}{
		{
			name:               "repository_absent",
			expectedCandidates: [][]string{{"create-commit", "repo-init", "stage-changes"}},
			expectedCount:      3,
			prepare: func(t *testing.T, deliverable string) {
				t.Helper()
				writeRuntimeGitFixtureFile(t, deliverable)
			},
		},
		{
			name:               "initialized_unstaged",
			expectedCandidates: [][]string{{"create-commit", "stage-changes"}},
			expectedCount:      2,
			prepare: func(t *testing.T, deliverable string) {
				t.Helper()
				runRuntimeGit(t, deliverable, "init", "-q")
				runRuntimeGit(t, deliverable, "config", "user.name", "WorkOrch Test")
				runRuntimeGit(t, deliverable, "config", "user.email", "workorch@example.test")
				writeRuntimeGitFixtureFile(t, deliverable)
			},
		},
		{
			name: "initialized_staged", expectedCandidates: [][]string{{"create-commit"}}, expectedCount: 1,
			prepare: func(t *testing.T, deliverable string) {
				t.Helper()
				runRuntimeGit(t, deliverable, "init", "-q")
				runRuntimeGit(t, deliverable, "config", "user.name", "WorkOrch Test")
				runRuntimeGit(t, deliverable, "config", "user.email", "workorch@example.test")
				writeRuntimeGitFixtureFile(t, deliverable)
				runRuntimeGit(t, deliverable, "add", "target.txt")
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

			bootstrap := runtimeGitCapabilityStateBootstrap()
			runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
			if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2", DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive}); err != nil {
				t.Fatal(err)
			}
			runOwner := runmodule.New()
			if err := runOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: hub}); err != nil {
				t.Fatal(err)
			}
			if err := runOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = runOwner.Stop(ctx) })
			startRuntimeReceiptTestOwner(t, hub)

			workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
			workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
				result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
			})
			t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
			startRuntimeFileManifestOwner(t, ctx, hub)

			receipts, err := gitbiz.NewCapabilityStateReceiptStore(filepath.Join(root, "git-owner-data"))
			if err != nil {
				t.Fatal(err)
			}
			gitOwner := gitmodule.New()
			if err := gitOwner.Configure(gitbiz.Dependencies{EventHub: hub, RuntimeAdapter: gitbiz.NewRuntimeAdapter(), CapabilityStateReceipts: receipts}); err != nil {
				t.Fatal(err)
			}
			if err := gitOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = gitOwner.Stop(ctx) })

			candidateCalls := 0
			assessmentCalls := 0
			candidateRefsMatch := true
			llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
			llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
				command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
				if command.Decision.State == runtimecontract.ConvergenceStateNeedAssessment {
					assessmentCalls++
					result.Set(runtimeCandidateResultForTest(t, command, runtimeGitCandidateForDecision(command)), nil)
					return
				}
				candidateCalls++
				refs := requiredOperationRefs(command.Decision.RequiredOutput.RequiredOperationSequence)
				if candidateCalls > len(testCase.expectedCandidates) || !equalStrings(refs, testCase.expectedCandidates[candidateCalls-1]) {
					candidateRefsMatch = false
				}
				result.Set(runtimeCandidateResultForTest(t, command, runtimeGitCandidateForDecision(command)), nil)
			})
			t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })

			runtimeTestPersistBootstrappedOperationFixture(t, ctx, hub, "ws-v2", "run-v2", bootstrap)
			module := newRuntimeDriveTestModule(t, hub)
			driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
			driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntime, module.handleRuntimeDrive)
			t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntime) })
			completed, err := DriveRuntime(ctx, hub, orchestratorevents.DriveRuntimeCommand{WorkspaceID: "ws-v2", RunID: "run-v2"})
			if err != nil {
				t.Fatal(err)
			}
			if completed.State != runtimecontract.ConvergenceStateComplete || candidateCalls != len(testCase.expectedCandidates) || assessmentCalls != 1 || !candidateRefsMatch {
				persisted, persistErr := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
				t.Fatalf("completed=%+v terminal=%+v candidateCalls=%d assessmentCalls=%d candidateRefsMatch=%t persisted=%+v persistErr=%v", completed, completed.Terminal, candidateCalls, assessmentCalls, candidateRefsMatch, persisted.RuntimeState, persistErr)
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
			if output := runtimeGitOutput(t, deliverable, "rev-list", "--count", "HEAD"); strings.TrimSpace(output) != "1" {
				t.Fatalf("commit count = %q, want 1", output)
			}
		})
	}
}

func TestRuntimeFileBackedGitRestartAfterInitEvidenceCommitContinuesSequence(t *testing.T) {
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
	writeRuntimeGitFixtureFile(t, deliverable)

	bootstrap := runtimeGitCapabilityStateBootstrap()
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2", DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	startRuntimeReceiptTestOwner(t, hub)
	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	startRuntimeFileManifestOwner(t, ctx, hub)
	receipts, err := gitbiz.NewCapabilityStateReceiptStore(filepath.Join(root, "git-owner-data"))
	if err != nil {
		t.Fatal(err)
	}
	gitOwner := gitmodule.New()
	if err := gitOwner.Configure(gitbiz.Dependencies{EventHub: hub, RuntimeAdapter: gitbiz.NewRuntimeAdapter(), CapabilityStateReceipts: receipts}); err != nil {
		t.Fatal(err)
	}
	if err := gitOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = gitOwner.Stop(ctx) })

	module := newRuntimeDriveTestModule(t, hub)
	run, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err := module.initializeRuntimeFromRun(ctx, run)
	if err != nil {
		t.Fatal(err)
	}
	aggregate = runtimeTestPersistSealedSemanticAuthority(t, ctx, runtimeStateRepository{hub: hub}, aggregate)
	aggregate, err = module.prepareRuntimePrerequisites(ctx, aggregate)
	if err != nil {
		t.Fatal(err)
	}
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(aggregate.Definition, aggregate.Facts.Activation.ExecutionContract)
	candidate := runtimeGitStateCandidate([]string{"create-commit", "repo-init", "stage-changes"})
	initialLLM := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	initialLLM.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		result.Set(runtimeCandidateResultForTest(t, command, candidate), nil)
	})
	aggregate, err = module.requestAndSealRuntimeCandidate(ctx, aggregate, prefix, 0)
	initialLLM.Unsubscribe(llmevents.TopicRequestRuntimeCandidate)
	if err != nil || aggregate.ActiveDispatch == nil {
		t.Fatal(err)
	}
	baseRepository := runtimeStateRepository{hub: hub}
	crashRepository := &runtimeSaveCrashRepository{
		delegate: baseRepository, after: true, marker: "git-init-evidence",
		match: func(current runtime.RuntimeAggregate) bool {
			if current.ActiveDispatch == nil || current.EvidenceSequence != 1 || len(current.OperationReceipts) != 1 ||
				current.Decision.State != runtimecontract.ConvergenceStateNeedStateChange || len(current.CompletedDispatches) != 0 ||
				len(current.ActiveDispatch.CompletedOperationIDs) != 1 {
				return false
			}
			return current.ActiveDispatch.CompletedOperationIDs[0] == "git-repo-init"
		},
	}
	crashingController, err := runtime.NewRuntimeController(crashRepository)
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "git-init-evidence" {
				t.Fatalf("unexpected crash marker: %v", recovered)
			}
		}()
		_, _ = runtimeTestControllerExecuteDispatch(crashingController, ctx, "ws-v2", "run-v2", eventRuntimeOperationExecutor{hub: hub})
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
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		if command.Decision.State == runtimecontract.ConvergenceStateNeedStateChange {
			candidateCalls++
		}
		result.Set(runtimeCandidateResultForTest(t, command, runtimeGitCandidateForDecision(command)), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })
	restarted := newRuntimeDriveTestModule(t, hub)
	driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntime, restarted.handleRuntimeDrive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntime) })
	completed, err := DriveRuntime(ctx, hub, orchestratorevents.DriveRuntimeCommand{WorkspaceID: "ws-v2", RunID: "run-v2"})
	if err != nil || completed.State != runtimecontract.ConvergenceStateComplete || candidateCalls != 0 {
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
	if countInit != 1 || strings.TrimSpace(runtimeGitOutput(t, deliverable, "rev-list", "--count", "HEAD")) != "1" {
		t.Fatalf("init count=%d", countInit)
	}
}

func TestRuntimeFileBackedGitQueryReceiptCrashBeforePrerequisiteCommitReusesSnapshot(t *testing.T) {
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
	writeRuntimeGitFixtureFile(t, deliverable)
	bootstrap := runtimeGitCapabilityStateBootstrap()
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2", DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	startRuntimeReceiptTestOwner(t, hub)
	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	startRuntimeFileManifestOwner(t, ctx, hub)
	storeRoot := filepath.Join(root, "git-owner-data")
	store, err := gitbiz.NewCapabilityStateReceiptStore(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	gitOwner := gitmodule.New()
	if err := gitOwner.Configure(gitbiz.Dependencies{EventHub: hub, RuntimeAdapter: gitbiz.NewRuntimeAdapter(), CapabilityStateReceipts: store}); err != nil {
		t.Fatal(err)
	}
	if err := gitOwner.Start(); err != nil {
		t.Fatal(err)
	}

	baseRepository := runtimeStateRepository{hub: hub}
	crashRepository := &runtimeSaveCrashRepository{
		delegate: baseRepository, marker: "git-query-before-prerequisite-commit",
		match: func(current runtime.RuntimeAggregate) bool {
			return current.Decision.State == runtimecontract.ConvergenceStateNeedPrerequisite &&
				len(current.Facts.Resources.CapabilityStates) == 1 &&
				len(current.Facts.Resources.OperationAdmissibilities) == 0
		},
	}
	crashingBiz := newRuntimeTestOwner()
	if err := crashingBiz.ConfigureRuntimeRepository(crashRepository); err != nil {
		t.Fatal(err)
	}
	crashingModule := newTestBiz(crashingBiz, hub, nil)
	run, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err := crashingBiz.initializeRuntimeFromRun(ctx, run)
	if err != nil {
		t.Fatal(err)
	}
	aggregate = runtimeTestPersistSealedSemanticAuthority(t, ctx, crashRepository, aggregate)
	func() {
		defer func() {
			if recovered := recover(); recovered != "git-query-before-prerequisite-commit" {
				t.Fatalf("unexpected crash marker: %v", recovered)
			}
		}()
		_, _ = crashingModule.prepareRuntimePrerequisites(ctx, aggregate)
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
	acquisitionKey := stableRuntimeCapabilityStateAcquisitionKey("ws-v2", "run-v2", prerequisiteID, 0)
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
	if err := restartedRunOwner.Configure(runbiz.Dependencies{Runs: restartedRuns, EventHub: restartedHub}); err != nil {
		t.Fatal(err)
	}
	if err := restartedRunOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedRunOwner.Stop(ctx) })
	startRuntimeReceiptTestOwner(t, restartedHub)
	restartedWorkspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, restartedHub)
	restartedWorkspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
	})
	t.Cleanup(func() { restartedWorkspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	startRuntimeFileManifestOwner(t, ctx, restartedHub)
	reopenedStore, err := gitbiz.NewCapabilityStateReceiptStore(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	restartedGitOwner := gitmodule.New()
	if err := restartedGitOwner.Configure(gitbiz.Dependencies{EventHub: restartedHub, RuntimeAdapter: gitbiz.NewRuntimeAdapter(), CapabilityStateReceipts: reopenedStore}); err != nil {
		t.Fatal(err)
	}
	if err := restartedGitOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedGitOwner.Stop(ctx) })

	candidateCalls := 0
	firstCandidateReusedQueryReceipt := false
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, restartedHub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		stateChange := command.Decision.State == runtimecontract.ConvergenceStateNeedStateChange
		if stateChange {
			candidateCalls++
		}
		result.Set(runtimeCandidateResultForTest(t, command, runtimeGitCandidateForDecision(command)), nil)
		if stateChange && candidateCalls == 1 && (len(command.CandidateContext.Resources.CapabilityStates) != 1 ||
			command.CandidateContext.Resources.CapabilityStates[0].SnapshotID != queried.SnapshotID ||
			command.CandidateContext.Resources.CapabilityStates[0].AcquisitionKey != queried.AcquisitionKey ||
			command.CandidateContext.Resources.CapabilityStates[0].QueryReceiptRef != queried.QueryReceiptRef) {
			t.Errorf("restart did not reuse durable query receipt in first candidate: %+v", command.CandidateContext.Resources.CapabilityStates)
			return
		}
		if stateChange && candidateCalls == 1 {
			firstCandidateReusedQueryReceipt = true
		}
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })
	restarted := newRuntimeDriveTestModule(t, restartedHub)
	driveObserver := magicevent.NewSimpleObserver(ModuleID, restartedHub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntime, restarted.handleRuntimeDrive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntime) })
	completed, err := DriveRuntime(ctx, restartedHub, orchestratorevents.DriveRuntimeCommand{WorkspaceID: "ws-v2", RunID: "run-v2"})
	if err != nil || completed.State != runtimecontract.ConvergenceStateComplete || candidateCalls != 1 || !firstCandidateReusedQueryReceipt {
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
func TestRuntimeFileBackedManifestCommitResponseLossRestartReusesCommittedManifest(t *testing.T) {
	runRuntimeFileBackedManifestCommitCrashRestart(t, true)
}

func TestRuntimeFileBackedManifestCommitBeforeCASRestartReacquiresManifest(t *testing.T) {
	runRuntimeFileBackedManifestCommitCrashRestart(t, false)
}

func runRuntimeFileBackedManifestCommitCrashRestart(t *testing.T, crashAfterCAS bool) {
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
	bootstrap := runtimeBootstrap()
	bootstrap.Requirements = append([]runtimecontract.RuntimeRequirement{
		{Kind: runtimecontract.PrerequisiteKindManifest, OwnerModuleID: runtimecontract.RuntimeModuleFile, Reason: "file owner manifest"},
	}, bootstrap.Requirements...)
	runs := filerepository.NewRunRepository(runsPath)
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2", DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive,
	}); err != nil {
		t.Fatal(err)
	}

	startProcess := func(t *testing.T) (magicevent.Hub, *filerepository.RunRepository, *runmodule.Module, *filemodule.Module, magicevent.SimpleObserver) {
		t.Helper()
		hub := magicevent.NewHub(16)
		processRuns := filerepository.NewRunRepository(runsPath)
		runOwner := runmodule.New()
		if err := runOwner.Configure(runbiz.Dependencies{Runs: processRuns, EventHub: hub}); err != nil {
			hub.Terminate(ctx)
			t.Fatal(err)
		}
		if err := runOwner.Start(); err != nil {
			hub.Terminate(ctx)
			t.Fatal(err)
		}
		workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
		workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
			result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable}}, nil)
		})
		fileOwner := filemodule.New()
		if err := fileOwner.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
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
	baseRepository := runtimeStateRepository{hub: hub}
	crashMarker := "manifest-commit-before-cas"
	if crashAfterCAS {
		crashMarker = "manifest-response-loss"
	}
	crashRepository := &runtimeSaveCrashRepository{
		delegate: baseRepository, after: crashAfterCAS, marker: crashMarker,
		match: func(current runtime.RuntimeAggregate) bool {
			return current.Facts.Resources.Manifest != nil && current.Facts.Resources.Manifest.Revision != ""
		},
	}
	crashingBiz := newRuntimeTestOwner()
	if err := crashingBiz.ConfigureRuntimeRepository(crashRepository); err != nil {
		t.Fatal(err)
	}
	crashingModule := newTestBiz(crashingBiz, hub, nil)
	run, err := processRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err := crashingBiz.initializeRuntimeFromRun(ctx, run)
	if err != nil {
		t.Fatal(err)
	}
	aggregate = runtimeTestPersistSealedSemanticAuthority(t, ctx, crashRepository, aggregate)
	func() {
		defer func() {
			if recovered := recover(); recovered != crashMarker {
				t.Fatalf("unexpected manifest response-loss marker: %v", recovered)
			}
		}()
		_, _ = crashingModule.prepareRuntimePrerequisites(ctx, aggregate)
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
	restarted := newRuntimeDriveTestModule(t, hub)
	loaded, err := restarted.loadRuntime(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := restarted.prepareRuntimePrerequisites(ctx, loaded)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Facts.Resources.Manifest == nil || resumed.Facts.Resources.Manifest.Revision == "" ||
		len(resumed.Facts.Prerequisites.Pending) != 0 || resumed.Decision.State != runtimecontract.ConvergenceStateNeedStateChange ||
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
func TestRuntimeFileBackedProviderCeilingCommitCrashRestart(t *testing.T) {
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
			bootstrap := runtimeBootstrap()
			bootstrap.Activation.ProviderSelection.Target = runtimecontract.ProviderModelTarget{ProviderID: runtimecontract.BuiltinAIProxyProviderID, ModelID: "model-1"}
			bootstrap.Requirements = []runtimecontract.RuntimeRequirement{{
				Kind: runtimecontract.PrerequisiteKindProviderCeiling, OwnerModuleID: llmmodule.ModuleID,
				MinimumBytes: 32 * 1024, Reason: "sealed provider ceiling",
			}}
			if err := filerepository.NewRunRepository(runsPath).Create(ctx, &runmodel.Run{
				ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
				DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive,
			}); err != nil {
				t.Fatal(err)
			}

			startProcess := func(t *testing.T) (magicevent.Hub, *filerepository.RunRepository, *runmodule.Module, *llmmodule.Module, magicevent.SimpleObserver, magicevent.SimpleObserver, *int) {
				t.Helper()
				// The module must resolve configuration from a freshly opened source on
				// every simulated process start; no DefaultLLMSelection or process-global
				// test configuration is reused as prerequisite authority.
				if err := workorchconfig.Close(); err != nil {
					t.Fatal(err)
				}
				hub := magicevent.NewHub(16)
				processRuns := filerepository.NewRunRepository(runsPath)
				runOwner := runmodule.New()
				if err := runOwner.Configure(runbiz.Dependencies{Runs: processRuns, EventHub: hub}); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := runOwner.Start(); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
				workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
					result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable}}, nil)
				})
				llmOwner := llmmodule.New()
				if err := llmOwner.Configure(llmbiz.Dependencies{EventHub: hub}); err != nil {
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
				ceilingObserver.Subscribe(llmevents.TopicAcquireProviderCeiling, func(event magicevent.Event, _ magicevent.Result) {
					if _, ok := event.Data().(llmevents.AcquireProviderCeilingCommand); !ok {
						t.Errorf("provider ceiling spy received %T", event.Data())
						return
					}
					*ceilingCalls++
				})
				return hub, processRuns, runOwner, llmOwner, workspaceObserver, ceilingObserver, ceilingCalls
			}
			stopProcess := func(hub magicevent.Hub, runOwner *runmodule.Module, llmOwner *llmmodule.Module, workspaceObserver, ceilingObserver magicevent.SimpleObserver) {
				if ceilingObserver != nil {
					ceilingObserver.Unsubscribe(llmevents.TopicAcquireProviderCeiling)
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
			baseRepository := runtimeStateRepository{hub: hub}
			marker := "provider-ceiling-before-cas"
			if crashAfterCAS {
				marker = "provider-ceiling-after-cas"
			}
			crashRepository := &runtimeSaveCrashRepository{
				delegate: baseRepository, after: crashAfterCAS, marker: marker,
				match: func(current runtime.RuntimeAggregate) bool {
					return current.Facts.Resources.ProviderCeiling != nil && current.Facts.Resources.ProviderCeiling.ConfigurationRevision != ""
				},
			}
			crashingBiz := newRuntimeTestOwner()
			if err := crashingBiz.ConfigureRuntimeRepository(crashRepository); err != nil {
				t.Fatal(err)
			}
			crashingModule := newTestBiz(crashingBiz, hub, nil)
			run, err := processRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			aggregate, err := crashingBiz.initializeRuntimeFromRun(ctx, run)
			if err != nil {
				t.Fatal(err)
			}
			aggregate = runtimeTestPersistSealedSemanticAuthorityWithoutProviderCeiling(t, ctx, crashRepository, aggregate)
			func() {
				defer func() {
					if recovered := recover(); recovered != marker {
						t.Fatalf("unexpected provider ceiling crash marker: %v", recovered)
					}
				}()
				_, _ = crashingModule.prepareRuntimePrerequisites(ctx, aggregate)
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
			restarted := newRuntimeDriveTestModule(t, hub)
			loaded, err := restarted.loadRuntime(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			resumed, err := restarted.prepareRuntimePrerequisites(ctx, loaded)
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
				if !got.Selection.Same(committedCeiling.Selection) || got.CapabilityRevision != committedCeiling.CapabilityRevision ||
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
func TestRuntimeFileBackedFileCapabilityCommitCrashRestart(t *testing.T) {
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
			bootstrap := runtimeBootstrap()
			bootstrap.Requirements = []runtimecontract.RuntimeRequirement{{
				Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: filemodule.ModuleID,
				CapabilityClass:      runtimecontract.RuntimeCapabilityClassStateChange,
				RequiredCapabilities: []string{runtimecontract.StepKindFileEdit}, Reason: "sealed file capability",
			}}
			if err := filerepository.NewRunRepository(runsPath).Create(ctx, &runmodel.Run{
				ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
				DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive,
			}); err != nil {
				t.Fatal(err)
			}

			startProcess := func(t *testing.T) (magicevent.Hub, *filerepository.RunRepository, *runmodule.Module, *filemodule.Module, magicevent.SimpleObserver, magicevent.SimpleObserver, *int) {
				t.Helper()
				hub := magicevent.NewHub(16)
				processRuns := filerepository.NewRunRepository(runsPath)
				runOwner := runmodule.New()
				if err := runOwner.Configure(runbiz.Dependencies{Runs: processRuns, EventHub: hub}); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := runOwner.Start(); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
				workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
					result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable}}, nil)
				})
				fileOwner := filemodule.New()
				if err := fileOwner.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
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
				capabilityObserver.Subscribe(fileevents.TopicAcquireRuntimeCapabilities, func(event magicevent.Event, _ magicevent.Result) {
					command, ok := event.Data().(fileevents.AcquireRuntimeCapabilitiesCommand)
					if !ok || command.OwnerModuleID != filemodule.ModuleID {
						t.Errorf("file capability spy received %T / %+v", event.Data(), event.Data())
						return
					}
					*capabilityCalls++
				})
				return hub, processRuns, runOwner, fileOwner, workspaceObserver, capabilityObserver, capabilityCalls
			}
			stopProcess := func(hub magicevent.Hub, runOwner *runmodule.Module, fileOwner *filemodule.Module, workspaceObserver, capabilityObserver magicevent.SimpleObserver) {
				if capabilityObserver != nil {
					capabilityObserver.Unsubscribe(fileevents.TopicAcquireRuntimeCapabilities)
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
			baseRepository := runtimeStateRepository{hub: hub}
			marker := "file-capability-before-cas"
			if crashAfterCAS {
				marker = "file-capability-after-cas"
			}
			crashRepository := &runtimeSaveCrashRepository{
				delegate: baseRepository, after: crashAfterCAS, marker: marker,
				match: func(current runtime.RuntimeAggregate) bool {
					return len(current.Facts.Capabilities.StateChangeKinds) == 1 && current.Facts.Capabilities.StateChangeKinds[0] == runtimecontract.StepKindFileEdit
				},
			}
			crashingBiz := newRuntimeTestOwner()
			if err := crashingBiz.ConfigureRuntimeRepository(crashRepository); err != nil {
				t.Fatal(err)
			}
			crashingModule := newTestBiz(crashingBiz, hub, nil)
			run, err := processRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			aggregate, err := crashingBiz.initializeRuntimeFromRun(ctx, run)
			if err != nil {
				t.Fatal(err)
			}
			aggregate = runtimeTestPersistSealedSemanticAuthority(t, ctx, crashRepository, aggregate)
			func() {
				defer func() {
					if recovered := recover(); recovered != marker {
						t.Fatalf("unexpected file capability crash marker: %v", recovered)
					}
				}()
				_, _ = crashingModule.prepareRuntimePrerequisites(ctx, aggregate)
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
			restarted := newRuntimeDriveTestModule(t, hub)
			loaded, err := restarted.loadRuntime(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			resumed, err := restarted.prepareRuntimePrerequisites(ctx, loaded)
			if err != nil {
				t.Fatal(err)
			}
			if len(resumed.Facts.Capabilities.StateChangeKinds) != 1 || resumed.Facts.Capabilities.StateChangeKinds[0] != runtimecontract.StepKindFileEdit ||
				len(resumed.Facts.Prerequisites.Pending) != 0 || resumed.Decision.State != runtimecontract.ConvergenceStateNeedStateChange ||
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
func TestRuntimeFileBackedGitCapabilityCommitCrashRestart(t *testing.T) {
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
			bootstrap := runtimeBootstrap()
			bootstrap.Definition.CapabilityContract = []string{gitmodule.StepKindWrite}
			bootstrap.Requirements = []runtimecontract.RuntimeRequirement{{
				Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: gitmodule.ModuleID,
				CapabilityClass: runtimecontract.RuntimeCapabilityClassStateChange, RequiredCapabilities: []string{gitmodule.StepKindWrite}, Reason: "sealed git capability",
			}}
			bootstrap = runtimeTestBootstrapWithReboundBindings(bootstrap)
			if err := filerepository.NewRunRepository(runsPath).Create(ctx, &runmodel.Run{
				ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
				DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive,
			}); err != nil {
				t.Fatal(err)
			}

			startProcess := func(t *testing.T) (magicevent.Hub, *filerepository.RunRepository, *runmodule.Module, *gitmodule.Module, magicevent.SimpleObserver, magicevent.SimpleObserver, *int) {
				t.Helper()
				hub := magicevent.NewHub(16)
				processRuns := filerepository.NewRunRepository(runsPath)
				runOwner := runmodule.New()
				if err := runOwner.Configure(runbiz.Dependencies{Runs: processRuns, EventHub: hub}); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := runOwner.Start(); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
				workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
					result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDirectory}}, nil)
				})
				store, err := gitbiz.NewCapabilityStateReceiptStore(filepath.Join(root, "git-capability-receipts"))
				if err != nil {
					workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
					_ = runOwner.Stop(ctx)
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				gitOwner := gitmodule.New()
				if err := gitOwner.Configure(gitbiz.Dependencies{EventHub: hub, RuntimeAdapter: gitbiz.NewRuntimeAdapter(), CapabilityStateReceipts: store}); err != nil {
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
				spy.Subscribe(gitevents.TopicAcquireRuntimeCapabilities, func(event magicevent.Event, _ magicevent.Result) {
					command, ok := event.Data().(gitevents.AcquireRuntimeCapabilitiesCommand)
					if !ok || command.OwnerModuleID != gitmodule.ModuleID || command.CapabilityClass != runtimecontract.RuntimeCapabilityClassStateChange || len(command.RequiredKinds) != 1 || command.RequiredKinds[0] != gitmodule.StepKindWrite {
						t.Errorf("git capability spy received %T / %+v", event.Data(), event.Data())
						return
					}
					*calls++
				})
				return hub, processRuns, runOwner, gitOwner, workspaceObserver, spy, calls
			}
			stopProcess := func(hub magicevent.Hub, runOwner *runmodule.Module, gitOwner *gitmodule.Module, workspaceObserver, spy magicevent.SimpleObserver) {
				if spy != nil {
					spy.Unsubscribe(gitevents.TopicAcquireRuntimeCapabilities)
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
			baseRepository := runtimeStateRepository{hub: hub}
			marker := "git-capability-before-cas"
			if crashAfterCAS {
				marker = "git-capability-after-cas"
			}
			crashRepository := &runtimeSaveCrashRepository{delegate: baseRepository, after: crashAfterCAS, marker: marker, match: func(current runtime.RuntimeAggregate) bool {
				return len(current.Facts.Capabilities.StateChangeKinds) == 1 && current.Facts.Capabilities.StateChangeKinds[0] == gitmodule.StepKindWrite
			}}
			crashingOwner := newRuntimeTestOwner()
			if err := crashingOwner.ConfigureRuntimeRepository(crashRepository); err != nil {
				t.Fatal(err)
			}
			crashing := newTestBiz(crashingOwner, hub, nil)
			run, err := processRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			aggregate, err := crashingOwner.initializeRuntimeFromRun(ctx, run)
			if err != nil {
				t.Fatal(err)
			}
			aggregate = runtimeTestPersistSealedSemanticAuthority(t, ctx, crashRepository, aggregate)
			func() {
				defer func() {
					if recovered := recover(); recovered != marker {
						t.Fatalf("unexpected git capability crash marker: %v", recovered)
					}
				}()
				_, _ = crashing.prepareRuntimePrerequisites(ctx, aggregate)
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
			restarted := newRuntimeDriveTestModule(t, hub)
			loaded, err := restarted.loadRuntime(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			resumed, err := restarted.prepareRuntimePrerequisites(ctx, loaded)
			if err != nil {
				t.Fatal(err)
			}
			if len(resumed.Facts.Capabilities.StateChangeKinds) != 1 || resumed.Facts.Capabilities.StateChangeKinds[0] != gitmodule.StepKindWrite ||
				len(resumed.Facts.Prerequisites.Pending) != 0 || resumed.Decision.State != runtimecontract.ConvergenceStateNeedStateChange ||
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
func TestRuntimeFileBackedShellCapabilityCommitCrashRestart(t *testing.T) {
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
			bootstrap := runtimeBootstrap()
			bootstrap.Requirements = []runtimecontract.RuntimeRequirement{{
				Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: shellmodule.ModuleID,
				CapabilityClass: runtimecontract.RuntimeCapabilityClassValidation, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, Reason: "sealed shell validation capability",
			}}
			if err := filerepository.NewRunRepository(runsPath).Create(ctx, &runmodel.Run{
				ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
				DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive,
			}); err != nil {
				t.Fatal(err)
			}

			startProcess := func(t *testing.T) (magicevent.Hub, *filerepository.RunRepository, *runmodule.Module, *shellmodule.Module, magicevent.SimpleObserver, magicevent.SimpleObserver, *int) {
				t.Helper()
				hub := magicevent.NewHub(16)
				processRuns := filerepository.NewRunRepository(runsPath)
				runOwner := runmodule.New()
				if err := runOwner.Configure(runbiz.Dependencies{Runs: processRuns, EventHub: hub}); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := runOwner.Start(); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
				workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
					result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDirectory}}, nil)
				})
				shellOwner := shellmodule.New()
				if err := shellOwner.Configure(shellbiz.Dependencies{EventHub: hub, RuntimeAdapter: shellbiz.NewRuntimeAdapter()}); err != nil {
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
				spy.Subscribe(shellevents.TopicAcquireRuntimeCapabilities, func(event magicevent.Event, _ magicevent.Result) {
					command, ok := event.Data().(shellevents.AcquireRuntimeCapabilitiesCommand)
					if !ok || command.OwnerModuleID != shellmodule.ModuleID || command.CapabilityClass != runtimecontract.RuntimeCapabilityClassValidation || len(command.RequiredKinds) != 1 || command.RequiredKinds[0] != runtimecontract.RuntimeBlockKindManagedValidationSession {
						t.Errorf("shell capability spy received %T / %+v", event.Data(), event.Data())
						return
					}
					*calls++
				})
				return hub, processRuns, runOwner, shellOwner, workspaceObserver, spy, calls
			}
			stopProcess := func(hub magicevent.Hub, runOwner *runmodule.Module, shellOwner *shellmodule.Module, workspaceObserver, spy magicevent.SimpleObserver) {
				if spy != nil {
					spy.Unsubscribe(shellevents.TopicAcquireRuntimeCapabilities)
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
			baseRepository := runtimeStateRepository{hub: hub}
			marker := "shell-capability-before-cas"
			if crashAfterCAS {
				marker = "shell-capability-after-cas"
			}
			crashRepository := &runtimeSaveCrashRepository{delegate: baseRepository, after: crashAfterCAS, marker: marker, match: func(current runtime.RuntimeAggregate) bool {
				return len(current.Facts.Capabilities.ValidationKinds) == 1 && current.Facts.Capabilities.ValidationKinds[0] == runtimecontract.RuntimeBlockKindManagedValidationSession
			}}
			crashingOwner := newRuntimeTestOwner()
			if err := crashingOwner.ConfigureRuntimeRepository(crashRepository); err != nil {
				t.Fatal(err)
			}
			crashing := newTestBiz(crashingOwner, hub, nil)
			run, err := processRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			aggregate, err := crashingOwner.initializeRuntimeFromRun(ctx, run)
			if err != nil {
				t.Fatal(err)
			}
			aggregate = runtimeTestPersistSealedSemanticAuthority(t, ctx, crashRepository, aggregate)
			func() {
				defer func() {
					if recovered := recover(); recovered != marker {
						t.Fatalf("unexpected shell capability crash marker: %v", recovered)
					}
				}()
				_, _ = crashing.prepareRuntimePrerequisites(ctx, aggregate)
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
			restarted := newRuntimeDriveTestModule(t, hub)
			loaded, err := restarted.loadRuntime(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			resumed, err := restarted.prepareRuntimePrerequisites(ctx, loaded)
			if err != nil {
				t.Fatal(err)
			}
			// This fixture intentionally has no state-change capability. The
			// reducer therefore terminalizes after acquisition; the capability
			// receiver boundary remains candidate-free on either CAS side.
			if len(resumed.Facts.Capabilities.ValidationKinds) != 1 || resumed.Facts.Capabilities.ValidationKinds[0] != runtimecontract.RuntimeBlockKindManagedValidationSession ||
				len(resumed.Facts.Prerequisites.Pending) != 0 || resumed.Decision.State != runtimecontract.ConvergenceStateFailed ||
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
func TestRuntimeFileBackedValidationScratchCommitCrashRestart(t *testing.T) {
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
			bootstrap := runtimeBootstrap()
			bootstrap.Requirements = []runtimecontract.RuntimeRequirement{
				{Kind: runtimecontract.PrerequisiteKindValidationScratch, OwnerModuleID: runtimecontract.RuntimeModuleOrchestrator, Reason: "workspace scoped validation scratch"},
				{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleFile,
					CapabilityClass: runtimecontract.RuntimeCapabilityClassStateChange, RequiredCapabilities: []string{runtimecontract.StepKindFileEdit}, Reason: "file capability keeps the authority nonterminal"},
			}
			if err := filerepository.NewRunRepository(runsPath).Create(ctx, &runmodel.Run{
				ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
				DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive,
			}); err != nil {
				t.Fatal(err)
			}

			startProcess := func(t *testing.T) (magicevent.Hub, *filerepository.RunRepository, *runmodule.Module, magicevent.SimpleObserver) {
				t.Helper()
				hub := magicevent.NewHub(16)
				processRuns := filerepository.NewRunRepository(runsPath)
				runOwner := runmodule.New()
				if err := runOwner.Configure(runbiz.Dependencies{Runs: processRuns, EventHub: hub}); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := runOwner.Start(); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
				workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
					result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{
						ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDirectory,
					}}, nil)
				})
				startRuntimeFileManifestOwner(t, ctx, hub)
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
			baseRepository := runtimeStateRepository{hub: hub}
			marker := "validation-scratch-before-cas"
			if crashAfterCAS {
				marker = "validation-scratch-after-cas"
			}
			crashRepository := &runtimeSaveCrashRepository{
				delegate: baseRepository, after: crashAfterCAS, marker: marker,
				match: func(current runtime.RuntimeAggregate) bool {
					return current.Facts.Resources.ValidationScratch != nil && current.Facts.Resources.ValidationScratch.Reference != ""
				},
			}
			crashingBiz := newRuntimeTestOwner()
			if err := crashingBiz.ConfigureRuntimeRepository(crashRepository); err != nil {
				t.Fatal(err)
			}
			crashingModule := newTestBiz(crashingBiz, hub, nil)
			run, err := processRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			aggregate, err := crashingBiz.initializeRuntimeFromRun(ctx, run)
			if err != nil {
				t.Fatal(err)
			}
			aggregate = runtimeTestPersistSealedSemanticAuthority(t, ctx, crashRepository, aggregate)
			func() {
				defer func() {
					if recovered := recover(); recovered != marker {
						t.Fatalf("unexpected validation scratch crash marker: %v", recovered)
					}
				}()
				_, _ = crashingModule.prepareRuntimePrerequisites(ctx, aggregate)
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
			restarted := newRuntimeDriveTestModule(t, hub)
			loaded, err := restarted.loadRuntime(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			resumed, err := restarted.prepareRuntimePrerequisites(ctx, loaded)
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
func TestRuntimeFileBackedValidationScratchStaleInvalidationCrashRestart(t *testing.T) {
	runRuntimeFileBackedValidationScratchStaleInvalidationCrashRestart(t, 0)
}

// The read-only stale ledger is independent from owner receipt progress. Its
// exhausted edge must persist a terminal fact after a complete process reload;
// it may not create a provider request, candidate, dispatch or ticket.
func TestRuntimeFileBackedValidationScratchStaleRefreshExhaustionIsTerminal(t *testing.T) {
	runRuntimeFileBackedValidationScratchStaleInvalidationCrashRestart(t, 1)
}

func runRuntimeFileBackedValidationScratchStaleInvalidationCrashRestart(t *testing.T, maxStaleRefreshes int) {
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
			bootstrap := runtimeBootstrap()
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
				DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive,
			}); err != nil {
				t.Fatal(err)
			}

			startProcess := func(t *testing.T) (magicevent.Hub, *filerepository.RunRepository, *runmodule.Module, magicevent.SimpleObserver) {
				t.Helper()
				hub := magicevent.NewHub(16)
				processRuns := filerepository.NewRunRepository(runsPath)
				runOwner := runmodule.New()
				if err := runOwner.Configure(runbiz.Dependencies{Runs: processRuns, EventHub: hub}); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := runOwner.Start(); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
				workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
					result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{
						ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDirectory,
					}}, nil)
				})
				startRuntimeFileManifestOwner(t, ctx, hub)
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
			first := newRuntimeDriveTestModule(t, hub)
			run, err := processRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			aggregate, err := first.initializeRuntimeFromRun(ctx, run)
			if err != nil {
				t.Fatal(err)
			}
			aggregate = runtimeTestPersistSealedSemanticAuthority(t, ctx, runtimeStateRepository{hub: hub}, aggregate)
			aggregate, err = first.prepareRuntimePrerequisites(ctx, aggregate)
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
			baseRepository := runtimeStateRepository{hub: hub}
			marker := "validation-scratch-stale-before-cas"
			if crashAfterCAS {
				marker = "validation-scratch-stale-after-cas"
			}
			crashRepository := &runtimeSaveCrashRepository{
				delegate: baseRepository, after: crashAfterCAS, marker: marker,
				match: func(current runtime.RuntimeAggregate) bool {
					return current.Facts.Resources.ValidationScratch == nil && current.Facts.Recovery.PrerequisiteRefreshes == 1 &&
						len(current.Facts.Prerequisites.Pending) == 1 && current.Facts.Prerequisites.Pending[0].Kind == runtimecontract.PrerequisiteKindValidationScratch
				},
			}
			crashingOwner := newRuntimeTestOwner()
			if err := crashingOwner.ConfigureRuntimeRepository(crashRepository); err != nil {
				t.Fatal(err)
			}
			crashing := newTestBiz(crashingOwner, hub, nil)
			loaded, err := crashing.loadRuntime(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			func() {
				defer func() {
					if recovered := recover(); recovered != marker {
						t.Fatalf("unexpected validation scratch stale crash marker: %v", recovered)
					}
				}()
				_, _ = crashing.driveRuntimeDecision(ctx, loaded, "stable-prefix-test")
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
			restarted := newRuntimeDriveTestModule(t, hub)
			loaded, err = restarted.loadRuntime(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			if !crashAfterCAS {
				loaded, err = restarted.driveRuntimeDecision(ctx, loaded, "stable-prefix-test")
				if err != nil {
					t.Fatal(err)
				}
			}
			resumed, err := restarted.prepareRuntimePrerequisites(ctx, loaded)
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
				crashRepository := &runtimeSaveCrashRepository{
					delegate: runtimeStateRepository{hub: hub}, after: crashAfterCAS, marker: marker,
					match: func(current runtime.RuntimeAggregate) bool {
						return current.Decision.State == runtimecontract.ConvergenceStateFailed && current.Facts.Failure != nil &&
							current.Facts.Failure.Code == "prerequisite_refresh_exhausted" &&
							current.Facts.Recovery.PrerequisiteRefreshes == maxStaleRefreshes
					},
				}
				crashingOwner := newRuntimeTestOwner()
				if err := crashingOwner.ConfigureRuntimeRepository(crashRepository); err != nil {
					t.Fatal(err)
				}
				crashing := newTestBiz(crashingOwner, hub, nil)
				func() {
					defer func() {
						if recovered := recover(); recovered != marker {
							t.Fatalf("unexpected read-only exhaustion crash marker: %v", recovered)
						}
					}()
					_, _ = crashing.driveRuntimeDecision(ctx, resumed, "stable-prefix-test")
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
					if crashed.RuntimeState.Decision.State != runtimecontract.ConvergenceStateFailed || crashed.RuntimeState.Facts.Failure == nil ||
						crashed.RuntimeState.Facts.Failure.Code != "prerequisite_refresh_exhausted" || crashed.RuntimeState.Facts.Resources.ValidationScratch != nil ||
						crashed.RuntimeState.Facts.Recovery.PrerequisiteRefreshes != maxStaleRefreshes {
						t.Fatalf("after-CAS read-only exhaustion was not terminal: %+v", crashed.RuntimeState)
					}
				} else if crashed.RuntimeState.Decision.State == runtimecontract.ConvergenceStateFailed || crashed.RuntimeState.Facts.Resources.ValidationScratch == nil ||
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
				terminalBiz := newRuntimeDriveTestModule(t, terminalHub)
				terminalResult, terminalErr := terminalBiz.driveRuntimeLocked(ctx, orchestratorevents.DriveRuntimeCommand{WorkspaceID: "ws-v2", RunID: "run-v2"})
				if terminalErr != nil || terminalResult.State != runtimecontract.ConvergenceStateFailed {
					t.Fatalf("read-only exhaustion fresh terminal reload=%+v err=%v", terminalResult, terminalErr)
				}
				persisted, loadErr := terminalRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
				if loadErr != nil || persisted.RuntimeState == nil || persisted.RuntimeState.Decision.State != runtimecontract.ConvergenceStateFailed ||
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
func TestRuntimeFileBackedFailureTargetCommitCrashRestart(t *testing.T) {
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
			bootstrap := runtimeBootstrap()
			if err := filerepository.NewRunRepository(runsPath).Create(ctx, &runmodel.Run{
				ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
				DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive,
			}); err != nil {
				t.Fatal(err)
			}

			startProcess := func(t *testing.T) (magicevent.Hub, *filerepository.RunRepository, *runmodule.Module, magicevent.SimpleObserver) {
				t.Helper()
				hub := magicevent.NewHub(16)
				processRuns := filerepository.NewRunRepository(runsPath)
				runOwner := runmodule.New()
				if err := runOwner.Configure(runbiz.Dependencies{Runs: processRuns, EventHub: hub}); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := runOwner.Start(); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
				workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
					result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable}}, nil)
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
			baseRepository := runtimeStateRepository{hub: hub}
			marker := "failure-target-before-cas"
			if crashAfterCAS {
				marker = "failure-target-after-cas"
			}
			crashRepository := &runtimeSaveCrashRepository{
				delegate: baseRepository, after: crashAfterCAS, marker: marker,
				match: func(current runtime.RuntimeAggregate) bool {
					target := current.Facts.Resources.FailureTarget
					return target != nil && target.Signature == "failure-target-v1" && len(target.TargetRefs) == 1 && target.TargetRefs[0] == "implementation.go"
				},
			}
			crashingOwner := newRuntimeTestOwner()
			if err := crashingOwner.ConfigureRuntimeRepository(crashRepository); err != nil {
				t.Fatal(err)
			}
			crashing := newTestBiz(crashingOwner, hub, nil)
			facts := runtimeAggregate(t, 1).Facts
			facts.Failure = &runtimecontract.FailureFact{Signature: "failure-target-v1", TargetRefs: []string{"implementation.go"}}
			aggregate, err := crashing.InitializeRuntime(ctx, facts, runtimeDefinition(), []runtimecontract.RuntimeRequirement{{
				Kind: runtimecontract.PrerequisiteKindFailureTarget, OwnerModuleID: runtimecontract.RuntimeModuleOrchestrator,
				Reason: "typed failure repair surface",
			}})
			if err != nil {
				t.Fatal(err)
			}
			if aggregate.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite {
				t.Fatalf("failure-target bootstrap action=%s, aggregate=%+v", aggregate.Decision.State, aggregate)
			}
			initialRevision := aggregate.StateRevision
			func() {
				defer func() {
					if recovered := recover(); recovered != marker {
						t.Fatalf("unexpected failure-target crash marker: %v", recovered)
					}
				}()
				_, _ = crashing.prepareRuntimePrerequisites(ctx, aggregate)
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
			restarted := newRuntimeDriveTestModule(t, hub)
			loaded, err := restarted.loadRuntime(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			resumed, err := restarted.prepareRuntimePrerequisites(ctx, loaded)
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
func TestRuntimeFileBackedCreationSelectionCommitCrashRestart(t *testing.T) {
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
			bootstrap := runtimeGroundedTargetSequenceBootstrap()
			bootstrap.Definition.CapabilityContract = []string{runtimecontract.StepKindFileWrite}
			bootstrap.Requirements = nil
			bootstrap.DefinitionFocuses[0].RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
			bootstrap.DefinitionFocuses[0].TargetSelectorRef = "file/workspace_deliverable"
			runtimeTestDeclareProspectiveWorkspaceTarget(&bootstrap, "initial-deliverable.txt")
			bootstrap = runtimeTestBootstrapWithReboundBindings(bootstrap)
			if err := filerepository.NewRunRepository(runsPath).Create(ctx, &runmodel.Run{
				ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
				DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive,
			}); err != nil {
				t.Fatal(err)
			}

			startProcess := func(t *testing.T) (magicevent.Hub, *filerepository.RunRepository, *runmodule.Module, *filemodule.Module, magicevent.SimpleObserver, magicevent.SimpleObserver, *int) {
				t.Helper()
				hub := magicevent.NewHub(16)
				processRuns := filerepository.NewRunRepository(runsPath)
				runOwner := runmodule.New()
				if err := runOwner.Configure(runbiz.Dependencies{Runs: processRuns, EventHub: hub}); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := runOwner.Start(); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
				workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
					result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable}}, nil)
				})
				fileOwner := filemodule.New()
				if err := fileOwner.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
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
				selectionObserver.Subscribe(fileevents.TopicAcquireRuntimeTargetSelection, func(event magicevent.Event, _ magicevent.Result) {
					if _, ok := event.Data().(fileevents.AcquireRuntimeTargetSelectionCommand); !ok {
						t.Errorf("creation selection spy received %T", event.Data())
						return
					}
					*selectionCalls++
				})
				return hub, processRuns, runOwner, fileOwner, workspaceObserver, selectionObserver, selectionCalls
			}
			stopProcess := func(hub magicevent.Hub, runOwner *runmodule.Module, fileOwner *filemodule.Module, workspaceObserver, selectionObserver magicevent.SimpleObserver) {
				if selectionObserver != nil {
					selectionObserver.Unsubscribe(fileevents.TopicAcquireRuntimeTargetSelection)
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
			baseRepository := runtimeStateRepository{hub: hub}
			marker := "creation-selection-before-cas"
			if crashAfterCAS {
				marker = "creation-selection-after-cas"
			}
			crashRepository := &runtimeSaveCrashRepository{
				delegate: baseRepository, after: crashAfterCAS, marker: marker,
				match: func(current runtime.RuntimeAggregate) bool {
					selection := current.Facts.Resources.TargetSelection
					return selection != nil && selection.Disposition == runtimecontract.RuntimeTargetSelectionCreate && selection.ProspectiveTargetRef == "initial-deliverable.txt"
				},
			}
			crashingBiz := newRuntimeTestOwner()
			if err := crashingBiz.ConfigureRuntimeRepository(crashRepository); err != nil {
				t.Fatal(err)
			}
			crashingModule := newTestBiz(crashingBiz, hub, nil)
			run, err := processRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			aggregate, err := crashingBiz.initializeRuntimeFromRun(ctx, run)
			if err != nil {
				t.Fatal(err)
			}
			aggregate = runtimeTestPersistSealedSemanticAuthority(t, ctx, crashRepository, aggregate)
			func() {
				defer func() {
					if recovered := recover(); recovered != marker {
						t.Fatalf("unexpected creation selection crash marker: %v", recovered)
					}
				}()
				_, _ = crashingModule.prepareRuntimePrerequisites(ctx, aggregate)
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
			restartedBiz := newRuntimeTestOwner()
			if err := restartedBiz.ConfigureRuntimeRepository(runtimeStateRepository{hub: hub}); err != nil {
				t.Fatal(err)
			}
			restartedModule := newTestBiz(restartedBiz, hub, nil)
			loaded, err := restartedBiz.loadRuntime(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			resumed, err := restartedModule.prepareRuntimePrerequisites(ctx, loaded)
			if err != nil {
				t.Fatal(err)
			}
			if resumed.Facts.Resources.TargetSelection == nil || resumed.Facts.Resources.TargetSelection.Disposition != runtimecontract.RuntimeTargetSelectionCreate ||
				resumed.Facts.Resources.TargetSelection.SelectorRef != "workspace-relative-new-file-target" || resumed.Facts.Resources.TargetSelection.ProspectiveTargetRef != "initial-deliverable.txt" ||
				len(resumed.Facts.Resources.PrerequisiteObservations) != 1 || len(resumed.CandidateAttempts) != 0 || resumed.ActiveDispatch != nil {
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
func TestRuntimeFileBackedTargetSnapshotCommitCrashRestart(t *testing.T) {
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
			bootstrap := runtimeBootstrap()
			runtimeTestDeclareExistingWorkspaceTarget(&bootstrap, "target.txt")
			bootstrap = runtimeTestBootstrapWithReboundBindings(bootstrap)
			if err := filerepository.NewRunRepository(runsPath).Create(ctx, &runmodel.Run{
				ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
				DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive,
			}); err != nil {
				t.Fatal(err)
			}

			startProcess := func(t *testing.T) (magicevent.Hub, *filerepository.RunRepository, *runmodule.Module, *filemodule.Module, magicevent.SimpleObserver, magicevent.SimpleObserver, *int) {
				t.Helper()
				hub := magicevent.NewHub(16)
				processRuns := filerepository.NewRunRepository(runsPath)
				runOwner := runmodule.New()
				if err := runOwner.Configure(runbiz.Dependencies{Runs: processRuns, EventHub: hub}); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := runOwner.Start(); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
				workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
					result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable}}, nil)
				})
				fileOwner := filemodule.New()
				if err := fileOwner.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
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
				snapshotObserver.Subscribe(fileevents.TopicAcquireTargetSnapshot, func(event magicevent.Event, _ magicevent.Result) {
					if _, ok := event.Data().(fileevents.AcquireTargetSnapshotCommand); !ok {
						t.Errorf("target snapshot spy received %T", event.Data())
						return
					}
					*snapshotCalls++
				})
				return hub, processRuns, runOwner, fileOwner, workspaceObserver, snapshotObserver, snapshotCalls
			}
			stopProcess := func(hub magicevent.Hub, runOwner *runmodule.Module, fileOwner *filemodule.Module, workspaceObserver, snapshotObserver magicevent.SimpleObserver) {
				if snapshotObserver != nil {
					snapshotObserver.Unsubscribe(fileevents.TopicAcquireTargetSnapshot)
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
			crashRepository := &runtimeSaveCrashRepository{
				delegate: runtimeStateRepository{hub: hub}, after: crashAfterCAS, marker: marker,
				match: func(current runtime.RuntimeAggregate) bool {
					return len(current.Facts.Resources.TargetSnapshots) == 1 && current.Facts.Resources.TargetSnapshots[0].TargetRef == "target.txt"
				},
			}
			crashingBiz := newRuntimeTestOwner()
			if err := crashingBiz.ConfigureRuntimeRepository(crashRepository); err != nil {
				t.Fatal(err)
			}
			crashingModule := newTestBiz(crashingBiz, hub, nil)
			run, err := processRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			aggregate, err := crashingBiz.initializeRuntimeFromRun(ctx, run)
			if err != nil {
				t.Fatal(err)
			}
			aggregate = runtimeTestPersistSealedSemanticAuthority(t, ctx, crashRepository, aggregate)
			func() {
				defer func() {
					if recovered := recover(); recovered != marker {
						t.Fatalf("unexpected target snapshot crash marker: %v", recovered)
					}
				}()
				_, _ = crashingModule.prepareRuntimePrerequisites(ctx, aggregate)
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
			restartedBiz := newRuntimeTestOwner()
			if err := restartedBiz.ConfigureRuntimeRepository(runtimeStateRepository{hub: hub}); err != nil {
				t.Fatal(err)
			}
			restartedModule := newTestBiz(restartedBiz, hub, nil)
			loaded, err := restartedBiz.loadRuntime(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			resumed, err := restartedModule.prepareRuntimePrerequisites(ctx, loaded)
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
			duplicateResponse, err := AcquireTargetSnapshotPrerequisite(ctx, hub, fileevents.AcquireTargetSnapshotCommand{
				WorkspaceID: "ws-v2", RunID: "run-v2", AuthoritySnapshotID: resumed.Decision.AuthoritySnapshot.ID,
				PrerequisiteID: committedPrerequisite.ID, WorkspaceDeliverableDirectory: deliverable,
				TargetRef: "target.txt", MaxBytes: runtimeDefaultTargetSnapshotBytes,
			})
			if err != nil {
				t.Fatalf("delayed duplicate File target snapshot query: %v", err)
			}
			if *restartedSnapshotCalls != duplicateCallsBefore+1 {
				t.Fatalf("delayed duplicate File target snapshot calls=%d, want %d", *restartedSnapshotCalls, duplicateCallsBefore+1)
			}
			if _, err := restartedBiz.CommitRuntimePrerequisite(ctx, "ws-v2", "run-v2", runtime.PrerequisiteResult{
				PrerequisiteID: duplicateResponse.Observation.PrerequisiteID, TargetSnapshot: &duplicateResponse.Snapshot, Observation: &duplicateResponse.Observation,
			}); err == nil {
				t.Fatal("delayed duplicate File target snapshot unexpectedly mutated committed aggregate")
			}
			final, err := restartedBiz.loadRuntime(ctx, "ws-v2", "run-v2")
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
			_, session, err := restartedBiz.EnsureRuntimeCandidateRequestSession(ctx, "ws-v2", "run-v2", prefix, manifest.Hash, "")
			if err != nil {
				t.Fatal(err)
			}
			physicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, session.NextSequence)
			if _, err := restartedBiz.MarkRuntimeCandidateRequestSubmitted(ctx, "ws-v2", "run-v2", session.LogicalRequestID, session.NextSequence, physicalCallID); err != nil {
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
			invalidated, err := restartedBiz.InvalidateRuntimeCandidateRequestSessionForPrerequisiteStale(ctx, "ws-v2", "run-v2", session.LogicalRequestID, stale)
			if err != nil {
				t.Fatal(err)
			}
			if invalidated.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite || len(invalidated.Facts.Resources.TargetSnapshots) != 0 ||
				len(invalidated.Facts.Resources.PrerequisiteObservations) != 1 || len(invalidated.Facts.Prerequisites.Pending) != 1 ||
				invalidated.Facts.Prerequisites.Pending[0].ID != committedPrerequisite.ID || invalidated.Facts.Recovery.PrerequisiteRefreshes != 1 || len(invalidated.CandidateAttempts) != 0 || invalidated.ActiveDispatch != nil ||
				len(invalidated.CandidateRequestSessions) != 1 || invalidated.CandidateRequestSessions[0].State != runtimecontract.RuntimeCandidateRequestSessionTerminal ||
				invalidated.CandidateRequestSessions[0].LastOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomePrerequisiteStale {
				t.Fatalf("target snapshot stale did not isolate owner fact: %+v", invalidated)
			}

			stopProcess(hub, runOwner, fileOwner, workspaceObserver, snapshotObserver)
			hub, processRuns, runOwner, fileOwner, workspaceObserver, snapshotObserver, staleReacquireCalls := startProcess(t)
			defer stopProcess(hub, runOwner, fileOwner, workspaceObserver, snapshotObserver)
			thirdBiz := newRuntimeTestOwner()
			if err := thirdBiz.ConfigureRuntimeRepository(runtimeStateRepository{hub: hub}); err != nil {
				t.Fatal(err)
			}
			thirdModule := newTestBiz(thirdBiz, hub, nil)
			loadedAfterStale, err := thirdBiz.loadRuntime(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			reacquired, err := thirdModule.prepareRuntimePrerequisites(ctx, loadedAfterStale)
			if err != nil {
				t.Fatal(err)
			}
			if *staleReacquireCalls != 1 || len(reacquired.Facts.Resources.TargetSnapshots) != 1 ||
				reacquired.Facts.Resources.TargetSnapshots[0].Content != "after\n" || len(reacquired.Facts.Resources.PrerequisiteObservations) != 2 ||
				reacquired.Facts.Recovery.PrerequisiteRefreshes != 1 ||
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
func TestRuntimeFileBackedFileAdmissibilityCommitCrashRestart(t *testing.T) {
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
			bootstrap := runtimeBootstrap()
			bootstrap.Requirements = []runtimecontract.RuntimeRequirement{
				{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleFile,
					CapabilityClass: runtimecontract.RuntimeCapabilityClassStateChange, RequiredCapabilities: []string{runtimecontract.StepKindFileEdit}, Reason: "File capability"},
				{Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleFile,
					RequiredCapabilities: []string{runtimecontract.StepKindFileEdit}, Reason: "File owner inventory"},
			}
			if err := filerepository.NewRunRepository(runsPath).Create(ctx, &runmodel.Run{
				ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
				DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive,
			}); err != nil {
				t.Fatal(err)
			}

			startProcess := func(t *testing.T) (magicevent.Hub, *filerepository.RunRepository, *runmodule.Module, *filemodule.Module, magicevent.SimpleObserver, magicevent.SimpleObserver, *int) {
				t.Helper()
				hub := magicevent.NewHub(16)
				processRuns := filerepository.NewRunRepository(runsPath)
				runOwner := runmodule.New()
				if err := runOwner.Configure(runbiz.Dependencies{Runs: processRuns, EventHub: hub}); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := runOwner.Start(); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
				workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
					result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable}}, nil)
				})
				fileOwner := filemodule.New()
				if err := fileOwner.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
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
				inventoryObserver.Subscribe(fileevents.TopicAcquireRuntimeOperationAdmissibility, func(event magicevent.Event, _ magicevent.Result) {
					if _, ok := event.Data().(fileevents.AcquireRuntimeOperationAdmissibilityCommand); !ok {
						t.Errorf("File admissibility spy received %T", event.Data())
						return
					}
					*inventoryCalls++
				})
				return hub, processRuns, runOwner, fileOwner, workspaceObserver, inventoryObserver, inventoryCalls
			}
			stopProcess := func(hub magicevent.Hub, runOwner *runmodule.Module, fileOwner *filemodule.Module, workspaceObserver, inventoryObserver magicevent.SimpleObserver) {
				if inventoryObserver != nil {
					inventoryObserver.Unsubscribe(fileevents.TopicAcquireRuntimeOperationAdmissibility)
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
			crashRepository := &runtimeSaveCrashRepository{
				delegate: runtimeStateRepository{hub: hub}, after: crashAfterCAS, marker: marker,
				match: func(current runtime.RuntimeAggregate) bool {
					return len(current.Facts.Resources.OperationAdmissibilities) == 1 && current.Facts.Resources.OperationAdmissibilities[0].OwnerModuleID == runtimecontract.RuntimeModuleFile
				},
			}
			crashingBiz := newRuntimeTestOwner()
			if err := crashingBiz.ConfigureRuntimeRepository(crashRepository); err != nil {
				t.Fatal(err)
			}
			crashingModule := newTestBiz(crashingBiz, hub, nil)
			run, err := processRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			aggregate, err := crashingBiz.initializeRuntimeFromRun(ctx, run)
			if err != nil {
				t.Fatal(err)
			}
			aggregate = runtimeTestPersistSealedSemanticAuthority(t, ctx, crashRepository, aggregate)
			func() {
				defer func() {
					if recovered := recover(); recovered != marker {
						t.Fatalf("unexpected File admissibility crash marker: %v", recovered)
					}
				}()
				_, _ = crashingModule.prepareRuntimePrerequisites(ctx, aggregate)
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
			restartedBiz := newRuntimeTestOwner()
			if err := restartedBiz.ConfigureRuntimeRepository(runtimeStateRepository{hub: hub}); err != nil {
				t.Fatal(err)
			}
			restartedModule := newTestBiz(restartedBiz, hub, nil)
			loaded, err := restartedBiz.loadRuntime(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			resumed, err := restartedModule.prepareRuntimePrerequisites(ctx, loaded)
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
			duplicateResponse, err := AcquireRuntimeOperationAdmissibilityPrerequisite(ctx, hub, runtimecontract.RuntimeOperationAdmissibilityRequest{
				WorkspaceID: "ws-v2", RunID: "run-v2", AuthoritySnapshotID: resumed.Decision.AuthoritySnapshot.ID,
				PrerequisiteID: committedPrerequisite.ID, OwnerModuleID: runtimecontract.RuntimeModuleFile,
				WorkspaceDeliverableDirectory: deliverable, RequiredKinds: committedPrerequisite.RequiredCapabilities,
				RequiredTargetRefs: runtime.GroundedRepairTargets(resumed.Facts), MaxItems: runtimecontract.DefaultRuntimeOwnerInventoryBudget().MaxItems,
			})
			if err != nil {
				t.Fatalf("delayed duplicate File inventory query: %v", err)
			}
			if *restartedInventoryCalls != duplicateCallsBefore+1 {
				t.Fatalf("delayed duplicate File inventory calls=%d, want %d", *restartedInventoryCalls, duplicateCallsBefore+1)
			}
			if _, err := restartedBiz.CommitRuntimePrerequisite(ctx, "ws-v2", "run-v2", runtime.PrerequisiteResult{
				PrerequisiteID: duplicateResponse.Observation.PrerequisiteID, OperationAdmissibility: &duplicateResponse.Admissibility, Observation: &duplicateResponse.Observation,
			}); err == nil {
				t.Fatal("delayed duplicate File inventory unexpectedly mutated committed aggregate")
			}
			final, err := restartedBiz.loadRuntime(ctx, "ws-v2", "run-v2")
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
			_, session, err := restartedBiz.EnsureRuntimeCandidateRequestSession(ctx, "ws-v2", "run-v2", prefix, manifest.Hash, "")
			if err != nil {
				t.Fatal(err)
			}
			physicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, session.NextSequence)
			if _, err := restartedBiz.MarkRuntimeCandidateRequestSubmitted(ctx, "ws-v2", "run-v2", session.LogicalRequestID, session.NextSequence, physicalCallID); err != nil {
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
			invalidated, err := restartedBiz.InvalidateRuntimeCandidateRequestSessionForPrerequisiteStale(ctx, "ws-v2", "run-v2", session.LogicalRequestID, stale)
			if err != nil {
				t.Fatal(err)
			}
			if invalidated.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite || len(invalidated.Facts.Resources.OperationAdmissibilities) != 0 ||
				len(invalidated.Facts.Resources.PrerequisiteObservations) != 1 || len(invalidated.Facts.Prerequisites.Pending) != 1 ||
				invalidated.Facts.Prerequisites.Pending[0].ID != inventory.PrerequisiteID || invalidated.Facts.Recovery.PrerequisiteRefreshes != 1 || len(invalidated.CandidateAttempts) != 0 || invalidated.ActiveDispatch != nil ||
				len(invalidated.CandidateRequestSessions) != 1 || invalidated.CandidateRequestSessions[0].State != runtimecontract.RuntimeCandidateRequestSessionTerminal ||
				invalidated.CandidateRequestSessions[0].LastOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomePrerequisiteStale {
				t.Fatalf("File inventory stale did not isolate owner fact: %+v", invalidated)
			}

			stopProcess(hub, runOwner, fileOwner, workspaceObserver, inventoryObserver)
			hub, processRuns, runOwner, fileOwner, workspaceObserver, inventoryObserver, staleReacquireCalls := startProcess(t)
			defer stopProcess(hub, runOwner, fileOwner, workspaceObserver, inventoryObserver)
			thirdBiz := newRuntimeTestOwner()
			if err := thirdBiz.ConfigureRuntimeRepository(runtimeStateRepository{hub: hub}); err != nil {
				t.Fatal(err)
			}
			thirdModule := newTestBiz(thirdBiz, hub, nil)
			loadedAfterStale, err := thirdBiz.loadRuntime(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			reacquired, err := thirdModule.prepareRuntimePrerequisites(ctx, loadedAfterStale)
			if err != nil {
				t.Fatal(err)
			}
			if *staleReacquireCalls != 1 || len(reacquired.Facts.Resources.OperationAdmissibilities) != 1 ||
				reacquired.Facts.Resources.OperationAdmissibilities[0].ObservedRevision == inventory.ObservedRevision ||
				len(reacquired.Facts.Resources.PrerequisiteObservations) != 2 || reacquired.Facts.Recovery.PrerequisiteRefreshes != 1 || len(reacquired.CandidateAttempts) != 0 || reacquired.ActiveDispatch != nil {
				t.Fatalf("fresh-process File inventory stale reacquisition=%+v calls=%d", reacquired, *staleReacquireCalls)
			}
			if _, err := processRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRuntimeFileBackedGitAdmissibilityCommitCrashRestart(t *testing.T) {
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
			writeRuntimeGitFixtureFile(t, deliverable)
			runsPath := filepath.Join(root, "runs.json")
			receiptRoot := filepath.Join(root, "git-owner-data")
			bootstrap := runtimeGitCapabilityStateBootstrap()
			if err := filerepository.NewRunRepository(runsPath).Create(ctx, &runmodel.Run{
				ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
				DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive,
			}); err != nil {
				t.Fatal(err)
			}

			startProcess := func(t *testing.T) (magicevent.Hub, *filerepository.RunRepository, *runmodule.Module, *filemodule.Module, *gitmodule.Module, magicevent.SimpleObserver, magicevent.SimpleObserver, *int) {
				t.Helper()
				hub := magicevent.NewHub(32)
				processRuns := filerepository.NewRunRepository(runsPath)
				runOwner := runmodule.New()
				if err := runOwner.Configure(runbiz.Dependencies{Runs: processRuns, EventHub: hub}); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := runOwner.Start(); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
				workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
					result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
				})
				fileOwner := filemodule.New()
				if err := fileOwner.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
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
				if err := gitOwner.Configure(gitbiz.Dependencies{EventHub: hub, RuntimeAdapter: gitbiz.NewRuntimeAdapter(), CapabilityStateReceipts: receipts}); err != nil {
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
				inventoryObserver.Subscribe(gitevents.TopicAcquireRuntimeOperationAdmissibility, func(event magicevent.Event, _ magicevent.Result) {
					command, ok := event.Data().(gitevents.AcquireRuntimeOperationAdmissibilityCommand)
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
					inventoryObserver.Unsubscribe(gitevents.TopicAcquireRuntimeOperationAdmissibility)
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
			crashRepository := &runtimeSaveCrashRepository{
				delegate: runtimeStateRepository{hub: hub}, after: crashAfterCAS, marker: marker,
				match: func(current runtime.RuntimeAggregate) bool {
					return len(current.Facts.Resources.OperationAdmissibilities) == 1 && current.Facts.Resources.OperationAdmissibilities[0].OwnerModuleID == runtimecontract.RuntimeModuleGit
				},
			}
			crashingBiz := newRuntimeTestOwner()
			if err := crashingBiz.ConfigureRuntimeRepository(crashRepository); err != nil {
				t.Fatal(err)
			}
			crashingModule := newTestBiz(crashingBiz, hub, nil)
			run, err := processRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			aggregate, err := crashingBiz.initializeRuntimeFromRun(ctx, run)
			if err != nil {
				t.Fatal(err)
			}
			aggregate = runtimeTestPersistSealedSemanticAuthority(t, ctx, crashRepository, aggregate)
			func() {
				defer func() {
					if recovered := recover(); recovered != marker {
						t.Fatalf("unexpected Git admissibility crash marker: %v", recovered)
					}
				}()
				_, _ = crashingModule.prepareRuntimePrerequisites(ctx, aggregate)
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
			restartedBiz := newRuntimeTestOwner()
			if err := restartedBiz.ConfigureRuntimeRepository(runtimeStateRepository{hub: hub}); err != nil {
				t.Fatal(err)
			}
			restartedModule := newTestBiz(restartedBiz, hub, nil)
			loaded, err := restartedBiz.loadRuntime(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			resumed, err := restartedModule.prepareRuntimePrerequisites(ctx, loaded)
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
			duplicateResponse, err := AcquireRuntimeOperationAdmissibilityPrerequisite(ctx, hub, runtimecontract.RuntimeOperationAdmissibilityRequest{
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
			if _, err := restartedBiz.CommitRuntimePrerequisite(ctx, "ws-v2", "run-v2", runtime.PrerequisiteResult{
				PrerequisiteID: duplicateResponse.Observation.PrerequisiteID, OperationAdmissibility: &duplicateResponse.Admissibility, Observation: &duplicateResponse.Observation,
			}); err == nil {
				t.Fatal("delayed duplicate Git inventory unexpectedly mutated committed aggregate")
			}
			final, err := restartedBiz.loadRuntime(ctx, "ws-v2", "run-v2")
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
			_, session, err := restartedBiz.EnsureRuntimeCandidateRequestSession(ctx, "ws-v2", "run-v2", prefix, manifest.Hash, "")
			if err != nil {
				t.Fatal(err)
			}
			physicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, session.NextSequence)
			if _, err := restartedBiz.MarkRuntimeCandidateRequestSubmitted(ctx, "ws-v2", "run-v2", session.LogicalRequestID, session.NextSequence, physicalCallID); err != nil {
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
			invalidated, err := restartedBiz.InvalidateRuntimeCandidateRequestSessionForPrerequisiteStale(ctx, "ws-v2", "run-v2", session.LogicalRequestID, stale)
			if err != nil {
				t.Fatal(err)
			}
			if invalidated.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite || len(invalidated.Facts.Resources.OperationAdmissibilities) != 0 ||
				len(invalidated.Facts.Resources.CapabilityStates) != 0 || len(invalidated.Facts.Resources.PrerequisiteObservations) != 1 ||
				len(invalidated.Facts.Prerequisites.Pending) != 2 || invalidated.Facts.Recovery.PrerequisiteRefreshes != 1 || len(invalidated.CandidateAttempts) != 0 || invalidated.ActiveDispatch != nil ||
				len(invalidated.CandidateRequestSessions) != 1 || invalidated.CandidateRequestSessions[0].State != runtimecontract.RuntimeCandidateRequestSessionTerminal ||
				invalidated.CandidateRequestSessions[0].LastOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomePrerequisiteStale {
				t.Fatalf("Git inventory stale did not retire the bound repository-state chain: %+v", invalidated)
			}
			// The owner stale receipt represents a real repository revision change.
			// Make that change before the fresh process re-acquires the bound state;
			// otherwise a correct owner would return the same old revision and SM-2
			// must reject it through the persisted invalidation fence.
			runRuntimeGit(t, deliverable, "init", "-q")
			runRuntimeGit(t, deliverable, "config", "user.name", "WorkOrch Test")
			runRuntimeGit(t, deliverable, "config", "user.email", "workorch@example.test")

			stopProcess(hub, runOwner, fileOwner, gitOwner, workspaceObserver, inventoryObserver)
			hub, processRuns, runOwner, fileOwner, gitOwner, workspaceObserver, inventoryObserver, staleReacquireCalls := startProcess(t)
			defer stopProcess(hub, runOwner, fileOwner, gitOwner, workspaceObserver, inventoryObserver)
			thirdBiz := newRuntimeTestOwner()
			if err := thirdBiz.ConfigureRuntimeRepository(runtimeStateRepository{hub: hub}); err != nil {
				t.Fatal(err)
			}
			thirdModule := newTestBiz(thirdBiz, hub, nil)
			loadedAfterStale, err := thirdBiz.loadRuntime(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			reacquired, err := thirdModule.prepareRuntimePrerequisites(ctx, loadedAfterStale)
			if err != nil {
				t.Fatal(err)
			}
			if *staleReacquireCalls != 1 || len(reacquired.Facts.Resources.OperationAdmissibilities) != 1 || len(reacquired.Facts.Resources.CapabilityStates) != 1 ||
				reacquired.Facts.Resources.OperationAdmissibilities[0].AcquisitionBinding != reacquired.Facts.Resources.CapabilityStates[0].AcquisitionBinding ||
				len(reacquired.Facts.Resources.PrerequisiteObservations) != 3 || reacquired.Facts.Recovery.PrerequisiteRefreshes != 1 || len(reacquired.CandidateAttempts) != 0 || reacquired.ActiveDispatch != nil {
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
func TestRuntimeFileBackedShellAdmissibilityCommitCrashRestart(t *testing.T) {
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
			bootstrap := runtimeBootstrap()
			bootstrap.Definition.CapabilityContract = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
			bootstrap.DefinitionFocuses[0].RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
			bootstrap.Slices[0].RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
			runtimeTestDeclareExistingWorkspaceTarget(&bootstrap, "target.txt")
			bootstrap.Requirements = []runtimecontract.RuntimeRequirement{
				{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleShell,
					CapabilityClass: runtimecontract.RuntimeCapabilityClassValidation, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, Reason: "Shell capability"},
				{Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleShell,
					RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, Reason: "Shell owner inventory"},
			}
			bootstrap = runtimeTestBootstrapWithReboundBindings(bootstrap)
			if err := filerepository.NewRunRepository(runsPath).Create(ctx, &runmodel.Run{
				ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
				DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive,
			}); err != nil {
				t.Fatal(err)
			}

			shellTimeout := ""
			startProcess := func(t *testing.T) (magicevent.Hub, *filerepository.RunRepository, *runmodule.Module, *filemodule.Module, *shellmodule.Module, magicevent.SimpleObserver, magicevent.SimpleObserver, *int) {
				t.Helper()
				hub := magicevent.NewHub(32)
				processRuns := filerepository.NewRunRepository(runsPath)
				runOwner := runmodule.New()
				if err := runOwner.Configure(runbiz.Dependencies{Runs: processRuns, EventHub: hub}); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := runOwner.Start(); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
				workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
					result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable}}, nil)
				})
				fileOwner := filemodule.New()
				if err := fileOwner.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
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
				if err := shellOwner.Configure(shellbiz.Dependencies{EventHub: hub, RuntimeAdapter: adapter}); err != nil {
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
				inventoryObserver.Subscribe(shellevents.TopicAcquireRuntimeOperationAdmissibility, func(event magicevent.Event, _ magicevent.Result) {
					command, ok := event.Data().(shellevents.AcquireRuntimeOperationAdmissibilityCommand)
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
					inventoryObserver.Unsubscribe(shellevents.TopicAcquireRuntimeOperationAdmissibility)
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
			crashRepository := &runtimeSaveCrashRepository{
				delegate: runtimeStateRepository{hub: hub}, after: crashAfterCAS, marker: marker,
				match: func(current runtime.RuntimeAggregate) bool {
					return len(current.Facts.Resources.OperationAdmissibilities) == 1 && current.Facts.Resources.OperationAdmissibilities[0].OwnerModuleID == runtimecontract.RuntimeModuleShell
				},
			}
			crashingBiz := newRuntimeTestOwner()
			if err := crashingBiz.ConfigureRuntimeRepository(crashRepository); err != nil {
				t.Fatal(err)
			}
			crashingModule := newTestBiz(crashingBiz, hub, nil)
			run, err := processRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			aggregate, err := crashingBiz.initializeRuntimeFromRun(ctx, run)
			if err != nil {
				t.Fatal(err)
			}
			aggregate = runtimeTestPersistSealedSemanticAuthority(t, ctx, crashRepository, aggregate)
			func() {
				defer func() {
					if recovered := recover(); recovered != marker {
						t.Fatalf("unexpected Shell admissibility crash marker: %v", recovered)
					}
				}()
				_, _ = crashingModule.prepareRuntimePrerequisites(ctx, aggregate)
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
				if len(crashed.RuntimeState.Facts.Resources.OperationAdmissibilities) != 1 || len(crashed.RuntimeState.Facts.Resources.PrerequisiteObservations) != 3 {
					t.Fatalf("after-CAS Shell inventory was not persisted: %+v", crashed.RuntimeState)
				}
			} else if len(crashed.RuntimeState.Facts.Resources.OperationAdmissibilities) != 0 || len(crashed.RuntimeState.Facts.Resources.PrerequisiteObservations) != 2 {
				t.Fatalf("before-CAS Shell inventory leaked into persisted state: %+v", crashed.RuntimeState)
			}

			stopProcess(hub, runOwner, fileOwner, shellOwner, workspaceObserver, inventoryObserver)
			hub, processRuns, runOwner, fileOwner, shellOwner, workspaceObserver, inventoryObserver, restartedInventoryCalls := startProcess(t)
			defer stopProcess(hub, runOwner, fileOwner, shellOwner, workspaceObserver, inventoryObserver)
			restartedBiz := newRuntimeTestOwner()
			if err := restartedBiz.ConfigureRuntimeRepository(runtimeStateRepository{hub: hub}); err != nil {
				t.Fatal(err)
			}
			restartedModule := newTestBiz(restartedBiz, hub, nil)
			loaded, err := restartedBiz.loadRuntime(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			resumed, err := restartedModule.prepareRuntimePrerequisites(ctx, loaded)
			if err != nil {
				t.Fatal(err)
			}
			if len(resumed.Facts.Resources.OperationAdmissibilities) != 1 || resumed.Facts.Resources.OperationAdmissibilities[0].OwnerModuleID != runtimecontract.RuntimeModuleShell ||
				resumed.Facts.Resources.OperationAdmissibilities[0].SnapshotID == "" || resumed.Facts.Resources.OperationAdmissibilities[0].ObservedRevision == "" ||
				len(resumed.Facts.Resources.PrerequisiteObservations) != 4 || len(resumed.CandidateRequestSessions) != 0 || len(resumed.CandidateAttempts) != 0 ||
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
			duplicateResponse, err := AcquireRuntimeOperationAdmissibilityPrerequisite(ctx, hub, runtimecontract.RuntimeOperationAdmissibilityRequest{
				WorkspaceID: "ws-v2", RunID: "run-v2", AuthoritySnapshotID: resumed.Decision.AuthoritySnapshot.ID,
				PrerequisiteID: committedPrerequisite.ID, OwnerModuleID: runtimecontract.RuntimeModuleShell,
				WorkspaceDeliverableDirectory: deliverable, RequiredKinds: committedPrerequisite.RequiredCapabilities,
				RequiredTargetRefs: runtime.GroundedRepairTargets(resumed.Facts), MaxItems: runtimecontract.DefaultRuntimeOwnerInventoryBudget().MaxItems,
			})
			if err != nil {
				t.Fatalf("delayed duplicate Shell inventory query: %v", err)
			}
			if *restartedInventoryCalls != duplicateCallsBefore+1 {
				t.Fatalf("delayed duplicate Shell inventory calls=%d, want %d", *restartedInventoryCalls, duplicateCallsBefore+1)
			}
			if _, err := restartedBiz.CommitRuntimePrerequisite(ctx, "ws-v2", "run-v2", runtime.PrerequisiteResult{
				PrerequisiteID: duplicateResponse.Observation.PrerequisiteID, OperationAdmissibility: &duplicateResponse.Admissibility, Observation: &duplicateResponse.Observation,
			}); err == nil {
				t.Fatal("delayed duplicate Shell inventory unexpectedly mutated committed aggregate")
			}
			final, err := restartedBiz.loadRuntime(ctx, "ws-v2", "run-v2")
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
			_, session, err := restartedBiz.EnsureRuntimeCandidateRequestSession(ctx, "ws-v2", "run-v2", prefix, manifest.Hash, "")
			if err != nil {
				t.Fatal(err)
			}
			physicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, session.NextSequence)
			if _, err := restartedBiz.MarkRuntimeCandidateRequestSubmitted(ctx, "ws-v2", "run-v2", session.LogicalRequestID, session.NextSequence, physicalCallID); err != nil {
				t.Fatal(err)
			}
			stale := runtimecontract.RuntimePrerequisiteStale{
				PrerequisiteID: inventory.PrerequisiteID, ObservationID: inventoryObservation.ObservationID,
				Domain: targetDependency.Domain, OwnerModuleID: targetDependency.OwnerModuleID, SubjectRef: targetDependency.SubjectRef,
				ExpectedRevision: targetDependency.Revision, ObservedRevision: "target-after-read-only-stale", Reason: "File owner observed a newer validation target revision",
			}
			invalidated, err := restartedBiz.InvalidateRuntimeCandidateRequestSessionForPrerequisiteStale(ctx, "ws-v2", "run-v2", session.LogicalRequestID, stale)
			if err != nil {
				t.Fatal(err)
			}
			if invalidated.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite || len(invalidated.Facts.Resources.OperationAdmissibilities) != 0 || len(invalidated.Facts.Resources.TargetSnapshots) != 0 ||
				len(invalidated.Facts.Resources.PrerequisiteObservations) != 2 || len(invalidated.Facts.Prerequisites.Pending) != 2 ||
				invalidated.Facts.Prerequisites.Pending[0].ID != inventory.PrerequisiteID || invalidated.Facts.Prerequisites.Pending[1].Kind != runtimecontract.PrerequisiteKindTargetSnapshot ||
				invalidated.Facts.Recovery.PrerequisiteRefreshes != 1 || len(invalidated.CandidateAttempts) != 0 || invalidated.ActiveDispatch != nil ||
				len(invalidated.CandidateRequestSessions) != 1 || invalidated.CandidateRequestSessions[0].State != runtimecontract.RuntimeCandidateRequestSessionTerminal ||
				invalidated.CandidateRequestSessions[0].LastOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomePrerequisiteStale {
				t.Fatalf("Shell target stale did not isolate Shell inventory: %+v", invalidated)
			}

			stopProcess(hub, runOwner, fileOwner, shellOwner, workspaceObserver, inventoryObserver)
			hub, processRuns, runOwner, fileOwner, shellOwner, workspaceObserver, inventoryObserver, staleReacquireCalls := startProcess(t)
			defer stopProcess(hub, runOwner, fileOwner, shellOwner, workspaceObserver, inventoryObserver)
			thirdBiz := newRuntimeTestOwner()
			if err := thirdBiz.ConfigureRuntimeRepository(runtimeStateRepository{hub: hub}); err != nil {
				t.Fatal(err)
			}
			thirdModule := newTestBiz(thirdBiz, hub, nil)
			loadedAfterStale, err := thirdBiz.loadRuntime(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			reacquired, err := thirdModule.prepareRuntimePrerequisites(ctx, loadedAfterStale)
			if err != nil {
				t.Fatal(err)
			}
			reacquiredInventory := reacquired.Facts.Resources.OperationAdmissibilities[0]
			reacquiredObservation, found := runtimecontract.RuntimePrerequisiteObservationForID(reacquired.Facts.Resources.PrerequisiteObservations, reacquiredInventory.PrerequisiteID)
			if !found || *staleReacquireCalls != 1 || len(reacquired.Facts.Resources.OperationAdmissibilities) != 1 ||
				len(reacquired.Facts.Resources.PrerequisiteObservations) != 4 || reacquired.Facts.Recovery.PrerequisiteRefreshes != 1 || reacquiredObservation.ObservationID == inventoryObservation.ObservationID ||
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
			_, policySession, err := thirdBiz.EnsureRuntimeCandidateRequestSession(ctx, "ws-v2", "run-v2", prefix, manifest.Hash, "")
			if err != nil {
				t.Fatal(err)
			}
			policyPhysicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(policySession.LogicalRequestID, policySession.NextSequence)
			if _, err := thirdBiz.MarkRuntimeCandidateRequestSubmitted(ctx, "ws-v2", "run-v2", policySession.LogicalRequestID, policySession.NextSequence, policyPhysicalCallID); err != nil {
				t.Fatal(err)
			}
			policyStale := runtimecontract.RuntimePrerequisiteStale{
				PrerequisiteID: reacquiredInventory.PrerequisiteID, ObservationID: reacquiredObservation.ObservationID,
				Domain: policyDependency.Domain, OwnerModuleID: policyDependency.OwnerModuleID, SubjectRef: policyDependency.SubjectRef,
				ExpectedRevision: policyDependency.Revision, ObservedRevision: "shell-policy-changed", Reason: "Shell runtime policy changed",
			}
			policyInvalidated, err := thirdBiz.InvalidateRuntimeCandidateRequestSessionForPrerequisiteStale(ctx, "ws-v2", "run-v2", policySession.LogicalRequestID, policyStale)
			if err != nil {
				t.Fatal(err)
			}
			if policyInvalidated.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite || len(policyInvalidated.Facts.Resources.OperationAdmissibilities) != 0 ||
				len(policyInvalidated.Facts.Resources.PrerequisiteObservations) != 3 || len(policyInvalidated.Facts.Prerequisites.Pending) != 1 ||
				policyInvalidated.Facts.Prerequisites.Pending[0].ID != reacquiredInventory.PrerequisiteID || policyInvalidated.Facts.Recovery.PrerequisiteRefreshes != 2 || len(policyInvalidated.CandidateAttempts) != 0 || policyInvalidated.ActiveDispatch != nil {
				t.Fatalf("Shell policy stale did not isolate Shell inventory: %+v", policyInvalidated)
			}

			// A changed timeout is a different Shell policy revision. It is supplied
			// only to the newly constructed owner after the stale CAS; no policy
			// value is written by SM-2 or inherited from the prior process.
			stopProcess(hub, runOwner, fileOwner, shellOwner, workspaceObserver, inventoryObserver)
			shellTimeout = "90s"
			hub, processRuns, runOwner, fileOwner, shellOwner, workspaceObserver, inventoryObserver, policyReacquireCalls := startProcess(t)
			defer stopProcess(hub, runOwner, fileOwner, shellOwner, workspaceObserver, inventoryObserver)
			fourthBiz := newRuntimeTestOwner()
			if err := fourthBiz.ConfigureRuntimeRepository(runtimeStateRepository{hub: hub}); err != nil {
				t.Fatal(err)
			}
			fourthModule := newTestBiz(fourthBiz, hub, nil)
			loadedAfterPolicyStale, err := fourthBiz.loadRuntime(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			reacquiredPolicy, err := fourthModule.prepareRuntimePrerequisites(ctx, loadedAfterPolicyStale)
			if err != nil {
				t.Fatal(err)
			}
			policyInventory := reacquiredPolicy.Facts.Resources.OperationAdmissibilities[0]
			policyObservation, found := runtimecontract.RuntimePrerequisiteObservationForID(reacquiredPolicy.Facts.Resources.PrerequisiteObservations, policyInventory.PrerequisiteID)
			if !found || *policyReacquireCalls != 1 || len(reacquiredPolicy.Facts.Resources.OperationAdmissibilities) != 1 ||
				len(reacquiredPolicy.Facts.Resources.PrerequisiteObservations) != 4 || reacquiredPolicy.Facts.Recovery.PrerequisiteRefreshes != 2 || policyInventory.ObservedRevision == reacquiredInventory.ObservedRevision ||
				policyObservation.ObservationID == reacquiredObservation.ObservationID || len(reacquiredPolicy.CandidateAttempts) != 0 || reacquiredPolicy.ActiveDispatch != nil {
				t.Fatalf("fresh-process Shell policy stale reacquisition=%+v calls=%d", reacquiredPolicy, *policyReacquireCalls)
			}
			if _, err := processRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestRuntimeFileBackedAdmissibilityThreeOwnerMergeRestart proves the SM-2
// merge boundary with the three production inventory owners. Each inventory is
// committed independently, then every process-local object is destroyed before
// the next owner runs. The final restart must therefore recover only the three
// persisted owner facts, never an in-memory receipt or candidate side effect.
func TestRuntimeFileBackedAdmissibilityThreeOwnerMergeRestart(t *testing.T) {
	runRuntimeFileBackedAdmissibilityThreeOwnerRestart(t, "")
}

func TestRuntimeFileBackedAdmissibilityThreeOwnerStaleRestart(t *testing.T) {
	for _, staleCase := range []string{"file_target", "git_repository", "shell_policy"} {
		t.Run(staleCase, func(t *testing.T) {
			runRuntimeFileBackedAdmissibilityThreeOwnerRestart(t, staleCase)
		})
	}
}

func runRuntimeFileBackedAdmissibilityThreeOwnerRestart(t *testing.T, staleCase string) {
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
	bootstrap := runtimeFileShellGitBootstrap()
	runtimeTestUseExistingMainTarget(&bootstrap)
	bootstrap = runtimeTestBootstrapWithReboundBindings(bootstrap)
	if err := filerepository.NewRunRepository(runsPath).Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
		DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive,
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
		if err := value.runOwner.Configure(runbiz.Dependencies{Runs: value.runs, EventHub: value.hub}); err != nil {
			value.hub.Terminate(ctx)
			t.Fatal(err)
		}
		if err := value.runOwner.Start(); err != nil {
			value.hub.Terminate(ctx)
			t.Fatal(err)
		}
		value.workspaceObs = magicevent.NewSimpleObserver(workspaceevents.CommandDestination, value.hub)
		value.workspaceObs.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
			result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
		})
		value.fileOwner = filemodule.New()
		if err := value.fileOwner.Configure(filebiz.Dependencies{EventHub: value.hub}); err != nil {
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
		if err := value.gitOwner.Configure(gitbiz.Dependencies{EventHub: value.hub, RuntimeAdapter: gitbiz.NewRuntimeAdapter(), CapabilityStateReceipts: receipts}); err != nil {
			t.Fatal(err)
		}
		if err := value.gitOwner.Start(); err != nil {
			t.Fatal(err)
		}
		value.shellOwner = shellmodule.New()
		if err := value.shellOwner.Configure(shellbiz.Dependencies{EventHub: value.hub, RuntimeAdapter: shellbiz.NewRuntimeAdapterWithTimeout(shellTimeout)}); err != nil {
			t.Fatal(err)
		}
		if err := value.shellOwner.Start(); err != nil {
			t.Fatal(err)
		}
		startRuntimeProviderCeilingTestOwner(t, value.hub)
		value.inventoryObs = magicevent.NewSimpleObserverWithMatchID("three-owner-admissibility-query-spy", "blocks/#", value.hub)
		value.inventoryObs.Subscribe(fileevents.TopicAcquireRuntimeOperationAdmissibility, func(event magicevent.Event, _ magicevent.Result) {
			if _, ok := event.Data().(fileevents.AcquireRuntimeOperationAdmissibilityCommand); !ok {
				t.Errorf("File admissibility spy received %T", event.Data())
				return
			}
			value.queries[runtimecontract.RuntimeModuleFile]++
		})
		value.inventoryObs.Subscribe(gitevents.TopicAcquireRuntimeOperationAdmissibility, func(event magicevent.Event, _ magicevent.Result) {
			if _, ok := event.Data().(gitevents.AcquireRuntimeOperationAdmissibilityCommand); !ok {
				t.Errorf("Git admissibility spy received %T", event.Data())
				return
			}
			value.queries[runtimecontract.RuntimeModuleGit]++
		})
		value.inventoryObs.Subscribe(shellevents.TopicAcquireRuntimeOperationAdmissibility, func(event magicevent.Event, _ magicevent.Result) {
			if _, ok := event.Data().(shellevents.AcquireRuntimeOperationAdmissibilityCommand); !ok {
				t.Errorf("Shell admissibility spy received %T", event.Data())
				return
			}
			value.queries[runtimecontract.RuntimeModuleShell]++
		})
		return value
	}
	stopProcess := func(value *process) {
		if value == nil {
			return
		}
		if value.inventoryObs != nil {
			value.inventoryObs.Unsubscribe(fileevents.TopicAcquireRuntimeOperationAdmissibility)
			value.inventoryObs.Unsubscribe(gitevents.TopicAcquireRuntimeOperationAdmissibility)
			value.inventoryObs.Unsubscribe(shellevents.TopicAcquireRuntimeOperationAdmissibility)
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
	newBiz := func(hub magicevent.Hub) (*Biz, *Biz) {
		owner := newRuntimeTestOwner()
		if err := owner.ConfigureRuntimeRepository(runtimeStateRepository{hub: hub}); err != nil {
			t.Fatal(err)
		}
		return owner, newTestBiz(owner, hub, nil)
	}
	biz, module := newBiz(value.hub)
	run, err := value.runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err := biz.initializeRuntimeFromRun(ctx, run)
	if err != nil {
		t.Fatal(err)
	}
	aggregate = runtimeTestPersistSealedSemanticAuthority(t, ctx, runtimeStateRepository{hub: value.hub}, aggregate)

	// Establish capabilities, manifest and the Git repository-state binding
	// first. They are prerequisites for the three owner inventories but are not
	// the subject of this merge proof.
	for {
		var next *runtimecontract.RuntimePrerequisite
		for _, prerequisite := range runtimeOrderedPrerequisites(aggregate.Facts.Prerequisites.Pending) {
			if prerequisite.Kind != runtimecontract.PrerequisiteKindOperationAdmissibility {
				candidate := prerequisite
				next = &candidate
				break
			}
		}
		if next == nil {
			break
		}
		result, err := module.resolveRuntimePrerequisite(ctx, aggregate, deliverable, root, *next)
		if err != nil {
			t.Fatal(err)
		}
		aggregate, err = biz.CommitRuntimePrerequisite(ctx, "ws-v2", "run-v2", result)
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(aggregate.Facts.Resources.OperationAdmissibilities) != 0 || len(aggregate.Facts.Resources.CapabilityStates) != 1 {
		t.Fatalf("three-owner fixture did not establish prerequisites before inventories: %+v", aggregate.Facts)
	}

	findPending := func(current runtime.RuntimeAggregate, owner string) runtimecontract.RuntimePrerequisite {
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
		result, err := module.resolveRuntimePrerequisite(ctx, aggregate, deliverable, root, prerequisite)
		if err != nil {
			t.Fatalf("resolve %s inventory: %v", owner, err)
		}
		aggregate, err = biz.CommitRuntimePrerequisite(ctx, "ws-v2", "run-v2", result)
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
		aggregate, err = biz.loadRuntime(ctx, "ws-v2", "run-v2")
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
		response, err := AcquireRuntimeOperationAdmissibilityPrerequisite(ctx, value.hub, runtimecontract.RuntimeOperationAdmissibilityRequest{
			WorkspaceID: "ws-v2", RunID: "run-v2", AuthoritySnapshotID: aggregate.Decision.AuthoritySnapshot.ID,
			PrerequisiteID: prerequisite.ID, OwnerModuleID: owner, WorkspaceDeliverableDirectory: deliverable,
			RequiredKinds: prerequisite.RequiredCapabilities, RequiredTargetRefs: runtime.GroundedRepairTargets(aggregate.Facts),
			CapabilityStateBinding: inventory.AcquisitionBinding, MaxItems: runtimecontract.DefaultRuntimeOwnerInventoryBudget().MaxItems,
		})
		if err != nil {
			t.Fatalf("delayed duplicate %s inventory query: %v", owner, err)
		}
		if _, err := biz.CommitRuntimePrerequisite(ctx, "ws-v2", "run-v2", runtime.PrerequisiteResult{
			PrerequisiteID: response.Observation.PrerequisiteID, OperationAdmissibility: &response.Admissibility, Observation: &response.Observation,
		}); err == nil {
			t.Fatalf("delayed duplicate %s inventory unexpectedly committed", owner)
		}
		aggregate, err = biz.loadRuntime(ctx, "ws-v2", "run-v2")
		if err != nil || aggregate.StateRevision != beforeRevision || len(aggregate.Facts.Resources.OperationAdmissibilities) != len(owners) ||
			len(aggregate.CandidateRequestSessions) != 0 || len(aggregate.CandidateAttempts) != 0 || aggregate.ActiveDispatch != nil {
			t.Fatalf("duplicate %s inventory mutated merged aggregate: aggregate=%+v err=%v", owner, aggregate, err)
		}
	}
	if staleCase == "" {
		return
	}

	findObservation := func(current runtime.RuntimeAggregate, prerequisiteID string) runtimecontract.RuntimePrerequisiteObservation {
		for _, observation := range current.Facts.Resources.PrerequisiteObservations {
			if observation.PrerequisiteID == prerequisiteID {
				return observation
			}
		}
		t.Fatalf("missing current observation for %s: %+v", prerequisiteID, current.Facts.Resources.PrerequisiteObservations)
		return runtimecontract.RuntimePrerequisiteObservation{}
	}
	findInventory := func(current runtime.RuntimeAggregate, owner string) runtimecontract.RuntimeOperationAdmissibility {
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
		runRuntimeGit(t, deliverable, "init", "-q")
		runRuntimeGit(t, deliverable, "config", "user.name", "WorkOrch Test")
		runRuntimeGit(t, deliverable, "config", "user.email", "workorch@example.test")
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
	invalidated, err := biz.InvalidateRuntimePrerequisitesForReadOnlyStale(ctx, "ws-v2", "run-v2", stale)
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
	aggregate, err = biz.loadRuntime(ctx, "ws-v2", "run-v2")
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
		for _, prerequisite := range runtimeOrderedPrerequisites(aggregate.Facts.Prerequisites.Pending) {
			if prerequisite.Kind != runtimecontract.PrerequisiteKindOperationAdmissibility {
				candidate := prerequisite
				next = &candidate
				break
			}
		}
		if next == nil {
			break
		}
		result, err := module.resolveRuntimePrerequisite(ctx, aggregate, deliverable, root, *next)
		if err != nil {
			t.Fatalf("%s stale reacquire dependency %s: %v", staleCase, next.ID, err)
		}
		aggregate, err = biz.CommitRuntimePrerequisite(ctx, "ws-v2", "run-v2", result)
		if err != nil {
			t.Fatalf("%s stale commit dependency %s: %v", staleCase, next.ID, err)
		}
	}
	for _, owner := range wantRetired {
		prerequisite := findPending(aggregate, owner)
		result, err := module.resolveRuntimePrerequisite(ctx, aggregate, deliverable, root, prerequisite)
		if err != nil {
			t.Fatalf("%s stale reacquire %s: %v", staleCase, owner, err)
		}
		aggregate, err = biz.CommitRuntimePrerequisite(ctx, "ws-v2", "run-v2", result)
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
	duplicate, err := biz.InvalidateRuntimePrerequisitesForReadOnlyStale(ctx, "ws-v2", "run-v2", stale)
	if err != nil || duplicate.StateRevision != beforeDuplicate || duplicate.Facts.Recovery != aggregate.Facts.Recovery ||
		len(duplicate.PrerequisiteInvalidations) != len(aggregate.PrerequisiteInvalidations) {
		t.Fatalf("%s stale duplicate was not a no-op: duplicate=%+v err=%v", staleCase, duplicate, err)
	}
}

func TestRuntimeFileBackedGitSnapshotCommitResponseLossRestartReusesCommittedSnapshot(t *testing.T) {
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
	writeRuntimeGitFixtureFile(t, deliverable)
	bootstrap := runtimeGitCapabilityStateBootstrap()
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2", DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	startRuntimeReceiptTestOwner(t, hub)
	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	startRuntimeFileManifestOwner(t, ctx, hub)
	storeRoot := filepath.Join(root, "git-owner-data")
	store, err := gitbiz.NewCapabilityStateReceiptStore(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	gitOwner := gitmodule.New()
	if err := gitOwner.Configure(gitbiz.Dependencies{EventHub: hub, RuntimeAdapter: gitbiz.NewRuntimeAdapter(), CapabilityStateReceipts: store}); err != nil {
		t.Fatal(err)
	}
	if err := gitOwner.Start(); err != nil {
		t.Fatal(err)
	}

	// This is the durable boundary under test: all prerequisite snapshots have
	// committed, while no candidate attempt/dispatch has been persisted yet.
	// The panic models a lost caller response after the Run CAS, so recovery
	// cannot rely on an in-process aggregate or owner responder.
	initializer := newRuntimeDriveTestModule(t, hub)
	run, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err := initializer.initializeRuntimeFromRun(ctx, run)
	if err != nil {
		t.Fatal(err)
	}
	aggregate = runtimeTestPersistSealedSemanticAuthority(t, ctx, runtimeStateRepository{hub: hub}, aggregate)
	baseRepository := runtimeStateRepository{hub: hub}
	crashRepository := &runtimeSaveCrashRepository{
		delegate: baseRepository, after: true, marker: "git-snapshot-response-loss",
		match: func(current runtime.RuntimeAggregate) bool {
			return current.Decision.State == runtimecontract.ConvergenceStateNeedStateChange &&
				len(current.Facts.Resources.CapabilityStates) == 1 && len(current.CandidateAttempts) == 0 && current.ActiveDispatch == nil
		},
	}
	crashingBiz := newRuntimeTestOwner()
	if err := crashingBiz.ConfigureRuntimeRepository(crashRepository); err != nil {
		t.Fatal(err)
	}
	crashingModule := newTestBiz(crashingBiz, hub, nil)
	func() {
		defer func() {
			if recovered := recover(); recovered != "git-snapshot-response-loss" {
				t.Fatalf("unexpected snapshot response-loss marker: %v", recovered)
			}
		}()
		_, _ = crashingModule.prepareRuntimePrerequisites(ctx, aggregate)
		t.Fatal("expected injected snapshot response loss")
	}()
	if !crashRepository.fired {
		t.Fatal("snapshot response-loss CAS point was not reached")
	}
	committedRun, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || committedRun.RuntimeState == nil || committedRun.RuntimeState.Decision.State != runtimecontract.ConvergenceStateNeedStateChange ||
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
	if err := restartedRunOwner.Configure(runbiz.Dependencies{Runs: restartedRuns, EventHub: restartedHub}); err != nil {
		t.Fatal(err)
	}
	if err := restartedRunOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedRunOwner.Stop(ctx) })
	startRuntimeReceiptTestOwner(t, restartedHub)
	restartedWorkspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, restartedHub)
	restartedWorkspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
	})
	t.Cleanup(func() { restartedWorkspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	startRuntimeFileManifestOwner(t, ctx, restartedHub)
	reopenedStore, err := gitbiz.NewCapabilityStateReceiptStore(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	restartedGitOwner := gitmodule.New()
	if err := restartedGitOwner.Configure(gitbiz.Dependencies{EventHub: restartedHub, RuntimeAdapter: gitbiz.NewRuntimeAdapter(), CapabilityStateReceipts: reopenedStore}); err != nil {
		t.Fatal(err)
	}
	if err := restartedGitOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedGitOwner.Stop(ctx) })
	candidateCalls := 0
	firstCandidateReusedSnapshot := false
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, restartedHub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		stateChange := command.Decision.State == runtimecontract.ConvergenceStateNeedStateChange
		if stateChange {
			candidateCalls++
		}
		result.Set(runtimeCandidateResultForTest(t, command, runtimeGitCandidateForDecision(command)), nil)
		if stateChange && candidateCalls == 1 && (len(command.CandidateContext.Resources.CapabilityStates) != 1 || command.CandidateContext.Resources.CapabilityStates[0].SnapshotID != committedSnapshot.SnapshotID ||
			command.CandidateContext.Resources.CapabilityStates[0].AcquisitionKey != committedSnapshot.AcquisitionKey) {
			t.Errorf("candidate did not reuse committed capability snapshot: %+v", command.CandidateContext.Resources.CapabilityStates)
			return
		}
		if stateChange && candidateCalls == 1 {
			firstCandidateReusedSnapshot = true
		}
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })
	driveObserver := magicevent.NewSimpleObserver(ModuleID, restartedHub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntime, newRuntimeDriveTestModule(t, restartedHub).handleRuntimeDrive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntime) })
	completed, err := DriveRuntime(ctx, restartedHub, orchestratorevents.DriveRuntimeCommand{WorkspaceID: "ws-v2", RunID: "run-v2"})
	if err != nil || completed.State != runtimecontract.ConvergenceStateComplete || candidateCalls != 1 || !firstCandidateReusedSnapshot {
		t.Fatalf("restart after snapshot commit completed=%+v candidateCalls=%d err=%v", completed, candidateCalls, err)
	}
	persisted, err := restartedRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.RuntimeState == nil || len(persisted.RuntimeState.Facts.Resources.CapabilityStates) != 1 || len(persisted.RuntimeState.OperationReceipts) != 3 {
		t.Fatalf("persisted state=%+v err=%v", persisted, err)
	}
}

func TestDriveRuntimeFileBackedFreshFileShellGitAcceptance(t *testing.T) {
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
	bootstrap := runtimeFileShellGitBootstrap()
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2",
		DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive,
	}); err != nil {
		t.Fatal(err)
	}
	if err := workspaces.Create(ctx, runtimeWorkspaceForPersistedRunTest(t, ctx, runs, "ws-v2", "run-v2", deliverable, internalDir)); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	startRuntimeReceiptTestOwner(t, hub)
	workspaceOwner := workspacemodule.New()
	if err := workspaceOwner.Configure(workspacebiz.Dependencies{Workspaces: workspaces, DeletionMaterialCleaner: workspaces, Goals: memory.NewGoalRepository(), DoDs: memory.NewDoDRepository(), Templates: memory.NewWorkspaceTemplateRepository(), EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := workspaceOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = workspaceOwner.Stop(ctx) })
	fileOwner := filemodule.New()
	if err := fileOwner.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := fileOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fileOwner.Stop(ctx) })
	shellOwner := shellmodule.New()
	if err := shellOwner.Configure(shellbiz.Dependencies{EventHub: hub, RuntimeAdapter: shellbiz.NewRuntimeAdapter()}); err != nil {
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
	if err := gitOwner.Configure(gitbiz.Dependencies{EventHub: hub, RuntimeAdapter: gitbiz.NewRuntimeAdapter(), CapabilityStateReceipts: gitReceipts}); err != nil {
		t.Fatal(err)
	}
	if err := gitOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = gitOwner.Stop(ctx) })
	resultOwner := resultmodule.New()
	if err := resultOwner.Configure(resultbiz.Dependencies{OwnerRepositories: testResultOwnerRepositories(results, artifacts, memory.NewResultWorkspaceDeletionRepository()), EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := resultOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resultOwner.Stop(ctx) })
	startRuntimeProviderCeilingTestOwner(t, hub)

	candidateAttempts := map[string][]int{}
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		if command.DocumentKind == runtimecontract.RuntimeProviderDocumentSemanticPlan {
			response, responseErr := runtimeFileShellGitSemanticResult(command)
			if responseErr != nil {
				t.Errorf("semantic candidate: %v", responseErr)
				result.Set(runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{
					DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity,
					ExpectedDocumentHash: runtimeExpectedDocumentHash(command.ExpectedDocument),
					Feedback: runtimeProviderFeedbackForTest(t, command, runtimecontract.RuntimeCandidateFailure{
						Class: runtimecontract.RuntimeCandidateFailureClassContract, Code: "semantic_fixture_invalid",
						Signature: "semantic-fixture-invalid", Reason: responseErr.Error(),
					}),
				}), nil)
				return
			}
			result.Set(runtimeProviderResultWithAuditForTest(command, response), nil)
			return
		}
		focusID, action := command.Decision.Focus.ID, command.Decision.State
		coverageKey := string(command.Decision.Focus.CoverageWorkItemIDs[0])
		candidateAttempts[coverageKey+"/"+string(action)] = append(candidateAttempts[coverageKey+"/"+string(action)], command.CandidateAttempt)
		candidate, candidateErr := runtimeFileShellGitCandidate(command)
		if candidateErr != nil {
			t.Errorf("candidate for %s/%s: %v", focusID, action, candidateErr)
			failure := runtimecontract.RuntimeCandidateFailure{Class: runtimecontract.RuntimeCandidateFailureClassInternal, Reason: candidateErr.Error()}
			result.Set(runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity, ExpectedDocumentHash: runtimeExpectedDocumentHash(command.ExpectedDocument), Feedback: runtimeProviderFeedbackForTest(t, command, failure)}), nil)
			return
		}
		result.Set(runtimeCandidateResultForTest(t, command, candidate), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })
	runtimeTestPersistBootstrappedOperationFixture(t, ctx, hub, "ws-v2", "run-v2", bootstrap)
	module := newRuntimeDriveTestModule(t, hub)
	driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntime, module.handleRuntimeDrive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntime) })
	completed, err := DriveRuntime(ctx, hub, orchestratorevents.DriveRuntimeCommand{WorkspaceID: "ws-v2", RunID: "run-v2"})
	if err != nil || completed.State != runtimecontract.ConvergenceStateComplete || completed.Terminal == nil {
		current, loadErr := module.loadRuntime(ctx, "ws-v2", "run-v2")
		t.Fatalf("file-shell-git drive completion=%+v terminal=%+v aggregate=%+v loadErr=%v err=%v", completed, completed.Terminal, current, loadErr, err)
	}
	if content, readErr := os.ReadFile(filepath.Join(deliverable, "main.go")); readErr != nil || string(content) != "package main\n// edited\n" {
		t.Fatalf("fresh File sequence content=%q err=%v", string(content), readErr)
	}
	t.Logf("file-shell-git candidate attempts: %+v", candidateAttempts)
	if got := strings.TrimSpace(runtimeGitOutput(t, deliverable, "rev-list", "--count", "HEAD")); got != "1" {
		t.Fatalf("fresh Git sequence commit count=%q", got)
	}
	persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.RuntimeState == nil {
		t.Fatalf("persisted file-shell-git run=%+v err=%v", persisted, err)
	}
	state := persisted.RuntimeState
	if state.Facts.Recovery.PrerequisiteRefreshes != 0 || len(state.PrerequisiteInvalidations) < 3 {
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
	if attempts := candidateAttempts["cwi-git/"+string(runtimecontract.ConvergenceStateNeedStateChange)]; len(attempts) != 1 {
		t.Fatalf("Git sequence did not remain one sealed transaction: %+v", candidateAttempts)
	}
	for _, receipt := range state.OperationReceipts {
		if receipt.PrerequisiteStale != nil || receipt.Outcome == runtimecontract.RuntimeOperationOutcomeRejected {
			t.Fatalf("fresh positive path produced stale/rejected receipt: %+v", receipt)
		}
	}
	aggregate, err := module.loadRuntime(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	deliveryOwner := newRuntimeTestOwner()
	if err := deliveryOwner.ConfigureRuntimeRepository(runtimeStateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	delivery := newTestBiz(deliveryOwner, hub, nil)
	if err := delivery.deliverRuntimeTerminal(ctx, aggregate); err != nil {
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

func runtimeFileShellGitBootstrap() runtimecontract.RuntimeBootstrap {
	bootstrap := runtimeBootstrap()
	// This fixture deliberately performs five distinct durable effects
	// (create, edit, init, add, commit). Each can invalidate a mutable
	// prerequisite. The activation-sealed resource-change allowance covers this
	// normal progressing path; stale/no-progress recovery remains independent.
	bootstrap.Definition.CapabilityContract = []string{
		runtimecontract.StepKindFileWrite, runtimecontract.StepKindFileEdit,
		runtimecontract.RuntimeBlockKindManagedValidationSession, runtimecontract.StepKindGitWrite,
	}
	// The candidate can select a stable recipe ID only.  These fixture-specific
	// command lines are activation-sealed Shell-owner policy, not provider
	// generated operation parameters.
	bootstrap.Activation.EvidencePolicies = []runtimecontract.RuntimeEvidencePolicy{
		{
			ID: "fixture-main-go-created", AppliesTo: runtimecontract.StepKindShellCommand,
			CommandName: "sh", RecipeDeclared: true, RecipeArgs: []string{"-c", "test -f main.go && grep -q 'package main' main.go && printf 'WORKORCH_CHECK_PASSED=fixture-main-go-created\\n'"},
		},
		{
			ID: "fixture-main-go-committed", AppliesTo: runtimecontract.StepKindShellCommand,
			CommandName: "sh", RecipeDeclared: true, RecipeArgs: []string{"-c", "test -f main.go && grep -q 'edited' main.go && printf 'WORKORCH_CHECK_PASSED=fixture-main-go-committed\\n'"},
		},
	}
	bootstrap.Activation.ExecutionContract.ProfileContracts = []runtimecontract.RuntimeProfileContract{{
		ID: "test-file-shell-git",
		EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{
			{
				ID: "cwi-file", Scope: "test", TargetRole: "deliverable",
				TargetDiscoverySources: []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest},
				TargetSelectorRef:      "file/workspace_deliverable", TargetSelectorOwnerRef: runtimecontract.RuntimeModuleFile, TargetSelectorSubjectRef: "main-deliverable",
				TargetSelectorCandidateRefs: []string{"main.go"}, RequireTargetGrounding: true,
				RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
				AcceptedStepKinds:     []string{runtimecontract.StepKindFileWrite},
			},
			{
				ID: "cwi-edit", Scope: "test", TargetRole: "deliverable", TargetRefs: []string{"main.go"}, RequireTargetGrounding: true,
				RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation},
				AcceptedStepKinds:     []string{runtimecontract.StepKindFileEdit, runtimecontract.RuntimeBlockKindManagedValidationSession},
				ValidationRecipeID:    "fixture-main-go-created", RequiredCheckRefs: []string{"fixture-main-go-created"},
			},
			{
				ID: "cwi-git", Scope: "test", TargetRole: "deliverable", TargetRefs: []string{"main.go"}, RequireTargetGrounding: true,
				RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, AcceptedStepKinds: []string{runtimecontract.StepKindGitWrite},
				AcceptedOperationVariants: []runtimecontract.RuntimeOperationVariantRef{{Kind: runtimecontract.StepKindGitWrite, VariantID: "commit"}},
			},
		},
	}}
	bootstrap.DefinitionFocuses = []runtimecontract.RuntimeDefinitionFocus{
		{
			ID: "focus-file", SliceID: "slice-file", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-file"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
			TargetRole: "deliverable", TargetDiscoverySources: []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest},
			TargetSelectorRef: "file/workspace_deliverable", TargetSelectorOwnerRef: runtimecontract.RuntimeModuleFile,
			TargetSelectorSubjectRef: "main-deliverable", TargetSelectorCandidateRefs: []string{"main.go"}, RequireTargetGrounding: true,
		},
		{ID: "focus-edit", SliceID: "slice-edit", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-edit"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}},
		{ID: "focus-git", SliceID: "slice-git", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-git"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, RequireAssessment: true},
	}
	bootstrap.Slices = []runtimecontract.RuntimeAcceptanceSlice{
		{ID: "slice-file", Title: "file", Scope: "fixture", Priority: 1, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-file"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, CompletionPolicy: runtimecontract.RuntimeAcceptanceSliceCompletionAll, ResultPolicy: runtimecontract.RuntimeAcceptanceSliceResultAllowed},
		{ID: "slice-edit", Title: "edit", Scope: "fixture", Priority: 1, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-edit"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}, CompletionPolicy: runtimecontract.RuntimeAcceptanceSliceCompletionAll, ResultPolicy: runtimecontract.RuntimeAcceptanceSliceResultAllowed},
		{ID: "slice-git", Title: "git", Scope: "fixture", Priority: 1, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-git"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, CompletionPolicy: runtimecontract.RuntimeAcceptanceSliceCompletionAll, ResultPolicy: runtimecontract.RuntimeAcceptanceSliceResultAllowed},
	}
	bootstrap.CoverageItems = []runtimecontract.RuntimeCoverageItem{
		{ID: "cwi-file", SliceID: "slice-file"},
		{ID: "cwi-edit", SliceID: "slice-edit", PrerequisiteCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-file"}},
		{ID: "cwi-git", SliceID: "slice-git", PrerequisiteCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-edit"}},
	}
	bootstrap.Requirements = []runtimecontract.RuntimeRequirement{
		{Kind: runtimecontract.PrerequisiteKindProviderCeiling, OwnerModuleID: runtimecontract.RuntimeModuleLLM, MinimumBytes: 64 * 1024, Reason: "semantic planning provider ceiling"},
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
	return runtimeTestBootstrapWithBindings(bootstrap)
}

func runtimeTestUseExistingMainTarget(bootstrap *runtimecontract.RuntimeBootstrap) {
	for profileIndex := range bootstrap.Activation.ExecutionContract.ProfileContracts {
		for requirementIndex := range bootstrap.Activation.ExecutionContract.ProfileContracts[profileIndex].EvidenceRequirements {
			requirement := &bootstrap.Activation.ExecutionContract.ProfileContracts[profileIndex].EvidenceRequirements[requirementIndex]
			if requirement.ID != "cwi-file" {
				continue
			}
			requirement.TargetSelectorCandidateRefs = nil
			requirement.TargetRefs = []string{"main.go"}
			requirement.AcceptedStepKinds = []string{runtimecontract.StepKindFileEdit, runtimecontract.RuntimeBlockKindManagedValidationSession}
		}
	}
}

func startRuntimeProviderCeilingTestOwner(t *testing.T, hub magicevent.Hub) {
	t.Helper()
	observer := magicevent.NewSimpleObserverWithMatchID(
		runtimecontract.RuntimeModuleLLM+"/test-prerequisite",
		orchestratorevents.PrerequisiteCommandDestination,
		hub,
	)
	observer.Subscribe(llmevents.TopicAcquireProviderCeiling, func(event magicevent.Event, result magicevent.Result) {
		command, ok := event.Data().(llmevents.AcquireProviderCeilingCommand)
		if !ok {
			t.Errorf("provider ceiling command = %T", event.Data())
			return
		}
		ceiling := runtimecontract.ProviderCeiling{
			Selection: command.Selection, ContextWindowTokens: 65536, ProviderMaxOutputTokens: 8192,
			EffectiveMaxOutputTokens: 8192, MaxInputTokens: 57344, MaxBytes: 256 * 1024,
			CapabilityRevision: "runtime-drive-test-capability", ConfigurationRevision: "runtime-drive-test-config",
		}
		observation := runtimecontract.RuntimePrerequisiteObservation{
			PrerequisiteID: command.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindProviderCeiling,
			OwnerModuleID: runtimecontract.RuntimeModuleLLM, ContentHash: "runtime-drive-test-provider-ceiling",
			Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
				Domain: runtimecontract.RuntimeResourceProviderConfig, OwnerModuleID: runtimecontract.RuntimeModuleLLM,
				SubjectRef: command.Selection.Target.ProviderID + "/" + command.Selection.Target.ModelID,
				Revision:   ceiling.ConfigurationRevision,
			}},
		}
		observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
		result.Set(llmevents.AcquireProviderCeilingResult{
			WorkspaceID: command.WorkspaceID, RunID: command.RunID, AuthoritySnapshotID: command.AuthoritySnapshotID,
			Ceiling: ceiling, Observation: observation,
		}, nil)
	})
	t.Cleanup(func() { observer.Unsubscribe(llmevents.TopicAcquireProviderCeiling) })
}

func runtimeFileShellGitSemanticResult(command llmevents.RequestRuntimeCandidateCommand) (llmevents.RequestRuntimeCandidateResult, error) {
	document := runtimecontract.RuntimeSemanticPlanDocument{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent,
		ManifestHash:  command.SemanticManifest.Hash, DefinitionSeedHash: command.SemanticManifest.DefinitionSeedHash,
		PlanStep: command.SemanticContext.PlanStep,
	}
	switch command.SemanticContext.PlanStep {
	case runtimecontract.RuntimeSemanticPlanStepCoverageOrder:
		preferred := []common.CoverageWorkItemID{"cwi-file", "cwi-edit", "cwi-git"}
		present := make(map[common.CoverageWorkItemID]struct{}, len(command.SemanticManifest.CoverageItems))
		for _, item := range command.SemanticManifest.CoverageItems {
			present[item.ID] = struct{}{}
		}
		for _, coverageID := range preferred {
			if _, found := present[coverageID]; found {
				document.CoverageOrder = append(document.CoverageOrder, coverageID)
			}
		}
		if len(document.CoverageOrder) != len(present) {
			return llmevents.RequestRuntimeCandidateResult{}, fmt.Errorf("unexpected semantic coverage set %+v", command.SemanticManifest.CoverageItems)
		}
	case runtimecontract.RuntimeSemanticPlanStepProofBatch:
		document.BatchID = command.SemanticContext.BatchID
		document.BatchSequence = command.SemanticContext.BatchSequence
		document.BatchCoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), command.SemanticContext.BatchCoverageWorkItemIDs...)
		for _, coverageID := range document.BatchCoverageWorkItemIDs {
			proposals, err := runtimeFileShellGitSemanticProposals(coverageID)
			if err != nil {
				return llmevents.RequestRuntimeCandidateResult{}, err
			}
			document.Proposals = append(document.Proposals, proposals...)
		}
	default:
		return llmevents.RequestRuntimeCandidateResult{}, fmt.Errorf("unexpected semantic plan step %q", command.SemanticContext.PlanStep)
	}
	payload, err := runtimecontract.EncodeRuntimeSemanticPlanPayloadWithManifest(command.SemanticManifest, document)
	if err != nil {
		var violation *runtimecontract.RuntimeSemanticProposalViolationError
		if errors.As(err, &violation) {
			fact := violation.Violation()
			return llmevents.RequestRuntimeCandidateResult{}, fmt.Errorf("%w (path=%s coverage=%s actual=%s allowed=%v required=%v)", err, fact.Path, fact.CoverageWorkItemID, fact.Actual, fact.Allowed, fact.Required)
		}
		return llmevents.RequestRuntimeCandidateResult{}, err
	}
	return runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{
		DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity,
		ExpectedDocumentHash: runtimeExpectedDocumentHash(command.ExpectedDocument),
		DocumentOutcome:      runtimeCompleteDocumentOutcome(command.ExpectedDocument, payload),
	}), nil
}

func runtimeFileShellGitSemanticProposals(coverageID common.CoverageWorkItemID) ([]runtimecontract.RuntimeSemanticProofProposal, error) {
	exactTarget := runtimecontract.RuntimeOperationTargetAuthorization{
		Policy: runtimecontract.RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{"main.go"},
	}
	existingScope := func(selector string) runtimecontract.RuntimeOperationTargetAuthorization {
		return runtimecontract.RuntimeOperationTargetAuthorization{
			Policy: runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope, OwnerSelectorRefs: []string{selector},
		}
	}
	proposal := func(owner, kind, variant, evidenceKind, recipe string, effect runtimecontract.RuntimeEffectScope, target runtimecontract.RuntimeOperationTargetAuthorization) runtimecontract.RuntimeSemanticProofProposal {
		value := runtimecontract.RuntimeSemanticProofProposal{
			CoverageWorkItemID: coverageID, OwnerModuleID: owner, OperationKind: kind, OperationVariantID: variant,
			Target: target, RequiredEvidenceKinds: []string{evidenceKind}, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{effect},
			ValidationRecipeID: recipe, DeclarationRef: "file-shell-git/" + string(coverageID),
		}
		if recipe != "" {
			value.RequiredCheckRefs = []string{recipe}
		}
		return value
	}
	switch coverageID {
	case "cwi-file":
		return []runtimecontract.RuntimeSemanticProofProposal{
			proposal(runtimecontract.RuntimeModuleFile, runtimecontract.StepKindFileEdit, "", runtimecontract.EvidenceKindStateChange, "", runtimecontract.RuntimeEffectWorkspaceDurable, exactTarget),
		}, nil
	case "cwi-edit":
		return []runtimecontract.RuntimeSemanticProofProposal{
			proposal(runtimecontract.RuntimeModuleFile, runtimecontract.StepKindFileEdit, "", runtimecontract.EvidenceKindStateChange, "", runtimecontract.RuntimeEffectWorkspaceDurable, exactTarget),
			proposal(runtimecontract.RuntimeModuleShell, runtimecontract.RuntimeBlockKindManagedValidationSession, "", runtimecontract.EvidenceKindValidation, "fixture-main-go-created", runtimecontract.RuntimeEffectEphemeralSession, exactTarget),
		}, nil
	case "cwi-git":
		return []runtimecontract.RuntimeSemanticProofProposal{
			proposal(runtimecontract.RuntimeModuleGit, runtimecontract.StepKindGitWrite, "commit", runtimecontract.EvidenceKindStateChange, "", runtimecontract.RuntimeEffectWorkspaceDurable, existingScope("workspace-repository")),
		}, nil
	default:
		return nil, fmt.Errorf("unexpected semantic coverage %q", coverageID)
	}
}

func runtimeFileShellGitCandidate(command llmevents.RequestRuntimeCandidateCommand) (runtimecontract.RuntimeCandidate, error) {
	focus := command.Decision.Focus
	coverageID := focus.CoverageWorkItemIDs[0]
	if command.Decision.State == runtimecontract.ConvergenceStateNeedAssessment {
		refs := make([]string, 0)
		for _, evidence := range command.CandidateContext.Evidence.Accepted {
			if containsRuntimeCoverageID(evidence.CoverageWorkItemIDs, coverageID) {
				refs = append(refs, string(evidence.ID))
			}
		}
		return runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{
			FocusID: focus.ID, CoverageWorkItemIDs: []common.CoverageWorkItemID{coverageID},
			Disposition: runtimecontract.AssessmentDispositionSatisfied, Summary: "current coverage evidence satisfies the binding",
			Items: []runtimecontract.RuntimeAssessmentItemFact{{CoverageWorkItemID: coverageID, Satisfied: true, EvidenceRefs: refs}}, EvidenceRefs: refs,
		}}, nil
	}
	switch coverageID {
	case "cwi-file":
		switch command.Decision.State {
		case runtimecontract.ConvergenceStateNeedStateChange:
			target, content := "main.go", "package main\n"
			return runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{ID: "file-create", Kind: runtimecontract.StepKindFileWrite, CoverageWorkItemIDs: []common.CoverageWorkItemID{coverageID}, TargetRefs: []string{target}, Parameters: []runtimecontract.OperationParameter{{Name: "content", String: &content}}}}}}}, nil
		}
	case "cwi-edit":
		switch command.Decision.State {
		case runtimecontract.ConvergenceStateNeedStateChange:
			path, oldText, newText := "main.go", "package main\n", "package main\n// edited\n"
			return runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{ID: "file-edit", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{coverageID}, TargetRefs: []string{path}, Parameters: []runtimecontract.OperationParameter{{Name: "oldText", String: &oldText}, {Name: "newText", String: &newText}}}}}}}, nil
		case runtimecontract.ConvergenceStateNeedValidation:
			return runtimeManagedShellValidationCandidate("validate-created", coverageID, "main.go", "fixture-main-go-created"), nil
		}
	case "cwi-git":
		switch command.Decision.State {
		case runtimecontract.ConvergenceStateNeedStateChange:
			return runtimeGitStateCandidateForCoverage(requiredOperationRefs(command.Decision.RequiredOutput.RequiredOperationSequence), coverageID, "main.go"), nil
		case runtimecontract.ConvergenceStateNeedValidation:
			return runtimeManagedShellValidationCandidate("validate-committed", coverageID, "main.go", "fixture-main-go-committed"), nil
		}
	}
	return runtimecontract.RuntimeCandidate{}, fmt.Errorf("unexpected focus/action %s/%s", focus.ID, command.Decision.State)
}

func runtimeManagedShellValidationCandidate(id string, coverageID common.CoverageWorkItemID, target, recipeID string) runtimecontract.RuntimeCandidate {
	return runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: id, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession, CoverageWorkItemIDs: []common.CoverageWorkItemID{coverageID}, TargetRefs: []string{target},
		Parameters: []runtimecontract.OperationParameter{{Name: "recipeId", String: &recipeID}},
	}}}}}
}

func runtimeGitStateCandidateForCoverage(requirementRefs []string, coverageID common.CoverageWorkItemID, target string) runtimecontract.RuntimeCandidate {
	operations := make([]runtimecontract.StateChangingOperation, 0, len(requirementRefs))
	for _, ref := range requirementRefs {
		operation := runtimecontract.RuntimeOperation{ID: "fresh-git-" + ref, Kind: runtimecontract.StepKindGitWrite, RequiredOperationRef: ref, TargetRefs: []string{target}}
		switch ref {
		case "repo-init":
			value := "init"
			operation.Parameters = []runtimecontract.OperationParameter{{Name: "operation", String: &value}}
		case "stage-changes":
			value := "add"
			operation.Parameters = []runtimecontract.OperationParameter{{Name: "operation", String: &value}, {Name: "pathspecs", StringList: []string{target}}}
		case "create-commit":
			value, message := "commit", "fresh runtime fixture"
			operation.Parameters = []runtimecontract.OperationParameter{{Name: "operation", String: &value}, {Name: "message", String: &message}}
			operation.CoverageWorkItemIDs = []common.CoverageWorkItemID{coverageID}
		}
		switch ref {
		case "stage-changes":
			if containsRuntimeString(requirementRefs, "repo-init") {
				operation.DependsOn = []string{"fresh-git-repo-init"}
			}
		case "create-commit":
			if containsRuntimeString(requirementRefs, "stage-changes") {
				operation.DependsOn = []string{"fresh-git-stage-changes"}
			}
		}
		operations = append(operations, runtimecontract.StateChangingOperation{Operation: operation})
	}
	return runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: operations}}
}

func TestRuntimeFileBackedGitStaleRefreshCrashRestartDoesNotReplayOldDispatch(t *testing.T) {
	runRuntimeFileBackedGitStaleRefreshCrashRestart(t, true)
}

func TestRuntimeFileBackedGitStaleRefreshBeforeCASRestartDoesNotLeakInvalidation(t *testing.T) {
	runRuntimeFileBackedGitStaleRefreshCrashRestart(t, false)
}

func runRuntimeFileBackedGitStaleRefreshCrashRestart(t *testing.T, crashAfterCAS bool) {
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
	writeRuntimeGitFixtureFile(t, deliverable)
	bootstrap := runtimeGitCapabilityStateBootstrap()
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2", DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	startRuntimeReceiptTestOwner(t, hub)
	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	startRuntimeFileManifestOwner(t, ctx, hub)
	storeRoot := filepath.Join(root, "git-owner-data")
	store, err := gitbiz.NewCapabilityStateReceiptStore(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	gitOwner := gitmodule.New()
	if err := gitOwner.Configure(gitbiz.Dependencies{EventHub: hub, RuntimeAdapter: gitbiz.NewRuntimeAdapter(), CapabilityStateReceipts: store}); err != nil {
		t.Fatal(err)
	}
	if err := gitOwner.Start(); err != nil {
		t.Fatal(err)
	}

	module := newRuntimeDriveTestModule(t, hub)
	run, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err := module.initializeRuntimeFromRun(ctx, run)
	if err != nil {
		t.Fatal(err)
	}
	aggregate = runtimeTestPersistSealedSemanticAuthority(t, ctx, runtimeStateRepository{hub: hub}, aggregate)
	aggregate, err = module.prepareRuntimePrerequisites(ctx, aggregate)
	if err != nil {
		t.Fatal(err)
	}
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(aggregate.Definition, aggregate.Facts.Activation.ExecutionContract)
	if _, err := module.BeginRuntimeCandidateAttemptForTest(ctx, "ws-v2", "run-v2", 0, prefix); err != nil {
		t.Fatal(err)
	}
	initialCandidate := runtimeGitStateCandidate([]string{"create-commit", "repo-init", "stage-changes"})
	if err := recordRuntimeDriveFixtureCandidateFeasibilities(ctx, module, aggregate, 0, initialCandidate); err != nil {
		t.Fatal(err)
	}
	if _, err := module.AcceptRuntimeCandidateAttempt(ctx, "ws-v2", "run-v2", 0, initialCandidate); err != nil {
		t.Fatal(err)
	}

	// External owner state changes after the sealed absent-repository snapshot
	// and before the first operation. The old init must become a typed stale
	// receipt, never a replayable execution failure or an ordinary repair.
	runRuntimeGit(t, deliverable, "init", "-q")
	runRuntimeGit(t, deliverable, "config", "user.name", "WorkOrch Test")
	runRuntimeGit(t, deliverable, "config", "user.email", "workorch@example.test")
	baseRepository := runtimeStateRepository{hub: hub}
	crashMarker := "git-stale-refresh-before-cas"
	if crashAfterCAS {
		crashMarker = "git-stale-refresh-after-cas"
	}
	crashRepository := &runtimeSaveCrashRepository{
		delegate: baseRepository, after: crashAfterCAS, marker: crashMarker,
		match: func(current runtime.RuntimeAggregate) bool {
			return current.ActiveDispatch == nil && current.Decision.State == runtimecontract.ConvergenceStateNeedPrerequisite &&
				current.Facts.Recovery.PrerequisiteRefreshes == 1 && len(current.Facts.Resources.CapabilityStates) == 0
		},
	}
	crashingController, err := runtime.NewRuntimeController(crashRepository)
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != crashMarker {
				t.Fatalf("unexpected stale-refresh crash marker: %v", recovered)
			}
		}()
		_, _ = runtimeTestControllerExecuteDispatch(crashingController, ctx, "ws-v2", "run-v2", eventRuntimeOperationExecutor{hub: hub})
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
	if err := restartedRunOwner.Configure(runbiz.Dependencies{Runs: restartedRuns, EventHub: restartedHub}); err != nil {
		t.Fatal(err)
	}
	if err := restartedRunOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedRunOwner.Stop(ctx) })
	startRuntimeReceiptTestOwner(t, restartedHub)
	restartedWorkspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, restartedHub)
	restartedWorkspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
	})
	t.Cleanup(func() { restartedWorkspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	startRuntimeFileManifestOwner(t, ctx, restartedHub)
	reopenedStore, err := gitbiz.NewCapabilityStateReceiptStore(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	restartedGitOwner := gitmodule.New()
	if err := restartedGitOwner.Configure(gitbiz.Dependencies{EventHub: restartedHub, RuntimeAdapter: gitbiz.NewRuntimeAdapter(), CapabilityStateReceipts: reopenedStore}); err != nil {
		t.Fatal(err)
	}
	if err := restartedGitOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedGitOwner.Stop(ctx) })
	candidateCalls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, restartedHub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		if command.Decision.State == runtimecontract.ConvergenceStateNeedStateChange {
			candidateCalls++
			refs := requiredOperationRefs(command.Decision.RequiredOutput.RequiredOperationSequence)
			expected := []string{"create-commit", "stage-changes"}
			if !equalStrings(refs, expected) {
				t.Errorf("stale refresh used wrong required sequence on candidate %d: %v", candidateCalls, refs)
			}
		}
		result.Set(runtimeCandidateResultForTest(t, command, runtimeGitCandidateForDecision(command)), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })
	driveObserver := magicevent.NewSimpleObserver(ModuleID, restartedHub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntime, newRuntimeDriveTestModule(t, restartedHub).handleRuntimeDrive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntime) })
	completed, err := DriveRuntime(ctx, restartedHub, orchestratorevents.DriveRuntimeCommand{WorkspaceID: "ws-v2", RunID: "run-v2"})
	if err != nil || completed.State != runtimecontract.ConvergenceStateComplete || candidateCalls != 1 {
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
	if initReceipts != 1 || strings.TrimSpace(runtimeGitOutput(t, deliverable, "rev-list", "--count", "HEAD")) != "1" {
		t.Fatalf("old init dispatch was replayed, receipts=%d", initReceipts)
	}
}

// An accepted owner receipt that changed a prerequisite resource advances the
// affected prerequisite generation without consuming recovery budget. Restart
// must never replay the durable operation that produced the receipt.
func TestRuntimeFileBackedGitReceiptResourceChangeRefreshCrashRestartDoesNotReplayReceipt(t *testing.T) {
	runRuntimeFileBackedGitReceiptResourceChangeRefreshCrashRestart(t, true)
}

func TestRuntimeFileBackedGitReceiptResourceChangeRefreshBeforeCASRestartUsesCommittedReceipt(t *testing.T) {
	runRuntimeFileBackedGitReceiptResourceChangeRefreshCrashRestart(t, false)
}

// Resource-changing receipts never terminalize solely by count, including
// when a process loses the response around the invalidation CAS.
func TestRuntimeFileBackedGitSingleDispatchResourceRefreshDoesNotExhaust(t *testing.T) {
	for _, crashAfterCAS := range []bool{false, true} {
		phase := "before_cas"
		if crashAfterCAS {
			phase = "after_cas"
		}
		t.Run(phase, func(t *testing.T) {
			runRuntimeFileBackedGitReceiptResourceChangeRefreshCrashRestart(t, crashAfterCAS)
		})
	}
}

func runRuntimeFileBackedGitReceiptResourceChangeRefreshCrashRestart(t *testing.T, crashAfterCAS bool) {
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
	writeRuntimeGitFixtureFile(t, deliverable)
	bootstrap := runtimeGitCapabilityStateBootstrap()
	runsPath := filepath.Join(root, "runs.json")
	runs := filerepository.NewRunRepository(runsPath)
	if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2", DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	startRuntimeReceiptTestOwner(t, hub)
	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	startRuntimeFileManifestOwner(t, ctx, hub)
	storeRoot := filepath.Join(root, "git-owner-data")
	store, err := gitbiz.NewCapabilityStateReceiptStore(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	gitOwner := gitmodule.New()
	if err := gitOwner.Configure(gitbiz.Dependencies{EventHub: hub, RuntimeAdapter: gitbiz.NewRuntimeAdapter(), CapabilityStateReceipts: store}); err != nil {
		t.Fatal(err)
	}
	if err := gitOwner.Start(); err != nil {
		t.Fatal(err)
	}

	module := newRuntimeDriveTestModule(t, hub)
	run, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err := module.initializeRuntimeFromRun(ctx, run)
	if err != nil {
		t.Fatal(err)
	}
	aggregate = runtimeTestPersistSealedSemanticAuthority(t, ctx, runtimeStateRepository{hub: hub}, aggregate)
	aggregate, err = module.prepareRuntimePrerequisites(ctx, aggregate)
	if err != nil {
		t.Fatal(err)
	}
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(aggregate.Definition, aggregate.Facts.Activation.ExecutionContract)
	if _, err := module.BeginRuntimeCandidateAttemptForTest(ctx, "ws-v2", "run-v2", 0, prefix); err != nil {
		t.Fatal(err)
	}
	initialCandidate := runtimeGitStateCandidate([]string{"create-commit", "repo-init", "stage-changes"})
	if err := recordRuntimeDriveFixtureCandidateFeasibilities(ctx, module, aggregate, 0, initialCandidate); err != nil {
		t.Fatal(err)
	}
	if _, err := module.AcceptRuntimeCandidateAttempt(ctx, "ws-v2", "run-v2", 0, initialCandidate); err != nil {
		t.Fatal(err)
	}

	baseRepository := runtimeStateRepository{hub: hub}
	crashMarker := "git-receipt-resource-change-refresh-before-cas"
	if crashAfterCAS {
		crashMarker = "git-receipt-resource-change-refresh-after-cas"
	}
	crashRepository := &runtimeSaveCrashRepository{
		delegate: baseRepository, after: crashAfterCAS, marker: crashMarker,
		match: func(current runtime.RuntimeAggregate) bool {
			return current.ActiveDispatch != nil && current.Decision.State == runtimecontract.ConvergenceStateNeedStateChange &&
				len(current.Facts.Resources.CapabilityStates) == 0 && len(current.OperationReceipts) == 1 && current.EvidenceSequence == 1
		},
	}
	crashingController, err := runtime.NewRuntimeController(crashRepository)
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != crashMarker {
				t.Fatalf("unexpected resource-change crash marker: %v", recovered)
			}
		}()
		_, _ = runtimeTestControllerExecuteDispatch(crashingController, ctx, "ws-v2", "run-v2", eventRuntimeOperationExecutor{hub: hub})
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
		if crashed.RuntimeState.ActiveDispatch == nil || crashed.RuntimeState.Facts.Recovery.PrerequisiteRefreshes != 0 || len(crashed.RuntimeState.Facts.Resources.CapabilityStates) != 0 ||
			crashed.RuntimeState.EvidenceSequence != 1 || len(crashed.RuntimeState.PrerequisiteInvalidations) != 1 ||
			crashed.RuntimeState.PrerequisiteInvalidations[0].ReceiptHash == "" {
			t.Fatalf("after-CAS resource-change invalidation was not committed exactly once: %+v", crashed.RuntimeState)
		}
	} else if crashed.RuntimeState.ActiveDispatch == nil || crashed.RuntimeState.Facts.Recovery.PrerequisiteRefreshes != 0 || len(crashed.RuntimeState.Facts.Resources.CapabilityStates) != 1 ||
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
	if err := restartedRunOwner.Configure(runbiz.Dependencies{Runs: restartedRuns, EventHub: restartedHub}); err != nil {
		t.Fatal(err)
	}
	if err := restartedRunOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedRunOwner.Stop(ctx) })
	startRuntimeReceiptTestOwner(t, restartedHub)
	restartedWorkspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, restartedHub)
	restartedWorkspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
	})
	t.Cleanup(func() { restartedWorkspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	startRuntimeFileManifestOwner(t, ctx, restartedHub)
	reopenedStore, err := gitbiz.NewCapabilityStateReceiptStore(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	restartedGitOwner := gitmodule.New()
	if err := restartedGitOwner.Configure(gitbiz.Dependencies{EventHub: restartedHub, RuntimeAdapter: gitbiz.NewRuntimeAdapter(), CapabilityStateReceipts: reopenedStore}); err != nil {
		t.Fatal(err)
	}
	if err := restartedGitOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedGitOwner.Stop(ctx) })

	candidateCalls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, restartedHub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		if command.Decision.State == runtimecontract.ConvergenceStateNeedStateChange {
			candidateCalls++
		}
		result.Set(runtimeCandidateResultForTest(t, command, runtimeGitCandidateForDecision(command)), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })
	driveObserver := magicevent.NewSimpleObserver(ModuleID, restartedHub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntime, newRuntimeDriveTestModule(t, restartedHub).handleRuntimeDrive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntime) })
	completed, err := DriveRuntime(ctx, restartedHub, orchestratorevents.DriveRuntimeCommand{WorkspaceID: "ws-v2", RunID: "run-v2"})
	if err != nil || completed.State != runtimecontract.ConvergenceStateComplete || candidateCalls != 0 {
		t.Fatalf("resource-change restart completed=%+v candidateCalls=%d err=%v", completed, candidateCalls, err)
	}
	persisted, err := restartedRuns.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.RuntimeState == nil || persisted.RuntimeState.Facts.Recovery.PrerequisiteRefreshes != 0 {
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
	if initReceipts != 1 || len(persisted.RuntimeState.PrerequisiteInvalidations) != 1 || strings.TrimSpace(runtimeGitOutput(t, deliverable, "rev-list", "--count", "HEAD")) != "1" {
		t.Fatalf("resource-change restart replayed init or duplicated invalidations: receipts=%d invalidations=%+v", initReceipts, persisted.RuntimeState.PrerequisiteInvalidations)
	}
}

// A-5 combines a receipt-driven prerequisite invalidation with a local stale
// prerequisite. Only the latter consumes the bounded stale-refresh budget.
// Git's accepted init receipt first retires its external repository-state
// observations.  Only after that committed owner transition do we lose the
// orchestrator-local validation scratch.  The second transition must consume
// the read-only ledger without replaying the already durable Git operation or
// borrowing the receipt-resource-change ledger.
func TestRuntimeFileBackedMixedOwnerReceiptThenLocalScratchStaleCrashRestart(t *testing.T) {
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
			writeRuntimeGitFixtureFile(t, deliverable)
			bootstrap := runtimeGitCapabilityStateBootstrap()
			bootstrap.Requirements = append(bootstrap.Requirements, runtimecontract.RuntimeRequirement{
				Kind: runtimecontract.PrerequisiteKindValidationScratch, OwnerModuleID: runtimecontract.RuntimeModuleOrchestrator,
				Reason: "mixed-owner validation scratch",
			})
			runsPath := filepath.Join(root, "runs.json")
			if err := filerepository.NewRunRepository(runsPath).Create(ctx, &runmodel.Run{
				ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
				DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive,
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
				if err := runOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: hub}); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := runOwner.Start(); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				startRuntimeReceiptTestOwner(t, hub)
				workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
				workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
					result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{
						ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDirectory,
					}}, nil)
				})
				startRuntimeFileManifestOwner(t, ctx, hub)
				store, err := gitbiz.NewCapabilityStateReceiptStore(storeRoot)
				if err != nil {
					workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
					_ = runOwner.Stop(ctx)
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				gitOwner := gitmodule.New()
				if err := gitOwner.Configure(gitbiz.Dependencies{EventHub: hub, RuntimeAdapter: gitbiz.NewRuntimeAdapter(), CapabilityStateReceipts: store}); err != nil {
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
			owner := newRuntimeDriveTestModule(t, first.hub)
			run, err := first.runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			aggregate, err := owner.initializeRuntimeFromRun(ctx, run)
			if err != nil {
				t.Fatal(err)
			}
			aggregate = runtimeTestPersistSealedSemanticAuthority(t, ctx, runtimeStateRepository{hub: first.hub}, aggregate)
			aggregate, err = owner.prepareRuntimePrerequisites(ctx, aggregate)
			if err != nil || aggregate.Facts.Resources.ValidationScratch == nil {
				t.Fatalf("initial mixed prerequisites aggregate=%+v err=%v", aggregate, err)
			}
			committedScratch := *aggregate.Facts.Resources.ValidationScratch
			prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(aggregate.Definition, aggregate.Facts.Activation.ExecutionContract)
			if _, err := owner.BeginRuntimeCandidateAttemptForTest(ctx, "ws-v2", "run-v2", 0, prefix); err != nil {
				t.Fatal(err)
			}
			initialCandidate := runtimeGitStateCandidate([]string{"create-commit", "repo-init", "stage-changes"})
			if err := recordRuntimeDriveFixtureCandidateFeasibilities(ctx, owner, aggregate, 0, initialCandidate); err != nil {
				t.Fatal(err)
			}
			if _, err := owner.AcceptRuntimeCandidateAttempt(ctx, "ws-v2", "run-v2", 0, initialCandidate); err != nil {
				t.Fatal(err)
			}
			controller, err := runtime.NewRuntimeController(runtimeStateRepository{hub: first.hub})
			if err != nil {
				t.Fatal(err)
			}
			firstReceipt, err := runtimeTestControllerExecuteDispatch(controller, ctx, "ws-v2", "run-v2", eventRuntimeOperationExecutor{hub: first.hub})
			if err != nil {
				t.Fatal(err)
			}
			if firstReceipt.ActiveDispatch != nil || firstReceipt.Facts.Recovery.PrerequisiteRefreshes != 0 || firstReceipt.Facts.Resources.ValidationScratch == nil ||
				firstReceipt.Facts.Resources.ValidationScratch.Revision != committedScratch.Revision || len(firstReceipt.OperationReceipts) != 3 {
				t.Fatalf("external receipt did not establish the mixed-owner boundary: %+v", firstReceipt)
			}
			if err := os.RemoveAll(committedScratch.Reference); err != nil {
				t.Fatal(err)
			}
			stopProcess(first)

			// Process two sees exactly the persisted external invalidation plus a
			// missing local resource.  Fault only the local-stale CAS transition.
			second := startProcess(t)
			baseRepository := runtimeStateRepository{hub: second.hub}
			marker := "mixed-owner-local-stale-before-cas"
			if crashAfterCAS {
				marker = "mixed-owner-local-stale-after-cas"
			}
			crashRepository := &runtimeSaveCrashRepository{
				delegate: baseRepository, after: crashAfterCAS, marker: marker,
				match: func(current runtime.RuntimeAggregate) bool {
					return current.ActiveDispatch == nil && current.Facts.Resources.ValidationScratch == nil &&
						current.Facts.Recovery.PrerequisiteRefreshes == 1
				},
			}
			crashingOwner := newRuntimeTestOwner()
			if err := crashingOwner.ConfigureRuntimeRepository(crashRepository); err != nil {
				t.Fatal(err)
			}
			crashing := newTestBiz(crashingOwner, second.hub, nil)
			loaded, err := crashing.loadRuntime(ctx, "ws-v2", "run-v2")
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
					current, loadErr := crashing.loadRuntime(ctx, "ws-v2", "run-v2")
					t.Fatalf("mixed-owner local stale CAS was not reached: loaded=%+v current=%+v err=%v", loaded, current, loadErr)
				}()
				_, _ = crashing.driveRuntimeDecision(ctx, loaded, "stable-prefix-test")
			}()
			if !crashRepository.fired {
				t.Fatal("mixed-owner local stale CAS point was not reached")
			}
			crashed, err := second.runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil || crashed.RuntimeState == nil {
				t.Fatalf("mixed-owner stale crash state=%+v err=%v", crashed, err)
			}
			if crashAfterCAS {
				if crashed.RuntimeState.Facts.Resources.ValidationScratch != nil || crashed.RuntimeState.Facts.Recovery.PrerequisiteRefreshes != 1 {
					t.Fatalf("after-CAS mixed stale did not preserve independent ledgers: %+v", crashed.RuntimeState)
				}
			} else if crashed.RuntimeState.Facts.Resources.ValidationScratch == nil || crashed.RuntimeState.Facts.Recovery.PrerequisiteRefreshes != 0 {
				t.Fatalf("before-CAS mixed stale leaked local invalidation: %+v", crashed.RuntimeState)
			}
			if len(crashed.RuntimeState.CandidateRequestSessions) != 1 || len(crashed.RuntimeState.CandidateAttempts) != 1 || crashed.RuntimeState.ActiveDispatch != nil ||
				len(crashed.RuntimeState.GovernanceRequests) != 0 || len(crashed.RuntimeState.OperationReceipts) != 3 {
				t.Fatalf("mixed stale replayed a candidate, dispatch, ticket, or operation: %+v", crashed.RuntimeState)
			}
			stopProcess(second)

			// A fresh process either persists the one missing stale transition or
			// merely reacquires it.  In both cases the Git receipt is retained
			// exactly once and no provider/operation path is entered.
			third := startProcess(t)
			defer stopProcess(third)
			restarted := newRuntimeDriveTestModule(t, third.hub)
			loaded, err = restarted.loadRuntime(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			if !crashAfterCAS {
				loaded, err = restarted.driveRuntimeDecision(ctx, loaded, "stable-prefix-test")
				if err != nil {
					t.Fatal(err)
				}
			}
			resumed, err := restarted.prepareRuntimePrerequisites(ctx, loaded)
			if err != nil {
				t.Fatal(err)
			}
			if resumed.Facts.Resources.ValidationScratch == nil || resumed.Facts.Resources.ValidationScratch.Reference != committedScratch.Reference ||
				resumed.Facts.Resources.ValidationScratch.Revision == committedScratch.Revision || resumed.Facts.Recovery.PrerequisiteRefreshes != 1 || len(resumed.Facts.Prerequisites.Pending) != 0 ||
				len(resumed.CandidateRequestSessions) != 1 || len(resumed.CandidateAttempts) != 1 || resumed.ActiveDispatch != nil ||
				len(resumed.GovernanceRequests) != 0 || len(resumed.OperationReceipts) != 3 {
				t.Fatalf("mixed-owner restart did not converge only prerequisite facts: %+v", resumed)
			}
			if info, statErr := os.Stat(resumed.Facts.Resources.ValidationScratch.Reference); statErr != nil || !info.IsDir() {
				t.Fatalf("reacquired mixed-owner scratch = %v, info=%+v", statErr, info)
			}
		})
	}
}

func runtimeGitCapabilityStateBootstrap() runtimecontract.RuntimeBootstrap {
	bootstrap := runtimeBootstrap()
	bootstrap.Definition.CapabilityContract = []string{runtimecontract.StepKindGitWrite}
	bootstrap.Activation.ExecutionContract.ProfileContracts = []runtimecontract.RuntimeProfileContract{{
		ID: "test-git-proof",
		EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{
			ID: "git-commit", Scope: "test", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
			AcceptedStepKinds:         []string{runtimecontract.StepKindGitWrite},
			AcceptedOperationVariants: []runtimecontract.RuntimeOperationVariantRef{{Kind: runtimecontract.StepKindGitWrite, VariantID: "commit"}},
		}},
	}}
	bootstrap.Requirements = []runtimecontract.RuntimeRequirement{
		{Kind: runtimecontract.PrerequisiteKindCapabilityState, OwnerModuleID: runtimecontract.RuntimeModuleGit, CapabilityStateKind: "repository_state", SubjectRef: "workspace_deliverable", Reason: "repository state required"},
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleGit, CapabilityClass: runtimecontract.RuntimeCapabilityClassStateChange, RequiredCapabilities: []string{runtimecontract.StepKindGitWrite}, Reason: "git write capability required"},
		{Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleGit, RequiredCapabilities: []string{runtimecontract.StepKindGitWrite}, Reason: "git owner inventory"},
	}
	return runtimeTestBootstrapWithBindings(bootstrap)
}

func runtimeGitStateCandidate(requirementRefs []string) runtimecontract.RuntimeCandidate {
	operations := make([]runtimecontract.StateChangingOperation, 0, len(requirementRefs))
	for _, ref := range requirementRefs {
		operation := runtimecontract.RuntimeOperation{ID: "git-" + ref, Kind: runtimecontract.StepKindGitWrite, RequiredOperationRef: ref, TargetRefs: []string{"target.txt"}}
		switch ref {
		case "repo-init":
			value := "init"
			operation.Parameters = []runtimecontract.OperationParameter{{Name: "operation", String: &value}}
		case "stage-changes":
			value := "add"
			operation.Parameters = []runtimecontract.OperationParameter{{Name: "operation", String: &value}, {Name: "pathspecs", StringList: []string{"target.txt"}}}
		case "create-commit":
			value, message := "commit", "runtime fixture"
			operation.Parameters = []runtimecontract.OperationParameter{{Name: "operation", String: &value}, {Name: "message", String: &message}}
			operation.CoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-v2"}
		}
		switch ref {
		case "stage-changes":
			if containsRuntimeString(requirementRefs, "repo-init") {
				operation.DependsOn = []string{"git-repo-init"}
			}
		case "create-commit":
			if containsRuntimeString(requirementRefs, "stage-changes") {
				operation.DependsOn = []string{"git-stage-changes"}
			}
		}
		operations = append(operations, runtimecontract.StateChangingOperation{Operation: operation})
	}
	return runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: operations}}
}

func runtimeGitCandidateForDecision(command llmevents.RequestRuntimeCandidateCommand) runtimecontract.RuntimeCandidate {
	if command.Decision.State != runtimecontract.ConvergenceStateNeedAssessment {
		return runtimeGitStateCandidate(requiredOperationRefs(command.Decision.RequiredOutput.RequiredOperationSequence))
	}
	coverageIDs := append([]common.CoverageWorkItemID(nil), command.Decision.Focus.CoverageWorkItemIDs...)
	refs := make([]string, 0, len(command.CandidateContext.Evidence.Accepted))
	items := make([]runtimecontract.RuntimeAssessmentItemFact, 0, len(coverageIDs))
	for _, evidence := range command.CandidateContext.Evidence.Accepted {
		for _, coverageID := range evidence.CoverageWorkItemIDs {
			if containsRuntimeCoverageID(coverageIDs, coverageID) {
				refs = append(refs, string(evidence.ID))
				break
			}
		}
	}
	for _, coverageID := range coverageIDs {
		items = append(items, runtimecontract.RuntimeAssessmentItemFact{
			CoverageWorkItemID: coverageID,
			Satisfied:          true,
			EvidenceRefs:       append([]string(nil), refs...),
		})
	}
	return runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{
		FocusID: command.Decision.Focus.ID, CoverageWorkItemIDs: coverageIDs,
		Disposition: runtimecontract.AssessmentDispositionSatisfied,
		Summary:     "git durable effect satisfies the current coverage binding",
		Items:       items, EvidenceRefs: refs,
	}}
}

func runtimeSatisfiedAssessmentCandidate(command llmevents.RequestRuntimeCandidateCommand, summary string) runtimecontract.RuntimeCandidate {
	coverageIDs := append([]common.CoverageWorkItemID(nil), command.Decision.Focus.CoverageWorkItemIDs...)
	evidenceRefs := make([]string, 0, len(command.CandidateContext.Evidence.Accepted))
	for _, evidence := range command.CandidateContext.Evidence.Accepted {
		if len(evidence.CoverageWorkItemIDs) == 0 {
			continue
		}
		for _, coverageID := range evidence.CoverageWorkItemIDs {
			if containsRuntimeCoverageID(coverageIDs, coverageID) {
				evidenceRefs = append(evidenceRefs, string(evidence.ID))
				break
			}
		}
	}
	items := make([]runtimecontract.RuntimeAssessmentItemFact, 0, len(coverageIDs))
	for _, coverageID := range coverageIDs {
		items = append(items, runtimecontract.RuntimeAssessmentItemFact{
			CoverageWorkItemID: coverageID, Satisfied: true, EvidenceRefs: append([]string(nil), evidenceRefs...),
		})
	}
	return runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{
		FocusID: command.Decision.Focus.ID, CoverageWorkItemIDs: coverageIDs,
		Disposition: runtimecontract.AssessmentDispositionSatisfied, Summary: summary,
		Items: items, EvidenceRefs: evidenceRefs,
	}}
}

func recordRuntimeDriveFixtureCandidateFeasibilities(ctx context.Context, module *Biz, aggregate runtime.RuntimeAggregate, attempt int, candidate runtimecontract.RuntimeCandidate) error {
	if module == nil || candidate.StateChange == nil {
		return common.ErrInvalidArgument
	}
	if _, err := module.AdmitRuntimeCandidateProposalForTest(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, attempt, candidate); err != nil {
		return err
	}
	for _, candidateOperation := range candidate.StateChange.Operations {
		operation := candidateOperation.Operation.Normalized()
		descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
		if !found {
			return fmt.Errorf("fixture operation %q has unknown kind %q", operation.ID, operation.Kind)
		}
		var inventory runtimecontract.RuntimeOperationAdmissibility
		for _, current := range aggregate.Facts.Resources.OperationAdmissibilities {
			if current.OwnerModuleID == descriptor.OwnerModuleID && containsRuntimeString(current.OperationKinds, operation.Kind) {
				inventory = current
				break
			}
		}
		if inventory.SnapshotID == "" {
			return fmt.Errorf("fixture operation %q has no %s admissibility inventory", operation.ID, descriptor.OwnerModuleID)
		}
		if _, err := module.RecordRuntimeCandidateFeasibility(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, attempt, runtimecontract.RuntimeCandidateFeasibilityRecord{
			OperationID: operation.ID, AdmissibilitySnapshotID: inventory.SnapshotID, AcquisitionBinding: inventory.AcquisitionBinding,
			Observation: &runtimecontract.RuntimeOwnerFeasibilityObservation{Outcome: runtimecontract.RuntimeOwnerFeasibilityFeasible},
		}); err != nil {
			return err
		}
	}
	return nil
}

func containsRuntimeCoverageID(values []common.CoverageWorkItemID, want common.CoverageWorkItemID) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsRuntimeString(values []string, want string) bool {
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

func writeRuntimeGitFixtureFile(t *testing.T, deliverable string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(deliverable, "target.txt"), []byte("runtime\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runRuntimeGit(t *testing.T, directory string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = directory
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
}

func runtimeGitOutput(t *testing.T, directory string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = directory
	output, err := command.Output()
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return string(output)
}

func TestRuntimeFileBackedPatchVariantMatrix(t *testing.T) {
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
				baseHash := fmt.Sprintf("%x", sha256.Sum256([]byte("old\n")))
				return runtimePatchCandidate("patch-file-write", target, []runtimecontract.OperationParameter{{Name: "patchType", String: &patchType}, {Name: "content", String: &content}, {Name: "baseHash", String: &baseHash}})
			},
		},
		{
			name: "text_replace", target: "replace.txt", initial: "old\n", variant: "text_replace", want: "new\n",
			candidate: func() runtimecontract.RuntimeCandidate {
				patchType, oldText, newText, target := "text_replace", "old\n", "new\n", "replace.txt"
				return runtimePatchCandidate("patch-text-replace", target, []runtimecontract.OperationParameter{{Name: "patchType", String: &patchType}, {Name: "oldText", String: &oldText}, {Name: "newText", String: &newText}})
			},
		},
		{
			name: "unified_diff", target: "diff.txt", initial: "one\ntwo\nthree\n", variant: "unified_diff", want: "one\nTWO\nthree\n",
			candidate: func() runtimecontract.RuntimeCandidate {
				patchType, target := "unified_diff", "diff.txt"
				diff := "--- a/diff.txt\n+++ b/diff.txt\n@@ -1,3 +1,3 @@\n one\n-two\n+TWO\n three\n"
				return runtimePatchCandidate("patch-unified-diff", target, []runtimecontract.OperationParameter{{Name: "patchType", String: &patchType}, {Name: "diff", String: &diff}})
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

			bootstrap := runtimePatchVariantBootstrap(testCase.variant, testCase.target)
			runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
			if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2", DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive}); err != nil {
				t.Fatal(err)
			}
			runOwner := runmodule.New()
			if err := runOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: hub}); err != nil {
				t.Fatal(err)
			}
			if err := runOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = runOwner.Stop(ctx) })
			startRuntimeReceiptTestOwner(t, hub)
			workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
			workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
				result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
			})
			t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
			fileOwner := filemodule.New()
			if err := fileOwner.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
				t.Fatal(err)
			}
			if err := fileOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = fileOwner.Stop(ctx) })
			llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
			llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
				command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
				if command.Decision.State == runtimecontract.ConvergenceStateNeedAssessment {
					result.Set(runtimeCandidateResultForTest(t, command, runtimeSatisfiedAssessmentCandidate(command, "patch evidence satisfies current coverage")), nil)
					return
				}
				result.Set(runtimeCandidateResultForTest(t, command, testCase.candidate()), nil)
			})
			t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })
			runtimeTestPersistBootstrappedOperationFixture(t, ctx, hub, "ws-v2", "run-v2", bootstrap)
			module := newRuntimeDriveTestModule(t, hub)
			driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
			driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntime, module.handleRuntimeDrive)
			t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntime) })
			completed, err := DriveRuntime(ctx, hub, orchestratorevents.DriveRuntimeCommand{WorkspaceID: "ws-v2", RunID: "run-v2"})
			if err != nil || completed.State != runtimecontract.ConvergenceStateComplete {
				t.Fatalf("completed=%+v terminal=%+v err=%v", completed, completed.Terminal, err)
			}
			content, err := os.ReadFile(filepath.Join(deliverable, testCase.target))
			if err != nil || string(content) != testCase.want {
				t.Fatalf("content=%q err=%v want=%q", content, err, testCase.want)
			}
		})
	}
}

func runtimePatchVariantBootstrap(variant, target string) runtimecontract.RuntimeBootstrap {
	bootstrap := runtimeBootstrap()
	bootstrap.Definition.CapabilityContract = []string{runtimecontract.RuntimeBlockKindPatchApply}
	bootstrap.Activation.ExecutionContract.ProfileContracts = []runtimecontract.RuntimeProfileContract{{
		ID: "test-patch-profile",
		EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{
			ID: "cwi-v2", Scope: "test", TargetRole: "patch_target", TargetRefs: []string{target}, RequireTargetGrounding: true,
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
			AcceptedStepKinds:     []string{runtimecontract.RuntimeBlockKindPatchApply},
			AcceptedOperationVariants: []runtimecontract.RuntimeOperationVariantRef{{
				Kind: runtimecontract.RuntimeBlockKindPatchApply, VariantID: variant,
			}},
		}},
	}}
	bootstrap.DefinitionFocuses[0].TargetRole = "patch_target"
	bootstrap.DefinitionFocuses[0].TargetDiscoverySources = []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest}
	bootstrap.DefinitionFocuses[0].TargetSelectorRef = "file/profile_declared_candidates"
	bootstrap.DefinitionFocuses[0].TargetSelectorOwnerRef = "test-patch-profile"
	bootstrap.DefinitionFocuses[0].TargetSelectorSubjectRef = "patch-target"
	bootstrap.DefinitionFocuses[0].TargetSelectorCandidateRefs = []string{target}
	bootstrap.DefinitionFocuses[0].RequireTargetGrounding = true
	bootstrap.Requirements = []runtimecontract.RuntimeRequirement{
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleFile, CapabilityClass: runtimecontract.RuntimeCapabilityClassStateChange, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindPatchApply}, Reason: "patch capability required"},
		{Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleFile, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindPatchApply}, Reason: "patch owner inventory required"},
	}
	return runtimeTestBootstrapWithBindings(bootstrap)
}

func runtimePatchCandidate(id, target string, parameters []runtimecontract.OperationParameter) runtimecontract.RuntimeCandidate {
	return runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{ID: id, Kind: runtimecontract.RuntimeBlockKindPatchApply, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{target}, Parameters: parameters}}}}}
}

func TestRuntimeFileBackedPatchReplayProofRestartsAfterOwnerReceiptCASFailure(t *testing.T) {
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
				baseHash := fmt.Sprintf("%x", sha256.Sum256([]byte("old\n")))
				return runtimePatchCandidate("patch-file-write", target, []runtimecontract.OperationParameter{{Name: "patchType", String: &patchType}, {Name: "content", String: &content}, {Name: "baseHash", String: &baseHash}})
			},
		},
		{
			name: "text_replace", target: "written.txt", initial: "old\n", variant: "text_replace", operationID: "patch-text-replace", want: "written\n",
			candidate: func(target string) runtimecontract.RuntimeCandidate {
				patchType, oldText, newText := "text_replace", "old\n", "written\n"
				return runtimePatchCandidate("patch-text-replace", target, []runtimecontract.OperationParameter{{Name: "patchType", String: &patchType}, {Name: "oldText", String: &oldText}, {Name: "newText", String: &newText}})
			},
		},
		{
			name: "unified_diff", target: "written.txt", initial: "old\n", variant: "unified_diff", operationID: "patch-unified-diff", want: "written\n",
			candidate: func(target string) runtimecontract.RuntimeCandidate {
				patchType := "unified_diff"
				diff := "--- a/written.txt\n+++ b/written.txt\n@@ -1,1 +1,1 @@\n-old\n+written\n"
				return runtimePatchCandidate("patch-unified-diff", target, []runtimecontract.OperationParameter{{Name: "patchType", String: &patchType}, {Name: "diff", String: &diff}})
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			testRuntimeFileBackedPatchReplayProofRestartsAfterOwnerReceiptCASFailure(t, testCase.target, testCase.initial, testCase.variant, testCase.operationID, testCase.want, testCase.candidate)
		})
	}
}

func testRuntimeFileBackedPatchReplayProofRestartsAfterOwnerReceiptCASFailure(t *testing.T, target, initial, variant, operationID, want string, candidateForTarget func(string) runtimecontract.RuntimeCandidate) {
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
	bootstrap := runtimePatchVariantBootstrap(variant, target)
	// Make the continuation cross the P4 assessment boundary as well as the
	// P3 owner-receipt boundary. Other patch-variant tests intentionally keep
	// their bootstrap validation-free, so scope this requirement to this
	// receipt-loss recovery sequence.
	bootstrap.DefinitionFocuses[0].RequireAssessment = true
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2", DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deliverable, target), []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	fileOwner := filemodule.New()
	if err := fileOwner.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := fileOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fileOwner.Stop(ctx) })
	baseRepository := runtimeStateRepository{hub: hub}
	failingRepository := &failWhenRuntimeStateRepository{
		RuntimeStateRepository: baseRepository, err: errors.New("injected owner receipt CAS failure"),
		match: func(aggregate runtime.RuntimeAggregate) bool {
			if aggregate.ActiveDispatch == nil || len(aggregate.OperationReceipts) != 1 || aggregate.EvidenceSequence != 0 {
				return false
			}
			execution, found := runtimeDriveOperationExecutionRecord(aggregate.ActiveDispatch.OperationExecutions, operationID)
			return found && execution.State == runtimecontract.OperationExecutionOwnerCommitted
		},
	}
	crashingBiz := newRuntimeTestOwner()
	if err := crashingBiz.ConfigureRuntimeRepository(failingRepository); err != nil {
		t.Fatal(err)
	}
	crashingModule := newTestBiz(crashingBiz, hub, nil)
	receiptObserver := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	receiptObserver.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, crashingModule.handleRuntimeOperationReceipt)
	receiptObserver.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, crashingModule.handleRuntimeOperationReceipt)

	run, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err := crashingBiz.initializeRuntimeFromRun(ctx, run)
	if err != nil {
		t.Fatal(err)
	}
	aggregate = runtimeTestPersistSealedSemanticAuthority(t, ctx, baseRepository, aggregate)
	aggregate, err = crashingModule.prepareRuntimePrerequisites(ctx, aggregate)
	if err != nil {
		t.Fatal(err)
	}
	candidate := candidateForTarget(target)
	providerCalls := 0
	stateChangeCalls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		providerCalls++
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		if command.Decision.State == runtimecontract.ConvergenceStateNeedAssessment {
			evidenceRefs := make([]string, 0, len(command.CandidateContext.Evidence.Accepted))
			for _, evidence := range command.CandidateContext.Evidence.Accepted {
				evidenceRefs = append(evidenceRefs, string(evidence.ID))
			}
			candidate := runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{
				FocusID: command.Decision.Focus.ID, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, Disposition: runtimecontract.AssessmentDispositionSatisfied,
				Summary: "replay-proof durable patch assessed", Items: []runtimecontract.RuntimeAssessmentItemFact{{CoverageWorkItemID: "cwi-v2", Satisfied: true, EvidenceRefs: evidenceRefs}}, EvidenceRefs: evidenceRefs,
			}}
			result.Set(runtimeCandidateResultForTest(t, command, candidate), nil)
			return
		}
		if command.Decision.State == runtimecontract.ConvergenceStateNeedStateChange {
			stateChangeCalls++
		}
		result.Set(runtimeCandidateResultForTest(t, command, candidate), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(aggregate.Definition, aggregate.Facts.Activation.ExecutionContract)
	aggregate, err = crashingModule.requestAndSealRuntimeCandidateWithRetry(ctx, aggregate, prefix)
	if err != nil || aggregate.ActiveDispatch == nil {
		t.Fatalf("seal replay-proof dispatch aggregate=%+v err=%v", aggregate, err)
	}
	sealedDispatchID := aggregate.ActiveDispatch.ID
	sealedExecution, found := runtimeDriveOperationExecutionRecord(aggregate.ActiveDispatch.OperationExecutions, operationID)
	if !found || sealedExecution.ReplayProof == nil {
		t.Fatalf("patch dispatch has no sealed replay proof: %+v", sealedExecution)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "runtime-test-crash-after-owner-receipt-commit-error" {
				t.Fatalf("unexpected crash marker: %v", recovered)
			}
		}()
		_, _ = crashingBiz.ExecuteRuntimeDispatch(ctx, "ws-v2", "run-v2", panicAfterOwnerReceiptCommitErrorExecutor{delegate: eventRuntimeOperationExecutor{hub: hub}})
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
	claimed, found := runtimeDriveOperationExecutionRecord(crashed.RuntimeState.ActiveDispatch.OperationExecutions, operationID)
	if !found || claimed.State != runtimecontract.OperationExecutionClaimed || claimed.Fence == 0 || claimed.ExecutionKey != sealedExecution.ExecutionKey || claimed.ReplayProof == nil {
		t.Fatalf("receipt-CAS crash lost claimed proof execution=%+v", claimed)
	}

	receiptObserver.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
	receiptObserver.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
	llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate)
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
	if err := restartedRunOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: restartedHub}); err != nil {
		t.Fatal(err)
	}
	if err := restartedRunOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedRunOwner.Stop(ctx) })
	restartedWorkspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, restartedHub)
	restartedWorkspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
	})
	t.Cleanup(func() { restartedWorkspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	restartedFileOwner := filemodule.New()
	if err := restartedFileOwner.Configure(filebiz.Dependencies{EventHub: restartedHub}); err != nil {
		t.Fatal(err)
	}
	if err := restartedFileOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedFileOwner.Stop(ctx) })
	restartedLLMObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, restartedHub)
	restartedLLMObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		providerCalls++
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		if command.Decision.State == runtimecontract.ConvergenceStateNeedAssessment {
			evidenceRefs := make([]string, 0, len(command.CandidateContext.Evidence.Accepted))
			for _, evidence := range command.CandidateContext.Evidence.Accepted {
				evidenceRefs = append(evidenceRefs, string(evidence.ID))
			}
			assessment := runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{
				FocusID: command.Decision.Focus.ID, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, Disposition: runtimecontract.AssessmentDispositionSatisfied,
				Summary: "replay-proof durable patch assessed", Items: []runtimecontract.RuntimeAssessmentItemFact{{CoverageWorkItemID: "cwi-v2", Satisfied: true, EvidenceRefs: evidenceRefs}}, EvidenceRefs: evidenceRefs,
			}}
			result.Set(runtimeCandidateResultForTest(t, command, assessment), nil)
			return
		}
		if command.Decision.State == runtimecontract.ConvergenceStateNeedStateChange {
			stateChangeCalls++
		}
		result.Set(runtimeCandidateResultForTest(t, command, candidate), nil)
	})
	t.Cleanup(func() { restartedLLMObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })
	restartedRepository := runtimeStateRepository{hub: restartedHub}
	restartedBiz := newRuntimeTestOwner()
	if err := restartedBiz.ConfigureRuntimeRepository(restartedRepository); err != nil {
		t.Fatal(err)
	}
	restartedModule := newTestBiz(restartedBiz, restartedHub, nil)
	restartedReceiptObserver := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, restartedHub)
	restartedReceiptObserver.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, restartedModule.handleRuntimeOperationReceipt)
	restartedReceiptObserver.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, restartedModule.handleRuntimeOperationReceipt)
	t.Cleanup(func() {
		restartedReceiptObserver.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
		restartedReceiptObserver.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
	})
	completed, err := restartedBiz.ExecuteRuntimeDispatch(ctx, "ws-v2", "run-v2", eventRuntimeOperationExecutor{hub: restartedHub})
	if err != nil || completed.ActiveDispatch != nil || completed.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite || completed.Facts.Recovery.OperationRecoveryAttempts != 1 || len(completed.OperationReceipts) != 1 || len(completed.Facts.Evidence.Accepted) != 1 {
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
	var sealedDispatch runtime.SealedDispatch
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
	duplicate, err := (eventRuntimeOperationExecutor{hub: restartedHub}).ExecuteRuntimeOperation(ctx, runtime.RuntimeOperationExecutionRequest{
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
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntime, restartedModule.handleRuntimeDrive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntime) })
	terminal, err := DriveRuntime(ctx, restartedHub, orchestratorevents.DriveRuntimeCommand{WorkspaceID: "ws-v2", RunID: "run-v2"})
	if err != nil || terminal.State != runtimecontract.ConvergenceStateComplete || terminal.Terminal == nil {
		t.Fatalf("receipt-loss continuation did not commit terminal authority result=%+v err=%v", terminal, err)
	}
	if providerCalls != 2 || stateChangeCalls != 1 {
		t.Fatalf("receipt-loss continuation provider calls=%d state-change calls=%d, want one initial patch and one assessment", providerCalls, stateChangeCalls)
	}
	terminalAggregate, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || terminalAggregate.RuntimeState == nil || terminalAggregate.RuntimeState.ActiveDispatch != nil ||
		terminalAggregate.RuntimeState.Decision.State != runtimecontract.ConvergenceStateComplete || terminalAggregate.RuntimeState.Decision.Terminal == nil ||
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
	duplicateTerminal, duplicateReceipt, err := restartedBiz.CommitRuntimeOperationReceipt(
		ctx, "ws-v2", "run-v2", sealedDispatchID, finalReceipt.Fence, finalReceipt,
	)
	if err != nil || duplicateTerminal.StateRevision != terminalAggregate.RuntimeState.StateRevision ||
		!runtimecontract.SameRuntimeOperationReceipt(duplicateReceipt, finalReceipt) {
		t.Fatalf("duplicate receipt after terminal aggregate=%+v receipt=%+v err=%v", duplicateTerminal, duplicateReceipt, err)
	}
	tamperedReceipt := finalReceipt.Clone()
	tamperedReceipt.Evidence.Summary = "tampered delayed receipt"
	if _, _, err := restartedBiz.CommitRuntimeOperationReceipt(ctx, "ws-v2", "run-v2", sealedDispatchID, tamperedReceipt.Fence, tamperedReceipt); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("different delayed receipt after terminal err=%v, want conflict", err)
	}
	immutableTerminal, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || immutableTerminal.RuntimeState == nil ||
		immutableTerminal.RuntimeState.StateRevision != terminalAggregate.RuntimeState.StateRevision ||
		immutableTerminal.RuntimeState.Decision.State != runtimecontract.ConvergenceStateComplete ||
		len(immutableTerminal.RuntimeState.OperationReceipts) != 1 || len(immutableTerminal.RuntimeState.Facts.Evidence.Accepted) != 1 ||
		providerCalls != 2 || stateChangeCalls != 1 {
		t.Fatalf("late receipt changed terminal authority=%+v provider=%d stateChanges=%d err=%v", immutableTerminal, providerCalls, stateChangeCalls, err)
	}

	// Carry the recovered P3 receipt through a P5 owner acknowledgement loss.
	// The workspace observer was only the File prerequisite adapter; terminal
	// projection below deliberately uses the production Workspace owner.
	restartedWorkspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
	workspaces := filerepository.NewWorkspaceRepository(filepath.Join(root, "workspaces.json"))
	if err := workspaces.Create(ctx, runtimeWorkspaceForPersistedRunTest(t, ctx, runs, "ws-v2", "run-v2", deliverable, internalDir)); err != nil {
		t.Fatal(err)
	}
	workspaceOwner := workspacemodule.New()
	if err := workspaceOwner.Configure(workspacebiz.Dependencies{
		Workspaces: workspaces, DeletionMaterialCleaner: workspaces, Goals: memory.NewGoalRepository(), DoDs: memory.NewDoDRepository(),
		Templates: memory.NewWorkspaceTemplateRepository(), EventHub: restartedHub,
	}); err != nil {
		t.Fatal(err)
	}
	if err := workspaceOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = workspaceOwner.Stop(ctx) })
	deliveryOwner := newRuntimeTestOwner()
	if err := deliveryOwner.ConfigureRuntimeRepository(runtimeStateRepository{hub: restartedHub}); err != nil {
		t.Fatal(err)
	}
	deliveryModule := newTestBiz(deliveryOwner, restartedHub, nil)
	results := filerepository.NewResultRepository(filepath.Join(root, "results.json"))
	interruptedResults := &failAfterResultCreateRepository{ResultRepository: results}
	firstResultOwner := resultmodule.New()
	if err := firstResultOwner.Configure(resultbiz.Dependencies{OwnerRepositories: testResultOwnerRepositories(
		interruptedResults, filerepository.NewArtifactRepository(filepath.Join(root, "artifacts.json")), memory.NewResultWorkspaceDeletionRepository()), EventHub: restartedHub}); err != nil {
		t.Fatal(err)
	}
	if err := firstResultOwner.Start(); err != nil {
		t.Fatal(err)
	}
	if err := deliveryModule.deliverRuntimeTerminal(ctx, runtimeAggregateFromRecord(*terminalAggregate.RuntimeState)); err == nil {
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
	if err := restartedResultOwner.Configure(resultbiz.Dependencies{OwnerRepositories: testResultOwnerRepositories(
		results, filerepository.NewArtifactRepository(filepath.Join(root, "artifacts.json")), memory.NewResultWorkspaceDeletionRepository()), EventHub: restartedHub}); err != nil {
		t.Fatal(err)
	}
	if err := restartedResultOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedResultOwner.Stop(ctx) })
	persistedTerminal, err := restartedBiz.loadRuntime(ctx, "ws-v2", "run-v2")
	if err != nil || persistedTerminal.StateRevision != terminalAggregate.RuntimeState.StateRevision || persistedTerminal.Decision.State != runtimecontract.ConvergenceStateComplete {
		t.Fatalf("P3/P5 restart did not retain terminal authority=%+v err=%v", persistedTerminal, err)
	}
	if err := deliveryModule.deliverRuntimeTerminal(ctx, persistedTerminal); err != nil {
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
	finalAggregate, err := restartedBiz.loadRuntime(ctx, "ws-v2", "run-v2")
	if err != nil || finalAggregate.StateRevision != terminalAggregate.RuntimeState.StateRevision+3 || len(finalAggregate.DeliveryAcknowledgements) != 3 ||
		len(finalAggregate.OperationReceipts) != 1 || len(finalAggregate.Facts.Evidence.Accepted) != 1 ||
		providerCalls != 2 || stateChangeCalls != 1 {
		t.Fatalf("P3/P5 replay rewrote authority=%+v provider=%d stateChanges=%d err=%v", finalAggregate, providerCalls, stateChangeCalls, err)
	}
}

func runtimeDriveOperationExecutionRecord(values []runtimecontract.RuntimeOperationExecutionRecord, operationID string) (runtimecontract.RuntimeOperationExecutionRecord, bool) {
	for _, value := range values {
		if value.OperationID == operationID {
			return value, true
		}
	}
	return runtimecontract.RuntimeOperationExecutionRecord{}, false
}

func TestRuntimeFileBackedPatchVariantRestartAfterOwnerReceipt(t *testing.T) {
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
				baseHash := fmt.Sprintf("%x", sha256.Sum256([]byte("old\n")))
				return runtimePatchCandidate("patch-file-write", target, []runtimecontract.OperationParameter{{Name: "patchType", String: &patchType}, {Name: "content", String: &content}, {Name: "baseHash", String: &baseHash}})
			},
		},
		{
			name: "text_replace", target: "replace.txt", initial: "old\n", variant: "text_replace", want: "new\n",
			candidate: func() runtimecontract.RuntimeCandidate {
				patchType, oldText, newText, target := "text_replace", "old\n", "new\n", "replace.txt"
				return runtimePatchCandidate("patch-text-replace", target, []runtimecontract.OperationParameter{{Name: "patchType", String: &patchType}, {Name: "oldText", String: &oldText}, {Name: "newText", String: &newText}})
			},
		},
		{
			name: "unified_diff", target: "diff.txt", initial: "one\ntwo\nthree\n", variant: "unified_diff", want: "one\nTWO\nthree\n",
			candidate: func() runtimecontract.RuntimeCandidate {
				patchType, target := "unified_diff", "diff.txt"
				diff := "--- a/diff.txt\n+++ b/diff.txt\n@@ -1,3 +1,3 @@\n one\n-two\n+TWO\n three\n"
				return runtimePatchCandidate("patch-unified-diff", target, []runtimecontract.OperationParameter{{Name: "patchType", String: &patchType}, {Name: "diff", String: &diff}})
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
			bootstrap := runtimePatchVariantBootstrap(testCase.variant, testCase.target)
			runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
			if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2", DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive}); err != nil {
				t.Fatal(err)
			}
			runOwner := runmodule.New()
			if err := runOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: hub}); err != nil {
				t.Fatal(err)
			}
			if err := runOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = runOwner.Stop(ctx) })
			startRuntimeReceiptTestOwner(t, hub)
			workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
			workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
				result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
			})
			t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
			fileOwner := filemodule.New()
			if err := fileOwner.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
				t.Fatal(err)
			}
			if err := fileOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = fileOwner.Stop(ctx) })
			candidateCalls := 0
			llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
			llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
				command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
				if command.Decision.State == runtimecontract.ConvergenceStateNeedAssessment {
					result.Set(runtimeCandidateResultForTest(t, command, runtimeSatisfiedAssessmentCandidate(command, "recovered patch receipt satisfies current coverage")), nil)
					return
				}
				candidateCalls++
				result.Set(runtimeCandidateResultForTest(t, command, testCase.candidate()), nil)
			})
			t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })

			module := newRuntimeDriveTestModule(t, hub)
			run, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			aggregate, err := module.initializeRuntimeFromRun(ctx, run)
			if err != nil {
				t.Fatal(err)
			}
			aggregate = runtimeTestPersistSealedSemanticAuthority(t, ctx, runtimeStateRepository{hub: hub}, aggregate)
			aggregate, err = module.prepareRuntimePrerequisites(ctx, aggregate)
			if err != nil {
				t.Fatal(err)
			}
			prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(aggregate.Definition, aggregate.Facts.Activation.ExecutionContract)
			aggregate, err = module.requestAndSealRuntimeCandidateWithRetry(ctx, aggregate, prefix)
			if err != nil || aggregate.ActiveDispatch == nil {
				t.Fatalf("seal patch variant before crash aggregate=%+v err=%v", aggregate, err)
			}
			func() {
				defer func() {
					if recovered := recover(); recovered != "runtime-test-crash-after-owner-receipt" {
						t.Fatalf("unexpected patch variant crash marker: %v", recovered)
					}
				}()
				_, _ = module.ExecuteRuntimeDispatch(ctx, "ws-v2", "run-v2", panicAfterOwnerReceiptExecutor{delegate: eventRuntimeOperationExecutor{hub: hub}})
				t.Fatal("expected injected patch variant crash after owner receipt")
			}()
			content, err := os.ReadFile(filepath.Join(deliverable, testCase.target))
			if err != nil || string(content) != testCase.want {
				state, _ := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
				var diagnostic *runtimecontract.RuntimeOriginalDiagnostic
				if state.RuntimeState != nil && len(state.RuntimeState.OperationReceipts) > 0 {
					feedback := state.RuntimeState.OperationReceipts[0].Feedback
					if feedback != nil {
						diagnostic = feedback.OriginalDiagnostic
					}
				}
				t.Fatalf("side effect before patch restart content=%q err=%v want=%q diagnostic=%+v", content, err, testCase.want, diagnostic)
			}
			crashed, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil || crashed.RuntimeState == nil || crashed.RuntimeState.ActiveDispatch == nil || len(crashed.RuntimeState.OperationReceipts) != 1 || crashed.RuntimeState.EvidenceSequence != 0 {
				t.Fatalf("patch receipt crash boundary=%+v err=%v", crashed, err)
			}
			execution := crashed.RuntimeState.ActiveDispatch.OperationExecutions[0]
			if execution.State != runtimecontract.OperationExecutionOwnerCommitted || execution.Fence == 0 {
				t.Fatalf("patch receipt crash execution=%+v", execution)
			}
			restarted := newRuntimeDriveTestModule(t, hub)
			driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
			driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntime, restarted.handleRuntimeDrive)
			t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntime) })
			completed, err := DriveRuntime(ctx, hub, orchestratorevents.DriveRuntimeCommand{WorkspaceID: "ws-v2", RunID: "run-v2"})
			if err != nil || completed.State != runtimecontract.ConvergenceStateComplete || candidateCalls != 1 {
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

func TestDriveRuntimeFileBackedRestartRecoversCrashAfterOwnerReceipt(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	root := t.TempDir()
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	bootstrap := runtimeBootstrap()
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2",
		DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive,
	}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	startRuntimeReceiptTestOwner(t, hub)

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
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{
			ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir,
		}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	fileOwner := filemodule.New()
	if err := fileOwner.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := fileOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fileOwner.Stop(ctx) })
	candidateCount := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		if command.Decision.State == runtimecontract.ConvergenceStateNeedAssessment {
			result.Set(runtimeCandidateResultForTest(t, command, runtimeSatisfiedAssessmentCandidate(command, "recovered file receipt satisfies current coverage")), nil)
			return
		}
		candidateCount++
		candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
			ID: "write-1", Kind: runtimecontract.StepKindFileEdit,
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"},
			Parameters: runtimeFileEditParameters("target.txt"),
		}}}}}
		result.Set(runtimeCandidateResultForTest(t, command, candidate), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })
	module := newRuntimeDriveTestModule(t, hub)
	run, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err := module.initializeRuntimeFromRun(ctx, run)
	if err != nil {
		t.Fatal(err)
	}
	aggregate = runtimeTestPersistSealedSemanticAuthority(t, ctx, runtimeStateRepository{hub: hub}, aggregate)
	aggregate, err = module.prepareRuntimePrerequisites(ctx, aggregate)
	if err != nil {
		t.Fatal(err)
	}
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(aggregate.Definition, aggregate.Facts.Activation.ExecutionContract)
	aggregate, err = module.requestAndSealRuntimeCandidateWithRetry(ctx, aggregate, prefix)
	if err != nil || aggregate.ActiveDispatch == nil {
		t.Fatalf("seal before crash aggregate=%+v err=%v", aggregate, err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "runtime-test-crash-after-owner-receipt" {
				t.Fatalf("unexpected crash marker: %v", recovered)
			}
		}()
		_, _ = module.ExecuteRuntimeDispatch(ctx, "ws-v2", "run-v2", panicAfterOwnerReceiptExecutor{delegate: eventRuntimeOperationExecutor{hub: hub}})
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
	llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate)
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
	if err := restartedRunOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: restartedHub}); err != nil {
		t.Fatal(err)
	}
	if err := restartedRunOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedRunOwner.Stop(ctx) })
	startRuntimeReceiptTestOwner(t, restartedHub)
	restartedWorkspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, restartedHub)
	restartedWorkspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{
			ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir,
		}}, nil)
	})
	t.Cleanup(func() { restartedWorkspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	restartedFileOwner := filemodule.New()
	if err := restartedFileOwner.Configure(filebiz.Dependencies{EventHub: restartedHub}); err != nil {
		t.Fatal(err)
	}
	if err := restartedFileOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedFileOwner.Stop(ctx) })

	restarted := newRuntimeDriveTestModule(t, restartedHub)
	restartedLLMObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, restartedHub)
	restartedLLMObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		if command.Decision.State != runtimecontract.ConvergenceStateNeedAssessment {
			t.Errorf("restart requested unexpected candidate state %q", command.Decision.State)
			return
		}
		result.Set(runtimeCandidateResultForTest(t, command, runtimeSatisfiedAssessmentCandidate(command, "recovered file receipt satisfies current coverage")), nil)
	})
	t.Cleanup(func() { restartedLLMObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })
	driveObserver := magicevent.NewSimpleObserver(ModuleID, restartedHub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntime, restarted.handleRuntimeDrive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntime) })
	completed, err := DriveRuntime(ctx, restartedHub, orchestratorevents.DriveRuntimeCommand{WorkspaceID: "ws-v2", RunID: "run-v2"})
	if err != nil {
		t.Fatal(err)
	}
	if completed.State != runtimecontract.ConvergenceStateComplete || candidateCount != 1 {
		t.Fatalf("restart completion=%+v candidateCount=%d", completed, candidateCount)
	}
	completedRun, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || completedRun.RuntimeState == nil {
		t.Fatalf("completed persisted run=%+v err=%v", completedRun, err)
	}
	if len(completedRun.RuntimeState.OperationReceipts) != 1 || len(completedRun.RuntimeState.Facts.Evidence.Accepted) != 1 ||
		len(completedRun.RuntimeState.Facts.Assessments) != 1 || completedRun.RuntimeState.EvidenceSequence != 2 {
		t.Fatalf("restart exactly-once receipts=%d accepted=%d assessments=%d evidenceSequence=%d", len(completedRun.RuntimeState.OperationReceipts), len(completedRun.RuntimeState.Facts.Evidence.Accepted), len(completedRun.RuntimeState.Facts.Assessments), completedRun.RuntimeState.EvidenceSequence)
	}
	if completedRun.RuntimeState.OperationReceipts[0].ExecutionKey != execution.ExecutionKey || completedRun.RuntimeState.OperationReceipts[0].Fence != execution.Fence {
		t.Fatal(fmt.Sprintf("restart replaced owner receipt: before=%+v after=%+v", execution, completedRun.RuntimeState.OperationReceipts[0]))
	}
}

func TestRuntimeFileBackedClaimEvidenceAndDispatchCloseCrashMatrix(t *testing.T) {
	testCases := []struct {
		name           string
		validation     bool
		after          bool
		outcomeUnknown bool
		match          func(runtime.RuntimeAggregate) bool
	}{
		{
			name: "claim_cas_before", validation: true,
			match: func(aggregate runtime.RuntimeAggregate) bool {
				return aggregate.ActiveDispatch != nil && len(aggregate.ActiveDispatch.OperationExecutions) == 1 &&
					aggregate.ActiveDispatch.OperationExecutions[0].State == runtimecontract.OperationExecutionClaimed && len(aggregate.OperationReceipts) == 0
			},
		},
		{
			name: "claim_cas_after_execute_before", validation: true, after: true, outcomeUnknown: true,
			match: func(aggregate runtime.RuntimeAggregate) bool {
				return aggregate.ActiveDispatch != nil && len(aggregate.ActiveDispatch.OperationExecutions) == 1 &&
					aggregate.ActiveDispatch.OperationExecutions[0].State == runtimecontract.OperationExecutionClaimed && len(aggregate.OperationReceipts) == 0
			},
		},
		{
			name: "evidence_and_dispatch_close_cas_before", validation: false,
			match: func(aggregate runtime.RuntimeAggregate) bool { return aggregate.EvidenceSequence == 1 },
		},
		{
			name: "evidence_and_dispatch_close_cas_after", validation: false, after: true,
			match: func(aggregate runtime.RuntimeAggregate) bool { return aggregate.EvidenceSequence == 1 },
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			hub := magicevent.NewHub(32)
			t.Cleanup(func() { hub.Terminate(ctx) })
			root := t.TempDir()
			runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
			bootstrap := runtimeBootstrap()
			if testCase.validation {
				bootstrap.Definition.CapabilityContract = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
				bootstrap.Activation.EvidencePolicies = []runtimecontract.RuntimeEvidencePolicy{{
					ID: "claim-crash-validation", AppliesTo: runtimecontract.StepKindShellCommand,
					CommandName: "sh", RecipeDeclared: true, RecipeArgs: []string{"-c", "printf 'WORKORCH_CHECK_PASSED=claim-crash-validation\\n'"},
				}}
				bootstrap.DefinitionFocuses[0].RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
				bootstrap.Requirements = []runtimecontract.RuntimeRequirement{{
					Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleShell,
					CapabilityClass: runtimecontract.RuntimeCapabilityClassValidation, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
				}, {
					Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleShell,
					RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, Reason: "Shell validation inventory",
				}}
			}
			bootstrap = runtimeTestBootstrapWithReboundBindings(bootstrap)
			if err := runs.Create(ctx, &runmodel.Run{
				ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2",
				DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive,
			}); err != nil {
				t.Fatal(err)
			}
			runOwner := runmodule.New()
			if err := runOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: hub}); err != nil {
				t.Fatal(err)
			}
			if err := runOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = runOwner.Stop(ctx) })
			startRuntimeReceiptTestOwner(t, hub)

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
				result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
			})
			t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
			fileOwner := filemodule.New()
			if err := fileOwner.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
				t.Fatal(err)
			}
			if err := fileOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = fileOwner.Stop(ctx) })
			if testCase.validation {
				startRuntimeManagedValidationTestOwner(t, hub)
			}

			module := newRuntimeDriveTestModule(t, hub)
			run, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			aggregate, err := module.initializeRuntimeFromRun(ctx, run)
			if err != nil {
				t.Fatal(err)
			}
			aggregate = runtimeTestPersistSealedSemanticAuthority(t, ctx, runtimeStateRepository{hub: hub}, aggregate)
			aggregate, err = module.prepareRuntimePrerequisites(ctx, aggregate)
			if err != nil {
				t.Fatal(err)
			}
			prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(aggregate.Definition, aggregate.Facts.Activation.ExecutionContract)
			if _, err := module.BeginRuntimeCandidateAttemptForTest(ctx, "ws-v2", "run-v2", 0, prefix); err != nil {
				t.Fatal(err)
			}
			target := "target.txt"
			candidate := runtimecontract.RuntimeCandidate{}
			if testCase.validation {
				candidate = runtimeManagedShellValidationCandidate("validate-1", "cwi-v2", target, "claim-crash-validation")
			} else {
				candidate.StateChange = &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
					ID: "edit-1", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{target},
					Parameters: runtimeFileEditParameters(target),
				}}}}
			}
			if testCase.validation {
				// The direct controller fixture bypasses the normal mainline owner
				// probe.  Persist the same scoped Shell feasibility fact before
				// sealing; the record is not a File-read compatibility shortcut.
				if _, err := module.AdmitRuntimeCandidateProposalForTest(ctx, "ws-v2", "run-v2", 0, candidate); err != nil {
					t.Fatal(err)
				}
				var shellInventory runtimecontract.RuntimeOperationAdmissibility
				for _, inventory := range aggregate.Facts.Resources.OperationAdmissibilities {
					if inventory.OwnerModuleID == runtimecontract.RuntimeModuleShell {
						shellInventory = inventory
						break
					}
				}
				if shellInventory.SnapshotID == "" {
					t.Fatal("managed validation fixture is missing Shell admissibility inventory")
				}
				if _, err := module.RecordRuntimeCandidateFeasibility(ctx, "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidateFeasibilityRecord{
					OperationID: "validate-1", AdmissibilitySnapshotID: shellInventory.SnapshotID, Observation: &runtimecontract.RuntimeOwnerFeasibilityObservation{Outcome: runtimecontract.RuntimeOwnerFeasibilityFeasible},
				}); err != nil {
					t.Fatal(err)
				}
			}
			sealed, err := module.AcceptRuntimeCandidateAttempt(ctx, "ws-v2", "run-v2", 0, candidate)
			if err != nil || sealed.ActiveDispatch == nil {
				t.Fatalf("sealed aggregate=%+v err=%v", sealed, err)
			}
			baseRepository := runtimeStateRepository{hub: hub}
			crashRepository := &runtimeSaveCrashRepository{delegate: baseRepository, match: testCase.match, after: testCase.after, marker: testCase.name}
			crashingController, err := runtime.NewRuntimeController(crashRepository)
			if err != nil {
				t.Fatal(err)
			}
			func() {
				defer func() {
					if recovered := recover(); recovered != testCase.name {
						t.Fatalf("unexpected crash marker: %v", recovered)
					}
				}()
				_, _ = runtimeTestControllerExecuteDispatch(crashingController, ctx, "ws-v2", "run-v2", eventRuntimeOperationExecutor{hub: hub})
				t.Fatal("expected injected save crash")
			}()
			if !crashRepository.fired {
				t.Fatal("crash point was not reached")
			}

			// Rebuild every process-owned actor before recovery. A claim that was
			// not persisted can still execute once. A persisted claimed managed
			// validation has an ambiguous owner-call boundary and must become
			// outcome_unknown without replay; durable state change consumes its
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
			if err := restartedRunOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: restartedHub}); err != nil {
				t.Fatal(err)
			}
			if err := restartedRunOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = restartedRunOwner.Stop(ctx) })
			startRuntimeReceiptTestOwner(t, restartedHub)
			restartedWorkspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, restartedHub)
			restartedWorkspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
				result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDir}}, nil)
			})
			t.Cleanup(func() { restartedWorkspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
			restartedFileOwner := filemodule.New()
			if err := restartedFileOwner.Configure(filebiz.Dependencies{EventHub: restartedHub}); err != nil {
				t.Fatal(err)
			}
			if err := restartedFileOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = restartedFileOwner.Stop(ctx) })
			if testCase.validation {
				startRuntimeManagedValidationTestOwner(t, restartedHub)
			}
			restartedRepository := runtimeStateRepository{hub: restartedHub}
			restarted, err := runtime.NewRuntimeController(restartedRepository)
			if err != nil {
				t.Fatal(err)
			}
			loaded, err := restarted.Load(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			if loaded.ActiveDispatch != nil {
				loaded, err = runtimeTestControllerExecuteDispatch(restarted, ctx, "ws-v2", "run-v2", eventRuntimeOperationExecutor{hub: restartedHub})
				if err != nil && !testCase.outcomeUnknown {
					t.Fatal(err)
				}
			}
			if testCase.outcomeUnknown {
				var unknown *runtime.OperationOutcomeUnknownError
				if !errors.As(err, &unknown) {
					t.Fatalf("persisted managed-validation claim err=%v, want outcome_unknown", err)
				}
				loaded, err = restarted.Load(ctx, "ws-v2", "run-v2")
				if err != nil {
					t.Fatal(err)
				}
				if loaded.ActiveDispatch == nil {
					t.Fatalf("outcome-unknown managed validation lost active dispatch: %+v", loaded)
				}
				execution, found := runtimeDriveOperationExecutionRecord(loaded.ActiveDispatch.OperationExecutions, "validate-1")
				if !found || execution.State != runtimecontract.OperationExecutionOutcomeUnknown ||
					len(loaded.OperationReceipts) != 0 || len(loaded.Facts.Evidence.Accepted) != 0 || loaded.Facts.Recovery.OperationRecoveryAttempts != 0 {
					t.Fatalf("persisted managed-validation claim was not fenced: aggregate=%+v execution=%+v", loaded, execution)
				}
				content, readErr := os.ReadFile(filepath.Join(deliverable, "target.txt"))
				if readErr != nil || string(content) != "old\n" {
					t.Fatalf("outcome-unknown validation changed workspace content=%q err=%v", string(content), readErr)
				}
				return
			}
			if loaded.Decision.State == runtimecontract.ConvergenceStateNeedAssessment {
				if _, err := beginRuntimeCandidateAttemptForTest(ctx, restarted, "ws-v2", "run-v2", 0, "crash-recovery-assessment"); err != nil {
					t.Fatal(err)
				}
				assessment := runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{
					FocusID: loaded.Decision.Focus.ID, CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), loaded.Decision.Focus.CoverageWorkItemIDs...),
					Disposition: runtimecontract.AssessmentDispositionSatisfied, Summary: "recovered owner evidence satisfies current coverage",
					EvidenceRefs: []string{string(loaded.Facts.Evidence.Accepted[0].ID)},
					Items:        []runtimecontract.RuntimeAssessmentItemFact{{CoverageWorkItemID: loaded.Decision.Focus.CoverageWorkItemIDs[0], Satisfied: true, EvidenceRefs: []string{string(loaded.Facts.Evidence.Accepted[0].ID)}}},
				}}
				sealedAssessment, sealErr := admitAndSealRuntimeCandidateAttemptForTest(ctx, restarted, "ws-v2", "run-v2", 0, assessment)
				if sealErr != nil || !sealedAssessment.Admission.Accepted() {
					t.Fatalf("seal crash recovery assessment=%+v err=%v", sealedAssessment.Admission, sealErr)
				}
				loaded, err = commitRuntimeAssessmentForTest(ctx, restarted, "ws-v2", "run-v2")
				if err != nil {
					t.Fatal(err)
				}
			}
			wantSequence, wantAssessments := uint64(2), 1
			if loaded.Decision.State != runtimecontract.ConvergenceStateComplete || len(loaded.OperationReceipts) != 1 || len(loaded.Facts.Evidence.Accepted) != 1 ||
				loaded.EvidenceSequence != wantSequence || len(loaded.Facts.Assessments) != wantAssessments {
				t.Fatalf("restart exactly-once aggregate=%+v", loaded)
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
func TestRuntimeFileBackedEvidenceCoverageFocusAdvanceCrashRestart(t *testing.T) {
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
			bootstrap := runtimeBootstrap()
			bootstrap.DefinitionFocuses = []runtimecontract.RuntimeDefinitionFocus{
				{ID: "focus-1", SliceID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}},
				{ID: "focus-2", SliceID: "slice-2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-2"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}},
			}
			bootstrap.Slices = []runtimecontract.RuntimeAcceptanceSlice{
				{ID: "slice-1", Title: "one", Scope: "fixture", Priority: 1, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, CompletionPolicy: runtimecontract.RuntimeAcceptanceSliceCompletionAll, ResultPolicy: runtimecontract.RuntimeAcceptanceSliceResultAllowed},
				{ID: "slice-2", Title: "two", Scope: "fixture", Priority: 2, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-2"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, CompletionPolicy: runtimecontract.RuntimeAcceptanceSliceCompletionAll, ResultPolicy: runtimecontract.RuntimeAcceptanceSliceResultAllowed},
			}
			bootstrap.CoverageItems = []runtimecontract.RuntimeCoverageItem{{ID: "cwi-1", SliceID: "slice-1"}, {ID: "cwi-2", SliceID: "slice-2"}}
			bootstrap = runtimeTestBootstrapWithBindings(bootstrap)
			if err := filerepository.NewRunRepository(runsPath).Create(ctx, &runmodel.Run{
				ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
				DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive,
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
				if err := runOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: hub}); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := runOwner.Start(); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				startRuntimeReceiptTestOwner(t, hub)
				workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
				workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
					result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: "ws-v2", DeliverableDirectory: deliverable}}, nil)
				})
				fileOwner := filemodule.New()
				if err := fileOwner.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
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
			module := newRuntimeDriveTestModule(t, first.hub)
			run, err := first.runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			aggregate, err := module.initializeRuntimeFromRun(ctx, run)
			if err != nil {
				t.Fatal(err)
			}
			aggregate = runtimeTestPersistSealedSemanticAuthority(t, ctx, runtimeStateRepository{hub: first.hub}, aggregate)
			aggregate, err = module.prepareRuntimePrerequisites(ctx, aggregate)
			if err != nil {
				t.Fatal(err)
			}
			prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(aggregate.Definition, aggregate.Facts.Activation.ExecutionContract)
			if _, err := module.BeginRuntimeCandidateAttemptForTest(ctx, "ws-v2", "run-v2", 0, prefix); err != nil {
				t.Fatal(err)
			}
			target := "one.txt"
			sealed, err := module.AcceptRuntimeCandidateAttempt(ctx, "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
				ID: "write-focus-1", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{target}, Parameters: runtimeFileEditParameters(target),
			}}}}})
			if err != nil || sealed.ActiveDispatch == nil {
				t.Fatalf("sealed focus-one dispatch=%+v err=%v", sealed, err)
			}

			baseRepository := runtimeStateRepository{hub: first.hub}
			ownerController, err := runtime.NewRuntimeController(baseRepository)
			if err != nil {
				t.Fatal(err)
			}
			executor := &runtimeFocusAdvanceReceiptExecutor{owner: ownerController}
			observed, err := runtimeTestControllerExecuteDispatch(ownerController, ctx, "ws-v2", "run-v2", executor)
			if err != nil || observed.Decision.State != runtimecontract.ConvergenceStateNeedAssessment || observed.ActiveDispatch != nil || executor.calls != 1 {
				t.Fatalf("focus-one owner evidence=%+v calls=%d err=%v", observed, executor.calls, err)
			}
			if _, err := beginRuntimeCandidateAttemptForTest(ctx, ownerController, "ws-v2", "run-v2", 0, "focus-advance-assessment"); err != nil {
				t.Fatal(err)
			}
			assessment := runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{
				FocusID: observed.Decision.Focus.ID, CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), observed.Decision.Focus.CoverageWorkItemIDs...),
				Disposition: runtimecontract.AssessmentDispositionSatisfied, Summary: "focus one owner evidence satisfies coverage",
				EvidenceRefs: []string{string(observed.Facts.Evidence.Accepted[0].ID)},
				Items:        []runtimecontract.RuntimeAssessmentItemFact{{CoverageWorkItemID: "cwi-1", Satisfied: true, EvidenceRefs: []string{string(observed.Facts.Evidence.Accepted[0].ID)}}},
			}}
			sealedAssessment, err := admitAndSealRuntimeCandidateAttemptForTest(ctx, ownerController, "ws-v2", "run-v2", 0, assessment)
			if err != nil || !sealedAssessment.Admission.Accepted() || sealedAssessment.Aggregate.ActiveDispatch == nil {
				t.Fatalf("sealed focus-one assessment=%+v err=%v", sealedAssessment, err)
			}

			marker := "assessment-coverage-focus-before-cas"
			if crashAfterCAS {
				marker = "assessment-coverage-focus-after-cas"
			}
			crashRepository := &runtimeSaveCrashRepository{
				delegate: baseRepository, after: crashAfterCAS, marker: marker,
				match: func(current runtime.RuntimeAggregate) bool {
					return current.EvidenceSequence == 2 && runtimeFocusCovers(current.Facts.Focus, "cwi-2") && current.ActiveDispatch == nil &&
						len(current.Facts.Evidence.Accepted) == 1 && len(current.Facts.Assessments) == 1 &&
						len(current.Facts.CompletedFocuses) == 1 && runtimeFocusCovers(current.Facts.CompletedFocuses[0], "cwi-1")
				},
			}
			crashingController, err := runtime.NewRuntimeController(crashRepository)
			if err != nil {
				t.Fatal(err)
			}
			func() {
				defer func() {
					if recovered := recover(); recovered != marker {
						t.Fatalf("unexpected evidence/focus crash marker: %v", recovered)
					}
				}()
				_, err := commitRuntimeAssessmentForTest(ctx, crashingController, "ws-v2", "run-v2")
				if err != nil {
					t.Fatalf("assessment/coverage/focus transition returned before crash: %v", err)
				}
				t.Fatal("expected assessment/coverage/focus CAS crash")
			}()
			if !crashRepository.fired || executor.calls != 1 {
				t.Fatalf("focus-one receipt was not committed exactly once: crash=%t calls=%d", crashRepository.fired, executor.calls)
			}
			crashed, err := first.runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil || crashed.RuntimeState == nil {
				t.Fatalf("evidence/focus crash state=%+v err=%v", crashed, err)
			}
			if crashAfterCAS {
				if crashed.RuntimeState.EvidenceSequence != 2 || !runtimeFocusCovers(crashed.RuntimeState.Facts.Focus, "cwi-2") ||
					len(crashed.RuntimeState.Facts.Evidence.Accepted) != 1 || len(crashed.RuntimeState.Facts.Assessments) != 1 || len(crashed.RuntimeState.Facts.CompletedFocuses) != 1 {
					t.Fatalf("after-CAS evidence/focus state was not atomic: %+v", crashed.RuntimeState)
				}
			} else if crashed.RuntimeState.EvidenceSequence != 1 || !runtimeFocusCovers(crashed.RuntimeState.Facts.Focus, "cwi-1") ||
				len(crashed.RuntimeState.Facts.Evidence.Accepted) != 1 || len(crashed.RuntimeState.Facts.Assessments) != 0 || len(crashed.RuntimeState.Facts.CompletedFocuses) != 0 ||
				crashed.RuntimeState.ActiveDispatch == nil || crashed.RuntimeState.ActiveDispatch.Candidate.Assessment == nil || len(crashed.RuntimeState.OperationReceipts) != 1 {
				t.Fatalf("before-CAS evidence/focus transition leaked or lost its receipt: %+v", crashed.RuntimeState)
			}
			stopProcess(first)

			second := startProcess(t)
			defer stopProcess(second)
			restarted, err := runtime.NewRuntimeController(runtimeStateRepository{hub: second.hub})
			if err != nil {
				t.Fatal(err)
			}
			loaded, err := restarted.Load(ctx, "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			if loaded.ActiveDispatch != nil {
				if loaded.ActiveDispatch.Candidate.Assessment == nil {
					t.Fatalf("fresh process has unexpected active dispatch: %+v", loaded.ActiveDispatch)
				}
				loaded, err = commitRuntimeAssessmentForTest(ctx, restarted, "ws-v2", "run-v2")
				if err != nil {
					t.Fatal(err)
				}
			}
			if loaded.EvidenceSequence != 2 || !runtimeFocusCovers(loaded.Facts.Focus, "cwi-2") || len(loaded.Facts.RemainingFocuses) != 0 ||
				len(loaded.Facts.CompletedFocuses) != 1 || !runtimeFocusCovers(loaded.Facts.CompletedFocuses[0], "cwi-1") ||
				len(loaded.Facts.Evidence.Accepted) != 1 || len(loaded.Facts.Assessments) != 1 || loaded.ActiveDispatch != nil || len(loaded.OperationReceipts) != 1 {
				t.Fatalf("fresh process did not converge evidence/coverage/focus exactly once: %+v", loaded)
			}
			for _, item := range loaded.Facts.Coverage {
				if item.ID == "cwi-1" && item.State != runtimecontract.RuntimeCoverageItemStateSatisfied {
					t.Fatalf("focus-one coverage was not closed by accepted evidence: %+v", item)
				}
			}
			// Package-local dispatch-commit and coverage lifecycle tests exercise
			// duplicate evidence admission directly. This parent restart fixture
			// intentionally has no second mutation API beyond ApplyInternalEvent.
		})
	}
}

// TestRuntimeFileBackedOperationGovernanceGrantFreshProcessCrashRestart
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
func TestRuntimeFileBackedOperationGovernanceGrantFreshProcessCrashRestart(t *testing.T) {
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
			approvals := filerepository.NewApprovalRepository(filepath.Join(root, "approvals.json"))
			bootstrap := runtimeBootstrap()
			bootstrap.Definition.CapabilityContract = []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindApprovalRequest}
			bootstrap.Requirements = []runtimecontract.RuntimeRequirement{
				{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleFile, CapabilityClass: runtimecontract.RuntimeCapabilityClassStateChange, RequiredCapabilities: []string{runtimecontract.StepKindFileWrite}},
				{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleApproval, CapabilityClass: runtimecontract.RuntimeCapabilityClassGovernance, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindApprovalRequest}},
			}
			bootstrap = runtimeTestBootstrapWithReboundBindings(bootstrap)
			if err := runs.Create(ctx, &runmodel.Run{
				ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
				DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive,
			}); err != nil {
				t.Fatal(err)
			}

			type process struct {
				hub           magicevent.Hub
				runOwner      *runmodule.Module
				approvalOwner *approvalmodule.Module
				fileOwner     *filemodule.Module
				workspace     magicevent.SimpleObserver
				receipts      magicevent.SimpleObserver
				module        *Biz
				controller    *runtime.RuntimeController
			}
			startProcess := func(t *testing.T) process {
				t.Helper()
				hub := magicevent.NewHub(16)
				runOwner := runmodule.New()
				if err := runOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: hub}); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := runOwner.Start(); err != nil {
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				approvalOwner := approvalmodule.New()
				if err := approvalOwner.Configure(approvalbiz.Dependencies{OwnerRepositories: testApprovalOwnerRepositories(approvals, memory.NewApprovalWorkspaceDeletionRepository()), EventHub: hub}); err != nil {
					_ = runOwner.Stop(ctx)
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := approvalOwner.Start(); err != nil {
					_ = runOwner.Stop(ctx)
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				workspace := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
				workspace.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
					result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{
						ID: "ws-v2", DeliverableDirectory: deliverable, InternalDirectory: internalDirectory,
					}}, nil)
				})
				fileOwner := filemodule.New()
				if err := fileOwner.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
					workspace.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
					_ = approvalOwner.Stop(ctx)
					_ = runOwner.Stop(ctx)
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				if err := fileOwner.Start(); err != nil {
					workspace.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
					_ = approvalOwner.Stop(ctx)
					_ = runOwner.Stop(ctx)
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				owner := newRuntimeTestOwner()
				if err := owner.ConfigureRuntimeRepository(runtimeStateRepository{hub: hub}); err != nil {
					_ = fileOwner.Stop(ctx)
					workspace.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
					_ = approvalOwner.Stop(ctx)
					_ = runOwner.Stop(ctx)
					hub.Terminate(ctx)
					t.Fatal(err)
				}
				module := newTestBiz(owner, hub, nil)
				receipts := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
				receipts.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, module.handleRuntimeOperationReceipt)
				receipts.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, module.handleRuntimeOperationReceipt)
				return process{hub: hub, runOwner: runOwner, approvalOwner: approvalOwner, fileOwner: fileOwner, workspace: workspace, receipts: receipts, module: module, controller: owner.runtimeController}
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
				if value.approvalOwner != nil {
					_ = value.approvalOwner.Stop(ctx)
				}
				if value.runOwner != nil {
					_ = value.runOwner.Stop(ctx)
				}
				if value.hub != nil {
					value.hub.Terminate(ctx)
				}
			}

			first := startProcess(t)
			facts := runtimeAggregate(t, 1).Facts
			facts.Focus.TargetRefs = []string{".env"}
			facts.Focus.AcceptedStepKinds = []string{runtimecontract.StepKindFileWrite}
			facts.Capabilities = runtimecontract.RuntimeCapabilities{
				StateChangeKinds: []string{runtimecontract.StepKindFileWrite},
				GovernanceKinds:  []string{runtimecontract.RuntimeBlockKindApprovalRequest},
			}
			definition := bootstrap.Definition
			initialized, err := runtimeTestControllerInitialize(first.controller, ctx, facts, definition, nil)
			if err != nil || initialized.Decision.State != runtimecontract.ConvergenceStateNeedStateChange {
				stopProcess(first)
				t.Fatalf("governance fixture initialization=%+v err=%v", initialized, err)
			}
			initialized = runtimeTestPersistSealedSemanticAuthority(t, ctx, runtimeStateRepository{hub: first.hub}, initialized)
			if _, err := beginRuntimeCandidateAttemptForTest(ctx, first.controller, "ws-v2", "run-v2", 0, "governance-grant-prefix"); err != nil {
				stopProcess(first)
				t.Fatal(err)
			}
			target, content := ".env", "TOKEN=granted\n"
			candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
				ID: "write-governed", Kind: runtimecontract.StepKindFileWrite, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{target},
				Parameters: []runtimecontract.OperationParameter{{Name: "content", String: &content}},
			}}}}}
			sealed, err := admitAndSealRuntimeCandidateAttemptForTest(ctx, first.controller, "ws-v2", "run-v2", 0, candidate)
			if err != nil || sealed.Aggregate.ActiveDispatch == nil {
				stopProcess(first)
				t.Fatalf("governed write seal=%+v err=%v", sealed, err)
			}
			executor := &countingRuntimeOperationExecutor{delegate: eventRuntimeOperationExecutor{hub: first.hub}}
			waiting, err := runtimeTestControllerExecuteDispatch(first.controller, ctx, "ws-v2", "run-v2", executor)
			if err != nil || waiting.ActiveDispatch != nil || waiting.Decision.State != runtimecontract.ConvergenceStateNeedGovernance || len(waiting.OperationGovernanceBindings) != 1 || len(executor.requests) != 1 {
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
			governed, err := runtimeTestControllerSealGovernanceDispatch(first.controller, ctx, "ws-v2", "run-v2", manifest, runtimecontract.RuntimeCandidate{Governance: &runtimecontract.GovernanceCandidate{ExternalDependency: runtimecontract.ExternalDependency{
				Kind: binding.DependencyKind, SubjectRef: binding.SubjectRef, Reason: waiting.Facts.Failure.GovernanceDependency.Reason,
				RequestedAction: binding.RequestedAction, RiskLevel: binding.RiskLevel,
				AuthorizationContext: waiting.Facts.Failure.GovernanceDependency.AuthorizationContext.Clone(),
			}}})
			if err != nil || governed.ActiveDispatch == nil {
				stopProcess(first)
				t.Fatalf("governance dispatch seal=%+v err=%v", governed, err)
			}
			bound, err := first.module.prepareRuntimeGovernanceRequest(ctx, governed)
			if err != nil || len(bound.GovernanceRequests) != 1 || len(bound.OperationGovernanceBindings) != 1 || bound.OperationGovernanceBindings[0].State != runtimecontract.RuntimeOperationGovernanceBindingTicketBound {
				stopProcess(first)
				t.Fatalf("governance ticket binding=%+v err=%v", bound, err)
			}
			request := bound.GovernanceRequests[0]
			if _, err := approvals.GetScoped(ctx, "ws-v2", "run-v2", request.ApprovalID); err != nil {
				stopProcess(first)
				t.Fatalf("approval owner did not persist governed approval %q: %v", request.ApprovalID, err)
			}
			resolution := runtimecontract.RuntimeGovernanceResolution{
				ResolutionID: "governance-grant-resolution", ApprovalID: request.ApprovalID, WorkspaceID: "ws-v2", RunID: "run-v2",
				AuthoritySnapshotID: request.AuthoritySnapshotID, DispatchID: request.DispatchID,
				DependencyKind: request.DependencyKind, SubjectRef: request.SubjectRef,
				Outcome: runtimecontract.RuntimeGovernanceResolutionOutcomeContinue, Summary: "persisted approval granted",
			}
			marker := "governance-grant-before-cas"
			if crashAfterCAS {
				marker = "governance-grant-after-cas"
			}
			crashRepository := &runtimeSaveCrashRepository{
				delegate: runtimeStateRepository{hub: first.hub}, after: crashAfterCAS, marker: marker,
				match: func(aggregate runtime.RuntimeAggregate) bool {
					return aggregate.ActiveDispatch != nil && len(aggregate.OperationGovernanceBindings) == 1 &&
						aggregate.OperationGovernanceBindings[0].State == runtimecontract.RuntimeOperationGovernanceBindingGranted
				},
			}
			crashing, err := runtime.NewRuntimeController(crashRepository)
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
				_, _ = runtimeTestControllerCommitGovernanceResolution(crashing, ctx, resolution)
				t.Fatal("expected governance grant CAS interruption")
			}()
			if !crashRepository.fired {
				stopProcess(first)
				t.Fatal("governance grant CAS interruption was not reached")
			}
			stopProcess(first)

			second := startProcess(t)
			defer stopProcess(second)
			continued, err := runtimeTestControllerCommitGovernanceResolution(second.controller, ctx, resolution)
			if err != nil {
				t.Fatal(err)
			}
			wantDisposition := runtime.CommitDispositionCommitted
			if crashAfterCAS {
				wantDisposition = runtime.CommitDispositionDuplicate
			}
			if continued.Disposition != wantDisposition || continued.Aggregate.ActiveDispatch == nil || len(continued.Aggregate.OperationGovernanceBindings) != 1 {
				t.Fatalf("fresh-process governance continuation=%+v, want disposition=%s", continued, wantDisposition)
			}
			grantBinding := continued.Aggregate.OperationGovernanceBindings[0].Normalized()
			if grantBinding.State != runtimecontract.RuntimeOperationGovernanceBindingGranted || grantBinding.Grant == nil ||
				grantBinding.ExecutionKey != binding.ExecutionKey || grantBinding.Grant.GovernanceFence != binding.Fence || grantBinding.Grant.ResumeFence != binding.Fence+1 {
				t.Fatalf("persisted governance grant=%+v initial binding=%+v", grantBinding, binding)
			}
			resumeExecution, found := runtimeOperationExecution(continued.Aggregate.ActiveDispatch.OperationExecutions, "write-governed")
			if !found || resumeExecution.ExecutionKey != binding.ExecutionKey || resumeExecution.Fence != binding.Fence+1 || resumeExecution.State != runtimecontract.OperationExecutionClaimed {
				t.Fatalf("fresh-process resume execution=%+v binding=%+v", resumeExecution, binding)
			}
			if _, _, err := second.controller.QueryOperationReceipt(ctx, "ws-v2", "run-v2", continued.Aggregate.ActiveDispatch.ID, "write-governed", binding.ExecutionKey, binding.Fence, grantBinding.Grant); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("old fence must not consume the resume grant: %v", err)
			}
			stale := resolution
			stale.ResolutionID = "governance-grant-stale"
			stale.AuthoritySnapshotID = "stale-governance-authority"
			staleCommit, err := runtimeTestControllerCommitGovernanceResolution(second.controller, ctx, stale)
			if err != nil || staleCommit.Disposition != runtime.CommitDispositionStale || staleCommit.Aggregate.ActiveDispatch == nil {
				t.Fatalf("stale governance resolution=%+v err=%v", staleCommit, err)
			}
			duplicate, err := runtimeTestControllerCommitGovernanceResolution(second.controller, ctx, resolution)
			if err != nil || duplicate.Disposition != runtime.CommitDispositionDuplicate || duplicate.Aggregate.ActiveDispatch == nil {
				t.Fatalf("duplicate governance resolution=%+v err=%v", duplicate, err)
			}

			executor.delegate.hub = second.hub
			completed, err := runtimeTestControllerExecuteDispatch(second.controller, ctx, "ws-v2", "run-v2", executor)
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
			if _, err := runtimeTestControllerCommitGovernanceResolution(second.controller, ctx, resolution); err != nil || len(executor.requests) != 2 {
				t.Fatalf("post-effect governance duplicate must not replay File owner: calls=%d err=%v", len(executor.requests), err)
			}
		})
	}
}

func runtimeOperationExecution(values []runtimecontract.RuntimeOperationExecutionRecord, operationID string) (runtimecontract.RuntimeOperationExecutionRecord, bool) {
	for _, value := range values {
		value = value.Normalized()
		if value.OperationID == operationID {
			return value, true
		}
	}
	return runtimecontract.RuntimeOperationExecutionRecord{}, false
}

func runtimeFocusCovers(focus runtimecontract.CoverageFocus, coverageID common.CoverageWorkItemID) bool {
	for _, id := range focus.CoverageWorkItemIDs {
		if id == coverageID {
			return true
		}
	}
	return false
}

func TestDriveRuntimeFileBackedTwoFocusStateValidationAssessmentAndRestart(t *testing.T) {
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
	bootstrap := runtimeBootstrap()
	bootstrap.Definition.CapabilityContract = []string{runtimecontract.StepKindFileEdit, runtimecontract.RuntimeBlockKindPatchApply, runtimecontract.RuntimeBlockKindManagedValidationSession, runtimecontract.RuntimeBlockKindDecisionRequest}
	bootstrap.Activation.EvidencePolicies = []runtimecontract.RuntimeEvidencePolicy{
		{ID: "two-focus-validation-1", AppliesTo: runtimecontract.StepKindShellCommand, CommandName: "sh", RecipeDeclared: true, RecipeArgs: []string{"-c", "printf 'WORKORCH_CHECK_PASSED=two-focus-validation-1\\n'"}},
		{ID: "two-focus-validation-2", AppliesTo: runtimecontract.StepKindShellCommand, CommandName: "sh", RecipeDeclared: true, RecipeArgs: []string{"-c", "printf 'WORKORCH_CHECK_PASSED=two-focus-validation-2\\n'"}},
	}
	bootstrap.Activation.ExecutionContract.ProfileContracts = []runtimecontract.RuntimeProfileContract{{
		ID: "two-focus-profile",
		EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{
			{
				ID: "cwi-1", Scope: "test", TargetRole: "deliverable", TargetRefs: []string{"focus-1.txt"}, RequireTargetGrounding: true,
				RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation},
				AcceptedStepKinds:     []string{runtimecontract.StepKindFileEdit, runtimecontract.RuntimeBlockKindManagedValidationSession},
				ValidationRecipeID:    "two-focus-validation-1", RequiredCheckRefs: []string{"two-focus-validation-1"},
			},
			{
				ID: "cwi-2", Scope: "test", TargetRole: "deliverable", TargetRefs: []string{"focus-1.txt"}, RequireTargetGrounding: true,
				RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation},
				AcceptedStepKinds:     []string{runtimecontract.RuntimeBlockKindPatchApply, runtimecontract.RuntimeBlockKindManagedValidationSession},
				AcceptedOperationVariants: []runtimecontract.RuntimeOperationVariantRef{{
					Kind: runtimecontract.RuntimeBlockKindPatchApply, VariantID: "file_write",
				}},
				ValidationRecipeID: "two-focus-validation-2", RequiredCheckRefs: []string{"two-focus-validation-2"},
			},
		},
	}}
	bootstrap.DefinitionFocuses = []runtimecontract.RuntimeDefinitionFocus{
		{ID: "focus-1", SliceID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}, TargetRole: "deliverable", RequireTargetGrounding: true, RequireAssessment: true},
		{ID: "focus-2", SliceID: "slice-2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-2"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}, TargetRole: "deliverable", RequireTargetGrounding: true, RequireAssessment: true},
	}
	bootstrap.Slices = []runtimecontract.RuntimeAcceptanceSlice{
		{ID: "slice-1", Title: "Focus 1", Scope: "test", Priority: 1, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}, CompletionPolicy: runtimecontract.RuntimeAcceptanceSliceCompletionAll, ResultPolicy: runtimecontract.RuntimeAcceptanceSliceResultAllowed},
		{ID: "slice-2", Title: "Focus 2", Scope: "test", Priority: 1, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-2"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}, CompletionPolicy: runtimecontract.RuntimeAcceptanceSliceCompletionAll, ResultPolicy: runtimecontract.RuntimeAcceptanceSliceResultAllowed},
	}
	bootstrap.CoverageItems = []runtimecontract.RuntimeCoverageItem{{ID: "cwi-1", SliceID: "slice-1"}, {ID: "cwi-2", SliceID: "slice-2"}}
	bootstrap.Requirements = []runtimecontract.RuntimeRequirement{
		{Kind: runtimecontract.PrerequisiteKindProviderCeiling, OwnerModuleID: runtimecontract.RuntimeModuleLLM, MinimumBytes: 64 * 1024, Reason: "semantic planning provider ceiling"},
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleFile, CapabilityClass: runtimecontract.RuntimeCapabilityClassStateChange, RequiredCapabilities: []string{runtimecontract.StepKindFileEdit, runtimecontract.RuntimeBlockKindPatchApply}},
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleShell, CapabilityClass: runtimecontract.RuntimeCapabilityClassValidation, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}},
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleDecision, CapabilityClass: runtimecontract.RuntimeCapabilityClassGovernance, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindDecisionRequest}},
		{Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleFile, RequiredCapabilities: []string{runtimecontract.StepKindFileEdit, runtimecontract.RuntimeBlockKindPatchApply}, Reason: "File state-change inventory"},
		{Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleShell, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, Reason: "Shell validation inventory"},
	}
	bootstrap = runtimeTestBootstrapWithBindings(bootstrap)
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
		DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive,
	}); err != nil {
		t.Fatal(err)
	}
	if err := workspaces.Create(ctx, runtimeWorkspaceForPersistedRunTest(t, ctx, runs, "ws-v2", "run-v2", deliverable, internalDir)); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	workspaceOwner := workspacemodule.New()
	if err := workspaceOwner.Configure(workspacebiz.Dependencies{
		Workspaces: workspaces, DeletionMaterialCleaner: workspaces, Goals: memory.NewGoalRepository(), DoDs: memory.NewDoDRepository(),
		Templates: memory.NewWorkspaceTemplateRepository(), EventHub: hub,
	}); err != nil {
		t.Fatal(err)
	}
	if err := workspaceOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = workspaceOwner.Stop(ctx) })
	startRuntimeReceiptTestOwner(t, hub)
	decisionOwner := decisionmodule.New()
	if err := decisionOwner.Configure(decisionbiz.Dependencies{OwnerRepositories: testDecisionOwnerRepositories(decisionTickets, decisionResolutions, memory.NewDecisionWorkspaceDeletionRepository()), EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := decisionOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = decisionOwner.Stop(ctx) })

	if err := os.WriteFile(filepath.Join(deliverable, "focus-1.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deliverable, ".env"), []byte("TOKEN=old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fileOwner := filemodule.New()
	if err := fileOwner.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := fileOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fileOwner.Stop(ctx) })
	startRuntimeManagedValidationTestOwner(t, hub)
	startRuntimeProviderCeilingTestOwner(t, hub)
	resultOwner := resultmodule.New()
	if err := resultOwner.Configure(resultbiz.Dependencies{OwnerRepositories: testResultOwnerRepositories(failingResults, artifacts, memory.NewResultWorkspaceDeletionRepository()), EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := resultOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resultOwner.Stop(ctx) })

	candidateCount := 0
	providerTransientInjected := false
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		candidateCount++
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		if command.DocumentKind == runtimecontract.RuntimeProviderDocumentSemanticPlan {
			response, responseErr := runtimeTwoFocusSemanticResult(command)
			if responseErr != nil {
				t.Errorf("two-focus semantic candidate: %v", responseErr)
				result.Set(runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{
					DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity,
					ExpectedDocumentHash: runtimeExpectedDocumentHash(command.ExpectedDocument),
					Feedback: runtimeProviderFeedbackForTest(t, command, runtimecontract.RuntimeCandidateFailure{
						Class: runtimecontract.RuntimeCandidateFailureClassContract, Code: "semantic_fixture_invalid",
						Signature: "semantic-fixture-invalid", Reason: responseErr.Error(),
					}),
				}), nil)
				return
			}
			result.Set(runtimeProviderResultWithAuditForTest(command, response), nil)
			return
		}
		focus := command.Decision.Focus
		coverageID := focus.CoverageWorkItemIDs[0]
		target := focus.TargetRefs[0]
		response := llmevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity, ExpectedDocumentHash: runtimeExpectedDocumentHash(command.ExpectedDocument)}
		var candidate runtimecontract.RuntimeCandidate
		if coverageID == "cwi-2" && command.Decision.State == runtimecontract.ConvergenceStateNeedStateChange && !providerTransientInjected {
			providerTransientInjected = true
			response.Feedback = runtimeProviderFeedbackForTest(t, command, runtimecontract.RuntimeCandidateFailure{Class: runtimecontract.RuntimeCandidateFailureClassProviderTransient, Reason: "injected provider transport interruption"})
			result.Set(runtimeProviderResultWithAuditForTest(command, response), nil)
			return
		}
		switch command.Decision.State {
		case runtimecontract.ConvergenceStateNeedStateChange:
			if coverageID == "cwi-2" {
				patchType, content := "file_write", "TOKEN=granted\n"
				baseContent := "TOKEN=old\n"
				if target == "focus-1.txt" {
					baseContent = "new\n"
				}
				baseHash := fmt.Sprintf("%x", sha256.Sum256([]byte(baseContent)))
				candidate = runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
					ID: "patch-" + focus.ID, Kind: runtimecontract.RuntimeBlockKindPatchApply,
					CoverageWorkItemIDs: []common.CoverageWorkItemID{coverageID}, TargetRefs: []string{target},
					Parameters: []runtimecontract.OperationParameter{{Name: "patchType", String: &patchType}, {Name: "content", String: &content}, {Name: "baseHash", String: &baseHash}},
				}}}}}
			} else {
				oldText, newText := "old\n", "new\n"
				candidate = runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
					ID: "edit-" + focus.ID, Kind: runtimecontract.StepKindFileEdit,
					CoverageWorkItemIDs: []common.CoverageWorkItemID{coverageID}, TargetRefs: []string{target},
					Parameters: []runtimecontract.OperationParameter{{Name: "oldText", String: &oldText}, {Name: "newText", String: &newText}},
				}}}}}
			}
		case runtimecontract.ConvergenceStateNeedValidation:
			recipeID := "two-focus-validation-1"
			if coverageID == "cwi-2" {
				recipeID = "two-focus-validation-2"
			}
			candidate = runtimeManagedShellValidationCandidate("validate-"+focus.ID, coverageID, target, recipeID)
		case runtimecontract.ConvergenceStateNeedAssessment:
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
			t.Errorf("unexpected candidate action %s", command.Decision.State)
			return
		}
		document := runtimeCandidateDocumentForTest(t, command.CandidateManifest, candidate)
		response.DocumentOutcome = runtimeCompleteDocumentOutcome(command.ExpectedDocument, document)
		result.Set(runtimeProviderResultWithAuditForTest(command, response), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })

	runtimeTestPersistBootstrappedOperationFixture(t, ctx, hub, "ws-v2", "run-v2", bootstrap)
	moduleBiz := newRuntimeTestOwner()
	casRepository := &failOnceCASRuntimeRepository{delegate: runtimeStateRepository{hub: hub}, remainingFailures: 1}
	if err := moduleBiz.ConfigureRuntimeRepository(casRepository); err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(moduleBiz, hub, nil)
	driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntime, module.handleRuntimeDrive)
	command := orchestratorevents.DriveRuntimeCommand{WorkspaceID: "ws-v2", RunID: "run-v2"}
	if err := module.scheduleRuntimeDrive(ctx, "ws-v2", "run-v2", "cas-conflict-injection"); err != nil {
		t.Fatal(err)
	}
	casRecovered, err := module.loadRuntime(ctx, "ws-v2", "run-v2")
	if err != nil || casRecovered.Facts.Recovery.CASReloadAttempts != 1 || casRecovered.Facts.Failure != nil {
		t.Fatalf("CAS recovery aggregate=%+v err=%v", casRecovered, err)
	}
	completed, err := DriveRuntime(ctx, hub, command)
	if err != nil {
		t.Fatal(err)
	}
	if completed.State != runtimecontract.ConvergenceStateComplete || completed.Terminal == nil || candidateCount != 9 {
		persisted, persistErr := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
		var failure *runtimecontract.FailureFact
		var semanticPlanning runtimecontract.RuntimeSemanticPlanningLedger
		if persisted.RuntimeState != nil {
			failure = persisted.RuntimeState.Facts.Failure
			semanticPlanning = persisted.RuntimeState.SemanticPlanning
		}
		var eligible []common.CoverageWorkItemID
		semanticReady := false
		if persisted.RuntimeState != nil {
			eligible = runtimecontract.RuntimeCWIsEligibleForPlanning(persisted.RuntimeState.Facts)
			semanticReady = runtime.SemanticOperationAuthorityReady(runtimeAggregateFromRecord(*persisted.RuntimeState))
		}
		t.Fatalf("two-focus state=%s terminal=%+v failure=%+v semanticPlanning=%+v eligible=%v semanticReady=%t candidates=%d persistErr=%v", completed.State, completed.Terminal, failure, semanticPlanning, eligible, semanticReady, candidateCount, persistErr)
	}
	persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.RuntimeState == nil {
		t.Fatalf("persisted two-focus run=%+v err=%v", persisted, err)
	}
	if len(persisted.RuntimeState.OperationReceipts) != 4 || len(persisted.RuntimeState.Facts.Assessments) != 2 {
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
	if readErr != nil || string(content) != "TOKEN=granted\n" {
		t.Fatalf("focus-1 content=%q err=%v", string(content), readErr)
	}
	content, readErr = os.ReadFile(filepath.Join(deliverable, ".env"))
	if readErr != nil || string(content) != "TOKEN=old\n" {
		t.Fatalf("unselected focus-2 legacy target content=%q err=%v", string(content), readErr)
	}
	driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntime)
	restarted := newRuntimeDriveTestModule(t, hub)
	driveObserver = magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntime, restarted.handleRuntimeDrive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntime) })
	replayed, err := DriveRuntime(ctx, hub, command)
	if err != nil || replayed.StateRevision != completed.StateRevision || candidateCount != 9 {
		t.Fatalf("two-focus restart=%+v completed=%+v candidates=%d err=%v", replayed, completed, candidateCount, err)
	}
	aggregate, err := restarted.loadRuntime(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	deliveryBiz := newRuntimeTestOwner()
	if err := deliveryBiz.ConfigureRuntimeRepository(runtimeStateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	deliveryModule := newTestBiz(deliveryBiz, hub, nil)
	if err := deliveryModule.deliverRuntimeTerminal(ctx, aggregate); err == nil {
		t.Fatal("first terminal delivery must fail before Result persistence")
	}
	if _, err := results.Get(ctx, "ws-v2/run-v2/result/dod-assessment"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("failed terminal delivery unexpectedly persisted Result: %v", err)
	}
	recoveryAggregate, consumed, err := restarted.ConsumeRuntimeRecoveryBudget(ctx, "ws-v2", "run-v2", runtimecontract.RuntimeRecoveryBudgetTerminalDelivery)
	if err != nil || !consumed || recoveryAggregate.Facts.Recovery.TerminalDeliveryAttempts != 1 {
		t.Fatalf("terminal delivery recovery budget aggregate=%+v consumed=%v err=%v", recoveryAggregate, consumed, err)
	}
	if err := resultOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	restartedResultOwner := resultmodule.New()
	if err := restartedResultOwner.Configure(resultbiz.Dependencies{OwnerRepositories: testResultOwnerRepositories(results, artifacts, memory.NewResultWorkspaceDeletionRepository()), EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := restartedResultOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedResultOwner.Stop(ctx) })
	if err := deliveryModule.deliverRuntimeTerminal(ctx, recoveryAggregate); err != nil {
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

func runtimeTwoFocusSemanticResult(command llmevents.RequestRuntimeCandidateCommand) (llmevents.RequestRuntimeCandidateResult, error) {
	document := runtimecontract.RuntimeSemanticPlanDocument{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent,
		ManifestHash:  command.SemanticManifest.Hash, DefinitionSeedHash: command.SemanticManifest.DefinitionSeedHash,
		PlanStep: command.SemanticContext.PlanStep,
	}
	switch command.SemanticContext.PlanStep {
	case runtimecontract.RuntimeSemanticPlanStepCoverageOrder:
		for _, item := range command.SemanticManifest.CoverageItems {
			document.CoverageOrder = append(document.CoverageOrder, item.ID)
		}
	case runtimecontract.RuntimeSemanticPlanStepProofBatch:
		document.BatchID = command.SemanticContext.BatchID
		document.BatchSequence = command.SemanticContext.BatchSequence
		document.BatchCoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), command.SemanticContext.BatchCoverageWorkItemIDs...)
		for _, coverageID := range document.BatchCoverageWorkItemIDs {
			if coverageID != "cwi-2" {
				return llmevents.RequestRuntimeCandidateResult{}, fmt.Errorf("unexpected two-focus semantic coverage %q", coverageID)
			}
			sharedTarget, err := runtimeTwoFocusSemanticSharedTarget(command)
			if err != nil {
				return llmevents.RequestRuntimeCandidateResult{}, err
			}
			document.Proposals = append(document.Proposals,
				runtimecontract.RuntimeSemanticProofProposal{
					CoverageWorkItemID: coverageID, OwnerModuleID: runtimecontract.RuntimeModuleFile,
					OperationKind: runtimecontract.RuntimeBlockKindPatchApply, OperationVariantID: "file_write",
					Target: runtimecontract.RuntimeOperationTargetAuthorization{
						Policy: runtimecontract.RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{sharedTarget},
					},
					RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
					RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
					DeclarationRef:        "two-focus/cwi-2",
				},
				runtimecontract.RuntimeSemanticProofProposal{
					CoverageWorkItemID: coverageID, OwnerModuleID: runtimecontract.RuntimeModuleShell,
					OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession,
					Target: runtimecontract.RuntimeOperationTargetAuthorization{
						Policy: runtimecontract.RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{sharedTarget},
					},
					RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
					RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectEphemeralSession},
					ValidationRecipeID:    "two-focus-validation-2", RequiredCheckRefs: []string{"two-focus-validation-2"},
					DeclarationRef: "two-focus/cwi-2",
				},
			)
		}
	default:
		return llmevents.RequestRuntimeCandidateResult{}, fmt.Errorf("unexpected two-focus semantic step %q", command.SemanticContext.PlanStep)
	}
	payload, err := runtimecontract.EncodeRuntimeSemanticPlanPayloadWithManifest(command.SemanticManifest, document)
	if err != nil {
		return llmevents.RequestRuntimeCandidateResult{}, err
	}
	return runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{
		DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity,
		ExpectedDocumentHash: runtimeExpectedDocumentHash(command.ExpectedDocument),
		DocumentOutcome:      runtimeCompleteDocumentOutcome(command.ExpectedDocument, payload),
	}), nil
}

func runtimeTwoFocusSemanticSharedTarget(command llmevents.RequestRuntimeCandidateCommand) (string, error) {
	ownerTargets := make(map[string]map[string]struct{}, 2)
	for _, catalog := range command.SemanticManifest.OwnerCatalogs {
		if catalog.OwnerModuleID != runtimecontract.RuntimeModuleFile && catalog.OwnerModuleID != runtimecontract.RuntimeModuleShell {
			continue
		}
		targets := make(map[string]struct{}, len(catalog.TargetRefs))
		for _, target := range catalog.TargetRefs {
			targets[target] = struct{}{}
		}
		ownerTargets[catalog.OwnerModuleID] = targets
	}
	sharedTargets := make([]string, 0)
	for target := range ownerTargets[runtimecontract.RuntimeModuleFile] {
		if _, shared := ownerTargets[runtimecontract.RuntimeModuleShell][target]; shared {
			sharedTargets = append(sharedTargets, target)
		}
	}
	sort.Strings(sharedTargets)
	for _, target := range sharedTargets {
		if target == "focus-1.txt" {
			return target, nil
		}
	}
	if len(sharedTargets) != 0 {
		return sharedTargets[0], nil
	}
	return "", fmt.Errorf("two-focus semantic manifest has no unused File/Shell target intersection")
}

func TestDriveRuntimeCompletesTypedPathAndIsRestartIdempotent(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	bootstrap := runtimeBootstrap()
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2",
		DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive,
	}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	startRuntimeReceiptTestOwner(t, hub)

	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{
			ID: "ws-v2", DeliverableDirectory: t.TempDir(), InternalDirectory: t.TempDir(),
		}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })

	fileObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleFile, hub)
	fileObserver.Subscribe(fileevents.TopicAcquireRuntimeCapabilities, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(fileevents.AcquireRuntimeCapabilitiesCommand)
		response, err := fileevents.ResolveRuntimeCapabilities(command, runtimecontract.RuntimeModuleFile, runtimecontract.StepKindFileEdit)
		if err != nil {
			t.Errorf("resolve capabilities: %v", err)
			return
		}
		result.Set(response, nil)
	})
	operationCount := 0
	fileObserver.Subscribe(fileevents.TopicExecuteFileEditOperation, func(event magicevent.Event, result magicevent.Result) {
		operationCount++
		command := event.Data().(fileevents.ExecuteFileEditOperationCommand)
		receipt := orchestratorevents.NewRuntimeOperationReceipt(command.Execution, runtimecontract.RuntimeModuleFile, command.OperationID, runtimecontract.RuntimeOperationExecutionStatusAccepted, command.TargetRefs, runtimecontract.RuntimeOperationEvidence{Summary: "状态变更已完成"}, nil, nil, "2026-07-20T00:00:00Z")
		committed, err := commitRuntimeOperationReceiptForTest(event.Context(), hub, runtimecontract.RuntimeModuleFile, command.WorkspaceID, command.RunID, command.DispatchID, command.Execution, receipt)
		if err != nil {
			t.Errorf("commit owner receipt: %v", err)
			return
		}
		result.Set(fileevents.ExecuteOperationResult{
			Receipt: committed,
		}, nil)
	})
	t.Cleanup(func() {
		fileObserver.Unsubscribe(fileevents.TopicExecuteFileEditOperation)
		fileObserver.Unsubscribe(fileevents.TopicAcquireRuntimeCapabilities)
	})
	runtimeTestPersistBootstrappedOperationFixture(t, ctx, hub, "ws-v2", "run-v2", bootstrap)

	candidateCount := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		candidateCount++
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		if command.Decision.State == runtimecontract.ConvergenceStateNeedAssessment {
			result.Set(runtimeCandidateResultForTest(t, command, runtimeSatisfiedAssessmentCandidate(command, "file evidence satisfies current coverage")), nil)
			return
		}
		candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
			ID: "write-1", Kind: runtimecontract.StepKindFileEdit,
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"},
			Parameters: runtimeFileEditParameters("target.txt"),
		}}}}}
		result.Set(runtimeCandidateResultForTest(t, command, candidate), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })

	module := newRuntimeDriveTestModule(t, hub)
	driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntime, module.handleRuntimeDrive)

	command := orchestratorevents.DriveRuntimeCommand{WorkspaceID: "ws-v2", RunID: "run-v2"}
	completed, err := DriveRuntime(ctx, hub, command)
	if err != nil {
		t.Fatal(err)
	}
	if completed.State != runtimecontract.ConvergenceStateComplete || completed.Terminal == nil {
		t.Fatalf("completed drive result = %+v", completed)
	}
	if candidateCount != 2 || operationCount != 1 {
		t.Fatalf("candidateCount=%d operationCount=%d, want 2/1", candidateCount, operationCount)
	}

	replayed, err := DriveRuntime(ctx, hub, command)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.StateRevision != completed.StateRevision || replayed.AuthoritySnapshotID != completed.AuthoritySnapshotID || candidateCount != 2 || operationCount != 1 {
		t.Fatalf("replayed=%+v completed=%+v counts=%d/%d", replayed, completed, candidateCount, operationCount)
	}

	driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntime)
	restarted := newRuntimeDriveTestModule(t, hub)
	driveObserver = magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntime, restarted.handleRuntimeDrive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntime) })
	recovered, err := DriveRuntime(ctx, hub, command)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.StateRevision != completed.StateRevision || recovered.AuthoritySnapshotID != completed.AuthoritySnapshotID || candidateCount != 2 || operationCount != 1 {
		t.Fatalf("recovered=%+v completed=%+v counts=%d/%d", recovered, completed, candidateCount, operationCount)
	}
}

func TestDriveRuntimeReturnsPersistedGovernedWait(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	bootstrap := runtimeBootstrap()
	bootstrap.Definition.CapabilityContract = append(bootstrap.Definition.CapabilityContract, runtimecontract.RuntimeBlockKindDecisionRequest)
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2",
		DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive,
	}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })

	decisionOwner := decisionmodule.New()
	if err := decisionOwner.Configure(decisionbiz.Dependencies{OwnerRepositories: testDecisionOwnerRepositories(
		memory.NewDecisionTicketRepository(), memory.NewDecisionResolutionRepository(), memory.NewDecisionWorkspaceDeletionRepository()), EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := decisionOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = decisionOwner.Stop(ctx) })

	module := newRuntimeDriveTestModule(t, hub)
	facts := runtimeAggregate(t, 1).Facts
	facts.Capabilities.GovernanceKinds = []string{runtimecontract.RuntimeBlockKindDecisionRequest}
	governanceFailure := runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassGovernanceDependency, Signature: "external-approval", Message: "需要外部裁决",
		GovernanceDependency: &runtimecontract.ExternalDependency{Kind: "external-approval", SubjectRef: "subject-1", Reason: "需要外部裁决"},
	}
	initialized, err := module.InitializeRuntime(ctx, facts, bootstrap.Definition, nil)
	if err != nil {
		t.Fatal(err)
	}
	initialized = runtimeTestPersistSealedSemanticAuthority(t, ctx, runtimeStateRepository{hub: hub}, initialized)
	initialized, err = module.runtimeRecordFailure(ctx, initialized, governanceFailure)
	if err != nil || initialized.Decision.State != runtimecontract.ConvergenceStateNeedGovernance {
		t.Fatalf("persist governance dependency aggregate=%+v err=%v", initialized, err)
	}

	requestCount := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		requestCount++
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		candidate := runtimecontract.RuntimeCandidate{Governance: &runtimecontract.GovernanceCandidate{ExternalDependency: runtimecontract.ExternalDependency{
			Kind: "external-approval", SubjectRef: "subject-1", Reason: "需要外部裁决",
		}}}
		result.Set(runtimeCandidateResultForTest(t, command, candidate), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })

	driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntime, module.handleRuntimeDrive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntime) })
	command := orchestratorevents.DriveRuntimeCommand{WorkspaceID: "ws-v2", RunID: "run-v2"}
	waiting, err := DriveRuntime(ctx, hub, command)
	if err != nil {
		t.Fatal(err)
	}
	if waiting.State != runtimecontract.ConvergenceStateNeedGovernance || waiting.Terminal != nil || waiting.BlockingTicketID == "" || requestCount != 0 {
		t.Fatalf("waiting=%+v terminal=%+v requestCount=%d", waiting, waiting.Terminal, requestCount)
	}
	replayed, err := DriveRuntime(ctx, hub, command)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.StateRevision != waiting.StateRevision || replayed.BlockingTicketID != waiting.BlockingTicketID || requestCount != 0 {
		t.Fatalf("replayed=%+v waiting=%+v requestCount=%d", replayed, waiting, requestCount)
	}
}

func TestRuntimeProviderAuthenticationCreatesConfigurationDependency(t *testing.T) {
	facts := runtimecontract.RuntimeFacts{
		Focus:   runtimecontract.CoverageFocus{ID: "focus-1"},
		Failure: &runtimecontract.FailureFact{Class: runtimecontract.FailureClassProviderAuthentication, Message: "invalid provider credential"},
		Resources: runtimecontract.RuntimeResourceFacts{ProviderCeiling: &runtimecontract.ProviderCeiling{
			Selection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "model-1"}},
		}},
	}
	dependency, found := runtime.GovernanceDependencyForAggregate(runtime.RuntimeAggregate{Facts: facts})
	if !found || dependency.Kind != string(runtimecontract.GovernanceConfiguration) || dependency.SubjectRef != "ai-proxy/model-1" || dependency.Reason != "provider authentication requires configuration" {
		t.Fatalf("configuration dependency = %+v", dependency)
	}
}

func TestRuntimeGovernanceDependencyPreservesSuspendedOperationBinding(t *testing.T) {
	aggregate := runtime.RuntimeAggregate{
		Facts: runtimecontract.RuntimeFacts{
			Focus: runtimecontract.CoverageFocus{ID: "focus-1"},
			Failure: &runtimecontract.FailureFact{
				Class: runtimecontract.FailureClassGovernanceRequired, Message: "approval required",
				GovernanceDependency: &runtimecontract.ExternalDependency{
					Kind: string(runtimecontract.GovernanceApproval), SubjectRef: ".env", Reason: "approval required",
					RequestedAction: "file/write", AuthorizationContext: workorcheventpayload.New(workorcheventpayload.StringField("targetRef", ".env")),
				},
			},
		},
		OperationGovernanceBindings: []runtimecontract.RuntimeOperationGovernanceBinding{{
			ExecutionKey: "exec-1", Fence: 1, DependencyKind: string(runtimecontract.GovernanceApproval), SubjectRef: ".env",
			RequestedAction: "file/write", AuthorizationHash: runtimecontract.StableRuntimeAuthorizationContextHash(workorcheventpayload.New(workorcheventpayload.StringField("targetRef", ".env"))),
			State: runtimecontract.RuntimeOperationGovernanceBindingSuspended,
		}},
	}
	dependency, found := runtime.GovernanceDependencyForAggregate(aggregate)
	if !found || dependency.Kind != string(runtimecontract.GovernanceApproval) || dependency.SubjectRef != ".env" || dependency.Reason != "approval required" {
		t.Fatalf("operation governance dependency = %+v", dependency)
	}
}

func TestRuntimeGovernanceDependencyRejectsGenericFailure(t *testing.T) {
	aggregate := runtime.RuntimeAggregate{Facts: runtimecontract.RuntimeFacts{
		Focus: runtimecontract.CoverageFocus{ID: "focus-1"},
		Failure: &runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassGovernanceRequired, Message: "runtime requires governance",
		},
	}}
	if dependency, found := runtime.GovernanceDependencyForAggregate(aggregate); found || !runtimecontract.SameExternalDependency(dependency, runtimecontract.ExternalDependency{}) {
		t.Fatalf("generic governance dependency = %+v found=%v, want none", dependency, found)
	}
}

func TestDriveRuntimeCompleteSchemaFailureStopsAfterSameContractRetry(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	bootstrap := runtimeBootstrap()
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2",
		DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive,
	}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })

	module := newRuntimeDriveTestModule(t, hub)
	facts := runtimeAggregate(t, 1).Facts
	facts.Capabilities.GovernanceKinds = []string{runtimecontract.RuntimeBlockKindDecisionRequest}
	// A later candidate contract consumes its bounded attempt 0/1 budget. Any
	// resource freshness work is represented by canonical persisted prerequisite
	// invalidation/observation facts, never by a caller-injected action override.
	initialized, err := module.InitializeRuntime(ctx, facts, bootstrap.Definition, nil)
	if err != nil {
		t.Fatal(err)
	}
	initialized = runtimeTestPersistSealedSemanticAuthority(t, ctx, runtimeStateRepository{hub: hub}, initialized)

	requestCount := 0
	var requests []llmevents.RequestRuntimeCandidateCommand
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		requestCount++
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		requests = append(requests, command)
		result.Set(runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{
			Transaction: command.Transaction, DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity,
			ExpectedDocumentHash: runtimeExpectedDocumentHash(command.ExpectedDocument),
			DocumentOutcome:      runtimeCompleteDocumentOutcome(command.ExpectedDocument, []byte(`{"unexpected":true}`)),
		}), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })

	driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntime, module.handleRuntimeDrive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntime) })

	completed, err := DriveRuntime(ctx, hub, orchestratorevents.DriveRuntimeCommand{WorkspaceID: "ws-v2", RunID: "run-v2"})
	if err != nil {
		t.Fatal(err)
	}
	if requestCount != 2 || completed.State != runtimecontract.ConvergenceStateFailed || completed.Terminal == nil || completed.Terminal.Code != "candidate_retry_exhausted" {
		t.Fatalf("completed=%+v requestCount=%d, want bounded terminal after two requests", completed, requestCount)
	}
	if len(requests) != 2 || requests[0].PreviousFailure != nil || requests[1].PreviousFailure == nil ||
		requests[1].PreviousFailure.Class != runtimecontract.RuntimeCandidateFailureClassSchema || requests[1].PreviousFailure.Code != "candidate_schema_invalid" ||
		requests[1].PreviousFailure.Signature != "candidate-schema-invalid" || requests[1].PreviousFailure.Reason == "" {
		t.Fatalf("candidate retry did not carry the persisted first failure: %+v", requests)
	}
}

func TestRequestAndSealRuntimeCandidateRecoversProviderLimitedOutputWithCompleteDocument(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	facts := runtimeAggregate(t, 1).Facts
	facts.Activation.ExecutionContract.CandidateDocumentRecovery = runtimecontract.RuntimeCandidateDocumentRecoveryPolicy{MaxAttempts: 2}
	repository := &inMemoryRuntimeStateRepository{}
	controller, err := runtime.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := runtimeTestControllerInitialize(controller, ctx, facts, runtimeDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	initialized = runtimeTestPersistSealedSemanticAuthority(t, ctx, repository, initialized)
	if len(initialized.Facts.Focus.CoverageEvidenceBindings) != 1 {
		t.Fatalf("candidate recovery fixture bindings = %+v", initialized.Facts.Focus.CoverageEvidenceBindings)
	}
	continuations := memory.NewArtifactContinuationRepository()
	owner := newRuntimeTestOwnerWithEvidence(RuntimeEvidenceRepositories{ArtifactContinuations: continuations})
	if err := owner.ConfigureRuntimeRepository(repository); err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(owner, hub, nil)
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(initialized.Definition, initialized.Facts.Activation.ExecutionContract)
	first := fmt.Sprintf(`{"operations":[{"id":"op-1","kind":"file/edit","bindingRefs":[%q],`, initialized.Facts.Focus.CoverageEvidenceBindings[0].ID)
	second := `"targetRefs":["target.txt"],"parameters":[{"name":"oldText","string":"old"},{"name":"newText","string":"new"}]}]}`
	calls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		calls++
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		response := llmevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity, ExpectedDocumentHash: runtimeExpectedDocumentHash(command.ExpectedDocument)}
		if calls == 1 {
			response.DocumentOutcome = &llmevents.RuntimeProviderDocumentOutcome{
				Framing: runtimecontract.RuntimeProviderDocumentFraming{
					Disposition: runtimecontract.RuntimeProviderDocumentIncomplete, ExpectedDocumentHash: command.ExpectedDocument.Hash,
					ResponseHash: runtimecontract.StableRuntimeProviderResponseHash([]byte(first)), ResponseBytes: int64(len(first)), Code: "max_output_tokens", Signature: "provider-output-limit",
				},
			}
		} else {
			if command.RequestIdentity.Sequence != 2 || command.ExpectedDocument.Hash != response.ExpectedDocumentHash || command.StablePrefixHash != prefix {
				t.Fatalf("whole-document recovery command = %+v", command)
			}
			document := []byte(first + second)
			response.DocumentOutcome = runtimeCompleteDocumentOutcome(command.ExpectedDocument, document)
		}
		result.Set(runtimeProviderResultWithAuditForTest(command, response), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })
	completed, err := module.requestAndSealRuntimeCandidateWithRetry(ctx, initialized, prefix)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || completed.ActiveDispatch == nil || len(completed.CandidateAttempts) != 1 || completed.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateAccepted {
		t.Fatalf("candidate document recovery completion = %+v calls=%d", completed, calls)
	}
	if len(completed.CandidateRequestSessions) != 1 || completed.CandidateRequestSessions[0].State != runtimecontract.RuntimeCandidateRequestSessionDocumentComplete || completed.CandidateRequestSessions[0].NextSequence != 2 || completed.CandidateRequestSessions[0].IncompleteOutcomes != 1 {
		t.Fatalf("persisted C4 request session = %+v", completed.CandidateRequestSessions)
	}
	id := runtimecontract.StableRuntimeCandidateRequestContentID(completed.CandidateRequestSessions[0].LogicalRequestID)
	continuation, err := continuations.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if continuation.State != common.ArtifactContinuationStateApplied || !continuation.FinalReceived || continuation.AuthorityTransaction != initialized.Decision.AuthorityTransaction() || continuation.ArtifactID != completed.CandidateRequestSessions[0].LogicalRequestID {
		t.Fatalf("persisted complete candidate document = %+v", continuation)
	}
}

func TestRequestAndSealRuntimeCandidateCommitsSessionBeforeProviderAndAttemptAfterDocument(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	facts := runtimeAggregate(t, 1).Facts
	state := &inMemoryRuntimeStateRepository{}
	controller, err := runtime.NewRuntimeController(state)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := runtimeTestControllerInitialize(controller, ctx, facts, runtimeDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	initialized = runtimeTestPersistSealedSemanticAuthority(t, ctx, state, initialized)
	owner := newRuntimeTestOwnerWithEvidence(RuntimeEvidenceRepositories{ArtifactContinuations: memory.NewArtifactContinuationRepository()})
	if err := owner.ConfigureRuntimeRepository(state); err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(owner, hub, nil)
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(initialized.Definition, initialized.Facts.Activation.ExecutionContract)
	bindingID := initialized.Facts.Focus.CoverageEvidenceBindings[0].ID
	document := []byte("```json\n{\"operations\":[{\"id\":\"op-1\",\"kind\":\"file/edit\",\"bindingRefs\":[\"" + bindingID + "\"],\"targetRefs\":[\"target.txt\"],\"parameters\":[{\"name\":\"oldText\",\"string\":\"old\"},{\"name\":\"newText\",\"string\":\"new\"}]}]}\n```")
	providerCalls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		providerCalls++
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		persisted, loadErr := state.Load(ctx, command.WorkspaceID, command.RunID)
		if loadErr != nil {
			t.Errorf("load session before provider call: %v", loadErr)
		} else if len(persisted.CandidateRequestSessions) != 1 || !persisted.CandidateRequestSessions[0].ProviderSubmitted || persisted.CandidateRequestSessions[0].LastPhysicalCallID != command.RequestIdentity.PhysicalCallID || len(persisted.CandidateAttempts) != 0 {
			t.Errorf("provider call did not observe committed request session without candidate attempt: %+v", persisted)
		}
		result.Set(runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{
			Transaction: command.Transaction, DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity,
			ExpectedDocumentHash: runtimeExpectedDocumentHash(command.ExpectedDocument),
			DocumentOutcome:      runtimeCompleteDocumentOutcome(command.ExpectedDocument, document),
		}), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })
	completed, err := module.requestAndSealRuntimeCandidateWithRetry(ctx, initialized, prefix)
	if err != nil {
		t.Fatal(err)
	}
	if providerCalls != 1 || completed.ActiveDispatch == nil || len(completed.CandidateAttempts) != 1 || completed.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateAccepted || len(completed.CandidateRequestSessions) != 1 || completed.CandidateRequestSessions[0].State != runtimecontract.RuntimeCandidateRequestSessionDocumentComplete {
		t.Fatalf("session-first candidate completion = %+v calls=%d", completed, providerCalls)
	}
}

func TestRequestAndSealRuntimeCandidateRetriesTransportWithoutCandidateAttempt(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	facts := runtimeAggregate(t, 1).Facts
	state := &inMemoryRuntimeStateRepository{}
	controller, err := runtime.NewRuntimeController(state)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := runtimeTestControllerInitialize(controller, ctx, facts, runtimeDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	initialized = runtimeTestPersistSealedSemanticAuthority(t, ctx, state, initialized)
	owner := newRuntimeTestOwnerWithEvidence(RuntimeEvidenceRepositories{ArtifactContinuations: memory.NewArtifactContinuationRepository()})
	if err := owner.ConfigureRuntimeRepository(state); err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(owner, hub, nil)
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(initialized.Definition, initialized.Facts.Activation.ExecutionContract)
	bindingID := initialized.Facts.Focus.CoverageEvidenceBindings[0].ID
	document := []byte(`{"operations":[{"id":"op-1","kind":"file/edit","bindingRefs":["` + bindingID + `"],"targetRefs":["target.txt"],"parameters":[{"name":"oldText","string":"old"},{"name":"newText","string":"new"}]}]}`)
	var calls []llmevents.RequestRuntimeCandidateCommand
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		calls = append(calls, command)
		if len(calls) == 1 {
			persisted, loadErr := state.Load(ctx, command.WorkspaceID, command.RunID)
			if loadErr != nil || len(persisted.CandidateAttempts) != 0 || len(persisted.CandidateRequestSessions) != 1 || !persisted.CandidateRequestSessions[0].ProviderSubmitted {
				t.Errorf("first transport call did not observe prepared/submitted session only: aggregate=%+v err=%v", persisted, loadErr)
			}
			result.Set(runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{
				Transaction: command.Transaction, DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity,
				ExpectedDocumentHash: runtimeExpectedDocumentHash(command.ExpectedDocument),
				Feedback:             runtimeProviderFeedbackForTest(t, command, runtimecontract.RuntimeCandidateFailure{Class: runtimecontract.RuntimeCandidateFailureClassProviderTransient, Code: "provider_body_idle_timeout", Signature: "provider-body-idle-timeout/exact", Reason: "provider body became idle"}),
			}), nil)
			return
		}
		result.Set(runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{
			Transaction: command.Transaction, DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity,
			ExpectedDocumentHash: runtimeExpectedDocumentHash(command.ExpectedDocument),
			DocumentOutcome:      runtimeCompleteDocumentOutcome(command.ExpectedDocument, document),
		}), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })
	completed, err := module.requestAndSealRuntimeCandidateWithRetry(ctx, initialized, prefix)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || calls[0].RequestIdentity.LogicalRequestID != calls[1].RequestIdentity.LogicalRequestID || calls[0].RequestIdentity.Sequence != 1 || calls[1].RequestIdentity.Sequence != 2 || calls[0].RequestIdentity.PhysicalCallID == calls[1].RequestIdentity.PhysicalCallID || completed.Facts.Recovery.ProviderTransportAttempts != 1 || len(completed.CandidateAttempts) != 1 || completed.ActiveDispatch == nil || len(completed.CandidateRequestSessions) != 1 || completed.CandidateRequestSessions[0].TransportOutcomes != 1 {
		t.Fatalf("transport retry did not preserve request identity and candidate budget: calls=%+v aggregate=%+v", calls, completed)
	}
}

func TestRequestAndSealRuntimeCandidateBodyIdleExhaustionReloadsWithoutProviderReplay(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	facts := runtimeAggregate(t, 1).Facts
	facts.RecoveryPolicy.MaxProviderTransportAttempts = 1
	state := &inMemoryRuntimeStateRepository{}
	controller, err := runtime.NewRuntimeController(state)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := runtimeTestControllerInitialize(controller, ctx, facts, runtimeDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	initialized = runtimeTestPersistSealedSemanticAuthority(t, ctx, state, initialized)
	continuations := memory.NewArtifactContinuationRepository()
	owner := newRuntimeTestOwnerWithEvidence(RuntimeEvidenceRepositories{ArtifactContinuations: continuations})
	if err := owner.ConfigureRuntimeRepository(state); err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(owner, hub, nil)
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(initialized.Definition, initialized.Facts.Activation.ExecutionContract)
	providerCalls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		providerCalls++
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		result.Set(runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{
			Transaction: command.Transaction, DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity,
			ExpectedDocumentHash: runtimeExpectedDocumentHash(command.ExpectedDocument),
			Feedback: runtimeProviderFeedbackForTest(t, command, runtimecontract.RuntimeCandidateFailure{
				Class: runtimecontract.RuntimeCandidateFailureClassProviderTransient, Code: "provider_body_idle_timeout",
				Signature: "provider-body-idle-timeout/exact", Reason: "provider body became idle",
			}),
		}), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })

	failed, err := module.requestAndSealRuntimeCandidateWithRetry(ctx, initialized, prefix)
	if err != nil {
		t.Fatal(err)
	}
	if providerCalls != 2 || failed.Decision.State != runtimecontract.ConvergenceStateFailed || failed.Facts.Failure == nil || failed.Facts.Failure.Class != runtimecontract.FailureClassProviderTransient || failed.Facts.Failure.Code != "provider_transport_exhausted" || failed.Facts.Failure.OriginalDiagnostic == nil || failed.Facts.Failure.OriginalDiagnostic.OriginalCode != "provider_body_idle_timeout" || failed.Facts.Failure.OriginalDiagnostic.Signature != "provider-body-idle-timeout/exact" || failed.Facts.Recovery.ProviderTransportAttempts != 1 || len(failed.CandidateAttempts) != 0 || failed.ActiveDispatch != nil {
		t.Fatalf("body-idle exhaustion did not retain atomic transport authority: calls=%d aggregate=%+v", providerCalls, failed)
	}
	session := failed.CandidateRequestSessions[0]
	if session.State != runtimecontract.RuntimeCandidateRequestSessionTerminal || session.LastOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomeProviderTransient || session.LastOutcome.Code != "provider_body_idle_timeout" || session.LastOutcome.Signature != "provider-body-idle-timeout/exact" || session.TransportOutcomes != 2 {
		t.Fatalf("body-idle terminal session rewrote LLM fact: %+v", session)
	}
	reloaded, err := state.Load(ctx, failed.Facts.WorkspaceID, failed.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	restartedOwner := newRuntimeTestOwnerWithEvidence(RuntimeEvidenceRepositories{ArtifactContinuations: continuations})
	if err := restartedOwner.ConfigureRuntimeRepository(state); err != nil {
		t.Fatal(err)
	}
	replayed, err := newTestBiz(restartedOwner, hub, nil).requestAndSealRuntimeCandidateWithRetry(ctx, reloaded, prefix)
	if err != nil {
		t.Fatal(err)
	}
	if providerCalls != 2 || replayed.StateRevision != failed.StateRevision || replayed.Facts.Failure == nil || replayed.Facts.Failure.OriginalDiagnostic == nil || replayed.Facts.Failure.OriginalDiagnostic.OriginalCode != "provider_body_idle_timeout" {
		t.Fatalf("fresh body-idle terminal reload replayed provider or changed failure: calls=%d aggregate=%+v", providerCalls, replayed)
	}
}

func TestRequestAndSealRuntimeCandidateRestartsFromPersistedIncompleteOutcome(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	facts := runtimeAggregate(t, 1).Facts
	facts.Activation.ExecutionContract.CandidateDocumentRecovery = runtimecontract.RuntimeCandidateDocumentRecoveryPolicy{MaxAttempts: 2}
	state := &inMemoryRuntimeStateRepository{}
	controller, err := runtime.NewRuntimeController(state)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := runtimeTestControllerInitialize(controller, ctx, facts, runtimeDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	initialized = runtimeTestPersistSealedSemanticAuthority(t, ctx, state, initialized)
	continuations := memory.NewArtifactContinuationRepository()
	owner := newRuntimeTestOwnerWithEvidence(RuntimeEvidenceRepositories{ArtifactContinuations: continuations})
	if err := owner.ConfigureRuntimeRepository(state); err != nil {
		t.Fatal(err)
	}
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(initialized.Definition, initialized.Facts.Activation.ExecutionContract)
	candidateContext := runtimecontract.RuntimeCandidateContextForDecision(initialized.Facts, initialized.Decision, nil)
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(initialized.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(candidateContext.Resources))
	if err != nil {
		t.Fatalf("build resumed candidate manifest: decision=%+v terminal=%+v failure=%+v err=%v", initialized.Decision, initialized.Decision.Terminal, initialized.Facts.Failure, err)
	}
	prepared, session, err := owner.EnsureRuntimeCandidateRequestSession(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, prefix, manifest.Hash, "")
	if err != nil {
		t.Fatal(err)
	}
	bindingID := initialized.Facts.Focus.CoverageEvidenceBindings[0].ID
	first := `{"operations":[{"id":"op-1","kind":"file/edit","bindingRefs":["` + bindingID + `"],`
	firstPhysicalID := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, 1)
	submitted, err := owner.MarkRuntimeCandidateRequestSubmitted(ctx, prepared.Facts.WorkspaceID, prepared.Facts.RunID, session.LogicalRequestID, 1, firstPhysicalID)
	if err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(owner, hub, nil)
	incomplete := runtimecontract.RuntimeProviderDocumentFraming{
		Disposition: runtimecontract.RuntimeProviderDocumentIncomplete, ExpectedDocumentHash: session.ExpectedDocument.Hash,
		ResponseHash: runtimecontract.StableRuntimeProviderResponseHash([]byte(first)), ResponseBytes: int64(len(first)), Code: "max_output_tokens", Signature: "provider-output-limit",
	}
	audit := runtimecontract.ProviderRequestAuditView{
		RequestID: firstPhysicalID, LogicalRequestID: session.LogicalRequestID, PhysicalCallID: firstPhysicalID,
		WorkspaceID: string(submitted.Facts.WorkspaceID), RunID: string(submitted.Facts.RunID), Status: "failed",
		ProviderSubmitted: true, PhysicalResponseMode: llmaudit.PhysicalResponseModeNonStreaming, RequestBytes: 1,
		ContextBudget: llmaudit.ContextBudget{ProviderID: "test-provider", ModelID: "test-model", ProviderSubmitted: true},
	}
	settlement, err := runtimeProviderBudgetSettlement(string(submitted.Facts.WorkspaceID), string(submitted.Facts.RunID), session.LogicalRequestID, firstPhysicalID, &audit, nil)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := module.runtimeCoordinateProviderRequestRecovery(ctx, submitted, session.LogicalRequestID, firstPhysicalID, runtime.RuntimeProviderRequestRecoveryInput{
		DocumentKind: runtimecontract.RuntimeProviderDocumentOperations, DocumentFraming: &incomplete,
	}, settlement)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		calls++
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		if command.RequestIdentity.Sequence != 2 {
			t.Fatalf("restart whole-document recovery command = %+v", command)
		}
		document := []byte(first + `"targetRefs":["target.txt"],"parameters":[{"name":"oldText","string":"old"},{"name":"newText","string":"new"}]}]}`)
		result.Set(runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity, ExpectedDocumentHash: runtimeExpectedDocumentHash(command.ExpectedDocument), DocumentOutcome: runtimeCompleteDocumentOutcome(command.ExpectedDocument, document)}), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })
	restarted := newTestBiz(owner, hub, nil)
	completed, err := restarted.requestAndSealRuntimeCandidateWithRetry(ctx, pending, prefix)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || completed.ActiveDispatch == nil || len(completed.CandidateAttempts) != 1 || completed.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateAccepted || len(completed.CandidateRequestSessions) != 1 || completed.CandidateRequestSessions[0].NextSequence != 2 || completed.CandidateRequestSessions[0].IncompleteOutcomes != 1 {
		var attemptFailure *runtimecontract.RuntimeCandidateFailure
		if len(completed.CandidateAttempts) != 0 {
			attemptFailure = completed.CandidateAttempts[0].Failure
		}
		t.Fatalf("restart continuation completion: decision=%+v terminal=%+v factsFailure=%+v attemptFailure=%+v activeDispatch=%+v calls=%d", completed.Decision, completed.Decision.Terminal, completed.Facts.Failure, attemptFailure, completed.ActiveDispatch, calls)
	}
}

// P3-B3: a complete C4 document is durable before C5 allocates attempt 0.
// Recreating the owner, state adapter, and content repository must therefore
// decode/admit that document exactly once without another provider request.
func TestRuntimeFileBackedCompleteDocumentRestartAdmitsWithoutProviderReplay(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	root := t.TempDir()
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	bootstrap := runtimeBootstrap()
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
		DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive,
	}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })

	contentsRoot := filepath.Join(root, "candidate-contents")
	firstOwner := newRuntimeTestOwnerWithEvidence(RuntimeEvidenceRepositories{
		ArtifactContinuations: filerepository.NewArtifactContinuationRepository(contentsRoot),
	})
	runtimeRepository := runtimeStateRepository{hub: hub}
	if err := firstOwner.ConfigureRuntimeRepository(runtimeRepository); err != nil {
		t.Fatal(err)
	}
	initialized := runtimeTestInitializeOperationFixture(t, ctx, firstOwner, runtimeRepository, runtimeAggregate(t, 1).Facts, runtimeDefinition(), nil)
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(initialized.Definition, initialized.Facts.Activation.ExecutionContract)
	candidateContext := runtimecontract.RuntimeCandidateContextForDecision(initialized.Facts, initialized.Decision, nil)
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(initialized.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(candidateContext.Resources))
	if err != nil {
		t.Fatal(err)
	}
	_, session, err := firstOwner.EnsureRuntimeCandidateRequestSession(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, prefix, manifest.Hash, "")
	if err != nil {
		t.Fatal(err)
	}
	physicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, session.NextSequence)
	submitted, err := firstOwner.MarkRuntimeCandidateRequestSubmitted(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, session.LogicalRequestID, session.NextSequence, physicalCallID)
	if err != nil {
		t.Fatal(err)
	}
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "reloaded-edit", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeFileEditParameters("target.txt"),
	}}}}}
	bareDocument := runtimeCandidateDocumentForTest(t, manifest, candidate)
	document := []byte("```json\n" + string(bareDocument) + "\n```")
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
	if err := runOwner.Configure(runbiz.Dependencies{Runs: filerepository.NewRunRepository(filepath.Join(root, "runs.json")), EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	restartedContents := filerepository.NewArtifactContinuationRepository(contentsRoot)
	restartedOwner := newRuntimeTestOwnerWithEvidence(RuntimeEvidenceRepositories{
		ArtifactContinuations: restartedContents,
	})
	if err := restartedOwner.ConfigureRuntimeRepository(runtimeStateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	reloaded, err := restartedOwner.loadRuntime(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	providerCalls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(_ magicevent.Event, _ magicevent.Result) {
		providerCalls++
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })
	sealed, err := newTestBiz(restartedOwner, hub, nil).requestAndSealRuntimeCandidateWithRetry(ctx, reloaded, prefix)
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
func TestRuntimeFileBackedPrepareRestartSubmitsOnce(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	root := t.TempDir()
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	bootstrap := runtimeBootstrap()
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
		DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive,
	}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })

	contentsRoot := filepath.Join(root, "candidate-contents")
	firstOwner := newRuntimeTestOwnerWithEvidence(RuntimeEvidenceRepositories{
		ArtifactContinuations: filerepository.NewArtifactContinuationRepository(contentsRoot),
	})
	runtimeRepository := runtimeStateRepository{hub: hub}
	if err := firstOwner.ConfigureRuntimeRepository(runtimeRepository); err != nil {
		t.Fatal(err)
	}
	initialized := runtimeTestInitializeOperationFixture(t, ctx, firstOwner, runtimeRepository, runtimeAggregate(t, 1).Facts, runtimeDefinition(), nil)
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(initialized.Definition, initialized.Facts.Activation.ExecutionContract)
	candidateContext := runtimecontract.RuntimeCandidateContextForDecision(initialized.Facts, initialized.Decision, nil)
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(initialized.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(candidateContext.Resources))
	if err != nil {
		t.Fatal(err)
	}
	prepared, session, err := firstOwner.EnsureRuntimeCandidateRequestSession(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, prefix, manifest.Hash, "")
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
	if err := runOwner.Configure(runbiz.Dependencies{Runs: filerepository.NewRunRepository(filepath.Join(root, "runs.json")), EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	restartedOwner := newRuntimeTestOwnerWithEvidence(RuntimeEvidenceRepositories{
		ArtifactContinuations: filerepository.NewArtifactContinuationRepository(contentsRoot),
	})
	if err := restartedOwner.ConfigureRuntimeRepository(runtimeStateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	reloaded, err := restartedOwner.loadRuntime(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	providerCalls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		providerCalls++
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		if command.RequestIdentity.LogicalRequestID != session.LogicalRequestID || command.RequestIdentity.Sequence != 1 || command.RequestIdentity.PhysicalCallID != runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, 1) {
			t.Errorf("restart changed prepared request identity: %+v", command)
		}
		candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
			ID: "prepared-restart-edit", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeFileEditParameters("target.txt"),
		}}}}}
		document := runtimeCandidateDocumentForTest(t, command.CandidateManifest, candidate)
		result.Set(runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{
			Transaction: command.Transaction, DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity,
			ExpectedDocumentHash: runtimeExpectedDocumentHash(command.ExpectedDocument),
			DocumentOutcome:      runtimeCompleteDocumentOutcome(command.ExpectedDocument, document),
		}), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })
	sealed, err := newTestBiz(restartedOwner, hub, nil).requestAndSealRuntimeCandidateWithRetry(ctx, reloaded, prefix)
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
func TestRuntimeFileBackedSubmittedCandidateRestartDoesNotReplayProvider(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	root := t.TempDir()
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	bootstrap := runtimeBootstrap()
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
		DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive,
	}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })

	firstOwner := newRuntimeTestOwnerWithEvidence(RuntimeEvidenceRepositories{
		ArtifactContinuations: filerepository.NewArtifactContinuationRepository(filepath.Join(root, "candidate-contents")),
	})
	runtimeRepository := runtimeStateRepository{hub: hub}
	if err := firstOwner.ConfigureRuntimeRepository(runtimeRepository); err != nil {
		t.Fatal(err)
	}
	initialized := runtimeTestInitializeOperationFixture(t, ctx, firstOwner, runtimeRepository, runtimeAggregate(t, 1).Facts, runtimeDefinition(), nil)
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(initialized.Definition, initialized.Facts.Activation.ExecutionContract)
	context := runtimecontract.RuntimeCandidateContextForDecision(initialized.Facts, initialized.Decision, nil)
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(initialized.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(context.Resources))
	if err != nil {
		t.Fatal(err)
	}
	_, session, err := firstOwner.EnsureRuntimeCandidateRequestSession(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, prefix, manifest.Hash, "")
	if err != nil {
		t.Fatal(err)
	}
	physicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, session.NextSequence)
	if _, err := firstOwner.MarkRuntimeCandidateRequestSubmitted(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, session.LogicalRequestID, session.NextSequence, physicalCallID); err != nil {
		t.Fatal(err)
	}

	if err := runOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	hub.Terminate(ctx)
	hub = magicevent.NewHub(16)
	runOwner = runmodule.New()
	if err := runOwner.Configure(runbiz.Dependencies{Runs: filerepository.NewRunRepository(filepath.Join(root, "runs.json")), EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	restartedOwner := newRuntimeTestOwnerWithEvidence(RuntimeEvidenceRepositories{
		ArtifactContinuations: filerepository.NewArtifactContinuationRepository(filepath.Join(root, "candidate-contents")),
	})
	if err := restartedOwner.ConfigureRuntimeRepository(runtimeStateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	reloaded, err := restartedOwner.loadRuntime(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	providerCalls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(_ magicevent.Event, _ magicevent.Result) { providerCalls++ })
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })
	terminal, err := newTestBiz(restartedOwner, hub, nil).requestAndSealRuntimeCandidateWithRetry(ctx, reloaded, prefix)
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
func TestRuntimeFileBackedStaleCompleteSessionInvalidatesBeforeCandidateReuse(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	root := t.TempDir()
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	bootstrap := runtimeBootstrap()
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
		DefinitionRevision: bootstrap.Definition.Revision, RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStateActive,
	}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runbiz.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })

	facts := runtimeAggregate(t, 1).Facts
	ceiling := runtimecontract.ProviderCeiling{
		Selection: runtimecontract.ProviderInferenceSelection{Target: facts.Activation.ProviderSelection.Target}, ContextWindowTokens: 128000, ProviderMaxOutputTokens: 16384,
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
			SubjectRef: string(ceiling.Selection.Target.ProviderID) + "/" + string(ceiling.Selection.Target.ModelID), Revision: ceiling.ConfigurationRevision,
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	facts.Resources.ProviderCeiling = &ceiling
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{prerequisite}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{observation}

	contentsRoot := filepath.Join(root, "candidate-contents")
	firstOwner := newRuntimeTestOwnerWithEvidence(RuntimeEvidenceRepositories{
		ArtifactContinuations: filerepository.NewArtifactContinuationRepository(contentsRoot),
	})
	runtimeRepository := runtimeStateRepository{hub: hub}
	if err := firstOwner.ConfigureRuntimeRepository(runtimeRepository); err != nil {
		t.Fatal(err)
	}
	initialized := runtimeTestInitializeOperationFixture(t, ctx, firstOwner, runtimeRepository, facts, runtimeDefinition(), nil)
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(initialized.Definition, initialized.Facts.Activation.ExecutionContract)
	candidateContext := runtimecontract.RuntimeCandidateContextForDecision(initialized.Facts, initialized.Decision, nil)
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(initialized.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(candidateContext.Resources))
	if err != nil {
		t.Fatal(err)
	}
	_, session, err := firstOwner.EnsureRuntimeCandidateRequestSession(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, prefix, manifest.Hash, "")
	if err != nil {
		t.Fatal(err)
	}
	physicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, session.NextSequence)
	submitted, err := firstOwner.MarkRuntimeCandidateRequestSubmitted(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, session.LogicalRequestID, session.NextSequence, physicalCallID)
	if err != nil {
		t.Fatal(err)
	}
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "stale-complete-edit", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeFileEditParameters("target.txt"),
	}}}}}
	completed, _, _, err := newTestBiz(firstOwner, hub, nil).persistAndCompleteRuntimeCandidateRequestDocument(ctx, submitted, session.LogicalRequestID, session.NextSequence, physicalCallID, runtimeCandidateDocumentForTest(t, manifest, candidate))
	if err != nil {
		t.Fatal(err)
	}
	if len(completed.CandidateAttempts) != 0 || completed.CandidateRequestSessions[0].State != runtimecontract.RuntimeCandidateRequestSessionDocumentComplete {
		t.Fatalf("stale fixture did not stop at C4 completion: %+v", completed)
	}

	stale := runtimecontract.RuntimePrerequisiteStale{
		PrerequisiteID: prerequisite.ID, ObservationID: observation.ObservationID,
		Domain: runtimecontract.RuntimeResourceProviderConfig, OwnerModuleID: runtimecontract.RuntimeModuleLLM,
		SubjectRef: string(ceiling.Selection.Target.ProviderID) + "/" + string(ceiling.Selection.Target.ModelID), ExpectedRevision: "config-v1", ObservedRevision: "config-v2",
		Reason: "provider configuration changed",
	}
	invalidated, err := firstOwner.InvalidateRuntimeCandidateRequestSessionForPrerequisiteStale(ctx, completed.Facts.WorkspaceID, completed.Facts.RunID, session.LogicalRequestID, stale)
	if err != nil {
		t.Fatal(err)
	}
	if invalidated.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite || len(invalidated.CandidateAttempts) != 0 ||
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
	if err := runOwner.Configure(runbiz.Dependencies{Runs: filerepository.NewRunRepository(filepath.Join(root, "runs.json")), EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	restartedOwner := newRuntimeTestOwnerWithEvidence(RuntimeEvidenceRepositories{
		ArtifactContinuations: filerepository.NewArtifactContinuationRepository(contentsRoot),
	})
	if err := restartedOwner.ConfigureRuntimeRepository(runtimeStateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	reloaded, err := restartedOwner.loadRuntime(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	providerCalls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(_ magicevent.Event, _ magicevent.Result) { providerCalls++ })
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })
	resumed, err := newTestBiz(restartedOwner, hub, nil).requestAndSealRuntimeCandidateWithRetry(ctx, reloaded, prefix)
	if err != nil {
		t.Fatal(err)
	}
	if providerCalls != 0 || resumed.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite || len(resumed.CandidateAttempts) != 0 ||
		resumed.ActiveDispatch != nil || len(resumed.GovernanceRequests) != 0 || resumed.Facts.Repair.RepairAttempts != 0 ||
		resumed.Facts.Recovery.ProviderTransportAttempts != 0 {
		t.Fatalf("stale complete session reused candidate content: providerCalls=%d aggregate=%+v", providerCalls, resumed)
	}
}

func TestRequestAndSealRuntimeCandidateFailsWhenDocumentRecoveryBudgetExhausted(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	facts := runtimeAggregate(t, 1).Facts
	facts.Activation.ExecutionContract.CandidateDocumentRecovery = runtimecontract.RuntimeCandidateDocumentRecoveryPolicy{MaxAttempts: 1}
	state := &inMemoryRuntimeStateRepository{}
	controller, err := runtime.NewRuntimeController(state)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := runtimeTestControllerInitialize(controller, ctx, facts, runtimeDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	initialized = runtimeTestPersistSealedSemanticAuthority(t, ctx, state, initialized)
	continuations := memory.NewArtifactContinuationRepository()
	owner := newRuntimeTestOwnerWithEvidence(RuntimeEvidenceRepositories{ArtifactContinuations: continuations})
	if err := owner.ConfigureRuntimeRepository(state); err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(owner, hub, nil)
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(initialized.Definition, initialized.Facts.Activation.ExecutionContract)
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	calls := 0
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		calls++
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		partial := []byte(`{"operations":[`)
		outcome := llmevents.RuntimeProviderDocumentOutcome{Framing: runtimecontract.RuntimeProviderDocumentFraming{Disposition: runtimecontract.RuntimeProviderDocumentIncomplete, ExpectedDocumentHash: command.ExpectedDocument.Hash, ResponseHash: runtimecontract.StableRuntimeProviderResponseHash(partial), ResponseBytes: int64(len(partial)), Code: "max_output_tokens", Signature: "provider-output-limit"}}
		result.Set(runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity, ExpectedDocumentHash: runtimeExpectedDocumentHash(command.ExpectedDocument), DocumentOutcome: &outcome}), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })
	failed, err := module.requestAndSealRuntimeCandidateWithRetry(ctx, initialized, prefix)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Decision.State != runtimecontract.ConvergenceStateFailed || failed.Decision.Terminal == nil || failed.Decision.Terminal.Code != "candidate_document_recovery_exhausted" {
		t.Fatalf("candidate document recovery exhaustion = %+v", failed.Decision)
	}
	if len(failed.CandidateAttempts) != 0 || failed.ActiveDispatch != nil || failed.Facts.Repair.RepairAttempts != 0 || len(failed.GovernanceRequests) != 0 {
		t.Fatalf("document recovery exhaustion must not allocate a candidate/dispatch/repair/governance: %+v", failed)
	}
	if len(failed.CandidateRequestSessions) != 1 {
		t.Fatalf("candidate request sessions = %+v", failed.CandidateRequestSessions)
	}
	session := failed.CandidateRequestSessions[0]
	if session.State != runtimecontract.RuntimeCandidateRequestSessionTerminal ||
		session.LastOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomeDocumentRecoveryExhausted ||
		session.LastOutcome.Code != "candidate_document_recovery_exhausted" || session.IncompleteOutcomes != 2 {
		t.Fatalf("terminal document recovery session = %+v", session)
	}
	id := runtimeCandidateRequestContentID(t, initialized, prefix, 0)
	continuation, err := continuations.Get(ctx, id)
	if !errors.Is(err, common.ErrNotFound) || continuation != nil {
		t.Fatalf("incomplete whole-document recovery persisted raw content = %+v err=%v", continuation, err)
	}
	reloaded, err := state.Load(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	restartedOwner := newRuntimeTestOwnerWithEvidence(RuntimeEvidenceRepositories{ArtifactContinuations: continuations})
	if err := restartedOwner.ConfigureRuntimeRepository(state); err != nil {
		t.Fatal(err)
	}
	restarted := newTestBiz(restartedOwner, hub, nil)
	resumed, err := restarted.requestAndSealRuntimeCandidateWithRetry(ctx, reloaded, prefix)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.StateRevision != failed.StateRevision || resumed.Decision.Terminal == nil || calls != 2 {
		t.Fatalf("restart must preserve terminal continuation exhaustion without a provider replay: aggregate=%+v calls=%d", resumed, calls)
	}
}

func TestRequestAndSealRuntimeCandidateTerminatesUnknownSubmittedRequestWithoutCandidateAttempt(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	state := &inMemoryRuntimeStateRepository{}
	controller, err := runtime.NewRuntimeController(state)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := runtimeTestControllerInitialize(controller, ctx, runtimeAggregate(t, 1).Facts, runtimeDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	initialized = runtimeTestPersistSealedSemanticAuthority(t, ctx, state, initialized)
	owner := newRuntimeTestOwner()
	if err := owner.ConfigureRuntimeRepository(state); err != nil {
		t.Fatal(err)
	}
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(initialized.Definition, initialized.Facts.Activation.ExecutionContract)
	context := runtimecontract.RuntimeCandidateContextForDecision(initialized.Facts, initialized.Decision, nil)
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(initialized.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(context.Resources))
	if err != nil {
		t.Fatal(err)
	}
	prepared, session, err := owner.EnsureRuntimeCandidateRequestSession(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, prefix, manifest.Hash, "")
	if err != nil {
		t.Fatal(err)
	}
	physicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, session.NextSequence)
	if _, err := owner.MarkRuntimeCandidateRequestSubmitted(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, session.LogicalRequestID, session.NextSequence, physicalCallID); err != nil {
		t.Fatal(err)
	}
	terminal, err := newTestBiz(owner, hub, nil).requestAndSealRuntimeCandidateWithRetry(ctx, prepared, prefix)
	if err != nil {
		t.Fatal(err)
	}
	if terminal.Decision.State != runtimecontract.ConvergenceStateFailed || terminal.Decision.Terminal == nil ||
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
		stored.ProviderSubmitted {
		t.Fatalf("unknown submission session = %+v", stored)
	}
}

func TestRequestAndSealRuntimeCandidateFailsCompleteDocumentContentDefectWithoutProviderOrAttempt(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	state := &inMemoryRuntimeStateRepository{}
	controller, err := runtime.NewRuntimeController(state)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := runtimeTestControllerInitialize(controller, ctx, runtimeAggregate(t, 1).Facts, runtimeDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	initialized = runtimeTestPersistSealedSemanticAuthority(t, ctx, state, initialized)
	contents := memory.NewArtifactContinuationRepository()
	owner := newRuntimeTestOwnerWithEvidence(RuntimeEvidenceRepositories{ArtifactContinuations: contents})
	if err := owner.ConfigureRuntimeRepository(state); err != nil {
		t.Fatal(err)
	}
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(initialized.Definition, initialized.Facts.Activation.ExecutionContract)
	candidateContext := runtimecontract.RuntimeCandidateContextForDecision(initialized.Facts, initialized.Decision, nil)
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(initialized.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(candidateContext.Resources))
	if err != nil {
		t.Fatal(err)
	}
	prepared, session, err := owner.EnsureRuntimeCandidateRequestSession(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, prefix, manifest.Hash, "")
	if err != nil {
		t.Fatal(err)
	}
	physicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, session.NextSequence)
	if _, err := owner.MarkRuntimeCandidateRequestSubmitted(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, session.LogicalRequestID, session.NextSequence, physicalCallID); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.CompleteRuntimeCandidateRequestDocument(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, session.LogicalRequestID, physicalCallID); err != nil {
		t.Fatal(err)
	}
	providerCalls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(_ magicevent.Event, _ magicevent.Result) {
		providerCalls++
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })
	terminal, err := newTestBiz(owner, hub, nil).requestAndSealRuntimeCandidateWithRetry(ctx, prepared, prefix)
	if err != nil {
		t.Fatal(err)
	}
	if providerCalls != 0 || terminal.Decision.State != runtimecontract.ConvergenceStateFailed || terminal.Decision.Terminal == nil ||
		terminal.Decision.Terminal.Code != "runtime_contract_defect" || len(terminal.CandidateAttempts) != 0 ||
		terminal.ActiveDispatch != nil || terminal.Facts.Repair.RepairAttempts != 0 {
		t.Fatalf("complete document content defect must not replay provider or allocate an attempt: %+v", terminal)
	}
}

func TestRequestAndSealRuntimeCandidateReacquiresProviderCeilingAfterTypedStale(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	facts := runtimeAggregate(t, 1).Facts
	ceiling := runtimecontract.ProviderCeiling{
		Selection:                runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
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
	repository := &inMemoryRuntimeStateRepository{}
	controller, err := runtime.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := runtimeTestControllerInitialize(controller, ctx, facts, runtimeDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	initialized = runtimeTestPersistSealedSemanticAuthority(t, ctx, repository, initialized)
	owner := newRuntimeTestOwner()
	if err := owner.ConfigureRuntimeRepository(repository); err != nil {
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
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		calls++
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		result.Set(runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity, ExpectedDocumentHash: runtimeExpectedDocumentHash(command.ExpectedDocument), Stale: &stale}), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })
	invalidated, err := module.requestAndSealRuntimeCandidate(ctx, initialized, "stable-prefix-provider-config", 0)
	if err != nil {
		t.Fatal(err)
	}
	if invalidated.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite ||
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
	released := 0
	for _, allocation := range invalidated.ExecutionBudget.Allocations {
		if allocation.LogicalConsumptionID != terminated.LogicalRequestID || allocation.ScopeRef != terminated.LastPhysicalCallID {
			continue
		}
		released++
		if allocation.State != runtimecontract.RuntimeBudgetAllocationReleased || !strings.HasPrefix(allocation.OwnerFactRef, "llm/provider-request-audit/") {
			t.Fatalf("unsubmitted stale provider allocation was not released from the exact owner fact: %+v", allocation)
		}
	}
	if released == 0 || len(invalidated.ExecutionBudget.Consumptions) != 0 {
		t.Fatalf("unsubmitted stale result consumed provider budget: released=%d consumptions=%+v", released, invalidated.ExecutionBudget.Consumptions)
	}
	restarted, err := repository.Load(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if restarted.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite || len(restarted.CandidateAttempts) != 0 || len(restarted.ExecutionBudget.Consumptions) != 0 ||
		len(restarted.CandidateRequestSessions) != 1 || restarted.CandidateRequestSessions[0].LastOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomePrerequisiteStale {
		t.Fatalf("reload did not preserve stale session and prerequisite reacquisition: %+v", restarted)
	}
	refreshedCeiling := ceiling
	refreshedCeiling.ConfigurationRevision = "config-v2"
	refreshedObservation := observation
	refreshedObservation.ContentHash = "provider-ceiling-v2"
	refreshedObservation.Dependencies[0].Revision = refreshedCeiling.ConfigurationRevision
	refreshedObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(refreshedObservation)
	reacquired, err := owner.CommitRuntimePrerequisite(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, runtime.PrerequisiteResult{
		PrerequisiteID: prerequisite.ID, ProviderCeiling: &refreshedCeiling, Observation: &refreshedObservation,
	})
	if err != nil {
		t.Fatal(err)
	}
	if reacquired.Decision.State != runtimecontract.ConvergenceStateNeedStateChange || len(reacquired.CandidateAttempts) != 0 {
		t.Fatalf("reacquired provider ceiling did not seal a clean authority: %+v", reacquired)
	}
	withNewSession, nextSession, err := owner.EnsureRuntimeCandidateRequestSession(ctx, initialized.Facts.WorkspaceID, initialized.Facts.RunID, "stable-prefix-provider-config", "manifest-provider-config-v2", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(withNewSession.CandidateAttempts) != 0 || nextSession.LogicalRequestID == terminated.LogicalRequestID ||
		nextSession.Transaction.AuthoritySnapshotID == terminated.Transaction.AuthoritySnapshotID ||
		nextSession.State != runtimecontract.RuntimeCandidateRequestSessionPrepared {
		t.Fatalf("reacquired authority did not create a distinct clean request session: previous=%+v next=%+v aggregate=%+v", terminated, nextSession, withNewSession)
	}
}

func startRuntimeFileManifestOwner(t *testing.T, ctx context.Context, hub magicevent.Hub) {
	t.Helper()
	fileOwner := filemodule.New()
	if err := fileOwner.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := fileOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fileOwner.Stop(ctx) })
}

func newRuntimeDriveTestModule(t *testing.T, hub magicevent.Hub) *Biz {
	t.Helper()
	biz := newRuntimeTestOwner()
	if err := biz.ConfigureRuntimeRepository(runtimeStateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	return newTestBiz(biz, hub, nil)
}

func TestRuntimeEnsureCandidateFeasibleBlocksProviderWithoutKinds(t *testing.T) {
	aggregate := runtime.RuntimeAggregate{
		Decision: runtimecontract.RuntimeDecision{
			State: runtimecontract.ConvergenceStateNeedValidation,
			RequiredOutput: runtimecontract.RequiredOutputContract{
				AcceptedKinds:       []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
				CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
			},
		},
		Facts: runtimecontract.RuntimeFacts{
			// file/edit is pure durable — kind-level validation feasibility must exclude it.
			Capabilities: runtimecontract.RuntimeCapabilities{ValidationKinds: []string{runtimecontract.StepKindFileEdit}},
		},
	}
	if err := runtimeEnsureCandidateFeasible(aggregate); err == nil || !strings.Contains(err.Error(), "no feasible operation kinds") {
		t.Fatalf("expected infeasible validation, got %v", err)
	}
	aggregate.Facts.Capabilities.ValidationKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
	aggregate.Facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{{
		PrerequisiteID: "admissibility-shell", OwnerModuleID: runtimecontract.RuntimeModuleShell,
		OperationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, ObservedRevision: "shell-policy-v1",
		ValidationRecipes: []runtimecontract.RuntimeValidationRecipe{{ID: "fixture-validation", Command: "sh", Args: []string{"-c", "true"}}},
	}}
	aggregate.Facts.Resources.OperationAdmissibilities[0].SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(aggregate.Facts.Resources.OperationAdmissibilities[0])
	if err := runtimeEnsureCandidateFeasible(aggregate); err != nil {
		t.Fatalf("managed validation should be feasible: %v", err)
	}
}

func TestRuntimeDecisionPersistsFeasibilityWithoutCallingProvider(t *testing.T) {
	facts := runtimecontract.RuntimeFacts{
		Activation:  runtimecontract.RuntimeActivationContext{ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}}},
		WorkspaceID: "ws-v2", RunID: "run-v2", DefinitionRevision: "definition-v2", EvidenceRevision: runtimecontract.InitialRuntimeEvidenceRevision,
		// The synthetic semantic seal must still have a concrete proof-capable
		// validation operation. The later feasibility gate verifies that the
		// current owner facts do not provide an executable recipe, before any
		// provider request is sent.
		Focus: runtimecontract.CoverageFocus{
			ID: "focus-v2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
			AcceptedStepKinds:     []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
		},
		Coverage:     []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-v2", State: runtimecontract.RuntimeCoverageItemStateOpen}},
		Capabilities: runtimecontract.RuntimeCapabilities{ValidationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}},
	}
	facts.ConvergencePolicy = runtimecontract.DefaultRuntimeConvergencePolicy()
	facts.RecoveryPolicy = runtimecontract.DefaultRuntimeRecoveryPolicy()
	facts.ExecutionBudgetPolicy = runtimecontract.DefaultRuntimeExecutionBudgetPolicy()
	facts.Activation.ExecutionContract.StateChangePolicy = runtimecontract.RuntimeStateChangeAllowedButNotRequired
	decision, err := runtime.ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	repository := &runtimeDriveRepository{aggregate: runtimeTestAggregateWithSemanticPlanning(t, runtime.RuntimeAggregate{StateRevision: 1, Definition: runtimeDefinition(), Facts: facts.Normalized(), Decision: decision})}
	biz := newRuntimeTestOwner()
	if err := biz.ConfigureRuntimeRepository(repository); err != nil {
		t.Fatal(err)
	}
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	providerCalls := 0
	observer := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	observer.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(_ magicevent.Event, _ magicevent.Result) { providerCalls++ })
	t.Cleanup(func() { observer.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })
	module := newTestBiz(biz, hub, nil)
	next, err := module.driveRuntimeDecision(context.Background(), repository.aggregate, "unused-prefix")
	if err != nil {
		t.Fatal(err)
	}
	if providerCalls != 0 || next.Facts.Feasibility == nil ||
		next.Facts.Feasibility.Category != runtimecontract.RuntimeOwnerFeasibilityCategoryValidationRecipeUnavailable ||
		next.Decision.State != runtimecontract.ConvergenceStateFailed || next.Decision.Terminal == nil || next.Decision.Terminal.Code != "validation_recipe_exhausted" {
		t.Fatalf("feasibility transition = %+v providerCalls=%d", next, providerCalls)
	}
}

func TestRuntimeDecisionTerminatesBeforeProviderWhenShellConfirmsNoValidationRecipe(t *testing.T) {
	facts := runtimecontract.RuntimeFacts{
		Activation:  runtimecontract.RuntimeActivationContext{ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}}},
		WorkspaceID: "ws-v2", RunID: "run-v2", DefinitionRevision: "definition-v2", EvidenceRevision: runtimecontract.InitialRuntimeEvidenceRevision,
		Focus: runtimecontract.CoverageFocus{
			ID: "focus-v2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
		},
		Coverage:     []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-v2", State: runtimecontract.RuntimeCoverageItemStateOpen}},
		Capabilities: runtimecontract.RuntimeCapabilities{ValidationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}},
		Resources: runtimecontract.RuntimeResourceFacts{OperationAdmissibilities: []runtimecontract.RuntimeOperationAdmissibility{{
			PrerequisiteID: "admissibility-shell", OwnerModuleID: runtimecontract.RuntimeModuleShell,
			OperationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, ObservedRevision: "shell-policy-v1",
		}}},
	}
	facts.ConvergencePolicy = runtimecontract.DefaultRuntimeConvergencePolicy()
	facts.RecoveryPolicy = runtimecontract.DefaultRuntimeRecoveryPolicy()
	facts.ExecutionBudgetPolicy = runtimecontract.DefaultRuntimeExecutionBudgetPolicy()
	facts.Activation.ExecutionContract.StateChangePolicy = runtimecontract.RuntimeStateChangeAllowedButNotRequired
	facts.Resources.OperationAdmissibilities[0].SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(facts.Resources.OperationAdmissibilities[0])
	decision, err := runtime.ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	repository := &runtimeDriveRepository{aggregate: runtimeTestAggregateWithSemanticPlanning(t, runtime.RuntimeAggregate{StateRevision: 1, Definition: runtimeDefinition(), Facts: facts.Normalized(), Decision: decision})}
	biz := newRuntimeTestOwner()
	if err := biz.ConfigureRuntimeRepository(repository); err != nil {
		t.Fatal(err)
	}
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	providerCalls := 0
	observer := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	observer.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(_ magicevent.Event, _ magicevent.Result) { providerCalls++ })
	t.Cleanup(func() { observer.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })
	module := newTestBiz(biz, hub, nil)
	next, err := module.driveRuntimeDecision(context.Background(), repository.aggregate, "unused-prefix")
	if err != nil {
		t.Fatal(err)
	}
	if providerCalls != 0 || len(next.CandidateAttempts) != 0 || next.ActiveDispatch != nil || len(next.GovernanceRequests) != 0 ||
		next.Facts.Feasibility == nil || next.Facts.Feasibility.Category != runtimecontract.RuntimeOwnerFeasibilityCategoryValidationRecipeUnavailable ||
		next.Decision.State != runtimecontract.ConvergenceStateFailed || next.Decision.Terminal == nil || next.Decision.Terminal.Code != "validation_recipe_exhausted" {
		t.Fatalf("recipe-unavailable pre-provider terminal = %+v providerCalls=%d", next, providerCalls)
	}
}

func TestRuntimeDecisionReconcilesHistoricalAuthorityBeforeProvider(t *testing.T) {
	facts := runtimecontract.RuntimeFacts{
		Activation: runtimecontract.RuntimeActivationContext{
			ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
			ExecutionContract: runtimecontract.RuntimeExecutionContract{
				StateChangePolicy:         runtimecontract.RuntimeStateChangeAllowedButNotRequired,
				CandidateDocumentRecovery: runtimecontract.RuntimeCandidateDocumentRecoveryPolicy{MaxAttempts: 1},
			},
		},
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
	facts.ConvergencePolicy = runtimecontract.DefaultRuntimeConvergencePolicy()
	facts.RecoveryPolicy = runtimecontract.DefaultRuntimeRecoveryPolicy()
	facts.ExecutionBudgetPolicy = runtimecontract.DefaultRuntimeExecutionBudgetPolicy()
	preparedFacts, preparedDecision, err := runtime.ReduceRuntimeStateWithDirective(facts, nil)
	if err != nil || preparedDecision.State != runtimecontract.ConvergenceStateNeedPrerequisite {
		t.Fatalf("prepared authority facts=%+v decision=%+v err=%v", preparedFacts, preparedDecision, err)
	}
	for len(preparedFacts.Prerequisites.Pending) > 0 {
		prerequisite := preparedFacts.Prerequisites.Pending[0]
		result := runtime.PrerequisiteResult{PrerequisiteID: prerequisite.ID}
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
		preparedFacts, err = runtime.CommitPrerequisiteResult(preparedFacts, result)
		if err != nil {
			t.Fatalf("commit current authority prerequisite %s: %v", prerequisite.ID, err)
		}
		preparedFacts, preparedDecision, err = runtime.ReduceRuntimeStateWithDirective(preparedFacts, &preparedDecision.AuthoritySnapshot)
		if err != nil {
			t.Fatalf("reduce current authority prerequisite %s: %v", prerequisite.ID, err)
		}
	}
	if preparedDecision.State != runtimecontract.ConvergenceStateNeedStateChange {
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
	aggregate := runtimeTestAggregateWithSemanticPlanning(t, runtime.RuntimeAggregate{
		StateRevision: 7, Definition: runtimeDefinition(), Facts: preparedFacts, Decision: preparedDecision,
	})
	aggregate.Facts.Prerequisites.Satisfied = append(aggregate.Facts.Prerequisites.Satisfied, historicalPrerequisite)
	aggregate.Facts.Resources.TargetSelection = &historicalSelection
	aggregate.Facts.Resources.TargetSnapshots = append(aggregate.Facts.Resources.TargetSnapshots, runtimecontract.TargetSnapshot{
		TargetRef: "go.mod", ContentHash: "go-mod-old", ContentState: runtimecontract.SnapshotContentStateComplete,
	})
	aggregate.Facts.Resources.PrerequisiteObservations = append(aggregate.Facts.Resources.PrerequisiteObservations, historicalObservation)
	repository := &runtimeDriveRepository{aggregate: aggregate}
	biz := newRuntimeTestOwner()
	if err := biz.ConfigureRuntimeRepository(repository); err != nil {
		t.Fatal(err)
	}
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	providerCalls := 0
	observer := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	observer.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(_ magicevent.Event, _ magicevent.Result) { providerCalls++ })
	t.Cleanup(func() { observer.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })
	module := newTestBiz(biz, hub, nil)

	next, err := module.driveRuntimeDecision(context.Background(), repository.aggregate, "unused-prefix")
	if err != nil {
		t.Fatal(err)
	}
	if providerCalls != 0 || next.StateRevision != 8 || next.Decision.State != runtimecontract.ConvergenceStateNeedStateChange ||
		len(next.CandidateAttempts) != 0 || next.Facts.Repair.RepairAttempts != 0 ||
		next.Facts.Recovery.PrerequisiteRefreshes != 0 {
		t.Fatalf("pre-provider authority gate leaked into candidate/recovery path: %+v providerCalls=%d", next, providerCalls)
	}
	historicalPrerequisiteRetired := false
	for _, retired := range next.Facts.Prerequisites.Retired {
		historicalPrerequisiteRetired = historicalPrerequisiteRetired || retired.ID == historicalPrerequisite.ID
	}
	historicalObservationRetired := false
	for _, retired := range next.Facts.Resources.RetiredPrerequisiteObservations {
		historicalObservationRetired = historicalObservationRetired || retired.ObservationID == historicalObservation.ObservationID
	}
	if next.Facts.Resources.TargetSelection != nil || !historicalPrerequisiteRetired || !historicalObservationRetired {
		t.Fatalf("historical authority was not retired before provider path: %+v", next.Facts)
	}
	if next.Facts.Focus.ID == "" || len(next.Facts.Focus.TargetRefs) != 1 || next.Facts.Focus.TargetRefs[0] != "internal/lsp/manager.go" ||
		len(next.Facts.Focus.CoverageEvidenceBindings) != 1 || len(next.Facts.Prerequisites.Pending) != 0 {
		t.Fatalf("current sealed authority changed while retiring historical facts: %+v", next.Facts)
	}
}

type runtimeDriveRepository struct {
	aggregate runtime.RuntimeAggregate
}

func (r *runtimeDriveRepository) Load(_ context.Context, workspaceID common.WorkspaceID, runID common.RunID) (runtime.RuntimeAggregate, error) {
	if r == nil || r.aggregate.Facts.WorkspaceID != workspaceID || r.aggregate.Facts.RunID != runID {
		return runtime.RuntimeAggregate{}, common.ErrNotFound
	}
	return r.aggregate, nil
}

func (r *runtimeDriveRepository) Save(_ context.Context, aggregate runtime.RuntimeAggregate, expectedRevision uint64) error {
	if r == nil || r.aggregate.StateRevision != expectedRevision {
		return common.ErrConflict
	}
	r.aggregate = aggregate
	return nil
}
