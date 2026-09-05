package filecommit

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestPublishReplacesCanonicalWithDurableCompleteFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "state.json")
	want := []byte(`{"generation":"new"}`)

	outcome, err := Publish(path, want, 0o600)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if !outcome.Published || !outcome.DurabilityAcknowledged || outcome.PublishedButDurabilityUnacknowledged() {
		t.Fatalf("outcome = %#v, want published and durability acknowledged", outcome)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("canonical bytes = %q, want %q", got, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("canonical mode = %o, want 600", info.Mode().Perm())
	}
	assertNoPublicationTemps(t, path)
}

func TestPublishSynchronizesNewDirectoryChainBeforeCanonicalRename(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first")
	second := filepath.Join(first, "second")
	path := filepath.Join(second, "state.json")
	synchronized := make([]string, 0, 3)

	outcome, err := publishWithHooks(path, []byte("state"), 0o644, publishHooks{
		syncDirectory: func(directory string) error {
			synchronized = append(synchronized, filepath.Clean(directory))
			return syncDirectory(directory)
		},
		beforeRename: func(_, _ string) error {
			want := []string{filepath.Clean(root), filepath.Clean(first)}
			if !equalPaths(synchronized, want) {
				t.Fatalf("directories synchronized before rename = %v, want %v", synchronized, want)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if !outcome.Published || !outcome.DurabilityAcknowledged {
		t.Fatalf("outcome = %#v, want durable publication", outcome)
	}
	want := []string{filepath.Clean(root), filepath.Clean(first), filepath.Clean(second)}
	if !equalPaths(synchronized, want) {
		t.Fatalf("directory synchronization order = %v, want %v", synchronized, want)
	}
}

func TestPublishDirectoryCreationSyncFailurePreventsFilePublication(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "new", "state.json")
	wantErr := errors.New("directory entry sync failed")

	outcome, err := publishWithHooks(path, []byte("state"), 0o644, publishHooks{
		syncDirectory: func(directory string) error {
			if filepath.Clean(directory) != filepath.Clean(root) {
				t.Fatalf("first synchronized directory = %q, want %q", directory, root)
			}
			return wantErr
		},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("publish error = %v, want %v", err, wantErr)
	}
	if errors.Is(err, ErrPublishedButDurabilityUnacknowledged) {
		t.Fatalf("directory-creation error was marked as file publication: %v", err)
	}
	if outcome.Published || outcome.DurabilityAcknowledged {
		t.Fatalf("outcome = %#v, want no file publication", outcome)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("canonical path exists after directory sync failure: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatalf("unacknowledged directory remains after sync failure: %v", err)
	}

	synchronized := make([]string, 0, 2)
	outcome, err = publishWithHooks(path, []byte("state"), 0o644, publishHooks{
		syncDirectory: func(directory string) error {
			synchronized = append(synchronized, filepath.Clean(directory))
			return syncDirectory(directory)
		},
	})
	if err != nil {
		t.Fatalf("retry publish: %v", err)
	}
	if !outcome.Published || !outcome.DurabilityAcknowledged {
		t.Fatalf("retry outcome = %#v, want durable publication", outcome)
	}
	wantSynchronized := []string{filepath.Clean(root), filepath.Clean(filepath.Dir(path))}
	if !equalPaths(synchronized, wantSynchronized) {
		t.Fatalf("retry directory synchronization = %v, want %v", synchronized, wantSynchronized)
	}
}

func TestPublishPreRenameFailurePreservesCanonicalAndRemovesTemp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	oldData := []byte(`{"generation":"old"}`)
	newData := []byte(`{"generation":"new"}`)
	if err := os.WriteFile(path, oldData, 0o644); err != nil {
		t.Fatal(err)
	}

	wantErr := errors.New("stop before rename")
	var observedTemp string
	outcome, err := publishWithHooks(path, newData, 0o640, publishHooks{
		beforeRename: func(tempPath, targetPath string) error {
			observedTemp = tempPath
			if targetPath != path {
				t.Fatalf("target path = %q, want %q", targetPath, path)
			}
			if filepath.Dir(tempPath) != filepath.Dir(path) {
				t.Fatalf("temporary directory = %q, want %q", filepath.Dir(tempPath), filepath.Dir(path))
			}
			got, readErr := os.ReadFile(tempPath)
			if readErr != nil {
				t.Fatalf("read temporary file before rename: %v", readErr)
			}
			if !bytes.Equal(got, newData) {
				t.Fatalf("temporary bytes = %q, want %q", got, newData)
			}
			info, statErr := os.Stat(tempPath)
			if statErr != nil {
				t.Fatalf("stat temporary file before rename: %v", statErr)
			}
			if info.Mode().Perm() != 0o640 {
				t.Fatalf("temporary mode = %o, want 640", info.Mode().Perm())
			}
			return wantErr
		},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("publish error = %v, want %v", err, wantErr)
	}
	if errors.Is(err, ErrPublishedButDurabilityUnacknowledged) {
		t.Fatalf("pre-publication error was marked as published: %v", err)
	}
	if outcome.Published || outcome.DurabilityAcknowledged || outcome.PublishedButDurabilityUnacknowledged() {
		t.Fatalf("outcome = %#v, want pre-publication failure", outcome)
	}
	if observedTemp == "" {
		t.Fatal("before-rename hook did not observe the temporary file")
	}
	if _, err := os.Stat(observedTemp); !os.IsNotExist(err) {
		t.Fatalf("temporary file remains after failure: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, oldData) {
		t.Fatalf("canonical bytes changed before rename: got %q want %q", got, oldData)
	}
}

func TestPublishReportsPublishedButDurabilityUnacknowledged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	want := []byte(`{"generation":"new"}`)
	wantErr := errors.New("parent directory sync failed")

	outcome, err := publishWithHooks(path, want, 0o644, publishHooks{
		syncDirectory: func(directory string) error {
			if directory != filepath.Dir(path) {
				t.Fatalf("sync directory = %q, want %q", directory, filepath.Dir(path))
			}
			got, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatalf("canonical path not published before parent sync: %v", readErr)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("canonical bytes at parent sync = %q, want %q", got, want)
			}
			return wantErr
		},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("publish error = %v, want %v", err, wantErr)
	}
	if !errors.Is(err, ErrPublishedButDurabilityUnacknowledged) {
		t.Fatalf("publish error = %v, want published-but-durability-unacknowledged marker", err)
	}
	if !outcome.Published || outcome.DurabilityAcknowledged || !outcome.PublishedButDurabilityUnacknowledged() {
		t.Fatalf("outcome = %#v, want published with durability unacknowledged", outcome)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("published canonical bytes = %q, want %q", got, want)
	}
	assertNoPublicationTemps(t, path)
}

func TestPublishConcurrentReadersObserveOnlyCompleteGenerations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	oldData := []byte(`{"generation":"old","payload":"` + strings.Repeat("a", 1<<20) + `"}`)
	newData := []byte(`{"generation":"new","payload":"` + strings.Repeat("b", 1<<20) + `"}`)
	if _, err := Publish(path, oldData, 0o644); err != nil {
		t.Fatalf("publish initial generation: %v", err)
	}

	stop := make(chan struct{})
	errs := make(chan error, 1)
	var reads atomic.Uint64
	var readers sync.WaitGroup
	readers.Add(1)
	go func() {
		defer readers.Done()
		for {
			data, err := os.ReadFile(path)
			if err != nil {
				select {
				case errs <- fmt.Errorf("read canonical file: %w", err):
				default:
				}
				return
			}
			reads.Add(1)
			if !bytes.Equal(data, oldData) && !bytes.Equal(data, newData) {
				select {
				case errs <- fmt.Errorf("observed partial or mixed generation: bytes=%d", len(data)):
				default:
				}
				return
			}
			select {
			case <-stop:
				return
			default:
			}
		}
	}()

	for index := 0; index < 12; index++ {
		data := newData
		if index%2 != 0 {
			data = oldData
		}
		if _, err := Publish(path, data, 0o644); err != nil {
			close(stop)
			readers.Wait()
			t.Fatalf("publish generation %d: %v", index, err)
		}
	}
	close(stop)
	readers.Wait()
	select {
	case err := <-errs:
		t.Fatal(err)
	default:
	}
	if reads.Load() == 0 {
		t.Fatal("concurrent reader did not observe the canonical file")
	}
	assertNoPublicationTemps(t, path)
}

func assertNoPublicationTemps(t *testing.T, path string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("publication temporary files remain: %v", matches)
	}
}

func equalPaths(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}
