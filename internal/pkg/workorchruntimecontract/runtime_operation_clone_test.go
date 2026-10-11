package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeOperationClonePreservesRawAndDetachesParameters(t *testing.T) {
	text, integer, boolean := " 原文\r\n\x00 ", int64(17), true
	operation := RuntimeOperation{
		ID: " operation ", Kind: " file/read ", RequiredOperationRef: " required ",
		DependsOn: []string{" z ", "a", "a"}, BindingRefs: []string{" binding "},
		CoverageWorkItemIDs: []common.CoverageWorkItemID{" cwi "}, TargetRefs: []string{" target "},
		Parameters: []OperationParameter{
			{Name: " text ", String: &text}, {Name: "integer", Integer: &integer},
			{Name: "boolean", Boolean: &boolean}, {Name: "args", StringList: []string{"test", "-run", "^Original$"}},
		},
	}
	body, err := json.Marshal(operation)
	if err != nil {
		t.Fatal(err)
	}
	clone := operation.Clone()
	if !reflect.DeepEqual(operation, clone) {
		t.Fatal("Clone 规范化或改写原 operation")
	}
	clone.DependsOn[0], clone.BindingRefs[0], clone.CoverageWorkItemIDs[0], clone.TargetRefs[0] = "changed", "changed", "changed", "changed"
	*clone.Parameters[0].String = "changed"
	*clone.Parameters[1].Integer = 99
	*clone.Parameters[2].Boolean = false
	clone.Parameters[3].StringList[0] = "changed"
	current, err := json.Marshal(operation)
	if err != nil || string(current) != string(body) {
		t.Fatal("返回副本污染原参数或集合")
	}
	for _, empty := range []RuntimeOperation{{}, {DependsOn: []string{}, BindingRefs: []string{}, CoverageWorkItemIDs: []common.CoverageWorkItemID{}, TargetRefs: []string{}, Parameters: []OperationParameter{}}} {
		if !reflect.DeepEqual(empty, empty.Clone()) {
			t.Fatal("Clone 改变 nil/empty 表示")
		}
	}
	var loaded RuntimeOperation
	if err := json.Unmarshal(body, &loaded); err != nil || !reflect.DeepEqual(loaded, loaded.Clone()) {
		t.Fatal("JSON 重载后的原正文未保全")
	}
}
