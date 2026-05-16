package appservice

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

type RuntimeRawContent struct {
	Ref         string
	FileName    string
	ContentType string
	Content     []byte
}

func ReadRuntimeRawContentWithSession(ctx context.Context, session *RuntimeSession, ref string) (ret RuntimeRawContent, err error) {
	exports, err := session.Exports()
	if err != nil {
		return RuntimeRawContent{}, err
	}
	defer session.CloseOnReturn(ctx, &err)
	if !strings.EqualFold(session.RepositoryBackend(), "file") || strings.TrimSpace(session.RepositoryDataDir()) == "" {
		return RuntimeRawContent{}, common.ErrNotFound
	}
	parsed, err := parseRuntimeRawContentRef(ref)
	if err != nil {
		return RuntimeRawContent{}, err
	}
	switch parsed.Collection {
	case "llm-calls", "steps", "tool-calls":
		root := filepath.Join(session.RepositoryDataDir(), "internal", "workspaces", string(parsed.WorkspaceID), "runs", string(parsed.RunID), parsed.Collection, filepath.FromSlash(parsed.ItemPath))
		cleanPath, err := safeRuntimeRawContentPath(root, parsed.FileName)
		if err != nil {
			return RuntimeRawContent{}, err
		}
		content, err := os.ReadFile(cleanPath)
		if err != nil {
			if os.IsNotExist(err) {
				return RuntimeRawContent{}, common.ErrNotFound
			}
			return RuntimeRawContent{}, err
		}
		return RuntimeRawContent{
			Ref:         parsed.Ref,
			FileName:    parsed.FileName,
			ContentType: runtimeRawContentType(parsed.FileName),
			Content:     content,
		}, nil
	case "runtime-checkpoints":
		if exports.Repositories.RuntimeCheckpoints == nil {
			return RuntimeRawContent{}, common.ErrNotFound
		}
		item, err := exports.Repositories.RuntimeCheckpoints.Get(ctx, common.CheckpointID(parsed.ItemPath))
		if err != nil {
			return RuntimeRawContent{}, err
		}
		if item.WorkspaceID != parsed.WorkspaceID || item.RunID != parsed.RunID {
			return RuntimeRawContent{}, common.ErrNotFound
		}
		content, err := json.MarshalIndent(item.Data, "", "  ")
		if err != nil {
			return RuntimeRawContent{}, err
		}
		return RuntimeRawContent{Ref: parsed.Ref, FileName: parsed.FileName, ContentType: runtimeRawContentType(parsed.FileName), Content: content}, nil
	case "results":
		if exports.Repositories.Results == nil {
			return RuntimeRawContent{}, common.ErrNotFound
		}
		item, err := exports.Repositories.Results.Get(ctx, common.ResultID(parsed.ItemPath))
		if err != nil {
			return RuntimeRawContent{}, err
		}
		if item.WorkspaceID != parsed.WorkspaceID || item.RunID != parsed.RunID {
			return RuntimeRawContent{}, common.ErrNotFound
		}
		var payload any
		switch parsed.FileName {
		case "dod-assessment.json":
			payload = item.DoDAssessment
		case "summary-detail.json":
			payload = item.SummaryDetail
		default:
			return RuntimeRawContent{}, common.ErrInvalidArgument
		}
		content, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return RuntimeRawContent{}, err
		}
		return RuntimeRawContent{Ref: parsed.Ref, FileName: parsed.FileName, ContentType: runtimeRawContentType(parsed.FileName), Content: content}, nil
	default:
		return RuntimeRawContent{}, common.ErrInvalidArgument
	}
}

func ReadRuntimeRawContentWithBackend(ctx context.Context, backend string, dataRoot string, ref string) (RuntimeRawContent, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return RuntimeRawContent{}, err
	}
	return ReadRuntimeRawContentWithSession(ctx, session, ref)
}

type runtimeRawContentRef struct {
	Ref         string
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	Collection  string
	ItemPath    string
	FileName    string
}

func parseRuntimeRawContentRef(ref string) (runtimeRawContentRef, error) {
	ref = strings.TrimSpace(ref)
	const prefix = "workorch://workspaces/"
	if ref == "" || !strings.HasPrefix(ref, prefix) {
		return runtimeRawContentRef{}, common.ErrInvalidArgument
	}
	rest := strings.TrimPrefix(ref, prefix)
	parts := strings.Split(rest, "/")
	if len(parts) < 6 || !safeRuntimeRawRefPart(parts[0]) || parts[1] != "runs" || !safeRuntimeRawRefPart(parts[2]) || !safeRuntimeRawRefPart(parts[3]) {
		return runtimeRawContentRef{}, common.ErrInvalidArgument
	}
	collection := parts[3]
	fileName := parts[len(parts)-1]
	if !allowedRuntimeRawContentFile(collection, fileName) {
		return runtimeRawContentRef{}, common.ErrInvalidArgument
	}
	itemParts := parts[4 : len(parts)-1]
	if len(itemParts) == 0 {
		return runtimeRawContentRef{}, common.ErrInvalidArgument
	}
	for _, part := range itemParts {
		if !safeRuntimeRawRefPart(part) {
			return runtimeRawContentRef{}, common.ErrInvalidArgument
		}
	}
	return runtimeRawContentRef{
		Ref:         ref,
		WorkspaceID: common.WorkspaceID(parts[0]),
		RunID:       common.RunID(parts[2]),
		Collection:  collection,
		ItemPath:    strings.Join(itemParts, "/"),
		FileName:    fileName,
	}, nil
}

func safeRuntimeRawRefPart(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && value != "." && value != ".." && !strings.Contains(value, "/") && !strings.Contains(value, "\\")
}

func allowedRuntimeRawContentFile(collection string, fileName string) bool {
	collection = strings.TrimSpace(collection)
	switch strings.TrimSpace(fileName) {
	case "request.json", "response.json", "response.sse", "usage.json", "repair-response.json":
		return collection == "llm-calls"
	case "input.json":
		return collection == "steps"
	case "arguments.json":
		return collection == "tool-calls"
	case "output.json":
		return collection == "steps" || collection == "tool-calls"
	case "data.json":
		return collection == "runtime-checkpoints"
	case "dod-assessment.json", "summary-detail.json":
		return collection == "results"
	default:
		return false
	}
}

func safeRuntimeRawContentPath(root string, fileName string) (string, error) {
	root = filepath.Clean(root)
	candidate := filepath.Join(root, filepath.Clean(fileName))
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return "", err
	}
	if rel == "." || strings.Contains(rel, string(filepath.Separator)) || strings.HasPrefix(rel, "..") {
		return "", common.ErrInvalidArgument
	}
	return candidate, nil
}

func runtimeRawContentType(fileName string) string {
	switch strings.TrimSpace(fileName) {
	case "response.sse":
		return "text/event-stream; charset=utf-8"
	case "request.json", "response.json", "usage.json", "repair-response.json", "input.json", "output.json", "arguments.json", "data.json", "dod-assessment.json", "summary-detail.json":
		return "application/json; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}
