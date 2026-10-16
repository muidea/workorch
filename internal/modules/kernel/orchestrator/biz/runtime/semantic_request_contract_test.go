package runtime

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/semanticplan"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func semanticRequestTestCeiling(facts runtimecontract.RuntimeFacts) *runtimecontract.ProviderCeiling {
	return &runtimecontract.ProviderCeiling{
		Selection: facts.Activation.ProviderSelection, ContextWindowTokens: 8192,
		ProviderMaxOutputTokens: 4096, EffectiveMaxOutputTokens: 4096,
		MaxInputTokens: 4096, MaxBytes: 65536, CapabilityRevision: "cap-1", ConfigurationRevision: "config-1",
	}
}

func semanticRequestIntentFixture(t *testing.T) (*controllerRepository, *RuntimeController, RuntimeAggregate, runtimecontract.RuntimeExpectedDocumentContract) {
	t.Helper()
	repo := &controllerRepository{}
	controller, err := NewRuntimeController(repo)
	if err != nil {
		t.Fatal(err)
	}
	facts := controllerFacts()
	facts.Resources.ProviderCeiling = semanticRequestTestCeiling(facts)
	catalog, prerequisite, observation := semanticPlanningCatalogFixture()
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{catalog}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{observation}
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{prerequisite}
	aggregate, err := controller.initialize(context.Background(), facts, controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	manifest, _, _ := semanticPlanningFixture(t, aggregate)
	aggregate, expected := startSemanticRequestForTest(t, controller, aggregate, manifest, "semantic-intent-prefix")
	return repo, controller, aggregate, expected
}

func TestSemanticRequestOwnerRejectsStaleOrForeignIntentBeforeSessionCAS(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		mutate func(*RuntimeAggregate, *runtimecontract.RuntimeExpectedDocumentContract)
	}{
		{"required", func(a *RuntimeAggregate, _ *runtimecontract.RuntimeExpectedDocumentContract) {
			a.SemanticPlanning.Phase = runtimecontract.RuntimeSemanticPlanningRequired
		}},
		{"order_accepted", func(a *RuntimeAggregate, _ *runtimecontract.RuntimeExpectedDocumentContract) {
			a.SemanticPlanning.Phase = runtimecontract.RuntimeSemanticPlanningOrderAccepted
		}},
		{"batch_document_accepted", func(a *RuntimeAggregate, _ *runtimecontract.RuntimeExpectedDocumentContract) {
			a.SemanticPlanning.Phase = runtimecontract.RuntimeSemanticPlanningProofBatchDocumentAccepted
		}},
		{"logical_intent", func(a *RuntimeAggregate, _ *runtimecontract.RuntimeExpectedDocumentContract) {
			a.SemanticPlanning.LogicalRequestID = "different-request"
		}},
		{"manifest", func(a *RuntimeAggregate, _ *runtimecontract.RuntimeExpectedDocumentContract) {
			a.SemanticPlanning.Manifest.Hash = "different-manifest"
		}},
		{"prefix", func(a *RuntimeAggregate, _ *runtimecontract.RuntimeExpectedDocumentContract) {
			a.SemanticPlanning.StablePrefixHash = "different-prefix"
		}},
		{"correction_attempt", func(a *RuntimeAggregate, _ *runtimecontract.RuntimeExpectedDocumentContract) {
			a.SemanticPlanning.Attempt = 1
		}},
		{"missing_ceiling", func(a *RuntimeAggregate, _ *runtimecontract.RuntimeExpectedDocumentContract) {
			a.Facts.Resources.ProviderCeiling = nil
		}},
		{"output_ceiling", func(a *RuntimeAggregate, _ *runtimecontract.RuntimeExpectedDocumentContract) {
			a.Facts.Resources.ProviderCeiling.EffectiveMaxOutputTokens++
		}},
		{"recovery_budget", func(a *RuntimeAggregate, _ *runtimecontract.RuntimeExpectedDocumentContract) {
			a.Facts.Activation.ExecutionContract.CandidateDocumentRecovery.MaxAttempts = 2
		}},
		{"foreign_batch", func(_ *RuntimeAggregate, e *runtimecontract.RuntimeExpectedDocumentContract) {
			e.PlanStep = runtimecontract.RuntimeSemanticPlanStepProofBatch
			e.BatchID, e.CoverageOrderHash = "foreign-batch", "foreign-order"
			e.BatchCoverageWorkItemIDs = []common.CoverageWorkItemID{"foreign-cwi"}
		}},
		{"expected_logical_identity", func(_ *RuntimeAggregate, e *runtimecontract.RuntimeExpectedDocumentContract) {
			e.LogicalRequestID = "foreign-request"
		}},
		{"expected_manifest", func(_ *RuntimeAggregate, e *runtimecontract.RuntimeExpectedDocumentContract) {
			e.ManifestHash = "foreign-manifest"
		}},
		{"expected_prefix", func(_ *RuntimeAggregate, e *runtimecontract.RuntimeExpectedDocumentContract) {
			e.StablePrefixHash = "foreign-prefix"
		}},
		{"expected_schema", func(_ *RuntimeAggregate, e *runtimecontract.RuntimeExpectedDocumentContract) {
			e.SchemaVersion = "foreign-schema"
		}},
		{"expected_hash", func(_ *RuntimeAggregate, _ *runtimecontract.RuntimeExpectedDocumentContract) {}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			repo, controller, aggregate, expected := semanticRequestIntentFixture(t)
			prefix, manifest := expected.StablePrefixHash, expected.ManifestHash
			scenario.mutate(&aggregate, &expected)
			expected.Hash = runtimecontract.StableRuntimeExpectedDocumentHash(expected)
			if scenario.name == "expected_hash" {
				expected.Hash = "invalid-expected-document-hash"
			}
			repo.aggregate = &aggregate
			before, err := json.Marshal(aggregate)
			if err != nil {
				t.Fatal(err)
			}
			saves := repo.saves
			_, _, err = controller.ensureCandidateRequestSessionWithExpected(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, runtimecontract.RuntimeProviderDocumentSemanticPlan, prefix, manifest, "", &expected)
			if err == nil {
				t.Fatal("stale/foreign request was admitted")
			}
			after, marshalErr := json.Marshal(repo.aggregate)
			if marshalErr != nil || repo.saves != saves || string(before) != string(after) {
				t.Fatal("rejected request changed state, sessions, or execution budget")
			}
		})
	}
}

func TestSemanticRequestOwnerBindsExactActiveProofBatch(t *testing.T) {
	for _, field := range []string{"exact", "batch_id", "batch_sequence", "batch_membership", "coverage_order", "correction_hash", "missing_current_batch", "ambiguous_current_batch"} {
		t.Run(field, func(t *testing.T) {
			repo, controller, aggregate, _ := semanticRequestIntentFixture(t)
			ctx := context.Background()
			manifest := aggregate.SemanticPlanning.Manifest.Clone()
			order := runtimecontract.RuntimeSemanticPlanDocument{
				SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent,
				ManifestHash:  manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
				PlanStep:      runtimecontract.RuntimeSemanticPlanStepCoverageOrder,
				CoverageOrder: []common.CoverageWorkItemID{manifest.CoverageItems[0].ID},
			}
			aggregate, err := controller.evaluateSemanticPlan(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, aggregate.SemanticPlanning.LogicalRequestID, order)
			if err != nil {
				t.Fatal(err)
			}
			identityHash := "semantic-proof-batch-0"
			logicalID := runtimecontract.StableRuntimeCandidateRequestSessionID(runtimecontract.RuntimeProviderDocumentSemanticPlan, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, aggregate.Decision.AuthorityTransaction(), aggregate.SemanticPlanning.StablePrefixHash, manifest.Hash, identityHash)
			batch, err := semanticplan.CompileFocusBatch(manifest, order, 0, logicalID, aggregate.Facts.Focus.CoverageWorkItemIDs, aggregate.SemanticPlanning.PendingCoverageIDs, 1)
			if err != nil {
				t.Fatal(err)
			}
			aggregate, err = controller.compileSemanticPlanBatch(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, batch)
			if err != nil {
				t.Fatal(err)
			}
			expected, err := SemanticExpectedDocument(aggregate)
			if err != nil {
				t.Fatal(err)
			}
			switch field {
			case "batch_id":
				expected.BatchID = "another-batch"
			case "batch_sequence":
				expected.BatchSequence++
			case "batch_membership":
				expected.BatchCoverageWorkItemIDs = []common.CoverageWorkItemID{"another-cwi"}
			case "coverage_order":
				expected.CoverageOrderHash = "another-order"
			case "correction_hash":
				identityHash = "semantic-proof-batch-1"
			case "missing_current_batch":
				repo.aggregate.SemanticPlanning.ActiveBatchID = "missing-batch"
			case "ambiguous_current_batch":
				repo.aggregate.SemanticPlanning.Batches = append(repo.aggregate.SemanticPlanning.Batches, batch.Clone())
			}
			expected.Hash = runtimecontract.StableRuntimeExpectedDocumentHash(expected)
			before, _ := json.Marshal(repo.aggregate)
			saves := repo.saves
			_, session, err := controller.ensureCandidateRequestSessionWithExpected(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, runtimecontract.RuntimeProviderDocumentSemanticPlan, expected.StablePrefixHash, expected.ManifestHash, identityHash, &expected)
			if field == "exact" {
				if err != nil || session.ExpectedDocument.Hash != expected.Hash || repo.saves != saves+1 {
					t.Fatalf("exact proof-batch request rejected: %v", err)
				}
				return
			}
			after, _ := json.Marshal(repo.aggregate)
			if err == nil || repo.saves != saves || string(before) != string(after) {
				t.Fatalf("foreign proof-batch request mutated owner state: %v", err)
			}
		})
	}
}

func TestSemanticRequestOwnerExactReplayAfterRestartDoesNotAllocate(t *testing.T) {
	repo, controller, aggregate, expected := semanticRequestIntentFixture(t)
	ctx := context.Background()
	prepared, session, err := controller.ensureCandidateRequestSessionWithExpected(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, runtimecontract.RuntimeProviderDocumentSemanticPlan, expected.StablePrefixHash, expected.ManifestHash, "", &expected)
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.CandidateRequestSessions) != 1 || session.ProviderSubmitted || !reflect.DeepEqual(prepared.ExecutionBudget, aggregate.ExecutionBudget) {
		t.Fatal("preparing session allocated a physical call or budget")
	}
	detached := cloneProviderTerminalAggregate(prepared)
	repo.aggregate = &detached
	restarted, err := NewRuntimeController(repo)
	if err != nil {
		t.Fatal(err)
	}
	saves := repo.saves
	for replay := 0; replay < 2; replay++ {
		result, actual, err := restarted.ensureCandidateRequestSessionWithExpected(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, runtimecontract.RuntimeProviderDocumentSemanticPlan, expected.StablePrefixHash, expected.ManifestHash, "", &expected)
		if err != nil || !reflect.DeepEqual(actual, session) || !reflect.DeepEqual(result, detached) || repo.saves != saves {
			t.Fatalf("exact replay mutated session: err=%v saves=%d", err, repo.saves)
		}
	}
}

func TestSemanticRequestIdentityChangesAfterGenerationRetirement(t *testing.T) {
	ledger := runtimecontract.RuntimeSemanticPlanningLedger{}
	if got := SemanticRequestIdentityHashForKey(ledger, ""); got != "" {
		t.Fatalf("first coverage-order identity = %q", got)
	}
	if got := SemanticRequestIdentityHashForKey(ledger, "semantic-proof-batch-0"); got != "semantic-proof-batch-0" {
		t.Fatalf("first proof-batch identity = %q", got)
	}

	ledger.History = []runtimecontract.RuntimeSemanticPlanningGeneration{{
		RetiredAtStateRevision: 17,
		Reason:                 runtimecontract.RuntimeSemanticPlanningRetirementEligibleClosureChanged,
		Snapshot: runtimecontract.RuntimeSemanticPlanningLedger{
			LogicalRequestID: "retired-request",
			Manifest:         &runtimecontract.RuntimeSemanticPlanManifest{Hash: "retired-manifest"},
		},
	}}
	order := SemanticRequestIdentityHashForKey(ledger, "")
	batch := SemanticRequestIdentityHashForKey(ledger, "semantic-proof-batch-0")
	if order == "" || batch == "semantic-proof-batch-0" || order == batch {
		t.Fatalf("retired generation did not create distinct request identities: order=%q batch=%q", order, batch)
	}
	if replay := SemanticRequestIdentityHashForKey(ledger, ""); replay != order {
		t.Fatalf("retired generation identity is not deterministic: first=%q replay=%q", order, replay)
	}
	encoded, err := json.Marshal(ledger)
	if err != nil {
		t.Fatal(err)
	}
	var restarted runtimecontract.RuntimeSemanticPlanningLedger
	if err := json.Unmarshal(encoded, &restarted); err != nil {
		t.Fatal(err)
	}
	if SemanticRequestIdentityHashForKey(restarted, "") != order || SemanticRequestIdentityHashForKey(restarted, "semantic-proof-batch-0") != batch {
		t.Fatal("fresh process changed generation or batch request identity")
	}
	for _, test := range []struct {
		name   string
		mutate func(*runtimecontract.RuntimeSemanticPlanningLedger)
	}{
		{"retirement revision", func(value *runtimecontract.RuntimeSemanticPlanningLedger) { value.History[0].RetiredAtStateRevision++ }},
		{"reason", func(value *runtimecontract.RuntimeSemanticPlanningLedger) {
			value.History[0].Reason = runtimecontract.RuntimeSemanticPlanningRetirementResourceInvalidated
		}},
		{"manifest", func(value *runtimecontract.RuntimeSemanticPlanningLedger) {
			value.History[0].Snapshot.Manifest.Hash = "other-manifest"
		}},
		{"logical lineage", func(value *runtimecontract.RuntimeSemanticPlanningLedger) {
			value.History[0].Snapshot.LogicalRequestID = "other-request"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var altered runtimecontract.RuntimeSemanticPlanningLedger
			if err := json.Unmarshal(encoded, &altered); err != nil {
				t.Fatal(err)
			}
			test.mutate(&altered)
			if SemanticRequestIdentityHashForKey(altered, "") == order {
				t.Fatal("altered retirement lineage reused the original request identity")
			}
		})
	}

	ledger.History[0].RetiredAtStateRevision++
	if next := SemanticRequestIdentityHashForKey(ledger, ""); next == order {
		t.Fatalf("next retired generation reused identity %q", next)
	}
}
