package events

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const definitionPackage = "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"

type AcquireTargetSnapshotCommand struct {
	WorkspaceID                   common.WorkspaceID
	RunID                         common.RunID
	AuthoritySnapshotID           string
	PrerequisiteID                string
	WorkspaceDeliverableDirectory string
	TargetRef                     string
	MaxBytes                      int
}

type AcquireTargetSnapshotResult struct {
	WorkspaceID         common.WorkspaceID
	RunID               common.RunID
	AuthoritySnapshotID string
	Snapshot            runtimecontract.TargetSnapshot
	Observation         runtimecontract.RuntimePrerequisiteObservation
}

// AcquireRuntimeTargetSelectionCommand asks File for a bounded, read-only
// target-selection observation. TargetRole remains opaque to the owner.
type AcquireRuntimeTargetSelectionCommand struct {
	WorkspaceID                          common.WorkspaceID
	RunID                                common.RunID
	AuthoritySnapshotID                  string
	PrerequisiteID                       string
	WorkspaceDeliverableDirectory        string
	TargetRole                           string
	TargetSelectorRef                    string
	TargetSelectorOwnerRef               string
	TargetSelectorSubjectRef             string
	TargetSelectorCandidateRefs          []string
	ProspectiveTargetAllOfConstraintRefs []string
	ProspectiveTargetRef                 string
	MaxItems                             int
}

type AcquireRuntimeTargetSelectionResult struct {
	WorkspaceID         common.WorkspaceID
	RunID               common.RunID
	AuthoritySnapshotID string
	Selection           runtimecontract.RuntimeTargetSelection
	Observation         runtimecontract.RuntimePrerequisiteObservation
}

type AcquireWorkspaceManifestCommand struct {
	WorkspaceID                   common.WorkspaceID
	RunID                         common.RunID
	AuthoritySnapshotID           string
	PrerequisiteID                string
	WorkspaceDeliverableDirectory string
}

type AcquireWorkspaceManifestResult struct {
	WorkspaceID         common.WorkspaceID
	RunID               common.RunID
	AuthoritySnapshotID string
	Manifest            runtimecontract.WorkspaceManifest
	Observation         runtimecontract.RuntimePrerequisiteObservation
}

// ConfirmRuntimeRepairSurfaceCommand asks File to verify a bounded validation
// observation against its current workspace inventory without choosing a
// Runtime action or mutating Runtime authority. RequestID,
// IssuedAtStateRevision and ExpectedSubjectRevision are opaque Orchestrator
// fences that File must echo. File compares the expected revision with its
// newly acquired workspace manifest before classifying target availability.
type ConfirmRuntimeRepairSurfaceCommand struct {
	RequestID                     string
	IssuedAtStateRevision         uint64
	ExpectedSubjectRevision       string
	WorkspaceID                   common.WorkspaceID
	RunID                         common.RunID
	WorkspaceDeliverableDirectory string
	Observation                   runtimecontract.RuntimeValidationFailureObservation
}

type ConfirmRuntimeRepairSurfaceResult struct {
	Confirmation runtimecontract.RuntimeRepairSurfaceConfirmation
}

type FileListOperationData struct {
	Path       string
	Recursive  bool
	MaxEntries int64
}

type FileReadOperationData struct {
	Path      string
	Optional  bool
	MaxBytes  int64
	StartLine int64
	EndLine   int64
}

type FileEditOperationData struct {
	Path       string
	OldText    string
	NewText    string
	BaseHash   string
	Occurrence int64
}

type FileWriteOperationData struct {
	TargetRef      string
	Content        string
	ContentPresent bool
}

type PatchApplyOperationData struct {
	TargetRef       string
	PatchType       string
	Content         string
	ContentPresent  bool
	Diff            string
	OldText         string
	NewText         string
	NewTextPresent  bool
	BaseHash        string
	Occurrence      int64
	CoverageTargets []string
	Summary         string
}

// Concrete commands remain File-owned EventHub roots while the sealed,
// owner-neutral execution scope is shared by all capability owners.
type ExecuteFileListOperationCommand struct {
	runtimecontract.RuntimeOperationExecutionContext
	SealedOperation SealedRuntimeOperation
}

type ExecuteFileReadOperationCommand struct {
	runtimecontract.RuntimeOperationExecutionContext
	SealedOperation SealedRuntimeOperation
}

type ExecuteFileEditOperationCommand struct {
	runtimecontract.RuntimeOperationExecutionContext
	SealedOperation           SealedRuntimeOperation
	ResultDeliveryProtocolRef string
}

type ExecuteFileWriteOperationCommand struct {
	runtimecontract.RuntimeOperationExecutionContext
	SealedOperation           SealedRuntimeOperation
	ResultDeliveryProtocolRef string
}

type ExecutePatchApplyOperationCommand struct {
	runtimecontract.RuntimeOperationExecutionContext
	SealedOperation           SealedRuntimeOperation
	ResultDeliveryProtocolRef string
}

const MutationResultDeliveryV1 = "file/mutation-result-delivery/v1"

// Delivers only previously saved owner material. Missing material never grants
// a physical call, even when the original execution is still claimed.
type DeliverMutationResultCommand struct {
	runtimecontract.RuntimeOperationExecutionContext
	SealedOperation SealedRuntimeOperation
}
type DeliverMutationResultResult struct{ Found bool }

type ExecutePatchPreviewOperationCommand struct {
	runtimecontract.RuntimeOperationExecutionContext
	SealedOperation SealedRuntimeOperation
}

type ExecuteOperationResult struct {
	Receipt runtimecontract.RuntimeOperationReceipt
}

func MessageContracts() []eventcontract.MessageContract {
	return eventcontract.MessageContracts(
		eventcontract.CommandTopic[DeliverMutationResultCommand, DeliverMutationResultResult]("file", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyExecute, TopicDeliverMutationResult),
		eventcontract.CommandTopic[ReobservePreparationFilesCommand, ReobservePreparationFilesResult]("file", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicReobservePreparationFiles),
		eventcontract.CommandTopic[InitializePreparationResourcesCommand, InitializePreparationResourcesResult]("file", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyExecute, TopicInitializePreparationResources),
		eventcontract.CommandTopic[QueryPreparationEpochCommand, QueryPreparationEpochResult]("file", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicQueryPreparationEpoch),
		eventcontract.CommandTopic[ObservePreparationDirectoriesCommand, ObservePreparationDirectoriesResult]("file", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicObservePreparationDirectories),
		eventcontract.CommandTopic[ObservePreparationResourcesCommand, ObservePreparationResourcesResult]("file", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicObservePreparationResources),
		eventcontract.CommandTopic[ObservePreparationManagedConfigsCommand, ObservePreparationManagedConfigsResult]("file", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicObservePreparationManagedConfigs),
		eventcontract.CommandTopic[AcquireRuntimeCapabilitiesCommand, AcquireRuntimeCapabilitiesResult]("file", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicAcquireRuntimeCapabilities),
		eventcontract.CommandTopic[AcquireRuntimeOperationAdmissibilityCommand, AcquireRuntimeOperationAdmissibilityResult]("file", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicAcquireRuntimeOperationAdmissibility),
		eventcontract.CommandTopic[ValidateRuntimeOperationCandidateConformanceCommand, ValidateRuntimeOperationCandidateConformanceResult]("file", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicValidateRuntimeOperationCandidateConformance),
		eventcontract.CommandTopic[ProbeRuntimeOperationFeasibilityCommand, ProbeRuntimeOperationFeasibilityResult]("file", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicProbeRuntimeOperationFeasibility),
		eventcontract.CommandTopic[ProbeRuntimeSemanticProofFeasibilityCommand, ProbeRuntimeSemanticProofFeasibilityResult]("file", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicProbeRuntimeSemanticProofFeasibility),
		eventcontract.CommandTopic[QueryRuntimeCWIReconciliationCommand, QueryRuntimeCWIReconciliationResult]("file", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicQueryRuntimeCWIReconciliation),
		eventcontract.CommandTopic[QueryRuntimeFileReadDependenciesCommand, QueryRuntimeFileReadDependenciesResult]("file", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicQueryRuntimeFileReadDependencies),
		eventcontract.CommandTopic[AcquireTargetSnapshotCommand, AcquireTargetSnapshotResult]("file", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicAcquireTargetSnapshot),
		eventcontract.CommandTopic[AcquireRuntimeTargetSelectionCommand, AcquireRuntimeTargetSelectionResult]("file", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicAcquireRuntimeTargetSelection),
		eventcontract.CommandTopic[AcquireWorkspaceManifestCommand, AcquireWorkspaceManifestResult]("file", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicAcquireWorkspaceManifest),
		eventcontract.CommandTopic[ConfirmRuntimeRepairSurfaceCommand, ConfirmRuntimeRepairSurfaceResult]("file", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicConfirmRuntimeRepairSurface),
		eventcontract.CommandTopic[AcquireAuthoringResourceCommand, AcquireAuthoringResourceResult]("file", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicAcquireAuthoringResources),
		eventcontract.CommandTopic[VerifyAuthoringResourceCommand, VerifyAuthoringResourceResult]("file", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicVerifyAuthoringResource),
		eventcontract.CommandTopic[ExecuteFileListOperationCommand, ExecuteOperationResult]("file", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicExecuteFileListOperation),
		eventcontract.CommandTopic[ExecuteFileReadOperationCommand, ExecuteOperationResult]("file", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicExecuteFileReadOperation),
		eventcontract.CommandTopic[ExecuteFileEditOperationCommand, ExecuteOperationResult]("file", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicExecuteFileEditOperation),
		eventcontract.CommandTopic[ExecuteFileWriteOperationCommand, ExecuteOperationResult]("file", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicExecuteFileWriteOperation),
		eventcontract.CommandTopic[ExecutePatchApplyOperationCommand, ExecuteOperationResult]("file", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicExecutePatchApplyOperation),
		eventcontract.CommandTopic[ExecutePatchPreviewOperationCommand, ExecuteOperationResult]("file", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicExecutePatchPreviewOperation),
	)
}
