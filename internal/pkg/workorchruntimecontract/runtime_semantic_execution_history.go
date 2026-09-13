package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
)

const (
	RuntimeSemanticExecutionHistoryLimit    = 16
	RuntimeSemanticExecutionHistoryMaxBytes = 64 * 1024
)

// RuntimeSemanticExecutionHistory observes Run-local committed receipts in
// persistence order. It is not a plan, current owner readiness, or CWI credit.
// Earlier receipts are counted explicitly, never silently presented as absent.
type RuntimeSemanticExecutionHistory struct {
	WorkspaceID         common.WorkspaceID                  `json:"workspaceId"`
	RunID               common.RunID                        `json:"runId"`
	DefinitionRevision  string                              `json:"definitionRevision"`
	StartupInputHash    string                              `json:"startupInputHash"`
	ManifestHash        string                              `json:"manifestHash"`
	SourceStateRevision uint64                              `json:"sourceStateRevision"`
	TotalReceipts       int                                 `json:"totalReceipts"`
	OmittedReceipts     int                                 `json:"omittedReceipts"`
	Receipts            []RuntimeSemanticReceiptObservation `json:"receipts"`
	Hash                string                              `json:"hash"`
}

// RuntimeSemanticReceiptObservation.ObservedState is the immutable owner-reported
// operation outcome, not CWI acceptance or a requested lifecycle transition.
type RuntimeSemanticReceiptObservation struct {
	ReceiptSequence          int                                 `json:"receiptSequence"`
	DispatchID               string                              `json:"dispatchId"`
	OperationID              string                              `json:"operationId"`
	OperationKind            string                              `json:"operationKind"`
	OwnerModuleID            string                              `json:"ownerModuleId"`
	ReceiptRef               string                              `json:"receiptRef"`
	ReceiptHash              string                              `json:"receiptHash"`
	Fence                    uint64                              `json:"fence"`
	BoundCoverageWorkItemIDs []common.CoverageWorkItemID         `json:"boundCoverageWorkItemIds"`
	ObservedState            RuntimeOperationOutcome             `json:"observedState"`
	ExpectedEffectScope      RuntimeEffectScope                  `json:"expectedEffectScope"`
	ObservedEffectScope      RuntimeEffectScope                  `json:"observedEffectScope"`
	Changed                  bool                                `json:"changed"`
	RequestedTargetRefs      []string                            `json:"requestedTargetRefs"`
	TargetRefs               []string                            `json:"targetRefs"`
	BeforeHash               string                              `json:"beforeHash,omitempty"`
	AfterHash                string                              `json:"afterHash,omitempty"`
	Validation               *RuntimeValidationExecutionFeedback `json:"validation"`
	Diagnostic               *RuntimeSemanticExecutionDiagnostic `json:"diagnostic"`
}

func (h RuntimeSemanticExecutionHistory) Clone() RuntimeSemanticExecutionHistory {
	h.Receipts = append([]RuntimeSemanticReceiptObservation(nil), h.Receipts...)
	for i := range h.Receipts {
		h.Receipts[i].BoundCoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), h.Receipts[i].BoundCoverageWorkItemIDs...)
		h.Receipts[i].TargetRefs = append([]string(nil), h.Receipts[i].TargetRefs...)
		h.Receipts[i].RequestedTargetRefs = append([]string(nil), h.Receipts[i].RequestedTargetRefs...)
		h.Receipts[i].Diagnostic = h.Receipts[i].Diagnostic.Clone()
		if h.Receipts[i].Validation != nil {
			v := h.Receipts[i].Validation.Clone()
			h.Receipts[i].Validation = &v
		}
	}
	return h
}

func StableRuntimeSemanticExecutionHistoryHash(h RuntimeSemanticExecutionHistory) string {
	h.Hash = ""
	data, _ := json.Marshal(h)
	hash := sha256.Sum256(data)
	return "runtime-semantic-execution-history:" + hex.EncodeToString(hash[:])
}

// BuildRuntimeSemanticExecutionHistory binds an already owner-projected bounded
// suffix. The shared value layer does not read receipts or select transitions.
func BuildRuntimeSemanticExecutionHistory(c RuntimeSemanticPlanRequestContext, m RuntimeSemanticPlanManifest, total int, receipts []RuntimeSemanticReceiptObservation) (RuntimeSemanticExecutionHistory, error) {
	h := RuntimeSemanticExecutionHistory{
		WorkspaceID: c.WorkspaceID, RunID: c.RunID, DefinitionRevision: c.Definition.Revision,
		StartupInputHash: c.Definition.StartupInputHash, ManifestHash: m.Hash,
		SourceStateRevision: c.CoverageProgress.SourceStateRevision,
		TotalReceipts:       total, OmittedReceipts: total - len(receipts), Receipts: receipts,
	}.Clone()
	h.Hash = StableRuntimeSemanticExecutionHistoryHash(h)
	return h, h.Validate(c, m)
}

func (h RuntimeSemanticExecutionHistory) Validate(c RuntimeSemanticPlanRequestContext, m RuntimeSemanticPlanManifest) error {
	if h.WorkspaceID == "" || h.RunID == "" || h.DefinitionRevision == "" || h.StartupInputHash == "" || h.ManifestHash == "" ||
		h.WorkspaceID != c.WorkspaceID || h.RunID != c.RunID || h.DefinitionRevision != c.Definition.Revision ||
		h.StartupInputHash != c.Definition.StartupInputHash || h.ManifestHash != m.Hash || h.SourceStateRevision == 0 ||
		h.SourceStateRevision != c.CoverageProgress.SourceStateRevision || h.Hash != StableRuntimeSemanticExecutionHistoryHash(h) {
		return fmt.Errorf("semantic execution history scope, source revision or hash mismatch")
	}
	if h.TotalReceipts < 0 || h.OmittedReceipts < 0 || len(h.Receipts) > RuntimeSemanticExecutionHistoryLimit || h.TotalReceipts-h.OmittedReceipts != len(h.Receipts) ||
		len(h.Receipts) != min(h.TotalReceipts, RuntimeSemanticExecutionHistoryLimit) {
		return fmt.Errorf("semantic execution history requires the complete bounded receipt suffix")
	}
	seedIDs := make(map[common.CoverageWorkItemID]bool, len(c.DefinitionSeed.CoverageItems))
	for _, item := range c.DefinitionSeed.CoverageItems {
		seedIDs[item.ID] = true
	}
	seen := make(map[string]map[uint64]bool)
	for i, r := range h.Receipts {
		descriptor, found := RuntimeOperationDescriptorForKind(r.OperationKind)
		if r.ReceiptSequence != h.OmittedReceipts+i+1 || r.DispatchID == "" || r.OperationID == "" || r.ReceiptRef == "" || r.ReceiptHash == "" || r.Fence == 0 ||
			!found || descriptor.OwnerModuleID != r.OwnerModuleID || !ValidRuntimeOperationOutcome(r.ObservedState) || !validSealedRuntimeEffectScope(r.ExpectedEffectScope) || !ValidRuntimeEffectScope(r.ObservedEffectScope) {
			return fmt.Errorf("semantic execution history has invalid receipt identity or observation")
		}
		if seen[r.ReceiptRef] == nil {
			seen[r.ReceiptRef] = make(map[uint64]bool)
		}
		if seen[r.ReceiptRef][r.Fence] {
			return fmt.Errorf("semantic execution history duplicates receipt")
		}
		seen[r.ReceiptRef][r.Fence] = true
		members := make(map[common.CoverageWorkItemID]bool)
		for _, id := range r.BoundCoverageWorkItemIDs {
			if !seedIDs[id] || members[id] {
				return fmt.Errorf("semantic execution history has foreign or duplicate bound CWI")
			}
			members[id] = true
		}
		if r.Validation != nil {
			if err := r.Validation.Validate(); err != nil {
				return err
			}
		}
		if r.Diagnostic != nil {
			if err := r.Diagnostic.Validate(r.OwnerModuleID); err != nil {
				return err
			}
		}
	}
	data, err := json.Marshal(h)
	if err != nil || len(data) > RuntimeSemanticExecutionHistoryMaxBytes {
		return fmt.Errorf("semantic execution history exceeds bounded context capacity")
	}
	return nil
}
