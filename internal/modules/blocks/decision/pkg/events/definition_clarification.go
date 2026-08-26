package events

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	definitioncontract "github.com/muidea/workorch/internal/pkg/workorchdefinitioncontract"
)

const DefinitionClarificationSchemaVersionCurrent = definitioncontract.DefinitionClarificationSchemaVersionCurrent

const (
	DefinitionClarificationActionRewriteAllOf            = definitioncontract.DefinitionClarificationActionRewriteAllOf
	DefinitionClarificationActionFixTemplateProfile      = definitioncontract.DefinitionClarificationActionFixTemplateProfile
	DefinitionClarificationActionReviseAndRestart        = "revise_definition_and_restart"
	DefinitionClarificationResolutionCandidateAdjusted   = definitioncontract.DefinitionClarificationResolutionCandidateAdjusted
	DefinitionClarificationResolutionCandidateSuperseded = definitioncontract.DefinitionClarificationResolutionCandidateSuperseded
	DefinitionClarificationResolutionReplacementBound    = "definition_replacement_intent_bound"
)

type DefinitionClarificationDisposition string

const (
	DefinitionClarificationCreated   DefinitionClarificationDisposition = DefinitionClarificationDisposition(definitioncontract.DefinitionClarificationCreated)
	DefinitionClarificationDuplicate DefinitionClarificationDisposition = DefinitionClarificationDisposition(definitioncontract.DefinitionClarificationDuplicate)
	DefinitionClarificationResolved  DefinitionClarificationDisposition = DefinitionClarificationDisposition(definitioncontract.DefinitionClarificationResolved)
	DefinitionClarificationStale     DefinitionClarificationDisposition = DefinitionClarificationDisposition(definitioncontract.DefinitionClarificationStale)
)

type RuntimeDefinitionGapKind string

const (
	RuntimeDefinitionGapImplicitOneOf          RuntimeDefinitionGapKind = "implicit_one_of"
	RuntimeDefinitionGapUndeclaredPrerequisite RuntimeDefinitionGapKind = "undeclared_prerequisite"
	RuntimeDefinitionGapSemanticAmbiguity      RuntimeDefinitionGapKind = "semantic_ambiguity"
)

// Workspace clarification EventHub roots are Decision-owned declared types.
// Their underlying values are the owner-neutral DTO contract, but keeping the
// roots named prevents another owner package from becoming the EventHub
// authority by type aliasing.
type OpenWorkspaceDefinitionClarificationRequestCommand definitioncontract.WorkspaceDefinitionClarificationRequestIdentity
type OpenWorkspaceDefinitionClarificationRequestResult struct {
	TicketID                        common.TicketID
	WorkspaceID                     common.WorkspaceID
	CandidateID                     string
	CandidateRevision               uint64
	Disposition                     DefinitionClarificationDisposition
	ClarificationRequestBindingHash string
}
type ResolveWorkspaceDefinitionClarificationByAdmissionCommand definitioncontract.WorkspaceDefinitionClarificationAdmissionResolutionIdentity

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

type QueryWorkspaceDefinitionClarificationFactCommand struct {
	WorkspaceID common.WorkspaceID
	TicketID    common.TicketID
}

type QueryWorkspaceDefinitionClarificationFactResult struct {
	Found      bool
	Ticket     *definitioncontract.WorkspaceDefinitionClarificationTicketRecord
	Resolution *definitioncontract.WorkspaceDefinitionClarificationResolutionRecord
}

func (c QueryWorkspaceDefinitionClarificationFactCommand) Validate() error {
	if c.WorkspaceID == "" || c.TicketID == "" {
		return fmt.Errorf("%w: workspace Definition clarification query requires scope", common.ErrInvalidArgument)
	}
	return nil
}

func (r QueryWorkspaceDefinitionClarificationFactResult) Normalized() QueryWorkspaceDefinitionClarificationFactResult {
	if r.Ticket != nil {
		value := r.Ticket.Normalized()
		r.Ticket = &value
	}
	if r.Resolution != nil {
		value := r.Resolution.Normalized()
		r.Resolution = &value
	}
	return r
}

func (r QueryWorkspaceDefinitionClarificationFactResult) ValidateFor(c QueryWorkspaceDefinitionClarificationFactCommand) error {
	if err := c.Validate(); err != nil {
		return err
	}
	r = r.Normalized()
	if !r.Found {
		if r.Ticket != nil || r.Resolution != nil {
			return fmt.Errorf("workspace Definition clarification absent query contains facts")
		}
		return nil
	}
	if r.Ticket == nil {
		return fmt.Errorf("workspace Definition clarification persisted ticket is missing")
	}
	if err := r.Ticket.Validate(); err != nil {
		return err
	}
	request := r.Ticket.RequestIdentity
	if request.WorkspaceID != c.WorkspaceID || request.TicketID != c.TicketID {
		return fmt.Errorf("workspace Definition clarification persisted ticket scope mismatch")
	}
	if r.Resolution == nil {
		return nil
	}
	if err := r.Resolution.Validate(); err != nil {
		return err
	}
	resolution := r.Resolution.ResolutionIdentity
	if resolution.WorkspaceID != request.WorkspaceID || resolution.TicketID != request.TicketID ||
		resolution.OldCandidateID != request.CandidateID || resolution.OldCandidateRevision != request.CandidateRevision || resolution.OldCandidateHash != request.CandidateHash {
		return fmt.Errorf("workspace Definition clarification persisted resolution does not bind ticket")
	}
	return nil
}

func (c OpenWorkspaceDefinitionClarificationRequestCommand) contract() definitioncontract.WorkspaceDefinitionClarificationRequestIdentity {
	return definitioncontract.WorkspaceDefinitionClarificationRequestIdentity(c)
}

func (c OpenWorkspaceDefinitionClarificationRequestCommand) Normalized() OpenWorkspaceDefinitionClarificationRequestCommand {
	return OpenWorkspaceDefinitionClarificationRequestCommand(c.contract().Normalized())
}

func (c OpenWorkspaceDefinitionClarificationRequestCommand) Validate() error {
	return c.contract().Validate()
}

func (c OpenWorkspaceDefinitionClarificationRequestCommand) BindingHash() string {
	return c.contract().BindingHash()
}

func (r OpenWorkspaceDefinitionClarificationRequestResult) Normalized() OpenWorkspaceDefinitionClarificationRequestResult {
	value := r.contract().Normalized()
	return OpenWorkspaceDefinitionClarificationRequestResult{
		TicketID: value.TicketID, WorkspaceID: value.WorkspaceID, CandidateID: value.CandidateID,
		CandidateRevision: value.CandidateRevision, Disposition: DefinitionClarificationDisposition(value.Disposition),
		ClarificationRequestBindingHash: value.ClarificationRequestBindingHash,
	}
}

func (r OpenWorkspaceDefinitionClarificationRequestResult) ValidateFor(c OpenWorkspaceDefinitionClarificationRequestCommand) error {
	return r.contract().ValidateFor(c.contract())
}

func (r OpenWorkspaceDefinitionClarificationRequestResult) contract() definitioncontract.WorkspaceDefinitionClarificationTicketBinding {
	return definitioncontract.WorkspaceDefinitionClarificationTicketBinding{
		TicketID: r.TicketID, WorkspaceID: r.WorkspaceID, CandidateID: r.CandidateID,
		CandidateRevision: r.CandidateRevision, Disposition: definitioncontract.DefinitionClarificationDisposition(r.Disposition),
		ClarificationRequestBindingHash: r.ClarificationRequestBindingHash,
	}
}

func (r OpenWorkspaceDefinitionClarificationRequestResult) BindingValue() definitioncontract.WorkspaceDefinitionClarificationTicketBinding {
	return r.contract().Normalized()
}

func (c ResolveWorkspaceDefinitionClarificationByAdmissionCommand) contract() definitioncontract.WorkspaceDefinitionClarificationAdmissionResolutionIdentity {
	return definitioncontract.WorkspaceDefinitionClarificationAdmissionResolutionIdentity(c)
}

func (c ResolveWorkspaceDefinitionClarificationByAdmissionCommand) Normalized() ResolveWorkspaceDefinitionClarificationByAdmissionCommand {
	return ResolveWorkspaceDefinitionClarificationByAdmissionCommand(c.contract().Normalized())
}

func (c ResolveWorkspaceDefinitionClarificationByAdmissionCommand) Validate() error {
	return c.contract().Validate()
}

func (c ResolveWorkspaceDefinitionClarificationByAdmissionCommand) BindingHash(resolutionID common.ResolutionID) string {
	return c.contract().BindingHash(resolutionID)
}

func (c ReconcileWorkspaceDefinitionClarificationCandidateCommand) contract() definitioncontract.WorkspaceDefinitionClarificationReconciliationIdentity {
	return definitioncontract.WorkspaceDefinitionClarificationReconciliationIdentity{
		OldRequest: c.OldRequest.contract(),
		NewRequest: c.NewRequest.contract(),
	}
}

func (c ReconcileWorkspaceDefinitionClarificationCandidateCommand) Normalized() ReconcileWorkspaceDefinitionClarificationCandidateCommand {
	return ReconcileWorkspaceDefinitionClarificationCandidateCommand{
		OldRequest: c.OldRequest.Normalized(),
		NewRequest: c.NewRequest.Normalized(),
	}
}

func (c ReconcileWorkspaceDefinitionClarificationCandidateCommand) Validate() error {
	return c.contract().Validate()
}

func (c ReconcileWorkspaceDefinitionClarificationCandidateCommand) BindingHash(resolutionID common.ResolutionID) string {
	return c.contract().BindingHash(resolutionID)
}

func (r ReconcileWorkspaceDefinitionClarificationCandidateResult) Normalized() ReconcileWorkspaceDefinitionClarificationCandidateResult {
	value := r.contract().Normalized()
	return ReconcileWorkspaceDefinitionClarificationCandidateResult{
		WorkspaceID: value.WorkspaceID, OldTicketID: value.OldTicketID, OldResolutionID: value.OldResolutionID,
		NewTicketID: value.NewTicketID, NewTicketDisposition: DefinitionClarificationDisposition(value.NewTicketDisposition),
		NewRequestBindingHash: value.NewRequestBindingHash, ReconciliationBindingHash: value.ReconciliationBindingHash,
	}
}

func (r ReconcileWorkspaceDefinitionClarificationCandidateResult) ValidateFor(c ReconcileWorkspaceDefinitionClarificationCandidateCommand) error {
	return r.contract().ValidateFor(c.contract())
}

func (r ReconcileWorkspaceDefinitionClarificationCandidateResult) contract() definitioncontract.WorkspaceDefinitionClarificationReconciliationBinding {
	return definitioncontract.WorkspaceDefinitionClarificationReconciliationBinding{
		WorkspaceID: r.WorkspaceID, OldTicketID: r.OldTicketID, OldResolutionID: r.OldResolutionID,
		NewTicketID: r.NewTicketID, NewTicketDisposition: definitioncontract.DefinitionClarificationDisposition(r.NewTicketDisposition),
		NewRequestBindingHash: r.NewRequestBindingHash, ReconciliationBindingHash: r.ReconciliationBindingHash,
	}
}

func (r ResolveWorkspaceDefinitionClarificationByAdmissionResult) Normalized() ResolveWorkspaceDefinitionClarificationByAdmissionResult {
	value := r.contract().Normalized()
	return ResolveWorkspaceDefinitionClarificationByAdmissionResult{
		TicketID: value.TicketID, WorkspaceID: value.WorkspaceID, ResolutionID: value.ResolutionID,
		ResolutionType: value.ResolutionType, Disposition: DefinitionClarificationDisposition(value.Disposition),
		ResolutionBindingHash: value.ResolutionBindingHash,
	}
}

func (r ResolveWorkspaceDefinitionClarificationByAdmissionResult) ValidateFor(c ResolveWorkspaceDefinitionClarificationByAdmissionCommand) error {
	return r.contract().ValidateFor(c.contract())
}

func (r ResolveWorkspaceDefinitionClarificationByAdmissionResult) contract() definitioncontract.WorkspaceDefinitionClarificationResolutionBinding {
	return definitioncontract.WorkspaceDefinitionClarificationResolutionBinding{
		TicketID: r.TicketID, WorkspaceID: r.WorkspaceID, ResolutionID: r.ResolutionID,
		ResolutionType: r.ResolutionType, Disposition: definitioncontract.DefinitionClarificationDisposition(r.Disposition),
		ResolutionBindingHash: r.ResolutionBindingHash,
	}
}

func (r ResolveWorkspaceDefinitionClarificationByAdmissionResult) BindingValue() definitioncontract.WorkspaceDefinitionClarificationResolutionBinding {
	return r.contract().Normalized()
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
	return definitioncontract.StableDefinitionClarificationResolutionID(ticketID, bindingHash)
}

func CanonicalDefinitionClarificationSignatureHash(kind, version, signature string) string {
	return definitioncontract.CanonicalDefinitionClarificationSignatureHash(kind, version, signature)
}

func StableWorkspaceDefinitionClarificationTicketID(c OpenWorkspaceDefinitionClarificationRequestCommand) common.TicketID {
	return definitioncontract.StableWorkspaceDefinitionClarificationTicketID(c.contract())
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
