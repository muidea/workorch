package events

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const definitionPackage = "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/events"

type Argument struct {
	Name       string
	String     *string
	Integer    *int64
	Boolean    *bool
	StringList []string
}

type OperationData struct {
	ServerID, Tool, URI, Prompt string
	Input, Arguments            []Argument
	Summary                     string
	Stream                      bool
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
	EvidenceKind, TargetRole                                  string
	EvidencePolicies                                          []runtimecontract.RuntimeEvidencePolicy
	WorkspaceDeliverableDirectory, WorkspaceInternalDirectory string
	Data                                                      OperationData
}

type ExecuteOperationResult struct {
	Receipt runtimecontract.RuntimeOperationReceipt
}

func MessageContracts() []eventcontract.MessageContract {
	return eventcontract.MessageContracts(
		eventcontract.CommandTopic[AcquireRuntimeCapabilitiesCommand, AcquireRuntimeCapabilitiesResult]("mcp", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicAcquireRuntimeCapabilities),
		eventcontract.CommandTopic[AcquireRuntimeOperationAdmissibilityCommand, AcquireRuntimeOperationAdmissibilityResult]("mcp", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicAcquireRuntimeOperationAdmissibility),
		eventcontract.CommandTopic[ProbeRuntimeOperationFeasibilityCommand, ProbeRuntimeOperationFeasibilityResult]("mcp", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicProbeRuntimeOperationFeasibility),
		eventcontract.CommandTopic[ProbeRuntimeSemanticProofFeasibilityCommand, ProbeRuntimeSemanticProofFeasibilityResult]("mcp", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicProbeRuntimeSemanticProofFeasibility),
		eventcontract.CommandTopic[QueryRuntimeCWIReconciliationCommand, QueryRuntimeCWIReconciliationResult]("mcp", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicQueryRuntimeCWIReconciliation),
		eventcontract.CommandTopic[ExecuteOperationCommand, ExecuteOperationResult]("mcp", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicExecuteOperation),
	)
}
