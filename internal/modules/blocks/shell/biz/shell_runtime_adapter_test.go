package shell

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeAdapter_ExecuteRuntimeStepRunsCommandInWorkspaceDirectory(t *testing.T) {
	adapter := NewRuntimeAdapter()
	deliverableDirectory := t.TempDir()

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Kind:                          "shell/command",
		Input: map[string]any{
			"command": "pwd",
		},
	})
	if err != nil {
		t.Fatalf("ExecuteRuntimeStep: %v", err)
	}
	if result.Output["exitCode"] != 0 {
		t.Fatalf("exitCode = %v, want 0", result.Output["exitCode"])
	}
	stdout, _ := result.Output["stdout"].(string)
	if strings.TrimSpace(stdout) != deliverableDirectory {
		t.Fatalf("stdout = %q, want deliverable directory %q", stdout, deliverableDirectory)
	}
}

func TestRuntimeAdapter_ExecuteRuntimeStepUsesDefaultCommandTimeout(t *testing.T) {
	adapter := NewRuntimeAdapter()
	deliverableDirectory := t.TempDir()

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Kind:                          "shell/command",
		Input: map[string]any{
			"command": "pwd",
		},
	})
	if err != nil {
		t.Fatalf("ExecuteRuntimeStep: %v", err)
	}
	if result.Output["timeoutMillis"] != int64(DefaultCommandTimeout/time.Millisecond) {
		t.Fatalf("timeoutMillis = %v, want %d", result.Output["timeoutMillis"], DefaultCommandTimeout/time.Millisecond)
	}
}

func TestNewRuntimeAdapterFromTimeoutString(t *testing.T) {
	adapter, err := NewRuntimeAdapterFromTimeoutString("90s")
	if err != nil {
		t.Fatalf("NewRuntimeAdapterFromTimeoutString: %v", err)
	}
	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: t.TempDir(),
		Kind:                          "shell/command",
		Input: map[string]any{
			"command": "pwd",
		},
	})
	if err != nil {
		t.Fatalf("ExecuteRuntimeStep: %v", err)
	}
	if result.Output["timeoutMillis"] != int64(90000) {
		t.Fatalf("timeoutMillis = %v, want 90000", result.Output["timeoutMillis"])
	}
}

func TestNewRuntimeAdapterFromTimeoutStringRejectsOutOfRange(t *testing.T) {
	if _, err := NewRuntimeAdapterFromTimeoutString("500ms"); err == nil {
		t.Fatal("NewRuntimeAdapterFromTimeoutString err = nil, want invalid argument")
	}
	if _, err := NewRuntimeAdapterFromTimeoutString("11m"); err == nil {
		t.Fatal("NewRuntimeAdapterFromTimeoutString err = nil, want invalid argument")
	}
}

func TestNewRuntimeAdapterFromTimeoutStringAllowsTenMinutes(t *testing.T) {
	adapter, err := NewRuntimeAdapterFromTimeoutString("10m")
	if err != nil {
		t.Fatalf("NewRuntimeAdapterFromTimeoutString: %v", err)
	}
	timeout, err := shellCommandTimeout(nil, adapter.timeout)
	if err != nil {
		t.Fatalf("shellCommandTimeout: %v", err)
	}
	if timeout != 10*time.Minute {
		t.Fatalf("timeout = %s, want 10m", timeout)
	}
}

func TestRuntimeAdapter_ExecuteRuntimeStepCreatesMissingWorkspaceDirectory(t *testing.T) {
	adapter := NewRuntimeAdapter()
	deliverableDirectory := filepath.Join(t.TempDir(), "missing", "workspace")

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Kind:                          "shell/command",
		Input: map[string]any{
			"command": "pwd",
		},
	})
	if err != nil {
		t.Fatalf("ExecuteRuntimeStep: %v", err)
	}
	if result.Output["exitCode"] != 0 {
		t.Fatalf("exitCode = %v, want 0", result.Output["exitCode"])
	}
	info, err := os.Stat(deliverableDirectory)
	if err != nil {
		t.Fatalf("stat deliverable directory: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("deliverableDirectory is not a directory")
	}
}

func TestRuntimeAdapter_ExecuteRuntimeStepReturnsOutputForNonZeroExit(t *testing.T) {
	adapter := NewRuntimeAdapter()
	deliverableDirectory := t.TempDir()

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Kind:                          "shell/command",
		Input: map[string]any{
			"command": "sh",
			"args":    []any{"-c", "echo shell-out; echo shell-err >&2; exit 7"},
		},
	})
	if err == nil {
		t.Fatal("ExecuteRuntimeStep err = nil, want non-zero exit error")
	}
	if result.Output["status"] != "failed" {
		t.Fatalf("status = %v, want failed", result.Output["status"])
	}
	if result.Output["exitCode"] != 7 {
		t.Fatalf("exitCode = %v, want 7", result.Output["exitCode"])
	}
	stdout, _ := result.Output["stdout"].(string)
	stderr, _ := result.Output["stderr"].(string)
	if !strings.Contains(stdout, "shell-out") || !strings.Contains(stderr, "shell-err") {
		t.Fatalf("stdout/stderr = %q/%q, want captured command output", stdout, stderr)
	}
}

func TestRuntimeAdapter_ExecuteRuntimeStepAddsValidationFailureDiagnostic(t *testing.T) {
	adapter := NewRuntimeAdapter()
	deliverableDirectory := t.TempDir()

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Kind:                          "shell/command",
		Input: map[string]any{
			"command": "sh",
			"args":    []any{"-c", "echo assertion failed >&2; exit 3"},
			"validationEvidence": map[string]any{
				"mode":            "runtime_behavior",
				"observationType": "test_result",
				"targetRefs":      []any{"feature-contract"},
			},
		},
	})
	if err == nil {
		t.Fatal("ExecuteRuntimeStep err = nil, want non-zero exit error")
	}
	diagnostic, ok := result.Output["validationFailureDiagnostic"].(map[string]any)
	if !ok {
		t.Fatalf("validationFailureDiagnostic = %T, want map", result.Output["validationFailureDiagnostic"])
	}
	if diagnostic["class"] != runtimemodel.ValidationDiagnosticTestFailed ||
		diagnostic["recommendedEvidenceTargetType"] != runtimemodel.ValidationEvidenceTargetBehaviorEvidence {
		t.Fatalf("validationFailureDiagnostic = %+v, want test failure behavior diagnostic", diagnostic)
	}
	refs, _ := diagnostic["targetRefs"].([]string)
	if len(refs) != 1 || refs[0] != "feature-contract" {
		t.Fatalf("targetRefs = %#v, want feature-contract", diagnostic["targetRefs"])
	}
	runtimeDiagnostics, _ := result.Output["runtimeDiagnostics"].(map[string]any)
	if _, ok := runtimeDiagnostics["validationFailureDiagnostic"].(map[string]any); !ok {
		t.Fatalf("runtimeDiagnostics = %+v, want nested validation diagnostic", runtimeDiagnostics)
	}
}

func TestRuntimeAdapter_ExecuteRuntimeStepRejectsValidationShellMaskedExitStatus(t *testing.T) {
	adapter := NewRuntimeAdapter()
	deliverableDirectory := t.TempDir()

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Kind:                          "shell/command",
		Input: map[string]any{
			"command": "sh",
			"args":    []any{"-c", "false; echo EXIT_CODE=$?"},
			"validationEvidence": map[string]any{
				"mode":            "runtime_behavior",
				"observationType": "runtime_observation",
				"targetRefs":      []any{"feature-contract"},
				"observedClaims":  []any{"feature contract was validated"},
			},
		},
	})
	if err == nil {
		t.Fatal("ExecuteRuntimeStep err = nil, want masked validation exit status rejection")
	}
	if result.Output["status"] != "failed" ||
		result.Output["stage"] != "shell_validation_contract_rejected" ||
		result.Output["failureReason"] != "validation_shell_masks_failure" {
		t.Fatalf("output = %+v, want validation shell contract rejection", result.Output)
	}
	diagnostic, ok := result.Output["validationFailureDiagnostic"].(map[string]any)
	if !ok {
		t.Fatalf("validationFailureDiagnostic = %T, want map", result.Output["validationFailureDiagnostic"])
	}
	if diagnostic["class"] != runtimemodel.ValidationDiagnosticCommandInvalid {
		t.Fatalf("validationFailureDiagnostic = %+v, want command invalid", diagnostic)
	}
}

func TestRuntimeAdapter_ExecuteRuntimeStepRejectsValidationShellFallbackMaskedFailure(t *testing.T) {
	adapter := NewRuntimeAdapter()
	deliverableDirectory := t.TempDir()

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Kind:                          "shell/command",
		Input: map[string]any{
			"command": "sh",
			"args":    []any{"-c", "command -v tool && false 2>&1 || echo 'failed'"},
			"validationEvidence": map[string]any{
				"mode":            "runtime_behavior",
				"observationType": "runtime_observation",
				"targetRefs":      []any{"feature-contract"},
				"observedClaims":  []any{"feature contract was validated"},
			},
		},
	})
	if err == nil {
		t.Fatal("ExecuteRuntimeStep err = nil, want fallback-masked validation failure rejection")
	}
	if result.Output["status"] != "failed" ||
		result.Output["stage"] != "shell_validation_contract_rejected" ||
		result.Output["failureReason"] != "validation_shell_masks_failure" {
		t.Fatalf("output = %+v, want validation shell contract rejection", result.Output)
	}
	diagnostic, ok := result.Output["validationFailureDiagnostic"].(map[string]any)
	if !ok {
		t.Fatalf("validationFailureDiagnostic = %T, want map", result.Output["validationFailureDiagnostic"])
	}
	if diagnostic["class"] != runtimemodel.ValidationDiagnosticCommandInvalid {
		t.Fatalf("validationFailureDiagnostic = %+v, want command invalid", diagnostic)
	}
}

func TestRuntimeAdapter_ExecuteRuntimeStepCarriesManagedValidationMetadata(t *testing.T) {
	adapter := NewRuntimeAdapter()
	deliverableDirectory := t.TempDir()
	scratchDirectory := filepath.Join(os.TempDir(), "workorch-shell-managed-validation-test", strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()))
	t.Cleanup(func() {
		_ = os.RemoveAll(scratchDirectory)
	})

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceID:                   "ws-managed-validation",
		RunID:                         "run-managed-validation",
		StepID:                        "run-managed-validation/verify",
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Kind:                          "shell/command",
		Input: map[string]any{
			"command":              "sh",
			"args":                 []any{"-c", "mkdir -p \"$1\"; printf managed-ok > \"$1/out.txt\"; printf managed-ok", "sh", scratchDirectory},
			"validationScratchDir": scratchDirectory,
			"validationEvidence": map[string]any{
				"mode":            "runtime_behavior",
				"observationType": "runtime_observation",
			},
			"managedValidation": map[string]any{
				"runtimeManaged": true,
				"autoCleanup":    true,
				"cleanupPolicy":  "process_group_on_timeout_or_completion",
			},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteRuntimeStep: %v", err)
	}
	managed, _ := result.Output["managedValidation"].(map[string]any)
	if managed["runtimeManaged"] != true ||
		managed["autoCleanup"] != true ||
		managed["cleanupPolicy"] != "process_group_on_timeout_or_completion" {
		t.Fatalf("managedValidation = %+v, want carried runtime-managed metadata", managed)
	}
	evidence, _ := result.Output["validationEvidence"].(map[string]any)
	if evidence["mode"] != "runtime_behavior" || evidence["observationType"] != "runtime_observation" {
		t.Fatalf("validationEvidence = %+v, want carried validation evidence", evidence)
	}
	if managed["resourceCount"] != 2 || managed["resourcesReleased"] != true {
		t.Fatalf("managedValidation = %+v, want released process and scratch resources", managed)
	}
	resources, ok := result.Output["validationResources"].([]map[string]any)
	if !ok || len(resources) != 2 {
		t.Fatalf("validationResources = %#v, want two resource records", result.Output["validationResources"])
	}
	kinds := map[string]string{}
	for _, resource := range resources {
		kinds[stringFromShellValue(resource["kind"])] = stringFromShellValue(resource["status"])
		if resource["workspaceId"] != "ws-managed-validation" ||
			resource["runId"] != "run-managed-validation" ||
			resource["stepId"] != "run-managed-validation/verify" {
			t.Fatalf("resource = %+v, want scoped workspace/run/step ids", resource)
		}
	}
	if kinds["process"] != "released" || kinds["scratch_dir"] != "released" {
		t.Fatalf("resource statuses = %+v, want released process and scratch_dir", kinds)
	}
	if _, statErr := os.Stat(scratchDirectory); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("scratchDirectory stat = %v, want removed by managed validation cleanup", statErr)
	}
}

func TestRuntimeAdapter_ExecuteRuntimeStepReleasesOrdinaryValidationScratch(t *testing.T) {
	adapter := NewRuntimeAdapter()
	deliverableDirectory := t.TempDir()
	scratchDirectory := filepath.Join(os.TempDir(), "workorch-shell-validation-scratch-test", strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()))
	t.Cleanup(func() {
		_ = os.RemoveAll(scratchDirectory)
	})

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceID:                   "ws-validation-scratch",
		RunID:                         "run-validation-scratch",
		StepID:                        "run-validation-scratch/verify",
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Kind:                          "shell/command",
		Input: map[string]any{
			"command":              "sh",
			"args":                 []any{"-c", "mkdir -p \"$1\"; printf ordinary-ok > \"$1/out.txt\"; printf ordinary-ok", "sh", scratchDirectory},
			"validationScratchDir": scratchDirectory,
			"validationEvidence": map[string]any{
				"mode":            "runtime_behavior",
				"observationType": "runtime_observation",
			},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteRuntimeStep: %v", err)
	}
	if _, ok := result.Output["managedValidation"].(map[string]any); ok {
		t.Fatalf("managedValidation = %+v, want ordinary validation lifecycle without managed metadata", result.Output["managedValidation"])
	}
	resources, ok := result.Output["validationResources"].([]map[string]any)
	if !ok || len(resources) != 2 {
		t.Fatalf("validationResources = %#v, want process and scratch resource records", result.Output["validationResources"])
	}
	statuses := map[string]string{}
	for _, resource := range resources {
		statuses[stringFromShellValue(resource["kind"])] = stringFromShellValue(resource["status"])
		if resource["workspaceId"] != "ws-validation-scratch" ||
			resource["runId"] != "run-validation-scratch" ||
			resource["stepId"] != "run-validation-scratch/verify" {
			t.Fatalf("resource = %+v, want scoped workspace/run/step ids", resource)
		}
	}
	if statuses["process"] != "released" || statuses["scratch_dir"] != "released" {
		t.Fatalf("resource statuses = %+v, want released process and scratch_dir", statuses)
	}
	runtimeDiagnostics, _ := result.Output["runtimeDiagnostics"].(map[string]any)
	lifecycle, _ := runtimeDiagnostics["validationResourceLifecycle"].(map[string]any)
	if lifecycle["released"] != true {
		t.Fatalf("runtimeDiagnostics = %+v, want released ordinary validation lifecycle", runtimeDiagnostics)
	}
	if _, statErr := os.Stat(scratchDirectory); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("scratchDirectory stat = %v, want removed by ordinary validation scratch cleanup", statErr)
	}
}

func TestRuntimeAdapter_ExecuteRuntimeStepRetainsValidationScratchWhenAutoCleanupDisabled(t *testing.T) {
	adapter := NewRuntimeAdapter()
	deliverableDirectory := t.TempDir()
	scratchDirectory := filepath.Join(os.TempDir(), "workorch-shell-validation-scratch-test", strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()))
	t.Cleanup(func() {
		_ = os.RemoveAll(scratchDirectory)
	})

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceID:                   "ws-validation-scratch-retain",
		RunID:                         "run-validation-scratch-retain",
		StepID:                        "run-validation-scratch-retain/verify",
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Kind:                          "shell/command",
		Input: map[string]any{
			"command":                      "sh",
			"args":                         []any{"-c", "mkdir -p \"$1\"; printf retained-ok > \"$1/out.txt\"; printf retained-ok", "sh", scratchDirectory},
			"validationScratchDir":         scratchDirectory,
			"validationScratchAutoCleanup": false,
			"validationEvidence": map[string]any{
				"mode":            "runtime_behavior",
				"observationType": "runtime_observation",
			},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteRuntimeStep: %v", err)
	}
	resources, ok := result.Output["validationResources"].([]map[string]any)
	if !ok || len(resources) != 2 {
		t.Fatalf("validationResources = %#v, want process and scratch resource records", result.Output["validationResources"])
	}
	statuses := map[string]string{}
	for _, resource := range resources {
		statuses[stringFromShellValue(resource["kind"])] = stringFromShellValue(resource["status"])
	}
	if statuses["scratch_dir"] != "retained" {
		t.Fatalf("resource statuses = %+v, want retained scratch_dir", statuses)
	}
	if _, statErr := os.Stat(filepath.Join(scratchDirectory, "out.txt")); statErr != nil {
		t.Fatalf("scratch output stat = %v, want retained", statErr)
	}
}

func TestRuntimeAdapter_ExecuteRuntimeStepClassifiesUnavailableCommand(t *testing.T) {
	adapter := NewRuntimeAdapter()
	deliverableDirectory := t.TempDir()
	command := "workorch-command-that-should-not-exist"

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Kind:                          "shell/command",
		Input: map[string]any{
			"command": command,
		},
	})
	if err == nil {
		t.Fatal("ExecuteRuntimeStep err = nil, want command lookup failure")
	}
	if result.Output["status"] != "failed" {
		t.Fatalf("status = %v, want failed", result.Output["status"])
	}
	if result.Output["failureReason"] != "command_not_found" || result.Output["unavailableCommand"] != command || result.Output["capabilityUnavailable"] != true {
		t.Fatalf("output = %+v, want command-not-found metadata", result.Output)
	}
	diagnostics, ok := result.Output["runtimeDiagnostics"].(map[string]any)
	if !ok {
		t.Fatalf("runtimeDiagnostics = %T, want map", result.Output["runtimeDiagnostics"])
	}
	if diagnostics["kind"] != "runtime_capability_unavailable" || diagnostics["unavailableCommand"] != command {
		t.Fatalf("runtimeDiagnostics = %+v, want runtime capability unavailable", diagnostics)
	}
	validationDiagnostic, ok := result.Output["validationFailureDiagnostic"].(map[string]any)
	if !ok {
		t.Fatalf("validationFailureDiagnostic = %T, want map", result.Output["validationFailureDiagnostic"])
	}
	if validationDiagnostic["class"] != runtimemodel.ValidationDiagnosticEnvironmentMissing {
		t.Fatalf("validationFailureDiagnostic = %+v, want environment missing", validationDiagnostic)
	}
}

func TestRuntimeAdapter_ExecuteRuntimeStepReturnsPolicyDiagnostic(t *testing.T) {
	adapter := NewRuntimeAdapter()

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: t.TempDir(),
		Kind:                          "shell/command",
		Input: map[string]any{
			"command": "kill",
			"args":    []any{"123"},
		},
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("ExecuteRuntimeStep err = %v, want invalid argument", err)
	}
	if result.Output["stage"] != "shell_command_policy_rejected" {
		t.Fatalf("stage = %v, want shell_command_policy_rejected", result.Output["stage"])
	}
	diagnostic, ok := result.Output["validationFailureDiagnostic"].(map[string]any)
	if !ok {
		t.Fatalf("validationFailureDiagnostic = %T, want map", result.Output["validationFailureDiagnostic"])
	}
	if diagnostic["class"] != runtimemodel.ValidationDiagnosticCommandInvalid ||
		diagnostic["commandPolicyViolation"] != true ||
		diagnostic["recommendedEvidenceTargetType"] != runtimemodel.ValidationEvidenceTargetCommandContract {
		t.Fatalf("validationFailureDiagnostic = %+v, want command invalid policy diagnostic", diagnostic)
	}
}

func TestRuntimeAdapter_ExecuteRuntimeStepReleasesManagedValidationScratchOnPolicyReject(t *testing.T) {
	adapter := NewRuntimeAdapter()
	deliverableDirectory := t.TempDir()
	scratchDirectory := filepath.Join(os.TempDir(), "workorch-shell-managed-validation-test", strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()))
	if err := os.MkdirAll(scratchDirectory, 0o755); err != nil {
		t.Fatalf("mkdir scratch: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(scratchDirectory)
	})

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceID:                   "ws-managed-validation-policy-reject",
		RunID:                         "run-managed-validation-policy-reject",
		StepID:                        "run-managed-validation-policy-reject/verify",
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Kind:                          "shell/command",
		Input: map[string]any{
			"command":              "kill",
			"args":                 []any{"123"},
			"validationScratchDir": scratchDirectory,
			"managedValidation": map[string]any{
				"runtimeManaged": true,
				"autoCleanup":    true,
			},
		},
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("ExecuteRuntimeStep err = %v, want invalid argument", err)
	}
	if result.Output["stage"] != "shell_command_policy_rejected" {
		t.Fatalf("stage = %v, want shell_command_policy_rejected", result.Output["stage"])
	}
	resources, ok := result.Output["validationResources"].([]map[string]any)
	if !ok || len(resources) != 1 {
		t.Fatalf("validationResources = %#v, want released scratch resource", result.Output["validationResources"])
	}
	if resources[0]["kind"] != "scratch_dir" || resources[0]["status"] != "released" {
		t.Fatalf("resource = %+v, want released scratch_dir", resources[0])
	}
	managed, _ := result.Output["managedValidation"].(map[string]any)
	if managed["resourcesReleased"] != true {
		t.Fatalf("managedValidation = %+v, want resources released", managed)
	}
	if _, statErr := os.Stat(scratchDirectory); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("scratchDirectory stat = %v, want removed by policy rejection cleanup", statErr)
	}
}

func TestRuntimeAdapter_ExecuteRuntimeStepAllowsManagedValidationProcessLifecycle(t *testing.T) {
	adapter := NewRuntimeAdapter()
	deliverableDirectory := t.TempDir()
	scratchDirectory := filepath.Join(os.TempDir(), "workorch-shell-managed-validation-test", strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()))
	if err := os.MkdirAll(scratchDirectory, 0o755); err != nil {
		t.Fatalf("mkdir scratch: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(scratchDirectory)
	})

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceID:                   "ws-managed-validation-process",
		RunID:                         "run-managed-validation-process",
		StepID:                        "run-managed-validation-process/verify",
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Kind:                          "shell/command",
		Input: map[string]any{
			"command":              "sh",
			"args":                 []any{"-c", "sleep 10 & pid=$!; sleep 0.1; kill $pid; wait $pid 2>/dev/null || true; printf managed-ok"},
			"validationScratchDir": scratchDirectory,
			"managedValidation": map[string]any{
				"runtimeManaged": true,
				"autoCleanup":    true,
			},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteRuntimeStep err = %v, want managed validation process lifecycle allowed", err)
	}
	if result.Output["status"] != "completed" || !strings.Contains(fmt.Sprint(result.Output["stdout"]), "managed-ok") {
		t.Fatalf("output = %+v, want completed managed validation shell", result.Output)
	}
	managed, _ := result.Output["managedValidation"].(map[string]any)
	if managed["resourcesReleased"] != true {
		t.Fatalf("managedValidation = %+v, want resources released", managed)
	}
}

func TestRuntimeAdapter_ExecuteRuntimeStepRejectsManagedValidationGlobalProcessTermination(t *testing.T) {
	adapter := NewRuntimeAdapter()
	_, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: t.TempDir(),
		Kind:                          "shell/command",
		Input: map[string]any{
			"command": "sh",
			"args":    []any{"-c", "pkill -f workorch || true"},
			"managedValidation": map[string]any{
				"runtimeManaged": true,
			},
		},
	})
	if err == nil {
		t.Fatal("ExecuteRuntimeStep err = nil, want managed validation to reject global process termination")
	}
}

func TestRuntimeAdapter_ExecuteRuntimeStepRejectsManagedValidationBackgroundWithoutWait(t *testing.T) {
	adapter := NewRuntimeAdapter()
	_, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: t.TempDir(),
		Kind:                          "shell/command",
		Input: map[string]any{
			"command": "sh",
			"args":    []any{"-c", "sleep 10 & printf started"},
			"managedValidation": map[string]any{
				"runtimeManaged": true,
			},
		},
	})
	if err == nil {
		t.Fatal("ExecuteRuntimeStep err = nil, want managed validation to reject background process without wait")
	}
}

func TestRuntimeAdapter_ExecuteRuntimeStepBoundsCommandOutput(t *testing.T) {
	adapter := NewRuntimeAdapter()
	deliverableDirectory := t.TempDir()

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Kind:                          "shell/command",
		Input: map[string]any{
			"command": "sh",
			"args":    []any{"-c", "yes x | head -c 70000"},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteRuntimeStep: %v", err)
	}
	stdout, _ := result.Output["stdout"].(string)
	if len([]byte(stdout)) != MaxCommandOutputBytes {
		t.Fatalf("stdout bytes = %d, want %d", len([]byte(stdout)), MaxCommandOutputBytes)
	}
	if result.Output["outputTruncated"] != true || result.Output["stdoutTruncated"] != true {
		t.Fatalf("output = %+v, want stdout truncation metadata", result.Output)
	}
	if result.Output["stdoutBytes"] != 70000 {
		t.Fatalf("stdoutBytes = %v, want 70000", result.Output["stdoutBytes"])
	}
}

func TestRuntimeAdapterAppliesConfiguredZeroWorkEvidencePolicy(t *testing.T) {
	adapter := NewRuntimeAdapter()
	deliverableDirectory := t.TempDir()
	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Kind:                          "shell/command",
		Input: map[string]any{
			"command": "sh",
			"args":    []any{"-c", "printf 'testing: warning: no tests to run\\nPASS\\n'"},
			"evidencePolicies": []map[string]any{
				{
					"appliesTo":         "shell/command",
					"commandName":       "sh",
					"argsContain":       []string{"-c"},
					"zeroWorkMarkers":   []string{"no tests to run"},
					"qualityOnZeroWork": string(common.EvidenceQualityInconclusive),
					"evidenceReason":    "configured zero-work marker matched",
					"evidenceTags":      []string{"test", "zero_work"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteRuntimeStep: %v", err)
	}
	if result.Output["evidenceQuality"] != string(common.EvidenceQualityInconclusive) ||
		result.Output["zeroWork"] != true ||
		result.Output["evidenceReason"] != "configured zero-work marker matched" {
		t.Fatalf("output = %+v, want configured inconclusive zero-work evidence", result.Output)
	}
}

func TestRuntimeAdapterPrefersWorkEvidenceMarkersOverZeroWorkMarkers(t *testing.T) {
	adapter := NewRuntimeAdapter()
	deliverableDirectory := t.TempDir()
	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Kind:                          "shell/command",
		Input: map[string]any{
			"command": "sh",
			"args":    []any{"-c", "printf '? pkg [no test files]\\n=== RUN   TestExample\\n--- PASS: TestExample (0.00s)\\nok  pkg 0.001s\\n'"},
			"evidencePolicies": []map[string]any{
				{
					"appliesTo":           "shell/command",
					"commandName":         "sh",
					"argsContain":         []string{"-c"},
					"zeroWorkMarkers":     []string{"[no test files]"},
					"workEvidenceMarkers": []string{"=== RUN", "--- PASS:", "ok  "},
					"qualityOnZeroWork":   string(common.EvidenceQualityInconclusive),
					"evidenceReason":      "configured zero-work marker matched",
					"evidenceTags":        []string{"test", "zero_work"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteRuntimeStep: %v", err)
	}
	if result.Output["zeroWork"] == true {
		t.Fatalf("output = %+v, want work evidence to override zero-work marker", result.Output)
	}
	if result.Output["evidenceQuality"] == string(common.EvidenceQualityInconclusive) {
		t.Fatalf("output = %+v, want evidence quality not inconclusive", result.Output)
	}
}

func TestSanitizeSuccessfulCommandStderrStripsCurlProgressMeter(t *testing.T) {
	progress := "  % Total    % Received % Xferd  Average Speed   Time    Time     Time  Current\n" +
		"                                 Dload  Upload   Total   Spent    Left  Speed\n" +
		"\r100    10  100    10    0     0     33      0 --:--:-- --:--:-- --:--:--    33\n"
	got := sanitizeSuccessfulCommandStderr("curl", []string{"wttr.in/Hangzhou?format=%C+%t"}, 0, "Clear +21C\n", progress)
	if got != "" {
		t.Fatalf("stderr = %q, want curl progress meter stripped", got)
	}
}

func TestSanitizeSuccessfulCommandStderrPreservesRealCurlWarnings(t *testing.T) {
	progress := "  % Total    % Received % Xferd  Average Speed   Time    Time     Time  Current\n" +
		"curl: warning: example warning\n" +
		"\r100    10  100    10    0     0     33      0 --:--:-- --:--:-- --:--:--    33\n"
	got := sanitizeSuccessfulCommandStderr("sh", []string{"-c", "curl http://example.test"}, 0, "ok\n", progress)
	if got != "curl: warning: example warning" {
		t.Fatalf("stderr = %q, want non-progress curl stderr preserved", got)
	}
}

func TestSanitizeSuccessfulCommandStderrPreservesFailedCommandStderr(t *testing.T) {
	progress := "  % Total    % Received % Xferd  Average Speed   Time    Time     Time  Current\ncurl: (6) Could not resolve host\n"
	got := sanitizeSuccessfulCommandStderr("curl", []string{"http://missing.invalid"}, 6, "", progress)
	if got != progress {
		t.Fatalf("stderr = %q, want failed command stderr unchanged", got)
	}
}

func TestRuntimeAdapter_ExecuteRuntimeStepReturnsTimeoutOutput(t *testing.T) {
	adapter := NewRuntimeAdapterWithTimeout(10 * time.Millisecond)
	deliverableDirectory := t.TempDir()

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Kind:                          "shell/command",
		Input: map[string]any{
			"command": "sh",
			"args":    []any{"-c", "sleep 1"},
		},
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ExecuteRuntimeStep err = %v, want deadline exceeded", err)
	}
	if result.Output["status"] != "timeout" {
		t.Fatalf("status = %v, want timeout", result.Output["status"])
	}
	if result.Output["workingDir"] != deliverableDirectory {
		t.Fatalf("workingDir = %v, want %s", result.Output["workingDir"], deliverableDirectory)
	}
	diagnostic, ok := result.Output["validationFailureDiagnostic"].(map[string]any)
	if !ok {
		t.Fatalf("validationFailureDiagnostic = %T, want map", result.Output["validationFailureDiagnostic"])
	}
	if diagnostic["class"] != runtimemodel.ValidationDiagnosticTimeout || diagnostic["timedOut"] != true {
		t.Fatalf("validationFailureDiagnostic = %+v, want timeout diagnostic", diagnostic)
	}
}

func TestRuntimeAdapter_ExecuteRuntimeStepTimeoutKillsProcessGroup(t *testing.T) {
	adapter := NewRuntimeAdapterWithTimeout(50 * time.Millisecond)
	deliverableDirectory := t.TempDir()

	startedAt := time.Now()
	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Kind:                          "shell/command",
		Input: map[string]any{
			"command": "sh",
			"args":    []any{"-c", "sleep 5 & wait"},
		},
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ExecuteRuntimeStep err = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(startedAt); elapsed > time.Second {
		t.Fatalf("ExecuteRuntimeStep elapsed = %s, want process group timeout without waiting for background child", elapsed)
	}
	if result.Output["status"] != "timeout" {
		t.Fatalf("status = %v, want timeout", result.Output["status"])
	}
}

func TestRuntimeAdapter_ExecuteRuntimeStepReleasesManagedValidationResourcesOnTimeout(t *testing.T) {
	adapter := NewRuntimeAdapterWithTimeout(50 * time.Millisecond)
	deliverableDirectory := t.TempDir()
	scratchDirectory := filepath.Join(os.TempDir(), "workorch-shell-managed-validation-test", strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()))
	t.Cleanup(func() {
		_ = os.RemoveAll(scratchDirectory)
	})

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceID:                   "ws-managed-validation-timeout",
		RunID:                         "run-managed-validation-timeout",
		StepID:                        "run-managed-validation-timeout/verify",
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Kind:                          "shell/command",
		Input: map[string]any{
			"command":              "sh",
			"args":                 []any{"-c", "mkdir -p \"$1\"; printf pending > \"$1/out.txt\"; sleep 5", "sh", scratchDirectory},
			"validationScratchDir": scratchDirectory,
			"managedValidation": map[string]any{
				"runtimeManaged": true,
				"autoCleanup":    true,
			},
		},
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ExecuteRuntimeStep err = %v, want deadline exceeded", err)
	}
	if result.Output["status"] != "timeout" {
		t.Fatalf("status = %v, want timeout", result.Output["status"])
	}
	resources, ok := result.Output["validationResources"].([]map[string]any)
	if !ok || len(resources) != 2 {
		t.Fatalf("validationResources = %#v, want process and scratch records", result.Output["validationResources"])
	}
	statuses := map[string]string{}
	for _, resource := range resources {
		statuses[stringFromShellValue(resource["kind"])] = stringFromShellValue(resource["status"])
		if resource["workspaceId"] != "ws-managed-validation-timeout" ||
			resource["runId"] != "run-managed-validation-timeout" ||
			resource["stepId"] != "run-managed-validation-timeout/verify" {
			t.Fatalf("resource = %+v, want scoped workspace/run/step ids", resource)
		}
		if strings.TrimSpace(stringFromShellValue(resource["releasedAt"])) == "" {
			t.Fatalf("resource = %+v, want releasedAt diagnostic", resource)
		}
	}
	if statuses["process"] != "timeout" || statuses["scratch_dir"] != "released" {
		t.Fatalf("resource statuses = %+v, want timeout process and released scratch", statuses)
	}
	managed, _ := result.Output["managedValidation"].(map[string]any)
	if managed["resourceCount"] != 2 || managed["resourcesReleased"] != true {
		t.Fatalf("managedValidation = %+v, want resources released after timeout cleanup", managed)
	}
	runtimeDiagnostics, _ := result.Output["runtimeDiagnostics"].(map[string]any)
	lifecycle, _ := runtimeDiagnostics["validationResourceLifecycle"].(map[string]any)
	if lifecycle["released"] != true {
		t.Fatalf("runtimeDiagnostics = %+v, want released lifecycle diagnostic", runtimeDiagnostics)
	}
	if _, statErr := os.Stat(scratchDirectory); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("scratchDirectory stat = %v, want removed after timeout cleanup", statErr)
	}
}

func TestRuntimeAdapter_ExecuteRuntimeStepUsesInputTimeoutMillis(t *testing.T) {
	adapter := NewRuntimeAdapterWithTimeout(time.Second)
	deliverableDirectory := t.TempDir()

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Kind:                          "shell/command",
		Input: map[string]any{
			"command":       "sh",
			"args":          []any{"-c", "sleep 0.2; echo done"},
			"timeoutMillis": 2000,
		},
	})
	if err != nil {
		t.Fatalf("ExecuteRuntimeStep: %v", err)
	}
	if result.Output["status"] != "completed" {
		t.Fatalf("status = %v, want completed", result.Output["status"])
	}
	if result.Output["timeoutMillis"] != int64(2000) {
		t.Fatalf("timeoutMillis = %v, want 2000", result.Output["timeoutMillis"])
	}
	stdout, _ := result.Output["stdout"].(string)
	if strings.TrimSpace(stdout) != "done" {
		t.Fatalf("stdout = %q, want done", stdout)
	}
}

func TestRuntimeAdapter_ExecuteRuntimeStepNormalizesShPipefailScriptToBash(t *testing.T) {
	adapter := NewRuntimeAdapter()
	deliverableDirectory := t.TempDir()

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Kind:                          "shell/command",
		Input: map[string]any{
			"command": "sh",
			"args":    []any{"-c", "set -euo pipefail\nprintf ok"},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteRuntimeStep: %v", err)
	}
	if result.Output["status"] != "completed" {
		t.Fatalf("status = %v, want completed", result.Output["status"])
	}
	if result.Output["command"] != "sh" || result.Output["actualCommand"] != "bash" {
		t.Fatalf("command fields = %+v, want requested sh executed by bash", result.Output)
	}
	if result.Output["commandNormalization"] != "sh_script_requires_bash_pipefail" {
		t.Fatalf("commandNormalization = %v, want pipefail normalization", result.Output["commandNormalization"])
	}
	stdout, _ := result.Output["stdout"].(string)
	if strings.TrimSpace(stdout) != "ok" {
		t.Fatalf("stdout = %q, want ok", stdout)
	}
}

func TestRuntimeAdapter_ExecuteRuntimeStepNormalizesShPipeStatusScriptToBash(t *testing.T) {
	adapter := NewRuntimeAdapter()
	deliverableDirectory := t.TempDir()

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Kind:                          "shell/command",
		Input: map[string]any{
			"command": "sh",
			"args":    []any{"-c", `false | true; printf 'first_status=%s\n' "${PIPESTATUS[0]}"`},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteRuntimeStep: %v output=%+v", err, result.Output)
	}
	if result.Output["status"] != "completed" {
		t.Fatalf("status = %v, want completed", result.Output["status"])
	}
	if result.Output["command"] != "sh" || result.Output["actualCommand"] != "bash" {
		t.Fatalf("command fields = %+v, want requested sh executed by bash", result.Output)
	}
	if result.Output["commandNormalization"] != "sh_script_requires_bash_pipestatus" {
		t.Fatalf("commandNormalization = %v, want pipestatus normalization", result.Output["commandNormalization"])
	}
	stdout, _ := result.Output["stdout"].(string)
	if strings.TrimSpace(stdout) != "first_status=1" {
		t.Fatalf("stdout = %q, want first_status=1", stdout)
	}
}

func TestRuntimeAdapter_ExecuteRuntimeStepWrapsCompoundCommandWithShell(t *testing.T) {
	adapter := NewRuntimeAdapter()
	deliverableDirectory := t.TempDir()

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Kind:                          "shell/command",
		Input: map[string]any{
			"command": "cd",
			"args":    []any{".", "&&", "pwd"},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteRuntimeStep: %v", err)
	}
	if result.Output["commandNormalization"] != "compound_command_wrapped_with_sh_c" {
		t.Fatalf("commandNormalization = %v, want compound command normalization", result.Output["commandNormalization"])
	}
	stdout, _ := result.Output["stdout"].(string)
	if strings.TrimSpace(stdout) != deliverableDirectory {
		t.Fatalf("stdout = %q, want deliverable directory %q", stdout, deliverableDirectory)
	}
}

func TestRuntimeAdapter_ExecuteRuntimeStepRejectsDangerousCompoundCommandAfterWrapping(t *testing.T) {
	adapter := NewRuntimeAdapter()
	_, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: t.TempDir(),
		Kind:                          "shell/command",
		Input: map[string]any{
			"command": "echo",
			"args":    []any{"ok", "&&", "sudo", "true"},
		},
	})
	if err == nil {
		t.Fatal("ExecuteRuntimeStep err = nil, want invalid argument")
	}
}

func TestRuntimeAdapter_ExecuteRuntimeStepRejectsInvalidInputTimeoutMillis(t *testing.T) {
	adapter := NewRuntimeAdapter()
	_, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceDeliverableDirectory: t.TempDir(),
		Kind:                          "shell/command",
		Input: map[string]any{
			"command":       "pwd",
			"timeoutMillis": 500,
		},
	})
	if err == nil {
		t.Fatal("ExecuteRuntimeStep err = nil, want invalid argument")
	}
}

func TestRuntimeAdapter_ExecuteRuntimeStepRejectsDangerousCommands(t *testing.T) {
	adapter := NewRuntimeAdapter()
	cases := []struct {
		name    string
		command string
		args    []any
	}{
		{name: "sudo", command: "sudo", args: []any{"true"}},
		{name: "rm root", command: "rm", args: []any{"-rf", "/"}},
		{name: "rm git", command: "rm", args: []any{"-rf", ".git"}},
		{name: "chmod 777", command: "chmod", args: []any{"777", "file.txt"}},
		{name: "git force push", command: "git", args: []any{"push", "--force"}},
		{name: "shell script", command: "sh", args: []any{"-c", "rm -rf /tmp/project/.git"}},
		{name: "direct kill", command: "kill", args: []any{"1234"}},
		{name: "direct pkill", command: "pkill", args: []any{"workorch"}},
		{name: "direct killall", command: "killall", args: []any{"workorch"}},
		{name: "fuser kill port", command: "fuser", args: []any{"-k", "8080/tcp"}},
		{name: "shell fuser kill port", command: "sh", args: []any{"-c", "fuser -k 8080/tcp 2>/dev/null || true"}},
		{name: "shell lsof xargs kill", command: "sh", args: []any{"-c", "lsof -ti :8080 | xargs kill -9"}},
		{name: "shell detached background", command: "sh", args: []any{"-c", "./service >/tmp/service.log 2>&1 & echo $!"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
				WorkspaceDeliverableDirectory: t.TempDir(),
				Kind:                          "shell/command",
				Input: map[string]any{
					"command": tc.command,
					"args":    tc.args,
				},
			})
			if err == nil {
				t.Fatal("ExecuteRuntimeStep err = nil, want invalid argument")
			}
		})
	}
}
