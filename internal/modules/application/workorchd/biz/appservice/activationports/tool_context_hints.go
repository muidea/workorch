package activationports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/events"
)

type WorkspaceToolContextHints struct {
	Template  map[string]any
	Workspace map[string]any
	Effective map[string]any
}

func WorkspaceEffectiveToolContextHints(ctx context.Context, hub events.Hub, workspace *workspacemodel.Workspace) map[string]any {
	return WorkspaceToolContextHintsFor(ctx, hub, workspace).Effective
}

func WorkspaceToolContextHintsFor(ctx context.Context, hub events.Hub, workspace *workspacemodel.Workspace) WorkspaceToolContextHints {
	if hub == nil || workspace == nil {
		if workspace == nil {
			return WorkspaceToolContextHints{}
		}
		workspaceHints := activationcore.CloneContextHints(workspace.ContextHints)
		return WorkspaceToolContextHints{
			Workspace: workspaceHints,
			Effective: activationcore.MergeTemplateWorkspaceToolContextHints(
				nil,
				workspaceHints,
			),
		}
	}
	templateHints := activationcore.MatchingWorkspaceTemplateContextHints(ctx, workspaceTemplateReader{hub: hub}, workspace)
	workspaceHints := activationcore.CloneContextHints(workspace.ContextHints)
	return WorkspaceToolContextHints{
		Template:  templateHints,
		Workspace: workspaceHints,
		Effective: activationcore.MergeTemplateWorkspaceToolContextHints(templateHints, workspaceHints),
	}
}
