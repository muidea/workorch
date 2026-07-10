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
	"strconv"
	"strings"
	"sync"
	"time"

	webcommon "github.com/muidea/workorch/internal/modules/blocks/web/pkg/common"
	webmodels "github.com/muidea/workorch/internal/modules/blocks/web/pkg/models"
	webproviderconfig "github.com/muidea/workorch/internal/modules/blocks/web/pkg/providerconfig"
	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
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

func (a *RuntimeAdapter) ExecuteRuntimeStep(ctx context.Context, request runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
	if a == nil {
		return runtimemodel.StepExecutionResult{}, common.ErrInvalidArgument
	}
	provider, err := a.selectProvider(request)
	if err != nil {
		output := failedOutput(request.Kind, webcommon.ErrorProviderUnavailable, err, map[string]any{
			"provider": provider.ID,
		})
		return runtimemodel.StepExecutionResult{Output: output}, err
	}
	switch strings.TrimSpace(request.Kind) {
	case webcommon.StepKindSearch:
		return a.search(ctx, request, provider)
	case webcommon.StepKindFetch:
		return a.fetch(ctx, request, provider)
	default:
		err := fmt.Errorf("%w: unsupported web step kind %q", common.ErrInvalidArgument, request.Kind)
		return runtimemodel.StepExecutionResult{Output: failedOutput(request.Kind, webcommon.ErrorInvalidRequest, err, nil)}, err
	}
}

func (a *RuntimeAdapter) search(ctx context.Context, request runtimemodel.StepExecutionRequest, provider ProviderConfig) (runtimemodel.StepExecutionResult, error) {
	query := stringInput(request.Input, "query")
	if query == "" {
		err := fmt.Errorf("%w: web/search query is required", common.ErrInvalidArgument)
		return runtimemodel.StepExecutionResult{Output: failedOutput(request.Kind, webcommon.ErrorInvalidRequest, err, map[string]any{
			"provider": provider.ID,
		})}, err
	}
	policy := policyDecision{Decision: policyAllowed}
	requestedMaxResults := intInput(request.Input, "maxResults", provider.MaxResults)
	if requestedMaxResults <= 0 {
		requestedMaxResults = provider.MaxResults
	}
	providerMaxResults := minPositive(provider.MaxResults, systemMaxResults)
	approvalID := common.ApprovalID(stringInput(request.Input, "approvalId"))
	if requestedMaxResults > providerMaxResults {
		policy = policyDecision{Decision: policyApproval, Reasons: []string{"large_result_set"}}
		if approvalID == "" {
			return runtimemodel.StepExecutionResult{Output: approvalRequiredSearchOutput(request.Kind, provider, query, requestedMaxResults, policy)}, nil
		}
		if err := a.validateApproval(ctx, approvalID, request.RunID, webcommon.StepKindSearch, provider.ID, approvalContextQuery, query, policy); err != nil {
			err = fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
			return runtimemodel.StepExecutionResult{Output: failedOutput(request.Kind, webcommon.ErrorPolicyDenied, err, map[string]any{
				"provider":   provider.ID,
				"query":      query,
				"approvalId": string(approvalID),
				"policy":     policyOutput(policy),
			})}, err
		}
		providerMaxResults = systemMaxResults
	}
	maxResults := boundedInt(requestedMaxResults, defaultMaxResults, providerMaxResults)
	payload := map[string]any{
		"workspaceId":    string(request.WorkspaceID),
		"runId":          string(request.RunID),
		"stepId":         string(request.StepID),
		"query":          query,
		"locale":         stringInput(request.Input, "locale"),
		"recency":        stringInput(request.Input, "recency"),
		"domains":        stringSliceInput(request.Input, "domains"),
		"excludeDomains": stringSliceInput(request.Input, "excludeDomains"),
		"maxResults":     maxResults,
		"safeSearch":     boolInputWithDefault(request.Input, "safeSearch", true),
	}
	raw, duration, errCode, err := a.doProviderRequest(ctx, provider, provider.SearchEndpoint, payload, provider.MaxFetchBytes)
	if err != nil {
		return runtimemodel.StepExecutionResult{Output: failedOutput(request.Kind, errCode, err, map[string]any{
			"provider":       provider.ID,
			"query":          query,
			"searchedAt":     time.Now().UTC().Format(time.RFC3339),
			"durationMillis": duration.Milliseconds(),
			"policy":         policyOutput(policy),
		})}, err
	}
	var response searchGatewayResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		err = fmt.Errorf("%w: decode web/search response: %v", common.ErrInvalidArgument, err)
		return runtimemodel.StepExecutionResult{Output: failedOutput(request.Kind, webcommon.ErrorBadGatewayResponse, err, map[string]any{
			"provider":       provider.ID,
			"query":          query,
			"durationMillis": duration.Milliseconds(),
			"policy":         policyOutput(policy),
		})}, err
	}
	results := make([]map[string]any, 0, len(response.Results))
	for idx, item := range response.Results {
		itemURL := strings.TrimSpace(item.URL)
		if itemURL == "" {
			err := fmt.Errorf("%w: web/search result url is required", common.ErrInvalidArgument)
			return runtimemodel.StepExecutionResult{Output: failedOutput(request.Kind, webcommon.ErrorBadGatewayResponse, err, map[string]any{
				"provider": provider.ID,
				"query":    query,
				"policy":   policyOutput(policy),
			})}, err
		}
		decision := a.classifyURL(ctx, itemURL, provider, false)
		if decision.Decision != policyAllowed {
			err := fmt.Errorf("%w: web/search result url is not allowed: %s", common.ErrInvalidArgument, strings.Join(decision.Reasons, ","))
			return runtimemodel.StepExecutionResult{Output: failedOutput(request.Kind, webcommon.ErrorBadGatewayResponse, err, map[string]any{
				"provider": provider.ID,
				"query":    query,
				"policy":   policyOutput(decision),
			})}, err
		}
		results = append(results, map[string]any{
			"title":       item.Title,
			"url":         itemURL,
			"snippet":     item.Snippet,
			"source":      sourceForResult(item),
			"publishedAt": item.PublishedAt,
			"rank":        idx + 1,
			"contentHash": stableHash(item.Title, itemURL, item.Snippet, item.Source, item.PublishedAt),
		})
		if len(results) >= maxResults {
			break
		}
	}
	return runtimemodel.StepExecutionResult{Output: map[string]any{
		"status":         "completed",
		"stage":          "web_search",
		"provider":       provider.ID,
		"query":          query,
		"searchedAt":     time.Now().UTC().Format(time.RFC3339),
		"results":        results,
		"resultCount":    len(results),
		"truncated":      len(response.Results) > len(results),
		"approvalId":     string(approvalID),
		"durationMillis": duration.Milliseconds(),
		"policy":         policyOutput(policy),
	}}, nil
}

func (a *RuntimeAdapter) fetch(ctx context.Context, request runtimemodel.StepExecutionRequest, provider ProviderConfig) (runtimemodel.StepExecutionResult, error) {
	rawURL := stringInput(request.Input, "url")
	if rawURL == "" {
		err := fmt.Errorf("%w: web/fetch url is required", common.ErrInvalidArgument)
		return runtimemodel.StepExecutionResult{Output: failedOutput(request.Kind, webcommon.ErrorInvalidRequest, err, map[string]any{
			"provider": provider.ID,
		})}, err
	}
	if provider.FetchEndpoint == "" {
		err := fmt.Errorf("%w: web search provider fetchEndpoint is not configured", common.ErrInvalidArgument)
		return runtimemodel.StepExecutionResult{Output: failedOutput(request.Kind, webcommon.ErrorProviderUnavailable, err, map[string]any{
			"provider": provider.ID,
			"url":      rawURL,
		})}, err
	}
	policy := a.classifyURL(ctx, rawURL, provider, true)
	approvalID := common.ApprovalID(stringInput(request.Input, "approvalId"))
	if policy.Decision == policyDenied {
		err := fmt.Errorf("%w: web/fetch policy denied: %s", common.ErrInvalidArgument, strings.Join(policy.Reasons, ","))
		return runtimemodel.StepExecutionResult{Output: failedOutput(request.Kind, webcommon.ErrorPolicyDenied, err, map[string]any{
			"provider": provider.ID,
			"url":      rawURL,
			"policy":   policyOutput(policy),
		})}, err
	}
	if policy.Decision == policyApproval {
		if approvalID == "" {
			return runtimemodel.StepExecutionResult{Output: approvalRequiredOutput(request.Kind, provider, rawURL, policy, request.Input)}, nil
		}
		if err := a.validateApproval(ctx, approvalID, request.RunID, webcommon.StepKindFetch, provider.ID, approvalContextURL, rawURL, policy); err != nil {
			err = fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
			return runtimemodel.StepExecutionResult{Output: failedOutput(request.Kind, webcommon.ErrorPolicyDenied, err, map[string]any{
				"provider":   provider.ID,
				"url":        rawURL,
				"approvalId": string(approvalID),
				"policy":     policyOutput(policy),
			})}, err
		}
	}
	maxBytes := int64Input(request.Input, "maxBytes", provider.MaxFetchBytes)
	maxBytes = boundedInt64(maxBytes, defaultMaxFetchBytes, minPositive64(provider.MaxFetchBytes, systemMaxFetchBytes))
	payload := map[string]any{
		"workspaceId": string(request.WorkspaceID),
		"runId":       string(request.RunID),
		"stepId":      string(request.StepID),
		"url":         rawURL,
		"maxBytes":    maxBytes,
		"extractMode": stringInput(request.Input, "extractMode"),
	}
	raw, duration, errCode, err := a.doProviderRequest(ctx, provider, provider.FetchEndpoint, payload, maxBytes)
	if err != nil {
		return runtimemodel.StepExecutionResult{Output: failedOutput(request.Kind, errCode, err, map[string]any{
			"provider":       provider.ID,
			"url":            rawURL,
			"durationMillis": duration.Milliseconds(),
			"approvalId":     string(approvalID),
			"policy":         policyOutput(policy),
		})}, err
	}
	var response fetchGatewayResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		err = fmt.Errorf("%w: decode web/fetch response: %v", common.ErrInvalidArgument, err)
		return runtimemodel.StepExecutionResult{Output: failedOutput(request.Kind, webcommon.ErrorBadGatewayResponse, err, map[string]any{
			"provider": provider.ID,
			"url":      rawURL,
			"policy":   policyOutput(policy),
		})}, err
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
		return runtimemodel.StepExecutionResult{Output: failedOutput(request.Kind, webcommon.ErrorPolicyDenied, err, map[string]any{
			"provider": provider.ID,
			"url":      rawURL,
			"finalUrl": finalURL,
			"policy":   policyOutput(finalPolicy),
		})}, err
	}
	if finalPolicy.Decision == policyApproval {
		if approvalID == "" {
			return runtimemodel.StepExecutionResult{Output: approvalRequiredOutputWithContextURL(request.Kind, provider, rawURL, finalURL, finalPolicy, request.Input, map[string]any{
				"finalUrl": finalURL,
			})}, nil
		}
		if err := a.validateApproval(ctx, approvalID, request.RunID, webcommon.StepKindFetch, provider.ID, approvalContextURL, finalURL, finalPolicy); err != nil {
			err = fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
			return runtimemodel.StepExecutionResult{Output: failedOutput(request.Kind, webcommon.ErrorPolicyDenied, err, map[string]any{
				"provider":   provider.ID,
				"url":        rawURL,
				"finalUrl":   finalURL,
				"approvalId": string(approvalID),
				"policy":     policyOutput(finalPolicy),
			})}, err
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
	return runtimemodel.StepExecutionResult{Output: map[string]any{
		"status":         "completed",
		"stage":          "web_fetch",
		"provider":       provider.ID,
		"url":            rawURL,
		"finalUrl":       finalURL,
		"title":          response.Title,
		"content":        content,
		"summary":        response.Summary,
		"contentType":    response.ContentType,
		"contentHash":    stableHash(finalURL, response.Title, content, response.Summary, retrievedAt),
		"retrievedAt":    retrievedAt,
		"publishedAt":    response.PublishedAt,
		"source":         response.Source,
		"size":           len([]byte(content)),
		"truncated":      truncated,
		"redirects":      0,
		"approvalId":     string(approvalID),
		"durationMillis": duration.Milliseconds(),
		"policy":         policyOutput(policy),
	}}, nil
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

func (a *RuntimeAdapter) selectProvider(request runtimemodel.StepExecutionRequest) (ProviderConfig, error) {
	defaultID, providers, err := a.providerConfigs()
	if err != nil {
		return ProviderConfig{}, err
	}
	providerID := stringInput(request.Input, "providerId")
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
	if strings.TrimSpace(stringFromAny(ctx["providerId"])) != providerID {
		return false
	}
	if strings.TrimSpace(stringFromAny(ctx[contextKey])) != strings.TrimSpace(contextValue) {
		return false
	}
	gotReasons := stringSliceFromAny(ctx["policyReasons"])
	for _, reason := range policy.Reasons {
		if !containsString(gotReasons, reason) {
			return false
		}
	}
	return true
}

func approvalRequiredOutput(kind string, provider ProviderConfig, rawURL string, policy policyDecision, input map[string]any) map[string]any {
	return approvalRequiredOutputWithContextURL(kind, provider, rawURL, rawURL, policy, input, nil)
}

func approvalRequiredOutputWithContextURL(kind string, provider ProviderConfig, rawURL string, contextURL string, policy policyDecision, input map[string]any, extra map[string]any) map[string]any {
	context := map[string]any{
		"providerId":     provider.ID,
		"url":            contextURL,
		"maxBytes":       int64Input(input, "maxBytes", provider.MaxFetchBytes),
		"policyDecision": policy.Decision,
		"policyReasons":  append([]string(nil), policy.Reasons...),
	}
	reason := approvalReasonPrefix + ": " + strings.Join(policy.Reasons, ",")
	output := map[string]any{
		"status":               "blocked",
		"stage":                "web_fetch",
		"provider":             provider.ID,
		"url":                  rawURL,
		"errorCode":            webcommon.ErrorApprovalRequired,
		"error":                reason,
		"approvalRequired":     true,
		"approvalAction":       webcommon.StepKindFetch,
		"approvalContext":      cloneMap(context),
		"blockApprovalReason":  reason,
		"blockApprovalAction":  webcommon.StepKindFetch,
		"blockApprovalContext": cloneMap(context),
		"nextKind":             kind,
		"policy":               policyOutput(policy),
	}
	for key, value := range extra {
		output[key] = value
	}
	return output
}

func approvalRequiredSearchOutput(kind string, provider ProviderConfig, query string, maxResults int, policy policyDecision) map[string]any {
	context := map[string]any{
		"providerId":     provider.ID,
		"query":          query,
		"maxResults":     maxResults,
		"policyDecision": policy.Decision,
		"policyReasons":  append([]string(nil), policy.Reasons...),
	}
	reason := approvalReasonPrefix + ": " + strings.Join(policy.Reasons, ",")
	return map[string]any{
		"status":               "blocked",
		"stage":                "web_search",
		"provider":             provider.ID,
		"query":                query,
		"maxResults":           maxResults,
		"errorCode":            webcommon.ErrorApprovalRequired,
		"error":                reason,
		"approvalRequired":     true,
		"approvalAction":       webcommon.StepKindSearch,
		"approvalContext":      cloneMap(context),
		"blockApprovalReason":  reason,
		"blockApprovalAction":  webcommon.StepKindSearch,
		"blockApprovalContext": cloneMap(context),
		"nextKind":             kind,
		"policy":               policyOutput(policy),
	}
}

func failedOutput(kind string, code string, err error, extra map[string]any) map[string]any {
	stage := "web_failed"
	switch kind {
	case webcommon.StepKindSearch:
		stage = "web_search"
	case webcommon.StepKindFetch:
		stage = "web_fetch"
	}
	output := map[string]any{
		"status":    "failed",
		"stage":     stage,
		"errorCode": code,
	}
	if err != nil {
		output["error"] = err.Error()
	}
	for key, value := range extra {
		output[key] = value
	}
	return output
}

func policyOutput(policy policyDecision) map[string]any {
	return map[string]any{
		"decision": policy.Decision,
		"reasons":  append([]string(nil), policy.Reasons...),
	}
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

func stringInput(input map[string]any, key string) string {
	if input == nil {
		return ""
	}
	value, _ := input[key].(string)
	return strings.TrimSpace(value)
}

func stringFromAny(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func intInput(input map[string]any, key string, fallback int) int {
	if input == nil || input[key] == nil {
		return fallback
	}
	switch value := input[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		if value == float64(int(value)) {
			return int(value)
		}
	case json.Number:
		if parsed, err := value.Int64(); err == nil {
			return int(parsed)
		}
	}
	return fallback
}

func int64Input(input map[string]any, key string, fallback int64) int64 {
	if input == nil || input[key] == nil {
		return fallback
	}
	switch value := input[key].(type) {
	case int:
		return int64(value)
	case int64:
		return value
	case float64:
		if value == float64(int64(value)) {
			return int64(value)
		}
	case json.Number:
		if parsed, err := value.Int64(); err == nil {
			return parsed
		}
	}
	return fallback
}

func boolInputWithDefault(input map[string]any, key string, fallback bool) bool {
	if input == nil || input[key] == nil {
		return fallback
	}
	value, ok := input[key].(bool)
	if !ok {
		return fallback
	}
	return value
}

func stringSliceInput(input map[string]any, key string) []string {
	if input == nil {
		return nil
	}
	return stringSliceFromAny(input[key])
}

func stringSliceFromAny(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			text := strings.TrimSpace(fmt.Sprint(item))
			if text != "" {
				ret = append(ret, text)
			}
		}
		return ret
	default:
		return nil
	}
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

func cloneMap(input map[string]any) map[string]any {
	ret := map[string]any{}
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func cloneStringMap(input map[string]string) map[string]string {
	ret := map[string]string{}
	for key, value := range input {
		ret[key] = value
	}
	return ret
}
