package readmodelports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type ToolCallReader struct {
	Hub magicevent.Hub
}

func (r ToolCallReader) ListByRun(ctx context.Context, runID common.RunID) ([]*runmodel.ToolCall, error) {
	return ownerreadports.ListToolCallsByRunOwnerCommand(ctx, r.Hub, runID)
}

type PatchReader struct {
	Hub magicevent.Hub
}

func (r PatchReader) ListByRun(ctx context.Context, runID common.RunID) ([]*runmodel.Patch, error) {
	return ownerreadports.ListPatchesByRunOwnerCommand(ctx, r.Hub, runID)
}

type ValidationResultReader struct {
	Hub magicevent.Hub
}

func (r ValidationResultReader) ListByRun(ctx context.Context, runID common.RunID) ([]*runmodel.ValidationResult, error) {
	return ownerreadports.ListValidationResultsByRunOwnerCommand(ctx, r.Hub, runID)
}
