package llm

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/llmaudit"
)

type providerStreamLimits struct {
	MaxContentBytes int
	MaxDataEvents   int
	MaxLineBytes    int
}

const (
	providerResponseMediaTypeJSON        = "application/json"
	providerResponseMediaTypeEventStream = "text/event-stream"
)

// ProviderHTTPStatusError preserves the provider status code across adapter
// boundaries. Upstream response bodies are intentionally not retained because
// they are untrusted and may contain credentials or provider-private context.
type ProviderHTTPStatusError struct {
	StatusCode int
	Body       string
}

// ProviderProtocolError marks a response/framing/security defect after the
// provider request was submitted. It carries only a stable code and byte
// count; raw provider content remains private to this adapter.
type ProviderProtocolError struct {
	Code          string
	ResponseBytes int
}

// ProviderTransportError is the audit-safe boundary for errors returned by
// http.Client.Do. Standard url.Error values include the requested endpoint,
// while custom transports may include credentials or request fragments. Only
// a stable category crosses the provider adapter boundary.
type ProviderTransportError struct {
	Code  string
	cause error
}

func (e *ProviderTransportError) Error() string {
	if e == nil || strings.TrimSpace(e.Code) == "" {
		return "llm provider transport error"
	}
	return "llm provider transport error: " + strings.TrimSpace(e.Code)
}

func (e *ProviderTransportError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// RuntimeProviderFailureIdentity exposes only the adapter-owned, audit-safe
// factual identity.  It deliberately omits the wrapped transport cause, which
// may contain an endpoint, credential header, or provider-private fragment.
func (e *ProviderTransportError) RuntimeProviderFailureIdentity() (string, string) {
	if e == nil {
		return "", ""
	}
	code := strings.TrimSpace(e.Code)
	return code, strings.ReplaceAll(code, "_", "-")
}

func (e *ProviderProtocolError) Error() string {
	if e == nil || strings.TrimSpace(e.Code) == "" {
		return "llm provider protocol error"
	}
	return "llm provider protocol error: " + strings.TrimSpace(e.Code)
}

func (e *ProviderProtocolError) Unwrap() error { return common.ErrInvalidArgument }

// RuntimeProviderFailureIdentity preserves the exact protocol guard that
// rejected the response while keeping raw provider content inside this owner.
func (e *ProviderProtocolError) RuntimeProviderFailureIdentity() (string, string) {
	if e == nil {
		return "", ""
	}
	code := strings.TrimSpace(e.Code)
	return code, strings.ReplaceAll(code, "_", "-")
}

func providerProtocolError(code string, responseBytes int) error {
	return &ProviderProtocolError{Code: strings.TrimSpace(code), ResponseBytes: max(responseBytes, 0)}
}

func providerResponseBytesFromError(err error) int {
	var protocolErr *ProviderProtocolError
	if errors.As(err, &protocolErr) && protocolErr != nil {
		return protocolErr.ResponseBytes
	}
	return 0
}

func withProviderProtocolResponseBytes(err error, responseBytes int) error {
	var protocolErr *ProviderProtocolError
	if !errors.As(err, &protocolErr) || protocolErr == nil {
		return err
	}
	return &ProviderProtocolError{Code: protocolErr.Code, ResponseBytes: max(protocolErr.ResponseBytes, responseBytes)}
}

type providerResponseLimitReader struct {
	reader    io.Reader
	remaining int
	read      int
}

func newProviderResponseLimitReader(reader io.Reader, limit int) *providerResponseLimitReader {
	return &providerResponseLimitReader{reader: reader, remaining: max(limit, 0)}
}

func (r *providerResponseLimitReader) Read(buffer []byte) (int, error) {
	if r == nil || r.reader == nil {
		return 0, io.EOF
	}
	if r.remaining <= 0 {
		var probe [1]byte
		n, err := r.reader.Read(probe[:])
		r.read += n
		if n > 0 {
			return 0, providerProtocolError("response_byte_limit_exceeded", r.read)
		}
		return 0, err
	}
	if len(buffer) > r.remaining {
		buffer = buffer[:r.remaining]
	}
	n, err := r.reader.Read(buffer)
	r.read += n
	r.remaining -= n
	return n, err
}

func (r *providerResponseLimitReader) BytesRead() int {
	if r == nil {
		return 0
	}
	return r.read
}

func readBoundedProviderBody(reader io.Reader, limit int) ([]byte, int, error) {
	limited := newProviderResponseLimitReader(reader, limit)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, limited.BytesRead(), withProviderProtocolResponseBytes(err, limited.BytesRead())
	}
	return body, limited.BytesRead(), nil
}

func validateProviderResponseMediaType(header http.Header, expected string) error {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(header.Get("Content-Type")))
	if err != nil {
		return providerProtocolError("response_media_type_mismatch", 0)
	}
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	matched := mediaType == expected
	if expected == providerResponseMediaTypeJSON {
		matched = matched || (strings.HasPrefix(mediaType, "application/") && strings.HasSuffix(mediaType, "+json"))
	}
	if !matched {
		return providerProtocolError("response_media_type_mismatch", 0)
	}
	return nil
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

// RuntimeProviderFailureIdentity turns the status code into a stable identity
// without exposing even the sanitized status body across the provider owner
// boundary.
func (e *ProviderHTTPStatusError) RuntimeProviderFailureIdentity() (string, string) {
	if e == nil || e.StatusCode <= 0 {
		return "", ""
	}
	code := fmt.Sprintf("provider_http_status_%d", e.StatusCode)
	return code, strings.ReplaceAll(code, "_", "-")
}

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
	return &providerStreamGuard{limits: limits}
}

func (g *providerStreamGuard) observeDataEvent() error {
	if g == nil {
		return nil
	}
	next, ok := safeProviderLimitAdd(g.dataEvents, 1)
	if !ok || next > g.limits.MaxDataEvents {
		return providerProtocolError("stream_event_limit_exceeded", 0)
	}
	g.dataEvents = next
	return nil
}

func (g *providerStreamGuard) observeDelta(delta string) error {
	if g == nil || delta == "" {
		return nil
	}
	next, ok := safeProviderLimitAdd(g.contentBytes, len(delta))
	if !ok || next > g.limits.MaxContentBytes {
		return providerProtocolError("stream_content_limit_exceeded", 0)
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
		client: providerHTTPClient(nil),
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
	return &HTTPProvider{config: config, client: providerHTTPClient(client)}
}

func providerHTTPClient(source *http.Client) *http.Client {
	client := &http.Client{}
	if source != nil {
		copy := *source
		client = &copy
	}
	client.Timeout = 0
	previous := client.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req == nil || len(via) == 0 || via[0] == nil || via[0].URL == nil || req.URL == nil {
			return providerProtocolError("redirect_identity_invalid", 0)
		}
		origin := via[0].URL
		if !strings.EqualFold(origin.Scheme, req.URL.Scheme) || !strings.EqualFold(origin.Host, req.URL.Host) {
			return providerProtocolError("cross_origin_redirect_rejected", 0)
		}
		if len(via) >= 10 {
			return providerProtocolError("redirect_limit_exceeded", 0)
		}
		// Caller policy may narrow redirects further, but it cannot bypass the
		// owner guard above or authorize an unbounded replay of a POST body.
		if previous != nil {
			return previous(req, via)
		}
		return nil
	}
	return client
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
	if err == nil {
		return nil
	}
	var protocolErr *ProviderProtocolError
	if errors.As(err, &protocolErr) && protocolErr != nil {
		return &ProviderProtocolError{Code: protocolErr.Code, ResponseBytes: protocolErr.ResponseBytes}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("llm provider request timeout: %w", context.DeadlineExceeded)
	}
	if errors.Is(err, context.Canceled) {
		return &ProviderTransportError{Code: "request_canceled", cause: context.Canceled}
	}
	var unknownAuthority x509.UnknownAuthorityError
	var hostnameError x509.HostnameError
	var certificateError x509.CertificateInvalidError
	var recordHeaderError tls.RecordHeaderError
	if errors.As(err, &unknownAuthority) || errors.As(err, &hostnameError) || errors.As(err, &certificateError) || errors.As(err, &recordHeaderError) {
		return &ProviderTransportError{Code: "tls_verification_failed"}
	}
	var dnsError *net.DNSError
	if errors.As(err, &dnsError) {
		return &ProviderTransportError{Code: "dns_resolution_failed"}
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return &ProviderTransportError{Code: "network_timeout"}
	}
	return &ProviderTransportError{Code: "request_failed"}
}

func (p *HTTPProvider) Complete(ctx context.Context, request llmmodels.ProviderRequest) (llmmodels.ProviderCompletionResult, error) {
	if p == nil || p.client == nil {
		return llmmodels.ProviderCompletionResult{}, common.ErrInvalidArgument
	}
	return p.completeResponses(ctx, request)
}

func (p *HTTPProvider) completeResponses(ctx context.Context, request llmmodels.ProviderRequest) (llmmodels.ProviderCompletionResult, error) {
	limits, err := providerCompletionResponseLimits(p.config, request)
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
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
	result := llmmodels.ProviderCompletionResult{RequestBytes: len(payload)}
	requestCtx, cancel := p.requestContext(ctx)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return result, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	setBearerAuthorization(httpReq.Header, p.config.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return result, providerRequestError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return result, providerHTTPStatusError(resp.StatusCode)
	}
	if err := validateProviderResponseMediaType(resp.Header, providerResponseMediaTypeJSON); err != nil {
		return result, err
	}
	body, responseBytes, err := readBoundedProviderBody(resp.Body, limits.MaxNonStreamWireBytes)
	result.ResponseBytes = responseBytes
	if err != nil {
		return result, err
	}
	var decoded responsesResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return result, providerProtocolError("invalid_response_json", responseBytes)
	}
	if decoded.Error != nil {
		return result, providerProtocolError("response_error_object", responseBytes)
	}
	// Enforce the decoded-content ceiling before ordinary structured-output
	// normalization. Otherwise a provider can consume an unbounded response
	// budget with leading/trailing whitespace that TrimSpace subsequently
	// removes from the delivered payload.
	rawContent := decoded.Content()
	if len(rawContent) > limits.MaxContentBytes {
		return result, providerProtocolError("stream_content_limit_exceeded", responseBytes)
	}
	usage, err := decoded.Usage.ProviderUsage()
	if err != nil {
		return result, withProviderProtocolResponseBytes(err, responseBytes)
	}
	content := providerCompletionContent(request, rawContent)
	result.Payload = []byte(content)
	result.Usage = usage
	if decoded.Status == "incomplete" && decoded.IncompleteDetails.Reason == "max_output_tokens" {
		return result, &ProviderOutputLimitError{Payload: []byte(content), Usage: usage}
	}
	if strings.TrimSpace(content) == "" {
		result.Payload = nil
		return result, providerProtocolError("final_output_missing", responseBytes)
	}
	return result, nil
}

func (p *HTTPProvider) StreamComplete(ctx context.Context, request llmmodels.ProviderRequest, emit func(llmmodels.ProviderStreamEvent) error) (llmmodels.ProviderCompletionResult, error) {
	if p == nil || p.client == nil {
		return llmmodels.ProviderCompletionResult{}, common.ErrInvalidArgument
	}
	return p.streamResponses(ctx, request, emit)
}

func (p *HTTPProvider) streamResponses(ctx context.Context, request llmmodels.ProviderRequest, emit func(llmmodels.ProviderStreamEvent) error) (llmmodels.ProviderCompletionResult, error) {
	limits, err := providerCompletionResponseLimits(p.config, request)
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
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
	result := llmmodels.ProviderCompletionResult{RequestBytes: len(payload)}
	requestCtx, cancel := p.requestContext(ctx)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return result, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	setBearerAuthorization(httpReq.Header, p.config.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return result, providerRequestError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return result, providerHTTPStatusError(resp.StatusCode)
	}
	if err := validateProviderResponseMediaType(resp.Header, providerResponseMediaTypeEventStream); err != nil {
		return result, err
	}
	limited := newProviderResponseLimitReader(resp.Body, limits.MaxStreamWireBytes)
	content, usage, err := readResponsesStream(limited, emit, providerStreamLimits{
		MaxContentBytes: limits.MaxContentBytes,
		MaxDataEvents:   limits.MaxDataEvents,
		MaxLineBytes:    limits.MaxLineBytes,
	})
	result.ResponseBytes = limited.BytesRead()
	result.Usage = usage
	if err != nil {
		var protocolErr *ProviderProtocolError
		if errors.As(err, &protocolErr) {
			result.Payload = nil
			result.Usage = nil
			return result, withProviderProtocolResponseBytes(err, result.ResponseBytes)
		}
		content = providerCompletionContent(request, content)
		result.Payload = []byte(content)
		return result, withProviderProtocolResponseBytes(err, result.ResponseBytes)
	}
	content = providerCompletionContent(request, content)
	if strings.TrimSpace(content) == "" {
		return result, providerProtocolError("final_output_missing", result.ResponseBytes)
	}
	result.Payload = []byte(content)
	return result, nil
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
	responseLimit, err := providerEmbeddingResponseLimit(p.config, input)
	if err != nil {
		return llmmodels.ProviderEmbeddingResult{}, err
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
	result := llmmodels.ProviderEmbeddingResult{ProviderID: p.config.ID, Model: model, Input: input, RequestBytes: len(payload)}
	requestCtx, cancel := p.requestContext(ctx)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return result, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	setBearerAuthorization(httpReq.Header, p.config.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return result, providerRequestError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return result, providerHTTPStatusError(resp.StatusCode)
	}
	if err := validateProviderResponseMediaType(resp.Header, providerResponseMediaTypeJSON); err != nil {
		return result, err
	}
	body, responseBytes, err := readBoundedProviderBody(resp.Body, responseLimit)
	result.ResponseBytes = responseBytes
	if err != nil {
		return result, err
	}
	var decoded embeddingResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return result, providerProtocolError("invalid_embedding_response_json", responseBytes)
	}
	usage, err := decoded.Usage.ProviderUsage()
	if err != nil {
		return result, withProviderProtocolResponseBytes(err, responseBytes)
	}
	vector := decoded.FirstEmbedding()
	if len(vector) == 0 {
		return result, providerProtocolError("embedding_output_missing", responseBytes)
	}
	result.Usage = usage
	result.Vector = vector
	return result, nil
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
	if r.OutputText != "" {
		return r.OutputText
	}
	var b strings.Builder
	for _, item := range r.Output {
		for _, content := range item.Content {
			if content.Type == "output_text" {
				b.WriteString(content.Text)
			}
		}
	}
	return b.String()
}

func (u responsesUsage) ProviderUsage() (*llmaudit.ProviderUsage, error) {
	if u.InputTokens < 0 || u.OutputTokens < 0 || u.TotalTokens < 0 {
		return nil, providerProtocolError("invalid_response_usage", 0)
	}
	if u.InputTokens == 0 && u.OutputTokens == 0 && u.TotalTokens == 0 {
		return nil, nil
	}
	if u.TotalTokens == 0 {
		totalTokens, ok := safeProviderLimitAdd(u.InputTokens, u.OutputTokens)
		if !ok {
			return nil, providerProtocolError("invalid_response_usage", 0)
		}
		u.TotalTokens = totalTokens
	}
	return &llmaudit.ProviderUsage{InputTokens: u.InputTokens, OutputTokens: u.OutputTokens, TotalTokens: u.TotalTokens}, nil
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
	Usage embeddingUsage `json:"usage"`
}

type embeddingUsage struct {
	PromptTokens int `json:"prompt_tokens,omitempty"`
	InputTokens  int `json:"input_tokens,omitempty"`
	TotalTokens  int `json:"total_tokens,omitempty"`
}

func (u embeddingUsage) ProviderUsage() (*llmaudit.ProviderUsage, error) {
	if u.PromptTokens < 0 || u.InputTokens < 0 || u.TotalTokens < 0 {
		return nil, providerProtocolError("invalid_embedding_usage", 0)
	}
	inputTokens := max(u.PromptTokens, u.InputTokens)
	totalTokens := u.TotalTokens
	if totalTokens == 0 {
		totalTokens = inputTokens
	}
	if inputTokens == 0 && totalTokens == 0 {
		return nil, nil
	}
	return &llmaudit.ProviderUsage{InputTokens: inputTokens, TotalTokens: totalTokens}, nil
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

func readResponsesStream(reader io.Reader, emit func(llmmodels.ProviderStreamEvent) error, limits providerStreamLimits) (string, *llmaudit.ProviderUsage, error) {
	// Scanner's maximum token buffer includes the line delimiter. Reserve the
	// delimiter plus one probe byte, then enforce MaxLineBytes against the
	// delimiter-free token so an exact-size line is accepted and limit+1 is
	// rejected deterministically.
	scannerLimit, ok := safeProviderLimitAdd(limits.MaxLineBytes, 2)
	if !ok {
		return "", nil, providerProtocolError("stream_line_limit_exceeded", 0)
	}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, min(64*1024, scannerLimit)), scannerLimit)
	stream := &responsesStreamReader{guard: newProviderStreamGuard(limits), emit: emit}
	var pendingData string
	pending := false
	consumePending := func() (stop bool, preservePartial bool, err error) {
		if !pending {
			return false, false, nil
		}
		data := pendingData
		pendingData = ""
		pending = false
		return stream.consume(data)
	}
	for scanner.Scan() {
		if len(scanner.Bytes()) > limits.MaxLineBytes {
			return "", nil, providerProtocolError("stream_line_limit_exceeded", 0)
		}
		// Delay consuming the final scanned data line until Scanner has proved
		// that the line ended cleanly. A bounded reader can return one partial
		// token before surfacing its limit error; decoding that token first would
		// incorrectly replace response_byte_limit_exceeded with invalid JSON.
		if stop, preservePartial, err := consumePending(); err != nil {
			if preservePartial {
				return stream.builder.String(), stream.usage, err
			}
			return "", nil, err
		} else if stop {
			return stream.builder.String(), stream.usage, nil
		}
		data, ok := responsesStreamData(scanner.Text())
		if !ok {
			continue
		}
		pendingData = data
		pending = true
	}
	if err := scanner.Err(); err != nil {
		var protocolErr *ProviderProtocolError
		if errors.As(err, &protocolErr) {
			return "", nil, err
		}
		if strings.Contains(strings.ToLower(err.Error()), "token too long") {
			return "", nil, providerProtocolError("stream_line_limit_exceeded", 0)
		}
		return "", nil, err
	}
	if stop, preservePartial, err := consumePending(); err != nil {
		if preservePartial {
			return stream.builder.String(), stream.usage, err
		}
		return "", nil, err
	} else if stop {
		return stream.builder.String(), stream.usage, nil
	}
	if !stream.completed {
		return "", nil, providerProtocolError("stream_ended_before_completion", 0)
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
		usage, err := event.Response.Usage.ProviderUsage()
		if err != nil {
			return false, false, err
		}
		if usage != nil {
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
		return responsesStreamEvent{}, providerProtocolError("invalid_stream_event_json", len(data))
	}
	if event.Error != nil {
		return responsesStreamEvent{}, providerProtocolError("stream_error_object", len(data))
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
	return providerProtocolError("stream_incomplete_"+sanitizeCatalogLogValue(reason, 64), 0)
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
