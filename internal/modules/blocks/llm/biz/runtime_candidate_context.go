package llm

import (
	"github.com/muidea/workorch/internal/pkg/common"
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
	Attempt                   int                                                    `json:"candidateAttempt"`
	Retry                     *runtimeCandidateRetryProjection                       `json:"retry,omitempty"`
	Failure                   *runtimeCandidateProviderSafeFailureProjection         `json:"failure,omitempty"`
	AcceptedEvidence          []runtimeCandidateProviderSafeEvidenceProjection       `json:"latestAcceptedEvidence,omitempty"`
	RejectedEvidence          []runtimeCandidateProviderSafeEvidenceProjection       `json:"latestRejectedEvidence,omitempty"`
	Assessments               []runtimeCandidateProviderSafeAssessmentProjection     `json:"recentAssessments,omitempty"`
	RejectedOperationShapes   []runtimeCandidateProviderSafeOperationShapeProjection `json:"rejectedOperationShapes,omitempty"`
	NoProgressOperationShapes []runtimeCandidateProviderSafeOperationShapeProjection `json:"noProgressOperationShapes,omitempty"`
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

func buildRuntimeCandidateContextProjection(context runtimecontract.RuntimeCandidateContext, manifest runtimecontract.RuntimeCandidateContractManifest) runtimeCandidateContextProjection {
	context = context.Normalized()
	manifest = manifest.Normalized()
	activation := context.Activation
	stage := runtimeCandidateStage(manifest.State)
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
		AdmissibleOperationKinds:  runtimeCandidateManifestOperationKinds(manifest),
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
		TargetSelectionInstruction: runtimeTargetSelectionInstruction(context.Resources.TargetSelection, manifest.State),
		Activation: runtimeCandidateActivationProjection{
			DialogLanguage: activation.DialogLanguage, CurrentStage: stage, Soul: runtimeCandidateSoul(activation, stage),
			Skills: activation.Skills, MCPServerIDs: activation.MCPServerIDs,
			MCPCapabilities: runtimecontract.RuntimeMCPCapabilitiesForState(activation.MCPCapabilities, manifest.State),
			Memories:        activation.Memories, EvidencePolicyIDs: runtimeCandidateEvidencePolicyIDs(activation.EvidencePolicies),
			Continuation: runtimeCandidateContinuationProjection(activation.Continuation),
		},
	}
}

// runtimeCandidateEvidencePolicyIDs intentionally projects only stable
// policy identities. Commands, arguments, timeouts and zero-work markers are
// Shell-owned execution facts and must not offer a second free-form command
// surface to the provider.
func runtimeCandidateEvidencePolicyIDs(policies []runtimecontract.RuntimeEvidencePolicy) []string {
	normalized := runtimecontract.NormalizeRuntimeEvidencePolicies(policies)
	ret := make([]string, 0, len(normalized))
	for _, policy := range normalized {
		if policy.ID != "" {
			ret = append(ret, policy.ID)
		}
	}
	return ret
}

func runtimeCandidateAuthorityContextProjection(context runtimecontract.RuntimeCandidateContext, manifest runtimecontract.RuntimeCandidateContractManifest) runtimeCandidateAuthorityProjection {
	full := buildRuntimeCandidateContextProjection(context, manifest)
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

func buildRuntimeCandidateExecutionTailProjection(
	context runtimecontract.RuntimeCandidateContext,
	attempt int,
	previousFailure *runtimecontract.RuntimeCandidateFailure,
	manifest runtimecontract.RuntimeCandidateContractManifest,
) runtimeCandidateExecutionTailProjection {
	context = context.Normalized()
	return runtimeCandidateExecutionTailProjection{
		Attempt:                   attempt,
		Retry:                     runtimeCandidateRetryForManifest(attempt, previousFailure, manifest),
		Failure:                   runtimeProviderSafeFailure(context.Failure),
		AcceptedEvidence:          runtimeProviderSafeEvidence(latestRuntimeEvidence(context.Evidence.Accepted)),
		RejectedEvidence:          runtimeProviderSafeEvidence(latestRuntimeEvidence(context.Evidence.Rejected)),
		Assessments:               runtimeProviderSafeAssessments(latestRuntimeAssessments(context.Assessments)),
		RejectedOperationShapes:   runtimeProviderSafeOperationShapes(context.RejectedOperationShapes),
		NoProgressOperationShapes: runtimeProviderSafeOperationShapes(context.NoProgressOperationShapes),
	}
}

func runtimeProviderSafeFailure(value *runtimecontract.FailureFact) *runtimeCandidateProviderSafeFailureProjection {
	if value == nil {
		return nil
	}
	failure := value.Normalized()
	return &runtimeCandidateProviderSafeFailureProjection{
		Class: failure.Class, Code: failure.Code, Signature: failure.Signature,
		TargetRefs: append([]string(nil), failure.TargetRefs...),
	}
}

func runtimeProviderSafeEvidence(values []runtimecontract.EvidenceFact) []runtimeCandidateProviderSafeEvidenceProjection {
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

func runtimeProviderSafeAssessments(values []runtimecontract.RuntimeAssessmentFact) []runtimeCandidateProviderSafeAssessmentProjection {
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

func runtimeProviderSafeOperationShapes(values []runtimecontract.RuntimeRejectedOperationShape) []runtimeCandidateProviderSafeOperationShapeProjection {
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

func runtimeCandidateManifestOperationKinds(manifest runtimecontract.RuntimeCandidateContractManifest) []string {
	manifest = manifest.Normalized()
	ret := make([]string, 0, len(manifest.Operations))
	for _, operation := range manifest.Operations {
		ret = append(ret, operation.Kind)
	}
	return ret
}

func runtimeCandidateContinuationProjection(value runtimecontract.RuntimeContinuationContext) *runtimecontract.RuntimeContinuationContext {
	value = value.Normalized()
	if !value.DefinitionChangeSupersedesPrior && len(value.GovernanceHistory) == 0 {
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

func buildRuntimeCandidateLargeRefsProjection(context runtimecontract.RuntimeCandidateContext) runtimeCandidateLargeRefsProjection {
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

func runtimeCandidateStage(state runtimecontract.ConvergenceState) string {
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

func runtimeCandidateSoul(activation runtimecontract.RuntimeActivationContext, stage string) *runtimecontract.RuntimeSoulContext {
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
