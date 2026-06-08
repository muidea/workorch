package tui

import (
	backend "github.com/muidea/workorch/internal/pkg/workorchport"
)

type WorkspaceListMsg struct {
	Workspaces []backend.WorkspaceInfo
	Error      error
}

type WorkspaceContextMsg struct {
	Workspace        *backend.WorkspaceInfo
	Snapshot         *backend.WorkspaceSnapshotInfo
	Runs             []backend.RunInfo
	ActiveRun        *backend.RunInfo
	DisplayRun       *backend.RunInfo
	Status           *backend.RunStatusInfo
	Goal             *backend.GoalInfo
	DoD              *backend.DoDInfo
	Decisions        []backend.DecisionInfo
	Approvals        []backend.ApprovalInfo
	WorkspaceHasLive bool
	Error            error
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

type DoDUpdatedMsg struct {
	DoD   *backend.DoDInfo
	Error error
}

type RunControlMsg struct {
	Run         *backend.RunInfo
	WorkspaceID string
	Message     string
	Error       error
}

type DecisionResolvedMsg struct {
	Decision *backend.DecisionInfo
	Error    error
}

type ApprovalResolvedMsg struct {
	Approval *backend.ApprovalInfo
	Error    error
}

type ConfigMsg struct {
	Config *backend.ServiceConfig
	Error  error
}
