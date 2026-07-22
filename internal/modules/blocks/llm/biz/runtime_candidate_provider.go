package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type runtimeCandidateDefinitionProjection struct {
	Revision       string                                         `json:"revision"`
	GoalID         common.GoalID                                  `json:"goalId"`
	GoalDefinition string                                         `json:"goalDefinition"`
	DoDID          common.DoDID                                   `json:"doDId"`
	DoDDefinition  string                                         `json:"doDDefinition"`
	ChangeImpact   *runtimecontract.RuntimeDefinitionChangeImpact `json:"changeImpact,omitempty"`
}

type runtimeCandidateResourceProjection struct {
	Manifest                 *runtimecontract.WorkspaceManifest               `json:"manifest,omitempty"`
	TargetSnapshots          []runtimecontract.TargetSnapshot                 `json:"targetSnapshots,omitempty"`
	TargetSelection          *runtimecontract.RuntimeTargetSelection          `json:"targetSelection,omitempty"`
	FailureTarget            *runtimecontract.FailureTarget                   `json:"failureTarget,omitempty"`
	ValidationScratch        *runtimecontract.ValidationScratch               `json:"validationScratch,omitempty"`
	CapabilityStates         []runtimecontract.RuntimeCapabilityStateSnapshot `json:"capabilityStates,omitempty"`
	OperationAdmissibilities []runtimecontract.RuntimeOperationAdmissibility  `json:"operationAdmissibilities,omitempty"`
	// Freshness metadata is C3 authority input. It must never be moved into
	// C0-C2 because a resource refresh legitimately changes it between
	// transactions while the provider cache prefix stays stable.
	PrerequisiteObservations []runtimecontract.RuntimePrerequisiteObservation `json:"prerequisiteObservations,omitempty"`
}

type runtimeCandidateActivationProjection struct {
	DialogLanguage   string                                        `json:"dialogLanguage,omitempty"`
	CurrentStage     string                                        `json:"currentStage"`
	Soul             *runtimecontract.RuntimeSoulContext           `json:"soul,omitempty"`
	Skills           []runtimecontract.RuntimeSkillContext         `json:"skills,omitempty"`
	MCPServerIDs     []common.MCPServerID                          `json:"mcpServerIds,omitempty"`
	MCPCapabilities  []runtimecontract.RuntimeMCPCapabilityContext `json:"mcpCapabilities,omitempty"`
	Memories         []runtimecontract.RuntimeMemoryContext        `json:"memories,omitempty"`
	EvidencePolicies []runtimecontract.RuntimeEvidencePolicy       `json:"evidencePolicies,omitempty"`
	Continuation     runtimecontract.RuntimeContinuationContext    `json:"continuation"`
}

type runtimeCandidateContextProjection struct {
	WorkspaceID               common.WorkspaceID                              `json:"workspaceId"`
	RunID                     common.RunID                                    `json:"runId"`
	DefinitionRevision        string                                          `json:"definitionRevision"`
	EvidenceRevision          string                                          `json:"evidenceRevision"`
	Focus                     runtimecontract.CoverageFocus                   `json:"focus"`
	RequiredAction            runtimecontract.RuntimeAction                   `json:"requiredAction"`
	RequiredEffectScopes      []runtimecontract.RuntimeEffectScope            `json:"requiredEffectScopes,omitempty"`
	RequiredEvidenceKinds     []string                                        `json:"requiredEvidenceKinds,omitempty"`
	RepairDirective           *runtimecontract.RuntimeRepairDirective         `json:"repairDirective,omitempty"`
	MissingEvidenceKinds      []string                                        `json:"missingEvidenceKinds,omitempty"`
	AllowedEvidenceRefs       []string                                        `json:"allowedAcceptedEvidenceRefs,omitempty"`
	Evidence                  runtimecontract.EvidenceLedger                  `json:"evidence"`
	Assessments               []runtimecontract.RuntimeAssessmentFact         `json:"assessments,omitempty"`
	Failure                   *runtimecontract.FailureFact                    `json:"failure,omitempty"`
	RepairHints               []runtimecontract.RuntimeRepairHint             `json:"repairHints,omitempty"`
	AdmissibleOperationKinds  []string                                        `json:"admissibleOperationKinds,omitempty"`
	RejectedOperationShapes   []runtimecontract.RuntimeRejectedOperationShape `json:"rejectedOperationShapes,omitempty"`
	NoProgressOperationShapes []runtimecontract.RuntimeRejectedOperationShape `json:"noProgressOperationShapes,omitempty"`
	Capabilities              runtimecontract.RuntimeCapabilities             `json:"capabilities"`
	Resources                 runtimeCandidateResourceProjection              `json:"resources"`
	// TargetSelectionInstruction renders a sealed File-owner creation or bounded
	// feasibility scope as volatile C3 guidance. It neither creates a concrete
	// target fact nor grants any operation that the owner has not already
	// declared admissible.
	TargetSelectionInstruction string                               `json:"targetSelectionInstruction,omitempty"`
	Activation                 runtimeCandidateActivationProjection `json:"activation"`
}

type runtimeCandidateOutputContractProjection struct {
	Action                  runtimecontract.RuntimeAction                 `json:"action"`
	Schema                  json.RawMessage                               `json:"schema"`
	RequiredTopLevelExample string                                        `json:"requiredTopLevelExample"`
	ForbiddenTopLevelFields []string                                      `json:"forbiddenTopLevelFields"`
	OperationContracts      []runtimeCandidateOperationContractProjection `json:"operationContracts,omitempty"`
}

type runtimeCandidateOperationContractProjection struct {
	Kind                   string                                         `json:"kind"`
	Parameters             []runtimeCandidateOperationParameterProjection `json:"parameters"`
	RequireOneOfParameters []string                                       `json:"requireOneOfParameters,omitempty"`
	RequireTargetRefs      bool                                           `json:"requireTargetRefs,omitempty"`
	ForbiddenParameters    []string                                       `json:"forbiddenParameters,omitempty"`
	Variants               []runtimeCandidateOperationVariantProjection   `json:"variants,omitempty"`
	Rules                  []string                                       `json:"rules,omitempty"`
}

type runtimeCandidateOperationParameterProjection struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
}

type runtimeCandidateOperationVariantProjection struct {
	ID                  string                                               `json:"id"`
	Discriminators      []runtimecontract.RuntimeOperationParameterPredicate `json:"discriminators"`
	RequiredParameters  []string                                             `json:"requiredParameters,omitempty"`
	RequiredNonEmpty    []string                                             `json:"requiredNonEmpty,omitempty"`
	ForbiddenParameters []string                                             `json:"forbiddenParameters,omitempty"`
	EffectScope         runtimecontract.RuntimeEffectScope                   `json:"effectScope"`
	IdempotencyClass    runtimecontract.RuntimeOperationIdempotencyClass     `json:"idempotencyClass"`
}

type runtimeCandidateRetryProjection struct {
	PreviousFailure runtimecontract.RuntimeCandidateFailure `json:"previousFailure"`
	Instruction     string                                  `json:"instruction"`
}

func (m *Biz) GenerateCandidate(ctx context.Context, request llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error) {
	if m == nil {
		return llmmodels.RuntimeCandidateGenerationResult{
			Status: llmmodels.RuntimeCandidateStatusFailed, FailureClass: "internal", FailureReason: "runtime candidate provider is not configured",
		}, fmt.Errorf("runtime candidate provider is not configured")
	}
	startedAt := time.Now().UTC()
	requestID := newProviderRequestID(request)
	providerRequest, err := runtimeV2ProviderRequest(request)
	if err != nil {
		audit := sealedAuditFromRequest(request, requestID, nil, startedAt, time.Now().UTC(), false, llmmodels.RuntimeCandidateStatusFailed, "internal", 0)
		return llmmodels.RuntimeCandidateGenerationResult{
			RequestID: requestID, Audit: audit, Status: llmmodels.RuntimeCandidateStatusFailed,
			FailureClass: "internal", FailureReason: err.Error(),
		}, err
	}
	ceiling := request.CandidateContext.Resources.ProviderCeiling
	if ceiling == nil {
		err := fmt.Errorf("runtime candidate provider ceiling is missing")
		audit := sealedAuditFromRequest(request, requestID, providerRequest.ContextBudget, startedAt, time.Now().UTC(), false, llmmodels.RuntimeCandidateStatusFailed, "internal", 0)
		return llmmodels.RuntimeCandidateGenerationResult{
			RequestID: requestID, Audit: audit, Status: llmmodels.RuntimeCandidateStatusFailed,
			FailureClass: "internal", FailureReason: err.Error(),
		}, err
	}
	if err := ceiling.Validate(); err != nil {
		err = fmt.Errorf("runtime candidate provider ceiling invalid: %w", err)
		audit := sealedAuditFromRequest(request, requestID, providerRequest.ContextBudget, startedAt, time.Now().UTC(), false, llmmodels.RuntimeCandidateStatusFailed, "internal", 0)
		return llmmodels.RuntimeCandidateGenerationResult{
			RequestID: requestID, Audit: audit, Status: llmmodels.RuntimeCandidateStatusFailed,
			FailureClass: "internal", FailureReason: err.Error(),
		}, err
	}
	providerRequest.Model = ceiling.Target.ModelID
	if providerRequest.ContextBudget == nil {
		providerRequest.ContextBudget = &llmmodels.ProviderContextBudget{}
	}
	providerRequest.ContextBudget.ProviderID = ceiling.Target.ProviderID
	providerRequest.ContextBudget.ModelID = ceiling.Target.ModelID
	providerRequest.ContextBudget.CapabilityRevision = ceiling.CapabilityRevision
	providerRequest.ContextBudget.ConfigurationRevision = ceiling.ConfigurationRevision
	providerRequest.ContextBudget.ContextWindowTokens = ceiling.ContextWindowTokens
	providerRequest.ContextBudget.ProviderMaxOutputTokens = ceiling.ProviderMaxOutputTokens
	providerRequest.ContextBudget.EffectiveMaxOutputTokens = ceiling.EffectiveMaxOutputTokens
	providerRequest.ContextBudget.MaxBytes = ceiling.MaxBytes
	providerRequest.ContextBudget.MaxRequestBytes = ceiling.MaxBytes
	providerRequest.ContextBudget.MaxTokens = ceiling.MaxInputTokens
	providerRequest.ContextBudget.EstimatedOutputTokens = ceiling.EffectiveMaxOutputTokens
	providerRequest.ContextBudget.CapabilitySource = "sealed_ceiling"
	providerRequest.ContextBudget.ProfileMode = "runtime_v2_sealed_ceiling"

	// Sealed path: never re-resolve catalog/current profile.
	// Capture a pre-submit audit snapshot; completeProviderWithSealedCeiling mutates a local copy.
	budgetSnapshot := cloneProviderContextBudget(providerRequest.ContextBudget)
	completion, err := m.completeProviderWithSealedCeiling(ctx, *ceiling, providerRequest)
	completedAt := time.Now().UTC()
	if err != nil {
		stale := providerCeilingConfigurationStale(request, err)
		// Provider may have been contacted; mark submitted when the sealed path reached Complete.
		// completeProviderWithSealedCeiling only returns after client construction; treat as submitted
		// once we leave that call with a non-config error. For pre-dispatch errors ProviderSubmitted stays false.
		submitted := isProviderDispatchAttempted(err)
		if budgetSnapshot != nil {
			budgetSnapshot.ProviderSubmitted = submitted
		}
		failureClass := runtimeCandidateProviderFailureClass(err, submitted)
		audit := sealedAuditFromRequest(request, requestID, budgetSnapshot, startedAt, completedAt, submitted, llmmodels.RuntimeCandidateStatusFailed, string(failureClass), len(providerRequest.Schema))
		if budgetSnapshot != nil {
			audit.RequestBytes = budgetSnapshot.ActualBytes
		}
		return llmmodels.RuntimeCandidateGenerationResult{
			RequestID: requestID, Audit: audit, Status: llmmodels.RuntimeCandidateStatusFailed,
			FailureClass: string(failureClass), FailureReason: err.Error(), PrerequisiteStale: stale,
		}, err
	}
	if budgetSnapshot != nil {
		budgetSnapshot.ProviderSubmitted = true
		if completion.Usage != nil {
			budgetSnapshot.ActualTokens = completion.Usage.TotalTokens
		}
	}
	responseBytes := len(completion.Payload)
	audit := sealedAuditFromRequest(request, requestID, budgetSnapshot, startedAt, completedAt, true, llmmodels.RuntimeCandidateStatusCompleted, "", responseBytes)
	if budgetSnapshot != nil {
		audit.RequestBytes = budgetSnapshot.ActualBytes
	}
	var usage *llmmodels.ProviderUsage
	if completion.Usage != nil {
		cloned := *completion.Usage
		usage = &cloned
	}
	return llmmodels.RuntimeCandidateGenerationResult{
		RequestID: requestID,
		Payload:   append([]byte(nil), completion.Payload...),
		Usage:     usage,
		Audit:     audit,
		Status:    llmmodels.RuntimeCandidateStatusCompleted,
	}, nil
}

// providerCeilingConfigurationStale recognizes the one pre-dispatch LLM
// condition that is freshness rather than provider transport: the current
// provider configuration no longer matches the provider-ceiling prerequisite.
// The returned detail is consumed only by Orchestrator's generic invalidation
// path; this owner neither retries nor changes the sealed target.
func providerCeilingConfigurationStale(request llmmodels.RuntimeCandidateRequest, err error) *runtimecontract.RuntimePrerequisiteStale {
	var failure llmmodels.ProviderModelFailure
	if !errors.As(err, &failure) || failure.Code != llmmodels.ProviderModelFailureConfigurationDrift {
		return nil
	}
	ceiling := request.CandidateContext.Resources.ProviderCeiling
	if ceiling == nil {
		return nil
	}
	for _, observation := range request.CandidateContext.Resources.PrerequisiteObservations {
		observation = observation.Normalized()
		if observation.Kind != runtimecontract.PrerequisiteKindProviderCeiling || observation.OwnerModuleID != ModuleID {
			continue
		}
		for _, dependency := range observation.Dependencies {
			if dependency.Domain != runtimecontract.RuntimeResourceProviderConfig || dependency.OwnerModuleID != ModuleID || dependency.Revision != ceiling.ConfigurationRevision {
				continue
			}
			cfg, configErr := workorchconfig.Current()
			if configErr != nil {
				return nil
			}
			observed := TargetConfigurationRevision(cfg.LLM, ceiling.Target)
			if observed == "" || observed == dependency.Revision {
				return nil
			}
			stale := runtimecontract.RuntimePrerequisiteStale{
				PrerequisiteID: observation.PrerequisiteID, ObservationID: observation.ObservationID,
				Domain: dependency.Domain, OwnerModuleID: dependency.OwnerModuleID, SubjectRef: dependency.SubjectRef,
				ExpectedRevision: dependency.Revision, ObservedRevision: observed,
				Reason: "sealed provider configuration revision changed before candidate request",
			}
			if stale.Validate() == nil {
				return &stale
			}
			return nil
		}
	}
	return nil
}

func runtimeCandidateProviderFailureClass(err error, submitted bool) runtimecontract.RuntimeCandidateFailureClass {
	if err == nil {
		return runtimecontract.RuntimeCandidateFailureClassProviderTransient
	}
	var statusErr *ProviderHTTPStatusError
	if errors.As(err, &statusErr) {
		switch {
		case statusErr.StatusCode == 401 || statusErr.StatusCode == 403:
			return runtimecontract.RuntimeCandidateFailureClassProviderAuthentication
		case statusErr.StatusCode == 408 || statusErr.StatusCode == 409 || statusErr.StatusCode == 425 || statusErr.StatusCode == 429 || statusErr.StatusCode >= 500:
			return runtimecontract.RuntimeCandidateFailureClassProviderTransient
		default:
			return runtimecontract.RuntimeCandidateFailureClassProviderConfiguration
		}
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "authentication_failed") || strings.Contains(message, "invalid api key") || strings.Contains(message, "unauthorized") || strings.Contains(message, "forbidden") {
		return runtimecontract.RuntimeCandidateFailureClassProviderAuthentication
	}
	if !submitted || strings.Contains(message, "configuration") || strings.Contains(message, "provider client is not configured") || strings.Contains(message, "model not found") {
		return runtimecontract.RuntimeCandidateFailureClassProviderConfiguration
	}
	return runtimecontract.RuntimeCandidateFailureClassProviderTransient
}

func newProviderRequestID(request llmmodels.RuntimeCandidateRequest) string {
	workspaceID := string(request.CandidateContext.WorkspaceID)
	runID := string(request.CandidateContext.RunID)
	stepID := request.Transaction.AuthoritySnapshotID
	attempt := request.CandidateAttempt
	return fmt.Sprintf("llm-req-%s-%s-%s-%d-%d", workspaceID, runID, stepID, attempt, time.Now().UTC().UnixNano())
}

func sealedAuditFromRequest(
	request llmmodels.RuntimeCandidateRequest,
	requestID string,
	budget *llmmodels.ProviderContextBudget,
	startedAt, completedAt time.Time,
	submitted bool,
	status, failureClass string,
	responseBytes int,
) *llmmodels.ProviderRequestAudit {
	kind := "runtime_v2_candidate"
	if request.OutputContinuation != nil {
		kind = "runtime_v2_candidate_output_continuation"
	}
	ceiling := request.CandidateContext.Resources.ProviderCeiling
	target := runtimecontract.ProviderModelTarget{}
	capabilityRevision := ""
	configurationRevision := ""
	if ceiling != nil {
		target = ceiling.Target
		capabilityRevision = ceiling.CapabilityRevision
		configurationRevision = ceiling.ConfigurationRevision
	}
	if budget != nil {
		if budget.ProviderID != "" {
			target.ProviderID = budget.ProviderID
		}
		if budget.ModelID != "" {
			target.ModelID = budget.ModelID
		}
		if budget.CapabilityRevision != "" {
			capabilityRevision = budget.CapabilityRevision
		}
		if budget.ConfigurationRevision != "" {
			configurationRevision = budget.ConfigurationRevision
		}
	}
	durationMs := int(completedAt.Sub(startedAt).Milliseconds())
	if durationMs < 0 {
		durationMs = 0
	}
	requestBytes := 0
	if budget != nil {
		requestBytes = budget.ActualBytes
	}
	return &llmmodels.ProviderRequestAudit{
		RequestID:             requestID,
		WorkspaceID:           string(request.CandidateContext.WorkspaceID),
		RunID:                 string(request.CandidateContext.RunID),
		StepID:                request.Transaction.AuthoritySnapshotID,
		Target:                target,
		Operation:             "chat_completions",
		CapabilityRevision:    capabilityRevision,
		ConfigurationRevision: configurationRevision,
		StablePrefixHash:      request.StablePrefixHash,
		Kind:                  kind,
		ContextBudget:         cloneProviderContextBudget(budget),
		ProviderSubmitted:     submitted,
		Status:                status,
		FailureClass:          failureClass,
		StartedAt:             startedAt.Format(time.RFC3339Nano),
		CompletedAt:           completedAt.Format(time.RFC3339Nano),
		DurationMs:            durationMs,
		RequestBytes:          requestBytes,
		ResponseBytes:         responseBytes,
	}
}

func cloneProviderContextBudget(budget *llmmodels.ProviderContextBudget) *llmmodels.ProviderContextBudget {
	if budget == nil {
		return nil
	}
	cloned := *budget
	if budget.CachePlan != nil {
		plan := *budget.CachePlan
		plan.SectionHashes = append([]llmmodels.ProviderCacheSectionHash(nil), budget.CachePlan.SectionHashes...)
		plan.CacheBreakers = append([]llmmodels.ProviderCacheBreaker(nil), budget.CachePlan.CacheBreakers...)
		plan.Canonicalization.SectionOrder = append([]string(nil), budget.CachePlan.Canonicalization.SectionOrder...)
		plan.AdapterHint.CacheablePrefix = append([]string(nil), budget.CachePlan.AdapterHint.CacheablePrefix...)
		plan.AdapterHint.VolatileSections = append([]string(nil), budget.CachePlan.AdapterHint.VolatileSections...)
		plan.AdapterHint.CacheBreakers = append([]string(nil), budget.CachePlan.AdapterHint.CacheBreakers...)
		cloned.CachePlan = &plan
	}
	cloned.Rules = append([]string(nil), budget.Rules...)
	cloned.SourceRefs = append([]string(nil), budget.SourceRefs...)
	cloned.IncludedSections = append([]string(nil), budget.IncludedSections...)
	cloned.NormalizedSections = append([]string(nil), budget.NormalizedSections...)
	cloned.CompressedSections = append([]string(nil), budget.CompressedSections...)
	cloned.OmittedSections = append([]string(nil), budget.OmittedSections...)
	cloned.RedactedPaths = append([]string(nil), budget.RedactedPaths...)
	cloned.ProtectedFields = append([]string(nil), budget.ProtectedFields...)
	cloned.IntegrityWarnings = append([]string(nil), budget.IntegrityWarnings...)
	return &cloned
}

// isProviderDispatchAttempted reports whether a sealed completion error likely occurred after
// the HTTP/SDK client was invoked. Pre-dispatch configuration/client errors stay unsubmitted.
func isProviderDispatchAttempted(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	// Configuration / client construction failures never reach the network.
	for _, marker := range []string{
		"provider client is not configured",
		"configuration revision",
		"provider_configuration_drift",
		"sealed ceiling",
		"llm config",
		"resolve provider",
		"api key",
		"baseurl",
		"base url",
	} {
		if strings.Contains(message, marker) {
			return false
		}
	}
	return true
}

func runtimeV2ProviderRequest(request llmmodels.RuntimeCandidateRequest) (llmmodels.ProviderRequest, error) {
	definitionContext := request.DefinitionContext.Normalized()
	candidateContext := request.CandidateContext.Normalized()
	wantPrefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(definitionContext, candidateContext.Activation.ExecutionContract)
	if strings.TrimSpace(request.StablePrefixHash) != wantPrefix {
		return llmmodels.ProviderRequest{}, fmt.Errorf("runtime candidate stable prefix mismatch")
	}
	ceiling := candidateContext.Resources.ProviderCeiling
	if ceiling == nil || ceiling.MaxBytes <= 0 {
		return llmmodels.ProviderRequest{}, fmt.Errorf("runtime candidate provider ceiling is missing")
	}
	if err := ceiling.Target.Validate(); err != nil {
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
	dynamicJSON, err := json.Marshal(struct {
		Decision           runtimecontract.RuntimeDecision                            `json:"decision"`
		Transaction        runtimecontract.AuthorityTransaction                       `json:"transaction"`
		Attempt            int                                                        `json:"candidateAttempt"`
		OutputContract     runtimeCandidateOutputContractProjection                   `json:"outputContract"`
		Retry              *runtimeCandidateRetryProjection                           `json:"retry,omitempty"`
		Context            runtimeCandidateContextProjection                          `json:"context"`
		OutputContinuation *runtimecontract.RuntimeCandidateOutputContinuationRequest `json:"outputContinuation,omitempty"`
	}{
		request.Decision,
		request.Transaction,
		request.CandidateAttempt,
		runtimeV2CandidateOutputContract(request.Decision, candidateContext.Resources.TargetSelection),
		runtimeV2CandidateRetry(request.CandidateAttempt, request.PreviousFailure),
		runtimeV2CandidateContextProjection(candidateContext, request.Decision.Action),
		request.OutputContinuation,
	})
	if err != nil {
		return llmmodels.ProviderRequest{}, err
	}
	providerRequest := llmmodels.ProviderRequest{
		WorkspaceID: string(candidateContext.WorkspaceID), RunID: string(candidateContext.RunID),
		StepID: request.Transaction.AuthoritySnapshotID, Kind: "runtime_v2_candidate",
		Schema: runtimeV2CandidateSchemaForDecisionWithTargetSelection(request.Decision, candidateContext.Resources.TargetSelection),
		PromptContext: &llmmodels.PromptContext{SchemaVersion: "workorch.runtime_candidate_prompt.v2", Sections: []llmmodels.PromptSection{
			{ID: "C0.runtime-candidate-contract", Name: "Runtime candidate contract", Role: "system", Cacheability: "stable", Protected: true, DisplayOrder: 0, Content: runtimeV2CandidateContractCatalog()},
			{ID: "C1.goal-dod", Name: "Goal and DoD", Role: "system", Cacheability: "stable", Protected: true, DisplayOrder: 1, DefinitionRevision: definitionContext.Revision, Content: string(definitionJSON)},
			{ID: "C2.capability-contract", Name: "Capability contract", Role: "system", Cacheability: "stable", Protected: true, DisplayOrder: 2, Content: string(capabilityJSON)},
			{ID: "C3.runtime-authority", Name: "Current runtime authority", Role: "user", Cacheability: "volatile", Protected: true, DisplayOrder: 3, Content: string(dynamicJSON)},
		}},
		ContextBudget: &llmmodels.ProviderContextBudget{
			ProviderID: ceiling.Target.ProviderID, ModelID: ceiling.Target.ModelID,
			CapabilityRevision: ceiling.CapabilityRevision, ConfigurationRevision: ceiling.ConfigurationRevision,
			ContextWindowTokens: ceiling.ContextWindowTokens, ProviderMaxOutputTokens: ceiling.ProviderMaxOutputTokens,
			EffectiveMaxOutputTokens: ceiling.EffectiveMaxOutputTokens,
			MaxBytes:                 ceiling.MaxBytes, MaxRequestBytes: ceiling.MaxBytes, MaxTokens: ceiling.MaxInputTokens,
			EstimatedOutputTokens: ceiling.EffectiveMaxOutputTokens,
			CapabilitySource:      "sealed_ceiling", ProfileMode: "runtime_v2_typed_ceiling",
			CachePlan: &llmmodels.ProviderCachePlan{
				SchemaVersion: "workorch.provider_cache_plan.v1", Strategy: "stable_prefix_dynamic_tail",
				StablePrefixHash: wantPrefix, RenderedStablePrefixHash: wantPrefix,
			},
		},
	}
	if request.OutputContinuation != nil {
		providerRequest.Kind = "runtime_v2_candidate_output_continuation"
		providerRequest.Schema = ""
		providerRequest.RawTextOutput = true
		providerRequest.PromptContext.Sections[3].Content = string(dynamicJSON) + "\n\nReturn only the exact next bytes of the candidate JSON document after outputContinuation.previousChunkTail. Do not repeat the tail, do not start a new JSON object, and do not use markdown."
	}
	payload, err := json.Marshal(providerRequest)
	if err != nil {
		return llmmodels.ProviderRequest{}, err
	}
	providerRequest.ContextBudget.ActualBytes = len(payload)
	providerRequest.ContextBudget.OverBudget = len(payload) > ceiling.MaxBytes
	if providerRequest.ContextBudget.OverBudget {
		providerRequest.ContextBudget.ContextGateAction = "fail_fast"
		providerRequest.ContextBudget.ContextGateReason = "runtime_v2_provider_ceiling_exceeded"
		providerRequest.ContextBudget.BudgetFailureClass = "provider_ceiling_exceeded"
		return llmmodels.ProviderRequest{}, fmt.Errorf("runtime candidate request exceeds provider ceiling: %d > %d", len(payload), ceiling.MaxBytes)
	}
	return providerRequest, nil
}

func runtimeV2CandidateContextProjection(context runtimecontract.RuntimeCandidateContext, action runtimecontract.RuntimeAction) runtimeCandidateContextProjection {
	context = context.Normalized()
	activation := context.Activation
	stage := runtimeV2CandidateStage(action)
	facts := runtimecontract.RuntimeFacts{
		WorkspaceID: context.WorkspaceID, RunID: context.RunID, DefinitionRevision: context.DefinitionRevision,
		EvidenceRevision: context.EvidenceRevision, Focus: context.Focus, RemainingFocuses: context.RemainingFocuses,
		Evidence: context.Evidence, Assessments: context.Assessments, Failure: context.Failure,
		Capabilities: context.Capabilities, Resources: context.Resources, Activation: context.Activation,
		RepairDirective: context.RepairDirective,
	}.Normalized()
	return runtimeCandidateContextProjection{
		WorkspaceID: context.WorkspaceID, RunID: context.RunID,
		DefinitionRevision: context.DefinitionRevision, EvidenceRevision: context.EvidenceRevision,
		Focus:          context.Focus,
		RequiredAction: context.RequiredAction, RequiredEffectScopes: append([]runtimecontract.RuntimeEffectScope(nil), context.RequiredEffectScopes...),
		RequiredEvidenceKinds: append([]string(nil), context.RequiredEvidenceKinds...), RepairDirective: context.RepairDirective,
		MissingEvidenceKinds: runtimecontract.MissingRequiredEvidenceKinds(facts),
		AllowedEvidenceRefs:  runtimecontract.AcceptedEvidenceRefsForFocus(facts), Evidence: context.Evidence,
		Assessments: context.Assessments, Failure: context.Failure, RepairHints: runtimecontract.DeriveRuntimeRepairHints(facts),
		AdmissibleOperationKinds:  runtimeV2CandidateAllowedOperationKinds(action, context.AdmissibleOperationKinds, nil, context.Resources.TargetSelection),
		RejectedOperationShapes:   context.RejectedOperationShapes,
		NoProgressOperationShapes: context.NoProgressOperationShapes,
		Capabilities:              context.Capabilities,
		Resources: runtimeCandidateResourceProjection{
			Manifest: context.Resources.Manifest, TargetSnapshots: context.Resources.TargetSnapshots,
			TargetSelection: context.Resources.TargetSelection,
			FailureTarget:   context.Resources.FailureTarget, ValidationScratch: context.Resources.ValidationScratch,
			CapabilityStates: context.Resources.CapabilityStates, OperationAdmissibilities: context.Resources.OperationAdmissibilities,
			PrerequisiteObservations: context.Resources.PrerequisiteObservations,
		},
		TargetSelectionInstruction: runtimeV2TargetSelectionInstruction(context.Resources.TargetSelection, action),
		Activation: runtimeCandidateActivationProjection{
			DialogLanguage: activation.DialogLanguage, CurrentStage: stage, Soul: runtimeV2CandidateSoul(activation, stage),
			Skills: activation.Skills, MCPServerIDs: activation.MCPServerIDs,
			MCPCapabilities: runtimecontract.RuntimeMCPCapabilitiesForAction(activation.MCPCapabilities, action),
			Memories:        activation.Memories, EvidencePolicies: activation.EvidencePolicies, Continuation: activation.Continuation,
		},
	}
}

// runtimeV2TargetSelectionInstruction converts the persisted File-owner
// selection disposition into provider-facing C3 guidance. A create or complete
// scope is intentionally not a concrete repair target: a candidate may propose
// one concrete relative path only when the sealed authority contains no
// higher-precedence concrete target. Only a durable File-owner operation is
// admissible in that case, and the File owner remains the final path authority.
func runtimeV2TargetSelectionInstruction(selection *runtimecontract.RuntimeTargetSelection, action runtimecontract.RuntimeAction) string {
	if selection == nil || action != runtimecontract.RuntimeActionProduceStateChange {
		return ""
	}
	value := selection.Normalized()
	if value.OwnerModuleID != runtimecontract.RuntimeModuleFile {
		return ""
	}
	switch value.Disposition {
	case runtimecontract.RuntimeTargetSelectionCreate:
		return "The File owner observed an empty workspace creation scope. Propose at least one durable File-owner operation with a concrete workspace-relative targetRefs entry and matching target parameter. Choose that proposed path from the Goal and DoD; do not leave it blank, and do not use this scope to authorize Git, Shell, MCP, or another owner. The File owner will validate the proposed path before execution."
	case runtimecontract.RuntimeTargetSelectionScope:
		return "The File owner provided a bounded workspace feasibility scope, not a selected repair target. Follow any concrete target already sealed in the Focus, FailureTarget, current failure, or RepairDirective. Only when none is present, propose at least one durable File-owner operation with a concrete workspace-relative targetRefs entry and matching target parameter; that target may be a new file and remains subject to File-owner feasibility. Do not use this scope to authorize Git, Shell, MCP, or another owner."
	default:
		return ""
	}
}

func runtimeV2CandidateStage(action runtimecontract.RuntimeAction) string {
	switch action {
	case runtimecontract.RuntimeActionProduceStateChange:
		return "execution"
	case runtimecontract.RuntimeActionProduceValidation, runtimecontract.RuntimeActionProduceAssessment:
		return "validation"
	case runtimecontract.RuntimeActionRequestGovernance:
		return "decision"
	default:
		return ""
	}
}

func runtimeV2CandidateSoul(activation runtimecontract.RuntimeActivationContext, stage string) *runtimecontract.RuntimeSoulContext {
	for _, binding := range activation.StageSoulBindings {
		if binding.Stage != stage {
			continue
		}
		value := binding.Soul
		return &value
	}
	if activation.DefaultSoul == nil {
		return nil
	}
	value := *activation.DefaultSoul
	return &value
}

func runtimeV2CandidateSchema(action runtimecontract.RuntimeAction) string {
	switch action {
	case runtimecontract.RuntimeActionProduceStateChange:
		return runtimeV2OperationCandidateSchema("workorch.runtime_candidate.state_change.v2", "Runtime V2 durable state-change candidate", "At least one operation must have a sealed durable effect compatible with requiredEffectScopes.")
	case runtimecontract.RuntimeActionProduceValidation:
		return runtimeV2OperationCandidateSchema("workorch.runtime_candidate.validation.v2", "Runtime V2 non-durable validation candidate", "Every operation must have a sealed none or explicitly accepted ephemeral_session effect; durable and unknown effects are forbidden.")
	case runtimecontract.RuntimeActionProduceAssessment:
		return `{"type":"object","additionalProperties":false,"required":["focusId","coverageWorkItemIds","disposition","summary"],"properties":{"focusId":{"type":"string"},"sliceId":{"type":"string"},"coverageWorkItemIds":{"type":"array","minItems":1,"items":{"type":"string"}},"disposition":{"type":"string","enum":["satisfied","repairable_gap","governance_required","terminal_rejection"]},"items":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["coverageWorkItemId","satisfied"],"properties":{"coverageWorkItemId":{"type":"string"},"satisfied":{"type":"boolean"},"reason":{"type":"string"},"evidenceRefs":{"type":"array","items":{"type":"string"}}}}},"gap":{"type":"object","additionalProperties":false,"properties":{"coverageWorkItemIds":{"type":"array","items":{"type":"string"}},"missingEvidenceKinds":{"type":"array","items":{"type":"string"}},"targetRefs":{"type":"array","items":{"type":"string"}},"evidenceRefs":{"type":"array","items":{"type":"string"}},"reason":{"type":"string"},"failureClass":{"type":"string"},"failureCode":{"type":"string"},"failureSignature":{"type":"string"}}},"summary":{"type":"string"},"evidenceRefs":{"type":"array","items":{"type":"string"}},"artifactRefs":{"type":"array","items":{"type":"string"}}},"allOf":[{"if":{"properties":{"disposition":{"const":"satisfied"}},"required":["disposition"]},"then":{"required":["evidenceRefs"],"properties":{"evidenceRefs":{"type":"array","minItems":1,"items":{"type":"string"}}}}}]}`
	case runtimecontract.RuntimeActionRequestGovernance:
		return `{"type":"object","additionalProperties":false,"required":["externalDependency"],"properties":{"externalDependency":{"type":"object","additionalProperties":false,"required":["kind","reason"],"properties":{"kind":{"type":"string"},"subjectRef":{"type":"string"},"reason":{"type":"string"}}}}}`
	default:
		return ""
	}
}

// runtimeV2CandidateSchemaForDecision refines only the volatile provider
// output schema from the sealed operation contracts. The stable C0-C2 prompt
// prefix remains unchanged; admissible operation kinds belong to C3 authority.
func runtimeV2CandidateSchemaForDecision(decision runtimecontract.RuntimeDecision) string {
	return runtimeV2CandidateSchemaForDecisionWithTargetSelection(decision, nil)
}

// runtimeV2CandidateSchemaForDecisionWithTargetSelection narrows the volatile
// provider schema when the persisted File-owner selection is a creation/scope
// observation. That observation is not a generic durable-side-effect grant:
// until a concrete File target has been proposed and owner-validated, only a
// durable File operation is a legal candidate shape. This is request rendering
// only; Runtime V2 admission remains the final SM-3 authority.
func runtimeV2CandidateSchemaForDecisionWithTargetSelection(decision runtimecontract.RuntimeDecision, selection *runtimecontract.RuntimeTargetSelection) string {
	base := runtimeV2CandidateSchema(decision.Action)
	if decision.Action != runtimecontract.RuntimeActionProduceStateChange && decision.Action != runtimecontract.RuntimeActionProduceValidation {
		return base
	}
	var schema map[string]any
	if err := json.Unmarshal([]byte(base), &schema); err != nil {
		return base
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return base
	}
	operations, ok := properties["operations"].(map[string]any)
	if !ok {
		return base
	}
	items, ok := operations["items"].(map[string]any)
	if !ok {
		return base
	}
	allowedKinds := runtimeV2CandidateAllowedOperationKinds(decision.Action, decision.RequiredOutput.AcceptedKinds, decision.RequiredOutput.RequiredOperationSequence, selection)
	conditions := runtimeV2OperationSchemaConditions(
		allowedKinds,
		decision.RequiredOutput.AcceptedOperationVariants,
		decision.RequiredOutput.RequiredOperationSequence,
	)
	if itemProperties, ok := items["properties"].(map[string]any); ok {
		itemProperties["kind"] = map[string]any{"type": "string", "enum": allowedKinds}
	}
	if len(conditions) > 0 {
		items["allOf"] = conditions
	}
	if len(decision.RequiredOutput.RequiredOperationSequence) > 0 {
		requirements := make([]any, 0, len(decision.RequiredOutput.RequiredOperationSequence))
		for _, requirement := range decision.RequiredOutput.RequiredOperationSequence {
			requirements = append(requirements, map[string]any{"contains": map[string]any{
				"type": "object", "properties": map[string]any{
					"requiredOperationRef": map[string]any{"const": requirement.RequirementID},
					"kind":                 map[string]any{"const": requirement.Kind},
				}, "required": []string{"requiredOperationRef", "kind"},
			}})
		}
		operations["allOf"] = requirements
	}
	encoded, err := json.Marshal(schema)
	if err != nil {
		return base
	}
	return string(encoded)
}

// runtimeV2CandidateAllowedOperationKinds projects a persisted File-owner
// creation/scope fact into the C3 provider contract. It does not discover a
// target, choose an action, or modify the sealed decision. The File owner and
// Orchestrator admission still validate the concrete target and operation.
func runtimeV2CandidateAllowedOperationKinds(action runtimecontract.RuntimeAction, kinds []string, requiredSequence []runtimecontract.RuntimeRequiredOperationVariant, selection *runtimecontract.RuntimeTargetSelection) []string {
	if action != runtimecontract.RuntimeActionProduceStateChange || selection == nil {
		return append([]string(nil), kinds...)
	}
	value := selection.Normalized()
	if value.OwnerModuleID != runtimecontract.RuntimeModuleFile ||
		(value.Disposition != runtimecontract.RuntimeTargetSelectionCreate && value.Disposition != runtimecontract.RuntimeTargetSelectionScope) {
		return append([]string(nil), kinds...)
	}
	seen := make(map[string]struct{}, len(kinds))
	ret := make([]string, 0, len(kinds))
	for _, raw := range kinds {
		descriptor, ok := runtimecontract.RuntimeOperationDescriptorForKind(raw)
		if !ok || descriptor.OwnerModuleID != runtimecontract.RuntimeModuleFile || !runtimecontract.RuntimeEffectIsDurable(descriptor.DefaultEffectScope) {
			continue
		}
		if _, duplicate := seen[descriptor.Kind]; duplicate {
			continue
		}
		seen[descriptor.Kind] = struct{}{}
		ret = append(ret, descriptor.Kind)
	}
	// A required sequence is an explicit sealed authority of its own. Keep its
	// kinds so that File-scope narrowing cannot make a valid multi-owner
	// sequence schema self-contradictory; the scope still does not authorize any
	// arbitrary operation outside that sequence.
	for _, requirement := range requiredSequence {
		descriptor, ok := runtimecontract.RuntimeOperationDescriptorForKind(requirement.Kind)
		if !ok {
			continue
		}
		if _, duplicate := seen[descriptor.Kind]; duplicate {
			continue
		}
		seen[descriptor.Kind] = struct{}{}
		ret = append(ret, descriptor.Kind)
	}
	return ret
}

func runtimeV2OperationSchemaConditions(
	kinds []string,
	acceptedVariants []runtimecontract.RuntimeOperationVariantRef,
	requiredSequence []runtimecontract.RuntimeRequiredOperationVariant,
) []any {
	allowedVariants := make(map[string]map[string]struct{}, len(acceptedVariants)+len(requiredSequence))
	for _, ref := range runtimecontract.NormalizeRuntimeOperationVariantRefs(acceptedVariants) {
		if allowedVariants[ref.Kind] == nil {
			allowedVariants[ref.Kind] = map[string]struct{}{}
		}
		allowedVariants[ref.Kind][ref.VariantID] = struct{}{}
	}
	for _, requirement := range requiredSequence {
		kind := runtimecontract.CanonicalRuntimeBlockKind(requirement.Kind)
		variantID := strings.TrimSpace(requirement.VariantID)
		if kind == "" || variantID == "" {
			continue
		}
		if allowedVariants[kind] == nil {
			allowedVariants[kind] = map[string]struct{}{}
		}
		allowedVariants[kind][variantID] = struct{}{}
	}
	seen := map[string]struct{}{}
	ret := make([]any, 0, len(kinds))
	for _, requestedKind := range kinds {
		contract, ok := runtimecontract.RuntimeOperationContractForKind(requestedKind)
		if !ok {
			continue
		}
		if _, duplicate := seen[contract.Kind]; duplicate {
			continue
		}
		seen[contract.Kind] = struct{}{}
		parameterNames := make([]string, 0, len(contract.Parameters))
		parameterTypes := make(map[string]runtimecontract.RuntimeOperationParameterType, len(contract.Parameters))
		parameterRequirements := make([]any, 0, len(contract.Parameters)+1)
		for _, parameter := range contract.Parameters {
			parameterNames = append(parameterNames, parameter.Name)
			parameterTypes[parameter.Name] = parameter.Type
			if parameter.Required {
				parameterRequirements = append(parameterRequirements, runtimeV2ParameterContainsSchema(parameter.Name))
			}
		}
		if len(contract.RequireOneOfParameters) > 0 {
			oneOf := make([]any, 0, len(contract.RequireOneOfParameters))
			for _, name := range contract.RequireOneOfParameters {
				// A one-of target must have a value. Presence-only validation
				// admitted targetRef="", which then failed only after generation.
				// This is provider-facing schema rendering; DTO admission remains
				// the authoritative Runtime V2 contract gate.
				oneOf = append(oneOf, runtimeV2NonEmptyParameterContainsSchema(name, parameterTypes[name]))
			}
			parameterRequirements = append(parameterRequirements, map[string]any{"anyOf": oneOf})
		}
		parameterSchema := map[string]any{
			"type":  "array",
			"items": map[string]any{"properties": map[string]any{"name": map[string]any{"enum": parameterNames}}},
		}
		if len(parameterRequirements) > 0 {
			parameterSchema["allOf"] = parameterRequirements
		}
		if len(contract.Variants) > 0 {
			variantSchemas := make([]any, 0, len(contract.Variants))
			constrained := allowedVariants[contract.Kind]
			for _, variant := range contract.Variants {
				if len(constrained) > 0 {
					if _, allowed := constrained[variant.ID]; !allowed {
						continue
					}
				}
				variantSchemas = append(variantSchemas, runtimeV2OperationVariantParameterSchema(contract.Kind, variant))
			}
			// A sealed ref unknown to the descriptor must fail closed at the
			// provider boundary too. Omitting oneOf here would silently widen the
			// schema back to every descriptor variant.
			if len(constrained) > 0 || len(variantSchemas) > 0 {
				parameterSchema["oneOf"] = variantSchemas
			}
		}
		thenProperties := map[string]any{"parameters": parameterSchema}
		then := map[string]any{"properties": thenProperties}
		if contract.RequireTargetRefs {
			then["required"] = []string{"targetRefs"}
			thenProperties["targetRefs"] = map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"type": "string"}}
		}
		ret = append(ret, map[string]any{
			"if":   map[string]any{"properties": map[string]any{"kind": map[string]any{"const": contract.Kind}}, "required": []string{"kind"}},
			"then": then,
		})
	}
	return ret
}

func runtimeV2ParameterContainsSchema(name string) map[string]any {
	return map[string]any{
		"contains": map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"const": name}}, "required": []string{"name"}},
	}
}

// runtimeV2NonEmptyParameterContainsSchema keeps a descriptor's one-of
// parameter requirement aligned with the operation DTO: a selected canonical
// target cannot be an empty string. It only renders a provider schema and
// never discovers targets or changes Runtime V2 state.
func runtimeV2NonEmptyParameterContainsSchema(name string, parameterType runtimecontract.RuntimeOperationParameterType) map[string]any {
	properties := map[string]any{"name": map[string]any{"const": name}}
	required := []string{"name"}
	switch parameterType {
	case runtimecontract.RuntimeOperationParameterTypeString:
		properties["string"] = map[string]any{"type": "string", "minLength": 1}
		required = append(required, "string")
	case runtimecontract.RuntimeOperationParameterTypeStringList:
		properties["stringList"] = map[string]any{"type": "array", "minItems": 1}
		required = append(required, "stringList")
	case runtimecontract.RuntimeOperationParameterTypeInteger:
		properties["integer"] = map[string]any{"type": "integer"}
		required = append(required, "integer")
	case runtimecontract.RuntimeOperationParameterTypeBoolean:
		properties["boolean"] = map[string]any{"type": "boolean"}
		required = append(required, "boolean")
	default:
		return runtimeV2ParameterContainsSchema(name)
	}
	return map[string]any{"contains": map[string]any{"type": "object", "properties": properties, "required": required}}
}

func runtimeV2OperationVariantParameterSchema(kind string, variant runtimecontract.RuntimeOperationVariantContract) map[string]any {
	conditions := make([]any, 0, len(variant.Discriminators)+len(variant.RequiredParameters)+len(variant.RequiredNonEmpty)+len(variant.ForbiddenParameters))
	for _, discriminator := range variant.Discriminators {
		conditions = append(conditions, map[string]any{
			"contains": map[string]any{
				"type": "object", "properties": map[string]any{
					"name":   map[string]any{"const": discriminator.Name},
					"string": map[string]any{"const": discriminator.EqualsString},
				},
				"required": []string{"name", "string"},
			},
		})
	}
	for _, name := range variant.RequiredParameters {
		conditions = append(conditions, runtimeV2ParameterContainsSchema(name))
	}
	for _, name := range variant.RequiredNonEmpty {
		conditions = append(conditions, map[string]any{
			"contains": map[string]any{
				"type": "object", "allOf": []any{
					map[string]any{"properties": map[string]any{"name": map[string]any{"const": name}}, "required": []string{"name"}},
					map[string]any{"anyOf": []any{
						map[string]any{"properties": map[string]any{"string": map[string]any{"type": "string", "minLength": 1}}, "required": []string{"string"}},
						map[string]any{"properties": map[string]any{"stringList": map[string]any{"type": "array", "minItems": 1}}, "required": []string{"stringList"}},
					}},
				},
			},
		})
	}
	for _, name := range variant.ForbiddenParameters {
		conditions = append(conditions, map[string]any{
			"not": map[string]any{"contains": map[string]any{
				"type": "object", "properties": map[string]any{"name": map[string]any{"const": name}}, "required": []string{"name"},
			}},
		})
	}
	return map[string]any{
		"title": "operation variant contract: " + runtimecontract.RuntimeOperationVariantRuleID(kind, variant.ID, "contract", ""),
		"allOf": conditions,
	}
}

func runtimeV2OperationCandidateSchema(id, title, description string) string {
	value := map[string]any{
		"$id": id, "title": title, "description": description,
		"type": "object", "additionalProperties": false, "required": []string{"operations"},
		"properties": map[string]any{
			"operations": map[string]any{
				"type": "array", "minItems": 1,
				"items": map[string]any{
					"type": "object", "additionalProperties": false,
					"required": []string{"id", "kind", "coverageWorkItemIds"},
					"properties": map[string]any{
						"id": map[string]any{"type": "string"}, "kind": map[string]any{"type": "string"},
						"requiredOperationRef": map[string]any{"type": "string"},
						"dependsOn":            map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						"coverageWorkItemIds":  map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"type": "string"}},
						"targetRefs":           map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						"parameters": map[string]any{"type": "array", "items": map[string]any{
							"type": "object", "additionalProperties": false, "required": []string{"name"},
							"properties": map[string]any{
								"name": map[string]any{"type": "string"}, "string": map[string]any{"type": "string"},
								"integer": map[string]any{"type": "integer"}, "boolean": map[string]any{"type": "boolean"},
								"stringList": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
							},
						}},
					},
				},
			},
		},
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func runtimeV2CandidateContractCatalog() string {
	return strings.Join([]string{
		"Return exactly one JSON object for the requested Runtime V2 action. The JSON object itself is the candidate; never wrap it in another object.",
		"The only legal top-level shapes are:",
		"produce_state_change: " + runtimeV2CandidateSchema(runtimecontract.RuntimeActionProduceStateChange),
		"produce_validation: " + runtimeV2CandidateSchema(runtimecontract.RuntimeActionProduceValidation),
		"produce_assessment: " + runtimeV2CandidateSchema(runtimecontract.RuntimeActionProduceAssessment),
		"request_governance: " + runtimeV2CandidateSchema(runtimecontract.RuntimeActionRequestGovernance),
		"Forbidden top-level fields include candidate, candidates, executionGraph, convergenceState, focus, authority, transaction, outputMode, result, and runtimeState.",
		"Use the requested action, required output kinds, coverage work item IDs, and available capabilities from the current runtime authority. Do not invent or modify convergence state, focus, authority, transaction, or output mode. Do not use markdown fences or explanatory text.",
		"context.requiredAction is the sole next-action authority. When context.repairDirective is present, obey its requiredAction, requiredEffectScopes, requiredEvidenceKinds, allowedOperationKinds, targetRefs, and rejectedOperationShapes.",
		"context.missingEvidenceKinds is completion-audit information only and must never override context.requiredAction or context.repairDirective.",
		"Use operationContracts for allowed parameters of each kind. Dynamic fields are only valid where the contract permits them.",
		"Operation parameters are a closed typed whitelist: omit every field that is not listed by the selected operationContract. Do not add narrative, summary, explanation, metadata, or convenience parameters unless that exact parameter is explicitly listed by the selected operationContract.",
		"For an operation expected to produce a durable effect, targetRefs must name concrete declared targets that the operation itself will actually change. Do not label a read, check, preparation, or no-op action as state-changing, and do not cite unrelated targets as durable-effect witnesses.",
		"When an operation has a canonical targetRef or path parameter, copy that exact value into its top-level targetRefs. targetRefs may declare a concrete new workspace-relative file before it exists; the owning capability validates creation at execution time.",
		"When context.repairHints is present, follow those domain-neutral hints and do not repeat an active rejected operation shape.",
		"When context.rejectedOperationShapes is present, every listed operation hash and exact structural operation is forbidden until later accepted evidence supersedes it. Choose an admissible kind and change kind, targets, or execution-semantic parameters.",
		"For a satisfied assessment, evidenceRefs is mandatory and every value must be copied exactly from context.allowedAcceptedEvidenceRefs. Satisfied item evidenceRefs must use the same allowed set.",
	}, "\n")
}

func runtimeV2CandidateOutputContract(decision runtimecontract.RuntimeDecision, selection *runtimecontract.RuntimeTargetSelection) runtimeCandidateOutputContractProjection {
	allowedKinds := runtimeV2CandidateAllowedOperationKinds(decision.Action, decision.RequiredOutput.AcceptedKinds, decision.RequiredOutput.RequiredOperationSequence, selection)
	return runtimeCandidateOutputContractProjection{
		Action:                  decision.Action,
		Schema:                  json.RawMessage(runtimeV2CandidateSchemaForDecisionWithTargetSelection(decision, selection)),
		RequiredTopLevelExample: runtimeV2CandidateTopLevelExample(decision.Action),
		ForbiddenTopLevelFields: []string{"candidate", "candidates", "executionGraph", "convergenceState", "focus", "authority", "transaction", "outputMode", "result", "runtimeState"},
		OperationContracts:      runtimeV2CandidateOperationContracts(allowedKinds),
	}
}

func runtimeV2CandidateOperationContracts(kinds []string) []runtimeCandidateOperationContractProjection {
	ret := make([]runtimeCandidateOperationContractProjection, 0, len(kinds))
	for _, kind := range kinds {
		contract, ok := runtimecontract.RuntimeOperationContractForKind(kind)
		if !ok {
			continue
		}
		projection := runtimeCandidateOperationContractProjection{
			Kind: contract.Kind, RequireOneOfParameters: append([]string(nil), contract.RequireOneOfParameters...), RequireTargetRefs: contract.RequireTargetRefs,
			ForbiddenParameters: append([]string(nil), contract.ForbiddenParameters...), Rules: append([]string(nil), contract.Rules...),
			Parameters: make([]runtimeCandidateOperationParameterProjection, 0, len(contract.Parameters)),
		}
		if len(contract.Variants) > 0 {
			projection.Variants = make([]runtimeCandidateOperationVariantProjection, 0, len(contract.Variants))
			for _, variant := range contract.Variants {
				projection.Variants = append(projection.Variants, runtimeCandidateOperationVariantProjection{
					ID: variant.ID, Discriminators: append([]runtimecontract.RuntimeOperationParameterPredicate(nil), variant.Discriminators...),
					RequiredParameters: append([]string(nil), variant.RequiredParameters...), RequiredNonEmpty: append([]string(nil), variant.RequiredNonEmpty...),
					ForbiddenParameters: append([]string(nil), variant.ForbiddenParameters...), EffectScope: variant.EffectScope, IdempotencyClass: variant.IdempotencyClass,
				})
			}
		}
		for _, parameter := range contract.Parameters {
			projection.Parameters = append(projection.Parameters, runtimeCandidateOperationParameterProjection{
				Name: parameter.Name, Type: string(parameter.Type), Required: parameter.Required, Description: parameter.Description,
			})
		}
		ret = append(ret, projection)
	}
	return ret
}

func runtimeV2CandidateTopLevelExample(action runtimecontract.RuntimeAction) string {
	switch action {
	case runtimecontract.RuntimeActionProduceStateChange, runtimecontract.RuntimeActionProduceValidation:
		return `{"operations":[...]}`
	case runtimecontract.RuntimeActionProduceAssessment:
		return `{"focusId":"...","coverageWorkItemIds":["..."],"disposition":"repairable_gap","gap":{"coverageWorkItemIds":["..."],"missingEvidenceKinds":["validation"],"reason":"..."},"summary":"..."}`
	case runtimecontract.RuntimeActionRequestGovernance:
		return `{"externalDependency":{"kind":"...","reason":"..."}}`
	default:
		return `{}`
	}
}

func runtimeV2CandidateRetry(attempt int, failure *runtimecontract.RuntimeCandidateFailure) *runtimeCandidateRetryProjection {
	if attempt != 1 || failure == nil {
		return nil
	}
	previous := *failure
	previous.Reason = boundedRuntimeCandidateFailureReason(previous.Reason)
	instruction := "Retry the exact same sealed authority contract. Return only the legal top-level object for the requested action."
	if previous.Class == runtimecontract.RuntimeCandidateFailureClassSchema || previous.Class == runtimecontract.RuntimeCandidateFailureClassContract {
		instruction += " The previous candidate shape was rejected; do not repeat any wrapper or field named candidate, candidates, or executionGraph."
	}
	if previous.Class == runtimecontract.RuntimeCandidateFailureClassInfeasible {
		instruction += " The previous operation was owner-infeasible before dispatch seal. Choose only targets, selectors, and parameters in the current operationAdmissibilities inventory; do not repeat its failure signature."
	}
	if runtimeV2CandidateFailureIsOperationShapeReplay(previous.Code) {
		instruction += " The previous candidate replayed a rejected or no-progress operation shape. Inspect context.rejectedOperationShapes and context.noProgressOperationShapes, then choose a materially different operation shape that changes its declared durable targets. Changing only an operation ID, narrative, dependency list, or cosmetic parameter is not sufficient."
	}
	instruction += runtimeV2CandidateRetryParameterHint(previous.Reason)
	return &runtimeCandidateRetryProjection{PreviousFailure: previous, Instruction: instruction}
}

func runtimeV2CandidateFailureIsOperationShapeReplay(code string) bool {
	switch strings.TrimSpace(code) {
	case "candidate_replays_rejected_shape", "candidate_partially_replays_rejected_shape", "candidate_replays_no_progress_shape":
		return true
	default:
		return false
	}
}

func runtimeV2CandidateRetryParameterHint(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return ""
	}
	lower := strings.ToLower(reason)
	if strings.Contains(lower, "targetrefs must contain targetref") || strings.Contains(lower, "targetrefs must contain path") || strings.Contains(lower, "targetrefs must contain target ") {
		return " Declare exactly one non-empty canonical targetRef or path parameter, then copy that exact value into the operation's top-level targetRefs. A concrete new workspace-relative target is valid even when the file does not yet exist."
	}
	if strings.Contains(lower, "does not target the sealed focus, failure, or repair target set") {
		return " The authority already has a sealed concrete target set. Return a durable operation that actually changes one of the target refs in context.focus, context.resources.failureTarget, or context.repairDirective, and copy that exact target into top-level targetRefs. A build, test, read, or command without that durable target effect cannot satisfy this state-change action."
	}
	if strings.Contains(lower, "owner-grounded workspace scope permits only a durable file operation") || strings.Contains(lower, "creation scope requires a durable file operation") {
		return " The current target-selection scope permits only a durable File-owner operation. Use one of the File operation contracts, declare a concrete workspace-relative targetRefs value that the operation changes, and do not use Shell, Git, MCP, Web, or SSH until File owner evidence establishes a concrete target."
	}
	if strings.Contains(lower, "does not accept parameter") || strings.Contains(lower, "requires parameter") {
		return " Remove every rejected parameter entirely and rebuild the operation from only the exact parameter names listed by its operationContract. Do not replace a rejected parameter with summary, narrative, explanation, metadata, or another invented field; operation-level commentary is never a legal substitute for a typed parameter."
	}
	if strings.Contains(lower, "conflict") || strings.Contains(lower, "not found") || strings.Contains(lower, "mismatch") {
		return " Re-read current target content and realign operation inputs before retrying a conflicting state-changing operation."
	}
	return ""
}

func boundedRuntimeCandidateFailureReason(reason string) string {
	const maxBytes = 1024
	reason = strings.TrimSpace(reason)
	if len(reason) <= maxBytes {
		return reason
	}
	return reason[:maxBytes] + "...[truncated]"
}
