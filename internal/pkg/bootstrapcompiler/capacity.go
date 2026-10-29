package bootstrapcompiler

import (
	"fmt"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const CapacityCompilerVersion = "workorch-coverage-capacity/v1"

// Capacity counts the same independent work units as bootstrap compilation.
// Grouping never reduces this count or merges evidence obligations.
type Capacity struct {
	Version                              string
	DoDItems, ProfileItems, Total, Limit int
}

func CompileCapacity(goal string, items []string, profiles []runtimecontract.RuntimeProfileContract, limit int) (Capacity, error) {
	c := Capacity{Version: CapacityCompilerVersion, Limit: limit}
	if limit < 1 || limit > runtimecontract.RuntimeAcceptanceFocusMaxCount {
		return c, fmt.Errorf("coverage capacity limit must be in 1..%d", runtimecontract.RuntimeAcceptanceFocusMaxCount)
	}
	units, err := compileRuntimeCoverageWorkUnits(&GoalInput{Text: goal}, &DoDInput{Items: items}, profiles)
	if err != nil {
		return c, err
	}
	for _, u := range units {
		if u.sourceScope == "dodItem" {
			c.DoDItems++
		} else {
			c.ProfileItems++
		}
	}
	c.Total = len(units)
	if c.Total > limit {
		return c, fmt.Errorf("coverage capacity exceeded: %d DoD + %d profile = %d independent focuses; sealed limit %d", c.DoDItems, c.ProfileItems, c.Total, limit)
	}
	return c, nil
}
