package workspacecore

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// DirectoryLayout describes the filesystem layout for a workspace.
type DirectoryLayout struct {
	DeliverableRoot      string
	DeliverableDirectory string
	InternalDirectory    string
	Managed              bool
}

func ResolveDirectoryLayout(repositoryBackend string, repositoryDataDir string, workspaceID string, deliverableDirectory string) (DirectoryLayout, error) {
	var err error
	workspaceID, err = normalizeWorkspaceDirectorySegment(workspaceID)
	if err != nil {
		return DirectoryLayout{}, err
	}
	deliverableDirectory, err = normalizeExplicitWorkspaceDirectory(deliverableDirectory)
	if err != nil {
		return DirectoryLayout{}, err
	}
	internalDirectory, err := ResolveInternalDirectory(repositoryBackend, repositoryDataDir, workspaceID)
	if err != nil {
		return DirectoryLayout{}, err
	}
	if strings.EqualFold(repositoryBackend, "file") && strings.TrimSpace(repositoryDataDir) != "" {
		repositoryDataDir, err = filepath.Abs(strings.TrimSpace(repositoryDataDir))
		if err != nil {
			return DirectoryLayout{}, err
		}
		if deliverableDirectory != "" {
			return DirectoryLayout{
				DeliverableDirectory: deliverableDirectory,
				InternalDirectory:    internalDirectory,
				Managed:              true,
			}, nil
		}
		deliverableRoot := filepath.Join(filepath.Dir(repositoryDataDir), ".workorch-workspaces")
		return DirectoryLayout{
			DeliverableRoot:      deliverableRoot,
			DeliverableDirectory: filepath.Join(deliverableRoot, workspaceID),
			InternalDirectory:    internalDirectory,
			Managed:              true,
		}, nil
	}
	if deliverableDirectory != "" {
		return DirectoryLayout{DeliverableDirectory: deliverableDirectory, InternalDirectory: internalDirectory}, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return DirectoryLayout{}, err
	}
	return DirectoryLayout{
		DeliverableDirectory: filepath.Join(wd, ".workorch-workspaces", workspaceID),
		InternalDirectory:    internalDirectory,
	}, nil
}

func ResolveInternalDirectory(repositoryBackend string, repositoryDataDir string, workspaceID string) (string, error) {
	workspaceID, err := normalizeWorkspaceDirectorySegment(workspaceID)
	if err != nil {
		return "", err
	}
	if strings.EqualFold(repositoryBackend, "file") && strings.TrimSpace(repositoryDataDir) != "" {
		root, err := filepath.Abs(strings.TrimSpace(repositoryDataDir))
		if err != nil {
			return "", err
		}
		return filepath.Join(root, "internal", "workspaces", workspaceID), nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(wd, ".workorch-data", "internal", "workspaces", workspaceID), nil
}

func normalizeWorkspaceDirectorySegment(workspaceID string) (string, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" || workspaceID == "." || workspaceID == ".." || filepath.IsAbs(workspaceID) ||
		filepath.Base(workspaceID) != workspaceID || strings.ContainsAny(workspaceID, `/\`) || strings.ContainsRune(workspaceID, 0) {
		return "", common.ErrInvalidArgument
	}
	return workspaceID, nil
}

func normalizeExplicitWorkspaceDirectory(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if strings.ContainsRune(value, 0) {
		return "", common.ErrInvalidArgument
	}
	clean := filepath.Clean(value)
	if !filepath.IsAbs(clean) && (clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator))) {
		return "", common.ErrInvalidArgument
	}
	// Resolve against the creation process, before paths enter owner hashes and
	// sealed startup input. Consumers must never reinterpret sealed relative paths.
	return filepath.Abs(clean)
}
