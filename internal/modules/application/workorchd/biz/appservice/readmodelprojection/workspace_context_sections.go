package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/localizationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func workspaceContextSections(snapshot readmodelcore.WorkspaceSnapshotState, progress projectioncontract.TaskProgress, language string) []projectioncontract.ContextSection {
	return []projectioncontract.ContextSection{
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
