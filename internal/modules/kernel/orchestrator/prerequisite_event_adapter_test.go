package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	filemodule "github.com/muidea/workorch/internal/modules/blocks/file"
	llmmodule "github.com/muidea/workorch/internal/modules/blocks/llm"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	runtimev2 "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtimev2"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

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
	if err := llm.Configure(llmmodule.Dependencies{
		EventHub: hub,
		ProviderConfigLoader: func() (string, map[string]llmmodels.ProviderConfig, error) {
			return "provider-1", map[string]llmmodels.ProviderConfig{
				"provider-1": {ID: "provider-1", ContextProfile: llmmodels.ProviderContextProfile{MaxRequestBytes: 4 * 1024 * 1024}},
			}, nil
		},
		ProviderConfigRevision: func() uint64 { return 7 },
	}); err != nil {
		t.Fatal(err)
	}
	if err := llm.Start(); err != nil {
		t.Fatal(err)
	}
	defer llm.Stop(ctx)

	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "go.mod"), []byte("module example.com/runtimev2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	facts := runtimev2.PreflightRuntimeFacts(runtimecontract.RuntimeFacts{
		WorkspaceID:        "ws-1",
		RunID:              "run-1",
		DefinitionRevision: "definition-1",
		EvidenceRevision:   "evidence-1",
		Focus:              runtimecontract.CoverageFocus{ID: "cwi-1"},
	}, []runtimev2.RuntimeRequirement{
		{Kind: runtimecontract.PrerequisiteKindManifest},
		{Kind: runtimecontract.PrerequisiteKindCapability, SubjectRef: filemodule.ModuleID, CapabilityClass: runtimecontract.RuntimeCapabilityClassStateChange, RequiredCapabilities: []string{filemodule.StepKindPatchApply}},
		{Kind: runtimecontract.PrerequisiteKindProviderCeiling, MinimumBytes: 32 * 1024},
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
				WorkspaceID: facts.WorkspaceID, RunID: facts.RunID, AuthoritySnapshotID: "authority-1", PrerequisiteID: item.ID, MinimumBytes: 64 * 1024,
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
	if facts.Resources.ProviderCeiling == nil || facts.Resources.ProviderCeiling.MaxBytes != 4*1024*1024 || facts.Resources.ProviderCeiling.ProviderID == "" {
		t.Fatalf("provider ceiling = %+v", facts.Resources.ProviderCeiling)
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
	if err := llm.Configure(llmmodule.Dependencies{
		EventHub: hub,
		ProviderConfigLoader: func() (string, map[string]llmmodels.ProviderConfig, error) {
			return "provider-1", map[string]llmmodels.ProviderConfig{
				"provider-1": {ID: "provider-1", ContextProfile: llmmodels.ProviderContextProfile{MaxRequestBytes: 16 * 1024}},
			}, nil
		},
		ProviderConfigRevision: func() uint64 { return 1 },
	}); err != nil {
		t.Fatal(err)
	}
	if err := llm.Start(); err != nil {
		t.Fatal(err)
	}
	defer llm.Stop(ctx)

	facts := runtimev2.PreflightRuntimeFacts(runtimecontract.RuntimeFacts{
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", EvidenceRevision: "evidence-1",
		Focus: runtimecontract.CoverageFocus{ID: "focus-1"},
	}, []runtimev2.RuntimeRequirement{{Kind: runtimecontract.PrerequisiteKindProviderCeiling, MinimumBytes: 32 * 1024}})
	item := facts.Prerequisites.Pending[0]
	_, err := AcquireAndCommitProviderCeilingPrerequisite(ctx, hub, facts, orchestratorevents.AcquireProviderCeilingCommand{
		WorkspaceID: facts.WorkspaceID, RunID: facts.RunID, AuthoritySnapshotID: "authority-1",
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
	facts := runtimev2.PreflightRuntimeFacts(runtimecontract.RuntimeFacts{
		WorkspaceID:        "ws-1",
		RunID:              "run-1",
		DefinitionRevision: "definition-1",
		EvidenceRevision:   "evidence-1",
		Focus:              runtimecontract.CoverageFocus{ID: "cwi-1"},
	}, []runtimev2.RuntimeRequirement{{Kind: runtimecontract.PrerequisiteKindTargetSnapshot, SubjectRef: "main.go"}})
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
