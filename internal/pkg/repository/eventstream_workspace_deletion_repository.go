package repository

import (
	"context"

	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type EventStreamWorkspaceDeletionRepository interface {
	Create(context.Context, *eventstreammodel.WorkspaceDeletion) error
	Get(context.Context, common.WorkspaceID) (*eventstreammodel.WorkspaceDeletion, error)
	CompareAndSwap(context.Context, *eventstreammodel.WorkspaceDeletion, uint64) error
	WithWorkspaceActiveFence(context.Context, common.WorkspaceID, func(context.Context) error) error
	WithWorkspacePurgeFence(context.Context, eventstreammodel.WorkspaceDeletion, func(context.Context) error) error
}

// EventStreamWorkspacePurger is the Eventstream owner's exact privileged
// physical cleanup sink. Ordinary EventRecord Delete/PurgeWorkspace calls do
// not carry the persisted fence and cannot use this path.
type EventStreamWorkspacePurger interface {
	PurgeWorkspaceEventRecords(context.Context, eventstreammodel.WorkspaceDeletion) (int, error)
}
