package readmodelprojection

import (
	"testing"

	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRunSnapshotProjectsUsedCapabilityFromCommittedRuntimeMCPDispatch(t *testing.T) {
	workspaceID := common.WorkspaceID("ws-runtime-used-capability")
	runID := common.RunID("run-runtime-used-capability")
	capabilityID := common.MCPCapabilityID("mcp-runtime:tool:repo.summary")
	serverID := "mcp-runtime"
	toolName := "repo.summary"
	dispatchID := "dispatch-runtime-mcp"
	operationID := "mcp-summary"
	operation := runtimecontract.RuntimeOperation{
		ID: operationID, Kind: runtimecontract.StepKindMCPToolCall,
		Parameters: []runtimecontract.OperationParameter{
			{Name: "serverId", String: &serverID},
			{Name: "tool", String: &toolName},
		},
	}
	dispatch := runtimecontract.RuntimeDispatchRecord{
		ID: dispatchID, State: runtimecontract.RuntimeDispatchStateCommitted,
		Transaction:           runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-runtime-mcp"},
		Candidate:             runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: operation}}}},
		CompletedOperationIDs: []string{operationID},
		OperationExecutions: []runtimecontract.RuntimeOperationExecutionRecord{{
			OperationID: operationID, State: runtimecontract.OperationExecutionEvidenceCommitted,
		}},
	}
	run := &runmodel.Run{
		ID: runID, WorkspaceID: workspaceID, State: common.RunStateCompleted,
		RuntimeBootstrap: &runtimecontract.RuntimeBootstrap{Activation: runtimecontract.RuntimeActivationContext{MCPCapabilities: []runtimecontract.RuntimeMCPCapabilityContext{{
			ID: capabilityID, ServerID: common.MCPServerID(serverID), Kind: mcpmodel.CapabilityKindTool, Name: toolName,
			DisplayName: "Repository summary", Enabled: true, Status: "active",
		}}}},
		RuntimeState: &runtimecontract.RuntimeStateRecord{CompletedDispatches: []runtimecontract.RuntimeDispatchRecord{dispatch}},
	}

	snapshot := runSnapshotFromModels(RunSnapshotModelInput{
		Workspace: &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive},
		Run:       run,
	})
	if len(snapshot.UsedCapabilities) != 1 {
		t.Fatalf("used capabilities = %+v, want one Runtime MCP capability", snapshot.UsedCapabilities)
	}
	used := snapshot.UsedCapabilities[0]
	if used.ID != string(capabilityID) || used.DisplayName != "Repository summary" || used.UseCount != 1 {
		t.Fatalf("used capability = %+v", used)
	}
	wantEvidence := "workorch://runtime-operations/" + dispatchID + "/" + operationID
	if len(used.EvidenceRefs) != 2 || used.EvidenceRefs[0] != wantEvidence || used.EvidenceRefs[1] != "authority-runtime-mcp" {
		t.Fatalf("used capability evidence = %+v, want %q and authority", used.EvidenceRefs, wantEvidence)
	}
	if !containsProjectionString(snapshot.TaskProgress.EvidenceRefs, wantEvidence) {
		t.Fatalf("task progress evidence = %+v, want completed Runtime operation %q", snapshot.TaskProgress.EvidenceRefs, wantEvidence)
	}
}

func containsProjectionString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
