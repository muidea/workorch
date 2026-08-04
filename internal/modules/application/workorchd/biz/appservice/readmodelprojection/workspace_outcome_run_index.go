package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"

func workspaceOutcomeRunsByID(runs []appmodelcore.Run) map[string]*appmodelcore.Run {
	ret := make(map[string]*appmodelcore.Run, len(runs))
	for idx := range runs {
		ret[runs[idx].ID] = &runs[idx]
	}
	return ret
}
