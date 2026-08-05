package workorchport

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRunCommandResultValidatePreservesTypedOwnerAdmission(t *testing.T) {
	valid := RunCommandResult{
		WorkspaceID: "ws-run-command", RunID: "run-command", RequestID: "request-command",
		Intent:  RunCommandIntentResume,
		Outcome: runtimecontract.RuntimeExecutionIntentReconciliationRequested,
		Reason:  runtimecontract.RuntimeExecutionIntentReasonCurrentRunContinuationAccepted,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid run command result: %v", err)
	}

	for name, mutate := range map[string]func(*RunCommandResult){
		"missing workspace": func(result *RunCommandResult) { result.WorkspaceID = "" },
		"missing request":   func(result *RunCommandResult) { result.RequestID = "" },
		"implicit intent":   func(result *RunCommandResult) { result.Intent = "" },
		"invalid pair": func(result *RunCommandResult) {
			result.Outcome = runtimecontract.RuntimeExecutionIntentGovernanceRequired
			result.Reason = runtimecontract.RuntimeExecutionIntentReasonCurrentRunContinuationAccepted
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatalf("invalid result accepted: %+v", candidate)
			}
		})
	}
}

func TestValidateWorkspaceRunCommandRequiresWorkspaceScope(t *testing.T) {
	if _, err := ValidateWorkspaceRunCommand(WorkspaceRunCommand{}); err == nil {
		t.Fatal("workspace-free command was accepted")
	}
	intent, err := ValidateWorkspaceRunCommand(WorkspaceRunCommand{WorkspaceID: common.WorkspaceID("ws-command")})
	if err != nil || intent != RunCommandIntentResume {
		t.Fatalf("normalized command = %q, %v", intent, err)
	}
}
