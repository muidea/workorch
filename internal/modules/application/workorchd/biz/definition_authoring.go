package biz

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
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
	response.History = append([]workspaceevents.DefinitionAuthoringConfirmedRevision(nil), response.History...)
	for n := range response.History {
		response.History[n] = response.History[n].Clone()
	}
	if response.Confirmation != nil {
		confirmation := response.Confirmation.Clone()
		response.Confirmation = &confirmation
	}
	if response.PriorClarification != nil {
		binding := response.PriorClarification.Clone()
		response.PriorClarification = &binding
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
	if response.Record.Input.WorkspaceID != command.Input.WorkspaceID || response.Receipt.Validate() != nil || response.Receipt.RequestID != command.RequestID || command.ExpectedWorkspaceRevision == ^uint64(0) || response.Receipt.WorkspaceRevision != command.ExpectedWorkspaceRevision+1 || response.Receipt.RequestHash != workspace.DefinitionAuthoringSaveRequestHash(command.RequestID, command.Input, command.ExpectedWorkspaceRevision, command.ExpectedInputHash, command.DeliverableDirectory) {
		return workspaceevents.SaveDefinitionAuthoringResult{}, fmt.Errorf("%w: authoring save identity mismatch", common.ErrConflict)
	}
	matched := false
	for _, receipt := range response.Record.SaveReceipts {
		if receipt == response.Receipt {
			matched = true
			break
		}
	}
	if !matched {
		return workspaceevents.SaveDefinitionAuthoringResult{}, fmt.Errorf("%w: missing persisted save receipt", common.ErrConflict)
	}
	response.Record = response.Record.Clone()
	return response, nil
}

func (b *Biz) RecordPreparationDiagnostic(ctx context.Context, command workspaceevents.RecordPreparationDiagnosticCommand) (workspaceevents.RecordPreparationDiagnosticResult, error) {
	if b == nil || b.EventHub() == nil || ctx == nil || command.Validate() != nil {
		return workspaceevents.RecordPreparationDiagnosticResult{}, common.ErrInvalidArgument
	}
	if err := ctx.Err(); err != nil {
		return workspaceevents.RecordPreparationDiagnosticResult{}, err
	}
	ev := magicevent.NewEventWithContext(workspaceevents.TopicWorkspacePreparationDiagnosticRecord, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(workspaceevents.Contracts(), workspaceevents.TopicWorkspacePreparationDiagnosticRecord, eventcontract.LaneScope{WorkspaceID: string(command.Diagnostic.WorkspaceID)}))
	result := b.SendEvent(ev)
	if result == nil {
		return workspaceevents.RecordPreparationDiagnosticResult{}, common.ErrConflict
	}
	if err := result.Error(); err != nil {
		return workspaceevents.RecordPreparationDiagnosticResult{}, err
	}
	value, err := result.Get()
	if err != nil {
		return workspaceevents.RecordPreparationDiagnosticResult{}, err
	}
	response, ok := value.(workspaceevents.RecordPreparationDiagnosticResult)
	if !ok || response.ValidateFor(command) != nil {
		return workspaceevents.RecordPreparationDiagnosticResult{}, common.ErrConflict
	}
	return response.DeepClone(), nil
}

func (b *Biz) QueryPreparationDiagnostic(ctx context.Context, command workspaceevents.QueryPreparationDiagnosticCommand) (workspaceevents.QueryPreparationDiagnosticResult, error) {
	if b == nil || b.EventHub() == nil || ctx == nil || command.Validate() != nil {
		return workspaceevents.QueryPreparationDiagnosticResult{}, common.ErrInvalidArgument
	}
	if err := ctx.Err(); err != nil {
		return workspaceevents.QueryPreparationDiagnosticResult{}, err
	}
	ev := magicevent.NewEventWithContext(workspaceevents.TopicWorkspacePreparationDiagnosticQuery, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(workspaceevents.Contracts(), workspaceevents.TopicWorkspacePreparationDiagnosticQuery, eventcontract.LaneScope{WorkspaceID: string(command.WorkspaceID)}))
	result := b.SendEvent(ev)
	if result == nil {
		return workspaceevents.QueryPreparationDiagnosticResult{}, common.ErrConflict
	}
	if err := result.Error(); err != nil {
		return workspaceevents.QueryPreparationDiagnosticResult{}, err
	}
	value, err := result.Get()
	if err != nil {
		return workspaceevents.QueryPreparationDiagnosticResult{}, err
	}
	response, ok := value.(workspaceevents.QueryPreparationDiagnosticResult)
	if !ok || response.ValidateFor(command) != nil {
		return workspaceevents.QueryPreparationDiagnosticResult{}, common.ErrConflict
	}
	return response.DeepClone(), nil
}

func (b *Biz) QueryAuthoringDiagnostic(ctx context.Context, command workspaceevents.QueryAuthoringDiagnosticCommand) (workspaceevents.QueryAuthoringDiagnosticResult, error) {
	if b == nil || b.EventHub() == nil || ctx == nil || command.Validate() != nil {
		return workspaceevents.QueryAuthoringDiagnosticResult{}, common.ErrInvalidArgument
	}
	if err := ctx.Err(); err != nil {
		return workspaceevents.QueryAuthoringDiagnosticResult{}, err
	}
	ev := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceAuthoringDiagnosticQuery, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(workspaceevents.Contracts(), workspaceevents.TopicWorkspaceAuthoringDiagnosticQuery, eventcontract.LaneScope{WorkspaceID: string(command.WorkspaceID)}))
	result := b.SendEvent(ev)
	if result == nil {
		return workspaceevents.QueryAuthoringDiagnosticResult{}, common.ErrConflict
	}
	if err := result.Error(); err != nil {
		return workspaceevents.QueryAuthoringDiagnosticResult{}, err
	}
	value, err := result.Get()
	if err != nil {
		return workspaceevents.QueryAuthoringDiagnosticResult{}, err
	}
	response, ok := value.(workspaceevents.QueryAuthoringDiagnosticResult)
	if !ok || response.ValidateFor(command) != nil {
		return workspaceevents.QueryAuthoringDiagnosticResult{}, common.ErrConflict
	}
	d := response.Diagnostic.Clone()
	response.Diagnostic = d
	return response, nil
}
