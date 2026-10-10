package workorchruntimecontract

import (
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeLineageAssertionSources 是原 aggregate 的只读 typed 投影，不是成功凭证。
type RuntimeLineageAssertionSources struct {
	Definition        RuntimeDefinitionContext
	Facts             RuntimeFacts
	Work              RuntimeImplementationWorkLedger
	Semantic          RuntimeSemanticPlanningLedger
	Active            *RuntimeDispatchRecord
	Completed         []RuntimeDispatchRecord
	Receipts          []RuntimeOperationReceipt
	Governance        []RuntimeOperationGovernanceBinding
	Budget            RuntimeExecutionBudgetLedger
	EvidenceSequence  uint64
	Resolutions       []RuntimeGovernanceResolutionRecord
	Attempts          []RuntimeCandidateAttemptRecord
	Sessions          []RuntimeCandidateRequestSession
	ProcessedEventIDs []string
	ProofLineage      []RuntimeCoverageProofLineage
	Confirmations     []RuntimeRepairSurfaceConfirmationRecord
}

// runtimeExpectedLineageAssertions 只重编当前 CWI 的封存来源；其他 source 的规则不参与。
func runtimeExpectedLineageAssertions(binding CoverageEvidenceBinding, sources RuntimeLineageAssertionSources) ([]RuntimeLineageAssertion, error) {
	var item RuntimeCoverageItem
	matches := 0
	for _, candidate := range sources.Facts.DefinitionSeed.CoverageItems {
		if candidate.ID == binding.CoverageWorkItemID {
			item, matches = candidate, matches+1
		}
	}
	if matches > 1 {
		return nil, fmt.Errorf("lineage assertion requires unique original CWI")
	}
	if matches == 0 {
		// 未声明 guard 时只需要当前 CWI 的 source identity；无关 source 不要求补充文本前像。
		for _, candidate := range sources.Facts.Coverage {
			if candidate.ID == binding.CoverageWorkItemID {
				item = RuntimeCoverageItem{ID: candidate.ID, SourceScope: candidate.SourceScope, SourceID: candidate.SourceID}
				matches++
			}
		}
		if matches != 1 {
			return nil, fmt.Errorf("lineage assertion requires unique current CWI source")
		}
	}
	policy, err := CompileRuntimeSourceEvidencePolicy(item.SourceScope, item.SourceID, sources.Facts.Activation.ExecutionContract.ProfileContracts, nil)
	if err != nil {
		return nil, err
	}
	expected, err := CompileRuntimeLineageAssertions(policy.LineageAssertions, policy.SemanticProofDeclarationRef, item)
	if err != nil {
		return nil, err
	}
	// nil 与空切片同为未声明；非空必须保留每一条 exact 身份。
	if !slices.Equal(expected, binding.LineageAssertions) {
		return nil, fmt.Errorf("lineage assertion differs from sealed source")
	}
	return expected, nil
}

// RuntimeExecutionLineageAssertionsSatisfied 仅在普通 proof 之后附加 AND。
// delivery 在执行期延期，不在这里授予交付或终验 credit。
func RuntimeExecutionLineageAssertionsSatisfied(binding CoverageEvidenceBinding, sources RuntimeLineageAssertionSources, evidenceRevision string) bool {
	// 未有任何 Profile guard 且 binding 也为空时，新增门禁不改变普通证明。
	if len(binding.LineageAssertions) == 0 && !runtimeProfilesDeclareLineageAssertions(sources.Facts.Activation.ExecutionContract.ProfileContracts) {
		return true
	}
	expected, err := runtimeExpectedLineageAssertions(binding, sources)
	if err != nil {
		return false
	}
	if len(expected) == 0 {
		return true
	}
	if binding.Validate() != nil || sources.Definition.StartupInputHash == "" {
		return false
	}
	executionRequired := false
	for _, assertion := range expected {
		executionRequired = executionRequired || assertion.Rule.ConsumptionPoint == RuntimeProofPhaseExecution
	}
	if !executionRequired {
		return true
	}
	if !runtimeCanonicalLineageCutoff(evidenceRevision) || ValidateRuntimeEvidenceLedgerSequence(sources.EvidenceSequence, sources.Facts, sources.Active, sources.Completed, sources.Resolutions) != nil || EvidenceRevisionAfter(evidenceRevision, sources.Facts.EvidenceRevision) {
		return false
	}
	for _, assertion := range expected {
		if assertion.Rule.ConsumptionPoint == RuntimeProofPhasePostTerminalDelivery {
			continue
		}
		switch assertion.Rule.Kind {
		case RuntimeLineageCompletedWorkRounds:
			if !runtimeCompletedWorkRoundsSatisfied(binding.CoverageWorkItemID, assertion.Rule.MinCompletedRounds, sources, evidenceRevision) {
				return false
			}
		case RuntimeLineageOwnerOperationReceipt:
			if !runtimeOwnerOperationReceiptSatisfied(binding.CoverageWorkItemID, assertion.Rule, sources, evidenceRevision) {
				return false
			}
		case RuntimeLineageRepairThenFreshValidation:
			if !runtimeRepairThenFreshValidationSatisfied(binding, sources, evidenceRevision) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func runtimeCompletedWorkRoundsSatisfied(coverageID common.CoverageWorkItemID, minimum int, sources RuntimeLineageAssertionSources, revision string) bool {
	if ValidateRuntimeImplementationWorkState(sources.Definition, sources.Facts, sources.Work, sources.Semantic, sources.Active, sources.Completed, sources.Receipts, sources.Governance, sources.Budget) != nil {
		return false
	}
	ordinals, dispatches, keys := map[int]bool{}, map[string]bool{}, map[string]bool{}
	for _, round := range sources.Work.Rounds {
		if round.CoverageWorkItemID != coverageID || round.CompletedReceiptSetHash == "" {
			continue
		}
		if ordinals[round.Ordinal] || dispatches[round.DispatchID] || round.StartupInputHash != sources.Definition.StartupInputHash || round.ScopeObservation == nil || !round.ScopeRequest.Policy.RequireFreshObservation || !round.ScopeRequest.Policy.ProtectValidationInputs {
			return false
		}
		var dispatch RuntimeDispatchRecord
		matches := 0
		for _, value := range sources.Completed {
			if value.ID == round.DispatchID {
				dispatch, matches = value, matches+1
			}
		}
		if matches != 1 {
			return false
		}
		members, err := RuntimeImplementationWorkReceiptMembers(dispatch, sources.Receipts, sources.Facts, sources.Governance)
		if err != nil {
			return false
		}
		hash, err := ValidateRuntimeImplementationWorkReceiptSet(round, members)
		if err != nil || hash != round.CompletedReceiptSetHash {
			return false
		}
		inWindow := true
		for _, member := range members {
			if EvidenceRevisionAfter(member.Evidence.Revision, revision) {
				inWindow = false
			}
		}
		if !inWindow {
			continue
		}
		for _, member := range members {
			if keys[member.Execution.ExecutionKey] {
				return false
			}
			keys[member.Execution.ExecutionKey] = true
		}
		ordinals[round.Ordinal], dispatches[round.DispatchID] = true, true
	}
	return len(ordinals) >= minimum
}

func runtimeOwnerOperationReceiptSatisfied(coverageID common.CoverageWorkItemID, rule RuntimeLineageAssertionRule, sources RuntimeLineageAssertionSources, revision string) bool {
	if runtimeValidateLineageOperations(sources) != nil {
		return false
	}
	for _, dispatch := range sources.Completed {
		if dispatch.State != RuntimeDispatchStateCommitted || dispatch.ImplementationWorkCommitment != nil {
			continue
		}
		for _, operation := range runtimeCandidateAdmissionOperations(dispatch.Candidate) {
			if operation.Kind != rule.OperationKind {
				continue
			}
			descriptor, known := RuntimeOperationDescriptorForKind(operation.Kind)
			contract, contractKnown := RuntimeOperationContractForKind(operation.Kind)
			if !known || !contractKnown || descriptor.OwnerModuleID != rule.OwnerModuleID {
				continue
			}
			variantID := ""
			if len(contract.Variants) != 0 {
				variant, err := MatchRuntimeOperationVariant(operation, contract)
				if err != nil {
					continue
				}
				variantID = variant.ID
			}
			if variantID != rule.OperationVariantID {
				continue
			}
			fact, receipt, valid := runtimeLineageOperationFact(dispatch, operation, sources, true)
			if !valid || EvidenceRevisionAfter(fact.Revision, revision) || receipt.ObservedEffectScope != rule.RequiredEffectScope || !slices.Contains(fact.CoverageWorkItemIDs, coverageID) {
				continue
			}
			for _, binding := range dispatch.CoverageBindings {
				if binding.CoverageWorkItemID == coverageID && slices.Contains(operation.BindingRefs, binding.ID) && slices.Contains(fact.BindingRefs, binding.ID) {
					return true
				}
			}
		}
	}
	return false
}

// runtimeValidateLineageOperations 复用原 SM-3 封存、准入及 owner 回执边界，不递归 aggregate。
func runtimeValidateLineageOperations(sources RuntimeLineageAssertionSources) error {
	if err := ValidateRuntimeDispatchHistoryShape(sources.Facts.WorkspaceID, sources.Facts.RunID, sources.Active, sources.Completed); err != nil {
		return err
	}
	if err := ValidateRuntimeCandidateAttemptRecords(sources.Attempts); err != nil {
		return err
	}
	if err := ValidateRuntimeAcceptedCandidateManifestLineage(sources.Active, sources.Completed, sources.Attempts, sources.Sessions); err != nil {
		return err
	}
	if err := ValidateRuntimeDispatchAdmissionLineage(sources.Active, sources.Completed, sources.Attempts, sources.Governance); err != nil {
		return err
	}
	if err := ValidateRuntimeDispatchExecutionKeyLineage(sources.Facts.WorkspaceID, sources.Facts.RunID, sources.Active, sources.Completed, sources.Governance); err != nil {
		return err
	}
	if err := ValidateRuntimeOperationGovernanceGrantLineage(sources.Governance, sources.Resolutions, sources.Facts.Evidence.Accepted, sources.Facts.Evidence.Rejected); err != nil {
		return err
	}
	return ValidateRuntimeOperationReceiptExecutionLineage(sources.Active, sources.Completed, sources.Receipts)
}

// runtimeLineageOperationFact 反查唯一 receipt/evidence/processed fence 与完整原文投影。
func runtimeLineageOperationFact(dispatch RuntimeDispatchRecord, operation RuntimeOperation, sources RuntimeLineageAssertionSources, accepted bool) (EvidenceFact, RuntimeOperationReceipt, bool) {
	execution, found := runtimeDispatchExecutionForOperation(dispatch, operation.ID)
	if !found || execution.State != OperationExecutionEvidenceCommitted || execution.OperationHash != StableRuntimeOperationHash(operation) {
		return EvidenceFact{}, RuntimeOperationReceipt{}, false
	}
	var receipt RuntimeOperationReceipt
	matches := 0
	for _, candidate := range sources.Receipts {
		if candidate.ExecutionKey == execution.ExecutionKey && candidate.Fence == execution.Fence {
			receipt, matches = candidate, matches+1
		}
	}
	if matches != 1 || receipt.Validate() != nil || receipt.OperationID != operation.ID || receipt.PrerequisiteStale != nil || receipt.ObservedEffectScope != execution.ExpectedEffectScope || StableRuntimeOperationReceiptHash(receipt) != execution.OwnerReceiptHash || execution.OwnerReceiptRef != "runtime-receipt/"+receipt.ExecutionKey || !slices.Equal(receipt.TargetRefs, operation.TargetRefs) {
		return EvidenceFact{}, RuntimeOperationReceipt{}, false
	}
	if accepted && (receipt.Outcome != RuntimeOperationOutcomeAccepted || (RuntimeEffectIsDurable(receipt.ObservedEffectScope) && (!receipt.Changed || receipt.BeforeHash == receipt.AfterHash || receipt.AfterHash == ""))) {
		return EvidenceFact{}, RuntimeOperationReceipt{}, false
	}
	if !accepted && receipt.Outcome != RuntimeOperationOutcomeRejected {
		return EvidenceFact{}, RuntimeOperationReceipt{}, false
	}
	var fact EvidenceFact
	matches = 0
	for _, candidate := range append(slices.Clone(sources.Facts.Evidence.Accepted), sources.Facts.Evidence.Rejected...) {
		if candidate.ID == execution.EvidenceFactID {
			fact, matches = candidate, matches+1
		}
	}
	if matches != 1 || StableRuntimeEvidenceFactHash(fact) != execution.EvidenceFactHash || fact.DispatchID != dispatch.ID || fact.OperationID != operation.ID || fact.AuthoritySnapshotID != dispatch.Transaction.AuthoritySnapshotID || fact.SourceRef != execution.OwnerReceiptRef || fact.ID != common.EvidenceRecordID("runtime-receipt|"+execution.OwnerReceiptHash) || fact.SubjectRef != operation.ID || !slices.Equal(fact.BindingRefs, operation.BindingRefs) || !slices.Equal(fact.TargetRefs, receipt.TargetRefs) {
		return EvidenceFact{}, RuntimeOperationReceipt{}, false
	}
	processed := 0
	for _, id := range sources.ProcessedEventIDs {
		if id == string(fact.ID) {
			processed++
		}
	}
	if processed != 1 {
		return EvidenceFact{}, RuntimeOperationReceipt{}, false
	}
	ledger := sources.Facts.Evidence.Accepted
	if !accepted {
		ledger = sources.Facts.Evidence.Rejected
	}
	present := 0
	for _, candidate := range ledger {
		if candidate.ID == fact.ID {
			present++
		}
	}
	if present != 1 || (accepted && assessmentEvidenceWeak(fact)) {
		return EvidenceFact{}, RuntimeOperationReceipt{}, false
	}
	evidence := receipt.Evidence
	if fact.Summary != evidence.Summary || fact.ContentHash != evidence.ContentHash || !reflect.DeepEqual(fact.OwnerRevisionSet, evidence.OwnerRevisionSet) || !reflect.DeepEqual(fact.ContentAddress, evidence.ContentAddress) || !reflect.DeepEqual(fact.ExitCode, evidence.ExitCode) || !slices.Equal(fact.EvidenceRefs, evidence.EvidenceRefs) || fact.Quality != evidence.Quality || fact.ZeroWork != evidence.ZeroWork || fact.EvidenceReason != evidence.EvidenceReason || !slices.Equal(fact.EvidenceTags, evidence.EvidenceTags) || !reflect.DeepEqual(fact.Validation, evidence.Validation) {
		return EvidenceFact{}, RuntimeOperationReceipt{}, false
	}
	return fact, receipt, true
}

// runtimeCanonicalLineageCutoff 只接受原生产序列格式；比较前必须验证完整序列。
func runtimeCanonicalLineageCutoff(value string) bool {
	const prefix = "runtime-evidence-"
	if !strings.HasPrefix(value, prefix) || len(value) != len(prefix)+20 {
		return false
	}
	sequence, err := strconv.ParseUint(strings.TrimPrefix(value, prefix), 10, 64)
	return err == nil && value == runtimeEvidenceRevisionForSequence(sequence)
}
