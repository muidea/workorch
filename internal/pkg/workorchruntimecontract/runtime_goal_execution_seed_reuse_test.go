package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"testing"
)

// Original seed gate: every item's public compiler independently revalidates
// the complete mapping. Keep its error order as the invocation-local oracle.
func referenceGoalExecutionSeedGate(seed RuntimeDefinitionSeed, profiles []RuntimeProfileContract) error {
	for _, item := range seed.CoverageItems {
		expected, err := CompileRuntimeGoalExecutionAssertions(seed.SourceObligations, item, profiles)
		if err != nil {
			return err
		}
		if !slices.Equal(expected, item.GoalExecutionAssertions) {
			return fmt.Errorf("Goal execution assertions differ from confirmed source and Profile")
		}
	}
	return nil
}

func goalExecutionReuseSeed(t testing.TB) (RuntimeDefinitionSeed, []RuntimeProfileContract) {
	t.Helper()
	i, p, item := goalExecutionFixture()
	var err error
	item.GoalExecutionAssertions, err = CompileRuntimeGoalExecutionAssertions(&i, item, p)
	if err != nil {
		t.Fatal(err)
	}
	seed := RuntimeDefinitionSeed{DefinitionRevision: i.DefinitionRevision, SourceObligations: &i, CoverageItems: []RuntimeCoverageItem{item}}
	for n := 1; n < 19; n++ {
		seed.CoverageItems = append(seed.CoverageItems, RuntimeCoverageItem{ID: "other", SourceScope: "deliverable", SourceID: fmt.Sprintf("other-%d", n), TargetText: "unchanged ordinary obligation"})
	}
	return seed, p
}

func assertGoalSeedReference(t testing.TB, seed RuntimeDefinitionSeed, profiles []RuntimeProfileContract) {
	t.Helper()
	before, _ := json.Marshal([]any{seed, profiles})
	got, want := ValidateRuntimeGoalExecutionAssertionSeed(seed, profiles), referenceGoalExecutionSeedGate(seed, profiles)
	if (got == nil) != (want == nil) || got != nil && got.Error() != want.Error() {
		t.Fatalf("original Goal seed gate changed: %v / %v", got, want)
	}
	after, _ := json.Marshal([]any{seed, profiles})
	if string(before) != string(after) {
		t.Fatal("gate changed original input")
	}
}

func TestGoalExecutionSeedReusesOnlySameInvocationCompleteMappingGate(t *testing.T) {
	for _, scenario := range []string{"original", "reload", "nil-inventory", "empty-items", "bad-empty", "missing-map", "source-drift", "rule-drift", "strip", "extra", "text-drift", "mixed-error"} {
		t.Run(scenario, func(t *testing.T) {
			seed, profiles := goalExecutionReuseSeed(t)
			switch scenario {
			case "reload":
				body, _ := json.Marshal(seed)
				if json.Unmarshal(body, &seed) != nil {
					t.Fatal("reload failed")
				}
			case "nil-inventory":
				seed.SourceObligations = nil
			case "empty-items", "bad-empty":
				seed.CoverageItems = nil
				if scenario == "bad-empty" {
					seed.SourceObligations.ExecutionProofMappings[0].SourceItemRef = "unknown"
				}
			case "missing-map":
				seed.SourceObligations.ExecutionProofMappings = nil
			case "source-drift":
				seed.SourceObligations.ExecutionProofMappings[0].ConsumerGoalText += "drift"
			case "rule-drift":
				profiles[0].EvidenceRequirements[0].LineageAssertions[0].MinCompletedRounds++
			case "strip":
				seed.CoverageItems[0].GoalExecutionAssertions = nil
			case "extra":
				seed.CoverageItems[18].GoalExecutionAssertions = seed.CoverageItems[0].GoalExecutionAssertions
			case "text-drift":
				seed.CoverageItems[0].TargetText += "drift"
			case "mixed-error":
				seed.CoverageItems[0].TargetText += "drift"
				seed.SourceObligations.ExecutionProofMappings[0].SourceItemRef = "unknown"
			}
			assertGoalSeedReference(t, seed, profiles)
		})
	}
	seed, profiles := goalExecutionReuseSeed(t)
	if ValidateRuntimeGoalExecutionAssertionSeed(seed, profiles) != nil {
		t.Fatal("valid first call rejected")
	}
	seed.SourceObligations.ExecutionProofMappings[0].SourceItemRef = "later-mutation"
	assertGoalSeedReference(t, seed, profiles)
	if ValidateRuntimeGoalExecutionAssertionSeed(seed, profiles) == nil {
		t.Fatal("previous successful invocation authorized later mutation")
	}
	i, p, item := goalExecutionFixture()
	first, _ := CompileRuntimeGoalExecutionAssertions(&i, item, p)
	second, _ := CompileRuntimeGoalExecutionAssertions(&i, item, p)
	first[0].Rule.MinCompletedRounds++
	if reflect.DeepEqual(first, second) || p[0].EvidenceRequirements[0].LineageAssertions[0].MinCompletedRounds != 2 {
		t.Fatal("public result was not detached")
	}
}

func FuzzGoalExecutionSeedMatchesOriginalGate(f *testing.F) {
	f.Add(uint8(0), "original")
	f.Add(uint8(1), "unknown")
	f.Add(uint8(2), "\x00raw\xff")
	f.Fuzz(func(t *testing.T, mode uint8, value string) {
		if len(value) > 4097 {
			t.Skip()
		}
		seed, profiles := goalExecutionReuseSeed(t)
		switch mode % 5 {
		case 0:
			seed.CoverageItems[0].TargetText = value
		case 1:
			seed.SourceObligations.ExecutionProofMappings[0].SourceItemRef = value
		case 2:
			profiles[0].EvidenceRequirements[0].LineageAssertions[0].Ref = value
		case 3:
			seed.SourceObligations.ExecutionProofMappings[0].ConsumerGoalText = value
		case 4:
			seed.CoverageItems[18].SourceID = value
		}
		assertGoalSeedReference(t, seed, profiles)
	})
}

func BenchmarkGoalExecutionSeedMappingGate(b *testing.B) {
	seed, profiles := goalExecutionReuseSeed(b)
	for _, original := range []bool{true, false} {
		b.Run(fmt.Sprintf("original=%t", original), func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				var err error
				if original {
					err = referenceGoalExecutionSeedGate(seed, profiles)
				} else {
					err = ValidateRuntimeGoalExecutionAssertionSeed(seed, profiles)
				}
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
