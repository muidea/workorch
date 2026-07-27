package readmodelprojection

import (
	"strings"

	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func SelectWorkspaceProjectionRun(workspace *workspacemodel.Workspace, runs []*runmodel.Run, results []*resultmodel.Result, eventRecords []*eventstreammodel.EventRecord) *runmodel.Run {
	if workspace == nil || len(runs) == 0 {
		return nil
	}
	candidates := workspaceProjectionRunCandidates(workspace, runs)
	if candidates.active != nil && !IsTerminalProjectionRunState(candidates.active.State) {
		return candidates.active
	}
	latestEvent := SelectLatestRunEvent(workspace.ID, candidates.byID, eventRecords)
	if resultRun := selectTerminalWorkspaceResultRun(workspace, candidates.byID, results, latestEvent); resultRun != nil {
		return resultRun
	}
	if latestEvent != nil {
		return candidates.byID[latestEvent.RunID]
	}
	if resultRun := SelectAcceptedResultRun(workspace.ID, candidates.byID, results); resultRun != nil {
		return resultRun
	}
	if candidates.active != nil {
		return candidates.active
	}
	if workspace.State == common.WorkspaceStateActive && strings.TrimSpace(string(workspace.ActiveRunID)) == "" {
		return nil
	}
	return candidates.latest
}
