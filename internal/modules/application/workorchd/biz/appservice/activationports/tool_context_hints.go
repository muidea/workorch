package activationports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

type WorkspaceToolContextHints struct {
	Template  map[string]any
	Workspace map[string]any
	Effective map[string]any
}

func WorkspaceEffectiveToolContextHints(ctx context.Context, hub magicevent.Hub, workspace *workspacemodel.Workspace) map[string]any {
	return WorkspaceToolContextHintsFor(ctx, hub, workspace).Effective
}

func WorkspaceToolContextHintsFor(ctx context.Context, hub magicevent.Hub, workspace *workspacemodel.Workspace) WorkspaceToolContextHints {
	if hub == nil || workspace == nil {
		if workspace == nil {
			return WorkspaceToolContextHints{}
		}
		workspaceHints := activationcore.CloneContextHints(workspace.ContextHints.Map())
		return WorkspaceToolContextHints{
			Workspace: workspaceHints,
			Effective: activationcore.MergeTemplateWorkspaceToolContextHints(
				nil,
				workspaceHints,
			),
		}
	}
	templateHints := activationcore.MatchingWorkspaceTemplateContextHints(ctx, workspaceTemplateReader{hub: hub}, workspace)
	workspaceHints := activationcore.CloneContextHints(workspace.ContextHints.Map())
	return WorkspaceToolContextHints{
		Template:  templateHints,
		Workspace: workspaceHints,
		Effective: activationcore.MergeTemplateWorkspaceToolContextHints(templateHints, workspaceHints),
	}
}

// WorkspaceToolContextHintsFromTemplate is the Runtime activation path. It
// consumes the exact Template instance already bound by Definition admission
// and therefore performs no second live owner read.
func WorkspaceToolContextHintsFromTemplate(workspace *workspacemodel.Workspace, template *workspacemodel.WorkspaceTemplate) WorkspaceToolContextHints {
	if workspace == nil {
		return WorkspaceToolContextHints{}
	}
	workspaceHints := activationcore.CloneContextHints(workspace.ContextHints.Map())
	var templateHints map[string]any
	if template != nil {
		templateHints = activationcore.CloneContextHints(template.ContextHints.Map())
	}
	return WorkspaceToolContextHints{
		Template:  templateHints,
		Workspace: workspaceHints,
		Effective: activationcore.MergeTemplateWorkspaceToolContextHints(templateHints, workspaceHints),
	}
}
