package workspace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"time"
)

// Rebind complete history to the accepted original operation. No ready flag,
// hash-only shortcut or caller-selected current configuration is consumed.
func (m *Biz) bindOriginalPreparationSkillRegistry(ctx context.Context, c workspaceevents.SettleWorkspacePreparationBlockedCommand) (*workspace.WorkspacePreparationSkillRegistryEvidence, string) {
	if c.SkillRegistryFacts == nil {
		return nil, c.SkillRegistryReason
	}
	const unavailable = "skill_registry_observation_unavailable_not_saved"
	w, err := m.workspaces.Get(ctx, c.Original.WorkspaceID)
	if err != nil {
		return nil, unavailable
	}
	i, err := preparationOriginalIndex(w, c.Original)
	if err != nil || i < 0 || i != len(w.StartupPreparation.Attempts)-1 {
		return nil, unavailable
	}
	a := w.StartupPreparation.Attempts[i]
	f := c.SkillRegistryFacts.Clone()
	if a.State != workspace.WorkspacePreparationAccepted || w.Revision != a.CommittedWorkspaceRevision || f.Validate() != nil || f.WorkspaceID != w.ID || f.WorkspaceRevision != w.Revision || f.StartedAt.Before(a.StartedAt) || f.CompletedAt.After(time.Now().UTC()) {
		return nil, unavailable
	}
	read, err := m.queryExactConfirmedStartupSnapshot(ctx, workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: w.ID, ExpectedWorkspaceRevision: w.Revision})
	if err != nil || read.Snapshot.SnapshotHash != f.SnapshotHash {
		return nil, unavailable
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, unavailable
	}
	e := workspace.WorkspacePreparationSkillRegistryEvidence{Version: workspace.PreparationSkillRegistryEvidenceVersion, Ref: workspace.PreparationSkillRegistryRefPrefix + hex.EncodeToString(nonce[:]), RequestID: c.Original.RequestID, PayloadHash: c.Original.PayloadHash(), Facts: f}
	e.ContentHash = e.Hash()
	if e.Validate() != nil {
		return nil, unavailable
	}
	return &e, "skill_registry_observed_selection_pending"
}
