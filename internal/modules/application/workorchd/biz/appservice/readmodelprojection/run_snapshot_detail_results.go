package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func projectRunSnapshotResultDetails(ret *readmodelcore.RunSnapshot) {
	if ret == nil {
		return
	}

	projectRunSnapshotResultViews(ret)
	projectRunSnapshotPatchViews(ret)
	projectRunSnapshotValidationViews(ret)
	projectRunSnapshotArtifactViews(ret)
}
