package activationcore

import (
	"fmt"
	"sort"
	"strings"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func RuntimeBootstrap(goal *workspacemodel.Goal, dod *workspacemodel.DoD, definitionRevision string, startupInputHash string, changeImpact *runtimecontract.RuntimeDefinitionChangeImpact, activation runtimecontract.RuntimeActivationContext) (*runtimecontract.RuntimeBootstrap, error) {
	definitionRevision = strings.TrimSpace(definitionRevision)
	startupInputHash = strings.TrimSpace(startupInputHash)
	if goal == nil || dod == nil || goal.ID == "" || dod.ID == "" || goal.WorkspaceID == "" || goal.WorkspaceID != dod.WorkspaceID ||
		strings.TrimSpace(goal.Text) == "" || len(dod.Items) == 0 || definitionRevision == "" || startupInputHash == "" {
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
	definition := runtimecontract.RuntimeDefinitionContext{
		StartupInputHash: startupInputHash, Revision: definitionRevision, GoalID: goal.ID, GoalDefinition: goal.Text,
		DoDID: dod.ID, DoDDefinition: strings.Join(dodLines, "\n"),
	}
	// Goal/DoD compilation declares the required role and bounded discovery
	// source. Runtime then asks the owning File block to select from the actual
	// workspace manifest before any candidate request; activation never asks a
	// user to provide a file path.
	// Bootstrap carries only a presentation/cursor seed. The richer compiler
	// output below is still useful for deriving the finite capability and
	// prerequisite vocabulary, but target, selector, recipe, operation and
	// proof fields are semantic authority. They may enter RuntimeFacts only via
	// the Orchestrator's semantic_contract_sealed private transition.
	compiledFocuses := compiled.Focuses
	focuses := runtimeBootstrapFocusSeeds(compiledFocuses)
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
	focusCapabilityKinds, err := runtimeFocusCapabilityKinds(compiledFocuses)
	if err != nil {
		return nil, err
	}
	capabilityContract = append(capabilityContract, focusCapabilityKinds...)
	profileCapabilityKinds, err := runtimeProfileCapabilityKinds(activation.ExecutionContract.ProfileContracts)
	if err != nil {
		return nil, err
	}
	capabilityContract = append(capabilityContract, profileCapabilityKinds...)
	definition.CapabilityContract = capabilityContract
	definition.ChangeImpact = changeImpact
	definitionSeed, err := runtimecontract.CompileRuntimeDefinitionSeed(definition, activation, compiled.CoverageItems, compiled.Slices)
	if err != nil {
		return nil, fmt.Errorf("runtime definition seed: %w", err)
	}
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
	profileRequirements, err := runtimeProfileCapabilityRequirements(activation.ExecutionContract.ProfileContracts, activation.OperationPolicies)
	if err != nil {
		return nil, err
	}
	requirements = append(requirements, profileRequirements...)
	focusRequirements, err := runtimeFocusCapabilityRequirements(compiledFocuses, activation.OperationPolicies)
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
	executionBudgetPolicy := runtimecontract.RuntimeExecutionBudgetPolicyForRuntimePolicies(convergencePolicy, recoveryPolicy)
	bootstrap := runtimecontract.RuntimeBootstrap{
		SchemaVersion:          runtimecontract.RuntimeStateSchemaVersionCurrent,
		Activation:             activation,
		Definition:             definition,
		DefinitionSeed:         definitionSeed,
		Focuses:                focuses,
		Slices:                 compiled.Slices,
		CoverageItems:          compiled.CoverageItems,
		Requirements:           requirements,
		StateChangeRequirement: stateChangeRequirement,
		ConvergencePolicy:      convergencePolicy,
		RecoveryPolicy:         recoveryPolicy,
		ExecutionBudgetPolicy:  executionBudgetPolicy,
	}
	bootstrap = bootstrap.Normalized()
	if err := bootstrap.Validate(); err != nil {
		return nil, err
	}
	return &bootstrap, nil
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

func runtimeBootstrapFocusSeeds(values []runtimecontract.CoverageFocus) []runtimecontract.CoverageFocus {
	ret := make([]runtimecontract.CoverageFocus, 0, len(values))
	for _, value := range values {
		value = value.Normalized()
		ret = append(ret, runtimecontract.CoverageFocus{
			ID:                    value.ID,
			SliceID:               value.SliceID,
			AssessmentScope:       value.AssessmentScope,
			AssessmentID:          value.AssessmentID,
			AssessmentText:        value.AssessmentText,
			CoverageWorkItemIDs:   append([]common.CoverageWorkItemID(nil), value.CoverageWorkItemIDs...),
			RequiredEvidenceKinds: append([]string(nil), value.RequiredEvidenceKinds...),
			RequireAssessment:     value.RequireAssessment,
			RequiredOwnerFacts:    append([]runtimecontract.RuntimeOwnerFactRequirement(nil), value.RequiredOwnerFacts...),
			OwnerFactConvergences: runtimecontract.CloneRuntimeOwnerFactConvergences(value.OwnerFactConvergences),
		}.Normalized())
	}
	return ret
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
// Dynamic operation effect remains determined by activation-sealed owner
// policies; absent policy stays conservative and uses the descriptor's
// default capability class.
func runtimeFocusCapabilityRequirements(focuses []runtimecontract.CoverageFocus, operationPolicies []runtimecontract.RuntimeOperationPolicyFact) ([]runtimecontract.RuntimeRequirement, error) {
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
			capabilityClass := runtimeProfileCapabilityClass(requirement, kind, descriptor, operationPolicies)
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

func runtimeProfileCapabilityRequirements(profiles []runtimecontract.RuntimeProfileContract, operationPolicies []runtimecontract.RuntimeOperationPolicyFact) ([]runtimecontract.RuntimeRequirement, error) {
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
				capabilityClass := runtimeProfileCapabilityClass(requirement, canonical, descriptor, operationPolicies)
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
// inventory class that the owner must acquire for this Run. Descriptor defaults
// cover fixed kinds; any dynamic effect is read only from owner-published typed
// policies, without protocol- or capability-name branches.
func runtimeProfileCapabilityClass(requirement runtimecontract.RuntimeProfileEvidenceRequirement, kind string, descriptor runtimecontract.RuntimeOperationDescriptor, operationPolicies []runtimecontract.RuntimeOperationPolicyFact) runtimecontract.RuntimeCapabilityClass {
	if requirement.EvidenceRole == "primary_state_change" || len(requirement.AcceptedOperationVariants) > 0 {
		return runtimecontract.RuntimeCapabilityClassStateChange
	}
	if runtimecontract.OperationEffectCompatibleWithPolicies(
		kind,
		runtimecontract.ConvergenceStateNeedValidation,
		operationPolicies,
	) {
		return runtimecontract.RuntimeCapabilityClassValidation
	}
	return descriptor.CapabilityClass
}
