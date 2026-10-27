package events

import (
	"reflect"
	"slices"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
)

const TopicObservePreparationResources = "/file/startup-preparation/resources/query"
const PreparationResourcesVersion = "workorch-file-preparation-resources/v1"

// Only Workspace can resolve these exact sealed refs. File observes bytes;
// it does not consume draft scope or decide whether a requirement is satisfied.
type ObservePreparationResourcesCommand struct {
	WorkspaceID            common.WorkspaceID
	RequestID, PayloadHash string
	WorkspaceRevision      uint64
	DeliverableDirectory   string
	ResourceRefs           []string
}

func (c ObservePreparationResourcesCommand) Clone() ObservePreparationResourcesCommand {
	c.ResourceRefs = slices.Clone(c.ResourceRefs)
	return c
}

func (c ObservePreparationResourcesCommand) Validate() error {
	identity := ObservePreparationDirectoriesCommand{WorkspaceID: c.WorkspaceID, RequestID: c.RequestID, PayloadHash: c.PayloadHash, WorkspaceRevision: c.WorkspaceRevision, InternalDirectory: c.DeliverableDirectory, DeliverableDirectory: c.DeliverableDirectory}
	if identity.Validate() != nil || len(c.ResourceRefs) > fileobservation.ResourceLimit {
		return common.ErrInvalidArgument
	}
	for _, ref := range c.ResourceRefs {
		if !fileobservation.ValidResourceRef(ref) {
			return common.ErrInvalidArgument
		}
	}
	return nil
}

type ObservePreparationResourcesResult struct {
	Version                string
	Command                ObservePreparationResourcesCommand
	StartedAt, CompletedAt time.Time
	Resources              []fileobservation.Resource
}

func (r ObservePreparationResourcesResult) ValidateFor(c ObservePreparationResourcesCommand) error {
	if c.Validate() != nil || r.Version != PreparationResourcesVersion || !reflect.DeepEqual(r.Command, c) || r.StartedAt.IsZero() || r.CompletedAt.Before(r.StartedAt) || len(r.Resources) != len(c.ResourceRefs) {
		return common.ErrConflict
	}
	total := 0
	for i, item := range r.Resources {
		if item.Ref != c.ResourceRefs[i] || item.Validate() != nil {
			return common.ErrConflict
		}
		total += item.ContentBytes
	}
	if total > fileobservation.ResourceTotalBytes {
		return common.ErrConflict
	}
	return nil
}
