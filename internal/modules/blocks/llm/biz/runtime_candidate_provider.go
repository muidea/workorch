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
	DialogLanguage    string                                        `json:"dialogLanguage,omitempty"`
	CurrentStage      string                                        `json:"currentStage"`
	Soul              *runtimecontract.RuntimeSoulContext           `json:"soul,omitempty"`
	Skills            []runtimecontract.RuntimeSkillContext         `json:"skills,omitempty"`
	MCPServerIDs      []common.MCPServerID                          `json:"mcpServerIds,omitempty"`
	MCPCapabilities   []runtimecontract.RuntimeMCPCapabilityContext `json:"mcpCapabilities,omitempty"`
	Memories          []runtimecontract.RuntimeMemoryContext        `json:"memories,omitempty"`
	EvidencePolicyIDs []string                                      `json:"evidencePolicyIds,omitempty"`
	Continuation      *runtimecontract.RuntimeContinuationContext   `json:"continuation,omitempty"`
}

type runtimeCandidateContextProjection struct {
	WorkspaceID               common.WorkspaceID                              `json:"workspaceId"`
	RunID                     common.RunID                                    `json:"runId"`
	DefinitionRevision        string                                          `json:"definitionRevision"`
	EvidenceRevision          string                                          `json:"evidenceRevision"`
	Focus                     runtimecontract.CoverageFocus                   `json:"focus"`
	RequiredState             runtimecontract.ConvergenceState                `json:"requiredState"`
	RequiredEffectScopes      []runtimecontract.RuntimeEffectScope            `json:"requiredEffectScopes,omitempty"`
	RequiredEvidenceKinds     []string                                        `json:"requiredEvidenceKinds,omitempty"`
	RepairDirective           *runtimecontract.RuntimeRepairDirective         `json:"repairDirective,omitempty"`
	MissingEvidenceKinds      []string                                        `json:"missingEvidenceKinds,omitempty"`
	AllowedEvidenceRefs       []string                                        `json:"allowedAcceptedEvidenceRefs,omitempty"`
	Evidence                  runtimecontract.EvidenceLedger                  `json:"evidence"`
	Assessments               []runtimecontract.RuntimeAssessmentFact         `json:"assessments,omitempty"`
	Failure                   *runtimecontract.FailureFact                    `json:"failure,omitempty"`
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
	State                       runtimecontract.ConvergenceState              `json:"state"`
	EffectPolicy                runtimeCandidateEffectPolicyProjection        `json:"effectPolicy"`
	ManifestHash                string                                        `json:"manifestHash,omitempty"`
	Schema                      json.RawMessage                               `json:"schema"`
	StructuredOutputInstruction string                                        `json:"structuredOutputInstruction"`
	ForbiddenTopLevelFields     []string                                      `json:"forbiddenTopLevelFields"`
	OperationContracts          []runtimeCandidateOperationContractProjection `json:"operationContracts,omitempty"`
}

type runtimeCandidateEffectPolicyProjection struct {
	ExecutableOperationsAllowed   bool                                 `json:"executableOperationsAllowed"`
	AllowedOperationEffects       []runtimecontract.RuntimeEffectScope `json:"allowedOperationEffects,omitempty"`
	AllowedAcceptedReceiptEffects []runtimecontract.RuntimeEffectScope `json:"allowedAcceptedReceiptEffects,omitempty"`
}

type runtimeCandidateOperationContractProjection struct {
	Kind                        string                                            `json:"kind"`
	OwnerModuleID               string                                            `json:"ownerModuleId,omitempty"`
	AllowedOperationFields      []string                                          `json:"allowedOperationFields,omitempty"`
	AllowedVariantIDs           []string                                          `json:"allowedVariantIds,omitempty"`
	Parameters                  []runtimeCandidateOperationParameterProjection    `json:"parameters"`
	RequireOneOfParameters      []string                                          `json:"requireOneOfParameters,omitempty"`
	TargetCardinality           runtimecontract.RuntimeOperationTargetCardinality `json:"targetCardinality,omitempty"`
	RequireTargetRefs           bool                                              `json:"requireTargetRefs,omitempty"`
	ForbiddenParameters         []string                                          `json:"forbiddenParameters,omitempty"`
	Variants                    []runtimeCandidateOperationVariantProjection      `json:"variants,omitempty"`
	Rules                       []string                                          `json:"rules,omitempty"`
	AllowsDynamicParameters     bool                                              `json:"allowsDynamicParameters,omitempty"`
	DynamicParameterPrefixes    []string                                          `json:"dynamicParameterPrefixes,omitempty"`
	RequiredOperationReferences []string                                          `json:"requiredOperationReferences,omitempty"`
	AcceptedEffectScopes        []runtimecontract.RuntimeEffectScope              `json:"acceptedEffectScopes,omitempty"`
	RequiredEffectScopes        []runtimecontract.RuntimeEffectScope              `json:"requiredEffectScopes,omitempty"`
	// Target is the sealed TargetPolicy authorization compiled by the single
	// manifest compiler. Renderer/schema/instruction must consume it instead of
	// re-deriving owner inventory or inventing a second allow-list.
	Target                    runtimeCandidateOperationTargetProjection `json:"target"`
	PrerequisiteSnapshotRefs  []string                                  `json:"prerequisiteSnapshotRefs,omitempty"`
	AdmissibilitySnapshotRefs []string                                  `json:"admissibilitySnapshotRefs,omitempty"`
	// ValidationRecipeIDs exposes only selection identity. Full command, args
	// and timeout remain activation-sealed owner facts consumed by Shell; a
	// provider must never receive a second command-construction interface.
	ValidationRecipeIDs       []string `json:"validationRecipeIds,omitempty"`
	OperationContractRevision string   `json:"operationContractRevision,omitempty"`
}

// runtimeCandidateOperationTargetProjection is the provider-facing pure view of
// one sealed TargetPolicy. It carries membership facts only; no raw owner
// error text and no domain heuristics.
type runtimeCandidateOperationTargetProjection struct {
	Policy                    runtimecontract.RuntimeOperationTargetPolicy `json:"policy"`
	ExactTargetRefs           []string                                     `json:"exactTargetRefs,omitempty"`
	OwnerScopeTargetRefs      []string                                     `json:"ownerScopeTargetRefs,omitempty"`
	OwnerSelectorRefs         []string                                     `json:"ownerSelectorRefs,omitempty"`
	ProspectiveConstraintRefs []string                                     `json:"prospectiveConstraintRefs,omitempty"`
	ProspectiveTargetRef      string                                       `json:"prospectiveTargetRef,omitempty"`
	AllowedTargetRefs         []string                                     `json:"allowedTargetRefs,omitempty"`
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
	PreviousFailure runtimeCandidateCorrectionFailureProjection `json:"previousFailure"`
	Instruction     string                                      `json:"instruction"`
}

// runtimeCandidateCorrectionFailureProjection is the only representation of
// an earlier rejection allowed into C4. It intentionally has no Reason field:
// raw provider/parser/owner text remains in persisted audit, never in the next
// model prompt.
type runtimeCandidateCorrectionFailureProjection struct {
	Class      runtimecontract.RuntimeCandidateFailureClass       `json:"class"`
	Code       string                                             `json:"code,omitempty"`
	Signature  string                                             `json:"signature,omitempty"`
	TargetRefs []string                                           `json:"targetRefs,omitempty"`
	Diagnostic *runtimecontract.RuntimeCandidateFailureDiagnostic `json:"diagnostic,omitempty"`
}

// runtimeCandidateAuthorityProjection is C3: the current sealed authority
// needed to choose one legal candidate. Historical evidence and retry
// correction intentionally live in C4 so they cannot pollute the stable
// prefix or obscure the current focus.
type runtimeCandidateAuthorityProjection struct {
	WorkspaceID                common.WorkspaceID                      `json:"workspaceId"`
	RunID                      common.RunID                            `json:"runId"`
	DefinitionRevision         string                                  `json:"definitionRevision"`
	EvidenceRevision           string                                  `json:"evidenceRevision"`
	Focus                      runtimecontract.CoverageFocus           `json:"focus"`
	RequiredState              runtimecontract.ConvergenceState        `json:"requiredState"`
	RequiredEffectScopes       []runtimecontract.RuntimeEffectScope    `json:"requiredEffectScopes,omitempty"`
	RequiredEvidenceKinds      []string                                `json:"requiredEvidenceKinds,omitempty"`
	RepairDirective            *runtimecontract.RuntimeRepairDirective `json:"repairDirective,omitempty"`
	AllowedEvidenceRefs        []string                                `json:"allowedAcceptedEvidenceRefs,omitempty"`
	AdmissibleOperationKinds   []string                                `json:"admissibleOperationKinds,omitempty"`
	Capabilities               runtimecontract.RuntimeCapabilities     `json:"capabilities"`
	Resources                  runtimeCandidateResourceProjection      `json:"resources"`
	TargetSelectionInstruction string                                  `json:"targetSelectionInstruction,omitempty"`
	Activation                 runtimeCandidateActivationProjection    `json:"activation"`
}

// runtimeCandidateExecutionTailProjection is C4: bounded current failure,
// evidence and retry context. It never changes C0-C2 and never carries raw
// provider payloads.
type runtimeCandidateExecutionTailProjection struct {
	Attempt                   int                                                        `json:"candidateAttempt"`
	Retry                     *runtimeCandidateRetryProjection                           `json:"retry,omitempty"`
	Failure                   *runtimeCandidateProviderSafeFailureProjection             `json:"failure,omitempty"`
	AcceptedEvidence          []runtimeCandidateProviderSafeEvidenceProjection           `json:"latestAcceptedEvidence,omitempty"`
	RejectedEvidence          []runtimeCandidateProviderSafeEvidenceProjection           `json:"latestRejectedEvidence,omitempty"`
	Assessments               []runtimeCandidateProviderSafeAssessmentProjection         `json:"recentAssessments,omitempty"`
	RejectedOperationShapes   []runtimeCandidateProviderSafeOperationShapeProjection     `json:"rejectedOperationShapes,omitempty"`
	NoProgressOperationShapes []runtimeCandidateProviderSafeOperationShapeProjection     `json:"noProgressOperationShapes,omitempty"`
	OutputContinuation        *runtimecontract.RuntimeCandidateOutputContinuationRequest `json:"outputContinuation,omitempty"`
}

// The following C4 values are deliberately narrower than the persisted
// Runtime facts. C4 is a provider-facing diagnostic tail, not an aggregate
// mirror: raw messages/summaries, governance/recovery continuation and any
// Runtime action or retry policy stay with their owning state machine.
type runtimeCandidateProviderSafeFailureProjection struct {
	Class      runtimecontract.FailureClass `json:"class,omitempty"`
	Code       string                       `json:"code,omitempty"`
	Signature  string                       `json:"signature,omitempty"`
	TargetRefs []string                     `json:"targetRefs,omitempty"`
}

type runtimeCandidateProviderSafeEvidenceProjection struct {
	ID                  common.EvidenceRecordID      `json:"id,omitempty"`
	Kind                string                       `json:"kind,omitempty"`
	SubjectRef          string                       `json:"subjectRef,omitempty"`
	AuthoritySnapshotID string                       `json:"authoritySnapshotId,omitempty"`
	DispatchID          string                       `json:"dispatchId,omitempty"`
	OperationID         string                       `json:"operationId,omitempty"`
	Revision            string                       `json:"revision,omitempty"`
	FailureClass        runtimecontract.FailureClass `json:"failureClass,omitempty"`
	FailureCode         string                       `json:"failureCode,omitempty"`
	FailureSignature    string                       `json:"failureSignature,omitempty"`
	ContentHash         string                       `json:"contentHash,omitempty"`
	ExitCode            *int64                       `json:"exitCode,omitempty"`
	EvidenceRefs        []string                     `json:"evidenceRefs,omitempty"`
	TargetRefs          []string                     `json:"targetRefs,omitempty"`
	CoverageWorkItemIDs []common.CoverageWorkItemID  `json:"coverageWorkItemIds,omitempty"`
	Quality             common.EvidenceQuality       `json:"quality,omitempty"`
	ZeroWork            bool                         `json:"zeroWork,omitempty"`
}

type runtimeCandidateProviderSafeAssessmentProjection struct {
	ID                  string                                `json:"id,omitempty"`
	FocusID             string                                `json:"focusId,omitempty"`
	SliceID             string                                `json:"sliceId,omitempty"`
	RequirementID       string                                `json:"requirementId,omitempty"`
	CoverageWorkItemIDs []common.CoverageWorkItemID           `json:"coverageWorkItemIds,omitempty"`
	Disposition         runtimecontract.AssessmentDisposition `json:"disposition,omitempty"`
	EvidenceRefs        []string                              `json:"evidenceRefs,omitempty"`
	ArtifactRefs        []string                              `json:"artifactRefs,omitempty"`
}

type runtimeCandidateProviderSafeOperationShapeProjection struct {
	Hash             string                       `json:"hash,omitempty"`
	Kind             string                       `json:"kind,omitempty"`
	TargetRefs       []string                     `json:"targetRefs,omitempty"`
	FailureClass     runtimecontract.FailureClass `json:"failureClass,omitempty"`
	FailureCode      string                       `json:"failureCode,omitempty"`
	FailureSignature string                       `json:"failureSignature,omitempty"`
}

// runtimeCandidateLargeRefsProjection is C5. Large/history material remains
// reference-only: the LLM gets identities and counts, never raw snapshot or
// evidence content through this section.
type runtimeCandidateLargeRefsProjection struct {
	RemainingFocuses []runtimeCandidateFocusReference `json:"remainingFocuses,omitempty"`
}

type runtimeCandidateFocusReference struct {
	ID      string `json:"id"`
	SliceID string `json:"sliceId,omitempty"`
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
		budget := providerRequest.ContextBudget
		audit := sealedAuditFromRequest(request, requestID, budget, startedAt, time.Now().UTC(), false, llmmodels.RuntimeCandidateStatusFailed, "internal", 0)
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
	if physicalCallID := strings.TrimSpace(request.PhysicalCallID); physicalCallID != "" {
		return physicalCallID
	}
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
	// The Orchestrator compiles this sealed manifest before request-session
	// persistence. The renderer consumes that exact value; it must not rebuild
	// field/parameter rules from a parallel descriptor path.
	manifest := request.CandidateManifest
	if err := runtimecontract.ValidateRuntimeCandidateContractManifestForDecision(manifest, request.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromContext(candidateContext)); err != nil {
		return llmmodels.ProviderRequest{}, fmt.Errorf("runtime candidate contract manifest: %w", err)
	}
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
	authorityJSON, err := json.Marshal(struct {
		Decision       runtimecontract.RuntimeDecision          `json:"decision"`
		Transaction    runtimecontract.AuthorityTransaction     `json:"transaction"`
		OutputContract runtimeCandidateOutputContractProjection `json:"outputContract"`
		Context        runtimeCandidateAuthorityProjection      `json:"context"`
	}{
		request.Decision,
		request.Transaction,
		runtimeV2CandidateOutputContractForManifest(manifest),
		runtimeV2CandidateAuthorityContextProjection(candidateContext, manifest),
	})
	if err != nil {
		return llmmodels.ProviderRequest{}, err
	}
	executionTailJSON, err := json.Marshal(runtimeV2CandidateExecutionTailProjection(candidateContext, request.CandidateAttempt, request.PreviousFailure, request.OutputContinuation, manifest))
	if err != nil {
		return llmmodels.ProviderRequest{}, err
	}
	largeRefsJSON, err := json.Marshal(runtimeV2CandidateLargeRefsProjection(candidateContext))
	if err != nil {
		return llmmodels.ProviderRequest{}, err
	}
	providerRequest := llmmodels.ProviderRequest{
		WorkspaceID: string(candidateContext.WorkspaceID), RunID: string(candidateContext.RunID),
		StepID: request.Transaction.AuthoritySnapshotID, Kind: "runtime_v2_candidate",
		Schema: runtimeV2CandidateSchemaForManifest(manifest),
		PromptContext: &llmmodels.PromptContext{SchemaVersion: "workorch.runtime_candidate_prompt.v3", Sections: []llmmodels.PromptSection{
			{ID: "C0.runtime-candidate-contract", Name: "Runtime candidate contract", Role: "system", Cacheability: "stable", Protected: true, DisplayOrder: 0, SourceRefs: []string{"runtime://candidate-contract/v2"}, Content: runtimeV2CandidateContractCatalog()},
			{ID: "C1.goal-dod", Name: "Goal and DoD", Role: "system", Cacheability: "stable", Protected: true, DisplayOrder: 1, DefinitionRevision: definitionContext.Revision, SourceRefs: []string{"runtime://definition/" + definitionContext.Revision}, Content: string(definitionJSON)},
			{ID: "C2.capability-contract", Name: "Capability contract", Role: "system", Cacheability: "stable", Protected: true, DisplayOrder: 2, SourceRefs: []string{"runtime://execution-contract/" + definitionContext.Revision}, Content: string(capabilityJSON)},
			{ID: "C3.runtime-authority", Name: "Current runtime authority", Role: "user", Cacheability: "volatile", Protected: true, DisplayOrder: 3, SourceRefs: []string{"runtime://authority/" + request.Transaction.AuthoritySnapshotID}, Content: string(authorityJSON)},
			{ID: "C4.execution-tail", Name: "Current failure and evidence", Role: "user", Cacheability: "volatile", Protected: true, DisplayOrder: 4, SourceRefs: []string{"runtime://evidence/" + candidateContext.EvidenceRevision}, Content: string(executionTailJSON)},
			{ID: "C5.large-refs", Name: "Large and historical references", Role: "user", Cacheability: "volatile", DisplayOrder: 5, SourceRefs: []string{"runtime://remaining-focuses"}, Content: string(largeRefsJSON)},
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
		providerRequest.PromptContext.Sections[4].Content = string(executionTailJSON) + "\n\nReturn only the exact next bytes of the candidate JSON document after outputContinuation.previousChunkTail. Do not repeat the tail, do not start a new JSON object, and do not use markdown."
	}
	attachProviderCachePlan(&providerRequest)
	if providerRequest.ContextBudget.CachePlan != nil {
		// The persisted Runtime prefix is the authority contract. The provider
		// cache plan independently records the exact rendered prefix so audit can
		// detect a rendering mismatch without replacing the sealed identity.
		providerRequest.ContextBudget.CachePlan.StablePrefixHash = wantPrefix
	}
	populateRuntimeCandidateContextBudget(providerRequest.ContextBudget, providerRequest.PromptContext)
	if err := validateRuntimeCandidateContextIntegrity(definitionContext, candidateContext, request.CandidateAttempt, request.PreviousFailure, providerRequest.ContextBudget, providerRequest.PromptContext); err != nil {
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
		providerRequest.ContextBudget.ContextGateReason = "runtime_v2_provider_ceiling_exceeded"
		providerRequest.ContextBudget.BudgetFailureClass = "provider_ceiling_exceeded"
		// The rendered request itself remains owner-private, but its bounded
		// context budget is the only evidence that explains a preflight refusal.
		// Returning an empty request here made the not-ready path lose section
		// hashes, source refs and the gate disposition before it could be audited.
		return providerRequest, fmt.Errorf("runtime candidate request exceeds provider ceiling: %d > %d", len(payload), ceiling.MaxBytes)
	}
	return providerRequest, nil
}

func validateRuntimeCandidateContextIntegrity(
	definition runtimecontract.RuntimeDefinitionContext,
	context runtimecontract.RuntimeCandidateContext,
	attempt int,
	previousFailure *runtimecontract.RuntimeCandidateFailure,
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

func runtimeV2CandidateContextProjection(context runtimecontract.RuntimeCandidateContext, manifest runtimecontract.RuntimeCandidateContractManifest) runtimeCandidateContextProjection {
	context = context.Normalized()
	manifest = manifest.Normalized()
	activation := context.Activation
	stage := runtimeV2CandidateStage(manifest.State)
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
		Focus:         context.Focus,
		RequiredState: context.RequiredState, RequiredEffectScopes: append([]runtimecontract.RuntimeEffectScope(nil), context.RequiredEffectScopes...),
		RequiredEvidenceKinds: append([]string(nil), context.RequiredEvidenceKinds...), RepairDirective: context.RepairDirective,
		MissingEvidenceKinds: runtimecontract.MissingRequiredEvidenceKinds(facts),
		AllowedEvidenceRefs:  runtimecontract.AcceptedEvidenceRefsForFocus(facts), Evidence: context.Evidence,
		Assessments: context.Assessments, Failure: context.Failure,
		AdmissibleOperationKinds:  runtimeV2CandidateManifestOperationKinds(manifest),
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
		TargetSelectionInstruction: runtimeV2TargetSelectionInstruction(context.Resources.TargetSelection, manifest.State),
		Activation: runtimeCandidateActivationProjection{
			DialogLanguage: activation.DialogLanguage, CurrentStage: stage, Soul: runtimeV2CandidateSoul(activation, stage),
			Skills: activation.Skills, MCPServerIDs: activation.MCPServerIDs,
			MCPCapabilities: runtimecontract.RuntimeMCPCapabilitiesForState(activation.MCPCapabilities, manifest.State),
			Memories:        activation.Memories, EvidencePolicyIDs: runtimeV2CandidateEvidencePolicyIDs(activation.EvidencePolicies),
			Continuation: runtimeV2CandidateContinuationProjection(activation.Continuation),
		},
	}
}

// runtimeV2CandidateEvidencePolicyIDs intentionally projects only stable
// policy identities. Commands, arguments, timeouts and zero-work markers are
// Shell-owned execution facts and must not offer a second free-form command
// surface to the provider.
func runtimeV2CandidateEvidencePolicyIDs(policies []runtimecontract.RuntimeEvidencePolicy) []string {
	normalized := runtimecontract.NormalizeRuntimeEvidencePolicies(policies)
	ret := make([]string, 0, len(normalized))
	for _, policy := range normalized {
		if policy.ID != "" {
			ret = append(ret, policy.ID)
		}
	}
	return ret
}

func runtimeV2CandidateAuthorityContextProjection(context runtimecontract.RuntimeCandidateContext, manifest runtimecontract.RuntimeCandidateContractManifest) runtimeCandidateAuthorityProjection {
	full := runtimeV2CandidateContextProjection(context, manifest)
	activation := full.Activation
	// Continuation is retry/history information and belongs exclusively to C4.
	activation.Continuation = nil
	return runtimeCandidateAuthorityProjection{
		WorkspaceID:                full.WorkspaceID,
		RunID:                      full.RunID,
		DefinitionRevision:         full.DefinitionRevision,
		EvidenceRevision:           full.EvidenceRevision,
		Focus:                      full.Focus,
		RequiredState:              full.RequiredState,
		RequiredEffectScopes:       append([]runtimecontract.RuntimeEffectScope(nil), full.RequiredEffectScopes...),
		RequiredEvidenceKinds:      append([]string(nil), full.RequiredEvidenceKinds...),
		RepairDirective:            full.RepairDirective,
		AllowedEvidenceRefs:        append([]string(nil), full.AllowedEvidenceRefs...),
		AdmissibleOperationKinds:   append([]string(nil), full.AdmissibleOperationKinds...),
		Capabilities:               full.Capabilities,
		Resources:                  full.Resources,
		TargetSelectionInstruction: full.TargetSelectionInstruction,
		Activation:                 activation,
	}
}

func runtimeV2CandidateExecutionTailProjection(
	context runtimecontract.RuntimeCandidateContext,
	attempt int,
	previousFailure *runtimecontract.RuntimeCandidateFailure,
	outputContinuation *runtimecontract.RuntimeCandidateOutputContinuationRequest,
	manifest runtimecontract.RuntimeCandidateContractManifest,
) runtimeCandidateExecutionTailProjection {
	context = context.Normalized()
	return runtimeCandidateExecutionTailProjection{
		Attempt:                   attempt,
		Retry:                     runtimeV2CandidateRetryForManifest(attempt, previousFailure, manifest),
		Failure:                   runtimeV2ProviderSafeFailure(context.Failure),
		AcceptedEvidence:          runtimeV2ProviderSafeEvidence(latestRuntimeEvidence(context.Evidence.Accepted)),
		RejectedEvidence:          runtimeV2ProviderSafeEvidence(latestRuntimeEvidence(context.Evidence.Rejected)),
		Assessments:               runtimeV2ProviderSafeAssessments(latestRuntimeAssessments(context.Assessments)),
		RejectedOperationShapes:   runtimeV2ProviderSafeOperationShapes(context.RejectedOperationShapes),
		NoProgressOperationShapes: runtimeV2ProviderSafeOperationShapes(context.NoProgressOperationShapes),
		OutputContinuation:        outputContinuation,
	}
}

func runtimeV2ProviderSafeFailure(value *runtimecontract.FailureFact) *runtimeCandidateProviderSafeFailureProjection {
	if value == nil {
		return nil
	}
	failure := value.Normalized()
	return &runtimeCandidateProviderSafeFailureProjection{
		Class: failure.Class, Code: failure.Code, Signature: failure.Signature,
		TargetRefs: append([]string(nil), failure.TargetRefs...),
	}
}

func runtimeV2ProviderSafeEvidence(values []runtimecontract.EvidenceFact) []runtimeCandidateProviderSafeEvidenceProjection {
	if len(values) == 0 {
		return nil
	}
	ret := make([]runtimeCandidateProviderSafeEvidenceProjection, 0, len(values))
	for _, value := range values {
		ret = append(ret, runtimeCandidateProviderSafeEvidenceProjection{
			ID: value.ID, Kind: value.Kind, SubjectRef: value.SubjectRef, AuthoritySnapshotID: value.AuthoritySnapshotID,
			DispatchID: value.DispatchID, OperationID: value.OperationID, Revision: value.Revision,
			FailureClass: value.FailureClass, FailureCode: value.FailureCode, FailureSignature: value.FailureSignature,
			ContentHash: value.ContentHash, ExitCode: value.ExitCode, EvidenceRefs: append([]string(nil), value.EvidenceRefs...),
			TargetRefs: append([]string(nil), value.TargetRefs...), CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), value.CoverageWorkItemIDs...),
			Quality: value.Quality, ZeroWork: value.ZeroWork,
		})
	}
	return ret
}

func runtimeV2ProviderSafeAssessments(values []runtimecontract.RuntimeAssessmentFact) []runtimeCandidateProviderSafeAssessmentProjection {
	if len(values) == 0 {
		return nil
	}
	ret := make([]runtimeCandidateProviderSafeAssessmentProjection, 0, len(values))
	for _, value := range values {
		ret = append(ret, runtimeCandidateProviderSafeAssessmentProjection{
			ID: value.ID, FocusID: value.FocusID, SliceID: value.SliceID, RequirementID: value.RequirementID,
			CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), value.CoverageWorkItemIDs...), Disposition: value.Disposition,
			EvidenceRefs: append([]string(nil), value.EvidenceRefs...), ArtifactRefs: append([]string(nil), value.ArtifactRefs...),
		})
	}
	return ret
}

func runtimeV2ProviderSafeOperationShapes(values []runtimecontract.RuntimeRejectedOperationShape) []runtimeCandidateProviderSafeOperationShapeProjection {
	if len(values) == 0 {
		return nil
	}
	ret := make([]runtimeCandidateProviderSafeOperationShapeProjection, 0, len(values))
	for _, value := range runtimecontract.NormalizeRuntimeRejectedOperationShapes(values) {
		ret = append(ret, runtimeCandidateProviderSafeOperationShapeProjection{
			Hash: value.Hash, Kind: value.Operation.Kind, TargetRefs: append([]string(nil), value.Operation.TargetRefs...),
			FailureClass: value.FailureClass, FailureCode: value.FailureCode, FailureSignature: value.FailureSignature,
		})
	}
	return ret
}

func runtimeV2CandidateManifestOperationKinds(manifest runtimecontract.RuntimeCandidateContractManifest) []string {
	manifest = manifest.Normalized()
	ret := make([]string, 0, len(manifest.Operations))
	for _, operation := range manifest.Operations {
		ret = append(ret, operation.Kind)
	}
	return ret
}

func runtimeV2CandidateContinuationProjection(value runtimecontract.RuntimeContinuationContext) *runtimecontract.RuntimeContinuationContext {
	value = value.Normalized()
	if !value.DefinitionChangeSupersedesPrior && len(value.GovernanceHistory) == 0 && value.Replan == nil && value.RejectedResult == nil {
		return nil
	}
	return &value
}

const runtimeCandidateExecutionTailLimit = 4

func latestRuntimeEvidence(values []runtimecontract.EvidenceFact) []runtimecontract.EvidenceFact {
	if len(values) == 0 {
		return nil
	}
	start := len(values) - runtimeCandidateExecutionTailLimit
	if start < 0 {
		start = 0
	}
	return append([]runtimecontract.EvidenceFact(nil), values[start:]...)
}

func latestRuntimeAssessments(values []runtimecontract.RuntimeAssessmentFact) []runtimecontract.RuntimeAssessmentFact {
	if len(values) == 0 {
		return nil
	}
	start := len(values) - runtimeCandidateExecutionTailLimit
	if start < 0 {
		start = 0
	}
	return append([]runtimecontract.RuntimeAssessmentFact(nil), values[start:]...)
}

func runtimeV2CandidateLargeRefsProjection(context runtimecontract.RuntimeCandidateContext) runtimeCandidateLargeRefsProjection {
	context = context.Normalized()
	ret := runtimeCandidateLargeRefsProjection{RemainingFocuses: make([]runtimeCandidateFocusReference, 0, len(context.RemainingFocuses))}
	for _, focus := range context.RemainingFocuses {
		focus = focus.Normalized()
		if focus.ID == "" {
			continue
		}
		ret.RemainingFocuses = append(ret.RemainingFocuses, runtimeCandidateFocusReference{ID: focus.ID, SliceID: focus.SliceID})
	}
	if len(ret.RemainingFocuses) == 0 {
		ret.RemainingFocuses = nil
	}
	return ret
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

// runtimeV2TargetSelectionInstruction converts the persisted File-owner
// selection disposition into provider-facing C3 guidance. A create selection
// carries the activation-sealed prospective path; a complete scope does not.
// Only a durable File-owner operation is admissible for creation, and the File
// owner remains the final workspace-boundary authority.
func runtimeV2TargetSelectionInstruction(selection *runtimecontract.RuntimeTargetSelection, state runtimecontract.ConvergenceState) string {
	if selection == nil || state != runtimecontract.ConvergenceStateNeedStateChange {
		return ""
	}
	value := selection.Normalized()
	if value.OwnerModuleID != runtimecontract.RuntimeModuleFile {
		return ""
	}
	switch value.Disposition {
	case runtimecontract.RuntimeTargetSelectionCreate:
		if value.ProspectiveTargetRef == "" {
			return ""
		}
		return fmt.Sprintf("The File owner observed an empty workspace creation scope with sealed prospective target %q. Propose at least one durable File-owner operation whose sole targetRefs entry is exactly that path. Do not duplicate the target as an operation parameter, substitute another path, leave it blank, or use this scope to authorize Git, Shell, MCP, or another owner. The File owner will validate the path before execution.", value.ProspectiveTargetRef)
	case runtimecontract.RuntimeTargetSelectionScope:
		return "The File owner provided a bounded workspace feasibility scope, not a selected repair target. Follow any concrete target already sealed in the Focus, FailureTarget, current failure, or RepairDirective. Only when none is present, propose at least one durable File-owner operation with a concrete workspace-relative targetRefs entry; do not duplicate it as an operation parameter. That target may be a new file and remains subject to File-owner feasibility. Do not use this scope to authorize Git, Shell, MCP, or another owner."
	default:
		return ""
	}
}

func runtimeV2CandidateStage(state runtimecontract.ConvergenceState) string {
	switch state {
	case runtimecontract.ConvergenceStateNeedStateChange:
		return "execution"
	case runtimecontract.ConvergenceStateNeedValidation, runtimecontract.ConvergenceStateNeedAssessment:
		return "validation"
	case runtimecontract.ConvergenceStateNeedGovernance:
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

func runtimeV2CandidateSchema(state runtimecontract.ConvergenceState) string {
	switch state {
	case runtimecontract.ConvergenceStateNeedStateChange:
		return runtimeV2OperationCandidateSchema("workorch.runtime_candidate.state_change.v2", "Runtime V2 durable state-change candidate", runtimecontract.RuntimeConvergenceEffectInstruction(state))
	case runtimecontract.ConvergenceStateNeedValidation:
		return runtimeV2OperationCandidateSchema("workorch.runtime_candidate.validation.v2", "Runtime V2 non-durable validation candidate", runtimecontract.RuntimeConvergenceEffectInstruction(state))
	case runtimecontract.ConvergenceStateNeedAssessment:
		return `{"type":"object","additionalProperties":false,"required":["focusId","coverageWorkItemIds","disposition","summary"],"properties":{"focusId":{"type":"string"},"sliceId":{"type":"string"},"coverageWorkItemIds":{"type":"array","minItems":1,"items":{"type":"string"}},"disposition":{"type":"string","enum":["satisfied","repairable_gap","governance_required","terminal_rejection"]},"items":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["coverageWorkItemId","satisfied"],"properties":{"coverageWorkItemId":{"type":"string"},"satisfied":{"type":"boolean"},"reason":{"type":"string"},"evidenceRefs":{"type":"array","items":{"type":"string"}}}}},"gap":{"type":"object","additionalProperties":false,"properties":{"coverageWorkItemIds":{"type":"array","items":{"type":"string"}},"missingEvidenceKinds":{"type":"array","items":{"type":"string"}},"targetRefs":{"type":"array","items":{"type":"string"}},"evidenceRefs":{"type":"array","items":{"type":"string"}},"reason":{"type":"string"},"failureClass":{"type":"string"},"failureCode":{"type":"string"},"failureSignature":{"type":"string"}}},"summary":{"type":"string"},"evidenceRefs":{"type":"array","items":{"type":"string"}},"artifactRefs":{"type":"array","items":{"type":"string"}}},"allOf":[{"if":{"properties":{"disposition":{"const":"satisfied"}},"required":["disposition"]},"then":{"required":["evidenceRefs"],"properties":{"evidenceRefs":{"type":"array","minItems":1,"items":{"type":"string"}}}}}]}`
	case runtimecontract.ConvergenceStateNeedGovernance:
		return `{"type":"object","additionalProperties":false,"required":["externalDependency"],"properties":{"externalDependency":{"type":"object","additionalProperties":false,"required":["kind","reason"],"properties":{"kind":{"type":"string"},"subjectRef":{"type":"string"},"reason":{"type":"string"}}}}}`
	default:
		return ""
	}
}

// runtimeV2CandidateSchemaForManifest renders the volatile operation schema
// exclusively from the pure candidate manifest. It is the production path;
// descriptor/parameter/dependency rules must not be reconstructed here.
func runtimeV2CandidateSchemaForManifest(manifest runtimecontract.RuntimeCandidateContractManifest) string {
	manifest = manifest.Normalized()
	if manifest.EffectPolicy.State != manifest.State || manifest.EffectPolicy.Validate() != nil {
		return ""
	}
	base := runtimeV2CandidateSchema(manifest.State)
	if manifest.State != runtimecontract.ConvergenceStateNeedStateChange && manifest.State != runtimecontract.ConvergenceStateNeedValidation {
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
	allowedKinds := make([]string, 0, len(manifest.Operations))
	allowsRequiredOperationRef := false
	for _, operation := range manifest.Operations {
		allowedKinds = append(allowedKinds, operation.Kind)
		if runtimeV2StringIn(operation.AllowedOperationFields, "requiredOperationRef") {
			allowsRequiredOperationRef = true
		}
	}
	conditions := runtimeV2OperationSchemaConditionsForManifest(manifest.Operations)
	if itemProperties, ok := items["properties"].(map[string]any); ok {
		itemProperties["kind"] = map[string]any{"type": "string", "enum": allowedKinds}
		if !manifest.DependenciesAllowed {
			delete(itemProperties, "dependsOn")
		}
		if !allowsRequiredOperationRef {
			delete(itemProperties, "requiredOperationRef")
		}
	}
	if len(conditions) > 0 {
		items["allOf"] = conditions
	}
	if len(manifest.RequiredOperationSequence) > 0 {
		requirements := make([]any, 0, len(manifest.RequiredOperationSequence))
		for _, requirement := range manifest.RequiredOperationSequence {
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

func runtimeV2StringIn(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

// runtimeV2OperationSchemaConditionsForManifest is the production schema
// projection. Its input is already descriptor-derived by
// BuildRuntimeCandidateContractManifest; it must not call the operation
// contract inventory again or maintain a parallel field whitelist.
func runtimeV2OperationSchemaConditionsForManifest(operations []runtimecontract.RuntimeCandidateOperationManifest) []any {
	ret := make([]any, 0, len(operations))
	for _, operation := range operations {
		operation = operation.Normalized()
		if operation.Kind == "" {
			continue
		}
		parameterTypes := make(map[string]runtimecontract.RuntimeOperationParameterType, len(operation.Parameters))
		parameterRequirements := make([]any, 0, len(operation.Parameters)+1)
		for _, parameter := range operation.Parameters {
			parameterTypes[parameter.Name] = parameter.Type
			if parameter.Required {
				parameterRequirements = append(parameterRequirements, runtimeV2ParameterContainsSchema(parameter.Name))
			}
		}
		if len(operation.RequireOneOfParameters) > 0 {
			oneOf := make([]any, 0, len(operation.RequireOneOfParameters))
			for _, name := range operation.RequireOneOfParameters {
				oneOf = append(oneOf, runtimeV2NonEmptyParameterContainsSchema(name, parameterTypes[name]))
			}
			parameterRequirements = append(parameterRequirements, map[string]any{"anyOf": oneOf})
		}
		parameterSchema := map[string]any{
			"type":  "array",
			"items": runtimeV2OperationParameterItemSchemaForManifest(operation),
		}
		if len(parameterRequirements) > 0 {
			parameterSchema["allOf"] = parameterRequirements
		}
		if len(operation.Variants) > 0 {
			variantSchemas := make([]any, 0, len(operation.Variants))
			for _, variant := range operation.Variants {
				variantSchemas = append(variantSchemas, runtimeV2OperationVariantParameterSchema(operation.Kind, variant))
			}
			parameterSchema["oneOf"] = variantSchemas
		}
		thenProperties := map[string]any{"parameters": parameterSchema}
		then := map[string]any{"properties": thenProperties}
		if len(parameterRequirements) > 0 || len(operation.Variants) > 0 {
			then["required"] = []string{"parameters"}
		}
		allowedTargets := operation.Target.Normalized().AllowedTargetRefs()
		if operation.RequireTargetRefs || len(allowedTargets) > 0 {
			if operation.RequireTargetRefs {
				required, _ := then["required"].([]string)
				then["required"] = append(required, "targetRefs")
			}
			targetItems := map[string]any{"type": "string"}
			if len(allowedTargets) > 0 {
				targetItems["enum"] = append([]string(nil), allowedTargets...)
			}
			targetSchema := map[string]any{"type": "array", "items": targetItems}
			switch operation.TargetCardinality {
			case runtimecontract.RuntimeOperationTargetCardinalityOne:
				targetSchema["minItems"] = 1
				targetSchema["maxItems"] = 1
			case runtimecontract.RuntimeOperationTargetCardinalityMany:
				if operation.RequireTargetRefs {
					targetSchema["minItems"] = 1
				}
			}
			thenProperties["targetRefs"] = targetSchema
		}
		ret = append(ret, map[string]any{
			"if":   map[string]any{"properties": map[string]any{"kind": map[string]any{"const": operation.Kind}}, "required": []string{"kind"}},
			"then": then,
		})
	}
	return ret
}

func runtimeV2OperationParameterItemSchemaForManifest(operation runtimecontract.RuntimeCandidateOperationManifest) map[string]any {
	branches := make([]any, 0, len(operation.Parameters)+3)
	for _, parameter := range operation.Parameters {
		if parameter.Name == "recipeId" && len(operation.ValidationRecipes) > 0 {
			branches = append(branches, runtimeV2ValidationRecipeIDParameterSchema(operation.ValidationRecipes))
			continue
		}
		branches = append(branches, runtimeV2NamedOperationParameterSchema(parameter.Name, parameter.Type))
	}
	if operation.AllowsDynamicParameters {
		branches = append(branches, runtimeV2MCPDynamicParameterSchemas(operation.Kind)...)
	}
	return map[string]any{"type": "object", "oneOf": branches}
}

func runtimeV2ValidationRecipeIDParameterSchema(recipes []runtimecontract.RuntimeValidationRecipe) map[string]any {
	recipes = runtimecontract.NormalizeRuntimeValidationRecipes(recipes)
	ids := make([]string, 0, len(recipes))
	for _, recipe := range recipes {
		ids = append(ids, recipe.ID)
	}
	return map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"name", "string"},
		"properties": map[string]any{
			"name":   map[string]any{"const": "recipeId"},
			"string": map[string]any{"type": "string", "enum": ids},
		},
	}
}

func runtimeV2NamedOperationParameterSchema(name string, parameterType runtimecontract.RuntimeOperationParameterType) map[string]any {
	valueField, valueSchema := runtimeV2OperationParameterValueSchema(parameterType)
	return map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"name", valueField},
		"properties": map[string]any{
			"name":     map[string]any{"const": name},
			valueField: valueSchema,
		},
	}
}

func runtimeV2OperationParameterValueSchema(parameterType runtimecontract.RuntimeOperationParameterType) (string, map[string]any) {
	switch parameterType {
	case runtimecontract.RuntimeOperationParameterTypeInteger:
		return "integer", map[string]any{"type": "integer"}
	case runtimecontract.RuntimeOperationParameterTypeBoolean:
		return "boolean", map[string]any{"type": "boolean"}
	case runtimecontract.RuntimeOperationParameterTypeStringList:
		return "stringList", map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"type": "string"}}
	default:
		return "string", map[string]any{"type": "string"}
	}
}

func runtimeV2MCPDynamicParameterSchemas(kind string) []any {
	prefix := ""
	switch runtimecontract.CanonicalRuntimeBlockKind(kind) {
	case runtimecontract.StepKindMCPToolCall:
		prefix = "input"
	case runtimecontract.StepKindMCPPromptGet:
		prefix = "arguments"
	default:
		return nil
	}
	// The shared operation contract permits only scalar dynamic MCP values. The
	// suffix needs at least one non-space rune, matching the contract helper's
	// trimmed non-empty suffix check without inventing tool-specific fields.
	nameSchema := map[string]any{"type": "string", "pattern": "^" + prefix + `\\..*\\S.*$`}
	ret := make([]any, 0, 3)
	for _, value := range []struct {
		field  string
		schema map[string]any
	}{
		{field: "string", schema: map[string]any{"type": "string"}},
		{field: "integer", schema: map[string]any{"type": "integer"}},
		{field: "boolean", schema: map[string]any{"type": "boolean"}},
	} {
		ret = append(ret, map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []string{"name", value.field},
			"properties": map[string]any{
				"name": nameSchema, value.field: value.schema,
			},
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
		"Return exactly one JSON object for the requested Runtime V2 convergence state. The JSON object itself is the candidate; never wrap it in another object.",
		"The current runtime authority's context.outputContract is the sole structured-output contract. Its manifestHash, schema, structuredOutputInstruction, and operationContracts define the complete allowed fields, kinds, variants, parameters, targets, coverage and dependencies for this request.",
		"Do not infer a field, operation kind, parameter, target, coverage item, dependency, or operation variant from this stable instruction. If it is not in context.outputContract, omit it. Do not use markdown fences or explanatory text.",
		"context.requiredState is the sole canonical decision authority. Do not invent or modify convergence state, focus, authority, transaction, or output mode.",
		"When context.repairDirective is present, obey its sealed state, effect, evidence and operation constraints. The exact fields and values remain in context.outputContract and current runtime authority.",
		"context.missingEvidenceKinds is completion-audit information only and must never override context.requiredState or context.repairDirective.",
		"When context.rejectedOperationShapes is present, every listed operation hash and exact structural operation is forbidden until later accepted evidence supersedes it. Choose an admissible kind and change kind, targets, or execution-semantic parameters.",
	}, "\n")
}

func runtimeV2CandidateOutputContractForManifest(manifest runtimecontract.RuntimeCandidateContractManifest) runtimeCandidateOutputContractProjection {
	manifest = manifest.Normalized()
	return runtimeCandidateOutputContractProjection{
		State: manifest.State,
		EffectPolicy: runtimeCandidateEffectPolicyProjection{
			ExecutableOperationsAllowed:   manifest.EffectPolicy.ExecutableOperationsAllowed,
			AllowedOperationEffects:       append([]runtimecontract.RuntimeEffectScope(nil), manifest.EffectPolicy.AllowedOperationEffects...),
			AllowedAcceptedReceiptEffects: append([]runtimecontract.RuntimeEffectScope(nil), manifest.EffectPolicy.AllowedAcceptedReceiptEffects...),
		},
		ManifestHash:                manifest.Hash,
		Schema:                      json.RawMessage(runtimeV2CandidateSchemaForManifest(manifest)),
		StructuredOutputInstruction: runtimeV2CandidateStructuredOutputInstruction(manifest),
		ForbiddenTopLevelFields:     append([]string(nil), manifest.ForbiddenTopLevelFields...),
		OperationContracts:          runtimeV2CandidateOperationContractsForManifest(manifest.Operations),
	}
}

func runtimeV2CandidateOperationContractsForManifest(values []runtimecontract.RuntimeCandidateOperationManifest) []runtimeCandidateOperationContractProjection {
	ret := make([]runtimeCandidateOperationContractProjection, 0, len(values))
	for _, operation := range values {
		operation = operation.Normalized()
		target := operation.Target.Normalized()
		projection := runtimeCandidateOperationContractProjection{
			Kind:                        operation.Kind,
			OwnerModuleID:               operation.OwnerModuleID,
			AllowedOperationFields:      append([]string(nil), operation.AllowedOperationFields...),
			AllowedVariantIDs:           append([]string(nil), operation.AllowedVariantIDs...),
			RequireOneOfParameters:      append([]string(nil), operation.RequireOneOfParameters...),
			TargetCardinality:           operation.TargetCardinality,
			RequireTargetRefs:           operation.RequireTargetRefs,
			ForbiddenParameters:         append([]string(nil), operation.ForbiddenParameters...),
			Rules:                       append([]string(nil), operation.Rules...),
			AllowsDynamicParameters:     operation.AllowsDynamicParameters,
			DynamicParameterPrefixes:    append([]string(nil), operation.DynamicParameterPrefixes...),
			RequiredOperationReferences: append([]string(nil), operation.RequiredOperationReferences...),
			AcceptedEffectScopes:        append([]runtimecontract.RuntimeEffectScope(nil), operation.AcceptedEffectScopes...),
			RequiredEffectScopes:        append([]runtimecontract.RuntimeEffectScope(nil), operation.RequiredEffectScopes...),
			Target: runtimeCandidateOperationTargetProjection{
				Policy:                    target.Policy,
				ExactTargetRefs:           append([]string(nil), target.ExactTargetRefs...),
				OwnerScopeTargetRefs:      append([]string(nil), target.OwnerScopeTargetRefs...),
				OwnerSelectorRefs:         append([]string(nil), target.OwnerSelectorRefs...),
				ProspectiveConstraintRefs: append([]string(nil), target.ProspectiveConstraintRefs...),
				ProspectiveTargetRef:      target.ProspectiveTargetRef,
				AllowedTargetRefs:         target.AllowedTargetRefs(),
			},
			PrerequisiteSnapshotRefs:  append([]string(nil), operation.PrerequisiteSnapshotRefs...),
			AdmissibilitySnapshotRefs: append([]string(nil), operation.AdmissibilitySnapshotRefs...),
			ValidationRecipeIDs:       runtimeV2ValidationRecipeIDs(operation.ValidationRecipes),
			OperationContractRevision: operation.OperationContractRevision,
			Parameters:                make([]runtimeCandidateOperationParameterProjection, 0, len(operation.Parameters)),
		}
		for _, parameter := range operation.Parameters {
			projection.Parameters = append(projection.Parameters, runtimeCandidateOperationParameterProjection{
				Name: parameter.Name, Type: string(parameter.Type), Required: parameter.Required, Description: parameter.Description,
			})
		}
		for _, variant := range operation.Variants {
			projection.Variants = append(projection.Variants, runtimeCandidateOperationVariantProjection{
				ID: variant.ID, Discriminators: append([]runtimecontract.RuntimeOperationParameterPredicate(nil), variant.Discriminators...),
				RequiredParameters: append([]string(nil), variant.RequiredParameters...), RequiredNonEmpty: append([]string(nil), variant.RequiredNonEmpty...),
				ForbiddenParameters: append([]string(nil), variant.ForbiddenParameters...), EffectScope: variant.EffectScope, IdempotencyClass: variant.IdempotencyClass,
			})
		}
		ret = append(ret, projection)
	}
	return ret
}

func runtimeV2ValidationRecipeIDs(recipes []runtimecontract.RuntimeValidationRecipe) []string {
	recipes = runtimecontract.NormalizeRuntimeValidationRecipes(recipes)
	ret := make([]string, 0, len(recipes))
	for _, recipe := range recipes {
		ret = append(ret, recipe.ID)
	}
	return ret
}

// runtimeV2CandidateStructuredOutputInstruction renders the request-specific
// instruction only from the sealed manifest. It deliberately does not inspect
// operation descriptors, candidate context, rejected shapes or error strings:
// those inputs have already been normalized into the manifest or belong to
// C4 retry context. Keeping this projection adjacent to the provider renderer
// makes the LLM-facing wording a consumer rather than a second contract owner.
func runtimeV2CandidateStructuredOutputInstruction(manifest runtimecontract.RuntimeCandidateContractManifest) string {
	manifest = manifest.Normalized()
	lines := []string{
		"Sealed candidate manifest hash: " + manifest.Hash + ".",
		"Return a candidate for convergence state " + string(manifest.State) + " with required top-level fields: " + runtimeV2CandidateContractFieldList(manifest.RequiredTopLevelFields) + ".",
		runtimecontract.RuntimeConvergenceEffectInstruction(manifest.EffectPolicy.State),
		"The only allowed top-level fields are: " + runtimeV2CandidateContractFieldList(manifest.AllowedTopLevelFields) + ".",
	}
	if len(manifest.ForbiddenTopLevelFields) > 0 {
		lines = append(lines, "Forbidden top-level fields: "+runtimeV2CandidateContractFieldList(manifest.ForbiddenTopLevelFields)+".")
	}
	if manifest.RequireCoverageWorkItemIDs {
		lines = append(lines, "Every operation must bind its sealed coverageWorkItemIds.")
	}
	if len(manifest.RequiredOperationSequence) > 0 {
		refs := make([]string, 0, len(manifest.RequiredOperationSequence))
		for _, requirement := range manifest.RequiredOperationSequence {
			refs = append(refs, requirement.RequirementID+"="+requirement.Kind+"/"+requirement.VariantID)
		}
		lines = append(lines, "Required operation sequence references: "+strings.Join(refs, ", ")+".")
	}
	for _, operation := range manifest.Operations {
		operation = operation.Normalized()
		line := "Operation kind " + operation.Kind + " allows fields: " + runtimeV2CandidateContractFieldList(operation.AllowedOperationFields) + "."
		if len(operation.AllowedVariantIDs) > 0 {
			line += " Allowed variants: " + runtimeV2CandidateContractFieldList(operation.AllowedVariantIDs) + "."
		}
		if len(operation.Parameters) > 0 {
			parameters := make([]string, 0, len(operation.Parameters))
			for _, parameter := range operation.Parameters {
				name := parameter.Name + ":" + string(parameter.Type)
				if parameter.Required {
					name += "(required)"
				}
				parameters = append(parameters, name)
			}
			line += " Parameters: " + strings.Join(parameters, ", ") + "."
		}
		if len(operation.RequireOneOfParameters) > 0 {
			line += " Require one non-empty parameter from: " + runtimeV2CandidateContractFieldList(operation.RequireOneOfParameters) + "."
		}
		if operation.TargetCardinality != "" {
			line += " targetRefs cardinality=" + string(operation.TargetCardinality) + "."
		}
		if operation.RequireTargetRefs {
			line += " targetRefs is required and must name the concrete target changed by this operation."
		}
		target := operation.Target.Normalized()
		if target.Policy != "" {
			line += " TargetPolicy=" + string(target.Policy) + "."
			if allowed := target.AllowedTargetRefs(); len(allowed) > 0 {
				line += " Allowed targets: " + runtimeV2CandidateContractFieldList(allowed) + "."
			}
			if target.ProspectiveTargetRef != "" {
				line += " Prospective target: " + target.ProspectiveTargetRef + "."
			}
			if len(target.OwnerSelectorRefs) > 0 {
				line += " Owner selectors: " + runtimeV2CandidateContractFieldList(target.OwnerSelectorRefs) + "."
			}
		}
		if len(operation.RequiredOperationReferences) > 0 {
			line += " requiredOperationRef must be one of: " + runtimeV2CandidateContractFieldList(operation.RequiredOperationReferences) + "."
		}
		if len(operation.ForbiddenParameters) > 0 {
			line += " Forbidden parameters: " + runtimeV2CandidateContractFieldList(operation.ForbiddenParameters) + "."
		}
		if ids := runtimeV2ValidationRecipeIDs(operation.ValidationRecipes); len(ids) > 0 {
			line += " recipeId must select exactly one sealed validation recipe: " + runtimeV2CandidateContractFieldList(ids) + ". Do not include command, args, timeoutMillis, or a test-name filter."
		}
		if operation.AllowsDynamicParameters {
			line += " Dynamic parameters are allowed only by this operation contract."
		}
		lines = append(lines, line)
	}
	if !manifest.DependenciesAllowed {
		lines = append(lines, "dependsOn is not an allowed operation field for this sealed contract.")
	}
	return strings.Join(lines, "\n")
}

func runtimeV2CandidateContractFieldList(values []string) string {
	if len(values) == 0 {
		return "(none)"
	}
	return strings.Join(values, ", ")
}

func runtimeV2CandidateRetry(attempt int, failure *runtimecontract.RuntimeCandidateFailure) *runtimeCandidateRetryProjection {
	return runtimeV2CandidateRetryForManifest(attempt, failure, runtimecontract.RuntimeCandidateContractManifest{})
}

func runtimeV2CandidateRetryForManifest(attempt int, failure *runtimecontract.RuntimeCandidateFailure, manifest runtimecontract.RuntimeCandidateContractManifest) *runtimeCandidateRetryProjection {
	if attempt != 1 || failure == nil {
		return nil
	}
	previous := runtimeCandidateCorrectionFailureProjection{
		Class:      failure.Class,
		Code:       strings.TrimSpace(failure.Code),
		Signature:  strings.TrimSpace(failure.Signature),
		TargetRefs: append([]string(nil), failure.TargetRefs...),
	}
	if diagnostic := failure.Diagnostic.Clone(); diagnostic != nil {
		// P3-B2a has already sealed the complete diagnostic. C4 only projects
		// that immutable, manifest-bound delta; it must not infer a missing hash,
		// rejected shape, or correction objective from the failure text/signature.
		// Attempt-1 eligibility belongs to SM-3 and is fenced before this LLM
		// renderer is called.
		previous.Diagnostic = diagnostic.Clone()
	}
	instruction := "Retry the exact same sealed authority contract. Return only the legal top-level object for the requested action."
	manifest = manifest.Normalized()
	if manifest.Hash != "" {
		instruction += " Re-read context.outputContract.structuredOutputInstruction and schema for sealed manifest hash " + manifest.Hash + "; they are the only source of allowed fields, kinds, variants, parameters, targets, coverage and dependencies."
	}
	if previous.Class == runtimecontract.RuntimeCandidateFailureClassSchema || previous.Class == runtimecontract.RuntimeCandidateFailureClassContract {
		instruction += " The previous candidate shape was rejected. Remove every field or value not permitted by that sealed manifest instead of adding a wrapper, narrative, summary, explanation or convenience parameter."
	}
	if runtimeV2CandidateFailureIsOperationShapeReplay(previous.Code) {
		instruction += " The previous candidate replayed a rejected or no-progress operation shape. Inspect context.rejectedOperationShapes and context.noProgressOperationShapes, then choose a materially different operation shape that changes its declared durable targets. Changing only an operation ID, narrative, dependency list, or cosmetic parameter is not sufficient."
	}
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
