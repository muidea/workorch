package retirementbackup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateAndRestoreRuntimeV1RetirementBackup(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	configDir := filepath.Join(root, "config")
	backupDir := filepath.Join(root, "backup")
	if err := os.MkdirAll(filepath.Join(dataDir, "internal"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "internal", "run.json"), []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte("before-config"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest, err := Create(dataDir, configDir, backupDir)
	if err != nil || manifest.DataSource == "" {
		t.Fatalf("Create manifest=%+v err=%v", manifest, err)
	}
	_ = os.WriteFile(filepath.Join(dataDir, "internal", "run.json"), []byte("after"), 0o600)
	_ = os.WriteFile(filepath.Join(configDir, "application.toml"), []byte("after-config"), 0o600)
	if err := Restore(backupDir); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(dataDir, "internal", "run.json"))
	config, _ := os.ReadFile(filepath.Join(configDir, "application.toml"))
	if string(data) != "before" || string(config) != "before-config" {
		t.Fatalf("restored data/config = %q/%q", data, config)
	}
}

func TestCreateRejectsBackupInsideDataDirectory(t *testing.T) {
	dataDir := t.TempDir()
	if _, err := Create(dataDir, "", filepath.Join(dataDir, "backup")); err == nil {
		t.Fatal("Create returned nil error")
	}
}

func TestCreateRequiresAvailableNonOverlappingConfigurationDirectory(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(dataDir, filepath.Join(root, "missing-config"), filepath.Join(root, "backup-missing")); err == nil {
		t.Fatal("Create accepted missing configuration directory")
	}
	if _, err := Create(dataDir, root, filepath.Join(t.TempDir(), "backup-overlap")); err == nil {
		t.Fatal("Create accepted overlapping data and configuration directories")
	}
}

func TestRestoreValidatesAllSnapshotsBeforeChangingTargets(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	configDir := filepath.Join(root, "config")
	backupDir := filepath.Join(root, "backup")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dataPath := filepath.Join(dataDir, "run.json")
	if err := os.WriteFile(dataPath, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(dataDir, configDir, backupDir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dataPath, []byte("after"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(backupDir, "config")); err != nil {
		t.Fatal(err)
	}
	if err := Restore(backupDir); err == nil {
		t.Fatal("Restore accepted incomplete backup")
	}
	content, err := os.ReadFile(dataPath)
	if err != nil || string(content) != "after" {
		t.Fatalf("data changed before all snapshots were validated: content=%q err=%v", content, err)
	}
}
