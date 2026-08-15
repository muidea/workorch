package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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

func providerTimeout(provider webmodels.ProviderConfig) time.Duration {
	if provider.Timeout <= 0 {
		return webproviderconfig.DefaultTimeout
	}
	return provider.Timeout
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
