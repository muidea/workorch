package orchestrator

// Runtime prerequisite acquisition is owned by Biz.

import (
	"context"
	"fmt"
	"sort"
	"strings"

	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	gitevents "github.com/muidea/workorch/internal/modules/blocks/git/pkg/events"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (m *Biz) prepareRuntimePrerequisites(ctx context.Context, aggregate runtime.RuntimeAggregate) (runtime.RuntimeAggregate, error) {
	if m == nil || m.runtimeController == nil || m.EventHub() == nil {
		return runtime.RuntimeAggregate{}, common.ErrInvalidArgument
	}
	workspace, err := (workspaceSnapshotReader{hub: m.EventHub()}).Read(ctx, aggregate.Facts.WorkspaceID)
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	for {
		pending := append([]runtimecontract.RuntimePrerequisite(nil), aggregate.Facts.Prerequisites.Pending...)
		if len(pending) == 0 {
			return aggregate, nil
		}
		progressed := false
		for _, prerequisite := range runtimeOrderedPrerequisites(pending) {
			// A previous owner result may have committed a new aggregate and
			// removed this copied pending item. Never resolve stale work.
			if !runtimePendingPrerequisiteExists(aggregate.Facts.Prerequisites.Pending, prerequisite.ID) {
				continue
			}
			result, resolveErr := m.resolveRuntimePrerequisite(ctx, aggregate, workspace.DeliverableDirectory, workspace.InternalDirectory, prerequisite)
			if resolveErr != nil {
				if prerequisite.Kind == runtimecontract.PrerequisiteKindProviderCeiling && runtimeRecoverableWaitEligibleError(resolveErr) {
					return m.openRuntimeProviderCeilingRecoverableWait(ctx, aggregate, prerequisite)
				}
				return runtime.RuntimeAggregate{}, resolveErr
			}
			committed, commitErr := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
				Kind: runtime.RuntimeInternalEventPrerequisiteObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Authority: &runtime.RuntimeInternalAuthorityPayload{PrerequisiteResult: &result},
			})
			aggregate, err = committed.Aggregate, commitErr
			if err != nil {
				return runtime.RuntimeAggregate{}, err
			}
			progressed = true
			// Re-read and re-order after every persisted owner fact. Capability
			// acquisition can add owner-published resource dependencies to the
			// current graph, and those dependencies must be acquired before an
			// already-copied dependent inventory item.
			break
		}
		if !progressed {
			return runtime.RuntimeAggregate{}, fmt.Errorf("runtime prerequisite dispatcher made no progress")
		}
	}
}

func runtimePendingPrerequisiteExists(values []runtimecontract.RuntimePrerequisite, id string) bool {
	for _, value := range values {
		if value.ID == id {
			return true
		}
	}
	return false
}

// confirmRuntimeRepairSurface implements the owner handoff after a rejected
// validation receipt. The File command is a query; only the resulting internal
// event may change SM-2 authority.
func (m *Biz) confirmRuntimeRepairSurface(ctx context.Context, aggregate runtime.RuntimeAggregate) (runtime.RuntimeAggregate, bool, error) {
	if m == nil || m.EventHub() == nil || aggregate.Facts.Failure == nil || aggregate.Facts.Failure.ValidationFailureObservation == nil || aggregate.Facts.Resources.FailureTarget != nil {
		return aggregate, false, nil
	}
	observation := aggregate.Facts.Failure.ValidationFailureObservation.Clone()
	if observation.Validate() != nil {
		return runtime.RuntimeAggregate{}, false, common.ErrInvalidArgument
	}
	// Keep the receipt-to-confirmation handoff explicit in the owner-local
	// vocabulary. This identity event deliberately does not mutate aggregate
	// state; the following confirmation event is the only authority-changing
	// step.
	if _, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventValidationFailureObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Authority: &runtime.RuntimeInternalAuthorityPayload{FailureFact: aggregate.Facts.Failure},
	}); err != nil {
		return runtime.RuntimeAggregate{}, false, err
	}
	workspace, err := (workspaceSnapshotReader{hub: m.EventHub()}).Read(ctx, aggregate.Facts.WorkspaceID)
	if err != nil {
		return runtime.RuntimeAggregate{}, false, err
	}
	confirmation, err := ConfirmRuntimeRepairSurface(ctx, m.EventHub(), fileevents.ConfirmRuntimeRepairSurfaceCommand{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, WorkspaceDeliverableDirectory: workspace.DeliverableDirectory, Observation: observation})
	if err != nil {
		return runtime.RuntimeAggregate{}, false, err
	}
	updated, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventRepairSurfaceConfirmationObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Authority: &runtime.RuntimeInternalAuthorityPayload{RepairSurfaceConfirmation: &confirmation.Confirmation}})
	return updated.Aggregate, true, err
}

func (m *Biz) resolveRuntimePrerequisite(
	ctx context.Context,
	aggregate runtime.RuntimeAggregate,
	deliverableDirectory string,
	workspaceInternalDirectory string,
	prerequisite runtimecontract.RuntimePrerequisite,
) (runtime.PrerequisiteResult, error) {
	commandScope := struct {
		workspaceID common.WorkspaceID
		runID       common.RunID
		snapshotID  string
	}{aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, aggregate.Decision.AuthoritySnapshot.ID}
	if runtimePrerequisiteResolverClassification(prerequisite.Kind) == runtimePrerequisiteResolverUnknown {
		return runtime.PrerequisiteResult{}, fmt.Errorf("unsupported runtime prerequisite kind %q", prerequisite.Kind)
	}
	switch prerequisite.Kind {
	case runtimecontract.PrerequisiteKindManifest:
		facts, err := AcquireAndCommitWorkspaceManifestPrerequisite(ctx, m.EventHub(), aggregate.Facts, fileevents.AcquireWorkspaceManifestCommand{
			WorkspaceID: commandScope.workspaceID, RunID: commandScope.runID, AuthoritySnapshotID: commandScope.snapshotID,
			PrerequisiteID: prerequisite.ID, WorkspaceDeliverableDirectory: deliverableDirectory,
		})
		if err != nil {
			return runtime.PrerequisiteResult{}, err
		}
		observation, _ := runtimecontract.RuntimePrerequisiteObservationForID(facts.Resources.PrerequisiteObservations, prerequisite.ID)
		return runtime.PrerequisiteResult{PrerequisiteID: prerequisite.ID, Manifest: facts.Resources.Manifest, Observation: &observation}, nil
	case runtimecontract.PrerequisiteKindTargetSnapshot:
		maxBytes := prerequisite.MinimumBytes
		if maxBytes <= 0 {
			maxBytes = runtimeDefaultTargetSnapshotBytes
		}
		result, err := AcquireTargetSnapshotPrerequisite(ctx, m.EventHub(), fileevents.AcquireTargetSnapshotCommand{
			WorkspaceID: commandScope.workspaceID, RunID: commandScope.runID, AuthoritySnapshotID: commandScope.snapshotID,
			PrerequisiteID: prerequisite.ID, WorkspaceDeliverableDirectory: deliverableDirectory,
			TargetRef: prerequisite.SubjectRef, MaxBytes: maxBytes,
		})
		if err != nil {
			return runtime.PrerequisiteResult{}, err
		}
		return runtime.PrerequisiteResult{PrerequisiteID: prerequisite.ID, TargetSnapshot: &result.Snapshot, Observation: &result.Observation}, nil
	case runtimecontract.PrerequisiteKindTargetSelection:
		result, err := AcquireRuntimeTargetSelectionPrerequisite(ctx, m.EventHub(), fileevents.AcquireRuntimeTargetSelectionCommand{
			WorkspaceID: commandScope.workspaceID, RunID: commandScope.runID, AuthoritySnapshotID: commandScope.snapshotID,
			PrerequisiteID: prerequisite.ID, WorkspaceDeliverableDirectory: deliverableDirectory, TargetRole: prerequisite.SubjectRef, TargetSelectorRef: prerequisite.TargetSelectorRef, TargetSelectorOwnerRef: prerequisite.TargetSelectorOwnerRef, TargetSelectorSubjectRef: prerequisite.TargetSelectorSubjectRef, TargetSelectorCandidateRefs: prerequisite.TargetSelectorCandidateRefs, ProspectiveTargetRef: prerequisite.ProspectiveTargetRef,
			// Target selection seals a complete File-owner feasibility fact for
			// SM-2.  It must not inherit the much smaller per-request candidate
			// context budget, otherwise a normal workspace with more files than
			// coverage items is incorrectly reported as ambiguous.
			MaxItems: runtimecontract.DefaultRuntimeOwnerInventoryBudget().MaxItems,
		})
		if err != nil {
			return runtime.PrerequisiteResult{}, err
		}
		selection := result.Selection
		return runtime.PrerequisiteResult{PrerequisiteID: prerequisite.ID, TargetSelection: &selection, Observation: &result.Observation}, nil
	case runtimecontract.PrerequisiteKindValidationScratch:
		facts, err := AcquireValidationScratchPrerequisite(aggregate.Facts, workspaceInternalDirectory, prerequisite.ID)
		if err != nil {
			return runtime.PrerequisiteResult{}, err
		}
		observation, ok := runtimecontract.RuntimePrerequisiteObservationForID(facts.Resources.PrerequisiteObservations, prerequisite.ID)
		if !ok {
			return runtime.PrerequisiteResult{}, fmt.Errorf("validation scratch prerequisite %q did not return an observation", prerequisite.ID)
		}
		return runtime.PrerequisiteResult{PrerequisiteID: prerequisite.ID, ValidationScratch: facts.Resources.ValidationScratch, Observation: &observation}, nil
	case runtimecontract.PrerequisiteKindProviderCeiling:
		return m.acquireRuntimeProviderCeilingPrerequisite(ctx, aggregate, prerequisite)
	case runtimecontract.PrerequisiteKindCapability:
		if prerequisite.OwnerModuleID == "" {
			return runtime.PrerequisiteResult{}, fmt.Errorf("runtime capability prerequisite %q has no owner module", prerequisite.ID)
		}
		facts, err := AcquireAndCommitRuntimeCapabilitiesPrerequisite(ctx, m.EventHub(), aggregate.Facts, runtimecontract.RuntimeCapabilityAcquisitionRequest{
			WorkspaceID: commandScope.workspaceID, RunID: commandScope.runID, AuthoritySnapshotID: commandScope.snapshotID,
			PrerequisiteID: prerequisite.ID, OwnerModuleID: prerequisite.OwnerModuleID,
			CapabilityClass: prerequisite.CapabilityClass, RequiredKinds: prerequisite.RequiredCapabilities,
		})
		if err != nil {
			return runtime.PrerequisiteResult{}, err
		}
		capabilities := facts.Capabilities
		var observation *runtimecontract.RuntimePrerequisiteObservation
		if value, observed := runtimecontract.RuntimePrerequisiteObservationForID(facts.Resources.PrerequisiteObservations, prerequisite.ID); observed {
			observation = &value
		}
		return runtime.PrerequisiteResult{PrerequisiteID: prerequisite.ID, Capabilities: &capabilities, Observation: observation}, nil
	case runtimecontract.PrerequisiteKindOperationAdmissibility:
		binding, bindingErr := runtimeCapabilityStateBinding(aggregate.Facts, prerequisite.OwnerModuleID)
		if bindingErr != nil {
			return runtime.PrerequisiteResult{}, bindingErr
		}
		validationRecipes := []runtimecontract.RuntimeValidationRecipe(nil)
		manifestBinding := runtimecontract.RuntimeWorkspaceManifestBinding{}
		if prerequisite.OwnerModuleID == runtimecontract.RuntimeModuleShell && runtimeRequiredKindsContain(prerequisite.RequiredCapabilities, runtimecontract.RuntimeBlockKindManagedValidationSession) {
			validationRecipes = runtimecontract.RuntimeValidationRecipeDeclarationsFromEvidencePolicies(aggregate.Facts.Activation.EvidencePolicies)
			if len(validationRecipes) != 0 {
				var manifestErr error
				manifestBinding, manifestErr = runtimeWorkspaceManifestBinding(aggregate.Facts)
				if manifestErr != nil {
					return runtime.PrerequisiteResult{}, manifestErr
				}
			}
		}
		result, err := AcquireRuntimeOperationAdmissibilityPrerequisite(ctx, m.EventHub(), runtimecontract.RuntimeOperationAdmissibilityRequest{
			WorkspaceID: commandScope.workspaceID, RunID: commandScope.runID, AuthoritySnapshotID: commandScope.snapshotID,
			PrerequisiteID: prerequisite.ID, OwnerModuleID: prerequisite.OwnerModuleID,
			WorkspaceDeliverableDirectory: deliverableDirectory, RequiredKinds: append([]string(nil), prerequisite.RequiredCapabilities...),
			RequiredTargetRefs: runtimeOperationAdmissibilityTargetRefs(aggregate, prerequisite.OwnerModuleID), CapabilityStateBinding: binding,
			ValidationRecipes: validationRecipes, WorkspaceManifestBinding: manifestBinding,
			// Operation admissibility is another owner-grounded SM-2 inventory.
			// In particular, it must include a file created by a prior accepted
			// operation so the next sealed validation can read it. Candidate
			// context compression must not remove that owner authority.
			MaxItems: runtimecontract.DefaultRuntimeOwnerInventoryBudget().MaxItems,
		})
		if err != nil {
			return runtime.PrerequisiteResult{}, err
		}
		admissibility := result.Admissibility
		observation := result.Observation
		return runtime.PrerequisiteResult{PrerequisiteID: prerequisite.ID, OperationAdmissibility: &admissibility, Observation: &observation}, nil
	case runtimecontract.PrerequisiteKindCapabilityState:
		state, err := AcquireGitCapabilityStatePrerequisite(ctx, m.EventHub(), prerequisite.OwnerModuleID, gitevents.AcquireCapabilityStateCommand{
			WorkspaceID: commandScope.workspaceID, RunID: commandScope.runID, AuthoritySnapshotID: commandScope.snapshotID,
			PrerequisiteID: prerequisite.ID, WorkspaceDeliverableDirectory: deliverableDirectory, CapabilityStateKind: prerequisite.CapabilityStateKind, SubjectRef: prerequisite.SubjectRef,
			AcquisitionKey: stableRuntimeCapabilityStateAcquisitionKey(commandScope.workspaceID, commandScope.runID, prerequisite.ID, runtimeCapabilityStateAcquisitionGeneration(aggregate.PrerequisiteInvalidations, prerequisite.ID)),
		})
		if err != nil {
			return runtime.PrerequisiteResult{}, err
		}
		return runtime.PrerequisiteResult{PrerequisiteID: prerequisite.ID, CapabilityState: &state.Snapshot, Observation: &state.Observation}, nil
	default:
		return runtime.PrerequisiteResult{}, fmt.Errorf("unsupported runtime prerequisite kind %q", prerequisite.Kind)
	}
}

// runtimeWorkspaceManifestBinding returns only the active File-owner manifest
// fact and its matching persisted observation.  Orchestrator copies this value
// into a capability-owner request; it does not inspect paths or decide recipe
// applicability.
func runtimeWorkspaceManifestBinding(facts runtimecontract.RuntimeFacts) (runtimecontract.RuntimeWorkspaceManifestBinding, error) {
	facts = facts.Normalized()
	if facts.Resources.Manifest == nil || strings.TrimSpace(facts.Resources.Manifest.Revision) == "" {
		return runtimecontract.RuntimeWorkspaceManifestBinding{}, fmt.Errorf("runtime Shell validation admissibility requires a committed File workspace manifest")
	}
	for _, observation := range facts.Resources.PrerequisiteObservations {
		if observation.Kind != runtimecontract.PrerequisiteKindManifest || observation.OwnerModuleID != runtimecontract.RuntimeModuleFile ||
			observation.ContentHash != facts.Resources.Manifest.Revision {
			continue
		}
		binding := runtimecontract.RuntimeWorkspaceManifestBinding{
			Manifest: runtimecontract.WorkspaceManifest{
				Revision: facts.Resources.Manifest.Revision, TargetRefs: append([]string(nil), facts.Resources.Manifest.TargetRefs...),
			},
			Observation: observation,
		}.Clone()
		if err := binding.Validate(); err == nil {
			return binding, nil
		}
	}
	return runtimecontract.RuntimeWorkspaceManifestBinding{}, fmt.Errorf("runtime Shell validation admissibility has no current File workspace manifest observation")
}

// runtimeOperationAdmissibilityTargetRefs supplies the Shell owner with the
// bounded File-owner catalog targets needed to describe a validation recipe.
// The Orchestrator only forwards already persisted owner facts; it neither
// invents a path nor lets Shell become the workspace-target owner. Retaining
// that intersection after a semantic seal is required because a File effect
// retires both owner catalogs before SM-2 can seal the next generation.
func runtimeOperationAdmissibilityTargetRefs(aggregate runtime.RuntimeAggregate, ownerModuleID string) []string {
	targets := runtime.GroundedRepairTargets(aggregate.Facts)
	if ownerModuleID != runtimecontract.RuntimeModuleShell {
		return targets
	}
	for _, inventory := range aggregate.Facts.Resources.OperationAdmissibilities {
		if inventory.OwnerModuleID == runtimecontract.RuntimeModuleFile {
			targets = append(targets, inventory.TargetRefs...)
		}
	}
	sort.Strings(targets)
	ret := targets[:0]
	for _, target := range targets {
		target = strings.TrimSpace(target)
		if target == "" || (len(ret) != 0 && ret[len(ret)-1] == target) {
			continue
		}
		ret = append(ret, target)
	}
	return ret
}

func runtimeRequiredKindsContain(values []string, wanted string) bool {
	wanted = runtimecontract.CanonicalRuntimeBlockKind(wanted)
	for _, value := range values {
		if runtimecontract.CanonicalRuntimeBlockKind(value) == wanted {
			return true
		}
	}
	return false
}

// runtimeOrderedPrerequisites makes prerequisite fact dependencies available
// before a dependent owner inventory is acquired. In particular, pre-seal Shell
// managed-validation inventories consume the already persisted File catalog as
// their bounded target vocabulary. This is dispatcher ordering only: both
// owners still return facts and SM-2 commits each one through its internal
// event/CAS path.
func runtimeOrderedPrerequisites(values []runtimecontract.RuntimePrerequisite) []runtimecontract.RuntimePrerequisite {
	ret := append([]runtimecontract.RuntimePrerequisite(nil), values...)
	sort.SliceStable(ret, func(i, j int) bool {
		return runtimePrerequisiteAcquisitionRank(ret[i]) < runtimePrerequisiteAcquisitionRank(ret[j])
	})
	return ret
}

func runtimePrerequisiteAcquisitionRank(value runtimecontract.RuntimePrerequisite) int {
	switch value.Kind {
	case runtimecontract.PrerequisiteKindCapabilityState:
		return 0
	case runtimecontract.PrerequisiteKindOperationAdmissibility:
		if value.OwnerModuleID == runtimecontract.RuntimeModuleFile {
			return 2
		}
		return 3
	default:
		return 1
	}
}

// runtimeCapabilityStateBinding returns the only opaque acquisition binding
// an owner admissibility request may cite. Only Git currently declares this
// repository-state relationship; all other owners remain unbound.
func runtimeCapabilityStateBinding(facts runtimecontract.RuntimeFacts, owner string) (string, error) {
	if owner != runtimecontract.RuntimeModuleGit {
		return "", nil
	}
	for _, snapshot := range facts.Resources.CapabilityStates {
		if snapshot.OwnerModuleID != owner || snapshot.CapabilityStateKind != "repository_state" {
			continue
		}
		binding := strings.TrimSpace(snapshot.AcquisitionBinding)
		if binding == "" {
			binding = strings.TrimSpace(snapshot.AcquisitionKey)
		}
		if binding != "" {
			return binding, nil
		}
	}
	return "", fmt.Errorf("runtime operation admissibility owner %q has no committed capability-state acquisition binding", owner)
}

func stableRuntimeCapabilityStateAcquisitionKey(workspaceID common.WorkspaceID, runID common.RunID, prerequisiteID string, refreshGeneration int) string {
	return fmt.Sprintf("capability-state/%s/%s/%s/%d", workspaceID, runID, strings.TrimSpace(prerequisiteID), refreshGeneration)
}

// runtimeCapabilityStateAcquisitionGeneration is derived exclusively from
// persisted invalidations that affect this prerequisite. A durable receipt is
// normal progress, not recovery; its exact invalidation record advances the
// key without borrowing a run-global retry budget.
func runtimeCapabilityStateAcquisitionGeneration(invalidations []runtimecontract.RuntimePrerequisiteInvalidationRecord, prerequisiteID string) int {
	return runtimecontract.RuntimePrerequisiteInvalidationGeneration(invalidations, prerequisiteID)
}
