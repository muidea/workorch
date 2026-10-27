package mcp

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestPreparationMCPRegistryReadIsCompleteBoundedAndReadOnly(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	defer hub.Terminate(ctx)
	repo := memoryrepo.NewMCPRegistryRepository()
	owner, err := New(hub, nil, Dependencies{Registry: repo, EventHub: hub})
	if err != nil {
		t.Fatal(err)
	}
	read := func() (mcpevents.RegistrySnapshotResult, error) {
		ev := magicevent.NewEventWithContext(mcpevents.TopicMCPCommandReadRegistrySnapshot, "application/workorchd", mcpevents.CommandDestination, magicevent.NewHeader(), ctx, mcpevents.ReadRegistrySnapshotCommand{})
		r := magicevent.NewResult(ev.ID(), ev.Source(), ev.Destination())
		owner.handleCommand(ev, r)
		if err := r.Error(); err != nil {
			return mcpevents.RegistrySnapshotResult{}, err
		}
		v, err := r.Get()
		if err != nil {
			return mcpevents.RegistrySnapshotResult{}, err
		}
		return v.(mcpevents.RegistrySnapshotResult), nil
	}
	empty, err := read()
	if err != nil || empty.ValidatePreparation() != nil || len(empty.Servers) != 0 || len(empty.Capabilities) != 0 {
		t.Fatal("empty read", err)
	}
	before, _ := repo.Load(ctx)
	if before.Revision != 0 {
		t.Fatal("read seeded defaults")
	}
	if _, err := owner.service.ensureBuiltinDefaults(ctx); err != nil {
		t.Fatal(err)
	}
	before, _ = repo.Load(ctx)
	value, err := read()
	if err != nil || value.ValidatePreparation() != nil || len(value.Servers) != len(before.Servers) || len(value.Capabilities) != len(before.Capabilities) {
		t.Fatal("incomplete registry", err)
	}
	clone := value.Clone()
	clone.Servers[0].Args = append(clone.Servers[0].Args, "caller")
	clone.Capabilities[0].Description = "caller"
	// Preserve exact DTO shape as well as bytes; even an explicit empty list
	// must not become nil while crossing the historical observation boundary.
	shape := value.Clone()
	shape.Capabilities[0].Tags, shape.Capabilities[0].StageHints = []string{}, []string{}
	if !reflect.DeepEqual(shape, shape.Clone()) {
		t.Fatal("clone normalized original empty lists")
	}
	for _, mutate := range []func(*mcpevents.RegistrySnapshotResult){
		func(v *mcpevents.RegistrySnapshotResult) { v.Capabilities = v.Capabilities[:len(v.Capabilities)-1] },
		func(v *mcpevents.RegistrySnapshotResult) { v.Servers[0].Name = "substituted" },
		func(v *mcpevents.RegistrySnapshotResult) { v.Servers = append(v.Servers, v.Servers[0]) },
		func(v *mcpevents.RegistrySnapshotResult) { v.Capabilities[0].ServerID = "unknown" },
		func(v *mcpevents.RegistrySnapshotResult) { v.Servers[0].Name = string([]byte{0xff}) },
		func(v *mcpevents.RegistrySnapshotResult) { v.Capabilities[0].Description = string([]byte{0xff}) },
		func(v *mcpevents.RegistrySnapshotResult) {
			v.Servers[0].EnvironmentRefs = append(v.Servers[0].EnvironmentRefs, mcpevents.EnvironmentReference{Name: " EXTRA", Reference: "env://EXTRA "})
		},
		func(v *mcpevents.RegistrySnapshotResult) {
			v.Capabilities[0].Description = strings.Repeat("x", mcpevents.PreparationSnapshotMaxBytes+1)
		},
	} {
		bad := value.Clone()
		mutate(&bad)
		if bad.ValidatePreparation() == nil {
			t.Fatal("partial/corrupt snapshot accepted")
		}
	}
	after, _ := repo.Load(ctx)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("read mutated registry")
	}
	// A valid large owner image is explicitly rejected before the preparation
	// response boundary, not silently trimmed or returned as successful empty.
	large := mcpmodel.CloneRegistrySnapshot(before)
	large.Revision = before.Revision + 1
	for i := 0; i < 80; i++ {
		c := mcpmodel.CloneCapability(large.Capabilities[0])
		c.ID = common.MCPCapabilityID(fmt.Sprintf("large-%03d", i))
		c.Name = string(c.ID)
		c.Description = strings.Repeat("x", 16000)
		large.Capabilities = append(large.Capabilities, c)
	}
	if err := repo.Commit(ctx, before.Revision, large); err != nil {
		t.Fatal(err)
	}
	if _, err := read(); err == nil {
		t.Fatal("oversized response passed preparation boundary")
	}
}
