package workorchmemorycontract

import (
	"errors"
	"math"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func validRecallSelectionEvidence() RecallSelectionEvidence {
	return RecallSelectionEvidence{
		Policy: "test", Status: "selected",
		CandidateMemoryIDs: []common.MemoryID{"memory-1", "memory-2"},
		SelectedMemoryIDs:  []common.MemoryID{"memory-1"},
		ExcludedMemoryIDs:  []common.MemoryID{"memory-2"},
		SelectionScores: []RecallSelectionScore{
			{ID: "memory-1", Score: 10, Reason: "selected_within_context_budget"},
			{ID: "memory-2", Score: 5, Reason: "excluded_type_quota"},
		},
		RetrievedMemoryIDs:  []common.MemoryID{"memory-1"},
		RetrievedMemoryHits: []RecallHit{{ID: "memory-1", Summary: "selected"}},
		ContextBudget: RecallContextBudget{
			MaxItems: 8, MaxBytes: 16 * 1024, MaxEstimatedTokens: 4096,
			UsedItems: 1, UsedBytes: 8, EstimatedTokens: 2,
		},
	}
}

func TestRecallSelectionEvidenceRequiresCompleteBoundedTrajectory(t *testing.T) {
	valid := validRecallSelectionEvidence()
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid evidence: %v", err)
	}
	for name, mutate := range map[string]func(*RecallSelectionEvidence){
		"unclassified candidate": func(value *RecallSelectionEvidence) { value.ExcludedMemoryIDs = nil },
		"missing score":          func(value *RecallSelectionEvidence) { value.SelectionScores = value.SelectionScores[:1] },
		"missing hit":            func(value *RecallSelectionEvidence) { value.RetrievedMemoryHits = nil },
		"non finite score":       func(value *RecallSelectionEvidence) { value.MinScore = math.NaN() },
		"unknown exclusion": func(value *RecallSelectionEvidence) {
			value.ExcludedMemoryIDs = []common.MemoryID{"memory-3"}
		},
	} {
		t.Run(name, func(t *testing.T) {
			value := validRecallSelectionEvidence()
			mutate(&value)
			if err := value.Validate(); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("Validate error = %v, want invalid argument", err)
			}
		})
	}
}

func TestRecallSelectionEvidenceRejectsNestedCycleAndClones(t *testing.T) {
	valid := validRecallSelectionEvidence()
	nested := validRecallSelectionEvidence()
	valid.SemanticRecall = &nested
	valid.FallbackFrom = "semantic_vector_search"
	if err := valid.Validate(); err != nil {
		t.Fatalf("one-level nested evidence: %v", err)
	}
	deep := validRecallSelectionEvidence()
	nested.SemanticRecall = &deep
	valid.SemanticRecall = &nested
	if err := valid.Validate(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("nested evidence error = %v, want invalid argument", err)
	}

	valid = validRecallSelectionEvidence()
	clone := CloneRecallSelectionEvidence(&valid)
	valid.CandidateMemoryIDs[0] = "mutated"
	valid.RetrievedMemoryHits[0].Summary = "mutated"
	if clone.CandidateMemoryIDs[0] != "memory-1" || clone.RetrievedMemoryHits[0].Summary != "selected" {
		t.Fatalf("selection clone shares source storage: %+v", clone)
	}
}
