package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	webcommon "github.com/muidea/workorch/internal/modules/blocks/web/pkg/common"
	webmodels "github.com/muidea/workorch/internal/modules/blocks/web/pkg/models"
	webproviderconfig "github.com/muidea/workorch/internal/modules/blocks/web/pkg/providerconfig"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (a *RuntimeAdapter) doProviderRequest(ctx context.Context, provider webmodels.ProviderConfig, endpoint string, payload map[string]any, maxResponseBytes int64) ([]byte, time.Duration, string, error) {
	if maxResponseBytes <= 0 || maxResponseBytes > webproviderconfig.SystemMaxFetchBytes {
		maxResponseBytes = webproviderconfig.SystemMaxFetchBytes
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, webcommon.ErrorInvalidRequest, fmt.Errorf("%w: encode bounded Web provider request", common.ErrInvalidArgument)
	}
	parsedEndpoint, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || parsedEndpoint.User != nil || parsedEndpoint.Opaque != "" || parsedEndpoint.Hostname() == "" ||
		(parsedEndpoint.Scheme != "http" && parsedEndpoint.Scheme != "https") || len(parsedEndpoint.String()) > webURLMaxBytes {
		return nil, 0, webcommon.ErrorProviderUnavailable, fmt.Errorf("%w: Web provider endpoint is invalid", common.ErrInvalidArgument)
	}
	callCtx, cancel := context.WithTimeout(ctx, providerTimeout(provider))
	defer cancel()
	req, err := http.NewRequestWithContext(callCtx, http.MethodPost, parsedEndpoint.String(), bytes.NewReader(body))
	if err != nil {
		return nil, 0, webcommon.ErrorInvalidRequest, fmt.Errorf("%w: Web provider request is invalid", common.ErrInvalidArgument)
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
	client := a.providerHTTPClient(provider, parsedEndpoint)
	resp, err := client.Do(req)
	duration := time.Since(startedAt)
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		if errors.Is(callCtx.Err(), context.DeadlineExceeded) {
			return nil, duration, webcommon.ErrorTimeout, newWebTransportError(webcommon.ErrorTimeout, "Web provider request timed out", context.DeadlineExceeded)
		}
		return nil, duration, webcommon.ErrorBadGatewayResponse, newWebTransportError(webcommon.ErrorBadGatewayResponse, "Web provider transport failed", err)
	}
	defer resp.Body.Close()
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if readErr != nil {
		return nil, duration, webcommon.ErrorBadGatewayResponse, newWebTransportError(webcommon.ErrorBadGatewayResponse, "Web provider response read failed", readErr)
	}
	if int64(len(raw)) > maxResponseBytes {
		return nil, duration, webcommon.ErrorContentTooLarge, fmt.Errorf("%w: web provider response exceeds max bytes", common.ErrInvalidArgument)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, duration, webcommon.ErrorBadGatewayResponse, fmt.Errorf("web provider returned status %d", resp.StatusCode)
	}
	return raw, duration, "", nil
}

func providerTimeout(provider webmodels.ProviderConfig) time.Duration {
	if provider.Timeout <= 0 {
		return webproviderconfig.DefaultTimeout
	}
	if provider.Timeout > webproviderconfig.SystemMaxTimeout {
		return webproviderconfig.SystemMaxTimeout
	}
	return provider.Timeout
}

func (a *RuntimeAdapter) providerHTTPClient(provider webmodels.ProviderConfig, endpoint *url.URL) *http.Client {
	base := a.httpClient()
	client := *base
	baseRedirect := base.CheckRedirect
	maxRedirects := boundedInt(provider.MaxRedirects, webproviderconfig.DefaultMaxRedirects, webproviderconfig.SystemMaxRedirects)
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if request == nil || request.URL == nil || len(via) > maxRedirects {
			return fmt.Errorf("Web provider redirect limit exceeded")
		}
		if !sameWebProviderOrigin(endpoint, request.URL) {
			return fmt.Errorf("Web provider cross-origin redirect is not allowed")
		}
		if baseRedirect != nil {
			return baseRedirect(request, via)
		}
		return nil
	}
	return &client
}

func sameWebProviderOrigin(left *url.URL, right *url.URL) bool {
	if left == nil || right == nil || !strings.EqualFold(left.Scheme, right.Scheme) || !strings.EqualFold(left.Hostname(), right.Hostname()) {
		return false
	}
	return webURLPort(left) == webURLPort(right)
}

func webURLPort(value *url.URL) string {
	if value == nil {
		return ""
	}
	if port := value.Port(); port != "" {
		return port
	}
	if strings.EqualFold(value.Scheme, "https") {
		return "443"
	}
	return "80"
}

func (a *RuntimeAdapter) httpClient() *http.Client {
	if a != nil && a.client != nil {
		return a.client
	}
	return http.DefaultClient
}

func reservedHeader(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "content-type", "accept", "user-agent":
		return true
	default:
		return false
	}
}
