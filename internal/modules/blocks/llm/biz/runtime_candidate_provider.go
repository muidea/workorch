package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/llmaudit"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (m *Biz) GenerateCandidate(ctx context.Context, request llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error) {
	if m == nil {
		return llmmodels.RuntimeCandidateGenerationResult{
			Status: llmmodels.RuntimeCandidateStatusFailed, FailureClass: "internal", FailureReason: "runtime candidate provider is not configured",
		}, fmt.Errorf("runtime candidate provider is not configured")
	}
	startedAt := time.Now().UTC()
	requestID := newProviderRequestID(request)
	providerRequest, err := runtimeProviderRequest(request)
	if err != nil {
		budget := providerRequest.ContextBudget
		audit := sealedAuditFromRequest(request, requestID, budget, startedAt, time.Now().UTC(), false, llmmodels.RuntimeCandidateStatusFailed, "internal", 0)
		return llmmodels.RuntimeCandidateGenerationResult{
			RequestID: requestID, Audit: audit, Status: llmmodels.RuntimeCandidateStatusFailed,
			FailureClass: "internal", FailureReason: err.Error(),
		}, err
	}
	ceiling := runtimeRequestProviderCeiling(request)
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
	providerRequest.Model = ceiling.Selection.Target.ModelID
	providerRequest.MaxOutputTokens = ceiling.EffectiveMaxOutputTokens
	if request.ExpectedDocument != nil {
		providerRequest.MaxOutputTokens = request.ExpectedDocument.Normalized().MaxOutputTokens
	}
	if providerRequest.ContextBudget == nil {
		providerRequest.ContextBudget = &llmmodels.ProviderContextBudget{}
	}
	providerRequest.ContextBudget.ProviderID = ceiling.Selection.Target.ProviderID
	providerRequest.ContextBudget.ModelID = ceiling.Selection.Target.ModelID
	providerRequest.ContextBudget.CapabilityRevision = ceiling.CapabilityRevision
	providerRequest.ContextBudget.ConfigurationRevision = ceiling.ConfigurationRevision
	providerRequest.ContextBudget.ContextWindowTokens = ceiling.ContextWindowTokens
	providerRequest.ContextBudget.ProviderMaxOutputTokens = ceiling.ProviderMaxOutputTokens
	providerRequest.ContextBudget.EffectiveMaxOutputTokens = ceiling.EffectiveMaxOutputTokens
	providerRequest.ContextBudget.MaxBytes = ceiling.MaxBytes
	providerRequest.ContextBudget.MaxRequestBytes = ceiling.MaxBytes
	providerRequest.ContextBudget.MaxTokens = ceiling.MaxInputTokens
	providerRequest.ContextBudget.EstimatedOutputTokens = ceiling.EffectiveMaxOutputTokens
	providerRequest.ContextBudget.EffectiveMaxOutputTokens = providerRequest.MaxOutputTokens
	providerRequest.ContextBudget.EstimatedOutputTokens = providerRequest.MaxOutputTokens
	providerRequest.ContextBudget.CapabilitySource = "sealed_ceiling"
	providerRequest.ContextBudget.ProfileMode = "runtime_sealed_ceiling"

	// Sealed path: never re-resolve catalog/current profile.
	// Capture a pre-submit audit snapshot; completeProviderWithSealedCeiling mutates a local copy.
	budgetSnapshot := cloneProviderContextBudget(providerRequest.ContextBudget)
	completion, execution, err := m.completeProviderWithSealedCeiling(ctx, *ceiling, providerRequest)
	completedAt := time.Now().UTC()
	if err != nil {
		var limitErr *ProviderOutputLimitError
		if errors.As(err, &limitErr) {
			if budgetSnapshot != nil {
				budgetSnapshot.ProviderSubmitted = execution.Submitted
				if completion.Usage != nil {
					budgetSnapshot.ActualTokens = completion.Usage.TotalTokens
				}
			}
			// The provider transaction completed with an explicit incomplete
			// document outcome. Keep that factual outcome orthogonal to protocol
			// failure: the Runtime owner decides bounded whole-document recovery.
			audit := sealedAuditFromRequest(request, requestID, budgetSnapshot, startedAt, completedAt, execution.Submitted, llmmodels.RuntimeCandidateStatusCompleted, "", completion.ResponseBytes)
			audit.PhysicalResponseMode = canonicalProviderPhysicalResponseMode(execution)
			audit.RequestBytes = providerAuditRequestBytes(completion.RequestBytes, budgetSnapshot)
			usage := completion.Usage
			if usage != nil {
				cloned := *usage
				usage = &cloned
			}
			return llmmodels.RuntimeCandidateGenerationResult{
				RequestID: requestID, Payload: append([]byte(nil), completion.Payload...), Usage: usage,
				Audit: audit, Status: llmmodels.RuntimeCandidateStatusCompleted,
				ReachedProviderLimit: true,
			}, err
		}
		stale := m.providerCeilingConfigurationStale(request, err)
		// The protocol adapter reports submission independently at the exact
		// http.Client.Do boundary; neither request bytes nor error text may be
		// used to reconstruct this fact.
		submitted := execution.Submitted
		if budgetSnapshot != nil {
			budgetSnapshot.ProviderSubmitted = submitted
		}
		failureClass := runtimeCandidateProviderFailureClass(err, submitted)
		responseBytes := max(completion.ResponseBytes, providerResponseBytesFromError(err))
		audit := sealedAuditFromRequest(request, requestID, budgetSnapshot, startedAt, completedAt, submitted, llmmodels.RuntimeCandidateStatusFailed, string(failureClass), responseBytes)
		audit.PhysicalResponseMode = canonicalProviderPhysicalResponseMode(execution)
		audit.RequestBytes = providerAuditRequestBytes(completion.RequestBytes, budgetSnapshot)
		return llmmodels.RuntimeCandidateGenerationResult{
			RequestID: requestID, Audit: audit, Status: llmmodels.RuntimeCandidateStatusFailed,
			FailureClass: string(failureClass), FailureReason: err.Error(), PrerequisiteStale: stale,
		}, err
	}
	if budgetSnapshot != nil {
		budgetSnapshot.ProviderSubmitted = execution.Submitted
		if completion.Usage != nil {
			budgetSnapshot.ActualTokens = completion.Usage.TotalTokens
		}
	}
	responseBytes := completion.ResponseBytes
	audit := sealedAuditFromRequest(request, requestID, budgetSnapshot, startedAt, completedAt, execution.Submitted, llmmodels.RuntimeCandidateStatusCompleted, "", responseBytes)
	audit.PhysicalResponseMode = canonicalProviderPhysicalResponseMode(execution)
	audit.RequestBytes = providerAuditRequestBytes(completion.RequestBytes, budgetSnapshot)
	var usage *llmaudit.ProviderUsage
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

// canonicalProviderPhysicalResponseMode keeps the sealed, explicit
// non-streaming preflight fact when no physical request was submitted. Once
// the adapter reports the independent client.Do submission fact,
// providerExecutionMetadata carries the exact dispatch path selected by this
// owner. This avoids treating serialized bytes or a zero completion returned
// by an early guard as a physical response observation.
func canonicalProviderPhysicalResponseMode(execution providerExecutionMetadata) llmaudit.PhysicalResponseMode {
	if !execution.Submitted {
		return llmaudit.PhysicalResponseModeNonStreaming
	}
	return execution.PhysicalResponseMode
}

func providerAuditRequestBytes(adapterBytes int, budget *llmmodels.ProviderContextBudget) int {
	if adapterBytes > 0 {
		return adapterBytes
	}
	if budget != nil {
		return max(budget.ActualBytes, 0)
	}
	return 0
}

// providerCeilingConfigurationStale recognizes the one pre-dispatch LLM
// condition that is freshness rather than provider transport: the current
// provider configuration no longer matches the provider-ceiling prerequisite.
// The returned detail is consumed only by Orchestrator's generic invalidation
// path; this owner neither retries nor changes the sealed target.
func (m *Biz) providerCeilingConfigurationStale(request llmmodels.RuntimeCandidateRequest, err error) *runtimecontract.RuntimePrerequisiteStale {
	var failure llmmodels.ProviderModelFailure
	if !errors.As(err, &failure) || failure.Code != llmmodels.ProviderModelFailureConfigurationDrift {
		return nil
	}
	ceiling := runtimeRequestProviderCeiling(request)
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
			cfg, configErr := m.activeConfiguration()
			if configErr != nil {
				return nil
			}
			observed := TargetConfigurationRevision(cfg, ceiling.Selection.Target)
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
	var protocolErr *ProviderProtocolError
	if errors.As(err, &protocolErr) {
		return runtimecontract.RuntimeCandidateFailureClassProviderProtocol
	}
	var outputLimitErr *ProviderOutputLimitError
	if errors.As(err, &outputLimitErr) {
		return runtimecontract.RuntimeCandidateFailureClassProviderProtocol
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
	if physicalCallID := strings.TrimSpace(request.RequestIdentity.PhysicalCallID); physicalCallID != "" {
		return physicalCallID
	}
	workspaceID, runID := runtimeRequestScope(request)
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
	kind := "runtime_candidate"
	if request.DocumentKind == runtimecontract.RuntimeProviderDocumentSemanticPlan {
		kind = "runtime_semantic_plan"
	}
	ceiling := runtimeRequestProviderCeiling(request)
	target := runtimecontract.ProviderModelTarget{}
	capabilityRevision := ""
	configurationRevision := ""
	if ceiling != nil {
		target = ceiling.Selection.Target
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
	if budget == nil && ceiling != nil {
		budget = &llmmodels.ProviderContextBudget{
			ProviderID:               ceiling.Selection.Target.ProviderID,
			ModelID:                  ceiling.Selection.Target.ModelID,
			CapabilityRevision:       ceiling.CapabilityRevision,
			ConfigurationRevision:    ceiling.ConfigurationRevision,
			ContextWindowTokens:      ceiling.ContextWindowTokens,
			ProviderMaxOutputTokens:  ceiling.ProviderMaxOutputTokens,
			EffectiveMaxOutputTokens: ceiling.EffectiveMaxOutputTokens,
			MaxBytes:                 ceiling.MaxBytes,
			MaxTokens:                ceiling.MaxInputTokens,
			MaxRequestBytes:          ceiling.MaxBytes,
			ProfileMode:              "runtime_sealed_ceiling",
			CapabilitySource:         "sealed_ceiling",
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
	workspaceID, runID := runtimeRequestScope(request)
	auditBudget := cloneProviderContextBudget(budget)
	if auditBudget != nil {
		auditBudget.ProviderID = target.ProviderID
		auditBudget.ModelID = target.ModelID
		auditBudget.CapabilityRevision = capabilityRevision
		auditBudget.ConfigurationRevision = configurationRevision
		auditBudget.ProviderSubmitted = submitted
	}
	if physicalCallID := strings.TrimSpace(request.RequestIdentity.PhysicalCallID); physicalCallID != "" {
		requestID = physicalCallID
	}
	return &llmmodels.ProviderRequestAudit{
		RequestID:             requestID,
		LogicalRequestID:      request.RequestIdentity.LogicalRequestID,
		PhysicalCallID:        request.RequestIdentity.PhysicalCallID,
		WorkspaceID:           workspaceID,
		RunID:                 runID,
		StepID:                request.Transaction.AuthoritySnapshotID,
		Target:                target,
		Operation:             workorchconfig.LLMOperationResponses,
		CapabilityRevision:    capabilityRevision,
		ConfigurationRevision: configurationRevision,
		StablePrefixHash:      request.StablePrefixHash,
		Kind:                  kind,
		PhysicalResponseMode:  llmaudit.PhysicalResponseModeNonStreaming,
		ContextBudget:         auditBudget,
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
