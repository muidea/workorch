package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func ContextWorkspaceSection(snapshot readmodelcore.RunSnapshot) readmodelcore.ContextSection {
	return readmodelcore.ContextSection{
		ID:       "workspace",
		Kind:     "workspace",
		Title:    "Workspace",
		Priority: 100,
		State:    snapshot.Workspace.State,
		Summary:  snapshot.Workspace.Title,
		Data: map[string]any{
			"id":                   snapshot.Workspace.ID,
			"title":                snapshot.Workspace.Title,
			"description":          snapshot.Workspace.Description,
			"deliverableDirectory": snapshot.Workspace.DeliverableDirectory,
		},
	}
}
