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
	ret.State = string(common.WorkspaceStateActive)
	ret.Terminal = false
	ret.Accepted = false
	ret.Reason = "workspace definitions changed; execution must be rerun"
	ret.Summary = ret.Reason
	return true
}
