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
func (b *Biz) observePreparationFinalOwners(ctx context.Context, original workspaceevents.CheckWorkspacePreparationCommand) (*workspace.PreparationOwnerRevalidation, []workspace.PreparationObservationDiagnostic) {
	r := workspace.PreparationOwnerRevalidation{Version: workspace.PreparationOwnerRevalidationVersion, WorkspaceID: original.WorkspaceID, RequestID: original.RequestID, PayloadHash: original.PayloadHash(), WorkspaceRevision: original.ExpectedWorkspaceRevision + 1, StartedAt: time.Now().UTC()}
	c := workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: original.WorkspaceID, ExpectedWorkspaceRevision: r.WorkspaceRevision}
	diagnostics := make([]workspace.PreparationObservationDiagnostic, 0, 4)
	observe := func(owner string, f func() (bool, error)) {
		start := time.Now().UTC()
		attempted := ctx.Err() == nil
		complete, err := false, ctx.Err()
		if attempted {
			complete, err = f()
		}
		diagnostics = append(diagnostics, preparationDiagnostic(ctx, "final", owner, "observe", start, attempted, complete, err))
	}
	observe("soul", func() (bool, error) {
		value, err := b.ObservePreparationSouls(ctx, c)
		if err != nil {
			return false, preparationStageError(value.FailureCode, err)
		}
		if err = value.PreparationSoulFacts.Validate(); err != nil {
			return false, preparationStageError("validate", err)
		}
		f := value.PreparationSoulFacts.Clone()
		r.Souls = &f
		return true, nil
	})
	observe("skill", func() (bool, error) {
		value, err := b.ObservePreparationSkillRegistry(ctx, c)
		if err != nil {
			return false, preparationStageError(value.FailureCode, err)
		}
		f := value.Facts()
		if err = f.Validate(); err != nil {
			return false, preparationStageError("validate", err)
		}
		r.Skills = &f
		return true, nil
	})
	observe("memory", func() (bool, error) {
		f, err := b.observePreparationMemoryFacts(ctx, original)
		// Preserve a complete inventory even when its mode/seal read failed.
		if f != nil && f.Validate() == nil {
			r.Memory = f
		}
		return r.Memory != nil && r.Memory.SelectionSource != nil, err
	})
	observe("mcpregistry", func() (bool, error) {
		f, err := b.observePreparationMCPRegistry(ctx, original)
		if err != nil {
			return false, err
		}
		if f == nil {
			return false, nil
		}
		if err = f.Validate(); err != nil {
			return false, preparationStageError("validate", err)
		}
		r.MCP = f
		return true, nil
	})
	r.CompletedAt = time.Now().UTC()
	if err := r.Validate(); err != nil {
		diagnostics = append(diagnostics, preparationDiagnostic(ctx, "final", "workspace", "validate_final_observation", r.StartedAt, true, false, err))
		return nil, diagnostics
	}
	return &r, diagnostics
}
