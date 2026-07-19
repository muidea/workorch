package orchestrator

// Evidence purge coordination is internal to the owner Biz.

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

func (m *Biz) purgeRunOwnerResources(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (runevents.RunCommandResult, error) {
	if m == nil || m.EventHub() == nil || workspaceID == "" || runID == "" {
		return runevents.RunCommandResult{}, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(
		runevents.TopicRunCommandPurgeRuntimeResources,
		ModuleID,
		runevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		runevents.PurgeRuntimeResourcesCommand{WorkspaceID: workspaceID, RunID: runID},
	)
	bindOwnerCommandLane(event, runevents.TopicRunCommandPurgeRuntimeResources, "run", string(runID), workspaceID, runID)
	return runCommandResponse(m.SendEvent(event), runevents.TopicRunCommandPurgeRuntimeResources)
}

func purgeRunEvidence(ctx context.Context, evidence RuntimeEvidenceRepositories, runID common.RunID) (int, error) {
	deleted := 0
	if evidence.ToolCalls != nil {
		items, err := evidence.ToolCalls.ListByRun(ctx, runID)
		if err != nil {
			return deleted, err
		}
		for _, item := range items {
			if item == nil {
				continue
			}
			if err := evidence.ToolCalls.Delete(ctx, item.ID); err != nil {
				return deleted, err
			}
			deleted++
		}
	}
	if evidence.Patches != nil {
		items, err := evidence.Patches.ListByRun(ctx, runID)
		if err != nil {
			return deleted, err
		}
		for _, item := range items {
			if item == nil {
				continue
			}
			if err := evidence.Patches.Delete(ctx, item.ID); err != nil {
				return deleted, err
			}
			deleted++
		}
	}
	if evidence.ValidationResults != nil {
		items, err := evidence.ValidationResults.ListByRun(ctx, runID)
		if err != nil {
			return deleted, err
		}
		for _, item := range items {
			if item == nil {
				continue
			}
			if err := evidence.ValidationResults.Delete(ctx, item.ID); err != nil {
				return deleted, err
			}
			deleted++
		}
	}
	if evidence.EvidenceRecords != nil {
		items, err := evidence.EvidenceRecords.ListByRun(ctx, runID)
		if err != nil {
			return deleted, err
		}
		for _, item := range items {
			if item == nil {
				continue
			}
			if err := evidence.EvidenceRecords.Delete(ctx, item.ID); err != nil {
				return deleted, err
			}
			deleted++
		}
	}
	if evidence.CoverageWorkItems != nil {
		items, err := evidence.CoverageWorkItems.ListByRun(ctx, runID)
		if err != nil {
			return deleted, err
		}
		for _, item := range items {
			if item == nil {
				continue
			}
			if err := evidence.CoverageWorkItems.Delete(ctx, item.ID); err != nil {
				return deleted, err
			}
			deleted++
		}
	}
	if evidence.ArtifactContinuations != nil {
		items, err := evidence.ArtifactContinuations.ListByRun(ctx, runID)
		if err != nil {
			return deleted, err
		}
		for _, item := range items {
			if item == nil {
				continue
			}
			if err := evidence.ArtifactContinuations.Delete(ctx, item.ID); err != nil {
				return deleted, err
			}
			deleted++
		}
	}
	if evidence.MemoryRecallTraces != nil {
		items, err := evidence.MemoryRecallTraces.List(ctx, repository.MemoryRecallTraceQuery{RunID: runID})
		if err != nil {
			return deleted, err
		}
		for _, item := range items {
			if item == nil {
				continue
			}
			if err := evidence.MemoryRecallTraces.Delete(ctx, item.ID); err != nil {
				return deleted, err
			}
			deleted++
		}
	}
	return deleted, nil
}
