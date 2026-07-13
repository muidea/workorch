package retirementbackup

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const manifestVersion = "workorch.runtime-v1-retirement-backup.v1"

type Manifest struct {
	BackupDirectory string `json:"backupDirectory"`
	Version         string `json:"version"`
	CreatedAt       string `json:"createdAt"`
	DataSource      string `json:"dataSource"`
	ConfigSource    string `json:"configSource,omitempty"`
	DataSnapshot    string `json:"dataSnapshot"`
	ConfigSnapshot  string `json:"configSnapshot,omitempty"`
}

func Create(dataDir string, configDir string, destination string) (Manifest, error) {
	dataDir, err := filepath.Abs(strings.TrimSpace(dataDir))
	if err != nil || dataDir == "" {
		return Manifest{}, fmt.Errorf("invalid repository data directory")
	}
	if info, statErr := os.Stat(dataDir); statErr != nil || !info.IsDir() {
		return Manifest{}, fmt.Errorf("repository data directory is unavailable: %s", dataDir)
	}
	if strings.TrimSpace(destination) == "" {
		destination = dataDir + ".runtime-v1-retirement-backup-" + time.Now().UTC().Format("20060102T150405.000000000Z")
	}
	destination, err = filepath.Abs(destination)
	if err != nil {
		return Manifest{}, err
	}
	if pathWithin(destination, dataDir) || pathWithin(dataDir, destination) {
		return Manifest{}, fmt.Errorf("backup destination must be outside repository data directory")
	}
	if _, err := os.Stat(destination); err == nil {
		return Manifest{}, fmt.Errorf("backup destination already exists: %s", destination)
	} else if !os.IsNotExist(err) {
		return Manifest{}, err
	}
	manifest := Manifest{
		BackupDirectory: destination,
		Version:         manifestVersion,
		CreatedAt:       time.Now().UTC().Format(time.RFC3339Nano),
		DataSource:      dataDir,
		DataSnapshot:    "data",
	}
	if strings.TrimSpace(configDir) != "" {
		configDir, err = filepath.Abs(configDir)
		if err != nil {
			return Manifest{}, err
		}
		if info, statErr := os.Stat(configDir); statErr != nil || !info.IsDir() {
			return Manifest{}, fmt.Errorf("configuration directory is unavailable: %s", configDir)
		}
		if pathWithin(destination, configDir) || pathWithin(configDir, destination) {
			return Manifest{}, fmt.Errorf("backup destination must be outside configuration directory")
		}
		if pathWithin(configDir, dataDir) || pathWithin(dataDir, configDir) {
			return Manifest{}, fmt.Errorf("repository data and configuration directories must not overlap")
		}
		manifest.ConfigSource = configDir
		manifest.ConfigSnapshot = "config"
	}
	if err := os.MkdirAll(destination, 0o700); err != nil {
		return Manifest{}, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(destination)
		}
	}()
	if err := copyTree(dataDir, filepath.Join(destination, manifest.DataSnapshot)); err != nil {
		return Manifest{}, err
	}
	if manifest.ConfigSource != "" {
		if err := copyTree(manifest.ConfigSource, filepath.Join(destination, manifest.ConfigSnapshot)); err != nil {
			return Manifest{}, err
		}
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return Manifest{}, err
	}
	if err := os.WriteFile(filepath.Join(destination, "manifest.json"), data, 0o600); err != nil {
		return Manifest{}, err
	}
	cleanup = false
	return manifest, nil
}

func Restore(backupDir string) error {
	backupDir, err := filepath.Abs(strings.TrimSpace(backupDir))
	if err != nil || backupDir == "" {
		return fmt.Errorf("invalid backup directory")
	}
	data, err := os.ReadFile(filepath.Join(backupDir, "manifest.json"))
	if err != nil {
		return err
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return err
	}
	if manifest.Version != manifestVersion || manifest.DataSource == "" || manifest.DataSnapshot == "" {
		return fmt.Errorf("invalid Runtime V1 retirement backup manifest")
	}
	entries := []restoreEntry{{
		snapshot: filepath.Join(backupDir, manifest.DataSnapshot),
		target:   manifest.DataSource,
	}}
	if manifest.ConfigSource != "" || manifest.ConfigSnapshot != "" {
		if manifest.ConfigSource == "" || manifest.ConfigSnapshot == "" {
			return fmt.Errorf("invalid Runtime V1 retirement configuration backup manifest")
		}
		entries = append(entries, restoreEntry{
			snapshot: filepath.Join(backupDir, manifest.ConfigSnapshot),
			target:   manifest.ConfigSource,
		})
	}
	for idx := range entries {
		entry := &entries[idx]
		var err error
		entry.snapshot, err = filepath.Abs(entry.snapshot)
		if err != nil {
			return err
		}
		entry.target, err = filepath.Abs(entry.target)
		if err != nil {
			return err
		}
		if !pathWithin(entry.snapshot, backupDir) || pathWithin(entry.target, backupDir) || pathWithin(backupDir, entry.target) {
			return fmt.Errorf("invalid Runtime V1 retirement backup path")
		}
		if info, statErr := os.Stat(entry.snapshot); statErr != nil || !info.IsDir() {
			return fmt.Errorf("backup snapshot is unavailable: %s", entry.snapshot)
		}
	}
	if len(entries) == 2 && (pathWithin(entries[0].target, entries[1].target) || pathWithin(entries[1].target, entries[0].target)) {
		return fmt.Errorf("backup restore targets must not overlap")
	}
	return restoreTrees(entries)
}

func copyTree(source string, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			_ = input.Close()
			return err
		}
		_, copyErr := io.Copy(output, input)
		inputCloseErr := input.Close()
		outputCloseErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		if inputCloseErr != nil {
			return inputCloseErr
		}
		return outputCloseErr
	})
}

type restoreEntry struct {
	snapshot  string
	target    string
	staging   string
	previous  string
	hadTarget bool
	committed bool
}

func restoreTrees(entries []restoreEntry) error {
	for idx := range entries {
		entry := &entries[idx]
		if err := os.MkdirAll(filepath.Dir(entry.target), 0o700); err != nil {
			cleanupRestoreEntries(entries)
			return err
		}
		staging, err := os.MkdirTemp(filepath.Dir(entry.target), ".runtime-v1-retirement-restore-staging-")
		if err != nil {
			cleanupRestoreEntries(entries)
			return err
		}
		entry.staging = staging
		if err := copyTree(entry.snapshot, entry.staging); err != nil {
			cleanupRestoreEntries(entries)
			return err
		}
	}
	for idx := range entries {
		entry := &entries[idx]
		if _, err := os.Stat(entry.target); err == nil {
			previous, reserveErr := os.MkdirTemp(filepath.Dir(entry.target), ".runtime-v1-retirement-restore-previous-")
			if reserveErr != nil {
				rollbackRestoreEntries(entries, idx)
				return reserveErr
			}
			if err := os.Remove(previous); err != nil {
				rollbackRestoreEntries(entries, idx)
				return err
			}
			entry.previous = previous
			entry.hadTarget = true
			if err := os.Rename(entry.target, entry.previous); err != nil {
				rollbackRestoreEntries(entries, idx)
				return err
			}
		} else if !os.IsNotExist(err) {
			rollbackRestoreEntries(entries, idx)
			return err
		}
		if err := os.Rename(entry.staging, entry.target); err != nil {
			rollbackRestoreEntries(entries, idx+1)
			return err
		}
		entry.staging = ""
		entry.committed = true
	}
	for idx := range entries {
		if entries[idx].previous != "" {
			if err := os.RemoveAll(entries[idx].previous); err != nil {
				return err
			}
			entries[idx].previous = ""
		}
	}
	return nil
}

func rollbackRestoreEntries(entries []restoreEntry, count int) {
	for idx := count - 1; idx >= 0; idx-- {
		entry := &entries[idx]
		if entry.committed {
			_ = os.RemoveAll(entry.target)
		}
		if entry.hadTarget && entry.previous != "" {
			_ = os.Rename(entry.previous, entry.target)
			entry.previous = ""
		}
	}
	cleanupRestoreEntries(entries)
}

func cleanupRestoreEntries(entries []restoreEntry) {
	for idx := range entries {
		if entries[idx].staging != "" {
			_ = os.RemoveAll(entries[idx].staging)
		}
	}
}

func pathWithin(path string, root string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
