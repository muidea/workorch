package eventmapping

import (
	"reflect"
	"strings"
	"testing"

	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	workorchport "github.com/muidea/workorch/internal/pkg/workorchport"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestCLIWorkspaceAndTemplateMappingIsStableDetachedAndRejectsDuplicates(t *testing.T) {
	workspace := workorchport.WorkspaceInfo{
		ID: "ws-map", SoulBindings: map[string]common.SoulID{"validation": "soul-v", "execution": "soul-e"},
		ViewModel: workorchport.WorkspaceViewModelInfo{StreamGeneration: "generation-1", FailedWorkspaceIds: []string{"ws-failed"}, DegradedSince: "now", Sections: map[string]projectioncontract.WorkspaceViewModelSection{"runtime": {Name: "runtime"}, "overview": {Name: "overview"}}},
	}
	dto := WorkspaceDTO(workspace)
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
	duplicate := WorkspaceDTO(workspace)
	duplicate.SoulBindings = append(duplicate.SoulBindings, duplicate.SoulBindings[0])
	if _, err := WorkspacePort(duplicate); err == nil {
		t.Fatal("duplicate workspace soul binding was accepted")
	}
	duplicate = WorkspaceDTO(workspace)
	duplicate.ViewModel.Sections = append(duplicate.ViewModel.Sections, duplicate.ViewModel.Sections[0])
	if _, err := WorkspacePort(duplicate); err == nil {
		t.Fatal("duplicate workspace view section was accepted")
	}
}

func TestCLIWorkspaceMappingRoundTripClonesDefaultLLMTarget(t *testing.T) {
	target := runtimecontract.ProviderModelTarget{ProviderID: "provider-a", ModelID: "model-a"}
	workspace := workorchport.WorkspaceInfo{
		ID:               "ws-map",
		DefaultLLMTarget: &target,
		SoulBindings:     map[string]common.SoulID{"execution": "soul-e"},
		ContextHints:     workorcheventpayload.New(workorcheventpayload.StringField("scope", "workspace")),
	}
	dto := WorkspaceDTO(workspace)
	if dto.DefaultLLMTarget == workspace.DefaultLLMTarget {
		t.Fatal("workspace DTO retained DefaultLLMTarget pointer")
	}
	dto.DefaultLLMTarget.ModelID = "mutated"
	if workspace.DefaultLLMTarget.ModelID != "model-a" {
		t.Fatal("workspace DTO DefaultLLMTarget mutation leaked into port DTO")
	}
	dto = WorkspaceDTO(workspace)
	roundTrip, err := WorkspacePort(dto)
	if err != nil {
		t.Fatalf("workspace round trip: %v", err)
	}
	if !reflect.DeepEqual(roundTrip, workspace) {
		t.Fatalf("workspace round trip = %#v, want %#v", roundTrip, workspace)
	}
	if roundTrip.DefaultLLMTarget == dto.DefaultLLMTarget {
		t.Fatal("workspace port retained Event DTO DefaultLLMTarget pointer")
	}
}

func TestCLITemplateMappingRoundTripIsStableAndDetached(t *testing.T) {
	template := workorchport.TemplateInfo{
		ID: "template-a", Name: "Template A", GoalText: "goal", DoDItems: []string{"one", "two"},
		SoulBindings:     map[string]string{"validation": "soul-v", "execution": "soul-e"},
		ContextHints:     workorcheventpayload.New(workorcheventpayload.StringField("language", "go")),
		EvidencePolicies: []workorcheventpayload.Payload{workorcheventpayload.New(workorcheventpayload.StringField("kind", "test"))},
	}
	dto := TemplateDTO(template)
	if len(dto.SoulBindings) != 2 || dto.SoulBindings[0].Stage != "execution" {
		t.Fatalf("stable template DTO = %+v", dto)
	}
	dto.DoDItems[0] = "mutated"
	if template.DoDItems[0] != "one" {
		t.Fatal("template DTO DoD mutation leaked into port DTO")
	}
	dto = TemplateDTO(template)
	roundTrip, err := TemplatePort(dto)
	if err != nil {
		t.Fatalf("template round trip: %v", err)
	}
	if !reflect.DeepEqual(roundTrip, template) {
		t.Fatalf("template round trip = %#v, want %#v", roundTrip, template)
	}
	duplicate := TemplateDTO(template)
	duplicate.SoulBindings = append(duplicate.SoulBindings, duplicate.SoulBindings[0])
	if _, err := TemplatePort(duplicate); err == nil {
		t.Fatal("duplicate template soul binding was accepted")
	}
}

func TestCLIWorkspaceSnapshotMappingRoundTripAndNilSemantics(t *testing.T) {
	snapshot := workorchport.WorkspaceSnapshotInfo{
		Workspace:            workorchport.WorkspaceInfo{ID: "ws-snapshot", SoulBindings: map[string]common.SoulID{"execution": "soul-e"}},
		ViewModel:            workorchport.WorkspaceViewModelInfo{WorkspaceID: "ws-snapshot", Sections: map[string]projectioncontract.WorkspaceViewModelSection{"runtime": {Name: "runtime"}}},
		Run:                  &workorchport.RunInfo{ID: "run-current"},
		Step:                 &workorchport.StepViewInfo{ID: "step-current", Input: workorcheventpayload.New(workorcheventpayload.StringField("input", "kept"))},
		Result:               &workorchport.ResultInfo{ID: "result-current"},
		AuthorityDiagnostics: []workorchport.GapDiagnosticInfo{{Code: "authority", Message: "original"}},
		ExecutionSections:    []projectioncontract.ExecutionSection{{ID: "section-1", ActualEvidenceRefs: []string{"evidence-1"}}},
		Goals:                nil,
		DoDs:                 []workorchport.DoDInfo{},
		MemoryRecallTraces: []workorchport.MemoryRecallTraceInfo{{
			ID: "memory-trace-1", TypeQuota: map[string]int{"semantic": 2, "episodic": 1},
			CandidateMemoryIDs: []common.MemoryID{"memory-1"},
		}},
	}
	dto := SnapshotDTO(snapshot)
	dto.Step.Input = workorcheventpayload.New(workorcheventpayload.StringField("input", "mutated"))
	if value, _ := snapshot.Step.Input.String("input"); value != "kept" {
		t.Fatal("snapshot step payload shares EventHub DTO storage")
	}
	dto = SnapshotDTO(snapshot)
	roundTrip, err := SnapshotPort(dto)
	if err != nil {
		t.Fatalf("snapshot round trip: %v", err)
	}
	if !reflect.DeepEqual(roundTrip, snapshot) {
		t.Fatalf("snapshot round trip = %#v, want %#v", roundTrip, snapshot)
	}
	if WorkspaceDTOs(nil) != nil || TemplateDTOs(nil) != nil {
		t.Fatal("nil collection mapping must preserve nil")
	}
	if got := WorkspaceDTOs([]workorchport.WorkspaceInfo{}); got == nil || len(got) != 0 {
		t.Fatalf("empty workspace collection = %#v", got)
	}
	if got := TemplateDTOs([]workorchport.TemplateInfo{}); got == nil || len(got) != 0 {
		t.Fatalf("empty template collection = %#v", got)
	}
}

func TestWorkorchdCLIMapFreeMessageGraph(t *testing.T) {
	for _, root := range []reflect.Type{
		reflect.TypeOf(workorchdevents.CLIWorkspaceCreateCommand{}), reflect.TypeOf(workorchdevents.CLIWorkspaceCreateResult{}),
		reflect.TypeOf(workorchdevents.CLIWorkspaceGetResult{}), reflect.TypeOf(workorchdevents.CLIWorkspaceListResult{}),
		reflect.TypeOf(workorchdevents.CLIWorkspaceSnapshotResult{}), reflect.TypeOf(workorchdevents.CLIWorkspaceUpdateLLMTargetResult{}),
		reflect.TypeOf(workorchdevents.CLITemplateCreateCommand{}), reflect.TypeOf(workorchdevents.CLITemplateCreateResult{}),
		reflect.TypeOf(workorchdevents.CLITemplateGetResult{}), reflect.TypeOf(workorchdevents.CLITemplateListResult{}),
		reflect.TypeOf(workorchdevents.CLITemplateUpdateCommand{}), reflect.TypeOf(workorchdevents.CLITemplateUpdateResult{}),
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
