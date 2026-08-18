package events

import (
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchresultcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestResultRuntimeTerminalDeliveryFactValidate(t *testing.T) {
	cmd := RuntimeTerminalDeliveryFact{
		RuntimeTerminalDeliveryFact: runtimecontract.RuntimeTerminalDeliveryFact{RuntimeTerminalDeliveryIdentity: runtimecontract.RuntimeTerminalDeliveryIdentity{WorkspaceID: "ws-1", RunID: "run-1", TerminalDeliveryID: "td-1", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1", Outcome: runtimecontract.TerminalOutcome{Kind: runtimecontract.TerminalOutcomeKindCompleted, Code: "completed", Summary: "done"}}, SourceOwner: "kernel/orchestrator", CausationID: "terminal-delivery/td-1"},
		Material:                    ResultMaterial{WorkspaceID: "ws-1", RunID: "run-1", GoalID: "goal-1", DoDID: "dod-1", Summary: "done"},
	}
	if err := cmd.Validate(); err != nil {
		t.Fatalf("valid terminal: %v", err)
	}
	cmd.Material.WorkspaceID = "ws-other"
	if err := cmd.Validate(); err == nil || !errors.Is(err, common.ErrConflict) {
		t.Fatalf("mismatched result state err = %v", err)
	}
	cmd.Material = ResultMaterial{}
	if err := cmd.Validate(); err == nil || !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("missing result err = %v", err)
	}
}

func TestCanonicalResultMaterialHashNormalizesReferenceSetsAndDetectsDrift(t *testing.T) {
	base := ResultMaterial{
		WorkspaceID: "ws-1", RunID: "run-1", GoalID: "goal-1", DoDID: "dod-1", Summary: "done",
		EvidenceRefs: []string{" evidence-b ", "evidence-a", "evidence-a"},
		ArtifactRefs: []common.ArtifactID{" artifact-b ", "artifact-a", "artifact-a"},
		SummaryDetail: workorchresultcontract.ResultSummary{
			EvidenceRefs: []string{" evidence-b ", "evidence-a", "evidence-a"},
			ArtifactRefs: []common.ArtifactID{"artifact-b", "artifact-a"},
		},
		DoDAssessment: workorchresultcontract.DoDAssessment{GoalCoverage: &workorchresultcontract.DoDCoverage{Items: []workorchresultcontract.DoDCoverageItem{{
			EvidenceRefs: []string{"evidence-b", "evidence-a"}, ArtifactRefs: []common.ArtifactID{"artifact-b", "artifact-a"},
		}}}},
	}
	first, err := CanonicalResultMaterialHash(base)
	if err != nil {
		t.Fatal(err)
	}
	reordered := base
	reordered.EvidenceRefs = []string{"evidence-a", "evidence-b"}
	reordered.ArtifactRefs = []common.ArtifactID{"artifact-a", "artifact-b"}
	reordered.SummaryDetail.EvidenceRefs = []string{"evidence-a", "evidence-b"}
	reordered.SummaryDetail.ArtifactRefs = []common.ArtifactID{"artifact-a", "artifact-b"}
	reordered.DoDAssessment.GoalCoverage.Items[0].EvidenceRefs = []string{"evidence-a", "evidence-b"}
	reordered.DoDAssessment.GoalCoverage.Items[0].ArtifactRefs = []common.ArtifactID{"artifact-a", "artifact-b"}
	second, err := CanonicalResultMaterialHash(reordered)
	if err != nil || first != second {
		t.Fatalf("normalized hashes differ: first=%q second=%q err=%v", first, second, err)
	}
	drifted := base
	drifted.Summary = "different"
	third, err := CanonicalResultMaterialHash(drifted)
	if err != nil || third == first {
		t.Fatalf("material drift hash = %q err=%v", third, err)
	}
	missingChild := base
	missingChild.SummaryDetail.ArtifactRefs = []common.ArtifactID{"artifact-missing"}
	if err := missingChild.Validate(); err == nil || !errors.Is(err, common.ErrConflict) {
		t.Fatalf("missing child reference err = %v, want conflict", err)
	}
}
