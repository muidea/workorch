package contracts

import "github.com/muidea/workorch/internal/pkg/common"

type CreateWorkspaceCommand struct {
	WorkspaceID          common.WorkspaceID
	Title                string
	Description          string
	DeliverableDirectory string
	InternalDirectory    string
}

type EnsureWorkspaceCommand struct {
	WorkspaceID          common.WorkspaceID
	Title                string
	Description          string
	DeliverableDirectory string
	InternalDirectory    string
}

type DeleteWorkspaceCommand struct {
	WorkspaceID common.WorkspaceID
}

type DeleteWorkspaceTemplateCommand struct {
	TemplateID string
}

type DefineGoalCommand struct {
	GoalID      common.GoalID
	WorkspaceID common.WorkspaceID
	Text        string
}

type UpdateGoalCommand struct {
	WorkspaceID common.WorkspaceID
	GoalID      common.GoalID
	Text        string
}

type DeleteGoalCommand struct {
	WorkspaceID common.WorkspaceID
	GoalID      common.GoalID
}

type DefineDoDCommand struct {
	DoDID       common.DoDID
	WorkspaceID common.WorkspaceID
	Items       []string
}

type UpdateDoDCommand struct {
	WorkspaceID common.WorkspaceID
	DoDID       common.DoDID
	Items       []string
}

type DeleteDoDCommand struct {
	WorkspaceID common.WorkspaceID
	DoDID       common.DoDID
}
