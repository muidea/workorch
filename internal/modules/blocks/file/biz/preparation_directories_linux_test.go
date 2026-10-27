//go:build linux

package file

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
)

func TestPreparationDirectoryRealIdentityNoMutationAndFailures(t *testing.T) {
	root := t.TempDir()
	before, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	r := observePreparationDirectory(root)
	if r.Validate() != nil || !r.Complete || !r.Readable || !r.Writable || !r.Searchable || r.Inode == 0 {
		t.Fatalf("actual directory: %+v", r)
	}
	again := observePreparationDirectory(root)
	if !reflect.DeepEqual(r, again) {
		t.Fatal("read mutated directory identity/version")
	}
	after, _ := os.Stat(root)
	entries, _ := os.ReadDir(root)
	if before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) || len(entries) != 0 {
		t.Fatal("observation wrote a probe/config or changed permissions")
	}
	missing := filepath.Join(root, "missing")
	link := filepath.Join(root, "link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	regular := filepath.Join(root, "regular")
	if err := os.WriteFile(regular, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{missing, link, filepath.Join(link, "missing"), regular} {
		bad := observePreparationDirectory(path)
		if bad.Validate() != nil || bad.Complete || bad.DiagnosticBytes == 0 || bad.Readable || bad.Writable || bad.Searchable {
			t.Fatalf("failure promoted or lost diagnostic: %+v", bad)
		}
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("missing directory was prepared")
	}
	locked := filepath.Join(root, "locked")
	if err := os.Mkdir(locked, 0500); err != nil {
		t.Fatal(err)
	}
	permission := observePreparationDirectory(locked)
	if !permission.Complete || permission.Validate() != nil {
		t.Fatalf("permission observation unavailable: %+v", permission)
	}
	if os.Geteuid() != 0 && permission.Writable {
		t.Fatal("mode bit substituted for effective write denial")
	}
}

func TestPreparationDirectoryTypedSourceScopeAndLifecycle(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	defer hub.Terminate(ctx)
	b, err := New(hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Start(); err != nil {
		t.Fatal(err)
	}
	defer b.Teardown(ctx)
	root := t.TempDir()
	c := fileevents.ObservePreparationDirectoriesCommand{WorkspaceID: "ws-directory", RequestID: "original", PayloadHash: "payload", WorkspaceRevision: 7, InternalDirectory: root, DeliverableDirectory: root}
	for _, source := range []string{"application/workorchd", "test", "blocks/workspace"} {
		ev := magicevent.NewEventWithContext(fileevents.TopicObservePreparationDirectories, source, ModuleID, magicevent.NewHeader(), ctx, c)
		result := hub.Send(ev)
		if result == nil {
			t.Fatal("missing typed response")
		}
		if source != "blocks/workspace" {
			if result.Error() == nil {
				t.Fatal("non-owner supplied filesystem scope")
			}
			continue
		}
		value, err := result.Get()
		r, ok := value.(fileevents.ObservePreparationDirectoriesResult)
		if err != nil || !ok || r.ValidateFor(c) != nil || !r.Internal.Complete || !r.Deliverable.Complete {
			t.Fatalf("typed File query failed: %v %v", result.Error(), value)
		}
		wrong := c
		wrong.WorkspaceRevision++
		if r.ValidateFor(wrong) == nil {
			t.Fatal("wrong revision accepted")
		}
	}
	b.handleObservePreparationDirectories(nil, nil)
	b.BeginShutdown(ctx)
	result := hub.Send(magicevent.NewEventWithContext(fileevents.TopicObservePreparationDirectories, "blocks/workspace", ModuleID, magicevent.NewHeader(), ctx, c))
	if result == nil || result.Error() == nil {
		t.Fatal("new observation admitted during shutdown")
	}
}
