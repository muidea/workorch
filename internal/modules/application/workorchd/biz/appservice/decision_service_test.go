package appservice

import (
	"context"
	"errors"
	"testing"

	legacycommon "github.com/muidea/workorch/internal/modules/blocks/legacy/pkg/common"
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
	first := prepareDecisionRun(t, dataRoot, "ws-decision-list-a", "run-"+legacycommon.RuntimeDecisionBlockRunToken+"-list-a")
	second := prepareDecisionRun(t, dataRoot, "ws-decision-list-b", "run-"+legacycommon.RuntimeDecisionBlockRunToken+"-list-b")

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

	filteredByRun, err := ListDecisionsWithBackend(context.Background(), "file", dataRoot, "", "run-"+legacycommon.RuntimeDecisionBlockRunToken+"-list-b")
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
	started := prepareDecisionRun(t, dataRoot, "ws-decision-read", "run-"+legacycommon.RuntimeDecisionBlockRunToken+"-read")
	if _, err := ResolveDecisionWithBackend(context.Background(), "file", dataRoot, started.BlockingTicketID); err != nil {
		t.Fatalf("ResolveDecisionWithBackend: %v", err)
	}

	item, err := ReadDecisionWithBackend(context.Background(), "file", dataRoot, started.BlockingTicketID)
	if err != nil {
		t.Fatalf("ReadDecisionWithBackend: %v", err)
	}
	if item.ID != started.BlockingTicketID || item.State != "resolved" {
		t.Fatalf("decision = %+v, want resolved decision", item)
	}
	if item.ResolutionType != decisioncommon.ResolutionTypeApprovePath {
		t.Fatalf("resolutionType = %q, want approve_path", item.ResolutionType)
	}
}

func TestResolveDecisionWithBackendReturnsResolvedDecision(t *testing.T) {
	dataRoot := t.TempDir()
	started := prepareDecisionRun(t, dataRoot, "ws-decision-resolve", "run-"+legacycommon.RuntimeDecisionBlockRunToken+"-resolve")

	item, err := ResolveDecisionWithBackend(context.Background(), "file", dataRoot, started.BlockingTicketID)
	if err != nil {
		t.Fatalf("ResolveDecisionWithBackend: %v", err)
	}
	if item.State != "resolved" || item.ResolutionType != decisioncommon.ResolutionTypeApprovePath {
		t.Fatalf("decision = %+v, want resolved approve_path", item)
	}
	if item.ResolutionID != "resolution-"+started.BlockingTicketID {
		t.Fatalf("resolutionID = %q, want resolution-%s", item.ResolutionID, started.BlockingTicketID)
	}
}

func TestRejectDecisionWithBackendReturnsResolvedRejectDecision(t *testing.T) {
	dataRoot := t.TempDir()
	started := prepareDecisionRun(t, dataRoot, "ws-decision-reject", "run-"+legacycommon.RuntimeDecisionBlockRunToken+"-reject")

	item, err := RejectDecisionWithBackend(context.Background(), "file", dataRoot, started.BlockingTicketID)
	if err != nil {
		t.Fatalf("RejectDecisionWithBackend: %v", err)
	}
	if item.State != "resolved" || item.ResolutionType != decisioncommon.ResolutionTypeRejectPath {
		t.Fatalf("decision = %+v, want resolved reject_path", item)
	}
	if item.ResolutionID != "resolution-"+started.BlockingTicketID+"-reject" {
		t.Fatalf("resolutionID = %q, want resolution-%s-reject", item.ResolutionID, started.BlockingTicketID)
	}
}

func TestRejectDecisionWithBackendReturnsConflictAfterApproveResolution(t *testing.T) {
	dataRoot := t.TempDir()
	started := prepareDecisionRun(t, dataRoot, "ws-decision-conflict", "run-"+legacycommon.RuntimeDecisionBlockRunToken+"-conflict")
	if _, err := ResolveDecisionWithBackend(context.Background(), "file", dataRoot, started.BlockingTicketID); err != nil {
		t.Fatalf("ResolveDecisionWithBackend: %v", err)
	}

	if _, err := RejectDecisionWithBackend(context.Background(), "file", dataRoot, started.BlockingTicketID); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("err = %v, want conflict", err)
	}
}

func prepareDecisionRun(t *testing.T, dataRoot string, workspaceID string, runID string) Run {
	t.Helper()

	if _, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, workspaceID, workspaceID+" title", "", "workspaces/"+workspaceID); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	if _, err := CreateRunWithBackend(context.Background(), "file", dataRoot, workspaceID, runID, "", ""); err != nil {
		t.Fatalf("CreateRunWithBackend: %v", err)
	}
	started, err := StartRunWithBackend(context.Background(), "file", dataRoot, runID)
	if err != nil {
		t.Fatalf("StartRunWithBackend: %v", err)
	}
	if started.State != "blocked_on_decision" || started.BlockingTicketID == "" {
		t.Fatalf("started run = %+v, want blocked decision with ticket", started)
	}
	return started
}
