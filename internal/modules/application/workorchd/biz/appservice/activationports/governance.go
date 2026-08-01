package activationports

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func WorkspaceGovernanceHistory(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, goalID common.GoalID, dodID common.DoDID, limit int) []runtimecontract.RuntimeGovernanceHistoryEntry {
	if hub == nil || workspaceID == "" || limit <= 0 {
		return nil
	}
	readCtx := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	workspaceTickets, err := ownerreadports.ListDecisionTicketsByWorkspaceOwnerCommand(readCtx, hub, workspaceID)
	if err != nil {
		return nil
	}
	ret := make([]runtimecontract.RuntimeGovernanceHistoryEntry, 0, min(limit, len(workspaceTickets)))
	for _, ticket := range workspaceTickets {
		if ticket == nil || ticket.State != common.DecisionTicketStateResolved {
			continue
		}
		run, err := ownerreadports.ReadRunByOwnerCommand(readCtx, hub, ticket.RunID)
		if err != nil || run == nil || run.WorkspaceID != workspaceID {
			continue
		}
		if goalID != "" && run.GoalID != goalID {
			continue
		}
		if dodID != "" && run.DoDID != dodID {
			continue
		}
		resolution, err := ownerreadports.ReadDecisionResolutionByTicketOwnerCommand(readCtx, hub, ticket.ID)
		if err != nil || resolution == nil {
			continue
		}
		ret = append(ret, runtimeGovernanceHistoryEntry(ticket, resolution))
		if len(ret) >= limit {
			break
		}
	}
	return runtimecontract.RuntimeContinuationContext{GovernanceHistory: ret}.Normalized().GovernanceHistory
}

func runtimeGovernanceHistoryEntry(ticket *decisionmodel.Ticket, resolution *decisionmodel.Resolution) runtimecontract.RuntimeGovernanceHistoryEntry {
	recoveryAction := firstRuntimeContinuationString(
		resolution.ContinuationAction,
		continuationPayloadString(resolution.Payload, "recoveryAction"),
	)
	return runtimecontract.RuntimeGovernanceHistoryEntry{
		TicketID: ticket.ID, RunID: ticket.RunID, StepID: ticket.StepID,
		DecisionKind: ticket.DecisionKind, Reason: ticket.Reason, Question: ticket.Question,
		ResolutionID: resolution.ID, ResolutionType: resolution.Type, RecoveryAction: recoveryAction,
		ResolutionSummary: firstRuntimeContinuationString(
			continuationPayloadString(resolution.Payload, "summary"),
			continuationPayloadString(resolution.Payload, "answer"),
			continuationPayloadString(resolution.Payload, "note"),
			continuationPayloadString(resolution.Payload, "resumeInstruction"),
			continuationPayloadString(resolution.Payload, "reason"),
		),
		Failure: runtimeContinuationFailure(ticket),
	}.Normalized()
}

func runtimeContinuationFailure(ticket *decisionmodel.Ticket) *runtimecontract.RuntimeContinuationFailure {
	if ticket == nil {
		return nil
	}
	contextData := ticket.Context
	value := runtimecontract.RuntimeContinuationFailure{
		Class:      firstRuntimeContinuationString(ticket.FailureClass, continuationPayloadString(contextData, "failureClass")),
		Signature:  firstRuntimeContinuationString(ticket.FailureSignature, continuationPayloadString(contextData, "failureSignature")),
		TargetRefs: runtimeContinuationTargetRefs(contextData),
		FailedStepID: common.StepID(firstRuntimeContinuationString(
			continuationPayloadString(contextData, "failedStepId"), string(ticket.StepID),
		)),
		FailedStepKind: continuationPayloadString(contextData, "failedStepKind"),
		Summary: firstRuntimeContinuationString(
			continuationPayloadString(contextData, "failureReason"),
			continuationPayloadString(contextData, "repairObjective"),
			ticket.Reason,
		),
		RecoveryPolicy: continuationPayloadString(contextData, "recoveryPolicy"),
	}.Normalized()
	if value.Class == "" && value.Signature == "" && len(value.TargetRefs) == 0 && value.FailedStepID == "" && value.FailedStepKind == "" && value.Summary == "" && value.RecoveryPolicy == "" {
		return nil
	}
	return &value
}

func runtimeContinuationTargetRefs(input workorcheventpayload.Payload) []string {
	refs := make([]string, 0)
	for _, key := range []string{"failureTargetScope", "targetRef", "targetRefs", "repairTargetSet"} {
		if value, ok := input.Field(key); ok {
			refs = appendRuntimeContinuationPayloadRefs(refs, value)
		}
	}
	return refs
}

func appendRuntimeContinuationPayloadRefs(refs []string, value workorcheventpayload.Value) []string {
	switch value.Kind {
	case workorcheventpayload.ValueString:
		if text := strings.TrimSpace(value.String); text != "" {
			refs = append(refs, text)
		}
	case workorcheventpayload.ValueArray:
		for _, item := range value.Array {
			refs = appendRuntimeContinuationPayloadRefs(refs, item)
		}
	case workorcheventpayload.ValueObject:
		object := workorcheventpayload.Payload{Fields: value.Object}
		for _, key := range []string{"targetRef", "targetRefs", "path", "uri", "resourceRef", "subjectRef", "workspaceRelativePath"} {
			if nested, ok := object.Field(key); ok {
				refs = appendRuntimeContinuationPayloadRefs(refs, nested)
			}
		}
	}
	return refs
}

func appendRuntimeContinuationRefs(refs []string, value any) []string {
	switch typed := value.(type) {
	case string:
		if text := strings.TrimSpace(typed); text != "" {
			refs = append(refs, text)
		}
	case []string:
		for _, item := range typed {
			refs = appendRuntimeContinuationRefs(refs, item)
		}
	case []any:
		for _, item := range typed {
			refs = appendRuntimeContinuationRefs(refs, item)
		}
	case map[string]any:
		for _, key := range []string{"targetRef", "targetRefs", "path", "uri", "resourceRef", "subjectRef", "workspaceRelativePath"} {
			refs = appendRuntimeContinuationRefs(refs, typed[key])
		}
	}
	return refs
}

func continuationPayloadString(input workorcheventpayload.Payload, key string) string {
	value, _ := input.String(key)
	return compactRuntimeContinuationText(value)
}

func firstRuntimeContinuationString(values ...string) string {
	for _, value := range values {
		if value = compactRuntimeContinuationText(value); value != "" {
			return value
		}
	}
	return ""
}

func compactRuntimeContinuationText(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > 2048 {
		value = string(runes[:2048])
	}
	return value
}
