package appservice

import "github.com/muidea/workorch/application/workorchd/bootstrap"

func RuntimeDependenciesFromExports(exports bootstrap.Exports) RuntimeDependencies {
	return RuntimeDependencies{
		Snapshot: RuntimeSnapshotDependencies{
			Workspaces: exports.Repositories.Workspaces,
			Runs:       exports.Repositories.Runs,
			Steps:      exports.Repositories.Steps,
			Tickets:    exports.Repositories.Tickets,
			Approvals:  exports.Repositories.Approvals,
		},
		Actions: RuntimeActionDependencies{
			Workspace: newWorkspaceActionPort(exports.Services.WorkspaceService),
			Run:       newRunActionPort(exports.Services.RunService),
			Decision:  newDecisionActionPort(exports.Services.DecisionService),
			Approval:  newApprovalActionPort(exports.Services.ApprovalService),
		},
	}
}
