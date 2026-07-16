package contracts

import "github.com/muidea/workorch/internal/pkg/common"

type PurgeRunCommand struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
}
