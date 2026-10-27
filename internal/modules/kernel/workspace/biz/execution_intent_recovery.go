package workspace

import (
	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (m *Biz) handleRecoverExecutionIntentCommand(ev magicevent.Event, result magicevent.Result) {
	if result == nil {
		return
	}
	m.deletionIngressMu.RLock()
	defer m.deletionIngressMu.RUnlock()
	r, err := m.recoverExecutionIntent(ev)
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(r, nil)
}

// This is a command used only by the explicit execution POST. GET never
// settles. A new request can observe a blocking original but cannot re-send it.
func (m *Biz) recoverExecutionIntent(ev magicevent.Event) (workspaceevents.RecoverWorkspaceExecutionIntentResult, error) {
	c, ok := ev.Data().(workspaceevents.RecoverWorkspaceExecutionIntentCommand)
	if !ok || ev.Source() != workspaceExecutionIngressSource || c.Validate() != nil || m == nil || m.workspaces == nil {
		return workspaceevents.RecoverWorkspaceExecutionIntentResult{}, common.ErrInvalidArgument
	}
	ctx, err := common.BindRuntimeWorkspaceContext(ev.Context(), c.WorkspaceID)
	if err != nil {
		return workspaceevents.RecoverWorkspaceExecutionIntentResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return workspaceevents.RecoverWorkspaceExecutionIntentResult{}, err
	}
	w, err := m.workspaces.Get(ctx, c.WorkspaceID)
	if err != nil {
		return workspaceevents.RecoverWorkspaceExecutionIntentResult{}, err
	}
	if err := workspaceDeletionRejectsMutation(w); err != nil {
		return workspaceevents.RecoverWorkspaceExecutionIntentResult{}, err
	}
	r := workspaceevents.RecoverWorkspaceExecutionIntentResult{WorkspaceID: c.WorkspaceID, RequestID: c.RequestID, Kind: c.Kind}
	f := w.PreparationFence
	if f == nil {
		p := preparedWorkspaceExecutionIntent{workspace: w, command: workspaceevents.WorkspaceExecutionIntentCommand{WorkspaceID: c.WorkspaceID, RequestID: c.RequestID, Kind: c.Kind}}
		admission, _ := p.ownerAdmission()
		r.Admission = &admission
		return r, r.ValidateFor(c)
	}
	if f.Validate(w.ID) != nil {
		return r, common.ErrConflict
	}
	var original *workspacemodel.WorkspaceFreshIntentReservation
	for _, settled := range f.Settled {
		if settled.Reservation.RequestID == c.RequestID {
			value := settled.Reservation
			original = &value
			break
		}
	}
	if original == nil {
		original = f.Pending
	}
	if original == nil {
		return r, nil
	}
	p := preparedWorkspaceExecutionIntent{workspace: w, command: workspaceevents.WorkspaceExecutionIntentCommand{WorkspaceID: c.WorkspaceID, RequestID: c.RequestID, Kind: c.Kind}}
	admission := unresolvedFreshExecutionIntent(p, *original)
	if original.RequestID == c.RequestID {
		receipt, err := m.reconcileOriginalFreshIdentity(ctx, w.ID, *original, originalFreshReceiptQuery(w.ID, *original))
		if err != nil {
			return r, err
		}
		if receipt != nil {
			admission = workspaceevents.WorkspaceExecutionIntentResult(receipt.Admission)
		}
	}
	r.Admission = &admission
	return r, r.ValidateFor(c)
}
