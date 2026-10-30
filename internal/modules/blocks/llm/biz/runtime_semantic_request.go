package llm

import (
	"encoding/json"
	"fmt"
	"strings"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type runtimeSemanticPreparedRequest struct {
	context      runtimecontract.RuntimeSemanticPlanRequestContext
	manifest     runtimecontract.RuntimeSemanticPlanManifest
	ceiling      *runtimecontract.ProviderCeiling
	schema       string
	stablePrefix string
}

func prepareRuntimeSemanticPlanRequest(request llmmodels.RuntimeCandidateRequest) (runtimeSemanticPreparedRequest, error) {
	if request.CandidateManifest.Hash != "" || request.CandidateContext.WorkspaceID != "" || request.CandidateContext.RunID != "" || request.DefinitionContext.Revision != "" || request.CandidateCorrectionObservation != nil || request.CandidateCorrection != nil {
		return runtimeSemanticPreparedRequest{}, fmt.Errorf("semantic plan provider request must not carry operations fields")
	}
	if err := runtimecontract.ValidateRuntimeEvidencePolicies(request.SemanticContext.Activation.EvidencePolicies); err != nil {
		return runtimeSemanticPreparedRequest{}, fmt.Errorf("runtime semantic plan context activation evidence policies: %w", err)
	}
	if request.ExpectedDocument != nil && request.SemanticContext.PlanStep == runtimecontract.RuntimeSemanticPlanStepProofBatch {
		for _, obligation := range request.SemanticManifest.ProofObligations {
			if obligation.ImplementationWork != nil && request.ExpectedDocument.SemanticRenderContractRef != runtimecontract.RuntimeSemanticRenderContractV2 {
				return runtimeSemanticPreparedRequest{}, &runtimecontract.RuntimeSemanticRequestContractError{Reason: "sealed work renderer is obsolete; exact owner retirement is required before a new physical call"}
			}
		}
	}
	context := request.SemanticContext.Clone()
	manifest := request.SemanticManifest.Clone()
	if err := context.Validate(manifest); err != nil {
		return runtimeSemanticPreparedRequest{}, fmt.Errorf("runtime semantic plan context: %w", err)
	}
	if err := validateRuntimeSemanticAttempt(request, context, manifest); err != nil {
		return runtimeSemanticPreparedRequest{}, err
	}
	stablePrefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(context.Definition, context.Activation.ExecutionContract)
	if strings.TrimSpace(request.StablePrefixHash) != stablePrefix {
		return runtimeSemanticPreparedRequest{}, fmt.Errorf("runtime semantic plan stable prefix mismatch")
	}
	ceiling := context.ProviderCeiling
	if ceiling == nil || ceiling.MaxBytes <= 0 {
		return runtimeSemanticPreparedRequest{}, fmt.Errorf("runtime semantic plan provider ceiling is missing")
	}
	if err := ceiling.Selection.Target.Validate(); err != nil {
		return runtimeSemanticPreparedRequest{}, fmt.Errorf("runtime semantic plan provider ceiling target invalid: %w", err)
	}
	schema, err := runtimecontract.RuntimeSemanticPlanJSONSchemaForRequest(manifest, context.PlanStep, context.BatchID, context.BatchSequence, context.BatchCoverageWorkItemIDs)
	if err != nil {
		return runtimeSemanticPreparedRequest{}, err
	}
	return runtimeSemanticPreparedRequest{
		context: context, manifest: manifest, ceiling: ceiling, schema: schema, stablePrefix: stablePrefix,
	}, nil
}

func validateRuntimeSemanticAttempt(request llmmodels.RuntimeCandidateRequest, context runtimecontract.RuntimeSemanticPlanRequestContext, manifest runtimecontract.RuntimeSemanticPlanManifest) error {
	if request.CandidateAttempt < 0 || request.CandidateAttempt > 1 {
		return fmt.Errorf("runtime semantic plan attempt must be 0 or 1")
	}
	if request.CandidateAttempt == 0 {
		if request.SemanticCorrectionObservation != nil || request.SemanticCorrectionDirective != nil {
			return fmt.Errorf("initial runtime semantic plan request must not carry correction lineage")
		}
		return nil
	}
	if request.SemanticCorrectionObservation == nil || request.SemanticCorrectionDirective == nil {
		return fmt.Errorf("runtime semantic plan correction requires rejection and directive")
	}
	if err := request.SemanticCorrectionObservation.Validate(); err != nil {
		return err
	}
	if err := request.SemanticCorrectionDirective.ValidateAgainstObservation(manifest, *request.SemanticCorrectionObservation); err != nil {
		return err
	}
	if request.SemanticCorrectionDirective.PlanStep != context.PlanStep || request.SemanticCorrectionDirective.StablePrefixHash != strings.TrimSpace(request.StablePrefixHash) {
		return fmt.Errorf("runtime semantic plan correction identity is stale")
	}
	return nil
}

func finalizeRuntimeSemanticProviderRequest(request llmmodels.RuntimeCandidateRequest, prepared runtimeSemanticPreparedRequest, providerRequest llmmodels.ProviderRequest) (llmmodels.ProviderRequest, error) {
	providerRequest.MaxOutputTokens = prepared.ceiling.EffectiveMaxOutputTokens
	if request.ExpectedDocument != nil {
		providerRequest.MaxOutputTokens = request.ExpectedDocument.Normalized().MaxOutputTokens
	}
	providerRequest.ContextBudget.EffectiveMaxOutputTokens = providerRequest.MaxOutputTokens
	providerRequest.ContextBudget.EstimatedOutputTokens = providerRequest.MaxOutputTokens
	attachProviderCachePlan(&providerRequest)
	if providerRequest.ContextBudget.CachePlan != nil {
		providerRequest.ContextBudget.CachePlan.StablePrefixHash = prepared.stablePrefix
	}
	populateRuntimeCandidateContextBudget(providerRequest.ContextBudget, providerRequest.PromptContext)
	if err := validateRuntimeSemanticContextIntegrity(prepared.context, prepared.manifest, request.CandidateAttempt, request.SemanticCorrectionObservation, request.SemanticCorrectionDirective, providerRequest.ContextBudget, providerRequest.PromptContext); err != nil {
		providerRequest.ContextBudget.ContextGateAction = "fail_fast"
		providerRequest.ContextBudget.ContextGateReason = "runtime_semantic_context_integrity_failed"
		providerRequest.ContextBudget.BudgetFailureClass = "context_integrity"
		return providerRequest, err
	}
	payload, err := json.Marshal(providerRequest)
	if err != nil {
		return llmmodels.ProviderRequest{}, err
	}
	providerRequest.ContextBudget.ActualBytes = len(payload)
	providerRequest.ContextBudget.OverBudget = len(payload) > prepared.ceiling.MaxBytes
	if providerRequest.ContextBudget.OverBudget {
		providerRequest.ContextBudget.ContextGateAction = "fail_fast"
		providerRequest.ContextBudget.ContextGateReason = "runtime_provider_ceiling_exceeded"
		providerRequest.ContextBudget.BudgetFailureClass = "provider_ceiling_exceeded"
		return providerRequest, fmt.Errorf("runtime semantic plan request exceeds provider ceiling: %d > %d", len(payload), prepared.ceiling.MaxBytes)
	}
	return providerRequest, nil
}
