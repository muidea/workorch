package activationcore

import (
	"errors"
	"strings"
	"testing"

	memorycommon "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/common"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestSelectRuntimeMemoryContextEnforcesBudgetQuotaAndVisibility(t *testing.T) {
	candidates := []*memorymodel.MemoryItem{
		{ID: "project-1", MemoryType: memorycommon.MemoryTypeProjectContext, Summary: "one"},
		{ID: "project-2", MemoryType: memorycommon.MemoryTypeProjectContext, Summary: "two"},
		{ID: "project-3", MemoryType: memorycommon.MemoryTypeProjectContext, Summary: "three"},
		{ID: "hidden-tool", MemoryType: memorycommon.MemoryTypeToolFeedback, Summary: "retry", Tags: []string{"provider_retry"}},
		{ID: "too-large", MemoryType: memorycommon.MemoryTypeExecutionLesson, Content: strings.Repeat("x", RuntimeMemoryContextMaxBytes+1)},
		{ID: "decision-1", MemoryType: memorycommon.MemoryTypeDecision, Summary: "keep"},
	}
	selection, err := SelectRuntimeMemoryContext(candidates)
	if err != nil {
		t.Fatalf("select memory context: %v", err)
	}
	if got := MemoryItemIDs(selection.Selected); len(got) != 3 || got[0] != "project-1" || got[1] != "project-2" || got[2] != "decision-1" {
		t.Fatalf("selected memories = %#v", got)
	}
	if got := MemoryItemIDs(selection.Excluded); len(got) != 3 || got[0] != "project-3" || got[1] != "hidden-tool" || got[2] != "too-large" {
		t.Fatalf("excluded memories = %#v", got)
	}
	reasons := map[common.MemoryID]string{}
	for _, score := range selection.SelectionScores {
		reasons[score.ID] = score.Reason
	}
	if reasons["project-3"] != "excluded_type_quota" || reasons["hidden-tool"] != "excluded_visibility_policy" || reasons["too-large"] != "excluded_byte_budget" {
		t.Fatalf("selection reasons = %#v", reasons)
	}
	if selection.ContextBudget.UsedItems != len(selection.Selected) || selection.ContextBudget.UsedBytes > selection.ContextBudget.MaxBytes || selection.ContextBudget.EstimatedTokens > selection.ContextBudget.MaxEstimatedTokens {
		t.Fatalf("context budget = %+v", selection.ContextBudget)
	}
	if err := selection.ContextBudget.Validate(); err != nil {
		t.Fatalf("context budget validation: %v", err)
	}
	if len(selection.RetrievedHits) != len(selection.Selected) {
		t.Fatalf("retrieved hits = %d, selected = %d", len(selection.RetrievedHits), len(selection.Selected))
	}
}

func TestSelectRuntimeMemoryContextRejectsUnboundedCandidateSet(t *testing.T) {
	candidates := make([]*memorymodel.MemoryItem, 0, RuntimeMemorySelectionCandidateLimit+1)
	for idx := 0; idx <= RuntimeMemorySelectionCandidateLimit; idx++ {
		candidates = append(candidates, &memorymodel.MemoryItem{
			ID:         common.MemoryID(strings.Repeat("x", idx/10+1) + string(rune('a'+idx%10))),
			MemoryType: memorycommon.MemoryTypeProjectContext,
		})
	}
	if _, err := SelectRuntimeMemoryContext(candidates); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("candidate overflow error = %v, want invalid argument", err)
	}
}
