package orchestrator

import "github.com/muidea/workorch/internal/pkg/repository"

// RuntimeEvidenceRepositories are Orchestrator-owned persistence dependencies.
// They are wired directly into Biz and are not a second Runtime owner or
// mutation facade.
type RuntimeEvidenceRepositories struct {
	Results               repository.ResultRepository
	Artifacts             repository.ArtifactRepository
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
