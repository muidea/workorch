package readmodelports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	runtimeevidencemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type ToolCallReader struct {
	Hub magicevent.Hub
}

func (r ToolCallReader) ListByRun(ctx context.Context, runID common.RunID) ([]*runtimeevidencemodel.ToolCall, error) {
	return ownerreadports.ListToolCallsByRunOwnerCommand(ctx, r.Hub, runID)
}

type PatchReader struct {
	Hub magicevent.Hub
}

func (r PatchReader) ListByRun(ctx context.Context, runID common.RunID) ([]*runtimeevidencemodel.Patch, error) {
	return ownerreadports.ListPatchesByRunOwnerCommand(ctx, r.Hub, runID)
}

type ValidationResultReader struct {
	Hub magicevent.Hub
}

func (r ValidationResultReader) ListByRun(ctx context.Context, runID common.RunID) ([]*runtimeevidencemodel.ValidationResult, error) {
	return ownerreadports.ListValidationResultsByRunOwnerCommand(ctx, r.Hub, runID)
}
