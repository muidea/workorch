package orchestrator

import (
	"context"

	orchestratorbiz "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

func purgeRunEvidence(ctx context.Context, steps repository.StepRepository, evidence orchestratorbiz.RuntimeEvidenceRepositories, runID common.RunID) (int, error) {
	deleted := 0
	if steps != nil {
		items, err := steps.ListByRun(ctx, runID)
		if err != nil {
			return deleted, err
		}
		for _, item := range items {
			if item == nil {
				continue
			}
			if err := steps.Delete(ctx, item.ID); err != nil {
				return deleted, err
			}
			deleted++
		}
	}
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
	if evidence.RuntimeCheckpoints != nil {
		items, err := evidence.RuntimeCheckpoints.ListByRun(ctx, runID)
		if err != nil {
			return deleted, err
		}
		for _, item := range items {
			if item == nil {
				continue
			}
			if err := evidence.RuntimeCheckpoints.Delete(ctx, item.ID); err != nil {
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
