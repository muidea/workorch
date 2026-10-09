package runtime

import runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

func RuntimeDeliveryLineageBindings(aggregate RuntimeAggregate) ([]runtimecontract.RuntimeDeliveryLineageBinding, error) {
	return runtimecontract.RuntimeDeliveryLineageBindingsFromState(terminalMaterialSource(aggregate))
}

func EvaluateRuntimeDeliveryLineageAssertion(aggregate RuntimeAggregate, binding runtimecontract.RuntimeDeliveryLineageBinding) (runtimecontract.RuntimeDeliveryLineageState, error) {
	source := terminalMaterialSource(aggregate)
	source.TerminalMaterialSeal = aggregate.TerminalMaterialSeal
	return runtimecontract.EvaluateRuntimeDeliveryLineageAssertionFromState(source, binding)
}
