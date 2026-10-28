package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	skillmodel "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/models"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/startuppreparation"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
	"github.com/muidea/workorch/internal/pkg/skillhubpreparation"
)

func TestPreparationSkillHubReservationRecoveryFencesNewIntent(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	owners := openFileDefinitionAdmissionOwners(t, root)
	defer func() { owners.close() }()
	w := seedPreparationEligible(t, owners, "ws-status-fence")
	c := workspaceevents.CheckWorkspacePreparationCommand{WorkspaceID: w.ID, RequestID: "status-original", ExpectedWorkspaceRevision: w.Revision}
	read, err := owners.workspace.queryExactConfirmedStartupSnapshot(ctx, workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: w.ID, ExpectedWorkspaceRevision: w.Revision})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := startuppreparation.CompilePreparationManifest(read.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	fact := workspacePreparationAccepted{Command: c, Snapshot: read.Snapshot, Manifest: manifest, At: time.Now().UTC(), SkillHubRef: workspace.PreparationSkillHubRefPrefix + strings.Repeat("a", 32)}
	plain := fact
	plain.SkillHubRef = ""
	base, baseErr := reduceWorkspacePreparationAccepted(w, plain)
	if baseErr != nil {
		t.Fatal("plain reducer", baseErr)
	}
	a := base.Next.StartupPreparation.Attempts[0]
	e := workspace.WorkspacePreparationSkillHubEffects{Version: workspace.PreparationSkillHubEffectsVersion, Ref: fact.SkillHubRef, WorkspaceID: w.ID, RequestID: c.RequestID, PayloadHash: c.PayloadHash(), ExpectedWorkspaceRevision: w.Revision, Binding: a.Binding, ProjectDirectory: read.Snapshot.StartupInput.ResolvedProjectWorkspaceDirectory, ReservedAt: fact.At}
	e.ContentHash = e.Hash()
	if e.Validate() != nil {
		t.Fatalf("plan invalid: %+v binding=%v", e, a.Binding.Validate())
	}
	a.SkillHubEffectRef = e.Ref
	a.SkillHubEffectState = workspace.PreparationSkillHubReserved
	if a.Validate() != nil {
		t.Fatalf("attempt invalid %+v", a)
	}
	base.Next.StartupPreparation.Attempts[0] = a
	base.Next.PreparationSkillHubEffects = []workspace.WorkspacePreparationSkillHubEffects{e}
	if workspace.ValidatePreparationEvidencePersistence(base.Next) != nil {
		t.Fatal("effects persistence invalid", workspace.ValidatePreparationSkillHubEffectsPersistence(base.Next))
	}
	reduced, err := reduceWorkspacePreparationAccepted(w, fact)
	if err != nil {
		t.Fatal("reserve reducer", err)
	}
	if err := workspace.ValidatePreparationRecordTransition(w, reduced.Next, true); err != nil {
		t.Fatal("reservation transition", err)
	}
	first, err := owners.workspace.beginWorkspacePreparation(ctx, c, false, true)
	if err != nil || !first.ObservationRequired || first.SkillHubPlan == nil {
		t.Fatal("first continuation", err)
	}
	again, err := owners.workspace.beginWorkspacePreparation(ctx, c, false, true)
	if err != nil || again.ObservationRequired || again.SkillHubPlan != nil {
		t.Fatal("duplicate redispatched", err)
	}
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
	recovered, err := owners.workspace.beginWorkspacePreparation(ctx, c, true, true)
	if err != nil || recovered.State.Record.Attempts[0].SkillHubEffectState != workspace.PreparationSkillHubUnresolved {
		t.Fatal("recovery concealed unknown effects", err)
	}
	next := c
	next.RequestID = "bypass"
	next.ExpectedWorkspaceRevision = recovered.State.WorkspaceRevision
	if _, err := owners.workspace.beginWorkspacePreparation(ctx, next, false, true); err == nil {
		t.Fatal("new intent bypassed unresolved effect")
	}
	stored, err := owners.repositories.Workspaces.Get(ctx, w.ID)
	if err != nil || len(stored.PreparationSkillHubEffects) != 1 || stored.PreparationSkillHubEffects[0].Global != nil || stored.ActiveRunID != "" {
		t.Fatal("recovery fabricated command receipt/Run", err)
	}
}

func TestPreparationUnregisteredProjectDiagnosticSurvivesReopenWithoutEffects(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	owners := openFileDefinitionAdmissionOwners(t, root)
	defer func() { owners.close() }()
	w := seedPreparationEligibleInDirectory(t, owners, "ws-project-unregistered", t.TempDir())
	c := workspaceevents.CheckWorkspacePreparationCommand{WorkspaceID: w.ID, RequestID: "unregistered-original", ExpectedWorkspaceRevision: w.Revision}
	first, err := owners.workspace.beginWorkspacePreparation(ctx, c, false, true)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	gRaw := []byte(`{"scope":"global","items":[]}`)
	g := workspace.PreparationSkillHubStatusFacts{Version: "workorch-skillhub-preparation-observation/v2", EffectContract: "skillhub-status-effects/v1", Scope: "global", StartedAt: now, CompletedAt: now, RawOutput: gRaw, TotalOutputBytes: uint64(len(gRaw)), RawSHA256: fileobservation.Digest(gRaw), StderrSHA256: fileobservation.Digest(nil), OutputComplete: true, StderrComplete: true, CommandStarted: true, ExitCodeKnown: true}
	p := g.Clone()
	p.Scope, p.Directory = "workspace", first.SkillHubPlan.ProjectDirectory
	p.RawOutput = []byte(`{"command_error":{"code":"PROJECT_NOT_FOUND","message":"private original path"}}`)
	p.RawSHA256, p.TotalOutputBytes = fileobservation.Digest(p.RawOutput), uint64(len(p.RawOutput))
	p.FailureCode, p.ExitCode = "project_not_registered", 1
	_, err = owners.workspace.finishBlockedPreparation(ctx, workspacePreparationBlocked{Command: c, Reason: "check_pipeline_incomplete", SkillHubEffectRef: first.SkillHubPlan.Ref, SkillHubGlobal: &g, SkillHubProject: &p, At: now})
	if err != nil {
		t.Fatal(err)
	}
	before, err := owners.repositories.Workspaces.Get(ctx, w.ID)
	if err != nil {
		t.Fatal(err)
	}
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
	for range 2 {
		view, err := owners.workspace.queryWorkspacePreparationState(ctx, workspaceevents.QueryWorkspacePreparationStateCommand{WorkspaceID: w.ID})
		if err != nil || view.SkillHubProjectIssue != "project_registration_required" || view.Record.Attempts[0].SkillHubEffectState != workspace.PreparationSkillHubUnresolved {
			t.Fatal("reopen/GET lost exact negative diagnostic", err)
		}
		raw, _ := json.Marshal(view)
		if bytes.Contains(raw, p.RawOutput) || bytes.Contains(raw, []byte("private original path")) {
			t.Fatal("query leaked raw command diagnostic")
		}
	}
	if _, err := owners.workspace.beginWorkspacePreparation(ctx, c, true, true); err != nil {
		t.Fatal(err)
	}
	newIntent := c
	newIntent.RequestID, newIntent.ExpectedWorkspaceRevision = "bypass", before.Revision
	if _, err := owners.workspace.beginWorkspacePreparation(ctx, newIntent, false, true); err == nil {
		t.Fatal("diagnostic cleared original unresolved fence")
	}
	after, err := owners.repositories.Workspaces.Get(ctx, w.ID)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("read/recover rewrote original owner state", err)
	}
}

func TestPreparationSkillHubReceiptSettlementImmutableAndCutRecovery(t *testing.T) {
	for _, scenario := range []string{"recorded", "known_failure", "write_failed", "reply_lost", "installed", "installed_write_failed", "installed_reply_lost", "installed_context_bounded", "installed_context_bounded_write_failed", "installed_context_bounded_reply_lost", "installed_context_rejected", "installed_context_rejected_write_failed", "installed_context_rejected_reply_lost"} {
		t.Run(scenario, func(t *testing.T) {
			installedScenario := strings.HasPrefix(scenario, "installed")
			writeFailed := strings.HasSuffix(scenario, "write_failed")
			replyLost := strings.HasSuffix(scenario, "reply_lost")
			ctx := context.Background()
			root := t.TempDir()
			owners := openFileDefinitionAdmissionOwners(t, root)
			defer func() { owners.close() }()
			w := seedPreparationEligibleInDirectory(t, owners, "ws-status-receipt", t.TempDir())
			c := workspaceevents.CheckWorkspacePreparationCommand{WorkspaceID: w.ID, RequestID: "original-status", ExpectedWorkspaceRevision: w.Revision}
			first, err := owners.workspace.beginWorkspacePreparation(ctx, c, false, true)
			if err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC()
			raw := []byte("stdout 原文\xff\n")
			stderr := []byte("stderr 原文\r\n")
			f := workspace.PreparationSkillHubStatusFacts{Version: "workorch-skillhub-preparation-observation/v2", EffectContract: "skillhub-status-effects/v1", Scope: "global", StartedAt: now, CompletedAt: now, RawOutput: raw, RawStderr: stderr, TotalOutputBytes: uint64(len(raw)), TotalStderrBytes: uint64(len(stderr)), RawSHA256: fileobservation.Digest(raw), StderrSHA256: fileobservation.Digest(stderr), OutputComplete: true, StderrComplete: true, CommandStarted: true, ExitCodeKnown: true}
			p := f.Clone()
			p.Scope = "workspace"
			p.Directory = first.SkillHubPlan.ProjectDirectory
			installedRaw := []byte("---\r\nname: owner 原文\r\n---\r\n独立指令\x00\r\n")
			contextWant := "compiled_checks_pending"
			if strings.HasPrefix(scenario, "installed_context_") {
				trigger, _ := json.Marshal(w.DefinitionAuthoringConfirmation.Goal.Text)
				body, metadata := strings.Repeat("界", 4000), ""
				contextWant = "bounded_checks_pending"
				if strings.HasPrefix(scenario, "installed_context_rejected") {
					body, contextWant = "original body", "rejected"
					// Each entry satisfies the installed metadata contract; their
					// combined context cost exceeds the independent 16KiB budget.
					metadata = "constraints:\r\n"
					for i := range 10 {
						metadata += fmt.Sprintf("  - %d%s\r\n", i, strings.Repeat("m", 1800))
					}
				}
				installedRaw = []byte("---\r\nname: owner 原文\r\ntriggers: [" + string(trigger) + "]\r\n" + metadata + "---\r\n" + body + "\x00\r\n")
			}
			if installedScenario {
				for i, facts := range []*workspace.PreparationSkillHubStatusFacts{&f, &p} {
					root := filepath.Join(p.Directory, ".agents", "skills")
					status := skillhubpreparation.Status{Scope: "project", ProjectPath: p.Directory, Items: []skillhubpreparation.StatusItem{{SkillID: "s", LocalPath: filepath.Join(root, "s", "SKILL.md")}}}
					if i == 0 {
						root = filepath.Join(t.TempDir(), "codex")
						status = skillhubpreparation.Status{Scope: "global", Items: []skillhubpreparation.StatusItem{{SkillID: "s", Agent: "codex", TargetPath: filepath.Join(root, "s")}}}
					}
					facts.RawOutput, _ = json.Marshal(status)
					facts.TotalOutputBytes = uint64(len(facts.RawOutput))
					facts.RawSHA256 = fileobservation.Digest(facts.RawOutput)
					file := skillhubpreparation.Instruction{SkillID: "s", Path: filepath.Join(root, "s", "SKILL.md"), Raw: installedRaw, RawSHA256: fileobservation.Digest(installedRaw), ReadBytes: uint64(len(installedRaw)), ObservedSize: int64(len(installedRaw)), Complete: true}
					facts.Installed = &skillhubpreparation.Installed{Version: skillhubpreparation.InstalledVersion, Root: root, First: []skillhubpreparation.Instruction{file}, Second: []skillhubpreparation.Instruction{file}, StartedAt: now, CompletedAt: now}
				}
			}
			if scenario == "known_failure" {
				f.ExitCode = 9
				f.FailureCode = "command_failed"
			}
			fact := workspacePreparationBlocked{Command: c, Reason: "check_pipeline_incomplete", SkillHubEffectRef: first.SkillHubPlan.Ref, SkillHubGlobal: &f, SkillHubProject: &p, At: time.Now().UTC()}
			if installedScenario {
				// Reducer/CAS cut fixture: complete unbound Soul plan and a real
				// empty Registry value, not a fabricated resource readiness flag.
				read, err := owners.workspace.queryExactConfirmedStartupSnapshot(ctx, workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: w.ID, ExpectedWorkspaceRevision: w.Revision + 1})
				if err != nil {
					t.Fatal(err)
				}
				plan, lookup, err := workspace.PreparationSoulPlan("", nil, "")
				if err != nil || w.DefaultSoulID != "" || len(w.SoulBindings) != 0 {
					t.Fatal("unexpected cut fixture Soul bindings", err)
				}
				s := workspace.WorkspacePreparationSoulEvidence{Version: workspace.PreparationSoulEvidenceVersion, Ref: workspace.PreparationSoulRefPrefix + strings.Repeat("c", 32), RequestID: c.RequestID, PayloadHash: c.PayloadHash(), Facts: workspace.PreparationSoulFacts{Version: "workorch-preparation-souls/v1", WorkspaceID: w.ID, WorkspaceRevision: w.Revision + 1, SnapshotHash: read.Snapshot.SnapshotHash, ReobservedSnapshotHash: read.Snapshot.SnapshotHash, ConfigurationHash: read.Snapshot.AuthoringConfigurationHash, DefaultLookupSoulID: lookup, Selections: plan, StartedAt: now, CompletedAt: now}}
				s.ContentHash = s.Hash()
				catalog := skillmodel.PreparationRegistryCatalog{Version: skillmodel.RegistryCatalogVersion, WorkspaceID: w.ID, OwnerEpoch: "skill-preparation-epoch:" + strings.Repeat("A", 26), ObservedAt: now, Skills: []*skillmodel.Skill{}}
				catalog.CatalogHash, _ = skillmodel.RegistryCatalogHash(catalog.Skills)
				r := workspace.WorkspacePreparationSkillRegistryEvidence{Version: workspace.PreparationSkillRegistryEvidenceVersion, Ref: workspace.PreparationSkillRegistryRefPrefix + strings.Repeat("d", 32), RequestID: c.RequestID, PayloadHash: c.PayloadHash(), Facts: workspace.PreparationSkillCatalogFacts{Version: "workorch-preparation-skill-registry-facts/v1", WorkspaceID: w.ID, WorkspaceRevision: w.Revision + 1, SnapshotHash: read.Snapshot.SnapshotHash, ReobservedSnapshotHash: read.Snapshot.SnapshotHash, Catalog: catalog, ReobservedCatalog: catalog.Clone(), StartedAt: now, CompletedAt: now}}
				r.ContentHash = r.Hash()
				fact.SoulEvidence, fact.SoulReason = &s, "souls_observed_remaining_checks"
				fact.SkillRegistryEvidence, fact.SkillRegistryReason = &r, "skill_registry_observed_selection_pending"
				fact.At = time.Now().UTC()
				preimage, err := owners.repositories.Workspaces.Get(ctx, w.ID)
				if err != nil {
					t.Fatal(err)
				}
				transition, err := reduceWorkspacePreparationBlocked(preimage, fact)
				if err != nil || workspace.ValidatePreparationRecordTransition(preimage, transition.Next, true) != nil {
					t.Fatal("valid combined settlement rejected", err)
				}
				// Even an internally consistent alternate Goal, recomputed
				// selection and envelope hash cannot replace the confirmed input
				// in the first append (not just in later immutable history).
				bad := transition.Next
				archive := &bad.PreparationSkillRegistryEvidence[0]
				definition := archive.Selection.Definition
				definition.GoalText = "unconfirmed alternate goal"
				definition.GoalTextHash = workspace.StableDefinitionTextHash(definition.GoalText)
				compiled := workspace.CompilePreparationSkillSelection(definition, *archive, &bad.PreparationSoulEvidence[0], &bad.PreparationSkillHubEffects[0])
				context := workspace.CompilePreparationSkillContext(compiled.Selected)
				compiled.Context = &context
				bad.StartupPreparation.Attempts[0].SkillContextState = context.Summary()
				references := workspace.CompilePreparationSkillReferences(compiled, &bad.PreparationSkillHubEffects[0])
				compiled.References = &references
				bad.StartupPreparation.Attempts[0].SkillReferenceState = references.State
				status := workspace.CompilePreparationSkillStatus(compiled, &bad.PreparationSkillHubEffects[0])
				compiled.Status = &status
				bad.StartupPreparation.Attempts[0].SkillStatusState = status.State
				archive.Selection, archive.ContentHash = &compiled, ""
				archive.ContentHash = archive.Hash()
				if workspace.ValidatePreparationEvidencePersistence(bad) != nil {
					t.Fatalf("alternate Goal test is not internally consistent: selection=%s context=%s selected=%d envelope=%v record=%v effects=%v", compiled.FailureCode, context.Summary(), len(compiled.Selected), archive.Validate(), bad.StartupPreparation.Validate(bad.ID), workspace.ValidatePreparationSkillHubEffectsPersistence(bad))
				}
				if workspace.ValidatePreparationRecordTransition(preimage, bad, true) == nil {
					t.Fatal("self-consistent alternate Goal bypassed owner preimage")
				}
			}
			if writeFailed || replyLost {
				owners.workspace.workspaces = &preparationWriteCutRepository{WorkspaceRepository: owners.workspace.workspaces, failAt: 1, persistFirst: replyLost}
			}
			result, err := owners.workspace.finishBlockedPreparation(ctx, fact)
			if writeFailed || replyLost {
				if err == nil {
					t.Fatal("ambiguous write reported success")
				}
			} else if err != nil {
				t.Fatal(err)
			}
			owners.close()
			owners = openFileDefinitionAdmissionOwners(t, root)
			result, err = owners.workspace.beginWorkspacePreparation(ctx, c, true, true)
			if err != nil {
				t.Fatal(err)
			}
			stored, err := owners.repositories.Workspaces.Get(ctx, w.ID)
			if err != nil {
				t.Fatal(err)
			}
			e := stored.PreparationSkillHubEffects[0]
			info, statErr := os.Stat(filepath.Join(root, "internal", "workspaces", string(w.ID), "workspace.json"))
			if statErr != nil || info.Mode().Perm() != 0600 {
				t.Fatal("private effects not protected with 0600", statErr)
			}
			want := workspace.PreparationSkillHubRecorded
			if writeFailed || scenario == "known_failure" {
				want = workspace.PreparationSkillHubUnresolved
			}
			if e.Summary() != want || result.State.Record.Attempts[0].SkillHubEffectState != want {
				t.Fatal("wrong effect classification")
			}
			if installedScenario && !writeFailed {
				if result.State.Record.Attempts[0].SkillHubInstalledState != "observed_selection_pending" || e.Global.Installed == nil || !bytes.Equal(e.Global.Installed.Second[0].Raw, installedRaw) {
					t.Fatal("owner reopen lost installed material")
				}
				if result.State.Record.Attempts[0].SkillContextState != contextWant || result.State.Record.Attempts[0].SkillSelectionState != "compiled_checks_pending" || len(stored.PreparationSkillRegistryEvidence) != 1 || stored.PreparationSkillRegistryEvidence[0].Selection == nil || stored.PreparationSkillRegistryEvidence[0].Selection.Context == nil || len(stored.PreparationSkillRegistryEvidence[0].Selection.Catalog) != 2 || workspace.ValidatePreparationEvidencePersistence(stored) != nil {
					t.Fatal("owner cut/reopen lost exact historical compilation")
				}
				if contextWant != "compiled_checks_pending" {
					s := stored.PreparationSkillRegistryEvidence[0].Selection
					if len(s.Selected) != 2 || s.Selected[0].Content.Instruction != string(installedRaw) || contextWant == "bounded_checks_pending" && (s.Context.Budget == nil || s.Context.Budget.TruncatedSkills != 2) || contextWant == "rejected" && (len(s.Context.Diagnostic) == 0 || len(s.Context.Projection) != 0) {
						t.Fatal("bounded/rejected projection replaced source or lost diagnosis")
					}
				}
				for _, mutate := range []func(*workspace.Workspace){
					func(v *workspace.Workspace) {
						v.PreparationSkillHubEffects[0].Global.Installed = nil
						v.StartupPreparation.Attempts[0].SkillHubInstalledState = "incomplete"
					},
					func(v *workspace.Workspace) {
						installed := v.PreparationSkillHubEffects[0].Global.Installed
						for _, files := range [][]skillhubpreparation.Instruction{installed.First, installed.Second} {
							files[0].Raw[len(files[0].Raw)-3] = 'X'
							files[0].RawSHA256 = fileobservation.Digest(files[0].Raw)
						}
					},
				} {
					bad, _ := owners.repositories.Workspaces.Get(ctx, w.ID)
					mutate(bad)
					bad.PreparationSkillHubEffects[0].ContentHash = bad.PreparationSkillHubEffects[0].Hash()
					if workspace.ValidatePreparationSkillHubEffectsPersistence(bad) != nil {
						t.Fatal("test did not construct internally consistent replacement")
					}
					if err := owners.repositories.Workspaces.CompareAndSwap(ctx, bad, bad.Revision); err == nil {
						t.Fatal("completed installed history replaced after rehash")
					}
				}
				copy := e.Clone()
				copy.Global.Installed.Second[0].Raw[0] = 'X'
				fresh, _ := owners.repositories.Workspaces.Get(ctx, w.ID)
				if !bytes.Equal(fresh.PreparationSkillHubEffects[0].Global.Installed.Second[0].Raw, installedRaw) {
					t.Fatal("installed clone aliases repository")
				}
			}
			if writeFailed {
				if e.Global != nil || e.Project != nil {
					t.Fatal("lost receipt fabricated")
				}
				if result.State.Record.Attempts[0].SkillHubInstalledState != "" {
					t.Fatal("missing material manufactured on recovery")
				}
				if result.State.Record.Attempts[0].SkillContextState != "" || result.State.Record.Attempts[0].SkillSelectionState != "" || len(stored.PreparationSkillRegistryEvidence) != 0 {
					t.Fatal("recovery compiled a lost selection")
				}
			} else if !bytes.Equal(e.Global.RawOutput, f.RawOutput) || !bytes.Equal(e.Global.RawStderr, stderr) || (scenario == "known_failure" && e.Global.ExitCode != 9) {
				t.Fatal("original capture lost")
			}
			public, _ := json.Marshal(stored)
			if bytes.Contains(public, raw) || bytes.Contains(public, []byte("PreparationSkillHubEffects")) {
				t.Fatal("private receipt leaked")
			}
			before := e.Clone()
			if e.Global != nil {
				e.Global.RawOutput[0] = '!'
				fresh, _ := owners.repositories.Workspaces.Get(ctx, w.ID)
				if !reflect.DeepEqual(fresh.PreparationSkillHubEffects[0], before) {
					t.Fatal("repository clone aliases bytes")
				}
			}
			for _, mutate := range []func(*workspace.Workspace){
				func(w *workspace.Workspace) { w.PreparationSkillHubEffects = nil },
				func(w *workspace.Workspace) {
					e := &w.PreparationSkillHubEffects[0]
					e.ProjectDirectory = "another"
					if e.Project != nil {
						e.Project.Directory = "another"
					}
					e.ContentHash = e.Hash()
				},
			} {
				bad, _ := owners.repositories.Workspaces.Get(ctx, w.ID)
				mutate(bad)
				if err := owners.repositories.Workspaces.CompareAndSwap(ctx, bad, bad.Revision); err == nil {
					t.Fatal("completed history rewritten")
				}
			}
			if want == workspace.PreparationSkillHubUnresolved {
				next := c
				next.RequestID = "bypass"
				next.ExpectedWorkspaceRevision = stored.Revision
				if _, err := owners.workspace.beginWorkspacePreparation(ctx, next, false, true); err == nil || !errors.Is(err, common.ErrConflict) {
					t.Fatal("unresolved effect bypassed", err)
				}
			}
		})
	}
}
