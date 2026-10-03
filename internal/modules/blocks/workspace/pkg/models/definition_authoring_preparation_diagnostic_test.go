package workspace

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func preparationModelFixture() (DefinitionAuthoringRecord, DefinitionAuthoringPreparationDiagnostic) {
	input := DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws-preparation", Revision: 1, TemplateID: "t", GoalText: "目标", ResourceRefs: []string{"input.txt"}}
	template := WorkspaceTemplate{ID: "t"}
	r := DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: WorkspaceTemplateSnapshotHash(&template)}
	d := NewDefinitionAuthoringPreparationDiagnostic(DefinitionAuthoringPreparationDiagnostic{WorkspaceID: input.WorkspaceID, InputHash: r.InputHash, TemplateSnapshotHash: r.TemplateSnapshotHash, ObservedWorkspaceRevision: 2, Stage: DefinitionAuthoringPreparationDerivation, TargetRequestID: "derive", PhaseCode: "resources", ResourceRef: "input.txt"}, []byte{' ', 0, 0xff, '\r', '\n'})
	return r, d
}

func TestPreparationDiagnosticBytesHashNonUTF8AndBoundary(t *testing.T) {
	_, d := preparationModelFixture()
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(d.Cause)
	if d.CauseSHA256 != hex.EncodeToString(h[:]) || d.CauseBytes != uint64(len(d.Cause)) || !d.CauseSaved {
		t.Fatal("cause identity changed")
	}
	cloned := d.Clone()
	cloned.Cause[0] = '!'
	if bytes.Equal(cloned.Cause, d.Cause) {
		t.Fatal("clone aliased bytes")
	}
	wire, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	var reopened DefinitionAuthoringPreparationDiagnostic
	if err := json.Unmarshal(wire, &reopened); err != nil || !reflect.DeepEqual(reopened, d) || reopened.Validate() != nil {
		t.Fatalf("non-UTF8 persistence lost: %v", err)
	}
	empty := NewDefinitionAuthoringPreparationDiagnostic(d, []byte(errors.New("").Error()))
	if empty.Validate() != nil || empty.Cause == nil || empty.CauseBytes != 0 || !empty.CauseSaved {
		t.Fatal("empty original cause not preserved")
	}
	wireEmpty, err := json.Marshal(empty)
	if err != nil {
		t.Fatal(err)
	}
	var emptyReopened DefinitionAuthoringPreparationDiagnostic
	if err := json.Unmarshal(wireEmpty, &emptyReopened); err != nil || emptyReopened.Cause == nil || emptyReopened.Validate() != nil {
		t.Fatal("empty bytes became missing cause")
	}
	for _, size := range []int{DefinitionAuthoringPreparationCauseLimit, DefinitionAuthoringPreparationCauseLimit + 1} {
		raw := bytes.Repeat([]byte{0xff}, size)
		got := NewDefinitionAuthoringPreparationDiagnostic(d, raw)
		if err := got.Validate(); err != nil {
			t.Fatal(err)
		}
		expected := sha256.Sum256(raw)
		if got.CauseSHA256 != hex.EncodeToString(expected[:]) || got.CauseBytes != uint64(size) {
			t.Fatal("oversize lost full identity")
		}
		if size > DefinitionAuthoringPreparationCauseLimit {
			if got.CauseSaved || got.Cause != nil || got.NotSavedReason == "" {
				t.Fatal("oversize truncated or marked complete")
			}
		} else if !bytes.Equal(got.Cause, raw) || !got.CauseSaved || got.NotSavedReason != "" {
			t.Fatal("boundary cause not saved exactly")
		}
	}
	mutations := map[string]func(*DefinitionAuthoringPreparationDiagnostic){
		"bytes":         func(d *DefinitionAuthoringPreparationDiagnostic) { d.CauseBytes++ },
		"hash":          func(d *DefinitionAuthoringPreparationDiagnostic) { d.CauseSHA256 = fmt.Sprintf("%064d", 0) },
		"raw":           func(d *DefinitionAuthoringPreparationDiagnostic) { d.Cause[0]++ },
		"reason":        func(d *DefinitionAuthoringPreparationDiagnostic) { d.NotSavedReason = "redacted" },
		"missing-cause": func(d *DefinitionAuthoringPreparationDiagnostic) { d.Cause = nil },
		"phase": func(d *DefinitionAuthoringPreparationDiagnostic) {
			d.PhaseCode = "unknown"
			d.DiagnosticID = d.ExpectedDiagnosticID()
		},
		"stage": func(d *DefinitionAuthoringPreparationDiagnostic) {
			d.Stage = "unknown"
			d.DiagnosticID = d.ExpectedDiagnosticID()
		},
		"review": func(d *DefinitionAuthoringPreparationDiagnostic) {
			d.ReviewKind = "forged"
			d.DiagnosticID = d.ExpectedDiagnosticID()
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			bad := d.Clone()
			mutate(&bad)
			if bad.Validate() == nil {
				t.Fatal("forged diagnostic accepted")
			}
		})
	}
}

func TestPreparationDiagnosticIDBindsEverySemanticFieldNotObservedRevision(t *testing.T) {
	_, d := preparationModelFixture()
	for name, mutate := range map[string]func(*DefinitionAuthoringPreparationDiagnostic){
		"workspace":  func(d *DefinitionAuthoringPreparationDiagnostic) { d.WorkspaceID += "-other" },
		"input":      func(d *DefinitionAuthoringPreparationDiagnostic) { d.InputHash += "-other" },
		"template":   func(d *DefinitionAuthoringPreparationDiagnostic) { d.TemplateSnapshotHash += "-other" },
		"stage":      func(d *DefinitionAuthoringPreparationDiagnostic) { d.Stage = DefinitionAuthoringPreparationProposal },
		"target":     func(d *DefinitionAuthoringPreparationDiagnostic) { d.TargetRequestID += "-other" },
		"derivation": func(d *DefinitionAuthoringPreparationDiagnostic) { d.DerivationPermitID = "permit" },
		"review":     func(d *DefinitionAuthoringPreparationDiagnostic) { d.ReviewKind = GoalReviewTraceability },
		"proposal":   func(d *DefinitionAuthoringPreparationDiagnostic) { d.ProposalHash = "proposal" },
		"phase":      func(d *DefinitionAuthoringPreparationDiagnostic) { d.PhaseCode = "render" },
		"resource":   func(d *DefinitionAuthoringPreparationDiagnostic) { d.ResourceRef += "-other" },
		"provider":   func(d *DefinitionAuthoringPreparationDiagnostic) { d.ProviderBindingHash = "provider" },
		"rawhash":    func(d *DefinitionAuthoringPreparationDiagnostic) { d.CauseSHA256 += "-other" },
		"rawbytes":   func(d *DefinitionAuthoringPreparationDiagnostic) { d.CauseBytes++ },
		"saved":      func(d *DefinitionAuthoringPreparationDiagnostic) { d.CauseSaved = !d.CauseSaved },
		"reason":     func(d *DefinitionAuthoringPreparationDiagnostic) { d.NotSavedReason = "reason" },
	} {
		t.Run(name, func(t *testing.T) {
			other := d.Clone()
			mutate(&other)
			if other.ExpectedDiagnosticID() == d.DiagnosticID {
				t.Fatal("semantic field omitted from ID")
			}
		})
	}
	d.ObservedWorkspaceRevision += 10
	if d.ExpectedDiagnosticID() != d.DiagnosticID || d.Validate() != nil {
		t.Fatal("observation revision changed semantic ID")
	}
}

func TestPreparationDiagnosticLedgerLimitCloneDuplicateAndStale(t *testing.T) {
	r, d := preparationModelFixture()
	for n := 0; n < DefinitionAuthoringPreparationDiagnosticLimit; n++ {
		entry := d.Clone()
		entry.TargetRequestID = fmt.Sprintf("target-%d", n)
		entry.DiagnosticID = entry.ExpectedDiagnosticID()
		next, saved, duplicate, err := r.RecordPreparationDiagnostic(entry)
		if err != nil || duplicate || saved.DiagnosticID != entry.DiagnosticID {
			t.Fatalf("append %d: %v", n, err)
		}
		r = next
	}
	overflow := d.Clone()
	overflow.TargetRequestID = "overflow"
	overflow.DiagnosticID = overflow.ExpectedDiagnosticID()
	_, _, _, err := r.RecordPreparationDiagnostic(overflow)
	var limit *PreparationDiagnosticLimitError
	if !errors.As(err, &limit) || limit.Limit != 32 || !errors.Is(err, common.ErrConflict) {
		t.Fatalf("limit refusal: %v", err)
	}
	clone := r.Clone()
	clone.PreparationDiagnostics[0].Cause[0]++
	if r.PreparationDiagnostics[0].Validate() != nil || clone.Validate() == nil {
		t.Fatal("record clone or full validation failed")
	}
	replay := r.PreparationDiagnostics[0].Clone()
	replay.ObservedWorkspaceRevision += 100
	next, saved, duplicate, err := r.RecordPreparationDiagnostic(replay)
	if err != nil || !duplicate || !reflect.DeepEqual(next, r) || saved.ObservedWorkspaceRevision != d.ObservedWorkspaceRevision {
		t.Fatalf("full ledger duplicate not first-write: %v", err)
	}
	stale := overflow.Clone()
	stale.InputHash = "old"
	stale.DiagnosticID = stale.ExpectedDiagnosticID()
	_, _, _, err = r.RecordPreparationDiagnostic(stale)
	if errors.As(err, &limit) || !errors.Is(err, common.ErrConflict) {
		t.Fatalf("stale became capacity refusal: %v", err)
	}
}

func TestPreparationDiagnosticTargetReservationAndBudgetRefusal(t *testing.T) {
	r, d := preparationModelFixture()
	r.Input.ResourceRefs = nil
	r.InputHash = r.Input.InputHash()
	d.InputHash, d.ResourceRef = r.InputHash, ""
	d.PhaseCode = "render"
	d.DiagnosticID = d.ExpectedDiagnosticID()
	var reserved DefinitionDerivationRequest
	var err error
	r, reserved, _, err = r.ReserveDerivation(d.TargetRequestID, "provider", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := r.RecordPreparationDiagnostic(d); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("reserved target accepted: %v", err)
	}
	r, _, _, err = r.ReserveDerivation("derive-2", "provider", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, denied, fresh, err := r.ReserveDerivation("derive-3", "provider", nil)
	var refusal *DefinitionAuthoringBudgetRefusalError
	if !errors.As(err, &refusal) || refusal.Budget != "attempt" || refusal.Stage != DefinitionAuthoringPreparationDerivation || refusal.Cause.Error() != "derivation input attempt budget exhausted" || fresh || denied.PermitID != "" {
		t.Fatalf("no-permit typed guard: %+v %v", denied, err)
	}
	_, replay, fresh, err := r.ReserveDerivation(d.TargetRequestID, "provider", nil)
	if err != nil || fresh || replay.PermitID != reserved.PermitID {
		t.Fatalf("duplicate granted permit: %v", err)
	}
	d.TargetRequestID, d.PhaseCode, d.ProviderBindingHash = "derive-3", "attempt_budget", "provider"
	d.DiagnosticID = d.ExpectedDiagnosticID()
	if _, _, _, err := r.RecordPreparationDiagnostic(d); err != nil {
		t.Fatalf("deterministic budget diagnosis: %v", err)
	}
}

func TestPreparationDiagnosticArchiveDistinctLimitAndImmutableCopy(t *testing.T) {
	r, d := preparationModelFixture()
	r.PreparationDiagnostics = []DefinitionAuthoringPreparationDiagnostic{d}
	w := Workspace{ID: r.Input.WorkspaceID, Revision: 10, DefinitionAuthoring: &r, DefinitionAuthoringHistory: []DefinitionAuthoringConfirmedRevision{{Record: r.Clone()}}}
	if err := w.ValidatePreparationDiagnosticHistory(); err != nil {
		t.Fatal(err)
	}
	altered := w.DefinitionAuthoringHistory[0].Clone()
	altered.Record.PreparationDiagnostics[0].ObservedWorkspaceRevision++
	w.DefinitionAuthoringHistory[0] = altered
	if w.ValidatePreparationDiagnosticHistory() == nil {
		t.Fatal("archive rewrote first observation")
	}
	w.DefinitionAuthoringHistory[0].Record = r.Clone()
	w.DefinitionAuthoring = &DefinitionAuthoringRecord{Input: r.Input, InputHash: r.InputHash, TemplateSnapshot: r.TemplateSnapshot, TemplateSnapshotHash: r.TemplateSnapshotHash}
	if w.ValidatePreparationDiagnosticHistory() == nil {
		t.Fatal("successor lost inherited diagnostic")
	}
	w.DefinitionAuthoring = &r
	for n := 0; n < DefinitionAuthoringPreparationDiagnosticLimit; n++ {
		other := d.Clone()
		other.TargetRequestID = fmt.Sprintf("history-%d", n)
		other.DiagnosticID = other.ExpectedDiagnosticID()
		w.DefinitionAuthoringHistory[0].Record.PreparationDiagnostics = append(w.DefinitionAuthoringHistory[0].Record.PreparationDiagnostics, other)
	}
	if w.ValidatePreparationDiagnosticHistory() == nil {
		t.Fatal("history escaped lineage limit")
	}
}

func TestPreparationDiagnosticSpecificationScopeConfigDriftAndReview(t *testing.T) {
	r, proposal := authoringLedgerFixture(t)
	d := NewDefinitionAuthoringPreparationDiagnostic(DefinitionAuthoringPreparationDiagnostic{WorkspaceID: r.Input.WorkspaceID, InputHash: r.InputHash, TemplateSnapshotHash: r.TemplateSnapshotHash, ObservedWorkspaceRevision: 1, Stage: DefinitionAuthoringPreparationProposal, TargetRequestID: "prep-proposal", DerivationPermitID: r.DerivationRequests[0].PermitID, PhaseCode: "config", ProviderBindingHash: "changed-provider"}, []byte("provider configuration changed"))
	if _, _, _, err := r.RecordPreparationDiagnostic(d); err != nil {
		t.Fatal(err)
	}
	d.PhaseCode = "render"
	d.DiagnosticID = d.ExpectedDiagnosticID()
	if _, _, _, err := r.RecordPreparationDiagnostic(d); err == nil {
		t.Fatal("render ignored sealed provider binding")
	}
	d.ProviderBindingHash = "provider"
	d.DiagnosticID = d.ExpectedDiagnosticID()
	if _, _, _, err := r.RecordPreparationDiagnostic(d); err != nil {
		t.Fatal(err)
	}
	request := DefinitionAuthoringSpecificationRequest{RequestID: "proposal", DerivationPermitID: d.DerivationPermitID, Phase: DefinitionSpecificationProposal, ProviderBindingHash: "provider"}
	r, request, _, _ = r.ReserveSpecification(request)
	var err error
	r, err = r.CompleteSpecification(request.PermitID, &proposal, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	d.Stage, d.TargetRequestID, d.ReviewKind, d.ProposalHash = DefinitionAuthoringPreparationReview, "prep-review", GoalReviewTraceability, proposal.ContentHash()
	d.DiagnosticID = d.ExpectedDiagnosticID()
	if _, _, _, err := r.RecordPreparationDiagnostic(d); err != nil {
		t.Fatal(err)
	}
	d.ProposalHash = "invented"
	d.DiagnosticID = d.ExpectedDiagnosticID()
	if _, _, _, err := r.RecordPreparationDiagnostic(d); err == nil {
		t.Fatal("review has no exact proposal")
	}
	for n := 0; n < DefinitionAuthoringSpecificationAttemptLimit; n++ {
		req := DefinitionAuthoringSpecificationRequest{RequestID: fmt.Sprintf("review-%d", n), DerivationPermitID: d.DerivationPermitID, Phase: DefinitionSpecificationCritic, ReviewKind: GoalReviewTraceability, ProposalHash: proposal.ContentHash(), ProviderBindingHash: "provider"}
		r, _, _, err = r.ReserveSpecification(req)
		if err != nil {
			t.Fatal(err)
		}
	}
	_, noPermit, fresh, err := r.ReserveSpecification(DefinitionAuthoringSpecificationRequest{RequestID: "review-overflow", DerivationPermitID: d.DerivationPermitID, Phase: DefinitionSpecificationCritic, ReviewKind: GoalReviewTraceability, ProposalHash: proposal.ContentHash(), ProviderBindingHash: "provider"})
	var refusal *DefinitionAuthoringBudgetRefusalError
	if !errors.As(err, &refusal) || refusal.Stage != DefinitionAuthoringPreparationReview || refusal.RequestID != "review-overflow" || fresh || noPermit.PermitID != "" {
		t.Fatalf("review typed guard: %v", err)
	}
}
