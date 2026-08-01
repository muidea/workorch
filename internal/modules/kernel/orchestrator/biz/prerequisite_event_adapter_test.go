package orchestrator

// Owner Biz prerequisite tests.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	filemodule "github.com/muidea/workorch/internal/modules/blocks/file"
	llmmodule "github.com/muidea/workorch/internal/modules/blocks/llm"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestAcquireRuntimeCapabilitiesPrerequisiteReturnsTypedOwnerUnavailable(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(8)
	defer hub.Terminate(ctx)
	facts := runtime.PreflightRuntimeFacts(runtimecontract.RuntimeFacts{
		WorkspaceID: "ws-capability", RunID: "run-capability", DefinitionRevision: "definition-1", EvidenceRevision: "evidence-1",
		Focus: runtimecontract.CoverageFocus{ID: "focus-1"},
	}, []runtime.RuntimeRequirement{{
		Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleWeb,
		CapabilityClass: runtimecontract.RuntimeCapabilityClassValidation, RequiredCapabilities: []string{runtimecontract.StepKindWebFetch},
	}})
	prerequisite := facts.Prerequisites.Pending[0]
	_, err := AcquireAndCommitRuntimeCapabilitiesPrerequisite(ctx, hub, facts, orchestratorevents.AcquireRuntimeCapabilitiesCommand{
		WorkspaceID: facts.WorkspaceID, RunID: facts.RunID, AuthoritySnapshotID: "authority-1", PrerequisiteID: prerequisite.ID,
		ModuleID: prerequisite.OwnerModuleID, CapabilityClass: prerequisite.CapabilityClass, RequiredKinds: prerequisite.RequiredCapabilities,
	})
	var unsupported *orchestratorevents.RuntimeCapabilityUnsupportedError
	if !errors.As(err, &unsupported) || unsupported.ModuleID != runtimecontract.RuntimeModuleWeb || unsupported.Reason != "owner_unavailable" {
		t.Fatalf("missing capability owner error = %#v, want typed owner_unavailable", err)
	}
	if len(facts.Capabilities.StateChangeKinds) != 0 || len(facts.Capabilities.ValidationKinds) != 0 || len(facts.Prerequisites.Satisfied) != 0 {
		t.Fatalf("absent owner mutated prerequisite facts: %+v", facts)
	}
}

func TestAcquireTargetSnapshotPrerequisiteUsesTypedEventHubContract(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	defer hub.Terminate(ctx)
	background := magictask.NewBackgroundRoutine(2)
	defer background.Shutdown(ctx)
	file := filemodule.New()
	if err := file.Configure(filemodule.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := file.Start(); err != nil {
		t.Fatal(err)
	}
	defer file.Stop(ctx)

	workspace := t.TempDir()
	content := "module example.com/gateway\n\ngo 1.24\n"
	if err := os.WriteFile(filepath.Join(workspace, "go.mod"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := AcquireTargetSnapshotPrerequisite(ctx, hub, orchestratorevents.AcquireTargetSnapshotCommand{
		WorkspaceID:                   "ws-gateway",
		RunID:                         "run-gateway",
		AuthoritySnapshotID:           "authority-1",
		PrerequisiteID:                "target_snapshot:go.mod::0",
		WorkspaceDeliverableDirectory: workspace,
		TargetRef:                     "go.mod",
	})
	if err != nil {
		t.Fatalf("AcquireTargetSnapshotPrerequisite() error = %v", err)
	}
	if result.Snapshot.TargetRef != "go.mod" || result.Snapshot.Content != content || result.Snapshot.ContentState != runtimecontract.SnapshotContentStateComplete || result.Snapshot.ContentHash == "" {
		t.Fatalf("snapshot = %+v, want complete typed go.mod snapshot", result.Snapshot)
	}
}

func TestAcquireRuntimeTargetSelectionPrerequisiteUsesTypedEventHubContract(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	defer hub.Terminate(ctx)
	background := magictask.NewBackgroundRoutine(2)
	defer background.Shutdown(ctx)
	file := filemodule.New()
	if err := file.Configure(filemodule.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := file.Start(); err != nil {
		t.Fatal(err)
	}
	defer file.Stop(ctx)

	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "implementation.txt"), []byte("deliverable\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := AcquireRuntimeTargetSelectionPrerequisite(ctx, hub, orchestratorevents.AcquireRuntimeTargetSelectionCommand{
		WorkspaceID: "ws-selection", RunID: "run-selection", AuthoritySnapshotID: "authority-1",
		PrerequisiteID: "target_selection:blocks/file:deliverable:file/unique_workspace_entry:::0", WorkspaceDeliverableDirectory: workspace,
		TargetRole: "deliverable", TargetSelectorRef: "file/unique_workspace_entry", MaxItems: 4,
	})
	if err != nil {
		t.Fatalf("AcquireRuntimeTargetSelectionPrerequisite() error = %v", err)
	}
	if result.Selection.Disposition != runtimecontract.RuntimeTargetSelectionSelected || result.Selection.SelectorRef != "file/unique_workspace_entry" || len(result.Selection.TargetRefs) != 1 || result.Selection.TargetRefs[0] != "implementation.txt" || result.Observation == nil {
		t.Fatalf("target selection result = %+v", result)
	}
	_, err = AcquireRuntimeTargetSelectionPrerequisite(ctx, hub, orchestratorevents.AcquireRuntimeTargetSelectionCommand{
		WorkspaceID: "ws-selection", RunID: "run-selection", AuthoritySnapshotID: "authority-1",
		PrerequisiteID: "target_selection:blocks/file:deliverable:file/unique_workspace_entry:::overflow", WorkspaceDeliverableDirectory: workspace,
		TargetRole: "deliverable", TargetSelectorRef: "file/unique_workspace_entry", MaxItems: runtimecontract.RuntimeOwnerInventoryItemsMaxCount + 1,
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("target selection accepted an out-of-bound owner inventory size: %v", err)
	}
	emptyWorkspace := t.TempDir()
	creation, err := AcquireRuntimeTargetSelectionPrerequisite(ctx, hub, orchestratorevents.AcquireRuntimeTargetSelectionCommand{
		WorkspaceID: "ws-selection", RunID: "run-selection", AuthoritySnapshotID: "authority-1",
		PrerequisiteID: "target_selection:blocks/file:deliverable:file/workspace_deliverable:::0", WorkspaceDeliverableDirectory: emptyWorkspace,
		TargetRole: "deliverable", TargetSelectorRef: "file/workspace_deliverable", ProspectiveTargetRef: "initial-deliverable.txt", MaxItems: 4,
	})
	if err != nil {
		t.Fatalf("AcquireRuntimeTargetSelectionPrerequisite() empty workspace error = %v", err)
	}
	if creation.Selection.Disposition != runtimecontract.RuntimeTargetSelectionCreate || creation.Selection.ProspectiveTargetRef != "initial-deliverable.txt" || creation.Selection.CandidateCount != 0 || len(creation.Selection.TargetRefs) != 0 || creation.Observation == nil ||
		len(creation.Observation.Dependencies) != 1 || creation.Observation.Dependencies[0].Domain != runtimecontract.RuntimeResourceWorkspaceTree || creation.Observation.Dependencies[0].Revision != creation.Selection.ObservedRevision {
		t.Fatalf("empty workspace creation selection = %+v", creation)
	}
	unsupported, err := AcquireRuntimeTargetSelectionPrerequisite(ctx, hub, orchestratorevents.AcquireRuntimeTargetSelectionCommand{
		WorkspaceID: "ws-selection", RunID: "run-selection", AuthoritySnapshotID: "authority-1",
		PrerequisiteID: "target_selection:blocks/file:deliverable:profile/free_form_selector:::0", WorkspaceDeliverableDirectory: workspace,
		TargetRole: "deliverable", TargetSelectorRef: "profile/free_form_selector", MaxItems: 4,
	})
	if err != nil || unsupported.Selection.Disposition != runtimecontract.RuntimeTargetSelectionUnsupported || unsupported.Observation == nil {
		t.Fatalf("unsupported File selector result = %+v err=%v", unsupported, err)
	}
	profileDeclared, err := AcquireRuntimeTargetSelectionPrerequisite(ctx, hub, orchestratorevents.AcquireRuntimeTargetSelectionCommand{
		WorkspaceID: "ws-selection", RunID: "run-selection", AuthoritySnapshotID: "authority-1",
		PrerequisiteID: "target_selection:blocks/file:deliverable:file/profile_declared_candidates:profile-deliverable:implementation:implementation.txt::0", WorkspaceDeliverableDirectory: workspace,
		TargetRole: "deliverable", TargetSelectorRef: "file/profile_declared_candidates", TargetSelectorOwnerRef: "profile-deliverable", TargetSelectorSubjectRef: "implementation", TargetSelectorCandidateRefs: []string{"implementation.txt"}, MaxItems: 4,
	})
	if err != nil || profileDeclared.Selection.Disposition != runtimecontract.RuntimeTargetSelectionSelected || profileDeclared.Selection.SelectorOwnerRef != "profile-deliverable" || profileDeclared.Selection.SelectorSubjectRef != "implementation" || !runtimecontract.SameRuntimeTargetSelectorCandidateRefs(profileDeclared.Selection.SelectorCandidateRefs, []string{"implementation.txt"}) || profileDeclared.Observation == nil {
		t.Fatalf("profile declared File selector result = %+v err=%v", profileDeclared, err)
	}
}

func TestAcquireRuntimeTargetSelectionPrerequisiteRejectsUnboundSelectionObservation(t *testing.T) {
	ctx := context.Background()
	command := orchestratorevents.AcquireRuntimeTargetSelectionCommand{
		WorkspaceID: "ws-selection", RunID: "run-selection", AuthoritySnapshotID: "authority-1",
		PrerequisiteID: "target_selection:blocks/file:implementation:file/unique_workspace_entry:::0", WorkspaceDeliverableDirectory: t.TempDir(),
		TargetRole: "implementation", TargetSelectorRef: "file/unique_workspace_entry", MaxItems: 4,
	}
	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: command.PrerequisiteID, OwnerModuleID: runtimecontract.RuntimeModuleFile, TargetRole: command.TargetRole,
		SelectorRef: command.TargetSelectorRef, Disposition: runtimecontract.RuntimeTargetSelectionSelected,
		TargetRefs: []string{"implementation.txt"}, CandidateTargetRefs: []string{"implementation.txt"}, CandidateCount: 1, ObservedRevision: "tree-v1",
	}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	validObservation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: command.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindTargetSelection, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		ContentHash: selection.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable", Revision: selection.ObservedRevision,
		}},
	}
	validObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(validObservation)

	for _, testCase := range []struct {
		name        string
		selection   runtimecontract.RuntimeTargetSelection
		observation *runtimecontract.RuntimePrerequisiteObservation
	}{
		{name: "missing observation", selection: selection},
		{name: "role mismatch", selection: func() runtimecontract.RuntimeTargetSelection {
			value := selection
			value.TargetRole = "validation_entry"
			value.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(value)
			return value
		}(), observation: &validObservation},
		{name: "tree revision mismatch", selection: selection, observation: func() *runtimecontract.RuntimePrerequisiteObservation {
			value := validObservation
			value.Dependencies[0].Revision = "tree-v0"
			value.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(value)
			return &value
		}()},
		{name: "selection hash mismatch", selection: selection, observation: func() *runtimecontract.RuntimePrerequisiteObservation {
			value := validObservation
			value.ContentHash = "target-selection-other"
			value.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(value)
			return &value
		}()},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			hub := magicevent.NewHub(4)
			t.Cleanup(func() { hub.Terminate(ctx) })
			observer := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleFile, hub)
			observer.Subscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeTargetSelection, func(_ magicevent.Event, result magicevent.Result) {
				result.Set(orchestratorevents.AcquireRuntimeTargetSelectionResult{
					PrerequisiteID: command.PrerequisiteID, Selection: testCase.selection, Observation: testCase.observation,
				}, nil)
			})
			t.Cleanup(func() { observer.Unsubscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeTargetSelection) })
			if _, err := AcquireRuntimeTargetSelectionPrerequisite(ctx, hub, command); err == nil {
				t.Fatalf("target selection accepted %s", testCase.name)
			}
		})
	}
}

func TestFilePrerequisiteObservationBindingsRejectMissingOrStaleFacts(t *testing.T) {
	snapshot := runtimecontract.TargetSnapshot{
		TargetRef: "implementation.txt", Content: "implementation\n", ContentHash: "content-v1", ContentState: runtimecontract.SnapshotContentStateComplete,
	}
	snapshotObservation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: "target_snapshot:implementation.txt::0", Kind: runtimecontract.PrerequisiteKindTargetSnapshot, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		ContentHash: snapshot.ContentHash,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTarget, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: snapshot.TargetRef, Revision: snapshot.ContentHash,
		}},
	}
	snapshotObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(snapshotObservation)
	manifest := runtimecontract.WorkspaceManifest{Revision: "tree-v1", TargetRefs: []string{snapshot.TargetRef}}
	manifestObservation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: "manifest:blocks/file:::::0", Kind: runtimecontract.PrerequisiteKindManifest, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		ContentHash: manifest.Revision,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable", Revision: manifest.Revision,
		}},
	}
	manifestObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(manifestObservation)

	if err := validateFileTargetSnapshotObservation(snapshotObservation.PrerequisiteID, snapshot, &snapshotObservation); err != nil {
		t.Fatalf("valid target snapshot observation rejected: %v", err)
	}
	if err := validateFileWorkspaceManifestObservation(manifestObservation.PrerequisiteID, manifest, &manifestObservation); err != nil {
		t.Fatalf("valid manifest observation rejected: %v", err)
	}
	if err := validateFileTargetSnapshotObservation(snapshotObservation.PrerequisiteID, snapshot, nil); err == nil {
		t.Fatal("target snapshot accepted a missing freshness observation")
	}
	staleSnapshotObservation := snapshotObservation
	staleSnapshotObservation.Dependencies[0].Revision = "content-v0"
	staleSnapshotObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(staleSnapshotObservation)
	if err := validateFileTargetSnapshotObservation(snapshotObservation.PrerequisiteID, snapshot, &staleSnapshotObservation); err == nil {
		t.Fatal("target snapshot accepted a stale target revision")
	}
	if err := validateFileWorkspaceManifestObservation(manifestObservation.PrerequisiteID, manifest, nil); err == nil {
		t.Fatal("workspace manifest accepted a missing freshness observation")
	}
	staleManifestObservation := manifestObservation
	staleManifestObservation.Dependencies[0].Revision = "tree-v0"
	staleManifestObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(staleManifestObservation)
	if err := validateFileWorkspaceManifestObservation(manifestObservation.PrerequisiteID, manifest, &staleManifestObservation); err == nil {
		t.Fatal("workspace manifest accepted a stale tree revision")
	}
}

func TestAcquireAndCommitManifestAndProviderCeilingPrerequisites(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	defer hub.Terminate(ctx)
	file := filemodule.New()
	if err := file.Configure(filemodule.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := file.Start(); err != nil {
		t.Fatal(err)
	}
	defer file.Stop(ctx)
	llm := llmmodule.New()
	useTestLLMConfig(t, 128000, 8192)
	if err := llm.Configure(llmmodule.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := llm.Start(); err != nil {
		t.Fatal(err)
	}
	defer llm.Stop(ctx)

	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "go.mod"), []byte("module example.com/runtime\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	facts := runtime.PreflightRuntimeFacts(runtimecontract.RuntimeFacts{
		WorkspaceID:        "ws-1",
		RunID:              "run-1",
		DefinitionRevision: "definition-1",
		EvidenceRevision:   "evidence-1",
		Focus:              runtimecontract.CoverageFocus{ID: "cwi-1"},
		Activation:         runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "custom-test", ModelID: "model-1"}},
	}, []runtime.RuntimeRequirement{
		{Kind: runtimecontract.PrerequisiteKindManifest, OwnerModuleID: filemodule.ModuleID},
		{Kind: runtimecontract.PrerequisiteKindCapability, SubjectRef: filemodule.ModuleID, CapabilityClass: runtimecontract.RuntimeCapabilityClassStateChange, RequiredCapabilities: []string{filemodule.StepKindPatchApply}},
		{Kind: runtimecontract.PrerequisiteKindProviderCeiling, OwnerModuleID: llmmodule.ModuleID, MinimumBytes: 32 * 1024},
	})
	for _, item := range append([]runtimecontract.RuntimePrerequisite(nil), facts.Prerequisites.Pending...) {
		var err error
		switch item.Kind {
		case runtimecontract.PrerequisiteKindManifest:
			facts, err = AcquireAndCommitWorkspaceManifestPrerequisite(ctx, hub, facts, orchestratorevents.AcquireWorkspaceManifestCommand{
				WorkspaceID: facts.WorkspaceID, RunID: facts.RunID, AuthoritySnapshotID: "authority-1", PrerequisiteID: item.ID, WorkspaceDeliverableDirectory: workspace,
			})
		case runtimecontract.PrerequisiteKindProviderCeiling:
			facts, err = AcquireAndCommitProviderCeilingPrerequisite(ctx, hub, facts, orchestratorevents.AcquireProviderCeilingCommand{
				WorkspaceID: facts.WorkspaceID, RunID: facts.RunID, AuthoritySnapshotID: "authority-1", PrerequisiteID: item.ID,
				Target: facts.Activation.ProviderTarget, MinimumBytes: 64 * 1024,
			})
		case runtimecontract.PrerequisiteKindCapability:
			facts, err = AcquireAndCommitRuntimeCapabilitiesPrerequisite(ctx, hub, facts, orchestratorevents.AcquireRuntimeCapabilitiesCommand{
				WorkspaceID: facts.WorkspaceID, RunID: facts.RunID, AuthoritySnapshotID: "authority-1", PrerequisiteID: item.ID,
				ModuleID: item.SubjectRef, CapabilityClass: item.CapabilityClass, RequiredKinds: item.RequiredCapabilities,
			})
		}
		if err != nil {
			t.Fatalf("acquire %s: %v", item.Kind, err)
		}
	}
	if facts.Resources.Manifest == nil || facts.Resources.Manifest.Revision == "" || len(facts.Resources.Manifest.TargetRefs) != 1 || facts.Resources.Manifest.TargetRefs[0] != "go.mod" {
		t.Fatalf("manifest = %+v", facts.Resources.Manifest)
	}
	if facts.Resources.ProviderCeiling == nil ||
		facts.Resources.ProviderCeiling.MaxBytes < 64*1024 ||
		facts.Resources.ProviderCeiling.Target.ProviderID != "custom-test" ||
		facts.Resources.ProviderCeiling.Target.ModelID != "model-1" ||
		facts.Resources.ProviderCeiling.CapabilityRevision == "" ||
		facts.Resources.ProviderCeiling.ConfigurationRevision == "" {
		t.Fatalf("provider ceiling = %+v", facts.Resources.ProviderCeiling)
	}
	providerPrerequisiteID := ""
	for _, prerequisite := range facts.Prerequisites.Satisfied {
		if prerequisite.Kind == runtimecontract.PrerequisiteKindProviderCeiling {
			providerPrerequisiteID = prerequisite.ID
			break
		}
	}
	providerObservation, found := runtimecontract.RuntimePrerequisiteObservationForID(facts.Resources.PrerequisiteObservations, providerPrerequisiteID)
	if !found || providerObservation.Kind != runtimecontract.PrerequisiteKindProviderCeiling || providerObservation.OwnerModuleID != llmmodule.ModuleID ||
		len(providerObservation.Dependencies) != 1 || providerObservation.Dependencies[0].Domain != runtimecontract.RuntimeResourceProviderConfig ||
		providerObservation.Dependencies[0].Revision != facts.Resources.ProviderCeiling.ConfigurationRevision {
		t.Fatalf("provider ceiling observation = %+v", providerObservation)
	}
	if len(facts.Capabilities.StateChangeKinds) != 1 || facts.Capabilities.StateChangeKinds[0] != filemodule.StepKindPatchApply {
		t.Fatalf("runtime capabilities = %+v", facts.Capabilities)
	}
}

func TestProviderCeilingPrerequisiteRejectsProviderBelowRequiredMinimum(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	defer hub.Terminate(ctx)
	llm := llmmodule.New()
	useTestLLMConfig(t, 8192, 4096)
	if err := llm.Configure(llmmodule.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := llm.Start(); err != nil {
		t.Fatal(err)
	}
	defer llm.Stop(ctx)

	facts := runtime.PreflightRuntimeFacts(runtimecontract.RuntimeFacts{
		Activation:  runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "custom-test", ModelID: "model-1"}},
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", EvidenceRevision: "evidence-1",
		Focus: runtimecontract.CoverageFocus{ID: "focus-1"},
	}, []runtime.RuntimeRequirement{{Kind: runtimecontract.PrerequisiteKindProviderCeiling, MinimumBytes: 32 * 1024}})
	item := facts.Prerequisites.Pending[0]
	_, err := AcquireAndCommitProviderCeilingPrerequisite(ctx, hub, facts, orchestratorevents.AcquireProviderCeilingCommand{
		WorkspaceID: facts.WorkspaceID, RunID: facts.RunID, AuthoritySnapshotID: "authority-1", Target: facts.Activation.ProviderTarget,
		PrerequisiteID: item.ID, MinimumBytes: item.MinimumBytes,
	})
	if err == nil || !strings.Contains(err.Error(), "below required minimum") {
		t.Fatalf("provider ceiling error = %v, want minimum admission failure", err)
	}
}

func TestAcquireAndCommitTargetSnapshotPrerequisiteAdvancesTypedFacts(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	defer hub.Terminate(ctx)
	background := magictask.NewBackgroundRoutine(2)
	defer background.Shutdown(ctx)
	file := filemodule.New()
	if err := file.Configure(filemodule.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := file.Start(); err != nil {
		t.Fatal(err)
	}
	defer file.Stop(ctx)

	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	facts := runtime.PreflightRuntimeFacts(runtimecontract.RuntimeFacts{
		Activation:         runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "custom-test", ModelID: "model-1"}},
		WorkspaceID:        "ws-1",
		RunID:              "run-1",
		DefinitionRevision: "definition-1",
		EvidenceRevision:   "evidence-1",
		Focus:              runtimecontract.CoverageFocus{ID: "cwi-1"},
	}, []runtime.RuntimeRequirement{{
		Kind:          runtimecontract.PrerequisiteKindTargetSnapshot,
		OwnerModuleID: runtimecontract.RuntimeModuleFile,
		SubjectRef:    "main.go",
	}})
	item := facts.Prerequisites.Pending[0]
	committed, err := AcquireAndCommitTargetSnapshotPrerequisite(ctx, hub, facts, orchestratorevents.AcquireTargetSnapshotCommand{
		WorkspaceID:                   facts.WorkspaceID,
		RunID:                         facts.RunID,
		AuthoritySnapshotID:           "authority-1",
		PrerequisiteID:                item.ID,
		WorkspaceDeliverableDirectory: workspace,
		TargetRef:                     item.SubjectRef,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(committed.Prerequisites.Pending) != 0 || len(committed.Prerequisites.Satisfied) != 1 || len(committed.Resources.TargetSnapshots) != 1 || !committed.Resources.TargetSnapshots[0].Complete() {
		t.Fatalf("committed facts = %+v, want one completed snapshot prerequisite", committed)
	}
}

func useTestLLMConfig(t *testing.T, contextWindowTokens int, maxOutputTokens int) {
	t.Helper()
	configDir := t.TempDir()
	content := fmt.Sprintf(`endpointName = "workorch"

[server]
addr = "127.0.0.1:18080"

[repository]
backend = "file"
dataDir = ".workorch-data"

[llm.aiProxy]
baseURL = "http://127.0.0.1:9"
timeout = "180s"
catalogRequestTimeout = "5s"
catalogTTL = "5m"
catalogMaxStaleAge = "24h"
maxCatalogResponseBytes = 1048576
maxCatalogModels = 1000

[llm.defaultChat]
providerID = "custom-test"
modelID = "model-1"

[llm.budget]
contextUtilizationRatio = 0.75
estimatedTokensPerByte = 0.25

[llm.customProviders.custom-test]
provider = "compatible"
apiSurface = "openai_compatible_http"
baseURL = "http://127.0.0.1:9"
timeout = "30s"

[llm.customProviders.custom-test.models.model-1]
contextWindowTokens = %d
maxOutputTokens = %d
operations = ["chat_completions"]
`, contextWindowTokens, maxOutputTokens)
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_ = workorchconfig.Close()
	t.Setenv(workorchconfig.EnvConfigDir, configDir)
	t.Cleanup(func() { _ = workorchconfig.Close() })
	if _, err := workorchconfig.Current(); err != nil {
		t.Fatalf("current config: %v", err)
	}
}
