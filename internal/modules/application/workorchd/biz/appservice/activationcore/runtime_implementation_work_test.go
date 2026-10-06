package activationcore

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func definitionWorkPolicyForTest() runtimecontract.RuntimeImplementationWorkPolicy {
	return runtimecontract.RuntimeImplementationWorkPolicy{
		ProtocolRef: runtimecontract.RuntimeImplementationWorkProtocolV1, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		AllowedOperations:     []runtimecontract.RuntimeOperationVariantRef{{Kind: runtimecontract.StepKindFileWrite}},
		AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget},
		TargetSelectorRef:     "implementation", TargetSelectorOwnerRef: "profile", TargetSelectorSubjectRef: "work",
		TargetSelectorCandidateRefs: []string{"resource-a"}, MaxRoundsPerCWI: 2, MaxTargetsPerRound: 1,
		RequireFreshObservation: true, RequireFreshValidation: true, ProtectValidationInputs: true,
	}
}

func TestImplementationWorkCapabilityCompilationPreservesTerminalProof(t *testing.T) {
	policy := definitionWorkPolicyForTest()
	profiles := []runtimecontract.RuntimeProfileContract{{ID: "profile", EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{
		ID: "dod-001", Scope: "dodItem", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
		AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, ImplementationWorkPolicy: &policy,
	}}}}
	compiled, err := compileRuntimeCoverage(
		&workspacemodel.Goal{ID: "goal", WorkspaceID: "ws", Text: "original goal"},
		&workspacemodel.DoD{ID: "dod", WorkspaceID: "ws", Items: []string{"original criterion", "unmatched criterion"}},
		runtimecontract.RuntimeActivationContext{ExecutionContract: runtimecontract.RuntimeExecutionContract{ProfileContracts: profiles}})
	if err != nil {
		t.Fatal(err)
	}
	work, err := runtimecontract.CompileRuntimeImplementationWorkCapabilityRequirements(compiled.DefinitionFocuses, profiles)
	if err != nil || len(work) != 1 {
		t.Fatalf("work inventory: %#v, %v", work, err)
	}
	kinds := work[0].RequiredCapabilities
	admissibility, err := runtimeOperationAdmissibilityRequirements(kinds)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtimecontract.ValidateRuntimeImplementationWorkCapabilityRequirements(compiled.DefinitionFocuses, profiles, kinds, append(work, admissibility...)); err != nil {
		t.Fatal(err)
	}
	for _, focus := range compiled.DefinitionFocuses {
		for _, kind := range focus.AcceptedStepKinds {
			if kind == runtimecontract.StepKindFileWrite {
				t.Fatal("work inventory became terminal proof")
			}
		}
	}
}

func TestCompileRuntimeCoverageInstantiatesImplementationWorkWithoutChangingTerminalProof(t *testing.T) {
	for _, sourceID := range []string{"*", "dod-001"} {
		t.Run(sourceID, func(t *testing.T) {
			policy := definitionWorkPolicyForTest()
			activation := runtimecontract.RuntimeActivationContext{ExecutionContract: runtimecontract.RuntimeExecutionContract{
				StateChangePolicy: runtimecontract.RuntimeStateChangeAllowedButNotRequired,
				ProfileContracts: []runtimecontract.RuntimeProfileContract{{ID: "profile", EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{
					ID: sourceID, Scope: "dodItem", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
					AcceptedStepKinds:     []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
					RequireDirectEvidence: true, RequireExecutableBehaviorEvidence: true, ImplementationWorkPolicy: &policy,
				}}}},
			}}
			goal := &workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "original goal"}
			dod := &workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"original first criterion", "original second criterion"}}
			compiled, err := compileRuntimeCoverage(goal, dod, activation)
			if err != nil {
				t.Fatal(err)
			}
			without := activation.Clone()
			without.ExecutionContract.ProfileContracts[0].EvidenceRequirements[0].ImplementationWorkPolicy = nil
			baseline, err := compileRuntimeCoverage(goal, dod, without)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(compiled.Focuses, baseline.Focuses) || !reflect.DeepEqual(compiled.CoverageItems, baseline.CoverageItems) ||
				!reflect.DeepEqual(compiled.Slices, baseline.Slices) {
				t.Fatal("work policy changed immutable original CWI membership/dependencies or terminal proof")
			}
			for i, focus := range compiled.DefinitionFocuses {
				want := sourceID == "*" || i == 0
				if (focus.ImplementationWorkPolicy != nil) != want {
					t.Fatalf("CWI %s policy presence differs from exact source", focus.AssessmentID)
				}
				if want && !runtimecontract.SameRuntimeImplementationWorkPolicy(focus.ImplementationWorkPolicy, &policy) {
					t.Fatal("compiled Definition lost a policy boundary")
				}
			}
			if err := runtimecontract.ValidateRuntimeDefinitionImplementationWorkPolicies(compiled.DefinitionFocuses, activation.ExecutionContract.ProfileContracts); err != nil {
				t.Fatal(err)
			}
			definition := runtimecontract.RuntimeDefinitionContext{Revision: "definition-1", StartupInputHash: "startup-1"}
			withSeed, err := runtimecontract.CompileRuntimeDefinitionSeed(definition, activation, compiled.CoverageItems, compiled.Slices)
			if err != nil {
				t.Fatal(err)
			}
			withoutSeed, err := runtimecontract.CompileRuntimeDefinitionSeed(definition, without, compiled.CoverageItems, compiled.Slices)
			if err != nil {
				t.Fatal(err)
			}
			if withSeed.Hash == withoutSeed.Hash || withSeed.PolicyHash == withoutSeed.PolicyHash {
				t.Fatal("sealed seed identity ignored the policy source")
			}
			// 纯声明校验不授执行权；新 activation 必须独立经过 executable gate。
			if err := activation.ExecutionContract.Validate(); err != nil {
				t.Fatalf("合法声明被纯合同校验拒绝: %v", err)
			}
			if err := activation.ExecutionContract.ValidateExecutableAdmission(); !errors.Is(err, runtimecontract.ErrRuntimeImplementationWorkUnavailable) {
				t.Fatalf("declaration projection bypassed execution gate: %v", err)
			}
			compiled.DefinitionFocuses[0].ImplementationWorkPolicy.TargetSelectorCandidateRefs[0] = "changed"
			if sourceID == "*" && compiled.DefinitionFocuses[1].ImplementationWorkPolicy.TargetSelectorCandidateRefs[0] != "resource-a" {
				t.Fatal("two CWI declarations share mutable policy data")
			}
			if policy.TargetSelectorCandidateRefs[0] != "resource-a" || activation.ExecutionContract.ProfileContracts[0].EvidenceRequirements[0].ImplementationWorkPolicy.TargetSelectorCandidateRefs[0] != "resource-a" {
				t.Fatal("CWI projection mutated original Profile")
			}
		})
	}
}

func TestCompileRuntimeCoverageRejectsImplementationWorkConflictsAfterPolicyMerge(t *testing.T) {
	base := definitionWorkPolicyForTest()
	profiles := []runtimecontract.RuntimeProfileContract{{ID: "profile", EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{
		ID: "*", Scope: "dodItem", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
		AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, ImplementationWorkPolicy: &base,
	}}}}
	activation := runtimecontract.RuntimeActivationContext{ExecutionContract: runtimecontract.RuntimeExecutionContract{ProfileContracts: profiles}}
	changed := base.Clone()
	changed.MaxRoundsPerCWI++
	exact := profiles[0].EvidenceRequirements[0]
	exact.ID, exact.ImplementationWorkPolicy = "dod-001", &changed
	activation.ExecutionContract.ProfileContracts[0].EvidenceRequirements = append(profiles[0].EvidenceRequirements, exact)
	if _, err := resolveCoverageEvidencePolicy("dodItem", "dod-001", activation, false); err == nil || !strings.Contains(err.Error(), "conflicting implementation work") {
		t.Fatalf("exact policy overrode wildcard policy: %v", err)
	}
	activation.ExecutionContract.ProfileContracts[0].EvidenceRequirements = activation.ExecutionContract.ProfileContracts[0].EvidenceRequirements[:1]
	activation.EvidencePolicies = []runtimecontract.RuntimeEvidencePolicy{{ID: "extra", AppliesTo: runtimecontract.StepKindFileWrite,
		CoverageRequirements: []runtimecontract.RuntimeCoverageEvidenceRequirement{{ID: "dod-001", Scope: "dodItem",
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, AcceptedStepKinds: []string{runtimecontract.StepKindFileWrite}}}}}
	_, err := compileRuntimeCoverage(&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "goal"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"criterion"}}, activation)
	if err == nil || !strings.Contains(err.Error(), "validation-only CWI") {
		t.Fatalf("merged evidence policy made intermediate work terminal proof: %v", err)
	}
}
