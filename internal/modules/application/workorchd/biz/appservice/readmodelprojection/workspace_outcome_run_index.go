package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func workspaceOutcomeRunsByID(runs []readmodelcore.Run) map[string]*readmodelcore.Run {
	ret := make(map[string]*readmodelcore.Run, len(runs))
	for idx := range runs {
		ret[runs[idx].ID] = &runs[idx]
	}
	return ret
}
