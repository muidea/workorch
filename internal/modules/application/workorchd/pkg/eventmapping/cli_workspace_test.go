package eventmapping

import (
	"reflect"
	"strings"
	"testing"

	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	"github.com/muidea/workorch/internal/pkg/workorchmemorycontract"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestCLIWorkspaceAndTemplateMappingIsStableDetachedAndRejectsDuplicates(t *testing.T) {
	workspace := projectioncontract.Workspace{
		ID: "ws-map", SoulBindings: map[string]string{"validation": "soul-v", "execution": "soul-e"},
		ViewModel: projectioncontract.WorkspaceViewModel{StreamGeneration: "generation-1", FailedWorkspaceIds: []string{"ws-failed"}, DegradedSince: "now", Sections: map[string]projectioncontract.WorkspaceViewModelSection{"runtime": {Name: "runtime"}, "overview": {Name: "overview"}}},
	}
	dto, err := WorkspaceDTO(workspace)
	if err != nil {
		t.Fatalf("workspace DTO: %v", err)
	}
	if len(dto.SoulBindings) != 2 || dto.SoulBindings[0].Stage != "execution" || len(dto.ViewModel.Sections) != 2 || dto.ViewModel.Sections[0].Name != "overview" {
		t.Fatalf("stable workspace DTO = %+v", dto)
	}
	dto.SoulBindings[0].SoulID = "mutated"
	dto.ViewModel.FailedWorkspaceIds[0] = "mutated"
	if workspace.SoulBindings["execution"] != "soul-e" {
		t.Fatal("workspace DTO mutation leaked into port DTO")
	}
	if workspace.ViewModel.FailedWorkspaceIds[0] != "ws-failed" {
		t.Fatal("workspace view model failure list shares EventHub DTO storage")
	}
	duplicate, err := WorkspaceDTO(workspace)
	if err != nil {
		t.Fatalf("workspace DTO: %v", err)
	}
	duplicate.SoulBindings = append(duplicate.SoulBindings, duplicate.SoulBindings[0])
	if _, err := WorkspaceProjection(duplicate); err == nil {
		t.Fatal("duplicate workspace soul binding was accepted")
	}
	duplicate, err = WorkspaceDTO(workspace)
	if err != nil {
		t.Fatalf("workspace DTO: %v", err)
	}
	duplicate.ViewModel.Sections = append(duplicate.ViewModel.Sections, duplicate.ViewModel.Sections[0])
	if _, err := WorkspaceProjection(duplicate); err == nil {
		t.Fatal("duplicate workspace view section was accepted")
	}
}

func TestCLIWorkspaceMappingRoundTripClonesDefaultLLMSelection(t *testing.T) {
	target := runtimecontract.ProviderModelTarget{ProviderID: "provider-a", ModelID: "model-a"}
	workspace := projectioncontract.Workspace{
		ID:                  "ws-map",
		DefaultLLMSelection: &runtimecontract.ProviderInferenceSelection{Target: target},
		SoulBindings:        map[string]string{"execution": "soul-e"},
		ContextHints:        map[string]any{"scope": "workspace", "nested": map[string]any{"items": []any{"one"}}},
		EvidencePolicies:    []map[string]any{{"kind": "test"}},
	}
	dto, err := WorkspaceDTO(workspace)
	if err != nil {
		t.Fatalf("workspace DTO: %v", err)
	}
	if dto.DefaultLLMSelection == workspace.DefaultLLMSelection {
		t.Fatal("workspace DTO retained DefaultLLMSelection pointer")
	}
	dto.DefaultLLMSelection.Target.ModelID = "mutated"
	if workspace.DefaultLLMSelection.Target.ModelID != "model-a" {
		t.Fatal("workspace DTO DefaultLLMSelection mutation leaked into port DTO")
	}
	dto, err = WorkspaceDTO(workspace)
	if err != nil {
		t.Fatalf("workspace DTO: %v", err)
	}
	roundTrip, err := WorkspaceProjection(dto)
	if err != nil {
		t.Fatalf("workspace round trip: %v", err)
	}
	if !reflect.DeepEqual(roundTrip, workspace) {
		t.Fatalf("workspace round trip = %#v, want %#v", roundTrip, workspace)
	}
	if roundTrip.DefaultLLMSelection == dto.DefaultLLMSelection {
		t.Fatal("workspace port retained Event DTO DefaultLLMSelection pointer")
	}
}

func TestCLITemplateMappingRoundTripIsStableAndDetached(t *testing.T) {
	template := workorchmanagementcontract.WorkspaceTemplate{
		ID: "template-a", Name: "Template A", GoalText: "goal", DoDItems: []string{"one", "two"},
		SoulBindings:     map[string]string{"validation": "soul-v", "execution": "soul-e"},
		ContextHints:     map[string]any{"language": "go"},
		EvidencePolicies: []map[string]any{{"kind": "test"}},
	}
	dto, err := TemplateDTO(template)
	if err != nil {
		t.Fatalf("template DTO: %v", err)
	}
	if len(dto.SoulBindings) != 2 || dto.SoulBindings[0].Stage != "execution" {
		t.Fatalf("stable template DTO = %+v", dto)
	}
	dto.DoDItems[0] = "mutated"
	if template.DoDItems[0] != "one" {
		t.Fatal("template DTO DoD mutation leaked into port DTO")
	}
	dto, err = TemplateDTO(template)
	if err != nil {
		t.Fatalf("template DTO: %v", err)
	}
	roundTrip, err := TemplateManagement(dto)
	if err != nil {
		t.Fatalf("template round trip: %v", err)
	}
	if !reflect.DeepEqual(roundTrip, template) {
		t.Fatalf("template round trip = %#v, want %#v", roundTrip, template)
	}
	duplicate, err := TemplateDTO(template)
	if err != nil {
		t.Fatalf("template DTO: %v", err)
	}
	duplicate.SoulBindings = append(duplicate.SoulBindings, duplicate.SoulBindings[0])
	if _, err := TemplateManagement(duplicate); err == nil {
		t.Fatal("duplicate template soul binding was accepted")
	}
}

func TestCLIWorkspaceSnapshotMappingRoundTripAndNilSemantics(t *testing.T) {
	snapshot := projectioncontract.WorkspaceSnapshot{
		Workspace:            projectioncontract.Workspace{ID: "ws-snapshot", SoulBindings: map[string]string{"execution": "soul-e"}},
		ViewModel:            projectioncontract.WorkspaceViewModel{WorkspaceID: "ws-snapshot", Sections: map[string]projectioncontract.WorkspaceViewModelSection{"runtime": {Name: "runtime"}}},
		Run:                  &projectioncontract.Run{ID: "run-current"},
		Step:                 &projectioncontract.StepView{ID: "step-current", Input: map[string]any{"input": "kept"}},
		Result:               &projectioncontract.Result{ID: "result-current"},
		AuthorityDiagnostics: []projectioncontract.GapDiagnostic{{Code: "authority", Message: "original"}},
		ExecutionSections:    []projectioncontract.ExecutionSection{{ID: "section-1", ActualEvidenceRefs: []string{"evidence-1"}}},
		Goals:                nil,
		DoDs:                 []workorchmanagementcontract.DoD{},
		MemoryRecallTraces: []projectioncontract.MemoryRecallTrace{{
			ID: "memory-trace-1", TypeQuota: map[string]int{"semantic": 2, "episodic": 1},
			CandidateMemoryIDs: []string{"memory-1"},
			ModelID:            "embedding-model", ContextBudget: workorchmemorycontract.RecallContextBudget{MaxItems: 8, MaxBytes: 16384, MaxEstimatedTokens: 4096},
			SemanticRecall: &workorchmemorycontract.RecallSelectionEvidence{Policy: "semantic", Status: "degraded"}, FallbackFrom: "semantic_vector_search",
		}},
	}
	dto, err := SnapshotDTO(snapshot)
	if err != nil {
		t.Fatalf("snapshot DTO: %v", err)
	}
	dto.Step.Input = workorcheventpayload.New(workorcheventpayload.StringField("input", "mutated"))
	dto.MemoryRecallTraces[0].TypeQuota[0].Quota = 99
	dto.MemoryRecallTraces[0].SemanticRecall.Policy = "mutated"
	if value, _ := snapshot.Step.Input["input"].(string); value != "kept" {
		t.Fatal("snapshot step payload shares EventHub DTO storage")
	}
	if snapshot.MemoryRecallTraces[0].TypeQuota["episodic"] != 1 {
		t.Fatal("memory recall quota shares EventHub DTO storage")
	}
	if snapshot.MemoryRecallTraces[0].SemanticRecall.Policy != "semantic" {
		t.Fatal("memory recall nested evidence shares EventHub DTO storage")
	}
	dto, err = SnapshotDTO(snapshot)
	if err != nil {
		t.Fatalf("snapshot DTO: %v", err)
	}
	roundTrip, err := SnapshotPort(dto)
	if err != nil {
		t.Fatalf("snapshot round trip: %v", err)
	}
	if !reflect.DeepEqual(roundTrip, snapshot) {
		t.Fatalf("snapshot round trip = %#v, want %#v", roundTrip, snapshot)
	}
	nilTemplates, err := TemplateDTOs(nil)
	if err != nil {
		t.Fatalf("nil template DTOs: %v", err)
	}
	nilWorkspaces, err := WorkspaceDTOs(nil)
	if err != nil {
		t.Fatalf("nil workspace DTOs: %v", err)
	}
	if nilWorkspaces != nil || nilTemplates != nil {
		t.Fatal("nil collection mapping must preserve nil")
	}
	if got, err := WorkspaceDTOs([]projectioncontract.Workspace{}); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("empty workspace collection = %#v", got)
	}
	if got, err := TemplateDTOs([]workorchmanagementcontract.WorkspaceTemplate{}); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("empty template collection = %#v", got)
	}
}

func TestWorkorchdCLIMapFreeMessageGraph(t *testing.T) {
	for _, root := range []reflect.Type{
		reflect.TypeOf(workorchdevents.CLIWorkspaceCreateCommand{}), reflect.TypeOf(workorchdevents.CLIWorkspaceCreateResult{}),
		reflect.TypeOf(workorchdevents.CLIWorkspaceGetResult{}), reflect.TypeOf(workorchdevents.CLIWorkspaceListResult{}),
		reflect.TypeOf(workorchdevents.CLIWorkspaceSnapshotResult{}), reflect.TypeOf(workorchdevents.ReadWorkspaceRuntimeViewResult{}),
		reflect.TypeOf(workorchdevents.CLIWorkspaceUpdateLLMSelectionResult{}),
		reflect.TypeOf(workorchdevents.CLITemplateCreateCommand{}), reflect.TypeOf(workorchdevents.CLITemplateCreateResult{}),
		reflect.TypeOf(workorchdevents.CLITemplateGetResult{}), reflect.TypeOf(workorchdevents.CLITemplateListResult{}),
		reflect.TypeOf(workorchdevents.CLITemplateUpdateCommand{}), reflect.TypeOf(workorchdevents.CLITemplateUpdateResult{}),
		reflect.TypeOf(workorchdevents.CLIMCPServerCreateCommand{}), reflect.TypeOf(workorchdevents.CLIMCPServerCreateResult{}),
		reflect.TypeOf(workorchdevents.CLIMCPServerUpdateCommand{}), reflect.TypeOf(workorchdevents.CLIMCPServerListResult{}),
		reflect.TypeOf(workorchdevents.CLIMCPCapabilityListResult{}),
		reflect.TypeOf(workorchdevents.CLIArtifactInfo{}), reflect.TypeOf(workorchdevents.CLIArtifactContinuationInfo{}),
		reflect.TypeOf(workorchdevents.CLIPatchInfo{}),
		reflect.TypeOf(workorchdevents.CLITaskDecompositionInfo{}), reflect.TypeOf(workorchdevents.CLITaskPlanInfo{}),
		reflect.TypeOf(workorchdevents.CLITaskProgressInfo{}), reflect.TypeOf(workorchdevents.CLIRepairLoopInfo{}),
		reflect.TypeOf(workorchdevents.CLIStepViewInfo{}), reflect.TypeOf(workorchdevents.CLIGapBaselineInfo{}),
		reflect.TypeOf(workorchdevents.CLIRunStatusResult{}),
	} {
		assertMapFreeMessageType(t, root, root.String(), map[reflect.Type]bool{})
	}
}

func assertMapFreeMessageType(t *testing.T, value reflect.Type, path string, seen map[reflect.Type]bool) {
	t.Helper()
	for value.Kind() == reflect.Pointer || value.Kind() == reflect.Slice || value.Kind() == reflect.Array {
		value = value.Elem()
	}
	if value.PkgPath() == "github.com/muidea/workorch/internal/pkg/workorcheventpayload" && value.Name() == "Payload" {
		return
	}
	if seen[value] {
		return
	}
	seen[value] = true
	switch value.Kind() {
	case reflect.Map, reflect.Interface, reflect.Chan, reflect.Func, reflect.UnsafePointer:
		t.Fatalf("forbidden EventHub message field path=%s type=%s", path, value)
	case reflect.Struct:
		for idx := 0; idx < value.NumField(); idx++ {
			field := value.Field(idx)
			assertMapFreeMessageType(t, field.Type, path+"."+field.Name, seen)
		}
	default:
		if strings.Contains(value.String(), "io.Writer") {
			t.Fatalf("forbidden EventHub resource path=%s type=%s", path, value)
		}
	}
}
