package workspace

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type Workspace struct {
	// Revision is the owner-local persistence revision. Projection reducers use
	// it as their CAS fence instead of relying on in-process EventHub ordering.
	Revision             uint64
	ID                   common.WorkspaceID
	Title                string
	Description          string
	DeliverableDirectory string
	InternalDirectory    string
	State                common.WorkspaceState
	ActiveRunID          common.RunID
	TemplateID           string
	ProfileID            string
	DefaultLLMSelection  *runtimecontract.ProviderInferenceSelection
	DefaultSoulID        common.SoulID
	SoulBindings         map[string]common.SoulID
	ContextHints         workorcheventpayload.Payload
	EvidencePolicies     []workorcheventpayload.Payload
	// DefinitionAdmissionCandidates is immutable owner history. The current
	// candidate is the highest revision in one CandidateID lineage.
	DefinitionAdmissionCandidates []DefinitionAdmissionCandidate
	StartupInput                  *WorkspaceStartupInputContract
	ActivationAdmission           *WorkspaceActivationAdmission
	// TerminalDelivery is the Workspace owner's persisted release fence. It is
	// written only by the lifecycle reducer and never by generic metadata update.
	TerminalDelivery *runtimecontract.RuntimeTerminalDeliveryIdentity
}
