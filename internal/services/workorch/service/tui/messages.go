package tui

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	backend "github.com/muidea/workorch/internal/pkg/workorchport"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type WorkspaceListMsg struct {
	RequestSeq int64
	Projection projectioncontract.ProjectionStatus
	Workspaces []projectioncontract.Workspace
	Error      error
}

type WorkspaceListRetryMsg struct{}

type WorkspaceContextMsg struct {
	RequestSeq        int64
	WorkspaceID       string
	ResetMainViewport bool
	Workspace         *projectioncontract.Workspace
	Projection        projectioncontract.ProjectionStatus
	Snapshot          *projectioncontract.WorkspaceSnapshot
	RuntimeView       *projectioncontract.WorkspaceRuntimeView
	Runs              []projectioncontract.Run
	ActiveRun         *projectioncontract.Run
	DisplayRun        *projectioncontract.Run
	Status            *projectioncontract.RunStatus
	Goal              *workorchmanagementcontract.Goal
	DoD               *workorchmanagementcontract.DoD
	Decisions         []projectioncontract.Decision
	Approvals         []projectioncontract.Approval
	WorkspaceHasLive  bool
	Error             error
}

// WorkspaceRuntimeViewMsg carries the lightweight runtime projection used for
// active refresh and history recovery after switch/reconnect.
type WorkspaceRuntimeViewMsg struct {
	RequestSeq        int64
	WorkspaceID       string
	ResetMainViewport bool
	View              *projectioncontract.WorkspaceRuntimeView
	Error             error
}

type WorkspaceContextRetryMsg struct {
	WorkspaceID       string
	ResetMainViewport bool
}

type WorkspaceCreatedMsg struct {
	Workspace *projectioncontract.Workspace
	Error     error
}

type DecisionListMsg struct {
	WorkspaceID string
	Decisions   []projectioncontract.Decision
	Error       error
}

type DecisionDetailMsg struct {
	WorkspaceID string
	Decision    *projectioncontract.Decision
	Error       error
}

type ApprovalListMsg struct {
	WorkspaceID string
	Approvals   []projectioncontract.Approval
	Error       error
}

type ApprovalDetailMsg struct {
	WorkspaceID string
	Approval    *projectioncontract.Approval
	Error       error
}

type ErrorMsg struct{ Error error }

type WorkspaceProjectionChangedMsg struct {
	Info projectioncontract.WorkspaceProjectionChanged
}

type SubscriptionStatusMsg struct {
	Status backend.SubscriptionStatus
}

type RunStartedMsg struct {
	Admission   *backend.RunCommandResult
	Run         *projectioncontract.Run
	WorkspaceID string
	Error       error
}

type DoDUpdatedMsg struct {
	DoD   *workorchmanagementcontract.DoD
	Error error
}

type RunControlMsg struct {
	Admission   *backend.RunCommandResult
	Run         *projectioncontract.Run
	WorkspaceID string
	Message     string
	Error       error
}

type DecisionResolvedMsg struct {
	WorkspaceID common.WorkspaceID
	Decision    *projectioncontract.Decision
	Error       error
}

type ApprovalResolvedMsg struct {
	WorkspaceID common.WorkspaceID
	Approval    *projectioncontract.Approval
	Error       error
}

type ConfigMsg struct {
	Config *backend.ServiceConfig
	Error  error
}

type ChatModelTargetsLoadedMsg struct {
	RequestSeq  int64
	WorkspaceID string
	Refreshing  bool
	Options     []backend.ChatModelTargetOption
	Config      *backend.ServiceConfig
	Error       error
}

type WorkspaceLLMTargetUpdatedMsg struct {
	RequestSeq  int64
	WorkspaceID string
	Workspace   *projectioncontract.Workspace
	Target      *runtimecontract.ProviderModelTarget
	Cleared     bool
	Error       error
}

type WorkspaceLLMTargetShowMsg struct {
	WorkspaceID string
	Config      *backend.ServiceConfig
	Error       error
}
