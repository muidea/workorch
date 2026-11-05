package runtime

import (
	"encoding/json"
	"go/ast"
	"os"
	"reflect"
	"testing"

	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// This is an ownership guard, not a timing threshold. Raw admission and event
// postimages retain their complete gate; formal-result consumers must not add
// another full historical adjudication before a provider request.
func TestSemanticProjectionsConsumeOwnerAdmission(t *testing.T) {
	consumers := map[string]bool{"SemanticExecutionHistory": true, "SemanticExecutionBudget": true, "SemanticWorkContext": true, "CompileRuntimeTerminalSnapshotSourcesFromAggregate": true}
	found := 0
	for _, file := range parseRuntimeProductionSources(t) {
		for _, declaration := range file.Decls {
			fn, ok := declaration.(*ast.FuncDecl)
			if !ok || !consumers[fn.Name.Name] {
				continue
			}
			found++
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				if selector, ok := call.Fun.(*ast.SelectorExpr); ok && selector.Sel.Name == "Validate" {
					if id, ok := selector.X.(*ast.Ident); ok && (id.Name == "a" || id.Name == "aggregate") {
						t.Errorf("%s re-admits the formal aggregate", fn.Name.Name)
					}
				}
				return true
			})
		}
	}
	if found != len(consumers) {
		t.Fatal("semantic projection guard lost a production consumer")
	}
}

type semanticCommittedProjection struct {
	History rc.RuntimeSemanticExecutionHistory
	Budget  rc.RuntimeSemanticExecutionBudget
	Work    *rc.RuntimeSemanticWorkContext
}

func semanticCommittedProjectionForTest(t *testing.T, a RuntimeAggregate) semanticCommittedProjection {
	t.Helper()
	m := a.SemanticPlanning.Manifest.Clone()
	c := rc.RuntimeSemanticPlanRequestContext{
		WorkspaceID: a.Facts.WorkspaceID, RunID: a.Facts.RunID, Definition: a.Definition,
		DefinitionSeed: a.Facts.DefinitionSeed, Activation: a.Facts.Activation,
		PlanStep: rc.RuntimeSemanticPlanStepCoverageOrder,
	}
	// Use the archived batch intent, without reconstructing an old frozen
	// provider expectation using today's request renderer.
	if a.SemanticPlanning.ActiveBatchID != "" {
		for _, batch := range a.SemanticPlanning.Batches {
			if batch.BatchID == a.SemanticPlanning.ActiveBatchID {
				c.PlanStep = rc.RuntimeSemanticPlanStepProofBatch
				c.BatchID, c.BatchSequence = batch.BatchID, batch.Sequence
				c.BatchCoverageWorkItemIDs = batch.CoverageWorkItemIDs
			}
		}
	}
	var err error
	c.CoverageProgress, err = rc.BuildRuntimeSemanticCoverageProgress(c, m, a.Facts, a.StateRevision)
	if err != nil {
		t.Fatal(err)
	}
	var result semanticCommittedProjection
	result.History, err = SemanticExecutionHistory(a, c, m)
	if err != nil {
		t.Fatal(err)
	}
	result.Budget, err = SemanticExecutionBudget(a, c, m)
	if err != nil {
		t.Fatal(err)
	}
	result.Work, err = SemanticWorkContext(a, m)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestSemanticCommittedProjectionAdmissionAndRecovery(t *testing.T) {
	_, _, a, _ := semanticRequestIntentFixture(t)
	record := RuntimeStateRecordFromAggregate(a)
	admitted, err := AdmitRuntimeOwnerStateRecord(record)
	if err != nil {
		t.Fatal(err)
	}
	baseline := semanticCommittedProjectionForTest(t, admitted)
	// Structural corruption is still the raw owner's responsibility. The formal
	// consumption contract does not turn arbitrary input into an admitted result.
	bad := record.Clone()
	bad.Facts.RunID = "foreign-run"
	if _, err := AdmitRuntimeOwnerStateRecord(bad); err == nil {
		t.Fatal("raw admission accepted foreign aggregate scope")
	}
	if _, err := ReadCommittedRuntimeOwnerStateRecord(bad); err == nil {
		t.Fatal("committed read lost scope fence")
	}
	restored, err := ReadCommittedRuntimeOwnerStateRecord(record)
	if err != nil {
		t.Fatal(err)
	}
	if got := semanticCommittedProjectionForTest(t, restored); !reflect.DeepEqual(got, baseline) {
		t.Fatal("formal reload changed request observations")
	}
}

// The restricted archive is optional in ordinary CI. An explicit path makes
// the current complete real input replayable without shipping business bytes.
func TestSemanticCommittedProjectionFixedArchive(t *testing.T) {
	path := os.Getenv("WORKORCH_WORK_COMPILER_ARCHIVE")
	if path == "" {
		t.Skip("explicit fixed read-only archive required")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var archive map[string]struct{ RuntimeState rc.RuntimeStateRecord }
	if err := json.Unmarshal(body, &archive); err != nil {
		t.Fatal(err)
	}
	count, receipts := 0, 0
	for _, item := range archive {
		if item.RuntimeState.SemanticPlanning.Manifest == nil {
			continue
		}
		// Independent full audit remains available for unaccepted input. Production
		// read/recovery consumes the owner result rather than running this again.
		admitted, err := AdmitRuntimeOwnerStateRecord(item.RuntimeState)
		if err != nil {
			t.Fatal(err)
		}
		before, err := json.Marshal(admitted)
		if err != nil {
			t.Fatal(err)
		}
		baseline := semanticCommittedProjectionForTest(t, admitted)
		receipts += baseline.History.TotalReceipts
		restored, err := ReadCommittedRuntimeOwnerStateRecord(item.RuntimeState)
		if err != nil {
			t.Fatal(err)
		}
		if got := semanticCommittedProjectionForTest(t, restored); !reflect.DeepEqual(got, baseline) {
			t.Fatal("fixed archive changed across formal recovery")
		}
		after, err := json.Marshal(admitted)
		if err != nil || string(before) != string(after) {
			t.Fatal("semantic projection changed original owner input")
		}
		count++
	}
	if count == 0 || receipts == 0 {
		t.Fatal("archive lacks committed semantic receipt sources")
	}
}

func TestSemanticCommittedWorkContextKeepsExactIntent(t *testing.T) {
	a := implementationWorkPrivatePipeline(t, rc.RuntimeImplementationWorkPurposeEffect, []string{"target.txt"}, nil, true)
	before, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := SemanticWorkContext(a, *a.SemanticPlanning.Manifest)
	if err != nil || baseline == nil || baseline.Intent == nil || baseline.Content == nil {
		t.Fatalf("work context lost admitted intent/content: %+v %v", baseline, err)
	}
	reloaded, err := ReadCommittedRuntimeOwnerStateRecord(RuntimeStateRecordFromAggregate(a))
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := SemanticWorkContext(reloaded, *reloaded.SemanticPlanning.Manifest)
	if err != nil || !reflect.DeepEqual(baseline, replayed) {
		t.Fatalf("formal reload changed work context: %v", err)
	}
	for name, mutate := range map[string]func(*RuntimeAggregate){
		"foreign scope":           func(a *RuntimeAggregate) { a.Facts.RunID = "foreign" },
		"missing original intent": func(a *RuntimeAggregate) { a.SemanticPlanning.History = nil; a.SemanticPlanning.HistoryArchive = nil },
	} {
		t.Run(name, func(t *testing.T) {
			bad := a.CloneRaw()
			mutate(&bad)
			if _, err := SemanticWorkContext(bad, *a.SemanticPlanning.Manifest); err == nil {
				t.Fatal("work source mismatch accepted")
			}
			if _, err := AdmitRuntimeOwnerStateRecord(RuntimeStateRecordFromAggregate(bad)); err == nil {
				t.Fatal("owner admitted corrupt original work source")
			}
		})
	}
	baseline.Intent.RemainingWork = "caller change"
	baseline.Content.Snapshots[0].Content = "caller change"
	after, err := json.Marshal(a)
	if err != nil || string(after) != string(before) {
		t.Fatal("work context aliases original owner values")
	}
}
