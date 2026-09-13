package runtime

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestSemanticExecutionBudgetProviderCASRestartAndSettlement(t *testing.T) {
	ctx := context.Background()
	for _, mode := range []providerTerminalCASMode{providerTerminalCASNormal, providerTerminalCASFailBeforeWrite, providerTerminalCASLoseResponse} {
		for _, unknown := range []bool{false, true} {
			t.Run(string(mode)+map[bool]string{false: "/confirmed", true: "/unknown"}[unknown], func(t *testing.T) {
				_, controller, a, expected := semanticRequestIntentFixture(t)
				prepared, session, err := controller.ensureCandidateRequestSessionWithExpected(ctx, a.Facts.WorkspaceID, a.Facts.RunID, runtimecontract.RuntimeProviderDocumentSemanticPlan, expected.StablePrefixHash, expected.ManifestHash, "", &expected)
				if err != nil {
					t.Fatal(err)
				}
				before, _ := semanticExecutionBudgetSnapshot(t, prepared)
				repository := &executionLimitsCASRepository{providerTerminalCASRepository: &providerTerminalCASRepository{aggregate: prepared, mode: mode}, t: t}
				controller, err = NewRuntimeController(repository)
				if err != nil {
					t.Fatal(err)
				}
				physical := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, 1)
				_, err = controller.markCandidateRequestSubmitted(ctx, a.Facts.WorkspaceID, a.Facts.RunID, session.LogicalRequestID, 1, physical)
				if (mode == providerTerminalCASNormal) != (err == nil) {
					t.Fatalf("CAS result: %v", err)
				}
				interrupted, _ := semanticExecutionBudgetSnapshot(t, repository.aggregate)
				wantReserved := int64(1)
				if mode == providerTerminalCASFailBeforeWrite {
					wantReserved = 0
				}
				if interrupted.PhysicalDimensions[0].Reserved != wantReserved {
					t.Fatal("CAS window projected an uncommitted reservation")
				}
				repository.aggregate = reloadExecutionLimitedAggregate(t, repository.aggregate)
				repository.mode = providerTerminalCASNormal
				controller, err = NewRuntimeController(repository)
				if err != nil {
					t.Fatal(err)
				}
				submitted, err := controller.markCandidateRequestSubmitted(ctx, a.Facts.WorkspaceID, a.Facts.RunID, session.LogicalRequestID, 1, physical)
				if err != nil {
					t.Fatal(err)
				}
				reserved, _ := semanticExecutionBudgetSnapshot(t, submitted)
				if reserved.PhysicalDimensions[0].Reserved != 1 || reserved.PhysicalDimensions[0].Remaining != before.PhysicalDimensions[0].Remaining-1 {
					t.Fatal("restart lost reservation")
				}
				settlement := runtimeProviderConfirmedBudgetSettlementForTest(t, session.LogicalRequestID, physical, nil)
				if unknown {
					value, e := NewRuntimeProviderBudgetUnknownSettlement(session.LogicalRequestID, physical)
					if e != nil {
						t.Fatal(e)
					}
					settlement = &value
				}
				completed, err := controller.planning.completeCandidateRequestDocument(ctx, a.Facts.WorkspaceID, a.Facts.RunID, session.LogicalRequestID, physical, settlement)
				if err != nil {
					t.Fatal(err)
				}
				settled, _ := semanticExecutionBudgetSnapshot(t, completed)
				d := settled.PhysicalDimensions[0]
				if d.Reserved != 0 || d.Confirmed+d.Unknown != 1 || (unknown && d.Unknown != 1) || (!unknown && d.Confirmed != 1) || d.Remaining != reserved.PhysicalDimensions[0].Remaining {
					t.Fatal("settlement refunded physical call")
				}
				writes := repository.saves
				duplicate, err := controller.planning.completeCandidateRequestDocument(ctx, a.Facts.WorkspaceID, a.Facts.RunID, session.LogicalRequestID, physical, settlement)
				if err != nil || repository.saves != writes {
					t.Fatalf("duplicate settlement wrote: %v", err)
				}
				replayed, _ := semanticExecutionBudgetSnapshot(t, duplicate)
				if !reflect.DeepEqual(settled, replayed) {
					t.Fatal("duplicate changed planning budget")
				}
			})
		}
	}
}

func semanticExecutionBudgetSnapshot(t *testing.T, a RuntimeAggregate) (runtimecontract.RuntimeSemanticExecutionBudget, runtimecontract.RuntimeSemanticPlanRequestContext) {
	t.Helper()
	if a.SemanticPlanning.Manifest == nil {
		t.Fatal("budget fixture has no semantic manifest")
	}
	m := a.SemanticPlanning.Manifest.Clone()
	c := runtimecontract.RuntimeSemanticPlanRequestContext{WorkspaceID: a.Facts.WorkspaceID, RunID: a.Facts.RunID, Definition: a.Definition,
		DefinitionSeed: a.Facts.DefinitionSeed, Activation: a.Facts.Activation, PlanStep: runtimecontract.RuntimeSemanticPlanStepCoverageOrder}
	var err error
	c.CoverageProgress, err = runtimecontract.BuildRuntimeSemanticCoverageProgress(c, m, a.Facts, a.StateRevision)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(a)
	b, err := SemanticExecutionBudget(a, c, m)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(a)
	if string(before) != string(after) {
		t.Fatal("budget projection changed owner aggregate")
	}
	restored := reloadExecutionLimitedAggregate(t, a)
	replayed, err := SemanticExecutionBudget(restored, c, m)
	if err != nil || !reflect.DeepEqual(b, replayed) {
		t.Fatalf("budget changed on same-version reload: %v", err)
	}
	return b, c
}

func TestSemanticExecutionBudgetRequiresExactCurrentOwnerSnapshot(t *testing.T) {
	_, _, a, _, _, _ := sealedSemanticPlanningAggregate(t, &runtimecontract.RuntimeExecutionLimits{ProtocolRef: runtimecontract.RuntimeExecutionLimitsV1, MaxProviderCalls: 8, MaxCapabilityOperations: 4, MaxProofBatchesPerCWI: 2})
	b, c := semanticExecutionBudgetSnapshot(t, a)
	if len(b.VisibleProofBatches) != 1 || b.VisibleProofBatches[0].Used != 1 || *b.VisibleProofBatches[0].Remaining != 1 {
		t.Fatal("sealed proof batch omitted")
	}
	for _, defect := range []string{"scope", "revision", "evidence revision", "limits", "seed", "manifest", "ledger"} {
		t.Run(defect, func(t *testing.T) {
			badContext := c.Normalized()
			badAggregate := reloadExecutionLimitedAggregate(t, a)
			manifest := a.SemanticPlanning.Manifest.Clone()
			switch defect {
			case "scope":
				badContext.RunID = "foreign"
			case "revision":
				badContext.CoverageProgress.SourceStateRevision--
			case "evidence revision":
				badContext.CoverageProgress.EvidenceRevision = "other"
			case "limits":
				badContext.Activation.ExecutionContract.ExecutionLimits = nil
			case "seed":
				badContext.DefinitionSeed.Hash = "other"
			case "manifest":
				manifest.Hash = "retired"
			case "ledger":
				badAggregate.ExecutionBudget.Policy.PolicyHash = "corrupt"
			}
			badContext.CoverageProgress.Hash = runtimecontract.StableRuntimeSemanticCoverageProgressHash(badContext.CoverageProgress)
			if _, err := SemanticExecutionBudget(badAggregate, badContext, manifest); err == nil {
				t.Fatal("unrelated or stale owner budget accepted")
			}
		})
	}
}
