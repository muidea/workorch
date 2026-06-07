package workspacecore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

func EnsureManagedConfigs(internalDirectory string, contextHints map[string]any) error {
	internalDirectory = strings.TrimSpace(internalDirectory)
	if internalDirectory == "" || len(contextHints) == 0 {
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
		if err := ensureManagedConfig(internalDirectory, config); err != nil {
			return err
		}
	}
	return nil
}

func ensureManagedConfig(internalDirectory string, config map[string]any) error {
	relPath := strings.TrimSpace(stringValueFromMap(config, "path"))
	if relPath == "" {
		return common.ErrInvalidArgument
	}
	cleanRelPath := filepath.Clean(relPath)
	if filepath.IsAbs(cleanRelPath) || cleanRelPath == "." || strings.HasPrefix(cleanRelPath, ".."+string(os.PathSeparator)) || cleanRelPath == ".." {
		return common.ErrInvalidArgument
	}
	target := filepath.Join(internalDirectory, cleanRelPath)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	format := strings.ToLower(strings.TrimSpace(stringValueFromMap(config, "format")))
	switch format {
	case "", "json":
		return ensureManagedJSONConfig(target, config["content"], boolValueFromMap(config, "mergeMissing"))
	default:
		return fmt.Errorf("%w: unsupported workspace managed config format %q", common.ErrInvalidArgument, format)
	}
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
