package shell

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeAdapter_ExecuteOperationRunsCommandInWorkspaceDirectory(t *testing.T) {
	adapter := NewRuntimeAdapter()
	deliverableDirectory := t.TempDir()

	result, err := adapter.ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Command:                       "pwd",
	})
	if err != nil {
		t.Fatalf("ExecuteOperation: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exitCode = %v, want 0", result.ExitCode)
	}
	stdout := result.Stdout
	if strings.TrimSpace(stdout) != deliverableDirectory {
		t.Fatalf("stdout = %q, want deliverable directory %q", stdout, deliverableDirectory)
	}
}

func TestRuntimeAdapterPreflightValidationRecipesFiltersUnavailableCommandsWithoutExecuting(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is not available")
	}
	workspace := t.TempDir()
	marker := filepath.Join(workspace, "marker")
	if err := os.WriteFile(marker, []byte("unchanged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	confirmed, err := NewRuntimeAdapter().PreflightRuntimeValidationRecipes(workspace, []runtimemodel.RuntimeValidationRecipe{
		{ID: "valid", Command: "sh", Args: []string{"-c", "printf should-not-run > marker"}},
		{ID: "missing", Command: "workorch-command-that-does-not-exist", Args: []string{"check"}},
	})
	if err != nil {
		t.Fatalf("preflight: %v", err)
	}
	if len(confirmed) != 1 || confirmed[0].ID != "valid" {
		t.Fatalf("confirmed recipes = %+v", confirmed)
	}
	content, err := os.ReadFile(marker)
	if err != nil || string(content) != "unchanged\n" {
		t.Fatalf("preflight executed a recipe: content=%q err=%v", content, err)
	}
}

func TestShellOwnerResolvesProjectNativeAdapterFromFileManifestFact(t *testing.T) {
	declaration := runtimemodel.RuntimeValidationRecipe{
		ID: "project-native", AdapterRef: modulecommon.ValidationRecipeAdapterProjectNative, TimeoutMillis: 60_000,
	}
	resolved := resolveRuntimeValidationRecipeDeclarations([]runtimemodel.RuntimeValidationRecipe{declaration}, runtimemodel.WorkspaceManifest{
		Revision: "tree-go", TargetRefs: []string{"go.mod", "main.go"},
	})
	if len(resolved) != 1 || resolved[0].ID != declaration.ID || resolved[0].AdapterRef != declaration.AdapterRef ||
		resolved[0].Command != "go" || !reflect.DeepEqual(resolved[0].Args, []string{"test", "-v", "./..."}) {
		t.Fatalf("Shell owner resolved project-native adapter = %+v", resolved)
	}
	if got := resolveRuntimeValidationRecipeDeclarations([]runtimemodel.RuntimeValidationRecipe{declaration}, runtimemodel.WorkspaceManifest{
		Revision: "tree-python", TargetRefs: []string{"pyproject.toml", "main.py"},
	}); len(got) != 0 {
		t.Fatalf("Shell owner inferred unsupported language recipe: %+v", got)
	}
}

func TestRuntimeAdapterPreflightValidationRecipesFiltersWorkspaceInapplicableGoTest(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not available")
	}
	workspace := t.TempDir()
	recipe := runtimemodel.RuntimeValidationRecipe{ID: "go-tests", Command: "go", Args: []string{"test", "-v", "./..."}}
	withoutTests, err := NewRuntimeAdapter().PreflightRuntimeValidationRecipesForManifest(workspace, []runtimemodel.RuntimeValidationRecipe{recipe}, runtimemodel.WorkspaceManifest{
		Revision: "tree-module-without-tests", TargetRefs: []string{"go.mod", "main.go"},
	})
	if err != nil {
		t.Fatalf("preflight without tests: %v", err)
	}
	if len(withoutTests) != 0 {
		t.Fatalf("workspace-inapplicable recipe was published: %+v", withoutTests)
	}
	withTestsWithoutModule, err := NewRuntimeAdapter().PreflightRuntimeValidationRecipesForManifest(workspace, []runtimemodel.RuntimeValidationRecipe{recipe}, runtimemodel.WorkspaceManifest{
		Revision: "tree-tests-without-module", TargetRefs: []string{"main.go", "main_test.go"},
	})
	if err != nil {
		t.Fatalf("preflight without go.mod: %v", err)
	}
	if len(withTestsWithoutModule) != 0 {
		t.Fatalf("go test recipe was published without root go.mod: %+v", withTestsWithoutModule)
	}
	withTests, err := NewRuntimeAdapter().PreflightRuntimeValidationRecipesForManifest(workspace, []runtimemodel.RuntimeValidationRecipe{recipe}, runtimemodel.WorkspaceManifest{
		Revision: "tree-with-module-and-tests", TargetRefs: []string{"go.mod", "main.go", "main_test.go"},
	})
	if err != nil {
		t.Fatalf("preflight with tests: %v", err)
	}
	if len(withTests) != 1 || withTests[0].ID != recipe.ID {
		t.Fatalf("workspace-applicable recipe was omitted: %+v", withTests)
	}
}

func TestRuntimeAdapterProbeRejectsMissingGoTestTargetWithoutStartingProcess(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "marker"), []byte("unchanged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	recipe := runtimemodel.RuntimeValidationRecipe{ID: "missing-go-test", Command: "go", Args: []string{"test", "./missing/package"}}
	recipeID := recipe.ID
	adapter := NewRuntimeAdapter()
	inventory, err := adapter.AcquireRuntimeOperationAdmissibility(workspace, "admissibility-shell", []string{runtimemodel.RuntimeBlockKindManagedValidationSession})
	if err != nil {
		t.Fatal(err)
	}
	failure, stale := adapter.ProbeRuntimeOperationFeasibilityWithObservationAndValidationRecipes(workspace, inventory, runtimemodel.RuntimeOperation{
		ID: "validation-missing-target", Kind: runtimemodel.RuntimeBlockKindManagedValidationSession,
		Parameters: []runtimemodel.OperationParameter{{Name: "recipeId", String: &recipeID}},
	}, nil, []runtimemodel.RuntimeValidationRecipe{recipe})
	if stale != nil {
		t.Fatalf("missing target probe returned stale: %+v", stale)
	}
	if failure == nil || failure.Code != "validation_target_missing" {
		t.Fatalf("missing go test target feasibility failure = %+v", failure)
	}
	if failure.ValidationDiagnostic == nil || failure.ValidationDiagnostic.Class != runtimemodel.ValidationDiagnosticCommandInvalid ||
		failure.ValidationDiagnostic.RecommendedEvidenceTargetType != runtimemodel.ValidationEvidenceTargetCommandContract {
		t.Fatalf("missing go test target diagnostic = %+v", failure.ValidationDiagnostic)
	}
	content, err := os.ReadFile(filepath.Join(workspace, "marker"))
	if err != nil || string(content) != "unchanged\n" {
		t.Fatalf("probe changed workspace content=%q err=%v", content, err)
	}
}

func TestRuntimeAdapterManagedValidationAdmissibilityPublishesOwnerSelector(t *testing.T) {
	workspace := t.TempDir()
	inventory, err := NewRuntimeAdapter().AcquireRuntimeOperationAdmissibility(
		workspace,
		"admissibility-shell-selector",
		[]string{runtimemodel.RuntimeBlockKindManagedValidationSession},
	)
	if err != nil {
		t.Fatalf("AcquireRuntimeOperationAdmissibility: %v", err)
	}
	if !reflect.DeepEqual(inventory.SelectorRefs, []string{"workspace-validation-target"}) {
		t.Fatalf("selector refs = %+v, want workspace-validation-target", inventory.SelectorRefs)
	}
	if len(inventory.OperationVariants) != 0 {
		t.Fatalf("managed validation unexpectedly widened target authority: %+v", inventory.OperationVariants)
	}
	if err := inventory.Validate(); err != nil {
		t.Fatalf("inventory validation: %v", err)
	}
}

func TestRuntimeAdapterProbeDoesNotTreatWorkspaceDirectoryMTimeAsAdmissibilityStale(t *testing.T) {
	workspace := t.TempDir()
	adapter := NewRuntimeAdapter()
	inventory, err := adapter.AcquireRuntimeOperationAdmissibility(workspace, "admissibility-shell", []string{runtimemodel.RuntimeBlockKindManagedValidationSession})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "changed-after-inventory"), []byte("unchanged by probe\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	recipe := runtimemodel.RuntimeValidationRecipe{ID: "missing-go-test", Command: "go", Args: []string{"test", "./missing/package"}}
	recipeID := recipe.ID
	failure, stale := adapter.ProbeRuntimeOperationFeasibilityWithObservationAndValidationRecipes(workspace, inventory, runtimemodel.RuntimeOperation{
		ID: "stale-validation", Kind: runtimemodel.RuntimeBlockKindManagedValidationSession,
		Parameters: []runtimemodel.OperationParameter{{Name: "recipeId", String: &recipeID}},
	}, nil, []runtimemodel.RuntimeValidationRecipe{recipe})
	if stale != nil {
		t.Fatalf("workspace mutation probe returned stale: %+v", stale)
	}
	if failure == nil || failure.Code != "validation_target_missing" {
		t.Fatalf("workspace mutation incorrectly changed shell freshness semantics: %+v", failure)
	}
	content, err := os.ReadFile(filepath.Join(workspace, "changed-after-inventory"))
	if err != nil || string(content) != "unchanged by probe\n" {
		t.Fatalf("probe changed workspace content=%q err=%v", content, err)
	}
}

func TestRuntimeAdapterProbeReturnsTypedStaleForObservedValidationTargetChange(t *testing.T) {
	workspace := t.TempDir()
	targetRef := "target.txt"
	if err := os.WriteFile(filepath.Join(workspace, targetRef), []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	adapter := NewRuntimeAdapter()
	inventory, err := adapter.AcquireRuntimeOperationAdmissibility(workspace, "admissibility-shell", []string{runtimemodel.StepKindShellCommand})
	if err != nil {
		t.Fatal(err)
	}
	observation := adapter.runtimeAdmissibilityObservation(workspace, inventory.PrerequisiteID, inventory, []string{targetRef})
	if err := observation.Validate(); err != nil {
		t.Fatalf("observation: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, targetRef), []byte("after\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	command := "pwd"
	failure, stale := adapter.ProbeRuntimeOperationFeasibilityWithObservation(workspace, inventory, runtimemodel.RuntimeOperation{
		ID: "shell-target-stale", Kind: runtimemodel.StepKindShellCommand, Parameters: []runtimemodel.OperationParameter{{Name: "command", String: &command}},
	}, &observation)
	if failure != nil || stale == nil || stale.SubjectRef != targetRef || stale.ExpectedRevision == stale.ObservedRevision {
		t.Fatalf("target mutation did not return typed stale: failure=%+v stale=%+v", failure, stale)
	}
	if stale.OwnerModuleID != runtimemodel.RuntimeModuleFile {
		t.Fatalf("validation target stale owner = %q, want File resource owner", stale.OwnerModuleID)
	}
	if !runtimemodel.RuntimeFreshnessDependencyIntersectsChange(observation.Dependencies[1], runtimemodel.RuntimeResourceChange{
		Domain: runtimemodel.RuntimeResourceWorkspaceTarget, OwnerModuleID: runtimemodel.RuntimeModuleFile,
		SubjectRef: targetRef, AfterRevision: stale.ObservedRevision,
	}) {
		t.Fatalf("File target change does not intersect Shell validation observation: %+v", observation.Dependencies)
	}
}

func TestRuntimeAdapterObservationTracksMissingValidationTargetForFileCreate(t *testing.T) {
	workspace := t.TempDir()
	targetRef := "main.go"
	adapter := NewRuntimeAdapter()
	inventory, err := adapter.AcquireRuntimeOperationAdmissibility(workspace, "admissibility-shell", []string{runtimemodel.StepKindShellCommand})
	if err != nil {
		t.Fatal(err)
	}
	observation := adapter.runtimeAdmissibilityObservation(workspace, inventory.PrerequisiteID, inventory, []string{targetRef})
	if err := observation.Validate(); err != nil {
		t.Fatalf("missing-target observation: %v", err)
	}
	var targetDependency *runtimemodel.RuntimeFreshnessDependency
	for idx := range observation.Dependencies {
		dependency := observation.Dependencies[idx]
		if dependency.Domain == runtimemodel.RuntimeResourceWorkspaceTarget {
			targetDependency = &dependency
			break
		}
	}
	if targetDependency == nil || targetDependency.OwnerModuleID != runtimemodel.RuntimeModuleFile || targetDependency.Revision != "missing" {
		t.Fatalf("missing target dependency = %+v", observation.Dependencies)
	}
	if !runtimemodel.RuntimeFreshnessDependencyIntersectsChange(*targetDependency, runtimemodel.RuntimeResourceChange{
		Domain: runtimemodel.RuntimeResourceWorkspaceTarget, OwnerModuleID: runtimemodel.RuntimeModuleFile,
		SubjectRef: targetRef, AfterRevision: "main-v1",
	}) {
		t.Fatalf("File create does not intersect missing Shell validation target: %+v", targetDependency)
	}
}

func TestRuntimeAdapterRejectsStateChangingShellWithoutWorkspaceDelta(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	adapter := NewRuntimeAdapter()
	deliverableDirectory := t.TempDir()
	initShellTestGitRepo(t, deliverableDirectory)

	result, err := adapter.ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Command:                       "sh",
		Args:                          []string{"-c", "printf checked"},
		StateChanging:                 true,
		TargetRef:                     ".",
	})
	if err == nil || !strings.Contains(err.Error(), "no observable workspace delta") {
		t.Fatalf("ExecuteOperation err = %v, want no-progress rejection", err)
	}
	if result.FailureReason != "state_changing_no_workspace_delta" ||
		result.ZeroWork != true ||
		result.EvidenceQuality != string(common.EvidenceQualityInconclusive) {
		t.Fatalf("output = %+v, want state-changing no-progress diagnostic", result)
	}
}

func TestRuntimeAdapterAllowsStateChangingShellWithWorkspaceDelta(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	adapter := NewRuntimeAdapter()
	deliverableDirectory := t.TempDir()
	initShellTestGitRepo(t, deliverableDirectory)

	result, err := adapter.ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Command:                       "sh",
		Args:                          []string{"-c", "printf changed > generated.txt"},
		StateChanging:                 true,
		TargetRef:                     "generated.txt",
	})
	if err != nil {
		t.Fatalf("ExecuteOperation: %v output=%+v", err, result)
	}
	if result.FailureReason == "state_changing_no_workspace_delta" {
		t.Fatalf("output = %+v, want accepted workspace delta", result)
	}
}

func initShellTestGitRepo(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@example.local"},
		{"config", "user.name", "WorkOrch Test"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, string(out))
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("initial\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	for _, args := range [][]string{
		{"add", "README.md"},
		{"commit", "-m", "initial"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, string(out))
		}
	}
}

func TestRuntimeAdapter_ExecuteOperationUsesDefaultCommandTimeout(t *testing.T) {
	adapter := NewRuntimeAdapter()
	deliverableDirectory := t.TempDir()

	result, err := adapter.ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Command:                       "pwd",
	})
	if err != nil {
		t.Fatalf("ExecuteOperation: %v", err)
	}
	if result.TimeoutMillis != int64(DefaultCommandTimeout/time.Millisecond) {
		t.Fatalf("timeoutMillis = %v, want %d", result.TimeoutMillis, DefaultCommandTimeout/time.Millisecond)
	}
}

func TestNewRuntimeAdapterFromTimeoutString(t *testing.T) {
	adapter, err := NewRuntimeAdapterFromTimeoutString("90s")
	if err != nil {
		t.Fatalf("NewRuntimeAdapterFromTimeoutString: %v", err)
	}
	result, err := adapter.ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: t.TempDir(),
		Command:                       "pwd",
	})
	if err != nil {
		t.Fatalf("ExecuteOperation: %v", err)
	}
	if result.TimeoutMillis != int64(90000) {
		t.Fatalf("timeoutMillis = %v, want 90000", result.TimeoutMillis)
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
	timeout, err := shellCommandTimeout(0, adapter.timeout)
	if err != nil {
		t.Fatalf("shellCommandTimeout: %v", err)
	}
	if timeout != 10*time.Minute {
		t.Fatalf("timeout = %s, want 10m", timeout)
	}
}

func TestRuntimeAdapter_ExecuteOperationCreatesMissingWorkspaceDirectory(t *testing.T) {
	adapter := NewRuntimeAdapter()
	deliverableDirectory := filepath.Join(t.TempDir(), "missing", "workspace")

	result, err := adapter.ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Command:                       "pwd",
	})
	if err != nil {
		t.Fatalf("ExecuteOperation: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exitCode = %v, want 0", result.ExitCode)
	}
	info, err := os.Stat(deliverableDirectory)
	if err != nil {
		t.Fatalf("stat deliverable directory: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("deliverableDirectory is not a directory")
	}
}

func TestRuntimeAdapter_ExecuteOperationReturnsOutputForNonZeroExit(t *testing.T) {
	adapter := NewRuntimeAdapter()
	deliverableDirectory := t.TempDir()

	result, err := adapter.ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Command:                       "sh",
		Args:                          []string{"-c", "echo shell-out; echo shell-err >&2; exit 7"},
	})
	if err == nil {
		t.Fatal("ExecuteOperation err = nil, want non-zero exit error")
	}
	if result.Status != "failed" {
		t.Fatalf("status = %v, want failed", result.Status)
	}
	if result.ExitCode != 7 {
		t.Fatalf("exitCode = %v, want 7", result.ExitCode)
	}
	stdout := result.Stdout
	stderr := result.Stderr
	if !strings.Contains(stdout, "shell-out") || !strings.Contains(stderr, "shell-err") {
		t.Fatalf("stdout/stderr = %q/%q, want captured command output", stdout, stderr)
	}
}

func TestRuntimeAdapter_ExecuteOperationDoesNotPromoteValidationInputTargets(t *testing.T) {
	adapter := NewRuntimeAdapter()
	deliverableDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(deliverableDirectory, "feature_test.go"), []byte("package fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deliverableDirectory, "feature.go"), []byte("package fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := adapter.ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Command:                       "sh",
		Args:                          []string{"-c", "echo assertion failed >&2; exit 3"},
		TargetRefs:                    []string{"feature_test.go"},
		ValidationEvidence:            runtimemodel.RuntimeValidationEvidence{Mode: "runtime_behavior", ObservationType: "test_result", TargetRefs: []string{"feature-contract"}},
	})
	if err == nil {
		t.Fatal("ExecuteOperation err = nil, want non-zero exit error")
	}
	diagnostic := result.ValidationFailureDiagnostic
	if diagnostic == nil {
		t.Fatal("validationFailureDiagnostic = nil, want test failure diagnostic")
	}
	if diagnostic.Class != runtimemodel.ValidationDiagnosticTestFailed ||
		diagnostic.RecommendedEvidenceTargetType != runtimemodel.ValidationEvidenceTargetBehaviorEvidence {
		t.Fatalf("validationFailureDiagnostic = %+v, want test failure behavior diagnostic", diagnostic)
	}
	if len(diagnostic.TargetRefs) != 0 {
		t.Fatalf("validation input targets became diagnostic failure targets: %#v", diagnostic.TargetRefs)
	}
	if result.RuntimeDiagnostics.ValidationFailureDiagnostic == nil {
		t.Fatalf("runtimeDiagnostics = %+v, want nested validation diagnostic", result.RuntimeDiagnostics)
	}
}

func TestRuntimeAdapter_ExecuteOperationRejectsValidationShellMaskedExitStatus(t *testing.T) {
	adapter := NewRuntimeAdapter()
	deliverableDirectory := t.TempDir()

	result, err := adapter.ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Command:                       "sh",
		Args:                          []string{"-c", "false; echo EXIT_CODE=$?"},
		ValidationEvidence:            runtimemodel.RuntimeValidationEvidence{Mode: "runtime_behavior", ObservationType: "runtime_observation", TargetRefs: []string{"feature-contract"}, ObservedClaims: []string{"feature contract was validated"}},
	})
	if err == nil {
		t.Fatal("ExecuteOperation err = nil, want masked validation exit status rejection")
	}
	if result.Status != "failed" ||
		result.Stage != "shell_validation_contract_rejected" ||
		result.FailureReason != "validation_shell_masks_failure" {
		t.Fatalf("output = %+v, want validation shell contract rejection", result)
	}
	diagnostic := result.ValidationFailureDiagnostic
	if diagnostic == nil {
		t.Fatal("validationFailureDiagnostic = nil, want command invalid diagnostic")
	}
	if diagnostic.Class != runtimemodel.ValidationDiagnosticCommandInvalid {
		t.Fatalf("validationFailureDiagnostic = %+v, want command invalid", diagnostic)
	}
}

func TestRuntimeAdapter_ExecuteOperationRejectsValidationShellFallbackMaskedFailure(t *testing.T) {
	adapter := NewRuntimeAdapter()
	deliverableDirectory := t.TempDir()

	result, err := adapter.ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Command:                       "sh",
		Args:                          []string{"-c", "command -v tool && false 2>&1 || echo 'failed'"},
		ValidationEvidence:            runtimemodel.RuntimeValidationEvidence{Mode: "runtime_behavior", ObservationType: "runtime_observation", TargetRefs: []string{"feature-contract"}, ObservedClaims: []string{"feature contract was validated"}},
	})
	if err == nil {
		t.Fatal("ExecuteOperation err = nil, want fallback-masked validation failure rejection")
	}
	if result.Status != "failed" ||
		result.Stage != "shell_validation_contract_rejected" ||
		result.FailureReason != "validation_shell_masks_failure" {
		t.Fatalf("output = %+v, want validation shell contract rejection", result)
	}
	diagnostic := result.ValidationFailureDiagnostic
	if diagnostic == nil {
		t.Fatal("validationFailureDiagnostic = nil, want command invalid diagnostic")
	}
	if diagnostic.Class != runtimemodel.ValidationDiagnosticCommandInvalid {
		t.Fatalf("validationFailureDiagnostic = %+v, want command invalid", diagnostic)
	}
}

func TestRuntimeAdapter_ExecuteOperationCarriesManagedValidationMetadata(t *testing.T) {
	adapter := NewRuntimeAdapter()
	deliverableDirectory := t.TempDir()
	scratchDirectory := filepath.Join(os.TempDir(), "workorch-shell-managed-validation-test", strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()))
	t.Cleanup(func() {
		_ = os.RemoveAll(scratchDirectory)
	})

	result, err := adapter.ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceID:                   "ws-managed-validation",
		RunID:                         "run-managed-validation",
		StepID:                        "run-managed-validation/verify",
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Command:                       "sh",
		Args:                          []string{"-c", "mkdir -p \"$1\"; printf managed-ok > \"$1/out.txt\"; printf managed-ok", "sh", scratchDirectory},
		ValidationScratchDirectory:    scratchDirectory,
		ValidationEvidence:            runtimemodel.RuntimeValidationEvidence{Mode: "runtime_behavior", ObservationType: "runtime_observation"},
		ManagedValidation:             runtimemodel.RuntimeManagedValidation{RuntimeManaged: true, AutoCleanup: true, CleanupPolicy: "process_group_on_timeout_or_completion", AutoCleanupSet: true},
	})
	if err != nil {
		t.Fatalf("ExecuteOperation: %v", err)
	}
	managed := result.ManagedValidation
	if !managed.RuntimeManaged || !managed.AutoCleanup ||
		managed.CleanupPolicy != "process_group_on_timeout_or_completion" {
		t.Fatalf("managedValidation = %+v, want carried runtime-managed metadata", managed)
	}
	evidence := result.ValidationEvidence
	if evidence.Mode != "runtime_behavior" || evidence.ObservationType != "runtime_observation" {
		t.Fatalf("validationEvidence = %+v, want carried validation evidence", evidence)
	}
	if managed.ResourceCount != 2 || !managed.ResourcesReleased {
		t.Fatalf("managedValidation = %+v, want released process and scratch resources", managed)
	}
	resources := result.ValidationResources
	if len(resources) != 2 {
		t.Fatalf("validationResources = %#v, want two resource records", result.ValidationResources)
	}
	kinds := map[string]string{}
	for _, resource := range resources {
		kinds[resource.Kind] = resource.Status
		if resource.WorkspaceID != "ws-managed-validation" ||
			resource.RunID != "run-managed-validation" ||
			resource.StepID != "run-managed-validation/verify" {
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

func TestRuntimeAdapter_ManagedValidationRecordsSealedRecipeWithoutInventingChecks(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is not available")
	}
	result, err := NewRuntimeAdapter().ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: t.TempDir(),
		ValidationRecipeID:            "presence-check",
		Command:                       "sh",
		Args:                          []string{"-c", "true"},
		TargetRefs:                    []string{"go.mod"},
		ManagedValidation:             runtimemodel.RuntimeManagedValidation{RuntimeManaged: true, Kind: runtimemodel.RuntimeBlockKindManagedValidationSession},
	})
	if err != nil {
		t.Fatalf("ExecuteOperation: %v", err)
	}
	if result.ValidationObservation == nil || result.ValidationObservation.RecipeID != "presence-check" {
		t.Fatalf("validation observation = %+v, want sealed recipe receipt", result.ValidationObservation)
	}
	if len(result.ValidationObservation.ExecutedChecks) != 0 || len(result.ValidationObservation.PassedChecks) != 0 {
		t.Fatalf("generic managed validation invented checks: %+v", result.ValidationObservation)
	}
	if result.Evidence.Validation == nil || result.Evidence.Validation.RecipeID != "presence-check" {
		t.Fatalf("evidence validation = %+v, want sealed recipe receipt", result.Evidence.Validation)
	}
}

func TestRuntimeAdapter_ExecuteOperationReleasesOrdinaryValidationScratch(t *testing.T) {
	adapter := NewRuntimeAdapter()
	deliverableDirectory := t.TempDir()
	scratchDirectory := filepath.Join(os.TempDir(), "workorch-shell-validation-scratch-test", strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()))
	t.Cleanup(func() {
		_ = os.RemoveAll(scratchDirectory)
	})

	result, err := adapter.ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceID:                   "ws-validation-scratch",
		RunID:                         "run-validation-scratch",
		StepID:                        "run-validation-scratch/verify",
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Command:                       "sh",
		Args:                          []string{"-c", "mkdir -p \"$1\"; printf ordinary-ok > \"$1/out.txt\"; printf ordinary-ok", "sh", scratchDirectory},
		ValidationScratchDirectory:    scratchDirectory,
		ValidationEvidence:            runtimemodel.RuntimeValidationEvidence{Mode: "runtime_behavior", ObservationType: "runtime_observation"},
	})
	if err != nil {
		t.Fatalf("ExecuteOperation: %v", err)
	}
	if result.ManagedValidation.RuntimeManaged || result.ManagedValidation.Kind != "" {
		t.Fatalf("managedValidation = %+v, want ordinary validation lifecycle without managed metadata", result.ManagedValidation)
	}
	resources := result.ValidationResources
	if len(resources) != 2 {
		t.Fatalf("validationResources = %#v, want process and scratch resource records", result.ValidationResources)
	}
	statuses := map[string]string{}
	for _, resource := range resources {
		statuses[resource.Kind] = resource.Status
		if resource.WorkspaceID != "ws-validation-scratch" ||
			resource.RunID != "run-validation-scratch" ||
			resource.StepID != "run-validation-scratch/verify" {
			t.Fatalf("resource = %+v, want scoped workspace/run/step ids", resource)
		}
	}
	if statuses["process"] != "released" || statuses["scratch_dir"] != "released" {
		t.Fatalf("resource statuses = %+v, want released process and scratch_dir", statuses)
	}
	if !result.RuntimeDiagnostics.ValidationResourceLifecycle.Released {
		t.Fatalf("runtimeDiagnostics = %+v, want released ordinary validation lifecycle", result.RuntimeDiagnostics)
	}
	if _, statErr := os.Stat(scratchDirectory); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("scratchDirectory stat = %v, want removed by ordinary validation scratch cleanup", statErr)
	}
}

func TestRuntimeAdapter_ExecuteOperationRetainsValidationScratchWhenAutoCleanupDisabled(t *testing.T) {
	adapter := NewRuntimeAdapter()
	deliverableDirectory := t.TempDir()
	scratchDirectory := filepath.Join(os.TempDir(), "workorch-shell-validation-scratch-test", strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()))
	t.Cleanup(func() {
		_ = os.RemoveAll(scratchDirectory)
	})

	result, err := adapter.ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceID:                     "ws-validation-scratch-retain",
		RunID:                           "run-validation-scratch-retain",
		StepID:                          "run-validation-scratch-retain/verify",
		WorkspaceDeliverableDirectory:   deliverableDirectory,
		Command:                         "sh",
		Args:                            []string{"-c", "mkdir -p \"$1\"; printf retained-ok > \"$1/out.txt\"; printf retained-ok", "sh", scratchDirectory},
		ValidationScratchDirectory:      scratchDirectory,
		ValidationScratchAutoCleanup:    false,
		ValidationEvidence:              runtimemodel.RuntimeValidationEvidence{Mode: "runtime_behavior", ObservationType: "runtime_observation"},
		ValidationScratchAutoCleanupSet: true,
	})
	if err != nil {
		t.Fatalf("ExecuteOperation: %v", err)
	}
	resources := result.ValidationResources
	if len(resources) != 2 {
		t.Fatalf("validationResources = %#v, want process and scratch resource records", result.ValidationResources)
	}
	statuses := map[string]string{}
	for _, resource := range resources {
		statuses[resource.Kind] = resource.Status
	}
	if statuses["scratch_dir"] != "retained" {
		t.Fatalf("resource statuses = %+v, want retained scratch_dir", statuses)
	}
	if _, statErr := os.Stat(filepath.Join(scratchDirectory, "out.txt")); statErr != nil {
		t.Fatalf("scratch output stat = %v, want retained", statErr)
	}
}

func TestRuntimeAdapter_ExecuteOperationClassifiesUnavailableCommand(t *testing.T) {
	adapter := NewRuntimeAdapter()
	deliverableDirectory := t.TempDir()
	command := "workorch-command-that-should-not-exist"

	result, err := adapter.ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Command:                       command,
	})
	if err == nil {
		t.Fatal("ExecuteOperation err = nil, want command lookup failure")
	}
	if result.Status != "failed" {
		t.Fatalf("status = %v, want failed", result.Status)
	}
	if result.FailureReason != "command_not_found" || result.UnavailableCommand != command || result.CapabilityMissing != true {
		t.Fatalf("output = %+v, want command-not-found metadata", result)
	}
	diagnostics := result.RuntimeDiagnostics
	if diagnostics.Kind != "runtime_capability_unavailable" || diagnostics.UnavailableCommand != command {
		t.Fatalf("runtimeDiagnostics = %+v, want runtime capability unavailable", diagnostics)
	}
	validationDiagnostic := result.ValidationFailureDiagnostic
	if validationDiagnostic == nil {
		t.Fatal("validationFailureDiagnostic = nil, want environment missing diagnostic")
	}
	if validationDiagnostic.Class != runtimemodel.ValidationDiagnosticEnvironmentMissing {
		t.Fatalf("validationFailureDiagnostic = %+v, want environment missing", validationDiagnostic)
	}
}

func TestRuntimeAdapter_ExecuteOperationReturnsPolicyDiagnostic(t *testing.T) {
	adapter := NewRuntimeAdapter()

	result, err := adapter.ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: t.TempDir(),
		Command:                       "kill",
		Args:                          []string{"123"},
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("ExecuteOperation err = %v, want invalid argument", err)
	}
	if result.Stage != "shell_command_policy_rejected" {
		t.Fatalf("stage = %v, want shell_command_policy_rejected", result.Stage)
	}
	diagnostic := result.ValidationFailureDiagnostic
	if diagnostic == nil {
		t.Fatal("validationFailureDiagnostic = nil, want command invalid policy diagnostic")
	}
	if diagnostic.Class != runtimemodel.ValidationDiagnosticCommandInvalid ||
		!diagnostic.CommandPolicyViolation ||
		diagnostic.RecommendedEvidenceTargetType != runtimemodel.ValidationEvidenceTargetCommandContract {
		t.Fatalf("validationFailureDiagnostic = %+v, want command invalid policy diagnostic", diagnostic)
	}
}

func TestRuntimeAdapter_ExecuteOperationReleasesManagedValidationScratchOnPolicyReject(t *testing.T) {
	adapter := NewRuntimeAdapter()
	deliverableDirectory := t.TempDir()
	scratchDirectory := filepath.Join(os.TempDir(), "workorch-shell-managed-validation-test", strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()))
	if err := os.MkdirAll(scratchDirectory, 0o755); err != nil {
		t.Fatalf("mkdir scratch: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(scratchDirectory)
	})

	result, err := adapter.ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceID:                   "ws-managed-validation-policy-reject",
		RunID:                         "run-managed-validation-policy-reject",
		StepID:                        "run-managed-validation-policy-reject/verify",
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Command:                       "kill",
		Args:                          []string{"123"},
		ValidationScratchDirectory:    scratchDirectory,
		ManagedValidation:             runtimemodel.RuntimeManagedValidation{RuntimeManaged: true, AutoCleanup: true, AutoCleanupSet: true},
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("ExecuteOperation err = %v, want invalid argument", err)
	}
	if result.Stage != "shell_command_policy_rejected" {
		t.Fatalf("stage = %v, want shell_command_policy_rejected", result.Stage)
	}
	resources := result.ValidationResources
	if len(resources) != 1 {
		t.Fatalf("validationResources = %#v, want released scratch resource", result.ValidationResources)
	}
	if resources[0].Kind != "scratch_dir" || resources[0].Status != "released" {
		t.Fatalf("resource = %+v, want released scratch_dir", resources[0])
	}
	managed := result.ManagedValidation
	if !managed.ResourcesReleased {
		t.Fatalf("managedValidation = %+v, want resources released", managed)
	}
	if _, statErr := os.Stat(scratchDirectory); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("scratchDirectory stat = %v, want removed by policy rejection cleanup", statErr)
	}
}

func TestRuntimeAdapter_ExecuteOperationAllowsManagedValidationProcessLifecycle(t *testing.T) {
	adapter := NewRuntimeAdapter()
	deliverableDirectory := t.TempDir()
	scratchDirectory := filepath.Join(os.TempDir(), "workorch-shell-managed-validation-test", strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()))
	if err := os.MkdirAll(scratchDirectory, 0o755); err != nil {
		t.Fatalf("mkdir scratch: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(scratchDirectory)
	})

	result, err := adapter.ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceID:                   "ws-managed-validation-process",
		RunID:                         "run-managed-validation-process",
		StepID:                        "run-managed-validation-process/verify",
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Command:                       "sh",
		Args:                          []string{"-c", "sleep 10 & pid=$!; sleep 0.1; kill $pid; wait $pid 2>/dev/null || true; printf managed-ok"},
		ValidationScratchDirectory:    scratchDirectory,
		ManagedValidation:             runtimemodel.RuntimeManagedValidation{RuntimeManaged: true, AutoCleanup: true, AutoCleanupSet: true},
	})
	if err != nil {
		t.Fatalf("ExecuteOperation err = %v, want managed validation process lifecycle allowed", err)
	}
	if result.Status != "completed" || !strings.Contains(fmt.Sprint(result.Stdout), "managed-ok") {
		t.Fatalf("output = %+v, want completed managed validation shell", result)
	}
	managed := result.ManagedValidation
	if !managed.ResourcesReleased {
		t.Fatalf("managedValidation = %+v, want resources released", managed)
	}
}

func TestRuntimeAdapter_ExecuteOperationRejectsManagedValidationGlobalProcessTermination(t *testing.T) {
	adapter := NewRuntimeAdapter()
	_, err := adapter.ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: t.TempDir(),
		Command:                       "sh",
		Args:                          []string{"-c", "pkill -f workorch || true"},
		ManagedValidation:             runtimemodel.RuntimeManagedValidation{RuntimeManaged: true},
	})
	if err == nil {
		t.Fatal("ExecuteOperation err = nil, want managed validation to reject global process termination")
	}
}

func TestRuntimeAdapter_ExecuteOperationRejectsManagedValidationBackgroundWithoutWait(t *testing.T) {
	adapter := NewRuntimeAdapter()
	_, err := adapter.ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: t.TempDir(),
		Command:                       "sh",
		Args:                          []string{"-c", "sleep 10 & printf started"},
		ManagedValidation:             runtimemodel.RuntimeManagedValidation{RuntimeManaged: true},
	})
	if err == nil {
		t.Fatal("ExecuteOperation err = nil, want managed validation to reject background process without wait")
	}
}

func TestRuntimeAdapter_ExecuteOperationBoundsCommandOutput(t *testing.T) {
	adapter := NewRuntimeAdapter()
	deliverableDirectory := t.TempDir()

	result, err := adapter.ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Command:                       "sh",
		Args:                          []string{"-c", "yes x | head -c 70000"},
	})
	if err != nil {
		t.Fatalf("ExecuteOperation: %v", err)
	}
	stdout := result.Stdout
	if len([]byte(stdout)) != MaxCommandOutputBytes {
		t.Fatalf("stdout bytes = %d, want %d", len([]byte(stdout)), MaxCommandOutputBytes)
	}
	if !result.OutputTruncated || !result.StdoutTruncated {
		t.Fatalf("output = %+v, want stdout truncation metadata", result)
	}
	if result.StdoutBytes != 70000 {
		t.Fatalf("stdoutBytes = %v, want 70000", result.StdoutBytes)
	}
}

func TestRuntimeAdapterAppliesConfiguredZeroWorkEvidencePolicy(t *testing.T) {
	adapter := NewRuntimeAdapter()
	deliverableDirectory := t.TempDir()
	result, err := adapter.ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Command:                       "sh",
		Args:                          []string{"-c", "printf 'testing: warning: no tests to run\\nPASS\\n'"},
		EvidencePolicies: []runtimemodel.RuntimeEvidencePolicy{{
			ID: "test-zero-work", AppliesTo: "shell/command", CommandName: "sh", ArgsContain: []string{"-c"},
			ZeroWorkMarkers: []string{"no tests to run"}, QualityOnZeroWork: string(common.EvidenceQualityInconclusive),
			EvidenceReason: "configured zero-work marker matched", EvidenceTags: []string{"test", "zero_work"},
		}},
	})
	if err != nil {
		t.Fatalf("ExecuteOperation: %v", err)
	}
	if result.EvidenceQuality != string(common.EvidenceQualityInconclusive) ||
		result.ZeroWork != true ||
		result.EvidenceReason != "configured zero-work marker matched" {
		t.Fatalf("output = %+v, want configured inconclusive zero-work evidence", result)
	}
}

func TestRuntimeAdapterPrefersWorkEvidenceMarkersOverZeroWorkMarkers(t *testing.T) {
	adapter := NewRuntimeAdapter()
	deliverableDirectory := t.TempDir()
	result, err := adapter.ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Command:                       "sh",
		Args:                          []string{"-c", "printf '? pkg [no test files]\\n=== RUN   TestExample\\n--- PASS: TestExample (0.00s)\\nok  pkg 0.001s\\n'"},
		EvidencePolicies: []runtimemodel.RuntimeEvidencePolicy{{
			ID: "test-work-wins", AppliesTo: "shell/command", CommandName: "sh", ArgsContain: []string{"-c"},
			ZeroWorkMarkers: []string{"[no test files]"}, WorkEvidenceMarkers: []string{"=== RUN", "--- PASS:", "ok  "},
			QualityOnZeroWork: string(common.EvidenceQualityInconclusive), EvidenceReason: "configured zero-work marker matched",
			EvidenceTags: []string{"test", "zero_work"},
		}},
	})
	if err != nil {
		t.Fatalf("ExecuteOperation: %v", err)
	}
	if result.ZeroWork == true {
		t.Fatalf("output = %+v, want work evidence to override zero-work marker", result)
	}
	if result.EvidenceQuality == string(common.EvidenceQualityInconclusive) {
		t.Fatalf("output = %+v, want evidence quality not inconclusive", result)
	}
}

func TestRuntimeAdapterPreservesNonzeroFailureWhenOtherPackageReportsZeroWork(t *testing.T) {
	adapter := NewRuntimeAdapter()
	result, err := adapter.ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: t.TempDir(),
		Command:                       "sh",
		Args: []string{"-c", "printf 'FAIL github.com/user/lsp-bridge/cmd/lsp-bridge [setup failed]\\n" +
			"? github.com/user/lsp-bridge/internal/config [no test files]\\nFAIL\\n'; exit 1"},
		EvidencePolicies: []runtimemodel.RuntimeEvidencePolicy{{
			ID: "test-zero-work", AppliesTo: "shell/command", CommandName: "sh", ArgsContain: []string{"-c"},
			ZeroWorkMarkers: []string{"[no test files]"}, WorkEvidenceMarkers: []string{"=== RUN", "--- PASS:", "ok  "},
			QualityOnZeroWork: string(common.EvidenceQualityInconclusive), EvidenceReason: "configured zero-work marker matched",
			EvidenceTags: []string{"test", "zero_work"},
		}},
	})
	if err == nil {
		t.Fatal("ExecuteOperation err = nil, want nonzero command failure")
	}
	if result.ExitCode != 1 || result.Status != "failed" {
		t.Fatalf("result status = %q exitCode = %d, want failed/1", result.Status, result.ExitCode)
	}
	if result.ZeroWork || result.Evidence.ZeroWork {
		t.Fatalf("result = %+v, want nonzero failure to remain non-zero-work", result)
	}
	if result.EvidenceQuality == string(common.EvidenceQualityInconclusive) || result.Evidence.Quality == common.EvidenceQualityInconclusive {
		t.Fatalf("result = %+v, want nonzero failure not to be classified as inconclusive", result)
	}
	if result.EvidenceReason == "configured zero-work marker matched" || len(result.EvidenceTags) != 0 ||
		result.Evidence.EvidenceReason == "configured zero-work marker matched" || len(result.Evidence.EvidenceTags) != 0 {
		t.Fatalf("result = %+v, want zero-work policy metadata absent", result)
	}
	if result.ValidationFailureDiagnostic == nil ||
		result.ValidationFailureDiagnostic.Class != runtimemodel.ValidationDiagnosticBuildFailed ||
		result.ValidationFailureDiagnostic.RecommendedEvidenceTargetType != runtimemodel.ValidationEvidenceTargetFailureSurface {
		t.Fatalf("diagnostic = %+v, want validation build failure", result.ValidationFailureDiagnostic)
	}
	if result.RuntimeDiagnostics.ValidationFailureDiagnostic == nil ||
		result.RuntimeDiagnostics.ValidationFailureDiagnostic.Class != runtimemodel.ValidationDiagnosticBuildFailed {
		t.Fatalf("runtime diagnostic = %+v, want validation build failure", result.RuntimeDiagnostics.ValidationFailureDiagnostic)
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

func TestRuntimeAdapter_ExecuteOperationReturnsTimeoutOutput(t *testing.T) {
	adapter := NewRuntimeAdapterWithTimeout(10 * time.Millisecond)
	deliverableDirectory := t.TempDir()

	result, err := adapter.ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Command:                       "sh",
		Args:                          []string{"-c", "sleep 1"},
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ExecuteOperation err = %v, want deadline exceeded", err)
	}
	if result.Status != "timeout" {
		t.Fatalf("status = %v, want timeout", result.Status)
	}
	if result.WorkingDirectory != deliverableDirectory {
		t.Fatalf("workingDir = %v, want %s", result.WorkingDirectory, deliverableDirectory)
	}
	diagnostic := result.ValidationFailureDiagnostic
	if diagnostic == nil {
		t.Fatal("validationFailureDiagnostic = nil, want timeout diagnostic")
	}
	if diagnostic.Class != runtimemodel.ValidationDiagnosticTimeout || !diagnostic.TimedOut {
		t.Fatalf("validationFailureDiagnostic = %+v, want timeout diagnostic", diagnostic)
	}
}

func TestRuntimeAdapter_ExecuteOperationTimeoutKillsProcessGroup(t *testing.T) {
	adapter := NewRuntimeAdapterWithTimeout(50 * time.Millisecond)
	deliverableDirectory := t.TempDir()

	startedAt := time.Now()
	result, err := adapter.ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Command:                       "sh",
		Args:                          []string{"-c", "sleep 5 & wait"},
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ExecuteOperation err = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(startedAt); elapsed > time.Second {
		t.Fatalf("ExecuteOperation elapsed = %s, want process group timeout without waiting for background child", elapsed)
	}
	if result.Status != "timeout" {
		t.Fatalf("status = %v, want timeout", result.Status)
	}
}

func TestRuntimeAdapter_ExecuteOperationReleasesManagedValidationResourcesOnTimeout(t *testing.T) {
	adapter := NewRuntimeAdapterWithTimeout(50 * time.Millisecond)
	deliverableDirectory := t.TempDir()
	scratchDirectory := filepath.Join(os.TempDir(), "workorch-shell-managed-validation-test", strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()))
	t.Cleanup(func() {
		_ = os.RemoveAll(scratchDirectory)
	})

	result, err := adapter.ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceID:                   "ws-managed-validation-timeout",
		RunID:                         "run-managed-validation-timeout",
		StepID:                        "run-managed-validation-timeout/verify",
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Command:                       "sh",
		Args:                          []string{"-c", "mkdir -p \"$1\"; printf pending > \"$1/out.txt\"; sleep 5", "sh", scratchDirectory},
		ValidationScratchDirectory:    scratchDirectory,
		ManagedValidation:             runtimemodel.RuntimeManagedValidation{RuntimeManaged: true, AutoCleanup: true, AutoCleanupSet: true},
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ExecuteOperation err = %v, want deadline exceeded", err)
	}
	if result.Status != "timeout" {
		t.Fatalf("status = %v, want timeout", result.Status)
	}
	resources := result.ValidationResources
	if len(resources) != 2 {
		t.Fatalf("validationResources = %#v, want process and scratch records", result.ValidationResources)
	}
	statuses := map[string]string{}
	for _, resource := range resources {
		statuses[resource.Kind] = resource.Status
		if resource.WorkspaceID != "ws-managed-validation-timeout" ||
			resource.RunID != "run-managed-validation-timeout" ||
			resource.StepID != "run-managed-validation-timeout/verify" {
			t.Fatalf("resource = %+v, want scoped workspace/run/step ids", resource)
		}
		if strings.TrimSpace(resource.ReleasedAt) == "" {
			t.Fatalf("resource = %+v, want releasedAt diagnostic", resource)
		}
	}
	if statuses["process"] != "timeout" || statuses["scratch_dir"] != "released" {
		t.Fatalf("resource statuses = %+v, want timeout process and released scratch", statuses)
	}
	managed := result.ManagedValidation
	if managed.ResourceCount != 2 || !managed.ResourcesReleased {
		t.Fatalf("managedValidation = %+v, want resources released after timeout cleanup", managed)
	}
	if !result.RuntimeDiagnostics.ValidationResourceLifecycle.Released {
		t.Fatalf("runtimeDiagnostics = %+v, want released lifecycle diagnostic", result.RuntimeDiagnostics)
	}
	if _, statErr := os.Stat(scratchDirectory); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("scratchDirectory stat = %v, want removed after timeout cleanup", statErr)
	}
}

func TestRuntimeAdapter_ExecuteOperationUsesInputTimeoutMillis(t *testing.T) {
	adapter := NewRuntimeAdapterWithTimeout(time.Second)
	deliverableDirectory := t.TempDir()

	result, err := adapter.ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Command:                       "sh",
		Args:                          []string{"-c", "sleep 0.2; echo done"},
		TimeoutMillis:                 2000,
	})
	if err != nil {
		t.Fatalf("ExecuteOperation: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("status = %v, want completed", result.Status)
	}
	if result.TimeoutMillis != int64(2000) {
		t.Fatalf("timeoutMillis = %v, want 2000", result.TimeoutMillis)
	}
	stdout := result.Stdout
	if strings.TrimSpace(stdout) != "done" {
		t.Fatalf("stdout = %q, want done", stdout)
	}
}

func TestRuntimeAdapter_ExecuteOperationNormalizesShPipefailScriptToBash(t *testing.T) {
	adapter := NewRuntimeAdapter()
	deliverableDirectory := t.TempDir()

	result, err := adapter.ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Command:                       "sh",
		Args:                          []string{"-c", "set -euo pipefail\nprintf ok"},
	})
	if err != nil {
		t.Fatalf("ExecuteOperation: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("status = %v, want completed", result.Status)
	}
	if result.Command != "sh" || result.ActualCommand != "bash" {
		t.Fatalf("command fields = %+v, want requested sh executed by bash", result)
	}
	if result.CommandNormalization != "sh_script_requires_bash_pipefail" {
		t.Fatalf("commandNormalization = %v, want pipefail normalization", result.CommandNormalization)
	}
	stdout := result.Stdout
	if strings.TrimSpace(stdout) != "ok" {
		t.Fatalf("stdout = %q, want ok", stdout)
	}
}

func TestRuntimeAdapter_ExecuteOperationNormalizesShPipeStatusScriptToBash(t *testing.T) {
	adapter := NewRuntimeAdapter()
	deliverableDirectory := t.TempDir()

	result, err := adapter.ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Command:                       "sh",
		Args:                          []string{"-c", `false | true; printf 'first_status=%s\n' "${PIPESTATUS[0]}"`},
	})
	if err != nil {
		t.Fatalf("ExecuteOperation: %v output=%+v", err, result)
	}
	if result.Status != "completed" {
		t.Fatalf("status = %v, want completed", result.Status)
	}
	if result.Command != "sh" || result.ActualCommand != "bash" {
		t.Fatalf("command fields = %+v, want requested sh executed by bash", result)
	}
	if result.CommandNormalization != "sh_script_requires_bash_pipestatus" {
		t.Fatalf("commandNormalization = %v, want pipestatus normalization", result.CommandNormalization)
	}
	stdout := result.Stdout
	if strings.TrimSpace(stdout) != "first_status=1" {
		t.Fatalf("stdout = %q, want first_status=1", stdout)
	}
}

func TestRuntimeAdapter_ExecuteOperationWrapsCompoundCommandWithShell(t *testing.T) {
	adapter := NewRuntimeAdapter()
	deliverableDirectory := t.TempDir()

	result, err := adapter.ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: deliverableDirectory,
		Command:                       "cd",
		Args:                          []string{".", "&&", "pwd"},
	})
	if err != nil {
		t.Fatalf("ExecuteOperation: %v", err)
	}
	if result.CommandNormalization != "compound_command_wrapped_with_sh_c" {
		t.Fatalf("commandNormalization = %v, want compound command normalization", result.CommandNormalization)
	}
	stdout := result.Stdout
	if strings.TrimSpace(stdout) != deliverableDirectory {
		t.Fatalf("stdout = %q, want deliverable directory %q", stdout, deliverableDirectory)
	}
}

func TestRuntimeAdapter_ExecuteOperationRejectsDangerousCompoundCommandAfterWrapping(t *testing.T) {
	adapter := NewRuntimeAdapter()
	_, err := adapter.ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: t.TempDir(),
		Command:                       "echo",
		Args:                          []string{"ok", "&&", "sudo", "true"},
	})
	if err == nil {
		t.Fatal("ExecuteOperation err = nil, want invalid argument")
	}
}

func TestRuntimeAdapter_ExecuteOperationRejectsInvalidInputTimeoutMillis(t *testing.T) {
	adapter := NewRuntimeAdapter()
	_, err := adapter.ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: t.TempDir(),
		Command:                       "pwd",
		TimeoutMillis:                 500,
	})
	if err == nil {
		t.Fatal("ExecuteOperation err = nil, want invalid argument")
	}
}

func TestRuntimeAdapter_ExecuteOperationRejectsDangerousCommands(t *testing.T) {
	adapter := NewRuntimeAdapter()
	cases := []struct {
		name    string
		command string
		args    []string
	}{
		{name: "sudo", command: "sudo", args: []string{"true"}},
		{name: "rm root", command: "rm", args: []string{"-rf", "/"}},
		{name: "rm git", command: "rm", args: []string{"-rf", ".git"}},
		{name: "chmod 777", command: "chmod", args: []string{"777", "file.txt"}},
		{name: "git force push", command: "git", args: []string{"push", "--force"}},
		{name: "shell script", command: "sh", args: []string{"-c", "rm -rf /tmp/project/.git"}},
		{name: "direct kill", command: "kill", args: []string{"1234"}},
		{name: "direct pkill", command: "pkill", args: []string{"workorch"}},
		{name: "direct killall", command: "killall", args: []string{"workorch"}},
		{name: "fuser kill port", command: "fuser", args: []string{"-k", "8080/tcp"}},
		{name: "shell fuser kill port", command: "sh", args: []string{"-c", "fuser -k 8080/tcp 2>/dev/null || true"}},
		{name: "shell lsof xargs kill", command: "sh", args: []string{"-c", "lsof -ti :8080 | xargs kill -9"}},
		{name: "shell detached background", command: "sh", args: []string{"-c", "./service >/tmp/service.log 2>&1 & echo $!"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := adapter.ExecuteOperation(context.Background(), RuntimeCommandRequest{
				WorkspaceDeliverableDirectory: t.TempDir(),
				Command:                       tc.command,
				Args:                          tc.args,
			})
			if err == nil {
				t.Fatal("ExecuteOperation err = nil, want invalid argument")
			}
		})
	}
}
