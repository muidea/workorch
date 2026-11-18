package workorchruntimecontract

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"strings"
	"sync"

	"github.com/muidea/workorch/internal/pkg/common"
)

type runtimeStateHashCacheSlot struct {
	mu        sync.Mutex
	wsID      common.WorkspaceID
	runID     common.RunID
	state     *RuntimeStateRecord
	raw       []byte
	prevAlloc []byte
	spans     *RuntimeStateJSONSpans
	hash      string
}

var runtimeStateHashCache [8]runtimeStateHashCacheSlot

func runtimeStateHashSlotIndex(wsID common.WorkspaceID, runID common.RunID) int {
	var h uint32 = 2166136261
	for i := 0; i < len(wsID); i++ {
		h = (h ^ uint32(wsID[i])) * 16777619
	}
	h = (h ^ 0xff) * 16777619
	for i := 0; i < len(runID); i++ {
		h = (h ^ uint32(runID[i])) * 16777619
	}
	return int(h & 7)
}

func StableRuntimeStateRecordHash(state RuntimeStateRecord) string {
	wsID, runID := state.Facts.WorkspaceID, state.Facts.RunID
	slot := &runtimeStateHashCache[runtimeStateHashSlotIndex(wsID, runID)]
	slot.mu.Lock()
	defer slot.mu.Unlock()

	if slot.wsID == wsID && slot.runID == runID && slot.state != nil && slot.hash != "" && runtimeRawEqual(&state, slot.state) {
		return slot.hash
	}

	var prev *RuntimeStateRecord
	var prevRaw []byte
	var prevSpans *RuntimeStateJSONSpans
	if slot.wsID == wsID && slot.runID == runID && slot.state != nil && len(slot.raw) > 0 && slot.spans != nil {
		prev, prevRaw, prevSpans = slot.state, slot.raw, slot.spans
	}
	buf := bytes.NewBuffer(slot.prevAlloc[:0])
	if len(prevRaw) > 0 && buf.Cap() < len(prevRaw)+4096 {
		buf.Grow(len(prevRaw) + 4096 - buf.Cap())
	}
	spans, err := WriteRuntimeStateRecordJSONWithPreimage(buf, state, prev, prevRaw, prevSpans)
	if err != nil {
		return ""
	}
	raw := buf.Bytes()
	sum := SumRuntimeStateJSONWithPreimage(raw, prevRaw, prevSpans, &spans)
	hash := "runtime-state-" + hex.EncodeToString(sum[:])

	oldRaw := slot.raw
	detached := SnapshotRuntimeStateRecordWithSharedReadOnlyHistory(&state)
	slot.wsID = wsID
	slot.runID = runID
	slot.state = &detached
	slot.raw = raw
	slot.prevAlloc = nil
	if cap(oldRaw) > 0 && (len(raw) == 0 || &oldRaw[:1][0] != &raw[:1][0]) {
		slot.prevAlloc = oldRaw[:0]
	}
	spansCopy := spans
	slot.spans = &spansCopy
	slot.hash = hash
	return hash
}

// Delay only the last byte, independently of Encoder's write chunking. The
// caller verifies that it is the protocol framing newline before taking Sum.
// All value bytes, including any escaped original newlines, enter the digest.
type runtimeJSONValueHashWriter struct {
	digest     hash.Hash
	pending    byte
	hasPending bool
}

func (w *runtimeJSONValueHashWriter) Write(body []byte) (int, error) {
	n := len(body)
	if n == 0 {
		return 0, nil
	}
	if w.hasPending {
		_, _ = w.digest.Write([]byte{w.pending})
	}
	_, _ = w.digest.Write(body[:n-1])
	w.pending, w.hasPending = body[n-1], true
	return n, nil
}

func StableRuntimeBootstrapHash(bootstrap RuntimeBootstrap) string {
	payload, _ := json.Marshal(bootstrap)
	digest := sha256.Sum256(payload)
	return "runtime-bootstrap-" + hex.EncodeToString(digest[:])
}

// RuntimeOwnerPolicyIdentity identifies the capability-owner policy used to
// produce an observation. It carries no retry or transition choice.
type RuntimeOwnerPolicyIdentity struct {
	OwnerModuleID     string
	PolicyRevision    string
	CatalogSnapshotID string
}

func (v RuntimeOwnerPolicyIdentity) Normalized() RuntimeOwnerPolicyIdentity {
	v.OwnerModuleID = strings.TrimSpace(v.OwnerModuleID)
	v.PolicyRevision = strings.TrimSpace(v.PolicyRevision)
	v.CatalogSnapshotID = strings.TrimSpace(v.CatalogSnapshotID)
	return v
}

func (v RuntimeOwnerPolicyIdentity) Validate() error {
	v = v.Normalized()
	if v.OwnerModuleID == "" || v.PolicyRevision == "" || v.CatalogSnapshotID == "" {
		return fmt.Errorf("runtime owner policy identity requires owner, policy revision and catalog snapshot")
	}
	return nil
}

// RuntimeEquivalentAuthorityInputIdentity is the semantic input to one SM-2
// authority compilation. Request, lease, physical call and CAS revisions are
// intentionally absent.
type RuntimeEquivalentAuthorityInputIdentity struct {
	DefinitionRevision        string
	FocusHash                 string
	BindingSetHash            string
	EvidenceRevision          string
	CapabilityStateHash       string
	CatalogRevisionHash       string
	PrerequisiteFreshnessHash string
	OwnerObservationHash      string
	// Semantic dependency observations, excluding request/CAS identity. Repeated
	// reads of the same source/result must not reset the no-progress guard.
	ApplicabilityObservationHash string `json:",omitempty"`
}

func (v RuntimeEquivalentAuthorityInputIdentity) Normalized() RuntimeEquivalentAuthorityInputIdentity {
	v.DefinitionRevision = strings.TrimSpace(v.DefinitionRevision)
	v.FocusHash = strings.TrimSpace(v.FocusHash)
	v.BindingSetHash = strings.TrimSpace(v.BindingSetHash)
	v.EvidenceRevision = strings.TrimSpace(v.EvidenceRevision)
	v.CapabilityStateHash = strings.TrimSpace(v.CapabilityStateHash)
	v.CatalogRevisionHash = strings.TrimSpace(v.CatalogRevisionHash)
	v.PrerequisiteFreshnessHash = strings.TrimSpace(v.PrerequisiteFreshnessHash)
	v.OwnerObservationHash = strings.TrimSpace(v.OwnerObservationHash)
	v.ApplicabilityObservationHash = strings.TrimSpace(v.ApplicabilityObservationHash)
	return v
}

func (v RuntimeEquivalentAuthorityInputIdentity) Validate() error {
	v = v.Normalized()
	if v.DefinitionRevision == "" || v.FocusHash == "" || v.EvidenceRevision == "" || v.CapabilityStateHash == "" {
		return fmt.Errorf("equivalent authority input identity requires definition, focus, evidence and capability identity")
	}
	return nil
}

func StableRuntimeEquivalentAuthorityInputID(v RuntimeEquivalentAuthorityInputIdentity) string {
	v = v.Normalized()
	h := sha256.New()
	writeHashString(h, v.DefinitionRevision)
	writeHashString(h, v.FocusHash)
	writeHashString(h, v.BindingSetHash)
	writeHashString(h, v.EvidenceRevision)
	writeHashString(h, v.CapabilityStateHash)
	writeHashString(h, v.CatalogRevisionHash)
	writeHashString(h, v.PrerequisiteFreshnessHash)
	writeHashString(h, v.OwnerObservationHash)
	if v.ApplicabilityObservationHash != "" {
		writeHashString(h, "evidence-applicability/v1")
		writeHashString(h, v.ApplicabilityObservationHash)
	}
	return "authority-input-" + hex.EncodeToString(h.Sum(nil))[:20]
}

// RuntimeBindingContradictionIdentity ties one owner observation to one exact
// current binding. It is an immutable fact, not a recovery instruction.
type RuntimeBindingContradictionIdentity struct {
	WorkspaceID      common.WorkspaceID
	RunID            common.RunID
	AuthorityInputID string
	BindingID        string
	BindingSetHash   string
	ProposalHash     string
	ObservationHash  string
	OwnerPolicy      RuntimeOwnerPolicyIdentity
	ObservedRevision string
}

func (v RuntimeBindingContradictionIdentity) Normalized() RuntimeBindingContradictionIdentity {
	v.AuthorityInputID = strings.TrimSpace(v.AuthorityInputID)
	v.BindingID = strings.TrimSpace(v.BindingID)
	v.BindingSetHash = strings.TrimSpace(v.BindingSetHash)
	v.ProposalHash = strings.TrimSpace(v.ProposalHash)
	v.ObservationHash = strings.TrimSpace(v.ObservationHash)
	v.OwnerPolicy = v.OwnerPolicy.Normalized()
	v.ObservedRevision = strings.TrimSpace(v.ObservedRevision)
	return v
}

func (v RuntimeBindingContradictionIdentity) Validate() error {
	v = v.Normalized()
	if v.WorkspaceID == "" || v.RunID == "" || v.AuthorityInputID == "" || v.BindingID == "" || v.BindingSetHash == "" || v.ProposalHash == "" || v.ObservationHash == "" || v.ObservedRevision == "" {
		return fmt.Errorf("runtime binding contradiction identity is incomplete")
	}
	return v.OwnerPolicy.Validate()
}

// RuntimeOpaqueLifecycleFence is authored by Orchestrator and compared by
// resource owners without decoding SM-2/SM-3/SM-4 aggregate fields.
type RuntimeOpaqueLifecycleFence struct {
	WorkspaceID          common.WorkspaceID
	RunID                common.RunID
	RuntimeStateRevision uint64
	RuntimeStateRef      string
	RuntimeStateHash     string
	BootstrapHash        string
	GovernanceFenceID    string
	// GovernanceResolutionID is empty for an unresolved governance wait and
	// carries the exact persisted resolution for a continuation handoff. Run
	// treats it as an opaque equality fence; it never decodes Runtime authority.
	GovernanceResolutionID common.ResolutionID
	TerminalFenceID        string
}

func (v RuntimeOpaqueLifecycleFence) Normalized() RuntimeOpaqueLifecycleFence {
	v.RuntimeStateRef = strings.TrimSpace(v.RuntimeStateRef)
	v.RuntimeStateHash = strings.TrimSpace(v.RuntimeStateHash)
	v.BootstrapHash = strings.TrimSpace(v.BootstrapHash)
	v.GovernanceFenceID = strings.TrimSpace(v.GovernanceFenceID)
	v.GovernanceResolutionID = common.ResolutionID(strings.TrimSpace(string(v.GovernanceResolutionID)))
	v.TerminalFenceID = strings.TrimSpace(v.TerminalFenceID)
	return v
}

func (v RuntimeOpaqueLifecycleFence) Validate() error {
	v = v.Normalized()
	if v.WorkspaceID == "" || v.RunID == "" || v.RuntimeStateRevision == 0 || v.RuntimeStateRef == "" || v.RuntimeStateHash == "" || v.BootstrapHash == "" {
		return fmt.Errorf("runtime opaque lifecycle fence requires scope, state and bootstrap identity")
	}
	if (v.GovernanceFenceID == "") == (v.TerminalFenceID == "") {
		return fmt.Errorf("runtime opaque lifecycle fence requires exactly one governance or terminal identity")
	}
	if v.GovernanceResolutionID != "" && v.GovernanceFenceID == "" {
		return fmt.Errorf("runtime opaque lifecycle fence resolution requires governance identity")
	}
	return nil
}
