package content

import "github.com/muidea/workorch/internal/pkg/common"

type ArtifactContent struct {
	URI         string
	FileName    string
	ContentType string
	Content     []byte
}

type ArtifactRef struct {
	URI          string
	WorkspaceID  common.WorkspaceID
	RunID        common.RunID
	RelativePath string
}

type ArtifactDescriptor struct {
	ContentType string
}

type ArtifactView struct {
	URI         string
	WorkspaceID string
	RunID       string
	ContentType string
}
