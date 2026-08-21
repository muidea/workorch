package events

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

const DefinitionClarificationSchemaVersionCurrent uint32 = 1

const (
	DefinitionClarificationActionRewriteAllOf            = "rewrite_definition_all_of"
	DefinitionClarificationActionFixTemplateProfile      = "select_or_fix_template_profile"
	DefinitionClarificationActionReviseAndRestart        = "revise_definition_and_restart"
	DefinitionClarificationResolutionCandidateAdjusted   = "definition_candidate_adjusted"
	DefinitionClarificationResolutionCandidateSuperseded = "definition_candidate_revision_superseded"
	DefinitionClarificationResolutionReplacementBound    = "definition_replacement_intent_bound"
)

type DefinitionClarificationDisposition string

const (
	DefinitionClarificationCreated   DefinitionClarificationDisposition = "created"
	DefinitionClarificationDuplicate DefinitionClarificationDisposition = "duplicate"
	DefinitionClarificationResolved  DefinitionClarificationDisposition = "resolved"
	DefinitionClarificationStale     DefinitionClarificationDisposition = "stale"
)

type RuntimeDefinitionGapKind string

const (
	RuntimeDefinitionGapImplicitOneOf          RuntimeDefinitionGapKind = "implicit_one_of"
	RuntimeDefinitionGapUndeclaredPrerequisite RuntimeDefinitionGapKind = "undeclared_prerequisite"
	RuntimeDefinitionGapSemanticAmbiguity      RuntimeDefinitionGapKind = "semantic_ambiguity"
)

type OpenWorkspaceDefinitionClarificationRequestCommand struct {
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

type OpenWorkspaceDefinitionClarificationRequestResult struct {
	TicketID                        common.TicketID
	WorkspaceID                     common.WorkspaceID
	CandidateID                     string
	CandidateRevision               uint64
	Disposition                     DefinitionClarificationDisposition
	ClarificationRequestBindingHash string
}

type ResolveWorkspaceDefinitionClarificationByAdmissionCommand struct {
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

// ReconcileWorkspaceDefinitionClarificationCandidate replaces one exact
// invalid pre-seal candidate with a newer exact invalid candidate. Both open
// request values are carried in full so Decision can verify the old ticket and
// create/return the new deterministic ticket without consulting Workspace.
type ReconcileWorkspaceDefinitionClarificationCandidateCommand struct {
	OldRequest OpenWorkspaceDefinitionClarificationRequestCommand
	NewRequest OpenWorkspaceDefinitionClarificationRequestCommand
}

type ReconcileWorkspaceDefinitionClarificationCandidateResult struct {
	WorkspaceID               common.WorkspaceID
	OldTicketID               common.TicketID
	OldResolutionID           common.ResolutionID
	NewTicketID               common.TicketID
	NewTicketDisposition      DefinitionClarificationDisposition
	NewRequestBindingHash     string
	ReconciliationBindingHash string
}

type ResolveWorkspaceDefinitionClarificationByAdmissionResult struct {
	TicketID              common.TicketID
	WorkspaceID           common.WorkspaceID
	ResolutionID          common.ResolutionID
	ResolutionType        string
	Disposition           DefinitionClarificationDisposition
	ResolutionBindingHash string
}

type OpenRuntimeDefinitionClarificationRequestCommand struct {
	SchemaVersion                uint32
	TicketID                     common.TicketID
	WorkspaceID                  common.WorkspaceID
	RunID                        common.RunID
	DefinitionRevision           string
	StartupInputHash             string
	AuthoritySnapshotID          string
	ClarificationDispatchID      string
	FocusRef                     string
	LogicalRequestID             string
	SemanticDocumentHash         string
	GapKind                      RuntimeDefinitionGapKind
	OffendingItemRefs            []string
	CanonicalGapSignatureVersion string
	CanonicalGapSignature        string
	CanonicalGapSignatureHash    string
	RequiredActionKind           string
}

type OpenRuntimeDefinitionClarificationRequestResult struct {
	TicketID                        common.TicketID
	WorkspaceID                     common.WorkspaceID
	RunID                           common.RunID
	AuthoritySnapshotID             string
	ClarificationDispatchID         string
	Disposition                     DefinitionClarificationDisposition
	ClarificationRequestBindingHash string
}

type RuntimeDefinitionClarificationConflictKind string

const (
	RuntimeDefinitionClarificationNoConflict         RuntimeDefinitionClarificationConflictKind = "none"
	RuntimeDefinitionClarificationTicketAlreadyBound RuntimeDefinitionClarificationConflictKind = "ticket_already_bound"
	RuntimeDefinitionClarificationScopeMismatch      RuntimeDefinitionClarificationConflictKind = "scope_mismatch"
)

type ResolveRuntimeDefinitionClarificationByReplacementCommand struct {
	TicketID                      common.TicketID
	WorkspaceID                   common.WorkspaceID
	PredecessorRunID              common.RunID
	ReplacementIntentID           string
	PredecessorDefinitionRevision string
	PredecessorStartupInputHash   string
	SuccessorDefinitionRevision   string
	SuccessorStartupInputHash     string
	ReplacementIntentHash         string
}

type ResolveRuntimeDefinitionClarificationByReplacementResult struct {
	TicketID                     common.TicketID
	WorkspaceID                  common.WorkspaceID
	PredecessorRunID             common.RunID
	ReplacementIntentID          string
	ResolutionID                 common.ResolutionID
	ResolutionType               string
	Disposition                  DefinitionClarificationDisposition
	ConflictKind                 RuntimeDefinitionClarificationConflictKind
	CurrentReplacementIntentID   string
	CurrentReplacementIntentHash string
	ResolutionBindingHash        string
}

func (r ResolveRuntimeDefinitionClarificationByReplacementResult) Normalized() ResolveRuntimeDefinitionClarificationByReplacementResult {
	r.ReplacementIntentID = strings.TrimSpace(r.ReplacementIntentID)
	r.ResolutionType = strings.TrimSpace(r.ResolutionType)
	r.CurrentReplacementIntentID = strings.TrimSpace(r.CurrentReplacementIntentID)
	r.CurrentReplacementIntentHash = strings.TrimSpace(r.CurrentReplacementIntentHash)
	r.ResolutionBindingHash = strings.TrimSpace(r.ResolutionBindingHash)
	return r
}

// ValidateFor seals disposition, conflict and binding hash as one result
// contract. A caller must not accept a merely non-empty hash or reinterpret a
// stale conflict as a successful replacement acknowledgement.
func (r ResolveRuntimeDefinitionClarificationByReplacementResult) ValidateFor(c ResolveRuntimeDefinitionClarificationByReplacementCommand) error {
	c = c.Normalized()
	r = r.Normalized()
	if err := c.Validate(); err != nil {
		return err
	}
	if r.TicketID != c.TicketID || r.WorkspaceID != c.WorkspaceID || r.PredecessorRunID != c.PredecessorRunID || r.ReplacementIntentID != c.ReplacementIntentID || r.ResolutionType != DefinitionClarificationResolutionReplacementBound {
		return fmt.Errorf("runtime Definition clarification result scope is invalid")
	}
	switch {
	case r.ConflictKind == RuntimeDefinitionClarificationNoConflict && (r.Disposition == DefinitionClarificationResolved || r.Disposition == DefinitionClarificationDuplicate):
		if r.ResolutionID == "" || r.CurrentReplacementIntentID != "" || r.CurrentReplacementIntentHash != "" || r.ResolutionBindingHash != c.BindingHash(r.ResolutionID, DefinitionClarificationResolved, RuntimeDefinitionClarificationNoConflict, "", "") {
			return fmt.Errorf("runtime Definition clarification success binding is invalid")
		}
		return nil
	case r.Disposition == DefinitionClarificationStale && r.ResolutionID == "" && r.ConflictKind == RuntimeDefinitionClarificationScopeMismatch:
		if r.CurrentReplacementIntentID != "" || r.CurrentReplacementIntentHash != "" || r.ResolutionBindingHash != c.BindingHash("", DefinitionClarificationStale, r.ConflictKind, "", "") {
			return fmt.Errorf("runtime Definition clarification scope-mismatch binding is invalid")
		}
		return nil
	case r.Disposition == DefinitionClarificationStale && r.ResolutionID == "" && r.ConflictKind == RuntimeDefinitionClarificationTicketAlreadyBound:
		if r.CurrentReplacementIntentID == "" || r.CurrentReplacementIntentHash == "" || r.ResolutionBindingHash != c.BindingHash("", DefinitionClarificationStale, r.ConflictKind, r.CurrentReplacementIntentID, r.CurrentReplacementIntentHash) {
			return fmt.Errorf("runtime Definition clarification stale-binding result is invalid")
		}
		return nil
	default:
		return fmt.Errorf("runtime Definition clarification result disposition is invalid")
	}
}

func (c OpenWorkspaceDefinitionClarificationRequestCommand) Normalized() OpenWorkspaceDefinitionClarificationRequestCommand {
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

func (c OpenWorkspaceDefinitionClarificationRequestCommand) Validate() error {
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

func (c OpenWorkspaceDefinitionClarificationRequestCommand) BindingHash() string {
	c = c.Normalized()
	return clarificationHash("workspace-open", fmt.Sprintf("%d", c.SchemaVersion), string(c.TicketID), string(c.WorkspaceID), c.CandidateID, fmt.Sprintf("%d", c.CandidateRevision), c.CandidateHash, c.ViolationCode, strings.Join(c.OffendingRefs, "\x00"), c.CanonicalSignatureVersion, c.CanonicalSignature, c.CanonicalSignatureHash, c.RequiredActionKind)
}

func StableWorkspaceDefinitionClarificationTicketID(c OpenWorkspaceDefinitionClarificationRequestCommand) common.TicketID {
	c = c.Normalized()
	return common.TicketID("workspace-definition-clarification:" + clarificationDigest(string(c.WorkspaceID), c.CandidateID, fmt.Sprintf("%d", c.CandidateRevision), c.CandidateHash, c.ViolationCode, c.CanonicalSignatureHash)[:32])
}

func (c OpenRuntimeDefinitionClarificationRequestCommand) Normalized() OpenRuntimeDefinitionClarificationRequestCommand {
	c.DefinitionRevision = strings.TrimSpace(c.DefinitionRevision)
	c.StartupInputHash = strings.TrimSpace(c.StartupInputHash)
	c.AuthoritySnapshotID = strings.TrimSpace(c.AuthoritySnapshotID)
	c.ClarificationDispatchID = strings.TrimSpace(c.ClarificationDispatchID)
	c.FocusRef = strings.TrimSpace(c.FocusRef)
	c.LogicalRequestID = strings.TrimSpace(c.LogicalRequestID)
	c.SemanticDocumentHash = strings.TrimSpace(c.SemanticDocumentHash)
	c.GapKind = RuntimeDefinitionGapKind(strings.TrimSpace(string(c.GapKind)))
	c.OffendingItemRefs = normalizeClarificationRefs(c.OffendingItemRefs)
	c.CanonicalGapSignatureVersion = strings.TrimSpace(c.CanonicalGapSignatureVersion)
	c.CanonicalGapSignature = strings.TrimSpace(c.CanonicalGapSignature)
	c.CanonicalGapSignatureHash = strings.TrimSpace(c.CanonicalGapSignatureHash)
	c.RequiredActionKind = strings.TrimSpace(c.RequiredActionKind)
	return c
}

func (c OpenRuntimeDefinitionClarificationRequestCommand) Validate() error {
	c = c.Normalized()
	if c.SchemaVersion != DefinitionClarificationSchemaVersionCurrent || c.TicketID == "" || c.WorkspaceID == "" || c.RunID == "" || c.DefinitionRevision == "" || c.StartupInputHash == "" || c.AuthoritySnapshotID == "" || c.ClarificationDispatchID == "" || c.FocusRef == "" || c.LogicalRequestID == "" || c.SemanticDocumentHash == "" || len(c.OffendingItemRefs) == 0 || c.CanonicalGapSignatureVersion == "" || c.CanonicalGapSignature == "" || c.CanonicalGapSignatureHash == "" || c.RequiredActionKind != DefinitionClarificationActionReviseAndRestart {
		return fmt.Errorf("runtime Definition clarification request is incomplete")
	}
	switch c.GapKind {
	case RuntimeDefinitionGapImplicitOneOf, RuntimeDefinitionGapUndeclaredPrerequisite, RuntimeDefinitionGapSemanticAmbiguity:
	default:
		return fmt.Errorf("runtime Definition clarification gap kind is invalid")
	}
	if c.CanonicalGapSignatureHash != clarificationHash("runtime-signature", c.CanonicalGapSignatureVersion, c.CanonicalGapSignature) || c.TicketID != StableRuntimeDefinitionClarificationTicketID(c) {
		return fmt.Errorf("runtime Definition clarification signature or ticket identity mismatch")
	}
	return nil
}

func (c OpenRuntimeDefinitionClarificationRequestCommand) BindingHash() string {
	c = c.Normalized()
	return clarificationHash("runtime-open", fmt.Sprintf("%d", c.SchemaVersion), string(c.TicketID), string(c.WorkspaceID), string(c.RunID), c.DefinitionRevision, c.StartupInputHash, c.AuthoritySnapshotID, c.ClarificationDispatchID, c.FocusRef, c.LogicalRequestID, c.SemanticDocumentHash, string(c.GapKind), strings.Join(c.OffendingItemRefs, "\x00"), c.CanonicalGapSignatureVersion, c.CanonicalGapSignature, c.CanonicalGapSignatureHash, c.RequiredActionKind)
}

func StableRuntimeDefinitionClarificationTicketID(c OpenRuntimeDefinitionClarificationRequestCommand) common.TicketID {
	c = c.Normalized()
	return common.TicketID("runtime-definition-clarification:" + clarificationDigest(string(c.WorkspaceID), string(c.RunID), c.ClarificationDispatchID, c.CanonicalGapSignatureHash)[:32])
}

func (c ResolveWorkspaceDefinitionClarificationByAdmissionCommand) Normalized() ResolveWorkspaceDefinitionClarificationByAdmissionCommand {
	c.OldCandidateID = strings.TrimSpace(c.OldCandidateID)
	c.OldCandidateHash = strings.TrimSpace(c.OldCandidateHash)
	c.NewCandidateID = strings.TrimSpace(c.NewCandidateID)
	c.NewCandidateHash = strings.TrimSpace(c.NewCandidateHash)
	c.AdmissionResultHash = strings.TrimSpace(c.AdmissionResultHash)
	c.DefinitionRevision = strings.TrimSpace(c.DefinitionRevision)
	c.StartupInputHash = strings.TrimSpace(c.StartupInputHash)
	return c
}

func (c ReconcileWorkspaceDefinitionClarificationCandidateCommand) Normalized() ReconcileWorkspaceDefinitionClarificationCandidateCommand {
	c.OldRequest = c.OldRequest.Normalized()
	c.NewRequest = c.NewRequest.Normalized()
	return c
}

func (c ReconcileWorkspaceDefinitionClarificationCandidateCommand) Validate() error {
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

func (c ReconcileWorkspaceDefinitionClarificationCandidateCommand) BindingHash(resolutionID common.ResolutionID) string {
	c = c.Normalized()
	return clarificationHash("workspace-reconcile", c.OldRequest.BindingHash(), c.NewRequest.BindingHash(), string(resolutionID), DefinitionClarificationResolutionCandidateSuperseded)
}

func (c ResolveWorkspaceDefinitionClarificationByAdmissionCommand) Validate() error {
	c = c.Normalized()
	if c.TicketID == "" || c.WorkspaceID == "" || c.OldCandidateID == "" || c.OldCandidateRevision == 0 || c.OldCandidateHash == "" || c.NewCandidateID == "" || c.NewCandidateRevision == 0 || c.NewCandidateHash == "" || c.AdmissionResultHash == "" || c.DefinitionRevision == "" || c.StartupInputHash == "" || (c.OldCandidateID == c.NewCandidateID && c.NewCandidateRevision <= c.OldCandidateRevision) {
		return fmt.Errorf("workspace Definition clarification admission resolution is incomplete")
	}
	return nil
}

func (c ResolveWorkspaceDefinitionClarificationByAdmissionCommand) BindingHash(resolutionID common.ResolutionID) string {
	c = c.Normalized()
	return clarificationHash("workspace-resolution", string(c.TicketID), string(c.WorkspaceID), c.OldCandidateID, fmt.Sprintf("%d", c.OldCandidateRevision), c.OldCandidateHash, c.NewCandidateID, fmt.Sprintf("%d", c.NewCandidateRevision), c.NewCandidateHash, c.AdmissionResultHash, c.DefinitionRevision, c.StartupInputHash, string(resolutionID), DefinitionClarificationResolutionCandidateAdjusted)
}

func (c ResolveRuntimeDefinitionClarificationByReplacementCommand) Normalized() ResolveRuntimeDefinitionClarificationByReplacementCommand {
	c.ReplacementIntentID = strings.TrimSpace(c.ReplacementIntentID)
	c.PredecessorDefinitionRevision = strings.TrimSpace(c.PredecessorDefinitionRevision)
	c.PredecessorStartupInputHash = strings.TrimSpace(c.PredecessorStartupInputHash)
	c.SuccessorDefinitionRevision = strings.TrimSpace(c.SuccessorDefinitionRevision)
	c.SuccessorStartupInputHash = strings.TrimSpace(c.SuccessorStartupInputHash)
	c.ReplacementIntentHash = strings.TrimSpace(c.ReplacementIntentHash)
	return c
}

func (c ResolveRuntimeDefinitionClarificationByReplacementCommand) Validate() error {
	c = c.Normalized()
	if c.TicketID == "" || c.WorkspaceID == "" || c.PredecessorRunID == "" || c.ReplacementIntentID == "" || c.PredecessorDefinitionRevision == "" || c.PredecessorStartupInputHash == "" || c.SuccessorDefinitionRevision == "" || c.SuccessorStartupInputHash == "" || c.ReplacementIntentHash == "" || c.PredecessorDefinitionRevision == c.SuccessorDefinitionRevision || c.PredecessorStartupInputHash == c.SuccessorStartupInputHash {
		return fmt.Errorf("runtime Definition clarification replacement resolution is incomplete")
	}
	return nil
}

func (c ResolveRuntimeDefinitionClarificationByReplacementCommand) BindingHash(resolutionID common.ResolutionID, disposition DefinitionClarificationDisposition, conflict RuntimeDefinitionClarificationConflictKind, currentID, currentHash string) string {
	c = c.Normalized()
	return clarificationHash("runtime-resolution", string(c.TicketID), string(c.WorkspaceID), string(c.PredecessorRunID), c.ReplacementIntentID, c.PredecessorDefinitionRevision, c.PredecessorStartupInputHash, c.SuccessorDefinitionRevision, c.SuccessorStartupInputHash, c.ReplacementIntentHash, string(resolutionID), DefinitionClarificationResolutionReplacementBound, string(disposition), string(conflict), strings.TrimSpace(currentID), strings.TrimSpace(currentHash))
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
