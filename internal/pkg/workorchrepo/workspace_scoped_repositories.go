package workorchrepo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runtimeevidencemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	fileRepo "github.com/muidea/workorch/internal/pkg/repository/file"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func decodeCurrentJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return common.ErrInvalidArgument
		}
		return err
	}
	return nil
}

const (
	workspaceScopedWorkspaceFile           = "workspace.json"
	workspaceScopedWorkspaceIndexFile      = "workspace_index.json"
	workspaceScopedConfigDir               = "config"
	workspaceScopedSoulBindingsFile        = "soul_bindings.json"
	workspaceScopedGoalFile                = "goals.json"
	workspaceScopedDoDFile                 = "dods.json"
	workspaceScopedMemoryItemFile          = "memory_item.json"
	workspaceScopedMemoryRecallTraceFile   = "memory_recall_trace.json"
	workspaceScopedRunFile                 = "run.json"
	workspaceScopedRunGenerationDir        = "run-generations"
	workspaceScopedStepFile                = "steps"
	workspaceScopedToolCallFile            = "tool-calls"
	workspaceScopedPatchFile               = "patches"
	workspaceScopedValidationResultFile    = "validations"
	workspaceScopedRuntimeCheckpointFile   = "checkpoints"
	workspaceScopedEvidenceRecordDir       = "evidence-records"
	workspaceScopedCoverageWorkItemDir     = "coverage-work-items"
	workspaceScopedArtifactContinuationDir = "artifact-continuations"
	workspaceScopedResultFile              = "results"
	workspaceScopedArtifactFile            = "artifacts"
	workspaceScopedEventRecordFile         = "."
	workspaceScopedEventPositionFile       = ".eventstream-position"
	workspaceScopedTicketFile              = "decisions/tickets"
	workspaceScopedResolutionFile          = "decisions/resolutions"
	workspaceScopedApprovalFile            = "approvals"
)

type workspaceScopedFileStore struct {
	dataRoot        string
	runOwnerBinding repository.RunOwnerRepositoryBinding

	indexMu               sync.Mutex
	mu                    sync.Mutex
	workspaces            map[string]*fileRepo.WorkspaceRepository
	goals                 map[string]*fileRepo.GoalRepository
	dods                  map[string]*fileRepo.DoDRepository
	memoryItems           map[string]*fileRepo.MemoryItemRepository
	memoryRecallTraces    map[string]*fileRepo.MemoryRecallTraceRepository
	runs                  map[string]*fileRepo.RunRepository
	steps                 map[string]*fileRepo.StepRepository
	toolCalls             map[string]*fileRepo.ToolCallRepository
	patches               map[string]*fileRepo.PatchRepository
	validationResults     map[string]*fileRepo.ValidationResultRepository
	runtimeCheckpoints    map[string]*fileRepo.RuntimeCheckpointRepository
	evidenceRecords       map[string]*fileRepo.EvidenceRecordRepository
	coverageWorkItems     map[string]*fileRepo.CoverageWorkItemRepository
	artifactContinuations map[string]*fileRepo.ArtifactContinuationRepository
	results               map[string]*fileRepo.ResultRepository
	artifacts             map[string]*fileRepo.ArtifactRepository
	eventRecords          map[string]*fileRepo.EventRecordRepository
	tickets               map[string]*fileRepo.DecisionTicketRepository
	resolutions           map[string]*fileRepo.DecisionResolutionRepository
	approvals             map[string]*fileRepo.ApprovalRepository
}

var workspaceRunGenerationLocks sync.Map

var workspaceIndexLocks sync.Map

func workspaceRunGenerationLock(path string) *sync.Mutex {
	key, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		key = filepath.Clean(path)
	}
	value, _ := workspaceRunGenerationLocks.LoadOrStore(key, &sync.Mutex{})
	return value.(*sync.Mutex)
}

func workspaceIndexLock(path string) *sync.Mutex {
	key, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		key = filepath.Clean(path)
	}
	value, _ := workspaceIndexLocks.LoadOrStore(key, &sync.Mutex{})
	return value.(*sync.Mutex)
}

type WorkspaceScopedFileRepositorySet struct {
	MemoryItems           repository.MemoryItemRepository
	MemoryRecallTraces    repository.MemoryRecallTraceRepository
	MemoryDeletions       repository.MemoryWorkspaceDeletionRepository
	Workspaces            repository.WorkspaceRepository
	WorkspaceCleaner      repository.WorkspaceDeletionMaterialCleaner
	Goals                 repository.GoalRepository
	DoDs                  repository.DoDRepository
	Results               repository.ResultRepository
	Artifacts             repository.ArtifactRepository
	ResultDeletions       repository.ResultWorkspaceDeletionRepository
	EventRecords          repository.EventRecordRepository
	EventStreamDeletions  repository.EventStreamWorkspaceDeletionRepository
	EventStreamPurger     repository.EventStreamWorkspacePurger
	Runs                  repository.RunRepository
	Steps                 repository.StepRepository
	RunOwner              repository.RunOwnerRepositories
	ToolCalls             repository.ToolCallRepository
	Patches               repository.PatchRepository
	ValidationResults     repository.ValidationResultRepository
	RuntimeCheckpoints    repository.RuntimeCheckpointRepository
	EvidenceRecords       repository.EvidenceRecordRepository
	CoverageWorkItems     repository.CoverageWorkItemRepository
	ArtifactContinuations repository.ArtifactContinuationRepository
	OrchestratorDeletions repository.OrchestratorWorkspaceDeletionRepository
	OrchestratorPurger    repository.OrchestratorWorkspaceEvidencePurger
	OrchestratorEvidence  repository.OrchestratorEvidenceOwnerRepositories
	Tickets               repository.DecisionTicketRepository
	Resolutions           repository.DecisionResolutionRepository
	Approvals             repository.ApprovalRepository
	DecisionDeletions     repository.DecisionWorkspaceDeletionRepository
	ApprovalDeletions     repository.ApprovalWorkspaceDeletionRepository
}

type workspaceIndex struct {
	Workspaces []workspaceIndexEntry `json:"workspaces"`
}

type workspaceIndexEntry struct {
	WorkspaceID          common.WorkspaceID    `json:"workspaceId"`
	InternalRoot         string                `json:"internalRoot"`
	DeliverableDirectory string                `json:"deliverableDirectory"`
	InternalDirectory    string                `json:"internalDirectory"`
	State                common.WorkspaceState `json:"state"`
	ActiveRunID          common.RunID          `json:"activeRunId,omitempty"`
	UpdatedAt            string                `json:"updatedAt"`
}

type workspaceSoulBindingsFile struct {
	DefaultSoulID common.SoulID            `json:"defaultSoulId,omitempty"`
	Bindings      map[string]common.SoulID `json:"bindings,omitempty"`
}

func NewWorkspaceScopedFileRepositorySet(dataRoot string) WorkspaceScopedFileRepositorySet {
	store := newWorkspaceScopedFileStore(dataRoot)
	workspaces := newWorkspaceScopedWorkspaceRepository(store)
	runs := newWorkspaceScopedRunRepository(store)
	steps := newWorkspaceScopedStepRepository(store, runs)
	checkpoints := newWorkspaceScopedRuntimeCheckpointRepository(store, runs)
	runPurger := &workspaceScopedRunWorkspaceResourcePurger{store: store, runs: runs}
	runOwner, runOwnerErr := repository.NewRunOwnerRepositories(runs, steps, checkpoints, runPurger)
	if runOwnerErr != nil {
		panic(runOwnerErr)
	}
	tickets := newWorkspaceScopedDecisionTicketRepository(store)
	artifacts := NewArtifactStorageRepository(newWorkspaceScopedArtifactRepository(store), dataRoot)
	eventStreamDeletions := newEventStreamWorkspaceDeletionRepository(dataRoot)
	eventRecords := newWorkspaceScopedEventRecordRepository(store, eventStreamDeletions)
	orchestratorDeletions := newOrchestratorWorkspaceDeletionRepository(dataRoot)
	orchestratorPurger := &workspaceScopedOrchestratorEvidencePurger{store: store, deletions: orchestratorDeletions}
	orchestratorEvidence, _ := repository.NewOrchestratorEvidenceOwnerRepositories(orchestratorPurger)
	return WorkspaceScopedFileRepositorySet{
		MemoryItems:           newWorkspaceScopedMemoryItemRepository(store),
		MemoryRecallTraces:    newWorkspaceScopedMemoryRecallTraceRepository(store),
		MemoryDeletions:       newMemoryWorkspaceDeletionRepository(dataRoot),
		Workspaces:            workspaces,
		WorkspaceCleaner:      workspaces,
		Goals:                 newWorkspaceScopedGoalRepository(store),
		DoDs:                  newWorkspaceScopedDoDRepository(store),
		Results:               newWorkspaceScopedResultRepository(store),
		Artifacts:             artifacts,
		ResultDeletions:       newResultWorkspaceDeletionRepository(dataRoot),
		EventRecords:          eventRecords,
		EventStreamDeletions:  eventStreamDeletions,
		EventStreamPurger:     eventRecords,
		Runs:                  runs,
		Steps:                 steps,
		RunOwner:              runOwner,
		ToolCalls:             orchestratorEvidence.ToolCalls(),
		Patches:               orchestratorEvidence.Patches(),
		ValidationResults:     orchestratorEvidence.ValidationResults(),
		RuntimeCheckpoints:    checkpoints,
		EvidenceRecords:       orchestratorEvidence.EvidenceRecords(),
		CoverageWorkItems:     orchestratorEvidence.CoverageWorkItems(),
		ArtifactContinuations: orchestratorEvidence.ArtifactContinuations(),
		OrchestratorDeletions: orchestratorDeletions,
		OrchestratorPurger:    orchestratorPurger,
		OrchestratorEvidence:  orchestratorEvidence,
		Tickets:               tickets,
		Resolutions:           newWorkspaceScopedDecisionResolutionRepository(store, tickets),
		Approvals:             newWorkspaceScopedApprovalRepository(store),
		DecisionDeletions:     newDecisionWorkspaceDeletionRepository(dataRoot),
		ApprovalDeletions:     newApprovalWorkspaceDeletionRepository(dataRoot),
	}
}

type workspaceScopedOrchestratorEvidencePurger struct {
	store              *workspaceScopedFileStore
	deletions          repository.OrchestratorWorkspaceDeletionRepository
	afterPhysicalPurge func() error
}

type orchestratorEvidencePurgeJournalPhase string

const (
	orchestratorEvidencePurgeJournalPending   orchestratorEvidencePurgeJournalPhase = "pending"
	orchestratorEvidencePurgeJournalCompleted orchestratorEvidencePurgeJournalPhase = "completed"
)

type orchestratorEvidencePurgeJournal struct {
	Phase   orchestratorEvidencePurgeJournalPhase                `json:"phase"`
	Receipt repository.OrchestratorWorkspaceEvidencePurgeReceipt `json:"receipt"`
}

func (p *workspaceScopedOrchestratorEvidencePurger) OrchestratorWorkspaceDeletions() repository.OrchestratorWorkspaceDeletionRepository {
	return p.deletions
}

func (p *workspaceScopedOrchestratorEvidencePurger) OrchestratorToolCalls() repository.ToolCallRepository {
	return newWorkspaceScopedToolCallRepository(p.store)
}

func (p *workspaceScopedOrchestratorEvidencePurger) OrchestratorPatches() repository.PatchRepository {
	return newWorkspaceScopedPatchRepository(p.store)
}

func (p *workspaceScopedOrchestratorEvidencePurger) OrchestratorValidationResults() repository.ValidationResultRepository {
	return newWorkspaceScopedValidationResultRepository(p.store)
}

func (p *workspaceScopedOrchestratorEvidencePurger) OrchestratorEvidenceRecords() repository.EvidenceRecordRepository {
	return newWorkspaceScopedEvidenceRecordRepository(p.store)
}

func (p *workspaceScopedOrchestratorEvidencePurger) OrchestratorCoverageWorkItems() repository.CoverageWorkItemRepository {
	return newWorkspaceScopedCoverageWorkItemRepository(p.store)
}

func (p *workspaceScopedOrchestratorEvidencePurger) OrchestratorArtifactContinuations() repository.ArtifactContinuationRepository {
	return newWorkspaceScopedArtifactContinuationRepository(p.store)
}

func (p *workspaceScopedOrchestratorEvidencePurger) OrchestratorWorkspacePurger() repository.OrchestratorWorkspaceEvidencePurger {
	return p
}

func (p *workspaceScopedOrchestratorEvidencePurger) PurgeWorkspaceEvidence(ctx context.Context, scope deletioncontract.Scope) (repository.OrchestratorWorkspaceEvidencePurgeReceipt, error) {
	if p == nil || p.store == nil || p.deletions == nil || scope.Validate() != nil || scope.Mode != deletioncontract.ModePurge {
		return repository.OrchestratorWorkspaceEvidencePurgeReceipt{}, common.ErrInvalidArgument
	}
	var receipt repository.OrchestratorWorkspaceEvidencePurgeReceipt
	err := p.deletions.WithWorkspacePurgeFence(ctx, scope, func(context.Context) error {
		journal, err := p.loadPurgeJournal(scope)
		switch {
		case err == nil:
			receipt = journal.Receipt.Clone()
			if journal.Phase == orchestratorEvidencePurgeJournalCompleted {
				after, observeErr := observeWorkspaceScopedOrchestratorEvidence(p.store, scope.WorkspaceID)
				if observeErr != nil {
					return observeErr
				}
				if !orchestratorWorkspaceEvidenceCountsEmpty(after) {
					return common.ErrConflict
				}
				return nil
			}
		case errors.Is(err, common.ErrNotFound):
			before, observeErr := observeWorkspaceScopedOrchestratorEvidence(p.store, scope.WorkspaceID)
			if observeErr != nil {
				return observeErr
			}
			observations := repository.EmptyOrchestratorWorkspaceEvidencePurgeObservations()
			for idx := range observations {
				observations[idx].ObservedBefore = before[observations[idx].Surface]
				observations[idx].Deleted = observations[idx].ObservedBefore
			}
			receipt, err = repository.NewOrchestratorWorkspaceEvidencePurgeReceipt(scope, observations)
			if err != nil {
				return err
			}
			journal = orchestratorEvidencePurgeJournal{Phase: orchestratorEvidencePurgeJournalPending, Receipt: receipt.Clone()}
			if err := p.savePurgeJournal(scope, journal); err != nil {
				return err
			}
		default:
			return err
		}

		for _, root := range p.store.workspaceRunRoots(scope.WorkspaceID) {
			for _, name := range []string{
				workspaceScopedToolCallFile,
				workspaceScopedPatchFile,
				workspaceScopedValidationResultFile,
				workspaceScopedEvidenceRecordDir,
				workspaceScopedCoverageWorkItemDir,
				workspaceScopedArtifactContinuationDir,
			} {
				if err := os.RemoveAll(p.store.runPath(root, name)); err != nil {
					return err
				}
				if _, err := os.Stat(p.store.runPath(root, name)); err == nil {
					return common.ErrConflict
				} else if !errors.Is(err, os.ErrNotExist) {
					return err
				}
			}
		}
		p.store.evictOrchestratorEvidenceRepositories(scope.WorkspaceID)
		if p.afterPhysicalPurge != nil {
			if err := p.afterPhysicalPurge(); err != nil {
				return err
			}
		}
		after, err := observeWorkspaceScopedOrchestratorEvidence(p.store, scope.WorkspaceID)
		if err != nil {
			return err
		}
		if !orchestratorWorkspaceEvidenceCountsEmpty(after) {
			return common.ErrConflict
		}
		journal = orchestratorEvidencePurgeJournal{Phase: orchestratorEvidencePurgeJournalCompleted, Receipt: receipt.Clone()}
		return p.savePurgeJournal(scope, journal)
	})
	if err != nil {
		return repository.OrchestratorWorkspaceEvidencePurgeReceipt{}, err
	}
	return receipt, receipt.ValidateFor(scope)
}

func (p *workspaceScopedOrchestratorEvidencePurger) purgeJournalPath(workspaceID common.WorkspaceID) (string, error) {
	if p == nil || p.store == nil {
		return "", common.ErrInvalidArgument
	}
	return workspaceDeletionFilePath(p.store.dataRoot, "orchestrator-evidence-purge", workspaceID)
}

func (p *workspaceScopedOrchestratorEvidencePurger) loadPurgeJournal(scope deletioncontract.Scope) (orchestratorEvidencePurgeJournal, error) {
	path, err := p.purgeJournalPath(scope.WorkspaceID)
	if err != nil {
		return orchestratorEvidencePurgeJournal{}, err
	}
	journal, err := readWorkspaceDeletionFile[orchestratorEvidencePurgeJournal](path)
	if err != nil {
		return orchestratorEvidencePurgeJournal{}, err
	}
	if journal == nil || (journal.Phase != orchestratorEvidencePurgeJournalPending && journal.Phase != orchestratorEvidencePurgeJournalCompleted) || journal.Receipt.ValidateFor(scope) != nil {
		return orchestratorEvidencePurgeJournal{}, common.ErrConflict
	}
	journal.Receipt = journal.Receipt.Clone()
	return *journal, nil
}

func (p *workspaceScopedOrchestratorEvidencePurger) savePurgeJournal(scope deletioncontract.Scope, journal orchestratorEvidencePurgeJournal) error {
	if (journal.Phase != orchestratorEvidencePurgeJournalPending && journal.Phase != orchestratorEvidencePurgeJournalCompleted) || journal.Receipt.ValidateFor(scope) != nil {
		return common.ErrInvalidArgument
	}
	path, err := p.purgeJournalPath(scope.WorkspaceID)
	if err != nil {
		return err
	}
	journal.Receipt = journal.Receipt.Clone()
	return writeWorkspaceDeletionFile(path, journal)
}

func (s *workspaceScopedFileStore) evictOrchestratorEvidenceRepositories(workspaceID common.WorkspaceID) {
	if s == nil || workspaceID == "" {
		return
	}
	prefix := filepath.Clean(filepath.Join(s.workspaceRoot(workspaceID), "runs")) + string(filepath.Separator)
	s.mu.Lock()
	defer s.mu.Unlock()
	for path := range s.toolCalls {
		if strings.HasPrefix(filepath.Clean(path), prefix) {
			delete(s.toolCalls, path)
		}
	}
	for path := range s.patches {
		if strings.HasPrefix(filepath.Clean(path), prefix) {
			delete(s.patches, path)
		}
	}
	for path := range s.validationResults {
		if strings.HasPrefix(filepath.Clean(path), prefix) {
			delete(s.validationResults, path)
		}
	}
	for path := range s.evidenceRecords {
		if strings.HasPrefix(filepath.Clean(path), prefix) {
			delete(s.evidenceRecords, path)
		}
	}
	for path := range s.coverageWorkItems {
		if strings.HasPrefix(filepath.Clean(path), prefix) {
			delete(s.coverageWorkItems, path)
		}
	}
	for path := range s.artifactContinuations {
		if strings.HasPrefix(filepath.Clean(path), prefix) {
			delete(s.artifactContinuations, path)
		}
	}
}

func observeWorkspaceScopedOrchestratorEvidence(store *workspaceScopedFileStore, workspaceID common.WorkspaceID) (map[string]uint64, error) {
	if store == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	counts := map[string]uint64{}
	for _, root := range store.workspaceRunRoots(workspaceID) {
		for _, surface := range []struct {
			name         string
			surface      string
			metadataOnly bool
		}{
			{name: workspaceScopedToolCallFile, surface: repository.OrchestratorPurgeSurfaceToolCalls, metadataOnly: true},
			{name: workspaceScopedPatchFile, surface: repository.OrchestratorPurgeSurfacePatches},
			{name: workspaceScopedValidationResultFile, surface: repository.OrchestratorPurgeSurfaceValidationResults},
			{name: workspaceScopedEvidenceRecordDir, surface: repository.OrchestratorPurgeSurfaceEvidenceRecords},
			{name: workspaceScopedCoverageWorkItemDir, surface: repository.OrchestratorPurgeSurfaceCoverageWorkItems},
		} {
			count, err := countWorkspaceScopedJSONItems(store.runPath(root, surface.name), surface.metadataOnly)
			if err != nil {
				return nil, err
			}
			counts[surface.surface] += count
		}
		artifactCounts, err := countWorkspaceScopedArtifactEvidence(store.runPath(root, workspaceScopedArtifactContinuationDir))
		if err != nil {
			return nil, err
		}
		for surface, count := range artifactCounts {
			counts[surface] += count
		}
	}
	return counts, nil
}

func orchestratorWorkspaceEvidenceCountsEmpty(counts map[string]uint64) bool {
	for _, count := range counts {
		if count != 0 {
			return false
		}
	}
	return true
}

func countWorkspaceScopedJSONItems(root string, metadataOnly bool) (uint64, error) {
	var count uint64
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, os.ErrNotExist) {
				return nil
			}
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if metadataOnly && entry.Name() != "metadata.json" {
			return nil
		}
		if !metadataOnly && filepath.Ext(entry.Name()) != ".json" {
			return common.ErrConflict
		}
		count++
		return nil
	})
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	return count, err
}

func countWorkspaceScopedArtifactEvidence(root string) (map[string]uint64, error) {
	counts := map[string]uint64{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, os.ErrNotExist) {
				return nil
			}
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		parent := filepath.Base(filepath.Dir(path))
		switch {
		case parent == "chunks" && filepath.Ext(entry.Name()) == ".json":
			counts[repository.OrchestratorPurgeSurfaceArtifactChunkMetadata]++
		case parent == "chunks" && filepath.Ext(entry.Name()) == ".content":
			counts[repository.OrchestratorPurgeSurfaceArtifactChunkContent]++
		case parent == "assembled" && entry.Name() == "metadata.json":
			counts[repository.OrchestratorPurgeSurfaceArtifactAssembledMetadata]++
		case parent == "assembled" && entry.Name() == "content":
			counts[repository.OrchestratorPurgeSurfaceArtifactAssembledContent]++
		case entry.Name() == "metadata.json":
			counts[repository.OrchestratorPurgeSurfaceArtifactContinuations]++
		default:
			return common.ErrConflict
		}
		return nil
	})
	if errors.Is(err, os.ErrNotExist) {
		return counts, nil
	}
	return counts, err
}

func newWorkspaceScopedFileStore(dataRoot string) *workspaceScopedFileStore {
	return &workspaceScopedFileStore{
		dataRoot:              dataRoot,
		runOwnerBinding:       repository.NewRunOwnerRepositoryBinding(),
		workspaces:            map[string]*fileRepo.WorkspaceRepository{},
		goals:                 map[string]*fileRepo.GoalRepository{},
		dods:                  map[string]*fileRepo.DoDRepository{},
		memoryItems:           map[string]*fileRepo.MemoryItemRepository{},
		memoryRecallTraces:    map[string]*fileRepo.MemoryRecallTraceRepository{},
		runs:                  map[string]*fileRepo.RunRepository{},
		steps:                 map[string]*fileRepo.StepRepository{},
		toolCalls:             map[string]*fileRepo.ToolCallRepository{},
		patches:               map[string]*fileRepo.PatchRepository{},
		validationResults:     map[string]*fileRepo.ValidationResultRepository{},
		runtimeCheckpoints:    map[string]*fileRepo.RuntimeCheckpointRepository{},
		evidenceRecords:       map[string]*fileRepo.EvidenceRecordRepository{},
		coverageWorkItems:     map[string]*fileRepo.CoverageWorkItemRepository{},
		artifactContinuations: map[string]*fileRepo.ArtifactContinuationRepository{},
		results:               map[string]*fileRepo.ResultRepository{},
		artifacts:             map[string]*fileRepo.ArtifactRepository{},
		eventRecords:          map[string]*fileRepo.EventRecordRepository{},
		tickets:               map[string]*fileRepo.DecisionTicketRepository{},
		resolutions:           map[string]*fileRepo.DecisionResolutionRepository{},
		approvals:             map[string]*fileRepo.ApprovalRepository{},
	}
}

func (s *workspaceScopedFileStore) workspaceRoot(workspaceID common.WorkspaceID) string {
	return filepath.Join(s.dataRoot, "internal", "workspaces", string(workspaceID))
}

func (s *workspaceScopedFileStore) workspaceSoulBindingsPath(workspaceID common.WorkspaceID) string {
	return filepath.Join(s.workspaceRoot(workspaceID), workspaceScopedConfigDir, workspaceScopedSoulBindingsFile)
}

func (s *workspaceScopedFileStore) runRoot(workspaceID common.WorkspaceID, runID common.RunID) string {
	return filepath.Join(s.dataRoot, "internal", "workspaces", string(workspaceID), "runs", string(runID))
}

func (s *workspaceScopedFileStore) workspaceRunGenerationPath(workspaceID common.WorkspaceID) string {
	return filepath.Join(s.dataRoot, "system", workspaceScopedRunGenerationDir, string(workspaceID)+".json")
}

type workspaceRunGenerationHead struct {
	Generation uint64                                  `json:"generation"`
	Retirement *runmodel.WorkspaceRetirementFence      `json:"retirement,omitempty"`
	Purge      *workspaceRunResourcePurgeJournalRecord `json:"purge,omitempty"`
}

const (
	workspaceRunResourcePurgePhasePending   workspaceRunResourcePurgePhase = "pending"
	workspaceRunResourcePurgePhaseCompleted workspaceRunResourcePurgePhase = "completed"
)

type workspaceRunResourcePurgePhase string

// workspaceRunResourcePurgeJournalRecord lives in the Run generation head.
// Pending is written before physical removal, so a crash after partial or
// complete removal still replays the original observations and proof.
type workspaceRunResourcePurgeJournalRecord struct {
	Phase   workspaceRunResourcePurgePhase              `json:"phase"`
	Receipt repository.RunWorkspaceResourcePurgeReceipt `json:"receipt"`
}

func (s *workspaceScopedFileStore) loadWorkspaceRunGenerationHead(workspaceID common.WorkspaceID) (workspaceRunGenerationHead, error) {
	data, err := os.ReadFile(s.workspaceRunGenerationPath(workspaceID))
	if err != nil {
		if os.IsNotExist(err) {
			return workspaceRunGenerationHead{}, nil
		}
		return workspaceRunGenerationHead{}, err
	}
	value := workspaceRunGenerationHead{}
	if err := decodeCurrentJSON(data, &value); err != nil {
		return workspaceRunGenerationHead{}, err
	}
	if value.Generation == 0 && value.Retirement == nil {
		return workspaceRunGenerationHead{}, common.ErrConflict
	}
	if value.Retirement != nil {
		fence := value.Retirement.Normalized()
		if fence.Validate() != nil || fence.WorkspaceID != workspaceID || fence.WorkspaceRunGeneration != value.Generation {
			return workspaceRunGenerationHead{}, common.ErrConflict
		}
		value.Retirement = &fence
	}
	if value.Purge != nil {
		if value.Retirement == nil || !value.Retirement.IsRetired() || value.Retirement.DataDispositionIntent != "purge" ||
			(value.Purge.Phase != workspaceRunResourcePurgePhasePending && value.Purge.Phase != workspaceRunResourcePurgePhaseCompleted) ||
			value.Purge.Receipt.ValidateFor(*value.Retirement) != nil {
			return workspaceRunGenerationHead{}, common.ErrConflict
		}
		value.Purge.Receipt = value.Purge.Receipt.Clone()
	}
	return value, nil
}

func (s *workspaceScopedFileStore) saveWorkspaceRunGenerationHead(workspaceID common.WorkspaceID, value workspaceRunGenerationHead) error {
	if workspaceID == "" || (value.Generation == 0 && value.Retirement == nil) {
		return common.ErrInvalidArgument
	}
	if value.Retirement != nil {
		fence := value.Retirement.Normalized()
		if fence.Validate() != nil || fence.WorkspaceID != workspaceID || fence.WorkspaceRunGeneration != value.Generation {
			return common.ErrInvalidArgument
		}
		value.Retirement = &fence
	}
	if value.Purge != nil {
		if value.Retirement == nil || !value.Retirement.IsRetired() || value.Retirement.DataDispositionIntent != "purge" ||
			(value.Purge.Phase != workspaceRunResourcePurgePhasePending && value.Purge.Phase != workspaceRunResourcePurgePhaseCompleted) ||
			value.Purge.Receipt.ValidateFor(*value.Retirement) != nil {
			return common.ErrInvalidArgument
		}
		value.Purge.Receipt = value.Purge.Receipt.Clone()
	}
	path := s.workspaceRunGenerationPath(workspaceID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".run-generation-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	keep := false
	defer func() {
		_ = temporary.Close()
		if !keep {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o644); err != nil {
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	keep = true
	return nil
}

func (s *workspaceScopedFileStore) systemPath(name string) string {
	return filepath.Join(s.dataRoot, "system", name)
}

func (s *workspaceScopedFileStore) workspaceIndexPath() string {
	return s.systemPath(workspaceScopedWorkspaceIndexFile)
}

func (s *workspaceScopedFileStore) workspaceRootRef(workspaceID common.WorkspaceID) string {
	return filepath.ToSlash(filepath.Join("internal", "workspaces", string(workspaceID)))
}

func (s *workspaceScopedFileStore) runPath(root string, name string) string {
	return filepath.Join(root, name)
}

func (s *workspaceScopedFileStore) allWorkspaceRoots() []string {
	matches, _ := filepath.Glob(filepath.Join(s.dataRoot, "internal", "workspaces", "*"))
	sort.Strings(matches)
	return matches
}

func (s *workspaceScopedFileStore) workspaceRootsForContext(ctx context.Context) []string {
	if workspaceID := common.RuntimeWorkspaceIDFromContext(ctx); workspaceID != "" {
		return []string{s.workspaceRoot(workspaceID)}
	}
	return s.allWorkspaceRoots()
}

func (s *workspaceScopedFileStore) allRunRoots() []string {
	matches, _ := filepath.Glob(filepath.Join(s.dataRoot, "internal", "workspaces", "*", "runs", "*"))
	sort.Strings(matches)
	return matches
}

func (s *workspaceScopedFileStore) workspaceRunRoots(workspaceID common.WorkspaceID) []string {
	if workspaceID == "" {
		return s.allRunRoots()
	}
	matches, _ := filepath.Glob(filepath.Join(s.dataRoot, "internal", "workspaces", string(workspaceID), "runs", "*"))
	sort.Strings(matches)
	return matches
}

func (s *workspaceScopedFileStore) runRootByID(ctx context.Context, runID common.RunID) (string, bool) {
	if runID == "" {
		return "", false
	}
	if workspaceID := common.RuntimeWorkspaceIDFromContext(ctx); workspaceID != "" {
		root := filepath.Join(s.dataRoot, "internal", "workspaces", string(workspaceID), "runs", string(runID))
		if _, err := os.Stat(filepath.Join(root, workspaceScopedRunFile)); err == nil {
			return root, true
		}
		return "", false
	}
	matches, _ := filepath.Glob(filepath.Join(s.dataRoot, "internal", "workspaces", "*", "runs", string(runID)))
	sort.Strings(matches)
	if len(matches) == 0 {
		return "", false
	}
	if len(matches) > 1 {
		return "", false
	}
	return matches[0], true
}

func (s *workspaceScopedFileStore) runRootsByRunID(ctx context.Context, runID common.RunID) ([]string, error) {
	if runID == "" {
		return nil, common.ErrInvalidArgument
	}
	if root, ok := s.runRootByID(ctx, runID); ok {
		return []string{root}, nil
	}
	if workspaceID := common.RuntimeWorkspaceIDFromContext(ctx); workspaceID != "" {
		return nil, common.ErrNotFound
	}
	matches, _ := filepath.Glob(filepath.Join(s.dataRoot, "internal", "workspaces", "*", "runs", string(runID)))
	sort.Strings(matches)
	if len(matches) == 0 {
		return nil, common.ErrNotFound
	}
	if len(matches) > 1 {
		return nil, common.ErrConflict
	}
	return matches, nil
}

func findUniqueInPaths[T any](paths []string, read func(string) (T, error)) (T, error) {
	var zero T
	var ret T
	found := false
	for _, path := range paths {
		item, err := read(path)
		if err == nil {
			if found {
				return zero, common.ErrConflict
			}
			ret = item
			found = true
			continue
		}
		if err != common.ErrNotFound {
			return zero, err
		}
	}
	if !found {
		return zero, common.ErrNotFound
	}
	return ret, nil
}

func runIDFromStepID(stepID common.StepID) common.RunID {
	prefix, _, ok := strings.Cut(string(stepID), "/")
	if !ok || prefix == "" {
		return ""
	}
	return common.RunID(prefix)
}

func (s *workspaceScopedFileStore) workspaceRepo(path string) *fileRepo.WorkspaceRepository {
	s.mu.Lock()
	defer s.mu.Unlock()
	repo, ok := s.workspaces[path]
	if !ok {
		repo = fileRepo.NewWorkspaceRepository(path)
		s.workspaces[path] = repo
	}
	return repo
}

func (s *workspaceScopedFileStore) goalRepo(path string) *fileRepo.GoalRepository {
	s.mu.Lock()
	defer s.mu.Unlock()
	repo, ok := s.goals[path]
	if !ok {
		repo = fileRepo.NewGoalRepository(path)
		s.goals[path] = repo
	}
	return repo
}

func (s *workspaceScopedFileStore) dodRepo(path string) *fileRepo.DoDRepository {
	s.mu.Lock()
	defer s.mu.Unlock()
	repo, ok := s.dods[path]
	if !ok {
		repo = fileRepo.NewDoDRepository(path)
		s.dods[path] = repo
	}
	return repo
}

func (s *workspaceScopedFileStore) memoryItemRepo(path string) *fileRepo.MemoryItemRepository {
	s.mu.Lock()
	defer s.mu.Unlock()
	repo, ok := s.memoryItems[path]
	if !ok {
		repo = fileRepo.NewMemoryItemRepository(path)
		s.memoryItems[path] = repo
	}
	return repo
}

func (s *workspaceScopedFileStore) memoryRecallTraceRepo(path string) *fileRepo.MemoryRecallTraceRepository {
	s.mu.Lock()
	defer s.mu.Unlock()
	repo, ok := s.memoryRecallTraces[path]
	if !ok {
		repo = fileRepo.NewMemoryRecallTraceRepository(path)
		s.memoryRecallTraces[path] = repo
	}
	return repo
}

func (s *workspaceScopedFileStore) runRepo(path string) *fileRepo.RunRepository {
	s.mu.Lock()
	defer s.mu.Unlock()
	repo, ok := s.runs[path]
	if !ok {
		repo = fileRepo.NewRunRepository(path)
		s.runs[path] = repo
	}
	return repo
}

func (s *workspaceScopedFileStore) stepRepo(path string) *fileRepo.StepRepository {
	s.mu.Lock()
	defer s.mu.Unlock()
	repo, ok := s.steps[path]
	if !ok {
		repo = fileRepo.NewStepRepository(path)
		s.steps[path] = repo
	}
	return repo
}

func (s *workspaceScopedFileStore) toolCallRepo(path string) *fileRepo.ToolCallRepository {
	s.mu.Lock()
	defer s.mu.Unlock()
	repo, ok := s.toolCalls[path]
	if !ok {
		repo = fileRepo.NewToolCallRepository(path)
		s.toolCalls[path] = repo
	}
	return repo
}

func (s *workspaceScopedFileStore) patchRepo(path string) *fileRepo.PatchRepository {
	s.mu.Lock()
	defer s.mu.Unlock()
	repo, ok := s.patches[path]
	if !ok {
		repo = fileRepo.NewPatchRepository(path)
		s.patches[path] = repo
	}
	return repo
}

func (s *workspaceScopedFileStore) validationResultRepo(path string) *fileRepo.ValidationResultRepository {
	s.mu.Lock()
	defer s.mu.Unlock()
	repo, ok := s.validationResults[path]
	if !ok {
		repo = fileRepo.NewValidationResultRepository(path)
		s.validationResults[path] = repo
	}
	return repo
}

func (s *workspaceScopedFileStore) runtimeCheckpointRepo(path string) *fileRepo.RuntimeCheckpointRepository {
	s.mu.Lock()
	defer s.mu.Unlock()
	repo, ok := s.runtimeCheckpoints[path]
	if !ok {
		repo = fileRepo.NewRuntimeCheckpointRepository(path)
		s.runtimeCheckpoints[path] = repo
	}
	return repo
}

func (s *workspaceScopedFileStore) evidenceRecordRepo(path string) *fileRepo.EvidenceRecordRepository {
	s.mu.Lock()
	defer s.mu.Unlock()
	repo, ok := s.evidenceRecords[path]
	if !ok {
		repo = fileRepo.NewEvidenceRecordRepository(path)
		s.evidenceRecords[path] = repo
	}
	return repo
}

func (s *workspaceScopedFileStore) coverageWorkItemRepo(path string) *fileRepo.CoverageWorkItemRepository {
	s.mu.Lock()
	defer s.mu.Unlock()
	repo, ok := s.coverageWorkItems[path]
	if !ok {
		repo = fileRepo.NewCoverageWorkItemRepository(path)
		s.coverageWorkItems[path] = repo
	}
	return repo
}

func (s *workspaceScopedFileStore) artifactContinuationRepo(path string) *fileRepo.ArtifactContinuationRepository {
	s.mu.Lock()
	defer s.mu.Unlock()
	repo, ok := s.artifactContinuations[path]
	if !ok {
		repo = fileRepo.NewArtifactContinuationRepository(path)
		s.artifactContinuations[path] = repo
	}
	return repo
}

func (s *workspaceScopedFileStore) resultRepo(path string) *fileRepo.ResultRepository {
	s.mu.Lock()
	defer s.mu.Unlock()
	repo, ok := s.results[path]
	if !ok {
		repo = fileRepo.NewResultRepository(path)
		s.results[path] = repo
	}
	return repo
}

func (s *workspaceScopedFileStore) artifactRepo(path string) *fileRepo.ArtifactRepository {
	s.mu.Lock()
	defer s.mu.Unlock()
	repo, ok := s.artifacts[path]
	if !ok {
		repo = fileRepo.NewArtifactRepository(path)
		s.artifacts[path] = repo
	}
	return repo
}

func (s *workspaceScopedFileStore) eventRecordRepo(path string) *fileRepo.EventRecordRepository {
	s.mu.Lock()
	defer s.mu.Unlock()
	repo, ok := s.eventRecords[path]
	if !ok {
		repo = fileRepo.NewEventRecordRepository(path)
		s.eventRecords[path] = repo
	}
	return repo
}

func (s *workspaceScopedFileStore) ticketRepo(path string) *fileRepo.DecisionTicketRepository {
	s.mu.Lock()
	defer s.mu.Unlock()
	repo, ok := s.tickets[path]
	if !ok {
		repo = fileRepo.NewDecisionTicketRepository(path)
		s.tickets[path] = repo
	}
	return repo
}

func (s *workspaceScopedFileStore) resolutionRepo(path string) *fileRepo.DecisionResolutionRepository {
	s.mu.Lock()
	defer s.mu.Unlock()
	repo, ok := s.resolutions[path]
	if !ok {
		repo = fileRepo.NewDecisionResolutionRepository(path)
		s.resolutions[path] = repo
	}
	return repo
}

func (s *workspaceScopedFileStore) approvalRepo(path string) *fileRepo.ApprovalRepository {
	s.mu.Lock()
	defer s.mu.Unlock()
	repo, ok := s.approvals[path]
	if !ok {
		repo = fileRepo.NewApprovalRepository(path)
		s.approvals[path] = repo
	}
	return repo
}

func (s *workspaceScopedFileStore) loadWorkspaceIndexLocked() (workspaceIndex, error) {
	path := s.workspaceIndexPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return workspaceIndex{}, nil
		}
		return workspaceIndex{}, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return workspaceIndex{}, common.ErrInvalidArgument
	}
	var idx workspaceIndex
	if err := decodeCurrentJSON(data, &idx); err != nil {
		return workspaceIndex{}, err
	}
	return cloneWorkspaceIndex(idx), nil
}

func (s *workspaceScopedFileStore) saveWorkspaceIndexLocked(idx workspaceIndex) error {
	sort.Slice(idx.Workspaces, func(i, j int) bool {
		return string(idx.Workspaces[i].WorkspaceID) < string(idx.Workspaces[j].WorkspaceID)
	})
	path := s.workspaceIndexPath()
	return writeWorkspaceScopedJSONFileAtomic(path, idx)
}

func writeWorkspaceScopedJSONFileAtomic(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".workspace-owner-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	keep := false
	defer func() {
		_ = temporary.Close()
		if !keep {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o644); err != nil {
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	keep = true
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func (s *workspaceScopedFileStore) upsertWorkspaceIndex(item *workspacemodel.Workspace) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	shared := workspaceIndexLock(s.workspaceIndexPath())
	shared.Lock()
	defer shared.Unlock()
	s.indexMu.Lock()
	defer s.indexMu.Unlock()

	idx, err := s.loadWorkspaceIndexLocked()
	if err != nil {
		return err
	}
	entry := workspaceIndexEntry{
		WorkspaceID:          item.ID,
		InternalRoot:         s.workspaceRootRef(item.ID),
		DeliverableDirectory: item.DeliverableDirectory,
		InternalDirectory:    item.InternalDirectory,
		State:                item.State,
		ActiveRunID:          item.ActiveRunID,
		UpdatedAt:            time.Now().UTC().Format(time.RFC3339Nano),
	}
	replaced := false
	for pos := range idx.Workspaces {
		if idx.Workspaces[pos].WorkspaceID == item.ID {
			idx.Workspaces[pos] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		idx.Workspaces = append(idx.Workspaces, entry)
	}
	return s.saveWorkspaceIndexLocked(idx)
}

func (s *workspaceScopedFileStore) listWorkspaceIndexEntries() ([]workspaceIndexEntry, error) {
	shared := workspaceIndexLock(s.workspaceIndexPath())
	shared.Lock()
	defer shared.Unlock()
	s.indexMu.Lock()
	defer s.indexMu.Unlock()

	idx, err := s.loadWorkspaceIndexLocked()
	if err != nil {
		return nil, err
	}
	sort.Slice(idx.Workspaces, func(i, j int) bool {
		return string(idx.Workspaces[i].WorkspaceID) < string(idx.Workspaces[j].WorkspaceID)
	})
	return append([]workspaceIndexEntry(nil), idx.Workspaces...), nil
}

func (s *workspaceScopedFileStore) deleteWorkspaceIndexEntry(workspaceID common.WorkspaceID) error {
	shared := workspaceIndexLock(s.workspaceIndexPath())
	shared.Lock()
	defer shared.Unlock()
	s.indexMu.Lock()
	defer s.indexMu.Unlock()

	idx, err := s.loadWorkspaceIndexLocked()
	if err != nil {
		return err
	}
	next := idx.Workspaces[:0]
	found := false
	for _, item := range idx.Workspaces {
		if item.WorkspaceID == workspaceID {
			found = true
			continue
		}
		next = append(next, item)
	}
	if !found {
		return common.ErrNotFound
	}
	idx.Workspaces = next
	return s.saveWorkspaceIndexLocked(idx)
}

func cloneWorkspaceIndex(idx workspaceIndex) workspaceIndex {
	return workspaceIndex{
		Workspaces: append([]workspaceIndexEntry(nil), idx.Workspaces...),
	}
}

func (s *workspaceScopedFileStore) cleanupWorkspaceIndexDeletionMaterial(current *workspacemodel.Workspace) error {
	if current == nil || current.ID == "" {
		return common.ErrInvalidArgument
	}
	shared := workspaceIndexLock(s.workspaceIndexPath())
	shared.Lock()
	defer shared.Unlock()
	s.indexMu.Lock()
	defer s.indexMu.Unlock()

	idx, err := s.loadWorkspaceIndexLocked()
	if err != nil {
		return err
	}
	position := -1
	for pos, entry := range idx.Workspaces {
		if entry.WorkspaceID != current.ID {
			continue
		}
		if position >= 0 || entry.InternalRoot != s.workspaceRootRef(current.ID) {
			return common.ErrConflict
		}
		position = pos
	}
	if position < 0 {
		return common.ErrConflict
	}
	idx.Workspaces[position].DeliverableDirectory = ""
	idx.Workspaces[position].InternalDirectory = ""
	idx.Workspaces[position].State = current.State
	idx.Workspaces[position].ActiveRunID = ""
	idx.Workspaces[position].UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.saveWorkspaceIndexLocked(idx); err != nil {
		return err
	}

	proof, err := s.loadWorkspaceIndexLocked()
	if err != nil {
		return err
	}
	matches := 0
	for _, entry := range proof.Workspaces {
		if entry.WorkspaceID != current.ID {
			continue
		}
		matches++
		if entry.InternalRoot != s.workspaceRootRef(current.ID) || entry.DeliverableDirectory != "" || entry.InternalDirectory != "" || entry.ActiveRunID != "" || entry.State != current.State {
			return common.ErrConflict
		}
	}
	if matches != 1 {
		return common.ErrConflict
	}
	return nil
}

type workspaceScopedWorkspaceRepository struct {
	store *workspaceScopedFileStore
}

func newWorkspaceScopedWorkspaceRepository(store *workspaceScopedFileStore) *workspaceScopedWorkspaceRepository {
	return &workspaceScopedWorkspaceRepository{store: store}
}

func (r *workspaceScopedWorkspaceRepository) Create(ctx context.Context, item *workspacemodel.Workspace) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	path := filepath.Join(r.store.workspaceRoot(item.ID), workspaceScopedWorkspaceFile)
	if err := r.store.workspaceRepo(path).Create(ctx, workspaceMetadata(item)); err != nil {
		return err
	}
	if err := r.store.saveWorkspaceSoulBindings(item); err != nil {
		return err
	}
	return r.store.upsertWorkspaceIndex(item)
}

func (r *workspaceScopedWorkspaceRepository) Get(ctx context.Context, id common.WorkspaceID) (*workspacemodel.Workspace, error) {
	if id == "" {
		return nil, common.ErrInvalidArgument
	}
	path := filepath.Join(r.store.workspaceRoot(id), workspaceScopedWorkspaceFile)
	item, err := r.store.workspaceRepo(path).Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return r.store.attachWorkspaceSoulBindings(item)
}

func (r *workspaceScopedWorkspaceRepository) List(ctx context.Context) ([]*workspacemodel.Workspace, error) {
	entries, err := r.store.listWorkspaceIndexEntries()
	if err != nil {
		return nil, err
	}
	ret := []*workspacemodel.Workspace{}
	for _, entry := range entries {
		if entry.WorkspaceID == "" {
			continue
		}
		item, err := r.Get(ctx, entry.WorkspaceID)
		if err == common.ErrNotFound {
			continue
		}
		if err != nil {
			return nil, err
		}
		ret = append(ret, item)
	}
	sortWorkspaces(ret)
	return ret, nil
}

func (r *workspaceScopedWorkspaceRepository) Update(ctx context.Context, item *workspacemodel.Workspace) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	path := filepath.Join(r.store.workspaceRoot(item.ID), workspaceScopedWorkspaceFile)
	if err := r.store.workspaceRepo(path).Update(ctx, workspaceMetadata(item)); err != nil {
		return err
	}
	if err := r.store.saveWorkspaceSoulBindings(item); err != nil {
		return err
	}
	return r.store.upsertWorkspaceIndex(item)
}

func (r *workspaceScopedWorkspaceRepository) CompareAndSwap(ctx context.Context, item *workspacemodel.Workspace, expectedRevision uint64) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	path := filepath.Join(r.store.workspaceRoot(item.ID), workspaceScopedWorkspaceFile)
	if err := r.store.workspaceRepo(path).CompareAndSwap(ctx, workspaceMetadata(item), expectedRevision); err != nil {
		return err
	}
	item.Revision = expectedRevision + 1
	if err := r.store.saveWorkspaceSoulBindings(item); err != nil {
		return err
	}
	return r.store.upsertWorkspaceIndex(item)
}

func (r *workspaceScopedWorkspaceRepository) CleanupDeletionMaterial(ctx context.Context, scope repository.WorkspaceDeletionMaterialCleanupScope) error {
	if r == nil || r.store == nil {
		return common.ErrInvalidArgument
	}
	if err := repository.ValidateWorkspaceDeletionMaterialCleanupContext(ctx, scope); err != nil {
		return err
	}
	path := filepath.Join(r.store.workspaceRoot(scope.WorkspaceID), workspaceScopedWorkspaceFile)
	current, err := r.store.workspaceRepo(path).Get(ctx, scope.WorkspaceID)
	if errors.Is(err, common.ErrNotFound) {
		return common.ErrConflict
	}
	if err != nil {
		return err
	}
	if err := repository.ValidateWorkspaceDeletionMaterialCleanupCurrent(current, scope); err != nil {
		return err
	}
	if err := r.store.saveWorkspaceSoulBindings(&workspacemodel.Workspace{ID: scope.WorkspaceID}); err != nil {
		return err
	}
	bindings, err := r.store.loadWorkspaceSoulBindings(scope.WorkspaceID)
	if err != nil {
		return err
	}
	if bindings.DefaultSoulID != "" || len(bindings.Bindings) != 0 {
		return common.ErrConflict
	}
	return r.store.cleanupWorkspaceIndexDeletionMaterial(current)
}

func (r *workspaceScopedWorkspaceRepository) Delete(ctx context.Context, id common.WorkspaceID) error {
	if id == "" {
		return common.ErrInvalidArgument
	}
	path := filepath.Join(r.store.workspaceRoot(id), workspaceScopedWorkspaceFile)
	if _, err := r.store.workspaceRepo(path).Get(ctx, id); err != nil {
		return err
	}
	if err := r.store.workspaceRepo(path).Delete(ctx, id); err != nil {
		return err
	}
	if err := os.Remove(r.store.workspaceSoulBindingsPath(id)); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := r.store.deleteWorkspaceIndexEntry(id); err != nil && err != common.ErrNotFound {
		return err
	}
	return nil
}

func workspaceMetadata(item *workspacemodel.Workspace) *workspacemodel.Workspace {
	if item == nil {
		return nil
	}
	cloned := *item
	cloned.DefaultSoulID = ""
	cloned.SoulBindings = nil
	return &cloned
}

func (s *workspaceScopedFileStore) attachWorkspaceSoulBindings(item *workspacemodel.Workspace) (*workspacemodel.Workspace, error) {
	if item == nil {
		return nil, nil
	}
	bindings, err := s.loadWorkspaceSoulBindings(item.ID)
	if err != nil {
		return nil, err
	}
	cloned := *item
	cloned.DefaultSoulID = bindings.DefaultSoulID
	if len(bindings.Bindings) > 0 {
		cloned.SoulBindings = map[string]common.SoulID{}
		for key, value := range bindings.Bindings {
			cloned.SoulBindings[key] = value
		}
	} else {
		cloned.SoulBindings = nil
	}
	return &cloned, nil
}

func (s *workspaceScopedFileStore) loadWorkspaceSoulBindings(workspaceID common.WorkspaceID) (workspaceSoulBindingsFile, error) {
	if workspaceID == "" {
		return workspaceSoulBindingsFile{}, common.ErrInvalidArgument
	}
	data, err := os.ReadFile(s.workspaceSoulBindingsPath(workspaceID))
	if os.IsNotExist(err) {
		return workspaceSoulBindingsFile{}, nil
	}
	if err != nil {
		return workspaceSoulBindingsFile{}, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return workspaceSoulBindingsFile{}, common.ErrInvalidArgument
	}
	var ret workspaceSoulBindingsFile
	if err := decodeCurrentJSON(data, &ret); err != nil {
		return workspaceSoulBindingsFile{}, err
	}
	return ret, nil
}

func (s *workspaceScopedFileStore) saveWorkspaceSoulBindings(item *workspacemodel.Workspace) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	payload := workspaceSoulBindingsFile{
		DefaultSoulID: item.DefaultSoulID,
	}
	if len(item.SoulBindings) > 0 {
		payload.Bindings = map[string]common.SoulID{}
		for key, value := range item.SoulBindings {
			payload.Bindings[key] = value
		}
	}
	path := s.workspaceSoulBindingsPath(item.ID)
	return writeWorkspaceScopedJSONFileAtomic(path, payload)
}

type workspaceScopedGoalRepository struct {
	store *workspaceScopedFileStore
}

func newWorkspaceScopedGoalRepository(store *workspaceScopedFileStore) *workspaceScopedGoalRepository {
	return &workspaceScopedGoalRepository{store: store}
}

func (r *workspaceScopedGoalRepository) Create(ctx context.Context, item *workspacemodel.Goal) error {
	if item == nil || item.WorkspaceID == "" || item.ID == "" {
		return common.ErrInvalidArgument
	}
	return r.store.goalRepo(filepath.Join(r.store.workspaceRoot(item.WorkspaceID), workspaceScopedGoalFile)).Create(ctx, item)
}

func (r *workspaceScopedGoalRepository) Get(ctx context.Context, id common.GoalID) (*workspacemodel.Goal, error) {
	if id == "" {
		return nil, common.ErrInvalidArgument
	}
	return findUniqueInPaths(r.store.workspaceRootsForContext(ctx), func(root string) (*workspacemodel.Goal, error) {
		return r.store.goalRepo(filepath.Join(root, workspaceScopedGoalFile)).Get(ctx, id)
	})
}

func (r *workspaceScopedGoalRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*workspacemodel.Goal, error) {
	if workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	return r.store.goalRepo(filepath.Join(r.store.workspaceRoot(workspaceID), workspaceScopedGoalFile)).ListByWorkspaceID(ctx, workspaceID)
}

func (r *workspaceScopedGoalRepository) Update(ctx context.Context, item *workspacemodel.Goal) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if item.WorkspaceID != "" {
		err := r.store.goalRepo(filepath.Join(r.store.workspaceRoot(item.WorkspaceID), workspaceScopedGoalFile)).Update(ctx, item)
		if err == nil || err != common.ErrNotFound {
			return err
		}
	}
	current, err := r.Get(ctx, item.ID)
	if err != nil {
		return err
	}
	item.WorkspaceID = current.WorkspaceID
	return r.store.goalRepo(filepath.Join(r.store.workspaceRoot(item.WorkspaceID), workspaceScopedGoalFile)).Update(ctx, item)
}

func (r *workspaceScopedGoalRepository) Delete(ctx context.Context, id common.GoalID) error {
	if id == "" {
		return common.ErrInvalidArgument
	}
	current, err := r.Get(ctx, id)
	if err != nil {
		return err
	}
	return r.store.goalRepo(filepath.Join(r.store.workspaceRoot(current.WorkspaceID), workspaceScopedGoalFile)).Delete(ctx, id)
}

type workspaceScopedDoDRepository struct {
	store *workspaceScopedFileStore
}

func newWorkspaceScopedDoDRepository(store *workspaceScopedFileStore) *workspaceScopedDoDRepository {
	return &workspaceScopedDoDRepository{store: store}
}

func (r *workspaceScopedDoDRepository) Create(ctx context.Context, item *workspacemodel.DoD) error {
	if item == nil || item.WorkspaceID == "" || item.ID == "" {
		return common.ErrInvalidArgument
	}
	return r.store.dodRepo(filepath.Join(r.store.workspaceRoot(item.WorkspaceID), workspaceScopedDoDFile)).Create(ctx, item)
}

func (r *workspaceScopedDoDRepository) Get(ctx context.Context, id common.DoDID) (*workspacemodel.DoD, error) {
	if id == "" {
		return nil, common.ErrInvalidArgument
	}
	return findUniqueInPaths(r.store.workspaceRootsForContext(ctx), func(root string) (*workspacemodel.DoD, error) {
		return r.store.dodRepo(filepath.Join(root, workspaceScopedDoDFile)).Get(ctx, id)
	})
}

func (r *workspaceScopedDoDRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*workspacemodel.DoD, error) {
	if workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	return r.store.dodRepo(filepath.Join(r.store.workspaceRoot(workspaceID), workspaceScopedDoDFile)).ListByWorkspaceID(ctx, workspaceID)
}

func (r *workspaceScopedDoDRepository) Update(ctx context.Context, item *workspacemodel.DoD) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if item.WorkspaceID != "" {
		err := r.store.dodRepo(filepath.Join(r.store.workspaceRoot(item.WorkspaceID), workspaceScopedDoDFile)).Update(ctx, item)
		if err == nil || err != common.ErrNotFound {
			return err
		}
	}
	current, err := r.Get(ctx, item.ID)
	if err != nil {
		return err
	}
	item.WorkspaceID = current.WorkspaceID
	return r.store.dodRepo(filepath.Join(r.store.workspaceRoot(item.WorkspaceID), workspaceScopedDoDFile)).Update(ctx, item)
}

func (r *workspaceScopedDoDRepository) Delete(ctx context.Context, id common.DoDID) error {
	if id == "" {
		return common.ErrInvalidArgument
	}
	current, err := r.Get(ctx, id)
	if err != nil {
		return err
	}
	return r.store.dodRepo(filepath.Join(r.store.workspaceRoot(current.WorkspaceID), workspaceScopedDoDFile)).Delete(ctx, id)
}

type workspaceScopedMemoryItemRepository struct {
	store *workspaceScopedFileStore
}

func newWorkspaceScopedMemoryItemRepository(store *workspaceScopedFileStore) *workspaceScopedMemoryItemRepository {
	return &workspaceScopedMemoryItemRepository{store: store}
}

func (r *workspaceScopedMemoryItemRepository) Create(ctx context.Context, item *memorymodel.MemoryItem) error {
	if item == nil || item.WorkspaceID == "" || item.ID == "" {
		return common.ErrInvalidArgument
	}
	return withWorkspaceDeletionMutation(ctx, r.store.dataRoot, "memory", item.WorkspaceID, func(guarded context.Context) error {
		return r.store.memoryItemRepo(filepath.Join(r.store.workspaceRoot(item.WorkspaceID), workspaceScopedMemoryItemFile)).Create(guarded, item)
	})
}

func (r *workspaceScopedMemoryItemRepository) Get(ctx context.Context, id common.MemoryID) (*memorymodel.MemoryItem, error) {
	if id == "" {
		return nil, common.ErrInvalidArgument
	}
	return findUniqueInPaths(r.store.workspaceRootsForContext(ctx), func(root string) (*memorymodel.MemoryItem, error) {
		return r.store.memoryItemRepo(filepath.Join(root, workspaceScopedMemoryItemFile)).Get(ctx, id)
	})
}

func (r *workspaceScopedMemoryItemRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*memorymodel.MemoryItem, error) {
	if workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	return r.store.memoryItemRepo(filepath.Join(r.store.workspaceRoot(workspaceID), workspaceScopedMemoryItemFile)).ListByWorkspaceID(ctx, workspaceID)
}

func (r *workspaceScopedMemoryItemRepository) Search(ctx context.Context, query workorchquerycontract.MemorySearchQuery) ([]*memorymodel.MemoryItem, error) {
	if query.WorkspaceID != "" {
		return r.store.memoryItemRepo(filepath.Join(r.store.workspaceRoot(query.WorkspaceID), workspaceScopedMemoryItemFile)).Search(ctx, query)
	}
	queryWithoutLimit := query
	queryWithoutLimit.Limit = 0
	ret := []*memorymodel.MemoryItem{}
	for _, root := range r.store.allWorkspaceRoots() {
		items, err := r.store.memoryItemRepo(filepath.Join(root, workspaceScopedMemoryItemFile)).Search(ctx, queryWithoutLimit)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
	}
	sortMemoryItems(ret, query)
	if query.Limit > 0 && len(ret) > query.Limit {
		ret = ret[:query.Limit]
	}
	return nilIfEmpty(ret), nil
}

func (r *workspaceScopedMemoryItemRepository) Update(ctx context.Context, item *memorymodel.MemoryItem) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if item.WorkspaceID != "" {
		err := withWorkspaceDeletionMutation(ctx, r.store.dataRoot, "memory", item.WorkspaceID, func(guarded context.Context) error {
			return r.store.memoryItemRepo(filepath.Join(r.store.workspaceRoot(item.WorkspaceID), workspaceScopedMemoryItemFile)).Update(guarded, item)
		})
		if err == nil || err != common.ErrNotFound {
			return err
		}
		if fenced, fenceErr := workspaceDeletionExists(r.store.dataRoot, "memory", item.WorkspaceID); fenceErr != nil || fenced {
			if fenceErr != nil {
				return fenceErr
			}
			return common.ErrNotFound
		}
	}
	current, err := r.Get(ctx, item.ID)
	if err != nil {
		return err
	}
	item.WorkspaceID = current.WorkspaceID
	return withWorkspaceDeletionMutation(ctx, r.store.dataRoot, "memory", item.WorkspaceID, func(guarded context.Context) error {
		return r.store.memoryItemRepo(filepath.Join(r.store.workspaceRoot(item.WorkspaceID), workspaceScopedMemoryItemFile)).Update(guarded, item)
	})
}

func (r *workspaceScopedMemoryItemRepository) Delete(ctx context.Context, id common.MemoryID) error {
	if id == "" {
		return common.ErrInvalidArgument
	}
	current, err := r.Get(ctx, id)
	if err != nil {
		return err
	}
	return withWorkspaceDeletionMutation(ctx, r.store.dataRoot, "memory", current.WorkspaceID, func(guarded context.Context) error {
		return r.store.memoryItemRepo(filepath.Join(r.store.workspaceRoot(current.WorkspaceID), workspaceScopedMemoryItemFile)).Delete(guarded, id)
	})
}

type workspaceScopedMemoryRecallTraceRepository struct {
	store *workspaceScopedFileStore
}

func newWorkspaceScopedMemoryRecallTraceRepository(store *workspaceScopedFileStore) *workspaceScopedMemoryRecallTraceRepository {
	return &workspaceScopedMemoryRecallTraceRepository{store: store}
}

func (r *workspaceScopedMemoryRecallTraceRepository) Create(ctx context.Context, item *memorymodel.MemoryRecallTrace) error {
	if item == nil || item.WorkspaceID == "" || item.ID == "" {
		return common.ErrInvalidArgument
	}
	return withWorkspaceDeletionMutation(ctx, r.store.dataRoot, "memory", item.WorkspaceID, func(guarded context.Context) error {
		return r.store.memoryRecallTraceRepo(filepath.Join(r.store.workspaceRoot(item.WorkspaceID), workspaceScopedMemoryRecallTraceFile)).Create(guarded, item)
	})
}

func (r *workspaceScopedMemoryRecallTraceRepository) Upsert(ctx context.Context, item *memorymodel.MemoryRecallTrace) error {
	if item == nil || item.WorkspaceID == "" || item.ID == "" {
		return common.ErrInvalidArgument
	}
	return withWorkspaceDeletionMutation(ctx, r.store.dataRoot, "memory", item.WorkspaceID, func(guarded context.Context) error {
		return r.store.memoryRecallTraceRepo(filepath.Join(r.store.workspaceRoot(item.WorkspaceID), workspaceScopedMemoryRecallTraceFile)).Upsert(guarded, item)
	})
}

func (r *workspaceScopedMemoryRecallTraceRepository) Get(ctx context.Context, id common.MemoryRecallTraceID) (*memorymodel.MemoryRecallTrace, error) {
	if id == "" {
		return nil, common.ErrInvalidArgument
	}
	return findUniqueInPaths(r.store.workspaceRootsForContext(ctx), func(root string) (*memorymodel.MemoryRecallTrace, error) {
		return r.store.memoryRecallTraceRepo(filepath.Join(root, workspaceScopedMemoryRecallTraceFile)).Get(ctx, id)
	})
}

func (r *workspaceScopedMemoryRecallTraceRepository) List(ctx context.Context, query workorchquerycontract.MemoryRecallTraceQuery) ([]*memorymodel.MemoryRecallTrace, error) {
	if query.WorkspaceID != "" {
		return r.store.memoryRecallTraceRepo(filepath.Join(r.store.workspaceRoot(query.WorkspaceID), workspaceScopedMemoryRecallTraceFile)).List(ctx, query)
	}
	ret := []*memorymodel.MemoryRecallTrace{}
	repositoryQuery := query
	repositoryQuery.CursorAfterID = ""
	repositoryQuery.Limit = 0
	for _, root := range r.store.allWorkspaceRoots() {
		items, err := r.store.memoryRecallTraceRepo(filepath.Join(root, workspaceScopedMemoryRecallTraceFile)).List(ctx, repositoryQuery)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
	}
	sortMemoryRecallTraces(ret)
	if query.CursorAfterID != "" {
		start := -1
		for idx, item := range ret {
			if item != nil && item.ID == query.CursorAfterID {
				start = idx + 1
				break
			}
		}
		if start < 0 {
			return nil, common.ErrInvalidArgument
		}
		ret = ret[start:]
	}
	if query.Limit > 0 && len(ret) > query.Limit {
		ret = ret[:query.Limit]
	}
	return nilIfEmpty(ret), nil
}

func (r *workspaceScopedMemoryRecallTraceRepository) Delete(ctx context.Context, id common.MemoryRecallTraceID) error {
	if id == "" {
		return common.ErrInvalidArgument
	}
	current, err := r.Get(ctx, id)
	if err != nil {
		return err
	}
	return withWorkspaceDeletionMutation(ctx, r.store.dataRoot, "memory", current.WorkspaceID, func(guarded context.Context) error {
		return r.store.memoryRecallTraceRepo(filepath.Join(r.store.workspaceRoot(current.WorkspaceID), workspaceScopedMemoryRecallTraceFile)).Delete(guarded, id)
	})
}

type workspaceScopedRunRepository struct {
	store *workspaceScopedFileStore
}

func newWorkspaceScopedRunRepository(store *workspaceScopedFileStore) *workspaceScopedRunRepository {
	return &workspaceScopedRunRepository{store: store}
}

func (r *workspaceScopedRunRepository) RunOwnerRepositoryBinding() repository.RunOwnerRepositoryBinding {
	if r == nil || r.store == nil {
		return repository.RunOwnerRepositoryBinding{}
	}
	return r.store.runOwnerBinding
}

func (r *workspaceScopedRunRepository) Create(ctx context.Context, item *runmodel.Run) error {
	return r.create(ctx, item, false)
}

func (r *workspaceScopedRunRepository) CreateNextWorkspaceRun(ctx context.Context, item *runmodel.Run) error {
	return r.create(ctx, item, true)
}

func (r *workspaceScopedRunRepository) create(ctx context.Context, item *runmodel.Run, allocateID bool) error {
	if item == nil || item.WorkspaceID == "" || (allocateID && item.ID != "") || (!allocateID && item.ID == "") {
		return common.ErrInvalidArgument
	}
	headLock := workspaceRunGenerationLock(r.store.workspaceRunGenerationPath(item.WorkspaceID))
	headLock.Lock()
	defer headLock.Unlock()
	head, err := r.store.loadWorkspaceRunGenerationHead(item.WorkspaceID)
	if err != nil {
		return err
	}
	if head.Retirement != nil {
		return common.ErrConflict
	}
	watermark := head.Generation
	existing, err := r.ListByWorkspaceID(ctx, item.WorkspaceID)
	if err != nil {
		return err
	}
	seen := map[uint64]struct{}{}
	occupiedIDs := map[common.RunID]struct{}{}
	for _, current := range existing {
		if current == nil || current.WorkspaceRunGeneration == 0 || current.WorkspaceRunGeneration > watermark {
			return common.ErrConflict
		}
		if _, duplicate := seen[current.WorkspaceRunGeneration]; duplicate {
			return common.ErrConflict
		}
		seen[current.WorkspaceRunGeneration] = struct{}{}
		occupiedIDs[current.ID] = struct{}{}
		if !workspaceScopedRunStateIsTerminal(item.State) && !workspaceScopedRunStateIsTerminal(current.State) {
			return common.ErrConflict
		}
	}
	if !allocateID {
		if _, duplicate := occupiedIDs[item.ID]; duplicate {
			return common.ErrConflict
		}
	}
	if item.WorkspaceRunGeneration != 0 {
		return common.ErrInvalidArgument
	}
	next := watermark
	for {
		if next == ^uint64(0) {
			return common.ErrConflict
		}
		next++
		if !allocateID {
			break
		}
		candidate := runmodel.WorkspaceRunID(next)
		if _, occupied := occupiedIDs[candidate]; occupied {
			continue
		}
		break
	}
	// Persist the shared Workspace watermark first. A later Run write failure
	// may skip a generation but cannot reuse one after response loss, restart,
	// individual Run deletion, or workspace-directory purge.
	head.Generation = next
	if err := r.store.saveWorkspaceRunGenerationHead(item.WorkspaceID, head); err != nil {
		return err
	}
	targetRunID := item.ID
	if allocateID {
		targetRunID = runmodel.WorkspaceRunID(next)
	}
	repo := r.store.runRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, targetRunID), workspaceScopedRunFile))
	var createErr error
	if allocateID {
		createErr = repo.CreateNextWorkspaceRunWithGeneration(ctx, item, next)
	} else {
		createErr = repo.CreateWithWorkspaceRunGeneration(ctx, item, next)
	}
	if createErr != nil {
		item.WorkspaceRunGeneration = 0
		if allocateID {
			item.ID = ""
		}
		return createErr
	}
	return nil
}

func workspaceScopedRunStateIsTerminal(state common.RunState) bool {
	switch state {
	case common.RunStateCompleted, common.RunStateFailed, common.RunStateCancelled, common.RunStateSuperseded:
		return true
	default:
		return false
	}
}

func (r *workspaceScopedRunRepository) Get(ctx context.Context, id common.RunID) (*runmodel.Run, error) {
	if id == "" {
		return nil, common.ErrInvalidArgument
	}
	if root, ok := r.store.runRootByID(ctx, id); ok {
		item, err := r.store.runRepo(r.store.runPath(root, workspaceScopedRunFile)).Get(ctx, id)
		if err == nil {
			return item, nil
		}
		if err != common.ErrNotFound {
			return nil, err
		}
	}
	roots, err := r.store.runRootsByRunID(ctx, id)
	if err != nil {
		return nil, err
	}
	for _, root := range roots {
		item, err := r.store.runRepo(r.store.runPath(root, workspaceScopedRunFile)).Get(ctx, id)
		if err == nil {
			return item, nil
		}
		if err != common.ErrNotFound {
			return nil, err
		}
	}
	return nil, common.ErrNotFound
}

func (r *workspaceScopedRunRepository) GetByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID, id common.RunID) (*runmodel.Run, error) {
	if workspaceID == "" || id == "" {
		return nil, common.ErrInvalidArgument
	}
	return r.store.runRepo(r.store.runPath(r.store.runRoot(workspaceID, id), workspaceScopedRunFile)).GetByWorkspaceID(ctx, workspaceID, id)
}

func (r *workspaceScopedRunRepository) List(ctx context.Context) ([]*runmodel.Run, error) {
	ret := []*runmodel.Run{}
	for _, root := range r.store.allRunRoots() {
		items, err := r.store.runRepo(r.store.runPath(root, workspaceScopedRunFile)).List(ctx)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
	}
	sortRuns(ret)
	return ret, nil
}

func (r *workspaceScopedRunRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*runmodel.Run, error) {
	ret := []*runmodel.Run{}
	for _, root := range r.store.workspaceRunRoots(workspaceID) {
		items, err := r.store.runRepo(r.store.runPath(root, workspaceScopedRunFile)).ListByWorkspaceID(ctx, workspaceID)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
	}
	sortRuns(ret)
	return ret, nil
}

func (r *workspaceScopedRunRepository) Update(ctx context.Context, item *runmodel.Run) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if item.WorkspaceID != "" {
		headLock := workspaceRunGenerationLock(r.store.workspaceRunGenerationPath(item.WorkspaceID))
		headLock.Lock()
		head, headErr := r.store.loadWorkspaceRunGenerationHead(item.WorkspaceID)
		if headErr != nil {
			headLock.Unlock()
			return headErr
		}
		if head.Retirement != nil {
			headLock.Unlock()
			return common.ErrConflict
		}
		err := r.store.runRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.ID), workspaceScopedRunFile)).Update(ctx, item)
		headLock.Unlock()
		if err == nil || err != common.ErrNotFound {
			return err
		}
	}
	current, err := r.Get(ctx, item.ID)
	if err != nil {
		return err
	}
	item.WorkspaceID = current.WorkspaceID
	headLock := workspaceRunGenerationLock(r.store.workspaceRunGenerationPath(item.WorkspaceID))
	headLock.Lock()
	defer headLock.Unlock()
	head, err := r.store.loadWorkspaceRunGenerationHead(item.WorkspaceID)
	if err != nil {
		return err
	}
	if head.Retirement != nil {
		return common.ErrConflict
	}
	return r.store.runRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.ID), workspaceScopedRunFile)).Update(ctx, item)
}

func (r *workspaceScopedRunRepository) CompareAndSwap(ctx context.Context, item *runmodel.Run, expectedRevision uint64) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if item.WorkspaceID != "" {
		headLock := workspaceRunGenerationLock(r.store.workspaceRunGenerationPath(item.WorkspaceID))
		headLock.Lock()
		head, headErr := r.store.loadWorkspaceRunGenerationHead(item.WorkspaceID)
		if headErr != nil {
			headLock.Unlock()
			return headErr
		}
		if head.Retirement != nil {
			headLock.Unlock()
			return common.ErrConflict
		}
		err := r.store.runRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.ID), workspaceScopedRunFile)).CompareAndSwap(ctx, item, expectedRevision)
		headLock.Unlock()
		if err == nil || err != common.ErrNotFound {
			return err
		}
	}
	current, err := r.Get(ctx, item.ID)
	if err != nil {
		return err
	}
	if current.Revision != expectedRevision {
		return common.ErrConflict
	}
	item.WorkspaceID = current.WorkspaceID
	headLock := workspaceRunGenerationLock(r.store.workspaceRunGenerationPath(item.WorkspaceID))
	headLock.Lock()
	defer headLock.Unlock()
	head, err := r.store.loadWorkspaceRunGenerationHead(item.WorkspaceID)
	if err != nil {
		return err
	}
	if head.Retirement != nil {
		return common.ErrConflict
	}
	return r.store.runRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.ID), workspaceScopedRunFile)).CompareAndSwap(ctx, item, expectedRevision)
}

func (r *workspaceScopedRunRepository) CompareAndSwapRuntimeState(ctx context.Context, item *runmodel.Run, expectedRevision, expectedRuntimeStateRevision uint64) error {
	if r == nil || item == nil || item.ID == "" || item.WorkspaceID == "" || expectedRevision == 0 {
		return common.ErrInvalidArgument
	}
	if scoped := common.RuntimeWorkspaceIDFromContext(ctx); scoped != "" && scoped != item.WorkspaceID {
		return common.ErrConflict
	}
	headLock := workspaceRunGenerationLock(r.store.workspaceRunGenerationPath(item.WorkspaceID))
	headLock.Lock()
	defer headLock.Unlock()
	head, err := r.store.loadWorkspaceRunGenerationHead(item.WorkspaceID)
	if err != nil {
		return err
	}
	repo := r.store.runRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.ID), workspaceScopedRunFile))
	current, err := repo.GetByWorkspaceID(ctx, item.WorkspaceID, item.ID)
	if err != nil {
		return err
	}
	if current.Revision != expectedRevision {
		return common.ErrConflict
	}
	if head.Retirement != nil {
		fence := head.Retirement.Normalized()
		if fence.Validate() != nil || fence.IsRetired() || !workspaceScopedRunStateIsTerminal(current.State) {
			return common.ErrConflict
		}
	}
	if err := runmodel.ValidateOpaqueRuntimeStateCAS(current, item, expectedRuntimeStateRevision); err != nil {
		return err
	}
	return repo.CompareAndSwap(ctx, item, expectedRevision)
}

func (r *workspaceScopedRunRepository) Delete(ctx context.Context, id common.RunID) error {
	if id == "" {
		return common.ErrInvalidArgument
	}
	current, err := r.Get(ctx, id)
	if err != nil {
		return err
	}
	headLock := workspaceRunGenerationLock(r.store.workspaceRunGenerationPath(current.WorkspaceID))
	headLock.Lock()
	defer headLock.Unlock()
	head, err := r.store.loadWorkspaceRunGenerationHead(current.WorkspaceID)
	if err != nil {
		return err
	}
	if head.Retirement != nil {
		return common.ErrConflict
	}
	return r.store.runRepo(r.store.runPath(r.store.runRoot(current.WorkspaceID, current.ID), workspaceScopedRunFile)).Delete(ctx, id)
}

func (r *workspaceScopedRunRepository) PrepareWorkspaceRetirement(ctx context.Context, intent runmodel.WorkspaceRetirementIntent) (*runmodel.WorkspaceRetirementFence, error) {
	intent = intent.Normalized()
	if err := intent.Validate(); err != nil {
		return nil, common.ErrInvalidArgument
	}
	if scoped := common.RuntimeWorkspaceIDFromContext(ctx); scoped != "" && scoped != intent.WorkspaceID {
		return nil, common.ErrConflict
	}
	headLock := workspaceRunGenerationLock(r.store.workspaceRunGenerationPath(intent.WorkspaceID))
	headLock.Lock()
	defer headLock.Unlock()
	head, err := r.store.loadWorkspaceRunGenerationHead(intent.WorkspaceID)
	if err != nil {
		return nil, err
	}
	if head.Retirement != nil {
		current := head.Retirement.Normalized()
		if current.Intent() != intent {
			return nil, common.ErrConflict
		}
		return &current, nil
	}
	items, err := r.ListByWorkspaceID(ctx, intent.WorkspaceID)
	if err != nil {
		return nil, err
	}
	if err := runmodel.WorkspaceRetirementEligible(items, intent.WorkspaceID); err != nil {
		return nil, err
	}
	fence := runmodel.WorkspaceRetirementFence{
		WorkspaceID:            intent.WorkspaceID,
		DeletionID:             intent.DeletionID,
		DeletionGeneration:     intent.DeletionGeneration,
		DataDispositionIntent:  intent.DataDispositionIntent,
		WorkspaceIntentHash:    intent.WorkspaceIntentHash,
		Phase:                  runmodel.WorkspaceRetirementPhaseDraining,
		FenceRevision:          1,
		WorkspaceRunGeneration: head.Generation,
	}
	head.Retirement = &fence
	if err := r.store.saveWorkspaceRunGenerationHead(intent.WorkspaceID, head); err != nil {
		return nil, err
	}
	ret := fence
	return &ret, nil
}

func (r *workspaceScopedRunRepository) FinalizeWorkspaceRetirement(ctx context.Context, token deletioncontract.WorkspaceRuntimeQuiescenceToken) (*runmodel.WorkspaceRetirementFence, error) {
	token = token.Normalized()
	if r == nil || token.Validate() != nil {
		return nil, common.ErrInvalidArgument
	}
	if scoped := common.RuntimeWorkspaceIDFromContext(ctx); scoped != "" && scoped != token.WorkspaceID {
		return nil, common.ErrConflict
	}
	headLock := workspaceRunGenerationLock(r.store.workspaceRunGenerationPath(token.WorkspaceID))
	headLock.Lock()
	defer headLock.Unlock()
	head, err := r.store.loadWorkspaceRunGenerationHead(token.WorkspaceID)
	if err != nil {
		return nil, err
	}
	if head.Retirement == nil {
		return nil, common.ErrNotFound
	}
	current := head.Retirement.Normalized()
	if current.IsRetired() {
		if runmodel.ValidateWorkspaceRetirementFinalizationReplay(current, token) != nil {
			return nil, common.ErrConflict
		}
		return &current, nil
	}
	items, err := r.ListByWorkspaceID(ctx, token.WorkspaceID)
	if err != nil {
		return nil, err
	}
	if head.Generation != token.WorkspaceRunGeneration || runmodel.ValidateWorkspaceRetirementFinalization(current, items, token) != nil {
		return nil, common.ErrConflict
	}
	current.Phase = runmodel.WorkspaceRetirementPhaseRetired
	current.FenceRevision++
	current.QuiescenceID = token.QuiescenceID
	current.RuntimeInventoryHash = token.RuntimeInventoryHash
	if err := current.Validate(); err != nil {
		return nil, err
	}
	head.Retirement = &current
	if err := r.store.saveWorkspaceRunGenerationHead(token.WorkspaceID, head); err != nil {
		return nil, err
	}
	return &current, nil
}

func (r *workspaceScopedRunRepository) GetWorkspaceRetirement(ctx context.Context, workspaceID common.WorkspaceID) (*runmodel.WorkspaceRetirementFence, error) {
	workspaceID = common.WorkspaceID(strings.TrimSpace(string(workspaceID)))
	if workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	if scoped := common.RuntimeWorkspaceIDFromContext(ctx); scoped != "" && scoped != workspaceID {
		return nil, common.ErrConflict
	}
	headLock := workspaceRunGenerationLock(r.store.workspaceRunGenerationPath(workspaceID))
	headLock.Lock()
	defer headLock.Unlock()
	head, err := r.store.loadWorkspaceRunGenerationHead(workspaceID)
	if err != nil {
		return nil, err
	}
	if head.Retirement == nil {
		return nil, common.ErrNotFound
	}
	ret := head.Retirement.Normalized()
	return &ret, nil
}

func (r *workspaceScopedRunRepository) ListWorkspaceRetirements(_ context.Context) ([]runmodel.WorkspaceRetirementFence, error) {
	matches, err := filepath.Glob(filepath.Join(r.store.dataRoot, "system", workspaceScopedRunGenerationDir, "*.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	ret := make([]runmodel.WorkspaceRetirementFence, 0, len(matches))
	for _, path := range matches {
		workspaceID := common.WorkspaceID(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
		if workspaceID == "" {
			return nil, common.ErrConflict
		}
		headLock := workspaceRunGenerationLock(path)
		headLock.Lock()
		head, loadErr := r.store.loadWorkspaceRunGenerationHead(workspaceID)
		headLock.Unlock()
		if loadErr != nil {
			return nil, loadErr
		}
		if head.Retirement != nil {
			ret = append(ret, head.Retirement.Normalized())
		}
	}
	return ret, nil
}

func (r *workspaceScopedRunRepository) rootForRun(ctx context.Context, runID common.RunID) (string, error) {
	item, err := r.Get(ctx, runID)
	if err != nil {
		return "", err
	}
	return r.store.runRoot(item.WorkspaceID, item.ID), nil
}

func (r *workspaceScopedRunRepository) withRunResourceMutation(ctx context.Context, runID common.RunID, mutate func(common.WorkspaceID, string) error) error {
	if r == nil || runID == "" || mutate == nil {
		return common.ErrInvalidArgument
	}
	item, err := r.Get(ctx, runID)
	if err != nil {
		return err
	}
	headLock := workspaceRunGenerationLock(r.store.workspaceRunGenerationPath(item.WorkspaceID))
	headLock.Lock()
	defer headLock.Unlock()
	head, err := r.store.loadWorkspaceRunGenerationHead(item.WorkspaceID)
	if err != nil {
		return err
	}
	if head.Retirement != nil {
		return common.ErrConflict
	}
	current, err := r.GetByWorkspaceID(ctx, item.WorkspaceID, runID)
	if err != nil || current.WorkspaceRunGeneration != item.WorkspaceRunGeneration {
		if err != nil {
			return err
		}
		return common.ErrConflict
	}
	return mutate(item.WorkspaceID, r.store.runRoot(item.WorkspaceID, runID))
}

type workspaceScopedStepRepository struct {
	store *workspaceScopedFileStore
	runs  *workspaceScopedRunRepository
}

func newWorkspaceScopedStepRepository(store *workspaceScopedFileStore, runs *workspaceScopedRunRepository) *workspaceScopedStepRepository {
	return &workspaceScopedStepRepository{store: store, runs: runs}
}

func (r *workspaceScopedStepRepository) RunOwnerRepositoryBinding() repository.RunOwnerRepositoryBinding {
	if r == nil || r.store == nil {
		return repository.RunOwnerRepositoryBinding{}
	}
	return r.store.runOwnerBinding
}

func (r *workspaceScopedStepRepository) Create(ctx context.Context, item *runmodel.Step) error {
	if item == nil || item.RunID == "" || item.ID == "" {
		return common.ErrInvalidArgument
	}
	return r.runs.withRunResourceMutation(ctx, item.RunID, func(_ common.WorkspaceID, root string) error {
		return r.store.stepRepo(r.store.runPath(root, workspaceScopedStepFile)).Create(ctx, item)
	})
}

func (r *workspaceScopedStepRepository) Get(ctx context.Context, id common.StepID) (*runmodel.Step, error) {
	if runID := runIDFromStepID(id); runID != "" {
		if root, ok := r.store.runRootByID(ctx, runID); ok {
			item, err := r.store.stepRepo(r.store.runPath(root, workspaceScopedStepFile)).Get(ctx, id)
			if err == nil {
				return item, nil
			}
			if err != common.ErrNotFound {
				return nil, err
			}
		}
		if common.RuntimeWorkspaceIDFromContext(ctx) != "" {
			return nil, common.ErrNotFound
		}
	}
	return findUniqueInPaths(r.store.workspaceRunRoots(common.RuntimeWorkspaceIDFromContext(ctx)), func(root string) (*runmodel.Step, error) {
		return r.store.stepRepo(r.store.runPath(root, workspaceScopedStepFile)).Get(ctx, id)
	})
}

func (r *workspaceScopedStepRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*runmodel.Step, error) {
	ret := []*runmodel.Step{}
	roots, err := r.store.runRootsByRunID(ctx, runID)
	if err != nil {
		if err == common.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	for _, root := range roots {
		items, err := r.store.stepRepo(r.store.runPath(root, workspaceScopedStepFile)).ListByRun(ctx, runID)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
	}
	sortSteps(ret)
	return nilIfEmpty(ret), nil
}

func (r *workspaceScopedStepRepository) ListByRunSummary(ctx context.Context, runID common.RunID) ([]*runmodel.Step, error) {
	ret := []*runmodel.Step{}
	roots, err := r.store.runRootsByRunID(ctx, runID)
	if err != nil {
		if err == common.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	for _, root := range roots {
		items, err := r.store.stepRepo(r.store.runPath(root, workspaceScopedStepFile)).ListByRunSummary(ctx, runID)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
	}
	sortSteps(ret)
	return nilIfEmpty(ret), nil
}

func (r *workspaceScopedStepRepository) Update(ctx context.Context, item *runmodel.Step) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if item.RunID != "" {
		if err := r.runs.withRunResourceMutation(ctx, item.RunID, func(_ common.WorkspaceID, root string) error {
			return r.store.stepRepo(r.store.runPath(root, workspaceScopedStepFile)).Update(ctx, item)
		}); err == nil || err != common.ErrNotFound {
			return err
		}
	}
	current, err := r.Get(ctx, item.ID)
	if err != nil {
		return err
	}
	item.RunID = current.RunID
	return r.runs.withRunResourceMutation(ctx, item.RunID, func(_ common.WorkspaceID, root string) error {
		return r.store.stepRepo(r.store.runPath(root, workspaceScopedStepFile)).Update(ctx, item)
	})
}

func (r *workspaceScopedStepRepository) Delete(ctx context.Context, id common.StepID) error {
	if id == "" {
		return common.ErrInvalidArgument
	}
	current, err := r.Get(ctx, id)
	if err != nil {
		return err
	}
	return r.runs.withRunResourceMutation(ctx, current.RunID, func(_ common.WorkspaceID, root string) error {
		return r.store.stepRepo(r.store.runPath(root, workspaceScopedStepFile)).Delete(ctx, id)
	})
}

type workspaceScopedToolCallRepository struct {
	store *workspaceScopedFileStore
}

func newWorkspaceScopedToolCallRepository(store *workspaceScopedFileStore) *workspaceScopedToolCallRepository {
	return &workspaceScopedToolCallRepository{store: store}
}

func (r *workspaceScopedToolCallRepository) Create(ctx context.Context, item *runtimeevidencemodel.ToolCall) error {
	if item == nil || item.WorkspaceID == "" || item.RunID == "" || item.ID == "" {
		return common.ErrInvalidArgument
	}
	return r.store.toolCallRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedToolCallFile)).Create(ctx, item)
}

func (r *workspaceScopedToolCallRepository) Get(ctx context.Context, id common.ToolCallID) (*runtimeevidencemodel.ToolCall, error) {
	return findUniqueInPaths(r.store.workspaceRunRoots(common.RuntimeWorkspaceIDFromContext(ctx)), func(root string) (*runtimeevidencemodel.ToolCall, error) {
		return r.store.toolCallRepo(r.store.runPath(root, workspaceScopedToolCallFile)).Get(ctx, id)
	})
}

func (r *workspaceScopedToolCallRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*runtimeevidencemodel.ToolCall, error) {
	ret := []*runtimeevidencemodel.ToolCall{}
	roots, err := r.store.runRootsByRunID(ctx, runID)
	if err != nil {
		if err == common.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	for _, root := range roots {
		items, err := r.store.toolCallRepo(r.store.runPath(root, workspaceScopedToolCallFile)).ListByRun(ctx, runID)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
	}
	sortToolCalls(ret)
	return nilIfEmpty(ret), nil
}

func (r *workspaceScopedToolCallRepository) ListByStep(ctx context.Context, stepID common.StepID) ([]*runtimeevidencemodel.ToolCall, error) {
	if runID := runIDFromStepID(stepID); runID != "" {
		if root, ok := r.store.runRootByID(ctx, runID); ok {
			items, err := r.store.toolCallRepo(r.store.runPath(root, workspaceScopedToolCallFile)).ListByStep(ctx, stepID)
			if err != nil {
				return nil, err
			}
			sortToolCalls(items)
			return nilIfEmpty(items), nil
		}
	}
	ret := []*runtimeevidencemodel.ToolCall{}
	for _, root := range r.store.allRunRoots() {
		items, err := r.store.toolCallRepo(r.store.runPath(root, workspaceScopedToolCallFile)).ListByStep(ctx, stepID)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
	}
	sortToolCalls(ret)
	return nilIfEmpty(ret), nil
}

func (r *workspaceScopedToolCallRepository) Update(ctx context.Context, item *runtimeevidencemodel.ToolCall) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if item.WorkspaceID != "" && item.RunID != "" {
		err := r.store.toolCallRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedToolCallFile)).Update(ctx, item)
		if err == nil || err != common.ErrNotFound {
			return err
		}
	}
	current, err := r.Get(ctx, item.ID)
	if err != nil {
		return err
	}
	item.WorkspaceID = current.WorkspaceID
	item.RunID = current.RunID
	return r.store.toolCallRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedToolCallFile)).Update(ctx, item)
}

func (r *workspaceScopedToolCallRepository) Delete(ctx context.Context, id common.ToolCallID) error {
	if id == "" {
		return common.ErrInvalidArgument
	}
	current, err := r.Get(ctx, id)
	if err != nil {
		return err
	}
	return r.store.toolCallRepo(r.store.runPath(r.store.runRoot(current.WorkspaceID, current.RunID), workspaceScopedToolCallFile)).Delete(ctx, id)
}

type workspaceScopedPatchRepository struct {
	store *workspaceScopedFileStore
}

func newWorkspaceScopedPatchRepository(store *workspaceScopedFileStore) *workspaceScopedPatchRepository {
	return &workspaceScopedPatchRepository{store: store}
}

func (r *workspaceScopedPatchRepository) Create(ctx context.Context, item *runtimeevidencemodel.Patch) error {
	if item == nil || item.WorkspaceID == "" || item.RunID == "" || item.ID == "" {
		return common.ErrInvalidArgument
	}
	return r.store.patchRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedPatchFile)).Create(ctx, item)
}

func (r *workspaceScopedPatchRepository) Get(ctx context.Context, id common.PatchID) (*runtimeevidencemodel.Patch, error) {
	return findUniqueInPaths(r.store.workspaceRunRoots(common.RuntimeWorkspaceIDFromContext(ctx)), func(root string) (*runtimeevidencemodel.Patch, error) {
		return r.store.patchRepo(r.store.runPath(root, workspaceScopedPatchFile)).Get(ctx, id)
	})
}

func (r *workspaceScopedPatchRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*runtimeevidencemodel.Patch, error) {
	ret := []*runtimeevidencemodel.Patch{}
	roots, err := r.store.runRootsByRunID(ctx, runID)
	if err != nil {
		if err == common.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	for _, root := range roots {
		items, err := r.store.patchRepo(r.store.runPath(root, workspaceScopedPatchFile)).ListByRun(ctx, runID)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
	}
	sortPatches(ret)
	return nilIfEmpty(ret), nil
}

func (r *workspaceScopedPatchRepository) ListByStep(ctx context.Context, stepID common.StepID) ([]*runtimeevidencemodel.Patch, error) {
	if runID := runIDFromStepID(stepID); runID != "" {
		if root, ok := r.store.runRootByID(ctx, runID); ok {
			items, err := r.store.patchRepo(r.store.runPath(root, workspaceScopedPatchFile)).ListByStep(ctx, stepID)
			if err != nil {
				return nil, err
			}
			sortPatches(items)
			return nilIfEmpty(items), nil
		}
	}
	ret := []*runtimeevidencemodel.Patch{}
	for _, root := range r.store.allRunRoots() {
		items, err := r.store.patchRepo(r.store.runPath(root, workspaceScopedPatchFile)).ListByStep(ctx, stepID)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
	}
	sortPatches(ret)
	return nilIfEmpty(ret), nil
}

func (r *workspaceScopedPatchRepository) Update(ctx context.Context, item *runtimeevidencemodel.Patch) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if item.WorkspaceID != "" && item.RunID != "" {
		err := r.store.patchRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedPatchFile)).Update(ctx, item)
		if err == nil || err != common.ErrNotFound {
			return err
		}
	}
	current, err := r.Get(ctx, item.ID)
	if err != nil {
		return err
	}
	item.WorkspaceID = current.WorkspaceID
	item.RunID = current.RunID
	return r.store.patchRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedPatchFile)).Update(ctx, item)
}

func (r *workspaceScopedPatchRepository) Delete(ctx context.Context, id common.PatchID) error {
	if id == "" {
		return common.ErrInvalidArgument
	}
	current, err := r.Get(ctx, id)
	if err != nil {
		return err
	}
	return r.store.patchRepo(r.store.runPath(r.store.runRoot(current.WorkspaceID, current.RunID), workspaceScopedPatchFile)).Delete(ctx, id)
}

type workspaceScopedValidationResultRepository struct {
	store *workspaceScopedFileStore
}

func newWorkspaceScopedValidationResultRepository(store *workspaceScopedFileStore) *workspaceScopedValidationResultRepository {
	return &workspaceScopedValidationResultRepository{store: store}
}

func (r *workspaceScopedValidationResultRepository) Create(ctx context.Context, item *runtimeevidencemodel.ValidationResult) error {
	if item == nil || item.WorkspaceID == "" || item.RunID == "" || item.ID == "" {
		return common.ErrInvalidArgument
	}
	return r.store.validationResultRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedValidationResultFile)).Create(ctx, item)
}

func (r *workspaceScopedValidationResultRepository) Get(ctx context.Context, id common.ValidationResultID) (*runtimeevidencemodel.ValidationResult, error) {
	return findUniqueInPaths(r.store.workspaceRunRoots(common.RuntimeWorkspaceIDFromContext(ctx)), func(root string) (*runtimeevidencemodel.ValidationResult, error) {
		return r.store.validationResultRepo(r.store.runPath(root, workspaceScopedValidationResultFile)).Get(ctx, id)
	})
}

func (r *workspaceScopedValidationResultRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*runtimeevidencemodel.ValidationResult, error) {
	ret := []*runtimeevidencemodel.ValidationResult{}
	roots, err := r.store.runRootsByRunID(ctx, runID)
	if err != nil {
		if err == common.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	for _, root := range roots {
		items, err := r.store.validationResultRepo(r.store.runPath(root, workspaceScopedValidationResultFile)).ListByRun(ctx, runID)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
	}
	sortValidationResults(ret)
	return nilIfEmpty(ret), nil
}

func (r *workspaceScopedValidationResultRepository) ListByStep(ctx context.Context, stepID common.StepID) ([]*runtimeevidencemodel.ValidationResult, error) {
	if runID := runIDFromStepID(stepID); runID != "" {
		if root, ok := r.store.runRootByID(ctx, runID); ok {
			items, err := r.store.validationResultRepo(r.store.runPath(root, workspaceScopedValidationResultFile)).ListByStep(ctx, stepID)
			if err != nil {
				return nil, err
			}
			sortValidationResults(items)
			return nilIfEmpty(items), nil
		}
	}
	ret := []*runtimeevidencemodel.ValidationResult{}
	for _, root := range r.store.allRunRoots() {
		items, err := r.store.validationResultRepo(r.store.runPath(root, workspaceScopedValidationResultFile)).ListByStep(ctx, stepID)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
	}
	sortValidationResults(ret)
	return nilIfEmpty(ret), nil
}

func (r *workspaceScopedValidationResultRepository) Update(ctx context.Context, item *runtimeevidencemodel.ValidationResult) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if item.WorkspaceID != "" && item.RunID != "" {
		err := r.store.validationResultRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedValidationResultFile)).Update(ctx, item)
		if err == nil || err != common.ErrNotFound {
			return err
		}
	}
	current, err := r.Get(ctx, item.ID)
	if err != nil {
		return err
	}
	item.WorkspaceID = current.WorkspaceID
	item.RunID = current.RunID
	return r.store.validationResultRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedValidationResultFile)).Update(ctx, item)
}

func (r *workspaceScopedValidationResultRepository) Delete(ctx context.Context, id common.ValidationResultID) error {
	if id == "" {
		return common.ErrInvalidArgument
	}
	current, err := r.Get(ctx, id)
	if err != nil {
		return err
	}
	return r.store.validationResultRepo(r.store.runPath(r.store.runRoot(current.WorkspaceID, current.RunID), workspaceScopedValidationResultFile)).Delete(ctx, id)
}

type workspaceScopedRuntimeCheckpointRepository struct {
	store *workspaceScopedFileStore
	runs  *workspaceScopedRunRepository
}

func newWorkspaceScopedRuntimeCheckpointRepository(store *workspaceScopedFileStore, runs *workspaceScopedRunRepository) *workspaceScopedRuntimeCheckpointRepository {
	return &workspaceScopedRuntimeCheckpointRepository{store: store, runs: runs}
}

func (r *workspaceScopedRuntimeCheckpointRepository) RunOwnerRepositoryBinding() repository.RunOwnerRepositoryBinding {
	if r == nil || r.store == nil {
		return repository.RunOwnerRepositoryBinding{}
	}
	return r.store.runOwnerBinding
}

func (r *workspaceScopedRuntimeCheckpointRepository) Create(ctx context.Context, item *runmodel.RuntimeCheckpoint) error {
	if item == nil || item.WorkspaceID == "" || item.RunID == "" || item.ID == "" {
		return common.ErrInvalidArgument
	}
	return r.runs.withRunResourceMutation(ctx, item.RunID, func(workspaceID common.WorkspaceID, root string) error {
		if item.WorkspaceID != workspaceID {
			return common.ErrConflict
		}
		return r.store.runtimeCheckpointRepo(r.store.runPath(root, workspaceScopedRuntimeCheckpointFile)).Create(ctx, item)
	})
}

func (r *workspaceScopedRuntimeCheckpointRepository) Get(ctx context.Context, id common.CheckpointID) (*runmodel.RuntimeCheckpoint, error) {
	return findUniqueInPaths(r.store.workspaceRunRoots(common.RuntimeWorkspaceIDFromContext(ctx)), func(root string) (*runmodel.RuntimeCheckpoint, error) {
		return r.store.runtimeCheckpointRepo(r.store.runPath(root, workspaceScopedRuntimeCheckpointFile)).Get(ctx, id)
	})
}

func (r *workspaceScopedRuntimeCheckpointRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*runmodel.RuntimeCheckpoint, error) {
	ret := []*runmodel.RuntimeCheckpoint{}
	roots, err := r.store.runRootsByRunID(ctx, runID)
	if err != nil {
		if err == common.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	for _, root := range roots {
		items, err := r.store.runtimeCheckpointRepo(r.store.runPath(root, workspaceScopedRuntimeCheckpointFile)).ListByRun(ctx, runID)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
	}
	sortRuntimeCheckpoints(ret)
	return nilIfEmpty(ret), nil
}

func (r *workspaceScopedRuntimeCheckpointRepository) Update(ctx context.Context, item *runmodel.RuntimeCheckpoint) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if item.WorkspaceID != "" && item.RunID != "" {
		err := r.runs.withRunResourceMutation(ctx, item.RunID, func(workspaceID common.WorkspaceID, root string) error {
			if item.WorkspaceID != workspaceID {
				return common.ErrConflict
			}
			return r.store.runtimeCheckpointRepo(r.store.runPath(root, workspaceScopedRuntimeCheckpointFile)).Update(ctx, item)
		})
		if err == nil || err != common.ErrNotFound {
			return err
		}
	}
	current, err := r.Get(ctx, item.ID)
	if err != nil {
		return err
	}
	item.WorkspaceID = current.WorkspaceID
	item.RunID = current.RunID
	return r.runs.withRunResourceMutation(ctx, item.RunID, func(workspaceID common.WorkspaceID, root string) error {
		if item.WorkspaceID != workspaceID {
			return common.ErrConflict
		}
		return r.store.runtimeCheckpointRepo(r.store.runPath(root, workspaceScopedRuntimeCheckpointFile)).Update(ctx, item)
	})
}

func (r *workspaceScopedRuntimeCheckpointRepository) Delete(ctx context.Context, id common.CheckpointID) error {
	if id == "" {
		return common.ErrInvalidArgument
	}
	current, err := r.Get(ctx, id)
	if err != nil {
		return err
	}
	return r.runs.withRunResourceMutation(ctx, current.RunID, func(workspaceID common.WorkspaceID, root string) error {
		if current.WorkspaceID != workspaceID {
			return common.ErrConflict
		}
		return r.store.runtimeCheckpointRepo(r.store.runPath(root, workspaceScopedRuntimeCheckpointFile)).Delete(ctx, id)
	})
}

type workspaceScopedEvidenceRecordRepository struct {
	store *workspaceScopedFileStore
}

func newWorkspaceScopedEvidenceRecordRepository(store *workspaceScopedFileStore) *workspaceScopedEvidenceRecordRepository {
	return &workspaceScopedEvidenceRecordRepository{store: store}
}

func (r *workspaceScopedEvidenceRecordRepository) Create(ctx context.Context, item *orchestratormodel.EvidenceRecord) error {
	if item == nil || item.WorkspaceID == "" || item.RunID == "" || item.ID == "" {
		return common.ErrInvalidArgument
	}
	return r.repoForRun(item.WorkspaceID, item.RunID).Create(ctx, item)
}

func (r *workspaceScopedEvidenceRecordRepository) Get(ctx context.Context, id common.EvidenceRecordID) (*orchestratormodel.EvidenceRecord, error) {
	if id == "" {
		return nil, common.ErrInvalidArgument
	}
	return findUniqueInPaths(r.store.workspaceRunRoots(common.RuntimeWorkspaceIDFromContext(ctx)), func(root string) (*orchestratormodel.EvidenceRecord, error) {
		return r.store.evidenceRecordRepo(r.store.runPath(root, workspaceScopedEvidenceRecordDir)).Get(ctx, id)
	})
}

func (r *workspaceScopedEvidenceRecordRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*orchestratormodel.EvidenceRecord, error) {
	ret := []*orchestratormodel.EvidenceRecord{}
	roots, err := r.store.runRootsByRunID(ctx, runID)
	if err != nil {
		if err == common.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	for _, root := range roots {
		items, err := r.store.evidenceRecordRepo(r.store.runPath(root, workspaceScopedEvidenceRecordDir)).ListByRun(ctx, runID)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
	}
	sortEvidenceRecords(ret)
	return nilIfEmpty(ret), nil
}

func (r *workspaceScopedEvidenceRecordRepository) ListByStep(ctx context.Context, stepID common.StepID) ([]*orchestratormodel.EvidenceRecord, error) {
	if stepID == "" {
		return nil, common.ErrInvalidArgument
	}
	ret := []*orchestratormodel.EvidenceRecord{}
	for _, root := range r.store.workspaceRunRoots(common.RuntimeWorkspaceIDFromContext(ctx)) {
		items, err := r.store.evidenceRecordRepo(r.store.runPath(root, workspaceScopedEvidenceRecordDir)).ListByStep(ctx, stepID)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
	}
	sortEvidenceRecords(ret)
	return nilIfEmpty(ret), nil
}

func (r *workspaceScopedEvidenceRecordRepository) ListBySourceRef(ctx context.Context, runID common.RunID, sourceRef string) ([]*orchestratormodel.EvidenceRecord, error) {
	items, err := r.ListByRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	ret := items[:0]
	for _, item := range items {
		if item.SourceRef == sourceRef {
			ret = append(ret, item)
		}
	}
	return nilIfEmpty(ret), nil
}

func (r *workspaceScopedEvidenceRecordRepository) Update(ctx context.Context, item *orchestratormodel.EvidenceRecord) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if item.WorkspaceID != "" && item.RunID != "" {
		err := r.repoForRun(item.WorkspaceID, item.RunID).Update(ctx, item)
		if err == nil || err != common.ErrNotFound {
			return err
		}
	}
	current, err := r.Get(ctx, item.ID)
	if err != nil {
		return err
	}
	item.WorkspaceID = current.WorkspaceID
	item.RunID = current.RunID
	return r.repoForRun(item.WorkspaceID, item.RunID).Update(ctx, item)
}

func (r *workspaceScopedEvidenceRecordRepository) Delete(ctx context.Context, id common.EvidenceRecordID) error {
	if id == "" {
		return common.ErrInvalidArgument
	}
	current, err := r.Get(ctx, id)
	if err != nil {
		return err
	}
	return r.repoForRun(current.WorkspaceID, current.RunID).Delete(ctx, id)
}

func (r *workspaceScopedEvidenceRecordRepository) repoForRun(workspaceID common.WorkspaceID, runID common.RunID) *fileRepo.EvidenceRecordRepository {
	return r.store.evidenceRecordRepo(r.store.runPath(r.store.runRoot(workspaceID, runID), workspaceScopedEvidenceRecordDir))
}

type workspaceScopedCoverageWorkItemRepository struct {
	store *workspaceScopedFileStore
}

func newWorkspaceScopedCoverageWorkItemRepository(store *workspaceScopedFileStore) *workspaceScopedCoverageWorkItemRepository {
	return &workspaceScopedCoverageWorkItemRepository{store: store}
}

func (r *workspaceScopedCoverageWorkItemRepository) Create(ctx context.Context, item *orchestratormodel.CoverageWorkItem) error {
	if item == nil || item.WorkspaceID == "" || item.RunID == "" || item.ID == "" {
		return common.ErrInvalidArgument
	}
	return r.repoForRun(item.WorkspaceID, item.RunID).Create(ctx, item)
}

func (r *workspaceScopedCoverageWorkItemRepository) Get(ctx context.Context, id common.CoverageWorkItemID) (*orchestratormodel.CoverageWorkItem, error) {
	if id == "" {
		return nil, common.ErrInvalidArgument
	}
	return findUniqueInPaths(r.store.workspaceRunRoots(common.RuntimeWorkspaceIDFromContext(ctx)), func(root string) (*orchestratormodel.CoverageWorkItem, error) {
		return r.store.coverageWorkItemRepo(r.store.runPath(root, workspaceScopedCoverageWorkItemDir)).Get(ctx, id)
	})
}

func (r *workspaceScopedCoverageWorkItemRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*orchestratormodel.CoverageWorkItem, error) {
	ret := []*orchestratormodel.CoverageWorkItem{}
	roots, err := r.store.runRootsByRunID(ctx, runID)
	if err != nil {
		if err == common.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	for _, root := range roots {
		items, err := r.store.coverageWorkItemRepo(r.store.runPath(root, workspaceScopedCoverageWorkItemDir)).ListByRun(ctx, runID)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
	}
	sortCoverageWorkItems(ret)
	return nilIfEmpty(ret), nil
}

func (r *workspaceScopedCoverageWorkItemRepository) ListActiveByRun(ctx context.Context, runID common.RunID) ([]*orchestratormodel.CoverageWorkItem, error) {
	items, err := r.ListByRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	ret := items[:0]
	for _, item := range items {
		if item.IsActive() {
			ret = append(ret, item)
		}
	}
	return nilIfEmpty(ret), nil
}

func (r *workspaceScopedCoverageWorkItemRepository) Update(ctx context.Context, item *orchestratormodel.CoverageWorkItem) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if item.WorkspaceID != "" && item.RunID != "" {
		err := r.repoForRun(item.WorkspaceID, item.RunID).Update(ctx, item)
		if err == nil || err != common.ErrNotFound {
			return err
		}
	}
	current, err := r.Get(ctx, item.ID)
	if err != nil {
		return err
	}
	item.WorkspaceID = current.WorkspaceID
	item.RunID = current.RunID
	return r.repoForRun(item.WorkspaceID, item.RunID).Update(ctx, item)
}

func (r *workspaceScopedCoverageWorkItemRepository) Delete(ctx context.Context, id common.CoverageWorkItemID) error {
	if id == "" {
		return common.ErrInvalidArgument
	}
	current, err := r.Get(ctx, id)
	if err != nil {
		return err
	}
	return r.repoForRun(current.WorkspaceID, current.RunID).Delete(ctx, id)
}

func (r *workspaceScopedCoverageWorkItemRepository) repoForRun(workspaceID common.WorkspaceID, runID common.RunID) *fileRepo.CoverageWorkItemRepository {
	return r.store.coverageWorkItemRepo(r.store.runPath(r.store.runRoot(workspaceID, runID), workspaceScopedCoverageWorkItemDir))
}

type workspaceScopedArtifactContinuationRepository struct {
	store *workspaceScopedFileStore
}

func newWorkspaceScopedArtifactContinuationRepository(store *workspaceScopedFileStore) *workspaceScopedArtifactContinuationRepository {
	return &workspaceScopedArtifactContinuationRepository{store: store}
}

func (r *workspaceScopedArtifactContinuationRepository) Create(ctx context.Context, item *orchestratormodel.ArtifactContinuation) error {
	if item == nil || item.ID == "" || item.WorkspaceID == "" || item.RunID == "" {
		return common.ErrInvalidArgument
	}
	return r.repoForRun(item.WorkspaceID, item.RunID).Create(ctx, item)
}

func (r *workspaceScopedArtifactContinuationRepository) Get(ctx context.Context, id common.ArtifactContinuationID) (*orchestratormodel.ArtifactContinuation, error) {
	if id == "" {
		return nil, common.ErrInvalidArgument
	}
	return findUniqueInPaths(r.store.workspaceRunRoots(common.RuntimeWorkspaceIDFromContext(ctx)), func(root string) (*orchestratormodel.ArtifactContinuation, error) {
		return r.store.artifactContinuationRepo(r.store.runPath(root, workspaceScopedArtifactContinuationDir)).Get(ctx, id)
	})
}

func (r *workspaceScopedArtifactContinuationRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*orchestratormodel.ArtifactContinuation, error) {
	ret := []*orchestratormodel.ArtifactContinuation{}
	roots, err := r.store.runRootsByRunID(ctx, runID)
	if err != nil {
		if err == common.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	for _, root := range roots {
		items, err := r.store.artifactContinuationRepo(r.store.runPath(root, workspaceScopedArtifactContinuationDir)).ListByRun(ctx, runID)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
	}
	sortArtifactContinuations(ret)
	return nilIfEmptyArtifactContinuations(ret), nil
}

func (r *workspaceScopedArtifactContinuationRepository) ListOpenByRun(ctx context.Context, runID common.RunID) ([]*orchestratormodel.ArtifactContinuation, error) {
	items, err := r.ListByRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	ret := items[:0]
	for _, item := range items {
		if item.State != common.ArtifactContinuationStateApplied && item.State != common.ArtifactContinuationStateFailed {
			ret = append(ret, item)
		}
	}
	return nilIfEmptyArtifactContinuations(ret), nil
}

func (r *workspaceScopedArtifactContinuationRepository) Update(ctx context.Context, item *orchestratormodel.ArtifactContinuation) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if item.WorkspaceID != "" && item.RunID != "" {
		err := r.repoForRun(item.WorkspaceID, item.RunID).Update(ctx, item)
		if err == nil || err != common.ErrNotFound {
			return err
		}
	}
	current, err := r.Get(ctx, item.ID)
	if err != nil {
		return err
	}
	item.WorkspaceID = current.WorkspaceID
	item.RunID = current.RunID
	return r.repoForRun(item.WorkspaceID, item.RunID).Update(ctx, item)
}

func (r *workspaceScopedArtifactContinuationRepository) Delete(ctx context.Context, id common.ArtifactContinuationID) error {
	if id == "" {
		return common.ErrInvalidArgument
	}
	current, err := r.Get(ctx, id)
	if err != nil {
		return err
	}
	return r.repoForRun(current.WorkspaceID, current.RunID).Delete(ctx, id)
}

func (r *workspaceScopedArtifactContinuationRepository) SaveChunk(ctx context.Context, chunk *orchestratormodel.ArtifactContinuationChunk, content string) error {
	if chunk == nil || chunk.ContinuationID == "" {
		return common.ErrInvalidArgument
	}
	repo, err := r.repoForContinuation(ctx, chunk.ContinuationID)
	if err != nil {
		return err
	}
	return repo.SaveChunk(ctx, chunk, content)
}

func (r *workspaceScopedArtifactContinuationRepository) GetChunk(ctx context.Context, id common.ArtifactContinuationID, sequence int) (*orchestratormodel.ArtifactContinuationChunk, string, error) {
	repo, err := r.repoForContinuation(ctx, id)
	if err != nil {
		return nil, "", err
	}
	return repo.GetChunk(ctx, id, sequence)
}

func (r *workspaceScopedArtifactContinuationRepository) ListChunks(ctx context.Context, id common.ArtifactContinuationID) ([]*orchestratormodel.ArtifactContinuationChunk, error) {
	repo, err := r.repoForContinuation(ctx, id)
	if err != nil {
		return nil, err
	}
	return repo.ListChunks(ctx, id)
}

func (r *workspaceScopedArtifactContinuationRepository) SaveAssembled(ctx context.Context, item *orchestratormodel.ArtifactContinuationAssembledMetadata, content string) error {
	if item == nil || item.ContinuationID == "" {
		return common.ErrInvalidArgument
	}
	repo, err := r.repoForContinuation(ctx, item.ContinuationID)
	if err != nil {
		return err
	}
	return repo.SaveAssembled(ctx, item, content)
}

func (r *workspaceScopedArtifactContinuationRepository) GetAssembled(ctx context.Context, id common.ArtifactContinuationID) (*orchestratormodel.ArtifactContinuationAssembledMetadata, string, error) {
	repo, err := r.repoForContinuation(ctx, id)
	if err != nil {
		return nil, "", err
	}
	return repo.GetAssembled(ctx, id)
}

func (r *workspaceScopedArtifactContinuationRepository) repoForContinuation(ctx context.Context, id common.ArtifactContinuationID) (*fileRepo.ArtifactContinuationRepository, error) {
	item, err := r.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return r.repoForRun(item.WorkspaceID, item.RunID), nil
}

func (r *workspaceScopedArtifactContinuationRepository) repoForRun(workspaceID common.WorkspaceID, runID common.RunID) *fileRepo.ArtifactContinuationRepository {
	return r.store.artifactContinuationRepo(r.store.runPath(r.store.runRoot(workspaceID, runID), workspaceScopedArtifactContinuationDir))
}

type workspaceScopedResultRepository struct {
	store *workspaceScopedFileStore
}

func newWorkspaceScopedResultRepository(store *workspaceScopedFileStore) *workspaceScopedResultRepository {
	return &workspaceScopedResultRepository{store: store}
}

func (r *workspaceScopedResultRepository) Create(ctx context.Context, item *resultmodel.Result) error {
	if item == nil || item.WorkspaceID == "" || item.RunID == "" || item.ID == "" {
		return common.ErrInvalidArgument
	}
	return withWorkspaceDeletionMutation(ctx, r.store.dataRoot, "result", item.WorkspaceID, func(guarded context.Context) error {
		return r.store.resultRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedResultFile)).Create(guarded, item)
	})
}

func (r *workspaceScopedResultRepository) Get(ctx context.Context, id common.ResultID) (*resultmodel.Result, error) {
	return findUniqueInPaths(r.store.workspaceRunRoots(common.RuntimeWorkspaceIDFromContext(ctx)), func(root string) (*resultmodel.Result, error) {
		return r.store.resultRepo(r.store.runPath(root, workspaceScopedResultFile)).Get(ctx, id)
	})
}

func (r *workspaceScopedResultRepository) ListByWorkspace(ctx context.Context, workspaceID common.WorkspaceID) ([]*resultmodel.Result, error) {
	ret := []*resultmodel.Result{}
	for _, root := range r.store.workspaceRunRoots(workspaceID) {
		items, err := r.store.resultRepo(r.store.runPath(root, workspaceScopedResultFile)).ListByWorkspace(ctx, workspaceID)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
	}
	sortResults(ret)
	return nilIfEmpty(ret), nil
}

func (r *workspaceScopedResultRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*resultmodel.Result, error) {
	ret := []*resultmodel.Result{}
	roots, err := r.store.runRootsByRunID(ctx, runID)
	if err != nil {
		if err == common.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	for _, root := range roots {
		items, err := r.store.resultRepo(r.store.runPath(root, workspaceScopedResultFile)).ListByRun(ctx, runID)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
	}
	sortResults(ret)
	return nilIfEmpty(ret), nil
}

func (r *workspaceScopedResultRepository) ListForRecovery(ctx context.Context, cursor string, limit int) ([]*resultmodel.Result, string, error) {
	if limit < 1 {
		return nil, "", common.ErrInvalidArgument
	}
	ret := make([]*resultmodel.Result, 0, limit+1)
	for _, root := range r.store.allRunRoots() {
		localCursor := ""
		rootComplete := false
		for page := 0; page < 1000 && len(ret) <= limit; page++ {
			remaining := limit + 1 - len(ret)
			items, next, err := r.store.resultRepo(r.store.runPath(root, workspaceScopedResultFile)).ListForRecovery(ctx, localCursor, remaining)
			if err != nil {
				return nil, "", err
			}
			for _, item := range items {
				if item == nil || workspaceScopedResultRecoveryKey(root, item) <= cursor {
					continue
				}
				fenced, fenceErr := workspaceDeletionExists(r.store.dataRoot, "result", item.WorkspaceID)
				if fenceErr != nil {
					return nil, "", fenceErr
				}
				if fenced {
					continue
				}
				ret = append(ret, item)
				if len(ret) > limit {
					break
				}
			}
			if next == "" {
				rootComplete = true
				break
			}
			if len(ret) > limit {
				break
			}
			localCursor = next
		}
		if !rootComplete && len(ret) <= limit {
			return nil, "", common.ErrConflict
		}
		if len(ret) > limit {
			break
		}
	}
	next := ""
	if len(ret) > limit {
		ret = ret[:limit]
		last := ret[len(ret)-1]
		next = workspaceScopedResultRecoveryKey(r.store.runRoot(last.WorkspaceID, last.RunID), last)
	}
	return nilIfEmpty(ret), next, nil
}

func workspaceScopedResultRecoveryKey(root string, item *resultmodel.Result) string {
	if item == nil {
		return ""
	}
	return root + "\x00" + string(item.WorkspaceID) + "\x00" + string(item.ID)
}

func (r *workspaceScopedResultRepository) Update(ctx context.Context, item *resultmodel.Result) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if item.WorkspaceID != "" && item.RunID != "" {
		err := withWorkspaceDeletionMutation(ctx, r.store.dataRoot, "result", item.WorkspaceID, func(guarded context.Context) error {
			return r.store.resultRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedResultFile)).Update(guarded, item)
		})
		if err == nil || err != common.ErrNotFound {
			return err
		}
		if fenced, fenceErr := workspaceDeletionExists(r.store.dataRoot, "result", item.WorkspaceID); fenceErr != nil || fenced {
			if fenceErr != nil {
				return fenceErr
			}
			return common.ErrNotFound
		}
	}
	current, err := r.Get(ctx, item.ID)
	if err != nil {
		return err
	}
	item.WorkspaceID = current.WorkspaceID
	item.RunID = current.RunID
	return withWorkspaceDeletionMutation(ctx, r.store.dataRoot, "result", item.WorkspaceID, func(guarded context.Context) error {
		return r.store.resultRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedResultFile)).Update(guarded, item)
	})
}

func (r *workspaceScopedResultRepository) CompareAndSwap(ctx context.Context, item *resultmodel.Result, expectedRevision uint64) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if item.WorkspaceID != "" && item.RunID != "" {
		err := withWorkspaceDeletionMutation(ctx, r.store.dataRoot, "result", item.WorkspaceID, func(guarded context.Context) error {
			return r.store.resultRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedResultFile)).CompareAndSwap(guarded, item, expectedRevision)
		})
		if err == nil || err != common.ErrNotFound {
			return err
		}
		if fenced, fenceErr := workspaceDeletionExists(r.store.dataRoot, "result", item.WorkspaceID); fenceErr != nil || fenced {
			if fenceErr != nil {
				return fenceErr
			}
			return common.ErrNotFound
		}
	}
	current, err := r.Get(ctx, item.ID)
	if err != nil {
		return err
	}
	if current.Revision != expectedRevision {
		return common.ErrConflict
	}
	item.WorkspaceID = current.WorkspaceID
	item.RunID = current.RunID
	return withWorkspaceDeletionMutation(ctx, r.store.dataRoot, "result", item.WorkspaceID, func(guarded context.Context) error {
		return r.store.resultRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedResultFile)).CompareAndSwap(guarded, item, expectedRevision)
	})
}

func (r *workspaceScopedResultRepository) Delete(ctx context.Context, id common.ResultID) error {
	if id == "" {
		return common.ErrInvalidArgument
	}
	current, err := r.Get(ctx, id)
	if err != nil {
		return err
	}
	return withWorkspaceDeletionMutation(ctx, r.store.dataRoot, "result", current.WorkspaceID, func(guarded context.Context) error {
		return r.store.resultRepo(r.store.runPath(r.store.runRoot(current.WorkspaceID, current.RunID), workspaceScopedResultFile)).Delete(guarded, id)
	})
}

type workspaceScopedArtifactRepository struct {
	store *workspaceScopedFileStore
}

func newWorkspaceScopedArtifactRepository(store *workspaceScopedFileStore) *workspaceScopedArtifactRepository {
	return &workspaceScopedArtifactRepository{store: store}
}

func (r *workspaceScopedArtifactRepository) Create(ctx context.Context, item *resultmodel.Artifact) error {
	if item == nil || item.WorkspaceID == "" || item.RunID == "" || item.ID == "" {
		return common.ErrInvalidArgument
	}
	return withWorkspaceDeletionMutation(ctx, r.store.dataRoot, "result", item.WorkspaceID, func(guarded context.Context) error {
		return r.store.artifactRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedArtifactFile)).Create(guarded, item)
	})
}

func (r *workspaceScopedArtifactRepository) Get(ctx context.Context, id common.ArtifactID) (*resultmodel.Artifact, error) {
	return findUniqueInPaths(r.store.workspaceRunRoots(common.RuntimeWorkspaceIDFromContext(ctx)), func(root string) (*resultmodel.Artifact, error) {
		return r.store.artifactRepo(r.store.runPath(root, workspaceScopedArtifactFile)).Get(ctx, id)
	})
}

func (r *workspaceScopedArtifactRepository) ListByWorkspace(ctx context.Context, workspaceID common.WorkspaceID) ([]*resultmodel.Artifact, error) {
	ret := []*resultmodel.Artifact{}
	for _, root := range r.store.workspaceRunRoots(workspaceID) {
		items, err := r.store.artifactRepo(r.store.runPath(root, workspaceScopedArtifactFile)).ListByWorkspace(ctx, workspaceID)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
	}
	sortArtifacts(ret)
	return nilIfEmpty(ret), nil
}

func (r *workspaceScopedArtifactRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*resultmodel.Artifact, error) {
	ret := []*resultmodel.Artifact{}
	roots, err := r.store.runRootsByRunID(ctx, runID)
	if err != nil {
		if err == common.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	for _, root := range roots {
		items, err := r.store.artifactRepo(r.store.runPath(root, workspaceScopedArtifactFile)).ListByRun(ctx, runID)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
	}
	sortArtifacts(ret)
	return nilIfEmpty(ret), nil
}

func (r *workspaceScopedArtifactRepository) ListByResult(ctx context.Context, resultID common.ResultID) ([]*resultmodel.Artifact, error) {
	ret := []*resultmodel.Artifact{}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	roots := r.store.workspaceRunRoots(workspaceID)
	if workspaceID == "" {
		roots = r.store.allRunRoots()
	}
	for _, root := range roots {
		items, err := r.store.artifactRepo(r.store.runPath(root, workspaceScopedArtifactFile)).ListByResult(ctx, resultID)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
	}
	if workspaceID == "" {
		var matchedWorkspace common.WorkspaceID
		for _, item := range ret {
			if item == nil {
				continue
			}
			if matchedWorkspace != "" && matchedWorkspace != item.WorkspaceID {
				return nil, common.ErrConflict
			}
			matchedWorkspace = item.WorkspaceID
		}
	}
	sortArtifacts(ret)
	return nilIfEmpty(ret), nil
}

func (r *workspaceScopedArtifactRepository) Update(ctx context.Context, item *resultmodel.Artifact) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if item.WorkspaceID != "" && item.RunID != "" {
		err := withWorkspaceDeletionMutation(ctx, r.store.dataRoot, "result", item.WorkspaceID, func(guarded context.Context) error {
			return r.store.artifactRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedArtifactFile)).Update(guarded, item)
		})
		if err == nil || err != common.ErrNotFound {
			return err
		}
		if fenced, fenceErr := workspaceDeletionExists(r.store.dataRoot, "result", item.WorkspaceID); fenceErr != nil || fenced {
			if fenceErr != nil {
				return fenceErr
			}
			return common.ErrNotFound
		}
	}
	current, err := r.Get(ctx, item.ID)
	if err != nil {
		return err
	}
	item.WorkspaceID = current.WorkspaceID
	item.RunID = current.RunID
	return withWorkspaceDeletionMutation(ctx, r.store.dataRoot, "result", item.WorkspaceID, func(guarded context.Context) error {
		return r.store.artifactRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedArtifactFile)).Update(guarded, item)
	})
}

func (r *workspaceScopedArtifactRepository) Delete(ctx context.Context, id common.ArtifactID) error {
	if id == "" {
		return common.ErrInvalidArgument
	}
	current, err := r.Get(ctx, id)
	if err != nil {
		return err
	}
	return withWorkspaceDeletionMutation(ctx, r.store.dataRoot, "result", current.WorkspaceID, func(guarded context.Context) error {
		return r.store.artifactRepo(r.store.runPath(r.store.runRoot(current.WorkspaceID, current.RunID), workspaceScopedArtifactFile)).Delete(guarded, id)
	})
}

type workspaceScopedEventRecordRepository struct {
	store     *workspaceScopedFileStore
	deletions *eventStreamWorkspaceDeletionRepository
}

func newWorkspaceScopedEventRecordRepository(store *workspaceScopedFileStore, deletions *eventStreamWorkspaceDeletionRepository) *workspaceScopedEventRecordRepository {
	return &workspaceScopedEventRecordRepository{store: store, deletions: deletions}
}

func (r *workspaceScopedEventRecordRepository) Append(ctx context.Context, item *eventstreammodel.EventRecord) error {
	if r == nil || r.store == nil || r.deletions == nil || item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	appendRecord := func(guarded context.Context) error {
		positionPath := r.store.systemPath(workspaceScopedEventPositionFile)
		positionLock := workspaceDeletionFileLock(positionPath)
		positionLock.Lock()
		err := r.store.eventRecordRepo(positionPath).ObserveEventRecordStreamPosition(item.StreamPosition)
		positionLock.Unlock()
		if err != nil {
			return err
		}
		storagePath := r.store.systemPath(workspaceScopedEventRecordFile)
		if item.WorkspaceID != "" && item.RunID != "" {
			storagePath = r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedEventRecordFile)
		}
		storageLock := workspaceDeletionFileLock(storagePath)
		storageLock.Lock()
		defer storageLock.Unlock()
		return r.store.eventRecordRepo(storagePath).Append(guarded, item)
	}
	if item.WorkspaceID == "" {
		return appendRecord(ctx)
	}
	return withWorkspaceDeletionMutation(ctx, r.store.dataRoot, "eventstream", item.WorkspaceID, appendRecord)
}

func (r *workspaceScopedEventRecordRepository) ReserveEventRecordStreamPosition(ctx context.Context) (int64, error) {
	if r == nil || r.store == nil {
		return 0, common.ErrInvalidArgument
	}
	// Records are physically partitioned by run. Reserve their global cursor
	// positions from one system-scoped durable counter.
	positionPath := r.store.systemPath(workspaceScopedEventPositionFile)
	lock := workspaceDeletionFileLock(positionPath)
	lock.Lock()
	defer lock.Unlock()
	return r.store.eventRecordRepo(positionPath).ReserveEventRecordStreamPosition(ctx)
}

func (r *workspaceScopedEventRecordRepository) Get(ctx context.Context, id common.EventRecordID) (*eventstreammodel.EventRecord, error) {
	if r == nil || r.store == nil || r.deletions == nil || id == "" {
		return nil, common.ErrInvalidArgument
	}
	if workspaceID := common.RuntimeWorkspaceIDFromContext(ctx); workspaceID != "" {
		hidden, err := workspaceDeletionExists(r.store.dataRoot, "eventstream", workspaceID)
		if err != nil {
			return nil, err
		}
		if hidden {
			return nil, common.ErrNotFound
		}
	}
	paths := []string{r.store.systemPath(workspaceScopedEventRecordFile)}
	for _, root := range r.store.workspaceRunRoots(common.RuntimeWorkspaceIDFromContext(ctx)) {
		paths = append(paths, r.store.runPath(root, workspaceScopedEventRecordFile))
	}
	visible := make([]*eventstreammodel.EventRecord, 0, 1)
	for _, path := range paths {
		item, err := r.getEventRecordFromPath(ctx, path, id)
		if err == common.ErrNotFound {
			continue
		}
		if err != nil {
			return nil, err
		}
		item, err = r.visibleEventRecord(item)
		if err == common.ErrNotFound {
			continue
		}
		if err != nil {
			return nil, err
		}
		visible = append(visible, item)
	}
	if len(visible) == 0 {
		return nil, common.ErrNotFound
	}
	if len(visible) != 1 {
		return nil, common.ErrConflict
	}
	return visible[0], nil
}

func (r *workspaceScopedEventRecordRepository) List(ctx context.Context, query workorchquerycontract.EventRecordQuery) ([]*eventstreammodel.EventRecord, error) {
	if r == nil || r.store == nil || r.deletions == nil {
		return nil, common.ErrInvalidArgument
	}
	if query.WorkspaceID == "" {
		query.WorkspaceID = common.RuntimeWorkspaceIDFromContext(ctx)
	}
	if query.WorkspaceID != "" {
		hidden, err := workspaceDeletionExists(r.store.dataRoot, "eventstream", query.WorkspaceID)
		if err != nil {
			return nil, err
		}
		if hidden {
			return nil, nil
		}
	}
	listQuery := query
	if query.WorkspaceID == "" {
		listQuery.CursorAfterID = ""
		listQuery.Limit = 0
	} else if query.RunID == "" {
		listQuery.CursorAfterID = ""
		if query.CursorAfterID != "" {
			listQuery.Limit = 0
		}
	}
	ret := []*eventstreammodel.EventRecord{}
	if query.RunID == "" {
		systemItems, err := r.listEventRecordsFromPath(ctx, r.store.systemPath(workspaceScopedEventRecordFile), listQuery)
		if err != nil {
			return nil, err
		}
		ret = append(ret, systemItems...)
	}
	runRoots := r.store.workspaceRunRoots(query.WorkspaceID)
	if query.RunID != "" && query.WorkspaceID != "" {
		queryCtx := ctx
		queryCtx = common.ContextWithRuntimeWorkspaceID(ctx, query.WorkspaceID)
		roots, err := r.store.runRootsByRunID(queryCtx, query.RunID)
		if err != nil {
			if err == common.ErrNotFound {
				runRoots = nil
			} else {
				return nil, err
			}
		} else {
			runRoots = roots
		}
	}
	for _, root := range runRoots {
		items, err := r.listEventRecordsFromPath(ctx, r.store.runPath(root, workspaceScopedEventRecordFile), listQuery)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
	}
	if query.WorkspaceID == "" {
		visible := ret[:0]
		visibility := map[common.WorkspaceID]bool{}
		for _, item := range ret {
			if item == nil || item.WorkspaceID == "" {
				visible = append(visible, item)
				continue
			}
			hidden, known := visibility[item.WorkspaceID]
			if !known {
				var err error
				hidden, err = workspaceDeletionExists(r.store.dataRoot, "eventstream", item.WorkspaceID)
				if err != nil {
					return nil, err
				}
				visibility[item.WorkspaceID] = hidden
			}
			if !hidden {
				visible = append(visible, item)
			}
		}
		ret = visible
		if query.RunID != "" {
			matchedWorkspaces := map[common.WorkspaceID]struct{}{}
			for _, item := range ret {
				if item != nil {
					matchedWorkspaces[item.WorkspaceID] = struct{}{}
				}
			}
			if len(matchedWorkspaces) > 1 {
				return nil, common.ErrConflict
			}
		}
	} else {
		hidden, err := workspaceDeletionExists(r.store.dataRoot, "eventstream", query.WorkspaceID)
		if err != nil {
			return nil, err
		}
		if hidden {
			return nil, nil
		}
	}
	repository.SortEventRecords(ret, query)
	if query.RunID == "" || query.WorkspaceID == "" {
		ret = applyEventRecordCursorAndLimit(ret, query)
	}
	return nilIfEmpty(ret), nil
}

func (r *workspaceScopedEventRecordRepository) Delete(ctx context.Context, id common.EventRecordID) error {
	if id == "" {
		return common.ErrInvalidArgument
	}
	current, err := r.Get(ctx, id)
	if err != nil {
		return err
	}
	deleteRecord := func(guarded context.Context) error {
		storagePath := r.store.systemPath(workspaceScopedEventRecordFile)
		if current.WorkspaceID != "" && current.RunID != "" {
			storagePath = r.store.runPath(r.store.runRoot(current.WorkspaceID, current.RunID), workspaceScopedEventRecordFile)
		}
		lock := workspaceDeletionFileLock(storagePath)
		lock.Lock()
		defer lock.Unlock()
		return r.store.eventRecordRepo(storagePath).Delete(guarded, id)
	}
	if current.WorkspaceID == "" {
		return deleteRecord(ctx)
	}
	return withWorkspaceDeletionMutation(ctx, r.store.dataRoot, "eventstream", current.WorkspaceID, deleteRecord)
}

func (r *workspaceScopedEventRecordRepository) PurgeWorkspaceEventRecords(ctx context.Context, expected eventstreammodel.WorkspaceDeletion) (int, error) {
	if r == nil || r.store == nil || r.deletions == nil || expected.Validate() != nil ||
		expected.Phase != eventstreammodel.WorkspaceDeletionPhaseFenced || expected.Mode != deletioncontract.ModePurge {
		return 0, common.ErrInvalidArgument
	}
	return r.purgeWorkspaceEventRecordsPhysical(ctx, expected)
}

func (r *workspaceScopedEventRecordRepository) purgeWorkspaceEventRecordsPhysical(ctx context.Context, expected eventstreammodel.WorkspaceDeletion) (int, error) {
	deleted := 0
	paths := []string{r.store.systemPath(workspaceScopedEventRecordFile)}
	for _, root := range r.store.workspaceRunRoots(expected.WorkspaceID) {
		paths = append(paths, r.store.runPath(root, workspaceScopedEventRecordFile))
	}
	for _, path := range paths {
		purger := fileRepo.NewEventStreamWorkspacePurger(r.store.eventRecordRepo(path), r.deletions, workspaceDeletionFileLock(path))
		count, err := purger.PurgeWorkspaceEventRecords(ctx, expected)
		if err != nil {
			return deleted, err
		}
		deleted += count
	}
	return deleted, nil
}

func (r *workspaceScopedEventRecordRepository) visibleEventRecord(item *eventstreammodel.EventRecord) (*eventstreammodel.EventRecord, error) {
	if item == nil || item.WorkspaceID == "" {
		return item, nil
	}
	hidden, err := workspaceDeletionExists(r.store.dataRoot, "eventstream", item.WorkspaceID)
	if err != nil {
		return nil, err
	}
	if hidden {
		return nil, common.ErrNotFound
	}
	return item, nil
}

func (r *workspaceScopedEventRecordRepository) getEventRecordFromPath(ctx context.Context, path string, id common.EventRecordID) (*eventstreammodel.EventRecord, error) {
	lock := workspaceDeletionFileLock(path)
	lock.Lock()
	defer lock.Unlock()
	return r.store.eventRecordRepo(path).Get(ctx, id)
}

func (r *workspaceScopedEventRecordRepository) listEventRecordsFromPath(ctx context.Context, path string, query workorchquerycontract.EventRecordQuery) ([]*eventstreammodel.EventRecord, error) {
	lock := workspaceDeletionFileLock(path)
	lock.Lock()
	defer lock.Unlock()
	return r.store.eventRecordRepo(path).List(ctx, query)
}

type workspaceScopedDecisionTicketRepository struct {
	store *workspaceScopedFileStore
}

func newWorkspaceScopedDecisionTicketRepository(store *workspaceScopedFileStore) *workspaceScopedDecisionTicketRepository {
	return &workspaceScopedDecisionTicketRepository{store: store}
}

func (r *workspaceScopedDecisionTicketRepository) Create(ctx context.Context, item *decisionmodel.Ticket) error {
	if item == nil || item.WorkspaceID == "" || item.RunID == "" || item.ID == "" {
		return common.ErrInvalidArgument
	}
	return withWorkspaceDeletionMutation(ctx, r.store.dataRoot, "decision", item.WorkspaceID, func(guarded context.Context) error {
		return r.store.ticketRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedTicketFile)).Create(guarded, item)
	})
}

func (r *workspaceScopedDecisionTicketRepository) Get(ctx context.Context, id common.TicketID) (*decisionmodel.Ticket, error) {
	if id == "" {
		return nil, common.ErrInvalidArgument
	}
	return findUniqueInPaths(r.store.workspaceRunRoots(common.RuntimeWorkspaceIDFromContext(ctx)), func(root string) (*decisionmodel.Ticket, error) {
		return r.store.ticketRepo(r.store.runPath(root, workspaceScopedTicketFile)).Get(ctx, id)
	})
}

func (r *workspaceScopedDecisionTicketRepository) List(ctx context.Context) ([]*decisionmodel.Ticket, error) {
	ret := []*decisionmodel.Ticket{}
	for _, root := range r.store.allRunRoots() {
		items, err := r.store.ticketRepo(r.store.runPath(root, workspaceScopedTicketFile)).List(ctx)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
	}
	sortTickets(ret)
	return ret, nil
}

func (r *workspaceScopedDecisionTicketRepository) ListByRunID(ctx context.Context, runID common.RunID) ([]*decisionmodel.Ticket, error) {
	if runID == "" {
		return nil, common.ErrInvalidArgument
	}
	roots, err := r.store.runRootsByRunID(ctx, runID)
	if err != nil {
		if err == common.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	for _, root := range roots {
		items, err := r.store.ticketRepo(r.store.runPath(root, workspaceScopedTicketFile)).ListByRunID(ctx, runID)
		if err != nil {
			return nil, err
		}
		if len(items) > 0 {
			sortTickets(items)
			return items, nil
		}
	}
	return nil, nil
}

func (r *workspaceScopedDecisionTicketRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*decisionmodel.Ticket, error) {
	ret := []*decisionmodel.Ticket{}
	for _, root := range r.store.workspaceRunRoots(workspaceID) {
		items, err := r.store.ticketRepo(r.store.runPath(root, workspaceScopedTicketFile)).ListByWorkspaceID(ctx, workspaceID)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
	}
	sortTickets(ret)
	return ret, nil
}

func (r *workspaceScopedDecisionTicketRepository) Update(ctx context.Context, item *decisionmodel.Ticket) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if item.WorkspaceID != "" && item.RunID != "" {
		err := withWorkspaceDeletionMutation(ctx, r.store.dataRoot, "decision", item.WorkspaceID, func(guarded context.Context) error {
			return r.store.ticketRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedTicketFile)).Update(guarded, item)
		})
		if err == nil || err != common.ErrNotFound {
			return err
		}
	}
	current, err := r.Get(ctx, item.ID)
	if err != nil {
		return err
	}
	item.WorkspaceID = current.WorkspaceID
	item.RunID = current.RunID
	return withWorkspaceDeletionMutation(ctx, r.store.dataRoot, "decision", item.WorkspaceID, func(guarded context.Context) error {
		return r.store.ticketRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedTicketFile)).Update(guarded, item)
	})
}

func (r *workspaceScopedDecisionTicketRepository) Delete(ctx context.Context, id common.TicketID) error {
	if id == "" {
		return common.ErrInvalidArgument
	}
	current, err := r.Get(ctx, id)
	if err != nil {
		return err
	}
	return withWorkspaceDeletionMutation(ctx, r.store.dataRoot, "decision", current.WorkspaceID, func(guarded context.Context) error {
		return r.store.ticketRepo(r.store.runPath(r.store.runRoot(current.WorkspaceID, current.RunID), workspaceScopedTicketFile)).Delete(guarded, id)
	})
}

type workspaceScopedDecisionResolutionRepository struct {
	store   *workspaceScopedFileStore
	tickets *workspaceScopedDecisionTicketRepository
}

func newWorkspaceScopedDecisionResolutionRepository(store *workspaceScopedFileStore, tickets *workspaceScopedDecisionTicketRepository) *workspaceScopedDecisionResolutionRepository {
	return &workspaceScopedDecisionResolutionRepository{store: store, tickets: tickets}
}

func (r *workspaceScopedDecisionResolutionRepository) Create(ctx context.Context, item *decisionmodel.Resolution) error {
	if item == nil || item.TicketID == "" {
		return common.ErrInvalidArgument
	}
	ticket, err := r.tickets.Get(ctx, item.TicketID)
	if err != nil {
		return err
	}
	return withWorkspaceDeletionMutation(ctx, r.store.dataRoot, "decision", ticket.WorkspaceID, func(guarded context.Context) error {
		return r.store.resolutionRepo(r.store.runPath(r.store.runRoot(ticket.WorkspaceID, ticket.RunID), workspaceScopedResolutionFile)).Create(guarded, item)
	})
}

func (r *workspaceScopedDecisionResolutionRepository) GetByTicketID(ctx context.Context, ticketID common.TicketID) (*decisionmodel.Resolution, error) {
	if ticketID == "" {
		return nil, common.ErrInvalidArgument
	}
	if ticket, err := r.tickets.Get(ctx, ticketID); err == nil {
		item, err := r.store.resolutionRepo(r.store.runPath(r.store.runRoot(ticket.WorkspaceID, ticket.RunID), workspaceScopedResolutionFile)).GetByTicketID(ctx, ticketID)
		if err == nil || err != common.ErrNotFound {
			return item, err
		}
	} else if err != common.ErrNotFound {
		return nil, err
	}
	return findUniqueInPaths(r.store.workspaceRunRoots(common.RuntimeWorkspaceIDFromContext(ctx)), func(root string) (*decisionmodel.Resolution, error) {
		return r.store.resolutionRepo(r.store.runPath(root, workspaceScopedResolutionFile)).GetByTicketID(ctx, ticketID)
	})
}

func (r *workspaceScopedDecisionResolutionRepository) DeleteByTicketID(ctx context.Context, ticketID common.TicketID) error {
	if ticketID == "" {
		return common.ErrInvalidArgument
	}
	if ticket, err := r.tickets.Get(ctx, ticketID); err == nil {
		err := withWorkspaceDeletionMutation(ctx, r.store.dataRoot, "decision", ticket.WorkspaceID, func(guarded context.Context) error {
			return r.store.resolutionRepo(r.store.runPath(r.store.runRoot(ticket.WorkspaceID, ticket.RunID), workspaceScopedResolutionFile)).DeleteByTicketID(guarded, ticketID)
		})
		if err == nil || err != common.ErrNotFound {
			return err
		}
	} else if err != common.ErrNotFound {
		return err
	}
	root, err := findUniqueInPaths(r.store.workspaceRunRoots(common.RuntimeWorkspaceIDFromContext(ctx)), func(root string) (string, error) {
		if _, err := r.store.resolutionRepo(r.store.runPath(root, workspaceScopedResolutionFile)).GetByTicketID(ctx, ticketID); err != nil {
			return "", err
		}
		return root, nil
	})
	if err != nil {
		return err
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if workspaceID == "" {
		return common.ErrConflict
	}
	return withWorkspaceDeletionMutation(ctx, r.store.dataRoot, "decision", workspaceID, func(guarded context.Context) error {
		return r.store.resolutionRepo(r.store.runPath(root, workspaceScopedResolutionFile)).DeleteByTicketID(guarded, ticketID)
	})
}

type workspaceScopedApprovalRepository struct {
	store *workspaceScopedFileStore
}

func newWorkspaceScopedApprovalRepository(store *workspaceScopedFileStore) *workspaceScopedApprovalRepository {
	return &workspaceScopedApprovalRepository{store: store}
}

func (r *workspaceScopedApprovalRepository) Create(ctx context.Context, item *approvalmodel.Approval) error {
	if item == nil || item.WorkspaceID == "" || item.RunID == "" || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if scoped := common.RuntimeWorkspaceIDFromContext(ctx); scoped != "" && scoped != item.WorkspaceID {
		return common.ErrConflict
	}
	return withWorkspaceDeletionMutation(ctx, r.store.dataRoot, "approval", item.WorkspaceID, func(guarded context.Context) error {
		return r.store.approvalRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedApprovalFile)).Create(guarded, item)
	})
}

func (r *workspaceScopedApprovalRepository) Get(ctx context.Context, id common.ApprovalID) (*approvalmodel.Approval, error) {
	if id == "" {
		return nil, common.ErrInvalidArgument
	}
	return findUniqueInPaths(r.store.workspaceRunRoots(common.RuntimeWorkspaceIDFromContext(ctx)), func(root string) (*approvalmodel.Approval, error) {
		return r.store.approvalRepo(r.store.runPath(root, workspaceScopedApprovalFile)).Get(ctx, id)
	})
}

func (r *workspaceScopedApprovalRepository) GetScoped(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, id common.ApprovalID) (*approvalmodel.Approval, error) {
	if workspaceID == "" || runID == "" || id == "" {
		return nil, common.ErrInvalidArgument
	}
	if scoped := common.RuntimeWorkspaceIDFromContext(ctx); scoped != "" && scoped != workspaceID {
		return nil, common.ErrConflict
	}
	return r.store.approvalRepo(r.store.runPath(r.store.runRoot(workspaceID, runID), workspaceScopedApprovalFile)).GetScoped(ctx, workspaceID, runID, id)
}

func (r *workspaceScopedApprovalRepository) List(ctx context.Context) ([]*approvalmodel.Approval, error) {
	ret := []*approvalmodel.Approval{}
	for _, root := range r.store.allRunRoots() {
		items, err := r.store.approvalRepo(r.store.runPath(root, workspaceScopedApprovalFile)).List(ctx)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
	}
	sortApprovals(ret)
	return ret, nil
}

func (r *workspaceScopedApprovalRepository) ListByRunID(ctx context.Context, runID common.RunID) ([]*approvalmodel.Approval, error) {
	if runID == "" {
		return nil, common.ErrInvalidArgument
	}
	roots, err := r.store.runRootsByRunID(ctx, runID)
	if err != nil {
		if err == common.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	for _, root := range roots {
		items, err := r.store.approvalRepo(r.store.runPath(root, workspaceScopedApprovalFile)).ListByRunID(ctx, runID)
		if err != nil {
			return nil, err
		}
		if len(items) > 0 {
			sortApprovals(items)
			return items, nil
		}
	}
	return nil, nil
}

func (r *workspaceScopedApprovalRepository) ListByRunScope(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) ([]*approvalmodel.Approval, error) {
	if workspaceID == "" || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	if scoped := common.RuntimeWorkspaceIDFromContext(ctx); scoped != "" && scoped != workspaceID {
		return nil, common.ErrConflict
	}
	return r.store.approvalRepo(r.store.runPath(r.store.runRoot(workspaceID, runID), workspaceScopedApprovalFile)).ListByRunScope(ctx, workspaceID, runID)
}

func (r *workspaceScopedApprovalRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*approvalmodel.Approval, error) {
	if workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	if scoped := common.RuntimeWorkspaceIDFromContext(ctx); scoped != "" && scoped != workspaceID {
		return nil, common.ErrConflict
	}
	ret := []*approvalmodel.Approval{}
	for _, root := range r.store.workspaceRunRoots(workspaceID) {
		items, err := r.store.approvalRepo(r.store.runPath(root, workspaceScopedApprovalFile)).ListByWorkspaceID(ctx, workspaceID)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
	}
	sortApprovals(ret)
	return ret, nil
}

func (r *workspaceScopedApprovalRepository) Update(ctx context.Context, item *approvalmodel.Approval) error {
	if item == nil || item.ID == "" || item.WorkspaceID == "" || item.RunID == "" {
		return common.ErrInvalidArgument
	}
	if scoped := common.RuntimeWorkspaceIDFromContext(ctx); scoped != "" && scoped != item.WorkspaceID {
		return common.ErrConflict
	}
	if item.WorkspaceID != "" && item.RunID != "" {
		err := withWorkspaceDeletionMutation(ctx, r.store.dataRoot, "approval", item.WorkspaceID, func(guarded context.Context) error {
			return r.store.approvalRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedApprovalFile)).Update(guarded, item)
		})
		if err == nil || err != common.ErrNotFound {
			return err
		}
	}
	current, err := r.Get(ctx, item.ID)
	if err != nil {
		return err
	}
	item.WorkspaceID = current.WorkspaceID
	item.RunID = current.RunID
	return withWorkspaceDeletionMutation(ctx, r.store.dataRoot, "approval", item.WorkspaceID, func(guarded context.Context) error {
		return r.store.approvalRepo(r.store.runPath(r.store.runRoot(item.WorkspaceID, item.RunID), workspaceScopedApprovalFile)).Update(guarded, item)
	})
}

func (r *workspaceScopedApprovalRepository) Delete(ctx context.Context, id common.ApprovalID) error {
	if id == "" {
		return common.ErrInvalidArgument
	}
	current, err := r.Get(ctx, id)
	if err != nil {
		return err
	}
	return withWorkspaceDeletionMutation(ctx, r.store.dataRoot, "approval", current.WorkspaceID, func(guarded context.Context) error {
		return r.store.approvalRepo(r.store.runPath(r.store.runRoot(current.WorkspaceID, current.RunID), workspaceScopedApprovalFile)).Delete(guarded, id)
	})
}

func (r *workspaceScopedApprovalRepository) DeleteScoped(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, id common.ApprovalID) error {
	if workspaceID == "" || runID == "" || id == "" {
		return common.ErrInvalidArgument
	}
	if scoped := common.RuntimeWorkspaceIDFromContext(ctx); scoped != "" && scoped != workspaceID {
		return common.ErrConflict
	}
	return withWorkspaceDeletionMutation(ctx, r.store.dataRoot, "approval", workspaceID, func(guarded context.Context) error {
		return r.store.approvalRepo(r.store.runPath(r.store.runRoot(workspaceID, runID), workspaceScopedApprovalFile)).DeleteScoped(guarded, workspaceID, runID, id)
	})
}

func nilIfEmpty[T any](items []T) []T {
	if len(items) == 0 {
		return nil
	}
	return items
}

func nilIfEmptyArtifactContinuations(items []*orchestratormodel.ArtifactContinuation) []*orchestratormodel.ArtifactContinuation {
	if len(items) == 0 {
		return nil
	}
	return items
}

func sortRuns(items []*runmodel.Run) {
	sort.Slice(items, func(i, j int) bool { return string(items[i].ID) < string(items[j].ID) })
}

func sortWorkspaces(items []*workspacemodel.Workspace) {
	sort.Slice(items, func(i, j int) bool { return string(items[i].ID) < string(items[j].ID) })
}

func sortMemoryItems(items []*memorymodel.MemoryItem, query workorchquerycontract.MemorySearchQuery) {
	sort.Slice(items, func(i, j int) bool {
		if len(query.Vector) > 0 && items[i].VectorScore != items[j].VectorScore {
			return items[i].VectorScore > items[j].VectorScore
		}
		if items[i].Importance != items[j].Importance {
			return items[i].Importance > items[j].Importance
		}
		return string(items[i].ID) < string(items[j].ID)
	})
}

func sortMemoryRecallTraces(items []*memorymodel.MemoryRecallTrace) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].OccurredAt != items[j].OccurredAt {
			return items[i].OccurredAt < items[j].OccurredAt
		}
		return string(items[i].ID) < string(items[j].ID)
	})
}

func sortSteps(items []*runmodel.Step) {
	sort.Slice(items, func(i, j int) bool { return string(items[i].ID) < string(items[j].ID) })
}

func sortToolCalls(items []*runtimeevidencemodel.ToolCall) {
	sort.Slice(items, func(i, j int) bool { return string(items[i].ID) < string(items[j].ID) })
}

func sortPatches(items []*runtimeevidencemodel.Patch) {
	sort.Slice(items, func(i, j int) bool { return string(items[i].ID) < string(items[j].ID) })
}

func sortValidationResults(items []*runtimeevidencemodel.ValidationResult) {
	sort.Slice(items, func(i, j int) bool { return string(items[i].ID) < string(items[j].ID) })
}

func sortRuntimeCheckpoints(items []*runmodel.RuntimeCheckpoint) {
	sort.Slice(items, func(i, j int) bool { return string(items[i].ID) < string(items[j].ID) })
}

func sortEvidenceRecords(items []*orchestratormodel.EvidenceRecord) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].RunID != items[j].RunID {
			return string(items[i].RunID) < string(items[j].RunID)
		}
		if items[i].StepID != items[j].StepID {
			return string(items[i].StepID) < string(items[j].StepID)
		}
		if items[i].CreatedAt != items[j].CreatedAt {
			return items[i].CreatedAt < items[j].CreatedAt
		}
		return string(items[i].ID) < string(items[j].ID)
	})
}

func sortCoverageWorkItems(items []*orchestratormodel.CoverageWorkItem) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].RunID != items[j].RunID {
			return string(items[i].RunID) < string(items[j].RunID)
		}
		if items[i].DefinitionRevision != items[j].DefinitionRevision {
			return items[i].DefinitionRevision < items[j].DefinitionRevision
		}
		if items[i].NormalizedSourceKey != items[j].NormalizedSourceKey {
			return items[i].NormalizedSourceKey < items[j].NormalizedSourceKey
		}
		return string(items[i].ID) < string(items[j].ID)
	})
}

func sortArtifactContinuations(items []*orchestratormodel.ArtifactContinuation) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].RunID != items[j].RunID {
			return string(items[i].RunID) < string(items[j].RunID)
		}
		return string(items[i].ID) < string(items[j].ID)
	})
}

func sortResults(items []*resultmodel.Result) {
	sort.Slice(items, func(i, j int) bool { return string(items[i].ID) < string(items[j].ID) })
}

func sortArtifacts(items []*resultmodel.Artifact) {
	sort.Slice(items, func(i, j int) bool { return string(items[i].ID) < string(items[j].ID) })
}

func sortTickets(items []*decisionmodel.Ticket) {
	sort.Slice(items, func(i, j int) bool { return string(items[i].ID) < string(items[j].ID) })
}

func sortApprovals(items []*approvalmodel.Approval) {
	sort.Slice(items, func(i, j int) bool { return string(items[i].ID) < string(items[j].ID) })
}

func applyEventRecordCursorAndLimit(items []*eventstreammodel.EventRecord, query workorchquerycontract.EventRecordQuery) []*eventstreammodel.EventRecord {
	if query.CursorAfterID != "" {
		cursorMatched := false
		filtered := items[:0]
		for _, item := range items {
			if !cursorMatched {
				if item.ID == query.CursorAfterID {
					cursorMatched = true
				}
				continue
			}
			filtered = append(filtered, item)
		}
		items = filtered
	}
	if query.Limit > 0 && len(items) > query.Limit {
		if query.CursorAfterID == "" {
			items = items[len(items)-query.Limit:]
		} else {
			items = items[:query.Limit]
		}
	}
	return items
}
