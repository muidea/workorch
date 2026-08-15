package llm

import (
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
)

// runtimeSemanticPlanProviderRequest is the single semantic-plan rendering
// transaction. Each private phase is deterministic and side-effect free; the
// physical provider call remains owned by GenerateCandidate.
func runtimeSemanticPlanProviderRequest(request llmmodels.RuntimeCandidateRequest) (llmmodels.ProviderRequest, error) {
	prepared, err := prepareRuntimeSemanticPlanRequest(request)
	if err != nil {
		return llmmodels.ProviderRequest{}, err
	}
	prompt, err := buildRuntimeSemanticPrompt(request, prepared)
	if err != nil {
		return llmmodels.ProviderRequest{}, err
	}
	providerRequest := newRuntimeSemanticProviderRequest(request, prepared, prompt)
	return finalizeRuntimeSemanticProviderRequest(request, prepared, providerRequest)
}
