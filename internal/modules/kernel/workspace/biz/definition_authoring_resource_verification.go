package workspace

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	filecommon "github.com/muidea/workorch/internal/modules/blocks/file/pkg/common"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
)

// File verification runs before entering the Workspace mutation lock. File
// queries the current Workspace scope while observing each resource; the
// reducer then fences that observation with the Workspace revision and CAS.
func (m *Biz) verifyAuthoringDerivationResources(ctx context.Context, reserve workspaceevents.ReserveDefinitionDerivationCommand) error {
	if m == nil || m.EventHub() == nil || ctx == nil || reserve.Validate() != nil || len(reserve.Resources) == 0 {
		return common.ErrInvalidArgument
	}
	for _, resource := range reserve.Resources {
		command := fileevents.VerifyAuthoringResourceCommand{
			WorkspaceID: reserve.WorkspaceID, WorkspaceRevision: reserve.WorkspaceRevision,
			AuthoringInputHash: reserve.InputHash, ResourceRef: resource.Ref,
			Content: resource.Content, ContentHash: resource.ContentHash,
		}
		if err := command.Validate(); err != nil {
			return err
		}
		ev := magicevent.NewEventWithContext(fileevents.TopicVerifyAuthoringResource, ModuleID, filecommon.ModuleID, magicevent.NewHeader(), ctx, command)
		ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(fileevents.Contracts(), fileevents.TopicVerifyAuthoringResource, eventcontract.LaneScope{WorkspaceID: string(reserve.WorkspaceID)}))
		result := m.SendEvent(ev)
		if result == nil {
			return common.ErrConflict
		}
		if err := result.Error(); err != nil {
			return err
		}
		value, err := result.Get()
		if err != nil {
			return err
		}
		verified, ok := value.(fileevents.VerifyAuthoringResourceResult)
		if !ok || verified.ValidateFor(command) != nil {
			return common.ErrConflict
		}
	}
	return nil
}

// A File result is an observation at a Workspace revision, not an atomic
// snapshot across resources. The owner fences that revision again before CAS.
type verifiedAuthoringResources struct {
	workspaceRevision uint64
	inputHash         string
	derivationPermit  string
	snapshotHash      string
}

func (m *Biz) verifyCurrentAuthoringResources(ctx context.Context, id common.WorkspaceID, permitID string) (verifiedAuthoringResources, error) {
	if permitID == "" {
		return verifiedAuthoringResources{}, common.ErrInvalidArgument
	}
	view, err := m.queryDefinitionAuthoring(ctx, workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: id})
	if err != nil {
		return verifiedAuthoringResources{}, err
	}
	if view.Record == nil || !view.CurrentTemplateAvailable || view.Record.TemplateSnapshotHash != view.CurrentTemplateSnapshotHash || view.Record.Validate() != nil {
		return verifiedAuthoringResources{}, common.ErrConflict
	}
	record, err := view.Record.Model()
	if err != nil {
		return verifiedAuthoringResources{}, err
	}
	var derivation *workspacemodel.DefinitionDerivationRequest
	for n := range record.DerivationRequests {
		request := &record.DerivationRequests[n]
		if request.PermitID == permitID {
			derivation = request
			break
		}
	}
	if view.Confirmation != nil && view.Confirmation.DerivationPermitID != permitID {
		return verifiedAuthoringResources{}, common.ErrConflict
	}
	if derivation == nil || derivation.ValidateView() != nil || derivation.Proposal == nil || derivation.Input.InputHash() != view.Record.InputHash || workspacemodel.WorkspaceTemplateSnapshotHash(&derivation.Template) != view.Record.TemplateSnapshotHash {
		return verifiedAuthoringResources{}, common.ErrConflict
	}
	if view.Confirmation != nil && view.Confirmation.ResourceSnapshotHash != derivation.ResourceSnapshotHash {
		return verifiedAuthoringResources{}, common.ErrConflict
	}
	proof := verifiedAuthoringResources{workspaceRevision: view.WorkspaceRevision, inputHash: view.Record.InputHash, derivationPermit: derivation.PermitID, snapshotHash: derivation.ResourceSnapshotHash}
	if len(derivation.Resources) == 0 {
		return proof, nil
	}
	if err := m.verifyAuthoringDerivationResources(ctx, workspaceevents.ReserveDefinitionDerivationCommand{
		WorkspaceID: id, WorkspaceRevision: view.WorkspaceRevision, InputHash: view.Record.InputHash,
		RequestID: derivation.RequestID, Resources: workspacemodel.CloneDefinitionAuthoringResources(derivation.Resources),
		ResourceSnapshotHash: derivation.ResourceSnapshotHash, ProviderBindingHash: derivation.ProviderBindingHash,
	}); err != nil {
		return verifiedAuthoringResources{}, err
	}
	return proof, nil
}

func (p verifiedAuthoringResources) matches(current *workspacemodel.Workspace, derivation workspacemodel.DefinitionDerivationRequest) bool {
	return current != nil && current.DefinitionAuthoring != nil && p.workspaceRevision == current.Revision &&
		p.inputHash == current.DefinitionAuthoring.InputHash && p.derivationPermit == derivation.PermitID &&
		p.snapshotHash == derivation.ResourceSnapshotHash && derivation.Validate() == nil
}
