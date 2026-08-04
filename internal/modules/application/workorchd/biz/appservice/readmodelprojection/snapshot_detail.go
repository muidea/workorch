package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func ProjectRunSnapshotDetail(snapshot readmodelcore.RunSnapshotState) readmodelcore.RunSnapshotState {
	ret := snapshot
	projectRunSnapshotStepDetails(&ret)
	projectRunSnapshotRuntimeDetails(&ret)
	projectRunSnapshotResultDetails(&ret)
	projectRunSnapshotGovernanceDetails(&ret)
	ret.ConvergenceStatus = MapConvergenceStatus(ret)
	return ret
}

func ProjectRunSnapshotCompact(snapshot readmodelcore.RunSnapshotState) readmodelcore.RunSnapshotState {
	ret := snapshot
	projectRunSnapshotStepDetails(&ret)
	ret.ConvergenceStatus = MapConvergenceStatus(ret)
	return ret
}
