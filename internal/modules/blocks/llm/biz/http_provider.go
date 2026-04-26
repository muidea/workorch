package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

const maxProviderErrorBodyBytes = 4096

type HTTPProvider struct {
	config llmmodels.ProviderConfig
	client *http.Client
}

func NewHTTPProvider(config llmmodels.ProviderConfig) *HTTPProvider {
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
	return &HTTPProvider{config: config, client: client}
}

func (p *HTTPProvider) Complete(ctx context.Context, request llmmodels.ProviderRequest) ([]byte, error) {
	if p == nil || p.client == nil {
		return nil, common.ErrInvalidArgument
	}
	endpoint, err := chatCompletionsURL(p.config.BaseURL)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(chatCompletionRequest{
		Model: p.config.Model,
		Messages: []chatCompletionMessage{
			{Role: "system", Content: systemPrompt()},
			{Role: "user", Content: userPrompt(request)},
		},
		Temperature:    0,
		ResponseFormat: map[string]string{"type": "json_object"},
	})
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.config.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxProviderErrorBodyBytes))
		return nil, fmt.Errorf("%w: llm provider http status %d: %s", common.ErrInvalidArgument, resp.StatusCode, sanitizeProviderBody(body, p.config.APIKey))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var decoded chatCompletionResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("%w: invalid llm provider response: %v", common.ErrInvalidArgument, err)
	}
	content := strings.TrimSpace(decoded.FirstContent())
	if content == "" {
		return nil, fmt.Errorf("%w: llm provider response content is empty", common.ErrInvalidArgument)
	}
	return []byte(content), nil
}

type chatCompletionRequest struct {
	Model          string                  `json:"model"`
	Messages       []chatCompletionMessage `json:"messages"`
	Temperature    float64                 `json:"temperature"`
	ResponseFormat map[string]string       `json:"response_format,omitempty"`
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
}

func (r chatCompletionResponse) FirstContent() string {
	if len(r.Choices) == 0 {
		return ""
	}
	return r.Choices[0].Message.Content
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
	trimmedPath := strings.TrimRight(parsed.Path, "/")
	if strings.HasSuffix(trimmedPath, "/chat/completions") {
		parsed.Path = trimmedPath
	} else {
		parsed.Path = trimmedJoin(trimmedPath, "chat/completions")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func trimmedJoin(basePath string, child string) string {
	if basePath == "" {
		return "/" + child
	}
	return basePath + "/" + child
}

func systemPrompt() string {
	return strings.Join([]string{
		"You are the workorch workspace execution planner.",
		"Return only one JSON object matching schema workorch.llmExecutionDecision.v1.",
		"Do not wrap the JSON in markdown.",
		"Do not execute tools directly. Request shell, patch, MCP, Decision, Approval, Memory, DoD assessment, or ExecutionGraph through the JSON fields only.",
		"The word json is intentional because some providers require it when response_format is json_object.",
	}, "\n")
}

func userPrompt(request llmmodels.ProviderRequest) string {
	body, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		body = []byte("{}")
	}
	if request.Repair != nil {
		return strings.Join([]string{
			"Repair the invalid workorch.llmExecutionDecision.v1 json output.",
			"Return only the corrected JSON object. Do not change the business intent except where required to satisfy the schema.",
			decisionContractPrompt(),
			"",
			"Invalid provider request and repair context:",
			string(body),
		}, "\n")
	}
	return strings.Join([]string{
		"Plan the next workspace execution step from this provider request.",
		decisionContractPrompt(),
		"",
		string(body),
	}, "\n")
}

func decisionContractPrompt() string {
	return strings.Join([]string{
		"Required output JSON fields use the LLMExecutionDecision contract:",
		"- completionState must be one of: continue, blocked_on_decision, blocked_on_approval, completed, failed.",
		"- Use executionGraph when the next work contains multiple blocks, dependencies, or future parallel-ready work.",
		"- executionGraph.nodes items require id and kind; dependsOn is a list of node ids; input carries block input.",
		"- Use shellCommands, patchRequests, mcpToolCalls, requiredDecision, requiredApproval, memoryWrites, or dodAssessment as appropriate.",
		"- For completed, include dodAssessment with evidenceRefs for every satisfied item.",
		"- Do not include workspaceId, runId, stepId, schema, traceRefs, config, or any unknown top-level field in the decision JSON.",
		"Minimal examples:",
		`{"completionState":"continue","executionGraph":{"nodes":[{"id":"inspect","kind":"shell/command","input":{"command":"ls","args":["-la"]}},{"id":"test","kind":"shell/command","dependsOn":["inspect"],"input":{"command":"go","args":["test","./..."]}}]}}`,
		`{"completionState":"blocked_on_decision","requiredDecision":{"reason":"missing product choice","decisionKind":"provide_missing_info","question":"Which runtime adapter should be used?"}}`,
		`{"completionState":"completed","dodAssessment":{"satisfied":true,"items":[{"item":"tests pass","state":"satisfied","evidenceRefs":["run-1/validation/runtime-step"]}]}}`,
	}, "\n")
}

func sanitizeProviderBody(body []byte, secret string) string {
	text := strings.TrimSpace(string(body))
	if text == "" {
		return http.StatusText(http.StatusBadGateway)
	}
	if secret = strings.TrimSpace(secret); secret != "" {
		text = strings.ReplaceAll(text, secret, RedactSecret(secret))
	}
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, "\r", " ")
	if len(text) > maxProviderErrorBodyBytes {
		return text[:maxProviderErrorBodyBytes]
	}
	return text
}
