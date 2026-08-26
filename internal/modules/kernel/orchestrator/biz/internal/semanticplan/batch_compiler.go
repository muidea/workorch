package semanticplan

import (
	"fmt"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// CompileBatch creates the next bounded, owner-private SM-3 batch from an
// already accepted provider-authored order. Initial Pending state is selected
// here, inside the Orchestrator owner, and can only be persisted by its private
// internal-event reducer.
func CompileBatch(manifest runtimecontract.RuntimeSemanticPlanManifest, orderDocument runtimecontract.RuntimeSemanticPlanDocument, sequence int, logicalRequestID string, maxItems int) (runtimecontract.RuntimeSemanticPlanBatch, error) {
	return CompileBatchFromOffset(manifest, orderDocument, sequence, sequence*maxItems, logicalRequestID, maxItems)
}

// CompileBatchFromOffset creates an owner-private batch from an explicit
// provider-order offset. The offset is persisted/recomputed from completed
// batches so a changed batch-size policy cannot replay an earlier slice.
func CompileBatchFromOffset(manifest runtimecontract.RuntimeSemanticPlanManifest, orderDocument runtimecontract.RuntimeSemanticPlanDocument, sequence, start int, logicalRequestID string, maxItems int) (runtimecontract.RuntimeSemanticPlanBatch, error) {
	manifest = manifest.Normalized()
	orderDocument = orderDocument.Normalized()
	logicalRequestID = strings.TrimSpace(logicalRequestID)
	if err := manifest.Validate(); err != nil {
		return runtimecontract.RuntimeSemanticPlanBatch{}, err
	}
	if orderDocument.PlanStep != runtimecontract.RuntimeSemanticPlanStepCoverageOrder || orderDocument.ManifestHash != manifest.Hash || orderDocument.DefinitionSeedHash != manifest.DefinitionSeedHash {
		return runtimecontract.RuntimeSemanticPlanBatch{}, fmt.Errorf("runtime semantic batch requires matching coverage-order document")
	}
	if err := orderDocument.ValidateAgainstManifest(manifest); err != nil {
		return runtimecontract.RuntimeSemanticPlanBatch{}, err
	}
	if sequence < 0 || logicalRequestID == "" || maxItems <= 0 {
		return runtimecontract.RuntimeSemanticPlanBatch{}, common.ErrInvalidArgument
	}
	if start < 0 || start > len(orderDocument.CoverageOrder) {
		return runtimecontract.RuntimeSemanticPlanBatch{}, fmt.Errorf("runtime semantic batch sequence is outside provider order")
	}
	if maxItems > len(orderDocument.CoverageOrder)-start {
		maxItems = len(orderDocument.CoverageOrder) - start
	}
	selected := append([]common.CoverageWorkItemID(nil), orderDocument.CoverageOrder[start:start+maxItems]...)
	selectedSet := make(map[common.CoverageWorkItemID]struct{}, len(selected))
	for _, id := range selected {
		selectedSet[id] = struct{}{}
	}
	pending := append([]common.CoverageWorkItemID(nil), orderDocument.CoverageOrder[start+maxItems:]...)
	items := semanticBatchItems(manifest)
	batch := runtimecontract.RuntimeSemanticPlanBatch{
		BatchID:                    semanticBatchID(sequence, orderDocument),
		ManifestHash:               manifest.Hash,
		DefinitionSeedHash:         manifest.DefinitionSeedHash,
		CoverageOrderHash:          runtimecontract.StableRuntimeSemanticPlanCoverageOrderHash(orderDocument),
		Sequence:                   sequence,
		CoverageWorkItemIDs:        selected,
		PendingCoverageWorkItemIDs: pending,
		RequestLogicalID:           logicalRequestID,
		DependencyBarrierRefs:      semanticPendingDependencyBarriers(items, pending, selectedSet),
		State:                      runtimecontract.RuntimeSemanticPlanBatchPending,
	}
	batch.Hash = runtimecontract.StableRuntimeSemanticPlanBatchHash(batch)
	return batch, batch.ValidateAgainstManifest(manifest)
}

// CompileFocusBatch derives one owner-private proof batch from the accepted
// global order, scoped to the active focus and its transitive prerequisites.
// It never chooses a focus or invents an order; both are persisted facts.
func CompileFocusBatch(
	manifest runtimecontract.RuntimeSemanticPlanManifest,
	orderDocument runtimecontract.RuntimeSemanticPlanDocument,
	sequence int,
	logicalRequestID string,
	activeCoverageIDs []common.CoverageWorkItemID,
	pendingCoverageIDs []common.CoverageWorkItemID,
	maxItems int,
) (runtimecontract.RuntimeSemanticPlanBatch, error) {
	rawActiveCoverageIDs := append([]common.CoverageWorkItemID(nil), activeCoverageIDs...)
	rawPendingCoverageIDs := append([]common.CoverageWorkItemID(nil), pendingCoverageIDs...)
	manifest = manifest.Normalized()
	orderDocument = orderDocument.Normalized()
	logicalRequestID = strings.TrimSpace(logicalRequestID)
	activeCoverageIDs = normalizeOrderedCoverageIDs(activeCoverageIDs)
	pendingCoverageIDs = normalizeOrderedCoverageIDs(pendingCoverageIDs)
	if len(rawActiveCoverageIDs) != len(activeCoverageIDs) || len(rawPendingCoverageIDs) != len(pendingCoverageIDs) {
		return runtimecontract.RuntimeSemanticPlanBatch{}, fmt.Errorf("runtime semantic focus batch contains duplicate or empty coverage identity")
	}
	if err := manifest.Validate(); err != nil {
		return runtimecontract.RuntimeSemanticPlanBatch{}, err
	}
	if orderDocument.PlanStep != runtimecontract.RuntimeSemanticPlanStepCoverageOrder || orderDocument.ManifestHash != manifest.Hash || orderDocument.DefinitionSeedHash != manifest.DefinitionSeedHash {
		return runtimecontract.RuntimeSemanticPlanBatch{}, fmt.Errorf("runtime semantic focus batch requires matching coverage-order document")
	}
	if err := orderDocument.ValidateAgainstManifest(manifest); err != nil {
		return runtimecontract.RuntimeSemanticPlanBatch{}, err
	}
	if sequence < 0 || logicalRequestID == "" || maxItems <= 0 || len(activeCoverageIDs) == 0 {
		return runtimecontract.RuntimeSemanticPlanBatch{}, common.ErrInvalidArgument
	}
	if len(pendingCoverageIDs) == 0 {
		pendingCoverageIDs = append([]common.CoverageWorkItemID(nil), orderDocument.CoverageOrder...)
	}
	orderIndex := make(map[common.CoverageWorkItemID]int, len(orderDocument.CoverageOrder))
	for idx, id := range orderDocument.CoverageOrder {
		orderIndex[id] = idx
	}
	pendingSet := make(map[common.CoverageWorkItemID]struct{}, len(pendingCoverageIDs))
	for _, id := range pendingCoverageIDs {
		if _, known := orderIndex[id]; !known {
			return runtimecontract.RuntimeSemanticPlanBatch{}, fmt.Errorf("runtime semantic pending coverage %q is outside provider order", id)
		}
		pendingSet[id] = struct{}{}
	}
	items := semanticBatchItems(manifest)
	selectedSet := make(map[common.CoverageWorkItemID]struct{}, len(activeCoverageIDs))
	var include func(common.CoverageWorkItemID) error
	include = func(id common.CoverageWorkItemID) error {
		if _, selected := selectedSet[id]; selected {
			return nil
		}
		if _, pending := pendingSet[id]; !pending {
			return nil
		}
		item, known := items[id]
		if !known {
			return fmt.Errorf("runtime semantic active coverage %q is outside manifest", id)
		}
		for _, prerequisiteID := range item.PrerequisiteCoverageWorkItemIDs {
			if err := include(prerequisiteID); err != nil {
				return err
			}
		}
		selectedSet[id] = struct{}{}
		return nil
	}
	for _, id := range activeCoverageIDs {
		if _, pending := pendingSet[id]; !pending {
			return runtimecontract.RuntimeSemanticPlanBatch{}, fmt.Errorf("runtime semantic active coverage %q is not pending", id)
		}
		if err := include(id); err != nil {
			return runtimecontract.RuntimeSemanticPlanBatch{}, err
		}
	}
	selectedIDs := make([]common.CoverageWorkItemID, 0, len(selectedSet))
	pending := make([]common.CoverageWorkItemID, 0, len(pendingSet)-len(selectedSet))
	for _, id := range orderDocument.CoverageOrder {
		if _, isPending := pendingSet[id]; !isPending {
			continue
		}
		if _, isSelected := selectedSet[id]; isSelected {
			selectedIDs = append(selectedIDs, id)
			continue
		}
		pending = append(pending, id)
	}
	if len(selectedIDs) == 0 || len(selectedIDs) > maxItems {
		return runtimecontract.RuntimeSemanticPlanBatch{}, fmt.Errorf("runtime semantic current focus dependency closure has %d items, budget is %d", len(selectedIDs), maxItems)
	}
	batch := runtimecontract.RuntimeSemanticPlanBatch{
		BatchID:                    semanticBatchID(sequence, orderDocument),
		ManifestHash:               manifest.Hash,
		DefinitionSeedHash:         manifest.DefinitionSeedHash,
		CoverageOrderHash:          runtimecontract.StableRuntimeSemanticPlanCoverageOrderHash(orderDocument),
		Sequence:                   sequence,
		CoverageWorkItemIDs:        selectedIDs,
		PendingCoverageWorkItemIDs: pending,
		RequestLogicalID:           logicalRequestID,
		DependencyBarrierRefs:      semanticPendingDependencyBarriers(items, pending, selectedSet),
		State:                      runtimecontract.RuntimeSemanticPlanBatchPending,
	}
	batch.Hash = runtimecontract.StableRuntimeSemanticPlanBatchHash(batch)
	return batch, batch.ValidateAgainstManifest(manifest)
}

func semanticBatchID(sequence int, orderDocument runtimecontract.RuntimeSemanticPlanDocument) string {
	const prefix = "runtime-semantic-plan-order-"
	return fmt.Sprintf("semantic-batch-%d-%s", sequence, runtimecontract.StableRuntimeSemanticPlanCoverageOrderHash(orderDocument)[len(prefix):])
}

func semanticBatchItems(manifest runtimecontract.RuntimeSemanticPlanManifest) map[common.CoverageWorkItemID]runtimecontract.RuntimeCoverageItem {
	items := make(map[common.CoverageWorkItemID]runtimecontract.RuntimeCoverageItem, len(manifest.CoverageItems))
	for _, item := range manifest.CoverageItems {
		items[item.ID] = item
	}
	return items
}

func semanticPendingDependencyBarriers(items map[common.CoverageWorkItemID]runtimecontract.RuntimeCoverageItem, pending []common.CoverageWorkItemID, selected map[common.CoverageWorkItemID]struct{}) []string {
	barriers := make([]string, 0)
	for _, id := range pending {
		for _, prerequisiteID := range items[id].PrerequisiteCoverageWorkItemIDs {
			if _, inBatch := selected[prerequisiteID]; !inBatch {
				barriers = append(barriers, string(prerequisiteID)+"->"+string(id))
			}
		}
	}
	return normalizeSemanticBatchStrings(barriers)
}

func normalizeOrderedCoverageIDs(values []common.CoverageWorkItemID) []common.CoverageWorkItemID {
	ret := make([]common.CoverageWorkItemID, 0, len(values))
	seen := make(map[common.CoverageWorkItemID]struct{}, len(values))
	for _, value := range values {
		value = common.CoverageWorkItemID(strings.TrimSpace(string(value)))
		if value == "" {
			continue
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		ret = append(ret, value)
	}
	return ret
}

func normalizeSemanticBatchStrings(values []string) []string {
	ret := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		ret = append(ret, value)
	}
	sort.Strings(ret)
	if len(ret) == 0 {
		return nil
	}
	return ret
}
