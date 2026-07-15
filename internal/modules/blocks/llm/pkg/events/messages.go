package events

import (
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	llmmodel "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const definitionPackage = "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
const CommandDestination = modulecommon.ModuleID

type CompleteProviderCommand struct {
	Target  runtimecontract.ProviderModelTarget
	Request llmmodel.ProviderRequest
}

type GenerateEmbeddingCommand struct {
	Target runtimecontract.ProviderModelTarget
	Input  string
}

type GetProviderModelCatalogCommand struct {
	ProviderID string
}

type GetProviderModelCatalogResult struct {
	Catalog llmmodel.ProviderModelCatalog
}

type RefreshProviderModelCatalogCommand struct {
	ProviderID string
}

type RefreshProviderModelCatalogResult struct {
	Catalog llmmodel.ProviderModelCatalog
}

type ResolveProviderModelTargetCommand struct {
	Target    runtimecontract.ProviderModelTarget
	Operation string // chat_completions | embeddings; empty means chat_completions for activation default
}

type ResolveProviderModelTargetResult struct {
	Profile llmmodel.ResolvedProviderModelProfile
}

type ProviderUsageData struct {
	InputTokens             int
	OutputTokens            int
	TotalTokens             int
	CacheReadTokens         int
	CacheWriteTokens        int
	CacheCreationTokens     int
	CacheableInputTokens    int
	NonCacheableInputTokens int
}

type ProviderCompletionData struct {
	Content string
	Usage   *ProviderUsageData
}

type LLMCommandResult struct {
	Target     runtimecontract.ProviderModelTarget
	Completion *ProviderCompletionData
	Embedding  *llmmodel.ProviderEmbeddingResult
}

// LLMUsageRecordedData is the durable request audit notification for one provider call.
// eventstream persists it under TopicLLMUsageRecorded (/run/llm_usage/recorded).
type LLMUsageRecordedData struct {
	RequestID             string
	WorkspaceID           common.WorkspaceID
	RunID                 common.RunID
	StepID                string
	Target                runtimecontract.ProviderModelTarget
	Operation             string
	CapabilityRevision    string
	ConfigurationRevision string
	StablePrefixHash      string
	Kind                  string
	APISurface            string
	Provider              string
	ProviderSubmitted     bool
	Status                string
	FailureClass          string
	StartedAt             string
	CompletedAt           string
	DurationMs            int
	RequestBytes          int
	ResponseBytes         int
	InputTokens           int
	OutputTokens          int
	TotalTokens           int
	Estimated             bool
	UsageSource           string
	// ContextBudget is the sealed budget snapshot as a scalar map for event payload encoding.
	ContextBudget map[string]any
	// CacheUsage carries provider-native or estimated cache token fields when present.
	CacheUsage map[string]any
	OccurredAt string
}

func MessageContracts() []eventcontract.MessageContract {
	return eventcontract.MessageContracts(
		eventcontract.CommandTopic[CompleteProviderCommand, LLMCommandResult]("llm", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicLLMCommandCompleteProvider),
		eventcontract.CommandTopic[GenerateEmbeddingCommand, LLMCommandResult]("llm", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicLLMCommandGenerateEmbedding),
		eventcontract.CommandTopic[GetProviderModelCatalogCommand, GetProviderModelCatalogResult]("llm", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicLLMCommandGetProviderModelCatalog),
		eventcontract.CommandTopic[RefreshProviderModelCatalogCommand, RefreshProviderModelCatalogResult]("llm", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicLLMCommandRefreshProviderModelCatalog),
		eventcontract.CommandTopic[ResolveProviderModelTargetCommand, ResolveProviderModelTargetResult]("llm", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicLLMCommandResolveProviderModelTarget),
		eventcontract.DataTopic[LLMUsageRecordedData]("llm", definitionPackage, eventcontract.LanePolicyState, TopicLLMUsageRecorded),
	)
}
