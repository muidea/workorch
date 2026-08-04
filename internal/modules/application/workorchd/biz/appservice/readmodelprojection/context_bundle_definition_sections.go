package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func ContextGoalSection(snapshot readmodelcore.RunSnapshotState) projectioncontract.ContextSection {
	return projectioncontract.ContextSection{
		ID:       "goal",
		Kind:     "goal",
		Title:    "Goal",
		Priority: 95,
	}
}

func ContextDoDSection(snapshot readmodelcore.RunSnapshotState) projectioncontract.ContextSection {
	return projectioncontract.ContextSection{
		ID:       "dod",
		Kind:     "dod",
		Title:    "Definition of Done",
		Priority: 95,
	}
}
