package tui

import (
	"github.com/muidea/workorch/internal/modules/application/workorch/biz/backend"
)

type WorkspaceListMsg struct {
	Workspaces []backend.WorkspaceInfo
	Error      error
}

type WorkspaceContextMsg struct {
	Workspace *backend.WorkspaceInfo
	Snapshot  *backend.WorkspaceSnapshotInfo
	Runs      []backend.RunInfo
	ActiveRun *backend.RunInfo
	Status    *backend.RunStatusInfo
	Goal      *backend.GoalInfo
	Decisions []backend.DecisionInfo
	Approvals []backend.ApprovalInfo
	Error     error
}

type WorkspaceCreatedMsg struct {
	Workspace *backend.WorkspaceInfo
	Error     error
}

type DecisionListMsg struct {
	Decisions []backend.DecisionInfo
	Error     error
}

type DecisionDetailMsg struct {
	Decision *backend.DecisionInfo
	Error    error
}

type ApprovalListMsg struct {
	Approvals []backend.ApprovalInfo
	Error     error
}

type ApprovalDetailMsg struct {
	Approval *backend.ApprovalInfo
	Error    error
}

type ErrorMsg struct{ Error error }

type BackendEventMsg struct {
	Event backend.Event
}

type RunStartedMsg struct {
	Run   *backend.RunInfo
	Error error
}

type DecisionResolvedMsg struct {
	Decision *backend.DecisionInfo
	Error    error
}

type ApprovalResolvedMsg struct {
	Approval *backend.ApprovalInfo
	Error    error
}
