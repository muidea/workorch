package workorchport

import (
	"testing"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchresultcontract"
)

func TestResultAndPatchProjectionPreserveCompleteDetachedFields(t *testing.T) {
	summary := &workorchresultcontract.ResultSummary{
		EvidenceRefs: []string{"evidence-result"},
		DoDItems:     []workorchresultcontract.DoDAssessmentItem{{Item: "dod", EvidenceRefs: []string{"evidence-dod"}}},
	}
	results := []projectioncontract.Result{{
		ID: "result-1", EvidenceRefs: []string{"evidence-1"}, ArtifactRefs: []string{"artifact-1"}, SummaryDetail: summary,
	}}
	patches := []projectioncontract.Patch{{
		ID: "patch-1", AffectedFiles: []string{"main.go"}, RiskLevel: "medium", RiskReasons: []string{"generated"},
		Diagnostics: []map[string]any{{"code": "warning", "detail": map[string]any{"state": "kept"}}},
	}}

	gotResults := projectioncontract.CloneResults(results)
	gotPatches := projectioncontract.ClonePatches(patches)
	results[0].EvidenceRefs[0] = "mutated"
	summary.EvidenceRefs[0] = "mutated"
	summary.DoDItems[0].EvidenceRefs[0] = "mutated"
	patches[0].AffectedFiles[0] = "mutated.go"
	patches[0].RiskReasons[0] = "mutated"
	patches[0].Diagnostics[0]["code"] = "mutated"
	patches[0].Diagnostics[0]["detail"].(map[string]any)["state"] = "mutated"

	if gotResults[0].EvidenceRefs[0] != "evidence-1" || gotResults[0].SummaryDetail.EvidenceRefs[0] != "evidence-result" || gotResults[0].SummaryDetail.DoDItems[0].EvidenceRefs[0] != "evidence-dod" {
		t.Fatalf("result projection shares source storage: %+v", gotResults[0])
	}
	if gotPatches[0].AffectedFiles[0] != "main.go" || gotPatches[0].RiskLevel != "medium" || gotPatches[0].RiskReasons[0] != "generated" {
		t.Fatalf("patch projection lost complete risk fields: %+v", gotPatches[0])
	}
	if code, ok := gotPatches[0].Diagnostics[0]["code"].(string); !ok || code != "warning" {
		t.Fatalf("patch diagnostic = %+v", gotPatches[0].Diagnostics)
	}
	if detail := gotPatches[0].Diagnostics[0]["detail"].(map[string]any); detail["state"] != "kept" {
		t.Fatalf("nested patch diagnostic = %+v", detail)
	}
}

func TestTaskProgressProjectionDeepClonesNestedFacts(t *testing.T) {
	input := projectioncontract.TaskProgress{
		WorkspaceID: "ws-1", RunID: "run-1",
		Milestones: []projectioncontract.ProgressMilestone{{ID: "milestone-1", EvidenceRefs: []string{"milestone-evidence"}}},
		Items:      []projectioncontract.ProgressItem{{ID: "item-1", EvidenceRefs: []string{"item-evidence"}}},
		RepairLoop: projectioncontract.RepairLoop{
			StructuredFailureSignals: map[string]any{"code": "contract"},
			MissingDoDItems:          []map[string]any{{"id": "dod-1"}},
		},
		Recovery: projectioncontract.RecoveryPlan{BlockingResource: &projectioncontract.BlockingResourceRef{
			OwnerKind: "decision", ResourceID: "ticket-1", AllowedUserActions: []string{"revise_definition_and_restart"},
		}},
		Runtime: &projectioncontract.RuntimeProgress{
			MissingEvidenceKinds: []string{"validation"},
			PrerequisiteInvalidations: []projectioncontract.RuntimePrerequisiteRefreshInvalidation{{
				PrerequisiteIDs: []string{"prerequisite-1"},
				ResourceChanges: []projectioncontract.RuntimePrerequisiteResourceChange{{SubjectRef: "main.go"}},
			}},
		},
	}

	got := projectioncontract.CloneTaskProgress(input)
	input.Milestones[0].EvidenceRefs[0] = "mutated"
	input.Items[0].EvidenceRefs[0] = "mutated"
	input.RepairLoop.StructuredFailureSignals["code"] = "mutated"
	input.RepairLoop.MissingDoDItems[0]["id"] = "mutated"
	input.Recovery.BlockingResource.AllowedUserActions[0] = "mutated"
	input.Runtime.MissingEvidenceKinds[0] = "mutated"
	input.Runtime.PrerequisiteInvalidations[0].PrerequisiteIDs[0] = "mutated"
	input.Runtime.PrerequisiteInvalidations[0].ResourceChanges[0].SubjectRef = "mutated.go"

	if got.Milestones[0].EvidenceRefs[0] != "milestone-evidence" || got.Items[0].EvidenceRefs[0] != "item-evidence" {
		t.Fatalf("progress projection shares nested evidence refs: %+v", got)
	}
	if got.Recovery.BlockingResource == nil || got.Recovery.BlockingResource.AllowedUserActions[0] != "revise_definition_and_restart" || got.Runtime.MissingEvidenceKinds[0] != "validation" {
		t.Fatalf("progress projection shares source slices: %+v", got)
	}
	invalidation := got.Runtime.PrerequisiteInvalidations[0]
	if invalidation.PrerequisiteIDs[0] != "prerequisite-1" || invalidation.ResourceChanges[0].SubjectRef != "main.go" {
		t.Fatalf("runtime invalidation shares source storage: %+v", invalidation)
	}
	if code, ok := got.RepairLoop.StructuredFailureSignals["code"].(string); !ok || code != "contract" || got.RepairLoop.MissingDoDItems[0]["id"] != "dod-1" {
		t.Fatalf("repair signal = %+v", got.RepairLoop.StructuredFailureSignals)
	}
}
