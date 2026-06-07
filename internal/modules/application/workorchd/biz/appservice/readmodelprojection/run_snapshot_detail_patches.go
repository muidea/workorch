package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func projectRunSnapshotPatchViews(ret *readmodelcore.RunSnapshot) {
	if len(ret.Patches) == 0 {
		return
	}
	limit := min(len(ret.Patches), patchPageLimit)
	projected := make([]readmodelcore.Patch, 0, limit)
	for _, item := range ret.Patches[:limit] {
		projected = append(projected, ProjectPatchView(item))
	}
	ret.Patches = projected
}
