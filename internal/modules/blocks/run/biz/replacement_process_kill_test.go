package run

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	filerepo "github.com/muidea/workorch/internal/pkg/repository/file"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const (
	runReplacementCrashHelperEnv = "WORKORCH_TEST_RUN_REPLACEMENT_CRASH_HELPER"
	runReplacementCrashRootEnv   = "WORKORCH_TEST_RUN_REPLACEMENT_CRASH_ROOT"
	runReplacementCrashCutEnv    = "WORKORCH_TEST_RUN_REPLACEMENT_CRASH_CUT"
	runReplacementCrashMarkerEnv = "WORKORCH_TEST_RUN_REPLACEMENT_CRASH_MARKER"
)

type runReplacementCrashRepository struct {
	repository.RunRepository
	cut    string
	marker string
}

func (r *runReplacementCrashRepository) ReadWorkspaceExecutionIntentReceipt(ctx context.Context, workspaceID common.WorkspaceID, requestID string) (*runmodel.WorkspaceExecutionIntentReceipt, error) {
	return r.RunRepository.(repository.RunExecutionIntentReceiptReader).ReadWorkspaceExecutionIntentReceipt(ctx, workspaceID, requestID)
}

func (r *runReplacementCrashRepository) CreateNextWorkspaceRun(ctx context.Context, item *runmodel.Run) error {
	err := r.RunRepository.CreateNextWorkspaceRun(ctx, item)
	if err == nil && r.cut == "successor_created" && item.Lineage.PredecessorRunID != "" {
		runReplacementCrashAfterDurableWrite(r.marker)
	}
	return err
}

func (r *runReplacementCrashRepository) CompareAndSwap(ctx context.Context, item *runmodel.Run, expectedRevision uint64) error {
	err := r.RunRepository.CompareAndSwap(ctx, item, expectedRevision)
	if err == nil && runReplacementCrashMatches(r.cut, item) {
		runReplacementCrashAfterDurableWrite(r.marker)
	}
	return err
}

func runReplacementCrashMatches(cut string, item *runmodel.Run) bool {
	if item == nil {
		return false
	}
	if cut == "successor_activated" {
		return item.State == common.RunStateActive && item.Lineage.PredecessorRunID != ""
	}
	intent := item.ReplacementIntent
	if intent == nil {
		return false
	}
	switch cut {
	case "replacement_intent_sealed":
		return intent.State == runmodel.ReplacementIntentPending && intent.Recovery.Stage == runmodel.ReplacementRecoveryAdministrativeTermination && intent.Recovery.State == runmodel.ReplacementRecoveryScheduled
	case "administrative_recovery_claimed":
		return intent.State == runmodel.ReplacementIntentPending && intent.Recovery.Stage == runmodel.ReplacementRecoveryAdministrativeTermination && intent.Recovery.State == runmodel.ReplacementRecoveryRunning && item.State == common.RunStateActive
	case "predecessor_terminal_sealed":
		return intent.State == runmodel.ReplacementIntentPending && intent.Recovery.Stage == runmodel.ReplacementRecoveryAdministrativeTermination && intent.Recovery.State == runmodel.ReplacementRecoveryRunning && item.State == common.RunStateSuperseded && item.TerminalDelivery != nil
	case "termination_acknowledged":
		return intent.State == runmodel.ReplacementIntentTerminationAcknowledged && intent.Recovery.Stage == runmodel.ReplacementRecoverySuccessorMaterialization && intent.Recovery.State == runmodel.ReplacementRecoveryScheduled
	case "successor_recovery_claimed":
		return intent.State == runmodel.ReplacementIntentTerminationAcknowledged && intent.Recovery.Stage == runmodel.ReplacementRecoverySuccessorMaterialization && intent.Recovery.State == runmodel.ReplacementRecoveryRunning
	case "lineage_sealed":
		return intent.State == runmodel.ReplacementIntentSuccessorCreated && intent.Recovery.State == runmodel.ReplacementRecoveryRunning
	case "replacement_completed":
		return intent.State == runmodel.ReplacementIntentCompleted && intent.Recovery.State == runmodel.ReplacementRecoveryCompleted
	default:
		return false
	}
}

func runReplacementCrashAfterDurableWrite(marker string) {
	if err := os.WriteFile(marker, []byte("durable\n"), 0o600); err != nil {
		panic(err)
	}
	select {}
}

func TestRunReplacementDurableWriteCutsRecoverAfterProcessKill(t *testing.T) {
	if os.Getenv(runReplacementCrashHelperEnv) == "1" {
		runReplacementCrashProcessHelper(t)
		return
	}

	for _, cut := range []string{
		"replacement_intent_sealed",
		"administrative_recovery_claimed",
		"predecessor_terminal_sealed",
		"termination_acknowledged",
		"successor_recovery_claimed",
		"successor_created",
		"lineage_sealed",
		"successor_activated",
		"replacement_completed",
	} {
		cut := cut
		t.Run(cut, func(t *testing.T) {
			root := t.TempDir()
			marker := filepath.Join(root, "crash-ready")
			command := exec.Command(os.Args[0], "-test.run=^TestRunReplacementDurableWriteCutsRecoverAfterProcessKill$", "-test.count=1")
			command.Env = append(os.Environ(),
				runReplacementCrashHelperEnv+"=1",
				runReplacementCrashRootEnv+"="+root,
				runReplacementCrashCutEnv+"="+cut,
				runReplacementCrashMarkerEnv+"="+marker,
			)
			var output bytes.Buffer
			command.Stdout = &output
			command.Stderr = &output
			if err := command.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if command.ProcessState == nil {
					_ = command.Process.Kill()
					_ = command.Wait()
				}
			}()
			waitForRunReplacementCrashMarker(t, marker, command, &output)
			if err := command.Process.Kill(); err != nil {
				t.Fatalf("kill helper at %s: %v", cut, err)
			}
			if err := command.Wait(); err == nil {
				t.Fatalf("helper at %s exited successfully, want process kill", cut)
			} else {
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) {
					t.Fatalf("wait helper at %s: %v", cut, err)
				}
			}
			assertRunReplacementCrashRecovery(t, root, cut)
		})
	}
}

func runReplacementCrashProcessHelper(t *testing.T) {
	t.Helper()
	root := os.Getenv(runReplacementCrashRootEnv)
	cut := os.Getenv(runReplacementCrashCutEnv)
	marker := os.Getenv(runReplacementCrashMarkerEnv)
	if root == "" || cut == "" || marker == "" {
		t.Fatal("missing Run replacement crash helper input")
	}
	base := filerepo.NewRunRepository(runReplacementCrashRepositoryPath(root))
	workspaceID := common.WorkspaceID("ws-run-replacement-process-kill")
	predecessor := seedRunReplacementCrashPredecessor(t, base, workspaceID)
	crashing := &runReplacementCrashRepository{RunRepository: base, cut: cut, marker: marker}
	hub := magicevent.NewHub(16)
	owner := New()
	if err := owner.Configure(runBizTestDependencies(crashing, nil, nil, hub, nil)); err != nil {
		t.Fatal(err)
	}
	observeRunReplacementTermination(t, hub, owner, predecessor)
	command := runReplacementCrashCommand(t, workspaceID)
	if _, err := owner.restartWorkspaceExecutionIntent(common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID), command); err != nil {
		t.Fatalf("seal replacement before %s cut: %v", cut, err)
	}
	for stage := 0; stage < 3; stage++ {
		stored, err := base.GetByWorkspaceID(context.Background(), workspaceID, predecessor.ID)
		if err != nil || stored.ReplacementIntent == nil {
			t.Fatalf("load replacement before %s cut: predecessor=%+v err=%v", cut, stored, err)
		}
		if stored.ReplacementIntent.State == runmodel.ReplacementIntentCompleted {
			break
		}
		if err := owner.driveReplacementRecoveryGeneration(workspaceID, predecessor.ID, stored.ReplacementIntent.Recovery.Generation); err != nil {
			t.Fatalf("drive replacement stage %d before %s cut: %v", stage, cut, err)
		}
	}
	t.Fatalf("Run replacement helper passed %s without reaching the crash cut", cut)
}

func waitForRunReplacementCrashMarker(t *testing.T, marker string, command *exec.Cmd, output *bytes.Buffer) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(marker); err == nil {
			return
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("stat crash marker: %v", err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	_ = command.Process.Kill()
	_ = command.Wait()
	t.Fatalf("Run replacement helper did not reach durable cut; output: %s", output.String())
}

func assertRunReplacementCrashRecovery(t *testing.T, root, cut string) {
	t.Helper()
	ctx := context.Background()
	workspaceID := common.WorkspaceID("ws-run-replacement-process-kill")
	runs := filerepo.NewRunRepository(runReplacementCrashRepositoryPath(root))
	if _, err := runs.ListByWorkspaceID(ctx, workspaceID); err != nil {
		t.Fatalf("reopen Run repository after %s: %v", cut, err)
	}

	hub := magicevent.NewHub(32)
	background := magictask.NewBackgroundRoutine(16)
	owner := New()
	if err := owner.Configure(runBizTestDependencies(runs, nil, nil, hub, background)); err != nil {
		t.Fatalf("configure reopened Run owner after %s: %v", cut, err)
	}
	configuredItems, err := runs.ListByWorkspaceID(ctx, workspaceID)
	if err != nil {
		t.Fatalf("load configured Run inventory after %s: %v", cut, err)
	}
	var configuredPredecessor *runmodel.Run
	for _, item := range configuredItems {
		if item.Lineage.PredecessorRunID == "" {
			configuredPredecessor = item
			break
		}
	}
	if configuredPredecessor == nil {
		t.Fatalf("configured Run inventory after %s lacks predecessor: %+v", cut, configuredItems)
	}
	observeRunReplacementTermination(t, hub, owner, configuredPredecessor)
	if cut == "successor_created" || cut == "lineage_sealed" {
		items, err := runs.ListByWorkspaceID(ctx, workspaceID)
		if err != nil || len(items) != 2 {
			t.Fatalf("linked successor inventory after %s Configure: items=%+v err=%v", cut, items, err)
		}
		var successor *runmodel.Run
		for _, item := range items {
			if item.Lineage.PredecessorRunID != "" {
				successor = item
			}
		}
		if successor == nil || successor.State != common.RunStatePending {
			t.Fatalf("Configure activated linked successor before predecessor lineage recovery at %s: %+v", cut, successor)
		}
		replayCommand := runReplacementCrashCommand(t, workspaceID)
		if !replacementRunMatchesIntent(successor, replacementIntentFromCommand(replayCommand)) {
			t.Fatalf("linked successor lost exact restart identity at %s: %+v", cut, successor)
		}
		preStartReplay, replayErr := owner.restartWorkspaceExecutionIntent(
			common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), replayCommand,
		)
		if replayErr != nil || preStartReplay.RunID != successor.ID || preStartReplay.Outcome != runtimecontract.RuntimeExecutionIntentReplacementPending {
			t.Fatalf("pending linked successor exact replay at %s: successor=%s activation=%q result=%+v err=%v", cut, successor.ID, successor.ActivationRequestID, preStartReplay, replayErr)
		}
		stillPending, loadErr := runs.GetByWorkspaceID(ctx, workspaceID, successor.ID)
		if loadErr != nil || stillPending.State != common.RunStatePending {
			t.Fatalf("exact replay activated linked successor before lineage recovery at %s: run=%+v err=%v", cut, stillPending, loadErr)
		}
		inventory := buildRuntimeRecoveryInventory(items, runevents.QueryRuntimeRecoveryInventoryCommand{Limit: 10})
		for _, item := range inventory.Items {
			if item.RunID == successor.ID {
				t.Fatalf("pending linked successor leaked into SM-4 inventory at %s: %+v", cut, item)
			}
		}
	}
	if err := owner.Start(); err != nil {
		t.Fatalf("start reopened Run owner after %s: %v", cut, err)
	}
	t.Cleanup(func() {
		_ = owner.Stop(context.Background())
		background.Shutdown(context.Background())
		hub.Terminate(context.Background())
	})

	predecessor, successor := waitForRunReplacementCompletion(t, runs, workspaceID, cut)
	predecessorRevision, successorRevision := predecessor.Revision, successor.Revision
	command := runReplacementCrashCommand(t, workspaceID)
	replayed, err := owner.restartWorkspaceExecutionIntent(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), command)
	if err != nil || replayed.RunID != successor.ID || replayed.Outcome != runtimecontract.RuntimeExecutionIntentReconciliationRequested || replayed.Reason != runtimecontract.RuntimeExecutionIntentReasonCurrentRunContinuationAccepted {
		t.Fatalf("exact restart replay after %s: result=%+v err=%v", cut, replayed, err)
	}
	afterReplay, err := runs.ListByWorkspaceID(ctx, workspaceID)
	if err != nil || len(afterReplay) != 2 {
		t.Fatalf("Run inventory after exact replay at %s: items=%+v err=%v", cut, afterReplay, err)
	}
	for _, item := range afterReplay {
		switch item.ID {
		case predecessor.ID:
			if item.Revision != predecessorRevision {
				t.Fatalf("exact replay changed predecessor revision at %s: %d -> %d", cut, predecessorRevision, item.Revision)
			}
		case successor.ID:
			if item.Revision != successorRevision {
				t.Fatalf("exact replay changed successor revision at %s: %d -> %d", cut, successorRevision, item.Revision)
			}
		}
	}
}

func waitForRunReplacementCompletion(t *testing.T, runs repository.RunRepository, workspaceID common.WorkspaceID, cut string) (*runmodel.Run, *runmodel.Run) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID)
	for time.Now().Before(deadline) {
		items, err := runs.ListByWorkspaceID(ctx, workspaceID)
		if err != nil {
			t.Fatalf("list replacement recovery at %s: %v", cut, err)
		}
		if len(items) == 2 {
			var predecessor, successor *runmodel.Run
			for _, item := range items {
				if item.Lineage.PredecessorRunID == "" {
					predecessor = item
				} else {
					successor = item
				}
			}
			if predecessor != nil && successor != nil && predecessor.ReplacementIntent != nil &&
				predecessor.ReplacementIntent.State == runmodel.ReplacementIntentCompleted &&
				predecessor.ReplacementIntent.Recovery.State == runmodel.ReplacementRecoveryCompleted &&
				predecessor.Lineage.SuccessorRunID == successor.ID && predecessor.Lineage.LineageState == runmodel.RunLineageStateSuperseded &&
				successor.State == common.RunStateActive && successor.Lineage.PredecessorRunID == predecessor.ID &&
				successor.Lineage.PredecessorResultID == predecessor.TerminalResultID &&
				successor.WorkspaceRunGeneration == predecessor.WorkspaceRunGeneration+1 {
				return predecessor, successor
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	items, _ := runs.ListByWorkspaceID(ctx, workspaceID)
	t.Fatalf("replacement did not recover after %s: %+v", cut, items)
	return nil, nil
}

func seedRunReplacementCrashPredecessor(t *testing.T, runs repository.RunRepository, workspaceID common.WorkspaceID) *runmodel.Run {
	t.Helper()
	bootstrap := restartTestBootstrapWithDefinition(t, workspaceID, "definition-old", "startup-old")
	item := &runmodel.Run{
		WorkspaceID:        workspaceID,
		GoalID:             bootstrap.Definition.GoalID,
		DoDID:              bootstrap.Definition.DoDID,
		DefinitionRevision: bootstrap.Definition.Revision,
		StartupInputHash:   bootstrap.Definition.StartupInputHash,
		RuntimeBootstrap:   &bootstrap,
		RuntimeState: &runtimecontract.RuntimeStateRecord{
			StateRevision: 9,
			Facts: runtimecontract.RuntimeFacts{
				WorkspaceID:        workspaceID,
				DefinitionRevision: bootstrap.Definition.Revision,
				EvidenceRevision:   "evidence-run-replacement-process-kill",
			},
		},
		State:   common.RunStateActive,
		Lineage: runmodel.RunLineage{LineageState: runmodel.RunLineageStateActive},
	}
	if err := runs.CreateNextWorkspaceRun(context.Background(), item); err != nil {
		t.Fatalf("create active predecessor: %v", err)
	}
	item.RuntimeState.Facts.RunID = item.ID
	item.RuntimeLifecycleFence = &runtimecontract.RuntimeOpaqueLifecycleFence{
		WorkspaceID:          workspaceID,
		RunID:                item.ID,
		RuntimeStateRevision: item.RuntimeState.StateRevision,
		RuntimeStateRef:      runevents.RuntimeStateRef(workspaceID, item.ID, item.RuntimeState.StateRevision),
		RuntimeStateHash:     runtimecontract.StableRuntimeStateRecordHash(*item.RuntimeState),
		BootstrapHash:        runtimecontract.StableRuntimeBootstrapHash(bootstrap),
		TerminalFenceID:      runReplacementTerminalDeliveryID(),
	}
	if err := runs.Update(context.Background(), item); err != nil {
		t.Fatalf("seal predecessor Runtime fence: %v", err)
	}
	return item
}

func observeRunReplacementTermination(t *testing.T, hub magicevent.Hub, owner *Biz, predecessor *runmodel.Run) {
	t.Helper()
	if owner == nil || predecessor == nil || predecessor.RuntimeState == nil {
		t.Fatal("Run replacement termination observer requires owner and Runtime predecessor")
	}
	observer := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	observer.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeTermination, func(ev magicevent.Event, result magicevent.Result) {
		command, ok := ev.Data().(orchestratorevents.RequestRuntimeTerminationCommand)
		if !ok || ev.Source() != ModuleID || command.WorkspaceID != predecessor.WorkspaceID || command.RunID != predecessor.ID || command.Kind != runtimecontract.RuntimeTerminationSuperseded {
			t.Errorf("termination command = source=%q data=%+v", ev.Source(), ev.Data())
			result.Set(nil, nil)
			return
		}
		identity := runtimecontract.RuntimeTerminalDeliveryIdentity{
			WorkspaceID:         predecessor.WorkspaceID,
			RunID:               predecessor.ID,
			TerminalDeliveryID:  runReplacementTerminalDeliveryID(),
			AuthoritySnapshotID: "authority-run-replacement-process-kill",
			DefinitionRevision:  predecessor.DefinitionRevision,
			EvidenceRevision:    "evidence-run-replacement-process-kill",
			DispatchID:          "dispatch-run-replacement-process-kill",
			Outcome: runtimecontract.TerminalOutcome{
				Kind:    runtimecontract.TerminalOutcomeKindSuperseded,
				Code:    "superseded",
				Summary: "workspace execution restart",
			},
		}
		terminalFact := runevents.RuntimeTerminalDeliveryFact{
			RuntimeTerminalDeliveryFact: runtimecontract.RuntimeTerminalDeliveryFact{
				RuntimeTerminalDeliveryIdentity: identity,
				SourceOwner:                     runtimeLifecycleFactSource,
				CausationID:                     "replacement-termination/" + runReplacementTerminalDeliveryID(),
			},
			ResultID: common.ResultID(string(predecessor.WorkspaceID) + "/" + string(predecessor.ID) + "/result/dod-assessment"),
		}
		lifecycleResult, err := owner.executeLifecycleCommand(magicevent.NewEventWithContext(
			runevents.TopicRunFactTerminalDelivery,
			runtimeLifecycleFactSource,
			runevents.CommandDestination,
			magicevent.NewHeader(),
			common.ContextWithRuntimeWorkspaceID(context.Background(), predecessor.WorkspaceID),
			terminalFact,
		))
		if err != nil || lifecycleResult.DeliveryAck == nil {
			t.Errorf("apply Run-owner terminal fact: result=%+v err=%v", lifecycleResult, err)
			result.Set(nil, nil)
			return
		}
		result.Set(orchestratorevents.RequestRuntimeTerminationResult{
			TerminalDeliveryID: identity.TerminalDeliveryID,
			WorkspaceReleased:  true,
			StateRevision:      predecessor.RuntimeState.StateRevision,
		}, nil)
	})
	t.Cleanup(func() { observer.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeTermination) })
}

func runReplacementTerminalDeliveryID() string {
	return "terminal-run-replacement-process-kill"
}

func runReplacementCrashCommand(t *testing.T, workspaceID common.WorkspaceID) runevents.WorkspaceExecutionIntentCommand {
	t.Helper()
	bootstrap := restartTestBootstrap(t, workspaceID)
	return runevents.WorkspaceExecutionIntentCommand{
		WorkspaceID:        workspaceID,
		RequestID:          "restart-run-replacement-process-kill",
		Kind:               runtimecontract.RuntimeExecutionIntentRestart,
		GoalID:             bootstrap.Definition.GoalID,
		DoDID:              bootstrap.Definition.DoDID,
		DefinitionRevision: bootstrap.Definition.Revision,
		StartupInputHash:   bootstrap.Definition.StartupInputHash,
		Bootstrap:          &bootstrap,
	}
}

func runReplacementCrashRepositoryPath(root string) string {
	return filepath.Join(root, "runs.json")
}

var _ repository.RunRepository = (*runReplacementCrashRepository)(nil)
