package file

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

var errEventRecordPurgeSimulatedCrash = errors.New("simulated event record purge process death")

type eventRecordPurgeCrashFixture struct {
	root                 string
	repo                 *EventRecordRepository
	expected             eventstreammodel.WorkspaceDeletion
	keep                 *eventstreammodel.EventRecord
	largeKeepPayload     string
	otherDeletedID       common.EventRecordID
	otherDeletedPayload  string
	keepPayloadBytes     []byte
	deletedPayloadBytes  []byte
	streamPositionBefore []byte
	randomBackup         string
}

func newEventRecordPurgeCrashFixture(t *testing.T) eventRecordPurgeCrashFixture {
	t.Helper()
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "event-records")
	repo := NewEventRecordRepository(root)
	positions := make([]int64, 3)
	for idx := range positions {
		position, err := repo.ReserveEventRecordStreamPosition(ctx)
		if err != nil {
			t.Fatal(err)
		}
		positions[idx] = position
	}
	largeTargetPayload := "target-secret-" + strings.Repeat("t", eventRecordPayloadInlineThreshold+257)
	largeKeepPayload := "retained-exact-" + strings.Repeat("k", eventRecordPayloadInlineThreshold+513)
	otherDeletedPayload := "retained-tombstone-" + strings.Repeat("d", eventRecordPayloadInlineThreshold+769)
	keep := &eventstreammodel.EventRecord{
		ID: "event-keep-crash", Version: "workorch.event", Topic: "/keep/exact", OccurredAt: "2026-08-26T01:02:03Z",
		SourceModule: "blocks/keep", SubjectType: "retained", SubjectID: "subject-retained",
		WorkspaceID: "ws-keep-crash", RunID: "run-keep-crash", StepID: "run-keep-crash/step-7",
		StreamPosition: positions[1], Revision: 17,
		Payload: mustFileRepositoryPayload(t, map[string]any{"content": largeKeepPayload, "ordinal": float64(7)}),
	}
	otherDeleted := &eventstreammodel.EventRecord{
		ID: "event-other-deleted", Version: "workorch.event", Topic: "/keep/deleted", OccurredAt: "2026-08-26T01:02:04Z",
		SourceModule: "blocks/keep", SubjectType: "retained", SubjectID: "subject-deleted",
		WorkspaceID: "ws-other-deleted", RunID: "run-other-deleted", StepID: "run-other-deleted/step-3",
		StreamPosition: positions[2], Revision: 23,
		Payload: mustFileRepositoryPayload(t, map[string]any{"content": otherDeletedPayload}),
	}
	for _, record := range []*eventstreammodel.EventRecord{
		{
			ID: "event-target-crash", Version: "workorch.event", Topic: "/target/purge", OccurredAt: "2026-08-26T01:02:02Z",
			SourceModule: "blocks/target", SubjectType: "target", SubjectID: "subject-target",
			WorkspaceID: "ws-target-crash", RunID: "run-target-crash", StepID: "run-target-crash/step-1",
			StreamPosition: positions[0], Revision: 11,
			Payload: mustFileRepositoryPayload(t, map[string]any{"content": largeTargetPayload}),
		},
		keep,
		otherDeleted,
	} {
		if err := repo.Append(ctx, record); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.Delete(ctx, otherDeleted.ID); err != nil {
		t.Fatal(err)
	}
	keepPayloadBytes, err := os.ReadFile(filepath.Join(root, "payloads", safeEventRecordFileName(string(keep.ID))+".json"))
	if err != nil {
		t.Fatal(err)
	}
	deletedPayloadBytes, err := os.ReadFile(filepath.Join(root, "payloads", safeEventRecordFileName(string(otherDeleted.ID))+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "unrelated-owner-state.json"), []byte(`{"preserved":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	streamPositionBefore, err := os.ReadFile(repo.eventRecordStreamPositionPath())
	if err != nil {
		t.Fatal(err)
	}
	randomBackup := filepath.Join(filepath.Dir(root), ".unmanaged-event-record-backup-random")
	if err := os.MkdirAll(filepath.Join(randomBackup, "events"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(randomBackup, "events", "not-authority"), []byte("random-backup-must-never-win"), 0o600); err != nil {
		t.Fatal(err)
	}
	return eventRecordPurgeCrashFixture{
		root: root, repo: repo, expected: fileEventStreamPurgeFence("ws-target-crash"), keep: keep,
		largeKeepPayload: largeKeepPayload, otherDeletedID: otherDeleted.ID, otherDeletedPayload: otherDeletedPayload,
		keepPayloadBytes: keepPayloadBytes, deletedPayloadBytes: deletedPayloadBytes,
		streamPositionBefore: streamPositionBefore, randomBackup: randomBackup,
	}
}

func eventRecordPurgeSwapCrashCuts() []eventRecordPurgeSwapCut {
	cuts := []eventRecordPurgeSwapCut{
		eventRecordPurgeSwapCutPreparing,
		eventRecordPurgeSwapCut("after_preparing_append_event-keep-crash"),
		eventRecordPurgeSwapCut("after_preparing_append_event-other-deleted"),
		eventRecordPurgeSwapCut("after_preparing_delete_event-other-deleted"),
		eventRecordPurgeSwapCutPrepared,
		eventRecordPurgeSwapCutSwapping,
	}
	for _, name := range eventRecordPurgeSwapNames {
		cuts = append(cuts,
			eventRecordPurgeSwapCut("before_old_rename_"+name),
			eventRecordPurgeSwapCut("after_old_rename_"+name),
			eventRecordPurgeSwapCut("recorded_old_rename_"+name),
		)
	}
	for _, name := range eventRecordPurgeSwapNames {
		cuts = append(cuts,
			eventRecordPurgeSwapCut("before_replacement_rename_"+name),
			eventRecordPurgeSwapCut("after_replacement_rename_"+name),
			eventRecordPurgeSwapCut("recorded_replacement_rename_"+name),
		)
	}
	return append(cuts,
		eventRecordPurgeSwapCutInstalled,
		eventRecordPurgeSwapCutCompleted,
		eventRecordPurgeSwapCutBackupRemoved,
		eventRecordPurgeSwapCutReplacementRemoved,
		eventRecordPurgeSwapCutJournalRemoved,
	)
}

func crashEventRecordPurgeAt(t *testing.T, fixture eventRecordPurgeCrashFixture, cut eventRecordPurgeSwapCut) {
	t.Helper()
	reached := false
	fixture.repo.eventRecordPurgeSwapCut = func(current eventRecordPurgeSwapCut) error {
		if current != cut {
			return nil
		}
		reached = true
		return errEventRecordPurgeSimulatedCrash
	}
	if _, err := fixture.repo.purgeWorkspace(context.Background(), fixture.expected); !errors.Is(err, errEventRecordPurgeSimulatedCrash) {
		t.Fatalf("purge at cut %q error = %v, want simulated crash", cut, err)
	}
	if !reached {
		t.Fatalf("purge cut %q was not reached", cut)
	}
}

func assertRetainedEventRecordExact(t *testing.T, repo *EventRecordRepository, fixture eventRecordPurgeCrashFixture) {
	t.Helper()
	kept, err := repo.Get(context.Background(), fixture.keep.ID)
	if err != nil {
		t.Fatalf("get retained record: %v", err)
	}
	if kept.ID != fixture.keep.ID || kept.Version != fixture.keep.Version || kept.Topic != fixture.keep.Topic ||
		kept.OccurredAt != fixture.keep.OccurredAt || kept.SourceModule != fixture.keep.SourceModule ||
		kept.SubjectType != fixture.keep.SubjectType || kept.SubjectID != fixture.keep.SubjectID ||
		kept.WorkspaceID != fixture.keep.WorkspaceID || kept.RunID != fixture.keep.RunID || kept.StepID != fixture.keep.StepID ||
		kept.StreamPosition != fixture.keep.StreamPosition || kept.Revision != fixture.keep.Revision {
		t.Fatalf("retained record fields drifted: got %+v want %+v", kept, fixture.keep)
	}
	content, ok := kept.Payload.String("content")
	if !ok || content != fixture.largeKeepPayload || kept.PayloadRef == "" || len(kept.PayloadSummary.Fields) == 0 {
		t.Fatalf("retained large payload drifted: ref=%q summary=%+v bytes=%d", kept.PayloadRef, kept.PayloadSummary, len(content))
	}
}

func assertEventRecordPurgeFixedPoint(t *testing.T, repo *EventRecordRepository, fixture eventRecordPurgeCrashFixture) {
	t.Helper()
	ctx := context.Background()
	if _, err := repo.purgeWorkspace(ctx, fixture.expected); err != nil {
		t.Fatalf("exact purge retry: %v", err)
	}
	if _, err := repo.Get(ctx, "event-target-crash"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("target Get error = %v, want not found", err)
	}
	assertRetainedEventRecordExact(t, repo, fixture)
	if _, err := repo.Get(ctx, fixture.otherDeletedID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("other-workspace tombstone Get error = %v, want not found", err)
	}
	deletedMarker := repo.deletedMarkerPath(fixture.otherDeletedID)
	if _, err := os.Stat(deletedMarker); err != nil {
		t.Fatalf("other-workspace delete marker was not retained: %v", err)
	}
	foundDeletedRecord := false
	foundDeletedPayload := false
	if err := filepath.Walk(fixture.root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info == nil || info.IsDir() {
			return walkErr
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		foundDeletedRecord = foundDeletedRecord || strings.Contains(string(data), string(fixture.otherDeletedID))
		foundDeletedPayload = foundDeletedPayload || strings.Contains(string(data), fixture.otherDeletedPayload)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !foundDeletedRecord || !foundDeletedPayload {
		t.Fatalf("non-target tombstoned physical record/payload lost: record=%v payload=%v", foundDeletedRecord, foundDeletedPayload)
	}
	for id, want := range map[common.EventRecordID][]byte{
		fixture.keep.ID: fixture.keepPayloadBytes, fixture.otherDeletedID: fixture.deletedPayloadBytes,
	} {
		got, err := os.ReadFile(filepath.Join(fixture.root, "payloads", safeEventRecordFileName(string(id))+".json"))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("non-target external payload bytes drifted for %s: got=%d want=%d err=%v", id, len(got), len(want), err)
		}
	}
	streamPositionAfter, err := os.ReadFile(repo.eventRecordStreamPositionPath())
	if err != nil || string(streamPositionAfter) != string(fixture.streamPositionBefore) {
		t.Fatalf("stream-position bytes drifted: before=%q after=%q err=%v", fixture.streamPositionBefore, streamPositionAfter, err)
	}
	if _, err := repo.purgeWorkspace(ctx, fixture.expected); err != nil {
		t.Fatalf("second exact purge replay: %v", err)
	}
	next, err := repo.ReserveEventRecordStreamPosition(ctx)
	if err != nil || next <= fixture.keep.StreamPosition+1 {
		t.Fatalf("stream position after purge = %d err=%v, want monotonic above %d", next, err, fixture.keep.StreamPosition+1)
	}
	if _, err := os.Stat(filepath.Join(fixture.root, eventRecordPurgeSwapJournalName)); !os.IsNotExist(err) {
		t.Fatalf("purge journal remains: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(fixture.root))
	if err != nil {
		t.Fatal(err)
	}
	managedPrefix := "." + filepath.Base(fixture.root) + ".eventstream-purge-"
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), managedPrefix) {
			t.Fatalf("managed purge directory remains: %s", entry.Name())
		}
	}
	randomData, err := os.ReadFile(filepath.Join(fixture.randomBackup, "events", "not-authority"))
	if err != nil || string(randomData) != "random-backup-must-never-win" {
		t.Fatalf("random backup was treated as authority or cleanup target: %q err=%v", randomData, err)
	}
}

func TestEventRecordPurgeSwapRecoversEveryDurableCutPoint(t *testing.T) {
	for _, cut := range eventRecordPurgeSwapCrashCuts() {
		cut := cut
		t.Run(string(cut), func(t *testing.T) {
			fixture := newEventRecordPurgeCrashFixture(t)
			crashEventRecordPurgeAt(t, fixture, cut)

			fresh := NewEventRecordRepository(fixture.root)
			// A normal read is allowed to be the first post-restart access. It
			// must recover before observing any partially swapped generation.
			if _, err := fresh.List(context.Background(), workorchquerycontract.EventRecordQuery{IncludePayload: true}); err != nil {
				t.Fatalf("first post-crash List at %q: %v", cut, err)
			}
			assertEventRecordPurgeFixedPoint(t, fresh, fixture)
		})
	}
}

func TestEventRecordPurgeSwapEveryRepositoryIngressRecoversFirst(t *testing.T) {
	tests := []struct {
		name string
		run  func(*EventRecordRepository, eventRecordPurgeCrashFixture) error
	}{
		{name: "append", run: func(repo *EventRecordRepository, fixture eventRecordPurgeCrashFixture) error {
			return repo.Append(context.Background(), &eventstreammodel.EventRecord{
				ID: "event-after-recovery", Version: "workorch.event", Topic: "/after/recovery", OccurredAt: "2026-08-26T01:02:05Z",
				SourceModule: "blocks/test", SubjectType: "test", SubjectID: "after-recovery",
				WorkspaceID: "ws-after-recovery", StreamPosition: 40, Revision: 1,
				Payload: mustFileRepositoryPayload(t, map[string]any{"ok": true}),
			})
		}},
		{name: "reserve", run: func(repo *EventRecordRepository, _ eventRecordPurgeCrashFixture) error {
			_, err := repo.ReserveEventRecordStreamPosition(context.Background())
			return err
		}},
		{name: "observe", run: func(repo *EventRecordRepository, _ eventRecordPurgeCrashFixture) error {
			return repo.ObserveEventRecordStreamPosition(41)
		}},
		{name: "get", run: func(repo *EventRecordRepository, fixture eventRecordPurgeCrashFixture) error {
			_, err := repo.Get(context.Background(), fixture.keep.ID)
			return err
		}},
		{name: "list", run: func(repo *EventRecordRepository, _ eventRecordPurgeCrashFixture) error {
			_, err := repo.List(context.Background(), workorchquerycontract.EventRecordQuery{})
			return err
		}},
		{name: "delete", run: func(repo *EventRecordRepository, _ eventRecordPurgeCrashFixture) error {
			return repo.Delete(context.Background(), "event-target-crash")
		}},
		{name: "purge", run: func(repo *EventRecordRepository, fixture eventRecordPurgeCrashFixture) error {
			_, err := repo.purgeWorkspace(context.Background(), fixture.expected)
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newEventRecordPurgeCrashFixture(t)
			crashEventRecordPurgeAt(t, fixture, eventRecordPurgeSwapCut("after_old_rename_events"))
			fresh := NewEventRecordRepository(fixture.root)
			if err := tt.run(fresh, fixture); err != nil {
				t.Fatalf("first %s after crash: %v", tt.name, err)
			}
			assertRetainedEventRecordExact(t, fresh, fixture)
		})
	}
}

func TestEventRecordPurgeSwapRejectsStaleFenceWithoutRecoveryMutation(t *testing.T) {
	fixture := newEventRecordPurgeCrashFixture(t)
	crashEventRecordPurgeAt(t, fixture, eventRecordPurgeSwapCut("after_old_rename_events"))
	beforePresent, beforeHash, err := inspectEventRecordPurgeTree(filepath.Dir(fixture.root))
	if err != nil || !beforePresent {
		t.Fatal(err)
	}
	stale := fixture.expected
	stale.FenceID += "-drift"
	fresh := NewEventRecordRepository(fixture.root)
	if _, err := fresh.purgeWorkspace(context.Background(), stale); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("stale purge error = %v, want conflict", err)
	}
	afterPresent, afterHash, err := inspectEventRecordPurgeTree(filepath.Dir(fixture.root))
	if err != nil || !afterPresent || afterHash != beforeHash {
		t.Fatalf("stale purge mutated pending swap: before=%s after=%s err=%v", beforeHash, afterHash, err)
	}
	if _, err := fresh.List(context.Background(), workorchquerycontract.EventRecordQuery{}); err != nil {
		t.Fatalf("normal ingress did not recover exact pending fence: %v", err)
	}
	assertEventRecordPurgeFixedPoint(t, fresh, fixture)
}

func TestEventRecordPurgeSwapInstalledRequiresExactBackup(t *testing.T) {
	fixture := newEventRecordPurgeCrashFixture(t)
	crashEventRecordPurgeAt(t, fixture, eventRecordPurgeSwapCutInstalled)
	journal, err := loadEventRecordPurgeSwapJournal(fixture.root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(journal.BackupRoot, "events")); err != nil {
		t.Fatal(err)
	}
	beforePresent, beforeHash, err := inspectEventRecordPurgeTree(filepath.Dir(fixture.root))
	if err != nil || !beforePresent {
		t.Fatal(err)
	}
	if _, err := NewEventRecordRepository(fixture.root).List(context.Background(), workorchquerycontract.EventRecordQuery{}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("List error = %v, want installed-backup conflict", err)
	}
	afterPresent, afterHash, err := inspectEventRecordPurgeTree(filepath.Dir(fixture.root))
	if err != nil || !afterPresent || afterHash != beforeHash {
		t.Fatalf("invalid installed backup mutated state: before=%s after=%s err=%v", beforeHash, afterHash, err)
	}
}

func TestEventRecordPurgeSwapJournalIsStrictAndContained(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*eventRecordPurgeSwapJournal, map[string]any)
	}{
		{name: "unknown field", mutate: func(_ *eventRecordPurgeSwapJournal, wire map[string]any) { wire["retiredRecoveryGuess"] = true }},
		{name: "path escape", mutate: func(journal *eventRecordPurgeSwapJournal, wire map[string]any) {
			wire["replacementRoot"] = filepath.Join(filepath.Dir(filepath.Dir(journal.Root)), "escaped-event-records")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newEventRecordPurgeCrashFixture(t)
			crashEventRecordPurgeAt(t, fixture, eventRecordPurgeSwapCutPreparing)
			journal, err := loadEventRecordPurgeSwapJournal(fixture.root)
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(fixture.root, eventRecordPurgeSwapJournalName))
			if err != nil {
				t.Fatal(err)
			}
			wire := map[string]any{}
			if err := json.Unmarshal(data, &wire); err != nil {
				t.Fatal(err)
			}
			tt.mutate(journal, wire)
			data, err = json.Marshal(wire)
			if err != nil {
				t.Fatal(err)
			}
			if err := writeFileAtomic(filepath.Join(fixture.root, eventRecordPurgeSwapJournalName), data, 0o600); err != nil {
				t.Fatal(err)
			}
			beforePresent, beforeHash, err := inspectEventRecordPurgeTree(filepath.Dir(fixture.root))
			if err != nil || !beforePresent {
				t.Fatal(err)
			}
			if _, err := NewEventRecordRepository(fixture.root).List(context.Background(), workorchquerycontract.EventRecordQuery{}); err == nil {
				t.Fatal("List error = nil, want corrupt/escaped journal rejection")
			}
			afterPresent, afterHash, err := inspectEventRecordPurgeTree(filepath.Dir(fixture.root))
			if err != nil || !afterPresent || afterHash != beforeHash {
				t.Fatalf("invalid journal mutated state: before=%s after=%s err=%v", beforeHash, afterHash, err)
			}
		})
	}
}

func TestEventRecordPurgeSwapJournalRejectsImpossibleProgressWithoutMutation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*eventRecordPurgeSwapJournal)
	}{
		{name: "old progress is not a prefix", mutate: func(journal *eventRecordPurgeSwapJournal) {
			journal.Entries[1].OldMoved = true
		}},
		{name: "replacement progress is not a prefix", mutate: func(journal *eventRecordPurgeSwapJournal) {
			for idx := range journal.Entries {
				journal.Entries[idx].OldMoved = journal.Entries[idx].OldPresent
			}
			journal.Entries[1].ReplacementMoved = true
		}},
		{name: "replacement precedes old completion", mutate: func(journal *eventRecordPurgeSwapJournal) {
			journal.Entries[0].ReplacementMoved = true
		}},
		{name: "intent is not next exact move", mutate: func(journal *eventRecordPurgeSwapJournal) {
			journal.IntentKind = "move_old"
			journal.IntentName = journal.Entries[1].Name
		}},
		{name: "intent points to absent source", mutate: func(journal *eventRecordPurgeSwapJournal) {
			journal.Entries[0].OldPresent = false
			journal.Entries[0].OldSHA256 = ""
			journal.IntentKind = "move_old"
			journal.IntentName = journal.Entries[0].Name
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newEventRecordPurgeCrashFixture(t)
			crashEventRecordPurgeAt(t, fixture, eventRecordPurgeSwapCutSwapping)
			journal, err := loadEventRecordPurgeSwapJournal(fixture.root)
			if err != nil {
				t.Fatal(err)
			}
			tt.mutate(journal)
			data, err := json.Marshal(journal)
			if err != nil {
				t.Fatal(err)
			}
			if err := writeFileAtomic(filepath.Join(fixture.root, eventRecordPurgeSwapJournalName), data, 0o600); err != nil {
				t.Fatal(err)
			}
			beforePresent, beforeHash, err := inspectEventRecordPurgeTree(filepath.Dir(fixture.root))
			if err != nil || !beforePresent {
				t.Fatal(err)
			}
			if _, err := NewEventRecordRepository(fixture.root).List(context.Background(), workorchquerycontract.EventRecordQuery{}); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("List error = %v, want impossible-progress conflict", err)
			}
			afterPresent, afterHash, err := inspectEventRecordPurgeTree(filepath.Dir(fixture.root))
			if err != nil || !afterPresent || afterHash != beforeHash {
				t.Fatalf("impossible progress mutated state: before=%s after=%s err=%v", beforeHash, afterHash, err)
			}
		})
	}
}

func TestEventRecordPurgeSwapJournalTempRecoveryIsUnambiguous(t *testing.T) {
	t.Run("temp without canonical", func(t *testing.T) {
		fixture := newEventRecordPurgeCrashFixture(t)
		tempPath := filepath.Join(fixture.root, eventRecordPurgeSwapJournalTempName)
		if err := os.WriteFile(tempPath, []byte(`{"partial":"preparing"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		fresh := NewEventRecordRepository(fixture.root)
		if _, err := fresh.List(context.Background(), workorchquerycontract.EventRecordQuery{}); err != nil {
			t.Fatalf("List with uncommitted initial temp: %v", err)
		}
		if _, err := os.Stat(tempPath); !os.IsNotExist(err) {
			t.Fatalf("uncommitted temp remains: %v", err)
		}
		assertRetainedEventRecordExact(t, fresh, fixture)
	})
	t.Run("canonical wins over temp", func(t *testing.T) {
		fixture := newEventRecordPurgeCrashFixture(t)
		crashEventRecordPurgeAt(t, fixture, eventRecordPurgeSwapCutPrepared)
		tempPath := filepath.Join(fixture.root, eventRecordPurgeSwapJournalTempName)
		if err := os.WriteFile(tempPath, []byte(`{"stale":"phase-update"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		fresh := NewEventRecordRepository(fixture.root)
		if _, err := fresh.List(context.Background(), workorchquerycontract.EventRecordQuery{}); err != nil {
			t.Fatalf("List with canonical and stale temp: %v", err)
		}
		if _, err := os.Stat(tempPath); !os.IsNotExist(err) {
			t.Fatalf("stale phase temp remains: %v", err)
		}
		assertRetainedEventRecordExact(t, fresh, fixture)
	})
	for _, tt := range []struct {
		name  string
		build func(string) error
	}{
		{name: "symlink", build: func(path string) error { return os.Symlink(filepath.Dir(path), path) }},
		{name: "special directory", build: func(path string) error { return os.Mkdir(path, 0o700) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newEventRecordPurgeCrashFixture(t)
			tempPath := filepath.Join(fixture.root, eventRecordPurgeSwapJournalTempName)
			if err := tt.build(tempPath); err != nil {
				t.Fatal(err)
			}
			if _, err := NewEventRecordRepository(fixture.root).List(context.Background(), workorchquerycontract.EventRecordQuery{}); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("List error = %v, want invalid temp conflict", err)
			}
		})
	}
	t.Run("multiple temps", func(t *testing.T) {
		fixture := newEventRecordPurgeCrashFixture(t)
		for _, name := range []string{eventRecordPurgeSwapJournalTempName, eventRecordPurgeSwapJournalTempName + ".ambiguous"} {
			if err := os.WriteFile(filepath.Join(fixture.root, name), []byte("partial"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := NewEventRecordRepository(fixture.root).List(context.Background(), workorchquerycontract.EventRecordQuery{}); !errors.Is(err, common.ErrConflict) {
			t.Fatalf("List error = %v, want ambiguous temp conflict", err)
		}
	})
}

func TestEventRecordPurgeSwapRejectsManagedOrphanButIgnoresUnmanagedRandomBackup(t *testing.T) {
	root := filepath.Join(t.TempDir(), "event-records")
	repo := NewEventRecordRepository(root)
	if _, err := repo.List(context.Background(), workorchquerycontract.EventRecordQuery{}); err != nil {
		t.Fatal(err)
	}
	randomBackup := filepath.Join(filepath.Dir(root), ".unmanaged-event-record-backup-untrusted")
	if err := os.MkdirAll(randomBackup, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := NewEventRecordRepository(root).List(context.Background(), workorchquerycontract.EventRecordQuery{}); err != nil {
		t.Fatalf("unmanaged random backup became authority: %v", err)
	}
	managedOrphan := filepath.Join(filepath.Dir(root), "."+filepath.Base(root)+".eventstream-purge-orphan.backup")
	if err := os.MkdirAll(managedOrphan, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := NewEventRecordRepository(root).List(context.Background(), workorchquerycontract.EventRecordQuery{}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("managed orphan List error = %v, want conflict", err)
	}
}

func TestEventRecordPurgeSwapRejectsSymlinkedRootManagedPathAndTree(t *testing.T) {
	t.Run("root", func(t *testing.T) {
		actual := filepath.Join(t.TempDir(), "actual")
		if err := os.MkdirAll(actual, 0o700); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(filepath.Dir(actual), "event-record-link")
		if err := os.Symlink(actual, link); err != nil {
			t.Fatal(err)
		}
		if _, err := NewEventRecordRepository(link).List(context.Background(), workorchquerycontract.EventRecordQuery{}); !errors.Is(err, common.ErrConflict) {
			t.Fatalf("symlink root List error = %v, want conflict", err)
		}
	})
	t.Run("managed replacement", func(t *testing.T) {
		fixture := newEventRecordPurgeCrashFixture(t)
		crashEventRecordPurgeAt(t, fixture, eventRecordPurgeSwapCutPreparing)
		journal, err := loadEventRecordPurgeSwapJournal(fixture.root)
		if err != nil {
			t.Fatal(err)
		}
		outside := filepath.Join(t.TempDir(), "outside")
		if err := os.MkdirAll(outside, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, journal.ReplacementRoot); err != nil {
			t.Fatal(err)
		}
		if _, err := NewEventRecordRepository(fixture.root).List(context.Background(), workorchquerycontract.EventRecordQuery{}); !errors.Is(err, common.ErrConflict) {
			t.Fatalf("symlink managed root List error = %v, want conflict", err)
		}
	})
	t.Run("managed tree", func(t *testing.T) {
		fixture := newEventRecordPurgeCrashFixture(t)
		outside := filepath.Join(t.TempDir(), "outside-payload")
		if err := os.WriteFile(outside, []byte("must-not-read"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(fixture.root, "payloads", "escaped.json")); err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.repo.purgeWorkspace(context.Background(), fixture.expected); !errors.Is(err, common.ErrConflict) {
			t.Fatalf("symlink tree purge error = %v, want conflict", err)
		}
		data, err := os.ReadFile(outside)
		if err != nil || string(data) != "must-not-read" {
			t.Fatalf("outside payload changed: %q err=%v", data, err)
		}
	})
}

func TestEventRecordPurgeSwapCutInventoryIsExact(t *testing.T) {
	fixture := newEventRecordPurgeCrashFixture(t)
	reached := make([]eventRecordPurgeSwapCut, 0)
	fixture.repo.eventRecordPurgeSwapCut = func(cut eventRecordPurgeSwapCut) error {
		reached = append(reached, cut)
		return nil
	}
	if deleted, err := fixture.repo.purgeWorkspace(context.Background(), fixture.expected); err != nil || deleted != 1 {
		t.Fatalf("purge deleted=%d err=%v", deleted, err)
	}
	want := eventRecordPurgeSwapCrashCuts()
	if fmt.Sprint(reached) != fmt.Sprint(want) {
		t.Fatalf("cut inventory drifted:\n got %v\nwant %v", reached, want)
	}
}
