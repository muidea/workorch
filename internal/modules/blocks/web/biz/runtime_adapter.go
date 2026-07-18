package web

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	webcommon "github.com/muidea/workorch/internal/modules/blocks/web/pkg/common"
	webmodels "github.com/muidea/workorch/internal/modules/blocks/web/pkg/models"
	webproviderconfig "github.com/muidea/workorch/internal/modules/blocks/web/pkg/providerconfig"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const (
	defaultProviderID    = "default-http"
	defaultTimeout       = 20 * time.Second
	defaultMaxResults    = 8
	systemMaxResults     = 20
	defaultMaxFetchBytes = int64(256 * 1024)
	systemMaxFetchBytes  = int64(1024 * 1024)
	defaultMaxRedirects  = 3
	systemMaxRedirects   = 5
	policyAllowed        = "allowed"
	policyDenied         = "denied"
	policyApproval       = "approval_required"
	approvalReasonPrefix = "web access requires approval"
	approvalContextQuery = "query"
	approvalContextURL   = "url"
)

type ProviderConfig = webmodels.ProviderConfig
type ProviderConfigLoader func() (string, map[string]ProviderConfig, error)
type ProviderConfigRevision func() uint64

type ApprovalGetter interface {
	Get(context.Context, common.ApprovalID) (*approvalmodel.Approval, error)
}

type ScopedApprovalGetter interface {
	GetForRun(context.Context, common.ApprovalID, common.RunID) (*approvalmodel.Approval, error)
}

type providerRuntimeCache struct {
	ready             bool
	revision          uint64
	defaultProviderID string
	providers         map[string]ProviderConfig
}

type RuntimeAdapter struct {
	defaultProviderID string
	providers         map[string]ProviderConfig
	configLoader      ProviderConfigLoader
	configRevision    ProviderConfigRevision
	approvals         ApprovalGetter
	client            *http.Client
	resolver          *net.Resolver
	cacheMu           sync.RWMutex
	cache             providerRuntimeCache
}

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

type policyDecision struct {
	Decision string
	Reasons  []string
}

func NewRuntimeAdapter(defaultProviderID string, providers map[string]ProviderConfig, approvals ApprovalGetter) *RuntimeAdapter {
	defaultProviderID = strings.TrimSpace(defaultProviderID)
	if defaultProviderID == "" {
		defaultProviderID = defaultProviderIDFromProviders(providers)
	}
	return &RuntimeAdapter{
		defaultProviderID: defaultProviderID,
		providers:         normalizeProviders(providers),
		approvals:         approvals,
		client:            http.DefaultClient,
		resolver:          net.DefaultResolver,
	}
}

func NewRuntimeAdapterFromConfigLoaderWithRevision(loader ProviderConfigLoader, revision ProviderConfigRevision, approvals ApprovalGetter) *RuntimeAdapter {
	return &RuntimeAdapter{
		defaultProviderID: defaultProviderID,
		configLoader:      loader,
		configRevision:    revision,
		approvals:         approvals,
		client:            http.DefaultClient,
		resolver:          net.DefaultResolver,
	}
}

func NewRuntimeAdapterWithHTTPClient(defaultProviderID string, providers map[string]ProviderConfig, approvals ApprovalGetter, client *http.Client) *RuntimeAdapter {
	adapter := NewRuntimeAdapter(defaultProviderID, providers, approvals)
	if client != nil {
		adapter.client = client
	}
	return adapter
}

func LoadProviderConfigs(defaultProviderID string, valuesByID map[string]map[string]string) (string, map[string]ProviderConfig, error) {
	return webproviderconfig.LoadProviderConfigs(defaultProviderID, valuesByID)
}

func (a *RuntimeAdapter) ExecuteOperation(ctx context.Context, request RuntimeOperationRequest) (RuntimeOperationResult, error) {
	if a == nil {
		return RuntimeOperationResult{}, common.ErrInvalidArgument
	}
	provider, err := a.selectProviderID(request.ProviderID)
	if err != nil {
		return failedWebResult(request, provider, webcommon.ErrorProviderUnavailable, err), err
	}
	switch strings.TrimSpace(request.Kind) {
	case webcommon.StepKindSearch:
		result, executeErr := a.search(ctx, request, provider)
		return requireWebApprovalFailure(result, executeErr)
	case webcommon.StepKindFetch:
		result, executeErr := a.fetch(ctx, request, provider)
		return requireWebApprovalFailure(result, executeErr)
	default:
		err := fmt.Errorf("%w: unsupported web step kind %q", common.ErrInvalidArgument, request.Kind)
		return failedWebResult(request, provider, webcommon.ErrorInvalidRequest, err), err
	}
}

// OperationAdmissibility returns the current, bounded provider selector set and
// a stable revision for Runtime V2's read-only candidate-admission inventory.
// It only reads provider configuration; it never performs an HTTP request.
func (a *RuntimeAdapter) OperationAdmissibility() ([]string, string, error) {
	if a == nil {
		return nil, "", common.ErrInvalidArgument
	}
	defaultID, providers, err := a.providerConfigs()
	if err != nil {
		return nil, "", err
	}
	ids := make([]string, 0, len(providers))
	for id := range providers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	payload, err := json.Marshal(struct {
		DefaultProviderID string                    `json:"defaultProviderId"`
		Providers         map[string]ProviderConfig `json:"providers"`
	}{DefaultProviderID: defaultID, Providers: providers})
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(payload)
	return ids, "web-admissibility-" + hex.EncodeToString(sum[:])[:20], nil
}

// ProbeOperation checks only local/provider-policy prerequisites. It does not
// contact a search/fetch endpoint, create a receipt, or commit any evidence.
// A fetch may perform DNS classification, which is read-only and uses the same
// SSRF policy as execution so an invalid target cannot be sealed.
func (a *RuntimeAdapter) ProbeOperation(ctx context.Context, request RuntimeOperationRequest) error {
	if a == nil {
		return common.ErrInvalidArgument
	}
	provider, err := a.selectProviderID(request.ProviderID)
	if err != nil {
		return err
	}
	switch strings.TrimSpace(request.Kind) {
	case webcommon.StepKindSearch:
		if strings.TrimSpace(request.Query) == "" {
			return fmt.Errorf("%w: web/search query is required", common.ErrInvalidArgument)
		}
		return nil
	case webcommon.StepKindFetch:
		if strings.TrimSpace(request.URL) == "" {
			return fmt.Errorf("%w: web/fetch url is required", common.ErrInvalidArgument)
		}
		if decision := a.classifyURL(ctx, request.URL, provider, true); decision.Decision == policyDenied {
			return fmt.Errorf("%w: web/fetch URL is not allowed: %s", common.ErrInvalidArgument, strings.Join(decision.Reasons, ","))
		}
		return nil
	default:
		return fmt.Errorf("%w: unsupported web step kind %q", common.ErrInvalidArgument, request.Kind)
	}
}

func requireWebApprovalFailure(result RuntimeOperationResult, executeErr error) (RuntimeOperationResult, error) {
	if result.ApprovalNeeded && executeErr == nil {
		executeErr = fmt.Errorf("web operation requires approval")
		result.FailureCode = webcommon.ErrorApprovalRequired
		result.FailureMessage = executeErr.Error()
	}
	return result, executeErr
}

func (a *RuntimeAdapter) search(ctx context.Context, request RuntimeOperationRequest, provider ProviderConfig) (RuntimeOperationResult, error) {
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
	providerMaxResults := minPositive(provider.MaxResults, systemMaxResults)
	approvalID := request.ApprovalID
	if requestedMaxResults > providerMaxResults {
		policy = policyDecision{Decision: policyApproval, Reasons: []string{"large_result_set"}}
		if approvalID == "" {
			return approvalRequiredSearchResult(request, provider, query, requestedMaxResults, policy), nil
		}
		if err := a.validateApproval(ctx, approvalID, request.RunID, webcommon.StepKindSearch, provider.ID, approvalContextQuery, query, policy); err != nil {
			err = fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
			return failedWebResult(request, provider, webcommon.ErrorPolicyDenied, err), err
		}
		providerMaxResults = systemMaxResults
	}
	maxResults := boundedInt(requestedMaxResults, defaultMaxResults, providerMaxResults)
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

func (a *RuntimeAdapter) fetch(ctx context.Context, request RuntimeOperationRequest, provider ProviderConfig) (RuntimeOperationResult, error) {
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
		if approvalID == "" {
			return approvalRequiredFetchResult(request, provider, rawURL, rawURL, policy), nil
		}
		if err := a.validateApproval(ctx, approvalID, request.RunID, webcommon.StepKindFetch, provider.ID, approvalContextURL, rawURL, policy); err != nil {
			err = fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
			return failedWebResult(request, provider, webcommon.ErrorPolicyDenied, err), err
		}
	}
	maxBytes := request.MaxBytes
	if maxBytes <= 0 {
		maxBytes = provider.MaxFetchBytes
	}
	maxBytes = boundedInt64(maxBytes, defaultMaxFetchBytes, minPositive64(provider.MaxFetchBytes, systemMaxFetchBytes))
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
		if approvalID == "" {
			return approvalRequiredFetchResult(request, provider, rawURL, finalURL, finalPolicy), nil
		}
		if err := a.validateApproval(ctx, approvalID, request.RunID, webcommon.StepKindFetch, provider.ID, approvalContextURL, finalURL, finalPolicy); err != nil {
			err = fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
			return failedWebResult(request, provider, webcommon.ErrorPolicyDenied, err), err
		}
	}
	content := response.Content
	truncated := response.Truncated
	if int64(len([]byte(content))) > maxBytes {
		content = string([]byte(content)[:maxBytes])
		truncated = true
	}
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

func (a *RuntimeAdapter) doProviderRequest(ctx context.Context, provider ProviderConfig, endpoint string, payload map[string]any, maxResponseBytes int64) ([]byte, time.Duration, string, error) {
	if maxResponseBytes <= 0 || maxResponseBytes > systemMaxFetchBytes {
		maxResponseBytes = systemMaxFetchBytes
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, webcommon.ErrorInvalidRequest, err
	}
	callCtx, cancel := context.WithTimeout(ctx, providerTimeout(provider))
	defer cancel()
	req, err := http.NewRequestWithContext(callCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, 0, webcommon.ErrorInvalidRequest, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "workorch-web-search/1")
	if provider.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+provider.APIKey)
	}
	for key, value := range provider.Headers {
		if reservedHeader(key) {
			continue
		}
		if strings.TrimSpace(key) != "" && strings.TrimSpace(value) != "" {
			req.Header.Set(key, value)
		}
	}
	startedAt := time.Now()
	resp, err := a.httpClient().Do(req)
	duration := time.Since(startedAt)
	if err != nil {
		if errors.Is(callCtx.Err(), context.DeadlineExceeded) {
			return nil, duration, webcommon.ErrorTimeout, context.DeadlineExceeded
		}
		return nil, duration, webcommon.ErrorBadGatewayResponse, err
	}
	defer resp.Body.Close()
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if readErr != nil {
		return nil, duration, webcommon.ErrorBadGatewayResponse, readErr
	}
	if int64(len(raw)) > maxResponseBytes {
		return nil, duration, webcommon.ErrorContentTooLarge, fmt.Errorf("%w: web provider response exceeds max bytes", common.ErrInvalidArgument)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, duration, webcommon.ErrorBadGatewayResponse, fmt.Errorf("web provider returned status %d", resp.StatusCode)
	}
	return raw, duration, "", nil
}

func (a *RuntimeAdapter) selectProviderID(requestedID string) (ProviderConfig, error) {
	defaultID, providers, err := a.providerConfigs()
	if err != nil {
		return ProviderConfig{}, err
	}
	providerID := strings.TrimSpace(requestedID)
	if providerID == "" {
		providerID = defaultID
	}
	provider, ok := providers[providerID]
	if !ok {
		return ProviderConfig{ID: providerID}, fmt.Errorf("%w: web search provider %q is not configured", common.ErrInvalidArgument, providerID)
	}
	if provider.Unavailable {
		reason := provider.UnavailableReason
		if reason == "" {
			reason = "web search provider is unavailable"
		}
		return provider, fmt.Errorf("%w: %s", common.ErrInvalidArgument, reason)
	}
	return provider, nil
}

func failedWebResult(request RuntimeOperationRequest, provider ProviderConfig, code string, err error) RuntimeOperationResult {
	stage := "web_failed"
	if request.Kind == webcommon.StepKindSearch {
		stage = "web_search"
	} else if request.Kind == webcommon.StepKindFetch {
		stage = "web_fetch"
	}
	ret := RuntimeOperationResult{
		Status: "failed", Stage: stage, ProviderID: provider.ID, Query: strings.TrimSpace(request.Query),
		URL: strings.TrimSpace(request.URL), FailureCode: code, TargetRefs: append([]string(nil), request.TargetRefs...),
	}
	if err != nil {
		ret.FailureMessage = err.Error()
	}
	ret.Summary = firstWebNonEmpty(ret.FailureMessage, stage)
	ret.Evidence = runtimecontract.RuntimeOperationEvidence{Summary: ret.Summary}
	return ret
}

func approvalRequiredSearchResult(request RuntimeOperationRequest, provider ProviderConfig, query string, maxResults int, policy policyDecision) RuntimeOperationResult {
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

func approvalRequiredFetchResult(request RuntimeOperationRequest, provider ProviderConfig, rawURL string, contextURL string, policy policyDecision) RuntimeOperationResult {
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

func (a *RuntimeAdapter) providerConfigs() (string, map[string]ProviderConfig, error) {
	if a.configLoader == nil {
		return a.defaultProviderID, normalizeProviders(a.providers), nil
	}
	revision := uint64(0)
	if a.configRevision != nil {
		revision = a.configRevision()
	}
	a.cacheMu.RLock()
	if a.cache.ready && a.cache.revision == revision {
		defaultID := a.cache.defaultProviderID
		providers := cloneProviders(a.cache.providers)
		a.cacheMu.RUnlock()
		return defaultID, providers, nil
	}
	a.cacheMu.RUnlock()

	defaultID, providers, err := a.configLoader()
	if err != nil {
		return "", nil, err
	}
	providers = normalizeProviders(providers)
	if strings.TrimSpace(defaultID) == "" {
		defaultID = defaultProviderIDFromProviders(providers)
	}
	a.cacheMu.Lock()
	a.cache = providerRuntimeCache{
		ready:             true,
		revision:          revision,
		defaultProviderID: defaultID,
		providers:         cloneProviders(providers),
	}
	a.cacheMu.Unlock()
	return defaultID, providers, nil
}

func (a *RuntimeAdapter) classifyURL(ctx context.Context, rawURL string, provider ProviderConfig, fetch bool) policyDecision {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return policyDecision{Decision: policyDenied, Reasons: []string{"invalid_url"}}
	}
	if parsed.User != nil {
		return policyDecision{Decision: policyDenied, Reasons: []string{"userinfo_url"}}
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return policyDecision{Decision: policyDenied, Reasons: []string{"unsupported_scheme"}}
	}
	host := strings.ToLower(strings.Trim(parsed.Hostname(), "."))
	if host == "" {
		return policyDecision{Decision: policyDenied, Reasons: []string{"missing_host"}}
	}
	if metadataHost(host) {
		return policyDecision{Decision: policyDenied, Reasons: []string{"metadata_endpoint"}}
	}
	if domainDenied(host, provider.DenyDomains) {
		return policyDecision{Decision: policyDenied, Reasons: []string{"deny_domain"}}
	}
	if len(provider.AllowDomains) > 0 && !domainDenied(host, provider.AllowDomains) {
		return policyDecision{Decision: policyDenied, Reasons: []string{"domain_not_allowed"}}
	}
	reasons := []string{}
	if scheme == "http" {
		reasons = append(reasons, "plain_http")
	}
	if fetch {
		ipDecision := a.classifyHostIPs(ctx, host)
		if ipDecision.Decision == policyDenied {
			return ipDecision
		}
		if ipDecision.Decision == policyApproval {
			reasons = append(reasons, ipDecision.Reasons...)
		}
	}
	if len(reasons) > 0 {
		return policyDecision{Decision: policyApproval, Reasons: uniqueStrings(reasons)}
	}
	return policyDecision{Decision: policyAllowed}
}

func (a *RuntimeAdapter) classifyHostIPs(ctx context.Context, host string) policyDecision {
	if addr, err := netip.ParseAddr(host); err == nil {
		return classifyAddr(addr)
	}
	ips, err := a.netResolver().LookupIPAddr(ctx, host)
	if err != nil {
		return policyDecision{Decision: policyDenied, Reasons: []string{"dns_lookup_failed"}}
	}
	reasons := []string{}
	for _, ip := range ips {
		addr, ok := netip.AddrFromSlice(ip.IP)
		if !ok {
			return policyDecision{Decision: policyDenied, Reasons: []string{"invalid_resolved_ip"}}
		}
		decision := classifyAddr(addr)
		if decision.Decision == policyDenied {
			return decision
		}
		if decision.Decision == policyApproval {
			reasons = append(reasons, decision.Reasons...)
		}
	}
	if len(reasons) > 0 {
		return policyDecision{Decision: policyApproval, Reasons: uniqueStrings(reasons)}
	}
	return policyDecision{Decision: policyAllowed}
}

func classifyAddr(addr netip.Addr) policyDecision {
	if addr.IsLoopback() {
		return policyDecision{Decision: policyApproval, Reasons: []string{"loopback"}}
	}
	if addr.IsPrivate() {
		return policyDecision{Decision: policyApproval, Reasons: []string{"private_network"}}
	}
	if addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() {
		return policyDecision{Decision: policyDenied, Reasons: []string{"link_local"}}
	}
	if addr.IsUnspecified() {
		return policyDecision{Decision: policyDenied, Reasons: []string{"unspecified_address"}}
	}
	if addr.IsMulticast() {
		return policyDecision{Decision: policyDenied, Reasons: []string{"multicast_address"}}
	}
	if addr.String() == "169.254.169.254" || addr.String() == "100.100.100.200" || strings.EqualFold(addr.String(), "fd00:ec2::254") {
		return policyDecision{Decision: policyDenied, Reasons: []string{"metadata_endpoint"}}
	}
	return policyDecision{Decision: policyAllowed}
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

func normalizeProviders(providers map[string]ProviderConfig) map[string]ProviderConfig {
	ret := map[string]ProviderConfig{}
	for id, provider := range providers {
		id = strings.TrimSpace(id)
		if id == "" {
			id = strings.TrimSpace(provider.ID)
		}
		if id == "" {
			continue
		}
		provider.ID = id
		if provider.Provider == "" {
			provider.Provider = webcommon.ProviderHTTP
		}
		if provider.Timeout <= 0 {
			provider.Timeout = defaultTimeout
		}
		provider.MaxResults = boundedInt(provider.MaxResults, defaultMaxResults, systemMaxResults)
		provider.MaxFetchBytes = boundedInt64(provider.MaxFetchBytes, defaultMaxFetchBytes, systemMaxFetchBytes)
		provider.MaxRedirects = boundedInt(provider.MaxRedirects, defaultMaxRedirects, systemMaxRedirects)
		if provider.Headers == nil {
			provider.Headers = map[string]string{}
		}
		ret[id] = provider
	}
	return ret
}

func defaultProviderIDFromProviders(providers map[string]ProviderConfig) string {
	if _, ok := providers[defaultProviderID]; ok {
		return defaultProviderID
	}
	for id := range providers {
		return id
	}
	return defaultProviderID
}

func cloneProviders(input map[string]ProviderConfig) map[string]ProviderConfig {
	ret := map[string]ProviderConfig{}
	for id, provider := range input {
		provider.Headers = cloneStringMap(provider.Headers)
		provider.AllowDomains = append([]string(nil), provider.AllowDomains...)
		provider.DenyDomains = append([]string(nil), provider.DenyDomains...)
		ret[id] = provider
	}
	return ret
}

func headersFromValues(values map[string]string) map[string]string {
	ret := map[string]string{}
	for key, value := range values {
		if strings.HasPrefix(key, "header.") {
			name := strings.TrimPrefix(key, "header.")
			if strings.TrimSpace(name) != "" && strings.TrimSpace(value) != "" {
				ret[name] = strings.TrimSpace(value)
			}
		}
	}
	return ret
}

func providerTimeout(provider ProviderConfig) time.Duration {
	if provider.Timeout <= 0 {
		return defaultTimeout
	}
	return provider.Timeout
}

func (a *RuntimeAdapter) httpClient() *http.Client {
	if a != nil && a.client != nil {
		return a.client
	}
	return http.DefaultClient
}

func (a *RuntimeAdapter) netResolver() *net.Resolver {
	if a != nil && a.resolver != nil {
		return a.resolver
	}
	return net.DefaultResolver
}

func reservedHeader(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "content-type", "accept", "user-agent":
		return true
	default:
		return false
	}
}

func metadataHost(host string) bool {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "metadata.google.internal", "169.254.169.254", "100.100.100.200", "fd00:ec2::254":
		return true
	default:
		return false
	}
}

func domainDenied(host string, domains []string) bool {
	host = strings.ToLower(strings.Trim(host, "."))
	for _, domain := range domains {
		domain = strings.ToLower(strings.Trim(strings.TrimSpace(domain), "."))
		if domain == "" {
			continue
		}
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
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

func durationValue(raw string, fallback time.Duration) time.Duration {
	if strings.TrimSpace(raw) == "" {
		return fallback
	}
	value, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func boundedIntValue(raw string, fallback int, max int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return fallback
	}
	return boundedInt(value, fallback, max)
}

func boundedInt64Value(raw string, fallback int64, max int64) int64 {
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return fallback
	}
	return boundedInt64(value, fallback, max)
}

func boundedInt(value int, fallback int, max int) int {
	if value <= 0 {
		value = fallback
	}
	if max > 0 && value > max {
		value = max
	}
	return value
}

func boundedInt64(value int64, fallback int64, max int64) int64 {
	if value <= 0 {
		value = fallback
	}
	if max > 0 && value > max {
		value = max
	}
	return value
}

func minPositive(value int, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func minPositive64(value int64, fallback int64) int64 {
	if value > 0 {
		return value
	}
	return fallback
}

func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	ret := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			ret = append(ret, part)
		}
	}
	return ret
}

func boolValue(raw string) bool {
	return strings.EqualFold(strings.TrimSpace(raw), "true")
}

func stableHash(parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		_, _ = hash.Write([]byte(part))
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func uniqueStrings(input []string) []string {
	seen := map[string]struct{}{}
	ret := make([]string, 0, len(input))
	for _, item := range input {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		ret = append(ret, item)
	}
	return ret
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func cloneStringMap(input map[string]string) map[string]string {
	ret := map[string]string{}
	for key, value := range input {
		ret[key] = value
	}
	return ret
}
