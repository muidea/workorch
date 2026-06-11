package workspacecore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagedconfig"
)

func EnsureManagedConfigs(internalDirectory string, systemContextHints map[string]any, workspaceContextHints ...map[string]any) error {
	internalDirectory = strings.TrimSpace(internalDirectory)
	if internalDirectory == "" {
		return nil
	}
	if len(workspaceContextHints) == 0 {
		return ensureManagedConfigsInRoot(internalDirectory, systemContextHints, managedConfigWritePolicy{})
	}
	if err := ensureManagedConfigsInRoot(SystemManagedConfigDirectory(internalDirectory), systemContextHints, managedConfigWritePolicy{overwriteExisting: true}); err != nil {
		return err
	}
	return ensureManagedConfigsInRoot(internalDirectory, workspaceContextHints[0], managedConfigWritePolicy{})
}

type managedConfigWritePolicy struct {
	overwriteExisting bool
}

func ensureManagedConfigsInRoot(root string, contextHints map[string]any, policy managedConfigWritePolicy) error {
	root = strings.TrimSpace(root)
	if root == "" || len(contextHints) == 0 {
		return nil
	}
	configs, ok := contextHints["managedWorkspaceConfigs"].([]any)
	if !ok || len(configs) == 0 {
		return nil
	}
	for _, raw := range configs {
		config, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if err := ensureManagedConfig(root, config, policy); err != nil {
			return err
		}
	}
	return nil
}

func ensureManagedConfig(root string, config map[string]any, policy managedConfigWritePolicy) error {
	cleanRelPath, err := workorchmanagedconfig.CleanPath(stringValueFromMap(config, "path"))
	if err != nil {
		return err
	}
	target := filepath.Join(root, cleanRelPath)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	format := strings.ToLower(strings.TrimSpace(stringValueFromMap(config, "format")))
	switch format {
	case "", "json":
		return ensureManagedJSONConfig(target, config["content"], boolValueFromMap(config, "mergeMissing"), policy.overwriteExisting)
	default:
		return fmt.Errorf("%w: unsupported workspace managed config format %q", common.ErrInvalidArgument, format)
	}
}

func CleanManagedConfigPath(relPath string) (string, error) {
	return workorchmanagedconfig.CleanPath(relPath)
}

func SystemManagedConfigDirectory(workspaceInternalDirectory string) string {
	return workorchmanagedconfig.SystemDirectory(workspaceInternalDirectory)
}

func ManagedConfigPathForWorkspace(workspaceInternalDirectory string, relPath string) string {
	return workorchmanagedconfig.PathForWorkspace(workspaceInternalDirectory, relPath)
}

func stringValueFromMap(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	value, _ := values[key].(string)
	return value
}

func boolValueFromMap(values map[string]any, key string) bool {
	if values == nil {
		return false
	}
	value, _ := values[key].(bool)
	return value
}
