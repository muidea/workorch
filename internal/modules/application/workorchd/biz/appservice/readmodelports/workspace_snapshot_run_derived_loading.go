package readmodelports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (models *workspaceSnapshotModels) loadRunDerivedItems(ctx context.Context, readers WorkspaceSnapshotReaders, workspaceID common.WorkspaceID, projectionRun *runmodel.Run, options workspaceSnapshotLoadOptions) error {
	for _, runItem := range models.Runs {
		if runItem == nil || runItem.WorkspaceID != workspaceID {
			continue
		}
		if readers.ArtifactContinuations != nil {
			items, err := readers.ArtifactContinuations.ListByRun(ctx, runItem.ID)
			if err != nil {
				return err
			}
			models.ArtifactContinuations = append(models.ArtifactContinuations, items...)
		}
		if readers.Patches != nil {
			items, err := readers.Patches.ListByRun(ctx, runItem.ID)
			if err != nil {
				return err
			}
			models.Patches = append(models.Patches, items...)
		}
		if options.includeLLMUsagePayloads && readers.EventRecords != nil && projectionRun != nil && runItem.ID == projectionRun.ID {
			items, err := readers.EventRecords.List(ctx, readmodelcore.RunLLMUsageEventRecordRepositoryQuery(runItem.ID, readmodelcore.DefaultLLMUsageEventRecordLimit))
			if err != nil {
				return err
			}
			models.appendLLMUsageEventRecords(items...)
		}
	}
	return nil
}

func (models *workspaceSnapshotModels) appendLLMUsageEventRecords(items ...*eventstreammodel.EventRecord) {
	if models == nil || len(items) == 0 {
		return
	}
	seen := make(map[common.EventRecordID]struct{}, len(models.LLMUsageEventRecords)+len(items))
	for _, item := range models.LLMUsageEventRecords {
		if item == nil || item.ID == "" {
			continue
		}
		seen[item.ID] = struct{}{}
	}
	for _, item := range items {
		if item == nil || item.ID == "" {
			continue
		}
		if _, ok := seen[item.ID]; ok {
			continue
		}
		models.LLMUsageEventRecords = append(models.LLMUsageEventRecords, item)
		seen[item.ID] = struct{}{}
	}
}
