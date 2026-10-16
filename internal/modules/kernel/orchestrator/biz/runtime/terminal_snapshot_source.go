package runtime

import runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

func CompileRuntimeTerminalSnapshotSourcesFromAggregate(aggregate RuntimeAggregate) ([]runtimecontract.RuntimeTerminalSnapshotSource, error) {
	if err := aggregate.Validate(); err != nil {
		return nil, err
	}
	return runtimecontract.CompileRuntimeTerminalSnapshotSources(terminalMaterialSource(aggregate))
}
