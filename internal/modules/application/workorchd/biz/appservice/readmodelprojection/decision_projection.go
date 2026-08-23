package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governancecore"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func mapDecision(item *decisionmodel.Ticket, resolution *decisionmodel.Resolution, autoGovernance *projectioncontract.AutoGovernanceSummary) projectioncontract.Decision {
	if item == nil {
		return projectioncontract.Decision{}
	}
	ret := projectioncontract.Decision{
		ID:                      string(item.ID),
		WorkspaceID:             string(item.WorkspaceID),
		RunID:                   string(item.RunID),
		StepID:                  string(item.StepID),
		Reason:                  item.Reason,
		DecisionKind:            item.DecisionKind,
		Question:                item.Question,
		Context:                 governancecore.CloneDecisionPayload(item.Context),
		FailureClass:            firstNonEmptyReadModelString(item.FailureClass, governancecore.StringFromDecisionPayload(item.Context, "failureClass")),
		FailureSignature:        firstNonEmptyReadModelString(item.FailureSignature, governancecore.StringFromDecisionPayload(item.Context, "failureSignature")),
		AllowedRecoveryActions:  firstNonEmptyReadModelStringSlice(item.AllowedRecoveryActions, governancecore.RuntimeRepairAllowedRecoveryActions(item.Context)),
		DefinitionClarification: mapDefinitionClarification(item.DefinitionClarification),
		State:                   string(item.State),
		AutoGovernance:          autoGovernance,
		ResolvedAt:              item.ResolvedAt,
		ResolvedBy:              item.ResolvedBy,
		CreatedAt:               item.CreatedAt,
		UpdatedAt:               item.UpdatedAt,
	}
	if resolution != nil {
		ret.ResolutionID = string(resolution.ID)
		ret.ResolutionType = resolution.Type
		ret.ResolutionPayload = governancecore.CloneDecisionPayload(resolution.Payload)
		ret.ContinuationAction = resolution.ContinuationAction
		ret.ResumeRunID = string(resolution.ResumeRunID)
		ret.SuccessorRunID = string(resolution.SuccessorRunID)
	}
	return ret
}

func mapDefinitionClarification(item *decisionmodel.DefinitionClarificationBinding) *projectioncontract.DefinitionClarification {
	if item == nil {
		return nil
	}
	bindingState := "open"
	if item.ResolutionID != "" {
		switch item.Class {
		case decisionmodel.DefinitionClarificationWorkspacePreSeal:
			bindingState = "admission_bound"
		case decisionmodel.DefinitionClarificationRuntime:
			bindingState = "replacement_intent_bound"
		default:
			bindingState = "resolved"
		}
	}
	return &projectioncontract.DefinitionClarification{
		Class:                       string(item.Class),
		BindingState:                bindingState,
		OpenBindingHash:             item.OpenBindingHash,
		RequiredAction:              item.RequiredActionKind,
		CandidateID:                 item.CandidateID,
		CandidateRevision:           item.CandidateRevision,
		CandidateHash:               item.CandidateHash,
		ViolationOrGapCode:          item.ViolationOrGapCode,
		CanonicalSignatureVersion:   item.CanonicalSignatureVersion,
		CanonicalSignature:          item.CanonicalSignature,
		CanonicalSignatureHash:      item.CanonicalSignatureHash,
		OffendingRefs:               append([]string(nil), item.OffendingRefs...),
		DefinitionRevision:          item.DefinitionRevision,
		StartupInputHash:            item.StartupInputHash,
		AuthoritySnapshotID:         item.AuthoritySnapshotID,
		ClarificationDispatchID:     item.ClarificationDispatchID,
		FocusRef:                    item.FocusRef,
		LogicalRequestID:            item.LogicalRequestID,
		SemanticDocumentHash:        item.SemanticDocumentHash,
		ResolutionID:                string(item.ResolutionID),
		ResolutionBindingHash:       item.ResolutionBindingHash,
		ReplacementIntentID:         item.ReplacementIntentID,
		ReplacementIntentHash:       item.ReplacementIntentHash,
		SuccessorDefinitionRevision: item.SuccessorDefinitionRevision,
		SuccessorStartupInputHash:   item.SuccessorStartupInputHash,
	}
}

func firstNonEmptyReadModelString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstNonEmptyReadModelStringSlice(values ...[]string) []string {
	for _, value := range values {
		if len(value) > 0 {
			return append([]string(nil), value...)
		}
	}
	return nil
}
