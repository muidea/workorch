package file

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

func TestAuthoringResourceOwnerRequiresSafeCompleteScopedFile(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner, err := New(hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })
	root := t.TempDir()
	outside := t.TempDir()
	write := func(ref, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, ref), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("requirements.txt", "目标与验收说明\n")
	write(".env", "KEY=value\n")
	write("linked.txt", "可见但不能导出\n")
	write("nested-secret.txt", "内层秘密\n")
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "nested-secret.txt"), filepath.Join(root, "sub", "nested-secret.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(filepath.Join(root, "linked.txt"), filepath.Join(root, "linked-copy.txt")); err != nil {
		t.Fatal(err)
	}
	write("credentials.txt", "API_TOKEN=secret\n")
	write("redacted.txt", "[REDACTED]\n")
	write("binary.txt", "a\x00b")
	write("large.txt", strings.Repeat("x", fileevents.AuthoringResourceMaxBytes+1))
	if err := os.WriteFile(filepath.Join(outside, "outside.txt"), []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "outside.txt"), filepath.Join(root, "escape.txt")); err != nil {
		t.Fatal(err)
	}
	command := fileevents.AcquireAuthoringResourceCommand{WorkspaceID: "ws", AuthoringInputHash: "input-hash", ResourceRef: "requirements.txt"}
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: command.WorkspaceID, Revision: 1, GoalText: "目标", TemplateID: "template", ResourceRefs: []string{
		"requirements.txt", ".env", "credentials.txt", "redacted.txt", "binary.txt", "large.txt", "escape.txt", "linked.txt",
	}}
	template := workspace.WorkspaceTemplate{ID: input.TemplateID}
	record := workspace.DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template)}
	command.AuthoringInputHash = record.InputHash
	observer := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	queryCount := 0
	revision := uint64(1)
	driftOnSecondQuery := false
	if err := observer.Subscribe(workspaceevents.TopicWorkspaceAuthoringResourceScopeQuery, func(ev magicevent.Event, result magicevent.Result) {
		queryCount++
		if ev == nil || ev.Source() != ModuleID {
			result.Set(nil, nil)
			return
		}
		query, ok := ev.Data().(workspaceevents.QueryAuthoringResourceScopeCommand)
		if !ok {
			result.Set(nil, nil)
			return
		}
		if driftOnSecondQuery && queryCount == 2 {
			revision++
		}
		view := workspaceevents.QueryDefinitionAuthoringResult{WorkspaceID: input.WorkspaceID, WorkspaceRevision: revision, Record: &record, DeliverableDirectory: root, CurrentTemplateAvailable: true, CurrentTemplateSnapshotHash: record.TemplateSnapshotHash}
		scope, err := workspaceevents.AuthoringResourceScopeFor(view, query)
		if err != nil {
			result.Set(nil, nil)
			return
		}
		result.Set(scope, nil)
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { observer.Unsubscribe(workspaceevents.TopicWorkspaceAuthoringResourceScopeQuery) })
	send := func(source string, request fileevents.AcquireAuthoringResourceCommand) (fileevents.AcquireAuthoringResourceResult, error) {
		t.Helper()
		ev := magicevent.NewEventWithContext(fileevents.TopicAcquireAuthoringResources, source, ModuleID, magicevent.NewHeader(), ctx, request)
		response := hub.Send(ev)
		if response == nil {
			t.Fatal("File owner returned no result")
		}
		if err := response.Error(); err != nil {
			return fileevents.AcquireAuthoringResourceResult{}, err
		}
		value, err := response.Get()
		if err != nil {
			return fileevents.AcquireAuthoringResourceResult{}, err
		}
		result, ok := value.(fileevents.AcquireAuthoringResourceResult)
		if !ok {
			t.Fatal("File owner returned wrong concrete result")
		}
		return result, nil
	}
	if _, err := send("kernel/orchestrator", command); err == nil {
		t.Fatal("Runtime source was admitted to creation-only query")
	}
	got, err := send(authoringApplicationSource, command)
	if runtime.GOOS != "linux" {
		if err == nil {
			t.Fatal("unsupported platform accepted authoring resource content")
		}
		return
	}
	if err != nil || got.ValidateFor(command) != nil || got.Content != "目标与验收说明\n" || queryCount != 2 {
		t.Fatalf("complete resource snapshot: %+v queries=%d %v", got, queryCount, err)
	}
	verify := fileevents.VerifyAuthoringResourceCommand{WorkspaceID: got.WorkspaceID, WorkspaceRevision: got.WorkspaceRevision, AuthoringInputHash: got.AuthoringInputHash, ResourceRef: got.ResourceRef, Content: got.Content, ContentHash: got.ContentHash}
	sendVerify := func(source string, request fileevents.VerifyAuthoringResourceCommand) error {
		t.Helper()
		response := hub.Send(magicevent.NewEventWithContext(fileevents.TopicVerifyAuthoringResource, source, ModuleID, magicevent.NewHeader(), ctx, request))
		if response == nil {
			t.Fatal("File verification returned no result")
		}
		if err := response.Error(); err != nil {
			return err
		}
		value, getErr := response.Get()
		if getErr != nil {
			return getErr
		}
		matched, ok := value.(fileevents.VerifyAuthoringResourceResult)
		if !ok {
			t.Fatalf("File verification returned %T", value)
		}
		return matched.ValidateFor(request)
	}
	if err := sendVerify(authoringApplicationSource, verify); err == nil {
		t.Fatal("Application used Workspace-only File verification")
	}
	if err := sendVerify(workspaceevents.CommandDestination, verify); err != nil {
		t.Fatalf("Workspace verification failed: %v", err)
	}
	forged := verify
	forged.Content = "伪造内容"
	checksum := sha256.Sum256([]byte(forged.Content))
	forged.ContentHash = "sha256:" + hex.EncodeToString(checksum[:])
	if err := sendVerify(workspaceevents.CommandDestination, forged); err == nil {
		t.Fatal("self-consistent forged content matched File owner")
	}
	verify.WorkspaceRevision++
	if err := sendVerify(workspaceevents.CommandDestination, verify); err == nil {
		t.Fatal("stale Workspace revision matched File owner")
	}
	queryCount = 0
	driftOnSecondQuery = true
	if fact, err := send(authoringApplicationSource, command); err == nil || fact.Content != "" || queryCount != 2 {
		t.Fatalf("workspace revision drift was accepted: %+v queries=%d err=%v", fact, queryCount, err)
	}
	driftOnSecondQuery = false
	unbound := command
	unbound.WorkspaceID = "another-workspace"
	if _, err := send(authoringApplicationSource, unbound); err == nil {
		t.Fatal("unbound workspace accepted a resource query")
	}
	unbound = command
	unbound.AuthoringInputHash = "another-input"
	if _, err := send(authoringApplicationSource, unbound); err == nil {
		t.Fatal("stale authoring input accepted a resource query")
	}
	for _, ref := range []string{"../outside.txt", "/etc/passwd", ".env", "escape.txt", "linked.txt", "linked-copy.txt", "sub/nested-secret.txt", "binary.txt", "redacted.txt", "credentials.txt", "large.txt", "missing.txt", ".git/config", "node_modules/pkg.txt", "requirements.txt/../.env"} {
		t.Run(ref, func(t *testing.T) {
			changed := command
			changed.ResourceRef = ref
			if _, err := send(authoringApplicationSource, changed); err == nil {
				t.Fatal("untrusted resource was sent to Application")
			}
		})
	}
	for _, ref := range []string{".env", "escape.txt", "linked.txt", "binary.txt", "redacted.txt", "credentials.txt", "large.txt", "linked-copy.txt", "sub/nested-secret.txt", "missing.txt"} {
		t.Run("scoped/"+ref, func(t *testing.T) {
			updated := input.Clone()
			updated.ResourceRefs[len(updated.ResourceRefs)-1] = ref
			record.Input = updated
			record.InputHash = updated.InputHash()
			changed := command
			changed.AuthoringInputHash = record.InputHash
			changed.ResourceRef = ref
			queryCount = 0
			if _, err := send(authoringApplicationSource, changed); err == nil || (ref != ".env" && queryCount == 0) {
				t.Fatalf("scoped unsafe resource bypassed File checks: queries=%d err=%v", queryCount, err)
			}
		})
	}
	record.Input = input
	record.InputHash = input.InputHash()
	write("requirements.txt", "已修改\n")
	changed, err := send(authoringApplicationSource, command)
	if err != nil || changed.ContentHash == got.ContentHash {
		t.Fatalf("resource drift retained old snapshot: %+v %v", changed, err)
	}
	root = outside
	if _, err := send(authoringApplicationSource, command); err == nil {
		t.Fatal("changed Workspace directory read an unbound file")
	}
	owner.Teardown(ctx)
	if response := hub.Send(magicevent.NewEventWithContext(fileevents.TopicAcquireAuthoringResources, authoringApplicationSource, ModuleID, magicevent.NewHeader(), ctx, command)); response != nil && response.Error() == nil {
		t.Fatal("File query still processed after teardown")
	}
}
