package repository

import (
	"context"

	mcp "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type MCPServerRepository interface {
	Create(context.Context, *mcp.Server) error
	Get(context.Context, common.MCPServerID) (*mcp.Server, error)
	List(context.Context) ([]*mcp.Server, error)
	Update(context.Context, *mcp.Server) error
}
