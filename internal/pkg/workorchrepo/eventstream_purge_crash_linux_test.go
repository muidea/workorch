package workorchrepo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
	"unsafe"

	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
	"golang.org/x/sys/unix"
)

const (
	eventStreamPurgeCrashHelperEnv = "WORKORCH_TEST_EVENTSTREAM_PURGE_CRASH_HELPER"
	eventStreamPurgeCrashRootEnv   = "WORKORCH_TEST_EVENTSTREAM_PURGE_CRASH_ROOT"
)

func TestFileEventStreamPurgeCrashProcessHelper(t *testing.T) {
	if os.Getenv(eventStreamPurgeCrashHelperEnv) != "1" {
		return
	}
	root := os.Getenv(eventStreamPurgeCrashRootEnv)
	if root == "" {
		t.Fatal("missing crash helper root")
	}
	repos := NewWorkspaceScopedFileRepositorySet(root)
	fence, err := repos.EventStreamDeletions.Get(context.Background(), "ws-eventstream-crash-target")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repos.EventStreamPurger.PurgeWorkspaceEventRecords(context.Background(), *fence); err != nil {
		t.Fatal(err)
	}
}

func TestFileEventStreamPurgeCrashBetweenWorkspaceScopedPathsRecoversExactly(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	repos := NewWorkspaceScopedFileRepositorySet(dataRoot)
	targetWorkspace := common.WorkspaceID("ws-eventstream-crash-target")
	otherWorkspace := common.WorkspaceID("ws-eventstream-crash-other")
	targetRuns := []common.RunID{"run-a-target", "run-b-target"}
	largeOtherSystem := "other-system-" + strings.Repeat("s", 10*1024)
	largeOtherRun := "other-run-" + strings.Repeat("r", 11*1024)
	targetSecrets := []string{
		"target-system-" + strings.Repeat("x", 9*1024),
		"target-a-" + strings.Repeat("a", 9*1024),
		"target-b-" + strings.Repeat("b", 9*1024),
	}
	records := []*eventstreammodel.EventRecord{
		{ID: "event-target-system", Topic: "/target/system", WorkspaceID: targetWorkspace, Revision: 1,
			Payload: mustWorkspaceScopedPayload(t, map[string]any{"content": targetSecrets[0]})},
		{ID: "event-target-run-a", Topic: "/target/run-a", WorkspaceID: targetWorkspace, RunID: targetRuns[0], Revision: 2,
			Payload: mustWorkspaceScopedPayload(t, map[string]any{"content": targetSecrets[1]})},
		{ID: "event-target-run-b", Topic: "/target/run-b", WorkspaceID: targetWorkspace, RunID: targetRuns[1], Revision: 3,
			Payload: mustWorkspaceScopedPayload(t, map[string]any{"content": targetSecrets[2]})},
		{ID: "event-other-system", Version: "workorch.event", Topic: "/other/system", OccurredAt: "2026-08-26T03:00:00Z",
			SourceModule: "blocks/other", SubjectType: "other", SubjectID: "other-system", WorkspaceID: otherWorkspace, Revision: 4,
			Payload: mustWorkspaceScopedPayload(t, map[string]any{"content": largeOtherSystem})},
		{ID: "event-other-run", Version: "workorch.event", Topic: "/other/run", OccurredAt: "2026-08-26T03:00:01Z",
			SourceModule: "blocks/other", SubjectType: "other", SubjectID: "other-run", WorkspaceID: otherWorkspace, RunID: "run-other", Revision: 5,
			Payload: mustWorkspaceScopedPayload(t, map[string]any{"content": largeOtherRun})},
	}
	for _, record := range records {
		position, err := repos.EventRecords.ReserveEventRecordStreamPosition(ctx)
		if err != nil {
			t.Fatal(err)
		}
		record.StreamPosition = position
		if err := repos.EventRecords.Append(ctx, record); err != nil {
			t.Fatal(err)
		}
	}
	fence := fileEventStreamFence(fileEventStreamDeletionScope(targetWorkspace, "purge"))
	if err := repos.EventStreamDeletions.Create(ctx, fence); err != nil {
		t.Fatal(err)
	}
	positionPath := filepath.Join(dataRoot, "system", workspaceScopedEventPositionFile, ".eventstream-stream-position.json")
	positionBefore, err := os.ReadFile(positionPath)
	if err != nil {
		t.Fatal(err)
	}

	watchRoots := []string{
		filepath.Join(dataRoot, "system"),
		filepath.Join(dataRoot, "internal", "workspaces", string(targetWorkspace), "runs", string(targetRuns[0])),
		filepath.Join(dataRoot, "internal", "workspaces", string(targetWorkspace), "runs", string(targetRuns[1])),
	}
	inotifyFD, err := unix.InotifyInit1(unix.IN_CLOEXEC)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(inotifyFD)
	watches := map[int]string{}
	for _, root := range watchRoots {
		watch, err := unix.InotifyAddWatch(inotifyFD, root, unix.IN_MOVED_TO)
		if err != nil {
			t.Fatalf("watch %s: %v", root, err)
		}
		watches[watch] = root
	}

	command := exec.Command(os.Args[0], "-test.run=^TestFileEventStreamPurgeCrashProcessHelper$", "-test.count=1")
	command.Env = append(os.Environ(), eventStreamPurgeCrashHelperEnv+"=1", eventStreamPurgeCrashRootEnv+"="+dataRoot)
	var helperOutput bytes.Buffer
	command.Stdout = &helperOutput
	command.Stderr = &helperOutput
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if command.ProcessState == nil {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	}()
	killedAtMiddlePath := false
	deadline := time.Now().Add(15 * time.Second)
	buffer := make([]byte, 64*1024)
	for !killedAtMiddlePath && time.Now().Before(deadline) {
		poll := []unix.PollFd{{Fd: int32(inotifyFD), Events: unix.POLLIN}}
		if _, err := unix.Poll(poll, 250); err != nil {
			if errors.Is(err, unix.EINTR) {
				continue
			}
			t.Fatal(err)
		}
		if poll[0].Revents&unix.POLLIN == 0 {
			if command.ProcessState != nil && command.ProcessState.Exited() {
				break
			}
			continue
		}
		count, err := unix.Read(inotifyFD, buffer)
		if err != nil {
			if errors.Is(err, unix.EINTR) {
				continue
			}
			t.Fatal(err)
		}
		for offset := 0; offset+unix.SizeofInotifyEvent <= count; {
			event := (*unix.InotifyEvent)(unsafe.Pointer(&buffer[offset]))
			nameStart := offset + unix.SizeofInotifyEvent
			nameEnd := nameStart + int(event.Len)
			name := string(bytes.TrimRight(buffer[nameStart:nameEnd], "\x00"))
			offset = nameEnd
			root := watches[int(event.Wd)]
			if root != watchRoots[1] || name != ".eventstream-purge-swap.json" {
				continue
			}
			if err := command.Process.Signal(os.Signal(unix.SIGSTOP)); err != nil {
				continue
			}
			data, readErr := os.ReadFile(filepath.Join(root, name))
			wire := struct {
				Phase string `json:"phase"`
			}{}
			if readErr == nil {
				readErr = json.Unmarshal(data, &wire)
			}
			if readErr == nil && (wire.Phase == "swapping" || wire.Phase == "installed") {
				if err := command.Process.Kill(); err != nil {
					t.Fatal(err)
				}
				killedAtMiddlePath = true
				break
			}
			if err := command.Process.Signal(os.Signal(unix.SIGCONT)); err != nil {
				continue
			}
		}
	}
	if !killedAtMiddlePath {
		_ = command.Process.Kill()
		_ = command.Wait()
		t.Fatalf("did not stop purge in middle workspace path; helper output: %s", helperOutput.String())
	}
	if err := command.Wait(); err == nil {
		t.Fatal("crash helper exited successfully, want SIGKILL")
	} else {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("crash helper wait: %v", err)
		}
	}

	reopened := NewWorkspaceScopedFileRepositorySet(dataRoot)
	deleted, err := reopened.EventStreamPurger.PurgeWorkspaceEventRecords(ctx, *fence)
	if err != nil || deleted != len(targetRuns) {
		t.Fatalf("reopen exact retry deleted=%d err=%v, want %d", deleted, err, len(targetRuns))
	}
	if replayed, err := reopened.EventStreamPurger.PurgeWorkspaceEventRecords(ctx, *fence); err != nil || replayed != 0 {
		t.Fatalf("fixed-point exact replay deleted=%d err=%v", replayed, err)
	}

	concrete := reopened.EventRecords.(*workspaceScopedEventRecordRepository)
	for _, path := range []string{
		concrete.store.systemPath(workspaceScopedEventRecordFile),
		concrete.store.runPath(concrete.store.runRoot(targetWorkspace, targetRuns[0]), workspaceScopedEventRecordFile),
		concrete.store.runPath(concrete.store.runRoot(targetWorkspace, targetRuns[1]), workspaceScopedEventRecordFile),
	} {
		items, err := concrete.store.eventRecordRepo(path).List(ctx, workorchquerycontract.EventRecordQuery{WorkspaceID: targetWorkspace, IncludePayload: true})
		if err != nil || len(items) != 0 {
			t.Fatalf("target physical records at %s = %+v err=%v", path, items, err)
		}
	}
	other, err := reopened.EventRecords.List(ctx, workorchquerycontract.EventRecordQuery{WorkspaceID: otherWorkspace, IncludePayload: true})
	if err != nil || len(other) != 2 {
		t.Fatalf("other workspace records = %+v err=%v", other, err)
	}
	sort.Slice(other, func(i, j int) bool { return other[i].ID < other[j].ID })
	wantPayloads := map[common.EventRecordID]string{"event-other-run": largeOtherRun, "event-other-system": largeOtherSystem}
	for _, item := range other {
		content, ok := item.Payload.String("content")
		if !ok || content != wantPayloads[item.ID] || item.PayloadRef == "" {
			t.Fatalf("other workspace payload drifted: id=%s ref=%q bytes=%d", item.ID, item.PayloadRef, len(content))
		}
	}
	positionAfter, err := os.ReadFile(positionPath)
	if err != nil || !bytes.Equal(positionAfter, positionBefore) {
		t.Fatalf("global position bytes drifted: before=%q after=%q err=%v", positionBefore, positionAfter, err)
	}
	if next, err := reopened.EventRecords.ReserveEventRecordStreamPosition(ctx); err != nil || next != int64(len(records)+1) {
		t.Fatalf("next global position=%d err=%v, want %d", next, err, len(records)+1)
	}
	targetMaterials := []string{"event-target-system", "event-target-run-a", "event-target-run-b"}
	targetMaterials = append(targetMaterials, targetSecrets...)
	if err := filepath.Walk(dataRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info != nil && (info.Name() == ".eventstream-purge-swap.json" || info.Name() == ".eventstream-purge-swap.pending" ||
			strings.Contains(info.Name(), ".eventstream-purge-") && (strings.HasSuffix(info.Name(), ".backup") || strings.HasSuffix(info.Name(), ".replacement"))) {
			return errors.New("eventstream purge transaction artifact remains: " + path)
		}
		if info == nil || info.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, material := range targetMaterials {
			if bytes.Contains(data, []byte(material)) {
				return errors.New("target event material remains after purge: " + path)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
