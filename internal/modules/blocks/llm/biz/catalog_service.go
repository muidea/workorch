package llm

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	llmproviderconfig "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/providerconfig"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// External wire DTOs for ai-proxy GET /v1/models.
type aiProxyModelsResponse struct {
	Object string               `json:"object"`
	Data   []aiProxyModelRecord `json:"data"`
}

type aiProxyModelRecord struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
	// Operations is intentionally ignored; latest ai-proxy uses supported_endpoints.
	Operations          []string `json:"operations"`
	SupportedEndpoints  []string `json:"supported_endpoints"`
	ContextWindowTokens int      `json:"contextWindowTokens"`
	MaxOutputTokens     int      `json:"maxOutputTokens"`
	Capabilities        struct {
		Reasoning *struct {
			Supported     bool     `json:"supported"`
			DefaultEffort string   `json:"default_effort"`
			Efforts       []string `json:"efforts"`
		} `json:"reasoning"`
	} `json:"capabilities"`
}

type catalogCacheKey struct {
	ProviderID         string
	BaseURL            string
	ConnectionRevision string
}

type catalogCacheEntry struct {
	key           catalogCacheKey
	catalog       llmmodels.ProviderModelCatalog
	connectionRev string
}

// CatalogService owns ai-proxy catalog HTTP access, TTL/LKG cache, and refresh merge.
// Entries and inflight refreshes are isolated by catalog cache key.
type CatalogService struct {
	mu       sync.Mutex
	entries  map[catalogCacheKey]*catalogCacheEntry
	inflight map[catalogCacheKey]*catalogRefreshCall
	client   *http.Client
	now      func() time.Time
}

type catalogRefreshCall struct {
	key     catalogCacheKey
	done    chan struct{}
	catalog llmmodels.ProviderModelCatalog
	err     error
}

func NewCatalogService() *CatalogService {
	return &CatalogService{
		entries:  map[catalogCacheKey]*catalogCacheEntry{},
		inflight: map[catalogCacheKey]*catalogRefreshCall{},
		client:   &http.Client{CheckRedirect: sameHostRedirectsOnly},
		now:      time.Now,
	}
}

func catalogCacheKeyFromConfig(cfg workorchconfig.LLMConfig) catalogCacheKey {
	return catalogCacheKey{
		ProviderID:         runtimecontract.BuiltinAIProxyProviderID,
		BaseURL:            strings.TrimSpace(cfg.AIProxy.BaseURL),
		ConnectionRevision: CatalogConnectionRevision(cfg),
	}
}

func sameHostRedirectsOnly(req *http.Request, via []*http.Request) error {
	if len(via) == 0 {
		return nil
	}
	if len(via) >= 5 {
		return fmt.Errorf("too many redirects")
	}
	origin := via[0].URL
	if req.URL.Scheme != origin.Scheme || req.URL.Host != origin.Host {
		return fmt.Errorf("cross-host redirect is not allowed")
	}
	// Do not forward Authorization across redirects.
	req.Header.Del("Authorization")
	return nil
}

func (s *CatalogService) GetCatalog(ctx context.Context, cfg workorchconfig.LLMConfig, forceRefresh bool) (llmmodels.ProviderModelCatalog, error) {
	if s == nil {
		return llmmodels.ProviderModelCatalog{}, common.ErrInvalidArgument
	}
	cfg, err := workorchconfig.NormalizeLLMConfig(cfg)
	if err != nil {
		return llmmodels.ProviderModelCatalog{}, err
	}
	key := catalogCacheKeyFromConfig(cfg)
	now := s.now()

	s.mu.Lock()
	if s.entries == nil {
		s.entries = map[catalogCacheKey]*catalogCacheEntry{}
	}
	if s.inflight == nil {
		s.inflight = map[catalogCacheKey]*catalogRefreshCall{}
	}
	if !forceRefresh {
		if entry := s.entries[key]; entry != nil {
			if now.Before(entry.catalog.ExpiresAt) {
				catalog := cloneCatalog(entry.catalog)
				catalog.State = llmmodels.ProviderCatalogStateFresh
				catalog.Stale = false
				s.mu.Unlock()
				return catalog, nil
			}
			maxStaleAge, _ := time.ParseDuration(cfg.AIProxy.CatalogMaxStaleAge)
			if now.Sub(entry.catalog.FetchedAt) > maxStaleAge {
				delete(s.entries, key)
			}
		}
	}
	if call := s.inflight[key]; call != nil {
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return llmmodels.ProviderModelCatalog{}, ctx.Err()
		case <-call.done:
			if call.err != nil {
				return s.fallbackOrError(cfg, key, call.err)
			}
			// Only accept result when call key still matches request key.
			if call.key != key {
				return s.fallbackOrError(cfg, key, fmt.Errorf("%w: catalog refresh key mismatch", common.ErrInvalidArgument))
			}
			return cloneCatalog(call.catalog), nil
		}
	}
	call := &catalogRefreshCall{key: key, done: make(chan struct{})}
	s.inflight[key] = call
	s.mu.Unlock()

	catalog, refreshErr := s.fetchCatalog(ctx, cfg)
	s.mu.Lock()
	call.catalog = catalog
	call.err = refreshErr
	close(call.done)
	delete(s.inflight, key)
	if refreshErr == nil {
		// Reject stale writes if the entry key no longer matches (config changed mid-flight).
		if call.key == key {
			s.entries[key] = &catalogCacheEntry{key: key, catalog: cloneCatalog(catalog), connectionRev: key.ConnectionRevision}
		}
		s.mu.Unlock()
		if call.key != key {
			return llmmodels.ProviderModelCatalog{}, fmt.Errorf("%w: catalog refresh result discarded due to key change", common.ErrInvalidArgument)
		}
		return cloneCatalog(catalog), nil
	}
	s.mu.Unlock()
	return s.fallbackOrError(cfg, key, refreshErr)
}

func (s *CatalogService) fallbackOrError(cfg workorchconfig.LLMConfig, key catalogCacheKey, refreshErr error) (llmmodels.ProviderModelCatalog, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.entries[key]
	if entry == nil || entry.key != key {
		return llmmodels.ProviderModelCatalog{
			ProviderID:   runtimecontract.BuiltinAIProxyProviderID,
			State:        llmmodels.ProviderCatalogStateUnavailable,
			Source:       "ai_proxy",
			ErrorCode:    string(llmmodels.ProviderModelFailureCatalogUnavailable),
			ErrorMessage: redactCatalogError(refreshErr, cfg.AIProxy.APIKey),
		}, refreshErr
	}
	maxStaleAge, _ := time.ParseDuration(cfg.AIProxy.CatalogMaxStaleAge)
	if s.now().Sub(entry.catalog.FetchedAt) > maxStaleAge {
		delete(s.entries, key)
		return llmmodels.ProviderModelCatalog{
			ProviderID:   runtimecontract.BuiltinAIProxyProviderID,
			State:        llmmodels.ProviderCatalogStateUnavailable,
			Source:       "ai_proxy",
			ErrorCode:    string(llmmodels.ProviderModelFailureCatalogUnavailable),
			ErrorMessage: redactCatalogError(refreshErr, cfg.AIProxy.APIKey),
		}, refreshErr
	}
	catalog := cloneCatalog(entry.catalog)
	catalog.Stale = true
	catalog.State = llmmodels.ProviderCatalogStateStale
	catalog.ErrorCode = string(llmmodels.ProviderModelFailureCatalogUnavailable)
	catalog.ErrorMessage = redactCatalogError(refreshErr, cfg.AIProxy.APIKey)
	return catalog, nil
}

func (s *CatalogService) fetchCatalog(ctx context.Context, cfg workorchconfig.LLMConfig) (llmmodels.ProviderModelCatalog, error) {
	timeout, err := time.ParseDuration(cfg.AIProxy.CatalogRequestTimeout)
	if err != nil || timeout <= 0 {
		timeout = 5 * time.Second
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	endpoint, err := normalizeAIProxyModelsURL(cfg.AIProxy.BaseURL)
	if err != nil {
		return llmmodels.ProviderModelCatalog{}, err
	}
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return llmmodels.ProviderModelCatalog{}, err
	}
	req.Header.Set("Accept", "application/json")
	setBearerAuthorization(req.Header, cfg.AIProxy.APIKey)

	client := s.client
	if client == nil {
		client = &http.Client{CheckRedirect: sameHostRedirectsOnly}
	}
	// One retry for network/5xx within the same timeout budget.
	var resp *http.Response
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		resp, lastErr = client.Do(req.Clone(reqCtx))
		if lastErr == nil && resp.StatusCode < 500 {
			break
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
		if attempt == 1 || reqCtx.Err() != nil {
			break
		}
	}
	if lastErr != nil {
		return llmmodels.ProviderModelCatalog{}, fmt.Errorf("%w: catalog request failed: %v", common.ErrInvalidArgument, lastErr)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		summary := readLimitedErrorBody(resp.Body, 1024, cfg.AIProxy.APIKey)
		return llmmodels.ProviderModelCatalog{}, fmt.Errorf("%w: catalog request status %d: %s", common.ErrInvalidArgument, resp.StatusCode, summary)
	}
	limited := io.LimitReader(resp.Body, int64(cfg.AIProxy.MaxCatalogResponseBytes)+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return llmmodels.ProviderModelCatalog{}, fmt.Errorf("%w: catalog response read failed: %v", common.ErrInvalidArgument, err)
	}
	if len(body) > cfg.AIProxy.MaxCatalogResponseBytes {
		return llmmodels.ProviderModelCatalog{}, fmt.Errorf("%w: catalog response exceeds maxCatalogResponseBytes", common.ErrInvalidArgument)
	}
	var wire aiProxyModelsResponse
	if err := json.Unmarshal(body, &wire); err != nil {
		return llmmodels.ProviderModelCatalog{}, fmt.Errorf("%w: catalog response decode failed: %v", common.ErrInvalidArgument, err)
	}
	if strings.TrimSpace(wire.Object) != "list" {
		return llmmodels.ProviderModelCatalog{}, fmt.Errorf("%w: catalog object must be list", common.ErrInvalidArgument)
	}
	if len(wire.Data) > cfg.AIProxy.MaxCatalogModels {
		return llmmodels.ProviderModelCatalog{}, fmt.Errorf("%w: catalog model count exceeds maxCatalogModels", common.ErrInvalidArgument)
	}
	return buildCatalogFromWire(wire, cfg, s.now())
}

func buildCatalogFromWire(wire aiProxyModelsResponse, cfg workorchconfig.LLMConfig, now time.Time) (llmmodels.ProviderModelCatalog, error) {
	ttl, _ := time.ParseDuration(cfg.AIProxy.CatalogTTL)
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	// ai-proxy routes model IDs by exact string. Model casing is therefore part
	// of the durable target identity and must not be collapsed in WorkOrch.
	seen := map[string]struct{}{}
	models := make([]llmmodels.ProviderModelDescriptor, 0, len(wire.Data))
	for _, item := range wire.Data {
		modelID := sanitizeCatalogLogValue(strings.TrimSpace(item.ID), 256)
		if modelID == "" {
			return llmmodels.ProviderModelCatalog{}, fmt.Errorf("%w: catalog model id is required", common.ErrInvalidArgument)
		}
		if _, ok := seen[modelID]; ok {
			return llmmodels.ProviderModelCatalog{}, fmt.Errorf("%w: catalog contains duplicate model id %q", common.ErrInvalidArgument, modelID)
		}
		seen[modelID] = struct{}{}
		routeOwner := sanitizeCatalogLogValue(strings.TrimSpace(item.OwnedBy), 128)
		if routeOwner == "" {
			routeOwner = runtimecontract.BuiltinAIProxyProviderID
		}
		endpoints := normalizeSupportedEndpoints(item.SupportedEndpoints)
		ops := normalizeCatalogOperations(endpoints)
		desc := llmmodels.ProviderModelDescriptor{
			Target: runtimecontract.ProviderModelTarget{
				ProviderID: runtimecontract.BuiltinAIProxyProviderID,
				ModelID:    modelID,
			},
			RouteOwner:          routeOwner,
			ContextWindowTokens: item.ContextWindowTokens,
			MaxOutputTokens:     item.MaxOutputTokens,
			SupportedEndpoints:  endpoints,
			Operations:          ops,
		}
		if item.Capabilities.Reasoning != nil {
			desc.Reasoning = llmmodels.ProviderReasoningCapability{
				Supported:     item.Capabilities.Reasoning.Supported,
				DefaultEffort: item.Capabilities.Reasoning.DefaultEffort,
				Efforts:       append([]string(nil), item.Capabilities.Reasoning.Efforts...),
			}.Normalized()
		} else {
			desc.Reasoning = (llmmodels.ProviderReasoningCapability{}).Normalized()
		}
		hasResponses := containsOperation(ops, workorchconfig.LLMOperationResponses)
		hasEmbeddings := containsOperation(ops, workorchconfig.LLMOperationEmbeddings)
		switch {
		case len(ops) == 0:
			// No supported endpoint for a WorkOrch adapter: displayable, but not ready.
			desc.CapabilityReady = false
			desc.UnavailableReason = string(llmmodels.ProviderModelFailureCapabilityIncomplete)
		case item.ContextWindowTokens <= 0:
			desc.CapabilityReady = false
			desc.UnavailableReason = string(llmmodels.ProviderModelFailureCapabilityIncomplete)
		case hasResponses && (item.MaxOutputTokens <= 0 || item.MaxOutputTokens >= item.ContextWindowTokens):
			desc.CapabilityReady = false
			desc.UnavailableReason = string(llmmodels.ProviderModelFailureCapabilityIncomplete)
		case !hasResponses && !hasEmbeddings:
			desc.CapabilityReady = false
			desc.UnavailableReason = string(llmmodels.ProviderModelFailureCapabilityIncomplete)
		default:
			desc.CapabilityReady = true
		}
		desc.CapabilityRevision = capabilityRevision(desc.Target, desc.ContextWindowTokens, desc.MaxOutputTokens, descriptorCapabilityRevisionInput(desc))
		models = append(models, desc)
	}
	sort.SliceStable(models, func(i, j int) bool {
		return models[i].Target.ModelID < models[j].Target.ModelID
	})
	catalog := llmmodels.ProviderModelCatalog{
		ProviderID: runtimecontract.BuiltinAIProxyProviderID,
		Models:     models,
		Revision:   catalogRevision(runtimecontract.BuiltinAIProxyProviderID, models),
		FetchedAt:  now,
		ExpiresAt:  now.Add(ttl),
		Stale:      false,
		Source:     "ai_proxy",
		State:      llmmodels.ProviderCatalogStateFresh,
	}
	return catalog, nil
}

func normalizeAIProxyModelsURL(baseURL string) (string, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return "", fmt.Errorf("%w: ai-proxy baseURL is required", common.ErrInvalidArgument)
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("%w: ai-proxy baseURL is invalid: %v", common.ErrInvalidArgument, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("%w: ai-proxy baseURL must use http or https", common.ErrInvalidArgument)
	}
	path := strings.TrimRight(parsed.Path, "/")
	switch {
	case strings.HasSuffix(path, "/v1/models"):
		// already correct
	case strings.HasSuffix(path, "/v1"):
		path = path + "/models"
	case path == "":
		path = "/v1/models"
	default:
		path = path + "/v1/models"
	}
	parsed.Path = path
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func catalogRevision(providerID string, models []llmmodels.ProviderModelDescriptor) string {
	h := sha256.New()
	writeLenPrefixed(h, providerID)
	_ = binary.Write(h, binary.BigEndian, int64(len(models)))
	for _, model := range models {
		writeLenPrefixed(h, model.Target.ModelID)
		writeLenPrefixed(h, model.RouteOwner)
		_ = binary.Write(h, binary.BigEndian, int64(model.ContextWindowTokens))
		_ = binary.Write(h, binary.BigEndian, int64(model.MaxOutputTokens))
		ops := append([]string(nil), model.Operations...)
		sort.Strings(ops)
		for _, op := range ops {
			writeLenPrefixed(h, op)
		}
		endpoints := append([]string(nil), model.SupportedEndpoints...)
		sort.Strings(endpoints)
		for _, endpoint := range endpoints {
			writeLenPrefixed(h, endpoint)
		}
		reasoning := model.Reasoning.Normalized()
		if reasoning.Supported {
			writeLenPrefixed(h, "reasoning-supported")
		} else {
			writeLenPrefixed(h, "reasoning-unsupported")
		}
		if reasoning.Valid {
			writeLenPrefixed(h, "reasoning-valid")
		} else {
			writeLenPrefixed(h, "reasoning-invalid")
		}
		writeLenPrefixed(h, reasoning.DefaultEffort)
		for _, effort := range reasoning.Efforts {
			writeLenPrefixed(h, effort)
		}
		if model.CapabilityReady {
			writeLenPrefixed(h, "1")
		} else {
			writeLenPrefixed(h, "0")
		}
	}
	return "catalog-sha256:" + hex.EncodeToString(h.Sum(nil))
}

func descriptorCapabilityRevisionInput(desc llmmodels.ProviderModelDescriptor) string {
	parts := append([]string(nil), desc.Operations...)
	reasoning := desc.Reasoning.Normalized()
	parts = append(parts, fmt.Sprintf("reasoning:supported=%t", reasoning.Supported))
	parts = append(parts, fmt.Sprintf("reasoning:valid=%t", reasoning.Valid))
	parts = append(parts, "reasoning:default="+reasoning.DefaultEffort)
	for _, effort := range reasoning.Efforts {
		parts = append(parts, "reasoning:effort="+effort)
	}
	return strings.Join(parts, ",")
}

func capabilityRevision(target runtimecontract.ProviderModelTarget, contextWindowTokens int, maxOutputTokens int, overrideRevision string) string {
	h := sha256.New()
	writeLenPrefixed(h, target.ProviderID)
	writeLenPrefixed(h, target.ModelID)
	_ = binary.Write(h, binary.BigEndian, int64(contextWindowTokens))
	_ = binary.Write(h, binary.BigEndian, int64(maxOutputTokens))
	writeLenPrefixed(h, overrideRevision)
	return "capability-sha256:" + hex.EncodeToString(h.Sum(nil))
}

func writeLenPrefixed(w io.Writer, value string) {
	_ = binary.Write(w, binary.BigEndian, int64(len(value)))
	_, _ = io.WriteString(w, value)
}

// configurationRevision is retained as a package-local alias for catalog connection identity.
// Target-specific sealed revisions must use TargetConfigurationRevision.
func configurationRevision(cfg workorchconfig.LLMConfig) string {
	return CatalogConnectionRevision(cfg)
}

func cloneCatalog(catalog llmmodels.ProviderModelCatalog) llmmodels.ProviderModelCatalog {
	catalog.Models = append([]llmmodels.ProviderModelDescriptor(nil), catalog.Models...)
	for idx := range catalog.Models {
		catalog.Models[idx].SupportedEndpoints = append([]string(nil), catalog.Models[idx].SupportedEndpoints...)
		catalog.Models[idx].Operations = append([]string(nil), catalog.Models[idx].Operations...)
		catalog.Models[idx].Reasoning = catalog.Models[idx].Reasoning.Clone()
	}
	return catalog
}

func readLimitedErrorBody(r io.Reader, limit int64, secrets ...string) string {
	body, _ := io.ReadAll(io.LimitReader(r, limit))
	text := strings.TrimSpace(string(body))
	text = strings.ReplaceAll(text, "\n", " ")
	if len(text) > 256 {
		text = text[:256]
	}
	return redactCatalogError(fmt.Errorf("%s", text), secrets...)
}

func redactCatalogError(err error, secrets ...string) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	// Drop control characters and collapse whitespace for safe log/read models.
	cleaned := make([]rune, 0, len(text))
	for _, r := range text {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			cleaned = append(cleaned, ' ')
		case r < 0x20 || r == 0x7f:
			continue
		default:
			cleaned = append(cleaned, r)
		}
	}
	text = strings.Join(strings.Fields(string(cleaned)), " ")
	for _, secret := range secrets {
		secret = strings.TrimSpace(secret)
		if secret == "" {
			continue
		}
		text = strings.ReplaceAll(text, secret, llmproviderconfig.RedactSecret(secret))
		text = strings.ReplaceAll(text, "Bearer "+secret, "Bearer "+llmproviderconfig.RedactSecret(secret))
		text = strings.ReplaceAll(text, "bearer "+secret, "bearer "+llmproviderconfig.RedactSecret(secret))
	}
	text = strings.ReplaceAll(text, "Bearer ", "Bearer [redacted] ")
	text = strings.ReplaceAll(text, "bearer ", "bearer [redacted] ")
	if len(text) > 512 {
		text = text[:512]
	}
	return text
}

func normalizeSupportedEndpoints(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	ret := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || !strings.HasPrefix(value, "/v1/") {
			return nil
		}
		if _, ok := seen[value]; ok {
			return nil
		}
		seen[value] = struct{}{}
		ret = append(ret, value)
	}
	sort.Strings(ret)
	return ret
}

// normalizeCatalogOperations derives WorkOrch operations from exact client
// endpoint capabilities. The response adapter currently supports Responses
// generation and embeddings; other endpoints remain informational only.
func normalizeCatalogOperations(endpoints []string) []string {
	if len(endpoints) == 0 {
		return nil
	}
	ret := make([]string, 0, len(endpoints))
	seen := map[string]struct{}{}
	for _, endpoint := range endpoints {
		var operation string
		switch endpoint {
		case "/v1/responses":
			operation = workorchconfig.LLMOperationResponses
		case "/v1/embeddings":
			operation = workorchconfig.LLMOperationEmbeddings
		default:
			continue
		}
		if _, ok := seen[operation]; !ok {
			seen[operation] = struct{}{}
			ret = append(ret, operation)
		}
	}
	if len(ret) == 0 {
		return nil
	}
	sort.Strings(ret)
	return ret
}
