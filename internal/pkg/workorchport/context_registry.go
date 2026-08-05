package workorchport

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

type SoulBackend interface {
	Create(ctx context.Context, s *workorchmanagementcontract.SoulMutation) (*workorchmanagementcontract.Soul, error)
	Get(ctx context.Context, id common.SoulID) (*workorchmanagementcontract.Soul, error)
	List(ctx context.Context) ([]workorchmanagementcontract.Soul, error)
	Update(ctx context.Context, id common.SoulID, s *workorchmanagementcontract.SoulMutation) (*workorchmanagementcontract.Soul, error)
	Delete(ctx context.Context, id common.SoulID) error
}

type SkillBackend interface {
	Create(ctx context.Context, s *workorchmanagementcontract.SkillMutation) (*workorchmanagementcontract.Skill, error)
	Get(ctx context.Context, id common.SkillID) (*workorchmanagementcontract.Skill, error)
	List(ctx context.Context) ([]workorchmanagementcontract.Skill, error)
}

type MCPBackend interface {
	CreateServer(ctx context.Context, s *workorchmanagementcontract.MCPServerMutation) (*workorchmanagementcontract.MCPServer, error)
	GetServer(ctx context.Context, id common.MCPServerID) (*workorchmanagementcontract.MCPServer, error)
	ListServers(ctx context.Context) ([]workorchmanagementcontract.MCPServer, error)
	UpdateServer(ctx context.Context, id common.MCPServerID, s *workorchmanagementcontract.MCPServerMutation) (*workorchmanagementcontract.MCPServer, error)
	ListCapabilities(ctx context.Context) ([]workorchmanagementcontract.MCPCapability, error)
}

type MemoryBackend interface {
	Create(ctx context.Context, wsID common.WorkspaceID, m *workorchmanagementcontract.MemoryMutation) (*workorchmanagementcontract.MemoryItem, error)
	Get(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID) (*workorchmanagementcontract.MemoryItem, error)
	Update(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID, m *workorchmanagementcontract.MemoryMutation) (*workorchmanagementcontract.MemoryItem, error)
	Delete(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID) error
	List(ctx context.Context, wsID common.WorkspaceID) ([]workorchmanagementcontract.MemoryItem, error)
	Search(ctx context.Context, wsID common.WorkspaceID, req workorchmanagementcontract.MemorySearchQuery) ([]workorchmanagementcontract.MemoryItem, error)
}
