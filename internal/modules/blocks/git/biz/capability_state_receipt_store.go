package git

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// CapabilityStateReceiptStore is Git-owner durable state for read-only
// prerequisite acquisition. It deliberately lives outside the workspace so a
// query cannot alter a deliverable.
type CapabilityStateReceiptStore struct {
	dir string
	mu  sync.Mutex
}

func NewCapabilityStateReceiptStore(dataDir string) (*CapabilityStateReceiptStore, error) {
	dir := strings.TrimSpace(dataDir)
	if dir == "" {
		return nil, common.ErrInvalidArgument
	}
	dir = filepath.Join(dir, "runtime-v2", "git-capability-state-receipts")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &CapabilityStateReceiptStore{dir: dir}, nil
}

func (s *CapabilityStateReceiptStore) Load(acquisitionKey string) (runtimecontract.RuntimeCapabilityStateSnapshot, bool, error) {
	if s == nil || strings.TrimSpace(acquisitionKey) == "" {
		return runtimecontract.RuntimeCapabilityStateSnapshot{}, false, common.ErrInvalidArgument
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load(acquisitionKey)
}

func (s *CapabilityStateReceiptStore) StoreIfAbsent(snapshot runtimecontract.RuntimeCapabilityStateSnapshot) (runtimecontract.RuntimeCapabilityStateSnapshot, error) {
	if s == nil {
		return runtimecontract.RuntimeCapabilityStateSnapshot{}, common.ErrInvalidArgument
	}
	normalized := runtimecontract.NormalizeRuntimeCapabilityStateSnapshots([]runtimecontract.RuntimeCapabilityStateSnapshot{snapshot})
	if len(normalized) != 1 {
		return runtimecontract.RuntimeCapabilityStateSnapshot{}, common.ErrInvalidArgument
	}
	snapshot = normalized[0]
	if snapshot.RequirementID == "" || snapshot.AcquisitionKey == "" || snapshot.SnapshotID == "" || snapshot.QueryReceiptRef == "" ||
		snapshot.OwnerModuleID == "" || snapshot.CapabilityStateKind == "" || snapshot.SubjectRef == "" || snapshot.ObservedRevision == "" || snapshot.ContentHash == "" {
		return runtimecontract.RuntimeCapabilityStateSnapshot{}, common.ErrInvalidArgument
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, found, err := s.load(snapshot.AcquisitionKey); err != nil {
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
	path := s.path(snapshot.AcquisitionKey)
	file, err := os.CreateTemp(s.dir, ".capability-state-*")
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
		value, found, loadErr := s.load(snapshot.AcquisitionKey)
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

func (s *CapabilityStateReceiptStore) load(acquisitionKey string) (runtimecontract.RuntimeCapabilityStateSnapshot, bool, error) {
	data, err := os.ReadFile(s.path(acquisitionKey))
	if errors.Is(err, os.ErrNotExist) {
		return runtimecontract.RuntimeCapabilityStateSnapshot{}, false, nil
	}
	if err != nil {
		return runtimecontract.RuntimeCapabilityStateSnapshot{}, false, err
	}
	var snapshot runtimecontract.RuntimeCapabilityStateSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
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

func (s *CapabilityStateReceiptStore) path(acquisitionKey string) string {
	sum := sha256.Sum256([]byte(acquisitionKey))
	return filepath.Join(s.dir, hex.EncodeToString(sum[:])+".json")
}
