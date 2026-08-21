package runtime

import (
	"context"
	"errors"
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
	observation, err := runtimecontract.NewRuntimeProviderBudgetObservation(session.LogicalRequestID, physicalCallID, runtimecontract.RuntimeProviderConsumptionConfirmed, &llmaudit.ProviderUsage{InputTokens: 11, OutputTokens: 7, TotalTokens: 18})
	if err != nil {
		t.Fatal(err)
	}
	completed, err := controller.planning.completeCandidateRequestDocument(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, physicalCallID, &observation)
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
	drifted, err := runtimecontract.NewRuntimeProviderBudgetObservation(session.LogicalRequestID, "physical-other", runtimecontract.RuntimeProviderConsumptionConfirmed, nil)
	if err != nil {
		t.Fatal(err)
	}
	saves := repository.saves
	if _, err := controller.planning.completeCandidateRequestDocument(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, physicalCallID, &drifted); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("drifted provider budget observation err=%v, want conflict", err)
	}
	stored := repository.aggregate.CandidateRequestSessions[0]
	if repository.saves != saves || !stored.ProviderSubmitted || stored.State == runtimecontract.RuntimeCandidateRequestSessionDocumentComplete {
		t.Fatalf("drifted observation changed request session: saves=%d/%d session=%+v", repository.saves, saves, stored)
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
	pending, err := controller.recordCandidateRequestTransportOutcome(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, firstPhysicalCallID, runtimecontract.RuntimeCandidateRequestOutcome{
		Class: runtimecontract.RuntimeCandidateRequestOutcomeProviderTransient, Code: "provider_timeout", Signature: "provider-timeout",
	})
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
