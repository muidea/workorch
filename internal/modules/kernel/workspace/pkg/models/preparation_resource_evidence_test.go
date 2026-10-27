package workspace

import (
	"strings"
	"testing"
	"time"

	"github.com/muidea/workorch/internal/pkg/fileobservation"
)

func TestPreparationResourceEvidenceClassificationAndRawBinding(t *testing.T) {
	raw := []byte{0xff, 0, '\r', '\n'}
	r := WorkspacePreparationResourceObservation{ID: "input", Required: true, ExpectedContentHash: "sha256:" + fileobservation.Digest(raw), Observation: fileobservation.Resource{Ref: "input", Complete: true, Content: raw, ContentBytes: len(raw), ContentSHA256: fileobservation.Digest(raw), Inode: 1, Mode: 0100600, DiagnosticSHA256: fileobservation.Digest(nil)}}
	if r.BlockedReason() != "resource_observed_remaining_checks" {
		t.Fatal("exact observation not classified")
	}
	for _, missing := range []string{"", "declared-snapshot", "sha256:abc", strings.Repeat("a", 64)} {
		changed := r
		changed.ExpectedContentHash = missing
		if changed.BlockedReason() != "resource_expected_hash_unavailable" {
			t.Fatal("untyped declaration promoted to actual hash")
		}
	}
	changed := r
	changed.ExpectedContentHash = "sha256:" + strings.Repeat("a", 64)
	if changed.BlockedReason() != "resource_content_changed" {
		t.Fatal("changed resource not blocked")
	}
	now := time.Now().UTC()
	e := WorkspacePreparationResourceEvidence{Version: PreparationResourceEvidenceVersion, WorkspaceID: "ws", RequestID: "original", PayloadHash: "payload", ObservedWorkspaceRevision: 2, SpecificationHash: "spec", DeliverableDirectory: "/workspace", StartedAt: now, CompletedAt: now, Resources: []WorkspacePreparationResourceObservation{r}}
	e.ContentHash = e.Hash()
	if e.Validate() != nil {
		t.Fatal("original full bytes rejected")
	}
	for _, mutate := range []func(*WorkspacePreparationResourceEvidence){
		func(e *WorkspacePreparationResourceEvidence) { e.Resources[0].Observation.Content[0] = 'x' },
		func(e *WorkspacePreparationResourceEvidence) { e.Resources = append(e.Resources, e.Resources[0]) },
		func(e *WorkspacePreparationResourceEvidence) {
			e.Resources[0].ExpectedContentHash = string([]byte{0xff})
		},
		func(e *WorkspacePreparationResourceEvidence) { e.DeliverableDirectory = "relative" },
		func(e *WorkspacePreparationResourceEvidence) { e.CompletedAt = e.StartedAt.Add(-time.Second) },
		func(e *WorkspacePreparationResourceEvidence) {
			e.CompletedAt = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
		},
		func(e *WorkspacePreparationResourceEvidence) { e.WorkspaceID = "\xff" },
	} {
		bad := e.Clone()
		mutate(&bad)
		bad.ContentHash = bad.Hash()
		if bad.Validate() == nil {
			t.Fatal("rehash bypassed structural evidence checks")
		}
	}
	clone := e.Clone()
	clone.Resources[0].Observation.Content[0] = 'x'
	if e.Resources[0].Observation.Content[0] != 0xff {
		t.Fatal("resource evidence aliases original")
	}
}
