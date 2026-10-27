package events

import (
	"reflect"

	"github.com/muidea/workorch/internal/modules/blocks/file/pkg/preparation"
	"github.com/muidea/workorch/internal/pkg/common"
)

const TopicReobservePreparationFiles = "/file/startup-preparation/revalidation/query"
const TopicQueryPreparationEpoch = "/file/startup-preparation/epoch/query"

type ReobservePreparationFilesCommand struct{ Scope preparation.Scope }
type ReobservePreparationFilesResult struct{ Observation preparation.Observation }

func (r ReobservePreparationFilesResult) ValidateFor(c ReobservePreparationFilesCommand) error {
	if c.Scope.Validate() != nil || r.Observation.Validate() != nil || !reflect.DeepEqual(c.Scope, r.Observation.Scope) {
		return common.ErrConflict
	}
	return nil
}

// Pure in-memory identity query: no filesystem read, token refresh or receipt.
type QueryPreparationEpochCommand struct{}
type QueryPreparationEpochResult struct{ OwnerEpoch string }
