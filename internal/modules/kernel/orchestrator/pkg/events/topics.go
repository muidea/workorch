package events

const NotificationDestination = "/orchestrator/consumers/#"
const PrerequisiteCommandDestination = "/runtime/prerequisite"

const (
	TopicOrchestratorCommandBeginWorkspaceRuntimeQuiescence   = "/orchestrator/command/begin-workspace-runtime-quiescence"
	TopicOrchestratorCommandConfirmWorkspaceRuntimeQuiescence = "/orchestrator/command/confirm-workspace-runtime-quiescence"
	TopicOrchestratorCommandSealWorkspaceDeletion             = "/orchestrator/command/seal-workspace-deletion"
	TopicOrchestratorCommandPurgeWorkspaceEvidence            = "/orchestrator/command/purge-workspace-evidence"
	TopicOrchestratorCommandRecordDecisionEvidence            = "/orchestrator/command/record-decision-evidence"
	TopicOrchestratorCommandRecordApprovalEvidence            = "/orchestrator/command/record-approval-evidence"
	TopicOrchestratorCommandListToolCallsByRun                = "/orchestrator/command/list-tool-calls-by-run"
	TopicOrchestratorCommandListPatchesByRun                  = "/orchestrator/command/list-patches-by-run"
	TopicOrchestratorCommandListValidationResultsByRun        = "/orchestrator/command/list-validation-results-by-run"
	TopicOrchestratorCommandListCoverageWorkItemsByRun        = "/orchestrator/command/list-coverage-work-items-by-run"
	TopicOrchestratorCommandListArtifactContinuationsByRun    = "/orchestrator/command/list-artifact-continuations-by-run"

	TopicOrchestratorDriveRuntime                      = "/orchestrator/runtime/drive"
	TopicOrchestratorQueryRuntimeOperationReceipt      = "/orchestrator/runtime-operation/receipt/query"
	TopicOrchestratorCommitRuntimeOperationReceipt     = "/orchestrator/runtime-operation/receipt/commit"
	TopicOrchestratorCommitRuntimeGovernanceResolution = "/orchestrator/runtime-governance/commit-resolution"
	TopicOrchestratorCommitRuntimeGovernanceApproval   = "/orchestrator/runtime-governance/commit-approval"
	TopicOrchestratorCommitRuntimeGovernanceTicket     = "/orchestrator/runtime-governance/commit-ticket-resolution"
	TopicOrchestratorRequestRuntimeTermination         = "/orchestrator/runtime-lifecycle/request-termination"
	// RUNTIME-SMO observability facts.
	// TopicOrchestratorRuntimeDriveObserved is a read-only SM-4 observability
	// notification. Consumers may render metrics/logs but must not use it to
	// resume, mutate, or rebuild Runtime authority.
	TopicOrchestratorRuntimeDriveObserved = "/orchestrator/runtime/drive-observed"
)
