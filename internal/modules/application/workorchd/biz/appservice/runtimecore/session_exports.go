package runtimecore

import (
	"fmt"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func (s *Session) Exports() (bootstrap.Exports, error) {
	if s == nil {
		return bootstrap.Exports{}, common.ErrInvalidArgument
	}
	if s.Bootstrap == nil {
		if !HasRuntimeExports(s.exports) {
			return bootstrap.Exports{}, common.ErrInvalidArgument
		}
		return s.exports, nil
	}
	return s.Bootstrap.Exports(), nil
}

func (s *Session) RequiredEventHub() (events.Hub, error) {
	exports, err := s.Exports()
	if err != nil {
		return nil, err
	}
	if exports.Infrastructure.EventHub == nil {
		return nil, common.ErrInvalidArgument
	}
	return exports.Infrastructure.EventHub, nil
}

func (s *Session) ConfigLoader() func() (workorchconfig.Config, error) {
	exports, err := s.Exports()
	if err != nil {
		return nil
	}
	if exports.Infrastructure.LoadConfig != nil {
		return exports.Infrastructure.LoadConfig
	}
	return func() (workorchconfig.Config, error) {
		return workorchconfig.Config{}, nil
	}
}

func (s *Session) SetRepositoryConfig(backend string, dataRoot string) {
	if s == nil {
		return
	}
	cfg := ResolveRepositoryConfig(backend, dataRoot)
	s.repositoryBackend = cfg.Backend
	s.repositoryDataDir = cfg.DataDir
}

func (s *Session) RepositoryBackend() string {
	if s == nil {
		return ""
	}
	return s.repositoryBackend
}

func (s *Session) RepositoryDataDir() string {
	if s == nil {
		return ""
	}
	return s.repositoryDataDir
}

func HasRuntimeExports(exports bootstrap.Exports) bool {
	return ValidateRuntimeExports(exports) == nil
}

func ValidateRuntimeExports(exports bootstrap.Exports) error {
	repositories := exports.Repositories
	required := []struct {
		name  string
		value any
	}{
		{"Repositories.Souls", repositories.Souls},
		{"Repositories.WorkspaceTemplates", repositories.WorkspaceTemplates},
		{"Repositories.MemoryItems", repositories.MemoryItems},
		{"Repositories.MemoryRecallTraces", repositories.MemoryRecallTraces},
		{"Repositories.AutoGovernanceTraces", repositories.AutoGovernanceTraces},
		{"Repositories.Skills", repositories.Skills},
		{"Repositories.MCPServers", repositories.MCPServers},
		{"Repositories.MCPCapabilities", repositories.MCPCapabilities},
		{"Repositories.Results", repositories.Results},
		{"Repositories.Artifacts", repositories.Artifacts},
		{"Repositories.EventRecords", repositories.EventRecords},
		{"Repositories.Workspaces", repositories.Workspaces},
		{"Repositories.Goals", repositories.Goals},
		{"Repositories.DoDs", repositories.DoDs},
		{"Repositories.Runs", repositories.Runs},
		{"Repositories.Steps", repositories.Steps},
		{"Repositories.ToolCalls", repositories.ToolCalls},
		{"Repositories.Patches", repositories.Patches},
		{"Repositories.ValidationResults", repositories.ValidationResults},
		{"Repositories.RuntimeCheckpoints", repositories.RuntimeCheckpoints},
		{"Repositories.EvidenceRecords", repositories.EvidenceRecords},
		{"Repositories.CoverageWorkItems", repositories.CoverageWorkItems},
		{"Repositories.ArtifactContinuations", repositories.ArtifactContinuations},
		{"Repositories.Tickets", repositories.Tickets},
		{"Repositories.Resolutions", repositories.Resolutions},
		{"Repositories.Approvals", repositories.Approvals},
		{"Infrastructure.EventHub", exports.Infrastructure.EventHub},
	}
	for _, item := range required {
		if item.value == nil {
			return fmt.Errorf("%w: missing runtime export %s", common.ErrInvalidArgument, item.name)
		}
	}
	return nil
}
