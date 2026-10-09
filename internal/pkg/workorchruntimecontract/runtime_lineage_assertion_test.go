package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"
)

func lineageSourceFixture(t *testing.T, rule RuntimeLineageAssertionRule) (RuntimeLineageAssertionSources, CoverageEvidenceBinding) {
	t.Helper()
	item := RuntimeCoverageItem{ID: "cwi-1", SourceScope: "dodItem", SourceID: "requirement-1", TargetText: "完整封存要求\n含第二行，不得摘要"}
	profiles := []RuntimeProfileContract{{ID: "profile", EvidenceRequirements: []RuntimeProfileEvidenceRequirement{{
		ID: item.SourceID, Scope: item.SourceScope, RequiredEvidenceKinds: []string{EvidenceKindStateChange},
		AcceptedStepKinds: []string{StepKindFileWrite}, LineageAssertions: []RuntimeLineageAssertionRule{rule},
	}}}}
	declaration := CanonicalRuntimeDefinitionEvidenceDeclarationRef(item.SourceScope, item.SourceID)
	assertions, err := CompileRuntimeLineageAssertions([]RuntimeLineageAssertionRule{rule}, declaration, item)
	if err != nil {
		t.Fatal(err)
	}
	binding := CoverageEvidenceBinding{
		ID: "binding-1", CoverageWorkItemID: item.ID, FocusID: "focus-1", DefinitionRevision: "definition-1",
		DeclarationRef: declaration, TargetRefs: []string{"artifact.txt"}, LineageAssertions: assertions,
		Proofs: []CoverageEvidenceProof{{EvidenceKind: EvidenceKindStateChange, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, AllowedOperationKinds: []string{StepKindFileWrite}}},
	}
	binding.Hash = StableCoverageEvidenceBindingHash(binding)
	sources := RuntimeLineageAssertionSources{}
	sources.Facts.DefinitionSeed.CoverageItems = []RuntimeCoverageItem{item}
	sources.Facts.Activation.ExecutionContract.ProfileContracts = profiles
	return sources, binding
}

func TestRuntimeLineageAssertionsExactSourceAndOptInBoundary(t *testing.T) {
	rule := RuntimeLineageAssertionRule{Ref: "delivery", Kind: RuntimeLineageDeliveryAcknowledgements, ConsumptionPoint: RuntimeProofPhasePostTerminalDelivery}
	sources, binding := lineageSourceFixture(t, rule)
	if err := binding.Validate(); err != nil {
		t.Fatal(err)
	}
	sources.Definition.StartupInputHash = "sealed-startup"
	if !RuntimeExecutionLineageAssertionsSatisfied(binding, sources, "") {
		t.Fatal("execution 不能要求预先完成 post-terminal delivery")
	}
	if expected, err := runtimeExpectedLineageAssertions(binding, sources); err != nil || !reflect.DeepEqual(expected, binding.LineageAssertions) {
		t.Fatalf("完整来源不能重编: %v", err)
	}
	for name, mutate := range map[string]func(*CoverageEvidenceBinding, *RuntimeLineageAssertionSources){
		"stripped assertions": func(b *CoverageEvidenceBinding, s *RuntimeLineageAssertionSources) { b.LineageAssertions = nil },
		"invented assertions": func(b *CoverageEvidenceBinding, s *RuntimeLineageAssertionSources) { s.Facts.Activation.ExecutionContract.ProfileContracts[0].EvidenceRequirements[0].LineageAssertions = nil },
		"summarized text": func(b *CoverageEvidenceBinding, s *RuntimeLineageAssertionSources) { s.Facts.DefinitionSeed.CoverageItems[0].TargetText = "完整封存要求" },
		"different CWI": func(b *CoverageEvidenceBinding, s *RuntimeLineageAssertionSources) { b.CoverageWorkItemID = "cwi-other" },
		"changed declaration": func(b *CoverageEvidenceBinding, s *RuntimeLineageAssertionSources) {
			b.DeclarationRef = "invented-declaration"
			b.LineageAssertions[0].DeclarationRef = b.DeclarationRef
		},
		"coordinated rule hash": func(b *CoverageEvidenceBinding, s *RuntimeLineageAssertionSources) {
			b.LineageAssertions[0].Rule.Ref = "changed"
			b.LineageAssertions[0].RuleHash = StableRuntimeLineageAssertionRuleHash(b.LineageAssertions[0].Rule)
		},
	} {
		t.Run(name, func(t *testing.T) {
			copySources, copyBinding := lineageSourceFixture(t, rule)
			mutate(&copyBinding, &copySources)
			copyBinding.Hash = StableCoverageEvidenceBindingHash(copyBinding)
			if _, err := runtimeExpectedLineageAssertions(copyBinding, copySources); err == nil {
				t.Fatal("来源漂移或单空 guard 被接受")
			}
		})
	}
	// 另一 source 有规则，不要求无规则当前 CWI 的完整文本或 guard ledger。
	sources.Facts.DefinitionSeed.CoverageItems = nil
	sources.Facts.Coverage = []RuntimeCoverageItemFact{{ID: "cwi-free", SourceScope: "dodItem", SourceID: "unrelated"}}
	binding.CoverageWorkItemID, binding.LineageAssertions = "cwi-free", nil
	if !RuntimeExecutionLineageAssertionsSatisfied(binding, sources, "") {
		t.Fatal("无关 source 的规则收紧了未 opt-in 的 CWI")
	}
	if RuntimeLineageSourceTextHash("text") == RuntimeLineageSourceTextHash(" text ") {
		t.Fatal("source hash 不应规范化完整封存文本")
	}
}

func TestRuntimeLineageRulesMergeHashAndRawClone(t *testing.T) {
	exact := RuntimeLineageAssertionRule{Ref: "exact", Kind: RuntimeLineageCompletedWorkRounds, ConsumptionPoint: RuntimeProofPhaseExecution, MinCompletedRounds: 2}
	wildcard := RuntimeLineageAssertionRule{Ref: "delivery", Kind: RuntimeLineageDeliveryAcknowledgements, ConsumptionPoint: RuntimeProofPhasePostTerminalDelivery}
	profiles := []RuntimeProfileContract{{ID: "profile", EvidenceRequirements: []RuntimeProfileEvidenceRequirement{
		{ID: "requirement-1", Scope: "dodItem", RequiredEvidenceKinds: []string{EvidenceKindStateChange}, LineageAssertions: []RuntimeLineageAssertionRule{exact}},
		{ID: "*", Scope: "dodItem", RequiredEvidenceKinds: []string{EvidenceKindStateChange}, LineageAssertions: []RuntimeLineageAssertionRule{wildcard}},
	}}}
	policy, err := CompileRuntimeSourceEvidencePolicy("dodItem", "requirement-1", profiles, nil)
	if err != nil || !reflect.DeepEqual(policy.LineageAssertions, []RuntimeLineageAssertionRule{exact, wildcard}) {
		t.Fatalf("exact/wildcard 合取声明丢失: %v %+v", err, policy)
	}
	invalidProfile := NormalizeRuntimeProfileContracts(profiles)
	invalidProfile[0].EvidenceRequirements[0].RequiredEvidenceKinds = nil
	if err := ValidateRuntimeProfileContracts(invalidProfile); err == nil { t.Fatal("guard 不能替代普通 evidence proof") }
	focuses := []RuntimeDefinitionFocus{{AssessmentScope: "dodItem", AssessmentID: "requirement-1", LineageAssertions: policy.LineageAssertions}}
	if err := ValidateRuntimeDefinitionLineageAssertions(focuses, profiles); err != nil { t.Fatal(err) }
	focuses[0].LineageAssertions = nil
	if err := ValidateRuntimeDefinitionLineageAssertions(focuses, profiles); err == nil { t.Fatal("Definition guard 被剥空后未拒绝") }
	conflicting := NormalizeRuntimeProfileContracts(profiles)
	conflicting[0].EvidenceRequirements[0].LineageAssertions = []RuntimeLineageAssertionRule{exact}
	conflicting[0].EvidenceRequirements[0].LineageAssertions[0].MinCompletedRounds++
	conflicting[0].EvidenceRequirements[1].LineageAssertions = []RuntimeLineageAssertionRule{exact}
	if _, err := CompileRuntimeSourceEvidencePolicy("dodItem", "requirement-1", conflicting, nil); err == nil {
		t.Fatal("同 Ref 不同规则的 exact/wildcard 冲突被接受")
	}
	sources, binding := lineageSourceFixture(t, wildcard)
	record := RuntimeStateRecord{Facts: sources.Facts}
	record.Facts.Focus.CoverageEvidenceBindings = []CoverageEvidenceBinding{binding}
	record.Facts.DefinitionFocuses = []RuntimeDefinitionFocus{{LineageAssertions: []RuntimeLineageAssertionRule{exact}}}
	cloned := record.Clone()
	if !reflect.DeepEqual(cloned, record) { t.Fatal("raw clone 规范化了规则来源") }
	cloned.Facts.Focus.CoverageEvidenceBindings[0].LineageAssertions[0].Rule.Ref = "changed"
	cloned.Facts.DefinitionFocuses[0].LineageAssertions[0].Ref = "changed"
	cloned.Facts.Activation.ExecutionContract.ProfileContracts[0].EvidenceRequirements[0].LineageAssertions[0].Ref = "changed"
	if record.Facts.Focus.CoverageEvidenceBindings[0].LineageAssertions[0].Rule.Ref != wildcard.Ref || record.Facts.DefinitionFocuses[0].LineageAssertions[0].Ref != exact.Ref || record.Facts.Activation.ExecutionContract.ProfileContracts[0].EvidenceRequirements[0].LineageAssertions[0].Ref != wildcard.Ref {
		t.Fatal("规则或断言 raw clone 共享原切片")
	}
	decoded, err := json.Marshal(record)
	if err != nil { t.Fatal(err) }
	var reloaded RuntimeStateRecord
	if err := json.Unmarshal(decoded, &reloaded); err != nil { t.Fatal(err) }
	if _, err := runtimeExpectedLineageAssertions(reloaded.Facts.Focus.CoverageEvidenceBindings[0], RuntimeLineageAssertionSources{Facts: reloaded.Facts}); err != nil { t.Fatalf("序列化丢失 exact guard 来源: %v", err) }
	changed := binding.Clone()
	changed.LineageAssertions[0].SourceTextHash = RuntimeLineageSourceTextHash("another exact source")
	if StableCoverageEvidenceBindingHash(changed) == binding.Hash { t.Fatal("binding hash 未封存 full source hash") }
}

func TestRuntimeLineageAssertionsInventoryAndCanonicalCutoff(t *testing.T) {
	valid := RuntimeLineageAssertionRule{Ref: "receipt", Kind: RuntimeLineageOwnerOperationReceipt, ConsumptionPoint: RuntimeProofPhaseExecution,
		OwnerModuleID: RuntimeModuleFile, OperationKind: RuntimeBlockKindPatchApply, OperationVariantID: "file_write", RequiredEffectScope: RuntimeEffectWorkspaceDurable}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*RuntimeLineageAssertionRule){
		func(r *RuntimeLineageAssertionRule) { r.OwnerModuleID = RuntimeModuleShell },
		func(r *RuntimeLineageAssertionRule) { r.OperationVariantID = "unknown" },
		func(r *RuntimeLineageAssertionRule) { r.RequiredEffectScope = RuntimeEffectNone },
		func(r *RuntimeLineageAssertionRule) { r.ConsumptionPoint = RuntimeProofPhasePostTerminalDelivery },
	} {
		invalid := valid
		mutate(&invalid)
		if invalid.Validate() == nil {
			t.Fatal("无效 owner/kind/variant/effect 库存被接受")
		}
	}
	for _, pair := range [][2]uint64{{9, 10}, {99, 100}} {
		before, after := runtimeEvidenceRevisionForSequence(pair[0]), runtimeEvidenceRevisionForSequence(pair[1])
		if !runtimeCanonicalLineageCutoff(before) || !runtimeCanonicalLineageCutoff(after) || !EvidenceRevisionAfter(after, before) || EvidenceRevisionAfter(before, after) {
			t.Fatalf("canonical cutoff 比较失败: %q %q", before, after)
		}
	}
	for _, revision := range []string{"", "runtime-evidence-9", "runtime-evidence-000000000000000000+9", "runtime-evidence-18446744073709551616"} {
		if runtimeCanonicalLineageCutoff(revision) {
			t.Fatalf("非 canonical cutoff 被接受: %q", revision)
		}
	}
}

func deliveryLineageSourceFixture(t *testing.T) (RuntimeStateRecord, RuntimeDeliveryLineageBinding) {
	t.Helper()
	rule := RuntimeLineageAssertionRule{Ref: "delivery", Kind: RuntimeLineageDeliveryAcknowledgements, ConsumptionPoint: RuntimeProofPhasePostTerminalDelivery}
	sources, _ := lineageSourceFixture(t, rule)
	source := terminalResultCompilerSource()
	source.Facts.DefinitionSeed.CoverageItems = sources.Facts.DefinitionSeed.CoverageItems
	source.Facts.Activation.ExecutionContract.ProfileContracts = sources.Facts.Activation.ExecutionContract.ProfileContracts
	source.Decision.State, source.Decision.Terminal = ConvergenceStateNeedStateChange, nil
	bindings, err := RuntimeDeliveryLineageBindingsFromState(source)
	if err != nil || len(bindings) != 1 {
		t.Fatalf("delivery source 编译失败: %v %+v", err, bindings)
	}
	return source, bindings[0]
}

func TestRuntimeDeliveryLineageNotApplicableAndSourceDrift(t *testing.T) {
	source, binding := deliveryLineageSourceFixture(t)
	missingInventory := source.Clone()
	missingInventory.Facts.DefinitionSeed.CoverageItems = nil
	if _, err := RuntimeDeliveryLineageBindingsFromState(missingInventory); err == nil { t.Fatal("delivery declaration 缺封存 CWI inventory 未拒绝") }
	before := source.Clone()
	for _, mutate := range []func(*RuntimeDeliveryLineageBinding){
		func(b *RuntimeDeliveryLineageBinding) { b.Assertions = nil },
		func(b *RuntimeDeliveryLineageBinding) { b.DeclarationRef = "another-source" },
		func(b *RuntimeDeliveryLineageBinding) { b.Assertions[0].SourceTextHash = RuntimeLineageSourceTextHash("摘要") },
	} {
		copyBinding := binding
		copyBinding.Assertions = append([]RuntimeLineageAssertion(nil), binding.Assertions...)
		mutate(&copyBinding)
		if _, err := EvaluateRuntimeDeliveryLineageAssertionFromState(source, copyBinding); err == nil {
			t.Fatal("delivery 单空或来源漂移被接受")
		}
	}
	if !reflect.DeepEqual(before, source) {
		t.Fatal("来源反证修改了封存材料")
	}
	source.Facts.Activation.ExecutionContract.ProfileContracts[0].EvidenceRequirements[0].LineageAssertions = nil
	binding.Assertions = nil
	if state, err := EvaluateRuntimeDeliveryLineageAssertionFromState(source, binding); err != nil || state != RuntimeDeliveryLineageNotApplicable {
		t.Fatalf("未声明 delivery guard 应不适用: %s %v", state, err)
	}
	if bindings, err := RuntimeDeliveryLineageBindingsFromState(source); err != nil || len(bindings) != 0 {
		t.Fatalf("未声明 source 生成了 delivery guard: %v %+v", err, bindings)
	}
}

func TestRuntimeDeliveryLineageDeferredPendingExactSealAndRestart(t *testing.T) {
	source, binding := deliveryLineageSourceFixture(t)
	check := func(want RuntimeDeliveryLineageState) {
		t.Helper()
		before := source.Clone()
		got, err := EvaluateRuntimeDeliveryLineageAssertionFromState(source, binding)
		if err != nil || got != want || !reflect.DeepEqual(before, source) {
			t.Fatalf("delivery state=%s want=%s err=%v", got, want, err)
		}
	}
	check(RuntimeDeliveryLineageDeferred)
	source.Decision.State = ConvergenceStateFailed
	source.Decision.Terminal = &RuntimeTerminalDecision{Code: "bounded_failure", Summary: "有界终态"}
	source.Decision.AuthoritySnapshot.WorkspaceID, source.Decision.AuthoritySnapshot.RunID = source.Facts.WorkspaceID, source.Facts.RunID
	source.Decision.AuthoritySnapshot.DefinitionRevision, source.Decision.AuthoritySnapshot.EvidenceRevision = source.Facts.DefinitionRevision, source.Facts.EvidenceRevision
	check(RuntimeDeliveryLineagePending)
	material, err := DeriveRuntimeTerminalResultMaterialFromState(source)
	if err != nil {
		t.Fatal(err)
	}
	outcome, _, _ := DeriveRuntimeTerminalOutcome(source.Decision, nil)
	identity := RuntimeTerminalDeliveryIdentity{TerminalDeliveryID: StableTerminalDeliveryID(source.Facts.WorkspaceID, source.Facts.RunID, source.Decision.AuthoritySnapshot.ID, outcome),
		WorkspaceID: source.Facts.WorkspaceID, RunID: source.Facts.RunID, AuthoritySnapshotID: source.Decision.AuthoritySnapshot.ID,
		DefinitionRevision: source.Facts.DefinitionRevision, EvidenceRevision: source.Facts.EvidenceRevision, DispatchID: source.Decision.AuthoritySnapshot.ID, Outcome: outcome}
	hash, err := material.CanonicalHash()
	if err != nil {
		t.Fatal(err)
	}
	source.TerminalMaterialSeal = &RuntimeTerminalMaterialSeal{Identity: identity, GenerationStateRevision: source.StateRevision,
		RecoveryPolicy: source.Facts.RecoveryPolicy, RecoveryLedger: source.Facts.Recovery, Material: material, MaterialHash: hash}
	check(RuntimeDeliveryLineagePending)
	for index, owner := range []RuntimeDeliveryOwner{RuntimeDeliveryOwnerResult, RuntimeDeliveryOwnerWorkspace, RuntimeDeliveryOwnerRun} {
		ack := RuntimeDeliveryAcknowledgement{WorkspaceID: identity.WorkspaceID, RunID: identity.RunID, TerminalDeliveryID: identity.TerminalDeliveryID,
			Owner: owner, AuthoritySnapshotID: identity.AuthoritySnapshotID, CausationID: "terminal-delivery/" + identity.TerminalDeliveryID, Outcome: outcome}
		switch owner {
		case RuntimeDeliveryOwnerResult:
			ack.ResultID, ack.AcknowledgedState = RuntimeTerminalResultID(identity.WorkspaceID, identity.RunID), RuntimeDeliveryAcknowledgedResultRecorded
			body := material.RuntimeCloseout.CloneRaw()
			ack.ResultCloseout = &body
		case RuntimeDeliveryOwnerWorkspace:
			ack.AcknowledgedState = RuntimeDeliveryAcknowledgedWorkspaceReleased
		case RuntimeDeliveryOwnerRun:
			ack.AcknowledgedState = RuntimeDeliveryAcknowledgedRunTerminal
		}
		source.DeliveryAcknowledgements = append(source.DeliveryAcknowledgements, ack)
		source.StateRevision++
		want := RuntimeDeliveryLineagePending
		if index == 2 { want = RuntimeDeliveryLineageSatisfied }
		check(want)
	}
	encoded, err := json.Marshal(source)
	if err != nil { t.Fatal(err) }
	var reloaded RuntimeStateRecord
	if err := json.Unmarshal(encoded, &reloaded); err != nil { t.Fatal(err) }
	if got, err := EvaluateRuntimeDeliveryLineageAssertionFromState(reloaded, binding); err != nil || got != RuntimeDeliveryLineageSatisfied {
		t.Fatalf("restart 丢失原 seal 满足证明: %s %v", got, err)
	}
	for _, mutate := range []func(*RuntimeStateRecord){
		func(s *RuntimeStateRecord) { s.TerminalMaterialSeal = nil },
		func(s *RuntimeStateRecord) { s.DeliveryAcknowledgements[0].ResultCloseout = nil },
		func(s *RuntimeStateRecord) { s.DeliveryAcknowledgements[0], s.DeliveryAcknowledgements[1] = s.DeliveryAcknowledgements[1], s.DeliveryAcknowledgements[0] },
		func(s *RuntimeStateRecord) { s.DeliveryAcknowledgements = append(s.DeliveryAcknowledgements, s.DeliveryAcknowledgements[2]) },
		func(s *RuntimeStateRecord) { s.DeliveryAcknowledgements[0].ResultCloseout.RuntimeStateRevision++; body := s.DeliveryAcknowledgements[0].ResultCloseout.WithCloseoutHash(); s.DeliveryAcknowledgements[0].ResultCloseout = &body },
	} {
		invalid := source.Clone()
		mutate(&invalid)
		if state, err := EvaluateRuntimeDeliveryLineageAssertionFromState(invalid, binding); err == nil || state == RuntimeDeliveryLineageSatisfied {
			t.Fatal("损坏 seal 或 ledger 被视为 delivery 满足")
		}
	}
	check(RuntimeDeliveryLineageSatisfied)
}
