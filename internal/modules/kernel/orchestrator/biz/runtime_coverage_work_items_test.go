package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"

	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestEnsureRunCoverageWorkItemsBuildsNeutralBaseline(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-cwi-baseline",
		ExecutionContext: map[string]any{
			"goal": map[string]any{
				"id":   "goal-cwi",
				"text": "### Parent\n- first leaf\n- second leaf",
			},
			"dod": map[string]any{
				"id":    "dod-cwi",
				"items": []string{"first acceptance gate", "container:"},
			},
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"neutral": map[string]any{
						"acceptanceDimensions": []map[string]any{{
							"id":       "dimension-required",
							"required": true,
						}},
						"deliverableExpectations": []map[string]any{{
							"id":       "deliverable-required",
							"required": true,
						}},
					},
				},
			},
		},
	}

	if !newCoverageSubsystem().ensure(run) {
		t.Fatalf("coverage.ensure changed = false, want true")
	}
	items := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])
	if len(items) != 5 {
		t.Fatalf("coverageWorkItems = %+v, want 5 leaf/profile items", items)
	}
	if run.ExecutionContext["goal"].(map[string]any)["text"] != "### Parent\n- first leaf\n- second leaf" {
		t.Fatalf("formal goal was modified: %+v", run.ExecutionContext["goal"])
	}
	if got := stringFromMap(items[0], "targetText"); got != "first leaf" {
		t.Fatalf("first targetText = %q, want first leaf", got)
	}
	for _, item := range items {
		if source := stringFromMap(item, "source"); source == "" || source == "software_dev" || source == "test" {
			t.Fatalf("coverage work item source = %q, want neutral source enum", source)
		}
	}
	context := mapFromValue(run.ExecutionContext[coverageWorkItemContextKey])
	if focus := mapFromValue(context["currentFocus"]); stringFromMap(focus, "id") != "cwi-001" {
		t.Fatalf("currentFocus = %+v, want first stable work item", focus)
	}
	if focuses := mapSliceFromValue(context["currentFocusItems"]); len(focuses) == 0 {
		t.Fatalf("currentFocusItems = %+v, want top-k focus projection", context["currentFocusItems"])
	}
	if ledger := mapSliceFromValue(context["ledgerSummary"]); len(ledger) != len(items) {
		t.Fatalf("ledgerSummary = %+v, want one compact entry per work item", context["ledgerSummary"])
	}
	if excluded := mapSliceFromValue(context["excludedSources"]); len(excluded) != 1 || stringFromMap(excluded[0], "excludedReason") != "container_source" {
		t.Fatalf("excludedSources = %+v, want container exclusion recorded", context["excludedSources"])
	}
}

func TestCoverageWorkItemsUseSameGoalLeavesAsDoDAssessmentGate(t *testing.T) {
	goalText := strings.Join([]string{
		"### Capability Table",
		"| Capability | Parameters | Purpose |",
		"| --- | --- | --- |",
		"| alpha | path | resolve target |",
		"| beta | content | update target |",
	}, "\n")
	run := &runmodel.Run{
		ID: "run-cwi-goal-table",
		ExecutionContext: map[string]any{
			"goal": map[string]any{
				"id":   "goal-cwi-table",
				"text": goalText,
			},
		},
	}

	if !newCoverageSubsystem().ensure(run) {
		t.Fatalf("coverage.ensure changed = false, want table data leaves")
	}
	items := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])
	required := normalizedRunGoalCoverageItems(run)
	if len(items) != len(required) {
		t.Fatalf("coverage work items = %+v, required goal items = %+v, want same count", items, required)
	}
	if len(items) != 2 {
		t.Fatalf("coverage work item count = %d, want 2 data rows", len(items))
	}
	for idx, requiredItem := range required {
		item := items[idx]
		if got := stringFromMap(item, "sourceRequirementId"); got != requiredItem.id {
			t.Fatalf("sourceRequirementId[%d] = %q, want %q", idx, got, requiredItem.id)
		}
		if got := stringFromMap(item, "targetText"); got != requiredItem.raw {
			t.Fatalf("targetText[%d] = %q, want %q", idx, got, requiredItem.raw)
		}
		if strings.Contains(stringFromMap(item, "targetText"), "Capability | Parameters") {
			t.Fatalf("table header was treated as a leaf requirement: %+v", item)
		}
	}
}

func TestBuildConvergenceContractUsesProfileStateChangingEvidenceRequirement(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-cwi-convergence",
		ExecutionContext: map[string]any{
			"goal": map[string]any{"text": "produce stateful outcome"},
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"neutral": map[string]any{
						"evidenceRequirements": []map[string]any{{
							"id":                 "goal-leaf-001",
							"scope":              "goal_coverage",
							"acceptedStepKinds":  []string{"artifact/write"},
							"acceptedOperations": []string{"change"},
						}},
					},
				},
			},
		},
	}
	newCoverageSubsystem().ensure(run)
	input := map[string]any{
		coverageWorkItemContextKey:        run.ExecutionContext[coverageWorkItemContextKey],
		stepInputKeyRuntimeStage:          runtimeStageRepair,
		stepInputKeyRuntimeStageMode:      "strict_convergence",
		stepInputKeyRuntimeRepairFollowUp: true,
		"repairActionPackage": map[string]any{
			"repairTargetSet": []string{"deliverable.txt"},
		},
	}

	contract := BuildConvergenceContract(run, nil, nil, map[string]any{"input": input}, nil)

	if contract.State != convergenceStateNeedStateChange {
		t.Fatalf("contract state = %q, want need_state_change", contract.State)
	}
	if contract.RepairObjective != repairObjectiveStateChangingOutput || !contract.StateChangingOutputRequired {
		t.Fatalf("contract = %+v, want state-changing output for profile-declared evidence requirement", contract)
	}
	pkg := mapFromValue(contract.RepairActionPackage)
	if len(mapFromValue(pkg[coverageWorkItemContextKey])) == 0 {
		t.Fatalf("repairActionPackage = %+v, want coverageWorkItemContext", pkg)
	}
	if targets := mapSliceFromValue(pkg["coverageTargetSet"]); len(targets) == 0 {
		t.Fatalf("repairActionPackage = %+v, want coverageTargetSet", pkg)
	}
	required := mapFromValue(pkg["requiredOutput"])
	if !boolValue(required["mustDeclareCoverageTargets"]) || !boolValue(required["mustIncludeStateChangingNode"]) {
		t.Fatalf("requiredOutput = %+v, want state-changing coverage repair contract", required)
	}
}

func TestCoverageWorkItemsTrackEvidenceByKind(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-cwi-kind",
		ExecutionContext: map[string]any{
			"goal": map[string]any{"text": "stateful outcome"},
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"neutral": map[string]any{
						"evidenceRequirements": []map[string]any{{
							"id":                "goal-leaf-001",
							"scope":             "goal_coverage",
							"acceptedStepKinds": []string{"artifact/write"},
						}},
					},
				},
			},
			coverageWorkItemsKey: []map[string]any{{
				"definitionRevision":     "rev-will-be-recomputed",
				"normalizedSourceKey":    "goal:goal_leaf:goal-leaf-001",
				"status":                 "satisfied",
				"acceptedEvidenceRefs":   []string{"legacy-step-ref"},
				"acceptedEvidenceByKind": map[string]any{"state_change": []string{"run-001/001-change"}},
			}},
		},
	}
	revision := coverageDefinitionRevision(run.ExecutionContext)
	run.ExecutionContext[coverageWorkItemsKey] = []map[string]any{{
		"definitionRevision":     revision,
		"normalizedSourceKey":    "goal:goal_leaf:goal-leaf-001",
		"status":                 "satisfied",
		"acceptedEvidenceRefs":   []string{"legacy-step-ref"},
		"acceptedEvidenceByKind": map[string]any{"state_change": []string{"run-001/001-change"}},
	}}

	if !newCoverageSubsystem().ensure(run) {
		t.Fatalf("coverage.ensure changed = false, want true because validation evidence is still missing")
	}
	items := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])
	if len(items) != 1 {
		t.Fatalf("coverageWorkItems = %+v, want one item", items)
	}
	item := items[0]
	if got := stringFromMap(item, "status"); got != "open" {
		t.Fatalf("status = %q, want open because validation evidence is missing", got)
	}
	missing := stringSet(coverageMissingEvidenceKinds(item))
	if missing["state_change"] || !missing["validation"] {
		t.Fatalf("missingEvidenceKinds = %+v, want only validation missing", coverageMissingEvidenceKinds(item))
	}
	if got := stringFromMap(item, "currentGap"); got != "missing accepted evidence binding for required evidence kind: validation" {
		t.Fatalf("currentGap = %q, want validation-only gap", got)
	}
}

func TestCoverageWorkItemsKeepOldRevisionHistorical(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-cwi-revision",
		ExecutionContext: map[string]any{
			"definitionRevision": "rev-current",
			"goal":               map[string]any{"text": "- current leaf"},
			coverageWorkItemsKey: []map[string]any{{
				"id":                  "cwi-001",
				"definitionRevision":  "rev-old",
				"normalizedSourceKey": "goal:goal_leaf:old",
				"sourceScope":         "goal_leaf",
				"status":              "open",
				"targetText":          "old leaf",
			}},
		},
	}

	if !newCoverageSubsystem().ensure(run) {
		t.Fatalf("coverage.ensure changed = false, want current revision rebuild")
	}
	context := mapFromValue(run.ExecutionContext[coverageWorkItemContextKey])
	if historical := mapSliceFromValue(context["historicalItems"]); len(historical) != 1 ||
		stringFromMap(historical[0], "definitionRevision") != "rev-old" ||
		stringFromMap(historical[0], "status") != "superseded" {
		t.Fatalf("historicalItems = %+v, want old revision superseded summary", context["historicalItems"])
	}
	focus := mapFromValue(context["currentFocus"])
	if stringFromMap(focus, "targetText") != "current leaf" || stringFromMap(focus, "definitionRevision") != "rev-current" {
		t.Fatalf("currentFocus = %+v, want current revision only", focus)
	}
}

func TestCoverageWorkItemsReplanSuccessorPreservesCurrentRevisionLineage(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-cwi-replan-lineage",
		ExecutionContext: map[string]any{
			"definitionRevision": "rev-current",
			"goal":               map[string]any{"text": "stateful outcome"},
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"neutral": map[string]any{
						"evidenceRequirements": []map[string]any{{
							"id":                "goal-leaf-001",
							"scope":             "goal_coverage",
							"acceptedStepKinds": []string{"artifact/write"},
						}},
					},
				},
			},
			coverageWorkItemsKey: []map[string]any{{
				"id":                     "cwi-009",
				"definitionRevision":     "rev-current",
				"normalizedSourceKey":    "goal:goal_leaf:goal-leaf-001",
				"source":                 "goal",
				"sourceScope":            "goal_leaf",
				"sourceRequirementId":    "goal-leaf-001",
				"sourcePath":             "goal.leaves[0]",
				"targetText":             "stateful outcome",
				"status":                 "in_progress",
				"acceptedEvidenceRefs":   []string{"run-prev/001-change"},
				"acceptedEvidenceByKind": map[string]any{"state_change": []string{"run-prev/001-change"}},
				"rejectedEvidenceRefs":   []string{"run-prev/002-verify"},
				"rejectedEvidenceByKind": map[string]any{"validation": []string{"run-prev/002-verify"}},
			}},
		},
	}

	if !newCoverageSubsystem().prepareReplanSuccessor(run) {
		t.Fatalf("coverage.prepareReplanSuccessor changed = false, want normalized successor ledger")
	}
	items := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])
	if len(items) != 1 {
		t.Fatalf("coverageWorkItems = %+v, want one inherited current item", items)
	}
	item := items[0]
	if got := stringFromMap(item, "id"); got != "cwi-001" {
		t.Fatalf("id = %q, want successor-local stable order id", got)
	}
	if got := stringFromMap(item, "status"); got != "in_progress" {
		t.Fatalf("status = %q, want inherited in_progress", got)
	}
	accepted := mapStringSliceValue(item["acceptedEvidenceByKind"])
	if len(accepted["state_change"]) != 1 || accepted["state_change"][0] != "run-prev/001-change" {
		t.Fatalf("acceptedEvidenceByKind = %+v, want state_change lineage", accepted)
	}
	rejected := mapStringSliceValue(item["rejectedEvidenceByKind"])
	if len(rejected["validation"]) != 1 || rejected["validation"][0] != "run-prev/002-verify" {
		t.Fatalf("rejectedEvidenceByKind = %+v, want validation rejection lineage", rejected)
	}
	focus := mapFromValue(mapFromValue(run.ExecutionContext[coverageWorkItemContextKey])["currentFocus"])
	if stringFromMap(focus, "targetText") != "stateful outcome" ||
		!stringSet(stringSliceFromValue(focus["acceptedEvidenceRefs"]))["run-prev/001-change"] ||
		!stringSet(stringSliceFromValue(focus["rejectedEvidenceRefs"]))["run-prev/002-verify"] {
		t.Fatalf("currentFocus = %+v, want inherited evidence lineage on current focus", focus)
	}
}

func TestCoverageWorkItemsRebuildInheritsLineageByNormalizedSourceText(t *testing.T) {
	goalText := strings.Join([]string{
		"| Capability | Parameters | Purpose |",
		"| --- | --- | --- |",
		"| alpha | path | resolve target |",
	}, "\n")
	run := &runmodel.Run{
		ID: "run-cwi-text-lineage",
		ExecutionContext: map[string]any{
			"definitionRevision": "rev-current",
			"goal":               map[string]any{"text": goalText},
			coverageWorkItemsKey: []map[string]any{{
				"id":                     "cwi-009",
				"definitionRevision":     "rev-current",
				"normalizedSourceKey":    "goal:goal_leaf:goal-req-002-legacy",
				"source":                 "goal",
				"sourceScope":            "goal_leaf",
				"sourceRequirementId":    "goal-req-002-legacy",
				"sourcePath":             "goal.leaves[1]",
				"targetText":             "| alpha | path | resolve target |",
				"status":                 "evidence_ready",
				"acceptedEvidenceRefs":   []string{"run-prev/graph/001-alpha"},
				"acceptedEvidenceByKind": map[string]any{"validation": []string{"run-prev/graph/001-alpha"}},
			}},
		},
	}

	if !newCoverageSubsystem().ensure(run) {
		t.Fatalf("coverage.ensure changed = false, want canonical key rebuild")
	}
	items := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])
	if len(items) != 1 {
		t.Fatalf("coverageWorkItems = %+v, want one table data row", items)
	}
	item := items[0]
	if got := stringFromMap(item, "sourceRequirementId"); got == "goal-req-002-legacy" || !strings.HasPrefix(got, "goal-req-001-") {
		t.Fatalf("sourceRequirementId = %q, want rebuilt canonical first data row id", got)
	}
	if refs := stringSliceFromValue(item["acceptedEvidenceRefs"]); len(refs) != 1 || refs[0] != "run-prev/graph/001-alpha" {
		t.Fatalf("acceptedEvidenceRefs = %+v, want inherited lineage by normalized text", refs)
	}
}

func TestCoverageWorkItemsReplanSuccessorMovesStaleSourcesToHistory(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-cwi-replan-stale-source",
		ExecutionContext: map[string]any{
			"definitionRevision": "rev-current",
			"goal":               map[string]any{"text": "current leaf"},
			coverageWorkItemsKey: []map[string]any{{
				"id":                  "cwi-001",
				"definitionRevision":  "rev-current",
				"normalizedSourceKey": "goal:goal_leaf:stale-leaf",
				"source":              "goal",
				"sourceScope":         "goal_leaf",
				"sourceRequirementId": "stale-leaf",
				"sourcePath":          "goal.leaves[9]",
				"targetText":          "stale leaf",
				"status":              "open",
			}},
			coverageWorkItemContextKey: map[string]any{
				"historicalItems": []map[string]any{{
					"id":                 "cwi-000",
					"definitionRevision": "rev-older",
					"historicalReason":   "already_historical",
				}},
			},
		},
	}

	if !newCoverageSubsystem().prepareReplanSuccessor(run) {
		t.Fatalf("coverage.prepareReplanSuccessor changed = false, want stale source historical isolation")
	}
	items := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])
	if len(items) != 1 || stringFromMap(items[0], "targetText") != "current leaf" {
		t.Fatalf("coverageWorkItems = %+v, want only current source active", items)
	}
	context := mapFromValue(run.ExecutionContext[coverageWorkItemContextKey])
	focus := mapFromValue(context["currentFocus"])
	if stringFromMap(focus, "targetText") != "current leaf" {
		t.Fatalf("currentFocus = %+v, want current source only", focus)
	}
	historical := mapSliceFromValue(context["historicalItems"])
	if len(historical) != 2 {
		t.Fatalf("historicalItems = %+v, want preserved previous history plus stale source", historical)
	}
	var foundStale bool
	for _, item := range historical {
		if stringFromMap(item, "historicalReason") == "superseded_coverage_source" &&
			stringFromMap(item, "definitionRevision") == "rev-current" {
			foundStale = true
		}
	}
	if !foundStale {
		t.Fatalf("historicalItems = %+v, want stale current-revision source marked superseded_coverage_source", historical)
	}
}

func TestCoverageWorkItemsHistoricalSupersededSourcesAreStable(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-cwi-stable-history",
		ExecutionContext: map[string]any{
			"definitionRevision": "rev-current",
			"goal":               map[string]any{"text": "current leaf"},
			coverageWorkItemsKey: []map[string]any{
				{
					"id":                  "cwi-003",
					"definitionRevision":  "rev-current",
					"normalizedSourceKey": "goal:goal_leaf:stale-c",
					"source":              "goal",
					"sourceScope":         "goal_leaf",
					"sourceRequirementId": "stale-c",
					"targetText":          "stale C",
					"status":              "open",
				},
				{
					"id":                  "cwi-002",
					"definitionRevision":  "rev-current",
					"normalizedSourceKey": "goal:goal_leaf:stale-b",
					"source":              "goal",
					"sourceScope":         "goal_leaf",
					"sourceRequirementId": "stale-b",
					"targetText":          "stale B",
					"status":              "open",
				},
			},
		},
	}

	if !newCoverageSubsystem().ensure(run) {
		t.Fatalf("coverage.ensure changed = false, want stale sources moved to history")
	}
	historical := mapSliceFromValue(mapFromValue(run.ExecutionContext[coverageWorkItemContextKey])["historicalItems"])
	if len(historical) != 2 {
		t.Fatalf("historicalItems = %+v, want two stale source entries", historical)
	}
	if got := []string{stringFromMap(historical[0], "sourceRequirementId"), stringFromMap(historical[1], "sourceRequirementId")}; got[0] != "stale-b" || got[1] != "stale-c" {
		t.Fatalf("historical source order = %+v, want stable normalized source key order", got)
	}
}

func TestCoverageWorkItemContextDoesNotFocusSkippedItems(t *testing.T) {
	revision := "rev-current"
	items := []map[string]any{
		{
			"id":                    "cwi-001",
			"definitionRevision":    revision,
			"source":                "goal",
			"sourceScope":           "goal_leaf",
			"sourceRequirementId":   "goal-leaf-001",
			"sourcePath":            "goal.leaves[0]",
			"targetText":            "skipped leaf",
			"normalizedSourceKey":   "goal:goal_leaf:goal-leaf-001",
			"requiredEvidenceKinds": []string{"validation"},
			"status":                "skipped",
			"reason":                "explicitly excluded by governance",
		},
		{
			"id":                    "cwi-002",
			"definitionRevision":    revision,
			"source":                "goal",
			"sourceScope":           "goal_leaf",
			"sourceRequirementId":   "goal-leaf-002",
			"sourcePath":            "goal.leaves[1]",
			"targetText":            "open leaf",
			"normalizedSourceKey":   "goal:goal_leaf:goal-leaf-002",
			"requiredEvidenceKinds": []string{"validation"},
			"status":                "open",
		},
	}

	context := coverageWorkItemContextFromItems(revision, items, nil, nil)

	focus := mapFromValue(context["currentFocus"])
	if stringFromMap(focus, "id") != "cwi-002" {
		t.Fatalf("currentFocus = %+v, want open item instead of skipped item", focus)
	}
	unresolved := mapSliceFromValue(context["unresolvedItems"])
	if len(unresolved) != 1 || stringFromMap(unresolved[0], "id") != "cwi-002" {
		t.Fatalf("unresolvedItems = %+v, want skipped item excluded", unresolved)
	}
}

func TestCoverageWorkItemContextPrioritizesStateChangingCoverageItem(t *testing.T) {
	revision := "rev-current"
	items := []map[string]any{
		{
			"id":                    "cwi-001",
			"definitionRevision":    revision,
			"source":                "goal",
			"sourceScope":           "goal_leaf",
			"sourceRequirementId":   "goal-leaf-001",
			"sourcePath":            "goal.leaves[0]",
			"targetText":            "observable behavior",
			"normalizedSourceKey":   "goal:goal_leaf:goal-leaf-001",
			"requiredEvidenceKinds": []string{"validation"},
			"status":                "open",
		},
		{
			"id":                    "cwi-002",
			"definitionRevision":    revision,
			"source":                "contract",
			"sourceScope":           "contract_item",
			"sourceRequirementId":   "stateful-output",
			"sourcePath":            "executionContract.profileContracts.neutral.requirements[0]",
			"targetText":            "stateful output",
			"normalizedSourceKey":   "contract:contract_item:stateful-output",
			"requiredEvidenceKinds": []string{"validation", "state_change"},
			"status":                "open",
		},
	}

	context := coverageWorkItemContextFromItems(revision, items, nil, nil)

	focus := mapFromValue(context["currentFocus"])
	if stringFromMap(focus, "id") != "cwi-002" {
		t.Fatalf("currentFocus = %+v, want state-changing item", focus)
	}
	focuses := mapSliceFromValue(context["currentFocusItems"])
	if len(focuses) < 2 || stringFromMap(focuses[0], "id") != "cwi-002" || stringFromMap(focuses[1], "id") != "cwi-001" {
		t.Fatalf("currentFocusItems = %+v, want state-changing item before validation-only goal", focuses)
	}
	ledger := mapSliceFromValue(context["ledgerSummary"])
	if len(ledger) != 2 || stringFromMap(ledger[0], "id") != "cwi-001" || stringFromMap(ledger[1], "id") != "cwi-002" {
		t.Fatalf("ledgerSummary = %+v, want original ledger order preserved", ledger)
	}
}

func TestCoverageWorkItemsRequireStateChangeForRequiredCoverageWhenRunRequiresStateChange(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-cwi-source-change-state",
		ExecutionContext: map[string]any{
			"goal": map[string]any{
				"id":   "goal-cwi-source-change",
				"text": "deliver observable behavior",
			},
			"executionContract": map[string]any{
				"stateChangePolicy": "must_produce_state_change",
				"profileContracts": map[string]any{
					"neutral": map[string]any{
						"deliverableExpectations": []map[string]any{
							{"id": "primary-output", "required": true, "evidenceRole": "primary_state_change"},
							{"id": "supporting-evidence", "required": true, "evidenceRole": "supplemental_evidence"},
						},
					},
				},
			},
		},
	}

	if !newCoverageSubsystem().ensure(run) {
		t.Fatalf("coverage.ensure changed = false, want baseline")
	}
	var primaryOutput map[string]any
	for _, item := range mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey]) {
		if stringFromMap(item, "sourceRequirementId") == "primary-output" {
			primaryOutput = item
			break
		}
	}
	if len(primaryOutput) == 0 {
		t.Fatalf("coverageWorkItems = %+v, want primary-output item", run.ExecutionContext[coverageWorkItemsKey])
	}
	missing := stringSet(coverageMissingEvidenceKinds(primaryOutput))
	if !missing["state_change"] || !missing["validation"] {
		t.Fatalf("primary-output missingEvidenceKinds = %+v, want state_change and validation", coverageMissingEvidenceKinds(primaryOutput))
	}
	context := mapFromValue(run.ExecutionContext[coverageWorkItemContextKey])
	focus := mapFromValue(context["currentFocus"])
	if stringFromMap(focus, "sourceRequirementId") != "primary-output" {
		t.Fatalf("currentFocus = %+v, want primary-output before validation-only items", focus)
	}
	for _, item := range mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey]) {
		if stringFromMap(item, "sourceRequirementId") != "supporting-evidence" {
			continue
		}
		missing := stringSet(coverageMissingEvidenceKinds(item))
		if missing["state_change"] || !missing["validation"] {
			t.Fatalf("supporting-evidence missingEvidenceKinds = %+v, want validation only", coverageMissingEvidenceKinds(item))
		}
		return
	}
	t.Fatalf("coverageWorkItems = %+v, want supporting-evidence item", run.ExecutionContext[coverageWorkItemsKey])
}

func TestCoverageWorkItemContextPrioritizesFailedItemsOverEvidenceReadyTopK(t *testing.T) {
	revision := "rev-current"
	items := []map[string]any{
		{
			"id":                     "cwi-001",
			"definitionRevision":     revision,
			"source":                 "goal",
			"sourceScope":            "goal_leaf",
			"sourceRequirementId":    "goal-leaf-001",
			"sourcePath":             "goal.leaves[0]",
			"targetText":             "first ready leaf",
			"normalizedSourceKey":    "goal:goal_leaf:goal-leaf-001",
			"requiredEvidenceKinds":  []string{"validation"},
			"status":                 "evidence_ready",
			"acceptedEvidenceByKind": map[string]any{"validation": []string{"run/graph/001"}},
			"acceptedEvidenceRefs":   []string{"run/graph/001"},
		},
		{
			"id":                     "cwi-002",
			"definitionRevision":     revision,
			"source":                 "goal",
			"sourceScope":            "goal_leaf",
			"sourceRequirementId":    "goal-leaf-002",
			"sourcePath":             "goal.leaves[1]",
			"targetText":             "second ready leaf",
			"normalizedSourceKey":    "goal:goal_leaf:goal-leaf-002",
			"requiredEvidenceKinds":  []string{"validation"},
			"status":                 "evidence_ready",
			"acceptedEvidenceByKind": map[string]any{"validation": []string{"run/graph/002"}},
			"acceptedEvidenceRefs":   []string{"run/graph/002"},
		},
		{
			"id":                     "cwi-003",
			"definitionRevision":     revision,
			"source":                 "goal",
			"sourceScope":            "goal_leaf",
			"sourceRequirementId":    "goal-leaf-003",
			"sourcePath":             "goal.leaves[2]",
			"targetText":             "third ready leaf",
			"normalizedSourceKey":    "goal:goal_leaf:goal-leaf-003",
			"requiredEvidenceKinds":  []string{"validation"},
			"status":                 "evidence_ready",
			"acceptedEvidenceByKind": map[string]any{"validation": []string{"run/graph/003"}},
			"acceptedEvidenceRefs":   []string{"run/graph/003"},
		},
		{
			"id":                     "cwi-004",
			"definitionRevision":     revision,
			"source":                 "dod",
			"sourceScope":            "dod_item",
			"sourceRequirementId":    "dod-item-004",
			"sourcePath":             "dod.items[3]",
			"targetText":             "failed validation leaf",
			"normalizedSourceKey":    "dod:dod_item:dod-item-004",
			"requiredEvidenceKinds":  []string{"validation"},
			"status":                 "failed",
			"currentGap":             "latest validation evidence was rejected",
			"acceptedEvidenceByKind": map[string]any{"validation": []string{"run/graph/004-precheck"}},
			"acceptedEvidenceRefs":   []string{"run/graph/004-precheck"},
			"rejectedEvidenceByKind": map[string]any{"validation": []string{"run/graph/004-verify"}},
			"rejectedEvidenceRefs":   []string{"run/graph/004-verify"},
		},
	}

	context := coverageWorkItemContextFromItems(revision, items, nil, nil)

	focus := mapFromValue(context["currentFocus"])
	if stringFromMap(focus, "id") != "cwi-004" {
		t.Fatalf("currentFocus = %+v, want failed item before evidence_ready items", focus)
	}
	if states := stringSliceFromValue(focus["nextAllowedConvergenceStates"]); !stringSliceContains(states, convergenceStateNeedValidation) {
		t.Fatalf("currentFocus.nextAllowedConvergenceStates = %+v, want need_validation", states)
	}
	targets := coverageTargetSetFromWorkItemContext(context)
	if len(targets) == 0 || stringFromMap(targets[0], "coverageWorkItemId") != "cwi-004" {
		t.Fatalf("coverageTargetSet = %+v, want failed focus first", targets)
	}
	ledger := mapSliceFromValue(context["ledgerSummary"])
	if len(ledger) != 4 || stringFromMap(ledger[0], "id") != "cwi-001" || stringFromMap(ledger[3], "id") != "cwi-004" {
		t.Fatalf("ledgerSummary = %+v, want original source order preserved", ledger)
	}
}

func TestCoverageWorkItemsPrioritizeMissingEvidenceBeforeAssessmentPendingWithRejectedHistory(t *testing.T) {
	revision := "rev-focus-missing"
	items := []map[string]any{
		{
			"id":                     "cwi-001",
			"definitionRevision":     revision,
			"source":                 "goal",
			"sourceScope":            "goal_leaf",
			"sourceRequirementId":    "goal-req-001",
			"sourcePath":             "goal.leaves[0]",
			"targetText":             "missing validation leaf",
			"normalizedSourceKey":    "goal:goal_leaf:goal-req-001",
			"requiredEvidenceKinds":  []string{"validation"},
			"status":                 "open",
			"acceptedEvidenceByKind": map[string]any{},
		},
		{
			"id":                     "cwi-002",
			"definitionRevision":     revision,
			"source":                 "dod",
			"sourceScope":            "dod_item",
			"sourceRequirementId":    "dod-item-001",
			"sourcePath":             "dod.items[0]",
			"targetText":             "assessment pending leaf",
			"normalizedSourceKey":    "dod:dod_item:dod-item-001",
			"requiredEvidenceKinds":  []string{"validation"},
			"status":                 "evidence_ready",
			"acceptedEvidenceByKind": map[string]any{"validation": []string{"run/graph/001-ok"}},
			"acceptedEvidenceRefs":   []string{"run/graph/001-ok"},
			"rejectedEvidenceByKind": map[string]any{"validation": []string{"run/graph/000-old-fail"}},
			"rejectedEvidenceRefs":   []string{"run/graph/000-old-fail"},
		},
	}

	context := coverageWorkItemContextFromItems(revision, items, nil, nil)

	focus := mapFromValue(context["currentFocus"])
	if stringFromMap(focus, "id") != "cwi-001" {
		t.Fatalf("currentFocus = %+v, want missing evidence item before assessment-pending item with old rejected refs", focus)
	}
}

func TestCoverageTargetSetSkipsUnlinkedGoalCoverageFailureWhenCatalogPresent(t *testing.T) {
	context := coverageWorkItemContextFromItems("rev-current", []map[string]any{{
		"id":                    "cwi-001",
		"definitionRevision":    "rev-current",
		"source":                "goal",
		"sourceScope":           "goal_leaf",
		"sourceRequirementId":   "goal-req-001-alpha",
		"sourcePath":            "goal.leaves[0]",
		"targetText":            "alpha requirement",
		"normalizedSourceKey":   "goal:goal_leaf:goal-req-001-alpha",
		"requiredEvidenceKinds": []string{"validation"},
		"status":                "open",
	}}, nil, nil)
	input := map[string]any{
		"missingGoalCoverage": []map[string]any{{
			"goalRequirementId": "goal-req-unlinked",
			"item":              "generic coverage mapping was incomplete",
			"state":             "failed",
		}},
		coverageWorkItemContextKey: context,
	}

	targets := coverageTargetSetFromRepairInput(input)

	if len(targets) != 1 {
		t.Fatalf("coverageTargetSet = %+v, want only catalog-backed goal item", targets)
	}
	if got := stringFromMap(targets[0], "sourceRequirementId"); got != "goal-req-001-alpha" {
		t.Fatalf("coverageTargetSet = %+v, want unlinked failed goalCoverage item skipped", targets)
	}
}

func TestCoverageWorkItemsAdvanceFromStepAndAssessmentEvidence(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-cwi-advance",
		ExecutionContext: map[string]any{
			"goal": map[string]any{"text": "stateful outcome"},
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"neutral": map[string]any{
						"evidenceRequirements": []map[string]any{{
							"id":                "goal-leaf-001",
							"scope":             "goal_coverage",
							"acceptedStepKinds": []string{"artifact/write"},
						}},
					},
				},
			},
		},
	}
	if !newCoverageSubsystem().ensure(run) {
		t.Fatalf("coverage.ensure changed = false, want baseline")
	}
	changeStep := &runmodel.Step{
		ID:    "run-cwi-advance/001-change",
		RunID: run.ID,
		Kind:  "artifact/write",
		State: common.StepStateCompleted,
		Output: map[string]any{
			"coverageTargets": []string{"stateful outcome"},
		},
	}
	if !newCoverageSubsystem().updateFromCompletedStep(run, changeStep) {
		t.Fatalf("coverage.updateFromCompletedStep changed = false, want state_change evidence update")
	}
	item := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])[0]
	if got := stringFromMap(item, "status"); got != "in_progress" {
		t.Fatalf("status after state change = %q, want in_progress", got)
	}
	missing := stringSet(coverageMissingEvidenceKinds(item))
	if missing["state_change"] || !missing["validation"] {
		t.Fatalf("missing after state change = %+v, want validation only", coverageMissingEvidenceKinds(item))
	}

	assessmentStep := &runmodel.Step{
		ID:    "run-cwi-advance/002-assess",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateCompleted,
	}
	assessment := map[string]any{
		"goalCoverage": map[string]any{
			"items": []map[string]any{{
				"goalRequirementId": "goal-leaf-001",
				"goal":              "stateful outcome",
				"state":             "satisfied",
				"evidenceRefs":      []string{"run-cwi-advance/002-assess/validation/runtime-step"},
			}},
		},
	}
	if !newCoverageSubsystem().updateFromAssessment(run, assessmentStep, assessment) {
		t.Fatalf("coverage.updateFromAssessment changed = false, want validation closure")
	}
	item = mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])[0]
	if got := stringFromMap(item, "status"); got != "satisfied" {
		t.Fatalf("status after assessment = %q, want satisfied", got)
	}
	if missing := coverageMissingEvidenceKinds(item); len(missing) != 0 {
		t.Fatalf("missing after assessment = %+v, want none", missing)
	}
}

func TestCoverageWorkItemsMatchStructuredSourceRequirementID(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-cwi-source-requirement-id",
		ExecutionContext: map[string]any{
			"goal": map[string]any{"text": "**Goal**: keep evidence aligned"},
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"neutral": map[string]any{
						"evidenceRequirements": []map[string]any{{
							"id":                "goal-leaf-001",
							"scope":             "goal_coverage",
							"acceptedStepKinds": []string{"mcp/tool_call"},
						}},
					},
				},
			},
		},
	}
	if !newCoverageSubsystem().ensure(run) {
		t.Fatalf("coverage.ensure changed = false, want baseline")
	}
	step := &runmodel.Step{
		ID:    "run-cwi-source-requirement-id/001-verify",
		RunID: run.ID,
		Kind:  "mcp/tool_call",
		State: common.StepStateCompleted,
		Input: map[string]any{
			"coverageTargets": []map[string]any{{
				"sourceRequirementId": "goal-leaf-001",
				"sourceScope":         "goal_leaf",
				"targetText":          "Goal: keep evidence aligned",
			}},
			"validationEvidence": map[string]any{
				"mode":            "runtime_behavior",
				"observationType": "runtime_observation",
			},
		},
		Output: map[string]any{"stage": "mcp_tool_call_completed"},
	}

	if !newCoverageSubsystem().updateFromCompletedStep(run, step) {
		t.Fatalf("coverage.updateFromCompletedStep changed = false, want sourceRequirementId-linked evidence")
	}
	item := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])[0]
	if got := stringFromMap(item, "status"); got != "evidence_ready" {
		t.Fatalf("status = %q, want evidence_ready", got)
	}
	if refs := mapStringSliceValue(item["acceptedEvidenceByKind"])["validation"]; len(refs) == 0 {
		t.Fatalf("acceptedEvidenceByKind = %+v, want validation refs", item["acceptedEvidenceByKind"])
	}
}

func TestCoverageWorkItemsRejectUnclosedAssessmentEvidence(t *testing.T) {
	run := &runmodel.Run{
		ID:               "run-cwi-reject",
		ExecutionContext: map[string]any{"dod": map[string]any{"items": []string{"acceptance gate"}}},
	}
	newCoverageSubsystem().ensure(run)
	step := &runmodel.Step{ID: "run-cwi-reject/001-assess", RunID: run.ID, State: common.StepStateCompleted}
	assessment := map[string]any{
		"items": []map[string]any{{
			"item":          "acceptance gate",
			"state":         "failed",
			"failureReason": "validation evidence did not close the item",
			"evidenceRefs":  []string{"run-cwi-reject/001-verify"},
		}},
	}

	if !newCoverageSubsystem().updateFromAssessment(run, step, assessment) {
		t.Fatalf("coverage.updateFromAssessment changed = false, want rejected evidence")
	}
	item := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])[0]
	if got := stringFromMap(item, "status"); got != "failed" {
		t.Fatalf("status = %q, want failed", got)
	}
	rejected := mapStringSliceValue(item["rejectedEvidenceByKind"])
	if len(rejected["validation"]) != 1 || rejected["validation"][0] != "run-cwi-reject/001-verify" {
		t.Fatalf("rejectedEvidenceByKind = %+v, want validation rejected ref", rejected)
	}
	if got := stringFromMap(item, "currentGap"); got != "validation evidence did not close the item" {
		t.Fatalf("currentGap = %q, want failure reason", got)
	}
}

func TestCoverageWorkItemsRejectUnlinkedSatisfiedAssessment(t *testing.T) {
	run := &runmodel.Run{
		ID:               "run-cwi-unlinked-assessment",
		ExecutionContext: map[string]any{"dod": map[string]any{"items": []string{"acceptance gate"}}},
	}
	newCoverageSubsystem().ensure(run)
	step := &runmodel.Step{ID: "run-cwi-unlinked-assessment/001-assess", RunID: run.ID, State: common.StepStateCompleted}
	assessment := map[string]any{
		"items": []map[string]any{{
			"item":         "unrelated satisfied claim",
			"state":        "satisfied",
			"evidenceRefs": []string{"run-cwi-unlinked-assessment/001-assess/validation/runtime-step"},
		}},
	}

	if !newCoverageSubsystem().updateFromAssessment(run, step, assessment) {
		t.Fatalf("coverage.updateFromAssessment changed=false, want rejected unlinked assessment")
	}
	item := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])[0]
	if got := stringFromMap(item, "status"); got != "failed" {
		t.Fatalf("status = %q, want failed", got)
	}
	if got := stringFromMap(item, "currentGap"); got != "satisfied assessment item did not relink to any active coverage work item" {
		t.Fatalf("currentGap = %q, want unlinked assessment reason", got)
	}
}

func TestCoverageWorkItemsRecordFailedStepRejectedEvidence(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-cwi-failed-step",
		ExecutionContext: map[string]any{
			"goal": map[string]any{"text": "stateful outcome"},
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"neutral": map[string]any{
						"evidenceRequirements": []map[string]any{{
							"id":                "goal-leaf-001",
							"scope":             "goal_coverage",
							"acceptedStepKinds": []string{"artifact/write"},
						}},
					},
				},
			},
		},
	}
	newCoverageSubsystem().ensure(run)
	step := &runmodel.Step{
		ID:    "run-cwi-failed-step/001-change",
		RunID: run.ID,
		Kind:  "artifact/write",
		State: common.StepStateFailed,
		Output: map[string]any{
			"coverageTargets": []string{"stateful outcome"},
			"error":           "state-changing evidence failed",
		},
	}

	if !newCoverageSubsystem().updateFromFailedStep(run, step, "") {
		t.Fatalf("coverage.updateFromFailedStep changed = false, want rejected state_change evidence")
	}
	item := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])[0]
	if got := stringFromMap(item, "status"); got != "failed" {
		t.Fatalf("status = %q, want failed", got)
	}
	rejected := mapStringSliceValue(item["rejectedEvidenceByKind"])
	if len(rejected["state_change"]) == 0 {
		t.Fatalf("rejectedEvidenceByKind = %+v, want rejected state_change evidence", rejected)
	}
	if got := stringFromMap(item, "currentGap"); got != "state-changing evidence failed" {
		t.Fatalf("currentGap = %q, want failure reason", got)
	}
}

func TestCoverageWorkItemsRejectionDoesNotDemoteCwiWithAcceptedEvidence(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-cwi-accepted-then-rejected",
		ExecutionContext: map[string]any{
			"goal": map[string]any{"text": "实现 基础 转发 请求 和 预期 回复 通过 go-build-verify"},
		},
	}
	c := newCoverageSubsystem()
	c.ensure(run)
	items := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])
	if len(items) == 0 {
		t.Fatalf("expected cwi to be built from goal")
	}

	// First, accept a validation evidence via a successful shell step — moves cwi to evidence_ready.
	acceptStep := &runmodel.Step{
		ID:    "run-cwi-accepted-then-rejected/graph-001/001-go-build-verify",
		RunID: run.ID,
		Kind:  "shell/command",
		State: common.StepStateCompleted,
		Output: map[string]any{
			"exitCode":        0,
			"command":         "go",
			"coverageTargets": []string{"go-build-verify"},
		},
	}
	if !c.updateFromCompletedStep(run, acceptStep) {
		t.Fatalf("updateFromCompletedStep changed=false on first accept")
	}
	items = mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])
	priorCwi := items[0]
	if got := stringFromMap(priorCwi, "status"); got != "evidence_ready" {
		t.Fatalf("after accept: status = %q, want evidence_ready", got)
	}

	// Then a destructive shell step fails — its rejection is recorded, but the
	// cwi already has accepted validation covering all required kinds, so the
	// status must NOT be demoted to "failed".
	rejectStep := &runmodel.Step{
		ID:    "run-cwi-accepted-then-rejected/graph-001/002-go-build-verify",
		RunID: run.ID,
		Kind:  "shell/command",
		State: common.StepStateFailed,
		Output: map[string]any{
			"exitCode":        1,
			"command":         "curl",
			"coverageTargets": []string{"go-build-verify"},
			"error":           "destructive shell script is not allowed",
		},
	}
	if !c.updateFromFailedStep(run, rejectStep, "") {
		t.Fatalf("updateFromFailedStep changed=false on rejection")
	}
	items = mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])
	afterReject := items[0]
	if got := stringFromMap(afterReject, "status"); got == "failed" {
		t.Fatalf("after rejection: status = %q, want NOT demoted to failed when accepted evidence covers all required kinds", got)
	}
	if got := stringFromMap(afterReject, "status"); got != "evidence_ready" {
		t.Fatalf("after rejection: status = %q, want evidence_ready preserved", got)
	}
	rejected := mapStringSliceValue(afterReject["rejectedEvidenceByKind"])
	if len(rejected["validation"]) == 0 {
		t.Fatalf("rejectedEvidenceByKind = %+v, want rejection lineage preserved even without demotion", rejected)
	}
	accepted := mapStringSliceValue(afterReject["acceptedEvidenceByKind"])
	if len(accepted["validation"]) == 0 {
		t.Fatalf("acceptedEvidenceByKind = %+v, want accepted validation refs preserved", accepted)
	}
}

func TestCoverageWorkItemsRejectionWithPartialAcceptedSetsInProgress(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-cwi-partial-accepted",
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"stateChangePolicy": "must_produce_state_change",
				"profileContracts": map[string]any{
					"software-dev": map[string]any{
						"deliverableExpectations": []map[string]any{
							{"id": "source_change", "required": true, "evidenceRole": "primary_state_change"},
						},
					},
				},
			},
		},
	}
	c := newCoverageSubsystem()
	c.ensure(run)
	items := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])
	if len(items) == 0 {
		t.Fatalf("expected cwi to be built from deliverable expectations")
	}
	// Find the deliverable cwi (evidenceRole=primary_state_change, required=state_change+validation)
	var cwiIdx int = -1
	for i, item := range items {
		if role := normalizedCoverageEvidenceRole(stringFromMap(item, "evidenceRole")); role == "primary_state_change" {
			cwiIdx = i
			break
		}
	}
	if cwiIdx < 0 {
		t.Fatalf("no primary_state_change cwi found; items=%+v", items)
	}
	if req := items[cwiIdx]["requiredEvidenceKinds"]; len(stringSliceFromValue(req)) < 2 {
		t.Fatalf("expected cwi to require both validation+state_change, got %v", req)
	}

	// First, accept a state_change evidence via artifact/write. targetText for the
	// deliverable cwi is "source_change" (or the deliverable id), so use a
	// coverageTarget that matches.
	stateChangeStep := &runmodel.Step{
		ID:     "run-cwi-partial-accepted/graph-001/001-source-change",
		RunID:  run.ID,
		Kind:   "artifact/write",
		State:  common.StepStateCompleted,
		Output: map[string]any{"coverageTargets": []string{"source_change"}},
	}
	if !c.updateFromCompletedStep(run, stateChangeStep) {
		t.Fatalf("updateFromCompletedStep changed=false on state_change")
	}
	items = mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])
	priorCwi := items[cwiIdx]
	if got := stringFromMap(priorCwi, "status"); got != "in_progress" {
		t.Fatalf("after partial bind: status = %q, want in_progress", got)
	}

	// Now a failed step tries to bind validation (the missing kind) but fails
	// (e.g. destructive shell). Since state_change is already bound, the cwi is
	// in progress, not failed. The rejection is recorded but does not demote.
	failedStep := &runmodel.Step{
		ID:    "run-cwi-partial-accepted/graph-001/002-source-change-verify",
		RunID: run.ID,
		Kind:  "shell/command",
		State: common.StepStateFailed,
		Output: map[string]any{
			"exitCode":        1,
			"command":         "curl",
			"coverageTargets": []string{"source_change"},
			"error":           "destructive shell script is not allowed",
		},
	}
	if !c.updateFromFailedStep(run, failedStep, "") {
		t.Fatalf("updateFromFailedStep changed=false on rejection")
	}
	items = mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])
	afterReject := items[cwiIdx]
	if got := stringFromMap(afterReject, "status"); got == "failed" {
		t.Fatalf("after rejection with partial accepted: status = %q, want NOT failed (state_change is bound)", got)
	}
	accepted := mapStringSliceValue(afterReject["acceptedEvidenceByKind"])
	if len(accepted["state_change"]) == 0 {
		t.Fatalf("acceptedEvidenceByKind.state_change must be preserved, got %+v", accepted)
	}
	rejected := mapStringSliceValue(afterReject["rejectedEvidenceByKind"])
	if len(rejected["validation"]) == 0 {
		t.Fatalf("rejectedEvidenceByKind.validation must record the rejection, got %+v", rejected)
	}
}

func TestCoverageWorkItemsAcceptanceGateRequiresSatisfiedItems(t *testing.T) {
	run := &runmodel.Run{
		ID:               "run-cwi-acceptance-gate",
		ExecutionContext: map[string]any{"dod": map[string]any{"items": []string{"acceptance gate"}}},
	}
	newCoverageSubsystem().ensure(run)

	err := newCoverageSubsystem().validateReadyForAcceptance(run)

	if err == nil || !strings.Contains(err.Error(), "coverageWorkItems not ready") {
		t.Fatalf("coverage.validateReadyForAcceptance err = %v, want unresolved work item failure", err)
	}
}

func TestCoverageWorkItemsAcceptanceGateAllowsSatisfiedEvidence(t *testing.T) {
	run := &runmodel.Run{
		ID:               "run-cwi-acceptance-ready",
		ExecutionContext: map[string]any{"dod": map[string]any{"items": []string{"acceptance gate"}}},
	}
	newCoverageSubsystem().ensure(run)
	step := &runmodel.Step{ID: "run-cwi-acceptance-ready/001-assess", RunID: run.ID, State: common.StepStateCompleted}
	assessment := map[string]any{
		"items": []map[string]any{{
			"item":         "acceptance gate",
			"state":        "satisfied",
			"evidenceRefs": []string{"run-cwi-acceptance-ready/001-assess/validation/runtime-step"},
		}},
	}
	if !newCoverageSubsystem().updateFromAssessment(run, step, assessment) {
		t.Fatalf("coverage.updateFromAssessment changed=false, want satisfied closure")
	}

	if err := newCoverageSubsystem().validateReadyForAcceptance(run); err != nil {
		t.Fatalf("coverage.validateReadyForAcceptance: %v", err)
	}
}

func TestCoverageWorkItemsGovernanceItemsRequireOwnerEvidence(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-cwi-governance-kinds",
		ExecutionContext: map[string]any{
			"dod": map[string]any{"items": []string{
				"two Decision tickets are resolved",
				"one Approval is approved",
				"MCP loadtest.governed_result evidence is collected",
			}},
		},
	}
	newCoverageSubsystem().ensure(run)
	items := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])
	if len(items) != 3 {
		t.Fatalf("coverageWorkItems len = %d, want 3", len(items))
	}
	assertRequired := func(index int, want string) {
		t.Helper()
		got := stringSliceFromValue(items[index]["requiredEvidenceKinds"])
		if len(got) != 1 || got[0] != want {
			t.Fatalf("item[%d] requiredEvidenceKinds = %+v, want [%s]", index, got, want)
		}
	}
	assertRequired(0, "decision")
	assertRequired(1, "approval")
	assertRequired(2, "validation")
}

func TestCoverageWorkItemsGovernanceItemsRejectMCPValidationEvidence(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-cwi-governance-reject-mcp",
		ExecutionContext: map[string]any{
			"dod": map[string]any{"items": []string{
				"two Decision tickets are resolved",
				"one Approval is approved",
				"MCP loadtest.governed_result evidence is collected",
			}},
		},
	}
	newCoverageSubsystem().ensure(run)
	assess := &runmodel.Step{ID: "run-cwi-governance-reject-mcp/001-assess", RunID: run.ID, State: common.StepStateCompleted}
	assessment := map[string]any{
		"items": []map[string]any{
			{
				"item":         "two Decision tickets are resolved",
				"state":        "satisfied",
				"evidenceRefs": []string{"run-cwi-governance-reject-mcp/graph-001/001-mcp-call"},
			},
			{
				"item":         "one Approval is approved",
				"state":        "satisfied",
				"evidenceRefs": []string{"run-cwi-governance-reject-mcp/graph-001/001-mcp-call"},
			},
			{
				"item":         "MCP loadtest.governed_result evidence is collected",
				"state":        "satisfied",
				"evidenceRefs": []string{"run-cwi-governance-reject-mcp/graph-001/001-mcp-call"},
			},
		},
	}
	if !newCoverageSubsystem().updateFromAssessment(run, assess, assessment) {
		t.Fatalf("coverage.updateFromAssessment changed=false, want MCP validation item closure")
	}
	items := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])
	if got := stringFromMap(items[0], "status"); got != "open" {
		t.Fatalf("decision item status = %q, want open", got)
	}
	if got := stringFromMap(items[1], "status"); got != "open" {
		t.Fatalf("approval item status = %q, want open", got)
	}
	if got := stringFromMap(items[2], "status"); got != "satisfied" {
		t.Fatalf("validation item status = %q, want satisfied", got)
	}
	err := newCoverageSubsystem().validateReadyForAcceptance(run)
	if err == nil || !strings.Contains(err.Error(), "cwi-001:decision") || !strings.Contains(err.Error(), "cwi-002:approval") {
		t.Fatalf("coverage.validateReadyForAcceptance err = %v, want decision/approval unresolved", err)
	}
}

func TestCoverageWorkItemsGovernanceItemsAcceptOwnerEvidenceRefs(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-cwi-governance-owner-evidence",
		ExecutionContext: map[string]any{
			"dod": map[string]any{"items": []string{
				"two Decision tickets are resolved",
				"one Approval is approved",
				"MCP loadtest.governed_result evidence is collected",
			}},
		},
	}
	newCoverageSubsystem().ensure(run)
	assess := &runmodel.Step{ID: "run-cwi-governance-owner-evidence/001-assess", RunID: run.ID, State: common.StepStateCompleted}
	assessment := map[string]any{
		"items": []map[string]any{
			{
				"item":         "two Decision tickets are resolved",
				"state":        "satisfied",
				"evidenceRefs": []string{"run-cwi-governance-owner-evidence-step-1-decision", "run-cwi-governance-owner-evidence-step-2-decision", "resolution-run-cwi-governance-owner-evidence-step-1-decision"},
			},
			{
				"item":         "one Approval is approved",
				"state":        "satisfied",
				"evidenceRefs": []string{"run-cwi-governance-owner-evidence-step-2-approval"},
			},
			{
				"item":         "MCP loadtest.governed_result evidence is collected",
				"state":        "satisfied",
				"evidenceRefs": []string{"run-cwi-governance-owner-evidence/graph-001/001-mcp-call"},
			},
		},
	}
	if !newCoverageSubsystem().updateFromAssessment(run, assess, assessment) {
		t.Fatalf("coverage.updateFromAssessment changed=false, want governance owner evidence closure")
	}
	if err := newCoverageSubsystem().validateReadyForAcceptance(run); err != nil {
		t.Fatalf("coverage.validateReadyForAcceptance: %v", err)
	}
	items := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])
	if refs := mapStringSliceValue(items[0]["acceptedEvidenceByKind"])["decision"]; len(refs) != 3 {
		t.Fatalf("decision accepted refs = %+v, want decision owner refs", refs)
	}
	if refs := mapStringSliceValue(items[1]["acceptedEvidenceByKind"])["approval"]; len(refs) != 1 {
		t.Fatalf("approval accepted refs = %+v, want approval owner ref", refs)
	}
}

func TestCoverageWorkItemsGovernanceOwnerSyncRespectsRequiredCounts(t *testing.T) {
	ctx := context.Background()
	run := &runmodel.Run{
		ID:          "run-cwi-governance-owner-sync",
		WorkspaceID: "ws-cwi-governance-owner-sync",
		ExecutionContext: map[string]any{
			"dod": map[string]any{"items": []string{
				"two Decision tickets are resolved",
				"one Approval is approved",
			}},
		},
	}
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{
		ID:          "run-cwi-governance-owner-sync-step-1-decision",
		WorkspaceID: run.WorkspaceID,
		RunID:       run.ID,
		StepID:      "run-cwi-governance-owner-sync/step-1",
		State:       common.DecisionTicketStateResolved,
	}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       "resolution-run-cwi-governance-owner-sync-step-1-decision",
		TicketID: "run-cwi-governance-owner-sync-step-1-decision",
		Type:     "provide_missing_info",
	}); err != nil {
		t.Fatalf("seed resolution: %v", err)
	}
	if err := approvalRepo.Create(ctx, &approvalmodel.Approval{
		ID:          "run-cwi-governance-owner-sync-step-2-approval",
		WorkspaceID: run.WorkspaceID,
		RunID:       run.ID,
		StepID:      "run-cwi-governance-owner-sync/step-2",
		State:       common.ApprovalStateApproved,
	}); err != nil {
		t.Fatalf("seed approval: %v", err)
	}
	orch := NewRuntimeOrchestratorWithResolutions(nil, memory.NewRunRepository(), memory.NewStepRepository(), memory.NewWorkspaceRepository(), ticketRepo, resolutionRepo, approvalRepo, nil)

	if !orch.ensureCoverageLedger(ctx, run) {
		t.Fatalf("ensureCoverageLedger changed=false, want owner evidence sync")
	}
	items := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])
	decisionItem := items[0]
	if got := stringFromMap(decisionItem, "status"); got != "in_progress" {
		t.Fatalf("decision status = %q, want in_progress with one of two refs", got)
	}
	if missing := stringSliceFromValue(decisionItem["missingEvidenceKinds"]); len(missing) != 1 || missing[0] != "decision" {
		t.Fatalf("decision missingEvidenceKinds = %+v, want decision still missing", missing)
	}
	if refs := mapStringSliceValue(decisionItem["acceptedEvidenceByKind"])["decision"]; len(refs) != 2 {
		t.Fatalf("decision refs = %+v, want ticket and resolution refs recorded as partial evidence", refs)
	}
	approvalItem := items[1]
	if got := stringFromMap(approvalItem, "status"); got != "evidence_ready" {
		t.Fatalf("approval status = %q, want evidence_ready", got)
	}
	if refs := mapStringSliceValue(approvalItem["acceptedEvidenceByKind"])["approval"]; len(refs) != 1 {
		t.Fatalf("approval refs = %+v, want approved owner ref", refs)
	}
}

func TestCoverageWorkItemsAssessmentClosesEvidenceReadyItemWithoutNewRefs(t *testing.T) {
	run := &runmodel.Run{
		ID:               "run-cwi-evidence-ready-close",
		ExecutionContext: map[string]any{"dod": map[string]any{"items": []string{"closure trace is exposed"}}},
	}
	newCoverageSubsystem().ensure(run)
	verify := &runmodel.Step{
		ID:    "run-cwi-evidence-ready-close/001-verify",
		RunID: run.ID,
		Kind:  "shell/command",
		State: common.StepStateCompleted,
		Input: map[string]any{
			"coverageTargets": []string{"closure trace is exposed"},
		},
	}
	if !newCoverageSubsystem().updateFromCompletedStep(run, verify) {
		t.Fatalf("coverage.updateFromCompletedStep changed=false, want evidence_ready update")
	}
	item := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])[0]
	if got := stringFromMap(item, "status"); got != "evidence_ready" {
		t.Fatalf("status after evidence = %q, want evidence_ready", got)
	}

	assess := &runmodel.Step{ID: "run-cwi-evidence-ready-close/002-assess", RunID: run.ID, State: common.StepStateCompleted}
	assessment := map[string]any{
		"items": []map[string]any{{
			"item":         "closure trace is exposed",
			"state":        "satisfied",
			"evidenceRefs": []string{"run-cwi-evidence-ready-close/001-verify"},
		}},
	}
	if !newCoverageSubsystem().updateFromAssessment(run, assess, assessment) {
		t.Fatalf("coverage.updateFromAssessment changed=false, want satisfied closure")
	}
	item = mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])[0]
	if got := stringFromMap(item, "status"); got != "satisfied" {
		t.Fatalf("status after assessment = %q, want satisfied", got)
	}
	if err := newCoverageSubsystem().validateReadyForAcceptance(run); err != nil {
		t.Fatalf("coverage.validateReadyForAcceptance: %v", err)
	}
}

func TestCoverageWorkItemGateFailureProjectsUnresolvedItemsIntoRepairInput(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-cwi-gate-repair-input",
		ExecutionContext: map[string]any{
			"dod": map[string]any{"items": []string{"acceptance gate"}},
		},
	}
	newCoverageSubsystem().ensure(run)
	step := &runmodel.Step{
		ID:    "run-cwi-gate-repair-input/001-assess",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
		},
		Output: map[string]any{
			"error": "invalid argument: coverageWorkItems not ready for accepted result; unresolved: cwi-001:validation",
		},
	}
	attachCoverageWorkItemFailureDiagnostics(run, step)

	input := (&RuntimeOrchestrator{}).runtimeRepairFollowUpStepInput(context.Background(), run, step, 1, nil)

	if unresolved := mapSliceFromValue(input["unresolvedCoverageWorkItems"]); len(unresolved) != 1 || stringFromMap(unresolved[0], "id") != "cwi-001" {
		t.Fatalf("unresolvedCoverageWorkItems = %+v, want cwi-001", input["unresolvedCoverageWorkItems"])
	}
	if context := mapFromValue(input[coverageWorkItemContextKey]); len(mapSliceFromValue(context["unresolvedItems"])) != 1 {
		t.Fatalf("coverageWorkItemContext = %+v, want unresolved context", input[coverageWorkItemContextKey])
	}
	if pkg := mapFromValue(input["repairActionPackage"]); len(mapFromValue(pkg[coverageWorkItemContextKey])) == 0 {
		t.Fatalf("repairActionPackage = %+v, want coverageWorkItemContext", pkg)
	}
	guidance := strings.Join(stringSliceFromValue(input["repairGuidance"]), "\n")
	if !strings.Contains(guidance, "Resolve unresolvedCoverageWorkItems") {
		t.Fatalf("repairGuidance = %q, want unresolved coverage guidance", guidance)
	}
}

func TestGraphCompletionGateFailurePersistsCoverageWorkItemDiagnostics(t *testing.T) {
	ctx := context.Background()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	workspaceRepo := memory.NewWorkspaceRepository()
	run := &runmodel.Run{
		ID:               "run-cwi-graph-gate",
		WorkspaceID:      "ws-cwi-graph-gate",
		State:            common.RunStateRunning,
		InitialStepKind:  llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{"dod": map[string]any{"items": []string{"acceptance gate"}}},
	}
	newCoverageSubsystem().ensure(run)
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: run.WorkspaceID, Title: "ws", State: common.WorkspaceStateRunning, ActiveRunID: run.ID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	step := &runmodel.Step{
		ID:    "run-cwi-graph-gate/001-assess",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateCompleted,
		Input: map[string]any{stepInputKeyRuntimeRepairFollowUp: true},
		Output: map[string]any{
			stepOutputKeyDoDAssessment: map[string]any{
				"satisfied": true,
				"items": []map[string]any{{
					"item":         "acceptance gate",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-cwi-graph-gate/unknown"},
				}},
			},
		},
	}
	if err := stepRepo.Create(ctx, step); err != nil {
		t.Fatalf("seed step: %v", err)
	}
	orch := &RuntimeOrchestrator{
		runtime:    &stubRuntimeAdapter{},
		runs:       runRepo,
		steps:      stepRepo,
		workspaces: workspaceRepo,
		tickets:    memory.NewDecisionTicketRepository(),
	}

	_ = orch.handleGraphBatchCompletionGateFailure(ctx, run, step, step.Output, "runtime completion gate failed", errors.New("invalid argument: coverageWorkItems not ready for accepted result; unresolved: cwi-001:validation"))

	stored, err := stepRepo.Get(ctx, step.ID)
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	if unresolved := mapSliceFromValue(stored.Output["unresolvedCoverageWorkItems"]); len(unresolved) == 0 {
		t.Fatalf("step output = %+v, want unresolved coverage work items", stored.Output)
	}
	updated, err := runRepo.Get(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	context := mapFromValue(updated.ExecutionContext[coverageWorkItemContextKey])
	if len(mapSliceFromValue(context["unresolvedItems"])) == 0 {
		t.Fatalf("run coverageWorkItemContext = %+v, want unresolved items", context)
	}
}
