package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

const RuntimeSemanticExecutionBudgetMaxBytes = 32 * 1024

// RuntimeSemanticExecutionBudget observes existing Run physical reservations
// and original-CWI proof batches and durable seals. It grants neither work nor a future call;
// submission/claim must still reserve against the current owner ledger.
type RuntimeSemanticExecutionBudget struct {
	WorkspaceID          common.WorkspaceID                    `json:"workspaceId"`
	RunID                common.RunID                          `json:"runId"`
	DefinitionRevision   string                                `json:"definitionRevision"`
	StartupInputHash     string                                `json:"startupInputHash"`
	ManifestHash         string                                `json:"manifestHash"`
	SourceStateRevision  uint64                                `json:"sourceStateRevision"`
	PolicyRevision       string                                `json:"policyRevision"`
	SourceBudgetViewHash string                                `json:"sourceBudgetViewHash"`
	PhysicalDimensions   []RuntimeExecutionBudgetDimensionView `json:"physicalDimensions"`
	// nil means no additional activation-declared per-CWI cap, not unlimited
	// execution. All physical, request, history and repair guards still apply.
	ProofBatchLimit          *int                                   `json:"proofBatchLimit"`
	VisibleProofBatches      []RuntimeSemanticProofBatchBudget      `json:"visibleProofBatches"`
	DurableDispatchLimit     *int                                   `json:"durableDispatchLimit"`
	VisibleDurableDispatches []RuntimeSemanticDurableDispatchBudget `json:"visibleDurableDispatches"`
	Hash                     string                                 `json:"hash"`
}

type RuntimeSemanticProofBatchBudget struct {
	CoverageWorkItemID common.CoverageWorkItemID `json:"coverageWorkItemId"`
	Used               int                       `json:"used"`
	Remaining          *int                      `json:"remaining"`
}

// Used counts original durable seals, not successful work or receipt count.
type RuntimeSemanticDurableDispatchBudget struct {
	CoverageWorkItemID common.CoverageWorkItemID `json:"coverageWorkItemId"`
	Used               int                       `json:"used"`
	Remaining          *int                      `json:"remaining"`
}

func (b RuntimeSemanticExecutionBudget) Clone() RuntimeSemanticExecutionBudget {
	b.PhysicalDimensions = append([]RuntimeExecutionBudgetDimensionView(nil), b.PhysicalDimensions...)
	b.VisibleProofBatches = append([]RuntimeSemanticProofBatchBudget(nil), b.VisibleProofBatches...)
	b.VisibleDurableDispatches = append([]RuntimeSemanticDurableDispatchBudget(nil), b.VisibleDurableDispatches...)
	if b.DurableDispatchLimit != nil {
		value := *b.DurableDispatchLimit
		b.DurableDispatchLimit = &value
	}
	for i := range b.VisibleDurableDispatches {
		if b.VisibleDurableDispatches[i].Remaining != nil {
			value := *b.VisibleDurableDispatches[i].Remaining
			b.VisibleDurableDispatches[i].Remaining = &value
		}
	}
	if b.ProofBatchLimit != nil {
		value := *b.ProofBatchLimit
		b.ProofBatchLimit = &value
	}
	for i := range b.VisibleProofBatches {
		if b.VisibleProofBatches[i].Remaining != nil {
			value := *b.VisibleProofBatches[i].Remaining
			b.VisibleProofBatches[i].Remaining = &value
		}
	}
	return b
}

func StableRuntimeSemanticExecutionBudgetHash(b RuntimeSemanticExecutionBudget) string {
	b.Hash = ""
	data, _ := json.Marshal(b)
	h := sha256.Sum256(data)
	return "runtime-semantic-execution-budget:" + hex.EncodeToString(h[:])
}

// BuildRuntimeSemanticExecutionBudget consumes detached owner projections,
// never a mutable counter. The owner validates the complete source ledgers.
func BuildRuntimeSemanticExecutionBudget(c RuntimeSemanticPlanRequestContext, m RuntimeSemanticPlanManifest, view RuntimeExecutionBudgetView, usage, durableUsage map[common.CoverageWorkItemID]int) (RuntimeSemanticExecutionBudget, error) {
	b := RuntimeSemanticExecutionBudget{WorkspaceID: view.WorkspaceID, RunID: view.RunID,
		DefinitionRevision: c.Definition.Revision, StartupInputHash: c.Definition.StartupInputHash,
		ManifestHash: m.Hash, SourceStateRevision: view.StateRevision,
		PolicyRevision: view.PolicyRevision, SourceBudgetViewHash: view.ViewHash}
	if view.ViewHash == "" || view.ViewHash != StableRuntimeExecutionBudgetViewHash(view) {
		return b, fmt.Errorf("semantic budget source view hash mismatch")
	}
	for _, dimension := range []RuntimeBudgetDimension{RuntimeBudgetDimensionProviderCall, RuntimeBudgetDimensionCapabilityOperation} {
		for _, item := range view.Dimensions {
			if item.Dimension == dimension {
				b.PhysicalDimensions = append(b.PhysicalDimensions, item)
			}
		}
	}
	if limits := c.Activation.ExecutionContract.ExecutionLimits; limits != nil && limits.MaxProofBatchesPerCWI != 0 {
		value := limits.MaxProofBatchesPerCWI
		b.ProofBatchLimit = &value
	}
	if limits := c.Activation.ExecutionContract.ExecutionLimits; limits != nil && limits.MaxDurableDispatchesPerCWI != 0 {
		value := limits.MaxDurableDispatchesPerCWI
		b.DurableDispatchLimit = &value
	}
	seedIDs := make(map[common.CoverageWorkItemID]bool)
	for _, item := range c.DefinitionSeed.CoverageItems {
		seedIDs[item.ID] = true
	}
	for id, used := range usage {
		if !seedIDs[id] || used < 0 || (b.ProofBatchLimit != nil && used > *b.ProofBatchLimit) {
			return b, fmt.Errorf("semantic budget has foreign or invalid original-CWI usage")
		}
	}
	for id, used := range durableUsage {
		if !seedIDs[id] || used < 0 || (b.DurableDispatchLimit != nil && used > *b.DurableDispatchLimit) {
			return b, fmt.Errorf("semantic budget has foreign or invalid original-CWI durable usage")
		}
	}
	for _, id := range semanticProgressVisibleIDs(c, m) {
		item := RuntimeSemanticProofBatchBudget{CoverageWorkItemID: id, Used: usage[id]}
		if b.ProofBatchLimit != nil {
			value := *b.ProofBatchLimit - item.Used
			item.Remaining = &value
		}
		b.VisibleProofBatches = append(b.VisibleProofBatches, item)
		durable := RuntimeSemanticDurableDispatchBudget{CoverageWorkItemID: id, Used: durableUsage[id]}
		if b.DurableDispatchLimit != nil {
			value := *b.DurableDispatchLimit - durable.Used
			durable.Remaining = &value
		}
		b.VisibleDurableDispatches = append(b.VisibleDurableDispatches, durable)
	}
	b.Hash = StableRuntimeSemanticExecutionBudgetHash(b)
	return b, b.Validate(c, m)
}

func (b RuntimeSemanticExecutionBudget) Validate(c RuntimeSemanticPlanRequestContext, m RuntimeSemanticPlanManifest) error {
	if b.WorkspaceID == "" || b.RunID == "" || b.DefinitionRevision == "" || b.StartupInputHash == "" || b.ManifestHash == "" || b.PolicyRevision == "" || b.SourceBudgetViewHash == "" || b.SourceStateRevision == 0 ||
		b.WorkspaceID != c.WorkspaceID || b.RunID != c.RunID || b.DefinitionRevision != c.Definition.Revision || b.StartupInputHash != c.Definition.StartupInputHash || b.ManifestHash != m.Hash ||
		b.SourceStateRevision != c.CoverageProgress.SourceStateRevision || b.Hash != StableRuntimeSemanticExecutionBudgetHash(b) {
		return fmt.Errorf("semantic budget scope, revision or hash mismatch")
	}
	limits := c.Activation.ExecutionContract.ExecutionLimits
	if limits != nil {
		if err := limits.Validate(); err != nil {
			return err
		}
	}
	proofLimit := 0
	durableLimit := 0
	if limits != nil {
		proofLimit = limits.MaxProofBatchesPerCWI
		durableLimit = limits.MaxDurableDispatchesPerCWI
	}
	if (durableLimit == 0 && b.DurableDispatchLimit != nil) || (durableLimit != 0 && (b.DurableDispatchLimit == nil || *b.DurableDispatchLimit != durableLimit)) {
		return fmt.Errorf("semantic budget durable cap differs from activation")
	}
	if (proofLimit == 0 && b.ProofBatchLimit != nil) || (proofLimit != 0 && (b.ProofBatchLimit == nil || *b.ProofBatchLimit != proofLimit)) {
		return fmt.Errorf("semantic budget proof cap differs from activation")
	}
	dimensions := []RuntimeBudgetDimension{RuntimeBudgetDimensionProviderCall, RuntimeBudgetDimensionCapabilityOperation}
	if len(b.PhysicalDimensions) != len(dimensions) {
		return fmt.Errorf("semantic budget physical dimensions incomplete")
	}
	for i, d := range b.PhysicalDimensions {
		// Without ExecutionLimits the existing sealed policy is authoritative;
		// a read model must not replace it with today's default policy ceiling.
		limit := int64(RuntimeBudgetAmountLimit)
		if limits != nil {
			baseline, _ := RuntimeBudgetDimensionPolicyFor(DefaultRuntimeExecutionBudgetPolicy(), dimensions[i])
			if d.Unit != baseline.Unit {
				return fmt.Errorf("semantic budget physical unit differs from activation")
			}
			limit = int64(limits.MaxProviderCalls)
			if i == 1 {
				limit = int64(limits.MaxCapabilityOperations)
			}
		}
		if d.Dimension != dimensions[i] || d.Unit == "" || d.Unit != strings.TrimSpace(d.Unit) || d.Limit <= 0 || d.Limit > limit || (limits != nil && d.Limit != limit) {
			return fmt.Errorf("semantic budget physical policy mismatch")
		}
		// Subtract sequentially so even hostile integers cannot overflow a sum.
		remaining := d.Limit
		for _, used := range []int64{d.Confirmed, d.Reserved, d.Unknown} {
			if used < 0 || used > remaining {
				return fmt.Errorf("semantic budget consumption out of range")
			}
			remaining -= used
		}
		if d.Remaining != remaining || d.Exhausted != (remaining == 0) {
			return fmt.Errorf("semantic budget remaining mismatch")
		}
	}
	ids := semanticProgressVisibleIDs(c, m)
	if len(b.VisibleDurableDispatches) != len(ids) {
		return fmt.Errorf("semantic budget visible durable CWI inventory incomplete")
	}
	for i, item := range b.VisibleDurableDispatches {
		if item.CoverageWorkItemID != ids[i] || item.Used < 0 ||
			(durableLimit == 0 && item.Remaining != nil) || (durableLimit != 0 && (item.Used > durableLimit || item.Remaining == nil || *item.Remaining != durableLimit-item.Used)) {
			return fmt.Errorf("semantic budget original-CWI durable consumption mismatch")
		}
	}
	if len(b.VisibleProofBatches) != len(ids) {
		return fmt.Errorf("semantic budget visible CWI inventory incomplete")
	}
	seen := make(map[common.CoverageWorkItemID]bool)
	for i, item := range b.VisibleProofBatches {
		if item.CoverageWorkItemID != ids[i] || seen[item.CoverageWorkItemID] || item.Used < 0 ||
			(proofLimit == 0 && item.Remaining != nil) || (proofLimit != 0 && (item.Used > proofLimit || item.Remaining == nil || *item.Remaining != proofLimit-item.Used)) {
			return fmt.Errorf("semantic budget original-CWI consumption mismatch")
		}
		seen[item.CoverageWorkItemID] = true
	}
	data, err := json.Marshal(b)
	if err != nil || len(data) > RuntimeSemanticExecutionBudgetMaxBytes {
		return fmt.Errorf("semantic budget exceeds bounded context capacity")
	}
	return nil
}
