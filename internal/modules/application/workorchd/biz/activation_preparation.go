package biz

import (
	"context"
	"github.com/muidea/workorch/internal/pkg/preparationtrace"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationports"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

// QueryExactConfirmedStartupSnapshot is the explicit preparation/compiler read
// boundary. The persisted-state GET deliberately does not call it.
func (b *Biz) QueryExactConfirmedStartupSnapshot(ctx context.Context, command workspaceevents.QueryExactConfirmedStartupSnapshotCommand) (workspaceevents.QueryExactConfirmedStartupSnapshotResult, error) {
	if b == nil || b.EventHub() == nil || ctx == nil || command.Validate() != nil {
		return workspaceevents.QueryExactConfirmedStartupSnapshotResult{}, common.ErrInvalidArgument
	}
	if err := ctx.Err(); err != nil {
		return workspaceevents.QueryExactConfirmedStartupSnapshotResult{}, err
	}
	ev := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceExactConfirmedStartupQuery, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(workspaceevents.Contracts(), workspaceevents.TopicWorkspaceExactConfirmedStartupQuery, eventcontract.LaneScope{WorkspaceID: string(command.WorkspaceID)}))
	result := b.SendEvent(ev)
	if result == nil {
		return workspaceevents.QueryExactConfirmedStartupSnapshotResult{}, common.ErrConflict
	}
	if err := result.Error(); err != nil {
		return workspaceevents.QueryExactConfirmedStartupSnapshotResult{}, err
	}
	value, err := result.Get()
	if err != nil {
		return workspaceevents.QueryExactConfirmedStartupSnapshotResult{}, err
	}
	response, ok := value.(workspaceevents.QueryExactConfirmedStartupSnapshotResult)
	if !ok || response.ValidateFor(command) != nil {
		return workspaceevents.QueryExactConfirmedStartupSnapshotResult{}, common.ErrConflict
	}
	response.Snapshot = response.Snapshot.Clone()
	return response, nil
}

// ReadStartupPreparationView sends only the persisted-state query. It does not
// invoke exact-seal compilation, resource observation or execution bootstrap.
func (b *Biz) ReadStartupPreparationView(ctx context.Context, workspaceID common.WorkspaceID) (workorchmanagementcontract.StartupPreparationView, error) {
	if b == nil || b.EventHub() == nil || ctx == nil || workspaceID == "" {
		return workorchmanagementcontract.StartupPreparationView{}, common.ErrInvalidArgument
	}
	if err := ctx.Err(); err != nil {
		return workorchmanagementcontract.StartupPreparationView{}, err
	}
	command := workspaceevents.QueryWorkspacePreparationStateCommand{WorkspaceID: workspaceID}
	ev := magicevent.NewEventWithContext(workspaceevents.TopicWorkspacePreparationStateQuery, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(workspaceevents.Contracts(), workspaceevents.TopicWorkspacePreparationStateQuery, eventcontract.LaneScope{WorkspaceID: string(workspaceID)}))
	result := b.SendEvent(ev)
	if result == nil {
		return workorchmanagementcontract.StartupPreparationView{}, common.ErrConflict
	}
	if err := result.Error(); err != nil {
		return workorchmanagementcontract.StartupPreparationView{}, err
	}
	value, err := result.Get()
	if err != nil {
		return workorchmanagementcontract.StartupPreparationView{}, err
	}
	response, ok := value.(workspaceevents.QueryWorkspacePreparationStateResult)
	if !ok || response.ValidateFor(command) != nil {
		return workorchmanagementcontract.StartupPreparationView{}, common.ErrConflict
	}
	return startupPreparationView(response)
}

func startupPreparationView(response workspaceevents.QueryWorkspacePreparationStateResult) (workorchmanagementcontract.StartupPreparationView, error) {
	view := workorchmanagementcontract.StartupPreparationView{
		WorkspaceID: string(response.WorkspaceID), WorkspaceRevision: response.WorkspaceRevision, ActiveRunID: string(response.ActiveRunID), OperationState: response.State, CurrentValidity: response.CurrentValidity,
		SkillHubProjectIssue: response.SkillHubProjectIssue, CurrentValidityReason: response.CurrentValidityReason,
	}
	for _, r := range response.ResourceInitializations {
		view.ResourceInitializations = append(view.ResourceInitializations, workorchmanagementcontract.PreparationResourceInitializationView{RequestID: r.RequestID, ExpectedWorkspaceRevision: r.ExpectedWorkspaceRevision, CommittedWorkspaceRevision: r.CommittedWorkspaceRevision, State: r.State, Attempts: r.Attempts, RegistrationState: r.RegistrationState, RegistrationAttempts: r.RegistrationAttempts})
	}
	if response.Record != nil {
		view.Record = &workorchmanagementcontract.PreparationRecordView{Version: response.Record.Version, Attempts: make([]workorchmanagementcontract.PreparationAttemptView, 0, len(response.Record.Attempts))}
		for _, c := range response.Record.Completions {
			view.Record.Completions = append(view.Record.Completions, workorchmanagementcontract.PreparationCompletionView{Version: c.Version, CompletionID: c.CompletionID, RequestID: c.RequestID, BootstrapHash: c.BootstrapHash, CreationEpoch: c.CreationEpoch, CompletedAt: c.CompletedAt, ExpiresAt: c.ExpiresAt, CommittedWorkspaceRevision: c.CommittedWorkspaceRevision})
		}
		for _, a := range response.Record.Attempts {
			binding := a.Binding
			attempt := workorchmanagementcontract.PreparationAttemptView{
				ExecutionInputVersion: a.ExecutionInputVersion,
				RequestID:             a.RequestID, PayloadHash: a.PayloadHash, ExpectedWorkspaceRevision: a.ExpectedWorkspaceRevision, CommittedWorkspaceRevision: a.CommittedWorkspaceRevision, StartedAt: a.StartedAt, State: string(a.State),
				CreationEpoch: a.CreationEpoch, PreparationEpoch: a.PreparationEpoch,
				DirectoryEvidenceHash:    a.DirectoryEvidenceHash,
				ResourceEvidenceHash:     a.ResourceEvidenceHash,
				ManagedConfigEvidenceRef: a.ManagedConfigEvidenceRef,
				SoulEvidenceRef:          a.SoulEvidenceRef,
				SealEvidenceRef:          a.SealEvidenceRef,
				SkillRegistryEvidenceRef: a.SkillRegistryEvidenceRef,
				SkillHubEffectRef:        a.SkillHubEffectRef, SkillHubEffectState: string(a.SkillHubEffectState),
				SkillHubInstalledState:          a.SkillHubInstalledState,
				SkillSelectionState:             a.SkillSelectionState,
				SkillContextState:               a.SkillContextState,
				SkillReferenceState:             a.SkillReferenceState,
				SkillStatusState:                a.SkillStatusState,
				ProviderCacheEvidenceRef:        a.ProviderCacheEvidenceRef,
				RunHistoryEvidenceRef:           a.RunHistoryEvidenceRef,
				MemoryInventoryEvidenceRef:      a.MemoryInventoryEvidenceRef,
				MCPRegistryEvidenceRef:          a.MCPRegistryEvidenceRef,
				EvaluationVersion:               a.EvaluationVersion,
				NoRunEvaluationVersion:          a.NoRunEvaluationVersion,
				EmptySelectionEvaluationVersion: a.EmptySelectionEvaluationVersion,
				Binding: workorchmanagementcontract.PreparationBindingView{
					InputHash: binding.AuthoringRef.InputHash, DerivationPermitID: binding.AuthoringRef.DerivationPermitID, ProposalHash: binding.AuthoringRef.ProposalHash, SpecificationHash: binding.AuthoringRef.SpecificationHash,
					CandidateID: binding.CandidateID, CandidateRevision: binding.CandidateRevision, CandidateHash: binding.CandidateHash, DefinitionRevision: binding.DefinitionRevision, StartupInputHash: binding.StartupInputHash,
					AuthoringConfigurationHash: binding.AuthoringConfigurationHash, SnapshotHash: binding.SnapshotHash, ManifestHash: binding.ManifestHash, CheckPolicyVersion: binding.CheckPolicyVersion, CompilerVersion: binding.CompilerVersion,
				},
			}
			if c := a.DefinitionCompilation; c != nil {
				attempt.DefinitionCompilation = &workorchmanagementcontract.PreparationDefinitionCompilationView{
					CompilerVersion: c.CompilerVersion, SnapshotHash: c.SnapshotHash, ManifestHash: c.ManifestHash,
					SourceInventoryHash: c.SourceInventoryHash, ProfileContractsHash: c.ProfileContractsHash,
					SourceCount: c.SourceCount, RequirementCount: c.RequirementCount, ObligationCount: c.ObligationCount, VerificationCount: c.VerificationCount,
				}
			}
			if a.Assessment != nil {
				s := a.Assessment
				assessment := workorchmanagementcontract.PreparationAssessmentView{Version: s.Version, ManifestHash: s.ManifestHash, Outcome: s.Outcome, RequiredCount: s.RequiredCount, PassedRequiredCount: s.PassedRequiredCount, Outstanding: make([]workorchmanagementcontract.PreparationOutstandingCheckView, 0, len(s.Outstanding))}
				for _, c := range s.Outstanding {
					assessment.Outstanding = append(assessment.Outstanding, workorchmanagementcontract.PreparationOutstandingCheckView{ID: c.ID, Owner: c.Owner, State: c.State, Reason: c.Reason})
				}
				attempt.Assessment = &assessment
			}
			for _, check := range a.Checks {
				attempt.Checks = append(attempt.Checks, workorchmanagementcontract.PreparationCheckView{ID: check.ID, Owner: check.Owner, Required: check.Required, State: string(check.State), Reason: check.Reason})
			}
			if !a.CompletedAt.IsZero() {
				completed := a.CompletedAt
				attempt.CompletedAt = &completed
			}
			if !a.ExpiresAt.IsZero() {
				expires := a.ExpiresAt
				attempt.ExpiresAt = &expires
			}
			view.Record.Attempts = append(view.Record.Attempts, attempt)
		}
	}
	if err := view.ValidateFor(string(response.WorkspaceID)); err != nil {
		return workorchmanagementcontract.StartupPreparationView{}, err
	}
	return view, nil
}

func (b *Biz) CheckStartupPreparation(ctx context.Context, workspaceID common.WorkspaceID, request workorchmanagementcontract.PreparationCheckRequest, recoverOriginal bool) (workorchmanagementcontract.PreparationOperationView, error) {
	if b == nil || b.EventHub() == nil || ctx == nil || workspaceID == "" || request.Validate() != nil {
		return workorchmanagementcontract.PreparationOperationView{}, common.ErrInvalidArgument
	}
	defer preparationtrace.Start(ctx, workspaceID, request.RequestID, "http_check", request.ExpectedWorkspaceRevision)()

	if err := ctx.Err(); err != nil {
		return workorchmanagementcontract.PreparationOperationView{}, err
	}
	command := workspaceevents.CheckWorkspacePreparationCommand{WorkspaceID: workspaceID, RequestID: request.RequestID, ExpectedWorkspaceRevision: request.ExpectedWorkspaceRevision}
	topic := workspaceevents.TopicWorkspacePreparationCheck
	if recoverOriginal {
		topic = workspaceevents.TopicWorkspacePreparationRecover
	}
	ev := magicevent.NewEventWithContext(topic, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(workspaceevents.Contracts(), topic, eventcontract.LaneScope{WorkspaceID: string(workspaceID)}))
	result := b.SendEvent(ev)
	if result == nil {
		return workorchmanagementcontract.PreparationOperationView{}, common.ErrConflict
	}
	if err := result.Error(); err != nil {
		return workorchmanagementcontract.PreparationOperationView{}, err
	}
	value, err := result.Get()
	if err != nil {
		return workorchmanagementcontract.PreparationOperationView{}, err
	}
	response, ok := value.(workspaceevents.CheckWorkspacePreparationResult)
	if !ok || response.ValidateFor(command) != nil {
		return workorchmanagementcontract.PreparationOperationView{}, common.ErrConflict
	}
	if response.ObservationRequired {
		if recoverOriginal {
			return workorchmanagementcontract.PreparationOperationView{}, common.ErrConflict
		}
		settled, observationErr := b.observeAndSettleBlockedPreparation(ctx, command, response.SkillHubPlan, response.ProviderDefinition)
		if observationErr != nil {
			return workorchmanagementcontract.PreparationOperationView{}, observationErr
		}
		response = settled
	}
	finishProjection := preparationtrace.Start(ctx, workspaceID, request.RequestID, "response_compilation", response.State.WorkspaceRevision)
	defer finishProjection()
	view, projectionErr := startupPreparationView(response.State)
	if projectionErr != nil {
		return workorchmanagementcontract.PreparationOperationView{}, projectionErr
	}
	operation := workorchmanagementcontract.PreparationOperationView{RequestID: response.RequestID, Preparation: view}
	// Only an explicit check/recovery consumes the original private archives
	// returned with this exact owner result. No extra read can mix a later
	// revision into the response; GET does not expose or compile this source.
	w, sourceErr := response.CompilationSource.Model()
	if sourceErr != nil || w.ID != workspaceID || w.StartupPreparation == nil {
		return workorchmanagementcontract.PreparationOperationView{}, common.ErrConflict
	}
	for _, a := range w.StartupPreparation.Attempts {
		if a.RequestID != command.RequestID || a.ExpectedWorkspaceRevision != command.ExpectedWorkspaceRevision {
			continue
		}
		compiled, compileErr := activationports.CompileOriginalPreparedBootstrap(w, a)
		if compileErr != nil {
			return workorchmanagementcontract.PreparationOperationView{}, compileErr
		}
		projection := &workorchmanagementcontract.PreparationBootstrapCompilationView{Version: compiled.Version, FailureCode: compiled.FailureCode, SnapshotHash: a.Binding.SnapshotHash, ManifestHash: a.Binding.ManifestHash}
		if compiled.Compilation != nil {
			projection.BootstrapHash = compiled.Compilation.BootstrapHash
		}
		operation.Compilation = projection
	}
	if operation.Compilation == nil {
		return workorchmanagementcontract.PreparationOperationView{}, common.ErrConflict
	}
	// Response compilation consumes real time. Downgrade newly expired original
	// observations before returning; never upgrade the owner's epoch-unknown or
	// stale projection and never acquire/renew any resource here.
	if err := projectPreparationResponseExpiry(&operation.Preparation, w, time.Now().UTC()); err != nil {
		return workorchmanagementcontract.PreparationOperationView{}, err
	}
	return operation, operation.ValidateFor(string(workspaceID), request)
}

func projectPreparationResponseExpiry(view *workorchmanagementcontract.StartupPreparationView, w *workspace.Workspace, now time.Time) error {
	if view.CurrentValidity == "stale" || w.StartupPreparation == nil {
		return nil
	}
	for _, c := range w.StartupPreparation.Completions {
		if c.RequestID == w.StartupPreparation.Attempts[len(w.StartupPreparation.Attempts)-1].RequestID && (now.Before(c.CompletedAt) || !now.Before(c.ExpiresAt)) {
			view.CurrentValidity, view.CurrentValidityReason = "stale", "completion_outside_window"
			return nil
		}
	}
	for _, check := range []struct {
		reason string
		stale  func() (bool, error)
	}{
		{"file_observation_stale", func() (bool, error) {
			return workspace.PreparationFileRevalidationStale(w, now, workspace.PreparationFileObservationEpoch(w))
		}},
		{"provider_cache_expired", func() (bool, error) { return workspace.PreparationProviderCacheExpired(w, now) }},
		{"owner_observation_stale", func() (bool, error) { return workspace.PreparationOwnerRevalidationStale(w, now) }},
	} {
		stale, err := check.stale()
		if err != nil {
			return err
		}
		if stale {
			view.CurrentValidity, view.CurrentValidityReason = "stale", check.reason
			return nil
		}
	}
	return nil
}
