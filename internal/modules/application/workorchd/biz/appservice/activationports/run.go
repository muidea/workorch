package activationports

import (
	"context"
	"errors"
	"fmt"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type CreateAndStartWorkspaceExecutionRunRequest struct {
	WorkspaceID        common.WorkspaceID
	GoalID             common.GoalID
	DoDID              common.DoDID
	DefinitionRevision string
	Priority           int
	MaxAttempts        int
	RuntimeBootstrap   *runtimecontract.RuntimeBootstrap
}

func CreateAndStartWorkspaceExecutionRun(ctx context.Context, commandHub magicevent.Hub, request CreateAndStartWorkspaceExecutionRunRequest) (common.RunID, error) {
	if commandHub == nil || request.WorkspaceID == "" || request.GoalID == "" || request.DoDID == "" {
		return "", common.ErrInvalidArgument
	}
	maxAttempts := request.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 8
	}
	for attempt := 0; attempt < maxAttempts; attempt++ {
		runID, err := NextWorkspaceExecutionRunID(ctx, commandHub, request.WorkspaceID)
		if err != nil {
			return "", err
		}
		if err := CreateWorkspaceExecutionRun(ctx, commandHub, runID, request.WorkspaceID, request.GoalID, request.DoDID, request.DefinitionRevision, request.RuntimeBootstrap, request.Priority); err != nil {
			if errors.Is(err, common.ErrConflict) {
				continue
			}
			return "", err
		}
		if err := RecordActivationMemoryRecallTrace(ctx, commandHub, runID, request.WorkspaceID, request.RuntimeBootstrap, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return "", err
		}
		startCtx := common.ContextWithRuntimeWorkspaceID(ctx, request.WorkspaceID)
		if err := StartRun(startCtx, commandHub, request.WorkspaceID, runID); err != nil {
			if errors.Is(err, common.ErrConflict) {
				observable, getErr := RunObservableAfterStartConflict(startCtx, commandHub, runID)
				if getErr != nil {
					return "", getErr
				}
				if observable {
					return runID, nil
				}
			}
			_ = FailRun(ctx, commandHub, request.WorkspaceID, runID, fmt.Sprintf("activation start failed: %v", err))
			return "", err
		}
		return runID, nil
	}
	return "", common.ErrConflict
}
