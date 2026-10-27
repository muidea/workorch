package workspace

import (
	"reflect"
	"slices"
	"time"

	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/memoryselection"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const PreparationMemorySelectionVersion = "workorch-preparation-memory-selection/v1"

func PreparationMemoryReasonRequiresEvidence(reason string) bool {
	switch reason {
	case "memory_inventory_observed_selection_pending", "memory_semantic_evidence_required", "memory_selection_candidate_limit", "memory_list_selection_observed_remaining_checks", "memory_empty_selection_observed_remaining_checks":
		return true
	}
	return false
}

// Read-only active LLM-owner facts. No credentials, mode override, success flag
// or semantic result is accepted here. Missing owner is never an empty target.
type PreparationMemoryModeObservation struct {
	ProviderID, ConfigurationRevision string
	DefaultEmbedding                  *runtimecontract.ProviderModelTarget
}

func (o PreparationMemoryModeObservation) Clone() PreparationMemoryModeObservation {
	if o.DefaultEmbedding != nil {
		v := *o.DefaultEmbedding
		o.DefaultEmbedding = &v
	}
	return o
}

func (o PreparationMemoryModeObservation) Validate() error {
	if o.ProviderID != runtimecontract.BuiltinAIProxyProviderID || !boundedPreparationIdentity(o.ConfigurationRevision) || o.DefaultEmbedding != nil && (o.DefaultEmbedding.Validate() != nil || o.DefaultEmbedding.ProviderID != o.ProviderID) {
		return common.ErrConflict
	}
	return nil
}

type PreparationMemorySelectionSource struct {
	Version                              string
	WorkspaceID                          common.WorkspaceID
	WorkspaceRevision                    uint64
	SnapshotHash, ReobservedSnapshotHash string
	Before, After                        PreparationMemoryModeObservation
	StartedAt, CompletedAt               time.Time
}

func (s PreparationMemorySelectionSource) Clone() PreparationMemorySelectionSource {
	s.Before, s.After = s.Before.Clone(), s.After.Clone()
	return s
}

func (s PreparationMemorySelectionSource) Validate() error {
	if s.Version != PreparationMemorySelectionVersion || !memorymodel.ValidPreparationWorkspaceID(s.WorkspaceID) || s.WorkspaceRevision == 0 || !boundedPreparationIdentity(s.SnapshotHash) || s.ReobservedSnapshotHash != s.SnapshotHash || s.Before.Validate() != nil || s.After.Validate() != nil || !reflect.DeepEqual(s.Before, s.After) || s.StartedAt.IsZero() || s.CompletedAt.Before(s.StartedAt) {
		return common.ErrConflict
	}
	return nil
}

// Compiled only by the original Workspace settlement, reproduced from the
// full original inventory and both active-configuration observations on reload.
// It does not create a Run recall trace or claim continuous freshness.
type PreparationMemorySelectionCompilation struct {
	Version           string
	Mode, FailureCode string
	Selection         *memoryselection.RuntimeMemorySelection
}

func (c PreparationMemorySelectionCompilation) Clone() PreparationMemorySelectionCompilation {
	if c.Selection != nil {
		s := *c.Selection
		clone := func(items []*memorymodel.MemoryItem) []*memorymodel.MemoryItem {
			out := slices.Clone(items)
			for i, item := range out {
				if item != nil {
					v := *item
					v.Tags, v.Vector = slices.Clone(v.Tags), slices.Clone(v.Vector)
					out[i] = &v
				}
			}
			return out
		}
		s.Candidates, s.Selected, s.Excluded = clone(s.Candidates), clone(s.Selected), clone(s.Excluded)
		s.TypeQuota, s.SelectionScores, s.RetrievedHits = slices.Clone(s.TypeQuota), slices.Clone(s.SelectionScores), slices.Clone(s.RetrievedHits)
		c.Selection = &s
	}
	return c
}

func CompilePreparationMemorySelection(f PreparationMemoryInventoryFacts) PreparationMemorySelectionCompilation {
	c := PreparationMemorySelectionCompilation{Version: PreparationMemorySelectionVersion}
	if f.SelectionSource == nil || f.Validate() != nil {
		c.FailureCode = "source_unavailable"
		return c
	}
	c.Mode = "list_only"
	if f.SelectionSource.Before.DefaultEmbedding != nil {
		c.Mode = "semantic"
	}
	// Preserve activation's actual empty-owner fast path, but not an empty
	// selected subset after exclusions. Nonempty semantic input cannot fall back.
	if c.Mode == "semantic" && len(f.Observation.Items) != 0 {
		c.FailureCode = "semantic_evidence_required"
		return c
	}
	s, err := memoryselection.SelectRuntimeMemoryContext(f.Observation.Items)
	if err != nil {
		c.FailureCode = "candidate_limit"
		return c
	}
	c.Selection = &s
	return c
}

func (c PreparationMemorySelectionCompilation) CheckResult() (WorkspacePreparationCheckState, string) {
	switch c.FailureCode {
	case "semantic_evidence_required":
		return "blocked", "memory_semantic_evidence_required"
	case "candidate_limit":
		return "blocked", "memory_selection_candidate_limit"
	case "":
		if c.Selection != nil {
			if len(c.Selection.Candidates) == 0 {
				return "passed", "memory_empty_selection_observed_remaining_checks"
			}
			return "passed", "memory_list_selection_observed_remaining_checks"
		}
	}
	return "blocked", "memory_inventory_observed_selection_pending"
}

func EvaluatePreparationMemoryCheck(a WorkspacePreparationAttempt, e *WorkspacePreparationMemoryInventoryEvidence) []WorkspacePreparationCheckResult {
	checks := slices.Clone(a.Checks)
	if a.EvaluationVersion != PreparationCheckEvaluationVersion || a.State != WorkspacePreparationBlocked {
		return checks
	}
	for i, check := range checks {
		if check.ID != "memory_selection" || check.Reason == "binding_changed" || check.Reason == "original_check_interrupted" {
			continue
		}
		if e != nil && e.Validate() == nil && e.Selection != nil && e.Ref == a.MemoryInventoryEvidenceRef && e.RequestID == a.RequestID && e.PayloadHash == a.PayloadHash && reflect.DeepEqual(e.Binding, a.Binding) && e.Facts.WorkspaceRevision == a.ExpectedWorkspaceRevision+1 && e.Facts.SelectionSource.SnapshotHash == a.ObservationSnapshotHash {
			checks[i].State, checks[i].Reason = e.Selection.CheckResult()
		} else if check.State != "blocked" || check.Reason != "memory_inventory_observed_selection_pending" && check.Reason != "memory_inventory_unavailable_not_saved" && check.Reason != "check_pipeline_incomplete" {
			checks[i].State, checks[i].Reason = "blocked", "check_pipeline_incomplete"
		}
	}
	return checks
}
