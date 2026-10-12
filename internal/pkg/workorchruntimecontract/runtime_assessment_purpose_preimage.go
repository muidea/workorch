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

type runtimeAssessmentPurposeBindingPreimage struct {
	bindings             []CoverageEvidenceBinding
	semanticManifestHash string
	obligation           RuntimeSemanticProofObligation
	// 原 accepted batch 的完整评审前像，只在同次纯反证中使用。
	acceptedDocument RuntimeSemanticPlanDocument
	ownerCatalogs    []RuntimeOperationAdmissibility
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
	entries, err := runtimeAssessmentPurposePreimagesFromSemanticLedger(ledger, false)
	if err != nil {
		return RuntimeAssessmentPurposePreimage{}, err
	}
	return resolveRuntimeAssessmentPurposePreimage(entries, bindings)
}

func runtimeAssessmentPurposePreimagesFromSemanticLedger(ledger RuntimeSemanticPlanningLedger, semanticLedgerAlreadyValidated bool) ([]runtimeAssessmentPurposeBindingPreimage, error) {
	ret := make([]runtimeAssessmentPurposeBindingPreimage, 0)
	// session 仅属于本次前像遍历；复用要求 exact raw 相等，不凭 manifest hash 授信。
	session := newRuntimeSemanticValidationSession()
	// Detach only the active generation. Cloning the ledger before clearing
	// History needlessly clones every immutable snapshot that is traversed
	// separately below.
	active := ledger
	active.History = nil
	active = active.Clone()
	activeEntries, err := runtimeAssessmentPurposePreimagesFromSemanticGenerationInSession(active, session)
	if err != nil {
		return nil, err
	}
	ret = append(ret, activeEntries...)
	for index, generation := range ledger.History {
		if !semanticLedgerAlreadyValidated {
			if err := validateRuntimeSemanticPlanningGenerationNormalized(generation, generation.Normalized(), session); err != nil {
				return nil, fmt.Errorf("runtime assessment purpose semantic history %d: %w", index, err)
			}
		}
		entries, err := runtimeAssessmentPurposePreimagesFromSemanticGenerationInSession(generation.Snapshot, session)
		if err != nil {
			return nil, fmt.Errorf("runtime assessment purpose semantic history %d: %w", index, err)
		}
		ret = append(ret, entries...)
	}
	return ret, nil
}

// Index every valid proof purpose, including intermediate repair proofs. Only
// the uniquely matched binding determines whether an assessment is authorized;
// unrelated current or historical generations cannot redefine that purpose.
func resolveRuntimeAssessmentPurposePreimage(entries []runtimeAssessmentPurposeBindingPreimage, bindings []CoverageEvidenceBinding) (RuntimeAssessmentPurposePreimage, error) {
	if len(bindings) == 0 {
		return RuntimeAssessmentPurposePreimage{}, fmt.Errorf("runtime assessment purpose requires a sealed semantic binding preimage")
	}
	if err := ValidateCoverageEvidenceBindingProjection(bindings); err != nil {
		return RuntimeAssessmentPurposePreimage{}, fmt.Errorf("runtime assessment purpose binding preimage: %w", err)
	}
	var matched runtimeAssessmentPurposeBindingPreimage
	count := 0
	for _, entry := range entries {
		if RuntimeCoverageEvidenceBindingProjectionsEqual(entry.bindings, bindings) {
			matched = entry
			count++
		}
	}
	if count != 1 {
		return RuntimeAssessmentPurposePreimage{}, fmt.Errorf("runtime assessment purpose has %d exact semantic binding preimages", count)
	}
	return matched.assessmentPurpose()
}

func runtimeAssessmentPurposePreimagesFromSemanticGeneration(
	ledger RuntimeSemanticPlanningLedger,
) ([]runtimeAssessmentPurposeBindingPreimage, error) {
	return runtimeAssessmentPurposePreimagesFromSemanticGenerationInSession(ledger, newRuntimeSemanticValidationSession())
}

func runtimeAssessmentPurposePreimagesFromSemanticGenerationInSession(
	ledger RuntimeSemanticPlanningLedger,
	session *runtimeSemanticValidationSession,
) ([]runtimeAssessmentPurposeBindingPreimage, error) {
	if ledger.Manifest == nil {
		return nil, nil
	}
	validated, err := validateRuntimeSemanticPlanManifestInSession(*ledger.Manifest, session)
	if err != nil {
		return nil, fmt.Errorf("runtime assessment purpose semantic manifest: %w", err)
	}
	manifest := validated.manifest
	documents := make(map[string]RuntimeSemanticPlanDocument, len(ledger.BatchDocuments))
	for _, document := range ledger.BatchDocuments {
		if _, duplicate := documents[document.BatchID]; duplicate {
			return nil, fmt.Errorf("runtime assessment purpose has duplicate batch document")
		}
		documents[document.BatchID] = document.Normalized()
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
			// 私有 compiler 已返回完整 Normalized focus；此处仅消费，不再次复制。
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

func runtimeAssessmentPurposeBindingPreimageForFocus(manifest RuntimeSemanticPlanManifest, focus CoverageFocus) (runtimeAssessmentPurposeBindingPreimage, error) {
	if len(focus.CoverageWorkItemIDs) != 1 {
		return runtimeAssessmentPurposeBindingPreimage{}, fmt.Errorf("runtime assessment purpose semantic focus must contain one coverage item")
	}
	coverageID := focus.CoverageWorkItemIDs[0]
	var obligation *RuntimeSemanticProofObligation
	for index := range manifest.ProofObligations {
		candidate := manifest.ProofObligations[index]
		if candidate.CoverageWorkItemID != coverageID {
			continue
		}
		if obligation != nil {
			return runtimeAssessmentPurposeBindingPreimage{}, fmt.Errorf("runtime assessment purpose semantic manifest has duplicate CWI obligations")
		}
		value := candidate.Clone()
		obligation = &value
	}
	if obligation == nil {
		return runtimeAssessmentPurposeBindingPreimage{}, fmt.Errorf("runtime assessment purpose semantic manifest has no CWI obligation")
	}
	return runtimeAssessmentPurposeBindingPreimage{
		bindings:             CloneCoverageEvidenceBindings(focus.CoverageEvidenceBindings),
		semanticManifestHash: manifest.Hash,
		obligation:           *obligation,
	}, nil
}

func (entry runtimeAssessmentPurposeBindingPreimage) assessmentPurpose() (RuntimeAssessmentPurposePreimage, error) {
	obligation := entry.obligation
	resolved, err := ResolveRuntimeSemanticEvidenceObligation(obligation)
	if err != nil {
		return RuntimeAssessmentPurposePreimage{}, fmt.Errorf("runtime assessment purpose evidence obligation: %w", err)
	}
	preimage := RuntimeAssessmentPurposePreimage{SemanticManifestHash: entry.semanticManifestHash}
	switch resolved.Purpose {
	case RuntimeSemanticEvidenceObligationDefinitionProof:
		preimage.Purpose = RuntimeAssessmentPurposeDefinitionProof
	case RuntimeSemanticEvidenceObligationOwnerFactAcquisition:
		if obligation.OwnerFactAcquisition == nil {
			return RuntimeAssessmentPurposePreimage{}, fmt.Errorf("runtime assessment owner-fact purpose has no acquisition contract")
		}
		value := obligation.OwnerFactAcquisition.Clone()
		if err := value.Validate(); err != nil {
			return RuntimeAssessmentPurposePreimage{}, fmt.Errorf("runtime assessment owner-fact purpose: %w", err)
		}
		preimage.Purpose = RuntimeAssessmentPurposeOwnerFactAcquisition
		preimage.OwnerFactAcquisition = &value
	default:
		return RuntimeAssessmentPurposePreimage{}, fmt.Errorf("runtime assessment purpose has unsupported semantic obligation %q", resolved.Purpose)
	}
	return preimage, nil
}

func runtimeAssessmentPurposeBatchFeasibilities(
	ledger RuntimeSemanticPlanningLedger,
	batch RuntimeSemanticPlanBatch,
	document RuntimeSemanticPlanDocument,
	results map[string]RuntimeSemanticPlanBatchFeasibilityResult,
	validated runtimeValidatedSemanticManifest,
) ([]RuntimeSemanticProofFeasibility, bool) {
	if result, found := results[batch.BatchID]; found {
		// 复用本代同次完整 manifest 校验；batch、document 和 owner result 仍逐项反证。
		if validateRuntimeSemanticPlanBatchAgainstValidatedManifest(batch, validated) != nil ||
			validateRuntimeSemanticPlanDocumentAgainstValidatedManifest(document, validated) != nil ||
			validateRuntimeSemanticPlanBatchFeasibilityResultWithValidatedManifest(result, batch, document, validated) != nil {
			return nil, false
		}
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
