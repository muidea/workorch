package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/muidea/workorch/application/workorchd/appservice"
	approvalmodule "github.com/muidea/workorch/internal/modules/blocks/approval"
	legacycommon "github.com/muidea/workorch/internal/modules/blocks/legacy/pkg/common"
	decisionmodule "github.com/muidea/workorch/internal/modules/kernel/decision"
	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestHealthzReturnsOK(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	newMux(Config{}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body statusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body.Service != "workorch" || body.Status != "ok" {
		t.Fatalf("body = %+v, want workorch/ok", body)
	}
}

func TestReadyzReturnsServiceUnavailableWhenBackendInvalid(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	newMux(Config{Backend: "unsupported"}).ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestRunEndpointReturnsSlicesWithMemoryBackend(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/runs", nil)
	rec := httptest.NewRecorder()

	newMux(Config{Backend: "memory"}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var body runResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body.Service != "workorch" {
		t.Fatalf("service = %q, want workorch", body.Service)
	}
	if len(body.Slices) != 17 {
		t.Fatalf("len(slices) = %d, want 17", len(body.Slices))
	}
	if body.Slices[0].Title != "workorch runnable slice" {
		t.Fatalf("first title = %q, want workorch runnable slice", body.Slices[0].Title)
	}
}

func TestRunEndpointPersistsFilesWithFileBackend(t *testing.T) {
	dataRoot := t.TempDir()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/runs", nil)
	rec := httptest.NewRecorder()

	newMux(Config{Backend: "file", DataRoot: dataRoot}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	for _, name := range []string{
		"workspace.json",
		"run.json",
		"step.json",
		"ticket.json",
		"resolution.json",
		"approval.json",
	} {
		if _, err := os.Stat(filepath.Join(dataRoot, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
}

func TestWorkspaceCreateEndpointReturnsCreatedWorkspace(t *testing.T) {
	handler := newMux(Config{Backend: "memory"})

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces", strings.NewReader(`{"workspaceId":"ws-http","title":"http workspace"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201, body=%s", createRec.Code, createRec.Body.String())
	}
	var created workspaceResponse
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal created workspace: %v", err)
	}
	if created.Workspace.ID != "ws-http" || created.Workspace.Title != "http workspace" || created.Workspace.State != "active" {
		t.Fatalf("created workspace = %+v, want ws-http/http workspace/active", created.Workspace)
	}
}

func TestWorkspaceListAndGetEndpointsReturnPersistedWorkspaces(t *testing.T) {
	handler := newMux(Config{Backend: "file", DataRoot: t.TempDir()})

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces", strings.NewReader(`{"workspaceId":"ws-http","title":"http workspace"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201, body=%s", createRec.Code, createRec.Body.String())
	}
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces", nil)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200, body=%s", listRec.Code, listRec.Body.String())
	}
	var listed workspacesResponse
	if err := json.Unmarshal(listRec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("unmarshal listed workspaces: %v", err)
	}
	if len(listed.Workspaces) != 1 || listed.Workspaces[0].ID != "ws-http" {
		t.Fatalf("listed workspaces = %+v, want ws-http", listed.Workspaces)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces/ws-http", nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want 200, body=%s", getRec.Code, getRec.Body.String())
	}
	var fetched workspaceResponse
	if err := json.Unmarshal(getRec.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("unmarshal fetched workspace: %v", err)
	}
	if fetched.Workspace.ID != "ws-http" || fetched.Workspace.Title != "http workspace" {
		t.Fatalf("fetched workspace = %+v, want ws-http/http workspace", fetched.Workspace)
	}
}

func TestWorkspaceCreateEndpointReturnsConflictForDuplicateID(t *testing.T) {
	handler := newMux(Config{Backend: "file", DataRoot: t.TempDir()})
	for idx := 0; idx < 2; idx++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces", strings.NewReader(`{"workspaceId":"ws-dup","title":"duplicate workspace"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if idx == 0 && rec.Code != http.StatusCreated {
			t.Fatalf("first create status = %d, want 201, body=%s", rec.Code, rec.Body.String())
		}
		if idx == 1 && rec.Code != http.StatusConflict {
			t.Fatalf("second create status = %d, want 409, body=%s", rec.Code, rec.Body.String())
		}
	}
}

func TestWorkspaceCreateEndpointReturnsBadRequestForInvalidBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces", strings.NewReader(`{"workspaceId":"","title":""}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	newMux(Config{Backend: "memory"}).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
}

func TestWorkspaceGetEndpointReturnsNotFoundWhenMissing(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces/ws-missing", nil)
	rec := httptest.NewRecorder()

	newMux(Config{Backend: "file", DataRoot: t.TempDir()}).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body.String())
	}
}

func TestRunCreateEndpointReturnsCreatedRun(t *testing.T) {
	handler := newMux(Config{Backend: "file", DataRoot: t.TempDir()})

	workspaceReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces", strings.NewReader(`{"workspaceId":"ws-run-http","title":"run http workspace"}`))
	workspaceReq.Header.Set("Content-Type", "application/json")
	workspaceRec := httptest.NewRecorder()
	handler.ServeHTTP(workspaceRec, workspaceReq)
	if workspaceRec.Code != http.StatusCreated {
		t.Fatalf("workspace create status = %d, want 201, body=%s", workspaceRec.Code, workspaceRec.Body.String())
	}

	runReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/ws-run-http/runs", strings.NewReader(`{"runId":"run-http","goalId":"goal-http","doDId":"dod-http"}`))
	runReq.Header.Set("Content-Type", "application/json")
	runRec := httptest.NewRecorder()
	handler.ServeHTTP(runRec, runReq)

	if runRec.Code != http.StatusCreated {
		t.Fatalf("run create status = %d, want 201, body=%s", runRec.Code, runRec.Body.String())
	}
	var body runInstanceResponse
	if err := json.Unmarshal(runRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal created run: %v", err)
	}
	if body.Run.ID != "run-http" || body.Run.WorkspaceID != "ws-run-http" || body.Run.State != "created" {
		t.Fatalf("created run = %+v, want run-http/ws-run-http/created", body.Run)
	}
	if body.Run.GoalID != "goal-http" || body.Run.DoDID != "dod-http" {
		t.Fatalf("created run goal/dod = %+v, want goal-http/dod-http", body.Run)
	}
}

func TestRunListAndGetEndpointsReturnPersistedRuns(t *testing.T) {
	handler := newMux(Config{Backend: "file", DataRoot: t.TempDir()})

	for _, payload := range []string{
		`{"workspaceId":"ws-run-a","title":"run workspace a"}`,
		`{"workspaceId":"ws-run-b","title":"run workspace b"}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces", strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("workspace create status = %d, want 201, body=%s", rec.Code, rec.Body.String())
		}
	}

	for _, tc := range []struct {
		path string
		body string
	}{
		{path: "/api/v1/workorch/workspaces/ws-run-b/runs", body: `{"runId":"run-b"}`},
		{path: "/api/v1/workorch/workspaces/ws-run-a/runs", body: `{"runId":"run-a"}`},
	} {
		req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("run create status = %d, want 201, body=%s", rec.Code, rec.Body.String())
		}
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/runs", nil)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("run list status = %d, want 200, body=%s", listRec.Code, listRec.Body.String())
	}
	var listed runsResponse
	if err := json.Unmarshal(listRec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("unmarshal run list: %v", err)
	}
	if len(listed.Runs) != 2 || listed.Runs[0].ID != "run-a" || listed.Runs[1].ID != "run-b" {
		t.Fatalf("listed runs = %+v, want run-a/run-b", listed.Runs)
	}

	filteredReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/runs?workspaceId=ws-run-b", nil)
	filteredRec := httptest.NewRecorder()
	handler.ServeHTTP(filteredRec, filteredReq)
	if filteredRec.Code != http.StatusOK {
		t.Fatalf("filtered run list status = %d, want 200, body=%s", filteredRec.Code, filteredRec.Body.String())
	}
	var filtered runsResponse
	if err := json.Unmarshal(filteredRec.Body.Bytes(), &filtered); err != nil {
		t.Fatalf("unmarshal filtered runs: %v", err)
	}
	if len(filtered.Runs) != 1 || filtered.Runs[0].ID != "run-b" {
		t.Fatalf("filtered runs = %+v, want run-b", filtered.Runs)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/runs/run-a", nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("run get status = %d, want 200, body=%s", getRec.Code, getRec.Body.String())
	}
	var fetched runInstanceResponse
	if err := json.Unmarshal(getRec.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("unmarshal run get: %v", err)
	}
	if fetched.Run.ID != "run-a" || fetched.Run.WorkspaceID != "ws-run-a" {
		t.Fatalf("fetched run = %+v, want run-a/ws-run-a", fetched.Run)
	}
}

func TestRunSnapshotEndpointReturnsPersistedRunSnapshot(t *testing.T) {
	handler := newMux(Config{Backend: "file", DataRoot: t.TempDir()})
	runID := "run-" + legacycommon.RuntimeDecisionBlockRunToken + "-snapshot-http"

	createWorkspaceAndRunForHTTP(t, handler, "ws-run-snapshot-http", "run snapshot http workspace", runID)
	started := startRunForHTTP(t, handler, runID)

	snapshotReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/runs/"+runID+"/snapshot", nil)
	snapshotRec := httptest.NewRecorder()
	handler.ServeHTTP(snapshotRec, snapshotReq)

	if snapshotRec.Code != http.StatusOK {
		t.Fatalf("snapshot status = %d, want 200, body=%s", snapshotRec.Code, snapshotRec.Body.String())
	}
	var body runSnapshotResponse
	if err := json.Unmarshal(snapshotRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal run snapshot: %v", err)
	}
	if body.Snapshot.Workspace.ID != "ws-run-snapshot-http" {
		t.Fatalf("workspace = %+v, want ws-run-snapshot-http", body.Snapshot.Workspace)
	}
	if body.Snapshot.Run.ID != runID || body.Snapshot.Run.State != "blocked_on_decision" {
		t.Fatalf("run = %+v, want blocked decision run", body.Snapshot.Run)
	}
	if body.Snapshot.Step == nil || body.Snapshot.Step.ID == "" {
		t.Fatalf("step = %+v, want current step", body.Snapshot.Step)
	}
	if len(body.Snapshot.Steps) == 0 {
		t.Fatalf("steps = %+v, want non-empty", body.Snapshot.Steps)
	}
	if len(body.Snapshot.Decisions) != 1 || body.Snapshot.Decisions[0].ID != started.BlockingTicketID {
		t.Fatalf("decisions = %+v, want decision %s", body.Snapshot.Decisions, started.BlockingTicketID)
	}
}

func TestRunStatusEndpointReturnsPersistedRunStatus(t *testing.T) {
	handler := newMux(Config{Backend: "file", DataRoot: t.TempDir()})
	runID := "run-" + legacycommon.RuntimeApprovalBlockRunToken + "-status-http"

	createWorkspaceAndRunForHTTP(t, handler, "ws-run-status-http", "run status http workspace", runID)
	started := startRunForHTTP(t, handler, runID)

	statusReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/runs/"+runID+"/status", nil)
	statusRec := httptest.NewRecorder()
	handler.ServeHTTP(statusRec, statusReq)

	if statusRec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want 200, body=%s", statusRec.Code, statusRec.Body.String())
	}
	var body runStatusResponse
	if err := json.Unmarshal(statusRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal run status: %v", err)
	}
	if body.Status.ID != runID || body.Status.WorkspaceID != "ws-run-status-http" || body.Status.State != "blocked_on_approval" {
		t.Fatalf("status = %+v, want blocked approval status", body.Status)
	}
	if body.Status.StepCount == 0 || body.Status.CurrentStepID == "" || body.Status.CurrentStepState == "" {
		t.Fatalf("status = %+v, want populated step summary", body.Status)
	}
	if body.Status.ApprovalCount != 1 || body.Status.BlockingApprovalID != started.BlockingApprovalID {
		t.Fatalf("status = %+v, want approval count 1 and blocking approval %s", body.Status, started.BlockingApprovalID)
	}
}

func TestWorkspaceSnapshotEndpointReturnsRelatedRunsAndBlockingItems(t *testing.T) {
	handler := newMux(Config{Backend: "file", DataRoot: t.TempDir()})

	workspaceReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces", strings.NewReader(`{"workspaceId":"ws-workspace-snapshot-http","title":"workspace snapshot http"}`))
	workspaceReq.Header.Set("Content-Type", "application/json")
	workspaceRec := httptest.NewRecorder()
	handler.ServeHTTP(workspaceRec, workspaceReq)
	if workspaceRec.Code != http.StatusCreated {
		t.Fatalf("workspace create status = %d, want 201, body=%s", workspaceRec.Code, workspaceRec.Body.String())
	}

	for _, runID := range []string{
		"run-workspace-snapshot-http-completed",
		"run-" + legacycommon.RuntimeDecisionBlockRunToken + "-workspace-snapshot-http",
		"run-" + legacycommon.RuntimeApprovalBlockRunToken + "-workspace-snapshot-http",
	} {
		runReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/ws-workspace-snapshot-http/runs", strings.NewReader(`{"runId":"`+runID+`"}`))
		runReq.Header.Set("Content-Type", "application/json")
		runRec := httptest.NewRecorder()
		handler.ServeHTTP(runRec, runReq)
		if runRec.Code != http.StatusCreated {
			t.Fatalf("run create status = %d, want 201, body=%s", runRec.Code, runRec.Body.String())
		}
	}

	completed := startRunForHTTP(t, handler, "run-workspace-snapshot-http-completed")
	if completed.State != "completed" {
		t.Fatalf("completed run = %+v, want completed", completed)
	}
	decisionRun := startRunForHTTP(t, handler, "run-"+legacycommon.RuntimeDecisionBlockRunToken+"-workspace-snapshot-http")
	resolveDecisionReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/decisions/"+decisionRun.BlockingTicketID+"/resolve", nil)
	resolveDecisionRec := httptest.NewRecorder()
	handler.ServeHTTP(resolveDecisionRec, resolveDecisionReq)
	if resolveDecisionRec.Code != http.StatusOK {
		t.Fatalf("decision resolve status = %d, want 200, body=%s", resolveDecisionRec.Code, resolveDecisionRec.Body.String())
	}
	resumeDecisionReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/runs/"+"run-"+legacycommon.RuntimeDecisionBlockRunToken+"-workspace-snapshot-http"+"/resume", nil)
	resumeDecisionRec := httptest.NewRecorder()
	handler.ServeHTTP(resumeDecisionRec, resumeDecisionReq)
	if resumeDecisionRec.Code != http.StatusOK {
		t.Fatalf("decision run resume status = %d, want 200, body=%s", resumeDecisionRec.Code, resumeDecisionRec.Body.String())
	}
	approvalRun := startRunForHTTP(t, handler, "run-"+legacycommon.RuntimeApprovalBlockRunToken+"-workspace-snapshot-http")

	snapshotReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/workspaces/ws-workspace-snapshot-http/snapshot", nil)
	snapshotRec := httptest.NewRecorder()
	handler.ServeHTTP(snapshotRec, snapshotReq)

	if snapshotRec.Code != http.StatusOK {
		t.Fatalf("workspace snapshot status = %d, want 200, body=%s", snapshotRec.Code, snapshotRec.Body.String())
	}
	var body workspaceSnapshotResponse
	if err := json.Unmarshal(snapshotRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal workspace snapshot: %v", err)
	}
	if body.Snapshot.Workspace.ID != "ws-workspace-snapshot-http" {
		t.Fatalf("workspace = %+v, want ws-workspace-snapshot-http", body.Snapshot.Workspace)
	}
	if len(body.Snapshot.Runs) != 3 {
		t.Fatalf("runs = %+v, want 3 runs", body.Snapshot.Runs)
	}
	if len(body.Snapshot.Decisions) != 1 || body.Snapshot.Decisions[0].ID != decisionRun.BlockingTicketID {
		t.Fatalf("decisions = %+v, want decision %s", body.Snapshot.Decisions, decisionRun.BlockingTicketID)
	}
	if len(body.Snapshot.Approvals) != 1 || body.Snapshot.Approvals[0].ID != approvalRun.BlockingApprovalID {
		t.Fatalf("approvals = %+v, want approval %s", body.Snapshot.Approvals, approvalRun.BlockingApprovalID)
	}
}

func TestRunCreateEndpointReturnsConflictForDuplicateID(t *testing.T) {
	handler := newMux(Config{Backend: "file", DataRoot: t.TempDir()})

	workspaceReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces", strings.NewReader(`{"workspaceId":"ws-run-dup","title":"run dup workspace"}`))
	workspaceReq.Header.Set("Content-Type", "application/json")
	workspaceRec := httptest.NewRecorder()
	handler.ServeHTTP(workspaceRec, workspaceReq)
	if workspaceRec.Code != http.StatusCreated {
		t.Fatalf("workspace create status = %d, want 201, body=%s", workspaceRec.Code, workspaceRec.Body.String())
	}

	for idx := 0; idx < 2; idx++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/ws-run-dup/runs", strings.NewReader(`{"runId":"run-dup"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if idx == 0 && rec.Code != http.StatusCreated {
			t.Fatalf("first run create status = %d, want 201, body=%s", rec.Code, rec.Body.String())
		}
		if idx == 1 && rec.Code != http.StatusConflict {
			t.Fatalf("second run create status = %d, want 409, body=%s", rec.Code, rec.Body.String())
		}
	}
}

func TestRunCreateEndpointReturnsNotFoundWhenWorkspaceMissing(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/ws-run-missing/runs", strings.NewReader(`{"runId":"run-missing"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	newMux(Config{Backend: "file", DataRoot: t.TempDir()}).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body.String())
	}
}

func TestRunGetEndpointReturnsNotFoundWhenMissing(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/runs/run-missing", nil)
	rec := httptest.NewRecorder()

	newMux(Config{Backend: "file", DataRoot: t.TempDir()}).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body.String())
	}
}

func TestRunStartEndpointReturnsCompletedRun(t *testing.T) {
	handler := newMux(Config{Backend: "file", DataRoot: t.TempDir()})

	workspaceReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces", strings.NewReader(`{"workspaceId":"ws-run-start-http","title":"run start http workspace"}`))
	workspaceReq.Header.Set("Content-Type", "application/json")
	workspaceRec := httptest.NewRecorder()
	handler.ServeHTTP(workspaceRec, workspaceReq)
	if workspaceRec.Code != http.StatusCreated {
		t.Fatalf("workspace create status = %d, want 201, body=%s", workspaceRec.Code, workspaceRec.Body.String())
	}

	runReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/ws-run-start-http/runs", strings.NewReader(`{"runId":"run-start-http"}`))
	runReq.Header.Set("Content-Type", "application/json")
	runRec := httptest.NewRecorder()
	handler.ServeHTTP(runRec, runReq)
	if runRec.Code != http.StatusCreated {
		t.Fatalf("run create status = %d, want 201, body=%s", runRec.Code, runRec.Body.String())
	}

	startReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/runs/run-start-http/start", nil)
	startRec := httptest.NewRecorder()
	handler.ServeHTTP(startRec, startReq)

	if startRec.Code != http.StatusOK {
		t.Fatalf("start status = %d, want 200, body=%s", startRec.Code, startRec.Body.String())
	}
	var body runInstanceResponse
	if err := json.Unmarshal(startRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal started run: %v", err)
	}
	if body.Run.ID != "run-start-http" || body.Run.State != "completed" {
		t.Fatalf("started run = %+v, want run-start-http/completed", body.Run)
	}
}

func TestRunStartEndpointReturnsBlockedDecisionRun(t *testing.T) {
	dataRoot := t.TempDir()
	handler := newMux(Config{Backend: "file", DataRoot: dataRoot})
	runID := "run-" + legacycommon.RuntimeDecisionBlockRunToken + "-http"

	createWorkspaceAndRunForHTTP(t, handler, "ws-run-start-blocked-http", "run start blocked http workspace", runID)

	startReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/runs/"+runID+"/start", nil)
	startRec := httptest.NewRecorder()
	handler.ServeHTTP(startRec, startReq)

	if startRec.Code != http.StatusOK {
		t.Fatalf("start status = %d, want 200, body=%s", startRec.Code, startRec.Body.String())
	}
	var body runInstanceResponse
	if err := json.Unmarshal(startRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal blocked run: %v", err)
	}
	if body.Run.State != "blocked_on_decision" || body.Run.BlockingTicketID == "" {
		t.Fatalf("run = %+v, want blocked decision with ticket", body.Run)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/runs/"+runID, nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("run get status = %d, want 200, body=%s", getRec.Code, getRec.Body.String())
	}
}

func TestRunResumeEndpointReturnsCompletedRunAfterDecisionResolved(t *testing.T) {
	dataRoot := t.TempDir()
	handler := newMux(Config{Backend: "file", DataRoot: dataRoot})
	runID := "run-" + legacycommon.RuntimeDecisionBlockRunToken + "-resume-http"

	createWorkspaceAndRunForHTTP(t, handler, "ws-run-resume-decision-http", "run resume decision http workspace", runID)
	started := startRunForHTTP(t, handler, runID)
	if started.State != "blocked_on_decision" || started.BlockingTicketID == "" {
		t.Fatalf("started run = %+v, want blocked decision with ticket", started)
	}

	resolveDecisionForHTTP(t, dataRoot, started.BlockingTicketID, false)

	resumeReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/runs/"+runID+"/resume", nil)
	resumeRec := httptest.NewRecorder()
	handler.ServeHTTP(resumeRec, resumeReq)

	if resumeRec.Code != http.StatusOK {
		t.Fatalf("resume status = %d, want 200, body=%s", resumeRec.Code, resumeRec.Body.String())
	}
	var body runInstanceResponse
	if err := json.Unmarshal(resumeRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal resumed run: %v", err)
	}
	if body.Run.State != "completed" || body.Run.BlockingTicketID != "" {
		t.Fatalf("run = %+v, want completed with cleared ticket", body.Run)
	}
}

func TestRunResumeEndpointReturnsCompletedRunAfterApprovalApproved(t *testing.T) {
	dataRoot := t.TempDir()
	handler := newMux(Config{Backend: "file", DataRoot: dataRoot})
	runID := "run-" + legacycommon.RuntimeApprovalBlockRunToken + "-resume-http"

	createWorkspaceAndRunForHTTP(t, handler, "ws-run-resume-approval-http", "run resume approval http workspace", runID)
	started := startRunForHTTP(t, handler, runID)
	if started.State != "blocked_on_approval" || started.BlockingApprovalID == "" {
		t.Fatalf("started run = %+v, want blocked approval with approval id", started)
	}

	resolveApprovalForHTTP(t, dataRoot, started.BlockingApprovalID, false)

	resumeReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/runs/"+runID+"/resume", nil)
	resumeRec := httptest.NewRecorder()
	handler.ServeHTTP(resumeRec, resumeReq)

	if resumeRec.Code != http.StatusOK {
		t.Fatalf("resume status = %d, want 200, body=%s", resumeRec.Code, resumeRec.Body.String())
	}
	var body runInstanceResponse
	if err := json.Unmarshal(resumeRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal resumed approval run: %v", err)
	}
	if body.Run.State != "completed" || body.Run.BlockingApprovalID != "" {
		t.Fatalf("run = %+v, want completed with cleared approval", body.Run)
	}
}

func TestRunResumeEndpointReturnsFailedRunAfterApprovalRejected(t *testing.T) {
	dataRoot := t.TempDir()
	handler := newMux(Config{Backend: "file", DataRoot: dataRoot})
	runID := "run-" + legacycommon.RuntimeApprovalBlockRunToken + "-rejected-http"

	createWorkspaceAndRunForHTTP(t, handler, "ws-run-resume-approval-rejected-http", "run resume approval rejected http workspace", runID)
	started := startRunForHTTP(t, handler, runID)
	if started.State != "blocked_on_approval" || started.BlockingApprovalID == "" {
		t.Fatalf("started run = %+v, want blocked approval with approval id", started)
	}

	resolveApprovalForHTTP(t, dataRoot, started.BlockingApprovalID, true)

	resumeReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/runs/"+runID+"/resume", nil)
	resumeRec := httptest.NewRecorder()
	handler.ServeHTTP(resumeRec, resumeReq)

	if resumeRec.Code != http.StatusOK {
		t.Fatalf("resume status = %d, want 200, body=%s", resumeRec.Code, resumeRec.Body.String())
	}
	var body runInstanceResponse
	if err := json.Unmarshal(resumeRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal resumed rejected approval run: %v", err)
	}
	if body.Run.State != "failed" || body.Run.BlockingApprovalID != "" || body.Run.FailureReason == "" {
		t.Fatalf("run = %+v, want failed with cleared approval and reason", body.Run)
	}
}

func TestRunResumeEndpointReturnsConflictWhenDependencyStillBlocked(t *testing.T) {
	handler := newMux(Config{Backend: "file", DataRoot: t.TempDir()})
	runID := "run-" + legacycommon.RuntimeApprovalBlockRunToken + "-pending-http"

	createWorkspaceAndRunForHTTP(t, handler, "ws-run-resume-pending-http", "run resume pending http workspace", runID)
	started := startRunForHTTP(t, handler, runID)
	if started.State != "blocked_on_approval" || started.BlockingApprovalID == "" {
		t.Fatalf("started run = %+v, want blocked approval with approval id", started)
	}

	resumeReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/runs/"+runID+"/resume", nil)
	resumeRec := httptest.NewRecorder()
	handler.ServeHTTP(resumeRec, resumeReq)

	if resumeRec.Code != http.StatusConflict {
		t.Fatalf("resume status = %d, want 409, body=%s", resumeRec.Code, resumeRec.Body.String())
	}
}

func TestRunResumeEndpointReturnsConflictForCreatedRun(t *testing.T) {
	handler := newMux(Config{Backend: "file", DataRoot: t.TempDir()})
	createWorkspaceAndRunForHTTP(t, handler, "ws-run-resume-created-http", "run resume created http workspace", "run-resume-created-http")

	resumeReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/runs/run-resume-created-http/resume", nil)
	resumeRec := httptest.NewRecorder()
	handler.ServeHTTP(resumeRec, resumeReq)

	if resumeRec.Code != http.StatusConflict {
		t.Fatalf("resume status = %d, want 409, body=%s", resumeRec.Code, resumeRec.Body.String())
	}
}

func TestDecisionListAndGetEndpointsReturnPersistedTickets(t *testing.T) {
	handler := newMux(Config{Backend: "file", DataRoot: t.TempDir()})
	runA := "run-" + legacycommon.RuntimeDecisionBlockRunToken + "-decision-list-a"
	runB := "run-" + legacycommon.RuntimeDecisionBlockRunToken + "-decision-list-b"

	createWorkspaceAndRunForHTTP(t, handler, "ws-decision-list-http-a", "decision list http workspace a", runA)
	startedA := startRunForHTTP(t, handler, runA)
	createWorkspaceAndRunForHTTP(t, handler, "ws-decision-list-http-b", "decision list http workspace b", runB)
	startedB := startRunForHTTP(t, handler, runB)

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/decisions", nil)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("decision list status = %d, want 200, body=%s", listRec.Code, listRec.Body.String())
	}
	var listed decisionsResponse
	if err := json.Unmarshal(listRec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("unmarshal decision list: %v", err)
	}
	if len(listed.Decisions) != 2 || listed.Decisions[0].ID != startedA.BlockingTicketID || listed.Decisions[1].ID != startedB.BlockingTicketID {
		t.Fatalf("listed decisions = %+v, want sorted tickets", listed.Decisions)
	}

	filteredReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/decisions?workspaceId=ws-decision-list-http-b", nil)
	filteredRec := httptest.NewRecorder()
	handler.ServeHTTP(filteredRec, filteredReq)
	if filteredRec.Code != http.StatusOK {
		t.Fatalf("filtered decision list status = %d, want 200, body=%s", filteredRec.Code, filteredRec.Body.String())
	}
	var filtered decisionsResponse
	if err := json.Unmarshal(filteredRec.Body.Bytes(), &filtered); err != nil {
		t.Fatalf("unmarshal filtered decision list: %v", err)
	}
	if len(filtered.Decisions) != 1 || filtered.Decisions[0].ID != startedB.BlockingTicketID {
		t.Fatalf("filtered decisions = %+v, want ticket %s", filtered.Decisions, startedB.BlockingTicketID)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/decisions/"+startedA.BlockingTicketID, nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("decision get status = %d, want 200, body=%s", getRec.Code, getRec.Body.String())
	}
	var fetched decisionResponse
	if err := json.Unmarshal(getRec.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("unmarshal decision get: %v", err)
	}
	if fetched.Decision.ID != startedA.BlockingTicketID || fetched.Decision.State != "open" {
		t.Fatalf("decision = %+v, want open ticket %s", fetched.Decision, startedA.BlockingTicketID)
	}
}

func TestDecisionResolveEndpointResolvesTicketAndAllowsRunResume(t *testing.T) {
	handler := newMux(Config{Backend: "file", DataRoot: t.TempDir()})
	runID := "run-" + legacycommon.RuntimeDecisionBlockRunToken + "-decision-resolve-http"

	createWorkspaceAndRunForHTTP(t, handler, "ws-decision-resolve-http", "decision resolve http workspace", runID)
	started := startRunForHTTP(t, handler, runID)

	resolveReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/decisions/"+started.BlockingTicketID+"/resolve", nil)
	resolveRec := httptest.NewRecorder()
	handler.ServeHTTP(resolveRec, resolveReq)
	if resolveRec.Code != http.StatusOK {
		t.Fatalf("decision resolve status = %d, want 200, body=%s", resolveRec.Code, resolveRec.Body.String())
	}
	var resolved decisionResponse
	if err := json.Unmarshal(resolveRec.Body.Bytes(), &resolved); err != nil {
		t.Fatalf("unmarshal decision resolve: %v", err)
	}
	if resolved.Decision.State != "resolved" || resolved.Decision.ResolutionType != decisioncommon.ResolutionTypeApprovePath {
		t.Fatalf("decision = %+v, want resolved approve_path", resolved.Decision)
	}

	resumeReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/runs/"+runID+"/resume", nil)
	resumeRec := httptest.NewRecorder()
	handler.ServeHTTP(resumeRec, resumeReq)
	if resumeRec.Code != http.StatusOK {
		t.Fatalf("run resume status = %d, want 200, body=%s", resumeRec.Code, resumeRec.Body.String())
	}
	var resumed runInstanceResponse
	if err := json.Unmarshal(resumeRec.Body.Bytes(), &resumed); err != nil {
		t.Fatalf("unmarshal resumed run: %v", err)
	}
	if resumed.Run.State != "completed" || resumed.Run.BlockingTicketID != "" {
		t.Fatalf("run = %+v, want completed with cleared ticket", resumed.Run)
	}
}

func TestDecisionRejectEndpointRejectsTicketAndRunResumeFailsTerminally(t *testing.T) {
	handler := newMux(Config{Backend: "file", DataRoot: t.TempDir()})
	runID := "run-" + legacycommon.RuntimeDecisionBlockRunToken + "-decision-reject-http"

	createWorkspaceAndRunForHTTP(t, handler, "ws-decision-reject-http", "decision reject http workspace", runID)
	started := startRunForHTTP(t, handler, runID)

	rejectReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/decisions/"+started.BlockingTicketID+"/reject", nil)
	rejectRec := httptest.NewRecorder()
	handler.ServeHTTP(rejectRec, rejectReq)
	if rejectRec.Code != http.StatusOK {
		t.Fatalf("decision reject status = %d, want 200, body=%s", rejectRec.Code, rejectRec.Body.String())
	}
	var rejected decisionResponse
	if err := json.Unmarshal(rejectRec.Body.Bytes(), &rejected); err != nil {
		t.Fatalf("unmarshal decision reject: %v", err)
	}
	if rejected.Decision.State != "resolved" || rejected.Decision.ResolutionType != decisioncommon.ResolutionTypeRejectPath {
		t.Fatalf("decision = %+v, want resolved reject_path", rejected.Decision)
	}

	resumeReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/runs/"+runID+"/resume", nil)
	resumeRec := httptest.NewRecorder()
	handler.ServeHTTP(resumeRec, resumeReq)
	if resumeRec.Code != http.StatusOK {
		t.Fatalf("run resume status = %d, want 200, body=%s", resumeRec.Code, resumeRec.Body.String())
	}
	var resumed runInstanceResponse
	if err := json.Unmarshal(resumeRec.Body.Bytes(), &resumed); err != nil {
		t.Fatalf("unmarshal resumed rejected run: %v", err)
	}
	if resumed.Run.State != "failed" || resumed.Run.FailureReason == "" {
		t.Fatalf("run = %+v, want failed with reason", resumed.Run)
	}
}

func TestDecisionResolveEndpointReturnsConflictForOppositeResolution(t *testing.T) {
	handler := newMux(Config{Backend: "file", DataRoot: t.TempDir()})
	runID := "run-" + legacycommon.RuntimeDecisionBlockRunToken + "-decision-conflict-http"

	createWorkspaceAndRunForHTTP(t, handler, "ws-decision-conflict-http", "decision conflict http workspace", runID)
	started := startRunForHTTP(t, handler, runID)
	rejectDecisionViaHTTP(t, handler, started.BlockingTicketID)

	resolveReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/decisions/"+started.BlockingTicketID+"/resolve", nil)
	resolveRec := httptest.NewRecorder()
	handler.ServeHTTP(resolveRec, resolveReq)
	if resolveRec.Code != http.StatusConflict {
		t.Fatalf("decision resolve status = %d, want 409, body=%s", resolveRec.Code, resolveRec.Body.String())
	}
}

func TestApprovalListAndGetEndpointsReturnPersistedApprovals(t *testing.T) {
	handler := newMux(Config{Backend: "file", DataRoot: t.TempDir()})
	runA := "run-" + legacycommon.RuntimeApprovalBlockRunToken + "-approval-list-a"
	runB := "run-" + legacycommon.RuntimeApprovalBlockRunToken + "-approval-list-b"

	createWorkspaceAndRunForHTTP(t, handler, "ws-approval-list-http-a", "approval list http workspace a", runA)
	startedA := startRunForHTTP(t, handler, runA)
	createWorkspaceAndRunForHTTP(t, handler, "ws-approval-list-http-b", "approval list http workspace b", runB)
	startedB := startRunForHTTP(t, handler, runB)

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/approvals", nil)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("approval list status = %d, want 200, body=%s", listRec.Code, listRec.Body.String())
	}
	var listed approvalsResponse
	if err := json.Unmarshal(listRec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("unmarshal approval list: %v", err)
	}
	if len(listed.Approvals) != 2 {
		t.Fatalf("len(approvals) = %d, want 2", len(listed.Approvals))
	}
	foundA := false
	foundB := false
	for _, item := range listed.Approvals {
		if item.ID == startedA.BlockingApprovalID {
			foundA = true
		}
		if item.ID == startedB.BlockingApprovalID {
			foundB = true
		}
	}
	if !foundA || !foundB {
		t.Fatalf("listed approvals = %+v, want approvals %s and %s", listed.Approvals, startedA.BlockingApprovalID, startedB.BlockingApprovalID)
	}

	filteredReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/approvals?workspaceId=ws-approval-list-http-b", nil)
	filteredRec := httptest.NewRecorder()
	handler.ServeHTTP(filteredRec, filteredReq)
	if filteredRec.Code != http.StatusOK {
		t.Fatalf("filtered approval list status = %d, want 200, body=%s", filteredRec.Code, filteredRec.Body.String())
	}
	var filtered approvalsResponse
	if err := json.Unmarshal(filteredRec.Body.Bytes(), &filtered); err != nil {
		t.Fatalf("unmarshal filtered approval list: %v", err)
	}
	if len(filtered.Approvals) != 1 || filtered.Approvals[0].ID != startedB.BlockingApprovalID {
		t.Fatalf("filtered approvals = %+v, want approval %s", filtered.Approvals, startedB.BlockingApprovalID)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/approvals/"+startedA.BlockingApprovalID, nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("approval get status = %d, want 200, body=%s", getRec.Code, getRec.Body.String())
	}
	var fetched approvalResponse
	if err := json.Unmarshal(getRec.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("unmarshal approval get: %v", err)
	}
	if fetched.Approval.ID != startedA.BlockingApprovalID || fetched.Approval.State != "pending" {
		t.Fatalf("approval = %+v, want pending approval %s", fetched.Approval, startedA.BlockingApprovalID)
	}
}

func TestApprovalApproveEndpointApprovesApprovalAndAllowsRunResume(t *testing.T) {
	handler := newMux(Config{Backend: "file", DataRoot: t.TempDir()})
	runID := "run-" + legacycommon.RuntimeApprovalBlockRunToken + "-approval-approve-http"

	createWorkspaceAndRunForHTTP(t, handler, "ws-approval-approve-http", "approval approve http workspace", runID)
	started := startRunForHTTP(t, handler, runID)

	approveReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/approvals/"+started.BlockingApprovalID+"/approve", nil)
	approveRec := httptest.NewRecorder()
	handler.ServeHTTP(approveRec, approveReq)
	if approveRec.Code != http.StatusOK {
		t.Fatalf("approval approve status = %d, want 200, body=%s", approveRec.Code, approveRec.Body.String())
	}
	var approved approvalResponse
	if err := json.Unmarshal(approveRec.Body.Bytes(), &approved); err != nil {
		t.Fatalf("unmarshal approval approve: %v", err)
	}
	if approved.Approval.State != "approved" {
		t.Fatalf("approval = %+v, want approved", approved.Approval)
	}

	resumeReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/runs/"+runID+"/resume", nil)
	resumeRec := httptest.NewRecorder()
	handler.ServeHTTP(resumeRec, resumeReq)
	if resumeRec.Code != http.StatusOK {
		t.Fatalf("run resume status = %d, want 200, body=%s", resumeRec.Code, resumeRec.Body.String())
	}
	var resumed runInstanceResponse
	if err := json.Unmarshal(resumeRec.Body.Bytes(), &resumed); err != nil {
		t.Fatalf("unmarshal resumed approval run: %v", err)
	}
	if resumed.Run.State != "completed" || resumed.Run.BlockingApprovalID != "" {
		t.Fatalf("run = %+v, want completed with cleared approval", resumed.Run)
	}
}

func TestApprovalRejectEndpointRejectsApprovalAndRunResumeFailsTerminally(t *testing.T) {
	handler := newMux(Config{Backend: "file", DataRoot: t.TempDir()})
	runID := "run-" + legacycommon.RuntimeApprovalBlockRunToken + "-approval-reject-http"

	createWorkspaceAndRunForHTTP(t, handler, "ws-approval-reject-http", "approval reject http workspace", runID)
	started := startRunForHTTP(t, handler, runID)

	rejectReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/approvals/"+started.BlockingApprovalID+"/reject", nil)
	rejectRec := httptest.NewRecorder()
	handler.ServeHTTP(rejectRec, rejectReq)
	if rejectRec.Code != http.StatusOK {
		t.Fatalf("approval reject status = %d, want 200, body=%s", rejectRec.Code, rejectRec.Body.String())
	}
	var rejected approvalResponse
	if err := json.Unmarshal(rejectRec.Body.Bytes(), &rejected); err != nil {
		t.Fatalf("unmarshal approval reject: %v", err)
	}
	if rejected.Approval.State != "rejected" {
		t.Fatalf("approval = %+v, want rejected", rejected.Approval)
	}

	resumeReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/runs/"+runID+"/resume", nil)
	resumeRec := httptest.NewRecorder()
	handler.ServeHTTP(resumeRec, resumeReq)
	if resumeRec.Code != http.StatusOK {
		t.Fatalf("run resume status = %d, want 200, body=%s", resumeRec.Code, resumeRec.Body.String())
	}
	var resumed runInstanceResponse
	if err := json.Unmarshal(resumeRec.Body.Bytes(), &resumed); err != nil {
		t.Fatalf("unmarshal resumed rejected approval run: %v", err)
	}
	if resumed.Run.State != "failed" || resumed.Run.FailureReason == "" {
		t.Fatalf("run = %+v, want failed with reason", resumed.Run)
	}
}

func TestApprovalApproveEndpointReturnsConflictForOppositeResolution(t *testing.T) {
	handler := newMux(Config{Backend: "file", DataRoot: t.TempDir()})
	runID := "run-" + legacycommon.RuntimeApprovalBlockRunToken + "-approval-conflict-http"

	createWorkspaceAndRunForHTTP(t, handler, "ws-approval-conflict-http", "approval conflict http workspace", runID)
	started := startRunForHTTP(t, handler, runID)

	rejectReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/approvals/"+started.BlockingApprovalID+"/reject", nil)
	rejectRec := httptest.NewRecorder()
	handler.ServeHTTP(rejectRec, rejectReq)
	if rejectRec.Code != http.StatusOK {
		t.Fatalf("approval reject status = %d, want 200, body=%s", rejectRec.Code, rejectRec.Body.String())
	}

	approveReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/approvals/"+started.BlockingApprovalID+"/approve", nil)
	approveRec := httptest.NewRecorder()
	handler.ServeHTTP(approveRec, approveReq)
	if approveRec.Code != http.StatusConflict {
		t.Fatalf("approval approve status = %d, want 409, body=%s", approveRec.Code, approveRec.Body.String())
	}
}

func TestSliceCatalogEndpointReturnsAvailableSlices(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/slices", nil)
	rec := httptest.NewRecorder()

	newMux(Config{Backend: "memory"}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var body sliceCatalogResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body.Service != "workorch" {
		t.Fatalf("service = %q, want workorch", body.Service)
	}
	if len(body.Slices) != 17 {
		t.Fatalf("len(slices) = %d, want 17", len(body.Slices))
	}
	if body.Slices[0].ID != "workorch-runnable" || body.Slices[0].Title != "workorch runnable slice" {
		t.Fatalf("first slice = %+v, want workorch-runnable/workorch runnable slice", body.Slices[0])
	}
}

func TestSliceStatusesEndpointReturnsPersistedStatuses(t *testing.T) {
	dataRoot := t.TempDir()
	handler := newMux(Config{Backend: "file", DataRoot: dataRoot})

	runReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/slices/workorch-runnable/runs", nil)
	runRec := httptest.NewRecorder()
	handler.ServeHTTP(runRec, runReq)
	if runRec.Code != http.StatusOK {
		t.Fatalf("run status = %d, want 200, body=%s", runRec.Code, runRec.Body.String())
	}

	statusesReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/slices/statuses", nil)
	statusesRec := httptest.NewRecorder()
	handler.ServeHTTP(statusesRec, statusesReq)

	if statusesRec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", statusesRec.Code, statusesRec.Body.String())
	}
	var body sliceStatusesResponse
	if err := json.Unmarshal(statusesRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if len(body.Statuses) != 1 {
		t.Fatalf("len(statuses) = %d, want 1", len(body.Statuses))
	}
	if body.Statuses[0].ID != "workorch-runnable" || body.Statuses[0].RunState != "completed" {
		t.Fatalf("status = %+v, want completed runnable summary", body.Statuses[0])
	}
}

func TestSliceSnapshotsEndpointReturnsPersistedSnapshots(t *testing.T) {
	dataRoot := t.TempDir()
	handler := newMux(Config{Backend: "file", DataRoot: dataRoot})

	runReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/slices/workorch-runnable/runs", nil)
	runRec := httptest.NewRecorder()
	handler.ServeHTTP(runRec, runReq)
	if runRec.Code != http.StatusOK {
		t.Fatalf("run status = %d, want 200, body=%s", runRec.Code, runRec.Body.String())
	}

	snapshotsReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/slices/snapshots", nil)
	snapshotsRec := httptest.NewRecorder()
	handler.ServeHTTP(snapshotsRec, snapshotsReq)

	if snapshotsRec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", snapshotsRec.Code, snapshotsRec.Body.String())
	}
	var body sliceSnapshotsResponse
	if err := json.Unmarshal(snapshotsRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if len(body.Slices) != 1 {
		t.Fatalf("len(slices) = %d, want 1", len(body.Slices))
	}
	if body.Slices[0].ID != "workorch-runnable" || body.Slices[0].Snapshot == nil || body.Slices[0].Snapshot.Slice != "runnable" {
		t.Fatalf("slice = %+v, want runnable snapshot", body.Slices[0])
	}
}

func TestSliceSnapshotsEndpointReturnsEmptyWhenNoPersistedSnapshots(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/slices/snapshots", nil)
	rec := httptest.NewRecorder()

	newMux(Config{Backend: "file", DataRoot: t.TempDir()}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var body sliceSnapshotsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if len(body.Slices) != 0 {
		t.Fatalf("len(slices) = %d, want 0", len(body.Slices))
	}
}

func TestSliceStatusesEndpointReturnsEmptyWhenNoPersistedStatuses(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/slices/statuses", nil)
	rec := httptest.NewRecorder()

	newMux(Config{Backend: "file", DataRoot: t.TempDir()}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var body sliceStatusesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if len(body.Statuses) != 0 {
		t.Fatalf("len(statuses) = %d, want 0", len(body.Statuses))
	}
}

func TestSingleSliceRunEndpointReturnsRequestedSlice(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/slices/workorch-runnable/runs", nil)
	rec := httptest.NewRecorder()

	newMux(Config{Backend: "memory"}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var body sliceRunResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body.Service != "workorch" {
		t.Fatalf("service = %q, want workorch", body.Service)
	}
	if body.Slice.ID != "workorch-runnable" || body.Slice.Title != "workorch runnable slice" {
		t.Fatalf("slice = %+v, want workorch-runnable/workorch runnable slice", body.Slice)
	}
	if body.Slice.Snapshot == nil || body.Slice.Snapshot.Slice != "runnable" {
		t.Fatalf("snapshot = %+v, want runnable", body.Slice.Snapshot)
	}
}

func TestSingleSliceSnapshotEndpointReturnsPersistedSlice(t *testing.T) {
	dataRoot := t.TempDir()
	handler := newMux(Config{Backend: "file", DataRoot: dataRoot})

	runReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/slices/workorch-runnable/runs", nil)
	runRec := httptest.NewRecorder()
	handler.ServeHTTP(runRec, runReq)
	if runRec.Code != http.StatusOK {
		t.Fatalf("run status = %d, want 200, body=%s", runRec.Code, runRec.Body.String())
	}

	snapshotReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/slices/workorch-runnable/snapshot", nil)
	snapshotRec := httptest.NewRecorder()
	handler.ServeHTTP(snapshotRec, snapshotReq)

	if snapshotRec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", snapshotRec.Code, snapshotRec.Body.String())
	}
	var body sliceRunResponse
	if err := json.Unmarshal(snapshotRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body.Slice.ID != "workorch-runnable" || body.Slice.Title != "workorch runnable slice" {
		t.Fatalf("slice = %+v, want workorch-runnable/workorch runnable slice", body.Slice)
	}
	if body.Slice.Snapshot == nil || body.Slice.Snapshot.Slice != "runnable" {
		t.Fatalf("snapshot = %+v, want runnable", body.Slice.Snapshot)
	}
}

func TestSingleSliceStatusEndpointReturnsPersistedSummary(t *testing.T) {
	dataRoot := t.TempDir()
	handler := newMux(Config{Backend: "file", DataRoot: dataRoot})

	runReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/slices/workorch-runnable/runs", nil)
	runRec := httptest.NewRecorder()
	handler.ServeHTTP(runRec, runReq)
	if runRec.Code != http.StatusOK {
		t.Fatalf("run status = %d, want 200, body=%s", runRec.Code, runRec.Body.String())
	}

	statusReq := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/slices/workorch-runnable/status", nil)
	statusRec := httptest.NewRecorder()
	handler.ServeHTTP(statusRec, statusReq)

	if statusRec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", statusRec.Code, statusRec.Body.String())
	}
	var body sliceStatusResponse
	if err := json.Unmarshal(statusRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body.Status.ID != "workorch-runnable" || body.Status.RunState != "completed" || body.Status.StepState != "completed" {
		t.Fatalf("status = %+v, want completed runnable summary", body.Status)
	}
}

func TestSingleSliceSnapshotEndpointReturnsNotFoundWithoutPersistedState(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/slices/workorch-runnable/snapshot", nil)
	rec := httptest.NewRecorder()

	newMux(Config{Backend: "file", DataRoot: t.TempDir()}).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body.String())
	}
}

func TestSingleSliceStatusEndpointReturnsNotFoundWithoutPersistedState(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workorch/slices/workorch-runnable/status", nil)
	rec := httptest.NewRecorder()

	newMux(Config{Backend: "file", DataRoot: t.TempDir()}).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body.String())
	}
}

func TestSingleSliceRunEndpointReturnsNotFoundForUnknownSlice(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/slices/workorch-missing/runs", nil)
	rec := httptest.NewRecorder()

	newMux(Config{Backend: "memory"}).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body.String())
	}
}

func TestMetricsEndpointReportsCounters(t *testing.T) {
	dataRoot := t.TempDir()
	handler := newMux(Config{Backend: "file", DataRoot: dataRoot})

	workspaceCreateReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces", strings.NewReader(`{"workspaceId":"ws-metrics","title":"metrics workspace"}`))
	workspaceCreateReq.Header.Set("Content-Type", "application/json")
	workspaceCreateRec := httptest.NewRecorder()
	handler.ServeHTTP(workspaceCreateRec, workspaceCreateReq)
	if workspaceCreateRec.Code != http.StatusCreated {
		t.Fatalf("workspace create status = %d, want 201", workspaceCreateRec.Code)
	}
	runCreateReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/ws-metrics/runs", strings.NewReader(`{"runId":"run-metrics"}`))
	runCreateReq.Header.Set("Content-Type", "application/json")
	runCreateRec := httptest.NewRecorder()
	handler.ServeHTTP(runCreateRec, runCreateReq)
	if runCreateRec.Code != http.StatusCreated {
		t.Fatalf("run create status = %d, want 201", runCreateRec.Code)
	}
	startReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/runs/run-metrics/start", nil)
	startRec := httptest.NewRecorder()
	handler.ServeHTTP(startRec, startReq)
	if startRec.Code != http.StatusOK {
		t.Fatalf("run start status = %d, want 200", startRec.Code)
	}

	workspaceBlockedReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces", strings.NewReader(`{"workspaceId":"ws-metrics-blocked","title":"metrics blocked workspace"}`))
	workspaceBlockedReq.Header.Set("Content-Type", "application/json")
	workspaceBlockedRec := httptest.NewRecorder()
	handler.ServeHTTP(workspaceBlockedRec, workspaceBlockedReq)
	if workspaceBlockedRec.Code != http.StatusCreated {
		t.Fatalf("blocked workspace create status = %d, want 201", workspaceBlockedRec.Code)
	}
	blockedRunID := "run-" + legacycommon.RuntimeDecisionBlockRunToken + "-metrics"
	blockedRunReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/ws-metrics-blocked/runs", strings.NewReader(`{"runId":"`+blockedRunID+`"}`))
	blockedRunReq.Header.Set("Content-Type", "application/json")
	blockedRunRec := httptest.NewRecorder()
	handler.ServeHTTP(blockedRunRec, blockedRunReq)
	if blockedRunRec.Code != http.StatusCreated {
		t.Fatalf("blocked run create status = %d, want 201", blockedRunRec.Code)
	}
	blockedStartReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/runs/"+blockedRunID+"/start", nil)
	blockedStartRec := httptest.NewRecorder()
	handler.ServeHTTP(blockedStartRec, blockedStartReq)
	if blockedStartRec.Code != http.StatusOK {
		t.Fatalf("blocked run start status = %d, want 200", blockedStartRec.Code)
	}
	var blockedBody runInstanceResponse
	if err := json.Unmarshal(blockedStartRec.Body.Bytes(), &blockedBody); err != nil {
		t.Fatalf("unmarshal blocked metrics run: %v", err)
	}
	resolveDecisionReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/decisions/"+blockedBody.Run.BlockingTicketID+"/resolve", nil)
	resolveDecisionRec := httptest.NewRecorder()
	handler.ServeHTTP(resolveDecisionRec, resolveDecisionReq)
	if resolveDecisionRec.Code != http.StatusOK {
		t.Fatalf("decision resolve status = %d, want 200", resolveDecisionRec.Code)
	}
	resumeReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/runs/"+blockedRunID+"/resume", nil)
	resumeRec := httptest.NewRecorder()
	handler.ServeHTTP(resumeRec, resumeReq)
	if resumeRec.Code != http.StatusOK {
		t.Fatalf("run resume status = %d, want 200", resumeRec.Code)
	}

	for _, path := range []string{"/healthz", "/readyz", "/api/v1/workorch/workspaces", "/api/v1/workorch/workspaces/ws-metrics", "/api/v1/workorch/runs", "/api/v1/workorch/runs/run-metrics", "/api/v1/workorch/slices", "/api/v1/workorch/slices/snapshots", "/api/v1/workorch/slices/statuses"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200", path, rec.Code)
		}
	}

	runReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/runs", nil)
	runRec := httptest.NewRecorder()
	handler.ServeHTTP(runRec, runReq)
	if runRec.Code != http.StatusOK {
		t.Fatalf("run status = %d, want 200", runRec.Code)
	}
	singleRunReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/slices/workorch-runnable/runs", nil)
	singleRunRec := httptest.NewRecorder()
	handler.ServeHTTP(singleRunRec, singleRunReq)
	if singleRunRec.Code != http.StatusOK {
		t.Fatalf("single run status = %d, want 200", singleRunRec.Code)
	}
	for _, path := range []string{
		"/api/v1/workorch/workspaces/ws-metrics/snapshot",
		"/api/v1/workorch/runs/run-metrics/snapshot",
		"/api/v1/workorch/runs/run-metrics/status",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200", path, rec.Code)
		}
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	handler.ServeHTTP(metricsRec, metricsReq)

	if metricsRec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", metricsRec.Code)
	}
	if contentType := metricsRec.Header().Get("Content-Type"); !strings.Contains(contentType, "text/plain") {
		t.Fatalf("content-type = %q, want text/plain", contentType)
	}

	body := metricsRec.Body.String()
	for _, want := range []string{
		`workorch_http_requests_total{endpoint="healthz"} 1`,
		`workorch_http_requests_total{endpoint="readyz"} 1`,
		`workorch_http_requests_total{endpoint="metrics"} 1`,
		`workorch_http_requests_total{endpoint="workspace_create"} 2`,
		`workorch_http_requests_total{endpoint="workspace_list"} 1`,
		`workorch_http_requests_total{endpoint="workspace_get"} 1`,
		`workorch_http_requests_total{endpoint="workspace_snapshot"} 1`,
		`workorch_http_requests_total{endpoint="run_create"} 2`,
		`workorch_http_requests_total{endpoint="run_list"} 1`,
		`workorch_http_requests_total{endpoint="run_get"} 1`,
		`workorch_http_requests_total{endpoint="run_snapshot"} 1`,
		`workorch_http_requests_total{endpoint="run_status"} 1`,
		`workorch_http_requests_total{endpoint="run_start"} 2`,
		`workorch_http_requests_total{endpoint="run_resume"} 1`,
		`workorch_http_requests_total{endpoint="decision_list"} 0`,
		`workorch_http_requests_total{endpoint="decision_get"} 0`,
		`workorch_http_requests_total{endpoint="decision_resolve"} 1`,
		`workorch_http_requests_total{endpoint="decision_reject"} 0`,
		`workorch_http_requests_total{endpoint="approval_list"} 0`,
		`workorch_http_requests_total{endpoint="approval_get"} 0`,
		`workorch_http_requests_total{endpoint="approval_approve"} 0`,
		`workorch_http_requests_total{endpoint="approval_reject"} 0`,
		`workorch_http_requests_total{endpoint="slices"} 1`,
		`workorch_http_requests_total{endpoint="slice_snapshots"} 1`,
		`workorch_http_requests_total{endpoint="slice_statuses"} 1`,
		`workorch_http_requests_total{endpoint="slice_snapshot"} 0`,
		`workorch_http_requests_total{endpoint="slice_status"} 0`,
		`workorch_http_requests_total{endpoint="runs"} 1`,
		`workorch_http_requests_total{endpoint="slice_runs"} 1`,
		`workorch_run_queue_waiting 0`,
		`workorch_run_queue_total 0`,
		`workorch_run_outcomes_total{status="succeeded"} 5`,
		`workorch_run_outcomes_total{status="canceled"} 0`,
		`workorch_server_max_concurrent_runs 1`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics body missing %q\nbody:\n%s", want, body)
		}
	}
}

func createWorkspaceAndRunForHTTP(t *testing.T, handler http.Handler, workspaceID string, title string, runID string) {
	t.Helper()

	workspaceReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces", strings.NewReader(`{"workspaceId":"`+workspaceID+`","title":"`+title+`"}`))
	workspaceReq.Header.Set("Content-Type", "application/json")
	workspaceRec := httptest.NewRecorder()
	handler.ServeHTTP(workspaceRec, workspaceReq)
	if workspaceRec.Code != http.StatusCreated {
		t.Fatalf("workspace create status = %d, want 201, body=%s", workspaceRec.Code, workspaceRec.Body.String())
	}

	runReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/"+workspaceID+"/runs", strings.NewReader(`{"runId":"`+runID+`"}`))
	runReq.Header.Set("Content-Type", "application/json")
	runRec := httptest.NewRecorder()
	handler.ServeHTTP(runRec, runReq)
	if runRec.Code != http.StatusCreated {
		t.Fatalf("run create status = %d, want 201, body=%s", runRec.Code, runRec.Body.String())
	}
}

func startRunForHTTP(t *testing.T, handler http.Handler, runID string) appservice.Run {
	t.Helper()

	startReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/runs/"+runID+"/start", nil)
	startRec := httptest.NewRecorder()
	handler.ServeHTTP(startRec, startReq)
	if startRec.Code != http.StatusOK {
		t.Fatalf("start status = %d, want 200, body=%s", startRec.Code, startRec.Body.String())
	}
	var body runInstanceResponse
	if err := json.Unmarshal(startRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal started run: %v", err)
	}
	return body.Run
}

func resolveDecisionForHTTP(t *testing.T, dataRoot string, ticketID string, reject bool) {
	t.Helper()

	ctx := context.Background()
	session, err := appservice.NewRuntimeSessionWithBackend("file", dataRoot)
	if err != nil {
		t.Fatalf("new runtime session: %v", err)
	}
	defer func() {
		if err := session.Close(ctx); err != nil {
			t.Fatalf("close runtime session: %v", err)
		}
	}()

	resolutionType := decisioncommon.ResolutionTypeApprovePath
	resolutionID := "resolution-" + ticketID
	if reject {
		resolutionType = decisioncommon.ResolutionTypeRejectPath
		resolutionID += "-reject"
	}
	if err := session.Bootstrap.Exports().Services.DecisionService.EnsureResolvedTicket(ctx, decisionmodule.ResolveTicketCommand{
		ResolutionID: common.ResolutionID(resolutionID),
		TicketID:     common.TicketID(ticketID),
		Type:         resolutionType,
	}); err != nil {
		t.Fatalf("EnsureResolvedTicket: %v", err)
	}
}

func rejectDecisionViaHTTP(t *testing.T, handler http.Handler, ticketID string) appservice.Decision {
	t.Helper()

	rejectReq := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/decisions/"+ticketID+"/reject", nil)
	rejectRec := httptest.NewRecorder()
	handler.ServeHTTP(rejectRec, rejectReq)
	if rejectRec.Code != http.StatusOK {
		t.Fatalf("decision reject status = %d, want 200, body=%s", rejectRec.Code, rejectRec.Body.String())
	}
	var body decisionResponse
	if err := json.Unmarshal(rejectRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal decision reject response: %v", err)
	}
	return body.Decision
}

func resolveApprovalForHTTP(t *testing.T, dataRoot string, approvalID string, reject bool) {
	t.Helper()

	ctx := context.Background()
	session, err := appservice.NewRuntimeSessionWithBackend("file", dataRoot)
	if err != nil {
		t.Fatalf("new runtime session: %v", err)
	}
	defer func() {
		if err := session.Close(ctx); err != nil {
			t.Fatalf("close runtime session: %v", err)
		}
	}()

	cmd := approvalmodule.ResolveApprovalCommand{ApprovalID: common.ApprovalID(approvalID)}
	if reject {
		if err := session.Bootstrap.Exports().Services.ApprovalService.EnsureRejected(ctx, cmd); err != nil {
			t.Fatalf("EnsureRejected: %v", err)
		}
		return
	}
	if err := session.Bootstrap.Exports().Services.ApprovalService.EnsureApproved(ctx, cmd); err != nil {
		t.Fatalf("EnsureApproved: %v", err)
	}
}

func TestRunEndpointWaitsWhenConcurrentRunLimitReached(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	done := make(chan struct{})
	secondDone := make(chan int, 1)
	state := newServerState(normalizeConfig(Config{
		Backend:           "memory",
		MaxConcurrentRuns: 1,
	}))
	state.validate = func(context.Context, string, string) error { return nil }
	state.run = func(context.Context, string, string) ([]appservice.Slice, error) {
		started <- struct{}{}
		<-release
		return []appservice.Slice{{Title: "held"}}, nil
	}

	server := httptest.NewServer(newMuxWithState(state))
	defer server.Close()

	go func() {
		req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/workorch/runs", nil)
		if err != nil {
			t.Errorf("new request: %v", err)
			close(done)
			return
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Errorf("first run request: %v", err)
			close(done)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("first run status = %d, want 200", resp.StatusCode)
		}
		close(done)
	}()

	<-started

	go func() {
		secondResp, err := http.Post(server.URL+"/api/v1/workorch/runs", "application/json", nil)
		if err != nil {
			t.Errorf("second run request: %v", err)
			secondDone <- 0
			return
		}
		defer secondResp.Body.Close()
		secondDone <- secondResp.StatusCode
	}()

	select {
	case status := <-secondDone:
		t.Fatalf("second run completed before slot released, status = %d", status)
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	<-done
	secondStatus := <-secondDone
	if secondStatus != http.StatusOK {
		t.Fatalf("second run status = %d, want 200", secondStatus)
	}

	metrics := state.metrics.render(state.config)
	for _, want := range []string{
		`workorch_run_queue_total 1`,
		`workorch_run_queue_waiting 0`,
		`workorch_run_outcomes_total{status="succeeded"} 2`,
		`workorch_run_outcomes_total{status="rejected"} 0`,
	} {
		if !strings.Contains(metrics, want) {
			t.Fatalf("metrics body missing %q\nbody:\n%s", want, metrics)
		}
	}
}

func TestRunStartEndpointWaitsForAvailableExecutionSlot(t *testing.T) {
	firstStarted := make(chan struct{}, 1)
	secondStarted := make(chan struct{}, 1)
	release := make(chan struct{})
	firstDone := make(chan struct{})
	secondDone := make(chan int, 1)
	state := newServerState(normalizeConfig(Config{
		Backend:           "memory",
		MaxConcurrentRuns: 1,
	}))
	state.validate = func(context.Context, string, string) error { return nil }
	state.startRun = func(ctx context.Context, backend string, dataRoot string, runID string) (appservice.Run, error) {
		if runID == "run-1" {
			firstStarted <- struct{}{}
			<-release
			return appservice.Run{ID: runID, State: "completed"}, nil
		}
		secondStarted <- struct{}{}
		return appservice.Run{ID: runID, State: "completed"}, nil
	}

	server := httptest.NewServer(newMuxWithState(state))
	defer server.Close()

	go func() {
		req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/workorch/runs/run-1/start", nil)
		if err != nil {
			t.Errorf("new first start request: %v", err)
			close(firstDone)
			return
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Errorf("first start request: %v", err)
			close(firstDone)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("first start status = %d, want 200", resp.StatusCode)
		}
		close(firstDone)
	}()

	<-firstStarted

	go func() {
		resp, err := http.Post(server.URL+"/api/v1/workorch/runs/run-2/start", "application/json", nil)
		if err != nil {
			t.Errorf("second start request: %v", err)
			secondDone <- 0
			return
		}
		defer resp.Body.Close()
		secondDone <- resp.StatusCode
	}()

	select {
	case <-secondStarted:
		t.Fatal("second start reached executor before slot released")
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	<-firstDone

	select {
	case <-secondStarted:
	case <-time.After(time.Second):
		t.Fatal("second start did not begin after slot released")
	}

	secondStatus := <-secondDone
	if secondStatus != http.StatusOK {
		t.Fatalf("second start status = %d, want 200", secondStatus)
	}

	metrics := state.metrics.render(state.config)
	for _, want := range []string{
		`workorch_http_requests_total{endpoint="run_start"} 2`,
		`workorch_run_queue_total 1`,
		`workorch_run_outcomes_total{status="succeeded"} 2`,
	} {
		if !strings.Contains(metrics, want) {
			t.Fatalf("metrics body missing %q\nbody:\n%s", want, metrics)
		}
	}
}

func TestRunResumeEndpointReturnsRequestTimeoutWhenQueueWaitCanceled(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	done := make(chan struct{})
	state := newServerState(normalizeConfig(Config{
		Backend:           "memory",
		MaxConcurrentRuns: 1,
	}))
	state.validate = func(context.Context, string, string) error { return nil }
	state.resumeRun = func(ctx context.Context, backend string, dataRoot string, runID string) (appservice.Run, error) {
		if runID == "run-held" {
			started <- struct{}{}
			<-release
			return appservice.Run{ID: runID, State: "completed"}, nil
		}
		return appservice.Run{ID: runID, State: "completed"}, nil
	}

	handler := newMuxWithState(state)

	go func() {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/runs/run-held/resume", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("held resume status = %d, want 200", rec.Code)
		}
		close(done)
	}()

	<-started

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/runs/run-queued/resume", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestTimeout {
		t.Fatalf("queued resume status = %d, want 408, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "waiting for available execution slot") {
		t.Fatalf("queued resume body = %s, want queue wait message", rec.Body.String())
	}

	close(release)
	<-done

	metrics := state.metrics.render(state.config)
	for _, want := range []string{
		`workorch_run_queue_total 1`,
		`workorch_run_queue_waiting 0`,
		`workorch_run_outcomes_total{status="canceled"} 1`,
	} {
		if !strings.Contains(metrics, want) {
			t.Fatalf("metrics body missing %q\nbody:\n%s", want, metrics)
		}
	}
}

func TestConfigFromEnvReturnsErrorForInvalidDuration(t *testing.T) {
	t.Setenv(envServerReadTimeout, "definitely-not-a-duration")

	_, err := ConfigFromEnv()
	if err == nil {
		t.Fatal("expected invalid duration error")
	}
	if !strings.Contains(err.Error(), envServerReadTimeout) {
		t.Fatalf("error = %v, want env name %s", err, envServerReadTimeout)
	}
}

func TestConfigFromEnvReturnsErrorForInvalidConcurrentLimit(t *testing.T) {
	t.Setenv(envServerMaxConcurrentRuns, "0")

	_, err := ConfigFromEnv()
	if err == nil {
		t.Fatal("expected invalid concurrent limit error")
	}
	if !strings.Contains(err.Error(), envServerMaxConcurrentRuns) {
		t.Fatalf("error = %v, want env name %s", err, envServerMaxConcurrentRuns)
	}
}
