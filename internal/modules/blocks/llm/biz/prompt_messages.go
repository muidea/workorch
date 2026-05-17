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
			ret.Messages = append([]providerPromptMessage{{Role: "system", Content: systemPromptForRequest(request)}}, ret.Messages...)
		}
		if len(ret.Messages) == 1 {
			ret.Messages = append(ret.Messages, providerPromptMessage{Role: "user", Content: userPrompt(request)})
		}
		return ret
	}

	build := providerPromptMessageBuildResult{
		Messages: []providerPromptMessage{
			{Role: "system", Content: systemPromptForRequest(request)},
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
	appendSection("language-and-contract", "user", "normalize_allowed", languageAndContractPrompt(request))
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
	appendSection("governance-soul-policy", "user", "normalize_allowed", governanceSoulPolicyPromptSection(request.Input))
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
	if !isDecisionSchemaRequest(request) {
		if request.Repair != nil {
			return strings.Join([]string{
				fmt.Sprintf("Repair the invalid JSON output for schema %s.", schemaNameForPrompt(request)),
				"Return only the corrected JSON object. Do not change the business intent except where required to satisfy the requested schema.",
			}, "\n")
		}
		return fmt.Sprintf("Generate or revise the requested structured JSON for kind %s.", kindNameForPrompt(request))
	}
	if request.Repair != nil {
		return strings.Join([]string{
			"Repair the invalid workorch.llmExecutionDecision.v1 json output.",
			"Return only the corrected JSON object. Do not change the business intent except where required to satisfy the schema.",
		}, "\n")
	}
	return "Plan the next workspace execution step using the multi-turn context below."
}

func languageAndContractPrompt(request llmmodels.ProviderRequest) string {
	if isDecisionSchemaRequest(request) {
		return strings.Join([]string{
			dialogLanguageInstruction(request),
			decisionContractPrompt(request),
			capabilityContractPrompt(request),
		}, "\n\n")
	}
	return strings.Join([]string{
		structuredJSONLanguageInstruction(request),
		structuredJSONContractPrompt(request),
	}, "\n\n")
}

func isDecisionSchemaRequest(request llmmodels.ProviderRequest) bool {
	schema := strings.TrimSpace(request.Schema)
	return schema == "" || schema == decisionSchemaName
}

func schemaNameForPrompt(request llmmodels.ProviderRequest) string {
	schema := strings.TrimSpace(request.Schema)
	if schema == "" {
		return "the requested output schema"
	}
	return schema
}

func kindNameForPrompt(request llmmodels.ProviderRequest) string {
	kind := strings.TrimSpace(request.Kind)
	if kind == "" {
		return "structured_json"
	}
	return kind
}

func structuredJSONLanguageInstruction(request llmmodels.ProviderRequest) string {
	language := strings.TrimSpace(request.DialogLanguage)
	if language == "" {
		language = "English"
	}
	return fmt.Sprintf("Conversation language: %s. Use this language for user-facing text values. Keep JSON keys, enum values, identifiers, file paths, schema names, and evidence refs unchanged.", language)
}

func structuredJSONContractPrompt(request llmmodels.ProviderRequest) string {
	return strings.Join([]string{
		fmt.Sprintf("Required output JSON must match schema %s.", schemaNameForPrompt(request)),
		"Return exactly one JSON object. Do not wrap it in markdown.",
		"Follow input.instructions and input.outputSchema when present.",
		"Use request.input.request, message history, template context, currentDraft, and any other supplied input fields as the source of truth.",
		"Do not return workorch.llmExecutionDecision.v1 fields such as completionState, requiredDecision, requiredApproval, executionGraph, patchRequests, shellCommands, mcpToolCalls, memoryWrites, or dodAssessment unless the requested output schema explicitly defines them.",
		"When revising a draft after a new user message, do not repeat stale clarifying questions. Either revise the draft, return no clarifyingQuestions, or ask only still-missing specific questions.",
	}, "\n")
}

func goalPromptSection(input map[string]any) string {
	section := map[string]any{}
	executionContext := recordFromAny(input["executionContext"])
	executionContextPackage := recordFromAny(input["executionContextPackage"])
	copyMapValue(section, "goal", executionContext["goal"])
	copyMapValue(section, "goals", executionContext["goals"])
	copyMapValue(section, "goalDefinitionPolicy", executionContext["goalDefinitionPolicy"])
	copyMapValue(section, "packagedGoal", executionContextPackage["goal"])
	copyMapValue(section, "packagedGoals", executionContextPackage["goals"])
	copyMapValue(section, "packagedGoalDefinitionPolicy", executionContextPackage["goalDefinitionPolicy"])
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
	executionContextPackage := recordFromAny(input["executionContextPackage"])
	copyMapValue(section, "dod", executionContext["dod"])
	copyMapValue(section, "dods", executionContext["dods"])
	copyMapValue(section, "doDDefinitionPolicy", executionContext["doDDefinitionPolicy"])
	copyMapValue(section, "packagedDoD", executionContextPackage["dod"])
	copyMapValue(section, "packagedDoDs", executionContextPackage["dods"])
	copyMapValue(section, "packagedDoDDefinitionPolicy", executionContextPackage["doDDefinitionPolicy"])
	copyMapValue(section, "topLevelDoD", input["dod"])
	copyMapValue(section, "topLevelDoDs", input["dods"])
	if len(section) == 0 {
		return ""
	}
	return marshalPromptSection("Original DoD Contract (Do not rewrite or compress)", section)
}

func effectiveGoalDoDPromptSection(input map[string]any) string {
	executionContext := recordFromAny(input["executionContext"])
	executionContextPackage := recordFromAny(input["executionContextPackage"])
	section := map[string]any{}
	copyMapValue(section, "goal", firstNonNilValue(executionContext["goal"], executionContextPackage["goal"]))
	copyMapValue(section, "dod", firstNonNilValue(executionContext["dod"], executionContextPackage["dod"]))
	copyMapValue(section, "goals", firstNonNilValue(executionContext["goals"], executionContextPackage["goals"]))
	copyMapValue(section, "dods", firstNonNilValue(executionContext["dods"], executionContextPackage["dods"]))
	copyMapValue(section, "goalId", input["goalId"])
	copyMapValue(section, "doDId", input["doDId"])
	copyMapValue(section, "packagedGoalId", executionContextPackage["goalId"])
	copyMapValue(section, "packagedDoDId", executionContextPackage["doDId"])
	if len(section) == 0 {
		return ""
	}
	return marshalPromptSection("Effective Goal and DoD View", section)
}

func governanceSoulPolicyPromptSection(input map[string]any) string {
	executionContext := recordFromAny(input["executionContext"])
	section := map[string]any{}
	copyMapValue(section, "currentSoulStage", input["currentSoulStage"])
	copyMapValue(section, "currentSoul", input["currentSoul"])
	copyMapValue(section, "currentSoulBinding", input["currentSoulBinding"])
	routing := recordFromAny(executionContext["soulRouting"])
	bindings := recordFromAny(routing["bindings"])
	if len(bindings) > 0 {
		selectedBindings := map[string]any{}
		for _, stage := range []string{"planning", "execution", "validation", "decision", "approval"} {
			if binding := recordFromAny(bindings[stage]); len(binding) > 0 {
				selectedBindings[stage] = binding
			}
		}
		if len(selectedBindings) > 0 {
			section["stageBindings"] = selectedBindings
		}
	}
	if len(section) == 0 {
		return ""
	}
	return marshalPromptSection("Governance and Soul Policy View", section)
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
	if stage := strings.TrimSpace(stringValue(request.Input["currentRuntimeStage"])); stage != "" {
		payload["runtimeStage"] = stage
	}
	if mode := strings.TrimSpace(stringValue(request.Input["currentRuntimeStageMode"])); mode != "" {
		payload["runtimeStageMode"] = mode
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
	if !isDecisionSchemaRequest(request) {
		if request.Repair != nil {
			return marshalPromptSection("Current Requested Action", map[string]any{
				"type": "repair_structured_json",
				"requirements": []string{
					"Return one valid JSON object matching " + schemaNameForPrompt(request),
					"Keep the original structured output intent unless schema correction requires a change",
				},
			})
		}
		return marshalPromptSection("Current Requested Action", map[string]any{
			"type": "generate_structured_json",
			"requirements": []string{
				"Match " + schemaNameForPrompt(request),
				"Use the supplied input instructions and output schema",
				"Do not emit execution-planning fields unless explicitly requested by the output schema",
			},
		})
	}
	if request.Repair != nil {
		requirements := []string{
			"Return one valid JSON object matching workorch.llmExecutionDecision.v1",
			"Keep decision intent unless schema correction requires a change",
		}
		if isAssessmentOnlyDecisionRequest(request) {
			requirements = append(requirements, "Do not include executionGraph or any executable work fields")
		}
		return marshalPromptSection("Current Requested Action", map[string]any{
			"type":         "repair_invalid_json",
			"requirements": requirements,
		})
	}
	if isAssessmentOnlyDecisionRequest(request) {
		return marshalPromptSection("Current Requested Action", map[string]any{
			"type": "assess_only",
			"requirements": []string{
				"Assess the current evidence against the effective Goal and DoD",
				"Return completed or failed with dodAssessment, or a real requiredDecision/requiredApproval",
				"When Goal requirements are not textually identical to DoD items, include dodAssessment.goalCoverage with item-level evidenceRefs",
				"When executionContract.profileContracts declares required acceptanceDimensions or deliverableExpectations, include dodAssessment.acceptanceDimensions and dodAssessment.deliverableCoverage with item-level evidenceRefs or artifactRefs for every required id",
				"dodAssessment.goalCoverage, dodAssessment.acceptanceDimensions, and dodAssessment.deliverableCoverage must be objects shaped as {\"satisfied\":boolean,\"items\":[...]}; never return these coverage fields as bare arrays",
				"Treat runtimeStage=verification or acceptance as strict convergence stages; do not reopen exploration or broad planning in these stages",
				"Before returning blocked_on_decision, reconcile the question with the current, decision, planning, and execution Soul policies; do not ask for information that those policies explicitly say is not required",
				"When evidence is insufficient but the missing work can be handled by later runtime repair or replan, return failed with dodAssessment.nextActions instead of asking the user to choose the execution target",
				"Do not include executionGraph, shellCommands, patchRequests, mcpToolCalls, memoryWrites, nextAction, or executable work",
			},
		})
	}
	if boolValue(request.Input["stateChangingOutputRequired"]) &&
		(strings.TrimSpace(stringValue(request.Input["stateChangingRecoveryMode"])) == "direct_state_change" ||
			boolValue(request.Input["providerTimeoutStateChangingRetry"])) {
		return marshalPromptSection("Current Requested Action", map[string]any{
			"type": "direct_state_changing_recovery",
			"requirements": []string{
				"Use the effective Goal and DoD already supplied in the request",
				"Return direct state-changing output, failed dodAssessment, or a real requiredDecision/requiredApproval blocker",
				"Do not return run snapshot, observability, self-inspection, context rehydration, or replan-through-LLM graphs",
				"When failed evidence includes current target snapshots, update those targets directly with auditable state-change artifacts before rerunning verification",
				"Do not use shell commands to modify workspace files when a patch artifact can represent the change",
				"If an earlier stream was incomplete, continue by producing a complete fresh JSON object rather than inspecting the run again",
			},
		})
	}
	return marshalPromptSection("Current Requested Action", map[string]any{
		"type": "plan_next_step",
		"requirements": []string{
			"Use runtimeCapabilities only",
			"Respect unresolved governance constraints",
			"Treat effective Goal and DoD as hard constraints",
			"When runtimeStageMode is open_judgment, prefer context gathering, plan refinement, and constraint clarification over premature state-changing output",
			"When runtimeStageMode is strict_convergence, produce executable output, failed assessment, or a real blocker; do not continue read-only looping",
			"If the current workspace is empty and the effective Goal/DoD can be satisfied by creating workspace deliverables with available runtime capabilities, continue with bounded state-changing work instead of asking whether initialization is allowed",
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

func firstNonNilValue(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
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
