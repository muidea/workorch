package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func ContextGoalSection(snapshot readmodelcore.RunSnapshot) readmodelcore.ContextSection {
	goal, _ := snapshot.Run.ExecutionContext["goal"].(map[string]any)
	return readmodelcore.ContextSection{
		ID:       "goal",
		Kind:     "goal",
		Title:    "Goal",
		Priority: 95,
		Summary:  readModelStringValue(goal["text"]),
		Data:     cloneReadModelMap(goal),
	}
}

func ContextDoDSection(snapshot readmodelcore.RunSnapshot) readmodelcore.ContextSection {
	dod, _ := snapshot.Run.ExecutionContext["dod"].(map[string]any)
	return readmodelcore.ContextSection{
		ID:       "dod",
		Kind:     "dod",
		Title:    "Definition of Done",
		Priority: 95,
		State:    readModelStringValue(dod["state"]),
		Summary:  ContextDoDSummary(dod),
		Data:     cloneReadModelMap(dod),
	}
}
