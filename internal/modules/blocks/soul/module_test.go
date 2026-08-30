package soul

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	soulbiz "github.com/muidea/workorch/internal/modules/blocks/soul/biz"

	magicevent "github.com/muidea/magicCommon/event"
	soulevents "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/events"
	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	hubevents "github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestModuleIdentityConfigureStartStopContract(t *testing.T) {
	module := New()
	if ModuleID != "blocks/soul" {
		t.Fatalf("ModuleID = %q, want relocated stateful block ID", ModuleID)
	}
	if module.ID() != ModuleID {
		t.Fatalf("ID = %q, want %q", module.ID(), ModuleID)
	}
	if err := module.Start(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Start before Configure err = %v, want invalid argument", err)
	}
	if err := module.Configure(soulModuleContractDependencies()); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if module.bizPtr == nil || module.bizPtr.EventHub() == nil {
		t.Fatalf("module not fully configured: %+v", module)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if module.bizPtr != nil {
		t.Fatalf("module was not cleared after Stop: %+v", module)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestModuleConfigureRequiresSoulRepository(t *testing.T) {
	if err := New().Configure(soulbiz.Dependencies{}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Configure err = %v, want invalid argument", err)
	}
	module := New()
	if err := module.Configure(soulbiz.Dependencies{Souls: memoryrepo.NewSoulRepository()}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Configure without EventHub err = %v, want invalid argument", err)
	}
}

func TestModuleSoulCommandsUseRegistryContract(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	module := New()
	if err := module.Configure(soulbiz.Dependencies{Souls: memoryrepo.NewSoulRepository(), EventHub: hub}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		if err := module.Stop(context.Background()); err != nil {
			t.Errorf("Stop: %v", err)
		}
	})

	createResult := sendSoulCommand[soulevents.CreateSoulResult](t, ctx, hub, soulevents.TopicSoulCommandCreate,
		soulevents.CreateSoulCommand{
			SoulID:          "soul-contract",
			Name:            "Contract Soul",
			SoulMD:          "# Contract Soul",
			BusinessDomains: []string{"contract"},
		},
	)
	if createResult.Soul == nil || createResult.Soul.ID != "soul-contract" || createResult.Soul.Name != "Contract Soul" {
		t.Fatalf("soul = %+v, want created soul", createResult.Soul)
	}

	listResult := sendSoulCommand[soulevents.ListSoulsResult](t, ctx, hub, soulevents.TopicSoulCommandList, soulevents.ListSoulsCommand{})
	if len(listResult.Souls) != 1 || listResult.Souls[0].ID != "soul-contract" {
		t.Fatalf("souls = %+v, want created soul", listResult.Souls)
	}
}

func TestDeleteSoulCommandAcceptsContractPayload(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	repo := memoryrepo.NewSoulRepository()
	if err := repo.Create(ctx, &soulmodel.Soul{ID: "soul-delete-contract", Name: "Delete Contract", SoulMD: "# soul"}); err != nil {
		t.Fatalf("seed soul: %v", err)
	}
	module := New()
	if err := module.Configure(soulbiz.Dependencies{Souls: repo, EventHub: hub}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		if err := module.Stop(context.Background()); err != nil {
			t.Errorf("Stop: %v", err)
		}
	})

	deleted := sendSoulCommand[soulevents.DeleteSoulResult](t, ctx, hub, soulevents.TopicSoulCommandDelete, soulevents.DeleteSoulCommand{SoulID: "soul-delete-contract"})
	if !deleted.Deleted || deleted.SoulID != "soul-delete-contract" {
		t.Fatalf("delete result = %+v", deleted)
	}
	if _, err := repo.Get(ctx, "soul-delete-contract"); err == nil {
		t.Fatalf("soul still exists after delete")
	}
}

func TestModuleSoulDuplicateCreateAndUpdateReplayAreOwnerConsistent(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	repository := memoryrepo.NewSoulRepository()
	module := New()
	if err := module.Configure(soulbiz.Dependencies{Souls: repository, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })
	command := soulevents.CreateSoulCommand{SoulID: "soul-duplicate", Name: "duplicate", SoulMD: "# Duplicate"}
	sendFrom := func(topic, source string, payload any) magicevent.Result {
		return hub.Send(magicevent.NewEventWithContext(
			topic, source, soulevents.CommandDestination,
			magicevent.NewHeader(), ctx, payload,
		))
	}
	for _, source := range []string{"untrusted/module", " application/workorchd/context-registry "} {
		forged := sendFrom(soulevents.TopicSoulCommandCreate, source, command)
		if forged == nil || !errors.Is(forged.Error(), common.ErrInvalidArgument) {
			t.Fatalf("forged source %q result = %+v", source, forged)
		}
	}
	if items, err := repository.List(ctx); err != nil || len(items) != 0 {
		t.Fatalf("forged create mutated Soul owner: %+v err=%v", items, err)
	}
	send := func(topic string, payload any) magicevent.Result {
		return sendFrom(topic, "application/workorchd/context-registry", payload)
	}
	first := send(soulevents.TopicSoulCommandCreate, command)
	if first == nil || first.Error() != nil {
		t.Fatalf("first create = %+v", first)
	}
	duplicate := send(soulevents.TopicSoulCommandCreate, command)
	if duplicate == nil || !errors.Is(duplicate.Error(), common.ErrConflict) {
		t.Fatalf("duplicate create = %+v, want conflict", duplicate)
	}
	update := soulevents.UpdateSoulCommand(command)
	update.Name = "duplicate updated"
	update.SoulMD = "# Duplicate Updated\n"
	firstUpdate := send(soulevents.TopicSoulCommandUpdate, update)
	secondUpdate := send(soulevents.TopicSoulCommandUpdate, update)
	for index, result := range []magicevent.Result{firstUpdate, secondUpdate} {
		if result == nil || result.Error() != nil {
			t.Fatalf("update replay %d = %+v", index+1, result)
		}
		data, err := result.Get()
		response, ok := data.(soulevents.UpdateSoulResult)
		if err != nil || !ok || response.Soul == nil || response.Soul.Revision != 2 || response.Soul.Name != update.Name || response.Soul.SoulMD != update.SoulMD {
			t.Fatalf("update replay %d response = %#v err=%v", index+1, data, err)
		}
	}
	items, err := repository.List(ctx)
	if err != nil || len(items) != 1 || items[0].Revision != 2 || items[0].Name != update.Name || items[0].SoulMD != update.SoulMD {
		t.Fatalf("souls after duplicate/replay = %+v err=%v", items, err)
	}
}

func TestModuleSoulStateLaneSerializesConcurrentUpdatesLatestCommitWins(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	inner := memoryrepo.NewSoulRepository()
	repository := &blockingSoulRepository{
		SoulRepository: inner,
		commitEntered:  make(chan struct{}),
		releaseCommit:  make(chan struct{}),
	}
	defer func() {
		select {
		case <-repository.releaseCommit:
		default:
			close(repository.releaseCommit)
		}
	}()
	module := New()
	if err := module.Configure(soulbiz.Dependencies{Souls: repository, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })

	soulID := common.SoulID("soul-latest-command-wins")
	lane := hubevents.EventHubLaneKeyFor(
		soulevents.Contracts(),
		soulevents.TopicSoulCommandUpdate,
		eventcontract.LaneScope{SubjectType: "soul", SubjectID: string(soulID)},
	)
	if lane != "state/soul/"+string(soulID) {
		t.Fatalf("Soul update lane = %q", lane)
	}
	send := func(topic string, payload any) magicevent.Result {
		ev := magicevent.NewEventWithContext(
			topic, "application/workorchd/context-registry", soulevents.CommandDestination,
			magicevent.NewHeader(), ctx, payload,
		)
		ev.BindLaneKey(lane)
		return hub.Send(ev)
	}
	create := soulevents.CreateSoulCommand{SoulID: soulID, Name: "base", SoulMD: "# Base"}
	if result := send(soulevents.TopicSoulCommandCreate, create); result == nil || result.Error() != nil {
		t.Fatalf("create = %+v", result)
	}

	repository.blockNextCommit.Store(true)
	firstCommand := soulevents.UpdateSoulCommand(create)
	firstCommand.Name = "first committed"
	firstCommand.SoulMD = "# First"
	secondCommand := soulevents.UpdateSoulCommand(create)
	secondCommand.Name = "second committed"
	secondCommand.SoulMD = "# Second"
	firstDone := make(chan magicevent.Result, 1)
	go func() { firstDone <- send(soulevents.TopicSoulCommandUpdate, firstCommand) }()
	select {
	case <-repository.commitEntered:
	case <-time.After(time.Second):
		t.Fatal("first Soul update did not reach persistence cut")
	}
	loadsAtCut := repository.gets.Load()
	secondStarted := make(chan struct{})
	secondDone := make(chan magicevent.Result, 1)
	go func() {
		close(secondStarted)
		secondDone <- send(soulevents.TopicSoulCommandUpdate, secondCommand)
	}()
	<-secondStarted
	select {
	case result := <-secondDone:
		t.Fatalf("second same-Soul update bypassed the first state-lane transaction: %+v", result)
	case <-time.After(50 * time.Millisecond):
	}
	if got := repository.gets.Load(); got != loadsAtCut {
		t.Fatalf("second same-Soul update reached repository before first commit: gets=%d, want %d", got, loadsAtCut)
	}
	close(repository.releaseCommit)

	first := requireSoulUpdateResult(t, <-firstDone)
	second := requireSoulUpdateResult(t, <-secondDone)
	if first.Soul == nil || first.Soul.Revision != 2 || first.Soul.Name != firstCommand.Name {
		t.Fatalf("first committed update = %+v", first)
	}
	if second.Soul == nil || second.Soul.Revision != 3 || second.Soul.Name != secondCommand.Name {
		t.Fatalf("second committed update = %+v", second)
	}
	replay := requireSoulUpdateResult(t, send(soulevents.TopicSoulCommandUpdate, secondCommand))
	if replay.Soul == nil || replay.Soul.Revision != second.Soul.Revision || replay.Soul.ContentHash != second.Soul.ContentHash {
		t.Fatalf("exact replay = %+v, want unchanged %+v", replay, second)
	}
	if got := repository.updates.Load(); got != 2 {
		t.Fatalf("repository updates = %d, want two content-changing commands and replay no-op", got)
	}
	stored, err := inner.Get(ctx, soulID)
	if err != nil || stored.Revision != 3 || stored.Name != secondCommand.Name || stored.ContentHash != second.Soul.ContentHash {
		t.Fatalf("latest committed Soul = %+v err=%v", stored, err)
	}
}

type blockingSoulRepository struct {
	repository.SoulRepository
	blockNextCommit atomic.Bool
	gets            atomic.Int32
	updates         atomic.Int32
	commitEntered   chan struct{}
	releaseCommit   chan struct{}
}

func (r *blockingSoulRepository) Get(ctx context.Context, id common.SoulID) (*soulmodel.Soul, error) {
	r.gets.Add(1)
	return r.SoulRepository.Get(ctx, id)
}

func (r *blockingSoulRepository) Update(ctx context.Context, item *soulmodel.Soul) error {
	r.updates.Add(1)
	if r.blockNextCommit.CompareAndSwap(true, false) {
		close(r.commitEntered)
		<-r.releaseCommit
	}
	return r.SoulRepository.Update(ctx, item)
}

func requireSoulUpdateResult(t *testing.T, result magicevent.Result) soulevents.UpdateSoulResult {
	t.Helper()
	if result == nil || result.Error() != nil {
		t.Fatalf("Soul update result = %+v", result)
	}
	data, err := result.Get()
	if err != nil {
		t.Fatalf("Soul update result error = %v", err)
	}
	response, ok := data.(soulevents.UpdateSoulResult)
	if !ok {
		t.Fatalf("Soul update result type = %T", data)
	}
	return response
}

func TestModuleStopFencesSubsequentSoulDelivery(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	hub := &soulLifecycleHub{
		Hub:             rawHub,
		subscriptions:   map[string]int{},
		unsubscriptions: map[string]int{},
	}
	t.Cleanup(func() { rawHub.Terminate(context.Background()) })
	repository := memoryrepo.NewSoulRepository()
	module := New()
	if err := module.Configure(soulbiz.Dependencies{Souls: repository, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	if err := module.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	for _, topic := range soulLifecycleTopics() {
		if hub.subscriptions[topic] != 1 || hub.unsubscriptions[topic] != 1 {
			t.Fatalf("topic %s lifecycle subscriptions=%d unsubscriptions=%d, want 1/1", topic, hub.subscriptions[topic], hub.unsubscriptions[topic])
		}
	}
	command := soulevents.CreateSoulCommand{SoulID: "soul-after-stop", Name: "after stop", SoulMD: "# After Stop"}
	result := hub.Send(magicevent.NewEventWithContext(
		soulevents.TopicSoulCommandCreate, "application/workorchd/context-registry", soulevents.CommandDestination,
		magicevent.NewHeader(), ctx, command,
	))
	if result != nil && result.Error() == nil {
		t.Fatalf("post-Stop command unexpectedly succeeded: %+v", result)
	}
	if _, err := repository.Get(ctx, command.SoulID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("post-Stop command mutated owner: %v", err)
	}
}

type soulLifecycleHub struct {
	magicevent.Hub
	subscriptions   map[string]int
	unsubscriptions map[string]int
}

func (h *soulLifecycleHub) Subscribe(topic string, observer magicevent.Observer) {
	h.subscriptions[topic]++
	h.Hub.Subscribe(topic, observer)
}

func (h *soulLifecycleHub) Unsubscribe(topic string, observer magicevent.Observer) {
	h.unsubscriptions[topic]++
	h.Hub.Unsubscribe(topic, observer)
}

func soulLifecycleTopics() []string {
	return []string{
		soulevents.TopicSoulCommandCreate,
		soulevents.TopicSoulCommandGet,
		soulevents.TopicSoulCommandList,
		soulevents.TopicSoulCommandUpdate,
		soulevents.TopicSoulCommandDelete,
		soulevents.TopicSoulCommandValidateCapability,
		soulevents.TopicSoulCommandEnsureDefaults,
	}
}

func soulModuleContractDependencies() soulbiz.Dependencies {
	return soulbiz.Dependencies{
		Souls:    memoryrepo.NewSoulRepository(),
		EventHub: magicevent.NewHub(16),
	}
}

func sendSoulCommand[Response any](t *testing.T, ctx context.Context, hub magicevent.Hub, topic string, command any) Response {
	t.Helper()
	source := "application/workorchd/context-registry"
	if topic == soulevents.TopicSoulCommandGet {
		source = "application/workorchd/appservice"
	}
	ev := magicevent.NewEventWithContext(topic, source, soulevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	result := hub.Send(ev)
	if result == nil || result.Error() != nil {
		t.Fatalf("send %s error: %v", topic, result)
	}
	data, err := result.Get()
	if err != nil {
		t.Fatalf("result %s: %v", topic, err)
	}
	typed, ok := data.(Response)
	if !ok {
		t.Fatalf("result %s type = %T", topic, data)
	}
	return typed
}
