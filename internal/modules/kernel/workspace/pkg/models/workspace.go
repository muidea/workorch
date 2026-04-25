package workspace

import "github.com/muidea/workorch/internal/pkg/common"

type Workspace struct {
	ID               common.WorkspaceID
	Title            string
	Description      string
	WorkingDirectory string
	State            common.WorkspaceState
	ActiveRunID      common.RunID
}
