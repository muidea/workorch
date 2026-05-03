package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

type Option func(*Client)

func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		c.httpClient.Timeout = d
	}
}

func New(baseURL string, opts ...Option) *Client {
	c := &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

func (c *Client) BaseURL() string {
	return c.baseURL
}

func (c *Client) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	url := c.baseURL + path

	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}

	if resp.StatusCode >= 400 {
		var errResp struct {
			Error string `json:"error"`
		}
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		resp.Body.Close()
		if json.Unmarshal(bodyBytes, &errResp) == nil && errResp.Error != "" {
			return nil, &APIError{StatusCode: resp.StatusCode, Message: errResp.Error}
		}
		return nil, &APIError{StatusCode: resp.StatusCode, Message: string(bodyBytes)}
	}

	return resp, nil
}

func decodeEnvelope[T any](resp *http.Response, key string) (*T, error) {
	defer resp.Body.Close()

	var raw map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode response envelope: %w", err)
	}

	dataBytes, ok := raw[key]
	if !ok {
		return nil, fmt.Errorf("response key %q not found in envelope", key)
	}

	var result T
	if err := json.Unmarshal(dataBytes, &result); err != nil {
		return nil, fmt.Errorf("decode resource from key %q: %w", key, err)
	}
	return &result, nil
}

func decodeEnvelopeSlice[T any](resp *http.Response, key string) ([]T, error) {
	defer resp.Body.Close()

	var raw map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode response envelope: %w", err)
	}

	dataBytes, ok := raw[key]
	if !ok {
		return nil, fmt.Errorf("response key %q not found in envelope", key)
	}

	var result []T
	if err := json.Unmarshal(dataBytes, &result); err != nil {
		return nil, fmt.Errorf("decode slice from key %q: %w", key, err)
	}
	return result, nil
}
