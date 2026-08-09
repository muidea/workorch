package shell

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestValidateGoTestValidationPreflightMissingTests(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := validateGoTestValidationPreflight(dir, []string{"test", "./...", "-run", "TestHealthz"})
	if err == nil || !strings.Contains(err.Error(), "no *_test.go files found") {
		t.Fatalf("expected missing test preflight error, got %v", err)
	}
}

func TestValidateGoTestValidationPreflightWithTests(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main_test.go"), []byte("package main\nimport \"testing\"\nfunc TestHealthz(t *testing.T) {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateGoTestValidationPreflight(dir, []string{"test", "./...", "-run", "TestHealthz"}); err != nil {
		t.Fatalf("expected preflight pass, got %v", err)
	}
}

func TestRuntimeAdapterSemanticValidationPreflightRejectsMissingTests(t *testing.T) {
	workspace := t.TempDir()
	recipe := runtimemodel.RuntimeValidationRecipe{ID: "go-tests", Command: "go", Args: []string{"test", "./..."}}
	err := NewRuntimeAdapter().PreflightRuntimeSemanticValidationRecipe(workspace, recipe)
	if err == nil || !strings.Contains(err.Error(), "no *_test.go files found") {
		t.Fatalf("expected semantic owner preflight to reject missing tests, got %v", err)
	}
}

func TestValidateGoTestPackageTargetsAcceptsExistingNestedRecursivePattern(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "lsp-mcp-server"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := validateGoTestPackageTargets(dir, []string{"test", "./lsp-mcp-server/..."}); err != nil {
		t.Fatalf("existing nested recursive package pattern rejected: %v", err)
	}
	if err := validateGoTestPackageTargets(dir, []string{"test", "./lsp-mcp-server/.../adapter"}); err != nil {
		t.Fatalf("existing nested wildcard package pattern rejected: %v", err)
	}
}

func TestValidateGoTestPackageTargetsRejectsMissingNestedRecursivePattern(t *testing.T) {
	dir := t.TempDir()
	err := validateGoTestPackageTargets(dir, []string{"test", "./missing/..."})
	if err == nil || !strings.Contains(err.Error(), `package target "./missing/..." does not exist`) {
		t.Fatalf("expected missing nested recursive package pattern, got %v", err)
	}
}

func TestExecuteOperationValidationPreflightRejectsMissingDeclaredTarget(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main_test.go"), []byte("package main\nimport \"testing\"\nfunc TestHealthz(t *testing.T) {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := NewRuntimeAdapter().ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: dir,
		Command:                       "go",
		Args:                          []string{"test", "-v", "./..."},
		TargetRefs:                    []string{"missing_test.go"},
	})
	if err == nil || result.Stage != "shell_validation_preflight_failed" || result.FailureCode != "validation_target_missing" ||
		!strings.Contains(result.FailureMessage, "does not exist") {
		t.Fatalf("missing declared validation target result=%+v err=%v", result, err)
	}
}

func TestExecuteOperationValidationPreflightReturnsStructuredFailure(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	adapter := NewRuntimeAdapter()
	result, err := adapter.ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: dir,
		Command:                       "go",
		Args:                          []string{"test", "./...", "-run", "TestHealthz"},
	})
	if err == nil {
		t.Fatal("expected preflight failure")
	}
	if result.Stage != "shell_validation_preflight_failed" {
		t.Fatalf("stage = %q", result.Stage)
	}
	if result.FailureCode != "validation_target_missing" {
		t.Fatalf("failure code = %q", result.FailureCode)
	}
	if !strings.Contains(result.FailureMessage, "no *_test.go files found") && !strings.Contains(err.Error(), "no *_test.go files found") {
		t.Fatalf("message = %q err = %v", result.FailureMessage, err)
	}
	if !strings.Contains(err.Error(), common.ErrInvalidArgument.Error()) && err != common.ErrInvalidArgument {
		// wrapped invalid argument is fine
		if !strings.Contains(strings.ToLower(err.Error()), "invalid") {
			t.Fatalf("unexpected error class: %v", err)
		}
	}
}

func TestLiveWorkspaceGoTestPreflight(t *testing.T) {
	root := "/home/rangh/aispace/workorch/.workorch-workspaces/ws-09c99cd1ce52f6b1"
	if _, err := os.Stat(root); err != nil {
		t.Skip(err)
	}
	hasTests, err := workspaceContainsGoTestFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	if hasTests {
		t.Skip("live workspace already has *_test.go; preflight missing-target path not applicable")
	}
	err = validateGoTestValidationPreflight(root, []string{"test", "-v", "-run", "TestHealthz", "./cmd/proxy"})
	if err == nil {
		t.Fatal("expected missing tests on live workspace")
	}
	t.Log(err)
	adapter := NewRuntimeAdapter()
	result, err := adapter.ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: root,
		Command:                       "go",
		Args:                          []string{"test", "-v", "-run", "TestHealthz", "./cmd/proxy"},
	})
	t.Logf("stage=%s code=%s msg=%s err=%v", result.Stage, result.FailureCode, result.FailureMessage, err)
	if result.FailureCode != "validation_target_missing" {
		t.Fatalf("want validation_target_missing got %q", result.FailureCode)
	}
}
