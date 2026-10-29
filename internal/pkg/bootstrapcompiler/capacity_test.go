package bootstrapcompiler

import (
	"fmt"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"testing"
)

func TestCapacityCountsIndependentDoDAndProfileObligations(t *testing.T) {
	items := make([]string, 67)
	for i := range items {
		items[i] = fmt.Sprintf("requirement %d", i)
	}
	profiles := []runtimecontract.RuntimeProfileContract{{ID: "p", DeliverableExpectations: []runtimecontract.RuntimeDeliverableExpectation{{ID: "source_change", Required: true}, {ID: "validation_report", Required: true}, {ID: "version_control_snapshot", Required: true}}}}
	for _, limit := range []int{64, 70, 128} {
		c, err := CompileCapacity("goal", items, profiles, limit)
		if c.DoDItems != 67 || c.ProfileItems != 3 || c.Total != 70 || (err == nil) != (limit >= 70) {
			t.Fatalf("limit %d: %+v %v", limit, c, err)
		}
	}
	for _, limit := range []int{0, 129} {
		if _, err := CompileCapacity("goal", items, profiles, limit); err == nil {
			t.Fatalf("accepted limit %d", limit)
		}
	}
}

func TestCapacityMaximumAndBlankItems(t *testing.T) {
	items := make([]string, 128)
	for i := range items {
		items[i] = fmt.Sprintf("r%d", i)
	}
	if c, err := CompileCapacity("goal", append(items, " "), nil, 128); err != nil || c.Total != 128 {
		t.Fatalf("boundary: %+v %v", c, err)
	}
	if _, err := CompileCapacity("goal", append(items, "extra"), nil, 128); err == nil {
		t.Fatal("accepted 129 focuses")
	}
}
