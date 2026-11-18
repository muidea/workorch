package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strconv"
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
	// Original owner-authored read snapshot, not a currentness assertion or
	// a replacement for the requirement's aggregate authority set.
	OwnerRevisionSet          *RuntimeOwnerRevisionSet                  `json:",omitempty"`
	ApplicabilityDependencies *RuntimeEvidenceApplicabilityDependencies `json:",omitempty"`
}

type RuntimeSourceObligationOutcome struct {
	Check                RuntimeSourceObligationCheck
	CoverageState        RuntimeRequirementCoverageState
	AssessmentState      RuntimeRequirementAssessmentState
	AssessmentReasonCode string
	AcceptedEvidenceRefs []string
	CurrentBindingRef    *RuntimeCWIBindingRef
}

// 源义务结论只投影同一 SM-2 CWI 的独立 all_of 检查，不形成第二份闭包账本。
func (o RuntimeSourceObligationOutcome) Normalized() RuntimeSourceObligationOutcome {
	o.Check.ProjectDesigns = slices.Clone(o.Check.ProjectDesigns)
	o.AcceptedEvidenceRefs = normalizeStrings(o.AcceptedEvidenceRefs)
	if o.CurrentBindingRef != nil {
		ref := o.CurrentBindingRef.Clone()
		o.CurrentBindingRef = &ref
	}
	return o
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
	SourceObligations    []RuntimeSourceObligationOutcome `json:",omitempty"`
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
	sort.Slice(s.Entries, func(i, j int) bool { return ownerRevisionRefLess(s.Entries[i], s.Entries[j]) })
	s.SetHash = strings.TrimSpace(s.SetHash)
	return s
}

func isRuntimeOwnerRevisionSetRawCanonical(s RuntimeOwnerRevisionSet) bool {
	if s.DefinitionRevision != strings.TrimSpace(s.DefinitionRevision) ||
		s.SetHash != strings.TrimSpace(s.SetHash) {
		return false
	}
	if s.Entries != nil && len(s.Entries) == 0 {
		return false
	}
	for idx, entry := range s.Entries {
		if entry.OwnerKind != strings.TrimSpace(entry.OwnerKind) ||
			entry.OwnerInstanceID != strings.TrimSpace(entry.OwnerInstanceID) ||
			entry.ResourceKind != strings.TrimSpace(entry.ResourceKind) ||
			entry.ResourceID != strings.TrimSpace(entry.ResourceID) ||
			entry.SnapshotID != strings.TrimSpace(entry.SnapshotID) ||
			entry.Revision != strings.TrimSpace(entry.Revision) ||
			entry.DependencyHash != strings.TrimSpace(entry.DependencyHash) {
			return false
		}
		if idx > 0 && ownerRevisionRefLess(entry, s.Entries[idx-1]) {
			return false
		}
	}
	return true
}

func computeNormalizedRuntimeOwnerRevisionSetHash(s RuntimeOwnerRevisionSet) string {
	if payload, ok := appendRuntimeOwnerRevisionSetPayloadJSON(nil, s); ok {
		sum := sha256.New()
		_, _ = sum.Write([]byte("runtime-owner-revision-set\x00"))
		_, _ = sum.Write(payload)
		return "runtime-owner-revision-set-sha256:" + hex.EncodeToString(sum.Sum(nil))
	}
	return stableFormalValueHash("runtime-owner-revision-set", struct {
		DefinitionRevision string
		Entries            []RuntimeOwnerRevisionRef
	}{s.DefinitionRevision, s.Entries})
}

func (s RuntimeOwnerRevisionSet) WithHash() RuntimeOwnerRevisionSet {
	s = s.Normalized()
	s.SetHash = computeNormalizedRuntimeOwnerRevisionSetHash(s)
	return s
}

func appendSafeJSONStringField(buf []byte, prefix, value string) ([]byte, bool) {
	buf = append(buf, prefix...)
	return appendSafeJSONString(buf, value)
}

func appendRuntimeOwnerRevisionSetPayloadJSON(buf []byte, s RuntimeOwnerRevisionSet) ([]byte, bool) {
	if cap(buf) == 0 {
		buf = make([]byte, 0, 64+len(s.Entries)*192)
	}
	var ok bool
	if buf, ok = appendSafeJSONStringField(buf, `{"DefinitionRevision":`, s.DefinitionRevision); !ok {
		return nil, false
	}
	buf = append(buf, `,"Entries":`...)
	if s.Entries == nil {
		buf = append(buf, "null}"...)
		return buf, true
	}
	buf = append(buf, '[')
	for idx, entry := range s.Entries {
		if idx > 0 {
			buf = append(buf, ',')
		}
		if buf, ok = appendSafeJSONStringField(buf, `{"OwnerKind":`, entry.OwnerKind); !ok {
			return nil, false
		}
		if buf, ok = appendSafeJSONStringField(buf, `,"OwnerInstanceID":`, entry.OwnerInstanceID); !ok {
			return nil, false
		}
		if buf, ok = appendSafeJSONStringField(buf, `,"ResourceKind":`, entry.ResourceKind); !ok {
			return nil, false
		}
		if buf, ok = appendSafeJSONStringField(buf, `,"ResourceID":`, entry.ResourceID); !ok {
			return nil, false
		}
		if buf, ok = appendSafeJSONStringField(buf, `,"SnapshotID":`, entry.SnapshotID); !ok {
			return nil, false
		}
		if buf, ok = appendSafeJSONStringField(buf, `,"Revision":`, entry.Revision); !ok {
			return nil, false
		}
		buf = append(buf, `,"Fence":`...)
		buf = strconv.AppendUint(buf, entry.Fence, 10)
		if buf, ok = appendSafeJSONStringField(buf, `,"DependencyHash":`, entry.DependencyHash); !ok {
			return nil, false
		}
		buf = append(buf, '}')
	}
	buf = append(buf, "]}"...)
	return buf, true
}

// CompileRuntimeOwnerRevisionSetUnion 只合并调用方提供的实际依赖，不补造 owner fact。
// 每份输入先反证原始 canonical 正文与 hash；跨集合的相同事实幂等，冲突版本拒绝。
func CompileRuntimeOwnerRevisionSetUnion(definitionRevision string, sets ...RuntimeOwnerRevisionSet) (RuntimeOwnerRevisionSet, error) {
	if definitionRevision == "" || definitionRevision != strings.TrimSpace(definitionRevision) {
		return RuntimeOwnerRevisionSet{}, fmt.Errorf("runtime owner revision union requires an exact definition revision")
	}
	entries := make(map[[4]string]RuntimeOwnerRevisionRef)
	for _, set := range sets {
		normalized := set.Normalized()
		if !equalRuntimeOwnerRevisionSet(&set, &normalized) {
			return RuntimeOwnerRevisionSet{}, fmt.Errorf("runtime owner revision union source is not raw canonical")
		}
		if err := set.Validate(); err != nil {
			return RuntimeOwnerRevisionSet{}, err
		}
		if set.DefinitionRevision != definitionRevision {
			return RuntimeOwnerRevisionSet{}, fmt.Errorf("runtime owner revision union definition mismatch")
		}
		for _, entry := range set.Entries {
			key := ownerRevisionResourceKey(entry)
			if previous, exists := entries[key]; exists && previous != entry {
				return RuntimeOwnerRevisionSet{}, fmt.Errorf("runtime owner revision union resource %q has conflicting dependencies", key)
			}
			entries[key] = entry
		}
	}
	result := RuntimeOwnerRevisionSet{DefinitionRevision: definitionRevision}
	for _, entry := range entries {
		result.Entries = append(result.Entries, entry)
	}
	return result.WithHash(), nil
}

func (s RuntimeOwnerRevisionSet) Validate() error {
	if !isRuntimeOwnerRevisionSetRawCanonical(s) {
		s = s.Normalized()
	}
	if s.DefinitionRevision == "" || s.SetHash == "" || s.SetHash != computeNormalizedRuntimeOwnerRevisionSetHash(s) {
		return fmt.Errorf("runtime owner revision set identity/hash is invalid")
	}
	for idx, entry := range s.Entries {
		key := ownerRevisionResourceKey(entry)
		if entry.OwnerKind == "" || entry.OwnerInstanceID == "" || entry.ResourceKind == "" || entry.ResourceID == "" || entry.DependencyHash == "" || (entry.SnapshotID == "" && entry.Revision == "" && entry.Fence == 0) {
			return fmt.Errorf("runtime owner revision entry is incomplete")
		}
		if idx > 0 && ownerRevisionResourceKey(s.Entries[idx-1]) == key {
			return fmt.Errorf("runtime owner revision resource %q is duplicated", key)
		}
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
		e.OwnerRevisionSet = CloneRuntimeOwnerRevisionSet(e.OwnerRevisionSet)
		e.ApplicabilityDependencies = CloneRuntimeEvidenceApplicabilityDependencies(e.ApplicabilityDependencies)
	}
	sort.Slice(o.EvidenceItems, func(i, j int) bool { return o.EvidenceItems[i].EvidenceRef < o.EvidenceItems[j].EvidenceRef })
	o.AcceptedEvidenceRefs = normalizeStrings(o.AcceptedEvidenceRefs)
	o.OwnerRevisionSetHash = strings.TrimSpace(o.OwnerRevisionSetHash)
	o.SourceObligations = append([]RuntimeSourceObligationOutcome(nil), o.SourceObligations...)
	for idx := range o.SourceObligations {
		o.SourceObligations[idx] = o.SourceObligations[idx].Normalized()
	}
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
		if err := ValidateRuntimeEvidenceDependencyPair(item.OwnerRevisionSet, item.ApplicabilityDependencies, o.DefinitionRevision); err != nil {
			return fmt.Errorf("runtime requirement evidence read dependencies: %w", err)
		}
		if item.OwnerRevisionSet != nil && item.OwnerRevisionSet.DefinitionRevision != o.DefinitionRevision {
			return fmt.Errorf("runtime requirement evidence read dependencies definition mismatch")
		}
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
	checks := make([]RuntimeSourceObligationCheck, 0, len(o.SourceObligations))
	for _, source := range o.SourceObligations {
		checks = append(checks, source.Check)
		switch source.CoverageState {
		case RuntimeRequirementUncovered, RuntimeRequirementPartiallyCovered, RuntimeRequirementCovered:
		default:
			return fmt.Errorf("runtime source obligation has invalid coverage state")
		}
		switch source.AssessmentState {
		case RuntimeRequirementUnassessed, RuntimeRequirementSatisfied, RuntimeRequirementUnsatisfied:
		case RuntimeRequirementIndeterminate:
			if source.AssessmentReasonCode == "" {
				return fmt.Errorf("indeterminate source obligation requires reason")
			}
		default:
			return fmt.Errorf("runtime source obligation has invalid assessment state")
		}
		for _, ref := range source.AcceptedEvidenceRefs {
			if !containsString(o.AcceptedEvidenceRefs, ref) {
				return fmt.Errorf("runtime source obligation evidence is outside requirement credit")
			}
		}
		if source.CurrentBindingRef != nil {
			if err := source.CurrentBindingRef.Validate(); err != nil {
				return err
			}
		}
		if source.CoverageState == RuntimeRequirementCovered || source.AssessmentState == RuntimeRequirementSatisfied {
			if o.CoverageState != RuntimeRequirementCovered || o.AssessmentState != RuntimeRequirementSatisfied || source.CoverageState != RuntimeRequirementCovered || source.AssessmentState != RuntimeRequirementSatisfied || source.CurrentBindingRef == nil || len(source.AcceptedEvidenceRefs) == 0 {
				return fmt.Errorf("runtime source obligation cannot restore absent current CWI credit")
			}
		}
	}
	if err := ValidateRuntimeSourceObligationChecks(o.DefinitionRevision, o.CoverageWorkItemID, checks); err != nil {
		return err
	}
	return nil
}

func ownerRevisionResourceKey(e RuntimeOwnerRevisionRef) [4]string {
	return [4]string{e.OwnerKind, e.OwnerInstanceID, e.ResourceKind, e.ResourceID}
}

func ownerRevisionRefLess(left, right RuntimeOwnerRevisionRef) bool {
	leftFields := [...]string{left.OwnerKind, left.OwnerInstanceID, left.ResourceKind, left.ResourceID, left.SnapshotID, left.Revision}
	rightFields := [...]string{right.OwnerKind, right.OwnerInstanceID, right.ResourceKind, right.ResourceID, right.SnapshotID, right.Revision}
	for idx, value := range leftFields {
		if value != rightFields[idx] {
			return value < rightFields[idx]
		}
	}
	return left.Fence < right.Fence
}

func stableFormalValueHash(kind string, value any) string {
	// Encoder retains Marshal's exact representation without its full output
	// copy and the second domain+payload copy. The call-local writer withholds
	// only Encoder's trailing newline; original field bytes are never changed.
	w := runtimeJSONHashWriter{digest: sha256.New()}
	_, _ = w.digest.Write([]byte(kind + "\x00"))
	if json.NewEncoder(&w).Encode(value) != nil {
		// Preserve the original Marshal-error hash (nil payload), too.
		w.digest.Reset()
		_, _ = w.digest.Write([]byte(kind + "\x00"))
	}
	return kind + "-sha256:" + hex.EncodeToString(w.digest.Sum(nil))
}
