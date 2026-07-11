package activationports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

type WorkspaceMCPSelectionRequest struct {
	Hub                    magicevent.Hub
	Workspace              *workspacemodel.Workspace
	RequestedMCPServerIDs  []string
	DefaultMCPServerIDs    []string
	RequestedCapabilityIDs []string
	DefaultCapabilityIDs   []string
	Skills                 []contextregistrycore.Skill
}

type WorkspaceMCPSelectionResult struct {
	Selection     activationcore.MCPSelectionContext
	MCPServerIDs  []string
	CapabilityIDs []string
	Servers       []*mcpmodel.Server
	Capabilities  []*mcpmodel.Capability
}

func (r WorkspaceMCPSelectionResult) Context() map[string]any {
	return r.Selection.Context()
}

func SelectedWorkspaceMCP(ctx context.Context, request WorkspaceMCPSelectionRequest) (WorkspaceMCPSelectionResult, error) {
	selection := activationcore.WorkspaceMCPSelectionContext(
		request.RequestedMCPServerIDs,
		request.DefaultMCPServerIDs,
		request.RequestedCapabilityIDs,
		request.DefaultCapabilityIDs,
		request.Skills,
	)
	ret := WorkspaceMCPSelectionResult{
		Selection:     selection,
		MCPServerIDs:  selection.SelectedMCPServerIDs,
		CapabilityIDs: selection.SelectedCapabilityIDs,
	}
	err := readmodelcore.RunTasks(
		func() error {
			var err error
			ret.Servers, err = SelectedMCPServers(ctx, request.Hub, ret.MCPServerIDs, request.Workspace)
			return err
		},
		func() error {
			var err error
			ret.Capabilities, err = SelectedMCPCapabilities(ctx, request.Hub, ret.CapabilityIDs)
			return err
		},
	)
	if err != nil {
		return WorkspaceMCPSelectionResult{}, err
	}
	return ret, nil
}
