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
	"github.com/muidea/workorch/internal/modules/blocks/file/pkg/preparation"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestPreparationFileRevalidationOriginalBytesWindowAndEpoch(t *testing.T) {
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
	raw := []byte{0xff, 0, '\r', '\n'}
	if err := os.WriteFile(filepath.Join(root, "input.txt"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	command := fileevents.ReobservePreparationFilesCommand{Scope: preparation.Scope{WorkspaceID: "ws", RequestID: "original", PayloadHash: "payload", WorkspaceRevision: 7, InternalDirectory: root, DeliverableDirectory: root, ResourceRefs: []string{"input.txt"}, Configs: []preparation.ConfigTarget{{Scope: "workspace", Path: "input.txt"}}}}
	send := func(topic, source string, ctx context.Context, value any) magicevent.Result {
		return hub.Send(magicevent.NewEventWithContext(topic, source, ModuleID, magicevent.NewHeader(), ctx, value))
	}
	read := func() preparation.Observation {
		t.Helper()
		r := send(fileevents.TopicReobservePreparationFiles, "blocks/workspace", ctx, command)
		if r == nil || r.Error() != nil {
			t.Fatal("missing observation", r)
		}
		value, err := r.Get()
		o, ok := value.(fileevents.ReobservePreparationFilesResult)
		if err != nil || !ok || o.ValidateFor(command) != nil {
			t.Fatal("invalid typed observation", value, err)
		}
		return o.Observation
	}
	first, second := read(), read()
	if !first.Internal.Complete || !first.Deliverable.Complete || !first.Resources[0].Complete || !bytes.Equal(first.Resources[0].Content, raw) || !bytes.Equal(first.Configs[0].Content, raw) || first.TokenID == second.TokenID || first.OwnerEpoch != second.OwnerEpoch || !first.ExpiresAt.Equal(first.StartedAt.Add(preparation.MaxAge)) {
		t.Fatal("raw material, original window or fresh token lost")
	}
	clone := first.Clone()
	clone.Scope.ResourceRefs[0] = "other"
	clone.Scope.Configs[0].Path = "other"
	clone.Resources[0].Content[0] = 'x'
	clone.Configs[0].Content[0] = 'x'
	if !bytes.Equal(first.Resources[0].Content, raw) || !bytes.Equal(first.Configs[0].Content, raw) || !reflect.DeepEqual(first.Scope, command.Scope) {
		t.Fatal("detached result aliases")
	}
	for _, mutate := range []func(*preparation.Observation){
		func(o *preparation.Observation) { o.Scope.WorkspaceRevision++ },
		func(o *preparation.Observation) { o.Scope.RequestID = "replacement" },
		func(o *preparation.Observation) { o.Resources = nil },
		func(o *preparation.Observation) { o.Configs = nil },
		func(o *preparation.Observation) { o.ExpiresAt = o.ExpiresAt.Add(preparation.MaxAge) },
		func(o *preparation.Observation) { o.ObservedAt = o.ExpiresAt },
		func(o *preparation.Observation) { o.OwnerEpoch = "invented" },
	} {
		bad := first.Clone()
		mutate(&bad)
		if (fileevents.ReobservePreparationFilesResult{Observation: bad}).ValidateFor(command) == nil {
			t.Fatal("altered full scope/window accepted")
		}
	}
	for _, source := range []string{"application/workorchd", "test"} {
		for _, topic := range []string{fileevents.TopicQueryPreparationEpoch, fileevents.TopicReobservePreparationFiles} {
			if r := send(topic, source, ctx, command); r == nil || r.Error() == nil {
				t.Fatal("non-owner query accepted")
			}
		}
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	for _, invalid := range []context.Context{cancelled, common.ContextWithRuntimeWorkspaceID(ctx, "other")} {
		if r := send(fileevents.TopicReobservePreparationFiles, "blocks/workspace", invalid, command); r == nil || r.Error() == nil {
			t.Fatal("bad context accepted")
		}
	}
	// Remove the original target. Epoch reads cannot probe or refresh it.
	if err := os.Rename(filepath.Join(root, "input.txt"), filepath.Join(root, "preserved.txt")); err != nil {
		t.Fatal(err)
	}
	r := send(fileevents.TopicQueryPreparationEpoch, "blocks/workspace", ctx, fileevents.QueryPreparationEpochCommand{})
	if r == nil || r.Error() != nil {
		t.Fatal("epoch query failed")
	}
	value, _ := r.Get()
	if epoch, ok := value.(fileevents.QueryPreparationEpochResult); !ok || epoch.OwnerEpoch != first.OwnerEpoch {
		t.Fatal("epoch query changed observation identity")
	}
	missing := read()
	if missing.Resources[0].Complete || missing.Resources[0].DiagnosticBytes == 0 || missing.Configs[0].Complete {
		t.Fatal("missing target promoted or diagnostic discarded")
	}
	other, err := New(hub, nil)
	if err != nil || other.preparationEpoch == first.OwnerEpoch {
		t.Fatal("new File owner reused old epoch")
	}
	b.handleQueryPreparationEpoch(nil, nil)
	b.handleReobservePreparationFiles(nil, nil)
	b.BeginShutdown(ctx)
	if r := send(fileevents.TopicQueryPreparationEpoch, "blocks/workspace", ctx, fileevents.QueryPreparationEpochCommand{}); r == nil || r.Error() == nil {
		t.Fatal("epoch admitted during shutdown")
	}
}
