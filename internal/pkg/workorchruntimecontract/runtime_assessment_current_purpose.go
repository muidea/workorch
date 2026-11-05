package workorchruntimecontract

import (
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeAssessmentPurposeForCurrentFocus consumes a formal owner's current
// binding and its original accepted batch. It does not recompile all historical
// proofs or admit raw external records. Raw admission and offline audits retain
// ResolveRuntimeAssessmentPurposePreimage's complete source proof.
func RuntimeAssessmentPurposeForCurrentFocus(facts RuntimeFacts, ledger RuntimeSemanticPlanningLedger, focus CoverageFocus) (RuntimeAssessmentPurposePreimage, error) {
	if len(focus.CoverageWorkItemIDs) != 1 || len(focus.CoverageEvidenceBindings) != 1 {
		return RuntimeAssessmentPurposePreimage{}, fmt.Errorf("current assessment requires one exact CWI binding")
	}
	if err := ValidateCoverageEvidenceBindingProjection(focus.CoverageEvidenceBindings); err != nil {
		return RuntimeAssessmentPurposePreimage{}, err
	}
	binding := focus.CoverageEvidenceBindings[0]
	coverageID := focus.CoverageWorkItemIDs[0]
	if binding.CoverageWorkItemID != coverageID || binding.FocusID != focus.ID {
		return RuntimeAssessmentPurposePreimage{}, fmt.Errorf("current assessment focus differs from its binding")
	}
	var ref RuntimeCWIBindingRef
	rows := 0
	for _, row := range facts.Coverage {
		if row.ID != coverageID {
			continue
		}
		rows++
		if row.CurrentBindingRef == nil || row.FocusID != focus.ID {
			return RuntimeAssessmentPurposePreimage{}, fmt.Errorf("current assessment has no owner-selected binding")
		}
		ref = *row.CurrentBindingRef
	}
	if rows != 1 || ref.Validate() != nil || !SameRuntimeCWIBindingRef(ref, RuntimeCWIBindingRefForCoverageBinding(binding, ref.SemanticManifestHash)) {
		return RuntimeAssessmentPurposePreimage{}, fmt.Errorf("current assessment has no unique exact owner-selected binding")
	}

	var matched RuntimeAssessmentPurposePreimage
	matches := 0
	consume := func(source RuntimeSemanticPlanningLedger) error {
		m := source.Manifest
		if m == nil || m.Hash != ref.SemanticManifestHash {
			return nil
		}
		if m.WorkspaceID != facts.WorkspaceID || m.RunID != facts.RunID || m.DefinitionRevision != facts.DefinitionRevision || m.DefinitionSeedHash != facts.DefinitionSeed.Hash {
			return fmt.Errorf("current assessment semantic source differs from its owner scope")
		}
		for _, batch := range source.Batches {
			if batch.State != RuntimeSemanticPlanBatchAccepted || !runtimeCurrentAssessmentBatchContains(batch, coverageID) {
				continue
			}
			for _, document := range source.BatchDocuments {
				if document.BatchID != batch.BatchID {
					continue
				}
				var proposals []RuntimeSemanticProofProposal
				for _, proposal := range document.Proposals {
					if proposal.CoverageWorkItemID == coverageID {
						proposals = append(proposals, proposal)
					}
				}
				id, bindingID, err := runtimeSemanticProofFocusIdentity(StableRuntimeSemanticGenerationID(m.Hash, batch.RequestLogicalID), coverageID, proposals)
				if err != nil || id != ref.FocusID || bindingID != ref.BindingID {
					continue
				}
				if len(proposals) == 0 || document.ManifestHash != m.Hash || document.DefinitionSeedHash != m.DefinitionSeedHash || document.BatchSequence != batch.Sequence || batch.DocumentHash != StableRuntimeSemanticPlanDocumentHash(document) {
					return fmt.Errorf("current assessment has no exact accepted source document")
				}
				obligations := 0
				for _, obligation := range m.ProofObligations {
					if obligation.CoverageWorkItemID != coverageID {
						continue
					}
					obligations++
					entry := runtimeAssessmentPurposeBindingPreimage{semanticManifestHash: m.Hash, obligation: obligation}
					matched, err = entry.assessmentPurpose()
					if err != nil {
						return err
					}
				}
				if obligations != 1 {
					return fmt.Errorf("current assessment has no unique original obligation")
				}
				matches++
			}
		}
		return nil
	}
	if err := consume(ledger); err != nil {
		return RuntimeAssessmentPurposePreimage{}, err
	}
	for _, generation := range ledger.AllHistory() {
		if err := consume(generation.Snapshot); err != nil {
			return RuntimeAssessmentPurposePreimage{}, err
		}
	}
	if matches != 1 {
		return RuntimeAssessmentPurposePreimage{}, fmt.Errorf("current assessment has %d exact accepted semantic sources", matches)
	}
	return matched, nil
}

func runtimeCurrentAssessmentBatchContains(batch RuntimeSemanticPlanBatch, coverageID common.CoverageWorkItemID) bool {
	for _, id := range batch.CoverageWorkItemIDs {
		if id == coverageID {
			return true
		}
	}
	return false
}
