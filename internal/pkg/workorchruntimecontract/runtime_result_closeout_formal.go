package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

type RuntimeRequirementCoverageState string
type RuntimeRequirementAssessmentState string
type RuntimeRequirementEvidenceState string

const (
	RuntimeRequirementUncovered        RuntimeRequirementCoverageState = "uncovered"
	RuntimeRequirementPartiallyCovered RuntimeRequirementCoverageState = "partially_covered"
	RuntimeRequirementCovered          RuntimeRequirementCoverageState = "covered"

	RuntimeRequirementUnassessed    RuntimeRequirementAssessmentState = "unassessed"
	RuntimeRequirementSatisfied     RuntimeRequirementAssessmentState = "satisfied"
	RuntimeRequirementUnsatisfied   RuntimeRequirementAssessmentState = "unsatisfied"
	RuntimeRequirementIndeterminate RuntimeRequirementAssessmentState = "indeterminate"

	RuntimeRequirementEvidenceCandidate         RuntimeRequirementEvidenceState = "candidate"
	RuntimeRequirementEvidenceAccepted          RuntimeRequirementEvidenceState = "accepted"
	RuntimeRequirementEvidenceRejected          RuntimeRequirementEvidenceState = "rejected"
	RuntimeRequirementEvidenceStaleOrSuperseded RuntimeRequirementEvidenceState = "stale_or_superseded"
)

type RuntimeOwnerRevisionRef struct {
	OwnerKind       string
	OwnerInstanceID string
	ResourceKind    string
	ResourceID      string
	SnapshotID      string
	Revision        string
	Fence           uint64
	DependencyHash  string
}

type RuntimeOwnerRevisionSet struct {
	DefinitionRevision string
	Entries            []RuntimeOwnerRevisionRef
	SetHash            string
}

type RuntimeRequirementEvidenceItem struct {
	EvidenceRef          string
	BindingRef           string
	State                RuntimeRequirementEvidenceState
	ReasonCode           string
	OwnerRevisionSetHash string
}

type RuntimeRequirementOutcome struct {
	RequirementRef       string
	SourceItemRef        string
	CoverageWorkItemID   common.CoverageWorkItemID
	DefinitionRevision   string
	CoverageState        RuntimeRequirementCoverageState
	AssessmentState      RuntimeRequirementAssessmentState
	AssessmentReasonCode string
	MissingEvidenceKinds []string
	EvidenceItems        []RuntimeRequirementEvidenceItem
	AcceptedEvidenceRefs []string
	OwnerRevisionSetHash string
}

func (s RuntimeOwnerRevisionSet) Normalized() RuntimeOwnerRevisionSet {
	s.DefinitionRevision = strings.TrimSpace(s.DefinitionRevision)
	s.Entries = append([]RuntimeOwnerRevisionRef(nil), s.Entries...)
	for idx := range s.Entries {
		e := &s.Entries[idx]
		e.OwnerKind = strings.TrimSpace(e.OwnerKind)
		e.OwnerInstanceID = strings.TrimSpace(e.OwnerInstanceID)
		e.ResourceKind = strings.TrimSpace(e.ResourceKind)
		e.ResourceID = strings.TrimSpace(e.ResourceID)
		e.SnapshotID = strings.TrimSpace(e.SnapshotID)
		e.Revision = strings.TrimSpace(e.Revision)
		e.DependencyHash = strings.TrimSpace(e.DependencyHash)
	}
	sort.Slice(s.Entries, func(i, j int) bool { return ownerRevisionRefKey(s.Entries[i]) < ownerRevisionRefKey(s.Entries[j]) })
	s.SetHash = strings.TrimSpace(s.SetHash)
	return s
}

func (s RuntimeOwnerRevisionSet) WithHash() RuntimeOwnerRevisionSet {
	s = s.Normalized()
	s.SetHash = stableFormalValueHash("runtime-owner-revision-set", struct {
		DefinitionRevision string
		Entries            []RuntimeOwnerRevisionRef
	}{s.DefinitionRevision, s.Entries})
	return s
}

func (s RuntimeOwnerRevisionSet) Validate() error {
	s = s.Normalized()
	if s.DefinitionRevision == "" || s.SetHash == "" || s.SetHash != s.WithHash().SetHash {
		return fmt.Errorf("runtime owner revision set identity/hash is invalid")
	}
	seen := map[string]struct{}{}
	for _, entry := range s.Entries {
		key := strings.Join([]string{entry.OwnerKind, entry.OwnerInstanceID, entry.ResourceKind, entry.ResourceID}, "\x00")
		if entry.OwnerKind == "" || entry.OwnerInstanceID == "" || entry.ResourceKind == "" || entry.ResourceID == "" || entry.DependencyHash == "" || (entry.SnapshotID == "" && entry.Revision == "" && entry.Fence == 0) {
			return fmt.Errorf("runtime owner revision entry is incomplete")
		}
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("runtime owner revision resource %q is duplicated", key)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func (o RuntimeRequirementOutcome) Normalized() RuntimeRequirementOutcome {
	o.RequirementRef = strings.TrimSpace(o.RequirementRef)
	o.SourceItemRef = strings.TrimSpace(o.SourceItemRef)
	o.CoverageWorkItemID = common.CoverageWorkItemID(strings.TrimSpace(string(o.CoverageWorkItemID)))
	o.DefinitionRevision = strings.TrimSpace(o.DefinitionRevision)
	o.CoverageState = RuntimeRequirementCoverageState(strings.TrimSpace(string(o.CoverageState)))
	o.AssessmentState = RuntimeRequirementAssessmentState(strings.TrimSpace(string(o.AssessmentState)))
	o.AssessmentReasonCode = strings.TrimSpace(o.AssessmentReasonCode)
	o.MissingEvidenceKinds = normalizeStrings(o.MissingEvidenceKinds)
	o.EvidenceItems = append([]RuntimeRequirementEvidenceItem(nil), o.EvidenceItems...)
	for idx := range o.EvidenceItems {
		e := &o.EvidenceItems[idx]
		e.EvidenceRef = strings.TrimSpace(e.EvidenceRef)
		e.BindingRef = strings.TrimSpace(e.BindingRef)
		e.State = RuntimeRequirementEvidenceState(strings.TrimSpace(string(e.State)))
		e.ReasonCode = strings.TrimSpace(e.ReasonCode)
		e.OwnerRevisionSetHash = strings.TrimSpace(e.OwnerRevisionSetHash)
	}
	sort.Slice(o.EvidenceItems, func(i, j int) bool { return o.EvidenceItems[i].EvidenceRef < o.EvidenceItems[j].EvidenceRef })
	o.AcceptedEvidenceRefs = normalizeStrings(o.AcceptedEvidenceRefs)
	o.OwnerRevisionSetHash = strings.TrimSpace(o.OwnerRevisionSetHash)
	return o
}

func (o RuntimeRequirementOutcome) Validate() error {
	o = o.Normalized()
	if o.RequirementRef == "" || o.SourceItemRef == "" || o.CoverageWorkItemID == "" || o.DefinitionRevision == "" {
		return fmt.Errorf("runtime requirement outcome identity is incomplete")
	}
	switch o.CoverageState {
	case RuntimeRequirementUncovered, RuntimeRequirementPartiallyCovered, RuntimeRequirementCovered:
	default:
		return fmt.Errorf("runtime requirement coverage state %q is invalid", o.CoverageState)
	}
	switch o.AssessmentState {
	case RuntimeRequirementUnassessed, RuntimeRequirementSatisfied, RuntimeRequirementUnsatisfied:
	case RuntimeRequirementIndeterminate:
		if o.AssessmentReasonCode == "" || len(o.MissingEvidenceKinds) == 0 {
			return fmt.Errorf("indeterminate requirement assessment requires reason and missing evidence")
		}
	default:
		return fmt.Errorf("runtime requirement assessment state %q is invalid", o.AssessmentState)
	}
	accepted := make([]string, 0)
	seen := map[string]struct{}{}
	for _, item := range o.EvidenceItems {
		if item.EvidenceRef == "" {
			return fmt.Errorf("runtime requirement evidence item requires identity")
		}
		if _, duplicate := seen[item.EvidenceRef]; duplicate {
			return fmt.Errorf("runtime requirement evidence %q is duplicated", item.EvidenceRef)
		}
		seen[item.EvidenceRef] = struct{}{}
		switch item.State {
		case RuntimeRequirementEvidenceCandidate, RuntimeRequirementEvidenceRejected, RuntimeRequirementEvidenceStaleOrSuperseded:
		case RuntimeRequirementEvidenceAccepted:
			if item.BindingRef == "" || item.OwnerRevisionSetHash == "" || item.OwnerRevisionSetHash != o.OwnerRevisionSetHash {
				return fmt.Errorf("accepted requirement evidence requires binding and the exact outcome owner revision set")
			}
			accepted = append(accepted, item.EvidenceRef)
		default:
			return fmt.Errorf("runtime requirement evidence state %q is invalid", item.State)
		}
	}
	if strings.Join(normalizeStrings(accepted), "\x00") != strings.Join(o.AcceptedEvidenceRefs, "\x00") {
		return fmt.Errorf("runtime requirement accepted evidence union mismatch")
	}
	return nil
}

func ownerRevisionRefKey(e RuntimeOwnerRevisionRef) string {
	return strings.Join([]string{e.OwnerKind, e.OwnerInstanceID, e.ResourceKind, e.ResourceID, e.SnapshotID, e.Revision, fmt.Sprintf("%020d", e.Fence)}, "\x00")
}

func stableFormalValueHash(kind string, value any) string {
	payload, _ := json.Marshal(value)
	digest := sha256.Sum256(append([]byte(kind+"\x00"), payload...))
	return kind + "-sha256:" + hex.EncodeToString(digest[:])
}
