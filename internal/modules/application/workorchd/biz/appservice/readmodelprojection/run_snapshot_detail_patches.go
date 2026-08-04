package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func projectRunSnapshotPatchViews(ret *readmodelcore.RunSnapshotState) {
	if len(ret.Patches) == 0 {
		return
	}
	limit := min(len(ret.Patches), patchPageLimit)
	projected := make([]projectioncontract.Patch, 0, limit)
	for _, item := range ret.Patches[:limit] {
		projected = append(projected, ProjectPatchView(item))
	}
	ret.Patches = projected
}
