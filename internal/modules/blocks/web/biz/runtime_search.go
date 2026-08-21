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
	if err := validateWebOperationRequest(request); err != nil {
		return failedWebResult(request, provider, webcommon.ErrorInvalidRequest, err), err
	}
	query := strings.TrimSpace(request.Query)
	requestedMaxResults := int(request.MaxResults)
	if requestedMaxResults <= 0 {
		requestedMaxResults = provider.MaxResults
	}
	providerMaxResults := minPositive(provider.MaxResults, webproviderconfig.DefaultMaxResults)
	policy := policyDecision{Decision: policyAllowed}
	if requestedMaxResults > providerMaxResults {
		policy = policyDecision{Decision: policyApproval, Reasons: []string{"large_result_set"}}
		requirement := webSearchApprovalRequirement(provider, query, requestedMaxResults, policy)
		if request.GovernanceGrant == nil {
			return approvalRequiredSearchResult(request, provider, query, requestedMaxResults, policy), nil
		}
		if err := validateRuntimeApprovalGrant(request.GovernanceGrant, requirement); err != nil {
			wrappedErr := fmt.Errorf("%w: runtime Web approval grant does not match the current search policy", common.ErrConflict)
			return failedWebResult(request, provider, webcommon.ErrorPolicyDenied, wrappedErr), wrappedErr
		}
		providerMaxResults = webproviderconfig.SystemMaxResults
	} else if request.GovernanceGrant != nil {
		err := fmt.Errorf("%w: runtime Web approval grant is stale for the current search policy", common.ErrConflict)
		return failedWebResult(request, provider, webcommon.ErrorPolicyDenied, err), err
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
		err = fmt.Errorf("%w: decode bounded Web search response", common.ErrInvalidArgument)
		ret := failedWebResult(request, provider, webcommon.ErrorBadGatewayResponse, err)
		ret.Query, ret.DurationMillis = query, duration.Milliseconds()
		return ret, err
	}
	results := make([]WebSearchResult, 0, maxResults)
	truncated := len(response.Results) > maxResults
	sourceRefs := make([]string, 0, maxResults)
	for index, item := range response.Results {
		if len(results) >= maxResults {
			break
		}
		itemURL := strings.TrimSpace(item.URL)
		decision := a.classifyURL(ctx, itemURL, provider, false)
		if itemURL == "" || decision.Decision != policyAllowed {
			err := fmt.Errorf("%w: Web search response contains a disallowed result URL", common.ErrInvalidArgument)
			ret := failedWebResult(request, provider, webcommon.ErrorBadGatewayResponse, err)
			ret.Query, ret.DurationMillis = query, duration.Milliseconds()
			return ret, err
		}
		normalized, clipped := normalizeWebSearchResult(WebSearchResult{
			Title: item.Title, URL: itemURL, Snippet: item.Snippet, Source: sourceForResult(item),
			PublishedAt: item.PublishedAt, Rank: int64(index + 1),
		})
		truncated = truncated || clipped
		results = append(results, normalized)
		sourceRefs = append(sourceRefs, "workorch://web/source/"+stableHash(itemURL))
	}
	summary, summaryTruncated := buildWebSearchSummary(results)
	truncated = truncated || summaryTruncated
	persistedPayload := webSearchEvidencePayload{
		ProviderID: provider.ID, Query: query, Results: results,
		ResultCount: int64(len(results)), Truncated: truncated,
	}
	rawRef, evidenceHash, _, persistErr := persistWebEvidence(request.WorkspaceInternalDirectory, webcommon.StepKindSearch, persistedPayload)
	if persistErr != nil {
		ret := failedWebResult(request, provider, webcommon.ErrorBadGatewayResponse, persistErr)
		ret.Query, ret.DurationMillis = query, duration.Milliseconds()
		return ret, persistErr
	}
	evidenceRefs := webEvidenceRefs(rawRef, sourceRefs...)
	tags := []string{"web/search", "content/normalized"}
	if truncated {
		tags = append(tags, "content/truncated")
	}
	ret := RuntimeOperationResult{
		Status: "completed", Stage: "web_search", ProviderID: provider.ID, Query: query,
		Results: results, ResultCount: int64(len(results)), Truncated: truncated,
		Summary: summary, ContentHash: evidenceHash, DurationMillis: duration.Milliseconds(),
		TargetRefs: append([]string(nil), request.TargetRefs...),
		Evidence: runtimecontract.RuntimeOperationEvidence{
			Summary: summary, ContentHash: evidenceHash, EvidenceRefs: evidenceRefs,
			Quality: common.EvidenceQualityValid, EvidenceReason: "owner-normalized Web search evidence", EvidenceTags: tags,
		},
	}
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
