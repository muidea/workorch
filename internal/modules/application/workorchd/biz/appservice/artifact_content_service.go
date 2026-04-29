package appservice

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type ArtifactContent struct {
	URI         string
	FileName    string
	ContentType string
	Content     []byte
}

func ReadArtifactContentWithSession(ctx context.Context, session *RuntimeSession, artifactURI string) (ret ArtifactContent, err error) {
	exports, err := session.Exports()
	if err != nil {
		return ArtifactContent{}, err
	}
	defer session.CloseOnReturn(ctx, &err)
	if exports.Repositories.Artifacts == nil || !strings.EqualFold(session.RepositoryBackend(), "file") || strings.TrimSpace(session.RepositoryDataDir()) == "" {
		return ArtifactContent{}, common.ErrNotFound
	}
	ref, err := parseArtifactContentURI(artifactURI)
	if err != nil {
		return ArtifactContent{}, err
	}
	artifacts, err := exports.Repositories.Artifacts.ListByRun(ctx, ref.RunID)
	if err != nil {
		return ArtifactContent{}, err
	}
	artifact := findArtifactByURI(artifacts, ref.URI)
	if artifact == nil {
		return ArtifactContent{}, common.ErrNotFound
	}
	root := filepath.Join(session.RepositoryDataDir(), "workspaces", string(ref.WorkspaceID), "artifacts", string(ref.RunID))
	cleanPath, err := safeArtifactContentPath(root, ref.RelativePath)
	if err != nil {
		return ArtifactContent{}, err
	}
	content, err := os.ReadFile(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ArtifactContent{}, common.ErrNotFound
		}
		return ArtifactContent{}, err
	}
	return ArtifactContent{
		URI:         ref.URI,
		FileName:    filepath.Base(cleanPath),
		ContentType: firstNonEmptyArtifactContent(artifact.ContentType, "application/octet-stream"),
		Content:     content,
	}, nil
}

func ReadArtifactContentWithBackend(ctx context.Context, backend string, dataRoot string, artifactURI string) (ArtifactContent, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return ArtifactContent{}, err
	}
	return ReadArtifactContentWithSession(ctx, session, artifactURI)
}

type artifactContentRef struct {
	URI          string
	WorkspaceID  common.WorkspaceID
	RunID        common.RunID
	RelativePath string
}

func parseArtifactContentURI(artifactURI string) (artifactContentRef, error) {
	artifactURI = strings.TrimSpace(artifactURI)
	const prefix = "workorch://workspaces/"
	if artifactURI == "" || !strings.HasPrefix(artifactURI, prefix) {
		return artifactContentRef{}, common.ErrInvalidArgument
	}
	rest := strings.TrimPrefix(artifactURI, prefix)
	parts := strings.Split(rest, "/")
	if len(parts) < 4 || parts[0] == "" || parts[1] != "artifacts" || parts[2] == "" {
		return artifactContentRef{}, common.ErrInvalidArgument
	}
	relativePath := strings.Join(parts[3:], "/")
	if strings.TrimSpace(relativePath) == "" {
		return artifactContentRef{}, common.ErrInvalidArgument
	}
	for _, part := range parts[3:] {
		if part == "" || part == "." || part == ".." {
			return artifactContentRef{}, common.ErrInvalidArgument
		}
	}
	return artifactContentRef{
		URI:          artifactURI,
		WorkspaceID:  common.WorkspaceID(parts[0]),
		RunID:        common.RunID(parts[2]),
		RelativePath: relativePath,
	}, nil
}

func safeArtifactContentPath(root string, relativePath string) (string, error) {
	root = filepath.Clean(root)
	candidate := filepath.Join(root, filepath.Clean(relativePath))
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return "", err
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return "", common.ErrInvalidArgument
	}
	return candidate, nil
}

func findArtifactByURI(items []*resultmodel.Artifact, uri string) *resultmodel.Artifact {
	for _, item := range items {
		if item != nil && strings.TrimSpace(item.URI) == uri {
			return item
		}
	}
	return nil
}

func firstNonEmptyArtifactContent(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
