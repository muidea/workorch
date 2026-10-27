//go:build linux

package file

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
	"golang.org/x/sys/unix"
)

func TestPreparationResourceRealBytesBoundaryAndNoMutation(t *testing.T) {
	root := t.TempDir()
	content := []byte{0xff, 0, '\r', '\n', 'x'}
	path := filepath.Join(root, "requirements.txt")
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(path)
	r := observePreparationResource(root, "requirements.txt", fileobservation.ResourceTotalBytes)
	if r.Validate() != nil || !r.Complete || !bytes.Equal(r.Content, content) || r.ContentSHA256 != fileobservation.Digest(content) || r.Inode == 0 {
		t.Fatal("full original bytes/version not observed", r)
	}
	again := observePreparationResource(root, "requirements.txt", fileobservation.ResourceTotalBytes)
	after, _ := os.Stat(path)
	if !reflect.DeepEqual(r, again) || before.Mode() != after.Mode() || !sameAuthoringFileVersion(before, after) {
		t.Fatal("read mutated resource")
	}
	if err := os.Symlink(path, filepath.Join(root, "symlink")); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(path, filepath.Join(root, "hardlink")); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mkfifo(filepath.Join(root, "fifo"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "oversized"), bytes.Repeat([]byte("x"), fileobservation.ResourceMaxBytes+1), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "small"), content, 0600); err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{"missing", "symlink", "hardlink", "requirements.txt", "fifo", "oversized", "../escape", "/absolute", "file:requirements.txt", ".hidden", "sub/../small"} {
		bad := observePreparationResource(root, ref, fileobservation.ResourceTotalBytes)
		if bad.Validate() != nil || bad.Complete || len(bad.Content) != 0 || bad.DiagnosticBytes == 0 || bad.Inode != 0 {
			t.Fatalf("incomplete/boundary read promoted: %s %+v", ref, bad)
		}
	}
	bad := observePreparationResource(root, "small", len(content)-1)
	if bad.Complete || len(bad.Content) != 0 || bad.Validate() != nil {
		t.Fatal("aggregate budget silently truncated")
	}
	if _, err := os.Stat(filepath.Join(root, "missing")); !os.IsNotExist(err) {
		t.Fatal("observation created missing resource")
	}
	long := bytes.Repeat([]byte("x"), fileobservation.DiagnosticLimit+1)
	omitted := failedPreparationResource("input", errors.New(string(long)))
	if omitted.Validate() != nil || omitted.Complete || len(omitted.Diagnostic) != 0 || omitted.DiagnosticBytes != len(long) || omitted.DiagnosticSHA256 != fileobservation.Digest(long) || omitted.DiagnosticNotSavedReason != "diagnostic_exceeds_byte_boundary" {
		t.Fatal("oversized diagnostic silently truncated or misrepresented")
	}
}

func TestPreparationResourceTypedScopeLimitsAndShutdown(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(root, "a"), []byte("raw"), 0600); err != nil {
		t.Fatal(err)
	}
	c := fileevents.ObservePreparationResourcesCommand{WorkspaceID: "ws-resource", RequestID: "original", PayloadHash: "payload", WorkspaceRevision: 7, DeliverableDirectory: root, ResourceRefs: []string{"a"}}
	for _, source := range []string{"application/workorchd", "test", "blocks/workspace"} {
		result := hub.Send(magicevent.NewEventWithContext(fileevents.TopicObservePreparationResources, source, ModuleID, magicevent.NewHeader(), ctx, c))
		if result == nil {
			t.Fatal("missing typed result")
		}
		if source != "blocks/workspace" {
			if result.Error() == nil {
				t.Fatal("nonowner supplied scope")
			}
			continue
		}
		value, err := result.Get()
		r, ok := value.(fileevents.ObservePreparationResourcesResult)
		if err != nil || !ok || r.ValidateFor(c) != nil || !r.Resources[0].Complete {
			t.Fatal("typed observation failed", err, value)
		}
		for _, change := range []func(*fileevents.ObservePreparationResourcesResult){
			func(r *fileevents.ObservePreparationResourcesResult) { r.Command.WorkspaceRevision++ },
			func(r *fileevents.ObservePreparationResourcesResult) { r.Command.ResourceRefs[0] = "other" },
			func(r *fileevents.ObservePreparationResourcesResult) { r.Resources = nil },
			func(r *fileevents.ObservePreparationResourcesResult) { r.Resources[0].Content[0] = 'z' },
		} {
			bad := r
			bad.Command = r.Command.Clone()
			bad.Resources = []fileobservation.Resource{r.Resources[0].Clone()}
			change(&bad)
			if bad.ValidateFor(c) == nil {
				t.Fatal("forged observation accepted")
			}
		}
	}
	b.handleObservePreparationResources(nil, nil)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	for _, checkCtx := range []context.Context{canceled, common.ContextWithRuntimeWorkspaceID(ctx, "other")} {
		r := hub.Send(magicevent.NewEventWithContext(fileevents.TopicObservePreparationResources, "blocks/workspace", ModuleID, magicevent.NewHeader(), checkCtx, c))
		if r == nil || r.Error() == nil {
			t.Fatal("canceled or wrong-scope read accepted")
		}
	}
	if err := os.WriteFile(filepath.Join(root, "large"), bytes.Repeat([]byte("x"), fileobservation.ResourceMaxBytes), 0600); err != nil {
		t.Fatal(err)
	}
	batch := c.Clone()
	batch.ResourceRefs = []string{"large", "large", "large", "large", "large"}
	batchResult := hub.Send(magicevent.NewEventWithContext(fileevents.TopicObservePreparationResources, "blocks/workspace", ModuleID, magicevent.NewHeader(), ctx, batch))
	if batchResult == nil || batchResult.Error() != nil {
		t.Fatal("batch observation missing")
	}
	value, valueErr := batchResult.Get()
	observed, ok := value.(fileevents.ObservePreparationResourcesResult)
	if valueErr != nil || !ok || observed.ValidateFor(batch) != nil || !observed.Resources[3].Complete || observed.Resources[4].Complete || len(observed.Resources[4].Content) != 0 || observed.Resources[4].DiagnosticBytes == 0 {
		t.Fatal("aggregate byte boundary did not preserve completed files and explicit incomplete failure")
	}
	b.BeginShutdown(ctx)
	r := hub.Send(magicevent.NewEventWithContext(fileevents.TopicObservePreparationResources, "blocks/workspace", ModuleID, magicevent.NewHeader(), ctx, c))
	if r == nil || r.Error() == nil {
		t.Fatal("new read admitted during shutdown")
	}
}
