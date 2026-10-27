package biz

import (
	"context"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
)

func (b *Biz) readPreparationMCPRegistry(ctx context.Context) (mcpevents.RegistrySnapshotResult, error) {
	if b == nil || b.EventHub() == nil || ctx == nil || ctx.Err() != nil {
		return mcpevents.RegistrySnapshotResult{}, common.ErrInvalidArgument
	}
	ev := magicevent.NewEventWithContext(mcpevents.TopicMCPCommandReadRegistrySnapshot, ModuleID, mcpevents.CommandDestination, magicevent.NewHeader(), ctx, mcpevents.ReadRegistrySnapshotCommand{})
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(mcpevents.Contracts(), ev.ID(), eventcontract.LaneScope{SubjectType: "mcp_registry", SubjectID: "global"}))
	r := b.SendEvent(ev)
	if r == nil {
		return mcpevents.RegistrySnapshotResult{}, common.ErrConflict
	}
	if err := r.Error(); err != nil {
		return mcpevents.RegistrySnapshotResult{}, err
	}
	raw, err := r.Get()
	value, ok := raw.(mcpevents.RegistrySnapshotResult)
	if err != nil || !ok || value.ValidatePreparation() != nil || ctx.Err() != nil {
		return mcpevents.RegistrySnapshotResult{}, common.ErrConflict
	}
	return value.Clone(), nil
}

// Uses only the complete owner read. Never EnsureDefaults, refresh, connect,
// initialize, invoke a tool or confuse a selected subset with a full registry.
func (b *Biz) observePreparationMCPRegistry(ctx context.Context, original workspaceevents.CheckWorkspacePreparationCommand) (*workspace.PreparationMCPObservationFacts, error) {
	started := time.Now().UTC()
	c := workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: original.WorkspaceID, ExpectedWorkspaceRevision: original.ExpectedWorkspaceRevision + 1}
	exact, err := b.QueryExactConfirmedStartupSnapshot(ctx, c)
	if err != nil {
		return nil, err
	}
	before, err := b.readPreparationMCPRegistry(ctx)
	if err != nil {
		return nil, err
	}
	after, err := b.readPreparationMCPRegistry(ctx)
	if err != nil {
		return nil, err
	}
	reobserved, err := b.QueryExactConfirmedStartupSnapshot(ctx, c)
	if err != nil {
		return nil, err
	}
	f := workspace.PreparationMCPObservationFacts{Version: workspace.PreparationMCPRegistryEvidenceVersion, WorkspaceID: original.WorkspaceID, WorkspaceRevision: c.ExpectedWorkspaceRevision, SnapshotHash: exact.Snapshot.SnapshotHash, ReobservedSnapshotHash: reobserved.Snapshot.SnapshotHash, Before: before, After: after, StartedAt: started, CompletedAt: time.Now().UTC()}
	if err := f.Validate(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &f, nil
}
