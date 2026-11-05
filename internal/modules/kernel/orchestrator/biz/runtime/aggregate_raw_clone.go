package runtime

import runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

// CloneRaw detaches the sole state representation without normalization.
func (a RuntimeAggregate) CloneRaw() RuntimeAggregate {
	return RuntimeAggregate(ownerStateRecord(a).Clone())
}

func cloneSealedDispatchRaw(value runtimecontract.RuntimeDispatchRecord) runtimecontract.RuntimeDispatchRecord {
	return runtimecontract.CloneRuntimeDispatchRecordRaw(value)
}
