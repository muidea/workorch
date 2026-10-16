package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// Original compiler: normalize every profile and every unrelated declaration
// before selecting one source. Keep its merge and error order as an oracle.
func referenceSourcePolicyFullProfiles(scope, sourceID string, profiles []RuntimeProfileContract, additional []RuntimeProfileEvidenceRequirement) (RuntimeSourceEvidencePolicy, error) {
	scope, sourceID = strings.TrimSpace(scope), strings.TrimSpace(sourceID)
	if scope == "" || sourceID == "" {
		return RuntimeSourceEvidencePolicy{}, fmt.Errorf("source evidence policy requires exact scope and source identity")
	}
	candidates := make([]RuntimeProfileEvidenceRequirement, 0)
	for _, profile := range NormalizeRuntimeProfileContracts(profiles) {
		for _, requirement := range profile.EvidenceRequirements {
			if !RuntimeEvidenceDeclarationMatchesSource(requirement, scope, sourceID) {
				continue
			}
			requirement.TargetSelectorRef, requirement.TargetSelectorOwnerRef, requirement.TargetSelectorSubjectRef = ResolveRuntimeProfileTargetSelectorIdentity(profile.ID, requirement)
			candidates = append(candidates, requirement)
		}
	}
	for _, requirement := range additional {
		if RuntimeEvidenceDeclarationMatchesSource(requirement, scope, sourceID) {
			candidates = append(candidates, requirement)
		}
	}
	for _, requirement := range candidates {
		if err := validateRuntimeSourceAcceptedKinds(requirement); err != nil {
			return RuntimeSourceEvidencePolicy{}, err
		}
	}
	policy, err := mergeRuntimeSourceEvidencePolicy(RuntimeSourceEvidencePolicy{RequireAssessment: true}, runtimeSourceFilterRequirementsByID(candidates, sourceID))
	if err != nil {
		return RuntimeSourceEvidencePolicy{}, err
	}
	if sourceID != "*" {
		policy, err = mergeRuntimeSourceEvidencePolicy(policy, runtimeSourceWildcardQualityRequirements(runtimeSourceFilterRequirementsByID(candidates, "*")))
		if err != nil {
			return RuntimeSourceEvidencePolicy{}, err
		}
	}
	if len(policy.RequiredEvidenceKinds) == 0 {
		switch scope {
		case "dodItem", "goalCoverage", "acceptanceDimension":
			policy.RequiredEvidenceKinds = []string{EvidenceKindValidation}
		}
	}
	if err := ValidateRuntimeOwnerFactConvergences(policy.RequiredOwnerFacts, policy.OwnerFactConvergences); err != nil {
		return RuntimeSourceEvidencePolicy{}, err
	}
	policy.SemanticProofDeclarationRef = CanonicalRuntimeDefinitionEvidenceDeclarationRef(scope, sourceID)
	return policy, nil
}

func sourcePolicyProjectionFixture() []RuntimeProfileContract {
	_, profiles, _ := goalExecutionFixture()
	for n := 0; n < 19; n++ {
		profiles[0].EvidenceRequirements = append(profiles[0].EvidenceRequirements, RuntimeProfileEvidenceRequirement{
			ID: fmt.Sprintf("unrelated-%03d", n), Scope: "dodItem", Description: "complete original text",
			RequiredEvidenceKinds: []string{EvidenceKindValidation}, AcceptedStepKinds: []string{RuntimeBlockKindManagedValidationSession},
			RequiredCheckRefs: []string{"check-b", "check-a"}, MetricConstraints: []string{"metric-b", "metric-a"},
			TargetRefs: []string{"original-target"}, ArtifactRefs: []string{"original-artifact"},
			LineageAssertions: profiles[0].EvidenceRequirements[0].LineageAssertions,
		})
	}
	profiles[0].EvidenceRequirements[1].TargetSelectorRef = " exact-selector "
	profiles[0].AcceptanceDimensions = []RuntimeAcceptanceDimension{{ID: "unrelated", Description: "complete bytes"}}
	return profiles
}

func assertSourcePolicyProjectionReference(t testing.TB, scope, id string, profiles []RuntimeProfileContract, additional []RuntimeProfileEvidenceRequirement) {
	t.Helper()
	before, _ := json.Marshal([]any{profiles, additional})
	got, err := CompileRuntimeSourceEvidencePolicy(scope, id, profiles, additional)
	want, expected := referenceSourcePolicyFullProfiles(scope, id, profiles, additional)
	if (err == nil) != (expected == nil) || err != nil && err.Error() != expected.Error() || !reflect.DeepEqual(got, want) {
		t.Fatalf("original source compiler differs: %v / %v", err, expected)
	}
	after, _ := json.Marshal([]any{profiles, additional})
	if string(before) != string(after) {
		t.Fatal("projection changed raw declaration bytes")
	}
}

func TestSourcePolicyProjectionKeepsOriginalMergeOrderAndCompleteMatchingFields(t *testing.T) {
	for _, scenario := range []string{"original", "reload", "wildcard", "duplicate-profile", "duplicate-requirement", "whitespace", "matching-invalid-kind", "unrelated-invalid", "conflict", "additional", "nil", "empty"} {
		t.Run(scenario, func(t *testing.T) {
			profiles := sourcePolicyProjectionFixture()
			var additional []RuntimeProfileEvidenceRequirement
			scope, id := "dodItem", "dod-001"
			switch scenario {
			case "reload":
				body, _ := json.Marshal(profiles)
				if json.Unmarshal(body, &profiles) != nil {
					t.Fatal("reload failed")
				}
			case "wildcard":
				profiles[0].EvidenceRequirements[2].ID = "*"
			case "duplicate-profile":
				profiles = append(profiles, CloneRuntimeProfileContracts(profiles)[0])
			case "duplicate-requirement":
				profiles[0].EvidenceRequirements = append(profiles[0].EvidenceRequirements, profiles[0].EvidenceRequirements[1])
			case "whitespace":
				profiles[0].ID, scope, id = " profile ", " dodItem ", " dod-001 "
				profiles[0].EvidenceRequirements[1].ID, profiles[0].EvidenceRequirements[1].Scope = " dod-001 ", " dodItem "
			case "matching-invalid-kind":
				profiles[0].EvidenceRequirements[1].AcceptedStepKinds = []string{"unknown"}
			case "unrelated-invalid":
				profiles[0].EvidenceRequirements[2].AcceptedStepKinds = []string{"unknown\x00\xff"}
			case "conflict":
				r := profiles[0].EvidenceRequirements[1]
				r.LineageAssertions = profiles[0].EvidenceRequirements[0].LineageAssertions
				profiles[0].EvidenceRequirements = append(profiles[0].EvidenceRequirements, r)
				profiles[0].EvidenceRequirements[1].LineageAssertions = append([]RuntimeLineageAssertionRule(nil), r.LineageAssertions...)
				profiles[0].EvidenceRequirements[1].LineageAssertions[0].MinCompletedRounds++
			case "additional":
				additional = []RuntimeProfileEvidenceRequirement{{ID: " dod-001 ", Scope: " dodItem ", RequiredEvidenceKinds: []string{EvidenceKindStateChange}, AcceptedStepKinds: []string{StepKindFileWrite}, TargetRefs: []string{" raw target "}}}
			case "nil":
				profiles = nil
			case "empty":
				scope = " "
			}
			assertSourcePolicyProjectionReference(t, scope, id, profiles, additional)
		})
	}
	profiles := sourcePolicyProjectionFixture()
	first, _ := CompileRuntimeSourceEvidencePolicy("dodItem", "dod-001", profiles, nil)
	first.AcceptedStepKinds[0] = "caller"
	profiles[0].EvidenceRequirements[1].AcceptedStepKinds = []string{"later-invalid"}
	assertSourcePolicyProjectionReference(t, "dodItem", "dod-001", profiles, nil)
	if _, err := CompileRuntimeSourceEvidencePolicy("dodItem", "dod-001", profiles, nil); err == nil {
		t.Fatal("previous projection cached acceptance for later changed source")
	}
}

func FuzzSourcePolicyProjectionMatchesOriginalCompiler(f *testing.F) {
	f.Add(uint8(0), "dod-001")
	f.Add(uint8(1), " dodItem ")
	f.Add(uint8(2), "\x00raw\xff")
	f.Fuzz(func(t *testing.T, mode uint8, value string) {
		if len(value) > 4096 {
			t.Skip()
		}
		profiles := sourcePolicyProjectionFixture()
		scope, id := "dodItem", "dod-001"
		switch mode % 6 {
		case 0:
			id = value
		case 1:
			scope = value
		case 2:
			profiles[0].ID = value
		case 3:
			profiles[0].EvidenceRequirements[1].ID = value
		case 4:
			profiles[0].EvidenceRequirements[1].AcceptedStepKinds = []string{value}
		case 5:
			profiles[0].EvidenceRequirements[1].Description = value
		}
		assertSourcePolicyProjectionReference(t, scope, id, profiles, nil)
	})
}

func BenchmarkSourcePolicyProfileProjection(b *testing.B) {
	profiles := sourcePolicyProjectionFixture()
	for _, original := range []bool{true, false} {
		b.Run(fmt.Sprintf("original=%t", original), func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				var err error
				if original {
					_, err = referenceSourcePolicyFullProfiles("dodItem", "dod-001", profiles, nil)
				} else {
					_, err = CompileRuntimeSourceEvidencePolicy("dodItem", "dod-001", profiles, nil)
				}
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
