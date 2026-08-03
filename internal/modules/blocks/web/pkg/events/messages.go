package events

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const definitionPackage = "github.com/muidea/workorch/internal/modules/blocks/web/pkg/events"

type OperationData struct {
	ProviderID, Query, Locale, Recency string
	Domains, ExcludeDomains            []string
	MaxResults                         int64
	SafeSearch                         bool
	ApprovalID                         common.ApprovalID
	URL                                string
	MaxBytes                           int64
	ExtractMode                        string
}

type ExecuteOperationCommand struct {
	Execution                                                 runtimecontract.RuntimeOperationExecutionEnvelope
	WorkspaceID                                               common.WorkspaceID
	RunID                                                     common.RunID
	DispatchID                                                string
	Transaction                                               runtimecontract.AuthorityTransaction
	OperationID, OperationKind                                string
	CoverageWorkItemIDs                                       []common.CoverageWorkItemID
	TargetRefs                                                []string
	EvidenceKind                                              string
	EvidencePolicies                                          []runtimecontract.RuntimeEvidencePolicy
	WorkspaceDeliverableDirectory, WorkspaceInternalDirectory string
	Data                                                      OperationData
}

type ExecuteOperationResult struct {
	Receipt runtimecontract.RuntimeOperationReceipt
}

func MessageContracts() []eventcontract.MessageContract {
	return eventcontract.MessageContracts(
		eventcontract.CommandTopic[AcquireRuntimeCapabilitiesCommand, AcquireRuntimeCapabilitiesResult]("web", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicAcquireRuntimeCapabilities),
		eventcontract.CommandTopic[AcquireRuntimeOperationAdmissibilityCommand, AcquireRuntimeOperationAdmissibilityResult]("web", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicAcquireRuntimeOperationAdmissibility),
		eventcontract.CommandTopic[ProbeRuntimeOperationFeasibilityCommand, ProbeRuntimeOperationFeasibilityResult]("web", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicProbeRuntimeOperationFeasibility),
		eventcontract.CommandTopic[ProbeRuntimeSemanticProofFeasibilityCommand, ProbeRuntimeSemanticProofFeasibilityResult]("web", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicProbeRuntimeSemanticProofFeasibility),
		eventcontract.CommandTopic[ExecuteOperationCommand, ExecuteOperationResult]("web", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicExecuteOperation),
	)
}
