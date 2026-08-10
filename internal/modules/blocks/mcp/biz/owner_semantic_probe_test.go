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

// RSO-25 owner behavior-probe matrix.  The probe is an owner-local fact
// acquisition: it may invoke only an explicitly non-durable, safe-to-replay
// tool and it must never enter the operation receipt/evidence path.
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
			wantEndpointCall: 1,
		},
		{
			name:             "safe ephemeral session replayable succeeds",
			effect:           runtimecontract.RuntimeEffectEphemeralSession,
			idempotency:      runtimecontract.RuntimeOperationIdempotencySafeToReplay,
			schema:           mcpBehaviorProbeSchema(t, nil),
			endpoint:         server.URL,
			wantStatus:       runtimecontract.RuntimeSemanticFeasibilityFeasible,
			wantEndpointCall: 1,
		},
		{
			name:         "durable capability is unavailable",
			effect:       runtimecontract.RuntimeEffectWorkspaceDurable,
			idempotency:  runtimecontract.RuntimeOperationIdempotencySafeToReplay,
			evidenceKind: runtimecontract.EvidenceKindStateChange,
			schema:       mcpBehaviorProbeSchema(t, nil),
			endpoint:     server.URL,
			wantStatus:   runtimecontract.RuntimeSemanticFeasibilityInfeasible,
			wantCode:     "mcp_semantic_proof_unavailable",
		},
		{
			name:        "owner-managed capability is unavailable",
			effect:      runtimecontract.RuntimeEffectEphemeralSession,
			idempotency: runtimecontract.RuntimeOperationIdempotencyOwnerManaged,
			schema:      mcpBehaviorProbeSchema(t, nil),
			endpoint:    server.URL,
			wantStatus:  runtimecontract.RuntimeSemanticFeasibilityInfeasible,
			wantCode:    "mcp_semantic_proof_unavailable",
		},
		{
			name:        "non replayable capability is unavailable",
			effect:      runtimecontract.RuntimeEffectNone,
			idempotency: runtimecontract.RuntimeOperationIdempotencyAtMostOnce,
			schema:      mcpBehaviorProbeSchema(t, nil),
			endpoint:    server.URL,
			wantStatus:  runtimecontract.RuntimeSemanticFeasibilityInfeasible,
			wantCode:    "mcp_semantic_proof_unavailable",
		},
		{
			name:        "required schema input is unavailable",
			effect:      runtimecontract.RuntimeEffectNone,
			idempotency: runtimecontract.RuntimeOperationIdempotencySafeToReplay,
			schema:      mcpBehaviorProbeSchema(t, []string{"path"}),
			endpoint:    server.URL,
			wantStatus:  runtimecontract.RuntimeSemanticFeasibilityInfeasible,
			wantCode:    "mcp_semantic_proof_unavailable",
		},
		{
			name:        "malformed schema is unavailable",
			effect:      runtimecontract.RuntimeEffectNone,
			idempotency: runtimecontract.RuntimeOperationIdempotencySafeToReplay,
			schema:      workorcheventpayload.New(workorcheventpayload.StringField("type", "object"), workorcheventpayload.StringField("required", "path")),
			endpoint:    server.URL,
			wantStatus:  runtimecontract.RuntimeSemanticFeasibilityInfeasible,
			wantCode:    "mcp_semantic_proof_unavailable",
		},
		{
			name:        "wrong root schema type is unavailable",
			effect:      runtimecontract.RuntimeEffectNone,
			idempotency: runtimecontract.RuntimeOperationIdempotencySafeToReplay,
			schema:      workorcheventpayload.New(workorcheventpayload.StringField("type", "string")),
			endpoint:    server.URL,
			wantStatus:  runtimecontract.RuntimeSemanticFeasibilityInfeasible,
			wantCode:    "mcp_semantic_proof_unavailable",
		},
		{
			name:        "nested required schema is unavailable",
			effect:      runtimecontract.RuntimeEffectNone,
			idempotency: runtimecontract.RuntimeOperationIdempotencySafeToReplay,
			schema:      mcpBehaviorProbeNestedRequiredSchema(t),
			endpoint:    server.URL,
			wantStatus:  runtimecontract.RuntimeSemanticFeasibilityInfeasible,
			wantCode:    "mcp_semantic_proof_unavailable",
		},
		{
			name:        "malformed combinator is unavailable",
			effect:      runtimecontract.RuntimeEffectNone,
			idempotency: runtimecontract.RuntimeOperationIdempotencySafeToReplay,
			schema:      workorcheventpayload.New(workorcheventpayload.StringField("type", "object"), workorcheventpayload.StringField("anyOf", "not-an-array")),
			endpoint:    server.URL,
			wantStatus:  runtimecontract.RuntimeSemanticFeasibilityInfeasible,
			wantCode:    "mcp_semantic_proof_unavailable",
		},
		{
			name:        "malformed additional properties is unavailable",
			effect:      runtimecontract.RuntimeEffectNone,
			idempotency: runtimecontract.RuntimeOperationIdempotencySafeToReplay,
			schema:      workorcheventpayload.New(workorcheventpayload.StringField("type", "object"), workorcheventpayload.StringField("additionalProperties", "not-a-schema")),
			endpoint:    server.URL,
			wantStatus:  runtimecontract.RuntimeSemanticFeasibilityInfeasible,
			wantCode:    "mcp_semantic_proof_unavailable",
		},
		{
			name:        "empty combinator is unavailable",
			effect:      runtimecontract.RuntimeEffectNone,
			idempotency: runtimecontract.RuntimeOperationIdempotencySafeToReplay,
			schema:      workorcheventpayload.New(workorcheventpayload.StringField("type", "object"), workorcheventpayload.Field{Name: "oneOf", Value: workorcheventpayload.Value{Kind: workorcheventpayload.ValueArray}}),
			endpoint:    server.URL,
			wantStatus:  runtimecontract.RuntimeSemanticFeasibilityInfeasible,
			wantCode:    "mcp_semantic_proof_unavailable",
		},
		{
			name:             "preflight execution failure is unavailable",
			effect:           runtimecontract.RuntimeEffectNone,
			idempotency:      runtimecontract.RuntimeOperationIdempotencySafeToReplay,
			schema:           mcpBehaviorProbeSchema(t, nil),
			endpoint:         server.URL + "/fail",
			wantStatus:       runtimecontract.RuntimeSemanticFeasibilityInfeasible,
			wantCode:         "mcp_semantic_proof_unavailable",
			wantEndpointCall: 1,
		},
		{
			name:        "preflight endpoint failure is unavailable",
			effect:      runtimecontract.RuntimeEffectNone,
			idempotency: runtimecontract.RuntimeOperationIdempotencySafeToReplay,
			schema:      mcpBehaviorProbeSchema(t, nil),
			endpoint:    "://invalid-endpoint",
			wantStatus:  runtimecontract.RuntimeSemanticFeasibilityInfeasible,
			wantCode:    "mcp_semantic_proof_unavailable",
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
				Schema: tc.schema,
			}}
			adapter := NewRuntimeAdapter(getter)
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
	_, recipes, revision, err := adapter.OperationAdmissibility(ctx)
	if err != nil || len(recipes) != 1 {
		t.Fatalf("probe catalog = recipes=%v revision=%q err=%v", recipes, revision, err)
	}
	serverID := "mcp-semantic-probe"
	tool := "lsp_status"
	capability := runtimecontract.RuntimeMCPCapabilityContext{
		ID: "mcp-semantic-probe:tool:lsp_status", ServerID: common.MCPServerID(serverID), Kind: mcpmodel.CapabilityKindTool,
		OperationKind: runtimecontract.StepKindMCPToolCall, Name: tool, EffectScope: string(runtimecontract.RuntimeEffectNone),
		IdempotencyClass: string(runtimecontract.RuntimeOperationIdempotencySafeToReplay), Enabled: true,
	}
	// The caller's adapter fixture may intentionally use unsafe metadata/schema;
	// recover those values from the catalog returned by the adapter so command
	// validation still represents the exact sealed owner fact.
	items, err := adapter.EffectiveCapabilities(ctx)
	if err != nil || len(items) != 1 {
		t.Fatalf("probe capability catalog = %#v err=%v", items, err)
	}
	capability.EffectScope = items[0].EffectScope
	capability.IdempotencyClass = items[0].IdempotencyClass
	sealedEffect := runtimecontract.NormalizeRuntimeEffectScope(runtimecontract.RuntimeEffectScope(capability.EffectScope))
	if evidenceKind == "" {
		evidenceKind = runtimecontract.EvidenceKindValidation
	}
	catalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "mcp-semantic-probe-catalog", OwnerModuleID: ModuleID,
		OperationKinds: []string{runtimecontract.StepKindMCPToolCall}, SelectorRefs: []string{serverID},
		ValidationRecipeRefs: recipes, MCPCapabilities: []runtimecontract.RuntimeMCPCapabilityContext{capability}, ObservedRevision: revision,
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
		RequiredEvidenceKinds: []string{evidenceKind}, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{sealedEffect}, ValidationRecipeID: recipes[0], DeclarationRef: "dod/cwi-mcp-semantic-probe",
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
