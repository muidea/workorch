package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func ContextGoalSection(snapshot readmodelcore.RunSnapshot) readmodelcore.ContextSection {
	return readmodelcore.ContextSection{
		ID:       "goal",
		Kind:     "goal",
		Title:    "Goal",
		Priority: 95,
	}
}

func ContextDoDSection(snapshot readmodelcore.RunSnapshot) readmodelcore.ContextSection {
	return readmodelcore.ContextSection{
		ID:       "dod",
		Kind:     "dod",
		Title:    "Definition of Done",
		Priority: 95,
	}
}
