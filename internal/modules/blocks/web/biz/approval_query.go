package web

import (
	"context"
	"fmt"
	"strings"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	webcommon "github.com/muidea/workorch/internal/modules/blocks/web/pkg/common"
	webmodels "github.com/muidea/workorch/internal/modules/blocks/web/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func approvalRequiredSearchResult(request RuntimeOperationRequest, provider webmodels.ProviderConfig, query string, maxResults int, policy policyDecision) RuntimeOperationResult {
	reason := approvalReasonPrefix + ": " + strings.Join(policy.Reasons, ",")
	return RuntimeOperationResult{
		Status: "blocked", Stage: "web_search", ProviderID: provider.ID, Query: query, ResultCount: int64(maxResults),
		ApprovalNeeded: true, FailureCode: webcommon.ErrorApprovalRequired, FailureMessage: reason,
		ApprovalContext: workorcheventpayload.New(
			workorcheventpayload.StringField("providerId", provider.ID), workorcheventpayload.StringField("query", query),
			workorcheventpayload.IntField("maxResults", int64(maxResults)), workorcheventpayload.StringField("policyDecision", policy.Decision),
			workorcheventpayload.StringArrayField("policyReasons", policy.Reasons),
		),
		TargetRefs: append([]string(nil), request.TargetRefs...),
	}
}

func approvalRequiredFetchResult(request RuntimeOperationRequest, provider webmodels.ProviderConfig, rawURL string, contextURL string, policy policyDecision) RuntimeOperationResult {
	reason := approvalReasonPrefix + ": " + strings.Join(policy.Reasons, ",")
	maxBytes := request.MaxBytes
	if maxBytes <= 0 {
		maxBytes = provider.MaxFetchBytes
	}
	return RuntimeOperationResult{
		Status: "blocked", Stage: "web_fetch", ProviderID: provider.ID, URL: rawURL, FinalURL: contextURL,
		ApprovalNeeded: true, FailureCode: webcommon.ErrorApprovalRequired, FailureMessage: reason,
		ApprovalContext: workorcheventpayload.New(
			workorcheventpayload.StringField("providerId", provider.ID), workorcheventpayload.StringField("url", contextURL),
			workorcheventpayload.IntField("maxBytes", maxBytes), workorcheventpayload.StringField("policyDecision", policy.Decision),
			workorcheventpayload.StringArrayField("policyReasons", policy.Reasons),
		),
		TargetRefs: []string{contextURL},
	}
}

func (a *RuntimeAdapter) validateApproval(ctx context.Context, approvalID common.ApprovalID, runID common.RunID, action string, providerID string, contextKey string, contextValue string, policy policyDecision) error {
	if a.approvals == nil {
		return fmt.Errorf("approval repository is not configured")
	}
	item, err := a.getApproval(ctx, approvalID, runID)
	if err != nil {
		return err
	}
	if item.RunID != runID {
		return fmt.Errorf("approval run mismatch")
	}
	if item.State != common.ApprovalStateApproved {
		return fmt.Errorf("approval is not approved")
	}
	if strings.TrimSpace(item.Action) != action {
		return fmt.Errorf("approval action mismatch")
	}
	if !approvalContextMatches(item, providerID, contextKey, contextValue, policy) {
		return fmt.Errorf("approval context mismatch")
	}
	return nil
}

func (a *RuntimeAdapter) getApproval(ctx context.Context, approvalID common.ApprovalID, runID common.RunID) (*approvalmodel.Approval, error) {
	if a == nil || a.approvals == nil {
		return nil, fmt.Errorf("approval repository is not configured")
	}
	if scoped, ok := a.approvals.(ScopedApprovalGetter); ok {
		return scoped.GetForRun(ctx, approvalID, runID)
	}
	return a.approvals.Get(ctx, approvalID)
}

func approvalContextMatches(item *approvalmodel.Approval, providerID string, contextKey string, contextValue string, policy policyDecision) bool {
	if item == nil {
		return false
	}
	ctx := item.Context
	providerValue, _ := ctx.String("providerId")
	if strings.TrimSpace(providerValue) != providerID {
		return false
	}
	contextFieldValue, _ := ctx.String(contextKey)
	if strings.TrimSpace(contextFieldValue) != strings.TrimSpace(contextValue) {
		return false
	}
	gotReasons, _ := ctx.StringSlice("policyReasons")
	for _, reason := range policy.Reasons {
		if !containsString(gotReasons, reason) {
			return false
		}
	}
	return true
}
