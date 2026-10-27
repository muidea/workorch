package events

import (
	"reflect"
	"slices"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
	"github.com/muidea/workorch/internal/pkg/workorchmanagedconfig"
)

const TopicObservePreparationManagedConfigs = "/file/startup-preparation/managed-configs/query"
const PreparationManagedConfigsVersion = "workorch-file-preparation-managed-configs/v1"

// Targets select exact roots, never the workspace-first fallback resolver.
// Defaults and policy verdicts are not sent to File.
type PreparationManagedConfigTarget struct{ Scope, Path string }

type ObservePreparationManagedConfigsCommand struct {
	WorkspaceID            common.WorkspaceID
	RequestID, PayloadHash string
	WorkspaceRevision      uint64
	InternalDirectory      string
	Targets                []PreparationManagedConfigTarget
}

func (c ObservePreparationManagedConfigsCommand) Clone() ObservePreparationManagedConfigsCommand {
	c.Targets = slices.Clone(c.Targets)
	return c
}

func (c ObservePreparationManagedConfigsCommand) Validate() error {
	identity := ObservePreparationDirectoriesCommand{WorkspaceID: c.WorkspaceID, RequestID: c.RequestID, PayloadHash: c.PayloadHash, WorkspaceRevision: c.WorkspaceRevision, InternalDirectory: c.InternalDirectory, DeliverableDirectory: c.InternalDirectory}
	if identity.Validate() != nil || len(c.Targets) > fileobservation.ResourceLimit {
		return common.ErrInvalidArgument
	}
	seen := make(map[PreparationManagedConfigTarget]bool)
	for _, target := range c.Targets {
		if !workorchmanagedconfig.ValidPreparationTarget(target.Scope, target.Path) || seen[target] {
			return common.ErrInvalidArgument
		}
		seen[target] = true
	}
	return nil
}

type ObservePreparationManagedConfigsResult struct {
	Version                string
	Command                ObservePreparationManagedConfigsCommand
	StartedAt, CompletedAt time.Time
	Files                  []fileobservation.Resource
}

func (r ObservePreparationManagedConfigsResult) ValidateFor(c ObservePreparationManagedConfigsCommand) error {
	if c.Validate() != nil || r.Version != PreparationManagedConfigsVersion || !reflect.DeepEqual(r.Command, c) || r.StartedAt.IsZero() || r.CompletedAt.Before(r.StartedAt) || len(r.Files) != len(c.Targets) {
		return common.ErrConflict
	}
	total := 0
	for i, item := range r.Files {
		if item.Ref != c.Targets[i].Path || item.Validate() != nil {
			return common.ErrConflict
		}
		total += item.ContentBytes
	}
	if total > fileobservation.ResourceTotalBytes {
		return common.ErrConflict
	}
	return nil
}
