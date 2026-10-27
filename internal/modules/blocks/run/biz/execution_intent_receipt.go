package run

import (
	"context"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

func (m *Biz) queryWorkspaceExecutionIntentReceipt(ctx context.Context, ev magicevent.Event) (runevents.QueryWorkspaceExecutionIntentReceiptResult, error) {
	c, ok := ev.Data().(runevents.QueryWorkspaceExecutionIntentReceiptCommand)
	if !ok || c.Validate() != nil || ev.Source() != workspaceExecutionIntentRunSource {
		return runevents.QueryWorkspaceExecutionIntentReceiptResult{}, common.ErrInvalidArgument
	}
	reader, ok := m.runs.(repository.RunExecutionIntentReceiptReader)
	if !ok {
		return runevents.QueryWorkspaceExecutionIntentReceiptResult{}, common.ErrConflict
	}
	m.workspaceExecutionMu.Lock()
	defer m.workspaceExecutionMu.Unlock()
	if err := ctx.Err(); err != nil {
		return runevents.QueryWorkspaceExecutionIntentReceiptResult{}, err
	}
	receipt, err := reader.ReadWorkspaceExecutionIntentReceipt(ctx, c.WorkspaceID, c.RequestID)
	if err != nil {
		return runevents.QueryWorkspaceExecutionIntentReceiptResult{}, err
	}
	r := runevents.QueryWorkspaceExecutionIntentReceiptResult{WorkspaceID: c.WorkspaceID, RequestID: c.RequestID, PayloadHash: c.PayloadHash, ObservedAt: time.Now().UTC(), Receipt: receipt}
	if err := r.ValidateFor(c); err != nil {
		return runevents.QueryWorkspaceExecutionIntentReceiptResult{}, err
	}
	return r.Clone(), nil
}
