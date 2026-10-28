package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	applicationbiz "github.com/muidea/workorch/internal/modules/application/workorchd/biz"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
	"github.com/muidea/workorch/internal/pkg/skillhubpreparation"
	management "github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func registrationSuccessForTest(t *testing.T, d workspaceevents.PreparationRegistrationDispatch) skillhubpreparation.RegistrationObservation {
	t.Helper()
	yes := true
	now := time.Now().UTC()
	raw, err := json.Marshal(skillhubpreparation.RegistrationReceipt{RequestID: d.Plan.RequestID, ProjectPath: d.Plan.Directory, Status: "succeeded", Executed: &yes, Finished: &yes, StartedAt: now.Format(time.RFC3339Nano), CompletedAt: now.Format(time.RFC3339Nano), Result: &skillhubpreparation.RegistrationEffects{Registered: true, CreatedDirectories: []string{}}})
	if err != nil {
		t.Fatal(err)
	}
	return skillhubpreparation.RegistrationObservation{StartedAt: now, CompletedAt: now, CommandStarted: true, ExitCodeKnown: true, Stdout: raw, TotalStdoutBytes: uint64(len(raw)), StdoutSHA256: fileobservation.Digest(raw), StderrSHA256: fileobservation.Digest(nil), ErrorSHA256: fileobservation.Digest(nil), ErrorComplete: true, Complete: true}
}
func TestPreparationRegistrationReservationRecoveryIdentityAndReopen(t *testing.T) {
	for _, scenario := range []string{"success", "lost_response", "never_started", "wrong_path", "scope_drift", "known_failed"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			storage := t.TempDir()
			owners := openFileDefinitionAdmissionOwners(t, storage)
			defer func() { owners.close() }()
			w := seedManagedPreparation(t, owners, t.TempDir(), "exact")
			startManagedPreparationOwners(t, owners)
			c := workspaceevents.InitializeWorkspacePreparationCommand{WorkspaceID: w.ID, RequestID: "original-registration", ExpectedWorkspaceRevision: w.Revision, RegistrationHome: t.TempDir()}
			reserved, err := owners.workspace.initializePreparationResources(ctx, c, false)
			if err != nil || reserved.Registration == nil || reserved.Registration.Query || reserved.State.ResourceInitializations[0].State != "pending" {
				t.Fatalf("missing durable dispatch: %+v %v", reserved, err)
			}
			d := *reserved.Registration
			original, _ := owners.repositories.Workspaces.Get(ctx, w.ID)
			bad := *original
			bad.PreparationResourceInitializations = workspace.ClonePreparationInitializations(original.PreparationResourceInitializations)
			bad.PreparationResourceInitializations[0].Registration.Plan.Home = t.TempDir()
			bad.PreparationResourceInitializations[0].Plan.BindingHash = bad.PreparationResourceInitializations[0].BindingHash()
			bad.Revision++
			if err := owners.repositories.Workspaces.CompareAndSwap(ctx, &bad, original.Revision); err == nil {
				t.Fatal("self-consistent replacement of original Hub home was accepted")
			}
			if original.Revision != w.Revision+3 || len(original.PreparationResourceInitializations[0].Registration.Attempts) != 1 {
				t.Fatal("registration forwarded before durable reservation")
			}
			for _, recover := range []bool{false} {
				replay, err := owners.workspace.initializePreparationResources(ctx, c, recover)
				if err != nil || replay.Registration != nil || replay.State.WorkspaceRevision != original.Revision {
					t.Fatal("ordinary replay repeated registration", err)
				}
			}
			// Pending registration is part of the existing execution/preparation fence.
			if _, err := owners.workspace.beginWorkspacePreparation(ctx, workspaceevents.CheckWorkspacePreparationCommand{WorkspaceID: w.ID, RequestID: "cannot-bypass", ExpectedWorkspaceRevision: original.Revision}, false, true); err == nil {
				t.Fatal("pending registration bypassed")
			}
			if scenario == "scope_drift" {
				wrong := c
				wrong.RegistrationHome = t.TempDir()
				if _, err := owners.workspace.initializePreparationResources(ctx, wrong, true); err == nil {
					t.Fatal("wrong home queried original receipt")
				}
				return
			}
			observation := registrationSuccessForTest(t, d)
			if scenario == "known_failed" {
				receipt, err := skillhubpreparation.ParseRegistrationReceipt(observation.Stdout, d.Plan)
				if err != nil {
					t.Fatal(err)
				}
				receipt.Status = "failed"
				receipt.CommandError = &skillhubpreparation.RegistrationError{Code: "FILE_OPERATION", Message: "original failure"}
				observation.Stdout, _ = json.Marshal(receipt)
				observation.TotalStdoutBytes = uint64(len(observation.Stdout))
				observation.StdoutSHA256 = fileobservation.Digest(observation.Stdout)
				observation.ExitCode = 1
			}

			if scenario == "wrong_path" {
				observation.Stdout = bytes.ReplaceAll(observation.Stdout, []byte(d.Plan.Directory), []byte("/other"))
				observation.TotalStdoutBytes = uint64(len(observation.Stdout))
				observation.StdoutSHA256 = fileobservation.Digest(observation.Stdout)
			}
			if scenario == "never_started" {
				observation = skillhubpreparation.RegistrationObservation{StartedAt: time.Now().UTC(), CompletedAt: time.Now().UTC(), StdoutSHA256: fileobservation.Digest(nil), StderrSHA256: fileobservation.Digest(nil), ErrorSHA256: fileobservation.Digest(nil), ErrorComplete: true}
			}
			if scenario != "lost_response" {
				settled, err := owners.workspace.settlePreparationRegistration(ctx, workspaceevents.SettlePreparationRegistrationCommand{Original: c, Dispatch: d, Observation: observation})
				if err != nil {
					t.Fatal("settlement failed", err)
				}
				want := "completed"
				if scenario != "success" {
					want = "blocked"
				}
				if settled.State.ResourceInitializations[0].State != want {
					t.Fatal("invalid observation awarded success", settled.State.ResourceInitializations)
				}
				// Identical settlement is read-only; altered observations cannot replace it.
				replay, err := owners.workspace.settlePreparationRegistration(ctx, workspaceevents.SettlePreparationRegistrationCommand{Original: c, Dispatch: d, Observation: observation})
				if err != nil || !reflect.DeepEqual(replay, settled) {
					t.Fatal("settlement not idempotent", err)
				}
			}
			owners.close()
			owners = openFileDefinitionAdmissionOwners(t, storage)
			startManagedPreparationOwners(t, owners)
			recovered, err := owners.workspace.initializePreparationResources(ctx, c, true)
			if err != nil {
				t.Fatal("reopen recovery failed", err)
			}
			if scenario == "known_failed" {
				if recovered.Registration != nil || recovered.State.ResourceInitializations[0].RegistrationAttempts != 1 || recovered.State.ResourceInitializations[0].RegistrationState != "failed" {
					t.Fatal("known failure was retried or given a fresh identity")
				}
				return
			}
			if scenario == "success" {
				if recovered.Registration != nil || recovered.State.ResourceInitializations[0].State != "completed" {
					t.Fatal("completed recovery executed CLI")
				}
				return
			}
			if recovered.Registration == nil || !recovered.Registration.Query || recovered.Registration.Attempt != 2 || recovered.Registration.Plan != d.Plan {
				t.Fatal("recovery re-registered or changed original identity")
			}
			// Query-only missing results consume the original budget, never a fresh ID.
			for i := 2; i <= skillhubpreparation.RegistrationAttemptLimit; i++ {
				if recovered.Registration == nil || !recovered.Registration.Query {
					t.Fatal("expected query continuation")
				}
				recovered, err = owners.workspace.initializePreparationResources(ctx, c, true)
				if err != nil {
					t.Fatal(err)
				}
			}
			if recovered.Registration != nil || recovered.State.ResourceInitializations[0].RegistrationAttempts != 4 {
				t.Fatal("query budget reset")
			}
			stored, _ := owners.repositories.Workspaces.Get(ctx, w.ID)
			if stored.ActiveRunID != "" || workspace.ValidatePreparationEvidencePersistence(stored) != nil || len(stored.PreparationResourceInitializations[0].Attempts) != 1 {
				t.Fatal("registration changed File history, created Run or invalidated persistence")
			}
			raw, _ := json.Marshal(stored)
			if bytes.Contains(raw, []byte(d.Plan.Home)) || bytes.Contains(raw, observation.Stdout) && len(observation.Stdout) > 0 {
				t.Fatal("private receipt leaked into Workspace projection")
			}
		})
	}
}

func TestPreparationRegistrationInstalledBinaryRecoversLostSettlementAfterOwnerReopen(t *testing.T) {
	binary := os.Getenv("WORKORCH_TEST_SKILLHUB_BINARY")
	if binary == "" {
		t.Skip("opt-in isolated production CLI/owner validation")
	}
	root := t.TempDir()
	home := filepath.Join(root, "hub")
	bin := filepath.Join(root, "bin")
	for _, p := range []string{bin, filepath.Join(home, "repositories", "main", "skills")} {
		if err := os.MkdirAll(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(binary, filepath.Join(bin, "skill-hub")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("SKILL_HUB_HOME", home)
	t.Setenv("SKILL_HUB_DISABLE_SERVICE_BRIDGE", "true")
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("multi_repo:\n  enabled: true\n  default_repo: main\n  repositories:\n    main:\n      name: main\n      enabled: true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "state.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	storage := t.TempDir()
	owners := openFileDefinitionAdmissionOwners(t, storage)
	defer func() { owners.close() }()
	w := seedManagedPreparation(t, owners, t.TempDir(), "exact")
	startManagedPreparationOwners(t, owners)
	b := &applicationbiz.Biz{Base: basebiz.New("application/workorchd", owners.hub, nil)}
	owners.workspace.workspaces = &preparationWriteCutRepository{WorkspaceRepository: owners.workspace.workspaces, failAt: 4}
	request := management.PreparationCheckRequest{RequestID: "lost-registration-settlement", ExpectedWorkspaceRevision: w.Revision}
	if _, err := b.InitializeStartupPreparationResources(ctx, w.ID, request, false); err == nil {
		t.Fatal("lost settlement reported success")
	}
	before, err := os.ReadFile(filepath.Join(home, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, storage)
	startManagedPreparationOwners(t, owners)
	b = &applicationbiz.Biz{Base: basebiz.New("application/workorchd", owners.hub, nil)}
	operation, err := b.InitializeStartupPreparationResources(ctx, w.ID, request, true)
	if err != nil || operation.Preparation.ResourceInitializations[0].State != "completed" || operation.Preparation.ResourceInitializations[0].RegistrationAttempts != 2 {
		t.Fatalf("original receipt recovery failed: %+v %v", operation, err)
	}
	after, err := os.ReadFile(filepath.Join(home, "state.json"))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("recovery re-executed registration or rewrote project state")
	}
	stored, _ := owners.repositories.Workspaces.Get(ctx, w.ID)
	g := stored.PreparationResourceInitializations[0].Registration
	savedRaw := bytes.Clone(g.Attempts[1].Result.Stdout)
	g.Attempts[1].Result.Stdout[0] ^= 1
	detached, err := owners.repositories.Workspaces.Get(ctx, w.ID)
	if err != nil || !bytes.Equal(detached.PreparationResourceInitializations[0].Registration.Attempts[1].Result.Stdout, savedRaw) {
		t.Fatal("repository Get leaked mutable receipt bytes")
	}

	if g.Attempts[0].Result != nil || !g.Attempts[1].Query || g.Attempts[1].Result == nil || stored.ActiveRunID != "" {
		t.Fatal("lost dispatch/receipt or Run boundary changed")
	}
}
