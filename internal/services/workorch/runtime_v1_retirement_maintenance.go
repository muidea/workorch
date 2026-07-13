package workorch

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/muidea/workorch/internal/services/workorch/biz/retirementbackup"
	"github.com/pelletier/go-toml/v2"
)

const runtimeV1RetirementBackupEnv = "WORKORCH_RUNTIME_V1_RETIREMENT_BACKUP"

type runtimeV1RetirementMaintenanceOptions struct {
	Matched     bool
	Apply       bool
	DryRun      bool
	BackupDir   string
	RestoreFrom string
}

func prepareRuntimeV1RetirementMaintenance(args StartupArgs) (bool, error) {
	options, err := parseRuntimeV1RetirementMaintenanceOptions(args.CobraArgs)
	if err != nil || !options.Matched {
		return false, err
	}
	_ = os.Unsetenv(runtimeV1RetirementBackupEnv)
	if options.RestoreFrom != "" {
		return true, retirementbackup.Restore(options.RestoreFrom)
	}
	if !options.Apply {
		return false, nil
	}
	if args.Mode == ModeRemote {
		return false, fmt.Errorf("Runtime V1 retirement apply is only available in local embedded maintenance mode")
	}
	configDir := ResolveConfigDir(args.ConfigDir)
	if err := EnsureConfigFile(configDir); err != nil {
		return false, err
	}
	cfg, err := loadRuntimeV1RetirementRepositoryConfig(configDir)
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(cfg.Backend) != "file" {
		return false, fmt.Errorf("Runtime V1 retirement apply requires file repository backend for mandatory backup")
	}
	manifest, err := retirementbackup.Create(cfg.DataDir, configDir, options.BackupDir)
	if err != nil {
		return false, err
	}
	backupDir := manifest.BackupDirectory
	if backupDir != "" {
		_ = os.Setenv(runtimeV1RetirementBackupEnv, backupDir)
	}
	return false, nil
}

func parseRuntimeV1RetirementMaintenanceOptions(args []string) (runtimeV1RetirementMaintenanceOptions, error) {
	options := runtimeV1RetirementMaintenanceOptions{}
	for _, arg := range args {
		if arg == "runtime-v1-retirement" {
			options.Matched = true
			break
		}
	}
	if !options.Matched {
		return options, nil
	}
	for idx := 0; idx < len(args); idx++ {
		switch args[idx] {
		case "--apply":
			options.Apply = true
		case "--dry-run":
			options.DryRun = true
		case "--backup-dir":
			if idx+1 >= len(args) {
				return options, fmt.Errorf("--backup-dir requires a value")
			}
			idx++
			options.BackupDir = args[idx]
		case "--restore-from":
			if idx+1 >= len(args) {
				return options, fmt.Errorf("--restore-from requires a value")
			}
			idx++
			options.RestoreFrom = args[idx]
		default:
			if value, ok := strings.CutPrefix(args[idx], "--backup-dir="); ok {
				options.BackupDir = value
			}
			if value, ok := strings.CutPrefix(args[idx], "--restore-from="); ok {
				options.RestoreFrom = value
			}
		}
	}
	if options.Apply && options.DryRun {
		return options, fmt.Errorf("--apply and --dry-run are mutually exclusive")
	}
	if options.RestoreFrom != "" && (options.Apply || options.DryRun || options.BackupDir != "") {
		return options, fmt.Errorf("--restore-from is mutually exclusive with --apply, --dry-run and --backup-dir")
	}
	if options.BackupDir != "" && !options.Apply {
		return options, fmt.Errorf("--backup-dir requires --apply")
	}
	return options, nil
}

type runtimeV1RetirementRepositoryConfig struct {
	Backend string `toml:"backend"`
	DataDir string `toml:"dataDir"`
}

func loadRuntimeV1RetirementRepositoryConfig(configDir string) (runtimeV1RetirementRepositoryConfig, error) {
	payload, err := os.ReadFile(filepath.Join(configDir, "application.toml"))
	if err != nil {
		return runtimeV1RetirementRepositoryConfig{}, err
	}
	var document struct {
		Repository runtimeV1RetirementRepositoryConfig `toml:"repository"`
	}
	if err := toml.Unmarshal(payload, &document); err != nil {
		return runtimeV1RetirementRepositoryConfig{}, err
	}
	if strings.TrimSpace(document.Repository.Backend) == "" || strings.TrimSpace(document.Repository.DataDir) == "" {
		return runtimeV1RetirementRepositoryConfig{}, fmt.Errorf("repository backend and dataDir are required for Runtime V1 retirement backup")
	}
	return document.Repository, nil
}

func runtimeV1RetirementRestoreHint(err error) error {
	if err == nil {
		return nil
	}
	backupDir := strings.TrimSpace(os.Getenv(runtimeV1RetirementBackupEnv))
	if backupDir == "" || strings.Contains(err.Error(), "restore with: workorch runtime-v1-retirement") {
		return err
	}
	return fmt.Errorf("%w; restore with: workorch runtime-v1-retirement --restore-from %q", err, backupDir)
}
