package workspace

import (
	"context"
	magicevent "github.com/muidea/magicCommon/event"
	mcpcommon "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/common"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	"time"
)

// Only the first accepted continuation may launch a bounded MCP handshake.
// The detached reducer derives the exact plan; its final pass independently
// recompiles it after observation. Recovery never calls this function.
func (m *Biz) observeOriginalPreparationMCP(ctx context.Context, w *workspace.Workspace, c workspaceevents.CheckWorkspacePreparationCommand) *mcpevents.ObservePreparationResult {
	if w == nil || w.StartupPreparation == nil {
		return nil
	}
	var command *mcpevents.ObservePreparationCommand
	for _, a := range w.StartupPreparation.Attempts {
		if a.RequestID != c.RequestID || a.ExecutionInputVersion == "" {
			continue
		}
		for _, e := range w.PreparationMCPRegistryEvidence {
			if e.Ref != a.MCPRegistryEvidenceRef || e.Selection == nil || e.Selection.FailureCode != "" || len(e.Selection.SelectedServerIDs) == 0 {
				continue
			}
			for _, d := range w.PreparationDirectoryEvidence {
				if d.ContentHash == a.DirectoryEvidenceHash {
					v := mcpevents.ObservePreparationCommand{Version: mcpevents.PreparationObservationVersion, WorkspaceID: w.ID, RequestID: a.RequestID, PayloadHash: a.PayloadHash, SnapshotHash: a.ObservationSnapshotHash, WorkspaceRevision: a.ExpectedWorkspaceRevision + 1, InternalDirectory: d.Internal.Path, DeliverableDirectory: d.Deliverable.Path, RegistryRevision: e.Selection.Selected.RegistryRevision, ServerIDs: append([]string(nil), e.Selection.SelectedServerIDs...)}
					for _, cap := range e.Selection.Selected.Capabilities {
						v.Capabilities = append(v.Capabilities, mcpevents.PreparationCapability{ID: string(cap.ID), ServerID: string(cap.ServerID), Kind: cap.Kind, Name: cap.Name, URI: cap.URI})
					}
					command = &v
				}
			}
		}
	}
	if command == nil || command.Validate() != nil {
		return nil
	}
	bounded, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	ev := magicevent.NewEventWithContext(mcpevents.TopicObservePreparation, ModuleID, mcpcommon.ModuleID, magicevent.NewHeader(), bounded, *command)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(mcpevents.Contracts(), ev.ID(), eventcontract.LaneScope{WorkspaceID: string(w.ID)}))
	result := m.SendEvent(ev)
	if result == nil || result.Error() != nil {
		// A lost observation reply can only query the original receipt. Never
		// launch another process under a new identity or during recovery.
		return m.queryOriginalPreparationMCP(bounded, *command)
	}
	value, err := result.Get()
	r, ok := value.(mcpevents.ObservePreparationResult)
	if err != nil || !ok || r.ValidateFor(*command) != nil {
		return nil
	}
	copy := r.Clone()
	return &copy
}

func (m *Biz) queryOriginalPreparationMCP(ctx context.Context, command mcpevents.ObservePreparationCommand) *mcpevents.ObservePreparationResult {
	q := mcpevents.QueryPreparationCommand{WorkspaceID: command.WorkspaceID, RequestID: command.RequestID, PayloadHash: command.PayloadHash}
	ev := magicevent.NewEventWithContext(mcpevents.TopicQueryPreparation, ModuleID, mcpcommon.ModuleID, magicevent.NewHeader(), ctx, q)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(mcpevents.Contracts(), ev.ID(), eventcontract.LaneScope{WorkspaceID: string(command.WorkspaceID)}))
	result := m.SendEvent(ev)
	if result == nil || result.Error() != nil {
		return nil
	}
	value, err := result.Get()
	r, ok := value.(mcpevents.QueryPreparationResult)
	if err != nil || !ok || r.State != "recorded" || r.Observation == nil || r.Observation.ValidateFor(command) != nil {
		return nil
	}
	copy := r.Observation.Clone()
	return &copy
}
