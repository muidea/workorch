package events

import (
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const definitionPackage = "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"

type OperationData struct {
	// ValidationRecipeID is present only for validation/managed_session. It is
	// selection identity; Shell resolves command/args from the sealed recipe.
	ValidationRecipeID              string
	Command                         string
	Args                            []string
	TimeoutMillis                   int64
	StateChanging                   bool
	TargetRef                       string
	TargetRefs                      []string
	VerificationScope               string
	BoundedReason                   string
	ValidationScratchDirectory      string
	ValidationScratchAutoCleanup    bool
	ValidationScratchAutoCleanupSet bool
	ValidationEvidence              runtimecontract.RuntimeValidationEvidence
	ManagedValidation               runtimecontract.RuntimeManagedValidation
}

type ExecuteOperationCommand struct {
	runtimecontract.RuntimeOperationExecutionContext
	// SealedOperation is the only Shell executable payload. Command/args remain
	// part of Operation; owner-confirmed recipes, diagnostic scope, target role
	// and evidence policies are hash-bound in its authorization.
	SealedOperation SealedRuntimeOperation
}

type ExecuteOperationResult struct {
	Receipt runtimecontract.RuntimeOperationReceipt
}

func MessageContracts() []eventcontract.MessageContract {
	return eventcontract.MessageContracts(
		eventcontract.CommandTopic[AcquireRuntimeCapabilitiesCommand, AcquireRuntimeCapabilitiesResult]("shell", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicAcquireRuntimeCapabilities),
		eventcontract.CommandTopic[AcquireRuntimeOperationAdmissibilityCommand, AcquireRuntimeOperationAdmissibilityResult]("shell", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicAcquireRuntimeOperationAdmissibility),
		eventcontract.CommandTopic[ProbeRuntimeOperationFeasibilityCommand, ProbeRuntimeOperationFeasibilityResult]("shell", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicProbeRuntimeOperationFeasibility),
		eventcontract.CommandTopic[ProbeRuntimeSemanticProofFeasibilityCommand, ProbeRuntimeSemanticProofFeasibilityResult]("shell", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicProbeRuntimeSemanticProofFeasibility),
		eventcontract.CommandTopic[ProbeRuntimeOwnerFactProspectiveSatisfactionCommand, ProbeRuntimeOwnerFactProspectiveSatisfactionResult]("shell", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicProbeRuntimeOwnerFactProspectiveSatisfaction),
		eventcontract.CommandTopic[QueryRuntimeCWIReconciliationCommand, QueryRuntimeCWIReconciliationResult]("shell", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicQueryRuntimeCWIReconciliation),
		eventcontract.CommandTopic[QueryRuntimeValidationRawOutputCommand, QueryRuntimeValidationRawOutputResult]("shell", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicQueryRuntimeValidationRawOutput),
		eventcontract.CommandTopic[ExecuteOperationCommand, ExecuteOperationResult]("shell", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicExecuteOperation),
	)
}
