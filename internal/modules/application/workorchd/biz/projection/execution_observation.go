package projection

import (
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// RecordWorkspaceExecutionOutcome projects the concrete Workspace-owner
// admission result. Only the owner outcome decides whether the derived view
// is accepted or rejected; the application never infers a lifecycle state.
func (c *Component) RecordWorkspaceExecutionOutcome(_ string, command workorchdevents.WorkspaceRunRequestedCommand, outcome workorchdevents.WorkspaceRunRequestedResult) {
	if c == nil {
		return
	}
	c.ApplyWorkspaceExecutionAdmission(command, runtimecontract.RuntimeExecutionIntentAdmission(outcome))
}
