package file

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/muidea/workorch/internal/pkg/repository/filecommit"
)

type atomicPersistenceTestRecord struct {
	Generation string `json:"generation"`
	Payload    string `json:"payload"`
}

func TestSaveMapPublishesOnlyCompleteOldOrNewLargeJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.json")
	defer EvictMapFileCache(path)

	oldItems := map[string]atomicPersistenceTestRecord{
		"run-001": {Generation: "old", Payload: strings.Repeat("a", 1<<20)},
	}
	newItems := map[string]atomicPersistenceTestRecord{
		"run-001": {Generation: "new", Payload: strings.Repeat("b", 1<<20)},
	}
	oldData, err := json.Marshal(oldItems)
	if err != nil {
		t.Fatal(err)
	}
	newData, err := json.Marshal(newItems)
	if err != nil {
		t.Fatal(err)
	}
	if err := saveMap(path, oldItems); err != nil {
		t.Fatalf("save initial map: %v", err)
	}

	stop := make(chan struct{})
	started := make(chan struct{})
	errs := make(chan error, 1)
	var reads atomic.Uint64
	var readers sync.WaitGroup
	readers.Add(1)
	go func() {
		defer readers.Done()
		close(started)
		for {
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				select {
				case errs <- fmt.Errorf("read canonical map: %w", readErr):
				default:
				}
				return
			}
			reads.Add(1)
			if !bytes.Equal(data, oldData) && !bytes.Equal(data, newData) {
				select {
				case errs <- fmt.Errorf("canonical map contained a partial or mixed generation: bytes=%d", len(data)):
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
	<-started

	for index := 0; index < 12; index++ {
		items := newItems
		if index%2 != 0 {
			items = oldItems
		}
		if err := saveMap(path, items); err != nil {
			close(stop)
			readers.Wait()
			t.Fatalf("save generation %d: %v", index, err)
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
		t.Fatal("concurrent reader did not observe the canonical map")
	}
}

func TestSaveMapPrePublicationFailurePreservesCanonicalAndCache(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	defer EvictMapFileCache(path)
	oldItems := map[string]string{"generation": "old"}
	newItems := map[string]string{"generation": "new"}
	if err := saveMap(path, oldItems); err != nil {
		t.Fatalf("save initial map: %v", err)
	}
	oldData, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	wantErr := errors.New("stop before rename")
	err = saveMapWithCommit(path, newItems, func(string, []byte, os.FileMode) (filecommit.Outcome, error) {
		return filecommit.Outcome{}, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("save error = %v, want pre-publication failure", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, oldData) {
		t.Fatalf("canonical data changed before publication: got %q want %q", after, oldData)
	}
	cached, err := loadMap[string, string](path)
	if err != nil {
		t.Fatalf("load cached map: %v", err)
	}
	if !reflect.DeepEqual(cached, oldItems) {
		t.Fatalf("cached map = %#v, want old %#v", cached, oldItems)
	}
}

func TestSaveMapPostPublicationSyncFailureInvalidatesOldCache(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	defer EvictMapFileCache(path)
	oldItems := map[string]string{"generation": "old"}
	newItems := map[string]string{"generation": "new"}
	if err := saveMap(path, oldItems); err != nil {
		t.Fatalf("save initial map: %v", err)
	}

	wantErr := errors.New("parent directory sync failed")
	err := saveMapWithCommit(path, newItems, func(path string, data []byte, perm os.FileMode) (filecommit.Outcome, error) {
		if err := os.WriteFile(path, data, perm); err != nil {
			return filecommit.Outcome{}, err
		}
		return filecommit.Outcome{Published: true}, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("save error = %v, want post-publication sync failure", err)
	}

	cache := mapCacheEntry(path)
	cache.mu.RLock()
	loaded, cachedItems := cache.loaded, cache.items
	cache.mu.RUnlock()
	if loaded || cachedItems != nil {
		t.Fatalf("cache after uncertain durable commit = loaded:%v items:%#v, want invalidated", loaded, cachedItems)
	}

	canonical, err := loadMap[string, string](path)
	if err != nil {
		t.Fatalf("reload published canonical map: %v", err)
	}
	if !reflect.DeepEqual(canonical, newItems) {
		t.Fatalf("canonical map = %#v, want published %#v", canonical, newItems)
	}
}

func TestSaveMapHoldsCacheFenceThroughParentDirectorySync(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	defer EvictMapFileCache(path)
	oldItems := map[string]string{"generation": "old"}
	newItems := map[string]string{"generation": "new"}
	if err := saveMap(path, oldItems); err != nil {
		t.Fatalf("save initial map: %v", err)
	}

	syncEntered := make(chan struct{})
	releaseSync := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseSync) }) }
	defer release()
	saveDone := make(chan error, 1)
	go func() {
		saveDone <- saveMapWithCommit(path, newItems, func(path string, data []byte, perm os.FileMode) (filecommit.Outcome, error) {
			if err := os.WriteFile(path, data, perm); err != nil {
				return filecommit.Outcome{}, err
			}
			close(syncEntered)
			<-releaseSync
			return filecommit.Outcome{Published: true, DurabilityAcknowledged: true}, nil
		})
	}()
	select {
	case <-syncEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("save did not reach parent-directory sync")
	}

	cache := mapCacheEntry(path)
	if cache.mu.TryRLock() {
		cache.mu.RUnlock()
		t.Fatal("cache fence was released before durable commit completed")
	}
	loadDone := make(chan map[string]string, 1)
	loadErr := make(chan error, 1)
	go func() {
		items, err := loadMap[string, string](path)
		if err != nil {
			loadErr <- err
			return
		}
		loadDone <- items
	}()
	select {
	case items := <-loadDone:
		t.Fatalf("load returned before durable commit completed: %#v", items)
	case err := <-loadErr:
		t.Fatalf("load failed before durable commit completed: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	release()
	select {
	case err := <-saveDone:
		if err != nil {
			t.Fatalf("save map: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("save did not finish")
	}
	select {
	case items := <-loadDone:
		if !reflect.DeepEqual(items, newItems) {
			t.Fatalf("load after durable commit = %#v, want %#v", items, newItems)
		}
	case err := <-loadErr:
		t.Fatalf("load after durable commit: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("load did not finish")
	}
}

func TestLoadMapTreatsOnlyCanonicalPathAsAuthority(t *testing.T) {
	directory := t.TempDir()
	validItems := map[string]string{"generation": "temp-only"}
	validData, err := json.Marshal(validItems)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("temp without canonical is ignored", func(t *testing.T) {
		path := filepath.Join(directory, "missing.json")
		defer EvictMapFileCache(path)
		temp, err := os.CreateTemp(directory, ".missing.json.tmp-*")
		if err != nil {
			t.Fatal(err)
		}
		tempPath := temp.Name()
		if _, err := temp.Write(validData); err != nil {
			_ = temp.Close()
			t.Fatal(err)
		}
		if err := temp.Close(); err != nil {
			t.Fatal(err)
		}

		items, err := loadMap[string, string](path)
		if err != nil {
			t.Fatalf("load missing canonical map: %v", err)
		}
		if len(items) != 0 {
			t.Fatalf("map loaded from uncommitted temp: %#v", items)
		}
		if _, err := os.Stat(tempPath); err != nil {
			t.Fatalf("ignored temp should not be treated as recovery authority or mutated: %v", err)
		}
	})

	t.Run("corrupt canonical fails closed despite valid temp", func(t *testing.T) {
		path := filepath.Join(directory, "corrupt.json")
		defer EvictMapFileCache(path)
		if err := os.WriteFile(path, []byte("{not-json"), 0o644); err != nil {
			t.Fatal(err)
		}
		temp, err := os.CreateTemp(directory, ".corrupt.json.tmp-*")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := temp.Write(validData); err != nil {
			_ = temp.Close()
			t.Fatal(err)
		}
		if err := temp.Close(); err != nil {
			t.Fatal(err)
		}

		if _, err := loadMap[string, string](path); err == nil {
			t.Fatal("load corrupt canonical error = nil, want fail closed")
		}
	})

	t.Run("existing complete canonical remains compatible", func(t *testing.T) {
		path := filepath.Join(directory, "complete.json")
		defer EvictMapFileCache(path)
		if err := os.WriteFile(path, validData, 0o644); err != nil {
			t.Fatal(err)
		}
		items, err := loadMap[string, string](path)
		if err != nil {
			t.Fatalf("load existing complete canonical: %v", err)
		}
		if !reflect.DeepEqual(items, validItems) {
			t.Fatalf("existing canonical map = %#v, want %#v", items, validItems)
		}
	})
}
