package repository

import (
	"context"

	mcp "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
)

// MCPRegistryRepository persists the complete Registry aggregate behind one
// CAS fence. Server and Capability state must never be committed separately
// on the production owner path.
type MCPRegistryRepository interface {
	Load(context.Context) (*mcp.RegistrySnapshot, error)
	Commit(context.Context, uint64, *mcp.RegistrySnapshot) error
}
