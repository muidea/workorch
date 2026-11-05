package runtime

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestSemanticFrontierKeepsCurrentRequestGroupAndOriginalObligations(t *testing.T) {
	_, _, same, manifest, _, _, _ := newTwoItemSemanticBatchFixture(t)
	if len(manifest.CoverageItems) != 2 {
		t.Fatal("complete current request group was mechanically split")
	}
	_, _, split, scoped, _, _, _ := newTwoItemSemanticBatchFixture(t, true)
	before, _ := json.Marshal(split)
	if len(scoped.CoverageItems) != 1 || scoped.CoverageItems[0].ID != split.Facts.Focus.CoverageWorkItemIDs[0] {
		t.Fatal("unrelated ready slice entered the current request")
	}
	all, err := rc.CompileRuntimeSemanticPlanManifestForCoverage(split.Facts.WorkspaceID, split.Facts.RunID, split.Definition, split.Facts.DefinitionSeed, split.Facts.DefinitionFocuses, split.Facts.Activation.ExecutionContract, split.Facts.Resources.OperationAdmissibilities, rc.RuntimeCWIsEligibleForPlanning(split.Facts))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(scoped.ProofObligations[0], all.ProofObligations[0]) || len(split.Facts.Coverage) != 2 || len(split.Facts.RemainingFocuses) != 1 {
		t.Fatal("scope selection removed original obligations or deferred work")
	}
	after, _ := json.Marshal(split)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("compiler changed owner facts")
	}
	// The focus remains the sole selected task. A partially ready/malformed
	// member set cannot certify a smaller task by silently dropping members.
	same.Facts.Focus.CoverageWorkItemIDs = append(same.Facts.Focus.CoverageWorkItemIDs, "missing")
	if !reflect.DeepEqual(runtimeSemanticPlanningFrontier(same.Facts), rc.RuntimeCWIsEligibleForPlanning(same.Facts)) {
		t.Fatal("incomplete focus was silently split")
	}
	duplicate := split.Facts
	duplicate.Focus.CoverageWorkItemIDs = []common.CoverageWorkItemID{scoped.CoverageItems[0].ID, scoped.CoverageItems[0].ID}
	if !reflect.DeepEqual(runtimeSemanticPlanningFrontier(duplicate), rc.RuntimeCWIsEligibleForPlanning(duplicate)) {
		t.Fatal("duplicate focus narrowed authority")
	}
}

func TestSemanticFrontierCompletionAdvancesIndependentSliceAndSurvivesReload(t *testing.T) {
	c, repo, a, m, order, proposals, feasibilities := newTwoItemSemanticBatchFixture(t, true)
	a = sealFirstTwoItemSemanticBatch(t, c, a, m, order, proposals[:1], feasibilities[:1])
	second := a.Facts.Coverage[1].ID
	if len(runtimeSemanticCompletedFrontierRefs(a)) != 0 || len(runtimeOrdinarySemanticEligibleClosure(a.Facts, m, []common.CoverageWorkItemID{second})) != 0 {
		t.Fatal("bound unfinished frontier lost its fence")
	}
	blocked := a.CloneRaw()
	if err := retireSemanticPlanningGenerationForEligibilityChange(&blocked, []common.CoverageWorkItemID{second}); err == nil || len(blocked.SemanticPlanning.History) != 0 {
		t.Fatal("unfinished frontier retired without completion lineage")
	}
	if err := runtimeTestAdvanceSemanticFocus(t, repo, &a); err != nil {
		t.Fatal(err)
	}
	if a.Facts.Focus.CoverageWorkItemIDs[0] != second || a.SemanticPlanning.Phase != rc.RuntimeSemanticPlanningRequired {
		t.Fatal("independent slice did not enter the next ordinary generation")
	}
	history := a.SemanticPlanning.History
	if len(history) != 1 || history[0].Reason != rc.RuntimeSemanticPlanningRetirementFrontierCompleted || len(history[0].ClosureChangeTriggerRefs) != 1 || !reflect.DeepEqual(history[0].ChangedCoverageIDs, []common.CoverageWorkItemID{second}) {
		t.Fatalf("completion provenance missing: %+v", history)
	}
	invalid := history[0].Clone()
	invalid.Snapshot.Phase = rc.RuntimeSemanticPlanningOrderAccepted
	if invalid.Validate() == nil {
		t.Fatal("frontier retirement accepted a partial generation")
	}
	invalid = history[0].Clone()
	invalid.ClosureChangeTriggerRefs = nil
	if invalid.Validate() == nil {
		t.Fatal("frontier retirement lost its completion bindings")
	}
	if a.Facts.Coverage[1].State != rc.RuntimeCoverageItemStateOpen || a.ActiveDispatch != nil || len(a.Facts.Focus.CoverageEvidenceBindings) != 0 {
		t.Fatal("frontier transition fabricated proof credit")
	}
	// A fresh controller must reconstruct the same current contract from the
	// persisted owner, without retaining a compiler result or old dispatch.
	fresh, err := NewRuntimeController(repo)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := fresh.Load(context.Background(), a.Facts.WorkspaceID, a.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	next, err := CompileRuntimeSemanticManifest(reloaded)
	if err != nil || len(next.CoverageItems) != 1 || next.CoverageItems[0].ID != second {
		t.Fatal("reload changed next frontier", err)
	}
	if err := validateSemanticManifestIdentity(reloaded, m); err == nil {
		t.Fatal("stale prior-frontier manifest admitted")
	}
	if err := retireSemanticPlanningGenerationForEligibilityChange(&reloaded, []common.CoverageWorkItemID{second}); err != nil || len(reloaded.SemanticPlanning.History) != 1 {
		t.Fatal("duplicate retirement changed history", err)
	}
}
