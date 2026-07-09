package events

import (
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	llmmodel "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
)

const definitionPackage = "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
const CommandDestination = modulecommon.ModuleID

type CompleteProviderCommand struct {
	ProviderID string
	Request    llmmodel.ProviderRequest
}

type GenerateEmbeddingCommand struct {
	ProviderID string
	Input      string
}

type LLMCommandResult struct {
	ProviderID string
	Model      string
	Completion *llmmodel.ProviderCompletionResult
	Embedding  *llmmodel.ProviderEmbeddingResult
}

func MessageContracts() []eventcontract.MessageContract {
	return eventcontract.MessageContracts(
		eventcontract.CommandTopic[CompleteProviderCommand, LLMCommandResult]("llm", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicLLMCommandCompleteProvider),
		eventcontract.CommandTopic[GenerateEmbeddingCommand, LLMCommandResult]("llm", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicLLMCommandGenerateEmbedding),
	)
}
