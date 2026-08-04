package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func taskRetryPlanFromStep(snapshot readmodelcore.RunSnapshot, step readmodelcore.StepView, occurrences map[string]string, knownAnchors map[string]bool) (projectioncontract.TaskPlan, bool) {
	meta := taskLifecycleMetadataFromStep(step)
	if meta.retryOf == "" {
		return projectioncontract.TaskPlan{}, false
	}
	anchor := strings.TrimSpace(meta.retryOf)
	if anchor == "" {
		anchor = strings.TrimSpace(readModelStringValue(step.Input["graphRootStepId"]))
	}
	if anchor == "" || !knownAnchors[anchor] {
		return projectioncontract.TaskPlan{}, false
	}
	if ticketID := taskBlockingTicketByStep(snapshot)[step.ID]; ticketID != "" {
		meta.blockingTicketID = ticketID
	}
	state := "retrying"
	if meta.blockingTicketID != "" || snapshot.Run.BlockingTicketID != "" && step.State == string(common.StepStateFailed) {
		state = "blocked_on_decision"
	} else if step.State == string(common.StepStateFailed) {
		state = "retry_exhausted"
	} else if step.State == string(common.StepStateSucceeded) {
		state = "retry_completed"
	}
	plan := projectioncontract.TaskPlan{
		WorkspaceID:       snapshot.Run.WorkspaceID,
		RunID:             snapshot.Run.ID,
		AnchorStepID:      anchor,
		RevisionStepID:    step.ID,
		State:             state,
		Stage:             firstNonEmptyString(readModelStringValue(step.Output["stage"]), readModelStringValue(step.Input["stage"])),
		SourceOutputState: meta.sourceOutputState,
		Summary:           TimelineStepSummary(step.Output),
		RetryOf:           anchor,
		RetryAttempt:      meta.retryAttempt,
		FailureClass:      meta.failureClass,
		FailureSignature:  meta.failureSignature,
		TargetScope:       meta.targetScope,
		BlockingTicketID:  meta.blockingTicketID,
		EvidenceRefs:      compactUniqueStrings(append([]string{step.ID}, meta.evidenceRefs...)),
		OccurredAt:        strings.TrimSpace(occurrences[step.ID]),
		SourceProjection:  "retry",
	}
	if plan.BlockingTicketID == "" && state == "blocked_on_decision" {
		plan.BlockingTicketID = snapshot.Run.BlockingTicketID
	}
	return plan, true
}
