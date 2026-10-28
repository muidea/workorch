package eventmapping

import (
	"reflect"
	"testing"

	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/soulbinding"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func compilationMappingFixture(t *testing.T) *workspace.Workspace {
	t.Helper()
	r := authoringMappingFixture(t)
	d := workspace.NewDefinitionAuthoringDiagnostic("原错误\r\n", []byte{0xff, 0, '\r', '\n'})
	r.DerivationRequests[0].Diagnostic = &d
	r.DerivationRequests[0].SuccessAudit = &workspace.DefinitionAuthoringSuccessByteAudit{Document: workspace.DefinitionProviderObservation{Content: []byte("derive original\r\n")}, Wire: workspace.DefinitionProviderObservation{Content: []byte("derive wire\r\n")}}
	d = d.Clone()
	r.SpecificationRequests[0].Diagnostic = &d
	r.PreparationDiagnostics = []workspace.DefinitionAuthoringPreparationDiagnostic{
		workspace.NewDefinitionAuthoringPreparationDiagnostic(workspace.DefinitionAuthoringPreparationDiagnostic{
			WorkspaceID: r.Input.WorkspaceID, InputHash: r.InputHash, TemplateSnapshotHash: r.TemplateSnapshotHash,
			ObservedWorkspaceRevision: 2, Stage: workspace.DefinitionAuthoringPreparationDerivation,
			TargetRequestID: "another-request", PhaseCode: "render",
		}, []byte{0xfe, 0, '\r', '\n'}),
	}
	c := workspace.WorkspaceConfigurationSnapshot{Directory: "project", InternalDirectory: "private", TemplateID: "template", ProfileID: "profile", ContextHints: r.TemplateSnapshot.ContextHints.Clone(), EvidencePolicies: workorcheventpayload.CloneSlice(r.TemplateSnapshot.EvidencePolicies)}
	return &workspace.Workspace{ID: r.Input.WorkspaceID, Revision: 2, DefinitionAuthoring: &r,
		DefinitionAuthoringHistory: []workspace.DefinitionAuthoringConfirmedRevision{{Record: r.Clone()}},
		PreparationSealEvidence:    []workspace.WorkspacePreparationSealEvidence{{ExecutionInputs: &workspace.PreparationExecutionInputs{Version: "test", Template: r.TemplateSnapshot.Clone(), Configuration: c}}},
	}
}

func TestPreparationCompilationSourcePreservesRawAndDetachedInputs(t *testing.T) {
	w := compilationMappingFixture(t)
	w.PreparationSealEvidence[0].FinalOwners = &workspace.PreparationOwnerRevalidation{
		Souls:  &workspace.PreparationSoulFacts{Souls: []*soulmodel.Soul{{SoulMD: "原文\r\n", AllowedSkillRefs: []string{}}}},
		Memory: &workspace.PreparationMemoryInventoryFacts{Observation: memorymodel.PreparationInventory{Items: []*memorymodel.MemoryItem{{Content: "原始库存\x00\r\n"}}}},
	}
	s := PreparationCompilationSource(w)
	model, err := s.Model()
	if err != nil || !reflect.DeepEqual(model.DefinitionAuthoring, w.DefinitionAuthoring) || !reflect.DeepEqual(model.DefinitionAuthoringHistory, w.DefinitionAuthoringHistory) || !reflect.DeepEqual(model.PreparationSealEvidence, w.PreparationSealEvidence) {
		t.Fatal("private conversion lost original diagnostics, history or policy", err)
	}
	if len(s.Authoring.Record.PreparationDiagnostics) != 0 || len(s.Authoring.Diagnostics[0].Cause) != 4 || s.Authoring.Record.SpecificationRequests[0].Diagnostic.ParseError != "原错误\r\n" {
		t.Fatal("ordinary metadata projection replaced original bytes")
	}
	s.Authoring.Diagnostics[0].Cause[0] = 'x'
	s.Authoring.Record.DerivationRequests[0].Diagnostic.Payload[0] = 'x'
	s.Authoring.Record.DerivationRequests[0].SuccessAudit.Document.Content[0] = 'x'
	s.History[0].Authoring.Record.DerivationRequests[0].SuccessAudit.Wire.Content[0] = 'x'
	s.Authoring.Record.SpecificationRequests[0].Diagnostic.Payload[0] = 'x'
	s.History[0].Authoring.Diagnostics[0].Cause[0] = 'x'
	s.Seals[0].Inputs.Configuration.EvidencePolicies[0].Fields[0].Value.Array[0].String = "changed"
	s.Seals[0].FinalOwners.Souls.Souls[0].SoulMD = "source-changed"
	s.Seals[0].FinalOwners.Memory.Observation.Items[0].Content = "source-changed"
	if !reflect.DeepEqual(model.DefinitionAuthoring, w.DefinitionAuthoring) || !reflect.DeepEqual(model.PreparationSealEvidence, w.PreparationSealEvidence) || !reflect.DeepEqual(model.DefinitionAuthoringHistory, w.DefinitionAuthoringHistory) {
		t.Fatal("source aliases owner or converted model")
	}
	model.DefinitionAuthoring.PreparationDiagnostics[0].Cause[0] = 'y'
	model.PreparationSealEvidence[0].ExecutionInputs.Template.SoulBindings["planning"] = "changed"
	model.PreparationSealEvidence[0].FinalOwners.Memory.Observation.Items[0].Content = "model-changed"
	if w.DefinitionAuthoring.PreparationDiagnostics[0].Cause[0] != 0xfe || w.PreparationSealEvidence[0].ExecutionInputs.Template.SoulBindings["planning"] != "planner" {
		t.Fatal("converted model aliases original owner input")
	}
	if w.PreparationSealEvidence[0].FinalOwners.Memory.Observation.Items[0].Content != "原始库存\x00\r\n" {
		t.Fatal("final owner observations alias converted model")
	}
}

func TestPreparationCompilationSourcePreservesNilAndEmptyConfiguration(t *testing.T) {
	for _, empty := range []bool{false, true} {
		w := compilationMappingFixture(t)
		if empty {
			w.SoulBindings = map[string]common.SoulID{}
			w.PreparationSealEvidence[0].ExecutionInputs.Configuration.SoulBindings = map[string]common.SoulID{}
			w.PreparationSealEvidence[0].ExecutionInputs.Configuration.EvidencePolicies = []workorcheventpayload.Payload{}
		} else {
			w.PreparationSealEvidence[0].ExecutionInputs.Configuration.EvidencePolicies = nil
		}
		model, err := PreparationCompilationSource(w).Model()
		if err != nil || !reflect.DeepEqual(w.SoulBindings, model.SoulBindings) || !reflect.DeepEqual(w.PreparationSealEvidence, model.PreparationSealEvidence) {
			t.Fatalf("nil/empty boundary changed: empty=%t err=%v", empty, err)
		}
	}
}

func TestPreparationCompilationSourceRejectsBindingCardinalityLoss(t *testing.T) {
	for _, target := range []string{"metadata", "configuration", "template", "authoring", "history"} {
		for _, invalid := range [][]soulbinding.Binding{
			{{Stage: "planning", SoulID: "one"}, {Stage: "planning", SoulID: "two"}},
			{{Stage: "", SoulID: "one"}}, {{Stage: "planning", SoulID: ""}},
		} {
			s := PreparationCompilationSource(compilationMappingFixture(t))
			switch target {
			case "metadata":
				s.Metadata.SoulBindings = invalid
			case "configuration":
				s.Seals[0].Inputs.Configuration.SoulBindings = invalid
			case "template":
				s.Seals[0].Inputs.Template.SoulBindings = invalid
			case "authoring":
				s.Authoring.Record.TemplateSnapshot.SoulBindings = invalid
			case "history":
				s.History[0].Authoring.Record.TemplateSnapshot.SoulBindings = invalid
			}
			if _, err := s.Model(); err == nil {
				t.Fatal("binding collapsed before validation", target)
			}
		}
	}
	s := PreparationCompilationSource(compilationMappingFixture(t))
	s.Authoring.Record.PreparationDiagnostics = DefinitionAuthoringRecordDTO(*compilationMappingFixture(t).DefinitionAuthoring).PreparationDiagnostics
	if _, err := s.Model(); err == nil {
		t.Fatal("mixed public metadata and private raw accepted")
	}
}
