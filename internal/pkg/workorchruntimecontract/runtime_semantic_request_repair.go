package workorchruntimecontract

import (
	"fmt"
	"github.com/muidea/workorch/internal/pkg/common"
	"reflect"
)

const RuntimeSemanticRequestRepairV1 = "semantic-request-contract-repair/v1"

// This narrow certificate identifies the legacy work renderer's deterministic
// forbidden-effect-field instruction. It does not interpret diagnostic prose or
// assert that the underlying Definition is complete. A new independent request
// must still establish that fact after the defective transaction is retired.
type RuntimeSemanticRequestRepairProof struct {
	ProtocolRef string
	Hash        string
	Manifest    RuntimeSemanticPlanManifest
	Expected    RuntimeExpectedDocumentContract
	Observation RuntimeDefinitionGapObservation
	Dispatch    RuntimeDefinitionClarificationDispatch
	Request     RuntimeDefinitionClarificationRequestRecord
}

func (p RuntimeSemanticRequestRepairProof) Clone() RuntimeSemanticRequestRepairProof {
	p.Manifest = p.Manifest.Clone()
	p.Expected = p.Expected.Clone()
	p.Observation = p.Observation.Clone()
	p.Dispatch = p.Dispatch.Clone()
	return p
}
func StableRuntimeSemanticRequestRepairProofHash(p RuntimeSemanticRequestRepairProof) string {
	p.Hash = ""
	return implementationWorkRawHash("semantic-request-repair:", p)
}
func (p RuntimeSemanticRequestRepairProof) Validate() error {
	if p.ProtocolRef != RuntimeSemanticRequestRepairV1 || p.Hash != StableRuntimeSemanticRequestRepairProofHash(p) {
		return fmt.Errorf("invalid semantic request repair certificate")
	}
	if err := p.Manifest.Validate(); err != nil {
		return err
	}
	if err := p.Expected.Validate(); err != nil {
		return err
	}
	if err := p.Observation.Validate(); err != nil {
		return err
	}
	if err := p.Dispatch.ValidateAgainst(p.Observation); err != nil {
		return err
	}
	if err := p.Request.ValidateAgainst(p.Dispatch); err != nil {
		return err
	}
	e, m, o := p.Expected, p.Manifest, p.Observation
	if e.DocumentKind != RuntimeProviderDocumentSemanticPlan || e.SemanticRenderContractRef != "" || e.PlanStep != RuntimeSemanticPlanStepProofBatch || e.ManifestHash != m.Hash || e.DefinitionSeedHash != m.DefinitionSeedHash || e.LogicalRequestID != o.LogicalRequestID || m.WorkspaceID != o.WorkspaceID || m.RunID != o.RunID || m.DefinitionRevision != o.DefinitionRevision || o.GapKind != RuntimeDefinitionGapSemanticAmbiguity || len(e.BatchCoverageWorkItemIDs) != 1 || len(m.ProofObligations) != 1 || len(m.CoverageItems) != 1 {
		return fmt.Errorf("request repair is limited to the exact legacy single-CWI work proof batch")
	}
	w := m.ProofObligations[0].ImplementationWork
	if w == nil || w.PurposeChoiceOnly || m.ProofObligations[0].CoverageWorkItemID != e.BatchCoverageWorkItemIDs[0] || w.Scope.Request.CoverageWorkItemID != e.BatchCoverageWorkItemIDs[0] || w.Scope.Request.DefinitionSeedHash != m.DefinitionSeedHash || w.Scope.Request.WorkspaceID != m.WorkspaceID || w.Scope.Request.RunID != m.RunID {
		return fmt.Errorf("request repair has no exact immutable implementation-work source")
	}
	if len(o.OffendingItemRefs) != 1 || (o.OffendingItemRefs[0] != string(m.CoverageItems[0].ID) && o.OffendingItemRefs[0] != m.CoverageItems[0].SourceID) {
		return fmt.Errorf("request repair gap has mixed or unknown sources")
	}
	// Full current compiler must be representable before any ticket is retired.
	return ValidateRuntimeSemanticRequestContract(m, e.PlanStep, e.BatchCoverageWorkItemIDs)
}

// ValidateRequestIntent binds the certificate to the exact active proof batch,
// including order, correction and prefix; a valid standalone hash is not enough.
func (p RuntimeSemanticRequestRepairProof) ValidateRequestIntent(l RuntimeSemanticPlanningLedger) error {
	if l.Manifest == nil || !reflect.DeepEqual(*l.Manifest, p.Manifest) || l.Phase != RuntimeSemanticPlanningProofBatchRequestOpen || l.LogicalRequestID != p.Expected.LogicalRequestID || l.ActiveBatchID != p.Expected.BatchID || l.StablePrefixHash != p.Expected.StablePrefixHash || l.Attempt != p.Expected.CorrectionAttempt {
		return common.ErrConflict
	}
	count := 0
	for _, b := range l.Batches {
		if b.BatchID == p.Expected.BatchID {
			if b.RequestLogicalID != p.Expected.LogicalRequestID || b.Sequence != p.Expected.BatchSequence || b.CoverageOrderHash != p.Expected.CoverageOrderHash || !reflect.DeepEqual(b.CoverageWorkItemIDs, p.Expected.BatchCoverageWorkItemIDs) {
				return common.ErrConflict
			}
			count++
		}
	}
	if count != 1 {
		return common.ErrConflict
	}
	return nil
}
func (p RuntimeSemanticRequestRepairProof) Matches(s RuntimeDefinitionClarificationState) bool {
	return s.Dispatch != nil && s.Request != nil && reflect.DeepEqual(p.Observation, s.Observation) && reflect.DeepEqual(p.Dispatch, *s.Dispatch) && p.Request == *s.Request
}

type RuntimeSemanticRequestRepairResolution struct {
	ProofHash    string
	TicketID     common.TicketID
	ResolutionID common.ResolutionID
	BindingHash  string
}

func (r RuntimeSemanticRequestRepairResolution) ValidateFor(p RuntimeSemanticRequestRepairProof) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if r.ProofHash != p.Hash || r.TicketID != p.Request.TicketID || r.ResolutionID == "" || r.BindingHash != StableRuntimeSemanticRequestRepairResolutionHash(p.Hash, r.TicketID, r.ResolutionID) {
		return fmt.Errorf("semantic request repair resolution binding mismatch")
	}
	return nil
}
func StableRuntimeSemanticRequestRepairResolutionHash(proofHash string, ticket common.TicketID, resolution common.ResolutionID) string {
	return implementationWorkRawHash("semantic-request-repair-resolution:", struct {
		Protocol, Proof string
		Ticket          common.TicketID
		Resolution      common.ResolutionID
	}{RuntimeSemanticRequestRepairV1, proofHash, ticket, resolution})
}

type RuntimeSemanticRequestRepairRecord struct {
	Proof      RuntimeSemanticRequestRepairProof
	Resolution RuntimeSemanticRequestRepairResolution
}

func (r RuntimeSemanticRequestRepairRecord) Clone() RuntimeSemanticRequestRepairRecord {
	r.Proof = r.Proof.Clone()
	return r
}
func (r RuntimeSemanticRequestRepairRecord) Validate() error {
	return r.Resolution.ValidateFor(r.Proof)
}

// ValidateRuntimeSemanticRequestRepairSources re-proves the cross-ledger joins
// on every DTO/aggregate reload, including after the work round has progressed.
func ValidateRuntimeSemanticRequestRepairSources(ledger RuntimeSemanticPlanningLedger, sessions []RuntimeCandidateRequestSession, work RuntimeImplementationWorkLedger, definition RuntimeDefinitionContext, revision uint64, pending *RuntimeDefinitionClarificationState) error {
	if pending != nil && pending.RequestContractRepair != nil {
		p := pending.RequestContractRepair
		if err := p.Validate(); err != nil {
			return err
		}
		if !p.Matches(*pending) {
			return common.ErrConflict
		}
		if err := p.ValidateRequestIntent(ledger); err != nil {
			return err
		}
		matched := 0
		for _, s := range sessions {
			if s.LogicalRequestID == p.Expected.LogicalRequestID {
				if s.ExpectedDocument == nil || !reflect.DeepEqual(*s.ExpectedDocument, p.Expected) || s.ManifestHash != p.Manifest.Hash || s.State != RuntimeCandidateRequestSessionDocumentComplete || s.LastOutcome.Class != RuntimeCandidateRequestOutcomeDocumentComplete || s.Transaction.AuthoritySnapshotID != p.Observation.AuthoritySnapshotID {
					return common.ErrConflict
				}
				matched++
			}
		}
		if matched != 1 {
			return common.ErrConflict
		}
		matched = 0
		for _, round := range work.Rounds {
			if round.Hash == p.Manifest.ProofObligations[0].ImplementationWork.RoundHash && round.WorkingBinding == nil && round.DispatchID == "" {
				matched++
			}
		}
		if matched != 1 {
			return common.ErrConflict
		}
	}
	seen := map[string]bool{}
	for _, g := range ledger.History {
		if g.RequestContractRepair == nil {
			continue
		}
		r := g.RequestContractRepair
		if err := r.Validate(); err != nil {
			return err
		}
		if seen[r.Proof.Expected.LogicalRequestID] || g.RetiredAtStateRevision > revision || r.Proof.Observation.DefinitionRevision != definition.Revision || r.Proof.Observation.StartupInputHash != definition.StartupInputHash {
			return fmt.Errorf("request repair history source is duplicate, stale or cross-definition")
		}
		seen[r.Proof.Expected.LogicalRequestID] = true
		matched := 0
		for _, s := range sessions {
			if s.LogicalRequestID != r.Proof.Expected.LogicalRequestID {
				continue
			}
			if s.ExpectedDocument == nil || !reflect.DeepEqual(*s.ExpectedDocument, r.Proof.Expected) || s.ManifestHash != r.Proof.Manifest.Hash || s.State != RuntimeCandidateRequestSessionTerminal || s.LastOutcome.Code != "semantic_request_contract_repaired" || s.Transaction.AuthoritySnapshotID != r.Proof.Observation.AuthoritySnapshotID {
				return fmt.Errorf("request repair lost original exact retired session")
			}
			matched++
		}
		if matched != 1 {
			return fmt.Errorf("request repair requires one original retired session")
		}
		w := r.Proof.Manifest.ProofObligations[0].ImplementationWork
		matched = 0
		for _, round := range work.Rounds {
			if round.ScopeRequest.RequestID == w.Scope.Request.RequestID && round.Ordinal == w.Ordinal && round.PolicyHash == w.PolicyHash && round.PrecedingReceiptSetHash == w.PrecedingReceiptSetHash && reflect.DeepEqual(round.ScopeObservation, &w.Scope) {
				matched++
			}
		}
		if matched != 1 {
			return fmt.Errorf("request repair lost original work reservation")
		}
	}
	return nil
}
