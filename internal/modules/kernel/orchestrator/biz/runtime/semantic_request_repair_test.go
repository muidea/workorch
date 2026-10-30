package runtime

import (
	"context"
	"encoding/json"
	magicevent "github.com/muidea/magicCommon/event"
	decisionbiz "github.com/muidea/workorch/internal/modules/blocks/decision/biz"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"reflect"
	"testing"

	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func legacyWorkRequestRepairFixture(t *testing.T) (*RuntimeController, *implementationWorkDetachedRepository, RuntimeAggregate, runtimecontract.RuntimeSemanticRequestRepairProof) {
	t.Helper()
	a := implementationWorkPrivatePipeline(t, runtimecontract.RuntimeImplementationWorkPurposeEffect, []string{"target.txt"}, nil, true)
	repo := &implementationWorkDetachedRepository{controllerRepository: controllerRepository{aggregate: &a}}
	controller, err := NewRuntimeController(repo)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := SemanticExpectedDocument(a)
	if err != nil {
		t.Fatal(err)
	}
	expected.SemanticRenderContractRef = ""
	expected.MaxOutputTokens = 4096
	expected.MaxRecoveryAttempts = 1
	expected.Hash = runtimecontract.StableRuntimeExpectedDocumentHash(expected)
	diagnostic := runtimecontract.RuntimeOriginalDiagnostic{SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: runtimecontract.RuntimeModuleLLM, OriginalClass: runtimecontract.RuntimeDefinitionGapResultKind, OriginalCode: string(runtimecontract.RuntimeDefinitionGapSemanticAmbiguity), OriginalMessage: "immutable synthetic diagnostic: conflicting request field contracts", Signature: "synthetic-original-gap", TargetRefs: []string{string(expected.BatchCoverageWorkItemIDs[0])}}
	observation := runtimecontract.RuntimeDefinitionGapObservation{SchemaVersion: runtimecontract.RuntimeDefinitionClarificationSchemaCurrent, WorkspaceID: a.Facts.WorkspaceID, RunID: a.Facts.RunID, DefinitionRevision: a.Definition.Revision, StartupInputHash: a.Definition.StartupInputHash, AuthoritySnapshotID: a.Decision.AuthoritySnapshot.ID, FocusRef: a.Facts.Focus.ID, LogicalRequestID: a.SemanticPlanning.LogicalRequestID, SemanticDocumentHash: "synthetic-complete-document", GapKind: runtimecontract.RuntimeDefinitionGapSemanticAmbiguity, OffendingItemRefs: []string{string(expected.BatchCoverageWorkItemIDs[0])}, OriginalDiagnostic: diagnostic}
	observation.ObservationID = runtimecontract.StableRuntimeDefinitionGapObservationID(observation)
	session := runtimecontract.RuntimeCandidateRequestSession{LogicalRequestID: expected.LogicalRequestID, DocumentKind: expected.DocumentKind, ExpectedDocument: &expected, Transaction: a.Decision.AuthorityTransaction(), StablePrefixHash: expected.StablePrefixHash, ManifestHash: expected.ManifestHash, State: runtimecontract.RuntimeCandidateRequestSessionDocumentComplete, NextSequence: 1, LastPhysicalCallID: "legacy-proof-call", ProviderSubmitted: true, LastOutcome: runtimecontract.RuntimeCandidateRequestOutcome{Class: runtimecontract.RuntimeCandidateRequestOutcomeDocumentComplete}}
	repo.aggregate.CandidateRequestSessions = append(repo.aggregate.CandidateRequestSessions, session)
	scope := RuntimeInternalEventScope{WorkspaceID: a.Facts.WorkspaceID, RunID: a.Facts.RunID}
	updated, err := controller.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{Kind: RuntimeInternalEventDefinitionGapObserved, Scope: scope, Authority: &RuntimeInternalAuthorityPayload{DefinitionGap: &observation}})
	if err != nil {
		t.Fatal(err)
	}
	a = updated.Aggregate
	updated, err = controller.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{Kind: RuntimeInternalEventDefinitionClarificationDispatchSealed, Scope: scope, Transaction: &RuntimeInternalTransactionPayload{}})
	if err != nil {
		t.Fatal(err)
	}
	a = updated.Aggregate
	// Compile the exact Decision opening binding from the persisted dispatch.
	p := runtimecontract.RuntimeSemanticRequestRepairProof{Dispatch: *a.DefinitionClarification.Dispatch}
	original := (decisionevents.RepairRuntimeDefinitionClarificationRequestContractCommand{Proof: p}).OriginalRequest()
	original.TicketID = decisionevents.StableRuntimeDefinitionClarificationTicketID(original)
	record := runtimecontract.RuntimeDefinitionClarificationRequestRecord{DispatchID: original.ClarificationDispatchID, TicketID: original.TicketID, RequestBindingHash: original.BindingHash()}
	updated, err = controller.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{Kind: RuntimeInternalEventDefinitionClarificationRequestObserved, Scope: scope, Transaction: &RuntimeInternalTransactionPayload{DefinitionClarificationRequest: &record}})
	if err != nil {
		t.Fatal(err)
	}
	a = updated.Aggregate
	proof, err := CompileRuntimeSemanticRequestRepairProof(a)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Validate(); err != nil {
		t.Fatal(err)
	}
	return controller, repo, a, proof
}

func TestSemanticRequestContractRepairCASRestartAndExactReplay(t *testing.T) {
	controller, repo, before, proof := legacyWorkRequestRepairFixture(t)
	scope := RuntimeInternalEventScope{WorkspaceID: before.Facts.WorkspaceID, RunID: before.Facts.RunID}
	preparedEvent := RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticRequestRepairPrepared, Scope: scope, Authority: &RuntimeInternalAuthorityPayload{RequestContractRepairProof: &proof}}
	prepared, err := controller.ApplyInternalEvent(context.Background(), preparedEvent)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Aggregate.StateRevision != before.StateRevision+1 || prepared.Aggregate.DefinitionClarification.RequestContractRepair == nil {
		t.Fatal("repair intent was not sealed before owner I/O")
	}
	saves := repo.saves
	if _, err = controller.ApplyInternalEvent(context.Background(), preparedEvent); err != nil || repo.saves != saves {
		t.Fatal("duplicate preparation changed state")
	}
	// An actual serialize/reload simulates the restart cut before owner reply.
	raw, _ := json.Marshal(repo.aggregate)
	var loaded RuntimeAggregate
	if err := json.Unmarshal(raw, &loaded); err != nil {
		t.Fatal(err)
	}
	repo.aggregate = &loaded
	controller, err = NewRuntimeController(repo)
	if err != nil {
		t.Fatal(err)
	}
	resolutionID := decisionevents.StableDefinitionClarificationResolutionID(proof.Request.TicketID, proof.Hash)
	resolution := runtimecontract.RuntimeSemanticRequestRepairResolution{ProofHash: proof.Hash, TicketID: proof.Request.TicketID, ResolutionID: resolutionID, BindingHash: runtimecontract.StableRuntimeSemanticRequestRepairResolutionHash(proof.Hash, proof.Request.TicketID, resolutionID)}
	lease := runtimecontract.RuntimeDriveLease{WorkspaceID: scope.WorkspaceID, RunID: scope.RunID, Reason: runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry, FailureSignature: SemanticRequestRepairDriveLeaseSignature(proof, resolution), DueAt: "2026-10-30T00:00:00Z"}
	resolvedEvent := RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticRequestRepairResolved, Scope: scope, Authority: &RuntimeInternalAuthorityPayload{RequestContractRepairProof: &proof, RequestContractRepairResolution: &resolution, RequestContractRepairDriveLease: &lease}}
	recovered, err := controller.ApplyInternalEvent(context.Background(), resolvedEvent)
	if err != nil {
		t.Fatal(err)
	}
	a := recovered.Aggregate
	if a.DefinitionClarification != nil || a.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningRequired || a.StateRevision != before.StateRevision+2 {
		t.Fatal("exact repair did not retire old generation")
	}
	if !reflect.DeepEqual(before.ExecutionBudget, a.ExecutionBudget) || !reflect.DeepEqual(before.Definition, a.Definition) || !reflect.DeepEqual(before.Facts.DefinitionFocuses, a.Facts.DefinitionFocuses) || !reflect.DeepEqual(before.ImplementationWork, a.ImplementationWork) || !reflect.DeepEqual(before.OperationReceipts, a.OperationReceipts) || !reflect.DeepEqual(before.CompletedDispatches, a.CompletedDispatches) || !reflect.DeepEqual(before.Facts.Evidence, a.Facts.Evidence) || !reflect.DeepEqual(before.Facts.Coverage, a.Facts.Coverage) {
		t.Fatal("repair changed immutable criteria, effect, budget or credit")
	}
	oldIdentity, _ := SemanticRequestIdentityHash(before.SemanticPlanning)
	newIdentity, _ := SemanticRequestIdentityHash(a.SemanticPlanning)
	if oldIdentity == newIdentity {
		t.Fatal("new rendering reused retired request identity")
	}
	saves = repo.saves
	if _, err := controller.ApplyInternalEvent(context.Background(), resolvedEvent); err != nil || repo.saves != saves {
		t.Fatal("duplicate owner reply changed state")
	}
	raw, _ = json.Marshal(a)
	if err := json.Unmarshal(raw, &loaded); err != nil || loaded.Validate() != nil {
		t.Fatalf("recovered source did not survive restart: %v", err)
	}
	for _, mutate := range []func(*RuntimeAggregate){
		func(v *RuntimeAggregate) {
			v.SemanticPlanning.History[len(v.SemanticPlanning.History)-1].RequestContractRepair.Proof.Observation.OriginalDiagnostic.OriginalMessage = "rewritten"
		},
		func(v *RuntimeAggregate) {
			v.CandidateRequestSessions[len(v.CandidateRequestSessions)-1].ExpectedDocument.SemanticRenderContractRef = runtimecontract.RuntimeSemanticRenderContractV2
		},
		func(v *RuntimeAggregate) {
			v.SemanticPlanning.History[len(v.SemanticPlanning.History)-1].RequestContractRepair.Resolution.BindingHash = "forged"
		},
	} {
		copyRaw, _ := json.Marshal(a)
		var changed RuntimeAggregate
		_ = json.Unmarshal(copyRaw, &changed)
		mutate(&changed)
		if changed.Validate() == nil {
			t.Fatal("tampered source survived complete reload")
		}
	}
}

func TestSemanticRequestContractRepairRejectsDriftBeforeOwnerWrite(t *testing.T) {
	_, _, a, proof := legacyWorkRequestRepairFixture(t)
	for name, mutate := range map[string]func(*RuntimeAggregate){
		"workspace":           func(v *RuntimeAggregate) { v.Facts.WorkspaceID = "foreign" },
		"round already bound": func(v *RuntimeAggregate) { v.ImplementationWork.Rounds[0].DispatchID = "effect" },
		"startup":             func(v *RuntimeAggregate) { v.Definition.StartupInputHash = "foreign" },
		"current version": func(v *RuntimeAggregate) {
			v.CandidateRequestSessions[len(v.CandidateRequestSessions)-1].ExpectedDocument.SemanticRenderContractRef = runtimecontract.RuntimeSemanticRenderContractV2
		},
		"mixed diagnostic": func(v *RuntimeAggregate) {
			v.DefinitionClarification.Observation.OffendingItemRefs = append(v.DefinitionClarification.Observation.OffendingItemRefs, "other")
		},
		"exhausted provider": func(v *RuntimeAggregate) {
			for i := range v.ExecutionBudget.Policy.Dimensions {
				if v.ExecutionBudget.Policy.Dimensions[i].Dimension == runtimecontract.RuntimeBudgetDimensionProviderCall {
					v.ExecutionBudget.Policy.Dimensions[i].Limit = 0
				}
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			raw, _ := json.Marshal(a)
			var changed RuntimeAggregate
			_ = json.Unmarshal(raw, &changed)
			mutate(&changed)
			if _, err := CompileRuntimeSemanticRequestRepairProof(changed); err == nil {
				t.Fatal("unsafe repair compiled")
			}
		})
	}
	_ = proof
}

// The real Decision owner persists both its resolution and ticket lifecycle.
// A restart/replay may finish delivery but cannot issue evidence or a new effect.
func TestSemanticRequestContractRepairRealDecisionOwnerReplay(t *testing.T) {
	for _, orphan := range []bool{false, true} {
		t.Run(map[bool]string{false: "normal", true: "resolution-before-ticket-cut"}[orphan], func(t *testing.T) { testSemanticRequestContractRepairRealDecisionOwnerReplay(t, orphan) })
	}
}
func testSemanticRequestContractRepairRealDecisionOwnerReplay(t *testing.T, orphan bool) {
	_, _, _, proof := legacyWorkRequestRepairFixture(t)
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), proof.Manifest.WorkspaceID)
	hub := magicevent.NewHub(16)
	defer hub.Terminate(ctx)
	repositories := memory.NewDecisionOwnerRepositorySet()
	start := func() *decisionbiz.Biz {
		owner, err := decisionbiz.New(hub, nil, decisionbiz.Dependencies{EventHub: hub, OwnerRepositories: repositories.OwnerRepositories()})
		if err != nil {
			t.Fatal(err)
		}
		if err := owner.Start(); err != nil {
			t.Fatal(err)
		}
		return owner
	}
	owner := start()
	send := func(topic, source string, payload any) (any, error) {
		result := hub.Send(magicevent.NewEventWithContext(topic, source, decisionevents.CommandDestination, magicevent.NewHeader(), ctx, payload))
		if result == nil {
			return nil, common.ErrConflict
		}
		data, err := result.Get()
		if err != nil {
			return nil, err
		}
		return data, nil
	}
	cmd := decisionevents.RepairRuntimeDefinitionClarificationRequestContractCommand{WorkspaceID: proof.Manifest.WorkspaceID, RunID: proof.Manifest.RunID, Proof: proof}
	if err := cmd.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := send(decisionevents.TopicOpenRuntimeDefinitionClarification, "kernel/orchestrator", cmd.OriginalRequest()); err != nil {
		t.Fatal(err)
	}
	if _, err := send(decisionevents.TopicRepairRuntimeDefinitionClarificationRequestContract, "untrusted/module", cmd); err == nil {
		t.Fatal("foreign source repaired ticket")
	}
	if orphan {
		r := &decisionmodel.Resolution{ID: decisionevents.StableDefinitionClarificationResolutionID(proof.Request.TicketID, proof.Hash), TicketID: proof.Request.TicketID, Type: decisionevents.DefinitionClarificationResolutionRequestContractRepaired, ResolvedAt: "2026-10-30T01:00:00Z", Payload: workorcheventpayload.New(workorcheventpayload.StringField("requestContractRepairHash", proof.Hash))}
		if err := repositories.Resolutions.Create(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	data, err := send(decisionevents.TopicRepairRuntimeDefinitionClarificationRequestContract, "kernel/orchestrator", cmd)
	if err != nil {
		t.Fatal(err)
	}
	response, ok := data.(decisionevents.RepairRuntimeDefinitionClarificationRequestContractResult)
	if !ok || response.ValidateFor(cmd) != nil || response.Disposition != decisionevents.DefinitionClarificationResolved {
		t.Fatalf("invalid repair response: %+v", data)
	}
	stored, err := repositories.Tickets.Get(ctx, proof.Request.TicketID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != common.DecisionTicketStateResolved || stored.DefinitionClarification.RequestContractRepairHash != proof.Hash || stored.DefinitionClarification.ReplacementIntentID != "" {
		t.Fatal("repair mutated original criteria/replacement binding")
	}
	owner.Teardown(ctx)
	owner = start()
	defer owner.Teardown(ctx)
	data, err = send(decisionevents.TopicRepairRuntimeDefinitionClarificationRequestContract, "kernel/orchestrator", cmd)
	if err != nil {
		t.Fatal(err)
	}
	replay := data.(decisionevents.RepairRuntimeDefinitionClarificationRequestContractResult)
	if replay.ValidateFor(cmd) != nil || replay.Resolution != response.Resolution || replay.Disposition != decisionevents.DefinitionClarificationDuplicate {
		t.Fatal("restart lost exact owner resolution")
	}
	changed := cmd
	changed.WorkspaceID = "other"
	if _, err := send(decisionevents.TopicRepairRuntimeDefinitionClarificationRequestContract, "kernel/orchestrator", changed); err == nil {
		t.Fatal("cross-workspace repair accepted")
	}
}

func TestSemanticRequestContractRepairCASCutsPreserveSingleIntentAndSuccessor(t *testing.T) {
	for _, phase := range []string{"prepare", "resolve"} {
		for _, after := range []bool{false, true} {
			t.Run(phase+map[bool]string{false: "/before-CAS", true: "/reply-loss"}[after], func(t *testing.T) {
				controller, repo, before, proof := legacyWorkRequestRepairFixture(t)
				scope := RuntimeInternalEventScope{WorkspaceID: before.Facts.WorkspaceID, RunID: before.Facts.RunID}
				prepare := RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticRequestRepairPrepared, Scope: scope, Authority: &RuntimeInternalAuthorityPayload{RequestContractRepairProof: &proof}}
				resolutionID := decisionevents.StableDefinitionClarificationResolutionID(proof.Request.TicketID, proof.Hash)
				resolution := runtimecontract.RuntimeSemanticRequestRepairResolution{ProofHash: proof.Hash, TicketID: proof.Request.TicketID, ResolutionID: resolutionID, BindingHash: runtimecontract.StableRuntimeSemanticRequestRepairResolutionHash(proof.Hash, proof.Request.TicketID, resolutionID)}
				lease := runtimecontract.RuntimeDriveLease{WorkspaceID: scope.WorkspaceID, RunID: scope.RunID, Reason: runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry, FailureSignature: SemanticRequestRepairDriveLeaseSignature(proof, resolution), DueAt: "2026-10-30T00:00:00Z"}
				resolve := RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticRequestRepairResolved, Scope: scope, Authority: &RuntimeInternalAuthorityPayload{RequestContractRepairProof: &proof, RequestContractRepairResolution: &resolution, RequestContractRepairDriveLease: &lease}}
				event := prepare
				if phase == "resolve" {
					if _, err := controller.ApplyInternalEvent(context.Background(), prepare); err != nil {
						t.Fatal(err)
					}
					event = resolve
				}
				revision := repo.aggregate.StateRevision
				repo.failBefore, repo.failAfter = !after, after
				if _, err := controller.ApplyInternalEvent(context.Background(), event); err == nil {
					t.Fatal("fault not reached")
				}
				want := revision
				if after {
					want++
				}
				if repo.aggregate.StateRevision != want {
					t.Fatal("unexpected persisted cut")
				}
				raw, err := json.Marshal(repo.aggregate)
				if err != nil {
					t.Fatal(err)
				}
				var loaded RuntimeAggregate
				if err := json.Unmarshal(raw, &loaded); err != nil {
					t.Fatal(err)
				}
				repo.aggregate = &loaded
				repo.failBefore, repo.failAfter = false, false
				controller, err = NewRuntimeController(repo)
				if err != nil {
					t.Fatal(err)
				}
				result, err := controller.ApplyInternalEvent(context.Background(), event)
				if err != nil {
					t.Fatal(err)
				}
				if result.Aggregate.StateRevision != revision+1 {
					t.Fatal("retry changed original transaction")
				}
				if phase == "resolve" {
					if result.Aggregate.DriveLease == nil || result.Aggregate.DriveLease.FailureSignature != lease.FailureSignature || result.Aggregate.DriveLease.AggregateRevision != result.Aggregate.StateRevision {
						t.Fatal("resolved CAS lost atomic SM-4 successor")
					}
				}
				saves := repo.saves
				if _, err := controller.ApplyInternalEvent(context.Background(), event); err != nil || repo.saves != saves {
					t.Fatal("duplicate replay mutated state")
				}
			})
		}
	}
}

func TestSemanticRequestContractRepairDoesNotEraseUnknownOrExhaustedBudget(t *testing.T) {
	for _, mode := range []string{"reserved", "unknown", "exhausted"} {
		t.Run(mode, func(t *testing.T) {
			_, _, a, _ := legacyWorkRequestRepairFixture(t)
			ledger := a.ExecutionBudget.Clone()
			if mode == "exhausted" {
				for i := range ledger.Policy.Dimensions {
					if ledger.Policy.Dimensions[i].Dimension == runtimecontract.RuntimeBudgetDimensionProviderCall {
						ledger.Policy.Dimensions[i].Limit = 1
					}
				}
				ledger.Policy.PolicyHash = runtimecontract.StableRuntimeExecutionBudgetPolicyHash(ledger.Policy)
			}
			ledger, allocations, _, err := runtimecontract.ReserveRuntimeBudgetAllocations(ledger, []runtimecontract.RuntimeBudgetAllocationSpec{{Dimension: runtimecontract.RuntimeBudgetDimensionProviderCall, LogicalConsumptionID: "exact-old-call", ScopeRef: "physical-old-call", ReservedAmount: 1}})
			if err != nil {
				t.Fatal(err)
			}
			if mode != "reserved" {
				join := runtimecontract.RuntimeBudgetConsumptionJoin{AllocationID: allocations[0].AllocationID, Owner: runtimecontract.RuntimeModuleLLM, PhysicalConsumptionID: "physical-old-call", OwnerResultRef: "original-provider-audit", OwnerResultHash: "original-audit-hash"}
				if mode == "unknown" {
					join.UnknownAmount = 1
				} else {
					join.ConfirmedAmount = 1
				}
				ledger, _, _, err = runtimecontract.JoinRuntimeBudgetConsumption(ledger, join)
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := ledger.Validate(); err != nil {
				t.Fatal(err)
			}
			a.ExecutionBudget = ledger
			raw, _ := json.Marshal(a)
			if _, err := CompileRuntimeSemanticRequestRepairProof(a); err == nil {
				t.Fatal("unsettled or exhausted budget permitted recovery")
			}
			after, _ := json.Marshal(a)
			if string(raw) != string(after) {
				t.Fatal("failed admission changed original consumption")
			}
		})
	}
}

func TestSemanticRequestContractRepairEnvelopeRejectsMixedAuthorityWithoutCAS(t *testing.T) {
	controller, repo, a, proof := legacyWorkRequestRepairFixture(t)
	scope := RuntimeInternalEventScope{WorkspaceID: a.Facts.WorkspaceID, RunID: a.Facts.RunID}
	for name, event := range map[string]RuntimeInternalEvent{
		"missing proof":       {Kind: RuntimeInternalEventSemanticRequestRepairPrepared, Scope: scope, Authority: &RuntimeInternalAuthorityPayload{}},
		"mixed failure":       {Kind: RuntimeInternalEventSemanticRequestRepairPrepared, Scope: scope, Authority: &RuntimeInternalAuthorityPayload{RequestContractRepairProof: &proof, CloseActiveDispatch: true}},
		"missing owner reply": {Kind: RuntimeInternalEventSemanticRequestRepairResolved, Scope: scope, Authority: &RuntimeInternalAuthorityPayload{RequestContractRepairProof: &proof}},
		"cross workspace":     {Kind: RuntimeInternalEventSemanticRequestRepairPrepared, Scope: RuntimeInternalEventScope{WorkspaceID: "other", RunID: scope.RunID}, Authority: &RuntimeInternalAuthorityPayload{RequestContractRepairProof: &proof}},
		"unrelated event":     {Kind: RuntimeInternalEventSemanticContractRequired, Scope: scope, Authority: &RuntimeInternalAuthorityPayload{RequestContractRepairProof: &proof}},
	} {
		t.Run(name, func(t *testing.T) {
			saves := repo.saves
			if _, err := controller.ApplyInternalEvent(context.Background(), event); err == nil || repo.saves != saves {
				t.Fatal("mixed authority reached CAS")
			}
		})
	}
}
