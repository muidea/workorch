package server

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

// Memory CRUD/search/recall adapters stay isolated from registry and Runtime
// lifecycle concerns.
func (s *serverState) configureMemoryOperations(session *runtimecore.Session) {
	s.createMemoryItem = func(ctx context.Context, _, _, workspaceID string, cmd workorchmanagementcontract.MemoryMutation) (workorchmanagementcontract.MemoryItem, error) {
		var ret workorchmanagementcontract.MemoryItem
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.CreateMemoryItemWithSession(ctx, session, workspaceID, cmd)
			return err
		})
		return ret, err
	}
	s.readMemoryItem = func(ctx context.Context, _, _, memoryID string) (workorchmanagementcontract.MemoryItem, error) {
		var ret workorchmanagementcontract.MemoryItem
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadMemoryItemWithSession(ctx, session, memoryID)
			return err
		})
		return ret, err
	}
	s.updateMemoryItem = func(ctx context.Context, _, _, memoryID string, cmd workorchmanagementcontract.MemoryMutation) (workorchmanagementcontract.MemoryItem, error) {
		var ret workorchmanagementcontract.MemoryItem
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.UpdateMemoryItemWithSession(ctx, session, memoryID, cmd)
			return err
		})
		return ret, err
	}
	s.deleteMemoryItem = func(ctx context.Context, _, _, memoryID string) error {
		return withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			return appservice.DeleteMemoryItemWithSession(ctx, session, memoryID)
		})
	}
	s.listMemoryByWorkspace = func(ctx context.Context, _, _, workspaceID string, query string) ([]workorchmanagementcontract.MemoryItem, error) {
		var ret []workorchmanagementcontract.MemoryItem
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ListMemoryByWorkspaceWithSession(ctx, session, workspaceID, query)
			return err
		})
		return ret, err
	}
	s.searchMemory = func(ctx context.Context, _, _ string, query workorchquerycontract.MemorySearchQuery) ([]workorchmanagementcontract.MemoryItem, error) {
		var ret []workorchmanagementcontract.MemoryItem
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.SearchMemoryWithSession(ctx, session, query)
			return err
		})
		return ret, err
	}
	s.listMemoryRecallTraces = func(ctx context.Context, _, _ string, query workorchquerycontract.MemoryRecallTraceQuery) ([]projectioncontract.MemoryRecallTrace, error) {
		var ret []projectioncontract.MemoryRecallTrace
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ListMemoryRecallTracesWithSession(ctx, session, query)
			return err
		})
		return ret, err
	}
}
