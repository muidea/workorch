package workorchruntimecontract

import (
	"fmt"
	"reflect"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeSemanticCompiledOrder records code origin, never provider provenance,
// review approval, proof feasibility or execution/validation credit.
type RuntimeSemanticCompiledOrder struct {
	Rule         string
	ManifestHash string
	DocumentHash string
}

const runtimeSingletonOrderRule = "singleton_ordinary_order"

func CompileRuntimeSingletonCoverageOrder(m RuntimeSemanticPlanManifest) (RuntimeSemanticPlanDocument, bool) {
	if len(m.CoverageItems) != 1 || RuntimeSemanticPurposeChoice(m, RuntimeSemanticPlanStepCoverageOrder) {
		return RuntimeSemanticPlanDocument{}, false
	}
	return RuntimeSemanticPlanDocument{SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent,
		ManifestHash: m.Hash, DefinitionSeedHash: m.DefinitionSeedHash, PlanStep: RuntimeSemanticPlanStepCoverageOrder,
		CoverageOrder: []common.CoverageWorkItemID{m.CoverageItems[0].ID}}, true
}
func NewRuntimeSemanticCompiledOrder(m RuntimeSemanticPlanManifest, d RuntimeSemanticPlanDocument) *RuntimeSemanticCompiledOrder {
	return &RuntimeSemanticCompiledOrder{Rule: runtimeSingletonOrderRule, ManifestHash: m.Hash, DocumentHash: StableRuntimeSemanticPlanDocumentHash(d)}
}
func (o *RuntimeSemanticCompiledOrder) Validate(m *RuntimeSemanticPlanManifest, d *RuntimeSemanticPlanDocument) error {
	if o == nil {
		return nil
	}
	if m == nil || d == nil {
		return fmt.Errorf("compiled semantic order requires original manifest/order")
	}
	expected, ok := CompileRuntimeSingletonCoverageOrder(*m)
	if !ok || o.Rule != runtimeSingletonOrderRule || o.ManifestHash != m.Hash || o.DocumentHash != StableRuntimeSemanticPlanDocumentHash(expected) || !reflect.DeepEqual(expected.Normalized(), d.Normalized()) {
		return fmt.Errorf("compiled semantic order differs from exact singleton rule")
	}
	return nil
}
