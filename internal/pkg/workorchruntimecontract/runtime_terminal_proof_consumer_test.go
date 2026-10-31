package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func terminalIndexProofFixture(t *testing.T) (RuntimeStateRecord, RuntimeTerminalResultMaterial) {
	t.Helper()
	source := terminalEvidenceApplicabilityFixture(t)
	source.Facts.Evidence.Accepted[0].AuthoritySnapshotID = "original-authority"
	source.Facts.Evidence.Accepted[0].Revision = "runtime-evidence-00000000000000000001"
	source.Facts.Evidence.Accepted[0].ContentHash = "original-content-hash"
	// Technical source fixture only, not a real capability sampling claim.
	reads := (RuntimeOwnerRevisionSet{DefinitionRevision: source.Facts.DefinitionRevision, Entries: []RuntimeOwnerRevisionRef{{OwnerKind: "capability", OwnerInstanceID: RuntimeModuleShell + "/" + string(source.Facts.WorkspaceID), ResourceKind: string(RuntimeResourceWorkspaceTarget), ResourceID: "original-fixture-input", Revision: "original-fixture-bytes", DependencyHash: "original-fixture-selector"}}}).WithHash()
	source.Facts.Evidence.Accepted[0].OwnerRevisionSet = &reads
	coverage := &source.Facts.Coverage[0]
	coverage.SourceScope, coverage.SourceID, coverage.TargetText = "dodItem", "original-dod", "完整原验收判据"
	item := RuntimeCoverageItem{ID: coverage.ID, SourceScope: coverage.SourceScope, SourceID: coverage.SourceID, TargetText: coverage.TargetText}
	source.Facts.DefinitionSeed.CoverageItems = []RuntimeCoverageItem{item}
	rule := RuntimeLineageAssertionRule{Ref: "formal-index", Kind: RuntimeLineageTerminalResultIndex, ConsumptionPoint: RuntimeProofPhasePostTerminalDelivery, TerminalContractRef: RuntimeTerminalReceiptProofContract}
	source.Facts.Activation.ExecutionContract.ProfileContracts = []RuntimeProfileContract{{ID: "original-profile", EvidenceRequirements: []RuntimeProfileEvidenceRequirement{{ID: item.SourceID, Scope: item.SourceScope, RequiredEvidenceKinds: []string{EvidenceKindValidation}, LineageAssertions: []RuntimeLineageAssertionRule{rule}}}}}
	source.Decision.State = ConvergenceStateComplete
	material, err := DeriveRuntimeTerminalResultMaterialFromState(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateRuntimeTerminalProofMaterial(source, material); err != nil {
		t.Fatal(err)
	}
	return source, material
}

func TestRuntimeTerminalProofRuleVersionedClosedUnionAndIdentity(t *testing.T) {
	index := RuntimeLineageAssertionRule{Ref: "index", Kind: RuntimeLineageTerminalResultIndex, ConsumptionPoint: RuntimeProofPhasePostTerminalDelivery, TerminalContractRef: RuntimeTerminalReceiptProofContract}
	receipt := RuntimeLineageAssertionRule{Ref: "receipt", Kind: RuntimeLineageTerminalOwnerReceipt, ConsumptionPoint: RuntimeProofPhasePostTerminalDelivery, TerminalContractRef: RuntimeTerminalReceiptProofContract, ReceiptSourceScope: "deliverable", ReceiptSourceID: "final-material", OwnerModuleID: RuntimeModuleFile, OperationKind: RuntimeBlockKindPatchApply, OperationVariantID: "file_write", RequiredEffectScope: RuntimeEffectWorkspaceDurable}
	for _, rule := range []RuntimeLineageAssertionRule{index, receipt} {
		if err := rule.Validate(); err != nil {
			t.Fatal(err)
		}
		body, _ := json.Marshal(rule)
		var reloaded RuntimeLineageAssertionRule
		if err := json.Unmarshal(body, &reloaded); err != nil || rule != reloaded || StableRuntimeLineageAssertionRuleHash(rule) != StableRuntimeLineageAssertionRuleHash(reloaded) {
			t.Fatal("versioned rule lost exact identity")
		}
		for name, mutate := range map[string]func(*RuntimeLineageAssertionRule){
			"missing version":     func(r *RuntimeLineageAssertionRule) { r.TerminalContractRef = "" },
			"unknown version":     func(r *RuntimeLineageAssertionRule) { r.TerminalContractRef += "-unknown" },
			"execution downgrade": func(r *RuntimeLineageAssertionRule) { r.ConsumptionPoint = RuntimeProofPhaseExecution },
			"mixed rounds":        func(r *RuntimeLineageAssertionRule) { r.MinCompletedRounds = 1 },
		} {
			t.Run(string(rule.Kind)+"/"+name, func(t *testing.T) {
				changed := rule
				mutate(&changed)
				if changed.Validate() == nil || StableRuntimeLineageAssertionRuleHash(rule) == StableRuntimeLineageAssertionRuleHash(changed) {
					t.Fatal("unsupported/downgraded terminal rule accepted or identity unchanged")
				}
			})
		}
	}
	for _, selector := range []string{"", "*", " final-material", "final-material\n", strings.Repeat("x", 513)} {
		changed := receipt
		changed.ReceiptSourceID = selector
		if changed.Validate() == nil {
			t.Fatal("non-exact receipt source accepted")
		}
	}
	for _, scope := range []string{"test", "unknown", " dodItem"} {
		changed := receipt
		changed.ReceiptSourceScope = scope
		if changed.Validate() == nil {
			t.Fatal("unsupported receipt source scope accepted")
		}
	}
	legacy := RuntimeLineageAssertionRule{Ref: "delivery", Kind: RuntimeLineageDeliveryAcknowledgements, ConsumptionPoint: RuntimeProofPhasePostTerminalDelivery}
	body, _ := json.Marshal(legacy)
	if string(body) != `{"Ref":"delivery","Kind":"delivery_acknowledgements","ConsumptionPoint":"post_terminal_delivery"}` {
		t.Fatal("absent terminal extension changed old rule encoding")
	}
	legacy.TerminalContractRef = RuntimeTerminalReceiptProofContract
	if legacy.Validate() == nil {
		t.Fatal("legacy ACK rule accepted material-extension laundering")
	}
}

func TestRuntimeTerminalResultIndexOriginalProofAndDetachedReload(t *testing.T) {
	source, material := terminalIndexProofFixture(t)
	before, original := source.Clone(), material.CloneRaw()
	for name, mutate := range map[string]func(*RuntimeStateRecord, *RuntimeTerminalResultMaterial){
		"missing inventory": func(s *RuntimeStateRecord, m *RuntimeTerminalResultMaterial) {
			s.Facts.DefinitionSeed.CoverageItems = nil
		},
		"invented DoD": func(s *RuntimeStateRecord, m *RuntimeTerminalResultMaterial) {
			s.Facts.DefinitionSeed.CoverageItems = append(s.Facts.DefinitionSeed.CoverageItems, RuntimeCoverageItem{ID: "other", SourceScope: "dodItem", SourceID: "other", TargetText: "完整义务"})
		},
		"duplicated coverage": func(s *RuntimeStateRecord, m *RuntimeTerminalResultMaterial) {
			s.Facts.Coverage = append(s.Facts.Coverage, s.Facts.Coverage[0])
		},
		"weak fact": func(s *RuntimeStateRecord, m *RuntimeTerminalResultMaterial) {
			s.Facts.Evidence.Accepted[0].ZeroWork = true
		},
		"stale ref": func(s *RuntimeStateRecord, m *RuntimeTerminalResultMaterial) {
			s.Facts.Coverage[0].CurrentBindingRef.BindingHash = "wrong"
		},
		"missing fact": func(s *RuntimeStateRecord, m *RuntimeTerminalResultMaterial) { s.Facts.Evidence.Accepted = nil },
		"missing requirement": func(s *RuntimeStateRecord, m *RuntimeTerminalResultMaterial) {
			m.RuntimeCloseout.RequirementOutcomes = nil
		},
		"duplicate requirement": func(s *RuntimeStateRecord, m *RuntimeTerminalResultMaterial) {
			m.RuntimeCloseout.RequirementOutcomes = append(m.RuntimeCloseout.RequirementOutcomes, m.RuntimeCloseout.RequirementOutcomes[0])
		},
		"missing top index": func(s *RuntimeStateRecord, m *RuntimeTerminalResultMaterial) { m.EvidenceRefs = nil },
		"missing item refs": func(s *RuntimeStateRecord, m *RuntimeTerminalResultMaterial) {
			m.DoDAssessment.Items[0].EvidenceRefs = nil
		},
		"wrong item original": func(s *RuntimeStateRecord, m *RuntimeTerminalResultMaterial) {
			m.DoDAssessment.Items[0].Item = "摘要"
		},
		"duplicate item": func(s *RuntimeStateRecord, m *RuntimeTerminalResultMaterial) {
			m.DoDAssessment.Items = append(m.DoDAssessment.Items, m.DoDAssessment.Items[0])
		},
		"summary omission": func(s *RuntimeStateRecord, m *RuntimeTerminalResultMaterial) { m.SummaryDetail.DoDItems = nil },
	} {
		t.Run(name, func(t *testing.T) {
			changed, body := source.Clone(), material.CloneRaw()
			mutate(&changed, &body)
			closeout := body.RuntimeCloseout.WithCloseoutHash()
			body.RuntimeCloseout = &closeout
			if ValidateRuntimeTerminalProofMaterial(changed, body) == nil {
				t.Fatal("self-rehashed incomplete index accepted")
			}
		})
	}
	encoded, _ := json.Marshal(source)
	var reloaded RuntimeStateRecord
	if err := json.Unmarshal(encoded, &reloaded); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRuntimeTerminalProofMaterial(reloaded, material); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, source) || !reflect.DeepEqual(original, material) {
		t.Fatal("pure terminal proof consumer rewrote authority or original material")
	}
	for _, kind := range []TerminalOutcomeKind{TerminalOutcomeKindFailed, TerminalOutcomeKindCancelled, TerminalOutcomeKindSuperseded} {
		failedSource := source.Clone()
		if kind == TerminalOutcomeKindFailed {
			failedSource.Decision.State = ConvergenceStateFailed
		} else {
			failedSource.Decision.State, failedSource.Decision.Terminal = ConvergenceStateNeedStateChange, nil
			termination := RuntimeTerminationCancelled
			if kind == TerminalOutcomeKindSuperseded {
				termination = RuntimeTerminationSuperseded
			}
			failedSource.Termination = &RuntimeTerminationRecord{Kind: termination, RequestedBy: "user", Reason: "原终止意图", Revision: 1}
		}
		failed, err := DeriveRuntimeTerminalResultMaterialFromState(failedSource)
		if err != nil {
			t.Fatal(err)
		}
		if err := ValidateRuntimeTerminalProofMaterial(failedSource, failed); err != nil {
			t.Fatal("failed-outcome material trapped by success obligation")
		}
		if ValidateRuntimeTerminalProofMaterial(source, failed) == nil {
			t.Fatal("caller's substituted failure outcome bypassed success-material guard")
		}
	}
}

func TestRuntimeTerminalProofSelectorsRequireOriginalInventoryBeforeExecution(t *testing.T) {
	_, binding := lineageSourceFixture(t, RuntimeLineageAssertionRule{Ref: "receipt", Kind: RuntimeLineageTerminalOwnerReceipt, ConsumptionPoint: RuntimeProofPhasePostTerminalDelivery, TerminalContractRef: RuntimeTerminalReceiptProofContract, ReceiptSourceScope: "deliverable", ReceiptSourceID: "snapshot", OwnerModuleID: RuntimeModuleFile, OperationKind: RuntimeBlockKindPatchApply, OperationVariantID: "file_write", RequiredEffectScope: RuntimeEffectWorkspaceDurable})
	profiles := []RuntimeProfileContract{{ID: "profile", EvidenceRequirements: []RuntimeProfileEvidenceRequirement{{LineageAssertions: []RuntimeLineageAssertionRule{binding.LineageAssertions[0].Rule}}}}}
	item := RuntimeCoverageItem{ID: common.CoverageWorkItemID("snapshot-cwi"), SourceScope: "deliverable", SourceID: "snapshot", TargetText: "完整原交付准则"}
	if err := ValidateRuntimeTerminalProofDeclarationSources([]RuntimeCoverageItem{item}, profiles); err != nil {
		t.Fatal(err)
	}
	for _, items := range [][]RuntimeCoverageItem{nil, {{ID: "foreign", SourceScope: "deliverable", SourceID: "other"}}, {item, item}} {
		if ValidateRuntimeTerminalProofDeclarationSources(items, profiles) == nil {
			t.Fatal("missing/foreign/ambiguous source accepted")
		}
	}
	for _, mutate := range []func(*RuntimeCoverageItem){
		func(item *RuntimeCoverageItem) { item.ID = "" },
		func(item *RuntimeCoverageItem) { item.TargetText = "" },
	} {
		incomplete := item
		mutate(&incomplete)
		if ValidateRuntimeTerminalProofDeclarationSources([]RuntimeCoverageItem{incomplete}, profiles) == nil {
			t.Fatal("selector resolved an incomplete original source preimage")
		}
	}
}
