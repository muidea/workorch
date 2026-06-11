package workspacecore

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

func ensureManagedJSONConfig(target string, content any, mergeMissing bool, overwriteExisting bool) error {
	defaults, ok := content.(map[string]any)
	if !ok {
		return common.ErrInvalidArgument
	}
	if overwriteExisting {
		return writeStableJSONFile(target, defaults)
	}
	if existingRaw, err := os.ReadFile(target); err == nil {
		if !mergeMissing {
			return nil
		}
		existing := map[string]any{}
		if len(strings.TrimSpace(string(existingRaw))) > 0 {
			if err := json.Unmarshal(existingRaw, &existing); err != nil {
				return err
			}
		}
		merged := mergeMissingMap(existing, defaults)
		return writeStableJSONFile(target, merged)
	} else if !os.IsNotExist(err) {
		return err
	}
	return writeStableJSONFile(target, defaults)
}

func mergeMissingMap(existing map[string]any, defaults map[string]any) map[string]any {
	if existing == nil {
		existing = map[string]any{}
	}
	for key, value := range defaults {
		if current, ok := existing[key]; ok {
			currentMap, currentOK := current.(map[string]any)
			defaultMap, defaultOK := value.(map[string]any)
			if currentOK && defaultOK {
				existing[key] = mergeMissingMap(currentMap, defaultMap)
			}
			continue
		}
		existing[key] = cloneContextValue(value)
	}
	return existing
}

func writeStableJSONFile(target string, payload any) error {
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(target, raw, 0o644)
}
