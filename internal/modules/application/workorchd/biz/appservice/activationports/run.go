package activationports

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
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
	Lineage            runmodel.RunLineage
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
		if err := CreateWorkspaceExecutionRun(ctx, commandHub, runID, request.WorkspaceID, request.GoalID, request.DoDID, request.DefinitionRevision, request.RuntimeBootstrap, request.Priority, request.Lineage); err != nil {
			if errors.Is(err, common.ErrConflict) {
				continue
			}
			return "", err
		}
		if request.Lineage.PredecessorRunID != "" {
			// Follow existing successor chain so resume links from the lineage leaf,
			// not an intermediate predecessor that already has a successor (RVC-12 CAS).
			predecessorID, err := resolveLineageLeafPredecessor(ctx, commandHub, request.WorkspaceID, request.Lineage.PredecessorRunID)
			if err != nil {
				return "", err
			}
			if err := LinkRunSuccessor(ctx, commandHub, request.WorkspaceID, predecessorID, runID, ""); err != nil {
				return "", err
			}
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

// resolveLineageLeafPredecessor walks SuccessorRunID until empty or cycle/missing.
// Resume/continuation must attach to the leaf so LinkRunSuccessor CAS does not conflict
// when an intermediate predecessor already points at a later successor.
func resolveLineageLeafPredecessor(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, start common.RunID) (common.RunID, error) {
	current := common.RunID(strings.TrimSpace(string(start)))
	if current == "" {
		return "", common.ErrInvalidArgument
	}
	seen := map[common.RunID]struct{}{}
	for steps := 0; steps < 64; steps++ {
		if _, ok := seen[current]; ok {
			return current, nil
		}
		seen[current] = struct{}{}
		run, err := ownerreadports.ReadRunByWorkspaceOwnerCommand(ctx, hub, workspaceID, current)
		if err != nil {
			if errors.Is(err, common.ErrNotFound) {
				return current, nil
			}
			return "", err
		}
		if run == nil {
			return current, nil
		}
		next := common.RunID(strings.TrimSpace(string(run.Lineage.SuccessorRunID)))
		if next == "" || next == current {
			return current, nil
		}
		current = next
	}
	return current, nil
}
