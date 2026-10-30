package workspace

import (
	"context"
	"strings"

	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

type workspaceTemplateUpgradeEvent struct {
	TemplateID          string
	ExpectedContentHash string
	TargetContentHash   string
}

// The retained template ingress validates an exact read-only current-version replay.
// This event cannot update a Workspace admission or a Run activation snapshot.
func (m *Biz) submitBuiltinTemplateUpgrade(ctx context.Context, event workspaceTemplateUpgradeEvent) (workspaceevents.UpgradeBuiltinWorkspaceTemplateDefaultResult, error) {
	if m == nil || event.TemplateID == "" || event.TemplateID != strings.TrimSpace(event.TemplateID) ||
		event.ExpectedContentHash == "" || event.TargetContentHash == "" || event.ExpectedContentHash != strings.TrimSpace(event.ExpectedContentHash) || event.TargetContentHash != strings.TrimSpace(event.TargetContentHash) {
		return workspaceevents.UpgradeBuiltinWorkspaceTemplateDefaultResult{}, common.ErrInvalidArgument
	}
	m.templateMutationMu.Lock()
	defer m.templateMutationMu.Unlock()
	return m.upgradeBuiltinTemplate(ctx, event)
}
