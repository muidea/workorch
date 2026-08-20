package activationcore

import (
	"context"
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	"github.com/muidea/workorch/internal/pkg/workorchmemorycontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func mustActivationPayload(t *testing.T, input map[string]any) workorcheventpayload.Payload {
	t.Helper()
	payload, err := workorcheventpayload.FromMap(input)
	if err != nil {
		t.Fatalf("decode activation payload: %v", err)
	}
	return payload
}

type fakeWorkspaceTemplateReader struct {
	item *workspacemodel.WorkspaceTemplate
}

func (f fakeWorkspaceTemplateReader) Get(context.Context, string) (*workspacemodel.WorkspaceTemplate, error) {
	return f.item, nil
}

func (f fakeWorkspaceTemplateReader) List(context.Context) ([]*workspacemodel.WorkspaceTemplate, error) {
	if f.item == nil {
		return nil, nil
	}
	return []*workspacemodel.WorkspaceTemplate{f.item}, nil
}

func TestOrderedWorkspaceGoalsUsesOwnerRevisionNotInputOrAuditOrder(t *testing.T) {
	workspaceID := common.WorkspaceID("ws-001")
	first := &workspacemodel.Goal{ID: "goal-z", WorkspaceID: workspaceID, Text: "first", DefinitionRevision: "goal:00000000000000000001:first"}
	second := &workspacemodel.Goal{ID: "goal-a", WorkspaceID: workspaceID, Text: "second", DefinitionRevision: "goal:00000000000000000002:second"}

	items := OrderedWorkspaceGoals([]*workspacemodel.Goal{second, first})

	if len(items) != 2 {
		t.Fatalf("ordered goals len = %d, want 2", len(items))
	}
	if items[0].ID != first.ID || items[1].ID != second.ID {
		t.Fatalf("ordered goals = [%s %s], want [%s %s]", items[0].ID, items[1].ID, first.ID, second.ID)
	}
}

func TestEffectiveWorkspaceDoDMergesReadyItems(t *testing.T) {
	ctx := context.Background()
	workspaceID := common.WorkspaceID("ws-001")
	first := &workspacemodel.DoD{ID: "dod-001", WorkspaceID: workspaceID, Items: []string{"one", "two"}, State: common.DoDStateReady}
	second := &workspacemodel.DoD{ID: "dod-002", WorkspaceID: workspaceID, Items: []string{"two", "three"}, State: common.DoDStateSatisfied}

	effective, contexts, err := EffectiveWorkspaceDoD(ctx, func(context.Context, common.WorkspaceID) ([]*workspacemodel.DoD, error) {
		return []*workspacemodel.DoD{first, second}, nil
	}, workspaceID, first)
	if err != nil {
		t.Fatalf("EffectiveWorkspaceDoD err = %v", err)
	}
	if effective == nil {
		t.Fatal("EffectiveWorkspaceDoD returned nil effective DoD")
	}
	if effective.ID != second.ID {
		t.Fatalf("effective DoD ID = %s, want %s", effective.ID, second.ID)
	}
	if got := effective.Items; len(got) != 3 || got[0] != "one" || got[1] != "two" || got[2] != "three" {
		t.Fatalf("effective DoD items = %#v, want [one two three]", got)
	}
	if len(contexts) != 2 {
		t.Fatalf("contexts len = %d, want 2", len(contexts))
	}
}

func TestBuildActivationMemoryRecallTrace(t *testing.T) {
	runID := common.RunID("run-001")
	workspaceID := common.WorkspaceID("ws-001")
	trace := BuildRuntimeActivationMemoryRecallTrace(runID, workspaceID, &runtimecontract.RuntimeBootstrap{Activation: runtimecontract.RuntimeActivationContext{
		Memories: []runtimecontract.RuntimeMemoryContext{
			{ID: "mem-001", Summary: "first", VectorScore: 0.25},
			{ID: "mem-002", Summary: "second", VectorScore: 0.5},
		},
		MemorySelection: &workorchmemorycontract.RecallSelectionEvidence{
			Policy:             "test_selection",
			Status:             "selected",
			CandidateMemoryIDs: []common.MemoryID{"mem-001", "mem-002"},
			SelectedMemoryIDs:  []common.MemoryID{"mem-001", "mem-002"},
			RetrievedMemoryIDs: []common.MemoryID{"mem-001", "mem-002"},
			SelectionScores: []workorchmemorycontract.RecallSelectionScore{
				{ID: "mem-001", Score: 1, Reason: "selected_within_context_budget"},
				{ID: "mem-002", Score: 2, Reason: "selected_within_context_budget"},
			},
			RetrievedMemoryHits: []workorchmemorycontract.RecallHit{{ID: "mem-001", Summary: "first"}, {ID: "mem-002", Summary: "second"}},
			ContextBudget: workorchmemorycontract.RecallContextBudget{
				MaxItems: 8, MaxBytes: 16 * 1024, MaxEstimatedTokens: 4096,
				UsedItems: 2, UsedBytes: 11, EstimatedTokens: 4,
			},
		},
	}}, "2026-06-04T00:00:00Z")
	if trace == nil {
		t.Fatal("BuildActivationMemoryRecallTrace returned nil")
	}
	if trace.ID != "run-001/memory-recall/activation" {
		t.Fatalf("trace ID = %s", trace.ID)
	}
	if trace.WorkspaceID != workspaceID || trace.RunID != runID {
		t.Fatalf("trace scope = workspace:%s run:%s", trace.WorkspaceID, trace.RunID)
	}
	if trace.Policy != "test_selection" || trace.Status != "selected" {
		t.Fatalf("trace policy/status = %s/%s", trace.Policy, trace.Status)
	}
	if len(trace.RetrievedMemoryIDs) != 2 || trace.RetrievedMemoryIDs[0] != "mem-001" || trace.RetrievedMemoryIDs[1] != "mem-002" {
		t.Fatalf("retrieved memory IDs = %#v", trace.RetrievedMemoryIDs)
	}
	if len(trace.SelectedMemoryIDs) != 2 || trace.SelectedMemoryIDs[0] != "mem-001" || trace.SelectedMemoryIDs[1] != "mem-002" {
		t.Fatalf("selected memory IDs = %#v", trace.SelectedMemoryIDs)
	}
}

func TestActiveRunDefinitionContextStale(t *testing.T) {
	workspaceID := common.WorkspaceID("ws-001")
	goal := &workspacemodel.Goal{ID: "goal-001", WorkspaceID: workspaceID, Text: "ship it"}
	dod := &workspacemodel.DoD{ID: "dod-001", WorkspaceID: workspaceID, Items: []string{"tests pass"}, State: common.DoDStateReady}
	run := &runmodel.Run{
		ID: "run-001", WorkspaceID: workspaceID, GoalID: goal.ID, DoDID: dod.ID,
		DefinitionRevision: "goal:goal-001|dod:dod-001",
		RuntimeBootstrap: &runtimecontract.RuntimeBootstrap{Definition: runtimecontract.RuntimeDefinitionContext{
			Revision: "goal:goal-001|dod:dod-001", GoalID: goal.ID, GoalDefinition: goal.Text,
			DoDID: dod.ID, DoDDefinition: "1. tests pass", CapabilityContract: []string{"shell/command"},
		}},
	}

	if ActiveRunDefinitionContextStale(run, goal, dod) {
		t.Fatal("matching definition context should not be stale")
	}

	changedGoal := *goal
	changedGoal.Text = "ship it differently"
	if !ActiveRunDefinitionContextStale(run, &changedGoal, dod) {
		t.Fatal("changed goal context should be stale")
	}

	changedDoD := *dod
	changedDoD.Items = []string{"tests pass", "docs updated"}
	if !ActiveRunDefinitionContextStale(run, goal, &changedDoD) {
		t.Fatal("changed DoD context should be stale")
	}

	otherGoal := *goal
	otherGoal.ID = "goal-002"
	if !ActiveRunDefinitionContextStale(run, &otherGoal, dod) {
		t.Fatal("changed goal ID should be stale")
	}
}

func TestWorkspaceMCPSelectionContextAutoBindsRequiredRefs(t *testing.T) {
	selection := WorkspaceMCPSelectionContext(
		[]string{"server-a", "server-a"},
		[]string{"server-default", "server-a"},
		[]string{"cap-a"},
		[]string{"cap-default", "cap-a"},
		[]workorchmanagementcontract.Skill{{
			RequiredMCPServerRefs: []string{"server-required", "server-a"},
			RequiredCapabilityRefs: []string{
				"cap-required",
				"cap-a",
			},
		}},
	)
	if got := selection.SelectedMCPServerIDs; len(got) != 3 || got[0] != "server-a" || got[1] != "server-default" || got[2] != "server-required" {
		t.Fatalf("selected MCP server IDs = %#v, want requested/default/required order", got)
	}
	if got := selection.AutoBoundMCPServerIDs; len(got) != 1 || got[0] != "server-required" {
		t.Fatalf("auto-bound MCP server IDs = %#v, want required-only missing ref", got)
	}
	if got := selection.SelectedCapabilityIDs; len(got) != 3 || got[0] != "cap-a" || got[1] != "cap-default" || got[2] != "cap-required" {
		t.Fatalf("selected capability IDs = %#v, want requested/default/required order", got)
	}
	if got := selection.AutoBoundCapabilityIDs; len(got) != 1 || got[0] != "cap-required" {
		t.Fatalf("auto-bound capability IDs = %#v, want required-only missing ref", got)
	}
	contextData := selection.Context()
	if contextData["policy"] != "template_default_and_skill_required_mcp_auto_bind" {
		t.Fatalf("context policy = %v", contextData["policy"])
	}
}

func TestDialogLanguageUsesRequestedThenFallback(t *testing.T) {
	if got := DialogLanguage("zh_CN", "en"); got != "zh-CN" {
		t.Fatalf("DialogLanguage requested = %q, want zh-CN", got)
	}
	if got := DialogLanguage("  ", " en "); got != "en" {
		t.Fatalf("DialogLanguage fallback = %q, want en", got)
	}
}

func TestCurrentWorkspaceDefinitionRevisionContextUsesEffectiveDefinitions(t *testing.T) {
	workspaceID := common.WorkspaceID("ws-revision-context")
	goal := &workspacemodel.Goal{ID: "goal-revision-context", WorkspaceID: workspaceID, Text: "ship"}
	dod := &workspacemodel.DoD{ID: "dod-revision-context", WorkspaceID: workspaceID, Items: []string{"done"}}

	revision := CurrentWorkspaceDefinitionRevisionContext(
		goal,
		[]map[string]any{{"id": "goal-old"}, {"id": string(goal.ID)}},
		dod,
		[]map[string]any{{"id": "dod-old"}, {"id": string(dod.ID)}},
	)
	if revision["policy"] != "workspace_goal_dod_cumulative" ||
		revision["effectiveGoalId"] != string(goal.ID) ||
		revision["effectiveDoDId"] != string(dod.ID) {
		t.Fatalf("definitionRevision = %+v, want effective definitions", revision)
	}
	if got := revision["goalIds"].([]string); len(got) != 2 || got[0] != "goal-old" || got[1] != string(goal.ID) {
		t.Fatalf("goalIds = %+v, want ordered goal ids", revision["goalIds"])
	}
	if got := revision["doDIds"].([]string); len(got) != 2 || got[0] != "dod-old" || got[1] != string(dod.ID) {
		t.Fatalf("doDIds = %+v, want ordered DoD ids", revision["doDIds"])
	}
}

func TestWorkspaceExecutionContractIncludesAcceptanceSlicePolicyFromHints(t *testing.T) {
	workspace := &workspacemodel.Workspace{
		ID:         "ws-policy",
		TemplateID: "template-policy",
	}
	reader := fakeWorkspaceTemplateReader{item: &workspacemodel.WorkspaceTemplate{
		ID: "template-policy",
		ContextHints: mustActivationPayload(t, map[string]any{
			"acceptanceSlicePolicy": map[string]any{
				"schemaVersion": "workorch.acceptance_slice_policy",
				"groups": []any{map[string]any{
					"id": "slice-neutral",
					"match": map[string]any{
						"sourceScopes": []any{"goal_leaf"},
					},
				}},
			},
		}),
	}}

	contract := WorkspaceExecutionContract(context.Background(), reader, workspace)

	policy, ok := contract["acceptanceSlicePolicy"].(map[string]any)
	if !ok {
		t.Fatalf("executionContract.acceptanceSlicePolicy = %+v, want map", contract["acceptanceSlicePolicy"])
	}
	groups, ok := policy["groups"].([]any)
	if !ok || len(groups) != 1 {
		t.Fatalf("acceptanceSlicePolicy.groups = %+v, want one group", policy["groups"])
	}
	group := groups[0].(map[string]any)
	if group["id"] != "slice-neutral" {
		t.Fatalf("acceptance slice group = %+v, want template policy group", group)
	}
}

func TestWorkspaceExecutionContractIncludesValidationPoliciesFromHints(t *testing.T) {
	workspace := &workspacemodel.Workspace{ID: "ws-validation", TemplateID: "template-validation"}
	reader := fakeWorkspaceTemplateReader{item: &workspacemodel.WorkspaceTemplate{
		ID: "template-validation",
		ContextHints: mustActivationPayload(t, map[string]any{"validationPolicies": []any{map[string]any{
			"id": "validation-1", "scope": "workspace_profile", "appliesToEvidenceKinds": []any{"validation"}, "maxValidationOnlyRetriesPerFocus": 2,
		}}}),
	}}
	contract := WorkspaceExecutionContract(context.Background(), reader, workspace)
	policies, ok := contract["validationPolicies"].([]any)
	if !ok || len(policies) != 1 || policies[0].(map[string]any)["id"] != "validation-1" {
		t.Fatalf("executionContract.validationPolicies = %+v", contract["validationPolicies"])
	}
}

func TestWorkspaceExecutionContractIncludesTemplateStagePoliciesFromHints(t *testing.T) {
	workspace := &workspacemodel.Workspace{
		ID:         "ws-stage-policy",
		TemplateID: "template-stage-policy",
	}
	reader := fakeWorkspaceTemplateReader{item: &workspacemodel.WorkspaceTemplate{
		ID: "template-stage-policy",
		ContextHints: mustActivationPayload(t, map[string]any{
			"templateStagePolicies": map[string]any{
				"schemaVersion": "workorch.template_stage_policy",
				"stages": []any{map[string]any{
					"id":          "neutral-stage",
					"soulBinding": "execution",
					"entry": map[string]any{
						"nextAction":                 "state_change",
						"requiresDefinitionRevision": true,
						"requiresGapBaseline":        true,
					},
					"requiredOutputs": []any{map[string]any{
						"kind":          "state_change",
						"evidenceKinds": []any{"state_change"},
					}},
					"completion": map[string]any{
						"policy": "required_outputs_accepted",
					},
				}},
			},
		}),
	}}

	contract := WorkspaceExecutionContract(context.Background(), reader, workspace)

	policy, ok := contract["templateStagePolicies"].(map[string]any)
	if !ok {
		t.Fatalf("executionContract.templateStagePolicies = %+v, want map", contract["templateStagePolicies"])
	}
	if policy["schemaVersion"] != "workorch.template_stage_policy" {
		t.Fatalf("templateStagePolicies schema = %v, want v1", policy["schemaVersion"])
	}
	stages, ok := policy["stages"].([]any)
	if !ok || len(stages) != 1 {
		t.Fatalf("templateStagePolicies.stages = %+v, want one stage", policy["stages"])
	}
	stage := stages[0].(map[string]any)
	if stage["id"] != "neutral-stage" || stage["soulBinding"] != "execution" {
		t.Fatalf("template stage = %+v, want neutral execution stage", stage)
	}

	source := reader.item.ContextHints.Map()["templateStagePolicies"].(map[string]any)
	source["schemaVersion"] = "mutated"
	if policy["schemaVersion"] != "workorch.template_stage_policy" {
		t.Fatalf("templateStagePolicies was not cloned: %+v", policy)
	}
}

func TestDefinitionChangeImpactUsesCurrentModelRevisionWhenEventProjectionLags(t *testing.T) {
	goal := &workspacemodel.Goal{
		ID:                     "goal-current",
		WorkspaceID:            "ws-definition-lag",
		Text:                   "updated goal",
		DefinitionRevision:     "goal:updated",
		PreviousDefinitionHash: "goal:previous",
	}
	dod := &workspacemodel.DoD{
		ID:          "dod-current",
		WorkspaceID: "ws-definition-lag",
		Items:       []string{"current dod"},
		State:       common.DoDStateReady,
	}

	impact := DefinitionChangeImpactFromDefinitions(
		goal,
		[]map[string]any{GoalContext(goal)},
		nil,
		dod,
		[]map[string]any{DoDContext(dod)},
		nil,
	)

	if !DefinitionChangeImpactChanged(impact) {
		t.Fatalf("definitionChangeImpact = %+v, want changed from current model previous hash", impact)
	}
	goalChange := impact["goalChange"].(map[string]any)
	revisions := goalChange["definitionRevisions"].([]map[string]any)
	if len(revisions) != 1 ||
		revisions[0]["id"] != "goal-current" ||
		revisions[0]["source"] != "current_definition_model" ||
		revisions[0]["previousDefinitionHash"] != "goal:previous" {
		t.Fatalf("goal definitionRevisions = %+v, want current model revision marker", revisions)
	}
}
