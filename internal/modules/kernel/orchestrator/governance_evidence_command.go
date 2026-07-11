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
	payload, err := decisionEvidenceFieldsToMap(command.ResolutionPayload)
	if err != nil {
		return decisionevents.RecordDecisionEvidenceResult{}, err
	}
	metadata := map[string]any{
		"ticketId":       string(command.TicketID),
		"resolutionId":   string(command.ResolutionID),
		"resolutionType": strings.TrimSpace(command.ResolutionType),
		"decisionKind":   strings.TrimSpace(command.DecisionKind),
	}
	if command.Reason != "" {
		metadata["reason"] = strings.TrimSpace(command.Reason)
	}
	if command.Question != "" {
		metadata["question"] = strings.TrimSpace(command.Question)
	}
	if len(payload) > 0 {
		metadata["resolutionPayload"] = payload
	}
	if command.ContinuationAction != "" {
		metadata["continuationAction"] = strings.TrimSpace(command.ContinuationAction)
	}
	if command.ResumeRunID != "" {
		metadata["resumeRunId"] = string(command.ResumeRunID)
	}
	if command.SuccessorRunID != "" {
		metadata["successorRunId"] = string(command.SuccessorRunID)
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
		Metadata:    metadata,
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
	approvalContext, err := approvalEvidenceFieldsToMap(command.Context)
	if err != nil {
		return approvalevents.RecordApprovalEvidenceResult{}, err
	}
	metadata := map[string]any{
		"approvalId":    string(command.ApprovalID),
		"approvalState": string(command.ApprovalState),
	}
	if command.Reason != "" {
		metadata["reason"] = strings.TrimSpace(command.Reason)
	}
	if command.Action != "" {
		metadata["action"] = strings.TrimSpace(command.Action)
	}
	if len(approvalContext) > 0 {
		metadata["context"] = approvalContext
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
		Metadata:    metadata,
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

func decisionEvidenceFieldsToMap(fields []decisionevents.DecisionEvidenceField) (map[string]any, error) {
	if len(fields) == 0 {
		return nil, nil
	}
	ret := make(map[string]any, len(fields))
	for _, field := range fields {
		if strings.TrimSpace(field.Name) == "" {
			return nil, common.ErrInvalidArgument
		}
		value, err := decisionEvidenceValueToAny(field.Value)
		if err != nil {
			return nil, fmt.Errorf("decision evidence field %s: %w", field.Name, err)
		}
		ret[field.Name] = value
	}
	return ret, nil
}

func decisionEvidenceValueToAny(value decisionevents.DecisionEvidenceValue) (any, error) {
	switch value.Kind {
	case decisionevents.DecisionEvidenceValueNull:
		return nil, nil
	case decisionevents.DecisionEvidenceValueString:
		return value.String, nil
	case decisionevents.DecisionEvidenceValueBool:
		return value.Bool, nil
	case decisionevents.DecisionEvidenceValueInt:
		return value.Int, nil
	case decisionevents.DecisionEvidenceValueUint:
		return value.Uint, nil
	case decisionevents.DecisionEvidenceValueFloat:
		return value.Float, nil
	case decisionevents.DecisionEvidenceValueObject:
		return decisionEvidenceFieldsToMap(value.Object)
	case decisionevents.DecisionEvidenceValueArray:
		items := make([]any, 0, len(value.Array))
		for _, item := range value.Array {
			converted, err := decisionEvidenceValueToAny(item)
			if err != nil {
				return nil, err
			}
			items = append(items, converted)
		}
		return items, nil
	default:
		return nil, common.ErrInvalidArgument
	}
}

func approvalEvidenceFieldsToMap(fields []approvalevents.ApprovalEvidenceField) (map[string]any, error) {
	if len(fields) == 0 {
		return nil, nil
	}
	ret := make(map[string]any, len(fields))
	for _, field := range fields {
		if strings.TrimSpace(field.Name) == "" {
			return nil, common.ErrInvalidArgument
		}
		value, err := approvalEvidenceValueToAny(field.Value)
		if err != nil {
			return nil, fmt.Errorf("approval evidence field %s: %w", field.Name, err)
		}
		ret[field.Name] = value
	}
	return ret, nil
}

func approvalEvidenceValueToAny(value approvalevents.ApprovalEvidenceValue) (any, error) {
	switch value.Kind {
	case approvalevents.ApprovalEvidenceValueNull:
		return nil, nil
	case approvalevents.ApprovalEvidenceValueString:
		return value.String, nil
	case approvalevents.ApprovalEvidenceValueBool:
		return value.Bool, nil
	case approvalevents.ApprovalEvidenceValueInt:
		return value.Int, nil
	case approvalevents.ApprovalEvidenceValueUint:
		return value.Uint, nil
	case approvalevents.ApprovalEvidenceValueFloat:
		return value.Float, nil
	case approvalevents.ApprovalEvidenceValueObject:
		return approvalEvidenceFieldsToMap(value.Object)
	case approvalevents.ApprovalEvidenceValueArray:
		items := make([]any, 0, len(value.Array))
		for _, item := range value.Array {
			converted, err := approvalEvidenceValueToAny(item)
			if err != nil {
				return nil, err
			}
			items = append(items, converted)
		}
		return items, nil
	default:
		return nil, common.ErrInvalidArgument
	}
}
