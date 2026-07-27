package events

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RetryRuntimeV2DriveCommand is an Orchestrator-private scheduling retry. It
// does not carry authority or a target state; normal SM-4 reloads the persisted
// aggregate only after the background drive is accepted.
type RetryRuntimeV2DriveCommand struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	CausationID string
}

func (c RetryRuntimeV2DriveCommand) Validate() error {
	if c.WorkspaceID == "" || c.RunID == "" || strings.TrimSpace(c.CausationID) == "" {
		return fmt.Errorf("%w: runtime drive retry requires scope and causation", common.ErrInvalidArgument)
	}
	return nil
}
