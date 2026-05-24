package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
)

type providerPromptMessage struct {
	Role               string
	Content            string
	SectionID          string
	SectionName        string
	Policy             string
	Cacheability       string
	SourceRefs         []string
	DefinitionRevision string
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
			ret.Messages = append(ret.Messages, providerPromptMessage{
				Role:      role,
				Content:   content,
				SectionID: strings.TrimSpace(turn.SectionID),
			})
		}
		if len(ret.Messages) == 0 || normalizedPromptRole(ret.Messages[0].Role) != "system" {
			ret.Messages = append([]providerPromptMessage{promptMessageForSection(request, "C0.system", "system", "normalize_allowed", systemPromptForRequest(request))}, ret.Messages...)
		}
		if len(ret.Messages) == 1 {
			ret.Messages = append(ret.Messages, promptMessageForSection(request, "C4.fallback-user-prompt", "user", "normalize_allowed", userPrompt(request)))
		}
		return ret
	}

	build := providerPromptMessageBuildResult{
		Messages: []providerPromptMessage{
			promptMessageForSection(request, "C0.system", "system", "normalize_allowed", systemPromptForRequest(request)),
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
		build.Messages = append(build.Messages, promptMessageForSection(request, id, role, policy, normalizedContent))
	}

	appendSection("C0.request-intent", "user", "normalize_allowed", requestIntentPrompt(request))
	appendSection("C0.language-and-contract", "user", "normalize_allowed", languageAndContractPrompt(request))
	appendSection("C1.goal-contract-original", "user", "raw_required", goalPromptSection(request.Input))
	appendSection("C1.dod-contract-original", "user", "raw_required", dodPromptSection(request.Input))
	appendSection("C1.effective-goal-dod", "user", "normalize_allowed", effectiveGoalDoDPromptSection(request.Input))
	appendSection("C2.capability-contract", "user", "normalize_allowed", capabilityPromptSection(request))
	appendSection("C2.governance-soul-policy", "user", "normalize_allowed", governanceSoulPolicyPromptSection(request.Input))
	appendSection("C2.runtime-capability-summary", "user", "normalize_allowed", runtimeCapabilitySummaryPromptSection(request))
	appendSection("C3.workspace-run", "user", "normalize_allowed", marshalPromptSection("Workspace and run context", map[string]any{
		"workspaceId": request.WorkspaceID,
		"runId":       request.RunID,
		"stepId":      request.StepID,
		"kind":        request.Kind,
		"model":       request.Model,
		"schema":      request.Schema,
	}))
	appendSection("C3.stage-contract", "user", "normalize_allowed", stageContractPromptSection(request.Input))
	appendSection("C3.repair-action-package", "user", "normalize_allowed", repairActionPackagePromptSection(request.Input))
	appendSection("C4.previous-decision", "assistant", "normalize_allowed", previousDecisionPromptSection(request.Input))
	for _, event := range executionSignalPromptEvents(request) {
		appendSection(event.SectionID, event.Role, "normalize_allowed", event.Content)
	}
	appendSection("C4.current-action", "user", "normalize_allowed", currentActionPromptSection(request))

	if len(build.Messages) == 1 {
		build.Messages = append(build.Messages, promptMessageForSection(request, "C4.fallback-user-prompt", "user", "normalize_allowed", userPrompt(request)))
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
	usedSectionIDs := map[string]int{}
	for idx, item := range build.Messages {
		content := strings.TrimSpace(item.Content)
		if content == "" {
			continue
		}
		sectionID := normalizedPromptSectionID(item.SectionID, idx)
		if seen := usedSectionIDs[sectionID]; seen > 0 {
			usedSectionIDs[sectionID] = seen + 1
			sectionID = fmt.Sprintf("%s.%02d", sectionID, seen+1)
		} else {
			usedSectionIDs[sectionID] = 1
		}
		policy := strings.TrimSpace(item.Policy)
		if policy == "" {
			policy = "normalize_allowed"
		}
		turns = append(turns, llmmodels.PromptTurn{
			Role:      normalizedPromptRole(item.Role),
			Content:   content,
			SectionID: sectionID,
		})
		sections = append(sections, llmmodels.PromptSection{
			ID:                 sectionID,
			Name:               item.SectionName,
			Role:               normalizedPromptRole(item.Role),
			Policy:             policy,
			Cacheability:       item.Cacheability,
			Hash:               promptSectionHash(sectionID, content),
			SourceRefs:         append([]string(nil), item.SourceRefs...),
			DefinitionRevision: item.DefinitionRevision,
			DisplayOrder:       len(sections) + 1,
			Content:            content,
			Protected:          policy == "raw_required",
		})
	}
	if len(turns) == 0 {
		return nil
	}
	return &llmmodels.PromptContext{
		SchemaVersion: "workorch.promptContext.cacheAware.v1",
		Sections:      sections,
		Turns:         turns,
		Normalization: &llmmodels.NormalizationManifest{
			NormalizedSections:   uniqueStrings(build.NormalizedSections),
			ProtectedSections:    uniqueStrings(build.ProtectedSections),
			EstimatedTokensSaved: build.EstimatedTokensSaved,
			SemanticLoss:         false,
		},
	}
}

func promptMessageForSection(request llmmodels.ProviderRequest, sectionID, role, policy, content string) providerPromptMessage {
	meta := promptSectionMetadata(request, sectionID)
	if policy = strings.TrimSpace(policy); policy != "" {
		meta.Policy = policy
	}
	return providerPromptMessage{
		Role:               normalizedPromptRole(role),
		Content:            content,
		SectionID:          meta.ID,
		SectionName:        meta.Name,
		Policy:             meta.Policy,
		Cacheability:       meta.Cacheability,
		SourceRefs:         append([]string(nil), meta.SourceRefs...),
		DefinitionRevision: meta.DefinitionRevision,
	}
}

func promptSectionMetadata(request llmmodels.ProviderRequest, sectionID string) llmmodels.PromptSection {
	sectionID = strings.TrimSpace(sectionID)
	if sectionID == "" {
		sectionID = "C4.unknown"
	}
	meta := llmmodels.PromptSection{
		ID:           sectionID,
		Name:         sectionNameFromID(sectionID),
		Policy:       "normalize_allowed",
		Cacheability: "volatile",
		SourceRefs:   []string{"providerRequest"},
	}
	switch promptSectionGroup(sectionID) {
	case "C0":
		meta.Cacheability = "stable"
		meta.SourceRefs = []string{"runtimeContract", "schemaContract"}
	case "C1":
		meta.Cacheability = "workspace_definition"
		meta.SourceRefs = []string{"executionContextPackage", "executionContext"}
		meta.DefinitionRevision = promptDefinitionRevision(request.Input)
	case "C2":
		meta.Cacheability = "capability_contract"
		meta.SourceRefs = []string{"runtimeCapabilities", "governancePolicy"}
	case "C3":
		meta.Cacheability = "stage_bundle"
		meta.SourceRefs = []string{"runtimeStage", "workspaceRun"}
	case "C4":
		meta.Cacheability = "volatile_tail"
		meta.SourceRefs = []string{"executionContext", "processEvidenceTimeline", "currentAction"}
	case "C5":
		meta.Cacheability = "large_refs"
		meta.SourceRefs = []string{"traceRefs"}
	}
	return meta
}

func normalizedPromptSectionID(sectionID string, idx int) string {
	sectionID = strings.TrimSpace(sectionID)
	if sectionID != "" {
		return sectionID
	}
	return fmt.Sprintf("C4.turn-%02d", idx+1)
}

func sectionNameFromID(sectionID string) string {
	parts := strings.Split(strings.TrimSpace(sectionID), ".")
	if len(parts) == 0 {
		return ""
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return strings.Join(parts[1:], ".")
}

func promptSectionGroup(sectionID string) string {
	sectionID = strings.TrimSpace(sectionID)
	if len(sectionID) < 2 {
		return ""
	}
	if strings.HasPrefix(sectionID, "C0") {
		return "C0"
	}
	if strings.HasPrefix(sectionID, "C1") {
		return "C1"
	}
	if strings.HasPrefix(sectionID, "C2") {
		return "C2"
	}
	if strings.HasPrefix(sectionID, "C3") {
		return "C3"
	}
	if strings.HasPrefix(sectionID, "C4") {
		return "C4"
	}
	if strings.HasPrefix(sectionID, "C5") {
		return "C5"
	}
	return ""
}

func promptDefinitionRevision(input map[string]any) string {
	for _, source := range []map[string]any{
		recordFromAny(input["executionContextPackage"]),
		recordFromAny(input["executionContext"]),
		input,
	} {
		for _, key := range []string{"definitionRevision", "goalDoDRevision", "goalRevision", "doDRevision"} {
			if revision := strings.TrimSpace(stringValue(source[key])); revision != "" {
				return revision
			}
		}
		if impact := recordFromAny(source["definitionChangeImpact"]); len(impact) > 0 {
			for _, key := range []string{"definitionRevision", "currentRevision", "revision"} {
				if revision := strings.TrimSpace(stringValue(impact[key])); revision != "" {
					return revision
				}
			}
		}
	}
	return ""
}

func promptSectionHash(sectionID, content string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(sectionID) + "\n" + strings.TrimSpace(content)))
	return "sha256:" + hex.EncodeToString(sum[:])
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
		}, "\n\n")
	}
	return strings.Join([]string{
		structuredJSONLanguageInstruction(request),
		structuredJSONContractPrompt(request),
	}, "\n\n")
}

func capabilityPromptSection(request llmmodels.ProviderRequest) string {
	return capabilityContractPrompt(request)
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
	copyMapValue(section, "definitionChangeImpact", firstNonNilValue(executionContext["definitionChangeImpact"], executionContextPackage["definitionChangeImpact"]))
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
	copyMapValue(section, "definitionChangeImpact", firstNonNilValue(executionContext["definitionChangeImpact"], executionContextPackage["definitionChangeImpact"]))
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
	copyMapValue(section, "definitionChangeImpact", firstNonNilValue(executionContext["definitionChangeImpact"], executionContextPackage["definitionChangeImpact"]))
	if len(section) == 0 {
		return ""
	}
	return marshalPromptSection("Effective Goal and DoD View", section)
}

func stageContractPromptSection(input map[string]any) string {
	section := map[string]any{}
	for _, key := range []string{
		"currentRuntimeStage",
		"currentRuntimeStageMode",
		"runtimeStage",
		"runtimeStageMode",
		"currentSoulStage",
		"currentSoul",
		"currentSoulBinding",
		"stateChangingOutputRequired",
		"repairObjective",
		"stateChangingRecoveryMode",
		"providerTimeoutStateChangingRetry",
	} {
		copyMapValue(section, key, input[key])
	}
	if revision := promptDefinitionRevision(input); revision != "" {
		section["definitionRevision"] = revision
	}
	copyMapValue(section, "definitionChangeImpact", definitionChangeImpactPayload(input))
	if pkg := recordFromAny(input["repairActionPackage"]); len(pkg) > 0 {
		copyMapValue(section, "requiredOutput", pkg["requiredOutput"])
		copyMapValue(section, "forbiddenActions", pkg["forbiddenActions"])
		copyMapValue(section, "exitCriteria", pkg["exitCriteria"])
	}
	if len(section) == 0 {
		return ""
	}
	return marshalPromptSection("Stage Contract", section)
}

func repairActionPackagePromptSection(input map[string]any) string {
	pkg := recordFromAny(input["repairActionPackage"])
	if len(pkg) == 0 {
		return ""
	}
	section := map[string]any{}
	for _, key := range []string{
		"currentFailure",
		"lastFailureDiagnostics",
		"requiredOutput",
		"structuredFailureSignals",
		"targetSnapshots",
		"workspaceFileManifest",
		"currentExecutionProgress",
		"coverageTargetSet",
		"recentRepairAttempts",
		"previousRejectedOutputSummary",
		"forbiddenActions",
		"repairBudget",
		"exitCriteria",
		"failureClass",
		"failureSignature",
		"failureTargetScope",
		"failedStepId",
		"failedStepKind",
		"reason",
	} {
		copyMapValue(section, key, pkg[key])
	}
	if len(section) == 0 {
		section = pkg
	}
	return marshalPromptSection("Current Repair Action Package", section)
}

func governanceSoulPolicyPromptSection(input map[string]any) string {
	executionContext := recordFromAny(input["executionContext"])
	section := map[string]any{}
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

type executionSignalPromptEvent struct {
	SectionID string
	Role      string
	Content   string
}

func executionSignalPromptEvents(request llmmodels.ProviderRequest) []executionSignalPromptEvent {
	events := make([]executionSignalPromptEvent, 0, 8)
	appendEvent := func(sectionID, title string, value any) {
		content := marshalPromptSection(title, value)
		if strings.TrimSpace(content) == "" {
			return
		}
		events = append(events, executionSignalPromptEvent{
			SectionID: sectionID,
			Role:      "user",
			Content:   content,
		})
	}
	if stage := executionStageSignalPayload(request); len(stage) > 0 {
		appendEvent("C4.runtime-stage", "Runtime Stage Signal", stage)
	}
	if currentFailure := currentFailureSignalPayload(request.Input); len(currentFailure) > 0 {
		appendEvent("C4.current-failure", "Current Failure Signal", currentFailure)
	}
	if progress := currentProgressSignalPayload(request.Input); len(progress) > 0 {
		appendEvent("C4.current-progress", "Current Progress Signal", progress)
	}
	if timeline := processEvidenceTimelineSignalPayload(request.Input); len(timeline) > 0 {
		appendEvent("C4.process-evidence-timeline", "Process Evidence Timeline", timeline)
	}
	if impact := definitionChangeImpactSignalPayload(request.Input); len(impact) > 0 {
		appendEvent("C4.definition-change-impact", "Definition Change Impact Signal", impact)
	}
	if validation := validationSignalPayload(request.Input); len(validation) > 0 {
		appendEvent("C4.validation-signal", "Validation Signal", validation)
	}
	if governance := governanceDecisionSignalPayload(request.Input); len(governance) > 0 {
		appendEvent("C4.governance-resolution", "Governance Resolution Signal", governance)
	}
	if memory := memorySignalPayload(request.Input); len(memory) > 0 {
		appendEvent("C4.memory-recall", "Memory Recall Signal", memory)
	}
	if len(request.TraceRefs) > 0 {
		appendEvent("C5.trace-refs", "Trace References", request.TraceRefs)
	}
	if request.ContextBudget != nil {
		appendEvent("C5.context-budget", "Context Budget Audit", request.ContextBudget)
	}
	if request.Repair != nil {
		appendEvent("C4.repair-json-output", "Structured JSON Repair Signal", request.Repair)
	}
	if len(events) == 0 {
		appendEvent("C4.execution-signals", "Execution Context and Evidence Signals", executionSignalsPayload(request))
	}
	return events
}

func runtimeCapabilitySummaryPromptSection(request llmmodels.ProviderRequest) string {
	if len(request.RuntimeCapabilities.BlockKinds) == 0 &&
		len(request.RuntimeCapabilities.ServiceAPIs) == 0 &&
		!request.RuntimeCapabilities.ShellPolicy.Enabled &&
		len(request.RuntimeCapabilities.ShellPolicy.UnavailableCommands) == 0 {
		return ""
	}
	return marshalPromptSection("Runtime Capability Summary", request.RuntimeCapabilities)
}

func executionSignalsPromptSection(request llmmodels.ProviderRequest) string {
	return marshalPromptSection("Execution Context and Evidence Signals", executionSignalsPayload(request))
}

func executionSignalsPayload(request llmmodels.ProviderRequest) map[string]any {
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
	return payload
}

func executionStageSignalPayload(request llmmodels.ProviderRequest) map[string]any {
	payload := map[string]any{}
	for _, key := range []string{
		"currentRuntimeStage",
		"currentRuntimeStageMode",
		"runtimeStage",
		"runtimeStageMode",
		"currentSoulStage",
		"stateChangingOutputRequired",
		"repairObjective",
		"stateChangingRecoveryMode",
		"providerTimeoutStateChangingRetry",
	} {
		copyMapValue(payload, key, request.Input[key])
	}
	if stage := strings.TrimSpace(stringValue(request.Input["currentRuntimeStage"])); stage != "" {
		payload["runtimeStage"] = stage
	}
	if mode := strings.TrimSpace(stringValue(request.Input["currentRuntimeStageMode"])); mode != "" {
		payload["runtimeStageMode"] = mode
	}
	return payload
}

func currentFailureSignalPayload(input map[string]any) map[string]any {
	payload := map[string]any{}
	copyMapValue(payload, "currentFailure", input["currentFailure"])
	if pkg := recordFromAny(input["repairActionPackage"]); len(pkg) > 0 {
		copyMapValue(payload, "packagedCurrentFailure", pkg["currentFailure"])
		copyMapValue(payload, "lastFailureDiagnostics", pkg["lastFailureDiagnostics"])
		copyMapValue(payload, "structuredFailureSignals", pkg["structuredFailureSignals"])
		copyMapValue(payload, "previousRejectedOutputSummary", pkg["previousRejectedOutputSummary"])
	}
	if digest := recordFromAny(firstNonNilValue(input["currentProgressDigest"], recordFromAny(input["executionContextPackage"])["currentProgressDigest"])); len(digest) > 0 {
		copyMapValue(payload, "lastObservedFailure", digest["lastObservedFailure"])
		copyMapValue(payload, "currentGap", digest["currentGap"])
		copyMapValue(payload, "nextRequiredAction", digest["nextRequiredAction"])
	}
	return payload
}

func currentProgressSignalPayload(input map[string]any) map[string]any {
	payload := map[string]any{}
	copyMapValue(payload, "currentExecutionProgress", input["currentExecutionProgress"])
	copyMapValue(payload, "currentProgressDigest", input["currentProgressDigest"])
	if pkg := recordFromAny(input["executionContextPackage"]); len(pkg) > 0 {
		copyMapValue(payload, "packagedCurrentProgressDigest", pkg["currentProgressDigest"])
		copyMapValue(payload, "currentDeliverables", pkg["currentDeliverables"])
		copyMapValue(payload, "latestAcceptedResult", pkg["latestAcceptedResult"])
	}
	if repairPkg := recordFromAny(input["repairActionPackage"]); len(repairPkg) > 0 {
		copyMapValue(payload, "packagedCurrentExecutionProgress", repairPkg["currentExecutionProgress"])
		copyMapValue(payload, "coverageTargetSet", repairPkg["coverageTargetSet"])
		copyMapValue(payload, "requiredOutput", repairPkg["requiredOutput"])
		copyMapValue(payload, "exitCriteria", repairPkg["exitCriteria"])
	}
	return payload
}

func definitionChangeImpactSignalPayload(input map[string]any) map[string]any {
	impact := definitionChangeImpactPayload(input)
	if len(impact) == 0 {
		return nil
	}
	payload := map[string]any{"definitionChangeImpact": impact}
	if revision := promptDefinitionRevision(input); revision != "" {
		payload["definitionRevision"] = revision
	}
	copyMapValue(payload, "currentRuntimeStage", input["currentRuntimeStage"])
	copyMapValue(payload, "currentRuntimeStageMode", input["currentRuntimeStageMode"])
	copyMapValue(payload, "repairObjective", input["repairObjective"])
	return payload
}

func definitionChangeImpactPayload(input map[string]any) map[string]any {
	for _, source := range []map[string]any{
		input,
		recordFromAny(input["executionContext"]),
		recordFromAny(input["executionContextPackage"]),
	} {
		if impact := recordFromAny(source["definitionChangeImpact"]); len(impact) > 0 {
			return impact
		}
	}
	return nil
}

func processEvidenceTimelineSignalPayload(input map[string]any) []map[string]any {
	if timeline := mapSliceFromAny(input["processEvidenceTimeline"]); len(timeline) > 0 {
		return timeline
	}
	if pkg := recordFromAny(input["executionContextPackage"]); len(pkg) > 0 {
		return mapSliceFromAny(pkg["processEvidenceTimeline"])
	}
	return nil
}

func validationSignalPayload(input map[string]any) map[string]any {
	payload := map[string]any{}
	for _, key := range []string{"failedStepInput", "failedStepOutput", "failedExecutionGraph", "graphEvidence", "runEvidenceSummary", "previousResults"} {
		copyMapValue(payload, key, input[key])
	}
	if pkg := recordFromAny(input["repairActionPackage"]); len(pkg) > 0 {
		copyMapValue(payload, "targetSnapshots", pkg["targetSnapshots"])
		copyMapValue(payload, "workspaceFileManifest", pkg["workspaceFileManifest"])
		copyMapValue(payload, "recentRepairAttempts", pkg["recentRepairAttempts"])
	}
	if pkg := recordFromAny(input["executionContextPackage"]); len(pkg) > 0 {
		copyMapValue(payload, "recentFailureSummary", pkg["recentFailureSummary"])
		copyMapValue(payload, "workspaceFileManifest", pkg["workspaceFileManifest"])
	}
	return payload
}

func governanceDecisionSignalPayload(input map[string]any) map[string]any {
	payload := map[string]any{}
	copyMapValue(payload, "decisionResolution", input["decisionResolution"])
	copyMapValue(payload, "approvalResolution", input["approvalResolution"])
	if executionContext := recordFromAny(input["executionContext"]); len(executionContext) > 0 {
		copyMapValue(payload, "governanceHistory", executionContext["governanceHistory"])
	}
	if pkg := recordFromAny(input["executionContextPackage"]); len(pkg) > 0 {
		copyMapValue(payload, "packagedGovernanceHistory", pkg["governanceHistory"])
		copyMapValue(payload, "currentGovernance", pkg["currentGovernance"])
	}
	return payload
}

func memorySignalPayload(input map[string]any) map[string]any {
	payload := map[string]any{}
	if retrieval := cacheAwareMemoryRetrievalSummary(recordFromAny(input["memoryRetrieval"])); len(retrieval) > 0 {
		payload["memoryRetrieval"] = retrieval
	}
	if executionContext := recordFromAny(input["executionContext"]); len(executionContext) > 0 {
		if retrieval := cacheAwareMemoryRetrievalSummary(recordFromAny(executionContext["memoryRetrieval"])); len(retrieval) > 0 {
			payload["executionContextMemoryRetrieval"] = retrieval
		}
	}
	if pkg := recordFromAny(input["executionContextPackage"]); len(pkg) > 0 {
		if retrieval := cacheAwareMemoryRetrievalSummary(recordFromAny(pkg["memoryRetrieval"])); len(retrieval) > 0 {
			payload["packagedMemoryRetrieval"] = retrieval
		}
		if selected := cacheAwareSelectedContextMemorySignal(recordFromAny(pkg["selectedContext"])); len(selected) > 0 {
			payload["selectedContext"] = selected
		}
	}
	return payload
}

func cacheAwareSelectedContextMemorySignal(selected map[string]any) map[string]any {
	if len(selected) == 0 {
		return nil
	}
	retrieval := recordFromAny(selected["memoryRetrieval"])
	summary := map[string]any{}
	if retrievalSummary := cacheAwareMemoryRetrievalSummary(retrieval); len(retrievalSummary) > 0 {
		summary["memoryRetrieval"] = retrievalSummary
	}
	memories := cacheAwareSelectedMemories(mapSliceFromAny(selected["memories"]), retrieval)
	if len(memories) > 0 {
		summary["memories"] = memories
	}
	if len(summary) == 0 {
		return nil
	}
	summary["cacheability"] = "current_decision_reference"
	return summary
}

func cacheAwareMemoryRetrievalSummary(retrieval map[string]any) map[string]any {
	if len(retrieval) == 0 {
		return nil
	}
	out := map[string]any{}
	for _, key := range []string{
		"policy",
		"selectionPolicyVersion",
		"status",
		"refreshSource",
		"limit",
		"selectedMemoryIds",
		"excludedMemoryIds",
		"retrievedMemoryIds",
		"queryTextSha256",
		"providerId",
		"operation",
		"reason",
	} {
		copyMapValue(out, key, retrieval[key])
	}
	if scores := cacheAwareMemorySelectionScores(mapSliceFromAny(retrieval["selectionScores"])); len(scores) > 0 {
		out["selectionScores"] = scores
	}
	return out
}

func cacheAwareMemorySelectionScores(scores []map[string]any) []map[string]any {
	if len(scores) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0, len(scores))
	for _, score := range scores {
		item := map[string]any{}
		copyMapValue(item, "id", score["id"])
		copyMapValue(item, "score", score["score"])
		copyMapValue(item, "reason", score["reason"])
		if len(item) > 0 {
			ret = append(ret, item)
		}
	}
	return ret
}

func cacheAwareSelectedMemories(memories []map[string]any, retrieval map[string]any) []map[string]any {
	if len(memories) == 0 {
		return nil
	}
	selectedIDs := stringSetFromProviderValue(retrieval["selectedMemoryIds"])
	excludedIDs := stringSetFromProviderValue(retrieval["excludedMemoryIds"])
	ret := make([]map[string]any, 0, len(memories))
	for _, memory := range memories {
		if !cacheAwareMemoryAllowed(memory, selectedIDs, excludedIDs) {
			continue
		}
		item := map[string]any{}
		for _, key := range []string{"id", "memoryType", "summary", "content", "tags", "importance", "confidence", "visibility", "provenance", "vectorScore"} {
			copyMapValue(item, key, memory[key])
		}
		item["cacheability"] = cacheAwareMemoryCacheability(memory)
		ret = append(ret, item)
	}
	return ret
}

func cacheAwareMemoryAllowed(memory map[string]any, selectedIDs map[string]bool, excludedIDs map[string]bool) bool {
	id := strings.TrimSpace(firstProviderString(memory["id"], memory["ID"]))
	if id == "" {
		return false
	}
	if excludedIDs[id] || (len(selectedIDs) > 0 && !selectedIDs[id]) {
		return false
	}
	if memoryMarkedStale(memory) || memoryLowValue(memory) {
		return false
	}
	return strings.TrimSpace(stringValue(memory["summary"])) != "" || strings.TrimSpace(stringValue(memory["content"])) != ""
}

func memoryMarkedStale(memory map[string]any) bool {
	for _, key := range []string{"status", "state", "evidenceValidity", "validity"} {
		switch strings.TrimSpace(strings.ToLower(stringValue(memory[key]))) {
		case "stale", "stale_or_rejected", "rejected", "superseded", "obsolete", "expired", "diagnostic_only":
			return true
		}
	}
	for _, tag := range stringSliceFromValue(memory["tags"]) {
		switch strings.TrimSpace(strings.ToLower(tag)) {
		case "stale", "superseded", "obsolete", "expired", "diagnostic_only", "provider_retry_noise":
			return true
		}
	}
	return false
}

func memoryLowValue(memory map[string]any) bool {
	confidence := numericValue(memory["confidence"])
	importance := numericValue(memory["importance"])
	if confidence > 0 && confidence < 0.2 {
		return true
	}
	if importance > 0 && importance < 0.2 {
		return true
	}
	return false
}

func cacheAwareMemoryCacheability(memory map[string]any) string {
	switch strings.TrimSpace(stringValue(memory["provenance"])) {
	case "current_run":
		return "volatile_current_run_memory"
	case "historical_run":
		return "historical_reference_memory"
	default:
		return "workspace_reference_memory"
	}
}

func numericValue(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		ret, _ := typed.Float64()
		return ret
	default:
		return 0
	}
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
				"Assessment-only mode is active",
				"Assess the current evidence against the effective Goal and DoD",
				"Return completed or failed with dodAssessment, or a real requiredDecision/requiredApproval",
				"When Goal requirements are not textually identical to DoD items, include dodAssessment.goalCoverage with item-level evidenceRefs",
				"When executionContract.profileContracts declares required acceptanceDimensions or deliverableExpectations, include dodAssessment.acceptanceDimensions and dodAssessment.deliverableCoverage with item-level evidenceRefs or artifactRefs for every required id",
				"dodAssessment.goalCoverage, dodAssessment.acceptanceDimensions, and dodAssessment.deliverableCoverage must be objects shaped as {\"satisfied\":boolean,\"items\":[...]}; never return these coverage fields as bare arrays",
				"Treat runtimeStage=verification or acceptance as strict convergence stages; do not reopen exploration or broad planning in these stages",
				"Before returning blocked_on_decision, reconcile the question with input.currentSoul, input.currentSoulBinding, and the decision/planning/execution/validation bindings in input.executionContext.soulRouting; do not ask for information that those policies explicitly say is not required",
				"When evidence is insufficient but the missing work can be handled by later runtime repair or replan, return failed with dodAssessment.nextActions instead of asking the user to choose the execution target",
				"Do not return executionGraph; do not include shellCommands, patchRequests, mcpToolCalls, memoryWrites, nextAction, or executable work",
			},
		})
	}
	if stateChangingDecisionRequest(request.Input) {
		required := stateChangingDecisionRequiredOutput(request.Input)
		return marshalPromptSection("Current Requested Action", map[string]any{
			"type":           "produce_state_changing_repair_or_failed_assessment",
			"requiredOutput": required,
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
	requirements := []string{
		"Use runtimeCapabilities only",
		"Respect unresolved governance constraints",
		"Treat effective Goal and DoD as hard constraints",
		"Return only allowed workorch.llmExecutionDecision.v1 top-level fields; do not return validationPlan or other unknown advisory fields",
		"When runtimeStageMode is open_judgment, prefer context gathering, plan refinement, and constraint clarification over premature state-changing output",
		"When runtimeStageMode is strict_convergence, produce executable output, failed assessment, or a real blocker; do not continue read-only looping",
		"If the current workspace is empty and the effective Goal/DoD can be satisfied by creating workspace deliverables with available runtime capabilities, continue with bounded state-changing work instead of asking whether initialization is allowed",
	}
	if definitionChangeImpactChanged(request.Input) {
		requirements = append(requirements,
			"definitionChangeImpact.changed is true: before state-changing work, identify additive constraints, still-valid prior constraints, conflicts, and the neutral strategy for artifact/evidence/verification targets",
			"Do not weaken prior Goal or DoD constraints, remove valid acceptance evidence, or relax the requested outcome to make the current run pass",
		)
	}
	return marshalPromptSection("Current Requested Action", map[string]any{
		"type":         "plan_next_step",
		"requirements": requirements,
	})
}

func stateChangingDecisionRequest(input map[string]any) bool {
	if !boolValue(input["stateChangingOutputRequired"]) {
		return false
	}
	objective := strings.TrimSpace(stringValue(input["repairObjective"]))
	if objective == "" ||
		objective == "produce_state_changing_output" ||
		strings.TrimSpace(stringValue(input["stateChangingRecoveryMode"])) == "direct_state_change" ||
		boolValue(input["providerTimeoutStateChangingRetry"]) {
		return true
	}
	return false
}

func stateChangingDecisionRequiredOutput(input map[string]any) map[string]any {
	pkg := recordFromAny(input["repairActionPackage"])
	required := recordFromAny(pkg["requiredOutput"])
	if len(required) > 0 {
		return required
	}
	return map[string]any{
		"acceptedKinds": []string{"patch/apply"},
		"forbiddenKinds": []string{
			"read_only_graph",
			"shell_only_graph",
			"llm_gated_graph",
			"self_inspection_graph",
			"context_rehydration_graph",
		},
		"mustIncludeStateChangingNode": true,
	}
}

func definitionChangeImpactChanged(input map[string]any) bool {
	if impact := recordFromAny(input["definitionChangeImpact"]); boolValue(impact["changed"]) {
		return true
	}
	executionContext := recordFromAny(input["executionContext"])
	if impact := recordFromAny(executionContext["definitionChangeImpact"]); boolValue(impact["changed"]) {
		return true
	}
	executionContextPackage := recordFromAny(input["executionContextPackage"])
	if impact := recordFromAny(executionContextPackage["definitionChangeImpact"]); boolValue(impact["changed"]) {
		return true
	}
	return false
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
