package orchestrator

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	orchestratorbiz "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/common"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	orchestratorservice "github.com/muidea/workorch/internal/modules/kernel/orchestrator/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const ModuleID = modulecommon.ModuleID

type Service = orchestratorservice.Service

type Dependencies struct {
	ToolCalls             repository.ToolCallRepository
	Patches               repository.PatchRepository
	ValidationResults     repository.ValidationResultRepository
	EvidenceRecords       repository.EvidenceRecordRepository
	CoverageWorkItems     repository.CoverageWorkItemRepository
	ArtifactContinuations repository.ArtifactContinuationRepository
	CommandHub            magicevent.Hub
	BackgroundRoutine     task.BackgroundRoutine
}

type Module struct {
	bizPtr            *orchestratorbiz.RuntimeOrchestrator
	servicePtr        *orchestratorservice.Module
	evidenceRecords   repository.EvidenceRecordRepository
	rawEventHub       magicevent.Hub
	rawObserver       magicevent.SimpleObserver
	rawTopics         []string
	backgroundRoutine task.BackgroundRoutine
	driveMu           sync.Mutex
	drivingRuns       map[string]struct{}
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Configure(deps Dependencies) error {
	if deps.ToolCalls == nil ||
		deps.Patches == nil ||
		deps.ValidationResults == nil ||
		deps.EvidenceRecords == nil ||
		deps.CoverageWorkItems == nil ||
		deps.ArtifactContinuations == nil ||
		deps.CommandHub == nil {
		return common.ErrInvalidArgument
	}
	m.bizPtr = orchestratorbiz.NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runEventRepository{hub: deps.CommandHub},
		workspaceEventRepository{hub: deps.CommandHub},
		decisionEventTicketRepository{hub: deps.CommandHub},
		decisionEventResolutionRepository{hub: deps.CommandHub},
		approvalEventRepository{hub: deps.CommandHub},
		orchestratorbiz.RuntimeEvidenceRepositories{
			Results:               resultEventResultRepository{hub: deps.CommandHub},
			Artifacts:             resultEventArtifactRepository{hub: deps.CommandHub},
			ToolCalls:             deps.ToolCalls,
			Patches:               deps.Patches,
			ValidationResults:     deps.ValidationResults,
			EvidenceRecords:       deps.EvidenceRecords,
			CoverageWorkItems:     deps.CoverageWorkItems,
			ArtifactContinuations: deps.ArtifactContinuations,
			MemoryItems:           memoryEventRepository{hub: deps.CommandHub},
			MemoryRecallTraces:    memoryRecallTraceEventRepository{hub: deps.CommandHub},
		},
	)

	m.servicePtr = orchestratorservice.New(orchestratorservice.Dependencies{
		BlockOnDecision: m.bizPtr.BlockOnDecision,
		BlockOnApproval: m.bizPtr.BlockOnApproval,
		CancelRun:       m.bizPtr.CancelRun,
		SupersedeRun:    m.bizPtr.SupersedeRun,
		FailRun:         m.bizPtr.FailRun,
	})
	if err := m.bizPtr.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: deps.CommandHub}); err != nil {
		return err
	}
	m.evidenceRecords = deps.EvidenceRecords
	m.rawEventHub = deps.CommandHub
	m.backgroundRoutine = deps.BackgroundRoutine
	return nil
}

func (m *Module) Start() error {
	if m == nil || m.servicePtr == nil || m.rawEventHub == nil {
		return common.ErrInvalidArgument
	}
	if m.rawObserver != nil {
		return nil
	}
	if m.rawEventHub != nil && m.rawObserver == nil {
		m.rawObserver = magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, m.rawEventHub)
		m.rawTopics = []string{
			orchestratorevents.TopicOrchestratorCommandStartRun,
			orchestratorevents.TopicOrchestratorCommandBlockDecision,
			orchestratorevents.TopicOrchestratorCommandBlockApproval,
			orchestratorevents.TopicOrchestratorCommandResumeRun,
			orchestratorevents.TopicOrchestratorCommandCancelRun,
			orchestratorevents.TopicOrchestratorCommandSupersedeRun,
			orchestratorevents.TopicOrchestratorCommandFailRun,
			orchestratorevents.TopicOrchestratorCommandPurgeRun,
			orchestratorevents.TopicOrchestratorCommandListToolCallsByRun,
			orchestratorevents.TopicOrchestratorCommandListPatchesByRun,
			orchestratorevents.TopicOrchestratorCommandListValidationResultsByRun,
			orchestratorevents.TopicOrchestratorCommandListCoverageWorkItemsByRun,
			orchestratorevents.TopicOrchestratorCommandListArtifactContinuationsByRun,
		}
		for _, topic := range m.rawTopics {
			m.rawObserver.Subscribe(topic, m.handleMagicCommand)
		}
		m.rawTopics = append(m.rawTopics,
			orchestratorevents.TopicOrchestratorDriveRuntimeV2,
			orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceResolution,
			orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceTicket,
			decisionevents.TopicDecisionCommandRecordEvidence,
			approvalevents.TopicApprovalCommandRecordEvidence,
		)
		m.rawObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, m.handleRuntimeV2Drive)
		m.rawObserver.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceResolution, m.handleRuntimeV2GovernanceResolution)
		m.rawObserver.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceTicket, m.handleRuntimeV2GovernanceTicketResolution)
		m.rawObserver.Subscribe(decisionevents.TopicDecisionCommandRecordEvidence, m.handleDecisionEvidenceCommand)
		m.rawObserver.Subscribe(approvalevents.TopicApprovalCommandRecordEvidence, m.handleApprovalEvidenceCommand)
	}
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	_ = ctx
	if m.rawObserver != nil {
		for idx := len(m.rawTopics) - 1; idx >= 0; idx-- {
			m.rawObserver.Unsubscribe(m.rawTopics[idx])
		}
	}
	m.rawObserver = nil
	m.rawTopics = nil
	m.servicePtr = nil
	m.bizPtr = nil
	m.evidenceRecords = nil
	m.rawEventHub = nil
	m.backgroundRoutine = nil
	m.driveMu.Lock()
	m.drivingRuns = nil
	m.driveMu.Unlock()
	return nil
}

func (m *Module) Service() Service {
	return m.servicePtr
}

func (m *Module) handleMagicCommand(ev magicevent.Event, result magicevent.Result) {
	response, err := m.executeMagicCommand(ev)
	setOrchestratorCommandResult(result, response, err)
}

func (m *Module) executeMagicCommand(ev magicevent.Event) (orchestratorevents.OrchestratorCommandResult, error) {
	if m == nil || m.servicePtr == nil {
		return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
	}
	ctx := ev.Context()
	switch ev.ID() {
	case orchestratorevents.TopicOrchestratorCommandStartRun:
		cmd, ok := ev.Data().(orchestratorevents.StartRunCommand)
		if !ok || cmd.RunID == "" {
			return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
		}
		return orchestratorevents.OrchestratorCommandResult{}, m.handleStartRunCommand(ctx, common.RuntimeWorkspaceIDFromContext(ctx), cmd.RunID)
	case orchestratorevents.TopicOrchestratorCommandBlockDecision:
		cmd, ok := ev.Data().(orchestratorevents.BlockDecisionCommand)
		if !ok || cmd.RunID == "" {
			return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
		}
		return orchestratorevents.OrchestratorCommandResult{}, m.servicePtr.BlockOnDecision(ctx, cmd.RunID, cmd.TicketID)
	case orchestratorevents.TopicOrchestratorCommandBlockApproval:
		cmd, ok := ev.Data().(orchestratorevents.BlockApprovalCommand)
		if !ok || cmd.RunID == "" {
			return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
		}
		return orchestratorevents.OrchestratorCommandResult{}, m.servicePtr.BlockOnApproval(ctx, cmd.RunID, cmd.ApprovalID)
	case orchestratorevents.TopicOrchestratorCommandResumeRun:
		cmd, ok := ev.Data().(orchestratorevents.ResumeRunCommand)
		if !ok || cmd.RunID == "" {
			return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
		}
		return orchestratorevents.OrchestratorCommandResult{}, m.resumeRunMagicCommand(ctx, cmd)
	case orchestratorevents.TopicOrchestratorCommandCancelRun:
		cmd, ok := ev.Data().(orchestratorevents.CancelRunCommand)
		if !ok || cmd.RunID == "" {
			return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
		}
		return orchestratorevents.OrchestratorCommandResult{}, m.servicePtr.CancelRun(ctx, cmd.RunID)
	case orchestratorevents.TopicOrchestratorCommandSupersedeRun:
		cmd, ok := ev.Data().(orchestratorevents.SupersedeRunCommand)
		if !ok || cmd.RunID == "" {
			return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
		}
		return orchestratorevents.OrchestratorCommandResult{}, m.servicePtr.SupersedeRun(ctx, cmd.RunID)
	case orchestratorevents.TopicOrchestratorCommandFailRun:
		cmd, ok := ev.Data().(orchestratorevents.FailRunCommand)
		if !ok || cmd.RunID == "" {
			return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
		}
		return orchestratorevents.OrchestratorCommandResult{}, m.servicePtr.FailRun(ctx, cmd.RunID, cmd.Reason)
	case orchestratorevents.TopicOrchestratorCommandPurgeRun:
		cmd, ok := ev.Data().(orchestratorevents.PurgeRunCommand)
		if !ok || cmd.WorkspaceID == "" || cmd.RunID == "" {
			return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
		}
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, cmd.WorkspaceID)
		runResult, err := m.purgeRunOwnerResources(ctx, cmd.WorkspaceID, cmd.RunID)
		if err != nil {
			return orchestratorevents.OrchestratorCommandResult{}, err
		}
		deleted, err := purgeRunEvidence(ctx, m.bizPtr.EvidenceRepositories(), cmd.RunID)
		return orchestratorevents.OrchestratorCommandResult{DeletedCount: runResult.DeletedCount + deleted}, err
	case orchestratorevents.TopicOrchestratorCommandListToolCallsByRun,
		orchestratorevents.TopicOrchestratorCommandListPatchesByRun,
		orchestratorevents.TopicOrchestratorCommandListValidationResultsByRun,
		orchestratorevents.TopicOrchestratorCommandListCoverageWorkItemsByRun,
		orchestratorevents.TopicOrchestratorCommandListArtifactContinuationsByRun:
		return m.readEvidenceMagicCommand(ev)
	default:
		return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
	}
}

func setOrchestratorCommandResult(result magicevent.Result, data orchestratorevents.OrchestratorCommandResult, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(data, nil)
}

func setOrchestratorAck(result magicevent.Result, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(nil, nil)
}

func (m *Module) handleStartRunCommand(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) error {
	if m == nil || m.bizPtr == nil || workspaceID == "" || runID == "" {
		return common.ErrInvalidArgument
	}
	if err := m.bizPtr.AcceptRunStart(ctx, runID); err != nil {
		return err
	}
	return m.scheduleRuntimeV2Drive(ctx, workspaceID, runID, "start")
}

func (m *Module) resumeRunMagicCommand(ctx context.Context, cmd orchestratorevents.ResumeRunCommand) error {
	if m == nil || m.bizPtr == nil || cmd.WorkspaceID == "" || cmd.RunID == "" {
		return common.ErrInvalidArgument
	}
	if err := m.bizPtr.CanResumeRun(ctx, cmd.RunID); err != nil {
		return err
	}
	if err := m.bizPtr.ActivateRuntimeV2Run(ctx, cmd.WorkspaceID, cmd.RunID); err != nil {
		return err
	}
	return m.scheduleRuntimeV2Drive(ctx, cmd.WorkspaceID, cmd.RunID, "resume")
}

func (m *Module) scheduleRuntimeV2Drive(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, source string) error {
	if m == nil || m.bizPtr == nil || workspaceID == "" || runID == "" {
		return common.ErrInvalidArgument
	}
	driveKey := string(workspaceID) + "\x00" + string(runID)
	if !m.beginRuntimeV2Drive(driveKey) {
		return nil
	}
	drive := func(driveCtx context.Context) {
		defer m.endRuntimeV2Drive(driveKey)
		if err := m.driveAndProjectRuntimeV2(driveCtx, workspaceID, runID); err != nil && !errors.Is(err, context.Canceled) {
			var projectionErr *runtimeV2ProjectionError
			if errors.As(err, &projectionErr) {
				slog.Warn("workorch Runtime V2 projection failed", "source", source, "workspaceID", workspaceID, "runID", runID, "err", err)
				return
			}
			slog.Warn("workorch Runtime V2 drive failed", "source", source, "workspaceID", workspaceID, "runID", runID, "err", err)
			aggregate, recordErr := m.bizPtr.RecordRuntimeV2FailureFact(driveCtx, workspaceID, runID, runtimecontract.FailureFact{
				Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "runtime_v2_drive_failed",
				Signature: "runtime-v2-drive-failed", Message: err.Error(),
			})
			if recordErr != nil {
				slog.Warn("workorch Runtime V2 drive failure fact commit failed", "source", source, "workspaceID", workspaceID, "runID", runID, "err", recordErr)
				return
			}
			if aggregate.Decision.Action != runtimecontract.RuntimeActionFail || aggregate.Decision.Terminal == nil {
				slog.Warn("workorch Runtime V2 drive failure did not reduce to terminal", "source", source, "workspaceID", workspaceID, "runID", runID)
				return
			}
			if projectErr := m.projectRuntimeV2Result(driveCtx, aggregate); projectErr != nil {
				slog.Warn("workorch Runtime V2 drive failure result projection failed", "source", source, "workspaceID", workspaceID, "runID", runID, "err", projectErr)
				return
			}
			if projectErr := m.bizPtr.ProjectRuntimeV2Terminal(driveCtx, workspaceID, runID, aggregate.Decision.Action, aggregate.Decision.Terminal); projectErr != nil {
				slog.Warn("workorch Runtime V2 drive failure terminal projection failed", "source", source, "workspaceID", workspaceID, "runID", runID, "err", projectErr)
			}
		}
	}
	bgCtx := orchestratorBackgroundContext(ctx, workspaceID)
	if m.backgroundRoutine == nil {
		drive(bgCtx)
		return nil
	}
	if err := m.backgroundRoutine.AsyncFunction(func() { drive(bgCtx) }); err != nil {
		m.endRuntimeV2Drive(driveKey)
		return err
	}
	return nil
}

func (m *Module) beginRuntimeV2Drive(key string) bool {
	if m == nil || key == "" {
		return false
	}
	m.driveMu.Lock()
	defer m.driveMu.Unlock()
	if m.drivingRuns == nil {
		m.drivingRuns = map[string]struct{}{}
	}
	if _, ok := m.drivingRuns[key]; ok {
		return false
	}
	m.drivingRuns[key] = struct{}{}
	return true
}

func (m *Module) endRuntimeV2Drive(key string) {
	if m == nil || key == "" {
		return
	}
	m.driveMu.Lock()
	delete(m.drivingRuns, key)
	m.driveMu.Unlock()
}

func orchestratorBackgroundContext(ctx context.Context, workspaceID common.WorkspaceID) context.Context {
	bgCtx := context.Background()
	if workspaceID != "" {
		bgCtx = common.ContextWithRuntimeWorkspaceID(bgCtx, workspaceID)
	} else if workspaceID := common.RuntimeWorkspaceIDFromContext(ctx); workspaceID != "" {
		bgCtx = common.ContextWithRuntimeWorkspaceID(bgCtx, workspaceID)
	}

	return bgCtx
}

func (m *Module) readEvidenceMagicCommand(ev magicevent.Event) (orchestratorevents.OrchestratorCommandResult, error) {
	if m == nil || m.bizPtr == nil {
		return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
	}
	cmd, ok := ev.Data().(orchestratorevents.ListRunEvidenceCommand)
	if !ok || cmd.RunID == "" {
		return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
	}
	ctx := ev.Context()
	if cmd.WorkspaceID != "" {
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, cmd.WorkspaceID)
	}
	evidence := m.bizPtr.EvidenceRepositories()
	switch ev.ID() {
	case orchestratorevents.TopicOrchestratorCommandListToolCallsByRun:
		if evidence.ToolCalls == nil {
			return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
		}
		items, err := evidence.ToolCalls.ListByRun(ctx, cmd.RunID)
		return orchestratorevents.OrchestratorCommandResult{ToolCalls: items}, err
	case orchestratorevents.TopicOrchestratorCommandListPatchesByRun:
		if evidence.Patches == nil {
			return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
		}
		items, err := evidence.Patches.ListByRun(ctx, cmd.RunID)
		return orchestratorevents.OrchestratorCommandResult{Patches: items}, err
	case orchestratorevents.TopicOrchestratorCommandListValidationResultsByRun:
		if evidence.ValidationResults == nil {
			return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
		}
		items, err := evidence.ValidationResults.ListByRun(ctx, cmd.RunID)
		return orchestratorevents.OrchestratorCommandResult{ValidationResults: items}, err
	case orchestratorevents.TopicOrchestratorCommandListCoverageWorkItemsByRun:
		if evidence.CoverageWorkItems == nil {
			return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
		}
		items, err := evidence.CoverageWorkItems.ListByRun(ctx, cmd.RunID)
		return orchestratorevents.OrchestratorCommandResult{CoverageWorkItems: items}, err
	case orchestratorevents.TopicOrchestratorCommandListArtifactContinuationsByRun:
		if evidence.ArtifactContinuations == nil {
			return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
		}
		items, err := evidence.ArtifactContinuations.ListByRun(ctx, cmd.RunID)
		return orchestratorevents.OrchestratorCommandResult{ArtifactContinuations: items}, err
	default:
		return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
	}
}
