package workspace

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	applicationbiz "github.com/muidea/workorch/internal/modules/application/workorchd/biz"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	filebiz "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchmanagedconfig"
	management "github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func TestPreparationFileRevalidationProductionRestartReadbackAndNoRenewal(t *testing.T) {
	ctx, storage := context.Background(), t.TempDir()
	owners := openFileDefinitionAdmissionOwners(t, storage)
	defer owners.close()
	internal := filepath.Join(t.TempDir(), "internal", "workspaces", "ws")
	w := seedManagedPreparation(t, owners, internal, "exact")
	startManagedPreparationOwners(t, owners)
	var configPath string
	for _, root := range []string{workorchmanagedconfig.SystemDirectory(internal), internal} {
		configPath = writeManagedFixture(t, root, []byte(`{"token":"fixture-config-value","nested":{"required":1}}`))
	}
	var calls atomic.Int32
	installPreparationReadFixtures(t, owners, w.ID, "miss", &calls, nil)
	app := &applicationbiz.Biz{Base: basebiz.New(applicationbiz.ModuleID, owners.hub, nil)}
	command := management.PreparationCheckRequest{RequestID: "original-observation", ExpectedWorkspaceRevision: w.Revision}
	original, err := app.CheckStartupPreparation(ctx, w.ID, command, false)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := owners.repositories.Workspaces.Get(ctx, w.ID)
	if err != nil || len(stored.PreparationDirectoryEvidence) != 1 {
		t.Fatal("missing archive", err)
	}
	d := stored.PreparationDirectoryEvidence[0]
	if d.Revalidation == nil || d.Revalidation.Scope.RequestID != command.RequestID || d.Revalidation.StartedAt.Before(stored.PreparationManagedConfigEvidence[0].CompletedAt) || original.Preparation.CurrentValidity != "not_evaluated" {
		t.Fatal("final observation not bound to original pipeline", original.Preparation.CurrentValidity)
	}
	record, archive := stored.StartupPreparation.Clone(), workspace.ClonePreparationDirectoryEvidence(stored.PreparationDirectoryEvidence)
	state, err := preparationStateFromPersistence(stored, d.Revalidation.ExpiresAt)
	if err != nil || state.CurrentValidity != "stale" || !reflect.DeepEqual(*state.Record, record) {
		t.Fatal("expiry renewed or changed history", err)
	}
	if err := os.Rename(configPath, configPath+".preserved"); err != nil {
		t.Fatal(err)
	}
	// A plain read does not pretend to detect arbitrary external filesystem
	// changes, nor repair them. It only consumes known epoch/age/drift facts.
	for _, recovery := range []bool{false, true} {
		again, err := app.CheckStartupPreparation(ctx, w.ID, command, recovery)
		if err != nil || !reflect.DeepEqual(original, again) {
			t.Fatal("replay reread files or renewed observation", err)
		}
	}
	// Independent Workspace repository + File owner epoch. Missing resources
	// cannot trigger new observations during GET, duplicate or recovery.
	reopened := openFileDefinitionAdmissionOwners(t, storage)
	defer reopened.close()
	if err := reopened.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	f, err := filebiz.New(reopened.hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Start(); err != nil {
		t.Fatal(err)
	}
	defer f.Teardown(ctx)
	read, err := reopened.workspace.queryWorkspacePreparationState(ctx, workspaceevents.QueryWorkspacePreparationStateCommand{WorkspaceID: w.ID})
	if err != nil || read.CurrentValidity != "stale" || !reflect.DeepEqual(*read.Record, record) {
		t.Fatal("File restart not projected", err)
	}
	newApp := &applicationbiz.Biz{Base: basebiz.New(applicationbiz.ModuleID, reopened.hub, nil)}
	for _, recovery := range []bool{false, true} {
		again, err := newApp.CheckStartupPreparation(ctx, w.ID, command, recovery)
		if err != nil || again.Preparation.CurrentValidity != "stale" || !reflect.DeepEqual(*again.Preparation.Record, *original.Preparation.Record) {
			t.Fatal("command readback differs from GET on restart", err)
		}
	}
	after, err := reopened.repositories.Workspaces.Get(ctx, w.ID)
	if err != nil || !reflect.DeepEqual(after.PreparationDirectoryEvidence, archive) || calls.Load() != 2 {
		t.Fatal("readback renewed or replaced original File evidence", err)
	}
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatal("readback restored missing file")
	}
	runs, err := reopened.repositories.Runs.List(ctx)
	if err != nil || len(runs) != 0 {
		t.Fatal("File revalidation created Run", err)
	}
	// Final re-observation consumes the real changed image and retains it as
	// history; it cannot relabel the old original checks as newly accepted.
	drift := d.Clone()
	owners.workspace.reobservePreparationFiles(ctx, &drift, &stored.PreparationResourceEvidence[0], &stored.PreparationManagedConfigEvidence[0])
	if drift.Revalidation == nil || drift.Revalidation.Configs[1].Complete {
		t.Fatal("real final image not observed")
	}
	copyWorkspace := *stored
	copyRecord := stored.StartupPreparation.Clone()
	copyWorkspace.StartupPreparation = &copyRecord
	copyWorkspace.PreparationDirectoryEvidence = []workspace.WorkspacePreparationDirectoryEvidence{drift}
	copyRecord.Attempts[0].DirectoryEvidenceHash = drift.ContentHash
	stale, err := workspace.PreparationFileRevalidationStale(&copyWorkspace, time.Now().UTC(), drift.Revalidation.OwnerEpoch)
	if err != nil || !stale || !reflect.DeepEqual(record, *stored.StartupPreparation) {
		t.Fatal("changed final image accepted or original altered", err)
	}
}
