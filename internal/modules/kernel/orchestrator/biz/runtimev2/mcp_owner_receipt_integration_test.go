package runtimev2

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	filebiz "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	llmbiz "github.com/muidea/workorch/internal/modules/blocks/llm/biz"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	mcpbiz "github.com/muidea/workorch/internal/modules/blocks/mcp/biz"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func mustRuntimeOperationAuthorizationIdentity(t *testing.T, kind string) runtimecontract.RuntimeCandidateOperationManifest {
	t.Helper()
	authorization, err := orchestratorevents.RuntimeOperationAuthorizationIdentity(kind)
	if err != nil {
		t.Fatalf("runtime operation authorization identity for %q: %v", kind, err)
	}
	return authorization
}

// runtimeV2MCPIntegrationRoundTripper keeps the MCP owner on its real HTTP
// adapter path while avoiding a listener. This makes the owner/receipt/C3-C5
// integration fixture runnable in sandboxed CI where loopback bind is denied.
type runtimeV2MCPIntegrationRoundTripper func(*http.Request) (*http.Response, error)

func (f runtimeV2MCPIntegrationRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

// TestMCPOwnerObservedFailureTargetBindsThroughEventHubAndFileSnapshot
// verifies the H1 -> H3/H5 boundary without a synthetic receipt: the MCP
// owner executes its real HTTP adapter, queries/commits through concrete
// EventHub contracts, the RuntimeController CAS-binds only the explicit
// failure target, and the File owner then returns the corresponding target
// snapshot. The validation entry remains operation lineage and duplicate
// delivery reuses the persisted receipt without another MCP call.
func TestMCPOwnerObservedFailureTargetBindsThroughEventHubAndFileSnapshot(t *testing.T) {
	ctx := context.Background()
	calls := 0
	client := &http.Client{Transport: runtimeV2MCPIntegrationRoundTripper(func(request *http.Request) (*http.Response, error) {
		calls++
		if request.Method != http.MethodPost || request.URL.String() != "http://mcp-observed-target.test/runtime" {
			return nil, fmt.Errorf("unexpected MCP HTTP request %s %s", request.Method, request.URL)
		}
		var payload []byte
		var err error
		if calls > 1 {
			payload, err = json.Marshal(map[string]any{
				"content": []map[string]any{{"type": "text", "text": "validation passed"}},
			})
		} else {
			payload, err = json.Marshal(map[string]any{
				"content":        []map[string]any{{"type": "text", "text": "implementation failed"}},
				"failureTargets": []string{"implementation.go"},
				"isError":        true,
			})
		}
		if err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(payload)), Request: request}, nil
	})}

	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	workspaceDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspaceDirectory, "implementation.go"), []byte("package fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fileOwner, err := filebiz.New(hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := fileOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fileOwner.Teardown(ctx) })
	mcpServer := &mcpmodel.Server{
		ID: "mcp-observed-target", Name: "observed target", Transport: "http", Endpoint: "http://mcp-observed-target.test/runtime",
		Enabled: true, AllowedTools: []string{"lookup.references"}, TimeoutMillis: 1000,
	}
	adapter := mcpbiz.NewRuntimeAdapterWithHTTPClient(runtimeV2MCPServerGetter{server: mcpServer}, client)
	_, catalogRevision, err := adapter.OperationAdmissibility(ctx)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := mcpbiz.NewWithDependencies(mcpbiz.Dependencies{EventHub: hub, RuntimeAdapter: adapter})
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })

	aggregate := validationAggregate(t)
	aggregate.Facts.Capabilities.ValidationKinds = []string{runtimecontract.StepKindMCPToolCall}
	aggregate.Facts.Activation.MCPCapabilities = []runtimecontract.RuntimeMCPCapabilityContext{{
		ID: "mcp-observed-target:tool:lookup.references", ServerID: "mcp-observed-target", Kind: "tool", Name: "lookup.references", Enabled: true,
		EffectScope: string(runtimecontract.RuntimeEffectNone), IdempotencyClass: string(runtimecontract.RuntimeOperationIdempotencySafeToReplay),
	}}
	aggregate.Facts.Focus.TargetRefs = nil
	aggregate.Facts.Focus.TargetRole = "implementation"
	aggregate.Facts.Focus.RequireTargetGrounding = true
	aggregate.Facts.Focus.RequireAssessment = true
	aggregate.Facts.ConvergencePolicy.MaxCandidateContextItems = 4
	aggregate.Facts.Focus.TargetDiscoverySources = []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest}
	addCurrentMCPFailureObservationFreshness(t, &aggregate, catalogRevision)
	decision, err := runtimecontract.SealRuntimeDecision(aggregate.Facts, runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedValidation,
		Focus: aggregate.Facts.Focus,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds:        []string{runtimecontract.StepKindMCPToolCall},
			AcceptedEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone},
			RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone},
			CoverageWorkItemIDs:  aggregate.Facts.Focus.CoverageWorkItemIDs,
		},
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	aggregate.Decision = decision
	aggregate.Definition.CapabilityContract = []string{runtimecontract.StepKindMCPToolCall}
	addObservedFileOperationAdmissibilityFixture(t, ctx, hub, &aggregate, workspaceDirectory)
	decision, err = runtimecontract.SealRuntimeDecision(aggregate.Facts, runtimecontract.RuntimeDecision{
		State: aggregate.Decision.State, Focus: aggregate.Decision.Focus, RequiredOutput: aggregate.Decision.RequiredOutput,
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	aggregate.Decision = decision
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	candidate := runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "mcp-validate", Kind: runtimecontract.StepKindMCPToolCall,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"validation-entry.go"},
		Parameters: []runtimecontract.OperationParameter{
			{Name: "serverId", String: runtimeV2StringPointer("mcp-observed-target")},
			{Name: "tool", String: runtimeV2StringPointer("lookup.references")},
			{Name: "input.path", String: runtimeV2StringPointer("validation-entry.go")},
		},
	}}}}}
	sealCandidateForTest(t, repository, candidate)
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	receiptObserver := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	receiptObserver.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.QueryRuntimeOperationReceiptCommand)
		receipt, found, queryErr := controller.QueryOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.OperationID, command.ExecutionKey, command.Fence, command.GovernanceGrant)
		if queryErr != nil {
			result.Set(nil, runtimeV2MagicFailure(queryErr))
			return
		}
		if !found {
			result.Set(orchestratorevents.QueryRuntimeOperationReceiptResult{Found: false}, nil)
			return
		}
		result.Set(orchestratorevents.QueryRuntimeOperationReceiptResult{Found: true, Receipt: &receipt}, nil)
	})
	receiptObserver.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.CommitRuntimeOperationReceiptCommand)
		next, receipt, commitErr := controller.CommitOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.Fence, command.Receipt)
		if commitErr != nil {
			result.Set(nil, runtimeV2MagicFailure(commitErr))
			return
		}
		result.Set(orchestratorevents.CommitRuntimeOperationReceiptResult{Receipt: receipt, StateRevision: next.StateRevision}, nil)
	})
	t.Cleanup(func() {
		receiptObserver.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
		receiptObserver.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
	})

	if _, err := controller.ExecuteActiveDispatch(ctx, "ws-1", "run-1", runtimeV2ObservedMCPExecutor{hub: hub, deliverableDirectory: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	persisted, err := controller.Load(ctx, "ws-1", "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Facts.Resources.FailureTarget == nil || persisted.Facts.Resources.FailureTarget.TargetObservation == nil ||
		len(persisted.Facts.Resources.FailureTarget.TargetRefs) != 1 || persisted.Facts.Resources.FailureTarget.TargetRefs[0] != "implementation.go" ||
		persisted.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite {
		t.Fatalf("controller did not CAS-bind observed target before prerequisite: %+v", persisted)
	}
	if len(persisted.Facts.Prerequisites.Pending) == 0 || calls != 1 || len(persisted.OperationReceipts) != 1 {
		t.Fatalf("missing target snapshot prerequisite or unexpected MCP calls: pending=%+v calls=%d", persisted.Facts.Prerequisites.Pending, calls)
	}
	var snapshotPrerequisite runtimecontract.RuntimePrerequisite
	for _, item := range persisted.Facts.Prerequisites.Pending {
		if item.Kind == runtimecontract.PrerequisiteKindTargetSnapshot && item.OwnerModuleID == runtimecontract.RuntimeModuleFile && item.SubjectRef == "implementation.go" {
			snapshotPrerequisite = item
			break
		}
	}
	if snapshotPrerequisite.ID == "" {
		t.Fatalf("observed target did not schedule File snapshot: %+v", persisted.Facts.Prerequisites.Pending)
	}
	snapshotResult := hub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorAcquireTargetSnapshot, runtimecontract.RuntimeModuleOrchestrator, orchestratorevents.PrerequisiteCommandDestination,
		magicevent.NewHeader(), ctx, orchestratorevents.AcquireTargetSnapshotCommand{
			WorkspaceID: persisted.Facts.WorkspaceID, RunID: persisted.Facts.RunID, AuthoritySnapshotID: persisted.Decision.AuthoritySnapshot.ID,
			PrerequisiteID: snapshotPrerequisite.ID, WorkspaceDeliverableDirectory: workspaceDirectory, TargetRef: snapshotPrerequisite.SubjectRef, MaxBytes: 16 * 1024,
		},
	))
	if snapshotResult == nil {
		t.Fatal("File owner returned no target snapshot result")
	}
	snapshotData, snapshotErr := snapshotResult.Get()
	if snapshotErr != nil {
		t.Fatal(snapshotErr)
	}
	snapshotResponse, ok := snapshotData.(orchestratorevents.AcquireTargetSnapshotResult)
	if !ok || !snapshotResponse.Snapshot.Complete() || snapshotResponse.Observation == nil {
		t.Fatalf("File target snapshot response = %#v", snapshotData)
	}
	persisted, err = controller.CommitPrerequisite(ctx, persisted.Facts.WorkspaceID, persisted.Facts.RunID, PrerequisiteResult{
		PrerequisiteID: snapshotPrerequisite.ID, TargetSnapshot: &snapshotResponse.Snapshot, Observation: snapshotResponse.Observation,
	})
	if err != nil || len(persisted.Facts.Resources.TargetSnapshots) != 1 || persisted.Facts.Resources.TargetSnapshots[0].TargetRef != "implementation.go" {
		t.Fatalf("File snapshot was not CAS-bound after observed MCP target: aggregate=%+v err=%v", persisted, err)
	}
	persisted = commitObservedWorkspaceManifest(t, ctx, hub, controller, persisted, workspaceDirectory)
	if persisted.Decision.State != runtimecontract.ConvergenceStateNeedStateChange {
		t.Fatalf("grounded aggregate did not reduce to a state-change candidate: action=%s pending=%+v", persisted.Decision.State, persisted.Facts.Prerequisites.Pending)
	}

	// H4: the LLM owner receives only the aggregate facts already CAS-bound by
	// H1--H3. It can return a target-bound proposal, but it cannot manufacture
	// the observation or mutate the authority. The controller remains the only
	// component that seals the resulting candidate.
	provider := &runtimeV2CapturedCandidateProvider{payloadFor: runtimeV2ObservedCandidatePayload}
	llmOwner, err := llmbiz.New(hub, nil, llmbiz.Dependencies{CandidateProvider: provider})
	if err != nil {
		t.Fatal(err)
	}
	if err := llmOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { llmOwner.Teardown(ctx) })
	stablePrefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(persisted.Definition, persisted.Facts.Activation.ExecutionContract)
	persisted, err = controller.BeginCandidateAttemptForTest(ctx, persisted.Facts.WorkspaceID, persisted.Facts.RunID, 0, stablePrefix)
	if err != nil {
		t.Fatal(err)
	}
	candidateContext := runtimecontract.RuntimeCandidateContextForDecision(persisted.Facts, persisted.Decision, ActiveRejectedOperationShapes(persisted))
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(persisted.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(candidateContext.Resources))
	if err != nil {
		t.Fatal(err)
	}
	requestResult := hub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, runtimecontract.RuntimeModuleOrchestrator, runtimecontract.RuntimeModuleLLM,
		magicevent.NewHeader(), ctx, orchestratorevents.RequestRuntimeCandidateCommand{
			WorkspaceID: persisted.Facts.WorkspaceID, RunID: persisted.Facts.RunID,
			LogicalRequestID: "candidate-request-observed", RequestSequence: 1, PhysicalCallID: "candidate-call-observed",
			Decision: persisted.Decision, Transaction: persisted.Decision.AuthorityTransaction(), CandidateAttempt: 0,
			StablePrefixHash: stablePrefix, CandidateManifest: manifest, DefinitionContext: persisted.Definition, CandidateContext: candidateContext,
		},
	))
	if requestResult == nil {
		t.Fatal("LLM owner returned no candidate result")
	}
	requestData, requestErr := requestResult.Get()
	if requestErr != nil {
		t.Fatal(requestErr)
	}
	response, ok := requestData.(orchestratorevents.RequestRuntimeCandidateResult)
	if !ok || response.Failure != nil {
		t.Fatalf("LLM candidate response = %#v", requestData)
	}
	decodedCandidate, err := runtimecontract.DecodeRuntimeCandidatePayloadWithManifest(manifest, response.DocumentContent)
	if err != nil || decodedCandidate.StateChange == nil {
		t.Fatalf("LLM candidate document = %q err=%v", response.DocumentContent, err)
	}
	if len(provider.requests) != 1 {
		t.Fatalf("LLM provider calls = %d, want 1", len(provider.requests))
	}
	assertObservedTargetAuthorityInCandidateRequest(t, provider.requests[0], stablePrefix)
	proposal, err := controller.AdmitCandidateProposalForTest(ctx, persisted.Facts.WorkspaceID, persisted.Facts.RunID, 0, decodedCandidate)
	if err != nil || !proposal.Admission.Accepted() {
		t.Fatalf("target-bound candidate proposal was not admitted: result=%+v err=%v", proposal, err)
	}
	persisted = proposal.Aggregate
	inventory, found := runtimeOperationAdmissibilityForKind(persisted.Facts.Resources.OperationAdmissibilities, runtimecontract.RuntimeModuleFile, runtimecontract.RuntimeBlockKindPatchApply)
	if !found {
		t.Fatalf("candidate proposal lost File owner admissibility: %+v", persisted.Facts.Resources.OperationAdmissibilities)
	}
	observation, found := runtimecontract.RuntimePrerequisiteObservationForID(persisted.Facts.Resources.PrerequisiteObservations, inventory.PrerequisiteID)
	if !found {
		t.Fatalf("candidate proposal lost File owner admissibility observation: %+v", persisted.Facts.Resources.PrerequisiteObservations)
	}
	probeResult := hub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorProbeRuntimeOperationFeasibility, runtimecontract.RuntimeModuleOrchestrator, runtimecontract.RuntimeModuleFile,
		magicevent.NewHeader(), ctx, orchestratorevents.ProbeRuntimeOperationFeasibilityCommand{
			WorkspaceID: persisted.Facts.WorkspaceID, RunID: persisted.Facts.RunID, Transaction: persisted.Decision.AuthorityTransaction(),
			WorkspaceDeliverableDirectory: workspaceDirectory, Operation: decodedCandidate.StateChange.Operations[0].Operation,
			Admissibility: inventory, CapabilityStateBinding: inventory.AcquisitionBinding, Observation: &observation,
			CandidateManifestHash: "manifest-mcp-file-integration", OperationAuthorization: mustRuntimeOperationAuthorizationIdentity(t, decodedCandidate.StateChange.Operations[0].Operation.Kind),
		},
	))
	if probeResult == nil {
		t.Fatal("File owner returned no candidate feasibility result")
	}
	probeData, probeErr := probeResult.Get()
	if probeErr != nil {
		t.Fatal(probeErr)
	}
	probeResponse, ok := probeData.(orchestratorevents.ProbeRuntimeOperationFeasibilityResult)
	if !ok || !probeResponse.Feasible || probeResponse.Stale != nil || probeResponse.Failure != nil {
		t.Fatalf("File candidate feasibility response = %#v", probeData)
	}
	persisted, err = controller.RecordCandidateFeasibility(ctx, persisted.Facts.WorkspaceID, persisted.Facts.RunID, 0, runtimecontract.RuntimeCandidateFeasibilityRecord{
		OperationID: decodedCandidate.StateChange.Operations[0].Operation.ID, AdmissibilitySnapshotID: inventory.SnapshotID,
		AcquisitionBinding: inventory.AcquisitionBinding, Feasible: true, ReplayProof: probeResponse.ReplayProof,
	})
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := controller.AdmitAndSealCandidateAttemptForTest(ctx, persisted.Facts.WorkspaceID, persisted.Facts.RunID, 0, decodedCandidate)
	if err != nil || !sealed.Admission.Accepted() || sealed.Aggregate.ActiveDispatch == nil {
		t.Fatalf("target-bound candidate was not sealed: result=%+v err=%v", sealed, err)
	}
	if targets := sealed.Aggregate.ActiveDispatch.Candidate.StateChange.Operations[0].Operation.TargetRefs; len(targets) != 1 || targets[0] != "implementation.go" {
		t.Fatalf("sealed candidate target refs = %v", targets)
	}
	sealedOperation := sealed.Aggregate.ActiveDispatch.Candidate.StateChange.Operations[0].Operation
	sealedBaseHash := ""
	for _, parameter := range sealedOperation.Parameters {
		if parameter.Name == "baseHash" && parameter.String != nil {
			sealedBaseHash = *parameter.String
			break
		}
	}
	if len(sealed.Aggregate.Facts.Resources.TargetSnapshots) != 1 || sealedBaseHash == "" || sealedBaseHash != sealed.Aggregate.Facts.Resources.TargetSnapshots[0].ContentHash {
		t.Fatalf("sealed file_write did not bind the owner snapshot revision: operation=%+v snapshots=%+v", sealedOperation, sealed.Aggregate.Facts.Resources.TargetSnapshots)
	}
	sealedExecution, found := operationExecutionRecord(sealed.Aggregate.ActiveDispatch.OperationExecutions, sealedOperation.ID)
	if !found || sealedExecution.ReplayProof == nil || sealedExecution.ReplayProof.BeforeHash != sealedBaseHash || sealedExecution.ReplayProof.OperationID != sealedOperation.ID {
		t.Fatalf("sealed file operation omitted owner replay proof: %+v", sealedExecution)
	}
	persisted = sealed.Aggregate
	persisted, err = controller.ExecuteActiveDispatch(ctx, persisted.Facts.WorkspaceID, persisted.Facts.RunID, runtimeV2ObservedFilePatchExecutor{hub: hub, deliverableDirectory: workspaceDirectory})
	if err != nil {
		t.Fatal(err)
	}
	if persisted.ActiveDispatch != nil || persisted.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite ||
		persisted.Facts.Repair.LastProgressRevision != "" || persisted.Facts.Resources.FailureTarget == nil {
		t.Fatalf("durable target receipt bypassed freshness/assessment progress gate: action=%s repair=%+v aggregate=%+v", persisted.Decision.State, persisted.Facts.Repair, persisted)
	}
	if !hasObservedTargetSnapshotPrerequisite(persisted.Facts.Prerequisites.Pending, "implementation.go") {
		t.Fatalf("durable target receipt did not invalidate and re-acquire its grounded snapshot: pending=%+v", persisted.Facts.Prerequisites.Pending)
	}
	controller, err = NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	persisted, err = controller.Load(ctx, persisted.Facts.WorkspaceID, persisted.Facts.RunID)
	if err != nil || persisted.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite || persisted.Facts.Resources.FailureTarget == nil {
		t.Fatalf("restart after committed File receipt did not reuse authority: aggregate=%+v err=%v", persisted, err)
	}
	persisted = reacquireObservedFilePrerequisites(t, ctx, hub, controller, persisted, workspaceDirectory)
	if persisted.Decision.State != runtimecontract.ConvergenceStateNeedValidation || len(persisted.Facts.Prerequisites.Pending) != 0 ||
		persisted.Facts.Repair.LastProgressRevision != "" {
		t.Fatalf("fresh owner facts did not return to validation without synthetic progress: action=%s pending=%+v repair=%+v", persisted.Decision.State, persisted.Facts.Prerequisites.Pending, persisted.Facts.Repair)
	}
	persisted = requestAndSealObservedCandidate(t, ctx, hub, controller, persisted, workspaceDirectory)
	if persisted.Decision.State != runtimecontract.ConvergenceStateNeedValidation || persisted.ActiveDispatch == nil || persisted.ActiveDispatch.Candidate.Validation == nil {
		t.Fatalf("reacquired aggregate did not seal a validation candidate: %+v", persisted)
	}
	persisted, err = controller.ExecuteActiveDispatch(ctx, persisted.Facts.WorkspaceID, persisted.Facts.RunID, runtimeV2ObservedMCPExecutor{hub: hub, deliverableDirectory: workspaceDirectory})
	if err != nil || calls != 2 || persisted.Decision.State != runtimecontract.ConvergenceStateNeedAssessment || persisted.Facts.Repair.LastProgressRevision != "" {
		t.Fatalf("accepted validation did not enter assessment without clearing progress: calls=%d action=%s repair=%+v err=%v", calls, persisted.Decision.State, persisted.Facts.Repair, err)
	}
	persisted = requestAndSealObservedCandidate(t, ctx, hub, controller, persisted, workspaceDirectory)
	if persisted.ActiveDispatch == nil || persisted.ActiveDispatch.Candidate.Assessment == nil {
		t.Fatalf("assessment candidate was not sealed: %+v", persisted)
	}
	persisted, err = controller.CommitAssessment(ctx, persisted.Facts.WorkspaceID, persisted.Facts.RunID)
	if err != nil || persisted.Decision.State != runtimecontract.ConvergenceStateComplete || persisted.Facts.Repair.LastProgressRevision == "" {
		t.Fatalf("assessment did not atomically close current focus progress: action=%s repair=%+v err=%v", persisted.Decision.State, persisted.Facts.Repair, err)
	}
	controller, err = NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	loadedTerminal, err := controller.Load(ctx, persisted.Facts.WorkspaceID, persisted.Facts.RunID)
	if err != nil || loadedTerminal.Decision.State != runtimecontract.ConvergenceStateComplete || loadedTerminal.StateRevision != persisted.StateRevision ||
		loadedTerminal.Facts.Repair.LastProgressRevision != persisted.Facts.Repair.LastProgressRevision {
		t.Fatalf("restart after committed assessment did not reuse terminal facts: aggregate=%+v err=%v", loadedTerminal, err)
	}
	content, readErr := os.ReadFile(filepath.Join(workspaceDirectory, "implementation.go"))
	if readErr != nil || string(content) != "package fixture\n// repaired\n" {
		t.Fatalf("File owner did not apply sealed target-bound patch: content=%q err=%v", content, readErr)
	}
	dispatch := persisted.CompletedDispatches[0]
	execution := dispatch.OperationExecutions[0]
	replayed := sendObservedMCPExecution(t, ctx, hub, orchestratorevents.ExecuteMCPOperationCommand{
		Execution:   orchestratorevents.RuntimeOperationExecutionEnvelope{ExecutionKey: execution.ExecutionKey, Fence: execution.Fence, ExpectedEffectScope: execution.ExpectedEffectScope},
		WorkspaceID: persisted.Facts.WorkspaceID, RunID: persisted.Facts.RunID, DispatchID: dispatch.ID, Transaction: dispatch.Transaction,
		OperationID: "mcp-validate", OperationKind: runtimecontract.StepKindMCPToolCall, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		TargetRefs: []string{"validation-entry.go"}, EvidenceKind: runtimecontract.EvidenceKindValidation, TargetRole: "implementation", WorkspaceDeliverableDirectory: t.TempDir(),
		Data: orchestratorevents.MCPOperationData{ServerID: "mcp-observed-target", Tool: "lookup.references", Input: []orchestratorevents.MCPArgument{{Name: "path", String: runtimeV2StringPointer("validation-entry.go")}}},
	})
	if calls != 2 || replayed.Receipt.ExecutionKey != execution.ExecutionKey || replayed.Receipt.Failure == nil ||
		replayed.Receipt.Failure.TargetObservation == nil {
		t.Fatalf("duplicate MCP receipt replayed side effect or lost observation: calls=%d replay=%+v", calls, replayed)
	}
}

type runtimeV2MCPServerGetter struct{ server *mcpmodel.Server }

func (g runtimeV2MCPServerGetter) GetServer(_ context.Context, id common.MCPServerID) (*mcpmodel.Server, error) {
	if g.server == nil || g.server.ID != id {
		return nil, common.ErrNotFound
	}
	return g.server, nil
}

func (g runtimeV2MCPServerGetter) ListServers(context.Context) ([]*mcpmodel.Server, error) {
	if g.server == nil {
		return nil, common.ErrNotFound
	}
	return []*mcpmodel.Server{g.server}, nil
}

func runtimeV2StringPointer(value string) *string { return &value }

type runtimeV2ObservedMCPExecutor struct {
	hub                  magicevent.Hub
	deliverableDirectory string
}

func (e runtimeV2ObservedMCPExecutor) ExecuteRuntimeOperation(ctx context.Context, request RuntimeOperationExecutionRequest) (RuntimeOperationExecutionResult, error) {
	data, err := orchestratorevents.MCPOperationDataFromRuntimeOperation(request.Operation)
	if err != nil {
		return RuntimeOperationExecutionResult{}, err
	}
	result := e.hub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorExecuteMCPOperation, runtimecontract.RuntimeModuleOrchestrator, runtimecontract.RuntimeModuleMCP,
		magicevent.NewHeader(), ctx, orchestratorevents.ExecuteMCPOperationCommand{
			Execution:   orchestratorevents.RuntimeOperationExecutionEnvelope{ExecutionKey: request.Execution.ExecutionKey, Fence: request.Execution.Fence, ExpectedEffectScope: request.Execution.ExpectedEffectScope, GovernanceGrant: request.GovernanceGrant},
			WorkspaceID: request.WorkspaceID, RunID: request.RunID, DispatchID: request.DispatchID, Transaction: request.Transaction,
			OperationID: request.Operation.ID, OperationKind: request.Operation.Kind, CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), request.Operation.CoverageWorkItemIDs...),
			TargetRefs: append([]string(nil), request.Operation.TargetRefs...), EvidenceKind: request.EvidenceKind, TargetRole: request.TargetRole,
			EvidencePolicies: append([]runtimecontract.RuntimeEvidencePolicy(nil), request.EvidencePolicies...), WorkspaceDeliverableDirectory: e.deliverableDirectory, Data: data,
		}))
	if result == nil {
		return RuntimeOperationExecutionResult{}, fmt.Errorf("MCP owner returned no execution result")
	}
	responseData, resultErr := result.Get()
	if resultErr != nil {
		return RuntimeOperationExecutionResult{}, resultErr
	}
	response, ok := responseData.(orchestratorevents.ExecuteMCPOperationResult)
	if !ok {
		return RuntimeOperationExecutionResult{}, fmt.Errorf("MCP owner result type %T", responseData)
	}
	status, evidence, failure, governance := orchestratorevents.RuntimeOperationResultFromReceipt(response.Receipt)
	return RuntimeOperationExecutionResult{EventID: response.EventID, OperationID: response.OperationID, EvidenceKind: response.EvidenceKind, Status: status, SourceRef: response.SourceRef, Failure: failure, Evidence: evidence, Receipt: response.Receipt, Governance: governance}, nil
}

// runtimeV2ObservedFilePatchExecutor keeps the H5 state-change receipt on the
// same concrete owner route as production: controller claim fence -> File
// owner receipt query/commit -> controller evidence CAS. It deliberately does
// not inspect coverage or choose the next action.
type runtimeV2ObservedFilePatchExecutor struct {
	hub                  magicevent.Hub
	deliverableDirectory string
}

func (e runtimeV2ObservedFilePatchExecutor) ExecuteRuntimeOperation(ctx context.Context, request RuntimeOperationExecutionRequest) (RuntimeOperationExecutionResult, error) {
	data, err := orchestratorevents.PatchApplyOperationDataFromRuntimeOperation(request.Operation)
	if err != nil {
		return RuntimeOperationExecutionResult{}, err
	}
	result := e.hub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorExecutePatchApplyOperation, runtimecontract.RuntimeModuleOrchestrator, runtimecontract.RuntimeModuleFile,
		magicevent.NewHeader(), ctx, orchestratorevents.ExecutePatchApplyOperationCommand{
			Execution:                     orchestratorevents.RuntimeOperationExecutionEnvelope{ExecutionKey: request.Execution.ExecutionKey, Fence: request.Execution.Fence, ExpectedEffectScope: request.Execution.ExpectedEffectScope, GovernanceGrant: request.GovernanceGrant},
			WorkspaceID:                   request.WorkspaceID,
			RunID:                         request.RunID,
			DispatchID:                    request.DispatchID,
			Transaction:                   request.Transaction,
			OperationID:                   request.Operation.ID,
			CoverageWorkItemIDs:           append([]common.CoverageWorkItemID(nil), request.Operation.CoverageWorkItemIDs...),
			TargetRefs:                    append([]string(nil), request.Operation.TargetRefs...),
			EvidenceKind:                  request.EvidenceKind,
			EvidencePolicies:              append([]runtimecontract.RuntimeEvidencePolicy(nil), request.EvidencePolicies...),
			WorkspaceDeliverableDirectory: e.deliverableDirectory,
			Data:                          data,
		},
	))
	if result == nil {
		return RuntimeOperationExecutionResult{}, fmt.Errorf("File owner returned no patch execution result")
	}
	responseData, resultErr := result.Get()
	if resultErr != nil {
		return RuntimeOperationExecutionResult{}, resultErr
	}
	response, ok := responseData.(orchestratorevents.ExecutePatchApplyOperationResult)
	if !ok {
		return RuntimeOperationExecutionResult{}, fmt.Errorf("File owner result type %T", responseData)
	}
	status, evidence, failure, governance := orchestratorevents.RuntimeOperationResultFromReceipt(response.Receipt)
	return RuntimeOperationExecutionResult{EventID: response.EventID, OperationID: response.OperationID, EvidenceKind: response.EvidenceKind, Status: status, SourceRef: response.SourceRef, Failure: failure, Evidence: evidence, Receipt: response.Receipt, Governance: governance}, nil
}

func sendObservedMCPExecution(t *testing.T, ctx context.Context, hub magicevent.Hub, command orchestratorevents.ExecuteMCPOperationCommand) orchestratorevents.ExecuteMCPOperationResult {
	t.Helper()
	result := hub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorExecuteMCPOperation, runtimecontract.RuntimeModuleOrchestrator, runtimecontract.RuntimeModuleMCP,
		magicevent.NewHeader(), ctx, command,
	))
	if result == nil {
		t.Fatal("MCP owner returned no execution result")
	}
	data, resultErr := result.Get()
	if resultErr != nil {
		t.Fatal(resultErr)
	}
	response, ok := data.(orchestratorevents.ExecuteMCPOperationResult)
	if !ok {
		t.Fatalf("MCP owner result type = %T", data)
	}
	return response
}

func commitObservedWorkspaceManifest(
	t *testing.T,
	ctx context.Context,
	hub magicevent.Hub,
	controller *RuntimeController,
	aggregate RuntimeAggregate,
	workspaceDirectory string,
) RuntimeAggregate {
	t.Helper()
	var prerequisite runtimecontract.RuntimePrerequisite
	for _, pending := range aggregate.Facts.Prerequisites.Pending {
		if pending.Kind == runtimecontract.PrerequisiteKindManifest && pending.OwnerModuleID == runtimecontract.RuntimeModuleFile {
			prerequisite = pending
			break
		}
	}
	if prerequisite.ID == "" {
		t.Fatalf("grounded aggregate did not request File manifest: pending=%+v", aggregate.Facts.Prerequisites.Pending)
	}
	result := hub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorAcquireWorkspaceManifest, runtimecontract.RuntimeModuleOrchestrator, orchestratorevents.PrerequisiteCommandDestination,
		magicevent.NewHeader(), ctx, orchestratorevents.AcquireWorkspaceManifestCommand{
			WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, AuthoritySnapshotID: aggregate.Decision.AuthoritySnapshot.ID,
			PrerequisiteID: prerequisite.ID, WorkspaceDeliverableDirectory: workspaceDirectory,
		},
	))
	if result == nil {
		t.Fatal("File owner returned no workspace manifest result")
	}
	data, resultErr := result.Get()
	if resultErr != nil {
		t.Fatal(resultErr)
	}
	response, ok := data.(orchestratorevents.AcquireWorkspaceManifestResult)
	if !ok || response.Manifest.Revision == "" || response.Observation == nil {
		t.Fatalf("File manifest response = %#v", data)
	}
	next, err := controller.CommitPrerequisite(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, PrerequisiteResult{
		PrerequisiteID: prerequisite.ID, Manifest: &response.Manifest, Observation: response.Observation,
	})
	if err != nil {
		t.Fatal(err)
	}
	return next
}

// addObservedFileOperationAdmissibilityFixture obtains the executable-owner
// inventory through its real typed EventHub boundary before the fixture enters
// the controller. It models already-completed activation preflight; later H4
// admission still performs its own persisted feasibility probe before sealing.
func addObservedFileOperationAdmissibilityFixture(
	t *testing.T,
	ctx context.Context,
	hub magicevent.Hub,
	aggregate *RuntimeAggregate,
	workspaceDirectory string,
) {
	t.Helper()
	if aggregate == nil || aggregate.Decision.AuthoritySnapshot.ID == "" {
		t.Fatal("aggregate must have a sealed initial authority before File preflight")
	}
	prerequisiteID := "operation_admissibility:file-h4"
	result := hub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorAcquireRuntimeOperationAdmissibility, runtimecontract.RuntimeModuleOrchestrator, runtimecontract.RuntimeModuleFile,
		magicevent.NewHeader(), ctx, orchestratorevents.AcquireRuntimeOperationAdmissibilityCommand{
			WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, AuthoritySnapshotID: aggregate.Decision.AuthoritySnapshot.ID,
			PrerequisiteID: prerequisiteID, OwnerModuleID: runtimecontract.RuntimeModuleFile, WorkspaceDeliverableDirectory: workspaceDirectory,
			RequiredKinds: []string{runtimecontract.RuntimeBlockKindPatchApply}, RequiredTargetRefs: []string{"implementation.go"}, MaxItems: 4,
		},
	))
	if result == nil {
		t.Fatal("File owner returned no operation admissibility result")
	}
	data, resultErr := result.Get()
	if resultErr != nil {
		t.Fatal(resultErr)
	}
	response, ok := data.(orchestratorevents.AcquireRuntimeOperationAdmissibilityResult)
	if !ok || response.Admissibility.SnapshotID == "" || response.Observation == nil {
		t.Fatalf("File operation admissibility response = %#v", data)
	}
	aggregate.Facts.Resources.OperationAdmissibilities = append(aggregate.Facts.Resources.OperationAdmissibilities, response.Admissibility)
	aggregate.Facts.Resources.PrerequisiteObservations = append(aggregate.Facts.Resources.PrerequisiteObservations, *response.Observation)
	aggregate.Facts.Prerequisites.Satisfied = append(aggregate.Facts.Prerequisites.Satisfied, runtimecontract.RuntimePrerequisite{
		ID: prerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: runtimecontract.RuntimeModuleFile, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindPatchApply},
	})
}

func reacquireObservedFilePrerequisites(t *testing.T, ctx context.Context, hub magicevent.Hub, controller *RuntimeController, aggregate RuntimeAggregate, workspaceDirectory string) RuntimeAggregate {
	t.Helper()
	for step := 0; step < 8 && aggregate.Decision.State == runtimecontract.ConvergenceStateNeedPrerequisite; step++ {
		if len(aggregate.Facts.Prerequisites.Pending) == 0 {
			t.Fatal("acquire prerequisite authority has no pending requirement")
		}
		prerequisite := aggregate.Facts.Prerequisites.Pending[0]
		var result PrerequisiteResult
		switch prerequisite.Kind {
		case runtimecontract.PrerequisiteKindManifest:
			response := sendObservedManifest(t, ctx, hub, aggregate, prerequisite, workspaceDirectory)
			result = PrerequisiteResult{PrerequisiteID: prerequisite.ID, Manifest: &response.Manifest, Observation: response.Observation}
		case runtimecontract.PrerequisiteKindTargetSnapshot:
			response := sendObservedTargetSnapshot(t, ctx, hub, aggregate, prerequisite, workspaceDirectory)
			result = PrerequisiteResult{PrerequisiteID: prerequisite.ID, TargetSnapshot: &response.Snapshot, Observation: response.Observation}
		case runtimecontract.PrerequisiteKindOperationAdmissibility:
			response := sendObservedFileAdmissibility(t, ctx, hub, aggregate, prerequisite, workspaceDirectory)
			result = PrerequisiteResult{PrerequisiteID: prerequisite.ID, OperationAdmissibility: &response.Admissibility, Observation: response.Observation}
		default:
			t.Fatalf("unexpected pending prerequisite during File refresh: %+v", prerequisite)
		}
		var err error
		aggregate, err = controller.CommitPrerequisite(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, result)
		if err != nil {
			t.Fatal(err)
		}
	}
	if aggregate.Decision.State == runtimecontract.ConvergenceStateNeedPrerequisite {
		t.Fatalf("File prerequisite refresh did not converge: pending=%+v", aggregate.Facts.Prerequisites.Pending)
	}
	return aggregate
}

func sendObservedManifest(t *testing.T, ctx context.Context, hub magicevent.Hub, aggregate RuntimeAggregate, prerequisite runtimecontract.RuntimePrerequisite, directory string) orchestratorevents.AcquireWorkspaceManifestResult {
	t.Helper()
	result := hub.Send(magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorAcquireWorkspaceManifest, runtimecontract.RuntimeModuleOrchestrator, orchestratorevents.PrerequisiteCommandDestination, magicevent.NewHeader(), ctx, orchestratorevents.AcquireWorkspaceManifestCommand{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, AuthoritySnapshotID: aggregate.Decision.AuthoritySnapshot.ID, PrerequisiteID: prerequisite.ID, WorkspaceDeliverableDirectory: directory}))
	data, err := result.Get()
	if err != nil {
		t.Fatal(err)
	}
	response, ok := data.(orchestratorevents.AcquireWorkspaceManifestResult)
	if !ok || response.Observation == nil {
		t.Fatalf("manifest refresh response = %#v", data)
	}
	return response
}

func sendObservedTargetSnapshot(t *testing.T, ctx context.Context, hub magicevent.Hub, aggregate RuntimeAggregate, prerequisite runtimecontract.RuntimePrerequisite, directory string) orchestratorevents.AcquireTargetSnapshotResult {
	t.Helper()
	result := hub.Send(magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorAcquireTargetSnapshot, runtimecontract.RuntimeModuleOrchestrator, orchestratorevents.PrerequisiteCommandDestination, magicevent.NewHeader(), ctx, orchestratorevents.AcquireTargetSnapshotCommand{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, AuthoritySnapshotID: aggregate.Decision.AuthoritySnapshot.ID, PrerequisiteID: prerequisite.ID, WorkspaceDeliverableDirectory: directory, TargetRef: prerequisite.SubjectRef, MaxBytes: 16 * 1024}))
	data, err := result.Get()
	if err != nil {
		t.Fatal(err)
	}
	response, ok := data.(orchestratorevents.AcquireTargetSnapshotResult)
	if !ok || response.Observation == nil || !response.Snapshot.Complete() {
		t.Fatalf("snapshot refresh response = %#v", data)
	}
	return response
}

func sendObservedFileAdmissibility(t *testing.T, ctx context.Context, hub magicevent.Hub, aggregate RuntimeAggregate, prerequisite runtimecontract.RuntimePrerequisite, directory string) orchestratorevents.AcquireRuntimeOperationAdmissibilityResult {
	t.Helper()
	result := hub.Send(magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorAcquireRuntimeOperationAdmissibility, runtimecontract.RuntimeModuleOrchestrator, runtimecontract.RuntimeModuleFile, magicevent.NewHeader(), ctx, orchestratorevents.AcquireRuntimeOperationAdmissibilityCommand{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, AuthoritySnapshotID: aggregate.Decision.AuthoritySnapshot.ID, PrerequisiteID: prerequisite.ID, OwnerModuleID: prerequisite.OwnerModuleID, WorkspaceDeliverableDirectory: directory, RequiredKinds: prerequisite.RequiredCapabilities, RequiredTargetRefs: GroundedRepairTargets(aggregate.Facts), MaxItems: 4}))
	data, err := result.Get()
	if err != nil {
		t.Fatal(err)
	}
	response, ok := data.(orchestratorevents.AcquireRuntimeOperationAdmissibilityResult)
	if !ok || response.Observation == nil || len(response.Admissibility.TargetRefs) != 1 || response.Admissibility.TargetRefs[0] != "implementation.go" {
		t.Fatalf("admissibility refresh response = %#v", data)
	}
	return response
}

type runtimeV2CapturedCandidateProvider struct {
	requests   []llmmodels.RuntimeCandidateRequest
	payload    []byte
	payloadFor func(llmmodels.RuntimeCandidateRequest) []byte
}

func (p *runtimeV2CapturedCandidateProvider) GenerateCandidate(_ context.Context, request llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error) {
	p.requests = append(p.requests, request)
	payload := p.payload
	if p.payloadFor != nil {
		payload = p.payloadFor(request)
	}
	return llmmodels.RuntimeCandidateGenerationResult{
		Payload: append([]byte(nil), payload...), Status: llmmodels.RuntimeCandidateStatusCompleted,
	}, nil
}

func runtimeV2ObservedCandidatePayload(request llmmodels.RuntimeCandidateRequest) []byte {
	switch request.Decision.State {
	case runtimecontract.ConvergenceStateNeedStateChange:
		baseHash := ""
		for _, snapshot := range request.CandidateContext.Resources.TargetSnapshots {
			if snapshot.TargetRef == "implementation.go" {
				baseHash = snapshot.ContentHash
				break
			}
		}
		return []byte(fmt.Sprintf(`{"operations":[{"id":"repair-implementation","kind":"patch/apply","coverageWorkItemIds":["cwi-1"],"targetRefs":["implementation.go"],"parameters":[{"name":"patchType","string":"file_write"},{"name":"content","string":"package fixture\n// repaired\n"},{"name":"baseHash","string":%q}]}]}`, baseHash))
	case runtimecontract.ConvergenceStateNeedValidation:
		// After an owner-observed FailureTarget is sealed, N1/N2 compile
		// exact_target membership from that grounded set. Revalidation must
		// target the sealed failure/repair target, not a second free path.
		target := "validation-entry.go"
		if refs := request.Decision.RequiredOutput.Normalized().TargetRefs; len(refs) == 1 {
			target = refs[0]
		} else if request.CandidateContext.Resources.FailureTarget != nil && len(request.CandidateContext.Resources.FailureTarget.TargetRefs) == 1 {
			target = request.CandidateContext.Resources.FailureTarget.TargetRefs[0]
		}
		return []byte(fmt.Sprintf(`{"operations":[{"id":"revalidate-implementation","kind":"mcp/tool_call","coverageWorkItemIds":["cwi-1"],"targetRefs":[%q],"parameters":[{"name":"serverId","string":"mcp-observed-target"},{"name":"tool","string":"lookup.references"},{"name":"input.path","string":%q}]}]}`, target, target))
	case runtimecontract.ConvergenceStateNeedAssessment:
		ref := ""
		if accepted := request.CandidateContext.Evidence.Accepted; len(accepted) > 0 {
			ref = string(accepted[len(accepted)-1].ID)
		}
		return []byte(fmt.Sprintf(`{"focusId":"focus-1","coverageWorkItemIds":["cwi-1"],"disposition":"satisfied","items":[{"coverageWorkItemId":"cwi-1","satisfied":true,"evidenceRefs":[%q]}],"summary":"current focus satisfied","evidenceRefs":[%q]}`, ref, ref))
	default:
		return nil
	}
}

func requestAndSealObservedCandidate(t *testing.T, ctx context.Context, hub magicevent.Hub, controller *RuntimeController, aggregate RuntimeAggregate, directory string) RuntimeAggregate {
	t.Helper()
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(aggregate.Definition, aggregate.Facts.Activation.ExecutionContract)
	var err error
	aggregate, err = controller.BeginCandidateAttemptForTest(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, prefix)
	if err != nil {
		t.Fatal(err)
	}
	candidateContext := runtimecontract.RuntimeCandidateContextForDecision(aggregate.Facts, aggregate.Decision, ActiveRejectedOperationShapes(aggregate))
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(aggregate.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(candidateContext.Resources))
	if err != nil {
		t.Fatal(err)
	}
	result := hub.Send(magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, runtimecontract.RuntimeModuleOrchestrator, runtimecontract.RuntimeModuleLLM, magicevent.NewHeader(), ctx, orchestratorevents.RequestRuntimeCandidateCommand{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, LogicalRequestID: "candidate-request-observed-seal", RequestSequence: 1, PhysicalCallID: "candidate-call-observed-seal", Decision: aggregate.Decision, Transaction: aggregate.Decision.AuthorityTransaction(), StablePrefixHash: prefix, CandidateManifest: manifest, DefinitionContext: aggregate.Definition, CandidateContext: candidateContext}))
	data, requestErr := result.Get()
	if requestErr != nil {
		t.Fatal(requestErr)
	}
	response, ok := data.(orchestratorevents.RequestRuntimeCandidateResult)
	if !ok || response.Failure != nil {
		t.Fatalf("candidate response = %#v", data)
	}
	candidate, err := runtimecontract.DecodeRuntimeCandidatePayloadWithManifest(manifest, response.DocumentContent)
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := controller.AdmitCandidateProposalForTest(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, candidate)
	if err != nil || !proposal.Admission.Accepted() {
		t.Fatalf("candidate proposal = %+v err=%v", proposal, err)
	}
	aggregate = proposal.Aggregate
	for _, operation := range candidateRuntimeOperations(candidate) {
		descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
		if !found {
			t.Fatalf("candidate operation descriptor missing: %+v", operation)
		}
		inventory, found := runtimeOperationAdmissibilityForKind(aggregate.Facts.Resources.OperationAdmissibilities, descriptor.OwnerModuleID, operation.Kind)
		if !found {
			t.Fatalf("candidate owner inventory missing: %s", operation.Kind)
		}
		observation, found := runtimecontract.RuntimePrerequisiteObservationForID(aggregate.Facts.Resources.PrerequisiteObservations, inventory.PrerequisiteID)
		if !found {
			t.Fatalf("candidate owner observation missing: %+v", inventory)
		}
		probe := hub.Send(magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorProbeRuntimeOperationFeasibility, runtimecontract.RuntimeModuleOrchestrator, descriptor.OwnerModuleID, magicevent.NewHeader(), ctx, orchestratorevents.ProbeRuntimeOperationFeasibilityCommand{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, Transaction: aggregate.Decision.AuthorityTransaction(), WorkspaceDeliverableDirectory: directory, Operation: operation, Admissibility: inventory, CapabilityStateBinding: inventory.AcquisitionBinding, Observation: &observation, CandidateManifestHash: "manifest-mcp-integration", OperationAuthorization: mustRuntimeOperationAuthorizationIdentity(t, operation.Kind)}))
		probeData, probeErr := probe.Get()
		if probeErr != nil {
			t.Fatal(probeErr)
		}
		probeResult, ok := probeData.(orchestratorevents.ProbeRuntimeOperationFeasibilityResult)
		if !ok || !probeResult.Feasible || probeResult.Failure != nil || probeResult.Stale != nil {
			t.Fatalf("candidate feasibility = %#v", probeData)
		}
		aggregate, err = controller.RecordCandidateFeasibility(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, runtimecontract.RuntimeCandidateFeasibilityRecord{OperationID: operation.ID, AdmissibilitySnapshotID: inventory.SnapshotID, AcquisitionBinding: inventory.AcquisitionBinding, Feasible: true, ReplayProof: probeResult.ReplayProof})
		if err != nil {
			t.Fatal(err)
		}
	}
	sealed, err := controller.AdmitAndSealCandidateAttemptForTest(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, candidate)
	if err != nil || !sealed.Admission.Accepted() {
		t.Fatalf("candidate seal = %+v err=%v", sealed, err)
	}
	return sealed.Aggregate
}

func assertObservedTargetAuthorityInCandidateRequest(t *testing.T, request llmmodels.RuntimeCandidateRequest, stablePrefix string) {
	t.Helper()
	if request.StablePrefixHash != stablePrefix || request.CandidateContext.RequiredState != runtimecontract.ConvergenceStateNeedStateChange {
		t.Fatalf("LLM request changed sealed authority: prefix=%q action=%s", request.StablePrefixHash, request.CandidateContext.RequiredState)
	}
	resources := request.CandidateContext.Resources
	if resources.FailureTarget == nil || resources.FailureTarget.TargetObservation == nil ||
		resources.FailureTarget.TargetObservation.OwnerModuleID != runtimecontract.RuntimeModuleMCP ||
		resources.FailureTarget.TargetObservation.SubjectRef != "mcp-observed-target" ||
		resources.FailureTarget.TargetObservation.TargetRole != "implementation" ||
		resources.FailureTarget.TargetObservation.ObservedRevision == "" ||
		len(resources.FailureTarget.TargetRefs) != 1 || resources.FailureTarget.TargetRefs[0] != "implementation.go" {
		t.Fatalf("LLM request lost committed failure target authority: %+v", resources.FailureTarget)
	}
	if len(resources.TargetSnapshots) != 1 || resources.TargetSnapshots[0].TargetRef != "implementation.go" {
		t.Fatalf("LLM request target snapshots = %+v, want only owner-observed target", resources.TargetSnapshots)
	}
	if len(request.CandidateContext.Focus.TargetRefs) != 0 {
		t.Fatalf("LLM request rewrote Focus target refs from failure target: %+v", request.CandidateContext.Focus)
	}
}

func hasObservedTargetSnapshotPrerequisite(values []runtimecontract.RuntimePrerequisite, targetRef string) bool {
	for _, value := range values {
		if value.Kind == runtimecontract.PrerequisiteKindTargetSnapshot && value.OwnerModuleID == runtimecontract.RuntimeModuleFile && value.SubjectRef == targetRef {
			return true
		}
	}
	return false
}

func runtimeV2MagicFailure(err error) *cd.Error {
	if err == nil {
		return nil
	}
	return &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err}
}
