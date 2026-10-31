package workorchruntimecontract

import (
	"fmt"
	"reflect"
	"slices"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeHistoricalBehaviorCompletion records only a collectively completed
// historical effect. The canonical CWI remains bound/open until its separately
// confirmed ordinary acceptance consumer proves every original obligation.
// This is SM-2 progress, not another coverage or execution ledger.
type RuntimeHistoricalBehaviorCompletion struct {
	Source       RuntimeHistoricalAcceptanceSource
	BindingRef   RuntimeCWIBindingRef
	DispatchID   string
	EvidenceRefs []string
}

func cloneRuntimeHistoricalBehaviorCompletion(value *RuntimeHistoricalBehaviorCompletion) *RuntimeHistoricalBehaviorCompletion {
	if value == nil {
		return nil
	}
	ret := *value
	ret.EvidenceRefs = slices.Clone(value.EvidenceRefs)
	return &ret
}

// CompileRuntimeHistoricalBehaviorCompletion independently reconstructs the
// exact source, complete dispatch, owner receipts and evidence member set. It
// never unions partial dispatches or consumes current catalogs as history.
func CompileRuntimeHistoricalBehaviorCompletion(sources RuntimeLineageAssertionSources, historicalID common.CoverageWorkItemID, dispatchID string) (*RuntimeHistoricalBehaviorCompletion, error) {
	return compileRuntimeHistoricalBehaviorCompletion(sources, historicalID, dispatchID, true)
}

func compileRuntimeHistoricalBehaviorCompletion(sources RuntimeLineageAssertionSources, historicalID common.CoverageWorkItemID, dispatchID string, requireOpen bool) (*RuntimeHistoricalBehaviorCompletion, error) {
	return compileRuntimeHistoricalBehaviorInSession(sources, historicalID, dispatchID, requireOpen, &runtimeHistoricalPreimageSession{})
}

func compileRuntimeHistoricalBehaviorInSession(sources RuntimeLineageAssertionSources, historicalID common.CoverageWorkItemID, dispatchID string, requireOpen bool, session *runtimeHistoricalPreimageSession) (*RuntimeHistoricalBehaviorCompletion, error) {
	invalid := func() (*RuntimeHistoricalBehaviorCompletion, error) {
		return nil, fmt.Errorf("historical behavior requires original confirmed mapping and complete exact accepted effect lineage")
	}
	endpoints, err := ResolveRuntimeEvidencePurposeEndpoints(sources.Definition, sources.Facts.Activation.ExecutionContract, sources.Facts.DefinitionSeed, historicalID)
	if err != nil {
		return nil, err
	}
	var row RuntimeCoverageItemFact
	rows := 0
	for _, candidate := range sources.Facts.Coverage {
		if candidate.ID == historicalID {
			row, rows = candidate, rows+1
		}
	}
	if rows != 1 || row.CurrentBindingRef == nil {
		return invalid()
	}
	open := row.CWIConvergenceState == RuntimeCWIConvergenceStateBound && row.State == RuntimeCoverageItemStateOpen
	settled := row.CWIConvergenceState == RuntimeCWIConvergenceStateSettled && row.State == RuntimeCoverageItemStateSatisfied
	if !open && (requireOpen || !settled) {
		return invalid()
	}
	var dispatch RuntimeDispatchRecord
	matches := 0
	for _, candidate := range sources.Completed {
		if candidate.ID == dispatchID {
			dispatch, matches = candidate, matches+1
		}
	}
	operations := runtimeCandidateAdmissionOperations(dispatch.Candidate)
	if matches != 1 || dispatch.ID == "" || dispatch.State != RuntimeDispatchStateCommitted ||
		dispatch.WorkspaceID != sources.Facts.WorkspaceID || dispatch.RunID != sources.Facts.RunID ||
		dispatch.Transaction.DefinitionRevision != sources.Facts.DefinitionRevision || dispatch.Transaction.State != ConvergenceStateNeedStateChange ||
		dispatch.Transaction.FocusID != row.FocusID || dispatch.RepairCommitment != nil || dispatch.ImplementationWorkCommitment != nil ||
		len(operations) == 0 || len(dispatch.CompletedOperationIDs) != len(operations) {
		return invalid()
	}
	var binding CoverageEvidenceBinding
	matches = 0
	for _, candidate := range dispatch.CoverageBindings {
		if candidate.CoverageWorkItemID == historicalID {
			binding, matches = candidate, matches+1
		}
	}
	if matches != 1 || binding.Validate() != nil || binding.HistoricalAcceptance == nil ||
		binding.DeclarationRef != CanonicalRuntimeDefinitionEvidenceDeclarationRef(endpoints.Historical.SourceScope, endpoints.Historical.SourceID) ||
		!sameRuntimeHistoricalAcceptanceSource(binding.HistoricalAcceptance, endpoints.Historical.HistoricalAcceptance) ||
		!SameRuntimeCWIBindingRef(*row.CurrentBindingRef, RuntimeCWIBindingRefForCoverageBinding(binding, row.CurrentBindingRef.SemanticManifestHash)) ||
		len(binding.Proofs) != 1 || binding.Proofs[0].EvidenceKind != EvidenceKindStateChange {
		return invalid()
	}
	// Binding hashes and dispatch-local declarations cannot certify their own
	// provenance. Replay the accepted provider document and receiving-owner
	// feasibility from the original semantic generation, including after it
	// has been retired by the effect's catalog invalidation.
	entries, err := session.preimages(sources.Semantic)
	if err != nil {
		return invalid()
	}
	preimage, err := resolveRuntimeAssessmentPurposePreimageInSession(entries, dispatch.CoverageBindings, session.semanticSession())
	if err != nil || preimage.Purpose != RuntimeAssessmentPurposeDefinitionProof ||
		preimage.SemanticManifestHash != row.CurrentBindingRef.SemanticManifestHash ||
		validateRuntimeHistoricalSemanticStartupSource(sources, preimage.SemanticManifestHash) != nil {
		return invalid()
	}
	refs := make([]string, 0, len(operations))
	targets := make([]string, 0)
	seenOperations := make(map[string]bool, len(operations))
	for _, operation := range operations {
		if seenOperations[operation.ID] || !slices.Contains(dispatch.CompletedOperationIDs, operation.ID) {
			return invalid()
		}
		seenOperations[operation.ID] = true
		if !slices.Contains(operation.BindingRefs, binding.ID) {
			continue
		}
		fact, receipt, valid := runtimeLineageOperationFact(dispatch, operation, sources, true)
		if !valid || fact.FocusID != row.FocusID || fact.Kind != EvidenceKindStateChange ||
			!slices.Contains(fact.CoverageWorkItemIDs, historicalID) || !RuntimeEvidenceIsHistoricalEffect(fact) ||
			ValidateRuntimeHistoricalEffectReceiptPurpose(dispatch, operation, receipt.Evidence.ApplicabilityDependencies) != nil ||
			MatchCoverageEvidenceBindingReceipt(binding, EvidenceKindStateChange, operation, receipt) != nil {
			return invalid()
		}
		refs = append(refs, string(fact.ID))
		targets = append(targets, operation.TargetRefs...)
	}
	if len(refs) == 0 {
		return invalid()
	}
	// Every sealed prospective member must occur in this one dispatch. The
	// normal receipt matcher above still validates each singleton target.
	for _, target := range binding.ProspectiveTargetRefs {
		if count := slices.Index(targets, target); count < 0 {
			return invalid()
		}
		members := 0
		for _, actual := range targets {
			if actual == target {
				members++
			}
		}
		if members != 1 {
			return invalid()
		}
	}
	slices.Sort(refs)
	return &RuntimeHistoricalBehaviorCompletion{Source: *binding.HistoricalAcceptance, BindingRef: *row.CurrentBindingRef, DispatchID: dispatch.ID, EvidenceRefs: refs}, nil
}

// Semantic history is immutable but not an independent Definition authority.
// Its original seed/revision must still resolve to the sealed startup input.
func validateRuntimeHistoricalSemanticStartupSource(sources RuntimeLineageAssertionSources, manifestHash string) error {
	validate := func(ledger RuntimeSemanticPlanningLedger) error {
		if ledger.Manifest == nil || ledger.Manifest.Hash != manifestHash {
			return nil
		}
		if ledger.Manifest.DefinitionRevision != sources.Facts.DefinitionRevision ||
			ledger.Manifest.DefinitionSeedHash != sources.Facts.DefinitionSeed.Hash ||
			!reflect.DeepEqual(ledger.DefinitionSeed, sources.Facts.DefinitionSeed) {
			return fmt.Errorf("historical semantic source differs from sealed startup seed")
		}
		return nil
	}
	if err := validate(sources.Semantic); err != nil {
		return err
	}
	for _, generation := range sources.Semantic.AllHistory() {
		if err := validate(generation.Snapshot); err != nil {
			return err
		}
	}
	return nil
}

func ValidateRuntimeHistoricalBehaviorCompletions(sources RuntimeLineageAssertionSources) error {
	return validateRuntimeHistoricalBehaviorCompletionsInSession(sources, &runtimeHistoricalPreimageSession{})
}

func validateRuntimeHistoricalBehaviorCompletionsInSession(sources RuntimeLineageAssertionSources, session *runtimeHistoricalPreimageSession) error {
	if session != nil && session.sourceCompiler != nil {
		return session.sourceCompiler.validate(sources, false, func() error {
			return validateRuntimeHistoricalBehaviorCompletionsComplete(sources, session)
		})
	}
	return validateRuntimeHistoricalBehaviorCompletionsComplete(sources, session)
}

func validateRuntimeHistoricalBehaviorCompletionsComplete(sources RuntimeLineageAssertionSources, session *runtimeHistoricalPreimageSession) error {
	for _, row := range sources.Facts.Coverage {
		if row.HistoricalBehavior == nil {
			continue
		}
		expected, err := compileRuntimeHistoricalBehaviorInSession(sources, row.ID, row.HistoricalBehavior.DispatchID, false, session)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(row.HistoricalBehavior, expected) {
			return fmt.Errorf("historical behavior differs from original complete effect lineage")
		}
		if err := validateRuntimeHistoricalAcceptanceProofsInSession(sources, row, session); err != nil {
			return err
		}
	}
	for _, row := range sources.Facts.Coverage {
		if row.HistoricalBehavior == nil && len(row.HistoricalAcceptanceProofs) != 0 {
			return fmt.Errorf("historical acceptance lacks original behavior completion")
		}
	}
	return nil
}

// RuntimeCWIPrerequisiteReadyForConsumer preserves ordinary settled admission.
// The sole exception is original mapped historical progress for its exact
// acceptance consumer. Full aggregate admission separately replays receipts.
func RuntimeCWIPrerequisiteReadyForConsumer(facts RuntimeFacts, prerequisite RuntimeCoverageItemFact, consumer common.CoverageWorkItemID) bool {
	if prerequisite.CWIConvergenceState == RuntimeCWIConvergenceStateSettled && prerequisite.State == RuntimeCoverageItemStateSatisfied && prerequisite.CurrentBindingRef != nil {
		return true
	}
	completion := prerequisite.HistoricalBehavior
	if completion == nil || prerequisite.CWIConvergenceState != RuntimeCWIConvergenceStateBound || prerequisite.State != RuntimeCoverageItemStateOpen ||
		prerequisite.CurrentBindingRef == nil || completion.Source.AcceptanceCoverageWorkItemID != consumer ||
		!SameRuntimeCWIBindingRef(completion.BindingRef, *prerequisite.CurrentBindingRef) || completion.Source.Validate(prerequisite.ID) != nil ||
		completion.DispatchID == "" || len(completion.EvidenceRefs) == 0 || facts.DefinitionSeed.SourceObligations == nil ||
		completion.Source.InventoryHash != facts.DefinitionSeed.SourceObligations.Hash || ValidateRuntimeSourceObligationSeed(facts.DefinitionSeed) != nil {
		return false
	}
	matches := 0
	for _, item := range facts.DefinitionSeed.CoverageItems {
		if item.ID == prerequisite.ID && sameRuntimeHistoricalAcceptanceSource(item.HistoricalAcceptance, &completion.Source) {
			matches++
		}
	}
	if matches != 1 {
		return false
	}
	seen := make(map[string]bool, len(completion.EvidenceRefs))
	for _, ref := range completion.EvidenceRefs {
		if seen[ref] {
			return false
		}
		seen[ref] = true
		matches = 0
		for _, fact := range facts.Evidence.Accepted {
			if string(fact.ID) == ref && fact.DispatchID == completion.DispatchID && fact.FocusID == prerequisite.FocusID &&
				RuntimeEvidenceIsHistoricalEffect(fact) && !assessmentEvidenceWeak(fact) && slices.Contains(fact.BindingRefs, completion.BindingRef.BindingID) && slices.Contains(fact.CoverageWorkItemIDs, prerequisite.ID) {
				matches++
			}
		}
		if matches != 1 {
			return false
		}
	}
	return true
}
