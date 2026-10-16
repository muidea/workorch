package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeOperationExecutionState is the per-operation execution transaction state (SM-3).
type RuntimeOperationExecutionState string

const (
	OperationExecutionSealed            RuntimeOperationExecutionState = "sealed"
	OperationExecutionClaimed           RuntimeOperationExecutionState = "claimed"
	OperationExecutionOwnerCommitted    RuntimeOperationExecutionState = "owner_committed"
	OperationExecutionEvidenceCommitted RuntimeOperationExecutionState = "evidence_committed"
	OperationExecutionOutcomeUnknown    RuntimeOperationExecutionState = "outcome_unknown"
)

const (
	RuntimeOperationClaimMarkerInitial                 = "claimed"
	RuntimeOperationClaimMarkerGovernanceGranted       = "governance_granted"
	RuntimeOperationClaimMarkerGovernanceEffectStarted = "governance_effect_started"
)

func ValidRuntimeOperationClaimMarker(value string) bool {
	switch strings.TrimSpace(value) {
	case RuntimeOperationClaimMarkerInitial, RuntimeOperationClaimMarkerGovernanceGranted, RuntimeOperationClaimMarkerGovernanceEffectStarted:
		return true
	default:
		return false
	}
}

func RuntimeOperationClaimMarkerIsGovernance(value string) bool {
	value = strings.TrimSpace(value)
	return value == RuntimeOperationClaimMarkerGovernanceGranted || value == RuntimeOperationClaimMarkerGovernanceEffectStarted
}

// RuntimeOperationOutcome is the owner-reported terminal outcome for an execution key.
type RuntimeOperationOutcome string

const (
	RuntimeOperationOutcomeAccepted           RuntimeOperationOutcome = "accepted"
	RuntimeOperationOutcomeRejected           RuntimeOperationOutcome = "rejected"
	RuntimeOperationOutcomeGovernanceRequired RuntimeOperationOutcome = "governance_required"
	RuntimeOperationOutcomeNoOp               RuntimeOperationOutcome = "no_op"
	RuntimeOperationOutcomeUnknown            RuntimeOperationOutcome = "unknown"
)

// RuntimeOperationExecutionRecord is the persisted per-operation claim/fence record
// inside a sealed dispatch. ExecutionKey must be stable for workspace/run/dispatch/operation.
type RuntimeOperationExecutionRecord struct {
	OperationID         string
	OperationHash       string
	ExecutionKey        string
	ExpectedEffectScope RuntimeEffectScope
	// IdempotencyClass is the replay policy resolved when SM-3 seals this
	// execution. Recovery must never re-resolve it from a newer owner catalog.
	IdempotencyClass RuntimeOperationIdempotencyClass
	// AuthorityHash independently seals the immutable execution authority. Claim,
	// receipt and evidence state may advance without changing this hash.
	AuthorityHash string
	// AdmissibilitySnapshotID/AcquisitionBinding persist the exact SM-2 owner
	// catalog used by the side-effect-free feasibility gate. Capability execute
	// producers may not substitute another current catalog during SM-3.
	AdmissibilitySnapshotID string
	AcquisitionBinding      string
	// ReplayProof is an owner-authored, read-only preflight proof sealed before
	// the operation is claimed. It is optional and can authorize recovery when
	// the sealed idempotency class alone does not; both authorities are immutable
	// after dispatch seal.
	ReplayProof      *RuntimeOperationReplayProof
	State            RuntimeOperationExecutionState
	Fence            uint64
	OwnerReceiptRef  string
	OwnerReceiptHash string
	// EvidenceFactID/EvidenceFactHash are the mutable SM-3 commit projection
	// written only when the matching owner receipt is promoted into one exact
	// Orchestrator-owned EvidenceFact. They deliberately remain outside
	// AuthorityHash: evidence progress cannot rewrite immutable dispatch
	// authority, while fresh-load validation can still reverse-prove the exact
	// persisted fact instead of re-deriving it from mutable current bindings.
	EvidenceFactID   common.EvidenceRecordID
	EvidenceFactHash string
	ClaimedAt        string
}

// RuntimeOperationReplayProof binds an owner-observed precondition and
// postcondition to one sealed operation. It is a DTO-only contract: owners
// create it during a side-effect-free feasibility probe, the orchestrator
// persists it, and the owner verifies it again during same-key/fence recovery.
// It must not select actions or itself constitute execution evidence.
type RuntimeOperationReplayProof struct {
	OwnerModuleID           string
	OperationID             string
	TargetRefs              []string
	BeforeHash              string
	AfterHash               string
	AdmissibilitySnapshotID string
	AcquisitionBinding      string
}

func (p RuntimeOperationReplayProof) Normalized() RuntimeOperationReplayProof {
	p.OwnerModuleID = strings.TrimSpace(p.OwnerModuleID)
	p.OperationID = strings.TrimSpace(p.OperationID)
	p.TargetRefs = normalizeStrings(p.TargetRefs)
	p.BeforeHash = strings.TrimSpace(p.BeforeHash)
	p.AfterHash = strings.TrimSpace(p.AfterHash)
	p.AdmissibilitySnapshotID = strings.TrimSpace(p.AdmissibilitySnapshotID)
	p.AcquisitionBinding = strings.TrimSpace(p.AcquisitionBinding)
	return p
}

func (p RuntimeOperationReplayProof) Validate() error {
	p = p.Normalized()
	if p.OwnerModuleID == "" || p.OperationID == "" || len(p.TargetRefs) == 0 || p.AfterHash == "" || p.AdmissibilitySnapshotID == "" {
		return fmt.Errorf("runtime operation replay proof requires owner, operation, targets, output and admissibility identity")
	}
	if p.BeforeHash == p.AfterHash {
		return fmt.Errorf("runtime operation replay proof requires a changed postcondition")
	}
	return nil
}

func (p RuntimeOperationReplayProof) Clone() RuntimeOperationReplayProof {
	p = p.Normalized()
	p.TargetRefs = append([]string(nil), p.TargetRefs...)
	return p
}

// RuntimeOperationReceipt is the owner-persisted receipt for one ExecutionKey.
// Orchestrator commits evidence only from sealed expectation + this receipt (INV-03).
type RuntimeOperationReceipt struct {
	ExecutionKey        string
	Fence               uint64
	OperationID         string
	Outcome             RuntimeOperationOutcome
	ObservedEffectScope RuntimeEffectScope
	Changed             bool
	TargetRefs          []string
	BeforeHash          string
	AfterHash           string
	Evidence            RuntimeOperationEvidence
	// Feedback preserves the producer-authored, audit-safe diagnostic for a
	// non-accepted operation fact. Outcome remains the only operation result
	// authority; Feedback must never select retry, repair, governance or terminal.
	Feedback   *RuntimeFeedback
	Governance *RuntimeGovernanceRequirement
	// ResourceChanges are owner-observed durable revisions changed by this
	// sealed operation. Orchestrator uses them only to invalidate intersecting
	// prerequisite observations.
	ResourceChanges []RuntimeResourceChange
	// PrerequisiteStale is the generic freshness conflict contract. It is
	// intentionally separate from diagnostic feedback so stale never spends candidate
	// retry or repair budget.
	PrerequisiteStale *RuntimePrerequisiteStale
	IdempotentReplay  bool
	CompletedAt       string
}

// RuntimeRequiredOperationReceiptBinding is the typed, bounded proof that a
// required predecessor has already completed in the same sealed dispatch. It
// is carried to a capability owner only for stale-preflight adjudication; it
// is not a second owner-state store.
type RuntimeRequiredOperationReceiptBinding struct {
	RequirementID string
	OperationID   string
	Kind          string
	VariantID     string
	ExecutionKey  string
	Fence         uint64
	ReceiptRef    string
	ReceiptHash   string
	Outcome       RuntimeOperationOutcome
	ObservedScope RuntimeEffectScope
	Changed       bool
}

func (b RuntimeRequiredOperationReceiptBinding) Normalized() RuntimeRequiredOperationReceiptBinding {
	b.RequirementID = strings.TrimSpace(b.RequirementID)
	b.OperationID = strings.TrimSpace(b.OperationID)
	b.Kind = CanonicalRuntimeBlockKind(b.Kind)
	b.VariantID = strings.TrimSpace(b.VariantID)
	b.ExecutionKey = strings.TrimSpace(b.ExecutionKey)
	b.ReceiptRef = strings.TrimSpace(b.ReceiptRef)
	b.ReceiptHash = strings.TrimSpace(b.ReceiptHash)
	b.Outcome = b.Outcome.Normalized()
	b.ObservedScope = NormalizeRuntimeEffectScope(b.ObservedScope)
	return b
}

func (b RuntimeRequiredOperationReceiptBinding) Validate() error {
	b = b.Normalized()
	if b.RequirementID == "" || b.OperationID == "" || b.Kind == "" || b.VariantID == "" || b.ExecutionKey == "" || b.Fence == 0 || b.ReceiptRef == "" || b.ReceiptHash == "" {
		return fmt.Errorf("required operation receipt binding requires requirement, operation, variant and receipt identity")
	}
	if !ValidRuntimeOperationOutcome(b.Outcome) || !ValidRuntimeEffectScope(b.ObservedScope) {
		return fmt.Errorf("required operation receipt binding has invalid receipt outcome or observed scope")
	}
	return nil
}

func NormalizeRuntimeRequiredOperationReceiptBindings(values []RuntimeRequiredOperationReceiptBinding) []RuntimeRequiredOperationReceiptBinding {
	byRequirement := map[string]RuntimeRequiredOperationReceiptBinding{}
	for _, value := range values {
		value = value.Normalized()
		if value.RequirementID != "" {
			byRequirement[value.RequirementID] = value
		}
	}
	keys := make([]string, 0, len(byRequirement))
	for key := range byRequirement {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ret := make([]RuntimeRequiredOperationReceiptBinding, 0, len(keys))
	for _, key := range keys {
		ret = append(ret, byRequirement[key])
	}
	return nilIfEmpty(ret)
}

// StableRuntimeOperationExecutionKey binds workspace/run/dispatch/operation into one key.
func StableRuntimeOperationExecutionKey(workspaceID common.WorkspaceID, runID common.RunID, dispatchID, operationID string) string {
	workspaceID = common.WorkspaceID(strings.TrimSpace(string(workspaceID)))
	runID = common.RunID(strings.TrimSpace(string(runID)))
	dispatchID = strings.TrimSpace(dispatchID)
	operationID = strings.TrimSpace(operationID)
	return fmt.Sprintf("exec/%s/%s/%s/%s", workspaceID, runID, dispatchID, operationID)
}

// RuntimeOperationExecutionKeyOrigin validates the canonical execution-key
// scope and returns the dispatch that originally minted the key. Governance
// continuation may preserve that origin key, so callers must separately prove
// whether a non-current origin is authorized by the aggregate history.
func RuntimeOperationExecutionKeyOrigin(workspaceID common.WorkspaceID, runID common.RunID, executionKey, operationID string) (string, error) {
	executionKey = strings.TrimSpace(executionKey)
	operationID = strings.TrimSpace(operationID)
	prefix := fmt.Sprintf("exec/%s/%s/", strings.TrimSpace(string(workspaceID)), strings.TrimSpace(string(runID)))
	suffix := "/" + operationID
	if operationID == "" || !strings.HasPrefix(executionKey, prefix) || !strings.HasSuffix(executionKey, suffix) {
		return "", fmt.Errorf("operation execution key does not match workspace, run and operation scope")
	}
	origin := strings.TrimSuffix(strings.TrimPrefix(executionKey, prefix), suffix)
	if origin == "" || strings.Contains(origin, "/") || executionKey != StableRuntimeOperationExecutionKey(workspaceID, runID, origin, operationID) {
		return "", fmt.Errorf("operation execution key has invalid origin dispatch")
	}
	return origin, nil
}

func (s RuntimeOperationExecutionState) Normalized() RuntimeOperationExecutionState {
	return RuntimeOperationExecutionState(strings.TrimSpace(string(s)))
}

func ValidRuntimeOperationExecutionState(state RuntimeOperationExecutionState) bool {
	switch state.Normalized() {
	case OperationExecutionSealed, OperationExecutionClaimed, OperationExecutionOwnerCommitted,
		OperationExecutionEvidenceCommitted, OperationExecutionOutcomeUnknown:
		return true
	default:
		return false
	}
}

func (o RuntimeOperationOutcome) Normalized() RuntimeOperationOutcome {
	return RuntimeOperationOutcome(strings.TrimSpace(string(o)))
}

func ValidRuntimeOperationOutcome(outcome RuntimeOperationOutcome) bool {
	switch outcome.Normalized() {
	case RuntimeOperationOutcomeAccepted, RuntimeOperationOutcomeRejected,
		RuntimeOperationOutcomeGovernanceRequired, RuntimeOperationOutcomeNoOp,
		RuntimeOperationOutcomeUnknown:
		return true
	default:
		return false
	}
}

func (r RuntimeOperationExecutionRecord) Normalized() RuntimeOperationExecutionRecord {
	r.OperationID = strings.TrimSpace(r.OperationID)
	r.OperationHash = strings.TrimSpace(r.OperationHash)
	r.ExecutionKey = strings.TrimSpace(r.ExecutionKey)
	r.ExpectedEffectScope = NormalizeRuntimeEffectScope(r.ExpectedEffectScope)
	r.IdempotencyClass = NormalizeRuntimeOperationIdempotencyClass(r.IdempotencyClass)
	r.AuthorityHash = strings.TrimSpace(r.AuthorityHash)
	r.AdmissibilitySnapshotID = strings.TrimSpace(r.AdmissibilitySnapshotID)
	r.AcquisitionBinding = strings.TrimSpace(r.AcquisitionBinding)
	if r.ReplayProof != nil {
		proof := r.ReplayProof.Normalized()
		r.ReplayProof = &proof
	}
	r.State = r.State.Normalized()
	r.OwnerReceiptRef = strings.TrimSpace(r.OwnerReceiptRef)
	r.OwnerReceiptHash = strings.TrimSpace(r.OwnerReceiptHash)
	r.EvidenceFactID = common.EvidenceRecordID(strings.TrimSpace(string(r.EvidenceFactID)))
	r.EvidenceFactHash = strings.TrimSpace(r.EvidenceFactHash)
	r.ClaimedAt = strings.TrimSpace(r.ClaimedAt)
	return r
}

func (r RuntimeOperationExecutionRecord) Validate() error {
	raw := r
	rawEffectScope := r.ExpectedEffectScope
	r = r.Normalized()
	if raw.OperationID != r.OperationID || raw.OperationHash != r.OperationHash || raw.ExecutionKey != r.ExecutionKey ||
		raw.State != r.State || raw.AuthorityHash != r.AuthorityHash || raw.AdmissibilitySnapshotID != r.AdmissibilitySnapshotID ||
		raw.AcquisitionBinding != r.AcquisitionBinding || raw.OwnerReceiptRef != r.OwnerReceiptRef ||
		raw.OwnerReceiptHash != r.OwnerReceiptHash || raw.EvidenceFactID != r.EvidenceFactID ||
		raw.EvidenceFactHash != r.EvidenceFactHash || raw.ClaimedAt != r.ClaimedAt {
		return fmt.Errorf("operation execution record requires canonical identity, state and receipt fields")
	}
	if raw.ReplayProof != nil && !canonicalRuntimeOperationReplayProof(*raw.ReplayProof) {
		return fmt.Errorf("operation execution replay proof requires canonical fields")
	}
	if r.OperationID == "" || r.OperationHash == "" || r.ExecutionKey == "" {
		return fmt.Errorf("operation execution record requires operation id, operation hash and execution key")
	}
	if !ValidRuntimeOperationExecutionState(r.State) {
		return fmt.Errorf("operation execution record has invalid state %q", r.State)
	}
	if !validSealedRuntimeEffectScope(rawEffectScope) {
		return fmt.Errorf("operation execution record requires an explicit resolved expected effect scope")
	}
	if !validSealedRuntimeOperationIdempotencyClass(raw.IdempotencyClass) {
		return fmt.Errorf("operation execution record requires an explicit valid idempotency class")
	}
	if r.AuthorityHash == "" || r.AuthorityHash != StableRuntimeOperationExecutionAuthorityHash(r) {
		return fmt.Errorf("operation execution record authority hash mismatch")
	}
	if r.ClaimedAt != "" && !ValidRuntimeOperationClaimMarker(r.ClaimedAt) {
		return fmt.Errorf("operation execution record has invalid claim marker %q", r.ClaimedAt)
	}
	if r.AdmissibilitySnapshotID == "" && r.AcquisitionBinding != "" {
		return fmt.Errorf("operation execution record acquisition binding requires owner catalog snapshot")
	}
	if r.ReplayProof != nil {
		if err := r.ReplayProof.Validate(); err != nil {
			return fmt.Errorf("operation execution replay proof: %w", err)
		}
		if r.ReplayProof.OperationID != r.OperationID {
			return fmt.Errorf("operation execution replay proof operation mismatch")
		}
		if r.ReplayProof.AdmissibilitySnapshotID != r.AdmissibilitySnapshotID || r.ReplayProof.AcquisitionBinding != r.AcquisitionBinding {
			return fmt.Errorf("operation execution replay proof owner authority mismatch")
		}
	}
	switch r.State {
	case OperationExecutionSealed:
		if r.Fence != 0 || r.OwnerReceiptRef != "" || r.OwnerReceiptHash != "" || r.EvidenceFactID != "" || r.EvidenceFactHash != "" || r.ClaimedAt != "" {
			return fmt.Errorf("sealed operation execution cannot carry fence, receipt, evidence or claim marker")
		}
	case OperationExecutionClaimed:
		if r.Fence == 0 || r.OwnerReceiptRef != "" || r.OwnerReceiptHash != "" || r.EvidenceFactID != "" || r.EvidenceFactHash != "" {
			return fmt.Errorf("claimed operation execution requires fence and cannot carry an owner receipt or evidence")
		}
		if !ValidRuntimeOperationClaimMarker(r.ClaimedAt) {
			return fmt.Errorf("claimed operation execution requires a valid claim marker")
		}
	case OperationExecutionOwnerCommitted, OperationExecutionEvidenceCommitted:
		if !ValidRuntimeOperationClaimMarker(r.ClaimedAt) || r.ClaimedAt == RuntimeOperationClaimMarkerGovernanceGranted {
			return fmt.Errorf("committed operation execution requires a started claim marker")
		}
		if r.Fence == 0 || r.OwnerReceiptRef == "" || r.OwnerReceiptHash == "" {
			return fmt.Errorf("committed operation execution requires fence and owner receipt ref/hash")
		}
		if r.State == OperationExecutionOwnerCommitted && (r.EvidenceFactID != "" || r.EvidenceFactHash != "") {
			return fmt.Errorf("owner_committed operation execution cannot carry evidence fact identity")
		}
		if r.State == OperationExecutionEvidenceCommitted && (r.EvidenceFactID == "" || r.EvidenceFactHash == "") {
			return fmt.Errorf("evidence_committed operation execution requires evidence fact id/hash")
		}
	case OperationExecutionOutcomeUnknown:
		if !ValidRuntimeOperationClaimMarker(r.ClaimedAt) || r.ClaimedAt == RuntimeOperationClaimMarkerGovernanceGranted {
			return fmt.Errorf("unknown operation execution requires a started claim marker")
		}
		if r.Fence == 0 || r.OwnerReceiptRef != "" || r.OwnerReceiptHash != "" || r.EvidenceFactID != "" || r.EvidenceFactHash != "" {
			return fmt.Errorf("outcome_unknown operation execution requires fence and cannot carry an owner receipt or evidence")
		}
	}
	return nil
}

func canonicalRuntimeOperationReplayProof(value RuntimeOperationReplayProof) bool {
	normalized := value.Normalized()
	return value.OwnerModuleID == normalized.OwnerModuleID && value.OperationID == normalized.OperationID &&
		value.BeforeHash == normalized.BeforeHash && value.AfterHash == normalized.AfterHash &&
		value.AdmissibilitySnapshotID == normalized.AdmissibilitySnapshotID && value.AcquisitionBinding == normalized.AcquisitionBinding &&
		sameStrings(value.TargetRefs, normalized.TargetRefs)
}

// StableRuntimeOperationExecutionAuthorityHash seals only the immutable SM-3
// owner authority. Mutable claim/fence/receipt/evidence fields are deliberately
// excluded so normal transaction progress cannot rewrite the seal.
func StableRuntimeOperationExecutionAuthorityHash(value RuntimeOperationExecutionRecord) string {
	value = value.Normalized()
	h := sha256.New()
	writeHashString(h, "runtime-operation-execution-authority/v1")
	writeHashString(h, value.OperationID)
	writeHashString(h, value.OperationHash)
	writeHashString(h, value.ExecutionKey)
	writeHashString(h, string(value.ExpectedEffectScope))
	writeHashString(h, string(value.IdempotencyClass))
	writeHashString(h, value.AdmissibilitySnapshotID)
	writeHashString(h, value.AcquisitionBinding)
	if value.ReplayProof == nil {
		writeHashString(h, "no-replay-proof")
	} else {
		proof := value.ReplayProof.Normalized()
		writeHashString(h, "replay-proof")
		writeHashString(h, proof.OwnerModuleID)
		writeHashString(h, proof.OperationID)
		writeHashStrings(h, proof.TargetRefs)
		writeHashString(h, proof.BeforeHash)
		writeHashString(h, proof.AfterHash)
		writeHashString(h, proof.AdmissibilitySnapshotID)
		writeHashString(h, proof.AcquisitionBinding)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// StableRuntimeOperationExecutionAuthoritiesHash seals the exact immutable
// execution-authority set of a dispatch. Sorting by operation identity makes
// the seal independent of storage order while the dispatch validator enforces
// the candidate-to-ledger one-to-one relationship.
func StableRuntimeOperationExecutionAuthoritiesHash(values []RuntimeOperationExecutionRecord) string {
	if len(values) == 0 {
		return ""
	}
	canonical := make([]RuntimeOperationExecutionRecord, len(values))
	for idx := range values {
		canonical[idx] = values[idx].Normalized()
	}
	sort.Slice(canonical, func(i, j int) bool {
		if canonical[i].OperationID == canonical[j].OperationID {
			return canonical[i].ExecutionKey < canonical[j].ExecutionKey
		}
		return canonical[i].OperationID < canonical[j].OperationID
	})
	h := sha256.New()
	writeHashString(h, "runtime-operation-execution-authorities/v1")
	writeHashString(h, fmt.Sprintf("%d", len(canonical)))
	for _, value := range canonical {
		writeHashString(h, value.OperationID)
		writeHashString(h, value.ExecutionKey)
		writeHashString(h, value.AuthorityHash)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (r RuntimeOperationExecutionRecord) Clone() RuntimeOperationExecutionRecord {
	ret := r
	if r.ReplayProof != nil {
		proof := *r.ReplayProof
		proof.TargetRefs = append([]string(nil), r.ReplayProof.TargetRefs...)
		ret.ReplayProof = &proof
	}
	return ret
}

func CloneRuntimeOperationExecutionRecords(values []RuntimeOperationExecutionRecord) []RuntimeOperationExecutionRecord {
	if values == nil {
		return nil
	}
	ret := make([]RuntimeOperationExecutionRecord, len(values))
	for idx := range values {
		ret[idx] = values[idx].Clone()
	}
	return ret
}

func NormalizeRuntimeOperationExecutionRecords(values []RuntimeOperationExecutionRecord) []RuntimeOperationExecutionRecord {
	if len(values) == 0 {
		return nil
	}
	ret := make([]RuntimeOperationExecutionRecord, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = value.Normalized()
		if value.OperationID == "" {
			continue
		}
		if _, ok := seen[value.OperationID]; ok {
			continue
		}
		seen[value.OperationID] = struct{}{}
		ret = append(ret, value)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func ValidateRuntimeOperationExecutionRecords(values []RuntimeOperationExecutionRecord) error {
	// Detect duplicates on the raw input before Normalize collapses them.
	seenOps := map[string]struct{}{}
	seenKeys := map[string]struct{}{}
	for _, raw := range values {
		if err := raw.Validate(); err != nil {
			return err
		}
		value := raw.Normalized()
		if _, ok := seenOps[value.OperationID]; ok {
			return fmt.Errorf("operation execution operation id %s is duplicated", value.OperationID)
		}
		seenOps[value.OperationID] = struct{}{}
		if _, ok := seenKeys[value.ExecutionKey]; ok {
			return fmt.Errorf("operation execution key %s is duplicated", value.ExecutionKey)
		}
		seenKeys[value.ExecutionKey] = struct{}{}
	}
	return nil
}

func SameRuntimeOperationExecutionRecords(left []RuntimeOperationExecutionRecord, right []RuntimeOperationExecutionRecord) bool {
	left = NormalizeRuntimeOperationExecutionRecords(left)
	right = NormalizeRuntimeOperationExecutionRecords(right)
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

func (r RuntimeOperationReceipt) Normalized() RuntimeOperationReceipt {
	if r.Evidence.OwnerSnapshot != nil {
		identity := *r.Evidence.OwnerSnapshot
		r.Evidence.OwnerSnapshot = &identity
	}
	r.ExecutionKey = strings.TrimSpace(r.ExecutionKey)
	r.OperationID = strings.TrimSpace(r.OperationID)
	r.Outcome = r.Outcome.Normalized()
	r.ObservedEffectScope = NormalizeRuntimeEffectScope(r.ObservedEffectScope)
	r.TargetRefs = normalizeStrings(r.TargetRefs)
	r.BeforeHash = strings.TrimSpace(r.BeforeHash)
	r.AfterHash = strings.TrimSpace(r.AfterHash)
	r.Evidence.Summary = strings.TrimSpace(r.Evidence.Summary)
	r.Evidence.ContentHash = strings.TrimSpace(r.Evidence.ContentHash)
	if r.Evidence.ContentAddress != nil {
		address := r.Evidence.ContentAddress.Clone()
		r.Evidence.ContentAddress = &address
	}
	r.Evidence.EvidenceRefs = normalizeStrings(r.Evidence.EvidenceRefs)
	r.Evidence.Quality = common.EvidenceQuality(strings.TrimSpace(string(r.Evidence.Quality)))
	r.Evidence.EvidenceReason = strings.TrimSpace(r.Evidence.EvidenceReason)
	r.Evidence.EvidenceTags = normalizeStrings(r.Evidence.EvidenceTags)
	if r.Evidence.Validation != nil {
		observation := r.Evidence.Validation.Clone()
		r.Evidence.Validation = &observation
	}
	r.Evidence.OwnerRevisionSet = CloneRuntimeOwnerRevisionSet(r.Evidence.OwnerRevisionSet)
	r.Evidence.ApplicabilityDependencies = CloneRuntimeEvidenceApplicabilityDependencies(r.Evidence.ApplicabilityDependencies)
	r.ResourceChanges = NormalizeRuntimeResourceChanges(r.ResourceChanges)
	if r.Feedback != nil {
		feedback := r.Feedback.CloneOriginal()
		r.Feedback = &feedback
	}
	if r.Governance != nil {
		gov := r.Governance.Normalized()
		r.Governance = &gov
	}
	if r.PrerequisiteStale != nil {
		value := r.PrerequisiteStale.Normalized()
		r.PrerequisiteStale = &value
	}
	r.CompletedAt = strings.TrimSpace(r.CompletedAt)
	return r
}

func (r RuntimeOperationReceipt) Validate() error {
	if err := ValidateRuntimeEvidenceDependencyPair(r.Evidence.OwnerRevisionSet, r.Evidence.ApplicabilityDependencies, ""); err != nil {
		return fmt.Errorf("operation receipt dependency pair: %w", err)
	}
	if value := r.Evidence.ApplicabilityDependencies; value != nil {
		if value.ProtocolRef == RuntimeEvidenceApplicabilityDependenciesV2 && (!RuntimeEffectIsDurable(r.ObservedEffectScope) || r.Evidence.Validation != nil) {
			return fmt.Errorf("historical effect purpose requires durable non-validation receipt")
		}
	}
	if err := validateRuntimeOwnerRevisionSetRaw(r.Evidence.OwnerRevisionSet); err != nil {
		return fmt.Errorf("operation receipt owner dependencies: %w", err)
	}
	r = r.Normalized()
	if r.ExecutionKey == "" || r.Fence == 0 || r.OperationID == "" {
		return fmt.Errorf("operation receipt requires execution key, fence and operation id")
	}
	if !ValidRuntimeOperationOutcome(r.Outcome) {
		return fmt.Errorf("operation receipt has invalid outcome %q", r.Outcome)
	}
	if !ValidRuntimeEffectScope(r.ObservedEffectScope) {
		return fmt.Errorf("operation receipt has invalid observed effect scope %q", r.ObservedEffectScope)
	}
	if r.Evidence.Validation != nil {
		if err := r.Evidence.Validation.ValidateFailureDetails(); err != nil {
			return err
		}
	}
	if r.Evidence.ContentAddress != nil {
		if err := r.Evidence.ContentAddress.Validate(); err != nil {
			return err
		}
		if r.Evidence.ContentHash != r.Evidence.ContentAddress.Digest || !containsString(r.Evidence.EvidenceRefs, r.Evidence.ContentAddress.Ref) {
			return fmt.Errorf("operation receipt evidence content address does not match content hash and evidence refs")
		}
	}
	if r.Evidence.OwnerSnapshot != nil {
		if err := r.Evidence.OwnerSnapshot.Validate(); err != nil {
			return err
		}
		if r.Outcome != RuntimeOperationOutcomeAccepted || r.Evidence.ContentAddress == nil || r.Evidence.OwnerRevisionSet == nil {
			return fmt.Errorf("owner snapshot requires original accepted archived read dependencies")
		}
	}
	if r.Outcome != RuntimeOperationOutcomeRejected && r.Feedback != nil {
		return fmt.Errorf("only rejected operation receipts can carry diagnostic feedback")
	}
	switch r.Outcome {
	case RuntimeOperationOutcomeAccepted:
		if r.ObservedEffectScope == RuntimeEffectUnknown {
			return fmt.Errorf("accepted operation receipt cannot observe unknown effect")
		}
		if r.Governance != nil {
			return fmt.Errorf("accepted operation receipt cannot carry governance")
		}
	case RuntimeOperationOutcomeNoOp:
		if r.Changed {
			return fmt.Errorf("no_op operation receipt cannot report changed=true")
		}
		if r.ObservedEffectScope != RuntimeEffectNone && r.ObservedEffectScope != RuntimeEffectEphemeralSession {
			return fmt.Errorf("no_op operation receipt must observe none or ephemeral_session")
		}
		if r.Governance != nil {
			return fmt.Errorf("no_op operation receipt cannot carry governance")
		}
	case RuntimeOperationOutcomeGovernanceRequired:
		if r.Governance == nil {
			return fmt.Errorf("governance_required operation receipt requires governance requirement")
		}
		if err := r.Governance.Validate(); err != nil {
			return err
		}
	case RuntimeOperationOutcomeRejected:
		if r.Feedback == nil || r.Feedback.OriginalDiagnostic == nil {
			return fmt.Errorf("rejected operation receipt requires original diagnostic feedback")
		}
		if err := r.Feedback.Validate(); err != nil {
			return err
		}
		if r.Feedback.Domain != RuntimeFeedbackDomainOperation || r.Feedback.StableCategory != RuntimeFeedbackCategory(r.Outcome) || r.Feedback.OriginalDiagnostic.OperationID != r.OperationID {
			return fmt.Errorf("rejected operation receipt feedback does not match receipt outcome or operation")
		}
		failureBinding := FailureFact{TargetRefs: append([]string(nil), r.Feedback.OriginalDiagnostic.TargetRefs...), TargetObservation: r.Feedback.OriginalDiagnostic.TargetObservation}
		if err := ValidateRuntimeFailureTargetObservationBinding(failureBinding); err != nil {
			return fmt.Errorf("rejected operation receipt failure target observation: %w", err)
		}
	case RuntimeOperationOutcomeUnknown:
		// unknown is intentionally sparse; recovery path decides governance vs fail.
	}
	if r.PrerequisiteStale != nil {
		if r.Outcome != RuntimeOperationOutcomeRejected || r.Feedback == nil || r.Feedback.OriginalDiagnostic == nil ||
			(r.Feedback.OriginalDiagnostic.OriginalCode != "prerequisite_stale" && r.Feedback.OriginalDiagnostic.OriginalCode != "operation_admissibility_stale" && r.Feedback.OriginalDiagnostic.OriginalCode != "capability_state_stale") {
			return fmt.Errorf("prerequisite stale receipt requires rejected generic stale detail")
		}
		if err := r.PrerequisiteStale.Validate(); err != nil {
			return err
		}
		if r.Changed || len(r.ResourceChanges) != 0 || r.ObservedEffectScope != RuntimeEffectNone {
			return fmt.Errorf("prerequisite stale receipt cannot report an effect or resource change")
		}
	}
	for _, change := range r.ResourceChanges {
		if err := change.Validate(); err != nil {
			return err
		}
	}
	if len(r.ResourceChanges) > 0 && (!r.Changed || !RuntimeEffectIsDurable(r.ObservedEffectScope)) {
		return fmt.Errorf("resource changes require a durable changed operation receipt")
	}
	if r.CompletedAt != "" {
		if _, err := time.Parse(time.RFC3339, r.CompletedAt); err != nil {
			if _, err2 := time.Parse(time.RFC3339Nano, r.CompletedAt); err2 != nil {
				return fmt.Errorf("operation receipt completedAt must be RFC3339: %w", err)
			}
		}
	}
	return nil
}

func (r RuntimeOperationReceipt) Clone() RuntimeOperationReceipt {
	ret := r.Normalized()
	if r.Feedback != nil {
		feedback := r.Feedback.CloneOriginal()
		ret.Feedback = &feedback
	}
	if r.Governance != nil {
		gov := *ret.Governance
		ret.Governance = &gov
	}
	if r.PrerequisiteStale != nil {
		detail := *r.PrerequisiteStale
		ret.PrerequisiteStale = &detail
	}
	ret.ResourceChanges = append([]RuntimeResourceChange(nil), r.ResourceChanges...)
	if r.Evidence.ExitCode != nil {
		code := *r.Evidence.ExitCode
		ret.Evidence.ExitCode = &code
	}
	return ret
}

func NormalizeRuntimeOperationReceipts(values []RuntimeOperationReceipt) []RuntimeOperationReceipt {
	if len(values) == 0 {
		return nil
	}
	ret := make([]RuntimeOperationReceipt, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = value.Normalized()
		if value.ExecutionKey == "" {
			continue
		}
		key := runtimeOperationReceiptIdentity(value.ExecutionKey, value.Fence)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		ret = append(ret, value.Clone())
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func ValidateRuntimeOperationReceipts(values []RuntimeOperationReceipt) error {
	return validateRuntimeOperationReceiptsInIndex(values, nil)
}

func validateRuntimeOperationReceiptsInIndex(values []RuntimeOperationReceipt, index *runtimeOwnerLineageIndex) error {
	seen := map[string]struct{}{}
	for _, value := range values {
		value = value.Normalized()
		if err := index.receipt(value); err != nil {
			return err
		}
		key := runtimeOperationReceiptIdentity(value.ExecutionKey, value.Fence)
		if _, ok := seen[key]; ok {
			return fmt.Errorf("operation receipt identity %s is duplicated", key)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func RuntimeOperationReceiptForExecutionKeyFence(values []RuntimeOperationReceipt, executionKey string, fence uint64) (RuntimeOperationReceipt, bool) {
	for _, value := range values {
		value = value.Normalized()
		if value.ExecutionKey == strings.TrimSpace(executionKey) && value.Fence == fence {
			return value.Clone(), true
		}
	}
	return RuntimeOperationReceipt{}, false
}

func runtimeOperationReceiptIdentity(executionKey string, fence uint64) string {
	return strings.TrimSpace(executionKey) + "|" + fmt.Sprintf("%d", fence)
}

func SameRuntimeOperationReceipt(left RuntimeOperationReceipt, right RuntimeOperationReceipt) bool {
	return StableRuntimeOperationReceiptHash(left) == StableRuntimeOperationReceiptHash(right)
}

func StableRuntimeOperationReceiptHash(receipt RuntimeOperationReceipt) string {
	receipt = receipt.Normalized()
	h := sha256.New()
	writeHashString(h, receipt.ExecutionKey)
	writeHashString(h, fmt.Sprintf("%d", receipt.Fence))
	writeHashString(h, receipt.OperationID)
	writeHashString(h, string(receipt.Outcome))
	writeHashString(h, string(receipt.ObservedEffectScope))
	writeHashString(h, fmt.Sprintf("%t", receipt.Changed))
	writeHashStrings(h, receipt.TargetRefs)
	writeHashString(h, receipt.BeforeHash)
	writeHashString(h, receipt.AfterHash)
	writeHashString(h, receipt.Evidence.Summary)
	writeHashString(h, receipt.Evidence.ContentHash)
	if receipt.Evidence.OwnerSnapshot != nil {
		writeHashString(h, "owner-snapshot-identity/v1")
		writeHashString(h, stableFormalValueHash("runtime-owner-snapshot-identity", *receipt.Evidence.OwnerSnapshot))
	}
	if receipt.Evidence.OwnerRevisionSet != nil {
		writeHashString(h, "owner-read-dependencies/v1")
		writeHashString(h, stableFormalValueHash("runtime-owner-read-dependencies", *receipt.Evidence.OwnerRevisionSet))
	}
	if receipt.Evidence.ApplicabilityDependencies != nil {
		writeHashString(h, "owner-applicability-dependencies/v1")
		writeHashString(h, stableFormalValueHash("runtime-owner-applicability-dependencies", *receipt.Evidence.ApplicabilityDependencies))
	}
	if receipt.Evidence.ContentAddress == nil {
		writeHashString(h, "content-address:nil")
	} else {
		address := receipt.Evidence.ContentAddress.Normalized()
		writeHashString(h, string(address.Algorithm))
		writeHashString(h, address.Digest)
		writeHashString(h, address.Ref)
	}
	writeHashStrings(h, receipt.Evidence.EvidenceRefs)
	writeHashString(h, string(receipt.Evidence.Quality))
	writeHashString(h, fmt.Sprintf("%t", receipt.Evidence.ZeroWork))
	writeHashString(h, receipt.Evidence.EvidenceReason)
	writeHashStrings(h, receipt.Evidence.EvidenceTags)
	if receipt.Evidence.Validation == nil {
		writeHashString(h, "validation:nil")
	} else {
		observation := receipt.Evidence.Validation.Normalized()
		writeHashString(h, observation.RecipeID)
		writeHashString(h, observation.ObservationProtocolRef)
		writeHashStrings(h, observation.ExecutedChecks)
		writeHashStrings(h, observation.PassedChecks)
		writeHashStrings(h, observation.FailedChecks)
		writeHashStrings(h, observation.SkippedChecks)
		writeHashString(h, observation.OutputPreview)
		writeRuntimeValidationFailureDetailsHash(h, observation)
		writeRuntimeValidationExecutionHash(h, observation)
		if observation.ProofClass != "" {
			writeHashString(h, "validation-proof-class/v1")
			writeHashString(h, string(observation.ProofClass))
		}
	}
	if receipt.Evidence.ExitCode == nil {
		writeHashString(h, "exit:nil")
	} else {
		writeHashString(h, fmt.Sprintf("exit:%d", *receipt.Evidence.ExitCode))
	}
	if receipt.Feedback == nil {
		writeHashString(h, "feedback:nil")
	} else {
		writeHashString(h, StableRuntimeFeedbackHash(*receipt.Feedback))
	}
	if receipt.Governance == nil {
		writeHashString(h, "governance:nil")
	} else {
		writeHashString(h, string(receipt.Governance.Kind))
		writeHashString(h, receipt.Governance.SubjectRef)
		writeHashString(h, receipt.Governance.Reason)
		writeHashString(h, receipt.Governance.RequestedAction)
		writeHashString(h, receipt.Governance.RiskLevel)
		writeHashString(h, receipt.Governance.AuthorizationContext.CanonicalString())
	}
	for _, change := range receipt.ResourceChanges {
		writeHashString(h, string(change.Domain))
		writeHashString(h, change.OwnerModuleID)
		writeHashString(h, change.SubjectRef)
		writeHashString(h, change.BeforeRevision)
		writeHashString(h, change.AfterRevision)
	}
	if receipt.PrerequisiteStale == nil {
		writeHashString(h, "prerequisite-stale:nil")
	} else {
		detail := receipt.PrerequisiteStale
		writeHashString(h, detail.PrerequisiteID)
		writeHashString(h, detail.ObservationID)
		writeHashString(h, string(detail.Domain))
		writeHashString(h, detail.OwnerModuleID)
		writeHashString(h, detail.SubjectRef)
		writeHashString(h, detail.ExpectedRevision)
		writeHashString(h, detail.ObservedRevision)
		writeHashString(h, detail.Reason)
	}
	writeHashString(h, fmt.Sprintf("%t", receipt.IdempotentReplay))
	return hex.EncodeToString(h.Sum(nil))
}

// EvidenceKindFromReceipt derives committed evidence kind from sealed convergence
// state expectation and owner receipt. State alone is never sufficient (INV-03).
func EvidenceKindFromReceipt(state ConvergenceState, expected RuntimeEffectScope, receipt RuntimeOperationReceipt) (string, error) {
	receipt = receipt.Normalized()
	expected = NormalizeRuntimeEffectScope(expected)
	if err := receipt.Validate(); err != nil {
		return "", err
	}
	policy, policyKnown := RuntimeConvergenceEffectPolicyFor(state)
	if !policyKnown {
		return "", fmt.Errorf("convergence state %q has no receipt effect policy", state)
	}
	switch receipt.Outcome {
	case RuntimeOperationOutcomeRejected:
		// rejected still forms evidence of the expected kind with failure facts
		return evidenceKindForConvergenceState(state), nil
	case RuntimeOperationOutcomeGovernanceRequired:
		return "", fmt.Errorf("governance_required receipt cannot commit ordinary evidence")
	case RuntimeOperationOutcomeUnknown:
		return "", fmt.Errorf("unknown outcome receipt cannot commit evidence")
	case RuntimeOperationOutcomeNoOp:
		// no-op may be diagnostic/validation observation only
		if state == ConvergenceStateNeedValidation && RuntimeEffectIsNonDurable(receipt.ObservedEffectScope) {
			return EvidenceKindValidation, nil
		}
		return "", fmt.Errorf("no_op receipt cannot close durable state-change or non-validation actions")
	case RuntimeOperationOutcomeAccepted:
		if receipt.ObservedEffectScope == RuntimeEffectUnknown {
			return "", fmt.Errorf("accepted receipt with unknown effect cannot commit evidence")
		}
		// durable mismatch: owner observed non-durable while seal expected durable
		if RuntimeEffectIsDurable(expected) && !RuntimeEffectIsDurable(receipt.ObservedEffectScope) {
			return "", fmt.Errorf("receipt observed effect %q cannot satisfy expected durable effect %q", receipt.ObservedEffectScope, expected)
		}
		if !policy.AllowsAcceptedReceiptEffect(receipt.ObservedEffectScope) {
			return "", fmt.Errorf("convergence state %s cannot commit accepted receipt effect %q", state, receipt.ObservedEffectScope)
		}
		return evidenceKindForConvergenceState(state), nil
	default:
		return "", fmt.Errorf("unsupported receipt outcome %q", receipt.Outcome)
	}
}

func evidenceKindForConvergenceState(state ConvergenceState) string {
	switch state {
	case ConvergenceStateNeedStateChange:
		return EvidenceKindStateChange
	case ConvergenceStateNeedValidation:
		return EvidenceKindValidation
	default:
		return ""
	}
}
