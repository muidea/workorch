package result

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestTerminalResultConflictPreservesOwnerStageAndImmutableFields(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	defer hub.Terminate(ctx)
	repos := memory.NewResultOwnerRepositorySet()
	owner, err := New(hub, nil, Dependencies{OwnerRepositories: repos.OwnerRepositories(), EventHub: hub})
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Teardown(ctx)
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	command := terminalDeliveryFixture("diagnostic-original-delivery", nil)
	forged := hub.Send(magicevent.NewEventWithContext(resultevents.TopicResultFactTerminalDelivery, "untrusted-owner", resultevents.CommandDestination, magicevent.NewHeader(), ctx, command))
	var admission *resultevents.TerminalDeliveryFailure
	if forged == nil || !errors.As(forged.Error(), &admission) || admission.Stage != resultevents.TerminalDeliveryStageCommand || !errors.Is(forged.Error(), common.ErrInvalidArgument) {
		t.Fatalf("command rejection misattributed to workspace fence: %v", forged)
	}
	first, err := owner.applyTerminalDelivery(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	original := resultevents.CloneResult(first.Result)
	conflicting := command
	conflicting.Material.Summary = "different immutable summary"
	response := hub.Send(magicevent.NewEventWithContext(resultevents.TopicResultFactTerminalDelivery, "kernel/orchestrator", resultevents.CommandDestination, magicevent.NewHeader(), ctx, conflicting))
	var detail *resultevents.TerminalDeliveryFailure
	if response == nil || !errors.As(response.Error(), &detail) || !errors.Is(response.Error(), common.ErrConflict) {
		t.Fatalf("typed cause lost at EventHub: %v", response)
	}
	if detail.Stage != resultevents.TerminalDeliveryStageResultCompare || detail.ResultID != first.Result.ID || !reflect.DeepEqual(detail.MismatchFields, []string{"Summary", "TerminalMaterialHash"}) {
		t.Fatalf("imprecise conflict: %+v", detail)
	}
	value, _ := response.Get()
	if value != nil {
		t.Fatal("conflict returned a success/ACK payload")
	}
	stored, err := owner.service.GetResult(ctx, first.Result.ID)
	if err != nil || !reflect.DeepEqual(stored, original) {
		t.Fatal("conflict changed original Result")
	}
	replayed, err := owner.applyTerminalDelivery(ctx, command)
	if err != nil || replayed.DeliveryAck == nil || replayed.DeliveryAck.AcknowledgedState != runtimecontract.RuntimeDeliveryAcknowledgedNoop {
		t.Fatalf("diagnostic blocked exact idempotent replay: %v", err)
	}
}

type terminalReadBackFailureRepository struct {
	repository.ResultRepository
	written  bool
	original error
}

func (r *terminalReadBackFailureRepository) Create(ctx context.Context, item *resultmodel.Result) error {
	if err := r.ResultRepository.Create(ctx, item); err != nil {
		return err
	}
	r.written = true
	return nil
}

func (r *terminalReadBackFailureRepository) Get(ctx context.Context, id common.ResultID) (*resultmodel.Result, error) {
	if r.written {
		return nil, r.original
	}
	return r.ResultRepository.Get(ctx, id)
}

func TestTerminalResultWrittenButReadBackFailureHasNoACKAndCanRecover(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(8)
	defer hub.Terminate(ctx)
	repos := memory.NewResultOwnerRepositorySet()
	original := errors.New("exact owner read error\r\n\x00\xff")
	results := &terminalReadBackFailureRepository{ResultRepository: repos.Results, original: original}
	owner, err := New(hub, nil, Dependencies{OwnerRepositories: resultBizTestOwnerRepositories(results, repos.Artifacts, repos.WorkspaceDeletions), EventHub: hub})
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Teardown(ctx)
	command := terminalDeliveryFixture("read-back-cut", nil)
	response, err := owner.applyTerminalDelivery(ctx, command)
	var detail *resultevents.TerminalDeliveryFailure
	if !errors.As(err, &detail) || detail.Stage != resultevents.TerminalDeliveryStageResultReadBack || !errors.Is(err, original) || detail.Cause.Error() != original.Error() || !strings.Contains(err.Error(), original.Error()) {
		t.Fatalf("original read-back error lost: %v", err)
	}
	if response.DeliveryAck != nil {
		t.Fatal("failed read-back granted ACK")
	}
	// The durable write stays intact; recovery consumes the same command.
	results.written = false
	response, err = owner.applyTerminalDelivery(ctx, command)
	if err != nil || response.DeliveryAck == nil || response.Transitioned {
		t.Fatalf("written-before-ACK recovery: %+v %v", response, err)
	}
}
