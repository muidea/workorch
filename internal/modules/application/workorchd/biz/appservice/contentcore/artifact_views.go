package contentcore

import projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

func ArtifactViews(items []projectioncontract.Artifact) []ArtifactView {
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
