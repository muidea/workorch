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
	dataDir string
	mu      sync.Mutex
}

func NewCapabilityStateReceiptStore(dataDir string) (*CapabilityStateReceiptStore, error) {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		return nil, common.ErrInvalidArgument
	}
	return &CapabilityStateReceiptStore{dataDir: filepath.Clean(dataDir)}, nil
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
		return existing, nil
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return runtimecontract.RuntimeCapabilityStateSnapshot{}, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
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
	defer os.Remove(temporaryPath)
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
	err = os.Link(temporaryPath, path)
	if errors.Is(err, os.ErrExist) {
		value, found, loadErr := s.load(workspaceID, snapshot.AcquisitionKey)
		if loadErr != nil {
			return runtimecontract.RuntimeCapabilityStateSnapshot{}, loadErr
		}
		if !found || !reflect.DeepEqual(value, snapshot) {
			return runtimecontract.RuntimeCapabilityStateSnapshot{}, fmt.Errorf("%w: capability-state acquisition key %q already has a different snapshot", common.ErrConflict, snapshot.AcquisitionKey)
		}
		return value, nil
	}
	if err != nil {
		return runtimecontract.RuntimeCapabilityStateSnapshot{}, err
	}
	return snapshot, nil
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
