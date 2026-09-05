// Package filecommit publishes one canonical file without exposing a
// partially-written generation to concurrent readers.
//
// It is an owner-neutral filesystem primitive. It does not provide a
// multi-file transaction, recovery policy, repository cache, or business
// state machine; callers retain those responsibilities.
package filecommit

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ErrPublishedButDurabilityUnacknowledged marks a failure after rename made
// the new canonical file visible but before the parent-directory sync was
// acknowledged. Retrying callers must first reconcile the canonical path.
var ErrPublishedButDurabilityUnacknowledged = errors.New("file published but durability unacknowledged")

// Outcome identifies which side of the canonical-path publication boundary a
// commit reached. Published may be true when Publish returns an error: rename
// has made the new bytes visible, but syncing the parent directory failed, so
// crash durability was not acknowledged. A caller with a read cache must not
// retain its pre-publication value in that case.
type Outcome struct {
	Published              bool
	DurabilityAcknowledged bool
}

// PublishedButDurabilityUnacknowledged reports that the canonical path was
// replaced even though Publish could not confirm the rename's crash durability.
func (o Outcome) PublishedButDurabilityUnacknowledged() bool {
	return o.Published && !o.DurabilityAcknowledged
}

// Publish crash-safely replaces path with data and perm. It writes a temporary
// file in the target directory, applies its mode, syncs and closes it, renames
// it over the canonical path, and finally syncs the parent directory.
//
// Only the canonical path is authoritative. Temporary files are removed after
// failures before rename and are never promoted as recovery candidates.
func Publish(path string, data []byte, perm os.FileMode) (Outcome, error) {
	return publishWithHooks(path, data, perm, publishHooks{})
}

// publishHooks are package-private test seams around the publication boundary.
// Production commits always use the zero value.
type publishHooks struct {
	beforeRename  func(tempPath, targetPath string) error
	syncDirectory func(directory string) error
}

func publishWithHooks(path string, data []byte, perm os.FileMode, hooks publishHooks) (Outcome, error) {
	directory := filepath.Dir(path)
	syncDir := syncDirectory
	if hooks.syncDirectory != nil {
		syncDir = hooks.syncDirectory
	}
	if err := ensureDirectory(directory, syncDir); err != nil {
		return Outcome{}, err
	}

	temporary, err := os.CreateTemp(directory, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return Outcome{}, err
	}
	temporaryPath := temporary.Name()
	published := false
	defer func() {
		_ = temporary.Close()
		if !published {
			_ = os.Remove(temporaryPath)
		}
	}()

	written, err := temporary.Write(data)
	if err != nil {
		return Outcome{}, err
	}
	if written != len(data) {
		return Outcome{}, io.ErrShortWrite
	}
	if err := temporary.Chmod(perm); err != nil {
		return Outcome{}, err
	}
	if err := temporary.Sync(); err != nil {
		return Outcome{}, err
	}
	if err := temporary.Close(); err != nil {
		return Outcome{}, err
	}
	if hooks.beforeRename != nil {
		if err := hooks.beforeRename(temporaryPath, path); err != nil {
			return Outcome{}, err
		}
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return Outcome{}, err
	}
	published = true
	outcome := Outcome{Published: true}

	if err := syncDir(directory); err != nil {
		return outcome, fmt.Errorf("%w at %s: sync parent directory: %w", ErrPublishedButDurabilityUnacknowledged, path, err)
	}
	outcome.DurabilityAcknowledged = true
	return outcome, nil
}

// ensureDirectory creates a missing target-directory chain from the nearest
// existing ancestor downward. Each mkdir is followed immediately by an fsync
// of its parent, so publication never proceeds while a newly-created path
// component is only present in an unsynchronized directory entry.
func ensureDirectory(path string, syncDir func(string) error) error {
	cleanPath := filepath.Clean(path)
	missing := make([]string, 0, 4)
	current := cleanPath
	for {
		info, err := os.Stat(current)
		if err == nil {
			if !info.IsDir() {
				return fmt.Errorf("file commit parent path is not a directory: %s", current)
			}
			break
		}
		if !os.IsNotExist(err) {
			return err
		}
		missing = append(missing, current)
		parent := filepath.Dir(current)
		if parent == current {
			return fmt.Errorf("file commit directory has no existing ancestor: %s", cleanPath)
		}
		current = parent
	}

	for index := len(missing) - 1; index >= 0; index-- {
		directory := missing[index]
		mkdirErr := os.Mkdir(directory, 0o755)
		created := mkdirErr == nil
		if mkdirErr != nil && !os.IsExist(mkdirErr) {
			return mkdirErr
		}
		info, err := os.Stat(directory)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return fmt.Errorf("file commit parent path is not a directory: %s", directory)
		}
		parent := filepath.Dir(directory)
		if err := syncDir(parent); err != nil {
			// The directory entry was not durably acknowledged. Remove the
			// still-empty directory so a same-process retry cannot mistake it
			// for a previously durable path and skip this synchronization.
			if created {
				if removeErr := os.Remove(directory); removeErr != nil {
					return fmt.Errorf("sync parent directory %s after creating %s: %w (rollback: %v)", parent, directory, err, removeErr)
				}
			}
			return fmt.Errorf("sync parent directory %s after creating %s: %w", parent, directory, err)
		}
	}
	return nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	syncErr := directory.Sync()
	closeErr := directory.Close()
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}
