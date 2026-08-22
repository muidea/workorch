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
			WorkspaceID: workspaceID,
			RunID:       strings.TrimSpace(string(outcome.RunID)),
			Intent:      strings.TrimSpace(command.Intent),
			Reason:      strings.TrimSpace(command.Reason),
			Disposition: disposition,
			Diagnostic:  diagnostic,
			RequestID:   strings.TrimSpace(command.RequestID),
			Source:      strings.TrimSpace(command.Source),
			AcceptedAt:  time.Now().UTC().Format(time.RFC3339Nano),
		})
		return
	}
	c.ApplyRunCommandRejected(workspaceviewcore.RunCommandRejectedUpdate{
		WorkspaceID: workspaceID,
		RunID:       strings.TrimSpace(string(outcome.RunID)),
		Intent:      strings.TrimSpace(command.Intent),
		Reason:      strings.TrimSpace(command.Reason),
		Disposition: disposition,
		Diagnostic:  diagnostic,
		RequestID:   strings.TrimSpace(command.RequestID),
		Source:      strings.TrimSpace(command.Source),
		RejectedAt:  time.Now().UTC().Format(time.RFC3339Nano),
		Error:       diagnostic,
	})
}
