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

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

const maxProviderErrorBodyBytes = 4096
const maxProviderStreamLineBytes = 1024 * 1024

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

func (p *HTTPProvider) StreamComplete(ctx context.Context, request llmmodels.ProviderRequest, emit func(llmmodels.ProviderStreamEvent) error) ([]byte, error) {
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
		Stream:         true,
	})
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
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
	content, err := readChatCompletionStream(resp.Body, emit)
	if err != nil {
		return nil, err
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, fmt.Errorf("%w: llm provider streamed response content is empty", common.ErrInvalidArgument)
	}
	return []byte(content), nil
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
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return llmmodels.ProviderEmbeddingResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.config.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return llmmodels.ProviderEmbeddingResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxProviderErrorBodyBytes))
		return llmmodels.ProviderEmbeddingResult{}, fmt.Errorf("%w: llm provider http status %d: %s", common.ErrInvalidArgument, resp.StatusCode, sanitizeProviderBody(body, p.config.APIKey))
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

type chatCompletionRequest struct {
	Model          string                  `json:"model"`
	Messages       []chatCompletionMessage `json:"messages"`
	Temperature    float64                 `json:"temperature"`
	ResponseFormat map[string]string       `json:"response_format,omitempty"`
	Stream         bool                    `json:"stream,omitempty"`
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
	trimmedPath := strings.TrimRight(parsed.Path, "/")
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

func readChatCompletionStream(reader io.Reader, emit func(llmmodels.ProviderStreamEvent) error) (string, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), maxProviderStreamLineBytes)
	var builder strings.Builder
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
					return "", err
				}
			}
			return builder.String(), nil
		}
		var chunk chatCompletionStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return "", fmt.Errorf("%w: invalid llm provider stream chunk: %v", common.ErrInvalidArgument, err)
		}
		for _, choice := range chunk.Choices {
			delta := choice.Delta.Content
			if delta == "" {
				delta = choice.Message.Content
			}
			if delta == "" {
				continue
			}
			builder.WriteString(delta)
			if emit != nil {
				if err := emit(llmmodels.ProviderStreamEvent{Type: "delta", Delta: delta}); err != nil {
					return "", err
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return builder.String(), nil
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
		"Do not invent a workorch CLI. workorch is a service exposed through HTTP APIs and runtime blocks; there is no shell command named workorch unless the provider request explicitly lists it as available.",
		"Use only runtime block kinds and service APIs declared in runtimeCapabilities. Do not invent local service endpoints.",
		"Use the provider request dialogLanguage for every user-facing text value, while keeping JSON field names and schema enum values unchanged.",
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
			dialogLanguageInstruction(request),
			decisionContractPrompt(),
			"",
			"Invalid provider request and repair context:",
			string(body),
		}, "\n")
	}
	return strings.Join([]string{
		"Plan the next workspace execution step from this provider request.",
		dialogLanguageInstruction(request),
		decisionContractPrompt(),
		"",
		string(body),
	}, "\n")
}

func dialogLanguageInstruction(request llmmodels.ProviderRequest) string {
	language := strings.TrimSpace(request.DialogLanguage)
	if language == "" {
		language = "English"
	}
	return fmt.Sprintf("Conversation language: %s. Use this language for all user-facing text, including thoughtSummary, plan entries, decision questions, approval reasons, memory summaries, DoD item labels, DoD summaries, result summaries, and artifact-facing summaries. Keep JSON keys, enum values, block kinds, commands, file paths, identifiers, and evidence refs unchanged.", language)
}

func decisionContractPrompt() string {
	return strings.Join([]string{
		"Required output JSON fields use the LLMExecutionDecision contract:",
		"- completionState must be one of: continue, blocked_on_decision, blocked_on_approval, completed, failed.",
		"- Use executionGraph when the next work contains multiple blocks, dependencies, or future parallel-ready work.",
		"- executionGraph.nodes items require id and kind; dependsOn is a list of node ids; input carries block input.",
		"- executionGraph.nodes kind values must be selected from runtimeCapabilities.blockKinds only.",
		"- Use executionGraph node kind patch/apply for file writes or diffs that must participate in graph dependencies. Do not use patchRequests as an executionGraph node kind.",
		"- Use shellCommands, patchRequests, mcpToolCalls, requiredDecision, requiredApproval, memoryWrites, or dodAssessment as appropriate.",
		"- Top-level patchRequests are immediate patch side effects of the current LLM step and may use only patchType, targetRef, content, diff, and summary. patchType must be file_write or unified_diff. Do not include an operation field.",
		"- Shell command names must be real commands available in the local process environment. Do not use a command named workorch.",
		"- Shell command arguments must be encoded only as args arrays. For example use {\"command\":\"find\",\"args\":[\".\",\"-maxdepth\",\"3\"]}; never place positional arguments as object properties after command.",
		"- shell/command.command must contain only the executable name or path. For shell syntax such as redirection, heredoc, cd, pipes, &&, ||, or multi-line scripts, use {\"command\":\"sh\",\"args\":[\"-c\",\"...\"]} or use patchRequests to write files.",
		"- Do not use commands listed in runtimeCapabilities.shellPolicy.unavailableCommands, including inside sh -c, bash -c, or other nested shell scripts.",
		"- Shell command input may include timeoutMillis for commands expected to take longer than the default; keep commands non-interactive and bounded.",
		"- Do not assume remote host access credentials, SSH users, keys, or passwords. If a goal requires remote host configuration and no access method is present in the request, return blocked_on_decision with decisionKind provide_missing_info and ask for the access method or credentials.",
		"- If input.executionContext.governanceHistory contains resolved decisions, treat their resolutionPayload as confirmed workspace context. Do not ask the same question again; continue from that confirmed decision unless it conflicts with the current Goal or DoD.",
		"- When remote SSH credentials are explicitly available, prefer the ssh/command block kind from runtimeCapabilities.blockKinds. Do not emulate password SSH with sshpass or expect through shell/command.",
		"- ssh/command input supports host, port, username, password, privateKey, privateKeyPath, command, and timeoutMillis. Keep the remote command non-interactive and bounded.",
		"- Shell commands may call local workorchd HTTP only through runtimeCapabilities.serviceAPIs. Do not use /runs, /observability, /metrics?run=..., or any endpoint that is not explicitly declared.",
		"- When input.runtimeRepairFollowUp is true, use input.failedStepInput, input.failedStepOutput, failureReason, runtimeDiagnostics, graphEvidence, workspaceFileManifest, and completed graph nodes as repair evidence. Plan the smallest safe patch or command sequence that fixes the failure, then rerun the failed validation or an equivalent verification step.",
		"- If runtimeDiagnostics.kind is workspace_path_conflict, the conflictRef already exists as a file and must not be used as a directory. Choose a different targetRef or write/update files at the existing project root.",
		"- For software development repair, if Go, Node, Python, or similar dependency metadata changed or a dependency checksum/lock file is missing, include the appropriate bounded dependency preparation command such as go mod tidy, npm install, or pip install before rerunning tests. Do not hardcode this unless the workspace files indicate the ecosystem.",
		"- When input.graphFollowUp is true, use input.graphEvidence as the collected execution graph evidence. Return completed with dodAssessment when the DoD can be assessed; request more work only when evidence is insufficient.",
		"- When input.completionFollowUp is true, use input.previousStepOutput as the latest execution evidence. Return completed with dodAssessment when the DoD can be assessed; otherwise plan only the missing verification work.",
		"- For completed, include dodAssessment.summary as a non-empty user-facing final result, and evidenceRefs for every satisfied item.",
		"- Do not include workspaceId, runId, stepId, schema, traceRefs, config, or any unknown top-level field in the decision JSON.",
		"Minimal examples:",
		`{"completionState":"continue","executionGraph":{"nodes":[{"id":"inspect","kind":"shell/command","input":{"command":"ls","args":["-la"]}},{"id":"test","kind":"shell/command","dependsOn":["inspect"],"input":{"command":"go","args":["test","./..."]}}]}}`,
		`{"completionState":"blocked_on_decision","requiredDecision":{"reason":"missing product choice","decisionKind":"provide_missing_info","question":"Which runtime adapter should be used?"}}`,
		`{"completionState":"completed","dodAssessment":{"satisfied":true,"summary":"Tests passed and the requested artifact is ready.","items":[{"item":"tests pass","state":"satisfied","evidenceRefs":["run-1/validation/runtime-step"]}]}}`,
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
