package web

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"

	webcommon "github.com/muidea/workorch/internal/modules/blocks/web/pkg/common"
	webmodels "github.com/muidea/workorch/internal/modules/blocks/web/pkg/models"
	webproviderconfig "github.com/muidea/workorch/internal/modules/blocks/web/pkg/providerconfig"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type safeFetchResponse struct {
	FinalURL    string
	Content     string
	ContentType string
	Duration    time.Duration
	Truncated   bool
}

type webFetchGovernanceRequiredError struct {
	TargetURL string
	Policy    policyDecision
}

func (e *webFetchGovernanceRequiredError) Error() string {
	return "runtime Web fetch requires approval before accessing the classified target"
}

type safeFetchPlan struct {
	adapter  *RuntimeAdapter
	provider webmodels.ProviderConfig
	request  RuntimeOperationRequest
	grant    *runtimecontract.RuntimeOperationGovernanceGrant

	mu          sync.Mutex
	addresses   map[string][]netip.Addr
	grantUsed   bool
	maxRedirect int
}

func newSafeFetchPlan(adapter *RuntimeAdapter, provider webmodels.ProviderConfig, request RuntimeOperationRequest) *safeFetchPlan {
	return &safeFetchPlan{
		adapter: adapter, provider: provider, request: request, grant: request.GovernanceGrant,
		addresses: map[string][]netip.Addr{}, maxRedirect: boundedInt(provider.MaxRedirects, webproviderconfig.DefaultMaxRedirects, webproviderconfig.SystemMaxRedirects),
	}
}

func (p *safeFetchPlan) prepare(ctx context.Context, rawURL string, redirected bool) (*url.URL, error) {
	parsed, urlDecision := parseWebURLPolicy(rawURL, p.provider)
	if parsed == nil || urlDecision.Decision == policyDenied {
		return nil, fmt.Errorf("%w: Web fetch URL policy denied", common.ErrInvalidArgument)
	}
	host := strings.ToLower(strings.Trim(parsed.Hostname(), "."))
	p.mu.Lock()
	addresses := append([]netip.Addr(nil), p.addresses[host]...)
	p.mu.Unlock()
	if len(addresses) == 0 {
		resolved, err := p.adapter.resolveHostAddresses(ctx, host)
		if err != nil {
			return nil, newWebTransportError(webcommon.ErrorBadGatewayResponse, "Web fetch DNS resolution failed", err)
		}
		addresses = resolved
	}
	decision := mergeURLAndAddressPolicy(urlDecision, classifyAddresses(addresses))
	if decision.Decision == policyDenied {
		return nil, fmt.Errorf("%w: Web fetch network policy denied", common.ErrInvalidArgument)
	}
	if decision.Decision == policyApproval {
		requirement := webFetchApprovalRequirement(p.provider, parsed.String(), boundedFetchBytes(p.request, p.provider), p.request.ExtractMode, decision)
		if p.grant == nil {
			return nil, &webFetchGovernanceRequiredError{TargetURL: parsed.String(), Policy: decision}
		}
		if err := validateRuntimeApprovalGrant(p.grant, requirement); err != nil {
			if redirected {
				return nil, &webFetchGovernanceRequiredError{TargetURL: parsed.String(), Policy: decision}
			}
			return nil, fmt.Errorf("%w: runtime Web approval grant does not match the current URL policy", common.ErrConflict)
		}
		p.mu.Lock()
		p.grantUsed = true
		p.mu.Unlock()
	}
	p.mu.Lock()
	if len(p.addresses[host]) == 0 {
		p.addresses[host] = append([]netip.Addr(nil), addresses...)
	}
	p.mu.Unlock()
	return parsed, nil
}

func (p *safeFetchPlan) checkRedirect(request *http.Request, via []*http.Request) error {
	if request == nil {
		return fmt.Errorf("Web fetch redirect request is missing")
	}
	if len(via) > p.maxRedirect {
		return fmt.Errorf("%w: Web fetch redirect limit exceeded", common.ErrInvalidArgument)
	}
	_, err := p.prepare(request.Context(), request.URL.String(), true)
	return err
}

func (p *safeFetchPlan) dialContext(ctx context.Context, network string, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, newWebTransportError(webcommon.ErrorBadGatewayResponse, "Web fetch transport address is invalid", err)
	}
	host = strings.ToLower(strings.Trim(strings.TrimSpace(host), "."))
	p.mu.Lock()
	addresses := append([]netip.Addr(nil), p.addresses[host]...)
	p.mu.Unlock()
	if len(addresses) == 0 {
		return nil, newWebTransportError(webcommon.ErrorBadGatewayResponse, "Web fetch target was not preflighted", nil)
	}
	dialer := &net.Dialer{}
	var lastErr error
	for _, target := range addresses {
		connection, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(target.String(), port))
		if dialErr == nil {
			return connection, nil
		}
		lastErr = dialErr
	}
	return nil, newWebTransportError(webcommon.ErrorBadGatewayResponse, "Web fetch connection failed", lastErr)
}

func (p *safeFetchPlan) usedGrant() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.grantUsed
}

func (a *RuntimeAdapter) doSafeFetchRequest(ctx context.Context, provider webmodels.ProviderConfig, request RuntimeOperationRequest, maxBytes int64) (safeFetchResponse, string, error) {
	if maxBytes <= 0 || maxBytes > webproviderconfig.SystemMaxFetchBytes {
		maxBytes = webproviderconfig.SystemMaxFetchBytes
	}
	plan := newSafeFetchPlan(a, provider, request)
	parsed, err := plan.prepare(ctx, request.URL, false)
	if err != nil {
		return safeFetchResponse{}, webFetchErrorCode(err), err
	}
	transport, err := a.safeFetchHTTPTransport(plan)
	if err != nil {
		return safeFetchResponse{}, webcommon.ErrorProviderUnavailable, err
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: plan.checkRedirect}
	callCtx, cancel := context.WithTimeout(ctx, providerTimeout(provider))
	defer cancel()
	httpRequest, err := http.NewRequestWithContext(callCtx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return safeFetchResponse{}, webcommon.ErrorInvalidRequest, fmt.Errorf("%w: invalid Web fetch request", common.ErrInvalidArgument)
	}
	httpRequest.Header.Set("Accept", "text/html,application/xhtml+xml,application/json,text/plain;q=0.9,*/*;q=0.1")
	httpRequest.Header.Set("User-Agent", "workorch-web-fetch/1")
	startedAt := time.Now()
	response, err := client.Do(httpRequest)
	duration := time.Since(startedAt)
	if err != nil {
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		var governanceRequired *webFetchGovernanceRequiredError
		if errors.As(err, &governanceRequired) {
			return safeFetchResponse{Duration: duration}, webcommon.ErrorApprovalRequired, governanceRequired
		}
		if errors.Is(callCtx.Err(), context.DeadlineExceeded) {
			return safeFetchResponse{Duration: duration}, webcommon.ErrorTimeout, newWebTransportError(webcommon.ErrorTimeout, "Web fetch timed out", context.DeadlineExceeded)
		}
		if errors.Is(err, common.ErrInvalidArgument) || errors.Is(err, common.ErrConflict) {
			return safeFetchResponse{Duration: duration}, webcommon.ErrorPolicyDenied, fmt.Errorf("%w: Web fetch redirect or network policy denied the request", common.ErrInvalidArgument)
		}
		return safeFetchResponse{Duration: duration}, webcommon.ErrorBadGatewayResponse, newWebTransportError(webcommon.ErrorBadGatewayResponse, "Web fetch transport failed", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return safeFetchResponse{Duration: duration}, webcommon.ErrorBadGatewayResponse, fmt.Errorf("Web fetch returned status %d", response.StatusCode)
	}
	raw, readErr := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if readErr != nil {
		return safeFetchResponse{Duration: duration}, webcommon.ErrorBadGatewayResponse, newWebTransportError(webcommon.ErrorBadGatewayResponse, "Web fetch response read failed", readErr)
	}
	truncated := int64(len(raw)) > maxBytes
	if truncated {
		raw = raw[:maxBytes]
	}
	if request.GovernanceGrant != nil && !plan.usedGrant() {
		return safeFetchResponse{Duration: duration}, webcommon.ErrorPolicyDenied, fmt.Errorf("%w: runtime Web approval grant is stale for the current URL policy", common.ErrConflict)
	}
	contentType, _ := boundWebText(response.Header.Get("Content-Type"), webContentTypeMaxBytes)
	return safeFetchResponse{
		FinalURL: response.Request.URL.String(), Content: string(raw), ContentType: contentType,
		Duration: duration, Truncated: truncated,
	}, "", nil
}

func (a *RuntimeAdapter) safeFetchHTTPTransport(plan *safeFetchPlan) (*http.Transport, error) {
	baseClient := a.httpClient()
	baseTransport := baseClient.Transport
	if baseTransport == nil {
		baseTransport = http.DefaultTransport
	}
	transport, ok := baseTransport.(*http.Transport)
	if !ok {
		return nil, fmt.Errorf("Web fetch requires an HTTP transport with owner-controlled dialing")
	}
	cloned := transport.Clone()
	cloned.Proxy = nil
	cloned.DialContext = plan.dialContext
	cloned.DialTLSContext = nil
	cloned.DialTLS = nil
	if cloned.TLSClientConfig != nil {
		cloned.TLSClientConfig = cloned.TLSClientConfig.Clone()
		cloned.TLSClientConfig.ServerName = ""
	}
	cloned.DisableKeepAlives = true
	cloned.ForceAttemptHTTP2 = false
	cloned.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
	if cloned.MaxResponseHeaderBytes <= 0 || cloned.MaxResponseHeaderBytes > 1024*1024 {
		cloned.MaxResponseHeaderBytes = 1024 * 1024
	}
	return cloned, nil
}

type webTransportError struct {
	code    string
	message string
	cause   error
}

func newWebTransportError(code string, message string, cause error) error {
	return &webTransportError{code: strings.TrimSpace(code), message: strings.TrimSpace(message), cause: cause}
}

func (e *webTransportError) Error() string {
	if e == nil || e.message == "" {
		return "Web transport failed"
	}
	return e.message
}

func (e *webTransportError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func webFetchErrorCode(err error) string {
	var governanceRequired *webFetchGovernanceRequiredError
	if errors.As(err, &governanceRequired) {
		return webcommon.ErrorApprovalRequired
	}
	var transportError *webTransportError
	if errors.As(err, &transportError) && transportError.code != "" {
		return transportError.code
	}
	if errors.Is(err, common.ErrConflict) || errors.Is(err, common.ErrInvalidArgument) {
		return webcommon.ErrorPolicyDenied
	}
	return webcommon.ErrorBadGatewayResponse
}

func boundedFetchBytes(request RuntimeOperationRequest, provider webmodels.ProviderConfig) int64 {
	maxBytes := request.MaxBytes
	if maxBytes <= 0 {
		maxBytes = provider.MaxFetchBytes
	}
	return boundedInt64(maxBytes, webproviderconfig.DefaultMaxFetchBytes, minPositive64(provider.MaxFetchBytes, webproviderconfig.SystemMaxFetchBytes))
}
