package workorchruntimecontract

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func definitionImplementationWorkFixture(t *testing.T) (RuntimeDefinitionSeed, []RuntimeDefinitionFocus, []RuntimeProfileContract) {
	t.Helper()
	profiles := implementationWorkProfilesFixture()
	seed := RuntimeDefinitionSeed{
		SchemaVersion: RuntimeDefinitionSeedSchemaCurrent, DefinitionRevision: "definition-1", PolicyHash: "policy-1",
		CoverageItems: []RuntimeCoverageItem{{ID: "cwi-1", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-1", TargetText: "original criterion"}},
		Slices: []RuntimeAcceptanceSlice{{ID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
			CompletionPolicy: RuntimeAcceptanceSliceCompletionAll, ResultPolicy: RuntimeAcceptanceSliceResultAllowed}},
	}.Normalized()
	seed.Hash = StableRuntimeDefinitionSeedHash(seed)
	policy, err := ResolveRuntimeDefinitionImplementationWorkPolicy(profiles, "dodItem", "dod-1")
	if err != nil {
		t.Fatal(err)
	}
	focuses := []RuntimeDefinitionFocus{{
		ID: "focus-1", SliceID: "slice-1", AssessmentScope: "dodItem", AssessmentID: "dod-1", AssessmentText: "original criterion",
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, EvidenceDeclarationRef: "definition/dodItem/dod-1",
		RequiredEvidenceKinds: []string{EvidenceKindValidation}, RequireAssessment: true,
		AcceptedStepKinds: []string{RuntimeBlockKindManagedValidationSession}, RequiredCheckRefs: []string{"check-1"},
		ImplementationWorkPolicy: policy,
	}}
	return seed, focuses, profiles
}

func TestRuntimeDefinitionImplementationWorkProjectionIsExactAndDoesNotGrantProof(t *testing.T) {
	seed, focuses, profiles := definitionImplementationWorkFixture(t)
	if err := ValidateRuntimeDefinitionFocuses(seed, focuses); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRuntimeDefinitionImplementationWorkPolicies(focuses, profiles); err != nil {
		t.Fatal(err)
	}
	// This declaration must not change the final proof or leak into a mutable
	// queue cursor, proposal, working binding, or terminal evidence contract.
	without := focuses[0].Clone()
	without.ImplementationWorkPolicy = nil
	if !reflect.DeepEqual(without.RuntimeFocusSeed(), focuses[0].RuntimeFocusSeed()) ||
		!reflect.DeepEqual(without.CoverageFocusDeclaration(), focuses[0].CoverageFocusDeclaration()) {
		t.Fatal("implementation declaration leaked into mutable focus/proof authority")
	}
	if sameRuntimeDefinitionFocus(without, focuses[0]) {
		t.Fatal("Definition equality erased implementation policy")
	}
	for name, mutate := range map[string]func(*RuntimeDefinitionFocus){
		"missing policy":        func(f *RuntimeDefinitionFocus) { f.ImplementationWorkPolicy = nil },
		"expanded rounds":       func(f *RuntimeDefinitionFocus) { f.ImplementationWorkPolicy.MaxRoundsPerCWI++ },
		"other CWI source":      func(f *RuntimeDefinitionFocus) { f.AssessmentScope = "acceptanceDimension" },
		"wildcard source":       func(f *RuntimeDefinitionFocus) { f.AssessmentID = "*" },
		"terminal state change": func(f *RuntimeDefinitionFocus) { f.RequiredEvidenceKinds = []string{EvidenceKindStateChange} },
		"mixed terminal kinds": func(f *RuntimeDefinitionFocus) {
			f.RequiredEvidenceKinds = []string{EvidenceKindValidation, EvidenceKindStateChange}
		},
		"multiple CWI": func(f *RuntimeDefinitionFocus) { f.CoverageWorkItemIDs = append(f.CoverageWorkItemIDs, "cwi-2") },
	} {
		t.Run(name, func(t *testing.T) {
			invalid := focuses[0].Clone()
			mutate(&invalid)
			if err := ValidateRuntimeDefinitionImplementationWorkPolicies([]RuntimeDefinitionFocus{invalid}, profiles); err == nil {
				t.Fatal("invalid exact-CWI policy projection accepted")
			}
		})
	}
	if err := ValidateRuntimeDefinitionImplementationWorkPolicies(focuses, nil); err == nil {
		t.Fatal("Definition acquired work permission without a Profile source")
	}
}

func TestRuntimeDefinitionImplementationWorkEveryFieldIsSourceBound(t *testing.T) {
	_, focuses, profiles := definitionImplementationWorkFixture(t)
	base := RuntimeBootstrap{DefinitionFocuses: focuses, Activation: RuntimeActivationContext{ExecutionContract: RuntimeExecutionContract{ProfileContracts: profiles}}}
	policyType := reflect.TypeOf(RuntimeImplementationWorkPolicy{})
	for i := 0; i < policyType.NumField(); i++ {
		t.Run(policyType.Field(i).Name, func(t *testing.T) {
			changed := base.Clone()
			field := reflect.ValueOf(changed.DefinitionFocuses[0].ImplementationWorkPolicy).Elem().Field(i)
			switch field.Kind() {
			case reflect.String:
				field.SetString(field.String() + "-changed")
			case reflect.Int:
				field.SetInt(field.Int() + 1)
			case reflect.Bool:
				field.SetBool(!field.Bool())
			case reflect.Slice:
				field.SetLen(field.Len() - 1)
			default:
				t.Fatal("new policy field requires a mutation test")
			}
			if SameRuntimeBootstrap(base, changed) {
				t.Fatal("bootstrap equality ignored a changed CWI policy field")
			}
			if err := ValidateRuntimeDefinitionImplementationWorkPolicies(changed.DefinitionFocuses, profiles); err == nil {
				t.Fatal("changed policy was accepted against the original sealed Profile")
			}
			if err := ValidateRuntimeDefinitionImplementationWorkPolicies(base.DefinitionFocuses, profiles); err != nil {
				t.Fatalf("clone mutation affected source: %v", err)
			}
		})
	}
}

func TestRuntimeDefinitionImplementationWorkPolicySurvivesRawCloneAndReload(t *testing.T) {
	_, focuses, profiles := definitionImplementationWorkFixture(t)
	record := RuntimeStateRecord{Facts: RuntimeFacts{DefinitionFocuses: focuses,
		Activation: RuntimeActivationContext{ExecutionContract: RuntimeExecutionContract{ProfileContracts: profiles}}}}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded RuntimeStateRecord
	if err := json.Unmarshal(data, &reloaded); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRuntimeDefinitionImplementationWorkPolicies(reloaded.Facts.DefinitionFocuses, reloaded.Facts.Activation.ExecutionContract.ProfileContracts); err != nil {
		t.Fatal(err)
	}
	cloned := reloaded.Clone()
	cloned.Facts.DefinitionFocuses[0].ImplementationWorkPolicy.TargetSelectorCandidateRefs[0] = "different"
	if err := ValidateRuntimeDefinitionImplementationWorkPolicies(cloned.Facts.DefinitionFocuses, cloned.Facts.Activation.ExecutionContract.ProfileContracts); err == nil {
		t.Fatal("raw clone aliases the independently sealed Profile source")
	}
	if !reflect.DeepEqual(record, reloaded) {
		t.Fatal("raw clone mutation changed the reloaded source")
	}
	invalid := record.Clone()
	policy := invalid.Facts.DefinitionFocuses[0].ImplementationWorkPolicy
	policy.TargetSelectorCandidateRefs = []string{"duplicate", " duplicate ", " "}
	policy.AllowedTargetPolicies = []RuntimeOperationTargetPolicy{}
	cloned = invalid.Clone()
	if !reflect.DeepEqual(invalid, cloned) {
		t.Fatal("raw clone normalized invalid persisted policy")
	}
	data, err = json.Marshal(cloned)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &reloaded); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRuntimeDefinitionImplementationWorkPolicies(reloaded.Facts.DefinitionFocuses, profiles); err == nil {
		t.Fatal("invalid current-version data acquired authority after reload")
	}
}

func TestRuntimeDefinitionImplementationWorkResolverRejectsConflictsWithoutPrecedence(t *testing.T) {
	profiles := implementationWorkProfilesFixture()
	exact := CloneRuntimeProfileContracts(profiles)[0].EvidenceRequirements[0]
	exact.ID = "dod-1"
	profiles[0].EvidenceRequirements = append(profiles[0].EvidenceRequirements, exact)
	if _, err := ResolveRuntimeDefinitionImplementationWorkPolicy(profiles, "dodItem", "dod-1"); err != nil {
		t.Fatal(err)
	}
	// No optional exact declaration can erase the wildcard's boundary.
	profiles[0].EvidenceRequirements[1].ImplementationWorkPolicy = nil
	if p, err := ResolveRuntimeDefinitionImplementationWorkPolicy(profiles, "dodItem", "dod-1"); err != nil || p == nil {
		t.Fatalf("absent exact declaration erased wildcard: %v", err)
	}
	exact.ImplementationWorkPolicy.MaxRoundsPerCWI++
	profiles[0].EvidenceRequirements[1] = exact
	if _, err := ResolveRuntimeDefinitionImplementationWorkPolicy(profiles, "dodItem", "dod-1"); err == nil {
		t.Fatal("exact declaration silently overrode wildcard")
	}
	if p, err := ResolveRuntimeDefinitionImplementationWorkPolicy(profiles, "goalCoverage", "dod-1"); err != nil || p != nil {
		t.Fatalf("policy crossed source scope: %v", err)
	}
	profiles[0].EvidenceRequirements = []RuntimeProfileEvidenceRequirement{exact}
	if p, err := ResolveRuntimeDefinitionImplementationWorkPolicy(profiles, "dodItem", "dod-2"); err != nil || p != nil {
		t.Fatalf("exact policy crossed CWI: %v", err)
	}
}

func TestRuntimeSemanticManifestRejectsInjectedOrUnavailableImplementationWorkBeforeOwnerFacts(t *testing.T) {
	seed, focuses, profiles := definitionImplementationWorkFixture(t)
	definition := RuntimeDefinitionContext{StartupInputHash: "startup-input", Revision: seed.DefinitionRevision,
		GoalID: "goal-1", GoalDefinition: "original goal", DoDID: "dod-1", DoDDefinition: "original criterion",
		CapabilityContract: []string{StepKindFileWrite, RuntimeBlockKindManagedValidationSession}}
	execution := RuntimeExecutionContract{OperationContractRevision: RuntimeOperationContractRevision(), StateChangePolicy: RuntimeStateChangeAllowedButNotRequired}
	if _, err := CompileRuntimeSemanticPlanManifest("ws-1", "run-1", definition, seed, focuses, execution, nil); err == nil ||
		!strings.Contains(err.Error(), "implementation work differs from its exact sealed Profile source") {
		t.Fatalf("semantic compiler did not reject source injection before owner catalog acquisition: %v", err)
	}
	execution.ProfileContracts = profiles
	if _, err := CompileRuntimeSemanticPlanManifest("ws-1", "run-1", definition, seed, focuses, execution, nil); err == nil || !strings.Contains(err.Error(), "has no sealed admissibility catalog") {
		t.Fatalf("matching declaration invented owner facts: %v", err)
	}
	if err := execution.ValidateExecutableAdmission(); !errors.Is(err, ErrRuntimeImplementationWorkUnavailable) {
		t.Fatalf("matching declaration bypassed executable ingress readiness: %v", err)
	}
}
