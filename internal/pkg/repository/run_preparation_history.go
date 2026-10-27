package repository

import (
	"context"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

// RunPreparationHistoryReader is an optional Run-owner persistence capability.
// Implementations read the durable generation watermark, retirement and all
// Workspace records atomically. A missing implementation is unknown, never an
// empty ordinary ListByWorkspaceID fallback.
type RunPreparationHistoryReader interface {
	ObserveWorkspacePreparationHistory(context.Context, common.WorkspaceID) (runmodel.WorkspacePreparationHistory, error)
}

// RunExecutionIntentReceiptReader uses the same complete, retirement-aware
// physical inventory as the preparation observation. A nil result is unknown.
type RunExecutionIntentReceiptReader interface {
	ReadWorkspaceExecutionIntentReceipt(context.Context, common.WorkspaceID, string) (*runmodel.WorkspaceExecutionIntentReceipt, error)
}
