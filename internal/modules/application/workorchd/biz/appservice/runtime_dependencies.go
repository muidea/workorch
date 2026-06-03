package appservice

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"

func RuntimeDependenciesFromExports(exports bootstrap.Exports) RuntimeDependencies {
	actionPort := newEventHubActionPort(exports.Infrastructure.EventHub)
	return RuntimeDependencies{
		Snapshot: newEventHubSnapshotReadPort(exports.Infrastructure.EventHub),
		Actions: RuntimeActionDependencies{
			Workspace: actionPort,
			Run:       actionPort,
			Decision:  actionPort,
			Approval:  actionPort,
		},
	}
}
