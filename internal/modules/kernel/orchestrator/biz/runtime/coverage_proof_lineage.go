package runtime

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/proof"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// appendAcceptedCoverageProofLineage records the immutable source side of an
// accepted, exact receipt proof. It runs inside the existing evidence CAS;
// capability owners never receive or interpret this SM-2-only ledger.
func appendAcceptedCoverageProofLineage(aggregate *RuntimeAggregate, fact runtimecontract.EvidenceFact, receipt runtimecontract.RuntimeOperationReceipt) error {
	if aggregate == nil {
		return common.ErrInvalidArgument
	}
	receipt = receipt.Normalized()
	if fact.ID == "" || fact.Kind == "" || len(fact.BindingRefs) != 1 || len(fact.CoverageWorkItemIDs) != 1 ||
		fact.ZeroWork || weakRuntimeEvidence(fact) || receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || receipt.Evidence.ZeroWork {
		return nil
	}
	if fact.Kind != runtimecontract.EvidenceKindStateChange && fact.Kind != runtimecontract.EvidenceKindValidation {
		return nil
	}
	binding, found := runtimecontract.CoverageEvidenceBindingForRef(aggregate.Facts.Focus.CoverageEvidenceBindings, fact.BindingRefs[0])
	if !found || binding.CoverageWorkItemID != fact.CoverageWorkItemIDs[0] {
		return fmt.Errorf("accepted coverage receipt has no exact current binding")
	}
	manifestHash, logicalRequestID, found := semanticAuditLineageForCurrentBinding(*aggregate, fact.CoverageWorkItemIDs[0])
	if !found {
		return fmt.Errorf("accepted coverage receipt has no sealed semantic audit lineage")
	}
	identity, err := runtimecontract.DeriveCoverageEvidenceProofIdentity(binding, fact.Kind)
	if err != nil {
		// An exact receipt may still earn direct current-binding credit for a
		// non-exact broad binding, but ambiguous proof ownership can never be
		// transferred across a later re-seal. Do not create a source row that
		// would turn an ambiguous proof into lineage authority.
		return nil
	}
	execution, found := operationExecutionRecordForDispatch(aggregate, fact.DispatchID, fact.OperationID)
	if !found || execution.ExecutionKey != receipt.ExecutionKey || execution.Fence != receipt.Fence ||
		execution.State != runtimecontract.OperationExecutionEvidenceCommitted {
		return fmt.Errorf("accepted coverage receipt has no exact committed execution fence")
	}
	receiptHash := runtimecontract.StableRuntimeOperationReceiptHash(receipt)
	if receiptHash == "" || execution.OwnerReceiptHash != "" && execution.OwnerReceiptHash != receiptHash {
		return fmt.Errorf("accepted coverage receipt hash does not match committed execution")
	}
	row := runtimecontract.RuntimeCoverageProofLineage{
		Kind:                   runtimecontract.RuntimeCoverageProofLineageKindSource,
		WorkspaceID:            aggregate.Facts.WorkspaceID,
		RunID:                  aggregate.Facts.RunID,
		CoverageWorkItemID:     fact.CoverageWorkItemIDs[0],
		EvidenceKind:           fact.Kind,
		SourceEvidenceID:       fact.ID,
		SourceDispatchID:       fact.DispatchID,
		SourceOperationID:      fact.OperationID,
		SourceExecutionKey:     receipt.ExecutionKey,
		SourceFence:            receipt.Fence,
		SourceReceiptHash:      receiptHash,
		SourceBindingRef:       binding.ID,
		SourceBindingHash:      binding.Hash,
		SourceManifestHash:     manifestHash,
		SourceLogicalRequestID: logicalRequestID,
		SourceProofIdentity:    identity,
	}
	row.ID = runtimecontract.StableRuntimeCoverageProofLineageSourceID(
		row.SourceReceiptHash,
		row.SourceEvidenceID,
		row.SourceBindingHash,
		row.SourceProofIdentity.Hash,
	)
	for _, existing := range aggregate.CoverageProofLineage {
		if existing.ID == row.ID {
			return nil
		}
	}
	if len(aggregate.CoverageProofLineage) >= runtimecontract.RuntimeCoverageProofLineageLimit {
		return fmt.Errorf("coverage proof lineage exceeds bounded limit %d", runtimecontract.RuntimeCoverageProofLineageLimit)
	}
	if err := row.Validate(); err != nil {
		return err
	}
	aggregate.CoverageProofLineage = append(aggregate.CoverageProofLineage, row)
	aggregate.CoverageProofLineage = runtimecontract.NormalizeRuntimeCoverageProofLineage(aggregate.CoverageProofLineage)
	return nil
}

func semanticAuditLineageForCurrentBinding(aggregate RuntimeAggregate, coverageID common.CoverageWorkItemID) (string, string, bool) {
	item, found := runtimeCoverageByID(aggregate.Facts.Coverage)[coverageID]
	if !found || item.CurrentBindingRef == nil {
		return "", "", false
	}
	manifestHash := item.CurrentBindingRef.SemanticManifestHash
	active := aggregate.SemanticPlanning
	if active.Phase == runtimecontract.RuntimeSemanticPlanningSealed && active.Manifest != nil &&
		active.Manifest.Hash == manifestHash && strings.TrimSpace(active.LogicalRequestID) != "" {
		return manifestHash, strings.TrimSpace(active.LogicalRequestID), true
	}
	for _, generation := range active.AllHistory() {
		snapshot := generation.Snapshot
		if snapshot.Phase != runtimecontract.RuntimeSemanticPlanningSealed || snapshot.Manifest == nil ||
			snapshot.Manifest.Hash != manifestHash || strings.TrimSpace(snapshot.LogicalRequestID) == "" {
			continue
		}
		return manifestHash, strings.TrimSpace(snapshot.LogicalRequestID), true
	}
	return "", "", false
}

// resolveCoverageProofLineageForFocus records the current-binding side of
// source proof lineage after SM-2 has re-sealed a focus. It never changes an
// old EvidenceFact, receipt, dispatch, or binding ref.
func resolveCoverageProofLineageForFocus(aggregate *RuntimeAggregate, focus runtimecontract.CoverageFocus) error {
	if aggregate == nil {
		return common.ErrInvalidArgument
	}
	if aggregate.ActiveDispatch != nil {
		return fmt.Errorf("coverage proof lineage cannot resolve while a dispatch is active")
	}
	focus = focus.Normalized()
	for _, binding := range focus.CoverageEvidenceBindings {
		binding = binding.Normalized()
		for _, proof := range binding.Proofs {
			identity, err := runtimecontract.DeriveCoverageEvidenceProofIdentity(binding, proof.EvidenceKind)
			if err != nil {
				return err
			}
			for _, source := range aggregate.CoverageProofLineage {
				source = source.Normalized()
				if source.Kind != runtimecontract.RuntimeCoverageProofLineageKindSource ||
					source.WorkspaceID != aggregate.Facts.WorkspaceID || source.RunID != aggregate.Facts.RunID ||
					source.CoverageWorkItemID != binding.CoverageWorkItemID || source.EvidenceKind != proof.EvidenceKind {
					continue
				}
				// Exact binding identity keeps the direct receipt path; only an
				// actual re-seal needs an additional resolution record.
				if source.SourceBindingRef == binding.ID && source.SourceBindingHash == binding.Hash {
					continue
				}
				row := runtimecontract.RuntimeCoverageProofLineage{
					Kind:                 runtimecontract.RuntimeCoverageProofLineageKindResolution,
					WorkspaceID:          aggregate.Facts.WorkspaceID,
					RunID:                aggregate.Facts.RunID,
					CoverageWorkItemID:   binding.CoverageWorkItemID,
					EvidenceKind:         proof.EvidenceKind,
					SourceLineageID:      source.ID,
					CurrentBindingRef:    binding.ID,
					CurrentBindingHash:   binding.Hash,
					CurrentProofIdentity: identity,
				}
				if runtimecontract.SameCoverageEvidenceProofIdentity(source.SourceProofIdentity, identity) {
					row.Disposition = runtimecontract.RuntimeCoverageProofLineageDispositionEquivalent
				} else {
					row.Disposition = runtimecontract.RuntimeCoverageProofLineageDispositionIncompatible
					gap := runtimecontract.RuntimeCoverageProofLineageGap{
						Code:                        runtimecontract.RuntimeCoverageProofLineageIncompatibleGapCode,
						CoverageWorkItemID:          binding.CoverageWorkItemID,
						EvidenceKind:                proof.EvidenceKind,
						SourceProofIdentityHash:     source.SourceProofIdentity.Hash,
						CurrentProofIdentityHash:    identity.Hash,
						FirstDifferingFieldCategory: runtimecontract.FirstCoverageEvidenceProofIdentityDifference(source.SourceProofIdentity, identity),
					}
					row.Gap = &gap
				}
				row.ID = runtimecontract.StableRuntimeCoverageProofLineageResolutionID(row.SourceLineageID, row.CurrentBindingRef, row.CurrentBindingHash)
				if runtimeCoverageProofLineageRowExists(aggregate.CoverageProofLineage, row.ID) {
					continue
				}
				if len(aggregate.CoverageProofLineage) >= runtimecontract.RuntimeCoverageProofLineageLimit {
					return fmt.Errorf("coverage proof lineage exceeds bounded limit %d", runtimecontract.RuntimeCoverageProofLineageLimit)
				}
				if err := row.Validate(); err != nil {
					return err
				}
				aggregate.CoverageProofLineage = append(aggregate.CoverageProofLineage, row)
			}
		}
	}
	aggregate.CoverageProofLineage = runtimecontract.NormalizeRuntimeCoverageProofLineage(aggregate.CoverageProofLineage)
	return nil
}

func runtimeCoverageProofLineageRowExists(values []runtimecontract.RuntimeCoverageProofLineage, id string) bool {
	for _, value := range values {
		if value.ID == id {
			return true
		}
	}
	return false
}

func operationExecutionRecordForDispatch(aggregate *RuntimeAggregate, dispatchID, operationID string) (runtimecontract.RuntimeOperationExecutionRecord, bool) {
	if aggregate == nil {
		return runtimecontract.RuntimeOperationExecutionRecord{}, false
	}
	if aggregate.ActiveDispatch != nil && aggregate.ActiveDispatch.ID == dispatchID {
		return operationExecutionRecord(aggregate.ActiveDispatch.OperationExecutions, operationID)
	}
	for _, dispatch := range aggregate.CompletedDispatches {
		if dispatch.ID == dispatchID {
			return operationExecutionRecord(dispatch.OperationExecutions, operationID)
		}
	}
	return runtimecontract.RuntimeOperationExecutionRecord{}, false
}

// runtimeEvidenceAppliesToCoverageWorkItemWithLineage leaves the persisted
// evidence unchanged and recognizes old binding evidence only after the
// matching equivalent current-binding resolution is persisted.
func runtimeEvidenceAppliesToCoverageWorkItemWithLineage(
	facts runtimecontract.RuntimeFacts,
	evidence runtimecontract.EvidenceFact,
	coverageID common.CoverageWorkItemID,
	lineage []runtimecontract.RuntimeCoverageProofLineage,
) bool {
	return proof.EvidenceAppliesToCoverageWorkItemWithLineage(facts, evidence, coverageID, lineage)
}

// runtimeEvidenceAppliesToCurrentCoverageWithLineage is the SM-2 attribution
// gate for an accepted evidence fact that may affect current CWI coverage or a
// later action derived from that coverage. It intentionally differs from the
// shared broad focus predicate: a source-binding rollover is usable only after
// this aggregate has persisted the matching equivalent resolution.
func runtimeEvidenceAppliesToCurrentCoverageWithLineage(
	facts runtimecontract.RuntimeFacts,
	evidence runtimecontract.EvidenceFact,
	lineage []runtimecontract.RuntimeCoverageProofLineage,
) bool {
	return proof.EvidenceAppliesToCurrentCoverageWithLineage(facts, evidence, lineage)
}

// runtimeEvidenceAppliesToCoverageIDsWithLineage narrows the same predicate to
// a sealed action's explicit CWI set. CWI credit is unavailable unless the
// current Runtime ledger and exact current binding are both present.
func runtimeEvidenceAppliesToCoverageIDsWithLineage(
	facts runtimecontract.RuntimeFacts,
	evidence runtimecontract.EvidenceFact,
	coverageIDs []common.CoverageWorkItemID,
	lineage []runtimecontract.RuntimeCoverageProofLineage,
) bool {
	return proof.EvidenceAppliesToCoverageIDsWithLineage(facts, evidence, coverageIDs, lineage)
}

// runtimeAcceptedEvidenceCreditsCurrentFocusWithLineage is the common
// binding-aware credit gate for accepted ordinary evidence. Diagnostic facts
// with no CWI projection deliberately return false.
func runtimeAcceptedEvidenceCreditsCurrentFocusWithLineage(
	facts runtimecontract.RuntimeFacts,
	evidence runtimecontract.EvidenceFact,
	lineage []runtimecontract.RuntimeCoverageProofLineage,
) bool {
	return proof.AcceptedEvidenceCreditsCurrentFocusWithLineage(facts, evidence, lineage)
}

func runtimeCoverageProofLineageArgument(values [][]runtimecontract.RuntimeCoverageProofLineage) []runtimecontract.RuntimeCoverageProofLineage {
	if len(values) == 0 {
		return nil
	}
	return runtimecontract.NormalizeRuntimeCoverageProofLineage(values[0])
}
