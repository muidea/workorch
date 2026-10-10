package eventmapping

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func TestWorkspaceResultMappingIsStableDetachedAndRoundTrips(t *testing.T) {
	hints, err := workorcheventpayload.FromMap(map[string]any{"mode": "strict"})
	if err != nil {
		t.Fatal(err)
	}
	model := &workspacemodel.Workspace{
		ID: "ws-mapping", SoulBindings: map[string]common.SoulID{"validation": "soul-validation", "execution": "soul-execution"},
		ContextHints: hints, EvidencePolicies: []workorcheventpayload.Payload{hints},
	}
	dto := WorkspaceDTO(model)
	if dto == nil || len(dto.SoulBindings) != 2 || dto.SoulBindings[0].Stage != "execution" || dto.SoulBindings[1].Stage != "validation" {
		t.Fatalf("stable bindings = %+v", dto)
	}
	dto.SoulBindings[0].SoulID = "mutated"
	dto.EvidencePolicies[0] = workorcheventpayload.Payload{}
	if model.SoulBindings["execution"] != "soul-execution" || model.ContextHints.Map()["mode"] != "strict" {
		t.Fatalf("DTO mutation leaked to model: %+v %+v", model.SoulBindings, model.ContextHints)
	}
	dto = WorkspaceDTO(model)
	roundTrip := Workspace(dto)
	if !reflect.DeepEqual(roundTrip.SoulBindings, model.SoulBindings) || !reflect.DeepEqual(roundTrip.ContextHints.Map(), model.ContextHints.Map()) {
		t.Fatalf("round trip = %+v, want %+v", roundTrip, model)
	}
}

func TestWorkspaceDTODropsTransientAuthoringSummary(t *testing.T) {
	item := &workspacemodel.Workspace{ID: "ws-transient-summary", AuthoringSummary: &workspacemodel.AuthoringSummary{State: "failed"}}
	dto := WorkspaceDTO(item)
	if dto == nil || dto.AuthoringSummary != nil {
		t.Fatalf("owner DTO carried transient model observation: %+v", dto)
	}
	wire, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(wire)), "authoringsummary") {
		t.Fatalf("Workspace model JSON included transient summary: %s", wire)
	}
	clone := item.AuthoringSummary.Clone()
	if clone == item.AuthoringSummary || clone.State != item.AuthoringSummary.State {
		t.Fatal("summary clone did not detach value")
	}
	clone.State = "changed"
	if item.AuthoringSummary.State != "failed" {
		t.Fatal("summary clone mutation changed original")
	}
}

func TestWorkspaceTemplateResultMappingDeepClonesEveryMutableField(t *testing.T) {
	hints := workorcheventpayload.New(workorcheventpayload.ObjectField("nested", workorcheventpayload.New(workorcheventpayload.StringField("value", "hint-original"))))
	policy := workorcheventpayload.New(workorcheventpayload.StringArrayField("checks", []string{"check-original"}))
	model := &workspacemodel.WorkspaceTemplate{
		ID: "template-1", DoDItems: []string{"dod-original"}, SoulBindings: map[string]string{"planning": "soul-original"},
		ContextHints: hints, EvidencePolicies: []workorcheventpayload.Payload{policy},
	}
	dto := TemplateDTO(model)
	if dto == nil || len(dto.DoDItems) != 1 || len(dto.SoulBindings) != 1 || len(dto.EvidencePolicies) != 1 {
		t.Fatalf("template DTO=%+v", dto)
	}
	dto.DoDItems[0] = "dod-mutated"
	dto.SoulBindings[0].SoulID = "soul-mutated"
	dto.ContextHints.Fields[0].Value.Object[0].Value.String = "hint-mutated"
	dto.EvidencePolicies[0].Fields[0].Value.Array[0].String = "check-mutated"
	if model.DoDItems[0] != "dod-original" || model.SoulBindings["planning"] != "soul-original" ||
		model.ContextHints.Fields[0].Value.Object[0].Value.String != "hint-original" || model.EvidencePolicies[0].Fields[0].Value.Array[0].String != "check-original" {
		t.Fatalf("DTO mutation leaked to template model: %+v", model)
	}

	dto = TemplateDTO(model)
	roundTrip := Template(dto)
	dto.DoDItems[0] = "dto-after-roundtrip"
	dto.SoulBindings[0].SoulID = "dto-soul-after-roundtrip"
	dto.ContextHints.Fields[0].Value.Object[0].Value.String = "dto-hint-after-roundtrip"
	dto.EvidencePolicies[0].Fields[0].Value.Array[0].String = "dto-policy-after-roundtrip"
	if roundTrip.DoDItems[0] != "dod-original" || roundTrip.SoulBindings["planning"] != "soul-original" ||
		roundTrip.ContextHints.Fields[0].Value.Object[0].Value.String != "hint-original" || roundTrip.EvidencePolicies[0].Fields[0].Value.Array[0].String != "check-original" {
		t.Fatalf("template round-trip retained DTO aliases: %+v", roundTrip)
	}

	dtos := TemplateDTOs([]*workspacemodel.WorkspaceTemplate{model})
	models := Templates(dtos)
	dtos[0].DoDItems[0] = "list-mutated"
	if models[0].DoDItems[0] != "dod-original" || model.DoDItems[0] != "dod-original" {
		t.Fatalf("template list mapping retained aliases: source=%+v mapped=%+v", model, models[0])
	}
}
