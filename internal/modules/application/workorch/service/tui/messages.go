package tui

import (
	"github.com/muidea/workorch/internal/modules/application/workorch/biz/backend"
)

type WorkspaceListMsg struct {
	Workspaces []backend.WorkspaceInfo
	Error      error
}

type DecisionListMsg struct {
	Decisions []backend.DecisionInfo
	Error     error
}

type ApprovalListMsg struct {
	Approvals []backend.ApprovalInfo
	Error     error
}

type ErrorMsg struct{ Error error }
