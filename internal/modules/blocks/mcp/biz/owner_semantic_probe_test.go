package mcp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/events"
	registryevents "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	workorcheventpayload "github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// RSO-25 owner semantic-feasibility matrix. The read lane verifies only the
// catalog, schema/input binding and local transport prerequisites; it never
// invokes an MCP endpoint or enters the operation receipt/evidence path.
func TestMCPOwnerSemanticBehaviorProbeMatrix(t *testing.T) {
	ctx := context.Background()

	var endpointCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		endpointCalls.Add(1)
		if r.URL.Path == "/fail" {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(server.Close)

	cases := []struct {
		name             string
		effect           runtimecontract.RuntimeEffectScope
		idempotency      runtimecontract.RuntimeOperationIdempotencyClass
		evidenceKind     string
		schema           workorcheventpayload.Payload
		endpoint         string
		wantStatus       runtimecontract.RuntimeSemanticFeasibilityStatus
		wantReadiness    runtimecontract.RuntimeValidationSurfaceReadiness
		wantCode         string
		wantEndpointCall int32
	}{
		{
			name:             "safe non-durable replayable succeeds",
			effect:           runtimecontract.RuntimeEffectNone,
			idempotency:      runtimecontract.RuntimeOperationIdempotencySafeToReplay,
			schema:           mcpBehaviorProbeSchema(t, nil),
			endpoint:         server.URL,
			wantStatus:       runtimecontract.RuntimeSemanticFeasibilityFeasible,
			wantReadiness:    runtimecontract.RuntimeValidationSurfaceReady,
			wantEndpointCall: 0,
		},
		{
			name:             "safe ephemeral session replayable succeeds",
			effect:           runtimecontract.RuntimeEffectEphemeralSession,
			idempotency:      runtimecontract.RuntimeOperationIdempotencySafeToReplay,
			schema:           mcpBehaviorProbeSchema(t, nil),
			endpoint:         server.URL,
			wantStatus:       runtimecontract.RuntimeSemanticFeasibilityFeasible,
			wantReadiness:    runtimecontract.RuntimeValidationSurfaceReady,
			wantEndpointCall: 0,
		},
		{
			name:          "durable capability is unavailable",
			effect:        runtimecontract.RuntimeEffectWorkspaceDurable,
			idempotency:   runtimecontract.RuntimeOperationIdempotencySafeToReplay,
			evidenceKind:  runtimecontract.EvidenceKindStateChange,
			schema:        mcpBehaviorProbeSchema(t, nil),
			endpoint:      server.URL,
			wantStatus:    runtimecontract.RuntimeSemanticFeasibilityInfeasible,
			wantReadiness: runtimecontract.RuntimeValidationSurfaceUnavailable,
			wantCode:      "mcp_semantic_proof_unavailable",
		},
		{
			name:          "owner-managed capability is unavailable",
			effect:        runtimecontract.RuntimeEffectEphemeralSession,
			idempotency:   runtimecontract.RuntimeOperationIdempotencyOwnerManaged,
			schema:        mcpBehaviorProbeSchema(t, nil),
			endpoint:      server.URL,
			wantStatus:    runtimecontract.RuntimeSemanticFeasibilityInfeasible,
			wantReadiness: runtimecontract.RuntimeValidationSurfaceUnavailable,
			wantCode:      "mcp_semantic_proof_unavailable",
		},
		{
			name:          "non replayable capability is unavailable",
			effect:        runtimecontract.RuntimeEffectNone,
			idempotency:   runtimecontract.RuntimeOperationIdempotencyAtMostOnce,
			schema:        mcpBehaviorProbeSchema(t, nil),
			endpoint:      server.URL,
			wantStatus:    runtimecontract.RuntimeSemanticFeasibilityInfeasible,
			wantReadiness: runtimecontract.RuntimeValidationSurfaceUnavailable,
			wantCode:      "mcp_semantic_proof_unavailable",
		},
		{
			name:          "required schema input is unavailable",
			effect:        runtimecontract.RuntimeEffectNone,
			idempotency:   runtimecontract.RuntimeOperationIdempotencySafeToReplay,
			schema:        mcpBehaviorProbeSchema(t, []string{"path"}),
			endpoint:      server.URL,
			wantStatus:    runtimecontract.RuntimeSemanticFeasibilityPrerequisiteRequired,
			wantReadiness: runtimecontract.RuntimeValidationSurfacePrerequisiteRequired,
			wantCode:      "mcp_validation_input_binding_required",
		},
		{
			name:          "malformed schema is unavailable",
			effect:        runtimecontract.RuntimeEffectNone,
			idempotency:   runtimecontract.RuntimeOperationIdempotencySafeToReplay,
			schema:        workorcheventpayload.New(workorcheventpayload.StringField("type", "object"), workorcheventpayload.StringField("required", "path")),
			endpoint:      server.URL,
			wantStatus:    runtimecontract.RuntimeSemanticFeasibilityInfeasible,
			wantReadiness: runtimecontract.RuntimeValidationSurfaceUnavailable,
			wantCode:      "mcp_semantic_proof_unavailable",
		},
		{
			name:          "wrong root schema type is unavailable",
			effect:        runtimecontract.RuntimeEffectNone,
			idempotency:   runtimecontract.RuntimeOperationIdempotencySafeToReplay,
			schema:        workorcheventpayload.New(workorcheventpayload.StringField("type", "string")),
			endpoint:      server.URL,
			wantStatus:    runtimecontract.RuntimeSemanticFeasibilityInfeasible,
			wantReadiness: runtimecontract.RuntimeValidationSurfaceUnavailable,
			wantCode:      "mcp_semantic_proof_unavailable",
		},
		{
			name:          "nested required schema is unavailable",
			effect:        runtimecontract.RuntimeEffectNone,
			idempotency:   runtimecontract.RuntimeOperationIdempotencySafeToReplay,
			schema:        mcpBehaviorProbeNestedRequiredSchema(t),
			endpoint:      server.URL,
			wantStatus:    runtimecontract.RuntimeSemanticFeasibilityPrerequisiteRequired,
			wantReadiness: runtimecontract.RuntimeValidationSurfacePrerequisiteRequired,
			wantCode:      "mcp_validation_input_binding_required",
		},
		{
			name:          "malformed combinator is unavailable",
			effect:        runtimecontract.RuntimeEffectNone,
			idempotency:   runtimecontract.RuntimeOperationIdempotencySafeToReplay,
			schema:        workorcheventpayload.New(workorcheventpayload.StringField("type", "object"), workorcheventpayload.StringField("anyOf", "not-an-array")),
			endpoint:      server.URL,
			wantStatus:    runtimecontract.RuntimeSemanticFeasibilityInfeasible,
			wantReadiness: runtimecontract.RuntimeValidationSurfaceUnavailable,
			wantCode:      "mcp_semantic_proof_unavailable",
		},
		{
			name:          "malformed additional properties is unavailable",
			effect:        runtimecontract.RuntimeEffectNone,
			idempotency:   runtimecontract.RuntimeOperationIdempotencySafeToReplay,
			schema:        workorcheventpayload.New(workorcheventpayload.StringField("type", "object"), workorcheventpayload.StringField("additionalProperties", "not-a-schema")),
			endpoint:      server.URL,
			wantStatus:    runtimecontract.RuntimeSemanticFeasibilityInfeasible,
			wantReadiness: runtimecontract.RuntimeValidationSurfaceUnavailable,
			wantCode:      "mcp_semantic_proof_unavailable",
		},
		{
			name:          "empty combinator is unavailable",
			effect:        runtimecontract.RuntimeEffectNone,
			idempotency:   runtimecontract.RuntimeOperationIdempotencySafeToReplay,
			schema:        workorcheventpayload.New(workorcheventpayload.StringField("type", "object"), workorcheventpayload.Field{Name: "oneOf", Value: workorcheventpayload.Value{Kind: workorcheventpayload.ValueArray}}),
			endpoint:      server.URL,
			wantStatus:    runtimecontract.RuntimeSemanticFeasibilityInfeasible,
			wantReadiness: runtimecontract.RuntimeValidationSurfaceUnavailable,
			wantCode:      "mcp_semantic_proof_unavailable",
		},
		{
			name:             "remote endpoint behavior is not sampled during feasibility",
			effect:           runtimecontract.RuntimeEffectNone,
			idempotency:      runtimecontract.RuntimeOperationIdempotencySafeToReplay,
			schema:           mcpBehaviorProbeSchema(t, nil),
			endpoint:         server.URL + "/fail",
			wantStatus:       runtimecontract.RuntimeSemanticFeasibilityFeasible,
			wantReadiness:    runtimecontract.RuntimeValidationSurfaceReady,
			wantEndpointCall: 0,
		},
		{
			name:          "preflight endpoint failure is unavailable",
			effect:        runtimecontract.RuntimeEffectNone,
			idempotency:   runtimecontract.RuntimeOperationIdempotencySafeToReplay,
			schema:        mcpBehaviorProbeSchema(t, nil),
			endpoint:      "://invalid-endpoint",
			wantStatus:    runtimecontract.RuntimeSemanticFeasibilityInfeasible,
			wantReadiness: runtimecontract.RuntimeValidationSurfaceReady,
			wantCode:      "mcp_semantic_proof_unavailable",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			beforeCalls := endpointCalls.Load()
			getter := &semanticProbeMCPGetter{server: &mcpmodel.Server{
				ID: common.MCPServerID("mcp-semantic-probe"), Name: "semantic probe", Transport: "http",
				Endpoint: tc.endpoint, Enabled: true, AllowedTools: []string{"lsp_status"}, TimeoutMillis: 1000,
			}, capability: &mcpmodel.Capability{
				ID: "mcp-semantic-probe:tool:lsp_status", ServerID: "mcp-semantic-probe", Kind: mcpmodel.CapabilityKindTool,
				Name: "lsp_status", EffectScope: string(tc.effect), IdempotencyClass: string(tc.idempotency), Enabled: true,
				ValidationProofApplicability: string(runtimecontract.RuntimeValidationProofDirect),
				Schema:                       tc.schema,
			}}
			adapter := NewRuntimeAdapter(getter)
			snapshot, snapshotErr := adapter.runtimeOperationAdmissibilitySnapshot(ctx)
			if snapshotErr != nil || len(snapshot.ValidationSurfaces) != 1 {
				t.Fatalf("semantic validation catalog = %+v err=%v", snapshot, snapshotErr)
			}
			surface := snapshot.ValidationSurfaces[0]
			if surface.Readiness != tc.wantReadiness {
				t.Fatalf("validation surface readiness = %q, want %q (surface=%+v)", surface.Readiness, tc.wantReadiness, surface)
			}
			if tc.wantReadiness != runtimecontract.RuntimeValidationSurfaceReady {
				if len(snapshot.ValidationRecipes) != 0 {
					t.Fatalf("non-ready validation surface leaked provider recipe refs: %v", snapshot.ValidationRecipes)
				}
				if calls := endpointCalls.Load() - beforeCalls; calls != 0 {
					t.Fatalf("catalog classification called MCP endpoint %d times", calls)
				}
				return
			}
			hub := magicevent.NewHub(16)
			t.Cleanup(func() { hub.Terminate(ctx) })
			owner := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: adapter}
			if err := owner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { owner.Teardown(ctx) })

			var receiptQueries, receiptCommits atomic.Int32
			observer := magicevent.NewSimpleObserver("mcp-semantic-probe-test", hub)
			observer.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, func(_ magicevent.Event, result magicevent.Result) {
				receiptQueries.Add(1)
				result.Set(orchestratorevents.QueryRuntimeOperationReceiptResult{Found: false}, nil)
			})
			observer.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, func(_ magicevent.Event, result magicevent.Result) {
				receiptCommits.Add(1)
				result.Set(orchestratorevents.CommitRuntimeOperationReceiptResult{}, nil)
			})
			t.Cleanup(func() {
				observer.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
				observer.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
			})

			command := semanticProbeCommand(t, ctx, adapter, tc.evidenceKind)
			response := hub.Send(magicevent.NewEventWithContext(
				mcpevents.TopicProbeRuntimeSemanticProofFeasibility,
				runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, command,
			))
			if response == nil || response.Error() != nil {
				t.Fatalf("semantic behavior probe response error: %v (result=%v)", response, func() error {
					if response == nil {
						return nil
					}
					return response.Error()
				}())
			}
			data, err := response.Get()
			if err != nil {
				t.Fatal(err)
			}
			result, ok := data.(mcpevents.ProbeRuntimeSemanticProofFeasibilityResult)
			if !ok {
				t.Fatalf("semantic behavior probe result type = %T", data)
			}
			if got := result.Observation.Status; got != tc.wantStatus {
				t.Fatalf("semantic behavior probe status = %q, want %q (observation=%+v)", got, tc.wantStatus, result.Observation)
			}
			if tc.wantCode == "" {
				if result.Observation.OriginalDiagnostic != nil {
					t.Fatalf("feasible probe returned diagnostic: %+v", result.Observation.OriginalDiagnostic)
				}
				if result.FeasibilityBatchID != command.FeasibilityBatchID || result.Observation.ValidationRecipeID != command.Proposal.ValidationRecipeID ||
					!reflect.DeepEqual(result.Observation.RequiredCheckRefs, []string{command.Proposal.ValidationRecipeID}) {
					t.Fatalf("feasible probe lost the owner-confirmed recipe/check identity: %+v", result)
				}
				if got, want := result.Observation.TargetRefs, []string(nil); !reflect.DeepEqual(got, want) {
					t.Fatalf("feasible probe target refs = %#v, want %#v", got, want)
				}
			} else if result.Observation.OriginalDiagnostic == nil || result.Observation.OriginalDiagnostic.OriginalCode != tc.wantCode {
				t.Fatalf("unavailable probe diagnostic = %+v, want code %q", result.Observation.OriginalDiagnostic, tc.wantCode)
			}
			calls := endpointCalls.Load() - beforeCalls
			if calls != tc.wantEndpointCall {
				t.Fatalf("MCP endpoint calls = %d, want %d", calls, tc.wantEndpointCall)
			}
			if got := receiptQueries.Load(); got != 0 {
				t.Fatalf("semantic probe queried %d operation receipts; behavior probe must not enter receipt path", got)
			}
			if got := receiptCommits.Load(); got != 0 {
				t.Fatalf("semantic probe committed %d operation receipts; behavior probe must not mutate Runtime", got)
			}
		})
	}
}

func TestMCPOwnerRejectsLegacyRequiredInputSemanticRecipeWithoutEndpointOrReceipt(t *testing.T) {
	ctx := context.Background()
	var endpointCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		endpointCalls.Add(1)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(server.Close)
	getter := &semanticProbeMCPGetter{server: &mcpmodel.Server{
		ID: common.MCPServerID("mcp-semantic-probe"), Name: "semantic probe", Transport: "http",
		Endpoint: server.URL, Enabled: true, AllowedTools: []string{"lsp_definition"}, TimeoutMillis: 1000,
	}, capability: &mcpmodel.Capability{
		ID: "mcp-semantic-probe:tool:lsp_definition", ServerID: "mcp-semantic-probe", Kind: mcpmodel.CapabilityKindTool,
		Name: "lsp_definition", EffectScope: string(runtimecontract.RuntimeEffectNone),
		IdempotencyClass: string(runtimecontract.RuntimeOperationIdempotencySafeToReplay), Enabled: true,
		ValidationProofApplicability: string(runtimecontract.RuntimeValidationProofDirect),
		Schema:                       mcpBehaviorProbeSchema(t, []string{"path", "line", "col"}),
	}}
	adapter := NewRuntimeAdapter(getter)
	snapshot, err := adapter.runtimeOperationAdmissibilitySnapshot(ctx)
	if err != nil || len(snapshot.ValidationRecipes) != 0 || len(snapshot.ValidationSurfaces) != 1 ||
		snapshot.ValidationSurfaces[0].Readiness != runtimecontract.RuntimeValidationSurfacePrerequisiteRequired {
		t.Fatalf("required-input catalog = %+v err=%v", snapshot, err)
	}

	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: adapter}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })

	var receiptQueries, receiptCommits atomic.Int32
	observer := magicevent.NewSimpleObserver("mcp-required-input-probe-test", hub)
	observer.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, func(_ magicevent.Event, result magicevent.Result) {
		receiptQueries.Add(1)
		result.Set(orchestratorevents.QueryRuntimeOperationReceiptResult{Found: false}, nil)
	})
	observer.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, func(_ magicevent.Event, result magicevent.Result) {
		receiptCommits.Add(1)
		result.Set(orchestratorevents.CommitRuntimeOperationReceiptResult{}, nil)
	})
	t.Cleanup(func() {
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
	})

	recipeID := "mcp/tool_call/mcp-semantic-probe/lsp_definition"
	// Simulate a pre-closure caller catalog that still advertised the recipe.
	// The current MCP owner must classify it from its own latest snapshot and
	// fail closed before invoking the endpoint or the operation receipt path.
	command := semanticProbeCommandFromSnapshot(t, snapshot, recipeID, nil, runtimecontract.EvidenceKindValidation)
	response := hub.Send(magicevent.NewEventWithContext(
		mcpevents.TopicProbeRuntimeSemanticProofFeasibility,
		runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, command,
	))
	if response == nil || response.Error() != nil {
		t.Fatalf("required-input semantic probe response = %v err=%v", response, func() error {
			if response == nil {
				return nil
			}
			return response.Error()
		}())
	}
	data, getErr := response.Get()
	if getErr != nil {
		t.Fatal(getErr)
	}
	result, ok := data.(mcpevents.ProbeRuntimeSemanticProofFeasibilityResult)
	if !ok || result.Observation.Status != runtimecontract.RuntimeSemanticFeasibilityPrerequisiteRequired ||
		result.Observation.OriginalDiagnostic == nil || result.Observation.OriginalDiagnostic.OriginalCode != "mcp_validation_input_binding_required" {
		t.Fatalf("required-input semantic observation = %#v", data)
	}
	if endpointCalls.Load() != 0 || receiptQueries.Load() != 0 || receiptCommits.Load() != 0 {
		t.Fatalf("required-input probe side effects = endpoint:%d receipt-query:%d receipt-commit:%d", endpointCalls.Load(), receiptQueries.Load(), receiptCommits.Load())
	}
}

func mcpBehaviorProbeSchema(t *testing.T, required []string) workorcheventpayload.Payload {
	t.Helper()
	fields := []workorcheventpayload.Field{workorcheventpayload.StringField("type", "object")}
	if required != nil {
		fields = append(fields, workorcheventpayload.StringArrayField("required", required))
	}
	return workorcheventpayload.New(fields...)
}

func mcpBehaviorProbeNestedRequiredSchema(t *testing.T) workorcheventpayload.Payload {
	t.Helper()
	nested := workorcheventpayload.Value{Kind: workorcheventpayload.ValueObject, Object: []workorcheventpayload.Field{
		workorcheventpayload.StringField("type", "object"),
		workorcheventpayload.StringArrayField("required", []string{"path"}),
	}}
	return workorcheventpayload.New(
		workorcheventpayload.StringField("type", "object"),
		workorcheventpayload.Field{Name: "anyOf", Value: workorcheventpayload.Value{Kind: workorcheventpayload.ValueArray, Array: []workorcheventpayload.Value{nested}}},
	)
}

func semanticProbeCommand(t *testing.T, ctx context.Context, adapter *RuntimeAdapter, evidenceKind string) mcpevents.ProbeRuntimeSemanticProofFeasibilityCommand {
	t.Helper()
	snapshot, err := adapter.runtimeOperationAdmissibilitySnapshot(ctx)
	if err != nil || len(snapshot.ValidationRecipes) != 1 || len(snapshot.ValidationSurfaces) != 1 || len(snapshot.Capabilities) != 1 {
		t.Fatalf("probe catalog = %+v err=%v", snapshot, err)
	}
	return semanticProbeCommandFromSnapshot(t, snapshot, snapshot.ValidationRecipes[0], snapshot.ValidationSurfaces, evidenceKind)
}

func semanticProbeCommandFromSnapshot(t *testing.T, snapshot runtimeMCPAdmissibilitySnapshot, recipeID string, surfaces []runtimecontract.RuntimeValidationSurface, evidenceKind string) mcpevents.ProbeRuntimeSemanticProofFeasibilityCommand {
	t.Helper()
	serverID, _, err := parseMCPValidationRecipe(recipeID)
	if err != nil || len(snapshot.Capabilities) != 1 {
		t.Fatalf("probe recipe/capability catalog = recipe=%q capabilities=%#v err=%v", recipeID, snapshot.Capabilities, err)
	}
	capabilities := runtimeMCPCapabilityContexts(snapshot.Capabilities)
	if len(capabilities) != 1 {
		t.Fatalf("probe capability contexts = %#v", capabilities)
	}
	capability := capabilities[0]
	sealedEffect := runtimecontract.NormalizeRuntimeEffectScope(runtimecontract.RuntimeEffectScope(capability.EffectScope))
	if evidenceKind == "" {
		evidenceKind = runtimecontract.EvidenceKindValidation
	}
	catalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "mcp-semantic-probe-catalog", OwnerModuleID: ModuleID,
		OperationKinds: []string{runtimecontract.StepKindMCPToolCall}, SelectorRefs: []string{serverID},
		ValidationRecipeRefs: []string{recipeID}, ValidationSurfaces: surfaces,
		MCPCapabilities: capabilities, ObservedRevision: snapshot.Revision,
	}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	manifest := runtimecontract.RuntimeSemanticPlanManifest{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanManifestSchemaCurrent,
		WorkspaceID:   "ws-mcp-semantic-probe", RunID: "run-mcp-semantic-probe", DefinitionRevision: "definition-mcp-semantic-probe", DefinitionSeedHash: "seed-mcp-semantic-probe",
		OperationContractRevision: runtimecontract.RuntimeOperationContractRevision(),
		CoverageItems:             []runtimecontract.RuntimeCoverageItem{{ID: "cwi-mcp-semantic-probe", SliceID: "slice-mcp-semantic-probe", SourceScope: "dodItem", SourceID: "dod-mcp-semantic-probe", TargetText: "probe", Order: 1}},
		AllowedOwnerModuleIDs:     []string{ModuleID}, AllowedOperationKinds: []string{runtimecontract.StepKindMCPToolCall}, AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope}, OwnerCatalogs: []runtimecontract.RuntimeOperationAdmissibility{catalog},
	}
	manifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(manifest)
	proposal := runtimecontract.RuntimeSemanticProofProposal{
		CoverageWorkItemID: "cwi-mcp-semantic-probe", OwnerModuleID: ModuleID, OperationKind: runtimecontract.StepKindMCPToolCall,
		Target:                runtimecontract.RuntimeOperationTargetAuthorization{Policy: runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope, OwnerSelectorRefs: []string{serverID}},
		RequiredEvidenceKinds: []string{evidenceKind}, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{sealedEffect}, ValidationRecipeID: recipeID, DeclarationRef: "dod/cwi-mcp-semantic-probe",
	}
	return mcpevents.ProbeRuntimeSemanticProofFeasibilityCommand(runtimecontract.RuntimeSemanticProofFeasibilityRequest{
		WorkspaceID: manifest.WorkspaceID, RunID: manifest.RunID, FeasibilityBatchID: "batch-mcp-semantic-probe", WorkspaceDeliverableDirectory: t.TempDir(), Manifest: manifest, Proposal: proposal, OwnerCatalog: catalog,
	})
}

type semanticProbeMCPGetter struct {
	server     *mcpmodel.Server
	capability *mcpmodel.Capability
}

func (g *semanticProbeMCPGetter) GetServer(_ context.Context, id common.MCPServerID) (*mcpmodel.Server, error) {
	if g == nil || g.server == nil || g.server.ID != id {
		return nil, common.ErrNotFound
	}
	server := *g.server
	server.Args = append([]string(nil), g.server.Args...)
	server.AllowedTools = append([]string(nil), g.server.AllowedTools...)
	return &server, nil
}

func (g *semanticProbeMCPGetter) ListServers(context.Context) ([]*mcpmodel.Server, error) {
	if g == nil || g.server == nil {
		return nil, common.ErrNotFound
	}
	server, err := g.GetServer(context.Background(), g.server.ID)
	if err != nil {
		return nil, err
	}
	return []*mcpmodel.Server{server}, nil
}

func (g *semanticProbeMCPGetter) ListCapabilities(context.Context, registryevents.CapabilityQuery) ([]*mcpmodel.Capability, error) {
	if g == nil || g.capability == nil {
		return nil, errors.New("capability catalog unavailable")
	}
	return []*mcpmodel.Capability{mcpmodel.CloneCapability(g.capability)}, nil
}

func (g *semanticProbeMCPGetter) RegistrySnapshot(ctx context.Context) (*mcpmodel.RegistrySnapshot, string, error) {
	servers, err := g.ListServers(ctx)
	if err != nil {
		return nil, "", err
	}
	capabilities, err := g.ListCapabilities(ctx, registryevents.CapabilityQuery{})
	if err != nil {
		return nil, "", err
	}
	return testMCPRegistrySnapshot(servers, capabilities)
}
