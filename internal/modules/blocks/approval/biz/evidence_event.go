package approval

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/common"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (s *Service) recordApprovalGovernanceEvidence(ctx context.Context, item *approvalmodel.Approval) error {
	if s == nil || s.commandHub == nil || item == nil || item.ID == "" {
		return nil
	}
	if item.State != common.ApprovalStateApproved && item.State != common.ApprovalStateRejected {
		return nil
	}
	contextFields, err := approvalEvidenceFields(item.Context)
	if err != nil {
		return err
	}
	timestamp := strings.TrimSpace(item.UpdatedAt)
	if timestamp == "" {
		timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	command := approvalevents.RecordApprovalEvidenceCommand{
		EvidenceRecordID: orchestratormodel.StableEvidenceRecordID(string(item.ID), "approval", "approval"),
		WorkspaceID:      item.WorkspaceID,
		RunID:            item.RunID,
		StepID:           item.StepID,
		ApprovalID:       item.ID,
		ApprovalState:    item.State,
		Reason:           strings.TrimSpace(item.Reason),
		Action:           strings.TrimSpace(item.Action),
		Context:          contextFields,
		Timestamp:        timestamp,
	}
	ev := magicevent.NewEventWithContext(
		approvalevents.TopicApprovalCommandRecordEvidence,
		modulecommon.ModuleID,
		approvalevents.EvidenceCommandDestination,
		magicevent.NewHeader(),
		ctx,
		command,
	)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(ev.ID(), workorchevents.EventHubLaneScope{
		SubjectType: "approval",
		SubjectID:   string(item.ID),
		WorkspaceID: string(item.WorkspaceID),
		RunID:       string(item.RunID),
		StepID:      string(item.StepID),
	}))
	result := s.commandHub.Send(ev)
	if result == nil {
		return fmt.Errorf("approval evidence command returned no result")
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
	response, ok := data.(approvalevents.RecordApprovalEvidenceResult)
	if !ok || response.EvidenceRecordID != command.EvidenceRecordID {
		return fmt.Errorf("%w: approval evidence result type %T", common.ErrInvalidArgument, data)
	}
	return nil
}

func approvalEvidenceFields(input map[string]any) ([]approvalevents.ApprovalEvidenceField, error) {
	if len(input) == 0 {
		return nil, nil
	}
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ret := make([]approvalevents.ApprovalEvidenceField, 0, len(keys))
	for _, key := range keys {
		value, err := approvalEvidenceValue(input[key])
		if err != nil {
			return nil, fmt.Errorf("approval evidence context %s: %w", key, err)
		}
		ret = append(ret, approvalevents.ApprovalEvidenceField{Name: key, Value: value})
	}
	return ret, nil
}

func approvalEvidenceValue(input any) (approvalevents.ApprovalEvidenceValue, error) {
	switch value := input.(type) {
	case nil:
		return approvalevents.ApprovalEvidenceValue{Kind: approvalevents.ApprovalEvidenceValueNull}, nil
	case string:
		return approvalevents.ApprovalEvidenceValue{Kind: approvalevents.ApprovalEvidenceValueString, String: value}, nil
	case bool:
		return approvalevents.ApprovalEvidenceValue{Kind: approvalevents.ApprovalEvidenceValueBool, Bool: value}, nil
	case int:
		return approvalevents.ApprovalEvidenceValue{Kind: approvalevents.ApprovalEvidenceValueInt, Int: int64(value)}, nil
	case int8:
		return approvalevents.ApprovalEvidenceValue{Kind: approvalevents.ApprovalEvidenceValueInt, Int: int64(value)}, nil
	case int16:
		return approvalevents.ApprovalEvidenceValue{Kind: approvalevents.ApprovalEvidenceValueInt, Int: int64(value)}, nil
	case int32:
		return approvalevents.ApprovalEvidenceValue{Kind: approvalevents.ApprovalEvidenceValueInt, Int: int64(value)}, nil
	case int64:
		return approvalevents.ApprovalEvidenceValue{Kind: approvalevents.ApprovalEvidenceValueInt, Int: value}, nil
	case uint:
		return approvalevents.ApprovalEvidenceValue{Kind: approvalevents.ApprovalEvidenceValueUint, Uint: uint64(value)}, nil
	case uint8:
		return approvalevents.ApprovalEvidenceValue{Kind: approvalevents.ApprovalEvidenceValueUint, Uint: uint64(value)}, nil
	case uint16:
		return approvalevents.ApprovalEvidenceValue{Kind: approvalevents.ApprovalEvidenceValueUint, Uint: uint64(value)}, nil
	case uint32:
		return approvalevents.ApprovalEvidenceValue{Kind: approvalevents.ApprovalEvidenceValueUint, Uint: uint64(value)}, nil
	case uint64:
		return approvalevents.ApprovalEvidenceValue{Kind: approvalevents.ApprovalEvidenceValueUint, Uint: value}, nil
	case float32:
		return approvalevents.ApprovalEvidenceValue{Kind: approvalevents.ApprovalEvidenceValueFloat, Float: float64(value)}, nil
	case float64:
		return approvalevents.ApprovalEvidenceValue{Kind: approvalevents.ApprovalEvidenceValueFloat, Float: value}, nil
	case map[string]any:
		fields, err := approvalEvidenceFields(value)
		return approvalevents.ApprovalEvidenceValue{Kind: approvalevents.ApprovalEvidenceValueObject, Object: fields}, err
	case []any:
		items := make([]approvalevents.ApprovalEvidenceValue, 0, len(value))
		for _, item := range value {
			converted, err := approvalEvidenceValue(item)
			if err != nil {
				return approvalevents.ApprovalEvidenceValue{}, err
			}
			items = append(items, converted)
		}
		return approvalevents.ApprovalEvidenceValue{Kind: approvalevents.ApprovalEvidenceValueArray, Array: items}, nil
	case []string:
		items := make([]approvalevents.ApprovalEvidenceValue, 0, len(value))
		for _, item := range value {
			items = append(items, approvalevents.ApprovalEvidenceValue{Kind: approvalevents.ApprovalEvidenceValueString, String: item})
		}
		return approvalevents.ApprovalEvidenceValue{Kind: approvalevents.ApprovalEvidenceValueArray, Array: items}, nil
	default:
		return approvalevents.ApprovalEvidenceValue{}, fmt.Errorf("%w: unsupported value type %T", common.ErrInvalidArgument, input)
	}
}
