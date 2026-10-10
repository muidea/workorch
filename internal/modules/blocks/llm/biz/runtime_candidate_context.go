package llm

import (
	"sort"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type runtimeCandidateDefinitionProjection struct {
	Revision          string                                            `json:"revision"`
	GoalID            common.GoalID                                     `json:"goalId"`
	GoalDefinition    string                                            `json:"goalDefinition"`
	DoDID             common.DoDID                                      `json:"doDId"`
	DoDDefinition     string                                            `json:"doDDefinition"`
	ChangeImpact      *runtimecontract.RuntimeDefinitionChangeImpact    `json:"changeImpact,omitempty"`
	SourceObligations *runtimecontract.RuntimeSourceObligationInventory `json:"sourceObligations,omitempty"`
}

// runtimeCandidateAuthorityResourceProjection is the provider-facing C3 view
// of current owner facts. The sealed manifest already contains the complete
// operation catalog, target authorization and freshness snapshot identities,
// so C3 must not publish the raw owner catalogs or prerequisite-observation
// ledger a second time. This projection is derived without mutating the
// EventHub command or persisted Runtime aggregate.
type runtimeCandidateAuthorityResourceProjection struct {
	Manifest        *runtimecontract.WorkspaceManifest                `json:"manifest,omitempty"`
	TargetSnapshots []runtimecontract.TargetSnapshot                  `json:"targetSnapshots,omitempty"`
	TargetSelection *runtimecontract.RuntimeTargetSelection           `json:"targetSelection,omitempty"`
	FailureTarget   *runtimeCandidateAuthorityFailureTargetProjection `json:"failureTarget,omitempty"`
}

type runtimeCandidateAuthorityFailureTargetProjection struct {
	Signature  string   `json:"signature,omitempty"`
	TargetRefs []string `json:"targetRefs,omitempty"`
}

type runtimeCandidateActivationProjection struct {
	DialogLanguage     string                                        `json:"dialogLanguage,omitempty"`
	CurrentStage       string                                        `json:"currentStage"`
	Soul               *runtimecontract.RuntimeSoulContext           `json:"soul,omitempty"`
	Skills             []runtimecontract.RuntimeSkillContext         `json:"skills,omitempty"`
	SkillContextBudget *runtimecontract.RuntimeSkillContextBudget    `json:"skillContextBudget,omitempty"`
	MCPServerIDs       []common.MCPServerID                          `json:"mcpServerIds,omitempty"`
	MCPCapabilities    []runtimecontract.RuntimeMCPCapabilityContext `json:"mcpCapabilities,omitempty"`
	Memories           []runtimecontract.RuntimeMemoryContext        `json:"memories,omitempty"`
	EvidencePolicyIDs  []string                                      `json:"evidencePolicyIds,omitempty"`
	Continuation       *runtimecontract.RuntimeContinuationContext   `json:"continuation,omitempty"`
}

// runtimeCandidateAuthorityProjection is C3: the current sealed authority
// needed to choose one legal candidate. Historical evidence and retry
// correction intentionally live in C4 so they cannot pollute the stable
// prefix or obscure the current focus.
type runtimeCandidateAuthorityProjection struct {
	WorkspaceID                 common.WorkspaceID                                `json:"workspaceId"`
	RunID                       common.RunID                                      `json:"runId"`
	DefinitionRevision          string                                            `json:"definitionRevision"`
	EvidenceRevision            string                                            `json:"evidenceRevision"`
	Focus                       runtimecontract.CoverageFocus                     `json:"focus"`
	RequiredState               runtimecontract.ConvergenceState                  `json:"requiredState"`
	RequiredEffectScopes        []runtimecontract.RuntimeEffectScope              `json:"requiredEffectScopes,omitempty"`
	RequiredEvidenceKinds       []string                                          `json:"requiredEvidenceKinds,omitempty"`
	RepairDirective             *runtimeCandidateRepairAuthorityProjection        `json:"repairDirective,omitempty"`
	AllowedEvidenceRefs         []string                                          `json:"allowedAcceptedEvidenceRefs,omitempty"`
	ValidationProofPreparations []runtimeSemanticNativeCheckAttestationProjection `json:"validationProofPreparations,omitempty"`
	Resources                   runtimeCandidateAuthorityResourceProjection       `json:"resources"`
	TargetSelectionInstruction  string                                            `json:"targetSelectionInstruction,omitempty"`
	Activation                  runtimeCandidateActivationProjection              `json:"activation"`
}

// runtimeCandidateExecutionTailProjection is C4: bounded current failure,
// evidence and retry context. It never changes C0-C2 and never carries raw
// provider payloads.
type runtimeCandidateExecutionTailProjection struct {
	Attempt                   int                                                    `json:"candidateAttempt"`
	Retry                     *runtimeCandidateRetryProjection                       `json:"retry,omitempty"`
	Failure                   *runtimeCandidateProviderSafeFailureProjection         `json:"failure,omitempty"`
	CurrentValidationFailure  *runtimeCandidateValidationFailureProjection           `json:"currentValidationFailure,omitempty"`
	AcceptedEvidence          []runtimeCandidateProviderSafeEvidenceProjection       `json:"latestAcceptedEvidence,omitempty"`
	RejectedEvidence          []runtimeCandidateProviderSafeEvidenceProjection       `json:"latestRejectedEvidence,omitempty"`
	Assessments               []runtimeCandidateProviderSafeAssessmentProjection     `json:"recentAssessments,omitempty"`
	RejectedOperationShapes   []runtimeCandidateProviderSafeOperationShapeProjection `json:"rejectedOperationShapes,omitempty"`
	NoProgressOperationShapes []runtimeCandidateProviderSafeOperationShapeProjection `json:"noProgressOperationShapes,omitempty"`
}

// Do not marshal the persisted directive: CurrentFailure and rejected operation
// bodies contain owner-private diagnostics and raw candidate parameters. The
// manifest remains the sole operation contract; failure facts belong in C4.
type runtimeCandidateRepairAuthorityProjection struct {
	ID                    string                                       `json:"id"`
	FocusID               string                                       `json:"focusId"`
	ClosureID             string                                       `json:"closureId,omitempty"`
	Purpose               runtimecontract.RuntimeDispatchRepairPurpose `json:"purpose,omitempty"`
	RequiredState         runtimecontract.ConvergenceState             `json:"requiredState"`
	RequiredEffectScopes  []runtimecontract.RuntimeEffectScope         `json:"requiredEffectScopes,omitempty"`
	RequiredEvidenceKinds []string                                     `json:"requiredEvidenceKinds,omitempty"`
	AllowedOperationKinds []string                                     `json:"allowedOperationKinds,omitempty"`
	CoverageWorkItemIDs   []common.CoverageWorkItemID                  `json:"coverageWorkItemIds,omitempty"`
	TargetRefs            []string                                     `json:"targetRefs,omitempty"`
}

type runtimeCandidateValidationFailureProjection struct {
	EvidenceID  common.EvidenceRecordID                             `json:"evidenceId"`
	Fingerprint string                                              `json:"fingerprint,omitempty"`
	Occurrences int                                                 `json:"occurrences"`
	Validation  *runtimecontract.RuntimeValidationExecutionFeedback `json:"validation,omitempty"`
}

// The following C4 values are deliberately narrower than the persisted
// Runtime facts. C4 is a provider-facing diagnostic tail, not an aggregate
// mirror: raw messages/summaries, governance/recovery continuation and any
// Runtime action or retry policy stay with their owning state machine.
type runtimeCandidateProviderSafeFailureProjection struct {
	Class                runtimecontract.FailureClass                    `json:"class,omitempty"`
	Code                 string                                          `json:"code,omitempty"`
	Signature            string                                          `json:"signature,omitempty"`
	TargetRefs           []string                                        `json:"targetRefs,omitempty"`
	ValidationDiagnostic *runtimeCandidateValidationDiagnosticProjection `json:"validationDiagnostic,omitempty"`
}

type runtimeCandidateValidationDiagnosticProjection struct {
	InputLocations     []runtimecontract.RuntimeValidationInputLocation `json:"inputLocations,omitempty"`
	InputAddition      runtimecontract.RuntimeValidationInputAddition   `json:"inputAddition,omitzero"`
	Class              string                                           `json:"class,omitempty"`
	OwnerCode          string                                           `json:"ownerCode,omitempty"`
	EvidenceTargetType string                                           `json:"evidenceTargetType,omitempty"`
	Operation          string                                           `json:"operation,omitempty"`
	CommandKind        string                                           `json:"commandKind,omitempty"`
	ExitCode           int                                              `json:"exitCode,omitempty"`
	TimedOut           bool                                             `json:"timedOut,omitempty"`
	UnavailableCommand string                                           `json:"unavailableCommand,omitempty"`
	TargetRefs         []string                                         `json:"targetRefs,omitempty"`
}

type runtimeCandidateProviderSafeEvidenceProjection struct {
	ID                  common.EvidenceRecordID                             `json:"id,omitempty"`
	Kind                string                                              `json:"kind,omitempty"`
	SubjectRef          string                                              `json:"subjectRef,omitempty"`
	AuthoritySnapshotID string                                              `json:"authoritySnapshotId,omitempty"`
	DispatchID          string                                              `json:"dispatchId,omitempty"`
	OperationID         string                                              `json:"operationId,omitempty"`
	Revision            string                                              `json:"revision,omitempty"`
	FailureClass        runtimecontract.FailureClass                        `json:"failureClass,omitempty"`
	FailureCode         string                                              `json:"failureCode,omitempty"`
	FailureSignature    string                                              `json:"failureSignature,omitempty"`
	ContentHash         string                                              `json:"contentHash,omitempty"`
	ExitCode            *int64                                              `json:"exitCode,omitempty"`
	EvidenceRefs        []string                                            `json:"evidenceRefs,omitempty"`
	TargetRefs          []string                                            `json:"targetRefs,omitempty"`
	CoverageWorkItemIDs []common.CoverageWorkItemID                         `json:"coverageWorkItemIds,omitempty"`
	Quality             common.EvidenceQuality                              `json:"quality,omitempty"`
	ZeroWork            bool                                                `json:"zeroWork,omitempty"`
	Validation          *runtimecontract.RuntimeValidationExecutionFeedback `json:"validation,omitempty"`
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

func runtimeCandidateActivationForStage(activation runtimecontract.RuntimeActivationContext, stage string, state runtimecontract.ConvergenceState) runtimeCandidateActivationProjection {
	soul := runtimeCandidateSoul(activation, stage)
	skills := runtimeCandidateSkillsForSoul(activation.Skills, soul)
	capabilities := runtimeCandidateMCPCapabilitiesForSoul(
		runtimecontract.RuntimeMCPCapabilitiesForState(activation.MCPCapabilities, activation.OperationPolicies, state),
		soul,
	)
	return runtimeCandidateActivationProjection{
		DialogLanguage: activation.DialogLanguage, CurrentStage: stage, Soul: soul,
		Skills: skills, SkillContextBudget: runtimeCandidateSkillBudget(skills, activation.SkillContextBudget),
		MCPServerIDs: runtimeCandidateMCPServerIDsForSoul(activation.MCPServerIDs, soul), MCPCapabilities: capabilities,
		Memories: activation.Memories, EvidencePolicyIDs: runtimeCandidateEvidencePolicyIDs(activation.EvidencePolicies),
		Continuation: runtimeCandidateContinuationProjection(activation.Continuation),
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
	context = context.Normalized()
	manifest = manifest.Normalized()
	activation := runtimeCandidateAuthorityActivationForManifest(context.Activation, context.Resources, manifest)
	resources := runtimeCandidateAuthorityResources(context.Resources, manifest)
	facts := runtimecontract.RuntimeFacts{
		WorkspaceID: context.WorkspaceID, RunID: context.RunID, DefinitionRevision: context.DefinitionRevision,
		EvidenceRevision: context.EvidenceRevision, Focus: context.Focus, RemainingFocuses: context.RemainingFocuses,
		Evidence: context.Evidence, Assessments: context.Assessments, Failure: context.Failure,
		Capabilities: context.Capabilities, Resources: context.Resources, Activation: context.Activation,
		RepairDirective: context.RepairDirective,
	}.Normalized()
	// Continuation is retry/history information and belongs exclusively to C4.
	activation.Continuation = nil
	return runtimeCandidateAuthorityProjection{
		WorkspaceID:                 context.WorkspaceID,
		RunID:                       context.RunID,
		DefinitionRevision:          context.DefinitionRevision,
		EvidenceRevision:            context.EvidenceRevision,
		Focus:                       context.Focus,
		RequiredState:               context.RequiredState,
		RequiredEffectScopes:        append([]runtimecontract.RuntimeEffectScope(nil), context.RequiredEffectScopes...),
		RequiredEvidenceKinds:       append([]string(nil), context.RequiredEvidenceKinds...),
		RepairDirective:             runtimeCandidateRepairAuthority(context.RepairDirective),
		AllowedEvidenceRefs:         runtimecontract.AcceptedEvidenceRefsForFocus(facts),
		ValidationProofPreparations: runtimeCandidateValidationProofPreparations(context.Focus, context.Resources.OperationAdmissibilities),
		Resources:                   resources,
		TargetSelectionInstruction:  runtimeTargetSelectionInstruction(resources.TargetSelection, manifest.State),
		Activation:                  activation,
	}
}

func runtimeCandidateValidationProofPreparations(focus runtimecontract.CoverageFocus, catalogs []runtimecontract.RuntimeOperationAdmissibility) []runtimeSemanticNativeCheckAttestationProjection {
	focus = focus.Normalized()
	if len(focus.CoverageWorkItemIDs) == 0 || len(focus.ProspectiveTargetRefs) == 0 {
		return nil
	}
	coverage := make(map[common.CoverageWorkItemID]struct{}, len(focus.CoverageWorkItemIDs))
	for _, id := range focus.CoverageWorkItemIDs {
		coverage[id] = struct{}{}
	}
	ret := make([]runtimeSemanticNativeCheckAttestationProjection, 0)
	for _, catalog := range runtimecontract.NormalizeRuntimeOperationAdmissibilities(catalogs) {
		if catalog.OwnerModuleID != runtimecontract.RuntimeModuleShell {
			continue
		}
		for _, surface := range catalog.ValidationSurfaces {
			for _, preparation := range surface.ProofRequirementPreparations {
				preparation = preparation.Clone()
				if _, found := coverage[preparation.CoverageWorkItemID]; !found ||
					!runtimecontract.RuntimeProspectiveTargetsSatisfyConstraint(focus.ProspectiveTargetRefs, preparation.ProspectiveTargetSatisfactionConstraint) {
					continue
				}
				for _, attestation := range preparation.CheckAttestations {
					ret = append(ret, runtimeSemanticNativeCheckAttestationProjection{
						ValidationRecipeID: preparation.ValidationRecipeID, RequiredCheckRef: attestation.RequiredCheckRef,
						ObservationProtocolRef: attestation.ObservationProtocolRef, NativeCheckRef: attestation.NativeCheckRef, SupplementalNativeCheckRef: attestation.SupplementalNativeCheckRef,
						TargetRef: preparation.TargetRef, ContentContractRef: preparation.ContentContractRef, ContentRequirements: preparation.ContentRequirements,
					})
				}
			}
		}
	}
	sort.SliceStable(ret, func(i, j int) bool {
		if ret[i].ValidationRecipeID != ret[j].ValidationRecipeID {
			return ret[i].ValidationRecipeID < ret[j].ValidationRecipeID
		}
		return ret[i].RequiredCheckRef < ret[j].RequiredCheckRef
	})
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func runtimeCandidateAuthorityResources(resources runtimecontract.RuntimeResourceFacts, manifest runtimecontract.RuntimeCandidateContractManifest) runtimeCandidateAuthorityResourceProjection {
	targetRefs := runtimeCandidateAuthorityTargetRefSet(resources, manifest)
	snapshotRefs := runtimeCandidateManifestPrerequisiteSnapshotRefSet(manifest)
	ret := runtimeCandidateAuthorityResourceProjection{
		Manifest:        runtimeCandidateAuthorityWorkspaceManifest(resources.Manifest, targetRefs),
		TargetSnapshots: runtimeCandidateAuthorityTargetSnapshots(resources.TargetSnapshots, targetRefs),
	}
	if resources.TargetSelection != nil {
		selection := resources.TargetSelection.Normalized()
		if runtimeCandidateTargetSelectionMatchesManifest(selection, manifest, snapshotRefs) {
			ret.TargetSelection = &selection
		}
	}
	if resources.FailureTarget != nil {
		refs := runtimeCandidateManifestScopedTargetRefs(resources.FailureTarget.TargetRefs, targetRefs)
		if len(refs) != 0 {
			ret.FailureTarget = &runtimeCandidateAuthorityFailureTargetProjection{
				Signature:  resources.FailureTarget.Signature,
				TargetRefs: refs,
			}
		}
	}
	return ret
}

func runtimeCandidateTargetSelectionMatchesManifest(
	selection runtimecontract.RuntimeTargetSelection,
	manifest runtimecontract.RuntimeCandidateContractManifest,
	snapshotRefs map[string]struct{},
) bool {
	if _, ok := snapshotRefs[selection.SnapshotID]; ok {
		return true
	}
	for _, operation := range manifest.Operations {
		for _, selectorRef := range operation.Target.OwnerSelectorRefs {
			if selectorRef == selection.SelectorRef {
				return true
			}
		}
	}
	return false
}

func runtimeCandidateAuthorityTargetRefSet(resources runtimecontract.RuntimeResourceFacts, manifest runtimecontract.RuntimeCandidateContractManifest) map[string]struct{} {
	manifest = manifest.Normalized()
	ret := make(map[string]struct{}, len(manifest.TargetRefs))
	for _, targetRef := range manifest.TargetRefs {
		ret[targetRef] = struct{}{}
	}
	for _, operation := range manifest.Operations {
		for _, targetRef := range operation.Target.AllowedTargetRefs() {
			ret[targetRef] = struct{}{}
		}
	}
	for _, requirement := range manifest.RequiredOperationSequence {
		if requirement.TargetRef != "" {
			ret[requirement.TargetRef] = struct{}{}
		}
	}
	fileWorkspaceFallback := false
	fileSelectionMatched := false
	for _, operation := range manifest.Operations {
		target := operation.Target.Normalized()
		if target.Policy != runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope || len(target.OwnerScopeTargetRefs) != 0 || len(target.OwnerSelectorRefs) == 0 {
			continue
		}
		if operation.OwnerModuleID == runtimecontract.RuntimeModuleFile {
			fileWorkspaceFallback = true
		}
		if resources.TargetSelection == nil {
			continue
		}
		selection := resources.TargetSelection.Normalized()
		for _, selectorRef := range target.OwnerSelectorRefs {
			if selectorRef != selection.SelectorRef {
				continue
			}
			// A cross-owner operation may consume a File-owned target selector,
			// but an unrelated MCP/Shell selector must never inherit File
			// inventory merely because both use existing_owner_scope.
			if operation.OwnerModuleID != runtimecontract.RuntimeModuleFile && selection.OwnerModuleID != runtimecontract.RuntimeModuleFile {
				continue
			}
			if operation.OwnerModuleID == runtimecontract.RuntimeModuleFile {
				fileSelectionMatched = true
			}
			for _, targetRef := range append(append([]string(nil), selection.TargetRefs...), selection.CandidateTargetRefs...) {
				ret[targetRef] = struct{}{}
			}
		}
	}
	// A selector-only existing-owner scope deliberately delegates final
	// membership to owner feasibility. It still needs a bounded factual
	// workspace inventory from which the provider can name a concrete target.
	// Prefer a matching TargetSelection; when none exists, the File-owned
	// WorkspaceManifest is the bounded candidate inventory, not an authorization
	// expansion.
	if fileWorkspaceFallback && !fileSelectionMatched && resources.Manifest != nil {
		for _, targetRef := range resources.Manifest.TargetRefs {
			ret[targetRef] = struct{}{}
		}
	}
	return ret
}

func runtimeCandidateManifestPrerequisiteSnapshotRefSet(manifest runtimecontract.RuntimeCandidateContractManifest) map[string]struct{} {
	manifest = manifest.Normalized()
	ret := make(map[string]struct{})
	for _, operation := range manifest.Operations {
		for _, snapshotRef := range operation.PrerequisiteSnapshotRefs {
			ret[snapshotRef] = struct{}{}
		}
	}
	return ret
}

func runtimeCandidateAuthorityWorkspaceManifest(manifest *runtimecontract.WorkspaceManifest, targetRefs map[string]struct{}) *runtimecontract.WorkspaceManifest {
	if manifest == nil || len(targetRefs) == 0 {
		return nil
	}
	refs := runtimeCandidateManifestScopedTargetRefs(manifest.TargetRefs, targetRefs)
	if len(refs) == 0 {
		return nil
	}
	return &runtimecontract.WorkspaceManifest{Revision: manifest.Revision, TargetRefs: refs}
}

func runtimeCandidateAuthorityTargetSnapshots(values []runtimecontract.TargetSnapshot, targetRefs map[string]struct{}) []runtimecontract.TargetSnapshot {
	if len(values) == 0 || len(targetRefs) == 0 {
		return nil
	}
	ret := make([]runtimecontract.TargetSnapshot, 0, len(values))
	for _, value := range values {
		if _, ok := targetRefs[value.TargetRef]; ok {
			ret = append(ret, value)
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func runtimeCandidateManifestScopedTargetRefs(values []string, targetRefs map[string]struct{}) []string {
	if len(values) == 0 || len(targetRefs) == 0 {
		return nil
	}
	ret := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, ok := targetRefs[value]; !ok {
			continue
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		ret = append(ret, value)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func runtimeCandidateAuthorityActivationForManifest(
	activation runtimecontract.RuntimeActivationContext,
	resources runtimecontract.RuntimeResourceFacts,
	manifest runtimecontract.RuntimeCandidateContractManifest,
) runtimeCandidateActivationProjection {
	manifest = manifest.Normalized()
	projected := runtimeCandidateActivationForStage(activation, runtimeCandidateStage(manifest.State), manifest.State)
	operationKinds := make(map[string]struct{}, len(manifest.Operations))
	mcpSnapshotRefs := make(map[string]struct{})
	for _, operation := range manifest.Operations {
		operationKinds[operation.Kind] = struct{}{}
		if operation.OwnerModuleID == runtimecontract.RuntimeModuleMCP {
			for _, snapshotRef := range operation.AdmissibilitySnapshotRefs {
				mcpSnapshotRefs[snapshotRef] = struct{}{}
			}
		}
	}
	manifestMCPCapabilityIDs := make(map[common.MCPCapabilityID]struct{})
	for _, catalog := range runtimecontract.NormalizeRuntimeOperationAdmissibilities(resources.OperationAdmissibilities) {
		if _, ok := mcpSnapshotRefs[catalog.SnapshotID]; !ok || catalog.OwnerModuleID != runtimecontract.RuntimeModuleMCP {
			continue
		}
		for _, capability := range catalog.MCPCapabilities {
			if _, ok := operationKinds[capability.OperationKind]; ok {
				manifestMCPCapabilityIDs[capability.ID] = struct{}{}
			}
		}
	}

	capabilities := make([]runtimecontract.RuntimeMCPCapabilityContext, 0, len(projected.MCPCapabilities))
	serverIDs := make(map[common.MCPServerID]struct{})
	for _, capability := range projected.MCPCapabilities {
		if _, ok := operationKinds[capability.OperationKind]; !ok {
			continue
		}
		if _, ok := manifestMCPCapabilityIDs[capability.ID]; !ok {
			continue
		}
		capabilities = append(capabilities, capability)
		serverIDs[capability.ServerID] = struct{}{}
	}
	projected.MCPCapabilities = capabilities
	projected.MCPServerIDs = nil
	for _, serverID := range runtimeCandidateMCPServerIDsForSoul(activation.MCPServerIDs, projected.Soul) {
		if _, ok := serverIDs[serverID]; ok {
			projected.MCPServerIDs = append(projected.MCPServerIDs, serverID)
		}
	}

	projected.EvidencePolicyIDs = nil
	for _, policy := range runtimecontract.NormalizeRuntimeEvidencePolicies(activation.EvidencePolicies) {
		if _, ok := operationKinds[policy.AppliesTo]; ok && policy.ID != "" {
			projected.EvidencePolicyIDs = append(projected.EvidencePolicyIDs, policy.ID)
		}
	}
	return projected
}

func buildRuntimeCandidateExecutionTailProjection(
	context runtimecontract.RuntimeCandidateContext,
	attempt int,
	previousFailure *runtimecontract.RuntimeCandidateCorrectionObservation,
	manifest runtimecontract.RuntimeCandidateContractManifest,
	correction *llmmodels.RuntimeCandidateCorrectionInput,
) runtimeCandidateExecutionTailProjection {
	context = context.Normalized()
	failure := context.Failure
	if context.RepairDirective != nil && context.RepairDirective.CurrentFailure != nil {
		failure = context.RepairDirective.CurrentFailure
	}
	return runtimeCandidateExecutionTailProjection{
		Attempt:                   attempt,
		Retry:                     runtimeCandidateRetryForManifest(attempt, previousFailure, manifest, correction),
		Failure:                   runtimeProviderSafeFailure(failure),
		CurrentValidationFailure:  runtimeCurrentValidationFailure(context),
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
	ret := &runtimeCandidateProviderSafeFailureProjection{
		Class: failure.Class, Code: failure.Code, Signature: failure.Signature,
		TargetRefs: append([]string(nil), failure.TargetRefs...),
	}
	if failure.ValidationDiagnostic != nil {
		d := failure.ValidationDiagnostic.Normalized()
		ret.ValidationDiagnostic = &runtimeCandidateValidationDiagnosticProjection{Class: d.Class, OwnerCode: d.OwnerCode, InputAddition: d.InputAddition,
			InputLocations:     append([]runtimecontract.RuntimeValidationInputLocation(nil), d.InputLocations...),
			EvidenceTargetType: d.RecommendedEvidenceTargetType, Operation: d.Operation, CommandKind: d.CommandKind,
			ExitCode: d.ExitCode, TimedOut: d.TimedOut, UnavailableCommand: d.UnavailableCommand, TargetRefs: append([]string(nil), d.TargetRefs...)}
	}
	return ret
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
			Validation: runtimecontract.ProjectRuntimeValidationExecutionFeedback(value.Validation),
		})
	}
	return ret
}

func runtimeCandidateRepairAuthority(value *runtimecontract.RuntimeRepairDirective) *runtimeCandidateRepairAuthorityProjection {
	if value == nil {
		return nil
	}
	d := value.Normalized()
	ret := &runtimeCandidateRepairAuthorityProjection{ID: d.ID, FocusID: d.FocusID, Purpose: d.Purpose,
		RequiredState: d.RequiredState, RequiredEffectScopes: d.RequiredEffectScopes, RequiredEvidenceKinds: d.RequiredEvidenceKinds,
		AllowedOperationKinds: d.AllowedOperationKinds, CoverageWorkItemIDs: d.CoverageWorkItemIDs, TargetRefs: d.TargetRefs}
	if d.Closure != nil {
		ret.ClosureID = d.Closure.ID
	}
	return ret
}

func runtimeCurrentValidationFailure(context runtimecontract.RuntimeCandidateContext) *runtimeCandidateValidationFailureProjection {
	failure := context.Failure
	if context.RepairDirective != nil && context.RepairDirective.CurrentFailure != nil {
		failure = context.RepairDirective.CurrentFailure
	}
	if failure == nil || failure.Signature == "" {
		return nil
	}
	// Do not lose the current causal receipt merely because unrelated recent
	// evidence filled the history tail. Nor substitute another check's failure.
	for i := len(context.Evidence.Rejected) - 1; i >= 0; i-- {
		fact := context.Evidence.Rejected[i]
		if fact.Kind != runtimecontract.EvidenceKindValidation || fact.FailureSignature != failure.Signature {
			continue
		}
		return &runtimeCandidateValidationFailureProjection{EvidenceID: fact.ID,
			Fingerprint: runtimecontract.RuntimeValidationFailureFingerprint(fact, context.Focus.CoverageEvidenceBindings...),
			Occurrences: runtimecontract.RuntimeValidationFailureOccurrences(context.Evidence, fact, context.Focus.CoverageEvidenceBindings...),
			Validation:  runtimecontract.ProjectRuntimeValidationExecutionFeedback(fact.Validation)}
	}
	return nil
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
