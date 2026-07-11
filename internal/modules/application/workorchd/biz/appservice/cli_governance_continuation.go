package appservice

import (
	"context"
	"fmt"
	"strings"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/clicore"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func (d *CLICommandDispatcher) publishDecisionContinuation(ctx context.Context, cmd clicore.Command, item appmodelcore.Decision) error {
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

func (d *CLICommandDispatcher) publishDecisionContinuations(ctx context.Context, cmd clicore.Command, items []appmodelcore.Decision) error {
	for _, item := range items {
		if err := d.publishDecisionContinuation(ctx, cmd, item); err != nil {
			return err
		}
	}
	return nil
}

func (d *CLICommandDispatcher) publishApprovalContinuation(ctx context.Context, cmd clicore.Command, item appmodelcore.Approval) error {
	return d.publishGovernanceContinuation(ctx, cmd, governanceContinuationRef{
		WorkspaceID: item.WorkspaceID,
		RunID:       item.RunID,
		Intent:      "resume",
		Reason:      "approval_resolved",
		RequestID:   cliGovernanceContinuationRequestID(cmd, "approval", item.ID, "resume"),
	})
}

func (d *CLICommandDispatcher) publishApprovalContinuations(ctx context.Context, cmd clicore.Command, items []appmodelcore.Approval) error {
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

func (d *CLICommandDispatcher) publishGovernanceContinuation(ctx context.Context, cmd clicore.Command, ref governanceContinuationRef) error {
	if d == nil || d.session == nil {
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
	commandHub, err := d.session.RequiredCommandHub()
	if err != nil {
		return err
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
	event.BindLaneKey(events.EventHubLaneKeyFor(event.ID(), events.EventHubLaneScope{SubjectType: "workspace", SubjectID: workspaceID, WorkspaceID: workspaceID, RunID: runID}))
	response := commandHub.Send(event)
	if response == nil {
		return fmt.Errorf("%w: scheduler command %s returned no result", common.ErrConflict, event.ID())
	}
	if err := cd.ToStdError(response.Error()); err != nil {
		return err
	}
	data, resultErr := response.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return err
	}
	result, ok := data.(workorchdevents.WorkspaceRunRequestedResult)
	if !ok || !result.Accepted {
		return fmt.Errorf("%w: scheduler command %s was not accepted", common.ErrConflict, workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested)
	}
	return nil
}

func cliGovernanceContinuationRequestID(cmd clicore.Command, kind string, id string, intent string) string {
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
