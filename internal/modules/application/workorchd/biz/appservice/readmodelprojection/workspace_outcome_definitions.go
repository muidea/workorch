package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func applyWorkspaceDefinitionsChangedOutcome(ret *readmodelcore.WorkspaceOutcome, snapshot readmodelcore.WorkspaceSnapshot) bool {
	if ret == nil {
		return false
	}
	if common.WorkspaceState(snapshot.Workspace.State) != common.WorkspaceStateActive ||
		strings.TrimSpace(snapshot.Workspace.ActiveRunID) != "" {
		return false
	}
	goalID, dodID := latestWorkspaceDefinitionIDs(snapshot)
	if goalID == "" && dodID == "" {
		return false
	}
	if !workspaceDefinitionsChangedInPlace(snapshot) && workspaceHasTerminalResultForDefinitions(snapshot, goalID, dodID) {
		return false
	}
	ret.State = string(common.WorkspaceStateActive)
	ret.Terminal = false
	ret.Accepted = false
	ret.Reason = "workspace definitions changed; execution must be rerun"
	ret.Summary = ret.Reason
	return true
}

func workspaceDefinitionsChangedInPlace(snapshot readmodelcore.WorkspaceSnapshot) bool {
	for idx := range snapshot.Goals {
		if strings.TrimSpace(snapshot.Goals[idx].PreviousDefinitionHash) != "" {
			return true
		}
	}
	for idx := range snapshot.DoDs {
		if strings.TrimSpace(snapshot.DoDs[idx].PreviousDefinitionHash) != "" {
			return true
		}
	}
	return false
}

func latestWorkspaceDefinitionIDs(snapshot readmodelcore.WorkspaceSnapshot) (string, string) {
	var goalID string
	for idx := range snapshot.Goals {
		if id := strings.TrimSpace(snapshot.Goals[idx].ID); id != "" {
			goalID = id
		}
	}
	var dodID string
	for idx := range snapshot.DoDs {
		if id := strings.TrimSpace(snapshot.DoDs[idx].ID); id != "" {
			dodID = id
		}
	}
	return goalID, dodID
}

func workspaceHasTerminalResultForDefinitions(snapshot readmodelcore.WorkspaceSnapshot, goalID string, dodID string) bool {
	for idx := range snapshot.Results {
		item := snapshot.Results[idx]
		if goalID != "" && strings.TrimSpace(item.GoalID) != goalID {
			continue
		}
		if dodID != "" && strings.TrimSpace(item.DoDID) != dodID {
			continue
		}
		switch common.ResultState(item.State) {
		case common.ResultStateAccepted, common.ResultStateRejected:
			return true
		}
	}
	return false
}
