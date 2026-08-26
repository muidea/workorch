package content

import "github.com/muidea/workorch/internal/pkg/common"

type ArtifactContent struct {
	URI           string
	FileName      string
	ContentType   string
	Content       []byte
	ContentSHA256 string
}

type ArtifactRef struct {
	URI          string
	WorkspaceID  common.WorkspaceID
	RunID        common.RunID
	RelativePath string
}
