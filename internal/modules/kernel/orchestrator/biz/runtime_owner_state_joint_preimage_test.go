package orchestrator

import (
	"fmt"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"reflect"
)

// Independent full replay for explicitly supplied raw/converted audit inputs.
func referenceOwnerStateReload(record runtimecontract.RuntimeStateRecord, converted runtime.RuntimeAggregate) error {
	if err := record.Validate(); err != nil {
		return fmt.Errorf("load Runtime state: %w", err)
	}
	if !reflect.DeepEqual(record, runtime.RuntimeStateRecordFromAggregate(converted)) {
		return fmt.Errorf("runtime owner record conversion changed original data")
	}
	return converted.Validate()
}
