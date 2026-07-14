package orchestrator

import (
	"context"
	"fmt"
	"strings"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (m *Module) handleDecisionEvidenceCommand(ev magicevent.Event, result magicevent.Result) {
	command, ok := ev.Data().(decisionevents.RecordDecisionEvidenceCommand)
	if !ok {
		setDecisionEvidenceResult(result, decisionevents.RecordDecisionEvidenceResult{}, common.ErrInvalidArgument)
		return
	}
	response, err := m.recordDecisionEvidence(ev.Context(), command)
	setDecisionEvidenceResult(result, response, err)
}

func (m *Module) recordDecisionEvidence(ctx context.Context, command decisionevents.RecordDecisionEvidenceCommand) (decisionevents.RecordDecisionEvidenceResult, error) {
	if m == nil || m.evidenceRecords == nil || command.EvidenceRecordID == "" || command.WorkspaceID == "" || command.RunID == "" || command.TicketID == "" || command.ResolutionID == "" {
		return decisionevents.RecordDecisionEvidenceResult{}, common.ErrInvalidArgument
	}
	payload, err := decisionEvidenceFieldsToPayload(command.ResolutionPayload)
	if err != nil {
		return decisionevents.RecordDecisionEvidenceResult{}, err
	}
	metadataFields := []workorcheventpayload.Field{
		workorcheventpayload.StringField("ticketId", string(command.TicketID)),
		workorcheventpayload.StringField("resolutionId", string(command.ResolutionID)),
		workorcheventpayload.StringField("resolutionType", strings.TrimSpace(command.ResolutionType)),
		workorcheventpayload.StringField("decisionKind", strings.TrimSpace(command.DecisionKind)),
	}
	if command.Reason != "" {
		metadataFields = append(metadataFields, workorcheventpayload.StringField("reason", strings.TrimSpace(command.Reason)))
	}
	if command.Question != "" {
		metadataFields = append(metadataFields, workorcheventpayload.StringField("question", strings.TrimSpace(command.Question)))
	}
	if len(payload.Fields) > 0 {
		metadataFields = append(metadataFields, workorcheventpayload.ObjectField("resolutionPayload", payload))
	}
	if command.ContinuationAction != "" {
		metadataFields = append(metadataFields, workorcheventpayload.StringField("continuationAction", strings.TrimSpace(command.ContinuationAction)))
	}
	if command.ResumeRunID != "" {
		metadataFields = append(metadataFields, workorcheventpayload.StringField("resumeRunId", string(command.ResumeRunID)))
	}
	if command.SuccessorRunID != "" {
		metadataFields = append(metadataFields, workorcheventpayload.StringField("successorRunId", string(command.SuccessorRunID)))
	}
	record := &orchestratormodel.EvidenceRecord{
		ID:          command.EvidenceRecordID,
		WorkspaceID: command.WorkspaceID,
		RunID:       command.RunID,
		StepID:      command.StepID,
		Kind:        orchestratormodel.EvidenceKindDecision,
		Status:      orchestratormodel.EvidenceStatusAccepted,
		SourceType:  "decision_resolution",
		SourceRef:   string(command.ResolutionID),
		SubjectType: "decision_ticket",
		SubjectID:   string(command.TicketID),
		CreatedAt:   command.Timestamp,
		UpdatedAt:   command.Timestamp,
		Metadata:    workorcheventpayload.New(metadataFields...),
	}
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID)
	if err := repository.CreateOrUpdateEvidenceRecord(ctx, m.evidenceRecords, record); err != nil {
		return decisionevents.RecordDecisionEvidenceResult{}, err
	}
	return decisionevents.RecordDecisionEvidenceResult{EvidenceRecordID: command.EvidenceRecordID}, nil
}

func setDecisionEvidenceResult(result magicevent.Result, response decisionevents.RecordDecisionEvidenceResult, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(response, nil)
}

func (m *Module) handleApprovalEvidenceCommand(ev magicevent.Event, result magicevent.Result) {
	command, ok := ev.Data().(approvalevents.RecordApprovalEvidenceCommand)
	if !ok {
		setApprovalEvidenceResult(result, approvalevents.RecordApprovalEvidenceResult{}, common.ErrInvalidArgument)
		return
	}
	response, err := m.recordApprovalEvidence(ev.Context(), command)
	setApprovalEvidenceResult(result, response, err)
}

func (m *Module) recordApprovalEvidence(ctx context.Context, command approvalevents.RecordApprovalEvidenceCommand) (approvalevents.RecordApprovalEvidenceResult, error) {
	if m == nil || m.evidenceRecords == nil || command.EvidenceRecordID == "" || command.WorkspaceID == "" || command.RunID == "" || command.ApprovalID == "" {
		return approvalevents.RecordApprovalEvidenceResult{}, common.ErrInvalidArgument
	}
	status := ""
	switch command.ApprovalState {
	case common.ApprovalStateApproved:
		status = orchestratormodel.EvidenceStatusAccepted
	case common.ApprovalStateRejected:
		status = orchestratormodel.EvidenceStatusRejected
	default:
		return approvalevents.RecordApprovalEvidenceResult{}, common.ErrInvalidArgument
	}
	approvalContext, err := approvalEvidenceFieldsToPayload(command.Context)
	if err != nil {
		return approvalevents.RecordApprovalEvidenceResult{}, err
	}
	metadataFields := []workorcheventpayload.Field{
		workorcheventpayload.StringField("approvalId", string(command.ApprovalID)),
		workorcheventpayload.StringField("approvalState", string(command.ApprovalState)),
	}
	if command.Reason != "" {
		metadataFields = append(metadataFields, workorcheventpayload.StringField("reason", strings.TrimSpace(command.Reason)))
	}
	if command.Action != "" {
		metadataFields = append(metadataFields, workorcheventpayload.StringField("action", strings.TrimSpace(command.Action)))
	}
	if len(approvalContext.Fields) > 0 {
		metadataFields = append(metadataFields, workorcheventpayload.ObjectField("context", approvalContext))
	}
	record := &orchestratormodel.EvidenceRecord{
		ID:          command.EvidenceRecordID,
		WorkspaceID: command.WorkspaceID,
		RunID:       command.RunID,
		StepID:      command.StepID,
		Kind:        orchestratormodel.EvidenceKindApproval,
		Status:      status,
		SourceType:  "approval",
		SourceRef:   string(command.ApprovalID),
		SubjectType: "approval",
		SubjectID:   string(command.ApprovalID),
		CreatedAt:   command.Timestamp,
		UpdatedAt:   command.Timestamp,
		Metadata:    workorcheventpayload.New(metadataFields...),
	}
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID)
	if err := repository.CreateOrUpdateEvidenceRecord(ctx, m.evidenceRecords, record); err != nil {
		return approvalevents.RecordApprovalEvidenceResult{}, err
	}
	return approvalevents.RecordApprovalEvidenceResult{EvidenceRecordID: command.EvidenceRecordID}, nil
}

func setApprovalEvidenceResult(result magicevent.Result, response approvalevents.RecordApprovalEvidenceResult, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(response, nil)
}

func decisionEvidenceFieldsToPayload(fields []decisionevents.DecisionEvidenceField) (workorcheventpayload.Payload, error) {
	if len(fields) == 0 {
		return workorcheventpayload.Payload{}, nil
	}
	ret := make([]workorcheventpayload.Field, 0, len(fields))
	for _, field := range fields {
		if strings.TrimSpace(field.Name) == "" {
			return workorcheventpayload.Payload{}, common.ErrInvalidArgument
		}
		value, err := decisionEvidenceValueToPayloadValue(field.Value)
		if err != nil {
			return workorcheventpayload.Payload{}, fmt.Errorf("decision evidence field %s: %w", field.Name, err)
		}
		ret = append(ret, workorcheventpayload.Field{Name: field.Name, Value: value})
	}
	return workorcheventpayload.New(ret...), nil
}

func decisionEvidenceValueToPayloadValue(value decisionevents.DecisionEvidenceValue) (workorcheventpayload.Value, error) {
	switch value.Kind {
	case decisionevents.DecisionEvidenceValueNull:
		return workorcheventpayload.Value{Kind: workorcheventpayload.ValueNull}, nil
	case decisionevents.DecisionEvidenceValueString:
		return workorcheventpayload.Value{Kind: workorcheventpayload.ValueString, String: value.String}, nil
	case decisionevents.DecisionEvidenceValueBool:
		return workorcheventpayload.Value{Kind: workorcheventpayload.ValueBool, Bool: value.Bool}, nil
	case decisionevents.DecisionEvidenceValueInt:
		return workorcheventpayload.Value{Kind: workorcheventpayload.ValueInt, Int: value.Int}, nil
	case decisionevents.DecisionEvidenceValueUint:
		return workorcheventpayload.Value{Kind: workorcheventpayload.ValueUint, Uint: value.Uint}, nil
	case decisionevents.DecisionEvidenceValueFloat:
		return workorcheventpayload.Value{Kind: workorcheventpayload.ValueFloat, Float: value.Float}, nil
	case decisionevents.DecisionEvidenceValueObject:
		payload, err := decisionEvidenceFieldsToPayload(value.Object)
		return workorcheventpayload.Value{Kind: workorcheventpayload.ValueObject, Object: payload.Fields}, err
	case decisionevents.DecisionEvidenceValueArray:
		items := make([]workorcheventpayload.Value, 0, len(value.Array))
		for _, item := range value.Array {
			converted, err := decisionEvidenceValueToPayloadValue(item)
			if err != nil {
				return workorcheventpayload.Value{}, err
			}
			items = append(items, converted)
		}
		return workorcheventpayload.Value{Kind: workorcheventpayload.ValueArray, Array: items}, nil
	default:
		return workorcheventpayload.Value{}, common.ErrInvalidArgument
	}
}

func approvalEvidenceFieldsToPayload(fields []approvalevents.ApprovalEvidenceField) (workorcheventpayload.Payload, error) {
	if len(fields) == 0 {
		return workorcheventpayload.Payload{}, nil
	}
	ret := make([]workorcheventpayload.Field, 0, len(fields))
	for _, field := range fields {
		if strings.TrimSpace(field.Name) == "" {
			return workorcheventpayload.Payload{}, common.ErrInvalidArgument
		}
		value, err := approvalEvidenceValueToPayloadValue(field.Value)
		if err != nil {
			return workorcheventpayload.Payload{}, fmt.Errorf("approval evidence field %s: %w", field.Name, err)
		}
		ret = append(ret, workorcheventpayload.Field{Name: field.Name, Value: value})
	}
	return workorcheventpayload.New(ret...), nil
}

func approvalEvidenceValueToPayloadValue(value approvalevents.ApprovalEvidenceValue) (workorcheventpayload.Value, error) {
	switch value.Kind {
	case approvalevents.ApprovalEvidenceValueNull:
		return workorcheventpayload.Value{Kind: workorcheventpayload.ValueNull}, nil
	case approvalevents.ApprovalEvidenceValueString:
		return workorcheventpayload.Value{Kind: workorcheventpayload.ValueString, String: value.String}, nil
	case approvalevents.ApprovalEvidenceValueBool:
		return workorcheventpayload.Value{Kind: workorcheventpayload.ValueBool, Bool: value.Bool}, nil
	case approvalevents.ApprovalEvidenceValueInt:
		return workorcheventpayload.Value{Kind: workorcheventpayload.ValueInt, Int: value.Int}, nil
	case approvalevents.ApprovalEvidenceValueUint:
		return workorcheventpayload.Value{Kind: workorcheventpayload.ValueUint, Uint: value.Uint}, nil
	case approvalevents.ApprovalEvidenceValueFloat:
		return workorcheventpayload.Value{Kind: workorcheventpayload.ValueFloat, Float: value.Float}, nil
	case approvalevents.ApprovalEvidenceValueObject:
		payload, err := approvalEvidenceFieldsToPayload(value.Object)
		return workorcheventpayload.Value{Kind: workorcheventpayload.ValueObject, Object: payload.Fields}, err
	case approvalevents.ApprovalEvidenceValueArray:
		items := make([]workorcheventpayload.Value, 0, len(value.Array))
		for _, item := range value.Array {
			converted, err := approvalEvidenceValueToPayloadValue(item)
			if err != nil {
				return workorcheventpayload.Value{}, err
			}
			items = append(items, converted)
		}
		return workorcheventpayload.Value{Kind: workorcheventpayload.ValueArray, Array: items}, nil
	default:
		return workorcheventpayload.Value{}, common.ErrInvalidArgument
	}
}
