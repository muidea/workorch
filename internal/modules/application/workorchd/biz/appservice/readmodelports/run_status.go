package readmodelports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
)

type RunStatusReaders struct {
	Runs        readmodelcore.RunReader
	Steps       readmodelcore.StepReader
	Checkpoints readmodelcore.RuntimeCheckpointReader
	Tickets     readmodelcore.DecisionTicketReader
	Approvals   readmodelcore.ApprovalReader
}

func RunStatus(ctx context.Context, readers RunStatusReaders, runID common.RunID) (readmodelcore.RunStatus, error) {
	item, err := readmodelcore.GetRunFromReader(ctx, readers.Runs, runID)
	if err != nil {
		return readmodelcore.RunStatus{}, err
	}
	related, err := loadRunStatusRelatedModels(ctx, readers, item)
	if err != nil {
		return readmodelcore.RunStatus{}, err
	}
	return projectRunStatus(item, related), nil
}
