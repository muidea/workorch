package workspace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
)

// Bind a full cache-only point observation to the actual accepted operation.
// Missing/late material never becomes a cache miss or a currentness claim.
func (m *Biz) bindOriginalPreparationProviderCache(ctx context.Context, c workspaceevents.SettleWorkspacePreparationBlockedCommand) *workspace.WorkspacePreparationProviderCacheEvidence {
	if c.ProviderCacheFacts == nil {
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
	a, f := w.StartupPreparation.Attempts[i], c.ProviderCacheFacts.Clone()
	if a.State != workspace.WorkspacePreparationAccepted || w.Revision != a.CommittedWorkspaceRevision || f.Validate() != nil || f.Observation.WorkspaceID != w.ID || f.WorkspaceRevision != w.Revision || f.Observation.ObservedAt.Before(a.StartedAt) || f.Observation.ObservedAt.After(time.Now().UTC()) {
		return nil
	}
	_, err = m.queryExactConfirmedStartupSnapshot(ctx, workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: w.ID, ExpectedWorkspaceRevision: w.Revision})
	if err != nil {
		return nil
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil
	}
	e := workspace.WorkspacePreparationProviderCacheEvidence{Version: workspace.PreparationProviderCacheEvidenceVersion, Ref: workspace.PreparationProviderCacheRefPrefix + hex.EncodeToString(nonce[:]), RequestID: c.Original.RequestID, PayloadHash: c.Original.PayloadHash(), Binding: a.Binding, Facts: f}
	e.ContentHash = e.Hash()
	if e.Validate() != nil {
		return nil
	}
	return &e
}
