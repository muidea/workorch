package appservice

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	runtimeevidencemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchresultcontract"
)

func TestReadRuntimeRawContentRejectsUnownedLLMCallFiles(t *testing.T) {
	ref := runtimePayloadTestRef("ws-raw-content", "run-raw-content", "llm-calls", "run-raw-content/llm-1", "response.sse")
	for _, backend := range []string{"memory", "file"} {
		t.Run(backend, func(t *testing.T) {
			if _, err := ReadRuntimeRawContentWithBackend(context.Background(), backend, t.TempDir(), ref); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("unowned llm raw read err = %v, want invalid argument", err)
			}
		})
	}
}

func TestReadRuntimeRawContentUsesOwnerTypedReadsAcrossBackends(t *testing.T) {
	for _, backend := range []string{"memory", "file"} {
		for _, tc := range []struct {
			name       string
			collection string
			itemID     string
			fileName   string
			want       string
		}{
			{name: "step", collection: "steps", itemID: "run-raw-runtime/step-1", fileName: "output.json", want: `{"stage":"done"}`},
			{name: "tool-call", collection: "tool-calls", itemID: "run-raw-runtime/step-1/tool-1", fileName: "arguments.json", want: `{"command":"test"}`},
		} {
			t.Run(backend+"/"+tc.name, func(t *testing.T) {
				dataRoot := t.TempDir()
				session, ctx := newTestRuntimeSession(t, backend, dataRoot)
				exports := testRuntimeExports(session.Bootstrap)
				workspaceID := common.WorkspaceID("ws-raw-runtime")
				runID := common.RunID("run-raw-runtime")
				stepID := common.StepID("run-raw-runtime/step-1")
				toolCallID := common.ToolCallID("run-raw-runtime/step-1/tool-1")
				ctx = common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
				if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "raw runtime", State: common.WorkspaceStateActive}); err != nil {
					t.Fatal(err)
				}
				if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCompleted}); err != nil {
					t.Fatal(err)
				}
				switch tc.collection {
				case "steps":
					if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{ID: stepID, RunID: runID, Kind: "test", State: common.StepStateSucceeded, Output: workorcheventpayload.New(workorcheventpayload.StringField("stage", "done"))}); err != nil {
						t.Fatal(err)
					}
				case "tool-calls":
					if err := exports.Repositories.ToolCalls.Create(ctx, &runtimeevidencemodel.ToolCall{ID: toolCallID, WorkspaceID: workspaceID, RunID: runID, StepID: stepID, ToolName: "test", State: common.ToolCallStateCompleted, Arguments: workorcheventpayload.New(workorcheventpayload.StringField("command", "test"))}); err != nil {
						t.Fatal(err)
					}
					toolCalls, err := ownerreadports.ListToolCallsByRunOwnerCommand(ctx, exports.Infrastructure.CommandHub, runID)
					if err != nil || len(toolCalls) != 1 || toolCalls[0] == nil || toolCalls[0].ID != toolCallID {
						t.Fatalf("owner tool-call read = %+v err=%v", toolCalls, err)
					}
				}

				ref := runtimePayloadTestRef(workspaceID, runID, tc.collection, tc.itemID, tc.fileName)
				payload, err := ReadRuntimeRawContentWithSession(ctx, session, ref)
				if err != nil || payload.ContentType != "application/json; charset=utf-8" || string(payload.Content) != tc.want || len(payload.ContentSHA256) != 64 {
					t.Fatalf("ref=%q payload=%+v/%q err=%v", ref, payload, payload.Content, err)
				}
			})
		}
	}
}

func TestReadRuntimeRawContentReturnsTypedCheckpointAndResultDetailsAcrossBackends(t *testing.T) {
	for _, backend := range []string{"memory", "file"} {
		for _, kind := range []string{"runtime-checkpoint", "result"} {
			t.Run(backend+"/"+kind, func(t *testing.T) {
				session, ctx := newTestRuntimeSession(t, backend, t.TempDir())
				exports := testRuntimeExports(session.Bootstrap)
				workspaceID := common.WorkspaceID("ws-raw-virtual")
				runID := common.RunID("run-raw-virtual")
				checkpointID := common.CheckpointID("run-raw-virtual/checkpoint-1")
				resultID := common.ResultID("run-raw-virtual/result-1")
				ctx = common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
				if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "raw virtual", State: common.WorkspaceStateActive}); err != nil {
					t.Fatalf("create workspace: %v", err)
				}
				if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCompleted}); err != nil {
					t.Fatalf("create run: %v", err)
				}
				var ref string
				var want string
				switch kind {
				case "runtime-checkpoint":
					if err := exports.Repositories.RuntimeCheckpoints.Create(ctx, &runmodel.RuntimeCheckpoint{
						ID: checkpointID, WorkspaceID: workspaceID, RunID: runID,
						CheckpointType: common.CheckpointTypeRunState, RunStateSnapshot: common.RunStateCompleted,
						Data: mustAppserviceGovernancePayload(t, map[string]any{"failureClass": "none"}),
					}); err != nil {
						t.Fatalf("create checkpoint: %v", err)
					}
					ref = runtimePayloadTestRef(workspaceID, runID, "runtime-checkpoints", string(checkpointID), "data.json")
					want = `"failureClass":"none"`
				case "result":
					if err := exports.Repositories.Results.Create(ctx, &resultmodel.Result{
						ID: resultID, WorkspaceID: workspaceID, RunID: runID, State: common.ResultStateAccepted,
						DoDAssessment: workorchresultcontract.DoDAssessment{Satisfied: true, Summary: "accepted"},
						SummaryDetail: workorchresultcontract.ResultSummary{GoalOutcome: "done"},
					}); err != nil {
						t.Fatalf("create result: %v", err)
					}
					ref = runtimePayloadTestRef(workspaceID, runID, "results", string(resultID), "summary-detail.json")
					want = `"goalOutcome":"done"`
				}
				payload, err := ReadRuntimeRawContentWithSession(ctx, session, ref)
				if err != nil || payload.ContentType != "application/json; charset=utf-8" || !strings.Contains(string(payload.Content), want) || len(payload.ContentSHA256) != 64 {
					t.Fatalf("payload = %+v/%q err=%v, want %s", payload, string(payload.Content), err, want)
				}
			})
		}
	}
}

func TestReadRuntimeRawContentWithBackendScopesVirtualReadsByURIWorkspace(t *testing.T) {
	dataRoot := t.TempDir()
	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports := testRuntimeExports(session.Bootstrap)
	workspaceA := common.WorkspaceID("ws-raw-scope-a")
	workspaceB := common.WorkspaceID("ws-raw-scope-b")
	runID := common.RunID("run-raw-scope")
	checkpointID := common.CheckpointID("run-raw-scope/checkpoint-1")
	resultID := common.ResultID("run-raw-scope/result-1")
	ctxA := common.ContextWithRuntimeWorkspaceID(ctx, workspaceA)
	ctxB := common.ContextWithRuntimeWorkspaceID(ctx, workspaceB)
	for _, workspaceID := range []common.WorkspaceID{workspaceA, workspaceB} {
		scopedCtx := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
		if err := exports.Repositories.Workspaces.Create(scopedCtx, &workspacemodel.Workspace{ID: workspaceID, Title: string(workspaceID), State: common.WorkspaceStateActive}); err != nil {
			t.Fatalf("create workspace %s: %v", workspaceID, err)
		}
		if err := exports.Repositories.Runs.Create(scopedCtx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCompleted}); err != nil {
			t.Fatalf("create run %s: %v", workspaceID, err)
		}
	}
	if err := exports.Repositories.RuntimeCheckpoints.Create(ctxA, &runmodel.RuntimeCheckpoint{
		ID:               checkpointID,
		WorkspaceID:      workspaceA,
		RunID:            runID,
		CheckpointType:   common.CheckpointTypeRunState,
		RunStateSnapshot: common.RunStateCompleted,
		Data:             mustAppserviceGovernancePayload(t, map[string]any{"workspace": string(workspaceA)}),
	}); err != nil {
		t.Fatalf("create checkpoint A: %v", err)
	}
	if err := exports.Repositories.RuntimeCheckpoints.Create(ctxB, &runmodel.RuntimeCheckpoint{
		ID:               checkpointID,
		WorkspaceID:      workspaceB,
		RunID:            runID,
		CheckpointType:   common.CheckpointTypeRunState,
		RunStateSnapshot: common.RunStateCompleted,
		Data:             mustAppserviceGovernancePayload(t, map[string]any{"workspace": string(workspaceB)}),
	}); err != nil {
		t.Fatalf("create checkpoint B: %v", err)
	}
	if err := exports.Repositories.Results.Create(ctxA, &resultmodel.Result{
		ID:            resultID,
		WorkspaceID:   workspaceA,
		RunID:         runID,
		State:         common.ResultStateAccepted,
		SummaryDetail: workorchresultcontract.ResultSummary{GoalOutcome: "workspace-a"},
	}); err != nil {
		t.Fatalf("create result A: %v", err)
	}
	if err := exports.Repositories.Results.Create(ctxB, &resultmodel.Result{
		ID:            resultID,
		WorkspaceID:   workspaceB,
		RunID:         runID,
		State:         common.ResultStateAccepted,
		SummaryDetail: workorchresultcontract.ResultSummary{GoalOutcome: "workspace-b"},
	}); err != nil {
		t.Fatalf("create result B: %v", err)
	}

	checkpointRef := runtimePayloadTestRef(workspaceB, runID, "runtime-checkpoints", string(checkpointID), "data.json")
	checkpointPayload, err := ReadRuntimeRawContentWithBackend(context.Background(), "file", dataRoot, checkpointRef)
	if err != nil {
		t.Fatalf("ReadRuntimeRawContentWithBackend checkpoint: %v", err)
	}
	if !strings.Contains(string(checkpointPayload.Content), `"workspace":"ws-raw-scope-b"`) {
		t.Fatalf("checkpoint payload = %q, want workspace B scoped content", string(checkpointPayload.Content))
	}

	resultRef := runtimePayloadTestRef(workspaceB, runID, "results", string(resultID), "summary-detail.json")
	resultPayload, err := ReadRuntimeRawContentWithBackend(context.Background(), "file", dataRoot, resultRef)
	if err != nil {
		t.Fatalf("ReadRuntimeRawContentWithBackend result: %v", err)
	}
	if !strings.Contains(string(resultPayload.Content), `"goalOutcome":"workspace-b"`) {
		t.Fatalf("result payload = %q, want workspace B scoped content", string(resultPayload.Content))
	}
}

func runtimePayloadTestRef(workspaceID common.WorkspaceID, runID common.RunID, collection string, itemID string, fileName string) string {
	itemToken := base64.RawURLEncoding.EncodeToString([]byte(itemID))
	return "workorch://workspaces/" + string(workspaceID) + "/runs/" + string(runID) + "/" + collection + "/" + itemToken + "/" + fileName
}

func TestReadRuntimeRawContentWithBackendRejectsUnsafeRefs(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	if _, err := ReadRuntimeRawContentWithBackend(ctx, "file", dataRoot, "file:///tmp/secret"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("external ref err = %v, want invalid argument", err)
	}
	if _, err := ReadRuntimeRawContentWithBackend(ctx, "file", dataRoot, "workorch://workspaces/../runs/run-a/llm-calls/step/request.json"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("unsafe workspace ref err = %v, want invalid argument", err)
	}
	if _, err := ReadRuntimeRawContentWithBackend(ctx, "file", dataRoot, "workorch://workspaces/ws-a/runs/run-a/llm-calls/step/secret.txt"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("unknown file ref err = %v, want invalid argument", err)
	}
	if _, err := ReadRuntimeRawContentWithBackend(ctx, "memory", dataRoot, "workorch://workspaces/ws-a/runs/run-a/llm-calls/step/request.json"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("memory backend err = %v, want invalid argument", err)
	}
}
