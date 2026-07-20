package appservice

import (
	"context"
	"fmt"
	"strings"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func (d *CLICommandDispatcher) publishDecisionContinuationForRequest(ctx context.Context, requestID string, item appmodelcore.Decision) error {
	reason := "decision_resolved"
	if item.ResolutionType == decisioncommon.ResolutionTypeReplan {
		reason = "decision_replan_resolved"
	}
	// A replan-style resolution is an audited Decision intent, never a second
	// Runtime V2 control verb. The scheduler receives the one supported
	// continuation command and Orchestrator re-drives the persisted aggregate.
	const intent = "resume"
	return d.publishGovernanceContinuation(ctx, governanceContinuationRef{
		WorkspaceID: item.WorkspaceID,
		RunID:       item.RunID,
		Intent:      intent,
		Reason:      reason,
		RequestID:   cliGovernanceContinuationRequestID(requestID, "decision", item.ID, intent),
		DecisionID:  item.ID,
	})
}

func (d *CLICommandDispatcher) publishDecisionContinuationsForRequest(ctx context.Context, requestID string, items []appmodelcore.Decision) error {
	for _, item := range items {
		if err := d.publishDecisionContinuationForRequest(ctx, requestID, item); err != nil {
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

func (d *CLICommandDispatcher) publishGovernanceContinuation(ctx context.Context, ref governanceContinuationRef) error {
	if d == nil || d.session == nil {
		return common.ErrInvalidArgument
	}
	workspaceID := strings.TrimSpace(ref.WorkspaceID)
	runID := strings.TrimSpace(ref.RunID)
	intent := strings.TrimSpace(ref.Intent)
	if workspaceID == "" || runID == "" || intent == "" {
		return common.ErrInvalidArgument
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
		Priority:    0,
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

func cliGovernanceContinuationRequestID(requestID string, kind string, id string, intent string) string {
	if value := strings.TrimSpace(requestID); value != "" {
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
