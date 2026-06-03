package appservice

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	soulmodel "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/doditems"
	"github.com/muidea/workorch/internal/pkg/events"
)

const workspaceSetupDraftSchemaName = "workorch.workspaceSetupDraft.v1"

type WorkspaceSetupDraftWorkspace struct {
	Title                string `json:"title,omitempty"`
	Description          string `json:"description,omitempty"`
	DeliverableDirectory string `json:"deliverableDirectory,omitempty"`
}

type WorkspaceSetupDraftMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type WorkspaceSetupDraft struct {
	GoalText             string   `json:"goalText,omitempty"`
	DoDItems             []string `json:"doDItems,omitempty"`
	Assumptions          []string `json:"assumptions,omitempty"`
	ClarifyingQuestions  []string `json:"clarifyingQuestions,omitempty"`
	Warnings             []string `json:"warnings,omitempty"`
	Confidence           string   `json:"confidence,omitempty"`
	NeedsClarification   bool     `json:"needsClarification"`
	GenerationSummary    string   `json:"generationSummary,omitempty"`
	TemplateContextUsed  bool     `json:"templateContextUsed"`
	PreviousDraftApplied bool     `json:"previousDraftApplied"`
}

type WorkspaceSetupDraftGenerationCommand struct {
	TemplateID     string                       `json:"templateId,omitempty"`
	Workspace      WorkspaceSetupDraftWorkspace `json:"workspace,omitempty"`
	Messages       []WorkspaceSetupDraftMessage `json:"messages,omitempty"`
	CurrentDraft   WorkspaceSetupDraft          `json:"currentDraft,omitempty"`
	DialogLanguage string                       `json:"dialogLanguage,omitempty"`
	Regenerate     bool                         `json:"regenerate,omitempty"`
}

type WorkspaceSetupDraftGenerationRequest struct {
	Template       WorkspaceTemplate            `json:"template,omitempty"`
	Souls          []WorkspaceSetupDraftSoul    `json:"souls,omitempty"`
	Workspace      WorkspaceSetupDraftWorkspace `json:"workspace,omitempty"`
	Messages       []WorkspaceSetupDraftMessage `json:"messages,omitempty"`
	CurrentDraft   WorkspaceSetupDraft          `json:"currentDraft,omitempty"`
	DialogLanguage string                       `json:"dialogLanguage,omitempty"`
	Regenerate     bool                         `json:"regenerate,omitempty"`
}

type WorkspaceSetupDraftSoul struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	SoulMd      string `json:"soulMd,omitempty"`
}

type WorkspaceSetupDraftGenerator interface {
	GenerateWorkspaceSetupDraft(context.Context, WorkspaceSetupDraftGenerationRequest) (WorkspaceSetupDraft, error)
}

type defaultWorkspaceSetupDraftGenerator struct {
	eventHub events.Hub
}

func GenerateWorkspaceSetupDraftWithBackend(ctx context.Context, backend string, dataRoot string, cmd WorkspaceSetupDraftGenerationCommand) (WorkspaceSetupDraft, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return WorkspaceSetupDraft{}, err
	}
	return GenerateWorkspaceSetupDraftWithSession(ctx, session, cmd, nil)
}

func GenerateWorkspaceSetupDraftWithSession(ctx context.Context, session *RuntimeSession, cmd WorkspaceSetupDraftGenerationCommand, generator WorkspaceSetupDraftGenerator) (ret WorkspaceSetupDraft, err error) {
	if generator == nil {
		exports, err := session.Exports()
		if err != nil {
			return WorkspaceSetupDraft{}, err
		}
		generator = defaultWorkspaceSetupDraftGenerator{eventHub: exports.Infrastructure.EventHub}
	}
	request, err := BuildWorkspaceSetupDraftGenerationRequest(ctx, session, cmd)
	if err != nil {
		return WorkspaceSetupDraft{}, err
	}
	defer session.CloseOnReturn(ctx, &err)
	draft, err := generator.GenerateWorkspaceSetupDraft(ctx, request)
	if err != nil {
		return WorkspaceSetupDraft{}, err
	}
	return MergeWorkspaceSetupDraft(request.CurrentDraft, draft), nil
}

func BuildWorkspaceSetupDraftGenerationRequest(ctx context.Context, session *RuntimeSession, cmd WorkspaceSetupDraftGenerationCommand) (WorkspaceSetupDraftGenerationRequest, error) {
	if session == nil {
		return WorkspaceSetupDraftGenerationRequest{}, common.ErrInvalidArgument
	}
	exports, err := session.Exports()
	if err != nil {
		return WorkspaceSetupDraftGenerationRequest{}, err
	}
	workspace := WorkspaceSetupDraftWorkspace{
		Title:                strings.TrimSpace(cmd.Workspace.Title),
		Description:          strings.TrimSpace(cmd.Workspace.Description),
		DeliverableDirectory: strings.TrimSpace(cmd.Workspace.DeliverableDirectory),
	}
	messages := normalizeWorkspaceSetupDraftMessages(cmd.Messages)
	if workspace.Title == "" && len(messages) == 0 && strings.TrimSpace(cmd.CurrentDraft.GoalText) == "" {
		return WorkspaceSetupDraftGenerationRequest{}, common.ErrInvalidArgument
	}
	templateID := strings.TrimSpace(cmd.TemplateID)
	var template WorkspaceTemplate
	if templateID != "" {
		if exports.Repositories.WorkspaceTemplates == nil {
			return WorkspaceSetupDraftGenerationRequest{}, common.ErrInvalidArgument
		}
		item, err := exports.Repositories.WorkspaceTemplates.Get(ctx, templateID)
		if err != nil {
			return WorkspaceSetupDraftGenerationRequest{}, err
		}
		template = workspaceTemplateFromRepository(item)
	}
	souls := []WorkspaceSetupDraftSoul{}
	if exports.Infrastructure.EventHub != nil {
		ids := map[string]struct{}{}
		if template.DefaultSoulID != "" {
			ids[template.DefaultSoulID] = struct{}{}
		}
		for _, soulID := range template.SoulBindings {
			soulID = strings.TrimSpace(soulID)
			if soulID != "" {
				ids[soulID] = struct{}{}
			}
		}
		for soulID := range ids {
			item, err := readSoulByOwnerCommand(ctx, exports.Infrastructure.EventHub, common.SoulID(soulID))
			if err == nil {
				souls = append(souls, workspaceSetupDraftSoulFromRepository(item))
			}
		}
	}
	return WorkspaceSetupDraftGenerationRequest{
		Template:       template,
		Souls:          souls,
		Workspace:      workspace,
		Messages:       messages,
		CurrentDraft:   NormalizeWorkspaceSetupDraft(cmd.CurrentDraft),
		DialogLanguage: strings.TrimSpace(cmd.DialogLanguage),
		Regenerate:     cmd.Regenerate,
	}, nil
}

func NormalizeWorkspaceSetupDraft(draft WorkspaceSetupDraft) WorkspaceSetupDraft {
	draft.GoalText = strings.TrimSpace(draft.GoalText)
	draft.DoDItems = doditems.Normalize(draft.DoDItems)
	draft.Assumptions = compactDraftStrings(draft.Assumptions)
	draft.ClarifyingQuestions = compactDraftStrings(draft.ClarifyingQuestions)
	draft.Warnings = compactDraftStrings(draft.Warnings)
	draft.Confidence = normalizedDraftConfidence(draft.Confidence)
	draft.GenerationSummary = strings.TrimSpace(draft.GenerationSummary)
	if (draft.GoalText == "" || len(draft.DoDItems) == 0) && len(draft.ClarifyingQuestions) == 0 && len(draft.Warnings) == 0 {
		draft.ClarifyingQuestions = []string{"请补充目标范围、验收条件或关键约束，以便生成完整的 Goal 和 DoD 草案。"}
	}
	draft.NeedsClarification = draft.GoalText == "" || len(draft.DoDItems) == 0 || len(draft.ClarifyingQuestions) > 0
	return draft
}

func MergeWorkspaceSetupDraft(base WorkspaceSetupDraft, update WorkspaceSetupDraft) WorkspaceSetupDraft {
	base = NormalizeWorkspaceSetupDraft(base)
	updateHasGoal := strings.TrimSpace(update.GoalText) != ""
	updateHasDoD := len(update.DoDItems) > 0
	updateHasAssumptions := len(update.Assumptions) > 0
	updateHasWarnings := len(update.Warnings) > 0
	updateHasSummary := strings.TrimSpace(update.GenerationSummary) != ""
	update = NormalizeWorkspaceSetupDraft(update)
	merged := update
	if !updateHasGoal {
		merged.GoalText = base.GoalText
	}
	if !updateHasDoD {
		merged.DoDItems = append([]string(nil), base.DoDItems...)
	}
	if !updateHasAssumptions {
		merged.Assumptions = append([]string(nil), base.Assumptions...)
	}
	if !updateHasWarnings {
		merged.Warnings = append([]string(nil), base.Warnings...)
	}
	if !updateHasSummary {
		merged.GenerationSummary = base.GenerationSummary
	}
	merged.TemplateContextUsed = update.TemplateContextUsed || base.TemplateContextUsed
	merged.PreviousDraftApplied = update.PreviousDraftApplied || base.PreviousDraftApplied
	return NormalizeWorkspaceSetupDraft(merged)
}

func WorkspaceSetupDraftRevisionBaseline(draft WorkspaceSetupDraft) WorkspaceSetupDraft {
	draft = NormalizeWorkspaceSetupDraft(draft)
	draft.ClarifyingQuestions = nil
	return NormalizeWorkspaceSetupDraft(draft)
}

func (g defaultWorkspaceSetupDraftGenerator) GenerateWorkspaceSetupDraft(ctx context.Context, request WorkspaceSetupDraftGenerationRequest) (WorkspaceSetupDraft, error) {
	if g.eventHub == nil {
		return WorkspaceSetupDraft{}, common.ErrInvalidArgument
	}
	payload := map[string]any{
		"instructions": workspaceSetupDraftInstructions(),
		"request":      request,
		"outputSchema": workspaceSetupDraftOutputSchema(),
	}
	err := g.eventHub.Publish(ctx, events.Envelope{
		Topic:        events.TopicLLMCommandCompleteProvider,
		SourceModule: "application/workorchd/appservice",
		SubjectType:  "workspace_setup_draft",
		Payload: map[string]any{
			events.PayloadKeyProviderRequest: llmmodels.ProviderRequest{
				Kind:           "workspace/setup_draft",
				Schema:         workspaceSetupDraftSchemaName,
				DialogLanguage: request.DialogLanguage,
				Input:          payload,
				TraceRefs: map[string]string{
					"templateId": request.Template.ID,
				},
			},
		},
	})
	if response, ok := events.ResponseEnvelope(err); ok {
		completion, ok := response.Payload[events.PayloadKeyProviderCompletion].(llmmodels.ProviderCompletionResult)
		if !ok {
			return WorkspaceSetupDraft{}, common.ErrInvalidArgument
		}
		return ParseWorkspaceSetupDraftCompletion(completion.Payload)
	}
	if err != nil {
		return WorkspaceSetupDraft{}, err
	}
	return WorkspaceSetupDraft{}, common.ErrNotFound
}

func ParseWorkspaceSetupDraftCompletion(payload []byte) (WorkspaceSetupDraft, error) {
	var envelope struct {
		Draft *WorkspaceSetupDraft `json:"draft"`
		WorkspaceSetupDraft
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return WorkspaceSetupDraft{}, fmt.Errorf("%w: invalid workspace setup draft response: %v", common.ErrInvalidArgument, err)
	}
	if envelope.Draft != nil {
		if hasParsedWorkspaceSetupDraftContent(*envelope.Draft) {
			return NormalizeWorkspaceSetupDraft(*envelope.Draft), nil
		}
	}
	if hasParsedWorkspaceSetupDraftContent(envelope.WorkspaceSetupDraft) {
		return NormalizeWorkspaceSetupDraft(envelope.WorkspaceSetupDraft), nil
	}
	return parseWorkspaceSetupDraftCompletionAliases(payload)
}

func workspaceSetupDraftInstructions() []string {
	return []string{
		"Generate or revise a workspace setup draft from a multi-turn conversation.",
		"Return JSON only and match the provided output schema.",
		"Use the complete message history and the currentDraft as the revision baseline.",
		"When the latest user message adds or changes requirements, update the existing goal and DoD dynamically instead of replacing the draft from scratch.",
		"Treat this as workspace creation-time drafting, not runtime execution. Produce a usable Goal and DoD draft once the user's natural-language intent is clear enough to create the workspace.",
		"Keep the goal as one clear executable objective, not a process checklist.",
		"Keep DoD items as verifiable acceptance facts. Do not output headings, section labels, parent-only bullets, or empty items as DoD items.",
		"Use the selected workspace template and soul context as guidance, but do not invent details that the user has not provided.",
		"Do not require users to manually translate natural-language requirements into internal tool parameters, IDs, enum values, or implementation-specific field names during draft setup.",
		"If template or soul context describes runtime decisions, approvals, or parameter completion, keep those as Goal/DoD behavior instead of treating them as blockers for creating the draft.",
		"If a user-provided value is ambiguous but the workspace intent is still clear, record the best interpretation as an assumption or warning and leave clarifyingQuestions empty.",
		"Use clarifyingQuestions only when the workspace Goal or DoD cannot be formed from the available conversation and template context.",
		"Preserve user-confirmed information from the previous draft and revise it with new messages.",
	}
}

func workspaceSetupDraftOutputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"goalText":            map[string]any{"type": "string"},
			"doDItems":            map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"assumptions":         map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"clarifyingQuestions": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"warnings":            map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"confidence":          map[string]any{"type": "string", "enum": []string{"low", "medium", "high"}},
			"generationSummary":   map[string]any{"type": "string"},
		},
	}
}

func parseWorkspaceSetupDraftCompletionAliases(payload []byte) (WorkspaceSetupDraft, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(payload, &raw); err != nil {
		return WorkspaceSetupDraft{}, fmt.Errorf("%w: invalid workspace setup draft response: %v", common.ErrInvalidArgument, err)
	}
	for _, key := range []string{"draft", "workspaceSetupDraft", "result", "data"} {
		if nestedPayload, ok := raw[key]; ok {
			var nested map[string]json.RawMessage
			if err := json.Unmarshal(nestedPayload, &nested); err == nil {
				draft := workspaceSetupDraftFromRawObject(nested)
				if hasParsedWorkspaceSetupDraftContent(draft) {
					return NormalizeWorkspaceSetupDraft(draft), nil
				}
			}
		}
	}
	draft := workspaceSetupDraftFromRawObject(raw)
	return NormalizeWorkspaceSetupDraft(draft), nil
}

func workspaceSetupDraftFromRawObject(raw map[string]json.RawMessage) WorkspaceSetupDraft {
	return WorkspaceSetupDraft{
		GoalText:            firstRawString(raw, "goalText", "goal", "objective", "target"),
		DoDItems:            firstRawStringSlice(raw, "doDItems", "dodItems", "doD", "dod", "definitionOfDone", "acceptanceCriteria", "acceptanceItems"),
		Assumptions:         firstRawStringSlice(raw, "assumptions", "assumption"),
		ClarifyingQuestions: firstRawStringSlice(raw, "clarifyingQuestions", "clarifying_questions", "clarificationQuestions", "questions"),
		Warnings:            firstRawStringSlice(raw, "warnings", "warning", "risks"),
		Confidence:          firstRawString(raw, "confidence"),
		GenerationSummary:   firstRawString(raw, "generationSummary", "generation_summary", "summary"),
	}
}

func hasParsedWorkspaceSetupDraftContent(draft WorkspaceSetupDraft) bool {
	return strings.TrimSpace(draft.GoalText) != "" ||
		len(draft.DoDItems) > 0 ||
		len(draft.Assumptions) > 0 ||
		len(draft.ClarifyingQuestions) > 0 ||
		len(draft.Warnings) > 0 ||
		strings.TrimSpace(draft.GenerationSummary) != ""
}

func firstRawString(raw map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		payload, ok := raw[key]
		if !ok {
			continue
		}
		var value string
		if err := json.Unmarshal(payload, &value); err == nil && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstRawStringSlice(raw map[string]json.RawMessage, keys ...string) []string {
	for _, key := range keys {
		payload, ok := raw[key]
		if !ok {
			continue
		}
		values := rawStringSlice(payload)
		if len(values) > 0 {
			return values
		}
	}
	return nil
}

func rawStringSlice(payload json.RawMessage) []string {
	var values []string
	if err := json.Unmarshal(payload, &values); err == nil {
		return compactDraftStrings(values)
	}
	var value string
	if err := json.Unmarshal(payload, &value); err == nil {
		return compactDraftStrings(strings.Split(value, "\n"))
	}
	var objects []map[string]any
	if err := json.Unmarshal(payload, &objects); err == nil {
		ret := make([]string, 0, len(objects))
		for _, object := range objects {
			for _, key := range []string{"text", "content", "item", "description", "title"} {
				if text, ok := object[key].(string); ok && strings.TrimSpace(text) != "" {
					ret = append(ret, text)
					break
				}
			}
		}
		return compactDraftStrings(ret)
	}
	return nil
}

func normalizeWorkspaceSetupDraftMessages(messages []WorkspaceSetupDraftMessage) []WorkspaceSetupDraftMessage {
	ret := make([]WorkspaceSetupDraftMessage, 0, len(messages))
	for _, item := range messages {
		role := strings.ToLower(strings.TrimSpace(item.Role))
		switch role {
		case "assistant", "system":
		default:
			role = "user"
		}
		content := strings.TrimSpace(item.Content)
		if content == "" {
			continue
		}
		ret = append(ret, WorkspaceSetupDraftMessage{Role: role, Content: content})
	}
	return ret
}

func workspaceSetupDraftSoulFromRepository(item *soulmodel.Soul) WorkspaceSetupDraftSoul {
	if item == nil {
		return WorkspaceSetupDraftSoul{}
	}
	return WorkspaceSetupDraftSoul{
		ID:          string(item.ID),
		Name:        item.Name,
		Description: item.Description,
		SoulMd:      item.SoulMD,
	}
}

func normalizedDraftConfidence(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "low", "medium", "high":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "medium"
	}
}

func compactDraftStrings(values []string) []string {
	seen := map[string]struct{}{}
	ret := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		ret = append(ret, value)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}
