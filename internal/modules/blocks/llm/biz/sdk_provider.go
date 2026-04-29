package llm

import (
	"context"
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
	return NewOpenAIProviderWithClient(config, &http.Client{Timeout: effectiveProviderTimeout(config)})
}

func NewOpenAIProviderWithClient(config llmmodels.ProviderConfig, client *http.Client) *OpenAIProvider {
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

func (p *OpenAIProvider) Complete(ctx context.Context, request llmmodels.ProviderRequest) ([]byte, error) {
	if p == nil {
		return nil, common.ErrInvalidArgument
	}
	completion, err := p.client.Chat.Completions.New(ctx, p.chatParams(request))
	if err != nil {
		return nil, providerSDKError("openai", err)
	}
	content := strings.TrimSpace(firstOpenAIChoiceContent(completion))
	if content == "" {
		return nil, fmt.Errorf("%w: llm provider response content is empty", common.ErrInvalidArgument)
	}
	return []byte(content), nil
}

func (p *OpenAIProvider) StreamComplete(ctx context.Context, request llmmodels.ProviderRequest, emit func(llmmodels.ProviderStreamEvent) error) ([]byte, error) {
	if p == nil {
		return nil, common.ErrInvalidArgument
	}
	stream := p.client.Chat.Completions.NewStreaming(ctx, p.chatParams(request))
	var builder strings.Builder
	for stream.Next() {
		chunk := stream.Current()
		for _, choice := range chunk.Choices {
			delta := choice.Delta.Content
			if delta == "" {
				continue
			}
			builder.WriteString(delta)
			if emit != nil {
				if err := emit(llmmodels.ProviderStreamEvent{Type: "delta", Delta: delta}); err != nil {
					return nil, err
				}
			}
		}
	}
	if err := stream.Err(); err != nil {
		return nil, providerSDKError("openai", err)
	}
	if emit != nil {
		if err := emit(llmmodels.ProviderStreamEvent{Type: "done"}); err != nil {
			return nil, err
		}
	}
	content := strings.TrimSpace(builder.String())
	if content == "" {
		return nil, fmt.Errorf("%w: llm provider streamed response content is empty", common.ErrInvalidArgument)
	}
	return []byte(content), nil
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
	return openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(systemPrompt()),
			openai.UserMessage(userPrompt(request)),
		},
		Model:       shared.ChatModel(p.config.Model),
		Temperature: openai.Float(0),
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONObject: &shared.ResponseFormatJSONObjectParam{Type: "json_object"},
		},
	}
}

type AnthropicProvider struct {
	config llmmodels.ProviderConfig
	client anthropic.Client
}

func NewAnthropicProvider(config llmmodels.ProviderConfig) *AnthropicProvider {
	return NewAnthropicProviderWithClient(config, &http.Client{Timeout: effectiveProviderTimeout(config)})
}

func NewAnthropicProviderWithClient(config llmmodels.ProviderConfig, client *http.Client) *AnthropicProvider {
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

func (p *AnthropicProvider) Complete(ctx context.Context, request llmmodels.ProviderRequest) ([]byte, error) {
	if p == nil {
		return nil, common.ErrInvalidArgument
	}
	message, err := p.client.Messages.New(ctx, p.messageParams(request))
	if err != nil {
		return nil, providerSDKError("anthropic", err)
	}
	content := strings.TrimSpace(firstAnthropicText(message))
	if content == "" {
		return nil, fmt.Errorf("%w: llm provider response content is empty", common.ErrInvalidArgument)
	}
	return []byte(content), nil
}

func (p *AnthropicProvider) StreamComplete(ctx context.Context, request llmmodels.ProviderRequest, emit func(llmmodels.ProviderStreamEvent) error) ([]byte, error) {
	if p == nil {
		return nil, common.ErrInvalidArgument
	}
	stream := p.client.Messages.NewStreaming(ctx, p.messageParams(request))
	var message anthropic.Message
	var builder strings.Builder
	for stream.Next() {
		event := stream.Current()
		if err := message.Accumulate(event); err != nil {
			return nil, err
		}
		switch current := event.AsAny().(type) {
		case anthropic.ContentBlockDeltaEvent:
			if delta, ok := current.Delta.AsAny().(anthropic.TextDelta); ok && delta.Text != "" {
				builder.WriteString(delta.Text)
				if emit != nil {
					if err := emit(llmmodels.ProviderStreamEvent{Type: "delta", Delta: delta.Text}); err != nil {
						return nil, err
					}
				}
			}
		}
	}
	if err := stream.Err(); err != nil {
		return nil, providerSDKError("anthropic", err)
	}
	if emit != nil {
		if err := emit(llmmodels.ProviderStreamEvent{Type: "done"}); err != nil {
			return nil, err
		}
	}
	content := strings.TrimSpace(builder.String())
	if content == "" {
		content = strings.TrimSpace(firstAnthropicText(&message))
	}
	if content == "" {
		return nil, fmt.Errorf("%w: llm provider streamed response content is empty", common.ErrInvalidArgument)
	}
	return []byte(content), nil
}

func (p *AnthropicProvider) messageParams(request llmmodels.ProviderRequest) anthropic.MessageNewParams {
	return anthropic.MessageNewParams{
		MaxTokens: anthropicDefaultMaxTokens,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(userPrompt(request))),
		},
		Model:       anthropic.Model(p.config.Model),
		Temperature: anthropic.Float(0),
		System: []anthropic.TextBlockParam{{
			Text: systemPrompt(),
		}},
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
