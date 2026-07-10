package activationports

import (
	"context"
	"sync"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	mcpmodel "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func SelectedMCPServers(ctx context.Context, hub magicevent.Hub, ids []string, workspace *workspacemodel.Workspace) ([]*mcpmodel.Server, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	type serverResult struct {
		item *mcpmodel.Server
		err  error
	}
	results := make([]serverResult, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func(idx int, sid string) {
			defer wg.Done()
			results[idx].item, results[idx].err = ownerreadports.ReadMCPServerByOwnerCommand(ctx, hub, common.MCPServerID(sid))
		}(i, id)
	}
	wg.Wait()
	ret := make([]*mcpmodel.Server, 0, len(ids))
	for _, r := range results {
		if r.err != nil {
			return nil, r.err
		}
		if !r.item.Enabled {
			return nil, common.ErrConflict
		}
		ret = append(ret, activationcore.MCPServerForWorkspace(r.item, workspace))
	}
	return ret, nil
}

func SelectedMCPCapabilities(ctx context.Context, hub magicevent.Hub, ids []string) ([]*mcpmodel.Capability, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	type capResult struct {
		item *mcpmodel.Capability
		err  error
	}
	results := make([]capResult, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func(idx int, cid string) {
			defer wg.Done()
			results[idx].item, results[idx].err = ownerreadports.ReadMCPCapabilityByOwnerCommand(ctx, hub, common.MCPCapabilityID(cid))
		}(i, id)
	}
	wg.Wait()
	ret := make([]*mcpmodel.Capability, 0, len(ids))
	for _, r := range results {
		if r.err != nil {
			return nil, r.err
		}
		if !r.item.Enabled {
			return nil, common.ErrConflict
		}
		ret = append(ret, r.item)
	}
	return ret, nil
}
