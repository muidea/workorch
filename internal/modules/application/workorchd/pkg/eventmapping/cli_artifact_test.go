package eventmapping

import (
	"reflect"
	"testing"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func TestCLIArtifactMappingRoundTripsAndDetachesCollections(t *testing.T) {
	artifacts := []projectioncontract.Artifact{{
		ID: "artifact-1", WorkspaceID: "workspace-1", RunID: "run-1", EvidenceRefs: []string{"evidence-1"},
		Metadata: map[string]any{"nested": map[string]any{"values": []any{"kept"}}},
	}}
	continuations := []projectioncontract.ArtifactContinuation{{
		ID: "continuation-1", WorkspaceID: "workspace-1", RunID: "run-1", ArtifactID: "artifact-1",
		ReceivedSequences: []int{1}, CoverageTargets: []string{"main.go"}, EvidenceRefs: []string{"evidence-1"},
		Progress: map[string]any{"nested": map[string]any{"state": "kept"}},
	}}
	patches := []projectioncontract.Patch{{
		ID: "patch-1", WorkspaceID: "workspace-1", RunID: "run-1", AffectedFiles: []string{"main.go"},
		RiskReasons: []string{"generated"}, Diagnostics: []map[string]any{{"nested": map[string]any{"code": "kept"}}},
	}}

	artifactDTOs, err := artifactDTOs(artifacts)
	if err != nil {
		t.Fatalf("artifact DTOs: %v", err)
	}
	continuationDTOs, err := artifactContinuationDTOs(continuations)
	if err != nil {
		t.Fatalf("continuation DTOs: %v", err)
	}
	patchDTOs, err := patchDTOs(patches)
	if err != nil {
		t.Fatalf("patch DTOs: %v", err)
	}

	artifacts[0].EvidenceRefs[0] = "mutated"
	artifacts[0].Metadata["nested"].(map[string]any)["values"].([]any)[0] = "mutated"
	continuations[0].ReceivedSequences[0] = 2
	continuations[0].CoverageTargets[0] = "mutated.go"
	continuations[0].Progress["nested"].(map[string]any)["state"] = "mutated"
	patches[0].AffectedFiles[0] = "mutated.go"
	patches[0].Diagnostics[0]["nested"].(map[string]any)["code"] = "mutated"

	wantArtifacts := []projectioncontract.Artifact{{
		ID: "artifact-1", WorkspaceID: "workspace-1", RunID: "run-1", EvidenceRefs: []string{"evidence-1"},
		Metadata: map[string]any{"nested": map[string]any{"values": []any{"kept"}}},
	}}
	wantContinuations := []projectioncontract.ArtifactContinuation{{
		ID: "continuation-1", WorkspaceID: "workspace-1", RunID: "run-1", ArtifactID: "artifact-1",
		ReceivedSequences: []int{1}, CoverageTargets: []string{"main.go"}, EvidenceRefs: []string{"evidence-1"},
		Progress: map[string]any{"nested": map[string]any{"state": "kept"}},
	}}
	wantPatches := []projectioncontract.Patch{{
		ID: "patch-1", WorkspaceID: "workspace-1", RunID: "run-1", AffectedFiles: []string{"main.go"},
		RiskReasons: []string{"generated"}, Diagnostics: []map[string]any{{"nested": map[string]any{"code": "kept"}}},
	}}
	if got := artifactProjections(artifactDTOs); !reflect.DeepEqual(got, wantArtifacts) {
		t.Fatalf("artifact round trip = %#v, want %#v", got, wantArtifacts)
	}
	if got := artifactContinuationProjections(continuationDTOs); !reflect.DeepEqual(got, wantContinuations) {
		t.Fatalf("continuation round trip = %#v, want %#v", got, wantContinuations)
	}
	if got := patchProjections(patchDTOs); !reflect.DeepEqual(got, wantPatches) {
		t.Fatalf("patch round trip = %#v, want %#v", got, wantPatches)
	}
}
