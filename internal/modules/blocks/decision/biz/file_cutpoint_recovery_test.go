package decision

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	decisioncontracts "github.com/muidea/workorch/internal/modules/blocks/decision/biz/contracts"
	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	filerepo "github.com/muidea/workorch/internal/pkg/repository/file"
)

const (
	decisionCutpointHelperEnv = "WORKORCH_DECISION_CUTPOINT_HELPER"
	decisionCutpointPhaseEnv  = "WORKORCH_DECISION_CUTPOINT_PHASE"
	decisionCutpointRootEnv   = "WORKORCH_DECISION_CUTPOINT_ROOT"
)

func TestFileBackedDecisionRecoversEveryOwnerWriteCutAfterProcessKill(t *testing.T) {
	for _, phase := range []string{
		"proposal-pending",
		"proposal-resolution-created",
		"delivery-finalized",
		"delivery-evidence-recorded",
		"purge-tombstone",
		"purge-resolution-deleted",
	} {
		t.Run(phase, func(t *testing.T) {
			root := t.TempDir()
			killDecisionCutpointHelper(t, root, phase)
			assertDecisionCutpointRecovery(t, root, phase)
		})
	}
}

func TestDecisionFileCutpointHelperProcess(t *testing.T) {
	if os.Getenv(decisionCutpointHelperEnv) != "1" {
		return
	}
	root := os.Getenv(decisionCutpointRootEnv)
	phase := os.Getenv(decisionCutpointPhaseEnv)
	if root == "" || phase == "" {
		t.Fatal("missing Decision cut-point helper configuration")
	}
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-decision-file-cut")
	tickets, resolutions := decisionFileCutpointRepositories(root)
	service := newMutationService(tickets, resolutions)
	seedDecisionFileCutpointTicket(t, ctx, tickets)
	command := decisionFileCutpointCommand(t)

	switch phase {
	case "proposal-pending":
		if _, _, err := service.EnsureRuntimeGovernanceResolutionProposal(ctx, command); err != nil {
			t.Fatalf("persist pending proposal: %v", err)
		}
	case "proposal-resolution-created":
		if _, _, err := service.EnsureRuntimeGovernanceResolutionProposal(ctx, command); err != nil {
			t.Fatalf("persist pending proposal: %v", err)
		}
		if err := resolutions.Create(ctx, resolutionFromResolveCommand(command)); err != nil {
			t.Fatalf("persist formal resolution cut: %v", err)
		}
	case "delivery-finalized", "delivery-evidence-recorded", "purge-tombstone", "purge-resolution-deleted":
		ticket := finalizeDecisionFileCutpointProposal(t, ctx, service, command)
		if phase == "delivery-evidence-recorded" {
			if _, err := service.markRuntimeGovernanceDelivery(ctx, ticket, lifecycleInternalEventProposalEvidenceRecorded); err != nil {
				t.Fatalf("persist evidence delivery marker: %v", err)
			}
		}
		if phase == "purge-tombstone" || phase == "purge-resolution-deleted" {
			current, err := tickets.Get(ctx, ticket.ID)
			if err != nil {
				t.Fatalf("load ticket before purge cut: %v", err)
			}
			if _, err := service.applyLifecycleInternalEvent(ctx, current, lifecycleInternalEvent{Kind: lifecycleInternalEventAggregateDeletionRequested, TicketID: current.ID}); err != nil {
				t.Fatalf("persist purge tombstone: %v", err)
			}
			if phase == "purge-resolution-deleted" {
				if err := resolutions.DeleteByTicketID(ctx, current.ID); err != nil {
					t.Fatalf("delete resolution at purge cut: %v", err)
				}
			}
		}
	default:
		t.Fatalf("unknown Decision cut-point phase %q", phase)
	}

	// Signal only after the atomic repository call returned, then wait without
	// running owner cleanup. The parent sends SIGKILL at this exact logical cut.
	if err := os.WriteFile(filepath.Join(root, "cutpoint-ready"), []byte(phase), 0o600); err != nil {
		t.Fatalf("write cut-point readiness marker: %v", err)
	}
	select {}
}

func killDecisionCutpointHelper(t *testing.T, root string, phase string) {
	t.Helper()
	command := exec.Command(os.Args[0], "-test.run=^TestDecisionFileCutpointHelperProcess$", "-test.v")
	command.Env = append(os.Environ(),
		decisionCutpointHelperEnv+"=1",
		decisionCutpointPhaseEnv+"="+phase,
		decisionCutpointRootEnv+"="+root,
	)
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Start(); err != nil {
		t.Fatalf("start Decision cut-point helper: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	readyPath := filepath.Join(root, "cutpoint-ready")
	for {
		if _, err := os.Stat(readyPath); err == nil {
			break
		} else if !os.IsNotExist(err) {
			_ = command.Process.Kill()
			_ = command.Wait()
			t.Fatalf("stat Decision cut-point marker: %v", err)
		}
		if time.Now().After(deadline) {
			_ = command.Process.Kill()
			_ = command.Wait()
			t.Fatalf("Decision cut-point helper did not become ready:\n%s", output.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatalf("kill Decision cut-point helper: %v", err)
	}
	if err := command.Wait(); err == nil {
		t.Fatal("Decision cut-point helper exited normally; expected process kill")
	}
}

func assertDecisionCutpointRecovery(t *testing.T, root string, phase string) {
	t.Helper()
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-decision-file-cut")
	tickets, resolutions := decisionFileCutpointRepositories(root)
	service := newMutationService(tickets, resolutions)
	command := decisionFileCutpointCommand(t)

	switch phase {
	case "proposal-pending":
		ticket, proposal, err := service.EnsureRuntimeGovernanceResolutionProposal(ctx, command)
		if err != nil || ticket.State != common.DecisionTicketStateOpen || proposal == nil || proposal.State != decisionmodel.RuntimeGovernanceResolutionProposalPending {
			t.Fatalf("pending proposal replay = ticket:%+v proposal:%+v err:%v", ticket, proposal, err)
		}
		if _, _, err := service.FinalizeRuntimeGovernanceResolutionProposal(ctx, command); err != nil {
			t.Fatalf("finalize recovered pending proposal: %v", err)
		}
	case "proposal-resolution-created":
		ticket, resolution, err := service.FinalizeRuntimeGovernanceResolutionProposal(ctx, command)
		if err != nil || ticket.State != common.DecisionTicketStateResolved || resolution == nil || resolution.ID != command.ResolutionID {
			t.Fatalf("resolution-create cut recovery = ticket:%+v resolution:%+v err:%v", ticket, resolution, err)
		}
	case "delivery-finalized", "delivery-evidence-recorded":
		hub := newRecordingDecisionHub(t)
		owner, err := New(hub, nil, Dependencies{Tickets: tickets, Resolutions: resolutions, EventHub: hub})
		if err != nil {
			t.Fatalf("recreate Decision owner: %v", err)
		}
		before, err := tickets.Get(ctx, command.TicketID)
		if err != nil {
			t.Fatalf("load delivery cut ticket: %v", err)
		}
		if phase == "delivery-finalized" && (before.RuntimeGovernanceProposal.EvidenceRecordedAt != "" || before.RuntimeGovernanceProposal.ResolutionNotificationPublishedAt != "") {
			t.Fatalf("finalize cut already has delivery markers: %+v", before.RuntimeGovernanceProposal)
		}
		if phase == "delivery-evidence-recorded" && (before.RuntimeGovernanceProposal.EvidenceRecordedAt == "" || before.RuntimeGovernanceProposal.ResolutionNotificationPublishedAt != "") {
			t.Fatalf("evidence cut markers = %+v", before.RuntimeGovernanceProposal)
		}
		if err := owner.ReconcileRuntimeGovernanceProposals(context.Background()); err != nil {
			t.Fatalf("reconcile delivery cut: %v", err)
		}
		after, err := tickets.Get(ctx, command.TicketID)
		if err != nil || after.RuntimeGovernanceProposal.EvidenceRecordedAt == "" || after.RuntimeGovernanceProposal.ResolutionNotificationPublishedAt == "" {
			t.Fatalf("recovered delivery markers = %+v err=%v", after.RuntimeGovernanceProposal, err)
		}
		if got := len(hub.Posted()); got != 1 {
			t.Fatalf("resolved notifications after recovery = %d, want exactly 1", got)
		}
		if err := owner.ReconcileRuntimeGovernanceProposals(context.Background()); err != nil {
			t.Fatalf("repeat delivery reconciliation: %v", err)
		}
		if got := len(hub.Posted()); got != 1 {
			t.Fatalf("resolved notification duplicated after marker = %d", got)
		}
	case "purge-tombstone", "purge-resolution-deleted":
		hub := magicevent.NewHub(8)
		t.Cleanup(func() { hub.Terminate(context.Background()) })
		owner, err := New(hub, nil, Dependencies{Tickets: tickets, Resolutions: resolutions, EventHub: hub})
		if err != nil {
			t.Fatalf("recreate Decision owner: %v", err)
		}
		if err := owner.ReconcileRuntimeGovernanceProposals(context.Background()); err != nil {
			t.Fatalf("reconcile purge cut: %v", err)
		}
		if _, err := tickets.Get(ctx, command.TicketID); !errors.Is(err, common.ErrNotFound) {
			t.Fatalf("ticket after purge recovery error = %v, want ErrNotFound", err)
		}
		if _, err := resolutions.GetByTicketID(ctx, command.TicketID); !errors.Is(err, common.ErrNotFound) {
			t.Fatalf("resolution after purge recovery error = %v, want ErrNotFound", err)
		}
	default:
		t.Fatalf("unknown Decision cut-point phase %q", phase)
	}
}

func decisionFileCutpointRepositories(root string) (*filerepo.DecisionTicketRepository, *filerepo.DecisionResolutionRepository) {
	return filerepo.NewDecisionTicketRepository(filepath.Join(root, "ticket.json")),
		filerepo.NewDecisionResolutionRepository(filepath.Join(root, "resolution.json"))
}

func seedDecisionFileCutpointTicket(t *testing.T, ctx context.Context, tickets *filerepo.DecisionTicketRepository) {
	t.Helper()
	if err := tickets.Create(ctx, &decisionmodel.Ticket{
		ID: "ticket-decision-file-cut", WorkspaceID: "ws-decision-file-cut", RunID: "run-decision-file-cut", StepID: "runtime/dispatch-file-cut",
		Reason: "recover file-backed governance", DecisionKind: "runtime_governance", State: common.DecisionTicketStateOpen,
		AllowedRecoveryActions: []string{decisioncommon.ResolutionTypeProvideMissingInfo}, CreatedNotificationPublishedAt: "2026-08-21T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed Decision cut-point ticket: %v", err)
	}
}

func decisionFileCutpointCommand(t *testing.T) decisioncontracts.ResolveTicketCommand {
	t.Helper()
	return decisioncontracts.ResolveTicketCommand{
		ResolutionID: "resolution-decision-file-cut", TicketID: "ticket-decision-file-cut", WorkspaceID: "ws-decision-file-cut",
		RunID: "run-decision-file-cut", StepID: "runtime/dispatch-file-cut", Type: decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload: mustDecisionPayload(t, map[string]any{"summary": "continue after cut"}),
	}
}

func finalizeDecisionFileCutpointProposal(t *testing.T, ctx context.Context, service *mutationService, command decisioncontracts.ResolveTicketCommand) *decisionmodel.Ticket {
	t.Helper()
	if _, _, err := service.EnsureRuntimeGovernanceResolutionProposal(ctx, command); err != nil {
		t.Fatalf("persist pending proposal: %v", err)
	}
	ticket, _, err := service.FinalizeRuntimeGovernanceResolutionProposal(ctx, command)
	if err != nil {
		t.Fatalf("finalize proposal: %v", err)
	}
	return ticket
}
