package decision

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	decisioncontracts "github.com/muidea/workorch/internal/modules/blocks/decision/biz/contracts"
	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	filerepo "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

const (
	decisionCutpointHelperEnv = "WORKORCH_DECISION_CUTPOINT_HELPER"
	decisionCutpointPhaseEnv  = "WORKORCH_DECISION_CUTPOINT_PHASE"
	decisionCutpointRootEnv   = "WORKORCH_DECISION_CUTPOINT_ROOT"
)

func TestFileBackedDecisionRecoversRuntimeGovernanceWriteCutsAfterProcessKill(t *testing.T) {
	for _, phase := range []string{
		"proposal-pending",
		"proposal-resolution-created",
		"delivery-finalized",
		"delivery-evidence-recorded",
	} {
		t.Run(phase, func(t *testing.T) {
			root := t.TempDir()
			killDecisionCutpointHelper(t, root, phase)
			assertDecisionCutpointRecovery(t, root, phase)
		})
	}
}

func TestFileBackedDecisionRecoversWorkspaceClarificationWriteCutsAfterProcessKill(t *testing.T) {
	for _, phase := range []string{
		"workspace-clarification-open-ticket-created",
		"workspace-clarification-resolution-created",
		"workspace-clarification-ticket-resolved",
		"workspace-clarification-reconcile-resolution-created",
		"workspace-clarification-successor-ticket-created",
		"workspace-clarification-successor-notified",
		"workspace-clarification-reconcile-ticket-resolved",
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
	tickets, resolutions := decisionFileCutpointRepositories(root)
	service := newMutationService(tickets, resolutions)
	if strings.HasPrefix(phase, "workspace-clarification-") {
		seedWorkspaceClarificationFileCutpoint(t, service, tickets, resolutions, phase)
	} else {
		ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-decision-file-cut")
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
		case "delivery-finalized", "delivery-evidence-recorded":
			ticket := finalizeDecisionFileCutpointProposal(t, ctx, service, command)
			if phase == "delivery-evidence-recorded" {
				if _, err := service.markRuntimeGovernanceDelivery(ctx, ticket, lifecycleInternalEventProposalEvidenceRecorded); err != nil {
					t.Fatalf("persist evidence delivery marker: %v", err)
				}
			}
		default:
			t.Fatalf("unknown Decision cut-point phase %q", phase)
		}
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
	if strings.HasPrefix(phase, "workspace-clarification-") {
		assertWorkspaceClarificationFileCutpointRecovery(t, root, phase)
		return
	}
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
		owner, err := New(hub, nil, Dependencies{OwnerRepositories: decisionBizTestOwnerRepositories(tickets, resolutions, memory.NewDecisionWorkspaceDeletionRepository()), EventHub: hub})
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
		resolution, err := resolutions.GetByTicketID(ctx, command.TicketID)
		if err != nil || resolution.ResolvedAt == "" || after.ResolvedAt != resolution.ResolvedAt {
			t.Fatalf("recovered durable resolution time ticket=%q resolution=%q err=%v", after.ResolvedAt, resolution.ResolvedAt, err)
		}
		evidenceCommands := hub.EvidenceCommands()
		wantEvidenceCommands := 1
		if phase == "delivery-evidence-recorded" {
			wantEvidenceCommands = 0
		}
		if len(evidenceCommands) != wantEvidenceCommands || (len(evidenceCommands) == 1 && evidenceCommands[0].Timestamp != resolution.ResolvedAt) {
			t.Fatalf("recovered evidence commands = %+v, want count=%d durable timestamp %q", evidenceCommands, wantEvidenceCommands, resolution.ResolvedAt)
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
		if got := len(hub.EvidenceCommands()); got != wantEvidenceCommands {
			t.Fatalf("resolved evidence duplicated after marker = %d, want %d", got, wantEvidenceCommands)
		}
	default:
		t.Fatalf("unknown Decision cut-point phase %q", phase)
	}
}

func seedWorkspaceClarificationFileCutpoint(
	t *testing.T,
	service *mutationService,
	_ *filerepo.DecisionTicketRepository,
	resolutions *filerepo.DecisionResolutionRepository,
	phase string,
) {
	t.Helper()
	open, resolve, reconcile := workspaceClarificationFileCutpointCommands()
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), open.WorkspaceID)
	oldTicket, err := service.applyLifecycleInternalEvent(ctx, nil, lifecycleInternalEvent{
		Kind: lifecycleInternalEventOpened, TicketID: open.TicketID, Ticket: workspaceDefinitionClarificationTicketForCommand(open),
	})
	if err != nil {
		t.Fatalf("persist Workspace clarification ticket cut: %v", err)
	}
	if phase == "workspace-clarification-open-ticket-created" {
		return
	}
	oldTicket, err = service.MarkTicketCreatedNotificationPublished(ctx, oldTicket)
	if err != nil {
		t.Fatalf("persist Workspace clarification creation marker: %v", err)
	}

	switch phase {
	case "workspace-clarification-resolution-created", "workspace-clarification-ticket-resolved":
		resolution := workspaceClarificationAdmissionResolution(resolve)
		if err := resolutions.Create(ctx, resolution); err != nil {
			t.Fatalf("persist Workspace clarification admission resolution cut: %v", err)
		}
		if phase == "workspace-clarification-resolution-created" {
			return
		}
		if _, err := service.applyLifecycleInternalEvent(ctx, oldTicket, lifecycleInternalEvent{
			Kind: lifecycleInternalEventResolutionObserved, TicketID: oldTicket.ID, ResolvedAt: resolution.ResolvedAt,
			Clarification: &definitionClarificationResolutionFact{
				ResolutionID: resolution.ID, ResolutionBindingHash: resolve.BindingHash(resolution.ID),
				ResolutionCandidateID: resolve.NewCandidateID, ResolutionCandidateRevision: resolve.NewCandidateRevision,
				ResolutionCandidateHash: resolve.NewCandidateHash, AdmissionResultHash: resolve.AdmissionResultHash,
				DefinitionRevision: resolve.DefinitionRevision, StartupInputHash: resolve.StartupInputHash,
			},
		}); err != nil {
			t.Fatalf("persist Workspace clarification admission ticket cut: %v", err)
		}
		return
	case "workspace-clarification-reconcile-resolution-created",
		"workspace-clarification-successor-ticket-created",
		"workspace-clarification-successor-notified",
		"workspace-clarification-reconcile-ticket-resolved":
		resolution := workspaceClarificationSupersededResolution(reconcile)
		if err := resolutions.Create(ctx, resolution); err != nil {
			t.Fatalf("persist Workspace clarification reconciliation resolution cut: %v", err)
		}
		if phase == "workspace-clarification-reconcile-resolution-created" {
			return
		}
		successor, createErr := service.applyLifecycleInternalEvent(ctx, nil, lifecycleInternalEvent{
			Kind: lifecycleInternalEventOpened, TicketID: reconcile.NewRequest.TicketID,
			Ticket: workspaceDefinitionClarificationTicketForCommand(reconcile.NewRequest),
		})
		if createErr != nil {
			t.Fatalf("persist Workspace clarification successor ticket cut: %v", createErr)
		}
		if phase == "workspace-clarification-successor-ticket-created" {
			return
		}
		if _, err := service.MarkTicketCreatedNotificationPublished(ctx, successor); err != nil {
			t.Fatalf("persist Workspace clarification successor notification cut: %v", err)
		}
		if phase == "workspace-clarification-successor-notified" {
			return
		}
		if _, err := service.applyLifecycleInternalEvent(ctx, oldTicket, lifecycleInternalEvent{
			Kind: lifecycleInternalEventResolutionObserved, TicketID: oldTicket.ID, ResolvedAt: resolution.ResolvedAt,
			Clarification: &definitionClarificationResolutionFact{
				ResolutionID: resolution.ID, ResolutionBindingHash: reconcile.BindingHash(resolution.ID),
			},
		}); err != nil {
			t.Fatalf("persist Workspace clarification reconciliation ticket cut: %v", err)
		}
		return
	default:
		t.Fatalf("unknown Workspace clarification cut-point phase %q", phase)
	}
}

func assertWorkspaceClarificationFileCutpointRecovery(t *testing.T, root, phase string) {
	t.Helper()
	open, resolve, reconcile := workspaceClarificationFileCutpointCommands()
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), open.WorkspaceID)
	tickets, resolutions := decisionFileCutpointRepositories(root)
	hub := newRecordingDecisionHub(t)
	owner, err := New(hub, nil, Dependencies{OwnerRepositories: decisionBizTestOwnerRepositories(
		tickets, resolutions, memory.NewDecisionWorkspaceDeletionRepository(),
	), EventHub: hub})
	if err != nil {
		t.Fatalf("recreate Decision owner for Workspace clarification cut: %v", err)
	}
	if phase == "workspace-clarification-reconcile-resolution-created" ||
		phase == "workspace-clarification-successor-ticket-created" ||
		phase == "workspace-clarification-successor-notified" {
		assertWorkspaceClarificationDriftFencedAtCut(t, owner, hub, tickets, resolutions, reconcile)
	}

	switch phase {
	case "workspace-clarification-open-ticket-created":
		result, replayErr := owner.openWorkspaceDefinitionClarification(ctx, open)
		if replayErr != nil || result.Disposition != decisionevents.DefinitionClarificationDuplicate {
			t.Fatalf("recover Workspace clarification open cut = %+v err=%v", result, replayErr)
		}
	case "workspace-clarification-resolution-created", "workspace-clarification-ticket-resolved":
		result, replayErr := owner.resolveWorkspaceDefinitionClarification(ctx, resolve)
		if replayErr != nil || result.ValidateFor(resolve) != nil {
			t.Fatalf("recover Workspace clarification admission cut = %+v err=%v", result, replayErr)
		}
	case "workspace-clarification-reconcile-resolution-created",
		"workspace-clarification-successor-ticket-created",
		"workspace-clarification-successor-notified",
		"workspace-clarification-reconcile-ticket-resolved":
		result, replayErr := owner.reconcileWorkspaceDefinitionClarification(ctx, reconcile)
		if replayErr != nil || result.ValidateFor(reconcile) != nil {
			t.Fatalf("recover Workspace clarification reconciliation cut = %+v err=%v", result, replayErr)
		}
	default:
		t.Fatalf("unknown Workspace clarification recovery phase %q", phase)
	}

	oldTicket, err := tickets.Get(ctx, open.TicketID)
	if err != nil || oldTicket.CreatedNotificationPublishedAt == "" {
		t.Fatalf("recovered old Workspace clarification creation marker = %+v err=%v", oldTicket, err)
	}
	if phase != "workspace-clarification-open-ticket-created" {
		if oldTicket.State != common.DecisionTicketStateResolved || oldTicket.ResolutionNotificationPublishedAt == "" || oldTicket.DefinitionClarification == nil || oldTicket.DefinitionClarification.ResolutionID == "" {
			t.Fatalf("recovered old Workspace clarification resolution = %+v", oldTicket)
		}
	}
	if strings.Contains(phase, "successor") || strings.Contains(phase, "reconcile") {
		successor, loadErr := tickets.Get(ctx, reconcile.NewRequest.TicketID)
		if loadErr != nil || successor.State != common.DecisionTicketStateOpen || successor.CreatedNotificationPublishedAt == "" {
			t.Fatalf("recovered Workspace clarification successor = %+v err=%v", successor, loadErr)
		}
	}

	postedBeforeExactReplay := len(hub.Posted())
	switch phase {
	case "workspace-clarification-open-ticket-created":
		_, err = owner.openWorkspaceDefinitionClarification(ctx, open)
	case "workspace-clarification-resolution-created", "workspace-clarification-ticket-resolved":
		_, err = owner.resolveWorkspaceDefinitionClarification(ctx, resolve)
	default:
		_, err = owner.reconcileWorkspaceDefinitionClarification(ctx, reconcile)
	}
	if err != nil {
		t.Fatalf("repeat exact Workspace clarification recovery: %v", err)
	}
	if postedAfterExactReplay := len(hub.Posted()); postedAfterExactReplay != postedBeforeExactReplay {
		t.Fatalf("exact Workspace clarification replay duplicated notification: before=%d after=%d", postedBeforeExactReplay, postedAfterExactReplay)
	}
}

func assertWorkspaceClarificationDriftFencedAtCut(
	t *testing.T,
	owner *Biz,
	hub *recordingDecisionHub,
	tickets *filerepo.DecisionTicketRepository,
	resolutions *filerepo.DecisionResolutionRepository,
	exact decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateCommand,
) {
	t.Helper()
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), exact.OldRequest.WorkspaceID)
	drift := exact
	drift.NewRequest.CandidateRevision++
	drift.NewRequest.CandidateHash = "candidate-hash-file-cut-drift"
	drift.NewRequest.CanonicalSignature = "one-of:file-cut-drift"
	drift.NewRequest.CanonicalSignatureHash = decisionevents.CanonicalDefinitionClarificationSignatureHash(
		"workspace-signature",
		drift.NewRequest.CanonicalSignatureVersion,
		drift.NewRequest.CanonicalSignature,
	)
	drift.NewRequest.TicketID = decisionevents.StableWorkspaceDefinitionClarificationTicketID(drift.NewRequest)
	drift = drift.Normalized()

	oldBefore, err := tickets.Get(ctx, exact.OldRequest.TicketID)
	if err != nil {
		t.Fatalf("load old clarification before drift replay: %v", err)
	}
	resolutionBefore, err := resolutions.GetByTicketID(ctx, exact.OldRequest.TicketID)
	if err != nil {
		t.Fatalf("load selected clarification resolution before drift replay: %v", err)
	}
	ticketsBefore, err := tickets.ListByWorkspaceID(ctx, exact.OldRequest.WorkspaceID)
	if err != nil {
		t.Fatalf("list clarification tickets before drift replay: %v", err)
	}
	postedBefore := len(hub.Posted())

	if _, err := owner.reconcileWorkspaceDefinitionClarification(ctx, drift); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("drifted successor replay error = %v, want conflict", err)
	}
	if _, err := tickets.Get(ctx, drift.NewRequest.TicketID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("drifted successor ticket lookup error = %v, want not found", err)
	}
	oldAfter, err := tickets.Get(ctx, exact.OldRequest.TicketID)
	if err != nil || !reflect.DeepEqual(oldAfter, oldBefore) {
		t.Fatalf("drifted replay mutated old clarification: before=%+v after=%+v err=%v", oldBefore, oldAfter, err)
	}
	resolutionAfter, err := resolutions.GetByTicketID(ctx, exact.OldRequest.TicketID)
	if err != nil || !reflect.DeepEqual(resolutionAfter, resolutionBefore) {
		t.Fatalf("drifted replay mutated selected resolution: before=%+v after=%+v err=%v", resolutionBefore, resolutionAfter, err)
	}
	ticketsAfter, err := tickets.ListByWorkspaceID(ctx, exact.OldRequest.WorkspaceID)
	if err != nil || len(ticketsAfter) != len(ticketsBefore) {
		t.Fatalf("drifted replay changed ticket inventory: before=%d after=%d err=%v", len(ticketsBefore), len(ticketsAfter), err)
	}
	if postedAfter := len(hub.Posted()); postedAfter != postedBefore {
		t.Fatalf("drifted replay published notification: before=%d after=%d", postedBefore, postedAfter)
	}
}

func workspaceClarificationFileCutpointCommands() (
	decisionevents.OpenWorkspaceDefinitionClarificationRequestCommand,
	decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionCommand,
	decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateCommand,
) {
	open := workspaceDefinitionClarificationCommandForBizTest().Normalized()
	resolve := decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionCommand{
		TicketID: open.TicketID, WorkspaceID: open.WorkspaceID,
		OldCandidateID: open.CandidateID, OldCandidateRevision: open.CandidateRevision, OldCandidateHash: open.CandidateHash,
		NewCandidateID: open.CandidateID, NewCandidateRevision: open.CandidateRevision + 1, NewCandidateHash: "candidate-hash-file-cut-recovered",
		AdmissionResultHash: "admission-hash-file-cut-recovered", DefinitionRevision: "definition-revision-file-cut-recovered", StartupInputHash: "startup-input-file-cut-recovered",
	}.Normalized()
	successor := open
	successor.CandidateRevision++
	successor.CandidateHash = "candidate-hash-file-cut-successor"
	successor.CanonicalSignature = "one-of:file-cut-successor"
	successor.CanonicalSignatureHash = decisionevents.CanonicalDefinitionClarificationSignatureHash("workspace-signature", successor.CanonicalSignatureVersion, successor.CanonicalSignature)
	successor.TicketID = decisionevents.StableWorkspaceDefinitionClarificationTicketID(successor)
	reconcile := decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateCommand{OldRequest: open, NewRequest: successor}.Normalized()
	return open, resolve, reconcile
}

func workspaceClarificationAdmissionResolution(command decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionCommand) *decisionmodel.Resolution {
	resolutionID := decisionevents.StableDefinitionClarificationResolutionID(command.TicketID, command.AdmissionResultHash+"\x00"+command.StartupInputHash)
	return &decisionmodel.Resolution{
		ID: resolutionID, TicketID: command.TicketID, ResolvedAt: newDecisionResolutionTimestamp(),
		Type: decisionevents.DefinitionClarificationResolutionCandidateAdjusted,
		Payload: workorcheventpayload.New(
			workorcheventpayload.StringField("definitionRevision", command.DefinitionRevision),
			workorcheventpayload.StringField("startupInputHash", command.StartupInputHash),
		),
	}
}

func workspaceClarificationSupersededResolution(command decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateCommand) *decisionmodel.Resolution {
	resolutionID := decisionevents.StableDefinitionClarificationResolutionID(command.OldRequest.TicketID, command.NewRequest.BindingHash())
	return &decisionmodel.Resolution{
		ID: resolutionID, TicketID: command.OldRequest.TicketID, ResolvedAt: newDecisionResolutionTimestamp(),
		Type: decisionevents.DefinitionClarificationResolutionCandidateSuperseded,
		Payload: workorcheventpayload.New(
			workorcheventpayload.StringField("newTicketId", string(command.NewRequest.TicketID)),
			workorcheventpayload.StringField("newCandidateHash", command.NewRequest.CandidateHash),
		),
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
