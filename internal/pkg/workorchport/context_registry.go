package workorchport

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
)

type SoulInfo struct {
	ID              common.SoulID
	Name            string
	Description     string
	BusinessDomains []string
	SoulMD          string
}

type SoulBackend interface {
	Create(ctx context.Context, s *SoulInfo) (*SoulInfo, error)
	Get(ctx context.Context, id common.SoulID) (*SoulInfo, error)
	List(ctx context.Context) ([]SoulInfo, error)
	Update(ctx context.Context, id common.SoulID, s *SoulInfo) (*SoulInfo, error)
	Delete(ctx context.Context, id common.SoulID) error
}

type SkillInfo struct {
	ID          common.SkillID
	Name        string
	Version     string
	Description string
	Instruction string
}

type SkillBackend interface {
	Create(ctx context.Context, s *SkillInfo) (*SkillInfo, error)
	Get(ctx context.Context, id common.SkillID) (*SkillInfo, error)
	List(ctx context.Context) ([]SkillInfo, error)
}

type MCPServerInfo struct {
	ID        common.MCPServerID
	Name      string
	Transport string
	Endpoint  string
	Enabled   bool
}

type MCPCapabilityInfo struct {
	ID          common.MCPCapabilityID
	ServerID    common.MCPServerID
	Kind        string
	Name        string
	DisplayName string
	Description string
}

type MCPBackend interface {
	CreateServer(ctx context.Context, s *MCPServerInfo) (*MCPServerInfo, error)
	GetServer(ctx context.Context, id common.MCPServerID) (*MCPServerInfo, error)
	ListServers(ctx context.Context) ([]MCPServerInfo, error)
	UpdateServer(ctx context.Context, id common.MCPServerID, s *MCPServerInfo) (*MCPServerInfo, error)
	ListCapabilities(ctx context.Context) ([]MCPCapabilityInfo, error)
}

type MemoryItemInfo struct {
	ID                  common.MemoryID
	WorkspaceID         common.WorkspaceID
	RunID               common.RunID
	SourceType          string
	SourceRef           string
	MemoryType          string
	Content             string
	Summary             string
	Tags                []string
	Importance          int
	Confidence          int
	Visibility          string
	EmbeddingProviderID string
	EmbeddingModel      string
	EmbeddingText       string
	VectorReady         bool
	VectorDimension     int
	VectorScore         float64
	Vector              []float64
}

type MemorySearchRequest struct {
	Query string
	Tags  []string
	Types []string
	Limit int
}

type MemoryBackend interface {
	Create(ctx context.Context, wsID common.WorkspaceID, m *MemoryItemInfo) (*MemoryItemInfo, error)
	Get(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID) (*MemoryItemInfo, error)
	Update(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID, m *MemoryItemInfo) (*MemoryItemInfo, error)
	Delete(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID) error
	List(ctx context.Context, wsID common.WorkspaceID) ([]MemoryItemInfo, error)
	Search(ctx context.Context, wsID common.WorkspaceID, req MemorySearchRequest) ([]MemoryItemInfo, error)
}
