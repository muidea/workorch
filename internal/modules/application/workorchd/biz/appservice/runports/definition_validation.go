package runports

import (
	"context"
	"sync"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ValidateRunDefinitions(ctx context.Context, hub magicevent.Hub, workspaceID string, goalID string, dodID string) error {
	if goalID == "" && dodID == "" {
		return nil
	}
	if hub == nil || goalID == "" || dodID == "" {
		return common.ErrInvalidArgument
	}
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, common.WorkspaceID(workspaceID))
	var (
		goal    *workspacemodel.Goal
		dod     *workspacemodel.DoD
		wsErr   error
		goalErr error
		dodErr  error
	)
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		_, wsErr = ownerreadports.ReadWorkspaceByOwnerCommand(ctx, hub, common.WorkspaceID(workspaceID))
	}()
	go func() {
		defer wg.Done()
		goal, goalErr = ownerreadports.ReadGoalByOwnerCommand(ctx, hub, common.GoalID(goalID))
	}()
	go func() {
		defer wg.Done()
		dod, dodErr = ownerreadports.ReadDoDByOwnerCommand(ctx, hub, common.DoDID(dodID))
	}()
	wg.Wait()
	if wsErr != nil {
		return wsErr
	}
	if goalErr != nil {
		return goalErr
	}
	if goal.WorkspaceID != common.WorkspaceID(workspaceID) {
		return common.ErrConflict
	}
	if dodErr != nil {
		return dodErr
	}
	if dod.WorkspaceID != common.WorkspaceID(workspaceID) {
		return common.ErrConflict
	}
	if dod.State != common.DoDStateReady && dod.State != common.DoDStateSatisfied {
		return common.ErrConflict
	}
	return nil
}
