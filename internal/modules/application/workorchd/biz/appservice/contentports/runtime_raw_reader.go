package contentports

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contentcore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (r *Reader) ReadRuntimeRaw(ctx context.Context, ref string) (contentcore.RuntimeRawContent, error) {
	if r == nil || !strings.EqualFold(r.repositoryBackend, "file") || r.repositoryDataDir == "" {
		return contentcore.RuntimeRawContent{}, common.ErrNotFound
	}
	parsed, err := contentcore.ParseRuntimeRawRef(ref)
	if err != nil {
		return contentcore.RuntimeRawContent{}, err
	}
	switch parsed.Collection {
	case "llm-calls", "steps", "tool-calls":
		return r.readRuntimeRawFile(parsed)
	case "runtime-checkpoints":
		return r.readRuntimeCheckpointRaw(ctx, parsed)
	case "results":
		return r.readResultRaw(ctx, parsed)
	default:
		return contentcore.RuntimeRawContent{}, common.ErrInvalidArgument
	}
}

func (r *Reader) readRuntimeRawFile(parsed contentcore.RuntimeRawRef) (contentcore.RuntimeRawContent, error) {
	collectionRoot := filepath.Join(r.repositoryDataDir, "internal", "workspaces", string(parsed.WorkspaceID), "runs", string(parsed.RunID), parsed.Collection)
	itemRoot, err := contentcore.RuntimeRawItemRoot(collectionRoot, parsed.RunID, parsed.ItemPath)
	if err != nil {
		return contentcore.RuntimeRawContent{}, err
	}
	cleanPath, err := contentcore.SafeRuntimeRawContentPath(itemRoot, parsed.FileName)
	if err != nil {
		return contentcore.RuntimeRawContent{}, err
	}
	content, err := os.ReadFile(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			return contentcore.RuntimeRawContent{}, common.ErrNotFound
		}
		return contentcore.RuntimeRawContent{}, err
	}
	return contentcore.RuntimeRawContent{
		Ref:         parsed.Ref,
		FileName:    parsed.FileName,
		ContentType: contentcore.RuntimeRawContentType(parsed.FileName),
		Content:     content,
	}, nil
}
