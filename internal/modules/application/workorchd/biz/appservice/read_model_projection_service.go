package appservice

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelports"

func ProjectRunSnapshotDetail(snapshot RunSnapshot) RunSnapshot {
	return readmodelports.ProjectRunSnapshotDetail(snapshot)
}

func ProjectWorkspaceSnapshotDetail(snapshot WorkspaceSnapshot) WorkspaceSnapshot {
	return readmodelports.ProjectWorkspaceSnapshotDetail(snapshot)
}

func ProjectWorkspaceSnapshotSummary(snapshot WorkspaceSnapshot) WorkspaceSnapshot {
	return readmodelports.ProjectWorkspaceSnapshotSummary(snapshot)
}

func ProjectDecisionView(item Decision) Decision {
	return readmodelports.ProjectDecisionView(item)
}
