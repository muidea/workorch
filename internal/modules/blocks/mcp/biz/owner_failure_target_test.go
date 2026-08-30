package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/events"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	workorcheventpayload "github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestMCPFailureTargetsPreferOwnerDiagnosticsOverOperationTarget(t *testing.T) {
	result := RuntimeOperationResult{
		TargetRefs:  []string{"validation-entry.go"},
		Diagnostics: MCPCapabilityFailureDiagnostics{FailureTargets: []string{"implementation.go", "implementation.go", ""}},
	}
	if got, want := mcpFailureTargets(result), []string{"implementation.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("mcpFailureTargets() = %#v, want %#v", got, want)
	}
}

func TestMCPFailureTargetsDoNotFallBackToOperationTarget(t *testing.T) {
	result := RuntimeOperationResult{TargetRefs: []string{"validation-entry.go", "validation-entry.go", ""}}
	if got := mcpFailureTargets(result); len(got) != 0 {
		t.Fatalf("mcpFailureTargets() = %#v, want no unobserved failure targets", got)
	}
}

func TestMCPFailureTargetObservationRequiresBoundedOwnerFields(t *testing.T) {
	evidenceHash := strings.Repeat("a", 64)
	evidence := runtimecontract.RuntimeOperationEvidence{
		ContentHash:    evidenceHash,
		ContentAddress: &runtimecontract.RuntimeEvidenceContentAddress{Algorithm: runtimecontract.RuntimeEvidenceDigestSHA256, Digest: evidenceHash, Ref: "workorch://mcp/evidence/" + evidenceHash},
		EvidenceRefs:   []string{"workorch://mcp/capability/server-a/revision-1", "workorch://mcp/evidence/" + evidenceHash},
	}
	result := RuntimeOperationResult{
		ServerID: "server-a", OwnerRevision: "mcp-server-v1",
		Diagnostics: MCPCapabilityFailureDiagnostics{TargetRole: "implementation", FailureTargets: []string{" implementation.go ", "implementation.go"}},
	}
	observation := mcpFailureTargetObservation(result, evidence)
	if observation == nil {
		t.Fatal("mcpFailureTargetObservation() = nil, want owner observation")
	}
	if observation.OwnerModuleID != ModuleID || observation.SubjectRef != "server-a" || observation.TargetRole != "implementation" ||
		observation.ObservedRevision != "mcp-server-v1" || !reflect.DeepEqual(observation.TargetRefs, []string{"implementation.go"}) ||
		observation.DiagnosticClass != runtimecontract.RuntimeFailureTargetDiagnosticReceiptEvidence || observation.ContentHash != evidence.ContentHash ||
		!reflect.DeepEqual(observation.EvidenceRefs, evidence.EvidenceRefs) {
		t.Fatalf("observation = %+v", observation)
	}
	for _, invalid := range []RuntimeOperationResult{
		{ServerID: "server-a", OwnerRevision: "mcp-server-v1", Diagnostics: MCPCapabilityFailureDiagnostics{FailureTargets: []string{"implementation.go"}}},
		{ServerID: "server-a", Diagnostics: MCPCapabilityFailureDiagnostics{TargetRole: "implementation", FailureTargets: []string{"implementation.go"}}},
		{ServerID: "server-a", OwnerRevision: "mcp-server-v1", Diagnostics: MCPCapabilityFailureDiagnostics{TargetRole: "implementation"}},
	} {
		if got := mcpFailureTargetObservation(invalid, evidence); got != nil {
			t.Fatalf("mcpFailureTargetObservation(%+v) = %+v, want nil", invalid, got)
		}
	}
	if got := mcpFailureTargetObservation(result, runtimecontract.RuntimeOperationEvidence{Summary: "evidence persistence failed"}); got != nil {
		t.Fatalf("observation without persisted owner evidence = %+v, want nil", got)
	}
}

func TestMCPFailureTargetOverflowDoesNotTruncateIntoAuthority(t *testing.T) {
	targets := make([]string, 0, runtimecontract.RuntimeFailureTargetObservationMaxTargetRefs+1)
	for index := 0; index <= runtimecontract.RuntimeFailureTargetObservationMaxTargetRefs; index++ {
		targets = append(targets, fmt.Sprintf("target-%02d.go", index))
	}
	result := RuntimeOperationResult{
		ServerID: "server-a", OwnerRevision: "mcp-server-v1",
		Diagnostics: MCPCapabilityFailureDiagnostics{TargetRole: "implementation", FailureTargets: targets},
	}
	if got := mcpFailureTargets(result); got != nil {
		t.Fatalf("overflow targets were truncated into authority: %+v", got)
	}
	raw, err := json.Marshal(map[string]any{"failureTargets": targets})
	if err != nil {
		t.Fatal(err)
	}
	if got := observedMCPFailureTargets(MCPResponsePayload{JSON: raw}); got != nil {
		t.Fatalf("response overflow was truncated into authority: %+v", got)
	}
	evidenceHash := strings.Repeat("a", 64)
	if got := mcpFailureTargetObservation(result, runtimecontract.RuntimeOperationEvidence{
		ContentHash:    evidenceHash,
		ContentAddress: &runtimecontract.RuntimeEvidenceContentAddress{Algorithm: runtimecontract.RuntimeEvidenceDigestSHA256, Digest: evidenceHash, Ref: "workorch://mcp/evidence/" + evidenceHash},
		EvidenceRefs:   []string{"workorch://mcp/evidence/" + evidenceHash},
	}); got != nil {
		t.Fatalf("overflow observation = %+v, want nil", got)
	}
}

// The adapter tests cover response parsing.  This owner-level regression
// proves the parsed observation survives the real typed EventHub execution
// path and receipt command: the validation entry remains operation lineage,
// while only the explicit MCP response field becomes a related failure target.
func TestMCPOwnerEmitsStructuredFailureTargetObservationThroughTypedReceipt(t *testing.T) {
	ctx := context.Background()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content":        []map[string]any{{"type": "text", "text": "implementation failed"}},
			"failureTargets": []string{"implementation.go"},
			"isError":        true,
		})
	}))
	defer server.Close()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })

	owner, err := New(hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	capability := testMCPToolCapability("mcp-observed-target", "lookup.references", mcpmodel.ValidationProofApplicabilityDirect)
	owner.runtimeAdapter = NewRuntimeAdapter(staticMCPServerGetter{server: &mcpmodel.Server{
		ID: "mcp-observed-target", Name: "observed target", Transport: "http", Endpoint: server.URL,
		Enabled: true, AllowedTools: []string{"lookup.references"}, TimeoutMillis: 1000,
	}, capabilities: []*mcpmodel.Capability{capability}})
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })

	var committed runtimecontract.RuntimeOperationReceipt
	receipts := map[string]runtimecontract.RuntimeOperationReceipt{}
	receiptOwner := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	receiptOwner.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.QueryRuntimeOperationReceiptCommand)
		receipt, found := receipts[command.ExecutionKey]
		response := orchestratorevents.QueryRuntimeOperationReceiptResult{Found: found}
		if found {
			response.Receipt = &receipt
		}
		result.Set(response, nil)
	})
	receiptOwner.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.CommitRuntimeOperationReceiptCommand)
		if receipt, found := receipts[command.Receipt.ExecutionKey]; found {
			result.Set(orchestratorevents.CommitRuntimeOperationReceiptResult{Receipt: receipt, StateRevision: 7}, nil)
			return
		}
		committed = command.Receipt.Clone()
		receipts[command.Receipt.ExecutionKey] = committed
		result.Set(orchestratorevents.CommitRuntimeOperationReceiptResult{Receipt: committed, StateRevision: 7}, nil)
	})
	t.Cleanup(func() {
		receiptOwner.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
		receiptOwner.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
	})

	operation := runtimecontract.RuntimeOperation{
		ID: "validate-entry", Kind: runtimecontract.StepKindMCPToolCall,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"validation-entry.go"},
		Parameters: []runtimecontract.OperationParameter{
			{Name: "serverId", String: stringPointer("mcp-observed-target")}, {Name: "tool", String: stringPointer("lookup.references")},
			{Name: "input.path", String: stringPointer("validation-entry.go")},
		},
	}
	sealed := sealMCPTestOperation(t, owner.runtimeAdapter, operation)
	command := mcpevents.ExecuteOperationCommand{
		RuntimeOperationExecutionContext: runtimecontract.RuntimeOperationExecutionContext{
			Execution: mcpTestExecutionEnvelope(operation, runtimecontract.RuntimeOperationExecutionEnvelope{
				ExecutionKey: "exec/ws-v2/run-v2/dispatch-1/validate-entry", Fence: 1,
				ExpectedEffectScope: runtimecontract.RuntimeEffectNone, AdmissibilitySnapshotID: sealed.Authorization.CatalogSnapshotID,
				AcquisitionBinding: sealed.Authorization.AcquisitionBinding,
			}, sealed.Authorization.Policy),
			WorkspaceID: "ws-v2", RunID: "run-v2", DispatchID: "dispatch-1",
			Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-1"},
			OperationID: "validate-entry", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"validation-entry.go"},
			EvidenceKind: runtimecontract.EvidenceKindValidation, WorkspaceDeliverableDirectory: t.TempDir(), WorkspaceInternalDirectory: t.TempDir(),
		},
		SealedOperation: sealed,
		TargetRole:      "implementation",
	}
	command.Execution.AdmissibilitySnapshotID = sealed.Authorization.CatalogSnapshotID
	command.Execution.AcquisitionBinding = sealed.Authorization.AcquisitionBinding
	result := hub.Send(magicevent.NewEventWithContext(
		mcpevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, ModuleID,
		magicevent.NewHeader(), ctx, command,
	))
	if result == nil {
		t.Fatal("MCP owner returned no execution result")
	}
	data, resultErr := result.Get()
	if resultErr != nil {
		t.Fatalf("MCP owner execution result error=%#v resultError=%#v", resultErr, result.Error())
	}
	response, ok := data.(mcpevents.ExecuteOperationResult)
	if !ok {
		t.Fatalf("MCP owner result type = %T", data)
	}
	diagnostic := response.Receipt.Feedback.OriginalDiagnostic
	if response.Receipt.Outcome != runtimecontract.RuntimeOperationOutcomeRejected || diagnostic == nil ||
		!reflect.DeepEqual(diagnostic.TargetRefs, []string{"implementation.go"}) ||
		diagnostic.TargetObservation == nil || diagnostic.TargetObservation.TargetRole != "implementation" ||
		!reflect.DeepEqual(diagnostic.TargetObservation.TargetRefs, []string{"implementation.go"}) ||
		diagnostic.TargetObservation.DiagnosticClass != runtimecontract.RuntimeFailureTargetDiagnosticReceiptEvidence ||
		diagnostic.TargetObservation.ContentHash != response.Receipt.Evidence.ContentHash ||
		!reflect.DeepEqual(diagnostic.TargetObservation.EvidenceRefs, response.Receipt.Evidence.EvidenceRefs) {
		t.Fatalf("MCP owner failure result = %+v", response)
	}
	if !reflect.DeepEqual(response.Receipt.TargetRefs, []string{"validation-entry.go"}) ||
		!reflect.DeepEqual(diagnostic.TargetObservation.TargetRefs, []string{"implementation.go"}) {
		t.Fatalf("MCP owner receipt lineage = %+v", response.Receipt)
	}
	committedDiagnostic := committed.Feedback.OriginalDiagnostic
	if committed.ExecutionKey != command.Execution.ExecutionKey || committed.Outcome != runtimecontract.RuntimeOperationOutcomeRejected ||
		!reflect.DeepEqual(committed.TargetRefs, []string{"validation-entry.go"}) || committedDiagnostic == nil ||
		committedDiagnostic.TargetObservation == nil || !reflect.DeepEqual(committedDiagnostic.TargetObservation.TargetRefs, []string{"implementation.go"}) {
		t.Fatalf("committed MCP receipt = %+v", committed)
	}
	if response.Receipt.Feedback == nil || response.Receipt.Feedback.Domain != runtimecontract.RuntimeFeedbackDomainOperation ||
		response.Receipt.Feedback.WorkspaceID != command.WorkspaceID || response.Receipt.Feedback.RunID != command.RunID ||
		response.Receipt.Feedback.AuthoritySnapshotID != command.Transaction.AuthoritySnapshotID || response.Receipt.Feedback.DispatchID != command.DispatchID ||
		response.Receipt.Feedback.OriginalDiagnostic == nil || response.Receipt.Feedback.OriginalDiagnostic.OwnerModuleID != runtimecontract.RuntimeModuleMCP ||
		response.Receipt.Feedback.OriginalDiagnostic.OperationID != command.OperationID || response.Receipt.Feedback.OriginalDiagnostic.OriginalCode != diagnostic.OriginalCode ||
		response.Receipt.Feedback.OriginalDiagnostic.OriginalMessage != diagnostic.OriginalMessage {
		t.Fatalf("MCP owner receipt lost scoped original feedback: %+v", response.Receipt.Feedback)
	}
	duplicateResult := hub.Send(magicevent.NewEventWithContext(
		mcpevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, ModuleID,
		magicevent.NewHeader(), ctx, command,
	))
	duplicateData, duplicateErr := duplicateResult.Get()
	if duplicateErr != nil {
		t.Fatalf("duplicate MCP owner execution: %v", duplicateErr)
	}
	duplicate, ok := duplicateData.(mcpevents.ExecuteOperationResult)
	if !ok || !runtimecontract.SameRuntimeOperationReceipt(duplicate.Receipt, response.Receipt) || requests.Load() != 1 {
		t.Fatalf("duplicate MCP receipt repeated owner effect: response=%+v requests=%d", duplicate, requests.Load())
	}

	owner.Teardown(ctx)
	restarted, err := New(hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	restarted.runtimeAdapter = NewRuntimeAdapter(staticMCPServerGetter{server: &mcpmodel.Server{
		ID: "mcp-observed-target", Name: "observed target", Transport: "http", Endpoint: server.URL,
		Enabled: true, AllowedTools: []string{"lookup.references"}, TimeoutMillis: 1000,
	}, capabilities: []*mcpmodel.Capability{capability}})
	if err := restarted.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { restarted.Teardown(ctx) })
	restartedResult := hub.Send(magicevent.NewEventWithContext(
		mcpevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, ModuleID,
		magicevent.NewHeader(), ctx, command,
	))
	restartedData, restartedErr := restartedResult.Get()
	if restartedErr != nil {
		t.Fatalf("fresh MCP owner receipt replay: %v", restartedErr)
	}
	restartedResponse, ok := restartedData.(mcpevents.ExecuteOperationResult)
	if !ok || !runtimecontract.SameRuntimeOperationReceipt(restartedResponse.Receipt, response.Receipt) || requests.Load() != 1 {
		t.Fatalf("fresh MCP owner repeated provider invocation: response=%+v requests=%d", restartedResponse, requests.Load())
	}
}

func TestMCPOwnerEvidencePersistenceFailureEmitsNoDirectTargetObservation(t *testing.T) {
	ctx := context.Background()
	internalDirectory := t.TempDir()
	evidenceDirectory, err := mcpEvidenceDirectory(internalDirectory)
	if err != nil {
		t.Fatal(err)
	}
	var sabotageErr atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if err := os.RemoveAll(evidenceDirectory); err != nil {
			sabotageErr.Store(err.Error())
		}
		if err := os.WriteFile(evidenceDirectory, []byte("not a directory"), 0o600); err != nil {
			sabotageErr.Store(err.Error())
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content":        []map[string]any{{"type": "text", "text": "implementation failed"}},
			"failureTargets": []string{"implementation.go"},
			"isError":        true,
		})
	}))
	defer server.Close()

	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner, err := New(hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	capability := testMCPToolCapability("mcp-evidence-failure", "lookup.references", mcpmodel.ValidationProofApplicabilityDirect)
	owner.runtimeAdapter = NewRuntimeAdapter(staticMCPServerGetter{server: &mcpmodel.Server{
		ID: "mcp-evidence-failure", Name: "evidence failure", Transport: "http", Endpoint: server.URL,
		Enabled: true, AllowedTools: []string{"lookup.references"}, TimeoutMillis: 1000,
	}, capabilities: []*mcpmodel.Capability{capability}})
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })

	receiptOwner := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	receiptOwner.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(orchestratorevents.QueryRuntimeOperationReceiptResult{Found: false}, nil)
	})
	receiptOwner.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.CommitRuntimeOperationReceiptCommand)
		result.Set(orchestratorevents.CommitRuntimeOperationReceiptResult{Receipt: command.Receipt, StateRevision: 1}, nil)
	})
	t.Cleanup(func() {
		receiptOwner.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
		receiptOwner.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
	})

	operation := runtimecontract.RuntimeOperation{
		ID: "validate-entry", Kind: runtimecontract.StepKindMCPToolCall,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"validation-entry.go"},
		Parameters: []runtimecontract.OperationParameter{
			{Name: "serverId", String: stringPointer("mcp-evidence-failure")}, {Name: "tool", String: stringPointer("lookup.references")},
			{Name: "input.path", String: stringPointer("validation-entry.go")},
		},
	}
	sealed := sealMCPTestOperation(t, owner.runtimeAdapter, operation)
	command := mcpevents.ExecuteOperationCommand{
		RuntimeOperationExecutionContext: runtimecontract.RuntimeOperationExecutionContext{
			Execution: mcpTestExecutionEnvelope(operation, runtimecontract.RuntimeOperationExecutionEnvelope{
				ExecutionKey: "exec/ws-v2/run-v2/dispatch-1/validate-entry", Fence: 1,
				ExpectedEffectScope: runtimecontract.RuntimeEffectNone, AdmissibilitySnapshotID: sealed.Authorization.CatalogSnapshotID,
				AcquisitionBinding: sealed.Authorization.AcquisitionBinding,
			}, sealed.Authorization.Policy),
			WorkspaceID: "ws-v2", RunID: "run-v2", DispatchID: "dispatch-1",
			Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-1"},
			OperationID: "validate-entry", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"validation-entry.go"},
			EvidenceKind: runtimecontract.EvidenceKindValidation, WorkspaceDeliverableDirectory: t.TempDir(), WorkspaceInternalDirectory: internalDirectory,
		},
		SealedOperation: sealed,
		TargetRole:      "implementation",
	}
	result := hub.Send(magicevent.NewEventWithContext(
		mcpevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, ModuleID,
		magicevent.NewHeader(), ctx, command,
	))
	if result == nil {
		t.Fatal("MCP owner returned no execution result")
	}
	data, resultErr := result.Get()
	if resultErr != nil {
		t.Fatalf("MCP owner execution result error=%#v resultError=%#v", resultErr, result.Error())
	}
	if value := sabotageErr.Load(); value != nil {
		t.Fatalf("failed to force MCP evidence persistence failure: %v", value)
	}
	response, ok := data.(mcpevents.ExecuteOperationResult)
	if !ok || response.Receipt.Outcome != runtimecontract.RuntimeOperationOutcomeRejected || response.Receipt.Feedback == nil ||
		response.Receipt.Feedback.OriginalDiagnostic == nil {
		t.Fatalf("MCP evidence-persistence failure result = %#v", data)
	}
	diagnostic := response.Receipt.Feedback.OriginalDiagnostic
	if diagnostic.TargetObservation != nil || len(diagnostic.TargetRefs) != 0 || response.Receipt.Evidence.ContentHash != "" || len(response.Receipt.Evidence.EvidenceRefs) != 0 {
		t.Fatalf("MCP evidence-persistence failure leaked target authority: receipt=%+v", response.Receipt)
	}
}

// C12: an accepted validation-scoped MCP tool invocation may attest only its
// exact sealed server/tool identity. The owner sends this fact through the
// normal typed receipt command; it neither sees nor grants a coverage item.
func TestMCPOwnerEmitsExactToolValidationObservationThroughTypedReceipt(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]any{{"type": "text", "text": "repository summary"}},
		})
	}))
	defer server.Close()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })

	owner, err := New(hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	capability := testMCPToolCapability("mcp-validation", "repo.summary", mcpmodel.ValidationProofApplicabilityDirect)
	owner.runtimeAdapter = NewRuntimeAdapter(staticMCPServerGetter{server: &mcpmodel.Server{
		ID: "mcp-validation", Name: "validation", Transport: "http", Endpoint: server.URL,
		Enabled: true, AllowedTools: []string{"repo.summary"}, TimeoutMillis: 1000,
	}, capabilities: []*mcpmodel.Capability{capability}})
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })

	var committed runtimecontract.RuntimeOperationReceipt
	receiptOwner := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	receiptOwner.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(orchestratorevents.QueryRuntimeOperationReceiptResult{Found: false}, nil)
	})
	receiptOwner.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.CommitRuntimeOperationReceiptCommand)
		committed = command.Receipt.Clone()
		result.Set(orchestratorevents.CommitRuntimeOperationReceiptResult{Receipt: command.Receipt, StateRevision: 7}, nil)
	})
	t.Cleanup(func() {
		receiptOwner.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
		receiptOwner.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
	})

	operation := runtimecontract.RuntimeOperation{
		ID: "mcp-summary", Kind: runtimecontract.StepKindMCPToolCall,
		Parameters: []runtimecontract.OperationParameter{
			{Name: "serverId", String: stringPointer("mcp-validation")}, {Name: "tool", String: stringPointer("repo.summary")},
			{Name: "input.path", String: stringPointer(".")},
		},
	}
	sealed := sealMCPTestOperation(t, owner.runtimeAdapter, operation)
	command := mcpevents.ExecuteOperationCommand{
		RuntimeOperationExecutionContext: runtimecontract.RuntimeOperationExecutionContext{
			Execution: mcpTestExecutionEnvelope(operation, runtimecontract.RuntimeOperationExecutionEnvelope{
				ExecutionKey: "exec/ws-v2/run-v2/dispatch-1/mcp-summary", Fence: 1,
				ExpectedEffectScope: runtimecontract.RuntimeEffectNone, AdmissibilitySnapshotID: sealed.Authorization.CatalogSnapshotID,
				AcquisitionBinding: sealed.Authorization.AcquisitionBinding,
			}, sealed.Authorization.Policy),
			WorkspaceID: "ws-v2", RunID: "run-v2", DispatchID: "dispatch-1",
			Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-1"},
			OperationID: "mcp-summary", EvidenceKind: runtimecontract.EvidenceKindValidation, WorkspaceDeliverableDirectory: t.TempDir(), WorkspaceInternalDirectory: t.TempDir(),
		},
		SealedOperation: sealed,
	}
	command.Execution.AdmissibilitySnapshotID = sealed.Authorization.CatalogSnapshotID
	command.Execution.AcquisitionBinding = sealed.Authorization.AcquisitionBinding
	result := hub.Send(magicevent.NewEventWithContext(
		mcpevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, ModuleID,
		magicevent.NewHeader(), ctx, command,
	))
	if result == nil {
		t.Fatal("MCP owner returned no execution result")
	}
	data, resultErr := result.Get()
	if resultErr != nil {
		t.Fatalf("MCP owner execution result error=%#v resultError=%#v", resultErr, result.Error())
	}
	response, ok := data.(mcpevents.ExecuteOperationResult)
	if !ok {
		t.Fatalf("MCP owner result type = %T", data)
	}
	recipeID := runtimecontract.RuntimeMCPToolValidationRecipeID("mcp-validation", "repo.summary")
	assertMCPToolValidationObservation(t, response.Receipt, recipeID)
	assertMCPToolValidationObservation(t, committed, recipeID)
}

func TestMCPToolValidationObservationDoesNotCreditOtherMCPShapes(t *testing.T) {
	cases := []struct {
		name         string
		status       runtimecontract.RuntimeOperationExecutionStatus
		operation    string
		evidenceKind string
	}{
		{name: "rejected", status: runtimecontract.RuntimeOperationExecutionStatusRejected, operation: runtimecontract.StepKindMCPToolCall, evidenceKind: runtimecontract.EvidenceKindValidation},
		{name: "non validation", status: runtimecontract.RuntimeOperationExecutionStatusAccepted, operation: runtimecontract.StepKindMCPToolCall, evidenceKind: runtimecontract.EvidenceKindStateChange},
		{name: "resource", status: runtimecontract.RuntimeOperationExecutionStatusAccepted, operation: runtimecontract.StepKindMCPResourceRead, evidenceKind: runtimecontract.EvidenceKindValidation},
		{name: "prompt", status: runtimecontract.RuntimeOperationExecutionStatusAccepted, operation: runtimecontract.StepKindMCPPromptGet, evidenceKind: runtimecontract.EvidenceKindValidation},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			evidence := runtimecontract.RuntimeOperationEvidence{}
			attachMCPToolValidationObservation(&evidence, tc.status, tc.operation, tc.evidenceKind, "mcp-validation", "repo.summary", nil)
			if evidence.Validation != nil || evidence.Quality != "" {
				t.Fatalf("non-proof MCP shape received validation observation: %+v", evidence)
			}
		})
	}
}

func assertMCPToolValidationObservation(t *testing.T, receipt runtimecontract.RuntimeOperationReceipt, recipeID string) {
	t.Helper()
	observation := receipt.Evidence.Validation
	if receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || receipt.Evidence.Quality != common.EvidenceQualityValid || observation == nil ||
		observation.RecipeID != recipeID || !reflect.DeepEqual(observation.ExecutedChecks, []string{recipeID}) ||
		!reflect.DeepEqual(observation.PassedChecks, []string{recipeID}) {
		t.Fatalf("MCP validation receipt = %+v, want exact owner observation", receipt)
	}
}

func stringPointer(value string) *string { return &value }

func testMCPToolCapability(serverID common.MCPServerID, tool string, applicability string) *mcpmodel.Capability {
	return &mcpmodel.Capability{
		ID: common.MCPCapabilityID(string(serverID) + ":tool:" + tool), ServerID: serverID,
		Kind: mcpmodel.CapabilityKindTool, Name: tool, Enabled: true,
		EffectScope: string(runtimecontract.RuntimeEffectNone), IdempotencyClass: string(runtimecontract.RuntimeOperationIdempotencySafeToReplay),
		ValidationProofApplicability: applicability,
		Schema: workorcheventpayload.New(
			workorcheventpayload.StringField("type", "object"),
			workorcheventpayload.ObjectField("properties", workorcheventpayload.New(
				workorcheventpayload.ObjectField("path", workorcheventpayload.New(workorcheventpayload.StringField("type", "string"))),
			)),
			workorcheventpayload.BoolField("additionalProperties", false),
		),
	}
}

func sealMCPTestOperation(t *testing.T, adapter *RuntimeAdapter, operation runtimecontract.RuntimeOperation) mcpevents.SealedRuntimeOperation {
	t.Helper()
	snapshot, err := adapter.runtimeOperationAdmissibilitySnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := buildRuntimeMCPAdmissibility("mcp-test-catalog", []string{operation.Kind}, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := mcpevents.SealRuntimeOperation(operation, catalog)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func mcpTestExecutionEnvelope(operation runtimecontract.RuntimeOperation, envelope runtimecontract.RuntimeOperationExecutionEnvelope, policy runtimecontract.RuntimeOperationPolicyFact) runtimecontract.RuntimeOperationExecutionEnvelope {
	envelope.OperationHash = runtimecontract.StableRuntimeOperationHash(operation)
	envelope.IdempotencyClass = runtimecontract.ResolveOperationIdempotencyClass(operation, []runtimecontract.RuntimeOperationPolicyFact{policy})
	envelope.AuthorityHash = runtimecontract.StableRuntimeOperationExecutionAuthorityHash(runtimecontract.RuntimeOperationExecutionRecord{
		OperationID: operation.ID, OperationHash: envelope.OperationHash, ExecutionKey: envelope.ExecutionKey, ExpectedEffectScope: envelope.ExpectedEffectScope,
		IdempotencyClass: envelope.IdempotencyClass, AdmissibilitySnapshotID: envelope.AdmissibilitySnapshotID,
		AcquisitionBinding: envelope.AcquisitionBinding, ReplayProof: envelope.ReplayProof,
	})
	return envelope
}
