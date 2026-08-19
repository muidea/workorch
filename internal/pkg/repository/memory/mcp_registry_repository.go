package memory

import (
	"context"
	"sync"

	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type MCPRegistryRepository struct {
	mu       sync.RWMutex
	snapshot *mcpmodel.RegistrySnapshot
}

func NewMCPRegistryRepository() *MCPRegistryRepository {
	return &MCPRegistryRepository{snapshot: &mcpmodel.RegistrySnapshot{}}
}

func (r *MCPRegistryRepository) Load(context.Context) (*mcpmodel.RegistrySnapshot, error) {
	if r == nil {
		return nil, common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return mcpmodel.CloneRegistrySnapshot(r.snapshot), nil
}

func (r *MCPRegistryRepository) Commit(_ context.Context, expectedRevision uint64, next *mcpmodel.RegistrySnapshot) error {
	if r == nil || next == nil || next.Revision != expectedRevision+1 {
		return common.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.snapshot == nil {
		r.snapshot = &mcpmodel.RegistrySnapshot{}
	}
	if r.snapshot.Revision != expectedRevision {
		return common.ErrConflict
	}
	r.snapshot = mcpmodel.CloneRegistrySnapshot(next)
	return nil
}
