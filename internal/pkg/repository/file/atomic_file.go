package file

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/muidea/workorch/internal/pkg/repository/filecommit"
)

type atomicFileCommitFunc func(string, []byte, os.FileMode) (filecommit.Outcome, error)

func saveMapWithCommit[K comparable, V any](path string, items map[K]V, commit atomicFileCommitFunc) error {
	return saveMapWithPermissions(path, items, 0o644, commit)
}

func saveMapWithPermissions[K comparable, V any](path string, items map[K]V, perm os.FileMode, commit atomicFileCommitFunc) error {
	if perm != 0o644 && perm != 0o600 {
		return fmt.Errorf("file repository permissions invalid")
	}
	data, err := json.Marshal(items)
	if err != nil {
		return err
	}
	if commit == nil {
		return fmt.Errorf("file repository commit is nil")
	}

	// Keep the decoded cache behind the same publication fence. This prevents
	// another repository instance in this process from returning the old value
	// after rename but before the cache is advanced.
	cache := mapCacheEntry(path)
	cache.mu.Lock()
	defer cache.mu.Unlock()

	// 已收紧的文件不得因后续不再包含诊断而重新变为公开可读。
	if info, statErr := os.Stat(path); statErr == nil && info.Mode().Perm() == 0o600 {
		perm = 0o600
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return statErr
	}
	outcome, err := commit(path, data, perm)
	if err != nil {
		if outcome.Published {
			// rename already changed the canonical path. A following directory
			// fsync failure makes durability uncertain, so force the next read
			// through the canonical file instead of retaining the old cache.
			cache.items = nil
			cache.loaded = false
		}
		return err
	}
	cache.items = cloneStoredMap(items)
	cache.loaded = true
	return nil
}

func commitFileAtomically(path string, data []byte, perm os.FileMode) (filecommit.Outcome, error) {
	return filecommit.Publish(path, data, perm)
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	_, err := filecommit.Publish(path, data, perm)
	return err
}
