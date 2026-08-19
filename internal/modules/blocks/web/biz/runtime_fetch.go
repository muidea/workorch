package web

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	webcommon "github.com/muidea/workorch/internal/modules/blocks/web/pkg/common"
	webmodels "github.com/muidea/workorch/internal/modules/blocks/web/pkg/models"
	webproviderconfig "github.com/muidea/workorch/internal/modules/blocks/web/pkg/providerconfig"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type fetchGatewayResponse struct {
	URL         string `json:"url"`
	FinalURL    string `json:"finalUrl"`
	Title       string `json:"title"`
	Content     string `json:"content"`
	Summary     string `json:"summary"`
	ContentType string `json:"contentType"`
	RetrievedAt string `json:"retrievedAt"`
	PublishedAt string `json:"publishedAt"`
	Source      string `json:"source"`
	Truncated   bool   `json:"truncated"`
}

func (a *RuntimeAdapter) fetch(ctx context.Context, request RuntimeOperationRequest, provider webmodels.ProviderConfig) (RuntimeOperationResult, error) {
	rawURL := strings.TrimSpace(request.URL)
	if rawURL == "" {
		err := fmt.Errorf("%w: web/fetch url is required", common.ErrInvalidArgument)
		return failedWebResult(request, provider, webcommon.ErrorInvalidRequest, err), err
	}
	if provider.FetchEndpoint == "" {
		err := fmt.Errorf("%w: web search provider fetchEndpoint is not configured", common.ErrInvalidArgument)
		return failedWebResult(request, provider, webcommon.ErrorProviderUnavailable, err), err
	}
	policy := a.classifyURL(ctx, rawURL, provider, true)
	approvalID := request.ApprovalID
	if policy.Decision == policyDenied {
		err := fmt.Errorf("%w: web/fetch policy denied: %s", common.ErrInvalidArgument, strings.Join(policy.Reasons, ","))
		return failedWebResult(request, provider, webcommon.ErrorPolicyDenied, err), err
	}
	if policy.Decision == policyApproval {
		required := approvalRequiredFetchResult(request, provider, rawURL, rawURL, policy)
		var approvalErr error
		if request.GovernanceGrant != nil {
			approvalID, approvalErr = validateRuntimeApprovalGrant(request.GovernanceGrant, webcommon.StepKindFetch, rawURL, required.ApprovalContext)
		} else if approvalID == "" {
			return required, nil
		} else {
			approvalErr = a.validateApproval(ctx, approvalID, request.RunID, webcommon.StepKindFetch, provider.ID, approvalContextURL, rawURL, policy)
		}
		if approvalErr != nil {
			wrappedErr := fmt.Errorf("%w: %v", common.ErrInvalidArgument, approvalErr)
			return failedWebResult(request, provider, webcommon.ErrorPolicyDenied, wrappedErr), wrappedErr
		}
	}
	maxBytes := request.MaxBytes
	if maxBytes <= 0 {
		maxBytes = provider.MaxFetchBytes
	}
	maxBytes = boundedInt64(maxBytes, webproviderconfig.DefaultMaxFetchBytes, minPositive64(provider.MaxFetchBytes, webproviderconfig.SystemMaxFetchBytes))
	payload := map[string]any{
		"workspaceId": string(request.WorkspaceID),
		"runId":       string(request.RunID),
		"stepId":      string(request.StepID),
		"url":         rawURL,
		"maxBytes":    maxBytes,
		"extractMode": strings.TrimSpace(request.ExtractMode),
	}
	raw, duration, errCode, err := a.doProviderRequest(ctx, provider, provider.FetchEndpoint, payload, maxBytes)
	if err != nil {
		ret := failedWebResult(request, provider, errCode, err)
		ret.URL, ret.DurationMillis = rawURL, duration.Milliseconds()
		return ret, err
	}
	var response fetchGatewayResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		err = fmt.Errorf("%w: decode web/fetch response: %v", common.ErrInvalidArgument, err)
		return failedWebResult(request, provider, webcommon.ErrorBadGatewayResponse, err), err
	}
	finalURL := strings.TrimSpace(response.FinalURL)
	if finalURL == "" {
		finalURL = strings.TrimSpace(response.URL)
	}
	if finalURL == "" {
		finalURL = rawURL
	}
	finalPolicy := a.classifyURL(ctx, finalURL, provider, true)
	if finalPolicy.Decision == policyDenied {
		err := fmt.Errorf("%w: web/fetch final url policy denied: %s", common.ErrInvalidArgument, strings.Join(finalPolicy.Reasons, ","))
		return failedWebResult(request, provider, webcommon.ErrorPolicyDenied, err), err
	}
	if finalPolicy.Decision == policyApproval {
		required := approvalRequiredFetchResult(request, provider, rawURL, finalURL, finalPolicy)
		var approvalErr error
		if request.GovernanceGrant != nil {
			approvalID, approvalErr = validateRuntimeApprovalGrant(request.GovernanceGrant, webcommon.StepKindFetch, finalURL, required.ApprovalContext)
		} else if approvalID == "" {
			return required, nil
		} else {
			approvalErr = a.validateApproval(ctx, approvalID, request.RunID, webcommon.StepKindFetch, provider.ID, approvalContextURL, finalURL, finalPolicy)
		}
		if approvalErr != nil {
			err = fmt.Errorf("%w: %v", common.ErrInvalidArgument, approvalErr)
			return failedWebResult(request, provider, webcommon.ErrorPolicyDenied, err), err
		}
	}
	content, truncated := boundWebContent(response.Content, maxBytes, response.Truncated)
	retrievedAt := strings.TrimSpace(response.RetrievedAt)
	if retrievedAt == "" {
		retrievedAt = time.Now().UTC().Format(time.RFC3339)
	}
	ret := RuntimeOperationResult{
		Status: "completed", Stage: "web_fetch", ProviderID: provider.ID, URL: rawURL, FinalURL: finalURL,
		Title: response.Title, Content: content, Summary: response.Summary, ContentType: response.ContentType,
		ContentHash: stableHash(finalURL, response.Title, content, response.Summary, retrievedAt), RetrievedAt: retrievedAt,
		PublishedAt: response.PublishedAt, Source: response.Source, Size: int64(len([]byte(content))), Truncated: truncated,
		ApprovalID: approvalID, DurationMillis: duration.Milliseconds(), TargetRefs: []string{finalURL},
	}
	if ret.Summary == "" {
		ret.Summary = firstWebNonEmpty(ret.Title, ret.FinalURL, "completed web fetch")
	}
	ret.Evidence = runtimecontract.RuntimeOperationEvidence{Summary: ret.Summary, ContentHash: ret.ContentHash}
	return ret, nil
}
