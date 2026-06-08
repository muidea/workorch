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
		"hunk/context/base mismatch",
		"Do not use shell commands to modify workspace files",
		"complete fresh JSON object",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("prompt missing %q, joined=%s", want, joined)
		}
	}
	if strings.Contains(joined, "\"type\":\"plan_next_step\"") {
		t.Fatalf("state-changing repair prompt still requests plan_next_step, joined=%s", joined)
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
