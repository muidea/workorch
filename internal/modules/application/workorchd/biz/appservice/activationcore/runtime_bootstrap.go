package activationcore

import (
	"fmt"
	"sort"
	"strings"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func RuntimeBootstrap(goal *workspacemodel.Goal, dod *workspacemodel.DoD, definitionRevision string, startupInputHash string, changeImpact *runtimecontract.RuntimeDefinitionChangeImpact, activation runtimecontract.RuntimeActivationContext) (*runtimecontract.RuntimeBootstrap, error) {
	return RuntimeBootstrapWithSourceObligations(goal, dod, definitionRevision, startupInputHash, changeImpact, activation, nil)
}

func RuntimeBootstrapWithSourceObligations(goal *workspacemodel.Goal, dod *workspacemodel.DoD, definitionRevision string, startupInputHash string, changeImpact *runtimecontract.RuntimeDefinitionChangeImpact, activation runtimecontract.RuntimeActivationContext, inventory *runtimecontract.RuntimeSourceObligationInventory) (*runtimecontract.RuntimeBootstrap, error) {
	if err := runtimecontract.ValidateRuntimeEvidencePurposeMappingAvailability(inventory); err != nil {
		return nil, err
	}
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
	if inventory != nil {
		if inventory.DefinitionRevision != definitionRevision || inventory.StartupInputHash != startupInputHash {
			return nil, fmt.Errorf("runtime source obligations do not bind sealed Definition")
		}
		compiled.CoverageItems, err = runtimecontract.CompileRuntimeExecutionProofMappingCoverage(*inventory, compiled.CoverageItems)
		if err != nil {
			return nil, err
		}
		compiled.CoverageItems, err = runtimecontract.CompileRuntimeEvidencePurposeMappingCoverage(*inventory, compiled.CoverageItems)
		if err != nil {
			return nil, err
		}
		for n := range compiled.CoverageItems {
			item := &compiled.CoverageItems[n]
			checks, err := runtimecontract.CompileRuntimeSourceObligationChecks(*inventory, *item)
			if err != nil {
				return nil, err
			}
			item.SourceObligationChecks = checks
			item.GoalExecutionAssertions, err = runtimecontract.CompileRuntimeGoalExecutionAssertions(inventory, *item, activation.ExecutionContract.ProfileContracts)
			if err != nil {
				return nil, err
			}
		}
	}
	definition := runtimecontract.RuntimeDefinitionContext{
		StartupInputHash: startupInputHash, Revision: definitionRevision, GoalID: goal.ID, GoalDefinition: goal.Text,
		DoDID: dod.ID, DoDDefinition: strings.Join(dodLines, "\n"),
	}
	if inventory != nil {
		value := inventory.Clone()
		definition.SourceObligations = &value
	}
	// Goal/DoD compilation declares the required role and bounded discovery
	// source. Runtime then asks the owning File block to select from the actual
	// workspace manifest before any candidate request; activation never asks a
	// user to provide a file path.
	// Bootstrap carries the complete immutable Definition declaration. The
	// contract type has no generation-specific target, operation, recipe or
	// proof fields; Orchestrator derives a non-authoritative queue cursor from it
	// and may add generation authority only via semantic_contract_sealed.
	compiledFocuses := compiled.Focuses
	definitionFocuses := runtimecontract.CloneRuntimeDefinitionFocuses(compiled.DefinitionFocuses)
	capabilityContract := []string{
		runtimecontract.StepKindFileEdit,
		runtimecontract.StepKindFileRead,
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
	workRequirements, err := runtimecontract.CompileRuntimeImplementationWorkCapabilityRequirements(definitionFocuses, activation.ExecutionContract.ProfileContracts)
	if err != nil {
		return nil, err
	}
	for _, requirement := range workRequirements {
		capabilityContract = append(capabilityContract, requirement.RequiredCapabilities...)
	}
	definition.CapabilityContract = capabilityContract
	definition.ChangeImpact = changeImpact
	definitionSeed, err := runtimecontract.CompileRuntimeDefinitionSeedWithSourceObligations(definition, activation, compiled.CoverageItems, compiled.Slices, inventory)
	if err != nil {
		return nil, fmt.Errorf("runtime definition seed: %w", err)
	}
	// Compile the exact per-CWI validation requirements while all immutable
	// Definition inputs are still available. The requirements are deliberately
	// not persisted as a parallel bootstrap field: Orchestrator recompiles the
	// same value from DefinitionSeed + DefinitionFocuses before owner
	// admissibility acquisition, and the semantic obligation persists the
	// generation-specific copy.
	validationProofRequirements, err := runtimecontract.CompileRuntimeValidationProofRequirements(definitionRevision, compiled.CoverageItems, definitionFocuses)
	if err != nil {
		return nil, fmt.Errorf("runtime validation proof requirements: %w", err)
	}
	definitionFocuses, err = bindRuntimeValidationProofOwnerFacts(definitionSeed, definitionFocuses, validationProofRequirements)
	if err != nil {
		return nil, fmt.Errorf("runtime validation proof owner facts: %w", err)
	}
	requirements := []runtimecontract.RuntimeRequirement{
		{Kind: runtimecontract.PrerequisiteKindManifest, OwnerModuleID: runtimecontract.RuntimeModuleFile, Reason: "workspace manifest required"},
		{Kind: runtimecontract.PrerequisiteKindValidationScratch, OwnerModuleID: runtimecontract.RuntimeModuleOrchestrator, Reason: "validation scratch required"},
		{Kind: runtimecontract.PrerequisiteKindProviderCeiling, OwnerModuleID: runtimecontract.RuntimeModuleLLM, MinimumBytes: 96 * 1024, Reason: "provider ceiling required"},
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleFile, CapabilityClass: runtimecontract.RuntimeCapabilityClassStateChange, RequiredCapabilities: []string{runtimecontract.StepKindFileEdit}, Reason: "state-change capability required"},
		// file/read is the neutral non-mutating validation baseline. Managed
		// validation is acquired only when an admitted exact focus/profile declares
		// that operation kind; MCP-only Definitions must not implicitly enlist Shell.
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleFile, CapabilityClass: runtimecontract.RuntimeCapabilityClassValidation, RequiredCapabilities: []string{runtimecontract.StepKindFileRead}, Reason: "read-only validation capability required"},
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
	requirements = append(requirements, workRequirements...)
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
		for _, focus := range definitionFocuses {
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
	executionBudgetPolicy, err := runtimecontract.RuntimeExecutionBudgetPolicyWithLimits(convergencePolicy, recoveryPolicy, activation.ExecutionContract.ExecutionLimits)
	if err != nil {
		return nil, err
	}
	bootstrap := runtimecontract.RuntimeBootstrap{
		Activation:             activation,
		Definition:             definition,
		DefinitionSeed:         definitionSeed,
		DefinitionFocuses:      definitionFocuses,
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

// bindRuntimeValidationProofOwnerFacts is the sole Application compiler step
// that connects a Definition-owned exact CWI requirement to an already
// declared selectable managed-validation owner fact. It never creates an
// owner fact, chooses a recipe, or interprets an owner-native preparation.
func bindRuntimeValidationProofOwnerFacts(
	seed runtimecontract.RuntimeDefinitionSeed,
	focuses []runtimecontract.RuntimeDefinitionFocus,
	requirements []runtimecontract.RuntimeValidationProofRequirement,
) ([]runtimecontract.RuntimeDefinitionFocus, error) {
	ret := runtimecontract.CloneRuntimeDefinitionFocuses(focuses)
	for focusIndex := range ret {
		focus := &ret[focusIndex]
		if len(focus.CoverageWorkItemIDs) != 1 {
			continue
		}
		requirement, found := runtimecontract.RuntimeValidationProofRequirementForCoverage(requirements, focus.CoverageWorkItemIDs[0])
		if !found {
			continue
		}
		refRemap := make(map[string]string)
		matchedOwnerFact := false
		for index := range focus.RequiredOwnerFacts {
			fact := focus.RequiredOwnerFacts[index].Normalized()
			if !runtimeOwnerFactCarriesValidationProofRequirement(fact) {
				continue
			}
			matchedOwnerFact = true
			if fact.ValidationProofRequirementHash != "" && fact.ValidationProofRequirementHash != requirement.Hash {
				return nil, fmt.Errorf("focus %q owner fact carries mismatched validation proof requirement", focus.ID)
			}
			oldRef := runtimecontract.RuntimeOwnerFactRequirementRef(fact)
			fact.ValidationProofRequirementHash = requirement.Hash
			focus.RequiredOwnerFacts[index] = fact
			refRemap[oldRef] = runtimecontract.RuntimeOwnerFactRequirementRef(fact)
		}
		for index := range focus.OwnerFactConvergences {
			convergence := focus.OwnerFactConvergences[index].Normalized()
			fact := convergence.Requirement
			if runtimeOwnerFactCarriesValidationProofRequirement(fact) {
				if fact.ValidationProofRequirementHash != "" && fact.ValidationProofRequirementHash != requirement.Hash {
					return nil, fmt.Errorf("focus %q owner-fact convergence carries mismatched validation proof requirement", focus.ID)
				}
				oldRef := runtimecontract.RuntimeOwnerFactRequirementRef(fact)
				fact.ValidationProofRequirementHash = requirement.Hash
				convergence.Requirement = fact
				refRemap[oldRef] = runtimecontract.RuntimeOwnerFactRequirementRef(fact)
			}
			focus.OwnerFactConvergences[index] = convergence
		}
		if !matchedOwnerFact {
			return nil, fmt.Errorf("focus %q managed validation proof requirement has no explicit selectable-surface owner fact and convergence", focus.ID)
		}
		if len(refRemap) != 0 {
			for index := range focus.OwnerFactConvergences {
				dependencies := append([]string(nil), focus.OwnerFactConvergences[index].DependsOnRequirementRefs...)
				for dependencyIndex, dependency := range dependencies {
					if replacement, ok := refRemap[dependency]; ok {
						dependencies[dependencyIndex] = replacement
					}
				}
				focus.OwnerFactConvergences[index].DependsOnRequirementRefs = dependencies
			}
		}
	}
	ret = runtimecontract.NormalizeRuntimeDefinitionFocuses(ret)
	if err := runtimecontract.ValidateRuntimeDefinitionFocuses(seed, ret); err != nil {
		return nil, err
	}
	return ret, nil
}

func runtimeOwnerFactCarriesValidationProofRequirement(value runtimecontract.RuntimeOwnerFactRequirement) bool {
	value = value.Normalized()
	return value.FactKind == runtimecontract.RuntimeOwnerFactSelectableValidationSurface &&
		value.OperationKind == runtimecontract.RuntimeBlockKindManagedValidationSession
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
			if requirement.ValidationFailureRepairConvergence != nil {
				for _, route := range requirement.ValidationFailureRepairConvergence.Normalized().Routes {
					for _, kind := range route.RepairOperationKinds {
						canonical := runtimecontract.CanonicalRuntimeBlockKind(kind)
						descriptor, ok := runtimecontract.RuntimeOperationDescriptorForKind(canonical)
						if !ok || descriptor.OwnerModuleID != route.RepairOwnerModuleID {
							return nil, fmt.Errorf("profile repair operation kind %q has no matching runtime owner descriptor", kind)
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
			if requirement.ValidationFailureRepairConvergence != nil {
				for _, route := range requirement.ValidationFailureRepairConvergence.Normalized().Routes {
					for _, kind := range route.RepairOperationKinds {
						canonical := runtimecontract.CanonicalRuntimeBlockKind(kind)
						descriptor, ok := runtimecontract.RuntimeOperationDescriptorForKind(canonical)
						if !ok || descriptor.OwnerModuleID != route.RepairOwnerModuleID {
							return nil, fmt.Errorf("profile repair operation kind %q has no matching runtime owner descriptor", kind)
						}
						key := descriptor.OwnerModuleID + "|" + string(runtimecontract.RuntimeCapabilityClassStateChange) + "|" + canonical
						if _, exists := seen[key]; exists {
							continue
						}
						seen[key] = struct{}{}
						ret = append(ret, runtimecontract.RuntimeRequirement{
							Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: descriptor.OwnerModuleID,
							CapabilityClass:      runtimecontract.RuntimeCapabilityClassStateChange,
							RequiredCapabilities: []string{canonical}, Reason: "profile validation-repair capability required",
						})
					}
				}
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
