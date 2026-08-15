package content

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

func (r *Reader) ReadRuntimeRaw(ctx context.Context, ref string) (RuntimeRawContent, error) {
	if r == nil || !strings.EqualFold(r.repositoryBackend, "file") || r.repositoryDataDir == "" {
		return RuntimeRawContent{}, common.ErrNotFound
	}
	parsed, err := ParseRuntimeRawRef(ref)
	if err != nil {
		return RuntimeRawContent{}, err
	}
	switch parsed.Collection {
	case "llm-calls", "steps", "tool-calls":
		return r.readRuntimeRawFile(parsed)
	case "runtime-checkpoints":
		return r.readRuntimeCheckpointRaw(ctx, parsed)
	case "results":
		return r.readResultRaw(ctx, parsed)
	default:
		return RuntimeRawContent{}, common.ErrInvalidArgument
	}
}

func (r *Reader) readRuntimeRawFile(parsed RuntimeRawRef) (RuntimeRawContent, error) {
	collectionRoot := filepath.Join(r.repositoryDataDir, "internal", "workspaces", string(parsed.WorkspaceID), "runs", string(parsed.RunID), parsed.Collection)
	itemRoot, err := RuntimeRawItemRoot(collectionRoot, parsed.RunID, parsed.ItemPath)
	if err != nil {
		return RuntimeRawContent{}, err
	}
	cleanPath, err := SafeRuntimeRawContentPath(itemRoot, parsed.FileName)
	if err != nil {
		return RuntimeRawContent{}, err
	}
	body, err := os.ReadFile(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			return RuntimeRawContent{}, common.ErrNotFound
		}
		return RuntimeRawContent{}, err
	}
	return RuntimeRawContent{
		Ref:         parsed.Ref,
		FileName:    parsed.FileName,
		ContentType: RuntimeRawContentType(parsed.FileName),
		Content:     body,
	}, nil
}
