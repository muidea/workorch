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
	WorkspaceID                   common.WorkspaceID
	RunID                         common.RunID
	AuthoritySnapshotID           string
	PrerequisiteID                string
	WorkspaceDeliverableDirectory string
	TargetRole                    string
	TargetSelectorRef             string
	TargetSelectorOwnerRef        string
	TargetSelectorSubjectRef      string
	TargetSelectorCandidateRefs   []string
	ProspectiveTargetRef          string
	MaxItems                      int
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
// Runtime action or mutating Runtime authority.
type ConfirmRuntimeRepairSurfaceCommand struct {
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
	Data FileListOperationData
}

type ExecuteFileReadOperationCommand struct {
	runtimecontract.RuntimeOperationExecutionContext
	Data FileReadOperationData
}

type ExecuteFileEditOperationCommand struct {
	runtimecontract.RuntimeOperationExecutionContext
	Data FileEditOperationData
}

type ExecuteFileWriteOperationCommand struct {
	runtimecontract.RuntimeOperationExecutionContext
	Data FileWriteOperationData
}

type ExecutePatchApplyOperationCommand struct {
	runtimecontract.RuntimeOperationExecutionContext
	Data PatchApplyOperationData
}

type ExecutePatchPreviewOperationCommand struct {
	runtimecontract.RuntimeOperationExecutionContext
	Data PatchApplyOperationData
}

type ExecuteOperationResult struct {
	Receipt runtimecontract.RuntimeOperationReceipt
}

func MessageContracts() []eventcontract.MessageContract {
	return eventcontract.MessageContracts(
		eventcontract.CommandTopic[AcquireRuntimeCapabilitiesCommand, AcquireRuntimeCapabilitiesResult]("file", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicAcquireRuntimeCapabilities),
		eventcontract.CommandTopic[AcquireRuntimeOperationAdmissibilityCommand, AcquireRuntimeOperationAdmissibilityResult]("file", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicAcquireRuntimeOperationAdmissibility),
		eventcontract.CommandTopic[ProbeRuntimeOperationFeasibilityCommand, ProbeRuntimeOperationFeasibilityResult]("file", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicProbeRuntimeOperationFeasibility),
		eventcontract.CommandTopic[ProbeRuntimeSemanticProofFeasibilityCommand, ProbeRuntimeSemanticProofFeasibilityResult]("file", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicProbeRuntimeSemanticProofFeasibility),
		eventcontract.CommandTopic[QueryRuntimeCWIReconciliationCommand, QueryRuntimeCWIReconciliationResult]("file", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicQueryRuntimeCWIReconciliation),
		eventcontract.CommandTopic[AcquireTargetSnapshotCommand, AcquireTargetSnapshotResult]("file", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicAcquireTargetSnapshot),
		eventcontract.CommandTopic[AcquireRuntimeTargetSelectionCommand, AcquireRuntimeTargetSelectionResult]("file", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicAcquireRuntimeTargetSelection),
		eventcontract.CommandTopic[AcquireWorkspaceManifestCommand, AcquireWorkspaceManifestResult]("file", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicAcquireWorkspaceManifest),
		eventcontract.CommandTopic[ConfirmRuntimeRepairSurfaceCommand, ConfirmRuntimeRepairSurfaceResult]("file", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicConfirmRuntimeRepairSurface),
		eventcontract.CommandTopic[ExecuteFileListOperationCommand, ExecuteOperationResult]("file", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicExecuteFileListOperation),
		eventcontract.CommandTopic[ExecuteFileReadOperationCommand, ExecuteOperationResult]("file", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicExecuteFileReadOperation),
		eventcontract.CommandTopic[ExecuteFileEditOperationCommand, ExecuteOperationResult]("file", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicExecuteFileEditOperation),
		eventcontract.CommandTopic[ExecuteFileWriteOperationCommand, ExecuteOperationResult]("file", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicExecuteFileWriteOperation),
		eventcontract.CommandTopic[ExecutePatchApplyOperationCommand, ExecuteOperationResult]("file", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicExecutePatchApplyOperation),
		eventcontract.CommandTopic[ExecutePatchPreviewOperationCommand, ExecuteOperationResult]("file", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicExecutePatchPreviewOperation),
	)
}
