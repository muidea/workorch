package runtime

import runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

// ownerStateRecord is a read-only value view. It owns no additional graph and
// performs no normalization, copying, validation or authority selection.
func ownerStateRecord(a RuntimeAggregate) runtimecontract.RuntimeStateRecord {
	return runtimecontract.RuntimeStateRecord(a)
}

// Public boundaries return detached original values. Internal pure consumers
// use the same representation; only an actual ownership boundary clones it.
func RuntimeStateRecordFromAggregate(a RuntimeAggregate) runtimecontract.RuntimeStateRecord {
	return ownerStateRecord(a).Clone()
}

func RuntimeAggregateFromRecord(record runtimecontract.RuntimeStateRecord) RuntimeAggregate {
	return RuntimeAggregate(record.Clone())
}
