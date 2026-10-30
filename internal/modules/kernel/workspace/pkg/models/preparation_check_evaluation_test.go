package workspace

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/muidea/workorch/internal/pkg/fileobservation"
	"github.com/muidea/workorch/internal/pkg/soulbinding"
	payload "github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func evaluationWorkspaceFixture() *Workspace {
	record := preparationRecordFixture()
	record.Version = WorkspacePreparationWorkflowVersion
	a := &record.Attempts[0]
	a.State, a.CommittedWorkspaceRevision = WorkspacePreparationBlocked, 3
	a.CreationEpoch, a.EvaluationVersion = strings.Repeat("a", 32), PreparationCheckEvaluationVersion
	a.ObservationSnapshotHash = "observed-snapshot"
	a.CompletedAt = a.StartedAt.Add(time.Second)
	dir := func(path string) fileobservation.Directory {
		return fileobservation.Directory{Path: path, Complete: true, Inode: 1, Mode: 0040700, Readable: true, Writable: true, Searchable: true, DiagnosticSHA256: fileobservation.Digest(nil)}
	}
	d := WorkspacePreparationDirectoryEvidence{Version: PreparationDirectoryEvidenceVersion, WorkspaceID: "ws", RequestID: a.RequestID, PayloadHash: a.PayloadHash, ObservedWorkspaceRevision: 2, StartedAt: a.StartedAt, CompletedAt: a.CompletedAt, Internal: dir("/internal"), Deliverable: dir("/deliverable")}
	d.ContentHash = d.Hash()
	raw := []byte{0xff, 0, '\r', '\n'}
	r := WorkspacePreparationResourceEvidence{Version: PreparationResourceEvidenceVersion, WorkspaceID: "ws", RequestID: a.RequestID, PayloadHash: a.PayloadHash, ObservedWorkspaceRevision: 2, SpecificationHash: a.Binding.AuthoringRef.SpecificationHash, DeliverableDirectory: "/deliverable", StartedAt: a.StartedAt, CompletedAt: a.CompletedAt, Resources: []WorkspacePreparationResourceObservation{{ID: "input", Required: true, ExpectedContentHash: "sha256:" + fileobservation.Digest(raw), Observation: fileobservation.Resource{Ref: "input.txt", Complete: true, Inode: 1, Mode: 0100600, Content: raw, ContentBytes: len(raw), ContentSHA256: fileobservation.Digest(raw), DiagnosticSHA256: fileobservation.Digest(nil)}}}}
	r.ContentHash = r.Hash()
	a.DirectoryEvidenceHash, a.ResourceEvidenceHash = d.ContentHash, r.ContentHash
	a.Checks = []WorkspacePreparationCheckResult{
		{ID: "directory_boundary", Owner: "file", Required: true, State: "blocked", Reason: "directory_observed_remaining_checks", SubjectHash: PreparationFileSubjectHash("directory_boundary", "/internal", "/deliverable", "/deliverable", "")},
		{ID: "exact_seal", Owner: "workspace", Required: true, State: "blocked", Reason: "check_pipeline_incomplete"},
		{ID: "resource/input", Owner: "file", Required: true, State: "blocked", Reason: "resource_observed_remaining_checks", SubjectHash: PreparationFileSubjectHash("resource/input", "/internal", "/deliverable", "input.txt", r.Resources[0].ExpectedContentHash)},
	}
	a.Checks = EvaluatePreparationFileChecks(*a, &d, &r, nil)
	return &Workspace{ID: "ws", Revision: 3, StartupPreparation: &record, PreparationFence: &WorkspacePreparationFence{Version: WorkspacePreparationFenceVersion, WorkspaceID: "ws", CreationEpoch: a.CreationEpoch}, PreparationDirectoryEvidence: []WorkspacePreparationDirectoryEvidence{d}, PreparationResourceEvidence: []WorkspacePreparationResourceEvidence{r}}
}

func TestPreparationManagedEvaluationRequiresOriginalCompleteDeclarations(t *testing.T) {
	for _, scenario := range []string{"exact", "empty_subset", "defaults", "path", "policy"} {
		t.Run(scenario, func(t *testing.T) {
			next := evaluationWorkspaceFixture()
			a := &next.StartupPreparation.Attempts[0]
			next.InternalDirectory = "/internal"
			next.DefinitionAuthoring = &DefinitionAuthoringRecord{TemplateSnapshot: WorkspaceTemplate{ContextHints: payload.New(payload.Field{Name: "managedWorkspaceConfigs", Value: payload.Value{Kind: payload.ValueArray, Array: []payload.Value{{Kind: payload.ValueObject, Object: []payload.Field{payload.StringField("path", "config.json"), payload.ObjectField("content", payload.New(payload.StringField("fixture", "original")))}}}}})}}
			raw := []byte(`{"fixture":"original"}`)
			e := WorkspacePreparationManagedConfigEvidence{Version: PreparationManagedConfigEvidenceVersion, Ref: PreparationManagedConfigRefPrefix + strings.Repeat("b", 32), WorkspaceID: next.ID, RequestID: a.RequestID, PayloadHash: a.PayloadHash, ObservedWorkspaceRevision: 2, ConfigurationHash: a.Binding.AuthoringConfigurationHash, InternalDirectory: "/internal", StartedAt: a.StartedAt, CompletedAt: a.CompletedAt, Files: []WorkspacePreparationManagedConfigObservation{{Scope: "system", Path: "config.json", Policy: "system_exact", DefaultsJSON: append([]byte(nil), raw...), Observation: fileobservation.Resource{Ref: "config.json", Complete: true, Inode: 1, Mode: 0100600, Content: raw, ContentBytes: len(raw), ContentSHA256: fileobservation.Digest(raw), DiagnosticSHA256: fileobservation.Digest(nil)}}}}
			a.Checks = append(a.Checks[:2], append([]WorkspacePreparationCheckResult{{ID: "managed_configuration", Owner: "file", Required: true, State: "blocked", Reason: "managed_configs_observed_remaining_checks", SubjectHash: PreparationFileSubjectHash("managed_configuration", "/internal", "/deliverable", "/internal", "")}}, a.Checks[2:]...)...)
			a.ManagedConfigEvidenceRef = e.Ref
			before := *next
			record := next.StartupPreparation.Clone()
			before.StartupPreparation = &record
			before.Revision = 2
			original := &record.Attempts[0]
			original.State, original.CompletedAt, original.CommittedWorkspaceRevision = WorkspacePreparationAccepted, time.Time{}, 2
			original.DirectoryEvidenceHash, original.ResourceEvidenceHash, original.ManagedConfigEvidenceRef = "", "", ""
			for i := range original.Checks {
				original.Checks[i].State, original.Checks[i].Reason = "pending", "not_observed"
			}
			before.PreparationDirectoryEvidence, before.PreparationResourceEvidence = nil, nil
			switch scenario {
			case "empty_subset":
				e.Files = nil
			case "defaults":
				e.Files[0].DefaultsJSON = []byte(`{"fixture":"substituted"}`)
			case "path":
				e.Files[0].Path = "other.json"
				e.Files[0].Observation.Ref = "other.json"
			case "policy":
				e.Files[0].Scope, e.Files[0].Policy = "workspace", "workspace_preserve"
			}
			e.ContentHash = e.Hash()
			next.PreparationManagedConfigEvidence = []WorkspacePreparationManagedConfigEvidence{e}
			a.Checks = EvaluatePreparationFileChecks(*a, &next.PreparationDirectoryEvidence[0], &next.PreparationResourceEvidence[0], &e)
			if err := ValidatePreparationEvidencePersistence(next); err != nil {
				t.Fatal("test must reach independent declaration gate", err)
			}
			err := validatePreparationManagedConfigEvidenceTransition(&before, next, true)
			if (err == nil) != (scenario == "exact") {
				t.Fatal("original declaration set not enforced", scenario, err)
			}
		})
	}
}

func TestPreparationCheckEvaluationExactRawAndNoAggregateSuccess(t *testing.T) {
	w := evaluationWorkspaceFixture()
	a := w.StartupPreparation.Attempts[0]
	if w.StartupPreparation.Validate(w.ID) != nil || ValidatePreparationEvidencePersistence(w) != nil || a.State != WorkspacePreparationBlocked || a.Checks[0].State != "passed" || a.Checks[1].State != "blocked" || a.Checks[2].State != "passed" {
		t.Fatal("exact original judgments not reproduced")
	}
	before := w.PreparationResourceEvidence[0].Clone()
	checks := EvaluatePreparationFileChecks(a, &w.PreparationDirectoryEvidence[0], &w.PreparationResourceEvidence[0], nil)
	checks[0].Reason = "caller"
	if a.Checks[0].Reason == "caller" || !reflect.DeepEqual(before, w.PreparationResourceEvidence[0]) {
		t.Fatal("pure evaluator aliases or changes original raw")
	}
	for _, mutate := range []func(*Workspace){
		func(w *Workspace) { w.StartupPreparation.Attempts[0].EvaluationVersion = "" },
		func(w *Workspace) { w.StartupPreparation.Attempts[0].EvaluationVersion = "future" },
		func(w *Workspace) { w.StartupPreparation.Attempts[0].Checks[0].Owner = "caller" },
		func(w *Workspace) { w.StartupPreparation.Attempts[0].Checks[0].Required = false },
		func(w *Workspace) { w.StartupPreparation.Attempts[0].Checks[0].SubjectHash = strings.Repeat("a", 64) },
		func(w *Workspace) { w.StartupPreparation.Attempts[0].Checks[1].State = "passed" },
		func(w *Workspace) {
			w.StartupPreparation.Attempts[0].SealEvidenceRef = PreparationSealRefPrefix + strings.Repeat("b", 32)
		},
		func(w *Workspace) {
			a := &w.StartupPreparation.Attempts[0]
			a.SealEvidenceRef = PreparationSealRefPrefix + strings.Repeat("b", 32)
			a.Checks[1].State, a.Checks[1].Reason = "passed", "seal_observed_remaining_checks"
			e := WorkspacePreparationSealEvidence{Version: PreparationSealEvidenceVersion, Ref: a.SealEvidenceRef, RequestID: a.RequestID, PayloadHash: a.PayloadHash, Binding: a.Binding}
			e.ContentHash = e.Hash()
			w.PreparationSealEvidence = []WorkspacePreparationSealEvidence{e}
		},
		func(w *Workspace) { w.StartupPreparation.Attempts[0].Checks[2].State = "not_applicable" },
		func(w *Workspace) { w.StartupPreparation.Attempts[0].Checks[2].State = "failed" },
		func(w *Workspace) { w.StartupPreparation.Attempts[0].State = "ready" },
		func(w *Workspace) { w.PreparationDirectoryEvidence = nil },
		func(w *Workspace) { w.PreparationResourceEvidence = nil },
		func(w *Workspace) {
			e := &w.PreparationDirectoryEvidence[0]
			e.Deliverable.Path = "/sibling"
			e.ContentHash = e.Hash()
			w.StartupPreparation.Attempts[0].DirectoryEvidenceHash = e.ContentHash
		},
		func(w *Workspace) {
			e := &w.PreparationResourceEvidence[0]
			e.Resources[0].Observation.Ref = "sibling.txt"
			e.ContentHash = e.Hash()
			w.StartupPreparation.Attempts[0].ResourceEvidenceHash = e.ContentHash
		},
		func(w *Workspace) {
			e := &w.PreparationResourceEvidence[0]
			r := &e.Resources[0].Observation
			r.Content = []byte("changed")
			r.ContentBytes = len(r.Content)
			r.ContentSHA256 = fileobservation.Digest(r.Content)
			e.ContentHash = e.Hash()
			w.StartupPreparation.Attempts[0].ResourceEvidenceHash = e.ContentHash
		},
	} {
		bad := evaluationWorkspaceFixture()
		mutate(bad)
		if bad.StartupPreparation.Validate(bad.ID) == nil && ValidatePreparationEvidencePersistence(bad) == nil {
			t.Fatal("unproved file judgment accepted")
		}
	}
}

func TestPreparationCheckEvaluationFailureStaleAndLegacy(t *testing.T) {
	w := evaluationWorkspaceFixture()
	a := &w.StartupPreparation.Attempts[0]
	e := &w.PreparationResourceEvidence[0]
	e.Resources[0].Observation.Content[0] = 'x'
	e.Resources[0].Observation.ContentSHA256 = fileobservation.Digest(e.Resources[0].Observation.Content)
	e.ContentHash = e.Hash()
	a.ResourceEvidenceHash = e.ContentHash
	a.Checks = EvaluatePreparationFileChecks(*a, &w.PreparationDirectoryEvidence[0], e, nil)
	if a.Checks[2].State != "failed" || a.Checks[2].Reason != "resource_content_changed" || ValidatePreparationEvidencePersistence(w) != nil {
		t.Fatal("known failure hidden as unknown or passed")
	}
	for i := range a.Checks {
		a.Checks[i].State, a.Checks[i].Reason = "blocked", "binding_changed"
	}
	if ValidatePreparationEvidencePersistence(w) != nil {
		t.Fatal("stale raw history not retained")
	}
	for _, c := range EvaluatePreparationFileChecks(*a, &w.PreparationDirectoryEvidence[0], e, nil) {
		if c.State != "blocked" {
			t.Fatal("stale history granted credit")
		}
	}
	a.EvaluationVersion = ""
	a.ObservationSnapshotHash = ""
	for i := range a.Checks {
		a.Checks[i].SubjectHash = ""
	}
	if w.StartupPreparation.Validate(w.ID) != nil || ValidatePreparationEvidencePersistence(w) != nil {
		t.Fatal("legacy blocked history rejected")
	}
	for _, c := range EvaluatePreparationFileChecks(*a, &w.PreparationDirectoryEvidence[0], e, nil) {
		if c.State != "blocked" {
			t.Fatal("legacy history upgraded")
		}
	}
}

func TestPreparationCheckEvaluationSoulRequiresExactAcceptedSnapshotAndFullDoubleRead(t *testing.T) {
	fixture := func() *Workspace {
		w := evaluationWorkspaceFixture()
		a := &w.StartupPreparation.Attempts[0]
		e := preparationSoulEvidenceFixture()
		e.RequestID, e.PayloadHash = a.RequestID, a.PayloadHash
		e.Facts.StartedAt, e.Facts.CompletedAt = a.StartedAt, a.CompletedAt
		e.Facts.SnapshotHash, e.Facts.ReobservedSnapshotHash = a.ObservationSnapshotHash, a.ObservationSnapshotHash
		e.ContentHash = e.Hash()
		a.SoulEvidenceRef = e.Ref
		a.Checks = append(a.Checks, WorkspacePreparationCheckResult{ID: "soul_selection", Owner: "soul", Required: true, State: "blocked", Reason: "souls_observed_remaining_checks"})
		w.PreparationSoulEvidence = []WorkspacePreparationSoulEvidence{e}
		a.Checks = EvaluatePreparationChecks(*a, &w.PreparationDirectoryEvidence[0], &w.PreparationResourceEvidence[0], nil, &e)
		return w
	}
	w := fixture()
	if w.StartupPreparation.Attempts[0].Checks[3].State != "passed" || w.StartupPreparation.Validate(w.ID) != nil || ValidatePreparationEvidencePersistence(w) != nil {
		t.Fatal("full original Soul read not consumed")
	}
	for _, allUnbound := range []bool{false, true} {
		unbound := fixture()
		e := &unbound.PreparationSoulEvidence[0]
		e.Facts.WorkspaceDefaultSoulID, e.Facts.SystemDefaultSoulID, e.Facts.ReobservedSystemDefaultSoulID = "", "", ""
		e.Facts.WorkspaceBindings = nil
		if !allUnbound {
			for _, selection := range e.Facts.Selections[:5] {
				e.Facts.WorkspaceBindings = append(e.Facts.WorkspaceBindings, soulbinding.Binding{Stage: selection.Stage, SoulID: selection.SoulID})
			}
		} else {
			e.Facts.Souls, e.Facts.ReobservedSouls = nil, nil
		}
		e.Facts.Selections, e.Facts.DefaultLookupSoulID, _ = PreparationSoulPlan("", e.Facts.WorkspaceBindings, "")
		e.ContentHash = e.Hash()
		if e.Validate() != nil {
			t.Fatal("unbound fixture must remain a complete valid observation")
		}
		a := &unbound.StartupPreparation.Attempts[0]
		a.Checks = EvaluatePreparationChecks(*a, &unbound.PreparationDirectoryEvidence[0], &unbound.PreparationResourceEvidence[0], nil, e)
		if a.Checks[3].State != "blocked" || ValidatePreparationEvidencePersistence(unbound) != nil {
			t.Fatal("unbound stage promoted or original complete facts rejected")
		}
	}
	for _, mutate := range []func(*Workspace){
		func(w *Workspace) { w.PreparationSoulEvidence = nil },
		func(w *Workspace) {
			w.StartupPreparation.Attempts[0].ObservationSnapshotHash = "original-pre-acceptance-snapshot"
		},
		func(w *Workspace) {
			e := &w.PreparationSoulEvidence[0]
			e.Facts.SnapshotHash, e.Facts.ReobservedSnapshotHash = "other", "other"
			e.ContentHash = e.Hash()
		},
		func(w *Workspace) {
			e := &w.PreparationSoulEvidence[0]
			e.Facts.ReobservedSouls = nil
			e.ContentHash = e.Hash()
		},
		func(w *Workspace) {
			e := &w.PreparationSoulEvidence[0]
			e.Facts.Selections = e.Facts.Selections[:5]
			e.ContentHash = e.Hash()
		},
		func(w *Workspace) { w.StartupPreparation.Attempts[0].Checks[3].Owner = "file" },
	} {
		bad := fixture()
		mutate(bad)
		if bad.StartupPreparation.Validate(bad.ID) == nil && ValidatePreparationEvidencePersistence(bad) == nil {
			t.Fatal("incomplete/unbound Soul evidence granted PASS")
		}
	}
}
