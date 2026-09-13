package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
)

const RuntimeSemanticCoverageProgressMaxBytes = 64 * 1024

// RuntimeSemanticCoverageProgress is a detached SM-2 observation, never a
// second coverage ledger or permission to select/execute work. Counts describe
// current CWI acceptance; evidence and reopen counts describe recorded history,
// not fresh proof, completed operations, or additional acceptance credit.
type RuntimeSemanticCoverageProgress struct {
	WorkspaceID         common.WorkspaceID                    `json:"workspaceId"`
	RunID               common.RunID                          `json:"runId"`
	DefinitionRevision  string                                `json:"definitionRevision"`
	StartupInputHash    string                                `json:"startupInputHash"`
	DefinitionSeedHash  string                                `json:"definitionSeedHash"`
	ManifestHash        string                                `json:"manifestHash"`
	PlanStep            RuntimeSemanticPlanStep               `json:"planStep"`
	BatchID             string                                `json:"batchId,omitempty"`
	BatchSequence       int                                   `json:"batchSequence"`
	SourceStateRevision uint64                                `json:"sourceStateRevision"`
	EvidenceRevision    string                                `json:"evidenceRevision"`
	Total               int                                   `json:"total"`
	Open                int                                   `json:"open"`
	Satisfied           int                                   `json:"satisfied"`
	Unsatisfied         int                                   `json:"unsatisfied"`
	VisibleItems        []RuntimeSemanticCoverageProgressItem `json:"visibleItems"`
	Hash                string                                `json:"hash"`
}

type RuntimeSemanticCoverageProgressItem struct {
	CoverageWorkItemID    common.CoverageWorkItemID `json:"coverageWorkItemId"`
	ObservedState         RuntimeCoverageItemState  `json:"observedState"`
	RecordedEvidenceCount int                       `json:"recordedEvidenceCount"`
	ReopenCount           int                       `json:"reopenCount"`
}

func (p RuntimeSemanticCoverageProgress) Clone() RuntimeSemanticCoverageProgress {
	p.VisibleItems = append([]RuntimeSemanticCoverageProgressItem(nil), p.VisibleItems...)
	return p
}

func StableRuntimeSemanticCoverageProgressHash(p RuntimeSemanticCoverageProgress) string {
	p.Hash = ""
	data, _ := json.Marshal(p)
	hash := sha256.Sum256(data)
	return "runtime-semantic-coverage-progress:" + hex.EncodeToString(hash[:])
}

func semanticProgressVisibleIDs(c RuntimeSemanticPlanRequestContext, m RuntimeSemanticPlanManifest) []common.CoverageWorkItemID {
	if c.PlanStep == RuntimeSemanticPlanStepProofBatch {
		return c.BatchCoverageWorkItemIDs
	}
	ids := make([]common.CoverageWorkItemID, 0, len(m.CoverageItems))
	for _, item := range m.Normalized().CoverageItems {
		ids = append(ids, item.ID)
	}
	return ids
}

// BuildRuntimeSemanticCoverageProgress uses the complete sealed inventory for
// totals, but only the request-visible CWI set for details. Missing, duplicate,
// foreign or unknown-state owner facts cannot be silently normalized away.
func BuildRuntimeSemanticCoverageProgress(c RuntimeSemanticPlanRequestContext, m RuntimeSemanticPlanManifest, facts RuntimeFacts, stateRevision uint64) (RuntimeSemanticCoverageProgress, error) {
	p := RuntimeSemanticCoverageProgress{
		WorkspaceID: facts.WorkspaceID, RunID: facts.RunID, DefinitionRevision: facts.DefinitionRevision,
		StartupInputHash:   c.Definition.StartupInputHash,
		DefinitionSeedHash: facts.DefinitionSeed.Hash, ManifestHash: m.Hash, PlanStep: c.PlanStep, BatchID: c.BatchID, BatchSequence: c.BatchSequence,
		SourceStateRevision: stateRevision, EvidenceRevision: facts.EvidenceRevision,
		Total: len(c.DefinitionSeed.CoverageItems),
	}
	byID := make(map[common.CoverageWorkItemID]RuntimeCoverageItemFact, len(facts.Coverage))
	for _, item := range facts.Coverage {
		if _, duplicate := byID[item.ID]; duplicate {
			return p, fmt.Errorf("semantic progress has duplicate coverage fact")
		}
		byID[item.ID] = item
	}
	if len(byID) != p.Total {
		return p, fmt.Errorf("semantic progress requires complete sealed coverage inventory")
	}
	seedIDs := make(map[common.CoverageWorkItemID]bool, p.Total)
	seedItems := NormalizeRuntimeCoverageItems(c.DefinitionSeed.CoverageItems)
	if len(seedItems) != p.Total {
		return p, fmt.Errorf("semantic progress has invalid sealed coverage inventory")
	}
	for _, seed := range seedItems {
		if seedIDs[seed.ID] {
			return p, fmt.Errorf("semantic progress has duplicate sealed coverage item")
		}
		seedIDs[seed.ID] = true
		item, found := byID[seed.ID]
		if !found || item.SliceID != seed.SliceID || item.SourceScope != seed.SourceScope || item.SourceID != seed.SourceID || item.TargetText != seed.TargetText {
			return p, fmt.Errorf("semantic progress coverage fact does not match sealed definition")
		}
		switch item.State {
		case RuntimeCoverageItemStateOpen:
			p.Open++
		case RuntimeCoverageItemStateSatisfied:
			p.Satisfied++
		case RuntimeCoverageItemStateUnsatisfied:
			p.Unsatisfied++
		default:
			return p, fmt.Errorf("semantic progress has unknown coverage state")
		}
	}
	for _, id := range semanticProgressVisibleIDs(c, m) {
		item, found := byID[id]
		if !found {
			return p, fmt.Errorf("semantic progress visible item has no owner fact")
		}
		p.VisibleItems = append(p.VisibleItems, RuntimeSemanticCoverageProgressItem{
			CoverageWorkItemID: id, ObservedState: item.State, RecordedEvidenceCount: len(item.EvidenceRefs), ReopenCount: len(item.ReopenLineage),
		})
	}
	p.Hash = StableRuntimeSemanticCoverageProgressHash(p)
	return p, p.Validate(c, m)
}

func (p RuntimeSemanticCoverageProgress) Validate(c RuntimeSemanticPlanRequestContext, m RuntimeSemanticPlanManifest) error {
	data, err := json.Marshal(p)
	if err != nil || len(data) > RuntimeSemanticCoverageProgressMaxBytes {
		return fmt.Errorf("semantic progress exceeds bounded context capacity")
	}
	if p.WorkspaceID == "" || p.RunID == "" || p.DefinitionRevision == "" || p.DefinitionSeedHash == "" || p.ManifestHash == "" || p.StartupInputHash == "" {
		return fmt.Errorf("semantic progress requires sealed scope")
	}
	if p.WorkspaceID != c.WorkspaceID || p.RunID != c.RunID || p.DefinitionRevision != c.Definition.Revision ||
		p.StartupInputHash != c.Definition.StartupInputHash || p.DefinitionSeedHash != c.DefinitionSeed.Hash || p.ManifestHash != m.Hash ||
		p.PlanStep != c.PlanStep || p.BatchID != c.BatchID || p.BatchSequence != c.BatchSequence ||
		p.SourceStateRevision == 0 || p.EvidenceRevision == "" || p.Hash == "" || p.Hash != StableRuntimeSemanticCoverageProgressHash(p) {
		return fmt.Errorf("semantic progress scope, source revision or hash mismatch")
	}
	if p.Total <= 0 || p.Total != len(c.DefinitionSeed.CoverageItems) || p.Open < 0 || p.Satisfied < 0 || p.Unsatisfied < 0 ||
		p.Open > p.Total || p.Satisfied > p.Total || p.Unsatisfied > p.Total || p.Open+p.Satisfied+p.Unsatisfied != p.Total {
		return fmt.Errorf("semantic progress totals do not cover sealed definition")
	}
	ids := semanticProgressVisibleIDs(c, m)
	if len(p.VisibleItems) != len(ids) {
		return fmt.Errorf("semantic progress does not cover visible request scope")
	}
	seen := make(map[common.CoverageWorkItemID]bool, len(ids))
	manifestIDs := make(map[common.CoverageWorkItemID]bool, len(m.CoverageItems))
	seedIDs := make(map[common.CoverageWorkItemID]bool, p.Total)
	for _, item := range m.CoverageItems {
		manifestIDs[item.ID] = true
	}
	for _, item := range c.DefinitionSeed.CoverageItems {
		seedIDs[item.ID] = true
	}
	open, satisfied, unsatisfied := 0, 0, 0
	for i, item := range p.VisibleItems {
		if item.CoverageWorkItemID != ids[i] || !manifestIDs[item.CoverageWorkItemID] || !seedIDs[item.CoverageWorkItemID] || seen[item.CoverageWorkItemID] || item.RecordedEvidenceCount < 0 || item.ReopenCount < 0 {
			return fmt.Errorf("semantic progress visible membership or history count mismatch")
		}
		seen[item.CoverageWorkItemID] = true
		switch item.ObservedState {
		case RuntimeCoverageItemStateOpen:
			open++
		case RuntimeCoverageItemStateSatisfied:
			satisfied++
		case RuntimeCoverageItemStateUnsatisfied:
			unsatisfied++
		default:
			return fmt.Errorf("semantic progress has unknown visible coverage state")
		}
	}
	if open > p.Open || satisfied > p.Satisfied || unsatisfied > p.Unsatisfied {
		return fmt.Errorf("semantic progress visible counts exceed totals")
	}
	return nil
}
