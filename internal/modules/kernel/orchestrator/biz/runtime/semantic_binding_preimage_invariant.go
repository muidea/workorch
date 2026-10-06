package runtime

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

var errRuntimeSemanticBindingPreimageViolation = errors.New("runtime semantic binding preimage violation")

func runtimeSemanticBindingPreimageViolation(err error) error {
	if err == nil || errors.Is(err, errRuntimeSemanticBindingPreimageViolation) {
		return err
	}
	return fmt.Errorf("%w: %v", errRuntimeSemanticBindingPreimageViolation, err)
}

func runtimeSemanticBindingPreimageViolationf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", errRuntimeSemanticBindingPreimageViolation, fmt.Sprintf(format, args...))
}

// runtimeSemanticBindingPreimage is the immutable result of replaying one
// persisted semantic-plan batch.  The inventory is invocation-local: it does
// not become another authority ledger and cannot select an operation or a
// transition.
type runtimeSemanticBindingPreimage struct {
	manifestHash string
	focus        runtimecontract.CoverageFocus
	binding      runtimecontract.CoverageEvidenceBinding
}

type runtimeSemanticBindingPreimageInventory struct {
	byBindingID map[string]runtimeSemanticBindingPreimage
	byFocusID   map[string][]runtimeSemanticBindingPreimage
}

// validateRuntimeSemanticBindingPreimages prevents semantic binding authority
// from being self-certified by coordinated rewrites of Focus, candidate
// manifest, accepted-attempt and dispatch copies.  Every such projection must
// replay exactly from the persisted semantic manifest, provider document and
// capability-owner feasibility facts that preceded it.
//
// This is an aggregate-only invariant.  It performs no I/O, normalization
// repair, owner call, action selection or aggregate mutation.
func validateRuntimeSemanticBindingPreimages(a RuntimeAggregate) error {
	inventory, err := buildRuntimeSemanticBindingPreimageInventory(a.SemanticPlanning)
	if err != nil {
		return runtimeSemanticBindingPreimageViolation(err)
	}
	if len(inventory.byBindingID) == 0 && runtimeFactsCarrySemanticBindingProjection(a.Facts) {
		historyBatches, historyDocuments, historyResults := 0, 0, 0
		for _, generation := range a.SemanticPlanning.History {
			historyBatches += len(generation.Snapshot.Batches)
			historyDocuments += len(generation.Snapshot.BatchDocuments)
			historyResults += len(generation.Snapshot.BatchFeasibilityResults)
		}
		return runtimeSemanticBindingPreimageViolationf(
			"runtime aggregate semantic binding projection has no replayable provider/owner batch preimage (phase=%s active=%d/%d/%d history=%d/%d/%d)",
			a.SemanticPlanning.Phase,
			len(a.SemanticPlanning.Batches), len(a.SemanticPlanning.BatchDocuments), len(a.SemanticPlanning.BatchFeasibilityResults),
			historyBatches, historyDocuments, historyResults,
		)
	}

	if err := validateRuntimeSemanticFocusPreimages(a.Facts, inventory); err != nil {
		return runtimeSemanticBindingPreimageViolation(err)
	}
	for index, session := range a.CandidateRequestSessions {
		if session.CandidateManifest == nil {
			continue
		}
		manifest := session.CandidateManifest.Normalized()
		if !runtimeSemanticBindingProjectionState(manifest.State) {
			continue
		}
		if err := validateRuntimeSemanticBindingProjectionPreimage(
			fmt.Sprintf("candidate request session %d", index), session.Transaction.FocusID, manifest.Bindings, inventory,
		); err != nil {
			return runtimeSemanticBindingPreimageViolation(err)
		}
	}
	for index, attempt := range a.CandidateAttempts {
		if attempt.State != runtimecontract.RuntimeCandidateAttemptStateAccepted ||
			!runtimeSemanticBindingProjectionState(attempt.Transaction.State) {
			continue
		}
		if err := validateRuntimeSemanticBindingProjectionPreimage(
			fmt.Sprintf("accepted candidate attempt %d", index), attempt.Transaction.FocusID, attempt.AdmissionCoverageBindings, inventory,
		); err != nil {
			return runtimeSemanticBindingPreimageViolation(err)
		}
	}
	if a.ActiveDispatch != nil && runtimeSemanticBindingProjectionState(a.ActiveDispatch.Transaction.State) {
		if err := validateRuntimeSemanticBindingProjectionPreimage(
			"active dispatch", a.ActiveDispatch.Transaction.FocusID, a.ActiveDispatch.CoverageBindings, inventory,
		); err != nil {
			return runtimeSemanticBindingPreimageViolation(err)
		}
	}
	for index, dispatch := range a.CompletedDispatches {
		if !runtimeSemanticBindingProjectionState(dispatch.Transaction.State) {
			continue
		}
		if err := validateRuntimeSemanticBindingProjectionPreimage(
			fmt.Sprintf("completed dispatch %d", index), dispatch.Transaction.FocusID, dispatch.CoverageBindings, inventory,
		); err != nil {
			return runtimeSemanticBindingPreimageViolation(err)
		}
	}
	return nil
}

func runtimeFactsCarrySemanticBindingProjection(facts runtimecontract.RuntimeFacts) bool {
	for _, focus := range append(append([]runtimecontract.CoverageFocus{facts.Focus}, facts.RemainingFocuses...), facts.CompletedFocuses...) {
		if len(focus.CoverageEvidenceBindings) != 0 {
			return true
		}
	}
	return false
}

func runtimeSemanticBindingProjectionState(state runtimecontract.ConvergenceState) bool {
	return state == runtimecontract.ConvergenceStateNeedStateChange ||
		state == runtimecontract.ConvergenceStateNeedValidation ||
		state == runtimecontract.ConvergenceStateNeedAssessment
}

func buildRuntimeSemanticBindingPreimageInventory(ledger runtimecontract.RuntimeSemanticPlanningLedger) (runtimeSemanticBindingPreimageInventory, error) {
	inventory := runtimeSemanticBindingPreimageInventory{
		byBindingID: make(map[string]runtimeSemanticBindingPreimage),
		byFocusID:   make(map[string][]runtimeSemanticBindingPreimage),
	}
	// Detach only the active generation. Cloning before clearing History would
	// deep-copy every immutable generation and then traverse each one again
	// below, multiplying validation cost without strengthening the preimage
	// proof.
	active := ledger
	active.History = nil
	active = active.Clone()
	if err := addRuntimeSemanticLedgerBindingPreimages(&inventory, "active", active); err != nil {
		return runtimeSemanticBindingPreimageInventory{}, err
	}
	for index, generation := range ledger.History {
		if err := addRuntimeSemanticLedgerBindingPreimages(&inventory, fmt.Sprintf("history %d", index), generation.Snapshot); err != nil {
			return runtimeSemanticBindingPreimageInventory{}, err
		}
	}
	return inventory, nil
}

func addRuntimeSemanticLedgerBindingPreimages(inventory *runtimeSemanticBindingPreimageInventory, label string, ledger runtimecontract.RuntimeSemanticPlanningLedger) error {
	if inventory == nil || ledger.Manifest == nil {
		return nil
	}
	manifest := ledger.Manifest.Normalized()
	documents := make(map[string]runtimecontract.RuntimeSemanticPlanDocument, len(ledger.BatchDocuments))
	for _, document := range ledger.BatchDocuments {
		documents[document.BatchID] = document.Clone()
	}
	results := make(map[string]runtimecontract.RuntimeSemanticPlanBatchFeasibilityResult, len(ledger.BatchFeasibilityResults))
	for _, result := range ledger.BatchFeasibilityResults {
		results[result.BatchID] = result.Normalized()
	}

	for _, batch := range ledger.Batches {
		batch = batch.Normalized()
		if batch.State != runtimecontract.RuntimeSemanticPlanBatchAccepted {
			continue
		}
		document, documentFound := documents[batch.BatchID]
		if !documentFound {
			continue
		}
		feasibilities, feasibilityFound := runtimeSemanticBatchPreimageFeasibilities(ledger, batch, document, results)
		if !feasibilityFound {
			// An accepted working batch has not created binding authority until its
			// owner facts are archived (or retained in the current sealed working
			// set). Any projection that tries to use it will fail the lookup below.
			continue
		}
		if len(manifest.ProofObligations) == 1 && manifest.ProofObligations[0].ImplementationWork != nil {
			// working proof 的 accepted provider/owner preimage 不编译 terminal binding。
			// seal 后的 working authority 另由完整 ImplementationWork source join 验证。
			if manifest.ProofObligations[0].ImplementationWork.PurposeChoiceOnly {
				return fmt.Errorf("implementation purpose-choice cannot contain an accepted proof batch")
			}
			if err := document.ValidateAgainstManifest(manifest); err != nil {
				return err
			}
			if document.BatchID != batch.BatchID || document.BatchSequence != batch.Sequence || batch.DocumentHash != runtimecontract.StableRuntimeSemanticPlanDocumentHash(document) || !sameCoverageIDs(document.BatchCoverageWorkItemIDs, batch.CoverageWorkItemIDs) {
				return fmt.Errorf("implementation accepted batch differs from original provider document")
			}
			if err := runtimecontract.ValidateRuntimeSemanticFeasibilitySet(manifest, document, feasibilities, true); err != nil {
				return err
			}
			continue
		}
		focuses, _, err := compileRuntimeSemanticAcceptedBatchBindings(manifest, batch, document, feasibilities)
		if err != nil {
			return fmt.Errorf("runtime aggregate semantic binding preimage %s batch %q: %w", label, batch.BatchID, err)
		}
		for _, rawFocus := range focuses {
			focus := rawFocus.Normalized()
			for _, rawBinding := range focus.CoverageEvidenceBindings {
				binding := rawBinding.Normalized()
				preimage := runtimeSemanticBindingPreimage{manifestHash: manifest.Hash, focus: focus, binding: binding}
				if previous, duplicate := inventory.byBindingID[binding.ID]; duplicate {
					if previous.manifestHash != preimage.manifestHash ||
						!reflect.DeepEqual(previous.focus, preimage.focus) ||
						!reflect.DeepEqual(previous.binding, preimage.binding) {
						return fmt.Errorf("runtime aggregate semantic binding preimage %q conflicts across generations", binding.ID)
					}
					return fmt.Errorf("runtime aggregate semantic binding preimage %q is duplicated across generations", binding.ID)
				}
				inventory.byBindingID[binding.ID] = preimage
				inventory.byFocusID[focus.ID] = append(inventory.byFocusID[focus.ID], preimage)
			}
		}
	}
	return nil
}

func runtimeSemanticBatchPreimageFeasibilities(
	ledger runtimecontract.RuntimeSemanticPlanningLedger,
	batch runtimecontract.RuntimeSemanticPlanBatch,
	document runtimecontract.RuntimeSemanticPlanDocument,
	results map[string]runtimecontract.RuntimeSemanticPlanBatchFeasibilityResult,
) ([]runtimecontract.RuntimeSemanticProofFeasibility, bool) {
	if result, found := results[batch.BatchID]; found {
		return append([]runtimecontract.RuntimeSemanticProofFeasibility(nil), result.Feasibilities...), true
	}
	// The seal CAS retains the exact current batch document and owner facts even
	// before the archival projection event is observed. This is still an earlier
	// persisted preimage, not a reconstruction from Focus or dispatch copies.
	if ledger.Document != nil && ledger.Document.BatchID == batch.BatchID &&
		runtimecontract.StableRuntimeSemanticPlanDocumentHash(*ledger.Document) == runtimecontract.StableRuntimeSemanticPlanDocumentHash(document) &&
		ledger.DocumentHash == runtimecontract.StableRuntimeSemanticPlanDocumentHash(document) &&
		ledger.FeasibilitySetHash == runtimecontract.StableRuntimeSemanticFeasibilitySetHash(ledger.Feasibilities) &&
		len(ledger.Feasibilities) == len(document.Proposals) {
		return append([]runtimecontract.RuntimeSemanticProofFeasibility(nil), ledger.Feasibilities...), true
	}
	return nil, false
}

func validateRuntimeSemanticFocusPreimages(facts runtimecontract.RuntimeFacts, inventory runtimeSemanticBindingPreimageInventory) error {
	focuses := append(append([]runtimecontract.CoverageFocus{facts.Focus}, facts.RemainingFocuses...), facts.CompletedFocuses...)
	for index, rawFocus := range focuses {
		focus := rawFocus.Normalized()
		if len(focus.CoverageEvidenceBindings) == 0 {
			continue
		}
		if len(focus.CoverageEvidenceBindings) != 1 {
			return fmt.Errorf("runtime aggregate semantic focus projection %d does not match one compiled CWI preimage", index)
		}
		binding := focus.CoverageEvidenceBindings[0].Normalized()
		preimage, found := inventory.byBindingID[binding.ID]
		if !found {
			available := make([]string, 0, len(inventory.byBindingID))
			for id := range inventory.byBindingID {
				available = append(available, id)
			}
			sort.Strings(available)
			return fmt.Errorf("runtime aggregate semantic focus %q binding %q has no exact provider/owner preimage (replayed=%s)", focus.ID, binding.ID, strings.Join(available, ","))
		}
		if preimage.binding.Hash != binding.Hash || !reflect.DeepEqual(preimage.binding, binding) {
			return fmt.Errorf("runtime aggregate semantic focus %q binding %q differs from its exact provider/owner preimage", focus.ID, binding.ID)
		}
		if !reflect.DeepEqual(preimage.focus, focus) {
			return fmt.Errorf("runtime aggregate semantic focus %q differs from binding %q provider/owner preimage", focus.ID, binding.ID)
		}
	}

	currentFocuses := make([]runtimecontract.CoverageFocus, 0)
	for _, item := range facts.Coverage {
		for lineageIndex, ref := range item.BindingLineage {
			if _, err := runtimeSemanticBindingPreimageForRef(item.ID, ref, inventory); err != nil {
				return fmt.Errorf("runtime aggregate CWI %q binding lineage %d: %w", item.ID, lineageIndex, err)
			}
		}
		if item.CurrentBindingRef == nil {
			continue
		}
		preimage, err := runtimeSemanticBindingPreimageForRef(item.ID, *item.CurrentBindingRef, inventory)
		if err != nil {
			return fmt.Errorf("runtime aggregate CWI %q current binding: %w", item.ID, err)
		}
		currentFocuses = append(currentFocuses, preimage.focus)
	}

	expected := runtimecontract.RuntimeCWIReconciliationContract{}
	if len(currentFocuses) != 0 {
		var err error
		expected, err = runtimecontract.CompileRuntimeCWIReconciliationContract(facts.DefinitionRevision, currentFocuses)
		if err != nil {
			return fmt.Errorf("runtime aggregate semantic reconciliation preimage: %w", err)
		}
	}
	if !runtimecontract.SameRuntimeCWIReconciliationContract(expected, facts.ReconciliationContract) {
		return fmt.Errorf("runtime aggregate reconciliation contract has no exact semantic binding preimage")
	}
	return nil
}

func runtimeSemanticBindingPreimageForRef(
	coverageID common.CoverageWorkItemID,
	ref runtimecontract.RuntimeCWIBindingRef,
	inventory runtimeSemanticBindingPreimageInventory,
) (runtimeSemanticBindingPreimage, error) {
	ref = ref.Normalized()
	preimage, found := inventory.byBindingID[ref.BindingID]
	if !found || preimage.manifestHash != ref.SemanticManifestHash ||
		preimage.binding.CoverageWorkItemID != coverageID ||
		preimage.binding.Hash != ref.BindingHash || preimage.focus.ID != ref.FocusID {
		return runtimeSemanticBindingPreimage{}, fmt.Errorf("binding %q has no exact semantic provider/owner preimage", ref.BindingID)
	}
	return preimage, nil
}

func validateRuntimeSemanticBindingProjectionPreimage(
	label string,
	focusID string,
	actual []runtimecontract.CoverageEvidenceBinding,
	inventory runtimeSemanticBindingPreimageInventory,
) error {
	expectedRows := inventory.byFocusID[focusID]
	expected := make([]runtimecontract.CoverageEvidenceBinding, 0, len(expectedRows))
	for _, row := range expectedRows {
		expected = append(expected, row.binding.Clone())
	}
	if len(actual) == 0 && len(expected) == 0 {
		return nil
	}
	if !runtimecontract.RuntimeCoverageEvidenceBindingProjectionsEqual(actual, expected) {
		return fmt.Errorf("runtime aggregate %s coverage bindings have no complete semantic provider/owner preimage for focus %q", label, focusID)
	}
	return nil
}
