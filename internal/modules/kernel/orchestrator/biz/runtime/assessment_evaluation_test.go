package runtime

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestAssessmentSubmissionClassifiesOnceAndConsumersUseSameResult(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "authority_assessment_transition.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	for _, declaration := range file.Decls {
		fn, ok := declaration.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "commitAssessment" {
			continue
		}
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			var name string
			switch f := call.Fun.(type) {
			case *ast.SelectorExpr:
				name = f.Sel.Name
			case *ast.Ident:
				name = f.Name
			}
			switch name {
			case "CompileRuntimeAssessmentEvaluation":
				calls++
			case "DeriveRuntimeAssessmentAdjudication", "canonicalAssessmentProofCompleteWithoutOwnerFactsForPurpose", "canonicalAssessmentEvidenceForCoverageForPurpose", "RuntimeMissingOwnerFactRequirements":
				t.Errorf("submission repeated classification through %s", name)
			}
			return true
		})
	}
	if calls != 1 {
		t.Fatalf("assessment submission has %d authoritative compiler calls", calls)
	}
	for _, ready := range []bool{false, true} {
		controller, repo, _ := prepareOwnerFactAssessment(t, ready)
		before := repo.aggregate.CloneRaw()
		e, err := runtimecontract.CompileRuntimeAssessmentEvaluation(sealedDispatchToRuntimeRecord(*before.ActiveDispatch), before.Facts, sealedDispatchesToRuntimeRecords(before.CompletedDispatches), before.CoverageProofLineage, runtimeLineageAssertionSources(before))
		if err != nil {
			t.Fatal(err)
		}
		if !e.ProofCompleteWithoutOwnerFacts() || (len(e.MissingOwnerFacts) == 0) != ready {
			t.Fatalf("owner-fact gap conflated with missing proof: %+v", e)
		}
		after, err := controller.commitAssessment(context.Background(), "ws-v2", "run-v2")
		if err != nil {
			t.Fatal(err)
		}
		di, _ := runtimeAssessmentAdjudicationIndexes(t, after)
		if !reflect.DeepEqual(e.Adjudication, *after.CompletedDispatches[di].AssessmentAdjudication) {
			t.Fatal("coverage and committed adjudication used different classification")
		}
		if !reflect.DeepEqual(before.Facts, repo.aggregate.Facts) && repo.aggregate.StateRevision == before.StateRevision {
			t.Fatal("uncommitted mutation escaped owner CAS")
		}
	}
}

func runtimeTestDeriveAssessmentAdjudication(dispatch runtimecontract.RuntimeDispatchRecord, facts runtimecontract.RuntimeFacts, completed []runtimecontract.RuntimeDispatchRecord, lineage []runtimecontract.RuntimeCoverageProofLineage, sources runtimecontract.RuntimeLineageAssertionSources) (runtimecontract.RuntimeAssessmentAdjudication, error) {
	e, err := runtimecontract.CompileRuntimeAssessmentEvaluation(dispatch, facts, completed, lineage, sources)
	return e.Adjudication, err
}

func canonicalAssessmentProofCompleteWithoutOwnerFacts(aggregate RuntimeAggregate) bool {
	return canonicalAssessmentProofCompleteWithoutOwnerFactsForPurpose(
		aggregate,
		runtimecontract.RuntimeAssessmentPurposeDefinitionProof,
	)
}

func canonicalAssessmentProofCompleteWithoutOwnerFactsForPurpose(aggregate RuntimeAggregate, purpose runtimecontract.RuntimeAssessmentPurpose) bool {
	focus := aggregate.Facts.Focus.Normalized()
	if len(focus.CoverageWorkItemIDs) == 0 {
		return false
	}
	for _, coverageID := range focus.CoverageWorkItemIDs {
		if _, satisfied := canonicalAssessmentEvidenceWithoutOwnerFactsForCoverageForPurpose(aggregate, coverageID, purpose); !satisfied {
			return false
		}
	}
	return true
}
