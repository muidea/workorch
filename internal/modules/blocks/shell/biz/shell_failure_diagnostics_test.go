package shell

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestFinalizeShellResultIncludesOutputPreviewOnFailure(t *testing.T) {
	exit := int64(2)
	output := RuntimeCommandResult{
		Status:        "failed",
		Stage:         "shell_command_failed",
		Command:       "go",
		Args:          []string{"test", "./..."},
		ExitCode:      exit,
		Stdout:        "ok package",
		Stderr:        "FAIL: TestHealthz\npackage path not found",
		FailureCode:   "shell_exit_nonzero",
		FailureReason: "shell_exit_nonzero",
	}
	result, err := finalizeShellResult(output, context.DeadlineExceeded)
	_ = err
	if result.FailureCode != "shell_exit_nonzero" {
		t.Fatalf("failure code = %q", result.FailureCode)
	}
	if !strings.Contains(result.FailureMessage, "stderr=") {
		t.Fatalf("failure message missing stderr preview: %q", result.FailureMessage)
	}
	if !strings.Contains(result.FailureMessage, "FAIL: TestHealthz") {
		t.Fatalf("failure message missing diagnostic content: %q", result.FailureMessage)
	}
	if result.Evidence.Summary == "" || result.Evidence.Summary == "failed" {
		t.Fatalf("evidence summary should carry diagnostics, got %q", result.Evidence.Summary)
	}
	if result.Evidence.ExitCode == nil || *result.Evidence.ExitCode != 2 {
		t.Fatalf("evidence exit code = %v", result.Evidence.ExitCode)
	}
}

func TestExecuteOperationNonzeroExitSetsStructuredFailure(t *testing.T) {
	dir := t.TempDir()
	// false always exits 1
	adapter := NewRuntimeAdapter()
	result, err := adapter.ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: dir,
		Command:                       "false",
	})
	if err == nil {
		t.Fatal("expected nonzero exit error")
	}
	if result.FailureCode != "shell_exit_nonzero" && result.FailureReason != "shell_exit_nonzero" {
		// command_not_found is also acceptable if false is missing, but usually present
		if result.FailureCode == "command_not_found" {
			t.Skip("false command unavailable in environment")
		}
		t.Fatalf("failure code = %q reason = %q", result.FailureCode, result.FailureReason)
	}
	if !strings.Contains(result.FailureMessage, "exit") && !strings.Contains(result.FailureMessage, "exitCode=") {
		t.Fatalf("failure message = %q", result.FailureMessage)
	}
	_ = os.WriteFile(filepath.Join(dir, "keep"), []byte("x"), 0o644)
}

func TestShellValidationDiagnosticPrioritizesNonzeroExitOverStaleZeroWork(t *testing.T) {
	class, targetType, ownerCode := shellValidationDiagnosticClassAndTarget(RuntimeCommandResult{
		ExitCode:        1,
		FailureCode:     "shell_exit_nonzero",
		ZeroWork:        true,
		EvidenceQuality: string(common.EvidenceQualityInconclusive),
	}, runtimecontract.RuntimeValidationEvidence{}, "go", false, false, errors.New("exit status 1"))
	if class != runtimecontract.ValidationDiagnosticBuildFailed || targetType != runtimecontract.ValidationEvidenceTargetFailureSurface || ownerCode != "shell_exit_nonzero" {
		t.Fatalf("diagnostic = (%q, %q, %q), want (%q, %q, shell_exit_nonzero)", class, targetType, ownerCode,
			runtimecontract.ValidationDiagnosticBuildFailed, runtimecontract.ValidationEvidenceTargetFailureSurface)
	}
}

func TestShellValidationDiagnosticClassifiesGoDependencyMetadataBeforeGenericBuildFailure(t *testing.T) {
	output := RuntimeCommandResult{
		Command: "go", ExitCode: 1, WorkingDirectory: t.TempDir(),
		Stderr: "internal/lsp/client.go:12:2: missing go.sum entry for module providing package github.com/sourcegraph/jsonrpc2 " +
			"(imported by example/internal/lsp); to add:\n\tgo get example/internal/lsp",
	}
	class, targetType, ownerCode := shellValidationDiagnosticClassAndTarget(
		output,
		runtimecontract.RuntimeValidationEvidence{DiagnosticClass: runtimecontract.ValidationDiagnosticBuildFailed},
		"go",
		false,
		false,
		errors.New("exit status 1"),
	)
	if class != runtimecontract.ValidationDiagnosticDependencyMismatch ||
		targetType != runtimecontract.ValidationEvidenceTargetDependencyContract ||
		ownerCode != runtimecontract.ValidationDiagnosticDependencyMismatch {
		t.Fatalf("diagnostic = (%q, %q, %q), want dependency mismatch", class, targetType, ownerCode)
	}

	request := RuntimeCommandRequest{
		ManagedValidation:         runtimecontract.RuntimeManagedValidation{RuntimeManaged: true, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession},
		ValidationScopeTargetRefs: []string{"go.mod", "internal/lsp/client.go"},
		ValidationEvidence:        runtimecontract.RuntimeValidationEvidence{DiagnosticClass: runtimecontract.ValidationDiagnosticBuildFailed},
	}
	attachShellValidationFailureDiagnostic(&output, request, "go", 1, false, false, errors.New("exit status 1"))
	if output.ValidationFailureDiagnostic == nil ||
		output.ValidationFailureDiagnostic.Class != runtimecontract.ValidationDiagnosticDependencyMismatch ||
		len(output.ValidationFailureDiagnostic.TargetRefs) != 0 ||
		output.validationFailureTargetCandidatesInvalid {
		t.Fatalf("dependency diagnostic leaked implementation repair targets: %+v", output.ValidationFailureDiagnostic)
	}

	nonGo := output
	nonGo.Command = "custom-validator"
	class, _, _ = shellValidationDiagnosticClassAndTarget(
		nonGo, runtimecontract.RuntimeValidationEvidence{}, "custom-validator", false, false, errors.New("exit status 1"),
	)
	if class != runtimecontract.ValidationDiagnosticBuildFailed {
		t.Fatalf("non-Go output borrowed Go dependency classification: %q", class)
	}
}

func TestEnrichShellFailureMessageRedactsSecrets(t *testing.T) {
	msg := enrichShellFailureMessage("boom", RuntimeCommandResult{
		ExitCode: 2,
		Stage:    "shell_command_failed",
		Stderr:   "api_key=sk-abc1234567890secret token=supersecretvalue",
		Stdout:   "password=hunter2",
	})
	if strings.Contains(msg, "sk-abc") || strings.Contains(msg, "supersecretvalue") || strings.Contains(msg, "hunter2") {
		t.Fatalf("secrets leaked into failure message: %q", msg)
	}
	if !strings.Contains(msg, "[REDACTED]") && !strings.Contains(msg, "stderr=") {
		t.Fatalf("expected redacted stderr preview, got %q", msg)
	}
}
