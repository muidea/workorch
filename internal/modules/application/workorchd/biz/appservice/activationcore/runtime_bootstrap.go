package activationcore

import (
	"fmt"
	"sort"
	"strings"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func RuntimeBootstrap(goal *workspacemodel.Goal, dod *workspacemodel.DoD, definitionRevision string, changeImpact *runtimecontract.RuntimeDefinitionChangeImpact, activation runtimecontract.RuntimeActivationContext) (*runtimecontract.RuntimeBootstrap, error) {
	definitionRevision = strings.TrimSpace(definitionRevision)
	if goal == nil || dod == nil || goal.ID == "" || dod.ID == "" || goal.WorkspaceID == "" || goal.WorkspaceID != dod.WorkspaceID ||
		strings.TrimSpace(goal.Text) == "" || len(dod.Items) == 0 || definitionRevision == "" {
		return nil, common.ErrInvalidArgument
	}
	if activation.ExecutionContract.AcceptanceSlicePolicy == nil {
		return nil, fmt.Errorf("runtime bootstrap requires acceptance slice policy %s", runtimecontract.RuntimeAcceptanceSlicePolicySchemaCurrent)
	}
	dodLines := make([]string, 0, len(dod.Items))
	for idx, item := range dod.Items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		dodLines = append(dodLines, fmt.Sprintf("%d. %s", idx+1, item))
	}
	compiled, err := compileRuntimeCoverage(goal, dod, activation)
	if err != nil {
		return nil, err
	}
	preflightFailures := make([]runtimeCoverageBindingPreflightFailure, 0)
	for idx := range compiled.Focuses {
		bindings, bindingErr := compileRuntimeCoverageEvidenceBindings(compiled.Focuses[idx], definitionRevision)
		if bindingErr != nil {
			preflightFailures = append(preflightFailures, runtimeCoverageBindingPreflightFailure{Focus: compiled.Focuses[idx], Cause: bindingErr})
			continue
		}
		if recipeErr := validateRuntimeFocusValidationRecipeDeclaration(compiled.Focuses[idx], activation.EvidencePolicies); recipeErr != nil {
			preflightFailures = append(preflightFailures, runtimeCoverageBindingPreflightFailure{Focus: compiled.Focuses[idx], Cause: recipeErr})
			continue
		}
		compiled.Focuses[idx].CoverageEvidenceBindings = bindings
		compiled.Focuses[idx] = compiled.Focuses[idx].Normalized()
	}
	if len(preflightFailures) != 0 {
		return nil, fmt.Errorf("coverage_binding_unavailable: %s", formatRuntimeCoverageBindingPreflightFailures(preflightFailures, compiled.CoverageItems))
	}
	reconciliationContract, err := runtimecontract.CompileRuntimeCWIReconciliationContract(definitionRevision, compiled.Focuses)
	if err != nil {
		return nil, fmt.Errorf("coverage_binding_unavailable: reconciliation contract: %w", err)
	}
	// Goal/DoD compilation declares the required role and bounded discovery
	// source. Runtime then asks the owning File block to select from the actual
	// workspace manifest before any candidate request; activation never asks a
	// user to provide a file path.
	focuses := compiled.Focuses
	capabilityContract := []string{
		runtimecontract.StepKindFileEdit,
		runtimecontract.StepKindFileRead,
		runtimecontract.StepKindShellCommand,
		runtimecontract.RuntimeBlockKindManagedValidationSession,
		runtimecontract.RuntimeBlockKindDecisionRequest,
	}
	// Evidence policies are compiled into sealed focuses before capability
	// prerequisites are assembled.  Their accepted operation kinds are just as
	// authoritative as profile-declared kinds; otherwise a template may seal a
	// valid MCP/Web/SSH proof shape while the Run aggregate never acquires the
	// corresponding owner capability.  Keep this derivation here (activation
	// only) so Runtime still receives a complete, finite capability contract.
	focusCapabilityKinds, err := runtimeFocusCapabilityKinds(focuses)
	if err != nil {
		return nil, err
	}
	capabilityContract = append(capabilityContract, focusCapabilityKinds...)
	profileCapabilityKinds, err := runtimeProfileCapabilityKinds(activation.ExecutionContract.ProfileContracts)
	if err != nil {
		return nil, err
	}
	capabilityContract = append(capabilityContract, profileCapabilityKinds...)
	requirements := []runtimecontract.RuntimeRequirement{
		{Kind: runtimecontract.PrerequisiteKindManifest, OwnerModuleID: runtimecontract.RuntimeModuleFile, Reason: "workspace manifest required"},
		{Kind: runtimecontract.PrerequisiteKindValidationScratch, OwnerModuleID: runtimecontract.RuntimeModuleOrchestrator, Reason: "validation scratch required"},
		{Kind: runtimecontract.PrerequisiteKindProviderCeiling, OwnerModuleID: runtimecontract.RuntimeModuleLLM, MinimumBytes: 96 * 1024, Reason: "provider ceiling required"},
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleFile, CapabilityClass: runtimecontract.RuntimeCapabilityClassStateChange, RequiredCapabilities: []string{runtimecontract.StepKindFileEdit}, Reason: "state-change capability required"},
		// file/read and managed validation are sealed non-mutating validation operations.
		// shell/command remains available for state-change-capable shell work, but generic
		// produce_validation must not depend exclusively on a side-effecting kind.
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleFile, CapabilityClass: runtimecontract.RuntimeCapabilityClassValidation, RequiredCapabilities: []string{runtimecontract.StepKindFileRead}, Reason: "read-only validation capability required"},
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleShell, CapabilityClass: runtimecontract.RuntimeCapabilityClassValidation, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, Reason: "managed validation capability required"},
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleDecision, CapabilityClass: runtimecontract.RuntimeCapabilityClassGovernance, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindDecisionRequest}, Reason: "governance capability required"},
	}
	profileRequirements, err := runtimeProfileCapabilityRequirements(activation.ExecutionContract.ProfileContracts, activation.MCPCapabilities)
	if err != nil {
		return nil, err
	}
	requirements = append(requirements, profileRequirements...)
	focusRequirements, err := runtimeFocusCapabilityRequirements(focuses, activation.MCPCapabilities)
	if err != nil {
		return nil, err
	}
	requirements = append(requirements, focusRequirements...)
	admissibilityRequirements, err := runtimeOperationAdmissibilityRequirements(capabilityContract)
	if err != nil {
		return nil, err
	}
	requirements = append(requirements, admissibilityRequirements...)
	stateRequirements := runtimeProfileCapabilityStateRequirements(activation.ExecutionContract.ProfileContracts)
	requirements = append(requirements, stateRequirements...)
	stateChangeRequirement := runtimecontract.RuntimeRunStateChangeRequirement{
		Required: activation.ExecutionContract.Normalized().StateChangePolicy == runtimecontract.RuntimeStateChangeMustProduce,
	}
	if stateChangeRequirement.Required {
		// Seal all coverage IDs as the Run-level state-change alignment set. Focus-level
		// requirements stay independent; ledger satisfaction needs coverage-bound evidence.
		for _, focus := range focuses {
			stateChangeRequirement.CoverageWorkItemIDs = append(stateChangeRequirement.CoverageWorkItemIDs, focus.CoverageWorkItemIDs...)
		}
	}
	slicePolicy := activation.ExecutionContract.AcceptanceSlicePolicy.Normalized()
	convergencePolicy := runtimecontract.DefaultRuntimeConvergencePolicy()
	convergencePolicy.MaxAcceptanceSliceWorkItems = slicePolicy.MaxWorkItems
	convergencePolicy.MaxFocuses = slicePolicy.MaxFocuses
	convergencePolicy.MaxCandidateContextItems = slicePolicy.MaxCandidateContextItems
	if maxAttempts := activation.ExecutionContract.MaxRepairAttemptsPerFocus; maxAttempts > 0 {
		convergencePolicy.MaxRepairAttemptsPerFocus = maxAttempts
	}
	if maxRetries, ok := runtimecontract.RuntimeValidationRetryLimit(activation.ExecutionContract.ValidationPolicies, runtimecontract.EvidenceKindValidation); ok {
		convergencePolicy.MaxValidationOnlyRetriesPerFocus = maxRetries
	}
	// Recovery policy is compiled once into the bootstrap. In particular, normal
	// owner resource-change refreshes and stale/no-progress recovery have
	// separate typed allowances; Resume only reads this sealed policy.
	recoveryPolicy := runtimecontract.DefaultRuntimeRecoveryPolicy()
	bootstrap := runtimecontract.RuntimeBootstrap{
		SchemaVersion: runtimecontract.RuntimeStateSchemaVersionCurrent,
		Activation:    activation,
		Definition: runtimecontract.RuntimeDefinitionContext{
			Revision: definitionRevision, GoalID: goal.ID, GoalDefinition: goal.Text,
			DoDID: dod.ID, DoDDefinition: strings.Join(dodLines, "\n"),
			ChangeImpact:       changeImpact,
			CapabilityContract: capabilityContract,
		},
		ReconciliationContract: reconciliationContract,
		Focuses:                focuses,
		Slices:                 compiled.Slices,
		CoverageItems:          compiled.CoverageItems,
		Requirements:           requirements,
		StateChangeRequirement: stateChangeRequirement,
		ConvergencePolicy:      convergencePolicy,
		RecoveryPolicy:         recoveryPolicy,
	}
	bootstrap = bootstrap.Normalized()
	if err := bootstrap.Validate(); err != nil {
		return nil, err
	}
	return &bootstrap, nil
}

// validateRuntimeFocusValidationRecipeDeclaration rejects a static proof
// contradiction before a Run exists: a Shell/managed-validation proof cannot
// name a recipe which activation did not declare for the Shell owner. MCP
// recipe identity remains verified by the MCP owner. Executable availability
// remains the Shell/MCP owner's later admissibility fact; this helper performs
// no owner I/O and grants no coverage credit.
func validateRuntimeFocusValidationRecipeDeclaration(focus runtimecontract.CoverageFocus, policies []runtimecontract.RuntimeEvidencePolicy) error {
	focus = focus.Normalized()
	if !containsRuntimeEvidenceKind(focus.RequiredEvidenceKinds, runtimecontract.EvidenceKindValidation) && !focus.RequireExecutableBehaviorEvidence {
		return nil
	}
	recipeID := strings.TrimSpace(focus.ValidationRecipeID)
	if recipeID == "" {
		return nil // binding compilation owns the required-recipe diagnostic.
	}
	allowedShell := false
	for _, kind := range runtimeCoverageProofOperationKinds(focus.AcceptedStepKinds, runtimecontract.EvidenceKindValidation) {
		switch runtimecontract.CanonicalRuntimeBlockKind(kind) {
		case runtimecontract.StepKindShellCommand, runtimecontract.RuntimeBlockKindManagedValidationSession:
			allowedShell = true
		}
	}
	if strings.HasPrefix(recipeID, "mcp/tool_call/") {
		return nil
	}
	if !allowedShell {
		return nil
	}
	if _, found := runtimecontract.RuntimeValidationRecipeForID(runtimecontract.RuntimeValidationRecipeDeclarationsFromEvidencePolicies(policies), recipeID); !found {
		return fmt.Errorf("focus %q validation recipe %q has no declared Shell recipe", focus.ID, recipeID)
	}
	return nil
}

// compileRuntimeCoverageEvidenceBindings freezes profile/template proof
// declarations into the activation output. It uses only already compiled focus
// policy and definition identity; it does not inspect a workspace, ask an LLM,
// or mutate a Runtime aggregate.
func compileRuntimeCoverageEvidenceBindings(focus runtimecontract.CoverageFocus, definitionRevision string) ([]runtimecontract.CoverageEvidenceBinding, error) {
	focus = focus.Normalized()
	definitionRevision = strings.TrimSpace(definitionRevision)
	if focus.ID == "" || definitionRevision == "" || len(focus.CoverageWorkItemIDs) == 0 {
		return nil, fmt.Errorf("focus identity and coverage are required")
	}
	if err := runtimecontract.ValidateRuntimeFocusEvidenceDeclaration(focus); err != nil {
		return nil, err
	}
	if len(focus.RequiredEvidenceKinds) == 0 || len(focus.AcceptedStepKinds) == 0 {
		return nil, fmt.Errorf("focus %q has no deterministic evidence kind or allowed operation", focus.ID)
	}
	if len(focus.TargetRefs) == 0 && focus.TargetSelectorRef == "" && focus.ProspectiveTargetRef == "" {
		return nil, fmt.Errorf("focus %q has no deterministic proof target or owner selector", focus.ID)
	}
	proofs := make([]runtimecontract.CoverageEvidenceProof, 0, len(focus.RequiredEvidenceKinds))
	for _, evidenceKind := range focus.RequiredEvidenceKinds {
		allowedKinds := runtimeCoverageProofOperationKinds(focus.AcceptedStepKinds, evidenceKind)
		if len(allowedKinds) == 0 {
			return nil, fmt.Errorf("focus %q has no deterministic %s proof operation", focus.ID, evidenceKind)
		}
		proof := runtimecontract.CoverageEvidenceProof{
			EvidenceKind:             evidenceKind,
			AllowedOperationKinds:    allowedKinds,
			AllowedOperationVariants: runtimeCoverageProofOperationVariants(focus.AcceptedOperationVariants, allowedKinds),
		}
		switch evidenceKind {
		case runtimecontract.EvidenceKindStateChange:
			proof.RequiredEffectScopes = []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable, runtimecontract.RuntimeEffectRemoteDurable}
		case runtimecontract.EvidenceKindValidation:
			proof.RequiredEffectScopes = []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone, runtimecontract.RuntimeEffectEphemeralSession}
			proof.ValidationRecipeID = focus.ValidationRecipeID
			proof.RequiredCheckRefs = append([]string(nil), focus.RequiredCheckRefs...)
			proof.MetricConstraints = append([]string(nil), focus.MetricConstraints...)
			proof.ArtifactRefs = append([]string(nil), focus.ArtifactRefs...)
		default:
			return nil, fmt.Errorf("focus %q has unsupported required evidence kind %q", focus.ID, evidenceKind)
		}
		proofs = append(proofs, proof.Normalized())
	}
	requiresValidationProof := false
	for _, kind := range focus.RequiredEvidenceKinds {
		if kind == runtimecontract.EvidenceKindValidation {
			requiresValidationProof = true
			break
		}
	}
	if (requiresValidationProof || focus.RequireExecutableBehaviorEvidence) && focus.ValidationRecipeID == "" {
		return nil, fmt.Errorf("focus %q validation proof has no declared recipe", focus.ID)
	}
	if requiresValidationProof && len(focus.RequiredCheckRefs) == 0 {
		return nil, fmt.Errorf("focus %q validation proof has no CWI-specific required checks", focus.ID)
	}
	if focus.RequireExecutableBehaviorEvidence && len(focus.RequiredCheckRefs) == 0 {
		return nil, fmt.Errorf("focus %q executable behavior proof has no CWI-specific required checks", focus.ID)
	}
	declarationRef := strings.TrimSpace(focus.EvidenceDeclarationRef)
	if declarationRef == "" {
		declarationRef = strings.Join([]string{"coverage", focus.AssessmentScope, focus.AssessmentID}, "/")
	}
	bindings := make([]runtimecontract.CoverageEvidenceBinding, 0, len(focus.CoverageWorkItemIDs))
	for _, coverageID := range focus.CoverageWorkItemIDs {
		binding := runtimecontract.CoverageEvidenceBinding{
			ID:                       "coverage-binding/" + focus.ID + "/" + string(coverageID),
			CoverageWorkItemID:       coverageID,
			FocusID:                  focus.ID,
			DefinitionRevision:       definitionRevision,
			Proofs:                   append([]runtimecontract.CoverageEvidenceProof(nil), proofs...),
			TargetRefs:               append([]string(nil), focus.TargetRefs...),
			TargetSelectorRef:        focus.TargetSelectorRef,
			TargetSelectorOwnerRef:   focus.TargetSelectorOwnerRef,
			TargetSelectorSubjectRef: focus.TargetSelectorSubjectRef,
			ProspectiveTargetRef:     focus.ProspectiveTargetRef,
			DeclarationRef:           declarationRef,
		}.Normalized()
		binding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(binding)
		if err := binding.Validate(); err != nil {
			return nil, err
		}
		bindings = append(bindings, binding)
	}
	if err := runtimecontract.ValidateCoverageEvidenceBindingsForFocus(runtimecontract.CoverageFocus{ID: focus.ID, CoverageWorkItemIDs: focus.CoverageWorkItemIDs, EvidenceDeclarationRef: focus.EvidenceDeclarationRef, RequiredEvidenceKinds: focus.RequiredEvidenceKinds, CoverageEvidenceBindings: bindings}, definitionRevision); err != nil {
		return nil, err
	}
	return runtimecontract.NormalizeCoverageEvidenceBindings(bindings), nil
}

type runtimeCoverageBindingPreflightFailure struct {
	Focus runtimecontract.CoverageFocus
	Cause error
}

// formatRuntimeCoverageBindingPreflightFailures keeps activation failure
// diagnostics complete and deterministic.  It consumes only compiler output:
// no filesystem, capability owner, Run, provider, or state transition is
// involved.  A restart with an incomplete proof contract therefore has a
// usable next step while still creating zero Runtime side effects.
func formatRuntimeCoverageBindingPreflightFailures(failures []runtimeCoverageBindingPreflightFailure, items []runtimecontract.RuntimeCoverageItem) string {
	byID := make(map[common.CoverageWorkItemID]runtimecontract.RuntimeCoverageItem, len(items))
	for _, item := range items {
		byID[item.ID] = item.Normalized()
	}
	type row struct {
		coverageID string
		message    string
	}
	rows := make([]row, 0, len(failures))
	for _, failure := range failures {
		cause := "unknown proof compilation failure"
		if failure.Cause != nil {
			cause = strings.TrimSpace(failure.Cause.Error())
		}
		missing := runtimeCoverageBindingPreflightMissingFields(failure.Focus)
		for _, coverageID := range failure.Focus.CoverageWorkItemIDs {
			item := byID[coverageID]
			declarationRef := strings.TrimSpace(failure.Focus.EvidenceDeclarationRef)
			if declarationRef == "" {
				declarationRef = strings.Join([]string{"coverage", strings.TrimSpace(item.SourceScope), strings.TrimSpace(item.SourceID)}, "/")
			}
			missingPart := ""
			if len(missing) != 0 {
				missingPart = fmt.Sprintf(" missing=%s", strings.Join(missing, ","))
			}
			rows = append(rows, row{
				coverageID: string(coverageID),
				message:    fmt.Sprintf("CWI %s source=%s/%s declaration=%s%s: %s", coverageID, item.SourceScope, item.SourceID, declarationRef, missingPart, cause),
			})
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].coverageID != rows[j].coverageID {
			return rows[i].coverageID < rows[j].coverageID
		}
		return rows[i].message < rows[j].message
	})
	messages := make([]string, 0, len(rows))
	for _, item := range rows {
		messages = append(messages, item.message)
	}
	return strings.Join(messages, "; ")
}

// runtimeCoverageBindingPreflightMissingFields adds a stable, field-level
// explanation to the original contract error.  It is deliberately derived
// only from the already compiled focus and therefore has no authority to
// relax a proof, inspect a workspace, or select a state transition.
func runtimeCoverageBindingPreflightMissingFields(focus runtimecontract.CoverageFocus) []string {
	focus = focus.Normalized()
	if !runtimecontract.RuntimeFocusRequiresIndependentEvidenceBinding(focus) {
		return nil
	}
	missing := make([]string, 0, 4)
	if strings.TrimSpace(focus.EvidenceDeclarationRef) == "" || strings.HasSuffix(focus.EvidenceDeclarationRef, "/*") {
		missing = append(missing, "cwiSpecificDeclaration")
	}
	if len(focus.TargetRefs) == 0 && strings.TrimSpace(focus.TargetSelectorRef) == "" && strings.TrimSpace(focus.ProspectiveTargetRef) == "" {
		missing = append(missing, "targetOrSelector")
	}
	if containsRuntimeEvidenceKind(focus.RequiredEvidenceKinds, runtimecontract.EvidenceKindValidation) || focus.RequireExecutableBehaviorEvidence {
		if strings.TrimSpace(focus.ValidationRecipeID) == "" {
			missing = append(missing, "validationRecipeId")
		}
		if len(focus.RequiredCheckRefs) == 0 {
			missing = append(missing, "requiredCheckRefs")
		}
	}
	return missing
}

func containsRuntimeEvidenceKind(values []string, wanted string) bool {
	wanted = strings.TrimSpace(wanted)
	for _, value := range values {
		if strings.TrimSpace(value) == wanted {
			return true
		}
	}
	return false
}

// runtimeCoverageProofOperationKinds narrows the profile-declared focus
// operations to the evidence kind they can actually produce. This is
// activation-time declaration compilation only: it reads the static operation
// descriptor and never asks an owner, starts a runtime action, or infers a
// target from Goal/DoD text.
func runtimeCoverageProofOperationKinds(values []string, evidenceKind string) []string {
	ret := make([]string, 0, len(values))
	for _, value := range values {
		kind := runtimecontract.CanonicalRuntimeBlockKind(value)
		if runtimecontract.RuntimeOperationKindSupportsCoverageEvidence(kind, evidenceKind) {
			ret = append(ret, kind)
		}
	}
	seen := make(map[string]struct{}, len(ret))
	for _, kind := range ret {
		seen[kind] = struct{}{}
	}
	ret = ret[:0]
	for kind := range seen {
		ret = append(ret, kind)
	}
	sort.Strings(ret)
	return ret
}

// runtimeCoverageProofOperationVariants preserves only variants belonging to
// the exact evidence-kind operation set. The binding then seals this filtered
// subset and the runtime matcher never needs to use a cross-kind union.
func runtimeCoverageProofOperationVariants(values []runtimecontract.RuntimeOperationVariantRef, allowedKinds []string) []runtimecontract.RuntimeOperationVariantRef {
	allowed := map[string]struct{}{}
	for _, kind := range allowedKinds {
		allowed[runtimecontract.CanonicalRuntimeBlockKind(kind)] = struct{}{}
	}
	ret := make([]runtimecontract.RuntimeOperationVariantRef, 0, len(values))
	for _, value := range values {
		value.Kind = runtimecontract.CanonicalRuntimeBlockKind(value.Kind)
		value.VariantID = strings.TrimSpace(value.VariantID)
		if _, found := allowed[value.Kind]; found {
			ret = append(ret, value)
		}
	}
	return runtimecontract.NormalizeRuntimeOperationVariantRefs(ret)
}

func runtimeOperationAdmissibilityRequirements(kinds []string) ([]runtimecontract.RuntimeRequirement, error) {
	byOwner := map[string][]string{}
	for _, kind := range kinds {
		canonical := runtimecontract.CanonicalRuntimeBlockKind(kind)
		descriptor, ok := runtimecontract.RuntimeOperationDescriptorForKind(canonical)
		if !ok {
			return nil, fmt.Errorf("runtime operation kind %q has no descriptor", kind)
		}
		if descriptor.OwnerModuleID == "" || !runtimeDescriptorProducesCandidate(descriptor) {
			continue
		}
		byOwner[descriptor.OwnerModuleID] = append(byOwner[descriptor.OwnerModuleID], canonical)
	}
	owners := make([]string, 0, len(byOwner))
	for owner := range byOwner {
		owners = append(owners, owner)
	}
	sort.Strings(owners)
	ret := make([]runtimecontract.RuntimeRequirement, 0, len(owners))
	for _, owner := range owners {
		values := byOwner[owner]
		sort.Strings(values)
		ret = append(ret, runtimecontract.RuntimeRequirement{
			Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: owner,
			RequiredCapabilities: values, Reason: "operation parameter admissibility required",
		})
	}
	return ret, nil
}

func runtimeDescriptorProducesCandidate(descriptor runtimecontract.RuntimeOperationDescriptor) bool {
	for _, state := range descriptor.AllowedStates {
		if state == runtimecontract.ConvergenceStateNeedStateChange || state == runtimecontract.ConvergenceStateNeedValidation {
			return true
		}
	}
	return false
}

func runtimeProfileCapabilityStateRequirements(profiles []runtimecontract.RuntimeProfileContract) []runtimecontract.RuntimeRequirement {
	ret := []runtimecontract.RuntimeRequirement{}
	seen := map[string]struct{}{}
	for _, profile := range runtimecontract.NormalizeRuntimeProfileContracts(profiles) {
		for _, evidence := range profile.EvidenceRequirements {
			for _, state := range evidence.CapabilityStateRequirements {
				key := state.OwnerModuleID + "|" + state.CapabilityStateKind + "|" + state.SubjectRef
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				ret = append(ret, runtimecontract.RuntimeRequirement{Kind: runtimecontract.PrerequisiteKindCapabilityState, OwnerModuleID: state.OwnerModuleID, CapabilityStateKind: state.CapabilityStateKind, SubjectRef: state.SubjectRef, Reason: "profile capability state required"})
			}
		}
	}
	return ret
}

func runtimeProfileCapabilityKinds(profiles []runtimecontract.RuntimeProfileContract) ([]string, error) {
	ret := []string{}
	seen := map[string]struct{}{}
	for _, profile := range runtimecontract.NormalizeRuntimeProfileContracts(profiles) {
		for _, requirement := range profile.EvidenceRequirements {
			for _, kind := range requirement.AcceptedStepKinds {
				canonical := runtimecontract.CanonicalRuntimeBlockKind(kind)
				if canonical == "" {
					continue
				}
				if _, ok := runtimecontract.RuntimeOperationDescriptorForKind(canonical); !ok {
					return nil, fmt.Errorf("profile accepted step kind %q has no runtime operation descriptor", kind)
				}
				if _, ok := seen[canonical]; ok {
					continue
				}
				seen[canonical] = struct{}{}
				ret = append(ret, canonical)
			}
		}
		for _, recipe := range profile.BootstrapRecipes {
			for _, artifact := range recipe.Artifacts {
				for _, kind := range artifact.AllowedOperationKinds {
					canonical := runtimecontract.CanonicalRuntimeBlockKind(kind)
					if canonical == "" {
						continue
					}
					if _, ok := runtimecontract.RuntimeOperationDescriptorForKind(canonical); !ok {
						return nil, fmt.Errorf("profile bootstrap artifact operation kind %q has no runtime operation descriptor", kind)
					}
					if _, ok := seen[canonical]; ok {
						continue
					}
					seen[canonical] = struct{}{}
					ret = append(ret, canonical)
				}
			}
		}
	}
	return ret, nil
}

// runtimeFocusCapabilityKinds returns the executable operation catalog
// referenced by the already compiled focus contracts.  Focuses may originate
// from template EvidencePolicies rather than ProfileContracts, so capability
// compilation must consume both sources.
func runtimeFocusCapabilityKinds(focuses []runtimecontract.CoverageFocus) ([]string, error) {
	seen := map[string]struct{}{}
	ret := make([]string, 0)
	for _, focus := range focuses {
		for _, value := range focus.AcceptedStepKinds {
			kind := runtimecontract.CanonicalRuntimeBlockKind(value)
			if kind == "" {
				continue
			}
			if _, ok := runtimecontract.RuntimeOperationDescriptorForKind(kind); !ok {
				return nil, fmt.Errorf("focus accepted step kind %q has no runtime operation descriptor", value)
			}
			if _, ok := seen[kind]; ok {
				continue
			}
			seen[kind] = struct{}{}
			ret = append(ret, kind)
		}
	}
	sort.Strings(ret)
	return ret, nil
}

// runtimeFocusCapabilityRequirements compiles owner capability
// prerequisites from sealed focus proof shapes.  This is deliberately a pure
// activation compiler: it does not inspect the workspace or invoke an owner.
// MCP tool effect remains determined by the activation-sealed capability
// metadata; absent metadata stays conservative and is treated as the
// descriptor's default state-change class.
func runtimeFocusCapabilityRequirements(focuses []runtimecontract.CoverageFocus, mcpCapabilities []runtimecontract.RuntimeMCPCapabilityContext) ([]runtimecontract.RuntimeRequirement, error) {
	ret := make([]runtimecontract.RuntimeRequirement, 0)
	seen := map[string]struct{}{}
	for _, focus := range focuses {
		requirement := runtimecontract.RuntimeProfileEvidenceRequirement{
			EvidenceRole:              focus.EvidenceRole,
			RequiredEvidenceKinds:     append([]string(nil), focus.RequiredEvidenceKinds...),
			AcceptedOperationVariants: append([]runtimecontract.RuntimeOperationVariantRef(nil), focus.AcceptedOperationVariants...),
		}
		// A focus that requires durable proof must acquire the state-change
		// capability even when its operation kind (notably mcp/tool_call) has a
		// dynamic default class.
		for _, evidenceKind := range focus.RequiredEvidenceKinds {
			if evidenceKind == runtimecontract.EvidenceKindStateChange {
				requirement.EvidenceRole = "primary_state_change"
				break
			}
		}
		for _, value := range focus.AcceptedStepKinds {
			kind := runtimecontract.CanonicalRuntimeBlockKind(value)
			if kind == "" {
				continue
			}
			descriptor, ok := runtimecontract.RuntimeOperationDescriptorForKind(kind)
			if !ok || descriptor.OwnerModuleID == "" {
				if !ok {
					return nil, fmt.Errorf("focus accepted step kind %q has no runtime operation descriptor", value)
				}
				return nil, fmt.Errorf("focus accepted step kind %q has no runtime owner", value)
			}
			capabilityClass := runtimeProfileCapabilityClass(requirement, kind, descriptor, mcpCapabilities)
			key := descriptor.OwnerModuleID + "|" + string(capabilityClass) + "|" + kind
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			ret = append(ret, runtimecontract.RuntimeRequirement{
				Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: descriptor.OwnerModuleID,
				CapabilityClass: capabilityClass, RequiredCapabilities: []string{kind}, Reason: "focus proof capability required",
			})
		}
	}
	return ret, nil
}

func runtimeProfileCapabilityRequirements(profiles []runtimecontract.RuntimeProfileContract, mcpCapabilities []runtimecontract.RuntimeMCPCapabilityContext) ([]runtimecontract.RuntimeRequirement, error) {
	ret := []runtimecontract.RuntimeRequirement{}
	seen := map[string]struct{}{}
	for _, profile := range runtimecontract.NormalizeRuntimeProfileContracts(profiles) {
		for _, requirement := range profile.EvidenceRequirements {
			for _, kind := range requirement.AcceptedStepKinds {
				canonical := runtimecontract.CanonicalRuntimeBlockKind(kind)
				descriptor, ok := runtimecontract.RuntimeOperationDescriptorForKind(canonical)
				if !ok {
					return nil, fmt.Errorf("profile accepted step kind %q has no runtime operation descriptor", kind)
				}
				if descriptor.OwnerModuleID == "" {
					return nil, fmt.Errorf("profile accepted step kind %q has no owner module", kind)
				}
				capabilityClass := runtimeProfileCapabilityClass(requirement, canonical, descriptor, mcpCapabilities)
				key := descriptor.OwnerModuleID + "|" + string(capabilityClass) + "|" + canonical
				if _, exists := seen[key]; exists {
					continue
				}
				seen[key] = struct{}{}
				ret = append(ret, runtimecontract.RuntimeRequirement{
					Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: descriptor.OwnerModuleID, CapabilityClass: capabilityClass,
					RequiredCapabilities: []string{canonical}, Reason: "profile evidence capability required",
				})
			}
		}
		for _, recipe := range profile.BootstrapRecipes {
			for _, artifact := range recipe.Artifacts {
				for _, kind := range artifact.AllowedOperationKinds {
					canonical := runtimecontract.CanonicalRuntimeBlockKind(kind)
					descriptor, ok := runtimecontract.RuntimeOperationDescriptorForKind(canonical)
					if !ok || descriptor.OwnerModuleID == "" {
						return nil, fmt.Errorf("profile bootstrap artifact operation kind %q has no runtime owner descriptor", kind)
					}
					key := descriptor.OwnerModuleID + "|" + string(runtimecontract.RuntimeCapabilityClassStateChange) + "|" + canonical
					if _, exists := seen[key]; exists {
						continue
					}
					seen[key] = struct{}{}
					ret = append(ret, runtimecontract.RuntimeRequirement{
						Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: descriptor.OwnerModuleID, CapabilityClass: runtimecontract.RuntimeCapabilityClassStateChange,
						RequiredCapabilities: []string{canonical}, Reason: "profile bootstrap artifact capability required",
					})
				}
			}
		}
	}
	return ret, nil
}

// runtimeProfileCapabilityClass maps a profile evidence requirement to the
// inventory class that the owner must acquire for this Run.  Most operation
// descriptors have a fixed class.  MCP tool calls are intentionally different:
// their concrete effect is capability-owner metadata sealed in Activation, so a
// selected non-durable tool can satisfy validation while a durable tool cannot.
// This only changes the capability inventory; candidate seal still verifies the
// exact server/tool and sealed effect scope before execution.
func runtimeProfileCapabilityClass(requirement runtimecontract.RuntimeProfileEvidenceRequirement, kind string, descriptor runtimecontract.RuntimeOperationDescriptor, mcpCapabilities []runtimecontract.RuntimeMCPCapabilityContext) runtimecontract.RuntimeCapabilityClass {
	if requirement.EvidenceRole == "primary_state_change" || len(requirement.AcceptedOperationVariants) > 0 {
		return runtimecontract.RuntimeCapabilityClassStateChange
	}
	if kind != runtimecontract.StepKindMCPToolCall {
		return descriptor.CapabilityClass
	}
	sealedCapabilities := make([]runtimecontract.RuntimeMCPCapabilityContext, 0, len(mcpCapabilities))
	for _, capability := range mcpCapabilities {
		sealedCapabilities = append(sealedCapabilities, runtimecontract.SealMCPCapabilityEffect(capability))
	}
	if runtimecontract.OperationEffectCompatibleWithSealedCapabilities(
		kind,
		runtimecontract.ConvergenceStateNeedValidation,
		runtimecontract.RuntimeCapabilities{ValidationKinds: []string{kind}},
		sealedCapabilities,
	) {
		return runtimecontract.RuntimeCapabilityClassValidation
	}
	return descriptor.CapabilityClass
}
