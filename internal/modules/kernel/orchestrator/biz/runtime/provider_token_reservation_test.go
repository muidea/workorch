package runtime

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/llmaudit"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestProviderTokenReservationRefusesSealedPerCallLimitBeforeSubmission(t *testing.T) {
	for _, missingCeiling := range []bool{false, true} {
		a := stateChangeAggregateWithPreparation(t, func(a *RuntimeAggregate) {
			c := runtimeSubmissionTestCeiling(a.Facts)
			c.ContextWindowTokens, c.MaxInputTokens = 500000, 279000
			a.Facts.Resources.ProviderCeiling = c
		})
		// An existing sealed policy is never enlarged on submission or reload.
		for i := range a.ExecutionBudget.Policy.Dimensions {
			if a.ExecutionBudget.Policy.Dimensions[i].Dimension == rc.RuntimeBudgetDimensionProviderInputToken {
				a.ExecutionBudget.Policy.Dimensions[i].MaxIncrement = 131072
			}
		}
		a.ExecutionBudget.Policy.PolicyHash = rc.StableRuntimeExecutionBudgetPolicyHash(a.ExecutionBudget.Policy)
		a.Facts.ExecutionBudgetPolicy = a.ExecutionBudget.Policy.Clone()
		repo := &controllerRepository{aggregate: &a}
		controller, err := NewRuntimeController(repo)
		if err != nil {
			t.Fatal(err)
		}
		_, session, err := controller.ensureCandidateRequestSession(context.Background(), a.Facts.WorkspaceID, a.Facts.RunID, rc.RuntimeProviderDocumentOperations, "fixed-prefix", "fixed-manifest", "")
		if err != nil {
			t.Fatal(err)
		}
		if missingCeiling {
			repo.aggregate.Facts.Resources.ProviderCeiling = nil
		}
		before := repo.aggregate.ExecutionBudget.Clone()
		saves := repo.saves
		_, err = controller.markCandidateRequestSubmitted(context.Background(), a.Facts.WorkspaceID, a.Facts.RunID, session.LogicalRequestID, 1, rc.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, 1))
		if missingCeiling {
			if err == nil {
				t.Fatal("missing ceiling authorized submission")
			}
		} else {
			var exhausted *rc.RuntimeBudgetExhaustedError
			if !errors.As(err, &exhausted) || exhausted.Required != 500000 || exhausted.MaxIncrement != 131072 {
				t.Fatalf("wrong refusal: %v", err)
			}
		}
		if repo.saves != saves || repo.aggregate.CandidateRequestSessions[0].ProviderSubmitted || !reflect.DeepEqual(repo.aggregate.ExecutionBudget, before) {
			t.Fatal("refusal persisted a submission or allocation")
		}
	}
}

func TestProviderTokenReservationOverrunPreservesExactUsageAndAtomicLedger(t *testing.T) {
	for _, usage := range []*llmaudit.ProviderUsage{{InputTokens: 32769}, {OutputTokens: 8193}} {
		a := stateChangeAggregate(t)
		session := rc.RuntimeCandidateRequestSession{DocumentKind: rc.RuntimeProviderDocumentOperations, LogicalRequestID: "logical", ExpectedDocument: &rc.RuntimeExpectedDocumentContract{MaxOutputTokens: 8192}}
		if err := reserveCandidateRequestExecutionBudget(&a, session, 1, "physical"); err != nil {
			t.Fatal(err)
		}
		before := a.ExecutionBudget.Clone()
		s := runtimeProviderConfirmedBudgetSettlementForTest(t, "logical", "physical", usage)
		if changed, err := applyCandidateRequestBudgetSettlement(&a, s); changed || err == nil || !strings.Contains(err.Error(), "physical=physical dimension=provider_") {
			t.Fatalf("missing exact overrun diagnostic: %v", err)
		}
		if !reflect.DeepEqual(a.ExecutionBudget, before) || !reflect.DeepEqual(s.Usage, usage) {
			t.Fatal("overrun changed ledger or native usage")
		}
	}
}

// Fixed values from the run-003 owner audit. No live owner data or provider
// request is needed to reproduce the accounting boundary.
func TestProviderReservationCoversNativeUsageAboveFormerFixedIncrement(t *testing.T) {
	for _, kind := range []rc.RuntimeProviderDocumentKind{rc.RuntimeProviderDocumentSemanticPlan, rc.RuntimeProviderDocumentOperations} {
		t.Run(string(kind), func(t *testing.T) {
			a := stateChangeAggregate(t)
			a.Facts.Resources.ProviderCeiling = &rc.ProviderCeiling{
				Selection: a.Facts.Activation.ProviderSelection, ContextWindowTokens: 500000,
				ProviderMaxOutputTokens: 128000, EffectiveMaxOutputTokens: 128000,
				MaxInputTokens: 279000, MaxBytes: 1116000,
				CapabilityRevision: "fixed-capability", ConfigurationRevision: "fixed-configuration",
			}
			policy := a.ExecutionBudget.Policy.Clone()
			session := rc.RuntimeCandidateRequestSession{DocumentKind: kind, LogicalRequestID: "fixed-logical",
				ExpectedDocument: &rc.RuntimeExpectedDocumentContract{MaxOutputTokens: 128000}}
			if err := reserveCandidateRequestExecutionBudget(&a, session, 1, "fixed-physical"); err != nil {
				t.Fatal(err)
			}
			s := runtimeProviderConfirmedBudgetSettlementForTest(t, session.LogicalRequestID, "fixed-physical", &llmaudit.ProviderUsage{InputTokens: 131979, OutputTokens: 516, TotalTokens: 132495})
			if changed, err := applyCandidateRequestBudgetSettlement(&a, s); err != nil || !changed {
				t.Fatalf("native usage must settle within the model-sized reservation: changed=%v err=%v", changed, err)
			}
			before := a.ExecutionBudget.Clone()
			if changed, err := applyCandidateRequestBudgetSettlement(&a, s); err != nil || changed || !reflect.DeepEqual(a.ExecutionBudget, before) {
				t.Fatalf("duplicate changed the budget: changed=%v err=%v", changed, err)
			}
			if !reflect.DeepEqual(a.ExecutionBudget.Policy, policy) {
				t.Fatal("submission or settlement changed the sealed policy")
			}
		})
	}
}

// Submission fixtures declare model capacity explicitly, just like the
// production LLM prerequisite. Missing-capacity tests clear this fact.
func runtimeSubmissionTestCeiling(facts rc.RuntimeFacts) *rc.ProviderCeiling {
	return &rc.ProviderCeiling{Selection: facts.Activation.ProviderSelection,
		ContextWindowTokens: 32768, ProviderMaxOutputTokens: 8192, EffectiveMaxOutputTokens: 8192,
		MaxInputTokens: 16384, MaxBytes: 262144, CapabilityRevision: "submission-cap", ConfigurationRevision: "submission-config"}
}
