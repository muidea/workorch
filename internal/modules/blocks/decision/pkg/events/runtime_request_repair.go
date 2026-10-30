package events

import (
	"fmt"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const DefinitionClarificationResolutionRequestContractRepaired = "runtime_request_contract_repaired"

type RepairRuntimeDefinitionClarificationRequestContractCommand struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	Proof       runtimecontract.RuntimeSemanticRequestRepairProof
}
type RepairRuntimeDefinitionClarificationRequestContractResult struct {
	Resolution  runtimecontract.RuntimeSemanticRequestRepairResolution
	Disposition DefinitionClarificationDisposition
}

func (c RepairRuntimeDefinitionClarificationRequestContractCommand) OriginalRequest() OpenRuntimeDefinitionClarificationRequestCommand {
	d := c.Proof.Dispatch
	return OpenRuntimeDefinitionClarificationRequestCommand{SchemaVersion: DefinitionClarificationSchemaVersionCurrent, TicketID: c.Proof.Request.TicketID, WorkspaceID: d.WorkspaceID, RunID: d.RunID, DefinitionRevision: d.DefinitionRevision, StartupInputHash: d.StartupInputHash, AuthoritySnapshotID: d.AuthoritySnapshotID, ClarificationDispatchID: d.DispatchID, FocusRef: d.FocusRef, LogicalRequestID: d.LogicalRequestID, SemanticDocumentHash: d.SemanticDocumentHash, GapKind: RuntimeDefinitionGapKind(d.GapKind), OffendingItemRefs: append([]string(nil), d.OffendingItemRefs...), CanonicalGapSignatureVersion: d.CanonicalGapSignatureVersion, CanonicalGapSignature: d.CanonicalGapSignature, CanonicalGapSignatureHash: d.CanonicalGapSignatureHash, RequiredActionKind: DefinitionClarificationActionReviseAndRestart}
}
func (c RepairRuntimeDefinitionClarificationRequestContractCommand) Validate() error {
	if err := c.Proof.Validate(); err != nil {
		return err
	}
	o := c.OriginalRequest()
	if err := o.Validate(); err != nil {
		return err
	}
	if c.WorkspaceID != o.WorkspaceID || c.RunID != o.RunID || o.BindingHash() != c.Proof.Request.RequestBindingHash {
		return fmt.Errorf("request contract repair scope or original ticket binding mismatch")
	}
	return nil
}
func (r RepairRuntimeDefinitionClarificationRequestContractResult) ValidateFor(c RepairRuntimeDefinitionClarificationRequestContractCommand) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if r.Disposition != DefinitionClarificationResolved && r.Disposition != DefinitionClarificationDuplicate {
		return fmt.Errorf("request contract repair was not acknowledged")
	}
	if r.Resolution.ResolutionID != StableDefinitionClarificationResolutionID(c.Proof.Request.TicketID, c.Proof.Hash) {
		return fmt.Errorf("request contract repair resolution identity mismatch")
	}
	return r.Resolution.ValidateFor(c.Proof)
}
