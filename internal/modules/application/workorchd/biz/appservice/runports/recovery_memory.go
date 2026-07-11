package runports

import (
	"context"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/repository"
)

func RecordRecoveryMemoryRecallTrace(ctx context.Context, commandHub magicevent.Hub, memoryRecalls readmodelcore.MemoryRecallTraceReader, item *runmodel.Run, compressed map[string]any) error {
	if item == nil || item.ID == "" || item.WorkspaceID == "" || memoryRecalls == nil || len(compressed) == 0 {
		return nil
	}
	existing, err := memoryRecalls.List(ctx, repository.MemoryRecallTraceQuery{WorkspaceID: item.WorkspaceID, RunID: item.ID})
	if err != nil {
		return err
	}
	trace := runtimecore.BuildRecoveryMemoryRecallTrace(item, compressed, existing, time.Now().UTC().Format(time.RFC3339Nano))
	if trace == nil || commandHub == nil {
		return nil
	}
	return RecordMemoryRecallTrace(ctx, commandHub, trace)
}
