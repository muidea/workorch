package activationcore

import (
	"sort"
	"strconv"
	"strings"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

// OrderedWorkspaceGoals derives definition order exclusively from Workspace
// owner state. EventRecord order is intentionally excluded: Eventstream is an
// audit/display projection and cannot become an SM-4 authority input.
func OrderedWorkspaceGoals(items []*workspacemodel.Goal) []*workspacemodel.Goal {
	ret := append([]*workspacemodel.Goal(nil), items...)
	sort.SliceStable(ret, func(i, j int) bool {
		return definitionOrderLess("goal", goalRevision(ret[i]), goalID(ret[i]), goalRevision(ret[j]), goalID(ret[j]))
	})
	return ret
}

// OrderedWorkspaceDoDs is the DoD equivalent of OrderedWorkspaceGoals.
func OrderedWorkspaceDoDs(items []*workspacemodel.DoD) []*workspacemodel.DoD {
	ret := append([]*workspacemodel.DoD(nil), items...)
	sort.SliceStable(ret, func(i, j int) bool {
		return definitionOrderLess("dod", dodRevision(ret[i]), dodID(ret[i]), dodRevision(ret[j]), dodID(ret[j]))
	})
	return ret
}

func definitionOrderLess(prefix, leftRevision, leftID, rightRevision, rightID string) bool {
	leftSequence, leftComparable := ownerDefinitionRevisionSequence(prefix, leftRevision)
	rightSequence, rightComparable := ownerDefinitionRevisionSequence(prefix, rightRevision)
	if leftComparable != rightComparable {
		// Legacy, non-comparable definitions precede owner-issued monotonic
		// revisions. This preserves a one-way migration without using clocks or
		// audit arrival order.
		return !leftComparable
	}
	if leftComparable && leftSequence != rightSequence {
		return leftSequence < rightSequence
	}
	return leftID < rightID
}

func ownerDefinitionRevisionSequence(prefix, revision string) (uint64, bool) {
	parts := strings.SplitN(strings.TrimSpace(revision), ":", 3)
	if len(parts) != 3 || parts[0] != prefix || parts[1] == "" || parts[2] == "" {
		return 0, false
	}
	sequence, err := strconv.ParseUint(parts[1], 10, 64)
	return sequence, err == nil && sequence > 0
}

func goalRevision(item *workspacemodel.Goal) string {
	if item == nil {
		return ""
	}
	return item.DefinitionRevision
}

func goalID(item *workspacemodel.Goal) string {
	if item == nil {
		return ""
	}
	return string(item.ID)
}

func dodRevision(item *workspacemodel.DoD) string {
	if item == nil {
		return ""
	}
	return item.DefinitionRevision
}

func dodID(item *workspacemodel.DoD) string {
	if item == nil {
		return ""
	}
	return string(item.ID)
}
