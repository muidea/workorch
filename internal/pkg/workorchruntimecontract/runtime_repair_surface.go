package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeValidationFailureObservation is the bounded fact emitted by a
// validation capability.  CandidateTargetRefs are deliberately only a
// candidate repair surface: they do not grant an operation or a Runtime
// action.  A resource owner must confirm them before SM-2 may use them.
type RuntimeValidationFailureObservation struct {
	WorkspaceID               common.WorkspaceID
	RunID                     common.RunID
	AuthoritySnapshotID       string
	DispatchID                string
	OperationID               string
	FailureSignature          string
	TargetRole                string
	DiagnosticClass           string
	CandidateTargetRefs       []string
	ValidationScopeTargetRefs []string
	DiagnosticHash            string
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
	if o.WorkspaceID == "" || o.RunID == "" || o.AuthoritySnapshotID == "" || o.DispatchID == "" || o.OperationID == "" || o.FailureSignature == "" || o.TargetRole == "" || o.DiagnosticClass == "" || len(o.CandidateTargetRefs) == 0 || len(o.ValidationScopeTargetRefs) == 0 || o.DiagnosticHash == "" {
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

// RuntimeRepairSurfaceConfirmation is the File owner's read-only reply. It
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
	c = c.Normalized()
	if err := c.Observation.Validate(); err != nil {
		return err
	}
	if c.OwnerModuleID == "" || c.SubjectRef == "" || c.Disposition == "" || c.ObservedRevision == "" {
		return fmt.Errorf("repair surface confirmation requires owner, subject, disposition and revision")
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
