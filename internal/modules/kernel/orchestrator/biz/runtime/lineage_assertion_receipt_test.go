package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/semanticplan"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// 此单元只检验 owner_operation_receipt；typed owner fact 不冒充真实 File I/O。
// 正例经生产 SM-2/SM-3 私有 semantic/candidate/claim/receipt/evidence 路径，
// 每个持久化切点均经过完整 aggregate gate，不用成功 hash 或 noop 代替来源。
func TestRuntimeLineageOwnerReceiptLegalAggregateAndCanonicalCutoff(t *testing.T) {
	controller, repository, sealed, binding, otherBinding := lineageReceiptSealedAggregate(t)
	claimed, receipt := lineageReceiptClaim(t, controller, sealed)
	before := claimed.Facts.EvidenceRevision
	ownerCommitted := lineageReceiptObserve(t, controller, claimed, receipt)
	if runtimecontract.RuntimeExecutionLineageAssertionsSatisfied(binding, runtimeLineageAssertionSources(ownerCommitted), before) {
		t.Fatal("仅 owner receipt、未提交证据不能满足 guard")
	}
	evidence := lineageReceiptEvidence(t, ownerCommitted, receipt)
	result, err := controller.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{
		Kind: RuntimeInternalEventOperationEvidenceObserved,
		Scope: lineageReceiptScope(ownerCommitted),
		Transaction: &RuntimeInternalTransactionPayload{ExecutionEvidence: &evidence},
	})
	if err != nil {
		t.Fatalf("生产 receipt-to-evidence commit: %v", err)
	}
	aggregate := result.Aggregate
	if err := aggregate.Validate(); err != nil {
		t.Fatalf("正例完整 aggregate: %v", err)
	}
	if aggregate.ActiveDispatch != nil || len(aggregate.CompletedDispatches) != 1 || aggregate.CompletedDispatches[0].State != DispatchStateCommitted || len(aggregate.Facts.Evidence.Accepted) != 1 {
		t.Fatal("正例没有形成完整 committed dispatch/accepted evidence 骨架")
	}
	lineageReceiptAssertOriginal(t, sealed, aggregate, receipt)
	for _, test := range []struct {
		name string
		cutoff string
		want bool
	}{
		{"before_receipt", before, false},
		{"inclusive_receipt", aggregate.Facts.EvidenceRevision, true},
		{"future_cutoff", "runtime-evidence-00000000000000000002", false},
		{"noncanonical", "runtime-evidence-1", false},
		{"empty", "", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			original := aggregate.CloneRaw()
			got := runtimecontract.RuntimeExecutionLineageAssertionsSatisfied(binding, runtimeLineageAssertionSources(aggregate), test.cutoff)
			if got != test.want || !reflect.DeepEqual(original, aggregate) {
				t.Fatalf("cutoff=%q guard=%t want=%t，或只读查询改写来源", test.cutoff, got, test.want)
			}
		})
	}
	if runtimecontract.RuntimeExecutionLineageAssertionsSatisfied(otherBinding, runtimeLineageAssertionSources(aggregate), aggregate.Facts.EvidenceRevision) {
		t.Fatal("同 Profile、同 owner/kind/effect 的另一个原始 CWI 借用了当前 receipt")
	}
	encoded, err := json.Marshal(aggregate)
	if err != nil { t.Fatal(err) }
	var reloaded RuntimeAggregate
	if err := json.Unmarshal(encoded, &reloaded); err != nil { t.Fatal(err) }
	if err := reloaded.Validate(); err != nil { t.Fatal(err) }
	if !runtimecontract.RuntimeExecutionLineageAssertionsSatisfied(binding, runtimeLineageAssertionSources(reloaded), reloaded.Facts.EvidenceRevision) {
		t.Fatal("同版本 JSON fresh-load 丢失原始 receipt lineage")
	}
	reopened, err := NewRuntimeController(repository)
	if err != nil { t.Fatal(err) }
	saves := repository.saves
	duplicate := lineageReceiptObserve(t, reopened, aggregate, receipt)
	if repository.saves != saves || !reflect.DeepEqual(duplicate, aggregate) {
		t.Fatal("duplicate receipt 改写 aggregate 或重复消费预算")
	}
	replay, err := reopened.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{
		Kind: RuntimeInternalEventOperationEvidenceObserved, Scope: lineageReceiptScope(aggregate),
		Transaction: &RuntimeInternalTransactionPayload{ExecutionEvidence: &evidence},
	})
	if err != nil || replay.Evidence == nil || replay.Evidence.Disposition != CommitDispositionDuplicate || repository.saves != saves || !reflect.DeepEqual(replay.Aggregate, aggregate) {
		t.Fatalf("duplicate evidence 非幂等: %v", err)
	}
	if !runtimecontract.RuntimeExecutionLineageAssertionsSatisfied(binding, runtimeLineageAssertionSources(replay.Aggregate), replay.Aggregate.Facts.EvidenceRevision) {
		t.Fatal("合法 duplicate replay 丢失唯一原始证明")
	}
	lineageReceiptTamperMatrix(t, aggregate, binding)
}

func TestRuntimeLineageOwnerReceiptCASFailureAndResponseLoss(t *testing.T) {
	for _, afterCommit := range []bool{false, true} {
		t.Run(map[bool]string{false: "before_commit", true: "after_commit_reply_loss"}[afterCommit], func(t *testing.T) {
			controller, _, sealed, binding, _ := lineageReceiptSealedAggregate(t)
			claimed, receipt := lineageReceiptClaim(t, controller, sealed)
			baselineValue := claimed.CloneRaw()
			baselineRepository := &controllerRepository{aggregate: &baselineValue}
			baselineController, err := NewRuntimeController(baselineRepository)
			if err != nil { t.Fatal(err) }
			baseline := lineageReceiptObserve(t, baselineController, baselineValue, receipt)
			repository := &implementationWorkDetachedRepository{controllerRepository: controllerRepository{aggregate: &claimed}, failBefore: !afterCommit, failAfter: afterCommit}
			controller, err = NewRuntimeController(repository)
			if err != nil { t.Fatal(err) }
			if _, err := controller.ApplyInternalEvent(context.Background(), lineageReceiptEvent(claimed, receipt)); err == nil { t.Fatal("未保留原 CAS 故障") }
			persisted, err := repository.Load(context.Background(), claimed.Facts.WorkspaceID, claimed.Facts.RunID)
			if err != nil { t.Fatal(err) }
			if err := persisted.Validate(); err != nil { t.Fatal(err) }
			if !afterCommit && !reflect.DeepEqual(persisted, claimed) { t.Fatal("CAS 前失败污染持久前像") }
			if afterCommit && (persisted.StateRevision != claimed.StateRevision+1 || len(persisted.OperationReceipts) != 1) { t.Fatal("响应丢失未保留唯一原 receipt CAS") }
			if persisted.Facts.Recovery != claimed.Facts.Recovery || (afterCommit && !reflect.DeepEqual(persisted.ExecutionBudget, baseline.ExecutionBudget)) { t.Fatal("持久化故障改变 recovery 或原正常 receipt settlement 预算") }
			budgetBeforeReplay := persisted.ExecutionBudget
			restarted, err := NewRuntimeController(repository)
			if err != nil { t.Fatal(err) }
			committed := lineageReceiptObserve(t, restarted, persisted, receipt)
			if repository.saves != 1 || committed.StateRevision != claimed.StateRevision+1 || !reflect.DeepEqual(committed.ExecutionBudget, baseline.ExecutionBudget) ||
				(afterCommit && !reflect.DeepEqual(committed.ExecutionBudget, budgetBeforeReplay)) { t.Fatal("重启重复消费 owner receipt CAS 或正常 settlement 预算") }
			evidence := lineageReceiptEvidence(t, committed, receipt)
			result, err := restarted.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{Kind: RuntimeInternalEventOperationEvidenceObserved, Scope: lineageReceiptScope(committed), Transaction: &RuntimeInternalTransactionPayload{ExecutionEvidence: &evidence}})
			if err != nil { t.Fatal(err) }
			if err := result.Aggregate.Validate(); err != nil { t.Fatal(err) }
			if !runtimecontract.RuntimeExecutionLineageAssertionsSatisfied(binding, runtimeLineageAssertionSources(result.Aggregate), result.Aggregate.Facts.EvidenceRevision) { t.Fatal("合法 CAS 故障重启丢失 exact receipt guard") }
			if runtimecontract.RuntimeExecutionLineageAssertionsSatisfied(binding, runtimeLineageAssertionSources(result.Aggregate), claimed.Facts.EvidenceRevision) { t.Fatal("重启未来回执补进原 cutoff") }
			lineageReceiptAssertOriginal(t, sealed, result.Aggregate, receipt)
		})
	}
}

func TestRuntimeLineageOwnerReceiptNoOpNoChangeAndStale(t *testing.T) {
	for _, mode := range []string{"noop", "no_change", "equal_before_after", "missing_after", "stale_fence", "prerequisite_stale"} {
		t.Run(mode, func(t *testing.T) {
			controller, repository, sealed, binding, _ := lineageReceiptSealedAggregate(t)
			claimed, receipt := lineageReceiptClaim(t, controller, sealed)
			switch mode {
			case "noop":
				receipt.Outcome, receipt.ObservedEffectScope, receipt.Changed = runtimecontract.RuntimeOperationOutcomeNoOp, runtimecontract.RuntimeEffectNone, false
				receipt.AfterHash = receipt.BeforeHash
			case "no_change":
				receipt.Changed = false
			case "equal_before_after":
				receipt.AfterHash = receipt.BeforeHash
			case "missing_after":
				receipt.AfterHash = ""
			case "stale_fence":
				bad := receipt.Clone()
				bad.Fence++
				saves := repository.saves
				_, err := controller.ApplyInternalEvent(context.Background(), lineageReceiptEvent(claimed, bad))
				if !errors.Is(err, common.ErrConflict) || repository.saves != saves || !reflect.DeepEqual(*repository.aggregate, claimed) {
					t.Fatalf("stale fence 未零写入拒绝: %v", err)
				}
				if runtimecontract.RuntimeExecutionLineageAssertionsSatisfied(binding, runtimeLineageAssertionSources(claimed), claimed.Facts.EvidenceRevision) {
					t.Fatal("stale fence 获得 proof")
				}
				return
			case "prerequisite_stale":
				observation := claimed.Facts.Resources.PrerequisiteObservations[0]
				dependency := observation.Dependencies[0]
				receipt.Outcome, receipt.ObservedEffectScope, receipt.Changed = runtimecontract.RuntimeOperationOutcomeRejected, runtimecontract.RuntimeEffectNone, false
				receipt.AfterHash = receipt.BeforeHash
				receipt.PrerequisiteStale = &runtimecontract.RuntimePrerequisiteStale{
					PrerequisiteID: observation.PrerequisiteID, ObservationID: observation.ObservationID,
					Domain: dependency.Domain, OwnerModuleID: dependency.OwnerModuleID, SubjectRef: dependency.SubjectRef,
					ExpectedRevision: dependency.Revision, ObservedRevision: "file-revision-2", Reason: "原始 owner 观察到目录 revision 已改变",
				}
				receipt.Feedback = runtimeOperationFeedbackForTest(t, claimed, receipt.OperationID, runtimecontract.FailureFact{
					Class: runtimecontract.FailureClassExecutionFailure, Code: "prerequisite_stale", Signature: "lineage-receipt/stale",
					Message: "原始 owner 观察到目录 revision 已改变",
				})
			}
			if err := receipt.Validate(); err != nil { t.Fatalf("typed %s receipt: %v", mode, err) }
			ownerCommitted := lineageReceiptObserve(t, controller, claimed, receipt)
			if mode == "prerequisite_stale" {
				result, err := controller.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{
					Kind: RuntimeInternalEventOperationPrerequisiteStaleObserved, Scope: lineageReceiptScope(ownerCommitted),
					Transaction: &RuntimeInternalTransactionPayload{DispatchID: claimed.ActiveDispatch.ID, OperationPrerequisiteStaleReceipt: &receipt},
				})
				if err != nil { t.Fatalf("生产 stale invalidation: %v", err) }
				if err := result.Aggregate.Validate(); err != nil { t.Fatal(err) }
				if len(result.Aggregate.Facts.Evidence.Accepted) != 0 || len(result.Aggregate.Facts.Evidence.Rejected) != 0 || result.Aggregate.ActiveDispatch != nil || len(result.Aggregate.CompletedDispatches) != 1 || result.Aggregate.CompletedDispatches[0].State != DispatchStateInvalidated {
					t.Fatal("stale 没有保留无 credit 的 invalidated 原始 transaction")
				}
				if runtimecontract.RuntimeExecutionLineageAssertionsSatisfied(binding, runtimeLineageAssertionSources(result.Aggregate), result.Aggregate.Facts.EvidenceRevision) { t.Fatal("stale receipt 获得 proof") }
				return
			}
			evidence := lineageReceiptEvidence(t, ownerCommitted, receipt)
			result, err := controller.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{
				Kind: RuntimeInternalEventOperationEvidenceObserved, Scope: lineageReceiptScope(ownerCommitted),
				Transaction: &RuntimeInternalTransactionPayload{ExecutionEvidence: &evidence},
			})
			if err != nil { t.Fatalf("生产 %s evidence commit: %v", mode, err) }
			if err := result.Aggregate.Validate(); err != nil { t.Fatal(err) }
			if runtimecontract.RuntimeExecutionLineageAssertionsSatisfied(binding, runtimeLineageAssertionSources(result.Aggregate), result.Aggregate.Facts.EvidenceRevision) {
				t.Fatalf("%s 被当作 durable receipt proof", mode)
			}
			persisted, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(result.Aggregate.OperationReceipts, receipt.ExecutionKey, receipt.Fence)
			if !found || !reflect.DeepEqual(persisted, receipt) { t.Fatal("非证明 outcome 改写了 owner 原文") }
		})
	}
}

func lineageReceiptSealedAggregate(t *testing.T) (*RuntimeController, *controllerRepository, RuntimeAggregate, runtimecontract.CoverageEvidenceBinding, runtimecontract.CoverageEvidenceBinding) {
	t.Helper()
	rule := runtimecontract.RuntimeLineageAssertionRule{
		Ref: "exact-file-receipt", Kind: runtimecontract.RuntimeLineageOwnerOperationReceipt, ConsumptionPoint: runtimecontract.RuntimeProofPhaseExecution,
		OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKind: runtimecontract.StepKindFileWrite, RequiredEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable,
	}
	return lineageReceiptSealedAggregateWithRule(t, rule)
}

func lineageReceiptSealedAggregateWithRule(t *testing.T, rule runtimecontract.RuntimeLineageAssertionRule, beforeSeal ...func(RuntimeAggregate, RuntimeInternalEvent)) (*RuntimeController, *controllerRepository, RuntimeAggregate, runtimecontract.CoverageEvidenceBinding, runtimecontract.CoverageEvidenceBinding) {
	t.Helper()
	facts := controllerFacts()
	if err := rule.Validate(); err != nil { t.Fatal(err) }
	const firstText = "原始准则：File owner 对 target.txt 产生持久改变；完整 receipt 必须可反查。\n第二行保留原文。"
	const otherText = "另一原始准则：File owner 对 other.txt 产生独立持久改变，不能借用 target.txt 回执。"
	facts.Coverage[0].TargetText = firstText
	facts.DefinitionSeed.CoverageItems[0].TargetText = firstText
	otherItem := runtimecontract.RuntimeCoverageItem{ID: "cwi-other", SliceID: "slice-other", SourceScope: "dodItem", SourceID: "dod-other", TargetText: otherText}
	facts.DefinitionSeed.CoverageItems = append(facts.DefinitionSeed.CoverageItems, otherItem)
	facts.Slices = append(facts.Slices, runtimecontract.RuntimeAcceptanceSlice{ID: otherItem.SliceID, CoverageWorkItemIDs: []common.CoverageWorkItemID{otherItem.ID}})
	facts.DefinitionSeed.Slices = facts.Slices
	facts.DefinitionSeed.Hash = runtimecontract.StableRuntimeDefinitionSeedHash(facts.DefinitionSeed)
	facts.Coverage = append(facts.Coverage, runtimecontract.RuntimeCoverageItemFact{ID: otherItem.ID, SliceID: otherItem.SliceID, SourceScope: otherItem.SourceScope, SourceID: otherItem.SourceID, TargetText: otherItem.TargetText, State: runtimecontract.RuntimeCoverageItemStateOpen})
	facts.Activation.ExecutionContract.ProfileContracts = []runtimecontract.RuntimeProfileContract{{ID: "receipt-profile", EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{
		{ID: "dod-v2", Scope: "dodItem", TargetRefs: []string{"target.txt"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, AcceptedStepKinds: []string{runtimecontract.StepKindFileWrite}, LineageAssertions: []runtimecontract.RuntimeLineageAssertionRule{rule}},
		{ID: otherItem.SourceID, Scope: otherItem.SourceScope, TargetRefs: []string{"other.txt"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, AcceptedStepKinds: []string{runtimecontract.StepKindFileWrite}, LineageAssertions: []runtimecontract.RuntimeLineageAssertionRule{rule}},
	}}}
	// 自有 pre-start Definition 编译：不让 legacy helper 在 seal 后回填/剥除规则。
	facts.DefinitionFocuses = nil
	for index, item := range facts.DefinitionSeed.CoverageItems {
		policy, err := runtimecontract.CompileRuntimeSourceEvidencePolicy(item.SourceScope, item.SourceID, facts.Activation.ExecutionContract.ProfileContracts, nil)
		if err != nil { t.Fatal(err) }
		focus := runtimecontract.RuntimeDefinitionFocus{
			ID: "receipt-definition/" + string(item.ID), SliceID: item.SliceID, AssessmentScope: item.SourceScope, AssessmentID: item.SourceID, AssessmentText: item.TargetText,
			CoverageWorkItemIDs: []common.CoverageWorkItemID{item.ID}, RequiredEvidenceKinds: policy.RequiredEvidenceKinds,
			AcceptedStepKinds: policy.AcceptedStepKinds, RequireAssessment: true, EvidenceDeclarationRef: policy.SemanticProofDeclarationRef,
			LineageAssertions: append([]runtimecontract.RuntimeLineageAssertionRule(nil), policy.LineageAssertions...),
		}
		facts.DefinitionFocuses = append(facts.DefinitionFocuses, focus)
		if index == 0 { facts.Focus = focus.CoverageFocusDeclaration() } else { facts.RemainingFocuses = append(facts.RemainingFocuses, focus.CoverageFocusDeclaration()) }
	}
	catalog, prerequisite, observation := semanticPlanningCatalogFixture()
	catalog.TargetRefs = []string{"other.txt", "target.txt"}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	observation.ContentHash = catalog.SnapshotID
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{catalog}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{observation}
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{prerequisite}
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil { t.Fatal(err) }
	aggregate, err := controller.initialize(context.Background(), facts, controllerDefinition(), nil)
	if err != nil { t.Fatalf("pre-start exact receipt declarations: %v", err) }
	apply := func(event RuntimeInternalEvent) RuntimeInternalEventResult {
		t.Helper()
		event.Scope = lineageReceiptScope(aggregate)
		result, err := controller.ApplyInternalEvent(context.Background(), event)
		if err != nil { t.Fatalf("生产 %s: %v", event.Kind, err) }
		if result.Admission != nil && result.Admission.Admission.Failure != nil { t.Fatalf("生产 %s 拒绝候选: %+v", event.Kind, result.Admission.Admission.Failure) }
		aggregate = result.Aggregate
		if err := aggregate.Validate(); err != nil { t.Fatalf("%s 完整 aggregate: %v", event.Kind, err) }
		return result
	}
	manifest, document, feasibility := semanticPlanningFixture(t, aggregate)
	order := runtimecontract.RuntimeSemanticPlanDocument{SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash, PlanStep: runtimecontract.RuntimeSemanticPlanStepCoverageOrder, CoverageOrder: []common.CoverageWorkItemID{"cwi-v2", otherItem.ID}}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanRequestStarted, Transaction: &RuntimeInternalTransactionPayload{SemanticManifest: &manifest, StablePrefixHash: "receipt-semantic-prefix", LogicalRequestID: "receipt-semantic-order"}})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, Transaction: &RuntimeInternalTransactionPayload{SemanticDocument: &order, LogicalRequestID: "receipt-semantic-order"}})
	batch, err := semanticplan.CompileFocusBatch(manifest, order, 0, "receipt-semantic-proof", aggregate.Facts.Focus.CoverageWorkItemIDs, aggregate.SemanticPlanning.PendingCoverageIDs, 1)
	if err != nil { t.Fatal(err) }
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanBatchCompiled, Transaction: &RuntimeInternalTransactionPayload{SemanticPlanBatch: &batch}})
	document.BatchID, document.BatchSequence, document.BatchCoverageWorkItemIDs = batch.BatchID, batch.Sequence, batch.CoverageWorkItemIDs
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, Transaction: &RuntimeInternalTransactionPayload{SemanticDocument: &document, LogicalRequestID: batch.RequestLogicalID}})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityRequired, Authority: &RuntimeInternalAuthorityPayload{}})
	feasibility.CatalogObservationID = observation.ObservationID
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityObserved, Authority: &RuntimeInternalAuthorityPayload{SemanticBatchID: aggregate.SemanticPlanning.FeasibilityBatchID, SemanticFeasibility: &feasibility}})
	accepted := aggregate.SemanticPlanning.Batches[0]
	batchResult := runtimecontract.RuntimeSemanticPlanBatchFeasibilityResult{BatchID: accepted.BatchID, BatchHash: accepted.Hash,
		DocumentHash: accepted.DocumentHash, Feasibilities: []runtimecontract.RuntimeSemanticProofFeasibility{feasibility}}
	batchResult.Hash = runtimecontract.StableRuntimeSemanticPlanBatchFeasibilityResultHash(batchResult)
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanBatchFeasibilityObserved, Transaction: &RuntimeInternalTransactionPayload{SemanticPlanBatchFeasibilityResult: &batchResult}})
	merged, err := runtimecontract.MergeRuntimeSemanticPlanFocusBatchDocument(manifest, accepted, document)
	if err != nil { t.Fatal(err) }
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanBatchDocumentsMerged, Transaction: &RuntimeInternalTransactionPayload{SemanticPlanMergedDocument: &merged}})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticContractSealed, Authority: &RuntimeInternalAuthorityPayload{}})
	if len(aggregate.Facts.Focus.CoverageEvidenceBindings) != 1 { t.Fatal("exact semantic seal 未生成唯一 binding") }
	binding := aggregate.Facts.Focus.CoverageEvidenceBindings[0].Clone()
	// 同一个原始 manifest 对另一个 exact CWI 的纯编译结果仅作查询输入，绝不写入 authority。
	otherDocument := document.Clone()
	otherDocument.BatchID, otherDocument.BatchCoverageWorkItemIDs = "receipt-other-query", []common.CoverageWorkItemID{otherItem.ID}
	otherDocument.Proposals[0].CoverageWorkItemID = otherItem.ID
	otherDocument.Proposals[0].Target.ExactTargetRefs = []string{"other.txt"}
	otherDocument.Proposals[0].DeclarationRef = runtimeTestSemanticDeclarationRef(manifest, otherItem.ID)
	otherFeasibility := feasibility.Clone()
	otherFeasibility.CoverageWorkItemID, otherFeasibility.TargetRefs = otherItem.ID, []string{"other.txt"}
	otherFeasibility.ProposalHash = runtimecontract.StableRuntimeSemanticProofProposalHash(otherDocument.Proposals[0])
	otherFocuses, _, err := runtimecontract.CompileRuntimeSemanticBindings(manifest, "receipt-other-query", otherDocument, []runtimecontract.RuntimeSemanticProofFeasibility{otherFeasibility})
	if err != nil || len(otherFocuses) != 1 || len(otherFocuses[0].CoverageEvidenceBindings) != 1 { t.Fatalf("另一 exact source query binding: %v", err) }
	otherBinding := otherFocuses[0].CoverageEvidenceBindings[0].Clone()
	for _, pending := range append([]runtimecontract.RuntimePrerequisite(nil), aggregate.Facts.Prerequisites.Pending...) {
		if pending.Kind != runtimecontract.PrerequisiteKindTargetSnapshot { t.Fatalf("未声明 prerequisite: %+v", pending) }
		content := "原始文件内容\n"
		snapshot := runtimecontract.TargetSnapshot{TargetRef: pending.SubjectRef, Content: content, ContentHash: lineageReceiptDigest(content), ContentState: runtimecontract.SnapshotContentStateComplete}
		observed := runtimecontract.RuntimePrerequisiteObservation{PrerequisiteID: pending.ID, Kind: pending.Kind, OwnerModuleID: pending.OwnerModuleID, ContentHash: snapshot.ContentHash, Dependencies: []runtimecontract.RuntimeFreshnessDependency{{Domain: runtimecontract.RuntimeResourceWorkspaceTarget, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: snapshot.TargetRef, Revision: snapshot.ContentHash}}}
		observed.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observed)
		fact := PrerequisiteResult{PrerequisiteID: pending.ID, TargetSnapshot: &snapshot, Observation: &observed}
		apply(RuntimeInternalEvent{Kind: RuntimeInternalEventPrerequisiteObserved, Authority: &RuntimeInternalAuthorityPayload{PrerequisiteResult: &fact}})
	}
	candidateManifest, err := BuildRuntimeCandidateContractManifestForAggregate(aggregate)
	if err != nil { t.Fatal(err) }
	expected := runtimeTestOperationsExpectedDocument(aggregate, candidateManifest.Hash, 0)
	ensured := apply(RuntimeInternalEvent{Kind: RuntimeInternalEventCandidateRequestEnsured, Transaction: &RuntimeInternalTransactionPayload{DocumentKind: runtimecontract.RuntimeProviderDocumentOperations, StablePrefixHash: "receipt-candidate-prefix", ManifestHash: candidateManifest.Hash, ExpectedDocument: &expected}})
	if ensured.CandidateSession.LogicalRequestID == "" { t.Fatal("缺少原始 candidate session") }
	session := ensured.CandidateSession
	physicalID := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, session.NextSequence)
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventCandidateRequestSubmitted, Transaction: &RuntimeInternalTransactionPayload{LogicalRequestID: session.LogicalRequestID, Sequence: session.NextSequence, PhysicalCallID: physicalID}})
	settlement := runtimeProviderConfirmedBudgetSettlementForTest(t, session.LogicalRequestID, physicalID, nil)
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventCandidateDocumentObserved, Transaction: &RuntimeInternalTransactionPayload{LogicalRequestID: session.LogicalRequestID, PhysicalCallID: physicalID, ProviderBudgetSettlement: settlement}})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventCandidateAttemptBegun, Transaction: &RuntimeInternalTransactionPayload{StablePrefixHash: session.StablePrefixHash, LogicalRequestID: session.LogicalRequestID, Manifest: &candidateManifest}})
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{Kind: runtimecontract.StepKindFileWrite, BindingRefs: []string{binding.ID}, TargetRefs: []string{"target.txt"}, Parameters: runtimeFileWriteParameters("完成后的文件内容\n")}}}}}
	candidate, err = runtimecontract.DeriveRuntimeCandidateOperationAuthority(candidateManifest, candidate)
	if err != nil { t.Fatal(err) }
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventCandidateProposalObserved, Transaction: &RuntimeInternalTransactionPayload{Manifest: &candidateManifest, LogicalRequestID: session.LogicalRequestID, Candidate: &candidate}})
	for _, fact := range runtimeTestCurrentCandidateFeasibilities(t, aggregate, candidateManifest, candidate) {
		fact := fact
		apply(RuntimeInternalEvent{Kind: RuntimeInternalEventCandidateFeasibilityObserved, Transaction: &RuntimeInternalTransactionPayload{CandidateFeasibility: &fact}})
	}
	sealEvent := RuntimeInternalEvent{Kind: RuntimeInternalEventCandidateSealed, Scope: lineageReceiptScope(aggregate), Transaction: &RuntimeInternalTransactionPayload{Manifest: &candidateManifest, LogicalRequestID: session.LogicalRequestID, Candidate: &candidate}}
	for _, inspect := range beforeSeal { inspect(aggregate.CloneRaw(), sealEvent) }
	apply(sealEvent)
	if aggregate.ActiveDispatch == nil || aggregate.ActiveDispatch.ImplementationWorkCommitment != nil { t.Fatal("不是普通 proof dispatch") }
	return controller, repository, aggregate, binding, otherBinding
}

func lineageReceiptScope(aggregate RuntimeAggregate) RuntimeInternalEventScope {
	return RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}
}

func lineageReceiptClaim(t *testing.T, controller *RuntimeController, sealed RuntimeAggregate) (RuntimeAggregate, runtimecontract.RuntimeOperationReceipt) {
	t.Helper()
	result, err := controller.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{Kind: RuntimeInternalEventDispatchExecutionRequested, Scope: lineageReceiptScope(sealed), Transaction: &RuntimeInternalTransactionPayload{}})
	if err != nil || result.OperationCall == nil { t.Fatalf("生产 sealed execution claim: %v", err) }
	claimed := result.Aggregate
	if err := claimed.Validate(); err != nil { t.Fatal(err) }
	execution := claimed.ActiveDispatch.OperationExecutions[0]
	operation, found := candidateOperationByID(claimed.ActiveDispatch.Candidate, execution.OperationID)
	if !found { t.Fatal("claim 缺少封存 operation") }
	before, after := lineageReceiptDigest("原始文件内容\n"), lineageReceiptDigest("完成后的文件内容\n")
	exit := int64(0)
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: execution.ExecutionKey, Fence: execution.Fence, OperationID: execution.OperationID,
		Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable,
		Changed: true, TargetRefs: append([]string(nil), operation.TargetRefs...), BeforeHash: before, AfterHash: after,
		Evidence: runtimecontract.RuntimeOperationEvidence{
			Summary: "原始 File owner 证据：target.txt 从原始内容改为完成内容。\n逐行材料保留。", ContentHash: after,
			ContentAddress: &runtimecontract.RuntimeEvidenceContentAddress{Algorithm: runtimecontract.RuntimeEvidenceDigestSHA256, Digest: after, Ref: "workorch://file/evidence/" + after},
			ExitCode: &exit, EvidenceRefs: []string{"workorch://file/evidence/" + after}, Quality: common.EvidenceQualityValid,
			EvidenceReason: "owner 已观察到真实持久改变", EvidenceTags: []string{"file-owner", "original-receipt"},
		},
		CompletedAt: "2026-10-08T00:00:00Z",
	}
	if err := receipt.Validate(); err != nil { t.Fatal(err) }
	return claimed, receipt
}

func lineageReceiptEvent(aggregate RuntimeAggregate, receipt runtimecontract.RuntimeOperationReceipt) RuntimeInternalEvent {
	dispatchID := ""
	if aggregate.ActiveDispatch != nil { dispatchID = aggregate.ActiveDispatch.ID } else { dispatchID = aggregate.CompletedDispatches[0].ID }
	return RuntimeInternalEvent{Kind: RuntimeInternalEventOperationReceiptObserved, Scope: lineageReceiptScope(aggregate), Transaction: &RuntimeInternalTransactionPayload{DispatchID: dispatchID, Fence: receipt.Fence, OperationReceiptOwnerSource: runtimecontract.RuntimeModuleFile, OperationReceipt: &receipt}}
}

func lineageReceiptObserve(t *testing.T, controller *RuntimeController, aggregate RuntimeAggregate, receipt runtimecontract.RuntimeOperationReceipt) RuntimeAggregate {
	t.Helper()
	result, err := controller.ApplyInternalEvent(context.Background(), lineageReceiptEvent(aggregate, receipt))
	if err != nil { t.Fatalf("生产 owner receipt commit: %v", err) }
	if err := result.Aggregate.Validate(); err != nil { t.Fatal(err) }
	return result.Aggregate
}

func lineageReceiptEvidence(t *testing.T, aggregate RuntimeAggregate, receipt runtimecontract.RuntimeOperationReceipt) ExecutionEvidence {
	t.Helper()
	operation, found := candidateOperationByID(aggregate.ActiveDispatch.Candidate, receipt.OperationID)
	if !found { t.Fatal("缺少原 sealed operation") }
	evidence, err := operationEvidenceFromPersistedReceipt(aggregate, *aggregate.ActiveDispatch, operation, receipt)
	if err != nil { t.Fatal(err) }
	return evidence
}

func lineageReceiptDigest(text string) string {
	digest := sha256.Sum256([]byte(text))
	return hex.EncodeToString(digest[:])
}

func lineageReceiptAssertOriginal(t *testing.T, sealed, aggregate RuntimeAggregate, receipt runtimecontract.RuntimeOperationReceipt) {
	t.Helper()
	if !reflect.DeepEqual(sealed.Definition, aggregate.Definition) || !reflect.DeepEqual(sealed.Facts.DefinitionSeed, aggregate.Facts.DefinitionSeed) || !reflect.DeepEqual(sealed.Facts.DefinitionFocuses, aggregate.Facts.DefinitionFocuses) || !reflect.DeepEqual(sealed.Facts.Activation, aggregate.Facts.Activation) || !reflect.DeepEqual(sealed.SemanticPlanning, aggregate.SemanticPlanning) || !reflect.DeepEqual(sealed.CandidateAttempts, aggregate.CandidateAttempts) || !reflect.DeepEqual(sealed.CandidateRequestSessions, aggregate.CandidateRequestSessions) {
		t.Fatal("receipt/evidence 改写 startup 或 semantic/candidate 原始前像")
	}
	persisted, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(aggregate.OperationReceipts, receipt.ExecutionKey, receipt.Fence)
	if !found || !reflect.DeepEqual(persisted, receipt) { t.Fatal("原始 receipt 未完整保留") }
	fact := aggregate.Facts.Evidence.Accepted[0]
	original := receipt.Evidence
	if fact.Summary != original.Summary || fact.ContentHash != original.ContentHash || !reflect.DeepEqual(fact.ContentAddress, original.ContentAddress) || !reflect.DeepEqual(fact.ExitCode, original.ExitCode) || !reflect.DeepEqual(fact.EvidenceRefs, original.EvidenceRefs) || fact.Quality != original.Quality || fact.ZeroWork != original.ZeroWork || fact.EvidenceReason != original.EvidenceReason || !reflect.DeepEqual(fact.EvidenceTags, original.EvidenceTags) || !reflect.DeepEqual(fact.Validation, original.Validation) {
		t.Fatal("EvidenceFact 用摘要/hash 替代或丢失 owner 原始来源")
	}
}

func lineageReceiptTamperMatrix(t *testing.T, original RuntimeAggregate, binding runtimecontract.CoverageEvidenceBinding) {
	t.Helper()
	for _, test := range []struct {
		name string
		mutate func(*RuntimeAggregate)
	}{
		{"receipt_missing", func(a *RuntimeAggregate) { a.OperationReceipts = nil }},
		{"receipt_duplicate", func(a *RuntimeAggregate) { a.OperationReceipts = append(a.OperationReceipts, a.OperationReceipts[0]) }},
		{"dispatch_duplicate", func(a *RuntimeAggregate) { a.CompletedDispatches = append(a.CompletedDispatches, a.CompletedDispatches[0]) }},
		{"attempt_missing", func(a *RuntimeAggregate) { a.CandidateAttempts = nil }},
		{"session_missing", func(a *RuntimeAggregate) { a.CandidateRequestSessions = nil }},
		{"semantic_source_text_changed", func(a *RuntimeAggregate) { for index := range a.Facts.DefinitionSeed.CoverageItems { if a.Facts.DefinitionSeed.CoverageItems[index].ID == binding.CoverageWorkItemID { a.Facts.DefinitionSeed.CoverageItems[index].TargetText += "改写" } } }},
		{"profile_rule_stripped", func(a *RuntimeAggregate) { for index := range a.Facts.Activation.ExecutionContract.ProfileContracts[0].EvidenceRequirements { if a.Facts.Activation.ExecutionContract.ProfileContracts[0].EvidenceRequirements[index].ID == "dod-v2" { a.Facts.Activation.ExecutionContract.ProfileContracts[0].EvidenceRequirements[index].LineageAssertions = nil } } }},
		{"receipt_before_changed", func(a *RuntimeAggregate) { a.OperationReceipts[0].BeforeHash = lineageReceiptDigest("伪造前像") }},
		{"receipt_after_changed", func(a *RuntimeAggregate) { a.OperationReceipts[0].AfterHash = lineageReceiptDigest("伪造后像") }},
		{"changed_false_coherent_receipt", func(a *RuntimeAggregate) { a.OperationReceipts[0].Changed = false; lineageReceiptRehashProjection(a) }},
		{"equal_before_after_coherent_receipt", func(a *RuntimeAggregate) { a.OperationReceipts[0].AfterHash = a.OperationReceipts[0].BeforeHash; lineageReceiptRehashProjection(a) }},
		{"missing_after_coherent_receipt", func(a *RuntimeAggregate) { a.OperationReceipts[0].AfterHash = ""; lineageReceiptRehashProjection(a) }},
		{"fact_cross_cwi_coherent_hash", func(a *RuntimeAggregate) { a.Facts.Evidence.Accepted[0].CoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-other"}; a.CompletedDispatches[0].OperationExecutions[0].EvidenceFactHash = runtimecontract.StableRuntimeEvidenceFactHash(a.Facts.Evidence.Accepted[0]) }},
		{"fact_summary_changed_coherent_hash", func(a *RuntimeAggregate) { a.Facts.Evidence.Accepted[0].Summary = "摘要不能替代原文"; a.CompletedDispatches[0].OperationExecutions[0].EvidenceFactHash = runtimecontract.StableRuntimeEvidenceFactHash(a.Facts.Evidence.Accepted[0]) }},
		{"processed_missing", func(a *RuntimeAggregate) { a.ProcessedEventIDs = nil }},
		{"processed_duplicate", func(a *RuntimeAggregate) { a.ProcessedEventIDs = append(a.ProcessedEventIDs, a.ProcessedEventIDs[0]) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := original.CloneRaw()
			test.mutate(&changed)
			if runtimecontract.RuntimeExecutionLineageAssertionsSatisfied(binding, runtimeLineageAssertionSources(changed), changed.Facts.EvidenceRevision) { t.Fatal("篡改来源仍满足 owner receipt guard") }
		})
	}
}

// 仅用于非法副本矩阵：同时更新 receipt/execution/fact/event hash，以避免只检验
// 一个 hash mismatch；不会把这些副本 Save，也不会调整 production fence 或预算。
func lineageReceiptRehashProjection(aggregate *RuntimeAggregate) {
	receipt := aggregate.OperationReceipts[0]
	execution := &aggregate.CompletedDispatches[0].OperationExecutions[0]
	fact := &aggregate.Facts.Evidence.Accepted[0]
	oldID := fact.ID
	execution.OwnerReceiptHash = runtimecontract.StableRuntimeOperationReceiptHash(receipt)
	fact.ID = common.EvidenceRecordID("runtime-receipt|" + execution.OwnerReceiptHash)
	execution.EvidenceFactID, execution.EvidenceFactHash = fact.ID, runtimecontract.StableRuntimeEvidenceFactHash(*fact)
	for index, id := range aggregate.ProcessedEventIDs { if id == string(oldID) { aggregate.ProcessedEventIDs[index] = string(fact.ID) } }
}
