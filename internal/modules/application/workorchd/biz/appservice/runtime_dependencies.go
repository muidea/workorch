package appservice

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"

func RuntimeDependenciesFromExports(exports bootstrap.Exports) RuntimeDependencies {
	return RuntimeDependencies{
		Snapshot: RuntimeSnapshotDependencies{
			Workspaces:         exports.Repositories.Workspaces,
			Goals:              exports.Repositories.Goals,
			DoDs:               exports.Repositories.DoDs,
			Results:            exports.Repositories.Results,
			Artifacts:          exports.Repositories.Artifacts,
			EventRecords:       exports.Repositories.EventRecords,
			Runs:               exports.Repositories.Runs,
			Steps:              exports.Repositories.Steps,
			ToolCalls:          exports.Repositories.ToolCalls,
			Patches:            exports.Repositories.Patches,
			ValidationResults:  exports.Repositories.ValidationResults,
			RuntimeCheckpoints: exports.Repositories.RuntimeCheckpoints,
			Tickets:            exports.Repositories.Tickets,
			Resolutions:        exports.Repositories.Resolutions,
			Approvals:          exports.Repositories.Approvals,
		},
		Actions: RuntimeActionDependencies{
			Workspace: newWorkspaceActionPort(exports.Services.WorkspaceService),
			Run:       newRunActionPort(exports.Services.RunService),
			Decision:  newDecisionActionPort(exports.Services.DecisionService),
			Approval:  newApprovalActionPort(exports.Services.ApprovalService),
		},
	}
}
