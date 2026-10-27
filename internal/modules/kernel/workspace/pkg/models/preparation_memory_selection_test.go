package workspace

import (
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func memorySelectionEvaluationFixture(mode string, count int) *Workspace {
	w := evaluationWorkspaceFixture()
	a := &w.StartupPreparation.Attempts[0]
	s := &PreparationMemorySelectionSource{Version: PreparationMemorySelectionVersion, WorkspaceID: w.ID, WorkspaceRevision: a.ExpectedWorkspaceRevision + 1, SnapshotHash: a.ObservationSnapshotHash, ReobservedSnapshotHash: a.ObservationSnapshotHash, Before: PreparationMemoryModeObservation{ProviderID: runtimecontract.BuiltinAIProxyProviderID, ConfigurationRevision: "owner-revision"}, StartedAt: a.StartedAt, CompletedAt: a.CompletedAt}
	if mode == "semantic" {
		s.Before.DefaultEmbedding = &runtimecontract.ProviderModelTarget{ProviderID: runtimecontract.BuiltinAIProxyProviderID, ModelID: "embedding"}
	}
	s.After = s.Before.Clone()
	inventory := memorymodel.PreparationInventory{Version: memorymodel.PreparationInventoryVersion, WorkspaceID: w.ID, OwnerEpoch: "memory-preparation-epoch:" + strings.Repeat("A", 26), ObservedAt: a.StartedAt.Add(time.Millisecond), Items: []*memorymodel.MemoryItem{}}
	for i := 0; i < count; i++ {
		inventory.Items = append(inventory.Items, &memorymodel.MemoryItem{ID: common.MemoryID("id-" + strings.Repeat("a", i)), WorkspaceID: w.ID, MemoryType: "project_context", SourceType: "user", Content: "原文\r\n", Tags: []string{"source"}, Vector: []float64{0.25}})
	}
	f := PreparationMemoryInventoryFacts{Version: PreparationMemoryInventoryEvidenceVersion, WorkspaceRevision: s.WorkspaceRevision, Observation: inventory, SelectionSource: s}
	c := CompilePreparationMemorySelection(f)
	e := WorkspacePreparationMemoryInventoryEvidence{Version: PreparationMemoryInventoryEvidenceVersion, Ref: PreparationMemoryInventoryRefPrefix + strings.Repeat("c", 32), RequestID: a.RequestID, PayloadHash: a.PayloadHash, Binding: a.Binding, Facts: f, Selection: &c}
	e.ContentHash = e.Hash()
	w.PreparationMemoryInventoryEvidence = []WorkspacePreparationMemoryInventoryEvidence{e}
	a.MemoryInventoryEvidenceRef = e.Ref
	a.Checks = append(a.Checks, WorkspacePreparationCheckResult{ID: "memory_selection", Owner: "memory", Required: true, State: "blocked", Reason: "memory_inventory_observed_selection_pending"})
	sort.Slice(a.Checks, func(i, j int) bool { return a.Checks[i].ID < a.Checks[j].ID })
	a.Checks = EvaluatePreparationMemoryCheck(*a, &e)
	return w
}

func TestPreparationMemorySelectionOriginalModeBudgetAndNoFallback(t *testing.T) {
	for _, scenario := range []struct {
		mode          string
		count         int
		state, reason string
	}{
		{"list_only", 5, "passed", "memory_list_selection_observed_remaining_checks"},
		{"list_only", 0, "passed", "memory_empty_selection_observed_remaining_checks"},
		{"semantic", 0, "passed", "memory_empty_selection_observed_remaining_checks"},
		{"semantic", 1, "blocked", "memory_semantic_evidence_required"},
		{"list_only", 201, "blocked", "memory_selection_candidate_limit"},
	} {
		w := memorySelectionEvaluationFixture(scenario.mode, scenario.count)
		a, e := w.StartupPreparation.Attempts[0], w.PreparationMemoryInventoryEvidence[0]
		if e.Validate() != nil || w.StartupPreparation.Validate(w.ID) != nil || ValidatePreparationEvidencePersistence(w) != nil || string(a.Checks[2].State) != scenario.state || a.Checks[2].Reason != scenario.reason || a.State != WorkspacePreparationBlocked {
			t.Fatalf("original mode not respected: %+v, %+v", scenario, a.Checks[2])
		}
		if scenario.count == 5 && (len(e.Selection.Selection.Candidates) != 5 || len(e.Selection.Selection.Selected) != 2 || len(e.Selection.Selection.Excluded) != 3) {
			t.Fatal("quota or complete exclusion trace lost")
		}
	}
}

func TestPreparationMemorySelectionRejectsSelfConsistentSubstitutionAndKeepsRaw(t *testing.T) {
	for _, mutate := range []func(*WorkspacePreparationMemoryInventoryEvidence){
		func(e *WorkspacePreparationMemoryInventoryEvidence) { e.Facts.SelectionSource = nil },
		func(e *WorkspacePreparationMemoryInventoryEvidence) {
			e.Facts.SelectionSource.After.ConfigurationRevision = "other"
		},
		func(e *WorkspacePreparationMemoryInventoryEvidence) {
			e.Facts.SelectionSource.SnapshotHash = "other"
			e.Facts.SelectionSource.ReobservedSnapshotHash = "other"
		},
		func(e *WorkspacePreparationMemoryInventoryEvidence) {
			e.Selection.Selection.Selected[0].Content = "changed"
		},
		func(e *WorkspacePreparationMemoryInventoryEvidence) { e.Selection.Selection.Excluded = nil },
		func(e *WorkspacePreparationMemoryInventoryEvidence) { e.Selection.Selection.ContextBudget.MaxItems++ },
		func(e *WorkspacePreparationMemoryInventoryEvidence) { e.Facts.Observation.Items[0].Content = "changed" },
	} {
		w := memorySelectionEvaluationFixture("list_only", 5)
		e := &w.PreparationMemoryInventoryEvidence[0]
		mutate(e)
		e.ContentHash = e.Hash()
		if ValidatePreparationEvidencePersistence(w) == nil {
			t.Fatal("self-consistent/selected-only source forgery accepted")
		}
	}
	w := memorySelectionEvaluationFixture("semantic", 1)
	e := w.PreparationMemoryInventoryEvidence[0].Clone()
	e.Facts.SelectionSource.Before.DefaultEmbedding.ModelID = "other"
	if reflect.DeepEqual(e, w.PreparationMemoryInventoryEvidence[0]) {
		t.Fatal("source aliases")
	}
	a := &w.StartupPreparation.Attempts[0]
	a.Checks[2].Reason, a.Checks[2].State = "binding_changed", "blocked"
	if ValidatePreparationEvidencePersistence(w) != nil {
		t.Fatal("stale history lost")
	}
	a.EvaluationVersion, a.ObservationSnapshotHash = "", ""
	for i := range a.Checks {
		a.Checks[i].SubjectHash = ""
		a.Checks[i].State, a.Checks[i].Reason = "blocked", "binding_changed"
	}
	w.PreparationMemoryInventoryEvidence[0].Facts.SelectionSource = nil
	w.PreparationMemoryInventoryEvidence[0].Selection = nil
	w.PreparationMemoryInventoryEvidence[0].ContentHash = w.PreparationMemoryInventoryEvidence[0].Hash()
	if ValidatePreparationEvidencePersistence(w) != nil {
		t.Fatal("legacy inventory rejected")
	}
	if EvaluatePreparationMemoryCheck(*a, &w.PreparationMemoryInventoryEvidence[0])[2].State != "blocked" {
		t.Fatal("legacy upgraded")
	}
}
