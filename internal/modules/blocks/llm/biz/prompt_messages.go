package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
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

const (
	repairSnapshotPromptContentLimit        = 65536
	validationSnapshotPromptContentLimit    = 32768
	maxRepairSnapshotPromptContentLimit     = 262144
	maxValidationSnapshotPromptContentLimit = 131072
	repairSnapshotPromptItemLimit           = 2
)

type promptSnapshotBudget struct {
	RepairContentLimit     int
	ValidationContentLimit int
	ItemLimit              int
}

func promptSnapshotBudgetForRequest(request llmmodels.ProviderRequest) promptSnapshotBudget {
	budget := defaultPromptSnapshotBudget()
	profile := request.Config.ContextProfile
	availableTokens := profile.MaxInputTokens
	if availableTokens <= 0 {
		availableTokens = profile.ContextWindowTokens
	}
	if profile.ReservedOutputTokens > 0 && availableTokens > profile.ReservedOutputTokens {
		availableTokens -= profile.ReservedOutputTokens
	}
	if availableTokens <= 0 {
		return budget
	}
	safetyRatio := profile.SafetyRatio
	if safetyRatio <= 0 || safetyRatio > 1 {
		safetyRatio = 0.8
	}
	tokensPerByte := profile.EstimatedTokensPerByte
	if tokensPerByte <= 0 {
		tokensPerByte = 0.25
	}
	availableBytes := int(float64(availableTokens) * safetyRatio / tokensPerByte)
	if profile.MaxRequestBytes > 0 {
		availableBytes = min(availableBytes, profile.MaxRequestBytes)
	}
	if profile.BytesHardLimit > 0 {
		availableBytes = min(availableBytes, profile.BytesHardLimit)
	}
	if availableBytes <= 0 {
		return budget
	}
	totalSnapshotBytes := availableBytes / 4
	if totalSnapshotBytes <= 0 {
		return budget
	}
	perRepairSnapshot := totalSnapshotBytes / max(1, budget.ItemLimit)
	budget.RepairContentLimit = min(maxRepairSnapshotPromptContentLimit, max(budget.RepairContentLimit, perRepairSnapshot))
	budget.ValidationContentLimit = min(maxValidationSnapshotPromptContentLimit, max(budget.ValidationContentLimit, perRepairSnapshot/2))
	return budget
}

func defaultPromptSnapshotBudget() promptSnapshotBudget {
	return promptSnapshotBudget{
		RepairContentLimit:     repairSnapshotPromptContentLimit,
		ValidationContentLimit: validationSnapshotPromptContentLimit,
		ItemLimit:              repairSnapshotPromptItemLimit,
	}
}

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
	appendSection("C2.capability-usage-guidance", "user", "normalize_allowed", capabilityUsageGuidancePromptSection(request.Input))
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
	snapshotBudget := promptSnapshotBudgetForRequest(request)
	if hasCurrentActionContract(request.Input) {
		appendSection("C3.current-action-contract", "user", "normalize_allowed", currentActionContractPromptSection(request.Input, snapshotBudget))
	} else {
		appendSection("C3.stage-contract", "user", "normalize_allowed", stageContractPromptSection(request.Input, snapshotBudget))
		appendSection("C3.stage-packet-contract", "user", "normalize_allowed", stagePacketPromptSection(request.Input, snapshotBudget))
		appendSection("C3.repair-action-package", "user", "normalize_allowed", repairActionPackagePromptSection(request.Input, snapshotBudget))
	}
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

func capabilityUsageGuidancePromptSection(input map[string]any) string {
	payload := capabilityUsageGuidancePayload(input)
	if len(payload) == 0 {
		return ""
	}
	return marshalPromptSection("Capability Usage Guidance", payload)
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

func stageContractPromptSection(input map[string]any, snapshotBudget promptSnapshotBudget) string {
	section := map[string]any{}
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
		"outputMode",
		"requestedOutputMode",
		"outputModeSelector",
		"llmOutputPolicy",
		"artifactContinuationId",
		"artifactContinuationContext",
		"artifactOutputContract",
	} {
		copyMapValue(section, key, input[key])
	}
	if soul := compactPromptSoul(recordFromAny(input["currentSoul"]), true); len(soul) > 0 {
		section["currentSoul"] = soul
	}
	if binding := compactPromptSoulBinding(recordFromAny(input["currentSoulBinding"]), false); len(binding) > 0 {
		section["currentSoulBinding"] = binding
	}
	if contract := recordFromAny(input["artifactOutputContract"]); len(contract) > 0 {
		copyMapValue(section, "artifactOutputPhase", contract["phase"])
		copyMapValue(section, "artifactOutputMode", contract["mode"])
	}
	if revision := promptDefinitionRevision(input); revision != "" {
		section["definitionRevision"] = revision
	}
	copyMapValue(section, "definitionChangeImpact", definitionChangeImpactPayload(input))
	if pkg := recordFromAny(input["repairActionPackage"]); len(pkg) > 0 {
		if required := promptRequiredOutputForRepairPackage(pkg, snapshotBudget.RepairContentLimit); required != nil {
			section["requiredOutput"] = required
		}
		copyMapValue(section, "forbiddenActions", pkg["forbiddenActions"])
		copyMapValue(section, "exitCriteria", pkg["exitCriteria"])
	}
	if len(section) == 0 {
		return ""
	}
	return marshalPromptSection("Stage Contract", section)
}

func hasCurrentActionContract(input map[string]any) bool {
	return len(recordFromAny(input["currentActionContract"])) > 0
}

func currentActionContractPromptSection(input map[string]any, snapshotBudget promptSnapshotBudget) string {
	contract := recordFromAny(input["currentActionContract"])
	if len(contract) == 0 {
		return ""
	}
	section := map[string]any{}
	for _, key := range []string{
		"schemaVersion",
		"workspaceId",
		"runId",
		"stepId",
		"runtimeStage",
		"runtimeStageMode",
		"currentRuntimeStage",
		"currentRuntimeStageMode",
		"repairObjective",
		"stateChangingOutputRequired",
		"concreteFailureEvidenceRequired",
		"boundedValidationRequired",
		"evidenceRepairRequired",
		"coverageRepairRequired",
	} {
		copyMapValue(section, key, contract[key])
	}
	for _, key := range []string{
		"nextAction",
		"currentFocus",
		"outputDecisionContract",
		"targetSnapshots",
		"latestRejectedDiagnostic",
	} {
		if value, ok := contract[key]; ok {
			section[key] = budgetProviderValue(value, 0, 2048, 8, 16, 3)
		}
	}
	if required := recordFromAny(contract["requiredOutput"]); len(required) > 0 {
		limit := min(max(snapshotBudget.RepairContentLimit, 4096), 16384)
		section["requiredOutput"] = budgetProviderValue(required, 0, limit, 12, 24, 4)
	}
	section["authorityPolicy"] = "currentActionContract is the only active action and requiredOutput contract for this provider turn; any older stagePacket, repairActionPackage, gapBaseline, convergenceContract, rejected output, or governance history is diagnostic lineage only"
	return marshalPromptSection("Current Action Contract", section)
}

func stagePacketPromptSection(input map[string]any, snapshotBudget promptSnapshotBudget) string {
	section := stagePacketContractPromptPayload(input, snapshotBudget)
	if len(section) == 0 {
		return ""
	}
	return marshalPromptSection("Stage Packet Contract", section)
}

func stagePacketContractPromptPayload(input map[string]any, snapshotBudget promptSnapshotBudget) map[string]any {
	packet := recordFromAny(input["stagePacket"])
	if len(packet) == 0 {
		return nil
	}
	section := map[string]any{}
	for _, key := range []string{
		"schemaVersion",
		"workspaceId",
		"runId",
		"stepId",
		"activeStage",
		"idempotencyKey",
	} {
		copyMapValue(section, key, packet[key])
	}
	for _, key := range []string{
		"definition",
		"activeFocus",
		"nextAction",
	} {
		if value, ok := packet[key]; ok {
			section[key] = budgetProviderValue(value, 0, 2048, 8, 16, 3)
		}
	}
	if value, ok := packet["requiredOutput"]; ok {
		limit := min(max(snapshotBudget.RepairContentLimit, 4096), 16384)
		section["requiredOutput"] = budgetProviderValue(value, 0, limit, 12, 24, 4)
	}
	for _, key := range []string{
		"allowedActions",
		"forbiddenActions",
		"requiredEvidenceKinds",
		"acceptedEvidenceRefs",
		"rejectedEvidenceRefs",
	} {
		if values := stringSliceFromAny(packet[key]); len(values) > 0 {
			section[key] = limitedStringSlice(values, 32)
		}
	}
	if len(section) == 0 {
		return nil
	}
	return section
}

func repairActionPackagePromptSection(input map[string]any, snapshotBudget promptSnapshotBudget) string {
	pkg := recordFromAny(input["repairActionPackage"])
	if len(pkg) == 0 {
		return ""
	}
	section := compactRepairActionPackagePromptPayload(pkg, snapshotBudget)
	if len(section) == 0 {
		return ""
	}
	return marshalPromptSection("Current Repair Action Package", section)
}

func compactRepairActionPackagePromptPayload(pkg map[string]any, snapshotBudget promptSnapshotBudget) map[string]any {
	if len(pkg) == 0 {
		return nil
	}
	section := map[string]any{}
	for _, key := range []string{
		"failureClass",
		"failureSignature",
		"failureTargetScope",
		"failedStepId",
		"failedStepKind",
		"reason",
		"coverageRepairRequired",
	} {
		copyMapValue(section, key, pkg[key])
	}
	for _, key := range []string{
		"currentFailure",
		"lastFailureDiagnostics",
		"outputDecisionContract",
		"structuredFailureSignals",
		"recentRepairAttempts",
		"previousRejectedOutputSummary",
		"lastRejectedOutputContract",
		"forbiddenActions",
		"repairBudget",
		"exitCriteria",
		"strategyReassessment",
	} {
		if value, ok := pkg[key]; ok {
			section[key] = budgetProviderValue(value, 0, 1024, 6, 12, 3)
		}
	}
	if progress := compactCurrentExecutionProgressSignal(recordFromAny(pkg["currentExecutionProgress"])); len(progress) > 0 {
		section["currentExecutionProgress"] = progress
	}
	if targets := compactCoverageTargetSetForPrompt(mapSliceFromAny(pkg["coverageTargetSet"]), 6); len(targets) > 0 {
		section["coverageTargetSet"] = targets
	}
	if targetSnapshots := providerSafeRepairSnapshotsFromValue(pkg["targetSnapshots"]); len(targetSnapshots) > 0 {
		section["targetSnapshots"] = compactRepairSnapshotsForPrompt(targetSnapshots, snapshotBudget.ItemLimit, snapshotBudget.RepairContentLimit)
	}
	if required := promptRequiredOutputForRepairPackage(pkg, snapshotBudget.RepairContentLimit); required != nil {
		section["requiredOutput"] = required
	}
	if manifest := compactWorkspaceFileManifestSignal(pkg["workspaceFileManifest"]); len(manifest) > 0 {
		section["workspaceFileManifestSummary"] = manifest
	}
	if context := compactCoverageWorkItemContextRef(recordFromAny(pkg["coverageWorkItemContext"])); len(context) > 0 {
		section["coverageWorkItemContextRef"] = context
	}
	section["projectionPolicy"] = "bounded C3 repair package; detailed coverage ledger is projected in C4.coverage-work-items when available"
	section["summaryAuthorityPolicy"] = promptAuxiliarySummaryAuthorityPolicy()
	return section
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
				selectedBindings[stage] = compactPromptSoulBinding(binding, false)
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

func compactPromptSoulBinding(binding map[string]any, includePolicyDigest bool) map[string]any {
	if len(binding) == 0 {
		return nil
	}
	out := map[string]any{}
	for _, key := range []string{"source", "soulId", "stage", "bindingStage"} {
		copyMapValue(out, key, binding[key])
	}
	if soul := compactPromptSoul(recordFromAny(binding["soul"]), includePolicyDigest); len(soul) > 0 {
		out["soul"] = soul
	}
	return out
}

func compactPromptSoul(soul map[string]any, includePolicyDigest bool) map[string]any {
	if len(soul) == 0 {
		return nil
	}
	out := map[string]any{}
	for _, key := range []string{
		"id", "name", "description",
		"allowedToolKinds", "allowedSkillRefs", "allowedMCPServerRefs",
		"businessDomains",
	} {
		copyMapValue(out, key, soul[key])
	}
	if includePolicyDigest {
		if digest := compactSoulMarkdownDigest(strings.TrimSpace(stringValue(firstNonNilValue(soul["soulMd"], soul["soulMD"])))); digest != "" {
			out["policyDigest"] = digest
			out["policyDigestSource"] = "compacted_current_stage_policy"
		}
	}
	return out
}

func compactSoulMarkdownDigest(value string) string {
	value = normalizePromptTextForDigest(value)
	if value == "" {
		return ""
	}
	const limit = 1800
	if len([]rune(value)) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit]) + "\n[truncated: full policy text omitted from LLM prompt; use policyDigest as bounded current-stage guidance]"
}

func normalizePromptTextForDigest(value string) string {
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	blank := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if blank {
				continue
			}
			blank = true
			out = append(out, "")
			continue
		}
		blank = false
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
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

type timelineEventSelection struct {
	Item    map[string]any
	Ordinal int
}

func executionSignalPromptEvents(request llmmodels.ProviderRequest) []executionSignalPromptEvent {
	events := make([]executionSignalPromptEvent, 0, 8)
	snapshotBudget := promptSnapshotBudgetForRequest(request)
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
	if progress := currentProgressSignalPayload(request.Input, snapshotBudget); len(progress) > 0 {
		appendEvent("C4.current-progress", "Current Progress Signal", progress)
	}
	if coverageWorkItems := coverageWorkItemSignalPayload(request.Input); len(coverageWorkItems) > 0 {
		appendEvent("C4.coverage-work-items", "Coverage Work Item Signal", coverageWorkItems)
	}
	if acceptance := acceptanceFocusSignalPayload(request.Input); len(acceptance) > 0 {
		appendEvent("C4.acceptance-focus", "Acceptance Slice Focus Signal", acceptance)
	}
	if timeline := processEvidenceTimelineSignalPayload(request.Input); len(timeline) > 0 {
		appendEvent("C4.process-evidence-timeline", "Process Evidence Timeline", timeline)
	}
	if ledger := evidenceLedgerSignalPayload(request.Input); len(ledger) > 0 {
		appendEvent("C4.evidence-ledger", "Evidence Ledger Proof Summary", ledger)
	}
	if impact := definitionChangeImpactSignalPayload(request.Input); len(impact) > 0 {
		appendEvent("C4.definition-change-impact", "Definition Change Impact Signal", impact)
	}
	if validation := validationSignalPayload(request.Input, snapshotBudget); len(validation) > 0 {
		appendEvent("C4.validation-signal", "Validation Signal", validation)
	}
	if governance := governanceDecisionSignalPayload(request.Input); len(governance) > 0 {
		appendEvent("C4.governance-resolution", "Governance Resolution Signal", governance)
	}
	if memory := memorySignalPayload(request.Input); len(memory) > 0 {
		appendEvent("C4.memory-recall", "Memory Recall Signal", memory)
	}
	if snapshots := stagePacketTargetSnapshotsPromptPayload(request.Input, snapshotBudget); len(snapshots) > 0 {
		appendEvent("C5.stage-packet-target-snapshots", "Stage Packet Target Snapshot Metadata", snapshots)
	}
	if len(request.TraceRefs) > 0 {
		appendEvent("C5.trace-refs", "Trace References", request.TraceRefs)
	}
	if request.ContextBudget != nil {
		appendEvent("C5.context-budget", "Context Budget Audit", compactPromptContextBudget(request.ContextBudget))
	}
	if request.Repair != nil {
		appendEvent("C4.repair-json-output", "Structured JSON Repair Signal", request.Repair)
	}
	if len(events) == 0 {
		appendEvent("C4.execution-signals", "Execution Context and Evidence Signals", executionSignalsPayload(request))
	}
	return events
}

func stagePacketTargetSnapshotsPromptPayload(input map[string]any, snapshotBudget promptSnapshotBudget) []map[string]any {
	packet := recordFromAny(input["stagePacket"])
	if len(packet) == 0 {
		return nil
	}
	items := mapSliceFromAny(packet["targetSnapshots"])
	if len(items) == 0 {
		return nil
	}
	limit := max(1, snapshotBudget.ItemLimit)
	if len(items) < limit {
		limit = len(items)
	}
	ret := make([]map[string]any, 0, limit)
	for _, item := range items {
		if len(ret) >= limit {
			break
		}
		next := compactStagePacketTargetSnapshotForPrompt(item)
		if len(next) == 0 {
			continue
		}
		ret = append(ret, next)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func compactStagePacketTargetSnapshotForPrompt(item map[string]any) map[string]any {
	if len(item) == 0 {
		return nil
	}
	out := map[string]any{}
	for _, key := range []string{
		"targetRef",
		"exists",
		"contentHash",
		"contentComplete",
		"safeForFileWrite",
		"safeForExactFileWrite",
		"integrityState",
		"redacted",
		"truncated",
		"isBinary",
		"size",
		"source",
		"contextSource",
		"error",
	} {
		copyMapValue(out, key, item[key])
	}
	if len(out) == 0 {
		return nil
	}
	integrity := strings.TrimSpace(stringValue(out["integrityState"]))
	if integrity == "" {
		integrity = stagePacketSnapshotIntegrityForPrompt(out)
		if integrity != "" {
			out["integrityState"] = integrity
		}
	}
	if integrity != "" {
		out["safeForExactFileWrite"] = integrity == "full"
		if integrity != "full" {
			out["safeForFileWrite"] = false
		}
	}
	return out
}

func stagePacketSnapshotIntegrityForPrompt(item map[string]any) string {
	switch {
	case boolValue(item["redacted"]):
		return "redacted"
	case boolValue(item["truncated"]):
		return "truncated"
	case boolValue(item["contentComplete"]) && boolValue(item["safeForFileWrite"]) && !boolValue(item["isBinary"]):
		return "full"
	default:
		return "partial"
	}
}

func compactPromptContextBudget(budget *llmmodels.ProviderContextBudget) map[string]any {
	if budget == nil {
		return nil
	}
	out := map[string]any{
		"maxBytes":                     budget.MaxBytes,
		"actualBytes":                  budget.ActualBytes,
		"minimumProtectedRequestBytes": budget.MinimumProtectedBytes,
		"maxTokens":                    budget.MaxTokens,
		"estimatedInputTokens":         budget.EstimatedInputTokens,
		"estimatedOutputTokens":        budget.EstimatedOutputTokens,
		"shrinkLevel":                  budget.ShrinkLevel,
		"truncated":                    budget.Truncated,
		"overBudget":                   budget.OverBudget,
		"p0Overflow":                   budget.P0Overflow,
		"contextGateAction":            budget.ContextGateAction,
		"contextGateReason":            budget.ContextGateReason,
		"budgetFailureClass":           budget.BudgetFailureClass,
		"stageBudgetRatio":             budget.StageBudgetRatio,
		"stageBudgetSource":            budget.StageBudgetSource,
		"tracePolicy":                  budget.TracePolicy,
	}
	if len(budget.Rules) > 0 {
		out["rules"] = limitedStringSlice(budget.Rules, 6)
	}
	if len(budget.IncludedSections) > 0 {
		out["includedSectionCount"] = len(budget.IncludedSections)
	}
	if len(budget.NormalizedSections) > 0 {
		out["normalizedSectionCount"] = len(budget.NormalizedSections)
	}
	if len(budget.CompressedSections) > 0 {
		out["compressedSections"] = limitedStringSlice(budget.CompressedSections, 8)
	}
	if len(budget.OmittedSections) > 0 {
		out["omittedSections"] = limitedStringSlice(budget.OmittedSections, 8)
	}
	if len(budget.RedactedPaths) > 0 {
		out["redactedPathCount"] = len(budget.RedactedPaths)
	}
	if len(budget.ProtectedFields) > 0 {
		out["protectedFields"] = limitedStringSlice(budget.ProtectedFields, 12)
	}
	if len(budget.IntegrityWarnings) > 0 {
		out["integrityWarnings"] = limitedStringSlice(budget.IntegrityWarnings, 8)
	}
	if budget.CachePlan != nil {
		out["cachePlan"] = map[string]any{
			"cacheKey":                budget.CachePlan.CacheKey,
			"stablePrefixHash":        budget.CachePlan.StablePrefixHash,
			"workspaceDefinitionHash": budget.CachePlan.WorkspaceDefinitionHash,
			"capabilityContractHash":  budget.CachePlan.CapabilityContractHash,
			"stageBundleHash":         budget.CachePlan.StageBundleHash,
			"volatileTailHash":        budget.CachePlan.VolatileTailHash,
			"largeRefsHash":           budget.CachePlan.LargeRefsHash,
			"cacheablePrefixCount":    len(budget.CachePlan.AdapterHint.CacheablePrefix),
			"volatileSectionCount":    len(budget.CachePlan.AdapterHint.VolatileSections),
			"cacheBreakerCount":       len(budget.CachePlan.CacheBreakers),
		}
	}
	return out
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

func capabilityUsageGuidancePayload(input map[string]any) map[string]any {
	pkg := recordFromAny(input["executionContextPackage"])
	if len(pkg) == 0 {
		return nil
	}
	selected := recordFromAny(pkg["selectedContext"])
	if len(selected) == 0 {
		return nil
	}
	servers := mapSliceFromAny(selected["mcpServers"])
	capabilities := mapSliceFromAny(selected["mcpCapabilities"])
	if len(servers) == 0 && len(capabilities) == 0 {
		return nil
	}
	payload := map[string]any{}
	if summarized := summarizedPromptMCPServers(servers); len(summarized) > 0 {
		payload["selectedMCPServers"] = summarized
	}
	if summarized := summarizedPromptMCPCapabilities(capabilities); len(summarized) > 0 {
		payload["selectedCapabilities"] = summarized
	}
	if policies := capabilityUsagePoliciesFromExecutionContextPackage(pkg); len(policies) > 0 {
		payload["usagePolicies"] = policies
	}
	return payload
}

func capabilityUsagePoliciesFromExecutionContextPackage(pkg map[string]any) []map[string]any {
	contract := recordFromAny(pkg["executionContract"])
	if len(contract) == 0 {
		return nil
	}
	items := mapSliceFromAny(contract["capabilityUsagePolicies"])
	if len(items) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0, min(len(items), 8))
	for _, item := range items {
		next := map[string]any{}
		for _, key := range []string{
			"id",
			"scope",
			"appliesToCategory",
			"evidenceStrength",
			"usageMode",
			"fallbackPolicy",
			"notes",
		} {
			copyMapValue(next, key, item[key])
		}
		for _, key := range []string{"triggerStages", "triggerSignals", "preferredTools", "requiredAfterKinds"} {
			if values := stringSliceFromAny(item[key]); len(values) > 0 {
				next[key] = values
			}
		}
		if len(next) > 0 {
			ret = append(ret, next)
		}
		if len(ret) >= 8 {
			break
		}
	}
	return ret
}

func summarizedPromptMCPServers(items []map[string]any) []map[string]any {
	ret := make([]map[string]any, 0, min(len(items), 8))
	for _, item := range items {
		next := map[string]any{}
		for _, key := range []string{"id", "name", "transport", "command", "defaultApprovalPolicy", "timeoutMillis"} {
			copyMapValue(next, key, item[key])
		}
		if values := stringSliceFromAny(item["allowedTools"]); len(values) > 0 {
			next["allowedTools"] = values
		}
		if len(next) > 0 {
			ret = append(ret, next)
		}
		if len(ret) >= 8 {
			break
		}
	}
	return ret
}

func summarizedPromptMCPCapabilities(items []map[string]any) []map[string]any {
	ret := make([]map[string]any, 0, min(len(items), 16))
	for _, item := range items {
		next := map[string]any{}
		for _, key := range []string{
			"id", "serverId", "kind", "name", "displayName", "category", "usageHint", "riskLevel", "status",
		} {
			copyMapValue(next, key, item[key])
		}
		if values := stringSliceFromAny(item["stageHints"]); len(values) > 0 {
			next["stageHints"] = values
		}
		if values := stringSliceFromAny(item["tags"]); len(values) > 0 {
			next["tags"] = values
		}
		if len(next) > 0 {
			ret = append(ret, next)
		}
		if len(ret) >= 16 {
			break
		}
	}
	return ret
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
	if packet := recordFromAny(input["stagePacket"]); len(packet) > 0 {
		copyMapValue(payload, "stagePacketCurrentFailure", packet["currentFailure"])
		copyMapValue(payload, "stagePacketCurrentGap", packet["currentGap"])
	}
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

func currentProgressSignalPayload(input map[string]any, snapshotBudget promptSnapshotBudget) map[string]any {
	payload := map[string]any{}
	if progress := compactCurrentExecutionProgressSignal(recordFromAny(input["currentExecutionProgress"])); len(progress) > 0 {
		payload["currentExecutionProgress"] = progress
	}
	if digest := recordFromAny(input["currentProgressDigest"]); len(digest) > 0 {
		payload["currentProgressDigest"] = budgetProviderValue(digest, 0, 1024, 6, 12, 3)
	}
	if pkg := recordFromAny(input["executionContextPackage"]); len(pkg) > 0 {
		if digest := recordFromAny(pkg["currentProgressDigest"]); len(digest) > 0 && payload["currentProgressDigest"] == nil {
			payload["packagedCurrentProgressDigest"] = budgetProviderValue(digest, 0, 1024, 6, 12, 3)
		}
		if value, ok := pkg["currentDeliverables"]; ok {
			payload["currentDeliverables"] = budgetProviderValue(value, 0, 768, 6, 12, 2)
		}
		if value, ok := pkg["latestAcceptedResult"]; ok {
			payload["latestAcceptedResult"] = budgetProviderValue(value, 0, 768, 4, 10, 2)
		}
	}
	if repairPkg := recordFromAny(input["repairActionPackage"]); len(repairPkg) > 0 && !hasCurrentActionContract(input) {
		if progress := compactCurrentExecutionProgressSignal(recordFromAny(repairPkg["currentExecutionProgress"])); len(progress) > 0 && payload["currentExecutionProgress"] == nil {
			payload["packagedCurrentExecutionProgress"] = progress
		}
		if targets := compactCoverageTargetSetForPrompt(mapSliceFromAny(repairPkg["coverageTargetSet"]), 6); len(targets) > 0 {
			payload["coverageTargetSet"] = targets
		}
		if required := promptRequiredOutputForRepairPackage(repairPkg, snapshotBudget.RepairContentLimit); required != nil {
			payload["requiredOutput"] = budgetProviderValue(required, 0, 1024, 6, 12, 3)
		}
		if value, ok := repairPkg["exitCriteria"]; ok {
			payload["exitCriteria"] = budgetProviderValue(value, 0, 768, 6, 12, 2)
		}
	}
	if len(payload) > 0 {
		payload["summaryAuthorityPolicy"] = promptAuxiliarySummaryAuthorityPolicy()
	}
	return payload
}

func coverageWorkItemSignalPayload(input map[string]any) map[string]any {
	context := recordFromAny(input["coverageWorkItemContext"])
	if len(context) == 0 {
		context = recordFromAny(recordFromAny(input["executionContext"])["coverageWorkItemContext"])
	}
	if len(context) == 0 {
		context = recordFromAny(recordFromAny(input["executionContextPackage"])["coverageWorkItemContext"])
	}
	if len(context) == 0 {
		context = recordFromAny(recordFromAny(input["repairActionPackage"])["coverageWorkItemContext"])
	}
	if len(context) == 0 {
		return nil
	}
	payload := compactCoverageWorkItemSignalContext(context)
	if len(payload) == 0 {
		return nil
	}
	payload["projectionPolicy"] = "dynamic C4 context; use currentFocus and unresolvedItems as convergence targets, do not rewrite formal Goal or DoD; missing evidence binding means the runtime ledger still needs accepted evidence refs, not that matching evidence is absent from history or memory"
	payload["ledgerAuthorityPolicy"] = "coverage_ledger_authoritative_for_current_gap_status_evidence_refs_and_completion"
	return payload
}

func evidenceLedgerSignalPayload(input map[string]any) map[string]any {
	ledger := recordFromAny(input["evidenceLedger"])
	if len(ledger) == 0 {
		ledger = recordFromAny(recordFromAny(input["executionContextPackage"])["evidenceLedger"])
	}
	if len(ledger) == 0 {
		return nil
	}
	payload, _ := budgetProviderValue(ledger, 0, 1024, 12, 18, 4).(map[string]any)
	if len(payload) == 0 {
		return nil
	}
	payload["evidenceAuthorityPolicy"] = "accepted_or_rejected_status_and_target_aligned_observed_claims_are_runtime_authoritative; raw payloads require explicit payloadRef/sourceRef reads"
	return payload
}

func acceptanceFocusSignalPayload(input map[string]any) map[string]any {
	if len(recordFromAny(recordFromAny(input["stagePacket"])["activeFocus"])) > 0 {
		return nil
	}
	acceptance := recordFromAny(input["acceptance"])
	if len(acceptance) == 0 {
		acceptance = recordFromAny(recordFromAny(input["executionContext"])["acceptance"])
	}
	if len(acceptance) == 0 {
		acceptance = recordFromAny(recordFromAny(input["executionContextPackage"])["acceptance"])
	}
	if len(acceptance) == 0 {
		return nil
	}
	payload := map[string]any{}
	for _, key := range []string{"activeSliceId", "projectionPolicy"} {
		if value, ok := acceptance[key]; ok && value != nil {
			payload[key] = value
		}
	}
	if active := recordFromAny(acceptance["activeSlice"]); len(active) > 0 {
		payload["activeSlice"] = budgetProviderValue(active, 0, 1024, 8, 16, 3)
	}
	if focus := recordFromAny(acceptance["currentFocus"]); len(focus) > 0 {
		payload["currentFocus"] = budgetProviderValue(focus, 0, 1024, 8, 16, 3)
	}
	if budget := recordFromAny(acceptance["budget"]); len(budget) > 0 {
		payload["budget"] = budgetProviderValue(budget, 0, 768, 6, 12, 2)
	}
	if len(payload) == 0 {
		return nil
	}
	payload["focusContract"] = "next output must address currentFocus within activeSlice, or return structured cannot_progress with blocker evidence"
	return payload
}

func compactCurrentExecutionProgressSignal(progress map[string]any) map[string]any {
	if len(progress) == 0 {
		return nil
	}
	out := map[string]any{}
	for _, key := range []string{
		"state",
		"stage",
		"currentStepId",
		"currentStepKind",
		"currentStepState",
		"stepTotal",
		"stepCompleted",
		"stepFailed",
		"stepDispatched",
		"blocked",
		"terminal",
		"summary",
		"currentGap",
		"nextRequiredAction",
	} {
		copyMapValue(out, key, progress[key])
	}
	for _, key := range []string{"latestEvidenceRefs", "acceptedEvidenceRefs", "rejectedEvidenceRefs"} {
		if refs := limitedStringSlice(stringSliceFromAny(progress[key]), 8); len(refs) > 0 {
			out[key] = refs
		}
	}
	if targets := compactCoverageTargetSetForPrompt(mapSliceFromAny(progress["unresolvedCoverageTargets"]), 6); len(targets) > 0 {
		out["unresolvedCoverageTargets"] = targets
	}
	if len(out) == 0 {
		value, _ := budgetProviderValue(progress, 0, 768, 6, 12, 2).(map[string]any)
		return value
	}
	return out
}

func compactCoverageTargetSetForPrompt(targets []map[string]any, limit int) []map[string]any {
	if limit <= 0 || len(targets) == 0 {
		return nil
	}
	if len(targets) > limit {
		targets = targets[:limit]
	}
	ret := make([]map[string]any, 0, len(targets))
	for _, target := range targets {
		item := map[string]any{}
		for _, key := range []string{
			"id",
			"coverageTargetToken",
			"source",
			"sourceScope",
			"sourceRequirementId",
			"sourcePath",
			"sourceRequirementAliases",
			"targetText",
			"status",
			"currentGap",
			"requiredEvidenceKinds",
			"missingEvidenceKinds",
			"evidenceRole",
		} {
			if value, ok := target[key]; ok {
				item[key] = budgetProviderValue(value, 0, 512, 4, 8, 2)
			}
		}
		if len(item) > 0 {
			ret = append(ret, item)
		}
	}
	return ret
}

func compactRepairSnapshotsForPrompt(items []any, limit int, contentLimit int) []map[string]any {
	if limit <= 0 || len(items) == 0 {
		return nil
	}
	if len(items) > limit {
		items = items[:limit]
	}
	ret := make([]map[string]any, 0, len(items))
	for _, raw := range items {
		item := recordFromAny(raw)
		if len(item) == 0 {
			continue
		}
		out := map[string]any{}
		for _, key := range []string{
			"targetRef",
			"snapshotRef",
			"payloadRef",
			"contentHash",
			"contentComplete",
			"safeForFileWrite",
			"redacted",
			"truncated",
			"isBinary",
			"baseHash",
			"language",
		} {
			copyMapValue(out, key, item[key])
		}
		if _, ok := out["snapshotRef"]; !ok {
			if ref := promptTargetSnapshotRef(item); ref != "" {
				out["snapshotRef"] = ref
			}
		}
		content, hasContent := item["content"].(string)
		preview, hasPreview := item["contentPreview"].(string)
		visibleContent, contentVisible := promptVisibleSnapshotContent(item, contentLimit)
		if contentVisible {
			out["content"] = visibleContent
			out["contentBytes"] = len([]byte(visibleContent))
			out["contentView"] = "complete"
			out["promptContentComplete"] = true
			out["promptSafeForFileWrite"] = true
			out["promptExtractionRequired"] = false
			out["promptSafetyNote"] = "prompt snapshot body is complete and safe for exact patch/apply with matching baseHash"
		} else if hasContent || hasPreview {
			out["contentOmitted"] = true
			out["contentOmittedReason"] = "target snapshot body is available only through snapshotRef or an explicit targeted read"
			out["promptContentComplete"] = false
			out["promptSafeForFileWrite"] = false
			out["promptExtractionRequired"] = true
			out["promptSafetyNote"] = "prompt snapshot body is omitted by default; acquire exact current content before complete file_write"
			if strings.TrimSpace(content) != "" {
				out["contentBytes"] = len([]byte(strings.TrimSpace(content)))
			} else if strings.TrimSpace(preview) != "" {
				out["contentPreviewBytes"] = len([]byte(strings.TrimSpace(preview)))
			}
		}
		if len(out) > 0 {
			ret = append(ret, out)
		}
	}
	return ret
}

func promptTargetSnapshotRef(item map[string]any) string {
	if len(item) == 0 {
		return ""
	}
	for _, key := range []string{"snapshotRef", "payloadRef", "sourceRef"} {
		if ref := strings.TrimSpace(stringValue(item[key])); ref != "" {
			return ref
		}
	}
	targetRef := strings.TrimSpace(stringValue(item["targetRef"]))
	contentHash := strings.TrimSpace(stringValue(firstNonNilValue(item["contentHash"], item["baseHash"])))
	if targetRef == "" || contentHash == "" {
		return ""
	}
	return "snapshot://target/" + promptSnapshotRefComponent(targetRef) + "#" + promptSnapshotRefComponent(contentHash)
}

func promptSnapshotRefComponent(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		" ", "%20",
		"#", "%23",
		"?", "%3F",
		"\n", "",
		"\r", "",
		"\t", "%09",
	)
	return replacer.Replace(value)
}

func promptRequiredOutputForRepairPackage(pkg map[string]any, contentLimit int) any {
	if len(pkg) == 0 {
		return nil
	}
	required, ok := cloneAny(pkg["requiredOutput"])
	if !ok || required == nil {
		return nil
	}
	requiredMap := recordFromAny(required)
	if len(requiredMap) == 0 {
		return required
	}
	if !repairPackageTargetSnapshotsIncompleteForPrompt(pkg, repairSnapshotPromptItemLimit, contentLimit) {
		return requiredMap
	}
	requiredMap["promptSnapshotContentIncomplete"] = true
	requiredMap["mustNotUsePromptSnapshotForCompleteFileWrite"] = true
	requiredMap["mustAcquireCompleteTargetSnapshotBeforeFullFileWrite"] = true
	requiredMap["directCompleteFileWriteFromPromptSnapshotAllowed"] = false
	if boolValue(requiredMap["mustPatchDirectlyFromProvidedSnapshot"]) {
		requiredMap["mustPatchDirectlyFromProvidedSnapshot"] = false
	}
	if boolValue(requiredMap["mustUseCurrentTargetSnapshot"]) {
		requiredMap["mustUseCurrentTargetSnapshot"] = false
	}
	if boolValue(requiredMap["directCompleteFileWriteFromPromptSnapshot"]) {
		requiredMap["directCompleteFileWriteFromPromptSnapshot"] = false
	}
	return requiredMap
}

func repairPackageTargetSnapshotsIncompleteForPrompt(pkg map[string]any, limit int, contentLimit int) bool {
	allItems := mapSliceFromAny(pkg["targetSnapshots"])
	items := providerSafeRepairSnapshotsFromValue(pkg["targetSnapshots"])
	if len(allItems) == 0 && len(items) == 0 {
		return false
	}
	if len(items) == 0 || len(items) < len(allItems) {
		return true
	}
	if limit > 0 && len(items) > limit {
		return true
	}
	for _, raw := range items {
		if _, ok := promptVisibleSnapshotContent(recordFromAny(raw), contentLimit); !ok {
			return true
		}
	}
	return false
}

func promptVisibleSnapshotContent(item map[string]any, contentLimit int) (string, bool) {
	if len(item) == 0 {
		return "", false
	}
	if boolValue(item["redacted"]) || boolValue(item["truncated"]) || boolValue(item["isBinary"]) {
		return "", false
	}
	if !boolValue(item["contentComplete"]) || !boolValue(item["safeForFileWrite"]) {
		return "", false
	}
	content, ok := item["content"].(string)
	if !ok {
		return "", false
	}
	if strings.TrimSpace(content) == "" {
		return "", false
	}
	if contentLimit > 0 && len([]byte(content)) > contentLimit {
		return "", false
	}
	return content, true
}

func compactWorkspaceFileManifestSignal(value any) map[string]any {
	manifest := recordFromAny(value)
	if len(manifest) == 0 {
		if items := mapSliceFromAny(value); len(items) > 0 {
			return map[string]any{
				"itemCount": len(items),
				"items":     budgetProviderValue(items, 0, 256, 12, 8, 2),
			}
		}
		return nil
	}
	out := map[string]any{}
	for _, key := range []string{"root", "workspaceRoot", "deliverableDirectory", "summary", "hash", "generatedAt"} {
		copyMapValue(out, key, manifest[key])
	}
	for _, key := range []string{"files", "entries", "items"} {
		if items := mapSliceFromAny(manifest[key]); len(items) > 0 {
			out[key+"Count"] = len(items)
			out[key] = budgetProviderValue(items, 0, 256, 12, 8, 2)
			break
		}
	}
	if len(out) == 0 {
		value, _ := budgetProviderValue(manifest, 0, 256, 12, 8, 2).(map[string]any)
		return value
	}
	return out
}

func compactCoverageWorkItemContextRef(context map[string]any) map[string]any {
	if len(context) == 0 {
		return nil
	}
	out := map[string]any{}
	for _, key := range []string{"schemaVersion", "definitionRevision", "coverageBaselineHash", "fullLedgerRef", "summary", "omittedCount", "excludedCount", "historicalCount"} {
		copyMapValue(out, key, context[key])
	}
	if focus := recordFromAny(context["currentFocus"]); len(focus) > 0 {
		out["currentFocus"] = budgetProviderValue(compactProviderCoverageWorkItemFocus(focus), 0, 512, 4, 10, 3)
	}
	return out
}

func compactCoverageWorkItemSignalContext(context map[string]any) map[string]any {
	if len(context) == 0 {
		return nil
	}
	compacted := compactProviderCoverageWorkItemContext(context)
	if len(compacted) == 0 {
		return nil
	}
	payload, _ := budgetProviderValue(compacted, 0, 768, 6, 16, 4).(map[string]any)
	for _, key := range []string{"excludedSources", "historicalItems"} {
		if items := mapSliceFromAny(context[key]); len(items) > 0 {
			payload[key+"Count"] = len(items)
		}
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

func processEvidenceTimelineSignalPayload(input map[string]any) map[string]any {
	var timeline []map[string]any
	if items := mapSliceFromAny(input["processEvidenceTimeline"]); len(items) > 0 {
		timeline = items
	} else if pkg := recordFromAny(input["executionContextPackage"]); len(pkg) > 0 {
		timeline = mapSliceFromAny(pkg["processEvidenceTimeline"])
	}
	return compactProcessEvidenceTimelineSignal(timeline)
}

func compactProcessEvidenceTimelineSignal(timeline []map[string]any) map[string]any {
	if len(timeline) == 0 {
		return nil
	}
	selected, omittedMiddle, omittedRefs := compactTimelineEventWindow(timeline)
	events := make([]map[string]any, 0, len(selected))
	missingAnchorCount := 0
	for _, selectedEvent := range selected {
		if !timelineEventHasAnchorRef(selectedEvent.Item) {
			missingAnchorCount++
		}
		if compacted := compactProcessEvidenceTimelineEvent(selectedEvent.Item, selectedEvent.Ordinal); len(compacted) > 0 {
			events = append(events, compacted)
		}
	}
	if len(events) == 0 {
		return nil
	}
	out := map[string]any{
		"schemaVersion":          "workorch.c4.processEvidenceTimeline.v1",
		"totalEvents":            len(timeline),
		"omittedMiddleCount":     omittedMiddle,
		"retentionPolicy":        "preserve first event and most recent events; compact bulky outputs into diagnostic summaries; retain omitted refs for audit",
		"summaryAuthorityPolicy": promptAuxiliarySummaryAuthorityPolicy(),
		"sourceRefValidation": map[string]any{
			"policy":             "sourceRef_or_payloadRef_or_contentHash_required_per_event",
			"checkedEvents":      len(selected),
			"missingAnchorCount": missingAnchorCount,
		},
		"events": events,
	}
	if len(omittedRefs) > 0 {
		out["omittedRefs"] = omittedRefs
	}
	if omittedMiddle > len(omittedRefs) {
		out["omittedRefPolicy"] = "omitted refs are capped to keep C4 audit bounded"
	}
	return out
}

func compactTimelineEventWindow(timeline []map[string]any) ([]timelineEventSelection, int, []string) {
	const maxEvents = 12
	const headEvents = 1
	if len(timeline) <= maxEvents {
		selected := make([]timelineEventSelection, 0, len(timeline))
		for idx, item := range timeline {
			selected = append(selected, timelineEventSelection{Item: item, Ordinal: idx + 1})
		}
		return selected, 0, nil
	}
	tailEvents := maxEvents - headEvents
	selected := make([]timelineEventSelection, 0, maxEvents)
	for idx := 0; idx < headEvents; idx++ {
		selected = append(selected, timelineEventSelection{Item: timeline[idx], Ordinal: idx + 1})
	}
	tailStart := len(timeline) - tailEvents
	for idx := tailStart; idx < len(timeline); idx++ {
		selected = append(selected, timelineEventSelection{Item: timeline[idx], Ordinal: idx + 1})
	}
	return selected, len(timeline) - maxEvents, omittedTimelineEventRefs(timeline[headEvents:tailStart])
}

func compactProcessEvidenceTimelineEvent(item map[string]any, ordinal int) map[string]any {
	if len(item) == 0 {
		return nil
	}
	out := map[string]any{"eventTapeSequence": ordinal}
	for _, key := range []string{
		"order",
		"sequence",
		"eventType",
		"actionKind",
		"kind",
		"state",
		"stage",
		"stepId",
		"sourceRef",
		"payloadRef",
		"contentHash",
		"artifactRef",
		"targetRef",
		"inputSummary",
		"outputSummary",
		"summary",
		"failureClass",
		"failureReason",
		"failureSignature",
		"evidenceValidity",
		"repairObjective",
		"recoveryAction",
	} {
		copyCompactTimelineValue(out, key, item[key])
	}
	if refs := stringSliceFromAny(item["evidenceRefs"]); len(refs) > 0 {
		out["evidenceRefs"] = limitedStringSlice(refs, 8)
	}
	if !timelineEventHasAnchorRef(item) {
		out["anchorRefStatus"] = "missing"
	}
	if output := recordFromAny(item["output"]); len(output) > 0 {
		if compacted := compactProcessEvidenceOutput(output); len(compacted) > 0 {
			out["output"] = compacted
		}
	}
	return out
}

func timelineEventHasAnchorRef(item map[string]any) bool {
	if len(item) == 0 {
		return false
	}
	for _, key := range []string{"sourceRef", "payloadRef", "contentHash", "artifactRef", "targetRef", "stepId"} {
		if strings.TrimSpace(firstProviderString(item[key])) != "" {
			return true
		}
	}
	if len(stringSliceFromAny(item["evidenceRefs"])) > 0 {
		return true
	}
	return false
}

func omittedTimelineEventRefs(items []map[string]any) []string {
	const maxRefs = 32
	if len(items) == 0 {
		return nil
	}
	refs := make([]string, 0, min(len(items), maxRefs))
	for _, item := range items {
		ref := timelineEventPrimaryRef(item)
		if ref == "" {
			continue
		}
		refs = append(refs, ref)
		if len(refs) >= maxRefs {
			break
		}
	}
	return uniqueStrings(refs)
}

func timelineEventPrimaryRef(item map[string]any) string {
	if len(item) == 0 {
		return ""
	}
	for _, key := range []string{"sourceRef", "payloadRef", "artifactRef", "targetRef", "stepId", "contentHash"} {
		if ref := strings.TrimSpace(firstProviderString(item[key])); ref != "" {
			return ref
		}
	}
	if refs := stringSliceFromAny(item["evidenceRefs"]); len(refs) > 0 {
		return refs[0]
	}
	return ""
}

func copyCompactTimelineValue(target map[string]any, key string, value any) {
	if target == nil || strings.TrimSpace(key) == "" || value == nil {
		return
	}
	switch typed := value.(type) {
	case string:
		if text := strings.TrimSpace(truncateProviderBudgetString(typed, compactTimelineStringLimit(key))); text != "" {
			target[key] = text
		}
	case []string:
		if values := limitedStringSlice(typed, 8); len(values) > 0 {
			target[key] = values
		}
	default:
		if cloned, ok := cloneAny(value); ok {
			target[key] = cloned
		}
	}
}

func compactTimelineStringLimit(key string) int {
	switch strings.TrimSpace(key) {
	case "failureReason", "failureSignature", "inputSummary", "outputSummary", "summary":
		return 512
	case "error", "reason":
		return 768
	default:
		return 2048
	}
}

func compactProcessEvidenceOutput(output map[string]any) map[string]any {
	out := map[string]any{}
	copied := map[string]bool{}
	for _, key := range []string{
		"status",
		"stage",
		"summary",
		"error",
		"reason",
		"failureClass",
		"failureSignature",
	} {
		copyCompactTimelineValue(out, key, output[key])
		if _, ok := out[key]; ok {
			copied[key] = true
		}
	}
	if usage := recordFromAny(output["llmUsage"]); len(usage) > 0 {
		out["llmUsage"] = compactTimelineLLMUsage(usage)
		copied["llmUsage"] = true
	}
	if budget := recordFromAny(output["contextBudget"]); len(budget) > 0 {
		out["contextBudget"] = compactTimelineContextBudget(budget)
		copied["contextBudget"] = true
	}
	if diagnostics := recordFromAny(output["lastFailureDiagnostics"]); len(diagnostics) > 0 {
		out["lastFailureDiagnostics"] = budgetProviderValue(diagnostics, 0, 2048, 8, 16, 3)
		copied["lastFailureDiagnostics"] = true
	}
	if runtimeDiagnostics := recordFromAny(output["runtimeDiagnostics"]); len(runtimeDiagnostics) > 0 {
		out["runtimeDiagnostics"] = budgetProviderValue(runtimeDiagnostics, 0, 1024, 8, 16, 3)
		copied["runtimeDiagnostics"] = true
	}
	if assessment := recordFromAny(output["dodAssessment"]); len(assessment) > 0 {
		out["dodAssessment"] = compactTimelineDoDAssessment(assessment)
		copied["dodAssessment"] = true
	}
	if omitted := omittedTimelineOutputKeys(output, copied); len(omitted) > 0 {
		out["omittedOutputKeys"] = omitted
	}
	return out
}

func omittedTimelineOutputKeys(output map[string]any, copied map[string]bool) []string {
	if len(output) == 0 {
		return nil
	}
	omitted := make([]string, 0, min(len(output), 12))
	for key := range output {
		if copied[key] || strings.TrimSpace(key) == "" {
			continue
		}
		omitted = append(omitted, key)
	}
	if len(omitted) == 0 {
		return nil
	}
	sort.Strings(omitted)
	if len(omitted) > 12 {
		omitted = omitted[:12]
	}
	return omitted
}

func compactTimelineLLMUsage(usage map[string]any) map[string]any {
	out := map[string]any{}
	for _, key := range []string{
		"status",
		"usageSource",
		"inputTokens",
		"outputTokens",
		"totalTokens",
		"requestBytes",
		"responseBytes",
		"durationMs",
		"streaming",
		"failureClass",
	} {
		copyMapValue(out, key, usage[key])
	}
	if cacheUsage := recordFromAny(usage["cacheUsage"]); len(cacheUsage) > 0 {
		out["cacheUsage"] = budgetProviderValue(cacheUsage, 0, 256, 8, 16, 2)
	}
	return out
}

func compactTimelineContextBudget(budget map[string]any) map[string]any {
	out := map[string]any{}
	for _, key := range []string{
		"actualBytes",
		"actualTokens",
		"estimatedInputTokens",
		"estimatedOutputTokens",
		"contextGateAction",
		"contextGateReason",
		"stageBudgetSource",
		"shrinkLevel",
		"overBudget",
		"p0Overflow",
	} {
		copyMapValue(out, key, budget[key])
	}
	if plan := recordFromAny(budget["cachePlan"]); len(plan) > 0 {
		out["cachePlan"] = map[string]any{
			"cacheKey":                plan["cacheKey"],
			"stablePrefixHash":        plan["stablePrefixHash"],
			"workspaceDefinitionHash": plan["workspaceDefinitionHash"],
			"capabilityContractHash":  plan["capabilityContractHash"],
			"stageBundleHash":         plan["stageBundleHash"],
			"volatileTailHash":        plan["volatileTailHash"],
		}
	}
	return out
}

func compactTimelineDoDAssessment(assessment map[string]any) map[string]any {
	out := map[string]any{}
	for _, key := range []string{"satisfied", "summary", "goalOutcome", "verificationSummary", "implementationSummary"} {
		copyCompactTimelineValue(out, key, assessment[key])
	}
	for _, key := range []string{"items", "goalCoverage", "acceptanceDimensions", "deliverableCoverage", "nextActions"} {
		switch typed := assessment[key].(type) {
		case []map[string]any:
			out[key] = len(typed)
		case []any:
			out[key] = len(typed)
		case map[string]any:
			out[key] = budgetProviderValue(typed, 0, 512, 6, 12, 2)
		}
	}
	return out
}

func compactFailedStepInputSignal(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := map[string]any{}
	for _, key := range []string{
		"workspaceId",
		"runId",
		"stepId",
		"kind",
		"schema",
		"runtimeStage",
		"runtimeStageMode",
		"currentRuntimeStage",
		"currentRuntimeStageMode",
		"currentSoulStage",
		"failureClass",
		"failureSignature",
		"repairObjective",
		"assessmentOnly",
		"assessmentOnlyReason",
	} {
		copyMapValue(out, key, input[key])
	}
	if digest := recordFromAny(input["currentProgressDigest"]); len(digest) > 0 {
		out["currentProgressDigest"] = budgetProviderValue(digest, 0, 768, 6, 12, 2)
	}
	if failure := recordFromAny(input["currentFailure"]); len(failure) > 0 {
		out["currentFailure"] = budgetProviderValue(failure, 0, 1024, 6, 12, 2)
	}
	if context := compactCoverageWorkItemContextRef(recordFromAny(input["coverageWorkItemContext"])); len(context) > 0 {
		out["coverageWorkItemContextRef"] = context
	}
	if pkg := compactRepairActionPackagePromptPayload(recordFromAny(input["repairActionPackage"]), defaultPromptSnapshotBudget()); len(pkg) > 0 {
		out["repairActionPackage"] = budgetProviderValue(pkg, 0, 1024, 4, 10, 2)
	}
	if timeline := compactProcessEvidenceTimelineSignal(mapSliceFromAny(input["processEvidenceTimeline"])); len(timeline) > 0 {
		out["processEvidenceTimelineRef"] = map[string]any{
			"totalEvents":        timeline["totalEvents"],
			"omittedMiddleCount": timeline["omittedMiddleCount"],
			"retentionPolicy":    timeline["retentionPolicy"],
		}
	}
	return out
}

func compactFailedStepOutputSignal(output map[string]any) map[string]any {
	if len(output) == 0 {
		return nil
	}
	out := compactProcessEvidenceOutput(output)
	for _, key := range []string{
		"status",
		"stage",
		"summary",
		"reason",
		"failureClass",
		"failureSignature",
		"error",
	} {
		copyCompactTimelineValue(out, key, output[key])
	}
	for _, key := range []string{"resultId", "artifactId", "decisionId", "approvalId"} {
		copyMapValue(out, key, output[key])
	}
	if graph := recordFromAny(output["executionGraph"]); len(graph) > 0 {
		out["executionGraphSummary"] = budgetProviderValue(graph, 0, 384, 4, 8, 2)
	}
	return out
}

func limitedStringSlice(values []string, limit int) []string {
	if limit <= 0 || len(values) == 0 {
		return nil
	}
	count := min(len(values), limit)
	out := make([]string, 0, count)
	for idx := 0; idx < count; idx++ {
		if value := strings.TrimSpace(values[idx]); value != "" {
			out = append(out, truncateProviderBudgetString(value, 512))
		}
	}
	return out
}

func validationSignalPayload(input map[string]any, snapshotBudget promptSnapshotBudget) map[string]any {
	payload := map[string]any{}
	if failedInput := compactFailedStepInputSignal(recordFromAny(input["failedStepInput"])); len(failedInput) > 0 {
		payload["failedStepInput"] = failedInput
	}
	if failedOutput := compactFailedStepOutputSignal(recordFromAny(input["failedStepOutput"])); len(failedOutput) > 0 {
		payload["failedStepOutput"] = failedOutput
	}
	for _, key := range []string{"failedExecutionGraph", "graphEvidence", "runEvidenceSummary", "previousResults"} {
		if value, ok := input[key]; ok {
			payload[key] = budgetProviderValue(value, 0, 512, 6, 12, 2)
		}
	}
	if pkg := recordFromAny(input["repairActionPackage"]); len(pkg) > 0 {
		if targets := providerSafeRepairSnapshotsFromValue(pkg["targetSnapshots"]); len(targets) > 0 {
			payload["targetSnapshots"] = compactRepairSnapshotsForPrompt(targets, snapshotBudget.ItemLimit, snapshotBudget.ValidationContentLimit)
		}
		if manifest := compactWorkspaceFileManifestSignal(pkg["workspaceFileManifest"]); len(manifest) > 0 {
			payload["workspaceFileManifestSummary"] = manifest
		}
		if value, ok := pkg["recentRepairAttempts"]; ok {
			payload["recentRepairAttempts"] = budgetProviderValue(value, 0, 768, 6, 12, 2)
		}
	}
	if pkg := recordFromAny(input["executionContextPackage"]); len(pkg) > 0 {
		if value, ok := pkg["recentFailureSummary"]; ok {
			payload["recentFailureSummary"] = budgetProviderValue(value, 0, 1024, 6, 12, 2)
		}
		if payload["workspaceFileManifestSummary"] == nil {
			if manifest := compactWorkspaceFileManifestSignal(pkg["workspaceFileManifest"]); len(manifest) > 0 {
				payload["workspaceFileManifestSummary"] = manifest
			}
		}
	}
	if len(payload) > 0 {
		payload["projectionPolicy"] = "bounded validation signal; full step inputs/outputs are available by evidence refs, not duplicated in the provider volatile tail"
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
	if len(payload) > 0 {
		payload["summaryAuthorityPolicy"] = promptAuxiliarySummaryAuthorityPolicy()
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
	summary["summaryAuthorityPolicy"] = promptAuxiliarySummaryAuthorityPolicy()
	return summary
}

func promptAuxiliarySummaryAuthorityPolicy() map[string]any {
	return map[string]any{
		"role":               "auxiliary_reference_only",
		"mustNotOverride":    []string{"stagePacket", "coverageWorkItemContext", "requiredOutput", "acceptedEvidenceRefs", "rejectedEvidenceRefs"},
		"ledgerAuthority":    "coverage ledger and StagePacket define current convergence state, required output, evidence gaps and completion",
		"supportRequirement": "summary claims require sourceRef, payloadRef, contentHash or evidenceRefs before use as supporting context",
		"completionPolicy":   "summary-only completion or progress claims cannot mark coverage satisfied",
	}
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
	} {
		switch key {
		case "selectedMemoryIds", "excludedMemoryIds", "retrievedMemoryIds":
			if values := limitedStringSlice(stringSliceFromAny(retrieval[key]), 12); len(values) > 0 {
				out[key] = values
			}
		default:
			copyMapValue(out, key, retrieval[key])
		}
	}
	if reason := strings.TrimSpace(firstProviderString(retrieval["reason"])); reason != "" {
		out["reason"] = truncateProviderBudgetString(reason, 256)
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
	const maxScores = 4
	if len(scores) > maxScores {
		scores = scores[:maxScores]
	}
	ret := make([]map[string]any, 0, len(scores))
	for _, score := range scores {
		item := map[string]any{}
		copyMapValue(item, "id", score["id"])
		copyMapValue(item, "score", score["score"])
		if reason := strings.TrimSpace(firstProviderString(score["reason"])); reason != "" {
			item["reason"] = truncateProviderBudgetString(reason, 256)
		}
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
	const maxMemories = 2
	ret := make([]map[string]any, 0, min(len(memories), maxMemories))
	for _, memory := range memories {
		if !cacheAwareMemoryAllowed(memory, selectedIDs, excludedIDs) {
			continue
		}
		item := map[string]any{}
		for _, key := range []string{"id", "memoryType", "tags", "importance", "confidence", "visibility", "provenance", "vectorScore"} {
			copyMapValue(item, key, memory[key])
		}
		if summary := strings.TrimSpace(firstProviderString(memory["summary"])); summary != "" {
			item["summary"] = truncateProviderBudgetString(summary, 512)
		}
		if content := strings.TrimSpace(firstProviderString(memory["content"])); content != "" {
			item["content"] = truncateProviderBudgetString(content, 512)
		}
		item["cacheability"] = cacheAwareMemoryCacheability(memory)
		ret = append(ret, item)
		if len(ret) >= maxMemories {
			break
		}
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
	snapshotBudget := promptSnapshotBudgetForRequest(request)
	if !isDecisionSchemaRequest(request) {
		if request.Repair != nil {
			return marshalCurrentActionPromptSection(map[string]any{
				"type": "repair_structured_json",
				"requirements": []string{
					"Return one valid JSON object matching " + schemaNameForPrompt(request),
					"Keep the original structured output intent unless schema correction requires a change",
				},
			}, request.Input)
		}
		return marshalCurrentActionPromptSection(map[string]any{
			"type": "generate_structured_json",
			"requirements": []string{
				"Match " + schemaNameForPrompt(request),
				"Use the supplied input instructions and output schema",
				"Do not emit execution-planning fields unless explicitly requested by the output schema",
			},
		}, request.Input)
	}
	if request.Repair != nil {
		requirements := []string{
			"Return one valid JSON object matching workorch.llmExecutionDecision.v1",
			"Keep decision intent unless schema correction requires a change",
		}
		if isAssessmentOnlyDecisionRequest(request) {
			requirements = append(requirements, "Do not include executionGraph or any executable work fields")
		}
		return marshalCurrentActionPromptSection(map[string]any{
			"type":         "repair_invalid_json",
			"requirements": requirements,
		}, request.Input)
	}
	if isAssessmentOnlyDecisionRequest(request) {
		return marshalCurrentActionPromptSection(map[string]any{
			"type": "assess_only",
			"requirements": []string{
				"Assessment-only mode is active",
				"Assess the current evidence against the effective Goal and DoD",
				"Return completed or failed with dodAssessment, or a real requiredDecision/requiredApproval",
				"When Goal requirements are not textually identical to DoD items, include dodAssessment.goalCoverage with item-level evidenceRefs",
				"When executionContract.profileContracts declares required acceptanceDimensions or deliverableExpectations, include dodAssessment.acceptanceDimensions and dodAssessment.deliverableCoverage with item-level evidenceRefs or artifactRefs for every required id",
				"dodAssessment.goalCoverage, dodAssessment.acceptanceDimensions, and dodAssessment.deliverableCoverage must be objects shaped as {\"satisfied\":boolean,\"items\":[...]}; never return these coverage fields as bare arrays",
				"Every dodAssessment.goalCoverage.items entry must include item or requirement, state, and evidenceRefs; when a goalRequirementId is known, copy it exactly",
				"Treat runtimeStage=verification or acceptance as strict convergence stages; do not reopen exploration or broad planning in these stages",
				"Before returning blocked_on_decision, reconcile the question with input.currentSoul, input.currentSoulBinding, and the decision/planning/execution/validation bindings in input.executionContext.soulRouting; do not ask for information that those policies explicitly say is not required",
				"When evidence is insufficient but the missing work can be handled by later runtime repair or replan, return failed with dodAssessment.nextActions instead of asking the user to choose the execution target",
				"Do not return executionGraph; do not include shellCommands, patchRequests, mcpToolCalls, memoryWrites, nextAction, or executable work",
			},
		}, request.Input)
	}
	if concreteValidationDecisionRequest(request.Input) {
		required := concreteValidationDecisionRequiredOutput(request.Input, snapshotBudget)
		return marshalCurrentActionPromptSection(map[string]any{
			"type":           "produce_concrete_validation_evidence_or_failed_assessment",
			"requiredOutput": required,
			"requirements": []string{
				"Concrete validation evidence is required by the current Stage Contract",
				"Use currentActionContract.requiredOutput.acceptedKinds as the authoritative allowed validation evidence contract when present",
				"Return bounded executable validation evidence, failed dodAssessment, or a real requiredDecision/requiredApproval blocker",
				"Do not ask the user to choose planning, execution, repair, or validation stage when runtime capabilities can continue",
				"Do not return patch/apply or other state-changing work unless a separate state-changing repair contract explicitly requires it",
				"Evidence acquisition shell/command or mcp/tool_call nodes must not create, modify, delete, or initialize workspace deliverable files; write generated fixtures or controlled substitutes only outside the deliverable tree, such as a runtime validation scratch directory or equivalent temporary path",
				"Validation evidence must cite concrete runtime observations, controlled substitutes, executable contracts, or dynamic execution observations when requiredOutput declares validationEvidence rules",
				"If an external credential, permission, unavailable provider, or runtime configuration is truly required, declare it through requiredDecision/requiredApproval with structured context",
			},
		}, request.Input)
	}
	if stateChangingDecisionRequest(request.Input) {
		required := stateChangingDecisionRequiredOutput(request.Input, snapshotBudget)
		outputContract := firstNonEmptyRecord(recordFromAny(recordFromAny(request.Input["currentActionContract"])["outputDecisionContract"]), recordFromAny(recordFromAny(request.Input["repairActionPackage"])["outputDecisionContract"]))
		requirements := []string{
			"State-changing output is required by the current Stage Contract",
			"Use currentActionContract.requiredOutput.acceptedKinds as the authoritative allowed state-changing output contract when present",
			"Use currentActionContract.nextAction and outputDecisionContract.requiredMode, allowedOutputs, and forbiddenOutputs as the current authoritative action selection contract when present",
			"Use the effective Goal and DoD already supplied in the request",
			"Return direct state-changing output, failed dodAssessment, or a real requiredDecision/requiredApproval blocker",
			"Do not return run snapshot, observability, self-inspection, context rehydration, or replan-through-LLM graphs",
			"If multiple dependent operations are needed, executionGraph is allowed only when it contains an accepted state-changing path declared by requiredOutput",
			"Do not return read-only, inspection-only, or shell-only executionGraph",
			"When failureTargetRepairContext, patchRepairContext, currentActionContract target snapshots, or repairActionPackage.targetSnapshots already contains a complete current target snapshot, do not return file/read for that target; use the snapshot to produce patch/apply, an accepted declared state-changing shell/mcp operation, failed dodAssessment, or a real blocker",
			"When currentActionContract currentFocus, repairActionPackage.targetSnapshots, failureTargetRepairContext, or repairTargetSet already identifies concrete repair targets, prioritize direct state-changing edits to those targets before auxiliary validation scaffolding, supplemental evidence resources, or unrelated supporting files.",
			"If more current evidence is truly required, return a real blocker or wait for an explicit acquire_concrete_failure_evidence contract; do not satisfy this state-changing contract with file/read-only nodes",
			"Patch content, diffs, and exact text replacements must be complete, concrete, directly applicable, and never copied from redacted or truncated snapshots",
			"Keep state-changing content bounded to small cohesive targets; if the requested content would exceed llmOutputPolicy budgets or become one oversized generated file, use the runtime-selected incrementalArtifact plan/chunk contract or split it across cohesive target resources.",
			"file_write content must be non-empty and cannot represent deletion; when no supported state-changing output can satisfy the request, return failed dodAssessment or a real blocker",
			"When failed evidence includes current target snapshots, update those targets directly with auditable state-change artifacts before rerunning verification",
			"Do not use shell commands to modify workspace files when a patch artifact can represent the change; shell commands should prepare or verify after the accepted state-changing artifact",
			"Do not use evidence acquisition or supplemental validation shell/tool nodes to create, initialize, or mutate workspace deliverable files; generated fixtures and probes that are only for evidence must stay in runtime scratch or temporary paths outside the deliverable tree",
			"Do not return no-op patch/apply content; text_replace oldText and newText must be different substantive literal strings",
			"If an earlier stream was incomplete, continue by producing a complete fresh JSON object rather than inspecting the run again",
		}
		if boolValue(required["mustReturnPatchApplyNode"]) {
			requirements = append(requirements,
				"The previous state-changing repair already failed because it returned a read-only or inspection-only graph.",
				"Return at least one concrete patch/apply node that edits one of the provided current target snapshots directly. Do not return file/read, git/read, shell/command, or mcp/tool_call as the only actionable nodes in this turn.",
			)
		}
		section := map[string]any{
			"type":              "produce_state_changing_repair_or_failed_assessment",
			"primaryNextAction": "produce an accepted state-changing output using the current available target snapshots, or return a traceable failed assessment or real governance blocker",
			"requiredOutput":    required,
			"requirements":      requirements,
		}
		if len(outputContract) > 0 {
			section["outputDecisionContract"] = outputContract
		}
		return marshalCurrentActionPromptSection(section, request.Input)
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
	return marshalCurrentActionPromptSection(map[string]any{
		"type":         "plan_next_step",
		"requirements": requirements,
	}, request.Input)
}

func marshalCurrentActionPromptSection(section map[string]any, input map[string]any) string {
	if section == nil {
		section = map[string]any{}
	}
	var availableSnapshots []map[string]any
	if snapshots := availableTargetSnapshotsPromptSummary(input, defaultPromptSnapshotBudget()); len(snapshots) > 0 {
		availableSnapshots = snapshots
		section["availableTargetSnapshots"] = snapshots
	}
	if recovery := contractViolationRecoveryPromptPayload(input); len(recovery) > 0 {
		recovery = alignContractViolationRecoveryToCurrentAction(recovery, availableSnapshots)
		section["contractViolationRecovery"] = recovery
		requirements := stringSliceFromAny(section["requirements"])
		recoveryRequirement := "Do not repeat contractViolationRecovery.actualOutputSummary; return one valid output that satisfies currentActionContract.requiredOutput when present, otherwise the current requiredOutput."
		if recovery["expectedContractAuthoritative"] == false {
			recoveryRequirement = "contractViolationRecovery.expectedContract is diagnostic lineage only; satisfy currentActionContract.requiredOutput and the current correctionContract."
		}
		requirements = append(requirements,
			"The previous LLM output was rejected by the runtime contract before side effects were executed; use contractViolationRecovery as the precise correction target.",
			recoveryRequirement,
		)
		section["requirements"] = requirements
	}
	if len(availableSnapshots) > 0 {
		requirements := stringSliceFromAny(section["requirements"])
		requirements = append(requirements,
			"Use availableTargetSnapshots as the active patch target set for this turn; older diagnostic target scopes must not add extra targets.",
			"Before returning patch/apply, compare the exact current snapshot content with the proposed oldText/newText and omit any target that would be a no-op.",
		)
		section["requirements"] = uniqueStrings(requirements)
	}
	return marshalPromptSection("Current Requested Action", section)
}

func alignContractViolationRecoveryToCurrentAction(recovery map[string]any, availableSnapshots []map[string]any) map[string]any {
	if len(recovery) == 0 || len(availableSnapshots) == 0 {
		return recovery
	}
	activeRefs := make([]string, 0, len(availableSnapshots))
	activeSet := map[string]bool{}
	for _, snapshot := range availableSnapshots {
		targetRef := strings.TrimSpace(stringValue(snapshot["targetRef"]))
		if targetRef == "" || activeSet[targetRef] {
			continue
		}
		activeSet[targetRef] = true
		activeRefs = append(activeRefs, targetRef)
	}
	if len(activeRefs) == 0 {
		return recovery
	}
	aligned, ok := cloneAny(recovery)
	if !ok {
		return recovery
	}
	ret := recordFromAny(aligned)
	if len(ret) == 0 {
		return recovery
	}
	ret["activeTargetRefs"] = activeRefs
	ret["targetScopePolicy"] = "currentAction.availableTargetSnapshots is the active target set; diagnostic targetScope must not expand this retry"
	if correction := recordFromAny(ret["correctionContract"]); len(correction) > 0 {
		if scope := strings.TrimSpace(stringValue(correction["targetScope"])); scope != "" {
			correction["diagnosticTargetScope"] = scope
		}
		correction["targetScope"] = strings.Join(activeRefs, ",")
		correction["activeTargetRefs"] = activeRefs
		if snapshots := filterPromptSnapshotsByTargetRefs(mapSliceFromAny(correction["targetSnapshots"]), activeSet); len(snapshots) > 0 {
			correction["targetSnapshots"] = snapshots
		} else {
			delete(correction, "targetSnapshots")
		}
		ret["correctionContract"] = correction
	}
	return ret
}

func filterPromptSnapshotsByTargetRefs(items []map[string]any, activeSet map[string]bool) []map[string]any {
	if len(items) == 0 || len(activeSet) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		targetRef := strings.TrimSpace(stringValue(item["targetRef"]))
		if targetRef == "" || !activeSet[targetRef] || seen[targetRef] {
			continue
		}
		seen[targetRef] = true
		ret = append(ret, item)
	}
	return ret
}

func availableTargetSnapshotsPromptSummary(input map[string]any, snapshotBudget promptSnapshotBudget) []map[string]any {
	if len(input) == 0 {
		return nil
	}
	limit := snapshotBudget.ItemLimit
	if limit <= 0 {
		limit = repairSnapshotPromptItemLimit
	}
	seen := map[string]bool{}
	ret := make([]map[string]any, 0, limit)
	add := func(raw map[string]any, source string) {
		if len(raw) == 0 || len(ret) >= limit {
			return
		}
		targetRef := strings.TrimSpace(stringValue(raw["targetRef"]))
		if targetRef == "" {
			return
		}
		contentHash := strings.TrimSpace(stringValue(firstNonNilValue(raw["contentHash"], raw["baseHash"])))
		key := targetRef + "|" + contentHash
		if seen[key] {
			return
		}
		seen[key] = true
		content, contentVisible := promptVisibleSnapshotContent(raw, snapshotBudget.RepairContentLimit)
		promptContentComplete := contentVisible
		promptSafeForFileWrite := contentVisible
		promptExtractionRequired := !contentVisible
		item := map[string]any{
			"targetRef":                targetRef,
			"snapshotRef":              promptTargetSnapshotRef(raw),
			"contentHash":              contentHash,
			"baseHash":                 strings.TrimSpace(stringValue(firstNonNilValue(raw["baseHash"], raw["contentHash"]))),
			"contentComplete":          boolValue(raw["contentComplete"]),
			"safeForFileWrite":         boolValue(raw["safeForFileWrite"]),
			"promptContentComplete":    promptContentComplete,
			"promptSafeForFileWrite":   promptSafeForFileWrite,
			"promptExtractionRequired": promptExtractionRequired,
			"allowedPatchModes":        []string{"unified_diff", "text_replace", "file_write_with_matching_baseHash_when_promptSafeForFileWrite"},
			"source":                   source,
		}
		if sourcePath := strings.TrimSpace(stringValue(firstNonNilValue(raw["sourcePath"], raw["path"], raw["resourcePath"]))); sourcePath != "" {
			item["sourcePath"] = sourcePath
		}
		if promptExtractionRequired {
			item["promptSafetyNote"] = "prompt snapshot is not safe for complete file_write; use exact targeted replacement, unified_diff with visible context, or acquire exact current content first"
		} else {
			item["content"] = content
			item["contentBytes"] = len([]byte(content))
			item["contentView"] = "complete"
			item["promptSafetyNote"] = "prompt snapshot is complete and safe for exact patch/apply with matching baseHash"
		}
		for key, value := range item {
			if promptSnapshotSummaryEmptyValue(value) {
				delete(item, key)
			}
		}
		ret = append(ret, item)
	}
	addItems := func(value any, source string) {
		for _, raw := range providerRepairSnapshotItemsFromValue(value) {
			add(raw, source)
			if len(ret) >= limit {
				return
			}
		}
	}
	addItems(input["failureTargetRepairContext"], "failureTargetRepairContext")
	addItems(input["patchRepairContext"], "patchRepairContext")
	if pkg := recordFromAny(input["repairActionPackage"]); len(pkg) > 0 {
		addItems(pkg["targetSnapshots"], "repairActionPackage.targetSnapshots")
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func promptSnapshotSummaryEmptyValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(typed) == ""
	case []string:
		return len(typed) == 0
	default:
		return false
	}
}

func contractViolationRecoveryPromptPayload(input map[string]any) map[string]any {
	recovery := recordFromAny(input["contractViolationRecovery"])
	if len(recovery) == 0 {
		return nil
	}
	value := budgetProviderValue(recovery, 0, 2048, 8, 16, 3)
	return recordFromAny(value)
}

func concreteValidationDecisionRequest(input map[string]any) bool {
	if strings.TrimSpace(stringValue(input["repairObjective"])) == "produce_concrete_validation_evidence" ||
		boolValue(input["boundedValidationRequired"]) ||
		boolValue(input["evidenceRepairRequired"]) {
		return true
	}
	if required := recordFromAny(recordFromAny(input["currentActionContract"])["requiredOutput"]); boolValue(required["mustProduceConcreteValidationEvidence"]) {
		return true
	}
	required := recordFromAny(recordFromAny(input["repairActionPackage"])["requiredOutput"])
	return boolValue(required["mustProduceConcreteValidationEvidence"])
}

func concreteValidationDecisionRequiredOutput(input map[string]any, snapshotBudget promptSnapshotBudget) map[string]any {
	if required := recordFromAny(recordFromAny(input["currentActionContract"])["requiredOutput"]); len(required) > 0 {
		return required
	}
	pkg := recordFromAny(input["repairActionPackage"])
	required := recordFromAny(promptRequiredOutputForRepairPackage(pkg, snapshotBudget.ValidationContentLimit))
	if len(required) > 0 {
		return required
	}
	return map[string]any{
		"mustIncludeStateChangingNode":             false,
		"mustProduceConcreteValidationEvidence":    true,
		"mustPreferExecutableBehaviorValidation":   true,
		"mustDeclareValidationEvidenceMode":        true,
		"mustDeclareValidationEvidenceObservation": true,
		"acceptedKinds":                            []string{"shell/command", "mcp/tool_call"},
		"forbiddenKinds":                           []string{"patch/apply", "read_only_graph", "llm_gated_graph", "self_inspection_graph", "context_rehydration_graph", "run_snapshot_graph", "observability_only_graph"},
	}
}

func stateChangingDecisionRequest(input map[string]any) bool {
	current := recordFromAny(input["currentActionContract"])
	if !boolValue(input["stateChangingOutputRequired"]) && !boolValue(current["stateChangingOutputRequired"]) {
		return false
	}
	objective := firstNonEmptyPromptString(stringValue(input["repairObjective"]), stringValue(current["repairObjective"]))
	if objective == "" ||
		objective == "produce_state_changing_output" ||
		strings.TrimSpace(stringValue(input["stateChangingRecoveryMode"])) == "direct_state_change" ||
		boolValue(input["providerTimeoutStateChangingRetry"]) {
		return true
	}
	return false
}

func stateChangingDecisionRequiredOutput(input map[string]any, snapshotBudget promptSnapshotBudget) map[string]any {
	if required := recordFromAny(recordFromAny(input["currentActionContract"])["requiredOutput"]); len(required) > 0 {
		return required
	}
	pkg := recordFromAny(input["repairActionPackage"])
	required := recordFromAny(promptRequiredOutputForRepairPackage(pkg, snapshotBudget.RepairContentLimit))
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

func firstNonEmptyRecord(values ...map[string]any) map[string]any {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
	}
	return nil
}

func firstNonEmptyPromptString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
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
