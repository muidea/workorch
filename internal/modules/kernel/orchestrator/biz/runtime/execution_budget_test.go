package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/llmaudit"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestCandidateProviderBudgetIsReservedBeforeSubmissionAndConfirmedWithOutcome(t *testing.T) {
	ctx := context.Background()
	aggregate := stateChangeAggregate(t)
	repository := &controllerRepository{aggregate: &aggregate}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	_, session, err := controller.ensureCandidateRequestSession(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, runtimecontract.RuntimeProviderDocumentOperations, "budget-prefix", "budget-manifest", "")
	if err != nil {
		t.Fatal(err)
	}
	physicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, 1)
	submitted, err := controller.markCandidateRequestSubmitted(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, 1, physicalCallID)
	if err != nil {
		t.Fatal(err)
	}
	view, err := runtimecontract.BuildRuntimeExecutionBudgetView(submitted.Facts.WorkspaceID, submitted.Facts.RunID, submitted.StateRevision, submitted.ExecutionBudget, "", "")
	if err != nil {
		t.Fatal(err)
	}
	assertRuntimeExecutionDimension(t, view, runtimecontract.RuntimeBudgetDimensionProviderCall, 0, 1, 0)
	settlement := runtimeProviderConfirmedBudgetSettlementForTest(t, session.LogicalRequestID, physicalCallID, &llmaudit.ProviderUsage{InputTokens: 11, OutputTokens: 7, TotalTokens: 18})
	completed, err := controller.planning.completeCandidateRequestDocument(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, physicalCallID, settlement)
	if err != nil {
		t.Fatal(err)
	}
	view, err = runtimecontract.BuildRuntimeExecutionBudgetView(completed.Facts.WorkspaceID, completed.Facts.RunID, completed.StateRevision, completed.ExecutionBudget, "", "")
	if err != nil {
		t.Fatal(err)
	}
	assertRuntimeExecutionDimension(t, view, runtimecontract.RuntimeBudgetDimensionProviderCall, 1, 0, 0)
	assertRuntimeExecutionDimension(t, view, runtimecontract.RuntimeBudgetDimensionProviderInputToken, 11, 0, 0)
	assertRuntimeExecutionDimension(t, view, runtimecontract.RuntimeBudgetDimensionProviderOutputToken, 7, 0, 0)
}

func TestCandidateProviderBudgetObservationMustMatchSubmittedPhysicalCall(t *testing.T) {
	ctx := context.Background()
	aggregate := stateChangeAggregate(t)
	repository := &controllerRepository{aggregate: &aggregate}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	_, session, err := controller.ensureCandidateRequestSession(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, runtimecontract.RuntimeProviderDocumentOperations, "budget-binding-prefix", "budget-binding-manifest", "")
	if err != nil {
		t.Fatal(err)
	}
	physicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, 1)
	if _, err := controller.markCandidateRequestSubmitted(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, 1, physicalCallID); err != nil {
		t.Fatal(err)
	}
	drifted := runtimeProviderConfirmedBudgetSettlementForTest(t, session.LogicalRequestID, "physical-other", nil)
	saves := repository.saves
	if _, err := controller.planning.completeCandidateRequestDocument(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, physicalCallID, drifted); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("drifted provider budget observation err=%v, want conflict", err)
	}
	stored := repository.aggregate.CandidateRequestSessions[0]
	if repository.saves != saves || !stored.ProviderSubmitted || stored.State == runtimecontract.RuntimeCandidateRequestSessionDocumentComplete {
		t.Fatalf("drifted observation changed request session: saves=%d/%d session=%+v", repository.saves, saves, stored)
	}
}

func TestProviderBudgetReleaseCoordinatesOperationsAndSemanticTerminalInOneCAS(t *testing.T) {
	ctx := context.Background()
	for _, kind := range []runtimecontract.RuntimeProviderDocumentKind{
		runtimecontract.RuntimeProviderDocumentOperations,
		runtimecontract.RuntimeProviderDocumentSemanticPlan,
	} {
		t.Run(string(kind), func(t *testing.T) {
			var aggregate RuntimeAggregate
			var session runtimecontract.RuntimeCandidateRequestSession
			if kind == runtimecontract.RuntimeProviderDocumentSemanticPlan {
				aggregate, session = preparedSemanticProviderTerminalSession(t)
			} else {
				aggregate, session = preparedOperationsProviderTerminalSession(t)
			}
			repository := &providerTerminalCASRepository{aggregate: aggregate, mode: providerTerminalCASNormal}
			controller, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			audit := runtimecontract.ProviderRequestAuditView{
				RequestID: session.LastPhysicalCallID, LogicalRequestID: session.LogicalRequestID, PhysicalCallID: session.LastPhysicalCallID,
				WorkspaceID: string(aggregate.Facts.WorkspaceID), RunID: string(aggregate.Facts.RunID),
				Status: "failed", ProviderSubmitted: false, PhysicalResponseMode: llmaudit.PhysicalResponseModeNonStreaming,
				ContextBudget: llmaudit.ContextBudget{ProviderID: "test-provider", ModelID: "test-model"},
				// Serialized request bytes are not a submission fact. A nonzero value
				// must therefore not prevent exact release.
				RequestBytes: 137,
			}
			settlement, err := NewRuntimeProviderBudgetSettlementFromAudit(string(aggregate.Facts.WorkspaceID), string(aggregate.Facts.RunID), session.LogicalRequestID, session.LastPhysicalCallID, &audit, nil)
			if err != nil {
				t.Fatal(err)
			}
			if settlement.Status != RuntimeProviderBudgetSettlementReleased {
				t.Fatalf("unsubmitted audit settlement=%+v", settlement)
			}
			diagnostic := runtimecontract.RuntimeOriginalDiagnostic{
				SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent,
				OwnerModuleID: runtimecontract.RuntimeModuleLLM,
				OriginalClass: string(runtimecontract.RuntimeCandidateFailureClassProviderConfiguration),
				OriginalCode:  "provider_configuration_unavailable", OriginalMessage: "provider configuration unavailable before submit", Signature: "provider-configuration-unavailable/exact",
			}
			terminalCode := "provider_configuration"
			if kind == runtimecontract.RuntimeProviderDocumentSemanticPlan {
				terminalCode = "semantic_provider_configuration"
			}
			terminal, err := NormalizeRuntimeProviderRequestTerminal(RuntimeProviderRequestTerminalInput{
				DocumentKind: kind, OutcomeClass: runtimecontract.RuntimeCandidateRequestOutcomeProviderConfiguration,
				TerminalCode: terminalCode, TerminalSignature: strings.ReplaceAll(terminalCode, "_", "-"),
				Message: diagnostic.OriginalMessage, OriginalDiagnostic: &diagnostic,
			})
			if err != nil {
				t.Fatal(err)
			}
			persisted, err := controller.planning.coordinateProviderRequestTerminal(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, terminal, &settlement)
			if err != nil {
				t.Fatal(err)
			}
			if repository.saves != 1 || persisted.CandidateRequestSessions[0].State != runtimecontract.RuntimeCandidateRequestSessionTerminal || persisted.Facts.Failure == nil {
				t.Fatalf("release terminal was not one CAS: saves=%d aggregate=%+v", repository.saves, persisted)
			}
			matched := 0
			for _, allocation := range persisted.ExecutionBudget.Allocations {
				if allocation.LogicalConsumptionID != session.LogicalRequestID || allocation.ScopeRef != session.LastPhysicalCallID {
					continue
				}
				matched++
				if allocation.State != runtimecontract.RuntimeBudgetAllocationReleased || allocation.OwnerFactRef != settlement.OwnerFactRef {
					t.Fatalf("unsubmitted allocation was not exactly released: %+v", allocation)
				}
			}
			if matched == 0 || len(persisted.ExecutionBudget.Consumptions) != 0 {
				t.Fatalf("release consumed provider budget: matched=%d consumptions=%+v", matched, persisted.ExecutionBudget.Consumptions)
			}
			fresh, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			replayed, err := fresh.planning.coordinateProviderRequestTerminal(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, terminal, &settlement)
			if err != nil || replayed.StateRevision != persisted.StateRevision || repository.saves != 1 {
				t.Fatalf("fresh release replay changed state: saves=%d aggregate=%+v err=%v", repository.saves, replayed, err)
			}
		})
	}
}

func TestProviderBudgetSettlementRejectsUntrustedSubmissionFacts(t *testing.T) {
	audit := runtimecontract.ProviderRequestAuditView{
		RequestID: "physical-1", LogicalRequestID: "logical-1", PhysicalCallID: "physical-1",
		WorkspaceID: "ws-1", RunID: "run-1", Status: "failed",
		PhysicalResponseMode: llmaudit.PhysicalResponseModeNonStreaming,
		ContextBudget:        llmaudit.ContextBudget{ProviderID: "test-provider", ModelID: "test-model"},
	}
	tests := []struct {
		name   string
		mutate func(*runtimecontract.ProviderRequestAuditView) *llmaudit.ProviderUsage
	}{
		{name: "physical_identity", mutate: func(a *runtimecontract.ProviderRequestAuditView) *llmaudit.ProviderUsage {
			a.RequestID = "physical-other"
			return nil
		}},
		{name: "logical_identity", mutate: func(a *runtimecontract.ProviderRequestAuditView) *llmaudit.ProviderUsage {
			a.LogicalRequestID = "logical-other"
			return nil
		}},
		{name: "physical_call_identity", mutate: func(a *runtimecontract.ProviderRequestAuditView) *llmaudit.ProviderUsage {
			a.PhysicalCallID = "physical-other"
			return nil
		}},
		{name: "workspace_scope", mutate: func(a *runtimecontract.ProviderRequestAuditView) *llmaudit.ProviderUsage {
			a.WorkspaceID = "ws-other"
			return nil
		}},
		{name: "run_scope", mutate: func(a *runtimecontract.ProviderRequestAuditView) *llmaudit.ProviderUsage {
			a.RunID = "run-other"
			return nil
		}},
		{name: "response_mode", mutate: func(a *runtimecontract.ProviderRequestAuditView) *llmaudit.ProviderUsage {
			a.PhysicalResponseMode = "stream"
			return nil
		}},
		{name: "unsubmitted_usage", mutate: func(*runtimecontract.ProviderRequestAuditView) *llmaudit.ProviderUsage {
			return &llmaudit.ProviderUsage{TotalTokens: 1}
		}},
		{name: "context_submission", mutate: func(a *runtimecontract.ProviderRequestAuditView) *llmaudit.ProviderUsage {
			a.ContextBudget.ProviderSubmitted = true
			return nil
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			value := audit
			usage := tc.mutate(&value)
			if _, err := NewRuntimeProviderBudgetSettlementFromAudit("ws-1", "run-1", "logical-1", "physical-1", &value, usage); err == nil {
				t.Fatal("untrusted provider submission fact was accepted")
			}
		})
	}
}

func TestCandidateProviderTransportRetryUsesDistinctPhysicalAllocation(t *testing.T) {
	ctx := context.Background()
	aggregate := stateChangeAggregate(t)
	repository := &controllerRepository{aggregate: &aggregate}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	_, session, err := controller.ensureCandidateRequestSession(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, runtimecontract.RuntimeProviderDocumentOperations, "transport-budget-prefix", "transport-budget-manifest", "")
	if err != nil {
		t.Fatal(err)
	}
	firstPhysicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, 1)
	if _, err := controller.markCandidateRequestSubmitted(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, 1, firstPhysicalCallID); err != nil {
		t.Fatal(err)
	}
	transportOutcome := runtimecontract.RuntimeCandidateRequestOutcome{
		Class: runtimecontract.RuntimeCandidateRequestOutcomeProviderTransient, Code: "provider_timeout", Signature: "provider-timeout",
	}
	diagnostic := runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent,
		OwnerModuleID: runtimecontract.RuntimeModuleLLM,
		OriginalClass: string(runtimecontract.RuntimeCandidateFailureClassProviderTransient),
		OriginalCode:  transportOutcome.Code, OriginalMessage: "provider timed out", Signature: transportOutcome.Signature,
	}
	recovery := runtimeProviderTransportRecoveryFactForTest(t, runtimecontract.RuntimeProviderDocumentOperations, transportOutcome, diagnostic.OriginalMessage, &diagnostic)
	pending, err := controller.coordinateProviderRequestRecovery(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, firstPhysicalCallID, recovery, runtimeProviderConfirmedBudgetSettlementForTest(t, session.LogicalRequestID, firstPhysicalCallID, nil))
	if err != nil {
		t.Fatal(err)
	}
	retry := pending.CandidateRequestSessions[0]
	secondPhysicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, retry.NextSequence)
	if retry.NextSequence != 2 || retry.TransportOutcomes != 1 || secondPhysicalCallID == firstPhysicalCallID {
		t.Fatalf("transport retry identity did not advance: session=%+v first=%s second=%s", retry, firstPhysicalCallID, secondPhysicalCallID)
	}
	resubmitted, err := controller.markCandidateRequestSubmitted(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, retry.NextSequence, secondPhysicalCallID)
	if err != nil {
		t.Fatal(err)
	}
	var callScopes []string
	for _, allocation := range resubmitted.ExecutionBudget.Allocations {
		if allocation.Dimension == runtimecontract.RuntimeBudgetDimensionProviderCall && allocation.LogicalConsumptionID == session.LogicalRequestID {
			callScopes = append(callScopes, allocation.ScopeRef)
		}
	}
	if len(callScopes) != 2 || callScopes[0] == callScopes[1] {
		t.Fatalf("transport retry provider-call allocations = %v", callScopes)
	}
}

func TestCandidateProviderBudgetExhaustionLeavesSubmissionUnchanged(t *testing.T) {
	ctx := context.Background()
	aggregate := stateChangeAggregate(t)
	policy := aggregate.ExecutionBudget.Policy.Clone()
	for idx := range policy.Dimensions {
		if policy.Dimensions[idx].Dimension == runtimecontract.RuntimeBudgetDimensionProviderCall {
			policy.Dimensions[idx].Limit = 1
		}
	}
	policy.PolicyHash = runtimecontract.StableRuntimeExecutionBudgetPolicyHash(policy)
	ledger, err := runtimecontract.NewRuntimeExecutionBudgetLedger(policy)
	if err != nil {
		t.Fatal(err)
	}
	ledger, _, _, err = runtimecontract.ReserveRuntimeBudgetAllocations(ledger, []runtimecontract.RuntimeBudgetAllocationSpec{{Dimension: runtimecontract.RuntimeBudgetDimensionProviderCall, LogicalConsumptionID: "used", ScopeRef: "used-physical", ReservedAmount: 1}})
	if err != nil {
		t.Fatal(err)
	}
	aggregate.ExecutionBudget = ledger
	aggregate.Facts.ExecutionBudgetPolicy = policy
	repository := &controllerRepository{aggregate: &aggregate}
	controller, _ := NewRuntimeController(repository)
	prepared, session, err := controller.ensureCandidateRequestSession(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, runtimecontract.RuntimeProviderDocumentOperations, "exhaust-prefix", "exhaust-manifest", "")
	if err != nil {
		t.Fatal(err)
	}
	saves := repository.saves
	_, err = controller.markCandidateRequestSubmitted(ctx, prepared.Facts.WorkspaceID, prepared.Facts.RunID, session.LogicalRequestID, 1, "physical-exhausted")
	var exhausted *runtimecontract.RuntimeBudgetExhaustedError
	if !errors.As(err, &exhausted) || repository.saves != saves || repository.aggregate.CandidateRequestSessions[0].ProviderSubmitted {
		t.Fatalf("exhaustion=%+v err=%v saves=%d/%d session=%+v", exhausted, err, repository.saves, saves, repository.aggregate.CandidateRequestSessions[0])
	}
}

func assertRuntimeExecutionDimension(t *testing.T, view runtimecontract.RuntimeExecutionBudgetView, dimension runtimecontract.RuntimeBudgetDimension, confirmed, reserved, unknown int64) {
	t.Helper()
	for _, item := range view.Dimensions {
		if item.Dimension == dimension {
			if item.Confirmed != confirmed || item.Reserved != reserved || item.Unknown != unknown {
				t.Fatalf("budget dimension %q = %+v", dimension, item)
			}
			return
		}
	}
	t.Fatalf("budget dimension %q is missing", dimension)
}
