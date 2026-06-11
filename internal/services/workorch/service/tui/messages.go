package tui

import (
	backend "github.com/muidea/workorch/internal/pkg/workorchport"
)

type WorkspaceListMsg struct {
	RequestSeq int64
	Projection backend.ProjectionInfo
	Workspaces []backend.WorkspaceInfo
	Error      error
}

type WorkspaceContextMsg struct {
	RequestSeq        int64
	WorkspaceID       string
	ResetMainViewport bool
	Workspace         *backend.WorkspaceInfo
	Projection        backend.ProjectionInfo
	Snapshot          *backend.WorkspaceSnapshotInfo
	Runs              []backend.RunInfo
	ActiveRun         *backend.RunInfo
	DisplayRun        *backend.RunInfo
	Status            *backend.RunStatusInfo
	Goal              *backend.GoalInfo
	DoD               *backend.DoDInfo
	Decisions         []backend.DecisionInfo
	Approvals         []backend.ApprovalInfo
	WorkspaceHasLive  bool
	Error             error
}

type WorkspaceContextRetryMsg struct {
	WorkspaceID       string
	ResetMainViewport bool
}

type WorkspaceCreatedMsg struct {
	Workspace *backend.WorkspaceInfo
	Error     error
}

type DecisionListMsg struct {
	WorkspaceID string
	Decisions   []backend.DecisionInfo
	Error       error
}

type DecisionDetailMsg struct {
	WorkspaceID string
	Decision    *backend.DecisionInfo
	Error       error
}

type ApprovalListMsg struct {
	WorkspaceID string
	Approvals   []backend.ApprovalInfo
	Error       error
}

type ApprovalDetailMsg struct {
	WorkspaceID string
	Approval    *backend.ApprovalInfo
	Error       error
}

type ErrorMsg struct{ Error error }

type BackendEventMsg struct {
	Event backend.Event
}

type RunStartedMsg struct {
	Run         *backend.RunInfo
	WorkspaceID string
	Error       error
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
