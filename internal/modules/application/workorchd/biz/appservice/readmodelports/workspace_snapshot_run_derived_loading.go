package readmodelports

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
)

func (models *workspaceSnapshotModels) loadRunDerivedItems(ctx context.Context, readers WorkspaceSnapshotReaders, workspaceID common.WorkspaceID) error {
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
	}
	return nil
}
