package workorchruntimecontract

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeImplementationWorkLedger is an aggregate-local value contract. It is
// not an execution ingress or a second candidate/receipt store. In particular,
// ValidateForSources does not make the protocol available: the private SM-2/3/4
// consumers must prove these commitments against their original transaction
// ledgers before any effect. Admission remains closed until that consumer exists.
// There is no normalization/migration of persisted work records.
type RuntimeImplementationWorkLedger struct {
	Rounds []RuntimeImplementationWorkRound
	// ScopePreview 仅封存只读需求/观察；模型 purpose 被 SM-2 接受前不是 effect round。
	ScopePreview   *RuntimeImplementationWorkScopePreview
	PurposeChoices []RuntimeImplementationWorkPurposeChoice
}

type RuntimeImplementationWorkScopePreview struct {
	Request     RuntimeImplementationWorkScopeRequest
	Observation *RuntimeImplementationWorkScopeObservation
}

type RuntimeImplementationWorkPurpose string

const (
	RuntimeImplementationWorkPurposeEffect   RuntimeImplementationWorkPurpose = "implementation_effect"
	RuntimeImplementationWorkPurposeTerminal RuntimeImplementationWorkPurpose = "terminal_proof"
)

// RuntimeImplementationWorkPurposeChoice 只引用原 accepted semantic order，不复制 next action。
// terminal intent 只开启原终验合同的收敛，不能授 credit 或绕过原 prerequisites。
type RuntimeImplementationWorkPurposeChoice struct {
	CoverageWorkItemID   common.CoverageWorkItemID
	ScopeRequestID       string
	SemanticManifestHash string
	LogicalRequestID     string
	DocumentHash         string
	Purpose              RuntimeImplementationWorkPurpose
}

func (p RuntimeImplementationWorkScopePreview) Clone() RuntimeImplementationWorkScopePreview {
	p.Request = p.Request.Clone()
	if p.Observation != nil {
		copy := p.Observation.Clone()
		p.Observation = &copy
	}
	return p
}

// RuntimeImplementationWorkRound reserves one original CWI round. The exact
// scope request is persisted before the owner query; a missing observation or
// binding never grants an operation. Completion is a reference to the existing
// same-dispatch receipt ledger, not a terminal proof or permission to continue.
type RuntimeImplementationWorkRound struct {
	WorkspaceID             common.WorkspaceID
	RunID                   common.RunID
	StartupInputHash        string
	DefinitionSeedHash      string
	CoverageWorkItemID      common.CoverageWorkItemID
	PolicyHash              string
	Ordinal                 int
	PrecedingReceiptSetHash string
	ScopeRequest            RuntimeImplementationWorkScopeRequest
	ScopeObservation        *RuntimeImplementationWorkScopeObservation
	WorkingBinding          *RuntimeImplementationWorkingBinding
	DispatchID              string
	DispatchAuthorityHash   string
	CompletedReceiptSetHash string
	// GovernanceReceiptHash 只挂起原 dispatch；不完成或失败、不占新轮。
	GovernanceReceiptHash string
	GovernanceDispatchIDs []string
	// OutcomeUnknownExecutionKey/Fence 绑定无 receipt 的原 physical claim，不推断 effect。
	OutcomeUnknownExecutionKey string
	OutcomeUnknownFence        uint64
	// FailedReceiptHash 绑定不可续轮的原 owner receipt；Disposition 由该原文独立推导。
	FailedReceiptHash  string
	ReceiptDisposition RuntimeImplementationWorkReceiptDisposition
	Hash               string
}

// RuntimeImplementationWorkingBinding is deliberately NOT a
// CoverageEvidenceBinding. It cannot satisfy terminal evidence, replace the
// immutable Definition or authorize unscoped input maintenance/confirmed repair.
// Its optional InputRevision is confined to the original rejected review. A
// consumer must seal it through the semantic proposal/owner feasibility private
// transaction and copy its hash into the candidate manifest and dispatch.
type RuntimeImplementationWorkingBinding struct {
	InputRevision        *RuntimeValidationInputRevision `json:",omitempty"`
	SemanticGenerationID string
	BindingGeneration    uint64
	SemanticManifestHash string
	LogicalRequestID     string
	Proposals            []RuntimeSemanticProofProposal
	Feasibilities        []RuntimeSemanticProofFeasibility
	OwnerCatalog         RuntimeOperationAdmissibility
	CatalogObservation   RuntimePrerequisiteObservation
	// These complete owner preimages only protect input targets. They never
	// add selectable operations or replace the working resource owner's scope.
	MutationInputCatalogs []RuntimeOperationAdmissibility
	ExcludedTargetRefs    []string
	Hash                  string
}

// RuntimeImplementationWorkLedgerSources are independently loaded immutable
// startup inputs, not fields copied back out of the work ledger under check.
// Current resources are deliberately absent: retired rounds retain their exact
// owner preimages; freshness of an active round needs a separate current-source
// check before its private transaction can seal or dispatch.
type RuntimeImplementationWorkLedgerSources struct {
	WorkspaceID      common.WorkspaceID
	RunID            common.RunID
	StartupInputHash string
	Seed             RuntimeDefinitionSeed
	Focuses          []RuntimeDefinitionFocus
	Profiles         []RuntimeProfileContract
	MaxRecords       int
	// ReceiptSets must be projected independently from the original dispatch,
	// owner receipt and evidence ledgers, not synthesized from round labels.
	ReceiptSets []RuntimeImplementationWorkReceiptSet
}

type RuntimeImplementationWorkReceiptSet struct {
	DispatchID string
	Members    []RuntimeImplementationWorkReceiptMember
}

func (l RuntimeImplementationWorkLedger) Clone() RuntimeImplementationWorkLedger {
	l.Rounds = cloneRuntimeStateRawSlice(l.Rounds, func(r RuntimeImplementationWorkRound) RuntimeImplementationWorkRound { return r.Clone() })
	l.PurposeChoices = slices.Clone(l.PurposeChoices)
	if l.ScopePreview != nil {
		copy := l.ScopePreview.Clone()
		l.ScopePreview = &copy
	}
	return l
}

func (r RuntimeImplementationWorkRound) Clone() RuntimeImplementationWorkRound {
	r.GovernanceDispatchIDs = slices.Clone(r.GovernanceDispatchIDs)
	r.ScopeRequest = r.ScopeRequest.Clone()
	if r.ScopeObservation != nil {
		o := r.ScopeObservation.Clone()
		r.ScopeObservation = &o
	}
	if r.WorkingBinding != nil {
		b := r.WorkingBinding.Clone()
		r.WorkingBinding = &b
	}
	return r
}

func (b RuntimeImplementationWorkingBinding) Clone() RuntimeImplementationWorkingBinding {
	b.InputRevision = b.InputRevision.Clone()
	b.Proposals = cloneRuntimeStateRawSlice(b.Proposals, cloneRuntimeSemanticProofProposalRaw)
	b.Feasibilities = cloneRuntimeStateRawSlice(b.Feasibilities, cloneRuntimeSemanticProofFeasibilityRaw)
	b.OwnerCatalog = b.OwnerCatalog.Clone()
	b.CatalogObservation.Dependencies = slices.Clone(b.CatalogObservation.Dependencies)
	b.MutationInputCatalogs = cloneRuntimeStateRawSlice(b.MutationInputCatalogs, cloneRuntimeOperationAdmissibilityRaw)
	b.ExcludedTargetRefs = slices.Clone(b.ExcludedTargetRefs)
	return b
}

func StableRuntimeImplementationWorkingBindingHash(b RuntimeImplementationWorkingBinding) string {
	b.Hash = ""
	return implementationWorkRawHash("implementation-working-binding:", b)
}

func StableRuntimeImplementationWorkRoundHash(r RuntimeImplementationWorkRound) string {
	r.Hash = ""
	return implementationWorkRawHash("implementation-work-round:", r)
}

func implementationWorkRawHash(prefix string, value any) string {
	return runtimeJSONValueHash(prefix, value)
}

func (r RuntimeImplementationWorkRound) Validate() error {
	return r.validateInSemanticSession(nil)
}

func (r RuntimeImplementationWorkRound) validateInSemanticSession(session *runtimeSemanticValidationSession) error {
	if session != nil {
		for _, prior := range session.workRounds {
			if reflect.DeepEqual(prior, r) {
				return nil
			}
		}
	}
	if err := r.validateCompleteInSemanticSession(session); err != nil {
		return err
	}
	if session != nil {
		// Only a complete successful gate may contribute an exact detached raw
		// preimage. Hash equality alone, normalization and failed gates cannot.
		session.workRounds = append(session.workRounds, r.Clone())
	}
	return nil
}

func (r RuntimeImplementationWorkRound) validateCompleteInSemanticSession(session *runtimeSemanticValidationSession) error {
	for _, ref := range []string{string(r.WorkspaceID), string(r.RunID), r.StartupInputHash, r.DefinitionSeedHash, string(r.CoverageWorkItemID), r.PolicyHash} {
		if !implementationWorkExactRef(ref) {
			return fmt.Errorf("implementation work round requires exact bounded startup/CWI identity")
		}
	}
	request := r.ScopeRequest
	if err := request.Validate(); err != nil {
		return err
	}
	if !reflect.DeepEqual(request.Policy, request.Policy.Normalized()) || !reflect.DeepEqual(request.ManifestBinding, request.ManifestBinding.Normalized()) {
		return fmt.Errorf("implementation work round policy/manifest must be raw-canonical")
	}
	if r.WorkspaceID != request.WorkspaceID || r.RunID != request.RunID || r.DefinitionSeedHash != request.DefinitionSeedHash || r.CoverageWorkItemID != request.CoverageWorkItemID ||
		r.PolicyHash != StableRuntimeImplementationWorkPolicyHash(request.Policy) {
		return fmt.Errorf("implementation work round scope/policy source mismatch")
	}
	if r.Ordinal < 1 || r.Ordinal > request.Policy.MaxRoundsPerCWI || (r.Ordinal == 1) != (r.PrecedingReceiptSetHash == "") {
		return fmt.Errorf("implementation work round ordinal/preceding receipt lineage is invalid")
	}
	if r.PrecedingReceiptSetHash != "" && !implementationWorkExactRef(r.PrecedingReceiptSetHash) {
		return fmt.Errorf("implementation work preceding receipt set must be exact")
	}
	if r.ScopeObservation != nil {
		if err := r.ScopeObservation.ValidateForRequest(request); err != nil {
			return err
		}
		if !reflect.DeepEqual(r.ScopeObservation.Selection, r.ScopeObservation.Selection.Normalized()) ||
			!reflect.DeepEqual(r.ScopeObservation.Observation, r.ScopeObservation.Observation.Normalized()) {
			return fmt.Errorf("implementation work observation must retain canonical complete owner facts")
		}
	}
	if r.WorkingBinding != nil {
		if r.ScopeObservation == nil {
			return fmt.Errorf("implementation working binding has no scope observation")
		}
		if err := r.WorkingBinding.validateForRoundInSemanticSession(r, session); err != nil {
			return err
		}
	}
	if (r.DispatchID == "") != (r.DispatchAuthorityHash == "") || (r.DispatchID != "" && r.WorkingBinding == nil) {
		return fmt.Errorf("implementation work dispatch requires exact sealed working binding")
	}
	for _, ref := range []string{r.DispatchID, r.DispatchAuthorityHash, r.CompletedReceiptSetHash, r.FailedReceiptHash} {
		if ref != "" && !implementationWorkExactRef(ref) {
			return fmt.Errorf("implementation work dispatch/receipt identity must be exact")
		}
	}
	if (r.OutcomeUnknownExecutionKey == "") != (r.OutcomeUnknownFence == 0) || r.OutcomeUnknownExecutionKey != "" && (r.DispatchID == "" || !implementationWorkExactRef(r.OutcomeUnknownExecutionKey) || r.CompletedReceiptSetHash != "" || r.FailedReceiptHash != "" || r.GovernanceReceiptHash != "") {
		return fmt.Errorf("implementation unknown outcome requires original unsettled physical identity")
	}
	if r.CompletedReceiptSetHash != "" && r.FailedReceiptHash != "" {
		return fmt.Errorf("implementation work cannot be both collectively complete and failed")
	}
	if r.FailedReceiptHash != "" && r.DispatchID == "" {
		return fmt.Errorf("implementation work failure has no original dispatch")
	}
	if (r.FailedReceiptHash == "") != (r.ReceiptDisposition == "") || (r.ReceiptDisposition != "" && !validImplementationWorkReceiptDisposition(r.ReceiptDisposition)) {
		return fmt.Errorf("implementation work closed receipt requires exact typed disposition")
	}
	if r.CompletedReceiptSetHash != "" && r.DispatchID == "" {
		return fmt.Errorf("implementation work completion has no dispatch commitment")
	}
	if len(r.GovernanceDispatchIDs) == 0 && r.GovernanceDispatchIDs != nil {
		return fmt.Errorf("implementation governance history must preserve canonical absence")
	}
	if r.GovernanceReceiptHash != "" && (r.DispatchID == "" || !implementationWorkExactRef(r.GovernanceReceiptHash) || r.CompletedReceiptSetHash != "" || r.FailedReceiptHash != "") || len(r.GovernanceDispatchIDs) > MaxRuntimeImplementationWorkRounds {
		return fmt.Errorf("implementation governance suspension must retain one outstanding bounded round")
	}
	seenGovernanceDispatches := map[string]bool{}
	for _, id := range r.GovernanceDispatchIDs {
		if !implementationWorkExactRef(id) || id == r.DispatchID || seenGovernanceDispatches[id] {
			return fmt.Errorf("implementation governance dispatch history is invalid")
		}
		seenGovernanceDispatches[id] = true
	}
	if r.Hash == "" || r.Hash != StableRuntimeImplementationWorkRoundHash(r) {
		return fmt.Errorf("implementation work round hash mismatch")
	}
	return nil
}

func (b RuntimeImplementationWorkingBinding) ValidateForRound(r RuntimeImplementationWorkRound) error {
	return b.validateForRoundInSemanticSession(r, nil)
}

func (b RuntimeImplementationWorkingBinding) validateForRoundInSemanticSession(r RuntimeImplementationWorkRound, session *runtimeSemanticValidationSession) error {
	for _, ref := range []string{b.SemanticGenerationID, b.SemanticManifestHash, b.LogicalRequestID} {
		if !implementationWorkExactRef(ref) {
			return fmt.Errorf("implementation working binding requires exact semantic request identity")
		}
	}
	if b.BindingGeneration == 0 || r.ScopeObservation == nil || b.SemanticGenerationID != StableRuntimeSemanticGenerationID(b.SemanticManifestHash, b.LogicalRequestID) {
		return fmt.Errorf("implementation working binding requires generation and complete scope observation")
	}
	selection := r.ScopeObservation.Selection
	if selection.Disposition != RuntimeTargetSelectionSelected && selection.Disposition != RuntimeTargetSelectionScope {
		return fmt.Errorf("implementation working binding cannot use unresolved/unsupported scope")
	}
	catalog := b.OwnerCatalog
	validatedCatalog, err := validateWorkRoundCatalogInSession(catalog, session)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(catalog, validatedCatalog) || catalog.OwnerModuleID != r.ScopeRequest.Policy.OwnerModuleID || catalog.ObservedRevision != selection.ObservedRevision {
		return fmt.Errorf("implementation working binding requires exact current resource owner catalog")
	}
	if err := b.CatalogObservation.Validate(); err != nil {
		return err
	}
	observation := b.CatalogObservation
	if !reflect.DeepEqual(observation, observation.Normalized()) || observation.Kind != PrerequisiteKindOperationAdmissibility || observation.OwnerModuleID != catalog.OwnerModuleID ||
		observation.PrerequisiteID != catalog.PrerequisiteID || observation.ContentHash != catalog.SnapshotID ||
		!runtimeObservationDependsOnWorkspaceTreeRevision(observation, catalog.OwnerModuleID, selection.ObservedRevision) {
		return fmt.Errorf("implementation working binding catalog observation has no exact scope revision preimage")
	}
	if len(b.Proposals) == 0 || len(b.Proposals) > r.ScopeRequest.Policy.MaxTargetsPerRound || len(b.Proposals) != len(b.Feasibilities) {
		return fmt.Errorf("implementation working binding requires bounded complete proposal/feasibility set")
	}
	if err := validateWorkRoundInputCatalogsInSession(b.MutationInputCatalogs, session); err != nil {
		return err
	}
	for _, inputCatalog := range b.MutationInputCatalogs {
		if !reflect.DeepEqual(inputCatalog, inputCatalog.Normalized()) {
			return fmt.Errorf("implementation work input protection must retain raw-canonical catalog")
		}
	}
	if !reflect.DeepEqual(b.InputRevision, r.ScopeRequest.InputRevision) {
		return fmt.Errorf("working revision differs from original scope")
	}
	excluded := runtimeWorkInputExclusions(catalog.OwnerModuleID, b.MutationInputCatalogs, b.InputRevision)
	if !reflect.DeepEqual(excluded, b.ExcludedTargetRefs) {
		return fmt.Errorf("implementation work input exclusions do not match complete owner preimages")
	}
	seen := map[string]bool{}
	for index, proposal := range b.Proposals {
		if err := proposal.Validate(); err != nil {
			return err
		}
		if !reflect.DeepEqual(proposal, proposal.Normalized()) || proposal.CoverageWorkItemID != r.CoverageWorkItemID || proposal.OwnerModuleID != catalog.OwnerModuleID ||
			!slices.Equal(proposal.RequiredEvidenceKinds, []string{EvidenceKindStateChange}) || !slices.Equal(proposal.RequiredEffectScopes, []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}) ||
			proposal.ValidationRecipeID != "" || len(proposal.RequiredCheckRefs) != 0 || proposal.ValidationInputReview != nil {
			return fmt.Errorf("implementation work proposal cannot carry terminal validation/input-maintenance proof")
		}
		if !slices.Contains(r.ScopeRequest.Policy.AllowedOperations, RuntimeOperationVariantRef{Kind: proposal.OperationKind, VariantID: proposal.OperationVariantID}) ||
			!slices.Contains(r.ScopeRequest.Policy.AllowedTargetPolicies, proposal.Target.Policy) {
			return fmt.Errorf("implementation work proposal exceeds sealed operation/variant/policy declaration")
		}
		if err := validateSemanticProposalAgainstOwnerCatalog(proposal, catalog); err != nil {
			return err
		}
		targets := RuntimeSemanticProposalTargetRefs(proposal)
		if err := validateRuntimeInputRevisionTargets(b.InputRevision, proposal); err != nil {
			return err
		}
		// The first value contract requires exact, non-overlapping targets; an
		// owner selector is observation vocabulary, never effect authorization.
		if len(targets) == 0 || len(targets) > r.ScopeRequest.Policy.MaxTargetsPerRound {
			return fmt.Errorf("implementation work proposal requires bounded exact targets")
		}
		for _, target := range targets {
			if !implementationWorkExactRef(target) || seen[target] || slices.Contains(excluded, target) {
				return fmt.Errorf("implementation work target duplicates or mutates protected validation input")
			}
			seen[target] = true
			policy := r.ScopeRequest.Policy
			if len(policy.TargetSelectorCandidateRefs) != 0 && !slices.Contains(policy.TargetSelectorCandidateRefs, target) {
				return fmt.Errorf("implementation work target is outside declared work selector candidates")
			}
			if proposal.Target.Policy != RuntimeOperationTargetPolicyProspectiveOwnerTarget && !slices.Contains(selection.TargetRefs, target) {
				return fmt.Errorf("implementation work target is absent from complete owner work scope")
			}
		}
		// The existing hypothetical delta is specific to prospective OWNER
		// FACT satisfaction. Ordinary implementation work must not borrow that
		// closure or its second owner's judgment as work authorization.
		if b.Feasibilities[index].ProspectiveWorkspaceManifestDelta != nil {
			return fmt.Errorf("implementation work cannot borrow owner-fact prospective satisfaction delta")
		}
		if proposal.Target.Policy == RuntimeOperationTargetPolicyProspectiveOwnerTarget && !SameRuntimeProspectiveTargetAllOfConstraintRefs(proposal.Target.ProspectiveConstraintRefs, r.ScopeRequest.Policy.ProspectiveTargetAllOfConstraintRefs) {
			return fmt.Errorf("implementation work prospective constraints differ from sealed all-of declaration")
		}
		fact := b.Feasibilities[index]
		if !reflect.DeepEqual(fact, fact.Normalized()) || fact.SchemaVersion != RuntimeSemanticFeasibilitySchemaCurrent || fact.Status != RuntimeSemanticFeasibilityFeasible ||
			fact.ManifestHash != b.SemanticManifestHash || fact.DefinitionSeedHash != r.DefinitionSeedHash || fact.CoverageWorkItemID != r.CoverageWorkItemID ||
			fact.ProposalHash != StableRuntimeSemanticProofProposalHash(proposal) || fact.OwnerModuleID != catalog.OwnerModuleID || fact.CatalogSnapshotID != catalog.SnapshotID ||
			fact.CatalogObservationID != observation.ObservationID || fact.ObservedRevision != catalog.ObservedRevision || !slices.Equal(fact.TargetRefs, targets) ||
			fact.ValidationRecipeID != "" || len(fact.RequiredCheckRefs) != 0 || fact.OwnerFactProspectiveSatisfactionObservation != nil || fact.OriginalDiagnostic != nil {
			return fmt.Errorf("implementation work feasibility differs from exact proposal/catalog/source")
		}
	}
	if len(seen) > r.ScopeRequest.Policy.MaxTargetsPerRound {
		return fmt.Errorf("implementation work collective target set exceeds sealed round limit")
	}
	if b.Hash == "" || b.Hash != StableRuntimeImplementationWorkingBindingHash(b) {
		return fmt.Errorf("implementation working binding hash mismatch")
	}
	return nil
}

func (l RuntimeImplementationWorkLedger) ValidateForSources(s RuntimeImplementationWorkLedgerSources) error {
	return l.validateForSourcesInSemanticSession(s, newRuntimeSemanticValidationSession())
}

func (l RuntimeImplementationWorkLedger) validateForSourcesInSemanticSession(s RuntimeImplementationWorkLedgerSources, session *runtimeSemanticValidationSession) error {
	if !implementationWorkExactRef(s.StartupInputHash) || !implementationWorkExactRef(string(s.WorkspaceID)) || !implementationWorkExactRef(string(s.RunID)) || s.MaxRecords < 1 || len(l.Rounds) > s.MaxRecords {
		return fmt.Errorf("implementation work ledger requires bounded exact startup sources")
	}
	if err := validateRuntimeWorkDefinitionSourcesInSession(s.Seed, s.Focuses, s.Profiles, session); err != nil {
		return err
	}
	previous := map[common.CoverageWorkItemID]RuntimeImplementationWorkRound{}
	requests, bindings, dispatches := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for index, r := range l.Rounds {
		if err := r.validateInSemanticSession(session); err != nil {
			return fmt.Errorf("implementation work round %d: %w", index, err)
		}
		if r.WorkspaceID != s.WorkspaceID || r.RunID != s.RunID || r.StartupInputHash != s.StartupInputHash || r.DefinitionSeedHash != s.Seed.Hash {
			return fmt.Errorf("implementation work ledger round escapes sealed startup")
		}
		scopeSources := RuntimeImplementationWorkScopeSources{WorkspaceID: s.WorkspaceID, RunID: s.RunID, AuthoritySnapshotID: r.ScopeRequest.AuthoritySnapshotID,
			Seed: s.Seed, Focuses: s.Focuses, Profiles: s.Profiles, CoverageWorkItemID: r.CoverageWorkItemID, ManifestBinding: r.ScopeRequest.ManifestBinding, InputRevision: r.ScopeRequest.InputRevision}
		if err := validateRuntimeWorkScopeInSession(r.ScopeRequest, scopeSources, session); err != nil {
			return err
		}
		if r.WorkingBinding != nil {
			for _, proposal := range r.WorkingBinding.Proposals {
				matched := false
				for _, focus := range s.Focuses {
					if len(focus.CoverageWorkItemIDs) == 1 && focus.CoverageWorkItemIDs[0] == r.CoverageWorkItemID && proposal.DeclarationRef == focus.EvidenceDeclarationRef {
						matched = true
					}
				}
				if !matched {
					return fmt.Errorf("implementation work proposal does not preserve original exact CWI declaration")
				}
			}
		}
		if r.CompletedReceiptSetHash != "" {
			matches := 0
			for _, set := range s.ReceiptSets {
				if set.DispatchID == r.DispatchID {
					matches++
					if _, err := validateRuntimeImplementationWorkReceiptSetInSession(r, set.Members, session); err != nil {
						return err
					}
				}
			}
			if matches != 1 {
				return fmt.Errorf("implementation work completion has no unique independent dispatch receipt source")
			}
		}
		prior, found := previous[r.CoverageWorkItemID]
		if (!found && r.Ordinal != 1) || (found && (r.Ordinal != prior.Ordinal+1 || prior.CompletedReceiptSetHash == "" || r.PrecedingReceiptSetHash != prior.CompletedReceiptSetHash || r.PolicyHash != prior.PolicyHash)) {
			return fmt.Errorf("implementation work round does not follow durable same-CWI receipt lineage")
		}
		if found {
			if r.ScopeRequest.ManifestBinding.Manifest.Revision == prior.ScopeRequest.ManifestBinding.Manifest.Revision || r.ScopeRequest.AuthoritySnapshotID == prior.ScopeRequest.AuthoritySnapshotID {
				return fmt.Errorf("implementation work successor requires fresh owner scope and authority")
			}
			if r.WorkingBinding != nil && prior.WorkingBinding != nil && (r.WorkingBinding.BindingGeneration <= prior.WorkingBinding.BindingGeneration || r.WorkingBinding.LogicalRequestID == prior.WorkingBinding.LogicalRequestID) {
				return fmt.Errorf("implementation work successor requires new semantic/working binding generation")
			}
		}
		if requests[r.ScopeRequest.RequestID] || (r.WorkingBinding != nil && bindings[r.WorkingBinding.Hash]) || (r.DispatchID != "" && dispatches[r.DispatchID]) {
			return fmt.Errorf("implementation work ledger reuses scope/binding/dispatch identity")
		}
		requests[r.ScopeRequest.RequestID] = true
		if r.WorkingBinding != nil {
			bindings[r.WorkingBinding.Hash] = true
		}
		if r.DispatchID != "" {
			dispatches[r.DispatchID] = true
		}
		previous[r.CoverageWorkItemID] = r
	}
	return nil
}

// RuntimeImplementationWorkReceiptMember is an exact join to a receipt already
// persisted by its capability owner and promoted by Orchestrator. It carries
// source explicitly because a self-hashed receipt cannot attest its producer.
// No receipt is copied into the work ledger itself.
type RuntimeImplementationWorkReceiptMember struct {
	OwnerModuleID string
	DispatchID    string
	Execution     RuntimeOperationExecutionRecord
	Operation     RuntimeOperation
	Receipt       RuntimeOperationReceipt
	Evidence      EvidenceFact
}

// ValidateRuntimeImplementationWorkReceiptSet checks collective durable work
// completion. It returns a lineage hash only: never CWI credit, validation proof,
// a next round, or recovery authorization. The caller still has to reverse-prove
// these detached members against the original sealed dispatch and evidence
// ledger in the SAME private aggregate transaction.
func ValidateRuntimeImplementationWorkReceiptSet(r RuntimeImplementationWorkRound, members []RuntimeImplementationWorkReceiptMember) (string, error) {
	return validateRuntimeImplementationWorkReceiptSetInSession(r, members, nil)
}

func validateRuntimeImplementationWorkReceiptSetInSession(r RuntimeImplementationWorkRound, members []RuntimeImplementationWorkReceiptMember, session *runtimeSemanticValidationSession) (string, error) {
	return validateImplementationWorkReceiptMembers(r, members, session, true)
}

func validateImplementationWorkReceiptMembers(r RuntimeImplementationWorkRound, members []RuntimeImplementationWorkReceiptMember, session *runtimeSemanticValidationSession, complete bool) (string, error) {
	if err := r.validateInSemanticSession(session); err != nil {
		return "", err
	}
	if r.DispatchID == "" || r.WorkingBinding == nil || len(members) == 0 || len(members) > r.ScopeRequest.Policy.MaxTargetsPerRound {
		return "", fmt.Errorf("implementation work completion requires one bounded sealed dispatch")
	}
	b := *r.WorkingBinding
	want := map[string]RuntimeSemanticProofProposal{}
	for _, proposal := range b.Proposals {
		for _, target := range RuntimeSemanticProposalTargetRefs(proposal) {
			want[target] = proposal
		}
	}
	if len(members) > len(want) || complete && len(members) != len(want) {
		return "", fmt.Errorf("implementation work cannot complete a partial collective receipt set")
	}
	type commitment struct {
		Target, DispatchID, OperationID, ExecutionKey, ReceiptRef, ReceiptHash, EvidenceHash string
		Fence                                                                                uint64
	}
	joined := make([]commitment, 0, len(members))
	seen, keys := map[string]bool{}, map[string]bool{}
	for _, member := range members {
		op, execution, receipt := member.Operation, member.Execution, member.Receipt
		if member.OwnerModuleID != r.ScopeRequest.Policy.OwnerModuleID {
			return "", fmt.Errorf("implementation work receipt came from wrong/noncanonical owner")
		}
		if err := execution.Validate(); err != nil {
			return "", err
		}
		if err := receipt.Validate(); err != nil {
			return "", err
		}
		if !reflect.DeepEqual(op, op.Normalized()) || !reflect.DeepEqual(receipt, receipt.Normalized()) || len(op.TargetRefs) != 1 || !slices.Equal(op.TargetRefs, receipt.TargetRefs) {
			return "", fmt.Errorf("implementation work receipt requires exact singleton operation target")
		}
		target := op.TargetRefs[0]
		proposal, found := want[target]
		if !found || seen[target] || keys[execution.ExecutionKey] {
			return "", fmt.Errorf("implementation work receipt is extra/duplicate/cross-round target")
		}
		seen[target], keys[execution.ExecutionKey] = true, true
		descriptor, known := RuntimeOperationDescriptorForKind(op.Kind)
		if !known || descriptor.OwnerModuleID != member.OwnerModuleID || op.Kind != proposal.OperationKind {
			return "", fmt.Errorf("implementation work receipt operation does not match sealed working proposal")
		}
		contract, known := RuntimeOperationContractForKind(op.Kind)
		if !known {
			return "", fmt.Errorf("implementation work receipt has no executable operation contract")
		}
		variantID := ""
		if len(contract.Variants) != 0 {
			variant, err := MatchRuntimeOperationVariant(op, contract)
			if err != nil {
				return "", err
			}
			variantID = variant.ID
		}
		if variantID != proposal.OperationVariantID || len(op.BindingRefs) != 0 {
			return "", fmt.Errorf("implementation work receipt cannot borrow terminal/repair coverage binding")
		}
		originDispatchID := ""
		for _, id := range append(slices.Clone(r.GovernanceDispatchIDs), r.DispatchID) {
			if execution.ExecutionKey == StableRuntimeOperationExecutionKey(r.WorkspaceID, r.RunID, id, op.ID) {
				originDispatchID = id
			}
		}
		effectDispatchID := member.DispatchID
		// 来源 dispatch 由 aggregate projector 独立取得；原 key 可跨治理 hop 保留，不能代替 effect 来源。
		if originDispatchID == "" || effectDispatchID != r.DispatchID && !slices.Contains(r.GovernanceDispatchIDs, effectDispatchID) || member.Evidence.DispatchID != effectDispatchID {
			return "", fmt.Errorf("implementation receipt has no original authorized dispatch hop")
		}
		if execution.OperationID != op.ID || execution.OperationHash != StableRuntimeOperationHash(op) || execution.ExecutionKey != StableRuntimeOperationExecutionKey(r.WorkspaceID, r.RunID, originDispatchID, op.ID) ||
			execution.State != OperationExecutionEvidenceCommitted || execution.ExpectedEffectScope != RuntimeEffectWorkspaceDurable || execution.OwnerReceiptHash != StableRuntimeOperationReceiptHash(receipt) ||
			execution.AdmissibilitySnapshotID != b.OwnerCatalog.SnapshotID || execution.AcquisitionBinding != b.OwnerCatalog.AcquisitionBinding || receipt.ExecutionKey != execution.ExecutionKey ||
			receipt.Fence != execution.Fence || receipt.OperationID != op.ID || receipt.Outcome != RuntimeOperationOutcomeAccepted || receipt.ObservedEffectScope != RuntimeEffectWorkspaceDurable || !receipt.Changed ||
			receipt.BeforeHash == receipt.AfterHash || receipt.AfterHash == "" || receipt.Evidence.ZeroWork || !runtimeImplementationWorkEffectQualityAcceptable(receipt.Evidence.Quality) || receipt.Evidence.Validation != nil || receipt.PrerequisiteStale != nil {
			return "", fmt.Errorf("implementation work receipt is partial/stale/no-op/wrong-fence or not durable exact owner evidence")
		}
		evidence := member.Evidence
		if evidence.ID != execution.EvidenceFactID || StableRuntimeEvidenceFactHash(evidence) != execution.EvidenceFactHash || evidence.Kind != EvidenceKindStateChange ||
			evidence.DispatchID != effectDispatchID || evidence.OperationID != op.ID || evidence.SourceRef != execution.OwnerReceiptRef || !slices.Equal(evidence.TargetRefs, op.TargetRefs) ||
			len(evidence.CoverageWorkItemIDs) != 0 || len(evidence.BindingRefs) != 0 || evidence.Validation != nil || !runtimeImplementationWorkEffectQualityAcceptable(evidence.Quality) || evidence.Quality != receipt.Evidence.Quality || evidence.ZeroWork ||
			evidence.ContentHash != receipt.Evidence.ContentHash || !slices.Equal(evidence.EvidenceRefs, receipt.Evidence.EvidenceRefs) || evidence.EvidenceReason != receipt.Evidence.EvidenceReason ||
			!reflect.DeepEqual(evidence.OwnerRevisionSet, receipt.Evidence.OwnerRevisionSet) ||
			!reflect.DeepEqual(evidence.ApplicabilityDependencies, receipt.Evidence.ApplicabilityDependencies) ||
			!reflect.DeepEqual(evidence.ContentAddress, receipt.Evidence.ContentAddress) || !reflect.DeepEqual(evidence.ExitCode, receipt.Evidence.ExitCode) ||
			!slices.Equal(evidence.EvidenceTags, receipt.Evidence.EvidenceTags) || evidence.Summary != receipt.Evidence.Summary || evidence.FailureClass != "" || evidence.FailureCode != "" {
			return "", fmt.Errorf("implementation work effect evidence cannot grant coverage/validation or drift from exact receipt")
		}
		joined = append(joined, commitment{Target: target, DispatchID: effectDispatchID, OperationID: op.ID, ExecutionKey: execution.ExecutionKey, Fence: execution.Fence,
			ReceiptRef: execution.OwnerReceiptRef, ReceiptHash: execution.OwnerReceiptHash, EvidenceHash: execution.EvidenceFactHash})
	}
	{
		chain := newImplementationWorkResourceChain(b.CatalogObservation.Dependencies)
		for _, member := range members {
			if !chain.advance(member.Receipt) {
				return "", fmt.Errorf("implementation work receipts lack an exact ordered resource revision chain")
			}
		}
	}
	slices.SortFunc(joined, func(a, b commitment) int { return strings.Compare(a.Target, b.Target) })
	hash := implementationWorkRawHash("implementation-work-receipts:", struct {
		WorkspaceID                                    common.WorkspaceID
		RunID                                          common.RunID
		StartupInputHash, DefinitionSeedHash           string
		CoverageWorkItemID                             common.CoverageWorkItemID
		Ordinal                                        int
		BindingHash, DispatchID, DispatchAuthorityHash string
		Members                                        []commitment
	}{r.WorkspaceID, r.RunID, r.StartupInputHash, r.DefinitionSeedHash, r.CoverageWorkItemID, r.Ordinal, b.Hash, r.DispatchID, r.DispatchAuthorityHash, joined})
	if r.CompletedReceiptSetHash != "" && r.CompletedReceiptSetHash != hash {
		return "", fmt.Errorf("implementation work completion hash differs from collective exact owner receipts")
	}
	return hash, nil
}

func implementationWorkExactRef(ref string) bool {
	return validRuntimeImplementationWorkRef(ref) && strings.TrimSpace(ref) == ref
}
