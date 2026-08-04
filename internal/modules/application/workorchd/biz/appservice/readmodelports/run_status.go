package readmodelports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

type RunStatusReaders struct {
	Runs        readmodelcore.RunReader
	Steps       readmodelcore.StepReader
	Checkpoints readmodelcore.RuntimeCheckpointReader
	Tickets     readmodelcore.DecisionTicketReader
	Approvals   readmodelcore.ApprovalReader
}

func RunStatus(ctx context.Context, readers RunStatusReaders, runID common.RunID) (projectioncontract.RunStatus, error) {
	item, err := readmodelcore.GetRunFromReader(ctx, readers.Runs, runID)
	if err != nil {
		return projectioncontract.RunStatus{}, err
	}
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, item.WorkspaceID)
	related, err := loadRunStatusRelatedModels(ctx, readers, item)
	if err != nil {
		return projectioncontract.RunStatus{}, err
	}
	return projectRunStatus(item, related), nil
}
