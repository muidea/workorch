package readmodelprojection

import (
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchresultcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func formalProgressFixture() readmodelcore.RunSnapshotState {
	return readmodelcore.RunSnapshotState{
		Workspace: appmodelcore.Workspace{ID: "ws-formal", AdmittedStartupInputHash: "startup-seal"},
		Run: appmodelcore.Run{ID: "run-formal", WorkspaceID: "ws-formal", GoalID: "goal-formal", DoDID: "dod-formal",
			StartupInputHash: "startup-seal", TerminalResultID: "result-formal", WorkspaceRunGeneration: 2, State: "completed"},
		Results: []projectioncontract.Result{{ID: "result-formal", WorkspaceID: "ws-formal", RunID: "run-formal",
			GoalID: "goal-formal", DoDID: "dod-formal", State: "accepted", Summary: "owner accepted final delivery"}},
	}
}

func formalMilestone(t *testing.T, progress projectioncontract.TaskProgress, id string) projectioncontract.ProgressMilestone {
	t.Helper()
	for _, item := range progress.Milestones {
		if item.ID == id {
			return item
		}
	}
	t.Fatalf("missing %s milestone in %+v", id, progress)
	return projectioncontract.ProgressMilestone{}
}

func TestFormalResultProgressRequiresExactOwnerTerminalScope(t *testing.T) {
	for name, mutate := range map[string]func(*readmodelcore.RunSnapshotState){
		"accepted":       func(*readmodelcore.RunSnapshotState) {},
		"draft":          func(s *readmodelcore.RunSnapshotState) { s.Results[0].State = "draft" },
		"rejected":       func(s *readmodelcore.RunSnapshotState) { s.Results[0].State = "rejected" },
		"archived":       func(s *readmodelcore.RunSnapshotState) { s.Results[0].State = "archived" },
		"missing result": func(s *readmodelcore.RunSnapshotState) { s.Results = nil },
		"only artifact": func(s *readmodelcore.RunSnapshotState) {
			s.Results = nil
			s.Artifacts = []projectioncontract.Artifact{{ID: "artifact", RunID: s.Run.ID}}
		},
		"active run":               func(s *readmodelcore.RunSnapshotState) { s.Run.State = "active" },
		"failed run":               func(s *readmodelcore.RunSnapshotState) { s.Run.State = "failed" },
		"cancelled run":            func(s *readmodelcore.RunSnapshotState) { s.Run.State = "cancelled" },
		"superseded run":           func(s *readmodelcore.RunSnapshotState) { s.Run.State = "superseded" },
		"missing run":              func(s *readmodelcore.RunSnapshotState) { s.Run.ID = "" },
		"missing terminal id":      func(s *readmodelcore.RunSnapshotState) { s.Run.TerminalResultID = "" },
		"different terminal id":    func(s *readmodelcore.RunSnapshotState) { s.Run.TerminalResultID = "other-result" },
		"missing seal":             func(s *readmodelcore.RunSnapshotState) { s.Workspace.AdmittedStartupInputHash = "" },
		"missing run hash":         func(s *readmodelcore.RunSnapshotState) { s.Run.StartupInputHash = "" },
		"different seal":           func(s *readmodelcore.RunSnapshotState) { s.Workspace.AdmittedStartupInputHash = "new-seal" },
		"cross workspace":          func(s *readmodelcore.RunSnapshotState) { s.Results[0].WorkspaceID = "other-workspace" },
		"run cross workspace":      func(s *readmodelcore.RunSnapshotState) { s.Run.WorkspaceID = "other-workspace" },
		"cross run":                func(s *readmodelcore.RunSnapshotState) { s.Results[0].RunID = "other-run" },
		"cross goal":               func(s *readmodelcore.RunSnapshotState) { s.Results[0].GoalID = "other-goal" },
		"cross dod":                func(s *readmodelcore.RunSnapshotState) { s.Results[0].DoDID = "other-dod" },
		"missing result goal":      func(s *readmodelcore.RunSnapshotState) { s.Results[0].GoalID = "" },
		"missing result workspace": func(s *readmodelcore.RunSnapshotState) { s.Results[0].WorkspaceID = "" },
		"missing run dod":          func(s *readmodelcore.RunSnapshotState) { s.Run.DoDID = "" },
		"acceptance slice":         func(s *readmodelcore.RunSnapshotState) { s.Results[0].SourceType = "acceptance_slice" },
		"diagnostic slice": func(s *readmodelcore.RunSnapshotState) {
			s.Results[0].SummaryDetail = &workorchresultcontract.ResultSummary{Diagnostics: workorcheventpayload.New(workorcheventpayload.StringField("resultScope", "acceptance_slice"))}
		},
		"not final gate": func(s *readmodelcore.RunSnapshotState) {
			s.Results[0].SummaryDetail = &workorchresultcontract.ResultSummary{Diagnostics: workorcheventpayload.New(workorcheventpayload.StringField("finalCompletionGateState", "not_final_result"))}
		},
		"duplicate terminal id": func(s *readmodelcore.RunSnapshotState) { s.Results = append(s.Results, s.Results[0]) },
		"duplicate terminal id wrong run": func(s *readmodelcore.RunSnapshotState) {
			other := s.Results[0]
			other.RunID = "other-run"
			s.Results = append(s.Results, other)
		},
		"accepted owner ignores coverage and assessments": func(s *readmodelcore.RunSnapshotState) {
			s.CoverageWorkItems = []map[string]any{{"status": "active"}}
			s.Results[0].DoDAssessment = &workorchresultcontract.DoDAssessment{Satisfied: false}
		},
	} {
		t.Run(name, func(t *testing.T) {
			snapshot := formalProgressFixture()
			mutate(&snapshot)
			progress := mapLocalizedTaskProgress(snapshot)
			got := formalMilestone(t, progress, "formal_result")
			want := "failed"
			if name == "accepted" || name == "accepted owner ignores coverage and assessments" {
				want = "completed"
			} else if name == "active run" {
				want = "running"
			}
			if got.State != want {
				t.Fatalf("formal result = %+v, want %s", got, want)
			}
			if want == "failed" && !strings.Contains(progress.Summary, "formal result failed") {
				// An exact rejected owner Result may supply its original failure summary.
				if name != "rejected" {
					t.Fatalf("missing explicit terminal diagnostic: %s", progress.Summary)
				}
			}
		})
	}
}

func TestWorkspaceFormalProgressRequiresSealAndOwnerGeneration(t *testing.T) {
	for _, state := range []string{"", "draft", "active", "archived"} {
		snapshot := readmodelcore.WorkspaceSnapshotState{Workspace: appmodelcore.Workspace{ID: "ws-formal", State: state},
			Results: []projectioncontract.Result{{ID: "historical-result", State: "accepted"}}}
		progress := mapLocalizedWorkspaceTaskProgress(snapshot)
		if formalMilestone(t, progress, "workspace_defined").State != "pending" || formalMilestone(t, progress, "formal_result").State != "pending" {
			t.Fatalf("state=%s inventory became formal completion: %+v", state, progress)
		}
		snapshot.Workspace.AdmittedStartupInputHash = "startup-seal"
		progress = mapLocalizedWorkspaceTaskProgress(snapshot)
		if formalMilestone(t, progress, "workspace_defined").State != "completed" || formalMilestone(t, progress, "formal_result").State != "pending" || progress.CurrentMilestone != "formal_result" {
			t.Fatalf("seal without Run must define, not complete: %+v", progress)
		}
	}
	base := formalProgressFixture()
	old := base.Run
	old.ID, old.TerminalResultID, old.WorkspaceRunGeneration = "zz-predecessor", "old-result", 1
	old.EndedAt = "2099-01-01T00:00:00Z"
	newRun := base.Run
	newRun.State, newRun.TerminalResultID = "active", ""
	oldResult := base.Results[0]
	oldResult.ID, oldResult.RunID = old.TerminalResultID, old.ID
	for name, mutate := range map[string]func(*readmodelcore.WorkspaceSnapshotState){
		"owner generation": func(*readmodelcore.WorkspaceSnapshotState) {},
		"reverse history":  func(s *readmodelcore.WorkspaceSnapshotState) { s.Runs[0], s.Runs[1] = s.Runs[1], s.Runs[0] },
		"cross workspace history": func(s *readmodelcore.WorkspaceSnapshotState) {
			other := old
			other.WorkspaceID, other.WorkspaceRunGeneration = "other-workspace", 99
			s.Runs = append(s.Runs, other)
		},
		"current run":        func(s *readmodelcore.WorkspaceSnapshotState) { s.Run = &newRun },
		"active identity":    func(s *readmodelcore.WorkspaceSnapshotState) { s.Workspace.ActiveRunID = newRun.ID },
		"unknown workspace":  func(s *readmodelcore.WorkspaceSnapshotState) { s.Runs[0].WorkspaceID = "" },
		"unknown generation": func(s *readmodelcore.WorkspaceSnapshotState) { s.Runs[0].WorkspaceRunGeneration = 0 },
		"tied generation": func(s *readmodelcore.WorkspaceSnapshotState) {
			s.Runs[1].WorkspaceRunGeneration = newRun.WorkspaceRunGeneration
		},
	} {
		t.Run(name, func(t *testing.T) {
			snapshot := readmodelcore.WorkspaceSnapshotState{Workspace: base.Workspace, Runs: []appmodelcore.Run{newRun, old}, Results: []projectioncontract.Result{oldResult},
				EventRecords: []projectioncontract.EventRecord{{RunID: old.ID, OccurredAt: "2099-01-01T00:00:00Z"}}}
			mutate(&snapshot)
			progress := mapLocalizedWorkspaceTaskProgress(snapshot)
			if got := formalMilestone(t, progress, "formal_result"); got.State == "completed" || len(got.EvidenceRefs) != 0 {
				t.Fatalf("historical accepted result rolled progress back: %+v", progress)
			}
			if name != "unknown generation" && name != "unknown workspace" && name != "tied generation" && progress.RunID != newRun.ID {
				t.Fatalf("progress Run = %s, want owner latest %s", progress.RunID, newRun.ID)
			}
		})
	}
}

func TestFormalProgressTerminalExactIDAndOverlayContextStayConsistent(t *testing.T) {
	for _, delivered := range []bool{false, true} {
		snapshot := formalProgressFixture()
		wrong := snapshot.Results[0]
		wrong.ID, wrong.Summary = "unacknowledged-result", "unacknowledged accepted result"
		snapshot.Results = append(snapshot.Results, wrong)
		if !delivered {
			snapshot.Results = snapshot.Results[1:]
		}
		progress := mapLocalizedTaskProgress(snapshot)
		raw := &runmodel.Run{ID: common.RunID(snapshot.Run.ID), State: common.RunStateCompleted, RuntimeState: &runtimecontract.RuntimeStateRecord{
			Decision: runtimecontract.RuntimeDecision{State: "completed"},
		}}
		applyRuntimeProgress(&progress, raw)
		progress = finalizeRunFormalProgress(progress, snapshot)
		snapshot.TaskProgress = progress
		bundle := mapRunContextBundle(snapshot)
		if !reflect.DeepEqual(bundle.TaskProgress, progress) || !reflect.DeepEqual(bundle.CompressedContext.Milestones, progress.Milestones) || bundle.Summary != progress.Summary {
			t.Fatalf("run context differs from formal progress: %+v", bundle)
		}
		workspace := readmodelcore.WorkspaceSnapshotState{Workspace: snapshot.Workspace, Run: &snapshot.Run, Runs: []appmodelcore.Run{snapshot.Run}, Results: snapshot.Results, TaskProgress: progress}
		workspace.TaskProgress = finalizeWorkspaceFormalProgress(workspace.TaskProgress, workspace)
		view := projectWorkspaceRuntimeView(workspace, 10, "")
		workspaceBundle := mapWorkspaceContextBundle(workspace)
		if !reflect.DeepEqual(view.TaskProgress, workspace.TaskProgress) || !reflect.DeepEqual(workspaceBundle.TaskProgress, workspace.TaskProgress) {
			t.Fatal("workspace overlay/context bypassed shared formal progress")
		}
		want := "failed"
		if delivered {
			want = "completed"
		}
		milestone := formalMilestone(t, view.TaskProgress, "formal_result")
		if milestone.State != want || (!delivered && !strings.Contains(view.TaskProgress.Summary, "formal result failed")) {
			t.Fatalf("runtime success fabricated delivery=%t: %+v", delivered, view.TaskProgress)
		}
		if delivered && (len(milestone.EvidenceRefs) != 1 || milestone.EvidenceRefs[0] != snapshot.Run.TerminalResultID || progress.Summary != "owner accepted final delivery") {
			t.Fatalf("exact terminal result lost to other accepted result: %+v", progress)
		}
	}
}

func TestFormalProgressProductionSnapshotsObserveCurrentRunAndRecheckAfterOverlay(t *testing.T) {
	workspace := &workspacemodel.Workspace{ID: "ws-owner", State: common.WorkspaceStateActive,
		StartupInput: &workspacemodel.WorkspaceStartupInputContract{StartupInputHash: "owner-seal"}}
	run := &runmodel.Run{ID: "run-owner", WorkspaceID: "ws-owner", GoalID: "goal-owner", DoDID: "dod-owner",
		State: common.RunStateCompleted, StartupInputHash: "owner-seal", TerminalResultID: "result-owner", WorkspaceRunGeneration: 2,
		RuntimeState: &runtimecontract.RuntimeStateRecord{Decision: runtimecontract.RuntimeDecision{State: "completed"}}}
	result := &resultmodel.Result{ID: "result-owner", WorkspaceID: "ws-owner", RunID: "run-owner", GoalID: "goal-owner", DoDID: "dod-owner",
		State: common.ResultStateAccepted, Summary: "exact owner final Result"}
	for _, delivered := range []bool{false, true} {
		var results []*resultmodel.Result
		if delivered {
			results = []*resultmodel.Result{result}
		}
		runSnapshot := runSnapshotFromModels(RunSnapshotModelInput{Workspace: workspace, Run: run, Results: results})
		workspaceSnapshot := workspaceSnapshotFromModels(WorkspaceSnapshotModelInput{Workspace: workspace, ProjectionRun: run, Results: results})
		want := "failed"
		if delivered {
			want = "completed"
		}
		for _, progress := range []projectioncontract.TaskProgress{runSnapshot.TaskProgress, workspaceSnapshot.TaskProgress} {
			if milestone := formalMilestone(t, progress, "formal_result"); milestone.State != want || progress.RunID != string(run.ID) {
				t.Fatalf("delivered=%t production snapshot lost exact current owner: %+v", delivered, progress)
			}
			if !delivered && !strings.Contains(progress.Summary, "formal result failed") {
				t.Fatalf("production Runtime summary overwrote missing delivery diagnostic: %s", progress.Summary)
			}
		}
		merged := workspaceSnapshotWithRunSnapshot(workspaceSnapshot, &runSnapshot)
		if formalMilestone(t, merged.TaskProgress, "formal_result").State != want || !reflect.DeepEqual(merged.TaskProgress, merged.ContextBundle.TaskProgress) {
			t.Fatalf("workspace merge/context differs from formal result: %+v", merged.TaskProgress)
		}
	}
	// A completed semantic Runtime is not a Run-owner terminal delivery.
	run.State, run.TerminalResultID = common.RunStateActive, ""
	active := runSnapshotFromModels(RunSnapshotModelInput{Workspace: workspace, Run: run})
	if formalMilestone(t, active.TaskProgress, "formal_result").State != "pending" {
		t.Fatalf("active Runtime success fabricated formal completion: %+v", active.TaskProgress)
	}
}

func TestFormalProgressRuntimeOverlayCannotChangeFormalResult(t *testing.T) {
	for _, runState := range []common.RunState{common.RunStateCompleted, common.RunStateFailed, common.RunStateActive} {
		progress := projectioncontract.TaskProgress{State: string(runState), Milestones: []projectioncontract.ProgressMilestone{{ID: "formal_result", State: "pending"}}}
		applyRuntimeProgressToMilestones(&progress, projectioncontract.RuntimeProgress{ConvergenceState: "completed", AcceptedEvidenceCount: 10}, string(runState))
		if formalMilestone(t, progress, "formal_result").State != "pending" {
			t.Fatalf("Runtime overlay changed formal result for %s: %+v", runState, progress)
		}
	}
}

func TestFormalProgressPreservesUnrelatedCachedSummaryWithoutFormalProjection(t *testing.T) {
	for _, state := range []string{"active", "completed"} {
		progress := projectioncontract.TaskProgress{WorkspaceID: "ws-custom", RunID: "run-custom", State: state, Summary: "custom cached summary"}
		snapshot := readmodelcore.RunSnapshotState{Workspace: appmodelcore.Workspace{ID: "ws-custom"}, Run: appmodelcore.Run{ID: "run-custom", WorkspaceID: "ws-custom", State: state}, TaskProgress: progress}
		if got := runSnapshotTaskProgress(snapshot); !reflect.DeepEqual(got, progress) {
			t.Fatalf("unrelated cached Run progress changed: %+v", got)
		}
		workspace := readmodelcore.WorkspaceSnapshotState{Workspace: snapshot.Workspace, Run: &snapshot.Run, TaskProgress: progress}
		if got := workspaceSnapshotTaskProgress(workspace); !reflect.DeepEqual(got, progress) {
			t.Fatalf("unrelated cached Workspace progress changed: %+v", got)
		}
	}
}

func TestFormalProgressRevalidatesCachedMilestonesWithoutMutatingSteps(t *testing.T) {
	snapshot := formalProgressFixture()
	snapshot.Results = nil
	snapshot.TaskProgress = projectioncontract.TaskProgress{WorkspaceID: snapshot.Run.WorkspaceID, RunID: snapshot.Run.ID, State: "completed", Summary: "stale success",
		Milestones: []projectioncontract.ProgressMilestone{{ID: "execution_steps", State: "completed"}, {ID: "validation", State: "completed"}, {ID: "formal_result", State: "completed"}}}
	got := runSnapshotTaskProgress(snapshot)
	if formalMilestone(t, got, "formal_result").State != "failed" || got.CurrentMilestone != "formal_result" || !strings.Contains(got.Summary, "formal result failed") {
		t.Fatalf("cached formal success bypassed owner fact: %+v", got)
	}
	if formalMilestone(t, snapshot.TaskProgress, "formal_result").State != "completed" || formalMilestone(t, got, "execution_steps").State != "completed" || formalMilestone(t, got, "validation").State != "completed" {
		t.Fatal("formal projection modified input or step/validation semantics")
	}
}

func TestRepairLoopUsesExactCurrentFailureCountWithoutLatchingDisplayFlags(t *testing.T) {
	for _, count := range []int{1, 2} {
		state := runtimecontract.RuntimeStateRecord{Facts: runtimecontract.RuntimeFacts{
			Repair:  runtimecontract.RuntimeRepairLedger{RepairAttempts: 2, SignatureCounts: []runtimecontract.RuntimeFailureSignatureCount{{Signature: "old", Count: 4}, {Signature: "current", Count: count}}},
			Failure: &runtimecontract.FailureFact{Class: runtimecontract.FailureClassExecutionFailure, Signature: "current", Message: "current failure"},
		}}
		summary := mapRuntimeProgress(state)
		wantRepeated := 0
		if count > 1 {
			wantRepeated = count
		}
		if got := runtimeRepeatedFailureOccurrences(state.Facts); got != wantRepeated {
			t.Fatalf("count=%d historical unrelated signature leaked into repeated failure count: %d", count, got)
		}
		progress := projectioncontract.TaskProgress{State: "active", Runtime: &summary,
			RepairLoop: projectioncontract.RepairLoop{RepeatedFailureReason: "prior projection", Escalated: true}}
		applyRuntimeProgressToRepairLoop(&progress, summary, state)
		if (progress.RepairLoop.RepeatedFailureReason != "") != (count > 1) || progress.RepairLoop.Escalated {
			t.Fatalf("count=%d loop=%+v", count, progress.RepairLoop)
		}
		if got := heartbeatIneffectiveRetry(readmodelcore.WorkspaceSnapshotState{TaskProgress: progress,
			ConvergenceStatus: &projectioncontract.ConvergenceStatus{IneffectiveRetry: true, DecisionCause: "retry_exhaustion"}}); got != (count > 1) {
			t.Fatalf("count=%d stale display flag became authority: %t", count, got)
		}
	}
}

func TestRuntimeProgressValidationCountRequiresCoverageCredit(t *testing.T) {
	state := runtimecontract.RuntimeStateRecord{Facts: runtimecontract.RuntimeFacts{
		Focus: runtimecontract.CoverageFocus{ID: "focus", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi"}},
		Evidence: runtimecontract.EvidenceLedger{Accepted: []runtimecontract.EvidenceFact{
			{ID: "auxiliary", FocusID: "focus", Kind: runtimecontract.EvidenceKindValidation},
			{ID: "credited", FocusID: "focus", Kind: runtimecontract.EvidenceKindValidation, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi"}},
		}},
	}}
	if got := mapRuntimeProgress(state).AcceptedValidationEvidenceCount; got != 1 {
		t.Fatalf("uncredited validation became acceptance progress: %d", got)
	}
}

func TestRuntimeProgressTimestampRequiresAcceptedExactOwnerReceipt(t *testing.T) {
	receipt := runtimecontract.RuntimeOperationReceipt{ExecutionKey: "effect-key", Fence: 1, OperationID: "effect", Outcome: runtimecontract.RuntimeOperationOutcomeAccepted,
		ObservedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable, Changed: true, CompletedAt: "2026-06-10T00:01:00Z"}
	state := runtimecontract.RuntimeStateRecord{Facts: runtimecontract.RuntimeFacts{WorkspaceID: "ws-a", RunID: "run-a",
		Evidence: runtimecontract.EvidenceLedger{Accepted: []runtimecontract.EvidenceFact{{ID: "evidence", DispatchID: "dispatch", OperationID: "effect", Kind: runtimecontract.EvidenceKindStateChange}}}},
		CompletedDispatches: []runtimecontract.RuntimeDispatchRecord{{ID: "dispatch", WorkspaceID: "ws-a", RunID: "run-a", OperationExecutions: []runtimecontract.RuntimeOperationExecutionRecord{{
			OperationID: "effect", ExecutionKey: "effect-key", Fence: 1, State: runtimecontract.OperationExecutionEvidenceCommitted, OwnerReceiptHash: runtimecontract.StableRuntimeOperationReceiptHash(receipt),
		}}}}, OperationReceipts: []runtimecontract.RuntimeOperationReceipt{receipt},
	}
	if got := latestRuntimeAcceptedEvidenceAt(state); got != "" {
		t.Fatalf("causation-only repair became acceptance progress: %q", got)
	}
	if acceptedAt, effectAt, effects := runtimeOwnerEvidenceProgress(state); acceptedAt != "" || effectAt != receipt.CompletedAt || effects != 1 {
		t.Fatalf("repair effect clock/count lost exact owner fact: %s %s %d", acceptedAt, effectAt, effects)
	}
	credited := state.Clone()
	credited.Facts.Evidence.Accepted[0].CoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-a"}
	if got := latestRuntimeAcceptedEvidenceAt(credited); got != receipt.CompletedAt {
		t.Fatalf("credited evidence lost its clock: %s", got)
	}
	for name, mutate := range map[string]func(*runtimecontract.RuntimeStateRecord){
		"rejected": func(s *runtimecontract.RuntimeStateRecord) {
			s.Facts.Evidence.Rejected = s.Facts.Evidence.Accepted
			s.Facts.Evidence.Accepted = nil
		},
		"receipt drift": func(s *runtimecontract.RuntimeStateRecord) { s.OperationReceipts[0].AfterHash = "changed" },
		"owner committed only": func(s *runtimecontract.RuntimeStateRecord) {
			s.CompletedDispatches[0].OperationExecutions[0].State = runtimecontract.OperationExecutionOwnerCommitted
		},
		"other run": func(s *runtimecontract.RuntimeStateRecord) { s.CompletedDispatches[0].RunID = "run-old" },
	} {
		t.Run(name, func(t *testing.T) {
			changed := state.Clone()
			mutate(&changed)
			if got := latestRuntimeAcceptedEvidenceAt(changed); got != "" {
				t.Fatalf("invalid progress timestamp: %q", got)
			}
			if _, at, count := runtimeOwnerEvidenceProgress(changed); at != "" || count != 0 {
				t.Fatal("invalid receipt supplied effect progress")
			}
		})
	}
}

func TestHeartbeatSeparatesActivityEffectiveProgressAndTerminalOutcome(t *testing.T) {
	snapshot := readmodelcore.WorkspaceSnapshotState{Workspace: appmodelcore.Workspace{ID: "ws-a"},
		Run:          &appmodelcore.Run{ID: "run-a", WorkspaceID: "ws-a", State: string(common.RunStateActive)},
		TaskProgress: projectioncontract.TaskProgress{RunID: "run-a", Runtime: &projectioncontract.RuntimeProgress{LatestAcceptedEvidenceAt: "2026-06-10T00:01:00Z"}},
		EventRecords: []projectioncontract.EventRecord{{RunID: "run-a", OccurredAt: "2026-06-10T00:12:00Z"}, {RunID: "", OccurredAt: "2026-06-10T00:20:00Z"}},
		Results:      []projectioncontract.Result{{RunID: "run-old", State: "accepted", CreatedAt: "2026-06-10T00:22:00Z"}, {RunID: "run-a", State: "rejected", CreatedAt: "2026-06-10T00:13:00Z"}},
	}
	heartbeat := mapRunHeartbeatAt(snapshot, mustParseTime(t, "2026-06-10T00:14:00Z"))
	if heartbeat.LatestEffectiveProgressAt != "2026-06-10T00:01:00Z" || heartbeat.LatestActivityAt != "2026-06-10T00:13:00Z" || heartbeat.Stalled || heartbeat.IneffectiveRetry {
		t.Fatalf("activity or other-run outcome promoted to effective progress: %+v", heartbeat)
	}
	for _, runID := range []string{"", "run-old"} {
		stale := snapshot
		stale.TaskProgress.RunID = runID
		stale.Acceptance = projectioncontract.AcceptanceProjection{Audit: &projectioncontract.RuntimeAudit{RunID: "run-old"}, Budget: &projectioncontract.ConvergenceBudget{Decision: "budget_exhausted"}}
		stale.ConvergenceStatus = &projectioncontract.ConvergenceStatus{IneffectiveRetry: true, DecisionCause: "retry_exhaustion"}
		projected := mapRunHeartbeatAt(stale, mustParseTime(t, "2026-06-10T00:14:00Z"))
		if projected.LatestEffectiveProgressAt != "" || projected.IneffectiveRetry {
			t.Fatalf("unscoped/old-run projection leaked progress or retry: %+v", projected)
		}
	}
	snapshot.Run.State = string(common.RunStateFailed)
	heartbeat = mapRunHeartbeatAt(snapshot, mustParseTime(t, "2026-06-10T00:30:00Z"))
	if heartbeat.ActivityState != "failed" || heartbeat.Stalled {
		t.Fatalf("earlier progress hid terminal failure: %+v", heartbeat)
	}
}
