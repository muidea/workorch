package orchestrator

import (
	"context"

	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type resultEvidencePurgeRepository interface {
	ListByRun(context.Context, common.RunID) ([]*resultmodel.Result, error)
	Delete(context.Context, common.ResultID) error
}

type artifactEvidencePurgeRepository interface {
	ListByRun(context.Context, common.RunID) ([]*resultmodel.Artifact, error)
	Delete(context.Context, common.ArtifactID) error
}

// RuntimeEvidenceRepositories are Orchestrator-owned persistence dependencies.
// They are wired directly into Biz and are not a second Runtime owner or
// mutation facade.
type RuntimeEvidenceRepositories struct {
	Results               resultEvidencePurgeRepository
	Artifacts             artifactEvidencePurgeRepository
	ToolCalls             repository.ToolCallRepository
	Patches               repository.PatchRepository
	ValidationResults     repository.ValidationResultRepository
	EvidenceRecords       repository.EvidenceRecordRepository
	CoverageWorkItems     repository.CoverageWorkItemRepository
	ArtifactContinuations repository.ArtifactContinuationRepository
	MemoryItems           repository.MemoryItemRepository
	MemoryRecallTraces    repository.MemoryRecallTraceRepository
	AutoGovernanceTraces  repository.AutoGovernanceTraceRepository
}
