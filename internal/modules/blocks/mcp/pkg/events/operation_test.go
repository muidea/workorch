package events

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestOwnerContractAndOperationMapper(t *testing.T) {
	contracts := Contracts()
	if len(contracts) != 6 || len(MessageContracts()) != 6 {
		t.Fatalf("contracts=%+v", contracts)
	}
	for _, contract := range contracts {
		if contract.Owner != "mcp" || !slices.Equal(contract.ReceiverOwners, []string{"mcp"}) {
			t.Fatalf("contract=%+v", contract)
		}
	}
	server, tool, path := "context", "lookup", "main.go"
	data, err := OperationDataFromRuntimeOperation(runtimecontract.RuntimeOperation{ID: "mcp-1", Kind: runtimecontract.StepKindMCPToolCall, Parameters: []runtimecontract.OperationParameter{{Name: "serverId", String: &server}, {Name: "tool", String: &tool}, {Name: "input.path", String: &path}}})
	if err != nil {
		t.Fatal(err)
	}
	if data.ServerID != server || data.Tool != tool || len(data.Input) != 1 || data.Input[0].Name != "path" || data.Input[0].String == nil || *data.Input[0].String != path {
		t.Fatalf("data=%+v", data)
	}
}

func TestSealedRuntimeOperationBindsOwnerCatalogAndExecutionScope(t *testing.T) {
	serverID, tool := "server-a", "lookup"
	operation := runtimecontract.RuntimeOperation{
		ID: "mcp-1", Kind: runtimecontract.StepKindMCPToolCall, TargetRefs: []string{"main.go"}, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		Parameters: []runtimecontract.OperationParameter{{Name: "serverId", String: &serverID}, {Name: "tool", String: &tool}},
	}
	capabilityID := common.MCPCapabilityID("capability-a")
	catalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "mcp-catalog", OwnerModuleID: runtimecontract.RuntimeModuleMCP,
		OperationKinds: []string{runtimecontract.StepKindMCPToolCall}, ObservedRevision: "registry-v1",
		MCPCapabilities: []runtimecontract.RuntimeMCPCapabilityContext{{
			ID: capabilityID, ServerID: common.MCPServerID(serverID), Kind: "tool", Name: tool,
			Enabled: true, Status: "enabled", EffectScope: string(runtimecontract.RuntimeEffectNone), IdempotencyClass: string(runtimecontract.RuntimeOperationIdempotencySafeToReplay),
		}},
		OperationPolicies: []runtimecontract.RuntimeOperationPolicyFact{{
			ID: "mcp-policy", OwnerModuleID: runtimecontract.RuntimeModuleMCP, OperationKind: runtimecontract.StepKindMCPToolCall, SubjectRef: string(capabilityID),
			ParameterMatches: []runtimecontract.RuntimeOperationParameterMatch{{Name: "serverId", String: &serverID}, {Name: "tool", String: &tool}},
			EffectScope:      runtimecontract.RuntimeEffectNone, IdempotencyClass: runtimecontract.RuntimeOperationIdempotencySafeToReplay,
		}},
	}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	sealed, err := SealRuntimeOperation(operation, catalog)
	if err != nil {
		t.Fatal(err)
	}
	context := runtimecontract.RuntimeOperationExecutionContext{
		WorkspaceID: "ws", RunID: "run", DispatchID: "dispatch",
		Execution: testExecutionEnvelope(operation, runtimecontract.RuntimeOperationExecutionEnvelope{
			ExecutionKey: "exec/ws/run/dispatch/mcp-1", Fence: 1, ExpectedEffectScope: runtimecontract.RuntimeEffectNone,
			AdmissibilitySnapshotID: catalog.SnapshotID, AcquisitionBinding: catalog.AcquisitionBinding,
		}, []runtimecontract.RuntimeOperationPolicyFact{sealed.Authorization.Policy}),
		OperationID: operation.ID, TargetRefs: append([]string(nil), operation.TargetRefs...), CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), operation.CoverageWorkItemIDs...),
	}
	got, authorization, err := sealed.ValidateForExecution(context)
	if err != nil || got.ID != operation.ID || authorization.Capability.ID != capabilityID || authorization.RegistryRevision != "registry-v1" {
		t.Fatalf("operation=%+v authorization=%+v err=%v", got, authorization, err)
	}
	idempotencyDrift := context
	idempotencyDrift.Execution.IdempotencyClass = runtimecontract.RuntimeOperationIdempotencyAtMostOnce
	if _, _, err := sealed.ValidateForExecution(idempotencyDrift); err == nil || !strings.Contains(err.Error(), "idempotency class mismatch") {
		t.Fatalf("sealed MCP operation accepted idempotency drift: %v", err)
	}
	authorityDrift := context
	authorityDrift.Execution.AuthorityHash = "forged"
	if _, _, err := sealed.ValidateForExecution(authorityDrift); err == nil || !strings.Contains(err.Error(), "authority hash mismatch") {
		t.Fatalf("sealed MCP operation accepted authority hash drift: %v", err)
	}
	drifted := sealed
	drifted.Authorization.Policy.EffectScope = runtimecontract.RuntimeEffectRemoteDurable
	if _, _, err := drifted.ValidateForExecution(context); err == nil {
		t.Fatal("sealed MCP operation accepted authorization drift")
	}
	wire, err := json.Marshal(ExecuteOperationCommand{RuntimeOperationExecutionContext: context, SealedOperation: sealed})
	if err != nil || bytes.Contains(wire, []byte(`"Data"`)) {
		t.Fatalf("sealed MCP command wire=%s err=%v", wire, err)
	}
}
