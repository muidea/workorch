package result

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
)

const (
	resultLifecycleCrashHelperEnv = "WORKORCH_TEST_RESULT_LIFECYCLE_CRASH_HELPER"
	resultLifecycleCrashRootEnv   = "WORKORCH_TEST_RESULT_LIFECYCLE_CRASH_ROOT"
	resultLifecycleCrashCutEnv    = "WORKORCH_TEST_RESULT_LIFECYCLE_CRASH_CUT"
	resultLifecycleCrashMarkerEnv = "WORKORCH_TEST_RESULT_LIFECYCLE_CRASH_MARKER"
)

type resultLifecycleCrashResultRepository struct {
	repository.ResultRepository
	cut    string
	marker string
}

func (r *resultLifecycleCrashResultRepository) Create(ctx context.Context, item *resultmodel.Result) error {
	err := r.ResultRepository.Create(ctx, item)
	if err == nil && r.cut == "draft_result" {
		resultLifecycleCrashAfterDurableWrite(r.marker)
	}
	return err
}

func (r *resultLifecycleCrashResultRepository) CompareAndSwap(ctx context.Context, item *resultmodel.Result, expectedRevision uint64) error {
	err := r.ResultRepository.CompareAndSwap(ctx, item, expectedRevision)
	if err == nil && r.cut == "terminal_result" {
		resultLifecycleCrashAfterDurableWrite(r.marker)
	}
	return err
}

type resultLifecycleCrashArtifactRepository struct {
	repository.ArtifactRepository
	cutAfter int
	writes   int
	marker   string
}

func (r *resultLifecycleCrashArtifactRepository) Create(ctx context.Context, item *resultmodel.Artifact) error {
	err := r.ArtifactRepository.Create(ctx, item)
	if err != nil {
		return err
	}
	r.writes++
	if r.cutAfter > 0 && r.writes == r.cutAfter {
		resultLifecycleCrashAfterDurableWrite(r.marker)
	}
	return nil
}

func resultLifecycleCrashAfterDurableWrite(marker string) {
	if err := os.WriteFile(marker, []byte("durable\n"), 0o600); err != nil {
		panic(err)
	}
	select {}
}

func TestResultArtifactDurableWriteCutsRecoverAfterProcessKill(t *testing.T) {
	if os.Getenv(resultLifecycleCrashHelperEnv) == "1" {
		resultLifecycleCrashProcessHelper(t)
		return
	}

	for _, cut := range []string{"draft_result", "artifact_1", "artifact_2", "artifact_3", "terminal_result"} {
		cut := cut
		t.Run(cut, func(t *testing.T) {
			root := t.TempDir()
			marker := filepath.Join(root, "crash-ready")
			command := exec.Command(os.Args[0], "-test.run=^TestResultArtifactDurableWriteCutsRecoverAfterProcessKill$", "-test.count=1")
			command.Env = append(os.Environ(),
				resultLifecycleCrashHelperEnv+"=1",
				resultLifecycleCrashRootEnv+"="+root,
				resultLifecycleCrashCutEnv+"="+cut,
				resultLifecycleCrashMarkerEnv+"="+marker,
			)
			var output bytes.Buffer
			command.Stdout = &output
			command.Stderr = &output
			if err := command.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if command.ProcessState == nil {
					_ = command.Process.Kill()
					_ = command.Wait()
				}
			}()
			waitForResultLifecycleCrashMarker(t, marker, command, &output)
			if err := command.Process.Kill(); err != nil {
				t.Fatalf("kill helper at %s: %v", cut, err)
			}
			if err := command.Wait(); err == nil {
				t.Fatalf("helper at %s exited successfully, want process kill", cut)
			} else {
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) {
					t.Fatalf("wait helper at %s: %v", cut, err)
				}
			}

			assertResultLifecycleCrashRecovery(t, root, cut)
		})
	}
}

func resultLifecycleCrashProcessHelper(t *testing.T) {
	t.Helper()
	root := os.Getenv(resultLifecycleCrashRootEnv)
	cut := os.Getenv(resultLifecycleCrashCutEnv)
	marker := os.Getenv(resultLifecycleCrashMarkerEnv)
	if root == "" || cut == "" || marker == "" {
		t.Fatal("missing Result lifecycle crash helper input")
	}
	cutAfter := 0
	if len(cut) > len("artifact_") && cut[:len("artifact_")] == "artifact_" {
		value, err := strconv.Atoi(cut[len("artifact_"):])
		if err != nil {
			t.Fatalf("decode artifact cut %q: %v", cut, err)
		}
		cutAfter = value
	}
	repositories := workorchrepo.NewWorkspaceScopedFileRepositorySet(root)
	results := &resultLifecycleCrashResultRepository{ResultRepository: repositories.Results, cut: cut, marker: marker}
	artifacts := &resultLifecycleCrashArtifactRepository{ArtifactRepository: repositories.Artifacts, cutAfter: cutAfter, marker: marker}
	hub := magicevent.NewHub(16)
	owner, err := New(hub, nil, Dependencies{
		OwnerRepositories: resultBizTestOwnerRepositories(results, artifacts, repositories.ResultDeletions),
		EventHub:          hub,
	})
	if err != nil {
		t.Fatal(err)
	}
	command := terminalDeliveryFixture("delivery-process-kill", resultLifecycleCrashArtifactRefs())
	if _, err := owner.applyTerminalDelivery(context.Background(), command); err != nil {
		t.Fatalf("apply terminal delivery before %s cut: %v", cut, err)
	}
	t.Fatalf("Result lifecycle helper passed %s without reaching the crash cut", cut)
}

func waitForResultLifecycleCrashMarker(t *testing.T, marker string, command *exec.Cmd, output *bytes.Buffer) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(marker); err == nil {
			return
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("stat crash marker: %v", err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	_ = command.Process.Kill()
	_ = command.Wait()
	t.Fatalf("Result lifecycle helper did not reach durable cut; output: %s", output.String())
}

func assertResultLifecycleCrashRecovery(t *testing.T, root, cut string) {
	t.Helper()
	ctx := context.Background()
	repositories := workorchrepo.NewWorkspaceScopedFileRepositorySet(root)
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner, err := New(hub, nil, Dependencies{OwnerRepositories: repositories.ResultOwner, EventHub: hub})
	if err != nil {
		t.Fatalf("reopen Result owner after %s: %v", cut, err)
	}
	command := terminalDeliveryFixture("delivery-process-kill", resultLifecycleCrashArtifactRefs())
	recovered, err := owner.applyTerminalDelivery(ctx, command)
	if err != nil || recovered.Result == nil || recovered.Result.State != common.ResultStateAccepted || recovered.DeliveryAck == nil {
		t.Fatalf("recover %s: result=%+v err=%v", cut, recovered, err)
	}
	resultID := canonicalRuntimeResultID(command.WorkspaceID, command.RunID)
	scoped := common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID)
	stored, err := repositories.Results.Get(scoped, resultID)
	if err != nil || stored.State != common.ResultStateAccepted || stored.Revision != 2 || len(stored.ArtifactRefs) != 3 {
		t.Fatalf("stored Result after %s: %+v err=%v", cut, stored, err)
	}
	children, err := repositories.Artifacts.ListByResult(scoped, resultID)
	if err != nil || len(children) != 3 {
		t.Fatalf("stored Artifacts after %s: %+v err=%v", cut, children, err)
	}
	expected := terminalArtifacts(command, resultID, command.Material.Normalized().ArtifactRefs)
	byID := make(map[common.ArtifactID]*resultmodel.Artifact, len(children))
	for _, child := range children {
		if _, duplicate := byID[child.ID]; duplicate {
			t.Fatalf("duplicate Artifact after %s: %q", cut, child.ID)
		}
		byID[child.ID] = child
	}
	for _, want := range expected {
		if !sameTerminalArtifact(byID[want.ID], want) {
			t.Fatalf("Artifact material drift after %s: got=%+v want=%+v", cut, byID[want.ID], want)
		}
	}
	if replayed, err := owner.applyTerminalDelivery(ctx, command); err != nil || replayed.Transitioned {
		t.Fatalf("exact replay after %s: result=%+v err=%v", cut, replayed, err)
	}
	unchanged, err := repositories.Results.Get(scoped, resultID)
	if err != nil || unchanged.Revision != stored.Revision {
		t.Fatalf("exact replay changed Result after %s: before=%d after=%+v err=%v", cut, stored.Revision, unchanged, err)
	}
}

func resultLifecycleCrashArtifactRefs() []common.ArtifactID {
	return []common.ArtifactID{
		"workorch://artifact/process-kill-a",
		"workorch://artifact/process-kill-b",
		"workorch://artifact/process-kill-c",
	}
}

var _ repository.ResultRepository = (*resultLifecycleCrashResultRepository)(nil)
var _ repository.ArtifactRepository = (*resultLifecycleCrashArtifactRepository)(nil)
