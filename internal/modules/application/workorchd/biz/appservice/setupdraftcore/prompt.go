package setupdraftcore

func Instructions() []string {
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

func OutputSchema() map[string]any {
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
