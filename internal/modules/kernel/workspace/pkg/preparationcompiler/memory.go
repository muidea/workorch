package preparationcompiler

import (
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/memoryselection"
	"github.com/muidea/workorch/internal/pkg/runtimeprojection"
	"github.com/muidea/workorch/internal/pkg/workorchmemorycontract"
)

func applyRuntimeMemorySelectionBudget(candidates []*memorymodel.MemoryItem, evidence *runtimeprojection.MemorySelectionEvidence) ([]*memorymodel.MemoryItem, *runtimeprojection.MemorySelectionEvidence, error) {
	selection, err := memoryselection.SelectRuntimeMemoryContext(candidates)
	if err != nil {
		return nil, nil, err
	}
	if evidence == nil {
		evidence = &runtimeprojection.MemorySelectionEvidence{}
	}
	evidence.CandidateMemoryIDs = memoryIDs(selection.Candidates)
	evidence.SelectedMemoryIDs = memoryIDs(selection.Selected)
	evidence.ExcludedMemoryIDs = memoryIDs(selection.Excluded)
	evidence.RetrievedMemoryIDs = append([]string(nil), evidence.SelectedMemoryIDs...)
	evidence.TypeQuota = append([]workorchmemorycontract.TypeQuotaEntry(nil), selection.TypeQuota...)
	evidence.SelectionScores = append([]workorchmemorycontract.RecallSelectionScore(nil), selection.SelectionScores...)
	evidence.ContextBudget = selection.ContextBudget
	evidence.SelectionPolicyVersion = memoryselection.RuntimeMemorySelectionPolicyVersion
	evidence.RetrievedMemoryHits = make([]runtimeprojection.MemorySelectionHit, 0, len(selection.RetrievedHits))
	for _, hit := range selection.RetrievedHits {
		evidence.RetrievedMemoryHits = append(evidence.RetrievedMemoryHits, runtimeprojection.MemorySelectionHit{
			ID: string(hit.ID), Summary: hit.Summary, Score: hit.VectorScore, VectorDimension: hit.VectorDimension,
		})
	}
	if evidence.Status != "degraded" {
		if len(selection.Selected) == 0 {
			evidence.Status = "none_selected"
		} else {
			evidence.Status = "selected"
		}
	}
	return selection.Selected, evidence, nil
}

func memoryIDs(items []*memorymodel.MemoryItem) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item != nil {
			out = append(out, string(item.ID))
		}
	}
	return out
}
