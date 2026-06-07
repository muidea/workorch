package runtimecore

import "github.com/muidea/workorch/internal/pkg/common"

type Dependencies struct {
	Snapshot SnapshotReadRuntime
	Actions  ActionDependencies
}

type ActionDependencies struct {
	Workspace WorkspaceActionRuntime
	Run       RunActionRuntime
	Decision  DecisionActionRuntime
	Approval  ApprovalActionRuntime
}

type Adapter struct {
	snapshot SnapshotReadRuntime
	actions  ActionDependencies
}

func NewAdapter(deps Dependencies) *Adapter {
	return &Adapter{
		snapshot: deps.Snapshot,
		actions:  deps.Actions,
	}
}

func validateRequiredDependencies(values ...any) error {
	for _, value := range values {
		if value == nil {
			return common.ErrInvalidArgument
		}
	}
	return nil
}
