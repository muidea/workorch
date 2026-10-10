package activationports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

type WorkspaceMCPSelectionRequest struct {
	Hub                    magicevent.Hub
	Workspace              *workspacemodel.Workspace
	RequestedMCPServerIDs  []string
	DefaultMCPServerIDs    []string
	RequestedCapabilityIDs []string
	DefaultCapabilityIDs   []string
	Skills                 []workorchmanagementcontract.Skill
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
	if len(ret.MCPServerIDs) == 0 && len(ret.CapabilityIDs) == 0 {
		return ret, nil
	}
	servers, capabilities, _, err := ownerreadports.ReadMCPSelectionSnapshot(ctx, request.Hub, ret.MCPServerIDs, ret.CapabilityIDs)
	if err != nil {
		return WorkspaceMCPSelectionResult{}, err
	}
	ret.Servers = make([]*mcpmodel.Server, 0, len(servers))
	for _, server := range servers {
		ret.Servers = append(ret.Servers, activationcore.MCPServerForWorkspace(server, request.Workspace))
	}
	ret.Capabilities = capabilities
	return ret, nil
}
