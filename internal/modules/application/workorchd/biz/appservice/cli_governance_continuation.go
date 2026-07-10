package appservice

import (
	"context"
	"fmt"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
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
	priority := 0
	if parsedPriority, ok := cliPayloadInt(cmd.Payload, events.PayloadKeyPriority); ok {
		priority = parsedPriority
	}
	provider, ok := d.eventHub.(interface{ RawHub() magicevent.Hub })
	if !ok || provider.RawHub() == nil {
		return common.ErrInvalidArgument
	}
	command := workorchdevents.WorkspaceRunRequestedCommand{
		WorkspaceID: common.WorkspaceID(workspaceID),
		RunID:       common.RunID(runID),
		Intent:      intent,
		Reason:      strings.TrimSpace(ref.Reason),
		RequestID:   strings.TrimSpace(ref.RequestID),
		Source:      "cli",
		DecisionID:  strings.TrimSpace(ref.DecisionID),
		Priority:    priority,
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, "workorchd-cli-dispatcher", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	event.BindLaneKey(events.EventHubLaneKey(events.Envelope{Topic: event.ID(), SubjectType: "workspace", SubjectID: workspaceID, WorkspaceID: workspaceID, RunID: runID}))
	response := provider.RawHub().Send(event)
	if response == nil {
		return fmt.Errorf("%w: scheduler command %s returned no result", common.ErrConflict, event.ID())
	}
	if err := response.Error(); err != nil {
		return err
	}
	data, err := response.Get()
	if err != nil {
		return err
	}
	result, ok := data.(workorchdevents.WorkspaceRunRequestedResult)
	if !ok || !result.Accepted {
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
