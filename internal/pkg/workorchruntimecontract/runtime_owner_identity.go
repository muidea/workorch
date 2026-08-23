package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

func StableRuntimeStateRecordHash(state RuntimeStateRecord) string {
	payload, _ := json.Marshal(state.Clone())
	digest := sha256.Sum256(payload)
	return "runtime-state-" + hex.EncodeToString(digest[:])
}

func StableRuntimeBootstrapHash(bootstrap RuntimeBootstrap) string {
	payload, _ := json.Marshal(bootstrap.Clone())
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
