package git

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func decodeCurrentCapabilityStateReceipt(data []byte, target *runtimecontract.RuntimeCapabilityStateSnapshot) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return common.ErrInvalidArgument
		}
		return err
	}
	return nil
}

// CapabilityStateReceiptStore is Git-owner durable state for read-only
// prerequisite acquisition. It lives in the workspace internal directory, not
// the deliverable, so a query cannot alter user-managed files.
type CapabilityStateReceiptStore struct {
	dataDir       string
	syncDirectory func(string) error
	linkFile      func(string, string) error
	removeFile    func(string) error
	mu            sync.Mutex
}

func NewCapabilityStateReceiptStore(dataDir string) (*CapabilityStateReceiptStore, error) {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		return nil, common.ErrInvalidArgument
	}
	return &CapabilityStateReceiptStore{
		dataDir: filepath.Clean(dataDir), syncDirectory: syncCapabilityStateReceiptDirectory,
		linkFile: os.Link, removeFile: os.Remove,
	}, nil
}

func (s *CapabilityStateReceiptStore) Load(workspaceID common.WorkspaceID, acquisitionKey string) (runtimecontract.RuntimeCapabilityStateSnapshot, bool, error) {
	if s == nil {
		return runtimecontract.RuntimeCapabilityStateSnapshot{}, false, common.ErrInvalidArgument
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load(workspaceID, acquisitionKey)
}

func (s *CapabilityStateReceiptStore) StoreIfAbsent(workspaceID common.WorkspaceID, snapshot runtimecontract.RuntimeCapabilityStateSnapshot) (runtimecontract.RuntimeCapabilityStateSnapshot, error) {
	if s == nil {
		return runtimecontract.RuntimeCapabilityStateSnapshot{}, common.ErrInvalidArgument
	}
	normalized := runtimecontract.NormalizeRuntimeCapabilityStateSnapshots([]runtimecontract.RuntimeCapabilityStateSnapshot{snapshot})
	if len(normalized) != 1 {
		return runtimecontract.RuntimeCapabilityStateSnapshot{}, common.ErrInvalidArgument
	}
	snapshot = normalized[0]
	if snapshot.RequirementID == "" || snapshot.SnapshotID == "" || snapshot.QueryReceiptRef == "" ||
		snapshot.OwnerModuleID == "" || snapshot.CapabilityStateKind == "" || snapshot.SubjectRef == "" || snapshot.ObservedRevision == "" || snapshot.ContentHash == "" {
		return runtimecontract.RuntimeCapabilityStateSnapshot{}, common.ErrInvalidArgument
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	dir, err := s.workspaceDir(workspaceID, snapshot.AcquisitionKey)
	if err != nil {
		return runtimecontract.RuntimeCapabilityStateSnapshot{}, err
	}
	if existing, found, err := s.load(workspaceID, snapshot.AcquisitionKey); err != nil {
		return runtimecontract.RuntimeCapabilityStateSnapshot{}, err
	} else if found {
		if !reflect.DeepEqual(existing, snapshot) {
			return runtimecontract.RuntimeCapabilityStateSnapshot{}, fmt.Errorf("%w: capability-state acquisition key %q already has a different snapshot", common.ErrConflict, snapshot.AcquisitionKey)
		}
		if err := s.syncDirectory(dir); err != nil {
			return runtimecontract.RuntimeCapabilityStateSnapshot{}, err
		}
		return existing, nil
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return runtimecontract.RuntimeCapabilityStateSnapshot{}, err
	}
	if err := s.ensureDurableDirectory(dir); err != nil {
		return runtimecontract.RuntimeCapabilityStateSnapshot{}, err
	}
	path, err := s.path(workspaceID, snapshot.AcquisitionKey)
	if err != nil {
		return runtimecontract.RuntimeCapabilityStateSnapshot{}, err
	}
	file, err := os.CreateTemp(dir, ".capability-state-*")
	if err != nil {
		return runtimecontract.RuntimeCapabilityStateSnapshot{}, err
	}
	temporaryPath := file.Name()
	cleanupTemporary := func() error {
		if temporaryPath == "" {
			return nil
		}
		if err := s.removeFile(temporaryPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		temporaryPath = ""
		// Persist the unlink as part of the same publication transaction. This
		// is deliberately the final leaf-directory sync: a successful call can
		// never acknowledge the final receipt while leaving a crash-recoverable
		// .capability-state-* alias behind.
		return s.syncDirectory(dir)
	}
	defer func() { _ = cleanupTemporary() }()
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return runtimecontract.RuntimeCapabilityStateSnapshot{}, err
	}
	if _, err = file.Write(encoded); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return runtimecontract.RuntimeCapabilityStateSnapshot{}, err
	}
	if closeErr != nil {
		return runtimecontract.RuntimeCapabilityStateSnapshot{}, closeErr
	}
	// Link publishes only when no other process has already installed this
	// acquisition key. Unlike O_EXCL on the final file, readers can never
	// observe a partially written JSON receipt.
	err = s.linkFile(temporaryPath, path)
	if errors.Is(err, os.ErrExist) {
		value, found, loadErr := s.load(workspaceID, snapshot.AcquisitionKey)
		if loadErr != nil {
			return runtimecontract.RuntimeCapabilityStateSnapshot{}, loadErr
		}
		if !found || !reflect.DeepEqual(value, snapshot) {
			return runtimecontract.RuntimeCapabilityStateSnapshot{}, fmt.Errorf("%w: capability-state acquisition key %q already has a different snapshot", common.ErrConflict, snapshot.AcquisitionKey)
		}
		if err := cleanupTemporary(); err != nil {
			return runtimecontract.RuntimeCapabilityStateSnapshot{}, err
		}
		return value, nil
	}
	if err != nil {
		return runtimecontract.RuntimeCapabilityStateSnapshot{}, err
	}
	// The file itself was synced before publication. Remove the temporary hard
	// link first, then fsync the leaf directory once so the final entry and the
	// absence of its temporary alias become one durable acknowledgement fence.
	if err := cleanupTemporary(); err != nil {
		return runtimecontract.RuntimeCapabilityStateSnapshot{}, err
	}
	return snapshot, nil
}

// ensureDurableDirectory creates the workspace receipt directory and fsyncs
// every directory from the leaf back through the configured data root. The
// leaf-to-root order first persists each child inode and then the parent entry
// naming it. Repeating the whole chain is intentional: a retry after any
// prior sync error cannot treat a merely visible MkdirAll result as durable.
func (s *CapabilityStateReceiptStore) ensureDurableDirectory(dir string) error {
	if s == nil || s.syncDirectory == nil {
		return common.ErrInvalidArgument
	}
	chain, err := capabilityStateReceiptDirectoryChain(s.dataDir, dir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	for _, path := range chain {
		if err := s.syncDirectory(path); err != nil {
			return err
		}
	}
	return nil
}

func capabilityStateReceiptDirectoryChain(dataDir, leaf string) ([]string, error) {
	root := filepath.Clean(strings.TrimSpace(dataDir))
	current := filepath.Clean(strings.TrimSpace(leaf))
	if root == "." || current == "." {
		return nil, common.ErrInvalidArgument
	}
	relative, err := filepath.Rel(root, current)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, common.ErrInvalidArgument
	}
	ret := make([]string, 0, 8)
	for {
		ret = append(ret, current)
		if current == root {
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			return nil, common.ErrInvalidArgument
		}
		current = parent
	}
	return ret, nil
}

func syncCapabilityStateReceiptDirectory(path string) error {
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

func (s *CapabilityStateReceiptStore) load(workspaceID common.WorkspaceID, acquisitionKey string) (runtimecontract.RuntimeCapabilityStateSnapshot, bool, error) {
	path, err := s.path(workspaceID, acquisitionKey)
	if err != nil {
		return runtimecontract.RuntimeCapabilityStateSnapshot{}, false, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return runtimecontract.RuntimeCapabilityStateSnapshot{}, false, nil
	}
	if err != nil {
		return runtimecontract.RuntimeCapabilityStateSnapshot{}, false, err
	}
	var snapshot runtimecontract.RuntimeCapabilityStateSnapshot
	if err := decodeCurrentCapabilityStateReceipt(data, &snapshot); err != nil {
		return runtimecontract.RuntimeCapabilityStateSnapshot{}, false, fmt.Errorf("decode capability-state receipt: %w", err)
	}
	normalized := runtimecontract.NormalizeRuntimeCapabilityStateSnapshots([]runtimecontract.RuntimeCapabilityStateSnapshot{snapshot})
	if len(normalized) != 1 {
		return runtimecontract.RuntimeCapabilityStateSnapshot{}, false, fmt.Errorf("invalid capability-state receipt")
	}
	snapshot = normalized[0]
	if snapshot.RequirementID == "" || snapshot.AcquisitionKey != acquisitionKey || snapshot.SnapshotID == "" || snapshot.QueryReceiptRef == "" ||
		snapshot.OwnerModuleID == "" || snapshot.CapabilityStateKind == "" || snapshot.SubjectRef == "" || snapshot.ObservedRevision == "" || snapshot.ContentHash == "" {
		return runtimecontract.RuntimeCapabilityStateSnapshot{}, false, fmt.Errorf("invalid capability-state receipt")
	}
	return snapshot, true, nil
}

func (s *CapabilityStateReceiptStore) path(workspaceID common.WorkspaceID, acquisitionKey string) (string, error) {
	dir, err := s.workspaceDir(workspaceID, acquisitionKey)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(acquisitionKey))
	return filepath.Join(dir, hex.EncodeToString(sum[:])+".json"), nil
}

func (s *CapabilityStateReceiptStore) workspaceDir(workspaceID common.WorkspaceID, acquisitionKey string) (string, error) {
	workspace := strings.TrimSpace(string(workspaceID))
	if s == nil || workspace == "" || workspace != string(workspaceID) || strings.ContainsAny(workspace, `/\\`) || filepath.Base(workspace) != workspace || strings.TrimSpace(acquisitionKey) == "" {
		return "", common.ErrInvalidArgument
	}
	prefix := "capability-state/" + workspace + "/"
	if !strings.HasPrefix(acquisitionKey, prefix) {
		return "", common.ErrInvalidArgument
	}
	return filepath.Join(s.dataDir, "internal", "workspaces", workspace, "runtime", "git-capability-state-receipts"), nil
}
