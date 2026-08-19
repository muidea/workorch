package file

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sort"
	"sync"

	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type MCPRegistryRepository struct {
	mu                   sync.RWMutex
	path                 string
	legacyServerPath     string
	legacyCapabilityPath string
}

func NewMCPRegistryRepository(path string) *MCPRegistryRepository {
	return &MCPRegistryRepository{path: path}
}

func NewMCPRegistryRepositoryWithLegacy(path, serverPath, capabilityPath string) *MCPRegistryRepository {
	return &MCPRegistryRepository{path: path, legacyServerPath: serverPath, legacyCapabilityPath: capabilityPath}
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
		return r.loadLegacy()
	}
	if err != nil {
		return nil, err
	}
	var snapshot mcpmodel.RegistrySnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, err
	}
	return mcpmodel.CloneRegistrySnapshot(&snapshot), nil
}

func (r *MCPRegistryRepository) loadLegacy() (*mcpmodel.RegistrySnapshot, error) {
	snapshot := &mcpmodel.RegistrySnapshot{}
	if r.legacyServerPath != "" {
		items, err := loadMap[common.MCPServerID, mcpmodel.Server](r.legacyServerPath)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			snapshot.Servers = append(snapshot.Servers, cloneMCPServer(&item))
		}
	}
	if r.legacyCapabilityPath != "" {
		items, err := loadMap[common.MCPCapabilityID, mcpmodel.Capability](r.legacyCapabilityPath)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if item.Source == "" {
				item.Source = mcpmodel.CapabilitySourceLegacyImport
			}
			if item.SourceRef == "" {
				item.SourceRef = string(item.ID)
			}
			if item.Revision == 0 {
				item.Revision = 1
			}
			snapshot.Capabilities = append(snapshot.Capabilities, cloneMCPCapability(&item))
		}
	}
	sort.Slice(snapshot.Servers, func(i, j int) bool { return snapshot.Servers[i].ID < snapshot.Servers[j].ID })
	sort.Slice(snapshot.Capabilities, func(i, j int) bool { return snapshot.Capabilities[i].ID < snapshot.Capabilities[j].ID })
	return snapshot, nil
}
