package runtimev2

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestGatewayPreflightAcquiresSnapshotsBeforeStateChange(t *testing.T) {
	facts := runtimecontract.RuntimeFacts{
		Activation:         runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		WorkspaceID:        "ws-gateway",
		RunID:              "run-gateway",
		DefinitionRevision: "definition-1",
		EvidenceRevision:   "evidence-1",
		Focus: runtimecontract.CoverageFocus{
			ID:                    "cwi-085",
			CoverageWorkItemIDs:   []common.CoverageWorkItemID{"cwi-085"},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
			TargetRefs:            []string{"go.mod", "cmd/gateway/main.go"},
		},
		Coverage:     []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-085", State: runtimecontract.RuntimeCoverageItemStateOpen}},
		Capabilities: runtimecontract.RuntimeCapabilities{StateChangeKinds: []string{"patch/apply"}},
	}
	requirements := []RuntimeRequirement{
		{Kind: runtimecontract.PrerequisiteKindManifest, Reason: "workspace manifest is required"},
		{Kind: runtimecontract.PrerequisiteKindTargetSnapshot, SubjectRef: "go.mod", Reason: "module manifest snapshot is required"},
		{Kind: runtimecontract.PrerequisiteKindTargetSnapshot, SubjectRef: "cmd/gateway/main.go", Reason: "source snapshot is required"},
	}
	facts = PreflightRuntimeFacts(facts, requirements)
	decision, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != runtimecontract.RuntimeActionAcquirePrerequisite || len(decision.Prerequisites) != 3 {
		t.Fatalf("preflight decision = %+v, want three deterministic prerequisites", decision)
	}

	for _, item := range append([]runtimecontract.RuntimePrerequisite(nil), facts.Prerequisites.Pending...) {
		result := PrerequisiteResult{PrerequisiteID: item.ID}
		switch item.Kind {
		case runtimecontract.PrerequisiteKindManifest:
			result.Manifest = &runtimecontract.WorkspaceManifest{Revision: "manifest-1", TargetRefs: []string{"go.mod", "cmd/gateway/main.go"}}
		case runtimecontract.PrerequisiteKindTargetSnapshot:
			result.TargetSnapshot = &runtimecontract.TargetSnapshot{TargetRef: item.SubjectRef, Content: "content", ContentHash: "hash-" + item.SubjectRef, ContentState: runtimecontract.SnapshotContentStateComplete}
		}
		facts, err = CommitPrerequisiteResult(facts, result)
		if err != nil {
			t.Fatalf("commit %s: %v", item.ID, err)
		}
	}
	facts = PreflightRuntimeFacts(facts, requirements)
	if len(facts.Prerequisites.Pending) != 0 {
		t.Fatalf("complete snapshots were scheduled again: %+v", facts.Prerequisites.Pending)
	}
	decision, err = ReduceRuntimeState(facts, &decision.AuthoritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != runtimecontract.RuntimeActionProduceStateChange {
		t.Fatalf("post-preflight decision = %+v, want state change", decision)
	}
	for _, kind := range decision.RequiredOutput.AcceptedKinds {
		if kind == "file/list" || kind == "file/read" {
			t.Fatalf("prerequisite kind leaked into LLM state-change contract: %+v", decision.RequiredOutput.AcceptedKinds)
		}
	}
}

func TestCommitPrerequisiteResultRejectsDuplicateAndUnsafeSnapshot(t *testing.T) {
	requirement := RuntimeRequirement{Kind: runtimecontract.PrerequisiteKindTargetSnapshot, SubjectRef: "main.go"}
	facts := PreflightRuntimeFacts(validationFacts(), []RuntimeRequirement{requirement})
	id := facts.Prerequisites.Pending[0].ID
	_, err := CommitPrerequisiteResult(facts, PrerequisiteResult{
		PrerequisiteID: id,
		TargetSnapshot: &runtimecontract.TargetSnapshot{TargetRef: "main.go", ContentState: runtimecontract.SnapshotContentStateTruncated},
	})
	if err == nil {
		t.Fatal("unsafe snapshot was accepted")
	}
	committed, err := CommitPrerequisiteResult(facts, PrerequisiteResult{
		PrerequisiteID: id,
		TargetSnapshot: &runtimecontract.TargetSnapshot{TargetRef: "main.go", ContentHash: "hash", ContentState: runtimecontract.SnapshotContentStateComplete},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CommitPrerequisiteResult(committed, PrerequisiteResult{PrerequisiteID: id}); err == nil {
		t.Fatal("duplicate prerequisite result was accepted")
	}
}

func TestReduceRuntimeStateWithDirectiveAcquiresDeclaredFocusTargetBeforeCandidate(t *testing.T) {
	facts := validationFacts()
	facts.Focus.RequireTargetGrounding = true
	facts.Focus.TargetRole = "implementation"
	facts.Focus.TargetRefs = []string{"implementation.go"}
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
	facts.Prerequisites = runtimecontract.PrerequisiteState{}
	updated, decision, err := ReduceRuntimeStateWithDirective(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != runtimecontract.RuntimeActionAcquirePrerequisite || len(updated.Prerequisites.Pending) != 1 {
		t.Fatalf("target-grounding decision=%+v prerequisites=%+v, want acquisition", decision, updated.Prerequisites)
	}
	prerequisite := updated.Prerequisites.Pending[0]
	if prerequisite.Kind != runtimecontract.PrerequisiteKindTargetSnapshot || prerequisite.OwnerModuleID != runtimecontract.RuntimeModuleFile || prerequisite.SubjectRef != "implementation.go" {
		t.Fatalf("target-grounding prerequisite = %+v", prerequisite)
	}
}

func TestReduceRuntimeStateWithDirectiveRetiresHistoricalSelectionBeforeCurrentFocusPreflight(t *testing.T) {
	facts := validationFacts()
	facts.Focus = runtimecontract.CoverageFocus{
		ID: "focus-manager", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, TargetRole: "implementation",
		TargetDiscoverySources: []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest},
		TargetSelectorRef:      "file/profile_declared_candidates", TargetSelectorOwnerRef: "profile-bootstrap",
		TargetSelectorSubjectRef: "manager", TargetSelectorCandidateRefs: []string{"internal/lsp/manager.go"},
		RequireTargetGrounding: true,
	}
	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: "selection-go-mod", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		TargetRole: "module_definition", SelectorRef: "file/profile_declared_candidates",
		Disposition: runtimecontract.RuntimeTargetSelectionSelected, TargetRefs: []string{"go.mod"},
		CandidateTargetRefs: []string{"go.mod"}, CandidateCount: 1, ObservedRevision: "tree-old",
	}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: selection.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindTargetSelection,
		OwnerModuleID: runtimecontract.RuntimeModuleFile, ContentHash: selection.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: "workspace_deliverable", Revision: "tree-old",
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	facts.Resources.TargetSelection = &selection
	facts.Resources.TargetSnapshots = []runtimecontract.TargetSnapshot{{TargetRef: "go.mod", ContentHash: "go-mod-old", ContentState: runtimecontract.SnapshotContentStateComplete}}
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{{
		ID: selection.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindTargetSelection,
		Scope: runtimecontract.RuntimePrerequisiteScopeFocus, FocusID: "focus-module", OwnerModuleID: runtimecontract.RuntimeModuleFile,
	}}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{observation}

	updated, decision, err := ReduceRuntimeStateWithDirective(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != runtimecontract.RuntimeActionAcquirePrerequisite || updated.Resources.TargetSelection != nil || len(updated.Resources.TargetSnapshots) != 0 ||
		len(updated.Prerequisites.Satisfied) != 0 || len(updated.Resources.PrerequisiteObservations) != 0 ||
		len(updated.Prerequisites.Retired) != 1 || updated.Prerequisites.Retired[0].ID != selection.PrerequisiteID ||
		len(updated.Resources.RetiredPrerequisiteObservations) != 1 || updated.Resources.RetiredPrerequisiteObservations[0].ObservationID != observation.ObservationID {
		t.Fatalf("historical target authority was not retired before preflight: facts=%+v decision=%+v", updated, decision)
	}
	foundCurrentSelection := false
	for _, prerequisite := range updated.Prerequisites.Pending {
		if prerequisite.Kind == runtimecontract.PrerequisiteKindTargetSelection && prerequisite.Scope == runtimecontract.RuntimePrerequisiteScopeFocus &&
			prerequisite.FocusID == facts.Focus.ID && prerequisite.TargetSelectorCandidateRefs[0] == "internal/lsp/manager.go" {
			foundCurrentSelection = true
		}
	}
	if !foundCurrentSelection {
		t.Fatalf("current focus target selection was not acquired: %+v", updated.Prerequisites.Pending)
	}
}

func TestPreflightRuntimeFactsReacquiresFailureTargetWhenTypedFailureChanges(t *testing.T) {
	requirement := RuntimeRequirement{Kind: runtimecontract.PrerequisiteKindFailureTarget, Scope: runtimecontract.RuntimePrerequisiteScopeGlobal, OwnerModuleID: runtimecontract.RuntimeModuleOrchestrator}
	facts := runtimecontract.RuntimeFacts{
		Failure:       &runtimecontract.FailureFact{Signature: "failure-v2", TargetRefs: []string{"next.go"}},
		Resources:     runtimecontract.RuntimeResourceFacts{FailureTarget: &runtimecontract.FailureTarget{Signature: "failure-v1", TargetRefs: []string{"old.go"}}},
		Prerequisites: runtimecontract.PrerequisiteState{Satisfied: []runtimecontract.RuntimePrerequisite{{ID: runtimePrerequisiteID(requirement), Kind: runtimecontract.PrerequisiteKindFailureTarget, OwnerModuleID: runtimecontract.RuntimeModuleOrchestrator}}},
	}
	prepared := PreflightRuntimeFacts(facts, []RuntimeRequirement{requirement})
	if len(prepared.Prerequisites.Pending) != 1 || prepared.Prerequisites.Pending[0].Kind != runtimecontract.PrerequisiteKindFailureTarget || len(prepared.Prerequisites.Satisfied) != 0 {
		t.Fatalf("changed failure target did not re-enter acquisition: %+v", prepared.Prerequisites)
	}
}

func TestCommitPrerequisiteResultRejectsFailureTargetOutsideCurrentTypedFailure(t *testing.T) {
	requirement := RuntimeRequirement{Kind: runtimecontract.PrerequisiteKindFailureTarget, Scope: runtimecontract.RuntimePrerequisiteScopeGlobal, OwnerModuleID: runtimecontract.RuntimeModuleOrchestrator}
	facts := runtimecontract.RuntimeFacts{
		Failure:       &runtimecontract.FailureFact{Signature: "failure-v2", TargetRefs: []string{"next.go"}},
		Prerequisites: runtimecontract.PrerequisiteState{Pending: []runtimecontract.RuntimePrerequisite{{ID: runtimePrerequisiteID(requirement), Kind: requirement.Kind, Scope: requirement.Scope, OwnerModuleID: requirement.OwnerModuleID}}},
	}
	if _, err := CommitPrerequisiteResult(facts, PrerequisiteResult{PrerequisiteID: runtimePrerequisiteID(requirement), FailureTarget: &runtimecontract.FailureTarget{Signature: "failure-v1", TargetRefs: []string{"old.go"}}}); err == nil {
		t.Fatal("failure target with a different typed failure identity was accepted")
	}
	if facts.Resources.FailureTarget != nil || len(facts.Prerequisites.Pending) != 1 {
		t.Fatalf("rejected local mapping mutated source facts: %+v", facts)
	}
}

func TestReduceRuntimeStateWithDirectiveAcquiresSnapshotForPersistedFailureTargetBeforeCandidate(t *testing.T) {
	facts := validationFacts()
	facts.Focus.RequireTargetGrounding = true
	facts.Focus.TargetRole = "implementation"
	facts.Focus.TargetRefs = nil
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
	facts.Resources.FailureTarget = &runtimecontract.FailureTarget{Signature: "failed-validation", TargetRefs: []string{"implementation.go"}}
	facts.Prerequisites = runtimecontract.PrerequisiteState{}

	updated, decision, err := ReduceRuntimeStateWithDirective(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != runtimecontract.RuntimeActionAcquirePrerequisite || len(updated.Prerequisites.Pending) != 1 {
		t.Fatalf("failure-target grounding decision=%+v prerequisites=%+v, want snapshot acquisition", decision, updated.Prerequisites)
	}
	prerequisite := updated.Prerequisites.Pending[0]
	if prerequisite.Kind != runtimecontract.PrerequisiteKindTargetSnapshot || prerequisite.OwnerModuleID != runtimecontract.RuntimeModuleFile || prerequisite.SubjectRef != "implementation.go" {
		t.Fatalf("failure-target snapshot prerequisite = %+v", prerequisite)
	}
}

func TestReduceRuntimeStateWithDirectiveAcquiresDiscoveryManifestBeforeCandidate(t *testing.T) {
	facts := validationFacts()
	facts.Focus.RequireTargetGrounding = true
	facts.Focus.TargetRole = "implementation"
	facts.Focus.TargetDiscoverySources = []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest}
	facts.Focus.TargetSelectorRef = "file/unique_workspace_entry"
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
	facts.Prerequisites = runtimecontract.PrerequisiteState{}
	updated, decision, err := ReduceRuntimeStateWithDirective(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != runtimecontract.RuntimeActionAcquirePrerequisite || len(updated.Prerequisites.Pending) != 2 || updated.Prerequisites.Pending[0].Kind != runtimecontract.PrerequisiteKindManifest || updated.Prerequisites.Pending[1].Kind != runtimecontract.PrerequisiteKindTargetSelection {
		t.Fatalf("discovery-grounding decision=%+v prerequisites=%+v, want manifest and owner selection acquisition", decision, updated.Prerequisites)
	}
}

func TestReduceRuntimeStateWithDirectiveDoesNotRequestStateChangeWithoutOwnerGroundedTarget(t *testing.T) {
	facts := validationFacts()
	facts.Focus.RequireTargetGrounding = true
	facts.Focus.TargetRole = "implementation"
	facts.Focus.TargetDiscoverySources = []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest}
	facts.Focus.TargetSelectorRef = "file/unique_workspace_entry"
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
	facts.Prerequisites = runtimecontract.PrerequisiteState{}

	grounding, decision, err := ReduceRuntimeStateWithDirective(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != runtimecontract.RuntimeActionAcquirePrerequisite || len(grounding.Prerequisites.Pending) != 2 {
		t.Fatalf("initial grounding = decision=%+v prerequisites=%+v, want manifest and owner selection acquisition", decision, grounding.Prerequisites)
	}
	for _, prerequisite := range append([]runtimecontract.RuntimePrerequisite(nil), grounding.Prerequisites.Pending...) {
		result := PrerequisiteResult{PrerequisiteID: prerequisite.ID}
		switch prerequisite.Kind {
		case runtimecontract.PrerequisiteKindManifest:
			result.Manifest = &runtimecontract.WorkspaceManifest{Revision: "manifest-1", TargetRefs: []string{"validation-entry.go", "implementation.go"}}
		case runtimecontract.PrerequisiteKindTargetSelection:
			selection := testRuntimeTargetSelection(prerequisite.ID, "implementation", runtimecontract.RuntimeTargetSelectionAmbiguous, []string{"validation-entry.go", "implementation.go"})
			result.TargetSelection = &selection
		}
		grounding, err = CommitPrerequisiteResult(grounding, result)
		if err != nil {
			t.Fatal(err)
		}
	}
	grounding, decision, err = ReduceRuntimeStateWithDirective(grounding, &decision.AuthoritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action == runtimecontract.RuntimeActionProduceStateChange || decision.Action == runtimecontract.RuntimeActionProduceValidation {
		t.Fatalf("ungrounded discovery requested provider candidate: %+v", decision)
	}
	if decision.Action != runtimecontract.RuntimeActionFail || decision.Terminal == nil || decision.Terminal.Code != "target_grounding_unresolved" {
		t.Fatalf("ungrounded discovery decision = %+v, want target-grounding terminal", decision)
	}
}

func TestReduceRuntimeStateWithDirectiveAllowsOwnerGroundedEmptyWorkspaceCreation(t *testing.T) {
	facts := validationFacts()
	facts.Focus.RequireTargetGrounding = true
	facts.Focus.TargetRole = "deliverable"
	facts.Focus.TargetDiscoverySources = []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest}
	facts.Focus.TargetSelectorRef = "file/workspace_deliverable"
	facts.Focus.ProspectiveTargetRef = "initial-deliverable.txt"
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
	facts.Focus.AcceptedStepKinds = []string{runtimecontract.StepKindFileWrite}
	facts.Capabilities.StateChangeKinds = []string{runtimecontract.StepKindFileWrite}
	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: "target_selection:blocks/file:deliverable:file/workspace_deliverable:::0",
		OwnerModuleID:  runtimecontract.RuntimeModuleFile, TargetRole: "deliverable", SelectorRef: "file/workspace_deliverable",
		Disposition: runtimecontract.RuntimeTargetSelectionCreate, ProspectiveTargetRef: "initial-deliverable.txt", CandidateCount: 0, ObservedRevision: "empty-workspace-v1",
	}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	facts.Resources.TargetSelection = &selection
	facts.Resources.Manifest = &runtimecontract.WorkspaceManifest{Revision: "empty-workspace-v1"}
	inventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "operation_admissibility:blocks/file:::file/write::0", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.StepKindFileWrite}, SelectorRefs: []string{"workspace-relative-new-file-target"},
		ObservedRevision: "empty-workspace-v1",
	}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{inventory}
	facts.Prerequisites = runtimecontract.PrerequisiteState{}
	bindCurrentFocusTargetSelection(&facts)

	updated, decision, err := ReduceRuntimeStateWithDirective(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != runtimecontract.RuntimeActionProduceStateChange || decision.RequiredOutput.Kind != runtimecontract.RequiredOutputKindStateChange || len(decision.RequiredOutput.AcceptedKinds) != 1 || decision.RequiredOutput.AcceptedKinds[0] != runtimecontract.StepKindFileWrite || len(decision.RequiredOutput.TargetRefs) != 1 || decision.RequiredOutput.TargetRefs[0] != "initial-deliverable.txt" || len(updated.Prerequisites.Pending) != 0 {
		t.Fatalf("empty workspace creation decision=%+v prerequisites=%+v", decision, updated.Prerequisites)
	}
}

func TestReduceRuntimeStateWithDirectiveAllowsBoundedOwnerWorkspaceScopeWithoutConcreteSnapshots(t *testing.T) {
	facts := validationFacts()
	facts.Focus.RequireTargetGrounding = true
	facts.Focus.TargetRole = "deliverable"
	facts.Focus.TargetDiscoverySources = []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest}
	facts.Focus.TargetSelectorRef = "file/workspace_deliverable"
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: "target_selection:blocks/file:deliverable:file/workspace_deliverable:::0", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		TargetRole: "deliverable", SelectorRef: "file/workspace_deliverable", Disposition: runtimecontract.RuntimeTargetSelectionScope,
		TargetRefs: []string{"implementation.go", "support.go"}, CandidateTargetRefs: []string{"implementation.go", "support.go"}, CandidateCount: 2, ObservedRevision: "workspace-tree-v1",
	}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	facts.Resources.TargetSelection = &selection
	facts.Resources.Manifest = &runtimecontract.WorkspaceManifest{Revision: "workspace-tree-v1", TargetRefs: []string{"implementation.go", "support.go"}}
	facts.Prerequisites = runtimecontract.PrerequisiteState{}
	bindCurrentFocusTargetSelection(&facts)

	updated, decision, err := ReduceRuntimeStateWithDirective(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != runtimecontract.RuntimeActionProduceStateChange || len(decision.RequiredOutput.TargetRefs) != 0 || len(updated.Prerequisites.Pending) != 0 {
		t.Fatalf("workspace scope decision=%+v prerequisites=%+v", decision, updated.Prerequisites)
	}
}

func TestReduceRuntimeStateWithDirectiveTerminatesUngroundedStateChangeWithoutGovernance(t *testing.T) {
	facts := validationFacts()
	facts.Focus.RequireTargetGrounding = true
	facts.Focus.TargetRole = "implementation"
	facts.Focus.TargetDiscoverySources = []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest}
	facts.Focus.TargetSelectorRef = "file/unique_workspace_entry"
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
	facts.Capabilities.GovernanceKinds = []string{"governance/decision"}
	facts.Resources.Manifest = &runtimecontract.WorkspaceManifest{Revision: "manifest-1", TargetRefs: []string{"validation-entry.go", "implementation.go"}}
	selection := testRuntimeTargetSelection("target_selection:blocks/file:implementation::::0", "implementation", runtimecontract.RuntimeTargetSelectionAmbiguous, []string{"validation-entry.go", "implementation.go"})
	facts.Resources.TargetSelection = &selection
	facts.Prerequisites = runtimecontract.PrerequisiteState{}
	bindCurrentFocusTargetSelection(&facts)

	_, decision, err := ReduceRuntimeStateWithDirective(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != runtimecontract.RuntimeActionFail || decision.Terminal == nil || decision.Terminal.Code != "target_grounding_unresolved" {
		t.Fatalf("ungrounded discovery decision = %+v, want target-grounding terminal", decision)
	}
}

func TestReduceRuntimeStateWithDirectiveTerminatesUngroundedDirectValidationWithoutGovernance(t *testing.T) {
	facts := validationFacts()
	facts.Focus.RequireTargetGrounding = true
	facts.Focus.TargetRole = "implementation"
	facts.Focus.TargetRefs = nil
	facts.Focus.TargetDiscoverySources = []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest}
	facts.Focus.TargetSelectorRef = "file/unique_workspace_entry"
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	facts.Capabilities.GovernanceKinds = []string{"governance/decision"}
	facts.Resources.Manifest = &runtimecontract.WorkspaceManifest{Revision: "manifest-1", TargetRefs: []string{"validation-entry.go", "implementation.go"}}
	selection := testRuntimeTargetSelection("target_selection:blocks/file:implementation:file/unique_workspace_entry:::0", "implementation", runtimecontract.RuntimeTargetSelectionAmbiguous, []string{"validation-entry.go", "implementation.go"})
	facts.Resources.TargetSelection = &selection
	facts.Prerequisites = runtimecontract.PrerequisiteState{}
	bindCurrentFocusTargetSelection(&facts)

	_, decision, err := ReduceRuntimeStateWithDirective(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != runtimecontract.RuntimeActionFail || decision.Terminal == nil || decision.Terminal.Code != "target_grounding_unresolved" {
		t.Fatalf("ungrounded direct-validation decision = %+v, want target-grounding terminal", decision)
	}
}

func TestReduceRuntimeStateWithDirectiveRequiresSnapshotForOwnerSelectedTarget(t *testing.T) {
	facts := validationFacts()
	facts.Focus.RequireTargetGrounding = true
	facts.Focus.TargetRole = "deliverable"
	facts.Focus.TargetDiscoverySources = []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest}
	facts.Focus.TargetSelectorRef = "file/unique_workspace_entry"
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
	facts.Prerequisites = runtimecontract.PrerequisiteState{}

	grounding, decision, err := ReduceRuntimeStateWithDirective(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, prerequisite := range append([]runtimecontract.RuntimePrerequisite(nil), grounding.Prerequisites.Pending...) {
		result := PrerequisiteResult{PrerequisiteID: prerequisite.ID}
		switch prerequisite.Kind {
		case runtimecontract.PrerequisiteKindManifest:
			result.Manifest = &runtimecontract.WorkspaceManifest{Revision: "manifest-1", TargetRefs: []string{"implementation.go"}}
		case runtimecontract.PrerequisiteKindTargetSelection:
			selection := testRuntimeTargetSelection(prerequisite.ID, "deliverable", runtimecontract.RuntimeTargetSelectionSelected, []string{"implementation.go"})
			result.TargetSelection = &selection
		}
		grounding, err = CommitPrerequisiteResult(grounding, result)
		if err != nil {
			t.Fatal(err)
		}
	}
	grounding, decision, err = ReduceRuntimeStateWithDirective(grounding, &decision.AuthoritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != runtimecontract.RuntimeActionAcquirePrerequisite || len(grounding.Prerequisites.Pending) != 1 || grounding.Prerequisites.Pending[0].Kind != runtimecontract.PrerequisiteKindTargetSnapshot || grounding.Prerequisites.Pending[0].SubjectRef != "implementation.go" {
		t.Fatalf("selected target did not require snapshot: decision=%+v prerequisites=%+v", decision, grounding.Prerequisites)
	}
	snapshotPrerequisite := grounding.Prerequisites.Pending[0]
	grounding, err = CommitPrerequisiteResult(grounding, PrerequisiteResult{
		PrerequisiteID: snapshotPrerequisite.ID,
		TargetSnapshot: &runtimecontract.TargetSnapshot{TargetRef: "implementation.go", Content: "package implementation", ContentHash: "hash-implementation", ContentState: runtimecontract.SnapshotContentStateComplete},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, decision, err = ReduceRuntimeStateWithDirective(grounding, &decision.AuthoritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != runtimecontract.RuntimeActionProduceStateChange || len(decision.RequiredOutput.TargetRefs) != 1 || decision.RequiredOutput.TargetRefs[0] != "implementation.go" {
		t.Fatalf("owner-selected target state-change decision = %+v", decision)
	}
}

func testRuntimeTargetSelection(prerequisiteID, targetRole string, disposition runtimecontract.RuntimeTargetSelectionDisposition, candidates []string) runtimecontract.RuntimeTargetSelection {
	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: prerequisiteID, OwnerModuleID: runtimecontract.RuntimeModuleFile, TargetRole: targetRole, SelectorRef: "file/unique_workspace_entry",
		Disposition: disposition, CandidateTargetRefs: append([]string(nil), candidates...), CandidateCount: len(candidates), ObservedRevision: "manifest-1",
	}
	if disposition == runtimecontract.RuntimeTargetSelectionSelected {
		selection.TargetRefs = append([]string(nil), candidates...)
	}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	return selection
}

func TestPreflightRuntimeFactsChecksAllRuntimeManagedPrerequisites(t *testing.T) {
	facts := validationFacts()
	facts.Failure = &runtimecontract.FailureFact{Signature: "failure-1", TargetRefs: []string{"main.go"}}
	requirements := []RuntimeRequirement{
		{Kind: runtimecontract.PrerequisiteKindManifest},
		{Kind: runtimecontract.PrerequisiteKindFailureTarget},
		{Kind: runtimecontract.PrerequisiteKindCapability, CapabilityClass: runtimecontract.RuntimeCapabilityClassValidation, RequiredCapabilities: []string{"shell/command"}},
		{Kind: runtimecontract.PrerequisiteKindValidationScratch},
		{Kind: runtimecontract.PrerequisiteKindProviderCeiling, MinimumBytes: 96 * 1024},
	}
	facts = PreflightRuntimeFacts(facts, requirements)
	if len(facts.Prerequisites.Pending) != len(requirements) {
		t.Fatalf("pending = %+v, want every missing managed prerequisite", facts.Prerequisites.Pending)
	}
	for _, prerequisite := range append([]runtimecontract.RuntimePrerequisite(nil), facts.Prerequisites.Pending...) {
		result := PrerequisiteResult{PrerequisiteID: prerequisite.ID}
		switch prerequisite.Kind {
		case runtimecontract.PrerequisiteKindManifest:
			result.Manifest = &runtimecontract.WorkspaceManifest{Revision: "manifest-1"}
		case runtimecontract.PrerequisiteKindFailureTarget:
			result.FailureTarget = &runtimecontract.FailureTarget{Signature: "failure-1", TargetRefs: []string{"main.go"}}
		case runtimecontract.PrerequisiteKindCapability:
			result.Capabilities = &runtimecontract.RuntimeCapabilities{StateChangeKinds: []string{"patch/apply"}, ValidationKinds: []string{"shell/command"}}
		case runtimecontract.PrerequisiteKindValidationScratch:
			result.ValidationScratch = &runtimecontract.ValidationScratch{Reference: ".workorch-validation/run-1", Revision: "scratch-1"}
		case runtimecontract.PrerequisiteKindProviderCeiling:
			result.ProviderCeiling = &runtimecontract.ProviderCeiling{
				Target:                   runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
				ContextWindowTokens:      128000,
				ProviderMaxOutputTokens:  16384,
				EffectiveMaxOutputTokens: 4096,
				MaxInputTokens:           92928,
				MaxBytes:                 128 * 1024,
				CapabilityRevision:       "capability-1",
				ConfigurationRevision:    "config-1",
			}
		}
		var err error
		facts, err = CommitPrerequisiteResult(facts, result)
		if err != nil {
			t.Fatalf("commit %s: %v", prerequisite.Kind, err)
		}
	}
	facts = PreflightRuntimeFacts(facts, requirements)
	if len(facts.Prerequisites.Pending) != 0 {
		t.Fatalf("satisfied prerequisites were scheduled again: %+v", facts.Prerequisites.Pending)
	}
}

func TestCommitPrerequisiteResultMergesCapabilitiesFromMultipleOwners(t *testing.T) {
	facts := runtimecontract.RuntimeFacts{
		Activation:  runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		WorkspaceID: "ws", RunID: "run", DefinitionRevision: "definition", EvidenceRevision: "evidence",
		Focus: runtimecontract.CoverageFocus{ID: "focus"},
	}
	requirements := []RuntimeRequirement{
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleFile, CapabilityClass: runtimecontract.RuntimeCapabilityClassStateChange, RequiredCapabilities: []string{"file/write"}},
		{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleShell, CapabilityClass: runtimecontract.RuntimeCapabilityClassValidation, RequiredCapabilities: []string{"shell/command"}},
	}
	facts = PreflightRuntimeFacts(facts, requirements)
	for _, prerequisite := range append([]runtimecontract.RuntimePrerequisite(nil), facts.Prerequisites.Pending...) {
		capabilities := runtimecontract.RuntimeCapabilities{}
		if prerequisite.CapabilityClass == runtimecontract.RuntimeCapabilityClassStateChange {
			capabilities.StateChangeKinds = prerequisite.RequiredCapabilities
		} else {
			capabilities.ValidationKinds = prerequisite.RequiredCapabilities
		}
		var err error
		facts, err = CommitPrerequisiteResult(facts, PrerequisiteResult{PrerequisiteID: prerequisite.ID, Capabilities: &capabilities})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(facts.Capabilities.StateChangeKinds) != 1 || facts.Capabilities.StateChangeKinds[0] != "file/write" ||
		len(facts.Capabilities.ValidationKinds) != 1 || facts.Capabilities.ValidationKinds[0] != "shell/command" {
		t.Fatalf("merged capabilities = %+v", facts.Capabilities)
	}
}

func TestCapabilityStatePrerequisiteRequiresOwnerConfirmedSnapshot(t *testing.T) {
	requirement := RuntimeRequirement{Kind: runtimecontract.PrerequisiteKindCapabilityState, OwnerModuleID: runtimecontract.RuntimeModuleGit, CapabilityStateKind: "repository_state", SubjectRef: "workspace_deliverable"}
	facts := PreflightRuntimeFacts(validationFacts(), []RuntimeRequirement{requirement})
	if len(facts.Prerequisites.Pending) != 1 || facts.Prerequisites.Pending[0].Kind != runtimecontract.PrerequisiteKindCapabilityState {
		t.Fatalf("pending prerequisites = %+v", facts.Prerequisites.Pending)
	}
	prerequisite := facts.Prerequisites.Pending[0]
	snapshot := runtimecontract.RuntimeCapabilityStateSnapshot{
		RequirementID: prerequisite.ID, OwnerModuleID: runtimecontract.RuntimeModuleGit, CapabilityStateKind: "repository_state", SubjectRef: "workspace_deliverable",
		ObservedRevision: "repo=absent", ContentHash: "state-hash", Disposition: runtimecontract.RuntimeCapabilityStateSequenceRequired,
		RequiredOperationSequence: []runtimecontract.RuntimeRequiredOperationVariant{
			{RequirementID: "repo-init", Kind: runtimecontract.StepKindGitWrite, VariantID: "init"},
			{RequirementID: "stage", Kind: runtimecontract.StepKindGitWrite, VariantID: "add", DependsOnRequirementIDs: []string{"repo-init"}},
		},
	}
	committed, err := CommitPrerequisiteResult(facts, PrerequisiteResult{PrerequisiteID: prerequisite.ID, CapabilityState: &snapshot})
	if err != nil {
		t.Fatal(err)
	}
	if len(committed.Prerequisites.Pending) != 0 || len(committed.Resources.CapabilityStates) != 1 || committed.Resources.CapabilityStates[0].ContentHash != "state-hash" {
		t.Fatalf("committed capability state = %+v", committed)
	}
}

func TestOperationAdmissibilityBindingRequiresCommittedOwnerCapabilityState(t *testing.T) {
	requirements := []RuntimeRequirement{
		{Kind: runtimecontract.PrerequisiteKindCapabilityState, OwnerModuleID: runtimecontract.RuntimeModuleGit, CapabilityStateKind: "repository_state", SubjectRef: "workspace_deliverable"},
		{Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleGit, RequiredCapabilities: []string{runtimecontract.StepKindGitWrite}},
	}
	baseFacts := validationFacts()
	baseFacts.ConvergencePolicy.MaxCandidateContextItems = 4
	facts := PreflightRuntimeFacts(baseFacts, requirements)
	var capability, admissibility runtimecontract.RuntimePrerequisite
	for _, prerequisite := range facts.Prerequisites.Pending {
		switch prerequisite.Kind {
		case runtimecontract.PrerequisiteKindCapabilityState:
			capability = prerequisite
		case runtimecontract.PrerequisiteKindOperationAdmissibility:
			admissibility = prerequisite
		}
	}
	inventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: admissibility.ID, OwnerModuleID: runtimecontract.RuntimeModuleGit, OperationKinds: []string{runtimecontract.StepKindGitWrite},
		ObservedRevision: "repo-v1", AcquisitionBinding: "capability-state/ws/run/git/0",
	}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	if _, err := CommitPrerequisiteResult(facts, PrerequisiteResult{PrerequisiteID: admissibility.ID, OperationAdmissibility: &inventory}); err == nil {
		t.Fatal("bound inventory was committed without its capability-state acquisition")
	}
	snapshot := runtimecontract.RuntimeCapabilityStateSnapshot{
		RequirementID: capability.ID, AcquisitionKey: "capability-state/ws/run/git/0", AcquisitionBinding: "capability-state/ws/run/git/0",
		OwnerModuleID: runtimecontract.RuntimeModuleGit, CapabilityStateKind: "repository_state", SubjectRef: "workspace_deliverable",
		ObservedRevision: "repo-v1", ContentHash: "hash-v1", Disposition: runtimecontract.RuntimeCapabilityStateReady,
		QueryReceiptRef: "capability-state-receipt/capability-state/ws/run/git/0",
	}
	snapshot.SnapshotID = runtimecontract.StableRuntimeCapabilityStateSnapshotID(snapshot)
	committed, err := CommitPrerequisiteResult(facts, PrerequisiteResult{PrerequisiteID: capability.ID, CapabilityState: &snapshot})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CommitPrerequisiteResult(committed, PrerequisiteResult{PrerequisiteID: admissibility.ID, OperationAdmissibility: &inventory}); err != nil {
		t.Fatalf("bound inventory rejected after matching capability-state acquisition: %v", err)
	}
}

func TestMergeCapabilityKindsDoesNotOverwrite(t *testing.T) {
	existing := []string{"file/read", "validation/managed_session"}
	incoming := []string{"validation/managed_session", "shell/command"}
	got := mergeCapabilityKinds(existing, incoming)
	want := []string{"file/read", "validation/managed_session", "shell/command"}
	if len(got) != len(want) {
		t.Fatalf("merge = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("merge = %v, want %v", got, want)
		}
	}
}

func TestApplyCapabilityPrerequisiteMergesOwners(t *testing.T) {
	facts := runtimecontract.RuntimeFacts{
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "definition-1",
		Activation:   runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		Focus:        runtimecontract.CoverageFocus{ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}},
		Capabilities: runtimecontract.RuntimeCapabilities{ValidationKinds: []string{"file/read"}},
		Prerequisites: runtimecontract.PrerequisiteState{Pending: []runtimecontract.RuntimePrerequisite{{
			ID: "cap-shell", Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleShell,
			CapabilityClass: runtimecontract.RuntimeCapabilityClassValidation, RequiredCapabilities: []string{"validation/managed_session"},
		}}},
	}
	facts = facts.Normalized()
	err := applyPrerequisiteResult(&facts, runtimecontract.RuntimePrerequisite{
		ID: "cap-shell", Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleShell,
		CapabilityClass: runtimecontract.RuntimeCapabilityClassValidation, RequiredCapabilities: []string{"validation/managed_session"},
	}, PrerequisiteResult{
		PrerequisiteID: "cap-shell",
		Capabilities:   &runtimecontract.RuntimeCapabilities{ValidationKinds: []string{"validation/managed_session"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(facts.Capabilities.ValidationKinds) != 2 || facts.Capabilities.ValidationKinds[0] != "file/read" || facts.Capabilities.ValidationKinds[1] != "validation/managed_session" {
		t.Fatalf("merged capabilities = %+v", facts.Capabilities)
	}
}
