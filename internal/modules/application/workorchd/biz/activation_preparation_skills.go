package biz

import (
	"context"
	"reflect"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	skillevents "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
)

// PreparationSkillRegistryObservation is restricted diagnostic material. It is
// not the full Registry+SkillHub catalog, selection proof, persisted readiness
// or M3. Reobserved records prove agreement only at these observation points,
// not a cross-owner atomic snapshot or immunity to subsequent changes.
type PreparationSkillRegistryObservation struct {
	WorkspaceID            common.WorkspaceID
	WorkspaceRevision      uint64
	SnapshotHash           string
	Catalog                skillevents.ObserveRegistryCatalogResult
	ReobservedCatalog      skillevents.ObserveRegistryCatalogResult
	ReobservedSnapshotHash string
	StartedAt              time.Time
	CompletedAt            time.Time
	FailureCode            string
	Reobserved             bool
	SealBindingCurrent     bool
}

func (b *Biz) ObservePreparationSkillRegistry(ctx context.Context, command workspaceevents.QueryExactConfirmedStartupSnapshotCommand) (PreparationSkillRegistryObservation, error) {
	value := PreparationSkillRegistryObservation{WorkspaceID: command.WorkspaceID, WorkspaceRevision: command.ExpectedWorkspaceRevision, StartedAt: time.Now().UTC()}
	fail := func(code string, err error) (PreparationSkillRegistryObservation, error) {
		value.FailureCode, value.CompletedAt = code, time.Now().UTC()
		return value, err
	}
	if b == nil || b.EventHub() == nil || ctx == nil || command.Validate() != nil {
		return fail("invalid_request", common.ErrInvalidArgument)
	}
	if err := ctx.Err(); err != nil {
		return fail("cancelled", err)
	}
	bound, err := common.BindRuntimeWorkspaceContext(ctx, command.WorkspaceID)
	if err != nil {
		return fail("scope_conflict", err)
	}
	bounded, cancel := context.WithTimeout(bound, 3*time.Second)
	defer cancel()
	before, err := b.QueryExactConfirmedStartupSnapshot(bounded, command)
	if err != nil {
		return fail("seal_unavailable", err)
	}
	value.SnapshotHash = before.Snapshot.SnapshotHash
	query := skillevents.ObserveRegistryCatalogCommand{WorkspaceID: command.WorkspaceID}
	value.Catalog, err = b.readPreparationSkillRegistry(bounded, query)
	if err != nil {
		return fail("registry_unavailable", err)
	}
	current, err := b.readPreparationSkillRegistry(bounded, query)
	value.ReobservedCatalog = current.Clone()
	if err != nil {
		return fail("registry_reobservation_failed", err)
	}
	if current.OwnerEpoch != value.Catalog.OwnerEpoch || current.ObservedAt.Before(value.Catalog.ObservedAt) || !reflect.DeepEqual(current.Skills, value.Catalog.Skills) {
		return fail("registry_drift", common.ErrConflict)
	}
	after, err := b.QueryExactConfirmedStartupSnapshot(bounded, command)
	if err != nil {
		return fail("seal_reobservation_failed", err)
	}
	if after.Snapshot.SnapshotHash != value.SnapshotHash {
		return fail("seal_drift", common.ErrConflict)
	}
	value.ReobservedSnapshotHash = after.Snapshot.SnapshotHash
	if err := bounded.Err(); err != nil {
		return fail("cancelled", err)
	}
	value.Reobserved, value.SealBindingCurrent = true, true
	value.CompletedAt = time.Now().UTC()
	return value, nil
}

// Facts retains both complete reads. Compatibility helper flags do not cross
// the settlement boundary and cannot substitute for independent validation.
func (v PreparationSkillRegistryObservation) Facts() workspace.PreparationSkillCatalogFacts {
	return workspace.PreparationSkillCatalogFacts{Version: "workorch-preparation-skill-registry-facts/v1", WorkspaceID: v.WorkspaceID, WorkspaceRevision: v.WorkspaceRevision, SnapshotHash: v.SnapshotHash, ReobservedSnapshotHash: v.ReobservedSnapshotHash, Catalog: v.Catalog.Snapshot(), ReobservedCatalog: v.ReobservedCatalog.Snapshot(), StartedAt: v.StartedAt, CompletedAt: v.CompletedAt}
}

func (b *Biz) readPreparationSkillRegistry(ctx context.Context, command skillevents.ObserveRegistryCatalogCommand) (skillevents.ObserveRegistryCatalogResult, error) {
	started := time.Now().UTC()
	ev := magicevent.NewEventWithContext(skillevents.TopicObserveRegistryCatalog, ModuleID, skillevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(skillevents.Contracts(), ev.ID(), eventcontract.LaneScope{WorkspaceID: string(command.WorkspaceID)}))
	result := b.SendEvent(ev)
	if result == nil {
		return skillevents.ObserveRegistryCatalogResult{}, common.ErrConflict
	}
	if err := result.Error(); err != nil {
		return skillevents.ObserveRegistryCatalogResult{}, err
	}
	data, err := result.Get()
	value, ok := data.(skillevents.ObserveRegistryCatalogResult)
	if err != nil || !ok || value.ValidateFor(command) != nil || value.ObservedAt.Before(started) || value.ObservedAt.After(time.Now().UTC()) {
		return skillevents.ObserveRegistryCatalogResult{}, common.ErrConflict
	}
	return value.Clone(), nil
}
