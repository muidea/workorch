package orchestrator

// Owner Biz projection tests.

import (
	"context"
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	resultmodule "github.com/muidea/workorch/internal/modules/blocks/result"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runtimev2 "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtimev2"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	filerepository "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// failAfterResultCreateRepository simulates a process/transport interruption
// after Result durability but before the owner can acknowledge the command.
// The EventHub deliberately recovers handler panics, so returning an error
// gives this file-backed test the same persisted boundary without test logs
// depending on panic recovery internals.
type failAfterResultCreateRepository struct {
	repository.ResultRepository
	fired bool
}

func (r *failAfterResultCreateRepository) Create(ctx context.Context, item *resultmodel.Result) error {
	if err := r.ResultRepository.Create(ctx, item); err != nil {
		return err
	}
	if !r.fired {
		r.fired = true
		return common.ErrConflict
	}
	return nil
}

// failBeforeResultCreateRepository keeps the terminal aggregate durable while
// making the first Result delivery unavailable. A fresh Result owner after a
// process restart must create the Result once and continue Run/Workspace
// projection from that persisted aggregate.
type failBeforeResultCreateRepository struct {
	repository.ResultRepository
	fired bool
}

func (r *failBeforeResultCreateRepository) Create(_ context.Context, _ *resultmodel.Result) error {
	r.fired = true
	return common.ErrConflict
}

func TestDeliverRuntimeV2TerminalCreatesResultAndProjectsOwnersIdempotently(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	resultRepo := memory.NewResultRepository()
	resultOwner := resultmodule.New()
	if err := resultOwner.Configure(resultmodule.Dependencies{
		Results: resultRepo, Artifacts: memory.NewArtifactRepository(), EventHub: hub,
	}); err != nil {
		t.Fatal(err)
	}
	if err := resultOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resultOwner.Stop(ctx) })
	aggregate := runtimeV2Aggregate(t, 2)
	aggregate.Facts.Failure = &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "runtime_contract_defect",
		Signature: "contract-defect", Message: "sealed contract mismatch",
	}
	decision, err := runtimev2.ReduceRuntimeState(aggregate.Facts, &aggregate.Decision.AuthoritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	aggregate.Decision = decision
	runs := memory.NewRunRepository()
	workspaces := memory.NewWorkspaceRepository()
	record := runtimeStateRecordFromAggregate(aggregate)
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", State: common.RunStateActive,
		RuntimeContractVersion: runtimecontract.RuntimeContractVersionV2, RuntimeState: &record,
	}); err != nil {
		t.Fatal(err)
	}
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-v2", State: common.WorkspaceStateActive, ActiveRunID: "run-v2"}); err != nil {
		t.Fatal(err)
	}
	biz := orchestratorbiz.NewRuntimeOrchestrator(runs, workspaces, nil, nil)
	module := newTestBiz(biz, hub, nil)
	for attempt := 0; attempt < 2; attempt++ {
		if err := module.deliverRuntimeV2Terminal(ctx, aggregate); err != nil {
			t.Fatalf("delivery attempt %d: %v", attempt, err)
		}
	}
	item, err := resultRepo.Get(ctx, "ws-v2/run-v2/result/dod-assessment")
	if err != nil || item.State != common.ResultStateRejected {
		t.Fatalf("result = %+v err=%v", item, err)
	}
	run, err := runs.Get(ctx, "run-v2")
	if err != nil || run.State != common.RunStateFailed {
		t.Fatalf("run = %+v err=%v", run, err)
	}
	workspace, err := workspaces.Get(ctx, "ws-v2")
	if err != nil || workspace.ActiveRunID != "" {
		t.Fatalf("workspace = %+v err=%v", workspace, err)
	}
}

// A prerequisite refresh is a recoverable SM-2/SM-3 transition until the
// reducer persists its own refresh-exhausted terminal.  SM-4 delivery must
// reject that aggregate before it can create a rejected Result or project a
// terminal Run/Workspace lifecycle.
func TestDeliverRuntimeV2TerminalRejectsRecoverablePrerequisiteRefresh(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	resultRepo := memory.NewResultRepository()
	resultOwner := resultmodule.New()
	if err := resultOwner.Configure(resultmodule.Dependencies{
		Results: resultRepo, Artifacts: memory.NewArtifactRepository(), EventHub: hub,
	}); err != nil {
		t.Fatal(err)
	}
	if err := resultOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resultOwner.Stop(ctx) })

	aggregate := runtimeV2Aggregate(t, 2)
	// This is the persisted post-invalidation shape: the affected mutable
	// prerequisite is pending, the refresh ledger is below its sealed limit,
	// and only the reducer may later decide whether it becomes terminal.
	aggregate.Facts.Prerequisites.Pending = []runtimecontract.RuntimePrerequisite{{
		ID: "operation_admissibility:file", Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: runtimecontract.RuntimeModuleFile, RequiredCapabilities: []string{runtimecontract.StepKindFileEdit},
	}}
	aggregate.Facts.Recovery.PrerequisiteRefreshes = 1
	aggregate.Facts.RecoveryPolicy.MaxPrerequisiteRefreshes = 2
	decision, err := runtimev2.ReduceRuntimeState(aggregate.Facts, &aggregate.Decision.AuthoritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != runtimecontract.RuntimeActionAcquirePrerequisite || decision.Terminal != nil {
		t.Fatalf("recoverable refresh decision = %+v", decision)
	}
	aggregate.Decision = decision
	if err := aggregate.Validate(); err != nil {
		t.Fatalf("recoverable aggregate must remain valid: %v", err)
	}

	runs := memory.NewRunRepository()
	workspaces := memory.NewWorkspaceRepository()
	record := runtimeStateRecordFromAggregate(aggregate)
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", State: common.RunStateActive,
		RuntimeContractVersion: runtimecontract.RuntimeContractVersionV2, RuntimeState: &record,
	}); err != nil {
		t.Fatal(err)
	}
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-v2", State: common.WorkspaceStateActive, ActiveRunID: "run-v2"}); err != nil {
		t.Fatal(err)
	}

	module := newTestBiz(orchestratorbiz.NewRuntimeOrchestrator(runs, workspaces, nil, nil), hub, nil)
	if err := module.deliverRuntimeV2Terminal(ctx, aggregate); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("recoverable refresh terminal delivery error = %v, want invalid argument", err)
	}
	if _, err := resultRepo.Get(ctx, "ws-v2/run-v2/result/dod-assessment"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("recoverable refresh created Result: %v", err)
	}
	if run, err := runs.Get(ctx, "run-v2"); err != nil || run.State != common.RunStateActive {
		t.Fatalf("recoverable refresh projected Run terminal: %+v err=%v", run, err)
	}
	if workspace, err := workspaces.Get(ctx, "ws-v2"); err != nil || workspace.ActiveRunID != "run-v2" {
		t.Fatalf("recoverable refresh released Workspace: %+v err=%v", workspace, err)
	}
}

func TestDeliverRuntimeV2TerminalRestartsAfterResultPersistenceBeforeOwnerProjection(t *testing.T) {
	ctx := context.Background()
	firstHub := magicevent.NewHub(32)
	root := t.TempDir()
	resultRepo := filerepository.NewResultRepository(root + "/results.json")
	interruptedRepo := &failAfterResultCreateRepository{ResultRepository: resultRepo}
	resultOwner := resultmodule.New()
	if err := resultOwner.Configure(resultmodule.Dependencies{
		Results: interruptedRepo, Artifacts: filerepository.NewArtifactRepository(root + "/artifacts.json"), EventHub: firstHub,
	}); err != nil {
		t.Fatal(err)
	}
	if err := resultOwner.Start(); err != nil {
		t.Fatal(err)
	}

	aggregate := runtimeV2Aggregate(t, 2)
	aggregate.Facts.Failure = &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "runtime_contract_defect",
		Signature: "contract-defect", Message: "sealed contract mismatch",
	}
	decision, err := runtimev2.ReduceRuntimeState(aggregate.Facts, &aggregate.Decision.AuthoritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	aggregate.Decision = decision
	runs := filerepository.NewRunRepository(root + "/runs.json")
	workspaces := filerepository.NewWorkspaceRepository(root + "/workspaces.json")
	record := runtimeStateRecordFromAggregate(aggregate)
	if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", State: common.RunStateActive, RuntimeContractVersion: runtimecontract.RuntimeContractVersionV2, RuntimeState: &record}); err != nil {
		t.Fatal(err)
	}
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-v2", State: common.WorkspaceStateActive, ActiveRunID: "run-v2"}); err != nil {
		t.Fatal(err)
	}
	firstModule := newTestBiz(orchestratorbiz.NewRuntimeOrchestrator(runs, workspaces, nil, nil), firstHub, nil)
	if err := firstModule.deliverRuntimeV2Terminal(ctx, aggregate); err == nil {
		t.Fatal("expected Result-persisted delivery interruption")
	}
	if !interruptedRepo.fired {
		t.Fatal("Result persistence crash point was not reached")
	}
	resultID := common.ResultID("ws-v2/run-v2/result/dod-assessment")
	if item, err := resultRepo.Get(ctx, resultID); err != nil || item.State != common.ResultStateRejected {
		t.Fatalf("persisted Result=%+v err=%v", item, err)
	}
	if run, err := runs.Get(ctx, "run-v2"); err != nil || run.State != common.RunStateActive {
		t.Fatalf("Run changed before restart projection: %+v err=%v", run, err)
	}
	if workspace, err := workspaces.Get(ctx, "ws-v2"); err != nil || workspace.ActiveRunID != "run-v2" {
		t.Fatalf("Workspace changed before restart projection: %+v err=%v", workspace, err)
	}
	if err := resultOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	firstHub.Terminate(ctx)

	// The Result has already reached durable storage. Recreate the complete
	// delivery process and reload the terminal aggregate before asking Result
	// for the idempotent create and Run/Workspace projection.
	restartedHub := magicevent.NewHub(32)
	t.Cleanup(func() { restartedHub.Terminate(ctx) })
	restartedOwner := resultmodule.New()
	if err := restartedOwner.Configure(resultmodule.Dependencies{
		Results: resultRepo, Artifacts: filerepository.NewArtifactRepository(root + "/artifacts.json"), EventHub: restartedHub,
	}); err != nil {
		t.Fatal(err)
	}
	if err := restartedOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedOwner.Stop(ctx) })
	persistedRun, err := runs.Get(ctx, "run-v2")
	if err != nil || persistedRun.RuntimeState == nil {
		t.Fatalf("persisted terminal Run=%+v err=%v", persistedRun, err)
	}
	restartedAggregate := runtimeAggregateFromRecord(*persistedRun.RuntimeState)
	if err := restartedAggregate.Validate(); err != nil {
		t.Fatalf("reloaded terminal aggregate: %v", err)
	}
	restartedModule := newTestBiz(orchestratorbiz.NewRuntimeOrchestrator(runs, workspaces, nil, nil), restartedHub, nil)
	if err := restartedModule.deliverRuntimeV2Terminal(ctx, restartedAggregate); err != nil {
		t.Fatal(err)
	}
	if run, err := runs.Get(ctx, "run-v2"); err != nil || run.State != common.RunStateFailed {
		t.Fatalf("Run restart projection=%+v err=%v", run, err)
	}
	if workspace, err := workspaces.Get(ctx, "ws-v2"); err != nil || workspace.ActiveRunID != "" {
		t.Fatalf("Workspace restart projection=%+v err=%v", workspace, err)
	}
	items, err := resultRepo.ListByRun(ctx, "run-v2")
	if err != nil || len(items) != 1 {
		t.Fatalf("Result restart delivery duplicated item=%+v err=%v", items, err)
	}
}

func TestDeliverRuntimeV2PrerequisiteRefreshExhaustedRestartsAfterTerminalPersistenceBeforeResult(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	runs := filerepository.NewRunRepository(root + "/runs.json")
	workspaces := filerepository.NewWorkspaceRepository(root + "/workspaces.json")
	resultRepo := filerepository.NewResultRepository(root + "/results.json")
	aggregate := runtimeV2Aggregate(t, 2)
	aggregate.Facts.Failure = &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassPrerequisiteRefreshExhausted, Code: "prerequisite_refresh_exhausted",
		Signature: "refresh-budget:receipt-1", Message: "prerequisite refresh budget exhausted after owner resource change",
	}
	decision, err := runtimev2.ReduceRuntimeState(aggregate.Facts, &aggregate.Decision.AuthoritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	aggregate.Decision = decision
	record := runtimeStateRecordFromAggregate(aggregate)
	if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", State: common.RunStateActive, RuntimeContractVersion: runtimecontract.RuntimeContractVersionV2, RuntimeState: &record}); err != nil {
		t.Fatal(err)
	}
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-v2", State: common.WorkspaceStateActive, ActiveRunID: "run-v2"}); err != nil {
		t.Fatal(err)
	}

	firstHub := magicevent.NewHub(32)
	firstOwner := resultmodule.New()
	interruptedRepo := &failBeforeResultCreateRepository{ResultRepository: resultRepo}
	if err := firstOwner.Configure(resultmodule.Dependencies{
		Results: interruptedRepo, Artifacts: filerepository.NewArtifactRepository(root + "/artifacts.json"), EventHub: firstHub,
	}); err != nil {
		t.Fatal(err)
	}
	if err := firstOwner.Start(); err != nil {
		t.Fatal(err)
	}
	firstModule := newTestBiz(orchestratorbiz.NewRuntimeOrchestrator(runs, workspaces, nil, nil), firstHub, nil)
	if err := firstModule.deliverRuntimeV2Terminal(ctx, aggregate); err == nil {
		t.Fatal("expected terminal delivery interruption before Result persistence")
	}
	if !interruptedRepo.fired {
		t.Fatal("Result pre-persistence crash point was not reached")
	}
	if _, err := resultRepo.Get(ctx, "ws-v2/run-v2/result/dod-assessment"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("Result persisted before restart: %v", err)
	}
	if run, err := runs.Get(ctx, "run-v2"); err != nil || run.State != common.RunStateActive {
		t.Fatalf("Run changed before Result persistence: %+v err=%v", run, err)
	}
	if workspace, err := workspaces.Get(ctx, "ws-v2"); err != nil || workspace.ActiveRunID != "run-v2" {
		t.Fatalf("Workspace changed before Result persistence: %+v err=%v", workspace, err)
	}
	if err := firstOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	firstHub.Terminate(ctx)

	// Recreate the EventHub, owner and delivery Biz, then reload the terminal
	// aggregate from its file-backed Run record rather than retaining memory.
	restartedHub := magicevent.NewHub(32)
	t.Cleanup(func() { restartedHub.Terminate(ctx) })
	restartedOwner := resultmodule.New()
	if err := restartedOwner.Configure(resultmodule.Dependencies{
		Results: resultRepo, Artifacts: filerepository.NewArtifactRepository(root + "/artifacts.json"), EventHub: restartedHub,
	}); err != nil {
		t.Fatal(err)
	}
	if err := restartedOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedOwner.Stop(ctx) })
	persistedRun, err := runs.Get(ctx, "run-v2")
	if err != nil || persistedRun.RuntimeState == nil {
		t.Fatalf("persisted terminal Run=%+v err=%v", persistedRun, err)
	}
	restartedAggregate := runtimeAggregateFromRecord(*persistedRun.RuntimeState)
	if err := restartedAggregate.Validate(); err != nil {
		t.Fatalf("reloaded terminal aggregate: %v", err)
	}
	restartedModule := newTestBiz(orchestratorbiz.NewRuntimeOrchestrator(runs, workspaces, nil, nil), restartedHub, nil)
	if err := restartedModule.deliverRuntimeV2Terminal(ctx, restartedAggregate); err != nil {
		t.Fatal(err)
	}
	items, err := resultRepo.ListByRun(ctx, "run-v2")
	if err != nil || len(items) != 1 || items[0].State != common.ResultStateRejected || items[0].RuntimeCloseout == nil ||
		items[0].RuntimeCloseout.FailureClass != runtimecontract.FailureClassPrerequisiteRefreshExhausted ||
		items[0].RuntimeCloseout.FailureCode != "prerequisite_refresh_exhausted" {
		t.Fatalf("Result after restart=%+v err=%v", items, err)
	}
	if run, err := runs.Get(ctx, "run-v2"); err != nil || run.State != common.RunStateFailed {
		t.Fatalf("Run after restart projection=%+v err=%v", run, err)
	}
	if workspace, err := workspaces.Get(ctx, "ws-v2"); err != nil || workspace.ActiveRunID != "" {
		t.Fatalf("Workspace after restart projection=%+v err=%v", workspace, err)
	}
}

func TestRuntimeV2ResultProjectionBuildsAcceptedDoDAssessment(t *testing.T) {
	aggregate := runtimev2.RuntimeAggregate{
		Definition: runtimecontract.RuntimeDefinitionContext{GoalID: "goal-1", DoDID: "dod-1"},
		Facts: runtimecontract.RuntimeFacts{
			Activation:         runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
			WorkspaceID:        "ws-1",
			RunID:              "run-1",
			DefinitionRevision: "definition-1",
			Focus:              runtimecontract.CoverageFocus{ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}},
			Coverage:           []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-1", State: runtimecontract.RuntimeCoverageItemStateSatisfied, EvidenceRefs: []string{"event-1"}}},
			Assessments: []runtimecontract.RuntimeAssessmentFact{{
				ID: "assessment-1", FocusID: "focus-1", Scope: "dodItem", RequirementID: "dod-001", RequirementText: "行为已验证",
				CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
				Disposition:         runtimecontract.AssessmentDispositionSatisfied, EvidenceRefs: []string{"event-1"}, ArtifactRefs: []string{"artifact-assessment"},
				Items: []runtimecontract.RuntimeAssessmentItemFact{{CoverageWorkItemID: "cwi-1", Satisfied: true, EvidenceRefs: []string{"event-1"}, ArtifactRefs: []common.ArtifactID{"artifact-item"}}},
			}}},
		Decision: runtimecontract.RuntimeDecision{
			Action:            runtimecontract.RuntimeActionComplete,
			AuthoritySnapshot: runtimecontract.AuthoritySnapshot{ID: "authority-1"},
			Terminal:          &runtimecontract.TerminalOutcome{Code: "coverage_satisfied", Summary: "coverage satisfied"},
		},
	}
	item := runtimeV2ResultFromAggregate(aggregate)
	if item.ID != "ws-1/run-1/result/dod-assessment" || item.State != common.ResultStateAccepted || !item.DoDAssessment.Satisfied ||
		len(item.DoDAssessment.Items) != 1 || item.DoDAssessment.Items[0].Item != "行为已验证" || len(item.EvidenceRefs) != 1 {
		t.Fatalf("runtime v2 result = %+v", item)
	}
	if item.RuntimeCloseout == nil || item.RuntimeCloseout.Disposition != runtimecontract.AssessmentDispositionSatisfied ||
		len(item.RuntimeCloseout.SatisfiedCoverageWorkItemIDs) != 1 || item.RuntimeCloseout.SatisfiedCoverageWorkItemIDs[0] != "cwi-1" {
		t.Fatalf("runtime closeout = %+v", item.RuntimeCloseout)
	}
	if item.RuntimeCloseout.SchemaVersion != runtimecontract.RuntimeResultCloseoutSchemaVersionCurrent {
		t.Fatalf("runtime closeout schema = %d", item.RuntimeCloseout.SchemaVersion)
	}
	if len(item.ArtifactRefs) != 2 || item.ArtifactRefs[0] != "artifact-assessment" || item.ArtifactRefs[1] != "artifact-item" ||
		len(item.SummaryDetail.ArtifactRefs) != 2 || len(item.RuntimeCloseout.ArtifactRefs) != 2 {
		t.Fatalf("runtime artifact refs = result:%v summary:%v closeout:%v", item.ArtifactRefs, item.SummaryDetail.ArtifactRefs, item.RuntimeCloseout.ArtifactRefs)
	}
}

func TestRuntimeV2ResultProjectionBuildsStructuredRejectedCloseout(t *testing.T) {
	aggregate := runtimev2.RuntimeAggregate{
		Definition: runtimecontract.RuntimeDefinitionContext{GoalID: "goal-1", DoDID: "dod-1", Revision: "definition-1"},
		Facts: runtimecontract.RuntimeFacts{
			Activation:         runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
			WorkspaceID:        "ws-1",
			RunID:              "run-1",
			DefinitionRevision: "definition-1",
			Focus: runtimecontract.CoverageFocus{
				ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1", "cwi-2"},
			},
			Coverage: []runtimecontract.RuntimeCoverageItemFact{
				{ID: "cwi-1", State: runtimecontract.RuntimeCoverageItemStateSatisfied, EvidenceRefs: []string{"ev-1"}},
				{ID: "cwi-2", State: runtimecontract.RuntimeCoverageItemStateUnsatisfied},
			},
			Assessments: []runtimecontract.RuntimeAssessmentFact{{
				ID: "assessment-1", FocusID: "focus-1", Scope: "dodItem", RequirementID: "dod-001", RequirementText: "行为一",
				CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1", "cwi-2"},
				Disposition:         runtimecontract.AssessmentDispositionRepairableGap,
				Summary:             "implementation incomplete",
				Items: []runtimecontract.RuntimeAssessmentItemFact{
					{CoverageWorkItemID: "cwi-1", Satisfied: true, EvidenceRefs: []string{"ev-1"}},
					{CoverageWorkItemID: "cwi-2", Satisfied: false, Reason: "missing implementation"},
				},
				Gap: &runtimecontract.RuntimeAssessmentGap{
					CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-2"},
					MissingEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
					TargetRefs:           []string{"main.go"},
					Reason:               "tools not implemented",
					FailureCode:          "implementation_gap",
					FailureSignature:     "sig-impl",
				},
				EvidenceRefs: []string{"ev-1"},
			}},
			Repair: runtimecontract.RuntimeRepairLedger{LastCycleID: "cycle-1"},
		},
		Decision: runtimecontract.RuntimeDecision{
			Action:            runtimecontract.RuntimeActionFail,
			AuthoritySnapshot: runtimecontract.AuthoritySnapshot{ID: "authority-1"},
			Terminal:          &runtimecontract.TerminalOutcome{Code: "no_progress_exhausted", Summary: "repair budget exhausted"},
		},
	}
	item := runtimeV2ResultFromAggregate(aggregate)
	if item.State != common.ResultStateRejected || item.RuntimeCloseout == nil {
		t.Fatalf("rejected result = %+v", item)
	}
	closeout := item.RuntimeCloseout
	if closeout.DefinitionRevision != "definition-1" || closeout.FailureCode != "implementation_gap" {
		t.Fatalf("closeout identity = %+v", closeout)
	}
	if len(closeout.SatisfiedCoverageWorkItemIDs) != 1 || closeout.SatisfiedCoverageWorkItemIDs[0] != "cwi-1" {
		t.Fatalf("satisfied = %v", closeout.SatisfiedCoverageWorkItemIDs)
	}
	if len(closeout.UnsatisfiedCoverageWorkItemIDs) != 1 || closeout.UnsatisfiedCoverageWorkItemIDs[0] != "cwi-2" {
		t.Fatalf("unsatisfied = %v", closeout.UnsatisfiedCoverageWorkItemIDs)
	}
	if len(closeout.Gaps) != 1 || len(closeout.MissingEvidenceKinds) != 1 || closeout.MissingEvidenceKinds[0] != runtimecontract.EvidenceKindStateChange {
		t.Fatalf("gaps/missing = %+v %v", closeout.Gaps, closeout.MissingEvidenceKinds)
	}
	if len(closeout.TargetRefs) != 1 || closeout.TargetRefs[0] != "main.go" {
		t.Fatalf("target refs = %v", closeout.TargetRefs)
	}
	if closeout.Disposition != runtimecontract.AssessmentDispositionRepairableGap {
		t.Fatalf("disposition = %q", closeout.Disposition)
	}
}

func TestRuntimeV2ResultProjectionPackagesLedgerEvidenceBeforeAssessment(t *testing.T) {
	aggregate := runtimev2.RuntimeAggregate{
		Definition: runtimecontract.RuntimeDefinitionContext{GoalID: "goal-1", DoDID: "dod-1", Revision: "definition-1"},
		Facts: runtimecontract.RuntimeFacts{
			Activation:  runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
			WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", EvidenceRevision: "evidence-v2-00000000000000000002",
			Focus:    runtimecontract.CoverageFocus{ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}},
			Coverage: []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-1", State: runtimecontract.RuntimeCoverageItemStateUnsatisfied}},
			Evidence: runtimecontract.EvidenceLedger{
				Accepted: []runtimecontract.EvidenceFact{{ID: "accepted-1", SourceRef: "workorch://accepted", EvidenceRefs: []string{"accepted-detail"}, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}}},
				Rejected: []runtimecontract.EvidenceFact{{ID: "rejected-1", SourceRef: "workorch://rejected", EvidenceRefs: []string{"rejected-detail"}, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}}},
			},
			Failure: &runtimecontract.FailureFact{Class: runtimecontract.FailureClassCandidateExhausted, Code: "candidate_retry_exhausted", Signature: "candidate-retry-exhausted", Message: "candidate exhausted"},
		},
		Decision: runtimecontract.RuntimeDecision{Action: runtimecontract.RuntimeActionFail, AuthoritySnapshot: runtimecontract.AuthoritySnapshot{ID: "authority-1"}, Terminal: &runtimecontract.TerminalOutcome{Code: "candidate_retry_exhausted", Summary: "candidate exhausted"}},
	}
	item := runtimeV2ResultFromAggregate(aggregate)
	for _, ref := range []string{"accepted-1", "workorch://accepted", "accepted-detail", "rejected-1", "workorch://rejected", "rejected-detail"} {
		if !containsResultEvidenceRef(item.EvidenceRefs, ref) {
			t.Fatalf("result evidence refs = %v, missing %q", item.EvidenceRefs, ref)
		}
	}
	if item.RuntimeCloseout == nil || item.RuntimeCloseout.FailureClass != runtimecontract.FailureClassCandidateExhausted || len(item.RuntimeCloseout.EvidenceRefs) != 6 {
		t.Fatalf("runtime closeout = %+v", item.RuntimeCloseout)
	}
}

func containsResultEvidenceRef(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
