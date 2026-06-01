package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspaceModel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type runStateSubsystem struct {
	runs repository.RunRepository
}

func newRunStateSubsystem(runs repository.RunRepository) *runStateSubsystem {
	return &runStateSubsystem{runs: runs}
}

func (s *runStateSubsystem) nextReplanRunID(ctx context.Context, predecessor common.RunID) (common.RunID, error) {
	if s == nil || s.runs == nil || predecessor == "" {
		return "", common.ErrInvalidArgument
	}
	base := runtimeReadableIDPart(string(predecessor))
	if base == "" {
		base = "run"
	}
	for seq := 1; seq <= 100; seq++ {
		candidate := common.RunID(fmt.Sprintf("%s-replan-%03d", base, seq))
		if _, err := s.runs.Get(ctx, candidate); errors.Is(err, common.ErrNotFound) {
			return candidate, nil
		} else if err != nil {
			return "", err
		}
	}
	return "", common.ErrConflict
}

func (s *runStateSubsystem) hydrateReplanContextWithInheritedPrimaryFailure(ctx context.Context, run *runModel.Run, replanContext map[string]any) {
	if s == nil || run == nil || len(replanContext) == 0 {
		return
	}
	if primary := primaryActionableFailureFromReplanContinuation(replanContext); len(primary) > 0 {
		return
	}
	inherited := s.findInheritedActionableReplanContinuation(ctx, run, map[common.RunID]bool{})
	if len(inherited) == 0 {
		return
	}
	if timeline := mapSliceFromValue(inherited[stepInputKeyProcessEvidenceTimeline]); len(timeline) > 0 {
		replanContext[stepInputKeyProcessEvidenceTimeline] = timeline
	}
	if primary := primaryActionableFailureFromReplanContinuation(inherited); len(primary) > 0 {
		if digest := currentProgressDigestFromPrimaryActionableFailure(primary); len(digest) > 0 {
			replanContext[stepInputKeyCurrentProgressDigest] = digest
		}
	} else if digest := mapFromValue(inherited[stepInputKeyCurrentProgressDigest]); len(digest) > 0 {
		replanContext[stepInputKeyCurrentProgressDigest] = digest
	}
	if pkg := mapFromValue(inherited["repairActionPackage"]); len(pkg) > 0 && len(mapFromValue(replanContext["repairActionPackage"])) == 0 {
		replanContext["repairActionPackage"] = pkg
	}
}

func (s *runStateSubsystem) findInheritedActionableReplanContinuation(ctx context.Context, run *runModel.Run, visited map[common.RunID]bool) map[string]any {
	if s == nil || run == nil || run.ID == "" {
		return nil
	}
	if visited == nil {
		visited = map[common.RunID]bool{}
	}
	if visited[run.ID] {
		return nil
	}
	visited[run.ID] = true
	replan := mapFromValue(run.ExecutionContext["replanContinuation"])
	if primary := primaryActionableFailureFromReplanContinuation(replan); len(primary) > 0 {
		return replan
	}
	predecessorID := common.RunID(strings.TrimSpace(stringFromMap(replan, "predecessorRunId")))
	if predecessorID == "" || s.runs == nil || visited[predecessorID] {
		return nil
	}
	predecessor, err := s.runs.Get(ctx, predecessorID)
	if err != nil {
		return nil
	}
	return s.findInheritedActionableReplanContinuation(ctx, predecessor, visited)
}

func (s *runStateSubsystem) canTerminalRunReleaseWorkspace(ctx context.Context, workspace *workspaceModel.Workspace, run *runModel.Run) (bool, error) {
	if workspace == nil || run == nil || workspace.ID != run.WorkspaceID {
		return false, nil
	}
	if workspace.ActiveRunID == "" || workspace.ActiveRunID == run.ID {
		return true, nil
	}
	if s == nil || s.runs == nil {
		return false, common.ErrInvalidArgument
	}
	activeRun, err := s.runs.Get(ctx, workspace.ActiveRunID)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return true, nil
		}
		return false, err
	}
	if activeRun.WorkspaceID != workspace.ID {
		return true, nil
	}
	return isTerminalRunState(activeRun.State), nil
}
