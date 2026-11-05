package events

import (
	"fmt"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type QueryRuntimeLifecycleCommand struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
}

func (c QueryRuntimeLifecycleCommand) Validate() error {
	if c.WorkspaceID == "" || c.RunID == "" {
		return fmt.Errorf("%w: runtime lifecycle query requires scope", common.ErrInvalidArgument)
	}
	return nil
}

type QueryRuntimeLifecycleResult struct {
	Lifecycle runmodel.RuntimeLifecycleState
}

func (r QueryRuntimeLifecycleResult) Validate() error {
	v := r.Lifecycle
	if v.WorkspaceID == "" || v.RunID == "" || v.BootstrapHash == "" || v.State == "" {
		return fmt.Errorf("%w: runtime lifecycle result lacks identity", common.ErrInvalidArgument)
	}
	if v.LifecycleFence != nil && (v.LifecycleFence.Validate() != nil || v.LifecycleFence.WorkspaceID != v.WorkspaceID || v.LifecycleFence.RunID != v.RunID || v.LifecycleFence.RuntimeStateRevision != v.RuntimeStateRevision) {
		return fmt.Errorf("%w: runtime lifecycle result has unrelated fence", common.ErrConflict)
	}
	return nil
}
