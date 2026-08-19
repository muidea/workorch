package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	webcommon "github.com/muidea/workorch/internal/modules/blocks/web/pkg/common"
	webmodels "github.com/muidea/workorch/internal/modules/blocks/web/pkg/models"
	webproviderconfig "github.com/muidea/workorch/internal/modules/blocks/web/pkg/providerconfig"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type searchGatewayResponse struct {
	Results []searchResult `json:"results"`
}

type searchResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Snippet     string `json:"snippet"`
	Source      string `json:"source"`
	PublishedAt string `json:"publishedAt"`
}

func (a *RuntimeAdapter) search(ctx context.Context, request RuntimeOperationRequest, provider webmodels.ProviderConfig) (RuntimeOperationResult, error) {
	query := strings.TrimSpace(request.Query)
	if query == "" {
		err := fmt.Errorf("%w: web/search query is required", common.ErrInvalidArgument)
		return failedWebResult(request, provider, webcommon.ErrorInvalidRequest, err), err
	}
	policy := policyDecision{Decision: policyAllowed}
	requestedMaxResults := int(request.MaxResults)
	if requestedMaxResults <= 0 {
		requestedMaxResults = provider.MaxResults
	}
	providerMaxResults := minPositive(provider.MaxResults, webproviderconfig.SystemMaxResults)
	approvalID := request.ApprovalID
	if requestedMaxResults > providerMaxResults {
		policy = policyDecision{Decision: policyApproval, Reasons: []string{"large_result_set"}}
		required := approvalRequiredSearchResult(request, provider, query, requestedMaxResults, policy)
		var approvalErr error
		if request.GovernanceGrant != nil {
			approvalID, approvalErr = validateRuntimeApprovalGrant(request.GovernanceGrant, webcommon.StepKindSearch, query, required.ApprovalContext)
		} else if approvalID == "" {
			return required, nil
		} else {
			approvalErr = a.validateApproval(ctx, approvalID, request.RunID, webcommon.StepKindSearch, provider.ID, approvalContextQuery, query, policy)
		}
		if approvalErr != nil {
			wrappedErr := fmt.Errorf("%w: %v", common.ErrInvalidArgument, approvalErr)
			return failedWebResult(request, provider, webcommon.ErrorPolicyDenied, wrappedErr), wrappedErr
		}
		providerMaxResults = webproviderconfig.SystemMaxResults
	}
	maxResults := boundedInt(requestedMaxResults, webproviderconfig.DefaultMaxResults, providerMaxResults)
	payload := map[string]any{
		"workspaceId":    string(request.WorkspaceID),
		"runId":          string(request.RunID),
		"stepId":         string(request.StepID),
		"query":          query,
		"locale":         strings.TrimSpace(request.Locale),
		"recency":        strings.TrimSpace(request.Recency),
		"domains":        append([]string(nil), request.Domains...),
		"excludeDomains": append([]string(nil), request.ExcludeDomains...),
		"maxResults":     maxResults,
		"safeSearch":     request.SafeSearch,
	}
	raw, duration, errCode, err := a.doProviderRequest(ctx, provider, provider.SearchEndpoint, payload, provider.MaxFetchBytes)
	if err != nil {
		ret := failedWebResult(request, provider, errCode, err)
		ret.Query, ret.DurationMillis = query, duration.Milliseconds()
		return ret, err
	}
	var response searchGatewayResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		err = fmt.Errorf("%w: decode web/search response: %v", common.ErrInvalidArgument, err)
		ret := failedWebResult(request, provider, webcommon.ErrorBadGatewayResponse, err)
		ret.Query, ret.DurationMillis = query, duration.Milliseconds()
		return ret, err
	}
	results := make([]WebSearchResult, 0, len(response.Results))
	for idx, item := range response.Results {
		itemURL := strings.TrimSpace(item.URL)
		if itemURL == "" {
			err := fmt.Errorf("%w: web/search result url is required", common.ErrInvalidArgument)
			return failedWebResult(request, provider, webcommon.ErrorBadGatewayResponse, err), err
		}
		decision := a.classifyURL(ctx, itemURL, provider, false)
		if decision.Decision != policyAllowed {
			err := fmt.Errorf("%w: web/search result url is not allowed: %s", common.ErrInvalidArgument, strings.Join(decision.Reasons, ","))
			return failedWebResult(request, provider, webcommon.ErrorBadGatewayResponse, err), err
		}
		results = append(results, WebSearchResult{
			Title: item.Title, URL: itemURL, Snippet: item.Snippet, Source: sourceForResult(item),
			PublishedAt: item.PublishedAt, Rank: int64(idx + 1),
			ContentHash: stableHash(item.Title, itemURL, item.Snippet, item.Source, item.PublishedAt),
		})
		if len(results) >= maxResults {
			break
		}
	}
	ret := RuntimeOperationResult{
		Status: "completed", Stage: "web_search", ProviderID: provider.ID, Query: query,
		Results: results, ResultCount: int64(len(results)), Truncated: len(response.Results) > len(results),
		ApprovalID: approvalID, DurationMillis: duration.Milliseconds(), TargetRefs: append([]string(nil), request.TargetRefs...),
	}
	ret.Summary = "completed web search"
	ret.Evidence = runtimecontract.RuntimeOperationEvidence{Summary: ret.Summary}
	return ret, nil
}

func sourceForResult(item searchResult) string {
	if strings.TrimSpace(item.Source) != "" {
		return strings.TrimSpace(item.Source)
	}
	parsed, err := url.Parse(item.URL)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}
