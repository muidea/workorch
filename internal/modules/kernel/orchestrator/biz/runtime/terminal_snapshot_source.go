package runtime

import runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

// CompileRuntimeTerminalSnapshotSourcesFromAggregate consumes the formal owner
// aggregate. Exact current snapshot sources are checked by the terminal compiler;
// raw aggregate admission is not a terminal-consumption responsibility.
func CompileRuntimeTerminalSnapshotSourcesFromAggregate(aggregate RuntimeAggregate) ([]runtimecontract.RuntimeTerminalSnapshotSource, error) {
	return runtimecontract.CompileRuntimeTerminalSnapshotSources(terminalMaterialSource(aggregate))
}
