package workspace

import (
	"context"
	"encoding/json"
	"maps"
	"reflect"
	"time"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/skillhubpreparation"
	"github.com/muidea/workorch/internal/pkg/workorchmanagedconfig"
)

func (m *Biz) handlePreparationInitialize(ev magicevent.Event, result magicevent.Result) {
	if result == nil {
		return
	}
	if ev == nil || ev.Source() != workspaceevents.CommandSource || ev.ID() != workspaceevents.TopicWorkspacePreparationInitializeResources && ev.ID() != workspaceevents.TopicWorkspacePreparationRecoverResources {
		result.Set(nil, cd.WrapError(cd.IllegalParam, common.ErrInvalidArgument, "invalid resource initialization source"))
		return
	}
	c, ok := ev.Data().(workspaceevents.InitializeWorkspacePreparationCommand)
	if !ok || c.Validate() != nil {
		result.Set(nil, cd.WrapError(cd.IllegalParam, common.ErrInvalidArgument, "invalid initialization identity"))
		return
	}
	r, err := m.initializePreparationResources(ev.Context(), c, ev.ID() == workspaceevents.TopicWorkspacePreparationRecoverResources)
	if err != nil {
		result.Set(nil, cd.WrapError(cd.Unexpected, err, "resource initialization unavailable; reconcile original identity"))
		return
	}
	result.Set(r, nil)
}
func preparationInitializationResult(w *workspace.Workspace, c workspaceevents.InitializeWorkspacePreparationCommand) (workspaceevents.InitializeWorkspacePreparationResult, error) {
	state, err := preparationStateFromPersistence(w, time.Now().UTC())
	if err != nil {
		return workspaceevents.InitializeWorkspacePreparationResult{}, err
	}
	r := workspaceevents.InitializeWorkspacePreparationResult{RequestID: c.RequestID, ExpectedWorkspaceRevision: c.ExpectedWorkspaceRevision, State: state}
	return r, r.ValidateFor(c)
}
func (m *Biz) initializePreparationResources(ctx context.Context, c workspaceevents.InitializeWorkspacePreparationCommand, recoverOriginal bool) (workspaceevents.InitializeWorkspacePreparationResult, error) {
	empty := workspaceevents.InitializeWorkspacePreparationResult{}
	if m == nil || ctx == nil || m.workspaces == nil || c.Validate() != nil {
		return empty, common.ErrInvalidArgument
	}
	bound, err := common.BindRuntimeWorkspaceContext(ctx, c.WorkspaceID)
	if err != nil {
		return empty, err
	}
	current, err := m.workspaces.Get(bound, c.WorkspaceID)
	if err != nil {
		return empty, err
	}
	if workspaceDeletionRejectsMutation(current) != nil || workspace.ValidatePreparationInitializations(current) != nil {
		return empty, common.ErrConflict
	}
	index := -1
	for i, r := range current.PreparationResourceInitializations {
		if r.Plan.RequestID == c.RequestID {
			if r.ExpectedWorkspaceRevision != c.ExpectedWorkspaceRevision {
				return empty, common.ErrConflict
			}
			index = i
		}
	}
	if index >= 0 && (!recoverOriginal || current.PreparationResourceInitializations[index].InfrastructureObserved()) {
		return preparationInitializationResult(current, c)
	}
	if index >= 0 && current.PreparationResourceInitializations[index].Registration != nil && current.PreparationResourceInitializations[index].Registration.Plan.Home != c.RegistrationHome {
		return empty, common.ErrConflict
	}
	if index >= 0 && recoverOriginal && current.PreparationResourceInitializations[index].FileObserved() {
		return m.reservePreparationRegistration(bound, current, index, c)
	}
	if index < 0 && recoverOriginal {
		return empty, common.ErrNotFound
	}
	if current.StartupPreparation != nil {
		last := current.StartupPreparation.Attempts[len(current.StartupPreparation.Attempts)-1]
		if last.State == workspace.WorkspacePreparationAccepted || last.State == workspace.WorkspacePreparationUnknown {
			return empty, common.ErrConflict
		}
	}
	for _, e := range current.PreparationSkillHubEffects {
		if e.Summary() == workspace.PreparationSkillHubReserved || e.Summary() == workspace.PreparationSkillHubUnresolved {
			return empty, common.ErrConflict
		}
	}
	if index < 0 && (current.Revision != c.ExpectedWorkspaceRevision || workspace.PreparationInitializationPending(current) || len(current.PreparationResourceInitializations) >= workspace.PreparationInitializationLimit) {
		return empty, common.ErrConflict
	}
	read, err := m.queryExactConfirmedStartupSnapshot(bound, workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: c.WorkspaceID, ExpectedWorkspaceRevision: current.Revision})
	if err != nil {
		return empty, err
	}
	if index < 0 {
		ds, err := workorchmanagedconfig.CompilePreparationDeclarations(current.DefinitionAuthoring.TemplateSnapshot.ContextHints, current.ContextHints)
		if err != nil {
			return empty, err
		}
		if current.PreparationFence == nil || current.PreparationFence.Validate(current.ID) != nil {
			return empty, common.ErrConflict
		}
		r := workspace.PreparationResourceInitialization{Version: workspace.PreparationInitializationVersion, ExpectedWorkspaceRevision: c.ExpectedWorkspaceRevision, CommittedWorkspaceRevision: current.Revision + 1, StartupInputHash: read.Snapshot.StartupInput.StartupInputHash, ConfigurationHash: read.Snapshot.AuthoringConfigurationHash, CreationEpoch: current.PreparationFence.CreationEpoch, PreparationEpoch: current.PreparationFence.PreparationEpoch, PreparedAt: time.Now().UTC(), Plan: fileevents.InitializePreparationResourcesCommand{WorkspaceID: c.WorkspaceID, RequestID: c.RequestID, InternalDirectory: read.Snapshot.InternalDirectory, DeliverableDirectory: read.Snapshot.StartupInput.ResolvedProjectWorkspaceDirectory, Declarations: nil}}
		for _, d := range ds {
			raw, e := json.Marshal(d.Defaults)
			if e != nil {
				return empty, e
			}
			r.Plan.Declarations = append(r.Plan.Declarations, fileevents.PreparationInitializationDeclaration{Scope: d.Scope, Path: d.Path, Policy: d.Policy, DefaultsJSON: raw})
		}
		if c.RegistrationHome != "" {
			r.Version = workspace.PreparationInitializationRegistrationVersion
			r.Registration = &workspace.PreparationRegistration{Plan: skillhubpreparation.RegistrationPlan{Version: skillhubpreparation.RegistrationVersion, Home: c.RegistrationHome, Directory: r.Plan.DeliverableDirectory, RequestID: skillhubpreparation.RegistrationRequestID(string(c.WorkspaceID), c.RequestID)}}
		}
		r.Attempts = []workspace.PreparationResourceInitializationAttempt{{PreparedAt: r.PreparedAt}}
		r.DefaultSoulID = current.DefaultSoulID
		r.SoulBindings = maps.Clone(current.SoulBindings)
		r.Plan.BindingHash = r.BindingHash()
		if r.Validate(current.ID) != nil || !r.Matches(current) {
			return empty, common.ErrConflict
		}
		next := *current
		next.Revision = current.Revision + 1
		next.PreparationResourceInitializations = append(workspace.ClonePreparationInitializations(current.PreparationResourceInitializations), r)
		// Never forward a File effect unless the original reservation CAS returned.
		if err = m.workspaces.CompareAndSwap(bound, &next, current.Revision); err != nil {
			return empty, err
		}
		current = &next
		index = len(next.PreparationResourceInitializations) - 1
	} else {
		previous := current.PreparationResourceInitializations[index]
		if !previous.Matches(current) || len(previous.Attempts) >= workspace.PreparationInitializationResultLimit {
			return empty, common.ErrConflict
		}
		next := *current
		next.Revision = current.Revision + 1
		next.PreparationResourceInitializations = workspace.ClonePreparationInitializations(current.PreparationResourceInitializations)
		r := &next.PreparationResourceInitializations[index]
		r.Attempts = append(r.Attempts, workspace.PreparationResourceInitializationAttempt{PreparedAt: time.Now().UTC()})
		r.CommittedWorkspaceRevision = next.Revision
		// A lost/failed dispatch still consumes its durable attempt; recovery never
		// resets the original bounded creation budget.
		if err = m.workspaces.CompareAndSwap(bound, &next, current.Revision); err != nil {
			return empty, err
		}
		current = &next
	}

	r := current.PreparationResourceInitializations[index]
	if index != len(current.PreparationResourceInitializations)-1 || !r.Matches(current) {
		return empty, common.ErrConflict
	}
	ev := magicevent.NewEventWithContext(fileevents.TopicInitializePreparationResources, ModuleID, "blocks/file", magicevent.NewHeader(), bound, r.Plan.Clone())
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(fileevents.Contracts(), ev.ID(), eventcontract.LaneScope{WorkspaceID: string(c.WorkspaceID)}))
	result := m.SendEvent(ev)
	if result == nil {
		return empty, common.ErrConflict
	}
	if ownerErr := result.Error(); ownerErr != nil {
		return empty, ownerErr
	}
	value, resultErr := result.Get()
	if resultErr != nil {
		return empty, resultErr
	}
	facts, ok := value.(fileevents.InitializePreparationResourcesResult)
	if !ok || facts.ValidateFor(r.Plan) != nil {
		return empty, common.ErrConflict
	}
	// Re-read after filesystem I/O. Drift or lost CAS keeps the journal pending;
	// explicit recovery may repeat only these convergent no-overwrite creations.
	after, err := m.workspaces.Get(bound, c.WorkspaceID)
	if err != nil {
		return empty, err
	}
	if after.Revision != current.Revision || !r.Matches(after) || !reflect.DeepEqual(after.PreparationResourceInitializations, current.PreparationResourceInitializations) {
		return empty, common.ErrConflict
	}
	next := *after
	next.Revision = after.Revision + 1
	next.PreparationResourceInitializations = workspace.ClonePreparationInitializations(after.PreparationResourceInitializations)
	updated := &next.PreparationResourceInitializations[index]
	saved := facts.Clone()
	updated.Attempts[len(updated.Attempts)-1].Result = &saved
	updated.CommittedWorkspaceRevision = after.Revision + 1
	if err = m.workspaces.CompareAndSwap(bound, &next, after.Revision); err != nil {
		return empty, err
	}
	if updated.FileObserved() && updated.Registration != nil {
		return m.reservePreparationRegistration(bound, &next, index, c)
	}
	return preparationInitializationResult(&next, c)
}
