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
const maxProviderStreamLineBytes = 16 * 1024 * 1024

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

func (p *HTTPProvider) Complete(ctx context.Context, request llmmodels.ProviderRequest) (llmmodels.ProviderCompletionResult, error) {
	if p == nil || p.client == nil {
		return llmmodels.ProviderCompletionResult{}, common.ErrInvalidArgument
	}
	endpoint, err := chatCompletionsURL(p.config.BaseURL)
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	payload, err := json.Marshal(chatCompletionRequest{
		Model:               p.config.Model,
		Messages:            chatMessagesForRequest(&request),
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
	httpReq.Header.Set("Authorization", "Bearer "+p.config.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxProviderErrorBodyBytes))
		return llmmodels.ProviderCompletionResult{}, fmt.Errorf("%w: llm provider http status %d: %s", common.ErrInvalidArgument, resp.StatusCode, sanitizeProviderBody(body, p.config.APIKey))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	var decoded chatCompletionResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return llmmodels.ProviderCompletionResult{}, fmt.Errorf("%w: invalid llm provider response: %v", common.ErrInvalidArgument, err)
	}
	content := strings.TrimSpace(decoded.FirstContent())
	if content == "" {
		return llmmodels.ProviderCompletionResult{}, fmt.Errorf("%w: llm provider response content is empty", common.ErrInvalidArgument)
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
	endpoint, err := chatCompletionsURL(p.config.BaseURL)
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	payload, err := json.Marshal(chatCompletionRequest{
		Model:               p.config.Model,
		Messages:            chatMessagesForRequest(&request),
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
	httpReq.Header.Set("Authorization", "Bearer "+p.config.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxProviderErrorBodyBytes))
		return llmmodels.ProviderCompletionResult{}, fmt.Errorf("%w: llm provider http status %d: %s", common.ErrInvalidArgument, resp.StatusCode, sanitizeProviderBody(body, p.config.APIKey))
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
	Model               string                         `json:"model"`
	Messages            []chatCompletionMessage        `json:"messages"`
	Temperature         float64                        `json:"temperature"`
	MaxCompletionTokens int64                          `json:"max_completion_tokens,omitempty"`
	ReasoningEffort     string                         `json:"reasoning_effort,omitempty"`
	Thinking            *compatibleThinkingConfigParam `json:"thinking,omitempty"`
	ResponseFormat      map[string]string              `json:"response_format,omitempty"`
	Stream              bool                           `json:"stream,omitempty"`
	StreamOptions       *chatCompletionStreamOptions   `json:"stream_options,omitempty"`
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
	Usage chatCompletionUsage `json:"usage"`
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
	Usage chatCompletionUsage `json:"usage"`
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

func readChatCompletionStream(reader io.Reader, emit func(llmmodels.ProviderStreamEvent) error) (string, *llmmodels.ProviderUsage, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), maxProviderStreamLineBytes)
	var builder strings.Builder
	var usage *llmmodels.ProviderUsage
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
		var chunk chatCompletionStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return "", nil, fmt.Errorf("%w: invalid llm provider stream chunk: %v", common.ErrInvalidArgument, err)
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

func (u chatCompletionUsage) ProviderUsage() *llmmodels.ProviderUsage {
	input := max(u.PromptTokens, u.InputTokens)
	output := max(u.CompletionTokens, u.OutputTokens)
	total := u.TotalTokens
	if total == 0 && (input > 0 || output > 0) {
		total = input + output
	}
	if input == 0 && output == 0 && total == 0 {
		return nil
	}
	return &llmmodels.ProviderUsage{
		InputTokens:             input,
		OutputTokens:            output,
		TotalTokens:             total,
		CacheReadTokens:         maxPositiveInt(u.PromptCacheHitTokens, u.CacheReadInputTokens, u.CachedTokens, u.InputCachedTokens, u.InputCacheReadTokens),
		CacheWriteTokens:        maxPositiveInt(u.PromptCacheMissTokens, u.CacheCreationInputTokens, u.InputCacheWriteTokens),
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
		return []chatCompletionMessage{
			{Role: "system", Content: systemPrompt(false)},
			{Role: "user", Content: "{}"},
		}
	}
	build := buildProviderPromptMessages(*request)
	messages := make([]chatCompletionMessage, 0, len(build.Messages))
	for _, item := range build.Messages {
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
		request.ContextBudget.NormalizedSections = uniqueStrings(append(request.ContextBudget.NormalizedSections, build.NormalizedSections...))
		request.ContextBudget.IncludedSections = uniqueStrings(append(request.ContextBudget.IncludedSections, defaultPromptIncludedSections(messages)...))
	}
	return messages
}

func defaultPromptIncludedSections(messages []chatCompletionMessage) []string {
	if len(messages) == 0 {
		return nil
	}
	out := make([]string, 0, len(messages))
	for _, item := range messages {
		out = append(out, item.Role)
	}
	return out
}

func systemPrompt(assessmentOnly bool) string {
	lines := []string{
		"You are the workorch workspace execution planner.",
		"Return only one JSON object matching schema workorch.llmExecutionDecision.v1.",
		"Do not wrap the JSON in markdown.",
		"Do not invent a workorch CLI. workorch is a service exposed through HTTP APIs and runtime blocks; there is no shell command named workorch unless the provider request explicitly lists it as available.",
		"Use the provider request dialogLanguage for every user-facing text value, while keeping JSON field names and schema enum values unchanged.",
		"The word json is intentional because some providers require it when response_format is json_object.",
	}
	if assessmentOnly {
		lines = append(lines,
			"Assessment-only mode is active: do not return executionGraph, shellCommands, patchRequests, mcpToolCalls, memoryWrites, nextAction, or any executable work.",
			"Return only dodAssessment with completed or failed, or requiredDecision/requiredApproval for a real governance blocker.",
		)
	} else {
		lines = append(lines,
			"Do not execute tools directly. Request shell, patch, MCP, Decision, Approval, Memory, DoD assessment, or ExecutionGraph through the JSON fields only.",
			"Use only runtime block kinds and service APIs declared in runtimeCapabilities. Do not invent local service endpoints.",
		)
	}
	return strings.Join(lines, "\n")
}

func systemPromptForRequest(request llmmodels.ProviderRequest) string {
	if isDecisionSchemaRequest(request) {
		return systemPrompt(false)
	}
	return strings.Join([]string{
		"You are a WorkOrch structured JSON generation assistant.",
		fmt.Sprintf("Return only one JSON object matching schema %s.", schemaNameForPrompt(request)),
		"Do not wrap the JSON in markdown.",
		"Follow the provider request input instructions and output schema.",
		"Do not plan runtime execution or return workorch.llmExecutionDecision.v1 fields unless the requested schema explicitly defines them.",
		"The word json is intentional because some providers require it when response_format is json_object.",
	}, "\n")
}

func userPrompt(request llmmodels.ProviderRequest) string {
	body, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		body = []byte("{}")
	}
	if !isDecisionSchemaRequest(request) {
		if request.Repair != nil {
			return strings.Join([]string{
				fmt.Sprintf("Repair the invalid JSON output for schema %s.", schemaNameForPrompt(request)),
				"Return only the corrected JSON object. Do not change the business intent except where required to satisfy the requested schema.",
				structuredJSONLanguageInstruction(request),
				structuredJSONContractPrompt(request),
				"",
				"Invalid provider request and repair context:",
				string(body),
			}, "\n")
		}
		return strings.Join([]string{
			fmt.Sprintf("Generate or revise structured JSON for kind %s.", kindNameForPrompt(request)),
			structuredJSONLanguageInstruction(request),
			structuredJSONContractPrompt(request),
			"",
			string(body),
		}, "\n")
	}
	if request.Repair != nil {
		return strings.Join([]string{
			"Repair the invalid workorch.llmExecutionDecision.v1 json output.",
			"Return only the corrected JSON object. Do not change the business intent except where required to satisfy the schema.",
			dialogLanguageInstruction(request),
			decisionContractPrompt(request),
			capabilityContractPrompt(request),
			"",
			"Invalid provider request and repair context:",
			string(body),
		}, "\n")
	}
	return strings.Join([]string{
		"Plan the next workspace execution step from this provider request.",
		dialogLanguageInstruction(request),
		decisionContractPrompt(request),
		capabilityContractPrompt(request),
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

func decisionContractPrompt(request llmmodels.ProviderRequest) string {
	lines := []string{
		"Required output JSON fields use the LLMExecutionDecision contract:",
		"- completionState must be one of: continue, blocked_on_decision, blocked_on_approval, completed, failed.",
		"- Allowed top-level JSON fields are only: completionState, thoughtSummary, plan, executionGraph, nextAction, requiredDecision, requiredApproval, patchRequests, mcpToolCalls, shellCommands, memoryWrites, dodAssessment.",
		"- Do not return unknown top-level fields such as validationPlan, testPlan, implementationPlan, verificationPlan, workspaceId, runId, stepId, schema, traceRefs, or config. Put advisory planning text into plan or thoughtSummary; put executable validation/check work into executionGraph or shellCommands when executable work is allowed.",
		"- When executable work is allowed, use executionGraph when the next work contains multiple blocks, dependencies, or future parallel-ready work.",
		"- executionGraph.nodes items require id and kind; dependsOn is a list of node ids; input carries block input.",
		"- executionGraph.nodes kind values must be selected from runtimeCapabilities.blockKinds only.",
		"- Sensitive paths and large patches require Approval before apply. After approval is granted, include the approved approvalId in patch/apply or patchRequests.",
		"- Use shellCommands, patchRequests, mcpToolCalls, requiredDecision, requiredApproval, memoryWrites, or dodAssessment as appropriate.",
		"- If nextAction is used, it must be an object such as {\"kind\":\"continue_run\",\"summary\":\"...\",\"input\":{}}; do not return nextAction as a string.",
		"- If the current workspace is empty and the effective Goal/DoD can be satisfied by creating workspace deliverables with available runtime capabilities, return continue with an executionGraph that produces those deliverables; do not ask whether initialization is allowed.",
		"- For blocked_on_decision, include requiredDecision.context whenever known values, missing fields, or safe candidate values exist. Use neutral structured keys: knownValues, missingFields, candidateValues, confidenceByField, requiresHumanFields, runtimeCanContinue. Do not put domain-specific policy in field names beyond the fields needed by the current task.",
		"- Set requiredDecision.context.runtimeCanContinue=true only when the runtime can proceed safely without human input; otherwise leave it false or omit it.",
		"- Auto governance may resolve only high-confidence structured candidates. Put fields that still require explicit human input in requiresHumanFields instead of guessing.",
		"- Top-level patchRequests are immediate patch side effects of the current LLM step and may use only patchType, targetRef, content, diff, oldText, newText, occurrence, summary, baseHash, coverageTargets, and approvalId. patchType must be file_write, unified_diff, or text_replace. Do not include an operation field.",
		"- Patch content, diffs, and exact text replacements must be complete, concrete, and directly applicable to the current workspace content. Do not return omitted sections, placeholder bodies, pseudo patches, or comment-only stand-ins for required work. If exact current content is unknown, read the file first or use file/edit with exact oldText.",
		"- unified_diff patches must include explicit hunk line ranges such as @@ -12,3 +12,4 @@. Never emit a bare @@ hunk. If exact line ranges are uncertain, use text_replace with exact oldText/newText/baseHash, file_write with a safe complete baseHash-backed snapshot, file/edit with exact oldText/newText, or read the target first.",
		"- baseHash must be omitted or copied as a literal sha256 contentHash from current file/read, patchRepairContext, or failureTargetRepairContext evidence. Do not put unresolved references such as ${...}, {{...}}, or symbolic placeholders in baseHash.",
		"- Before adding target content during repair, reconcile the current target snapshot and modify an existing equivalent item instead of creating duplicate named artifacts.",
		"- patch/apply and patchRequests do not support file deletion. Do not represent deletion as file_write with empty content; use a supported non-empty replacement or unified_diff change, or return failed/blocker when no supported state change can satisfy the Goal/DoD.",
		"- Do not use patch/apply or patchRequests to create report-only deliverables under delivery/, .workorch-data/, or other WorkOrch-managed artifact paths. Do not write governance/report files such as decision_record.md, approval_record.md, delivery_summary.md, result_summary.md, final_summary.md, deliverable_manifest.md, or evidence_chain.md into the workspace. For final user-facing results, place the result in dodAssessment and let the runtime record artifacts.",
		"- The effective Goal and DoD in input.executionContextPackage or input.executionContext are authoritative. Treat explicit constraints in them as hard constraints. Do not continue a previous plan, tool choice, validation command, or assumption that conflicts with the current Goal, DoD, or resolved governance decisions.",
		"- When input.definitionChangeImpact, input.executionContext.definitionChangeImpact, or input.executionContextPackage.definitionChangeImpact has changed=true, treat Goal/DoD definitions as cumulative. Before state-changing work, identify additive constraints, still-valid prior constraints, conflicts, and the neutral strategy for artifact, evidence, and verification targets.",
		"- Do not satisfy a definition change by weakening prior Goal/DoD constraints, removing still-valid acceptance evidence, or reducing the requested outcome. If constraints conflict, return a failed assessment or real governance blocker.",
		"- If input.decisionResolution, input.governanceHistory, input.executionContext.governanceHistory, or input.executionContextPackage.governanceHistory contains resolved decisions, treat their resolutionPayload as confirmed workspace context. Do not ask the same question again; continue from that confirmed decision unless it conflicts with the current Goal or DoD.",
		"- When input.runtimeRepairFollowUp is true, use input.failedStepInput, input.failedStepOutput, input.failedExecutionGraph, failureReason, runtimeDiagnostics, graphEvidence, workspaceFileManifest, patchRepairContext, and completed graph nodes as repair evidence. Reconcile that evidence with the current effective Goal, DoD, and resolved decisions before producing a new executionGraph; do not copy a failed graph unless it still matches all current constraints. Plan the smallest safe patch or command sequence that fixes the failure, then rerun the failed validation or an equivalent verification step.",
		"- When input.repairActionPackage.requiredOutput.mustDeclareStrategyReassessmentFields is true, put failedSurface, rootCauseTarget, repairTargetDecision, repairMode, forbiddenRepair, and verificationPlan directly inside each state-changing patch/apply node input. Do not return top-level strategyReassessment, and do not put strategyReassessment as a sibling of node.input.",
		"- In scoped repair, verification after a patch must be bounded to the failed surface or target. Either put targetRef, relatedFailureTargets, verificationScope, or boundedReason on shell/command input, or make the shell/command depend on a patch/apply node that directly targets the failure target.",
		"- When repairing a patch/preview or patch/apply failure, treat the failed diff as untrusted repair evidence. Prefer input.patchRepairContext current target snapshots, input.patchRepairSuggestions, or a fresh file/read result; preserve source text exactly and generate the smallest current file/edit, patch/preview, patch/apply, or patchRequests repair.",
		"- When patchRepairContext is present because a patch failed with a hunk/context/base mismatch, prefer patch/apply text_replace with exact oldText/newText/baseHash from input.patchRepairSuggestions or the current snapshot. Use complete patch/apply file_write only when the snapshot is marked safeForFileWrite/contentComplete and contains no redaction markers. Otherwise use a minimal unified_diff with exact visible context, file/edit with exact oldText, or first read the target.",
		"- Never copy [REDACTED], [SECRET], [MASKED], omitted sections, or truncated snapshot text into patch/apply content or diff.",
		"- When input.failureTargetRepairContext is present, treat those current target snapshots as the primary repair targets identified by failed evidence. Update those targets directly unless current Goal/DoD proves another target is required.",
		"- When runtime repair evidence includes concrete compile/build/test diagnostics such as undefined identifiers, unknown fields, cannot-use type mismatches, missing methods, constructor/schema mismatches, or line-referenced target errors, resolve those reported blockers in the identified target before secondary assertion tuning, formatting changes, or other opportunistic cleanup.",
		"- If a failing target already has a current snapshot in failureTargetRepairContext or patchRepairContext, reconcile the target against current visible API/resource definitions first; do not invent replacement names or broad new scaffolding unless the Goal explicitly requires a new definition.",
		"- When input.failureClass is dod_coverage_missing or input.missingDoDItems is non-empty, produce a DoD-gap repair plan that maps each missing item to concrete affected resources, state-changing operations, and verification actions with expected evidence. Do not return generic inspection-only loops when state-changing or verification gaps are already explicit.",
		"- If runtimeDiagnostics.kind is workspace_path_conflict, the conflictRef already exists as a workspace resource and must not be used as a directory. Choose a different targetRef or write/update resources at the existing workspace root.",
		"- If workspace evidence shows that prerequisite metadata, derived indexes, or similar supporting resources changed or are missing, include the appropriate bounded preparation step inferred from workspace resources and runtimeCapabilities before rerunning verification. Do not infer an ecosystem from the task category alone.",
		"- When input.graphFollowUp is true, use input.graphEvidence as the current execution graph evidence and input.runEvidenceSummary as the compact run-level evidence summary. Return completed with dodAssessment when the DoD can be assessed; request more work only when evidence is insufficient.",
		"- Graph follow-up is a closure step for the current execution graph evidence, not an unbounded planning loop. Prefer dodAssessment over adding another executionGraph when verification evidence is already available.",
		"- In strict-convergence graph follow-up or completion follow-up, if the current evidence already identifies specific missing DoD, Goal, acceptance-dimension, or deliverable coverage that can be closed automatically, prefer a bounded executable continuation graph over a descriptive failed assessment with no actionable next step.",
		"- When repairActionPackage.requiredOutput requires concrete validation evidence, do not use static inspection alone to close runtime-behavior coverage. If a dependency, credential, endpoint, port, path, or similar prerequisite is unavailable, first try a neutral controlled substitute through accepted tools, such as a local fixture, stub/mock, temporary config, isolated working directory, or alternate available port. Each evidence-producing node must declare input.validationEvidence.mode as runtime_behavior, controlled_substitute, executable_contract, or dynamic_execution, and input.validationEvidence.observationType as runtime_observation, controlled_substitute_observation, executable_contract_observation, or dynamic_execution_observation. For controlled_substitute, also declare validationEvidence.substituteFor or validationEvidence.substitute.",
		"- If a failed dodAssessment is returned in graph follow-up or completion follow-up, include concrete nextActions that map each unresolved gap to target resources, state-changing operations, and validation actions. Do not stop at a generic gap summary when runtime repair can continue automatically.",
		"- When input.assessmentOnly is true or input.graphExpansionLimitFollowUp is true, do not return executionGraph, patchRequests, shellCommands, or other executable work. Return completed/failed with dodAssessment, or blocked_on_decision/blocked_on_approval only when a real governance blocker remains.",
		"- If DoD is not satisfied in assessment-only mode, include failed/skipped items, evidenceRefs when available, known gaps, impact, residualRisks, and nextActions so the runtime can decide the next repair or replan path.",
		"- When input.completionFollowUp is true, use input.previousStepOutput as the latest execution evidence and input.runEvidenceSummary as the compact run-level evidence summary. Return completed with dodAssessment when the DoD can be assessed; otherwise plan only the missing verification work.",
		"- When input.continuationFollowUp is true, the previous response requested continuation but did not provide an actionable next step. Use the current evidence to either complete with dodAssessment, produce only missing actionable work, request a real decision/approval, or fail with a non-recoverable reason.",
		"- For completed, dodAssessment.items must cover every current effective DoD item from input.executionContextPackage.dod/items or input.executionContext.dod/items. Do not omit DoD items that are not yet satisfied; instead return continue with the missing state-changing or verification work, or mark the item failed/skipped with impact if completion is impossible.",
		"- For completed, if the effective Goal has explicit requirements that are not textually identical to DoD items, include dodAssessment.goalCoverage with item-level Goal requirement coverage and evidenceRefs. Use goalCoverage for assessment mapping only; it is not executable work.",
		"- dodAssessment.goalCoverage, dodAssessment.acceptanceDimensions, and dodAssessment.deliverableCoverage must be objects shaped as {\"satisfied\":boolean,\"items\":[...]}; never return these coverage fields as bare arrays.",
		"- For completed, include dodAssessment.summary as a non-empty user-facing final result, dodAssessment.goalOutcome as a concise one-sentence result (always required), evidenceRefs for every satisfied item, and when known include implementationSummary as a state-change summary, plus verificationSummary, knownLimitations, residualRisks, and nextActions.",
		"- Do not include workspaceId, runId, stepId, schema, traceRefs, config, or any unknown top-level field in the decision JSON.",
		"Minimal examples:",
		`{"completionState":"continue","executionGraph":{"nodes":[{"id":"inspect","kind":"file/list","input":{"path":""}},{"id":"verify","kind":"shell/command","dependsOn":["inspect"],"input":{"command":"sh","args":["-c","printf verified"]}}]}}`,
		`{"completionState":"blocked_on_decision","requiredDecision":{"reason":"missing execution target","decisionKind":"provide_missing_info","question":"Please provide target.","context":{"knownValues":{"source":"workspace"},"missingFields":["target"],"candidateValues":{"target":"artifact"},"confidenceByField":{"target":0.92}}}}`,
		`{"completionState":"completed","dodAssessment":{"satisfied":true,"summary":"The requested outcome is ready and evidenced.","goalOutcome":"The requested outcome is complete.","implementationSummary":"Produced the required state change.","verificationSummary":"Verification evidence passed.","items":[{"item":"required outcome is evidenced","state":"satisfied","evidenceRefs":["run-1/validation/runtime-step"]}],"knownLimitations":[],"residualRisks":[],"nextActions":["Review the generated artifacts."]}}`,
	}
	return strings.Join(lines, "\n")
}

func capabilityContractPrompt(request llmmodels.ProviderRequest) string {
	lines := []string{"Runtime capability contract:"}
	if requestHasBlockKind(request, "file/list") || requestHasBlockKind(request, "file/read") || requestHasBlockKind(request, "file/edit") {
		lines = append(lines, "- Prefer file/list and file/read for workspace file discovery and bounded file inspection. Probe unknown or optional files with file/list first, or set file/read optional:true so a missing file returns exists:false instead of becoming a repair failure. Use file/edit only to generate a non-mutating exact-text replacement patch preview; it does not write files.")
	}
	if requestHasBlockKind(request, "patch/preview") || requestHasBlockKind(request, "patch/apply") || requestHasBlockKind(request, "patch/revert") {
		lines = append(lines, "- Use executionGraph node kind patch/preview to validate and summarize an intended patch without writing files. Use patch/apply for file writes or diffs that must participate in graph dependencies. Use patch/revert only for a WorkOrch patchId when reverting an already applied WorkOrch patch. Do not use patchRequests as an executionGraph node kind.")
	}
	if requestHasBlockKind(request, "git/read") || requestHasBlockKind(request, "git/write") {
		lines = append(lines,
			"- Prefer git/read for repository status, diff, log, show, and branch evidence instead of shell git commands.",
			"- Use git/write only for the supported local operations init, add, and commit. Use init only when the workspace deliverable directory is not yet a Git repository. Use patch/apply for file content changes before staging them.",
			"- git/write commit does not accept pathspecs; stage files first with git/write add using explicit relative pathspecs, then commit the staged index with a non-empty message.",
		)
	}
	if requestHasBlockKind(request, "shell/command") {
		lines = append(lines,
			"- Shell command names must be real commands available in the local process environment. Do not use a command named workorch.",
			"- shell/command always executes with its working directory set by the runtime to the workspace deliverable directory. Use relative paths from that directory; do not hard-code cd into absolute workspace paths such as /home/user/workspace or generated workspace directories.",
			"- Shell command arguments must be encoded only as args arrays. For example use {\"command\":\"find\",\"args\":[\".\",\"-maxdepth\",\"3\"]}; never place positional arguments as object properties after command.",
			"- shell/command.command must contain only the executable name or path. For shell syntax such as redirection, heredoc, cd, pipes, &&, ||, or multi-line scripts, use {\"command\":\"sh\",\"args\":[\"-c\",\"...\"]} or use patchRequests to write files.",
			"- Do not use commands listed in runtimeCapabilities.shellPolicy.unavailableCommands, including inside sh -c, bash -c, or other nested shell scripts.",
			"- Shell command input may include timeoutMillis for commands expected to take longer than the default; keep commands non-interactive and bounded.",
			"- For curl-based data queries, use quiet output such as curl -sS so progress meters do not become execution evidence or final result text.",
			"- Shell commands may call local workorchd management HTTP paths only through runtimeCapabilities.serviceAPIs. Do not use /api/v1/workorch/*, /runs, /observability, /metrics?run=..., or any workorch management endpoint that is not explicitly declared. This policy does not prohibit bounded validation of workspace-owned local service endpoints that are outside workorch management paths.",
		)
	}
	if requestHasBlockKind(request, "web/search") || requestHasBlockKind(request, "web/fetch") {
		lines = append(lines,
			"- For current or external facts, prefer web/search and web/fetch from runtimeCapabilities instead of shell curl. Treat web/search results as candidate sources and use web/fetch for facts that affect the final answer.",
			"- When writing memory from web-backed work, write only adopted fact summaries plus source URL, retrievedAt/contentHash, and evidenceRefs. Never write raw webpage bodies or unadopted search candidates into memoryWrites.",
		)
	}
	if requestHasBlockKind(request, "ssh/command") {
		lines = append(lines,
			"- Do not assume remote host access credentials, SSH users, keys, or passwords. If a goal requires remote host configuration and no access method is present in the request, return blocked_on_decision with decisionKind provide_missing_info and ask for the access method or credentials.",
			"- When remote SSH credentials are explicitly available, prefer the ssh/command block kind from runtimeCapabilities.blockKinds. Do not emulate password SSH with sshpass or expect through shell/command.",
			"- ssh/command input supports host, port, username, password, privateKey, privateKeyPath, command, and timeoutMillis. Keep the remote command non-interactive and bounded.",
		)
	}
	if len(lines) == 1 {
		return "Runtime capability contract: use only runtimeCapabilities declared in this provider request."
	}
	return strings.Join(lines, "\n")
}

func requestHasBlockKind(request llmmodels.ProviderRequest, kind string) bool {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return false
	}
	for _, item := range request.RuntimeCapabilities.BlockKinds {
		if strings.TrimSpace(item.Kind) == kind {
			return true
		}
	}
	return false
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
