package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"
)

func profilePurposeFixture() (RuntimeSourceObligationInventory, []RuntimeProfileContract, []RuntimeCoverageItem) {
	i, p, items := evidencePurposeMappingFixture()
	p[0].DeliverableExpectations = []RuntimeDeliverableExpectation{{ID: "created-source", Required: true, Description: "  原 Profile 交付全文\n不能把历史创建当当前验收。  "}}
	p[0].EvidenceRequirements = append(p[0].EvidenceRequirements, RuntimeProfileEvidenceRequirement{ID: "created-source", Scope: "deliverable", RequiredEvidenceKinds: []string{EvidenceKindStateChange}, AcceptedStepKinds: []string{StepKindFileWrite}})
	i.EvidencePurposeMappings = []RuntimeEvidencePurposeMapping{{ContractRef: RuntimeProfileDeliverableEvidencePurpose,
		HistoricalItemRef: RuntimeProfileDeliverablePurposeRef(p[0].ID, "created-source"), AcceptanceItemRef: "acceptance",
		HistoricalProfileID: p[0].ID, HistoricalDeliverableID: "created-source", HistoricalText: p[0].DeliverableExpectations[0].Description,
		HistoricalProfileHash: StableRuntimeProfileContractsHash(p)}}
	i = i.WithHash()
	items[0].SourceScope, items[0].SourceID, items[0].TargetText = "deliverable", "created-source", "原 Profile 交付全文\n不能把历史创建当当前验收。"
	return i, p, items
}

func TestProfileEvidencePurposePreservesFullSnapshotChecksAndLegacyIdentity(t *testing.T) {
	i, p, items := profilePurposeFixture()
	before, _ := json.Marshal(i)
	if err := ValidateRuntimeSourceObligationApplicability(&i, p); err != nil {
		t.Fatal(err)
	}
	compiled, err := CompileRuntimeEvidencePurposeMappingCoverage(i, items)
	if err != nil {
		t.Fatal(err)
	}
	if compiled[0].HistoricalAcceptance.ContractRef != RuntimeProfileDeliverableEvidencePurpose || compiled[1].PrerequisiteCoverageWorkItemIDs[0] != items[0].ID {
		t.Fatal("Profile source was not bound to exact consumer")
	}
	for _, item := range compiled {
		checks, err := CompileRuntimeSourceObligationChecks(i, item)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, check := range checks {
			if check.Criterion == i.EvidencePurposeMappings[0].HistoricalText && check.Declaration == check.Criterion {
				found = true
			}
		}
		if !found {
			t.Fatal("complete raw Profile obligation omitted from source or consumer")
		}
		if item.ID == items[1].ID && len(checks) != 2 {
			t.Fatal("ordinary consumer lost own original obligation")
		}
	}
	var reload RuntimeSourceObligationInventory
	if json.Unmarshal(before, &reload) != nil || ValidateRuntimeSourceObligationApplicability(&reload, p) != nil {
		t.Fatal("reload changed snapshot identity")
	}
	again, err := CompileRuntimeEvidencePurposeMappingCoverage(reload, compiled)
	if err != nil || !reflect.DeepEqual(again, compiled) {
		t.Fatal("reload/duplicate compilation changed endpoints")
	}
	clone := i.Clone()
	clone.EvidencePurposeMappings[0].HistoricalText = "changed"
	after, _ := json.Marshal(i)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("compiler or clone changed original source")
	}
	if clone.WithHash().Hash == i.Hash {
		t.Fatal("raw Profile text absent from inventory hash")
	}
	legacy, _, _ := evidencePurposeMappingFixture()
	raw, _ := json.Marshal(legacy.EvidencePurposeMappings[0])
	if string(raw) != `{"ContractRef":"runtime/cross-cwi-evidence-purpose","HistoricalItemRef":"dod","AcceptanceItemRef":"acceptance"}` {
		t.Fatal("empty extension changed v1 wire")
	}
}

func TestProfileEvidencePurposeRejectsRehashedDriftMissingAndAmbiguousSources(t *testing.T) {
	for name, mutate := range map[string]func(*RuntimeSourceObligationInventory, *[]RuntimeProfileContract){
		"wrong Profile": func(i *RuntimeSourceObligationInventory, _ *[]RuntimeProfileContract) {
			i.EvidencePurposeMappings[0].HistoricalProfileID = "other"
		},
		"changed snapshot": func(i *RuntimeSourceObligationInventory, _ *[]RuntimeProfileContract) {
			i.EvidencePurposeMappings[0].HistoricalProfileHash = "replacement"
		},
		"summarized original": func(i *RuntimeSourceObligationInventory, _ *[]RuntimeProfileContract) {
			i.EvidencePurposeMappings[0].HistoricalText = "原 Profile 交付全文"
		},
		"wrong consumer": func(i *RuntimeSourceObligationInventory, _ *[]RuntimeProfileContract) {
			i.EvidencePurposeMappings[0].AcceptanceItemRef = "missing"
		},
		"missing deliverable": func(_ *RuntimeSourceObligationInventory, p *[]RuntimeProfileContract) {
			(*p)[0].DeliverableExpectations = nil
		},
		"optional deliverable": func(_ *RuntimeSourceObligationInventory, p *[]RuntimeProfileContract) {
			(*p)[0].DeliverableExpectations[0].Required = false
		},
		"ambiguous source domain": func(_ *RuntimeSourceObligationInventory, p *[]RuntimeProfileContract) { *p = append(*p, (*p)[0]) },
		"duplicate mapping": func(i *RuntimeSourceObligationInventory, _ *[]RuntimeProfileContract) {
			i.EvidencePurposeMappings = append(i.EvidencePurposeMappings, i.EvidencePurposeMappings[0])
		},
		"legacy cannot borrow Profile": func(i *RuntimeSourceObligationInventory, _ *[]RuntimeProfileContract) {
			i.EvidencePurposeMappings[0].ContractRef = RuntimeCrossCWIEvidencePurpose
		},
		"no ordinary proof": func(_ *RuntimeSourceObligationInventory, p *[]RuntimeProfileContract) {
			(*p)[0].EvidenceRequirements[1].RequiredEvidenceKinds = []string{EvidenceKindStateChange}
		},
	} {
		t.Run(name, func(t *testing.T) {
			i, p, _ := profilePurposeFixture()
			mutate(&i, &p)
			i = i.WithHash()
			if ValidateRuntimeSourceObligationApplicability(&i, p) == nil {
				t.Fatal("self-consistent hash bypassed confirmed Profile/original consumer")
			}
		})
	}
}
