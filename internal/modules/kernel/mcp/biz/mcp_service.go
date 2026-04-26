package mcp

import (
	"context"

	mcpmodel "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/models"
	mcpservice "github.com/muidea/workorch/internal/modules/kernel/mcp/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type Service struct {
	servers repository.MCPServerRepository
}

func NewService(servers repository.MCPServerRepository) *Service {
	return &Service{servers: servers}
}

func (s *Service) CreateServer(ctx context.Context, cmd mcpservice.CreateServerCommand) error {
	if err := validateServerCommand(cmd); err != nil {
		return err
	}
	return s.servers.Create(ctx, serverFromCommand(cmd))
}

func (s *Service) UpdateServer(ctx context.Context, cmd mcpservice.UpdateServerCommand) error {
	if err := validateServerCommand(cmd); err != nil {
		return err
	}
	return s.servers.Update(ctx, serverFromCommand(cmd))
}

func (s *Service) GetServer(ctx context.Context, id common.MCPServerID) (*mcpmodel.Server, error) {
	if id == "" {
		return nil, common.ErrInvalidArgument
	}
	return s.servers.Get(ctx, id)
}

func (s *Service) ListServers(ctx context.Context) ([]*mcpmodel.Server, error) {
	return s.servers.List(ctx)
}

func validateServerCommand(cmd mcpservice.CreateServerCommand) error {
	if cmd.ServerID == "" || cmd.Name == "" || cmd.Transport == "" {
		return common.ErrInvalidArgument
	}
	if cmd.TimeoutMillis < 0 {
		return common.ErrInvalidArgument
	}
	if hasBlank(cmd.AllowedTools) || hasBlank(cmd.AllowedResources) {
		return common.ErrInvalidArgument
	}
	return nil
}

func serverFromCommand(cmd mcpservice.CreateServerCommand) *mcpmodel.Server {
	return &mcpmodel.Server{
		ID:                    cmd.ServerID,
		Name:                  cmd.Name,
		Transport:             cmd.Transport,
		Endpoint:              cmd.Endpoint,
		Enabled:               cmd.Enabled,
		AllowedTools:          cloneStrings(cmd.AllowedTools),
		AllowedResources:      cloneStrings(cmd.AllowedResources),
		DefaultApprovalPolicy: cmd.DefaultApprovalPolicy,
		TimeoutMillis:         cmd.TimeoutMillis,
	}
}

func hasBlank(values []string) bool {
	for _, value := range values {
		if value == "" {
			return true
		}
	}
	return false
}

func cloneStrings(input []string) []string {
	if input == nil {
		return nil
	}
	return append([]string(nil), input...)
}
