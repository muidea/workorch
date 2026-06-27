package orchestrator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const validationScratchDirKey = "validationScratchDir"

func ensureValidationScratchContract(input map[string]any, pkg map[string]any, requiredOutput map[string]any) {
	if len(input) == 0 || len(requiredOutput) == 0 {
		return
	}
	scratchDir := validationScratchDirFromMaps(input, pkg, requiredOutput)
	if scratchDir == "" {
		scratchDir = defaultValidationScratchDir(input, pkg)
	}
	if scratchDir == "" {
		return
	}
	scratchDir = normalizeValidationScratchDir(scratchDir)
	input[validationScratchDirKey] = scratchDir
	if pkg != nil {
		pkg[validationScratchDirKey] = scratchDir
	}
	requiredOutput[validationScratchDirKey] = scratchDir
	requiredOutput["controlledSubstituteScratchRule"] = "controlled_substitute shell/command validation may write only under validationScratchDir; workspace deliverable writes remain forbidden and must use patch/apply"
	requiredOutput["validationScratchSetupRule"] = "shell/command validation must create validationScratchDir if needed and keep all controlled-substitute fixtures, caches, copied workspaces, and redirected outputs inside that directory"
}

func ensureConcreteValidationScratchContract(input map[string]any, pkg map[string]any, requiredOutput map[string]any) {
	if !boolValue(requiredOutput["mustProduceConcreteValidationEvidence"]) {
		return
	}
	ensureValidationScratchContract(input, pkg, requiredOutput)
}

func concreteValidationEvidenceRequiredOutputWithScratch(input map[string]any, pkg map[string]any, existing map[string]any) map[string]any {
	required := concreteValidationEvidenceRequiredOutput(existing)
	ensureConcreteValidationScratchContract(input, pkg, required)
	applyRepairCoverageFocusContract(input, pkg, required)
	return required
}

func managedValidationEvidenceRequiredOutputWithScratch(input map[string]any, pkg map[string]any, existing map[string]any) map[string]any {
	required := concreteValidationEvidenceRequiredOutputWithScratch(input, pkg, existing)
	required["validationRoute"] = runtimemodel.ValidationRouteManagedHarness
	required["mustUseManagedHarness"] = true
	required["acceptedKinds"] = []string{managedValidationSessionNodeKind}
	required["acceptedKindsOnly"] = true
	required["allowedOutputShapes"] = []string{"executionGraph_with_managed_validation_session"}
	required["forbiddenOutputShapes"] = []string{"plain_shell_validation", "top_level_shellCommands"}
	required["managedValidation"] = map[string]any{
		"scratchRequired": true,
		"timeoutRequired": true,
		"cleanupPolicy":   "process_group_on_timeout_or_completion",
		"portAllocation":  "optional",
	}
	required["managedValidationRule"] = "return validation/managed_session only; runtime owns scratch, timeout, cleanup, and optional port allocation for this retry"
	delete(required, "validationEvidenceNodeExample")
	return required
}

func validationScratchDirFromStep(step *runModel.Step) string {
	if step == nil {
		return ""
	}
	input := step.Input
	pkg := mapFromValue(input["repairActionPackage"])
	required := mapFromValue(pkg["requiredOutput"])
	return validationScratchDirFromMaps(input, pkg, required)
}

func validationScratchDirFromMaps(input map[string]any, pkg map[string]any, requiredOutput map[string]any) string {
	for _, candidate := range []string{
		stringFromMap(input, validationScratchDirKey),
		stringFromMap(pkg, validationScratchDirKey),
		stringFromMap(requiredOutput, validationScratchDirKey),
	} {
		if normalized := normalizeValidationScratchDir(candidate); normalized != "" {
			return normalized
		}
	}
	return ""
}

func ensureRuntimeValidationScratchDir(input map[string]any) (string, error) {
	scratchDir := validationScratchDirFromMaps(
		input,
		mapFromValue(input["repairActionPackage"]),
		mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"]),
	)
	if scratchDir == "" {
		return "", nil
	}
	root := normalizeValidationScratchDir(filepath.Join(os.TempDir(), "workorch-validation-scratch"))
	if !validationScratchDirUnderRoot(scratchDir, root) {
		return "", fmt.Errorf("%w: validationScratchDir must stay under runtime validation scratch root", common.ErrInvalidArgument)
	}
	if err := os.MkdirAll(scratchDir, 0o755); err != nil {
		return "", err
	}
	input[validationScratchDirKey] = scratchDir
	return scratchDir, nil
}

func validationScratchDirUnderRoot(scratchDir string, root string) bool {
	scratchDir = normalizeValidationScratchDir(scratchDir)
	root = normalizeValidationScratchDir(root)
	if scratchDir == "" || root == "" || scratchDir == root {
		return false
	}
	rel, err := filepath.Rel(root, scratchDir)
	if err != nil || rel == "." || rel == "" {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func defaultValidationScratchDir(input map[string]any, pkg map[string]any) string {
	workspaceID := firstNonEmptyString(
		stringFromMap(input, "workspaceId"),
		stringFromMap(input, "workspaceID"),
		stringFromMap(pkg, "workspaceId"),
		stringFromMap(pkg, "workspaceID"),
		"workspace",
	)
	runID := firstNonEmptyString(
		stringFromMap(input, "runId"),
		stringFromMap(input, "runID"),
		stringFromMap(pkg, "runId"),
		stringFromMap(pkg, "runID"),
		"run",
	)
	stepID := firstNonEmptyString(
		stringFromMap(input, "failedStepId"),
		stringFromMap(input, "stepId"),
		stringFromMap(pkg, "failedStepId"),
		stringFromMap(pkg, "stepId"),
		"step",
	)
	return filepath.Join(os.TempDir(), "workorch-validation-scratch", safeValidationScratchPathComponent(workspaceID), safeValidationScratchPathComponent(runID), safeValidationScratchPathComponent(stepID))
}

func safeValidationScratchPathComponent(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	var builder strings.Builder
	builder.Grow(len(value))
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			builder.WriteRune(r)
		default:
			builder.WriteRune('_')
		}
	}
	ret := strings.Trim(builder.String(), "._-")
	if ret == "" {
		return "unknown"
	}
	if len(ret) > 96 {
		return ret[:96]
	}
	return ret
}

func normalizeValidationScratchDir(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	cleaned := filepath.Clean(value)
	if !filepath.IsAbs(cleaned) {
		return ""
	}
	normalized := filepath.ToSlash(cleaned)
	if normalized == "/" {
		return ""
	}
	return normalized
}
