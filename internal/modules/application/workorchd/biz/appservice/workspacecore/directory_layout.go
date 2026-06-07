package workspacecore

import (
	"os"
	"path/filepath"
	"strings"
)

// DirectoryLayout describes the filesystem layout for a workspace.
type DirectoryLayout struct {
	DeliverableRoot      string
	DeliverableDirectory string
	InternalDirectory    string
	Managed              bool
}

func ResolveDirectoryLayout(repositoryBackend string, repositoryDataDir string, workspaceID string, deliverableDirectory string) (DirectoryLayout, error) {
	deliverableDirectory = strings.TrimSpace(deliverableDirectory)
	internalDirectory, err := ResolveInternalDirectory(repositoryBackend, repositoryDataDir, workspaceID)
	if err != nil {
		return DirectoryLayout{}, err
	}
	if strings.EqualFold(repositoryBackend, "file") && strings.TrimSpace(repositoryDataDir) != "" {
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
	if strings.EqualFold(repositoryBackend, "file") && strings.TrimSpace(repositoryDataDir) != "" {
		return filepath.Join(repositoryDataDir, "internal", "workspaces", workspaceID), nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(wd, ".workorch-data", "internal", "workspaces", workspaceID), nil
}
