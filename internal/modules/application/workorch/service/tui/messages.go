package tui

import (
	"time"

	"github.com/muidea/workorch/internal/modules/application/workorch/biz/client"
)

// Messages from background operations to the TUI.

type WorkspaceListMsg struct {
	Workspaces []client.Workspace
	Error      error
}

type DecisionListMsg struct {
	Decisions []client.Decision
	Error     error
}

type ApprovalListMsg struct {
	Approvals []client.Approval
	Error     error
}

type WorkspaceSnapshotMsg struct {
	WorkspaceID string
	Snapshot    *client.WorkspaceSnapshot
	Error       error
}

type RunSnapshotMsg struct {
	RunID    string
	Snapshot *client.RunSnapshot
	Error    error
}

type RunObservabilityMsg struct {
	RunID         string
	Observability *client.RunObservability
	Error         error
}

type RunListMsg struct {
	Runs  []client.Run
	Error error
}

// User actions.

type EnterWorkspaceMsg struct{ WorkspaceID string }
type LeaveWorkspaceMsg struct{}
type NextTicketMsg struct{}
type SelectNextMsg struct{}
type SelectPrevMsg struct{}
type RunActivateMsg struct {
	Goal  string
	Dod   string
	Error error
}
type DecisionResolveMsg struct {
	TicketID string
	Error    error
}
type ApprovalResolveMsg struct {
	ApprovalID string
	Action     string
	Error      error
}

// Internal tick for polling.

type TickMsg time.Time
type ErrorMsg struct{ Error error }

// Alert count per workspace.

type AlertCount struct {
	WorkspaceID    string
	DecisionCount  int
	ApprovalCount  int
	FailedRuns     int
}
