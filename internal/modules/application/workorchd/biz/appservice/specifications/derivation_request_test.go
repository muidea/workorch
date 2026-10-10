package specifications

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestDerivationOutputSchemaAndStrictParser(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal([]byte(derivationOutputSchema), &schema); err != nil {
		t.Fatal(err)
	}
	checkSchemaDTO(t, schema, reflect.TypeOf(workspace.DefinitionDerivedAcceptance{}))
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, GoalText: "交付目标", TemplateID: "template"}
	template := workspace.WorkspaceTemplate{ID: "template", DoDItems: []string{"模板验收"}}
	valid := workspace.DefinitionDerivedAcceptance{AuthoringInputHash: input.InputHash(), TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template), AdditionalItems: []string{}}
	payload, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	var value any
	if err := json.Unmarshal(payload, &value); err != nil {
		t.Fatal(err)
	}
	if err := validateOutputShape(schema, value); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseDerivedAcceptance(payload, input, &template); err != nil {
		t.Fatal(err)
	}
	for name, invalid := range map[string]string{
		"null":       strings.Replace(string(payload), `"AdditionalItems":[]`, `"AdditionalItems":null`, 1),
		"missing":    strings.Replace(string(payload), `,"AdditionalItems":[]`, "", 1),
		"unknown":    strings.TrimSuffix(string(payload), "}") + `,"ConditionMode":"all_of"}`,
		"wrong-type": strings.Replace(string(payload), `"AdditionalItems":[]`, `"AdditionalItems":"验收"`, 1),
		"duplicate":  strings.TrimSuffix(string(payload), "}") + `,"AdditionalItems":[]}`,
		"wrong-hash": strings.Replace(string(payload), input.InputHash(), "wrong", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseDerivedAcceptance([]byte(invalid), input, &template); err == nil {
				t.Fatal("非法派生文档未被拒绝")
			}
		})
	}
}

func TestDerivationCommandBudgetIncludesStableSchemaAtExactBoundary(t *testing.T) {
	target := runtimecontract.ProviderModelTarget{ProviderID: "provider", ModelID: "model"}
	pendingFor := func(escapedBytes int) workspace.DefinitionDerivationRequest {
		input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, GoalText: "目标", TemplateID: "template", ResourceRefs: []string{"a", "b", "c", "d"}}
		template := workspace.WorkspaceTemplate{ID: "template", DoDItems: []string{"验收"}}
		resources := make([]workspace.DefinitionAuthoringResourceSnapshot, 4)
		for index := range resources {
			content := strings.Repeat("\"", workspace.DefinitionAuthoringResourceMaxBytes)
			if index == 3 {
				content = strings.Repeat("\"", escapedBytes/2) + strings.Repeat("x", escapedBytes%2)
			}
			hash := sha256.Sum256([]byte(content))
			resources[index] = workspace.DefinitionAuthoringResourceSnapshot{Ref: input.ResourceRefs[index], Content: content, ContentHash: "sha256:" + hex.EncodeToString(hash[:])}
		}
		record := workspace.DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template)}
		_, pending, _, err := record.ReserveDerivation("derive", "provider", resources)
		if err != nil {
			t.Fatal(err)
		}
		return pending
	}
	low, high := 0, 2*workspace.DefinitionAuthoringResourceMaxBytes
	for low < high {
		middle := (low + high + 1) / 2
		if _, err := DerivationCommand(pendingFor(middle), target); err == nil {
			low = middle
		} else {
			high = middle - 1
		}
	}
	command, err := DerivationCommand(pendingFor(low), target)
	if err != nil {
		t.Fatal(err)
	}
	sections := command.Request.PromptContext.Sections
	if len(sections[0].Content)+len(sections[1].Content) != maxDocumentBytes {
		t.Fatal("完整合同与 payload 未覆盖精确预算边界")
	}
	if _, err := DerivationCommand(pendingFor(low+1), target); err == nil {
		t.Fatal("超出完整合同预算一字节的输入仍被接受")
	}
}

func TestDerivationCommandKeepsFullStableContract(t *testing.T) {
	target := runtimecontract.ProviderModelTarget{ProviderID: "provider", ModelID: "model"}
	var stable string
	for _, goal := range []string{"交付目标甲", "交付目标乙"} {
		input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, GoalText: goal, TemplateID: "template"}
		template := workspace.WorkspaceTemplate{ID: "template", DoDItems: []string{"模板验收"}}
		record := workspace.DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template)}
		_, pending, _, err := record.ReserveDerivation("derive", "provider", nil)
		if err != nil {
			t.Fatal(err)
		}
		command, err := DerivationCommand(pending, target)
		if err != nil {
			t.Fatal(err)
		}
		if command.Request.Schema != derivationOutputSchema || command.Request.SchemaFormat != llmmodels.ProviderSchemaFormatJSONSchema {
			t.Fatal("派生请求未携带显式完整 schema")
		}
		sections := command.Request.PromptContext.Sections
		if len(sections) != 2 || sections[0].Content != derivationOutputContract() || !strings.Contains(sections[1].Content, input.InputHash()) || strings.Contains(sections[0].Content, goal) {
			t.Fatal("派生请求稳定合同与动态输入分区漂移")
		}
		if stable != "" && stable != sections[0].Content {
			t.Fatal("Goal 变化导致稳定合同漂移")
		}
		stable = sections[0].Content
	}
}
