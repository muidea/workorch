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
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestEnsureRunCoverageWorkItemsBuildsNeutralBaseline(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-cwi-baseline",
		ExecutionContext: map[string]any{
			"definitionRevision": "rev-cwi-baseline",
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
		if revision := stringFromMap(item, "definitionRevision"); revision != "rev-cwi-baseline" {
			t.Fatalf("coverage work item definitionRevision = %q, want rev-cwi-baseline: %+v", revision, item)
		}
	}
	doDItems := 0
	for _, item := range items {
		if stringFromMap(item, "sourceDoDId") != "dod-cwi" {
			continue
		}
		doDItems++
		if !runtimecontract.CoverageWorkItemStatusActive(stringFromMap(item, "status")) {
			t.Fatalf("DoD coverage item status = %q, want active: %+v", stringFromMap(item, "status"), item)
		}
	}
	if doDItems != 1 {
		t.Fatalf("DoD-bound coverage work item count = %d, want 1 leaf item: %+v", doDItems, items)
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

func TestCoverageWorkItemsSyncProjectionToRepository(t *testing.T) {
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "workspace-cwi-repo-sync")
	repo := memory.NewCoverageWorkItemRepository()
	run := &runmodel.Run{
		ID:          "run-cwi-repo-sync",
		WorkspaceID: "workspace-cwi-repo-sync",
		ExecutionContext: map[string]any{
			"goal": map[string]any{
				"id":   "goal-cwi-repo-sync",
				"text": "- sync repository-backed coverage work item",
			},
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"neutral": map[string]any{
						"acceptanceDimensions": []map[string]any{{
							"id":       "dimension-required",
							"required": true,
						}},
					},
				},
			},
		},
	}
	coverage := newCoverageSubsystem(repo)
	if !coverage.ensure(run, ctx) {
		t.Fatalf("coverage.ensure changed = false, want true")
	}
	stored, err := repo.ListByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("ListByRun: %v", err)
	}
	if len(stored) != 2 {
		t.Fatalf("stored coverage work items = %d, want 2: %+v", len(stored), stored)
	}
	var targetID common.CoverageWorkItemID
	targetContextID := ""
	for _, item := range stored {
		if stringSet(item.RequiredEvidenceKinds)["validation"] {
			targetID = item.ID
			targetContextID = strings.TrimSpace(stringFromMap(item.Metadata, "contextCoverageWorkItemId"))
			break
		}
	}
	if targetID == "" || targetContextID == "" {
		t.Fatalf("stored coverage work items = %+v, want one validation-backed context item", stored)
	}
	if !strings.HasPrefix(string(targetID), string(run.ID)+"-") {
		t.Fatalf("stored id = %q, want run scoped repository id", targetID)
	}

	step := &runmodel.Step{
		ID:    "step-cwi-repo-sync",
		RunID: run.ID,
		Kind:  "mcp/tool_call",
		Input: map[string]any{
			"coverageWorkItemId": targetContextID,
			"validationEvidence": map[string]any{
				"mode":            "runtime_behavior",
				"observationType": "runtime_observation",
				"targetRefs":      []string{targetContextID},
				"observedClaims":  []string{"repository coverage work item validation observed"},
			},
		},
		State: common.StepStateCompleted,
		Output: map[string]any{
			"evidenceRef": "evidence-cwi-repo-sync",
		},
	}
	if !coverage.updateFromCompletedStep(run, step, ctx) {
		t.Fatalf("updateFromCompletedStep changed = false, want accepted evidence update")
	}
	stored, err = repo.ListByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("ListByRun after update: %v", err)
	}
	if len(stored) != 2 {
		t.Fatalf("stored coverage work items after update = %d, want 2: %+v", len(stored), stored)
	}
	storedByID := map[common.CoverageWorkItemID]int{}
	for idx, item := range stored {
		storedByID[item.ID] = idx
	}
	firstStored, ok := storedByID[targetID]
	if !ok {
		t.Fatalf("stored coverage work item IDs after update = %+v, want %s", storedByID, targetID)
	}
	refs := stringSet(stored[firstStored].AcceptedEvidenceRefs)
	if !refs["step-cwi-repo-sync"] || !refs["evidence-cwi-repo-sync"] {
		t.Fatalf("accepted evidence refs = %+v, want step and explicit evidence refs", stored[firstStored].AcceptedEvidenceRefs)
	}
}

func TestEnsureGapBaselineHydratesCoverageContextFromOwnerRepository(t *testing.T) {
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "workspace-cwi-repo-hydrate")
	repo := memory.NewCoverageWorkItemRepository()
	run := &runmodel.Run{
		ID:          "run-cwi-repo-hydrate",
		WorkspaceID: "workspace-cwi-repo-hydrate",
		State:       common.RunStateRunning,
		ExecutionContext: map[string]any{
			"goal": map[string]any{
				"id":   "goal-cwi-repo-hydrate",
				"text": "- hydrate coverage context from owner storage",
			},
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"neutral": map[string]any{
						"acceptanceDimensions": []map[string]any{{
							"id":       "dimension-required",
							"required": true,
						}},
					},
				},
			},
		},
	}
	coverage := newCoverageSubsystem(repo)
	if !coverage.ensure(run, ctx) {
		t.Fatalf("coverage.ensure changed = false, want initial owner sync")
	}
	items := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])
	if len(items) == 0 {
		t.Fatalf("coverageWorkItems = %+v, want initial projection", run.ExecutionContext[coverageWorkItemsKey])
	}
	targetContextID := strings.TrimSpace(stringFromMap(items[0], "id"))
	if targetContextID == "" {
		t.Fatalf("coverage item = %+v, want context id", items[0])
	}
	step := &runmodel.Step{
		ID:    "step-cwi-repo-hydrate",
		RunID: run.ID,
		Kind:  "mcp/tool_call",
		Input: map[string]any{
			"coverageWorkItemId": targetContextID,
			"validationEvidence": map[string]any{
				"mode":            "runtime_behavior",
				"observationType": "runtime_observation",
				"targetRefs":      []string{targetContextID},
				"observedClaims":  []string{"owner storage coverage context was hydrated"},
			},
		},
		State: common.StepStateCompleted,
		Output: map[string]any{
			"evidenceRef": "evidence-cwi-repo-hydrate",
		},
	}
	if !coverage.updateFromCompletedStep(run, step, ctx) {
		t.Fatalf("updateFromCompletedStep changed = false, want accepted evidence")
	}
	delete(run.ExecutionContext, coverageWorkItemsKey)
	delete(run.ExecutionContext, coverageWorkItemContextKey)
	delete(run.ExecutionContext, gapBaselineKey)

	runRepo := memory.NewRunRepository()
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	orch := &RuntimeOrchestrator{
		runs: runRepo,
		evidence: RuntimeEvidenceRepositories{
			CoverageWorkItems: repo,
		},
		coverage: newCoverageSubsystem(repo),
	}
	if err := orch.ensureGapBaselineBeforeDispatch(ctx, run); err != nil {
		t.Fatalf("ensureGapBaselineBeforeDispatch: %v", err)
	}
	context := mapFromValue(run.ExecutionContext[coverageWorkItemContextKey])
	foundOwnerBackedItem := false
	for _, item := range mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey]) {
		if stringFromMap(item, "id") != targetContextID {
			continue
		}
		refs := stringSet(stringSliceFromValue(item["acceptedEvidenceRefs"]))
		foundOwnerBackedItem = refs["evidence-cwi-repo-hydrate"]
	}
	if !foundOwnerBackedItem {
		t.Fatalf("coverageWorkItems = %+v, want owner-backed accepted evidence for %s", run.ExecutionContext[coverageWorkItemsKey], targetContextID)
	}
	if ledger := mapSliceFromValue(context["ledgerSummary"]); len(ledger) == 0 {
		t.Fatalf("coverageWorkItemContext = %+v, want owner-backed ledger summary", context)
	}
	baseline := mapFromValue(run.ExecutionContext[gapBaselineKey])
	if refs := stringSliceFromValue(baseline["acceptedEvidenceRefs"]); !stringSet(refs)["evidence-cwi-repo-hydrate"] {
		t.Fatalf("gapBaseline.acceptedEvidenceRefs = %+v, want owner repository evidence ref", refs)
	}
	persisted, err := runRepo.Get(ctx, run.ID)
	if err != nil {
		t.Fatalf("get persisted run: %v", err)
	}
	if baseline := mapFromValue(persisted.ExecutionContext[gapBaselineKey]); len(baseline) == 0 {
		t.Fatalf("persisted executionContext = %+v, want owner-backed gapBaseline", persisted.ExecutionContext)
	}
}

func TestNextDispatchSpecsPersistsGapBaselineBeforeInitialStep(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-gap-baseline-initial")
	workspaceID := common.WorkspaceID("ws-gap-baseline-initial")
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	run := &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateRunning,
		ExecutionContext: map[string]any{
			"goal": map[string]any{
				"id":   "goal-gap-baseline",
				"text": "- produce the requested deliverable",
			},
			"dod": map[string]any{
				"id":    "dod-gap-baseline",
				"items": []string{"deliverable can be verified"},
			},
		},
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	orch := &RuntimeOrchestrator{
		runs:  runRepo,
		steps: stepRepo,
	}

	specs, err := orch.nextDispatchSpecs(ctx, run)
	if err != nil {
		t.Fatalf("nextDispatchSpecs: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("specs = %+v, want one initial step", specs)
	}
	if baseline := mapFromValue(specs[0].Input[gapBaselineKey]); len(baseline) == 0 {
		t.Fatalf("initial step input = %+v, want gapBaseline", specs[0].Input)
	} else if todos := mapSliceFromValue(baseline["todoItems"]); len(todos) == 0 {
		t.Fatalf("gapBaseline = %+v, want todoItems", baseline)
	}
	persisted, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if baseline := mapFromValue(persisted.ExecutionContext[gapBaselineKey]); len(baseline) == 0 {
		t.Fatalf("persisted executionContext = %+v, want gapBaseline before dispatch", persisted.ExecutionContext)
	} else if stringFromMap(mapFromValue(baseline["nextAction"]), "kind") != "validate" {
		t.Fatalf("gapBaseline.nextAction = %+v, want validate", baseline["nextAction"])
	}
}

func TestGapBaselineNextActionUsesDeterministicCompleteForAcceptedBinding(t *testing.T) {
	run := &runmodel.Run{
		ID:          "run-gap-baseline-assessment",
		WorkspaceID: "ws-gap-baseline-assessment",
		State:       common.RunStateRunning,
		ExecutionContext: map[string]any{
			coverageWorkItemContextKey: map[string]any{
				"definitionRevision":   "rev-assessment",
				"coverageBaselineHash": "sha256:assessment",
				"currentFocus": map[string]any{
					"id":                    "cwi-ready",
					"status":                "evidence_ready",
					"requiredEvidenceKinds": []string{"validation"},
					"acceptedEvidenceRefs":  []string{"step-validation"},
					"acceptedEvidenceByKind": map[string]any{
						"validation": []string{"step-validation"},
					},
					"acceptedEvidenceBindingByKind": map[string]any{
						"validation": []string{"step-validation"},
					},
					"currentGap": "required evidence kinds present; coverage assessment required",
				},
				"unresolvedItems": []map[string]any{{
					"id":                    "cwi-ready",
					"status":                "evidence_ready",
					"requiredEvidenceKinds": []string{"validation"},
					"acceptedEvidenceRefs":  []string{"step-validation"},
					"acceptedEvidenceByKind": map[string]any{
						"validation": []string{"step-validation"},
					},
					"acceptedEvidenceBindingByKind": map[string]any{
						"validation": []string{"step-validation"},
					},
					"currentGap": "required evidence kinds present; coverage assessment required",
				}},
			},
		},
	}

	baseline := gapBaselineFromRun(run)
	if got := stringFromMap(mapFromValue(baseline["nextAction"]), "kind"); got != "complete" {
		t.Fatalf("gapBaseline.nextAction = %+v, want deterministic complete", baseline["nextAction"])
	}
}

func TestGapBaselineNextActionSkipsAssessmentForDeterministicAcceptedBinding(t *testing.T) {
	run := &runmodel.Run{
		ID:          "run-gap-baseline-deterministic",
		WorkspaceID: "ws-gap-baseline-deterministic",
		State:       common.RunStateRunning,
		ExecutionContext: map[string]any{
			coverageWorkItemContextKey: map[string]any{
				"definitionRevision":   "rev-deterministic",
				"coverageBaselineHash": "sha256:deterministic",
				"currentFocus": map[string]any{
					"id":                    "cwi-ready",
					"status":                "evidence_ready",
					"requiredEvidenceKinds": []string{"validation"},
					"missingEvidenceKinds":  []string{},
					"acceptedEvidenceByKind": map[string][]string{
						"validation": {"step-validation"},
					},
					"acceptedEvidenceBindingByKind": map[string][]string{
						"validation": {"step-validation"},
					},
					"acceptedEvidenceBindingRefs": []string{"step-validation"},
					"acceptedEvidenceRefs":        []string{"step-validation"},
					"currentGap":                  "required evidence kinds present; coverage assessment required",
				},
				"unresolvedItems": []map[string]any{{
					"id":                    "cwi-ready",
					"status":                "evidence_ready",
					"requiredEvidenceKinds": []string{"validation"},
					"missingEvidenceKinds":  []string{},
					"acceptedEvidenceByKind": map[string][]string{
						"validation": {"step-validation"},
					},
					"acceptedEvidenceBindingByKind": map[string][]string{
						"validation": {"step-validation"},
					},
					"acceptedEvidenceBindingRefs": []string{"step-validation"},
					"acceptedEvidenceRefs":        []string{"step-validation"},
					"currentGap":                  "required evidence kinds present; coverage assessment required",
				}},
			},
		},
	}

	baseline := gapBaselineFromRun(run)
	if got := stringFromMap(mapFromValue(baseline["nextAction"]), "kind"); got != "complete" {
		t.Fatalf("gapBaseline.nextAction = %+v, want complete for deterministic accepted binding", baseline["nextAction"])
	}
}

func TestRefreshRunConvergenceAuthorityAlignsFocusAndBudgetToCurrentCoverage(t *testing.T) {
	run := &runmodel.Run{
		ID:          "run-refresh-authority",
		WorkspaceID: "ws-refresh-authority",
		State:       common.RunStateRunning,
		ExecutionContext: map[string]any{
			activeSliceIDKey: "slice-primary",
			coverageFocusKey: map[string]any{
				"id":                  "focus-cwi-stale",
				"sliceId":             "slice-primary",
				"coverageWorkItemIds": []string{"cwi-stale"},
				"state":               "active",
			},
			convergenceBudgetKey: map[string]any{
				"focusId":                   "focus-cwi-stale",
				"sliceId":                   "slice-primary",
				"state":                     "escalated",
				"decision":                  "escalate_decision",
				"repairAttempts":            int64(4),
				"noCoverageDeltaRounds":     int64(3),
				"repeatedFailureSignatures": int64(2),
				"lastDecisionReason":        "max_no_coverage_delta_rounds_exceeded",
				"maxNoCoverageDeltaRounds":  int64(7),
			},
			coverageWorkItemContextKey: map[string]any{
				"definitionRevision":   "rev-refresh-authority",
				"coverageBaselineHash": "sha256:refresh-authority",
				"currentFocus": map[string]any{
					"id":                    "cwi-ready",
					"status":                "evidence_ready",
					"requiredEvidenceKinds": []string{"validation"},
					"acceptedEvidenceRefs":  []string{"step-validation"},
					"acceptedEvidenceByKind": map[string]any{
						"validation": []string{"step-validation"},
					},
					"acceptedEvidenceBindingByKind": map[string]any{
						"validation": []string{"step-validation"},
					},
					"currentGap": "required evidence kinds present; coverage assessment required",
				},
				"unresolvedItems": []map[string]any{{
					"id":                    "cwi-ready",
					"status":                "evidence_ready",
					"requiredEvidenceKinds": []string{"validation"},
					"acceptedEvidenceRefs":  []string{"step-validation"},
					"acceptedEvidenceByKind": map[string]any{
						"validation": []string{"step-validation"},
					},
					"acceptedEvidenceBindingByKind": map[string]any{
						"validation": []string{"step-validation"},
					},
					"currentGap": "required evidence kinds present; coverage assessment required",
				}},
			},
		},
	}

	if !refreshRunConvergenceAuthority(run) {
		t.Fatal("refreshRunConvergenceAuthority changed = false, want focus/budget alignment")
	}
	focus := mapFromValue(run.ExecutionContext[coverageFocusKey])
	if got := stringFromMap(focus, "id"); got != "focus-cwi-ready" {
		t.Fatalf("coverageFocus = %+v, want focus-cwi-ready", focus)
	}
	budget := mapFromValue(run.ExecutionContext[convergenceBudgetKey])
	if budget["decision"] != "continue" || budget["state"] != "tracking" {
		t.Fatalf("budget = %+v, want reset tracking budget", budget)
	}
	if budget["focusId"] != "focus-cwi-ready" || budget["sliceId"] != "slice-primary" {
		t.Fatalf("budget = %+v, want current focus scope", budget)
	}
	if int64FromValue(budget["maxNoCoverageDeltaRounds"]) != 7 {
		t.Fatalf("budget = %+v, want neutral limits preserved", budget)
	}
	for _, key := range []string{"repairAttempts", "noCoverageDeltaRounds", "repeatedFailureSignatures", "lastDecisionReason"} {
		if _, ok := budget[key]; ok {
			t.Fatalf("budget retained stale key %s: %+v", key, budget)
		}
	}
}

func TestConvergenceAuthoritySnapshotDrivesBaselineContractAndStagePacket(t *testing.T) {
	run := &runmodel.Run{
		ID:          "run-authority-snapshot",
		WorkspaceID: "ws-authority-snapshot",
		State:       common.RunStateRunning,
		ExecutionContext: map[string]any{
			activeSliceIDKey: "slice-authority",
			coverageFocusKey: map[string]any{
				"id":                  "focus-cwi-stale",
				"sliceId":             "slice-authority",
				"coverageWorkItemIds": []string{"cwi-stale"},
				"state":               "active",
			},
			convergenceBudgetKey: map[string]any{
				"focusId":                   "focus-cwi-stale",
				"sliceId":                   "slice-authority",
				"state":                     "escalated",
				"decision":                  "escalate_decision",
				"repairAttempts":            int64(3),
				"maxRepairAttemptsPerFocus": int64(9),
			},
			coverageWorkItemContextKey: map[string]any{
				"definitionRevision":   "rev-authority-snapshot",
				"coverageBaselineHash": "sha256:authority-snapshot",
				"currentFocus": map[string]any{
					"id":                    "cwi-ready",
					"status":                "evidence_ready",
					"requiredEvidenceKinds": []string{"validation"},
					"acceptedEvidenceRefs":  []string{"step-validation"},
					"acceptedEvidenceByKind": map[string]any{
						"validation": []string{"step-validation"},
					},
					"acceptedEvidenceBindingByKind": map[string]any{
						"validation": []string{"step-validation"},
					},
				},
				"unresolvedItems": []map[string]any{{
					"id":                    "cwi-ready",
					"status":                "evidence_ready",
					"requiredEvidenceKinds": []string{"validation"},
					"acceptedEvidenceRefs":  []string{"step-validation"},
					"acceptedEvidenceByKind": map[string]any{
						"validation": []string{"step-validation"},
					},
					"acceptedEvidenceBindingByKind": map[string]any{
						"validation": []string{"step-validation"},
					},
				}},
			},
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustProduceConcreteFailureEvidence": true,
					"acceptedKinds":                      []string{"file/read"},
				},
			},
			"stateChangingOutputRequired": true,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"requestedOutputMode":         outputModeIncrementalArtifact,
			"convergenceContract": map[string]any{
				"state":           convergenceStateNeedEvidence,
				"repairObjective": repairObjectiveAcquireFailureEvidence,
			},
		},
	}

	if !refreshRunConvergenceAuthority(run) {
		t.Fatal("refreshRunConvergenceAuthority changed = false, want authority snapshot")
	}
	authority := mapFromValue(run.ExecutionContext[convergenceAuthorityKey])
	if got := stringFromMap(mapFromValue(authority["nextAction"]), "kind"); got != "complete" {
		t.Fatalf("convergenceAuthority.nextAction = %+v, want deterministic complete", authority["nextAction"])
	}
	if got := stringFromMap(mapFromValue(authority["currentFocus"]), "id"); got != "cwi-ready" {
		t.Fatalf("convergenceAuthority.currentFocus = %+v, want raw current coverage work item", authority["currentFocus"])
	}

	baseline := gapBaselineFromRun(run)
	if got := stringFromMap(mapFromValue(baseline["nextAction"]), "kind"); got != "complete" {
		t.Fatalf("gapBaseline.nextAction = %+v, want deterministic complete", baseline["nextAction"])
	}

	input := runExecutionInput(run, "")
	for _, key := range []string{"stateChangingOutputRequired", "repairObjective", "requestedOutputMode", "repairActionPackage"} {
		if value, ok := input[key]; ok {
			t.Fatalf("input[%s] = %+v, want authority-bound step input without stale dynamic executionContext field", key, value)
		}
	}
	contract := BuildConvergenceContract(run, nil, nil, map[string]any{"input": input}, nil)
	if contract.State != "" || !contract.DeterministicComplete || contract.RuntimeStageMode != "deterministic_ledger" {
		t.Fatalf("convergenceContract = %+v, want deterministic completion without convergence state from authority next action", contract)
	}
	applied := ApplyConvergenceContract(input, contract)
	if boolValue(applied["concreteFailureEvidenceRequired"]) || boolValue(mapFromValue(mapFromValue(applied["repairActionPackage"])["requiredOutput"])["mustProduceConcreteFailureEvidence"]) {
		t.Fatalf("applied input = %+v, stale evidence acquisition contract leaked into current assessment", applied)
	}
	packet := buildStagePacketFromStepInput(run, "assessment-001", applied)
	if got := stringFromMap(packet.ActiveFocus, "id"); got != "cwi-ready" {
		t.Fatalf("stagePacket.activeFocus = %+v, want current authority focus", packet.ActiveFocus)
	}
}

func TestPrepareGapBaselineDispatchSpecsRebuildsStaleDynamicContractFromAuthority(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-authority-dispatch",
		WorkspaceID:     "ws-authority-dispatch",
		State:           common.RunStateRunning,
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"definitionRevision": "rev-authority-dispatch",
			"dod": map[string]any{
				"id": "dod-authority-dispatch",
				"items": []map[string]any{{
					"id":                    "dod-validate",
					"text":                  "validation evidence must be present",
					"requiredEvidenceKinds": []string{"validation"},
				}},
			},
			coverageFocusKey: map[string]any{
				"id":                  "focus-cwi-stale",
				"sliceId":             "slice-stale",
				"coverageWorkItemIds": []string{"cwi-stale"},
			},
			convergenceBudgetKey: map[string]any{
				"focusId":        "focus-cwi-stale",
				"state":          "escalated",
				"decision":       "escalate_decision",
				"repairAttempts": int64(4),
			},
			"convergenceContract": map[string]any{
				"state":                       convergenceStateNeedStateChange,
				"repairObjective":             repairObjectiveStateChangingOutput,
				"stateChangingOutputRequired": true,
			},
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustIncludeStateChangingNode": true,
				},
			},
		},
	}
	specs := []stepDispatchSpec{{
		ID:   "run-authority-dispatch/step-1",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{"mustIncludeStateChangingNode": true},
			},
		},
	}}

	orch := &RuntimeOrchestrator{}
	prepared, err := orch.prepareGapBaselineDispatchSpecs(context.Background(), run, specs)
	if err != nil {
		t.Fatalf("prepareGapBaselineDispatchSpecs: %v", err)
	}
	if len(prepared) != 1 {
		t.Fatalf("prepared specs = %+v, want one spec", prepared)
	}
	input := prepared[0].Input
	if boolValue(input["stateChangingOutputRequired"]) {
		t.Fatalf("stateChangingOutputRequired leaked into validation dispatch input: %+v", input)
	}
	contract := mapFromValue(input["convergenceContract"])
	if got := stringFromMap(contract, "state"); got != convergenceStateNeedValidation {
		t.Fatalf("convergenceContract = %+v, want need_validation", contract)
	}
	if got := stringFromMap(input, "repairObjective"); got != repairObjectiveConcreteValidationEvidence {
		t.Fatalf("repairObjective = %q, want validation evidence objective", got)
	}
	packet := mapFromValue(input[stepInputKeyStagePacket])
	if len(packet) == 0 {
		t.Fatalf("stagePacket missing from prepared input: %+v", input)
	}
	if got := stringFromMap(mapFromValue(packet["activeFocus"]), "id"); got != "cwi-001" {
		t.Fatalf("stagePacket.activeFocus = %+v, want cwi-001", packet["activeFocus"])
	}
	runContract := mapFromValue(run.ExecutionContext["convergenceContract"])
	if got := stringFromMap(runContract, "state"); got != convergenceStateNeedValidation {
		t.Fatalf("run convergenceContract = %+v, want need_validation", runContract)
	}
	if boolValue(run.ExecutionContext["stateChangingOutputRequired"]) {
		t.Fatalf("run executionContext retained stale stateChangingOutputRequired: %+v", run.ExecutionContext)
	}
}

func TestInitialDispatchBuildsStateChangingContractFromGapBaseline(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-gap-baseline-state-changing")
	workspaceID := common.WorkspaceID("ws-gap-baseline-state-changing")
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	run := &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateRunning,
		ExecutionContext: map[string]any{
			"goal": map[string]any{
				"id":   "goal-gap-state-changing",
				"text": "- produce the first deliverable",
			},
			"dod": map[string]any{
				"id":    "dod-gap-state-changing",
				"items": []string{"first deliverable has implementation evidence"},
			},
			"executionContract": map[string]any{
				"stateChangePolicy": "must_produce_state_change",
				"profileContracts": map[string]any{
					"neutral": map[string]any{
						"deliverableExpectations": []map[string]any{{
							"id":           "first-deliverable",
							"required":     true,
							"evidenceRole": "primary_state_change",
						}},
					},
				},
			},
		},
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	orch := &RuntimeOrchestrator{
		runs:  runRepo,
		steps: stepRepo,
	}

	specs, err := orch.nextDispatchSpecs(ctx, run)
	if err != nil {
		t.Fatalf("nextDispatchSpecs: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("specs = %+v, want one initial step", specs)
	}
	input := specs[0].Input
	baseline := mapFromValue(input[gapBaselineKey])
	if todos := mapSliceFromValue(baseline["todoItems"]); len(todos) == 0 {
		t.Fatalf("gapBaseline = %+v, want todoItems", baseline)
	}
	contract := mapFromValue(input["convergenceContract"])
	if stringFromMap(contract, "state") != convergenceStateNeedStateChange {
		t.Fatalf("convergenceContract = %+v, want state-changing contract from first-run gap", contract)
	}
	if !boolValue(input["stateChangingOutputRequired"]) || stringFromMap(input, "repairObjective") != repairObjectiveStateChangingOutput {
		t.Fatalf("input = %+v, want state-changing output required", input)
	}
	pkg := mapFromValue(input["repairActionPackage"])
	required := mapFromValue(pkg["requiredOutput"])
	if !boolValue(required["mustIncludeStateChangingNode"]) || !boolValue(required["mustDeclareCoverageTargets"]) {
		t.Fatalf("requiredOutput = %+v, want state-changing coverage target contract", required)
	}
	if targets := mapSliceFromValue(pkg["coverageTargetSet"]); len(targets) == 0 || stringFromMap(targets[0], "coverageWorkItemId") == "" {
		t.Fatalf("repairActionPackage = %+v, want coverage target set from gap baseline", pkg)
	}
	if focus := mapFromValue(mapFromValue(pkg[coverageWorkItemContextKey])["currentFocus"]); stringFromMap(focus, "id") == "" {
		t.Fatalf("repairActionPackage = %+v, want current focus", pkg)
	}
}

func TestRefreshConvergenceAuthorityStripsCoverageFocusRuntimeDiagnostics(t *testing.T) {
	run := &runmodel.Run{
		ID:          "run-authority-focus-diagnostics",
		WorkspaceID: "ws-authority-focus-diagnostics",
		State:       common.RunStateRunning,
		ExecutionContext: map[string]any{
			activeSliceIDKey: "slice-authority",
			coverageFocusKey: map[string]any{
				"id":                  "focus-cwi-ready",
				"sliceId":             "slice-authority",
				"coverageWorkItemIds": []string{"cwi-ready"},
				"state":               "active",
				"attempt":             int64(3),
				"failureClass":        failureClassShellCommand,
				"failureSignature":    "class=shell_command_failed|target=stale",
			},
			convergenceBudgetKey: map[string]any{
				"focusId":  "focus-cwi-ready",
				"sliceId":  "slice-authority",
				"state":    "tracking",
				"decision": "continue",
			},
			coverageWorkItemContextKey: map[string]any{
				"definitionRevision":   "rev-authority-focus-diagnostics",
				"coverageBaselineHash": "sha256:authority-focus-diagnostics",
				"currentFocus": map[string]any{
					"id":                    "cwi-ready",
					"status":                "open",
					"requiredEvidenceKinds": []string{"validation"},
					"missingEvidenceKinds":  []string{"validation"},
				},
				"unresolvedItems": []map[string]any{{
					"id":                    "cwi-ready",
					"status":                "open",
					"requiredEvidenceKinds": []string{"validation"},
					"missingEvidenceKinds":  []string{"validation"},
				}},
			},
		},
	}

	if !refreshRunConvergenceAuthority(run) {
		t.Fatal("refreshRunConvergenceAuthority changed = false, want focus diagnostics stripped")
	}
	focus := mapFromValue(run.ExecutionContext[coverageFocusKey])
	if got := stringFromMap(focus, "id"); got != "focus-cwi-ready" {
		t.Fatalf("coverageFocus = %+v, want same focus scope retained", focus)
	}
	if focus["attempt"] != nil || focus["failureClass"] != nil || focus["failureSignature"] != nil {
		t.Fatalf("coverageFocus = %+v, want runtime diagnostics stripped", focus)
	}
	authorityFocus := mapFromValue(mapFromValue(run.ExecutionContext[convergenceAuthorityKey])["currentFocus"])
	if authorityFocus["attempt"] != nil || authorityFocus["failureClass"] != nil || authorityFocus["failureSignature"] != nil {
		t.Fatalf("convergenceAuthority.currentFocus = %+v, want pure coverage ledger focus", authorityFocus)
	}
}

func TestGapBaselineFromRunPreservesAcceptedAndRejectedEvidenceRefs(t *testing.T) {
	run := &runmodel.Run{
		ID:          "run-gap-baseline-evidence",
		WorkspaceID: "ws-gap-baseline-evidence",
		State:       common.RunStateRunning,
		ExecutionContext: map[string]any{
			coverageWorkItemContextKey: map[string]any{
				"definitionRevision":   "rev-evidence",
				"coverageBaselineHash": "sha256:evidence",
				"unresolvedItems": []map[string]any{{
					"id":                   "cwi-001",
					"sourcePath":           "goal.leaves[0]",
					"missingEvidenceKinds": []string{"validation"},
				}},
			},
			coverageWorkItemsKey: []map[string]any{{
				"id":                   "cwi-001",
				"status":               "failed",
				"acceptedEvidenceRefs": []string{"evidence-accepted-a"},
				"rejectedEvidenceRefs": []string{"evidence-rejected-a"},
				"acceptedEvidenceByKind": map[string][]string{"state_change": {
					"evidence-accepted-a",
					"evidence-accepted-b",
				}},
				"rejectedEvidenceByKind": map[string][]string{"validation": {
					"evidence-rejected-a",
					"evidence-rejected-b",
				}},
			}},
		},
	}

	baseline := gapBaselineFromRun(run)
	if got := stringSliceFromValue(baseline["acceptedEvidenceRefs"]); len(got) != 2 || got[0] != "evidence-accepted-a" || got[1] != "evidence-accepted-b" {
		t.Fatalf("acceptedEvidenceRefs = %+v, want compact accepted refs", got)
	}
	if got := stringSliceFromValue(baseline["rejectedEvidenceRefs"]); len(got) != 2 || got[0] != "evidence-rejected-a" || got[1] != "evidence-rejected-b" {
		t.Fatalf("rejectedEvidenceRefs = %+v, want compact rejected refs", got)
	}
}

func TestEnsureRunCoverageWorkItemsBuildsAcceptanceSlicesFromGenericPolicy(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-acceptance-slices",
		ExecutionContext: map[string]any{
			"goal": map[string]any{
				"id":   "goal-acceptance-slices",
				"text": "- first leaf\n- second leaf\n- third leaf",
			},
			"executionContract": map[string]any{
				"acceptanceSlicePolicy": map[string]any{
					"schemaVersion": "workorch.acceptance_slice_policy.v1",
					"maxWorkItems":  2,
					"groups": []map[string]any{{
						"id":           "slice-goals",
						"title":        "Goal leaves",
						"scope":        "goal_leaf",
						"priority":     10,
						"maxWorkItems": 2,
						"match": map[string]any{
							"sourceScopes": []string{"goal_leaf"},
						},
					}},
				},
			},
		},
	}

	if !newCoverageSubsystem().ensure(run) {
		t.Fatalf("coverage.ensure changed = false, want acceptance projection")
	}
	slices := mapSliceFromValue(run.ExecutionContext[acceptanceSlicesKey])
	if len(slices) != 2 {
		t.Fatalf("acceptanceSlices = %+v, want two bounded slices", slices)
	}
	if got := stringFromMap(slices[0], "id"); got != "slice-goals-001" {
		t.Fatalf("first slice id = %q, want policy chunk id", got)
	}
	if ids := stringSliceFromValue(slices[0]["coverageWorkItemIds"]); len(ids) != 2 || ids[0] != "cwi-001" || ids[1] != "cwi-002" {
		t.Fatalf("first slice coverage ids = %+v, want first two work items", ids)
	}
	if active := stringFromMap(run.ExecutionContext, activeSliceIDKey); active != "slice-goals-001" {
		t.Fatalf("activeSliceId = %q, want first pending slice", active)
	}
	focus := mapFromValue(run.ExecutionContext[coverageFocusKey])
	if stringFromMap(focus, "id") != "focus-cwi-001" || stringFromMap(focus, "sliceId") != "slice-goals-001" {
		t.Fatalf("coverageFocus = %+v, want first item in active slice", focus)
	}
	input := initialStepInput(run)
	acceptance := mapFromValue(input["acceptance"])
	if stringFromMap(acceptance, "activeSliceId") != "slice-goals-001" {
		t.Fatalf("input.acceptance = %+v, want active slice", acceptance)
	}
	if inputFocus := mapFromValue(input[coverageFocusKey]); stringFromMap(inputFocus, "id") != "focus-cwi-001" {
		t.Fatalf("input.coverageFocus = %+v, want current focus", inputFocus)
	}
}

func TestEnsureRunCoverageWorkItemsBuildsBoundedDefaultAcceptanceSlices(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-acceptance-default-slices",
		ExecutionContext: map[string]any{
			"goal": map[string]any{
				"text": strings.Join([]string{
					"- one",
					"- two",
					"- three",
					"- four",
					"- five",
					"- six",
				}, "\n"),
			},
		},
	}

	if !newCoverageSubsystem().ensure(run) {
		t.Fatalf("coverage.ensure changed = false, want default acceptance slices")
	}
	slices := mapSliceFromValue(run.ExecutionContext[acceptanceSlicesKey])
	if len(slices) != 2 {
		t.Fatalf("acceptanceSlices = %+v, want default chunks of five work items", slices)
	}
	if ids := stringSliceFromValue(slices[0]["coverageWorkItemIds"]); len(ids) != 5 {
		t.Fatalf("first default slice ids = %+v, want five work items", ids)
	}
	if active := stringFromMap(run.ExecutionContext, activeSliceIDKey); active != "slice-required-001" {
		t.Fatalf("activeSliceId = %q, want first default slice", active)
	}
}

func TestEnsureRunCoverageWorkItemsUsesPolicyDataWithoutTemplateBranching(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-acceptance-policy-neutral",
		ExecutionContext: map[string]any{
			"goal": map[string]any{"text": "produce stateful output"},
			"executionContract": map[string]any{
				"acceptanceSlicePolicy": map[string]any{
					"groups": []map[string]any{{
						"id":       "slice-primary",
						"title":    "Primary outputs",
						"scope":    "primary",
						"priority": 5,
						"match": map[string]any{
							"evidenceRoles": []string{"primary_state_change"},
						},
					}},
				},
				"profileContracts": map[string]any{
					"neutral": map[string]any{
						"deliverableExpectations": []map[string]any{{
							"id":           "stateful-output",
							"required":     true,
							"evidenceRole": "primary_state_change",
						}},
					},
				},
			},
		},
	}

	if !newCoverageSubsystem().ensure(run) {
		t.Fatalf("coverage.ensure changed = false, want acceptance projection")
	}
	slices := mapSliceFromValue(run.ExecutionContext[acceptanceSlicesKey])
	if len(slices) == 0 || stringFromMap(slices[0], "id") != "slice-primary" {
		t.Fatalf("acceptanceSlices = %+v, want policy-declared primary slice", slices)
	}
	focus := mapFromValue(run.ExecutionContext[coverageFocusKey])
	focusIDs := stringSliceFromValue(focus["coverageWorkItemIds"])
	if len(focusIDs) != 1 {
		t.Fatalf("coverageFocus = %+v, want one focused work item", focus)
	}
	var focusItem map[string]any
	for _, item := range mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey]) {
		if stringFromMap(item, "id") == focusIDs[0] {
			focusItem = item
			break
		}
	}
	if role := stringFromMap(focusItem, "evidenceRole"); role != "primary_state_change" {
		t.Fatalf("coverageFocus item = %+v, want primary_state_change evidence role", focusItem)
	}
	if stringFromMap(focus, "sliceId") != "slice-primary" {
		t.Fatalf("coverageFocus = %+v, want focus bound to policy slice", focus)
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
		"acceptedEvidenceBindingByKind": map[string]any{
			"state_change": []string{"run-001/001-change"},
		},
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

func TestCoverageWorkItemsReplanSuccessorResetsActiveLedgerAndKeepsHistory(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-cwi-replan-lineage",
		ExecutionContext: map[string]any{
			"definitionRevision": "rev-current",
			"goal":               map[string]any{"text": "stateful outcome"},
			gapBaselineKey: map[string]any{
				"schemaVersion": gapBaselineSchema,
				"nextAction":    map[string]any{"kind": runtimecontract.NextActionTypeComplete},
			},
			convergenceAuthorityKey: map[string]any{
				"schemaVersion": convergenceAuthoritySchema,
				"nextAction":    map[string]any{"kind": runtimecontract.NextActionTypeComplete},
			},
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
				"acceptedEvidenceBindingByKind": map[string]any{
					"state_change": []string{"run-prev/001-change"},
				},
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
		t.Fatalf("coverageWorkItems = %+v, want one successor-local current item", items)
	}
	item := items[0]
	if got := stringFromMap(item, "id"); got != "cwi-001" {
		t.Fatalf("id = %q, want successor-local stable order id", got)
	}
	if got := stringFromMap(item, "status"); got == "satisfied" || got == "evidence_ready" || got == "in_progress" {
		t.Fatalf("status = %q, want reset active successor status", got)
	}
	if refs := stringSliceFromValue(item["acceptedEvidenceRefs"]); len(refs) != 0 {
		t.Fatalf("acceptedEvidenceRefs = %+v, want active successor ledger reset", refs)
	}
	if accepted := mapStringSliceValue(item["acceptedEvidenceByKind"]); len(accepted) != 0 {
		t.Fatalf("acceptedEvidenceByKind = %+v, want active successor ledger reset", accepted)
	}
	if rejected := mapStringSliceValue(item["rejectedEvidenceByKind"]); len(rejected) != 0 {
		t.Fatalf("rejectedEvidenceByKind = %+v, want active successor ledger reset", rejected)
	}
	context := mapFromValue(run.ExecutionContext[coverageWorkItemContextKey])
	focus := mapFromValue(context["currentFocus"])
	if stringFromMap(focus, "targetText") != "stateful outcome" ||
		len(stringSliceFromValue(focus["acceptedEvidenceRefs"])) != 0 ||
		len(stringSliceFromValue(focus["rejectedEvidenceRefs"])) != 0 {
		t.Fatalf("currentFocus = %+v, want reset active successor focus", focus)
	}
	historical := mapSliceFromValue(context["historicalItems"])
	if len(historical) != 1 ||
		stringFromMap(historical[0], "historicalReason") != "replan_successor_reset" ||
		!stringSet(stringSliceFromValue(historical[0]["acceptedEvidenceRefs"]))["run-prev/001-change"] ||
		!stringSet(stringSliceFromValue(historical[0]["rejectedEvidenceRefs"]))["run-prev/002-verify"] {
		t.Fatalf("historicalItems = %+v, want predecessor evidence preserved only as history", historical)
	}
	if baseline := mapFromValue(run.ExecutionContext[gapBaselineKey]); len(baseline) != 0 {
		t.Fatalf("gapBaseline = %+v, want stale predecessor baseline removed", baseline)
	}
	if authority := mapFromValue(run.ExecutionContext[convergenceAuthorityKey]); len(authority) != 0 {
		t.Fatalf("convergenceAuthority = %+v, want stale predecessor authority removed", authority)
	}
}

func TestCoverageWorkItemsRebuildDemotesTextMatchedAcceptedLineage(t *testing.T) {
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
	if refs := stringSliceFromValue(item["acceptedEvidenceRefs"]); len(refs) != 0 {
		t.Fatalf("acceptedEvidenceRefs = %+v, want text-matched legacy lineage demoted", refs)
	}
	if supporting := mapStringSliceValue(item["supportingEvidenceByKind"]); !stringSliceContains(supporting["validation"], "run-prev/graph/001-alpha") {
		t.Fatalf("supportingEvidenceByKind = %+v, want text-matched legacy lineage as supporting evidence", supporting)
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
		if stringFromMap(item, "historicalReason") == "replan_successor_reset" &&
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

func TestCoverageWorkItemContextAlignsCurrentFocusToActiveCoverageFocus(t *testing.T) {
	revision := "rev-current"
	items := []map[string]any{
		{
			"id":                    "cwi-012",
			"definitionRevision":    revision,
			"source":                "goal",
			"sourceScope":           "goal_leaf",
			"sourceRequirementId":   "goal-req-012",
			"sourcePath":            "goal.leaves[11]",
			"targetText":            "later validation target",
			"normalizedSourceKey":   "goal:goal_leaf:goal-req-012",
			"requiredEvidenceKinds": []string{"validation"},
			"missingEvidenceKinds":  []string{"validation"},
			"status":                "open",
		},
		{
			"id":                    "cwi-085",
			"definitionRevision":    revision,
			"source":                "deliverable",
			"sourceScope":           "deliverable",
			"sourceRequirementId":   "source_change",
			"sourcePath":            "executionContract.profileContracts.neutral.deliverableExpectations[0]",
			"targetText":            "primary source change",
			"normalizedSourceKey":   "deliverable:deliverable:source_change",
			"requiredEvidenceKinds": []string{"state_change", "validation"},
			"acceptedEvidenceByKind": map[string]any{
				"state_change": []string{"run/graph/001-patch"},
				"validation":   []string{"run/graph/002-verify"},
			},
			"acceptedEvidenceBindingByKind": map[string]any{
				"state_change": []string{"run/graph/001-patch"},
				"validation":   []string{"run/graph/002-verify"},
			},
			"acceptedEvidenceRefs": []string{"run/graph/001-patch"},
			"status":               "evidence_ready",
		},
	}
	context := coverageWorkItemContextFromItems(revision, items, nil, nil)
	if got := stringFromMap(mapFromValue(context["currentFocus"]), "id"); got != "cwi-012" {
		t.Fatalf("test setup currentFocus = %q, want cwi-012 before active focus alignment", got)
	}
	focus := map[string]any{
		"id":                  "focus-cwi-085",
		"coverageWorkItemIds": []string{"cwi-085"},
		"targetRefs":          []string{"source_change", "deliverable:deliverable:source_change"},
		"requiredEvidenceKinds": []string{
			"state_change",
			"validation",
		},
	}

	aligned := coverageWorkItemContextAlignedToFocus(context, focus)

	current := mapFromValue(aligned["currentFocus"])
	if stringFromMap(current, "id") != "cwi-085" ||
		stringFromMap(current, "activeCoverageFocusId") != "focus-cwi-085" ||
		!boolValue(current["focusScoped"]) {
		t.Fatalf("aligned currentFocus = %+v, want active cwi-085 focus", current)
	}
	if refs := stringSliceFromValue(current["targetRefs"]); !stringSliceContains(refs, "source_change") {
		t.Fatalf("aligned currentFocus.targetRefs = %+v, want active focus refs", refs)
	}
	focusItems := mapSliceFromValue(aligned["currentFocusItems"])
	if len(focusItems) == 0 || stringFromMap(focusItems[0], "id") != "cwi-085" {
		t.Fatalf("aligned currentFocusItems = %+v, want cwi-085 first", focusItems)
	}
	for _, item := range mapSliceFromValue(aligned["ledgerSummary"]) {
		if stringFromMap(item, "id") == "cwi-085" && boolValue(item["omittedFromCurrentFocus"]) {
			t.Fatalf("aligned ledger cwi-085 = %+v, want in-focus ledger item", item)
		}
	}
	if got := stringFromMap(mapFromValue(context["currentFocus"]), "id"); got != "cwi-012" {
		t.Fatalf("original context currentFocus mutated to %q", got)
	}
}

func TestCoverageWorkItemContextAlignedEvidenceReadyLedgerItemDoesNotReopenMissingEvidence(t *testing.T) {
	context := map[string]any{
		"ledgerSummary": []map[string]any{{
			"id":                    "cwi-ready",
			"source":                "deliverable",
			"sourceScope":           "deliverable",
			"sourceRequirementId":   "deliverable-ready",
			"targetText":            "deliverable-ready",
			"normalizedSourceKey":   "deliverable:deliverable:deliverable-ready",
			"requiredEvidenceKinds": []string{"validation", "state_change"},
			"acceptedEvidenceByKind": map[string]any{
				"state_change": []string{"run/graph/001-patch"},
				"validation":   []string{"run/graph/002-verify"},
			},
			"acceptedEvidenceBindingByKind": map[string]any{
				"state_change": []string{"run/graph/001-patch"},
				"validation":   []string{"run/graph/002-verify"},
			},
			"acceptedEvidenceRefs":    []string{"run/graph/001-patch", "run/graph/002-verify"},
			"rejectedEvidenceRefs":    []string{"run/graph/003-rejected-verify"},
			"status":                  "evidence_ready",
			"currentGap":              "required evidence kinds present; coverage assessment required",
			"omittedFromCurrentFocus": true,
			"omittedReason":           "context_budget_top_k",
		}},
	}
	focus := map[string]any{
		"id":                    "focus-cwi-ready",
		"coverageWorkItemIds":   []string{"cwi-ready"},
		"targetRefs":            []string{"deliverable-ready"},
		"requiredEvidenceKinds": []string{"validation", "state_change"},
		"acceptedEvidenceRefs":  []string{"run/graph/001-patch", "run/graph/002-verify"},
		"rejectedEvidenceRefs":  []string{"run/graph/003-rejected-verify"},
	}

	aligned := coverageWorkItemContextAlignedToFocus(context, focus)

	current := mapFromValue(aligned["currentFocus"])
	if got := stringFromMap(current, "id"); got != "cwi-ready" {
		t.Fatalf("aligned currentFocus id = %q, want cwi-ready; current=%+v", got, current)
	}
	if got := stringFromMap(current, "status"); got != "evidence_ready" {
		t.Fatalf("aligned currentFocus status = %q, want evidence_ready", got)
	}
	if missing := stringSliceFromValue(current["missingEvidenceKinds"]); len(missing) != 0 {
		t.Fatalf("aligned currentFocus missingEvidenceKinds = %+v, want empty for evidence_ready ledger item", missing)
	}
	nextStates := stringSliceFromValue(current["nextAllowedConvergenceStates"])
	if len(nextStates) != 0 {
		t.Fatalf("aligned currentFocus nextAllowedConvergenceStates = %+v, want none after accepted binding", nextStates)
	}
	if got := stringFromMap(current, "activeCoverageFocusId"); got != "focus-cwi-ready" {
		t.Fatalf("activeCoverageFocusId = %q, want focus-cwi-ready", got)
	}
}

func TestSlimExecutionContextAlignsCoverageContextWithCoverageFocus(t *testing.T) {
	revision := "rev-current"
	context := coverageWorkItemContextFromItems(revision, []map[string]any{
		{
			"id":                    "cwi-012",
			"definitionRevision":    revision,
			"source":                "goal",
			"sourceScope":           "goal_leaf",
			"sourceRequirementId":   "goal-req-012",
			"targetText":            "later validation target",
			"requiredEvidenceKinds": []string{"validation"},
			"missingEvidenceKinds":  []string{"validation"},
			"status":                "open",
		},
		{
			"id":                    "cwi-085",
			"definitionRevision":    revision,
			"source":                "deliverable",
			"sourceScope":           "deliverable",
			"sourceRequirementId":   "source_change",
			"targetText":            "primary source change",
			"requiredEvidenceKinds": []string{"state_change", "validation"},
			"acceptedEvidenceByKind": map[string]any{
				"state_change": []string{"run/graph/001-patch"},
				"validation":   []string{"run/graph/002-verify"},
			},
			"status": "evidence_ready",
		},
	}, nil, nil)
	executionContext := map[string]any{
		coverageWorkItemContextKey: context,
		coverageFocusKey: map[string]any{
			"id":                  "focus-cwi-085",
			"coverageWorkItemIds": []string{"cwi-085"},
			"targetRefs":          []string{"source_change"},
		},
	}

	slim := slimExecutionContextForStepInput(executionContext)

	nestedContext := mapFromValue(slim[coverageWorkItemContextKey])
	if got := stringFromMap(mapFromValue(nestedContext["currentFocus"]), "id"); got != "cwi-085" {
		t.Fatalf("slim executionContext coverageWorkItemContext.currentFocus = %q, want cwi-085", got)
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
			"acceptedEvidenceBindingByKind": map[string]any{
				"validation": []string{"run/graph/001-ok"},
			},
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
		Input: map[string]any{
			"targetRefs": []string{"goal-leaf-001"},
		},
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
				"observedClaims":    []string{"stateful outcome was observed for the target coverage item"},
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

func TestCoverageWorkItemsAcceptDeclaredStateChangingRuntimeStep(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-cwi-runtime-state-changing-step",
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
	items := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])
	if len(items) != 1 {
		t.Fatalf("coverageWorkItems = %+v, want one item", items)
	}
	step := &runmodel.Step{
		ID:    "run-cwi-runtime-state-changing-step/001-runtime-state-change",
		RunID: run.ID,
		Kind:  "shell/command",
		State: common.StepStateCompleted,
		Input: map[string]any{
			"stateChanging":       true,
			"coverageWorkItemIds": []string{stringFromMap(items[0], "id")},
			"coverageTargets":     []string{"stateful outcome"},
			"validationEvidence": map[string]any{
				"mode":            "runtime_behavior",
				"observationType": "runtime_observation",
				"targetRefs":      []string{stringFromMap(items[0], "id")},
				"observedClaims":  []string{"stateful outcome observed at runtime"},
			},
		},
		Output: map[string]any{"exitCode": 0},
	}

	if !newCoverageSubsystem().updateFromCompletedStep(run, step) {
		t.Fatalf("coverage.updateFromCompletedStep changed = false, want state_change and validation evidence update")
	}
	item := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])[0]
	if missing := coverageMissingEvidenceKinds(item); len(missing) != 0 {
		t.Fatalf("missingEvidenceKinds = %+v, want none", missing)
	}
	accepted := mapStringSliceValue(item["acceptedEvidenceByKind"])
	if len(accepted["state_change"]) == 0 || len(accepted["validation"]) == 0 {
		t.Fatalf("acceptedEvidenceByKind = %+v, want state_change and validation refs", accepted)
	}
	if got := stringFromMap(item, "status"); got != "evidence_ready" {
		t.Fatalf("status = %q, want evidence_ready", got)
	}
}

func TestCoverageWorkItemsDoNotAcceptValidationOnlyRuntimeStepAsStateChange(t *testing.T) {
	item := map[string]any{
		"id":                    "cwi-validation-only",
		"sourceRequirementId":   "goal-leaf-001",
		"status":                "open",
		"requiredEvidenceKinds": []string{"state_change", "validation"},
		"requiredEvidenceCounts": map[string]any{
			"state_change": 1,
			"validation":   1,
		},
		"missingEvidenceKinds": []string{"state_change", "validation"},
	}
	step := &runmodel.Step{
		ID:    "run-cwi-validation-only/001-validate",
		RunID: "run-cwi-validation-only",
		Kind:  "shell/command",
		State: common.StepStateCompleted,
		Input: map[string]any{
			"coverageWorkItemIds": []string{"cwi-validation-only"},
			"validationEvidence": map[string]any{
				"mode":            "runtime_behavior",
				"observationType": "runtime_observation",
				"targetRefs":      []string{"goal-leaf-001"},
				"observedClaims":  []string{"goal leaf evidence aligned"},
			},
		},
		Output: map[string]any{"exitCode": 0},
	}
	update := coverageWorkItemStepUpdate(step)

	if !coverageApplyWorkItemEvidenceWithOptions(item, update.EvidenceKinds, update.EvidenceRefs, update.Accepted, false, string(step.ID), "", update.AcceptanceOptions) {
		t.Fatalf("coverageApplyWorkItemEvidence changed = false, want validation evidence update")
	}
	accepted := mapStringSliceValue(item["acceptedEvidenceByKind"])
	if len(accepted["state_change"]) != 0 || len(accepted["validation"]) == 0 {
		t.Fatalf("acceptedEvidenceByKind = %+v, want validation only", accepted)
	}
	missing := stringSet(coverageMissingEvidenceKinds(item))
	if !missing["state_change"] || missing["validation"] {
		t.Fatalf("missingEvidenceKinds = %+v, want state_change only", coverageMissingEvidenceKinds(item))
	}
}

func TestCoverageTargetSetPreservesStateChangingWorkItemContract(t *testing.T) {
	context := map[string]any{
		"currentFocus": map[string]any{
			"id":                    "cwi-primary",
			"source":                "deliverableCoverage",
			"sourceScope":           "deliverable",
			"sourceRequirementId":   "primary-output",
			"sourcePath":            "profileContracts.neutral.deliverableExpectations[0]",
			"targetText":            "primary-output",
			"evidenceRole":          "primary_state_change",
			"requiredEvidenceKinds": []string{"state_change", "validation"},
			"missingEvidenceKinds":  []string{"state_change"},
			"status":                "open",
		},
	}

	targets := coverageTargetSetFromWorkItemContext(context)

	if len(targets) != 1 {
		t.Fatalf("coverageTargetSet = %+v, want one target", targets)
	}
	target := targets[0]
	if target["evidenceRole"] != "primary_state_change" ||
		!stringSliceContains(stringSliceFromValue(target["requiredEvidenceKinds"]), "state_change") ||
		!stringSliceContains(stringSliceFromValue(target["missingEvidenceKinds"]), "state_change") {
		t.Fatalf("coverage target = %+v, want preserved state-changing contract fields", target)
	}
}

func TestMissingDeliverableCoverageFromStepEnrichesFromCoverageWorkItemContext(t *testing.T) {
	step := &runmodel.Step{
		ID: "run-cwi-missing-deliverable/step-001",
		Input: map[string]any{
			coverageWorkItemContextKey: map[string]any{
				"currentFocus": map[string]any{
					"id":                    "cwi-primary",
					"source":                "deliverableCoverage",
					"sourceScope":           "deliverable",
					"sourceRequirementId":   "primary-output",
					"sourcePath":            "profileContracts.neutral.deliverableExpectations[0]",
					"targetText":            "primary-output",
					"evidenceRole":          "primary_state_change",
					"requiredEvidenceKinds": []string{"state_change", "validation"},
					"missingEvidenceKinds":  []string{"state_change"},
					"status":                "open",
				},
			},
		},
		Output: map[string]any{
			"runtimeDiagnostics": map[string]any{
				"missingDeliverableCoverage": []map[string]any{{
					"id":    "primary-output",
					"kind":  "deliverableExpectation",
					"state": "missing",
				}},
			},
		},
	}

	missing := missingDeliverableCoverageFromStep(step)

	if len(missing) != 1 {
		t.Fatalf("missingDeliverableCoverage = %+v, want one enriched item", missing)
	}
	item := missing[0]
	if item["evidenceRole"] != "primary_state_change" ||
		!stringSliceContains(stringSliceFromValue(item["requiredEvidenceKinds"]), "state_change") ||
		item["sourceRequirementId"] != "primary-output" {
		t.Fatalf("missingDeliverableCoverage = %+v, want enriched state-changing contract fields", missing)
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
	item := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])[0]
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
				"targetRefs":      []string{firstNonEmptyString(stringFromMap(item, "sourceRequirementId"), stringFromMap(item, "id"))},
				"observedClaims":  []string{"goal leaf evidence aligned"},
			},
		},
		Output: map[string]any{"stage": "mcp_tool_call_completed"},
	}

	if !newCoverageSubsystem().updateFromCompletedStep(run, step) {
		t.Fatalf("coverage.updateFromCompletedStep changed = false, want sourceRequirementId-linked evidence")
	}
	item = mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])[0]
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
	items := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])
	if len(items) == 0 {
		t.Fatalf("coverageWorkItems = empty, want generated coverage item")
	}
	workItemID := stringFromMap(items[0], "id")
	focus := mapFromValue(run.ExecutionContext[coverageFocusKey])
	if len(focus) == 0 {
		t.Fatalf("coverageFocus = empty, want generated active focus")
	}
	run.ExecutionContext[convergenceBudgetKey] = map[string]any{
		"focusId":                  stringFromMap(focus, "id"),
		"sliceId":                  stringFromMap(focus, "sliceId"),
		"coverageWorkItemIds":      []string{workItemID},
		"state":                    "tracking",
		"decision":                 "continue",
		"noCoverageDeltaRounds":    int64(1),
		"maxNoCoverageDeltaRounds": int64(5),
	}
	step := &runmodel.Step{
		ID:    "run-cwi-failed-step/001-change",
		RunID: run.ID,
		Kind:  "artifact/write",
		State: common.StepStateFailed,
		Output: map[string]any{
			"coverageTargets":         []string{"stateful outcome"},
			"error":                   "state-changing evidence failed",
			failureOutputKeyClass:     failureClassRuntimeContractMisalign,
			failureOutputKeySignature: "class=runtime_internal_contract_misalignment|target=stateful outcome",
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
	budget := mapFromValue(run.ExecutionContext[convergenceBudgetKey])
	if int64FromValue(budget["noCoverageDeltaRounds"]) != 2 || budget["lastNoDeltaReason"] == nil {
		t.Fatalf("convergenceBudget = %+v, want failed rejected evidence to count as no positive coverage delta", budget)
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
		Input: map[string]any{
			"coverageWorkItemIds": []string{stringFromMap(items[0], "id")},
			"validationEvidence": map[string]any{
				"mode":            "runtime_behavior",
				"observationType": "runtime_observation",
				"targetRefs":      []string{firstNonEmptyString(stringFromMap(items[0], "sourceRequirementId"), stringFromMap(items[0], "id"))},
				"observedClaims":  []string{"go-build-verify passed"},
			},
		},
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

func TestCoverageWorkItemsAcceptanceGateRejectsEmptyLedgerWithBaseline(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-cwi-empty-ledger-gate",
		ExecutionContext: map[string]any{
			gapBaselineKey: map[string]any{"schemaVersion": gapBaselineSchema, "baselineRevision": "gap-present"},
		},
	}

	err := newCoverageSubsystem().validateReadyForAcceptance(run)

	if err == nil || !strings.Contains(err.Error(), "coverage ledger empty") {
		t.Fatalf("coverage.validateReadyForAcceptance err = %v, want empty ledger failure", err)
	}
}

func TestCoverageWorkItemsAcceptanceGateRejectsBlockedItems(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewCoverageWorkItemRepository()
	workspaceID := common.WorkspaceID("ws-cwi-blocked-gate")
	runID := common.RunID("run-cwi-blocked-gate")
	itemID := coverageWorkItemRepositoryID(runID, "cwi-blocked")
	run := &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
	}
	if err := repo.Create(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), &runtimecontract.CoverageWorkItem{
		ID:                  itemID,
		WorkspaceID:         workspaceID,
		RunID:               runID,
		DefinitionRevision:  "rev-blocked-gate",
		NormalizedSourceKey: "blocked-gate",
		Status:              "blocked",
		Metadata: map[string]any{
			"id":     "cwi-blocked",
			"status": "blocked",
		},
	}); err != nil {
		t.Fatalf("create coverage item: %v", err)
	}

	err := newCoverageSubsystem(repo).validateReadyForAcceptance(run, ctx)

	if err == nil || !strings.Contains(err.Error(), "cwi-blocked:blocked") {
		t.Fatalf("coverage.validateReadyForAcceptance err = %v, want blocked item failure", err)
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

func TestCoverageWorkItemsAcceptanceGateAllowsEvidenceReadyAcceptedEvidence(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-cwi-evidence-ready-acceptance",
		ExecutionContext: map[string]any{
			coverageWorkItemsKey: []map[string]any{{
				"id":                    "cwi-evidence-ready",
				"status":                "evidence_ready",
				"requiredEvidenceKinds": []string{"validation"},
				"missingEvidenceKinds":  []string{},
				"acceptedEvidenceByKind": map[string][]string{
					"validation": {"run-cwi-evidence-ready-acceptance/validate"},
				},
				"acceptedEvidenceRefs": []string{"run-cwi-evidence-ready-acceptance/validate"},
			}},
		},
	}

	if err := newCoverageSubsystem().validateReadyForAcceptance(run); err != nil {
		t.Fatalf("coverage.validateReadyForAcceptance: %v, want evidence_ready accepted evidence to pass deterministic gate", err)
	}
}

func TestCoverageWorkItemRejectedOverrideOnlyBlocksSameKindBinding(t *testing.T) {
	item := map[string]any{
		"id":                    "cwi-rejected-cross-kind",
		"status":                "evidence_ready",
		"requiredEvidenceKinds": []string{runtimecontract.EvidenceKindValidation},
		"missingEvidenceKinds":  []string{},
		"acceptedEvidenceByKind": map[string][]string{
			runtimecontract.EvidenceKindValidation: {"evidence-shared"},
		},
		"acceptedEvidenceBindingByKind": map[string][]string{
			runtimecontract.EvidenceKindValidation: {"evidence-shared"},
		},
		"acceptedEvidenceRefs": []string{"evidence-shared"},
		"rejectedEvidenceByKind": map[string][]string{
			runtimecontract.EvidenceKindStateChange: {"evidence-shared"},
		},
	}

	if !coverageWorkItemCanCloseDeterministically(item) {
		t.Fatalf("coverageWorkItemCanCloseDeterministically = false, want cross-kind rejection not to override accepted validation binding")
	}

	item["rejectedEvidenceByKind"] = map[string][]string{
		runtimecontract.EvidenceKindValidation: {"evidence-shared"},
	}
	if coverageWorkItemCanCloseDeterministically(item) {
		t.Fatalf("coverageWorkItemCanCloseDeterministically = true, want same-kind rejection to override accepted validation binding")
	}
}

func TestCoverageWorkItemsAcceptanceGateDeterministicallyClosesHydratedReadyBinding(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewCoverageWorkItemRepository()
	workspaceID := common.WorkspaceID("ws-cwi-ready-binding-gate")
	runID := common.RunID("run-cwi-ready-binding-gate")
	coverageID := coverageWorkItemRepositoryID(runID, "cwi-ready-binding")
	if err := repo.Create(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), &runtimecontract.CoverageWorkItem{
		ID:                    coverageID,
		WorkspaceID:           workspaceID,
		RunID:                 runID,
		DefinitionRevision:    "rev-ready-binding-gate",
		NormalizedSourceKey:   "source-ready-binding-gate",
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
		Status:                "evidence_ready",
		AcceptedEvidenceRefs:  []string{"validation-001"},
		CurrentGap:            "required evidence kinds present; coverage assessment required",
		Metadata: map[string]any{
			"id":                   "cwi-ready-binding",
			"status":               "evidence_ready",
			"missingEvidenceKinds": []string{},
			"acceptedEvidenceByKind": map[string][]string{
				runtimecontract.EvidenceKindValidation: {"validation-001"},
			},
			"acceptedEvidenceBindingByKind": map[string][]string{
				runtimecontract.EvidenceKindValidation: {"validation-001"},
			},
			"acceptedEvidenceBindingRefs": []string{"validation-001"},
		},
	}); err != nil {
		t.Fatalf("create coverage item: %v", err)
	}
	run := &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		ExecutionContext: map[string]any{
			gapBaselineKey: map[string]any{"schemaVersion": gapBaselineSchema, "baselineRevision": "gap-present"},
		},
	}

	if err := newCoverageSubsystem(repo).validateReadyForAcceptance(run, ctx); err != nil {
		t.Fatalf("coverage.validateReadyForAcceptance: %v, want deterministic close before gate", err)
	}
	item, err := repo.Get(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), coverageID)
	if err != nil {
		t.Fatalf("get coverage item: %v", err)
	}
	if item.Status != "satisfied" {
		t.Fatalf("repo coverage status = %q, want satisfied", item.Status)
	}
	if refs := stringSliceFromValue(item.Metadata["satisfiedByEvidenceRefs"]); !stringSliceContains(refs, "validation-001") {
		t.Fatalf("satisfiedByEvidenceRefs = %+v, want validation-001", refs)
	}
}

func TestCoverageWorkItemsGovernanceItemsRequireOwnerEvidence(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-cwi-governance-kinds",
		ExecutionContext: map[string]any{
			"dod": map[string]any{"items": []map[string]any{
				{
					"id":                    "decision-tickets",
					"text":                  "resolved governance tickets",
					"requiredEvidenceKinds": []string{"decision"},
					"requiredEvidenceCounts": map[string]any{
						"decision": 2,
					},
				},
				{
					"id":                    "approval-record",
					"text":                  "approved governance record",
					"requiredEvidenceKinds": []string{"approval"},
				},
				{
					"id":   "mcp-validation",
					"text": "MCP loadtest.governed_result evidence is collected",
				},
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

func TestCoverageWorkItemsDeriveRequiredEvidenceKindsFromEvidencePolicies(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-cwi-policy-required-kinds",
		ExecutionContext: map[string]any{
			"goal": map[string]any{
				"id":   "goal-policy-required-kinds",
				"text": "- policy-bound goal coverage",
			},
			"dod": map[string]any{
				"id":    "dod-policy-required-kinds",
				"items": []string{"policy-bound dod item"},
			},
			"evidencePolicies": []map[string]any{
				{
					"id": "policy-required-validation",
					"coverageRequirements": []map[string]any{
						{
							"id":                    "*",
							"scope":                 "goal_coverage",
							"requiredEvidenceKinds": []string{"assessment"},
						},
						{
							"id":                    "*",
							"scope":                 "dod_item",
							"requiredEvidenceKinds": []string{"approval"},
						},
					},
				},
			},
		},
	}

	newCoverageSubsystem().ensure(run)

	items := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])
	if len(items) != 2 {
		t.Fatalf("coverageWorkItems len = %d, want goal and dod items: %+v", len(items), items)
	}
	for _, item := range items {
		required := stringSliceFromValue(item["requiredEvidenceKinds"])
		switch stringFromMap(item, "sourceScope") {
		case "goal_leaf":
			if !stringSliceContains(required, "assessment") {
				t.Fatalf("goal item requiredEvidenceKinds = %+v, want policy-bound assessment", required)
			}
		case "dod_item":
			if !stringSliceContains(required, "approval") {
				t.Fatalf("dod item requiredEvidenceKinds = %+v, want policy-bound approval", required)
			}
		default:
			t.Fatalf("unexpected coverage item scope: %+v", item)
		}
	}
}

func TestCoverageWorkItemsDoNotInferGovernanceEvidenceFromBusinessText(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-cwi-business-text-decision",
		ExecutionContext: map[string]any{
			"dod": map[string]any{"items": []string{
				"默认输出仅保留决策必需字段，并返回 complete/truncated 元信息",
				"operator decision text appears here as business data, not a governance ticket",
			}},
		},
	}

	newCoverageSubsystem().ensure(run)

	items := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])
	if len(items) != 2 {
		t.Fatalf("coverageWorkItems len = %d, want 2", len(items))
	}
	for idx, item := range items {
		got := stringSliceFromValue(item["requiredEvidenceKinds"])
		if len(got) != 1 || got[0] != "validation" {
			t.Fatalf("item[%d] requiredEvidenceKinds = %+v, want [validation]", idx, got)
		}
	}
}

func TestCoverageWorkItemsRebuildDemotesLegacyAcceptedEvidenceWithoutStructuredBinding(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-cwi-legacy-inferred-governance",
		ExecutionContext: map[string]any{
			"definitionRevision": "rev-current",
			"goal": map[string]any{
				"id":   "goal-cwi-legacy",
				"text": "- 默认输出仅保留决策必需字段，并返回 complete/truncated 元信息",
			},
			coverageWorkItemsKey: []map[string]any{{
				"id":                     "cwi-008",
				"definitionRevision":     "rev-current",
				"normalizedSourceKey":    "goal:goal_leaf:goal-cwi-legacy-goal-leaf-001",
				"source":                 "goal",
				"sourceScope":            "goal_leaf",
				"sourceGoalId":           "goal-cwi-legacy",
				"sourceRequirementId":    "goal-cwi-legacy-goal-leaf-001",
				"sourcePath":             "goal.leaves[0]",
				"targetText":             "默认输出仅保留决策必需字段，并返回 complete/truncated 元信息",
				"status":                 "in_progress",
				"requiredEvidenceKinds":  []string{"decision", "validation"},
				"missingEvidenceKinds":   []string{"decision"},
				"acceptedEvidenceRefs":   []string{"run-prev/graph/001-validate"},
				"acceptedEvidenceByKind": map[string]any{"validation": []string{"run-prev/graph/001-validate"}},
			}},
		},
	}

	if !newCoverageSubsystem().ensure(run) {
		t.Fatalf("coverage.ensure changed = false, want legacy inferred governance requirement normalized")
	}
	items := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])
	if len(items) != 1 {
		t.Fatalf("coverageWorkItems = %+v, want one normalized item", items)
	}
	item := items[0]
	if got := stringSliceFromValue(item["requiredEvidenceKinds"]); len(got) != 1 || got[0] != "validation" {
		t.Fatalf("requiredEvidenceKinds = %+v, want validation only", got)
	}
	if got := stringSliceFromValue(item["missingEvidenceKinds"]); len(got) != 1 || got[0] != "validation" {
		t.Fatalf("missingEvidenceKinds = %+v, want validation after weak legacy evidence demotion", got)
	}
	if got := stringFromMap(item, "status"); got != "in_progress" {
		t.Fatalf("status = %q, want in_progress after weak legacy evidence demotion", got)
	}
	if accepted := mapStringSliceValue(item["acceptedEvidenceByKind"]); len(accepted["validation"]) != 0 {
		t.Fatalf("acceptedEvidenceByKind = %+v, want weak legacy validation demoted", accepted)
	}
	if supporting := mapStringSliceValue(item["supportingEvidenceByKind"]); !stringSliceContains(supporting["validation"], "run-prev/graph/001-validate") {
		t.Fatalf("supportingEvidenceByKind = %+v, want demoted legacy validation ref", supporting)
	}
	if demotions := mapSliceFromValue(item["legacyEvidenceDemotions"]); len(demotions) != 1 || demotions[0]["reason"] != "legacy_evidence_not_structurally_bound" {
		t.Fatalf("legacyEvidenceDemotions = %+v, want weak accepted evidence demotion", demotions)
	}
}

func TestCoverageWorkItemsRebuildPreservesStructurallyBoundInheritedEvidence(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-cwi-structured-inherited-evidence",
		ExecutionContext: map[string]any{
			"definitionRevision": "rev-current",
			"goal": map[string]any{
				"id":   "goal-cwi-structured",
				"text": "- structured inherited evidence target",
			},
			coverageWorkItemsKey: []map[string]any{{
				"id":                     "cwi-001",
				"definitionRevision":     "rev-current",
				"normalizedSourceKey":    "goal:goal_leaf:goal-cwi-structured-goal-leaf-001",
				"source":                 "goal",
				"sourceScope":            "goal_leaf",
				"sourceGoalId":           "goal-cwi-structured",
				"sourceRequirementId":    "goal-cwi-structured-goal-leaf-001",
				"sourcePath":             "goal.leaves[0]",
				"targetText":             "structured inherited evidence target",
				"status":                 "evidence_ready",
				"requiredEvidenceKinds":  []string{"validation"},
				"missingEvidenceKinds":   []string{},
				"acceptedEvidenceRefs":   []string{"run-prev/graph/001-validate"},
				"acceptedEvidenceByKind": map[string]any{"validation": []string{"run-prev/graph/001-validate"}},
				"acceptedEvidenceBindingByKind": map[string]any{
					"validation": []string{"run-prev/graph/001-validate"},
				},
			}},
		},
	}

	newCoverageSubsystem().ensure(run)
	items := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])
	if len(items) != 1 {
		t.Fatalf("coverageWorkItems = %+v, want one item", items)
	}
	item := items[0]
	if got := stringFromMap(item, "status"); got != "evidence_ready" {
		t.Fatalf("status = %q, want structurally bound inherited evidence preserved", got)
	}
	if accepted := mapStringSliceValue(item["acceptedEvidenceByKind"]); !stringSliceContains(accepted["validation"], "run-prev/graph/001-validate") {
		t.Fatalf("acceptedEvidenceByKind = %+v, want structured inherited validation preserved", accepted)
	}
	if supporting := mapStringSliceValue(item["supportingEvidenceByKind"]); len(supporting["validation"]) != 0 {
		t.Fatalf("supportingEvidenceByKind = %+v, want no demotion", supporting)
	}
}

func TestCoverageWorkItemsRepositoryHydrationNormalizesLegacyInferredGovernanceEvidence(t *testing.T) {
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-cwi-legacy-inferred-governance-repo")
	repo := memory.NewCoverageWorkItemRepository()
	run := &runmodel.Run{
		ID:          "run-cwi-legacy-inferred-governance-repo",
		WorkspaceID: "ws-cwi-legacy-inferred-governance-repo",
		ExecutionContext: map[string]any{
			"definitionRevision": "rev-current",
			"goal": map[string]any{
				"id":   "goal-cwi-legacy-repo",
				"text": "- 默认输出仅保留决策必需字段，并返回 complete/truncated 元信息",
			},
		},
	}
	if err := repo.Create(ctx, &runtimecontract.CoverageWorkItem{
		ID:                    "run-cwi-legacy-inferred-governance-repo-cwi-001",
		WorkspaceID:           run.WorkspaceID,
		RunID:                 run.ID,
		DefinitionRevision:    "rev-current",
		NormalizedSourceKey:   "goal:goal_leaf:goal-cwi-legacy-repo-goal-leaf-001",
		SourceScope:           "goal_leaf",
		SourceRequirementID:   "goal-cwi-legacy-repo-goal-leaf-001",
		SourceGoalID:          "goal-cwi-legacy-repo",
		SourcePath:            "goal.leaves[0]",
		TargetText:            "默认输出仅保留决策必需字段，并返回 complete/truncated 元信息",
		RequiredEvidenceKinds: []string{"decision", "validation"},
		Status:                "in_progress",
		AcceptedEvidenceRefs:  []string{"run-prev/graph/001-validate"},
		CurrentGap:            "missing accepted evidence binding for required evidence kind: decision",
		Metadata: map[string]any{
			"id":                     "cwi-001",
			"source":                 "goal",
			"sourceScope":            "goal_leaf",
			"requiredEvidenceKinds":  []string{"decision", "validation"},
			"missingEvidenceKinds":   []string{"decision"},
			"acceptedEvidenceByKind": map[string]any{"validation": []string{"run-prev/graph/001-validate"}},
		},
	}); err != nil {
		t.Fatalf("seed coverage work item: %v", err)
	}

	changed, err := newCoverageSubsystem(repo).ensureFromRepository(ctx, run)
	if err != nil {
		t.Fatalf("coverage.ensureFromRepository: %v", err)
	}
	if !changed {
		t.Fatalf("coverage.ensureFromRepository changed=false, want owner repository normalization")
	}
	stored, err := repo.ListByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("ListByRun: %v", err)
	}
	if len(stored) != 1 {
		t.Fatalf("stored coverage work items = %+v, want one normalized owner item", stored)
	}
	gotKinds := stored[0].RequiredEvidenceKinds
	if len(gotKinds) != 1 || gotKinds[0] != "validation" {
		t.Fatalf("stored RequiredEvidenceKinds = %+v, want validation only", gotKinds)
	}
	if got := stored[0].Status; got != "in_progress" {
		t.Fatalf("stored status = %q, want in_progress after weak legacy evidence demotion", got)
	}
	if missing := stringSliceFromValue(stored[0].Metadata["missingEvidenceKinds"]); len(missing) != 1 || missing[0] != "validation" {
		t.Fatalf("stored missingEvidenceKinds = %+v, want validation after weak legacy evidence demotion", missing)
	}
	if accepted := mapStringSliceValue(stored[0].Metadata["acceptedEvidenceByKind"]); len(accepted["validation"]) != 0 {
		t.Fatalf("stored acceptedEvidenceByKind = %+v, want weak legacy validation demoted", accepted)
	}
	if supporting := mapStringSliceValue(stored[0].Metadata["supportingEvidenceByKind"]); !stringSliceContains(supporting["validation"], "run-prev/graph/001-validate") {
		t.Fatalf("stored supportingEvidenceByKind = %+v, want demoted legacy validation ref", supporting)
	}
}

func TestCoverageWorkItemsGovernanceItemsRejectMCPValidationEvidence(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-cwi-governance-reject-mcp",
		ExecutionContext: map[string]any{
			"dod": map[string]any{"items": []map[string]any{
				{
					"id":                    "decision-tickets",
					"text":                  "resolved governance tickets",
					"requiredEvidenceKinds": []string{"decision"},
					"requiredEvidenceCounts": map[string]any{
						"decision": 2,
					},
				},
				{
					"id":                    "approval-record",
					"text":                  "approved governance record",
					"requiredEvidenceKinds": []string{"approval"},
				},
				{
					"id":   "mcp-validation",
					"text": "MCP loadtest.governed_result evidence is collected",
				},
			}},
		},
	}
	newCoverageSubsystem().ensure(run)
	assess := &runmodel.Step{ID: "run-cwi-governance-reject-mcp/001-assess", RunID: run.ID, State: common.StepStateCompleted}
	assessment := map[string]any{
		"items": []map[string]any{
			{
				"id":            "decision-tickets",
				"item":          "resolved governance tickets",
				"evidenceKinds": []string{"decision"},
				"state":         "satisfied",
				"evidenceRefs":  []string{"run-cwi-governance-reject-mcp/graph-001/001-mcp-call"},
			},
			{
				"id":            "approval-record",
				"item":          "approved governance record",
				"evidenceKinds": []string{"approval"},
				"state":         "satisfied",
				"evidenceRefs":  []string{"run-cwi-governance-reject-mcp/graph-001/001-mcp-call"},
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
			"dod": map[string]any{"items": []map[string]any{
				{
					"id":                    "decision-tickets",
					"text":                  "resolved governance tickets",
					"requiredEvidenceKinds": []string{"decision"},
					"requiredEvidenceCounts": map[string]any{
						"decision": 2,
					},
				},
				{
					"id":                    "approval-record",
					"text":                  "approved governance record",
					"requiredEvidenceKinds": []string{"approval"},
				},
				{
					"id":   "mcp-validation",
					"text": "MCP loadtest.governed_result evidence is collected",
				},
			}},
		},
	}
	newCoverageSubsystem().ensure(run)
	assess := &runmodel.Step{ID: "run-cwi-governance-owner-evidence/001-assess", RunID: run.ID, State: common.StepStateCompleted}
	assessment := map[string]any{
		"items": []map[string]any{
			{
				"id":            "decision-tickets",
				"item":          "resolved governance tickets",
				"evidenceKinds": []string{"decision"},
				"state":         "satisfied",
				"evidenceRefs":  []string{"run-cwi-governance-owner-evidence-step-1-decision", "run-cwi-governance-owner-evidence-step-2-decision", "resolution-run-cwi-governance-owner-evidence-step-1-decision"},
			},
			{
				"id":            "approval-record",
				"item":          "approved governance record",
				"evidenceKinds": []string{"approval"},
				"state":         "satisfied",
				"evidenceRefs":  []string{"run-cwi-governance-owner-evidence-step-2-approval"},
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
			"dod": map[string]any{"items": []map[string]any{
				{
					"id":                    "decision-tickets",
					"text":                  "resolved governance tickets",
					"requiredEvidenceKinds": []string{"decision"},
					"requiredEvidenceCounts": map[string]any{
						"decision": 2,
					},
				},
				{
					"id":                    "approval-record",
					"text":                  "approved governance record",
					"requiredEvidenceKinds": []string{"approval"},
				},
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

	changed, err := orch.ensureCoverageLedger(ctx, run)
	if err != nil {
		t.Fatalf("ensureCoverageLedger: %v", err)
	}
	if !changed {
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
	items := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])
	if len(items) != 1 {
		t.Fatalf("coverageWorkItems = %+v, want one item", run.ExecutionContext[coverageWorkItemsKey])
	}
	targetRef := firstNonEmptyString(stringFromMap(items[0], "sourceRequirementId"), stringFromMap(items[0], "id"))
	verify := &runmodel.Step{
		ID:    "run-cwi-evidence-ready-close/001-verify",
		RunID: run.ID,
		Kind:  "shell/command",
		State: common.StepStateCompleted,
		Input: map[string]any{
			"coverageWorkItemIds": []string{stringFromMap(items[0], "id")},
			"validationEvidence": map[string]any{
				"mode":            "runtime_behavior",
				"observationType": "runtime_observation",
				"targetRefs":      []string{targetRef},
				"observedClaims":  []string{"closure trace is exposed"},
			},
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
