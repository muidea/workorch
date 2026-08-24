package activationcore

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

// OrderedWorkspaceGoals derives definition order exclusively from Workspace
// owner state. EventRecord order is intentionally excluded: Eventstream is an
// audit/display projection and cannot become an SM-4 authority input.
func OrderedWorkspaceGoals(items []*workspacemodel.Goal) ([]*workspacemodel.Goal, error) {
	ret := append([]*workspacemodel.Goal(nil), items...)
	for _, item := range ret {
		if item == nil {
			return nil, fmt.Errorf("%w: Goal data is outside the current contract", common.ErrConflict)
		}
		if _, ok := ownerDefinitionRevisionSequence("goal", item.DefinitionRevision); !ok {
			return nil, fmt.Errorf("%w: Goal definition revision is outside the current contract", common.ErrConflict)
		}
	}
	sort.SliceStable(ret, func(i, j int) bool {
		return definitionOrderLess("goal", goalRevision(ret[i]), goalID(ret[i]), goalRevision(ret[j]), goalID(ret[j]))
	})
	return ret, nil
}

// OrderedWorkspaceDoDs is the DoD equivalent of OrderedWorkspaceGoals.
func OrderedWorkspaceDoDs(items []*workspacemodel.DoD) ([]*workspacemodel.DoD, error) {
	ret := append([]*workspacemodel.DoD(nil), items...)
	for _, item := range ret {
		if item == nil {
			return nil, fmt.Errorf("%w: DoD data is outside the current contract", common.ErrConflict)
		}
		if _, ok := ownerDefinitionRevisionSequence("dod", item.DefinitionRevision); !ok {
			return nil, fmt.Errorf("%w: DoD definition revision is outside the current contract", common.ErrConflict)
		}
	}
	sort.SliceStable(ret, func(i, j int) bool {
		return definitionOrderLess("dod", dodRevision(ret[i]), dodID(ret[i]), dodRevision(ret[j]), dodID(ret[j]))
	})
	return ret, nil
}

func definitionOrderLess(prefix, leftRevision, leftID, rightRevision, rightID string) bool {
	leftSequence, _ := ownerDefinitionRevisionSequence(prefix, leftRevision)
	rightSequence, _ := ownerDefinitionRevisionSequence(prefix, rightRevision)
	if leftSequence != rightSequence {
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
