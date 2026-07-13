package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
)

func finalizeManagedValidationResourcesForStep(step *runModel.Step, finalStatus string, finalizedBy string) bool {
	if step == nil || len(step.Input) == 0 {
		return false
	}
	resources := mapSliceFromValue(step.Input["validationResources"])
	if len(resources) == 0 {
		return false
	}
	finalStatus = strings.TrimSpace(finalStatus)
	if finalStatus == "" {
		finalStatus = "released"
	}
	finalizedBy = strings.TrimSpace(finalizedBy)
	if finalizedBy == "" {
		finalizedBy = "runtime_orchestrator"
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	changed := false
	for idx := range resources {
		resource := resources[idx]
		if managedValidationResourceStatusTerminal(stringFromMap(resource, "status")) {
			continue
		}
		resource["status"] = finalStatus
		resource["finalizedAt"] = now
		resource["finalizedBy"] = finalizedBy
		if strings.TrimSpace(stringFromMap(resource, "kind")) == "scratch_dir" {
			if validationScratchResourceRetainedUntilRunTerminal(resource) && !managedValidationFinalizedByRun(finalizedBy) {
				resource["status"] = "retained"
				resource["retainedUntil"] = "run_terminal"
				resource["cleanupStatus"] = "retained_until_run_terminal"
				resources[idx] = resource
				changed = true
				continue
			}
			if cleanupStatus, cleanupErr := cleanupOwnedValidationScratch(resource); cleanupStatus != "" {
				resource["cleanupStatus"] = cleanupStatus
				if cleanupErr != "" {
					resource["cleanupError"] = cleanupErr
					resource["status"] = "cleanup_failed"
				}
			}
		}
		resources[idx] = resource
		changed = true
	}
	if !changed {
		return false
	}
	step.Input["validationResources"] = resources
	if ownership := mapFromValue(step.Input["validationResourceOwnership"]); len(ownership) > 0 {
		ownership["finalized"] = true
		ownership["finalizedAt"] = now
		ownership["finalizedBy"] = finalizedBy
		step.Input["validationResourceOwnership"] = ownership
	}
	if managed := mapFromValue(step.Input["managedValidation"]); len(managed) > 0 {
		managed["resourcesFinalized"] = true
		managed["resourcesFinalizedAt"] = now
		managed["resourcesFinalizedBy"] = finalizedBy
		step.Input["managedValidation"] = managed
	}
	return true
}

func validationScratchResourceRetainedUntilRunTerminal(resource map[string]any) bool {
	switch strings.TrimSpace(stringFromMap(resource, "cleanupPolicy")) {
	case "retain_graph_scoped_scratch_until_run_terminal", "retain_until_run_terminal":
		return true
	default:
		return false
	}
}

func managedValidationFinalizedByRun(finalizedBy string) bool {
	finalizedBy = strings.TrimSpace(finalizedBy)
	return finalizedBy == "run_terminal" || strings.HasPrefix(finalizedBy, "run_")
}

func managedValidationResourceStatusTerminal(status string) bool {
	switch strings.TrimSpace(status) {
	case "released", "skipped", "cleanup_failed", "cleanup_skipped", "removed":
		return true
	default:
		return false
	}
}

func (o *RuntimeOrchestrator) finalizeManagedValidationResourcesForRun(ctx context.Context, run *runModel.Run, finalStatus string, finalizedBy string) error {
	if o == nil || o.steps == nil || run == nil || run.ID == "" {
		return nil
	}
	steps, err := o.steps.ListByRun(ctx, run.ID)
	if err != nil {
		return err
	}
	for _, step := range steps {
		if !finalizeManagedValidationResourcesForStep(step, finalStatus, finalizedBy) {
			continue
		}
		if err := o.steps.Update(ctx, step); err != nil {
			return err
		}
	}
	return nil
}

func cleanupOwnedValidationScratch(resource map[string]any) (string, string) {
	path := strings.TrimSpace(stringFromMap(resource, "path"))
	if path == "" {
		return "", ""
	}
	root := normalizeValidationScratchDir(filepath.Join(os.TempDir(), "workorch-validation-scratch"))
	if !validationScratchDirUnderRoot(path, root) {
		return "skipped_outside_runtime_scratch_root", ""
	}
	if err := os.RemoveAll(path); err != nil {
		return "failed", err.Error()
	}
	return "removed", ""
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
