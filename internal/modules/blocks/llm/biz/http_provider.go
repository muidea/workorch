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
	"sync"
	"time"
	"unicode/utf8"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/llmaudit"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

type providerStreamLimits struct {
	MaxContentBytes int
	MaxDataEvents   int
	MaxLineBytes    int
}

const (
	providerResponseMediaTypeJSON        = "application/json"
	providerResponseMediaTypeEventStream = "text/event-stream"
	providerFinalOutputMissingCode       = "final_output_missing"
)

var (
	errProviderResponseIdleTimeout = errors.New("provider response became idle")
	errProviderRequestHardDeadline = errors.New("provider request hard deadline exceeded")
)

// ProviderHTTPStatusError 保持 audit-safe 状态码；文档调用的响应原文
// 通过独立的受限 WireObservation 保全，不混入错误字符串。
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

// ProviderTransportError is the audit-safe boundary for request/response
// transport errors. Standard url.Error values include the requested endpoint,
// while custom transports may include credentials or request fragments. Only
// a stable category crosses the provider adapter boundary.
type ProviderTransportError struct {
	Code  string
	cause error
	// 仅供文档调用读取的原始 transport leaf，绝不进入 Error/审计。
	diagnosticCause error
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

// providerResponseIdleGuard owns the response-header/body idle timer for one
// streaming physical call. Progress records the actual wire-read time, so a
// timer callback racing a read rechecks the latest timestamp before canceling.
type providerResponseIdleGuard struct {
	mu           sync.Mutex
	timeout      time.Duration
	lastProgress time.Time
	timer        *time.Timer
	cancel       context.CancelCauseFunc
	stopped      bool
}

func newProviderResponseIdleGuard(parent context.Context, timeout time.Duration) (context.Context, *providerResponseIdleGuard) {
	if timeout <= 0 {
		timeout = defaultProviderTimeout
	}
	ctx, cancel := context.WithCancelCause(parent)
	guard := &providerResponseIdleGuard{timeout: timeout, lastProgress: time.Now(), cancel: cancel}
	guard.timer = time.AfterFunc(timeout, guard.expire)
	return ctx, guard
}

func (g *providerResponseIdleGuard) progress() {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.stopped {
		return
	}
	g.lastProgress = time.Now()
	g.timer.Reset(g.timeout)
}

func (g *providerResponseIdleGuard) expire() {
	if g == nil {
		return
	}
	g.mu.Lock()
	if g.stopped {
		g.mu.Unlock()
		return
	}
	remaining := g.timeout - time.Since(g.lastProgress)
	if remaining > 0 {
		g.timer.Reset(remaining)
		g.mu.Unlock()
		return
	}
	g.stopped = true
	cancel := g.cancel
	g.mu.Unlock()
	cancel(errProviderResponseIdleTimeout)
}

func (g *providerResponseIdleGuard) stop() {
	if g == nil {
		return
	}
	g.mu.Lock()
	if g.stopped {
		g.mu.Unlock()
		return
	}
	g.stopped = true
	g.timer.Stop()
	cancel := g.cancel
	g.mu.Unlock()
	cancel(nil)
}

type providerResponseProgressReader struct {
	reader io.Reader
	guard  *providerResponseIdleGuard
}

func (r *providerResponseProgressReader) Read(buffer []byte) (int, error) {
	n, err := r.reader.Read(buffer)
	if n > 0 {
		// The positive read is linearized under the guard before it is returned
		// to the stream parser. If expiry already owns that lock, the configured
		// no-progress interval elapsed before this adapter-level Read completed.
		r.guard.progress()
	}
	return n, err
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
// provider explicitly stopped because the output token budget was reached.
// Payload is retained only for bounded framing/audit; it is not a continuation
// prefix. The Runtime owner may reissue the same sealed expected document from
// scratch with a new physical-call identity.
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
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req == nil || len(via) == 0 || via[0] == nil || via[0].URL == nil || req.URL == nil {
			return providerProtocolError("redirect_identity_invalid", 0)
		}
		origin := via[0].URL
		if !strings.EqualFold(origin.Scheme, req.URL.Scheme) || !strings.EqualFold(origin.Host, req.URL.Host) {
			return providerProtocolError("cross_origin_redirect_rejected", 0)
		}
		// A Runtime PhysicalCallID authorizes exactly one provider submission.
		// Following even a same-origin redirect would replay the POST body under
		// that identity, so fail closed before net/http performs a second call.
		// An injected client policy cannot broaden this owner invariant.
		return providerProtocolError("redirect_rejected", 0)
	}
	return client
}

// requestContext keeps non-streaming and embedding calls on the configured
// absolute timeout. Streaming uses streamRequestContext plus a separate
// response-idle guard because active wire progress must not consume the idle
// interval.
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

// streamRequestContext applies only the production hard ceiling. The parent
// context remains authoritative when it has an earlier deadline.
func (p *HTTPProvider) streamRequestContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := p.config.HardTimeout
	if timeout <= 0 {
		timeout = workorchconfig.MaxAIProxyTimeout
	}
	return context.WithTimeoutCause(ctx, timeout, errProviderRequestHardDeadline)
}

func providerRequestContextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	cause := context.Cause(ctx)
	switch {
	case errors.Is(cause, errProviderResponseIdleTimeout):
		return &ProviderTransportError{Code: "provider_body_idle_timeout", cause: cause}
	case errors.Is(cause, errProviderRequestHardDeadline), errors.Is(cause, context.DeadlineExceeded):
		return &ProviderTransportError{Code: "provider_request_timeout", cause: cause}
	case errors.Is(cause, context.Canceled):
		return &ProviderTransportError{Code: "request_canceled", cause: cause}
	default:
		return nil
	}
}

func providerRequestErrorWithContext(err error, ctx context.Context) error {
	if contextual := providerRequestContextError(ctx); contextual != nil {
		if transport, ok := contextual.(*ProviderTransportError); ok {
			transport.diagnosticCause = providerTransportLeaf(err)
		}
		return contextual
	}
	return providerRequestError(err)
}

// net/http 的 url.Error 包装包含调用配置 URL；仅保全实际 transport
// leaf，不将请求包装或配置拼入诊断，也不清洗 leaf 原文。
func providerTransportLeaf(err error) error {
	for {
		wrapped, ok := err.(*url.Error)
		if !ok {
			return err
		}
		err = wrapped.Err
	}
}

func providerRequestError(err error) error {
	if err == nil {
		return nil
	}
	var protocolErr *ProviderProtocolError
	if errors.As(err, &protocolErr) && protocolErr != nil {
		return &ProviderProtocolError{Code: protocolErr.Code, ResponseBytes: protocolErr.ResponseBytes}
	}
	leaf := providerTransportLeaf(err)
	transportError := func(code string, cause error) error {
		return &ProviderTransportError{Code: code, cause: cause, diagnosticCause: leaf}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return transportError("provider_request_timeout", context.DeadlineExceeded)
	}
	if errors.Is(err, context.Canceled) {
		return transportError("request_canceled", context.Canceled)
	}
	var unknownAuthority x509.UnknownAuthorityError
	var hostnameError x509.HostnameError
	var certificateError x509.CertificateInvalidError
	var recordHeaderError tls.RecordHeaderError
	if errors.As(err, &unknownAuthority) || errors.As(err, &hostnameError) || errors.As(err, &certificateError) || errors.As(err, &recordHeaderError) {
		return transportError("tls_verification_failed", nil)
	}
	var dnsError *net.DNSError
	if errors.As(err, &dnsError) {
		return transportError("dns_resolution_failed", nil)
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return transportError("network_timeout", nil)
	}
	return transportError("request_failed", nil)
}

func (p *HTTPProvider) Complete(ctx context.Context, request llmmodels.ProviderRequest) (result llmmodels.ProviderCompletionResult, err error) {
	defer func() { result.PhysicalResponseMode = llmaudit.PhysicalResponseModeNonStreaming }()
	if p == nil || p.client == nil {
		return llmmodels.ProviderCompletionResult{}, common.ErrInvalidArgument
	}
	return p.completeResponses(ctx, request)
}

func (p *HTTPProvider) completeResponses(ctx context.Context, request llmmodels.ProviderRequest) (result llmmodels.ProviderCompletionResult, callErr error) {
	if err := request.ValidatePromptLayout(); err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	if usesSectionMessages(request) {
		request.Model = p.config.Model
		request.Config.ID = p.config.ID
	}
	if err := request.ValidateSchemaFormat(); err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	limits, err := providerCompletionResponseLimits(p.config, request)
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	endpoint, err := responsesURL(p.config.BaseURL)
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	requestPayload := responsesRequest{
		Model:              p.config.Model,
		Input:              responsesWireInputForRequest(&request),
		PromptCacheKey:     responsesPromptCacheKey(request),
		PromptCacheOptions: responsesPromptCacheOptions(request),
		Temperature:        providerTemperature(p.config),
		MaxOutputTokens:    maxOutputTokensForRequest(p.config, request),
		Reasoning:          responsesReasoningForRequest(request),
		Text:               responsesTextForRequest(p.config.Capabilities, request),
	}
	payload, err := json.Marshal(requestPayload)
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	result = llmmodels.ProviderCompletionResult{RequestBytes: len(payload)}
	if usesSectionMessages(request) {
		request.ContextBudget.ActualBytes = len(payload)
	}
	if usesSectionMessages(request) && p.config.ContextProfile.MaxRequestBytes > 0 && len(payload) > p.config.ContextProfile.MaxRequestBytes {
		return result, fmt.Errorf("%w: section message request exceeds provider byte budget", common.ErrInvalidArgument)
	}
	requestCtx, cancel := p.requestContext(ctx)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return result, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	setBearerAuthorization(httpReq.Header, p.config.APIKey)

	resp, err := dispatchProviderHTTPRequest(p.client, httpReq, &result.ProviderSubmitted)
	if err != nil {
		return result, providerRequestError(err)
	}
	defer resp.Body.Close()
	var responseReader io.Reader = resp.Body
	if request.CaptureDocumentObservation {
		observed := newProviderDocumentObservationReader(resp.Body)
		responseReader = observed
		defer func() { result.WireObservation = observed.observation() }()
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		if request.CaptureDocumentObservation {
			_, result.ResponseBytes, _ = readBoundedProviderBody(responseReader, limits.MaxNonStreamWireBytes)
		}
		return result, providerHTTPStatusError(resp.StatusCode)
	}
	if err := validateProviderResponseMediaType(resp.Header, providerResponseMediaTypeJSON); err != nil {
		return result, err
	}
	body, responseBytes, err := readBoundedProviderBody(responseReader, limits.MaxNonStreamWireBytes)
	result.ResponseBytes = responseBytes
	if err != nil {
		if request.CaptureDocumentObservation {
			return result, providerRequestError(err)
		}
		return result, err
	}
	var decoded responsesResponse
	if request.CaptureDocumentObservation && !providerJSONTextLossless(body) {
		return result, providerProtocolError("response_text_encoding_invalid", responseBytes)
	}
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
	if request.CaptureDocumentObservation {
		result.DocumentObservation = providerDocumentObservation(rawContent, false)
	}
	if len(rawContent) > limits.MaxContentBytes {
		return result, providerProtocolError("stream_content_limit_exceeded", responseBytes)
	}
	usage, err := decoded.Usage.ProviderUsage()
	if err != nil {
		return result, withProviderProtocolResponseBytes(err, responseBytes)
	}
	content := providerCompletionContent(request, rawContent)
	disposition, err := classifyResponsesTerminal(decoded.Status, decoded.IncompleteDetails.Reason)
	if err != nil {
		return result, withProviderProtocolResponseBytes(err, responseBytes)
	}
	if disposition == responsesTerminalOutputLimit {
		result.Payload = []byte(content)
		result.Usage = usage
		return result, &ProviderOutputLimitError{Payload: []byte(content), Usage: usage}
	}
	if request.CaptureDocumentObservation {
		result.DocumentObservation = providerDocumentObservation(rawContent, true)
		result.Payload = []byte(content)
		result.Usage = usage
		return result, nil
	}
	if strings.TrimSpace(content) == "" {
		// Preserve provider-native usage for the Runtime candidate owner. The
		// generic completion surface still receives an error, while
		// GenerateCandidate converts this exact typed terminal observation into
		// the owner-defined empty_document fact.
		result.Usage = usage
		return result, providerProtocolError(providerFinalOutputMissingCode, responseBytes)
	}
	result.Payload = []byte(content)
	result.Usage = usage
	return result, nil
}

func (p *HTTPProvider) StreamComplete(ctx context.Context, request llmmodels.ProviderRequest, emit func(llmmodels.ProviderStreamEvent) error) (result llmmodels.ProviderCompletionResult, err error) {
	defer func() {
		result.PhysicalResponseMode = llmaudit.PhysicalResponseModeNonStreaming
		if result.ProviderSubmitted {
			result.PhysicalResponseMode = llmaudit.PhysicalResponseModeStreaming
		}
	}()
	if p == nil || p.client == nil {
		return llmmodels.ProviderCompletionResult{}, common.ErrInvalidArgument
	}
	return p.streamResponses(ctx, request, emit)
}

func (p *HTTPProvider) streamResponses(ctx context.Context, request llmmodels.ProviderRequest, emit func(llmmodels.ProviderStreamEvent) error) (result llmmodels.ProviderCompletionResult, callErr error) {
	if err := request.ValidatePromptLayout(); err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	if usesSectionMessages(request) {
		request.Model = p.config.Model
		request.Config.ID = p.config.ID
	}
	if err := request.ValidateSchemaFormat(); err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	limits, err := providerCompletionResponseLimits(p.config, request)
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	endpoint, err := responsesURL(p.config.BaseURL)
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	requestPayload := responsesRequest{
		Model:              p.config.Model,
		Input:              responsesWireInputForRequest(&request),
		PromptCacheKey:     responsesPromptCacheKey(request),
		PromptCacheOptions: responsesPromptCacheOptions(request),
		Temperature:        providerTemperature(p.config),
		MaxOutputTokens:    maxOutputTokensForRequest(p.config, request),
		Reasoning:          responsesReasoningForRequest(request),
		Text:               responsesTextForRequest(p.config.Capabilities, request),
		Stream:             true,
	}
	payload, err := json.Marshal(requestPayload)
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	result = llmmodels.ProviderCompletionResult{RequestBytes: len(payload)}
	if usesSectionMessages(request) {
		request.ContextBudget.ActualBytes = len(payload)
	}
	if usesSectionMessages(request) && p.config.ContextProfile.MaxRequestBytes > 0 && len(payload) > p.config.ContextProfile.MaxRequestBytes {
		return result, fmt.Errorf("%w: section message request exceeds provider byte budget", common.ErrInvalidArgument)
	}
	hardCtx, cancelHard := p.streamRequestContext(ctx)
	defer cancelHard()
	requestCtx, idleGuard := newProviderResponseIdleGuard(hardCtx, p.config.Timeout)
	defer idleGuard.stop()
	httpReq, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return result, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	setBearerAuthorization(httpReq.Header, p.config.APIKey)

	resp, err := dispatchProviderHTTPRequest(p.client, httpReq, &result.ProviderSubmitted)
	if err != nil {
		return result, providerRequestErrorWithContext(err, requestCtx)
	}
	idleGuard.progress()
	defer resp.Body.Close()
	var responseReader io.Reader = resp.Body
	if request.CaptureDocumentObservation {
		observed := newProviderDocumentObservationReader(resp.Body)
		responseReader = observed
		defer func() { result.WireObservation = observed.observation() }()
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		if request.CaptureDocumentObservation {
			progress := &providerResponseProgressReader{reader: responseReader, guard: idleGuard}
			_, result.ResponseBytes, _ = readBoundedProviderBody(progress, limits.MaxStreamWireBytes)
		}
		return result, providerHTTPStatusError(resp.StatusCode)
	}
	if err := validateProviderResponseMediaType(resp.Header, providerResponseMediaTypeEventStream); err != nil {
		return result, err
	}
	progressReader := &providerResponseProgressReader{reader: responseReader, guard: idleGuard}
	limited := newProviderResponseLimitReader(progressReader, limits.MaxStreamWireBytes)
	limitsForStream := providerStreamLimits{MaxContentBytes: limits.MaxContentBytes, MaxDataEvents: limits.MaxDataEvents, MaxLineBytes: limits.MaxLineBytes}
	var content string
	var usage *llmaudit.ProviderUsage
	if request.CaptureDocumentObservation {
		stream := &responsesStreamReader{guard: newProviderStreamGuard(limitsForStream), emit: emit, preserveDocument: true}
		content, usage, err = readResponsesStreamState(limited, limitsForStream, stream)
		if stream.outputObserved || err == nil && stream.completed {
			result.DocumentObservation = providerDocumentObservation(stream.builder.String(), err == nil && stream.completed)
		}
	} else {
		content, usage, err = readResponsesStream(limited, emit, limitsForStream)
	}
	result.ResponseBytes = limited.BytesRead()
	result.Usage = usage
	if err != nil {
		var protocolErr *ProviderProtocolError
		if errors.As(err, &protocolErr) {
			result.Payload = nil
			result.Usage = nil
			return result, withProviderProtocolResponseBytes(err, result.ResponseBytes)
		}
		var outputLimitErr *ProviderOutputLimitError
		if errors.As(err, &outputLimitErr) {
			content = providerCompletionContent(request, content)
			result.Payload = []byte(content)
			return result, err
		}
		if contextual := providerRequestContextError(requestCtx); contextual != nil {
			result.Payload = nil
			result.Usage = nil
			if request.CaptureDocumentObservation {
				return result, providerRequestErrorWithContext(err, requestCtx)
			}
			return result, contextual
		}
		if request.CaptureDocumentObservation {
			return result, providerRequestError(err)
		}
		content = providerCompletionContent(request, content)
		result.Payload = []byte(content)
		return result, withProviderProtocolResponseBytes(err, result.ResponseBytes)
	}
	content = providerCompletionContent(request, content)
	if strings.TrimSpace(content) == "" && !request.CaptureDocumentObservation {
		return result, providerProtocolError(providerFinalOutputMissingCode, result.ResponseBytes)
	}
	result.Payload = []byte(content)
	return result, nil
}

func (p *HTTPProvider) Embed(ctx context.Context, request llmmodels.ProviderEmbeddingRequest) (result llmmodels.ProviderEmbeddingResult, err error) {
	defer func() { result.PhysicalResponseMode = llmaudit.PhysicalResponseModeNonStreaming }()
	if p == nil || p.client == nil {
		return llmmodels.ProviderEmbeddingResult{}, common.ErrInvalidArgument
	}
	input := request.Input
	if strings.TrimSpace(input) == "" {
		return llmmodels.ProviderEmbeddingResult{}, fmt.Errorf("%w: embedding input is required", common.ErrInvalidArgument)
	}
	if !utf8.ValidString(input) {
		return llmmodels.ProviderEmbeddingResult{}, fmt.Errorf("%w: embedding input must be valid UTF-8", common.ErrInvalidArgument)
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
	if len(payload) > embeddingMaxRequestBytes {
		return llmmodels.ProviderEmbeddingResult{}, fmt.Errorf("%w: embedding JSON request exceeds local wire budget", common.ErrInvalidArgument)
	}
	result = llmmodels.ProviderEmbeddingResult{
		ProviderID: p.config.ID, Model: model, Input: input, RequestBytes: len(payload),
		PhysicalResponseMode: llmaudit.PhysicalResponseModeNonStreaming,
	}
	requestCtx, cancel := p.requestContext(ctx)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return result, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	setBearerAuthorization(httpReq.Header, p.config.APIKey)

	resp, err := dispatchProviderHTTPRequest(p.client, httpReq, &result.ProviderSubmitted)
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

// dispatchProviderHTTPRequest is the sole physical HTTP dispatch boundary for
// completion, streaming completion and embedding. The submitted fact becomes
// true immediately before invoking client.Do and remains true for every result
// returned after that invocation, including transport and response failures.
// Request construction/guard failures never reach this function and therefore
// remain explicitly unsubmitted even when a serialized payload size is known.
func dispatchProviderHTTPRequest(client *http.Client, request *http.Request, submitted *bool) (*http.Response, error) {
	if client == nil || request == nil || submitted == nil {
		return nil, common.ErrInvalidArgument
	}
	*submitted = true
	return client.Do(request)
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
	Model              string                 `json:"model"`
	Input              json.RawMessage        `json:"input"`
	PromptCacheKey     string                 `json:"prompt_cache_key,omitempty"`
	PromptCacheOptions *responsesCacheOptions `json:"prompt_cache_options,omitempty"`
	Temperature        float64                `json:"temperature,omitempty"`
	MaxOutputTokens    int64                  `json:"max_output_tokens,omitempty"`
	Reasoning          *responsesReasoning    `json:"reasoning,omitempty"`
	Text               *responsesTextConfig   `json:"text,omitempty"`
	Stream             bool                   `json:"stream,omitempty"`
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

type responsesTerminalDisposition uint8

const (
	responsesTerminalCompleted responsesTerminalDisposition = iota + 1
	responsesTerminalOutputLimit
)

// classifyResponsesTerminal is the provider-adapter-owned terminal reducer.
// Only an internally coherent completed response or the explicitly bounded
// max-output stop is authoritative; every other terminal shape fails closed.
func classifyResponsesTerminal(status string, reason string) (responsesTerminalDisposition, error) {
	status = strings.TrimSpace(status)
	reason = strings.TrimSpace(reason)
	switch status {
	case "completed":
		if reason != "" {
			return 0, providerProtocolError("response_completed_with_incomplete_details", 0)
		}
		return responsesTerminalCompleted, nil
	case "incomplete":
		if reason == "max_output_tokens" {
			return responsesTerminalOutputLimit, nil
		}
		if reason == "" {
			return 0, providerProtocolError("response_incomplete_reason_missing", 0)
		}
		return 0, providerProtocolError("response_incomplete_"+sanitizeCatalogLogValue(reason, 64), 0)
	case "":
		return 0, providerProtocolError("response_terminal_status_missing", 0)
	default:
		return 0, providerProtocolError("response_terminal_status_"+sanitizeCatalogLogValue(status, 64), 0)
	}
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
	InputTokens        int                        `json:"input_tokens,omitempty"`
	InputTokensDetails responsesInputTokenDetails `json:"input_tokens_details,omitempty"`
	OutputTokens       int                        `json:"output_tokens,omitempty"`
	TotalTokens        int                        `json:"total_tokens,omitempty"`
}

type responsesInputTokenDetails struct {
	CachedTokens          int  `json:"cached_tokens,omitempty"`
	CacheWriteTokens      int  `json:"cache_write_tokens,omitempty"`
	CachedTokensKnown     bool `json:"-"`
	CacheWriteTokensKnown bool `json:"-"`
}

// A missing or null measurement is unknown, never an observed zero.
func (d *responsesInputTokenDetails) UnmarshalJSON(data []byte) error {
	var value struct {
		CachedTokens     *int `json:"cached_tokens"`
		CacheWriteTokens *int `json:"cache_write_tokens"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*d = responsesInputTokenDetails{}
	if value.CachedTokens != nil {
		d.CachedTokens, d.CachedTokensKnown = *value.CachedTokens, true
	}
	if value.CacheWriteTokens != nil {
		d.CacheWriteTokens, d.CacheWriteTokensKnown = *value.CacheWriteTokens, true
	}
	return nil
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
	if u.InputTokens < 0 || u.OutputTokens < 0 || u.TotalTokens < 0 ||
		u.InputTokensDetails.CachedTokens < 0 || u.InputTokensDetails.CachedTokens > u.InputTokens || u.InputTokensDetails.CacheWriteTokens < 0 ||
		u.InputTokensDetails.CacheWriteTokens > u.InputTokens-u.InputTokensDetails.CachedTokens {
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
	return &llmaudit.ProviderUsage{
		InputTokens:           u.InputTokens,
		OutputTokens:          u.OutputTokens,
		TotalTokens:           u.TotalTokens,
		CacheReadTokens:       u.InputTokensDetails.CachedTokens,
		CacheWriteTokens:      u.InputTokensDetails.CacheWriteTokens,
		CacheReadTokensKnown:  u.InputTokensDetails.CachedTokensKnown || u.InputTokensDetails.CachedTokens != 0,
		CacheWriteTokensKnown: u.InputTokensDetails.CacheWriteTokensKnown || u.InputTokensDetails.CacheWriteTokens != 0,
	}, nil
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
	Type              string                 `json:"type"`
	Delta             string                 `json:"delta,omitempty"`
	Text              string                 `json:"text,omitempty"`
	Response          *responsesResponse     `json:"response,omitempty"`
	IncompleteDetails json.RawMessage        `json:"incomplete_details,omitempty"`
	Error             *providerResponseError `json:"error,omitempty"`
}

type responsesStreamReader struct {
	builder          strings.Builder
	usage            *llmaudit.ProviderUsage
	guard            *providerStreamGuard
	emit             func(llmmodels.ProviderStreamEvent) error
	completed        bool
	terminalErr      error
	doneReceived     bool
	doneEmitted      bool
	preserveDocument bool
	outputObserved   bool
}

func readResponsesStream(reader io.Reader, emit func(llmmodels.ProviderStreamEvent) error, limits providerStreamLimits) (string, *llmaudit.ProviderUsage, error) {
	return readResponsesStreamState(reader, limits, &responsesStreamReader{guard: newProviderStreamGuard(limits), emit: emit})
}

func readResponsesStreamState(reader io.Reader, limits providerStreamLimits, stream *responsesStreamReader) (string, *llmaudit.ProviderUsage, error) {
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
	if err := stream.finish(); err != nil {
		var outputLimitErr *ProviderOutputLimitError
		if errors.As(err, &outputLimitErr) {
			return stream.builder.String(), stream.usage, err
		}
		return "", nil, err
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
		if !s.completed {
			return false, false, providerProtocolError("stream_done_before_terminal", 0)
		}
		if s.doneReceived {
			return false, false, providerProtocolError("stream_duplicate_done", 0)
		}
		// [DONE] is a terminal wire marker, not permission to ignore the
		// remaining response body. Keep scanning to EOF so a provider cannot
		// append a contradictory data event after the marker and still obtain a
		// successful completion. The done callback is emitted only after that
		// bounded validation has succeeded.
		s.doneReceived = true
		return false, false, nil
	}
	if s.completed {
		return false, false, providerProtocolError("stream_event_after_terminal", 0)
	}
	if err := s.guard.observeDataEvent(); err != nil {
		return false, false, err
	}
	if s.preserveDocument && !providerJSONTextLossless([]byte(data)) {
		return false, false, providerProtocolError("response_text_encoding_invalid", len(data))
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
		return false, false, s.consumeOutputTextDone(event.Text)
	case "response.completed":
		return false, false, s.consumeCompleted(event)
	case "response.incomplete":
		err := s.incompleteError(event)
		var outputLimitErr *ProviderOutputLimitError
		if !errors.As(err, &outputLimitErr) {
			return false, false, err
		}
		return false, false, s.completeWithTerminalError(err)
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
	s.outputObserved = true
	s.builder.WriteString(content)
	if s.emit == nil {
		return nil
	}
	return s.emit(llmmodels.ProviderStreamEvent{Type: "delta", Delta: content})
}

func (s *responsesStreamReader) consumeOutputTextDone(content string) error {
	if content == "" {
		return nil
	}
	if s.builder.Len() == 0 {
		return s.appendOutput(content, true)
	}
	if content != s.builder.String() {
		return providerProtocolError("stream_output_text_done_conflict", 0)
	}
	return nil
}

func (s *responsesStreamReader) consumeCompleted(event responsesStreamEvent) error {
	if len(event.IncompleteDetails) != 0 {
		return providerProtocolError("stream_completed_with_incomplete_details", 0)
	}
	response := event.Response
	if response == nil {
		return providerProtocolError("stream_completed_response_missing", 0)
	}
	disposition, err := classifyResponsesTerminal(response.Status, response.IncompleteDetails.Reason)
	if err != nil {
		return err
	}
	if content := response.Content(); content != "" {
		if s.builder.Len() == 0 {
			if err := s.appendOutput(content, true); err != nil {
				return err
			}
		} else if content != s.builder.String() {
			return providerProtocolError("stream_completed_output_conflict", 0)
		}
	}
	if disposition == responsesTerminalOutputLimit {
		return s.completeWithTerminalError(&ProviderOutputLimitError{Payload: []byte(s.builder.String()), Usage: s.usage})
	}
	return s.complete()
}

func (s *responsesStreamReader) complete() error {
	if s.completed {
		return providerProtocolError("stream_duplicate_terminal", 0)
	}
	s.completed = true
	return nil
}

func (s *responsesStreamReader) completeWithTerminalError(err error) error {
	if err == nil {
		return providerProtocolError("stream_terminal_error_missing", 0)
	}
	if completeErr := s.complete(); completeErr != nil {
		return completeErr
	}
	s.terminalErr = err
	return nil
}

func (s *responsesStreamReader) finish() error {
	if !s.completed {
		return providerProtocolError("stream_done_before_terminal", 0)
	}
	if s.terminalErr != nil {
		return s.terminalErr
	}
	if s.doneEmitted {
		return providerProtocolError("stream_duplicate_done", 0)
	}
	s.doneEmitted = true
	if s.emit == nil {
		return nil
	}
	return s.emit(llmmodels.ProviderStreamEvent{Type: "done"})
}

func (s *responsesStreamReader) incompleteError(event responsesStreamEvent) error {
	if event.Response == nil {
		return providerProtocolError("stream_incomplete_response_missing", 0)
	}
	topReason := ""
	if len(event.IncompleteDetails) != 0 {
		var details responsesIncompleteDetails
		if err := json.Unmarshal(event.IncompleteDetails, &details); err != nil {
			return providerProtocolError("stream_incomplete_details_invalid", 0)
		}
		topReason = strings.TrimSpace(details.Reason)
	}
	responseReason := strings.TrimSpace(event.Response.IncompleteDetails.Reason)
	if topReason != "" && responseReason != "" && topReason != responseReason {
		return providerProtocolError("stream_incomplete_reason_conflict", 0)
	}
	reason := responseReason
	if reason == "" {
		reason = topReason
	}
	disposition, err := classifyResponsesTerminal(event.Response.Status, reason)
	if err != nil {
		return err
	}
	if disposition == responsesTerminalOutputLimit {
		return &ProviderOutputLimitError{Payload: []byte(s.builder.String()), Usage: s.usage}
	}
	return providerProtocolError("stream_incomplete_with_completed_status", 0)
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
