package setupdraftcore

import "encoding/json"

func fromRawObject(raw map[string]json.RawMessage) Draft {
	return Draft{
		GoalText:            firstRawString(raw, "goalText", "goal", "objective", "target"),
		DoDItems:            firstRawStringSlice(raw, "doDItems", "dodItems", "doD", "dod", "definitionOfDone", "acceptanceCriteria", "acceptanceItems"),
		Assumptions:         firstRawStringSlice(raw, "assumptions", "assumption"),
		ClarifyingQuestions: firstRawStringSlice(raw, "clarifyingQuestions", "clarifying_questions", "clarificationQuestions", "questions"),
		Warnings:            firstRawStringSlice(raw, "warnings", "warning", "risks"),
		Confidence:          firstRawString(raw, "confidence"),
		GenerationSummary:   firstRawString(raw, "generationSummary", "generation_summary", "summary"),
	}
}
