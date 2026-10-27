package workspace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchmanagedconfig"
)

func (m *Biz) observeOriginalPreparationManagedConfigs(ctx context.Context, original workspaceevents.CheckWorkspacePreparationCommand) (*workspace.WorkspacePreparationManagedConfigEvidence, string) {
	unavailable := "managed_config_observation_unavailable"
	w, err := m.workspaces.Get(ctx, original.WorkspaceID)
	if err != nil {
		return nil, unavailable
	}
	i, err := preparationOriginalIndex(w, original)
	if err != nil || i < 0 {
		return nil, unavailable
	}
	a := w.StartupPreparation.Attempts[i]
	if a.State != workspace.WorkspacePreparationAccepted || i != len(w.StartupPreparation.Attempts)-1 || w.Revision != a.CommittedWorkspaceRevision {
		return nil, unavailable
	}
	state, err := preparationStateFromPersistence(w, time.Now().UTC())
	if err != nil || state.CurrentValidity == "stale" {
		return nil, unavailable
	}
	read, err := m.queryExactConfirmedStartupSnapshot(ctx, workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: w.ID, ExpectedWorkspaceRevision: w.Revision})
	if err != nil {
		return nil, unavailable
	}
	// The confirmed template snapshot and the exact workspace configuration
	// remain independent source roots. Never fetch current template defaults,
	// use merged effective hints, or invoke the write/repair materializer.
	declarations, err := workorchmanagedconfig.CompilePreparationDeclarations(w.DefinitionAuthoring.TemplateSnapshot.ContextHints, w.ContextHints)
	if err != nil {
		return nil, "managed_config_declaration_invalid"
	}
	c := fileevents.ObservePreparationManagedConfigsCommand{WorkspaceID: w.ID, RequestID: original.RequestID, PayloadHash: original.PayloadHash(), WorkspaceRevision: w.Revision, InternalDirectory: read.Snapshot.InternalDirectory}
	for _, d := range declarations {
		c.Targets = append(c.Targets, fileevents.PreparationManagedConfigTarget{Scope: d.Scope, Path: d.Path})
	}
	if c.Validate() != nil {
		return nil, unavailable
	}
	bounded, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	ev := magicevent.NewEventWithContext(fileevents.TopicObservePreparationManagedConfigs, ModuleID, "blocks/file", magicevent.NewHeader(), bounded, c)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(fileevents.Contracts(), ev.ID(), eventcontract.LaneScope{WorkspaceID: string(w.ID)}))
	result := m.SendEvent(ev)
	if result == nil || result.Error() != nil {
		return nil, unavailable
	}
	value, resultErr := result.Get()
	r, ok := value.(fileevents.ObservePreparationManagedConfigsResult)
	if resultErr != nil || !ok || r.ValidateFor(c) != nil {
		return nil, unavailable
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, unavailable
	}
	e := workspace.WorkspacePreparationManagedConfigEvidence{Version: workspace.PreparationManagedConfigEvidenceVersion, Ref: workspace.PreparationManagedConfigRefPrefix + hex.EncodeToString(nonce[:]), WorkspaceID: w.ID, RequestID: original.RequestID, PayloadHash: original.PayloadHash(), ObservedWorkspaceRevision: w.Revision, ConfigurationHash: read.Snapshot.AuthoringConfigurationHash, InternalDirectory: c.InternalDirectory, StartedAt: r.StartedAt, CompletedAt: r.CompletedAt}
	for i, d := range declarations {
		raw, err := json.Marshal(d.Defaults)
		if err != nil {
			return nil, unavailable
		}
		e.Files = append(e.Files, workspace.WorkspacePreparationManagedConfigObservation{Scope: d.Scope, Path: d.Path, Policy: d.Policy, DefaultsJSON: raw, Observation: r.Files[i].Clone()})
	}
	e.ContentHash = e.Hash()
	if e.Validate() != nil || e.StartedAt.Before(a.StartedAt) || e.CompletedAt.After(time.Now().UTC()) {
		return nil, unavailable
	}
	return &e, e.BlockedReason()
}
