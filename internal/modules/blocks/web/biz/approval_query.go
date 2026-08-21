package web

import (
	"fmt"
	"strings"

	webcommon "github.com/muidea/workorch/internal/modules/blocks/web/pkg/common"
	webmodels "github.com/muidea/workorch/internal/modules/blocks/web/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func approvalRequiredSearchResult(request RuntimeOperationRequest, provider webmodels.ProviderConfig, query string, maxResults int, policy policyDecision) RuntimeOperationResult {
	requirement := webSearchApprovalRequirement(provider, query, maxResults, policy)
	return RuntimeOperationResult{
		Status: "blocked", Stage: "web_search", ProviderID: provider.ID, Query: query, ResultCount: int64(maxResults),
		ApprovalNeeded: true, FailureCode: webcommon.ErrorApprovalRequired, FailureMessage: requirement.Reason,
		ApprovalContext: requirement.AuthorizationContext.Clone(),
		Governance:      &requirement,
		TargetRefs:      append([]string(nil), request.TargetRefs...),
	}
}

func approvalRequiredFetchResult(request RuntimeOperationRequest, provider webmodels.ProviderConfig, rawURL string, contextURL string, policy policyDecision) RuntimeOperationResult {
	maxBytes := boundedFetchBytes(request, provider)
	requirement := webFetchApprovalRequirement(provider, contextURL, maxBytes, request.ExtractMode, policy)
	return RuntimeOperationResult{
		Status: "blocked", Stage: "web_fetch", ProviderID: provider.ID, URL: rawURL, FinalURL: contextURL,
		ApprovalNeeded: true, FailureCode: webcommon.ErrorApprovalRequired, FailureMessage: requirement.Reason,
		ApprovalContext: requirement.AuthorizationContext.Clone(),
		Governance:      &requirement,
		TargetRefs:      append([]string(nil), request.TargetRefs...),
	}
}

func webSearchApprovalRequirement(provider webmodels.ProviderConfig, query string, maxResults int, policy policyDecision) runtimecontract.RuntimeGovernanceRequirement {
	return runtimecontract.RuntimeGovernanceRequirement{
		Kind: runtimecontract.GovernanceApproval, SubjectRef: strings.TrimSpace(query),
		Reason:          approvalReasonPrefix + ": " + strings.Join(policy.Reasons, ","),
		RequestedAction: webcommon.StepKindSearch, RiskLevel: "external",
		AuthorizationContext: workorcheventpayload.New(
			workorcheventpayload.StringField("providerId", provider.ID), workorcheventpayload.StringField("query", strings.TrimSpace(query)),
			workorcheventpayload.IntField("maxResults", int64(maxResults)), workorcheventpayload.StringField("policyDecision", policy.Decision),
			workorcheventpayload.StringArrayField("policyReasons", policy.Reasons),
		),
	}
}

func webFetchApprovalRequirement(provider webmodels.ProviderConfig, targetURL string, maxBytes int64, extractMode string, policy policyDecision) runtimecontract.RuntimeGovernanceRequirement {
	return runtimecontract.RuntimeGovernanceRequirement{
		Kind: runtimecontract.GovernanceApproval, SubjectRef: strings.TrimSpace(targetURL),
		Reason:          approvalReasonPrefix + ": " + strings.Join(policy.Reasons, ","),
		RequestedAction: webcommon.StepKindFetch, RiskLevel: "external",
		AuthorizationContext: workorcheventpayload.New(
			workorcheventpayload.StringField("providerId", provider.ID), workorcheventpayload.StringField("url", strings.TrimSpace(targetURL)),
			workorcheventpayload.IntField("maxBytes", maxBytes), workorcheventpayload.StringField("extractMode", strings.TrimSpace(extractMode)),
			workorcheventpayload.StringField("policyDecision", policy.Decision), workorcheventpayload.StringArrayField("policyReasons", policy.Reasons),
		),
	}
}

func validateRuntimeApprovalGrant(grant *runtimecontract.RuntimeOperationGovernanceGrant, requirement runtimecontract.RuntimeGovernanceRequirement) error {
	requirement = requirement.Normalized()
	if err := requirement.Validate(); err != nil {
		return err
	}
	if grant == nil {
		return fmt.Errorf("runtime approval grant is required")
	}
	value := grant.Normalized()
	if value.Validate() != nil || value.ApprovalID == "" || value.TicketID != "" ||
		value.DependencyKind != string(requirement.Kind) || value.RequestedAction != requirement.RequestedAction ||
		value.SubjectRef != requirement.SubjectRef || value.RiskLevel != requirement.RiskLevel ||
		value.AuthorizationHash != runtimecontract.StableRuntimeAuthorizationContextHash(requirement.AuthorizationContext) {
		return fmt.Errorf("runtime approval grant does not match immutable Web authorization target")
	}
	return nil
}
