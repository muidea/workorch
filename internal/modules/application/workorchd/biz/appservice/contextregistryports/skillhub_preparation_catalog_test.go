package contextregistryports

import (
	"bytes"
	"context"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/pkg/workorchskillcontract"
)

func TestPreparationSkillHubCatalogPreservesRawMetadataAndOpaqueStatus(t *testing.T) {
	for _, global := range []bool{true, false} {
		raw := []byte("---\r\nname: '原始 名称'\r\nversion: 'v1'\r\ntriggers: ['go']\r\nrequired_tools: [shell]\r\nrequired_capability_refs: [cap-a]\r\nreference_refs: [refs/a.md]\r\n---\r\n原始正文\r\n")
		dir, _, runner := preparationInstructionFixture(t, global, "group/nested", raw)
		value, err := ObserveSkillHubPreparationCatalog(context.Background(), runner, "ws", dir, global)
		if err != nil || value.FailureCode != "" || len(value.Skills) != 1 || len(value.Metadata) != 1 || runner.calls != 1 {
			t.Fatalf("metadata observation failed: %+v %v", value, err)
		}
		item := value.Skills[0]
		if item.Name != "原始 名称" || item.Version != "v1" || item.Instruction != string(raw) || !bytes.Equal(value.Observation.Instructions[0].Raw, raw) ||
			!reflect.DeepEqual(item.RequiredCapabilityRefs, []string{"cap-a"}) || !reflect.DeepEqual(item.RequiredToolKinds, []string{"shell"}) || !reflect.DeepEqual(item.ReferenceRefs, []string{"refs/a.md"}) ||
			item.ContentHash != workorchskillcontract.StableContentHash(workorchskillcontract.Content{ID: item.ID, Name: item.Name, Version: item.Version, Instruction: string(raw), TriggerRules: []string{"go"}, RequiredToolKinds: []string{"shell"}, RequiredCapabilityRefs: []string{"cap-a"}, ReferenceRefs: []string{"refs/a.md"}}) {
			t.Fatal("projection discarded or invented material")
		}
		if global && (item.Scope != contextregistrycore.SkillScopeGlobal || item.WorkspaceID != "" || item.Agent != "codex") || !global && (item.Scope != contextregistrycore.SkillScopeWorkspace || item.WorkspaceID != "ws") {
			t.Fatal("source domains mixed")
		}
		value.Metadata[0].TriggerRules[0] = "caller"
		if item.TriggerRules[0] != "go" {
			t.Fatal("metadata and mapped material alias")
		}
	}
}

func TestPreparationSkillHubCatalogFailureRetainsCompleteObservationAndDoesNotRetry(t *testing.T) {
	raw := []byte("---\nrequired_tools: [shell]\nrequired_tools: []\n---\n原始正文\n")
	dir, _, runner := preparationInstructionFixture(t, false, "s", raw)
	value, err := ObserveSkillHubPreparationCatalog(context.Background(), runner, "ws", dir, false)
	if err == nil || value.FailureCode != "invalid_metadata" || value.FailureSkillID != "s" || len(value.Skills) != 0 || !value.Observation.InstructionsReobserved || !bytes.Equal(value.Observation.Instructions[0].Raw, raw) || !bytes.Equal(value.Observation.Inventory.RawOutput, runner.output) || runner.calls != 1 {
		t.Fatal("metadata failure became empty success, lost original bytes or retried status")
	}
	if value, err := ObserveSkillHubPreparationCatalog(nil, runner, "ws", dir, false); err == nil || value.FailureCode != "installed_observation_failed" || runner.calls != 1 {
		t.Fatal("invalid request ran status")
	}
}
