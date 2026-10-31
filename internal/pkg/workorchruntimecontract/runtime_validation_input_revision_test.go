package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func inputRevisionFixture(t *testing.T) (*RuntimeValidationInputRevision, RuntimeSemanticPlanningLedger) {
	t.Helper()
	m, d, _, binding := semanticInputReviewFixture(t)
	for i := range m.OwnerCatalogs {
		c := &m.OwnerCatalogs[i]
		for j := range c.ValidationSurfaces {
			s := &c.ValidationSurfaces[j]
			if s.ID == binding.ValidationRecipeID {
				s.InputTargetOwnerModuleID = RuntimeModuleFile
				s.InputTargetRefs = []string{binding.InputReview.Files[0].TargetRef}
			}
		}
		c.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(*c)
	}
	m.Hash = StableRuntimeSemanticPlanManifestHash(m)
	d.ManifestHash = m.Hash
	d.Proposals[1].ValidationInputReview.Checks[0].Conclusion = RuntimeValidationInputConflict
	d.Proposals[1].ValidationInputReview.Checks[0].Obligations[0].Finding = RuntimeValidationInputConflicting
	l := RuntimeSemanticPlanningLedger{Phase: RuntimeSemanticPlanningProofBatchDocumentAccepted, Manifest: &m, LogicalRequestID: runtimeSemanticBindingTestRequestID, ActiveBatchID: d.BatchID, BatchDocuments: []RuntimeSemanticPlanDocument{d}}
	v, err := CompileRuntimeValidationInputRevision(l)
	if err != nil {
		t.Fatal(err)
	}
	return v, l
}

func TestInputRevisionOriginalReviewAndEffectPreimage(t *testing.T) {
	v, l := inputRevisionFixture(t)
	before, _ := json.Marshal(l)
	p := &RuntimeOperationReplayProof{OwnerModuleID: v.OwnerModuleID, OperationID: "edit", TargetRefs: v.TargetRefs(), BeforeHash: v.Binding.InputReview.Files[0].ContentHash, AfterHash: RuntimeValidationInputContentHash("changed actual input"), AdmissibilitySnapshotID: "original-owner-catalog"}
	if !v.MatchesReplayProof(p) {
		t.Fatal("exact real effect was blocked")
	}
	for name, mutate := range map[string]func(*RuntimeOperationReplayProof){
		"stale": func(p *RuntimeOperationReplayProof) { p.BeforeHash = "stale" }, "unchanged": func(p *RuntimeOperationReplayProof) { p.AfterHash = p.BeforeHash }, "other owner": func(p *RuntimeOperationReplayProof) { p.OwnerModuleID = RuntimeModuleGit }, "other input": func(p *RuntimeOperationReplayProof) { p.TargetRefs = []string{"unrelated"} }, "missing identity": func(p *RuntimeOperationReplayProof) { p.OperationID = "" },
	} {
		t.Run(name, func(t *testing.T) {
			bad := p.Clone()
			mutate(&bad)
			if v.MatchesReplayProof(&bad) {
				t.Fatal("unbound effect opened reviewed input")
			}
		})
	}
	for _, conclusion := range []string{RuntimeValidationInputConsistent, RuntimeValidationInputInconclusive} {
		bad := v.Clone()
		for i := range bad.Report.Checks {
			bad.Report.Checks[i].Conclusion = conclusion
		}
		if bad.Validate() == nil {
			t.Fatal("unchanged report resampling acquired effect permission")
		}
	}
	copy := v.Clone()
	copy.Binding.InputReview.Files[0].Content = "mutated"
	copy.Report.Checks[0].Reason = "mutated"
	after, _ := json.Marshal(l)
	if string(before) != string(after) {
		t.Fatal("revision compilation/clone mutated raw original")
	}
}

func TestInputRevisionSourceRestartDuplicateAndWorkspaceIsolation(t *testing.T) {
	v, l := inputRevisionFixture(t)
	s := RuntimeSemanticPlanningLedger{History: []RuntimeSemanticPlanningGeneration{{Reason: RuntimeSemanticPlanningRetirementInputRevision, Snapshot: l}}}
	if err := ValidateRuntimeInputRevisionSource(v, s); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*RuntimeValidationInputRevision){"workspace": func(v *RuntimeValidationInputRevision) { v.WorkspaceID = "other" }, "run": func(v *RuntimeValidationInputRevision) { v.RunID = "other" }, "criteria": func(v *RuntimeValidationInputRevision) { v.Binding.DefinitionRevision = "changed" }, "original report": func(v *RuntimeValidationInputRevision) { v.Report.Checks[0].Reason = "changed" }, "request": func(v *RuntimeValidationInputRevision) { v.SourceLogicalRequestID = "other" }} {
		t.Run(name, func(t *testing.T) {
			bad := v.Clone()
			mutate(bad)
			if ValidateRuntimeInputRevisionSource(bad, s) == nil {
				t.Fatal("self-consistent replacement accepted")
			}
		})
	}
	w := RuntimeImplementationWorkLedger{}
	cwi := v.Binding.CoverageWorkItemID
	pending, err := PendingRuntimeValidationInputRevision(s, w, cwi)
	if err != nil || !reflect.DeepEqual(pending, v) {
		t.Fatal(err)
	}
	w.Rounds = []RuntimeImplementationWorkRound{{CoverageWorkItemID: cwi, ScopeRequest: RuntimeImplementationWorkScopeRequest{InputRevision: v.Clone()}}}
	pending, err = PendingRuntimeValidationInputRevision(s, w, cwi)
	if err != nil || pending == nil {
		t.Fatal("pending effect was assumed complete")
	}
	w.Rounds[0].CompletedReceiptSetHash = "real-receipt-set"
	pending, err = PendingRuntimeValidationInputRevision(s, w, cwi)
	if err != nil || pending != nil {
		t.Fatal("completed effect was replayed")
	}
	raw, _ := json.Marshal(w)
	var restarted RuntimeImplementationWorkLedger
	if err := json.Unmarshal(raw, &restarted); err != nil {
		t.Fatal(err)
	}
	if pending, err = PendingRuntimeValidationInputRevision(s, restarted, cwi); err != nil || pending != nil {
		t.Fatal("restart reset revision consumption")
	}
	w.Rounds = append(w.Rounds, w.Rounds[0])
	if _, err = PendingRuntimeValidationInputRevision(s, w, cwi); err == nil {
		t.Fatal("same review funded two effect rounds")
	}
	s.History = append(s.History, s.History[0])
	if ValidateRuntimeInputRevisionSource(v, s) == nil {
		t.Fatal("duplicate source accepted")
	}
}

func TestInputRevisionProtectionDoesNotGrantUnrelatedTargets(t *testing.T) {
	v, l := inputRevisionFixture(t)
	other := l.Manifest.OwnerCatalogs[1].Clone()
	for i := range other.ValidationSurfaces {
		other.ValidationSurfaces[i].InputTargetOwnerModuleID = RuntimeModuleFile
		other.ValidationSurfaces[i].InputTargetRefs = append(v.TargetRefs(), "other-input")
	}
	excluded := runtimeWorkInputExclusions(RuntimeModuleFile, []RuntimeOperationAdmissibility{other}, v)
	if !reflect.DeepEqual(excluded, []string{"other-input"}) {
		t.Fatalf("unrelated protection lost: %v", excluded)
	}
	p := RuntimeSemanticProofProposal{OwnerModuleID: v.OwnerModuleID, Target: RuntimeOperationTargetAuthorization{Policy: RuntimeOperationTargetPolicyExistingOwnerScope, OwnerScopeTargetRefs: v.TargetRefs()}}
	if err := validateRuntimeInputRevisionTargets(v, p); err != nil {
		t.Fatal(err)
	}
	p.Target.OwnerScopeTargetRefs = []string{"other-input"}
	if validateRuntimeInputRevisionTargets(v, p) == nil {
		t.Fatal("unrelated input writable")
	}
	p.Target = RuntimeOperationTargetAuthorization{Policy: RuntimeOperationTargetPolicyProspectiveOwnerTarget, ProspectiveTargetRefs: v.TargetRefs()}
	if validateRuntimeInputRevisionTargets(v, p) == nil {
		t.Fatal("create-only authority replaced existing input")
	}
	legacy := implementationWorkPolicyFixture()
	raw, _ := json.Marshal(legacy)
	if strings.Contains(string(raw), "inputRevision") {
		t.Fatal("empty extension changed legacy JSON")
	}
	legacy.InputRevisionProtocolRef = "unknown"
	if legacy.Validate() == nil {
		t.Fatal("unknown protocol accepted")
	}
}

func TestInputRevisionRequiresOriginalRetiredRequest(t *testing.T) {
	v, l := inputRevisionFixture(t)
	l.StablePrefixHash = "original-stable-prefix"
	d := l.BatchDocuments[0]
	s := RuntimeCandidateRequestSession{
		LogicalRequestID: v.SourceLogicalRequestID, DocumentKind: RuntimeProviderDocumentSemanticPlan,
		ManifestHash: v.SourceManifestHash, StablePrefixHash: l.StablePrefixHash,
		State: RuntimeCandidateRequestSessionTerminal,
		ExpectedDocument: &RuntimeExpectedDocumentContract{
			LogicalRequestID: v.SourceLogicalRequestID, ManifestHash: v.SourceManifestHash,
			DefinitionSeedHash: d.DefinitionSeedHash, PlanStep: d.PlanStep,
			BatchID: d.BatchID, BatchSequence: d.BatchSequence, BatchCoverageWorkItemIDs: d.BatchCoverageWorkItemIDs,
		},
		LastOutcome: RuntimeCandidateRequestOutcome{Class: RuntimeCandidateRequestOutcomePrerequisiteStale,
			Code:      RuntimeValidationInputRevisionRetirementOutcome,
			Signature: RuntimeValidationInputRevisionRetirementOutcome + "/" + v.SourceManifestHash + "/" + v.SourceLogicalRequestID},
	}
	history := RuntimeSemanticPlanningLedger{History: []RuntimeSemanticPlanningGeneration{{Reason: RuntimeSemanticPlanningRetirementInputRevision, Snapshot: l}}}
	if err := ValidateRuntimeInputRevisionRequestSources(history, []RuntimeCandidateRequestSession{s}); err != nil {
		t.Fatal(err)
	}
	if ValidateRuntimeInputRevisionRequestSources(history, nil) == nil {
		t.Fatal("report without original request accepted")
	}
	if ValidateRuntimeInputRevisionRequestSources(history, []RuntimeCandidateRequestSession{s, s}) == nil {
		t.Fatal("ambiguous original request accepted")
	}
	for name, mutate := range map[string]func(*RuntimeCandidateRequestSession){
		"pending":             func(s *RuntimeCandidateRequestSession) { s.State = RuntimeCandidateRequestSessionDocumentComplete },
		"unsettled provider":  func(s *RuntimeCandidateRequestSession) { s.ProviderSubmitted = true },
		"missing expectation": func(s *RuntimeCandidateRequestSession) { s.ExpectedDocument = nil },
		"other request":       func(s *RuntimeCandidateRequestSession) { s.LogicalRequestID = "other" },
		"other manifest":      func(s *RuntimeCandidateRequestSession) { s.ManifestHash = "other" },
		"other prefix":        func(s *RuntimeCandidateRequestSession) { s.StablePrefixHash = "other" },
		"other batch":         func(s *RuntimeCandidateRequestSession) { s.ExpectedDocument.BatchID = "other" },
		"other criteria":      func(s *RuntimeCandidateRequestSession) { s.ExpectedDocument.DefinitionSeedHash = "other" },
		"other retirement":    func(s *RuntimeCandidateRequestSession) { s.LastOutcome.Code = "other" },
		"other signature":     func(s *RuntimeCandidateRequestSession) { s.LastOutcome.Signature = "other" },
	} {
		t.Run(name, func(t *testing.T) {
			bad := s.Clone()
			mutate(&bad)
			if ValidateRuntimeInputRevisionRequestSources(history, []RuntimeCandidateRequestSession{bad}) == nil {
				t.Fatal("unbound retired request accepted")
			}
		})
	}
}
