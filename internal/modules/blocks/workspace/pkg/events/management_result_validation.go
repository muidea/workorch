package events

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// The command interfaces below are sealed to this package. They let the few
// resource-shaped result DTOs stay narrow while still binding validation to an
// exact concrete command type. Callers cannot pass an arbitrary scope or add a
// generic fallback command implementation outside the Workspace contract.
type WorkspaceResultCommand interface {
	workspaceResultWorkspaceID() common.WorkspaceID
}

func (c CreateWorkspaceCommand) workspaceResultWorkspaceID() common.WorkspaceID { return c.WorkspaceID }
func (c GetWorkspaceCommand) workspaceResultWorkspaceID() common.WorkspaceID    { return c.WorkspaceID }
func (c UpdateWorkspaceCommand) workspaceResultWorkspaceID() common.WorkspaceID { return c.WorkspaceID }

type GoalResultCommand interface {
	goalResultIdentity() (common.WorkspaceID, common.GoalID)
}

func (c DefineGoalCommand) goalResultIdentity() (common.WorkspaceID, common.GoalID) {
	return c.WorkspaceID, c.GoalID
}

func (c GetGoalCommand) goalResultIdentity() (common.WorkspaceID, common.GoalID) {
	return c.WorkspaceID, c.GoalID
}

func (c UpdateGoalCommand) goalResultIdentity() (common.WorkspaceID, common.GoalID) {
	return c.WorkspaceID, c.GoalID
}

type DoDResultCommand interface {
	doDResultIdentity() (common.WorkspaceID, common.DoDID)
}

func (c DefineDoDCommand) doDResultIdentity() (common.WorkspaceID, common.DoDID) {
	return c.WorkspaceID, c.DoDID
}

func (c GetDoDCommand) doDResultIdentity() (common.WorkspaceID, common.DoDID) {
	return c.WorkspaceID, c.DoDID
}

func (c UpdateDoDCommand) doDResultIdentity() (common.WorkspaceID, common.DoDID) {
	return c.WorkspaceID, c.DoDID
}

type WorkspaceTemplateResultCommand interface {
	workspaceTemplateResultID() string
}

func (c CreateWorkspaceTemplateCommand) workspaceTemplateResultID() string { return c.TemplateID }
func (c GetWorkspaceTemplateCommand) workspaceTemplateResultID() string    { return c.TemplateID }
func (c UpdateWorkspaceTemplateCommand) workspaceTemplateResultID() string { return c.TemplateID }

func (r WorkspaceResult) ValidateFor(command WorkspaceResultCommand) error {
	expected, ok := workspaceResultCommandID(command)
	if !ok || r.Workspace == nil {
		return common.ErrInvalidArgument
	}
	if !validWorkspaceResultID(expected) || !validWorkspaceResultID(r.Workspace.ID) {
		return common.ErrInvalidArgument
	}
	if r.Workspace.ID != expected {
		return common.ErrConflict
	}
	return nil
}

func (r WorkspaceListResult) ValidateFor(ListWorkspacesCommand) error {
	seen := make(map[common.WorkspaceID]struct{}, len(r.Workspaces))
	for _, item := range r.Workspaces {
		if item == nil || !validWorkspaceResultID(item.ID) {
			return common.ErrInvalidArgument
		}
		if _, exists := seen[item.ID]; exists {
			return common.ErrConflict
		}
		seen[item.ID] = struct{}{}
	}
	return nil
}

func (r WorkspaceLLMSelectionResult) ValidateFor(command UpdateWorkspaceLLMSelectionCommand) error {
	if !validWorkspaceResultID(command.WorkspaceID) || command.ExpectedRevision == 0 || r.Workspace == nil || !validWorkspaceResultID(r.Workspace.ID) {
		return common.ErrInvalidArgument
	}
	if r.Workspace.ID != command.WorkspaceID {
		return common.ErrConflict
	}
	expectedRevision := command.ExpectedRevision
	if r.Changed {
		if expectedRevision == ^uint64(0) {
			return common.ErrInvalidArgument
		}
		expectedRevision++
	}
	if r.Workspace.Revision != expectedRevision {
		return common.ErrConflict
	}
	if command.Selection == nil {
		if r.Workspace.DefaultLLMSelection != nil {
			return common.ErrConflict
		}
		return nil
	}
	expectedSelection := command.Selection.Normalized()
	if err := expectedSelection.Validate(); err != nil {
		return common.ErrInvalidArgument
	}
	if r.Workspace.DefaultLLMSelection == nil || !r.Workspace.DefaultLLMSelection.Normalized().Same(expectedSelection) {
		return common.ErrConflict
	}
	return nil
}

func (r GoalResult) ValidateFor(command GoalResultCommand) error {
	workspaceID, goalID, ok := goalResultCommandIdentity(command)
	if !ok || r.Goal == nil {
		return common.ErrInvalidArgument
	}
	if !validWorkspaceResultID(workspaceID) || !validGoalResultID(goalID) || !validWorkspaceResultID(r.Goal.WorkspaceID) || !validGoalResultID(r.Goal.ID) {
		return common.ErrInvalidArgument
	}
	if r.Goal.WorkspaceID != workspaceID || r.Goal.ID != goalID {
		return common.ErrConflict
	}
	return nil
}

func (r GoalListResult) ValidateFor(command ListGoalsByWorkspaceCommand) error {
	if !validWorkspaceResultID(command.WorkspaceID) {
		return common.ErrInvalidArgument
	}
	seen := make(map[common.GoalID]struct{}, len(r.Goals))
	for _, item := range r.Goals {
		if item == nil || !validWorkspaceResultID(item.WorkspaceID) || !validGoalResultID(item.ID) {
			return common.ErrInvalidArgument
		}
		if item.WorkspaceID != command.WorkspaceID {
			return common.ErrConflict
		}
		if _, exists := seen[item.ID]; exists {
			return common.ErrConflict
		}
		seen[item.ID] = struct{}{}
	}
	return nil
}

func (r DeleteGoalResult) ValidateFor(command DeleteGoalCommand) error {
	if !validWorkspaceResultID(command.WorkspaceID) || !validGoalResultID(command.GoalID) || !validWorkspaceResultID(r.WorkspaceID) || !validGoalResultID(r.GoalID) {
		return common.ErrInvalidArgument
	}
	if r.WorkspaceID != command.WorkspaceID || r.GoalID != command.GoalID {
		return common.ErrConflict
	}
	return nil
}

func (r DoDResult) ValidateFor(command DoDResultCommand) error {
	workspaceID, doDID, ok := doDResultCommandIdentity(command)
	if !ok || r.DoD == nil {
		return common.ErrInvalidArgument
	}
	if !validWorkspaceResultID(workspaceID) || !validDoDResultID(doDID) || !validWorkspaceResultID(r.DoD.WorkspaceID) || !validDoDResultID(r.DoD.ID) {
		return common.ErrInvalidArgument
	}
	if r.DoD.WorkspaceID != workspaceID || r.DoD.ID != doDID {
		return common.ErrConflict
	}
	return nil
}

func (r DoDListResult) ValidateFor(command ListDoDsByWorkspaceCommand) error {
	if !validWorkspaceResultID(command.WorkspaceID) {
		return common.ErrInvalidArgument
	}
	seen := make(map[common.DoDID]struct{}, len(r.DoDs))
	for _, item := range r.DoDs {
		if item == nil || !validWorkspaceResultID(item.WorkspaceID) || !validDoDResultID(item.ID) {
			return common.ErrInvalidArgument
		}
		if item.WorkspaceID != command.WorkspaceID {
			return common.ErrConflict
		}
		if _, exists := seen[item.ID]; exists {
			return common.ErrConflict
		}
		seen[item.ID] = struct{}{}
	}
	return nil
}

func (r DeleteDoDResult) ValidateFor(command DeleteDoDCommand) error {
	if !validWorkspaceResultID(command.WorkspaceID) || !validDoDResultID(command.DoDID) || !validWorkspaceResultID(r.WorkspaceID) || !validDoDResultID(r.DoDID) {
		return common.ErrInvalidArgument
	}
	if r.WorkspaceID != command.WorkspaceID || r.DoDID != command.DoDID {
		return common.ErrConflict
	}
	return nil
}

func (r WorkspaceTemplateResult) ValidateFor(command WorkspaceTemplateResultCommand) error {
	expected, ok := workspaceTemplateResultCommandID(command)
	if !ok || r.Template == nil {
		return common.ErrInvalidArgument
	}
	if !validTemplateResultID(expected) || !validTemplateResultID(r.Template.ID) {
		return common.ErrInvalidArgument
	}
	if r.Template.ID != expected {
		return common.ErrConflict
	}
	return nil
}

func (r WorkspaceTemplateListResult) ValidateFor(ListWorkspaceTemplatesCommand) error {
	seen := make(map[string]struct{}, len(r.Templates))
	for _, item := range r.Templates {
		if item == nil || !validTemplateResultID(item.ID) {
			return common.ErrInvalidArgument
		}
		if _, exists := seen[item.ID]; exists {
			return common.ErrConflict
		}
		seen[item.ID] = struct{}{}
	}
	return nil
}

func (r DeleteWorkspaceTemplateResult) ValidateFor(command DeleteWorkspaceTemplateCommand) error {
	if !validTemplateResultID(command.TemplateID) || !validTemplateResultID(r.TemplateID) {
		return common.ErrInvalidArgument
	}
	if r.TemplateID != command.TemplateID {
		return common.ErrConflict
	}
	return nil
}

func (r EnsureWorkspaceTemplateDefaultsResult) ValidateFor(EnsureWorkspaceTemplateDefaultsCommand) error {
	if !r.Ensured {
		return common.ErrInvalidArgument
	}
	return nil
}

func validWorkspaceResultID(value common.WorkspaceID) bool {
	return strings.TrimSpace(string(value)) != "" && strings.TrimSpace(string(value)) == string(value)
}

func validGoalResultID(value common.GoalID) bool {
	return strings.TrimSpace(string(value)) != "" && strings.TrimSpace(string(value)) == string(value)
}

func validDoDResultID(value common.DoDID) bool {
	return strings.TrimSpace(string(value)) != "" && strings.TrimSpace(string(value)) == string(value)
}

func validTemplateResultID(value string) bool {
	return strings.TrimSpace(value) != "" && strings.TrimSpace(value) == value
}

func workspaceResultCommandID(command WorkspaceResultCommand) (common.WorkspaceID, bool) {
	switch value := command.(type) {
	case CreateWorkspaceCommand:
		return value.WorkspaceID, true
	case GetWorkspaceCommand:
		return value.WorkspaceID, true
	case UpdateWorkspaceCommand:
		return value.WorkspaceID, true
	default:
		return "", false
	}
}

func goalResultCommandIdentity(command GoalResultCommand) (common.WorkspaceID, common.GoalID, bool) {
	switch value := command.(type) {
	case DefineGoalCommand:
		return value.WorkspaceID, value.GoalID, true
	case GetGoalCommand:
		return value.WorkspaceID, value.GoalID, true
	case UpdateGoalCommand:
		return value.WorkspaceID, value.GoalID, true
	default:
		return "", "", false
	}
}

func doDResultCommandIdentity(command DoDResultCommand) (common.WorkspaceID, common.DoDID, bool) {
	switch value := command.(type) {
	case DefineDoDCommand:
		return value.WorkspaceID, value.DoDID, true
	case GetDoDCommand:
		return value.WorkspaceID, value.DoDID, true
	case UpdateDoDCommand:
		return value.WorkspaceID, value.DoDID, true
	default:
		return "", "", false
	}
}

func workspaceTemplateResultCommandID(command WorkspaceTemplateResultCommand) (string, bool) {
	switch value := command.(type) {
	case CreateWorkspaceTemplateCommand:
		return value.TemplateID, true
	case GetWorkspaceTemplateCommand:
		return value.TemplateID, true
	case UpdateWorkspaceTemplateCommand:
		return value.TemplateID, true
	default:
		return "", false
	}
}
