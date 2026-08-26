package file

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	eventstream "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

const (
	eventRecordPurgeSwapJournalVersion  = "workorch.eventstream-purge-swap.v1"
	eventRecordPurgeSwapJournalName     = ".eventstream-purge-swap.json"
	eventRecordPurgeSwapJournalTempName = ".eventstream-purge-swap.pending"
)

var eventRecordPurgeSwapNames = [...]string{"events", "payloads", "ids", "deleted"}

// eventRecordPurgeSwapPhase is Eventstream-owned technical state. Preparing
// and swapping always recover by rolling back to the exact old generation;
// installed and completed only roll forward. That one-way decision prevents a
// process death from making retained workspaces temporarily disappear.
type eventRecordPurgeSwapPhase string

const (
	eventRecordPurgeSwapPhasePreparing eventRecordPurgeSwapPhase = "preparing"
	eventRecordPurgeSwapPhasePrepared  eventRecordPurgeSwapPhase = "prepared"
	eventRecordPurgeSwapPhaseSwapping  eventRecordPurgeSwapPhase = "swapping"
	eventRecordPurgeSwapPhaseInstalled eventRecordPurgeSwapPhase = "installed"
	eventRecordPurgeSwapPhaseCompleted eventRecordPurgeSwapPhase = "completed"
)

type eventRecordPurgeSwapCut string

const (
	eventRecordPurgeSwapCutPreparing          eventRecordPurgeSwapCut = "preparing"
	eventRecordPurgeSwapCutPrepared           eventRecordPurgeSwapCut = "prepared"
	eventRecordPurgeSwapCutSwapping           eventRecordPurgeSwapCut = "swapping"
	eventRecordPurgeSwapCutInstalled          eventRecordPurgeSwapCut = "installed"
	eventRecordPurgeSwapCutCompleted          eventRecordPurgeSwapCut = "completed"
	eventRecordPurgeSwapCutBackupRemoved      eventRecordPurgeSwapCut = "backup_removed"
	eventRecordPurgeSwapCutReplacementRemoved eventRecordPurgeSwapCut = "replacement_removed"
	eventRecordPurgeSwapCutJournalRemoved     eventRecordPurgeSwapCut = "journal_removed"
)

type eventRecordPurgeSwapEntry struct {
	Name               string `json:"name"`
	OldPresent         bool   `json:"oldPresent"`
	OldSHA256          string `json:"oldSha256,omitempty"`
	ReplacementPresent bool   `json:"replacementPresent"`
	ReplacementSHA256  string `json:"replacementSha256,omitempty"`
	OldMoved           bool   `json:"oldMoved"`
	ReplacementMoved   bool   `json:"replacementMoved"`
}

type eventRecordPurgeRetainedRecord struct {
	Record  *eventstream.EventRecord
	Deleted bool
}

type eventRecordPurgeSwapJournal struct {
	Version               string                        `json:"version"`
	TransactionID         string                        `json:"transactionId"`
	Expected              eventstream.WorkspaceDeletion `json:"expected"`
	Root                  string                        `json:"root"`
	ReplacementRoot       string                        `json:"replacementRoot"`
	BackupRoot            string                        `json:"backupRoot"`
	Phase                 eventRecordPurgeSwapPhase     `json:"phase"`
	IntentKind            string                        `json:"intentKind,omitempty"`
	IntentName            string                        `json:"intentName,omitempty"`
	Entries               []eventRecordPurgeSwapEntry   `json:"entries"`
	StreamPositionPresent bool                          `json:"streamPositionPresent"`
	StreamPositionSHA256  string                        `json:"streamPositionSha256,omitempty"`
	Deleted               int                           `json:"deleted"`
}

func (r *EventRecordRepository) installEventRecordPurgeSwap(expected eventstream.WorkspaceDeletion, retained []eventRecordPurgeRetainedRecord, deleted int) (int, error) {
	root, err := r.eventRecordPurgeCanonicalRoot()
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return 0, err
	}
	if err := validateEventRecordPurgePath(root, false); err != nil {
		return 0, err
	}
	if err := syncEventRecordPurgeDirectory(filepath.Dir(root)); err != nil {
		return 0, err
	}

	journal, err := newEventRecordPurgeSwapJournal(root, expected, deleted)
	if err != nil {
		return 0, err
	}
	for idx := range journal.Entries {
		entry := &journal.Entries[idx]
		if err := syncEventRecordPurgeTree(filepath.Join(root, entry.Name)); err != nil {
			return 0, err
		}
		entry.OldPresent, entry.OldSHA256, err = inspectEventRecordPurgeTree(filepath.Join(root, entry.Name))
		if err != nil {
			return 0, err
		}
	}
	streamPath := filepath.Join(root, ".eventstream-stream-position.json")
	if err := syncEventRecordPurgeRegularFile(streamPath); err != nil {
		return 0, err
	}
	journal.StreamPositionPresent, journal.StreamPositionSHA256, err = inspectEventRecordPurgeRegularFile(streamPath)
	if err != nil {
		return 0, err
	}
	if err := r.persistEventRecordPurgeSwapJournal(journal); err != nil {
		return 0, err
	}
	if err := r.invokeEventRecordPurgeSwapCut(eventRecordPurgeSwapCutPreparing); err != nil {
		return 0, err
	}

	if err := makeEventRecordPurgeManagedDirectory(journal.ReplacementRoot); err != nil {
		return 0, err
	}
	if err := makeEventRecordPurgeManagedDirectory(journal.BackupRoot); err != nil {
		return 0, err
	}
	if err := syncEventRecordPurgeDirectory(filepath.Dir(root)); err != nil {
		return 0, err
	}
	temporaryRepository := NewEventRecordRepository(journal.ReplacementRoot)
	for _, item := range retained {
		if item.Record == nil {
			return 0, common.ErrInvalidArgument
		}
		if err := temporaryRepository.Append(common.ContextWithRuntimeWorkspaceID(context.Background(), item.Record.WorkspaceID), item.Record); err != nil {
			return 0, err
		}
		if err := r.invokeEventRecordPurgeSwapCut(eventRecordPurgeSwapCut("after_preparing_append_" + string(item.Record.ID))); err != nil {
			return 0, err
		}
		if item.Deleted {
			if err := temporaryRepository.Delete(context.Background(), item.Record.ID); err != nil {
				return 0, err
			}
			if err := r.invokeEventRecordPurgeSwapCut(eventRecordPurgeSwapCut("after_preparing_delete_" + string(item.Record.ID))); err != nil {
				return 0, err
			}
		}
	}
	if err := syncEventRecordPurgeTree(journal.ReplacementRoot); err != nil {
		return 0, err
	}
	for idx := range journal.Entries {
		entry := &journal.Entries[idx]
		entry.ReplacementPresent, entry.ReplacementSHA256, err = inspectEventRecordPurgeTree(filepath.Join(journal.ReplacementRoot, entry.Name))
		if err != nil {
			return 0, err
		}
	}
	journal.Phase = eventRecordPurgeSwapPhasePrepared
	if err := r.persistEventRecordPurgeSwapJournal(journal); err != nil {
		return 0, err
	}
	if err := r.invokeEventRecordPurgeSwapCut(eventRecordPurgeSwapCutPrepared); err != nil {
		return 0, err
	}

	journal.Phase = eventRecordPurgeSwapPhaseSwapping
	if err := r.persistEventRecordPurgeSwapJournal(journal); err != nil {
		return 0, err
	}
	if err := r.invokeEventRecordPurgeSwapCut(eventRecordPurgeSwapCutSwapping); err != nil {
		return 0, err
	}
	for idx := range journal.Entries {
		entry := &journal.Entries[idx]
		if !entry.OldPresent {
			continue
		}
		if err := r.moveEventRecordPurgeSwapEntry(journal, entry, "old"); err != nil {
			return 0, err
		}
	}
	for idx := range journal.Entries {
		entry := &journal.Entries[idx]
		if !entry.ReplacementPresent {
			continue
		}
		if err := r.moveEventRecordPurgeSwapEntry(journal, entry, "replacement"); err != nil {
			return 0, err
		}
	}
	if err := verifyInstalledEventRecordPurgeSwap(journal, true); err != nil {
		return 0, err
	}
	journal.Phase = eventRecordPurgeSwapPhaseInstalled
	journal.IntentKind = ""
	journal.IntentName = ""
	if err := r.persistEventRecordPurgeSwapJournal(journal); err != nil {
		return 0, err
	}
	if err := r.invokeEventRecordPurgeSwapCut(eventRecordPurgeSwapCutInstalled); err != nil {
		return 0, err
	}
	journal.Phase = eventRecordPurgeSwapPhaseCompleted
	if err := r.persistEventRecordPurgeSwapJournal(journal); err != nil {
		return 0, err
	}
	if err := r.invokeEventRecordPurgeSwapCut(eventRecordPurgeSwapCutCompleted); err != nil {
		return 0, err
	}
	if err := r.finishEventRecordPurgeSwap(journal); err != nil {
		return 0, err
	}
	return deleted, nil
}

func (r *EventRecordRepository) moveEventRecordPurgeSwapEntry(journal *eventRecordPurgeSwapJournal, entry *eventRecordPurgeSwapEntry, kind string) error {
	if journal == nil || entry == nil || (kind != "old" && kind != "replacement") {
		return common.ErrInvalidArgument
	}
	journal.IntentKind = "move_" + kind
	journal.IntentName = entry.Name
	if err := r.persistEventRecordPurgeSwapJournal(journal); err != nil {
		return err
	}
	if err := r.invokeEventRecordPurgeSwapCut(eventRecordPurgeSwapCut("before_" + kind + "_rename_" + entry.Name)); err != nil {
		return err
	}
	fromRoot, toRoot := journal.Root, journal.BackupRoot
	if kind == "replacement" {
		fromRoot, toRoot = journal.ReplacementRoot, journal.Root
	}
	if err := os.Rename(filepath.Join(fromRoot, entry.Name), filepath.Join(toRoot, entry.Name)); err != nil {
		return err
	}
	if err := syncEventRecordPurgeDirectories(fromRoot, toRoot); err != nil {
		return err
	}
	if err := r.invokeEventRecordPurgeSwapCut(eventRecordPurgeSwapCut("after_" + kind + "_rename_" + entry.Name)); err != nil {
		return err
	}
	if kind == "old" {
		entry.OldMoved = true
	} else {
		entry.ReplacementMoved = true
	}
	journal.IntentKind = ""
	journal.IntentName = ""
	if err := r.persistEventRecordPurgeSwapJournal(journal); err != nil {
		return err
	}
	return r.invokeEventRecordPurgeSwapCut(eventRecordPurgeSwapCut("recorded_" + kind + "_rename_" + entry.Name))
}

func (r *EventRecordRepository) recoverEventRecordPurgeSwap(expected *eventstream.WorkspaceDeletion) (bool, int, error) {
	root, err := r.eventRecordPurgeCanonicalRoot()
	if err != nil {
		return false, 0, err
	}
	if err := reconcileEventRecordPurgeJournalTemps(root); err != nil {
		return false, 0, err
	}
	journal, err := loadEventRecordPurgeSwapJournal(root)
	if err != nil {
		if os.IsNotExist(err) {
			if orphanErr := rejectEventRecordPurgeManagedOrphans(root); orphanErr != nil {
				return false, 0, orphanErr
			}
			return false, 0, nil
		}
		return false, 0, err
	}
	if err := validateEventRecordPurgeSwapJournal(journal, root); err != nil {
		return false, 0, err
	}
	if expected != nil && journal.Expected != *expected {
		return false, 0, fmt.Errorf("%w: event record purge swap fence drift", common.ErrConflict)
	}
	if err := verifyEventRecordPurgeStreamPosition(journal); err != nil {
		return false, 0, err
	}
	switch journal.Phase {
	case eventRecordPurgeSwapPhasePreparing, eventRecordPurgeSwapPhasePrepared, eventRecordPurgeSwapPhaseSwapping:
		if err := rollbackEventRecordPurgeSwap(journal); err != nil {
			return false, 0, err
		}
		if err := r.finishRolledBackEventRecordPurgeSwap(journal); err != nil {
			return false, 0, err
		}
		return false, 0, nil
	case eventRecordPurgeSwapPhaseInstalled, eventRecordPurgeSwapPhaseCompleted:
		if err := verifyInstalledEventRecordPurgeSwap(journal, journal.Phase == eventRecordPurgeSwapPhaseInstalled); err != nil {
			return false, 0, err
		}
		if journal.Phase == eventRecordPurgeSwapPhaseInstalled {
			journal.Phase = eventRecordPurgeSwapPhaseCompleted
			if err := r.persistEventRecordPurgeSwapJournal(journal); err != nil {
				return false, 0, err
			}
		}
		if err := r.finishEventRecordPurgeSwap(journal); err != nil {
			return false, 0, err
		}
		return true, journal.Deleted, nil
	default:
		return false, 0, fmt.Errorf("%w: unknown event record purge swap phase", common.ErrConflict)
	}
}

func newEventRecordPurgeSwapJournal(root string, expected eventstream.WorkspaceDeletion, deleted int) (*eventRecordPurgeSwapJournal, error) {
	if expected.Validate() != nil || expected.Phase != eventstream.WorkspaceDeletionPhaseFenced || expected.Mode != deletioncontract.ModePurge || deleted < 0 {
		return nil, common.ErrInvalidArgument
	}
	txID, err := eventRecordPurgeSwapTransactionID(root, expected)
	if err != nil {
		return nil, err
	}
	replacementRoot, backupRoot := eventRecordPurgeSwapManagedRoots(root, txID)
	entries := make([]eventRecordPurgeSwapEntry, 0, len(eventRecordPurgeSwapNames))
	for _, name := range eventRecordPurgeSwapNames {
		entries = append(entries, eventRecordPurgeSwapEntry{Name: name})
	}
	return &eventRecordPurgeSwapJournal{
		Version: eventRecordPurgeSwapJournalVersion, TransactionID: txID, Expected: expected, Root: root,
		ReplacementRoot: replacementRoot, BackupRoot: backupRoot, Phase: eventRecordPurgeSwapPhasePreparing,
		Entries: entries, Deleted: deleted,
	}, nil
}

func eventRecordPurgeSwapTransactionID(root string, expected eventstream.WorkspaceDeletion) (string, error) {
	data, err := json.Marshal(struct {
		Root     string                        `json:"root"`
		Expected eventstream.WorkspaceDeletion `json:"expected"`
	}{Root: root, Expected: expected})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:]), nil
}

func eventRecordPurgeSwapManagedRoots(root, txID string) (string, string) {
	prefix := "." + filepath.Base(root) + ".eventstream-purge-" + txID
	parent := filepath.Dir(root)
	return filepath.Join(parent, prefix+".replacement"), filepath.Join(parent, prefix+".backup")
}

func validateEventRecordPurgeSwapJournal(journal *eventRecordPurgeSwapJournal, root string) error {
	if journal == nil || journal.Version != eventRecordPurgeSwapJournalVersion || journal.Expected.Validate() != nil ||
		journal.Expected.Phase != eventstream.WorkspaceDeletionPhaseFenced || journal.Expected.Mode != deletioncontract.ModePurge ||
		journal.Root != root || journal.Deleted < 0 || len(journal.Entries) != len(eventRecordPurgeSwapNames) {
		return fmt.Errorf("%w: invalid event record purge swap journal", common.ErrConflict)
	}
	txID, err := eventRecordPurgeSwapTransactionID(root, journal.Expected)
	if err != nil || journal.TransactionID != txID {
		return fmt.Errorf("%w: invalid event record purge swap transaction identity", common.ErrConflict)
	}
	replacementRoot, backupRoot := eventRecordPurgeSwapManagedRoots(root, txID)
	if journal.ReplacementRoot != replacementRoot || journal.BackupRoot != backupRoot {
		return fmt.Errorf("%w: event record purge swap path escaped its root", common.ErrConflict)
	}
	if err := validateEventRecordPurgeManagedRoot(root, replacementRoot); err != nil {
		return err
	}
	if err := validateEventRecordPurgeManagedRoot(root, backupRoot); err != nil {
		return err
	}
	if err := validateEventRecordPurgeManagedContents(replacementRoot, true); err != nil {
		return err
	}
	if err := validateEventRecordPurgeManagedContents(backupRoot, false); err != nil {
		return err
	}
	validPhase := journal.Phase == eventRecordPurgeSwapPhasePreparing || journal.Phase == eventRecordPurgeSwapPhasePrepared ||
		journal.Phase == eventRecordPurgeSwapPhaseSwapping || journal.Phase == eventRecordPurgeSwapPhaseInstalled || journal.Phase == eventRecordPurgeSwapPhaseCompleted
	if !validPhase {
		return fmt.Errorf("%w: invalid event record purge swap phase", common.ErrConflict)
	}
	if (journal.IntentKind == "") != (journal.IntentName == "") ||
		(journal.IntentKind != "" && journal.IntentKind != "move_old" && journal.IntentKind != "move_replacement") ||
		(journal.IntentKind != "" && journal.Phase != eventRecordPurgeSwapPhaseSwapping) {
		return fmt.Errorf("%w: invalid event record purge swap intent", common.ErrConflict)
	}
	intentFound := journal.IntentName == ""
	var intentEntry *eventRecordPurgeSwapEntry
	for idx, expectedName := range eventRecordPurgeSwapNames {
		entry := &journal.Entries[idx]
		if entry.Name != expectedName || (entry.OldPresent != validEventRecordPurgeDigest(entry.OldSHA256)) ||
			(entry.ReplacementPresent != validEventRecordPurgeDigest(entry.ReplacementSHA256)) {
			return fmt.Errorf("%w: invalid event record purge swap entry", common.ErrConflict)
		}
		if journal.IntentName == entry.Name {
			intentFound = true
			intentEntry = entry
		}
		if entry.OldMoved && !entry.OldPresent || entry.ReplacementMoved && !entry.ReplacementPresent {
			return fmt.Errorf("%w: event record purge swap progress has no source directory", common.ErrConflict)
		}
		if journal.Phase == eventRecordPurgeSwapPhasePreparing && (entry.ReplacementPresent || entry.OldMoved || entry.ReplacementMoved) {
			return fmt.Errorf("%w: preparing event record purge swap contains installed progress", common.ErrConflict)
		}
		if journal.Phase == eventRecordPurgeSwapPhasePrepared && (entry.OldMoved || entry.ReplacementMoved) {
			return fmt.Errorf("%w: prepared event record purge swap contains installed progress", common.ErrConflict)
		}
		if (journal.Phase == eventRecordPurgeSwapPhaseInstalled || journal.Phase == eventRecordPurgeSwapPhaseCompleted) &&
			(entry.OldMoved != entry.OldPresent || entry.ReplacementMoved != entry.ReplacementPresent) {
			return fmt.Errorf("%w: installed event record purge swap has incomplete progress", common.ErrConflict)
		}
	}
	if !intentFound {
		return fmt.Errorf("%w: invalid event record purge swap intent entry", common.ErrConflict)
	}
	if err := validateEventRecordPurgeSwapProgress(journal, intentEntry); err != nil {
		return err
	}
	if journal.StreamPositionPresent != validEventRecordPurgeDigest(journal.StreamPositionSHA256) {
		return fmt.Errorf("%w: invalid event record purge stream-position snapshot", common.ErrConflict)
	}
	return nil
}

func validateEventRecordPurgeSwapProgress(journal *eventRecordPurgeSwapJournal, intentEntry *eventRecordPurgeSwapEntry) error {
	if journal.Phase != eventRecordPurgeSwapPhaseSwapping {
		return nil
	}
	oldGap := false
	replacementGap := false
	oldComplete := true
	anyReplacementMoved := false
	firstUnmovedOld := ""
	firstUnmovedReplacement := ""
	for idx := range journal.Entries {
		entry := journal.Entries[idx]
		if entry.OldPresent {
			if !entry.OldMoved {
				oldGap = true
				oldComplete = false
				if firstUnmovedOld == "" {
					firstUnmovedOld = entry.Name
				}
			} else if oldGap {
				return fmt.Errorf("%w: event record purge old progress is not an exact prefix", common.ErrConflict)
			}
		}
		if entry.ReplacementPresent {
			if !entry.ReplacementMoved {
				replacementGap = true
				if firstUnmovedReplacement == "" {
					firstUnmovedReplacement = entry.Name
				}
			} else if replacementGap {
				return fmt.Errorf("%w: event record purge replacement progress is not an exact prefix", common.ErrConflict)
			}
			anyReplacementMoved = anyReplacementMoved || entry.ReplacementMoved
		}
	}
	if anyReplacementMoved && !oldComplete {
		return fmt.Errorf("%w: event record purge installed replacement before old generation moved", common.ErrConflict)
	}
	if journal.IntentKind == "" {
		return nil
	}
	if intentEntry == nil {
		return fmt.Errorf("%w: event record purge intent has no exact entry", common.ErrConflict)
	}
	switch journal.IntentKind {
	case "move_old":
		if !intentEntry.OldPresent || intentEntry.OldMoved || firstUnmovedOld != intentEntry.Name {
			return fmt.Errorf("%w: event record purge old intent does not match the next exact move", common.ErrConflict)
		}
		for _, entry := range journal.Entries {
			if entry.ReplacementMoved {
				return fmt.Errorf("%w: event record purge old intent follows replacement progress", common.ErrConflict)
			}
		}
	case "move_replacement":
		if !oldComplete || !intentEntry.ReplacementPresent || intentEntry.ReplacementMoved || firstUnmovedReplacement != intentEntry.Name {
			return fmt.Errorf("%w: event record purge replacement intent does not match the next exact move", common.ErrConflict)
		}
	default:
		return fmt.Errorf("%w: invalid event record purge intent kind", common.ErrConflict)
	}
	return nil
}

func validEventRecordPurgeDigest(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	for _, ch := range value {
		if !strings.ContainsRune("0123456789abcdef", ch) {
			return false
		}
	}
	return true
}

func (r *EventRecordRepository) eventRecordPurgeCanonicalRoot() (string, error) {
	if r == nil {
		return "", common.ErrInvalidArgument
	}
	clean := filepath.Clean(strings.TrimSpace(r.eventLogRoot()))
	absRoot, err := filepath.Abs(clean)
	if err != nil || clean == "." || absRoot == string(filepath.Separator) || filepath.Base(absRoot) == "." {
		return "", common.ErrInvalidArgument
	}
	if err := validateEventRecordPurgePath(absRoot, true); err != nil {
		return "", err
	}
	return absRoot, nil
}

func validateEventRecordPurgePath(path string, allowMissing bool) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || path == string(filepath.Separator) {
		return common.ErrInvalidArgument
	}
	current := string(filepath.Separator)
	parts := strings.Split(strings.TrimPrefix(path, string(filepath.Separator)), string(filepath.Separator))
	missing := false
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return common.ErrInvalidArgument
		}
		current = filepath.Join(current, part)
		if missing {
			continue
		}
		info, err := os.Lstat(current)
		if err != nil {
			if os.IsNotExist(err) && allowMissing {
				missing = true
				continue
			}
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("%w: event record purge path contains a non-directory or symlink", common.ErrConflict)
		}
	}
	return nil
}

func validateEventRecordPurgeManagedRoot(root, managed string) error {
	if filepath.Dir(managed) != filepath.Dir(root) || managed == root {
		return fmt.Errorf("%w: event record purge managed path escaped its parent", common.ErrConflict)
	}
	info, err := os.Lstat(managed)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%w: event record purge managed path is not a directory", common.ErrConflict)
	}
	return nil
}

func validateEventRecordPurgeManagedContents(root string, allowStreamPosition bool) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	allowed := map[string]struct{}{}
	for _, name := range eventRecordPurgeSwapNames {
		allowed[name] = struct{}{}
	}
	for _, entry := range entries {
		name := entry.Name()
		path := filepath.Join(root, name)
		if allowStreamPosition && name == ".eventstream-stream-position.json" {
			if present, _, err := inspectEventRecordPurgeRegularFile(path); err != nil || !present {
				if err != nil {
					return err
				}
				return fmt.Errorf("%w: invalid replacement stream-position file", common.ErrConflict)
			}
			continue
		}
		if _, ok := allowed[name]; !ok {
			return fmt.Errorf("%w: event record purge managed root contains an unknown entry", common.ErrConflict)
		}
		if present, _, err := inspectEventRecordPurgeTree(path); err != nil || !present {
			if err != nil {
				return err
			}
			return fmt.Errorf("%w: invalid event record purge managed entry", common.ErrConflict)
		}
	}
	return nil
}

func loadEventRecordPurgeSwapJournal(root string) (*eventRecordPurgeSwapJournal, error) {
	path := filepath.Join(root, eventRecordPurgeSwapJournalName)
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: event record purge journal is not a regular file", common.ErrConflict)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	journal := &eventRecordPurgeSwapJournal{}
	if err := decodeCurrentJSON(data, journal); err != nil {
		return nil, err
	}
	return journal, nil
}

func (r *EventRecordRepository) persistEventRecordPurgeSwapJournal(journal *eventRecordPurgeSwapJournal) error {
	if journal == nil {
		return common.ErrInvalidArgument
	}
	path := filepath.Join(journal.Root, eventRecordPurgeSwapJournalName)
	if err := reconcileEventRecordPurgeJournalTemps(journal.Root); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil && (info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular()) {
		return fmt.Errorf("%w: refusing to replace invalid event record purge journal", common.ErrConflict)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	data, err := json.Marshal(journal)
	if err != nil {
		return err
	}
	tempPath := filepath.Join(journal.Root, eventRecordPurgeSwapJournalTempName)
	temp, err := os.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if err := syncEventRecordPurgeDirectory(journal.Root); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	return syncEventRecordPurgeDirectory(journal.Root)
}

// reconcileEventRecordPurgeJournalTemps makes the canonical journal the only
// authority after a process death inside its atomic write. A lone regular temp
// without a canonical journal can only come from the initial preparing write,
// before managed directories or any rename exist, and is safe to discard. Any
// ambiguous, special or symlinked temp shape fails closed.
func reconcileEventRecordPurgeJournalTemps(root string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	temps := make([]string, 0, 1)
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, eventRecordPurgeSwapJournalTempName+".") {
			return fmt.Errorf("%w: unknown event record purge journal temp name", common.ErrConflict)
		}
		if name == eventRecordPurgeSwapJournalTempName {
			temps = append(temps, filepath.Join(root, name))
		}
	}
	if len(temps) == 0 {
		return nil
	}
	if len(temps) != 1 {
		return fmt.Errorf("%w: ambiguous event record purge journal temp files", common.ErrConflict)
	}
	info, err := os.Lstat(temps[0])
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("%w: event record purge journal temp is not a regular file", common.ErrConflict)
	}
	canonicalPath := filepath.Join(root, eventRecordPurgeSwapJournalName)
	if _, err := os.Lstat(canonicalPath); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		if err := rejectEventRecordPurgeManagedOrphans(root); err != nil {
			return err
		}
	}
	if err := os.Remove(temps[0]); err != nil {
		return err
	}
	return syncEventRecordPurgeDirectory(root)
}

func rejectEventRecordPurgeManagedOrphans(root string) error {
	parent := filepath.Dir(root)
	entries, err := os.ReadDir(parent)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	prefix := "." + filepath.Base(root) + ".eventstream-purge-"
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, prefix) && (strings.HasSuffix(name, ".replacement") || strings.HasSuffix(name, ".backup")) {
			return fmt.Errorf("%w: managed event record purge directory exists without its journal", common.ErrConflict)
		}
	}
	return nil
}

func makeEventRecordPurgeManagedDirectory(path string) error {
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("%w: event record purge managed directory already exists", common.ErrConflict)
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Mkdir(path, 0o700)
}

func rollbackEventRecordPurgeSwap(journal *eventRecordPurgeSwapJournal) error {
	if journal.Phase == eventRecordPurgeSwapPhasePreparing {
		return verifyOriginalEventRecordPurgeSwap(journal)
	}
	if err := makeEventRecordPurgeRecoveryDirectory(journal.ReplacementRoot); err != nil {
		return err
	}
	for idx := len(journal.Entries) - 1; idx >= 0; idx-- {
		entry := journal.Entries[idx]
		rootPath := filepath.Join(journal.Root, entry.Name)
		backupPath := filepath.Join(journal.BackupRoot, entry.Name)
		replacementPath := filepath.Join(journal.ReplacementRoot, entry.Name)
		backupPresent, backupHash, err := inspectEventRecordPurgeTree(backupPath)
		if err != nil {
			return err
		}
		rootPresent, rootHash, err := inspectEventRecordPurgeTree(rootPath)
		if err != nil {
			return err
		}
		replacementPresent, _, err := inspectEventRecordPurgeTree(replacementPath)
		if err != nil {
			return err
		}
		if backupPresent {
			if !entry.OldPresent || backupHash != entry.OldSHA256 {
				return fmt.Errorf("%w: event record purge backup does not match old generation", common.ErrConflict)
			}
			if rootPresent {
				if !entry.ReplacementPresent || rootHash != entry.ReplacementSHA256 || replacementPresent {
					return fmt.Errorf("%w: event record purge rollback found an impossible installed directory", common.ErrConflict)
				}
				if err := os.Rename(rootPath, replacementPath); err != nil {
					return err
				}
				if err := syncEventRecordPurgeDirectories(journal.Root, journal.ReplacementRoot); err != nil {
					return err
				}
			}
			if err := os.Rename(backupPath, rootPath); err != nil {
				return err
			}
			if err := syncEventRecordPurgeDirectories(journal.BackupRoot, journal.Root); err != nil {
				return err
			}
			continue
		}
		if entry.OldPresent {
			if !rootPresent || rootHash != entry.OldSHA256 {
				return fmt.Errorf("%w: event record purge rollback cannot locate the old generation", common.ErrConflict)
			}
			continue
		}
		if rootPresent {
			if !entry.ReplacementPresent || rootHash != entry.ReplacementSHA256 || replacementPresent {
				return fmt.Errorf("%w: event record purge rollback found an untracked root directory", common.ErrConflict)
			}
			if err := os.Rename(rootPath, replacementPath); err != nil {
				return err
			}
			if err := syncEventRecordPurgeDirectories(journal.Root, journal.ReplacementRoot); err != nil {
				return err
			}
		}
	}
	return verifyOriginalEventRecordPurgeSwap(journal)
}

func verifyOriginalEventRecordPurgeSwap(journal *eventRecordPurgeSwapJournal) error {
	for _, entry := range journal.Entries {
		present, hash, err := inspectEventRecordPurgeTree(filepath.Join(journal.Root, entry.Name))
		if err != nil {
			return err
		}
		if present != entry.OldPresent || present && hash != entry.OldSHA256 {
			return fmt.Errorf("%w: event record purge old generation is not exact", common.ErrConflict)
		}
		backupPresent, _, err := inspectEventRecordPurgeTree(filepath.Join(journal.BackupRoot, entry.Name))
		if err != nil {
			return err
		}
		if backupPresent {
			return fmt.Errorf("%w: event record purge rollback left an old backup entry", common.ErrConflict)
		}
	}
	return verifyEventRecordPurgeStreamPosition(journal)
}

func verifyInstalledEventRecordPurgeSwap(journal *eventRecordPurgeSwapJournal, requireExactBackup bool) error {
	if requireExactBackup {
		for _, managed := range []string{journal.BackupRoot, journal.ReplacementRoot} {
			info, err := os.Lstat(managed)
			if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return fmt.Errorf("%w: installed event record purge lost a managed directory", common.ErrConflict)
			}
		}
	}
	for _, entry := range journal.Entries {
		present, hash, err := inspectEventRecordPurgeTree(filepath.Join(journal.Root, entry.Name))
		if err != nil {
			return err
		}
		if present != entry.ReplacementPresent || present && hash != entry.ReplacementSHA256 {
			return fmt.Errorf("%w: installed event record purge generation is not exact", common.ErrConflict)
		}
		replacementPresent, _, err := inspectEventRecordPurgeTree(filepath.Join(journal.ReplacementRoot, entry.Name))
		if err != nil {
			return err
		}
		if replacementPresent {
			return fmt.Errorf("%w: installed event record purge retained a replacement entry", common.ErrConflict)
		}
		if requireExactBackup {
			backupPresent, backupHash, err := inspectEventRecordPurgeTree(filepath.Join(journal.BackupRoot, entry.Name))
			if err != nil {
				return err
			}
			if backupPresent != entry.OldPresent || backupPresent && backupHash != entry.OldSHA256 {
				return fmt.Errorf("%w: installed event record purge backup is not exact", common.ErrConflict)
			}
		}
	}
	return verifyEventRecordPurgeStreamPosition(journal)
}

func verifyEventRecordPurgeStreamPosition(journal *eventRecordPurgeSwapJournal) error {
	present, hash, err := inspectEventRecordPurgeRegularFile(filepath.Join(journal.Root, ".eventstream-stream-position.json"))
	if err != nil {
		return err
	}
	if present != journal.StreamPositionPresent || present && hash != journal.StreamPositionSHA256 {
		return fmt.Errorf("%w: event record purge changed the stream-position floor", common.ErrConflict)
	}
	return nil
}

func (r *EventRecordRepository) finishRolledBackEventRecordPurgeSwap(journal *eventRecordPurgeSwapJournal) error {
	if err := removeEventRecordPurgeManagedDirectory(journal.BackupRoot); err != nil {
		return err
	}
	if err := removeEventRecordPurgeManagedDirectory(journal.ReplacementRoot); err != nil {
		return err
	}
	return removeEventRecordPurgeJournal(journal.Root)
}

func (r *EventRecordRepository) finishEventRecordPurgeSwap(journal *eventRecordPurgeSwapJournal) error {
	if err := removeEventRecordPurgeManagedDirectory(journal.BackupRoot); err != nil {
		return err
	}
	if err := r.invokeEventRecordPurgeSwapCut(eventRecordPurgeSwapCutBackupRemoved); err != nil {
		return err
	}
	if err := removeEventRecordPurgeManagedDirectory(journal.ReplacementRoot); err != nil {
		return err
	}
	if err := r.invokeEventRecordPurgeSwapCut(eventRecordPurgeSwapCutReplacementRemoved); err != nil {
		return err
	}
	if err := removeEventRecordPurgeJournal(journal.Root); err != nil {
		return err
	}
	return r.invokeEventRecordPurgeSwapCut(eventRecordPurgeSwapCutJournalRemoved)
}

func removeEventRecordPurgeManagedDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%w: refusing to remove invalid event record purge directory", common.ErrConflict)
	}
	if err := os.RemoveAll(path); err != nil {
		return err
	}
	return syncEventRecordPurgeDirectory(filepath.Dir(path))
}

func removeEventRecordPurgeJournal(root string) error {
	path := filepath.Join(root, eventRecordPurgeSwapJournalName)
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("%w: refusing to remove invalid event record purge journal", common.ErrConflict)
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	return syncEventRecordPurgeDirectory(root)
}

func makeEventRecordPurgeRecoveryDirectory(path string) error {
	info, err := os.Lstat(path)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("%w: invalid event record purge recovery directory", common.ErrConflict)
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		return err
	}
	return syncEventRecordPurgeDirectory(filepath.Dir(path))
}

func inspectEventRecordPurgeTree(path string) (bool, string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, "", nil
		}
		return false, "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return false, "", fmt.Errorf("%w: event record purge managed entry is not a directory", common.ErrConflict)
	}
	hash := sha256.New()
	err = filepath.WalkDir(path, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: event record purge tree contains a symlink", common.ErrConflict)
		}
		rel, err := filepath.Rel(path, current)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("%w: event record purge tree escaped containment", common.ErrConflict)
		}
		rel = filepath.ToSlash(rel)
		switch {
		case info.IsDir():
			_, err = fmt.Fprintf(hash, "d\x00%s\x00", rel)
			return err
		case info.Mode().IsRegular():
			if _, err := fmt.Fprintf(hash, "f\x00%s\x00%d\x00", rel, info.Size()); err != nil {
				return err
			}
			file, err := os.Open(current)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(hash, file)
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
			_, err = hash.Write([]byte{0})
			return err
		default:
			return fmt.Errorf("%w: event record purge tree contains a special file", common.ErrConflict)
		}
	})
	if err != nil {
		return false, "", err
	}
	return true, fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func inspectEventRecordPurgeRegularFile(path string) (bool, string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, "", nil
		}
		return false, "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return false, "", fmt.Errorf("%w: event record purge snapshot is not a regular file", common.ErrConflict)
	}
	file, err := os.Open(path)
	if err != nil {
		return false, "", err
	}
	hash := sha256.New()
	_, copyErr := io.Copy(hash, file)
	closeErr := file.Close()
	if copyErr != nil {
		return false, "", copyErr
	}
	if closeErr != nil {
		return false, "", closeErr
	}
	return true, fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func syncEventRecordPurgeRegularFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("%w: event record purge cannot sync non-regular file", common.ErrConflict)
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	syncErr := file.Sync()
	closeErr := file.Close()
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}

func syncEventRecordPurgeTree(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%w: event record purge cannot sync invalid tree", common.ErrConflict)
	}
	directories := make([]string, 0)
	if err := filepath.WalkDir(path, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		entryInfo, err := entry.Info()
		if err != nil {
			return err
		}
		if entryInfo.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: event record purge tree contains a symlink", common.ErrConflict)
		}
		if entryInfo.IsDir() {
			directories = append(directories, current)
			return nil
		}
		if !entryInfo.Mode().IsRegular() {
			return fmt.Errorf("%w: event record purge tree contains a special file", common.ErrConflict)
		}
		return syncEventRecordPurgeRegularFile(current)
	}); err != nil {
		return err
	}
	sort.Slice(directories, func(i, j int) bool { return len(directories[i]) > len(directories[j]) })
	for _, directory := range directories {
		if err := syncEventRecordPurgeDirectory(directory); err != nil {
			return err
		}
	}
	return nil
}

func syncEventRecordPurgeDirectories(paths ...string) error {
	seen := map[string]struct{}{}
	for _, path := range paths {
		path = filepath.Clean(path)
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		if err := syncEventRecordPurgeDirectory(path); err != nil {
			return err
		}
	}
	return nil
}

func syncEventRecordPurgeDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	info, statErr := directory.Stat()
	if statErr != nil {
		_ = directory.Close()
		return statErr
	}
	if !info.IsDir() {
		_ = directory.Close()
		return common.ErrInvalidArgument
	}
	syncErr := directory.Sync()
	closeErr := directory.Close()
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}

func (r *EventRecordRepository) invokeEventRecordPurgeSwapCut(cut eventRecordPurgeSwapCut) error {
	if r == nil || r.eventRecordPurgeSwapCut == nil {
		return nil
	}
	return r.eventRecordPurgeSwapCut(cut)
}
