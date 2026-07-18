package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

const maxProviderErrorBodyBytes = 4096
const maxProviderStreamLineBytes = 16 * 1024 * 1024
const maxProviderStreamContentBytes = 8 * 1024 * 1024
const maxProviderStreamDataEvents = 32768

type providerStreamLimits struct {
	MaxContentBytes int
	MaxDataEvents   int
}

var defaultProviderStreamLimits = providerStreamLimits{
	MaxContentBytes: maxProviderStreamContentBytes,
	MaxDataEvents:   maxProviderStreamDataEvents,
}

// ProviderHTTPStatusError preserves the provider status code across adapter
// boundaries while keeping the response body sanitized.
type ProviderHTTPStatusError struct {
	StatusCode int
	Body       string
}

func (e *ProviderHTTPStatusError) Error() string {
	if e == nil {
		return "llm provider http status error"
	}
	return fmt.Sprintf("llm provider http status %d: %s", e.StatusCode, e.Body)
}

func (e *ProviderHTTPStatusError) Unwrap() error { return common.ErrInvalidArgument }

func providerHTTPStatusError(statusCode int, body []byte, apiKey string) error {
	return &ProviderHTTPStatusError{StatusCode: statusCode, Body: sanitizeProviderBody(body, apiKey)}
}

type providerStreamGuard struct {
	limits       providerStreamLimits
	contentBytes int
	dataEvents   int
}

func newProviderStreamGuard(limits providerStreamLimits) *providerStreamGuard {
	if limits.MaxContentBytes <= 0 {
		limits.MaxContentBytes = maxProviderStreamContentBytes
	}
	if limits.MaxDataEvents <= 0 {
		limits.MaxDataEvents = maxProviderStreamDataEvents
	}
	return &providerStreamGuard{limits: limits}
}

func (g *providerStreamGuard) observeDataEvent() error {
	if g == nil {
		return nil
	}
	g.dataEvents++
	if g.dataEvents > g.limits.MaxDataEvents {
		return fmt.Errorf("%w: llm provider stream incomplete: streamed response exceeded data event limit %d", common.ErrInvalidArgument, g.limits.MaxDataEvents)
	}
	return nil
}

func (g *providerStreamGuard) observeDelta(delta string) error {
	if g == nil || delta == "" {
		return nil
	}
	next := g.contentBytes + len(delta)
	if next > g.limits.MaxContentBytes {
		return fmt.Errorf("%w: llm provider stream incomplete: streamed response exceeded content byte limit %d", common.ErrInvalidArgument, g.limits.MaxContentBytes)
	}
	g.contentBytes = next
	return nil
}

type HTTPProvider struct {
	config llmmodels.ProviderConfig
	client *http.Client
}

func NewHTTPProvider(config llmmodels.ProviderConfig) *HTTPProvider {
	config = normalizedProviderConfigForRuntime(config)
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = defaultProviderTimeout
	}
	return &HTTPProvider{
		config: config,
		client: &http.Client{Timeout: timeout},
	}
}

func NewHTTPProviderWithClient(config llmmodels.ProviderConfig, client *http.Client) *HTTPProvider {
	if client == nil {
		return NewHTTPProvider(config)
	}
	config = normalizedProviderConfigForRuntime(config)
	return &HTTPProvider{config: config, client: client}
}

func (p *HTTPProvider) Complete(ctx context.Context, request llmmodels.ProviderRequest) (llmmodels.ProviderCompletionResult, error) {
	if p == nil || p.client == nil {
		return llmmodels.ProviderCompletionResult{}, common.ErrInvalidArgument
	}
	switch p.surfaceOrDefault() {
	case llmcommon.ProviderSurfaceOpenAICompatibleHTTP:
		return p.completeOpenAICompatible(ctx, request)
	case llmcommon.ProviderSurfaceAnthropicCompatibleHTTP:
		return p.completeAnthropicCompatible(ctx, request)
	default:
		return llmmodels.ProviderCompletionResult{}, fmt.Errorf("%w: llm provider apiSurface %s is not implemented by HTTPProvider", common.ErrInvalidArgument, p.surfaceOrDefault())
	}
}

func (p *HTTPProvider) completeOpenAICompatible(ctx context.Context, request llmmodels.ProviderRequest) (llmmodels.ProviderCompletionResult, error) {
	if err := p.ensureOpenAICompatibleHTTPSurface(); err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	endpoint, err := chatCompletionsURL(p.config.BaseURL)
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	payload, err := json.Marshal(chatCompletionRequest{
		Model:               p.config.Model,
		Messages:            chatMessagesForRequest(&request),
		PromptCacheKey:      promptCacheKeyForRequest(p.config, request),
		Temperature:         providerTemperature(p.config),
		MaxCompletionTokens: maxOutputTokens(p.config),
		ReasoningEffort:     p.config.InferenceProfile.ReasoningEffort,
		Thinking:            compatibleThinkingConfig(p.config),
		ResponseFormat:      map[string]string{"type": "json_object"},
	})
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	setBearerAuthorization(httpReq.Header, p.config.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxProviderErrorBodyBytes))
		return llmmodels.ProviderCompletionResult{}, providerHTTPStatusError(resp.StatusCode, body, p.config.APIKey)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	var decoded chatCompletionResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return llmmodels.ProviderCompletionResult{}, fmt.Errorf("%w: llm provider protocol invalid: invalid response json: %v", common.ErrInvalidArgument, err)
	}
	if decoded.Error != nil {
		return llmmodels.ProviderCompletionResult{}, fmt.Errorf("%w: llm provider protocol invalid: %s", common.ErrInvalidArgument, decoded.Error.Summary())
	}
	if len(decoded.Choices) == 0 {
		return llmmodels.ProviderCompletionResult{}, fmt.Errorf("%w: llm provider response schema invalid: choices are required", common.ErrInvalidArgument)
	}
	content := strings.TrimSpace(decoded.FirstContent())
	if content == "" {
		return llmmodels.ProviderCompletionResult{}, fmt.Errorf("%w: llm provider response schema invalid: first choice content is empty", common.ErrInvalidArgument)
	}
	return llmmodels.ProviderCompletionResult{
		Payload: []byte(content),
		Usage:   decoded.Usage.ProviderUsage(),
	}, nil
}

func (p *HTTPProvider) StreamComplete(ctx context.Context, request llmmodels.ProviderRequest, emit func(llmmodels.ProviderStreamEvent) error) (llmmodels.ProviderCompletionResult, error) {
	if p == nil || p.client == nil {
		return llmmodels.ProviderCompletionResult{}, common.ErrInvalidArgument
	}
	switch p.surfaceOrDefault() {
	case llmcommon.ProviderSurfaceOpenAICompatibleHTTP:
		return p.streamOpenAICompatible(ctx, request, emit)
	case llmcommon.ProviderSurfaceAnthropicCompatibleHTTP:
		return p.streamAnthropicCompatible(ctx, request, emit)
	default:
		return llmmodels.ProviderCompletionResult{}, fmt.Errorf("%w: llm provider apiSurface %s is not implemented by HTTPProvider", common.ErrInvalidArgument, p.surfaceOrDefault())
	}
}

func (p *HTTPProvider) streamOpenAICompatible(ctx context.Context, request llmmodels.ProviderRequest, emit func(llmmodels.ProviderStreamEvent) error) (llmmodels.ProviderCompletionResult, error) {
	if err := p.ensureOpenAICompatibleHTTPSurface(); err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	endpoint, err := chatCompletionsURL(p.config.BaseURL)
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	payload, err := json.Marshal(chatCompletionRequest{
		Model:               p.config.Model,
		Messages:            chatMessagesForRequest(&request),
		PromptCacheKey:      promptCacheKeyForRequest(p.config, request),
		Temperature:         providerTemperature(p.config),
		MaxCompletionTokens: maxOutputTokens(p.config),
		ReasoningEffort:     p.config.InferenceProfile.ReasoningEffort,
		Thinking:            compatibleThinkingConfig(p.config),
		ResponseFormat:      map[string]string{"type": "json_object"},
		Stream:              true,
		StreamOptions:       &chatCompletionStreamOptions{IncludeUsage: true},
	})
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	setBearerAuthorization(httpReq.Header, p.config.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxProviderErrorBodyBytes))
		return llmmodels.ProviderCompletionResult{}, providerHTTPStatusError(resp.StatusCode, body, p.config.APIKey)
	}
	content, usage, err := readChatCompletionStream(resp.Body, emit)
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return llmmodels.ProviderCompletionResult{}, fmt.Errorf("%w: llm provider streamed response content is empty", common.ErrInvalidArgument)
	}
	return llmmodels.ProviderCompletionResult{
		Payload: []byte(content),
		Usage:   usage,
	}, nil
}

func (p *HTTPProvider) Embed(ctx context.Context, request llmmodels.ProviderEmbeddingRequest) (llmmodels.ProviderEmbeddingResult, error) {
	if p == nil || p.client == nil {
		return llmmodels.ProviderEmbeddingResult{}, common.ErrInvalidArgument
	}
	if err := p.ensureOpenAICompatibleHTTPSurface(); err != nil {
		return llmmodels.ProviderEmbeddingResult{}, err
	}
	input := strings.TrimSpace(request.Input)
	if input == "" {
		return llmmodels.ProviderEmbeddingResult{}, fmt.Errorf("%w: embedding input is required", common.ErrInvalidArgument)
	}
	model := strings.TrimSpace(request.Model)
	if model == "" {
		model = p.config.Model
	}
	endpoint, err := embeddingsURL(p.config.BaseURL)
	if err != nil {
		return llmmodels.ProviderEmbeddingResult{}, err
	}
	payload, err := json.Marshal(embeddingRequest{
		Model: model,
		Input: input,
	})
	if err != nil {
		return llmmodels.ProviderEmbeddingResult{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return llmmodels.ProviderEmbeddingResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	setBearerAuthorization(httpReq.Header, p.config.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return llmmodels.ProviderEmbeddingResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxProviderErrorBodyBytes))
		return llmmodels.ProviderEmbeddingResult{}, providerHTTPStatusError(resp.StatusCode, body, p.config.APIKey)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return llmmodels.ProviderEmbeddingResult{}, err
	}
	var decoded embeddingResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return llmmodels.ProviderEmbeddingResult{}, fmt.Errorf("%w: invalid llm provider embedding response: %v", common.ErrInvalidArgument, err)
	}
	vector := decoded.FirstEmbedding()
	if len(vector) == 0 {
		return llmmodels.ProviderEmbeddingResult{}, fmt.Errorf("%w: llm provider embedding response is empty", common.ErrInvalidArgument)
	}
	return llmmodels.ProviderEmbeddingResult{
		ProviderID: p.config.ID,
		Model:      model,
		Input:      input,
		Vector:     vector,
	}, nil
}

func setBearerAuthorization(header http.Header, apiKey string) {
	if header == nil {
		return
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		header.Del("Authorization")
		return
	}
	header.Set("Authorization", "Bearer "+apiKey)
}

type chatCompletionRequest struct {
	Model               string                         `json:"model"`
	Messages            []chatCompletionMessage        `json:"messages"`
	PromptCacheKey      string                         `json:"prompt_cache_key,omitempty"`
	Temperature         float64                        `json:"temperature"`
	MaxCompletionTokens int64                          `json:"max_completion_tokens,omitempty"`
	ReasoningEffort     string                         `json:"reasoning_effort,omitempty"`
	Thinking            *compatibleThinkingConfigParam `json:"thinking,omitempty"`
	ResponseFormat      map[string]string              `json:"response_format,omitempty"`
	Stream              bool                           `json:"stream,omitempty"`
	StreamOptions       *chatCompletionStreamOptions   `json:"stream_options,omitempty"`
}

type anthropicMessagesRequest struct {
	Model       string                         `json:"model"`
	Messages    []anthropicMessageParam        `json:"messages,omitempty"`
	System      []anthropicTextBlockParam      `json:"system,omitempty"`
	MaxTokens   int64                          `json:"max_tokens"`
	Temperature float64                        `json:"temperature,omitempty"`
	Stream      bool                           `json:"stream,omitempty"`
	Thinking    *compatibleThinkingConfigParam `json:"thinking,omitempty"`
}

type anthropicMessageParam struct {
	Role    string                       `json:"role"`
	Content []anthropicContentBlockParam `json:"content"`
}

type anthropicContentBlockParam struct {
	Type         string                      `json:"type"`
	Text         string                      `json:"text,omitempty"`
	CacheControl *anthropicCacheControlParam `json:"cache_control,omitempty"`
}

type anthropicTextBlockParam struct {
	Text         string                      `json:"text"`
	Type         string                      `json:"type,omitempty"`
	CacheControl *anthropicCacheControlParam `json:"cache_control,omitempty"`
}

type anthropicCacheControlParam struct {
	Type string `json:"type"`
}

type anthropicMessagesResponse struct {
	Content []anthropicResponseContentBlock `json:"content"`
	Usage   anthropicUsagePayload           `json:"usage"`
	Error   *chatCompletionError            `json:"error,omitempty"`
}

type anthropicResponseContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type anthropicUsagePayload struct {
	InputTokens              int `json:"input_tokens,omitempty"`
	OutputTokens             int `json:"output_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	PromptCacheHitTokens     int `json:"prompt_cache_hit_tokens,omitempty"`
	PromptCacheMissTokens    int `json:"prompt_cache_miss_tokens,omitempty"`
	CacheableInputTokens     int `json:"cacheable_input_tokens,omitempty"`
	NonCacheableInputTokens  int `json:"non_cacheable_input_tokens,omitempty"`
}

type anthropicStreamEvent struct {
	Type  string                `json:"type"`
	Delta anthropicStreamDelta  `json:"delta,omitempty"`
	Usage anthropicUsagePayload `json:"usage,omitempty"`
	Error *chatCompletionError  `json:"error,omitempty"`
}

type anthropicStreamDelta struct {
	Type string `json:"type,omitempty"`
	Text string `json:"text,omitempty"`
}

func (p *HTTPProvider) completeAnthropicCompatible(ctx context.Context, request llmmodels.ProviderRequest) (llmmodels.ProviderCompletionResult, error) {
	if err := p.ensureAnthropicCompatibleHTTPSurface(); err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	endpoint, err := anthropicMessagesURL(p.config.BaseURL)
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	payload, err := json.Marshal(p.anthropicMessagesRequest(request, false))
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	p.setAnthropicHeaders(httpReq)
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxProviderErrorBodyBytes))
		return llmmodels.ProviderCompletionResult{}, providerHTTPStatusError(resp.StatusCode, body, p.config.APIKey)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	var decoded anthropicMessagesResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return llmmodels.ProviderCompletionResult{}, fmt.Errorf("%w: llm provider protocol invalid: invalid response json: %v", common.ErrInvalidArgument, err)
	}
	if decoded.Error != nil {
		return llmmodels.ProviderCompletionResult{}, fmt.Errorf("%w: llm provider protocol invalid: %s", common.ErrInvalidArgument, decoded.Error.Summary())
	}
	content := strings.TrimSpace(firstAnthropicHTTPText(decoded.Content))
	if content == "" {
		return llmmodels.ProviderCompletionResult{}, fmt.Errorf("%w: llm provider response schema invalid: content text is required", common.ErrInvalidArgument)
	}
	return llmmodels.ProviderCompletionResult{
		Payload: []byte(content),
		Usage:   decoded.Usage.ProviderUsage(),
	}, nil
}

func (p *HTTPProvider) streamAnthropicCompatible(ctx context.Context, request llmmodels.ProviderRequest, emit func(llmmodels.ProviderStreamEvent) error) (llmmodels.ProviderCompletionResult, error) {
	if err := p.ensureAnthropicCompatibleHTTPSurface(); err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	endpoint, err := anthropicMessagesURL(p.config.BaseURL)
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	payload, err := json.Marshal(p.anthropicMessagesRequest(request, true))
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	p.setAnthropicHeaders(httpReq)
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxProviderErrorBodyBytes))
		return llmmodels.ProviderCompletionResult{}, providerHTTPStatusError(resp.StatusCode, body, p.config.APIKey)
	}
	content, usage, err := readAnthropicHTTPStream(resp.Body, emit)
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return llmmodels.ProviderCompletionResult{}, fmt.Errorf("%w: llm provider streamed response content is empty", common.ErrInvalidArgument)
	}
	return llmmodels.ProviderCompletionResult{
		Payload: []byte(content),
		Usage:   usage,
	}, nil
}

func (p *HTTPProvider) ensureOpenAICompatibleHTTPSurface() error {
	surface := strings.TrimSpace(p.config.APISurface)
	if surface == "" || surface == llmcommon.ProviderSurfaceOpenAICompatibleHTTP {
		return nil
	}
	return fmt.Errorf("%w: llm provider apiSurface %s is not implemented by HTTPProvider", common.ErrInvalidArgument, surface)
}

func (p *HTTPProvider) ensureAnthropicCompatibleHTTPSurface() error {
	surface := strings.TrimSpace(p.config.APISurface)
	if surface == llmcommon.ProviderSurfaceAnthropicCompatibleHTTP {
		return nil
	}
	return fmt.Errorf("%w: llm provider apiSurface %s is not implemented by HTTPProvider", common.ErrInvalidArgument, surface)
}

func (p *HTTPProvider) surface() string {
	return strings.TrimSpace(p.config.APISurface)
}

func (p *HTTPProvider) surfaceOrDefault() string {
	if surface := p.surface(); surface != "" {
		return surface
	}
	return defaultProviderAPISurface(p.config.Provider)
}

func promptCacheKeyForRequest(config llmmodels.ProviderConfig, request llmmodels.ProviderRequest) string {
	if strings.TrimSpace(config.APISurface) != llmcommon.ProviderSurfaceOpenAICompatibleHTTP {
		return ""
	}
	if !config.CacheCapability.PromptCacheKey {
		return ""
	}
	switch strings.TrimSpace(config.CacheCapability.Mode) {
	case llmcommon.ProviderCacheModePromptCacheKey, llmcommon.ProviderCacheModeCompatibleDeclared:
	default:
		return ""
	}
	if request.ContextBudget == nil || request.ContextBudget.CachePlan == nil {
		return ""
	}
	return strings.TrimSpace(request.ContextBudget.CachePlan.CacheKey)
}

func (p *HTTPProvider) setAnthropicHeaders(httpReq *http.Request) {
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	httpReq.Header.Set("x-api-key", p.config.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
}

func (p *HTTPProvider) anthropicMessagesRequest(request llmmodels.ProviderRequest, stream bool) anthropicMessagesRequest {
	prompt := buildProviderFacingPrompt(request)
	req := anthropicMessagesRequest{
		Model:       p.config.Model,
		MaxTokens:   anthropicMaxTokens(p.config),
		Temperature: providerTemperature(p.config),
		Stream:      stream,
	}
	if p.config.InferenceProfile.ThinkingBudgetTokens > 0 {
		req.Thinking = compatibleThinkingConfig(p.config)
	}
	if stablePrefix := strings.TrimSpace(prompt.StablePrefix); stablePrefix != "" {
		block := anthropicTextBlockParam{Type: "text", Text: stablePrefix}
		if cacheControlSentForRequest(p.config, prompt) {
			block.CacheControl = &anthropicCacheControlParam{Type: "ephemeral"}
		}
		req.System = []anthropicTextBlockParam{block}
	}
	for _, item := range prompt.DynamicMessages {
		content := strings.TrimSpace(item.Content)
		if content == "" {
			continue
		}
		role := normalizedPromptRole(item.Role)
		if role == "system" {
			continue
		}
		req.Messages = append(req.Messages, anthropicMessageParam{
			Role: role,
			Content: []anthropicContentBlockParam{{
				Type: "text",
				Text: content,
			}},
		})
	}
	if len(req.Messages) == 0 {
		req.Messages = append(req.Messages, anthropicMessageParam{
			Role: "user",
			Content: []anthropicContentBlockParam{{
				Type: "text",
				Text: userPrompt(request),
			}},
		})
	}
	return req
}

func anthropicMessagesURL(baseURL string) (string, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return "", common.ErrInvalidArgument
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("%w: invalid llm provider base url", common.ErrInvalidArgument)
	}
	trimmedPath := strings.TrimRight(parsed.Path, "/")
	if strings.HasSuffix(trimmedPath, "/messages") {
		parsed.Path = trimmedPath
	} else {
		parsed.Path = trimmedJoin(trimmedPath, "messages")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func firstAnthropicHTTPText(content []anthropicResponseContentBlock) string {
	for _, item := range content {
		if strings.TrimSpace(item.Text) != "" {
			return item.Text
		}
	}
	return ""
}

func readAnthropicHTTPStream(reader io.Reader, emit func(llmmodels.ProviderStreamEvent) error) (string, *llmmodels.ProviderUsage, error) {
	return readAnthropicHTTPStreamWithLimits(reader, emit, defaultProviderStreamLimits)
}

func readAnthropicHTTPStreamWithLimits(reader io.Reader, emit func(llmmodels.ProviderStreamEvent) error, limits providerStreamLimits) (string, *llmmodels.ProviderUsage, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), maxProviderStreamLineBytes)
	var builder strings.Builder
	var usage *llmmodels.ProviderUsage
	guard := newProviderStreamGuard(limits)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			if emit != nil {
				if err := emit(llmmodels.ProviderStreamEvent{Type: "done"}); err != nil {
					return "", nil, err
				}
			}
			return builder.String(), usage, nil
		}
		if err := guard.observeDataEvent(); err != nil {
			return "", nil, err
		}
		var chunk anthropicStreamEvent
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			if providerStreamChunkJSONTruncated(err) {
				return "", nil, fmt.Errorf("%w: llm provider stream incomplete: truncated stream chunk json: %v", common.ErrInvalidArgument, err)
			}
			return "", nil, fmt.Errorf("%w: llm provider protocol invalid: invalid stream chunk json: %v", common.ErrInvalidArgument, err)
		}
		if chunk.Error != nil {
			return "", nil, fmt.Errorf("%w: llm provider protocol invalid: %s", common.ErrInvalidArgument, chunk.Error.Summary())
		}
		if parsed := chunk.Usage.ProviderUsage(); parsed != nil {
			usage = parsed
		}
		switch strings.TrimSpace(chunk.Type) {
		case "content_block_delta":
			if delta := strings.TrimSpace(chunk.Delta.Text); delta != "" {
				if err := guard.observeDelta(delta); err != nil {
					return "", nil, err
				}
				builder.WriteString(delta)
				if emit != nil {
					if err := emit(llmmodels.ProviderStreamEvent{Type: "delta", Delta: delta}); err != nil {
						return "", nil, err
					}
				}
			}
		case "message_delta":
			if parsed := chunk.Usage.ProviderUsage(); parsed != nil {
				usage = parsed
			}
		case "message_stop", "content_block_stop":
		default:
			if delta := strings.TrimSpace(chunk.Delta.Text); delta != "" {
				if err := guard.observeDelta(delta); err != nil {
					return "", nil, err
				}
				builder.WriteString(delta)
				if emit != nil {
					if err := emit(llmmodels.ProviderStreamEvent{Type: "delta", Delta: delta}); err != nil {
						return "", nil, err
					}
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", nil, err
	}
	return "", nil, fmt.Errorf("%w: llm provider stream incomplete: ended before [DONE]", common.ErrInvalidArgument)
}

func (u anthropicUsagePayload) ProviderUsage() *llmmodels.ProviderUsage {
	input := maxPositiveInt(u.InputTokens)
	output := maxPositiveInt(u.OutputTokens)
	cacheReadTokens := maxPositiveInt(u.CacheReadInputTokens, u.PromptCacheHitTokens)
	cacheWriteTokens := maxPositiveInt(u.CacheCreationInputTokens, u.PromptCacheMissTokens)
	if input == 0 && output == 0 && cacheReadTokens == 0 && cacheWriteTokens == 0 {
		return nil
	}
	return &llmmodels.ProviderUsage{
		InputTokens:             input,
		OutputTokens:            output,
		TotalTokens:             input + output,
		CacheReadTokens:         cacheReadTokens,
		CacheWriteTokens:        cacheWriteTokens,
		CacheCreationTokens:     maxPositiveInt(u.CacheCreationInputTokens),
		CacheableInputTokens:    maxPositiveInt(u.CacheableInputTokens),
		NonCacheableInputTokens: maxPositiveInt(u.NonCacheableInputTokens),
	}
}

type compatibleThinkingConfigParam struct {
	Type         string `json:"type"`
	BudgetTokens int64  `json:"budget_tokens"`
}

type chatCompletionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage chatCompletionUsage  `json:"usage"`
	Error *chatCompletionError `json:"error,omitempty"`
}

type chatCompletionStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage chatCompletionUsage  `json:"usage"`
	Error *chatCompletionError `json:"error,omitempty"`
}

type chatCompletionError struct {
	Message string `json:"message,omitempty"`
	Type    string `json:"type,omitempty"`
	Code    any    `json:"code,omitempty"`
}

type chatCompletionUsage struct {
	PromptTokens             int `json:"prompt_tokens,omitempty"`
	CompletionTokens         int `json:"completion_tokens,omitempty"`
	TotalTokens              int `json:"total_tokens,omitempty"`
	InputTokens              int `json:"input_tokens,omitempty"`
	OutputTokens             int `json:"output_tokens,omitempty"`
	PromptCacheHitTokens     int `json:"prompt_cache_hit_tokens,omitempty"`
	PromptCacheMissTokens    int `json:"prompt_cache_miss_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CachedTokens             int `json:"cached_tokens,omitempty"`
	InputCachedTokens        int `json:"input_cached_tokens,omitempty"`
	InputCacheReadTokens     int `json:"input_cache_read_tokens,omitempty"`
	InputCacheWriteTokens    int `json:"input_cache_write_tokens,omitempty"`
	CacheableInputTokens     int `json:"cacheable_input_tokens,omitempty"`
	NonCacheableInputTokens  int `json:"non_cacheable_input_tokens,omitempty"`
}

type chatCompletionStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type embeddingRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type embeddingResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
}

func (r embeddingResponse) FirstEmbedding() []float64 {
	if len(r.Data) == 0 || len(r.Data[0].Embedding) == 0 {
		return nil
	}
	return append([]float64(nil), r.Data[0].Embedding...)
}

func (r chatCompletionResponse) FirstContent() string {
	if len(r.Choices) == 0 {
		return ""
	}
	return r.Choices[0].Message.Content
}

func embeddingsURL(baseURL string) (string, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return "", common.ErrInvalidArgument
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("%w: invalid llm provider base url", common.ErrInvalidArgument)
	}
	trimmedPath := ensureOpenAICompatibleAPIRoot(strings.TrimRight(parsed.Path, "/"))
	if strings.HasSuffix(trimmedPath, "/embeddings") {
		parsed.Path = trimmedPath
	} else {
		parsed.Path = trimmedJoin(trimmedPath, "embeddings")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func chatCompletionsURL(baseURL string) (string, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return "", common.ErrInvalidArgument
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("%w: invalid llm provider base url", common.ErrInvalidArgument)
	}
	trimmedPath := ensureOpenAICompatibleAPIRoot(strings.TrimRight(parsed.Path, "/"))
	if strings.HasSuffix(trimmedPath, "/chat/completions") {
		parsed.Path = trimmedPath
	} else {
		parsed.Path = trimmedJoin(trimmedPath, "chat/completions")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

// ensureOpenAICompatibleAPIRoot normalizes an OpenAI-compatible base path so that
// both host-only bases (http://proxy:8080) and /v1 bases produce the same API root.
// Full endpoint paths (/chat/completions, /embeddings) are left unchanged.
func ensureOpenAICompatibleAPIRoot(path string) string {
	path = strings.TrimRight(strings.TrimSpace(path), "/")
	if path == "" {
		return "/v1"
	}
	if strings.HasSuffix(path, "/chat/completions") ||
		strings.HasSuffix(path, "/embeddings") ||
		strings.HasSuffix(path, "/models") ||
		strings.HasSuffix(path, "/messages") {
		return path
	}
	if path == "/v1" || strings.HasSuffix(path, "/v1") {
		return path
	}
	return path + "/v1"
}

func readChatCompletionStream(reader io.Reader, emit func(llmmodels.ProviderStreamEvent) error) (string, *llmmodels.ProviderUsage, error) {
	return readChatCompletionStreamWithLimits(reader, emit, defaultProviderStreamLimits)
}

func readChatCompletionStreamWithLimits(reader io.Reader, emit func(llmmodels.ProviderStreamEvent) error, limits providerStreamLimits) (string, *llmmodels.ProviderUsage, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), maxProviderStreamLineBytes)
	var builder strings.Builder
	var usage *llmmodels.ProviderUsage
	guard := newProviderStreamGuard(limits)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			if emit != nil {
				if err := emit(llmmodels.ProviderStreamEvent{Type: "done"}); err != nil {
					return "", nil, err
				}
			}
			return builder.String(), usage, nil
		}
		if err := guard.observeDataEvent(); err != nil {
			return "", nil, err
		}
		var chunk chatCompletionStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			if providerStreamChunkJSONTruncated(err) {
				return "", nil, fmt.Errorf("%w: llm provider stream incomplete: truncated stream chunk json: %v", common.ErrInvalidArgument, err)
			}
			return "", nil, fmt.Errorf("%w: llm provider protocol invalid: invalid stream chunk json: %v", common.ErrInvalidArgument, err)
		}
		if chunk.Error != nil {
			return "", nil, fmt.Errorf("%w: llm provider protocol invalid: %s", common.ErrInvalidArgument, chunk.Error.Summary())
		}
		if parsed := chunk.Usage.ProviderUsage(); parsed != nil {
			usage = parsed
		}
		for _, choice := range chunk.Choices {
			delta := choice.Delta.Content
			if delta == "" {
				delta = choice.Message.Content
			}
			if delta == "" {
				continue
			}
			if err := guard.observeDelta(delta); err != nil {
				return "", nil, err
			}
			builder.WriteString(delta)
			if emit != nil {
				if err := emit(llmmodels.ProviderStreamEvent{Type: "delta", Delta: delta}); err != nil {
					return "", nil, err
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", nil, err
	}
	return "", nil, fmt.Errorf("%w: llm provider stream incomplete: ended before [DONE]", common.ErrInvalidArgument)
}

func providerStreamChunkJSONTruncated(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(text, "unexpected end of json input") ||
		strings.Contains(text, "unexpected eof")
}

func (e *chatCompletionError) Summary() string {
	if e == nil {
		return "provider returned error object"
	}
	parts := []string{}
	if value := strings.TrimSpace(e.Type); value != "" {
		parts = append(parts, "type="+value)
	}
	if value := strings.TrimSpace(fmt.Sprint(e.Code)); value != "" && value != "<nil>" {
		parts = append(parts, "code="+value)
	}
	if value := strings.TrimSpace(e.Message); value != "" {
		parts = append(parts, value)
	}
	if len(parts) == 0 {
		return "provider returned error object"
	}
	return strings.Join(parts, ": ")
}

func (u chatCompletionUsage) ProviderUsage() *llmmodels.ProviderUsage {
	input := max(u.PromptTokens, u.InputTokens)
	output := max(u.CompletionTokens, u.OutputTokens)
	total := u.TotalTokens
	if total == 0 && (input > 0 || output > 0) {
		total = input + output
	}
	cacheReadTokens := maxPositiveInt(u.PromptCacheHitTokens, u.CacheReadInputTokens, u.CachedTokens, u.InputCachedTokens, u.InputCacheReadTokens)
	cacheWriteTokens := maxPositiveInt(u.PromptCacheMissTokens, u.CacheCreationInputTokens, u.InputCacheWriteTokens)
	if input == 0 && output == 0 && total == 0 && cacheReadTokens == 0 && cacheWriteTokens == 0 {
		return nil
	}
	return &llmmodels.ProviderUsage{
		InputTokens:             input,
		OutputTokens:            output,
		TotalTokens:             total,
		CacheReadTokens:         cacheReadTokens,
		CacheWriteTokens:        cacheWriteTokens,
		CacheCreationTokens:     max(0, u.CacheCreationInputTokens),
		CacheableInputTokens:    max(0, u.CacheableInputTokens),
		NonCacheableInputTokens: max(0, u.NonCacheableInputTokens),
	}
}

func maxPositiveInt(values ...int) int {
	ret := 0
	for _, value := range values {
		if value > ret {
			ret = value
		}
	}
	return ret
}

func trimmedJoin(basePath string, child string) string {
	if basePath == "" {
		return "/" + child
	}
	return basePath + "/" + child
}

func chatMessagesForRequest(request *llmmodels.ProviderRequest) []chatCompletionMessage {
	if request == nil {
		system, user := providerFallbackPrompts(llmmodels.ProviderRequest{})
		return []chatCompletionMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		}
	}
	prompt := buildProviderFacingPrompt(*request)
	messages := make([]chatCompletionMessage, 0, len(prompt.DynamicMessages)+1)
	if stablePrefix := strings.TrimSpace(prompt.StablePrefix); stablePrefix != "" {
		messages = append(messages, chatCompletionMessage{
			Role:    "system",
			Content: stablePrefix,
		})
	}
	for _, item := range prompt.DynamicMessages {
		content := strings.TrimSpace(item.Content)
		if content == "" {
			continue
		}
		messages = append(messages, chatCompletionMessage{
			Role:    normalizedPromptRole(item.Role),
			Content: content,
		})
	}
	if len(messages) == 0 {
		messages = []chatCompletionMessage{
			{Role: "system", Content: systemPromptForRequest(*request)},
			{Role: "user", Content: userPrompt(*request)},
		}
	}
	if request.ContextBudget != nil {
		request.ContextBudget.PromptCaptured = true
		request.ContextBudget.NormalizedSections = uniqueStrings(append(request.ContextBudget.NormalizedSections, prompt.NormalizedSections...))
		request.ContextBudget.IncludedSections = uniqueStrings(append(request.ContextBudget.IncludedSections, providerFacingPromptIncludedSections(prompt)...))
	}
	return messages
}

func systemPromptForRequest(request llmmodels.ProviderRequest) string {
	system, _ := providerFallbackPrompts(request)
	return system
}

func userPrompt(request llmmodels.ProviderRequest) string {
	_, user := providerFallbackPrompts(request)
	return user
}

func sanitizeProviderBody(body []byte, secret string) string {
	text := strings.TrimSpace(string(body))
	if text == "" {
		return http.StatusText(http.StatusBadGateway)
	}
	if secret = strings.TrimSpace(secret); secret != "" {
		text = strings.ReplaceAll(text, secret, RedactSecret(secret))
		// Also redact common Authorization bearer forms if the secret is embedded.
		text = strings.ReplaceAll(text, "Bearer "+secret, "Bearer "+RedactSecret(secret))
		text = strings.ReplaceAll(text, "bearer "+secret, "bearer "+RedactSecret(secret))
	}
	// Strip control characters while preserving readable spaces.
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
	if len(text) > maxProviderErrorBodyBytes {
		return text[:maxProviderErrorBodyBytes]
	}
	return text
}

// sanitizeCatalogLogValue bounds model IDs / route owners for logs and strips control characters.
func sanitizeCatalogLogValue(value string, maxLen int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	cleaned := make([]rune, 0, len(value))
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			continue
		}
		cleaned = append(cleaned, r)
	}
	value = string(cleaned)
	if maxLen <= 0 {
		maxLen = 128
	}
	if len(value) > maxLen {
		return value[:maxLen]
	}
	return value
}
