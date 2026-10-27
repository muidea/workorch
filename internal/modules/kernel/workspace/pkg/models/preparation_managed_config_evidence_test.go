package workspace

import (
	"strings"
	"testing"
	"time"

	"github.com/muidea/workorch/internal/pkg/fileobservation"
)

func TestPreparationManagedConfigEvidenceStrictPrivateBindingAndClone(t *testing.T) {
	now := time.Now().UTC()
	raw := []byte(`{"n":9007199254740993}`)
	r := WorkspacePreparationManagedConfigObservation{Scope: "system", Path: "a.json", Policy: "system_exact", DefaultsJSON: append([]byte(nil), raw...), Observation: fileobservation.Resource{Ref: "a.json", Complete: true, Content: raw, ContentBytes: len(raw), ContentSHA256: fileobservation.Digest(raw), Inode: 1, Mode: 0100600, DiagnosticSHA256: fileobservation.Digest(nil)}}
	e := WorkspacePreparationManagedConfigEvidence{Version: PreparationManagedConfigEvidenceVersion, Ref: PreparationManagedConfigRefPrefix + strings.Repeat("a", 32), WorkspaceID: "ws", RequestID: "original", PayloadHash: "payload", ObservedWorkspaceRevision: 2, ConfigurationHash: "config", InternalDirectory: "/internal", StartedAt: now, CompletedAt: now, Files: []WorkspacePreparationManagedConfigObservation{r}}
	e.ContentHash = e.Hash()
	if e.Validate() != nil || e.BlockedReason() != "managed_configs_observed_remaining_checks" {
		t.Fatal("valid evidence rejected")
	}
	for _, change := range []func(*WorkspacePreparationManagedConfigEvidence){
		func(e *WorkspacePreparationManagedConfigEvidence) { e.Ref = e.ContentHash },
		func(e *WorkspacePreparationManagedConfigEvidence) { e.Files[0].Policy = "workspace_preserve" },
		func(e *WorkspacePreparationManagedConfigEvidence) { e.Files[0].Path = "../escape" },
		func(e *WorkspacePreparationManagedConfigEvidence) { e.Files[0].Observation.Content[0] = 'x' },
		func(e *WorkspacePreparationManagedConfigEvidence) { e.Files[0].DefaultsJSON = []byte(`{"x":1,"x":2}`) },
		func(e *WorkspacePreparationManagedConfigEvidence) { e.Files = append(e.Files, e.Files[0]) },
		func(e *WorkspacePreparationManagedConfigEvidence) { e.InternalDirectory = "relative" },
		func(e *WorkspacePreparationManagedConfigEvidence) { e.CompletedAt = e.StartedAt.Add(-time.Second) },
	} {
		bad := e.Clone()
		change(&bad)
		bad.ContentHash = bad.Hash()
		if bad.Validate() == nil {
			t.Fatal("rehash bypassed structure")
		}
	}
	clone := e.Clone()
	clone.Files[0].DefaultsJSON[0] = 'x'
	clone.Files[0].Observation.Content[0] = 'x'
	if e.Files[0].DefaultsJSON[0] != '{' || e.Files[0].Observation.Content[0] != '{' {
		t.Fatal("private bytes alias clone")
	}
}
