package file

import (
	"context"
	"errors"
	"os"
	"sync"

	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type MCPRegistryRepository struct {
	mu   sync.RWMutex
	path string
}

func NewMCPRegistryRepository(path string) *MCPRegistryRepository {
	return &MCPRegistryRepository{path: path}
}

func (r *MCPRegistryRepository) Load(context.Context) (*mcpmodel.RegistrySnapshot, error) {
	if r == nil || r.path == "" {
		return nil, common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.load()
}

func (r *MCPRegistryRepository) Commit(_ context.Context, expectedRevision uint64, next *mcpmodel.RegistrySnapshot) error {
	if r == nil || r.path == "" || next == nil || next.Revision != expectedRevision+1 {
		return common.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	current, err := r.load()
	if err != nil {
		return err
	}
	if current.Revision != expectedRevision {
		return common.ErrConflict
	}
	return writeJSONFileAtomic(r.path, mcpmodel.CloneRegistrySnapshot(next))
}

func (r *MCPRegistryRepository) load() (*mcpmodel.RegistrySnapshot, error) {
	data, err := os.ReadFile(r.path)
	if errors.Is(err, os.ErrNotExist) {
		return &mcpmodel.RegistrySnapshot{}, nil
	}
	if err != nil {
		return nil, err
	}
	var snapshot mcpmodel.RegistrySnapshot
	if err := decodeCurrentJSON(data, &snapshot); err != nil {
		return nil, err
	}
	return mcpmodel.CloneRegistrySnapshot(&snapshot), nil
}
