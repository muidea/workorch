package approval

import (
	"strings"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	approvalcontracts "github.com/muidea/workorch/internal/modules/blocks/approval/biz/contracts"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (m *Biz) handleRuntimeApprovalRequest(event magicevent.Event, result magicevent.Result) {
	if event == nil || event.Source() != runtimecontract.RuntimeModuleOrchestrator || m == nil || m.service == nil {
		setRuntimeApprovalRequestResult(result, approvalevents.OpenRuntimeApprovalRequestResult{}, common.ErrInvalidArgument)
		return
	}
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	command, ok := event.Data().(approvalevents.OpenRuntimeApprovalRequestCommand)
	if !ok || command.ApprovalID == "" || command.WorkspaceID == "" || command.RunID == "" || strings.TrimSpace(command.DispatchID) == "" ||
		strings.TrimSpace(command.AuthoritySnapshotID) == "" || strings.TrimSpace(command.DependencyKind) != string(runtimecontract.GovernanceApproval) ||
		strings.TrimSpace(command.SubjectRef) == "" || strings.TrimSpace(command.Reason) == "" || strings.TrimSpace(command.RequestedAction) == "" || len(command.AuthorizationContext) == 0 {
		setRuntimeApprovalRequestResult(result, approvalevents.OpenRuntimeApprovalRequestResult{}, common.ErrInvalidArgument)
		return
	}
	ctx, err := common.BindRuntimeWorkspaceContext(event.Context(), command.WorkspaceID)
	if err != nil {
		setRuntimeApprovalRequestResult(result, approvalevents.OpenRuntimeApprovalRequestResult{}, err)
		return
	}
	authorization := workorcheventpayload.New(command.AuthorizationContext...)
	binding := &approvalmodel.RuntimeGovernanceBinding{
		AuthoritySnapshotID: command.AuthoritySnapshotID, DispatchID: command.DispatchID,
		DependencyKind: command.DependencyKind, SubjectRef: command.SubjectRef,
		RequestedAction: command.RequestedAction, RiskLevel: command.RiskLevel,
		AuthorizationHash: runtimecontract.StableRuntimeAuthorizationContextHash(authorization),
	}
	err = m.service.EnsureApproval(ctx, approvalcontracts.CreateApprovalCommand{
		ApprovalID: command.ApprovalID, WorkspaceID: command.WorkspaceID, RunID: command.RunID,
		StepID: common.StepID("runtime/" + command.DispatchID), Reason: command.Reason,
		Action: command.RequestedAction, Context: authorization, RuntimeGovernance: binding,
	})
	if err != nil {
		setRuntimeApprovalRequestResult(result, approvalevents.OpenRuntimeApprovalRequestResult{}, err)
		return
	}
	setRuntimeApprovalRequestResult(result, approvalevents.OpenRuntimeApprovalRequestResult{
		ApprovalID: command.ApprovalID, WorkspaceID: command.WorkspaceID, RunID: command.RunID,
		DispatchID: command.DispatchID, AuthoritySnapshotID: command.AuthoritySnapshotID,
	}, nil)
}

func setRuntimeApprovalRequestResult(result magicevent.Result, response approvalevents.OpenRuntimeApprovalRequestResult, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(response, nil)
}
