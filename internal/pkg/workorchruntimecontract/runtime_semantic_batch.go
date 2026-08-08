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
	b = b.Normalized()
	manifest = manifest.Normalized()
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
	for _, id := range b.PendingCoverageWorkItemIDs {
		if _, ok := valid[id]; !ok {
			return fmt.Errorf("runtime semantic plan batch pending coverage %q is outside manifest", id)
		}
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

// MergeRuntimeSemanticPlanBatchFeasibilityResults combines one result per
// accepted batch into the complete owner-fact set. Batch identity is checked
// before unioning facts; no result can expand the sealed proposal set.
func MergeRuntimeSemanticPlanBatchFeasibilityResults(manifest RuntimeSemanticPlanManifest, document RuntimeSemanticPlanDocument, batches []RuntimeSemanticPlanBatch, results []RuntimeSemanticPlanBatchFeasibilityResult) ([]RuntimeSemanticProofFeasibility, error) {
	manifest = manifest.Normalized()
	document = document.Normalized()
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	if err := document.ValidateAgainstManifest(manifest); err != nil {
		return nil, err
	}
	byID := make(map[string]RuntimeSemanticPlanBatch, len(batches))
	for _, batch := range batches {
		byID[batch.BatchID] = batch.Normalized()
	}
	seenBatch := make(map[string]struct{}, len(results))
	seenCoverage := make(map[common.CoverageWorkItemID]struct{})
	merged := make([]RuntimeSemanticProofFeasibility, 0)
	for _, result := range results {
		result = result.Normalized()
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
		if batch.State != RuntimeSemanticPlanBatchAccepted {
			return nil, fmt.Errorf("runtime semantic feasibility result references incomplete batch %q", result.BatchID)
		}
		for _, value := range result.Feasibilities {
			if _, duplicate := seenCoverage[value.CoverageWorkItemID]; duplicate {
				return nil, fmt.Errorf("runtime semantic feasibility duplicates coverage %q", value.CoverageWorkItemID)
			}
			seenCoverage[value.CoverageWorkItemID] = struct{}{}
			merged = append(merged, value.Clone())
		}
	}
	if len(seenCoverage) != len(document.Proposals) {
		return nil, fmt.Errorf("runtime semantic feasibility results do not cover merged document")
	}
	return merged, nil
}
