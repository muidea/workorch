package runtimev2

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestGatewayPreflightAcquiresSnapshotsBeforeStateChange(t *testing.T) {
	facts := runtimecontract.RuntimeFacts{
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
			result.ProviderCeiling = &runtimecontract.ProviderCeiling{ProviderID: "provider-1", MaxBytes: 128 * 1024, Revision: "ceiling-1"}
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
