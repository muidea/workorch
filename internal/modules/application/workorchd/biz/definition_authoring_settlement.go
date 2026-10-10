package biz

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
)

// 已知调用结果只结算原 permit，不因 caller 取消丢弃，也绝不重新调用 provider。
// completion CAS 响应丢失时只在同一有界调用内读取 owner 对账，不保存恢复队列。
func (b *Biz) settleDerivation(ctx context.Context, completion workspaceevents.CompleteDefinitionDerivationCommand) (workspaceevents.DefinitionDerivationRequestResult, error) {
	settlement, cancel := authoringSettlementContext(ctx)
	defer cancel()
	ev := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceDerivationComplete, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), settlement, completion)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(workspaceevents.Contracts(), workspaceevents.TopicWorkspaceDerivationComplete, eventcontract.LaneScope{WorkspaceID: string(completion.WorkspaceID)}))
	result := b.SendEvent(ev)
	var deliveryErr error
	if result == nil {
		deliveryErr = common.ErrConflict
	} else if resultErr := result.Error(); resultErr != nil {
		deliveryErr = resultErr
	} else {
		value, resultErr := result.Get()
		if resultErr == nil {
			response, ok := value.(workspaceevents.DefinitionDerivationRequestResult)
			if ok && response.ValidateCompletion(completion) == nil {
				return response, nil
			}
			deliveryErr = common.ErrConflict
		} else {
			deliveryErr = resultErr
		}
	}
	view, err := b.QueryDefinitionAuthoring(settlement, workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: completion.WorkspaceID})
	if err == nil && view.Record != nil {
		for _, request := range view.Record.DerivationRequests {
			if request.PermitID != completion.PermitID {
				continue
			}
			response := workspaceevents.DefinitionDerivationRequestResult{Request: request.Clone(), Consumed: uint32(len(view.Record.DerivationRequests)), Remaining: uint32(workspace.DefinitionDerivationRequestLimit - len(view.Record.DerivationRequests))}
			if response.ValidateCompletion(completion) == nil {
				return response, nil
			}
		}
	}
	return workspaceevents.DefinitionDerivationRequestResult{}, deliveryErr
}

func (b *Biz) settleAuthoringSpecification(ctx context.Context, completion workspaceevents.CompleteAuthoringSpecificationCommand) (workspaceevents.AuthoringSpecificationRequestResult, error) {
	settlement, cancel := authoringSettlementContext(ctx)
	defer cancel()
	ev := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceAuthoringSpecificationComplete, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), settlement, completion)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(workspaceevents.Contracts(), workspaceevents.TopicWorkspaceAuthoringSpecificationComplete, eventcontract.LaneScope{WorkspaceID: string(completion.WorkspaceID)}))
	result := b.SendEvent(ev)
	var deliveryErr error
	if result == nil {
		deliveryErr = common.ErrConflict
	} else if resultErr := result.Error(); resultErr != nil {
		deliveryErr = resultErr
	} else {
		value, resultErr := result.Get()
		if resultErr == nil {
			response, ok := value.(workspaceevents.AuthoringSpecificationRequestResult)
			if ok && response.ValidateCompletion(completion) == nil {
				return response, nil
			}
			deliveryErr = common.ErrConflict
		} else {
			deliveryErr = resultErr
		}
	}
	view, err := b.QueryDefinitionAuthoring(settlement, workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: completion.WorkspaceID})
	if err == nil && view.Record != nil {
		for _, request := range view.Record.SpecificationRequests {
			if request.PermitID == completion.PermitID && (request.FailureCode != "" || request.Proposal != nil || request.Review != nil) {
				response := workspaceevents.AuthoringSpecificationRequestResult{Request: request.Clone(), Consumed: uint32(len(view.Record.SpecificationRequests)), Remaining: uint32(workspace.DefinitionAuthoringSpecificationRequestLimit - len(view.Record.SpecificationRequests))}
				if response.ValidateCompletion(completion) == nil {
					return response, nil
				}
			}
		}
	}
	return workspaceevents.AuthoringSpecificationRequestResult{}, deliveryErr
}
