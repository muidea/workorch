package file

import (
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
	"github.com/muidea/workorch/internal/pkg/workorchmanagedconfig"
)

func (m *Biz) handleObservePreparationManagedConfigs(ev magicevent.Event, result magicevent.Result) {
	if result == nil {
		return
	}
	if ev == nil || ev.Source() != "blocks/workspace" || ev.ID() != fileevents.TopicObservePreparationManagedConfigs {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	c, ok := ev.Data().(fileevents.ObservePreparationManagedConfigsCommand)
	if !ok || c.Validate() != nil {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	ctx, err := common.BindRuntimeWorkspaceContext(ev.Context(), c.WorkspaceID)
	if setPrerequisiteError(result, err) || setPrerequisiteError(result, ctx.Err()) {
		return
	}
	r := fileevents.ObservePreparationManagedConfigsResult{Version: fileevents.PreparationManagedConfigsVersion, Command: c.Clone(), StartedAt: time.Now().UTC()}
	remaining := fileobservation.ResourceTotalBytes
	for _, target := range c.Targets {
		if setPrerequisiteError(result, ctx.Err()) {
			return
		}
		root := c.InternalDirectory
		if target.Scope == workorchmanagedconfig.ScopeSystem {
			root = workorchmanagedconfig.SystemDirectory(root)
		}
		item := observePreparationFile(root, target.Path, remaining)
		remaining -= item.ContentBytes
		r.Files = append(r.Files, item)
	}
	r.CompletedAt = time.Now().UTC()
	if setPrerequisiteError(result, ctx.Err()) || setPrerequisiteError(result, r.ValidateFor(c)) {
		return
	}
	result.Set(r, nil)
}
