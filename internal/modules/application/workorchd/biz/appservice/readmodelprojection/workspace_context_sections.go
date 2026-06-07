package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/localizationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func workspaceContextSections(snapshot readmodelcore.WorkspaceSnapshot, progress readmodelcore.TaskProgress, language string) []readmodelcore.ContextSection {
	return []readmodelcore.ContextSection{
		{
			ID:       "workspace",
			Kind:     "workspace",
			Title:    localizationcore.WorkspaceProgressText(language, "workspace"),
			Priority: 100,
			State:    snapshot.Workspace.State,
			Summary:  snapshot.Workspace.Title,
			Data: map[string]any{
				"id":                   snapshot.Workspace.ID,
				"title":                snapshot.Workspace.Title,
				"description":          snapshot.Workspace.Description,
				"deliverableDirectory": snapshot.Workspace.DeliverableDirectory,
			},
		},
		{
			ID:           "task_progress",
			Kind:         "task_progress",
			Title:        localizationcore.WorkspaceProgressText(language, "task_progress"),
			Priority:     90,
			State:        progress.State,
			Summary:      progress.Summary,
			EvidenceRefs: progress.EvidenceRefs,
		},
	}
}
