package events

import (
	"errors"
	"strings"
	"testing"

	governancemodel "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

func TestGovernanceCommandResultCloneDetachesOwnerState(t *testing.T) {
	item := &governancemodel.AutoGovernanceTrace{EvidenceRefs: []string{"owner"}}
	cloned := (GovernanceCommandResult{Traces: []*governancemodel.AutoGovernanceTrace{item}}).Clone()
	item.EvidenceRefs[0] = "mutated"
	if cloned.Traces[0].EvidenceRefs[0] != "owner" {
		t.Fatal("cloned governance trace shares evidence refs")
	}
}

func TestListAutoGovernanceTracesCommandRequiresCanonicalBoundedWorkspaceScope(t *testing.T) {
	valid := ListAutoGovernanceTracesCommand{Query: workorchquerycontract.AutoGovernanceTraceQuery{WorkspaceID: "workspace-1", Limit: 10}}
	if _, err := valid.Normalized(); err != nil {
		t.Fatalf("valid trace query rejected: %v", err)
	}
	for name, command := range map[string]ListAutoGovernanceTracesCommand{
		"empty workspace": {},
		"noncanonical workspace": {Query: workorchquerycontract.AutoGovernanceTraceQuery{
			WorkspaceID: " workspace-1 ", Limit: 10,
		}},
		"noncanonical item": {Query: workorchquerycontract.AutoGovernanceTraceQuery{
			WorkspaceID: "workspace-1", ItemID: " item-1 ", Limit: 10,
		}},
		"zero limit": {Query: workorchquerycontract.AutoGovernanceTraceQuery{WorkspaceID: "workspace-1"}},
		"negative limit": {Query: workorchquerycontract.AutoGovernanceTraceQuery{
			WorkspaceID: "workspace-1", Limit: -1,
		}},
		"oversized limit": {Query: workorchquerycontract.AutoGovernanceTraceQuery{
			WorkspaceID: "workspace-1", Limit: MaxAutoGovernanceTraceQueryLimit + 1,
		}},
		"oversized identity": {Query: workorchquerycontract.AutoGovernanceTraceQuery{
			WorkspaceID: common.WorkspaceID(strings.Repeat("x", MaxAutoGovernanceTraceQueryStringBytes+1)), Limit: 10,
		}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := command.Normalized(); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("error = %v, want invalid argument", err)
			}
		})
	}
}
