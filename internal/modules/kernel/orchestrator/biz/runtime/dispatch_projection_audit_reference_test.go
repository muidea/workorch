package runtime

import runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

// Detached projections retained for explicit offline reference tests. Production
// consumes the one canonical record and detaches only at ownership boundaries.
// Detached dispatch projections share the current representation. Pure internal
// consumers can read the record directly without a conversion.
func sealedDispatchesToRuntimeRecords(values []runtimecontract.RuntimeDispatchRecord) []runtimecontract.RuntimeDispatchRecord {
	if values == nil {
		return nil
	}
	result := make([]runtimecontract.RuntimeDispatchRecord, len(values))
	for i := range values {
		result[i] = runtimecontract.CloneRuntimeDispatchRecordRaw(values[i])
	}
	return result
}

func sealedDispatchToRuntimeRecord(value runtimecontract.RuntimeDispatchRecord) runtimecontract.RuntimeDispatchRecord {
	return runtimecontract.CloneRuntimeDispatchRecordRaw(value)
}

func sealedDispatchFromRecordRaw(record runtimecontract.RuntimeDispatchRecord) runtimecontract.RuntimeDispatchRecord {
	return runtimecontract.CloneRuntimeDispatchRecordRaw(record)
}
