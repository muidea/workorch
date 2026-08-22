package projection

import (
	"strings"
	"time"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceviewcore"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// RecordWorkspaceExecutionOutcome projects the concrete Workspace-owner
// admission result. Only the owner outcome decides whether the derived view
// is accepted or rejected; the application never infers a lifecycle state.
func (c *Component) RecordWorkspaceExecutionOutcome(_ string, command workorchdevents.WorkspaceRunRequestedCommand, outcome workorchdevents.WorkspaceRunRequestedResult) {
	if c == nil {
		return
	}
	workspaceID := strings.TrimSpace(string(outcome.WorkspaceID))
	if workspaceID == "" {
		workspaceID = strings.TrimSpace(string(command.WorkspaceID))
	}
	if workspaceID == "" {
		return
	}
	disposition := string(outcome.Outcome)
	diagnostic := runtimecontract.RuntimeExecutionIntentReasonSummary(outcome.Reason)
	if outcome.Outcome == runtimecontract.RuntimeExecutionIntentReconciliationRequested || outcome.Outcome == runtimecontract.RuntimeExecutionIntentReplacementPending {
		c.ApplyRunCommandAccepted(workspaceviewcore.RunCommandAcceptedUpdate{
			WorkspaceID:    workspaceID,
			RunID:          strings.TrimSpace(string(outcome.RunID)),
			Intent:         strings.TrimSpace(command.Intent),
			Reason:         strings.TrimSpace(command.Reason),
			Disposition:    disposition,
			Diagnostic:     diagnostic,
			RequestID:      strings.TrimSpace(command.RequestID),
			IdempotencyKey: executionIdempotencyKey(command),
			Source:         strings.TrimSpace(command.Source),
			AcceptedAt:     time.Now().UTC().Format(time.RFC3339Nano),
		})
		return
	}
	c.ApplyRunCommandRejected(workspaceviewcore.RunCommandRejectedUpdate{
		WorkspaceID:    workspaceID,
		RunID:          strings.TrimSpace(string(outcome.RunID)),
		Intent:         strings.TrimSpace(command.Intent),
		Reason:         strings.TrimSpace(command.Reason),
		Disposition:    disposition,
		Diagnostic:     diagnostic,
		RequestID:      strings.TrimSpace(command.RequestID),
		IdempotencyKey: executionIdempotencyKey(command),
		Source:         strings.TrimSpace(command.Source),
		RejectedAt:     time.Now().UTC().Format(time.RFC3339Nano),
		Error:          diagnostic,
	})
}

// RecordWorkspaceExecutionRejected records a typed admission failure without
// retrying or creating a successor request.
func (c *Component) RecordWorkspaceExecutionRejected(_ string, command workorchdevents.WorkspaceRunRequestedCommand, cause error) {
	c.recordWorkspaceExecutionRejected(command, cause)
}

func (c *Component) recordWorkspaceExecutionRejected(command workorchdevents.WorkspaceRunRequestedCommand, cause error) {
	if c == nil || strings.TrimSpace(string(command.WorkspaceID)) == "" {
		return
	}
	errText := ""
	if cause != nil {
		errText = cause.Error()
	}
	c.ApplyRunCommandRejected(workspaceviewcore.RunCommandRejectedUpdate{
		WorkspaceID:    strings.TrimSpace(string(command.WorkspaceID)),
		RunID:          "",
		Intent:         strings.TrimSpace(command.Intent),
		Reason:         strings.TrimSpace(command.Reason),
		RequestID:      strings.TrimSpace(command.RequestID),
		IdempotencyKey: executionIdempotencyKey(command),
		Source:         strings.TrimSpace(command.Source),
		RejectedAt:     time.Now().UTC().Format(time.RFC3339Nano),
		Error:          errText,
	})
}

func executionIdempotencyKey(command workorchdevents.WorkspaceRunRequestedCommand) string {
	workspaceID := strings.TrimSpace(string(command.WorkspaceID))
	if workspaceID == "" {
		return ""
	}
	switch strings.TrimSpace(command.Intent) {
	case "resume", "restart":
		return strings.TrimSpace(command.Intent) + "/" + workspaceID
	default:
		intent := strings.TrimSpace(command.Intent)
		if intent == "" {
			return ""
		}
		return intent + "/" + workspaceID
	}
}
