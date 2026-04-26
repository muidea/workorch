package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	legacycommon "github.com/muidea/workorch/internal/modules/blocks/legacy/pkg/common"
	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
)

type workorchRunResponse struct {
	Service string `json:"service"`
	Slices  []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	} `json:"slices"`
}

type workorchWorkspaceResponse struct {
	Service   string `json:"service"`
	Workspace struct {
		ID               string `json:"id"`
		Title            string `json:"title"`
		Description      string `json:"description"`
		WorkingDirectory string `json:"workingDirectory"`
		State            string `json:"state"`
		ActiveRunID      string `json:"activeRunId"`
	} `json:"workspace"`
}

type workorchWorkspacesResponse struct {
	Service    string `json:"service"`
	Workspaces []struct {
		ID               string `json:"id"`
		Title            string `json:"title"`
		Description      string `json:"description"`
		WorkingDirectory string `json:"workingDirectory"`
		State            string `json:"state"`
	} `json:"workspaces"`
}

type workorchGoalResponse struct {
	Service string `json:"service"`
	Goal    struct {
		ID          string `json:"id"`
		WorkspaceID string `json:"workspaceId"`
		Text        string `json:"text"`
	} `json:"goal"`
}

type workorchDoDResponse struct {
	Service string `json:"service"`
	DoD     struct {
		ID          string   `json:"id"`
		WorkspaceID string   `json:"workspaceId"`
		Items       []string `json:"items"`
		State       string   `json:"state"`
	} `json:"dod"`
}

type workorchRunInstanceResponse struct {
	Service string `json:"service"`
	Run     struct {
		ID                 string `json:"id"`
		WorkspaceID        string `json:"workspaceId"`
		GoalID             string `json:"goalId"`
		DoDID              string `json:"doDId"`
		State              string `json:"state"`
		BlockingTicketID   string `json:"blockingTicketId"`
		BlockingApprovalID string `json:"blockingApprovalId"`
		FailureReason      string `json:"failureReason"`
	} `json:"run"`
}

type workorchRunSnapshotResponse struct {
	Service  string `json:"service"`
	Snapshot struct {
		Workspace struct {
			ID          string `json:"id"`
			Title       string `json:"title"`
			State       string `json:"state"`
			ActiveRunID string `json:"activeRunId"`
		} `json:"workspace"`
		Run struct {
			ID                 string `json:"id"`
			WorkspaceID        string `json:"workspaceId"`
			State              string `json:"state"`
			BlockingTicketID   string `json:"blockingTicketId"`
			BlockingApprovalID string `json:"blockingApprovalId"`
		} `json:"run"`
		Step struct {
			ID    string `json:"id"`
			Kind  string `json:"kind"`
			State string `json:"state"`
		} `json:"step"`
		Steps []struct {
			ID    string `json:"id"`
			Kind  string `json:"kind"`
			State string `json:"state"`
		} `json:"steps"`
		Decisions []struct {
			ID    string `json:"id"`
			State string `json:"state"`
		} `json:"decisions"`
		Approvals []struct {
			ID    string `json:"id"`
			State string `json:"state"`
		} `json:"approvals"`
		Observed []struct {
			ID string `json:"id"`
		} `json:"observed"`
	} `json:"snapshot"`
}

type workorchRunStatusResponse struct {
	Service string `json:"service"`
	Status  struct {
		ID                 string `json:"id"`
		WorkspaceID        string `json:"workspaceId"`
		State              string `json:"state"`
		CurrentStepID      string `json:"currentStepId"`
		CurrentStepKind    string `json:"currentStepKind"`
		CurrentStepState   string `json:"currentStepState"`
		StepCount          int    `json:"stepCount"`
		DecisionCount      int    `json:"decisionCount"`
		ApprovalCount      int    `json:"approvalCount"`
		BlockingTicketID   string `json:"blockingTicketId"`
		BlockingApprovalID string `json:"blockingApprovalId"`
		FailureReason      string `json:"failureReason"`
	} `json:"status"`
}

type workorchRunsResponse struct {
	Service string `json:"service"`
	Runs    []struct {
		ID          string `json:"id"`
		WorkspaceID string `json:"workspaceId"`
		State       string `json:"state"`
	} `json:"runs"`
}

type workorchDecisionResponse struct {
	Service  string `json:"service"`
	Decision struct {
		ID                string         `json:"id"`
		WorkspaceID       string         `json:"workspaceId"`
		RunID             string         `json:"runId"`
		StepID            string         `json:"stepId"`
		Reason            string         `json:"reason"`
		State             string         `json:"state"`
		ResolutionID      string         `json:"resolutionId"`
		ResolutionType    string         `json:"resolutionType"`
		ResolutionPayload map[string]any `json:"resolutionPayload"`
	} `json:"decision"`
}

type workorchDecisionsResponse struct {
	Service   string `json:"service"`
	Decisions []struct {
		ID             string `json:"id"`
		WorkspaceID    string `json:"workspaceId"`
		RunID          string `json:"runId"`
		State          string `json:"state"`
		ResolutionType string `json:"resolutionType"`
	} `json:"decisions"`
}

type workorchApprovalResponse struct {
	Service  string `json:"service"`
	Approval struct {
		ID          string `json:"id"`
		WorkspaceID string `json:"workspaceId"`
		RunID       string `json:"runId"`
		StepID      string `json:"stepId"`
		Reason      string `json:"reason"`
		State       string `json:"state"`
	} `json:"approval"`
}

type workorchApprovalsResponse struct {
	Service   string `json:"service"`
	Approvals []struct {
		ID          string `json:"id"`
		WorkspaceID string `json:"workspaceId"`
		RunID       string `json:"runId"`
		State       string `json:"state"`
	} `json:"approvals"`
}

type workorchWorkspaceSnapshotResponse struct {
	Service  string `json:"service"`
	Snapshot struct {
		Workspace struct {
			ID          string `json:"id"`
			Title       string `json:"title"`
			State       string `json:"state"`
			ActiveRunID string `json:"activeRunId"`
		} `json:"workspace"`
		Runs []struct {
			ID          string `json:"id"`
			WorkspaceID string `json:"workspaceId"`
			State       string `json:"state"`
		} `json:"runs"`
		Decisions []struct {
			ID    string `json:"id"`
			State string `json:"state"`
		} `json:"decisions"`
		Approvals []struct {
			ID    string `json:"id"`
			State string `json:"state"`
		} `json:"approvals"`
	} `json:"snapshot"`
}

type workorchErrorResponse struct {
	Error string `json:"error"`
}

type workorchSliceCatalogResponse struct {
	Service string `json:"service"`
	Slices  []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	} `json:"slices"`
}

type workorchSliceSnapshotsResponse struct {
	Service string `json:"service"`
	Slices  []struct {
		ID       string `json:"id"`
		Title    string `json:"title"`
		Snapshot struct {
			Slice string `json:"slice"`
		} `json:"snapshot"`
	} `json:"slices"`
}

type workorchSliceStatusesResponse struct {
	Service  string `json:"service"`
	Statuses []struct {
		ID       string `json:"id"`
		RunState string `json:"runState"`
	} `json:"statuses"`
}

type workorchSingleSliceRunResponse struct {
	Service string `json:"service"`
	Slice   struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	} `json:"slice"`
}

type workorchSliceStatusResponse struct {
	Service string `json:"service"`
	Status  struct {
		ID        string `json:"id"`
		RunState  string `json:"runState"`
		StepState string `json:"stepState"`
	} `json:"status"`
}

func TestWorkorchdServerServesHealthAndRunEndpoints(t *testing.T) {
	addr := reserveLoopbackAddr(t)
	bin := buildWorkorchdBinary(t)
	cmd := startWorkorchdProcess(t, bin, addr, nil)
	defer stopWorkorchdProcess(t, cmd)

	waitForHealthy(t, "http://"+addr+"/healthz")
	assertMetricsAvailable(t, "http://"+addr+"/metrics")
	createWorkspace(t, "http://"+addr+"/api/v1/workorch/workspaces", "main memory workspace")
	assertSliceCatalogAvailable(t, "http://"+addr+"/api/v1/workorch/slices")

	resp := postRunRequest(t, "http://"+addr+"/api/v1/workorch/runs")
	if resp.Service != "workorch" {
		t.Fatalf("service = %q, want workorch", resp.Service)
	}
	if len(resp.Slices) != 17 {
		t.Fatalf("len(slices) = %d, want 17", len(resp.Slices))
	}
	if resp.Slices[0].Title != "workorch runnable slice" {
		t.Fatalf("first title = %q, want workorch runnable slice", resp.Slices[0].Title)
	}
	single := postSingleSliceRunRequest(t, "http://"+addr+"/api/v1/workorch/slices/workorch-runnable/runs")
	if single.Slice.ID != "workorch-runnable" || single.Slice.Title != "workorch runnable slice" {
		t.Fatalf("single slice = %+v, want workorch-runnable/workorch runnable slice", single.Slice)
	}
}

func TestWorkorchdServerSupportsFileRepositoryBackend(t *testing.T) {
	addr := reserveLoopbackAddr(t)
	dataDir := filepath.Join(t.TempDir(), "workorch-data")
	bin := buildWorkorchdBinary(t)
	cmd := startWorkorchdProcess(t, bin, addr, []string{
		"repository.backend=file",
		"repository.dataDir=" + dataDir,
	})
	defer stopWorkorchdProcess(t, cmd)

	waitForHealthy(t, "http://"+addr+"/healthz")

	workspaceID := createWorkspace(t, "http://"+addr+"/api/v1/workorch/workspaces", "main file workspace")
	createRunInstance(t, "http://"+addr+"/api/v1/workorch/workspaces/"+workspaceID+"/runs", "run-main-file", "goal-main-file", "dod-main-file")
	assertWorkspaceListAvailable(t, "http://"+addr+"/api/v1/workorch/workspaces", workspaceID)
	assertWorkspaceAvailable(t, "http://"+addr+"/api/v1/workorch/workspaces/"+workspaceID, workspaceID, "main file workspace")
	assertRunListAvailable(t, "http://"+addr+"/api/v1/workorch/runs", "run-main-file")
	assertRunAvailable(t, "http://"+addr+"/api/v1/workorch/runs/run-main-file", "run-main-file", workspaceID)
	assertRunListAvailable(t, "http://"+addr+"/api/v1/workorch/runs?workspaceId="+workspaceID, "run-main-file")
	assertRunStartAvailable(t, "http://"+addr+"/api/v1/workorch/runs/run-main-file/start", "run-main-file", "completed")
	assertRunSnapshotAvailable(t, "http://"+addr+"/api/v1/workorch/runs/run-main-file/snapshot", "run-main-file", workspaceID)
	assertRunStatusAvailable(t, "http://"+addr+"/api/v1/workorch/runs/run-main-file/status", "run-main-file", "completed")

	decisionResolveRunID := "run-" + legacycommon.RuntimeDecisionBlockRunToken + "-main-file"
	createRunInstance(t, "http://"+addr+"/api/v1/workorch/workspaces/"+workspaceID+"/runs", decisionResolveRunID, "", "")
	blockedDecision := assertRunStartAvailable(t, "http://"+addr+"/api/v1/workorch/runs/"+decisionResolveRunID+"/start", decisionResolveRunID, "blocked_on_decision")
	assertRunStatusAvailable(t, "http://"+addr+"/api/v1/workorch/runs/"+decisionResolveRunID+"/status", decisionResolveRunID, "blocked_on_decision")
	assertDecisionListAvailable(t, "http://"+addr+"/api/v1/workorch/decisions", blockedDecision.Run.BlockingTicketID)
	assertDecisionAvailable(t, "http://"+addr+"/api/v1/workorch/decisions/"+blockedDecision.Run.BlockingTicketID, blockedDecision.Run.BlockingTicketID, "open")
	assertDecisionListAvailable(t, "http://"+addr+"/api/v1/workorch/decisions?workspaceId="+workspaceID, blockedDecision.Run.BlockingTicketID)
	assertDecisionListAvailable(t, "http://"+addr+"/api/v1/workorch/decisions?runId="+decisionResolveRunID, blockedDecision.Run.BlockingTicketID)
	assertDecisionResolveAvailable(t, "http://"+addr+"/api/v1/workorch/decisions/"+blockedDecision.Run.BlockingTicketID+"/resolve", blockedDecision.Run.BlockingTicketID, decisioncommon.ResolutionTypeProvideMissingInfo)
	assertRunResumeAvailable(t, "http://"+addr+"/api/v1/workorch/runs/"+decisionResolveRunID+"/resume", decisionResolveRunID, "completed")

	decisionRejectRunID := "run-" + legacycommon.RuntimeDecisionBlockRunToken + "-rejected-main-file"
	createRunInstance(t, "http://"+addr+"/api/v1/workorch/workspaces/"+workspaceID+"/runs", decisionRejectRunID, "", "")
	rejectedDecision := assertRunStartAvailable(t, "http://"+addr+"/api/v1/workorch/runs/"+decisionRejectRunID+"/start", decisionRejectRunID, "blocked_on_decision")
	assertDecisionRejectAvailable(t, "http://"+addr+"/api/v1/workorch/decisions/"+rejectedDecision.Run.BlockingTicketID+"/resolve", rejectedDecision.Run.BlockingTicketID, decisioncommon.ResolutionTypeRejectCurrentPath)
	rejectedRun := assertRunResumeAvailable(t, "http://"+addr+"/api/v1/workorch/runs/"+decisionRejectRunID+"/resume", decisionRejectRunID, "failed")
	if rejectedRun.Run.FailureReason == "" {
		t.Fatalf("rejected run failureReason = %q, want non-empty", rejectedRun.Run.FailureReason)
	}

	approvalApproveRunID := "run-" + legacycommon.RuntimeApprovalBlockRunToken + "-main-file"
	createRunInstance(t, "http://"+addr+"/api/v1/workorch/workspaces/"+workspaceID+"/runs", approvalApproveRunID, "", "")
	blockedApproval := assertRunStartAvailable(t, "http://"+addr+"/api/v1/workorch/runs/"+approvalApproveRunID+"/start", approvalApproveRunID, "blocked_on_approval")
	assertApprovalListAvailable(t, "http://"+addr+"/api/v1/workorch/approvals", blockedApproval.Run.BlockingApprovalID)
	assertApprovalAvailable(t, "http://"+addr+"/api/v1/workorch/approvals/"+blockedApproval.Run.BlockingApprovalID, blockedApproval.Run.BlockingApprovalID, "pending")
	assertApprovalListAvailable(t, "http://"+addr+"/api/v1/workorch/approvals?workspaceId="+workspaceID, blockedApproval.Run.BlockingApprovalID)
	assertApprovalListAvailable(t, "http://"+addr+"/api/v1/workorch/approvals?runId="+approvalApproveRunID, blockedApproval.Run.BlockingApprovalID)
	assertApprovalApproveAvailable(t, "http://"+addr+"/api/v1/workorch/approvals/"+blockedApproval.Run.BlockingApprovalID+"/approve", blockedApproval.Run.BlockingApprovalID, "approved")
	assertRunResumeAvailable(t, "http://"+addr+"/api/v1/workorch/runs/"+approvalApproveRunID+"/resume", approvalApproveRunID, "completed")

	approvalRejectRunID := "run-" + legacycommon.RuntimeApprovalBlockRunToken + "-rejected-main-file"
	createRunInstance(t, "http://"+addr+"/api/v1/workorch/workspaces/"+workspaceID+"/runs", approvalRejectRunID, "", "")
	rejectedApproval := assertRunStartAvailable(t, "http://"+addr+"/api/v1/workorch/runs/"+approvalRejectRunID+"/start", approvalRejectRunID, "blocked_on_approval")
	assertApprovalRejectAvailable(t, "http://"+addr+"/api/v1/workorch/approvals/"+rejectedApproval.Run.BlockingApprovalID+"/reject", rejectedApproval.Run.BlockingApprovalID, "rejected")
	rejectedApprovalRun := assertRunResumeAvailable(t, "http://"+addr+"/api/v1/workorch/runs/"+approvalRejectRunID+"/resume", approvalRejectRunID, "failed")
	if rejectedApprovalRun.Run.FailureReason == "" {
		t.Fatalf("rejected approval run failureReason = %q, want non-empty", rejectedApprovalRun.Run.FailureReason)
	}
	assertWorkspaceSnapshotAvailable(
		t,
		"http://"+addr+"/api/v1/workorch/workspaces/"+workspaceID+"/snapshot",
		workspaceID,
		[]string{"run-main-file", decisionResolveRunID, decisionRejectRunID, approvalApproveRunID, approvalRejectRunID},
		[]string{blockedDecision.Run.BlockingTicketID, rejectedDecision.Run.BlockingTicketID},
		[]string{blockedApproval.Run.BlockingApprovalID, rejectedApproval.Run.BlockingApprovalID},
	)

	first := postRunRequest(t, "http://"+addr+"/api/v1/workorch/runs")
	second := postRunRequest(t, "http://"+addr+"/api/v1/workorch/runs")
	single := postSingleSliceRunRequest(t, "http://"+addr+"/api/v1/workorch/slices/workorch-runnable/runs")
	assertSliceSnapshotsAvailable(t, "http://"+addr+"/api/v1/workorch/slices/snapshots")
	assertSliceStatusesAvailable(t, "http://"+addr+"/api/v1/workorch/slices/statuses")
	assertSingleSliceSnapshot(t, "http://"+addr+"/api/v1/workorch/slices/workorch-runnable/snapshot")
	assertSingleSliceStatus(t, "http://"+addr+"/api/v1/workorch/slices/workorch-runnable/status")

	if len(first.Slices) != 17 || len(second.Slices) != 17 {
		t.Fatalf("run slices len = %d/%d, want 17/17", len(first.Slices), len(second.Slices))
	}
	if single.Slice.ID != "workorch-runnable" {
		t.Fatalf("single slice id = %q, want workorch-runnable", single.Slice.ID)
	}
	for _, name := range []string{
		"workspace.json",
		"goal.json",
		"dod.json",
		"run.json",
		"step.json",
		"ticket.json",
		"resolution.json",
		"approval.json",
	} {
		if _, err := os.Stat(filepath.Join(dataDir, name)); err != nil {
			t.Fatalf("expected persisted file %s: %v", name, err)
		}
	}
}

func TestWorkorchdServerRetainsFileBackedTerminalStateAcrossRestart(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "workorch-restart-data")
	bin := buildWorkorchdBinary(t)
	env := []string{
		"repository.backend=file",
		"repository.dataDir=" + dataDir,
	}

	firstAddr := reserveLoopbackAddr(t)
	first := startWorkorchdProcess(t, bin, firstAddr, env)
	waitForHealthy(t, "http://"+firstAddr+"/healthz")

	workspaceID := createWorkspace(t, "http://"+firstAddr+"/api/v1/workorch/workspaces", "restart file workspace")

	createRunInstance(t, "http://"+firstAddr+"/api/v1/workorch/workspaces/"+workspaceID+"/runs", "run-restart-completed", "", "")
	assertRunStartAvailable(t, "http://"+firstAddr+"/api/v1/workorch/runs/run-restart-completed/start", "run-restart-completed", "completed")

	decisionRunID := "run-" + legacycommon.RuntimeDecisionBlockRunToken + "-restart-completed"
	createRunInstance(t, "http://"+firstAddr+"/api/v1/workorch/workspaces/"+workspaceID+"/runs", decisionRunID, "", "")
	blockedDecision := assertRunStartAvailable(t, "http://"+firstAddr+"/api/v1/workorch/runs/"+decisionRunID+"/start", decisionRunID, "blocked_on_decision")
	assertDecisionResolveAvailable(t, "http://"+firstAddr+"/api/v1/workorch/decisions/"+blockedDecision.Run.BlockingTicketID+"/resolve", blockedDecision.Run.BlockingTicketID, decisioncommon.ResolutionTypeProvideMissingInfo)
	assertRunResumeAvailable(t, "http://"+firstAddr+"/api/v1/workorch/runs/"+decisionRunID+"/resume", decisionRunID, "completed")

	approvalRunID := "run-" + legacycommon.RuntimeApprovalBlockRunToken + "-restart-failed"
	createRunInstance(t, "http://"+firstAddr+"/api/v1/workorch/workspaces/"+workspaceID+"/runs", approvalRunID, "", "")
	blockedApproval := assertRunStartAvailable(t, "http://"+firstAddr+"/api/v1/workorch/runs/"+approvalRunID+"/start", approvalRunID, "blocked_on_approval")
	assertApprovalRejectAvailable(t, "http://"+firstAddr+"/api/v1/workorch/approvals/"+blockedApproval.Run.BlockingApprovalID+"/reject", blockedApproval.Run.BlockingApprovalID, "rejected")
	assertRunResumeAvailable(t, "http://"+firstAddr+"/api/v1/workorch/runs/"+approvalRunID+"/resume", approvalRunID, "failed")

	stopWorkorchdProcess(t, first)

	secondAddr := reserveLoopbackAddr(t)
	second := startWorkorchdProcess(t, bin, secondAddr, env)
	defer stopWorkorchdProcess(t, second)

	waitForHealthy(t, "http://"+secondAddr+"/healthz")

	assertWorkspaceAvailable(t, "http://"+secondAddr+"/api/v1/workorch/workspaces/"+workspaceID, workspaceID, "restart file workspace")
	assertRunAvailableWithState(t, "http://"+secondAddr+"/api/v1/workorch/runs/run-restart-completed", "run-restart-completed", workspaceID, "completed")
	assertRunSnapshotAvailableWithState(t, "http://"+secondAddr+"/api/v1/workorch/runs/run-restart-completed/snapshot", "run-restart-completed", workspaceID, "completed")
	assertRunStatusAvailable(t, "http://"+secondAddr+"/api/v1/workorch/runs/run-restart-completed/status", "run-restart-completed", "completed")

	assertRunAvailableWithState(t, "http://"+secondAddr+"/api/v1/workorch/runs/"+decisionRunID, decisionRunID, workspaceID, "completed")
	assertRunSnapshotAvailableWithState(t, "http://"+secondAddr+"/api/v1/workorch/runs/"+decisionRunID+"/snapshot", decisionRunID, workspaceID, "completed")
	assertDecisionAvailable(t, "http://"+secondAddr+"/api/v1/workorch/decisions/"+blockedDecision.Run.BlockingTicketID, blockedDecision.Run.BlockingTicketID, "resolved")

	failedStatus := assertRunStatusAvailable(t, "http://"+secondAddr+"/api/v1/workorch/runs/"+approvalRunID+"/status", approvalRunID, "failed")
	if failedStatus.Status.FailureReason == "" {
		t.Fatalf("failed status = %+v, want non-empty failure reason", failedStatus.Status)
	}
	assertRunAvailableWithState(t, "http://"+secondAddr+"/api/v1/workorch/runs/"+approvalRunID, approvalRunID, workspaceID, "failed")
	assertRunSnapshotAvailableWithState(t, "http://"+secondAddr+"/api/v1/workorch/runs/"+approvalRunID+"/snapshot", approvalRunID, workspaceID, "failed")
	assertApprovalAvailable(t, "http://"+secondAddr+"/api/v1/workorch/approvals/"+blockedApproval.Run.BlockingApprovalID, blockedApproval.Run.BlockingApprovalID, "rejected")

	assertWorkspaceSnapshotAvailable(
		t,
		"http://"+secondAddr+"/api/v1/workorch/workspaces/"+workspaceID+"/snapshot",
		workspaceID,
		[]string{"run-restart-completed", decisionRunID, approvalRunID},
		[]string{blockedDecision.Run.BlockingTicketID},
		[]string{blockedApproval.Run.BlockingApprovalID},
	)
	assertRunStartAvailable(t, "http://"+secondAddr+"/api/v1/workorch/runs/run-restart-completed/start", "run-restart-completed", "completed")
	assertRunResumeAvailable(t, "http://"+secondAddr+"/api/v1/workorch/runs/"+approvalRunID+"/resume", approvalRunID, "failed")
	assertConflictAction(t, "http://"+secondAddr+"/api/v1/workorch/runs/"+approvalRunID+"/start", "conflict")
}

func TestWorkorchdServerFailsWithUnsupportedRepositoryBackend(t *testing.T) {
	addr := reserveLoopbackAddr(t)
	bin := buildWorkorchdBinary(t)
	configDir := writeWorkorchdTestConfig(t, addr, []string{
		"repository.backend=unsupported",
		"repository.dataDir=" + t.TempDir(),
	})
	cmd := exec.Command(bin, "-config-dir", configDir)
	cmd.Env = os.Environ()

	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected go run workorchd to fail\noutput:\n%s", string(out))
	}

	text := string(out)
	for _, want := range []string{
		"invalid argument",
		"unsupported repository backend",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("unsupported backend output missing %q\noutput:\n%s", want, text)
		}
	}
}

func TestWorkorchdServerFailsWithInvalidServerTimeout(t *testing.T) {
	addr := reserveLoopbackAddr(t)
	bin := buildWorkorchdBinary(t)
	configDir := writeWorkorchdTestConfig(t, addr, []string{
		"server.readTimeout=not-a-duration",
	})
	cmd := exec.Command(bin, "-config-dir", configDir)
	cmd.Env = os.Environ()

	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected workorchd to fail with invalid server timeout\noutput:\n%s", string(out))
	}

	text := string(out)
	for _, want := range []string{
		"invalid duration",
		"server.readTimeout",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("invalid timeout output missing %q\noutput:\n%s", want, text)
		}
	}
}

func reserveLoopbackAddr(t *testing.T) string {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen loopback: %v", err)
	}
	defer ln.Close()
	return ln.Addr().String()
}

func buildWorkorchdBinary(t *testing.T) string {
	t.Helper()

	bin := filepath.Join(t.TempDir(), "workorchd")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build workorchd: %v\noutput:\n%s", err, string(out))
	}
	return bin
}

func startWorkorchdProcess(t *testing.T, bin string, addr string, extraEnv []string) *exec.Cmd {
	t.Helper()

	configDir := writeWorkorchdTestConfig(t, addr, extraEnv)
	cmd := exec.Command(bin, "-config-dir", configDir)
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		t.Fatalf("start workorchd: %v", err)
	}
	return cmd
}

func writeWorkorchdTestConfig(t *testing.T, addr string, extraEnv []string) string {
	t.Helper()

	values := map[string]string{
		"repository.backend": "memory",
		"repository.dataDir": "",
		"server.readTimeout": "15s",
	}
	for _, item := range extraEnv {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			values[key] = value
		}
	}
	configDir := t.TempDir()
	content := fmt.Sprintf(`endpointName = "workorchd"

[server]
addr = %q
readHeaderTimeout = "5s"
readTimeout = %q
writeTimeout = "30s"
idleTimeout = "60s"
shutdownTimeout = "5s"
maxConcurrentRuns = 1

[repository]
backend = %q
dataDir = %q

[llm]
timeout = "60s"
disabled = false
`, addr, values["server.readTimeout"], values["repository.backend"], values["repository.dataDir"])
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write workorchd config: %v", err)
	}
	return configDir
}

func stopWorkorchdProcess(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Signal(os.Interrupt)
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	select {
	case err := <-done:
		if err != nil && !isInterruptExit(err) {
			t.Fatalf("wait workorchd: %v", err)
		}
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("timed out waiting for workorchd to exit")
	}
}

func isInterruptExit(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr)
}

func waitForHealthy(t *testing.T, healthURL string) {
	t.Helper()

	client := &http.Client{Timeout: 500 * time.Millisecond}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(healthURL)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("service did not become healthy: %s", healthURL)
}

func assertMetricsAvailable(t *testing.T, metricsURL string) {
	t.Helper()

	resp, err := http.Get(metricsURL)
	if err != nil {
		t.Fatalf("get metrics: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("metrics status = %d, want 200", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read metrics body: %v", err)
	}
	text := string(body)
	for _, want := range []string{
		"workorch_http_requests_total",
		"workorch_server_max_concurrent_runs",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("metrics body missing %q\nbody:\n%s", want, text)
		}
	}
}

func assertSliceCatalogAvailable(t *testing.T, endpoint string) {
	t.Helper()

	resp, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("get slice catalog: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("slice catalog status = %d, want 200", resp.StatusCode)
	}
	var body workorchSliceCatalogResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode slice catalog response: %v", err)
	}
	if len(body.Slices) != 17 {
		t.Fatalf("len(slices) = %d, want 17", len(body.Slices))
	}
	if body.Slices[0].ID != "workorch-runnable" || body.Slices[0].Title != "workorch runnable slice" {
		t.Fatalf("first slice = %+v, want workorch-runnable/workorch runnable slice", body.Slices[0])
	}
}

func createWorkspace(t *testing.T, endpoint string, title string) string {
	t.Helper()

	description := title + " description"
	body := strings.NewReader(fmt.Sprintf(`{"title":%q,"description":%q}`, title, description))
	resp, err := http.Post(endpoint, "application/json", body)
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create workspace status = %d, want 201", resp.StatusCode)
	}
	var payload workorchWorkspaceResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode create workspace response: %v", err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	workingDirectory := filepath.Join(wd, "workspaces", payload.Workspace.ID)
	if payload.Workspace.ID == "" || payload.Workspace.Title != title || payload.Workspace.Description != description || payload.Workspace.WorkingDirectory != workingDirectory || payload.Workspace.State != "active" {
		t.Fatalf("created workspace = %+v, want generated/%s/%s/%s/active", payload.Workspace, title, description, workingDirectory)
	}
	return payload.Workspace.ID
}

func assertWorkspaceListAvailable(t *testing.T, endpoint string, workspaceID string) {
	t.Helper()

	resp, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("get workspaces: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("workspaces status = %d, want 200", resp.StatusCode)
	}
	var body workorchWorkspacesResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode workspaces response: %v", err)
	}
	if len(body.Workspaces) == 0 {
		t.Fatal("expected at least one workspace")
	}
	found := false
	for _, item := range body.Workspaces {
		if item.ID == workspaceID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("workspace %q not found in %+v", workspaceID, body.Workspaces)
	}
}

func assertWorkspaceAvailable(t *testing.T, endpoint string, workspaceID string, title string) {
	t.Helper()

	resp, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("workspace status = %d, want 200", resp.StatusCode)
	}
	var body workorchWorkspaceResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode workspace response: %v", err)
	}
	if body.Workspace.ID != workspaceID || body.Workspace.Title != title || body.Workspace.State != "active" {
		t.Fatalf("workspace = %+v, want %s/%s/active", body.Workspace, workspaceID, title)
	}
}

func createRunInstance(t *testing.T, endpoint string, runID string, goalID string, dodID string) {
	t.Helper()

	goalID, dodID = defineGoalAndDoD(t, endpoint)
	body := strings.NewReader(fmt.Sprintf(`{"runId":%q,"goalId":%q,"doDId":%q}`, runID, goalID, dodID))
	resp, err := http.Post(endpoint, "application/json", body)
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create run status = %d, want 201", resp.StatusCode)
	}
	var payload workorchRunInstanceResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode create run response: %v", err)
	}
	if payload.Run.ID != runID || payload.Run.State != "created" {
		t.Fatalf("created run = %+v, want %s/created", payload.Run, runID)
	}
}

func defineGoalAndDoD(t *testing.T, runEndpoint string) (string, string) {
	t.Helper()

	workspaceEndpoint := strings.TrimSuffix(runEndpoint, "/runs")
	if workspaceEndpoint == runEndpoint {
		t.Fatalf("run endpoint %q does not end with /runs", runEndpoint)
	}
	goalResp, err := http.Post(workspaceEndpoint+"/goals", "application/json", strings.NewReader(`{"text":"main test goal"}`))
	if err != nil {
		t.Fatalf("define goal: %v", err)
	}
	defer goalResp.Body.Close()
	if goalResp.StatusCode != http.StatusCreated {
		t.Fatalf("define goal status = %d, want 201", goalResp.StatusCode)
	}
	var goalBody workorchGoalResponse
	if err := json.NewDecoder(goalResp.Body).Decode(&goalBody); err != nil {
		t.Fatalf("decode goal response: %v", err)
	}

	dodResp, err := http.Post(workspaceEndpoint+"/dods", "application/json", strings.NewReader(`{"items":["main test completed"]}`))
	if err != nil {
		t.Fatalf("define dod: %v", err)
	}
	defer dodResp.Body.Close()
	if dodResp.StatusCode != http.StatusCreated {
		t.Fatalf("define dod status = %d, want 201", dodResp.StatusCode)
	}
	var dodBody workorchDoDResponse
	if err := json.NewDecoder(dodResp.Body).Decode(&dodBody); err != nil {
		t.Fatalf("decode dod response: %v", err)
	}
	return goalBody.Goal.ID, dodBody.DoD.ID
}

func assertRunListAvailable(t *testing.T, endpoint string, runID string) {
	t.Helper()

	resp, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("get runs: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("runs status = %d, want 200", resp.StatusCode)
	}
	var body workorchRunsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode runs response: %v", err)
	}
	found := false
	for _, item := range body.Runs {
		if item.ID == runID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("run %q not found in %+v", runID, body.Runs)
	}
}

func assertRunAvailable(t *testing.T, endpoint string, runID string, workspaceID string) {
	t.Helper()
	assertRunAvailableWithState(t, endpoint, runID, workspaceID, "created")
}

func assertRunAvailableWithState(t *testing.T, endpoint string, runID string, workspaceID string, expectedState string) workorchRunInstanceResponse {
	t.Helper()

	resp, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("run status = %d, want 200", resp.StatusCode)
	}
	var body workorchRunInstanceResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode run response: %v", err)
	}
	if body.Run.ID != runID || body.Run.WorkspaceID != workspaceID || body.Run.State != expectedState {
		t.Fatalf("run = %+v, want %s/%s/%s", body.Run, runID, workspaceID, expectedState)
	}
	return body
}

func assertRunSnapshotAvailable(t *testing.T, endpoint string, runID string, workspaceID string) {
	t.Helper()
	assertRunSnapshotAvailableWithState(t, endpoint, runID, workspaceID, "")
}

func assertRunSnapshotAvailableWithState(t *testing.T, endpoint string, runID string, workspaceID string, expectedState string) workorchRunSnapshotResponse {
	t.Helper()

	resp, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("get run snapshot: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("run snapshot status = %d, want 200", resp.StatusCode)
	}
	var body workorchRunSnapshotResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode run snapshot response: %v", err)
	}
	if body.Snapshot.Workspace.ID != workspaceID || body.Snapshot.Run.ID != runID || body.Snapshot.Run.WorkspaceID != workspaceID {
		t.Fatalf("snapshot = %+v, want workspace %s and run %s", body.Snapshot, workspaceID, runID)
	}
	if expectedState != "" && body.Snapshot.Run.State != expectedState {
		t.Fatalf("snapshot run = %+v, want state %s", body.Snapshot.Run, expectedState)
	}
	if len(body.Snapshot.Steps) == 0 || len(body.Snapshot.Observed) == 0 {
		t.Fatalf("snapshot = %+v, want steps and observed items", body.Snapshot)
	}
	return body
}

func assertRunStatusAvailable(t *testing.T, endpoint string, runID string, expectedState string) workorchRunStatusResponse {
	t.Helper()

	resp, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("get run status: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("run status code = %d, want 200", resp.StatusCode)
	}
	var body workorchRunStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode run status response: %v", err)
	}
	if body.Status.ID != runID || body.Status.State != expectedState {
		t.Fatalf("status = %+v, want %s/%s", body.Status, runID, expectedState)
	}
	if body.Status.StepCount == 0 || body.Status.CurrentStepID == "" || body.Status.CurrentStepState == "" {
		t.Fatalf("status = %+v, want populated step summary", body.Status)
	}
	return body
}

func assertRunStartAvailable(t *testing.T, endpoint string, runID string, expectedState string) workorchRunInstanceResponse {
	t.Helper()

	resp, err := http.Post(endpoint, "application/json", nil)
	if err != nil {
		t.Fatalf("post run start: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("run start status = %d, want 200", resp.StatusCode)
	}
	var body workorchRunInstanceResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode run start response: %v", err)
	}
	if body.Run.ID != runID || body.Run.State != expectedState {
		t.Fatalf("run = %+v, want %s/%s", body.Run, runID, expectedState)
	}
	return body
}

func assertRunResumeAvailable(t *testing.T, endpoint string, runID string, expectedState string) workorchRunInstanceResponse {
	t.Helper()

	resp, err := http.Post(endpoint, "application/json", nil)
	if err != nil {
		t.Fatalf("post run resume: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("run resume status = %d, want 200", resp.StatusCode)
	}
	var body workorchRunInstanceResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode run resume response: %v", err)
	}
	if body.Run.ID != runID || body.Run.State != expectedState {
		t.Fatalf("run = %+v, want %s/%s", body.Run, runID, expectedState)
	}
	return body
}

func assertDecisionListAvailable(t *testing.T, endpoint string, ticketID string) {
	t.Helper()

	resp, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("get decisions: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("decision list status = %d, want 200", resp.StatusCode)
	}
	var body workorchDecisionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode decisions response: %v", err)
	}
	found := false
	for _, item := range body.Decisions {
		if item.ID == ticketID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("decision %q not found in %+v", ticketID, body.Decisions)
	}
}

func assertDecisionAvailable(t *testing.T, endpoint string, ticketID string, expectedState string) workorchDecisionResponse {
	t.Helper()

	resp, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("get decision: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("decision status = %d, want 200", resp.StatusCode)
	}
	var body workorchDecisionResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode decision response: %v", err)
	}
	if body.Decision.ID != ticketID || body.Decision.State != expectedState {
		t.Fatalf("decision = %+v, want %s/%s", body.Decision, ticketID, expectedState)
	}
	return body
}

func assertDecisionResolveAvailable(t *testing.T, endpoint string, ticketID string, resolutionType string) workorchDecisionResponse {
	t.Helper()

	requestBody := strings.NewReader(`{"resolutionType":"` + resolutionType + `"}`)
	resp, err := http.Post(endpoint, "application/json", requestBody)
	if err != nil {
		t.Fatalf("post decision resolve: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("decision resolve status = %d, want 200", resp.StatusCode)
	}
	var body workorchDecisionResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode decision resolve response: %v", err)
	}
	if body.Decision.ID != ticketID || body.Decision.State != "resolved" || body.Decision.ResolutionType != resolutionType {
		t.Fatalf("decision = %+v, want %s/resolved/%s", body.Decision, ticketID, resolutionType)
	}
	return body
}

func assertDecisionRejectAvailable(t *testing.T, endpoint string, ticketID string, resolutionType string) workorchDecisionResponse {
	t.Helper()
	return assertDecisionResolveAvailable(t, endpoint, ticketID, resolutionType)
}

func assertApprovalListAvailable(t *testing.T, endpoint string, approvalID string) {
	t.Helper()

	resp, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("get approvals: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("approval list status = %d, want 200", resp.StatusCode)
	}
	var body workorchApprovalsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode approvals response: %v", err)
	}
	found := false
	for _, item := range body.Approvals {
		if item.ID == approvalID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("approval %q not found in %+v", approvalID, body.Approvals)
	}
}

func assertApprovalAvailable(t *testing.T, endpoint string, approvalID string, expectedState string) workorchApprovalResponse {
	t.Helper()

	resp, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("approval status = %d, want 200", resp.StatusCode)
	}
	var body workorchApprovalResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode approval response: %v", err)
	}
	if body.Approval.ID != approvalID || body.Approval.State != expectedState {
		t.Fatalf("approval = %+v, want %s/%s", body.Approval, approvalID, expectedState)
	}
	return body
}

func assertApprovalApproveAvailable(t *testing.T, endpoint string, approvalID string, expectedState string) workorchApprovalResponse {
	t.Helper()
	return assertApprovalActionAvailable(t, endpoint, approvalID, expectedState)
}

func assertApprovalRejectAvailable(t *testing.T, endpoint string, approvalID string, expectedState string) workorchApprovalResponse {
	t.Helper()
	return assertApprovalActionAvailable(t, endpoint, approvalID, expectedState)
}

func assertApprovalActionAvailable(t *testing.T, endpoint string, approvalID string, expectedState string) workorchApprovalResponse {
	t.Helper()

	resp, err := http.Post(endpoint, "application/json", nil)
	if err != nil {
		t.Fatalf("post approval action: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("approval action status = %d, want 200", resp.StatusCode)
	}
	var body workorchApprovalResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode approval action response: %v", err)
	}
	if body.Approval.ID != approvalID || body.Approval.State != expectedState {
		t.Fatalf("approval = %+v, want %s/%s", body.Approval, approvalID, expectedState)
	}
	return body
}

func assertWorkspaceSnapshotAvailable(t *testing.T, endpoint string, workspaceID string, runIDs []string, decisionIDs []string, approvalIDs []string) {
	t.Helper()

	resp, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("get workspace snapshot: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("workspace snapshot status = %d, want 200", resp.StatusCode)
	}
	var body workorchWorkspaceSnapshotResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode workspace snapshot response: %v", err)
	}
	if body.Snapshot.Workspace.ID != workspaceID {
		t.Fatalf("workspace = %+v, want %s", body.Snapshot.Workspace, workspaceID)
	}
	for _, runID := range runIDs {
		found := false
		for _, item := range body.Snapshot.Runs {
			if item.ID == runID {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("run %q not found in %+v", runID, body.Snapshot.Runs)
		}
	}
	for _, decisionID := range decisionIDs {
		found := false
		for _, item := range body.Snapshot.Decisions {
			if item.ID == decisionID {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("decision %q not found in %+v", decisionID, body.Snapshot.Decisions)
		}
	}
	for _, approvalID := range approvalIDs {
		found := false
		for _, item := range body.Snapshot.Approvals {
			if item.ID == approvalID {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("approval %q not found in %+v", approvalID, body.Snapshot.Approvals)
		}
	}
}

func assertSliceStatusesAvailable(t *testing.T, endpoint string) {
	t.Helper()

	resp, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("get slice statuses: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("slice statuses status = %d, want 200", resp.StatusCode)
	}
	var body workorchSliceStatusesResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode slice statuses response: %v", err)
	}
	if len(body.Statuses) == 0 {
		t.Fatal("expected at least one slice status")
	}
	if body.Statuses[0].ID != "workorch-runnable" || body.Statuses[0].RunState != "completed" {
		t.Fatalf("first status = %+v, want completed workorch-runnable", body.Statuses[0])
	}
}

func assertSliceSnapshotsAvailable(t *testing.T, endpoint string) {
	t.Helper()

	resp, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("get slice snapshots: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("slice snapshots status = %d, want 200", resp.StatusCode)
	}
	var body workorchSliceSnapshotsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode slice snapshots response: %v", err)
	}
	if len(body.Slices) == 0 {
		t.Fatal("expected at least one slice snapshot")
	}
	if body.Slices[0].ID != "workorch-runnable" || body.Slices[0].Snapshot.Slice != "runnable" {
		t.Fatalf("first snapshot = %+v, want runnable workorch-runnable", body.Slices[0])
	}
}

func postRunRequest(t *testing.T, endpoint string) workorchRunResponse {
	t.Helper()

	resp, err := http.Post(endpoint, "application/json", nil)
	if err != nil {
		t.Fatalf("post run request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("run request status = %d, want 200", resp.StatusCode)
	}
	var body workorchRunResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode run response: %v", err)
	}
	if body.Service == "" {
		t.Fatalf("empty run response: %+v", body)
	}
	return body
}

func postSingleSliceRunRequest(t *testing.T, endpoint string) workorchSingleSliceRunResponse {
	t.Helper()

	resp, err := http.Post(endpoint, "application/json", nil)
	if err != nil {
		t.Fatalf("post single slice run request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("single slice run request status = %d, want 200", resp.StatusCode)
	}
	var body workorchSingleSliceRunResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode single slice run response: %v", err)
	}
	if body.Service == "" {
		t.Fatalf("empty single slice run response: %+v", body)
	}
	return body
}

func assertSingleSliceSnapshot(t *testing.T, endpoint string) {
	t.Helper()

	resp, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("get single slice snapshot: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("single slice snapshot status = %d, want 200", resp.StatusCode)
	}
	var body workorchSingleSliceRunResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode single slice snapshot response: %v", err)
	}
	if body.Slice.ID != "workorch-runnable" || body.Slice.Title != "workorch runnable slice" {
		t.Fatalf("single slice snapshot = %+v, want workorch-runnable/workorch runnable slice", body.Slice)
	}
}

func assertSingleSliceStatus(t *testing.T, endpoint string) {
	t.Helper()

	resp, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("get single slice status: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("single slice status status = %d, want 200", resp.StatusCode)
	}
	var body workorchSliceStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode single slice status response: %v", err)
	}
	if body.Status.ID != "workorch-runnable" || body.Status.RunState != "completed" || body.Status.StepState != "completed" {
		t.Fatalf("single slice status = %+v, want completed runnable status", body.Status)
	}
}

func assertConflictAction(t *testing.T, endpoint string, expectedError string) {
	t.Helper()

	resp, err := http.Post(endpoint, "application/json", nil)
	if err != nil {
		t.Fatalf("post conflict action: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("conflict action status = %d, want 409", resp.StatusCode)
	}
	var body workorchErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode conflict action response: %v", err)
	}
	if body.Error != expectedError {
		t.Fatalf("error = %q, want %q", body.Error, expectedError)
	}
}

func TestWorkorchdServerRejectsUnsupportedMethodOnWorkspaceRunCreateEndpoint(t *testing.T) {
	addr := reserveLoopbackAddr(t)
	bin := buildWorkorchdBinary(t)
	cmd := startWorkorchdProcess(t, bin, addr, nil)
	defer stopWorkorchdProcess(t, cmd)

	waitForHealthy(t, "http://"+addr+"/healthz")

	resp, err := http.Get("http://" + addr + "/api/v1/workorch/workspaces/ws-main-memory/runs")
	if err != nil {
		t.Fatalf("get workspace run create endpoint: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
}

func TestWorkorchdServerReadyzReflectsConfiguration(t *testing.T) {
	addr := reserveLoopbackAddr(t)
	bin := buildWorkorchdBinary(t)
	cmd := startWorkorchdProcess(t, bin, addr, []string{
		"repository.backend=file",
		"repository.dataDir=" + filepath.Join(t.TempDir(), "workorch-data"),
	})
	defer stopWorkorchdProcess(t, cmd)

	waitForHealthy(t, "http://"+addr+"/healthz")

	resp, err := http.Get("http://" + addr + "/readyz")
	if err != nil {
		t.Fatalf("get readyz: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func Example_main() {
	fmt.Println("workorchd serves HTTP endpoints: /healthz, /readyz, /metrics, /api/v1/workorch/workspaces, /api/v1/workorch/workspaces/{workspaceId}, /api/v1/workorch/workspaces/{workspaceId}/snapshot, /api/v1/workorch/workspaces/{workspaceId}/runs, /api/v1/workorch/runs, /api/v1/workorch/runs/{runId}, /api/v1/workorch/runs/{runId}/snapshot, /api/v1/workorch/runs/{runId}/status, /api/v1/workorch/runs/{runId}/start, /api/v1/workorch/runs/{runId}/resume, /api/v1/workorch/decisions, /api/v1/workorch/decisions/{ticketId}, /api/v1/workorch/decisions/{ticketId}/resolve, /api/v1/workorch/approvals, /api/v1/workorch/approvals/{approvalId}, /api/v1/workorch/approvals/{approvalId}/approve, /api/v1/workorch/approvals/{approvalId}/reject, /api/v1/workorch/slices, /api/v1/workorch/slices/snapshots, /api/v1/workorch/slices/statuses, /api/v1/workorch/slices/{sliceId}/status, /api/v1/workorch/slices/{sliceId}/snapshot, /api/v1/workorch/slices/{sliceId}/runs")
	// Output: workorchd serves HTTP endpoints: /healthz, /readyz, /metrics, /api/v1/workorch/workspaces, /api/v1/workorch/workspaces/{workspaceId}, /api/v1/workorch/workspaces/{workspaceId}/snapshot, /api/v1/workorch/workspaces/{workspaceId}/runs, /api/v1/workorch/runs, /api/v1/workorch/runs/{runId}, /api/v1/workorch/runs/{runId}/snapshot, /api/v1/workorch/runs/{runId}/status, /api/v1/workorch/runs/{runId}/start, /api/v1/workorch/runs/{runId}/resume, /api/v1/workorch/decisions, /api/v1/workorch/decisions/{ticketId}, /api/v1/workorch/decisions/{ticketId}/resolve, /api/v1/workorch/approvals, /api/v1/workorch/approvals/{approvalId}, /api/v1/workorch/approvals/{approvalId}/approve, /api/v1/workorch/approvals/{approvalId}/reject, /api/v1/workorch/slices, /api/v1/workorch/slices/snapshots, /api/v1/workorch/slices/statuses, /api/v1/workorch/slices/{sliceId}/status, /api/v1/workorch/slices/{sliceId}/snapshot, /api/v1/workorch/slices/{sliceId}/runs
}
