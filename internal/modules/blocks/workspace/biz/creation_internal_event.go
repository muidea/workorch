package workspace

import (
	"context"
	"strings"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type workspaceCreationInternalEventKind string

const workspaceCreationInternalEventRequested workspaceCreationInternalEventKind = "creation_requested"

type workspaceCreationRequestedFact struct {
	WorkspaceID          common.WorkspaceID
	Title                string
	Description          string
	DeliverableDirectory string
	InternalDirectory    string
}

// workspaceCreationInternalEvent contains a bounded owner fact, never the
// public command or a requested lifecycle state. The Workspace owner chooses
// the initial state below.
type workspaceCreationInternalEvent struct {
	Kind      workspaceCreationInternalEventKind
	Requested *workspaceCreationRequestedFact
}

// submitWorkspaceCreationInternalEvent is the sole creation-state dispatcher
// for Workspace. Protocol adapters and callers may provide descriptive input,
// but cannot select Workspace.State or any active Run slot.
func (m *goalDoDManager) submitWorkspaceCreationInternalEvent(ctx context.Context, event workspaceCreationInternalEvent) (*workspacemodel.Workspace, error) {
	if m == nil || m.workspaces == nil || event.Kind != workspaceCreationInternalEventRequested || event.Requested == nil {
		return nil, common.ErrInvalidArgument
	}
	fact := *event.Requested
	if fact.WorkspaceID == "" || strings.TrimSpace(fact.Title) == "" || strings.TrimSpace(fact.DeliverableDirectory) == "" {
		return nil, common.ErrInvalidArgument
	}
	bound, err := common.BindRuntimeWorkspaceContext(ctx, fact.WorkspaceID)
	if err != nil {
		return nil, err
	}
	workspace := &workspacemodel.Workspace{
		ID:                   fact.WorkspaceID,
		Title:                fact.Title,
		Description:          fact.Description,
		DeliverableDirectory: fact.DeliverableDirectory,
		InternalDirectory:    fact.InternalDirectory,
		State:                common.WorkspaceStateActive,
	}
	if err := m.workspaces.Create(bound, workspace); err != nil {
		return nil, err
	}
	return workspace, nil
}
