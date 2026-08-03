package events

const NotificationDestination = "/orchestrator/consumers/#"
const PrerequisiteCommandDestination = "/runtime/prerequisite"

const (
	TopicOrchestratorCommandPurgeRun                       = "/orchestrator/command/purge-run"
	TopicOrchestratorCommandListToolCallsByRun             = "/orchestrator/command/list-tool-calls-by-run"
	TopicOrchestratorCommandListPatchesByRun               = "/orchestrator/command/list-patches-by-run"
	TopicOrchestratorCommandListValidationResultsByRun     = "/orchestrator/command/list-validation-results-by-run"
	TopicOrchestratorCommandListCoverageWorkItemsByRun     = "/orchestrator/command/list-coverage-work-items-by-run"
	TopicOrchestratorCommandListArtifactContinuationsByRun = "/orchestrator/command/list-artifact-continuations-by-run"

	TopicOrchestratorDriveRuntime                      = "/orchestrator/runtime/drive"
	TopicOrchestratorRetryRuntimeDrive                 = "/orchestrator/internal/retry-runtime-drive"
	TopicOrchestratorQueryRuntimeOperationReceipt      = "/orchestrator/runtime-operation/receipt/query"
	TopicOrchestratorCommitRuntimeOperationReceipt     = "/orchestrator/runtime-operation/receipt/commit"
	TopicOrchestratorCommitRuntimeGovernanceResolution = "/orchestrator/runtime-governance/commit-resolution"
	TopicOrchestratorCommitRuntimeGovernanceTicket     = "/orchestrator/runtime-governance/commit-ticket-resolution"
	TopicOrchestratorRequestRuntimeTermination         = "/orchestrator/runtime-lifecycle/request-termination"
	// RUNTIME-SMO facts and SM-4 delivery ledger.
	TopicOrchestratorRuntimeGovernanceWaitRecorded    = "/orchestrator/runtime-lifecycle/governance-wait-recorded"
	TopicOrchestratorRuntimeContinuationEligible      = "/orchestrator/runtime-lifecycle/continuation-eligible"
	TopicOrchestratorRuntimeTerminalDeliveryRequested = "/orchestrator/runtime-lifecycle/terminal-delivery-requested"
	// TopicOrchestratorRuntimeDriveObserved is a read-only SM-4 observability
	// notification. Consumers may render metrics/logs but must not use it to
	// resume, mutate, or rebuild Runtime authority.
	TopicOrchestratorRuntimeDriveObserved  = "/orchestrator/runtime/drive-observed"
	TopicPatchPreviewed                    = "/patch/previewed"
	TopicPatchApplied                      = "/patch/applied"
	TopicPatchFailed                       = "/patch/failed"
	TopicPatchReverted                     = "/patch/reverted"
	TopicArtifactContinuationCreated       = "/artifact_continuation/created"
	TopicArtifactContinuationChunkRecorded = "/artifact_continuation/chunk_recorded"
	TopicArtifactContinuationChunkRejected = "/artifact_continuation/chunk_rejected"
	TopicArtifactContinuationAssembled     = "/artifact_continuation/assembled"
	TopicArtifactContinuationApplied       = "/artifact_continuation/applied"
	TopicArtifactContinuationFailed        = "/artifact_continuation/failed"
)
