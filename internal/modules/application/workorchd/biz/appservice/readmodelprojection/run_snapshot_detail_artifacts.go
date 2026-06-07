package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func projectRunSnapshotArtifactViews(ret *readmodelcore.RunSnapshot) {
	if len(ret.Artifacts) == 0 {
		return
	}
	limit := min(len(ret.Artifacts), artifactPageLimit)
	projected := make([]readmodelcore.Artifact, 0, limit)
	for _, item := range ret.Artifacts[:limit] {
		projected = append(projected, ProjectArtifactView(item))
	}
	ret.Artifacts = projected
}
