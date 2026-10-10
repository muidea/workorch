package workspace

import (
	"reflect"
	"testing"
)

func TestRequiredReviewRequirementIDsAreDetachedAndDeterministic(t *testing.T) {
	spec := GoalSpecification{Requirements: []GoalSpecificationRequirement{{ID: "b", Required: true}, {ID: "optional"}, {ID: "a", Required: true}}}
	for _, kind := range goalReviewKinds {
		ids, err := spec.RequiredReviewRequirementIDs(kind)
		if err != nil || !reflect.DeepEqual(ids, []string{"a", "b"}) {
			t.Fatalf("应审集合不完整或不确定: %s %v %v", kind, ids, err)
		}
		ids[0] = "changed"
		again, err := spec.RequiredReviewRequirementIDs(kind)
		if err != nil || !reflect.DeepEqual(again, []string{"a", "b"}) || spec.Requirements[2].ID != "a" {
			t.Fatal("应审集合共享了可变来源")
		}
	}
	if _, err := spec.RequiredReviewRequirementIDs("invented"); err == nil {
		t.Fatal("未知审查维度被接受")
	}
}
