package appservice

import (
	"context"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func (d *CLICommandDispatcher) publishDecisionContinuation(ctx context.Context, cmd events.CLICommandPayload, item appmodelcore.Decision) error {
	intent := "resume"
	reason := "decision_resolved"
	if strings.TrimSpace(item.ResolutionType) == decisioncommon.ResolutionTypeReplan {
		intent = "replan"
		reason = "decision_replan_resolved"
	}
	return d.publishGovernanceContinuation(ctx, cmd, governanceContinuationRef{
		WorkspaceID: item.WorkspaceID,
		RunID:       item.RunID,
		Intent:      intent,
		Reason:      reason,
		RequestID:   cliGovernanceContinuationRequestID(cmd, "decision", item.ID, intent),
		DecisionID:  item.ID,
	})
}

func (d *CLICommandDispatcher) publishDecisionContinuations(ctx context.Context, cmd events.CLICommandPayload, items []appmodelcore.Decision) error {
	for _, item := range items {
		if err := d.publishDecisionContinuation(ctx, cmd, item); err != nil {
			return err
		}
	}
	return nil
}

func (d *CLICommandDispatcher) publishApprovalContinuation(ctx context.Context, cmd events.CLICommandPayload, item appmodelcore.Approval) error {
	return d.publishGovernanceContinuation(ctx, cmd, governanceContinuationRef{
		WorkspaceID: item.WorkspaceID,
		RunID:       item.RunID,
		Intent:      "resume",
		Reason:      "approval_resolved",
		RequestID:   cliGovernanceContinuationRequestID(cmd, "approval", item.ID, "resume"),
	})
}

func (d *CLICommandDispatcher) publishApprovalContinuations(ctx context.Context, cmd events.CLICommandPayload, items []appmodelcore.Approval) error {
	for _, item := range items {
		if err := d.publishApprovalContinuation(ctx, cmd, item); err != nil {
			return err
		}
	}
	return nil
}

type governanceContinuationRef struct {
	WorkspaceID string
	RunID       string
	Intent      string
	Reason      string
	RequestID   string
	DecisionID  string
}

func (d *CLICommandDispatcher) publishGovernanceContinuation(ctx context.Context, cmd events.CLICommandPayload, ref governanceContinuationRef) error {
	if d == nil || d.eventHub == nil {
		return common.ErrInvalidArgument
	}
	workspaceID := strings.TrimSpace(ref.WorkspaceID)
	runID := strings.TrimSpace(ref.RunID)
	intent := strings.TrimSpace(ref.Intent)
	if workspaceID == "" || runID == "" || intent == "" {
		return common.ErrInvalidArgument
	}
	payload := map[string]any{
		events.PayloadKeyWorkspaceID: workspaceID,
		events.PayloadKeyRunID:       runID,
		events.PayloadKeyIntent:      intent,
		events.PayloadKeyReason:      strings.TrimSpace(ref.Reason),
		events.PayloadKeyRequestID:   strings.TrimSpace(ref.RequestID),
		events.PayloadKeySource:      "cli",
	}
	if strings.TrimSpace(ref.DecisionID) != "" {
		payload[events.PayloadKeyDecisionID] = strings.TrimSpace(ref.DecisionID)
	}
	if priority, ok := cliPayloadInt(cmd.Payload, events.PayloadKeyPriority); ok {
		payload[events.PayloadKeyPriority] = priority
	}
	err := d.eventHub.Publish(ctx, events.Envelope{
		Topic:        workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested,
		SourceModule: "workorchd-cli-dispatcher",
		SubjectType:  "workspace",
		SubjectID:    workspaceID,
		WorkspaceID:  workspaceID,
		RunID:        runID,
		Payload:      payload,
	})
	response, ok := events.ResponseEnvelope(err)
	if !ok {
		if err != nil {
			return err
		}
		return fmt.Errorf("%w: scheduler command %s returned no accepted response", common.ErrConflict, workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested)
	}
	if strings.TrimSpace(response.Topic) != workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested+"/accepted" {
		return fmt.Errorf("%w: scheduler command %s returned unexpected response topic %s", common.ErrConflict, workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, response.Topic)
	}
	if accepted, _ := response.Payload["accepted"].(bool); !accepted {
		return fmt.Errorf("%w: scheduler command %s was not accepted", common.ErrConflict, workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested)
	}
	return nil
}

func cliGovernanceContinuationRequestID(cmd events.CLICommandPayload, kind string, id string, intent string) string {
	if value := strings.TrimSpace(cmd.CommandID); value != "" {
		return strings.Join([]string{value, strings.TrimSpace(kind), strings.TrimSpace(id), strings.TrimSpace(intent)}, "-")
	}
	parts := []string{"cli-governance", strings.TrimSpace(kind), strings.TrimSpace(id), strings.TrimSpace(intent)}
	for idx, part := range parts {
		if part == "" {
			parts[idx] = "none"
		}
	}
	return strings.Join(parts, "-")
}

func cliPayloadInt(payload map[string]any, key string) (int, bool) {
	if payload == nil {
		return 0, false
	}
	switch value := payload[key].(type) {
	case int:
		return value, true
	case int64:
		return int(value), true
	case float64:
		return int(value), true
	default:
		return 0, false
	}
}
