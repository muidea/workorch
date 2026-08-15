package biz

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

func (p *runtimeApplicationPorts) configureMemoryPorts() {
	ports := &p.ports
	ports.Memory.CreateMemoryItem = func(ctx context.Context, _, _, workspaceID string, cmd workorchmanagementcontract.MemoryMutation) (workorchmanagementcontract.MemoryItem, error) {
		var ret workorchmanagementcontract.MemoryItem
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.CreateMemoryItemWithSession(ctx, session, workspaceID, cmd)
			return err
		})
		return ret, err
	}
	ports.Memory.ReadMemoryItem = func(ctx context.Context, _, _, memoryID string) (workorchmanagementcontract.MemoryItem, error) {
		var ret workorchmanagementcontract.MemoryItem
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadMemoryItemWithSession(ctx, session, memoryID)
			return err
		})
		return ret, err
	}
	ports.Memory.UpdateMemoryItem = func(ctx context.Context, _, _, memoryID string, cmd workorchmanagementcontract.MemoryMutation) (workorchmanagementcontract.MemoryItem, error) {
		var ret workorchmanagementcontract.MemoryItem
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.UpdateMemoryItemWithSession(ctx, session, memoryID, cmd)
			return err
		})
		return ret, err
	}
	ports.Memory.DeleteMemoryItem = func(ctx context.Context, _, _, memoryID string) error {
		return p.withSession(ctx, func(session *runtimecore.Session) error {
			return appservice.DeleteMemoryItemWithSession(ctx, session, memoryID)
		})
	}
	ports.Memory.ListMemoryByWorkspace = func(ctx context.Context, _, _, workspaceID string, query string) ([]workorchmanagementcontract.MemoryItem, error) {
		var ret []workorchmanagementcontract.MemoryItem
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ListMemoryByWorkspaceWithSession(ctx, session, workspaceID, query)
			return err
		})
		return ret, err
	}
	ports.Memory.SearchMemory = func(ctx context.Context, _, _ string, query workorchquerycontract.MemorySearchQuery) ([]workorchmanagementcontract.MemoryItem, error) {
		var ret []workorchmanagementcontract.MemoryItem
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.SearchMemoryWithSession(ctx, session, query)
			return err
		})
		return ret, err
	}
	ports.Memory.ListMemoryRecallTraces = func(ctx context.Context, _, _ string, query workorchquerycontract.MemoryRecallTraceQuery) ([]projectioncontract.MemoryRecallTrace, error) {
		var ret []projectioncontract.MemoryRecallTrace
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ListMemoryRecallTracesWithSession(ctx, session, query)
			return err
		})
		return ret, err
	}
}
