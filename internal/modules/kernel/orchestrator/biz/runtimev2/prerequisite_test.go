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

func TestPreflightRuntimeFactsChecksAllRuntimeManagedPrerequisites(t *testing.T) {
	facts := validationFacts()
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
