package shell

import (
	"path/filepath"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

func applyShellEvidencePolicies(input map[string]any, command string, args []string, output map[string]any) {
	if input == nil || output == nil {
		return
	}
	for _, policy := range evidencePolicyMaps(input["evidencePolicies"]) {
		if !shellEvidencePolicyApplies(policy, command, args) {
			continue
		}
		if shellEvidencePolicyZeroWork(policy, output) {
			output["evidenceQuality"] = firstNonEmptyEvidencePolicyValue(policy, "qualityOnZeroWork", string(common.EvidenceQualityInconclusive))
			output["evidenceReason"] = firstNonEmptyEvidencePolicyValue(policy, "evidenceReason", "command completed without producing required evidence")
			output["zeroWork"] = true
			if tags := evidencePolicyStringSlice(policy["evidenceTags"]); len(tags) > 0 {
				output["evidenceTags"] = tags
			}
			return
		}
	}
}

func shellEvidencePolicyApplies(policy map[string]any, command string, args []string) bool {
	appliesTo := strings.TrimSpace(stringFromEvidencePolicy(policy["appliesTo"]))
	if appliesTo != "" && appliesTo != "shell/command" {
		return false
	}
	if expected := strings.TrimSpace(stringFromEvidencePolicy(policy["commandName"])); expected != "" {
		base := strings.ToLower(filepath.Base(strings.TrimSpace(command)))
		if base != strings.ToLower(expected) && base != strings.ToLower(expected)+".exe" {
			return false
		}
	}
	for _, required := range evidencePolicyStringSlice(policy["argsContain"]) {
		found := false
		for _, arg := range args {
			if strings.TrimSpace(arg) == required {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func shellEvidencePolicyZeroWork(policy map[string]any, output map[string]any) bool {
	text := strings.ToLower(strings.Join([]string{
		stringFromEvidencePolicy(output["stdout"]),
		stringFromEvidencePolicy(output["stderr"]),
	}, "\n"))
	if strings.TrimSpace(text) == "" {
		return false
	}
	for _, marker := range evidencePolicyStringSlice(policy["workEvidenceMarkers"]) {
		if marker != "" && strings.Contains(text, strings.ToLower(marker)) {
			return false
		}
	}
	for _, marker := range evidencePolicyStringSlice(policy["zeroWorkMarkers"]) {
		if marker != "" && strings.Contains(text, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

func evidencePolicyMaps(value any) []map[string]any {
	switch typed := value.(type) {
	case []map[string]any:
		return typed
	case []any:
		ret := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			mapped, ok := item.(map[string]any)
			if !ok {
				continue
			}
			ret = append(ret, mapped)
		}
		return ret
	default:
		return nil
	}
}

func evidencePolicyStringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			item = strings.TrimSpace(item)
			if item != "" {
				ret = append(ret, item)
			}
		}
		return ret
	case []any:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			text := strings.TrimSpace(stringFromEvidencePolicy(item))
			if text != "" {
				ret = append(ret, text)
			}
		}
		return ret
	default:
		return nil
	}
}

func firstNonEmptyEvidencePolicyValue(policy map[string]any, key string, fallback string) string {
	if value := strings.TrimSpace(stringFromEvidencePolicy(policy[key])); value != "" {
		return value
	}
	return fallback
}

func stringFromEvidencePolicy(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}
