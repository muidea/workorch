package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/events"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestMCPOwnerFailsClosedBeforeTransportForPostSourceSealAndRegistryDrift(t *testing.T) {
	ctx := context.Background()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"content": []any{}})
	}))
	t.Cleanup(server.Close)

	capability := testMCPToolCapability("mcp-authorized", "inspect", mcpmodel.ValidationProofApplicabilityDirect)
	owner, hub, receipts := newMCPExecutionTestOwner(t, &mcpmodel.Server{
		ID: "mcp-authorized", Name: "authorized", Transport: "http", Endpoint: server.URL,
		Enabled: true, AllowedTools: []string{"inspect"}, TimeoutMillis: 1000,
	}, []*mcpmodel.Capability{capability})
	operation := testMCPExecutionOperation("mcp-authorized", "inspect")
	sealed := sealMCPTestOperation(t, owner.runtimeAdapter, operation)
	command := testMCPExecutionCommand(t, operation, sealed, runtimecontract.RuntimeEffectNone, 1, nil, "")
	event := magicevent.NewEventWithContext(
		mcpevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, ModuleID,
		magicevent.NewHeader(), ctx, command,
	)

	// EventHub Post supplies no result. A strong execution command must not even
	// query authority, let alone contact the external MCP endpoint.
	owner.handleMCPOperation(event, nil)
	if requests.Load() != 0 || receipts.queryCount.Load() != 0 || receipts.commitCount.Load() != 0 {
		t.Fatalf("Post executed MCP work: requests=%d queries=%d commits=%d", requests.Load(), receipts.queryCount.Load(), receipts.commitCount.Load())
	}

	wrongSource := hub.Send(magicevent.NewEventWithContext(
		mcpevents.TopicExecuteOperation, runtimecontract.RuntimeModuleFile, ModuleID,
		magicevent.NewHeader(), ctx, command,
	))
	if wrongSource == nil {
		t.Fatal("wrong-source MCP execution returned no result")
	}
	if _, err := wrongSource.Get(); err == nil {
		t.Fatal("wrong-source MCP execution was accepted")
	}
	if requests.Load() != 0 || receipts.queryCount.Load() != 0 {
		t.Fatalf("wrong source reached MCP execution preflight: requests=%d queries=%d", requests.Load(), receipts.queryCount.Load())
	}

	drifted := command
	drifted.SealedOperation.PayloadHash = "drifted"
	driftedResult := sendMCPExecutionCommand(t, hub, drifted)
	if _, err := driftedResult.Get(); err == nil {
		t.Fatal("drifted sealed MCP operation was accepted")
	}
	if requests.Load() != 0 || receipts.queryCount.Load() != 0 {
		t.Fatalf("sealed payload drift reached receipt/transport: requests=%d queries=%d", requests.Load(), receipts.queryCount.Load())
	}
	tamperedExecution := command
	tamperedExecution.Execution.AdmissibilitySnapshotID = "another-owner-catalog"
	tamperedExecutionResult := sendMCPExecutionCommand(t, hub, tamperedExecution)
	if _, err := tamperedExecutionResult.Get(); err == nil {
		t.Fatal("MCP execution bound to another owner catalog was accepted")
	}
	if requests.Load() != 0 || receipts.queryCount.Load() != 0 {
		t.Fatalf("execution catalog drift reached receipt/transport: requests=%d queries=%d", requests.Load(), receipts.queryCount.Load())
	}

	// The authority query is deliberately before mutable Registry lookup so an
	// already committed receipt remains replayable. A missing receipt followed
	// by Registry drift must still fail before the external call.
	capability.Enabled = false
	registryDrift := sendMCPExecutionCommand(t, hub, command)
	if _, err := registryDrift.Get(); err == nil {
		t.Fatal("disabled current MCP capability was accepted")
	}
	if requests.Load() != 0 || receipts.queryCount.Load() != 1 || receipts.commitCount.Load() != 0 {
		t.Fatalf("Registry drift crossed the effect boundary: requests=%d queries=%d commits=%d", requests.Load(), receipts.queryCount.Load(), receipts.commitCount.Load())
	}
}

func TestMCPOwnerRequiresExactCapabilityApprovalBeforeTransport(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"content": []any{}})
	}))
	t.Cleanup(server.Close)

	capability := testMCPToolCapability("mcp-governed", "mutate", mcpmodel.ValidationProofApplicabilityNotDeclared)
	capability.ApprovalPolicy = "explicit"
	capability.RiskLevel = "external_change"
	owner, hub, _ := newMCPExecutionTestOwner(t, &mcpmodel.Server{
		ID: "mcp-governed", Name: "governed", Transport: "http", Endpoint: server.URL,
		Enabled: true, AllowedTools: []string{"mutate"}, TimeoutMillis: 1000,
	}, []*mcpmodel.Capability{capability})
	operation := testMCPExecutionOperation("mcp-governed", "mutate")
	sealed := sealMCPTestOperation(t, owner.runtimeAdapter, operation)
	first := testMCPExecutionCommand(t, operation, sealed, runtimecontract.RuntimeEffectNone, 1, nil, "")

	firstResult := sendMCPExecutionCommand(t, hub, first)
	firstData, err := firstResult.Get()
	if err != nil {
		t.Fatal(err)
	}
	governanceReceipt := firstData.(mcpevents.ExecuteOperationResult).Receipt
	if governanceReceipt.Outcome != runtimecontract.RuntimeOperationOutcomeGovernanceRequired || governanceReceipt.Governance == nil || governanceReceipt.Changed {
		t.Fatalf("MCP governance receipt = %+v", governanceReceipt)
	}
	if err := governanceReceipt.Validate(); err != nil {
		t.Fatalf("MCP governance receipt validation: %v", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("MCP endpoint called before approval: %d", requests.Load())
	}

	requirement := governanceReceipt.Governance.Normalized()
	grant := runtimecontract.RuntimeOperationGovernanceGrant{
		ResolutionID: "resolution-1", ApprovalID: "approval-1", ExecutionKey: first.Execution.ExecutionKey,
		GovernanceFence: 1, ResumeFence: 2, DependencyKind: string(requirement.Kind), SubjectRef: requirement.SubjectRef,
		RequestedAction: requirement.RequestedAction, RiskLevel: requirement.RiskLevel,
		AuthorizationHash: runtimecontract.StableRuntimeAuthorizationContextHash(requirement.AuthorizationContext), GovernanceReceiptHash: "governance-receipt-1",
	}
	wrongGrant := grant
	wrongGrant.AuthorizationHash = strings.Repeat("0", 64)
	wrong := testMCPExecutionCommand(t, operation, sealed, runtimecontract.RuntimeEffectNone, 2, &wrongGrant, "")
	wrongResult := sendMCPExecutionCommand(t, hub, wrong)
	if _, err := wrongResult.Get(); err == nil {
		t.Fatal("MCP execution accepted a drifted approval grant")
	}
	if requests.Load() != 0 {
		t.Fatalf("MCP endpoint called with drifted approval: %d", requests.Load())
	}

	approved := testMCPExecutionCommand(t, operation, sealed, runtimecontract.RuntimeEffectNone, 2, &grant, "")
	approvedResult := sendMCPExecutionCommand(t, hub, approved)
	approvedData, err := approvedResult.Get()
	if err != nil {
		t.Fatal(err)
	}
	receipt := approvedData.(mcpevents.ExecuteOperationResult).Receipt
	if receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || receipt.ObservedEffectScope != runtimecontract.RuntimeEffectNone || receipt.Changed {
		t.Fatalf("approved MCP receipt = %+v", receipt)
	}
	if requests.Load() != 1 {
		t.Fatalf("approved MCP request count = %d, want 1", requests.Load())
	}
}

func TestMCPOwnerPersistsBoundedPrivateOutputAndCrossesOnlyEvidenceReference(t *testing.T) {
	const providerSecret = "provider-private-secret-output"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]any{{"type": "text", "text": providerSecret}},
		})
	}))
	t.Cleanup(server.Close)

	capability := testMCPToolCapability("mcp-evidence", "inspect", mcpmodel.ValidationProofApplicabilityDirect)
	owner, hub, _ := newMCPExecutionTestOwner(t, &mcpmodel.Server{
		ID: "mcp-evidence", Name: "evidence", Transport: "http", Endpoint: server.URL,
		Enabled: true, AllowedTools: []string{"inspect"}, TimeoutMillis: 1000,
	}, []*mcpmodel.Capability{capability})
	operation := testMCPExecutionOperation("mcp-evidence", "inspect")
	sealed := sealMCPTestOperation(t, owner.runtimeAdapter, operation)
	internalDirectory := t.TempDir()
	command := testMCPExecutionCommand(t, operation, sealed, runtimecontract.RuntimeEffectNone, 1, nil, internalDirectory)

	result := sendMCPExecutionCommand(t, hub, command)
	data, err := result.Get()
	if err != nil {
		t.Fatal(err)
	}
	receipt := data.(mcpevents.ExecuteOperationResult).Receipt
	if receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || len(receipt.Evidence.ContentHash) != 64 || len(receipt.Evidence.EvidenceRefs) != 2 {
		t.Fatalf("MCP evidence receipt = %+v", receipt)
	}
	path := filepath.Join(internalDirectory, "mcp", "evidence", receipt.Evidence.ContentHash+".json")
	info, statErr := os.Stat(path)
	if statErr != nil {
		t.Fatalf("stat MCP private evidence: %v", statErr)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("MCP private evidence mode = %o, want 600", info.Mode().Perm())
	}
	raw, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !strings.Contains(string(raw), providerSecret) {
		t.Fatal("MCP owner-private evidence lost provider output")
	}
	wire, marshalErr := json.Marshal(receipt)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if strings.Contains(string(wire), providerSecret) || strings.Contains(string(wire), server.URL) {
		t.Fatalf("MCP provider output or endpoint crossed receipt boundary: %s", wire)
	}
	if !containsString(receipt.Evidence.EvidenceRefs, "workorch://mcp/evidence/"+receipt.Evidence.ContentHash) {
		t.Fatalf("MCP receipt lacks content-addressed evidence ref: %+v", receipt.Evidence)
	}
}

func TestMCPOwnerRequiresFactualDurableEffectObservation(t *testing.T) {
	tests := []struct {
		name        string
		response    map[string]any
		wantOutcome runtimecontract.RuntimeOperationOutcome
		wantScope   runtimecontract.RuntimeEffectScope
		wantChanged bool
		wantChanges int
	}{
		{
			name: "changed",
			response: map[string]any{"workorchEffect": map[string]any{
				"effectScope": "remote_durable", "changed": true, "beforeHash": "remote-v1", "afterHash": "remote-v2",
				"resourceChanges": []map[string]any{{"subjectRef": "remote-resource/item-a", "beforeRevision": "v1", "afterRevision": "v2"}},
			}},
			wantOutcome: runtimecontract.RuntimeOperationOutcomeAccepted, wantScope: runtimecontract.RuntimeEffectRemoteDurable, wantChanged: true, wantChanges: 1,
		},
		{
			name: "no-op",
			response: map[string]any{"workorchEffect": map[string]any{
				"effectScope": "remote_durable", "changed": false, "beforeHash": "remote-v2", "afterHash": "remote-v2",
			}},
			wantOutcome: runtimecontract.RuntimeOperationOutcomeNoOp, wantScope: runtimecontract.RuntimeEffectNone,
		},
		{
			name: "missing observation", response: map[string]any{"content": []any{}},
			wantOutcome: runtimecontract.RuntimeOperationOutcomeRejected, wantScope: runtimecontract.RuntimeEffectUnknown,
		},
		{
			name: "sensitive resource identity",
			response: map[string]any{"workorchEffect": map[string]any{
				"effectScope": "remote_durable", "changed": true, "beforeHash": "remote-v1", "afterHash": "remote-v2",
				"resourceChanges": []map[string]any{{"subjectRef": "https://user:secret@example.test/item", "beforeRevision": "v1", "afterRevision": "v2"}},
			}},
			wantOutcome: runtimecontract.RuntimeOperationOutcomeRejected, wantScope: runtimecontract.RuntimeEffectUnknown,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				_ = json.NewEncoder(w).Encode(tc.response)
			}))
			t.Cleanup(server.Close)

			capability := testMCPToolCapability("mcp-durable", "mutate", mcpmodel.ValidationProofApplicabilityNotDeclared)
			capability.EffectScope = string(runtimecontract.RuntimeEffectRemoteDurable)
			capability.IdempotencyClass = string(runtimecontract.RuntimeOperationIdempotencyAtMostOnce)
			owner, hub, _ := newMCPExecutionTestOwner(t, &mcpmodel.Server{
				ID: "mcp-durable", Name: "durable", Transport: "http", Endpoint: server.URL,
				Enabled: true, AllowedTools: []string{"mutate"}, TimeoutMillis: 1000,
			}, []*mcpmodel.Capability{capability})
			operation := testMCPExecutionOperation("mcp-durable", "mutate")
			sealed := sealMCPTestOperation(t, owner.runtimeAdapter, operation)
			command := testMCPExecutionCommand(t, operation, sealed, runtimecontract.RuntimeEffectRemoteDurable, 1, nil, "")

			result := sendMCPExecutionCommand(t, hub, command)
			data, err := result.Get()
			if err != nil {
				t.Fatal(err)
			}
			receipt := data.(mcpevents.ExecuteOperationResult).Receipt
			if receipt.Outcome != tc.wantOutcome || receipt.ObservedEffectScope != tc.wantScope || receipt.Changed != tc.wantChanged || len(receipt.ResourceChanges) != tc.wantChanges {
				t.Fatalf("durable MCP receipt = %+v", receipt)
			}
			if err := receipt.Validate(); err != nil {
				t.Fatalf("durable MCP receipt validation: %v", err)
			}
			if requests.Load() != 1 {
				t.Fatalf("durable MCP request count = %d, want 1", requests.Load())
			}
		})
	}
}

func TestMCPNonDurableEffectRejectsConflictingProviderObservation(t *testing.T) {
	authorization := mcpevents.RuntimeExecutionAuthorization{Policy: runtimecontract.RuntimeOperationPolicyFact{EffectScope: runtimecontract.RuntimeEffectNone}}
	response := MCPResponsePayload{JSON: json.RawMessage(`{"workorchEffect":{"effectScope":"remote_durable","changed":false,"beforeHash":"v1","afterHash":"v1"}}`)}
	if _, _, _, _, _, err := mcpReceiptEffectObservation(RuntimeOperationResult{Response: response}, authorization); err == nil {
		t.Fatal("non-durable MCP capability accepted a conflicting provider effect observation")
	}
}

func TestMCPOwnerBindsResourceAndPromptToExactCapabilityEffect(t *testing.T) {
	tests := []struct {
		name       string
		operation  runtimecontract.RuntimeOperation
		server     *mcpmodel.Server
		capability *mcpmodel.Capability
	}{
		{
			name: "resource",
			operation: runtimecontract.RuntimeOperation{ID: "resource-1", Kind: runtimecontract.StepKindMCPResourceRead, Parameters: []runtimecontract.OperationParameter{
				{Name: "serverId", String: stringPointer("mcp-read")}, {Name: "uri", String: stringPointer("file://README.md")},
			}},
			server: &mcpmodel.Server{ID: "mcp-read", Name: "read", Transport: "http", Enabled: true, AllowedResources: []string{"file://README.md"}, TimeoutMillis: 1000},
			capability: &mcpmodel.Capability{ID: "mcp-read:resource:README", ServerID: "mcp-read", Kind: mcpmodel.CapabilityKindResource,
				Name: "README", URI: "file://README.md", Enabled: true, EffectScope: string(runtimecontract.RuntimeEffectNone), IdempotencyClass: string(runtimecontract.RuntimeOperationIdempotencySafeToReplay)},
		},
		{
			name: "prompt",
			operation: runtimecontract.RuntimeOperation{ID: "prompt-1", Kind: runtimecontract.StepKindMCPPromptGet, Parameters: []runtimecontract.OperationParameter{
				{Name: "serverId", String: stringPointer("mcp-prompt")}, {Name: "prompt", String: stringPointer("delivery.plan")},
			}},
			server: &mcpmodel.Server{ID: "mcp-prompt", Name: "prompt", Transport: "http", Enabled: true, AllowedPrompts: []string{"delivery.plan"}, TimeoutMillis: 1000},
			capability: &mcpmodel.Capability{ID: "mcp-prompt:prompt:delivery.plan", ServerID: "mcp-prompt", Kind: mcpmodel.CapabilityKindPrompt,
				Name: "delivery.plan", Enabled: true, EffectScope: string(runtimecontract.RuntimeEffectNone), IdempotencyClass: string(runtimecontract.RuntimeOperationIdempotencySafeToReplay)},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"content": []any{}})
			}))
			t.Cleanup(server.Close)
			tc.server.Endpoint = server.URL
			owner, hub, _ := newMCPExecutionTestOwner(t, tc.server, []*mcpmodel.Capability{tc.capability})
			sealed := sealMCPTestOperation(t, owner.runtimeAdapter, tc.operation)
			command := testMCPExecutionCommand(t, tc.operation, sealed, runtimecontract.RuntimeEffectNone, 1, nil, "")
			result := sendMCPExecutionCommand(t, hub, command)
			data, err := result.Get()
			if err != nil {
				t.Fatal(err)
			}
			receipt := data.(mcpevents.ExecuteOperationResult).Receipt
			if receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || receipt.ObservedEffectScope != runtimecontract.RuntimeEffectNone {
				t.Fatalf("%s MCP receipt = %+v", tc.name, receipt)
			}
		})
	}
}

func TestValidateMCPOperationInputSchemaRejectsUnenforcedConstraints(t *testing.T) {
	capability := testMCPToolCapability("mcp-schema", "inspect", mcpmodel.ValidationProofApplicabilityDirect)
	capability.Schema = capability.Schema.Merge(workorcheventpayload.New(
		workorcheventpayload.ObjectField("properties", workorcheventpayload.New(
			workorcheventpayload.ObjectField("path", workorcheventpayload.New(
				workorcheventpayload.StringField("type", "string"),
				workorcheventpayload.StringArrayField("enum", []string{"allowed.txt"}),
			)),
		)),
	))
	spec := operationSpec{Operation: runtimecontract.StepKindMCPToolCall, Input: []runtimecontract.OperationParameter{{Name: "path", String: stringPointer("other.txt")}}}
	if err := validateMCPOperationInputSchema(capability, spec); err == nil || !strings.Contains(err.Error(), "keyword \"enum\" is unsupported") {
		t.Fatalf("unenforced MCP schema constraint error = %v", err)
	}
}

type mcpReceiptTestRecorder struct {
	queryCount  atomic.Int32
	commitCount atomic.Int32
	mu          sync.Mutex
	committed   []runtimecontract.RuntimeOperationReceipt
}

func newMCPExecutionTestOwner(t *testing.T, server *mcpmodel.Server, capabilities []*mcpmodel.Capability) (*Biz, magicevent.Hub, *mcpReceiptTestRecorder) {
	t.Helper()
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner, err := New(hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	owner.runtimeAdapter = NewRuntimeAdapter(staticMCPServerGetter{server: server, capabilities: capabilities})
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })
	recorder := installMCPReceiptTestObserver(t, hub)
	return owner, hub, recorder
}

func installMCPReceiptTestObserver(t *testing.T, hub magicevent.Hub) *mcpReceiptTestRecorder {
	t.Helper()
	recorder := &mcpReceiptTestRecorder{}
	observer := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	observer.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, func(_ magicevent.Event, result magicevent.Result) {
		recorder.queryCount.Add(1)
		result.Set(orchestratorevents.QueryRuntimeOperationReceiptResult{Found: false}, nil)
	})
	observer.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		recorder.commitCount.Add(1)
		command := event.Data().(orchestratorevents.CommitRuntimeOperationReceiptCommand)
		recorder.mu.Lock()
		recorder.committed = append(recorder.committed, command.Receipt.Clone())
		recorder.mu.Unlock()
		result.Set(orchestratorevents.CommitRuntimeOperationReceiptResult{Receipt: command.Receipt, StateRevision: uint64(recorder.commitCount.Load())}, nil)
	})
	t.Cleanup(func() {
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
	})
	return recorder
}

func testMCPExecutionOperation(serverID, tool string) runtimecontract.RuntimeOperation {
	return runtimecontract.RuntimeOperation{
		ID: "mcp-operation-1", Kind: runtimecontract.StepKindMCPToolCall, TargetRefs: []string{"target.txt"},
		Parameters: []runtimecontract.OperationParameter{
			{Name: "serverId", String: stringPointer(serverID)}, {Name: "tool", String: stringPointer(tool)},
			{Name: "input.path", String: stringPointer("target.txt")},
		},
	}
}

func testMCPExecutionCommand(t *testing.T, operation runtimecontract.RuntimeOperation, sealed mcpevents.SealedRuntimeOperation, effect runtimecontract.RuntimeEffectScope, fence uint64, grant *runtimecontract.RuntimeOperationGovernanceGrant, internalDirectory string) mcpevents.ExecuteOperationCommand {
	t.Helper()
	if internalDirectory == "" {
		internalDirectory = t.TempDir()
	}
	executionKey := runtimecontract.StableRuntimeOperationExecutionKey("ws-mcp", "run-mcp", "dispatch-mcp", operation.ID)
	if grant != nil {
		value := grant.Normalized()
		value.ExecutionKey = executionKey
		value.ResumeFence = fence
		grant = &value
	}
	evidenceKind := runtimecontract.EvidenceKindValidation
	if runtimecontract.RuntimeEffectIsDurable(effect) {
		evidenceKind = runtimecontract.EvidenceKindStateChange
	}
	return mcpevents.ExecuteOperationCommand{
		RuntimeOperationExecutionContext: runtimecontract.RuntimeOperationExecutionContext{
			Execution: runtimecontract.RuntimeOperationExecutionEnvelope{
				ExecutionKey: executionKey, Fence: fence, ExpectedEffectScope: effect,
				AdmissibilitySnapshotID: sealed.Authorization.CatalogSnapshotID, AcquisitionBinding: sealed.Authorization.AcquisitionBinding,
				GovernanceGrant: grant,
			},
			WorkspaceID: "ws-mcp", RunID: "run-mcp", DispatchID: "dispatch-mcp",
			Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-mcp"},
			OperationID: operation.ID, CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), operation.CoverageWorkItemIDs...),
			TargetRefs: append([]string(nil), operation.TargetRefs...), EvidenceKind: evidenceKind,
			WorkspaceDeliverableDirectory: t.TempDir(), WorkspaceInternalDirectory: internalDirectory,
		},
		SealedOperation: sealed,
		TargetRole:      "implementation",
	}
}

func sendMCPExecutionCommand(t *testing.T, hub magicevent.Hub, command mcpevents.ExecuteOperationCommand) magicevent.Result {
	t.Helper()
	result := hub.Send(magicevent.NewEventWithContext(
		mcpevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, ModuleID,
		magicevent.NewHeader(), context.Background(), command,
	))
	if result == nil {
		t.Fatal("MCP execution returned no result")
	}
	return result
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
