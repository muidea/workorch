package workorchruntimecontract

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func implementationWorkPolicyFixture() RuntimeImplementationWorkPolicy {
	return RuntimeImplementationWorkPolicy{
		ProtocolRef: RuntimeImplementationWorkProtocolV1, OwnerModuleID: RuntimeModuleFile,
		AllowedOperations:     []RuntimeOperationVariantRef{{Kind: StepKindFileWrite}, {Kind: RuntimeBlockKindPatchApply, VariantID: "text_replace"}},
		AllowedTargetPolicies: []RuntimeOperationTargetPolicy{RuntimeOperationTargetPolicyExistingOwnerScope, RuntimeOperationTargetPolicyProspectiveOwnerTarget},
		TargetSelectorRef:     "work-scope", TargetSelectorOwnerRef: "profile/example", TargetSelectorSubjectRef: "implementation",
		TargetSelectorCandidateRefs:          []string{"resource-a", "resource-b"},
		ProspectiveTargetAllOfConstraintRefs: []string{"constraint-a", "constraint-b"},
		MaxRoundsPerCWI:                      2, MaxTargetsPerRound: 2,
		RequireFreshObservation: true, RequireFreshValidation: true, ProtectValidationInputs: true,
	}
}

func implementationWorkProfilesFixture() []RuntimeProfileContract {
	policy := implementationWorkPolicyFixture()
	return []RuntimeProfileContract{{ID: "example", EvidenceRequirements: []RuntimeProfileEvidenceRequirement{{
		ID: "*", Scope: "dodItem", RequiredEvidenceKinds: []string{EvidenceKindValidation},
		AcceptedStepKinds: []string{RuntimeBlockKindManagedValidationSession}, ImplementationWorkPolicy: &policy,
	}}}}
}

func TestRuntimeImplementationWorkPolicyRejectsUnboundedOrExpandedAuthority(t *testing.T) {
	base := implementationWorkPolicyFixture()
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*RuntimeImplementationWorkPolicy){
		"unknown protocol":  func(p *RuntimeImplementationWorkPolicy) { p.ProtocolRef = "next" },
		"cross owner":       func(p *RuntimeImplementationWorkPolicy) { p.OwnerModuleID = RuntimeModuleShell },
		"unknown operation": func(p *RuntimeImplementationWorkPolicy) { p.AllowedOperations[0].Kind = "unknown" },
		"read is not work": func(p *RuntimeImplementationWorkPolicy) {
			p.AllowedOperations = []RuntimeOperationVariantRef{{Kind: StepKindFileRead}}
		},
		"unscoped effect": func(p *RuntimeImplementationWorkPolicy) {
			p.OwnerModuleID = RuntimeModuleShell
			p.AllowedOperations = []RuntimeOperationVariantRef{{Kind: StepKindShellCommand}}
		},
		"unknown variant": func(p *RuntimeImplementationWorkPolicy) { p.AllowedOperations[0].VariantID = "invented" },
		"duplicate operation": func(p *RuntimeImplementationWorkPolicy) {
			p.AllowedOperations = append(p.AllowedOperations, p.AllowedOperations[0])
		},
		"missing operations":  func(p *RuntimeImplementationWorkPolicy) { p.AllowedOperations = nil },
		"oversize operations": func(p *RuntimeImplementationWorkPolicy) { p.AllowedOperations = make([]RuntimeOperationVariantRef, 9) },
		"unknown policy":      func(p *RuntimeImplementationWorkPolicy) { p.AllowedTargetPolicies[0] = "unrestricted" },
		"duplicate policy": func(p *RuntimeImplementationWorkPolicy) {
			p.AllowedTargetPolicies = append(p.AllowedTargetPolicies, p.AllowedTargetPolicies[0])
		},
		"missing policies":  func(p *RuntimeImplementationWorkPolicy) { p.AllowedTargetPolicies = nil },
		"missing selector":  func(p *RuntimeImplementationWorkPolicy) { p.TargetSelectorRef = "" },
		"wildcard owner":    func(p *RuntimeImplementationWorkPolicy) { p.TargetSelectorOwnerRef = "*" },
		"invalid subject":   func(p *RuntimeImplementationWorkPolicy) { p.TargetSelectorSubjectRef = "bad\nsubject" },
		"oversize subject":  func(p *RuntimeImplementationWorkPolicy) { p.TargetSelectorSubjectRef = strings.Repeat("x", 513) },
		"bad utf8":          func(p *RuntimeImplementationWorkPolicy) { p.TargetSelectorSubjectRef = string([]byte{255}) },
		"zero rounds":       func(p *RuntimeImplementationWorkPolicy) { p.MaxRoundsPerCWI = 0 },
		"negative rounds":   func(p *RuntimeImplementationWorkPolicy) { p.MaxRoundsPerCWI = -1 },
		"oversize rounds":   func(p *RuntimeImplementationWorkPolicy) { p.MaxRoundsPerCWI = 9 },
		"zero targets":      func(p *RuntimeImplementationWorkPolicy) { p.MaxTargetsPerRound = 0 },
		"oversize targets":  func(p *RuntimeImplementationWorkPolicy) { p.MaxTargetsPerRound = 9 },
		"no observation":    func(p *RuntimeImplementationWorkPolicy) { p.RequireFreshObservation = false },
		"no validation":     func(p *RuntimeImplementationWorkPolicy) { p.RequireFreshValidation = false },
		"weaken old inputs": func(p *RuntimeImplementationWorkPolicy) { p.ProtectValidationInputs = false },
		"unbounded creation": func(p *RuntimeImplementationWorkPolicy) {
			p.TargetSelectorCandidateRefs = nil
			p.ProspectiveTargetAllOfConstraintRefs = nil
		},
		"duplicate candidates": func(p *RuntimeImplementationWorkPolicy) { p.TargetSelectorCandidateRefs = []string{"same", " same "} },
		"blank candidate":      func(p *RuntimeImplementationWorkPolicy) { p.TargetSelectorCandidateRefs = []string{" "} },
		"wildcard candidate":   func(p *RuntimeImplementationWorkPolicy) { p.TargetSelectorCandidateRefs = []string{"*"} },
		"oversize candidates":  func(p *RuntimeImplementationWorkPolicy) { p.TargetSelectorCandidateRefs = make([]string, 9) },
		"duplicate constraints": func(p *RuntimeImplementationWorkPolicy) {
			p.ProspectiveTargetAllOfConstraintRefs = []string{"same", "same"}
		},
		"unused constraints": func(p *RuntimeImplementationWorkPolicy) {
			p.AllowedTargetPolicies = []RuntimeOperationTargetPolicy{RuntimeOperationTargetPolicyExactTarget}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			p := base.Clone()
			mutate(&p)
			if err := p.Validate(); err == nil {
				t.Fatal("invalid declaration accepted")
			}
			if err := p.Normalized().Validate(); err == nil {
				t.Fatal("normalization erased invalid declaration")
			}
		})
	}
}

func TestRuntimeImplementationWorkPolicyEveryFieldIsHashBound(t *testing.T) {
	base := implementationWorkProfilesFixture()
	policy := base[0].EvidenceRequirements[0].ImplementationWorkPolicy
	for i := 0; i < reflect.TypeOf(*policy).NumField(); i++ {
		field := reflect.TypeOf(*policy).Field(i)
		t.Run(field.Name, func(t *testing.T) {
			changed := CloneRuntimeProfileContracts(base)
			p := changed[0].EvidenceRequirements[0].ImplementationWorkPolicy
			v := reflect.ValueOf(p).Elem().Field(i)
			switch v.Kind() {
			case reflect.String:
				v.SetString(v.String() + "-changed")
			case reflect.Bool:
				v.SetBool(!v.Bool())
			case reflect.Int:
				v.SetInt(v.Int() + 1)
			case reflect.Slice:
				v.SetLen(v.Len() - 1)
			default:
				t.Fatalf("add a hash mutation for %s", field.Name)
			}
			if SameRuntimeProfileContracts(base, changed) || StableRuntimeProfileContractsHash(base) == StableRuntimeProfileContractsHash(changed) {
				t.Fatal("policy change lost from sealed profile identity")
			}
			if StableRuntimeImplementationWorkPolicyHash(*policy) == StableRuntimeImplementationWorkPolicyHash(*p) {
				t.Fatal("policy hash lost field")
			}
		})
	}
	if err := policy.Validate(); err != nil {
		t.Fatalf("clone mutated source: %v", err)
	}
	clone := CloneRuntimeProfileContracts(base)
	clone[0].EvidenceRequirements[0].ImplementationWorkPolicy.AllowedOperations[0].Kind = "changed"
	clone[0].EvidenceRequirements[0].ImplementationWorkPolicy.TargetSelectorCandidateRefs[0] = "changed"
	if err := policy.Validate(); err != nil {
		t.Fatalf("nested clone alias: %v", err)
	}
	if policy.TargetSelectorCandidateRefs[0] != "resource-a" {
		t.Fatal("target clone alias")
	}
}

func TestRuntimeImplementationWorkPolicyCanonicalAndRawRoundTrips(t *testing.T) {
	base := implementationWorkPolicyFixture()
	reordered := base.Clone()
	slices.Reverse(reordered.AllowedOperations)
	slices.Reverse(reordered.AllowedTargetPolicies)
	slices.Reverse(reordered.TargetSelectorCandidateRefs)
	slices.Reverse(reordered.ProspectiveTargetAllOfConstraintRefs)
	if !SameRuntimeImplementationWorkPolicy(&base, &reordered) || StableRuntimeImplementationWorkPolicyHash(base) != StableRuntimeImplementationWorkPolicyHash(reordered) {
		t.Fatal("ordering changes policy identity")
	}
	profiles := implementationWorkProfilesFixture()
	profiles[0].EvidenceRequirements[0].ImplementationWorkPolicy.TargetSelectorCandidateRefs = []string{" x ", "x", " "}
	clone := CloneRuntimeProfileContracts(profiles)
	if !reflect.DeepEqual(clone, profiles) {
		t.Fatal("raw clone normalized invalid policy")
	}
	wire, err := json.Marshal(profiles)
	if err != nil {
		t.Fatal(err)
	}
	var restored []RuntimeProfileContract
	if err := json.Unmarshal(wire, &restored); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored, profiles) {
		t.Fatal("current JSON round trip lost declaration")
	}
	if err := ValidateRuntimeProfileContracts(restored); err == nil {
		t.Fatal("invalid stored declaration was repaired by load")
	}
}

func TestRuntimeImplementationWorkPolicyCannotEnableAnIncompleteProtocol(t *testing.T) {
	profiles := implementationWorkProfilesFixture()
	if err := ValidateRuntimeProfileContracts(profiles); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRuntimeImplementationWorkAvailability(profiles); !errors.Is(err, ErrRuntimeImplementationWorkUnavailable) {
		t.Fatalf("availability = %v", err)
	}
	report := ExplainRuntimeImplementationWorkAvailability(profiles)
	if report.Available || len(report.Blockers) != 1 || report.Blockers[0].ProfileID != profiles[0].ID {
		t.Fatalf("unexpected readiness report: %#v", report)
	}
	if report.Blockers[0].Reason != ErrRuntimeImplementationWorkUnavailable.Error() {
		t.Fatalf("unexpected valid-policy blocker: %#v", report.Blockers[0])
	}
	invalid := CloneRuntimeProfileContracts(profiles)
	invalid[0].EvidenceRequirements[0].ImplementationWorkPolicy.MaxRoundsPerCWI = 0
	invalidReport := ExplainRuntimeImplementationWorkAvailability(invalid)
	if invalidReport.Available || len(invalidReport.Blockers) != 1 || !strings.Contains(invalidReport.Blockers[0].Reason, "invalid implementation work declaration") {
		t.Fatalf("invalid declaration was not diagnosed: %#v", invalidReport)
	}
	execution := RuntimeExecutionContract{StateChangePolicy: RuntimeStateChangeAllowedButNotRequired, ProfileContracts: profiles}
	if err := execution.Validate(); !errors.Is(err, ErrRuntimeImplementationWorkUnavailable) {
		t.Fatalf("execution bypassed readiness: %v", err)
	}
	profiles[0].EvidenceRequirements[0].ImplementationWorkPolicy = nil
	if report := ExplainRuntimeImplementationWorkAvailability(profiles); !report.Available || len(report.Blockers) != 0 {
		t.Fatalf("cleared policy remains blocked: %#v", report)
	}
	if err := ValidateRuntimeImplementationWorkAvailability(profiles); err != nil {
		t.Fatal(err)
	}
	if err := (RuntimeExecutionContract{StateChangePolicy: RuntimeStateChangeAllowedButNotRequired, ProfileContracts: profiles}).Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeImplementationWorkPolicyCannotWeakenTerminalProofOrConflict(t *testing.T) {
	profiles := implementationWorkProfilesFixture()
	profiles[0].EvidenceRequirements[0].RequiredEvidenceKinds = []string{EvidenceKindStateChange}
	if err := ValidateRuntimeProfileContracts(profiles); err == nil {
		t.Fatal("intermediate work became terminal proof")
	}
	profiles = implementationWorkProfilesFixture()
	exact := CloneRuntimeProfileContracts(profiles)[0].EvidenceRequirements[0]
	exact.ID = "exact-cwi"
	profiles[0].EvidenceRequirements = append(profiles[0].EvidenceRequirements, exact)
	if err := ValidateRuntimeProfileContracts(profiles); err != nil {
		t.Fatal(err)
	}
	exact.ImplementationWorkPolicy.MaxRoundsPerCWI++
	if err := ValidateRuntimeProfileContracts(profiles); err == nil {
		t.Fatal("exact policy silently overrides wildcard boundary")
	}
}
