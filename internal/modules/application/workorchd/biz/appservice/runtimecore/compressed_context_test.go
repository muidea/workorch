package runtimecore

import (
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestCreatedRunCompressedContext(t *testing.T) {
	compressed := CreatedRunCompressedContext(&runmodel.Run{
		ID:          "run-core-created",
		WorkspaceID: "ws-core-created",
		State:       common.RunStateCreated,
	}, "en-US")
	if compressed == nil {
		t.Fatal("compressed context nil, want created-run projection")
	}
	if compressed["version"] != "workorch.compressedContext.v1" ||
		compressed["mode"] != "deterministic_recovery_summary" ||
		compressed["runId"] != "run-core-created" ||
		compressed["workspaceId"] != "ws-core-created" {
		t.Fatalf("compressed = %+v, want created run identity", compressed)
	}
	if compressed["currentMilestone"] != "execution_steps" {
		t.Fatalf("currentMilestone = %v, want execution_steps", compressed["currentMilestone"])
	}
	if running := CreatedRunCompressedContext(&runmodel.Run{
		ID:          "run-core-running",
		WorkspaceID: "ws-core-running",
		State:       common.RunStateRunning,
	}, "en-US"); running != nil {
		t.Fatalf("running compressed = %+v, want nil", running)
	}
}

func TestBlockedRunResumeCompressedContext(t *testing.T) {
	item := &runmodel.Run{
		ID:               "run-core-decision",
		WorkspaceID:      "ws-core-decision",
		State:            common.RunStateBlockedOnDecision,
		BlockingTicketID: "ticket-core-decision",
	}
	blocker, recovery, summary := ResolvedDecisionBlockerContext(item, "en-US")
	compressed := BlockedRunResumeCompressedContext(item, blocker, recovery, summary, "en-US")
	if compressed == nil {
		t.Fatal("compressed context nil, want blocked-run resume projection")
	}
	if compressed["runId"] != "run-core-decision" ||
		compressed["workspaceId"] != "ws-core-decision" ||
		compressed["currentMilestone"] != "execution_steps" {
		t.Fatalf("compressed = %+v, want run identity and execution milestone", compressed)
	}
	recoveryMap, _ := compressed["recovery"].(map[string]any)
	if recoveryMap["blockingTicketId"] != "ticket-core-decision" || recoveryMap["nextAction"] != "continue_run" {
		t.Fatalf("recovery = %+v, want resolved ticket resume", recoveryMap)
	}
}

func TestRunDialogLanguageUsesExecutionContext(t *testing.T) {
	item := &runmodel.Run{
		ExecutionContext: map[string]any{
			"dialogLanguage": "zh-CN",
		},
	}
	if got := RunDialogLanguage(item, "en-US"); got != "zh-CN" {
		t.Fatalf("language = %q, want zh-CN", got)
	}
	if got := RunDialogLanguage(&runmodel.Run{}, "en-US"); got != "en-US" {
		t.Fatalf("fallback language = %q, want en-US", got)
	}
}
