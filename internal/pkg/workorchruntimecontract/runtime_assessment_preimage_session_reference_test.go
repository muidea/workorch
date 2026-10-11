package workorchruntimecontract

import "fmt"

// 保留 fb56803d 的完整 ledger/generation 遍历与独立 manifest 校验。
func referenceAssessmentPurposePreimagesFromSemanticLedger(ledger RuntimeSemanticPlanningLedger, semanticLedgerAlreadyValidated bool) ([]runtimeAssessmentPurposeBindingPreimage, error) {
	ret := make([]runtimeAssessmentPurposeBindingPreimage, 0)
	// Detach only the active generation. Cloning the ledger before clearing
	// History needlessly clones every immutable snapshot that is traversed
	// separately below.
	active := ledger
	active.History = nil
	active = active.Clone()
	activeEntries, err := referenceAssessmentPurposePreimagesFromSemanticGeneration(active)
	if err != nil {
		return nil, err
	}
	ret = append(ret, activeEntries...)
	for index, generation := range ledger.History {
		if !semanticLedgerAlreadyValidated {
			if err := generation.Validate(); err != nil {
				return nil, fmt.Errorf("runtime assessment purpose semantic history %d: %w", index, err)
			}
		}
		entries, err := referenceAssessmentPurposePreimagesFromSemanticGeneration(generation.Snapshot)
		if err != nil {
			return nil, fmt.Errorf("runtime assessment purpose semantic history %d: %w", index, err)
		}
		ret = append(ret, entries...)
	}
	return ret, nil
}

func referenceAssessmentPurposePreimagesFromSemanticGeneration(
	ledger RuntimeSemanticPlanningLedger,
) ([]runtimeAssessmentPurposeBindingPreimage, error) {
	if ledger.Manifest == nil {
		return nil, nil
	}
	validated, err := validateRuntimeSemanticPlanManifest(*ledger.Manifest)
	if err != nil {
		return nil, fmt.Errorf("runtime assessment purpose semantic manifest: %w", err)
	}
	manifest := validated.manifest
	documents := make(map[string]RuntimeSemanticPlanDocument, len(ledger.BatchDocuments))
	for _, document := range ledger.BatchDocuments {
		if _, duplicate := documents[document.BatchID]; duplicate {
			return nil, fmt.Errorf("runtime assessment purpose has duplicate batch document")
		}
		documents[document.BatchID] = document.Clone()
	}
	results := make(map[string]RuntimeSemanticPlanBatchFeasibilityResult, len(ledger.BatchFeasibilityResults))
	for _, result := range ledger.BatchFeasibilityResults {
		if _, duplicate := results[result.BatchID]; duplicate {
			return nil, fmt.Errorf("runtime assessment purpose has duplicate batch feasibility")
		}
		results[result.BatchID] = result.Normalized()
	}

	ret := make([]runtimeAssessmentPurposeBindingPreimage, 0)
	for _, rawBatch := range ledger.Batches {
		batch := rawBatch.Normalized()
		if batch.State != RuntimeSemanticPlanBatchAccepted {
			continue
		}
		document, found := documents[batch.BatchID]
		if !found {
			continue
		}
		feasibilities, found := runtimeAssessmentPurposeBatchFeasibilities(ledger, batch, document, results, validated)
		if !found {
			continue
		}
		if err := validateRuntimeSemanticPlanBatchAgainstValidatedManifest(rawBatch, validated); err != nil {
			return nil, err
		}
		if batch.DocumentHash != StableRuntimeSemanticPlanDocumentHash(document) || document.BatchID != batch.BatchID || document.BatchSequence != batch.Sequence {
			return nil, fmt.Errorf("runtime assessment purpose requires exact accepted batch document")
		}
		focuses, _, err := compileRuntimeSemanticBindingsWithValidatedManifest(validated, batch.RequestLogicalID, document, feasibilities)
		if err != nil {
			return nil, fmt.Errorf("runtime assessment purpose semantic batch %q: %w", batch.BatchID, err)
		}
		for _, focus := range focuses {
			focus = focus.Normalized()
			preimage, err := runtimeAssessmentPurposeBindingPreimageForFocus(manifest, focus)
			if err != nil {
				return nil, err
			}
			preimage.acceptedDocument = document.Clone()
			preimage.ownerCatalogs = CloneRuntimeOperationAdmissibilities(manifest.OwnerCatalogs)
			ret = append(ret, preimage)
		}
	}
	return ret, nil
}
