package run

import (
	"context"
	"errors"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRunSM1FactsRejectForgedSourcesWithoutStateTransition(t *testing.T) {
	tests := []struct {
		name       string
		topic      string
		seed       *runmodel.Run
		data       any
		wouldApply func(*runmodel.Run, any) error
	}{
		{
			name: "governance wait", topic: runevents.TopicRunFactGovernanceWait,
			seed: runLifecycleSourceTestGovernanceRun(common.RunStateActive, ""),
			data: runevents.RuntimeGovernanceWaitFact{
				WorkspaceID: "workspace-source-wait", RunID: "run-source-wait", SourceOwner: runtimeLifecycleFactSource,
				CausationID: "cause-source-wait", AuthoritySnapshotID: "authority-source-wait",
				DefinitionRevision: "definition-source-wait", EvidenceRevision: "evidence-source-wait",
				DispatchID: "dispatch-source-wait", BlockingTicketID: "ticket-source-wait",
			},
			wouldApply: func(current *runmodel.Run, data any) error {
				transition, err := reduceGovernanceWait(current, data.(runevents.RuntimeGovernanceWaitFact))
				if err != nil || transition.Noop || transition.Target != common.RunStateBlockedOnDecision {
					return errors.New("production-valid governance wait would not transition")
				}
				return nil
			},
		},
		{
			name: "continuation", topic: runevents.TopicRunFactContinuationEligible,
			seed: runLifecycleSourceTestGovernanceRun(common.RunStateBlockedOnDecision, "resolution-source-continuation"),
			data: runevents.RuntimeContinuationEligibleFact{
				WorkspaceID: "workspace-source-continuation", RunID: "run-source-continuation", SourceOwner: runtimeLifecycleFactSource,
				CausationID: "cause-source-continuation", AuthoritySnapshotID: "authority-source-continuation",
				DefinitionRevision: "definition-source-continuation", EvidenceRevision: "evidence-source-continuation",
				DispatchID: "dispatch-source-continuation", ResolutionID: "resolution-source-continuation", TicketID: "ticket-source-continuation",
			},
			wouldApply: func(current *runmodel.Run, data any) error {
				transition, err := reduceContinuationEligible(current, data.(runevents.RuntimeContinuationEligibleFact))
				if err != nil || transition.Noop || transition.Target != common.RunStateActive {
					return errors.New("production-valid continuation would not transition")
				}
				return nil
			},
		},
		{
			name: "terminal delivery", topic: runevents.TopicRunFactTerminalDelivery,
			seed: runLifecycleSourceTestTerminalRun(),
			data: terminalDeliveryTestCommand(runtimecontract.RuntimeTerminalDeliveryIdentity{
				WorkspaceID: "workspace-source-terminal", RunID: "run-source-terminal", TerminalDeliveryID: "terminal-source-terminal",
				AuthoritySnapshotID: "authority-source-terminal", DefinitionRevision: "definition-source-terminal",
				EvidenceRevision: "evidence-source-terminal", DispatchID: "dispatch-source-terminal",
				Outcome: runtimecontract.TerminalOutcome{Kind: runtimecontract.TerminalOutcomeKindCompleted, Code: "completed", Summary: "done"},
			}, "cause-source-terminal"),
			wouldApply: func(current *runmodel.Run, data any) error {
				transition, err := reduceTerminalDelivery(current, data.(runevents.RuntimeTerminalDeliveryFact))
				if err != nil || transition.Noop || transition.Target != common.RunStateCompleted {
					return errors.New("production-valid terminal delivery would not transition")
				}
				return nil
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), test.seed.WorkspaceID)
			if err := test.wouldApply(runmodel.CloneRun(test.seed), test.data); err != nil {
				t.Fatal(err)
			}
			hub := magicevent.NewHub(16)
			ownerSet := memoryrepo.NewRunOwnerRepositorySet()
			runs := ownerSet.Runs()
			if err := runs.Create(ctx, runmodel.CloneRun(test.seed)); err != nil {
				t.Fatal(err)
			}
			owner := New()
			if err := owner.Configure(Dependencies{OwnerRepositories: ownerSet, EventHub: hub}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				_ = owner.Stop(context.Background())
				hub.Terminate(context.Background())
			})
			before, err := runs.GetByWorkspaceID(ctx, test.seed.WorkspaceID, test.seed.ID)
			if err != nil {
				t.Fatal(err)
			}
			event := magicevent.NewEventWithContext(test.topic, "blocks/workspace", runevents.CommandDestination, magicevent.NewHeader(), ctx, test.data)
			if _, err := owner.executeLifecycleCommand(event); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("forged %s source error = %v, want invalid argument", test.topic, err)
			}
			after, readErr := runs.GetByWorkspaceID(ctx, test.seed.WorkspaceID, test.seed.ID)
			if readErr != nil || !reflect.DeepEqual(after, before) {
				t.Fatalf("forged %s source mutated Run: before=%+v after=%+v err=%v", test.topic, before, after, readErr)
			}
		})
	}
}

func TestRunWorkspaceOwnedIngressRejectsForgedSourcesWithoutRetirementOrSuccessor(t *testing.T) {
	t.Run("prepare retirement", func(t *testing.T) {
		ctx, owner, ownerSet := newRunPrivilegedSourceTestOwner(t, "workspace-source-prepare")
		command := runevents.PrepareWorkspaceRetirementCommand{
			WorkspaceID: "workspace-source-prepare", DeletionID: "deletion-source-prepare", DeletionGeneration: 1,
			DataDispositionIntent: deletioncontract.ModeRetain, WorkspaceIntentHash: "workspace-intent-source-prepare",
		}
		if err := command.Validate(); err != nil {
			t.Fatalf("production-valid prepare command: %v", err)
		}
		event := magicevent.NewEventWithContext(runevents.TopicRunCommandPrepareWorkspaceRetirement, "application/workorchd", runevents.CommandDestination, magicevent.NewHeader(), ctx, command)
		if _, err := owner.executeWorkspaceRetirementCommand(ctx, event); !errors.Is(err, common.ErrInvalidArgument) {
			t.Fatalf("forged prepare source error = %v, want invalid argument", err)
		}
		if _, err := ownerSet.Runs().GetWorkspaceRetirement(ctx, command.WorkspaceID); !errors.Is(err, common.ErrNotFound) {
			t.Fatalf("forged prepare persisted fence: %v", err)
		}
	})

	t.Run("finalize retirement", func(t *testing.T) {
		ctx, owner, ownerSet := newRunPrivilegedSourceTestOwner(t, "workspace-source-finalize")
		fence, err := ownerSet.Runs().PrepareWorkspaceRetirement(ctx, runmodel.WorkspaceRetirementIntent{
			WorkspaceID: "workspace-source-finalize", DeletionID: "deletion-source-finalize", DeletionGeneration: 1,
			DataDispositionIntent: deletioncontract.ModeRetain, WorkspaceIntentHash: "workspace-intent-source-finalize",
		})
		if err != nil {
			t.Fatal(err)
		}
		token := runSourceTestQuiescenceToken(*fence, nil)
		command := runevents.FinalizeWorkspaceRetirementCommand{QuiescenceToken: token}
		if err := command.Validate(); err != nil {
			t.Fatalf("production-valid finalize command: %v", err)
		}
		event := magicevent.NewEventWithContext(runevents.TopicRunCommandFinalizeWorkspaceRetirement, "application/workorchd", runevents.CommandDestination, magicevent.NewHeader(), ctx, command)
		if _, err := owner.executeWorkspaceRetirementCommand(ctx, event); !errors.Is(err, common.ErrInvalidArgument) {
			t.Fatalf("forged finalize source error = %v, want invalid argument", err)
		}
		stored, err := ownerSet.Runs().GetWorkspaceRetirement(ctx, fence.WorkspaceID)
		if err != nil || !stored.IsDraining() || !runmodel.SameWorkspaceRetirementFence(*stored, *fence) {
			t.Fatalf("forged finalize mutated fence = %+v err=%v", stored, err)
		}
	})

	t.Run("purge retirement", func(t *testing.T) {
		workspaceID := common.WorkspaceID("workspace-source-purge")
		ctx, owner, ownerSet := newRunPrivilegedSourceTestOwner(t, workspaceID)
		run := &runmodel.Run{WorkspaceID: workspaceID, State: common.RunStateCompleted}
		if err := ownerSet.Runs().CreateNextWorkspaceRun(ctx, run); err != nil {
			t.Fatal(err)
		}
		if err := ownerSet.Steps().Create(ctx, &runmodel.Step{ID: common.StepID(string(run.ID) + "/step-source"), RunID: run.ID, State: common.StepStateSucceeded}); err != nil {
			t.Fatal(err)
		}
		if err := ownerSet.Checkpoints().Create(ctx, &runmodel.RuntimeCheckpoint{ID: "checkpoint-source-purge", WorkspaceID: workspaceID, RunID: run.ID, RunStateSnapshot: common.RunStateCompleted}); err != nil {
			t.Fatal(err)
		}
		fence, err := ownerSet.Runs().PrepareWorkspaceRetirement(ctx, runmodel.WorkspaceRetirementIntent{
			WorkspaceID: workspaceID, DeletionID: "deletion-source-purge", DeletionGeneration: 1,
			DataDispositionIntent: deletioncontract.ModePurge, WorkspaceIntentHash: "workspace-intent-source-purge",
		})
		if err != nil {
			t.Fatal(err)
		}
		current, err := ownerSet.Runs().GetByWorkspaceID(ctx, workspaceID, run.ID)
		if err != nil {
			t.Fatal(err)
		}
		token := runSourceTestQuiescenceToken(*fence, []deletioncontract.WorkspaceRuntimeQuiescenceRun{{
			RunID: current.ID, WorkspaceRunGeneration: current.WorkspaceRunGeneration, RunRevision: current.Revision, RunState: current.State,
		}})
		retired, err := ownerSet.Runs().FinalizeWorkspaceRetirement(ctx, token)
		if err != nil {
			t.Fatal(err)
		}
		command := runevents.PurgeWorkspaceRunsCommand{
			WorkspaceID: retired.WorkspaceID, DeletionID: retired.DeletionID, DeletionGeneration: retired.DeletionGeneration,
			DataDispositionIntent: retired.DataDispositionIntent, WorkspaceIntentHash: retired.WorkspaceIntentHash,
			FenceRevision: retired.FenceRevision, WorkspaceRunGeneration: retired.WorkspaceRunGeneration,
			RuntimeInventoryHash: retired.RuntimeInventoryHash, QuiescenceID: retired.QuiescenceID,
		}
		if err := command.Validate(); err != nil {
			t.Fatalf("production-valid purge command: %v", err)
		}
		event := magicevent.NewEventWithContext(runevents.TopicRunCommandPurgeWorkspace, "application/workorchd", runevents.CommandDestination, magicevent.NewHeader(), ctx, command)
		if _, err := owner.executeWorkspaceRetirementCommand(ctx, event); !errors.Is(err, common.ErrInvalidArgument) {
			t.Fatalf("forged purge source error = %v, want invalid argument", err)
		}
		if _, err := ownerSet.Runs().GetByWorkspaceID(ctx, workspaceID, run.ID); err != nil {
			t.Fatalf("forged purge removed seeded Run: %v", err)
		}
	})

	t.Run("retirement fact query", func(t *testing.T) {
		ctx, owner, ownerSet := newRunPrivilegedSourceTestOwner(t, "workspace-source-query")
		fence, err := ownerSet.Runs().PrepareWorkspaceRetirement(ctx, runmodel.WorkspaceRetirementIntent{
			WorkspaceID: "workspace-source-query", DeletionID: "deletion-source-query", DeletionGeneration: 1,
			DataDispositionIntent: deletioncontract.ModeRetain, WorkspaceIntentHash: "workspace-intent-source-query",
		})
		if err != nil {
			t.Fatal(err)
		}
		command := runevents.QueryWorkspaceRetirementFactCommand{WorkspaceID: fence.WorkspaceID, DeletionID: fence.DeletionID}
		if err := command.Validate(); err != nil {
			t.Fatalf("production-valid query command: %v", err)
		}
		query := magicevent.NewEventWithContext(runevents.TopicRunCommandWorkspaceRetirementFact, "blocks/workspace", runevents.CommandDestination, magicevent.NewHeader(), ctx, command)
		if _, err := owner.executeWorkspaceRetirementCommand(ctx, query); !errors.Is(err, common.ErrInvalidArgument) {
			t.Fatalf("forged retirement fact query source error = %v, want invalid argument", err)
		}
	})

	t.Run("workspace execution intent", func(t *testing.T) {
		workspaceID := common.WorkspaceID("workspace-source-execution")
		ctx, owner, ownerSet := newRunPrivilegedSourceTestOwner(t, workspaceID)
		bootstrap := restartTestBootstrap(t, workspaceID)
		seed := &runmodel.Run{
			ID: "run-source-execution", WorkspaceID: workspaceID, GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
			DefinitionRevision: bootstrap.Definition.Revision, StartupInputHash: bootstrap.Definition.StartupInputHash,
			RuntimeBootstrap: &bootstrap, State: common.RunStatePending, Lineage: runmodel.RunLineage{LineageState: runmodel.RunLineageStateActive},
			CreatedAt: "2026-08-30T00:00:00Z",
		}
		if err := ownerSet.Runs().Create(ctx, seed); err != nil {
			t.Fatal(err)
		}
		command := runevents.WorkspaceExecutionIntentCommand{
			WorkspaceID: workspaceID, RunID: seed.ID, RequestID: "resume-source-execution", Kind: runtimecontract.RuntimeExecutionIntentResume,
		}
		if err := command.Validate(); err != nil {
			t.Fatalf("production-valid execution command: %v", err)
		}
		execution := magicevent.NewEventWithContext(runevents.TopicRunCommandWorkspaceExecutionIntent, "application/workorchd", runevents.CommandDestination, magicevent.NewHeader(), ctx, command)
		if _, err := owner.executeWorkspaceExecutionIntent(execution); !errors.Is(err, common.ErrInvalidArgument) {
			t.Fatalf("forged execution source error = %v, want invalid argument", err)
		}
		stored, err := ownerSet.Runs().GetByWorkspaceID(ctx, workspaceID, seed.ID)
		if err != nil || stored.State != common.RunStatePending {
			t.Fatalf("forged execution source mutated Run = %+v err=%v", stored, err)
		}
	})
}

func runLifecycleSourceTestGovernanceRun(state common.RunState, resolutionID string) *runmodel.Run {
	suffix := "wait"
	blockingTicketID := common.TicketID("")
	if state == common.RunStateBlockedOnDecision {
		suffix = "continuation"
		blockingTicketID = "ticket-source-continuation"
	}
	return &runmodel.Run{
		ID: common.RunID("run-source-" + suffix), WorkspaceID: common.WorkspaceID("workspace-source-" + suffix),
		GoalID: "goal-source-" + common.GoalID(suffix), DoDID: "dod-source-" + common.DoDID(suffix),
		DefinitionRevision: "definition-source-" + suffix, Lineage: runmodel.RunLineage{LineageState: runmodel.RunLineageStateActive},
		State: state, BlockingTicketID: blockingTicketID, CreatedAt: "2026-08-30T00:00:00Z",
		RuntimeBootstrap: &runtimecontract.RuntimeBootstrap{}, RuntimeState: &runtimecontract.RuntimeStateRecord{StateRevision: 3},
		RuntimeLifecycleFence: &runtimecontract.RuntimeOpaqueLifecycleFence{
			WorkspaceID: common.WorkspaceID("workspace-source-" + suffix), RunID: common.RunID("run-source-" + suffix), RuntimeStateRevision: 3,
			GovernanceFenceID:      "runtime-governance/authority-source-" + suffix + "/dispatch-source-" + suffix,
			GovernanceResolutionID: common.ResolutionID(resolutionID),
		},
	}
}

func runLifecycleSourceTestTerminalRun() *runmodel.Run {
	state := runtimecontract.RuntimeStateRecord{
		StateRevision: 3,
		Facts:         runtimecontract.RuntimeFacts{WorkspaceID: "workspace-source-terminal", RunID: "run-source-terminal", DefinitionRevision: "definition-source-terminal", EvidenceRevision: "evidence-source-terminal"},
		Decision: runtimecontract.RuntimeDecision{
			State:             runtimecontract.ConvergenceStateComplete,
			Terminal:          &runtimecontract.RuntimeTerminalDecision{Code: "completed", Summary: "done"},
			AuthoritySnapshot: runtimecontract.AuthoritySnapshot{ID: "authority-source-terminal"},
		},
	}
	return &runmodel.Run{
		ID: "run-source-terminal", WorkspaceID: "workspace-source-terminal", GoalID: "goal-source-terminal", DoDID: "dod-source-terminal",
		DefinitionRevision: "definition-source-terminal", Lineage: runmodel.RunLineage{LineageState: runmodel.RunLineageStateActive},
		State: common.RunStateActive, CreatedAt: "2026-08-30T00:00:00Z", RuntimeBootstrap: &runtimecontract.RuntimeBootstrap{}, RuntimeState: &state,
		RuntimeLifecycleFence: &runtimecontract.RuntimeOpaqueLifecycleFence{
			WorkspaceID: "workspace-source-terminal", RunID: "run-source-terminal", RuntimeStateRevision: state.StateRevision,
			RuntimeStateRef:  runevents.RuntimeStateRef("workspace-source-terminal", "run-source-terminal", state.StateRevision),
			RuntimeStateHash: "runtime-state-source-terminal", BootstrapHash: "bootstrap-source-terminal", TerminalFenceID: "terminal-source-terminal",
		},
	}
}

func newRunPrivilegedSourceTestOwner(t *testing.T, workspaceID common.WorkspaceID) (context.Context, *Biz, repository.RunOwnerRepositories) {
	t.Helper()
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID)
	hub := magicevent.NewHub(16)
	ownerSet := memoryrepo.NewRunOwnerRepositorySet()
	owner := New()
	if err := owner.Configure(Dependencies{OwnerRepositories: ownerSet, EventHub: hub}); err != nil {
		hub.Terminate(context.Background())
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = owner.Stop(context.Background())
		hub.Terminate(context.Background())
	})
	return ctx, owner, ownerSet
}

func runSourceTestQuiescenceToken(fence runmodel.WorkspaceRetirementFence, runs []deletioncontract.WorkspaceRuntimeQuiescenceRun) deletioncontract.WorkspaceRuntimeQuiescenceToken {
	token := deletioncontract.WorkspaceRuntimeQuiescenceToken{
		WorkspaceID: fence.WorkspaceID, DeletionID: fence.DeletionID, DeletionGeneration: fence.DeletionGeneration,
		DataDispositionIntent: fence.DataDispositionIntent, WorkspaceIntentHash: fence.WorkspaceIntentHash,
		RunDrainingRevision: fence.FenceRevision, WorkspaceRunGeneration: fence.WorkspaceRunGeneration,
		OrchestratorQuiescenceRevision: 2, Runs: append([]deletioncontract.WorkspaceRuntimeQuiescenceRun(nil), runs...),
	}
	token.RuntimeInventoryHash = deletioncontract.StableWorkspaceRuntimeInventoryHash(token)
	token.QuiescenceID = deletioncontract.StableWorkspaceRuntimeQuiescenceID(token)
	return token
}
