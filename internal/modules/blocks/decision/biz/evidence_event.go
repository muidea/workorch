package decision

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (s *Service) recordDecisionResolutionEvidence(ctx context.Context, ticket *decisionmodel.Ticket, resolution *decisionmodel.Resolution) error {
	if s == nil || s.commandHub == nil || ticket == nil || resolution == nil || resolution.ID == "" {
		return nil
	}
	payload, err := decisionEvidenceFields(resolution.Payload)
	if err != nil {
		return err
	}
	timestamp := strings.TrimSpace(ticket.UpdatedAt)
	if timestamp == "" {
		timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	command := decisionevents.RecordDecisionEvidenceCommand{
		EvidenceRecordID:   orchestratormodel.StableEvidenceRecordID(string(resolution.ID), "decision-resolution", "decision"),
		WorkspaceID:        ticket.WorkspaceID,
		RunID:              ticket.RunID,
		StepID:             ticket.StepID,
		TicketID:           ticket.ID,
		ResolutionID:       resolution.ID,
		ResolutionType:     strings.TrimSpace(resolution.Type),
		DecisionKind:       strings.TrimSpace(ticket.DecisionKind),
		Reason:             strings.TrimSpace(ticket.Reason),
		Question:           strings.TrimSpace(ticket.Question),
		ResolutionPayload:  payload,
		ContinuationAction: strings.TrimSpace(resolution.ContinuationAction),
		ResumeRunID:        resolution.ResumeRunID,
		SuccessorRunID:     resolution.SuccessorRunID,
		Timestamp:          timestamp,
	}
	ev := magicevent.NewEventWithContext(
		decisionevents.TopicDecisionCommandRecordEvidence,
		modulecommon.ModuleID,
		decisionevents.EvidenceCommandDestination,
		magicevent.NewHeader(),
		ctx,
		command,
	)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(ev.ID(), workorchevents.EventHubLaneScope{
		SubjectType: "decision_ticket",
		SubjectID:   string(ticket.ID),
		WorkspaceID: string(ticket.WorkspaceID),
		RunID:       string(ticket.RunID),
		StepID:      string(ticket.StepID),
	}))
	result := s.commandHub.Send(ev)
	if result == nil {
		return fmt.Errorf("decision evidence command returned no result")
	}
	if resultErr := result.Error(); resultErr != nil {
		if resultErr.Cause != nil {
			return resultErr.Cause
		}
		return resultErr
	}
	data, resultErr := result.Get()
	if resultErr != nil {
		if resultErr.Cause != nil {
			return resultErr.Cause
		}
		return resultErr
	}
	response, ok := data.(decisionevents.RecordDecisionEvidenceResult)
	if !ok || response.EvidenceRecordID != command.EvidenceRecordID {
		return fmt.Errorf("%w: decision evidence result type %T", common.ErrInvalidArgument, data)
	}
	return nil
}

func decisionEvidenceFields(input map[string]any) ([]decisionevents.DecisionEvidenceField, error) {
	if len(input) == 0 {
		return nil, nil
	}
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ret := make([]decisionevents.DecisionEvidenceField, 0, len(keys))
	for _, key := range keys {
		value, err := decisionEvidenceValue(input[key])
		if err != nil {
			return nil, fmt.Errorf("decision evidence payload %s: %w", key, err)
		}
		ret = append(ret, decisionevents.DecisionEvidenceField{Name: key, Value: value})
	}
	return ret, nil
}

func decisionEvidenceValue(input any) (decisionevents.DecisionEvidenceValue, error) {
	switch value := input.(type) {
	case nil:
		return decisionevents.DecisionEvidenceValue{Kind: decisionevents.DecisionEvidenceValueNull}, nil
	case string:
		return decisionevents.DecisionEvidenceValue{Kind: decisionevents.DecisionEvidenceValueString, String: value}, nil
	case bool:
		return decisionevents.DecisionEvidenceValue{Kind: decisionevents.DecisionEvidenceValueBool, Bool: value}, nil
	case int:
		return decisionevents.DecisionEvidenceValue{Kind: decisionevents.DecisionEvidenceValueInt, Int: int64(value)}, nil
	case int8:
		return decisionevents.DecisionEvidenceValue{Kind: decisionevents.DecisionEvidenceValueInt, Int: int64(value)}, nil
	case int16:
		return decisionevents.DecisionEvidenceValue{Kind: decisionevents.DecisionEvidenceValueInt, Int: int64(value)}, nil
	case int32:
		return decisionevents.DecisionEvidenceValue{Kind: decisionevents.DecisionEvidenceValueInt, Int: int64(value)}, nil
	case int64:
		return decisionevents.DecisionEvidenceValue{Kind: decisionevents.DecisionEvidenceValueInt, Int: value}, nil
	case uint:
		return decisionevents.DecisionEvidenceValue{Kind: decisionevents.DecisionEvidenceValueUint, Uint: uint64(value)}, nil
	case uint8:
		return decisionevents.DecisionEvidenceValue{Kind: decisionevents.DecisionEvidenceValueUint, Uint: uint64(value)}, nil
	case uint16:
		return decisionevents.DecisionEvidenceValue{Kind: decisionevents.DecisionEvidenceValueUint, Uint: uint64(value)}, nil
	case uint32:
		return decisionevents.DecisionEvidenceValue{Kind: decisionevents.DecisionEvidenceValueUint, Uint: uint64(value)}, nil
	case uint64:
		return decisionevents.DecisionEvidenceValue{Kind: decisionevents.DecisionEvidenceValueUint, Uint: value}, nil
	case float32:
		return decisionevents.DecisionEvidenceValue{Kind: decisionevents.DecisionEvidenceValueFloat, Float: float64(value)}, nil
	case float64:
		return decisionevents.DecisionEvidenceValue{Kind: decisionevents.DecisionEvidenceValueFloat, Float: value}, nil
	case map[string]any:
		fields, err := decisionEvidenceFields(value)
		return decisionevents.DecisionEvidenceValue{Kind: decisionevents.DecisionEvidenceValueObject, Object: fields}, err
	case []any:
		items := make([]decisionevents.DecisionEvidenceValue, 0, len(value))
		for _, item := range value {
			converted, err := decisionEvidenceValue(item)
			if err != nil {
				return decisionevents.DecisionEvidenceValue{}, err
			}
			items = append(items, converted)
		}
		return decisionevents.DecisionEvidenceValue{Kind: decisionevents.DecisionEvidenceValueArray, Array: items}, nil
	case []string:
		items := make([]decisionevents.DecisionEvidenceValue, 0, len(value))
		for _, item := range value {
			items = append(items, decisionevents.DecisionEvidenceValue{Kind: decisionevents.DecisionEvidenceValueString, String: item})
		}
		return decisionevents.DecisionEvidenceValue{Kind: decisionevents.DecisionEvidenceValueArray, Array: items}, nil
	default:
		return decisionevents.DecisionEvidenceValue{}, fmt.Errorf("%w: unsupported value type %T", common.ErrInvalidArgument, input)
	}
}
