package file

import (
	"context"
	"crypto/rand"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	"github.com/muidea/workorch/internal/modules/blocks/file/pkg/preparation"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
	"github.com/muidea/workorch/internal/pkg/workorchmanagedconfig"
)

func (m *Biz) handleQueryPreparationEpoch(ev magicevent.Event, result magicevent.Result) {
	if result == nil {
		return
	}
	if ev == nil || ev.ID() != fileevents.TopicQueryPreparationEpoch || ev.Source() != "blocks/workspace" {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	if _, ok := ev.Data().(fileevents.QueryPreparationEpochCommand); !ok {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	if setPrerequisiteError(result, ev.Context().Err()) {
		return
	}
	result.Set(fileevents.QueryPreparationEpochResult{OwnerEpoch: m.preparationEpoch}, nil)
}

func (m *Biz) handleReobservePreparationFiles(ev magicevent.Event, result magicevent.Result) {
	if result == nil {
		return
	}
	if ev == nil || ev.ID() != fileevents.TopicReobservePreparationFiles || ev.Source() != "blocks/workspace" {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	c, ok := ev.Data().(fileevents.ReobservePreparationFilesCommand)
	if !ok || c.Scope.Validate() != nil {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	ctx, err := common.BindRuntimeWorkspaceContext(ev.Context(), c.Scope.WorkspaceID)
	if setPrerequisiteError(result, err) {
		return
	}
	o, err := m.reobservePreparationFiles(ctx, c.Scope)
	if setPrerequisiteError(result, err) {
		return
	}
	result.Set(fileevents.ReobservePreparationFilesResult{Observation: o}, nil)
}

func (m *Biz) reobservePreparationFiles(ctx context.Context, scope preparation.Scope) (preparation.Observation, error) {
	if ctx == nil || scope.Validate() != nil || !preparation.ValidEpoch(m.preparationEpoch) {
		return preparation.Observation{}, common.ErrInvalidArgument
	}
	if err := ctx.Err(); err != nil {
		return preparation.Observation{}, err
	}
	o := preparation.Observation{Version: preparation.Version, Scope: scope.Clone(), OwnerEpoch: m.preparationEpoch, TokenID: rand.Text(), StartedAt: time.Now().UTC()}
	o.Internal, o.Deliverable = observePreparationDirectory(scope.InternalDirectory), observePreparationDirectory(scope.DeliverableDirectory)
	remaining := fileobservation.ResourceTotalBytes
	for _, ref := range scope.ResourceRefs {
		if err := ctx.Err(); err != nil {
			return preparation.Observation{}, err
		}
		r := observePreparationResource(scope.DeliverableDirectory, ref, remaining)
		remaining -= r.ContentBytes
		o.Resources = append(o.Resources, r)
	}
	remaining = fileobservation.ResourceTotalBytes
	for _, target := range scope.Configs {
		if err := ctx.Err(); err != nil {
			return preparation.Observation{}, err
		}
		root := scope.InternalDirectory
		if target.Scope == workorchmanagedconfig.ScopeSystem {
			root = workorchmanagedconfig.SystemDirectory(root)
		}
		r := observePreparationFile(root, target.Path, remaining)
		remaining -= r.ContentBytes
		o.Configs = append(o.Configs, r)
	}
	o.ObservedAt, o.ExpiresAt = time.Now().UTC(), o.StartedAt.Add(preparation.MaxAge)
	if err := ctx.Err(); err != nil {
		return preparation.Observation{}, err
	}
	return o, o.Validate()
}
