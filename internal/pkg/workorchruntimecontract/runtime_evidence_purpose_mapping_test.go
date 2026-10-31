package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func evidencePurposeMappingFixture() (RuntimeSourceObligationInventory, []RuntimeProfileContract, []RuntimeCoverageItem) {
	inventory := sourceObligationInventoryFixture()
	inventory.Sources = append(inventory.Sources, RuntimeDefinitionSource{CandidateItemRef: "acceptance", ItemRef: "acceptance-formal", SourceScope: "dodItem", SourceID: "dod-002", Text: "后继当前验收；不能代替原历史行为"})
	inventory.Obligations = append(inventory.Obligations, RuntimeSourceObligation{ID: "C", RequirementID: "requirement", DoDItemRefs: []string{"acceptance"}, Text: inventory.Sources[2].Text})
	inventory.Verifications = append(inventory.Verifications, RuntimeSourceVerification{ID: "verify-C", ObligationID: "C", Declaration: "普通验收", ProfileID: "profile", EvidenceRequirementID: "dod-002", Scope: "dodItem"})
	inventory.EvidencePurposeMappings = []RuntimeEvidencePurposeMapping{{ContractRef: RuntimeCrossCWIEvidencePurpose, HistoricalItemRef: "dod", AcceptanceItemRef: "acceptance"}}
	inventory = inventory.WithHash()
	texts := map[string]string{}
	for _, source := range inventory.Sources {
		texts[source.CandidateItemRef] = source.Text
	}
	profiles := []RuntimeProfileContract{{ID: "profile", EvidenceRequirements: []RuntimeProfileEvidenceRequirement{
		{ID: "dod-001", Scope: "dodItem", RequiredEvidenceKinds: []string{EvidenceKindStateChange}, AcceptedStepKinds: []string{StepKindFileWrite}},
		{ID: "dod-002", Scope: "dodItem", RequiredEvidenceKinds: []string{EvidenceKindValidation}, AcceptedStepKinds: []string{RuntimeBlockKindManagedValidationSession}},
	}}}
	items := []RuntimeCoverageItem{
		{ID: common.CoverageWorkItemID("history"), SourceScope: "dodItem", SourceID: "dod-001", TargetText: texts["dod"], SliceID: "slice"},
		{ID: common.CoverageWorkItemID("current"), SourceScope: "dodItem", SourceID: "dod-002", TargetText: texts["acceptance"], SliceID: "slice"},
	}
	return inventory, profiles, items
}

func TestEvidencePurposeMappingPresealIdentityCoverageAndReload(t *testing.T) {
	inventory, profiles, items := evidencePurposeMappingFixture()
	before, _ := json.Marshal(inventory)
	var reload RuntimeSourceObligationInventory
	if err := json.Unmarshal(before, &reload); err != nil {
		t.Fatal(err)
	}
	for _, value := range []RuntimeSourceObligationInventory{inventory, reload} {
		if err := value.Validate(); err != nil {
			t.Fatal(err)
		}
		if err := ValidateRuntimeSourceObligationApplicability(&value, profiles); err != nil {
			t.Fatal(err)
		}
		compiled, err := CompileRuntimeEvidencePurposeMappingCoverage(value, items)
		if err != nil {
			t.Fatal(err)
		}
		if len(compiled[0].PrerequisiteCoverageWorkItemIDs) != 0 || !reflect.DeepEqual(compiled[1].PrerequisiteCoverageWorkItemIDs, []common.CoverageWorkItemID{items[0].ID}) {
			t.Fatal("mapped successor did not retain exact historical predecessor")
		}
		duplicate, err := CompileRuntimeEvidencePurposeMappingCoverage(value, compiled)
		if err != nil || !reflect.DeepEqual(duplicate, compiled) {
			t.Fatal("dependency compiler is not idempotent")
		}
		if ValidateRuntimeEvidencePurposeMappingCoverage(value, items) == nil {
			t.Fatal("missing historical predecessor accepted")
		}
		if err := ValidateRuntimeEvidencePurposeMappingCoverage(value, compiled); err != nil {
			t.Fatal(err)
		}
		compiled[1].PrerequisiteCoverageWorkItemIDs[0] = "changed"
		if len(items[1].PrerequisiteCoverageWorkItemIDs) != 0 {
			t.Fatal("dependency compiler aliases original input")
		}
	}
	clone := inventory.Clone()
	clone.EvidencePurposeMappings[0].AcceptanceItemRef = "changed"
	after, _ := json.Marshal(inventory)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("original sealed bytes changed")
	}
	legacy := inventory.Clone()
	legacy.EvidencePurposeMappings = nil
	legacy = legacy.WithHash()
	if legacy.Hash == inventory.Hash {
		t.Fatal("mapping omitted from original identity")
	}
}

func TestEvidencePurposeMappingAvailabilityRequiresCompleteOriginalSource(t *testing.T) {
	i, _, _ := evidencePurposeMappingFixture()
	if err := ValidateRuntimeEvidencePurposeMappingAvailability(&i); err != nil {
		t.Fatalf("supported original source rejected: %v", err)
	}
	changed := i.Clone()
	changed.EvidencePurposeMappings[0].AcceptanceItemRef = "not-original"
	changed = changed.WithHash()
	if err := ValidateRuntimeEvidencePurposeMappingAvailability(&changed); err == nil {
		t.Fatal("coordinated rehash enabled missing original consumer")
	}
	if err := (RuntimeDefinitionSeed{SchemaVersion: RuntimeDefinitionSeedSchemaCurrent, DefinitionRevision: i.DefinitionRevision, PolicyHash: "policy", CoverageItems: []RuntimeCoverageItem{{ID: "cwi"}}, Slices: []RuntimeAcceptanceSlice{{ID: "slice"}}, SourceObligations: &i}).Validate(); err == nil {
		t.Fatal("fresh/reloaded seed enabled a partial source without mapped coverage")
	}
	i.EvidencePurposeMappings = nil
	if err := ValidateRuntimeEvidencePurposeMappingAvailability(&i); err != nil {
		t.Fatal("legacy was silently upgraded or blocked")
	}
	if err := ValidateRuntimeEvidencePurposeMappingAvailability(nil); err != nil {
		t.Fatal(err)
	}
}

func TestEvidencePurposeMappingRejectsCoordinatedRehashAndNonordinarySources(t *testing.T) {
	for name, mutate := range map[string]func(*RuntimeSourceObligationInventory, *[]RuntimeProfileContract){
		"missing endpoint": func(i *RuntimeSourceObligationInventory, _ *[]RuntimeProfileContract) {
			i.EvidencePurposeMappings[0].AcceptanceItemRef = "unknown"
		},
		"self": func(i *RuntimeSourceObligationInventory, _ *[]RuntimeProfileContract) {
			i.EvidencePurposeMappings[0].AcceptanceItemRef = "dod"
		},
		"duplicate": func(i *RuntimeSourceObligationInventory, _ *[]RuntimeProfileContract) {
			i.EvidencePurposeMappings = append(i.EvidencePurposeMappings, i.EvidencePurposeMappings[0])
		},
		"ambiguous source": func(i *RuntimeSourceObligationInventory, _ *[]RuntimeProfileContract) {
			i.Sources = append(i.Sources, i.Sources[1])
		},
		"version": func(i *RuntimeSourceObligationInventory, _ *[]RuntimeProfileContract) {
			i.EvidencePurposeMappings[0].ContractRef = "runtime/cross-cwi-evidence-purpose/v0"
		},
		"cycle": func(i *RuntimeSourceObligationInventory, _ *[]RuntimeProfileContract) {
			i.EvidencePurposeMappings = append(i.EvidencePurposeMappings, RuntimeEvidencePurposeMapping{ContractRef: RuntimeCrossCWIEvidencePurpose, HistoricalItemRef: "acceptance", AcceptanceItemRef: "dod"})
		},
		"legacy owner has no purpose": func(_ *RuntimeSourceObligationInventory, p *[]RuntimeProfileContract) {
			(*p)[0].EvidenceRequirements[0].AcceptedStepKinds = []string{StepKindGitWrite}
		},
		"no ordinary validation": func(_ *RuntimeSourceObligationInventory, p *[]RuntimeProfileContract) {
			(*p)[0].EvidenceRequirements[1].AcceptedStepKinds = []string{StepKindFileRead}
		},
		"quality-only wildcard cannot publish proof": func(_ *RuntimeSourceObligationInventory, p *[]RuntimeProfileContract) {
			(*p)[0].EvidenceRequirements[1].ID = "*"
		},
		"mixed historical requirement": func(_ *RuntimeSourceObligationInventory, p *[]RuntimeProfileContract) {
			(*p)[0].EvidenceRequirements[0].RequiredEvidenceKinds = []string{EvidenceKindStateChange, EvidenceKindValidation}
		},
	} {
		t.Run(name, func(t *testing.T) {
			i, p, _ := evidencePurposeMappingFixture()
			mutate(&i, &p)
			i = i.WithHash()
			if i.Validate() == nil && ValidateRuntimeEvidencePurposeMappings(i, p) == nil {
				t.Fatal("invalid mapping acquired historical/current proof purpose")
			}
		})
	}
}

func TestEvidencePurposeMappingCannotGuessCoverageOrHideDependencyCycle(t *testing.T) {
	i, _, items := evidencePurposeMappingFixture()
	for name, mutate := range map[string]func(*[]RuntimeCoverageItem){
		"criterion drift":    func(v *[]RuntimeCoverageItem) { (*v)[0].TargetText = "summary" },
		"ambiguous coverage": func(v *[]RuntimeCoverageItem) { *v = append(*v, (*v)[0]) },
		"missing coverage":   func(v *[]RuntimeCoverageItem) { *v = (*v)[:1] },
		"inverse dependency": func(v *[]RuntimeCoverageItem) {
			(*v)[0].PrerequisiteCoverageWorkItemIDs = []common.CoverageWorkItemID{(*v)[1].ID}
		},
	} {
		t.Run(name, func(t *testing.T) {
			v := append([]RuntimeCoverageItem(nil), items...)
			mutate(&v)
			if _, err := CompileRuntimeEvidencePurposeMappingCoverage(i, v); err == nil {
				t.Fatal("invalid original coverage accepted")
			}
		})
	}
}

func purposeEndpointFixture(t *testing.T) (RuntimeDefinitionContext, RuntimeExecutionContract, RuntimeDefinitionSeed) {
	t.Helper()
	i, profiles, items := evidencePurposeMappingFixture()
	definition := RuntimeDefinitionContext{Revision: i.DefinitionRevision, StartupInputHash: i.StartupInputHash, SourceObligations: &i}
	execution := RuntimeExecutionContract{ProfileContracts: profiles}
	items, err := CompileRuntimeEvidencePurposeMappingCoverage(i, items)
	if err != nil {
		t.Fatal(err)
	}
	for n := range items {
		items[n].SourceObligationChecks, err = CompileRuntimeSourceObligationChecks(i, items[n])
		if err != nil {
			t.Fatal(err)
		}
	}
	seed := RuntimeDefinitionSeed{SchemaVersion: RuntimeDefinitionSeedSchemaCurrent, DefinitionRevision: i.DefinitionRevision, PolicyHash: StableRuntimeCandidatePrefixHashWithExecutionContract(definition, execution), CoverageItems: items, SourceObligations: &i}
	seed.Hash = StableRuntimeDefinitionSeedHash(seed)
	return definition, execution, seed
}

func TestEvidencePurposeMappingRejectsHiddenAcceptanceCreditCycle(t *testing.T) {
	inventory, _, items := evidencePurposeMappingFixture()
	intermediate := RuntimeCoverageItem{ID: "intermediate", SliceID: items[0].SliceID, SourceScope: "test", SourceID: "independent", TargetText: "original independent item", PrerequisiteCoverageWorkItemIDs: []common.CoverageWorkItemID{items[0].ID}}
	items = append(items, intermediate)
	// An independent item may wait for the historical source's final credit
	// after its consumer settles. This does not grant it the behavior exception.
	if _, err := CompileRuntimeEvidencePurposeMappingCoverage(inventory, items); err != nil {
		t.Fatal(err)
	}
	items[1].PrerequisiteCoverageWorkItemIDs = append(items[1].PrerequisiteCoverageWorkItemIDs, intermediate.ID)
	if err := ValidateRuntimeCoverageItemDependencies(items); err != nil {
		t.Fatal("fixture must remain an ordinary acyclic source graph")
	}
	if _, err := CompileRuntimeEvidencePurposeMappingCoverage(inventory, items); err == nil {
		t.Fatal("consumer waited for its own deferred historical terminal credit")
	}
}

func TestEvidencePurposeEndpointsAreExactReadOnlyAndNotExecutableCredit(t *testing.T) {
	d, e, s := purposeEndpointFixture(t)
	before, _ := json.Marshal(s)
	var reload RuntimeDefinitionSeed
	if err := json.Unmarshal(before, &reload); err != nil {
		t.Fatal(err)
	}
	for _, seed := range []RuntimeDefinitionSeed{s, reload} {
		endpoints, err := ResolveRuntimeEvidencePurposeEndpoints(d, e, seed, "history")
		if err != nil {
			t.Fatal(err)
		}
		if endpoints.Historical.ID != "history" || endpoints.Acceptance.ID != "current" || endpoints.InventoryHash != s.SourceObligations.Hash {
			t.Fatal("resolver substituted original endpoint")
		}
		endpoints.Acceptance.PrerequisiteCoverageWorkItemIDs[0] = "changed"
		endpoints.Historical.SourceObligationChecks[0].Criterion = "changed"
		if _, err := ResolveRuntimeEvidencePurposeEndpoints(d, e, seed, "current"); err == nil {
			t.Fatal("ordinary source auto-acquired historical use")
		}
	}
	after, _ := json.Marshal(s)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("detached resolution mutated original seed")
	}
}

func TestEvidencePurposeEndpointsRejectIndependentOriginalDrift(t *testing.T) {
	for name, mutate := range map[string]func(*RuntimeDefinitionContext, *RuntimeExecutionContract, *RuntimeDefinitionSeed){
		"no confirmed inventory": func(d *RuntimeDefinitionContext, _ *RuntimeExecutionContract, _ *RuntimeDefinitionSeed) {
			d.SourceObligations = nil
		},
		"startup drift": func(d *RuntimeDefinitionContext, _ *RuntimeExecutionContract, _ *RuntimeDefinitionSeed) {
			d.StartupInputHash = "other"
		},
		"seed mapping replaced": func(_ *RuntimeDefinitionContext, _ *RuntimeExecutionContract, s *RuntimeDefinitionSeed) {
			i := s.SourceObligations.Clone()
			i.EvidencePurposeMappings = nil
			i = i.WithHash()
			s.SourceObligations = &i
			s.Hash = StableRuntimeDefinitionSeedHash(*s)
		},
		"criterion drift and seed rehash": func(_ *RuntimeDefinitionContext, _ *RuntimeExecutionContract, s *RuntimeDefinitionSeed) {
			s.CoverageItems[0].TargetText = "summary"
			s.Hash = StableRuntimeDefinitionSeedHash(*s)
		},
		"missing predecessor and seed rehash": func(_ *RuntimeDefinitionContext, _ *RuntimeExecutionContract, s *RuntimeDefinitionSeed) {
			s.CoverageItems[1].PrerequisiteCoverageWorkItemIDs = nil
			s.Hash = StableRuntimeDefinitionSeedHash(*s)
		},
		"ordinary declaration unavailable": func(_ *RuntimeDefinitionContext, e *RuntimeExecutionContract, _ *RuntimeDefinitionSeed) {
			e.ProfileContracts[0].EvidenceRequirements[1].AcceptedStepKinds = nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			d, e, s := purposeEndpointFixture(t)
			mutate(&d, &e, &s)
			if _, err := ResolveRuntimeEvidencePurposeEndpoints(d, e, s, "history"); err == nil {
				t.Fatal("drifted original mapping resolved")
			}
		})
	}
}
