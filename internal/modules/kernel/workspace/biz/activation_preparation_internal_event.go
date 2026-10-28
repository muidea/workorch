package workspace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"slices"
	"strings"
	"time"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	workspaceeventmapping "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/eventmapping"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/startuppreparation"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (m *Biz) handlePreparationCheck(ev magicevent.Event, result magicevent.Result) {
	if result == nil {
		return
	}
	if ev == nil || ev.Source() != workspaceevents.CommandSource || (ev.ID() != workspaceevents.TopicWorkspacePreparationCheck && ev.ID() != workspaceevents.TopicWorkspacePreparationRecover) {
		result.Set(nil, cd.WrapError(cd.IllegalParam, common.ErrInvalidArgument, "invalid preparation command source"))
		return
	}
	command, ok := ev.Data().(workspaceevents.CheckWorkspacePreparationCommand)
	if !ok || command.Validate() != nil {
		result.Set(nil, cd.WrapError(cd.IllegalParam, common.ErrInvalidArgument, "invalid preparation command"))
		return
	}
	value, err := m.beginWorkspacePreparation(ev.Context(), command, ev.ID() == workspaceevents.TopicWorkspacePreparationRecover, true)
	if err != nil {
		result.Set(nil, cd.WrapError(cd.Unexpected, err, "preparation operation is unavailable; reconcile original identity"))
		return
	}
	result.Set(value, nil)
}

// The owner-local path can still close an unsupported pipeline without I/O.
// Production uses begin -> application read-only queries -> negative settlement.
// Neither path can issue positive readiness or M3.
func (m *Biz) checkWorkspacePreparation(ctx context.Context, command workspaceevents.CheckWorkspacePreparationCommand, recoverOriginal bool) (workspaceevents.CheckWorkspacePreparationResult, error) {
	return m.beginWorkspacePreparation(ctx, command, recoverOriginal, false)
}

func (m *Biz) beginWorkspacePreparation(ctx context.Context, command workspaceevents.CheckWorkspacePreparationCommand, recoverOriginal, deferSettlement bool) (workspaceevents.CheckWorkspacePreparationResult, error) {
	if m == nil || m.workspaces == nil || ctx == nil || command.Validate() != nil {
		return workspaceevents.CheckWorkspacePreparationResult{}, common.ErrInvalidArgument
	}
	bound, err := common.BindRuntimeWorkspaceContext(ctx, command.WorkspaceID)
	if err != nil {
		return workspaceevents.CheckWorkspacePreparationResult{}, err
	}
	current, err := m.workspaces.Get(bound, command.WorkspaceID)
	if err != nil {
		return workspaceevents.CheckWorkspacePreparationResult{}, err
	}
	if workspaceDeletionRejectsMutation(current) != nil {
		return workspaceevents.CheckWorkspacePreparationResult{}, common.ErrConflict
	}
	index, err := preparationOriginalIndex(current, command)
	if err != nil {
		return workspaceevents.CheckWorkspacePreparationResult{}, err
	}
	if index >= 0 {
		// An ordinary duplicate is a readback, never a retry. Only explicit
		// recovery may settle an interrupted accepted/unknown operation.
		if !recoverOriginal || current.StartupPreparation.Attempts[index].State == workspace.WorkspacePreparationBlocked || current.StartupPreparation.Attempts[index].State == workspace.WorkspacePreparationFailed {
			return m.preparationCommandReadback(bound, current, command)
		}
		return m.finishBlockedPreparation(bound, workspacePreparationBlocked{Command: command, Reason: "original_check_interrupted", At: time.Now().UTC()})
	}
	if recoverOriginal {
		return workspaceevents.CheckWorkspacePreparationResult{}, common.ErrNotFound
	}
	if workspace.PreparationInitializationPending(current) {
		return workspaceevents.CheckWorkspacePreparationResult{}, common.ErrConflict
	}
	if workspace.ValidatePreparationSkillHubEffectsPersistence(current) != nil {
		return workspaceevents.CheckWorkspacePreparationResult{}, common.ErrConflict
	}
	for _, e := range current.PreparationSkillHubEffects {
		if e.Summary() == workspace.PreparationSkillHubReserved || e.Summary() == workspace.PreparationSkillHubUnresolved {
			return workspaceevents.CheckWorkspacePreparationResult{}, common.ErrConflict
		}
	}
	read, err := m.queryExactConfirmedStartupSnapshot(bound, workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: command.WorkspaceID, ExpectedWorkspaceRevision: command.ExpectedWorkspaceRevision})
	if err != nil {
		return workspaceevents.CheckWorkspacePreparationResult{}, err
	}
	manifest, err := startuppreparation.CompilePreparationManifest(read.Snapshot)
	if err != nil {
		return workspaceevents.CheckWorkspacePreparationResult{}, err
	}
	fact := workspacePreparationAccepted{Command: command, Snapshot: read.Snapshot, Manifest: manifest, At: time.Now().UTC()}
	if deferSettlement {
		var nonce [16]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			return workspaceevents.CheckWorkspacePreparationResult{}, err
		}
		fact.SkillHubRef = workspace.PreparationSkillHubRefPrefix + hex.EncodeToString(nonce[:])
	}
	// Re-read after exact compilation. Only the winning initial CAS owns
	// this continuation; an ambiguous write must be reconciled, not retried.
	current, accepted, err := m.acceptPreparation(bound, fact)
	if err != nil {
		return workspaceevents.CheckWorkspacePreparationResult{}, err
	}
	if !accepted {
		return m.preparationCommandReadback(bound, current, command)
	}
	if deferSettlement {
		response, err := m.preparationCommandReadback(bound, current, command)
		response.ObservationRequired = err == nil
		if err == nil {
			definition := workspace.PreparationProviderDefinition(current)
			definition.ConfigurationHash = current.StartupPreparation.Attempts[len(current.StartupPreparation.Attempts)-1].Binding.AuthoringConfigurationHash
			response.ProviderDefinition = &definition
		}
		if err == nil && len(current.PreparationSkillHubEffects) > 0 {
			plan := current.PreparationSkillHubEffects[len(current.PreparationSkillHubEffects)-1].Clone()
			response.SkillHubPlan = &plan
			if err = response.ValidateFor(command); err != nil {
				return workspaceevents.CheckWorkspacePreparationResult{}, err
			}
		}
		return response, err
	}
	return m.finishBlockedPreparation(bound, workspacePreparationBlocked{Command: command, Reason: "check_pipeline_incomplete", At: time.Now().UTC()})
}

func (m *Biz) handlePreparationBlockedSettlement(ev magicevent.Event, result magicevent.Result) {
	if result == nil {
		return
	}
	if ev == nil || ev.Source() != workspaceevents.CommandSource || ev.ID() != workspaceevents.TopicWorkspacePreparationSettleBlocked {
		result.Set(nil, cd.WrapError(cd.IllegalParam, common.ErrInvalidArgument, "invalid preparation settlement source"))
		return
	}
	c, ok := ev.Data().(workspaceevents.SettleWorkspacePreparationBlockedCommand)
	if !ok || c.Validate() != nil {
		result.Set(nil, cd.WrapError(cd.IllegalParam, common.ErrInvalidArgument, "invalid preparation settlement"))
		return
	}
	ctx, err := common.BindRuntimeWorkspaceContext(ev.Context(), c.Original.WorkspaceID)
	if err != nil {
		result.Set(nil, cd.WrapError(cd.IllegalParam, err, "invalid preparation scope"))
		return
	}
	souls, soulReason := m.bindOriginalPreparationSouls(ctx, c)
	registry, registryReason := m.bindOriginalPreparationSkillRegistry(ctx, c)
	provider := m.bindOriginalPreparationProviderCache(ctx, c)
	runHistory := m.bindOriginalPreparationRunHistory(ctx, c)
	memoryInventory := m.bindOriginalPreparationMemoryInventory(ctx, c)
	mcpRegistry := m.bindOriginalPreparationMCPRegistry(ctx, c)
	mcpReason := c.MCPRegistryReason
	if mcpRegistry == nil && c.MCPRegistryFacts != nil {
		mcpReason = "mcp_registry_observation_unavailable_not_saved"
	}
	memoryReason := c.MemoryInventoryReason
	if memoryInventory == nil && c.MemoryInventoryFacts != nil {
		memoryReason = "memory_inventory_unavailable_not_saved"
	}
	evidence, directoryReason := m.observeOriginalPreparationDirectories(ctx, c.Original)
	resources := m.observeOriginalPreparationResources(ctx, c.Original)
	configs, configReason := m.observeOriginalPreparationManagedConfigs(ctx, c.Original)
	seal := m.observeOriginalPreparationSeal(ctx, c.Original)
	if seal != nil && c.FinalOwners != nil {
		f := c.FinalOwners.Clone()
		seal.FinalOwners = &f
		seal.CompletedAt = time.Now().UTC()
		seal.ContentHash = seal.Hash()
	}
	m.reobservePreparationFiles(ctx, evidence, resources, configs)
	value, err := m.finishBlockedPreparation(ctx, workspacePreparationBlocked{ProcessPolicyFacts: c.ProcessPolicyFacts, Command: c.Original, Reason: "check_pipeline_incomplete", SealEvidence: seal, MCPRegistryEvidence: mcpRegistry, MCPRegistryReason: mcpReason, ProviderCacheReason: c.ProviderCacheReason, ProviderCacheEvidence: provider, MemoryInventoryEvidence: memoryInventory, MemoryInventoryReason: memoryReason, RunHistoryReason: c.RunHistoryReason, RunHistoryEvidence: runHistory, DirectoryEvidence: evidence, DirectoryReason: directoryReason, ResourceEvidence: resources, ManagedConfigEvidence: configs, ManagedConfigReason: configReason, SoulEvidence: souls, SoulReason: soulReason, SkillRegistryEvidence: registry, SkillRegistryReason: registryReason, SkillHubEffectRef: c.SkillHubEffectRef, SkillHubGlobal: c.SkillHubGlobal, SkillHubProject: c.SkillHubProject, At: time.Now().UTC()})
	if err != nil {
		result.Set(nil, cd.WrapError(cd.Unexpected, err, "preparation settlement unavailable; reconcile original identity"))
		return
	}
	result.Set(value, nil)
}

func (m *Biz) acceptPreparation(ctx context.Context, fact workspacePreparationAccepted) (*workspace.Workspace, bool, error) {
	m.deletionIngressMu.RLock()
	defer m.deletionIngressMu.RUnlock()
	m.manager.definitionMu.Lock()
	defer m.manager.definitionMu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	current, err := m.workspaces.Get(ctx, fact.Command.WorkspaceID)
	if err != nil {
		return nil, false, err
	}
	transition, err := reduceWorkspacePreparationAccepted(current, fact)
	if err != nil {
		return nil, false, err
	}
	if transition.Noop {
		return current, false, nil
	}
	if err := m.workspaces.CompareAndSwap(ctx, transition.Next, current.Revision); err != nil {
		return nil, false, err
	}
	m.publishWorkspaceUpdated(ctx, transition.Next)
	return transition.Next, true, nil
}

// Private facts, not public commands: callers cannot choose mutations or
// submit a manifest/verdict. The owner constructs these from its exact rows.
type workspacePreparationAccepted struct {
	Command     workspaceevents.CheckWorkspacePreparationCommand
	Snapshot    startuppreparation.ExactConfirmedStartupSnapshot
	Manifest    startuppreparation.Manifest
	At          time.Time
	SkillHubRef string
}

type workspacePreparationBlocked struct {
	ProcessPolicyFacts              *workspace.PreparationProcessPolicyFacts
	SealEvidence                    *workspace.WorkspacePreparationSealEvidence
	MCPRegistryEvidence             *workspace.WorkspacePreparationMCPRegistryEvidence
	MCPRegistryReason               string
	Command                         workspaceevents.CheckWorkspacePreparationCommand
	Reason                          string
	At                              time.Time
	ProviderCacheReason             string
	ProviderCacheEvidence           *workspace.WorkspacePreparationProviderCacheEvidence
	MemoryInventoryReason           string
	MemoryInventoryEvidence         *workspace.WorkspacePreparationMemoryInventoryEvidence
	RunHistoryReason                string
	RunHistoryEvidence              *workspace.WorkspacePreparationRunHistoryEvidence
	DirectoryEvidence               *workspace.WorkspacePreparationDirectoryEvidence
	DirectoryReason                 string
	ResourceEvidence                *workspace.WorkspacePreparationResourceEvidence
	ManagedConfigEvidence           *workspace.WorkspacePreparationManagedConfigEvidence
	ManagedConfigReason             string
	SoulEvidence                    *workspace.WorkspacePreparationSoulEvidence
	SoulReason                      string
	SkillRegistryEvidence           *workspace.WorkspacePreparationSkillRegistryEvidence
	SkillRegistryReason             string
	SkillHubEffectRef               string
	SkillHubGlobal, SkillHubProject *workspace.PreparationSkillHubStatusFacts
}

func reduceWorkspacePreparationAccepted(current *workspace.Workspace, fact workspacePreparationAccepted) (workspaceExecutionTransition, error) {
	if current == nil || fact.Command.Validate() != nil || current.ID != fact.Command.WorkspaceID || fact.At.IsZero() || workspaceDeletionRejectsMutation(current) != nil {
		return workspaceExecutionTransition{}, common.ErrInvalidArgument
	}
	index, err := preparationOriginalIndex(current, fact.Command)
	if err != nil {
		return workspaceExecutionTransition{}, err
	}
	if index >= 0 {
		return workspaceExecutionTransition{Next: current, Noop: true}, nil
	}
	fence := current.PreparationFence
	if workspace.ValidatePreparationSkillHubEffectsPersistence(current) != nil {
		return workspaceExecutionTransition{}, common.ErrConflict
	}
	for _, e := range current.PreparationSkillHubEffects {
		if e.Summary() != workspace.PreparationSkillHubRecorded {
			return workspaceExecutionTransition{}, common.ErrConflict
		}
	}
	if current.Revision != fact.Command.ExpectedWorkspaceRevision || current.Revision == ^uint64(0) || fence == nil || fence.Validate(current.ID) != nil || fact.Snapshot.WorkspaceID != current.ID || fact.Snapshot.WorkspaceRevision != current.Revision || fact.Manifest.ValidateFor(fact.Snapshot) != nil {
		return workspaceExecutionTransition{}, common.ErrConflict
	}
	// Revision is necessary but not a substitute for the complete current
	// persisted seal/configuration binding. Preserve the original detached
	// formal rows; never substitute current inputs after a stale observation.
	fresh, err := startuppreparation.FromOwnerPersistence(current, &fact.Snapshot.Goal, &fact.Snapshot.DoD)
	if err != nil || fresh.SnapshotHash != fact.Snapshot.SnapshotHash {
		return workspaceExecutionTransition{}, common.ErrConflict
	}
	record := workspace.WorkspacePreparationRecord{Version: workspace.WorkspacePreparationWorkflowVersion, WorkspaceID: current.ID}
	if current.StartupPreparation != nil {
		if current.StartupPreparation.Version != workspace.WorkspacePreparationWorkflowVersion || len(current.StartupPreparation.Attempts) >= workspace.WorkspacePreparationAttemptLimit {
			return workspaceExecutionTransition{}, common.ErrConflict
		}
		record = current.StartupPreparation.Clone()
		last := record.Attempts[len(record.Attempts)-1]
		if last.State == workspace.WorkspacePreparationAccepted || last.State == workspace.WorkspacePreparationUnknown {
			return workspaceExecutionTransition{}, common.ErrConflict
		}
	}
	s := fact.Snapshot
	observedPreimage := *current
	observedPreimage.Revision++
	observationSnapshot, err := startuppreparation.FromOwnerPersistence(&observedPreimage, &s.Goal, &s.DoD)
	if err != nil {
		return workspaceExecutionTransition{}, err
	}
	attempt := workspace.WorkspacePreparationAttempt{
		OwnerRevalidationVersion: workspace.PreparationOwnerRevalidationVersion,
		RequestID:                fact.Command.RequestID, PayloadHash: fact.Command.PayloadHash(), ExpectedWorkspaceRevision: current.Revision, CommittedWorkspaceRevision: current.Revision + 1,
		Binding: workspace.WorkspacePreparationBinding{
			AuthoringRef: s.AuthoringRef, CandidateID: s.Candidate.CandidateID, CandidateRevision: s.Candidate.CandidateRevision, CandidateHash: s.Candidate.CandidateHash,
			DefinitionRevision: s.StartupInput.DefinitionRevision, StartupInputHash: s.StartupInput.StartupInputHash, AuthoringConfigurationHash: s.AuthoringConfigurationHash,
			SnapshotHash: s.SnapshotHash, ManifestHash: fact.Manifest.ManifestHash, CheckPolicyVersion: fact.Manifest.PolicyVersion, CompilerVersion: startuppreparation.BootstrapCompilerVersion,
		},
		StartedAt: fact.At, State: workspace.WorkspacePreparationAccepted, CreationEpoch: fence.CreationEpoch, PreparationEpoch: fence.PreparationEpoch, EvaluationVersion: workspace.PreparationCheckEvaluationVersion, ObservationSnapshotHash: observationSnapshot.SnapshotHash,
	}
	for _, check := range fact.Manifest.Checks {
		value := workspace.WorkspacePreparationCheckResult{ID: string(check.ID), Owner: check.Owner, Required: check.Required, State: "pending", Reason: "not_observed"}
		if check.ID == startuppreparation.CheckDirectory || check.ID == startuppreparation.CheckManagedConfigs || strings.HasPrefix(string(check.ID), "resource/") {
			value.SubjectHash = workspace.PreparationFileSubjectHash(string(check.ID), s.InternalDirectory, s.StartupInput.ResolvedProjectWorkspaceDirectory, check.SubjectRef, check.ExpectedContentHash)
		}
		attempt.Checks = append(attempt.Checks, value)
	}
	var effects *workspace.WorkspacePreparationSkillHubEffects
	if fact.SkillHubRef != "" {
		if !workspace.ValidPreparationSkillHubRef(fact.SkillHubRef) {
			return workspaceExecutionTransition{}, common.ErrConflict
		}
		e := workspace.WorkspacePreparationSkillHubEffects{Version: workspace.PreparationSkillHubEffectsVersion, Ref: fact.SkillHubRef, WorkspaceID: current.ID, RequestID: attempt.RequestID, PayloadHash: attempt.PayloadHash, ExpectedWorkspaceRevision: attempt.ExpectedWorkspaceRevision, Binding: attempt.Binding, ProjectDirectory: s.StartupInput.ResolvedProjectWorkspaceDirectory, ReservedAt: fact.At}
		e.ContentHash = e.Hash()
		if e.Validate() != nil {
			return workspaceExecutionTransition{}, common.ErrConflict
		}
		effects = &e
		attempt.SkillHubEffectRef, attempt.SkillHubEffectState = e.Ref, workspace.PreparationSkillHubReserved
	}
	record.Attempts = append(record.Attempts, attempt)
	if record.Validate(current.ID) != nil {
		return workspaceExecutionTransition{}, common.ErrConflict
	}
	next := *current
	next.StartupPreparation = &record
	if effects != nil {
		next.PreparationSkillHubEffects = append(workspace.ClonePreparationSkillHubEffects(current.PreparationSkillHubEffects), effects.Clone())
	}
	if workspace.ValidatePreparationEvidencePersistence(&next) != nil {
		return workspaceExecutionTransition{}, common.ErrConflict
	}
	return workspaceExecutionTransition{Next: &next}, nil
}

func preparationOriginalIndex(w *workspace.Workspace, command workspaceevents.CheckWorkspacePreparationCommand) (int, error) {
	if w.StartupPreparation == nil {
		return -1, nil
	}
	if w.StartupPreparation.Validate(w.ID) != nil {
		return -1, common.ErrConflict
	}
	for i, a := range w.StartupPreparation.Attempts {
		if a.RequestID == command.RequestID {
			if a.PayloadHash != command.PayloadHash() || a.ExpectedWorkspaceRevision != command.ExpectedWorkspaceRevision {
				return -1, common.ErrConflict
			}
			return i, nil
		}
	}
	return -1, nil
}

func (m *Biz) finishBlockedPreparation(ctx context.Context, fact workspacePreparationBlocked) (workspaceevents.CheckWorkspacePreparationResult, error) {
	command := fact.Command
	m.deletionIngressMu.RLock()
	defer m.deletionIngressMu.RUnlock()
	for retry := 0; retry < 4; retry++ {
		if err := ctx.Err(); err != nil {
			return workspaceevents.CheckWorkspacePreparationResult{}, err
		}
		current, err := m.workspaces.Get(ctx, command.WorkspaceID)
		if err != nil {
			return workspaceevents.CheckWorkspacePreparationResult{}, err
		}
		transition, err := reduceWorkspacePreparationBlocked(current, fact)
		if err != nil {
			return workspaceevents.CheckWorkspacePreparationResult{}, err
		}
		if transition.Noop {
			return m.preparationCommandReadback(ctx, current, command)
		}
		if err := m.workspaces.CompareAndSwap(ctx, transition.Next, current.Revision); err != nil {
			if errors.Is(err, common.ErrConflict) {
				continue
			}
			return workspaceevents.CheckWorkspacePreparationResult{}, err
		}
		m.publishWorkspaceUpdated(ctx, transition.Next)
		return m.preparationCommandReadback(ctx, transition.Next, command)
	}
	return workspaceevents.CheckWorkspacePreparationResult{}, common.ErrConflict
}

func reduceWorkspacePreparationBlocked(current *workspace.Workspace, fact workspacePreparationBlocked) (workspaceExecutionTransition, error) {
	command, reason, at := fact.Command, fact.Reason, fact.At
	if current == nil || current.ID != command.WorkspaceID || command.Validate() != nil || workspaceDeletionRejectsMutation(current) != nil || at.IsZero() || (reason != "check_pipeline_incomplete" && reason != "original_check_interrupted") {
		return workspaceExecutionTransition{}, common.ErrInvalidArgument
	}
	if fact.SealEvidence != nil && reason != "check_pipeline_incomplete" {
		return workspaceExecutionTransition{}, common.ErrConflict
	}
	if e := fact.RunHistoryEvidence; e != nil {
		if reason != "check_pipeline_incomplete" || e.Validate() != nil || e.Facts.Observation.History.WorkspaceID != current.ID || e.RequestID != command.RequestID || e.PayloadHash != command.PayloadHash() || e.Facts.WorkspaceRevision != command.ExpectedWorkspaceRevision+1 || e.Facts.Observation.ObservedAt.After(at) || fact.RunHistoryReason != e.Facts.BlockedReason() {
			return workspaceExecutionTransition{}, common.ErrConflict
		}
	}
	if (fact.ProviderCacheReason != "" || fact.RunHistoryReason != "") && (reason != "check_pipeline_incomplete" || (workspaceevents.SettleWorkspacePreparationBlockedCommand{Original: command, ProviderCacheReason: fact.ProviderCacheReason, RunHistoryReason: fact.RunHistoryReason}).Validate() != nil) {
		return workspaceExecutionTransition{}, common.ErrInvalidArgument
	}
	if fact.DirectoryReason != "" && !workspace.ValidPreparationBlockedReason("directory_boundary", fact.DirectoryReason) {
		return workspaceExecutionTransition{}, common.ErrInvalidArgument
	}
	if fact.MCPRegistryReason != "" && !workspace.ValidPreparationBlockedReason("mcp_selection", fact.MCPRegistryReason) {
		return workspaceExecutionTransition{}, common.ErrInvalidArgument
	}
	if e := fact.MCPRegistryEvidence; e != nil {
		if reason != "check_pipeline_incomplete" || e.Validate() != nil || e.Facts.WorkspaceID != current.ID || e.RequestID != command.RequestID || e.PayloadHash != command.PayloadHash() || e.Facts.WorkspaceRevision != command.ExpectedWorkspaceRevision+1 || e.Facts.CompletedAt.After(at) || fact.MCPRegistryReason != "mcp_registry_observed_connection_pending" {
			return workspaceExecutionTransition{}, common.ErrConflict
		}
	} else if fact.MCPRegistryReason == "mcp_registry_observed_connection_pending" {
		return workspaceExecutionTransition{}, common.ErrConflict
	}
	if fact.MemoryInventoryReason != "" && !workspace.ValidPreparationBlockedReason("memory_selection", fact.MemoryInventoryReason) {
		return workspaceExecutionTransition{}, common.ErrInvalidArgument
	}
	if e := fact.MemoryInventoryEvidence; e != nil {
		if reason != "check_pipeline_incomplete" || e.Validate() != nil || e.Facts.Observation.WorkspaceID != current.ID || e.RequestID != command.RequestID || e.PayloadHash != command.PayloadHash() || e.Facts.WorkspaceRevision != command.ExpectedWorkspaceRevision+1 || e.Facts.Observation.ObservedAt.After(at) || fact.MemoryInventoryReason != e.Facts.BlockedReason() {
			return workspaceExecutionTransition{}, common.ErrConflict
		}
	} else if fact.MemoryInventoryReason == "memory_inventory_observed_selection_pending" {
		return workspaceExecutionTransition{}, common.ErrConflict
	}
	if e := fact.ProviderCacheEvidence; e != nil {
		if reason != "check_pipeline_incomplete" || e.Validate() != nil || e.Facts.Observation.WorkspaceID != current.ID || e.RequestID != command.RequestID || e.PayloadHash != command.PayloadHash() || e.Facts.WorkspaceRevision != command.ExpectedWorkspaceRevision+1 || e.Facts.Observation.ObservedAt.After(at) || fact.ProviderCacheReason != e.Facts.BlockedReason() {
			return workspaceExecutionTransition{}, common.ErrConflict
		}
	}
	if fact.SkillRegistryReason != "" && !workspace.ValidPreparationBlockedReason("skill_selection", fact.SkillRegistryReason) {
		return workspaceExecutionTransition{}, common.ErrInvalidArgument
	}
	if e := fact.SkillRegistryEvidence; e != nil {
		if reason != "check_pipeline_incomplete" || e.Validate() != nil || e.Facts.WorkspaceID != current.ID || e.RequestID != command.RequestID || e.PayloadHash != command.PayloadHash() || e.Facts.WorkspaceRevision != command.ExpectedWorkspaceRevision+1 || e.Facts.CompletedAt.After(at) || fact.SkillRegistryReason != "skill_registry_observed_selection_pending" {
			return workspaceExecutionTransition{}, common.ErrConflict
		}
	} else if fact.SkillRegistryReason == "skill_registry_observed_selection_pending" {
		return workspaceExecutionTransition{}, common.ErrConflict
	}
	if fact.SoulReason != "" && !workspace.ValidPreparationBlockedReason("soul_selection", fact.SoulReason) {
		return workspaceExecutionTransition{}, common.ErrInvalidArgument
	}
	if e := fact.SoulEvidence; e != nil {
		if reason != "check_pipeline_incomplete" || e.Validate() != nil || e.Facts.WorkspaceID != current.ID || e.RequestID != command.RequestID || e.PayloadHash != command.PayloadHash() || e.Facts.WorkspaceRevision != command.ExpectedWorkspaceRevision+1 || e.Facts.CompletedAt.After(at) || fact.SoulReason != "souls_observed_remaining_checks" {
			return workspaceExecutionTransition{}, common.ErrConflict
		}
	} else if fact.SoulReason == "souls_observed_remaining_checks" {
		return workspaceExecutionTransition{}, common.ErrConflict
	}
	if fact.DirectoryEvidence != nil {
		e := fact.DirectoryEvidence
		if reason != "check_pipeline_incomplete" || e.Validate() != nil || e.WorkspaceID != current.ID || e.RequestID != command.RequestID || e.PayloadHash != command.PayloadHash() || e.ObservedWorkspaceRevision != command.ExpectedWorkspaceRevision+1 || e.CompletedAt.After(at) || fact.DirectoryReason != e.BlockedReason() {
			return workspaceExecutionTransition{}, common.ErrConflict
		}
	}
	if e := fact.ResourceEvidence; e != nil {
		if reason != "check_pipeline_incomplete" || e.Validate() != nil || e.WorkspaceID != current.ID || e.RequestID != command.RequestID || e.PayloadHash != command.PayloadHash() || e.ObservedWorkspaceRevision != command.ExpectedWorkspaceRevision+1 || e.CompletedAt.After(at) {
			return workspaceExecutionTransition{}, common.ErrConflict
		}
	}
	if fact.ManagedConfigReason != "" && !workspace.ValidPreparationBlockedReason("managed_configuration", fact.ManagedConfigReason) {
		return workspaceExecutionTransition{}, common.ErrInvalidArgument
	}
	if e := fact.ManagedConfigEvidence; e != nil {
		if reason != "check_pipeline_incomplete" || e.Validate() != nil || e.WorkspaceID != current.ID || e.RequestID != command.RequestID || e.PayloadHash != command.PayloadHash() || e.ObservedWorkspaceRevision != command.ExpectedWorkspaceRevision+1 || e.CompletedAt.After(at) || fact.ManagedConfigReason != e.BlockedReason() {
			return workspaceExecutionTransition{}, common.ErrConflict
		}
	}
	i, err := preparationOriginalIndex(current, command)
	if err != nil || i < 0 || current.StartupPreparation.Version != workspace.WorkspacePreparationWorkflowVersion {
		return workspaceExecutionTransition{}, common.ErrConflict
	}
	a := current.StartupPreparation.Attempts[i]
	if a.State == workspace.WorkspacePreparationBlocked || a.State == workspace.WorkspacePreparationFailed {
		return workspaceExecutionTransition{Next: current, Noop: true}, nil
	}
	if i != len(current.StartupPreparation.Attempts)-1 || current.Revision == ^uint64(0) {
		return workspaceExecutionTransition{}, common.ErrConflict
	}
	if f := fact.ProcessPolicyFacts; f != nil && (reason != "check_pipeline_incomplete" || f.Validate() != nil || f.WorkspaceID != current.ID || f.WorkspaceRevision != command.ExpectedWorkspaceRevision+1 || f.StartedAt.Before(a.StartedAt) || f.CompletedAt.After(at)) {
		return workspaceExecutionTransition{}, common.ErrConflict
	}
	var completedEffects *workspace.WorkspacePreparationSkillHubEffects
	if a.SkillHubEffectRef != "" {
		if workspace.ValidatePreparationSkillHubEffectsPersistence(current) != nil || len(current.PreparationSkillHubEffects) == 0 {
			return workspaceExecutionTransition{}, common.ErrConflict
		}
		e := current.PreparationSkillHubEffects[len(current.PreparationSkillHubEffects)-1].Clone()
		if e.Ref != a.SkillHubEffectRef || !e.CompletedAt.IsZero() || fact.SkillHubEffectRef != "" && fact.SkillHubEffectRef != e.Ref || fact.SkillHubEffectRef == "" && (fact.SkillHubGlobal != nil || fact.SkillHubProject != nil) {
			return workspaceExecutionTransition{}, common.ErrConflict
		}
		e.CompletedAt = at
		if fact.SkillHubGlobal != nil {
			f := fact.SkillHubGlobal.Clone()
			e.Global = &f
		}
		if fact.SkillHubProject != nil {
			f := fact.SkillHubProject.Clone()
			e.Project = &f
		}
		e.ContentHash = e.Hash()
		if e.Validate() != nil {
			return workspaceExecutionTransition{}, common.ErrConflict
		}
		completedEffects = &e
	} else if fact.SkillHubEffectRef != "" || fact.SkillHubGlobal != nil || fact.SkillHubProject != nil {
		return workspaceExecutionTransition{}, common.ErrConflict
	}
	if current.Revision != a.CommittedWorkspaceRevision || current.PreparationFence == nil || current.PreparationFence.Validate(current.ID) != nil || current.PreparationFence.CreationEpoch != a.CreationEpoch || current.PreparationFence.PreparationEpoch != a.PreparationEpoch {
		reason = "binding_changed"
	}
	record := current.StartupPreparation.Clone()
	a = record.Attempts[i]
	a.State, a.CompletedAt, a.CommittedWorkspaceRevision = workspace.WorkspacePreparationBlocked, at, current.Revision+1
	if completedEffects != nil {
		a.SkillHubEffectState = completedEffects.Summary()
		a.SkillHubInstalledState = completedEffects.InstalledSummary()
	}
	for n := range a.Checks {
		a.Checks[n].State, a.Checks[n].Reason = "blocked", reason
		if reason == "check_pipeline_incomplete" && fact.MCPRegistryReason != "" && a.Checks[n].ID == "mcp_selection" {
			a.Checks[n].Reason = fact.MCPRegistryReason
		}
		if reason == "check_pipeline_incomplete" && fact.MemoryInventoryReason != "" && a.Checks[n].ID == "memory_selection" {
			a.Checks[n].Reason = fact.MemoryInventoryReason
		}
		// Negative query classifications are never retained as current proof if
		// the exact owner revision or reservation epochs changed during I/O.
		if reason == "check_pipeline_incomplete" && fact.ProviderCacheReason != "" {
			switch a.Checks[n].ID {
			case string(startuppreparation.CheckProviderCache):
				a.Checks[n].Reason = fact.ProviderCacheReason
			case string(startuppreparation.CheckNoRun):
				a.Checks[n].Reason = fact.RunHistoryReason
			}
		}
		if reason == "check_pipeline_incomplete" && fact.DirectoryReason != "" && a.Checks[n].ID == "directory_boundary" {
			a.Checks[n].Reason = fact.DirectoryReason
		}
		if reason == "check_pipeline_incomplete" && fact.ManagedConfigReason != "" && a.Checks[n].ID == "managed_configuration" {
			a.Checks[n].Reason = fact.ManagedConfigReason
		}
		if reason == "check_pipeline_incomplete" && fact.SoulReason != "" && a.Checks[n].ID == "soul_selection" {
			a.Checks[n].Reason = fact.SoulReason
		}
		if reason == "check_pipeline_incomplete" && fact.SkillRegistryReason != "" && a.Checks[n].ID == "skill_selection" {
			a.Checks[n].Reason = fact.SkillRegistryReason
		}
		if reason == "check_pipeline_incomplete" && strings.HasPrefix(a.Checks[n].ID, "resource/") {
			a.Checks[n].Reason = "resource_observation_unavailable"
			if fact.ResourceEvidence != nil {
				for _, r := range fact.ResourceEvidence.Resources {
					if a.Checks[n].ID == "resource/"+r.ID {
						a.Checks[n].Reason = r.BlockedReason()
					}
				}
			}
		}
	}
	if fact.DirectoryEvidence != nil {
		a.DirectoryEvidenceHash = fact.DirectoryEvidence.ContentHash
	}
	if fact.ResourceEvidence != nil {
		a.ResourceEvidenceHash = fact.ResourceEvidence.ContentHash
	}
	if fact.ManagedConfigEvidence != nil {
		a.ManagedConfigEvidenceRef = fact.ManagedConfigEvidence.Ref
	}
	if fact.SoulEvidence != nil {
		a.SoulEvidenceRef = fact.SoulEvidence.Ref
	}
	if fact.ProviderCacheEvidence != nil {
		a.ProviderCacheEvidenceRef = fact.ProviderCacheEvidence.Ref
	}
	if fact.RunHistoryEvidence != nil {
		a.RunHistoryEvidenceRef = fact.RunHistoryEvidence.Ref
	}
	if fact.MemoryInventoryEvidence != nil {
		a.MemoryInventoryEvidenceRef = fact.MemoryInventoryEvidence.Ref
	}
	if fact.MCPRegistryEvidence != nil {
		a.MCPRegistryEvidenceRef = fact.MCPRegistryEvidence.Ref
	}
	if fact.SkillRegistryEvidence != nil {
		a.SkillRegistryEvidenceRef = fact.SkillRegistryEvidence.Ref
	}
	record.Attempts[i] = a
	if record.Validate(current.ID) != nil {
		return workspaceExecutionTransition{}, common.ErrConflict
	}
	next := *current
	next.StartupPreparation = &record
	if fact.RunHistoryEvidence != nil {
		next.PreparationRunHistoryEvidence = append(workspace.ClonePreparationRunHistoryEvidence(current.PreparationRunHistoryEvidence), fact.RunHistoryEvidence.Clone())
	}
	if fact.MCPRegistryEvidence != nil {
		next.PreparationMCPRegistryEvidence = append(workspace.ClonePreparationMCPRegistryEvidence(current.PreparationMCPRegistryEvidence), fact.MCPRegistryEvidence.Clone())
	}
	if fact.MemoryInventoryEvidence != nil {
		next.PreparationMemoryInventoryEvidence = append(workspace.ClonePreparationMemoryInventoryEvidence(current.PreparationMemoryInventoryEvidence), fact.MemoryInventoryEvidence.Clone())
	}
	if completedEffects != nil {
		next.PreparationSkillHubEffects = workspace.ClonePreparationSkillHubEffects(current.PreparationSkillHubEffects)
		next.PreparationSkillHubEffects[len(next.PreparationSkillHubEffects)-1] = completedEffects.Clone()
	}
	if fact.SoulEvidence != nil {
		next.PreparationSoulEvidence = append(workspace.ClonePreparationSoulEvidence(current.PreparationSoulEvidence), fact.SoulEvidence.Clone())
	}
	if fact.ProviderCacheEvidence != nil {
		next.PreparationProviderCacheEvidence = append(workspace.ClonePreparationProviderCacheEvidence(current.PreparationProviderCacheEvidence), fact.ProviderCacheEvidence.Clone())
		if reason == "check_pipeline_incomplete" && fact.ProviderCacheEvidence.Facts.Observation.Configuration != nil {
			provider := &next.PreparationProviderCacheEvidence[len(next.PreparationProviderCacheEvidence)-1]
			definition := workspace.PreparationProviderDefinition(current)
			definition.ConfigurationHash = a.Binding.AuthoringConfigurationHash
			compiled := workspace.CompilePreparationProviderSelection(definition, *provider)
			provider.Selection = &compiled
			provider.ContentHash = provider.Hash()
			for n := range a.Checks {
				if a.Checks[n].ID == "provider_cache" {
					a.Checks[n].Reason = provider.BlockedReason()
				}
			}
			record.Attempts[i] = a
			next.StartupPreparation = &record
		}
	}
	if fact.SkillRegistryEvidence != nil {
		next.PreparationSkillRegistryEvidence = append(workspace.ClonePreparationSkillRegistryEvidence(current.PreparationSkillRegistryEvidence), fact.SkillRegistryEvidence.Clone())
	}
	// Only this original owner settlement compiles selection. Recovery has no
	// registry facts and cannot backfill it. A stale final fence never earns a
	// compilation. Formal rows were independently exact-read by both binders;
	// their immutable journal images preserve original Goal/DoD order here.
	if reason == "check_pipeline_incomplete" && fact.SkillRegistryEvidence != nil && completedEffects != nil && current.DefinitionAuthoringConfirmation != nil && current.StartupInput != nil {
		j, input := current.DefinitionAuthoringConfirmation, current.StartupInput
		definition := workspace.PreparationSkillSelectionDefinition{WorkspaceID: current.ID, StartupInputHash: input.StartupInputHash, DefinitionRevision: input.DefinitionRevision, SnapshotHash: fact.SkillRegistryEvidence.Facts.SnapshotHash, GoalText: j.Goal.Text, DoDItems: slices.Clone(j.DoD.Items), GoalTextHash: input.GoalTextHash, DoDItemSetHash: input.DoDItemSetHash}
		registry := &next.PreparationSkillRegistryEvidence[len(next.PreparationSkillRegistryEvidence)-1]
		compiled := workspace.CompilePreparationSkillSelection(definition, *registry, fact.SoulEvidence, completedEffects)
		if compiled.FailureCode == "" {
			context := workspace.CompilePreparationSkillContext(compiled.Selected)
			compiled.Context = &context
			a.SkillContextState = context.Summary()
			references := workspace.CompilePreparationSkillReferences(compiled, completedEffects)
			compiled.References = &references
			a.SkillReferenceState = references.State
			status := workspace.CompilePreparationSkillStatus(compiled, completedEffects)
			compiled.Status = &status
			a.SkillStatusState = status.State
		}
		registry.Selection = &compiled
		registry.ContentHash = registry.Hash()
		a.SkillSelectionState = compiled.Summary()
		for n := range a.Checks {
			if a.Checks[n].ID == "skill_selection" {
				a.Checks[n].Reason = "skill_selection_" + a.SkillSelectionState
			}
		}
		record.Attempts[i] = a
		next.StartupPreparation = &record
	}
	// Compile only on the original non-stale settlement, after complete Skill
	// selection has been compiled. Missing sources stay an explicit failure;
	// neither recovery nor GET can backfill old registry-only history.
	if reason == "check_pipeline_incomplete" && fact.MCPRegistryEvidence != nil {
		registry := &next.PreparationMCPRegistryEvidence[len(next.PreparationMCPRegistryEvidence)-1]
		var skills *workspace.WorkspacePreparationSkillRegistryEvidence
		for n := range next.PreparationSkillRegistryEvidence {
			if next.PreparationSkillRegistryEvidence[n].Ref == a.SkillRegistryEvidenceRef {
				skills = &next.PreparationSkillRegistryEvidence[n]
			}
		}
		compiled := workspace.CompilePreparationMCPSelection(workspace.PreparationMCPDefinition(current), *registry, skills)
		registry.Selection = &compiled
		registry.SelectionEvaluationVersion = workspace.PreparationMCPSelectionEvaluationVersion
		registry.ContentHash = registry.Hash()
		for n := range a.Checks {
			if a.Checks[n].ID == "mcp_selection" {
				a.Checks[n].Reason = registry.BlockedReason()
			}
		}
		record.Attempts[i] = a
		next.StartupPreparation = &record
	}
	if fact.ManagedConfigEvidence != nil {
		next.PreparationManagedConfigEvidence = append(workspace.ClonePreparationManagedConfigEvidence(current.PreparationManagedConfigEvidence), fact.ManagedConfigEvidence.Clone())
	}
	if fact.ResourceEvidence != nil {
		next.PreparationResourceEvidence = append(workspace.ClonePreparationResourceEvidence(current.PreparationResourceEvidence), fact.ResourceEvidence.Clone())
	}
	if fact.DirectoryEvidence != nil {
		next.PreparationDirectoryEvidence = append(workspace.ClonePreparationDirectoryEvidence(current.PreparationDirectoryEvidence), fact.DirectoryEvidence.Clone())
	}
	// Only the original private settlement evaluates raw owner material. GET and
	// recovery never acquire facts or upgrade a previous blocked attempt.
	if fact.SealEvidence != nil {
		e := fact.SealEvidence.Clone()
		// Only the original exact settlement can introduce policy inputs. The
		// caller supplies local process facts, never Workspace/template policy.
		// Drift/recovery cannot construct old policy from today's declarations.
		if reason == "check_pipeline_incomplete" && fact.ProcessPolicyFacts != nil && current.DefinitionAuthoring != nil && workspace.WorkspaceConfigurationHash(current) == a.Binding.AuthoringConfigurationHash {
			inputs := workspace.PreparationExecutionInputs{Version: workspace.PreparationExecutionInputsVersion, Process: *fact.ProcessPolicyFacts, Template: current.DefinitionAuthoring.TemplateSnapshot.Clone(), Configuration: workspace.CaptureWorkspaceConfiguration(current)}
			e.ExecutionInputs = &inputs
			a.ExecutionInputVersion = inputs.Version
			e.ContentHash = e.Hash()
		}
		a.SealEvidenceRef = e.Ref
		next.PreparationSealEvidence = append(workspace.ClonePreparationSealEvidence(current.PreparationSealEvidence), e)
		record.Attempts[i] = a
		next.StartupPreparation = &record
	}
	if reason == "check_pipeline_incomplete" {
		a.Checks = workspace.EvaluatePreparationChecks(a, fact.DirectoryEvidence, fact.ResourceEvidence, fact.ManagedConfigEvidence, fact.SoulEvidence)
		a.Checks = workspace.EvaluatePreparationMemoryCheck(a, fact.MemoryInventoryEvidence)
		a.Checks = workspace.EvaluatePreparationSealChecks(&next, a)
		if fence := workspace.CompilePreparationNoRunFinalFence(current, &next, a); fence != nil {
			a.NoRunEvaluationVersion = workspace.PreparationNoRunFinalVersion
			for n := range next.PreparationRunHistoryEvidence {
				e := &next.PreparationRunHistoryEvidence[n]
				if e.Ref == a.RunHistoryEvidenceRef {
					e.FinalFence = fence
					e.ContentHash = e.Hash()
				}
			}
			for n := range a.Checks {
				if a.Checks[n].ID == "first_activation_no_run" {
					a.Checks[n].State, a.Checks[n].Reason = "passed", workspace.PreparationNoRunFinalReason
				}
			}
		}
		a.EmptySelectionEvaluationVersion = workspace.PreparationEmptySelectionEvaluationVersion
		a.Checks = workspace.EvaluatePreparationEmptySelections(&next, a)
		record.Attempts[i] = a
		next.StartupPreparation = &record
	}
	// Save the complete original checklist aggregate in this same CAS. GET and
	// replay never synthesize a missing historical aggregate or new owner facts.
	if a.EvaluationVersion == workspace.PreparationCheckEvaluationVersion {
		assessment, err := workspace.CompilePreparationAssessment(&next, a)
		if err != nil {
			return workspaceExecutionTransition{}, err
		}
		a.Assessment = &assessment
		record.Attempts[i] = a
		next.StartupPreparation = &record
	}
	// Compile sealed source obligations on the original settlement, even when
	// Provider is blocked. Never invent an activation/provider to force a full
	// bootstrap. Interrupted/stale histories are not upgraded on recovery.
	if reason == "check_pipeline_incomplete" && a.SealEvidenceRef != "" {
		compiled, err := startuppreparation.CompileOriginalPreparedDefinition(&next, a)
		if err != nil {
			return workspaceExecutionTransition{}, err
		}
		a.DefinitionCompilation = &compiled
		record.Attempts[i] = a
		next.StartupPreparation = &record
	}
	if workspace.ValidatePreparationEvidencePersistence(&next) != nil {
		return workspaceExecutionTransition{}, common.ErrConflict
	}
	return workspaceExecutionTransition{Next: &next}, nil
}

func preparationCommandResult(w *workspace.Workspace, command workspaceevents.CheckWorkspacePreparationCommand) (workspaceevents.CheckWorkspacePreparationResult, error) {
	state, err := preparationStateFromPersistence(w, time.Now().UTC())
	if err != nil {
		return workspaceevents.CheckWorkspacePreparationResult{}, err
	}
	// w is a detached repository read/committed after-image, never a repository
	// handle or shared owner object. Application recompiles it independently;
	// neither this source nor compilation results can be submitted as authority.
	result := workspaceevents.CheckWorkspacePreparationResult{RequestID: command.RequestID, State: state, CompilationSource: workspaceeventmapping.PreparationCompilationSource(w)}
	return result, result.ValidateFor(command)
}

func (m *Biz) preparationCommandReadback(ctx context.Context, w *workspace.Workspace, command workspaceevents.CheckWorkspacePreparationCommand) (workspaceevents.CheckWorkspacePreparationResult, error) {
	state, err := m.preparationState(ctx, w)
	if err != nil {
		return workspaceevents.CheckWorkspacePreparationResult{}, err
	}
	result := workspaceevents.CheckWorkspacePreparationResult{RequestID: command.RequestID, State: state, CompilationSource: workspaceeventmapping.PreparationCompilationSource(w)}
	return result, result.ValidateFor(command)
}
