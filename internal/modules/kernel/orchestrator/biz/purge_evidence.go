package orchestrator

// Evidence purge coordination is internal to the owner Biz.

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
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
	bindOwnerCommandLane(event, runevents.Contracts(), runevents.TopicRunCommandPurgeRuntimeResources, "run", string(runID), workspaceID, runID)
	return runCommandResponse(m.SendEvent(event), runevents.TopicRunCommandPurgeRuntimeResources)
}

func purgeRunEvidence(ctx context.Context, evidence RuntimeEvidenceRepositories, runID common.RunID) (int, error) {
	deleted := 0
	count, err := purgeRunToolCalls(ctx, evidence.ToolCalls, runID)
	deleted += count
	if err != nil {
		return deleted, err
	}
	count, err = purgeRunPatches(ctx, evidence.Patches, runID)
	deleted += count
	if err != nil {
		return deleted, err
	}
	count, err = purgeRunValidationResults(ctx, evidence.ValidationResults, runID)
	deleted += count
	if err != nil {
		return deleted, err
	}
	count, err = purgeRunEvidenceRecords(ctx, evidence.EvidenceRecords, runID)
	deleted += count
	if err != nil {
		return deleted, err
	}
	count, err = purgeRunCoverageWorkItems(ctx, evidence.CoverageWorkItems, runID)
	deleted += count
	if err != nil {
		return deleted, err
	}
	count, err = purgeRunArtifactContinuations(ctx, evidence.ArtifactContinuations, runID)
	deleted += count
	if err != nil {
		return deleted, err
	}
	count, err = purgeRunMemoryRecallTraces(ctx, evidence.MemoryRecallTraces, runID)
	deleted += count
	if err != nil {
		return deleted, err
	}
	return deleted, nil
}

func purgeRunToolCalls(ctx context.Context, repo repository.ToolCallRepository, runID common.RunID) (int, error) {
	if repo == nil {
		return 0, nil
	}
	items, err := repo.ListByRun(ctx, runID)
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, item := range items {
		if item != nil {
			if err := repo.Delete(ctx, item.ID); err != nil {
				return deleted, err
			}
			deleted++
		}
	}
	return deleted, nil
}

func purgeRunPatches(ctx context.Context, repo repository.PatchRepository, runID common.RunID) (int, error) {
	if repo == nil {
		return 0, nil
	}
	items, err := repo.ListByRun(ctx, runID)
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, item := range items {
		if item != nil {
			if err := repo.Delete(ctx, item.ID); err != nil {
				return deleted, err
			}
			deleted++
		}
	}
	return deleted, nil
}

func purgeRunValidationResults(ctx context.Context, repo repository.ValidationResultRepository, runID common.RunID) (int, error) {
	if repo == nil {
		return 0, nil
	}
	items, err := repo.ListByRun(ctx, runID)
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, item := range items {
		if item != nil {
			if err := repo.Delete(ctx, item.ID); err != nil {
				return deleted, err
			}
			deleted++
		}
	}
	return deleted, nil
}

func purgeRunEvidenceRecords(ctx context.Context, repo repository.EvidenceRecordRepository, runID common.RunID) (int, error) {
	if repo == nil {
		return 0, nil
	}
	items, err := repo.ListByRun(ctx, runID)
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, item := range items {
		if item != nil {
			if err := repo.Delete(ctx, item.ID); err != nil {
				return deleted, err
			}
			deleted++
		}
	}
	return deleted, nil
}

func purgeRunCoverageWorkItems(ctx context.Context, repo repository.CoverageWorkItemRepository, runID common.RunID) (int, error) {
	if repo == nil {
		return 0, nil
	}
	items, err := repo.ListByRun(ctx, runID)
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, item := range items {
		if item != nil {
			if err := repo.Delete(ctx, item.ID); err != nil {
				return deleted, err
			}
			deleted++
		}
	}
	return deleted, nil
}

func purgeRunArtifactContinuations(ctx context.Context, repo repository.ArtifactContinuationRepository, runID common.RunID) (int, error) {
	if repo == nil {
		return 0, nil
	}
	items, err := repo.ListByRun(ctx, runID)
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, item := range items {
		if item != nil {
			if err := repo.Delete(ctx, item.ID); err != nil {
				return deleted, err
			}
			deleted++
		}
	}
	return deleted, nil
}

func purgeRunMemoryRecallTraces(ctx context.Context, repo repository.MemoryRecallTraceRepository, runID common.RunID) (int, error) {
	if repo == nil {
		return 0, nil
	}
	items, err := repo.List(ctx, workorchquerycontract.MemoryRecallTraceQuery{RunID: runID})
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, item := range items {
		if item != nil {
			if err := repo.Delete(ctx, item.ID); err != nil {
				return deleted, err
			}
			deleted++
		}
	}
	return deleted, nil
}
