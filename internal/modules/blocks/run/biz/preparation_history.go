package run

import (
	"context"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

func (m *Biz) observeWorkspacePreparationHistory(ctx context.Context, ev magicevent.Event) (runevents.ObserveWorkspacePreparationHistoryResult, error) {
	c, ok := ev.Data().(runevents.ObserveWorkspacePreparationHistoryCommand)
	if !ok || c.Validate() != nil || (ev.Source() != workspaceExecutionIntentRunSource && ev.Source() != "application/workorchd") {
		return runevents.ObserveWorkspacePreparationHistoryResult{}, common.ErrInvalidArgument
	}
	reader, ok := m.runs.(repository.RunPreparationHistoryReader)
	if !ok {
		return runevents.ObserveWorkspacePreparationHistoryResult{}, common.ErrConflict
	}
	// Same owner ordering as fresh execution selection. No cross-owner I/O or
	// notification/recovery occurs while this lock is held. The repository
	// separately snapshots its complete records and durable watermark atomically.
	m.workspaceExecutionMu.Lock()
	defer m.workspaceExecutionMu.Unlock()
	if err := ctx.Err(); err != nil {
		return runevents.ObserveWorkspacePreparationHistoryResult{}, err
	}
	history, err := reader.ObserveWorkspacePreparationHistory(ctx, c.WorkspaceID)
	if err != nil {
		return runevents.ObserveWorkspacePreparationHistoryResult{}, err
	}
	r := runevents.ObserveWorkspacePreparationHistoryResult{RequestID: c.RequestID, ObservedAt: time.Now().UTC(), History: history}
	if err := r.ValidateFor(c); err != nil {
		return runevents.ObserveWorkspacePreparationHistoryResult{}, err
	}
	return r.Clone(), nil
}
