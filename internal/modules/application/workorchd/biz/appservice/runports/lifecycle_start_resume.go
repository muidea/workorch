package runports

import (
	"context"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func StartRunView(ctx context.Context, hub events.Hub, deps CompressedContextDependencies, runID string) (appmodelcore.Run, error) {
	if hub == nil {
		return appmodelcore.Run{}, common.ErrInvalidArgument
	}
	id := common.RunID(strings.TrimSpace(runID))
	loadedRun, err := ReadRun(ctx, hub, id)
	if err != nil {
		return appmodelcore.Run{}, err
	}
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, loadedRun.WorkspaceID)
	if err := AttachCompressedContextToLoadedRun(ctx, deps, loadedRun); err != nil {
		return appmodelcore.Run{}, err
	}
	if err := StartRun(ctx, hub, loadedRun); err != nil {
		return appmodelcore.Run{}, err
	}

	item, err := ReadRun(ctx, hub, id)
	if err != nil {
		return appmodelcore.Run{}, err
	}
	return RunView(item), nil
}

func ResumeRunView(ctx context.Context, hub events.Hub, deps CompressedContextDependencies, runID string, executionContextPatch ...map[string]any) (appmodelcore.Run, error) {
	if hub == nil {
		return appmodelcore.Run{}, common.ErrInvalidArgument
	}
	id := common.RunID(strings.TrimSpace(runID))
	patch, loadedRun, err := CompressedContextPatchForRun(ctx, deps, id)
	if err != nil {
		return appmodelcore.Run{}, err
	}
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, loadedRun.WorkspaceID)
	for _, item := range executionContextPatch {
		for key, value := range item {
			if strings.TrimSpace(key) == "" {
				continue
			}
			patch[key] = value
		}
	}
	if err := ResumeRun(ctx, hub, loadedRun, patch); err != nil {
		return appmodelcore.Run{}, err
	}

	item, err := ReadRun(ctx, hub, id)
	if err != nil {
		return appmodelcore.Run{}, err
	}
	return RunView(item), nil
}
