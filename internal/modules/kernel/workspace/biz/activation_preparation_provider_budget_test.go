package workspace

import (
	"context"
	"testing"

	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestPreparationProviderBudgetContinuationPinsOriginalAndCannotReplay(t *testing.T) {
	ctx := context.Background()
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	w := seedPreparationEligibleWithConfiguration(t, owners, "ws-budget-continuation", "delivery", func(w *workspace.Workspace) {
		w.DefaultLLMSelection = &runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "original-model"}}
	})
	c := workspaceevents.CheckWorkspacePreparationCommand{WorkspaceID: w.ID, RequestID: "original-budget", ExpectedWorkspaceRevision: w.Revision}
	r, err := owners.workspace.beginWorkspacePreparation(ctx, c, false, true)
	if err != nil || !r.ObservationRequired || r.ProviderDefinition == nil || r.ProviderDefinition.WorkspaceDefault == nil || r.ProviderDefinition.WorkspaceDefault.Target.ModelID != "original-model" || r.ValidateFor(c) != nil {
		t.Fatal("original declaration missing", err)
	}
	bad := r
	bad.ObservationRequired = false
	if bad.ValidateFor(c) == nil {
		t.Fatal("declaration escaped first continuation")
	}
	r.ProviderDefinition.WorkspaceDefault.Target.ModelID = "caller"
	for _, recovery := range []bool{false, true} {
		again, err := owners.workspace.beginWorkspacePreparation(ctx, c, recovery, true)
		if err != nil || again.ObservationRequired || again.ProviderDefinition != nil {
			t.Fatal("replay/recovery authorized new observation", err)
		}
	}
	stored, err := owners.repositories.Workspaces.Get(ctx, w.ID)
	if err != nil || stored.DefaultLLMSelection.Target.ModelID != "original-model" {
		t.Fatal("continuation aliased original declaration", err)
	}
}
