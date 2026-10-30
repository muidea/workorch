package workspace

import (
	"context"
	"errors"
	"github.com/muidea/workorch/internal/pkg/preparationtrace"
	"time"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/startuppreparation"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (m *Biz) handlePreparationStateQuery(ev magicevent.Event, result magicevent.Result) {
	if result == nil {
		return
	}
	if ev == nil || ev.Source() != workspaceevents.CommandSource {
		result.Set(nil, cd.WrapError(cd.IllegalParam, common.ErrInvalidArgument, "invalid preparation-state query source"))
		return
	}
	command, ok := ev.Data().(workspaceevents.QueryWorkspacePreparationStateCommand)
	if !ok || command.WorkspaceID == "" {
		result.Set(nil, cd.WrapError(cd.IllegalParam, common.ErrInvalidArgument, "invalid preparation-state query"))
		return
	}
	value, err := m.queryWorkspacePreparationState(ev.Context(), command)
	if err != nil {
		result.Set(nil, cd.WrapError(cd.Unexpected, err, "preparation state is unavailable"))
		return
	}
	result.Set(value, nil)
}

func (m *Biz) queryWorkspacePreparationState(ctx context.Context, command workspaceevents.QueryWorkspacePreparationStateCommand) (workspaceevents.QueryWorkspacePreparationStateResult, error) {
	if m == nil || m.workspaces == nil || ctx == nil || command.WorkspaceID == "" {
		return workspaceevents.QueryWorkspacePreparationStateResult{}, common.ErrInvalidArgument
	}
	m.deletionIngressMu.RLock()
	defer m.deletionIngressMu.RUnlock()
	bound, err := common.BindRuntimeWorkspaceContext(ctx, command.WorkspaceID)
	if err != nil {
		return workspaceevents.QueryWorkspacePreparationStateResult{}, err
	}
	w, err := m.workspaces.Get(bound, command.WorkspaceID)
	if err != nil {
		return workspaceevents.QueryWorkspacePreparationStateResult{}, err
	}
	if workspaceDeletionRejectsMutation(w) != nil {
		return workspaceevents.QueryWorkspacePreparationStateResult{}, common.ErrConflict
	}
	return m.preparationState(ctx, w)
}

// Query and command readback share the same known-currentness projection.
// The only live File query is its in-memory process epoch: no filesystem read,
// new observation, deadline extension or settlement is performed here.
func (m *Biz) preparationState(ctx context.Context, w *workspace.Workspace) (workspaceevents.QueryWorkspacePreparationStateResult, error) {
	if ctx == nil || w == nil {
		return workspaceevents.QueryWorkspacePreparationStateResult{}, common.ErrInvalidArgument
	}
	requestID := ""
	if w != nil && w.StartupPreparation != nil && len(w.StartupPreparation.Attempts) > 0 {
		requestID = w.StartupPreparation.Attempts[len(w.StartupPreparation.Attempts)-1].RequestID
	}
	defer preparationtrace.Start(ctx, w.ID, requestID, "owner_readback", w.Revision)()
	// Query the epoch before evaluating currentness, so projection uses the
	// clock after owner I/O rather than a timestamp captured before a slow read.
	epoch, epochErr := "", error(nil)
	if workspace.PreparationFileObservationEpoch(w) != "" {
		finishEpoch := preparationtrace.Start(ctx, w.ID, requestID, "file_epoch_query", w.Revision)
		epoch, epochErr = m.preparationFileEpoch(ctx)
		finishEpoch()
	}
	state, err := preparationStateFromPersistence(w, time.Now().UTC())
	if err != nil {
		return state, err
	}
	if workspace.PreparationFileObservationEpoch(w) != "" {
		if epochErr != nil {
			// Missing observation is unknown, not proof of expiry or epoch change.
			// An independently known stale reason above continues to take precedence.
			if state.CurrentValidity != "stale" {
				state.CurrentValidity, state.CurrentValidityReason = "not_evaluated", "file_epoch_unavailable"
				if errors.Is(epochErr, context.DeadlineExceeded) {
					state.CurrentValidityReason = "file_epoch_timeout"
				}
				if errors.Is(epochErr, context.Canceled) {
					state.CurrentValidityReason = "file_epoch_cancelled"
				}
			}
		} else {
			stale, err := workspace.PreparationFileRevalidationStale(w, time.Now().UTC(), epoch)
			if err != nil {
				return state, err
			}
			if stale && state.CurrentValidity != "stale" {
				state.CurrentValidity, state.CurrentValidityReason = "stale", "file_epoch_changed_or_expired"
			}
		}
	}

	return state, nil
}

// A read may project known drift, but cannot compile a new manifest/bootstrap,
// refresh any observation, write a receipt or settle unknown operations.
func preparationStateFromPersistence(w *workspace.Workspace, now time.Time) (workspaceevents.QueryWorkspacePreparationStateResult, error) {
	if w == nil || w.ID == "" || w.Revision == 0 || now.IsZero() {
		return workspaceevents.QueryWorkspacePreparationStateResult{}, common.ErrInvalidArgument
	}
	result := workspaceevents.QueryWorkspacePreparationStateResult{
		WorkspaceID: w.ID, WorkspaceRevision: w.Revision, ActiveRunID: w.ActiveRunID,
		State: "not_checked", CurrentValidity: "not_checked",
	}
	if workspace.ValidatePreparationInitializations(w) != nil {
		return result, common.ErrConflict
	}
	for _, r := range w.PreparationResourceInitializations {
		v := workspaceevents.PreparationInitializationView{RequestID: r.Plan.RequestID, ExpectedWorkspaceRevision: r.ExpectedWorkspaceRevision, CommittedWorkspaceRevision: r.CommittedWorkspaceRevision, State: r.State(), Attempts: len(r.Attempts)}
		if r.Registration != nil {
			v.RegistrationState = r.Registration.Status()
			if !r.FileObserved() {
				v.RegistrationState = "waiting_for_files"
			}
			v.RegistrationAttempts = len(r.Registration.Attempts)
		}
		result.ResourceInitializations = append(result.ResourceInitializations, v)
	}
	if w.StartupPreparation != nil {
		issue, err := workspace.PreparationSkillHubProjectIssue(w)
		if err != nil {
			return result, err
		}
		result.SkillHubProjectIssue = issue
		if w.StartupPreparation.Validate(w.ID) != nil {
			return workspaceevents.QueryWorkspacePreparationStateResult{}, common.ErrConflict
		}
		record := w.StartupPreparation.Clone()
		result.Record = &record
		last := record.Attempts[len(record.Attempts)-1]
		result.State, result.CurrentValidity = string(last.State), "not_evaluated"
		for _, c := range record.Completions {
			if c.RequestID == last.RequestID {
				result.State = "completed"
				result.CurrentValidity = "observed"
				if now.Before(c.CompletedAt) || !now.Before(c.ExpiresAt) {
					result.CurrentValidity, result.CurrentValidityReason = "stale", "completion_outside_window"
				}
			}
		}
		fileStale, err := workspace.PreparationFileRevalidationStale(w, now, workspace.PreparationFileObservationEpoch(w))
		if err != nil {
			return workspaceevents.QueryWorkspacePreparationStateResult{}, err
		}
		if fileStale {
			result.CurrentValidity, result.CurrentValidityReason = "stale", "file_observation_stale"
		}
		cacheExpired, err := workspace.PreparationProviderCacheExpired(w, now)
		if err != nil {
			return workspaceevents.QueryWorkspacePreparationStateResult{}, err
		}
		if cacheExpired {
			result.CurrentValidity, result.CurrentValidityReason = "stale", "provider_cache_expired"
		}
		ownersStale, err := workspace.PreparationOwnerRevalidationStale(w, now)
		if err != nil {
			return workspaceevents.QueryWorkspacePreparationStateResult{}, err
		}
		if ownersStale {
			result.CurrentValidity, result.CurrentValidityReason = "stale", "owner_observation_stale"
		}
		binding := last.Binding
		if record.Version == workspace.WorkspacePreparationWorkflowVersion {
			if w.PreparationFence == nil || w.PreparationFence.Validate(w.ID) != nil || last.CreationEpoch != w.PreparationFence.CreationEpoch || last.PreparationEpoch != w.PreparationFence.PreparationEpoch {
				result.CurrentValidity, result.CurrentValidityReason = "stale", "binding_changed"
			}
			for _, check := range last.Checks {
				if check.Reason == "binding_changed" {
					result.CurrentValidity, result.CurrentValidityReason = "stale", "binding_changed"
				}
			}
		}
		journal := w.DefinitionAuthoringConfirmation
		if !last.ExpiresAt.IsZero() && !now.Before(last.ExpiresAt) && result.CurrentValidity != "stale" {
			result.CurrentValidity, result.CurrentValidityReason = "stale", "attempt_outside_window"
		}
		if last.CommittedWorkspaceRevision != w.Revision ||
			binding.CheckPolicyVersion != startuppreparation.CheckPolicyVersion || binding.CompilerVersion != startuppreparation.BootstrapCompilerVersion ||
			binding.AuthoringConfigurationHash != workspace.WorkspaceConfigurationHash(w) ||
			w.StartupInput == nil || w.ActivationAdmission == nil ||
			binding.StartupInputHash != w.StartupInput.StartupInputHash || binding.DefinitionRevision != w.StartupInput.DefinitionRevision ||
			binding.CandidateID != w.ActivationAdmission.CandidateID || binding.CandidateRevision != w.ActivationAdmission.CandidateRevision || binding.CandidateHash != w.ActivationAdmission.CandidateHash ||
			w.ActivationAdmission.State != workspace.WorkspaceActivationEligible || w.StartupInput.SpecificationHash != binding.AuthoringRef.SpecificationHash ||
			journal == nil || journal.Phase != workspace.DefinitionAuthoringConfirmationMaterialized ||
			journal.InputHash != binding.AuthoringRef.InputHash || journal.DerivationPermitID != binding.AuthoringRef.DerivationPermitID ||
			journal.ProposalHash != binding.AuthoringRef.ProposalHash || journal.SpecificationHash != binding.AuthoringRef.SpecificationHash {
			result.CurrentValidity, result.CurrentValidityReason = "stale", "binding_changed"
		}
	}
	if err := result.ValidateFor(workspaceevents.QueryWorkspacePreparationStateCommand{WorkspaceID: w.ID}); err != nil {
		return workspaceevents.QueryWorkspacePreparationStateResult{}, err
	}
	return result, nil
}
