package file

import (
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
)

func (m *Biz) handleObservePreparationDirectories(ev magicevent.Event, result magicevent.Result) {
	if result == nil {
		return
	}
	if ev == nil || ev.Source() != "blocks/workspace" || ev.ID() != fileevents.TopicObservePreparationDirectories {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	c, ok := ev.Data().(fileevents.ObservePreparationDirectoriesCommand)
	if !ok || c.Validate() != nil {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	ctx, err := common.BindRuntimeWorkspaceContext(ev.Context(), c.WorkspaceID)
	if setPrerequisiteError(result, err) || setPrerequisiteError(result, ctx.Err()) {
		return
	}
	r := fileevents.ObservePreparationDirectoriesResult{Version: fileevents.PreparationDirectoriesVersion, Command: c, StartedAt: time.Now().UTC()}
	r.Internal = observePreparationDirectory(c.InternalDirectory)
	if setPrerequisiteError(result, ctx.Err()) {
		return
	}
	r.Deliverable = observePreparationDirectory(c.DeliverableDirectory)
	r.CompletedAt = time.Now().UTC()
	if setPrerequisiteError(result, ctx.Err()) || setPrerequisiteError(result, r.ValidateFor(c)) {
		return
	}
	result.Set(r, nil)
}

func failedPreparationDirectory(path string, err error) fileobservation.Directory {
	data := []byte(err.Error())
	return fileobservation.Directory{Path: path, Diagnostic: data, DiagnosticBytes: len(data), DiagnosticSHA256: fileobservation.Digest(data)}
}
