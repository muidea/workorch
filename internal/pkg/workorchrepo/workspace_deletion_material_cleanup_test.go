package workorchrepo

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	fileRepo "github.com/muidea/workorch/internal/pkg/repository/file"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func TestWorkspaceDeletionMaterialCleanerExactReplayAndCrashCut(t *testing.T) {
	dataRoot := t.TempDir()
	first := NewWorkspaceScopedFileRepositorySet(dataRoot)
	target, scope := workspaceDeletionMaterialFixture(t, "ws-cleanup-target", "/tmp/workorch-cleanup-target", "/tmp/workorch-internal-target")
	other := &workspacemodel.Workspace{
		Revision: 4, ID: "ws-cleanup-other", Title: "other", State: common.WorkspaceStateActive,
		DeliverableDirectory: "/tmp/workorch-cleanup-other", InternalDirectory: "/tmp/workorch-internal-other",
		DefaultSoulID: "soul-other", SoulBindings: map[string]common.SoulID{"planning": "soul-other"},
	}
	if err := first.Workspaces.Create(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	if err := first.Workspaces.Create(context.Background(), other); err != nil {
		t.Fatal(err)
	}
	beforeOtherIndex := workspaceIndexEntryForTest(t, first, other.ID)
	bound, err := common.BindRuntimeWorkspaceContext(context.Background(), target.ID)
	if err != nil {
		t.Fatal(err)
	}
	beforeTargetIndex := workspaceIndexEntryForTest(t, first, target.ID)
	rejected := []struct {
		name        string
		scope       repository.WorkspaceDeletionMaterialCleanupScope
		workspaceID common.WorkspaceID
		want        error
	}{
		{name: "Workspace revision", scope: mutateWorkspaceCleanupScope(scope, func(candidate *repository.WorkspaceDeletionMaterialCleanupScope) {
			candidate.ExpectedWorkspaceRevision++
		}), workspaceID: target.ID, want: common.ErrConflict},
		{name: "deletion ID", scope: mutateWorkspaceCleanupScope(scope, func(candidate *repository.WorkspaceDeletionMaterialCleanupScope) { candidate.DeletionID += "-forged" }), workspaceID: target.ID, want: common.ErrConflict},
		{name: "deletion generation", scope: mutateWorkspaceCleanupScope(scope, func(candidate *repository.WorkspaceDeletionMaterialCleanupScope) { candidate.DeletionGeneration++ }), workspaceID: target.ID, want: common.ErrInvalidArgument},
		{name: "intent hash", scope: mutateWorkspaceCleanupScope(scope, func(candidate *repository.WorkspaceDeletionMaterialCleanupScope) { candidate.IntentHash += "-forged" }), workspaceID: target.ID, want: common.ErrConflict},
		{name: "deletion revision", scope: mutateWorkspaceCleanupScope(scope, func(candidate *repository.WorkspaceDeletionMaterialCleanupScope) {
			candidate.ExpectedDeletionRevision++
		}), workspaceID: target.ID, want: common.ErrConflict},
		{name: "cleanup ID", scope: mutateWorkspaceCleanupScope(scope, func(candidate *repository.WorkspaceDeletionMaterialCleanupScope) { candidate.CleanupID += "-forged" }), workspaceID: target.ID, want: common.ErrConflict},
		{name: "missing Workspace aggregate", scope: mutateWorkspaceCleanupScope(scope, func(candidate *repository.WorkspaceDeletionMaterialCleanupScope) {
			candidate.WorkspaceID = "ws-cleanup-missing"
		}), workspaceID: "ws-cleanup-missing", want: common.ErrConflict},
		{name: "wrong bound Workspace", scope: scope, workspaceID: other.ID, want: common.ErrConflict},
		{name: "invalid empty cleanup ID", scope: mutateWorkspaceCleanupScope(scope, func(candidate *repository.WorkspaceDeletionMaterialCleanupScope) { candidate.CleanupID = "" }), workspaceID: target.ID, want: common.ErrInvalidArgument},
	}
	for _, testCase := range rejected {
		t.Run("reject_"+testCase.name, func(t *testing.T) {
			callContext, bindErr := common.BindRuntimeWorkspaceContext(context.Background(), testCase.workspaceID)
			if bindErr != nil {
				t.Fatal(bindErr)
			}
			if cleanupErr := first.WorkspaceCleaner.CleanupDeletionMaterial(callContext, testCase.scope); !errors.Is(cleanupErr, testCase.want) {
				t.Fatalf("cleanup error = %v, want %v", cleanupErr, testCase.want)
			}
			unchanged, getErr := first.Workspaces.Get(context.Background(), target.ID)
			if getErr != nil || unchanged.DefaultSoulID != target.DefaultSoulID || !reflect.DeepEqual(unchanged.SoulBindings, target.SoulBindings) {
				t.Fatalf("rejected cleanup changed target sidecar: Workspace=%+v err=%v", unchanged, getErr)
			}
			if entry := workspaceIndexEntryForTest(t, first, target.ID); !reflect.DeepEqual(entry, beforeTargetIndex) {
				t.Fatalf("rejected cleanup changed target index: before=%+v after=%+v", beforeTargetIndex, entry)
			}
		})
	}

	if err := first.WorkspaceCleaner.CleanupDeletionMaterial(bound, scope); err != nil {
		t.Fatalf("exact cleanup: %v", err)
	}
	if err := first.WorkspaceCleaner.CleanupDeletionMaterial(bound, scope); err != nil {
		t.Fatalf("duplicate exact cleanup: %v", err)
	}
	cleaned, err := first.Workspaces.Get(context.Background(), target.ID)
	if err != nil || cleaned.DefaultSoulID != "" || len(cleaned.SoulBindings) != 0 {
		t.Fatalf("cleaned sidecar = %+v err=%v", cleaned, err)
	}
	cleanedEntry := workspaceIndexEntryForTest(t, first, target.ID)
	if cleanedEntry.DeliverableDirectory != "" || cleanedEntry.InternalDirectory != "" || cleanedEntry.ActiveRunID != "" {
		t.Fatalf("cleaned index entry = %+v", cleanedEntry)
	}
	unchangedOther, err := first.Workspaces.Get(context.Background(), other.ID)
	if err != nil || unchangedOther.DefaultSoulID != other.DefaultSoulID || !reflect.DeepEqual(unchangedOther.SoulBindings, other.SoulBindings) {
		t.Fatalf("other Workspace sidecar changed: Workspace=%+v err=%v", unchangedOther, err)
	}
	afterOtherIndex := workspaceIndexEntryForTest(t, first, other.ID)
	beforeOtherIndex.UpdatedAt, afterOtherIndex.UpdatedAt = "", ""
	if !reflect.DeepEqual(afterOtherIndex, beforeOtherIndex) {
		t.Fatalf("other Workspace index changed: before=%+v after=%+v", beforeOtherIndex, afterOtherIndex)
	}

	// Crash cut: persist only the main Workspace metadata transition and skip
	// the ordinary CAS sidecar/index tail. A fresh repository must still see no
	// old configuration material because the exact private pre-clean completed
	// before directory_cleanup_observed was allowed to advance the phase.
	current, err := first.Workspaces.Get(context.Background(), target.ID)
	if err != nil {
		t.Fatal(err)
	}
	next := *current
	record := current.Deletion.Clone()
	record.DirectoryCleanupAck = &workspacemodel.WorkspaceDeletionDirectoryCleanupAck{CleanupID: scope.CleanupID, CleanedAt: record.UpdatedAt}
	record.Phase = workspacemodel.WorkspaceDeletionPhaseEventStreamFinalizing
	record.Revision++
	next.Title, next.Description, next.DeliverableDirectory, next.InternalDirectory = "", "", "", ""
	next.TemplateID, next.ProfileID, next.DefaultSoulID = "", "", ""
	next.DefaultLLMSelection, next.SoulBindings = nil, nil
	next.ContextHints, next.EvidencePolicies = workorcheventpayload.Payload{}, nil
	next.Deletion = &record
	if err := next.Deletion.Validate(next.ID); err != nil {
		t.Fatalf("cut-point next deletion: %v", err)
	}
	concrete := first.Workspaces.(*workspaceScopedWorkspaceRepository)
	metadataPath := filepath.Join(concrete.store.workspaceRoot(target.ID), workspaceScopedWorkspaceFile)
	if err := concrete.store.workspaceRepo(metadataPath).CompareAndSwap(bound, workspaceMetadata(&next), current.Revision); err != nil {
		t.Fatalf("persist main metadata cut point: %v", err)
	}
	fileRepo.EvictMapFileCache(metadataPath)
	reopened := NewWorkspaceScopedFileRepositorySet(dataRoot)
	fresh, err := reopened.Workspaces.Get(context.Background(), target.ID)
	if err != nil || fresh.Deletion == nil || fresh.Deletion.Phase != workspacemodel.WorkspaceDeletionPhaseEventStreamFinalizing || fresh.DefaultSoulID != "" || len(fresh.SoulBindings) != 0 {
		t.Fatalf("fresh reopen after metadata cut = %+v err=%v", fresh, err)
	}
	freshEntry := workspaceIndexEntryForTest(t, reopened, target.ID)
	if freshEntry.DeliverableDirectory != "" || freshEntry.InternalDirectory != "" || freshEntry.ActiveRunID != "" {
		t.Fatalf("fresh reopen restored old index material: %+v", freshEntry)
	}
	items, err := reopened.Workspaces.List(context.Background())
	if err != nil || !workspaceListContainsForTest(items, target.ID) || !workspaceListContainsForTest(items, other.ID) {
		t.Fatalf("fresh recovery inventory = %+v err=%v", items, err)
	}
}

func TestWorkspaceDeletionMaterialCleanerResumesAfterSidecarOnlyCrash(t *testing.T) {
	dataRoot := t.TempDir()
	first := NewWorkspaceScopedFileRepositorySet(dataRoot)
	target, scope := workspaceDeletionMaterialFixture(t, "ws-sidecar-cut-target", "/tmp/workorch-sidecar-cut-target", "/tmp/workorch-sidecar-cut-internal")
	other := &workspacemodel.Workspace{
		Revision: 3, ID: "ws-sidecar-cut-other", Title: "other", State: common.WorkspaceStateActive,
		DeliverableDirectory: "/tmp/workorch-sidecar-cut-other", InternalDirectory: "/tmp/workorch-sidecar-cut-other-internal",
		DefaultSoulID: "soul-sidecar-cut-other", SoulBindings: map[string]common.SoulID{"planning": "soul-sidecar-cut-other"},
	}
	if err := first.Workspaces.Create(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	if err := first.Workspaces.Create(context.Background(), other); err != nil {
		t.Fatal(err)
	}
	beforeOtherIndex := workspaceIndexEntryForTest(t, first, other.ID)
	concrete := first.Workspaces.(*workspaceScopedWorkspaceRepository)
	if err := concrete.store.saveWorkspaceSoulBindings(&workspacemodel.Workspace{ID: target.ID}); err != nil {
		t.Fatalf("persist sidecar-only crash cut: %v", err)
	}
	if bindings, err := concrete.store.loadWorkspaceSoulBindings(target.ID); err != nil || bindings.DefaultSoulID != "" || len(bindings.Bindings) != 0 {
		t.Fatalf("sidecar-only crash cut was not durable: bindings=%+v err=%v", bindings, err)
	}
	if entry := workspaceIndexEntryForTest(t, first, target.ID); entry.DeliverableDirectory == "" || entry.InternalDirectory == "" {
		t.Fatalf("sidecar-only crash cut unexpectedly changed index: %+v", entry)
	}

	metadataPath := filepath.Join(concrete.store.workspaceRoot(target.ID), workspaceScopedWorkspaceFile)
	fileRepo.EvictMapFileCache(metadataPath)
	reopened := NewWorkspaceScopedFileRepositorySet(dataRoot)
	bound, err := common.BindRuntimeWorkspaceContext(context.Background(), target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.WorkspaceCleaner.CleanupDeletionMaterial(bound, scope); err != nil {
		t.Fatalf("resume exact cleanup after sidecar-only cut: %v", err)
	}
	if err := reopened.WorkspaceCleaner.CleanupDeletionMaterial(bound, scope); err != nil {
		t.Fatalf("duplicate exact cleanup after sidecar-only cut: %v", err)
	}
	cleaned, err := reopened.Workspaces.Get(context.Background(), target.ID)
	if err != nil || cleaned.DefaultSoulID != "" || len(cleaned.SoulBindings) != 0 {
		t.Fatalf("resumed sidecar cleanup = %+v err=%v", cleaned, err)
	}
	if entry := workspaceIndexEntryForTest(t, reopened, target.ID); entry.DeliverableDirectory != "" || entry.InternalDirectory != "" || entry.ActiveRunID != "" {
		t.Fatalf("resumed index cleanup = %+v", entry)
	}
	assertOtherWorkspaceDeletionMaterialUnchanged(t, reopened, other, beforeOtherIndex)
}

func TestWorkspaceDeletionMaterialCleanerFailsClosedOnIndexAmbiguityAndReplaysAfterRepair(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		corrupt func(workspaceIndex, common.WorkspaceID) workspaceIndex
	}{
		{
			name: "missing target",
			corrupt: func(idx workspaceIndex, targetID common.WorkspaceID) workspaceIndex {
				next := workspaceIndex{}
				for _, entry := range idx.Workspaces {
					if entry.WorkspaceID != targetID {
						next.Workspaces = append(next.Workspaces, entry)
					}
				}
				return next
			},
		},
		{
			name: "duplicate target",
			corrupt: func(idx workspaceIndex, targetID common.WorkspaceID) workspaceIndex {
				for _, entry := range idx.Workspaces {
					if entry.WorkspaceID == targetID {
						idx.Workspaces = append(idx.Workspaces, entry)
						break
					}
				}
				return idx
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			dataRoot := t.TempDir()
			first := NewWorkspaceScopedFileRepositorySet(dataRoot)
			target, scope := workspaceDeletionMaterialFixture(t, "ws-index-corrupt-target", "/tmp/workorch-index-corrupt-target", "/tmp/workorch-index-corrupt-internal")
			other := &workspacemodel.Workspace{
				Revision: 5, ID: "ws-index-corrupt-other", Title: "other", State: common.WorkspaceStateActive,
				DeliverableDirectory: "/tmp/workorch-index-corrupt-other", InternalDirectory: "/tmp/workorch-index-corrupt-other-internal",
				DefaultSoulID: "soul-index-corrupt-other", SoulBindings: map[string]common.SoulID{"execution": "soul-index-corrupt-other"},
			}
			if err := first.Workspaces.Create(context.Background(), target); err != nil {
				t.Fatal(err)
			}
			if err := first.Workspaces.Create(context.Background(), other); err != nil {
				t.Fatal(err)
			}
			originalIndex := workspaceIndexForTest(t, first)
			beforeOtherIndex := workspaceIndexEntryForTest(t, first, other.ID)
			corruptIndex := testCase.corrupt(cloneWorkspaceIndex(originalIndex), target.ID)
			replaceWorkspaceIndexForTest(t, first, corruptIndex)

			bound, err := common.BindRuntimeWorkspaceContext(context.Background(), target.ID)
			if err != nil {
				t.Fatal(err)
			}
			if err := first.WorkspaceCleaner.CleanupDeletionMaterial(bound, scope); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("cleanup with ambiguous index error = %v, want conflict", err)
			}
			metadataPath := filepath.Join(first.Workspaces.(*workspaceScopedWorkspaceRepository).store.workspaceRoot(target.ID), workspaceScopedWorkspaceFile)
			metadata, err := first.Workspaces.(*workspaceScopedWorkspaceRepository).store.workspaceRepo(metadataPath).Get(bound, target.ID)
			if err != nil || !reflect.DeepEqual(metadata, workspaceMetadata(target)) {
				t.Fatalf("failed cleanup changed main Workspace metadata: Workspace=%+v err=%v", metadata, err)
			}
			if currentIndex := workspaceIndexForTest(t, first); !reflect.DeepEqual(currentIndex, corruptIndex) {
				t.Fatalf("failed cleanup rewrote ambiguous index: before=%+v after=%+v", corruptIndex, currentIndex)
			}
			assertOtherWorkspaceDeletionMaterialUnchanged(t, first, other, beforeOtherIndex)

			replaceWorkspaceIndexForTest(t, first, originalIndex)
			fileRepo.EvictMapFileCache(metadataPath)
			reopened := NewWorkspaceScopedFileRepositorySet(dataRoot)
			if err := reopened.WorkspaceCleaner.CleanupDeletionMaterial(bound, scope); err != nil {
				t.Fatalf("exact replay after index repair: %v", err)
			}
			if err := reopened.WorkspaceCleaner.CleanupDeletionMaterial(bound, scope); err != nil {
				t.Fatalf("duplicate replay after index repair: %v", err)
			}
			cleaned, err := reopened.Workspaces.Get(context.Background(), target.ID)
			if err != nil || cleaned.DefaultSoulID != "" || len(cleaned.SoulBindings) != 0 {
				t.Fatalf("replayed sidecar cleanup = %+v err=%v", cleaned, err)
			}
			if entry := workspaceIndexEntryForTest(t, reopened, target.ID); entry.DeliverableDirectory != "" || entry.InternalDirectory != "" || entry.ActiveRunID != "" {
				t.Fatalf("replayed index cleanup = %+v", entry)
			}
			assertOtherWorkspaceDeletionMaterialUnchanged(t, reopened, other, beforeOtherIndex)
		})
	}
}

func workspaceDeletionMaterialFixture(t *testing.T, workspaceID common.WorkspaceID, deliverable, internal string) (*workspacemodel.Workspace, repository.WorkspaceDeletionMaterialCleanupScope) {
	t.Helper()
	now := time.Now().UTC()
	record := workspacemodel.WorkspaceDeletionRecord{
		DeletionID: "delete-" + string(workspaceID), DeletionGeneration: 1, RequestID: "request-" + string(workspaceID),
		Mode: deletioncontract.ModePurge, Phase: workspacemodel.WorkspaceDeletionPhaseDirectoryCleanup,
		ExpectedRevision: 9, Revision: 12, RequestedAt: now, UpdatedAt: now,
		SealedDeliverableDirectory: deliverable, SealedInternalDirectory: internal,
	}
	record.IntentHash = workspacemodel.StableWorkspaceDeletionIntentHash(workspaceID, record)
	record.RunDrainingAck = &workspacemodel.WorkspaceDeletionRunDrainingAck{FenceRevision: 3, WorkspaceRunGeneration: 2}
	token := deletioncontract.WorkspaceRuntimeQuiescenceToken{
		WorkspaceID: workspaceID, DeletionID: record.DeletionID, DeletionGeneration: record.DeletionGeneration,
		DataDispositionIntent: record.Mode, WorkspaceIntentHash: record.IntentHash,
		RunDrainingRevision: 3, WorkspaceRunGeneration: 2, OrchestratorQuiescenceRevision: 2,
	}
	token.RuntimeInventoryHash = deletioncontract.StableWorkspaceRuntimeInventoryHash(token)
	token.QuiescenceID = deletioncontract.StableWorkspaceRuntimeQuiescenceID(token)
	record.RuntimeQuiescenceAck = &workspacemodel.WorkspaceDeletionRuntimeQuiescenceAck{Token: token}
	record.RunRetiredAck = &workspacemodel.WorkspaceDeletionRunRetiredAck{
		FenceRevision: 4, WorkspaceRunGeneration: 2, QuiescenceID: token.QuiescenceID, RuntimeInventoryHash: token.RuntimeInventoryHash,
	}
	sealOwners := workspacemodel.WorkspaceDeletionRequiredSealOwners()
	for _, owner := range sealOwners[:len(sealOwners)-1] {
		record.OwnerSealAcks = append(record.OwnerSealAcks, workspacemodel.WorkspaceDeletionOwnerSealAck{
			OwnerModuleID: owner, FenceID: owner + "-fence", Revision: 1, AckHash: owner + "-seal-ack", SealedAt: now,
		})
	}
	purgeOwners := workspacemodel.WorkspaceDeletionRequiredPurgeOwners()
	for _, owner := range purgeOwners[:len(purgeOwners)-1] {
		record.OwnerPurgeAcks = append(record.OwnerPurgeAcks, workspacemodel.WorkspaceDeletionOwnerPurgeAck{
			OwnerModuleID: owner, PurgeID: owner + "-purge", Revision: 2, AckHash: owner + "-purge-ack", PurgeProofHash: owner + "-proof", PurgedAt: now,
		})
	}
	workspace := &workspacemodel.Workspace{
		Revision: 21, ID: workspaceID, Title: "target", State: common.WorkspaceStateActive,
		DeliverableDirectory: deliverable, InternalDirectory: internal,
		TemplateID: "template-1", ProfileID: "profile-1", DefaultSoulID: "soul-target",
		SoulBindings: map[string]common.SoulID{"planning": "soul-target"}, Deletion: &record,
	}
	if err := record.Validate(workspaceID); err != nil {
		t.Fatalf("cleanup fixture: %v", err)
	}
	cleanupID := workspacemodel.StableWorkspaceDeletionCleanupID(record)
	if cleanupID == "" {
		t.Fatal("cleanup fixture has empty cleanup ID")
	}
	return workspace, repository.WorkspaceDeletionMaterialCleanupScope{
		WorkspaceID: workspaceID, DeletionID: record.DeletionID, DeletionGeneration: record.DeletionGeneration,
		IntentHash: record.IntentHash, ExpectedWorkspaceRevision: workspace.Revision,
		ExpectedDeletionRevision: record.Revision, CleanupID: cleanupID,
	}
}

func workspaceIndexEntryForTest(t *testing.T, repositories WorkspaceScopedFileRepositorySet, workspaceID common.WorkspaceID) workspaceIndexEntry {
	t.Helper()
	concrete := repositories.Workspaces.(*workspaceScopedWorkspaceRepository)
	entries, err := concrete.store.listWorkspaceIndexEntries()
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.WorkspaceID == workspaceID {
			return entry
		}
	}
	t.Fatalf("Workspace %s is absent from index", workspaceID)
	return workspaceIndexEntry{}
}

func workspaceListContainsForTest(items []*workspacemodel.Workspace, workspaceID common.WorkspaceID) bool {
	for _, item := range items {
		if item != nil && item.ID == workspaceID {
			return true
		}
	}
	return false
}

func mutateWorkspaceCleanupScope(scope repository.WorkspaceDeletionMaterialCleanupScope, mutate func(*repository.WorkspaceDeletionMaterialCleanupScope)) repository.WorkspaceDeletionMaterialCleanupScope {
	mutate(&scope)
	return scope
}

func workspaceIndexForTest(t *testing.T, repositories WorkspaceScopedFileRepositorySet) workspaceIndex {
	t.Helper()
	concrete := repositories.Workspaces.(*workspaceScopedWorkspaceRepository)
	shared := workspaceIndexLock(concrete.store.workspaceIndexPath())
	shared.Lock()
	defer shared.Unlock()
	concrete.store.indexMu.Lock()
	defer concrete.store.indexMu.Unlock()
	idx, err := concrete.store.loadWorkspaceIndexLocked()
	if err != nil {
		t.Fatal(err)
	}
	return cloneWorkspaceIndex(idx)
}

func replaceWorkspaceIndexForTest(t *testing.T, repositories WorkspaceScopedFileRepositorySet, idx workspaceIndex) {
	t.Helper()
	concrete := repositories.Workspaces.(*workspaceScopedWorkspaceRepository)
	shared := workspaceIndexLock(concrete.store.workspaceIndexPath())
	shared.Lock()
	defer shared.Unlock()
	concrete.store.indexMu.Lock()
	defer concrete.store.indexMu.Unlock()
	if err := concrete.store.saveWorkspaceIndexLocked(cloneWorkspaceIndex(idx)); err != nil {
		t.Fatal(err)
	}
}

func assertOtherWorkspaceDeletionMaterialUnchanged(t *testing.T, repositories WorkspaceScopedFileRepositorySet, expected *workspacemodel.Workspace, expectedIndex workspaceIndexEntry) {
	t.Helper()
	current, err := repositories.Workspaces.Get(context.Background(), expected.ID)
	if err != nil || !reflect.DeepEqual(current, expected) {
		t.Fatalf("other Workspace material changed: Workspace=%+v want=%+v err=%v", current, expected, err)
	}
	currentIndex := workspaceIndexEntryForTest(t, repositories, expected.ID)
	if !reflect.DeepEqual(currentIndex, expectedIndex) {
		t.Fatalf("other Workspace index changed: before=%+v after=%+v", expectedIndex, currentIndex)
	}
}
