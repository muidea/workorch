package llm

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func runtimeProviderRequest(request llmmodels.RuntimeCandidateRequest) (llmmodels.ProviderRequest, error) {
	switch request.DocumentKind {
	case runtimecontract.RuntimeProviderDocumentOperations:
		return runtimeOperationsProviderRequest(request)
	case runtimecontract.RuntimeProviderDocumentSemanticPlan:
		return runtimeSemanticPlanProviderRequest(request)
	default:
		return llmmodels.ProviderRequest{}, fmt.Errorf("runtime provider request requires explicit document kind")
	}
}

func runtimeOperationsProviderRequest(request llmmodels.RuntimeCandidateRequest) (llmmodels.ProviderRequest, error) {
	if request.SemanticManifest.Hash != "" || request.SemanticContext.WorkspaceID != "" || request.SemanticContext.RunID != "" || request.SemanticCorrectionObservation != nil || request.SemanticCorrectionDirective != nil {
		return llmmodels.ProviderRequest{}, fmt.Errorf("operations provider request must not carry semantic plan fields")
	}
	definitionContext := request.DefinitionContext.Normalized()
	candidateContext := request.CandidateContext.Normalized()
	// The Orchestrator compiles this sealed manifest before request-session
	// persistence. The renderer consumes that exact value; it must not rebuild
	// field/parameter rules from a parallel descriptor path.
	manifest := request.CandidateManifest
	if err := runtimecontract.ValidateRuntimeCandidateContractManifestForSealedDecision(manifest, request.Decision); err != nil {
		return llmmodels.ProviderRequest{}, fmt.Errorf("runtime candidate contract manifest: %w", err)
	}
	wire, err := runtimecontract.BuildRuntimeCandidateWireContract(manifest)
	if err != nil {
		return llmmodels.ProviderRequest{}, fmt.Errorf("runtime candidate wire contract: %w", err)
	}
	// The EventHub owner boundary validates that every production request carries
	// the sealed ExpectedDocument. Keep this pure renderer usable by focused
	// contract tests, while still refusing drift whenever the identity is
	// present. The renderer must not become a second request-session owner.
	if request.ExpectedDocument != nil && request.ExpectedDocument.Normalized().WireContractHash != wire.Hash {
		return llmmodels.ProviderRequest{}, fmt.Errorf("runtime candidate expected document wire contract mismatch")
	}
	if err := validateRuntimeCandidateManifestSupportingFacts(manifest, candidateContext.Resources); err != nil {
		return llmmodels.ProviderRequest{}, fmt.Errorf("runtime candidate manifest supporting facts: %w", err)
	}
	switch request.CandidateAttempt {
	case 0:
		if request.CandidateCorrectionObservation != nil || request.CandidateCorrection != nil {
			return llmmodels.ProviderRequest{}, fmt.Errorf("initial runtime candidate request must not carry correction lineage")
		}
	case 1:
		if request.CandidateCorrectionObservation == nil || request.CandidateCorrection == nil {
			return llmmodels.ProviderRequest{}, fmt.Errorf("runtime candidate retry requires correction observation and input")
		}
		if err := request.CandidateCorrection.ValidateAgainstObservation(manifest, *request.CandidateCorrectionObservation); err != nil {
			return llmmodels.ProviderRequest{}, fmt.Errorf("runtime candidate retry correction: %w", err)
		}
	default:
		return llmmodels.ProviderRequest{}, fmt.Errorf("runtime candidate attempt must be 0 or 1")
	}
	wantPrefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(definitionContext, candidateContext.Activation.ExecutionContract)
	if strings.TrimSpace(request.StablePrefixHash) != wantPrefix {
		return llmmodels.ProviderRequest{}, fmt.Errorf("runtime candidate stable prefix mismatch")
	}
	ceiling := candidateContext.Resources.ProviderCeiling
	if ceiling == nil || ceiling.MaxBytes <= 0 {
		return llmmodels.ProviderRequest{}, fmt.Errorf("runtime candidate provider ceiling is missing")
	}
	if err := ceiling.Selection.Target.Validate(); err != nil {
		return llmmodels.ProviderRequest{}, fmt.Errorf("runtime candidate provider ceiling target invalid: %w", err)
	}
	definitionJSON, err := json.Marshal(runtimeCandidateDefinitionProjection{
		Revision: definitionContext.Revision, GoalID: definitionContext.GoalID,
		GoalDefinition: definitionContext.GoalDefinition, DoDID: definitionContext.DoDID,
		DoDDefinition: definitionContext.DoDDefinition, ChangeImpact: definitionContext.ChangeImpact,
	})
	if err != nil {
		return llmmodels.ProviderRequest{}, err
	}
	capabilityJSON, err := json.Marshal(struct {
		Kinds             []string                                 `json:"kinds"`
		ExecutionContract runtimecontract.RuntimeExecutionContract `json:"executionContract"`
	}{definitionContext.CapabilityContract, candidateContext.Activation.ExecutionContract})
	if err != nil {
		return llmmodels.ProviderRequest{}, err
	}
	authorityJSON, err := json.Marshal(struct {
		Decision       runtimecontract.RuntimeDecision          `json:"decision"`
		Transaction    runtimecontract.AuthorityTransaction     `json:"transaction"`
		OutputContract runtimeCandidateOutputContractProjection `json:"outputContract"`
		Context        runtimeCandidateAuthorityProjection      `json:"context"`
	}{
		request.Decision,
		request.Transaction,
		runtimeCandidateOutputContractForManifest(manifest, wire),
		runtimeCandidateAuthorityContextProjection(candidateContext, manifest),
	})
	if err != nil {
		return llmmodels.ProviderRequest{}, err
	}
	executionTailJSON, err := json.Marshal(buildRuntimeCandidateExecutionTailProjection(candidateContext, request.CandidateAttempt, request.CandidateCorrectionObservation, manifest, request.CandidateCorrection))
	if err != nil {
		return llmmodels.ProviderRequest{}, err
	}
	largeRefsJSON, err := json.Marshal(buildRuntimeCandidateLargeRefsProjection(candidateContext))
	if err != nil {
		return llmmodels.ProviderRequest{}, err
	}
	providerRequest := llmmodels.ProviderRequest{
		WorkspaceID: string(candidateContext.WorkspaceID), RunID: string(candidateContext.RunID),
		StepID: request.Transaction.AuthoritySnapshotID, Kind: "runtime_candidate",
		ReasoningEffort: ceiling.Selection.ReasoningEffort,
		Schema:          runtimeCandidateSchemaForManifest(manifest),
		PromptContext: &llmmodels.PromptContext{SchemaVersion: "workorch.runtime_candidate_prompt", Sections: []llmmodels.PromptSection{
			{ID: "C0.runtime-candidate-contract", Name: "Runtime candidate contract", Role: "system", Cacheability: "stable", Protected: true, DisplayOrder: 0, SourceRefs: []string{"runtime://candidate-contract"}, Content: runtimeCandidateContractCatalog()},
			{ID: "C1.goal-dod", Name: "Goal and DoD", Role: "system", Cacheability: "stable", Protected: true, DisplayOrder: 1, DefinitionRevision: definitionContext.Revision, SourceRefs: []string{"runtime://definition/" + definitionContext.Revision}, Content: string(definitionJSON)},
			{ID: "C2.capability-contract", Name: "Capability contract", Role: "system", Cacheability: "stable", Protected: true, DisplayOrder: 2, SourceRefs: []string{"runtime://execution-contract/" + definitionContext.Revision}, Content: string(capabilityJSON)},
			{ID: "C3.runtime-authority", Name: "Current runtime authority", Role: "user", Cacheability: "volatile", Protected: true, DisplayOrder: 3, SourceRefs: []string{"runtime://authority/" + request.Transaction.AuthoritySnapshotID}, Content: string(authorityJSON)},
			{ID: "C4.execution-tail", Name: "Current failure and evidence", Role: "user", Cacheability: "volatile", Protected: true, DisplayOrder: 4, SourceRefs: []string{"runtime://evidence/" + candidateContext.EvidenceRevision}, Content: string(executionTailJSON)},
			{ID: "C5.large-refs", Name: "Large and historical references", Role: "user", Cacheability: "volatile", DisplayOrder: 5, SourceRefs: []string{"runtime://remaining-focuses"}, Content: string(largeRefsJSON)},
		}},
		ContextBudget: &llmmodels.ProviderContextBudget{
			ProviderID: ceiling.Selection.Target.ProviderID, ModelID: ceiling.Selection.Target.ModelID,
			CapabilityRevision: ceiling.CapabilityRevision, ConfigurationRevision: ceiling.ConfigurationRevision,
			ContextWindowTokens: ceiling.ContextWindowTokens, ProviderMaxOutputTokens: ceiling.ProviderMaxOutputTokens,
			EffectiveMaxOutputTokens: ceiling.EffectiveMaxOutputTokens,
			MaxBytes:                 ceiling.MaxBytes, MaxRequestBytes: ceiling.MaxBytes, MaxTokens: ceiling.MaxInputTokens,
			EstimatedOutputTokens: ceiling.EffectiveMaxOutputTokens,
			CapabilitySource:      "sealed_ceiling", ProfileMode: "runtime_typed_ceiling",
			CachePlan: &llmmodels.ProviderCachePlan{
				SchemaVersion: "workorch.provider_cache_plan", Strategy: "stable_prefix_dynamic_tail",
				StablePrefixHash: wantPrefix, RenderedStablePrefixHash: wantPrefix,
			},
		},
	}
	attachProviderCachePlan(&providerRequest)
	if providerRequest.ContextBudget.CachePlan != nil {
		// The persisted Runtime prefix is the authority contract. The provider
		// cache plan independently records the exact rendered prefix so audit can
		// detect a rendering mismatch without replacing the sealed identity.
		providerRequest.ContextBudget.CachePlan.StablePrefixHash = wantPrefix
	}
	populateRuntimeCandidateContextBudget(providerRequest.ContextBudget, providerRequest.PromptContext)
	if err := validateRuntimeCandidateContextIntegrity(definitionContext, candidateContext, request.CandidateAttempt, request.CandidateCorrectionObservation, providerRequest.ContextBudget, providerRequest.PromptContext); err != nil {
		providerRequest.ContextBudget.ContextGateAction = "fail_fast"
		providerRequest.ContextBudget.ContextGateReason = "runtime_context_integrity_failed"
		providerRequest.ContextBudget.BudgetFailureClass = "context_integrity"
		return providerRequest, err
	}
	payload, err := json.Marshal(providerRequest)
	if err != nil {
		return llmmodels.ProviderRequest{}, err
	}
	providerRequest.ContextBudget.ActualBytes = len(payload)
	providerRequest.ContextBudget.OverBudget = len(payload) > ceiling.MaxBytes
	if providerRequest.ContextBudget.OverBudget {
		providerRequest.ContextBudget.ContextGateAction = "fail_fast"
		providerRequest.ContextBudget.ContextGateReason = "runtime_provider_ceiling_exceeded"
		providerRequest.ContextBudget.BudgetFailureClass = "provider_ceiling_exceeded"
		// The rendered request itself remains owner-private, but its bounded
		// context budget is the only evidence that explains a preflight refusal.
		// Returning an empty request here made the not-ready path lose section
		// hashes, source refs and the gate disposition before it could be audited.
		return providerRequest, fmt.Errorf("runtime candidate request exceeds provider ceiling: %d > %d", len(payload), ceiling.MaxBytes)
	}
	return providerRequest, nil
}

// validateRuntimeCandidateManifestSupportingFacts is a downstream integrity
// fence, not a manifest compiler. The Orchestrator remains the only owner that
// selects authority and seals the manifest; the LLM owner only proves that
// every snapshot identity referenced by that immutable value is still present
// as the same canonical typed fact before spending a physical provider call.
func validateRuntimeCandidateManifestSupportingFacts(
	manifest runtimecontract.RuntimeCandidateContractManifest,
	resources runtimecontract.RuntimeResourceFacts,
) error {
	manifest = manifest.Normalized()
	resources = resources.Normalized()

	for _, operation := range manifest.Operations {
		for _, snapshotRef := range operation.PrerequisiteSnapshotRefs {
			matched := false
			if resources.TargetSelection != nil && resources.TargetSelection.SnapshotID == snapshotRef {
				matched = true
				if err := resources.TargetSelection.Validate(); err != nil {
					return fmt.Errorf("prerequisite snapshot %q is invalid: %w", snapshotRef, err)
				}
			}
			for _, snapshot := range resources.CapabilityStates {
				if snapshot.SnapshotID != snapshotRef {
					continue
				}
				matched = true
				if snapshot.SnapshotID != runtimecontract.StableRuntimeCapabilityStateSnapshotID(snapshot) {
					return fmt.Errorf("prerequisite snapshot %q has drifted content", snapshotRef)
				}
			}
			if !matched {
				return fmt.Errorf("prerequisite snapshot %q is missing", snapshotRef)
			}
		}

		for _, snapshotRef := range operation.AdmissibilitySnapshotRefs {
			matched := false
			for _, catalog := range resources.OperationAdmissibilities {
				if catalog.SnapshotID != snapshotRef {
					continue
				}
				matched = true
				if err := catalog.Validate(); err != nil {
					return fmt.Errorf("admissibility snapshot %q is invalid: %w", snapshotRef, err)
				}
				if catalog.OwnerModuleID != operation.OwnerModuleID || !slices.Contains(catalog.OperationKinds, operation.Kind) {
					return fmt.Errorf("admissibility snapshot %q does not support manifest owner/kind %s/%s", snapshotRef, operation.OwnerModuleID, operation.Kind)
				}
			}
			if !matched {
				return fmt.Errorf("admissibility snapshot %q is missing", snapshotRef)
			}
		}
	}
	return nil
}

func validateRuntimeCandidateContextIntegrity(
	definition runtimecontract.RuntimeDefinitionContext,
	context runtimecontract.RuntimeCandidateContext,
	attempt int,
	previousFailure *runtimecontract.RuntimeCandidateCorrectionObservation,
	budget *llmmodels.ProviderContextBudget,
	prompt *llmmodels.PromptContext,
) error {
	if budget == nil || prompt == nil {
		return fmt.Errorf("runtime candidate context integrity requires prompt and budget metadata")
	}
	missing := make([]string, 0, 5)
	contents := make(map[string]string, len(prompt.Sections))
	for _, section := range prompt.Sections {
		contents[promptSectionGroup(section.ID)] += strings.TrimSpace(section.Content)
	}
	for _, group := range []string{"C0", "C1", "C2", "C3", "C4"} {
		if strings.TrimSpace(contents[group]) == "" {
			missing = append(missing, group)
		}
	}
	definition = definition.Normalized()
	context = context.Normalized()
	if definition.GoalDefinition == "" || definition.DoDDefinition == "" || context.RequiredState == "" {
		missing = append(missing, "P0.contract")
	}
	if context.Focus.ID == "" || len(context.Focus.CoverageWorkItemIDs) == 0 {
		missing = append(missing, "P2.coverage_scope")
	}
	if context.Resources.ProviderCeiling == nil {
		missing = append(missing, "P4.current_snapshots")
	}
	if attempt > 0 && context.Failure == nil && previousFailure == nil {
		// The retry failure itself is supplied separately in C4, but an absent
		// current failure would make the repair authority inconsistent.
		missing = append(missing, "P1.current_failure")
	}
	if len(budget.RedactedPaths) > 0 && budget.RedactionApplied {
		missing = append(missing, "protected_context_redacted")
	}
	if len(missing) == 0 {
		return nil
	}
	budget.IntegrityWarnings = uniqueStrings(append(budget.IntegrityWarnings, missing...))
	return fmt.Errorf("runtime candidate context integrity missing or conflicted: %s", strings.Join(missing, ", "))
}

func populateRuntimeCandidateContextBudget(budget *llmmodels.ProviderContextBudget, prompt *llmmodels.PromptContext) {
	if budget == nil || prompt == nil {
		return
	}
	budget.IncludedSections = nil
	budget.SourceRefs = nil
	budget.ProtectedFields = nil
	for _, section := range prompt.Sections {
		if strings.TrimSpace(section.ID) == "" || strings.TrimSpace(section.Content) == "" {
			continue
		}
		budget.IncludedSections = append(budget.IncludedSections, section.ID)
		budget.SourceRefs = append(budget.SourceRefs, section.SourceRefs...)
		if section.Protected {
			switch promptSectionGroup(section.ID) {
			case "C0", "C1":
				budget.ProtectedFields = append(budget.ProtectedFields, "P0.contract")
			case "C3":
				budget.ProtectedFields = append(budget.ProtectedFields, "P2.coverage_scope", "P4.current_snapshots")
			case "C4":
				budget.ProtectedFields = append(budget.ProtectedFields, "P1.current_failure", "P3.current_evidence")
			}
		}
	}
	budget.IncludedSections = uniqueStrings(budget.IncludedSections)
	budget.SourceRefs = uniqueStrings(budget.SourceRefs)
	budget.ProtectedFields = uniqueStrings(budget.ProtectedFields)
}
