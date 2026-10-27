package workspace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"reflect"
	"time"

	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/soulbinding"
)

// Rebind complete factual values to the owner's accepted original operation.
// No application success flag or caller-selected configuration is consumed.
func (m *Biz) bindOriginalPreparationSouls(ctx context.Context, c workspaceevents.SettleWorkspacePreparationBlockedCommand) (*workspace.WorkspacePreparationSoulEvidence, string) {
	if c.SoulFacts == nil {
		return nil, c.SoulReason
	}
	unavailable := "soul_observation_unavailable_not_saved"
	w, err := m.workspaces.Get(ctx, c.Original.WorkspaceID)
	if err != nil {
		return nil, unavailable
	}
	i, err := preparationOriginalIndex(w, c.Original)
	if err != nil || i < 0 || i != len(w.StartupPreparation.Attempts)-1 {
		return nil, unavailable
	}
	a := w.StartupPreparation.Attempts[i]
	f := c.SoulFacts.Clone()
	if a.State != workspace.WorkspacePreparationAccepted || w.Revision != a.CommittedWorkspaceRevision || f.Validate() != nil || f.WorkspaceRevision != w.Revision || f.ConfigurationHash != a.Binding.AuthoringConfigurationHash || f.WorkspaceDefaultSoulID != w.DefaultSoulID || !reflect.DeepEqual(f.WorkspaceBindings, soulbinding.FromSoulIDMap(w.SoulBindings)) || f.StartedAt.Before(a.StartedAt) || f.CompletedAt.After(time.Now().UTC()) {
		return nil, unavailable
	}
	read, err := m.queryExactConfirmedStartupSnapshot(ctx, workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: w.ID, ExpectedWorkspaceRevision: w.Revision})
	if err != nil || read.Snapshot.SnapshotHash != f.SnapshotHash || read.Snapshot.AuthoringConfigurationHash != f.ConfigurationHash {
		return nil, unavailable
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, unavailable
	}
	e := workspace.WorkspacePreparationSoulEvidence{Version: workspace.PreparationSoulEvidenceVersion, Ref: workspace.PreparationSoulRefPrefix + hex.EncodeToString(nonce[:]), RequestID: c.Original.RequestID, PayloadHash: c.Original.PayloadHash(), Facts: f}
	e.ContentHash = e.Hash()
	if e.Validate() != nil {
		return nil, unavailable
	}
	return &e, "souls_observed_remaining_checks"
}
