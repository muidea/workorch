package appservice

import (
	"context"
	"errors"
	"testing"

	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestReadDecisionWithSessionReturnsInvalidArgumentWhenSessionMissing(t *testing.T) {
	if _, err := ReadDecisionWithSession(context.Background(), nil, "ticket-1"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
	}
}

func TestListDecisionsWithBackendReturnsFilteredTickets(t *testing.T) {
	dataRoot := t.TempDir()
	first := prepareDecisionRun(t, dataRoot, "ws-decision-list-a", "run-decision-list-a")
	second := prepareDecisionRun(t, dataRoot, "ws-decision-list-b", "run-decision-list-b")

	items, err := ListDecisionsWithBackend(context.Background(), "file", dataRoot, "", "")
	if err != nil {
		t.Fatalf("ListDecisionsWithBackend: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].ID != first.BlockingTicketID || items[1].ID != second.BlockingTicketID {
		t.Fatalf("ticket ids = %q/%q, want %q/%q", items[0].ID, items[1].ID, first.BlockingTicketID, second.BlockingTicketID)
	}

	filteredByRun, err := ListDecisionsWithBackend(context.Background(), "file", dataRoot, "", "run-decision-list-b")
	if err != nil {
		t.Fatalf("ListDecisionsWithBackend filtered by run: %v", err)
	}
	if len(filteredByRun) != 1 || filteredByRun[0].ID != second.BlockingTicketID {
		t.Fatalf("filteredByRun = %+v, want ticket %s", filteredByRun, second.BlockingTicketID)
	}

	filteredByWorkspace, err := ListDecisionsWithBackend(context.Background(), "file", dataRoot, "ws-decision-list-a", "")
	if err != nil {
		t.Fatalf("ListDecisionsWithBackend filtered by workspace: %v", err)
	}
	if len(filteredByWorkspace) != 1 || filteredByWorkspace[0].ID != first.BlockingTicketID {
		t.Fatalf("filteredByWorkspace = %+v, want ticket %s", filteredByWorkspace, first.BlockingTicketID)
	}
}

func TestReadDecisionWithBackendReturnsResolvedDecision(t *testing.T) {
	dataRoot := t.TempDir()
	started := prepareDecisionRun(t, dataRoot, "ws-decision-read", "run-decision-read")
	if _, err := ResolveDecisionWithBackend(context.Background(), "file", dataRoot, started.BlockingTicketID, ResolveDecisionCommand{
		ResolutionType: decisioncommon.ResolutionTypeProvideMissingInfo,
	}); err != nil {
		t.Fatalf("ResolveDecisionWithBackend: %v", err)
	}

	item, err := ReadDecisionWithBackend(context.Background(), "file", dataRoot, started.BlockingTicketID)
	if err != nil {
		t.Fatalf("ReadDecisionWithBackend: %v", err)
	}
	if item.ID != started.BlockingTicketID || item.State != "resolved" {
		t.Fatalf("decision = %+v, want resolved decision", item)
	}
	if item.ResolutionType != decisioncommon.ResolutionTypeProvideMissingInfo {
		t.Fatalf("resolutionType = %q, want provide_missing_info", item.ResolutionType)
	}
}

func TestResolveDecisionWithBackendReturnsResolvedDecision(t *testing.T) {
	dataRoot := t.TempDir()
	started := prepareDecisionRun(t, dataRoot, "ws-decision-resolve", "run-decision-resolve")

	item, err := ResolveDecisionWithBackend(context.Background(), "file", dataRoot, started.BlockingTicketID, ResolveDecisionCommand{
		ResolutionType:    decisioncommon.ResolutionTypeProvideMissingInfo,
		ResolutionPayload: map[string]any{"operator": "tester"},
	})
	if err != nil {
		t.Fatalf("ResolveDecisionWithBackend: %v", err)
	}
	if item.State != "resolved" || item.ResolutionType != decisioncommon.ResolutionTypeProvideMissingInfo {
		t.Fatalf("decision = %+v, want resolved provide_missing_info", item)
	}
	if item.ResolutionID != "resolution-"+started.BlockingTicketID+"-"+decisioncommon.ResolutionTypeProvideMissingInfo {
		t.Fatalf("resolutionID = %q, want resolution-%s-%s", item.ResolutionID, started.BlockingTicketID, decisioncommon.ResolutionTypeProvideMissingInfo)
	}
	if item.ResolutionPayload["operator"] != "tester" {
		t.Fatalf("resolutionPayload = %+v, want operator tester", item.ResolutionPayload)
	}
}

func TestResolveDecisionWithBackendSupportsRejectCurrentPath(t *testing.T) {
	dataRoot := t.TempDir()
	started := prepareDecisionRun(t, dataRoot, "ws-decision-reject", "run-decision-reject")

	item, err := ResolveDecisionWithBackend(context.Background(), "file", dataRoot, started.BlockingTicketID, ResolveDecisionCommand{
		ResolutionType:    decisioncommon.ResolutionTypeRejectCurrentPath,
		ResolutionPayload: map[string]any{"operator": "tester", "reason": "not viable"},
	})
	if err != nil {
		t.Fatalf("ResolveDecisionWithBackend: %v", err)
	}
	if item.State != "resolved" || item.ResolutionType != decisioncommon.ResolutionTypeRejectCurrentPath {
		t.Fatalf("decision = %+v, want resolved reject_current_path", item)
	}
	if item.ResolutionID != "resolution-"+started.BlockingTicketID+"-"+decisioncommon.ResolutionTypeRejectCurrentPath {
		t.Fatalf("resolutionID = %q, want formal reject resolution id", item.ResolutionID)
	}
	if item.ResolutionPayload["reason"] != "not viable" {
		t.Fatalf("resolutionPayload = %+v, want reject reason", item.ResolutionPayload)
	}
}

func TestResolveDecisionWithBackendReturnsConflictAfterResolution(t *testing.T) {
	dataRoot := t.TempDir()
	started := prepareDecisionRun(t, dataRoot, "ws-decision-conflict", "run-decision-conflict")
	if _, err := ResolveDecisionWithBackend(context.Background(), "file", dataRoot, started.BlockingTicketID, ResolveDecisionCommand{
		ResolutionType: decisioncommon.ResolutionTypeProvideMissingInfo,
	}); err != nil {
		t.Fatalf("ResolveDecisionWithBackend: %v", err)
	}

	if _, err := ResolveDecisionWithBackend(context.Background(), "file", dataRoot, started.BlockingTicketID, ResolveDecisionCommand{
		ResolutionType: decisioncommon.ResolutionTypeRejectCurrentPath,
	}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("err = %v, want conflict", err)
	}
}

func TestResolveDecisionWithBackendRejectsUnsupportedResolutionType(t *testing.T) {
	dataRoot := t.TempDir()
	started := prepareDecisionRun(t, dataRoot, "ws-decision-invalid", "run-decision-invalid")

	if _, err := ResolveDecisionWithBackend(context.Background(), "file", dataRoot, started.BlockingTicketID, ResolveDecisionCommand{
		ResolutionType: "unsupported_resolution",
	}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
	}
}

func TestBatchResolveDecisionsWithBackendResolvesTicketsInRequestOrder(t *testing.T) {
	dataRoot := t.TempDir()
	first := prepareDecisionRun(t, dataRoot, "ws-decision-batch-a", "run-decision-batch-a")
	second := prepareDecisionRun(t, dataRoot, "ws-decision-batch-b", "run-decision-batch-b")

	items, err := BatchResolveDecisionsWithBackend(context.Background(), "file", dataRoot, BatchResolveDecisionCommand{
		TicketIDs:         []string{second.BlockingTicketID, first.BlockingTicketID},
		ResolutionType:    decisioncommon.ResolutionTypeReplan,
		ResolutionPayload: map[string]any{"operator": "batch-tester"},
	})
	if err != nil {
		t.Fatalf("BatchResolveDecisionsWithBackend: %v", err)
	}
	if len(items) != 2 || items[0].ID != second.BlockingTicketID || items[1].ID != first.BlockingTicketID {
		t.Fatalf("items = %+v, want request order", items)
	}
	for _, item := range items {
		if item.State != "resolved" || item.ResolutionType != decisioncommon.ResolutionTypeReplan || item.ResolutionPayload["operator"] != "batch-tester" {
			t.Fatalf("item = %+v, want resolved replan with payload", item)
		}
	}
}

func TestBatchResolveDecisionsWithBackendRejectsDuplicateTicketIDs(t *testing.T) {
	dataRoot := t.TempDir()
	started := prepareDecisionRun(t, dataRoot, "ws-decision-batch-dup", "run-decision-batch-dup")

	if _, err := BatchResolveDecisionsWithBackend(context.Background(), "file", dataRoot, BatchResolveDecisionCommand{
		TicketIDs:      []string{started.BlockingTicketID, started.BlockingTicketID},
		ResolutionType: decisioncommon.ResolutionTypeProvideMissingInfo,
	}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
	}
}

func TestBatchResolveDecisionsWithBackendRejectsAlreadyResolvedTicketBeforeMutation(t *testing.T) {
	dataRoot := t.TempDir()
	first := prepareDecisionRun(t, dataRoot, "ws-decision-batch-conflict-a", "run-decision-batch-conflict-a")
	second := prepareDecisionRun(t, dataRoot, "ws-decision-batch-conflict-b", "run-decision-batch-conflict-b")
	if _, err := ResolveDecisionWithBackend(context.Background(), "file", dataRoot, second.BlockingTicketID, ResolveDecisionCommand{
		ResolutionType: decisioncommon.ResolutionTypeProvideMissingInfo,
	}); err != nil {
		t.Fatalf("ResolveDecisionWithBackend: %v", err)
	}

	if _, err := BatchResolveDecisionsWithBackend(context.Background(), "file", dataRoot, BatchResolveDecisionCommand{
		TicketIDs:      []string{first.BlockingTicketID, second.BlockingTicketID},
		ResolutionType: decisioncommon.ResolutionTypeReplan,
	}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("err = %v, want conflict", err)
	}
	firstDecision, err := ReadDecisionWithBackend(context.Background(), "file", dataRoot, first.BlockingTicketID)
	if err != nil {
		t.Fatalf("ReadDecisionWithBackend: %v", err)
	}
	if firstDecision.State != "open" {
		t.Fatalf("first decision = %+v, want still open after prevalidation conflict", firstDecision)
	}
}
