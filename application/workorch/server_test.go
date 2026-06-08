package main

import (
	"context"
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
	"sync/atomic"
	"testing"
	"time"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
)

type workorchWorkspaceResponse struct {
	Service   string `json:"service"`
	Workspace struct {
		ID                   string `json:"id"`
		Title                string `json:"title"`
		Description          string `json:"description"`
		DeliverableDirectory string `json:"deliverableDirectory"`
		State                string `json:"state"`
		ActiveRunID          string `json:"activeRunId"`
	} `json:"workspace"`
}

type workorchWorkspacesResponse struct {
	Service    string `json:"service"`
	Workspaces []struct {
		ID                   string `json:"id"`
		Title                string `json:"title"`
		Description          string `json:"description"`
		DeliverableDirectory string `json:"deliverableDirectory"`
		State                string `json:"state"`
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

var testPortCounter atomic.Uint32

func TestWorkorchServerModeServesHealthAndRunEndpoints(t *testing.T) {
	addr := reserveLoopbackAddr(t)
	bin := buildWorkorchBinary(t)
	cmd := startWorkorchServerProcess(t, bin, addr, nil)
	defer stopWorkorchProcess(t, cmd)

	waitForHealthy(t, "http://"+addr+"/healthz")
	assertMetricsAvailable(t, "http://"+addr+"/metrics")
	createWorkspace(t, "http://"+addr+"/api/v1/workorch/workspaces", "main memory workspace")
}

func TestWorkorchServerModeSupportsFileRepositoryBackend(t *testing.T) {
	addr := reserveLoopbackAddr(t)
	dataDir := filepath.Join(t.TempDir(), "workorch-data")
	bin := buildWorkorchBinary(t)
	cmd := startWorkorchServerProcess(t, bin, addr, []string{
		"repository.backend=file",
		"repository.dataDir=" + dataDir,
	})
	defer stopWorkorchProcess(t, cmd)

	waitForHealthy(t, "http://"+addr+"/healthz")

	workspaceID := createWorkspace(t, "http://"+addr+"/api/v1/workorch/workspaces", "main file workspace")
	assertWorkspaceLayoutDirectories(t, dataDir, workspaceID)
	createRunInstance(t, "http://"+addr+"/api/v1/workorch/workspaces/"+workspaceID+"/runs", "run-main-file", "goal-main-file", "dod-main-file")
	assertWorkspaceListAvailable(t, "http://"+addr+"/api/v1/workorch/workspaces", workspaceID)
	assertWorkspaceAvailable(t, "http://"+addr+"/api/v1/workorch/workspaces/"+workspaceID, workspaceID, "main file workspace", "active")
	assertRunListAvailable(t, "http://"+addr+"/api/v1/workorch/runs", "run-main-file")
	assertRunAvailable(t, "http://"+addr+"/api/v1/workorch/workspaces/"+workspaceID+"/runs/run-main-file", "run-main-file", workspaceID)
	assertRunListAvailable(t, "http://"+addr+"/api/v1/workorch/runs?workspaceId="+workspaceID, "run-main-file")
	assertRunStartAvailable(t, "http://"+addr+"/api/v1/workorch/workspaces/"+workspaceID+"/runs/run-main-file/start", "run-main-file", "running")
	blockedProviderRun := waitForRunStatusAvailable(t, "http://"+addr+"/api/v1/workorch/workspaces/"+workspaceID+"/runs/run-main-file/status", "run-main-file", "blocked_on_decision")
	if blockedProviderRun.Status.BlockingTicketID == "" {
		t.Fatalf("blocked provider run ticket = %q, want non-empty", blockedProviderRun.Status.BlockingTicketID)
	}
	assertRunSnapshotAvailableWithState(t, "http://"+addr+"/api/v1/workorch/workspaces/"+workspaceID+"/runs/run-main-file/snapshot", "run-main-file", workspaceID, "blocked_on_decision")
	assertRunStatusAvailable(t, "http://"+addr+"/api/v1/workorch/workspaces/"+workspaceID+"/runs/run-main-file/status", "run-main-file", "blocked_on_decision")

	decisionResolveRunID := "run-decision-main-file"
	blockedDecisionID := seedBlockedDecisionRunForProcess(t, dataDir, workspaceID, decisionResolveRunID)
	assertRunStatusAvailable(t, "http://"+addr+"/api/v1/workorch/workspaces/"+workspaceID+"/runs/"+decisionResolveRunID+"/status", decisionResolveRunID, "blocked_on_decision")
	assertDecisionListAvailable(t, "http://"+addr+"/api/v1/workorch/decisions", blockedDecisionID)
	assertDecisionAvailable(t, "http://"+addr+"/api/v1/workorch/decisions/"+blockedDecisionID, blockedDecisionID, "open")
	assertDecisionListAvailable(t, "http://"+addr+"/api/v1/workorch/decisions?workspaceId="+workspaceID, blockedDecisionID)
	assertDecisionListAvailable(t, "http://"+addr+"/api/v1/workorch/decisions?runId="+decisionResolveRunID, blockedDecisionID)
	assertDecisionResolveAvailable(t, "http://"+addr+"/api/v1/workorch/decisions/"+blockedDecisionID+"/resolve", blockedDecisionID, decisioncommon.ResolutionTypeProvideMissingInfo)

	decisionRejectRunID := "run-decision-rejected-main-file"
	rejectedDecisionID := seedBlockedDecisionRunForProcess(t, dataDir, workspaceID, decisionRejectRunID)
	assertDecisionRejectAvailable(t, "http://"+addr+"/api/v1/workorch/decisions/"+rejectedDecisionID+"/resolve", rejectedDecisionID, decisioncommon.ResolutionTypeRejectCurrentPath)
	rejectedRun := waitForRunStatusAvailable(t, "http://"+addr+"/api/v1/workorch/workspaces/"+workspaceID+"/runs/"+decisionRejectRunID+"/status", decisionRejectRunID, "failed")
	if rejectedRun.Status.FailureReason == "" {
		t.Fatalf("rejected run failureReason = %q, want non-empty", rejectedRun.Status.FailureReason)
	}

	approvalApproveRunID := "run-approval-main-file"
	blockedApprovalID := seedBlockedApprovalRunForProcess(t, dataDir, workspaceID, approvalApproveRunID)
	assertApprovalListAvailable(t, "http://"+addr+"/api/v1/workorch/approvals", blockedApprovalID)
	assertApprovalAvailable(t, "http://"+addr+"/api/v1/workorch/approvals/"+blockedApprovalID, blockedApprovalID, "pending")
	assertApprovalListAvailable(t, "http://"+addr+"/api/v1/workorch/approvals?workspaceId="+workspaceID, blockedApprovalID)
	assertApprovalListAvailable(t, "http://"+addr+"/api/v1/workorch/approvals?runId="+approvalApproveRunID, blockedApprovalID)
	assertApprovalApproveAvailable(t, "http://"+addr+"/api/v1/workorch/approvals/"+blockedApprovalID+"/approve", blockedApprovalID, "approved")

	approvalRejectRunID := "run-approval-rejected-main-file"
	rejectedApprovalID := seedBlockedApprovalRunForProcess(t, dataDir, workspaceID, approvalRejectRunID)
	assertApprovalRejectAvailable(t, "http://"+addr+"/api/v1/workorch/approvals/"+rejectedApprovalID+"/reject", rejectedApprovalID, "rejected")
	rejectedApprovalRun := waitForRunStatusAvailable(t, "http://"+addr+"/api/v1/workorch/workspaces/"+workspaceID+"/runs/"+approvalRejectRunID+"/status", approvalRejectRunID, "failed")
	if rejectedApprovalRun.Status.FailureReason == "" {
		t.Fatalf("rejected approval run failureReason = %q, want non-empty", rejectedApprovalRun.Status.FailureReason)
	}
	assertWorkspaceSnapshotAvailable(
		t,
		"http://"+addr+"/api/v1/workorch/workspaces/"+workspaceID+"/snapshot",
		workspaceID,
		[]string{"run-main-file", decisionResolveRunID, decisionRejectRunID, approvalApproveRunID, approvalRejectRunID},
		[]string{blockedProviderRun.Status.BlockingTicketID, blockedDecisionID, rejectedDecisionID},
		[]string{blockedApprovalID, rejectedApprovalID},
	)

	for _, name := range []string{"workspace.json", "goals.json", "dods.json"} {
		if _, err := os.Stat(filepath.Join(dataDir, "internal", "workspaces", workspaceID, name)); err != nil {
			t.Fatalf("expected workspace-scoped persisted file %s: %v", name, err)
		}
	}
	for _, runID := range []string{"run-main-file", decisionResolveRunID, decisionRejectRunID, approvalApproveRunID, approvalRejectRunID} {
		if _, err := os.Stat(filepath.Join(dataDir, "internal", "workspaces", workspaceID, "runs", runID, "run.json")); err != nil {
			t.Fatalf("expected run-scoped persistence for %s: %v", runID, err)
		}
	}
}

func TestWorkorchServerModeRetainsFileBackedTerminalStateAcrossRestart(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "workorch-restart-data")
	bin := buildWorkorchBinary(t)
	env := []string{
		"repository.backend=file",
		"repository.dataDir=" + dataDir,
	}

	firstAddr := reserveLoopbackAddr(t)
	first := startWorkorchServerProcess(t, bin, firstAddr, env)
	waitForHealthy(t, "http://"+firstAddr+"/healthz")

	workspaceID := createWorkspace(t, "http://"+firstAddr+"/api/v1/workorch/workspaces", "restart file workspace")
	assertWorkspaceLayoutDirectories(t, dataDir, workspaceID)

	createRunInstance(t, "http://"+firstAddr+"/api/v1/workorch/workspaces/"+workspaceID+"/runs", "run-restart-completed", "", "")
	assertRunStartAvailable(t, "http://"+firstAddr+"/api/v1/workorch/workspaces/"+workspaceID+"/runs/run-restart-completed/start", "run-restart-completed", "running")
	blockedProviderRun := waitForRunStatusAvailable(t, "http://"+firstAddr+"/api/v1/workorch/workspaces/"+workspaceID+"/runs/run-restart-completed/status", "run-restart-completed", "blocked_on_decision")

	decisionRunID := "run-decision-restart-resolved"
	blockedDecisionID := seedBlockedDecisionRunForProcess(t, dataDir, workspaceID, decisionRunID)
	assertDecisionResolveAvailable(t, "http://"+firstAddr+"/api/v1/workorch/decisions/"+blockedDecisionID+"/resolve", blockedDecisionID, decisioncommon.ResolutionTypeProvideMissingInfo)
	assertRunStatusAvailable(t, "http://"+firstAddr+"/api/v1/workorch/workspaces/"+workspaceID+"/runs/"+decisionRunID+"/status", decisionRunID, "blocked_on_decision")

	approvalRunID := "run-approval-restart-failed"
	blockedApprovalID := seedBlockedApprovalRunForProcess(t, dataDir, workspaceID, approvalRunID)
	assertApprovalRejectAvailable(t, "http://"+firstAddr+"/api/v1/workorch/approvals/"+blockedApprovalID+"/reject", blockedApprovalID, "rejected")
	waitForRunStatusAvailable(t, "http://"+firstAddr+"/api/v1/workorch/workspaces/"+workspaceID+"/runs/"+approvalRunID+"/status", approvalRunID, "failed")

	stopWorkorchProcess(t, first)

	secondAddr := reserveLoopbackAddr(t)
	second := startWorkorchServerProcess(t, bin, secondAddr, env)
	defer stopWorkorchProcess(t, second)

	waitForHealthy(t, "http://"+secondAddr+"/healthz")

	assertWorkspaceAvailable(t, "http://"+secondAddr+"/api/v1/workorch/workspaces/"+workspaceID, workspaceID, "restart file workspace", "waiting_decision")
	assertRunAvailableWithState(t, "http://"+secondAddr+"/api/v1/workorch/workspaces/"+workspaceID+"/runs/run-restart-completed", "run-restart-completed", workspaceID, "blocked_on_decision")
	assertRunSnapshotAvailableWithState(t, "http://"+secondAddr+"/api/v1/workorch/workspaces/"+workspaceID+"/runs/run-restart-completed/snapshot", "run-restart-completed", workspaceID, "blocked_on_decision")
	assertRunStatusAvailable(t, "http://"+secondAddr+"/api/v1/workorch/workspaces/"+workspaceID+"/runs/run-restart-completed/status", "run-restart-completed", "blocked_on_decision")

	assertRunAvailableWithState(t, "http://"+secondAddr+"/api/v1/workorch/workspaces/"+workspaceID+"/runs/"+decisionRunID, decisionRunID, workspaceID, "blocked_on_decision")
	assertRunSnapshotAvailableWithState(t, "http://"+secondAddr+"/api/v1/workorch/workspaces/"+workspaceID+"/runs/"+decisionRunID+"/snapshot", decisionRunID, workspaceID, "blocked_on_decision")
	assertDecisionAvailable(t, "http://"+secondAddr+"/api/v1/workorch/decisions/"+blockedDecisionID, blockedDecisionID, "resolved")

	failedStatus := assertRunStatusAvailable(t, "http://"+secondAddr+"/api/v1/workorch/workspaces/"+workspaceID+"/runs/"+approvalRunID+"/status", approvalRunID, "failed")
	if failedStatus.Status.FailureReason == "" {
		t.Fatalf("failed status = %+v, want non-empty failure reason", failedStatus.Status)
	}
	assertRunAvailableWithState(t, "http://"+secondAddr+"/api/v1/workorch/workspaces/"+workspaceID+"/runs/"+approvalRunID, approvalRunID, workspaceID, "failed")
	assertRunSnapshotAvailableWithState(t, "http://"+secondAddr+"/api/v1/workorch/workspaces/"+workspaceID+"/runs/"+approvalRunID+"/snapshot", approvalRunID, workspaceID, "failed")
	assertApprovalAvailable(t, "http://"+secondAddr+"/api/v1/workorch/approvals/"+blockedApprovalID, blockedApprovalID, "rejected")

	assertWorkspaceSnapshotAvailable(
		t,
		"http://"+secondAddr+"/api/v1/workorch/workspaces/"+workspaceID+"/snapshot",
		workspaceID,
		[]string{"run-restart-completed", decisionRunID, approvalRunID},
		[]string{blockedProviderRun.Status.BlockingTicketID, blockedDecisionID},
		[]string{blockedApprovalID},
	)
	assertConflictAction(t, "http://"+secondAddr+"/api/v1/workorch/workspaces/"+workspaceID+"/runs/run-restart-completed/start", "conflict")
	assertConflictAction(t, "http://"+secondAddr+"/api/v1/workorch/workspaces/"+workspaceID+"/runs/"+approvalRunID+"/resume", "conflict")
	assertConflictAction(t, "http://"+secondAddr+"/api/v1/workorch/workspaces/"+workspaceID+"/runs/"+approvalRunID+"/start", "conflict")
}

func TestWorkorchServerModeFailsWithUnsupportedRepositoryBackend(t *testing.T) {
	addr := reserveLoopbackAddr(t)
	bin := buildWorkorchBinary(t)
	configDir := writeWorkorchTestConfig(t, addr, []string{
		"repository.backend=unsupported",
		"repository.dataDir=" + t.TempDir(),
	})
	cmd := exec.Command(bin, "--server", "-config-dir", configDir)
	cmd.Env = os.Environ()

	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected workorch --server to fail\noutput:\n%s", string(out))
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

func TestWorkorchServerModeFailsWithInvalidServerTimeout(t *testing.T) {
	addr := reserveLoopbackAddr(t)
	bin := buildWorkorchBinary(t)
	configDir := writeWorkorchTestConfig(t, addr, []string{
		"server.readTimeout=not-a-duration",
	})
	cmd := exec.Command(bin, "--server", "-config-dir", configDir)
	cmd.Env = os.Environ()

	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected workorch --server to fail with invalid server timeout\noutput:\n%s", string(out))
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

	// Avoid :0 here. It returns an ephemeral port that the health-check client
	// can immediately reuse as a local source port before the child process
	// binds, making the child fail with "address already in use".
	base := 18000 + int(testPortCounter.Add(1)%4000)
	for offset := 0; offset < 4000; offset++ {
		port := 18000 + ((base + offset) % 4000)
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		ln, err := net.Listen("tcp", addr)
		if err == nil {
			_ = ln.Close()
			return addr
		}
	}
	t.Fatal("no available loopback test port")
	return ""
}

func buildWorkorchBinary(t *testing.T) string {
	t.Helper()

	bin := filepath.Join(t.TempDir(), "workorch")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build workorch: %v\noutput:\n%s", err, string(out))
	}
	return bin
}

func startWorkorchServerProcess(t *testing.T, bin string, addr string, extraEnv []string) *exec.Cmd {
	t.Helper()

	configDir := writeWorkorchTestConfig(t, addr, extraEnv)
	cmd := exec.Command(bin, "--server", "-config-dir", configDir)
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		t.Fatalf("start workorch --server: %v", err)
	}
	return cmd
}

func writeWorkorchTestConfig(t *testing.T, addr string, extraEnv []string) string {
	t.Helper()

	values := map[string]string{
		"repository.backend": "file",
		"repository.dataDir": filepath.Join(t.TempDir(), ".workorch-data"),
		"server.readTimeout": "15s",
	}
	for _, item := range extraEnv {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			values[key] = value
		}
	}
	configDir := t.TempDir()
	content := fmt.Sprintf(`endpointName = "workorch"

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
		t.Fatalf("write workorch config: %v", err)
	}
	return configDir
}

func stopWorkorchProcess(t *testing.T, cmd *exec.Cmd) {
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
			t.Fatalf("wait workorch: %v", err)
		}
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("timed out waiting for workorch to exit")
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
	if payload.Workspace.ID == "" || payload.Workspace.Title != title || payload.Workspace.Description != description || payload.Workspace.DeliverableDirectory == "" || payload.Workspace.State != "active" {
		t.Fatalf("created workspace = %+v, want generated/%s/%s/non-empty deliverable directory/active", payload.Workspace, title, description)
	}
	return payload.Workspace.ID
}

func assertWorkspaceLayoutDirectories(t *testing.T, dataDir string, workspaceID string) {
	t.Helper()
	for _, dir := range []string{
		filepath.Join(filepath.Dir(dataDir), ".workorch-workspaces", workspaceID),
		filepath.Join(dataDir, "internal", "workspaces", workspaceID),
		filepath.Join(dataDir, "internal", "workspaces", workspaceID, "artifacts"),
		filepath.Join(dataDir, "internal", "workspaces", workspaceID, "runs"),
	} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("workspace directory %q = %v/%v, want existing directory", dir, info, err)
		}
	}
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

func assertWorkspaceAvailable(t *testing.T, endpoint string, workspaceID string, title string, state string) {
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
	if body.Workspace.ID != workspaceID || body.Workspace.Title != title || body.Workspace.State != state {
		t.Fatalf("workspace = %+v, want %s/%s/%s", body.Workspace, workspaceID, title, state)
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

func seedBlockedDecisionRunForProcess(t *testing.T, dataRoot string, workspaceID string, runID string) string {
	t.Helper()

	ticketID := common.TicketID("ticket-" + runID + "-decision")
	stepID := common.StepID(runID + "/decision")
	ctx := context.Background()
	runScopedRepos := workorchrepo.NewWorkspaceScopedFileRepositorySet(dataRoot)
	runRepo := runScopedRepos.Runs
	stepRepo := runScopedRepos.Steps
	ticketRepo := runScopedRepos.Tickets

	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:               common.RunID(runID),
		WorkspaceID:      common.WorkspaceID(workspaceID),
		State:            common.RunStateBlockedOnDecision,
		BlockingTicketID: ticketID,
		InitialStepKind:  llmcommon.StepKindWorkspaceExecution,
	}); err != nil {
		t.Fatalf("seed decision run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:     stepID,
		RunID:  common.RunID(runID),
		Kind:   llmcommon.StepKindWorkspaceExecution,
		State:  common.StepStateCompleted,
		Output: map[string]any{"stage": "blocked_on_decision"},
	}); err != nil {
		t.Fatalf("seed decision step: %v", err)
	}
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{
		ID:           ticketID,
		WorkspaceID:  common.WorkspaceID(workspaceID),
		RunID:        common.RunID(runID),
		StepID:       stepID,
		Reason:       "process test decision gate",
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Question:     "provide process test input",
		Context:      map[string]any{"requiredFields": []any{"input"}},
		State:        common.DecisionTicketStateOpen,
	}); err != nil {
		t.Fatalf("seed decision ticket: %v", err)
	}
	return string(ticketID)
}

func seedBlockedApprovalRunForProcess(t *testing.T, dataRoot string, workspaceID string, runID string) string {
	t.Helper()

	approvalID := common.ApprovalID("approval-" + runID + "-approval")
	stepID := common.StepID(runID + "/approval")
	ctx := context.Background()
	runScopedRepos := workorchrepo.NewWorkspaceScopedFileRepositorySet(dataRoot)
	runRepo := runScopedRepos.Runs
	stepRepo := runScopedRepos.Steps
	approvalRepo := runScopedRepos.Approvals

	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:                 common.RunID(runID),
		WorkspaceID:        common.WorkspaceID(workspaceID),
		State:              common.RunStateBlockedOnApproval,
		BlockingApprovalID: approvalID,
		InitialStepKind:    llmcommon.StepKindWorkspaceExecution,
	}); err != nil {
		t.Fatalf("seed approval run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:     stepID,
		RunID:  common.RunID(runID),
		Kind:   llmcommon.StepKindWorkspaceExecution,
		State:  common.StepStateCompleted,
		Output: map[string]any{"stage": "blocked_on_approval"},
	}); err != nil {
		t.Fatalf("seed approval step: %v", err)
	}
	if err := approvalRepo.Create(ctx, &approvalmodel.Approval{
		ID:          approvalID,
		WorkspaceID: common.WorkspaceID(workspaceID),
		RunID:       common.RunID(runID),
		StepID:      stepID,
		Reason:      "process test approval gate",
		State:       common.ApprovalStatePending,
	}); err != nil {
		t.Fatalf("seed approval: %v", err)
	}
	return string(approvalID)
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
	assertRunSnapshot(t, endpoint, runID, workspaceID, "", true)
}

func assertRunSnapshotAvailableWithState(t *testing.T, endpoint string, runID string, workspaceID string, expectedState string) workorchRunSnapshotResponse {
	t.Helper()
	return assertRunSnapshot(t, endpoint, runID, workspaceID, expectedState, false)
}

func assertRunSnapshot(t *testing.T, endpoint string, runID string, workspaceID string, expectedState string, requireObserved bool) workorchRunSnapshotResponse {
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
	if len(body.Snapshot.Steps) == 0 {
		t.Fatalf("snapshot = %+v, want steps", body.Snapshot)
	}
	if requireObserved && len(body.Snapshot.Observed) == 0 {
		t.Fatalf("snapshot = %+v, want observed items", body.Snapshot)
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

func waitForRunStatusAvailable(t *testing.T, endpoint string, runID string, expectedState string) workorchRunStatusResponse {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	var last workorchRunStatusResponse
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := http.Get(endpoint)
		if err != nil {
			lastErr = err
			time.Sleep(20 * time.Millisecond)
			continue
		}
		var body workorchRunStatusResponse
		decodeErr := json.NewDecoder(resp.Body).Decode(&body)
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusOK && decodeErr == nil {
			last = body
			if body.Status.ID == runID && body.Status.State == expectedState {
				return body
			}
		} else if decodeErr != nil {
			lastErr = decodeErr
		}
		time.Sleep(20 * time.Millisecond)
	}
	if lastErr != nil {
		t.Fatalf("run %s did not reach %s: last error %v", runID, expectedState, lastErr)
	}
	t.Fatalf("run %s status = %+v, want %s", runID, last.Status, expectedState)
	return last
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

func TestWorkorchServerModeListsWorkspaceRunsEndpoint(t *testing.T) {
	addr := reserveLoopbackAddr(t)
	bin := buildWorkorchBinary(t)
	cmd := startWorkorchServerProcess(t, bin, addr, nil)
	defer stopWorkorchProcess(t, cmd)

	waitForHealthy(t, "http://"+addr+"/healthz")

	resp, err := http.Get("http://" + addr + "/api/v1/workorch/workspaces/ws-main-memory/runs")
	if err != nil {
		t.Fatalf("get workspace runs endpoint: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body workorchRunsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode workspace runs response: %v", err)
	}
	if body.Service != "workorch" {
		t.Fatalf("service = %q, want workorch", body.Service)
	}
}

func TestWorkorchServerModeReadyzReflectsConfiguration(t *testing.T) {
	addr := reserveLoopbackAddr(t)
	bin := buildWorkorchBinary(t)
	cmd := startWorkorchServerProcess(t, bin, addr, []string{
		"repository.backend=file",
		"repository.dataDir=" + filepath.Join(t.TempDir(), "workorch-data"),
	})
	defer stopWorkorchProcess(t, cmd)

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
	fmt.Println("workorch --server serves HTTP endpoints: /healthz, /readyz, /metrics, /api/v1/workorch/workspaces, /api/v1/workorch/workspaces/{workspaceId}, /api/v1/workorch/workspaces/{workspaceId}/snapshot, /api/v1/workorch/workspaces/{workspaceId}/runs, /api/v1/workorch/workspaces/{workspaceId}/runs/{runId}, /api/v1/workorch/workspaces/{workspaceId}/runs/{runId}/snapshot, /api/v1/workorch/workspaces/{workspaceId}/runs/{runId}/status, /api/v1/workorch/workspaces/{workspaceId}/runs/{runId}/observability, /api/v1/workorch/workspaces/{workspaceId}/runs/{runId}/start, /api/v1/workorch/workspaces/{workspaceId}/runs/{runId}/resume, /api/v1/workorch/decisions, /api/v1/workorch/decisions/{ticketId}, /api/v1/workorch/decisions/{ticketId}/resolve, /api/v1/workorch/approvals, /api/v1/workorch/approvals/{approvalId}, /api/v1/workorch/approvals/{approvalId}/approve, /api/v1/workorch/approvals/{approvalId}/reject")
	// Output: workorch --server serves HTTP endpoints: /healthz, /readyz, /metrics, /api/v1/workorch/workspaces, /api/v1/workorch/workspaces/{workspaceId}, /api/v1/workorch/workspaces/{workspaceId}/snapshot, /api/v1/workorch/workspaces/{workspaceId}/runs, /api/v1/workorch/workspaces/{workspaceId}/runs/{runId}, /api/v1/workorch/workspaces/{workspaceId}/runs/{runId}/snapshot, /api/v1/workorch/workspaces/{workspaceId}/runs/{runId}/status, /api/v1/workorch/workspaces/{workspaceId}/runs/{runId}/observability, /api/v1/workorch/workspaces/{workspaceId}/runs/{runId}/start, /api/v1/workorch/workspaces/{workspaceId}/runs/{runId}/resume, /api/v1/workorch/decisions, /api/v1/workorch/decisions/{ticketId}, /api/v1/workorch/decisions/{ticketId}/resolve, /api/v1/workorch/approvals, /api/v1/workorch/approvals/{approvalId}, /api/v1/workorch/approvals/{approvalId}/approve, /api/v1/workorch/approvals/{approvalId}/reject
}
