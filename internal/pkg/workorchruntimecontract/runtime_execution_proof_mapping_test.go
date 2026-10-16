package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"
)

func executionProofSourceFixture(t *testing.T) (RuntimeLineageAssertionSources, RuntimeLineageAssertion, CoverageEvidenceBinding) {
	t.Helper()
	rule := RuntimeLineageAssertionRule{Ref: "original-work", Kind: RuntimeLineageCompletedWorkRounds, ConsumptionPoint: RuntimeProofPhaseExecution, MinCompletedRounds: 2, ExecutionSourceContractRef: RuntimeCrossSourceExecutionProofV1}
	sources, consumer := lineageSourceFixture(t, rule)
	ledger, bindings := assessmentPurposeGenerationForTest(t, "cwi-source", "source-request", false, "dodItem")
	original := ledger.DefinitionSeed.CoverageItems[0]
	inventory := sourceObligationInventoryFixture()
	inventory.DefinitionRevision, inventory.StartupInputHash = "definition-purpose", "startup-purpose"
	for index := range inventory.Sources {
		if inventory.Sources[index].CandidateItemRef == "dod" {
			inventory.Sources[index].Text = original.TargetText
		}
	}
	item := sources.Facts.DefinitionSeed.CoverageItems[0]
	inventory.Sources = append(inventory.Sources, RuntimeDefinitionSource{CandidateItemRef: "consumer", ItemRef: "consumer-formal", SourceScope: item.SourceScope, SourceID: item.SourceID, Text: item.TargetText})
	inventory.Obligations[0].DoDItemRefs = append(inventory.Obligations[0].DoDItemRefs, "consumer")
	inventory.ExecutionProofMappings = []RuntimeExecutionProofMapping{{ContractRef: RuntimeCrossSourceExecutionProofV1, ConsumerItemRef: "consumer", RuleRef: rule.Ref, SourceItemRef: "dod"}}
	inventory = inventory.WithHash()
	if err := inventory.Validate(); err != nil {
		t.Fatal(err)
	}
	sources.Definition = RuntimeDefinitionContext{Revision: inventory.DefinitionRevision, StartupInputHash: inventory.StartupInputHash, SourceObligations: &inventory}
	sources.Facts.DefinitionSeed.SourceObligations = &inventory
	sources.Facts.DefinitionSeed.CoverageItems = append(sources.Facts.DefinitionSeed.CoverageItems, original)
	ref := RuntimeCWIBindingRefForCoverageBinding(bindings[0], ledger.Manifest.Hash)
	sources.Facts.Coverage = []RuntimeCoverageItemFact{{ID: original.ID, SourceScope: original.SourceScope, SourceID: original.SourceID, TargetText: original.TargetText, CurrentBindingRef: &ref}}
	sources.Semantic = ledger
	return sources, consumer.LineageAssertions[0], bindings[0]
}

// Pure resolver tests use original accepted semantic batches but do not claim
// a full aggregate or actual two-round/repair completion. The existing typed
// consumers must still prove those operations independently after resolution.
func TestExecutionProofMappingResolvesExactOrdinarySourceAndReload(t *testing.T) {
	sources, assertion, expected := executionProofSourceFixture(t)
	before, err := json.Marshal(sources)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded RuntimeLineageAssertionSources
	if err := json.Unmarshal(before, &reloaded); err != nil {
		t.Fatal(err)
	}
	for _, value := range []RuntimeLineageAssertionSources{sources, reloaded} {
		binding, err := runtimeExecutionProofSourceBinding(assertion, value)
		if err != nil || !reflect.DeepEqual(binding, expected) {
			t.Fatalf("original source resolution: %v", err)
		}
	}
	after, err := json.Marshal(sources)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("resolver mutated original source")
	}
	clone := sources.Definition.SourceObligations.Clone()
	clone.ExecutionProofMappings[0].SourceItemRef = "changed"
	if sources.Definition.SourceObligations.ExecutionProofMappings[0].SourceItemRef != "dod" {
		t.Fatal("source map clone aliases confirmed inventory")
	}
}

func TestExecutionProofMappingRejectsMissingAmbiguousDriftAndHistoricalBinding(t *testing.T) {
	for name, mutate := range map[string]func(*RuntimeLineageAssertionSources){
		"unconfirmed": func(s *RuntimeLineageAssertionSources) { s.Definition.SourceObligations = nil },
		"missing":     func(s *RuntimeLineageAssertionSources) { s.Definition.SourceObligations.ExecutionProofMappings = nil },
		"duplicate": func(s *RuntimeLineageAssertionSources) {
			i := s.Definition.SourceObligations
			i.ExecutionProofMappings = append(i.ExecutionProofMappings, i.ExecutionProofMappings[0])
		},
		"unknown source": func(s *RuntimeLineageAssertionSources) {
			s.Definition.SourceObligations.ExecutionProofMappings[0].SourceItemRef = "unknown"
		},
		"self source": func(s *RuntimeLineageAssertionSources) {
			s.Definition.SourceObligations.ExecutionProofMappings[0].SourceItemRef = "consumer"
		},
		"unknown rule": func(s *RuntimeLineageAssertionSources) {
			s.Definition.SourceObligations.ExecutionProofMappings[0].RuleRef = "unknown"
		},
		"changed text": func(s *RuntimeLineageAssertionSources) {
			s.Facts.DefinitionSeed.CoverageItems[1].TargetText = "summarized"
		},
		"duplicate CWI": func(s *RuntimeLineageAssertionSources) {
			s.Facts.DefinitionSeed.CoverageItems = append(s.Facts.DefinitionSeed.CoverageItems, s.Facts.DefinitionSeed.CoverageItems[1])
		},
		"retired binding":      func(s *RuntimeLineageAssertionSources) { s.Facts.Coverage[0].CurrentBindingRef = nil },
		"changed binding hash": func(s *RuntimeLineageAssertionSources) { s.Facts.Coverage[0].CurrentBindingRef.BindingHash = "changed" },
		"different semantic generation": func(s *RuntimeLineageAssertionSources) {
			s.Facts.Coverage[0].CurrentBindingRef.SemanticManifestHash = "historical"
		},
		"changed document": func(s *RuntimeLineageAssertionSources) {
			s.Semantic.BatchDocuments[0].Proposals[0].Target.ProspectiveTargetRefs = []string{"other.txt"}
		},
	} {
		t.Run(name, func(t *testing.T) {
			sources, assertion, _ := executionProofSourceFixture(t)
			mutate(&sources)
			if sources.Definition.SourceObligations != nil {
				value := sources.Definition.SourceObligations.WithHash()
				sources.Definition.SourceObligations = &value
				copy := value.Clone()
				sources.Facts.DefinitionSeed.SourceObligations = &copy
			}
			if _, err := runtimeExecutionProofSourceBinding(assertion, sources); err == nil {
				t.Fatal("invalid mapping/source acquired another CWI proof")
			}
		})
	}
}

func TestExecutionProofMappingCoverageExactDependencyAndCycles(t *testing.T) {
	sources, _, _ := executionProofSourceFixture(t)
	inventory := sources.Definition.SourceObligations.Clone()
	items := sources.Facts.DefinitionSeed.CoverageItems
	before, _ := json.Marshal(items)
	compiled, err := CompileRuntimeExecutionProofMappingCoverage(inventory, items)
	if err != nil {
		t.Fatal(err)
	}
	if len(compiled[0].PrerequisiteCoverageWorkItemIDs) != 1 || compiled[0].PrerequisiteCoverageWorkItemIDs[0] != compiled[1].ID {
		t.Fatal("consumer did not bind exact original predecessor")
	}
	if ValidateRuntimeExecutionProofMappingCoverage(inventory, items) == nil {
		t.Fatal("missing declared source dependency accepted")
	}
	if err := ValidateRuntimeExecutionProofMappingCoverage(inventory, compiled); err != nil {
		t.Fatal(err)
	}
	duplicate, err := CompileRuntimeExecutionProofMappingCoverage(inventory, compiled)
	if err != nil || !reflect.DeepEqual(compiled, duplicate) {
		t.Fatal("dependency compilation is not idempotent")
	}
	after, _ := json.Marshal(items)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("dependency compilation aliases original seed")
	}
	for _, missing := range []int{0, 1} {
		if _, err := CompileRuntimeExecutionProofMappingCoverage(inventory, items[missing:missing+1]); err == nil {
			t.Fatal("missing endpoint guessed from another source")
		}
	}
	inventory.ExecutionProofMappings = append(inventory.ExecutionProofMappings, RuntimeExecutionProofMapping{ContractRef: RuntimeCrossSourceExecutionProofV1, ConsumerItemRef: "dod", RuleRef: "reverse", SourceItemRef: "consumer"})
	inventory = inventory.WithHash()
	if _, err := CompileRuntimeExecutionProofMappingCoverage(inventory, items); err == nil {
		t.Fatal("cross-rule proof dependency cycle accepted")
	}
}

func TestExecutionProofMappingV1DoesNotImplicitlyExpandGoalSourceDomain(t *testing.T) {
	sources, assertion, _ := executionProofSourceFixture(t)
	inventory := sources.Definition.SourceObligations.Clone()
	goalRef := ""
	for _, source := range inventory.Sources {
		if source.SourceScope == "goal" {
			goalRef = source.CandidateItemRef
		}
	}
	if goalRef == "" {
		t.Fatal("fixture has no original Goal source")
	}
	inventory.ExecutionProofMappings[0].ConsumerItemRef = goalRef
	inventory = inventory.WithHash()
	before, err := json.Marshal(inventory)
	if err != nil {
		t.Fatal(err)
	}
	if err := inventory.Validate(); err == nil {
		t.Fatal("v1 accepted an unapproved Goal consumer source domain")
	}
	if err := ValidateRuntimeExecutionProofMappings(inventory, sources.Facts.Activation.ExecutionContract.ProfileContracts); err == nil {
		t.Fatal("Profile validator guessed a Goal consumer as an original DoD")
	}
	if _, err := CompileRuntimeExecutionProofMappingCoverage(inventory, sources.Facts.DefinitionSeed.CoverageItems); err == nil {
		t.Fatal("compiler created a missing Goal execution endpoint")
	}
	sources.Definition.SourceObligations = &inventory
	copy := inventory.Clone()
	sources.Facts.DefinitionSeed.SourceObligations = &copy
	if _, err := runtimeExecutionProofSourceBinding(assertion, sources); err == nil {
		t.Fatal("consumer reused another CWI's rounds outside the confirmed v1 domain")
	}
	after, err := json.Marshal(inventory)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("domain rejection rewrote original confirmed sources")
	}
}
