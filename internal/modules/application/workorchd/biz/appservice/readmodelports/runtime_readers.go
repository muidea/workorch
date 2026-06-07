package readmodelports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

type ToolCallReader struct {
	Hub events.Hub
}

func (r ToolCallReader) ListByRun(ctx context.Context, runID common.RunID) ([]*runmodel.ToolCall, error) {
	return ownerreadports.ListToolCallsByRunOwnerCommand(ctx, r.Hub, runID)
}

type PatchReader struct {
	Hub events.Hub
}

func (r PatchReader) ListByRun(ctx context.Context, runID common.RunID) ([]*runmodel.Patch, error) {
	return ownerreadports.ListPatchesByRunOwnerCommand(ctx, r.Hub, runID)
}

type ValidationResultReader struct {
	Hub events.Hub
}

func (r ValidationResultReader) ListByRun(ctx context.Context, runID common.RunID) ([]*runmodel.ValidationResult, error) {
	return ownerreadports.ListValidationResultsByRunOwnerCommand(ctx, r.Hub, runID)
}
