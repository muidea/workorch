package workorch

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/services/workorch/biz/retirementbackup"
)

func TestParseRuntimeV1RetirementMaintenanceOptions(t *testing.T) {
	options, err := parseRuntimeV1RetirementMaintenanceOptions([]string{"runtime-v1-retirement", "--apply", "--backup-dir", "/tmp/backup"})
	if err != nil || !options.Matched || !options.Apply || options.BackupDir != "/tmp/backup" {
		t.Fatalf("options=%+v err=%v", options, err)
	}
}

func TestParseRuntimeV1RetirementMaintenanceOptionsRejectsInvalidFlagCombinations(t *testing.T) {
	for _, args := range [][]string{
		{"runtime-v1-retirement", "--apply", "--dry-run"},
		{"runtime-v1-retirement", "--apply", "--restore-from", "/tmp/backup"},
		{"runtime-v1-retirement", "--backup-dir", "/tmp/backup"},
		{"runtime-v1-retirement", "--backup-dir"},
	} {
		if _, err := parseRuntimeV1RetirementMaintenanceOptions(args); err == nil {
			t.Fatalf("parse accepted args %#v", args)
		}
	}
}

func TestParseRuntimeV1RetirementMaintenanceOptionsSupportsEqualsValues(t *testing.T) {
	options, err := parseRuntimeV1RetirementMaintenanceOptions([]string{"runtime-v1-retirement", "--apply", "--backup-dir=/tmp/backup"})
	if err != nil || options.BackupDir != "/tmp/backup" {
		t.Fatalf("options=%+v err=%v", options, err)
	}
}

func TestPrepareRuntimeV1RetirementMaintenanceRestoresBeforeStartup(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	backupDir := filepath.Join(root, "backup")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dataDir, "run.json")
	if err := os.WriteFile(path, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := retirementbackup.Create(dataDir, "", backupDir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("after"), 0o600); err != nil {
		t.Fatal(err)
	}
	handled, err := prepareRuntimeV1RetirementMaintenance(StartupArgs{CobraArgs: []string{"runtime-v1-retirement", "--restore-from", backupDir}})
	if err != nil || !handled {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "before" {
		t.Fatalf("content=%q err=%v", content, err)
	}
}

func TestPrepareRuntimeV1RetirementMaintenanceDryRunDoesNotCreateBackup(t *testing.T) {
	backupDir := filepath.Join(t.TempDir(), "backup")
	handled, err := prepareRuntimeV1RetirementMaintenance(StartupArgs{CobraArgs: []string{"runtime-v1-retirement", "--dry-run"}})
	if err != nil || handled {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	if _, err := os.Stat(backupDir); !os.IsNotExist(err) {
		t.Fatalf("dry-run unexpectedly created backup: %v", err)
	}
	if value := os.Getenv(runtimeV1RetirementBackupEnv); value != "" {
		t.Fatalf("backup env=%q, want empty", value)
	}
}

func TestPrepareRuntimeV1RetirementMaintenanceApplyCreatesBackupBeforeStartup(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	dataDir := filepath.Join(root, "data")
	backupDir := filepath.Join(root, "backup")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "run.json"), []byte("runtime-v1"), 0o600); err != nil {
		t.Fatal(err)
	}
	config := "[repository]\nbackend = \"file\"\ndataDir = \"" + dataDir + "\"\n"
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	handled, err := prepareRuntimeV1RetirementMaintenance(StartupArgs{
		ConfigDir: configDir,
		CobraArgs: []string{"runtime-v1-retirement", "--apply", "--backup-dir", backupDir},
	})
	if err != nil || handled {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	if value := os.Getenv(runtimeV1RetirementBackupEnv); value != backupDir {
		t.Fatalf("backup env=%q, want %q", value, backupDir)
	}
	if _, err := os.Stat(filepath.Join(backupDir, "manifest.json")); err != nil {
		t.Fatalf("backup manifest: %v", err)
	}
}

func TestPrepareRuntimeV1RetirementMaintenanceBackupFailureStopsStartup(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	config := "[repository]\nbackend = \"file\"\ndataDir = \"" + filepath.Join(root, "missing-data") + "\"\n"
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	handled, err := prepareRuntimeV1RetirementMaintenance(StartupArgs{
		ConfigDir: configDir,
		CobraArgs: []string{"runtime-v1-retirement", "--apply"},
	})
	if err == nil || handled {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	if value := os.Getenv(runtimeV1RetirementBackupEnv); value != "" {
		t.Fatalf("backup env=%q, want empty", value)
	}
}

func TestRuntimeV1RetirementRestoreHintCoversPostBackupStartupFailure(t *testing.T) {
	t.Setenv(runtimeV1RetirementBackupEnv, "/tmp/workorch retirement backup")
	err := runtimeV1RetirementRestoreHint(errors.New("framework startup failed"))
	if err == nil || !strings.Contains(err.Error(), `--restore-from "/tmp/workorch retirement backup"`) {
		t.Fatalf("restore hint error=%v", err)
	}
}
