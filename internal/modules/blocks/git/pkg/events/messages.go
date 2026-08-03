package events

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const definitionPackage = "github.com/muidea/workorch/internal/modules/blocks/git/pkg/events"

type AcquireCapabilityStateCommand struct {
	WorkspaceID                   common.WorkspaceID
	RunID                         common.RunID
	AuthoritySnapshotID           string
	PrerequisiteID                string
	WorkspaceDeliverableDirectory string
	CapabilityStateKind           string
	SubjectRef                    string
	AcquisitionKey                string
}

type AcquireCapabilityStateResult struct {
	WorkspaceID         common.WorkspaceID
	RunID               common.RunID
	AuthoritySnapshotID string
	Snapshot            runtimecontract.RuntimeCapabilityStateSnapshot
	Observation         runtimecontract.RuntimePrerequisiteObservation
}

type OperationData struct {
	Operation     string
	TimeoutMillis int64
	Pathspecs     []string
	Ref           string
	BaseRef       string
	Limit         int64
	Message       string
	ExpectedHead  string
}

type ExecuteOperationCommand struct {
	Execution                          runtimecontract.RuntimeOperationExecutionEnvelope
	WorkspaceID                        common.WorkspaceID
	RunID                              common.RunID
	DispatchID                         string
	Transaction                        runtimecontract.AuthorityTransaction
	OperationID                        string
	OperationKind                      string
	RequiredOperationRef               string
	CapabilityStates                   []runtimecontract.RuntimeCapabilityStateSnapshot
	CompletedRequiredOperationReceipts []runtimecontract.RuntimeRequiredOperationReceiptBinding
	CoverageWorkItemIDs                []common.CoverageWorkItemID
	TargetRefs                         []string
	EvidenceKind                       string
	EvidencePolicies                   []runtimecontract.RuntimeEvidencePolicy
	WorkspaceDeliverableDirectory      string
	WorkspaceInternalDirectory         string
	Data                               OperationData
}

type ExecuteOperationResult struct {
	Receipt runtimecontract.RuntimeOperationReceipt
}

func MessageContracts() []eventcontract.MessageContract {
	return eventcontract.MessageContracts(
		eventcontract.CommandTopic[AcquireRuntimeCapabilitiesCommand, AcquireRuntimeCapabilitiesResult]("git", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicAcquireRuntimeCapabilities),
		eventcontract.CommandTopic[AcquireRuntimeOperationAdmissibilityCommand, AcquireRuntimeOperationAdmissibilityResult]("git", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicAcquireRuntimeOperationAdmissibility),
		eventcontract.CommandTopic[ProbeRuntimeOperationFeasibilityCommand, ProbeRuntimeOperationFeasibilityResult]("git", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicProbeRuntimeOperationFeasibility),
		eventcontract.CommandTopic[ProbeRuntimeSemanticProofFeasibilityCommand, ProbeRuntimeSemanticProofFeasibilityResult]("git", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicProbeRuntimeSemanticProofFeasibility),
		eventcontract.CommandTopic[QueryRuntimeCWIReconciliationCommand, QueryRuntimeCWIReconciliationResult]("git", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicQueryRuntimeCWIReconciliation),
		eventcontract.CommandTopic[AcquireCapabilityStateCommand, AcquireCapabilityStateResult]("git", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicAcquireCapabilityState),
		eventcontract.CommandTopic[ExecuteOperationCommand, ExecuteOperationResult]("git", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicExecuteOperation),
	)
}
