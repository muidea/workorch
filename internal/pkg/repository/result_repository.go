package repository

import (
	"context"

	result "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type ResultRepository interface {
	Create(context.Context, *result.Result) error
	Get(context.Context, common.ResultID) (*result.Result, error)
	ListByWorkspace(context.Context, common.WorkspaceID) ([]*result.Result, error)
	ListByRun(context.Context, common.RunID) ([]*result.Result, error)
	Update(context.Context, *result.Result) error
	CompareAndSwap(context.Context, *result.Result, uint64) error
	Delete(context.Context, common.ResultID) error
}

// ResultRecoveryRepository is an owner-private, bounded inventory used to
// replay derived notifications from durable Result rows after a process or
// EventHub delivery interruption. The cursor is opaque to callers.
//
// It is deliberately separate from ResultRepository so cross-owner query
// contracts cannot acquire an unscoped list surface.
type ResultRecoveryRepository interface {
	ListForRecovery(context.Context, string, int) ([]*result.Result, string, error)
}
