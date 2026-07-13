package runports

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func StartRunView(ctx context.Context, hub magicevent.Hub, runID string) (appmodelcore.Run, error) {
	if hub == nil {
		return appmodelcore.Run{}, common.ErrInvalidArgument
	}
	id := common.RunID(strings.TrimSpace(runID))
	if id == "" || common.RuntimeWorkspaceIDFromContext(ctx) == "" {
		return appmodelcore.Run{}, common.ErrInvalidArgument
	}
	loadedRun, err := ReadRun(ctx, hub, id)
	if err != nil {
		return appmodelcore.Run{}, err
	}
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, loadedRun.WorkspaceID)
	if err := StartRun(ctx, hub, loadedRun); err != nil {
		return appmodelcore.Run{}, err
	}

	item, err := ReadRun(ctx, hub, id)
	if err != nil {
		return appmodelcore.Run{}, err
	}
	return RunView(item), nil
}

func ResumeRunView(ctx context.Context, hub magicevent.Hub, runID string) (appmodelcore.Run, error) {
	if hub == nil {
		return appmodelcore.Run{}, common.ErrInvalidArgument
	}
	id := common.RunID(strings.TrimSpace(runID))
	if id == "" || common.RuntimeWorkspaceIDFromContext(ctx) == "" {
		return appmodelcore.Run{}, common.ErrInvalidArgument
	}
	loadedRun, err := ReadRun(ctx, hub, id)
	if err != nil {
		return appmodelcore.Run{}, err
	}
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, loadedRun.WorkspaceID)
	if err := ResumeRun(ctx, hub, loadedRun); err != nil {
		return appmodelcore.Run{}, err
	}

	item, err := ReadRun(ctx, hub, id)
	if err != nil {
		return appmodelcore.Run{}, err
	}
	return RunView(item), nil
}
