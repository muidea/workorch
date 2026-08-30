package mcp

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	mcpbiz "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/biz"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	hubevents "github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
	filerepo "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func TestModuleIdentityConfigureStartStopContract(t *testing.T) {
	module := New()
	if ModuleID != "blocks/mcp-registry" || module.ID() != ModuleID {
		t.Fatalf("module identity = %q/%q", ModuleID, module.ID())
	}
	if err := module.Start(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Start before Configure err = %v, want invalid argument", err)
	}
	if err := module.Configure(mcpModuleContractDependencies()); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if err := module.Stop(context.Background()); err != nil || module.bizPtr != nil {
		t.Fatalf("Stop: %v biz=%v", err, module.bizPtr)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestModuleConfigureRequiresAggregateAndEventHub(t *testing.T) {
	base := mcpModuleContractDependencies()
	tests := []struct {
		name string
		deps mcpbiz.Dependencies
	}{
		{name: "missing aggregate", deps: func() mcpbiz.Dependencies { d := base; d.Registry = nil; return d }()},
		{name: "missing event hub", deps: func() mcpbiz.Dependencies { d := base; d.EventHub = nil; return d }()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := New().Configure(tc.deps); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("Configure err = %v, want invalid argument", err)
			}
		})
	}
}

func TestModuleUsesConcreteCommandResultsAndConsistentSnapshot(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	registry := memory.NewMCPRegistryRepository()
	module := New()
	if err := module.Configure(mcpbiz.Dependencies{Registry: registry, EventHub: hub}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })

	create := sendMCPCommand[mcpevents.CreateServerCommand, mcpevents.CreateServerResult](t, ctx, hub, mcpevents.TopicMCPCommandCreateServer, mcpevents.CreateServerCommand{
		ServerID: "mcp-contract", Name: "contract", Transport: "stdio", Command: "mcp-contract", Enabled: true,
		EnvironmentRefs: []mcpevents.EnvironmentReference{{Name: "TOKEN", Reference: "env://MCP_TOKEN"}},
		AllowedTools:    []string{"repo.summary"},
	})
	if create.Server == nil || create.Server.ID != "mcp-contract" || create.RegistryRevision == "" {
		t.Fatalf("CreateServerResult = %+v", create)
	}
	snapshot := sendMCPCommand[mcpevents.ReadRegistrySnapshotCommand, mcpevents.RegistrySnapshotResult](t, ctx, hub, mcpevents.TopicMCPCommandReadRegistrySnapshot, mcpevents.ReadRegistrySnapshotCommand{})
	if snapshot.RegistryRevision != create.RegistryRevision || len(snapshot.Servers) != 1 || len(snapshot.Capabilities) != 1 {
		t.Fatalf("RegistrySnapshotResult = %+v", snapshot)
	}

	createdCapability := sendMCPCommand[mcpevents.CreateCapabilityCommand, mcpevents.CreateCapabilityResult](t, ctx, hub, mcpevents.TopicMCPCommandCreateCapability, mcpevents.CreateCapabilityCommand{
		CapabilityID: "cap-repo-summary", ServerID: "mcp-contract", Kind: "tool", Name: "repo.summary",
		DisplayName: "Repository Summary", Category: "repository", Tags: []string{"read"}, StageHints: []string{"planning"},
		EffectScope: "none", IdempotencyClass: "safe_to_replay", ValidationProofApplicability: "direct",
		Schema: mustMCPPayload(t, map[string]any{"type": "object"}), Enabled: true,
	})
	if createdCapability.Capability == nil || createdCapability.Capability.Source != "user" {
		t.Fatalf("CreateCapabilityResult = %+v", createdCapability)
	}
	selection := sendMCPCommand[mcpevents.ReadSelectionSnapshotCommand, mcpevents.SelectionSnapshotResult](t, ctx, hub, mcpevents.TopicMCPCommandReadSelectionSnapshot, mcpevents.ReadSelectionSnapshotCommand{
		ServerIDs: []common.MCPServerID{"mcp-contract"}, CapabilityIDs: []common.MCPCapabilityID{"cap-repo-summary"},
	})
	if len(selection.Servers) != 1 || len(selection.Capabilities) != 1 || selection.RegistryRevision != createdCapability.RegistryRevision {
		t.Fatalf("SelectionSnapshotResult = %+v", selection)
	}
}

func TestModulePostCannotMutateRegistry(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	registry := memory.NewMCPRegistryRepository()
	module := New()
	if err := module.Configure(mcpbiz.Dependencies{Registry: registry, EventHub: hub}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })

	control := sendMCPCommand[mcpevents.CreateServerCommand, mcpevents.CreateServerResult](
		t, ctx, hub, mcpevents.TopicMCPCommandCreateServer,
		mcpevents.CreateServerCommand{ServerID: "mcp-post-control", Name: "control", Transport: "stdio", Command: "mcp-post-control"},
	)
	before, err := registry.Load(ctx)
	if err != nil || control.Server == nil || before.Revision == 0 || len(before.Servers) != 1 {
		t.Fatalf("synchronous source precondition failed: control=%+v snapshot=%+v err=%v", control, before, err)
	}
	hub.Post(magicevent.NewEventWithContext(mcpevents.TopicMCPCommandCreateServer, "application/workorchd/context-registry", mcpevents.CommandDestination, magicevent.NewHeader(), ctx, mcpevents.CreateServerCommand{
		ServerID: "mcp-post", Name: "post", Transport: "stdio", Command: "mcp-post",
	}))
	deadline := time.Now().Add(250 * time.Millisecond)
	for time.Now().Before(deadline) {
		snapshot, err := registry.Load(ctx)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if snapshot.Revision != before.Revision || len(snapshot.Servers) != 1 || snapshot.Servers[0].ID != control.Server.ID || len(snapshot.Capabilities) != 0 {
			t.Fatalf("Post mutated registry: %+v", snapshot)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestModuleRejectsForgedMCPRegistryMutationWithoutOwnerIOEffect(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	registry := memory.NewMCPRegistryRepository()
	module := New()
	if err := module.Configure(mcpbiz.Dependencies{Registry: registry, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })
	command := mcpevents.CreateServerCommand{ServerID: "mcp-forged", Name: "forged", Transport: "stdio", Command: "mcp-forged"}
	for _, source := range []string{"untrusted/module", " application/workorchd/context-registry "} {
		result := hub.Send(magicevent.NewEventWithContext(
			mcpevents.TopicMCPCommandCreateServer, source, mcpevents.CommandDestination,
			magicevent.NewHeader(), ctx, command,
		))
		if result == nil || !errors.Is(result.Error(), common.ErrInvalidArgument) {
			t.Fatalf("forged source %q result = %+v", source, result)
		}
	}
	snapshot, err := registry.Load(ctx)
	if err != nil || snapshot.Revision != 0 || len(snapshot.Servers) != 0 || len(snapshot.Capabilities) != 0 {
		t.Fatalf("forged mutation changed registry: %+v err=%v", snapshot, err)
	}
}

func TestModuleFileOwnerRestartReturnsConsistentEventHubSnapshot(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "mcp_registry.json")
	firstHub := magicevent.NewHub(16)
	first := New()
	if err := first.Configure(mcpbiz.Dependencies{Registry: filerepo.NewMCPRegistryRepository(path), EventHub: firstHub}); err != nil {
		t.Fatal(err)
	}
	if err := first.Start(); err != nil {
		t.Fatal(err)
	}
	created := sendMCPCommand[mcpevents.CreateServerCommand, mcpevents.CreateServerResult](t, ctx, firstHub, mcpevents.TopicMCPCommandCreateServer, mcpevents.CreateServerCommand{
		ServerID: "mcp-restart", Name: "restart", Transport: "stdio", Command: "mcp-restart", Enabled: true,
		AllowedTools: []string{"repo.summary"},
	})
	if err := first.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	firstHub.Terminate(ctx)

	secondHub := magicevent.NewHub(16)
	second := New()
	if err := second.Configure(mcpbiz.Dependencies{Registry: filerepo.NewMCPRegistryRepository(path), EventHub: secondHub}); err != nil {
		t.Fatal(err)
	}
	if err := second.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = second.Stop(ctx)
		secondHub.Terminate(ctx)
	})
	snapshot := sendMCPCommand[mcpevents.ReadRegistrySnapshotCommand, mcpevents.RegistrySnapshotResult](t, ctx, secondHub, mcpevents.TopicMCPCommandReadRegistrySnapshot, mcpevents.ReadRegistrySnapshotCommand{})
	if snapshot.RegistryRevision != created.RegistryRevision || len(snapshot.Servers) != 1 || snapshot.Servers[0].ID != created.Server.ID || len(snapshot.Capabilities) != 1 {
		t.Fatalf("restarted snapshot = %+v, created=%+v", snapshot, created)
	}
}

func TestModuleMCPRegistryStateLaneSerializesConcurrentUpdatesLatestCommitWins(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	inner := memory.NewMCPRegistryRepository()
	repository := &blockingMCPRegistryRepository{
		MCPRegistryRepository: inner,
		commitEntered:         make(chan struct{}),
		releaseCommit:         make(chan struct{}),
	}
	defer func() {
		select {
		case <-repository.releaseCommit:
		default:
			close(repository.releaseCommit)
		}
	}()
	module := New()
	if err := module.Configure(mcpbiz.Dependencies{Registry: repository, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })

	serverID := common.MCPServerID("mcp-latest-command-wins")
	lane := hubevents.EventHubLaneKeyFor(
		mcpevents.Contracts(),
		mcpevents.TopicMCPCommandUpdateServer,
		eventcontract.LaneScope{SubjectType: "mcp_server", SubjectID: string(serverID)},
	)
	if lane != "state/mcp-registry/"+string(serverID) {
		t.Fatalf("MCP server update lane = %q", lane)
	}
	send := func(topic string, payload any) magicevent.Result {
		ev := magicevent.NewEventWithContext(
			topic, "application/workorchd/context-registry", mcpevents.CommandDestination,
			magicevent.NewHeader(), ctx, payload,
		)
		ev.BindLaneKey(lane)
		return hub.Send(ev)
	}
	create := mcpevents.CreateServerCommand{
		ServerID: serverID, Name: "base", Transport: "stdio", Command: "mcp-latest-command-wins", Enabled: true,
	}
	if result := send(mcpevents.TopicMCPCommandCreateServer, create); result == nil || result.Error() != nil {
		t.Fatalf("create = %+v", result)
	}

	repository.blockNextCommit.Store(true)
	firstCommand := mcpevents.UpdateServerCommand(create)
	firstCommand.Name = "first committed"
	secondCommand := mcpevents.UpdateServerCommand(create)
	secondCommand.Name = "second committed"
	firstDone := make(chan magicevent.Result, 1)
	go func() { firstDone <- send(mcpevents.TopicMCPCommandUpdateServer, firstCommand) }()
	select {
	case <-repository.commitEntered:
	case <-time.After(time.Second):
		t.Fatal("first MCP Registry update did not reach persistence cut")
	}
	loadsAtCut := repository.loads.Load()
	secondStarted := make(chan struct{})
	secondDone := make(chan magicevent.Result, 1)
	go func() {
		close(secondStarted)
		secondDone <- send(mcpevents.TopicMCPCommandUpdateServer, secondCommand)
	}()
	<-secondStarted
	select {
	case result := <-secondDone:
		t.Fatalf("second same-server update bypassed the first state-lane transaction: %+v", result)
	case <-time.After(50 * time.Millisecond):
	}
	if got := repository.loads.Load(); got != loadsAtCut {
		t.Fatalf("second same-server update loaded the aggregate before first commit: loads=%d, want %d", got, loadsAtCut)
	}
	close(repository.releaseCommit)

	first := requireMCPServerUpdateResult(t, <-firstDone)
	second := requireMCPServerUpdateResult(t, <-secondDone)
	if first.Server == nil || first.Server.Name != firstCommand.Name || first.RegistryRevision == "" {
		t.Fatalf("first committed update = %+v", first)
	}
	if second.Server == nil || second.Server.Name != secondCommand.Name || second.RegistryRevision == "" || second.RegistryRevision == first.RegistryRevision {
		t.Fatalf("second committed update = %+v, first=%+v", second, first)
	}
	commitsBeforeReplay := repository.commits.Load()
	replay := requireMCPServerUpdateResult(t, send(mcpevents.TopicMCPCommandUpdateServer, secondCommand))
	if replay.Server == nil || replay.Server.Name != second.Server.Name || replay.RegistryRevision != second.RegistryRevision {
		t.Fatalf("exact replay = %+v, want unchanged %+v", replay, second)
	}
	if got := repository.commits.Load(); got != commitsBeforeReplay {
		t.Fatalf("exact replay performed aggregate commit: before=%d after=%d", commitsBeforeReplay, got)
	}
	snapshot, err := inner.Load(ctx)
	if err != nil || snapshot.Revision != 3 || len(snapshot.Servers) != 1 || snapshot.Servers[0].Name != secondCommand.Name {
		t.Fatalf("latest committed Registry = %+v err=%v", snapshot, err)
	}
}

type blockingMCPRegistryRepository struct {
	repository.MCPRegistryRepository
	blockNextCommit atomic.Bool
	loads           atomic.Int32
	commits         atomic.Int32
	commitEntered   chan struct{}
	releaseCommit   chan struct{}
}

func (r *blockingMCPRegistryRepository) Load(ctx context.Context) (*mcpmodel.RegistrySnapshot, error) {
	r.loads.Add(1)
	return r.MCPRegistryRepository.Load(ctx)
}

func (r *blockingMCPRegistryRepository) Commit(ctx context.Context, expectedRevision uint64, next *mcpmodel.RegistrySnapshot) error {
	r.commits.Add(1)
	if r.blockNextCommit.CompareAndSwap(true, false) {
		close(r.commitEntered)
		<-r.releaseCommit
	}
	return r.MCPRegistryRepository.Commit(ctx, expectedRevision, next)
}

func requireMCPServerUpdateResult(t *testing.T, result magicevent.Result) mcpevents.UpdateServerResult {
	t.Helper()
	if result == nil || result.Error() != nil {
		t.Fatalf("MCP Registry update result = %+v", result)
	}
	data, err := result.Get()
	if err != nil {
		t.Fatalf("MCP Registry update result error = %v", err)
	}
	response, ok := data.(mcpevents.UpdateServerResult)
	if !ok {
		t.Fatalf("MCP Registry update result type = %T", data)
	}
	return response
}

func TestModuleStopFencesSubsequentMCPRegistryDelivery(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	hub := &mcpRegistryLifecycleHub{
		Hub:             rawHub,
		subscriptions:   map[string]int{},
		unsubscriptions: map[string]int{},
	}
	t.Cleanup(func() { rawHub.Terminate(context.Background()) })
	registry := memory.NewMCPRegistryRepository()
	module := New()
	if err := module.Configure(mcpbiz.Dependencies{Registry: registry, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	if err := module.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	for _, topic := range mcpRegistryLifecycleTopics() {
		if hub.subscriptions[topic] != 1 || hub.unsubscriptions[topic] != 1 {
			t.Fatalf("topic %s lifecycle subscriptions=%d unsubscriptions=%d, want 1/1", topic, hub.subscriptions[topic], hub.unsubscriptions[topic])
		}
	}
	command := mcpevents.CreateServerCommand{ServerID: "mcp-after-stop", Name: "after stop", Transport: "stdio", Command: "mcp-after-stop"}
	result := hub.Send(magicevent.NewEventWithContext(
		mcpevents.TopicMCPCommandCreateServer, "application/workorchd/context-registry", mcpevents.CommandDestination,
		magicevent.NewHeader(), ctx, command,
	))
	if result != nil && result.Error() == nil {
		t.Fatalf("post-Stop command unexpectedly succeeded: %+v", result)
	}
	snapshot, err := registry.Load(ctx)
	if err != nil || len(snapshot.Servers) != 0 || len(snapshot.Capabilities) != 0 {
		t.Fatalf("post-Stop command mutated owner: %+v err=%v", snapshot, err)
	}
}

type mcpRegistryLifecycleHub struct {
	magicevent.Hub
	subscriptions   map[string]int
	unsubscriptions map[string]int
}

func (h *mcpRegistryLifecycleHub) Subscribe(topic string, observer magicevent.Observer) {
	h.subscriptions[topic]++
	h.Hub.Subscribe(topic, observer)
}

func (h *mcpRegistryLifecycleHub) Unsubscribe(topic string, observer magicevent.Observer) {
	h.unsubscriptions[topic]++
	h.Hub.Unsubscribe(topic, observer)
}

func mcpRegistryLifecycleTopics() []string {
	return []string{
		mcpevents.TopicMCPCommandCreateServer,
		mcpevents.TopicMCPCommandGetServer,
		mcpevents.TopicMCPCommandListServers,
		mcpevents.TopicMCPCommandUpdateServer,
		mcpevents.TopicMCPCommandCreateCapability,
		mcpevents.TopicMCPCommandGetCapability,
		mcpevents.TopicMCPCommandListCapabilities,
		mcpevents.TopicMCPCommandUpdateCapability,
		mcpevents.TopicMCPCommandReadRegistrySnapshot,
		mcpevents.TopicMCPCommandReadSelectionSnapshot,
		mcpevents.TopicMCPCommandEnsureDefaults,
	}
}

func mustMCPPayload(t *testing.T, input map[string]any) workorcheventpayload.Payload {
	t.Helper()
	payload, err := workorcheventpayload.FromMap(input)
	if err != nil {
		t.Fatalf("workorcheventpayload.FromMap: %v", err)
	}
	return payload
}

func mcpModuleContractDependencies() mcpbiz.Dependencies {
	return mcpbiz.Dependencies{Registry: memory.NewMCPRegistryRepository(), EventHub: magicevent.NewHub(16)}
}

func sendMCPCommand[Command any, Result any](t *testing.T, ctx context.Context, hub magicevent.Hub, topic string, command Command) Result {
	t.Helper()
	var zero Result
	source := "application/workorchd/context-registry"
	switch topic {
	case mcpevents.TopicMCPCommandReadRegistrySnapshot:
		source = "blocks/mcp"
	case mcpevents.TopicMCPCommandReadSelectionSnapshot:
		source = "application/workorchd/appservice"
	}
	event := magicevent.NewEventWithContext(topic, source, mcpevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	result := hub.Send(event)
	if result == nil || result.Error() != nil {
		t.Fatalf("send %s error: %v", topic, result)
	}
	data, err := result.Get()
	if err != nil {
		t.Fatalf("result %s: %v", topic, err)
	}
	typed, ok := data.(Result)
	if !ok {
		t.Fatalf("result %s type = %T", topic, data)
		return zero
	}
	return typed
}
