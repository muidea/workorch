package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

// Original path: each public check compilation fully revalidates the inventory.
func referenceSourceObligationSeedRechecksInventory(seed RuntimeDefinitionSeed) error {
	if seed.SourceObligations == nil {
		return fmt.Errorf("runtime source obligation seed requires inventory")
	}
	inventory := seed.SourceObligations.Clone()
	if err := inventory.Validate(); err != nil {
		return err
	}
	if err := validateRuntimeGoalExecutionProjection(inventory, seed.CoverageItems); err != nil {
		return err
	}
	if inventory.DefinitionRevision != seed.DefinitionRevision {
		return fmt.Errorf("runtime source obligation seed definition revision mismatch")
	}
	if err := ValidateRuntimeEvidencePurposeMappingCoverage(inventory, seed.CoverageItems); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, item := range seed.CoverageItems {
		checks, err := CompileRuntimeSourceObligationChecks(inventory, item)
		if err != nil {
			return err
		}
		if !sameRuntimeSourceObligationChecks(checks, item.SourceObligationChecks) {
			return fmt.Errorf("runtime coverage %q source obligation checks do not match sealed inventory", item.ID)
		}
		if item.SourceScope == "dodItem" {
			if seen[item.SourceID] {
				return fmt.Errorf("runtime source obligation seed duplicates DoD coverage %q", item.SourceID)
			}
			seen[item.SourceID] = true
		}
	}
	for _, source := range inventory.Sources {
		if source.SourceScope == "dodItem" && !seen[source.SourceID] {
			return fmt.Errorf("runtime source obligation seed omits DoD coverage %q", source.SourceID)
		}
	}
	return nil
}

func sourceObligationReuseSeed(t testing.TB) RuntimeDefinitionSeed {
	t.Helper()
	inventory := sourceObligationInventoryFixture()
	for n := 2; n <= 19; n++ {
		ref, id := fmt.Sprintf("dod-%03d-ref", n), fmt.Sprintf("dod-%03d", n)
		inventory.Sources = append(inventory.Sources, RuntimeDefinitionSource{CandidateItemRef: ref, ItemRef: ref + "-formal", SourceScope: "dodItem", SourceID: id, Text: "完整原判据"})
		inventory.Requirements[0].SourceItemRefs = append(inventory.Requirements[0].SourceItemRefs, ref)
		obligation := fmt.Sprintf("obligation-%03d", n)
		inventory.Obligations = append(inventory.Obligations, RuntimeSourceObligation{ID: obligation, RequirementID: "requirement", DoDItemRefs: []string{ref}, Text: "完整原判据"})
		inventory.Verifications = append(inventory.Verifications, RuntimeSourceVerification{ID: fmt.Sprintf("verification-%03d", n), ObligationID: obligation, Declaration: "原独立验证声明", ProfileID: "profile", EvidenceRequirementID: id, Scope: "dodItem"})
	}
	inventory = inventory.WithHash()
	seed := RuntimeDefinitionSeed{DefinitionRevision: inventory.DefinitionRevision, SourceObligations: &inventory}
	for _, source := range inventory.Sources {
		if source.SourceScope != "dodItem" {
			continue
		}
		item := RuntimeCoverageItem{ID: common.CoverageWorkItemID("cwi-" + source.SourceID), SourceScope: source.SourceScope, SourceID: source.SourceID, TargetText: source.Text}
		var err error
		item.SourceObligationChecks, err = CompileRuntimeSourceObligationChecks(inventory, item)
		if err != nil {
			t.Fatal(err)
		}
		seed.CoverageItems = append(seed.CoverageItems, item)
	}
	if err := referenceSourceObligationSeedRechecksInventory(seed); err != nil {
		t.Fatal(err)
	}
	return seed
}

func assertSourceObligationSeedReference(t testing.TB, seed RuntimeDefinitionSeed) {
	t.Helper()
	before, err := json.Marshal(seed)
	if err != nil {
		t.Fatal(err)
	}
	got, want := ValidateRuntimeSourceObligationSeed(seed), referenceSourceObligationSeedRechecksInventory(seed)
	if (got == nil) != (want == nil) || got != nil && got.Error() != want.Error() {
		t.Fatalf("rejection differs: %v / %v", got, want)
	}
	after, err := json.Marshal(seed)
	if err != nil || string(before) != string(after) {
		t.Fatal("validation modified original source graph")
	}
}

func TestSourceObligationSeedSameInvocationReusePreservesAllGates(t *testing.T) {
	for _, scenario := range []string{"valid", "missing inventory", "hash", "raw criterion", "source order", "nested ref order", "duplicate source", "missing B", "wrong checks", "duplicate CWI", "missing CWI", "definition", "mixed failures"} {
		t.Run(scenario, func(t *testing.T) {
			seed := sourceObligationReuseSeed(t)
			switch scenario {
			case "missing inventory":
				seed.SourceObligations = nil
			case "hash":
				seed.SourceObligations.Hash = "changed"
			case "raw criterion":
				seed.SourceObligations.Obligations[0].Text += "\x00"
			case "source order":
				seed.SourceObligations.Sources[0], seed.SourceObligations.Sources[1] = seed.SourceObligations.Sources[1], seed.SourceObligations.Sources[0]
			case "nested ref order":
				refs := seed.SourceObligations.Requirements[0].SourceItemRefs
				refs[0], refs[1] = refs[1], refs[0]
			case "duplicate source":
				seed.SourceObligations.Sources = append(seed.SourceObligations.Sources, seed.SourceObligations.Sources[0])
			case "missing B":
				seed.SourceObligations.Verifications = seed.SourceObligations.Verifications[1:]
				*seed.SourceObligations = seed.SourceObligations.WithHash()
			case "wrong checks":
				seed.CoverageItems[0].SourceObligationChecks[0].Criterion = "replacement"
			case "duplicate CWI":
				seed.CoverageItems = append(seed.CoverageItems, seed.CoverageItems[0])
			case "missing CWI":
				seed.CoverageItems = seed.CoverageItems[1:]
			case "definition":
				seed.DefinitionRevision = "different"
			case "mixed failures":
				seed.DefinitionRevision = "different"
				seed.SourceObligations.Hash = "changed"
				seed.CoverageItems[0].TargetText = "different"
			}
			assertSourceObligationSeedReference(t, seed)
			body, err := json.Marshal(seed)
			if err != nil {
				t.Fatal(err)
			}
			var reload RuntimeDefinitionSeed
			if err := json.Unmarshal(body, &reload); err != nil {
				t.Fatal(err)
			}
			assertSourceObligationSeedReference(t, reload)
		})
	}
	seed := sourceObligationReuseSeed(t)
	if err := ValidateRuntimeSourceObligationSeed(seed); err != nil {
		t.Fatal(err)
	}
	seed.SourceObligations.Obligations[0].Text = "later mutation"
	if ValidateRuntimeSourceObligationSeed(seed) == nil {
		t.Fatal("later call reused stale validation")
	}
	inventory := sourceObligationInventoryFixture()
	item := RuntimeCoverageItem{ID: "cwi", SourceScope: "dodItem", SourceID: "dod-001", TargetText: "共同判据"}
	checks, err := CompileRuntimeSourceObligationChecks(inventory, item)
	if err != nil {
		t.Fatal(err)
	}
	checks[0].Criterion = "mutated returned value"
	fresh, err := CompileRuntimeSourceObligationChecks(inventory, item)
	if err != nil || reflect.DeepEqual(checks, fresh) {
		t.Fatal("returned checks alias original inventory")
	}
	inventory.Hash = "caller-supplied hash"
	if _, err := CompileRuntimeSourceObligationChecks(inventory, item); err == nil {
		t.Fatal("public compiler skipped full inventory validation")
	}
}

func TestSourceObligationSeedReusePreservesMappedInventoryGates(t *testing.T) {
	for _, scenario := range []string{"valid", "hash", "mapping", "missing endpoint", "missing predecessor", "source declaration", "consumer checks", "mixed failures"} {
		t.Run(scenario, func(t *testing.T) {
			inventory, _, items := evidencePurposeMappingFixture()
			var err error
			items, err = CompileRuntimeEvidencePurposeMappingCoverage(inventory, items)
			if err != nil {
				t.Fatal(err)
			}
			for n := range items {
				items[n].SourceObligationChecks, err = CompileRuntimeSourceObligationChecks(inventory, items[n])
				if err != nil {
					t.Fatal(err)
				}
			}
			seed := RuntimeDefinitionSeed{DefinitionRevision: inventory.DefinitionRevision, SourceObligations: &inventory, CoverageItems: items}
			if err := ValidateRuntimeSourceObligationSeed(seed); err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "hash":
				inventory.Hash = "changed"
			case "mapping":
				inventory.EvidencePurposeMappings[0].AcceptanceItemRef = "unknown"
				inventory = inventory.WithHash()
			case "missing endpoint":
				seed.CoverageItems = items[:1]
			case "missing predecessor":
				items[1].PrerequisiteCoverageWorkItemIDs = nil
			case "source declaration":
				items[0].HistoricalAcceptance = nil
			case "consumer checks":
				items[1].SourceObligationChecks = items[1].SourceObligationChecks[1:]
			case "mixed failures":
				inventory.Hash, seed.DefinitionRevision = "changed", "changed"
				items[0].HistoricalAcceptance = nil
			}
			if scenario != "valid" && ValidateRuntimeSourceObligationSeed(seed) == nil {
				t.Fatal("mapped seed gate bypassed")
			}
			assertSourceObligationSeedReference(t, seed)
			body, err := json.Marshal(seed)
			if err != nil {
				t.Fatal(err)
			}
			var reload RuntimeDefinitionSeed
			if err := json.Unmarshal(body, &reload); err != nil {
				t.Fatal(err)
			}
			assertSourceObligationSeedReference(t, reload)
		})
	}
	// Empty mapping retains its original narrow contract. The seed/public
	// compiler still reject this malformed inventory at their own boundaries.
	malformed := RuntimeSourceObligationInventory{}
	if err := ValidateRuntimeEvidencePurposeMappingCoverage(malformed, nil); err != nil {
		t.Fatalf("empty-mapping rejection order changed: %v", err)
	}
	if _, err := CompileRuntimeEvidencePurposeMappingCoverage(malformed, nil); err == nil {
		t.Fatal("public mapping compiler trusted an unvalidated inventory")
	}
	malformed.EvidencePurposeMappings = []RuntimeEvidencePurposeMapping{{}}
	if ValidateRuntimeEvidencePurposeMappingCoverage(malformed, nil) == nil {
		t.Fatal("public nonempty-mapping validator skipped inventory validation")
	}
}

func BenchmarkSourceObligationSeedSameInvocationReuse(b *testing.B) {
	seed := sourceObligationReuseSeed(b)
	for _, mode := range []string{"reference", "same-invocation"} {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			for n := 0; n < b.N; n++ {
				var err error
				if mode == "reference" {
					err = referenceSourceObligationSeedRechecksInventory(seed)
				} else {
					err = ValidateRuntimeSourceObligationSeed(seed)
				}
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
