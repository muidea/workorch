package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeSemanticPlanBatchState is the Orchestrator-owned lifecycle of one
// bounded semantic-plan request. LLM never emits or changes this state.
type RuntimeSemanticPlanBatchState string

const (
	RuntimeSemanticPlanBatchPending             RuntimeSemanticPlanBatchState = "pending"
	RuntimeSemanticPlanBatchRequesting          RuntimeSemanticPlanBatchState = "requesting"
	RuntimeSemanticPlanBatchDocumentComplete    RuntimeSemanticPlanBatchState = "document_complete"
	RuntimeSemanticPlanBatchFeasibilityPending  RuntimeSemanticPlanBatchState = "feasibility_pending"
	RuntimeSemanticPlanBatchFeasibilityComplete RuntimeSemanticPlanBatchState = "feasibility_complete"
	RuntimeSemanticPlanBatchAccepted            RuntimeSemanticPlanBatchState = "accepted"
	RuntimeSemanticPlanBatchFailed              RuntimeSemanticPlanBatchState = "failed"
)

type RuntimeSemanticPlanBatch struct {
	BatchID                    string
	ManifestHash               string
	DefinitionSeedHash         string
	CoverageOrderHash          string
	Sequence                   int
	CoverageWorkItemIDs        []common.CoverageWorkItemID
	PendingCoverageWorkItemIDs []common.CoverageWorkItemID
	RequestLogicalID           string
	DocumentHash               string
	DependencyBarrierRefs      []string
	State                      RuntimeSemanticPlanBatchState
	Hash                       string
}

type RuntimeSemanticPlanBatchFeasibilityResult struct {
	BatchID       string
	BatchHash     string
	DocumentHash  string
	Feasibilities []RuntimeSemanticProofFeasibility
	Hash          string
}

func (r RuntimeSemanticPlanBatchFeasibilityResult) Normalized() RuntimeSemanticPlanBatchFeasibilityResult {
	r.BatchID = strings.TrimSpace(r.BatchID)
	r.BatchHash = strings.TrimSpace(r.BatchHash)
	r.DocumentHash = strings.TrimSpace(r.DocumentHash)
	r.Feasibilities = normalizeRuntimeSemanticFeasibilities(r.Feasibilities)
	r.Hash = strings.TrimSpace(r.Hash)
	return r
}

func (r RuntimeSemanticPlanBatchFeasibilityResult) Validate(batch RuntimeSemanticPlanBatch, document RuntimeSemanticPlanDocument, manifest RuntimeSemanticPlanManifest) error {
	r = r.Normalized()
	batch = batch.Normalized()
	document = document.Normalized()
	if r.BatchID != batch.BatchID || r.BatchHash != batch.Hash || r.DocumentHash != StableRuntimeSemanticPlanDocumentHash(document) || r.Hash != StableRuntimeSemanticPlanBatchFeasibilityResultHash(r) {
		return fmt.Errorf("runtime semantic batch feasibility result identity does not match batch")
	}
	if batch.State != RuntimeSemanticPlanBatchAccepted {
		return fmt.Errorf("runtime semantic batch feasibility result requires accepted batch")
	}
	return ValidateRuntimeSemanticPlanBatchFeasibilities(manifest, document, r.Feasibilities)
}

func StableRuntimeSemanticPlanBatchFeasibilityResultHash(result RuntimeSemanticPlanBatchFeasibilityResult) string {
	result = result.Normalized()
	result.Hash = ""
	h := sha256.New()
	writeHashString(h, "runtime-semantic-batch-feasibility")
	writeHashString(h, result.BatchID)
	writeHashString(h, result.BatchHash)
	writeHashString(h, result.DocumentHash)
	for _, value := range result.Feasibilities {
		writeHashString(h, StableRuntimeSemanticFeasibilitySetHash([]RuntimeSemanticProofFeasibility{value}))
	}
	return "runtime-semantic-batch-feasibility-" + hex.EncodeToString(h.Sum(nil))[:20]
}

func (b RuntimeSemanticPlanBatch) Normalized() RuntimeSemanticPlanBatch {
	b.BatchID = strings.TrimSpace(b.BatchID)
	b.ManifestHash = strings.TrimSpace(b.ManifestHash)
	b.DefinitionSeedHash = strings.TrimSpace(b.DefinitionSeedHash)
	b.CoverageOrderHash = strings.TrimSpace(b.CoverageOrderHash)
	b.RequestLogicalID = strings.TrimSpace(b.RequestLogicalID)
	b.DocumentHash = strings.TrimSpace(b.DocumentHash)
	b.DependencyBarrierRefs = normalizeStrings(b.DependencyBarrierRefs)
	b.CoverageWorkItemIDs = normalizeOrderedCoverageWorkItemIDs(b.CoverageWorkItemIDs)
	b.PendingCoverageWorkItemIDs = normalizeOrderedCoverageWorkItemIDs(b.PendingCoverageWorkItemIDs)
	b.State = RuntimeSemanticPlanBatchState(strings.TrimSpace(string(b.State)))
	b.Hash = strings.TrimSpace(b.Hash)
	return b
}

func (b RuntimeSemanticPlanBatch) Clone() RuntimeSemanticPlanBatch {
	return b.Normalized()
}

func (b RuntimeSemanticPlanBatch) ValidateAgainstManifest(manifest RuntimeSemanticPlanManifest) error {
	raw := b
	b = b.Normalized()
	manifest = manifest.Normalized()
	// Batch identity is persisted authority.  Do not silently repair duplicate
	// or empty ordered IDs while validating it: doing so would change the
	// selected/pending lineage and allow a forged private event to acquire a
	// different hash from the one that was submitted.
	if len(raw.CoverageWorkItemIDs) != len(b.CoverageWorkItemIDs) ||
		len(raw.PendingCoverageWorkItemIDs) != len(b.PendingCoverageWorkItemIDs) ||
		len(raw.DependencyBarrierRefs) != len(b.DependencyBarrierRefs) {
		return fmt.Errorf("runtime semantic plan batch contains duplicate or empty identity values")
	}
	if err := manifest.Validate(); err != nil {
		return err
	}
	if b.BatchID == "" || b.ManifestHash != manifest.Hash || b.DefinitionSeedHash != manifest.DefinitionSeedHash || b.CoverageOrderHash == "" || b.Sequence < 0 || b.RequestLogicalID == "" || len(b.CoverageWorkItemIDs) == 0 {
		return fmt.Errorf("runtime semantic plan batch requires identity, manifest, sequence, request and coverage")
	}
	if b.Hash != StableRuntimeSemanticPlanBatchHash(b) {
		return fmt.Errorf("runtime semantic plan batch hash does not match contents")
	}
	valid := make(map[common.CoverageWorkItemID]struct{}, len(manifest.CoverageItems))
	for _, item := range manifest.CoverageItems {
		valid[item.ID] = struct{}{}
	}
	seen := make(map[common.CoverageWorkItemID]struct{}, len(b.CoverageWorkItemIDs))
	for _, id := range b.CoverageWorkItemIDs {
		if _, ok := valid[id]; !ok {
			return fmt.Errorf("runtime semantic plan batch coverage %q is outside manifest", id)
		}
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("runtime semantic plan batch duplicates coverage %q", id)
		}
		seen[id] = struct{}{}
	}
	pendingSeen := make(map[common.CoverageWorkItemID]struct{}, len(b.PendingCoverageWorkItemIDs))
	for _, id := range b.PendingCoverageWorkItemIDs {
		if _, selected := seen[id]; selected {
			return fmt.Errorf("runtime semantic plan batch coverage %q is both selected and pending", id)
		}
		if _, ok := valid[id]; !ok {
			return fmt.Errorf("runtime semantic plan batch pending coverage %q is outside manifest", id)
		}
		if _, duplicate := pendingSeen[id]; duplicate {
			return fmt.Errorf("runtime semantic plan batch duplicates pending coverage %q", id)
		}
		pendingSeen[id] = struct{}{}
	}
	switch b.State {
	case RuntimeSemanticPlanBatchPending, RuntimeSemanticPlanBatchRequesting,
		RuntimeSemanticPlanBatchDocumentComplete, RuntimeSemanticPlanBatchFeasibilityPending,
		RuntimeSemanticPlanBatchFeasibilityComplete, RuntimeSemanticPlanBatchAccepted,
		RuntimeSemanticPlanBatchFailed:
	default:
		return fmt.Errorf("runtime semantic plan batch has invalid state %q", b.State)
	}
	if b.State != RuntimeSemanticPlanBatchPending && b.State != RuntimeSemanticPlanBatchRequesting && b.State != RuntimeSemanticPlanBatchFailed && b.DocumentHash == "" {
		return fmt.Errorf("runtime semantic plan batch completed document is missing hash")
	}
	return nil
}

// SameRuntimeSemanticPlanBatchImmutableIdentity compares the fields that
// identify a batch independently of its owner-local lifecycle state and hash.
// State/hash change as the private reducer advances the batch; all other
// fields are the sealed request identity and must remain byte-for-byte
// equivalent for duplicate/restart replay.
func SameRuntimeSemanticPlanBatchImmutableIdentity(left, right RuntimeSemanticPlanBatch) bool {
	left = left.Normalized()
	right = right.Normalized()
	if left.BatchID != right.BatchID || left.ManifestHash != right.ManifestHash ||
		left.DefinitionSeedHash != right.DefinitionSeedHash || left.CoverageOrderHash != right.CoverageOrderHash ||
		left.Sequence != right.Sequence || left.RequestLogicalID != right.RequestLogicalID ||
		left.DocumentHash != right.DocumentHash ||
		!sameOrderedCoverageWorkItemIDs(left.CoverageWorkItemIDs, right.CoverageWorkItemIDs) ||
		!sameOrderedCoverageWorkItemIDs(left.PendingCoverageWorkItemIDs, right.PendingCoverageWorkItemIDs) {
		return false
	}
	return sameOrderedSemanticBatchStrings(left.DependencyBarrierRefs, right.DependencyBarrierRefs)
}

// AdvanceRuntimeSemanticPlanBatch validates the bounded batch lifecycle. The
// helper is pure; persistence and event ownership remain in Orchestrator.
func AdvanceRuntimeSemanticPlanBatch(batch RuntimeSemanticPlanBatch, next RuntimeSemanticPlanBatchState) (RuntimeSemanticPlanBatch, error) {
	batch = batch.Normalized()
	allowed := map[RuntimeSemanticPlanBatchState]map[RuntimeSemanticPlanBatchState]bool{
		RuntimeSemanticPlanBatchPending:             {RuntimeSemanticPlanBatchRequesting: true, RuntimeSemanticPlanBatchFailed: true},
		RuntimeSemanticPlanBatchRequesting:          {RuntimeSemanticPlanBatchDocumentComplete: true, RuntimeSemanticPlanBatchFailed: true},
		RuntimeSemanticPlanBatchDocumentComplete:    {RuntimeSemanticPlanBatchFeasibilityPending: true, RuntimeSemanticPlanBatchFailed: true},
		RuntimeSemanticPlanBatchFeasibilityPending:  {RuntimeSemanticPlanBatchFeasibilityComplete: true, RuntimeSemanticPlanBatchFailed: true},
		RuntimeSemanticPlanBatchFeasibilityComplete: {RuntimeSemanticPlanBatchAccepted: true, RuntimeSemanticPlanBatchFailed: true},
	}
	if !allowed[batch.State][next] {
		return RuntimeSemanticPlanBatch{}, fmt.Errorf("runtime semantic plan batch transition %q -> %q is not allowed", batch.State, next)
	}
	batch.State = next
	batch.Hash = StableRuntimeSemanticPlanBatchHash(batch)
	return batch, nil
}

func StableRuntimeSemanticPlanBatchHash(batch RuntimeSemanticPlanBatch) string {
	batch = batch.Normalized()
	batch.Hash = ""
	h := sha256.New()
	writeHashString(h, "runtime-semantic-plan-batch")
	writeHashString(h, batch.BatchID)
	writeHashString(h, batch.ManifestHash)
	writeHashString(h, batch.DefinitionSeedHash)
	writeHashString(h, batch.CoverageOrderHash)
	writeHashString(h, fmt.Sprintf("%d", batch.Sequence))
	for _, id := range batch.CoverageWorkItemIDs {
		writeHashString(h, string(id))
	}
	for _, id := range batch.PendingCoverageWorkItemIDs {
		writeHashString(h, string(id))
	}
	writeHashString(h, batch.RequestLogicalID)
	writeHashString(h, batch.DocumentHash)
	for _, ref := range batch.DependencyBarrierRefs {
		writeHashString(h, ref)
	}
	writeHashString(h, string(batch.State))
	return "runtime-semantic-plan-batch-" + hex.EncodeToString(h.Sum(nil))[:20]
}

// CompileRuntimeSemanticPlanBatch creates the next bounded batch from an
// already accepted provider-authored order. It never sorts or invents CWI
// order; the order document is the sole input for sequencing.
func CompileRuntimeSemanticPlanBatch(manifest RuntimeSemanticPlanManifest, orderDocument RuntimeSemanticPlanDocument, sequence int, logicalRequestID string, maxItems int) (RuntimeSemanticPlanBatch, error) {
	return CompileRuntimeSemanticPlanBatchFromOffset(manifest, orderDocument, sequence, sequence*maxItems, logicalRequestID, maxItems)
}

// CompileRuntimeSemanticPlanBatchFromOffset creates a batch from an explicit
// provider-order offset. The offset is persisted/recomputed from completed
// batches by the owner, so a changed batch-size policy cannot replay an
// earlier slice during resume.
func CompileRuntimeSemanticPlanBatchFromOffset(manifest RuntimeSemanticPlanManifest, orderDocument RuntimeSemanticPlanDocument, sequence, start int, logicalRequestID string, maxItems int) (RuntimeSemanticPlanBatch, error) {
	manifest = manifest.Normalized()
	orderDocument = orderDocument.Normalized()
	logicalRequestID = strings.TrimSpace(logicalRequestID)
	if err := manifest.Validate(); err != nil {
		return RuntimeSemanticPlanBatch{}, err
	}
	if orderDocument.PlanStep != RuntimeSemanticPlanStepCoverageOrder || orderDocument.ManifestHash != manifest.Hash || orderDocument.DefinitionSeedHash != manifest.DefinitionSeedHash {
		return RuntimeSemanticPlanBatch{}, fmt.Errorf("runtime semantic batch requires matching coverage-order document")
	}
	if err := orderDocument.ValidateAgainstManifest(manifest); err != nil {
		return RuntimeSemanticPlanBatch{}, err
	}
	if sequence < 0 || logicalRequestID == "" || maxItems <= 0 {
		return RuntimeSemanticPlanBatch{}, common.ErrInvalidArgument
	}
	if start < 0 || start > len(orderDocument.CoverageOrder) {
		return RuntimeSemanticPlanBatch{}, fmt.Errorf("runtime semantic batch sequence is outside provider order")
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
	barriers := make([]string, 0)
	items := make(map[common.CoverageWorkItemID]RuntimeCoverageItem, len(manifest.CoverageItems))
	for _, item := range manifest.CoverageItems {
		items[item.ID] = item
	}
	for _, id := range pending {
		for _, dep := range items[id].ReopenDependencies {
			if _, inBatch := selectedSet[dep.CoverageWorkItemID]; !inBatch {
				barriers = append(barriers, string(dep.CoverageWorkItemID)+"->"+string(id))
			}
		}
	}
	batch := RuntimeSemanticPlanBatch{
		BatchID:      fmt.Sprintf("semantic-batch-%d-%s", sequence, StableRuntimeSemanticPlanCoverageOrderHash(orderDocument)[len("runtime-semantic-plan-order-"):]),
		ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		CoverageOrderHash: StableRuntimeSemanticPlanCoverageOrderHash(orderDocument), Sequence: sequence,
		CoverageWorkItemIDs: selected, PendingCoverageWorkItemIDs: pending, RequestLogicalID: logicalRequestID,
		DependencyBarrierRefs: normalizeStrings(barriers), State: RuntimeSemanticPlanBatchPending,
	}
	batch.Hash = StableRuntimeSemanticPlanBatchHash(batch)
	return batch, batch.ValidateAgainstManifest(manifest)
}

// CompileRuntimeSemanticPlanFocusBatch derives one proof batch from the
// already accepted global order, but scopes that batch to the active focus and
// its transitive declared dependencies.  It never chooses a focus or invents
// an order: both are persisted Orchestrator facts.  Later CWI IDs remain
// pending lineage and cannot make the current SM-2 admission infeasible.
func CompileRuntimeSemanticPlanFocusBatch(
	manifest RuntimeSemanticPlanManifest,
	orderDocument RuntimeSemanticPlanDocument,
	sequence int,
	logicalRequestID string,
	activeCoverageIDs []common.CoverageWorkItemID,
	pendingCoverageIDs []common.CoverageWorkItemID,
	maxItems int,
) (RuntimeSemanticPlanBatch, error) {
	rawActiveCoverageIDs := append([]common.CoverageWorkItemID(nil), activeCoverageIDs...)
	rawPendingCoverageIDs := append([]common.CoverageWorkItemID(nil), pendingCoverageIDs...)
	manifest = manifest.Normalized()
	orderDocument = orderDocument.Normalized()
	logicalRequestID = strings.TrimSpace(logicalRequestID)
	activeCoverageIDs = normalizeOrderedCoverageWorkItemIDs(activeCoverageIDs)
	pendingCoverageIDs = normalizeOrderedCoverageWorkItemIDs(pendingCoverageIDs)
	if len(rawActiveCoverageIDs) != len(activeCoverageIDs) || len(rawPendingCoverageIDs) != len(pendingCoverageIDs) {
		return RuntimeSemanticPlanBatch{}, fmt.Errorf("runtime semantic focus batch contains duplicate or empty coverage identity")
	}
	if err := manifest.Validate(); err != nil {
		return RuntimeSemanticPlanBatch{}, err
	}
	if orderDocument.PlanStep != RuntimeSemanticPlanStepCoverageOrder || orderDocument.ManifestHash != manifest.Hash || orderDocument.DefinitionSeedHash != manifest.DefinitionSeedHash {
		return RuntimeSemanticPlanBatch{}, fmt.Errorf("runtime semantic focus batch requires matching coverage-order document")
	}
	if err := orderDocument.ValidateAgainstManifest(manifest); err != nil {
		return RuntimeSemanticPlanBatch{}, err
	}
	if sequence < 0 || logicalRequestID == "" || maxItems <= 0 || len(activeCoverageIDs) == 0 {
		return RuntimeSemanticPlanBatch{}, common.ErrInvalidArgument
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
			return RuntimeSemanticPlanBatch{}, fmt.Errorf("runtime semantic pending coverage %q is outside provider order", id)
		}
		pendingSet[id] = struct{}{}
	}
	items := make(map[common.CoverageWorkItemID]RuntimeCoverageItem, len(manifest.CoverageItems))
	for _, item := range manifest.CoverageItems {
		items[item.ID] = item
	}
	selectedSet := make(map[common.CoverageWorkItemID]struct{}, len(activeCoverageIDs))
	var include func(common.CoverageWorkItemID) error
	include = func(id common.CoverageWorkItemID) error {
		if _, selected := selectedSet[id]; selected {
			return nil
		}
		if _, pending := pendingSet[id]; !pending {
			// A dependency outside pending lineage was already sealed/settled.
			return nil
		}
		item, known := items[id]
		if !known {
			return fmt.Errorf("runtime semantic active coverage %q is outside manifest", id)
		}
		for _, dependency := range item.ReopenDependencies {
			if err := include(dependency.CoverageWorkItemID); err != nil {
				return err
			}
		}
		selectedSet[id] = struct{}{}
		return nil
	}
	for _, id := range activeCoverageIDs {
		if _, pending := pendingSet[id]; !pending {
			return RuntimeSemanticPlanBatch{}, fmt.Errorf("runtime semantic active coverage %q is not pending", id)
		}
		if err := include(id); err != nil {
			return RuntimeSemanticPlanBatch{}, err
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
		return RuntimeSemanticPlanBatch{}, fmt.Errorf("runtime semantic current focus dependency closure has %d items, budget is %d", len(selectedIDs), maxItems)
	}
	barriers := runtimeSemanticPendingDependencyBarriers(items, pending, selectedSet)
	batch := RuntimeSemanticPlanBatch{
		BatchID:      fmt.Sprintf("semantic-batch-%d-%s", sequence, StableRuntimeSemanticPlanCoverageOrderHash(orderDocument)[len("runtime-semantic-plan-order-"):]),
		ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		CoverageOrderHash: StableRuntimeSemanticPlanCoverageOrderHash(orderDocument), Sequence: sequence,
		CoverageWorkItemIDs: selectedIDs, PendingCoverageWorkItemIDs: pending, RequestLogicalID: logicalRequestID,
		DependencyBarrierRefs: barriers, State: RuntimeSemanticPlanBatchPending,
	}
	batch.Hash = StableRuntimeSemanticPlanBatchHash(batch)
	return batch, batch.ValidateAgainstManifest(manifest)
}

func runtimeSemanticPendingDependencyBarriers(items map[common.CoverageWorkItemID]RuntimeCoverageItem, pending []common.CoverageWorkItemID, selected map[common.CoverageWorkItemID]struct{}) []string {
	barriers := make([]string, 0)
	for _, id := range pending {
		for _, dependency := range items[id].ReopenDependencies {
			if _, inBatch := selected[dependency.CoverageWorkItemID]; !inBatch {
				barriers = append(barriers, string(dependency.CoverageWorkItemID)+"->"+string(id))
			}
		}
	}
	return normalizeStrings(barriers)
}

// MergeRuntimeSemanticPlanFocusBatchDocument converts one accepted current
// focus batch into the proof document used by SM-2 binding seal. The global
// coverage order intentionally stays in the separate persisted order document.
//
// The returned document deliberately retains its batch identity. Clearing it
// would make a partial, current-focus proof indistinguishable from a legacy
// full-manifest document and would either fail validation or, worse, invite a
// later caller to treat it as global authority. BatchID, sequence and exact
// membership are therefore part of the sealed SM-2 lineage.
func MergeRuntimeSemanticPlanFocusBatchDocument(manifest RuntimeSemanticPlanManifest, batch RuntimeSemanticPlanBatch, document RuntimeSemanticPlanDocument) (RuntimeSemanticPlanDocument, error) {
	manifest = manifest.Normalized()
	batch = batch.Normalized()
	document = document.Normalized()
	if err := manifest.Validate(); err != nil {
		return RuntimeSemanticPlanDocument{}, err
	}
	if err := batch.ValidateAgainstManifest(manifest); err != nil {
		return RuntimeSemanticPlanDocument{}, err
	}
	if document.PlanStep != RuntimeSemanticPlanStepProofBatch || document.BatchID != batch.BatchID ||
		document.BatchSequence != batch.Sequence || document.ManifestHash != manifest.Hash ||
		document.DefinitionSeedHash != manifest.DefinitionSeedHash ||
		!sameOrderedCoverageWorkItemIDs(document.BatchCoverageWorkItemIDs, batch.CoverageWorkItemIDs) {
		return RuntimeSemanticPlanDocument{}, fmt.Errorf("runtime semantic current focus batch document identity does not match batch")
	}
	if err := document.ValidateAgainstManifest(manifest); err != nil {
		return RuntimeSemanticPlanDocument{}, err
	}
	merged := document.Clone()
	merged.CoverageOrder = nil
	// Keep the exact local membership and identity as Orchestrator-only
	// ordering lineage.
	merged.BatchID = batch.BatchID
	merged.BatchSequence = batch.Sequence
	merged.BatchCoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), batch.CoverageWorkItemIDs...)
	if err := merged.ValidateAgainstManifest(manifest); err != nil {
		return RuntimeSemanticPlanDocument{}, err
	}
	return merged, nil
}

func sameOrderedCoverageWorkItemIDs(left, right []common.CoverageWorkItemID) bool {
	left = normalizeOrderedCoverageWorkItemIDs(left)
	right = normalizeOrderedCoverageWorkItemIDs(right)
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}
	return true
}

// MergeRuntimeSemanticPlanBatchDocuments assembles the complete proof
// document after every ordered batch is complete. It is pure and grants no
// feasibility or coverage credit.
func MergeRuntimeSemanticPlanBatchDocuments(manifest RuntimeSemanticPlanManifest, orderDocument RuntimeSemanticPlanDocument, documents []RuntimeSemanticPlanDocument) (RuntimeSemanticPlanDocument, error) {
	manifest = manifest.Normalized()
	orderDocument = orderDocument.Normalized()
	if err := orderDocument.ValidateAgainstManifest(manifest); err != nil {
		return RuntimeSemanticPlanDocument{}, err
	}
	if orderDocument.PlanStep != RuntimeSemanticPlanStepCoverageOrder {
		return RuntimeSemanticPlanDocument{}, fmt.Errorf("runtime semantic merge requires coverage-order document")
	}
	documents = append([]RuntimeSemanticPlanDocument(nil), documents...)
	sort.SliceStable(documents, func(i, j int) bool { return documents[i].BatchSequence < documents[j].BatchSequence })
	seen := make(map[common.CoverageWorkItemID]struct{}, len(manifest.CoverageItems))
	proposals := make([]RuntimeSemanticProofProposal, 0, len(manifest.CoverageItems))
	for idx, document := range documents {
		document = document.Normalized()
		if document.PlanStep != RuntimeSemanticPlanStepProofBatch || document.BatchID == "" || document.BatchSequence != idx || document.ManifestHash != manifest.Hash || document.DefinitionSeedHash != manifest.DefinitionSeedHash {
			return RuntimeSemanticPlanDocument{}, fmt.Errorf("runtime semantic merge has invalid batch document identity at sequence %d", idx)
		}
		if err := document.ValidateAgainstManifest(manifest); err != nil {
			return RuntimeSemanticPlanDocument{}, err
		}
		for _, proposal := range document.Proposals {
			if _, duplicate := seen[proposal.CoverageWorkItemID]; duplicate {
				return RuntimeSemanticPlanDocument{}, fmt.Errorf("runtime semantic merge duplicates coverage %q", proposal.CoverageWorkItemID)
			}
			seen[proposal.CoverageWorkItemID] = struct{}{}
			proposals = append(proposals, proposal.Clone())
		}
	}
	if len(seen) != len(manifest.CoverageItems) {
		return RuntimeSemanticPlanDocument{}, fmt.Errorf("runtime semantic merge does not cover full manifest")
	}
	merged := RuntimeSemanticPlanDocument{
		SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		PlanStep: RuntimeSemanticPlanStepProofBatch, CoverageOrder: append([]common.CoverageWorkItemID(nil), orderDocument.CoverageOrder...), Proposals: proposals,
	}
	return merged, merged.ValidateAgainstManifest(manifest)
}

// ValidateRuntimeSemanticPlanBatchFeasibilities validates one bounded owner
// feasibility fan-out. It does not decide whether the facts are feasible; it
// only enforces one exact observation per proposal in this batch.
func ValidateRuntimeSemanticPlanBatchFeasibilities(manifest RuntimeSemanticPlanManifest, document RuntimeSemanticPlanDocument, values []RuntimeSemanticProofFeasibility) error {
	manifest = manifest.Normalized()
	document = document.Normalized()
	if document.PlanStep != RuntimeSemanticPlanStepProofBatch || document.BatchID == "" {
		return fmt.Errorf("runtime semantic feasibility requires a proof batch document")
	}
	if err := document.ValidateAgainstManifest(manifest); err != nil {
		return err
	}
	proposals := make(map[common.CoverageWorkItemID]RuntimeSemanticProofProposal, len(document.Proposals))
	for _, proposal := range document.Proposals {
		proposals[proposal.CoverageWorkItemID] = proposal
	}
	seen := make(map[common.CoverageWorkItemID]struct{}, len(values))
	for _, value := range values {
		proposal, ok := proposals[value.CoverageWorkItemID]
		if !ok {
			return fmt.Errorf("runtime semantic batch feasibility coverage %q is outside batch", value.CoverageWorkItemID)
		}
		if _, duplicate := seen[value.CoverageWorkItemID]; duplicate {
			return fmt.Errorf("runtime semantic batch feasibility duplicates coverage %q", value.CoverageWorkItemID)
		}
		if err := value.ValidateForProposal(manifest, proposal); err != nil {
			return err
		}
		seen[value.CoverageWorkItemID] = struct{}{}
	}
	if len(seen) != len(proposals) {
		return fmt.Errorf("runtime semantic batch feasibility does not cover every proposal")
	}
	return nil
}

// MergeRuntimeSemanticPlanBatchFeasibilityResults combines one result and its
// original proof document per accepted batch into the complete owner-fact set.
// Batch identity is checked before unioning facts; no result can expand the
// sealed proposal set.  The caller must provide the exact documents persisted
// for those batches.  A merged/global document is intentionally not accepted:
// filtering its proposals by batch membership would manufacture a document
// identity that was never sealed by SM-3.
func MergeRuntimeSemanticPlanBatchFeasibilityResults(manifest RuntimeSemanticPlanManifest, documents []RuntimeSemanticPlanDocument, batches []RuntimeSemanticPlanBatch, results []RuntimeSemanticPlanBatchFeasibilityResult) ([]RuntimeSemanticProofFeasibility, error) {
	manifest = manifest.Normalized()
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	if len(documents) == 0 || len(batches) == 0 || len(results) == 0 || len(documents) != len(batches) || len(batches) != len(results) {
		return nil, fmt.Errorf("runtime semantic feasibility merge requires one document, batch and result per accepted batch")
	}
	documentsByBatchID := make(map[string]RuntimeSemanticPlanDocument, len(documents))
	for _, rawDocument := range documents {
		if err := rawDocument.ValidateAgainstManifest(manifest); err != nil {
			return nil, err
		}
		document := rawDocument.Clone()
		if document.BatchID == "" {
			return nil, fmt.Errorf("runtime semantic feasibility merge has a document without batch identity")
		}
		if _, duplicate := documentsByBatchID[document.BatchID]; duplicate {
			return nil, fmt.Errorf("runtime semantic feasibility merge duplicates batch document %q", document.BatchID)
		}
		documentsByBatchID[document.BatchID] = document
	}
	byID := make(map[string]RuntimeSemanticPlanBatch, len(batches))
	for _, rawBatch := range batches {
		if err := rawBatch.ValidateAgainstManifest(manifest); err != nil {
			return nil, err
		}
		batch := rawBatch.Normalized()
		if batch.BatchID == "" {
			return nil, fmt.Errorf("runtime semantic feasibility merge has a batch without identity")
		}
		if _, duplicate := byID[batch.BatchID]; duplicate {
			return nil, fmt.Errorf("runtime semantic feasibility merge duplicates batch %q", batch.BatchID)
		}
		if batch.State != RuntimeSemanticPlanBatchAccepted {
			return nil, fmt.Errorf("runtime semantic feasibility merge references incomplete batch %q", batch.BatchID)
		}
		document, found := documentsByBatchID[batch.BatchID]
		if !found {
			return nil, fmt.Errorf("runtime semantic feasibility merge has no matching batch document %q", batch.BatchID)
		}
		if document.BatchSequence != batch.Sequence ||
			!sameOrderedCoverageWorkItemIDs(document.BatchCoverageWorkItemIDs, batch.CoverageWorkItemIDs) ||
			batch.DocumentHash != StableRuntimeSemanticPlanDocumentHash(document) {
			return nil, fmt.Errorf("runtime semantic feasibility merge batch document identity does not match batch %q", batch.BatchID)
		}
		byID[batch.BatchID] = batch
	}
	seenBatch := make(map[string]struct{}, len(results))
	seenCoverage := make(map[common.CoverageWorkItemID]struct{})
	merged := make([]RuntimeSemanticProofFeasibility, 0)
	for _, rawResult := range results {
		result := rawResult.Normalized()
		if len(rawResult.Feasibilities) != len(result.Feasibilities) {
			return nil, fmt.Errorf("runtime semantic feasibility result contains duplicate or empty owner facts")
		}
		batch, ok := byID[result.BatchID]
		if !ok {
			return nil, fmt.Errorf("runtime semantic feasibility result references unknown batch %q", result.BatchID)
		}
		if _, duplicate := seenBatch[result.BatchID]; duplicate {
			return nil, fmt.Errorf("runtime semantic feasibility results duplicate batch %q", result.BatchID)
		}
		seenBatch[result.BatchID] = struct{}{}
		if result.BatchHash != batch.Hash || result.Hash != StableRuntimeSemanticPlanBatchFeasibilityResultHash(result) {
			return nil, fmt.Errorf("runtime semantic feasibility result identity drift for batch %q", result.BatchID)
		}
		batchDocument := documentsByBatchID[result.BatchID]
		if err := result.Validate(batch, batchDocument, manifest); err != nil {
			return nil, err
		}
		for _, value := range result.Feasibilities {
			if _, duplicate := seenCoverage[value.CoverageWorkItemID]; duplicate {
				return nil, fmt.Errorf("runtime semantic feasibility duplicates coverage %q", value.CoverageWorkItemID)
			}
			seenCoverage[value.CoverageWorkItemID] = struct{}{}
			merged = append(merged, value.Clone())
		}
	}
	if len(seenBatch) != len(byID) {
		return nil, fmt.Errorf("runtime semantic feasibility merge is missing a batch result")
	}
	expectedCoverage := make(map[common.CoverageWorkItemID]struct{})
	for _, document := range documentsByBatchID {
		for _, proposal := range document.Proposals {
			if _, duplicate := expectedCoverage[proposal.CoverageWorkItemID]; duplicate {
				return nil, fmt.Errorf("runtime semantic feasibility merge duplicates proposal coverage %q", proposal.CoverageWorkItemID)
			}
			expectedCoverage[proposal.CoverageWorkItemID] = struct{}{}
		}
	}
	if len(seenCoverage) != len(expectedCoverage) {
		return nil, fmt.Errorf("runtime semantic feasibility results do not cover every batch proposal")
	}
	for coverageID := range expectedCoverage {
		if _, found := seenCoverage[coverageID]; !found {
			return nil, fmt.Errorf("runtime semantic feasibility results are missing coverage %q", coverageID)
		}
	}
	return merged, nil
}

func sameOrderedSemanticBatchStrings(left, right []string) bool {
	left = normalizeOrderedStrings(left)
	right = normalizeOrderedStrings(right)
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}
	return true
}
