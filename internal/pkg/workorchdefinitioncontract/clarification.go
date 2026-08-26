// Package workorchdefinitioncontract contains owner-neutral immutable values
// shared by Workspace admission and Decision clarification. These values do
// not select or persist either owner's state; their helpers only normalize and
// prove deterministic identity/binding relationships.
package workorchdefinitioncontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

const DefinitionClarificationSchemaVersionCurrent uint32 = 1

const (
	DefinitionClarificationActionRewriteAllOf            = "rewrite_definition_all_of"
	DefinitionClarificationActionFixTemplateProfile      = "select_or_fix_template_profile"
	DefinitionClarificationResolutionCandidateAdjusted   = "definition_candidate_adjusted"
	DefinitionClarificationResolutionCandidateSuperseded = "definition_candidate_revision_superseded"
)

type DefinitionClarificationDisposition string

const (
	DefinitionClarificationCreated   DefinitionClarificationDisposition = "created"
	DefinitionClarificationDuplicate DefinitionClarificationDisposition = "duplicate"
	DefinitionClarificationResolved  DefinitionClarificationDisposition = "resolved"
	DefinitionClarificationStale     DefinitionClarificationDisposition = "stale"
)

type WorkspaceDefinitionClarificationRequestIdentity struct {
	SchemaVersion             uint32
	TicketID                  common.TicketID
	WorkspaceID               common.WorkspaceID
	CandidateID               string
	CandidateRevision         uint64
	CandidateHash             string
	ViolationCode             string
	OffendingRefs             []string
	CanonicalSignatureVersion string
	CanonicalSignature        string
	CanonicalSignatureHash    string
	RequiredActionKind        string
}

type WorkspaceDefinitionClarificationTicketBinding struct {
	TicketID                        common.TicketID
	WorkspaceID                     common.WorkspaceID
	CandidateID                     string
	CandidateRevision               uint64
	Disposition                     DefinitionClarificationDisposition
	ClarificationRequestBindingHash string
}

type WorkspaceDefinitionClarificationAdmissionResolutionIdentity struct {
	TicketID             common.TicketID
	WorkspaceID          common.WorkspaceID
	OldCandidateID       string
	OldCandidateRevision uint64
	OldCandidateHash     string
	NewCandidateID       string
	NewCandidateRevision uint64
	NewCandidateHash     string
	AdmissionResultHash  string
	DefinitionRevision   string
	StartupInputHash     string
}

// WorkspaceDefinitionClarificationReconciliationIdentity binds one exact prior
// request identity to a newer one without exposing either owner's aggregate.
type WorkspaceDefinitionClarificationReconciliationIdentity struct {
	OldRequest WorkspaceDefinitionClarificationRequestIdentity
	NewRequest WorkspaceDefinitionClarificationRequestIdentity
}

type WorkspaceDefinitionClarificationReconciliationBinding struct {
	WorkspaceID               common.WorkspaceID
	OldTicketID               common.TicketID
	OldResolutionID           common.ResolutionID
	NewTicketID               common.TicketID
	NewTicketDisposition      DefinitionClarificationDisposition
	NewRequestBindingHash     string
	ReconciliationBindingHash string
}

type WorkspaceDefinitionClarificationResolutionBinding struct {
	TicketID              common.TicketID
	WorkspaceID           common.WorkspaceID
	ResolutionID          common.ResolutionID
	ResolutionType        string
	Disposition           DefinitionClarificationDisposition
	ResolutionBindingHash string
}

// WorkspaceDefinitionClarificationTicketRecord is the immutable subset of a
// Decision-owned persisted ticket that another owner may verify. It carries
// identity/hash values only; it is neither an EventHub command nor transition
// authority.
type WorkspaceDefinitionClarificationTicketRecord struct {
	RequestIdentity WorkspaceDefinitionClarificationRequestIdentity
	OpenBindingHash string
}

// WorkspaceDefinitionClarificationResolutionRecord is the immutable subset
// of a Decision-owned persisted resolution. The Decision owner reconstructs
// it only from its ticket aggregate/repository; callers cannot establish its
// provenance by presenting a self-consistent receipt.
type WorkspaceDefinitionClarificationResolutionRecord struct {
	ResolutionIdentity    WorkspaceDefinitionClarificationAdmissionResolutionIdentity
	ResolutionID          common.ResolutionID
	ResolutionType        string
	ResolutionBindingHash string
}

func (c WorkspaceDefinitionClarificationRequestIdentity) Normalized() WorkspaceDefinitionClarificationRequestIdentity {
	c.CandidateID = strings.TrimSpace(c.CandidateID)
	c.CandidateHash = strings.TrimSpace(c.CandidateHash)
	c.ViolationCode = strings.TrimSpace(c.ViolationCode)
	c.OffendingRefs = normalizeClarificationRefs(c.OffendingRefs)
	c.CanonicalSignatureVersion = strings.TrimSpace(c.CanonicalSignatureVersion)
	c.CanonicalSignature = strings.TrimSpace(c.CanonicalSignature)
	c.CanonicalSignatureHash = strings.TrimSpace(c.CanonicalSignatureHash)
	c.RequiredActionKind = strings.TrimSpace(c.RequiredActionKind)
	return c
}

func (c WorkspaceDefinitionClarificationRequestIdentity) Validate() error {
	c = c.Normalized()
	if c.SchemaVersion != DefinitionClarificationSchemaVersionCurrent || c.TicketID == "" || c.WorkspaceID == "" || c.CandidateID == "" || c.CandidateRevision == 0 || c.CandidateHash == "" || c.ViolationCode == "" || len(c.OffendingRefs) == 0 || c.CanonicalSignatureVersion == "" || c.CanonicalSignature == "" || c.CanonicalSignatureHash == "" {
		return fmt.Errorf("workspace Definition clarification request is incomplete")
	}
	if c.RequiredActionKind != DefinitionClarificationActionRewriteAllOf && c.RequiredActionKind != DefinitionClarificationActionFixTemplateProfile {
		return fmt.Errorf("workspace Definition clarification action is invalid")
	}
	if c.CanonicalSignatureHash != clarificationHash("workspace-signature", c.CanonicalSignatureVersion, c.CanonicalSignature) || c.TicketID != StableWorkspaceDefinitionClarificationTicketID(c) {
		return fmt.Errorf("workspace Definition clarification signature or ticket identity mismatch")
	}
	return nil
}

func (c WorkspaceDefinitionClarificationRequestIdentity) BindingHash() string {
	c = c.Normalized()
	return clarificationHash("workspace-open", fmt.Sprintf("%d", c.SchemaVersion), string(c.TicketID), string(c.WorkspaceID), c.CandidateID, fmt.Sprintf("%d", c.CandidateRevision), c.CandidateHash, c.ViolationCode, strings.Join(c.OffendingRefs, "\x00"), c.CanonicalSignatureVersion, c.CanonicalSignature, c.CanonicalSignatureHash, c.RequiredActionKind)
}

// EqualNormalized reports whether two identities carry the same concrete value
// fields after the contract's canonical whitespace/ref normalization. It is
// intentionally stronger than comparing a derived binding hash: owner
// adapters use it to fail closed on every field, including OffendingRefs.
func (c WorkspaceDefinitionClarificationRequestIdentity) EqualNormalized(other WorkspaceDefinitionClarificationRequestIdentity) bool {
	c = c.Normalized()
	other = other.Normalized()
	return c.SchemaVersion == other.SchemaVersion &&
		c.TicketID == other.TicketID &&
		c.WorkspaceID == other.WorkspaceID &&
		c.CandidateID == other.CandidateID &&
		c.CandidateRevision == other.CandidateRevision &&
		c.CandidateHash == other.CandidateHash &&
		c.ViolationCode == other.ViolationCode &&
		slices.Equal(c.OffendingRefs, other.OffendingRefs) &&
		c.CanonicalSignatureVersion == other.CanonicalSignatureVersion &&
		c.CanonicalSignature == other.CanonicalSignature &&
		c.CanonicalSignatureHash == other.CanonicalSignatureHash &&
		c.RequiredActionKind == other.RequiredActionKind
}

func StableWorkspaceDefinitionClarificationTicketID(c WorkspaceDefinitionClarificationRequestIdentity) common.TicketID {
	c = c.Normalized()
	return common.TicketID("workspace-definition-clarification:" + clarificationDigest(string(c.WorkspaceID), c.CandidateID, fmt.Sprintf("%d", c.CandidateRevision), c.CandidateHash, c.ViolationCode, c.CanonicalSignatureHash)[:32])
}

func (r WorkspaceDefinitionClarificationTicketBinding) Normalized() WorkspaceDefinitionClarificationTicketBinding {
	r.CandidateID = strings.TrimSpace(r.CandidateID)
	r.ClarificationRequestBindingHash = strings.TrimSpace(r.ClarificationRequestBindingHash)
	return r
}

func (r WorkspaceDefinitionClarificationTicketBinding) ValidateFor(c WorkspaceDefinitionClarificationRequestIdentity) error {
	c = c.Normalized()
	r = r.Normalized()
	if err := c.Validate(); err != nil {
		return err
	}
	if r.TicketID != c.TicketID || r.WorkspaceID != c.WorkspaceID || r.CandidateID != c.CandidateID || r.CandidateRevision != c.CandidateRevision || r.ClarificationRequestBindingHash != c.BindingHash() {
		return fmt.Errorf("workspace Definition clarification open result binding is invalid")
	}
	if r.Disposition != DefinitionClarificationCreated && r.Disposition != DefinitionClarificationDuplicate {
		return fmt.Errorf("workspace Definition clarification open result disposition is invalid")
	}
	return nil
}

func (c WorkspaceDefinitionClarificationReconciliationIdentity) Normalized() WorkspaceDefinitionClarificationReconciliationIdentity {
	c.OldRequest = c.OldRequest.Normalized()
	c.NewRequest = c.NewRequest.Normalized()
	return c
}

func (c WorkspaceDefinitionClarificationReconciliationIdentity) Validate() error {
	c = c.Normalized()
	if err := c.OldRequest.Validate(); err != nil {
		return err
	}
	if err := c.NewRequest.Validate(); err != nil {
		return err
	}
	if c.OldRequest.WorkspaceID != c.NewRequest.WorkspaceID || c.OldRequest.CandidateID != c.NewRequest.CandidateID || c.NewRequest.CandidateRevision <= c.OldRequest.CandidateRevision || c.OldRequest.TicketID == c.NewRequest.TicketID {
		return fmt.Errorf("workspace Definition clarification reconciliation scope is invalid")
	}
	return nil
}

func (c WorkspaceDefinitionClarificationReconciliationIdentity) BindingHash(resolutionID common.ResolutionID) string {
	c = c.Normalized()
	return clarificationHash("workspace-reconcile", c.OldRequest.BindingHash(), c.NewRequest.BindingHash(), string(resolutionID), DefinitionClarificationResolutionCandidateSuperseded)
}

func (r WorkspaceDefinitionClarificationReconciliationBinding) Normalized() WorkspaceDefinitionClarificationReconciliationBinding {
	r.NewRequestBindingHash = strings.TrimSpace(r.NewRequestBindingHash)
	r.ReconciliationBindingHash = strings.TrimSpace(r.ReconciliationBindingHash)
	return r
}

func (r WorkspaceDefinitionClarificationReconciliationBinding) ValidateFor(c WorkspaceDefinitionClarificationReconciliationIdentity) error {
	c = c.Normalized()
	r = r.Normalized()
	if err := c.Validate(); err != nil {
		return err
	}
	resolutionID := StableDefinitionClarificationResolutionID(c.OldRequest.TicketID, c.NewRequest.BindingHash())
	if r.WorkspaceID != c.OldRequest.WorkspaceID || r.OldTicketID != c.OldRequest.TicketID || r.OldResolutionID != resolutionID || r.NewTicketID != c.NewRequest.TicketID || r.NewRequestBindingHash != c.NewRequest.BindingHash() || r.ReconciliationBindingHash != c.BindingHash(resolutionID) {
		return fmt.Errorf("workspace Definition clarification reconciliation result binding is invalid")
	}
	if r.NewTicketDisposition != DefinitionClarificationCreated && r.NewTicketDisposition != DefinitionClarificationDuplicate {
		return fmt.Errorf("workspace Definition clarification reconciliation disposition is invalid")
	}
	return nil
}

func (c WorkspaceDefinitionClarificationAdmissionResolutionIdentity) Normalized() WorkspaceDefinitionClarificationAdmissionResolutionIdentity {
	c.OldCandidateID = strings.TrimSpace(c.OldCandidateID)
	c.OldCandidateHash = strings.TrimSpace(c.OldCandidateHash)
	c.NewCandidateID = strings.TrimSpace(c.NewCandidateID)
	c.NewCandidateHash = strings.TrimSpace(c.NewCandidateHash)
	c.AdmissionResultHash = strings.TrimSpace(c.AdmissionResultHash)
	c.DefinitionRevision = strings.TrimSpace(c.DefinitionRevision)
	c.StartupInputHash = strings.TrimSpace(c.StartupInputHash)
	return c
}

func (c WorkspaceDefinitionClarificationAdmissionResolutionIdentity) Validate() error {
	c = c.Normalized()
	if c.TicketID == "" || c.WorkspaceID == "" || c.OldCandidateID == "" || c.OldCandidateRevision == 0 || c.OldCandidateHash == "" || c.NewCandidateID == "" || c.NewCandidateRevision == 0 || c.NewCandidateHash == "" || c.AdmissionResultHash == "" || c.DefinitionRevision == "" || c.StartupInputHash == "" || c.OldCandidateID != c.NewCandidateID || c.NewCandidateRevision <= c.OldCandidateRevision {
		return fmt.Errorf("workspace Definition clarification admission resolution is incomplete")
	}
	return nil
}

func (c WorkspaceDefinitionClarificationAdmissionResolutionIdentity) BindingHash(resolutionID common.ResolutionID) string {
	c = c.Normalized()
	return clarificationHash("workspace-resolution", string(c.TicketID), string(c.WorkspaceID), c.OldCandidateID, fmt.Sprintf("%d", c.OldCandidateRevision), c.OldCandidateHash, c.NewCandidateID, fmt.Sprintf("%d", c.NewCandidateRevision), c.NewCandidateHash, c.AdmissionResultHash, c.DefinitionRevision, c.StartupInputHash, string(resolutionID), DefinitionClarificationResolutionCandidateAdjusted)
}

func (r WorkspaceDefinitionClarificationResolutionBinding) Normalized() WorkspaceDefinitionClarificationResolutionBinding {
	r.ResolutionType = strings.TrimSpace(r.ResolutionType)
	r.ResolutionBindingHash = strings.TrimSpace(r.ResolutionBindingHash)
	return r
}

func (r WorkspaceDefinitionClarificationResolutionBinding) ValidateFor(c WorkspaceDefinitionClarificationAdmissionResolutionIdentity) error {
	c = c.Normalized()
	r = r.Normalized()
	if err := c.Validate(); err != nil {
		return err
	}
	seedHash := c.AdmissionResultHash + "\x00" + c.StartupInputHash
	resolutionID := StableDefinitionClarificationResolutionID(c.TicketID, seedHash)
	if r.TicketID != c.TicketID || r.WorkspaceID != c.WorkspaceID || r.ResolutionID != resolutionID || r.ResolutionType != DefinitionClarificationResolutionCandidateAdjusted || r.ResolutionBindingHash != c.BindingHash(resolutionID) {
		return fmt.Errorf("workspace Definition clarification resolution result binding is invalid")
	}
	if r.Disposition != DefinitionClarificationResolved && r.Disposition != DefinitionClarificationDuplicate {
		return fmt.Errorf("workspace Definition clarification resolution disposition is invalid")
	}
	return nil
}

func (r WorkspaceDefinitionClarificationTicketRecord) Normalized() WorkspaceDefinitionClarificationTicketRecord {
	r.RequestIdentity = r.RequestIdentity.Normalized()
	r.OpenBindingHash = strings.TrimSpace(r.OpenBindingHash)
	return r
}

func (r WorkspaceDefinitionClarificationTicketRecord) Validate() error {
	r = r.Normalized()
	if err := r.RequestIdentity.Validate(); err != nil {
		return err
	}
	if r.OpenBindingHash != r.RequestIdentity.BindingHash() {
		return fmt.Errorf("workspace Definition clarification persisted ticket binding is invalid")
	}
	return nil
}

func (r WorkspaceDefinitionClarificationResolutionRecord) Normalized() WorkspaceDefinitionClarificationResolutionRecord {
	r.ResolutionIdentity = r.ResolutionIdentity.Normalized()
	r.ResolutionType = strings.TrimSpace(r.ResolutionType)
	r.ResolutionBindingHash = strings.TrimSpace(r.ResolutionBindingHash)
	return r
}

func (r WorkspaceDefinitionClarificationResolutionRecord) Validate() error {
	r = r.Normalized()
	if err := r.ResolutionIdentity.Validate(); err != nil {
		return err
	}
	if r.ResolutionID == "" || r.ResolutionType != DefinitionClarificationResolutionCandidateAdjusted || r.ResolutionBindingHash != r.ResolutionIdentity.BindingHash(r.ResolutionID) {
		return fmt.Errorf("workspace Definition clarification persisted resolution binding is invalid")
	}
	return nil
}

func StableDefinitionClarificationResolutionID(ticketID common.TicketID, bindingHash string) common.ResolutionID {
	return common.ResolutionID("definition-clarification-resolution:" + clarificationDigest(string(ticketID), strings.TrimSpace(bindingHash))[:32])
}

func CanonicalDefinitionClarificationSignatureHash(kind, version, signature string) string {
	return clarificationHash(kind, strings.TrimSpace(version), strings.TrimSpace(signature))
}

func normalizeClarificationRefs(values []string) []string {
	seen := map[string]struct{}{}
	ret := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		ret = append(ret, value)
	}
	sort.Strings(ret)
	return ret
}

func clarificationHash(kind string, parts ...string) string {
	return kind + "-sha256:" + clarificationDigest(parts...)
}

func clarificationDigest(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		_, _ = fmt.Fprintf(h, "%08x%s", len(part), part)
	}
	return hex.EncodeToString(h.Sum(nil))
}
