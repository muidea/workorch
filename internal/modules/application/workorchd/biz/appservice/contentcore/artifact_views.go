package contentcore

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func ArtifactViews(items []readmodelcore.Artifact) []ArtifactView {
	views := make([]ArtifactView, 0, len(items))
	for _, item := range items {
		views = append(views, ArtifactView{
			URI:         item.URI,
			WorkspaceID: item.WorkspaceID,
			RunID:       item.RunID,
			ContentType: item.ContentType,
		})
	}
	return views
}
