package workspace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"reflect"
	"time"

	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
)

// Owner-only point-in-time observations. No public command accepts a seal,
// manifest or PASS. The existing exact query independently rereads formal rows
// and the complete authoring ledger and retains its full all_of compiler gate.
func (m *Biz) observeOriginalPreparationSeal(ctx context.Context, c workspaceevents.CheckWorkspacePreparationCommand) *workspace.WorkspacePreparationSealEvidence {
	started := time.Now().UTC()
	query := workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: c.WorkspaceID, ExpectedWorkspaceRevision: c.ExpectedWorkspaceRevision + 1}
	before, err := m.queryExactConfirmedStartupSnapshot(ctx, query)
	if err != nil {
		return nil
	}
	after, err := m.queryExactConfirmedStartupSnapshot(ctx, query)
	if err != nil || !reflect.DeepEqual(before.Snapshot, after.Snapshot) {
		return nil
	}
	w, err := m.workspaces.Get(ctx, c.WorkspaceID)
	if err != nil || w.Revision != query.ExpectedWorkspaceRevision || w.StartupPreparation == nil {
		return nil
	}
	index, err := preparationOriginalIndex(w, c)
	if err != nil || index < 0 {
		return nil
	}
	a := w.StartupPreparation.Attempts[index]
	if a.State != workspace.WorkspacePreparationAccepted || a.EvaluationVersion != workspace.PreparationCheckEvaluationVersion || a.ObservationSnapshotHash != before.Snapshot.SnapshotHash {
		return nil
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil
	}
	e := workspace.WorkspacePreparationSealEvidence{Version: workspace.PreparationSealEvidenceVersion, Ref: workspace.PreparationSealRefPrefix + hex.EncodeToString(nonce[:]), RequestID: c.RequestID, PayloadHash: c.PayloadHash(), Binding: a.Binding, StartedAt: started, CompletedAt: time.Now().UTC(), Before: workspace.PreparationSealSnapshot(before.Snapshot).Clone(), After: workspace.PreparationSealSnapshot(after.Snapshot).Clone()}
	e.ContentHash = e.Hash()
	if e.ContentHash == "" {
		return nil
	}
	return &e
}
