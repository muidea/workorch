//go:build linux

package file

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
	"github.com/muidea/workorch/internal/pkg/workorchmanagedconfig"
)

func TestPreparationManagedConfigsExactRootsBytesAndTypedBoundary(t *testing.T) {
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
	internal := filepath.Join(t.TempDir(), "internal", "workspaces", "ws")
	system := workorchmanagedconfig.SystemDirectory(internal)
	for _, root := range []string{internal, system} {
		if err := os.MkdirAll(root, 0700); err != nil {
			t.Fatal(err)
		}
	}
	write := func(root, path string, raw []byte) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, path), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(internal, "same.json", []byte("workspace\r\n"))
	write(system, "same.json", []byte{0xff, 0, '\r', '\n'})
	write(internal, "only-workspace.json", []byte("not a fallback"))
	c := fileevents.ObservePreparationManagedConfigsCommand{WorkspaceID: "ws", RequestID: "original", PayloadHash: "payload", WorkspaceRevision: 7, InternalDirectory: internal, Targets: []fileevents.PreparationManagedConfigTarget{{Scope: "system", Path: "same.json"}, {Scope: "workspace", Path: "same.json"}, {Scope: "system", Path: "only-workspace.json"}}}
	send := func(source string, ctx context.Context, c fileevents.ObservePreparationManagedConfigsCommand) magicevent.Result {
		return hub.Send(magicevent.NewEventWithContext(fileevents.TopicObservePreparationManagedConfigs, source, ModuleID, magicevent.NewHeader(), ctx, c))
	}
	result := send("blocks/workspace", ctx, c)
	if result == nil || result.Error() != nil {
		t.Fatal("read failed", result)
	}
	value, resultErr := result.Get()
	r, ok := value.(fileevents.ObservePreparationManagedConfigsResult)
	if resultErr != nil || !ok || r.ValidateFor(c) != nil || !r.Files[0].Complete || !bytes.Equal(r.Files[0].Content, []byte{0xff, 0, '\r', '\n'}) || !bytes.Equal(r.Files[1].Content, []byte("workspace\r\n")) || r.Files[2].Complete || r.Files[2].DiagnosticBytes == 0 {
		t.Fatal("root/bytes/failure changed", resultErr)
	}
	for _, change := range []func(*fileevents.ObservePreparationManagedConfigsResult){
		func(r *fileevents.ObservePreparationManagedConfigsResult) { r.Command.WorkspaceRevision++ },
		func(r *fileevents.ObservePreparationManagedConfigsResult) { r.Command.Targets[0].Scope = "workspace" },
		func(r *fileevents.ObservePreparationManagedConfigsResult) { r.Files = r.Files[:1] },
		func(r *fileevents.ObservePreparationManagedConfigsResult) { r.Files[0].Content[0] = 'x' },
	} {
		bad := r
		bad.Command = r.Command.Clone()
		bad.Files = make([]fileobservation.Resource, len(r.Files))
		for i := range r.Files {
			bad.Files[i] = r.Files[i].Clone()
		}
		change(&bad)
		if bad.ValidateFor(c) == nil {
			t.Fatal("forged result accepted")
		}
	}
	for _, target := range []fileevents.PreparationManagedConfigTarget{{Scope: "other", Path: "same.json"}, {Scope: "system", Path: "../escape"}, {Scope: "workspace", Path: "a/../same.json"}} {
		bad := c.Clone()
		bad.Targets = []fileevents.PreparationManagedConfigTarget{target}
		if bad.Validate() == nil {
			t.Fatal("bad target accepted")
		}
	}
	bad := c.Clone()
	bad.Targets = append(bad.Targets, bad.Targets[0])
	if bad.Validate() == nil {
		t.Fatal("duplicate target accepted")
	}
	for _, source := range []string{"application/workorchd", "test"} {
		if result := send(source, ctx, c); result == nil || result.Error() == nil {
			t.Fatal("nonowner read accepted")
		}
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	for _, checkCtx := range []context.Context{canceled, common.ContextWithRuntimeWorkspaceID(ctx, "other")} {
		if result := send("blocks/workspace", checkCtx, c); result == nil || result.Error() == nil {
			t.Fatal("bad scope/context accepted")
		}
	}
	b.handleObservePreparationManagedConfigs(nil, nil)
	for _, scenario := range []string{"symlink", "hardlink", "oversize"} {
		name := scenario + ".json"
		path := filepath.Join(system, name)
		switch scenario {
		case "symlink":
			err = os.Symlink(filepath.Join(system, "same.json"), path)
		case "hardlink":
			err = os.Link(filepath.Join(system, "same.json"), path)
		case "oversize":
			err = os.WriteFile(path, bytes.Repeat([]byte("x"), fileobservation.ResourceMaxBytes+1), 0600)
		}
		if err != nil {
			t.Fatal(err)
		}
		observed := observePreparationFile(system, name, fileobservation.ResourceTotalBytes)
		if observed.Validate() != nil || observed.Complete || len(observed.Content) != 0 {
			t.Fatal("incomplete read promoted")
		}
	}
	write(internal, "large.json", bytes.Repeat([]byte("x"), fileobservation.ResourceMaxBytes))
	batch := c.Clone()
	batch.Targets = nil
	for i := 0; i < 5; i++ {
		name := string(rune('a'+i)) + ".json"
		write(internal, name, bytes.Repeat([]byte("x"), fileobservation.ResourceMaxBytes))
		batch.Targets = append(batch.Targets, fileevents.PreparationManagedConfigTarget{Scope: "workspace", Path: name})
	}
	result = send("blocks/workspace", ctx, batch)
	if result == nil || result.Error() != nil {
		t.Fatal("batch failed")
	}
	value, resultErr = result.Get()
	observed, ok := value.(fileevents.ObservePreparationManagedConfigsResult)
	if resultErr != nil || !ok || observed.ValidateFor(batch) != nil || !observed.Files[3].Complete || observed.Files[4].Complete {
		t.Fatal("batch byte boundary failed")
	}
	before, _ := os.ReadFile(filepath.Join(internal, "same.json"))
	if !reflect.DeepEqual(before, []byte("workspace\r\n")) {
		t.Fatal("read mutated file")
	}
	b.BeginShutdown(ctx)
	if result := send("blocks/workspace", ctx, c); result == nil || result.Error() == nil {
		t.Fatal("read admitted after shutdown")
	}
}
