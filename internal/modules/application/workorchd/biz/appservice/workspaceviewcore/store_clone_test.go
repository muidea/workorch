package workspaceviewcore

import (
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestCloneWorkspaceClonesDefaultLLMSelection(t *testing.T) {
	target := runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "model-a"}
	cloned := cloneWorkspace(appmodelcore.Workspace{ID: "ws-model", DefaultLLMSelection: &runtimecontract.ProviderInferenceSelection{Target: target}})
	target.ModelID = "mutated"
	if cloned.DefaultLLMSelection == nil || cloned.DefaultLLMSelection.Target.ProviderID != "ai-proxy" || cloned.DefaultLLMSelection.Target.ModelID != "model-a" {
		t.Fatalf("cloned target = %+v", cloned.DefaultLLMSelection)
	}
}

func TestStoreDefinitionClarificationReadsDetachBindingAndOffendingRefs(t *testing.T) {
	workspace := appmodelcore.Workspace{ID: "ws-definition-clone", State: string(common.WorkspaceStateActive), ActiveRunID: "run-definition-clone"}
	input := readmodelcore.WorkspaceSnapshotState{
		Workspace: workspace,
		Runs: []appmodelcore.Run{{
			ID: "run-definition-clone", WorkspaceID: workspace.ID, State: string(common.RunStateBlockedOnDecision), BlockingTicketID: "ticket-definition-clone",
		}},
		Decisions: []projectioncontract.Decision{{
			ID: "ticket-definition-clone", WorkspaceID: workspace.ID, RunID: "run-definition-clone", State: string(common.DecisionTicketStateOpen),
			DefinitionClarification: &projectioncontract.DefinitionClarification{
				Class: "runtime_definition", BindingState: "open", RequiredAction: "revise_definition_and_restart",
				ViolationOrGapCode: "implicit_one_of", OffendingRefs: []string{"dod:item-1"},
			},
		}},
	}
	store := NewStore()
	store.ReplaceAll([]appmodelcore.Workspace{workspace}, map[string]readmodelcore.WorkspaceSnapshotState{workspace.ID: input}, nil)

	input.Decisions[0].DefinitionClarification.Class = "input-mutated"
	input.Decisions[0].DefinitionClarification.OffendingRefs[0] = "input-mutated"
	first, _, err := store.GetSnapshot(workspace.ID)
	if err != nil {
		t.Fatalf("get first snapshot: %v", err)
	}
	if got := first.Decisions[0].DefinitionClarification; got == nil || got.Class != "runtime_definition" || got.OffendingRefs[0] != "dod:item-1" {
		t.Fatalf("stored clarification aliases input: %+v", got)
	}
	first.Decisions[0].DefinitionClarification.Class = "read-mutated"
	first.Decisions[0].DefinitionClarification.OffendingRefs[0] = "read-mutated"

	second, _, err := store.GetSnapshot(workspace.ID)
	if err != nil {
		t.Fatalf("get second snapshot: %v", err)
	}
	summary, _, err := store.GetSummarySnapshot(workspace.ID)
	if err != nil {
		t.Fatalf("get summary snapshot: %v", err)
	}
	for name, snapshot := range map[string]readmodelcore.WorkspaceSnapshotState{"second": second, "summary": summary} {
		if len(snapshot.Decisions) != 1 || snapshot.Decisions[0].DefinitionClarification == nil ||
			snapshot.Decisions[0].DefinitionClarification.Class != "runtime_definition" ||
			snapshot.Decisions[0].DefinitionClarification.OffendingRefs[0] != "dod:item-1" {
			t.Fatalf("%s read aliases prior result: %+v", name, snapshot.Decisions)
		}
	}
}
