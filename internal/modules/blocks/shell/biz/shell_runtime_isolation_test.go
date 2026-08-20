package shell

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestManagedValidationRunsInDiscardedWorkspaceCopy(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is not available")
	}
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "fixture.go"), []byte("package fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(workspace, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := NewRuntimeAdapter().ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: workspace,
		ValidationRecipeID:            "isolated-write",
		ManagedValidation: runtimecontract.RuntimeManagedValidation{
			RuntimeManaged: true, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession,
		},
		Command: "sh", Args: []string{"-c", "printf changed > generated.txt; mkdir -p .git; printf changed > .git/HEAD; printf 'WORKORCH_CHECK_PASSED=isolated/write\\n'"},
		TargetRefs: []string{"fixture.go"},
	})
	if err != nil {
		t.Fatalf("ExecuteOperation: %v output=%+v", err, result)
	}
	if _, err := os.Stat(filepath.Join(workspace, "generated.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("managed validation wrote authoritative workspace: %v", err)
	}
	head, err := os.ReadFile(filepath.Join(workspace, ".git", "HEAD"))
	if err != nil || string(head) != "ref: refs/heads/main\n" {
		t.Fatalf("managed validation changed authoritative .git: head=%q err=%v", head, err)
	}
	if shellWorkspaceTargetChangesChanged(result.WorkspaceObservedTargetChanges) {
		t.Fatalf("managed validation reported authoritative workspace changes: %+v", result.WorkspaceObservedTargetChanges)
	}
	if !result.ManagedValidation.ResourcesReleased || len(result.ValidationResources) == 0 {
		t.Fatalf("managed validation resources were not released: %+v", result.ValidationResources)
	}
	for _, resource := range result.ValidationResources {
		if resource.Kind == "scratch_dir" {
			if _, statErr := os.Stat(resource.Path); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("managed validation scratch was retained: path=%q err=%v", resource.Path, statErr)
			}
		}
	}
}

func TestManagedValidationRejectsWorkspaceSymlinkBeforeExecution(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is not available")
	}
	workspace := t.TempDir()
	external := filepath.Join(t.TempDir(), "external.txt")
	if err := os.WriteFile(external, []byte("safe\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(workspace, "external-link")); err != nil {
		t.Fatal(err)
	}
	result, err := NewRuntimeAdapter().ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: workspace,
		ValidationRecipeID:            "symlink-escape",
		ManagedValidation: runtimecontract.RuntimeManagedValidation{
			RuntimeManaged: true, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession,
		},
		Command: "sh", Args: []string{"-c", "printf escaped > external-link; printf 'WORKORCH_CHECK_PASSED=symlink/escape\\n'"},
	})
	if err == nil || result.Stage != "shell_workspace_scope_rejected" || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("managed validation symlink escape result=%+v err=%v", result, err)
	}
	content, readErr := os.ReadFile(external)
	if readErr != nil || string(content) != "safe\n" {
		t.Fatalf("symlink escape changed external file: content=%q err=%v", content, readErr)
	}
}

func TestStateChangingShellObservesUndeclaredGitControlMutation(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is not available")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	workspace := t.TempDir()
	initCommand := exec.Command("git", "init", "-q", workspace)
	if output, err := initCommand.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	result, err := NewRuntimeAdapter().ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: workspace,
		Command:                       "sh",
		Args:                          []string{"-c", "printf delivered > generated.txt; printf 'ref: refs/heads/other\\n' > .git/HEAD"},
		StateChanging:                 true,
		TargetRef:                     "generated.txt",
		TargetRefs:                    []string{"generated.txt"},
	})
	if err != nil {
		t.Fatalf("ExecuteOperation: %v output=%+v", err, result)
	}
	undeclared := shellUndeclaredWorkspaceTargets(result.WorkspaceObservedTargetChanges, []string{"generated.txt"})
	if len(undeclared) != 1 || undeclared[0] != ".git/HEAD" {
		t.Fatalf("Git control mutation was not observed as undeclared: changes=%+v undeclared=%v", result.WorkspaceObservedTargetChanges, undeclared)
	}
}

func TestShellKillsBackgroundProcessGroupAfterNormalCompletion(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is not available")
	}
	result, err := NewRuntimeAdapter().ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: t.TempDir(),
		ValidationRecipeID:            "process-group-cleanup",
		ManagedValidation: runtimecontract.RuntimeManagedValidation{
			RuntimeManaged: true, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession,
		},
		Command: "sh", Args: []string{"-c", "sleep 30 >/dev/null 2>&1&child=$!; printf 'WORKORCH_CHECK_PASSED=process/cleanup\\n%s\\n' \"$child\""},
	})
	if err != nil {
		t.Fatalf("ExecuteOperation: %v output=%+v", err, result)
	}
	lines := strings.Fields(result.Stdout)
	if len(lines) < 2 {
		t.Fatalf("background process PID was not reported: stdout=%q", result.Stdout)
	}
	pid, parseErr := strconv.Atoi(lines[len(lines)-1])
	if parseErr != nil || pid <= 0 {
		t.Fatalf("background process PID = %q err=%v", lines[len(lines)-1], parseErr)
	}
	deadline := time.Now().Add(3 * time.Second)
	for shellTestProcessRunning(pid) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if shellTestProcessRunning(pid) {
		_ = syscall.Kill(pid, syscall.SIGKILL)
		t.Fatalf("background process %d survived normal Shell completion", pid)
	}
}

func shellTestProcessRunning(pid int) bool {
	err := syscall.Kill(pid, 0)
	if errors.Is(err, syscall.ESRCH) {
		return false
	}
	if err != nil {
		return true
	}
	stat, readErr := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if readErr == nil {
		fields := strings.Fields(string(stat))
		if len(fields) > 2 && fields[2] == "Z" {
			return false
		}
	}
	return true
}

func TestManagedValidationSandboxRejectsUnsafeExplicitScratch(t *testing.T) {
	workspace := t.TempDir()
	request := RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: workspace,
		ValidationScratchDirectory:    filepath.Join(workspace, "scratch"),
		ManagedValidation: runtimecontract.RuntimeManagedValidation{
			RuntimeManaged: true, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession,
		},
	}
	if _, err := prepareManagedValidationSandbox(&request, workspace); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("workspace-local managed validation scratch error = %v", err)
	}
}

func TestManagedValidationEnvironmentIsolatesSessionAndKeepsTechnicalGoCaches(t *testing.T) {
	scratch := t.TempDir()
	values := managedValidationEnvironment([]string{
		"HOME=/host/home",
		"PATH=/usr/bin",
		"GOCACHE=/host/cache/go-build",
		"GOMODCACHE=/host/go/pkg/mod",
	}, scratch)
	environment := make(map[string]string, len(values))
	for _, value := range values {
		key, item, found := strings.Cut(value, "=")
		if found {
			environment[key] = item
		}
	}
	if environment["HOME"] != filepath.Join(scratch, "home") || environment["TMPDIR"] != filepath.Join(scratch, "tmp") ||
		environment["XDG_CACHE_HOME"] != filepath.Join(scratch, "cache") {
		t.Fatalf("managed validation session environment was not isolated: %+v", environment)
	}
	if environment["GOCACHE"] != "/host/cache/go-build" || environment["GOMODCACHE"] != "/host/go/pkg/mod" {
		t.Fatalf("managed validation discarded owner technical Go caches: %+v", environment)
	}
}
