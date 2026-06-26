package llm

import (
	"fmt"
	"strings"
	"testing"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
)

func TestBuildProviderPromptMessagesUsesMultiTurnStructure(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID:    "ws-1",
		RunID:          "run-1",
		StepID:         "step-1",
		Kind:           llmcommon.StepKindWorkspaceExecution,
		Model:          "model",
		Schema:         decisionSchemaName,
		DialogLanguage: "Chinese (Simplified)",
		Input: map[string]any{
			"executionContext": map[string]any{
				"goal": "goal line 1\n\ngoal line 2",
				"dod":  "dod line 1\n\ndod line 2",
			},
		},
	}
	build := buildProviderPromptMessages(req)
	if len(build.Messages) < 4 {
		t.Fatalf("messages=%d, want multi-turn prompt messages", len(build.Messages))
	}
	if build.Messages[0].Role != "system" {
		t.Fatalf("first role = %q, want system", build.Messages[0].Role)
	}
	joined := ""
	for _, item := range build.Messages {
		joined += "\n" + item.Content
	}
	if !strings.Contains(joined, "Allowed top-level JSON fields") ||
		!strings.Contains(joined, "validationPlan") {
		t.Fatalf("prompt missing explicit decision schema field guidance, joined=%s", joined)
	}
}

func TestBuildProviderPromptMessagesUsesStructuredJSONPromptForNonDecisionSchema(t *testing.T) {
	req := llmmodels.ProviderRequest{
		Kind:           "workspace/setup_draft",
		Model:          "model",
		Schema:         "workorch.workspaceSetupDraft.v1",
		DialogLanguage: "zh-CN",
		Input: map[string]any{
			"instructions": []any{"Generate a draft"},
			"outputSchema": map[string]any{
				"properties": map[string]any{"goalText": map[string]any{"type": "string"}},
			},
		},
	}
	build := buildProviderPromptMessages(req)
	joined := ""
	for _, item := range build.Messages {
		joined += "\n" + item.Content
	}
	if !strings.Contains(joined, "workorch.workspaceSetupDraft.v1") {
		t.Fatalf("prompt missing requested schema, joined=%s", joined)
	}
	if strings.Contains(joined, "LLMExecutionDecision contract") ||
		strings.Contains(joined, "Plan the next workspace execution step") ||
		strings.Contains(joined, "completionState must be one of") {
		t.Fatalf("non-decision prompt contains execution decision contract, joined=%s", joined)
	}
	if !strings.Contains(joined, "Do not return workorch.llmExecutionDecision.v1 fields") {
		t.Fatalf("non-decision prompt missing schema isolation guidance, joined=%s", joined)
	}
}

func TestBuildProviderPromptMessagesProtectsGoalAndNormalizesOtherSections(t *testing.T) {
	rawGoal := "**Raw Goal**\n\n- item 1"
	req := llmmodels.ProviderRequest{
		WorkspaceID:    "ws-2",
		RunID:          "run-2",
		StepID:         "step-2",
		Kind:           llmcommon.StepKindWorkspaceExecution,
		Model:          "model",
		Schema:         decisionSchemaName,
		DialogLanguage: "English",
		Input: map[string]any{
			"executionContext": map[string]any{
				"goal": rawGoal,
				"dod":  "dod",
			},
			"failureReason": "### heading\n\n\n**bold** content",
		},
	}
	build := buildProviderPromptMessages(req)
	joined := ""
	for _, item := range build.Messages {
		joined += "\n" + item.Content
	}
	if !strings.Contains(joined, "**Raw Goal**") || !strings.Contains(joined, "- item 1") {
		t.Fatalf("prompt missing exact raw goal content")
	}
	if len(build.ProtectedSections) == 0 {
		t.Fatalf("protected sections missing")
	}
	if len(build.NormalizedSections) == 0 {
		t.Fatalf("normalized sections missing")
	}
}

func TestBuildPromptContextUsesCacheAwareSectionMetadata(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID:    "ws-cache",
		RunID:          "run-003",
		StepID:         "step-001",
		Kind:           llmcommon.StepKindWorkspaceExecution,
		Model:          "model",
		Schema:         decisionSchemaName,
		DialogLanguage: "English",
		Input: map[string]any{
			"executionContextPackage": map[string]any{
				"goal":               map[string]any{"text": "goal text"},
				"dod":                map[string]any{"items": []any{"dod item"}},
				"definitionRevision": "definition-004",
				"definitionChangeImpact": map[string]any{
					"changed": true,
					"policy":  "workspace_goal_dod_cumulative_latest_refines_previous",
				},
			},
			"currentRuntimeStage":     "execution",
			"currentRuntimeStageMode": "strict_convergence",
			"currentProgressDigest": map[string]any{
				"currentGap":         "missing verification evidence",
				"nextRequiredAction": "run validation",
				"lastObservedFailure": map[string]any{
					"sourceRef":    "run-003/steps/step-002",
					"failureClass": "validation_failed",
				},
			},
			"processEvidenceTimeline": []any{
				map[string]any{"order": 1, "sourceRef": "run-003/steps/step-001", "eventType": "tool_result"},
				map[string]any{"order": 2, "sourceRef": "run-003/steps/step-002", "eventType": "validation_result"},
			},
		},
	}

	context := buildPromptContextFromRequest(req)
	if context == nil {
		t.Fatalf("promptContext = nil")
	}
	if context.SchemaVersion != "workorch.promptContext.cacheAware.v1" {
		t.Fatalf("schemaVersion = %q, want cache-aware schema", context.SchemaVersion)
	}
	if len(context.Sections) != len(context.Turns) {
		t.Fatalf("sections=%d turns=%d, want one section per turn", len(context.Sections), len(context.Turns))
	}
	for idx, turn := range context.Turns {
		if turn.SectionID == "" {
			t.Fatalf("turn %d missing sectionId", idx)
		}
		if turn.SectionID != context.Sections[idx].ID {
			t.Fatalf("turn %d sectionId=%q section=%q, want preserved order mapping", idx, turn.SectionID, context.Sections[idx].ID)
		}
	}
	required := map[string]string{
		"C0.system":                    "stable",
		"C1.goal-contract-original":    "workspace_definition",
		"C1.dod-contract-original":     "workspace_definition",
		"C2.capability-contract":       "capability_contract",
		"C3.stage-contract":            "stage_bundle",
		"C4.runtime-stage":             "volatile_tail",
		"C4.current-failure":           "volatile_tail",
		"C4.current-progress":          "volatile_tail",
		"C4.process-evidence-timeline": "volatile_tail",
		"C4.definition-change-impact":  "volatile_tail",
		"C4.current-action":            "volatile_tail",
	}
	for sectionID, cacheability := range required {
		section, ok := promptContextSectionByID(context, sectionID)
		if !ok {
			t.Fatalf("missing section %s; sections=%+v", sectionID, context.Sections)
		}
		if section.Cacheability != cacheability {
			t.Fatalf("section %s cacheability=%q, want %q", sectionID, section.Cacheability, cacheability)
		}
		if section.Hash == "" || !strings.HasPrefix(section.Hash, "sha256:") {
			t.Fatalf("section %s hash=%q, want sha256 hash", sectionID, section.Hash)
		}
	}
	goalSection, _ := promptContextSectionByID(context, "C1.goal-contract-original")
	if goalSection.DefinitionRevision != "definition-004" {
		t.Fatalf("goal definition revision=%q, want definition-004", goalSection.DefinitionRevision)
	}
	if !goalSection.Protected || goalSection.Policy != "raw_required" {
		t.Fatalf("goal section protection = protected:%v policy:%q, want raw_required protected", goalSection.Protected, goalSection.Policy)
	}
	stageSection, _ := promptContextSectionByID(context, "C3.stage-contract")
	if !strings.Contains(stageSection.Content, "definition-004") || !strings.Contains(stageSection.Content, "definitionChangeImpact") {
		t.Fatalf("stage section missing definition revision/impact: %s", stageSection.Content)
	}
	impactSection, _ := promptContextSectionByID(context, "C4.definition-change-impact")
	if !strings.Contains(impactSection.Content, "workspace_goal_dod_cumulative_latest_refines_previous") {
		t.Fatalf("definition impact section missing policy: %s", impactSection.Content)
	}
}

func TestBuildPromptContextIncludesAcceptanceFocusAsVolatileTail(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID:    "ws-acceptance",
		RunID:          "run-acceptance",
		StepID:         "step-acceptance",
		Kind:           llmcommon.StepKindWorkspaceExecution,
		Model:          "model",
		Schema:         decisionSchemaName,
		DialogLanguage: "English",
		Input: map[string]any{
			"executionContext": map[string]any{
				"goal": map[string]any{"text": "goal text"},
				"dod":  map[string]any{"items": []any{"dod item"}},
			},
			"acceptance": map[string]any{
				"activeSliceId": "slice-001",
				"activeSlice": map[string]any{
					"id":                  "slice-001",
					"coverageWorkItemIds": []any{"cwi-001"},
				},
				"currentFocus": map[string]any{
					"id":                  "focus-cwi-001",
					"coverageWorkItemIds": []any{"cwi-001"},
				},
			},
		},
	}

	context := buildPromptContextFromRequest(req)

	section, ok := promptContextSectionByID(context, "C4.acceptance-focus")
	if !ok {
		t.Fatalf("missing C4.acceptance-focus; sections=%+v", promptContextSectionIDsForTest(context))
	}
	if section.Cacheability != "volatile_tail" {
		t.Fatalf("acceptance focus cacheability=%q, want volatile_tail", section.Cacheability)
	}
	if !strings.Contains(section.Content, "slice-001") || !strings.Contains(section.Content, "focus-cwi-001") {
		t.Fatalf("acceptance focus content = %s, want active slice and focus", section.Content)
	}
	c0, ok := promptContextSectionByID(context, "C0.system")
	if !ok {
		t.Fatalf("missing C0.system")
	}
	if strings.Contains(c0.Content, "focus-cwi-001") || strings.Contains(c0.Content, "slice-001") {
		t.Fatalf("C0 leaked dynamic acceptance focus: %s", c0.Content)
	}
}

func TestBuildPromptContextSuppressesAcceptanceFocusWhenStagePacketHasActiveFocus(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID:    "ws-acceptance-authority",
		RunID:          "run-acceptance-authority",
		StepID:         "step-acceptance-authority",
		Kind:           llmcommon.StepKindWorkspaceExecution,
		Model:          "model",
		Schema:         decisionSchemaName,
		DialogLanguage: "English",
		Input: map[string]any{
			"stagePacket": map[string]any{
				"activeFocus": map[string]any{
					"id": "cwi-current",
				},
			},
			"acceptance": map[string]any{
				"activeSliceId": "slice-stale",
				"currentFocus": map[string]any{
					"id":                  "focus-cwi-stale",
					"coverageWorkItemIds": []any{"cwi-stale"},
				},
			},
		},
	}

	context := buildPromptContextFromRequest(req)
	if section, ok := promptContextSectionByID(context, "C4.acceptance-focus"); ok {
		t.Fatalf("C4.acceptance-focus = %s, want suppressed when C3 stagePacket has activeFocus", section.Content)
	}
	contract, ok := promptContextSectionByID(context, "C3.stage-packet-contract")
	if !ok {
		t.Fatalf("missing C3.stage-packet-contract; sections=%+v", promptContextSectionIDsForTest(context))
	}
	if !strings.Contains(contract.Content, "cwi-current") || strings.Contains(contract.Content, "focus-cwi-stale") {
		t.Fatalf("stage packet contract content = %s, want only authority focus", contract.Content)
	}
}

func TestBuildPromptContextKeepsCacheableSectionsAsContiguousPrefix(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID:    "ws-cache-prefix",
		RunID:          "run-001",
		StepID:         "step-001",
		Kind:           llmcommon.StepKindWorkspaceExecution,
		Model:          "model",
		Schema:         decisionSchemaName,
		DialogLanguage: "English",
		RuntimeCapabilities: llmmodels.RuntimeCapabilities{
			BlockKinds: []llmmodels.RuntimeBlockCapability{{Kind: "file/read", Description: "read file"}},
		},
		Input: map[string]any{
			"executionContext": map[string]any{
				"goal": "goal text",
				"dod":  "dod text",
			},
			"currentRuntimeStage": "execution",
			"currentProgressDigest": map[string]any{
				"nextRequiredAction": "continue",
			},
		},
	}

	context := buildPromptContextFromRequest(req)
	if context == nil {
		t.Fatalf("promptContext = nil")
	}
	seenDynamic := false
	for _, section := range context.Sections {
		group := promptSectionGroup(section.ID)
		if group == "C3" || group == "C4" || group == "C5" {
			seenDynamic = true
			continue
		}
		if (group == "C0" || group == "C1" || group == "C2") && seenDynamic {
			t.Fatalf("cacheable section %s appears after dynamic sections; order=%+v", section.ID, promptContextSectionIDsForTest(context))
		}
	}
}

func TestProviderCachePlanKeepsCacheablePrefixStableAcrossVolatileProgress(t *testing.T) {
	baseInput := map[string]any{
		"executionContextPackage": map[string]any{
			"goal":               map[string]any{"text": "stable goal"},
			"dod":                map[string]any{"items": []any{"stable dod"}},
			"definitionRevision": "definition-stable",
		},
		"currentRuntimeStage":     "execution",
		"currentRuntimeStageMode": "strict_convergence",
		"currentProgressDigest": map[string]any{
			"latestStep": "step-a",
			"summary":    "first volatile progress",
		},
	}
	changedInput := cloneMap(baseInput)
	changedInput["currentProgressDigest"] = map[string]any{
		"latestStep": "step-b",
		"summary":    "second volatile progress",
	}
	changedInput["operatorInputQueue"] = []any{"continue with next step"}

	base := llmmodels.ProviderRequest{
		WorkspaceID:    "ws-cache-volatile",
		RunID:          "run-001",
		StepID:         "step-001",
		Kind:           llmcommon.StepKindWorkspaceExecution,
		Model:          "model",
		Schema:         decisionSchemaName,
		DialogLanguage: "English",
		RuntimeCapabilities: llmmodels.RuntimeCapabilities{
			BlockKinds: []llmmodels.RuntimeBlockCapability{
				{Kind: llmcommon.StepKindWorkspaceExecution, Description: "plan next step"},
				{Kind: "file/read", Description: "read file"},
			},
		},
		Input: baseInput,
	}
	changed := base
	changed.Input = changedInput
	base.PromptContext = buildPromptContextFromRequest(base)
	changed.PromptContext = buildPromptContextFromRequest(changed)
	attachProviderCachePlan(&base)
	attachProviderCachePlan(&changed)

	basePlan := base.ContextBudget.CachePlan
	changedPlan := changed.ContextBudget.CachePlan
	if basePlan == nil || changedPlan == nil {
		t.Fatalf("cache plan missing: base=%+v changed=%+v", base.ContextBudget, changed.ContextBudget)
	}
	if basePlan.StablePrefixHash != changedPlan.StablePrefixHash ||
		basePlan.RenderedStablePrefixHash != changedPlan.RenderedStablePrefixHash ||
		basePlan.WorkspaceDefinitionHash != changedPlan.WorkspaceDefinitionHash ||
		basePlan.CapabilityContractHash != changedPlan.CapabilityContractHash ||
		basePlan.CacheKey != changedPlan.CacheKey {
		t.Fatalf("cacheable prefix changed under volatile progress: base=%+v changed=%+v", basePlan, changedPlan)
	}
	if basePlan.RenderedStablePrefixHash == "" {
		t.Fatalf("rendered stable prefix hash is empty: %+v", basePlan)
	}
	if basePlan.RenderedStablePrefixHash == basePlan.StablePrefixHash {
		t.Fatalf("rendered stable prefix hash = C0 stable hash %q, want distinct rendered C0/C1/C2 hash", basePlan.RenderedStablePrefixHash)
	}
	if basePlan.VolatileTailHash == changedPlan.VolatileTailHash {
		t.Fatalf("volatile tail hash unchanged: %q", basePlan.VolatileTailHash)
	}
	if !stringSliceContains(basePlan.AdapterHint.CacheablePrefix, "C1.goal-contract-original") ||
		stringSliceContains(basePlan.AdapterHint.CacheablePrefix, "C4.current-progress") ||
		stringSliceContains(basePlan.AdapterHint.CacheablePrefix, "C4.current-action") {
		t.Fatalf("cacheable prefix sections = %+v, want C0/C1/C2 only", basePlan.AdapterHint.CacheablePrefix)
	}
}

func TestProviderCachePlanKeepsStablePrefixAcrossCoverageFocusChanges(t *testing.T) {
	baseInput := map[string]any{
		"executionContextPackage": map[string]any{
			"goal":               map[string]any{"text": "stable goal"},
			"dod":                map[string]any{"items": []any{"stable dod"}},
			"definitionRevision": "definition-stable",
			"acceptance": map[string]any{
				"activeSliceId": "slice-required",
				"currentFocus": map[string]any{
					"id":                  "focus-cwi-001",
					"coverageWorkItemIds": []any{"cwi-001"},
					"targetRefs":          []any{"target-a"},
				},
			},
			"coverageFocus": map[string]any{
				"id":                  "focus-cwi-001",
				"coverageWorkItemIds": []any{"cwi-001"},
				"targetRefs":          []any{"target-a"},
			},
		},
		"currentRuntimeStage":     "repair",
		"currentRuntimeStageMode": "strict_convergence",
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{"mustBindCoverageFocus": true, "coverageFocusId": "focus-cwi-001"},
		},
	}
	changedInput := cloneMap(baseInput)
	changedInput["executionContextPackage"] = map[string]any{
		"goal":               map[string]any{"text": "stable goal"},
		"dod":                map[string]any{"items": []any{"stable dod"}},
		"definitionRevision": "definition-stable",
		"acceptance": map[string]any{
			"activeSliceId": "slice-required",
			"currentFocus": map[string]any{
				"id":                  "focus-cwi-002",
				"coverageWorkItemIds": []any{"cwi-002"},
				"targetRefs":          []any{"target-b"},
			},
		},
		"coverageFocus": map[string]any{
			"id":                  "focus-cwi-002",
			"coverageWorkItemIds": []any{"cwi-002"},
			"targetRefs":          []any{"target-b"},
		},
	}
	changedInput["repairActionPackage"] = map[string]any{
		"requiredOutput": map[string]any{"mustBindCoverageFocus": true, "coverageFocusId": "focus-cwi-002"},
	}

	base := llmmodels.ProviderRequest{
		WorkspaceID:    "ws-cache-focus",
		RunID:          "run-001",
		StepID:         "step-001",
		Kind:           llmcommon.StepKindWorkspaceExecution,
		Model:          "model",
		Schema:         decisionSchemaName,
		DialogLanguage: "English",
		RuntimeCapabilities: llmmodels.RuntimeCapabilities{
			BlockKinds: []llmmodels.RuntimeBlockCapability{{Kind: "file/read", Description: "read file"}},
		},
		Input: baseInput,
	}
	changed := base
	changed.Input = changedInput
	base.PromptContext = buildPromptContextFromRequest(base)
	changed.PromptContext = buildPromptContextFromRequest(changed)
	attachProviderCachePlan(&base)
	attachProviderCachePlan(&changed)

	basePlan := base.ContextBudget.CachePlan
	changedPlan := changed.ContextBudget.CachePlan
	if basePlan == nil || changedPlan == nil {
		t.Fatalf("cache plan missing: base=%+v changed=%+v", base.ContextBudget, changed.ContextBudget)
	}
	if basePlan.StablePrefixHash != changedPlan.StablePrefixHash ||
		basePlan.RenderedStablePrefixHash != changedPlan.RenderedStablePrefixHash ||
		basePlan.WorkspaceDefinitionHash != changedPlan.WorkspaceDefinitionHash ||
		basePlan.CapabilityContractHash != changedPlan.CapabilityContractHash ||
		basePlan.CacheKey != changedPlan.CacheKey {
		t.Fatalf("cacheable prefix changed under focus change: base=%+v changed=%+v", basePlan, changedPlan)
	}
	if basePlan.VolatileTailHash == changedPlan.VolatileTailHash {
		t.Fatalf("volatile tail hash unchanged across focus change: %q", basePlan.VolatileTailHash)
	}
}

func TestStagePacketRendersInDynamicPromptPartitions(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID:    "ws-stage-packet",
		RunID:          "run-001",
		StepID:         "step-001",
		Kind:           llmcommon.StepKindWorkspaceExecution,
		Model:          "model",
		Schema:         decisionSchemaName,
		DialogLanguage: "English",
		Input: map[string]any{
			"executionContextPackage": map[string]any{
				"goal":               map[string]any{"text": "stable goal"},
				"dod":                map[string]any{"items": []any{"stable dod"}},
				"definitionRevision": "definition-stable",
			},
			"stagePacket": map[string]any{
				"schemaVersion": "workorch.stage_packet.v1",
				"workspaceId":   "ws-stage-packet",
				"runId":         "run-001",
				"stepId":        "step-001",
				"definition": map[string]any{
					"revision": "definition-stable",
					"goalId":   "goal-001",
					"doDId":    "dod-001",
				},
				"activeStage": "state_change",
				"activeFocus": map[string]any{
					"coverageItemId": "coverage-001",
				},
				"requiredOutput": map[string]any{
					"mustUseCurrentTargetSnapshot": true,
				},
				"allowedActions":        []string{"state_change"},
				"forbiddenActions":      []string{"complete_without_evidence"},
				"requiredEvidenceKinds": []string{"state_change"},
				"acceptedEvidenceRefs":  []string{"accepted-ref"},
				"rejectedEvidenceRefs":  []string{"rejected-ref"},
				"idempotencyKey":        "stage-packet-key",
				"targetSnapshots": []map[string]any{{
					"targetRef":             "target-truncated",
					"contentHash":           "sha256-target-truncated",
					"content":               "secret full content must not render",
					"contentPreview":        "secret preview must not render",
					"contentComplete":       false,
					"safeForFileWrite":      false,
					"safeForExactFileWrite": false,
					"truncated":             true,
					"integrityState":        "truncated",
					"source":                "stagePacket",
					"contextSource":         "runtime",
				}},
			},
		},
	}

	context := buildPromptContextFromRequest(req)
	contract, ok := promptContextSectionByID(context, "C3.stage-packet-contract")
	if !ok {
		t.Fatalf("missing C3.stage-packet-contract; sections=%+v", promptContextSectionIDsForTest(context))
	}
	for _, want := range []string{
		"Stage Packet Contract",
		"mustUseCurrentTargetSnapshot",
		"accepted-ref",
		"rejected-ref",
		"stage-packet-key",
	} {
		if !strings.Contains(contract.Content, want) {
			t.Fatalf("stage packet contract missing %q: %s", want, contract.Content)
		}
	}
	if strings.Contains(contract.Content, "secret full content") || strings.Contains(contract.Content, "secret preview") {
		t.Fatalf("stage packet contract leaked snapshot body: %s", contract.Content)
	}

	snapshots, ok := promptContextSectionByID(context, "C5.stage-packet-target-snapshots")
	if !ok {
		t.Fatalf("missing C5.stage-packet-target-snapshots; sections=%+v", promptContextSectionIDsForTest(context))
	}
	for _, want := range []string{"target-truncated", "sha256-target-truncated", "integrityState", "truncated", "safeForExactFileWrite"} {
		if !strings.Contains(snapshots.Content, want) {
			t.Fatalf("stage packet snapshot metadata missing %q: %s", want, snapshots.Content)
		}
	}
	for _, forbidden := range []string{"secret full content", "secret preview", "contentPreview"} {
		if strings.Contains(snapshots.Content, forbidden) {
			t.Fatalf("stage packet snapshot metadata leaked %q: %s", forbidden, snapshots.Content)
		}
	}

	req.PromptContext = context
	prompt := buildProviderFacingPrompt(req)
	for _, forbidden := range []string{"mustUseCurrentTargetSnapshot", "accepted-ref", "target-truncated", "stage-packet-key"} {
		if strings.Contains(prompt.StablePrefix, forbidden) {
			t.Fatalf("stable prefix leaked stage packet value %q: %s", forbidden, prompt.StablePrefix)
		}
	}
	dynamic := ""
	for _, message := range prompt.DynamicMessages {
		dynamic += "\n" + message.SectionID + "\n" + message.Content
	}
	for _, want := range []string{"C3.stage-packet-contract", "C5.stage-packet-target-snapshots", "mustUseCurrentTargetSnapshot", "target-truncated"} {
		if !strings.Contains(dynamic, want) {
			t.Fatalf("dynamic tail missing %q: %s", want, dynamic)
		}
	}
}

func TestProviderCachePlanKeepsStablePrefixAcrossStagePacketChanges(t *testing.T) {
	baseInput := map[string]any{
		"executionContextPackage": map[string]any{
			"goal":               map[string]any{"text": "stable goal"},
			"dod":                map[string]any{"items": []any{"stable dod"}},
			"definitionRevision": "definition-stable",
		},
		"stagePacket": map[string]any{
			"schemaVersion": "workorch.stage_packet.v1",
			"workspaceId":   "ws-stage-cache",
			"runId":         "run-001",
			"stepId":        "step-001",
			"definition":    map[string]any{"revision": "definition-stable"},
			"activeStage":   "state_change",
			"requiredOutput": map[string]any{
				"mustUseCurrentTargetSnapshot": true,
				"targetRef":                    "target-a",
			},
			"acceptedEvidenceRefs": []string{"accepted-a"},
			"targetSnapshots": []map[string]any{{
				"targetRef":             "target-a",
				"contentHash":           "sha256-a",
				"contentComplete":       true,
				"safeForFileWrite":      true,
				"safeForExactFileWrite": true,
				"integrityState":        "full",
			}},
		},
	}
	changedInput := cloneMap(baseInput)
	changedInput["stagePacket"] = map[string]any{
		"schemaVersion": "workorch.stage_packet.v1",
		"workspaceId":   "ws-stage-cache",
		"runId":         "run-001",
		"stepId":        "step-002",
		"definition":    map[string]any{"revision": "definition-stable"},
		"activeStage":   "state_change",
		"requiredOutput": map[string]any{
			"mustUseCurrentTargetSnapshot": true,
			"targetRef":                    "target-b",
		},
		"acceptedEvidenceRefs": []string{"accepted-b"},
		"rejectedEvidenceRefs": []string{"rejected-b"},
		"targetSnapshots": []map[string]any{{
			"targetRef":             "target-b",
			"contentHash":           "sha256-b",
			"contentComplete":       false,
			"safeForFileWrite":      false,
			"safeForExactFileWrite": false,
			"truncated":             true,
			"integrityState":        "truncated",
		}},
	}

	base := llmmodels.ProviderRequest{
		WorkspaceID:    "ws-stage-cache",
		RunID:          "run-001",
		StepID:         "step-001",
		Kind:           llmcommon.StepKindWorkspaceExecution,
		Model:          "model",
		Schema:         decisionSchemaName,
		DialogLanguage: "English",
		Input:          baseInput,
	}
	changed := base
	changed.StepID = "step-002"
	changed.Input = changedInput
	base.PromptContext = buildPromptContextFromRequest(base)
	changed.PromptContext = buildPromptContextFromRequest(changed)
	attachProviderCachePlan(&base)
	attachProviderCachePlan(&changed)

	basePlan := base.ContextBudget.CachePlan
	changedPlan := changed.ContextBudget.CachePlan
	if basePlan == nil || changedPlan == nil {
		t.Fatalf("cache plan missing: base=%+v changed=%+v", base.ContextBudget, changed.ContextBudget)
	}
	if basePlan.StablePrefixHash != changedPlan.StablePrefixHash ||
		basePlan.RenderedStablePrefixHash != changedPlan.RenderedStablePrefixHash ||
		basePlan.WorkspaceDefinitionHash != changedPlan.WorkspaceDefinitionHash ||
		basePlan.CapabilityContractHash != changedPlan.CapabilityContractHash ||
		basePlan.CacheKey != changedPlan.CacheKey {
		t.Fatalf("cacheable prefix changed under stage packet mutation: base=%+v changed=%+v", basePlan, changedPlan)
	}
	if basePlan.StageBundleHash == changedPlan.StageBundleHash {
		t.Fatalf("stage bundle hash unchanged across stage packet contract mutation: %q", basePlan.StageBundleHash)
	}
	if basePlan.LargeRefsHash == changedPlan.LargeRefsHash {
		t.Fatalf("large refs hash unchanged across stage packet snapshot mutation: %q", basePlan.LargeRefsHash)
	}
}

func TestCompactSummaryIsAuxiliaryToLedgerAndStagePacket(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID:    "ws-summary-authority",
		RunID:          "run-001",
		StepID:         "step-001",
		Kind:           llmcommon.StepKindWorkspaceExecution,
		Model:          "model",
		Schema:         decisionSchemaName,
		DialogLanguage: "English",
		Input: map[string]any{
			"executionContextPackage": map[string]any{
				"goal":               map[string]any{"text": "stable goal"},
				"dod":                map[string]any{"items": []any{"stable dod"}},
				"definitionRevision": "definition-summary-authority",
			},
			"currentProgressDigest": map[string]any{
				"state":              "completed",
				"summary":            "compact summary claims completion",
				"nextRequiredAction": "complete",
			},
			"coverageWorkItemContext": map[string]any{
				"schemaVersion":        "workorch.coverageWorkItemContext.v1",
				"definitionRevision":   "definition-summary-authority",
				"coverageBaselineHash": "sha256-ledger",
				"summary":              map[string]any{"open": 1, "satisfied": 0},
				"currentFocus": map[string]any{
					"id":                           "coverage-open-001",
					"definitionRevision":           "definition-summary-authority",
					"sourceScope":                  "dod",
					"sourceRequirementId":          "dod-001",
					"targetText":                   "stable dod",
					"currentGap":                   "state-changing evidence still missing",
					"status":                       "open",
					"missingEvidenceKinds":         []string{"state_change"},
					"acceptedEvidenceRefs":         []string{},
					"rejectedEvidenceRefs":         []string{"rejected-summary-claim"},
					"nextAllowedConvergenceStates": []string{"state_change"},
				},
				"unresolvedItems": []map[string]any{{
					"id":                   "coverage-open-001",
					"definitionRevision":   "definition-summary-authority",
					"sourceScope":          "dod",
					"sourceRequirementId":  "dod-001",
					"targetText":           "stable dod",
					"currentGap":           "state-changing evidence still missing",
					"status":               "open",
					"missingEvidenceKinds": []string{"state_change"},
					"acceptedEvidenceRefs": []string{},
					"rejectedEvidenceRefs": []string{"rejected-summary-claim"},
				}},
			},
			"stagePacket": map[string]any{
				"schemaVersion": "workorch.stage_packet.v1",
				"workspaceId":   "ws-summary-authority",
				"runId":         "run-001",
				"stepId":        "step-001",
				"definition":    map[string]any{"revision": "definition-summary-authority", "doDId": "dod-001"},
				"activeStage":   "state_change",
				"requiredOutput": map[string]any{
					"mustProduceStateChangingEvidence": true,
					"coverageItemId":                   "coverage-open-001",
				},
				"requiredEvidenceKinds": []string{"state_change"},
				"rejectedEvidenceRefs":  []string{"rejected-summary-claim"},
			},
			"processEvidenceTimeline": []map[string]any{{
				"stepId":       "run-001/summary-001",
				"sourceRef":    "step://run-001/summary-001",
				"summary":      "historical summary says done",
				"evidenceRefs": []string{"rejected-summary-claim"},
			}},
		},
	}

	context := buildPromptContextFromRequest(req)
	progress, ok := promptContextSectionByID(context, "C4.current-progress")
	if !ok {
		t.Fatalf("missing C4.current-progress; sections=%+v", promptContextSectionIDsForTest(context))
	}
	for _, want := range []string{"compact summary claims completion", "auxiliary_reference_only", "summary-only completion"} {
		if !strings.Contains(progress.Content, want) {
			t.Fatalf("current progress missing %q: %s", want, progress.Content)
		}
	}

	coverage, ok := promptContextSectionByID(context, "C4.coverage-work-items")
	if !ok {
		t.Fatalf("missing C4.coverage-work-items; sections=%+v", promptContextSectionIDsForTest(context))
	}
	for _, want := range []string{"coverage_ledger_authoritative", "coverage-open-001", "state-changing evidence still missing", "rejected-summary-claim"} {
		if !strings.Contains(coverage.Content, want) {
			t.Fatalf("coverage ledger signal missing %q: %s", want, coverage.Content)
		}
	}

	packet, ok := promptContextSectionByID(context, "C3.stage-packet-contract")
	if !ok {
		t.Fatalf("missing C3.stage-packet-contract; sections=%+v", promptContextSectionIDsForTest(context))
	}
	for _, want := range []string{"mustProduceStateChangingEvidence", "coverage-open-001", "requiredEvidenceKinds"} {
		if !strings.Contains(packet.Content, want) {
			t.Fatalf("stage packet contract missing %q: %s", want, packet.Content)
		}
	}

	timeline, ok := promptContextSectionByID(context, "C4.process-evidence-timeline")
	if !ok {
		t.Fatalf("missing C4.process-evidence-timeline; sections=%+v", promptContextSectionIDsForTest(context))
	}
	for _, want := range []string{"historical summary says done", "auxiliary_reference_only", "sourceRef_or_payloadRef_or_contentHash_required_per_event"} {
		if !strings.Contains(timeline.Content, want) {
			t.Fatalf("timeline missing %q: %s", want, timeline.Content)
		}
	}
}

func TestBuildProviderFacingPromptGroupsStablePrefixAndDynamicTail(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID:    "ws-provider-prefix",
		RunID:          "run-001",
		StepID:         "step-001",
		Kind:           llmcommon.StepKindWorkspaceExecution,
		Model:          "model",
		Schema:         decisionSchemaName,
		DialogLanguage: "English",
		RuntimeCapabilities: llmmodels.RuntimeCapabilities{
			BlockKinds: []llmmodels.RuntimeBlockCapability{{Kind: "file/read", Description: "read file"}},
		},
		Input: map[string]any{
			"executionContext": map[string]any{
				"goal": "goal text",
				"dod":  "dod text",
			},
			"currentRuntimeStage": "execution",
			"currentProgressDigest": map[string]any{
				"nextRequiredAction": "continue",
			},
		},
	}
	req.PromptContext = buildPromptContextFromRequest(req)

	prompt := buildProviderFacingPrompt(req)
	if prompt.StablePrefix == "" || len(prompt.StablePrefixSectionIDs) == 0 {
		t.Fatalf("stable prefix missing: %+v", prompt)
	}
	for _, want := range []string{"C0.system", "C1.goal-contract-original", "C2.runtime-capability-summary", "goal text", "runtimeCapabilities"} {
		if !strings.Contains(prompt.StablePrefix, want) {
			t.Fatalf("stable prefix missing %q: %s", want, prompt.StablePrefix)
		}
	}
	for _, forbidden := range []string{"C3.workspace-run", "C4.current-progress", "C4.current-action", "nextRequiredAction"} {
		if strings.Contains(prompt.StablePrefix, forbidden) {
			t.Fatalf("stable prefix leaked dynamic section %q: %s", forbidden, prompt.StablePrefix)
		}
	}
	dynamic := ""
	for _, message := range prompt.DynamicMessages {
		dynamic += "\n" + message.SectionID + "\n" + message.Content
	}
	for _, want := range []string{"C3.workspace-run", "C4.current-progress", "C4.current-action", "nextRequiredAction"} {
		if !strings.Contains(dynamic, want) {
			t.Fatalf("dynamic tail missing %q: %s", want, dynamic)
		}
	}
	for _, sectionID := range prompt.StablePrefixSectionIDs {
		group := promptSectionGroup(sectionID)
		if group != "C0" && group != "C1" && group != "C2" {
			t.Fatalf("stable prefix section %q group=%q, want C0/C1/C2", sectionID, group)
		}
	}
}

func TestContractViolationRecoveryRendersInCurrentActionOnly(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID:    "ws-contract",
		RunID:          "run-contract",
		StepID:         "step-contract",
		Kind:           llmcommon.StepKindWorkspaceExecution,
		Model:          "model",
		Schema:         decisionSchemaName,
		DialogLanguage: "English",
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			"repairObjective":             "produce_state_changing_output",
			"contractViolationRecovery": map[string]any{
				"violatedRule":     "state_changing_required_missing",
				"recoverability":   "retry_same_contract",
				"expectedContract": map[string]any{"stateChangingOutputRequired": true},
				"actualOutputSummary": map[string]any{
					"graphNodeKinds": []string{"file/read"},
				},
				"attempt":     1,
				"maxAttempts": 2,
			},
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"acceptedKinds": []string{"patch/apply"},
				},
			},
		},
	}

	context := buildPromptContextFromRequest(req)
	current, ok := promptContextSectionByID(context, "C4.current-action")
	if !ok {
		t.Fatalf("missing C4.current-action; sections=%+v", promptContextSectionIDsForTest(context))
	}
	for _, want := range []string{
		"contractViolationRecovery",
		"state_changing_required_missing",
		"retry_same_contract",
		"expectedContract",
		"actualOutputSummary",
	} {
		if !strings.Contains(current.Content, want) {
			t.Fatalf("current action missing %q: %s", want, current.Content)
		}
	}
	prompt := buildProviderFacingPrompt(req)
	if strings.Contains(prompt.StablePrefix, "contractViolationRecovery") ||
		strings.Contains(prompt.StablePrefix, "state_changing_required_missing") {
		t.Fatalf("stable prefix leaked contract violation recovery: %s", prompt.StablePrefix)
	}
}

func TestCurrentActionIncludesAvailableTargetSnapshots(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID:    "ws-available-snapshot",
		RunID:          "run-001",
		StepID:         "step-001",
		Kind:           llmcommon.StepKindWorkspaceExecution,
		Model:          "model",
		Schema:         decisionSchemaName,
		DialogLanguage: "English",
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			"repairObjective":             "produce_state_changing_output",
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"acceptedKinds":                []string{"patch/apply"},
					"mustIncludeStateChangingNode": true,
				},
				"targetSnapshots": []map[string]any{{
					"targetRef":        "target.txt",
					"sourcePath":       ".workorch-workspaces/ws/target.txt",
					"content":          "current target content\n",
					"contentHash":      "sha256-current-target",
					"contentComplete":  true,
					"safeForFileWrite": true,
				}},
			},
		},
	}

	context := buildPromptContextFromRequest(req)
	current, ok := promptContextSectionByID(context, "C4.current-action")
	if !ok {
		t.Fatalf("missing C4.current-action; sections=%+v", promptContextSectionIDsForTest(context))
	}
	for _, want := range []string{
		"availableTargetSnapshots",
		"primaryNextAction",
		"target.txt",
		"sha256-current-target",
		".workorch-workspaces/ws/target.txt",
		"allowedPatchModes",
		"promptSafeForFileWrite",
	} {
		if !strings.Contains(current.Content, want) {
			t.Fatalf("C4.current-action missing %q: %s", want, current.Content)
		}
	}
	prompt := buildProviderFacingPrompt(req)
	if strings.Contains(prompt.StablePrefix, "availableTargetSnapshots") ||
		strings.Contains(prompt.StablePrefix, "sha256-current-target") {
		t.Fatalf("stable prefix leaked available snapshot summary: %s", prompt.StablePrefix)
	}
}

func TestCurrentActionMarksTruncatedAvailableSnapshotUnsafe(t *testing.T) {
	content := strings.Repeat("x", repairSnapshotPromptContentLimit+128)
	req := llmmodels.ProviderRequest{
		WorkspaceID:    "ws-truncated-snapshot",
		RunID:          "run-001",
		StepID:         "step-001",
		Kind:           llmcommon.StepKindWorkspaceExecution,
		Model:          "model",
		Schema:         decisionSchemaName,
		DialogLanguage: "English",
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			"repairObjective":             "produce_state_changing_output",
			"repairActionPackage": map[string]any{
				"targetSnapshots": []map[string]any{{
					"targetRef":        "large-target.txt",
					"content":          content,
					"contentHash":      "sha256-large-target",
					"contentComplete":  true,
					"safeForFileWrite": true,
				}},
			},
		},
	}

	context := buildPromptContextFromRequest(req)
	current, ok := promptContextSectionByID(context, "C4.current-action")
	if !ok {
		t.Fatalf("missing C4.current-action; sections=%+v", promptContextSectionIDsForTest(context))
	}
	for _, want := range []string{
		"large-target.txt",
		"promptSafeForFileWrite\":false",
		"promptExtractionRequired\":true",
	} {
		if !strings.Contains(current.Content, want) {
			t.Fatalf("C4.current-action missing %q: %s", want, current.Content)
		}
	}
}

func TestProviderCachePlanKeepsStablePrefixForAvailableTargetSnapshots(t *testing.T) {
	base := llmmodels.ProviderRequest{
		WorkspaceID:    "ws-cache-available-snapshot",
		RunID:          "run-001",
		StepID:         "step-001",
		Kind:           llmcommon.StepKindWorkspaceExecution,
		Model:          "model",
		Schema:         decisionSchemaName,
		DialogLanguage: "English",
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			"repairObjective":             "produce_state_changing_output",
			"repairActionPackage": map[string]any{
				"targetSnapshots": []map[string]any{{
					"targetRef":        "target.txt",
					"content":          "old content\n",
					"contentHash":      "sha256-old",
					"contentComplete":  true,
					"safeForFileWrite": true,
				}},
			},
		},
	}
	changed := base
	changed.Input = map[string]any{
		"stateChangingOutputRequired": true,
		"repairObjective":             "produce_state_changing_output",
		"repairActionPackage": map[string]any{
			"targetSnapshots": []map[string]any{{
				"targetRef":        "target.txt",
				"content":          "new content\n",
				"contentHash":      "sha256-new",
				"contentComplete":  true,
				"safeForFileWrite": true,
			}},
		},
	}
	base.PromptContext = buildPromptContextFromRequest(base)
	changed.PromptContext = buildPromptContextFromRequest(changed)
	attachProviderCachePlan(&base)
	attachProviderCachePlan(&changed)

	if base.ContextBudget == nil || base.ContextBudget.CachePlan == nil ||
		changed.ContextBudget == nil || changed.ContextBudget.CachePlan == nil {
		t.Fatalf("cachePlan missing: base=%+v changed=%+v", base.ContextBudget, changed.ContextBudget)
	}
	if base.ContextBudget.CachePlan.StablePrefixHash != changed.ContextBudget.CachePlan.StablePrefixHash {
		t.Fatalf("stablePrefixHash base=%q changed=%q, want unchanged C0", base.ContextBudget.CachePlan.StablePrefixHash, changed.ContextBudget.CachePlan.StablePrefixHash)
	}
	if base.ContextBudget.CachePlan.VolatileTailHash == changed.ContextBudget.CachePlan.VolatileTailHash {
		t.Fatalf("volatileTailHash base=%q changed=%q, want C4 available snapshot change", base.ContextBudget.CachePlan.VolatileTailHash, changed.ContextBudget.CachePlan.VolatileTailHash)
	}
}

func TestProviderPromptDoesNotIncludeProjectionOnlyInputState(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID:    "ws-projection-guard",
		RunID:          "run-001",
		StepID:         "step-001",
		Kind:           llmcommon.StepKindWorkspaceExecution,
		Model:          "model",
		Schema:         decisionSchemaName,
		DialogLanguage: "English",
		RuntimeCapabilities: llmmodels.RuntimeCapabilities{
			BlockKinds: []llmmodels.RuntimeBlockCapability{{Kind: "file/read", Description: "read file"}},
		},
		Input: map[string]any{
			"executionContextPackage": map[string]any{
				"goal": map[string]any{"text": "authoritative run goal"},
				"dod":  map[string]any{"items": []any{"authoritative run dod"}},
			},
			"currentProgressDigest": map[string]any{
				"summary":            "allowed runtime evidence marker",
				"nextRequiredAction": "continue",
			},
			"processEvidenceTimeline": []any{
				map[string]any{"sourceRef": "run-001/step-001", "eventType": "runtime_evidence"},
			},
			"workspaceProjection": map[string]any{"summary": "FORBIDDEN_WORKSPACE_PROJECTION_STATE"},
			"workspaceSnapshot":   map[string]any{"summary": "FORBIDDEN_WORKSPACE_SNAPSHOT_STATE"},
			"runSnapshot":         map[string]any{"summary": "FORBIDDEN_RUN_SNAPSHOT_STATE"},
			"viewModel":           map[string]any{"summary": "FORBIDDEN_VIEW_MODEL_STATE"},
		},
	}
	req.PromptContext = buildPromptContextFromRequest(req)
	prompt := buildProviderFacingPrompt(req)
	joined := prompt.StablePrefix
	for _, message := range prompt.DynamicMessages {
		joined += "\n" + message.Content
	}
	for _, forbidden := range []string{
		"FORBIDDEN_WORKSPACE_PROJECTION_STATE",
		"FORBIDDEN_WORKSPACE_SNAPSHOT_STATE",
		"FORBIDDEN_RUN_SNAPSHOT_STATE",
		"FORBIDDEN_VIEW_MODEL_STATE",
	} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("provider prompt leaked projection-only input %q:\n%s", forbidden, joined)
		}
	}
	for _, want := range []string{"authoritative run goal", "authoritative run dod", "allowed runtime evidence marker"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("provider prompt missing allowed runtime context %q:\n%s", want, joined)
		}
	}
}

func TestBuildPromptContextIncludesSelectedCapabilityUsageGuidance(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID:    "ws-capability",
		RunID:          "run-capability",
		StepID:         "step-capability",
		Kind:           llmcommon.StepKindWorkspaceExecution,
		Model:          "model",
		Schema:         decisionSchemaName,
		DialogLanguage: "English",
		Input: map[string]any{
			"currentRuntimeStage":     "repair",
			"currentRuntimeStageMode": "strict_convergence",
			"executionContextPackage": map[string]any{
				"stage":     "repair",
				"stageMode": "strict_convergence",
				"executionContract": map[string]any{
					"capabilityUsagePolicies": []any{
						map[string]any{
							"id":                "code-semantics-guidance",
							"appliesToCategory": "code_semantics",
							"usageMode":         "prefer_when_triggered",
							"triggerStages":     []any{"planning", "repair"},
							"triggerSignals":    []any{"symbol_resolution_needed"},
							"fallbackPolicy":    "fallback_without_blocking",
						},
					},
				},
				"selectedContext": map[string]any{
					"mcpServers": []any{
						map[string]any{
							"id":           "semantic-server",
							"name":         "Semantic Server",
							"transport":    "stdio",
							"allowedTools": []any{"semantic.lookup"},
						},
					},
					"mcpCapabilities": []any{
						map[string]any{
							"id":         "semantic-server:tool:semantic.lookup",
							"serverId":   "semantic-server",
							"kind":       "tool",
							"name":       "semantic.lookup",
							"category":   "code_semantics",
							"usageHint":  "Use when symbol resolution is needed.",
							"stageHints": []any{"planning", "repair"},
						},
					},
				},
			},
		},
	}

	context := buildPromptContextFromRequest(req)
	section, ok := promptContextSectionByID(context, "C2.capability-usage-guidance")
	if !ok {
		t.Fatalf("missing capability usage guidance; sections=%+v", promptContextSectionIDsForTest(context))
	}
	for _, want := range []string{
		"Capability Usage Guidance",
		"semantic-server",
		"semantic.lookup",
		"code_semantics",
		"code-semantics-guidance",
		"symbol_resolution_needed",
		"fallback_without_blocking",
	} {
		if !strings.Contains(section.Content, want) {
			t.Fatalf("capability guidance missing %q: %s", want, section.Content)
		}
	}
	if strings.Contains(section.Content, "\"stage\"") || strings.Contains(section.Content, "\"stageMode\"") {
		t.Fatalf("capability guidance should stay stage-independent: %s", section.Content)
	}
	if section.Cacheability != "capability_contract" {
		t.Fatalf("capability guidance cacheability=%q, want capability_contract", section.Cacheability)
	}
}

func promptContextSectionByID(context *llmmodels.PromptContext, sectionID string) (llmmodels.PromptSection, bool) {
	if context == nil {
		return llmmodels.PromptSection{}, false
	}
	for _, section := range context.Sections {
		if section.ID == sectionID {
			return section, true
		}
	}
	return llmmodels.PromptSection{}, false
}

func promptContextSectionIDsForTest(context *llmmodels.PromptContext) []string {
	if context == nil {
		return nil
	}
	ret := make([]string, 0, len(context.Sections))
	for _, section := range context.Sections {
		ret = append(ret, section.ID)
	}
	return ret
}

func TestMemorySignalPayloadFiltersNonReferenceMemoryNoise(t *testing.T) {
	payload := memorySignalPayload(map[string]any{
		"executionContextPackage": map[string]any{
			"selectedContext": map[string]any{
				"memoryRetrieval": map[string]any{
					"policy":            "step_input_dynamic_workspace_memory",
					"status":            "selected",
					"selectedMemoryIds": []string{"memory-keep", "memory-stale", "memory-low"},
					"excludedMemoryIds": []string{"memory-excluded"},
					"selectionScores": []map[string]any{
						{"id": "memory-keep", "score": 100, "reason": "current decision"},
					},
				},
				"memories": []map[string]any{
					{"id": "memory-keep", "summary": "usable summary", "content": "usable content", "confidence": 0.8, "importance": 0.7, "provenance": "current_run"},
					{"id": "memory-excluded", "summary": "excluded summary", "confidence": 0.9, "importance": 0.9},
					{"id": "memory-stale", "summary": "stale summary", "confidence": 0.9, "importance": 0.9, "tags": []string{"stale"}},
					{"id": "memory-low", "summary": "low summary", "confidence": 0.1, "importance": 0.9},
				},
			},
		},
	})
	selected := recordFromAny(recordFromAny(payload["selectedContext"])["memoryRetrieval"])
	if selected["status"] != "selected" {
		t.Fatalf("memory retrieval summary = %+v, want selected status retained", selected)
	}
	memories := mapSliceFromAny(recordFromAny(payload["selectedContext"])["memories"])
	if len(memories) != 1 {
		t.Fatalf("memories = %+v, want only high-value selected memory", memories)
	}
	if memories[0]["id"] != "memory-keep" || memories[0]["cacheability"] != "volatile_current_run_memory" {
		t.Fatalf("memory = %+v, want kept current-run memory with cacheability", memories[0])
	}
	encoded := marshalPromptSection("Memory Recall Signal", payload)
	for _, forbidden := range []string{"excluded summary", "stale summary", "low summary"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("memory payload leaked %q: %s", forbidden, encoded)
		}
	}
}

func TestMemorySignalPayloadBoundsSelectedMemoryContent(t *testing.T) {
	memories := make([]map[string]any, 0, 8)
	selectedIDs := make([]string, 0, 8)
	scores := make([]map[string]any, 0, 8)
	for idx := 0; idx < 8; idx++ {
		id := fmt.Sprintf("memory-%02d", idx)
		selectedIDs = append(selectedIDs, id)
		memories = append(memories, map[string]any{
			"id":         id,
			"summary":    strings.Repeat("summary ", 300),
			"content":    strings.Repeat("content ", 600),
			"confidence": 0.9,
			"importance": 0.9,
		})
		scores = append(scores, map[string]any{
			"id":     id,
			"score":  100 - idx,
			"reason": strings.Repeat("reason ", 200),
		})
	}

	payload := memorySignalPayload(map[string]any{
		"executionContextPackage": map[string]any{
			"selectedContext": map[string]any{
				"memoryRetrieval": map[string]any{
					"status":            "selected",
					"selectedMemoryIds": selectedIDs,
					"selectionScores":   scores,
					"reason":            strings.Repeat("retrieval ", 200),
				},
				"memories": memories,
			},
		},
	})
	selected := recordFromAny(payload["selectedContext"])
	gotMemories := mapSliceFromAny(selected["memories"])
	if len(gotMemories) != 2 {
		t.Fatalf("selected memories = %d, want bounded 2", len(gotMemories))
	}
	gotScores := mapSliceFromAny(recordFromAny(selected["memoryRetrieval"])["selectionScores"])
	if len(gotScores) != 4 {
		t.Fatalf("selection scores = %d, want bounded 4", len(gotScores))
	}
	encoded := marshalPromptSection("Memory Recall Signal", payload)
	if len(encoded) > 9000 {
		t.Fatalf("encoded memory signal length = %d, want bounded payload", len(encoded))
	}
	if strings.Contains(encoded, strings.Repeat("content ", 200)) {
		t.Fatalf("memory signal leaked unbounded content")
	}
}

func TestRepairActionPackagePromptSectionBoundsDuplicatedDynamicContext(t *testing.T) {
	coverageItems := make([]map[string]any, 0, 20)
	for idx := 0; idx < 20; idx++ {
		coverageItems = append(coverageItems, map[string]any{
			"id":                   fmt.Sprintf("cwi-%03d", idx),
			"coverageTargetToken":  fmt.Sprintf("coverage-token-%03d", idx),
			"targetText":           strings.Repeat("coverage target ", 120),
			"acceptedEvidenceRefs": []string{fmt.Sprintf("run/step/%03d", idx), fmt.Sprintf("run/step/%03d/validation", idx)},
			"currentGap":           strings.Repeat("gap ", 120),
			"status":               "open",
		})
	}
	manifestItems := make([]map[string]any, 0, 40)
	for idx := 0; idx < 40; idx++ {
		manifestItems = append(manifestItems, map[string]any{
			"path":    fmt.Sprintf("file-%03d.go", idx),
			"summary": strings.Repeat("manifest-noise ", 120),
		})
	}
	input := map[string]any{
		"repairActionPackage": map[string]any{
			"currentFailure":           map[string]any{"reason": strings.Repeat("failure ", 300)},
			"workspaceFileManifest":    map[string]any{"items": manifestItems},
			"coverageWorkItemContext":  map[string]any{"summary": map[string]any{"open": 20}, "currentFocusItems": coverageItems, "unresolvedItems": coverageItems, "ledgerSummary": coverageItems},
			"currentExecutionProgress": map[string]any{"unresolvedCoverageTargets": coverageItems},
			"coverageTargetSet":        coverageItems,
		},
	}

	encoded := repairActionPackagePromptSection(input, defaultPromptSnapshotBudget())
	if len(encoded) > 24000 {
		t.Fatalf("repair action package length = %d, want bounded C3 projection", len(encoded))
	}
	for _, forbidden := range []string{`"workspaceFileManifest":`, `"coverageWorkItemContext":`, strings.Repeat("manifest-noise ", 40)} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("repair action package leaked unbounded field %q", forbidden)
		}
	}
	if !strings.Contains(encoded, "workspaceFileManifestSummary") || !strings.Contains(encoded, "coverageWorkItemContextRef") {
		t.Fatalf("repair action package missing bounded refs: %s", encoded)
	}
	if !strings.Contains(encoded, `"coverageTargetToken":"coverage-token-000"`) {
		t.Fatalf("repair action package missing copyable coverageTargetToken: %s", encoded)
	}
}

func TestValidationSignalPayloadBoundsFailedStepContext(t *testing.T) {
	timeline := make([]map[string]any, 0, 16)
	for idx := 0; idx < 16; idx++ {
		timeline = append(timeline, map[string]any{
			"stepId":    fmt.Sprintf("step-%03d", idx),
			"sourceRef": fmt.Sprintf("step://%03d", idx),
			"output": map[string]any{
				"executionGraph": map[string]any{"nodes": []map[string]any{{"input": strings.Repeat("graph-noise ", 600)}}},
				"contextBudget":  map[string]any{"actualBytes": 900000, "sectionHashes": []map[string]any{{"hash": strings.Repeat("hash", 1000)}}},
			},
		})
	}
	input := map[string]any{
		"failedStepInput": map[string]any{
			"workspaceId":             "ws-1",
			"processEvidenceTimeline": timeline,
			"repairActionPackage": map[string]any{
				"currentFailure":        map[string]any{"reason": strings.Repeat("failure ", 200)},
				"workspaceFileManifest": map[string]any{"items": []map[string]any{{"path": "main.go", "summary": strings.Repeat("manifest-noise ", 200)}}},
			},
		},
		"failedStepOutput": map[string]any{
			"summary":        strings.Repeat("summary ", 300),
			"executionGraph": map[string]any{"nodes": []map[string]any{{"input": strings.Repeat("graph-noise ", 800)}}},
			"contextBudget":  map[string]any{"actualBytes": 1000000, "sectionHashes": []map[string]any{{"hash": strings.Repeat("hash", 1000)}}},
		},
		"failedExecutionGraph": map[string]any{"nodes": []map[string]any{{"input": strings.Repeat("graph-noise ", 900)}}},
	}

	payload := validationSignalPayload(input, defaultPromptSnapshotBudget())
	encoded := marshalPromptSection("Validation Signal", payload)
	if len(encoded) > 28000 {
		t.Fatalf("validation signal length = %d, want bounded payload", len(encoded))
	}
	for _, forbidden := range []string{strings.Repeat("graph-noise ", 80), strings.Repeat("manifest-noise ", 80), "sectionHashes"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("validation signal leaked bulky field %q", forbidden)
		}
	}
	if !strings.Contains(encoded, "processEvidenceTimelineRef") || !strings.Contains(encoded, "executionGraphSummary") {
		t.Fatalf("validation signal missing compact refs: %s", encoded)
	}
}

func TestProcessEvidenceTimelineSignalCompactsBulkyHistoricalOutputs(t *testing.T) {
	timeline := make([]map[string]any, 0, 20)
	for idx := 1; idx <= 20; idx++ {
		timeline = append(timeline, map[string]any{
			"order":         idx,
			"kind":          "llm/workspace_execution",
			"state":         "completed",
			"stepId":        fmt.Sprintf("run-001/step-%03d", idx),
			"sourceRef":     fmt.Sprintf("step://run-001/step-%03d", idx),
			"failureReason": strings.Repeat("diagnostic ", 600),
			"output": map[string]any{
				"contextBudget": map[string]any{
					"actualBytes":          1000 + idx,
					"includedSections":     []string{"C0.system", "C4.process-evidence-timeline", strings.Repeat("large-section", 200)},
					"protectedFields":      []string{strings.Repeat("protected", 300)},
					"estimatedInputTokens": 2000 + idx,
					"cachePlan": map[string]any{
						"stablePrefixHash":        "sha256:stable",
						"workspaceDefinitionHash": "sha256:def",
						"capabilityContractHash":  "sha256:cap",
						"volatileTailHash":        "sha256:tail",
						"sectionHashes":           []map[string]any{{"sectionId": "C4.large", "hash": strings.Repeat("x", 5000)}},
					},
				},
				"executionGraph": map[string]any{
					"nodes": []map[string]any{{"id": "node", "input": map[string]any{"content": strings.Repeat("graph", 2000)}}},
				},
				"llmUsage": map[string]any{
					"inputTokens": 100,
					"cacheUsage":  map[string]any{"cacheReadTokens": 10, "volatileSections": []string{strings.Repeat("volatile", 200)}},
				},
			},
		})
	}

	payload := processEvidenceTimelineSignalPayload(map[string]any{"processEvidenceTimeline": timeline})
	if payload["totalEvents"] != 20 || payload["omittedMiddleCount"] != 8 {
		t.Fatalf("payload = %+v, want total 20 and omitted middle 8", payload)
	}
	events := mapSliceFromAny(payload["events"])
	if len(events) != 12 {
		t.Fatalf("events = %d, want compact 12", len(events))
	}
	if events[0]["stepId"] != "run-001/step-001" || events[len(events)-1]["stepId"] != "run-001/step-020" {
		t.Fatalf("events window = %+v, want first and latest retained", events)
	}
	encoded := marshalPromptSection("Process Evidence Timeline", payload)
	if len(encoded) > 60000 {
		t.Fatalf("encoded timeline length = %d, want compact payload", len(encoded))
	}
	for _, forbidden := range []string{"includedSections", "protectedFields", "sectionHashes", strings.Repeat("diagnostic ", 300)} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("encoded timeline leaked bulky field %q", forbidden)
		}
	}
	if !strings.Contains(encoded, "stablePrefixHash") || !strings.Contains(encoded, "cacheReadTokens") {
		t.Fatalf("encoded timeline missing cache diagnostics: %s", encoded)
	}
}

func TestProcessEvidenceTimelineSignalPreservesEventAnchorsAndTrimAudit(t *testing.T) {
	timeline := make([]map[string]any, 0, 16)
	for idx := 1; idx <= 16; idx++ {
		item := map[string]any{
			"order":     idx,
			"eventType": "tool_result",
			"stepId":    fmt.Sprintf("run-anchor/step-%03d", idx),
			"output": map[string]any{
				"summary":        "summary",
				"largePayload":   strings.Repeat("payload", 1000),
				"executionGraph": map[string]any{"nodes": []any{"node"}},
			},
		}
		switch idx {
		case 1:
			item["sourceRef"] = "step://run-anchor/step-001"
		case 2:
			item["payloadRef"] = "workorch://payload/002"
		case 3:
			item["contentHash"] = "sha256:003"
		case 4:
			item["artifactRef"] = "artifact://004"
		case 16:
			delete(item, "stepId")
		default:
			item["sourceRef"] = fmt.Sprintf("step://run-anchor/step-%03d", idx)
		}
		timeline = append(timeline, item)
	}

	payload := processEvidenceTimelineSignalPayload(map[string]any{"processEvidenceTimeline": timeline})
	if payload["schemaVersion"] != "workorch.c4.processEvidenceTimeline.v1" {
		t.Fatalf("schemaVersion = %#v", payload["schemaVersion"])
	}
	validation := recordFromAny(payload["sourceRefValidation"])
	if validation["policy"] != "sourceRef_or_payloadRef_or_contentHash_required_per_event" ||
		validation["missingAnchorCount"] != 1 {
		t.Fatalf("sourceRefValidation = %+v, want one missing anchor", validation)
	}
	omittedRefs := stringSliceFromAny(payload["omittedRefs"])
	if len(omittedRefs) == 0 || !strings.Contains(strings.Join(omittedRefs, "\n"), "workorch://payload/002") {
		t.Fatalf("omittedRefs = %+v, want middle event refs retained", omittedRefs)
	}
	events := mapSliceFromAny(payload["events"])
	if len(events) != 12 {
		t.Fatalf("events = %d, want 12", len(events))
	}
	first := events[0]
	if first["sourceRef"] != "step://run-anchor/step-001" || first["eventTapeSequence"] != 1 {
		t.Fatalf("first event = %+v, want sourceRef and eventTapeSequence", first)
	}
	last := events[len(events)-1]
	if last["anchorRefStatus"] != "missing" || last["eventTapeSequence"] != 16 {
		t.Fatalf("last event = %+v, want missing anchor status and original sequence", last)
	}
	output := recordFromAny(last["output"])
	omittedOutputKeys := stringSliceFromAny(output["omittedOutputKeys"])
	if !stringSliceContains(omittedOutputKeys, "executionGraph") || !stringSliceContains(omittedOutputKeys, "largePayload") {
		t.Fatalf("omittedOutputKeys = %+v, want bulky output keys audited", omittedOutputKeys)
	}
}

func TestProviderCachePlanMarksTypedEventTape(t *testing.T) {
	context := buildPromptContextFromRequest(llmmodels.ProviderRequest{
		WorkspaceID: "ws-tape",
		RunID:       "run-001",
		StepID:      "step-001",
		Kind:        llmcommon.StepKindWorkspaceExecution,
		Model:       "model",
		Schema:      decisionSchemaName,
		Input: map[string]any{
			"executionContext": map[string]any{
				"goal": "goal",
				"dod":  "dod",
			},
			"currentProgressDigest": map[string]any{"nextRequiredAction": "continue"},
			"processEvidenceTimeline": []any{
				map[string]any{"order": 1, "sourceRef": "step://run-001/step-001", "summary": "done"},
			},
		},
	})
	if context == nil {
		t.Fatalf("promptContext = nil")
	}
	plan := buildProviderCachePlan(llmmodels.ProviderRequest{PromptContext: context})
	if !plan.ObservedRequestShape.TypedEventTape {
		t.Fatalf("typedEventTape = false, want true")
	}
}

func TestNormalizePromptTextPreservesCodeOperatorsInStructuredEvidence(t *testing.T) {
	input := `Execution Context and Evidence Signals
{"input":{"failedStepOutput":{"diff":"func TestAlpha(t *testing.T) {\n\t_ = handler(func(w http.ResponseWriter, r *http.Request) {})\n\tif a < b && c > d { return }\n}"}}}`
	normalized, _ := normalizePromptText(input)
	required := []string{
		"*testing.T",
		"*http.Request",
		"&&",
		"< b",
		"> d",
	}
	for _, item := range required {
		if !strings.Contains(normalized, item) {
			t.Fatalf("normalized prompt missing %q\nnormalized=%s", item, normalized)
		}
	}
}

func TestBuildProviderPromptMessagesIncludesDecisionApprovalResolutions(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID:    "ws-3",
		RunID:          "run-3",
		StepID:         "step-3",
		Kind:           llmcommon.StepKindWorkspaceExecution,
		Model:          "model",
		Schema:         decisionSchemaName,
		DialogLanguage: "Chinese (Simplified)",
		Input: map[string]any{
			"decisionResolution": map[string]any{
				"resolutionType": "provide_missing_info",
				"resolutionPayload": map[string]any{
					"city": "Nanjing",
				},
			},
			"approvalResolution": map[string]any{
				"action": "approve",
			},
			"executionContext": map[string]any{
				"governanceHistory": []any{
					map[string]any{"id": "ticket-1", "state": "resolved"},
				},
				"goal": map[string]any{"text": "query weather"},
				"dod":  map[string]any{"items": []any{"return city weather"}},
			},
		},
	}
	build := buildProviderPromptMessages(req)
	joined := ""
	for _, item := range build.Messages {
		joined += "\n" + item.Content
	}
	if !strings.Contains(joined, "\"decisionResolution\"") || !strings.Contains(joined, "\"approvalResolution\"") {
		t.Fatalf("prompt missing decision/approval resolution payload, joined=%s", joined)
	}
	if !strings.Contains(joined, "\"governanceHistory\"") {
		t.Fatalf("prompt missing governance history, joined=%s", joined)
	}
}

func TestBuildProviderPromptMessagesIncludesRuntimeStageSignals(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID:    "ws-stage",
		RunID:          "run-stage",
		StepID:         "step-stage",
		Kind:           llmcommon.StepKindWorkspaceExecution,
		Model:          "model",
		Schema:         decisionSchemaName,
		DialogLanguage: "English",
		Input: map[string]any{
			"currentRuntimeStage":     "verification",
			"currentRuntimeStageMode": "strict_convergence",
		},
	}
	build := buildProviderPromptMessages(req)
	joined := ""
	for _, item := range build.Messages {
		joined += "\n" + item.Content
	}
	for _, want := range []string{"runtimeStage", "verification", "runtimeStageMode", "strict_convergence"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("prompt context missing %q, joined=%s", want, joined)
		}
	}
}

func TestBuildProviderPromptMessagesKeepsMultipleGoalAndDoDEntries(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID:    "ws-4",
		RunID:          "run-4",
		StepID:         "step-4",
		Kind:           llmcommon.StepKindWorkspaceExecution,
		Model:          "model",
		Schema:         decisionSchemaName,
		DialogLanguage: "English",
		Input: map[string]any{
			"executionContext": map[string]any{
				"goals": []any{
					map[string]any{"text": "goal one"},
					map[string]any{"text": "goal two"},
				},
				"dods": []any{
					map[string]any{"items": []any{"dod one"}},
					map[string]any{"items": []any{"dod two"}},
				},
			},
		},
	}
	build := buildProviderPromptMessages(req)
	joined := ""
	for _, item := range build.Messages {
		joined += "\n" + item.Content
	}
	if !strings.Contains(joined, "goal one") || !strings.Contains(joined, "goal two") {
		t.Fatalf("prompt missing multiple goal entries, joined=%s", joined)
	}
	if !strings.Contains(joined, "dod one") || !strings.Contains(joined, "dod two") {
		t.Fatalf("prompt missing multiple dod entries, joined=%s", joined)
	}
}

func TestBuildProviderPromptMessagesUsesPackagedGoalAndDoDWhenExecutionContextIsSlim(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID:    "ws-packaged-goal-dod",
		RunID:          "run-packaged-goal-dod",
		StepID:         "step-packaged-goal-dod",
		Kind:           llmcommon.StepKindWorkspaceExecution,
		Model:          "model",
		Schema:         decisionSchemaName,
		DialogLanguage: "Chinese (Simplified)",
		Input: map[string]any{
			"executionContext": map[string]any{
				"dialogLanguage": "zh-CN",
			},
			"executionContextPackage": map[string]any{
				"goal": map[string]any{
					"text": "packaged goal text",
				},
				"dod": map[string]any{
					"items": []any{"packaged dod item"},
				},
				"goalDefinitionPolicy": "workspace_goal",
				"doDDefinitionPolicy":  "workspace_dod",
				"definitionChangeImpact": map[string]any{
					"changed": true,
					"policy":  "workspace_goal_dod_cumulative_latest_refines_previous",
				},
			},
		},
	}

	build := buildProviderPromptMessages(req)
	joined := ""
	for _, item := range build.Messages {
		joined += "\n" + item.Content
	}
	for _, want := range []string{
		"packaged goal text",
		"packaged dod item",
		"packagedGoalDefinitionPolicy",
		"packagedDoDDefinitionPolicy",
		"Effective Goal and DoD View",
		"definitionChangeImpact.changed is true",
		"additive constraints",
		"workspace_goal_dod_cumulative_latest_refines_previous",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("prompt missing %q, joined=%s", want, joined)
		}
	}
}

func TestBuildProviderPromptMessagesUsesStateChangingRepairAction(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID:    "ws-direct-recovery",
		RunID:          "run-direct-recovery",
		StepID:         "step-direct-recovery",
		Kind:           llmcommon.StepKindWorkspaceExecution,
		Model:          "model",
		Schema:         decisionSchemaName,
		DialogLanguage: "English",
		Input: map[string]any{
			"stateChangingOutputRequired":       true,
			"stateChangingRecoveryMode":         "direct_state_change",
			"providerTimeoutStateChangingRetry": true,
			"repairObjective":                   "produce_state_changing_output",
			"repairActionPackage": map[string]any{
				"failureClass":     "runtime_failed",
				"failureSignature": "class=runtime_failed|kind=llm/workspace_execution",
				"requiredOutput": map[string]any{
					"acceptedKinds":                []string{"patch/apply"},
					"forbiddenKinds":               []string{"read_only_graph", "shell_only_graph"},
					"mustIncludeStateChangingNode": true,
				},
				"outputDecisionContract": map[string]any{
					"requiredMode":     "state_changing_patch_apply",
					"allowedOutputs":   []string{"patch/apply_text_replace_substantive", "failed_dodAssessment", "real_governance_blocker"},
					"forbiddenOutputs": []string{"read_only_or_inspection_only_graph", "no_op_text_replace", "identical_oldText_newText"},
				},
			},
			"executionContextPackage": map[string]any{
				"goal": map[string]any{"text": "create the requested artifact"},
				"dod":  map[string]any{"items": []any{"artifact exists"}},
			},
		},
	}

	build := buildProviderPromptMessages(req)
	joined := ""
	for _, item := range build.Messages {
		joined += "\n" + item.Content
	}
	for _, want := range []string{
		"Stage Contract",
		"Current Repair Action Package",
		"produce_state_changing_repair_or_failed_assessment",
		"patch/apply",
		"read_only_graph",
		"Do not return run snapshot",
		"context rehydration",
		"update those targets directly",
		"do not return file/read for that target",
		"prioritize direct state-changing edits to those targets",
		"do not satisfy this state-changing contract with file/read-only nodes",
		"hunk/context/base mismatch",
		"Do not use shell commands to modify workspace files",
		"Do not use evidence acquisition or supplemental validation shell/tool nodes to create, initialize, or mutate workspace deliverable files",
		"runtime scratch or temporary paths outside the deliverable tree",
		"complete fresh JSON object",
		"outputDecisionContract",
		"state_changing_patch_apply",
		"patch/apply_text_replace_substantive",
		"no_op_text_replace",
		"identical_oldText_newText",
		"oldText and newText must be different substantive literal strings",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("prompt missing %q, joined=%s", want, joined)
		}
	}
	if strings.Contains(joined, "\"type\":\"plan_next_step\"") {
		t.Fatalf("state-changing repair prompt still requests plan_next_step, joined=%s", joined)
	}
}

func TestBuildProviderPromptMessagesPrioritizesStateChangingOverAssessmentOnly(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID:    "ws-provider-retry-state-change",
		RunID:          "run-provider-retry-state-change",
		StepID:         "step-provider-retry-state-change",
		Kind:           llmcommon.StepKindWorkspaceExecution,
		Model:          "model",
		Schema:         decisionSchemaName,
		DialogLanguage: "English",
		Input: map[string]any{
			"providerRetryAssessmentOnly": true,
			"providerRetryFollowUp":       true,
			"completionFollowUp":          true,
			"stateChangingOutputRequired": true,
			"repairObjective":             "produce_state_changing_output",
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"acceptedKinds":                []string{"patch/apply", "git/write"},
					"forbiddenKinds":               []string{"read_only_graph", "shell_only_graph"},
					"mustIncludeStateChangingNode": true,
				},
			},
			"executionContextPackage": map[string]any{
				"goal": map[string]any{"text": "produce the requested artifact"},
				"dod":  map[string]any{"items": []any{"artifact has accepted evidence"}},
			},
		},
	}

	context := buildPromptContextFromRequest(req)
	currentAction, ok := promptContextSectionByID(context, "C4.current-action")
	if !ok {
		t.Fatalf("missing C4.current-action; sections=%+v", promptContextSectionIDsForTest(context))
	}
	for _, want := range []string{
		"produce_state_changing_repair_or_failed_assessment",
		"patch/apply",
		"git/write",
		"mustIncludeStateChangingNode",
	} {
		if !strings.Contains(currentAction.Content, want) {
			t.Fatalf("current action missing %q: %s", want, currentAction.Content)
		}
	}
	if strings.Contains(currentAction.Content, `"type":"assess_only"`) ||
		strings.Contains(currentAction.Content, `"type": "assess_only"`) {
		t.Fatalf("state-changing provider retry rendered assess_only current action: %s", currentAction.Content)
	}
}

func TestBuildProviderPromptMessagesMarksTruncatedRepairSnapshotUnsafe(t *testing.T) {
	longContent := "package main\n" + strings.Repeat("x", repairSnapshotPromptContentLimit+64)
	req := llmmodels.ProviderRequest{
		WorkspaceID:    "ws-truncated-snapshot",
		RunID:          "run-truncated-snapshot",
		StepID:         "step-truncated-snapshot",
		Kind:           llmcommon.StepKindWorkspaceExecution,
		Model:          "model",
		Schema:         decisionSchemaName,
		DialogLanguage: "English",
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			"repairObjective":             "produce_state_changing_output",
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"acceptedKinds":                             []string{"patch/apply"},
					"mustUseCurrentTargetSnapshot":              true,
					"mustPatchDirectlyFromProvidedSnapshot":     true,
					"directCompleteFileWriteFromPromptSnapshot": true,
				},
				"targetSnapshots": []map[string]any{{
					"targetRef":        "target.txt",
					"content":          longContent,
					"contentHash":      "sha256-target",
					"contentComplete":  true,
					"safeForFileWrite": true,
					"redacted":         false,
					"truncated":        false,
					"isBinary":         false,
				}},
			},
			"executionContextPackage": map[string]any{
				"goal": map[string]any{"text": "produce requested change"},
				"dod":  map[string]any{"items": []any{"change is present"}},
			},
		},
	}

	context := buildPromptContextFromRequest(req)
	repair, ok := promptContextSectionByID(context, "C3.repair-action-package")
	if !ok {
		t.Fatalf("missing repair action package section; sections=%+v", promptContextSectionIDsForTest(context))
	}
	for _, want := range []string{
		`"contentOmitted":true`,
		`"snapshotRef":"snapshot://target/target.txt#sha256-target"`,
		`"promptContentComplete":false`,
		`"promptSafeForFileWrite":false`,
		`"promptExtractionRequired":true`,
		`"promptSnapshotContentIncomplete":true`,
		`"mustNotUsePromptSnapshotForCompleteFileWrite":true`,
		`"directCompleteFileWriteFromPromptSnapshotAllowed":false`,
		`"directCompleteFileWriteFromPromptSnapshot":false`,
		`"mustUseCurrentTargetSnapshot":false`,
		`"mustPatchDirectlyFromProvidedSnapshot":false`,
	} {
		if !strings.Contains(repair.Content, want) {
			t.Fatalf("repair prompt missing %q: %s", want, repair.Content)
		}
	}
	if strings.Contains(repair.Content, `"mustPatchDirectlyFromProvidedSnapshot":true`) ||
		strings.Contains(repair.Content, `"directCompleteFileWriteFromPromptSnapshot":true`) ||
		strings.Contains(repair.Content, `"content":"`) ||
		strings.Contains(repair.Content, `"contentView":"complete"`) {
		t.Fatalf("repair prompt retained unsafe complete snapshot contract: %s", repair.Content)
	}

	stage, ok := promptContextSectionByID(context, "C3.stage-contract")
	if !ok {
		t.Fatalf("missing stage contract section; sections=%+v", promptContextSectionIDsForTest(context))
	}
	if strings.Contains(stage.Content, `"mustPatchDirectlyFromProvidedSnapshot":true`) ||
		!strings.Contains(stage.Content, `"mustPatchDirectlyFromProvidedSnapshot":false`) {
		t.Fatalf("stage contract did not use prompt-safe requiredOutput: %s", stage.Content)
	}

	current, ok := promptContextSectionByID(context, "C4.current-action")
	if !ok {
		t.Fatalf("missing current action section; sections=%+v", promptContextSectionIDsForTest(context))
	}
	if strings.Contains(current.Content, `"mustPatchDirectlyFromProvidedSnapshot":true`) ||
		!strings.Contains(current.Content, `"mustPatchDirectlyFromProvidedSnapshot":false`) {
		t.Fatalf("current action did not use prompt-safe requiredOutput: %s", current.Content)
	}
}

func TestBuildProviderPromptMessagesUsesProviderProfileSnapshotBudget(t *testing.T) {
	content := "package main\n" + strings.Repeat("x", repairSnapshotPromptContentLimit+64)
	req := llmmodels.ProviderRequest{
		WorkspaceID:    "ws-profile-snapshot-budget",
		RunID:          "run-profile-snapshot-budget",
		StepID:         "step-profile-snapshot-budget",
		Kind:           llmcommon.StepKindWorkspaceExecution,
		Model:          "model",
		Schema:         decisionSchemaName,
		DialogLanguage: "English",
		Config: llmmodels.RedactedProviderConfig{
			ContextProfile: llmmodels.ProviderContextProfile{
				MaxInputTokens:         200000,
				ReservedOutputTokens:   16000,
				SafetyRatio:            0.8,
				EstimatedTokensPerByte: 0.25,
			},
		},
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			"repairObjective":             "produce_state_changing_output",
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"acceptedKinds":                         []string{"patch/apply"},
					"mustUseCurrentTargetSnapshot":          true,
					"mustPatchDirectlyFromProvidedSnapshot": true,
				},
				"targetSnapshots": []map[string]any{{
					"targetRef":        "target.txt",
					"content":          content,
					"contentHash":      "sha256-target",
					"contentComplete":  true,
					"safeForFileWrite": true,
				}},
			},
		},
	}

	context := buildPromptContextFromRequest(req)
	repair, ok := promptContextSectionByID(context, "C3.repair-action-package")
	if !ok {
		t.Fatalf("missing repair action package section; sections=%+v", promptContextSectionIDsForTest(context))
	}
	if strings.Contains(repair.Content, "contentTruncatedForPrompt") ||
		strings.Contains(repair.Content, `"content":"`) ||
		strings.Contains(repair.Content, `"contentView":"complete"`) {
		t.Fatalf("repair prompt unexpectedly included provider-budgeted snapshot body: %s", repair.Content)
	}
	for _, want := range []string{
		`"contentOmitted":true`,
		`"snapshotRef":"snapshot://target/target.txt#sha256-target"`,
		`"promptSnapshotContentIncomplete":true`,
		`"mustUseCurrentTargetSnapshot":false`,
		`"mustPatchDirectlyFromProvidedSnapshot":false`,
	} {
		if !strings.Contains(repair.Content, want) {
			t.Fatalf("repair prompt missing %q: %s", want, repair.Content)
		}
	}
}

func TestPromptRequiredOutputTreatsUnavailableProviderSnapshotAsIncomplete(t *testing.T) {
	pkg := map[string]any{
		"requiredOutput": map[string]any{
			"mustUseCurrentTargetSnapshot":          true,
			"mustPatchDirectlyFromProvidedSnapshot": true,
		},
		"targetSnapshots": []map[string]any{{
			"targetRef":        "target.txt",
			"content":          "[REDACTED]",
			"contentHash":      "sha256-target",
			"contentComplete":  true,
			"safeForFileWrite": false,
			"redacted":         true,
		}},
	}

	required := recordFromAny(promptRequiredOutputForRepairPackage(pkg, repairSnapshotPromptContentLimit))
	if !boolValue(required["promptSnapshotContentIncomplete"]) ||
		boolValue(required["mustUseCurrentTargetSnapshot"]) ||
		boolValue(required["mustPatchDirectlyFromProvidedSnapshot"]) {
		t.Fatalf("requiredOutput = %+v, want unavailable provider snapshot treated as incomplete", required)
	}
}

func TestBuildProviderPromptMessagesCarriesIncrementalArtifactStageContract(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID:    "ws-artifact-contract",
		RunID:          "run-artifact-contract",
		StepID:         "step-artifact-contract",
		Kind:           llmcommon.StepKindWorkspaceExecution,
		Model:          "model",
		Schema:         decisionSchemaName,
		DialogLanguage: "English",
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			"repairObjective":             "produce_state_changing_output",
			"outputMode":                  "incrementalArtifact",
			"requestedOutputMode":         "incrementalArtifact",
			"outputModeSelector": map[string]any{
				"priority": "P5",
				"reason":   "requested_incremental_artifact_with_budget_signal",
				"mode":     "incrementalArtifact",
			},
			"llmOutputPolicy": map[string]any{
				"maxPatchContentBytes": 128,
			},
			"artifactContinuationId": "artifact-continuation-001",
			"artifactContinuationContext": map[string]any{
				"state":            "chunking",
				"artifactId":       "artifact-001",
				"targetRef":        "deliverable.txt",
				"expectedSequence": 2,
			},
			"artifactOutputContract": map[string]any{
				"mode":                  "incrementalArtifact",
				"phase":                 "artifact_chunk",
				"maxArtifactChunkBytes": 256,
			},
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"outputMode":                         "incrementalArtifact",
					"mustUseArtifactPlanThenChunks":      true,
					"mustNotReturnExecutionGraph":        true,
					"mustIncludeStateChangingNode":       false,
					"expectedStateChangingArtifactCount": 1,
				},
			},
			"executionContextPackage": map[string]any{
				"goal": map[string]any{"text": "produce requested output"},
				"dod":  map[string]any{"items": []any{"output exists"}},
			},
		},
	}

	context := buildPromptContextFromRequest(req)
	stage, ok := promptContextSectionByID(context, "C3.stage-contract")
	if !ok {
		t.Fatalf("missing C3.stage-contract; sections=%+v", promptContextSectionIDsForTest(context))
	}
	for _, want := range []string{
		"Stage Contract",
		"incrementalArtifact",
		"artifact_chunk",
		"artifactContinuationContext",
		"artifactOutputContract",
		"artifactOutputPhase",
		"maxArtifactChunkBytes",
	} {
		if !strings.Contains(stage.Content, want) {
			t.Fatalf("stage contract missing %q: %s", want, stage.Content)
		}
	}
}

func TestBuildProviderPromptMessagesUsesConcreteValidationRepairAction(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID:    "ws-validation-repair",
		RunID:          "run-validation-repair",
		StepID:         "step-validation-repair",
		Kind:           llmcommon.StepKindWorkspaceExecution,
		Model:          "model",
		Schema:         decisionSchemaName,
		DialogLanguage: "English",
		Input: map[string]any{
			"repairObjective":           "produce_concrete_validation_evidence",
			"boundedValidationRequired": true,
			"evidenceRepairRequired":    true,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustIncludeStateChangingNode":             false,
					"mustProduceConcreteValidationEvidence":    true,
					"mustDeclareValidationEvidenceMode":        true,
					"mustDeclareValidationEvidenceObservation": true,
					"acceptedKinds":                            []string{"shell/command", "mcp/tool_call"},
					"forbiddenKinds":                           []string{"patch/apply", "read_only_graph"},
				},
			},
			"executionContextPackage": map[string]any{
				"goal": map[string]any{"text": "satisfy the requested outcome"},
				"dod":  map[string]any{"items": []any{"validation evidence exists"}},
			},
		},
	}

	build := buildProviderPromptMessages(req)
	joined := ""
	for _, item := range build.Messages {
		joined += "\n" + item.Content
	}
	for _, want := range []string{
		"Current Repair Action Package",
		"produce_concrete_validation_evidence_or_failed_assessment",
		"mustProduceConcreteValidationEvidence",
		"shell/command",
		"mcp/tool_call",
		"Do not ask the user to choose planning, execution, repair, or validation stage",
		"Evidence acquisition shell/command or mcp/tool_call nodes must not create, modify, delete, or initialize workspace deliverable files",
		"runtime validation scratch directory or equivalent temporary path",
		"runtime observations",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("prompt missing %q, joined=%s", want, joined)
		}
	}
	if strings.Contains(joined, "\"type\":\"plan_next_step\"") {
		t.Fatalf("concrete validation repair prompt still requests plan_next_step, joined=%s", joined)
	}
}

func TestBuildProviderPromptMessagesUsesPromptContextWithoutDuplicatingSystem(t *testing.T) {
	req := llmmodels.ProviderRequest{
		PromptContext: &llmmodels.PromptContext{
			Turns: []llmmodels.PromptTurn{
				{Role: "system", Content: "system-prompt"},
				{Role: "user", Content: "user-1"},
				{Role: "assistant", Content: "assistant-1"},
			},
		},
	}
	build := buildProviderPromptMessages(req)
	if len(build.Messages) != 3 {
		t.Fatalf("messages=%d, want 3", len(build.Messages))
	}
	systemCount := 0
	for _, item := range build.Messages {
		if normalizedPromptRole(item.Role) == "system" {
			systemCount++
		}
	}
	if systemCount != 1 {
		t.Fatalf("system count=%d, want 1; messages=%+v", systemCount, build.Messages)
	}
	if build.Messages[0].Content != "system-prompt" {
		t.Fatalf("first system content=%q, want existing prompt context system content", build.Messages[0].Content)
	}
}

func TestBuildPromptContextCompactsSoulMarkdownInPolicySections(t *testing.T) {
	longPolicy := "# Execution\n\n" + strings.Repeat("very detailed execution policy that should not be duplicated in full. ", 200)
	req := llmmodels.ProviderRequest{
		WorkspaceID:    "ws-soul-compact",
		RunID:          "run-soul-compact",
		StepID:         "step-soul-compact",
		Kind:           llmcommon.StepKindWorkspaceExecution,
		Model:          "model",
		Schema:         decisionSchemaName,
		DialogLanguage: "English",
		Input: map[string]any{
			"currentRuntimeStage": "execution",
			"currentSoul": map[string]any{
				"id":               "soul-execution",
				"name":             "Execution",
				"description":      "Execute the task",
				"allowedToolKinds": []any{"file/read", "shell/command"},
				"soulMd":           longPolicy,
			},
			"currentSoulBinding": map[string]any{
				"source": "workspace_stage_binding",
				"soulId": "soul-execution",
				"soul": map[string]any{
					"id":          "soul-execution",
					"description": "Execute the task",
					"soulMd":      longPolicy,
				},
			},
			"executionContext": map[string]any{
				"soulRouting": map[string]any{
					"bindings": map[string]any{
						"execution": map[string]any{
							"source": "workspace_stage_binding",
							"soulId": "soul-execution",
							"soul": map[string]any{
								"id":          "soul-execution",
								"description": "Execute the task",
								"soulMd":      longPolicy,
							},
						},
						"decision": map[string]any{
							"source": "workspace_stage_binding",
							"soulId": "soul-decision",
							"soul": map[string]any{
								"id":          "soul-decision",
								"description": "Ask only for real blockers",
								"soulMd":      strings.Repeat("decision policy should not appear in full. ", 100),
							},
						},
					},
				},
			},
		},
	}
	context := buildPromptContextFromRequest(req)
	stage, ok := promptContextSectionByID(context, "C3.stage-contract")
	if !ok {
		t.Fatalf("missing C3.stage-contract; sections=%+v", promptContextSectionIDsForTest(context))
	}
	if strings.Contains(stage.Content, `"soulMd"`) || strings.Contains(stage.Content, `"soulMD"`) {
		t.Fatalf("stage contract leaked raw soulMd: %s", stage.Content)
	}
	if !strings.Contains(stage.Content, "policyDigest") || !strings.Contains(stage.Content, "compacted_current_stage_policy") {
		t.Fatalf("stage contract missing bounded policy digest: %s", stage.Content)
	}
	if strings.Count(stage.Content, `"policyDigest"`) != 1 {
		t.Fatalf("stage contract duplicated policy digest: %s", stage.Content)
	}
	if len(stage.Content) > 7000 {
		t.Fatalf("stage contract too large after soul compaction: %d bytes", len(stage.Content))
	}

	policy, ok := promptContextSectionByID(context, "C2.governance-soul-policy")
	if !ok {
		t.Fatalf("missing C2.governance-soul-policy; sections=%+v", promptContextSectionIDsForTest(context))
	}
	if strings.Contains(policy.Content, "soulMd") || strings.Contains(policy.Content, "policyDigest") {
		t.Fatalf("governance policy should contain binding summary only: %s", policy.Content)
	}
	if len(policy.Content) > 2500 {
		t.Fatalf("governance policy too large after soul compaction: %d bytes", len(policy.Content))
	}
}

func TestBuildPromptContextCompactsContextBudgetAuditForLLM(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID:    "ws-context-budget",
		RunID:          "run-context-budget",
		StepID:         "step-context-budget",
		Kind:           llmcommon.StepKindWorkspaceExecution,
		Model:          "model",
		Schema:         decisionSchemaName,
		DialogLanguage: "English",
		Input: map[string]any{
			"executionContextPackage": map[string]any{
				"goal": map[string]any{"text": "goal"},
				"dod":  map[string]any{"items": []any{"dod"}},
			},
		},
		ContextBudget: &llmmodels.ProviderContextBudget{
			MaxBytes:              100000,
			ActualBytes:           90000,
			MaxTokens:             25000,
			EstimatedInputTokens:  20000,
			EstimatedOutputTokens: 4096,
			ShrinkLevel:           2,
			Truncated:             true,
			OverBudget:            false,
			P0Overflow:            false,
			Rules:                 []string{"protect goal and dod", "compact volatile history"},
			IncludedSections:      []string{"C0.system", "C1.goal-contract-original", "C5.context-budget", "secret-section-marker"},
			NormalizedSections:    []string{"C2.capability-contract", "C4.current-progress"},
			CompressedSections:    []string{"C4.process-evidence-timeline"},
			RedactedPaths: []string{
				"input.executionContext.secrets.apiKey",
				"input.repairActionPackage.targetSnapshots[0].content",
				"input.large.private.audit.path",
			},
			ProtectedFields:    []string{"goal", "dod"},
			IntegrityWarnings:  []string{"truncated volatile timeline"},
			ContextGateAction:  "allow",
			ContextGateReason:  "within budget after compaction",
			StageBudgetRatio:   0.75,
			StageBudgetSource:  "provider_context_profile",
			TracePolicy:        "refs_only",
			RedactionApplied:   true,
			PromptCaptured:     true,
			ProfileMode:        "auto",
			CapabilitySource:   "explicit_config",
			ContextUtilization: 0.65,
			MaxRequestBytes:    1024 * 1024,
			Estimator:          "heuristic",
			ActualTokens:       21000,
			SourceRefs:         []string{"large-source-ref-should-not-be-needed-by-llm"},
			GoalDoDCompressed:  true,
			CachePlan: &llmmodels.ProviderCachePlan{
				CacheKey:                "cache-key",
				StablePrefixHash:        "sha256:stable",
				WorkspaceDefinitionHash: "sha256:workspace",
				CapabilityContractHash:  "sha256:capability",
				StageBundleHash:         "sha256:stage",
				VolatileTailHash:        "sha256:tail",
				LargeRefsHash:           "sha256:large",
				AdapterHint: llmmodels.ProviderAdapterCacheHint{
					CacheablePrefix:  []string{"C0.system", "C1.goal-contract-original"},
					VolatileSections: []string{"C4.current-progress", "C5.context-budget"},
				},
				CacheBreakers: []llmmodels.ProviderCacheBreaker{{SectionID: "C3.stage-contract", Reason: "stage changed"}},
			},
		},
	}

	context := buildPromptContextFromRequest(req)
	section, ok := promptContextSectionByID(context, "C5.context-budget")
	if !ok {
		t.Fatalf("missing C5.context-budget; sections=%+v", promptContextSectionIDsForTest(context))
	}
	for _, want := range []string{
		"Context Budget Audit",
		`"redactedPathCount":3`,
		`"includedSectionCount":4`,
		`"normalizedSectionCount":2`,
		`"stablePrefixHash":"sha256:stable"`,
		`"cacheablePrefixCount":2`,
		`"volatileSectionCount":2`,
		`"cacheBreakerCount":1`,
		`"contextGateAction":"allow"`,
	} {
		if !strings.Contains(section.Content, want) {
			t.Fatalf("context budget prompt missing %q: %s", want, section.Content)
		}
	}
	for _, forbidden := range []string{
		"input.executionContext.secrets.apiKey",
		"targetSnapshots[0].content",
		"input.large.private.audit.path",
		"secret-section-marker",
		"large-source-ref-should-not-be-needed-by-llm",
		`"redactedPaths"`,
		`"includedSections"`,
		`"sourceRefs"`,
		`"profile"`,
		`"sectionHashes"`,
	} {
		if strings.Contains(section.Content, forbidden) {
			t.Fatalf("context budget prompt leaked %q: %s", forbidden, section.Content)
		}
	}
	if len(section.Content) > 3000 {
		t.Fatalf("context budget prompt too large after compaction: %d bytes", len(section.Content))
	}
}
