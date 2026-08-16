package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/llmaudit"
)

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
// boundaries. Upstream response bodies are intentionally not retained because
// they are untrusted and may contain credentials or provider-private context.
type ProviderHTTPStatusError struct {
	StatusCode int
	Body       string
}

// ProviderOutputLimitError is a typed, non-transport completion outcome. The
// provider has returned a valid prefix and explicitly stopped because the
// output token budget was reached; callers must retain Payload and continue
// the same logical request rather than retrying the full request.
type ProviderOutputLimitError struct {
	Payload []byte
	Usage   *llmaudit.ProviderUsage
}

func (e *ProviderOutputLimitError) Error() string {
	return "llm provider response incomplete: max_output_tokens"
}

func (e *ProviderHTTPStatusError) Error() string {
	if e == nil {
		return "llm provider http status error"
	}
	return fmt.Sprintf("llm provider http status %d: %s", e.StatusCode, e.Body)
}

func (e *ProviderHTTPStatusError) Unwrap() error { return common.ErrInvalidArgument }

func providerHTTPStatusError(statusCode int) error {
	body := http.StatusText(statusCode)
	if body == "" {
		body = "upstream error"
	}
	return &ProviderHTTPStatusError{StatusCode: statusCode, Body: body}
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
	return &HTTPProvider{
		config: config,
		// The provider deadline belongs to the request context (see
		// requestContext). A client-wide timeout races it and can return while
		// leaving the server-side request context active.
		client: &http.Client{},
	}
}

func NewHTTPProviderWithClient(config llmmodels.ProviderConfig, client *http.Client) *HTTPProvider {
	if client == nil {
		return NewHTTPProvider(config)
	}
	config = normalizedProviderConfigForRuntime(config)
	// Do not mutate the caller's client. The shallow copy preserves its
	// transport and protocol behavior while moving timeout authority to the
	// request context consistently for both constructed and injected clients.
	clientCopy := *client
	clientCopy.Timeout = 0
	return &HTTPProvider{config: config, client: &clientCopy}
}

// requestContext makes the configured provider deadline part of the actual
// request contract. http.Client.Timeout alone can return to the caller while
// the request context remains live on the server, which leaves a stale active
// connection during a provider timeout. The derived context is also used by
// streaming and embedding requests.
func (p *HTTPProvider) requestContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := p.config.Timeout
	if timeout <= 0 {
		timeout = defaultProviderTimeout
	}
	return context.WithTimeout(ctx, timeout)
}

func providerRequestError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("llm provider request timeout: %w", err)
	}
	return err
}

func (p *HTTPProvider) Complete(ctx context.Context, request llmmodels.ProviderRequest) (llmmodels.ProviderCompletionResult, error) {
	if p == nil || p.client == nil {
		return llmmodels.ProviderCompletionResult{}, common.ErrInvalidArgument
	}
	return p.completeResponses(ctx, request)
}

func (p *HTTPProvider) completeResponses(ctx context.Context, request llmmodels.ProviderRequest) (llmmodels.ProviderCompletionResult, error) {
	endpoint, err := responsesURL(p.config.BaseURL)
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	requestPayload := responsesRequest{
		Model:           p.config.Model,
		Input:           responsesInputForRequest(&request),
		Temperature:     providerTemperature(p.config),
		MaxOutputTokens: maxOutputTokensForRequest(p.config, request),
		Reasoning:       responsesReasoningForRequest(request),
		Text:            responsesTextForRequest(p.config.Capabilities, request),
	}
	payload, err := json.Marshal(requestPayload)
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	requestCtx, cancel := p.requestContext(ctx)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	setBearerAuthorization(httpReq.Header, p.config.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, providerRequestError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return llmmodels.ProviderCompletionResult{}, providerHTTPStatusError(resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	var decoded responsesResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return llmmodels.ProviderCompletionResult{}, fmt.Errorf("%w: llm provider protocol invalid: invalid response json: %v", common.ErrInvalidArgument, err)
	}
	if decoded.Error != nil {
		return llmmodels.ProviderCompletionResult{}, fmt.Errorf("%w: llm provider protocol invalid: %s", common.ErrInvalidArgument, decoded.Error.Summary())
	}
	content := providerCompletionContent(request, decoded.Content())
	if decoded.Status == "incomplete" && decoded.IncompleteDetails.Reason == "max_output_tokens" {
		return llmmodels.ProviderCompletionResult{Payload: []byte(content), Usage: decoded.Usage.ProviderUsage()}, &ProviderOutputLimitError{Payload: []byte(content), Usage: decoded.Usage.ProviderUsage()}
	}
	if strings.TrimSpace(content) == "" {
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
	return p.streamResponses(ctx, request, emit)
}

func (p *HTTPProvider) streamResponses(ctx context.Context, request llmmodels.ProviderRequest, emit func(llmmodels.ProviderStreamEvent) error) (llmmodels.ProviderCompletionResult, error) {
	endpoint, err := responsesURL(p.config.BaseURL)
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	requestPayload := responsesRequest{
		Model:           p.config.Model,
		Input:           responsesInputForRequest(&request),
		Temperature:     providerTemperature(p.config),
		MaxOutputTokens: maxOutputTokensForRequest(p.config, request),
		Reasoning:       responsesReasoningForRequest(request),
		Text:            responsesTextForRequest(p.config.Capabilities, request),
		Stream:          true,
	}
	payload, err := json.Marshal(requestPayload)
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	requestCtx, cancel := p.requestContext(ctx)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	setBearerAuthorization(httpReq.Header, p.config.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, providerRequestError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return llmmodels.ProviderCompletionResult{}, providerHTTPStatusError(resp.StatusCode)
	}
	content, usage, err := readResponsesStream(resp.Body, emit)
	if err != nil {
		content = providerCompletionContent(request, content)
		return llmmodels.ProviderCompletionResult{Payload: []byte(content), Usage: usage}, err
	}
	content = providerCompletionContent(request, content)
	if strings.TrimSpace(content) == "" {
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
	requestCtx, cancel := p.requestContext(ctx)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return llmmodels.ProviderEmbeddingResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	setBearerAuthorization(httpReq.Header, p.config.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return llmmodels.ProviderEmbeddingResult{}, providerRequestError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return llmmodels.ProviderEmbeddingResult{}, providerHTTPStatusError(resp.StatusCode)
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

// responsesRequest is the OpenAI Responses wire contract used by ai-proxy.
// It is deliberately kept local to the protocol adapter; runtime contracts do
// not carry provider-specific JSON.
type responsesRequest struct {
	Model           string               `json:"model"`
	Input           string               `json:"input"`
	Temperature     float64              `json:"temperature,omitempty"`
	MaxOutputTokens int64                `json:"max_output_tokens,omitempty"`
	Reasoning       *responsesReasoning  `json:"reasoning,omitempty"`
	Text            *responsesTextConfig `json:"text,omitempty"`
	Stream          bool                 `json:"stream,omitempty"`
}

type responsesReasoning struct {
	Effort string `json:"effort"`
}

func responsesReasoningForRequest(request llmmodels.ProviderRequest) *responsesReasoning {
	effort := strings.TrimSpace(request.ReasoningEffort)
	if effort == "" {
		return nil
	}
	return &responsesReasoning{Effort: effort}
}

type responsesTextConfig struct {
	Format *responsesTextFormat `json:"format,omitempty"`
}

type responsesTextFormat struct {
	Type   string `json:"type"`
	Name   string `json:"name,omitempty"`
	Schema any    `json:"schema,omitempty"`
	Strict bool   `json:"strict,omitempty"`
}

type responsesResponse struct {
	Status            string                     `json:"status,omitempty"`
	IncompleteDetails responsesIncompleteDetails `json:"incomplete_details,omitempty"`
	Output            []responsesOutputItem      `json:"output"`
	OutputText        string                     `json:"output_text"`
	Usage             responsesUsage             `json:"usage"`
	Error             *providerResponseError     `json:"error,omitempty"`
}

type responsesIncompleteDetails struct {
	Reason string `json:"reason,omitempty"`
}

type responsesOutputItem struct {
	Type    string                 `json:"type"`
	Content []responsesContentItem `json:"content"`
}

type responsesContentItem struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type responsesUsage struct {
	InputTokens  int `json:"input_tokens,omitempty"`
	OutputTokens int `json:"output_tokens,omitempty"`
	TotalTokens  int `json:"total_tokens,omitempty"`
}

type providerResponseError struct {
	Message string `json:"message,omitempty"`
	Type    string `json:"type,omitempty"`
	Code    any    `json:"code,omitempty"`
}

func (r responsesResponse) Content() string {
	if strings.TrimSpace(r.OutputText) != "" {
		return r.OutputText
	}
	var b strings.Builder
	for _, item := range r.Output {
		for _, content := range item.Content {
			if content.Type == "output_text" && strings.TrimSpace(content.Text) != "" {
				b.WriteString(content.Text)
			}
		}
	}
	return b.String()
}

func (u responsesUsage) ProviderUsage() *llmaudit.ProviderUsage {
	if u.InputTokens == 0 && u.OutputTokens == 0 && u.TotalTokens == 0 {
		return nil
	}
	if u.TotalTokens == 0 {
		u.TotalTokens = u.InputTokens + u.OutputTokens
	}
	return &llmaudit.ProviderUsage{InputTokens: u.InputTokens, OutputTokens: u.OutputTokens, TotalTokens: u.TotalTokens}
}

func responsesInputForRequest(request *llmmodels.ProviderRequest) string {
	if request == nil {
		return ""
	}
	prompt := buildProviderFacingPrompt(*request)
	parts := make([]string, 0, len(prompt.DynamicMessages)+1)
	if stable := strings.TrimSpace(prompt.StablePrefix); stable != "" {
		parts = append(parts, stable)
	}
	for _, item := range prompt.DynamicMessages {
		if content := strings.TrimSpace(item.Content); content != "" {
			parts = append(parts, "["+normalizedPromptRole(item.Role)+"]\n"+content)
		}
	}
	if len(parts) == 0 {
		system, user := providerFallbackPrompts(*request)
		parts = append(parts, "[system]\n"+system, "[user]\n"+user)
	}
	if request.ContextBudget != nil {
		request.ContextBudget.PromptCaptured = true
		request.ContextBudget.NormalizedSections = uniqueStrings(append(request.ContextBudget.NormalizedSections, prompt.NormalizedSections...))
		request.ContextBudget.IncludedSections = uniqueStrings(append(request.ContextBudget.IncludedSections, providerFacingPromptIncludedSections(prompt)...))
	}
	return strings.Join(parts, "\n\n")
}

func responsesTextForRequest(capabilities llmmodels.ProviderCapabilities, request llmmodels.ProviderRequest) *responsesTextConfig {
	if request.RawTextOutput || !capabilities.JSONSchemaResponse || strings.TrimSpace(request.Schema) == "" {
		return nil
	}
	var schema any
	if err := json.Unmarshal([]byte(request.Schema), &schema); err != nil {
		return nil
	}
	return &responsesTextConfig{Format: &responsesTextFormat{Type: "json_schema", Name: "workorch_runtime_candidate", Schema: schema, Strict: true}}
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

func responsesURL(baseURL string) (string, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return "", common.ErrInvalidArgument
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("%w: invalid llm provider base url", common.ErrInvalidArgument)
	}
	trimmedPath := ensureOpenAICompatibleAPIRoot(strings.TrimRight(parsed.Path, "/"))
	if strings.HasSuffix(trimmedPath, "/responses") {
		parsed.Path = trimmedPath
	} else {
		parsed.Path = trimmedJoin(trimmedPath, "responses")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

// ensureOpenAICompatibleAPIRoot normalizes an OpenAI-compatible base path so that
// both host-only bases (http://proxy:8080) and /v1 bases produce the same API root.
// Full Responses, embedding, and catalog paths are left unchanged.
func ensureOpenAICompatibleAPIRoot(path string) string {
	path = strings.TrimRight(strings.TrimSpace(path), "/")
	if path == "" {
		return "/v1"
	}
	if strings.HasSuffix(path, "/responses") ||
		strings.HasSuffix(path, "/embeddings") ||
		strings.HasSuffix(path, "/models") {
		return path
	}
	if path == "/v1" || strings.HasSuffix(path, "/v1") {
		return path
	}
	return path + "/v1"
}

type responsesStreamEvent struct {
	Type              string                     `json:"type"`
	Delta             string                     `json:"delta,omitempty"`
	Text              string                     `json:"text,omitempty"`
	Response          *responsesResponse         `json:"response,omitempty"`
	IncompleteDetails responsesIncompleteDetails `json:"incomplete_details,omitempty"`
	Error             *providerResponseError     `json:"error,omitempty"`
}

type responsesStreamReader struct {
	builder   strings.Builder
	usage     *llmaudit.ProviderUsage
	guard     *providerStreamGuard
	emit      func(llmmodels.ProviderStreamEvent) error
	completed bool
}

func readResponsesStream(reader io.Reader, emit func(llmmodels.ProviderStreamEvent) error) (string, *llmaudit.ProviderUsage, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), maxProviderStreamLineBytes)
	stream := &responsesStreamReader{guard: newProviderStreamGuard(defaultProviderStreamLimits), emit: emit}
	for scanner.Scan() {
		data, ok := responsesStreamData(scanner.Text())
		if !ok {
			continue
		}
		stop, preservePartial, err := stream.consume(data)
		if err != nil {
			if preservePartial {
				return stream.builder.String(), stream.usage, err
			}
			return "", nil, err
		}
		if stop {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return "", nil, err
	}
	if !stream.completed {
		return "", nil, fmt.Errorf("%w: llm provider stream incomplete: ended before response.completed", common.ErrInvalidArgument)
	}
	return stream.builder.String(), stream.usage, nil
}

func responsesStreamData(line string) (string, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, ":") || !strings.HasPrefix(line, "data:") {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(line, "data:")), true
}

func (s *responsesStreamReader) consume(data string) (stop bool, preservePartial bool, err error) {
	if data == "[DONE]" {
		return true, false, s.complete()
	}
	if err := s.guard.observeDataEvent(); err != nil {
		return false, false, err
	}
	event, err := decodeResponsesStreamEvent(data)
	if err != nil {
		return false, false, err
	}
	if event.Response != nil {
		if usage := event.Response.Usage.ProviderUsage(); usage != nil {
			s.usage = usage
		}
	}
	switch event.Type {
	case "response.output_text.delta":
		return false, false, s.appendOutput(event.Delta, event.Delta != "")
	case "response.output_text.done":
		return false, false, s.appendOutput(event.Text, event.Text != "" && s.builder.Len() == 0)
	case "response.completed":
		return false, false, s.consumeCompleted(event.Response)
	case "response.incomplete":
		return false, true, s.incompleteError(event)
	default:
		return false, false, nil
	}
}

func decodeResponsesStreamEvent(data string) (responsesStreamEvent, error) {
	var event responsesStreamEvent
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		return responsesStreamEvent{}, fmt.Errorf("%w: invalid responses stream event: %v", common.ErrInvalidArgument, err)
	}
	if event.Error != nil {
		return responsesStreamEvent{}, fmt.Errorf("%w: llm provider protocol invalid: %s", common.ErrInvalidArgument, event.Error.Summary())
	}
	return event, nil
}

// appendOutput accepts only provider-declared final output. Reasoning and
// summaries are neither document content nor continuation prefixes.
func (s *responsesStreamReader) appendOutput(content string, accepted bool) error {
	if !accepted {
		return nil
	}
	if err := s.guard.observeDelta(content); err != nil {
		return err
	}
	s.builder.WriteString(content)
	if s.emit == nil {
		return nil
	}
	return s.emit(llmmodels.ProviderStreamEvent{Type: "delta", Delta: content})
}

func (s *responsesStreamReader) consumeCompleted(response *responsesResponse) error {
	if s.builder.Len() == 0 && response != nil {
		if content := response.Content(); content != "" {
			if err := s.appendOutput(content, true); err != nil {
				return err
			}
		}
	}
	return s.complete()
}

func (s *responsesStreamReader) complete() error {
	s.completed = true
	if s.emit == nil {
		return nil
	}
	return s.emit(llmmodels.ProviderStreamEvent{Type: "done"})
}

func (s *responsesStreamReader) incompleteError(event responsesStreamEvent) error {
	reason := event.IncompleteDetails.Reason
	if reason == "" && event.Response != nil {
		reason = event.Response.IncompleteDetails.Reason
	}
	if reason == "max_output_tokens" {
		return &ProviderOutputLimitError{Payload: []byte(s.builder.String()), Usage: s.usage}
	}
	return fmt.Errorf("%w: llm provider stream incomplete: %s", common.ErrInvalidArgument, reason)
}

func (e *providerResponseError) Summary() string {
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

func trimmedJoin(basePath string, child string) string {
	if basePath == "" {
		return "/" + child
	}
	return basePath + "/" + child
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
