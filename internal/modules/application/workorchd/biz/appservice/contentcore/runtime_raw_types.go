package contentcore

import "github.com/muidea/workorch/internal/pkg/common"

type RuntimeRawContent struct {
	Ref         string
	FileName    string
	ContentType string
	Content     []byte
}

type RuntimeRawRef struct {
	Ref         string
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	Collection  string
	ItemPath    string
	FileName    string
}
