package workspace

import (
	"reflect"
	"strings"
	"testing"
	"time"

	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
)

func preparationHistoryFixture() *Workspace {
	r := preparationRecordFixture()
	r.Version = WorkspacePreparationWorkflowVersion
	a := &r.Attempts[0]
	a.CreationEpoch = strings.Repeat("a", 32)
	a.State, a.CompletedAt, a.CommittedWorkspaceRevision = WorkspacePreparationBlocked, a.StartedAt.Add(time.Second), 3
	a.Checks = []WorkspacePreparationCheckResult{{ID: "first_activation_no_run", Owner: "run", Required: true, State: "blocked", Reason: "run_history_empty_fence_not_final"}}
	e := WorkspacePreparationRunHistoryEvidence{Version: PreparationRunHistoryEvidenceVersion, Ref: PreparationRunHistoryRefPrefix + strings.Repeat("b", 32), RequestID: a.RequestID, PayloadHash: a.PayloadHash, Binding: a.Binding, CreationEpoch: a.CreationEpoch, PreparationEpoch: a.PreparationEpoch, Facts: PreparationRunHistoryFacts{Version: PreparationRunHistoryEvidenceVersion, WorkspaceRevision: a.ExpectedWorkspaceRevision + 1, Observation: runevents.ObserveWorkspacePreparationHistoryResult{RequestID: a.RequestID, ObservedAt: a.StartedAt.Add(time.Millisecond), History: runmodel.WorkspacePreparationHistory{WorkspaceID: "ws"}}}}
	e.ContentHash = e.Hash()
	a.RunHistoryEvidenceRef = e.Ref
	return &Workspace{ID: "ws", Revision: 3, StartupPreparation: &r, PreparationRunHistoryEvidence: []WorkspacePreparationRunHistoryEvidence{e}}
}

func TestPreparationRunHistoryEvidenceKeepsAllOwnerFactsAndNeverPasses(t *testing.T) {
	for _, scenario := range []string{"empty", "generation_without_visible_rows", "pending_and_replacement", "retired"} {
		w := preparationHistoryFixture()
		e := &w.PreparationRunHistoryEvidence[0]
		h := &e.Facts.Observation.History
		reason := "run_history_present"
		switch scenario {
		case "empty":
			reason = "run_history_empty_fence_not_final"
		case "generation_without_visible_rows":
			h.GenerationHead = 9
		case "pending_and_replacement":
			h.GenerationHead, h.RunCount, h.PendingActivationCount, h.ReplacementCount = 4, 3, 2, 1
		case "retired":
			h.GenerationHead = 4
			h.Retirement = &runmodel.WorkspaceRetirementFence{WorkspaceID: w.ID, DeletionID: "original-deletion", DeletionGeneration: 1, WorkspaceIntentHash: "original-intent", DataDispositionIntent: "retain", Phase: "retired", FenceRevision: 2, WorkspaceRunGeneration: 4, QuiescenceID: "original-quiescence", RuntimeInventoryHash: "original-inventory"}
			reason = "run_history_retired"
		}
		e.ContentHash = e.Hash()
		w.StartupPreparation.Attempts[0].Checks[0].Reason = reason
		if e.Facts.BlockedReason() != reason || validatePreparationRunHistoryEvidencePersistence(w) != nil {
			t.Fatal("complete ordered history rejected", scenario)
		}
		copy := e.Clone()
		if h.Retirement != nil {
			copy.Facts.Observation.History.Retirement.QuiescenceID = "caller"
			if h.Retirement.QuiescenceID != "original-quiescence" {
				t.Fatal("retirement clone aliases owner history")
			}
		}
		w.StartupPreparation.Attempts[0].Checks[0].State = "passed"
		if validatePreparationRunHistoryEvidencePersistence(w) == nil {
			t.Fatal("point-in-time history conferred no-Run credit")
		}
	}
}

func TestPreparationRunHistoryRejectsUnencodableAndOversizedMaterialWithoutTruncation(t *testing.T) {
	e := preparationHistoryFixture().PreparationRunHistoryEvidence[0]
	e.Facts.Observation.ObservedAt = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
	e.ContentHash = e.Hash()
	if e.ContentHash != "" || e.Validate() == nil {
		t.Fatal("unencodable timestamp accepted using an empty content hash")
	}
	for _, field := range []string{"deletion", "intent", "quiescence", "inventory"} {
		f := preparationHistoryFixture().PreparationRunHistoryEvidence[0].Facts
		r := &runmodel.WorkspaceRetirementFence{WorkspaceID: "ws", DeletionID: "deletion", DeletionGeneration: 1, WorkspaceIntentHash: "intent", DataDispositionIntent: "retain", Phase: "retired", FenceRevision: 1}
		f.Observation.History.Retirement = r
		value := &r.DeletionID
		switch field {
		case "intent":
			value = &r.WorkspaceIntentHash
		case "quiescence":
			value = &r.QuiescenceID
		case "inventory":
			value = &r.RuntimeInventoryHash
		}
		*value = strings.Repeat("x", 513)
		if f.Validate() == nil || len(*value) != 513 || f.BlockedReason() != "run_history_unavailable" {
			t.Fatal("oversized complete material accepted or silently truncated", field)
		}
	}
}

func TestPreparationRunHistoryEvidenceRejectsRehashedForeignAndMissingMaterial(t *testing.T) {
	for _, change := range []string{"version", "scope", "request", "payload", "revision", "before", "after", "binding", "creation_epoch", "preparation_epoch", "count", "reason", "owner", "optional", "missing", "orphan", "duplicate", "ref", "legacy", "hash"} {
		t.Run(change, func(t *testing.T) {
			w := preparationHistoryFixture()
			a, e := &w.StartupPreparation.Attempts[0], &w.PreparationRunHistoryEvidence[0]
			switch change {
			case "version":
				e.Version = "future"
			case "scope":
				e.Facts.Observation.History.WorkspaceID = "other"
			case "request":
				e.RequestID, e.Facts.Observation.RequestID = "other", "other"
			case "payload":
				e.PayloadHash = "other"
			case "revision":
				e.Facts.WorkspaceRevision++
			case "before":
				e.Facts.Observation.ObservedAt = a.StartedAt.Add(-time.Nanosecond)
			case "after":
				e.Facts.Observation.ObservedAt = a.CompletedAt.Add(time.Nanosecond)
			case "binding":
				e.Binding.StartupInputHash = "other"
			case "creation_epoch":
				e.CreationEpoch = strings.Repeat("c", 32)
			case "preparation_epoch":
				e.PreparationEpoch++
			case "count":
				e.Facts.Observation.History.PendingActivationCount = 1
			case "reason":
				a.Checks[0].Reason = "run_history_present"
			case "owner":
				a.Checks[0].Owner = "caller"
			case "optional":
				a.Checks[0].Required = false
			case "missing":
				w.PreparationRunHistoryEvidence = nil
			case "orphan":
				a.RunHistoryEvidenceRef = ""
			case "duplicate":
				w.PreparationRunHistoryEvidence = append(w.PreparationRunHistoryEvidence, e.Clone())
			case "ref":
				a.RunHistoryEvidenceRef = PreparationRunHistoryRefPrefix + strings.Repeat("c", 32)
			case "legacy":
				w.StartupPreparation.Version = WorkspacePreparationRecordVersion
			case "hash":
				e.ContentHash = "other"
			}
			if change != "hash" {
				e.ContentHash = e.Hash()
			}
			if validatePreparationRunHistoryEvidencePersistence(w) == nil {
				t.Fatal("foreign or missing history accepted after local rehash")
			}
		})
	}
	w := preparationHistoryFixture()
	w.StartupPreparation.Attempts[0].Checks[0].Reason = "binding_changed"
	before := w.PreparationRunHistoryEvidence[0].Clone()
	if validatePreparationRunHistoryEvidencePersistence(w) != nil || !reflect.DeepEqual(w.PreparationRunHistoryEvidence[0], before) {
		t.Fatal("stale history lost or rewritten")
	}
}
