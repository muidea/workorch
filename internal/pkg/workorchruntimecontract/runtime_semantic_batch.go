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
	// RequestLogicalID is the immutable semantic generation identity sealed
	// when this proof batch opens. A later correction has its own provider
	// request-session identity and must not replace this value.
	RequestLogicalID string
	// DocumentHash seals the exact accepted attempt-0/attempt-1 document under
	// the batch generation. Together these fields are the binding preimage.
	DocumentHash          string
	DependencyBarrierRefs []string
	State                 RuntimeSemanticPlanBatchState
	Hash                  string
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

func runtimeSemanticPendingDependencyBarriers(items map[common.CoverageWorkItemID]RuntimeCoverageItem, pending []common.CoverageWorkItemID, selected map[common.CoverageWorkItemID]struct{}) []string {
	barriers := make([]string, 0)
	for _, id := range pending {
		for _, prerequisiteID := range items[id].PrerequisiteCoverageWorkItemIDs {
			if _, inBatch := selected[prerequisiteID]; !inBatch {
				barriers = append(barriers, string(prerequisiteID)+"->"+string(id))
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
// would make a partial, current-focus proof indistinguishable from an unscoped
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
	seenCoverage := make(map[common.CoverageWorkItemID]struct{}, len(manifest.CoverageItems))
	proposals := make([]RuntimeSemanticProofProposal, 0, len(manifest.CoverageItems))
	for idx, document := range documents {
		document = document.Normalized()
		if document.PlanStep != RuntimeSemanticPlanStepProofBatch || document.BatchID == "" || document.BatchSequence != idx || document.ManifestHash != manifest.Hash || document.DefinitionSeedHash != manifest.DefinitionSeedHash {
			return RuntimeSemanticPlanDocument{}, fmt.Errorf("runtime semantic merge has invalid batch document identity at sequence %d", idx)
		}
		if err := document.ValidateAgainstManifest(manifest); err != nil {
			return RuntimeSemanticPlanDocument{}, err
		}
		batchCoverage := make(map[common.CoverageWorkItemID]struct{}, len(document.BatchCoverageWorkItemIDs))
		for _, coverageID := range document.BatchCoverageWorkItemIDs {
			if _, duplicate := seenCoverage[coverageID]; duplicate {
				return RuntimeSemanticPlanDocument{}, fmt.Errorf("runtime semantic merge duplicates coverage %q", coverageID)
			}
			batchCoverage[coverageID] = struct{}{}
		}
		for _, coverageID := range document.BatchCoverageWorkItemIDs {
			seenCoverage[coverageID] = struct{}{}
		}
		for _, proposal := range document.Proposals {
			if _, found := batchCoverage[proposal.CoverageWorkItemID]; !found {
				return RuntimeSemanticPlanDocument{}, fmt.Errorf("runtime semantic merge proof coverage %q is outside batch", proposal.CoverageWorkItemID)
			}
			proposals = append(proposals, proposal.Clone())
		}
	}
	if len(seenCoverage) != len(manifest.CoverageItems) {
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
// only enforces one exact observation per independently feasible proof.
func ValidateRuntimeSemanticPlanBatchFeasibilities(manifest RuntimeSemanticPlanManifest, document RuntimeSemanticPlanDocument, values []RuntimeSemanticProofFeasibility) error {
	manifest = manifest.Normalized()
	document = document.Normalized()
	if document.PlanStep != RuntimeSemanticPlanStepProofBatch || document.BatchID == "" {
		return fmt.Errorf("runtime semantic feasibility requires a proof batch document")
	}
	if err := document.ValidateAgainstManifest(manifest); err != nil {
		return err
	}
	proposals := make(map[string]RuntimeSemanticProofProposal, len(document.Proposals))
	for _, proposal := range document.Proposals {
		proposals[StableRuntimeSemanticProofProposalHash(proposal)] = proposal
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		proposal, ok := proposals[value.ProposalHash]
		if !ok {
			return fmt.Errorf("runtime semantic batch feasibility proof %q is outside batch", value.ProposalHash)
		}
		if _, duplicate := seen[value.ProposalHash]; duplicate {
			return fmt.Errorf("runtime semantic batch feasibility duplicates proof %q", value.ProposalHash)
		}
		if err := value.ValidateForProposal(manifest, proposal); err != nil {
			return err
		}
		seen[value.ProposalHash] = struct{}{}
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
	seenProofs := make(map[string]struct{})
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
			if _, duplicate := seenProofs[value.ProposalHash]; duplicate {
				return nil, fmt.Errorf("runtime semantic feasibility duplicates proof %q", value.ProposalHash)
			}
			seenProofs[value.ProposalHash] = struct{}{}
			merged = append(merged, value.Clone())
		}
	}
	if len(seenBatch) != len(byID) {
		return nil, fmt.Errorf("runtime semantic feasibility merge is missing a batch result")
	}
	expectedProofs := make(map[string]struct{})
	for _, document := range documentsByBatchID {
		for _, proposal := range document.Proposals {
			proposalHash := StableRuntimeSemanticProofProposalHash(proposal)
			if _, duplicate := expectedProofs[proposalHash]; duplicate {
				return nil, fmt.Errorf("runtime semantic feasibility merge duplicates proposal proof %q", proposalHash)
			}
			expectedProofs[proposalHash] = struct{}{}
		}
	}
	if len(seenProofs) != len(expectedProofs) {
		return nil, fmt.Errorf("runtime semantic feasibility results do not cover every batch proposal")
	}
	for proposalHash := range expectedProofs {
		if _, found := seenProofs[proposalHash]; !found {
			return nil, fmt.Errorf("runtime semantic feasibility results are missing proof %q", proposalHash)
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
