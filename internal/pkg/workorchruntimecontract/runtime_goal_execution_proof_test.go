package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func goalExecutionFixture() (RuntimeSourceObligationInventory, []RuntimeProfileContract, RuntimeCoverageItem) {
	inventory := sourceObligationInventoryFixture()
	goalText := " 完整 Goal\r\n两轮与修复，普通工程行为仍须证明。 "
	for n := range inventory.Sources {
		if inventory.Sources[n].SourceScope == "goal" {
			inventory.Sources[n].Text = goalText
		}
	}
	rule := RuntimeLineageAssertionRule{Ref: "goal-original-work", Kind: RuntimeLineageCompletedWorkRounds, ConsumptionPoint: RuntimeProofPhaseExecution, MinCompletedRounds: 2, ExecutionSourceContractRef: RuntimeGoalSourceExecutionProof}
	inventory.ExecutionProofMappings = []RuntimeExecutionProofMapping{{ContractRef: RuntimeGoalSourceExecutionProof, ConsumerItemRef: "goal", RuleRef: rule.Ref, SourceItemRef: "dod", ConsumerGoalText: goalText}}
	inventory = inventory.WithHash()
	profiles := []RuntimeProfileContract{{ID: "profile", EvidenceRequirements: []RuntimeProfileEvidenceRequirement{
		{ID: "goal", Scope: "goal", RequiredEvidenceKinds: []string{EvidenceKindValidation}, AcceptedStepKinds: []string{RuntimeBlockKindManagedValidationSession}, LineageAssertions: []RuntimeLineageAssertionRule{rule}},
		{ID: "dod-001", Scope: "dodItem", RequiredEvidenceKinds: []string{EvidenceKindValidation}, AcceptedStepKinds: []string{RuntimeBlockKindManagedValidationSession}},
	}}}
	item := RuntimeCoverageItem{ID: "original-cwi", SliceID: "slice", SourceScope: "dodItem", SourceID: "dod-001", TargetText: "共同判据"}
	return inventory, profiles, item
}

func TestGoalExecutionProofSealsCompleteGoalANDOrdinaryOriginalChecks(t *testing.T) {
	i, p, item := goalExecutionFixture()
	if err := i.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRuntimeSourceObligationApplicability(&i, p); err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(i)
	var err error
	item.GoalExecutionAssertions, err = CompileRuntimeGoalExecutionAssertions(&i, item, p)
	if err != nil || len(item.GoalExecutionAssertions) != 1 {
		t.Fatalf("Goal projection: %v", err)
	}
	item.SourceObligationChecks, err = CompileRuntimeSourceObligationChecks(i, item)
	if err != nil || len(item.SourceObligationChecks) != 3 {
		t.Fatalf("ordinary checks: %v", err)
	}
	criteria := map[string]bool{}
	for _, check := range item.SourceObligationChecks {
		criteria[check.Criterion] = true
	}
	if !criteria[" A 原判据\n "] || !criteria["B 原判据"] || !criteria[i.ExecutionProofMappings[0].ConsumerGoalText] {
		t.Fatal("ordinary Goal/A/B source bytes lost")
	}
	focus := RuntimeDefinitionFocus{ID: "ordinary", CoverageWorkItemIDs: []common.CoverageWorkItemID{item.ID}, AssessmentScope: item.SourceScope, AssessmentID: item.SourceID, AssessmentText: item.TargetText,
		EvidenceDeclarationRef: CanonicalRuntimeDefinitionEvidenceDeclarationRef(item.SourceScope, item.SourceID), RequiredEvidenceKinds: []string{EvidenceKindValidation}, AcceptedStepKinds: []string{RuntimeBlockKindManagedValidationSession}}
	reqs, err := CompileRuntimeValidationProofRequirements(i.DefinitionRevision, []RuntimeCoverageItem{item}, []RuntimeDefinitionFocus{focus})
	if err != nil || len(reqs) != 1 || len(reqs[0].RequiredCheckRefs) != 4 || !reflect.DeepEqual(reqs[0].LineageAssertions, item.GoalExecutionAssertions) {
		t.Fatalf("ordinary AND Goal requirement: %v", err)
	}
	seed := RuntimeDefinitionSeed{DefinitionRevision: i.DefinitionRevision, SourceObligations: &i, CoverageItems: []RuntimeCoverageItem{item}}
	if err := ValidateRuntimeSourceObligationSeed(seed); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(seed)
	var reload RuntimeDefinitionSeed
	if json.Unmarshal(payload, &reload) != nil || ValidateRuntimeGoalExecutionAssertionSeed(reload, p) != nil {
		t.Fatal("reload lost Goal guard")
	}
	compiled, err := CompileRuntimeExecutionProofMappingCoverage(i, []RuntimeCoverageItem{item})
	if err != nil || len(compiled) != 1 || len(compiled[0].PrerequisiteCoverageWorkItemIDs) != 0 {
		t.Fatalf("invented Goal work/predecessor: %v", err)
	}
	clone := seed.Clone()
	clone.CoverageItems[0].GoalExecutionAssertions[0].Rule.MinCompletedRounds = 1
	if ValidateRuntimeGoalExecutionAssertionSeed(clone, p) == nil || seed.CoverageItems[0].GoalExecutionAssertions[0].Rule.MinCompletedRounds != 2 {
		t.Fatal("coordinated rule drift or clone alias accepted")
	}
	clone = seed.Clone()
	clone.CoverageItems[0].GoalExecutionAssertions = nil
	if ValidateRuntimeSourceObligationSeed(clone) == nil {
		t.Fatal("stripped Goal consumer accepted")
	}
	clone = seed.Clone()
	clone.CoverageItems[0].SourceObligationChecks = clone.CoverageItems[0].SourceObligationChecks[1:]
	if ValidateRuntimeSourceObligationSeed(clone) == nil {
		t.Fatal("stripped full ordinary Goal accepted")
	}
	after, _ := json.Marshal(i)
	if string(before) != string(after) {
		t.Fatal("compiler changed confirmed source")
	}
}

func TestGoalExecutionProofFailsClosedForWrongSourceVersionAndGoalBytes(t *testing.T) {
	for name, change := range map[string]func(*RuntimeSourceObligationInventory, *[]RuntimeProfileContract){
		"legacy": func(i *RuntimeSourceObligationInventory, _ *[]RuntimeProfileContract) {
			i.ExecutionProofMappings[0].ContractRef = RuntimeDoDSourceExecutionProof
		},
		"unknown": func(i *RuntimeSourceObligationInventory, _ *[]RuntimeProfileContract) {
			i.ExecutionProofMappings[0].SourceItemRef = "missing"
		},
		"missing": func(i *RuntimeSourceObligationInventory, _ *[]RuntimeProfileContract) { i.ExecutionProofMappings = nil },
		"duplicate": func(i *RuntimeSourceObligationInventory, _ *[]RuntimeProfileContract) {
			i.ExecutionProofMappings = append(i.ExecutionProofMappings, i.ExecutionProofMappings[0])
		},
		"trimmed": func(i *RuntimeSourceObligationInventory, _ *[]RuntimeProfileContract) {
			i.ExecutionProofMappings[0].ConsumerGoalText = strings.TrimSpace(i.ExecutionProofMappings[0].ConsumerGoalText)
		},
		"oversize": func(i *RuntimeSourceObligationInventory, _ *[]RuntimeProfileContract) {
			for n := range i.Sources {
				if i.Sources[n].SourceScope == "goal" {
					i.Sources[n].Text = strings.Repeat("g", 4097)
				}
			}
			i.ExecutionProofMappings[0].ConsumerGoalText = strings.Repeat("g", 4097)
		},
		"no ordinary": func(_ *RuntimeSourceObligationInventory, p *[]RuntimeProfileContract) {
			(*p)[0].EvidenceRequirements[1].RequiredEvidenceKinds = []string{EvidenceKindStateChange}
			(*p)[0].EvidenceRequirements[1].AcceptedStepKinds = []string{StepKindFileWrite}
		},
		"undeclared": func(_ *RuntimeSourceObligationInventory, p *[]RuntimeProfileContract) {
			(*p)[0].EvidenceRequirements[0].LineageAssertions = nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			i, p, _ := goalExecutionFixture()
			change(&i, &p)
			i = i.WithHash()
			if ValidateRuntimeSourceObligationApplicability(&i, p) == nil {
				t.Fatal("invalid Goal mapping accepted")
			}
		})
	}
}

func TestGoalExecutionProofRootDomainAndNoSyntheticWorkCredit(t *testing.T) {
	i, p, item := goalExecutionFixture()
	p[0].EvidenceRequirements[0].Scope = "goalCoverage"
	p[0].EvidenceRequirements[0].ID = "*"
	var err error
	item.GoalExecutionAssertions, err = CompileRuntimeGoalExecutionAssertions(&i, item, p)
	if err != nil || len(item.GoalExecutionAssertions) != 1 {
		t.Fatalf("confirmed root domain: %v", err)
	}
	item.SourceObligationChecks, err = CompileRuntimeSourceObligationChecks(i, item)
	if err != nil {
		t.Fatal(err)
	}
	sources := RuntimeLineageAssertionSources{Definition: RuntimeDefinitionContext{Revision: i.DefinitionRevision, StartupInputHash: i.StartupInputHash, SourceObligations: &i}, Facts: RuntimeFacts{DefinitionSeed: RuntimeDefinitionSeed{SourceObligations: &i, CoverageItems: []RuntimeCoverageItem{item}}, Activation: RuntimeActivationContext{ExecutionContract: RuntimeExecutionContract{ProfileContracts: p}}}}
	binding := CoverageEvidenceBinding{CoverageWorkItemID: item.ID, DeclarationRef: CanonicalRuntimeDefinitionEvidenceDeclarationRef(item.SourceScope, item.SourceID), LineageAssertions: item.GoalExecutionAssertions}
	if _, err := runtimeExpectedLineageAssertions(binding, sources); err != nil {
		t.Fatal(err)
	}
	if RuntimeExecutionLineageAssertionsSatisfied(binding, sources, InitialRuntimeEvidenceRevision) {
		t.Fatal("confirmed map manufactured work or ordinary evidence")
	}
	for _, change := range []func(*RuntimeLineageAssertionSources){
		func(s *RuntimeLineageAssertionSources) { s.Definition.SourceObligations = nil },
		func(s *RuntimeLineageAssertionSources) {
			s.Facts.DefinitionSeed.CoverageItems[0].GoalExecutionAssertions = nil
		},
	} {
		body, _ := json.Marshal(sources)
		var copy RuntimeLineageAssertionSources
		_ = json.Unmarshal(body, &copy)
		change(&copy)
		if RuntimeExecutionLineageAssertionsSatisfied(binding, copy, InitialRuntimeEvidenceRevision) {
			t.Fatal("missing/unconfirmed consumer acquired Goal credit")
		}
	}
}
