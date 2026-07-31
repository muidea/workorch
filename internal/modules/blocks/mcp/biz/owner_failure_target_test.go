package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
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
	result := RuntimeOperationResult{
		ServerID: "server-a", OwnerRevision: "mcp-server-v1",
		Diagnostics: MCPCapabilityFailureDiagnostics{TargetRole: "implementation", FailureTargets: []string{" implementation.go ", "implementation.go"}},
	}
	observation := mcpFailureTargetObservation(result)
	if observation == nil {
		t.Fatal("mcpFailureTargetObservation() = nil, want owner observation")
	}
	if observation.OwnerModuleID != ModuleID || observation.SubjectRef != "server-a" || observation.TargetRole != "implementation" ||
		observation.ObservedRevision != "mcp-server-v1" || !reflect.DeepEqual(observation.TargetRefs, []string{"implementation.go"}) {
		t.Fatalf("observation = %+v", observation)
	}
	for _, invalid := range []RuntimeOperationResult{
		{ServerID: "server-a", OwnerRevision: "mcp-server-v1", Diagnostics: MCPCapabilityFailureDiagnostics{FailureTargets: []string{"implementation.go"}}},
		{ServerID: "server-a", Diagnostics: MCPCapabilityFailureDiagnostics{TargetRole: "implementation", FailureTargets: []string{"implementation.go"}}},
		{ServerID: "server-a", OwnerRevision: "mcp-server-v1", Diagnostics: MCPCapabilityFailureDiagnostics{TargetRole: "implementation"}},
	} {
		if got := mcpFailureTargetObservation(invalid); got != nil {
			t.Fatalf("mcpFailureTargetObservation(%+v) = %+v, want nil", invalid, got)
		}
	}
}

// The adapter tests cover response parsing.  This owner-level regression
// proves the parsed observation survives the real typed EventHub execution
// path and receipt command: the validation entry remains operation lineage,
// while only the explicit MCP response field becomes a related failure target.
func TestMCPOwnerEmitsStructuredFailureTargetObservationThroughTypedReceipt(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
	owner.runtimeAdapter = NewRuntimeAdapter(staticMCPServerGetter{server: &mcpmodel.Server{
		ID: "mcp-observed-target", Name: "observed target", Transport: "http", Endpoint: server.URL,
		Enabled: true, AllowedTools: []string{"lookup.references"}, TimeoutMillis: 1000,
	}})
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

	command := orchestratorevents.ExecuteMCPOperationCommand{
		Execution: orchestratorevents.RuntimeOperationExecutionEnvelope{
			ExecutionKey: "exec/ws-v2/run-v2/dispatch-1/validate-entry", Fence: 1,
			ExpectedEffectScope: runtimecontract.RuntimeEffectNone,
		},
		WorkspaceID: "ws-v2", RunID: "run-v2", DispatchID: "dispatch-1",
		Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-1"},
		OperationID: "validate-entry", OperationKind: runtimecontract.StepKindMCPToolCall,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"validation-entry.go"},
		EvidenceKind: runtimecontract.EvidenceKindValidation, TargetRole: "implementation",
		WorkspaceDeliverableDirectory: t.TempDir(),
		Data: orchestratorevents.MCPOperationData{
			ServerID: "mcp-observed-target", Tool: "lookup.references",
			Input: []orchestratorevents.MCPArgument{{Name: "path", String: stringPointer("validation-entry.go")}},
		},
	}
	result := hub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorExecuteMCPOperation, "mcp-owner-test", ModuleID,
		magicevent.NewHeader(), ctx, command,
	))
	if result == nil {
		t.Fatal("MCP owner returned no execution result")
	}
	data, resultErr := result.Get()
	if resultErr != nil {
		t.Fatalf("MCP owner execution result error=%#v resultError=%#v", resultErr, result.Error())
	}
	response, ok := data.(orchestratorevents.ExecuteMCPOperationResult)
	if !ok {
		t.Fatalf("MCP owner result type = %T", data)
	}
	failure := response.Receipt.Failure
	if response.Receipt.Outcome != runtimecontract.RuntimeOperationOutcomeRejected || failure == nil ||
		!reflect.DeepEqual(failure.TargetRefs, []string{"implementation.go"}) ||
		failure.TargetObservation == nil || failure.TargetObservation.TargetRole != "implementation" ||
		!reflect.DeepEqual(failure.TargetObservation.TargetRefs, []string{"implementation.go"}) {
		t.Fatalf("MCP owner failure result = %+v", response)
	}
	if !reflect.DeepEqual(response.Receipt.TargetRefs, []string{"validation-entry.go"}) ||
		!reflect.DeepEqual(failure.TargetObservation.TargetRefs, []string{"implementation.go"}) {
		t.Fatalf("MCP owner receipt lineage = %+v", response.Receipt)
	}
	if committed.ExecutionKey != command.Execution.ExecutionKey || committed.Outcome != runtimecontract.RuntimeOperationOutcomeRejected ||
		!reflect.DeepEqual(committed.TargetRefs, []string{"validation-entry.go"}) || committed.Failure == nil ||
		committed.Failure.TargetObservation == nil || !reflect.DeepEqual(committed.Failure.TargetObservation.TargetRefs, []string{"implementation.go"}) {
		t.Fatalf("committed MCP receipt = %+v", committed)
	}
	if response.Receipt.Feedback == nil || response.Receipt.Feedback.Domain != runtimecontract.RuntimeFeedbackDomainOperation ||
		response.Receipt.Feedback.WorkspaceID != command.WorkspaceID || response.Receipt.Feedback.RunID != command.RunID ||
		response.Receipt.Feedback.AuthoritySnapshotID != command.Transaction.AuthoritySnapshotID || response.Receipt.Feedback.DispatchID != command.DispatchID ||
		response.Receipt.Feedback.OriginalDiagnostic == nil || response.Receipt.Feedback.OriginalDiagnostic.OwnerModuleID != runtimecontract.RuntimeModuleMCP ||
		response.Receipt.Feedback.OriginalDiagnostic.OperationID != command.OperationID || response.Receipt.Feedback.OriginalDiagnostic.OriginalCode != failure.Code ||
		response.Receipt.Feedback.OriginalDiagnostic.OriginalMessage != failure.Message {
		t.Fatalf("MCP owner receipt lost scoped original feedback: %+v", response.Receipt.Feedback)
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
	owner.runtimeAdapter = NewRuntimeAdapter(staticMCPServerGetter{server: &mcpmodel.Server{
		ID: "mcp-validation", Name: "validation", Transport: "http", Endpoint: server.URL,
		Enabled: true, AllowedTools: []string{"repo.summary"}, TimeoutMillis: 1000,
	}})
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

	command := orchestratorevents.ExecuteMCPOperationCommand{
		Execution: orchestratorevents.RuntimeOperationExecutionEnvelope{
			ExecutionKey: "exec/ws-v2/run-v2/dispatch-1/mcp-summary", Fence: 1,
			ExpectedEffectScope: runtimecontract.RuntimeEffectNone,
		},
		WorkspaceID: "ws-v2", RunID: "run-v2", DispatchID: "dispatch-1",
		Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-1"},
		OperationID: "mcp-summary", OperationKind: runtimecontract.StepKindMCPToolCall,
		EvidenceKind: runtimecontract.EvidenceKindValidation, WorkspaceDeliverableDirectory: t.TempDir(),
		Data: orchestratorevents.MCPOperationData{
			ServerID: "mcp-validation", Tool: "repo.summary",
			Input: []orchestratorevents.MCPArgument{{Name: "path", String: stringPointer(".")}},
		},
	}
	result := hub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorExecuteMCPOperation, "mcp-owner-test", ModuleID,
		magicevent.NewHeader(), ctx, command,
	))
	if result == nil {
		t.Fatal("MCP owner returned no execution result")
	}
	data, resultErr := result.Get()
	if resultErr != nil {
		t.Fatalf("MCP owner execution result error=%#v resultError=%#v", resultErr, result.Error())
	}
	response, ok := data.(orchestratorevents.ExecuteMCPOperationResult)
	if !ok {
		t.Fatalf("MCP owner result type = %T", data)
	}
	recipeID := runtimecontract.RuntimeMCPToolValidationRecipeID(command.Data.ServerID, command.Data.Tool)
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
			attachMCPToolValidationObservation(&evidence, tc.status, tc.operation, tc.evidenceKind, "mcp-validation", "repo.summary")
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
