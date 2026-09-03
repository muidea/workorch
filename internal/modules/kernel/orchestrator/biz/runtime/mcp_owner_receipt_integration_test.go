package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	filebiz "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	llmbiz "github.com/muidea/workorch/internal/modules/blocks/llm/biz"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	mcpbiz "github.com/muidea/workorch/internal/modules/blocks/mcp/biz"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/events"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/llmaudit"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	workorcheventpayload "github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func mustRuntimeOperationAuthorizationIdentity(t *testing.T, kind string) runtimecontract.RuntimeCandidateOperationManifest {
	t.Helper()
	descriptor, ok := runtimecontract.RuntimeOperationDescriptorForKind(kind)
	if !ok {
		t.Fatalf("runtime operation authorization identity has unknown kind %q", kind)
	}
	return runtimecontract.RuntimeCandidateOperationManifest{
		Kind: descriptor.Kind, OwnerModuleID: descriptor.OwnerModuleID,
		OperationContractRevision: runtimecontract.RuntimeOperationContractRevision(),
		Target: runtimecontract.RuntimeOperationTargetAuthorization{
			Policy:            runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope,
			OwnerSelectorRefs: []string{"test/owner-feasibility"},
		},
	}.Normalized()
}

func sendRuntimeOperationFeasibilityProbeForTest(ctx context.Context, hub magicevent.Hub, request runtimecontract.RuntimeOperationFeasibilityRequest) (runtimecontract.RuntimeOperationFeasibilityObservation, error) {
	var result magicevent.Result
	owner := request.Admissibility.OwnerModuleID
	switch owner {
	case runtimecontract.RuntimeModuleFile:
		command := fileevents.ProbeRuntimeOperationFeasibilityCommand{WorkspaceID: request.WorkspaceID, RunID: request.RunID, Transaction: request.Transaction, WorkspaceDeliverableDirectory: request.WorkspaceDeliverableDirectory, Operation: request.Operation, Admissibility: request.Admissibility, CapabilityStateBinding: request.CapabilityStateBinding, CandidateManifestHash: request.CandidateManifestHash, OperationAuthorization: request.OperationAuthorization, Observation: request.Observation, CurrentTargetSnapshot: request.CurrentTargetSnapshot}
		result = hub.Send(magicevent.NewEventWithContext(fileevents.TopicProbeRuntimeOperationFeasibility, runtimecontract.RuntimeModuleOrchestrator, owner, magicevent.NewHeader(), ctx, command))
	case runtimecontract.RuntimeModuleMCP:
		command := mcpevents.ProbeRuntimeOperationFeasibilityCommand{WorkspaceID: request.WorkspaceID, RunID: request.RunID, Transaction: request.Transaction, WorkspaceDeliverableDirectory: request.WorkspaceDeliverableDirectory, Operation: request.Operation, Admissibility: request.Admissibility, CapabilityStateBinding: request.CapabilityStateBinding, CandidateManifestHash: request.CandidateManifestHash, OperationAuthorization: request.OperationAuthorization, Observation: request.Observation, CurrentTargetSnapshot: request.CurrentTargetSnapshot}
		result = hub.Send(magicevent.NewEventWithContext(mcpevents.TopicProbeRuntimeOperationFeasibility, runtimecontract.RuntimeModuleOrchestrator, owner, magicevent.NewHeader(), ctx, command))
	default:
		return runtimecontract.RuntimeOperationFeasibilityObservation{}, fmt.Errorf("unsupported test feasibility owner %q", owner)
	}
	data, err := result.Get()
	if err != nil {
		return runtimecontract.RuntimeOperationFeasibilityObservation{}, err
	}
	switch value := data.(type) {
	case fileevents.ProbeRuntimeOperationFeasibilityResult:
		return runtimecontract.RuntimeOperationFeasibilityObservation{OperationID: value.OperationID, AdmissibilitySnapshotID: value.AdmissibilitySnapshotID, CapabilityStateBinding: value.CapabilityStateBinding, CandidateManifestHash: value.CandidateManifestHash, OwnerPolicy: value.OwnerPolicy, ConstraintImpact: value.ConstraintImpact, Outcome: value.Outcome, CandidateConstraintViolation: value.CandidateConstraintViolation, Feedback: value.Feedback, ReplayProof: value.ReplayProof, Stale: value.Stale, PrerequisiteRequest: value.PrerequisiteRequest}, nil
	case mcpevents.ProbeRuntimeOperationFeasibilityResult:
		return runtimecontract.RuntimeOperationFeasibilityObservation{OperationID: value.OperationID, AdmissibilitySnapshotID: value.AdmissibilitySnapshotID, CapabilityStateBinding: value.CapabilityStateBinding, CandidateManifestHash: value.CandidateManifestHash, OwnerPolicy: value.OwnerPolicy, ConstraintImpact: value.ConstraintImpact, Outcome: value.Outcome, CandidateConstraintViolation: value.CandidateConstraintViolation, Feedback: value.Feedback, ReplayProof: value.ReplayProof, Stale: value.Stale, PrerequisiteRequest: value.PrerequisiteRequest}, nil
	default:
		return runtimecontract.RuntimeOperationFeasibilityObservation{}, fmt.Errorf("invalid test feasibility result %T", data)
	}
}

// runtimeMCPIntegrationRoundTripper keeps the MCP owner on its real HTTP
// adapter path while avoiding a listener. This makes the owner/receipt/C3-C5
// integration fixture runnable in sandboxed CI where loopback bind is denied.
type runtimeMCPIntegrationRoundTripper func(*http.Request) (*http.Response, error)

func (f runtimeMCPIntegrationRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
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
	targetSnapshotForRef := func(values []runtimecontract.TargetSnapshot, targetRef string) (runtimecontract.TargetSnapshot, bool) {
		for _, value := range values {
			if value.TargetRef == targetRef {
				return value, true
			}
		}
		return runtimecontract.TargetSnapshot{}, false
	}
	calls := 0
	client := &http.Client{Transport: runtimeMCPIntegrationRoundTripper(func(request *http.Request) (*http.Response, error) {
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
	initialImplementation := []byte("package fixture\n")
	if err := os.WriteFile(filepath.Join(workspaceDirectory, "implementation.go"), initialImplementation, 0o644); err != nil {
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
	preSealSnapshotPrerequisite := runtimecontract.RuntimePrerequisite{
		ID: "test-mcp-preseal-implementation-snapshot", Kind: runtimecontract.PrerequisiteKindTargetSnapshot,
		Scope: runtimecontract.RuntimePrerequisiteScopeFocus, FocusID: "focus-1",
		OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "implementation.go",
	}
	preSealSnapshotResult := hub.Send(magicevent.NewEventWithContext(
		fileevents.TopicAcquireTargetSnapshot, runtimecontract.RuntimeModuleOrchestrator, orchestratorevents.PrerequisiteCommandDestination,
		magicevent.NewHeader(), ctx, fileevents.AcquireTargetSnapshotCommand{
			WorkspaceID: "ws-1", RunID: "run-1", AuthoritySnapshotID: "test-mcp-preseal-authority",
			PrerequisiteID: preSealSnapshotPrerequisite.ID, WorkspaceDeliverableDirectory: workspaceDirectory,
			TargetRef: preSealSnapshotPrerequisite.SubjectRef, MaxBytes: 16 * 1024,
		},
	))
	preSealSnapshotData, preSealSnapshotErr := preSealSnapshotResult.Get()
	if preSealSnapshotErr != nil {
		t.Fatal(preSealSnapshotErr)
	}
	preSealSnapshot, ok := preSealSnapshotData.(fileevents.AcquireTargetSnapshotResult)
	if !ok || !preSealSnapshot.Snapshot.Complete() || preSealSnapshot.Observation.ObservationID == "" {
		t.Fatalf("pre-seal File target snapshot response = %#v", preSealSnapshotData)
	}
	mcpServer := &mcpmodel.Server{
		ID: "mcp-observed-target", Name: "observed target", Transport: "http", Endpoint: "http://mcp-observed-target.test/runtime",
		Enabled: true, AllowedTools: []string{"lookup.references"}, TimeoutMillis: 1000,
	}
	adapter := mcpbiz.NewRuntimeAdapterWithHTTPClient(runtimeMCPServerGetter{server: mcpServer}, client)
	_, _, catalogRevision, err := adapter.OperationAdmissibility(ctx)
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

	aggregate := validationAggregateWithPreparation(t, func(initial *RuntimeAggregate) {
		// The semantic contract must authorize the same workspace target that the
		// later owner diagnostic can nominate for repair. Keeping the default
		// main.go binding would make implementation.go diagnostic-only by design.
		initial.Facts.Focus.TargetRefs = []string{"implementation.go"}
		initial.Facts.Resources.TargetSnapshots = []runtimecontract.TargetSnapshot{preSealSnapshot.Snapshot}
		initial.Facts.Resources.PrerequisiteObservations = append(initial.Facts.Resources.PrerequisiteObservations, preSealSnapshot.Observation)
		initial.Facts.Prerequisites.Satisfied = append(initial.Facts.Prerequisites.Satisfied, preSealSnapshotPrerequisite)
		initial.Definition.CapabilityContract = []string{
			runtimecontract.RuntimeBlockKindPatchApply,
			runtimecontract.RuntimeBlockKindManagedValidationSession,
			runtimecontract.StepKindMCPToolCall,
		}
		initial.Facts.Activation.MCPCapabilities = []runtimecontract.RuntimeMCPCapabilityContext{{
			ID: "mcp-observed-target:tool:lookup.references", ServerID: "mcp-observed-target", Kind: "tool", OperationKind: runtimecontract.StepKindMCPToolCall, Name: "lookup.references", Enabled: true,
			EffectScope: string(runtimecontract.RuntimeEffectNone), IdempotencyClass: string(runtimecontract.RuntimeOperationIdempotencySafeToReplay),
		}}
		initial.Facts.ConvergencePolicy.MaxCandidateContextItems = 4
	})
	aggregate.Facts.Resources.ProviderCeiling = &runtimecontract.ProviderCeiling{
		Selection:                aggregate.Facts.Activation.ProviderSelection,
		ContextWindowTokens:      128000,
		ProviderMaxOutputTokens:  16384,
		EffectiveMaxOutputTokens: 8192,
		MaxInputTokens:           119808,
		MaxBytes:                 128 * 1024,
		CapabilityRevision:       "capability-1",
		ConfigurationRevision:    "config-1",
	}
	// This fixture exercises the non-crediting failure-target path. Its
	// declaration deliberately reserves validation proof for managed Shell, so
	// the later MCP owner fact may ground implementation.go without a generic
	// MCP success acquiring CWI credit. The separate C12 MCP binding regression
	// covers the explicit server/tool proof path.
	mcpCatalog := addObservedMCPOperationAdmissibilityFixture(t, ctx, hub, &aggregate, catalogRevision)
	aggregate.Facts.Capabilities.ValidationKinds = append(aggregate.Facts.Capabilities.ValidationKinds, runtimecontract.StepKindMCPToolCall)
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
		TargetRefs: []string{"validation-entry.go"},
		Parameters: []runtimecontract.OperationParameter{
			{Name: "serverId", String: runtimeStringPointer("mcp-observed-target")},
			{Name: "tool", String: runtimeStringPointer("lookup.references")},
			{Name: "input.path", String: runtimeStringPointer("validation-entry.go")},
		},
	}}}}}
	validationManifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(
		aggregate.Decision,
		runtimecontract.RuntimeCandidateManifestOwnerFactsFromContext(
			runtimecontract.RuntimeCandidateContextForDecision(aggregate.Facts, aggregate.Decision, nil),
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err = runtimecontract.DeriveRuntimeCandidateOperationAuthority(validationManifest, candidate)
	if err != nil {
		t.Fatal(err)
	}
	canonicalOperation := candidate.Validation.Operations[0].Operation
	sealCandidateWithFeasibilitiesForTest(t, repository, candidate, []runtimecontract.RuntimeCandidateFeasibilityRecord{{
		OperationID: canonicalOperation.ID, AdmissibilitySnapshotID: mcpCatalog.SnapshotID,
		AcquisitionBinding: mcpCatalog.AcquisitionBinding,
		Observation:        &runtimecontract.RuntimeOwnerFeasibilityObservation{Outcome: runtimecontract.RuntimeOwnerFeasibilityFeasible},
	}})
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	assertMCPReceiptEvidenceDriftRejectedAtCommit(t, repository.aggregate, canonicalOperation)
	var ownerCommittedReceiptAggregate RuntimeAggregate
	receiptObserver := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	receiptObserver.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.QueryRuntimeOperationReceiptCommand)
		receipt, found, queryErr := controller.QueryOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.OperationID, command.ExecutionKey, command.Fence, event.Source(), command.GovernanceGrant)
		if queryErr != nil {
			result.Set(nil, runtimeMagicFailure(queryErr))
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
		next, receipt, commitErr := controller.commitOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.Fence, event.Source(), command.Receipt)
		if commitErr != nil {
			result.Set(nil, runtimeMagicFailure(commitErr))
			return
		}
		ownerCommittedReceiptAggregate = cloneRuntimeAggregateForMCPProvenanceTest(t, next)
		result.Set(orchestratorevents.CommitRuntimeOperationReceiptResult{Receipt: receipt, StateRevision: next.StateRevision}, nil)
	})
	t.Cleanup(func() {
		receiptObserver.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
		receiptObserver.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
	})

	if _, err := controller.executeActiveDispatch(ctx, "ws-1", "run-1", runtimeObservedMCPExecutor{hub: hub, deliverableDirectory: t.TempDir(), internalDirectory: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	persisted, err := controller.Load(ctx, "ws-1", "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Facts.Resources.FailureTarget == nil || persisted.Facts.Resources.FailureTarget.TargetObservation == nil ||
		len(persisted.Facts.Resources.FailureTarget.TargetRefs) != 1 || persisted.Facts.Resources.FailureTarget.TargetRefs[0] != "implementation.go" ||
		persisted.Decision.State != runtimecontract.ConvergenceStateNeedStateChange {
		t.Fatalf("controller did not CAS-bind the pre-authorized observed target: %+v", persisted)
	}
	if len(persisted.Facts.Prerequisites.Pending) != 0 || calls != 1 ||
		len(persisted.OperationReceipts) != len(ownerCommittedReceiptAggregate.OperationReceipts) {
		t.Fatalf("pre-sealed target authority scheduled unexpected prerequisites or MCP calls: pending=%+v calls=%d", persisted.Facts.Prerequisites.Pending, calls)
	}
	assertMCPFailureTargetReverseProvenance(t, controller, repository, ownerCommittedReceiptAggregate, persisted)
	if snapshot, found := targetSnapshotForRef(persisted.Facts.Resources.TargetSnapshots, "implementation.go"); !found || !snapshot.Complete() || snapshot.ContentHash != preSealSnapshot.Snapshot.ContentHash {
		t.Fatalf("pre-seal File target snapshot was not retained after MCP observation: %+v", persisted.Facts.Resources.TargetSnapshots)
	}

	// H4: the LLM owner receives only the aggregate facts already CAS-bound by
	// H1--H3. The MCP observation remains the failure-target authority; the File
	// snapshot only grounds that observed target in current workspace state. LLM
	// can return a target-bound proposal, but it cannot manufacture or mutate
	// that authority. The controller remains the only component that seals it.
	provider := &runtimeCapturedCandidateProvider{payloadFor: runtimeObservedCandidatePayload}
	llmConfig := workorchconfig.DefaultLLMConfig()
	llmConfig.AIProxy.BaseURL = "http://127.0.0.1:8090"
	llmConfig.DefaultChat = workorchconfig.ProviderModelTargetConfig{ProviderID: workorchconfig.BuiltinAIProxyProviderID, ModelID: "test-model"}
	llmOwner, err := llmbiz.New(hub, nil, llmbiz.Dependencies{
		CandidateProvider: provider,
		LoadLLMConfig: func() (workorchconfig.LLMConfig, error) {
			return llmConfig, nil
		},
	})
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
	requestIdentity := runtimecontract.RuntimeCandidateRequestIdentity{LogicalRequestID: "candidate-request-observed", Sequence: 1, PhysicalCallID: "candidate-call-observed"}
	expectedDocument := runtimecontract.RuntimeExpectedDocumentContract{
		SchemaVersion: runtimecontract.RuntimeExpectedDocumentSchemaCurrent,
		DocumentKind:  runtimecontract.RuntimeProviderDocumentOperations, ManifestHash: manifest.Hash,
		LogicalRequestID: requestIdentity.LogicalRequestID, StablePrefixHash: stablePrefix,
		MaxOutputTokens: candidateContext.Resources.ProviderCeiling.EffectiveMaxOutputTokens, MaxRecoveryAttempts: 1,
	}
	wire, err := runtimecontract.BuildRuntimeCandidateWireContract(manifest)
	if err != nil {
		t.Fatal(err)
	}
	expectedDocument.WireContractHash = wire.Hash
	expectedDocument.Hash = runtimecontract.StableRuntimeExpectedDocumentHash(expectedDocument)
	requestResult := hub.Send(magicevent.NewEventWithContext(
		llmevents.TopicRequestRuntimeCandidate, runtimecontract.RuntimeModuleOrchestrator, runtimecontract.RuntimeModuleLLM,
		magicevent.NewHeader(), ctx, llmevents.RequestRuntimeCandidateCommand{
			DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
			WorkspaceID:  persisted.Facts.WorkspaceID, RunID: persisted.Facts.RunID,
			RequestIdentity: requestIdentity, ExpectedDocument: &expectedDocument,
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
	response, ok := requestData.(llmevents.RequestRuntimeCandidateResult)
	if !ok || response.Feedback != nil {
		t.Fatalf("LLM candidate response = %#v", requestData)
	}
	if response.DocumentOutcome == nil {
		t.Fatal("LLM candidate response did not carry a document outcome")
	}
	decodedCandidate, err := runtimecontract.DecodeRuntimeCandidatePayloadWithManifest(manifest, response.DocumentOutcome.Content)
	if err != nil || decodedCandidate.StateChange == nil {
		t.Fatalf("LLM candidate document = %q err=%v", response.DocumentOutcome.Content, err)
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
		fileevents.TopicProbeRuntimeOperationFeasibility, runtimecontract.RuntimeModuleOrchestrator, runtimecontract.RuntimeModuleFile,
		magicevent.NewHeader(), ctx, fileevents.ProbeRuntimeOperationFeasibilityCommand{
			WorkspaceID: persisted.Facts.WorkspaceID, RunID: persisted.Facts.RunID, Transaction: persisted.Decision.AuthorityTransaction(),
			WorkspaceDeliverableDirectory: workspaceDirectory, Operation: decodedCandidate.StateChange.Operations[0].Operation,
			Admissibility: inventory, CapabilityStateBinding: inventory.AcquisitionBinding, Observation: &observation,
			CandidateManifestHash: manifest.Hash, OperationAuthorization: mustRuntimeOperationAuthorizationIdentity(t, decodedCandidate.StateChange.Operations[0].Operation.Kind),
		},
	))
	if probeResult == nil {
		t.Fatal("File owner returned no candidate feasibility result")
	}
	probeData, probeErr := probeResult.Get()
	if probeErr != nil {
		t.Fatal(probeErr)
	}
	probeResponse, ok := probeData.(fileevents.ProbeRuntimeOperationFeasibilityResult)
	if !ok || probeResponse.Outcome != runtimecontract.RuntimeOwnerFeasibilityFeasible || probeResponse.Stale != nil || probeResponse.Feedback != nil {
		t.Fatalf("File candidate feasibility response = %#v", probeData)
	}
	persisted, err = controller.recordCandidateFeasibility(ctx, persisted.Facts.WorkspaceID, persisted.Facts.RunID, 0, runtimecontract.RuntimeCandidateFeasibilityRecord{
		OperationID: decodedCandidate.StateChange.Operations[0].Operation.ID, AdmissibilitySnapshotID: inventory.SnapshotID,
		AcquisitionBinding: inventory.AcquisitionBinding, CandidateManifestHash: probeResponse.CandidateManifestHash,
		Observation: &runtimecontract.RuntimeOwnerFeasibilityObservation{Outcome: runtimecontract.RuntimeOwnerFeasibilityFeasible}, ReplayProof: probeResponse.ReplayProof,
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
	sealedSnapshot, found := targetSnapshotForRef(sealed.Aggregate.Facts.Resources.TargetSnapshots, "implementation.go")
	if !found || sealedBaseHash == "" || sealedBaseHash != sealedSnapshot.ContentHash {
		t.Fatalf("sealed file_write did not bind the owner snapshot revision: operation=%+v snapshots=%+v", sealedOperation, sealed.Aggregate.Facts.Resources.TargetSnapshots)
	}
	sealedExecution, found := operationExecutionRecord(sealed.Aggregate.ActiveDispatch.OperationExecutions, sealedOperation.ID)
	if !found || sealedExecution.ReplayProof == nil || sealedExecution.ReplayProof.BeforeHash != sealedBaseHash || sealedExecution.ReplayProof.OperationID != sealedOperation.ID {
		t.Fatalf("sealed file operation omitted owner replay proof: %+v", sealedExecution)
	}
	persisted = sealed.Aggregate
	persisted, err = controller.executeActiveDispatch(ctx, persisted.Facts.WorkspaceID, persisted.Facts.RunID, runtimeObservedFilePatchExecutor{hub: hub, deliverableDirectory: workspaceDirectory})
	if err != nil {
		t.Fatal(err)
	}
	if persisted.ActiveDispatch != nil || persisted.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite ||
		persisted.Facts.Repair.LastProgressRevision != "" || persisted.Facts.Resources.FailureTarget == nil {
		t.Fatalf("durable target receipt bypassed freshness/assessment progress gate: action=%s repair=%+v aggregate=%+v", persisted.Decision.State, persisted.Facts.Repair, persisted)
	}
	if _, current := runtimecontract.RuntimePrerequisiteObservationForID(persisted.Facts.Resources.PrerequisiteObservations, preSealSnapshotPrerequisite.ID); current {
		t.Fatalf("durable target receipt retained current authority for the stale grounded snapshot: %+v", persisted.Facts.Resources.PrerequisiteObservations)
	}
	if _, retired := runtimecontract.RuntimePrerequisiteObservationForID(persisted.Facts.Resources.RetiredPrerequisiteObservations, preSealSnapshotPrerequisite.ID); !retired {
		t.Fatalf("durable target receipt did not retire the grounded snapshot observation: %+v", persisted.Facts.Resources.RetiredPrerequisiteObservations)
	}
	hasFileRefresh := false
	for _, prerequisite := range persisted.Facts.Prerequisites.Pending {
		if prerequisite.OwnerModuleID == runtimecontract.RuntimeModuleFile &&
			(prerequisite.Kind == runtimecontract.PrerequisiteKindOperationAdmissibility || prerequisite.Kind == runtimecontract.PrerequisiteKindTargetSnapshot) {
			hasFileRefresh = true
			break
		}
	}
	if !hasFileRefresh {
		t.Fatalf("durable target receipt did not schedule File authority refresh: pending=%+v", persisted.Facts.Prerequisites.Pending)
	}
	controller, err = NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	persisted, err = controller.Load(ctx, persisted.Facts.WorkspaceID, persisted.Facts.RunID)
	if err != nil || persisted.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite || persisted.Facts.Resources.FailureTarget == nil {
		t.Fatalf("restart after committed repair-owner receipt did not reuse authority: aggregate=%+v err=%v", persisted, err)
	}
	persisted = reacquireObservedFilePrerequisites(t, ctx, hub, controller, persisted, workspaceDirectory)
	if persisted.Decision.State == runtimecontract.ConvergenceStateNeedPrerequisite || len(persisted.Facts.Prerequisites.Pending) != 0 ||
		persisted.Facts.Repair.LastProgressRevision != "" {
		t.Fatalf("fresh File owner facts did not close prerequisite acquisition: action=%s pending=%+v repair=%+v", persisted.Decision.State, persisted.Facts.Prerequisites.Pending, persisted.Facts.Repair)
	}
	// The old snapshot body remains an immutable replay input, but its owner
	// observation was retired above; it is therefore not current target authority.
	// The refreshed File catalog/observation below is the current owner fact and
	// the next state-change acquisition must obtain a new target snapshot before
	// another patch can be sealed.
	if snapshot, found := targetSnapshotForRef(persisted.Facts.Resources.TargetSnapshots, "implementation.go"); !found || !snapshot.Complete() || snapshot.ContentHash != preSealSnapshot.Snapshot.ContentHash {
		t.Fatalf("File refresh rewrote the immutable repair replay snapshot: before=%+v current=%+v", preSealSnapshot.Snapshot, persisted.Facts.Resources.TargetSnapshots)
	}
	fileCatalog, found := runtimeOperationAdmissibilityForKind(
		persisted.Facts.Resources.OperationAdmissibilities,
		runtimecontract.RuntimeModuleFile,
		runtimecontract.RuntimeBlockKindPatchApply,
	)
	if !found || len(fileCatalog.TargetRefs) != 1 || fileCatalog.TargetRefs[0] != "implementation.go" {
		t.Fatalf("File refresh did not retain exact repaired-target catalog authority: %+v", persisted.Facts.Resources.OperationAdmissibilities)
	}
	if _, found := runtimecontract.RuntimePrerequisiteObservationForID(
		persisted.Facts.Resources.PrerequisiteObservations,
		fileCatalog.PrerequisiteID,
	); !found {
		t.Fatalf("File refresh catalog has no current owner observation: catalog=%+v observations=%+v", fileCatalog, persisted.Facts.Resources.PrerequisiteObservations)
	}
	// Prerequisite acquisition may expose a transient reducer decision while the
	// invalidated semantic generation is being replaced. The next owner-private
	// SM-2 seal must replay the Definition/owner batch. The generic MCP diagnostic
	// is not validation credit, and one durable File effect does not authorize a
	// synthetic validation transition, so the canonical next action remains an
	// exact target-bound state change.
	persisted, err = controller.EnsureSemanticContractSealedForOperationTest(
		ctx,
		persisted.Facts.WorkspaceID,
		persisted.Facts.RunID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Decision.State != runtimecontract.ConvergenceStateNeedStateChange || persisted.ActiveDispatch != nil ||
		persisted.Facts.Resources.FailureTarget == nil || persisted.Facts.Resources.FailureTarget.TargetObservation == nil ||
		len(persisted.Facts.Resources.FailureTarget.TargetRefs) != 1 || persisted.Facts.Resources.FailureTarget.TargetRefs[0] != "implementation.go" ||
		persisted.Facts.Repair.LastProgressRevision == "" || calls != 1 {
		t.Fatalf("semantic reseal changed MCP target authority or invented validation credit: calls=%d action=%s failureTarget=%+v repair=%+v", calls, persisted.Decision.State, persisted.Facts.Resources.FailureTarget, persisted.Facts.Repair)
	}
	var repairProgress *runtimecontract.EvidenceFact
	for index := range persisted.Facts.Evidence.Accepted {
		fact := &persisted.Facts.Evidence.Accepted[index]
		if fact.Revision == persisted.Facts.Repair.LastProgressRevision {
			repairProgress = fact
			break
		}
	}
	if repairProgress == nil || repairProgress.Kind != runtimecontract.EvidenceKindStateChange ||
		repairProgress.OperationID != sealedOperation.ID || len(repairProgress.TargetRefs) != 1 || repairProgress.TargetRefs[0] != "implementation.go" {
		t.Fatalf("semantic reseal progress is not the exact File owner receipt: revision=%q evidence=%+v", persisted.Facts.Repair.LastProgressRevision, repairProgress)
	}
	controller, err = NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	loadedAfterValidation, err := controller.Load(ctx, persisted.Facts.WorkspaceID, persisted.Facts.RunID)
	if err != nil || loadedAfterValidation.Decision.State != runtimecontract.ConvergenceStateNeedStateChange || loadedAfterValidation.StateRevision != persisted.StateRevision ||
		loadedAfterValidation.Facts.Repair.LastProgressRevision != persisted.Facts.Repair.LastProgressRevision {
		t.Fatalf("restart after semantic reseal did not preserve target-bound state-change authority: aggregate=%+v err=%v", loadedAfterValidation, err)
	}
	persisted = loadedAfterValidation
	content, readErr := os.ReadFile(filepath.Join(workspaceDirectory, "implementation.go"))
	if readErr != nil || string(content) != "package fixture\n// repaired\n" {
		t.Fatalf("File owner did not apply sealed target-bound patch: content=%q err=%v", content, readErr)
	}
	var dispatch SealedDispatch
	var replayOperation runtimecontract.RuntimeOperation
	for _, completed := range persisted.CompletedDispatches {
		for _, candidateOperation := range candidateRuntimeOperations(completed.Candidate) {
			if candidateOperation.ID == canonicalOperation.ID {
				dispatch = completed
				replayOperation = candidateOperation
				break
			}
		}
		if replayOperation.ID != "" {
			break
		}
	}
	execution, executionFound := operationExecutionRecord(dispatch.OperationExecutions, replayOperation.ID)
	replayCatalog := runtimeOwnerCatalogForOperation(
		dispatch.OperationAdmissibilities,
		dispatch.OperationAdmissibilityObservations,
		replayOperation,
		execution.AdmissibilitySnapshotID,
		execution.AcquisitionBinding,
	)
	if replayOperation.ID == "" || !executionFound || replayCatalog == nil {
		t.Fatal("persisted MCP operation or owner catalog is missing")
	}
	replaySealed, err := mcpevents.SealRuntimeOperation(replayOperation, *replayCatalog)
	if err != nil {
		t.Fatal(err)
	}
	replayed := sendObservedMCPExecution(t, ctx, hub, mcpevents.ExecuteOperationCommand{
		RuntimeOperationExecutionContext: runtimecontract.RuntimeOperationExecutionContext{
			Execution:   runtimeOperationExecutionEnvelopeForTest(execution, nil),
			WorkspaceID: persisted.Facts.WorkspaceID, RunID: persisted.Facts.RunID, DispatchID: dispatch.ID, Transaction: dispatch.Transaction,
			OperationID: replayOperation.ID, CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), replayOperation.CoverageWorkItemIDs...),
			TargetRefs: append([]string(nil), replayOperation.TargetRefs...), EvidenceKind: runtimecontract.EvidenceKindValidation, WorkspaceDeliverableDirectory: t.TempDir(), WorkspaceInternalDirectory: t.TempDir(),
		},
		SealedOperation: replaySealed,
		TargetRole:      "implementation",
	})
	if calls != 1 || replayed.Receipt.ExecutionKey != execution.ExecutionKey || replayed.Receipt.Feedback == nil ||
		replayed.Receipt.Feedback.OriginalDiagnostic == nil || replayed.Receipt.Feedback.OriginalDiagnostic.TargetObservation == nil {
		t.Fatalf("duplicate MCP receipt replayed side effect or lost observation: calls=%d replay=%+v", calls, replayed)
	}
}

func assertMCPFailureTargetReverseProvenance(
	t *testing.T,
	controller *RuntimeController,
	repository *durableRuntimeStateRepository,
	ownerCommitted RuntimeAggregate,
	persisted RuntimeAggregate,
) {
	t.Helper()
	if ownerCommitted.ActiveDispatch == nil {
		t.Fatal("MCP reverse-provenance fixture did not capture owner_committed state")
	}
	var receipt runtimecontract.RuntimeOperationReceipt
	for _, candidate := range ownerCommitted.OperationReceipts {
		if candidate.Feedback == nil || candidate.Feedback.DispatchID != ownerCommitted.ActiveDispatch.ID {
			continue
		}
		receipt = candidate.Normalized()
		break
	}
	if receipt.OperationID == "" {
		t.Fatalf("captured MCP owner-committed receipt is missing: receipts=%+v", ownerCommitted.OperationReceipts)
	}
	operationID := receipt.OperationID
	if receipt.Feedback == nil || receipt.Feedback.OriginalDiagnostic == nil || receipt.Feedback.OriginalDiagnostic.TargetObservation == nil {
		t.Fatalf("captured MCP receipt has no direct target observation: %+v", receipt)
	}
	targetObservation := receipt.Feedback.OriginalDiagnostic.TargetObservation.Normalized()
	if targetObservation.DiagnosticClass != runtimecontract.RuntimeFailureTargetDiagnosticReceiptEvidence ||
		targetObservation.ContentHash != receipt.Evidence.ContentHash ||
		!sameRuntimeTargetRefs(targetObservation.EvidenceRefs, receipt.Evidence.EvidenceRefs) {
		t.Fatalf("captured MCP target observation is not bound to response evidence: observation=%+v evidence=%+v", targetObservation, receipt.Evidence)
	}
	if execution, found := operationExecutionRecord(ownerCommitted.ActiveDispatch.OperationExecutions, operationID); !found || execution.State != runtimecontract.OperationExecutionOwnerCommitted {
		t.Fatalf("captured MCP receipt state = %+v, want owner_committed", execution)
	}
	if err := cloneRuntimeAggregateForMCPProvenanceTest(t, persisted).Validate(); err != nil {
		t.Fatalf("valid MCP receipt-derived target authority rejected after serialization: %v", err)
	}

	t.Run("generic_commit_ingress_cannot_copy_MCP_authority", func(t *testing.T) {
		beforeRevision := repository.aggregate.StateRevision
		_, err := controller.recordFailureFact(
			context.Background(),
			persisted.Facts.WorkspaceID,
			persisted.Facts.RunID,
			persisted.Facts.Failure.Clone(),
		)
		if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "generic failure ingress cannot carry capability-owner authority") {
			t.Fatalf("generic MCP authority ingress error = %v", err)
		}
		if repository.aggregate.StateRevision != beforeRevision {
			t.Fatalf("rejected generic MCP authority ingress changed revision: before=%d after=%d", beforeRevision, repository.aggregate.StateRevision)
		}
	})

	t.Run("owner_committed_is_not_target_authority", func(t *testing.T) {
		tampered := cloneRuntimeAggregateForMCPProvenanceTest(t, ownerCommitted)
		authority := cloneRuntimeAggregateForMCPProvenanceTest(t, persisted)
		tampered.Facts.Failure = authority.Facts.Failure
		tampered.Facts.Resources.FailureTarget = authority.Facts.Resources.FailureTarget
		assertMCPProvenanceFreshLoadRejected(t, tampered, "evidence-committed execution")
	})

	t.Run("missing_rejected_evidence_is_not_target_authority", func(t *testing.T) {
		tampered := cloneRuntimeAggregateForMCPProvenanceTest(t, persisted)
		tampered.Facts.Evidence.Rejected = nil
		assertMCPProvenanceFreshLoadRejected(t, tampered, "has no persisted fact")
	})

	t.Run("missing_processed_event_fence_is_not_target_authority", func(t *testing.T) {
		tampered := cloneRuntimeAggregateForMCPProvenanceTest(t, persisted)
		receiptIndex := runtimeMCPFailureReceiptIndexForProvenanceTest(t, tampered)
		eventID := "runtime-receipt|" + runtimecontract.StableRuntimeOperationReceiptHash(tampered.OperationReceipts[receiptIndex])
		filtered := tampered.ProcessedEventIDs[:0]
		for _, value := range tampered.ProcessedEventIDs {
			if value != eventID {
				filtered = append(filtered, value)
			}
		}
		tampered.ProcessedEventIDs = filtered
		assertMCPProvenanceFreshLoadRejected(t, tampered, "processed-event fence")
	})

	t.Run("duplicate_rejected_evidence_is_not_target_authority", func(t *testing.T) {
		tampered := cloneRuntimeAggregateForMCPProvenanceTest(t, persisted)
		receiptIndex := runtimeMCPFailureReceiptIndexForProvenanceTest(t, tampered)
		eventID := "runtime-receipt|" + runtimecontract.StableRuntimeOperationReceiptHash(tampered.OperationReceipts[receiptIndex])
		for _, evidence := range tampered.Facts.Evidence.Rejected {
			if string(evidence.ID) == eventID {
				tampered.Facts.Evidence.Rejected = append(tampered.Facts.Evidence.Rejected, evidence)
				break
			}
		}
		assertMCPProvenanceFreshLoadRejected(t, tampered, "canonical raw facts")
	})

	t.Run("rejected_evidence_lineage_drift_is_not_target_authority", func(t *testing.T) {
		tampered := cloneRuntimeAggregateForMCPProvenanceTest(t, persisted)
		receiptIndex := runtimeMCPFailureReceiptIndexForProvenanceTest(t, tampered)
		eventID := "runtime-receipt|" + runtimecontract.StableRuntimeOperationReceiptHash(tampered.OperationReceipts[receiptIndex])
		for index := range tampered.Facts.Evidence.Rejected {
			if string(tampered.Facts.Evidence.Rejected[index].ID) == eventID {
				tampered.Facts.Evidence.Rejected[index].SourceRef = "runtime-receipt/drifted"
			}
		}
		assertMCPProvenanceFreshLoadRejected(t, tampered, "evidence fact projection mismatch")
	})

	t.Run("rejected_evidence_focus_drift_with_coordinated_hash_is_not_target_authority", func(t *testing.T) {
		tampered := cloneRuntimeAggregateForMCPProvenanceTest(t, persisted)
		receiptIndex := runtimeMCPFailureReceiptIndexForProvenanceTest(t, tampered)
		eventID := common.EvidenceRecordID("runtime-receipt|" + runtimecontract.StableRuntimeOperationReceiptHash(tampered.OperationReceipts[receiptIndex]))
		for index := range tampered.Facts.Evidence.Rejected {
			if tampered.Facts.Evidence.Rejected[index].ID == eventID {
				tampered.Facts.Evidence.Rejected[index].FocusID = "focus-forged"
				runtimeTestRebindEvidenceFactHash(t, &tampered, eventID)
			}
		}
		assertMCPProvenanceFreshLoadRejected(t, tampered, "receipt-derived rejected evidence does not match")
	})

	for _, test := range []struct {
		name   string
		mutate func(*runtimecontract.EvidenceFact)
		want   string
	}{
		{name: "kind", mutate: func(value *runtimecontract.EvidenceFact) { value.Kind = runtimecontract.EvidenceKindStateChange }, want: "evidence fact projection mismatch"},
		{name: "revision", mutate: func(value *runtimecontract.EvidenceFact) { value.Revision = "evidence-999" }, want: "evidence fact projection mismatch"},
		{name: "binding refs", mutate: func(value *runtimecontract.EvidenceFact) { value.BindingRefs = []string{"binding/forged"} }, want: "evidence fact projection mismatch"},
		{name: "coverage work item ids", mutate: func(value *runtimecontract.EvidenceFact) {
			value.CoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-forged"}
		}, want: "evidence fact projection mismatch"},
		{name: "evidence reason", mutate: func(value *runtimecontract.EvidenceFact) { value.EvidenceReason = "forged reason" }, want: "evidence fact projection mismatch"},
	} {
		t.Run("rejected_evidence_"+test.name+"_tamper_fails_fresh_load", func(t *testing.T) {
			tampered := cloneRuntimeAggregateForMCPProvenanceTest(t, persisted)
			receiptIndex := runtimeMCPFailureReceiptIndexForProvenanceTest(t, tampered)
			eventID := common.EvidenceRecordID("runtime-receipt|" + runtimecontract.StableRuntimeOperationReceiptHash(tampered.OperationReceipts[receiptIndex]))
			found := false
			for index := range tampered.Facts.Evidence.Rejected {
				if tampered.Facts.Evidence.Rejected[index].ID == eventID {
					test.mutate(&tampered.Facts.Evidence.Rejected[index])
					found = true
				}
			}
			if !found {
				t.Fatal("rejected evidence fact is missing")
			}
			assertMCPProvenanceFreshLoadRejected(t, tampered, test.want)
		})
	}

	t.Run("coordinated_live_runtime_operation_identity_is_rejected", func(t *testing.T) {
		tampered := cloneRuntimeAggregateForMCPProvenanceTest(t, persisted)
		receiptIndex := runtimeMCPFailureReceiptIndexForProvenanceTest(t, tampered)
		receipt := tampered.OperationReceipts[receiptIndex].Normalized()
		oldEventID := common.EvidenceRecordID("runtime-receipt|" + runtimecontract.StableRuntimeOperationReceiptHash(receipt))
		liveEventID := common.EvidenceRecordID("runtime-operation|" + receipt.Feedback.DispatchID + "|" + receipt.OperationID)
		liveSourceRef := "workorch://runtime-operations/" + receipt.Feedback.DispatchID + "/" + receipt.OperationID
		var rewritten runtimecontract.EvidenceFact
		for index := range tampered.Facts.Evidence.Rejected {
			if tampered.Facts.Evidence.Rejected[index].ID != oldEventID {
				continue
			}
			tampered.Facts.Evidence.Rejected[index].ID = liveEventID
			tampered.Facts.Evidence.Rejected[index].SourceRef = liveSourceRef
			rewritten = tampered.Facts.Evidence.Rejected[index]
		}
		for index := range tampered.ProcessedEventIDs {
			if tampered.ProcessedEventIDs[index] == string(oldEventID) {
				tampered.ProcessedEventIDs[index] = string(liveEventID)
			}
		}
		for dispatchIndex := range tampered.CompletedDispatches {
			dispatch := &tampered.CompletedDispatches[dispatchIndex]
			if dispatch.ID != receipt.Feedback.DispatchID {
				continue
			}
			for executionIndex := range dispatch.OperationExecutions {
				execution := &dispatch.OperationExecutions[executionIndex]
				if execution.OperationID == receipt.OperationID {
					execution.EvidenceFactID = liveEventID
					execution.EvidenceFactHash = runtimecontract.StableRuntimeEvidenceFactHash(rewritten)
				}
			}
		}
		assertMCPProvenanceFreshLoadRejected(t, tampered, "does not match its exact receipt")
	})

	t.Run("execution_evidence_fact_hash_drift_is_rejected", func(t *testing.T) {
		tampered := cloneRuntimeAggregateForMCPProvenanceTest(t, persisted)
		receiptIndex := runtimeMCPFailureReceiptIndexForProvenanceTest(t, tampered)
		receipt := tampered.OperationReceipts[receiptIndex].Normalized()
		for dispatchIndex := range tampered.CompletedDispatches {
			dispatch := &tampered.CompletedDispatches[dispatchIndex]
			if dispatch.ID != receipt.Feedback.DispatchID {
				continue
			}
			for executionIndex := range dispatch.OperationExecutions {
				if dispatch.OperationExecutions[executionIndex].OperationID == receipt.OperationID {
					dispatch.OperationExecutions[executionIndex].EvidenceFactHash = "forged-evidence-fact-hash"
				}
			}
		}
		assertMCPProvenanceFreshLoadRejected(t, tampered, "evidence fact projection mismatch")
	})

	t.Run("receipt_diagnostic_drift_is_not_target_authority", func(t *testing.T) {
		tampered := cloneRuntimeAggregateForMCPProvenanceTest(t, persisted)
		receiptIndex := runtimeMCPFailureReceiptIndexForProvenanceTest(t, tampered)
		receipt := &tampered.OperationReceipts[receiptIndex]
		receipt.Feedback.OriginalDiagnostic.OriginalMessage += " drifted"
		newHash := runtimecontract.StableRuntimeOperationReceiptHash(*receipt)
		for dispatchIndex := range tampered.CompletedDispatches {
			dispatch := &tampered.CompletedDispatches[dispatchIndex]
			if dispatch.ID != receipt.Feedback.DispatchID {
				continue
			}
			for executionIndex := range dispatch.OperationExecutions {
				if dispatch.OperationExecutions[executionIndex].OperationID == receipt.OperationID {
					dispatch.OperationExecutions[executionIndex].OwnerReceiptHash = newHash
				}
			}
		}
		assertMCPProvenanceFreshLoadRejected(t, tampered, "does not match its exact receipt")
	})

	t.Run("coordinated_observation_content_hash_drift_is_not_target_authority", func(t *testing.T) {
		tampered := cloneRuntimeAggregateForMCPProvenanceTest(t, persisted)
		mutateMCPObservationCopiesAndReceiptHashForProvenanceTest(t, &tampered, func(value *runtimecontract.RuntimeFailureTargetObservation) {
			value.ContentHash = strings.Repeat("b", 64)
		})
		assertMCPProvenanceFreshLoadRejected(t, tampered, "does not match receipt evidence")
	})

	t.Run("coordinated_observation_evidence_refs_drift_is_not_target_authority", func(t *testing.T) {
		tampered := cloneRuntimeAggregateForMCPProvenanceTest(t, persisted)
		mutateMCPObservationCopiesAndReceiptHashForProvenanceTest(t, &tampered, func(value *runtimecontract.RuntimeFailureTargetObservation) {
			value.EvidenceRefs = append(value.EvidenceRefs, "workorch://mcp/evidence/drifted")
		})
		assertMCPProvenanceFreshLoadRejected(t, tampered, "does not match receipt evidence")
	})

	t.Run("coordinated_observation_diagnostic_class_drift_is_not_target_authority", func(t *testing.T) {
		tampered := cloneRuntimeAggregateForMCPProvenanceTest(t, persisted)
		mutateMCPObservationCopiesAndReceiptHashForProvenanceTest(t, &tampered, func(value *runtimecontract.RuntimeFailureTargetObservation) {
			value.DiagnosticClass = "untyped_failure"
		})
		assertMCPProvenanceFreshLoadRejected(t, tampered, "invalid diagnostic class")
	})

	t.Run("coordinated_target_overflow_is_not_target_authority", func(t *testing.T) {
		tampered := cloneRuntimeAggregateForMCPProvenanceTest(t, persisted)
		targets := make([]string, 0, runtimecontract.RuntimeFailureTargetObservationMaxTargetRefs+1)
		for index := 0; index <= runtimecontract.RuntimeFailureTargetObservationMaxTargetRefs; index++ {
			targets = append(targets, fmt.Sprintf("target-%02d.go", index))
		}
		mutateMCPObservationCopiesAndReceiptHashForProvenanceTest(t, &tampered, func(value *runtimecontract.RuntimeFailureTargetObservation) {
			value.TargetRefs = append([]string(nil), targets...)
		})
		receiptIndex := runtimeMCPFailureReceiptIndexForProvenanceTest(t, tampered)
		receipt := &tampered.OperationReceipts[receiptIndex]
		receipt.Feedback.OriginalDiagnostic.TargetRefs = append([]string(nil), targets...)
		if tampered.Facts.Failure != nil {
			tampered.Facts.Failure.TargetRefs = append([]string(nil), targets...)
			if tampered.Facts.Failure.OriginalDiagnostic != nil {
				tampered.Facts.Failure.OriginalDiagnostic.TargetRefs = append([]string(nil), targets...)
			}
		}
		if tampered.Facts.Resources.FailureTarget != nil {
			tampered.Facts.Resources.FailureTarget.TargetRefs = append([]string(nil), targets...)
		}
		newHash := runtimecontract.StableRuntimeOperationReceiptHash(*receipt)
		for dispatchIndex := range tampered.CompletedDispatches {
			dispatch := &tampered.CompletedDispatches[dispatchIndex]
			if dispatch.ID != receipt.Feedback.DispatchID {
				continue
			}
			for executionIndex := range dispatch.OperationExecutions {
				if dispatch.OperationExecutions[executionIndex].OperationID == receipt.OperationID {
					dispatch.OperationExecutions[executionIndex].OwnerReceiptHash = newHash
				}
			}
		}
		assertMCPProvenanceFreshLoadRejected(t, tampered, "bounded targets")
	})

	t.Run("current_owner_catalog_revision_drift_is_not_target_authority", func(t *testing.T) {
		tampered := cloneRuntimeAggregateForMCPProvenanceTest(t, persisted)
		updated := false
		for index := range tampered.Facts.Resources.PrerequisiteObservations {
			observation := &tampered.Facts.Resources.PrerequisiteObservations[index]
			if observation.OwnerModuleID != runtimecontract.RuntimeModuleMCP || observation.Kind != runtimecontract.PrerequisiteKindOperationAdmissibility {
				continue
			}
			for dependencyIndex := range observation.Dependencies {
				dependency := &observation.Dependencies[dependencyIndex]
				if dependency.Domain == runtimecontract.RuntimeResourceOwnerCatalog && dependency.OwnerModuleID == runtimecontract.RuntimeModuleMCP {
					dependency.Revision += "-drifted"
					updated = true
				}
			}
			observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(*observation)
		}
		if !updated {
			t.Fatal("MCP provenance fixture has no current owner-catalog revision dependency")
		}
		assertMCPProvenanceFreshLoadRejected(t, tampered, "current exact owner catalog and prerequisite freshness")
	})
}

func assertMCPReceiptEvidenceDriftRejectedAtCommit(t *testing.T, sealed RuntimeAggregate, operation runtimecontract.RuntimeOperation) {
	t.Helper()
	for _, test := range []struct {
		name   string
		mutate func(*runtimecontract.RuntimeFeedback)
		want   string
	}{
		{name: "response evidence drift", want: "does not match receipt evidence", mutate: func(value *runtimecontract.RuntimeFeedback) {
			value.OriginalDiagnostic.TargetObservation.ContentHash = strings.Repeat("b", 64)
		}},
		{name: "target overflow", want: "bounded targets", mutate: func(value *runtimecontract.RuntimeFeedback) {
			targets := make([]string, 0, runtimecontract.RuntimeFailureTargetObservationMaxTargetRefs+1)
			for index := 0; index <= runtimecontract.RuntimeFailureTargetObservationMaxTargetRefs; index++ {
				targets = append(targets, fmt.Sprintf("target-%02d.go", index))
			}
			value.OriginalDiagnostic.TargetRefs = append([]string(nil), targets...)
			value.OriginalDiagnostic.TargetObservation.TargetRefs = append([]string(nil), targets...)
		}},
	} {
		t.Run("commit_"+test.name, func(t *testing.T) {
			repository := &durableRuntimeStateRepository{aggregate: cloneRuntimeAggregateForMCPProvenanceTest(t, sealed)}
			controller, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			claimed, err := controller.claimOperationExecution(context.Background(), sealed.Facts.WorkspaceID, sealed.Facts.RunID, operation.ID)
			if err != nil {
				t.Fatal(err)
			}
			execution, found := operationExecutionRecord(claimed.ActiveDispatch.OperationExecutions, operation.ID)
			if !found {
				t.Fatal("MCP commit-drift fixture has no claimed execution")
			}
			evidenceHash := strings.Repeat("a", 64)
			evidence := runtimecontract.RuntimeOperationEvidence{
				Summary:        "persisted MCP owner response",
				ContentHash:    evidenceHash,
				ContentAddress: &runtimecontract.RuntimeEvidenceContentAddress{Algorithm: runtimecontract.RuntimeEvidenceDigestSHA256, Digest: evidenceHash, Ref: "workorch://mcp/evidence/" + evidenceHash},
				EvidenceRefs:   []string{"workorch://mcp/capability/mcp-observed-target/catalog-1", "workorch://mcp/evidence/" + evidenceHash},
			}
			observation := runtimecontract.RuntimeFailureTargetObservation{
				OwnerModuleID: runtimecontract.RuntimeModuleMCP, SubjectRef: "mcp-observed-target", TargetRole: "implementation",
				ObservedRevision: "catalog-1", TargetRefs: []string{"implementation.go"},
				CatalogDependency: &runtimecontract.RuntimeFreshnessDependency{Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: runtimecontract.RuntimeModuleMCP, SubjectRef: "mcp_server_catalog", Revision: "catalog-1"},
				DiagnosticClass:   runtimecontract.RuntimeFailureTargetDiagnosticReceiptEvidence,
				EvidenceRefs:      append([]string(nil), evidence.EvidenceRefs...), ContentHash: evidence.ContentHash,
			}
			failure := runtimecontract.FailureFact{
				Class: runtimecontract.FailureClassExecutionFailure, Code: "tool_call_failed", Signature: "mcp-commit-drift",
				Message: "MCP owner operation failed", TargetRefs: append([]string(nil), observation.TargetRefs...), TargetObservation: &observation,
			}
			feedback := runtimeOperationFeedbackForTest(t, claimed, operation.ID, failure)
			test.mutate(feedback)
			receipt := runtimecontract.RuntimeOperationReceipt{
				ExecutionKey: execution.ExecutionKey, OperationID: operation.ID,
				Outcome: runtimecontract.RuntimeOperationOutcomeRejected, ObservedEffectScope: runtimecontract.RuntimeEffectUnknown,
				TargetRefs: append([]string(nil), operation.TargetRefs...), Evidence: evidence,
				Feedback: feedback,
			}
			revision := repository.aggregate.StateRevision
			receiptCount := len(repository.aggregate.OperationReceipts)
			if _, _, err := controller.commitOperationReceipt(context.Background(), claimed.Facts.WorkspaceID, claimed.Facts.RunID, claimed.ActiveDispatch.ID, execution.Fence, runtimecontract.RuntimeModuleMCP, receipt); err == nil ||
				!strings.Contains(err.Error(), test.want) {
				t.Fatalf("MCP %s commit error = %v", test.name, err)
			}
			if repository.aggregate.StateRevision != revision || len(repository.aggregate.OperationReceipts) != receiptCount {
				t.Fatalf("rejected MCP %s changed aggregate: revision=%d receipts=%d", test.name, repository.aggregate.StateRevision, len(repository.aggregate.OperationReceipts))
			}
		})
	}
}

func mutateMCPObservationCopiesAndReceiptHashForProvenanceTest(
	t *testing.T,
	aggregate *RuntimeAggregate,
	mutate func(*runtimecontract.RuntimeFailureTargetObservation),
) {
	t.Helper()
	receiptIndex := runtimeMCPFailureReceiptIndexForProvenanceTest(t, *aggregate)
	receipt := &aggregate.OperationReceipts[receiptIndex]
	mutate(receipt.Feedback.OriginalDiagnostic.TargetObservation)
	mutateFailure := func(failure *runtimecontract.FailureFact) {
		if failure == nil {
			return
		}
		if failure.TargetObservation != nil {
			mutate(failure.TargetObservation)
		}
		if failure.OriginalDiagnostic != nil && failure.OriginalDiagnostic.TargetObservation != nil {
			mutate(failure.OriginalDiagnostic.TargetObservation)
		}
	}
	mutateFailure(aggregate.Facts.Failure)
	if aggregate.Facts.Resources.FailureTarget != nil && aggregate.Facts.Resources.FailureTarget.TargetObservation != nil {
		mutate(aggregate.Facts.Resources.FailureTarget.TargetObservation)
	}
	if aggregate.Facts.RepairDirective != nil {
		mutateFailure(aggregate.Facts.RepairDirective.CurrentFailure)
	}
	newHash := runtimecontract.StableRuntimeOperationReceiptHash(*receipt)
	for dispatchIndex := range aggregate.CompletedDispatches {
		dispatch := &aggregate.CompletedDispatches[dispatchIndex]
		if dispatch.ID != receipt.Feedback.DispatchID {
			continue
		}
		for executionIndex := range dispatch.OperationExecutions {
			if dispatch.OperationExecutions[executionIndex].OperationID == receipt.OperationID {
				dispatch.OperationExecutions[executionIndex].OwnerReceiptHash = newHash
			}
		}
	}
}

func cloneRuntimeAggregateForMCPProvenanceTest(t *testing.T, aggregate RuntimeAggregate) RuntimeAggregate {
	t.Helper()
	payload, err := json.Marshal(aggregate)
	if err != nil {
		t.Fatal(err)
	}
	var cloned RuntimeAggregate
	if err := json.Unmarshal(payload, &cloned); err != nil {
		t.Fatal(err)
	}
	return cloned
}

func runtimeMCPFailureReceiptIndexForProvenanceTest(t *testing.T, aggregate RuntimeAggregate) int {
	t.Helper()
	for index, receipt := range aggregate.OperationReceipts {
		if receipt.Outcome == runtimecontract.RuntimeOperationOutcomeRejected && receipt.Feedback != nil && receipt.Feedback.OriginalDiagnostic != nil &&
			receipt.Feedback.OriginalDiagnostic.OwnerModuleID == runtimecontract.RuntimeModuleMCP && receipt.Feedback.OriginalDiagnostic.TargetObservation != nil {
			return index
		}
	}
	t.Fatal("MCP provenance fixture has no rejected target-observation receipt")
	return -1
}

func assertMCPProvenanceFreshLoadRejected(t *testing.T, aggregate RuntimeAggregate, message string) {
	t.Helper()
	if err := aggregate.Validate(); err == nil || !strings.Contains(err.Error(), message) {
		t.Fatalf("fresh-load MCP target authority error = %v, want %q", err, message)
	}
}

type runtimeMCPServerGetter struct{ server *mcpmodel.Server }

func (g runtimeMCPServerGetter) GetServer(_ context.Context, id common.MCPServerID) (*mcpmodel.Server, error) {
	if g.server == nil || g.server.ID != id {
		return nil, common.ErrNotFound
	}
	return g.server, nil
}

func (g runtimeMCPServerGetter) RegistrySnapshot(context.Context) (*mcpmodel.RegistrySnapshot, string, error) {
	if g.server == nil {
		return nil, "", common.ErrNotFound
	}
	capabilities := []*mcpmodel.Capability{{
		ID:       common.MCPCapabilityID(string(g.server.ID) + ":tool:lookup.references"),
		ServerID: g.server.ID, Kind: mcpmodel.CapabilityKindTool, Name: "lookup.references", Enabled: true,
		EffectScope: string(runtimecontract.RuntimeEffectNone), IdempotencyClass: string(runtimecontract.RuntimeOperationIdempotencySafeToReplay),
		ValidationProofApplicability: mcpmodel.ValidationProofApplicabilityNotDeclared,
		Schema: workorcheventpayload.New(
			workorcheventpayload.StringField("type", "object"),
			workorcheventpayload.ObjectField("properties", workorcheventpayload.New(
				workorcheventpayload.ObjectField("path", workorcheventpayload.New(workorcheventpayload.StringField("type", "string"))),
			)),
			workorcheventpayload.BoolField("additionalProperties", false),
		),
	}}
	return &mcpmodel.RegistrySnapshot{Servers: mcpmodel.CloneServers([]*mcpmodel.Server{g.server}), Capabilities: capabilities}, "test-mcp-registry-" + string(g.server.ID), nil
}

func runtimeStringPointer(value string) *string { return &value }

type runtimeObservedMCPExecutor struct {
	hub                  magicevent.Hub
	deliverableDirectory string
	internalDirectory    string
}

func (e runtimeObservedMCPExecutor) ExecuteRuntimeOperation(ctx context.Context, request RuntimeOperationExecutionRequest) (RuntimeOperationExecutionResult, error) {
	if request.OwnerCatalog == nil {
		return RuntimeOperationExecutionResult{}, fmt.Errorf("MCP owner catalog is missing")
	}
	sealedOperation, err := mcpevents.SealRuntimeOperation(request.Operation, *request.OwnerCatalog)
	if err != nil {
		return RuntimeOperationExecutionResult{}, err
	}
	result := e.hub.Send(magicevent.NewEventWithContext(
		mcpevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, runtimecontract.RuntimeModuleMCP,
		magicevent.NewHeader(), ctx, mcpevents.ExecuteOperationCommand{
			RuntimeOperationExecutionContext: runtimecontract.RuntimeOperationExecutionContext{
				Execution:   runtimeOperationExecutionEnvelopeForTest(request.Execution, request.GovernanceGrant),
				WorkspaceID: request.WorkspaceID, RunID: request.RunID, DispatchID: request.DispatchID, Transaction: request.Transaction,
				OperationID: request.Operation.ID, CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), request.Operation.CoverageWorkItemIDs...),
				TargetRefs: append([]string(nil), request.Operation.TargetRefs...), EvidenceKind: request.EvidenceKind,
				EvidencePolicies: append([]runtimecontract.RuntimeEvidencePolicy(nil), request.EvidencePolicies...), WorkspaceDeliverableDirectory: e.deliverableDirectory, WorkspaceInternalDirectory: e.internalDirectory,
			},
			SealedOperation: sealedOperation,
			TargetRole:      request.TargetRole,
		}))
	if result == nil {
		return RuntimeOperationExecutionResult{}, fmt.Errorf("MCP owner returned no execution result")
	}
	responseData, resultErr := result.Get()
	if resultErr != nil {
		return RuntimeOperationExecutionResult{}, resultErr
	}
	response, ok := responseData.(mcpevents.ExecuteOperationResult)
	if !ok {
		return RuntimeOperationExecutionResult{}, fmt.Errorf("MCP owner result type %T", responseData)
	}
	status, evidence, governance := orchestratorevents.RuntimeOperationResultFromReceipt(response.Receipt)
	failure := failureFactFromOperationReceiptFeedback(response.Receipt)
	return RuntimeOperationExecutionResult{EventID: orchestratorevents.RuntimeOperationEventID(request.DispatchID, request.Operation.ID), OperationID: request.Operation.ID, EvidenceKind: request.EvidenceKind, Status: status, SourceRef: orchestratorevents.RuntimeOperationSourceRef(request.DispatchID, request.Operation.ID), Failure: failure, Evidence: evidence, Receipt: response.Receipt, Governance: governance}, nil
}

// runtimeObservedFilePatchExecutor keeps the H5 state-change receipt on the
// same concrete owner route as production: controller claim fence -> File
// owner receipt query/commit -> controller evidence CAS. It deliberately does
// not inspect coverage or choose the next action.
type runtimeObservedFilePatchExecutor struct {
	hub                  magicevent.Hub
	deliverableDirectory string
}

func (e runtimeObservedFilePatchExecutor) ExecuteRuntimeOperation(ctx context.Context, request RuntimeOperationExecutionRequest) (RuntimeOperationExecutionResult, error) {
	sealedOperation, err := fileevents.SealRuntimeOperation(request.Operation)
	if err != nil {
		return RuntimeOperationExecutionResult{}, err
	}
	result := e.hub.Send(magicevent.NewEventWithContext(
		fileevents.TopicExecutePatchApplyOperation, runtimecontract.RuntimeModuleOrchestrator, runtimecontract.RuntimeModuleFile,
		magicevent.NewHeader(), ctx, fileevents.ExecutePatchApplyOperationCommand{
			RuntimeOperationExecutionContext: runtimecontract.RuntimeOperationExecutionContext{
				Execution:                     runtimeOperationExecutionEnvelopeForTest(request.Execution, request.GovernanceGrant),
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
			},
			SealedOperation: sealedOperation,
		},
	))
	if result == nil {
		return RuntimeOperationExecutionResult{}, fmt.Errorf("File owner returned no patch execution result")
	}
	responseData, resultErr := result.Get()
	if resultErr != nil {
		return RuntimeOperationExecutionResult{}, resultErr
	}
	response, ok := responseData.(fileevents.ExecuteOperationResult)
	if !ok {
		return RuntimeOperationExecutionResult{}, fmt.Errorf("File owner result type %T", responseData)
	}
	status, evidence, governance := orchestratorevents.RuntimeOperationResultFromReceipt(response.Receipt)
	failure := failureFactFromOperationReceiptFeedback(response.Receipt)
	return RuntimeOperationExecutionResult{EventID: orchestratorevents.RuntimeOperationEventID(request.DispatchID, request.Operation.ID), OperationID: request.Operation.ID, EvidenceKind: request.EvidenceKind, Status: status, SourceRef: orchestratorevents.RuntimeOperationSourceRef(request.DispatchID, request.Operation.ID), Failure: failure, Evidence: evidence, Receipt: response.Receipt, Governance: governance}, nil
}

func sendObservedMCPExecution(t *testing.T, ctx context.Context, hub magicevent.Hub, command mcpevents.ExecuteOperationCommand) mcpevents.ExecuteOperationResult {
	t.Helper()
	result := hub.Send(magicevent.NewEventWithContext(
		mcpevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, runtimecontract.RuntimeModuleMCP,
		magicevent.NewHeader(), ctx, command,
	))
	if result == nil {
		t.Fatal("MCP owner returned no execution result")
	}
	data, resultErr := result.Get()
	if resultErr != nil {
		t.Fatal(resultErr)
	}
	response, ok := data.(mcpevents.ExecuteOperationResult)
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
		fileevents.TopicAcquireWorkspaceManifest, runtimecontract.RuntimeModuleOrchestrator, orchestratorevents.PrerequisiteCommandDestination,
		magicevent.NewHeader(), ctx, fileevents.AcquireWorkspaceManifestCommand{
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
	response, ok := data.(fileevents.AcquireWorkspaceManifestResult)
	if !ok || response.Manifest.Revision == "" || response.Observation.ObservationID == "" {
		t.Fatalf("File manifest response = %#v", data)
	}
	next, err := controller.commitPrerequisite(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, PrerequisiteResult{
		PrerequisiteID: prerequisite.ID, Manifest: &response.Manifest, Observation: &response.Observation,
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
		fileevents.TopicAcquireRuntimeOperationAdmissibility, runtimecontract.RuntimeModuleOrchestrator, runtimecontract.RuntimeModuleFile,
		magicevent.NewHeader(), ctx, fileevents.AcquireRuntimeOperationAdmissibilityCommand{
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
	response, ok := data.(fileevents.AcquireRuntimeOperationAdmissibilityResult)
	if !ok || response.Acquisition.Admissibility.SnapshotID == "" || response.Acquisition.Observation.ObservationID == "" {
		t.Fatalf("File operation admissibility response = %#v", data)
	}
	aggregate.Facts.Resources.OperationAdmissibilities = append(aggregate.Facts.Resources.OperationAdmissibilities, response.Acquisition.Admissibility)
	aggregate.Facts.Resources.PrerequisiteObservations = append(aggregate.Facts.Resources.PrerequisiteObservations, response.Acquisition.Observation)
	aggregate.Facts.Prerequisites.Satisfied = append(aggregate.Facts.Prerequisites.Satisfied, runtimecontract.RuntimePrerequisite{
		ID: prerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: runtimecontract.RuntimeModuleFile, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindPatchApply},
	})
}

func addObservedMCPOperationAdmissibilityFixture(
	t *testing.T,
	ctx context.Context,
	hub magicevent.Hub,
	aggregate *RuntimeAggregate,
	expectedRevision string,
) runtimecontract.RuntimeOperationAdmissibility {
	t.Helper()
	if aggregate == nil || aggregate.Decision.AuthoritySnapshot.ID == "" {
		t.Fatal("aggregate must have sealed authority before MCP preflight")
	}
	prerequisiteID := "operation_admissibility:mcp"
	result := hub.Send(magicevent.NewEventWithContext(
		mcpevents.TopicAcquireRuntimeOperationAdmissibility, runtimecontract.RuntimeModuleOrchestrator, runtimecontract.RuntimeModuleMCP,
		magicevent.NewHeader(), ctx, mcpevents.AcquireRuntimeOperationAdmissibilityCommand{
			WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, AuthoritySnapshotID: aggregate.Decision.AuthoritySnapshot.ID,
			PrerequisiteID: prerequisiteID, OwnerModuleID: runtimecontract.RuntimeModuleMCP,
			WorkspaceDeliverableDirectory: t.TempDir(), RequiredKinds: []string{runtimecontract.StepKindMCPToolCall}, MaxItems: 4,
		},
	))
	if result == nil {
		t.Fatal("MCP owner returned no operation admissibility result")
	}
	data, resultErr := result.Get()
	if resultErr != nil {
		t.Fatal(resultErr)
	}
	response, ok := data.(mcpevents.AcquireRuntimeOperationAdmissibilityResult)
	if !ok || response.Acquisition.Admissibility.SnapshotID == "" || response.Acquisition.Observation.ObservationID == "" ||
		response.Acquisition.Admissibility.ObservedRevision != expectedRevision {
		t.Fatalf("MCP operation admissibility response = %#v", data)
	}
	aggregate.Facts.Resources.OperationAdmissibilities = append(aggregate.Facts.Resources.OperationAdmissibilities, response.Acquisition.Admissibility)
	aggregate.Facts.Resources.PrerequisiteObservations = append(aggregate.Facts.Resources.PrerequisiteObservations, response.Acquisition.Observation)
	aggregate.Facts.Prerequisites.Satisfied = append(aggregate.Facts.Prerequisites.Satisfied, runtimecontract.RuntimePrerequisite{
		ID: prerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: runtimecontract.RuntimeModuleMCP, RequiredCapabilities: []string{runtimecontract.StepKindMCPToolCall},
	})
	return response.Acquisition.Admissibility
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
			result = PrerequisiteResult{PrerequisiteID: prerequisite.ID, Manifest: &response.Manifest, Observation: &response.Observation}
		case runtimecontract.PrerequisiteKindTargetSnapshot:
			response := sendObservedTargetSnapshot(t, ctx, hub, aggregate, prerequisite, workspaceDirectory)
			result = PrerequisiteResult{PrerequisiteID: prerequisite.ID, TargetSnapshot: &response.Snapshot, Observation: &response.Observation}
		case runtimecontract.PrerequisiteKindOperationAdmissibility:
			response := sendObservedFileAdmissibility(t, ctx, hub, aggregate, prerequisite, workspaceDirectory)
			result = PrerequisiteResult{PrerequisiteID: prerequisite.ID, OperationAdmissibility: &response.Acquisition.Admissibility, Observation: &response.Acquisition.Observation}
		default:
			t.Fatalf("unexpected pending prerequisite during File refresh: %+v", prerequisite)
		}
		var err error
		aggregate, err = controller.commitPrerequisite(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, result)
		if err != nil {
			t.Fatal(err)
		}
	}
	if aggregate.Decision.State == runtimecontract.ConvergenceStateNeedPrerequisite {
		t.Fatalf("File prerequisite refresh did not converge: pending=%+v", aggregate.Facts.Prerequisites.Pending)
	}
	return aggregate
}

func sendObservedManifest(t *testing.T, ctx context.Context, hub magicevent.Hub, aggregate RuntimeAggregate, prerequisite runtimecontract.RuntimePrerequisite, directory string) fileevents.AcquireWorkspaceManifestResult {
	t.Helper()
	result := hub.Send(magicevent.NewEventWithContext(fileevents.TopicAcquireWorkspaceManifest, runtimecontract.RuntimeModuleOrchestrator, orchestratorevents.PrerequisiteCommandDestination, magicevent.NewHeader(), ctx, fileevents.AcquireWorkspaceManifestCommand{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, AuthoritySnapshotID: aggregate.Decision.AuthoritySnapshot.ID, PrerequisiteID: prerequisite.ID, WorkspaceDeliverableDirectory: directory}))
	data, err := result.Get()
	if err != nil {
		t.Fatal(err)
	}
	response, ok := data.(fileevents.AcquireWorkspaceManifestResult)
	if !ok || response.Observation.ObservationID == "" {
		t.Fatalf("manifest refresh response = %#v", data)
	}
	return response
}

func sendObservedTargetSnapshot(t *testing.T, ctx context.Context, hub magicevent.Hub, aggregate RuntimeAggregate, prerequisite runtimecontract.RuntimePrerequisite, directory string) fileevents.AcquireTargetSnapshotResult {
	t.Helper()
	result := hub.Send(magicevent.NewEventWithContext(fileevents.TopicAcquireTargetSnapshot, runtimecontract.RuntimeModuleOrchestrator, orchestratorevents.PrerequisiteCommandDestination, magicevent.NewHeader(), ctx, fileevents.AcquireTargetSnapshotCommand{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, AuthoritySnapshotID: aggregate.Decision.AuthoritySnapshot.ID, PrerequisiteID: prerequisite.ID, WorkspaceDeliverableDirectory: directory, TargetRef: prerequisite.SubjectRef, MaxBytes: 16 * 1024}))
	data, err := result.Get()
	if err != nil {
		t.Fatal(err)
	}
	response, ok := data.(fileevents.AcquireTargetSnapshotResult)
	if !ok || response.Observation.ObservationID == "" || !response.Snapshot.Complete() {
		t.Fatalf("snapshot refresh response = %#v", data)
	}
	return response
}

func sendObservedFileAdmissibility(t *testing.T, ctx context.Context, hub magicevent.Hub, aggregate RuntimeAggregate, prerequisite runtimecontract.RuntimePrerequisite, directory string) fileevents.AcquireRuntimeOperationAdmissibilityResult {
	t.Helper()
	result := hub.Send(magicevent.NewEventWithContext(fileevents.TopicAcquireRuntimeOperationAdmissibility, runtimecontract.RuntimeModuleOrchestrator, runtimecontract.RuntimeModuleFile, magicevent.NewHeader(), ctx, fileevents.AcquireRuntimeOperationAdmissibilityCommand{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, AuthoritySnapshotID: aggregate.Decision.AuthoritySnapshot.ID, PrerequisiteID: prerequisite.ID, OwnerModuleID: prerequisite.OwnerModuleID, WorkspaceDeliverableDirectory: directory, RequiredKinds: prerequisite.RequiredCapabilities, RequiredTargetRefs: GroundedRepairTargets(aggregate.Facts), MaxItems: 4}))
	data, err := result.Get()
	if err != nil {
		t.Fatal(err)
	}
	response, ok := data.(fileevents.AcquireRuntimeOperationAdmissibilityResult)
	if !ok || response.Acquisition.Observation.ObservationID == "" || len(response.Acquisition.Admissibility.TargetRefs) != 1 || response.Acquisition.Admissibility.TargetRefs[0] != "implementation.go" {
		t.Fatalf("admissibility refresh response = %#v", data)
	}
	return response
}

type runtimeCapturedCandidateProvider struct {
	requests   []llmmodels.RuntimeCandidateRequest
	payload    []byte
	payloadFor func(llmmodels.RuntimeCandidateRequest) []byte
}

func (p *runtimeCapturedCandidateProvider) GenerateCandidate(_ context.Context, request llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error) {
	p.requests = append(p.requests, request)
	payload := p.payload
	if p.payloadFor != nil {
		payload = p.payloadFor(request)
	}
	target := request.CandidateContext.Activation.ProviderSelection.Target.Normalized()
	workspaceID, runID, kind := string(request.CandidateContext.WorkspaceID), string(request.CandidateContext.RunID), "runtime_candidate"
	if request.DocumentKind == runtimecontract.RuntimeProviderDocumentSemanticPlan {
		target = request.SemanticContext.Activation.ProviderSelection.Target.Normalized()
		workspaceID, runID, kind = string(request.SemanticContext.WorkspaceID), string(request.SemanticContext.RunID), "runtime_semantic_plan"
	}
	return llmmodels.RuntimeCandidateGenerationResult{
		RequestID: request.RequestIdentity.PhysicalCallID, Payload: append([]byte(nil), payload...), Status: llmmodels.RuntimeCandidateStatusCompleted,
		Audit: &llmmodels.ProviderRequestAudit{
			RequestID: request.RequestIdentity.PhysicalCallID, LogicalRequestID: request.RequestIdentity.LogicalRequestID,
			PhysicalCallID: request.RequestIdentity.PhysicalCallID, WorkspaceID: workspaceID, RunID: runID,
			Target: target, Operation: "responses", StablePrefixHash: request.StablePrefixHash, Kind: kind,
			PhysicalResponseMode: llmaudit.PhysicalResponseModeNonStreaming, ProviderSubmitted: true,
			Status: llmmodels.RuntimeCandidateStatusCompleted, RequestBytes: 1, ResponseBytes: len(payload),
			ContextBudget: &llmmodels.ProviderContextBudget{
				ProviderID: target.ProviderID, ModelID: target.ModelID, ProviderSubmitted: true,
				CachePlan: &llmmodels.ProviderCachePlan{StablePrefixHash: request.StablePrefixHash},
			},
		},
	}, nil
}

func runtimeObservedCandidatePayload(request llmmodels.RuntimeCandidateRequest) []byte {
	switch request.Decision.State {
	case runtimecontract.ConvergenceStateNeedStateChange:
		baseHash := ""
		for _, snapshot := range request.CandidateContext.Resources.TargetSnapshots {
			if snapshot.TargetRef == "implementation.go" {
				baseHash = snapshot.ContentHash
				break
			}
		}
		patchType, content := "file_write", "package fixture\n// repaired\n"
		candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
			Kind: runtimecontract.RuntimeBlockKindPatchApply, BindingRefs: runtimeObservedCandidateBindingRefs(request, runtimecontract.RuntimeBlockKindPatchApply), TargetRefs: []string{"implementation.go"},
			Parameters: []runtimecontract.OperationParameter{{Name: "patchType", String: &patchType}, {Name: "content", String: &content}, {Name: "baseHash", String: &baseHash}},
		}}}}}
		payload, _ := runtimecontract.EncodeRuntimeCandidatePayloadWithManifest(request.CandidateManifest, candidate)
		return payload
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
		serverID, tool := "mcp-observed-target", "lookup.references"
		candidate := runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: runtimecontract.RuntimeOperation{
			Kind: runtimecontract.StepKindMCPToolCall, BindingRefs: runtimeObservedCandidateBindingRefs(request, runtimecontract.StepKindMCPToolCall), TargetRefs: []string{target},
			Parameters: []runtimecontract.OperationParameter{{Name: "serverId", String: &serverID}, {Name: "tool", String: &tool}, {Name: "input.path", String: &target}},
		}}}}}
		payload, _ := runtimecontract.EncodeRuntimeCandidatePayloadWithManifest(request.CandidateManifest, candidate)
		return payload
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

func runtimeObservedCandidateBindingRefs(request llmmodels.RuntimeCandidateRequest, kind string) []string {
	refs := make([]string, 0)
	seen := map[string]struct{}{}
	for _, authorization := range request.CandidateManifest.Operations {
		if runtimecontract.CanonicalRuntimeBlockKind(authorization.Kind) == runtimecontract.CanonicalRuntimeBlockKind(kind) {
			for _, bindingAuthorization := range authorization.BindingAuthorizations {
				for _, ref := range bindingAuthorization.AllowedBindingRefs {
					if _, found := seen[ref]; found {
						continue
					}
					seen[ref] = struct{}{}
					refs = append(refs, ref)
				}
			}
			break
		}
	}
	if len(refs) == 0 {
		return nil
	}
	sort.Strings(refs)
	return []string{refs[0]}
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
	// Each planning document is a distinct logical provider request. Reusing a
	// fixed PhysicalCallID across state-change, validation, and assessment would
	// model an illegal replay and is now correctly rejected by the LLM owner.
	logicalRequestID := fmt.Sprintf("candidate-request-observed-seal-%s-%d", aggregate.Decision.State, aggregate.StateRevision)
	requestIdentity := runtimecontract.RuntimeCandidateRequestIdentity{
		LogicalRequestID: logicalRequestID,
		Sequence:         1,
		PhysicalCallID:   runtimecontract.StableRuntimeCandidatePhysicalCallID(logicalRequestID, 1),
	}
	expectedDocument := runtimecontract.RuntimeExpectedDocumentContract{
		SchemaVersion: runtimecontract.RuntimeExpectedDocumentSchemaCurrent,
		DocumentKind:  runtimecontract.RuntimeProviderDocumentOperations, ManifestHash: manifest.Hash,
		LogicalRequestID: requestIdentity.LogicalRequestID, StablePrefixHash: prefix,
		MaxOutputTokens: candidateContext.Resources.ProviderCeiling.EffectiveMaxOutputTokens, MaxRecoveryAttempts: 1,
	}
	wire, err := runtimecontract.BuildRuntimeCandidateWireContract(manifest)
	if err != nil {
		t.Fatal(err)
	}
	expectedDocument.WireContractHash = wire.Hash
	expectedDocument.Hash = runtimecontract.StableRuntimeExpectedDocumentHash(expectedDocument)
	result := hub.Send(magicevent.NewEventWithContext(llmevents.TopicRequestRuntimeCandidate, runtimecontract.RuntimeModuleOrchestrator, runtimecontract.RuntimeModuleLLM, magicevent.NewHeader(), ctx, llmevents.RequestRuntimeCandidateCommand{DocumentKind: runtimecontract.RuntimeProviderDocumentOperations, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, RequestIdentity: requestIdentity, ExpectedDocument: &expectedDocument, Decision: aggregate.Decision, Transaction: aggregate.Decision.AuthorityTransaction(), StablePrefixHash: prefix, CandidateManifest: manifest, DefinitionContext: aggregate.Definition, CandidateContext: candidateContext}))
	data, requestErr := result.Get()
	if requestErr != nil {
		t.Fatal(requestErr)
	}
	response, ok := data.(llmevents.RequestRuntimeCandidateResult)
	if !ok || response.Feedback != nil {
		t.Fatalf("candidate response = %#v", data)
	}
	if response.DocumentOutcome == nil {
		t.Fatal("LLM candidate response did not carry a document outcome")
	}
	candidate, err := runtimecontract.DecodeRuntimeCandidatePayloadWithManifest(manifest, response.DocumentOutcome.Content)
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
		probeResult, probeErr := sendRuntimeOperationFeasibilityProbeForTest(ctx, hub, runtimecontract.RuntimeOperationFeasibilityRequest{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, Transaction: aggregate.Decision.AuthorityTransaction(), WorkspaceDeliverableDirectory: directory, Operation: operation, Admissibility: inventory, CapabilityStateBinding: inventory.AcquisitionBinding, Observation: &observation, CandidateManifestHash: manifest.Hash, OperationAuthorization: mustRuntimeOperationAuthorizationIdentity(t, operation.Kind)})
		if probeErr != nil {
			t.Fatal(probeErr)
		}
		if probeResult.Outcome != runtimecontract.RuntimeOwnerFeasibilityFeasible || probeResult.Feedback != nil || probeResult.Stale != nil {
			t.Fatalf("candidate feasibility state=%s operation=%+v inventory=%+v result=%#v", aggregate.Decision.State, operation, inventory, probeResult)
		}
		aggregate, err = controller.recordCandidateFeasibility(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, runtimecontract.RuntimeCandidateFeasibilityRecord{OperationID: operation.ID, AdmissibilitySnapshotID: inventory.SnapshotID, AcquisitionBinding: inventory.AcquisitionBinding, CandidateManifestHash: probeResult.CandidateManifestHash, Observation: &runtimecontract.RuntimeOwnerFeasibilityObservation{Outcome: runtimecontract.RuntimeOwnerFeasibilityFeasible}, ReplayProof: probeResult.ReplayProof})
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
		resources.FailureTarget.TargetObservation.TargetRole == "" ||
		resources.FailureTarget.TargetObservation.TargetRole != request.CandidateContext.Focus.TargetRole ||
		resources.FailureTarget.TargetObservation.ObservedRevision == "" ||
		len(resources.FailureTarget.TargetRefs) != 1 || resources.FailureTarget.TargetRefs[0] != "implementation.go" {
		t.Fatalf("LLM request lost committed failure target authority: %+v observation=%+v", resources.FailureTarget, resources.FailureTarget.TargetObservation)
	}
	foundObservedTargetSnapshot := false
	for _, snapshot := range resources.TargetSnapshots {
		if snapshot.TargetRef == "implementation.go" && snapshot.Complete() {
			foundObservedTargetSnapshot = true
			break
		}
	}
	if !foundObservedTargetSnapshot {
		t.Fatalf("LLM request target snapshots = %+v, want grounded owner-observed target", resources.TargetSnapshots)
	}
	if !sameRuntimeStrings(request.CandidateContext.Focus.TargetRefs, request.Decision.Focus.TargetRefs) {
		t.Fatalf("LLM request rewrote sealed semantic Focus targets from failure target: %+v", request.CandidateContext.Focus)
	}
}

func runtimeMagicFailure(err error) *cd.Error {
	if err == nil {
		return nil
	}
	return &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err}
}
