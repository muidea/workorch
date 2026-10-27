package workspace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
)

func (m *Biz) bindOriginalPreparationMCPRegistry(ctx context.Context, c workspaceevents.SettleWorkspacePreparationBlockedCommand) *workspace.WorkspacePreparationMCPRegistryEvidence {
	if c.MCPRegistryFacts == nil {
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
	a, f := w.StartupPreparation.Attempts[i], c.MCPRegistryFacts.Clone()
	if a.State != workspace.WorkspacePreparationAccepted || w.Revision != a.CommittedWorkspaceRevision || f.Validate() != nil || f.WorkspaceID != w.ID || f.WorkspaceRevision != w.Revision || a.EvaluationVersion != workspace.PreparationCheckEvaluationVersion || f.SnapshotHash != a.ObservationSnapshotHash || f.StartedAt.Before(a.StartedAt) || f.CompletedAt.After(time.Now().UTC()) {
		return nil
	}
	exact, err := m.queryExactConfirmedStartupSnapshot(ctx, workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: w.ID, ExpectedWorkspaceRevision: w.Revision})
	if err != nil || exact.Snapshot.SnapshotHash != f.SnapshotHash {
		return nil
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil
	}
	e := workspace.WorkspacePreparationMCPRegistryEvidence{Version: workspace.PreparationMCPRegistryEvidenceVersion, Ref: workspace.PreparationMCPRegistryRefPrefix + hex.EncodeToString(nonce[:]), RequestID: a.RequestID, PayloadHash: a.PayloadHash, Binding: a.Binding, Facts: f}
	e.ContentHash = e.Hash()
	if e.Validate() != nil {
		return nil
	}
	return &e
}
