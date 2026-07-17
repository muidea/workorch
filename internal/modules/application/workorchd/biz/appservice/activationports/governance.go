package activationports

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
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
		ret = append(ret, runtimeV2GovernanceHistoryEntry(ticket, resolution))
		if len(ret) >= limit {
			break
		}
	}
	return runtimecontract.RuntimeContinuationContext{GovernanceHistory: ret}.Normalized().GovernanceHistory
}

func RejectedResultContinuation(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, goalID common.GoalID, dodID common.DoDID) (*runtimecontract.RuntimeRejectedResultContinuation, error) {
	if hub == nil || workspaceID == "" || goalID == "" || dodID == "" {
		return nil, nil
	}
	items, err := ownerreadports.ListResultsByWorkspaceOwnerCommand(ctx, hub, workspaceID)
	if err != nil {
		return nil, err
	}
	latest := activationcore.LatestTerminalResultForDefinitions(items, goalID, dodID)
	if latest == nil || latest.State != common.ResultStateRejected {
		return nil, nil
	}
	value := runtimeV2RejectedResultContinuation(latest)
	return &value, nil
}

func runtimeV2GovernanceHistoryEntry(ticket *decisionmodel.Ticket, resolution *decisionmodel.Resolution) runtimecontract.RuntimeGovernanceHistoryEntry {
	recoveryAction := firstRuntimeV2ContinuationString(
		resolution.ContinuationAction,
		continuationPayloadString(resolution.Payload, "recoveryAction"),
	)
	return runtimecontract.RuntimeGovernanceHistoryEntry{
		TicketID: ticket.ID, RunID: ticket.RunID, StepID: ticket.StepID,
		DecisionKind: ticket.DecisionKind, Reason: ticket.Reason, Question: ticket.Question,
		ResolutionID: resolution.ID, ResolutionType: resolution.Type, RecoveryAction: recoveryAction,
		ResolutionSummary: firstRuntimeV2ContinuationString(
			continuationPayloadString(resolution.Payload, "summary"),
			continuationPayloadString(resolution.Payload, "answer"),
			continuationPayloadString(resolution.Payload, "note"),
			continuationPayloadString(resolution.Payload, "resumeInstruction"),
			continuationPayloadString(resolution.Payload, "reason"),
		),
		Failure: runtimeV2ContinuationFailure(ticket),
	}.Normalized()
}

func runtimeV2ContinuationFailure(ticket *decisionmodel.Ticket) *runtimecontract.RuntimeContinuationFailure {
	if ticket == nil {
		return nil
	}
	contextData := ticket.Context
	value := runtimecontract.RuntimeContinuationFailure{
		Class:      firstRuntimeV2ContinuationString(ticket.FailureClass, continuationPayloadString(contextData, "failureClass")),
		Signature:  firstRuntimeV2ContinuationString(ticket.FailureSignature, continuationPayloadString(contextData, "failureSignature")),
		TargetRefs: runtimeV2ContinuationTargetRefs(contextData),
		FailedStepID: common.StepID(firstRuntimeV2ContinuationString(
			continuationPayloadString(contextData, "failedStepId"), string(ticket.StepID),
		)),
		FailedStepKind: continuationPayloadString(contextData, "failedStepKind"),
		Summary: firstRuntimeV2ContinuationString(
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

func runtimeV2RejectedResultContinuation(item *resultmodel.Result) runtimecontract.RuntimeRejectedResultContinuation {
	ret := runtimecontract.RuntimeRejectedResultContinuation{
		PredecessorRunID: item.RunID, ResultID: item.ID, ResultState: item.State,
		SourceType: item.SourceType, SourceRef: item.SourceRef, Summary: item.Summary, GoalOutcome: item.SummaryDetail.GoalOutcome,
		RequiredEvidence:  append(append(append([]string(nil), item.SummaryDetail.NextActions...), item.SummaryDetail.ResidualRisks...), item.SummaryDetail.KnownLimitations...),
		EvidenceRefs:      append(append([]string(nil), item.EvidenceRefs...), item.SummaryDetail.EvidenceRefs...),
		ArtifactRefs:      append(append([]common.ArtifactID(nil), item.ArtifactRefs...), item.SummaryDetail.ArtifactRefs...),
		GateFailureReason: continuationPayloadString(item.SummaryDetail.Diagnostics, "gateFailureReason"),
		RejectionReason:   continuationPayloadString(item.SummaryDetail.Diagnostics, "rejectionReason"),
		SourceSummary:     continuationPayloadString(item.SummaryDetail.Diagnostics, "sourceSummary"),
	}
	// Prefer Orchestrator-authored RuntimeCloseout when present.
	if item.RuntimeCloseout != nil {
		closeout := item.RuntimeCloseout.Normalized()
		ret.DefinitionRevision = closeout.DefinitionRevision
		ret.Disposition = closeout.Disposition
		if closeout.FailureCode != "" || closeout.FailureSignature != "" || closeout.FailureClass != "" {
			ret.Failure = &runtimecontract.RuntimeContinuationFailure{
				Class: string(closeout.FailureClass), Signature: closeout.FailureSignature, Summary: item.Summary,
			}
		}
		for _, id := range closeout.SatisfiedCoverageWorkItemIDs {
			ret.SatisfiedDoDItems = append(ret.SatisfiedDoDItems, runtimecontract.RuntimeContinuationItem{
				CoverageWorkItemID: id, State: "satisfied",
			})
		}
		for _, id := range closeout.UnsatisfiedCoverageWorkItemIDs {
			ret.UnsatisfiedDoDItems = append(ret.UnsatisfiedDoDItems, runtimecontract.RuntimeContinuationItem{
				CoverageWorkItemID: id, State: "unsatisfied",
			})
		}
		for _, gap := range closeout.Gaps {
			ret.CoverageGaps = append(ret.CoverageGaps, runtimecontract.RuntimeContinuationCoverageGap{
				Scope: "coverage", CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), gap.CoverageWorkItemIDs...),
				MissingEvidenceKinds: append([]string(nil), gap.MissingEvidenceKinds...),
				TargetRefs:           append([]string(nil), gap.TargetRefs...),
				FailureClass:         gap.FailureClass, FailureCode: gap.FailureCode, FailureSignature: gap.FailureSignature,
				State: "unsatisfied", EvidenceRefs: append([]string(nil), gap.EvidenceRefs...), Requirement: gap.Reason,
			})
		}
		ret.RequiredEvidence = append(ret.RequiredEvidence, closeout.MissingEvidenceKinds...)
		ret.EvidenceRefs = append(ret.EvidenceRefs, closeout.EvidenceRefs...)
		ret.ArtifactRefs = append(ret.ArtifactRefs, closeout.ArtifactRefs...)
		return ret.Normalized()
	}
	for _, value := range item.DoDAssessment.Items {
		continuationItem := runtimecontract.RuntimeContinuationItem{
			Item: value.Item, State: value.State, Reason: value.FailureReason, EvidenceRefs: value.EvidenceRefs,
		}
		if strings.EqualFold(strings.TrimSpace(value.State), "satisfied") {
			ret.SatisfiedDoDItems = append(ret.SatisfiedDoDItems, continuationItem)
		} else {
			ret.UnsatisfiedDoDItems = append(ret.UnsatisfiedDoDItems, continuationItem)
		}
	}
	ret.CoverageGaps = append(ret.CoverageGaps, runtimeV2RejectedCoverageGaps(item.DoDAssessment.GoalCoverage, "goal")...)
	ret.CoverageGaps = append(ret.CoverageGaps, runtimeV2RejectedCoverageGaps(item.DoDAssessment.AcceptanceDimensions, "acceptance")...)
	ret.CoverageGaps = append(ret.CoverageGaps, runtimeV2RejectedCoverageGaps(item.DoDAssessment.DeliverableCoverage, "deliverable")...)
	return ret.Normalized()
}

func runtimeV2RejectedCoverageGaps(coverage *resultmodel.DoDCoverage, scope string) []runtimecontract.RuntimeContinuationCoverageGap {
	if coverage == nil {
		return nil
	}
	ret := make([]runtimecontract.RuntimeContinuationCoverageGap, 0, len(coverage.Items))
	for _, item := range coverage.Items {
		satisfied := strings.EqualFold(strings.TrimSpace(item.State), "satisfied")
		if satisfied && (len(item.EvidenceRefs) > 0 || (scope == "deliverable" && len(item.ArtifactRefs) > 0)) {
			continue
		}
		ret = append(ret, runtimecontract.RuntimeContinuationCoverageGap{
			Scope: scope, ID: item.ID, Deliverable: item.Deliverable, Name: item.Name, Path: item.Path,
			Goal: item.Goal, GoalItem: item.GoalItem, Item: item.Item, Requirement: item.Requirement, State: item.State,
			EvidenceRefs: item.EvidenceRefs, ArtifactRefs: item.ArtifactRefs,
		})
	}
	return ret
}

func runtimeV2ContinuationTargetRefs(input workorcheventpayload.Payload) []string {
	refs := make([]string, 0)
	for _, key := range []string{"failureTargetScope", "targetRef", "targetRefs", "repairTargetSet"} {
		if value, ok := input.Field(key); ok {
			refs = appendRuntimeV2ContinuationPayloadRefs(refs, value)
		}
	}
	return refs
}

func appendRuntimeV2ContinuationPayloadRefs(refs []string, value workorcheventpayload.Value) []string {
	switch value.Kind {
	case workorcheventpayload.ValueString:
		if text := strings.TrimSpace(value.String); text != "" {
			refs = append(refs, text)
		}
	case workorcheventpayload.ValueArray:
		for _, item := range value.Array {
			refs = appendRuntimeV2ContinuationPayloadRefs(refs, item)
		}
	case workorcheventpayload.ValueObject:
		object := workorcheventpayload.Payload{Fields: value.Object}
		for _, key := range []string{"targetRef", "targetRefs", "path", "uri", "resourceRef", "subjectRef", "workspaceRelativePath"} {
			if nested, ok := object.Field(key); ok {
				refs = appendRuntimeV2ContinuationPayloadRefs(refs, nested)
			}
		}
	}
	return refs
}

func appendRuntimeV2ContinuationRefs(refs []string, value any) []string {
	switch typed := value.(type) {
	case string:
		if text := strings.TrimSpace(typed); text != "" {
			refs = append(refs, text)
		}
	case []string:
		for _, item := range typed {
			refs = appendRuntimeV2ContinuationRefs(refs, item)
		}
	case []any:
		for _, item := range typed {
			refs = appendRuntimeV2ContinuationRefs(refs, item)
		}
	case map[string]any:
		for _, key := range []string{"targetRef", "targetRefs", "path", "uri", "resourceRef", "subjectRef", "workspaceRelativePath"} {
			refs = appendRuntimeV2ContinuationRefs(refs, typed[key])
		}
	}
	return refs
}

func continuationPayloadString(input workorcheventpayload.Payload, key string) string {
	value, _ := input.String(key)
	return compactRuntimeV2ContinuationText(value)
}

func firstRuntimeV2ContinuationString(values ...string) string {
	for _, value := range values {
		if value = compactRuntimeV2ContinuationText(value); value != "" {
			return value
		}
	}
	return ""
}

func compactRuntimeV2ContinuationText(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > 2048 {
		value = string(runes[:2048])
	}
	return value
}
