package run

import (
	cd "github.com/muidea/magicCommon/def"
	event "github.com/muidea/magicCommon/event"
	events "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (m *Biz) handleRuntimeLifecycleRead(ev event.Event, result event.Result) {
	if result == nil {
		return
	}
	command, ok := ev.Data().(events.QueryRuntimeLifecycleCommand)
	if !ok || command.Validate() != nil || ev.Source() != workspaceRetirementFactQuerySource {
		result.Set(nil, &cd.Error{Code: cd.IllegalParam, Message: "invalid runtime lifecycle query", Cause: common.ErrInvalidArgument})
		return
	}
	leave, err := m.enterOwnerCommand(true)
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	defer leave()
	m.workspaceExecutionMu.Lock()
	defer m.workspaceExecutionMu.Unlock()
	ctx, err := common.BindRuntimeWorkspaceContext(ev.Context(), command.WorkspaceID)
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	retired, err := m.workspaceRunsRetired(ctx, command.WorkspaceID)
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	if retired {
		result.Set(nil, &cd.Error{Code: cd.NotFound, Message: common.ErrNotFound.Error(), Cause: common.ErrNotFound})
		return
	}
	view, err := m.runs.GetRuntimeLifecycle(ctx, command.WorkspaceID, command.RunID)
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	response := events.QueryRuntimeLifecycleResult{Lifecycle: view}
	if err := response.Validate(); err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(response, nil)
}
