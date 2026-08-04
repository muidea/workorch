package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func projectRunSnapshotArtifactViews(ret *readmodelcore.RunSnapshotState) {
	if len(ret.Artifacts) == 0 {
		return
	}
	limit := min(len(ret.Artifacts), artifactPageLimit)
	projected := make([]projectioncontract.Artifact, 0, limit)
	for _, item := range ret.Artifacts[:limit] {
		projected = append(projected, ProjectArtifactView(item))
	}
	ret.Artifacts = projected
}
