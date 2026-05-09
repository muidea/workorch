package llm

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
)

type providerPromptMessage struct {
	Role    string
	Content string
}

type providerPromptMessageBuildResult struct {
	Messages             []providerPromptMessage
	NormalizedSections   []string
	ProtectedSections    []string
	EstimatedTokensSaved int
}

var promptDividerPattern = regexp.MustCompile(`(?m)^[-=]{3,}\s*$`)
var promptHeadingPattern = regexp.MustCompile(`(?m)^\s{0,3}#{2,}\s*`)
var promptBoldPattern = regexp.MustCompile(`\*\*([^\*]+)\*\*`)
var promptItalicPattern = regexp.MustCompile(`\*([^\*\n]+)\*`)

func buildProviderPromptMessages(request llmmodels.ProviderRequest) providerPromptMessageBuildResult {
	if turns := requestPromptContextTurns(request); len(turns) > 0 {
		ret := providerPromptMessageBuildResult{
			Messages:          make([]providerPromptMessage, 0, len(turns)+1),
			ProtectedSections: append([]string(nil), protectedPromptSections(request)...),
		}
		for _, turn := range turns {
			role := normalizedPromptRole(turn.Role)
			content := strings.TrimSpace(turn.Content)
			if content == "" {
				continue
			}
			ret.Messages = append(ret.Messages, providerPromptMessage{Role: role, Content: content})
		}
		if len(ret.Messages) == 0 || normalizedPromptRole(ret.Messages[0].Role) != "system" {
			ret.Messages = append([]providerPromptMessage{{Role: "system", Content: systemPrompt()}}, ret.Messages...)
		}
		if len(ret.Messages) == 1 {
			ret.Messages = append(ret.Messages, providerPromptMessage{Role: "user", Content: userPrompt(request)})
		}
		return ret
	}

	build := providerPromptMessageBuildResult{
		Messages: []providerPromptMessage{
			{Role: "system", Content: systemPrompt()},
		},
	}
	appendSection := func(id, role, policy, content string) {
		content = strings.TrimSpace(content)
		if content == "" {
			return
		}
		protected := policy == "raw_required"
		normalizedContent := content
		if !protected {
			normalizedText, saved := normalizePromptText(content)
			if normalizedText != "" {
				normalizedContent = normalizedText
				if saved > 0 {
					build.EstimatedTokensSaved += saved
					build.NormalizedSections = append(build.NormalizedSections, id)
				}
			}
		} else {
			build.ProtectedSections = append(build.ProtectedSections, id)
		}
		build.Messages = append(build.Messages, providerPromptMessage{
			Role:    normalizedPromptRole(role),
			Content: normalizedContent,
		})
	}

	appendSection("request-intent", "user", "normalize_allowed", requestIntentPrompt(request))
	appendSection("language-and-contract", "user", "normalize_allowed", strings.Join([]string{
		dialogLanguageInstruction(request),
		decisionContractPrompt(),
		capabilityContractPrompt(request),
	}, "\n\n"))
	appendSection("workspace-run", "user", "normalize_allowed", marshalPromptSection("Workspace and run context", map[string]any{
		"workspaceId": request.WorkspaceID,
		"runId":       request.RunID,
		"stepId":      request.StepID,
		"kind":        request.Kind,
		"model":       request.Model,
		"schema":      request.Schema,
	}))
	appendSection("goal-contract-original", "user", "raw_required", goalPromptSection(request.Input))
	appendSection("dod-contract-original", "user", "raw_required", dodPromptSection(request.Input))
	appendSection("effective-goal-dod", "user", "normalize_allowed", effectiveGoalDoDPromptSection(request.Input))
	appendSection("previous-decision", "assistant", "normalize_allowed", previousDecisionPromptSection(request.Input))
	appendSection("execution-signals", "user", "normalize_allowed", executionSignalsPromptSection(request))
	appendSection("current-action", "user", "normalize_allowed", currentActionPromptSection(request))

	if len(build.Messages) == 1 {
		build.Messages = append(build.Messages, providerPromptMessage{Role: "user", Content: userPrompt(request)})
	}
	return build
}

func buildPromptContextFromRequest(request llmmodels.ProviderRequest) *llmmodels.PromptContext {
	build := buildProviderPromptMessages(request)
	if len(build.Messages) == 0 {
		return nil
	}
	turns := make([]llmmodels.PromptTurn, 0, len(build.Messages))
	sections := make([]llmmodels.PromptSection, 0, len(build.Messages))
	for idx, item := range build.Messages {
		content := strings.TrimSpace(item.Content)
		if content == "" {
			continue
		}
		sectionID := fmt.Sprintf("turn-%02d", idx+1)
		turns = append(turns, llmmodels.PromptTurn{
			Role:      normalizedPromptRole(item.Role),
			Content:   content,
			SectionID: sectionID,
		})
		sections = append(sections, llmmodels.PromptSection{
			ID:      sectionID,
			Role:    normalizedPromptRole(item.Role),
			Policy:  "normalize_allowed",
			Content: content,
		})
	}
	if len(turns) == 0 {
		return nil
	}
	return &llmmodels.PromptContext{
		Sections: sections,
		Turns:    turns,
		Normalization: &llmmodels.NormalizationManifest{
			NormalizedSections:   uniqueStrings(build.NormalizedSections),
			ProtectedSections:    uniqueStrings(build.ProtectedSections),
			EstimatedTokensSaved: build.EstimatedTokensSaved,
			SemanticLoss:         false,
		},
	}
}

func requestPromptContextTurns(request llmmodels.ProviderRequest) []llmmodels.PromptTurn {
	if request.PromptContext == nil || len(request.PromptContext.Turns) == 0 {
		return nil
	}
	return request.PromptContext.Turns
}

func protectedPromptSections(request llmmodels.ProviderRequest) []string {
	if request.PromptContext == nil || request.PromptContext.Normalization == nil {
		return nil
	}
	return request.PromptContext.Normalization.ProtectedSections
}

func requestIntentPrompt(request llmmodels.ProviderRequest) string {
	if request.Repair != nil {
		return strings.Join([]string{
			"Repair the invalid workorch.llmExecutionDecision.v1 json output.",
			"Return only the corrected JSON object. Do not change the business intent except where required to satisfy the schema.",
		}, "\n")
	}
	return "Plan the next workspace execution step using the multi-turn context below."
}

func goalPromptSection(input map[string]any) string {
	section := map[string]any{}
	executionContext := recordFromAny(input["executionContext"])
	copyMapValue(section, "goal", executionContext["goal"])
	copyMapValue(section, "goals", executionContext["goals"])
	copyMapValue(section, "goalDefinitionPolicy", executionContext["goalDefinitionPolicy"])
	copyMapValue(section, "topLevelGoal", input["goal"])
	copyMapValue(section, "topLevelGoals", input["goals"])
	if len(section) == 0 {
		return ""
	}
	return marshalPromptSection("Original Goal Contract (Do not rewrite or compress)", section)
}

func dodPromptSection(input map[string]any) string {
	section := map[string]any{}
	executionContext := recordFromAny(input["executionContext"])
	copyMapValue(section, "dod", executionContext["dod"])
	copyMapValue(section, "dods", executionContext["dods"])
	copyMapValue(section, "doDDefinitionPolicy", executionContext["doDDefinitionPolicy"])
	copyMapValue(section, "topLevelDoD", input["dod"])
	copyMapValue(section, "topLevelDoDs", input["dods"])
	if len(section) == 0 {
		return ""
	}
	return marshalPromptSection("Original DoD Contract (Do not rewrite or compress)", section)
}

func effectiveGoalDoDPromptSection(input map[string]any) string {
	executionContext := recordFromAny(input["executionContext"])
	section := map[string]any{}
	copyMapValue(section, "goal", executionContext["goal"])
	copyMapValue(section, "dod", executionContext["dod"])
	copyMapValue(section, "goalId", input["goalId"])
	copyMapValue(section, "doDId", input["doDId"])
	if len(section) == 0 {
		return ""
	}
	return marshalPromptSection("Effective Goal and DoD View", section)
}

func previousDecisionPromptSection(input map[string]any) string {
	value, ok := input["previousStepOutput"]
	if !ok {
		return ""
	}
	return marshalPromptSection("Previous Decision Output", value)
}

func executionSignalsPromptSection(request llmmodels.ProviderRequest) string {
	payload := map[string]any{
		"input":               request.Input,
		"runtimeCapabilities": request.RuntimeCapabilities,
		"traceRefs":           request.TraceRefs,
	}
	if request.ContextBudget != nil {
		payload["contextBudget"] = request.ContextBudget
	}
	if request.Repair != nil {
		payload["repair"] = request.Repair
	}
	return marshalPromptSection("Execution Context and Evidence Signals", payload)
}

func currentActionPromptSection(request llmmodels.ProviderRequest) string {
	if request.Repair != nil {
		return marshalPromptSection("Current Requested Action", map[string]any{
			"type": "repair_invalid_json",
			"requirements": []string{
				"Return one valid JSON object matching workorch.llmExecutionDecision.v1",
				"Keep decision intent unless schema correction requires a change",
			},
		})
	}
	return marshalPromptSection("Current Requested Action", map[string]any{
		"type": "plan_next_step",
		"requirements": []string{
			"Use runtimeCapabilities only",
			"Respect unresolved governance constraints",
			"Treat effective Goal and DoD as hard constraints",
		},
	})
}

func marshalPromptSection(title string, value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return string(data)
	}
	return fmt.Sprintf("%s\n%s", title, string(data))
}

func normalizedPromptRole(role string) string {
	role = strings.ToLower(strings.TrimSpace(role))
	switch role {
	case "assistant", "system", "user":
		return role
	default:
		return "user"
	}
}

func normalizePromptText(content string) (string, int) {
	original := content
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	content = strings.TrimSpace(content)
	content = promptDividerPattern.ReplaceAllString(content, "")
	content = promptHeadingPattern.ReplaceAllString(content, "")
	content = promptBoldPattern.ReplaceAllString(content, "$1")
	content = promptItalicPattern.ReplaceAllString(content, "$1")
	content = strings.Join(compactPromptLines(strings.Split(content, "\n")), "\n")
	if content == "" {
		return strings.TrimSpace(original), 0
	}
	saved := len(strings.TrimSpace(original)) - len(content)
	if saved < 0 {
		saved = 0
	}
	return content, saved
}

func compactPromptLines(lines []string) []string {
	if len(lines) == 0 {
		return nil
	}
	out := make([]string, 0, len(lines))
	blankPending := false
	for _, line := range lines {
		trimmed := strings.TrimRight(line, " \t")
		if strings.TrimSpace(trimmed) == "" {
			if blankPending {
				continue
			}
			blankPending = true
			out = append(out, "")
			continue
		}
		blankPending = false
		out = append(out, trimmed)
	}
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	return out
}

func copyMapValue(target map[string]any, key string, value any) {
	if target == nil || strings.TrimSpace(key) == "" || value == nil {
		return
	}
	if cloned, ok := cloneAny(value); ok {
		target[key] = cloned
	}
}

func cloneAny(value any) (any, bool) {
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, false
	}
	var out any
	if err := json.Unmarshal(payload, &out); err != nil {
		return nil, false
	}
	return out, true
}
