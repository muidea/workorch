package workorchport

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestCloneWorkspaceSnapshotInfoDetachesNestedProjectionValues(t *testing.T) {
	original := WorkspaceSnapshotInfo{
		Workspace: projectioncontract.Workspace{
			SoulBindings:     map[string]string{"execution": "soul-1"},
			DefaultLLMTarget: &runtimecontract.ProviderModelTarget{ProviderID: "provider-1", ModelID: "model-1"},
		},
		Projection: projectioncontract.ProjectionStatus{FailedWorkspaceIds: []string{"ws-1"}},
		Run: &projectioncontract.Run{
			SealedLLMTarget: &runtimecontract.ProviderModelTarget{ProviderID: "provider-1", ModelID: "model-1"},
		},
		SchedulerDiagnostics: &projectioncontract.SchedulerDiagnostics{
			RecentContinuations: []projectioncontract.ContinuationDiagnostic{{RecommendedActions: []string{"resume"}}},
		},
		CoverageSummary: workorcheventpayload.New(workorcheventpayload.StringArrayField("open", []string{"cwi-1"})),
		DoDs:            []workorchmanagementcontract.DoD{{Items: []string{"item-1"}}},
		Runs: []projectioncontract.Run{{
			SealedLLMTarget: &runtimecontract.ProviderModelTarget{ProviderID: "provider-2", ModelID: "model-2"},
		}},
	}

	cloned := CloneWorkspaceSnapshotInfo(original)
	cloned.Workspace.SoulBindings["execution"] = "changed"
	cloned.Workspace.DefaultLLMTarget.ModelID = "changed"
	cloned.Projection.FailedWorkspaceIds[0] = "changed"
	cloned.Run.SealedLLMTarget.ModelID = "changed"
	cloned.SchedulerDiagnostics.RecentContinuations[0].RecommendedActions[0] = "changed"
	cloned.CoverageSummary.Fields[0].Value.Array[0].String = "changed"
	cloned.DoDs[0].Items[0] = "changed"
	cloned.Runs[0].SealedLLMTarget.ModelID = "changed"

	if got := original.Workspace.SoulBindings["execution"]; got != "soul-1" {
		t.Fatalf("workspace binding mutated: %q", got)
	}
	if got := original.Workspace.DefaultLLMTarget.ModelID; got != "model-1" {
		t.Fatalf("workspace target mutated: %q", got)
	}
	if got := original.Projection.FailedWorkspaceIds[0]; got != "ws-1" {
		t.Fatalf("projection mutated: %q", got)
	}
	if got := original.Run.SealedLLMTarget.ModelID; got != "model-1" {
		t.Fatalf("run target mutated: %q", got)
	}
	if got := original.SchedulerDiagnostics.RecentContinuations[0].RecommendedActions[0]; got != "resume" {
		t.Fatalf("scheduler diagnostic mutated: %q", got)
	}
	if got := original.CoverageSummary.Fields[0].Value.Array[0].String; got != "cwi-1" {
		t.Fatalf("coverage summary mutated: %q", got)
	}
	if got := original.DoDs[0].Items[0]; got != "item-1" {
		t.Fatalf("DoD mutated: %q", got)
	}
	if got := original.Runs[0].SealedLLMTarget.ModelID; got != "model-2" {
		t.Fatalf("runs mutated: %q", got)
	}
}

func TestCloneWorkspaceRuntimeViewInfoDetachesNestedValues(t *testing.T) {
	original := WorkspaceRuntimeViewInfo{
		Projection: projectioncontract.ProjectionStatus{FailedWorkspaceIds: []string{"ws-1"}},
		Activity: projectioncontract.WorkspaceActivityFeed{
			Items: []projectioncontract.WorkspaceActivityItem{{ID: "activity-1"}},
		},
		BlockingDecision: &projectioncontract.Decision{AllowedRecoveryActions: []string{"continue"}},
	}

	cloned := CloneWorkspaceRuntimeViewInfo(original)
	cloned.Projection.FailedWorkspaceIds[0] = "changed"
	cloned.Activity.Items[0].ID = "changed"
	cloned.BlockingDecision.AllowedRecoveryActions[0] = "changed"

	if got := original.Projection.FailedWorkspaceIds[0]; got != "ws-1" {
		t.Fatalf("projection mutated: %q", got)
	}
	if got := original.Activity.Items[0].ID; got != "activity-1" {
		t.Fatalf("activity mutated: %q", got)
	}
	if got := original.BlockingDecision.AllowedRecoveryActions[0]; got != "continue" {
		t.Fatalf("decision mutated: %q", got)
	}
}
