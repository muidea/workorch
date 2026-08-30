package mcp

import (
	"context"
	"errors"
	"strings"
	"testing"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/common"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestServiceUsesAtomicRegistryAndReferenceOnlyEnvironment(t *testing.T) {
	ctx := context.Background()
	repository := memory.NewMCPRegistryRepository()
	service := newService(repository)

	server, revision, err := service.createServer(ctx, mcpevents.CreateServerCommand{
		ServerID: "mcp-local", Name: "local", Transport: "stdio", Command: "mcp-local", Enabled: true,
		EnvironmentRefs: []mcpevents.EnvironmentReference{{Name: "TOKEN", Reference: "env://WORKORCH_MCP_TOKEN"}},
		AllowedTools:    []string{"repo.summary"}, AllowedResources: []string{"repo://summary"},
	})
	if err != nil {
		t.Fatalf("CreateServer: %v", err)
	}
	if revision == "" || server.Env["TOKEN"] != "env://WORKORCH_MCP_TOKEN" {
		t.Fatalf("created server/revision = %+v/%q", server, revision)
	}
	snapshot, snapshotRevision, err := service.registrySnapshot(ctx)
	if err != nil {
		t.Fatalf("RegistrySnapshot: %v", err)
	}
	if snapshotRevision != revision || len(snapshot.Servers) != 1 || len(snapshot.Capabilities) != 2 {
		t.Fatalf("snapshot = %+v revision=%q, want one atomic server and two capabilities", snapshot, snapshotRevision)
	}

	_, _, err = service.createServer(ctx, mcpevents.CreateServerCommand{
		ServerID: "mcp-secret", Name: "secret", Transport: "stdio", Command: "mcp-secret",
		EnvironmentRefs: []mcpevents.EnvironmentReference{{Name: "TOKEN", Reference: "plaintext-secret"}},
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("plaintext CreateServer err = %v, want invalid argument", err)
	}
	after, _, _ := service.registrySnapshot(ctx)
	if len(after.Servers) != 1 {
		t.Fatalf("rejected secret changed registry: %+v", after.Servers)
	}
}

func TestServiceCapabilityIdentityAvailabilityAndProvenance(t *testing.T) {
	ctx := context.Background()
	service := newService(memory.NewMCPRegistryRepository())
	_, _, err := service.createServer(ctx, mcpevents.CreateServerCommand{
		ServerID: "mcp-capability", Name: "capability", Transport: "stdio", Command: "mcp-capability", Enabled: true,
		AllowedResources: []string{"workspace://acceptance"},
	})
	if err != nil {
		t.Fatalf("CreateServer: %v", err)
	}
	created, _, err := service.createCapability(ctx, mcpevents.CreateCapabilityCommand{
		CapabilityID: "cap-acceptance", ServerID: "mcp-capability", Kind: mcpmodel.CapabilityKindResource,
		Name: "workspace://acceptance", DisplayName: "Acceptance", Category: "verification", Enabled: true,
		EffectScope: "none", IdempotencyClass: "safe_to_replay", ValidationProofApplicability: "auxiliary",
	})
	if err != nil {
		t.Fatalf("CreateCapability: %v", err)
	}
	if created.Source != mcpmodel.CapabilitySourceUser || created.SourceRef != "cap-acceptance" || created.Revision != 1 {
		t.Fatalf("provenance = %+v", created)
	}
	_, _, err = service.createServer(ctx, mcpevents.CreateServerCommand{
		ServerID: "mcp-other", Name: "other", Transport: "stdio", Command: "mcp-other", Enabled: true,
		AllowedResources: []string{"workspace://acceptance"},
	})
	if err != nil {
		t.Fatalf("CreateServer(other): %v", err)
	}
	updated, _, err := service.updateCapability(ctx, mcpevents.UpdateCapabilityCommand(mcpevents.CreateCapabilityCommand{
		CapabilityID: created.ID, ServerID: created.ServerID, Kind: created.Kind, Name: created.Name,
		DisplayName: "Acceptance v2", Category: created.Category, Enabled: true,
		EffectScope: created.EffectScope, IdempotencyClass: created.IdempotencyClass,
		ValidationProofApplicability: created.ValidationProofApplicability,
	}))
	if err != nil || updated.Revision != 2 || updated.Source != mcpmodel.CapabilitySourceUser {
		t.Fatalf("UpdateCapability = %+v, err=%v", updated, err)
	}

	_, _, err = service.updateCapability(ctx, mcpevents.UpdateCapabilityCommand(mcpevents.CreateCapabilityCommand{
		CapabilityID: created.ID, ServerID: "mcp-other", Kind: created.Kind, Name: created.Name,
		DisplayName: "rebound", Enabled: true, EffectScope: "none", IdempotencyClass: "safe_to_replay",
		ValidationProofApplicability: "auxiliary",
	}))
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("identity rebind err = %v, want conflict", err)
	}
	_, _, err = service.updateCapability(ctx, mcpevents.UpdateCapabilityCommand(mcpevents.CreateCapabilityCommand{
		CapabilityID: created.ID, ServerID: created.ServerID, Kind: created.Kind, Name: created.Name,
		DisplayName: "invalid", Enabled: true, EffectScope: "made_up", IdempotencyClass: "sometimes",
		ValidationProofApplicability: "auxiliary",
	}))
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("invalid metadata err = %v, want invalid argument", err)
	}

	_, _, err = service.createServer(ctx, mcpevents.CreateServerCommand{
		ServerID: "mcp-disabled", Name: "disabled", Transport: "stdio", Command: "mcp-disabled", Enabled: false,
		AllowedTools: []string{"tool.disabled"},
	})
	if err != nil {
		t.Fatalf("CreateServer(disabled): %v", err)
	}
	_, _, err = service.createCapability(ctx, mcpevents.CreateCapabilityCommand{
		CapabilityID: "cap-disabled", ServerID: "mcp-disabled", Kind: mcpmodel.CapabilityKindTool,
		Name: "tool.disabled", DisplayName: "disabled", Enabled: true,
	})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("enabled capability on disabled server err = %v, want conflict", err)
	}
}

func TestServiceServerReconciliationPreservesEditsAndDisablesStaleBindings(t *testing.T) {
	ctx := context.Background()
	service := newService(memory.NewMCPRegistryRepository())
	_, _, err := service.createServer(ctx, mcpevents.CreateServerCommand{
		ServerID: "mcp-tools", Name: "tools", Transport: "stdio", Command: "mcp-tools", Enabled: true,
		AllowedTools: []string{"tool.a", "tool.b"},
	})
	if err != nil {
		t.Fatalf("CreateServer: %v", err)
	}
	aID := DefaultCapabilityID("mcp-tools", mcpmodel.CapabilityKindTool, "tool.a")
	a, _, err := service.getCapability(ctx, aID)
	if err != nil {
		t.Fatalf("GetCapability(a): %v", err)
	}
	a, _, err = service.updateCapability(ctx, mcpevents.UpdateCapabilityCommand(mcpevents.CreateCapabilityCommand{
		CapabilityID: a.ID, ServerID: a.ServerID, Kind: a.Kind, Name: a.Name, DisplayName: "edited A",
		Enabled: false, EffectScope: "none", IdempotencyClass: "safe_to_replay",
		ValidationProofApplicability: "direct",
	}))
	if err != nil {
		t.Fatalf("UpdateCapability(a): %v", err)
	}
	_, _, err = service.updateServer(ctx, mcpevents.UpdateServerCommand(mcpevents.CreateServerCommand{
		ServerID: "mcp-tools", Name: "tools", Transport: "stdio", Command: "mcp-tools", Enabled: true,
		AllowedTools: []string{"tool.a", "tool.c"},
	}))
	if err != nil {
		t.Fatalf("UpdateServer: %v", err)
	}
	aAfter, _, _ := service.getCapability(ctx, aID)
	bAfter, _, _ := service.getCapability(ctx, DefaultCapabilityID("mcp-tools", mcpmodel.CapabilityKindTool, "tool.b"))
	cAfter, _, _ := service.getCapability(ctx, DefaultCapabilityID("mcp-tools", mcpmodel.CapabilityKindTool, "tool.c"))
	if aAfter.DisplayName != "edited A" || aAfter.Enabled || aAfter.Revision != a.Revision {
		t.Fatalf("edited capability was overwritten: %+v", aAfter)
	}
	if bAfter.Enabled {
		t.Fatalf("removed capability remains enabled: %+v", bAfter)
	}
	if cAfter == nil || !cAfter.Enabled || cAfter.Source != mcpmodel.CapabilitySourceServerAllowlist {
		t.Fatalf("new derived capability = %+v", cAfter)
	}
}

func TestServiceSelectionSnapshotFailsClosedOnServerRelationship(t *testing.T) {
	ctx := context.Background()
	service := newService(memory.NewMCPRegistryRepository())
	_, _, err := service.createServer(ctx, mcpevents.CreateServerCommand{
		ServerID: "mcp-select", Name: "select", Transport: "stdio", Command: "mcp-select", Enabled: true,
		AllowedTools: []string{"tool.a"},
	})
	if err != nil {
		t.Fatalf("CreateServer: %v", err)
	}
	capID := DefaultCapabilityID("mcp-select", mcpmodel.CapabilityKindTool, "tool.a")
	if _, _, err := service.selectionSnapshot(ctx, nil, []common.MCPCapabilityID{capID}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("capability without selected server err = %v, want conflict", err)
	}
	snapshot, revision, err := service.selectionSnapshot(ctx, []common.MCPServerID{"mcp-select"}, []common.MCPCapabilityID{capID})
	if err != nil || revision == "" || len(snapshot.Servers) != 1 || len(snapshot.Capabilities) != 1 {
		t.Fatalf("SelectionSnapshot = %+v/%q, err=%v", snapshot, revision, err)
	}
}

func TestServiceListsArePaginatedAndRejectInvalidBounds(t *testing.T) {
	ctx := context.Background()
	service := newService(memory.NewMCPRegistryRepository())
	for _, id := range []common.MCPServerID{"mcp-a", "mcp-b", "mcp-c"} {
		_, _, err := service.createServer(ctx, mcpevents.CreateServerCommand{
			ServerID: id, Name: string(id), Transport: "stdio", Command: string(id), Enabled: true,
			AllowedTools: []string{"tool.a"},
		})
		if err != nil {
			t.Fatalf("CreateServer(%s): %v", id, err)
		}
	}
	servers, cursor, revision, err := service.listServers(ctx, "", 2)
	if err != nil || len(servers) != 2 || cursor != "mcp-b" || revision == "" {
		t.Fatalf("first server page = %+v/%q/%q, err=%v", servers, cursor, revision, err)
	}
	servers, next, sameRevision, err := service.listServers(ctx, cursor, 2)
	if err != nil || len(servers) != 1 || servers[0].ID != "mcp-c" || next != "" || sameRevision != revision {
		t.Fatalf("second server page = %+v/%q/%q, err=%v", servers, next, sameRevision, err)
	}
	capabilities, capabilityCursor, _, err := service.listCapabilities(ctx, mcpevents.CapabilityQuery{Kind: mcpmodel.CapabilityKindTool}, "", 2)
	if err != nil || len(capabilities) != 2 || capabilityCursor == "" {
		t.Fatalf("capability page = %+v/%q, err=%v", capabilities, capabilityCursor, err)
	}

	if _, _, _, err := service.listServers(ctx, "", modulecommon.MaxListLimit+1); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("oversized server limit err = %v", err)
	}
	if _, _, _, err := service.listCapabilities(ctx, mcpevents.CapabilityQuery{Kind: "made_up"}, "", 1); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("invalid capability kind err = %v", err)
	}
	if _, _, _, err := service.listCapabilities(ctx, mcpevents.CapabilityQuery{Query: string(make([]byte, modulecommon.MaxQueryBytes+1))}, "", 1); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("oversized capability query err = %v", err)
	}
}

func TestServiceRejectsInvalidLookupAndUpdateCapabilityIDs(t *testing.T) {
	ctx := context.Background()
	service := newService(memory.NewMCPRegistryRepository())
	for name, value := range map[string]string{
		"non-canonical": " mcp-id ",
		"invalid UTF-8": string([]byte{0xff}),
		"oversized":     strings.Repeat("x", modulecommon.MaxIDBytes+1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := service.getServer(ctx, common.MCPServerID(value)); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("get server with invalid ID error = %v, want invalid argument", err)
			}
			if _, _, err := service.getCapability(ctx, common.MCPCapabilityID(value)); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("get capability with invalid ID error = %v, want invalid argument", err)
			}
			command := mcpevents.UpdateCapabilityCommand(mcpevents.CreateCapabilityCommand{CapabilityID: common.MCPCapabilityID(value)})
			if _, _, err := service.updateCapability(ctx, command); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("update capability with invalid ID error = %v, want invalid argument", err)
			}
		})
	}
}

func TestServiceCommitFailureLeavesNoPartialAggregate(t *testing.T) {
	ctx := context.Background()
	repository := &failingRegistryRepository{fail: true, snapshot: &mcpmodel.RegistrySnapshot{}}
	service := newService(repository)
	command := mcpevents.CreateServerCommand{
		ServerID: "mcp-atomic", Name: "atomic", Transport: "stdio", Command: "mcp-atomic", Enabled: true,
		AllowedTools: []string{"tool.a"},
	}
	if _, _, err := service.createServer(ctx, command); err == nil {
		t.Fatal("CreateServer unexpectedly succeeded")
	}
	if len(repository.snapshot.Servers) != 0 || len(repository.snapshot.Capabilities) != 0 {
		t.Fatalf("partial aggregate after failure: %+v", repository.snapshot)
	}
	repository.fail = false
	if _, _, err := service.createServer(ctx, command); err != nil {
		t.Fatalf("retry CreateServer: %v", err)
	}
	if len(repository.snapshot.Servers) != 1 || len(repository.snapshot.Capabilities) != 1 {
		t.Fatalf("atomic retry snapshot: %+v", repository.snapshot)
	}
}

type failingRegistryRepository struct {
	fail     bool
	snapshot *mcpmodel.RegistrySnapshot
}

func (r *failingRegistryRepository) Load(context.Context) (*mcpmodel.RegistrySnapshot, error) {
	return mcpmodel.CloneRegistrySnapshot(r.snapshot), nil
}

func (r *failingRegistryRepository) Commit(_ context.Context, expected uint64, next *mcpmodel.RegistrySnapshot) error {
	if r.fail {
		return errors.New("injected commit failure")
	}
	if r.snapshot.Revision != expected {
		return common.ErrConflict
	}
	r.snapshot = mcpmodel.CloneRegistrySnapshot(next)
	return nil
}
