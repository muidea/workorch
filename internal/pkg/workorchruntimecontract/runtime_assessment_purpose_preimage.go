package workorchruntimecontract

import (
	"fmt"
	"reflect"
)

// RuntimeAssessmentPurposePreimage is the immutable semantic-plan fact from
// which an assessment candidate manifest derives its purpose.  The semantic
// manifest and its accepted provider/owner batch predate the operations
// request, so the candidate manifest and dispatch cannot certify their own
// purpose by merely recomputing a local hash.
type RuntimeAssessmentPurposePreimage struct {
	Purpose              RuntimeAssessmentPurpose
	SemanticManifestHash string
	OwnerFactAcquisition *RuntimeSemanticOwnerFactAcquisition
}

func (p RuntimeAssessmentPurposePreimage) Clone() RuntimeAssessmentPurposePreimage {
	if p.OwnerFactAcquisition != nil {
		value := p.OwnerFactAcquisition.Clone()
		p.OwnerFactAcquisition = &value
	}
	return p
}

// ResolveRuntimeAssessmentPurposePreimage replays the exact semantic batch
// which produced bindings. It performs no normalization repair, state
// transition, owner I/O, or action selection.
func ResolveRuntimeAssessmentPurposePreimage(
	ledger RuntimeSemanticPlanningLedger,
	bindings []CoverageEvidenceBinding,
) (RuntimeAssessmentPurposePreimage, error) {
	bindings = CloneCoverageEvidenceBindings(bindings)
	if len(bindings) == 0 {
		return RuntimeAssessmentPurposePreimage{}, fmt.Errorf("runtime assessment purpose requires a sealed semantic binding preimage")
	}
	if err := ValidateCoverageEvidenceBindingProjection(bindings); err != nil {
		return RuntimeAssessmentPurposePreimage{}, fmt.Errorf("runtime assessment purpose binding preimage: %w", err)
	}

	var matches []RuntimeAssessmentPurposePreimage
	active := ledger.Clone()
	active.History = nil
	if preimage, found, err := runtimeAssessmentPurposePreimageFromSemanticGeneration(active, bindings); err != nil {
		return RuntimeAssessmentPurposePreimage{}, err
	} else if found {
		matches = append(matches, preimage)
	}
	for index, generation := range ledger.History {
		if err := generation.Validate(); err != nil {
			return RuntimeAssessmentPurposePreimage{}, fmt.Errorf("runtime assessment purpose semantic history %d: %w", index, err)
		}
		if preimage, found, err := runtimeAssessmentPurposePreimageFromSemanticGeneration(generation.Snapshot, bindings); err != nil {
			return RuntimeAssessmentPurposePreimage{}, fmt.Errorf("runtime assessment purpose semantic history %d: %w", index, err)
		} else if found {
			matches = append(matches, preimage)
		}
	}
	if len(matches) != 1 {
		return RuntimeAssessmentPurposePreimage{}, fmt.Errorf("runtime assessment purpose has %d exact semantic binding preimages", len(matches))
	}
	return matches[0].Clone(), nil
}

func runtimeAssessmentPurposePreimageFromSemanticGeneration(
	ledger RuntimeSemanticPlanningLedger,
	bindings []CoverageEvidenceBinding,
) (RuntimeAssessmentPurposePreimage, bool, error) {
	if ledger.Manifest == nil {
		return RuntimeAssessmentPurposePreimage{}, false, nil
	}
	manifest := ledger.Manifest.Normalized()
	if err := manifest.Validate(); err != nil {
		return RuntimeAssessmentPurposePreimage{}, false, fmt.Errorf("runtime assessment purpose semantic manifest: %w", err)
	}
	documents := make(map[string]RuntimeSemanticPlanDocument, len(ledger.BatchDocuments))
	for _, document := range ledger.BatchDocuments {
		documents[document.BatchID] = document.Clone()
	}
	results := make(map[string]RuntimeSemanticPlanBatchFeasibilityResult, len(ledger.BatchFeasibilityResults))
	for _, result := range ledger.BatchFeasibilityResults {
		results[result.BatchID] = result.Normalized()
	}

	matchedFocuses := make([]CoverageFocus, 0, 1)
	for _, rawBatch := range ledger.Batches {
		batch := rawBatch.Normalized()
		if batch.State != RuntimeSemanticPlanBatchAccepted {
			continue
		}
		document, found := documents[batch.BatchID]
		if !found {
			continue
		}
		feasibilities, found := runtimeAssessmentPurposeBatchFeasibilities(ledger, batch, document, results)
		if !found {
			continue
		}
		focuses, _, err := CompileRuntimeSemanticBindings(manifest, batch.RequestLogicalID, document, feasibilities)
		if err != nil {
			return RuntimeAssessmentPurposePreimage{}, false, fmt.Errorf("runtime assessment purpose semantic batch %q: %w", batch.BatchID, err)
		}
		for _, focus := range focuses {
			focus = focus.Normalized()
			if RuntimeCoverageEvidenceBindingProjectionsEqual(focus.CoverageEvidenceBindings, bindings) {
				matchedFocuses = append(matchedFocuses, focus)
			}
		}
	}
	if len(matchedFocuses) == 0 {
		return RuntimeAssessmentPurposePreimage{}, false, nil
	}
	if len(matchedFocuses) != 1 {
		return RuntimeAssessmentPurposePreimage{}, false, fmt.Errorf("runtime assessment purpose semantic manifest %q has duplicate binding preimages", manifest.Hash)
	}
	focus := matchedFocuses[0]
	if len(focus.CoverageWorkItemIDs) != 1 {
		return RuntimeAssessmentPurposePreimage{}, false, fmt.Errorf("runtime assessment purpose semantic focus must contain one coverage item")
	}
	coverageID := focus.CoverageWorkItemIDs[0]
	var obligation *RuntimeSemanticProofObligation
	for index := range manifest.ProofObligations {
		candidate := manifest.ProofObligations[index].Clone()
		if candidate.CoverageWorkItemID != coverageID {
			continue
		}
		if obligation != nil {
			return RuntimeAssessmentPurposePreimage{}, false, fmt.Errorf("runtime assessment purpose semantic manifest has duplicate CWI obligations")
		}
		obligation = &candidate
	}
	if obligation == nil {
		return RuntimeAssessmentPurposePreimage{}, false, fmt.Errorf("runtime assessment purpose semantic manifest has no CWI obligation")
	}
	resolved, err := ResolveRuntimeSemanticEvidenceObligation(*obligation)
	if err != nil {
		return RuntimeAssessmentPurposePreimage{}, false, fmt.Errorf("runtime assessment purpose evidence obligation: %w", err)
	}
	preimage := RuntimeAssessmentPurposePreimage{SemanticManifestHash: manifest.Hash}
	switch resolved.Purpose {
	case RuntimeSemanticEvidenceObligationDefinitionProof:
		preimage.Purpose = RuntimeAssessmentPurposeDefinitionProof
	case RuntimeSemanticEvidenceObligationOwnerFactAcquisition:
		if obligation.OwnerFactAcquisition == nil {
			return RuntimeAssessmentPurposePreimage{}, false, fmt.Errorf("runtime assessment owner-fact purpose has no acquisition contract")
		}
		value := obligation.OwnerFactAcquisition.Clone()
		if err := value.Validate(); err != nil {
			return RuntimeAssessmentPurposePreimage{}, false, fmt.Errorf("runtime assessment owner-fact purpose: %w", err)
		}
		preimage.Purpose = RuntimeAssessmentPurposeOwnerFactAcquisition
		preimage.OwnerFactAcquisition = &value
	default:
		return RuntimeAssessmentPurposePreimage{}, false, fmt.Errorf("runtime assessment purpose has unsupported semantic obligation %q", resolved.Purpose)
	}
	return preimage, true, nil
}

func runtimeAssessmentPurposeBatchFeasibilities(
	ledger RuntimeSemanticPlanningLedger,
	batch RuntimeSemanticPlanBatch,
	document RuntimeSemanticPlanDocument,
	results map[string]RuntimeSemanticPlanBatchFeasibilityResult,
) ([]RuntimeSemanticProofFeasibility, bool) {
	if result, found := results[batch.BatchID]; found {
		return append([]RuntimeSemanticProofFeasibility(nil), result.Feasibilities...), true
	}
	if ledger.Document != nil && ledger.Document.BatchID == batch.BatchID &&
		reflect.DeepEqual(*ledger.Document, document) &&
		ledger.DocumentHash == StableRuntimeSemanticPlanDocumentHash(document) &&
		ledger.FeasibilitySetHash == StableRuntimeSemanticFeasibilitySetHash(ledger.Feasibilities) &&
		len(ledger.Feasibilities) == len(document.Proposals) {
		return append([]RuntimeSemanticProofFeasibility(nil), ledger.Feasibilities...), true
	}
	return nil, false
}
