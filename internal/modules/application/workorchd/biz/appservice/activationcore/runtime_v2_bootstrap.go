package activationcore

import (
	"fmt"
	"sort"
	"strings"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func RuntimeV2Bootstrap(goal *workspacemodel.Goal, dod *workspacemodel.DoD, definitionRevision string, changeImpact *runtimecontract.RuntimeDefinitionChangeImpact, activation runtimecontract.RuntimeActivationContext) (*runtimecontract.RuntimeBootstrap, error) {
	definitionRevision = strings.TrimSpace(definitionRevision)
	if goal == nil || dod == nil || goal.ID == "" || dod.ID == "" || goal.WorkspaceID == "" || goal.WorkspaceID != dod.WorkspaceID ||
		strings.TrimSpace(goal.Text) == "" || len(dod.Items) == 0 || definitionRevision == "" {
		return nil, common.ErrInvalidArgument
	}
	if activation.ExecutionContract.AcceptanceSlicePolicy == nil {
		return nil, fmt.Errorf("runtime v2 bootstrap requires acceptance slice policy %s", runtimecontract.RuntimeAcceptanceSlicePolicySchemaV2)
	}
	dodLines := make([]string, 0, len(dod.Items))
	for idx, item := range dod.Items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		dodLines = append(dodLines, fmt.Sprintf("%d. %s", idx+1, item))
	}
	compiled, err := compileRuntimeV2Coverage(goal, dod, activation)
	if err != nil {
		return nil, err
	}
	focuses := compiled.Focuses
	capabilityContract := []string{
		runtimecontract.StepKindFileEdit,
		runtimecontract.StepKindFileRead,
		runtimecontract.StepKindShellCommand,
		runtimecontract.RuntimeBlockKindManagedValidationSession,
		runtimecontract.RuntimeBlockKindDecisionRequest,
	}
	profileCapabilityKinds, err := runtimeV2ProfileCapabilityKinds(activation.ExecutionContract.ProfileContracts)
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
	profileRequirements, err := runtimeV2ProfileCapabilityRequirements(activation.ExecutionContract.ProfileContracts)
	if err != nil {
		return nil, err
	}
	requirements = append(requirements, profileRequirements...)
	admissibilityRequirements, err := runtimeV2OperationAdmissibilityRequirements(capabilityContract)
	if err != nil {
		return nil, err
	}
	requirements = append(requirements, admissibilityRequirements...)
	stateRequirements := runtimeV2ProfileCapabilityStateRequirements(activation.ExecutionContract.ProfileContracts)
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
	bootstrap := runtimecontract.RuntimeBootstrap{
		SchemaVersion: runtimecontract.RuntimeStateSchemaVersionCurrent,
		Activation:    activation,
		Definition: runtimecontract.RuntimeDefinitionContext{
			Revision: definitionRevision, GoalID: goal.ID, GoalDefinition: goal.Text,
			DoDID: dod.ID, DoDDefinition: strings.Join(dodLines, "\n"),
			ChangeImpact:       changeImpact,
			CapabilityContract: capabilityContract,
		},
		Focuses:                focuses,
		Slices:                 compiled.Slices,
		CoverageItems:          compiled.CoverageItems,
		Requirements:           requirements,
		StateChangeRequirement: stateChangeRequirement,
		ConvergencePolicy:      convergencePolicy,
		RecoveryPolicy:         runtimecontract.DefaultRuntimeRecoveryPolicy(),
	}
	bootstrap = bootstrap.Normalized()
	if err := bootstrap.Validate(); err != nil {
		return nil, err
	}
	return &bootstrap, nil
}

func runtimeV2OperationAdmissibilityRequirements(kinds []string) ([]runtimecontract.RuntimeRequirement, error) {
	byOwner := map[string][]string{}
	for _, kind := range kinds {
		canonical := runtimecontract.CanonicalRuntimeBlockKind(kind)
		descriptor, ok := runtimecontract.RuntimeOperationDescriptorForKind(canonical)
		if !ok {
			return nil, fmt.Errorf("runtime operation kind %q has no descriptor", kind)
		}
		if descriptor.OwnerModuleID == "" || !runtimeV2DescriptorProducesCandidate(descriptor) {
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

func runtimeV2DescriptorProducesCandidate(descriptor runtimecontract.RuntimeOperationDescriptor) bool {
	for _, action := range descriptor.AllowedActions {
		if action == runtimecontract.RuntimeActionProduceStateChange || action == runtimecontract.RuntimeActionProduceValidation {
			return true
		}
	}
	return false
}

func runtimeV2ProfileCapabilityStateRequirements(profiles []runtimecontract.RuntimeProfileContract) []runtimecontract.RuntimeRequirement {
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

func runtimeV2ProfileCapabilityKinds(profiles []runtimecontract.RuntimeProfileContract) ([]string, error) {
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
	}
	return ret, nil
}

func runtimeV2ProfileCapabilityRequirements(profiles []runtimecontract.RuntimeProfileContract) ([]runtimecontract.RuntimeRequirement, error) {
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
				// Descriptor CapabilityClass is the inventory authority. Evidence role may only
				// tighten a validation kind into a state-change requirement when the profile
				// explicitly treats the evidence as primary_state_change / accepted variants.
				capabilityClass := descriptor.CapabilityClass
				if requirement.EvidenceRole == "primary_state_change" || len(requirement.AcceptedOperationVariants) > 0 {
					capabilityClass = runtimecontract.RuntimeCapabilityClassStateChange
				}
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
	}
	return ret, nil
}
