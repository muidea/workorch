package workspace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
)

// Archive the original ordered observation without asking Run again. Only the
// accepted operation may bind it; replay/recovery cannot fabricate a new point.
func (m *Biz) bindOriginalPreparationRunHistory(ctx context.Context, c workspaceevents.SettleWorkspacePreparationBlockedCommand) *workspace.WorkspacePreparationRunHistoryEvidence {
	if c.RunHistoryFacts == nil {
		return nil
	}
	w, err := m.workspaces.Get(ctx, c.Original.WorkspaceID)
	if err != nil {
		return nil
	}
	i, err := preparationOriginalIndex(w, c.Original)
	if err != nil || i < 0 || i != len(w.StartupPreparation.Attempts)-1 {
		return nil
	}
	a, f := w.StartupPreparation.Attempts[i], c.RunHistoryFacts.Clone()
	if a.State != workspace.WorkspacePreparationAccepted || w.Revision != a.CommittedWorkspaceRevision || f.Validate() != nil || f.Observation.History.WorkspaceID != w.ID || f.Observation.RequestID != a.RequestID || f.WorkspaceRevision != w.Revision || f.Observation.ObservedAt.Before(a.StartedAt) || f.Observation.ObservedAt.After(time.Now().UTC()) || c.RunHistoryReason != f.BlockedReason() {
		return nil
	}
	if _, err := m.queryExactConfirmedStartupSnapshot(ctx, workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: w.ID, ExpectedWorkspaceRevision: w.Revision}); err != nil {
		return nil
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil
	}
	e := workspace.WorkspacePreparationRunHistoryEvidence{Version: workspace.PreparationRunHistoryEvidenceVersion, Ref: workspace.PreparationRunHistoryRefPrefix + hex.EncodeToString(nonce[:]), RequestID: a.RequestID, PayloadHash: a.PayloadHash, Binding: a.Binding, CreationEpoch: a.CreationEpoch, PreparationEpoch: a.PreparationEpoch, Facts: f}
	e.ContentHash = e.Hash()
	if e.Validate() != nil {
		return nil
	}
	return &e
}
