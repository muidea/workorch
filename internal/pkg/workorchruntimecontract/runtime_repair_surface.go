package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

const RuntimeValidationFailureObservationMaxCandidateTargetRefs = 8

// RuntimeValidationFailureObservation is the bounded fact emitted by a
// validation capability.  CandidateTargetRefs are deliberately only a
// candidate repair surface: they do not grant an operation or a Runtime
// action.  A resource owner must confirm them before SM-2 may use them.
type RuntimeValidationFailureObservation struct {
	WorkspaceID               common.WorkspaceID `json:"workspaceId"`
	RunID                     common.RunID       `json:"runId"`
	AuthoritySnapshotID       string             `json:"authoritySnapshotId"`
	DispatchID                string             `json:"dispatchId"`
	OperationID               string             `json:"operationId"`
	FailureSignature          string             `json:"failureSignature"`
	TargetRole                string             `json:"targetRole"`
	DiagnosticClass           string             `json:"diagnosticClass"`
	CandidateTargetRefs       []string           `json:"candidateTargetRefs"`
	ValidationScopeTargetRefs []string           `json:"validationScopeTargetRefs"`
	DiagnosticHash            string             `json:"diagnosticHash"`
}

func (o RuntimeValidationFailureObservation) Normalized() RuntimeValidationFailureObservation {
	o.AuthoritySnapshotID = strings.TrimSpace(o.AuthoritySnapshotID)
	o.DispatchID = strings.TrimSpace(o.DispatchID)
	o.OperationID = strings.TrimSpace(o.OperationID)
	o.FailureSignature = strings.TrimSpace(o.FailureSignature)
	o.TargetRole = strings.TrimSpace(o.TargetRole)
	o.DiagnosticClass = strings.TrimSpace(o.DiagnosticClass)
	o.CandidateTargetRefs = normalizeStrings(o.CandidateTargetRefs)
	o.ValidationScopeTargetRefs = normalizeStrings(o.ValidationScopeTargetRefs)
	o.DiagnosticHash = strings.TrimSpace(o.DiagnosticHash)
	return o
}

func (o RuntimeValidationFailureObservation) Validate() error {
	o = o.Normalized()
	if o.WorkspaceID == "" || o.RunID == "" || o.AuthoritySnapshotID == "" || o.DispatchID == "" || o.OperationID == "" || o.FailureSignature == "" || o.TargetRole == "" || o.DiagnosticClass == "" || len(o.CandidateTargetRefs) == 0 || len(o.CandidateTargetRefs) > RuntimeValidationFailureObservationMaxCandidateTargetRefs || len(o.ValidationScopeTargetRefs) == 0 || o.DiagnosticHash == "" {
		return fmt.Errorf("validation failure observation requires scope, causation, role, diagnostic and bounded targets")
	}
	if !runtimeTargetRefsSubset(o.CandidateTargetRefs, o.ValidationScopeTargetRefs) {
		return fmt.Errorf("validation failure observation candidates must be within sealed validation scope")
	}
	if len(o.DiagnosticHash) != sha256.Size*2 {
		return fmt.Errorf("validation failure observation diagnostic hash must be sha256")
	}
	if _, err := hex.DecodeString(o.DiagnosticHash); err != nil {
		return fmt.Errorf("validation failure observation diagnostic hash must be hex: %w", err)
	}
	return nil
}

func (o RuntimeValidationFailureObservation) Clone() RuntimeValidationFailureObservation {
	o = o.Normalized()
	o.CandidateTargetRefs = append([]string(nil), o.CandidateTargetRefs...)
	o.ValidationScopeTargetRefs = append([]string(nil), o.ValidationScopeTargetRefs...)
	return o
}

func SameRuntimeValidationFailureObservation(left, right RuntimeValidationFailureObservation) bool {
	l, r := left.Normalized(), right.Normalized()
	return l.WorkspaceID == r.WorkspaceID && l.RunID == r.RunID && l.AuthoritySnapshotID == r.AuthoritySnapshotID &&
		l.DispatchID == r.DispatchID && l.OperationID == r.OperationID && l.FailureSignature == r.FailureSignature &&
		l.TargetRole == r.TargetRole && l.DiagnosticClass == r.DiagnosticClass && l.DiagnosticHash == r.DiagnosticHash &&
		sameStrings(l.CandidateTargetRefs, r.CandidateTargetRefs) && sameStrings(l.ValidationScopeTargetRefs, r.ValidationScopeTargetRefs)
}

type RuntimeRepairSurfaceDisposition string

const (
	RuntimeRepairSurfaceConfirmed RuntimeRepairSurfaceDisposition = "confirmed"
	RuntimeRepairSurfaceStale     RuntimeRepairSurfaceDisposition = "stale"
	RuntimeRepairSurfaceRejected  RuntimeRepairSurfaceDisposition = "rejected"
)

// RuntimeRepairSurfaceConfirmation is the resource owner's read-only reply. It
// contains no action or retry instruction; Orchestrator alone maps a confirmed
// fact to a new authority transaction.
type RuntimeRepairSurfaceConfirmation struct {
	Observation         RuntimeValidationFailureObservation
	OwnerModuleID       string
	SubjectRef          string
	Disposition         RuntimeRepairSurfaceDisposition
	ConfirmedTargetRefs []string
	ObservedRevision    string
	Reason              string
}

// RuntimeRepairSurfaceConfirmationRecord is the Orchestrator-owned replay
// fence for one resource-owner confirmation.  The complete normalized
// confirmation is retained so a duplicate can be recognized without relying
// on an event-id index or reconstructing the owner fact from mutable state.
// IdentitySignature deliberately excludes response-specific disposition,
// confirmed targets and reason so a conflicting response for the same owner
// observation is detected; Signature is the exact response identity.
type RuntimeRepairSurfaceConfirmationRecord struct {
	Confirmation      RuntimeRepairSurfaceConfirmation `json:"confirmation"`
	Signature         string                           `json:"signature"`
	IdentitySignature string                           `json:"identitySignature"`
}

func (r RuntimeRepairSurfaceConfirmationRecord) Normalized() RuntimeRepairSurfaceConfirmationRecord {
	r.Confirmation = r.Confirmation.Normalized()
	r.Signature = strings.TrimSpace(r.Signature)
	r.IdentitySignature = strings.TrimSpace(r.IdentitySignature)
	return r
}

func (r RuntimeRepairSurfaceConfirmationRecord) Validate() error {
	r = r.Normalized()
	if err := r.Confirmation.Validate(); err != nil {
		return fmt.Errorf("repair surface confirmation record: %w", err)
	}
	if r.Signature == "" || r.IdentitySignature == "" {
		return fmt.Errorf("repair surface confirmation record requires exact and identity signatures")
	}
	if r.Signature != StableRuntimeRepairSurfaceConfirmationHash(r.Confirmation) {
		return fmt.Errorf("repair surface confirmation record signature mismatch")
	}
	if r.IdentitySignature != StableRuntimeRepairSurfaceConfirmationIdentityHash(r.Confirmation) {
		return fmt.Errorf("repair surface confirmation record identity signature mismatch")
	}
	return nil
}

func (r RuntimeRepairSurfaceConfirmationRecord) Clone() RuntimeRepairSurfaceConfirmationRecord {
	r = r.Normalized()
	r.Confirmation = r.Confirmation.Clone()
	return r
}

// ValidateRuntimeRepairSurfaceConfirmationRecords validates the bounded
// owner-local replay history and rejects both exact duplicates and conflicting
// responses for one observation. The maps are scoped to this aggregate value;
// no process-wide event/confirmation index is introduced.
func ValidateRuntimeRepairSurfaceConfirmationRecords(values []RuntimeRepairSurfaceConfirmationRecord) error {
	exact := make(map[string]struct{}, len(values))
	identity := make(map[string]struct{}, len(values))
	sequenceSubjects := make(map[string]string, len(values))
	terminalSequence := make(map[string]int, len(values))
	for idx, record := range values {
		if err := record.Validate(); err != nil {
			return fmt.Errorf("runtime repair surface confirmation %d: %w", idx, err)
		}
		record = record.Normalized()
		if _, duplicate := exact[record.Signature]; duplicate {
			return fmt.Errorf("runtime repair surface confirmations duplicate exact response %q", record.Signature)
		}
		if _, duplicate := identity[record.IdentitySignature]; duplicate {
			return fmt.Errorf("runtime repair surface confirmations conflict for owner observation %q", record.IdentitySignature)
		}
		sequenceKey := runtimeRepairSurfaceObservationSequenceKey(record.Confirmation.Observation)
		if subject, found := sequenceSubjects[sequenceKey]; found && subject != record.Confirmation.SubjectRef {
			return fmt.Errorf("runtime repair surface confirmation %d changes subject within one validation observation", idx)
		}
		if terminalAt, found := terminalSequence[sequenceKey]; found {
			return fmt.Errorf("runtime repair surface confirmation %d follows terminal response %d for one validation observation", idx, terminalAt)
		}
		sequenceSubjects[sequenceKey] = record.Confirmation.SubjectRef
		if record.Confirmation.Disposition == RuntimeRepairSurfaceConfirmed || record.Confirmation.Disposition == RuntimeRepairSurfaceRejected {
			terminalSequence[sequenceKey] = idx
		}
		exact[record.Signature] = struct{}{}
		identity[record.IdentitySignature] = struct{}{}
	}
	return nil
}

func runtimeRepairSurfaceObservationSequenceKey(value RuntimeValidationFailureObservation) string {
	encoded, _ := json.Marshal(value.Normalized())
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

// StableRuntimeRepairSurfaceConfirmationHash returns the exact confirmation
// identity.  JSON is used only for this local persisted hash; the value never
// crosses an EventHub boundary as an untyped payload.
func StableRuntimeRepairSurfaceConfirmationHash(value RuntimeRepairSurfaceConfirmation) string {
	value = value.Normalized()
	encoded, _ := json.Marshal(value)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

// StableRuntimeRepairSurfaceConfirmationIdentityHash identifies the owner
// observation/revision and response context while intentionally omitting the
// response-specific disposition, confirmed target list and reason. A
// different response for the same observation is therefore a conflict, while
// a new owner revision is a new observation that can be processed normally.
func StableRuntimeRepairSurfaceConfirmationIdentityHash(value RuntimeRepairSurfaceConfirmation) string {
	value = value.Normalized()
	identity := struct {
		Observation      RuntimeValidationFailureObservation `json:"observation"`
		OwnerModuleID    string                              `json:"ownerModuleId"`
		SubjectRef       string                              `json:"subjectRef"`
		ObservedRevision string                              `json:"observedRevision"`
	}{
		Observation: value.Observation, OwnerModuleID: value.OwnerModuleID,
		SubjectRef: value.SubjectRef, ObservedRevision: value.ObservedRevision,
	}
	encoded, _ := json.Marshal(identity)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func NewRuntimeRepairSurfaceConfirmationRecord(value RuntimeRepairSurfaceConfirmation) (RuntimeRepairSurfaceConfirmationRecord, error) {
	value = value.Normalized()
	if err := value.Validate(); err != nil {
		return RuntimeRepairSurfaceConfirmationRecord{}, err
	}
	return RuntimeRepairSurfaceConfirmationRecord{
		Confirmation: value, Signature: StableRuntimeRepairSurfaceConfirmationHash(value),
		IdentitySignature: StableRuntimeRepairSurfaceConfirmationIdentityHash(value),
	}, nil
}

// SameRuntimeRepairSurfaceConfirmation compares the full normalized owner
// response, including disposition and any reason/target facts.
func SameRuntimeRepairSurfaceConfirmation(left, right RuntimeRepairSurfaceConfirmation) bool {
	return StableRuntimeRepairSurfaceConfirmationHash(left) == StableRuntimeRepairSurfaceConfirmationHash(right)
}

// SameRuntimeRepairSurfaceConfirmationIdentity compares the replay scope while
// ignoring only the response disposition and confirmed targets.
func SameRuntimeRepairSurfaceConfirmationIdentity(left, right RuntimeRepairSurfaceConfirmation) bool {
	return StableRuntimeRepairSurfaceConfirmationIdentityHash(left) == StableRuntimeRepairSurfaceConfirmationIdentityHash(right)
}

func (c RuntimeRepairSurfaceConfirmation) Normalized() RuntimeRepairSurfaceConfirmation {
	c.Observation = c.Observation.Normalized()
	c.OwnerModuleID = strings.TrimSpace(c.OwnerModuleID)
	c.SubjectRef = strings.TrimSpace(c.SubjectRef)
	c.Disposition = RuntimeRepairSurfaceDisposition(strings.TrimSpace(string(c.Disposition)))
	c.ConfirmedTargetRefs = normalizeStrings(c.ConfirmedTargetRefs)
	c.ObservedRevision = strings.TrimSpace(c.ObservedRevision)
	c.Reason = strings.TrimSpace(c.Reason)
	return c
}

func (c RuntimeRepairSurfaceConfirmation) Validate() error {
	rawOwnerModuleID := c.OwnerModuleID
	c = c.Normalized()
	if err := c.Observation.Validate(); err != nil {
		return err
	}
	if c.OwnerModuleID == "" || rawOwnerModuleID != c.OwnerModuleID || c.SubjectRef == "" || c.Disposition == "" || c.ObservedRevision == "" {
		return fmt.Errorf("repair surface confirmation requires canonical owner, subject, disposition and revision")
	}
	switch c.Disposition {
	case RuntimeRepairSurfaceConfirmed:
		if len(c.ConfirmedTargetRefs) == 0 || !sameStrings(c.ConfirmedTargetRefs, c.Observation.CandidateTargetRefs) {
			return fmt.Errorf("confirmed repair surface must exactly bind validation candidates")
		}
	case RuntimeRepairSurfaceStale, RuntimeRepairSurfaceRejected:
		if len(c.ConfirmedTargetRefs) != 0 {
			return fmt.Errorf("non-confirmed repair surface cannot contain targets")
		}
	default:
		return fmt.Errorf("unknown repair surface disposition %q", c.Disposition)
	}
	return nil
}

func (c RuntimeRepairSurfaceConfirmation) Clone() RuntimeRepairSurfaceConfirmation {
	c = c.Normalized()
	c.Observation = c.Observation.Clone()
	c.ConfirmedTargetRefs = append([]string(nil), c.ConfirmedTargetRefs...)
	return c
}

func runtimeTargetRefsSubset(values, scope []string) bool {
	allowed := make(map[string]struct{}, len(scope))
	for _, value := range normalizeStrings(scope) {
		allowed[value] = struct{}{}
	}
	for _, value := range normalizeStrings(values) {
		if _, ok := allowed[value]; !ok {
			return false
		}
	}
	return true
}
