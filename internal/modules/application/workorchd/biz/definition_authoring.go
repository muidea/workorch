package biz

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
)

func (b *Biz) QueryDefinitionAuthoring(ctx context.Context, command workspaceevents.QueryDefinitionAuthoringCommand) (workspaceevents.QueryDefinitionAuthoringResult, error) {
	if b == nil || b.EventHub() == nil || ctx == nil || command.WorkspaceID == "" {
		return workspaceevents.QueryDefinitionAuthoringResult{}, common.ErrInvalidArgument
	}
	if err := ctx.Err(); err != nil {
		return workspaceevents.QueryDefinitionAuthoringResult{}, err
	}
	ev := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceDefinitionAuthoringQuery, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(workspaceevents.Contracts(), workspaceevents.TopicWorkspaceDefinitionAuthoringQuery, eventcontract.LaneScope{WorkspaceID: string(command.WorkspaceID)}))
	result := b.SendEvent(ev)
	if result == nil {
		return workspaceevents.QueryDefinitionAuthoringResult{}, fmt.Errorf("%w: missing authoring query result", common.ErrConflict)
	}
	if err := result.Error(); err != nil {
		return workspaceevents.QueryDefinitionAuthoringResult{}, err
	}
	data, err := result.Get()
	if err != nil {
		return workspaceevents.QueryDefinitionAuthoringResult{}, err
	}
	response, ok := data.(workspaceevents.QueryDefinitionAuthoringResult)
	if !ok {
		return workspaceevents.QueryDefinitionAuthoringResult{}, fmt.Errorf("%w: unexpected authoring query result", common.ErrConflict)
	}
	if err := response.ValidateFor(command); err != nil {
		return workspaceevents.QueryDefinitionAuthoringResult{}, err
	}
	if response.Record != nil {
		record := response.Record.Clone()
		response.Record = &record
	}
	if response.Confirmation != nil {
		confirmation := response.Confirmation.Clone()
		response.Confirmation = &confirmation
	}
	if response.AdmissionCandidate != nil {
		candidate := response.AdmissionCandidate.Clone()
		response.AdmissionCandidate = &candidate
	}
	if response.AdmissionResult != nil {
		admission := *response.AdmissionResult
		admission.OffendingRefs = append([]string(nil), admission.OffendingRefs...)
		response.AdmissionResult = &admission
	}
	if response.ActivationAdmission != nil {
		activation := response.ActivationAdmission.Normalized()
		response.ActivationAdmission = &activation
	}
	return response, nil
}

func (b *Biz) SaveDefinitionAuthoring(ctx context.Context, command workspaceevents.SaveDefinitionAuthoringCommand) (workspaceevents.SaveDefinitionAuthoringResult, error) {
	if b == nil || b.EventHub() == nil || ctx == nil {
		return workspaceevents.SaveDefinitionAuthoringResult{}, common.ErrInvalidArgument
	}
	if err := ctx.Err(); err != nil {
		return workspaceevents.SaveDefinitionAuthoringResult{}, err
	}
	command.Input = command.Input.Normalized()
	if err := command.Validate(); err != nil {
		return workspaceevents.SaveDefinitionAuthoringResult{}, err
	}
	ev := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceDefinitionAuthoringSave, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(workspaceevents.Contracts(), workspaceevents.TopicWorkspaceDefinitionAuthoringSave, eventcontract.LaneScope{WorkspaceID: string(command.Input.WorkspaceID)}))
	result := b.SendEvent(ev)
	if result == nil {
		return workspaceevents.SaveDefinitionAuthoringResult{}, fmt.Errorf("%w: missing authoring save result", common.ErrConflict)
	}
	if err := result.Error(); err != nil {
		return workspaceevents.SaveDefinitionAuthoringResult{}, err
	}
	data, err := result.Get()
	if err != nil {
		return workspaceevents.SaveDefinitionAuthoringResult{}, err
	}
	response, ok := data.(workspaceevents.SaveDefinitionAuthoringResult)
	if !ok {
		return workspaceevents.SaveDefinitionAuthoringResult{}, fmt.Errorf("%w: unexpected authoring save result", common.ErrConflict)
	}
	if err := response.Record.Validate(); err != nil {
		return workspaceevents.SaveDefinitionAuthoringResult{}, err
	}
	if response.Record.InputHash != command.Input.InputHash() {
		return workspaceevents.SaveDefinitionAuthoringResult{}, fmt.Errorf("%w: authoring save identity mismatch", common.ErrConflict)
	}
	response.Record = response.Record.Clone()
	return response, nil
}
