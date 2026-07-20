package workorchport

import (
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestNormalizeRunCommandIntent(t *testing.T) {
	for _, tc := range []struct {
		name    string
		input   RunCommandIntent
		want    RunCommandIntent
		wantErr bool
	}{
		{name: "default resume", input: "", want: RunCommandIntentResume},
		{name: "resume", input: RunCommandIntentResume, want: RunCommandIntentResume},
		{name: "restart is historical only", input: RunCommandIntentRestart, wantErr: true},
		{name: "replan is Decision-only", input: RunCommandIntentReplan, wantErr: true},
		{name: "recover is not canonical", input: RunCommandIntent("recover"), wantErr: true},
		{name: "unsupported intent", input: RunCommandIntent("unsupported"), wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeRunCommandIntent(tc.input)
			if tc.wantErr {
				if !errors.Is(err, common.ErrInvalidArgument) {
					t.Fatalf("err = %v, want invalid argument", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeRunCommandIntent: %v", err)
			}
			if got != tc.want {
				t.Fatalf("intent = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestValidateWorkspaceRunCommandScopeContract(t *testing.T) {
	for _, tc := range []struct {
		name    string
		req     WorkspaceRunCommand
		want    RunCommandIntent
		wantErr bool
	}{
		{
			name:    "resume workspace command",
			req:     WorkspaceRunCommand{WorkspaceID: "ws-run-command", Intent: RunCommandIntentResume},
			want:    RunCommandIntentResume,
			wantErr: false,
		},
		{
			name:    "resume scoped run command",
			req:     WorkspaceRunCommand{WorkspaceID: "ws-run-command", ExpectedRunID: "run-command", Intent: RunCommandIntentResume},
			want:    RunCommandIntentResume,
			wantErr: false,
		},
		{name: "restart command is rejected", req: WorkspaceRunCommand{WorkspaceID: "ws-run-command", Intent: RunCommandIntentRestart}, wantErr: true},
		{name: "replan command is rejected", req: WorkspaceRunCommand{WorkspaceID: "ws-run-command", ExpectedRunID: "run-command", Intent: RunCommandIntentReplan}, wantErr: true},
		{
			name:    "workspace required",
			req:     WorkspaceRunCommand{ExpectedRunID: "run-command", Intent: RunCommandIntentResume},
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ValidateWorkspaceRunCommand(tc.req)
			if tc.wantErr {
				if !errors.Is(err, common.ErrInvalidArgument) {
					t.Fatalf("err = %v, want invalid argument", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateWorkspaceRunCommand: %v", err)
			}
			if got != tc.want {
				t.Fatalf("intent = %q, want %q", got, tc.want)
			}
		})
	}
}
