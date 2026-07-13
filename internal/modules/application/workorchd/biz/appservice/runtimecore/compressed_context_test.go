package runtimecore

import (
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

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
	if recoveryMap["blockingTicketId"] != "ticket-core-decision" || recoveryMap["nextAction"] != "resume_run" {
		t.Fatalf("recovery = %+v, want resolved ticket resume", recoveryMap)
	}
}

func TestRunDialogLanguageUsesRuntimeBootstrap(t *testing.T) {
	item := &runmodel.Run{
		RuntimeBootstrap: &runtimecontract.RuntimeBootstrap{Activation: runtimecontract.RuntimeActivationContext{DialogLanguage: "zh-CN"}},
	}
	if got := RunDialogLanguage(item, "en-US"); got != "zh-CN" {
		t.Fatalf("language = %q, want zh-CN", got)
	}
	if got := RunDialogLanguage(&runmodel.Run{}, "en-US"); got != "en-US" {
		t.Fatalf("fallback language = %q, want en-US", got)
	}
}
