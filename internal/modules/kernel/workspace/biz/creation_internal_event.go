package workspace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"

	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
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
	Authoring            *workspaceevents.CreateDefinitionAuthoringCommand
	Template             *workspacemodel.WorkspaceTemplate
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
	if fact.Authoring != nil {
		cmd := *fact.Authoring
		if cmd.Validate() != nil || cmd.WorkspaceID != fact.WorkspaceID || cmd.Title != fact.Title || cmd.Description != fact.Description || cmd.DeliverableDirectory != fact.DeliverableDirectory || cmd.InternalDirectory != fact.InternalDirectory {
			return nil, common.ErrInvalidArgument
		}
		current, readErr := m.workspaces.Get(bound, fact.WorkspaceID)
		if readErr == nil {
			if current.DefinitionAuthoringCreation == nil || current.DefinitionAuthoringCreation.Validate(current.ID) != nil || current.DefinitionAuthoringCreation.RequestID != cmd.RequestID || current.DefinitionAuthoringCreation.InputHash != cmd.CreationInputHash() || workspaceDeletionRejectsMutation(current) != nil {
				return nil, common.ErrConflict
			}
			return current, nil
		}
		if !errors.Is(readErr, common.ErrNotFound) {
			return nil, readErr
		}
		if fact.Template == nil || fact.Template.ID != cmd.Input.TemplateID {
			return nil, common.ErrConflict
		}
		record := workspacemodel.DefinitionAuthoringRecord{Input: cmd.Input.Normalized(), InputHash: cmd.Input.InputHash(), TemplateSnapshot: fact.Template.Clone(), TemplateSnapshotHash: workspacemodel.WorkspaceTemplateSnapshotHash(fact.Template)}
		if record.Validate() != nil {
			return nil, common.ErrConflict
		}
		workspace.TemplateID = fact.Template.ID
		workspace, err = workspacemodel.WorkspaceForCurrentTemplate(workspace, fact.Template)
		if err != nil {
			return nil, err
		}
		workspace.DefinitionAuthoring = &record
		workspace.DefinitionAuthoringCreation = &workspacemodel.DefinitionAuthoringCreation{RequestID: cmd.RequestID, InputHash: cmd.CreationInputHash()}
	}
	// Only the creation dispatcher may mint this marker. Existing authoring
	// replays returned above retain their original identity.
	var creationEpoch [16]byte
	if _, err := rand.Read(creationEpoch[:]); err != nil {
		return nil, err
	}
	workspace.PreparationFence = &workspacemodel.WorkspacePreparationFence{
		Version: workspacemodel.WorkspacePreparationFenceVersion, WorkspaceID: workspace.ID,
		CreationEpoch: hex.EncodeToString(creationEpoch[:]),
	}
	if err := m.workspaces.Create(bound, workspace); err != nil {
		return nil, err
	}
	return workspace, nil
}
