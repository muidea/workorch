package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func projectRunSnapshotResultViews(ret *readmodelcore.RunSnapshot) {
	if len(ret.Results) == 0 {
		return
	}
	limit := min(len(ret.Results), resultPageLimit)
	projected := make([]readmodelcore.Result, 0, limit)
	for _, item := range ret.Results[:limit] {
		projected = append(projected, ProjectResultView(item))
	}
	ret.Results = projected
}
