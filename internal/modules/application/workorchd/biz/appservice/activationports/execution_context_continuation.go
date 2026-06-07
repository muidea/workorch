package activationports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func mergeWorkspaceExecutionContinuations(
	ctx context.Context,
	contextData map[string]any,
	workspaceID common.WorkspaceID,
	goalID common.GoalID,
	dodID common.DoDID,
	definitionChangeImpact map[string]any,
	governanceHistory []map[string]any,
	request WorkspaceExecutionContextRequest,
) error {
	var rejectedContinuation map[string]any
	if !activationcore.DefinitionChangeImpactChanged(definitionChangeImpact) {
		var err error
		rejectedContinuation, err = RejectedResultContinuation(ctx, request.Hub, workspaceID, goalID, dodID)
		if err != nil {
			return err
		}
	}
	activationcore.MergeGovernanceContinuationContext(contextData, definitionChangeImpact, governanceHistory, rejectedContinuation)
	activationcore.MergeExecutionContextPatch(contextData, request.Command.ExecutionContextPatch)
	return nil
}
