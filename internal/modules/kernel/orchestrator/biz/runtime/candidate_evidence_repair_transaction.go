package runtime

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// confirmedRepairTransaction is a private, derived view of one exact
// validation-owner rejection -> resource-owner confirmation -> sealed state-change transaction. It
// is never persisted and carries no mutable target authority. In particular,
// a partial all-of dispatch keeps the old FailureTarget and NeedStateChange
// directive retired after generic freshness invalidation; the remaining
// operations are authorized exclusively by the immutable dispatch seal.
type confirmedRepairTransaction struct {
	confirmation  runtimecontract.RuntimeRepairSurfaceConfirmation
	sourceFact    runtimecontract.EvidenceFact
	sourceReceipt runtimecontract.RuntimeOperationReceipt
	sourceOp      runtimecontract.RuntimeOperation
	closure       runtimecontract.RuntimeRepairClosure
}

type confirmedRepairReceiptLink struct {
	beforeRevision string
	afterRevision  string
}

type completedConfirmedRepairTransaction struct {
	transaction            confirmedRepairTransaction
	dispatch               SealedDispatch
	finalEvidenceRevision  string
	finalWorkspaceRevision string
}

type confirmedRepairDispatchLineage struct {
	dispatches []SealedDispatch
	bindings   []runtimecontract.RuntimeOperationGovernanceBinding
}

// deriveConfirmedRepairAdmissionCommitment recognizes the exact
// confirmed-validation repair transaction while its mutable SM-2 authority is
// still current. The returned value is persisted on the operations request
// session in the proposal CAS, before feasibility and final dispatch seal.
// Later consumers may only copy and reverse-prove this commitment; they may
// not reconstruct repair purpose from current FailureTarget or directive.
func deriveConfirmedRepairAdmissionCommitment(
	aggregate RuntimeAggregate,
	manifest runtimecontract.RuntimeCandidateContractManifest,
	candidate runtimecontract.RuntimeCandidate,
) (*runtimecontract.RuntimeDispatchRepairCommitment, error) {
	manifest = manifest.Normalized()
	if manifest.State != runtimecontract.ConvergenceStateNeedStateChange ||
		manifest.RepairPurpose != runtimecontract.RuntimeDispatchRepairPurposeConfirmedValidation {
		return nil, nil
	}
	if candidate.StateChange == nil || aggregate.Decision.State != runtimecontract.ConvergenceStateNeedStateChange ||
		aggregate.Decision.RequiredOutput.RepairPurpose != runtimecontract.RuntimeDispatchRepairPurposeConfirmedValidation ||
		strings.TrimSpace(aggregate.Decision.RequiredOutput.RepairClosureID) == "" ||
		manifest.RepairClosureID != aggregate.Decision.RequiredOutput.RepairClosureID || aggregate.Facts.Focus.ID == "" {
		return nil, fmt.Errorf("%w: confirmed owner repair manifest has no exact typed decision purpose", common.ErrConflict)
	}
	operations := candidateRuntimeOperations(candidate)
	if len(operations) == 0 {
		return nil, fmt.Errorf("%w: confirmed owner repair candidate has no state-change operation", common.ErrConflict)
	}
	var targetRefs []string
	repairOwnerModuleID := ""
	for _, rawOperation := range operations {
		operation := rawOperation.Normalized()
		descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
		if !found {
			return nil, fmt.Errorf("%w: confirmed owner repair candidate operation %q has no descriptor", common.ErrConflict, operation.Kind)
		}
		if repairOwnerModuleID == "" {
			repairOwnerModuleID = descriptor.OwnerModuleID
		} else if repairOwnerModuleID != descriptor.OwnerModuleID {
			return nil, fmt.Errorf("%w: confirmed repair manifest spans multiple operation owners", common.ErrConflict)
		}
		if len(operation.TargetRefs) == 0 {
			return nil, fmt.Errorf("%w: confirmed owner repair candidate operation %q has no target", common.ErrConflict, operation.ID)
		}
		targetRefs = append(targetRefs, operation.TargetRefs...)
	}
	targetRefs = normalizeRuntimeTargetRefs(targetRefs)
	targetAuthorizations, err := runtimecontract.RuntimeCandidateAdmissionTargetAuthorizationsFromManifest(manifest, candidate)
	if err != nil {
		return nil, fmt.Errorf("%w: confirmed owner repair candidate target projection: %v", common.ErrConflict, err)
	}
	probe := SealedDispatch{Candidate: candidate, OperationTargetAuthorizations: targetAuthorizations}
	coverageIDs := append([]common.CoverageWorkItemID(nil), aggregate.Decision.RequiredOutput.CoverageWorkItemIDs...)
	// Decision targets are the complete owner-confirmed authorization surface,
	// not an implicit operation sequence. Admission may choose a non-empty
	// subset, while the confirmation and closure below must retain the surface.
	if len(coverageIDs) == 0 || !sameCoverageIDs(coverageIDs, aggregate.Facts.Focus.CoverageWorkItemIDs) ||
		!runtimeTargetRefsWithin(targetRefs, aggregate.Decision.RequiredOutput.TargetRefs) {
		return nil, fmt.Errorf("%w: confirmed owner repair candidate scope is outside its sealed Decision", common.ErrConflict)
	}
	originID := runtimecontract.StableRuntimeDispatchID(
		aggregate.Decision.AuthorityTransaction(),
		runtimecontract.StableRuntimeCandidateHash(candidate),
	)
	var matched *runtimecontract.RuntimeDispatchRepairCommitment
	for _, rawRecord := range aggregate.RepairSurfaceConfirmations {
		if err := rawRecord.Validate(); err != nil {
			return nil, fmt.Errorf("%w: confirmed owner repair admission confirmation: %v", common.ErrConflict, err)
		}
		record := rawRecord.Normalized()
		confirmation := record.Confirmation
		if confirmation.Disposition != runtimecontract.RuntimeRepairSurfaceConfirmed ||
			confirmation.OwnerModuleID != repairOwnerModuleID ||
			!sameRuntimeTargetRefs(confirmation.ConfirmedTargetRefs, aggregate.Decision.RequiredOutput.TargetRefs) {
			continue
		}
		if err := validateConfirmedRepairDispatchOperations(probe, confirmation); err != nil {
			return nil, err
		}
		sourceFact, sourceReceipt, sourceOperation, sourceErr := confirmedRepairSourceRejection(aggregate, confirmation)
		if sourceErr != nil {
			return nil, sourceErr
		}
		if sourceFact.Revision != aggregate.Decision.AuthorityTransaction().EvidenceRevision ||
			!sameRuntimeStringSet(sourceFact.BindingRefs, sourceOperation.BindingRefs) ||
			!confirmedRepairSourceBindingsCoverCurrentScope(aggregate, sourceOperation.BindingRefs, coverageIDs) {
			continue
		}
		if len(sourceFact.CoverageWorkItemIDs) != 0 && !sameCoverageIDs(sourceFact.CoverageWorkItemIDs, coverageIDs) {
			return nil, fmt.Errorf("%w: confirmed owner repair admission source coverage drifted", common.ErrConflict)
		}
		closure := runtimecontract.RuntimeRepairClosure{
			GapSignature:                      confirmation.Observation.FailureSignature,
			BaselineEvidenceRevision:          sourceFact.Revision,
			CoverageWorkItemIDs:               coverageIDs,
			RequiredEvidenceKinds:             []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation},
			GroundedTargetRefs:                append([]string(nil), confirmation.ConfirmedTargetRefs...),
			RequireValidationAfterStateChange: true,
		}.Normalized()
		closure.ID = runtimecontract.StableRuntimeRepairClosureID(closure)
		if closure.ID != aggregate.Decision.RequiredOutput.RepairClosureID {
			continue
		}
		transaction := confirmedRepairTransaction{
			confirmation: confirmation, sourceFact: sourceFact, sourceReceipt: sourceReceipt,
			sourceOp: sourceOperation, closure: closure,
		}
		// The current SM-2 directive is produced by the ordinary reducer in the
		// owner-confirmation CAS. Rebuild that owner-local projection exactly, then
		// independently bind its closure to the receipt-derived transaction. The
		// commitment does not require a second, repair-specific directive ID and
		// it never treats the mutable directive itself as the source lineage.
		expectedDirective := buildRepairDirective(aggregate.Facts, aggregate.Decision)
		if aggregate.Facts.RepairDirective == nil ||
			!runtimecontract.SameRuntimeRepairDirective(aggregate.Facts.RepairDirective, &expectedDirective) ||
			!runtimecontract.SameRuntimeRepairClosure(aggregate.Facts.RepairDirective.Closure, &transaction.closure) {
			return nil, fmt.Errorf("%w: confirmed owner repair admission has no exact current state-change directive", common.ErrConflict)
		}
		commitment := runtimecontract.RuntimeDispatchRepairCommitment{
			Purpose:                   runtimecontract.RuntimeDispatchRepairPurposeConfirmedValidation,
			RepairOwnerModuleID:       confirmation.OwnerModuleID,
			OriginDispatchID:          originID,
			FocusID:                   aggregate.Facts.Focus.ID,
			Closure:                   closure,
			ConfirmationIdentityHash:  record.IdentitySignature,
			ConfirmationHash:          record.Signature,
			SourceDispatchID:          confirmation.Observation.DispatchID,
			SourceAuthoritySnapshotID: confirmation.Observation.AuthoritySnapshotID,
			SourceOperationID:         sourceOperation.ID,
			SourceOperationHash:       runtimecontract.StableRuntimeOperationHash(sourceOperation),
			SourceEvidenceFactID:      sourceFact.ID,
			SourceEvidenceFactHash:    runtimecontract.StableRuntimeEvidenceFactHash(sourceFact),
			SourceReceiptExecutionKey: sourceReceipt.ExecutionKey,
			SourceReceiptFence:        sourceReceipt.Fence,
			SourceReceiptHash:         runtimecontract.StableRuntimeOperationReceiptHash(sourceReceipt),
		}.Normalized()
		if err := commitment.Validate(); err != nil {
			return nil, err
		}
		if matched != nil {
			return nil, fmt.Errorf("%w: confirmed owner repair admission has multiple exact source lineages", common.ErrConflict)
		}
		matched = &commitment
	}
	if matched == nil {
		return nil, fmt.Errorf("%w: confirmed owner repair directive has no pre-seal source commitment", common.ErrConflict)
	}
	return matched, nil
}

// confirmedRepairTransactionFromCommitment is the fresh-load reverse
// proof for the immutable repair purpose. Classification starts from the
// dispatch commitment, then requires the independently persisted owner
// confirmation and exact rejected receipt/evidence lineage. Missing either
// side fails closed; current mutable repair projections are never consulted.
func confirmedRepairTransactionFromCommitment(
	aggregate RuntimeAggregate,
	dispatch SealedDispatch,
	origin SealedDispatch,
) (*confirmedRepairTransaction, error) {
	if dispatch.RepairCommitment == nil {
		return nil, fmt.Errorf("%w: confirmed owner repair dispatch has no repair commitment", common.ErrConflict)
	}
	if err := runtimecontract.ValidateRuntimeDispatchRepairCommitmentProjection(
		dispatch.RepairCommitmentHash,
		dispatch.RepairCommitment,
	); err != nil {
		return nil, fmt.Errorf("%w: confirmed owner repair dispatch commitment: %v", common.ErrConflict, err)
	}
	commitment := dispatch.RepairCommitment.Normalized()
	if commitment.Purpose != runtimecontract.RuntimeDispatchRepairPurposeConfirmedValidation ||
		commitment.OriginDispatchID != origin.ID || commitment.FocusID != origin.Transaction.FocusID ||
		origin.RepairCommitmentHash != dispatch.RepairCommitmentHash ||
		!runtimecontract.SameRuntimeDispatchRepairCommitment(origin.RepairCommitment, dispatch.RepairCommitment) {
		return nil, fmt.Errorf("%w: confirmed owner repair commitment does not match its origin admission", common.ErrConflict)
	}
	var confirmation *runtimecontract.RuntimeRepairSurfaceConfirmation
	matches := 0
	for _, rawRecord := range aggregate.RepairSurfaceConfirmations {
		if rawRecord.IdentitySignature != commitment.ConfirmationIdentityHash ||
			rawRecord.Signature != commitment.ConfirmationHash {
			continue
		}
		if err := rawRecord.Validate(); err != nil {
			return nil, fmt.Errorf("%w: confirmed owner repair committed confirmation: %v", common.ErrConflict, err)
		}
		value := rawRecord.Normalized().Confirmation
		confirmation = &value
		matches++
	}
	if matches != 1 || confirmation == nil || confirmation.Disposition != runtimecontract.RuntimeRepairSurfaceConfirmed ||
		confirmation.OwnerModuleID != commitment.RepairOwnerModuleID {
		return nil, fmt.Errorf("%w: confirmed owner repair commitment has no unique exact owner confirmation", common.ErrConflict)
	}
	sourceFact, sourceReceipt, sourceOperation, err := confirmedRepairSourceRejection(aggregate, *confirmation)
	if err != nil {
		return nil, err
	}
	if commitment.SourceDispatchID != confirmation.Observation.DispatchID ||
		commitment.SourceDispatchID != sourceFact.DispatchID ||
		commitment.SourceAuthoritySnapshotID != confirmation.Observation.AuthoritySnapshotID ||
		commitment.SourceAuthoritySnapshotID != sourceFact.AuthoritySnapshotID ||
		commitment.SourceOperationID != sourceOperation.ID ||
		commitment.SourceOperationHash != runtimecontract.StableRuntimeOperationHash(sourceOperation) ||
		commitment.SourceEvidenceFactID != sourceFact.ID ||
		commitment.SourceEvidenceFactHash != runtimecontract.StableRuntimeEvidenceFactHash(sourceFact) ||
		commitment.SourceReceiptExecutionKey != sourceReceipt.ExecutionKey ||
		commitment.SourceReceiptFence != sourceReceipt.Fence ||
		commitment.SourceReceiptHash != runtimecontract.StableRuntimeOperationReceiptHash(sourceReceipt) ||
		sourceFact.Revision != origin.Transaction.EvidenceRevision {
		return nil, fmt.Errorf("%w: confirmed owner repair commitment source lineage mismatch", common.ErrConflict)
	}
	closure := commitment.Closure.Normalized()
	if closure.ID != runtimecontract.StableRuntimeRepairClosureID(closure) ||
		closure.GapSignature != confirmation.Observation.FailureSignature ||
		closure.BaselineEvidenceRevision != sourceFact.Revision ||
		!sameRuntimeTargetRefs(closure.GroundedTargetRefs, confirmation.ConfirmedTargetRefs) ||
		!sameRuntimeStringSet(closure.RequiredEvidenceKinds, []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}) ||
		!closure.RequireValidationAfterStateChange {
		return nil, fmt.Errorf("%w: confirmed owner repair commitment closure does not match its source", common.ErrConflict)
	}
	if len(sourceFact.CoverageWorkItemIDs) != 0 &&
		!sameCoverageIDs(sourceFact.CoverageWorkItemIDs, closure.CoverageWorkItemIDs) {
		return nil, fmt.Errorf("%w: confirmed owner repair commitment source coverage mismatch", common.ErrConflict)
	}
	if err := validateConfirmedRepairDispatchOperations(dispatch, *confirmation); err != nil {
		return nil, err
	}
	return &confirmedRepairTransaction{
		confirmation: *confirmation, sourceFact: sourceFact, sourceReceipt: sourceReceipt,
		sourceOp: sourceOperation, closure: closure,
	}, nil
}

func confirmedRepairAdmissionLineage(aggregate RuntimeAggregate, leaf SealedDispatch) (confirmedRepairDispatchLineage, error) {
	byID := make(map[string]SealedDispatch, len(aggregate.CompletedDispatches)+1)
	for _, dispatch := range aggregate.CompletedDispatches {
		if _, duplicate := byID[dispatch.ID]; duplicate {
			return confirmedRepairDispatchLineage{}, fmt.Errorf("%w: confirmed owner repair dispatch lineage duplicates dispatch %q", common.ErrConflict, dispatch.ID)
		}
		byID[dispatch.ID] = dispatch
	}
	if aggregate.ActiveDispatch != nil {
		if _, duplicate := byID[aggregate.ActiveDispatch.ID]; duplicate {
			return confirmedRepairDispatchLineage{}, fmt.Errorf("%w: confirmed owner repair dispatch lineage duplicates active dispatch", common.ErrConflict)
		}
		byID[aggregate.ActiveDispatch.ID] = *aggregate.ActiveDispatch
	}
	byID[leaf.ID] = leaf

	reversedDispatches := make([]SealedDispatch, 0, 2)
	reversedBindings := make([]runtimecontract.RuntimeOperationGovernanceBinding, 0, 1)
	visited := map[string]struct{}{}
	current := leaf
	for {
		if _, cycle := visited[current.ID]; cycle {
			return confirmedRepairDispatchLineage{}, fmt.Errorf("%w: confirmed owner repair governance dispatch lineage is cyclic", common.ErrConflict)
		}
		visited[current.ID] = struct{}{}
		reversedDispatches = append(reversedDispatches, current)

		acceptedMatches := 0
		for _, attempt := range aggregate.CandidateAttempts {
			if runtimecontract.RuntimeAcceptedCandidateAttemptMatchesDispatch(attempt, sealedDispatchToRuntimeRecord(current)) {
				acceptedMatches++
			}
		}
		var resumeBinding *runtimecontract.RuntimeOperationGovernanceBinding
		resumeMatches := 0
		for _, rawBinding := range aggregate.OperationGovernanceBindings {
			binding := rawBinding.Normalized()
			if binding.State != runtimecontract.RuntimeOperationGovernanceBindingGranted || binding.Grant == nil ||
				binding.Grant.Normalized().ResumeDispatchID != current.ID {
				continue
			}
			value := binding
			resumeBinding = &value
			resumeMatches++
		}
		if acceptedMatches+resumeMatches != 1 {
			return confirmedRepairDispatchLineage{}, fmt.Errorf("%w: confirmed owner repair dispatch %q has no unique accepted-attempt/governance admission", common.ErrConflict, current.ID)
		}
		if acceptedMatches == 1 {
			break
		}
		binding := resumeBinding.Normalized()
		source, found := byID[binding.OperationDispatchID]
		if !found || source.State != DispatchStateCommitted || binding.Grant == nil ||
			source.CandidateHash != current.CandidateHash || source.CandidateManifestHash != current.CandidateManifestHash ||
			source.RepairCommitmentHash != current.RepairCommitmentHash ||
			!runtimecontract.SameRuntimeDispatchRepairCommitment(source.RepairCommitment, current.RepairCommitment) ||
			!runtimecontract.RuntimeDispatchOperationTargetAuthorizationProjectionsEqual(source.OperationTargetAuthorizations, current.OperationTargetAuthorizations) ||
			!runtimecontract.RuntimeValidationRecipeProjectionsEqual(source.ValidationRecipes, current.ValidationRecipes) {
			return confirmedRepairDispatchLineage{}, fmt.Errorf("%w: confirmed owner repair governance resume does not inherit its exact source admission", common.ErrConflict)
		}
		reversedBindings = append(reversedBindings, binding)
		current = source
	}
	if len(reversedBindings) != 0 {
		if err := runtimecontract.ValidateRuntimeOperationGovernanceGrantLineage(
			aggregate.OperationGovernanceBindings,
			aggregate.GovernanceResolutions,
			aggregate.Facts.Evidence.Accepted,
			aggregate.Facts.Evidence.Rejected,
		); err != nil {
			return confirmedRepairDispatchLineage{}, fmt.Errorf("%w: confirmed owner repair governance lineage: %v", common.ErrConflict, err)
		}
	}
	lineage := confirmedRepairDispatchLineage{
		dispatches: make([]SealedDispatch, len(reversedDispatches)),
		bindings:   make([]runtimecontract.RuntimeOperationGovernanceBinding, len(reversedBindings)),
	}
	for index := range reversedDispatches {
		lineage.dispatches[len(reversedDispatches)-1-index] = reversedDispatches[index]
	}
	for index := range reversedBindings {
		lineage.bindings[len(reversedBindings)-1-index] = reversedBindings[index]
	}
	return lineage, nil
}

func (l confirmedRepairDispatchLineage) origin() (SealedDispatch, bool) {
	if len(l.dispatches) == 0 {
		return SealedDispatch{}, false
	}
	return l.dispatches[0], true
}

func (l confirmedRepairDispatchLineage) dispatchIndex(dispatchID string) int {
	for index := range l.dispatches {
		if l.dispatches[index].ID == dispatchID {
			return index
		}
	}
	return -1
}

// confirmedRepairGovernanceSourceTransaction proves that one operation
// governance binding belongs to an already admitted, single-owner repair
// transaction.  The proof is rooted in the source dispatch's accepted
// admission and the immutable validation rejection/owner confirmation pair; current
// FailureTarget or RepairDirective state is deliberately not consulted.
func confirmedRepairGovernanceSourceTransaction(
	aggregate RuntimeAggregate,
	binding runtimecontract.RuntimeOperationGovernanceBinding,
) (SealedDispatch, *confirmedRepairTransaction, bool, error) {
	binding = binding.Normalized()
	if binding.OperationDispatchID == "" || binding.OperationID == "" || binding.ExecutionKey == "" {
		return SealedDispatch{}, nil, false, nil
	}
	var source *SealedDispatch
	for index := range aggregate.CompletedDispatches {
		candidate := &aggregate.CompletedDispatches[index]
		if candidate.ID != binding.OperationDispatchID {
			continue
		}
		if source != nil {
			return SealedDispatch{}, nil, false, fmt.Errorf("%w: confirmed owner repair governance source dispatch is duplicated", common.ErrConflict)
		}
		source = candidate
	}
	if source == nil || source.State != DispatchStateCommitted || source.Transaction.State != runtimecontract.ConvergenceStateNeedStateChange {
		return SealedDispatch{}, nil, false, nil
	}
	operation, found := candidateOperationByID(source.Candidate, binding.OperationID)
	if !found {
		return SealedDispatch{}, nil, false, nil
	}
	execution, found := operationExecutionRecord(source.OperationExecutions, binding.OperationID)
	if !found || execution.ExecutionKey != binding.ExecutionKey || execution.Fence != binding.Fence {
		return SealedDispatch{}, nil, false, fmt.Errorf("%w: confirmed owner repair governance binding does not match its source execution", common.ErrConflict)
	}
	if source.RepairCommitment == nil {
		return SealedDispatch{}, nil, false, nil
	}
	lineage, err := confirmedRepairAdmissionLineage(aggregate, *source)
	if err != nil {
		return SealedDispatch{}, nil, false, err
	}
	origin, found := lineage.origin()
	if !found {
		return SealedDispatch{}, nil, false, fmt.Errorf("%w: confirmed owner repair governance source has no origin admission", common.ErrConflict)
	}
	transaction, err := confirmedRepairTransactionFromCommitment(aggregate, *source, origin)
	if err != nil {
		return SealedDispatch{}, nil, false, err
	}
	descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
	if !found || descriptor.OwnerModuleID != transaction.confirmation.OwnerModuleID {
		return SealedDispatch{}, nil, false, fmt.Errorf("%w: confirmed repair governance operation owner does not match its confirmation", common.ErrConflict)
	}
	coverageIDs := append([]common.CoverageWorkItemID(nil), transaction.closure.CoverageWorkItemIDs...)
	if len(coverageIDs) == 0 || !sameCoverageIDs(coverageIDs, aggregate.Facts.Focus.CoverageWorkItemIDs) ||
		!confirmedRepairSourceBindingsCoverCurrentScope(aggregate, transaction.sourceOp.BindingRefs, coverageIDs) {
		return SealedDispatch{}, nil, false, fmt.Errorf("%w: confirmed owner repair governance source has no current coverage", common.ErrConflict)
	}
	return *source, transaction, true, nil
}

// confirmedRepairGovernanceReductionEvent proves the repair-specific
// admission and freshness lineage, then returns only the typed owner-private
// event input consumed by the sole SM-2 reducer. It does not construct a
// RuntimeDecision or RequiredOutput; action selection remains in reducer.go.
func confirmedRepairGovernanceReductionEvent(
	aggregate RuntimeAggregate,
	resolution runtimecontract.RuntimeGovernanceResolution,
	binding runtimecontract.RuntimeOperationGovernanceBinding,
) (*runtimeOwnerInternalReductionEvent, bool, error) {
	binding = binding.Normalized()
	if binding.State != runtimecontract.RuntimeOperationGovernanceBindingGranted || binding.Grant == nil {
		return nil, false, nil
	}
	source, transaction, matched, err := confirmedRepairGovernanceSourceTransaction(aggregate, binding)
	if err != nil || !matched {
		return nil, false, err
	}
	if len(source.CompletedOperationIDs) == 0 || len(source.CompletedOperationIDs) >= candidateOperationCount(source.Candidate) ||
		containsString(source.CompletedOperationIDs, binding.OperationID) {
		return nil, false, nil
	}
	_, receipts, err := confirmedRepairCompletedReceiptChain(aggregate, source, transaction.confirmation)
	if err != nil {
		return nil, false, err
	}
	if err := validateConfirmedRepairPartialFreshness(aggregate, source, transaction.confirmation, receipts); err != nil {
		return nil, false, err
	}
	return &runtimeOwnerInternalReductionEvent{
		Kind:                 RuntimeInternalEventGovernanceResolutionObserved,
		GovernanceResolution: resolution.Normalized(),
		SourceDispatch:       source,
		OperationBinding:     binding,
	}, true, nil
}

func currentConfirmedRepairTransaction(
	aggregate RuntimeAggregate,
	admission executionEvidenceAdmission,
	fact runtimecontract.EvidenceFact,
) (*confirmedRepairTransaction, error) {
	if aggregate.ActiveDispatch == nil || aggregate.Decision.State != runtimecontract.ConvergenceStateNeedStateChange ||
		admission.Evidence.Status != ExecutionEvidenceStatusAccepted || fact.Kind != runtimecontract.EvidenceKindStateChange ||
		admission.Evidence.Receipt == nil {
		return nil, nil
	}
	transaction, found, err := resolveConfirmedRepairTransaction(aggregate, *aggregate.ActiveDispatch)
	if err != nil || !found {
		return nil, err
	}
	descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(admission.Operation.Kind)
	if !found || descriptor.OwnerModuleID != transaction.confirmation.OwnerModuleID {
		return nil, fmt.Errorf("%w: confirmed repair operation owner does not match its confirmation", common.ErrConflict)
	}
	if _, err := runtimecontract.RuntimeDispatchOperationTargetAuthorizationForOperation(
		aggregate.ActiveDispatch.Candidate,
		aggregate.ActiveDispatch.OperationTargetAuthorizations,
		admission.Operation,
	); err != nil {
		return nil, fmt.Errorf("%w: confirmed owner repair operation target authorization: %v", common.ErrConflict, err)
	}
	if !runtimeTargetRefsWithin(admission.Operation.TargetRefs, transaction.confirmation.ConfirmedTargetRefs) {
		return nil, fmt.Errorf("%w: confirmed owner repair operation exceeds confirmed target surface", common.ErrConflict)
	}
	if err := validateConfirmedRepairDispatchOperations(*aggregate.ActiveDispatch, transaction.confirmation); err != nil {
		return nil, err
	}
	expectedWorkspaceRevision, _, err := confirmedRepairCompletedReceiptChain(aggregate, *aggregate.ActiveDispatch, transaction.confirmation)
	if err != nil {
		return nil, err
	}
	receipt := admission.Evidence.Receipt.Normalized()
	if _, err := validateConfirmedRepairAcceptedEvidenceLineage(aggregate, *aggregate.ActiveDispatch, admission.Operation, receipt); err != nil {
		return nil, err
	}
	link, err := validateConfirmedRepairResourceChanges(admission.Operation, receipt, transaction.confirmation)
	if err != nil {
		return nil, err
	}
	if link.beforeRevision != expectedWorkspaceRevision {
		return nil, fmt.Errorf("%w: confirmed owner repair workspace revision does not continue the completed receipt chain", common.ErrConflict)
	}
	return transaction, nil
}

// resolveConfirmedRepairTransaction rebuilds the repair closure only from
// immutable lineage. The current FailureTarget and RepairDirective are
// intentionally absent from this derivation: both are owner-revision-bound
// authority and are retired as soon as the first durable repair-owner receipt changes
// that revision.
func resolveConfirmedRepairTransaction(aggregate RuntimeAggregate, dispatch SealedDispatch) (*confirmedRepairTransaction, bool, error) {
	if dispatch.State != DispatchStateSealed || dispatch.Candidate.StateChange == nil ||
		aggregate.Decision.State != runtimecontract.ConvergenceStateNeedStateChange ||
		strings.TrimSpace(aggregate.Decision.RequiredOutput.RepairClosureID) == "" {
		return nil, false, nil
	}
	if dispatch.Transaction != aggregate.Decision.AuthorityTransaction() {
		return nil, false, fmt.Errorf("%w: confirmed owner repair dispatch does not match the current sealed Decision", common.ErrConflict)
	}
	admissionLineage, err := confirmedRepairAdmissionLineage(aggregate, dispatch)
	if err != nil {
		return nil, false, err
	}
	originDispatch, found := admissionLineage.origin()
	if !found {
		return nil, false, fmt.Errorf("%w: confirmed owner repair dispatch has no origin admission", common.ErrConflict)
	}
	targetRefs, repairOwnerModuleID, err := confirmedRepairDispatchTargets(dispatch)
	if err != nil {
		return nil, false, err
	}
	if repairOwnerModuleID == "" {
		return nil, false, nil
	}
	decisionCoverage := append([]common.CoverageWorkItemID(nil), aggregate.Decision.RequiredOutput.CoverageWorkItemIDs...)
	if len(decisionCoverage) == 0 || !sameCoverageIDs(decisionCoverage, aggregate.Facts.Focus.CoverageWorkItemIDs) ||
		!runtimeTargetRefsWithin(targetRefs, aggregate.Decision.RequiredOutput.TargetRefs) {
		return nil, false, fmt.Errorf("%w: confirmed owner repair dispatch scope does not match its sealed Decision", common.ErrConflict)
	}

	if dispatch.RepairCommitment == nil {
		return nil, false, nil
	}
	matched, err := confirmedRepairTransactionFromCommitment(aggregate, dispatch, originDispatch)
	if err != nil {
		return nil, false, err
	}
	if matched.confirmation.OwnerModuleID != repairOwnerModuleID {
		return nil, false, fmt.Errorf("%w: confirmed repair dispatch owner does not match its committed confirmation", common.ErrConflict)
	}
	if matched.closure.ID != aggregate.Decision.RequiredOutput.RepairClosureID ||
		!sameRuntimeTargetRefs(matched.closure.GroundedTargetRefs, aggregate.Decision.RequiredOutput.TargetRefs) ||
		!sameCoverageIDs(matched.closure.CoverageWorkItemIDs, decisionCoverage) ||
		!confirmedRepairSourceBindingsCoverCurrentScope(aggregate, matched.sourceOp.BindingRefs, decisionCoverage) {
		return nil, false, fmt.Errorf("%w: confirmed owner repair commitment does not match the current sealed Decision", common.ErrConflict)
	}
	return matched, true, nil
}

func confirmedRepairDispatchTargets(dispatch SealedDispatch) ([]string, string, error) {
	operations := candidateRuntimeOperations(dispatch.Candidate)
	if dispatch.Candidate.StateChange == nil || len(operations) == 0 {
		return nil, "", nil
	}
	var targetRefs []string
	repairOwnerModuleID := ""
	for _, rawOperation := range operations {
		operation := rawOperation.Normalized()
		descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
		if !found {
			return nil, "", fmt.Errorf("%w: confirmed repair operation %q has no descriptor", common.ErrConflict, operation.Kind)
		}
		if repairOwnerModuleID == "" {
			repairOwnerModuleID = descriptor.OwnerModuleID
		} else if repairOwnerModuleID != descriptor.OwnerModuleID {
			return nil, "", fmt.Errorf("%w: confirmed repair dispatch spans multiple operation owners", common.ErrConflict)
		}
		if len(operation.TargetRefs) == 0 {
			return nil, "", fmt.Errorf("%w: confirmed repair operation %q has no sealed target", common.ErrConflict, operation.ID)
		}
		targetRefs = append(targetRefs, operation.TargetRefs...)
	}
	return normalizeRuntimeTargetRefs(targetRefs), repairOwnerModuleID, nil
}

func confirmedRepairSourceRejection(
	aggregate RuntimeAggregate,
	confirmation runtimecontract.RuntimeRepairSurfaceConfirmation,
) (runtimecontract.EvidenceFact, runtimecontract.RuntimeOperationReceipt, runtimecontract.RuntimeOperation, error) {
	confirmation = confirmation.Normalized()
	observation := confirmation.Observation.Normalized()
	var dispatch *SealedDispatch
	for idx := range aggregate.CompletedDispatches {
		candidate := &aggregate.CompletedDispatches[idx]
		if candidate.ID == observation.DispatchID && candidate.State == DispatchStateCommitted &&
			containsString(candidate.CompletedOperationIDs, observation.OperationID) {
			if dispatch != nil {
				return runtimecontract.EvidenceFact{}, runtimecontract.RuntimeOperationReceipt{}, runtimecontract.RuntimeOperation{}, fmt.Errorf("%w: confirmed owner repair observation matches multiple source dispatches", common.ErrConflict)
			}
			dispatch = candidate
		}
	}
	if dispatch == nil || !sealedValidationFailureObservationMatchesDispatch(*dispatch, observation) {
		return runtimecontract.EvidenceFact{}, runtimecontract.RuntimeOperationReceipt{}, runtimecontract.RuntimeOperation{}, fmt.Errorf("%w: confirmed owner repair has no exact sealed validation source", common.ErrConflict)
	}
	operation, found := candidateOperationByID(dispatch.Candidate, observation.OperationID)
	if !found {
		return runtimecontract.EvidenceFact{}, runtimecontract.RuntimeOperationReceipt{}, runtimecontract.RuntimeOperation{}, fmt.Errorf("%w: confirmed owner repair source operation is not sealed", common.ErrConflict)
	}
	execution, found := operationExecutionRecord(dispatch.OperationExecutions, observation.OperationID)
	if !found || execution.State != runtimecontract.OperationExecutionEvidenceCommitted || execution.Fence == 0 {
		return runtimecontract.EvidenceFact{}, runtimecontract.RuntimeOperationReceipt{}, runtimecontract.RuntimeOperation{}, fmt.Errorf("%w: confirmed owner repair source has no evidence-committed execution", common.ErrConflict)
	}
	receipt, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(aggregate.OperationReceipts, execution.ExecutionKey, execution.Fence)
	if !found || receipt.Outcome != runtimecontract.RuntimeOperationOutcomeRejected || receipt.OperationID != operation.ID ||
		execution.OwnerReceiptRef != "runtime-receipt/"+receipt.ExecutionKey ||
		execution.OwnerReceiptHash != runtimecontract.StableRuntimeOperationReceiptHash(receipt) ||
		!runtimeValidationFailureObservationHasPersistedReceipt(aggregate, observation) {
		return runtimecontract.EvidenceFact{}, runtimecontract.RuntimeOperationReceipt{}, runtimecontract.RuntimeOperation{}, fmt.Errorf("%w: confirmed owner repair source has no exact rejected validation-owner receipt", common.ErrConflict)
	}
	expectedFailure := failureFactFromOperationReceiptFeedback(receipt)
	if expectedFailure == nil || expectedFailure.Signature != observation.FailureSignature {
		return runtimecontract.EvidenceFact{}, runtimecontract.RuntimeOperationReceipt{}, runtimecontract.RuntimeOperation{}, fmt.Errorf("%w: confirmed owner repair source receipt failure identity mismatch", common.ErrConflict)
	}
	var fact runtimecontract.EvidenceFact
	matches := 0
	for _, candidate := range aggregate.Facts.Normalized().Evidence.Rejected {
		if candidate.DispatchID != dispatch.ID || candidate.OperationID != operation.ID {
			continue
		}
		fact, matches = candidate, matches+1
	}
	if matches != 1 {
		return runtimecontract.EvidenceFact{}, runtimecontract.RuntimeOperationReceipt{}, runtimecontract.RuntimeOperation{}, fmt.Errorf("%w: confirmed owner repair source requires one receipt-derived rejected evidence fact", common.ErrConflict)
	}
	receiptHash := runtimecontract.StableRuntimeOperationReceiptHash(receipt)
	eventID := string(fact.ID)
	recoveryEventID := "runtime-receipt|" + receiptHash
	recoverySourceRef := "runtime-receipt/" + receipt.ExecutionKey
	if eventID != recoveryEventID || fact.SourceRef != recoverySourceRef {
		return runtimecontract.EvidenceFact{}, runtimecontract.RuntimeOperationReceipt{}, runtimecontract.RuntimeOperation{}, fmt.Errorf("%w: confirmed owner repair source evidence has no canonical persisted receipt-recovery identity", common.ErrConflict)
	}
	processed := 0
	for _, eventID := range aggregate.ProcessedEventIDs {
		if eventID == string(fact.ID) {
			processed++
		}
	}
	receiptEvidence := receipt.Normalized().Evidence
	if processed != 1 {
		return runtimecontract.EvidenceFact{}, runtimecontract.RuntimeOperationReceipt{}, runtimecontract.RuntimeOperation{}, fmt.Errorf("%w: confirmed owner repair source evidence has no unique processed-event fence", common.ErrConflict)
	}
	if fact.Kind != runtimecontract.EvidenceKindValidation || fact.SubjectRef != operation.ID ||
		fact.Revision == "" || !runtimecontract.EvidenceRevisionAfter(fact.Revision, dispatch.Transaction.EvidenceRevision) ||
		fact.AuthoritySnapshotID != dispatch.Transaction.AuthoritySnapshotID ||
		fact.DispatchID != dispatch.ID || fact.OperationID != operation.ID {
		return runtimecontract.EvidenceFact{}, runtimecontract.RuntimeOperationReceipt{}, runtimecontract.RuntimeOperation{}, fmt.Errorf("%w: confirmed owner repair source evidence scope/revision does not match its sealed operation", common.ErrConflict)
	}
	if !sameRuntimeTargetRefs(fact.TargetRefs, operation.TargetRefs) ||
		!sameRuntimeStringSet(fact.BindingRefs, operation.BindingRefs) ||
		!sameCoverageIDs(fact.CoverageWorkItemIDs, operation.CoverageWorkItemIDs) {
		return runtimecontract.EvidenceFact{}, runtimecontract.RuntimeOperationReceipt{}, runtimecontract.RuntimeOperation{}, fmt.Errorf("%w: confirmed owner repair source evidence target/binding/coverage does not match its sealed operation", common.ErrConflict)
	}
	if fact.FailureClass != expectedFailure.Class || fact.FailureCode != expectedFailure.Code ||
		fact.FailureSignature != expectedFailure.Signature || fact.FailureMessage != expectedFailure.Message {
		return runtimecontract.EvidenceFact{}, runtimecontract.RuntimeOperationReceipt{}, runtimecontract.RuntimeOperation{}, fmt.Errorf("%w: confirmed owner repair source failure does not match its exact validation-owner receipt", common.ErrConflict)
	}
	if fact.Summary != receiptEvidence.Summary || fact.ContentHash != receiptEvidence.ContentHash ||
		!reflect.DeepEqual(fact.ContentAddress, receiptEvidence.ContentAddress) ||
		!reflect.DeepEqual(fact.ExitCode, receiptEvidence.ExitCode) || !sameRuntimeTargetRefs(fact.EvidenceRefs, receiptEvidence.EvidenceRefs) ||
		fact.Quality != receiptEvidence.Quality || fact.ZeroWork != receiptEvidence.ZeroWork ||
		!sameRuntimeTargetRefs(fact.EvidenceTags, receiptEvidence.EvidenceTags) ||
		!reflect.DeepEqual(fact.Validation, receiptEvidence.Validation) {
		return runtimecontract.EvidenceFact{}, runtimecontract.RuntimeOperationReceipt{}, runtimecontract.RuntimeOperation{}, fmt.Errorf("%w: confirmed owner repair source evidence does not match its exact validation-owner receipt", common.ErrConflict)
	}
	if fact.EvidenceReason != receiptEvidence.EvidenceReason {
		return runtimecontract.EvidenceFact{}, runtimecontract.RuntimeOperationReceipt{}, runtimecontract.RuntimeOperation{}, fmt.Errorf("%w: confirmed owner repair source evidence reason does not match its exact validation-owner receipt", common.ErrConflict)
	}
	return fact, receipt, operation, nil
}

func confirmedRepairSourceBindingsCoverCurrentScope(aggregate RuntimeAggregate, bindingRefs []string, coverageIDs []common.CoverageWorkItemID) bool {
	bindingRefs = normalizeRuntimeTargetRefs(bindingRefs)
	if len(bindingRefs) == 0 || len(coverageIDs) == 0 {
		return false
	}
	expected := make([]string, 0, len(coverageIDs))
	for _, coverageID := range coverageIDs {
		index := runtimeCWICoverageIndex(aggregate.Facts.Coverage, coverageID)
		if index < 0 || aggregate.Facts.Coverage[index].CurrentBindingRef == nil {
			return false
		}
		expected = append(expected, aggregate.Facts.Coverage[index].CurrentBindingRef.BindingID)
	}
	return sameRuntimeStringSet(bindingRefs, expected)
}

func confirmedRepairConsecutiveEvidenceRevision(current, previous string) bool {
	const prefix = "runtime-evidence-"
	previous = strings.TrimSpace(previous)
	current = strings.TrimSpace(current)
	if !strings.HasPrefix(previous, prefix) {
		return false
	}
	sequence, err := strconv.ParseUint(strings.TrimPrefix(previous, prefix), 10, 64)
	if err != nil || sequence == ^uint64(0) || previous != nextEvidenceRevision(sequence) {
		return false
	}
	return current == nextEvidenceRevision(sequence+1)
}

func validateConfirmedRepairResourceChanges(
	operation runtimecontract.RuntimeOperation,
	receipt runtimecontract.RuntimeOperationReceipt,
	confirmation runtimecontract.RuntimeRepairSurfaceConfirmation,
) (confirmedRepairReceiptLink, error) {
	operation = operation.Normalized()
	receipt = receipt.Normalized()
	confirmation = confirmation.Normalized()
	if receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || !receipt.Changed ||
		receipt.ObservedEffectScope != runtimecontract.RuntimeEffectWorkspaceDurable ||
		len(operation.TargetRefs) == 0 || !sameRuntimeTargetRefs(receipt.TargetRefs, operation.TargetRefs) ||
		receipt.BeforeHash == "" || receipt.AfterHash == "" || receipt.BeforeHash == receipt.AfterHash ||
		len(receipt.ResourceChanges) == 0 {
		return confirmedRepairReceiptLink{}, fmt.Errorf("%w: confirmed owner repair receipt is not an accepted target-bound workspace change", common.ErrConflict)
	}
	targets := make(map[string]bool, len(operation.TargetRefs))
	for _, targetRef := range operation.TargetRefs {
		targets[strings.TrimSpace(targetRef)] = false
	}
	var workspaceTree *confirmedRepairReceiptLink
	descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
	if !found || descriptor.OwnerModuleID != confirmation.OwnerModuleID {
		return confirmedRepairReceiptLink{}, fmt.Errorf("%w: confirmed repair operation owner does not match its confirmation", common.ErrConflict)
	}
	for _, rawChange := range receipt.ResourceChanges {
		change := rawChange.Normalized()
		if change.OwnerModuleID != descriptor.OwnerModuleID {
			return confirmedRepairReceiptLink{}, fmt.Errorf("%w: confirmed owner repair receipt carries a foreign-owner resource change", common.ErrConflict)
		}
		switch change.Domain {
		case runtimecontract.RuntimeResourceWorkspaceTarget:
			if _, found := targets[change.SubjectRef]; !found || change.BeforeRevision != receipt.BeforeHash || change.AfterRevision != receipt.AfterHash {
				return confirmedRepairReceiptLink{}, fmt.Errorf("%w: confirmed owner repair target change does not match the sealed operation receipt", common.ErrConflict)
			}
			targets[change.SubjectRef] = true
		case runtimecontract.RuntimeResourceWorkspaceTree:
			if workspaceTree != nil || change.SubjectRef != confirmation.SubjectRef ||
				change.BeforeRevision == "" || change.AfterRevision == "" || change.BeforeRevision == change.AfterRevision {
				return confirmedRepairReceiptLink{}, fmt.Errorf("%w: confirmed owner repair workspace change has no exact revision edge", common.ErrConflict)
			}
			workspaceTree = &confirmedRepairReceiptLink{beforeRevision: change.BeforeRevision, afterRevision: change.AfterRevision}
		default:
			return confirmedRepairReceiptLink{}, fmt.Errorf("%w: confirmed owner repair receipt carries an unexpected resource domain", common.ErrConflict)
		}
	}
	if workspaceTree == nil {
		return confirmedRepairReceiptLink{}, fmt.Errorf("%w: confirmed owner repair receipt omits the workspace-tree revision change", common.ErrConflict)
	}
	for _, observed := range targets {
		if !observed {
			return confirmedRepairReceiptLink{}, fmt.Errorf("%w: confirmed owner repair receipt omits an operation target change", common.ErrConflict)
		}
	}
	return *workspaceTree, nil
}

func validateConfirmedRepairDispatchOperations(dispatch SealedDispatch, confirmation runtimecontract.RuntimeRepairSurfaceConfirmation) error {
	if dispatch.Candidate.StateChange == nil || len(candidateRuntimeOperations(dispatch.Candidate)) == 0 {
		return fmt.Errorf("%w: confirmed owner repair transaction requires a state-change dispatch", common.ErrConflict)
	}
	for _, rawOperation := range candidateRuntimeOperations(dispatch.Candidate) {
		operation := rawOperation.Normalized()
		descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
		if !found || descriptor.OwnerModuleID != confirmation.OwnerModuleID ||
			!runtimeTargetRefsWithin(operation.TargetRefs, confirmation.ConfirmedTargetRefs) {
			return fmt.Errorf("%w: confirmed owner repair transaction contains an unauthorized operation", common.ErrConflict)
		}
		if _, err := runtimecontract.RuntimeDispatchOperationTargetAuthorizationForOperation(
			dispatch.Candidate,
			dispatch.OperationTargetAuthorizations,
			operation,
		); err != nil {
			return fmt.Errorf("%w: confirmed owner repair transaction target authorization: %v", common.ErrConflict, err)
		}
	}
	// all-of completion is defined by the sealed candidate operations. The
	// confirmation remains their bounded authorization surface and is not a
	// hidden requirement to produce an effect for every confirmed target.
	return nil
}

// confirmedRepairCompletedReceiptChain reconstructs only the revision
// order already proved by evidence-committed owner receipts. Operation IDs are
// normalized as a set in the dispatch, so their lexical order is never treated
// as execution order. A unique before->after workspace-tree chain rooted at
// the owner confirmation is the sole ordering authority.
func confirmedRepairCompletedReceiptChain(
	aggregate RuntimeAggregate,
	dispatch SealedDispatch,
	confirmation runtimecontract.RuntimeRepairSurfaceConfirmation,
) (string, map[string]runtimecontract.RuntimeOperationReceipt, error) {
	confirmation = confirmation.Normalized()
	if err := validateConfirmedRepairDispatchOperations(dispatch, confirmation); err != nil {
		return "", nil, err
	}
	receipts := make(map[string]runtimecontract.RuntimeOperationReceipt, len(dispatch.CompletedOperationIDs))
	links := make(map[string]confirmedRepairReceiptLink, len(dispatch.CompletedOperationIDs))
	evidenceByOperation := make(map[string]runtimecontract.EvidenceFact, len(dispatch.CompletedOperationIDs))
	for _, operationID := range dispatch.CompletedOperationIDs {
		operation, found := candidateOperationByID(dispatch.Candidate, operationID)
		if !found {
			return "", nil, fmt.Errorf("%w: confirmed owner repair transaction completed operation is not sealed", common.ErrConflict)
		}
		execution, found := operationExecutionRecord(dispatch.OperationExecutions, operationID)
		if !found || execution.State != runtimecontract.OperationExecutionEvidenceCommitted || execution.Fence == 0 {
			return "", nil, fmt.Errorf("%w: confirmed owner repair transaction has no evidence-committed execution", common.ErrConflict)
		}
		receipt, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(aggregate.OperationReceipts, execution.ExecutionKey, execution.Fence)
		if !found || receipt.OperationID != operationID || execution.OwnerReceiptRef != "runtime-receipt/"+receipt.ExecutionKey ||
			execution.OwnerReceiptHash != runtimecontract.StableRuntimeOperationReceiptHash(receipt) {
			return "", nil, fmt.Errorf("%w: confirmed owner repair transaction has no exact owner receipt", common.ErrConflict)
		}
		evidence, err := validateConfirmedRepairAcceptedEvidenceLineage(aggregate, dispatch, operation, receipt)
		if err != nil {
			return "", nil, err
		}
		link, err := validateConfirmedRepairResourceChanges(operation, receipt, confirmation)
		if err != nil {
			return "", nil, fmt.Errorf("%w: confirmed owner repair transaction receipt: %v", common.ErrConflict, err)
		}
		receipts[operationID] = receipt
		links[operationID] = link
		evidenceByOperation[operationID] = evidence
	}

	admissionLineage, err := confirmedRepairAdmissionLineage(aggregate, dispatch)
	if err != nil {
		return "", nil, err
	}
	originDispatch, found := admissionLineage.origin()
	if !found {
		return "", nil, fmt.Errorf("%w: confirmed owner repair receipt chain has no origin admission", common.ErrConflict)
	}
	expectedRevision := confirmation.ObservedRevision
	expectedEvidenceRevision := originDispatch.Transaction.EvidenceRevision
	currentDispatchIndex := 0
	remaining := make(map[string]confirmedRepairReceiptLink, len(links))
	for operationID, link := range links {
		remaining[operationID] = link
	}
	for len(remaining) != 0 {
		matchedOperationID := ""
		var matched confirmedRepairReceiptLink
		for operationID, link := range remaining {
			if link.beforeRevision != expectedRevision {
				continue
			}
			if matchedOperationID != "" {
				return "", nil, fmt.Errorf("%w: confirmed owner repair receipts branch from one workspace revision", common.ErrConflict)
			}
			matchedOperationID, matched = operationID, link
		}
		if matchedOperationID == "" {
			return "", nil, fmt.Errorf("%w: confirmed owner repair receipts do not form a complete workspace revision chain", common.ErrConflict)
		}
		accepted := evidenceByOperation[matchedOperationID]
		ownerDispatchIndex := admissionLineage.dispatchIndex(accepted.DispatchID)
		if ownerDispatchIndex < currentDispatchIndex {
			return "", nil, fmt.Errorf("%w: confirmed owner repair accepted evidence regresses across governance dispatch lineage", common.ErrConflict)
		}
		for currentDispatchIndex < ownerDispatchIndex {
			binding := admissionLineage.bindings[currentDispatchIndex]
			governanceRevision, governanceErr := confirmedRepairGovernanceEvidenceRevision(aggregate, binding)
			if governanceErr != nil {
				return "", nil, governanceErr
			}
			child := admissionLineage.dispatches[currentDispatchIndex+1]
			if child.Transaction.EvidenceRevision != governanceRevision ||
				!confirmedRepairConsecutiveEvidenceRevision(governanceRevision, expectedEvidenceRevision) {
				return "", nil, fmt.Errorf("%w: confirmed owner repair evidence revision gap has no exact governance-resolution lineage", common.ErrConflict)
			}
			expectedEvidenceRevision = governanceRevision
			currentDispatchIndex++
		}
		if !confirmedRepairConsecutiveEvidenceRevision(accepted.Revision, expectedEvidenceRevision) {
			return "", nil, fmt.Errorf("%w: confirmed owner repair accepted evidence revisions do not form a contiguous dispatch chain", common.ErrConflict)
		}
		delete(remaining, matchedOperationID)
		expectedRevision = matched.afterRevision
		expectedEvidenceRevision = accepted.Revision
	}
	return expectedRevision, receipts, nil
}

func confirmedRepairGovernanceEvidenceRevision(aggregate RuntimeAggregate, binding runtimecontract.RuntimeOperationGovernanceBinding) (string, error) {
	binding = binding.Normalized()
	if binding.Grant == nil {
		return "", fmt.Errorf("%w: confirmed owner repair governance hop has no grant", common.ErrConflict)
	}
	grant := binding.Grant.Normalized()
	wantID := common.EvidenceRecordID("governance-resolution:" + string(grant.ResolutionID) + ":" + binding.GovernanceSnapshotID)
	wantSource := "governance-resolution/" + string(grant.ResolutionID)
	var matched runtimecontract.EvidenceFact
	matches := 0
	for _, fact := range aggregate.Facts.Normalized().Evidence.Accepted {
		if fact.ID != wantID || fact.Kind != runtimecontract.EvidenceKindGovernance || fact.SourceRef != wantSource ||
			fact.DispatchID != binding.GovernanceDispatchID || fact.AuthoritySnapshotID != binding.GovernanceSnapshotID ||
			fact.ContentHash != grant.ResolutionHash || !sameRuntimeTargetRefs(fact.EvidenceRefs, grant.EvidenceRefs) {
			continue
		}
		matched, matches = fact, matches+1
	}
	processed := 0
	for _, eventID := range aggregate.ProcessedEventIDs {
		if eventID == string(wantID) {
			processed++
		}
	}
	if matches != 1 || processed != 1 || matched.Revision == "" {
		return "", fmt.Errorf("%w: confirmed owner repair governance hop has no unique accepted resolution evidence", common.ErrConflict)
	}
	return matched.Revision, nil
}

// completedConfirmedRepairTransactions reconstructs the immutable source
// and effect chain for committed confirmed-owner repairs. It deliberately does
// not consult RepairDirective or FailureTarget: those mutable projections are
// what this reverse proof is responsible for validating.
func completedConfirmedRepairTransactions(aggregate RuntimeAggregate) ([]completedConfirmedRepairTransaction, error) {
	ret := make([]completedConfirmedRepairTransaction, 0)
	for _, rawDispatch := range aggregate.CompletedDispatches {
		dispatch := rawDispatch
		if dispatch.State != DispatchStateCommitted || dispatch.Candidate.StateChange == nil ||
			dispatch.RepairCommitment == nil {
			continue
		}
		admissionLineage, err := confirmedRepairAdmissionLineage(aggregate, dispatch)
		if err != nil {
			return nil, err
		}
		originDispatch, found := admissionLineage.origin()
		if !found {
			return nil, fmt.Errorf("%w: completed confirmed owner repair has no origin admission", common.ErrConflict)
		}
		transaction, err := confirmedRepairTransactionFromCommitment(aggregate, dispatch, originDispatch)
		if err != nil {
			return nil, err
		}
		// A committed partial/rejected/governance-suspended dispatch still must
		// retain exact purpose lineage, but only an all-accepted full completion
		// creates the mandatory fresh-validation continuation.
		if len(dispatch.CompletedOperationIDs) != candidateOperationCount(dispatch.Candidate) {
			continue
		}
		allAccepted, err := confirmedRepairCompletedDispatchAllAccepted(aggregate, dispatch)
		if err != nil {
			return nil, err
		}
		if !allAccepted {
			continue
		}
		finalWorkspaceRevision, receipts, err := confirmedRepairCompletedReceiptChain(aggregate, dispatch, transaction.confirmation)
		if err != nil {
			return nil, err
		}
		finalEvidenceRevision, err := confirmedRepairFinalEvidenceRevision(aggregate, dispatch, receipts, finalWorkspaceRevision)
		if err != nil {
			return nil, err
		}
		ret = append(ret, completedConfirmedRepairTransaction{
			transaction: *transaction, dispatch: dispatch, finalEvidenceRevision: finalEvidenceRevision,
			finalWorkspaceRevision: finalWorkspaceRevision,
		})
	}
	return ret, nil
}

func confirmedRepairCompletedDispatchAllAccepted(aggregate RuntimeAggregate, dispatch SealedDispatch) (bool, error) {
	for _, operationID := range dispatch.CompletedOperationIDs {
		execution, found := operationExecutionRecord(dispatch.OperationExecutions, operationID)
		if !found || execution.State != runtimecontract.OperationExecutionEvidenceCommitted || execution.Fence == 0 {
			return false, fmt.Errorf("%w: completed confirmed owner repair has no evidence-committed execution", common.ErrConflict)
		}
		receipt, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(aggregate.OperationReceipts, execution.ExecutionKey, execution.Fence)
		if !found || receipt.OperationID != operationID || execution.OwnerReceiptRef != "runtime-receipt/"+receipt.ExecutionKey ||
			execution.OwnerReceiptHash != runtimecontract.StableRuntimeOperationReceiptHash(receipt) {
			return false, fmt.Errorf("%w: completed confirmed owner repair has no exact owner receipt", common.ErrConflict)
		}
		if receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted {
			return false, nil
		}
	}
	return true, nil
}

func confirmedRepairFinalEvidenceRevision(
	aggregate RuntimeAggregate,
	dispatch SealedDispatch,
	receipts map[string]runtimecontract.RuntimeOperationReceipt,
	finalWorkspaceRevision string,
) (string, error) {
	finalOperationID := ""
	for operationID, rawReceipt := range receipts {
		receipt := rawReceipt.Normalized()
		for _, rawChange := range receipt.ResourceChanges {
			change := rawChange.Normalized()
			if change.Domain != runtimecontract.RuntimeResourceWorkspaceTree || change.AfterRevision != finalWorkspaceRevision {
				continue
			}
			if finalOperationID != "" {
				return "", fmt.Errorf("%w: completed confirmed owner repair has multiple final workspace revision receipts", common.ErrConflict)
			}
			finalOperationID = operationID
		}
	}
	if finalOperationID == "" {
		return "", fmt.Errorf("%w: completed confirmed owner repair has no final workspace revision receipt", common.ErrConflict)
	}
	operation, found := candidateOperationByID(dispatch.Candidate, finalOperationID)
	if !found {
		return "", fmt.Errorf("%w: completed confirmed owner repair final operation is not sealed", common.ErrConflict)
	}
	fact, err := validateConfirmedRepairAcceptedEvidenceLineage(aggregate, dispatch, operation, receipts[finalOperationID])
	if err != nil || fact.Revision == "" {
		if err != nil {
			return "", err
		}
		return "", fmt.Errorf("%w: completed confirmed owner repair has no unique final accepted evidence revision", common.ErrConflict)
	}
	return fact.Revision, nil
}

func validateConfirmedRepairAcceptedEvidenceLineage(
	aggregate RuntimeAggregate,
	dispatch SealedDispatch,
	operation runtimecontract.RuntimeOperation,
	receipt runtimecontract.RuntimeOperationReceipt,
) (runtimecontract.EvidenceFact, error) {
	receipt = receipt.Normalized()
	receiptHash := runtimecontract.StableRuntimeOperationReceiptHash(receipt)
	execution, found := operationExecutionRecord(dispatch.OperationExecutions, operation.ID)
	if !found || execution.State != runtimecontract.OperationExecutionEvidenceCommitted || execution.EvidenceFactID == "" ||
		execution.EvidenceFactHash == "" || execution.ExecutionKey != receipt.ExecutionKey || execution.Fence != receipt.Fence {
		return runtimecontract.EvidenceFact{}, fmt.Errorf("%w: confirmed owner repair accepted evidence has no exact committed execution projection", common.ErrConflict)
	}
	var evidence runtimecontract.EvidenceFact
	evidenceMatches := 0
	for _, fact := range aggregate.Facts.Evidence.Accepted {
		if fact.ID != execution.EvidenceFactID {
			continue
		}
		evidence, evidenceMatches = fact, evidenceMatches+1
	}
	if evidenceMatches != 1 || runtimecontract.StableRuntimeEvidenceFactHash(evidence) != execution.EvidenceFactHash {
		return runtimecontract.EvidenceFact{}, fmt.Errorf("%w: confirmed owner repair accepted evidence does not match its execution-ledger fact id/hash", common.ErrConflict)
	}
	admissionLineage, err := confirmedRepairAdmissionLineage(aggregate, dispatch)
	if err != nil {
		return runtimecontract.EvidenceFact{}, err
	}
	ownerIndex := admissionLineage.dispatchIndex(evidence.DispatchID)
	if ownerIndex < 0 {
		return runtimecontract.EvidenceFact{}, fmt.Errorf("%w: confirmed owner repair accepted evidence dispatch is outside governance admission lineage", common.ErrConflict)
	}
	ownerDispatch := admissionLineage.dispatches[ownerIndex]
	ownerExecution, found := operationExecutionRecord(ownerDispatch.OperationExecutions, operation.ID)
	if !found || !reflect.DeepEqual(ownerExecution.Normalized(), execution.Normalized()) {
		return runtimecontract.EvidenceFact{}, fmt.Errorf("%w: confirmed owner repair governance resume changed a completed execution projection", common.ErrConflict)
	}
	eventID := string(evidence.ID)
	processedMatches := 0
	for _, processedEventID := range aggregate.ProcessedEventIDs {
		if processedEventID == eventID {
			processedMatches++
		}
	}
	if processedMatches != 1 {
		return runtimecontract.EvidenceFact{}, fmt.Errorf("%w: confirmed owner repair transaction requires one receipt-derived accepted evidence fact and processed-event fence", common.ErrConflict)
	}
	recoveryEventID := "runtime-receipt|" + receiptHash
	recoverySourceRef := "runtime-receipt/" + receipt.ExecutionKey
	if eventID != recoveryEventID || evidence.SourceRef != recoverySourceRef {
		return runtimecontract.EvidenceFact{}, fmt.Errorf("%w: confirmed owner repair accepted evidence has no canonical persisted receipt identity", common.ErrConflict)
	}
	receiptEvidence := receipt.Evidence
	if evidence.Kind != runtimecontract.EvidenceKindStateChange || evidence.SubjectRef != operation.ID ||
		evidence.AuthoritySnapshotID != ownerDispatch.Transaction.AuthoritySnapshotID ||
		evidence.DispatchID != ownerDispatch.ID || evidence.OperationID != operation.ID || evidence.FailureClass != "" ||
		evidence.FailureCode != "" || evidence.FailureSignature != "" || evidence.FailureMessage != "" ||
		!sameRuntimeTargetRefs(evidence.TargetRefs, operation.TargetRefs) ||
		!sameRuntimeStringSet(evidence.BindingRefs, operation.BindingRefs) || evidence.Summary != receiptEvidence.Summary ||
		evidence.ContentHash != receiptEvidence.ContentHash || !reflect.DeepEqual(evidence.ContentAddress, receiptEvidence.ContentAddress) ||
		!reflect.DeepEqual(evidence.ExitCode, receiptEvidence.ExitCode) ||
		!sameRuntimeTargetRefs(evidence.EvidenceRefs, receiptEvidence.EvidenceRefs) || evidence.Quality != receiptEvidence.Quality ||
		evidence.ZeroWork != receiptEvidence.ZeroWork || evidence.EvidenceReason != receiptEvidence.EvidenceReason ||
		!sameRuntimeTargetRefs(evidence.EvidenceTags, receiptEvidence.EvidenceTags) || !reflect.DeepEqual(evidence.Validation, receiptEvidence.Validation) {
		return runtimecontract.EvidenceFact{}, fmt.Errorf("%w: confirmed owner repair accepted evidence does not match its exact receipt and sealed operation", common.ErrConflict)
	}
	return evidence, nil
}

func runtimeTargetRefsWithin(values, allowed []string) bool {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, value := range normalizeRuntimeTargetRefs(allowed) {
		allowedSet[value] = struct{}{}
	}
	values = normalizeRuntimeTargetRefs(values)
	if len(values) == 0 {
		return false
	}
	for _, value := range values {
		if _, found := allowedSet[value]; !found {
			return false
		}
	}
	return true
}

// validateConfirmedRepairMandatoryCommitments is independent from the
// typed manifest-purpose marker. A regression that omits or coordinates away
// that marker still cannot turn an exact rejected managed-validation
// observation followed by an single-owner same-focus repair into an ordinary
// dispatch. The immutable receipt/evidence and accepted dispatch preimages
// prove only that a commitment is mandatory; the commitment remains the sole
// reverse key for confirmation, closure and fresh-validation settlement.
func validateConfirmedRepairMandatoryCommitments(aggregate RuntimeAggregate) error {
	dispatches := make([]SealedDispatch, 0, len(aggregate.CompletedDispatches)+1)
	dispatches = append(dispatches, aggregate.CompletedDispatches...)
	if aggregate.ActiveDispatch != nil {
		dispatches = append(dispatches, *aggregate.ActiveDispatch)
	}
	for _, dispatch := range dispatches {
		required, err := confirmedRepairCommitmentRequiredByRejectedObservation(aggregate, dispatch)
		if err != nil {
			return err
		}
		if required && dispatch.RepairCommitment == nil {
			return fmt.Errorf("runtime aggregate confirmed owner repair dispatch %q omitted its mandatory rejected-validation commitment", dispatch.ID)
		}
	}
	return nil
}

func confirmedRepairCommitmentRequiredByRejectedObservation(aggregate RuntimeAggregate, dispatch SealedDispatch) (bool, error) {
	if dispatch.Candidate.StateChange == nil || dispatch.Transaction.State != runtimecontract.ConvergenceStateNeedStateChange {
		return false, nil
	}
	targetRefs, repairOwnerModuleID, err := confirmedRepairDispatchTargets(dispatch)
	if err != nil || repairOwnerModuleID == "" {
		return false, err
	}
	operations := candidateRuntimeOperations(dispatch.Candidate)
	matches := 0
	facts := aggregate.Facts.Normalized()
	for _, fact := range facts.Evidence.Rejected {
		if fact.Kind != runtimecontract.EvidenceKindValidation || fact.FocusID != dispatch.Transaction.FocusID ||
			fact.Revision != dispatch.Transaction.EvidenceRevision || fact.DispatchID == "" || fact.OperationID == "" {
			continue
		}
		var sourceDispatch *SealedDispatch
		for index := range aggregate.CompletedDispatches {
			candidate := &aggregate.CompletedDispatches[index]
			if candidate.ID != fact.DispatchID || candidate.State != DispatchStateCommitted ||
				!containsString(candidate.CompletedOperationIDs, fact.OperationID) {
				continue
			}
			if sourceDispatch != nil {
				return false, fmt.Errorf("%w: confirmed owner repair mandatory source dispatch is duplicated", common.ErrConflict)
			}
			sourceDispatch = candidate
		}
		if sourceDispatch == nil {
			continue
		}
		sourceOperation, found := candidateOperationByID(sourceDispatch.Candidate, fact.OperationID)
		if !found {
			continue
		}
		execution, found := operationExecutionRecord(sourceDispatch.OperationExecutions, sourceOperation.ID)
		if !found || execution.State != runtimecontract.OperationExecutionEvidenceCommitted || execution.Fence == 0 {
			continue
		}
		receipt, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(
			aggregate.OperationReceipts, execution.ExecutionKey, execution.Fence,
		)
		if !found || receipt.Outcome != runtimecontract.RuntimeOperationOutcomeRejected {
			continue
		}
		failure := failureFactFromOperationReceiptFeedback(receipt)
		observation := runtimeValidationFailureObservationFromFailure(failure)
		if observation == nil || observation.DispatchID != fact.DispatchID || observation.OperationID != fact.OperationID ||
			observation.AuthoritySnapshotID != fact.AuthoritySnapshotID || observation.FailureSignature != fact.FailureSignature ||
			observation.TargetRole != dispatch.FocusTargetRole || !runtimeTargetRefsWithin(targetRefs, observation.CandidateTargetRefs) {
			continue
		}
		expectedOwner, ownerErr := runtimeExpectedRepairSurfaceOwner(aggregate, *observation)
		if ownerErr != nil || expectedOwner != repairOwnerModuleID {
			continue
		}
		// Reuse the full receipt/evidence reverse proof without depending on the
		// mutable confirmation ledger: the sealed Profile/binding owner plus source
		// rejection is sufficient to prove that a commitment was mandatory. The
		// commitment remains the reverse key for the exact confirmation itself.
		sourceFact, _, exactSourceOperation, sourceErr := confirmedRepairSourceRejection(
			aggregate,
			runtimecontract.RuntimeRepairSurfaceConfirmation{Observation: observation.Clone()},
		)
		if sourceErr != nil {
			return false, sourceErr
		}
		if sourceFact.ID != fact.ID || sourceFact.Revision != dispatch.Transaction.EvidenceRevision ||
			!sameRuntimeStringSet(sourceFact.BindingRefs, exactSourceOperation.BindingRefs) {
			return false, fmt.Errorf("%w: confirmed owner repair mandatory source evidence drifted", common.ErrConflict)
		}
		for _, operation := range operations {
			operation = operation.Normalized()
			if !sameRuntimeStringSet(operation.BindingRefs, exactSourceOperation.BindingRefs) ||
				!sameCoverageIDs(operation.CoverageWorkItemIDs, exactSourceOperation.CoverageWorkItemIDs) {
				return false, fmt.Errorf("%w: confirmed owner repair mandatory dispatch does not retain its source binding", common.ErrConflict)
			}
		}
		matches++
	}
	if matches > 1 {
		return false, fmt.Errorf("%w: confirmed owner repair dispatch has multiple mandatory rejected-validation sources", common.ErrConflict)
	}
	return matches == 1, nil
}

// validatePendingConfirmedRepairTransaction is the fresh-load proof for a
// partially completed single-owner all-of dispatch. It validates only immutable
// transaction lineage and explicitly rejects restoration of the superseded
// target/directive.
func validatePendingConfirmedRepairTransaction(aggregate RuntimeAggregate) error {
	if err := validateConfirmedRepairMandatoryCommitments(aggregate); err != nil {
		return err
	}
	if aggregate.ActiveDispatch == nil || len(aggregate.ActiveDispatch.CompletedOperationIDs) == 0 {
		return nil
	}
	dispatch := *aggregate.ActiveDispatch
	if dispatch.RepairCommitment == nil {
		return nil
	}
	if dispatch.Candidate.StateChange == nil || aggregate.Decision.State != runtimecontract.ConvergenceStateNeedStateChange ||
		aggregate.Decision.RequiredOutput.RepairClosureID == "" {
		return fmt.Errorf("runtime aggregate pending confirmed owner repair commitment has an invalid dispatch shape")
	}
	_, repairOwnerModuleID, err := confirmedRepairDispatchTargets(dispatch)
	if err != nil {
		return err
	}
	if repairOwnerModuleID == "" || repairOwnerModuleID != dispatch.RepairCommitment.Normalized().RepairOwnerModuleID {
		return fmt.Errorf("runtime aggregate pending confirmed repair commitment has no unique committed operation owner")
	}
	if len(dispatch.CompletedOperationIDs) >= candidateOperationCount(dispatch.Candidate) {
		return fmt.Errorf("runtime aggregate pending confirmed owner repair transaction has no remaining sealed operation")
	}
	transaction, found, err := resolveConfirmedRepairTransaction(aggregate, dispatch)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("runtime aggregate pending confirmed owner repair transaction has no exact immutable confirmation lineage")
	}
	if aggregate.Facts.RepairDirective != nil || aggregate.Facts.Resources.FailureTarget != nil {
		return fmt.Errorf("runtime aggregate pending confirmed owner repair transaction restored superseded mutable authority (directive=%t target=%t)", aggregate.Facts.RepairDirective != nil, aggregate.Facts.Resources.FailureTarget != nil)
	}
	_, receipts, err := confirmedRepairCompletedReceiptChain(aggregate, dispatch, transaction.confirmation)
	if err != nil {
		return fmt.Errorf("runtime aggregate pending confirmed owner repair transaction: %w", err)
	}
	return validateConfirmedRepairPartialFreshness(aggregate, dispatch, transaction.confirmation, receipts)
}

func pendingConfirmedRepairTransactionCovers(aggregate RuntimeAggregate, coverageID common.CoverageWorkItemID) (bool, error) {
	if aggregate.ActiveDispatch == nil || len(aggregate.ActiveDispatch.CompletedOperationIDs) == 0 ||
		aggregate.Facts.RepairDirective != nil || aggregate.Facts.Resources.FailureTarget != nil {
		return false, nil
	}
	if err := validatePendingConfirmedRepairTransaction(aggregate); err != nil {
		return false, err
	}
	transaction, found, err := resolveConfirmedRepairTransaction(aggregate, *aggregate.ActiveDispatch)
	if err != nil || !found {
		return false, err
	}
	for _, value := range transaction.closure.CoverageWorkItemIDs {
		if value == coverageID {
			return true, nil
		}
	}
	return false, nil
}

func validateConfirmedRepairPartialFreshness(
	aggregate RuntimeAggregate,
	dispatch SealedDispatch,
	confirmation runtimecontract.RuntimeRepairSurfaceConfirmation,
	receipts map[string]runtimecontract.RuntimeOperationReceipt,
) error {
	invalidationDispatchByOperation := make(map[string]string, len(receipts))
	for operationID, receipt := range receipts {
		operation, found := candidateOperationByID(dispatch.Candidate, operationID)
		if !found {
			return fmt.Errorf("runtime aggregate pending confirmed owner repair transaction freshness operation is not sealed")
		}
		fact, err := validateConfirmedRepairAcceptedEvidenceLineage(aggregate, dispatch, operation, receipt)
		if err != nil {
			return err
		}
		invalidationDispatchByOperation[operationID] = fact.DispatchID
	}
	invalidatedPrerequisites := []string{}
	invalidatedObservations := []string{}
	relevantConfirmationRevision := false
	invalidationByOperation := map[string]bool{}
	for _, rawInvalidation := range aggregate.PrerequisiteInvalidations {
		invalidation := rawInvalidation.Normalized()
		if invalidation.DispatchID != invalidationDispatchByOperation[invalidation.OperationID] {
			continue
		}
		receipt, found := receipts[invalidation.OperationID]
		if !found || invalidationByOperation[invalidation.OperationID] {
			return fmt.Errorf("runtime aggregate pending confirmed owner repair transaction has an invalidation outside its unique completed receipt chain")
		}
		invalidationByOperation[invalidation.OperationID] = true
		receiptHash := runtimecontract.StableRuntimeOperationReceiptHash(receipt)
		if invalidation.ReceiptHash != receiptHash ||
			invalidation.Signature != runtimecontract.StableRuntimeResourceChangeInvalidationSignature(receiptHash, receipt.ResourceChanges) ||
			!reflect.DeepEqual(invalidation.ResourceChanges, runtimecontract.NormalizeRuntimeResourceChanges(receipt.ResourceChanges)) {
			return fmt.Errorf("runtime aggregate pending confirmed owner repair transaction has a mismatched freshness invalidation")
		}
		invalidatedPrerequisites = append(invalidatedPrerequisites, invalidation.PrerequisiteIDs...)
		invalidatedObservations = append(invalidatedObservations, invalidation.ObservationIDs...)
		for _, rawDependency := range invalidation.InvalidatedDependencies {
			dependency := rawDependency.Normalized()
			if dependency.Domain == runtimecontract.RuntimeResourceWorkspaceTree &&
				dependency.OwnerModuleID == confirmation.OwnerModuleID && dependency.SubjectRef == confirmation.SubjectRef &&
				dependency.Revision == confirmation.ObservedRevision {
				relevantConfirmationRevision = true
			}
		}
	}
	invalidatedPrerequisites = normalizedFreshnessStrings(invalidatedPrerequisites)
	invalidatedObservations = normalizedFreshnessStrings(invalidatedObservations)
	pendingIDs := make([]string, 0, len(aggregate.Facts.Prerequisites.Pending))
	for _, prerequisite := range aggregate.Facts.Prerequisites.Pending {
		pendingIDs = append(pendingIDs, prerequisite.ID)
	}
	if len(invalidatedPrerequisites) == 0 || !relevantConfirmationRevision || !sameRuntimeStringSet(invalidatedPrerequisites, pendingIDs) {
		return fmt.Errorf("runtime aggregate pending confirmed owner repair transaction does not exactly bind current pending prerequisites to the confirmed revision invalidation")
	}
	for _, prerequisite := range aggregate.Facts.Prerequisites.Satisfied {
		if containsString(invalidatedPrerequisites, prerequisite.ID) {
			return fmt.Errorf("runtime aggregate pending confirmed owner repair transaction retains an invalidated satisfied prerequisite")
		}
	}
	for _, observation := range aggregate.Facts.Resources.PrerequisiteObservations {
		if containsString(invalidatedObservations, observation.ObservationID) || containsString(invalidatedPrerequisites, observation.PrerequisiteID) {
			return fmt.Errorf("runtime aggregate pending confirmed owner repair transaction retains an invalidated owner observation")
		}
	}
	return nil
}

func (t confirmedRepairTransaction) stateChangeDirective(facts runtimecontract.RuntimeFacts, decision runtimecontract.RuntimeDecision) runtimecontract.RuntimeRepairDirective {
	facts = facts.Normalized()
	decision.RequiredOutput = decision.RequiredOutput.Normalized()
	closure := t.closure.Clone()
	return runtimecontract.RuntimeRepairDirective{
		ID:                         fmt.Sprintf("confirmed-repair/%s/%s/state-change", facts.Focus.ID, closure.ID),
		FocusID:                    facts.Focus.ID,
		Trigger:                    runtimecontract.RuntimeRepairTriggerRejectedValidation,
		Purpose:                    runtimecontract.RuntimeDispatchRepairPurposeConfirmedValidation,
		RequiredState:              runtimecontract.ConvergenceStateNeedStateChange,
		RequiredEffectScopes:       append([]runtimecontract.RuntimeEffectScope(nil), decision.RequiredOutput.RequiredEffectScopes...),
		RequiredEvidenceKinds:      []string{runtimecontract.EvidenceKindStateChange},
		AllowedOperationKinds:      append([]string(nil), decision.RequiredOutput.AcceptedKinds...),
		NoProgressOperationShapes:  append([]runtimecontract.RuntimeRejectedOperationShape(nil), facts.Repair.NoProgressOperationShapes...),
		CoverageWorkItemIDs:        append([]common.CoverageWorkItemID(nil), closure.CoverageWorkItemIDs...),
		TargetRefs:                 append([]string(nil), closure.GroundedTargetRefs...),
		Closure:                    &closure,
		SupersedesEvidenceRevision: closure.BaselineEvidenceRevision,
		RepairAttempt:              facts.Repair.RepairAttempts,
	}.Normalized()
}

func (t confirmedRepairTransaction) freshValidationDirective(facts runtimecontract.RuntimeFacts, supersedesRevision string) runtimecontract.RuntimeRepairDirective {
	facts = facts.Normalized()
	closure := t.closure.Clone()
	return runtimecontract.RuntimeRepairDirective{
		ID:                    fmt.Sprintf("confirmed-repair/%s/%s/fresh-validation", facts.Focus.ID, closure.ID),
		FocusID:               facts.Focus.ID,
		Trigger:               runtimecontract.RuntimeRepairTriggerRejectedValidation,
		RequiredState:         runtimecontract.ConvergenceStateNeedValidation,
		RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone, runtimecontract.RuntimeEffectEphemeralSession},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
		AllowedOperationKinds: focusAcceptedKinds(facts.Capabilities.ValidationKinds, facts.Focus),
		NoProgressOperationShapes: append([]runtimecontract.RuntimeRejectedOperationShape(nil),
			facts.Repair.NoProgressOperationShapes...),
		CoverageWorkItemIDs:        append([]common.CoverageWorkItemID(nil), closure.CoverageWorkItemIDs...),
		Closure:                    &closure,
		SupersedesEvidenceRevision: strings.TrimSpace(supersedesRevision),
		RepairAttempt:              facts.Repair.RepairAttempts + 1,
	}.Normalized()
}

// runtimeConfirmedRepairOwnsFreshValidationDirective is the facts-only
// reducer guard for the exact target-free handoff created by
// settleConfirmedRepairDefinitionRepair. Aggregate fresh-load separately proves
// the completed repair-owner transaction and receipt lineage; this helper only keeps
// generic directive synchronization from copying audit-only closure targets
// back into action authority while prerequisites are reacquired.
func runtimeConfirmedRepairOwnsFreshValidationDirective(facts runtimecontract.RuntimeFacts) bool {
	facts = facts.Normalized()
	if facts.RepairDirective == nil || facts.RepairDirective.Closure == nil || len(facts.Focus.CoverageWorkItemIDs) == 0 {
		return false
	}
	directive := facts.RepairDirective.Normalized()
	closure := directive.Closure.Normalized()
	if directive.ID != fmt.Sprintf("confirmed-repair/%s/%s/fresh-validation", facts.Focus.ID, closure.ID) ||
		directive.FocusID != facts.Focus.ID || directive.Trigger != runtimecontract.RuntimeRepairTriggerRejectedValidation ||
		directive.Purpose != "" || directive.CurrentFailure != nil || directive.RequiredState != runtimecontract.ConvergenceStateNeedValidation ||
		len(directive.TargetRefs) != 0 || !sameCoverageIDs(directive.CoverageWorkItemIDs, facts.Focus.CoverageWorkItemIDs) ||
		!sameCoverageIDs(closure.CoverageWorkItemIDs, facts.Focus.CoverageWorkItemIDs) ||
		!closure.RequireValidationAfterStateChange || len(closure.GroundedTargetRefs) == 0 ||
		!sameRuntimeStringSet(closure.RequiredEvidenceKinds, []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}) ||
		directive.SupersedesEvidenceRevision == "" || repairDirectiveEvidenceSatisfied(facts, directive) {
		return false
	}
	for _, coverageID := range closure.CoverageWorkItemIDs {
		index := runtimeCWICoverageIndex(facts.Coverage, coverageID)
		if index < 0 || facts.Coverage[index].CurrentBindingRef == nil ||
			facts.Coverage[index].State == runtimecontract.RuntimeCoverageItemStateSatisfied {
			return false
		}
	}
	return true
}

// validateConfirmedRepairFreshValidationContinuation is the fresh-load
// reverse proof for the target-free NeedValidation handoff. Every applicable
// completed transaction must either have accepted fresh validation, an exact
// rejected fresh-validation receipt consuming its handoff, one canonical
// target-free directive, or an atomic SM-2 failed terminal. A stale, renamed,
// target-restored, or lineage-free nonterminal projection fails closed.
func validateConfirmedRepairFreshValidationContinuation(aggregate RuntimeAggregate) error {
	transactions, err := completedConfirmedRepairTransactions(aggregate)
	if err != nil {
		return err
	}
	facts := aggregate.Facts.Normalized()
	terminalFailureClosed := aggregate.Decision.State == runtimecontract.ConvergenceStateFailed &&
		aggregate.Decision.Terminal != nil && facts.Failure != nil
	matchedDirective := false
	outstanding := 0
	applicable := 0
	for _, completed := range transactions {
		transaction := completed.transaction
		if !sameCoverageIDs(transaction.closure.CoverageWorkItemIDs, facts.Focus.CoverageWorkItemIDs) ||
			!confirmedRepairSourceUsesCurrentBinding(facts, transaction.sourceOp.BindingRefs, transaction.closure.CoverageWorkItemIDs) {
			continue
		}
		applicable++
		expected := transaction.freshValidationDirective(facts, completed.finalEvidenceRevision)
		if repairDirectiveEvidenceSatisfied(facts, expected, aggregate.CoverageProofLineage) {
			if facts.RepairDirective != nil && runtimecontract.SameRuntimeRepairClosure(facts.RepairDirective.Closure, expected.Closure) {
				return fmt.Errorf("runtime aggregate settled confirmed owner repair retained a satisfied fresh-validation directive")
			}
			continue
		}
		if confirmedRepairFreshValidationRejected(aggregate, completed) {
			// A fresh rejected validation consumes this continuation just as a
			// successful validation does, but grants no credit. Its exact new
			// receipt may open a separate bounded confirmation sequence.
			if facts.RepairDirective != nil && runtimecontract.SameRuntimeRepairClosure(facts.RepairDirective.Closure, expected.Closure) {
				return fmt.Errorf("runtime aggregate rejected fresh validation retained its consumed repair continuation")
			}
			continue
		}
		if terminalFailureClosed {
			if facts.RepairDirective != nil {
				return fmt.Errorf("runtime aggregate failed terminal retained confirmed owner repair fresh-validation authority")
			}
			continue
		}
		outstanding++
		if facts.RepairDirective == nil || !runtimecontract.SameRuntimeRepairDirective(facts.RepairDirective, &expected) {
			return fmt.Errorf("runtime aggregate confirmed owner repair fresh-validation continuation does not exactly match its completed transaction")
		}
		matchedDirective = true
	}
	if outstanding > 1 {
		return fmt.Errorf("runtime aggregate has multiple outstanding confirmed owner repair fresh-validation continuations")
	}
	if facts.RepairDirective != nil && strings.HasPrefix(facts.RepairDirective.ID, "confirmed-repair/") && !matchedDirective {
		return fmt.Errorf("runtime aggregate confirmed owner repair fresh-validation directive has no exact completed transaction (completed=%d applicable=%d)", len(transactions), applicable)
	}
	return nil
}

// History proves consumption of a specific fresh-validation handoff, not new
// repair authority. The new failure still needs its own route, budget, owner
// confirmation and current resource facts before another effect is admissible.
func confirmedRepairFreshValidationRejected(aggregate RuntimeAggregate, completed completedConfirmedRepairTransaction) bool {
	for _, dispatch := range aggregate.CompletedDispatches {
		if dispatch.State != DispatchStateCommitted || dispatch.Candidate.Validation == nil || dispatch.RepairCommitment != nil {
			continue
		}
		matchingManifests := 0
		for _, session := range aggregate.CandidateRequestSessions {
			manifest := session.CandidateManifest
			if manifest != nil && session.Transaction == dispatch.Transaction && manifest.Hash == dispatch.CandidateManifestHash &&
				manifest.RepairClosureID == completed.transaction.closure.ID && manifest.RepairPurpose == "" {
				matchingManifests++
			}
		}
		if matchingManifests != 1 {
			continue
		}
		for _, fact := range aggregate.Facts.Evidence.Rejected {
			if fact.DispatchID != dispatch.ID || fact.Kind != runtimecontract.EvidenceKindValidation ||
				!runtimecontract.EvidenceRevisionAfter(fact.Revision, completed.finalEvidenceRevision) || !containsString(aggregate.ProcessedEventIDs, string(fact.ID)) ||
				!sameRuntimeStringSet(fact.BindingRefs, completed.transaction.sourceOp.BindingRefs) {
				continue
			}
			execution, found := operationExecutionRecord(dispatch.OperationExecutions, fact.OperationID)
			if !found || execution.State != runtimecontract.OperationExecutionEvidenceCommitted || !containsString(dispatch.CompletedOperationIDs, fact.OperationID) {
				continue
			}
			receipt, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(aggregate.OperationReceipts, execution.ExecutionKey, execution.Fence)
			if found && receipt.Outcome == runtimecontract.RuntimeOperationOutcomeRejected && receipt.Feedback != nil &&
				receipt.Feedback.DispatchID == dispatch.ID && receipt.OperationID == fact.OperationID &&
				execution.OwnerReceiptHash == runtimecontract.StableRuntimeOperationReceiptHash(receipt) {
				return true
			}
		}
	}
	return false
}

func settleConfirmedRepairDefinitionRepair(
	aggregate *RuntimeAggregate,
	fact runtimecontract.EvidenceFact,
	transaction confirmedRepairTransaction,
) (bool, error) {
	if aggregate == nil || aggregate.ActiveDispatch != nil || fact.Kind != runtimecontract.EvidenceKindStateChange || weakRuntimeEvidence(fact) {
		return false, common.ErrInvalidArgument
	}
	facts := aggregate.Facts.Normalized()
	if !sameCoverageIDs(transaction.closure.CoverageWorkItemIDs, facts.Focus.CoverageWorkItemIDs) ||
		!confirmedRepairSourceUsesCurrentBinding(facts, transaction.sourceOp.BindingRefs, transaction.closure.CoverageWorkItemIDs) {
		return false, fmt.Errorf("%w: confirmed owner Definition repair no longer has its exact source binding", common.ErrConflict)
	}
	directive := transaction.freshValidationDirective(facts, fact.Revision)
	if err := directive.Validate(); err != nil {
		return false, err
	}
	facts.Resources.FailureTarget = nil
	facts.RepairDirective = &directive
	aggregate.Facts = facts.Normalized()
	return true, nil
}

func confirmedRepairSourceUsesCurrentBinding(facts runtimecontract.RuntimeFacts, bindingRefs []string, coverageIDs []common.CoverageWorkItemID) bool {
	return confirmedRepairSourceBindingsCoverCurrentScope(RuntimeAggregate{Facts: facts}, bindingRefs, coverageIDs)
}
