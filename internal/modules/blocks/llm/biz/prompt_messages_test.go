package llm

import (
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
