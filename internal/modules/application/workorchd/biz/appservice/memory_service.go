package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistryports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func CreateMemoryItemWithSession(ctx context.Context, session *runtimecore.Session, workspaceID string, cmd workorchmanagementcontract.MemoryMutation) (ret workorchmanagementcontract.MemoryItem, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (workorchmanagementcontract.MemoryItem, error) {
		return contextregistryports.CreateMemoryItemForWorkspace(ctx, hub, workspaceID, cmd)
	})
}

func CreateMemoryItemWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string, cmd workorchmanagementcontract.MemoryMutation) (workorchmanagementcontract.MemoryItem, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (workorchmanagementcontract.MemoryItem, error) {
		return CreateMemoryItemWithSession(ctx, session, workspaceID, cmd)
	})
}

func UpdateMemoryItemWithSession(ctx context.Context, session *runtimecore.Session, memoryID string, cmd workorchmanagementcontract.MemoryMutation) (ret workorchmanagementcontract.MemoryItem, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (workorchmanagementcontract.MemoryItem, error) {
		return contextregistryports.UpdateMemoryItemView(ctx, hub, memoryID, cmd)
	})
}

func UpdateMemoryItemWithBackend(ctx context.Context, backend string, dataRoot string, memoryID string, cmd workorchmanagementcontract.MemoryMutation) (workorchmanagementcontract.MemoryItem, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (workorchmanagementcontract.MemoryItem, error) {
		return UpdateMemoryItemWithSession(ctx, session, memoryID, cmd)
	})
}

func DeleteMemoryItemWithSession(ctx context.Context, session *runtimecore.Session, memoryID string) (err error) {
	_, err = withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (struct{}, error) {
		return struct{}{}, contextregistryports.DeleteMemoryItemView(ctx, hub, memoryID)
	})
	return err
}

func DeleteMemoryItemWithBackend(ctx context.Context, backend string, dataRoot string, memoryID string) error {
	_, err := withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (struct{}, error) {
		return struct{}{}, DeleteMemoryItemWithSession(ctx, session, memoryID)
	})
	return err
}
