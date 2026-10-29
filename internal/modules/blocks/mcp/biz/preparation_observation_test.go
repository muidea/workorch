package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	magicevent "github.com/muidea/magicCommon/event"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/events"
	registryevents "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	workspacecommon "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/common"
	"os"
	"reflect"
	"testing"
	"time"
)

func TestPreparationProtocolProcess(t *testing.T) {
	if os.Getenv("WORKORCH_TEST_MCP_PREPARATION") != "1" {
		return
	}
	if os.Getenv("WORKORCH_TEST_MCP_PREPARATION_MODE") == "silent" {
		time.Sleep(time.Hour)
		os.Exit(0)
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var q struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if json.Unmarshal(scanner.Bytes(), &q) != nil {
			os.Exit(2)
		}
		switch q.Method {
		case "initialize":
			fmt.Printf("{\"jsonrpc\":\"2.0\",\"id\":%d,\"result\":{\"protocolVersion\":\"2024-11-05\",\"capabilities\":{\"tools\":{}}}}\n", q.ID)
		case "notifications/initialized":
		case "tools/list":
			fmt.Printf("{\"jsonrpc\":\"2.0\",\"id\":%d,\"result\":{\"tools\":[{\"name\":\"inspect\"}]}}\n", q.ID)
		default:
			os.Exit(3) // Any business tool/read/get method fails this process.
		}
	}
	fmt.Fprint(os.Stderr, " exact diagnostic\n")
	os.Exit(0)
}
func TestPreparationMCPRealProtocolDuplicateAndBoundary(t *testing.T) {
	t.Setenv("WORKORCH_TEST_MCP_PREPARATION", "1")
	// The race runtime normally sleeps for one second at process exit, which
	// is outside the fixture protocol and exceeds the real close deadline.
	// Remove only that test-instrumentation delay, not the production bound.
	t.Setenv("GORACE", os.Getenv("GORACE")+" atexit_sleep_ms=0")
	server := &mcpmodel.Server{ID: "prepare", Name: "fixture", Enabled: true, Transport: "stdio", Command: os.Args[0], Args: []string{"-test.run=^TestPreparationProtocolProcess$"}, AllowedTools: []string{"inspect"}}
	cap := &mcpmodel.Capability{ID: "prepare:inspect", ServerID: server.ID, Kind: "tool", Name: "inspect", Enabled: true}
	wire, err := registryevents.MCPServerFromModel(server)
	if err != nil {
		t.Fatal(err)
	}
	server, err = wire.Model()
	if err != nil {
		t.Fatal(err)
	}
	getter := staticMCPServerGetter{server: server, capabilities: []*mcpmodel.Capability{cap}}
	_, revision, err := getter.RegistrySnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	c := mcpevents.ObservePreparationCommand{Version: mcpevents.PreparationObservationVersion, WorkspaceID: "ws", WorkspaceRevision: 2, RequestID: "original", PayloadHash: "payload", SnapshotHash: "snapshot", InternalDirectory: t.TempDir(), DeliverableDirectory: t.TempDir(), RegistryRevision: revision, ServerIDs: []string{string(server.ID)}, Capabilities: []mcpevents.PreparationCapability{{ID: string(cap.ID), ServerID: string(cap.ServerID), Kind: cap.Kind, Name: cap.Name}}}
	snapshot, rev, e := getter.RegistrySnapshot(context.Background())
	if e != nil || !preparationRegistryMatches(snapshot, rev, c) {
		t.Fatalf("preflight mismatch: %+v %s %+v %v", snapshot, rev, c, e)
	}
	hub := magicevent.NewHub(8)
	defer hub.Terminate(context.Background())
	owner, err := NewWithDependencies(Dependencies{EventHub: hub, RuntimeAdapter: NewRuntimeAdapter(getter)})
	if err != nil {
		t.Fatal(err)
	}
	if err = owner.Start(); err != nil {
		t.Fatal(err)
	}
	defer owner.Teardown(context.Background())
	send := func(source string, c mcpevents.ObservePreparationCommand) (mcpevents.ObservePreparationResult, error) {
		r := hub.Send(magicevent.NewEventWithContext(mcpevents.TopicObservePreparation, source, ModuleID, magicevent.NewHeader(), context.Background(), c))
		if r.Error() != nil {
			return mcpevents.ObservePreparationResult{}, r.Error()
		}
		v, e := r.Get()
		if e != nil {
			return mcpevents.ObservePreparationResult{}, e
		}
		return v.(mcpevents.ObservePreparationResult), nil
	}
	if _, err = send("workorchd", c); err == nil {
		t.Fatal("unaccepted application source allowed")
	}
	oldCommand := c
	oldCommand.Version = mcpevents.PreparationObservationVersionV1
	if _, err = send(workspacecommon.ModuleID, oldCommand); err == nil {
		t.Fatal("historical protocol accepted as a fresh observation")
	}
	first, err := send(workspacecommon.ModuleID, c)
	if err != nil || !first.Satisfied() {
		t.Fatalf("handshake failed: %+v %v", first, err)
	}
	if string(first.Observations[0].Stderr) != " exact diagnostic\n" {
		t.Fatal("raw diagnostic changed")
	}
	again, err := send(workspacecommon.ModuleID, c)
	if err != nil || !reflect.DeepEqual(first, again) {
		t.Fatal("duplicate reexecuted", err)
	}
	changed := c
	changed.PayloadHash = "other"
	if _, err = send(workspacecommon.ModuleID, changed); err == nil {
		t.Fatal("same request different payload accepted")
	}
	q := mcpevents.QueryPreparationCommand{WorkspaceID: c.WorkspaceID, RequestID: c.RequestID, PayloadHash: c.PayloadHash}
	query := hub.Send(magicevent.NewEventWithContext(mcpevents.TopicQueryPreparation, workspacecommon.ModuleID, ModuleID, magicevent.NewHeader(), context.Background(), q))
	if query == nil || query.Error() != nil {
		t.Fatal("original query unavailable")
	}
	value, queryErr := query.Get()
	qr, ok := value.(mcpevents.QueryPreparationResult)
	if queryErr != nil || !ok || qr.State != "recorded" || qr.Observation == nil || !reflect.DeepEqual(*qr.Observation, first) {
		t.Fatal("query changed original observation", queryErr)
	}
	q.RequestID = "missing"
	unknown := hub.Send(magicevent.NewEventWithContext(mcpevents.TopicQueryPreparation, workspacecommon.ModuleID, ModuleID, magicevent.NewHeader(), context.Background(), q))
	value, queryErr = unknown.Get()
	if queryErr != nil || value.(mcpevents.QueryPreparationResult).State != "unknown" {
		t.Fatal("missing original became absence/success")
	}

	// An owner recreation has no original process-local receipt. Query cannot
	// recover it by launching a new protocol observation.
	if err := owner.Teardown(context.Background()); err != nil {
		t.Fatal(err)
	}
	recreated, err := NewWithDependencies(Dependencies{EventHub: hub, RuntimeAdapter: NewRuntimeAdapter(getter)})
	if err != nil {
		t.Fatal(err)
	}
	if err := recreated.Start(); err != nil {
		t.Fatal(err)
	}
	defer recreated.Teardown(context.Background())
	q.RequestID = c.RequestID
	afterRestart := hub.Send(magicevent.NewEventWithContext(mcpevents.TopicQueryPreparation, workspacecommon.ModuleID, ModuleID, magicevent.NewHeader(), context.Background(), q))
	restartedValue, restartedErr := afterRestart.Get()
	if restartedErr != nil || restartedValue.(mcpevents.QueryPreparationResult).State != "unknown" || restartedValue.(mcpevents.QueryPreparationResult).Observation != nil || len(recreated.preparation.results) != 0 {
		t.Fatal("restart query recreated an observation")
	}

	missing := first.Clone()
	missing.Observations[0].Frames = missing.Observations[0].Frames[:1]
	if missing.Satisfied() {
		t.Fatal("missing selected capability accepted")
	}
	bad := first.Clone()
	bad.Observations[0].Frames[1].Request = []byte(`{"jsonrpc":"2.0","id":2,"method":"tools/call"}`)
	if bad.Satisfied() {
		t.Fatal("business method counted as prepare")
	}
	drift := first.Clone()
	drift.RegistryStable = false
	if drift.Satisfied() {
		t.Fatal("registry drift accepted")
	}
	if first.CompletedAt.Sub(first.StartedAt) > 3*time.Second {
		t.Fatal("fixture handshake unexpectedly slow")
	}
}

func TestPreparationMCPProtocolDeadlineClosesRealProcess(t *testing.T) {
	t.Setenv("WORKORCH_TEST_MCP_PREPARATION", "1")
	t.Setenv("WORKORCH_TEST_MCP_PREPARATION_MODE", "silent")
	t.Setenv("GORACE", os.Getenv("GORACE")+" atexit_sleep_ms=0")
	c := mcpevents.ObservePreparationCommand{InternalDirectory: t.TempDir(), DeliverableDirectory: t.TempDir()}
	s := &mcpmodel.Server{ID: "silent", Enabled: true, Transport: "stdio", Command: os.Args[0], Args: []string{"-test.run=^TestPreparationProtocolProcess$"}}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	o := observePreparationStdio(ctx, c, s)
	if time.Since(started) > 2*time.Second || !o.Closed || o.ProcessID <= 0 || o.FailureCode != "protocol_failed" {
		t.Fatalf("unbounded or successful silent process: %+v", o)
	}
	if len(o.Frames) != 1 || len(o.Frames[0].Response) != 0 {
		t.Fatal("timeout invented protocol reply")
	}
}
