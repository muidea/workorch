package repository

import (
	"context"

	mcp "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type MCPCapabilityRepository interface {
	Create(context.Context, *mcp.Capability) error
	Get(context.Context, common.MCPCapabilityID) (*mcp.Capability, error)
	List(context.Context) ([]*mcp.Capability, error)
	Update(context.Context, *mcp.Capability) error
}
