package runtime

import (
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// settleImplementationWorkReceipt 仅在 OperationReceiptObserved 中、精确 owner receipt
// 与预算 join 已应用到 detached aggregate 后调用。同一次 aggregate CAS 协调
// SM-2 effect/round、SM-3 execution 与 SM-4 retirement，不调用 terminal credit helper。
func settleImplementationWorkReceipt(a *RuntimeAggregate, operation runtimecontract.RuntimeOperation, receipt runtimecontract.RuntimeOperationReceipt) error {
	if a == nil || a.ActiveDispatch == nil || a.ActiveDispatch.ImplementationWorkCommitment == nil {
		return common.ErrInvalidArgument
	}
	dispatch := *a.ActiveDispatch
	disposition := runtimecontract.RuntimeImplementationWorkDispositionForReceipt(*dispatch.ImplementationWorkCommitment, receipt)
	if disposition == runtimecontract.RuntimeImplementationWorkReceiptStale || disposition == runtimecontract.RuntimeImplementationWorkReceiptGovernance || disposition == runtimecontract.RuntimeImplementationWorkReceiptUnknown || disposition == runtimecontract.RuntimeImplementationWorkReceiptInvalidationMissing {
		// 独立 owner outcome 保留 owner_committed，无 ordinary rejected evidence。
		return closeImplementationWorkReceiptDisposition(a, dispatch, operation.ID, receipt, disposition)
	}
	evidence, err := operationEvidenceFromPersistedReceipt(*a, dispatch, operation, receipt)
	if err != nil {
		return err
	}
	admission := executionEvidenceAdmission{Evidence: evidence, Operation: operation, ExpectedScope: runtimecontract.RuntimeEffectWorkspaceDurable}
	if evidence.Status != ExecutionEvidenceStatusAccepted || disposition != "" {
		if evidence.Status == ExecutionEvidenceStatusAccepted || receipt.Evidence.Validation != nil {
			return closeImplementationWorkReceiptDisposition(a, dispatch, operation.ID, receipt, disposition)
		}
		return settleImplementationWorkRejectedReceipt(a, dispatch, admission, receipt)
	}
	fact, _, err := deriveExecutionEvidenceFact(*a, admission)
	if err != nil {
		return err
	}
	if fact.Kind != runtimecontract.EvidenceKindStateChange || len(fact.BindingRefs) != 0 || len(fact.CoverageWorkItemIDs) != 0 || fact.Validation != nil {
		return fmt.Errorf("implementation work effect cannot carry terminal coverage/validation credit")
	}
	dispatch.OperationExecutions = markOperationExecutionEvidenceCommitted(dispatch.OperationExecutions, operation.ID, evidence.OwnerReceiptRef, evidence.OwnerReceiptHash, fact)
	dispatch.CompletedOperationIDs = appendNormalizedString(dispatch.CompletedOperationIDs, operation.ID)
	a.ActiveDispatch = &dispatch
	a.Facts.Evidence.Accepted = append(a.Facts.Evidence.Accepted, fact)
	a.EvidenceSequence++
	a.Facts.EvidenceRevision = nextEvidenceRevision(a.EvidenceSequence)
	a.ProcessedEventIDs = appendNormalizedString(a.ProcessedEventIDs, evidence.EventID)
	if _, err := applyReceiptResourceChangeInvalidation(a, dispatch.ID, operation.ID, receipt); err != nil {
		return err
	}
	if len(dispatch.CompletedOperationIDs) != candidateOperationCount(dispatch.Candidate) {
		// collective 完成前保留原 sealed decision；退休 observation 仅由后继 authority 消费。
		return nil
	}
	index := -1
	for i, round := range a.ImplementationWork.Rounds {
		if round.DispatchID == dispatch.ID {
			if index >= 0 {
				return fmt.Errorf("implementation dispatch matches duplicate rounds")
			}
			index = i
		}
	}
	if index < 0 {
		return fmt.Errorf("implementation receipt has no original private round")
	}
	round := a.ImplementationWork.Rounds[index].Clone()
	members, err := runtimecontract.RuntimeImplementationWorkReceiptMembers(sealedDispatchToRuntimeRecord(dispatch), a.OperationReceipts, a.Facts, a.OperationGovernanceBindings)
	if err != nil {
		return err
	}
	hash, err := runtimecontract.ValidateRuntimeImplementationWorkReceiptSet(round, members)
	if err != nil {
		return err
	}
	round.CompletedReceiptSetHash = hash
	round.Hash = runtimecontract.StableRuntimeImplementationWorkRoundHash(round)
	a.ImplementationWork = a.ImplementationWork.Clone()
	a.ImplementationWork.Rounds[index] = round
	dispatch.State = DispatchStateCommitted
	a.CompletedDispatches = append(a.CompletedDispatches, dispatch)
	a.ActiveDispatch = nil
	retired, err := resetSemanticPlanningForInvalidatedCatalog(a, runtimecontract.RuntimeSemanticPlanningRetirementResourceInvalidated)
	if err != nil {
		return err
	}
	if !retired {
		// collective effect 已知完成，但当前 catalog 未产生可证明的退休。
		// 保留完整 receipt/effect，不把该事实变成新工作或 terminal 权限。
		failure := runtimecontract.FailureFact{Class: runtimecontract.FailureClassNoProgressExhausted, Code: "implementation_work_catalog_not_retired", Signature: "implementation-work-catalog/" + dispatch.ID, Message: "completed implementation work has no receipt-invalidated current catalog lineage"}
		if err := replaceRuntimeFailureAuthority(&a.Facts, failure, runtimeFailureAuthorityRetire, nil); err != nil {
			return err
		}
	}
	// 原 DefinitionFocus 与 CWI 不变。reducer 只看到 fresh pending owner prerequisites，
	// 不可将 working effect fact 转成 terminal proof，也不重置任何 repair/provider/execution budget。
	a.Facts, a.Decision, err = reduceRuntimeAggregateWithDirective(*a, a.Facts, nil)
	return err
}

// 非成功 receipt 原文入账并关闭本轮，不能换轮或借 exact repair 续权。
// 原 owner Failure/Diagnostic 保真；单独的有界无进展 failure 负责 SM-2 交付。
func settleImplementationWorkRejectedReceipt(a *RuntimeAggregate, dispatch SealedDispatch, admission executionEvidenceAdmission, receipt runtimecontract.RuntimeOperationReceipt) error {
	disposition := runtimecontract.RuntimeImplementationWorkDispositionForReceipt(*dispatch.ImplementationWorkCommitment, receipt)
	if disposition == "" {
		return fmt.Errorf("rejected implementation work receipt lacks independent non-success disposition")
	}
	admission.Evidence.Status = ExecutionEvidenceStatusRejected
	failure := admission.Evidence.Failure
	if failure == nil {
		failure = &runtimecontract.FailureFact{Class: runtimecontract.FailureClassEffectMismatch, Code: "implementation_work_effect_unacceptable", Signature: "implementation-work-effect/" + dispatch.ID + "/" + admission.Operation.ID, Message: "implementation work receipt does not prove a durable changed effect"}
	}
	admission.Evidence.Failure = failure
	fact, _, err := deriveExecutionEvidenceFact(*a, admission)
	if err != nil {
		return err
	}
	if fact.Kind != runtimecontract.EvidenceKindStateChange || len(fact.BindingRefs) != 0 || len(fact.CoverageWorkItemIDs) != 0 || fact.Validation != nil {
		return fmt.Errorf("rejected implementation work effect cannot carry terminal coverage/validation credit")
	}
	fact.FailureClass, fact.FailureCode = failure.Class, failure.Code
	fact.FailureSignature, fact.FailureMessage = failure.Signature, failure.Message
	a.Facts.Evidence.Rejected = append(a.Facts.Evidence.Rejected, fact)
	dispatch.OperationExecutions = markOperationExecutionEvidenceCommitted(dispatch.OperationExecutions, admission.Operation.ID, admission.Evidence.OwnerReceiptRef, admission.Evidence.OwnerReceiptHash, fact)
	dispatch.CompletedOperationIDs = appendNormalizedString(dispatch.CompletedOperationIDs, admission.Operation.ID)
	a.EvidenceSequence++
	a.Facts.EvidenceRevision = nextEvidenceRevision(a.EvidenceSequence)
	a.ProcessedEventIDs = appendNormalizedString(a.ProcessedEventIDs, admission.Evidence.EventID)
	if _, err := applyReceiptResourceChangeInvalidation(a, dispatch.ID, admission.Operation.ID, receipt); err != nil {
		return err
	}
	return closeImplementationWorkReceiptDisposition(a, dispatch, admission.Operation.ID, receipt, disposition)
}

// closeImplementationWorkReceiptDisposition 保留已知 owner outcome，独立关闭工作权限。
// 不把治理/stale/unknown 改写为 ordinary rejection，不生成继任 effect 权限。
func closeImplementationWorkReceiptDisposition(a *RuntimeAggregate, dispatch SealedDispatch, operationID string, receipt runtimecontract.RuntimeOperationReceipt, disposition runtimecontract.RuntimeImplementationWorkReceiptDisposition) error {
	if disposition == "" || disposition != runtimecontract.RuntimeImplementationWorkDispositionForReceipt(*dispatch.ImplementationWorkCommitment, receipt) {
		return fmt.Errorf("implementation receipt closure requires independently derived typed disposition")
	}
	found := false
	a.ImplementationWork = a.ImplementationWork.Clone()
	for i := range a.ImplementationWork.Rounds {
		round := &a.ImplementationWork.Rounds[i]
		if round.DispatchID == dispatch.ID {
			if found {
				return fmt.Errorf("closed implementation receipt matches duplicate rounds")
			}
			found = true
			if disposition == runtimecontract.RuntimeImplementationWorkReceiptGovernance {
				round.GovernanceReceiptHash = runtimecontract.StableRuntimeOperationReceiptHash(receipt)
			} else {
				round.GovernanceReceiptHash = ""
				round.FailedReceiptHash = runtimecontract.StableRuntimeOperationReceiptHash(receipt)
				round.ReceiptDisposition = disposition
			}
			round.Hash = runtimecontract.StableRuntimeImplementationWorkRoundHash(*round)
		}
	}
	if !found {
		return fmt.Errorf("closed implementation receipt has no original round")
	}
	if _, err := applyReceiptResourceChangeInvalidation(a, dispatch.ID, operationID, receipt); err != nil {
		return err
	}
	dispatch.State = DispatchStateInvalidated
	failure := runtimecontract.FailureFact{
		Class:     runtimecontract.FailureClassNoProgressExhausted,
		Code:      "implementation_work_" + string(disposition),
		Signature: "implementation-work-closed/" + dispatch.ID + "/" + operationID,
		Message:   "implementation work authority closed by persisted owner receipt disposition " + string(disposition),
	}
	failureMode := runtimeFailureAuthorityRetire
	if disposition == runtimecontract.RuntimeImplementationWorkReceiptGovernance {
		if receipt.Governance == nil {
			return fmt.Errorf("implementation governance receipt lacks original execution/requirement")
		}
		finding, err := runtimeOperationSuspensionFinding(*a, dispatch, operationID, receipt, *receipt.Governance)
		if err != nil {
			return err
		}
		a.OperationGovernanceBindings = append(a.OperationGovernanceBindings, finding.Binding)
		dispatch.State = DispatchStateCommitted
		failure = runtimecontract.FailureFact{Class: runtimecontract.FailureClassGovernanceRequired, Code: "approval_required", Signature: "governance-" + finding.Dependency.Kind + "-" + finding.Dependency.SubjectRef, Message: finding.Dependency.Reason, TargetRefs: []string{finding.Dependency.SubjectRef}, GovernanceDependency: &finding.Dependency}
		failureMode = runtimeFailureAuthorityRetainInterlude
	}
	if disposition == runtimecontract.RuntimeImplementationWorkReceiptStale {
		stale := receipt.PrerequisiteStale.Normalized()
		signature := runtimecontract.StableRuntimePrerequisiteStaleSignature(stale)
		if !prerequisiteInvalidationExists(*a, signature) {
			facts, prerequisiteIDs, observationIDs, dependencies, err := invalidatePrerequisiteObservationsForStale(a.Facts, stale)
			if err != nil {
				return err
			}
			recovery, consumed, err := consumeRecoveryBudget(facts.Recovery, runtimecontract.RuntimeRecoveryBudgetPrerequisiteRefresh, facts.RecoveryPolicy)
			if err != nil {
				return err
			}
			facts.Recovery = recovery
			a.Facts = facts
			a.PrerequisiteInvalidations = append(a.PrerequisiteInvalidations, runtimecontract.RuntimePrerequisiteInvalidationRecord{Signature: signature, PrerequisiteIDs: prerequisiteIDs, ObservationIDs: observationIDs, InvalidatedDependencies: dependencies, DispatchID: dispatch.ID, OperationID: operationID, Stale: &stale})
			if !consumed {
				failure.Class = runtimecontract.FailureClassPrerequisiteRefreshExhausted
			}
		}
	}
	if disposition == runtimecontract.RuntimeImplementationWorkReceiptUnknown {
		failure.Class = runtimecontract.FailureClassOperationOutcomeUnknown
	}
	a.CompletedDispatches = append(a.CompletedDispatches, dispatch)
	a.ActiveDispatch = nil
	retireImplementationWorkRecoveryAuthorizations(a, dispatch.ID)
	if err := replaceRuntimeFailureAuthority(&a.Facts, failure, failureMode, nil); err != nil {
		return err
	}
	var err error
	a.Facts, a.Decision, err = reduceRuntimeAggregateWithDirective(*a, a.Facts, nil)
	return err
}

// 关闭 effect authority 后，所有未结算 replay grant 原子退休；unknown budget 不释放。
func retireImplementationWorkRecoveryAuthorizations(a *RuntimeAggregate, dispatchID string) {
	for i := range a.OperationRecoveryAuthorizations {
		value := &a.OperationRecoveryAuthorizations[i]
		if value.DispatchID != dispatchID {
			continue
		}
		switch value.State {
		case runtimecontract.RuntimeOperationRecoveryAuthorizationIssued:
			value.State = runtimecontract.RuntimeOperationRecoveryAuthorizationRetired
		case runtimecontract.RuntimeOperationRecoveryAuthorizationInFlight, runtimecontract.RuntimeOperationRecoveryAuthorizationUncertain:
			if _, known := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(a.OperationReceipts, value.ExecutionKey, value.Fence); known {
				// callback 事务已结算，不表示 physical unknown budget 被确认或释放。
				value.State = runtimecontract.RuntimeOperationRecoveryAuthorizationSettled
			} else {
				value.State = runtimecontract.RuntimeOperationRecoveryAuthorizationUncertain
			}
		}
	}
}
