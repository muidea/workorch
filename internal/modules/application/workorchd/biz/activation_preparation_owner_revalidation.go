package biz

import (
	"context"
	"time"

	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
)

// Final read-only observations use the existing owner contracts and the same
// original scope/deadline. No catalog refresh, SkillHub command, inference,
// embedding, connection or fallback is introduced. Preserve nil on failure.
func (b *Biz) observePreparationFinalOwners(ctx context.Context, original workspaceevents.CheckWorkspacePreparationCommand) *workspace.PreparationOwnerRevalidation {
	r := workspace.PreparationOwnerRevalidation{Version: workspace.PreparationOwnerRevalidationVersion, WorkspaceID: original.WorkspaceID, RequestID: original.RequestID, PayloadHash: original.PayloadHash(), WorkspaceRevision: original.ExpectedWorkspaceRevision + 1, StartedAt: time.Now().UTC()}
	c := workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: original.WorkspaceID, ExpectedWorkspaceRevision: r.WorkspaceRevision}
	if value, err := b.ObservePreparationSouls(ctx, c); err == nil && value.PreparationSoulFacts.Validate() == nil {
		f := value.PreparationSoulFacts.Clone()
		r.Souls = &f
	}
	if value, err := b.ObservePreparationSkillRegistry(ctx, c); err == nil {
		if f := value.Facts(); f.Validate() == nil {
			r.Skills = &f
		}
	}
	if f, err := b.observePreparationMemoryFacts(ctx, original); err == nil && f != nil && f.Validate() == nil {
		r.Memory = f
	}
	if f, err := b.observePreparationMCPRegistry(ctx, original); err == nil && f != nil && f.Validate() == nil {
		r.MCP = f
	}
	r.CompletedAt = time.Now().UTC()
	if r.Validate() != nil {
		return nil
	}
	return &r
}
