package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	anthropicoption "github.com/anthropics/anthropic-sdk-go/option"
	openai "github.com/openai/openai-go"
	openaioption "github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

const anthropicDefaultMaxTokens int64 = 4096

type OpenAIProvider struct {
	config llmmodels.ProviderConfig
	client openai.Client
}

func NewOpenAIProvider(config llmmodels.ProviderConfig) *OpenAIProvider {
	config = normalizedProviderConfigForRuntime(config)
	return NewOpenAIProviderWithClient(config, &http.Client{Timeout: effectiveProviderTimeout(config)})
}

func NewOpenAIProviderWithClient(config llmmodels.ProviderConfig, client *http.Client) *OpenAIProvider {
	config = normalizedProviderConfigForRuntime(config)
	if client == nil {
		client = &http.Client{Timeout: effectiveProviderTimeout(config)}
	}
	opts := []openaioption.RequestOption{
		openaioption.WithAPIKey(config.APIKey),
		openaioption.WithHTTPClient(client),
		openaioption.WithRequestTimeout(effectiveProviderTimeout(config)),
	}
	if strings.TrimSpace(config.BaseURL) != "" {
		opts = append(opts, openaioption.WithBaseURL(config.BaseURL))
	}
	return &OpenAIProvider{
		config: config,
		client: openai.NewClient(opts...),
	}
}

func (p *OpenAIProvider) Complete(ctx context.Context, request llmmodels.ProviderRequest) (llmmodels.ProviderCompletionResult, error) {
	if p == nil {
		return llmmodels.ProviderCompletionResult{}, common.ErrInvalidArgument
	}
	completion, err := p.client.Chat.Completions.New(ctx, p.chatParams(request))
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, providerSDKError("openai", err)
	}
	content := providerCompletionContent(request, firstOpenAIChoiceContent(completion))
	if strings.TrimSpace(content) == "" {
		return llmmodels.ProviderCompletionResult{}, fmt.Errorf("%w: llm provider response content is empty", common.ErrInvalidArgument)
	}
	return llmmodels.ProviderCompletionResult{
		Payload: []byte(content),
		Usage:   openAIUsage(completion),
	}, nil
}

func (p *OpenAIProvider) StreamComplete(ctx context.Context, request llmmodels.ProviderRequest, emit func(llmmodels.ProviderStreamEvent) error) (llmmodels.ProviderCompletionResult, error) {
	if p == nil {
		return llmmodels.ProviderCompletionResult{}, common.ErrInvalidArgument
	}
	stream := p.client.Chat.Completions.NewStreaming(ctx, p.chatStreamParams(request))
	var builder strings.Builder
	var usage *llmmodels.ProviderUsage
	for stream.Next() {
		chunk := stream.Current()
		if parsed := openAICompletionUsage(chunk.Usage); parsed != nil {
			usage = parsed
		}
		for _, choice := range chunk.Choices {
			delta := choice.Delta.Content
			if delta == "" {
				continue
			}
			builder.WriteString(delta)
			if emit != nil {
				if err := emit(llmmodels.ProviderStreamEvent{Type: "delta", Delta: delta}); err != nil {
					return llmmodels.ProviderCompletionResult{}, err
				}
			}
		}
	}
	if err := stream.Err(); err != nil {
		return llmmodels.ProviderCompletionResult{}, providerSDKError("openai", err)
	}
	if emit != nil {
		if err := emit(llmmodels.ProviderStreamEvent{Type: "done"}); err != nil {
			return llmmodels.ProviderCompletionResult{}, err
		}
	}
	content := providerCompletionContent(request, builder.String())
	if strings.TrimSpace(content) == "" {
		return llmmodels.ProviderCompletionResult{}, fmt.Errorf("%w: llm provider streamed response content is empty", common.ErrInvalidArgument)
	}
	return llmmodels.ProviderCompletionResult{
		Payload: []byte(content),
		Usage:   usage,
	}, nil
}

func (p *OpenAIProvider) Embed(ctx context.Context, request llmmodels.ProviderEmbeddingRequest) (llmmodels.ProviderEmbeddingResult, error) {
	if p == nil {
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
	response, err := p.client.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Input: openai.EmbeddingNewParamsInputUnion{OfString: openai.String(input)},
		Model: openai.EmbeddingModel(model),
	})
	if err != nil {
		return llmmodels.ProviderEmbeddingResult{}, providerSDKError("openai", err)
	}
	if len(response.Data) == 0 || len(response.Data[0].Embedding) == 0 {
		return llmmodels.ProviderEmbeddingResult{}, fmt.Errorf("%w: llm provider embedding response is empty", common.ErrInvalidArgument)
	}
	return llmmodels.ProviderEmbeddingResult{
		ProviderID: p.config.ID,
		Model:      model,
		Input:      input,
		Vector:     append([]float64(nil), response.Data[0].Embedding...),
	}, nil
}

func (p *OpenAIProvider) chatParams(request llmmodels.ProviderRequest) openai.ChatCompletionNewParams {
	prompt := buildProviderFacingPrompt(request)
	chatMessages := make([]openai.ChatCompletionMessageParamUnion, 0, len(prompt.DynamicMessages)+1)
	if stablePrefix := strings.TrimSpace(prompt.StablePrefix); stablePrefix != "" {
		chatMessages = append(chatMessages, openai.SystemMessage(stablePrefix))
	}
	for _, item := range prompt.DynamicMessages {
		content := strings.TrimSpace(item.Content)
		if content == "" {
			continue
		}
		switch normalizedPromptRole(item.Role) {
		case "system":
			chatMessages = append(chatMessages, openai.SystemMessage(content))
		case "assistant":
			chatMessages = append(chatMessages, openai.AssistantMessage(content))
		default:
			chatMessages = append(chatMessages, openai.UserMessage(content))
		}
	}
	if len(chatMessages) == 0 {
		chatMessages = append(chatMessages, openai.SystemMessage(systemPromptForRequest(request)), openai.UserMessage(userPrompt(request)))
	}
	params := openai.ChatCompletionNewParams{
		Messages:    chatMessages,
		Model:       shared.ChatModel(p.config.Model),
		Temperature: openai.Float(providerTemperature(p.config)),
	}
	if !request.RawTextOutput && p.config.Capabilities.JSONSchemaResponse && strings.TrimSpace(request.Schema) != "" {
		var schema any
		if err := json.Unmarshal([]byte(request.Schema), &schema); err == nil {
			params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
				OfJSONSchema: &shared.ResponseFormatJSONSchemaParam{
					JSONSchema: shared.ResponseFormatJSONSchemaJSONSchemaParam{
						Name:   "workorch_runtime_v2_candidate",
						Schema: schema,
					},
				},
			}
		}
	}
	if maxTokens := maxOutputTokens(p.config); maxTokens > 0 {
		params.MaxCompletionTokens = openai.Int(maxTokens)
	}
	if p.config.InferenceProfile.ReasoningEffort != "" {
		params.ReasoningEffort = shared.ReasoningEffort(p.config.InferenceProfile.ReasoningEffort)
	}
	return params
}

func (p *OpenAIProvider) chatStreamParams(request llmmodels.ProviderRequest) openai.ChatCompletionNewParams {
	params := p.chatParams(request)
	params.StreamOptions = openai.ChatCompletionStreamOptionsParam{
		IncludeUsage: openai.Bool(true),
	}
	return params
}

type AnthropicProvider struct {
	config llmmodels.ProviderConfig
	client anthropic.Client
}

func NewAnthropicProvider(config llmmodels.ProviderConfig) *AnthropicProvider {
	config = normalizedProviderConfigForRuntime(config)
	return NewAnthropicProviderWithClient(config, &http.Client{Timeout: effectiveProviderTimeout(config)})
}

func NewAnthropicProviderWithClient(config llmmodels.ProviderConfig, client *http.Client) *AnthropicProvider {
	config = normalizedProviderConfigForRuntime(config)
	if client == nil {
		client = &http.Client{Timeout: effectiveProviderTimeout(config)}
	}
	opts := []anthropicoption.RequestOption{
		anthropicoption.WithAPIKey(config.APIKey),
		anthropicoption.WithHTTPClient(client),
		anthropicoption.WithRequestTimeout(effectiveProviderTimeout(config)),
	}
	if strings.TrimSpace(config.BaseURL) != "" {
		opts = append(opts, anthropicoption.WithBaseURL(config.BaseURL))
	}
	return &AnthropicProvider{
		config: config,
		client: anthropic.NewClient(opts...),
	}
}

func (p *AnthropicProvider) Complete(ctx context.Context, request llmmodels.ProviderRequest) (llmmodels.ProviderCompletionResult, error) {
	if p == nil {
		return llmmodels.ProviderCompletionResult{}, common.ErrInvalidArgument
	}
	message, err := p.client.Messages.New(ctx, p.messageParams(request))
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, providerSDKError("anthropic", err)
	}
	content := providerCompletionContent(request, firstAnthropicText(message))
	if strings.TrimSpace(content) == "" {
		return llmmodels.ProviderCompletionResult{}, fmt.Errorf("%w: llm provider response content is empty", common.ErrInvalidArgument)
	}
	return llmmodels.ProviderCompletionResult{
		Payload: []byte(content),
		Usage:   anthropicUsage(message),
	}, nil
}

func (p *AnthropicProvider) StreamComplete(ctx context.Context, request llmmodels.ProviderRequest, emit func(llmmodels.ProviderStreamEvent) error) (llmmodels.ProviderCompletionResult, error) {
	if p == nil {
		return llmmodels.ProviderCompletionResult{}, common.ErrInvalidArgument
	}
	stream := p.client.Messages.NewStreaming(ctx, p.messageParams(request))
	var message anthropic.Message
	var builder strings.Builder
	for stream.Next() {
		event := stream.Current()
		if err := message.Accumulate(event); err != nil {
			return llmmodels.ProviderCompletionResult{}, err
		}
		switch current := event.AsAny().(type) {
		case anthropic.ContentBlockDeltaEvent:
			if delta, ok := current.Delta.AsAny().(anthropic.TextDelta); ok && delta.Text != "" {
				builder.WriteString(delta.Text)
				if emit != nil {
					if err := emit(llmmodels.ProviderStreamEvent{Type: "delta", Delta: delta.Text}); err != nil {
						return llmmodels.ProviderCompletionResult{}, err
					}
				}
			}
		}
	}
	if err := stream.Err(); err != nil {
		return llmmodels.ProviderCompletionResult{}, providerSDKError("anthropic", err)
	}
	if emit != nil {
		if err := emit(llmmodels.ProviderStreamEvent{Type: "done"}); err != nil {
			return llmmodels.ProviderCompletionResult{}, err
		}
	}
	content := providerCompletionContent(request, builder.String())
	if strings.TrimSpace(content) == "" {
		content = providerCompletionContent(request, firstAnthropicText(&message))
	}
	if strings.TrimSpace(content) == "" {
		return llmmodels.ProviderCompletionResult{}, fmt.Errorf("%w: llm provider streamed response content is empty", common.ErrInvalidArgument)
	}
	return llmmodels.ProviderCompletionResult{
		Payload: []byte(content),
		Usage:   anthropicUsage(&message),
	}, nil
}

func (p *AnthropicProvider) messageParams(request llmmodels.ProviderRequest) anthropic.MessageNewParams {
	prompt := buildProviderFacingPrompt(request)
	anthropicMessages := make([]anthropic.MessageParam, 0, len(prompt.DynamicMessages))
	for _, item := range prompt.DynamicMessages {
		role := normalizedPromptRole(item.Role)
		content := strings.TrimSpace(item.Content)
		if content == "" || role == "system" {
			continue
		}
		if role == "assistant" {
			anthropicMessages = append(anthropicMessages, anthropic.NewAssistantMessage(anthropic.NewTextBlock(content)))
			continue
		}
		anthropicMessages = append(anthropicMessages, anthropic.NewUserMessage(anthropic.NewTextBlock(content)))
	}
	if len(anthropicMessages) == 0 {
		anthropicMessages = append(anthropicMessages, anthropic.NewUserMessage(anthropic.NewTextBlock(userPrompt(request))))
	}
	system := []anthropic.TextBlockParam{{Text: systemPromptForRequest(request)}}
	if stablePrefix := strings.TrimSpace(prompt.StablePrefix); stablePrefix != "" {
		block := anthropic.TextBlockParam{Text: stablePrefix}
		if cacheControlSentForRequest(p.config, prompt) {
			block.CacheControl = anthropic.NewCacheControlEphemeralParam()
		}
		system = []anthropic.TextBlockParam{block}
	}
	params := anthropic.MessageNewParams{
		MaxTokens:   anthropicMaxTokens(p.config),
		Messages:    anthropicMessages,
		Model:       anthropic.Model(p.config.Model),
		Temperature: anthropic.Float(providerTemperature(p.config)),
		System:      system,
	}
	if p.config.InferenceProfile.ThinkingBudgetTokens > 0 {
		params.Thinking = anthropic.ThinkingConfigParamOfEnabled(int64(p.config.InferenceProfile.ThinkingBudgetTokens))
	}
	return params
}

func providerTemperature(config llmmodels.ProviderConfig) float64 {
	if config.InferenceProfile.Temperature == nil {
		return 0
	}
	return *config.InferenceProfile.Temperature
}

func maxOutputTokens(config llmmodels.ProviderConfig) int64 {
	if config.InferenceProfile.MaxOutputTokens <= 0 {
		return 0
	}
	return int64(config.InferenceProfile.MaxOutputTokens)
}

func anthropicMaxTokens(config llmmodels.ProviderConfig) int64 {
	maxTokens := maxOutputTokens(config)
	if maxTokens <= 0 {
		maxTokens = anthropicDefaultMaxTokens
	}
	if budget := int64(config.InferenceProfile.ThinkingBudgetTokens); budget > 0 && maxTokens <= budget {
		maxTokens = budget + anthropicDefaultMaxTokens
	}
	return maxTokens
}

func compatibleThinkingConfig(config llmmodels.ProviderConfig) *compatibleThinkingConfigParam {
	if config.InferenceProfile.ThinkingBudgetTokens <= 0 {
		return nil
	}
	return &compatibleThinkingConfigParam{
		Type:         "enabled",
		BudgetTokens: int64(config.InferenceProfile.ThinkingBudgetTokens),
	}
}

func firstOpenAIChoiceContent(completion *openai.ChatCompletion) string {
	if completion == nil || len(completion.Choices) == 0 {
		return ""
	}
	return completion.Choices[0].Message.Content
}

func firstAnthropicText(message *anthropic.Message) string {
	if message == nil {
		return ""
	}
	for _, item := range message.Content {
		if strings.TrimSpace(item.Text) != "" {
			return item.Text
		}
	}
	return ""
}

func openAIUsage(completion *openai.ChatCompletion) *llmmodels.ProviderUsage {
	if completion == nil {
		return nil
	}
	return openAICompletionUsage(completion.Usage)
}

func openAICompletionUsage(usage openai.CompletionUsage) *llmmodels.ProviderUsage {
	input := int(usage.PromptTokens)
	output := int(usage.CompletionTokens)
	total := int(usage.TotalTokens)
	if input == 0 && output == 0 && total == 0 {
		return nil
	}
	if total == 0 && (input > 0 || output > 0) {
		total = input + output
	}
	return &llmmodels.ProviderUsage{
		InputTokens:     input,
		OutputTokens:    output,
		TotalTokens:     total,
		CacheReadTokens: int(usage.PromptTokensDetails.CachedTokens),
	}
}

func anthropicUsage(message *anthropic.Message) *llmmodels.ProviderUsage {
	if message == nil {
		return nil
	}
	input := int(message.Usage.InputTokens)
	output := int(message.Usage.OutputTokens)
	total := input + output
	if input == 0 && output == 0 {
		return nil
	}
	return &llmmodels.ProviderUsage{
		InputTokens:         input,
		OutputTokens:        output,
		TotalTokens:         total,
		CacheReadTokens:     int(message.Usage.CacheReadInputTokens),
		CacheCreationTokens: int(message.Usage.CacheCreationInputTokens),
		CacheWriteTokens:    int(message.Usage.CacheCreationInputTokens),
	}
}

func effectiveProviderTimeout(config llmmodels.ProviderConfig) time.Duration {
	if config.Timeout > 0 {
		return config.Timeout
	}
	return defaultProviderTimeout
}

func providerSDKError(provider string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %s llm provider error: %v", common.ErrInvalidArgument, provider, err)
}
