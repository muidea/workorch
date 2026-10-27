package workspace

import (
	"bytes"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	filepreparation "github.com/muidea/workorch/internal/modules/blocks/file/pkg/preparation"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
)

func fileRevalidationWorkspaceFixture() *Workspace {
	w := evaluationWorkspaceFixture()
	a := &w.StartupPreparation.Attempts[0]
	d, r := &w.PreparationDirectoryEvidence[0], &w.PreparationResourceEvidence[0]
	c := WorkspacePreparationManagedConfigEvidence{Version: PreparationManagedConfigEvidenceVersion, Ref: PreparationManagedConfigRefPrefix + strings.Repeat("a", 32), WorkspaceID: w.ID, RequestID: a.RequestID, PayloadHash: a.PayloadHash, ObservedWorkspaceRevision: 2, ConfigurationHash: a.Binding.AuthoringConfigurationHash, InternalDirectory: d.Internal.Path, StartedAt: d.StartedAt, CompletedAt: d.CompletedAt}
	c.ContentHash = c.Hash()
	w.PreparationManagedConfigEvidence = []WorkspacePreparationManagedConfigEvidence{c}
	a.ManagedConfigEvidenceRef = c.Ref
	a.Checks = append(a.Checks, WorkspacePreparationCheckResult{ID: "managed_configuration", Owner: "file", Required: true, State: "blocked", Reason: c.BlockedReason(), SubjectHash: PreparationFileSubjectHash("managed_configuration", d.Internal.Path, d.Deliverable.Path, d.Internal.Path, "")})
	o := filepreparation.Observation{Version: filepreparation.Version, OwnerEpoch: "file-preparation-epoch:" + strings.Repeat("A", 26), TokenID: strings.Repeat("B", 26), Scope: filepreparation.Scope{WorkspaceID: w.ID, RequestID: a.RequestID, PayloadHash: a.PayloadHash, WorkspaceRevision: d.ObservedWorkspaceRevision, InternalDirectory: d.Internal.Path, DeliverableDirectory: d.Deliverable.Path, ResourceRefs: []string{r.Resources[0].Observation.Ref}}, StartedAt: d.CompletedAt, ObservedAt: d.CompletedAt, ExpiresAt: d.CompletedAt.Add(filepreparation.MaxAge), Internal: d.Internal.Clone(), Deliverable: d.Deliverable.Clone(), Resources: []fileobservation.Resource{r.Resources[0].Observation.Clone()}}
	d.RevalidationVersion, d.Revalidation = filepreparation.Version, &o
	d.ContentHash = d.Hash()
	a.DirectoryEvidenceHash = d.ContentHash
	sort.Slice(a.Checks, func(i, j int) bool { return a.Checks[i].ID < a.Checks[j].ID })
	a.Checks = EvaluatePreparationFileChecks(*a, d, r, &c)
	return w
}

func TestPreparationFileRevalidationFullSourcesExpiryAndOwnerRestart(t *testing.T) {
	w := fileRevalidationWorkspaceFixture()
	o := w.PreparationDirectoryEvidence[0].Revalidation
	if err := ValidatePreparationEvidencePersistence(w); err != nil {
		t.Fatal("fixture invalid", err)
	}
	for _, scenario := range []struct {
		name, epoch string
		now         time.Time
		stale       bool
	}{
		{"fresh", o.OwnerEpoch, o.ObservedAt, false},
		{"before_expiry", o.OwnerEpoch, o.ExpiresAt.Add(-time.Nanosecond), false},
		{"at_expiry", o.OwnerEpoch, o.ExpiresAt, true},
		{"clock_rollback", o.OwnerEpoch, o.ObservedAt.Add(-time.Nanosecond), true},
		{"unknown_epoch", "", o.ObservedAt, true},
		{"restart", "file-preparation-epoch:" + strings.Repeat("C", 26), o.ObservedAt, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			before := w.PreparationDirectoryEvidence[0].Clone()
			stale, err := PreparationFileRevalidationStale(w, scenario.now, scenario.epoch)
			if err != nil || stale != scenario.stale || !reflect.DeepEqual(before, w.PreparationDirectoryEvidence[0]) {
				t.Fatal("wrong known-currentness or modified history", stale, err)
			}
		})
	}
	for _, change := range []func(*filepreparation.Observation){
		func(o *filepreparation.Observation) { o.Internal.Inode++ },
		func(o *filepreparation.Observation) { o.Resources[0].Inode++ },
		func(o *filepreparation.Observation) {
			o.Resources[0].Content[0] = 'x'
			o.Resources[0].ContentSHA256 = fileobservation.Digest(o.Resources[0].Content)
		},
	} {
		changed := fileRevalidationWorkspaceFixture()
		d := &changed.PreparationDirectoryEvidence[0]
		change(d.Revalidation)
		d.ContentHash = d.Hash()
		changed.StartupPreparation.Attempts[0].DirectoryEvidenceHash = d.ContentHash
		if err := ValidatePreparationEvidencePersistence(changed); err != nil {
			t.Fatal("real differing after-image must remain historical evidence", err)
		}
		if stale, err := PreparationFileRevalidationStale(changed, o.ObservedAt, o.OwnerEpoch); !stale || err != nil {
			t.Fatal("rehashed drift accepted", stale, err)
		}
	}
}

func TestPreparationFileRevalidationRejectsRehashedScopeAndPreservesHistory(t *testing.T) {
	for _, change := range []func(*WorkspacePreparationDirectoryEvidence){
		func(d *WorkspacePreparationDirectoryEvidence) { d.RevalidationVersion = "" },
		func(d *WorkspacePreparationDirectoryEvidence) { d.Revalidation.Scope.RequestID = "other" },
		func(d *WorkspacePreparationDirectoryEvidence) { d.Revalidation.Scope.WorkspaceRevision++ },
		func(d *WorkspacePreparationDirectoryEvidence) {
			d.Revalidation.Scope.ResourceRefs = nil
			d.Revalidation.Resources = nil
		},
		func(d *WorkspacePreparationDirectoryEvidence) {
			d.Revalidation.Scope.Configs = []filepreparation.ConfigTarget{{Scope: "workspace", Path: "input.txt"}}
			d.Revalidation.Configs = d.Revalidation.Resources
		},
		func(d *WorkspacePreparationDirectoryEvidence) {
			d.Revalidation.StartedAt = d.CompletedAt.Add(-time.Second)
			d.Revalidation.ExpiresAt = d.Revalidation.StartedAt.Add(filepreparation.MaxAge)
		},
	} {
		w := fileRevalidationWorkspaceFixture()
		d := &w.PreparationDirectoryEvidence[0]
		change(d)
		d.ContentHash = d.Hash()
		w.StartupPreparation.Attempts[0].DirectoryEvidenceHash = d.ContentHash
		if ValidatePreparationEvidencePersistence(w) == nil {
			t.Fatal("rehashed replacement/subset accepted")
		}
	}
	w := fileRevalidationWorkspaceFixture()
	d := &w.PreparationDirectoryEvidence[0]
	clone := ClonePreparationDirectoryEvidence(w.PreparationDirectoryEvidence)
	clone[0].Revalidation.Resources[0].Content[0] = 'x'
	if !bytes.Equal(d.Revalidation.Resources[0].Content, w.PreparationResourceEvidence[0].Resources[0].Observation.Content) {
		t.Fatal("clone aliases original bytes")
	}
	d.Revalidation = nil
	d.ContentHash = d.Hash()
	w.StartupPreparation.Attempts[0].DirectoryEvidenceHash = d.ContentHash
	if stale, err := PreparationFileRevalidationStale(w, d.CompletedAt, ""); !stale || err != nil {
		t.Fatal("missing observation accepted")
	}
	d.RevalidationVersion = ""
	d.ContentHash = d.Hash()
	w.StartupPreparation.Attempts[0].DirectoryEvidenceHash = d.ContentHash
	if stale, err := PreparationFileRevalidationStale(w, d.CompletedAt, ""); stale || err != nil || d.Revalidation != nil {
		t.Fatal("legacy history upgraded")
	}
}
