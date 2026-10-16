package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"testing"
)

// Keep the former per-outcome public-compiler path as an independent rejection
// oracle. Neither path decides coverage, evidence currentness or owner credit.
func referenceCloseoutSourceObligationsRechecksInventory(closeout RuntimeResultCloseout) error {
	if closeout.SourceObligations == nil {
		for _, outcome := range closeout.RequirementOutcomes {
			if len(outcome.SourceObligations) != 0 {
				return fmt.Errorf("runtime closeout source outcomes require sealed inventory")
			}
		}
		return nil
	}
	inventory := *closeout.SourceObligations
	if err := inventory.Validate(); err != nil {
		return err
	}
	if inventory.DefinitionRevision != closeout.DefinitionRevision || inventory.StartupInputHash != closeout.StartupInputHash {
		return fmt.Errorf("runtime closeout source inventory startup identity mismatch")
	}
	seen := map[string]bool{}
	for _, outcome := range closeout.RequirementOutcomes {
		item := RuntimeCoverageItem{ID: outcome.CoverageWorkItemID}
		for _, source := range RuntimeEvidencePurposeSources(inventory) {
			if outcome.RequirementRef == source.SourceScope+"/"+source.SourceID {
				item.SourceScope, item.SourceID, item.TargetText = source.SourceScope, source.SourceID, source.Text
			}
		}
		item = item.Normalized()
		checks, err := CompileRuntimeSourceObligationChecks(inventory, item)
		if err != nil {
			return err
		}
		actual := make([]RuntimeSourceObligationCheck, 0, len(outcome.SourceObligations))
		for _, source := range outcome.SourceObligations {
			actual = append(actual, source.Check)
		}
		if !sameRuntimeSourceObligationChecks(checks, actual) {
			return fmt.Errorf("runtime closeout coverage %q source outcomes omit or alter sealed obligation", item.ID)
		}
		if item.SourceScope == "dodItem" {
			if seen[item.SourceID] {
				return fmt.Errorf("runtime closeout duplicates source coverage")
			}
			seen[item.SourceID] = true
		}
	}
	for _, source := range inventory.Sources {
		if source.SourceScope == "dodItem" && !seen[source.SourceID] {
			return fmt.Errorf("runtime closeout omits sealed DoD source %q", source.SourceID)
		}
	}
	return nil
}

func sourceObligationReuseCloseout(t testing.TB, inventory RuntimeSourceObligationInventory, items []RuntimeCoverageItem) RuntimeResultCloseout {
	t.Helper()
	closeout := RuntimeResultCloseout{SourceObligations: &inventory, DefinitionRevision: inventory.DefinitionRevision, StartupInputHash: inventory.StartupInputHash}
	for _, item := range items {
		checks, err := CompileRuntimeSourceObligationChecks(inventory, item)
		if err != nil {
			t.Fatal(err)
		}
		outcome := RuntimeRequirementOutcome{RequirementRef: item.SourceScope + "/" + item.SourceID, CoverageWorkItemID: item.ID}
		for _, check := range checks {
			outcome.SourceObligations = append(outcome.SourceObligations, RuntimeSourceObligationOutcome{Check: check})
		}
		closeout.RequirementOutcomes = append(closeout.RequirementOutcomes, outcome)
	}
	return closeout
}

func assertCloseoutSourceObligationsReference(t testing.TB, closeout RuntimeResultCloseout) {
	t.Helper()
	before, err := json.Marshal(closeout)
	if err != nil {
		t.Fatal(err)
	}
	got, want := validateRuntimeCloseoutSourceObligations(closeout), referenceCloseoutSourceObligationsRechecksInventory(closeout)
	if (got == nil) != (want == nil) || got != nil && got.Error() != want.Error() {
		t.Fatalf("rejection differs: %v / %v", got, want)
	}
	after, err := json.Marshal(closeout)
	if err != nil || string(before) != string(after) {
		t.Fatal("validation modified original terminal material")
	}
}

func TestCloseoutSourceObligationSameInvocationReusePreservesAllGates(t *testing.T) {
	for _, scenario := range []string{"valid", "nil inventory", "nil inventory with checks", "hash", "original text", "source order", "nested ref order", "definition", "startup", "missing B", "wrong checks", "duplicate coverage", "missing coverage", "unknown source", "wrong CWI", "mixed failures"} {
		t.Run(scenario, func(t *testing.T) {
			seed := sourceObligationReuseSeed(t)
			closeout := sourceObligationReuseCloseout(t, *seed.SourceObligations, seed.CoverageItems)
			switch scenario {
			case "nil inventory":
				closeout.SourceObligations, closeout.RequirementOutcomes = nil, nil
			case "nil inventory with checks":
				closeout.SourceObligations = nil
			case "hash":
				closeout.SourceObligations.Hash = "changed"
			case "original text":
				closeout.SourceObligations.Obligations[0].Text += "\x00"
			case "source order":
				closeout.SourceObligations.Sources[0], closeout.SourceObligations.Sources[1] = closeout.SourceObligations.Sources[1], closeout.SourceObligations.Sources[0]
			case "nested ref order":
				refs := closeout.SourceObligations.Requirements[0].SourceItemRefs
				refs[0], refs[1] = refs[1], refs[0]
			case "definition":
				closeout.DefinitionRevision = "changed"
			case "startup":
				closeout.StartupInputHash = "changed"
			case "missing B":
				closeout.RequirementOutcomes[0].SourceObligations = closeout.RequirementOutcomes[0].SourceObligations[:1]
			case "wrong checks":
				closeout.RequirementOutcomes[0].SourceObligations[0].Check.Criterion = "replacement"
			case "duplicate coverage":
				closeout.RequirementOutcomes = append(closeout.RequirementOutcomes, closeout.RequirementOutcomes[0])
			case "missing coverage":
				closeout.RequirementOutcomes = closeout.RequirementOutcomes[1:]
			case "unknown source":
				closeout.RequirementOutcomes[0].RequirementRef = "dodItem/unknown"
			case "wrong CWI":
				closeout.RequirementOutcomes[0].CoverageWorkItemID = "other"
			case "mixed failures":
				closeout.SourceObligations.Hash, closeout.DefinitionRevision = "changed", "changed"
				closeout.RequirementOutcomes[0].SourceObligations[0].Check.Criterion = "changed"
			}
			assertCloseoutSourceObligationsReference(t, closeout)
			body, err := json.Marshal(closeout)
			if err != nil {
				t.Fatal(err)
			}
			var reload RuntimeResultCloseout
			if err := json.Unmarshal(body, &reload); err != nil {
				t.Fatal(err)
			}
			assertCloseoutSourceObligationsReference(t, reload)
		})
	}
	seed := sourceObligationReuseSeed(t)
	closeout := sourceObligationReuseCloseout(t, *seed.SourceObligations, seed.CoverageItems)
	if err := validateRuntimeCloseoutSourceObligations(closeout); err != nil {
		t.Fatal(err)
	}
	closeout.SourceObligations.Obligations[0].Text = "later mutation"
	if validateRuntimeCloseoutSourceObligations(closeout) == nil {
		t.Fatal("later call reused stale inventory validation")
	}
}

func TestCloseoutSourceObligationReusePreservesMappedHistoricalChecks(t *testing.T) {
	inventory, _, items := evidencePurposeMappingFixture()
	closeout := sourceObligationReuseCloseout(t, inventory, items)
	if err := validateRuntimeCloseoutSourceObligations(closeout); err != nil {
		t.Fatal(err)
	}
	assertCloseoutSourceObligationsReference(t, closeout)
	// The consumer must retain its own original obligation and both historical
	// obligations, not just the common CWI criterion or one producer check.
	if len(closeout.RequirementOutcomes[1].SourceObligations) != 3 {
		t.Fatal("fixture lost complete mapped original checks")
	}
	closeout.RequirementOutcomes[1].SourceObligations = closeout.RequirementOutcomes[1].SourceObligations[1:]
	if validateRuntimeCloseoutSourceObligations(closeout) == nil {
		t.Fatal("mapped historical obligation omission accepted")
	}
	assertCloseoutSourceObligationsReference(t, closeout)
}

func BenchmarkCloseoutSourceObligationSameInvocationReuse(b *testing.B) {
	seed := sourceObligationReuseSeed(b)
	closeout := sourceObligationReuseCloseout(b, *seed.SourceObligations, seed.CoverageItems)
	for _, mode := range []string{"reference", "same-invocation"} {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			for n := 0; n < b.N; n++ {
				var err error
				if mode == "reference" {
					err = referenceCloseoutSourceObligationsRechecksInventory(closeout)
				} else {
					err = validateRuntimeCloseoutSourceObligations(closeout)
				}
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
