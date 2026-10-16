package runtime

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestOwnerInvariantSourcesSemanticQueryColdOracle(t *testing.T) {
	_, repository, _ := prepareOwnerFactAssessment(t, true)
	base := repository.aggregate.CloneRaw()
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, arm := range []string{"valid", "raw definition", "converted definition", "raw manifest", "batch", "owner result", "coordinated focus", "current purpose", "budget", "nil empty", "mixed first error"} {
		t.Run(arm, func(t *testing.T) {
			r := &ownerQueryTestRepository{raw: runtimeStateRecordForTerminalNonterminalRecoveryTest(t, base), aggregate: base.CloneRaw()}
			c, _ := NewRuntimeController(r)
			for call := 0; call < 3; call++ {
				if call == 1 {
					switch arm {
					case "raw definition":
						r.raw.Facts.DefinitionFocuses[0].AssessmentText = "same hash drift"
					case "converted definition":
						r.aggregate.Facts.DefinitionFocuses[0].AssessmentText = "same hash drift"
					case "raw manifest":
						r.aggregate.SemanticPlanning.Manifest.ProofObligations[0].DeclarationRef = "same hash drift"
					case "batch":
						r.aggregate.SemanticPlanning.Batches[0].RequestLogicalID = "same hash drift"
					case "owner result":
						r.aggregate.SemanticPlanning.BatchFeasibilityResults[0].Feasibilities[0].OwnerModuleID = "wrong owner"
					case "coordinated focus":
						r.aggregate = runtimeTestRewriteSemanticBindingProjections(t, r.aggregate, true, false, false, false)
					case "current purpose":
						r.aggregate.Facts.Coverage[0].State = runtimecontract.RuntimeCoverageItemStateSatisfied
					case "budget":
						r.aggregate.ExecutionBudget.Policy.PolicyRevision = "same hash drift"
					case "nil empty":
						r.aggregate.SemanticPlanning.History = []runtimecontract.RuntimeSemanticPlanningGeneration{}
					case "mixed first error":
						r.raw.ResultFeedbackVersion = 0
						r.aggregate.SemanticPlanning.Manifest.Hash = "bad"
					}
				}
				if call == 2 {
					r.raw = runtimeStateRecordForTerminalNonterminalRecoveryTest(t, base)
					r.aggregate = base.CloneRaw()
				}
				before := r.aggregate.CloneRaw()
				got, err := c.Load(context.Background(), base.Facts.WorkspaceID, base.Facts.RunID)
				want := independentOwnerStateImage(&r.raw, r.aggregate)
				if fmt.Sprint(err) != fmt.Sprint(want) {
					t.Fatalf("call %d first error: %v != %v", call, err, want)
				}
				if err != nil && !reflect.DeepEqual(got, RuntimeAggregate{}) {
					t.Fatal("partial aggregate")
				}
				if !reflect.DeepEqual(before, r.aggregate) {
					t.Fatal("query mutated owner")
				}
			}
			if r.reads.Load() != 3 {
				t.Fatal("skipped real reads")
			}
		})
	}
}
