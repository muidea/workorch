package orchestrator

import (
	"context"

	runtimemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type RuntimeAdapter interface {
	ExecuteRuntimeStep(context.Context, runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error)
}

const (
	stepOutputKeyNextStep               = "nextStep"
	stepOutputKeyNextKind               = "nextKind"
	stepOutputKeyNextInput              = "nextInput"
	stepOutputKeyPatches                = "patches"
	stepOutputKeyMemoryWrites           = "memoryWrites"
	stepOutputKeyMemoryRefs             = "memoryRefs"
	stepOutputKeyDoDAssessment          = "dodAssessment"
	stepOutputKeyLLMDecision            = "llmDecision"
	stepOutputKeyExecutionGraph         = "executionGraph"
	stepOutputKeyGraphBranch            = "graphBranch"
	stepInputKeyGraphNodeID             = "graphNodeId"
	stepInputKeyGraphRootStepID         = "graphRootStepId"
	stepInputKeyGraphPreviousNodeID     = "graphPreviousNodeId"
	stepInputKeyGraphCompletedNodes     = "graphCompletedNodes"
	stepInputKeyGraphBatch              = "graphBatch"
	stepInputKeyGraphEvidence           = "graphEvidence"
	stepInputKeyRunEvidenceSummary      = "runEvidenceSummary"
	stepInputKeyProcessEvidenceTimeline = "processEvidenceTimeline"
	stepInputKeyCurrentProgressDigest   = "currentProgressDigest"
	stepInputKeyWorkspaceFileManifest   = "workspaceFileManifest"
	stepInputKeySoulStage               = "soulStage"
	stepInputKeyRuntimeRepairFollowUp   = "runtimeRepairFollowUp"
	stepInputKeyRuntimeRepairAttempt    = "runtimeRepairAttempt"
	stepInputKeyRepairOfStepID          = "repairOfStepId"
	stepInputKeyRepairOfRootStepID      = "repairOfRootStepId"
	stepInputKeyRepairOfFailureClass    = "repairOfFailureClass"
	stepInputKeyRepairOfFailureSig      = "repairOfFailureSignature"
	stepInputKeyProviderRetryFollowUp   = "providerRetryFollowUp"
	stepInputKeyProviderRetryAttempt    = "providerRetryAttempt"
	stepInputKeyProviderRetryRootStep   = "providerRetryRootStepId"
	stepInputKeyRecoveryAction          = "recoveryAction"
	stepInputKeyRecoveryRootStepID      = "recoveryRootStepId"
	stepInputKeyRecoveryLineageType     = "recoveryLineageType"
	stepInputKeyRecoveryBudgetScope     = "recoveryBudgetScope"
	stepInputKeyNoActionableFollowUp    = "noActionableContinuationFollowUp"
	stepInputKeyAssessmentOnly          = "assessmentOnly"
	stepInputKeyGraphLimitFollowUp      = "graphExpansionLimitFollowUp"
	artifactAssembleStepKind            = "artifact/assemble"
	artifactApplyStepKind               = "artifact/apply"
	stepOutputKeyBlockDecisionReason    = "blockDecisionReason"
	stepOutputKeyBlockDecisionTicketID  = "blockDecisionTicketId"
	stepOutputKeyBlockDecisionKind      = "blockDecisionKind"
	stepOutputKeyBlockDecisionQuestion  = "blockDecisionQuestion"
	stepOutputKeyBlockDecisionContext   = "blockDecisionContext"
	stepOutputKeyBlockApprovalReason    = "blockApprovalReason"
	stepOutputKeyBlockApprovalID        = "blockApprovalId"
	stepOutputKeyBlockApprovalAction    = "blockApprovalAction"
	stepOutputKeyBlockApprovalContext   = "blockApprovalContext"
	llmWorkspaceExecutionStepKind       = "llm/workspace_execution"
	defaultInitialStepKind              = llmWorkspaceExecutionStepKind
	soulStagePlanning                   = "planning"
	soulStageExecution                  = "execution"
	soulStageDecision                   = "decision"
	soulStageApproval                   = "approval"
	soulStageValidation                 = "validation"
	defaultMaxRuntimeRepairAttempts     = 10
	maxProviderRetryAttempts            = 10
	defaultMaxPatchContentBytes         = 12288
	defaultMaxGraphBytes                = 32768
	defaultMaxGraphFileWrites           = 3
	defaultSchemaInvalidPreviewBytes    = 8192
	defaultMaxArtifactChunkBytes        = 8192
	defaultPreviousChunkTailBytes       = 300
	defaultMaxChunkRetryAttempts        = 3
	defaultMaxArtifactPlanAttempts      = 2
	defaultMaxArtifactReplanAttempts    = 2
	maxGraphFollowUpExpansionAttempts   = 2
)

type RuntimePolicy struct {
	MaxRuntimeRepairAttempts int
	MaxProviderRetryAttempts int
	LLMOutput                LLMOutputRuntimePolicy
}

type LLMOutputRuntimePolicy struct {
	MaxPatchContentBytes               int
	MaxGraphBytes                      int
	MaxGraphFileWrites                 int
	SchemaInvalidLargePreviewThreshold int
	MaxArtifactChunkBytes              int
	PreviousChunkTailBytes             int
	MaxChunkRetryAttempts              int
	MaxArtifactPlanAttempts            int
	MaxArtifactReplanAttempts          int
}

type RuntimeOrchestrator struct {
	runtime     RuntimeAdapter
	runs        repository.RunRepository
	steps       repository.StepRepository
	workspaces  repository.WorkspaceRepository
	tickets     repository.DecisionTicketRepository
	resolutions repository.DecisionResolutionRepository
	approvals   repository.ApprovalRepository
	evidence    RuntimeEvidenceRepositories
	eventHub    events.Hub
	policy      RuntimePolicy
	checkpoints *checkpointSubsystem
	eventer     *eventSubsystem
	llmContext  *llmContextSubsystem
	repair      *repairSubsystem
	graph       *graphSubsystem
	convergence *convergenceSubsystem
	coverage    *coverageSubsystem
	patches     *patchSubsystem
	results     *resultSubsystem
	runState    *runStateSubsystem
	decision    *decisionResumeSubsystem
}

type RuntimeEvidenceRepositories struct {
	Results               repository.ResultRepository
	Artifacts             repository.ArtifactRepository
	ToolCalls             repository.ToolCallRepository
	Patches               repository.PatchRepository
	ValidationResults     repository.ValidationResultRepository
	RuntimeCheckpoints    repository.RuntimeCheckpointRepository
	ArtifactContinuations repository.ArtifactContinuationRepository
	MemoryItems           repository.MemoryItemRepository
	MemoryRecallTraces    repository.MemoryRecallTraceRepository
	AutoGovernanceTraces  repository.AutoGovernanceTraceRepository
}

type stepDispatchSpec struct {
	ID    common.StepID
	Kind  string
	Input map[string]any
}

func NewRuntimeOrchestrator(
	runtime RuntimeAdapter,
	runRepo repository.RunRepository,
	stepRepo repository.StepRepository,
	workspaceRepo repository.WorkspaceRepository,
	ticketRepo repository.DecisionTicketRepository,
	approvalRepo repository.ApprovalRepository,
	eventHub events.Hub,
) *RuntimeOrchestrator {
	return NewRuntimeOrchestratorWithResolutions(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, nil, approvalRepo, eventHub)
}

func NewRuntimeOrchestratorWithResolutions(
	runtime RuntimeAdapter,
	runRepo repository.RunRepository,
	stepRepo repository.StepRepository,
	workspaceRepo repository.WorkspaceRepository,
	ticketRepo repository.DecisionTicketRepository,
	resolutionRepo repository.DecisionResolutionRepository,
	approvalRepo repository.ApprovalRepository,
	eventHub events.Hub,
) *RuntimeOrchestrator {
	return NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		resolutionRepo,
		approvalRepo,
		RuntimeEvidenceRepositories{},
		eventHub,
	)
}

func NewRuntimeOrchestratorWithResolutionsAndEvidence(
	runtime RuntimeAdapter,
	runRepo repository.RunRepository,
	stepRepo repository.StepRepository,
	workspaceRepo repository.WorkspaceRepository,
	ticketRepo repository.DecisionTicketRepository,
	resolutionRepo repository.DecisionResolutionRepository,
	approvalRepo repository.ApprovalRepository,
	evidence RuntimeEvidenceRepositories,
	eventHub events.Hub,
	policies ...RuntimePolicy,
) *RuntimeOrchestrator {
	if eventHub == nil {
		eventHub = events.NewMemoryHub()
	}
	policy := NormalizeRuntimePolicy(RuntimePolicy{})
	if len(policies) > 0 {
		policy = NormalizeRuntimePolicy(policies[0])
	}
	orchestrator := &RuntimeOrchestrator{
		runtime:     runtime,
		runs:        runRepo,
		steps:       stepRepo,
		workspaces:  workspaceRepo,
		tickets:     ticketRepo,
		resolutions: resolutionRepo,
		approvals:   approvalRepo,
		evidence:    evidence,
		eventHub:    eventHub,
		policy:      policy,
		llmContext:  newLLMContextSubsystem(),
		repair:      newRepairSubsystem(stepRepo, workspaceRepo),
		graph:       newGraphSubsystem(stepRepo, nil),
		convergence: newConvergenceSubsystem(),
		coverage:    newCoverageSubsystem(),
		runState:    newRunStateSubsystem(runRepo, workspaceRepo),
		decision:    newDecisionResumeSubsystem(ticketRepo, resolutionRepo),
	}
	orchestrator.graph.closures = orchestrator
	orchestrator.checkpoints = newCheckpointSubsystem(evidence, stepRepo)
	orchestrator.eventer = newEventSubsystem(eventHub, orchestrator.checkpoints, orchestrator.llmContext)
	orchestrator.patches = newPatchSubsystem(evidence, approvalRepo, orchestrator.eventPublisher())
	orchestrator.results = newResultSubsystem(evidence, stepRepo, orchestrator.eventPublisher())
	return orchestrator
}

func (o *RuntimeOrchestrator) checkpointManager() *checkpointSubsystem {
	if o == nil {
		return newCheckpointSubsystem(RuntimeEvidenceRepositories{}, nil)
	}
	if o.checkpoints == nil {
		o.checkpoints = newCheckpointSubsystem(o.evidence, o.steps)
	}
	return o.checkpoints
}

func (o *RuntimeOrchestrator) eventPublisher() *eventSubsystem {
	if o == nil {
		return newEventSubsystem(nil, nil, nil)
	}
	if o.eventer == nil {
		o.eventer = newEventSubsystem(o.eventHub, o.checkpointManager(), o.llmContextManager())
	}
	return o.eventer
}

func (o *RuntimeOrchestrator) graphManager() *graphSubsystem {
	if o == nil {
		return newGraphSubsystem(nil, nil)
	}
	if o.graph == nil {
		o.graph = newGraphSubsystem(o.steps, o)
	}
	return o.graph
}

func (o *RuntimeOrchestrator) llmContextManager() *llmContextSubsystem {
	if o == nil || o.llmContext == nil {
		return newLLMContextSubsystem()
	}
	return o.llmContext
}

func (o *RuntimeOrchestrator) repairManager() *repairSubsystem {
	if o == nil {
		return newRepairSubsystem(nil, nil)
	}
	if o.repair == nil {
		o.repair = newRepairSubsystem(o.steps, o.workspaces)
	}
	return o.repair
}

func (o *RuntimeOrchestrator) convergenceManager() *convergenceSubsystem {
	if o == nil || o.convergence == nil {
		return newConvergenceSubsystem()
	}
	return o.convergence
}

func (o *RuntimeOrchestrator) coverageLedger() *coverageSubsystem {
	if o == nil || o.coverage == nil {
		return newCoverageSubsystem()
	}
	return o.coverage
}

func (o *RuntimeOrchestrator) patchManager() *patchSubsystem {
	if o == nil {
		return newPatchSubsystem(RuntimeEvidenceRepositories{}, nil, nil)
	}
	if o.patches == nil {
		o.patches = newPatchSubsystem(o.evidence, o.approvals, o.eventPublisher())
	}
	return o.patches
}

func (o *RuntimeOrchestrator) resultManager() *resultSubsystem {
	if o == nil {
		return newResultSubsystem(RuntimeEvidenceRepositories{}, nil, nil)
	}
	if o.results == nil {
		o.results = newResultSubsystem(o.evidence, o.steps, o.eventPublisher())
	} else {
		o.results.evidence = o.evidence
		o.results.steps = o.steps
		o.results.events = o.eventPublisher()
	}
	return o.results
}

func (o *RuntimeOrchestrator) runStateManager() *runStateSubsystem {
	if o == nil {
		return newRunStateSubsystem(nil, nil)
	}
	if o.runState == nil {
		o.runState = newRunStateSubsystem(o.runs, o.workspaces)
	} else {
		o.runState.runs = o.runs
		o.runState.workspaces = o.workspaces
	}
	return o.runState
}

func (o *RuntimeOrchestrator) decisionResumeManager() *decisionResumeSubsystem {
	if o == nil {
		return newDecisionResumeSubsystem(nil, nil)
	}
	if o.decision == nil {
		o.decision = newDecisionResumeSubsystem(o.tickets, o.resolutions)
	} else {
		o.decision.tickets = o.tickets
		o.decision.resolutions = o.resolutions
	}
	return o.decision
}

func NormalizeRuntimePolicy(policy RuntimePolicy) RuntimePolicy {
	if policy.MaxRuntimeRepairAttempts <= 0 {
		policy.MaxRuntimeRepairAttempts = defaultMaxRuntimeRepairAttempts
	}
	if policy.MaxProviderRetryAttempts <= 0 {
		policy.MaxProviderRetryAttempts = maxProviderRetryAttempts
	}
	policy.LLMOutput = NormalizeLLMOutputRuntimePolicy(policy.LLMOutput)
	return policy
}

func NormalizeLLMOutputRuntimePolicy(policy LLMOutputRuntimePolicy) LLMOutputRuntimePolicy {
	if policy.MaxPatchContentBytes <= 0 {
		policy.MaxPatchContentBytes = defaultMaxPatchContentBytes
	}
	if policy.MaxGraphBytes <= 0 {
		policy.MaxGraphBytes = defaultMaxGraphBytes
	}
	if policy.MaxGraphFileWrites <= 0 {
		policy.MaxGraphFileWrites = defaultMaxGraphFileWrites
	}
	if policy.SchemaInvalidLargePreviewThreshold <= 0 {
		policy.SchemaInvalidLargePreviewThreshold = defaultSchemaInvalidPreviewBytes
	}
	if policy.MaxArtifactChunkBytes <= 0 {
		policy.MaxArtifactChunkBytes = defaultMaxArtifactChunkBytes
	}
	if policy.PreviousChunkTailBytes <= 0 {
		policy.PreviousChunkTailBytes = defaultPreviousChunkTailBytes
	}
	if policy.MaxChunkRetryAttempts <= 0 {
		policy.MaxChunkRetryAttempts = defaultMaxChunkRetryAttempts
	}
	if policy.MaxArtifactPlanAttempts <= 0 {
		policy.MaxArtifactPlanAttempts = defaultMaxArtifactPlanAttempts
	}
	if policy.MaxArtifactReplanAttempts <= 0 {
		policy.MaxArtifactReplanAttempts = defaultMaxArtifactReplanAttempts
	}
	return policy
}

func (p LLMOutputRuntimePolicy) Values() map[string]any {
	normalized := NormalizeLLMOutputRuntimePolicy(p)
	return map[string]any{
		"maxPatchContentBytes":               normalized.MaxPatchContentBytes,
		"maxGraphBytes":                      normalized.MaxGraphBytes,
		"maxGraphFileWrites":                 normalized.MaxGraphFileWrites,
		"schemaInvalidLargePreviewThreshold": normalized.SchemaInvalidLargePreviewThreshold,
		"maxArtifactChunkBytes":              normalized.MaxArtifactChunkBytes,
		"previousChunkTailBytes":             normalized.PreviousChunkTailBytes,
		"maxChunkRetryAttempts":              normalized.MaxChunkRetryAttempts,
		"maxArtifactPlanAttempts":            normalized.MaxArtifactPlanAttempts,
		"maxArtifactReplanAttempts":          normalized.MaxArtifactReplanAttempts,
	}
}

func llmOutputPolicyValues(policy RuntimePolicy) map[string]any {
	return policy.LLMOutput.Values()
}

func maxRuntimeRepairAttemptsForPolicy(policy RuntimePolicy) int {
	if policy.MaxRuntimeRepairAttempts <= 0 {
		return defaultMaxRuntimeRepairAttempts
	}
	return policy.MaxRuntimeRepairAttempts
}

func maxProviderRetryAttemptsForPolicy(policy RuntimePolicy) int {
	if policy.MaxProviderRetryAttempts <= 0 {
		return maxProviderRetryAttempts
	}
	return policy.MaxProviderRetryAttempts
}
