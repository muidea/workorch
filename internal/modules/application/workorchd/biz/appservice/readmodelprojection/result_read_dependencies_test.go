package readmodelprojection

import (
	"reflect"
	"testing"

	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestResultProjectionDetachesOriginalEvidenceReadDependencies(t *testing.T) {
	dependencies := (runtimecontract.RuntimeOwnerRevisionSet{DefinitionRevision: "definition", Entries: []runtimecontract.RuntimeOwnerRevisionRef{{
		OwnerKind: "capability", OwnerInstanceID: "owner/workspace", ResourceKind: "target", ResourceID: "target",
		Revision: "original-read", DependencyHash: "selector",
	}}}).WithHash()
	item := &resultmodel.Result{RuntimeCloseout: &runtimecontract.RuntimeResultCloseout{
		RequirementOutcomes: []runtimecontract.RuntimeRequirementOutcome{{EvidenceItems: []runtimecontract.RuntimeRequirementEvidenceItem{{
			EvidenceRef: "original", State: runtimecontract.RuntimeRequirementEvidenceStaleOrSuperseded, OwnerRevisionSet: &dependencies,
		}}}},
	}}
	for _, projected := range []*runtimecontract.RuntimeResultCloseout{mapResult(item).RuntimeCloseout, projectResult(item).RuntimeCloseout} {
		if !reflect.DeepEqual(projected, item.RuntimeCloseout) {
			t.Fatal("projection changed original read snapshot or stale evidence state")
		}
		projected.RequirementOutcomes[0].EvidenceItems[0].OwnerRevisionSet.Entries[0].Revision = "caller-change"
		if dependencies.Entries[0].Revision != "original-read" {
			t.Fatal("projection exposed mutable owner dependency")
		}
	}
}
