package workspacecore

import (
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func TestNormalizeTemplatePreservesExactOrderedDoDSourceInventory(t *testing.T) {
	input := workorchmanagementcontract.WorkspaceTemplate{ID: "project", Name: "project", DoDItems: []string{
		"build/test 通过；无法执行时保留原诊断", "# 原文标题\n- 原文条目", "保留重复", "保留重复",
	}, ContextHints: map[string]any{"templateProofBindings": map[string]any{"protocolRef": "workorch.template-source-proof/v1"}}}
	want := append([]string(nil), input.DoDItems...)
	got, err := NormalizeWorkspaceTemplate(input)
	if err != nil || !reflect.DeepEqual(got.DoDItems, want) {
		t.Fatalf("template source text/order changed: %q err=%v", got.DoDItems, err)
	}
	got.DoDItems[0] = "changed"
	if !reflect.DeepEqual(input.DoDItems, want) {
		t.Fatal("normalization aliased caller source inventory")
	}
}
