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
	ExecutionKey        string
	ExpectedEffectScope RuntimeEffectScope
	State               RuntimeOperationExecutionState
	Fence               uint64
	OwnerReceiptRef     string
	OwnerReceiptHash    string
	ClaimedAt           string
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
	NoOp                bool
	TargetRefs          []string
	BeforeHash          string
	AfterHash           string
	Evidence            RuntimeOperationEvidence
	Failure             *FailureFact
	Governance          *RuntimeGovernanceRequirement
	// ResourceChanges are owner-observed durable revisions changed by this
	// sealed operation. Orchestrator uses them only to invalidate intersecting
	// prerequisite observations.
	ResourceChanges []RuntimeResourceChange
	// PrerequisiteStale is the generic freshness conflict contract. It is
	// intentionally separate from FailureFact so stale never spends candidate
	// retry or repair budget.
	PrerequisiteStale *RuntimePrerequisiteStale
	// CapabilityStateStale remains readable for existing persisted v2 records
	// during the rollout. New owners must use PrerequisiteStale.
	CapabilityStateStale *RuntimeCapabilityStateStaleDetail
	IdempotentReplay     bool
	CompletedAt          string
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
	r.ExecutionKey = strings.TrimSpace(r.ExecutionKey)
	r.ExpectedEffectScope = NormalizeRuntimeEffectScope(r.ExpectedEffectScope)
	r.State = r.State.Normalized()
	r.OwnerReceiptRef = strings.TrimSpace(r.OwnerReceiptRef)
	r.OwnerReceiptHash = strings.TrimSpace(r.OwnerReceiptHash)
	r.ClaimedAt = strings.TrimSpace(r.ClaimedAt)
	return r
}

func (r RuntimeOperationExecutionRecord) Validate() error {
	r = r.Normalized()
	if r.OperationID == "" || r.ExecutionKey == "" {
		return fmt.Errorf("operation execution record requires operation id and execution key")
	}
	if !ValidRuntimeOperationExecutionState(r.State) {
		return fmt.Errorf("operation execution record has invalid state %q", r.State)
	}
	if !ValidRuntimeEffectScope(r.ExpectedEffectScope) {
		return fmt.Errorf("operation execution record has invalid expected effect scope %q", r.ExpectedEffectScope)
	}
	switch r.State {
	case OperationExecutionSealed:
		if r.Fence != 0 || r.OwnerReceiptRef != "" || r.OwnerReceiptHash != "" {
			return fmt.Errorf("sealed operation execution cannot carry fence or receipt")
		}
	case OperationExecutionClaimed:
		if r.Fence == 0 {
			return fmt.Errorf("claimed operation execution requires fence")
		}
	case OperationExecutionOwnerCommitted, OperationExecutionEvidenceCommitted:
		if r.Fence == 0 || r.OwnerReceiptRef == "" || r.OwnerReceiptHash == "" {
			return fmt.Errorf("committed operation execution requires fence and owner receipt ref/hash")
		}
	case OperationExecutionOutcomeUnknown:
		if r.Fence == 0 {
			return fmt.Errorf("outcome_unknown operation execution requires fence")
		}
	}
	return nil
}

func (r RuntimeOperationExecutionRecord) Clone() RuntimeOperationExecutionRecord {
	return r.Normalized()
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
		value := raw.Normalized()
		if value.OperationID == "" {
			continue
		}
		if err := value.Validate(); err != nil {
			return err
		}
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
	r.ExecutionKey = strings.TrimSpace(r.ExecutionKey)
	r.OperationID = strings.TrimSpace(r.OperationID)
	r.Outcome = r.Outcome.Normalized()
	r.ObservedEffectScope = NormalizeRuntimeEffectScope(r.ObservedEffectScope)
	r.TargetRefs = normalizeStrings(r.TargetRefs)
	r.BeforeHash = strings.TrimSpace(r.BeforeHash)
	r.AfterHash = strings.TrimSpace(r.AfterHash)
	r.Evidence.Summary = strings.TrimSpace(r.Evidence.Summary)
	r.Evidence.ContentHash = strings.TrimSpace(r.Evidence.ContentHash)
	r.Evidence.EvidenceRefs = normalizeStrings(r.Evidence.EvidenceRefs)
	r.Evidence.Quality = common.EvidenceQuality(strings.TrimSpace(string(r.Evidence.Quality)))
	r.Evidence.EvidenceReason = strings.TrimSpace(r.Evidence.EvidenceReason)
	r.Evidence.EvidenceTags = normalizeStrings(r.Evidence.EvidenceTags)
	if r.Evidence.Validation != nil {
		observation := r.Evidence.Validation.Clone()
		r.Evidence.Validation = &observation
	}
	r.ResourceChanges = NormalizeRuntimeResourceChanges(r.ResourceChanges)
	if r.Failure != nil {
		failure := *r.Failure
		failure.Class = FailureClass(strings.TrimSpace(string(failure.Class)))
		failure.Code = strings.TrimSpace(failure.Code)
		failure.Signature = strings.TrimSpace(failure.Signature)
		failure.Message = strings.TrimSpace(failure.Message)
		failure.TargetRefs = normalizeStrings(failure.TargetRefs)
		r.Failure = &failure
	}
	if r.Governance != nil {
		gov := r.Governance.Normalized()
		r.Governance = &gov
	}
	if r.PrerequisiteStale != nil {
		value := r.PrerequisiteStale.Normalized()
		r.PrerequisiteStale = &value
	}
	if r.CapabilityStateStale != nil {
		value := *r.CapabilityStateStale
		value.StateRef.OwnerModuleID = strings.TrimSpace(value.StateRef.OwnerModuleID)
		value.StateRef.CapabilityStateKind = strings.TrimSpace(value.StateRef.CapabilityStateKind)
		value.StateRef.SubjectRef = strings.TrimSpace(value.StateRef.SubjectRef)
		value.SnapshotID = strings.TrimSpace(value.SnapshotID)
		value.ExpectedHash = strings.TrimSpace(value.ExpectedHash)
		value.ObservedRevision = strings.TrimSpace(value.ObservedRevision)
		value.ObservedHash = strings.TrimSpace(value.ObservedHash)
		value.Reason = strings.TrimSpace(value.Reason)
		r.CapabilityStateStale = &value
	}
	r.CompletedAt = strings.TrimSpace(r.CompletedAt)
	if r.NoOp {
		r.Changed = false
	}
	return r
}

func (r RuntimeOperationReceipt) Validate() error {
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
	switch r.Outcome {
	case RuntimeOperationOutcomeAccepted:
		if r.NoOp {
			return fmt.Errorf("accepted operation receipt cannot be no_op")
		}
		if r.ObservedEffectScope == RuntimeEffectUnknown {
			return fmt.Errorf("accepted operation receipt cannot observe unknown effect")
		}
	case RuntimeOperationOutcomeNoOp:
		if r.Changed {
			return fmt.Errorf("no_op operation receipt cannot report changed=true")
		}
		if r.ObservedEffectScope != RuntimeEffectNone && r.ObservedEffectScope != RuntimeEffectEphemeralSession {
			return fmt.Errorf("no_op operation receipt must observe none or ephemeral_session")
		}
	case RuntimeOperationOutcomeGovernanceRequired:
		if r.Governance == nil {
			return fmt.Errorf("governance_required operation receipt requires governance requirement")
		}
		if err := r.Governance.Validate(); err != nil {
			return err
		}
	case RuntimeOperationOutcomeRejected:
		if r.Failure == nil || r.Failure.Class == "" || strings.TrimSpace(r.Failure.Message) == "" {
			return fmt.Errorf("rejected operation receipt requires typed failure")
		}
	case RuntimeOperationOutcomeUnknown:
		// unknown is intentionally sparse; recovery path decides governance vs fail.
	}
	if r.CapabilityStateStale != nil {
		detail := r.CapabilityStateStale
		if r.Outcome != RuntimeOperationOutcomeRejected || r.Failure == nil || r.Failure.Code != "capability_state_stale" ||
			detail.StateRef.OwnerModuleID == "" || detail.StateRef.CapabilityStateKind == "" || detail.StateRef.SubjectRef == "" ||
			detail.SnapshotID == "" || detail.ExpectedHash == "" || detail.ObservedRevision == "" || detail.ObservedHash == "" {
			return fmt.Errorf("capability-state stale receipt requires rejected exact stale detail")
		}
	}
	if r.PrerequisiteStale != nil {
		if r.CapabilityStateStale != nil || r.Outcome != RuntimeOperationOutcomeRejected || r.Failure == nil ||
			(r.Failure.Code != "prerequisite_stale" && r.Failure.Code != "operation_admissibility_stale" && r.Failure.Code != "capability_state_stale") {
			return fmt.Errorf("prerequisite stale receipt requires rejected generic stale detail")
		}
		if err := r.PrerequisiteStale.Validate(); err != nil {
			return err
		}
		if r.Changed || r.NoOp || len(r.ResourceChanges) != 0 || r.ObservedEffectScope != RuntimeEffectNone {
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
	if r.Failure != nil {
		failure := *ret.Failure
		ret.Failure = &failure
	}
	if r.Governance != nil {
		gov := *ret.Governance
		ret.Governance = &gov
	}
	if r.CapabilityStateStale != nil {
		detail := *r.CapabilityStateStale
		ret.CapabilityStateStale = &detail
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
	seen := map[string]struct{}{}
	for _, value := range values {
		value = value.Normalized()
		if err := value.Validate(); err != nil {
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
	writeHashString(h, fmt.Sprintf("%t", receipt.NoOp))
	writeHashStrings(h, receipt.TargetRefs)
	writeHashString(h, receipt.BeforeHash)
	writeHashString(h, receipt.AfterHash)
	writeHashString(h, receipt.Evidence.Summary)
	writeHashString(h, receipt.Evidence.ContentHash)
	writeHashStrings(h, receipt.Evidence.EvidenceRefs)
	writeHashString(h, string(receipt.Evidence.Quality))
	writeHashString(h, fmt.Sprintf("%t", receipt.Evidence.ZeroWork))
	writeHashString(h, receipt.Evidence.EvidenceReason)
	writeHashStrings(h, receipt.Evidence.EvidenceTags)
	if receipt.Evidence.Validation == nil {
		writeHashString(h, "validation:nil")
	} else {
		observation := receipt.Evidence.Validation.Normalized()
		writeHashStrings(h, observation.ExecutedChecks)
		writeHashStrings(h, observation.PassedChecks)
		writeHashStrings(h, observation.FailedChecks)
		writeHashStrings(h, observation.SkippedChecks)
		writeHashString(h, observation.OutputPreview)
	}
	if receipt.Evidence.ExitCode == nil {
		writeHashString(h, "exit:nil")
	} else {
		writeHashString(h, fmt.Sprintf("exit:%d", *receipt.Evidence.ExitCode))
	}
	if receipt.Failure == nil {
		writeHashString(h, "failure:nil")
	} else {
		writeHashString(h, string(receipt.Failure.Class))
		writeHashString(h, receipt.Failure.Code)
		writeHashString(h, receipt.Failure.Signature)
		writeHashString(h, receipt.Failure.Message)
		writeHashStrings(h, receipt.Failure.TargetRefs)
	}
	if receipt.Governance == nil {
		writeHashString(h, "governance:nil")
	} else {
		writeHashString(h, string(receipt.Governance.Kind))
		writeHashString(h, receipt.Governance.SubjectRef)
		writeHashString(h, receipt.Governance.Reason)
		writeHashString(h, receipt.Governance.RequestedAction)
		writeHashString(h, receipt.Governance.RiskLevel)
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
	if receipt.CapabilityStateStale == nil {
		writeHashString(h, "capability-state-stale:nil")
	} else {
		detail := receipt.CapabilityStateStale
		writeHashString(h, detail.StateRef.OwnerModuleID)
		writeHashString(h, detail.StateRef.CapabilityStateKind)
		writeHashString(h, detail.StateRef.SubjectRef)
		writeHashString(h, detail.SnapshotID)
		writeHashString(h, detail.ExpectedHash)
		writeHashString(h, detail.ObservedRevision)
		writeHashString(h, detail.ObservedHash)
		writeHashString(h, detail.Reason)
	}
	writeHashString(h, fmt.Sprintf("%t", receipt.IdempotentReplay))
	return hex.EncodeToString(h.Sum(nil))
}

// EvidenceKindFromReceipt derives committed evidence kind from sealed action expectation
// and owner receipt. Decision action alone is never sufficient (INV-03).
func EvidenceKindFromReceipt(action RuntimeAction, expected RuntimeEffectScope, receipt RuntimeOperationReceipt) (string, error) {
	receipt = receipt.Normalized()
	expected = NormalizeRuntimeEffectScope(expected)
	if err := receipt.Validate(); err != nil {
		return "", err
	}
	switch receipt.Outcome {
	case RuntimeOperationOutcomeRejected:
		// rejected still forms evidence of the expected kind with failure facts
		return evidenceKindForAction(action), nil
	case RuntimeOperationOutcomeGovernanceRequired:
		return "", fmt.Errorf("governance_required receipt cannot commit ordinary evidence")
	case RuntimeOperationOutcomeUnknown:
		return "", fmt.Errorf("unknown outcome receipt cannot commit evidence")
	case RuntimeOperationOutcomeNoOp:
		// no-op may be diagnostic/validation observation only
		if action == RuntimeActionProduceValidation && RuntimeEffectIsNonDurable(receipt.ObservedEffectScope) {
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
		// validation cannot accept durable observed effect
		if action == RuntimeActionProduceValidation && RuntimeEffectIsDurable(receipt.ObservedEffectScope) {
			return "", fmt.Errorf("validation action cannot commit durable observed effect %q", receipt.ObservedEffectScope)
		}
		// ephemeral cannot close durable state-change requirement
		if action == RuntimeActionProduceStateChange && !RuntimeEffectIsDurable(receipt.ObservedEffectScope) {
			return "", fmt.Errorf("state-change action cannot commit non-durable observed effect %q", receipt.ObservedEffectScope)
		}
		return evidenceKindForAction(action), nil
	default:
		return "", fmt.Errorf("unsupported receipt outcome %q", receipt.Outcome)
	}
}

func evidenceKindForAction(action RuntimeAction) string {
	switch action {
	case RuntimeActionProduceStateChange:
		return EvidenceKindStateChange
	case RuntimeActionProduceValidation:
		return EvidenceKindValidation
	default:
		return ""
	}
}
