package shell

import (
	"path/filepath"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func applyShellEvidencePolicies(policies []runtimecontract.RuntimeEvidencePolicy, command string, args []string, output *RuntimeCommandResult) {
	if output == nil {
		return
	}
	for _, policy := range runtimecontract.NormalizeRuntimeEvidencePolicies(policies) {
		if !shellEvidencePolicyApplies(policy, command, args) {
			continue
		}
		if shellEvidencePolicyZeroWork(policy, output) {
			output.EvidenceQuality = firstNonEmptyEvidencePolicyValue(policy.QualityOnZeroWork, string(common.EvidenceQualityInconclusive))
			output.EvidenceReason = firstNonEmptyEvidencePolicyValue(policy.EvidenceReason, "command completed without producing required evidence")
			output.ZeroWork = true
			if tags := policy.EvidenceTags; len(tags) > 0 {
				output.EvidenceTags = append([]string(nil), tags...)
			}
			return
		}
	}
}

func shellEvidencePolicyApplies(policy runtimecontract.RuntimeEvidencePolicy, command string, args []string) bool {
	appliesTo := strings.TrimSpace(policy.AppliesTo)
	if appliesTo != "" && appliesTo != "shell/command" {
		return false
	}
	if expected := strings.TrimSpace(policy.CommandName); expected != "" {
		base := strings.ToLower(filepath.Base(strings.TrimSpace(command)))
		if base != strings.ToLower(expected) && base != strings.ToLower(expected)+".exe" {
			return false
		}
	}
	for _, required := range policy.ArgsContain {
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

func shellEvidencePolicyZeroWork(policy runtimecontract.RuntimeEvidencePolicy, output *RuntimeCommandResult) bool {
	text := strings.ToLower(strings.Join([]string{
		strings.TrimSpace(output.Stdout),
		strings.TrimSpace(output.Stderr),
	}, "\n"))
	if strings.TrimSpace(text) == "" {
		return false
	}
	for _, marker := range policy.WorkEvidenceMarkers {
		if marker != "" && strings.Contains(text, strings.ToLower(marker)) {
			return false
		}
	}
	for _, marker := range policy.ZeroWorkMarkers {
		if marker != "" && strings.Contains(text, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

func firstNonEmptyEvidencePolicyValue(value string, fallback string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return fallback
}
