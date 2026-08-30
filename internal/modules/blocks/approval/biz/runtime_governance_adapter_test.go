package approval

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func startApprovalBizForGovernanceTest(t *testing.T, hub magicevent.Hub, approvals *memory.ApprovalRepository) *Biz {
	t.Helper()
	owner, err := New(hub, nil, Dependencies{OwnerRepositories: approvalBizTestOwnerRepositories(approvals, memory.NewApprovalWorkspaceDeletionRepository()), EventHub: hub})
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(context.Background()) })
	return owner
}

func approvalCommandError(hub magicevent.Hub, source, topic string, command any) error {
	result := hub.Send(magicevent.NewEventWithContext(topic, source, approvalevents.CommandDestination, magicevent.NewHeader(), context.Background(), command))
	if result == nil {
		return common.ErrInvalidArgument
	}
	if resultErr := result.Error(); resultErr != nil {
		if resultErr.Cause != nil {
			return resultErr.Cause
		}
		return resultErr
	}
	_, resultErr := result.Get()
	if resultErr == nil {
		return nil
	}
	if resultErr.Cause != nil {
		return resultErr.Cause
	}
	return resultErr
}

func subscribeApprovalDeliveryResults(t *testing.T, hub magicevent.Hub, evidenceValid *atomic.Bool, resolutionValid *atomic.Bool) {
	t.Helper()
	observer := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	observer.Subscribe(orchestratorevents.TopicOrchestratorCommandRecordApprovalEvidence, func(event magicevent.Event, result magicevent.Result) {
		command, ok := event.Data().(orchestratorevents.RecordApprovalEvidenceCommand)
		if !ok || !evidenceValid.Load() {
			result.Set(struct{}{}, nil)
			return
		}
		result.Set(orchestratorevents.RecordApprovalEvidenceResult{
			EvidenceRecordID: command.EvidenceRecordID,
			WorkspaceID:      command.WorkspaceID,
			RunID:            command.RunID,
			ApprovalID:       command.ApprovalID,
			ResolutionID:     command.ResolutionID,
			ApprovalState:    command.ApprovalState,
		}, nil)
	})
	t.Cleanup(func() { observer.Unsubscribe(orchestratorevents.TopicOrchestratorCommandRecordApprovalEvidence) })
	resolutionObserver := magicevent.NewSimpleObserver(eventstreamevents.CommandDestination, hub)
	resolutionObserver.Subscribe(eventstreamevents.TopicEventStreamCommandRecordApprovalResolution, func(event magicevent.Event, result magicevent.Result) {
		command, ok := event.Data().(eventstreamevents.RecordApprovalResolutionCommand)
		if !ok || !resolutionValid.Load() {
			result.Set(struct{}{}, nil)
			return
		}
		data := command.Data
		result.Set(eventstreamevents.RecordApprovalResolutionResult{
			NotificationID: data.NotificationID, ResolutionID: data.ResolutionID, ApprovalID: data.ApprovalID,
			WorkspaceID: data.WorkspaceID, RunID: data.RunID,
		}, nil)
	})
	t.Cleanup(func() {
		resolutionObserver.Unsubscribe(eventstreamevents.TopicEventStreamCommandRecordApprovalResolution)
	})
}

func humanApprovalResolution(approvalID common.ApprovalID, workspaceID common.WorkspaceID, runID common.RunID) approvalevents.EnsureApprovedCommand {
	return approvalevents.EnsureApprovedCommand{
		ApprovalID: approvalID, WorkspaceID: workspaceID, RunID: runID,
		Principal: approvalmodel.ApprovalResolutionPrincipal{
			Kind: approvalmodel.ApprovalResolutionPrincipalHumanOperator, SubjectID: approvalApplicationSource,
		},
		ResolutionContext: workorcheventpayload.New(workorcheventpayload.StringField("operatorNote", "approved after review")).Fields,
	}
}

func TestApprovalResolutionIngressRequiresTrustedSourceAndPrincipal(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	repo := memory.NewApprovalRepository()
	startApprovalBizForGovernanceTest(t, hub, repo)
	var evidenceValid atomic.Bool
	evidenceValid.Store(true)
	var resolutionValid atomic.Bool
	resolutionValid.Store(true)
	subscribeApprovalDeliveryResults(t, hub, &evidenceValid, &resolutionValid)

	const (
		workspaceID = common.WorkspaceID("ws-trusted-resolution")
		runID       = common.RunID("run-trusted-resolution")
		approvalID  = common.ApprovalID("approval-trusted-resolution")
	)
	if err := repo.Create(ctx, &approvalmodel.Approval{
		ID: approvalID, WorkspaceID: workspaceID, RunID: runID, Reason: "operator authorization required",
		Context: workorcheventpayload.New(workorcheventpayload.StringField("targetRef", "service-a")), State: common.ApprovalStatePending,
	}); err != nil {
		t.Fatal(err)
	}
	command := humanApprovalResolution(approvalID, workspaceID, runID)
	if err := approvalCommandError(hub, "untrusted/module", approvalevents.TopicApprovalCommandEnsureApproved, command); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("untrusted source err = %v, want invalid argument", err)
	}
	spoofed := command
	spoofed.Principal.SubjectID = "operator-from-payload"
	if err := approvalCommandError(hub, approvalApplicationSource, approvalevents.TopicApprovalCommandEnsureApproved, spoofed); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("source/principal mismatch err = %v, want invalid argument", err)
	}
	if item, err := repo.GetScoped(ctx, workspaceID, runID, approvalID); err != nil || item.State != common.ApprovalStatePending {
		t.Fatalf("approval after rejected ingress = %+v err=%v, want pending", item, err)
	}
	if err := approvalCommandError(hub, approvalApplicationSource, approvalevents.TopicApprovalCommandEnsureApproved, command); err != nil {
		t.Fatalf("trusted resolution: %v", err)
	}
	item, err := repo.GetScoped(ctx, workspaceID, runID, approvalID)
	if err != nil {
		t.Fatal(err)
	}
	if item.State != common.ApprovalStateApproved || item.ResolutionPrincipal != command.Principal {
		t.Fatalf("resolved approval = %+v, want trusted principal", item)
	}
	target, _ := item.Context.String("targetRef")
	if target != "service-a" {
		t.Fatalf("immutable authorization target = %q, want service-a", target)
	}
}

func TestApprovalResolutionIngressCoversPrincipalAndTerminalMatrix(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(64)
	t.Cleanup(func() { hub.Terminate(ctx) })
	repo := memory.NewApprovalRepository()
	startApprovalBizForGovernanceTest(t, hub, repo)
	var evidenceValid atomic.Bool
	evidenceValid.Store(true)
	var resolutionValid atomic.Bool
	resolutionValid.Store(true)
	subscribeApprovalDeliveryResults(t, hub, &evidenceValid, &resolutionValid)

	tests := []struct {
		name      string
		source    string
		principal approvalmodel.ApprovalResolutionPrincipal
		topic     string
		wantState common.ApprovalState
	}{
		{
			name: "application human approved", source: approvalApplicationSource,
			principal: approvalmodel.ApprovalResolutionPrincipal{Kind: approvalmodel.ApprovalResolutionPrincipalHumanOperator, SubjectID: approvalApplicationSource},
			topic:     approvalevents.TopicApprovalCommandEnsureApproved, wantState: common.ApprovalStateApproved,
		},
		{
			name: "application human rejected", source: approvalApplicationSource,
			principal: approvalmodel.ApprovalResolutionPrincipal{Kind: approvalmodel.ApprovalResolutionPrincipalHumanOperator, SubjectID: approvalApplicationSource},
			topic:     approvalevents.TopicApprovalCommandEnsureRejected, wantState: common.ApprovalStateRejected,
		},
		{
			name: "governance auto approved", source: approvalGovernanceSource,
			principal: approvalmodel.ApprovalResolutionPrincipal{Kind: approvalmodel.ApprovalResolutionPrincipalAutoGovernance, SubjectID: approvalGovernanceSource},
			topic:     approvalevents.TopicApprovalCommandEnsureApproved, wantState: common.ApprovalStateApproved,
		},
		{
			name: "governance auto rejected", source: approvalGovernanceSource,
			principal: approvalmodel.ApprovalResolutionPrincipal{Kind: approvalmodel.ApprovalResolutionPrincipalAutoGovernance, SubjectID: approvalGovernanceSource},
			topic:     approvalevents.TopicApprovalCommandEnsureRejected, wantState: common.ApprovalStateRejected,
		},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			suffix := string(rune('a' + index))
			workspaceID := common.WorkspaceID("ws-principal-matrix-" + suffix)
			runID := common.RunID("run-principal-matrix-" + suffix)
			approvalID := common.ApprovalID("approval-principal-matrix-" + suffix)
			if err := repo.Create(ctx, &approvalmodel.Approval{
				ID: approvalID, WorkspaceID: workspaceID, RunID: runID,
				Reason: "principal matrix", State: common.ApprovalStatePending,
			}); err != nil {
				t.Fatal(err)
			}
			base := approvalevents.ResolveApprovalCommand{
				ApprovalID: approvalID, WorkspaceID: workspaceID, RunID: runID, Principal: test.principal,
				ResolutionContext: workorcheventpayload.New(workorcheventpayload.StringField("summary", test.name)).Fields,
			}
			var command any
			switch test.topic {
			case approvalevents.TopicApprovalCommandEnsureApproved:
				command = approvalevents.EnsureApprovedCommand(base)
			case approvalevents.TopicApprovalCommandEnsureRejected:
				command = approvalevents.EnsureRejectedCommand(base)
			default:
				t.Fatalf("unknown topic %q", test.topic)
			}
			if err := approvalCommandError(hub, test.source, test.topic, command); err != nil {
				t.Fatalf("trusted resolution: %v", err)
			}
			item, err := repo.GetScoped(ctx, workspaceID, runID, approvalID)
			if err != nil || item.State != test.wantState || item.ResolutionPrincipal != test.principal {
				t.Fatalf("resolved approval = %+v err=%v", item, err)
			}
		})
	}

	workspaceID := common.WorkspaceID("ws-forged-auto-principal")
	runID := common.RunID("run-forged-auto-principal")
	approvalID := common.ApprovalID("approval-forged-auto-principal")
	if err := repo.Create(ctx, &approvalmodel.Approval{
		ID: approvalID, WorkspaceID: workspaceID, RunID: runID,
		Reason: "forged auto principal", State: common.ApprovalStatePending,
	}); err != nil {
		t.Fatal(err)
	}
	forged := approvalevents.EnsureRejectedCommand{
		ApprovalID: approvalID, WorkspaceID: workspaceID, RunID: runID,
		Principal: approvalmodel.ApprovalResolutionPrincipal{
			Kind: approvalmodel.ApprovalResolutionPrincipalHumanOperator, SubjectID: approvalGovernanceSource,
		},
	}
	if err := approvalCommandError(hub, approvalGovernanceSource, approvalevents.TopicApprovalCommandEnsureRejected, forged); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("forged governance principal err = %v, want invalid argument", err)
	}
	item, err := repo.GetScoped(ctx, workspaceID, runID, approvalID)
	if err != nil || item.State != common.ApprovalStatePending || item.ResolutionID != "" {
		t.Fatalf("forged governance principal mutated approval = %+v err=%v", item, err)
	}
}

func TestRuntimeApprovalRequestRequiresOrchestratorAndExactReplay(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	repo := memory.NewApprovalRepository()
	startApprovalBizForGovernanceTest(t, hub, repo)

	command := approvalevents.OpenRuntimeApprovalRequestCommand{
		ApprovalID: "approval-runtime-open", WorkspaceID: "ws-runtime-open", RunID: "run-runtime-open",
		DispatchID: "dispatch-runtime-open", AuthoritySnapshotID: "authority-runtime-open",
		DependencyKind: string(runtimecontract.GovernanceApproval), SubjectRef: "https://example.test/resource",
		Reason: "network access requires authorization", RequestedAction: "web/fetch", RiskLevel: "high",
		AuthorizationContext: workorcheventpayload.New(
			workorcheventpayload.StringField("providerId", "provider-a"),
			workorcheventpayload.StringField("targetRef", "https://example.test/resource"),
		).Fields,
	}
	if err := approvalCommandError(hub, "untrusted/module", approvalevents.TopicOpenRuntimeApprovalRequest, command); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("untrusted open err = %v, want invalid argument", err)
	}
	if err := approvalCommandError(hub, runtimecontract.RuntimeModuleOrchestrator, approvalevents.TopicOpenRuntimeApprovalRequest, command); err != nil {
		t.Fatalf("open runtime approval: %v", err)
	}
	if err := approvalCommandError(hub, runtimecontract.RuntimeModuleOrchestrator, approvalevents.TopicOpenRuntimeApprovalRequest, command); err != nil {
		t.Fatalf("exact open replay: %v", err)
	}
	changed := command
	changed.AuthorizationContext = workorcheventpayload.New(
		workorcheventpayload.StringField("providerId", "provider-b"),
		workorcheventpayload.StringField("targetRef", "https://example.test/resource"),
	).Fields
	if err := approvalCommandError(hub, runtimecontract.RuntimeModuleOrchestrator, approvalevents.TopicOpenRuntimeApprovalRequest, changed); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("changed immutable request replay err = %v, want conflict", err)
	}
	item, err := repo.GetScoped(ctx, command.WorkspaceID, command.RunID, command.ApprovalID)
	if err != nil {
		t.Fatal(err)
	}
	provider, _ := item.Context.String("providerId")
	if provider != "provider-a" || item.RuntimeGovernance == nil || item.RuntimeGovernance.RequestedAction != "web/fetch" {
		t.Fatalf("persisted runtime approval = %+v, want original sealed authorization", item)
	}
}

func TestRuntimeApprovalProposalRecoversAfterInvalidCommitResult(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(64)
	hub := &recordingApprovalHub{Hub: rawHub}
	t.Cleanup(func() { rawHub.Terminate(ctx) })
	repo := memory.NewApprovalRepository()
	owner := startApprovalBizForGovernanceTest(t, hub, repo)
	var evidenceValid atomic.Bool
	evidenceValid.Store(true)
	var resolutionValid atomic.Bool
	resolutionValid.Store(true)
	subscribeApprovalDeliveryResults(t, rawHub, &evidenceValid, &resolutionValid)

	var validCommit atomic.Bool
	var commitCount atomic.Int64
	commitObserver := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, rawHub)
	commitObserver.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceApproval, func(event magicevent.Event, result magicevent.Result) {
		commitCount.Add(1)
		command, ok := event.Data().(orchestratorevents.CommitRuntimeGovernanceApprovalResolutionCommand)
		if !ok || !validCommit.Load() {
			result.Set(struct{}{}, nil)
			return
		}
		result.Set(orchestratorevents.CommitRuntimeGovernanceResolutionResult{
			WorkspaceID: command.WorkspaceID, RunID: command.RunID, ResolutionID: command.ResolutionID,
			Disposition:   orchestratorevents.RuntimeGovernanceResolutionDispositionCommitted,
			StateRevision: 2, EvidenceRevision: "evidence-2", AuthoritySnapshotID: "authority-runtime-recovery",
			State: runtimecontract.ConvergenceStateNeedStateChange,
		}, nil)
	})
	t.Cleanup(func() {
		commitObserver.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceApproval)
	})

	open := approvalevents.OpenRuntimeApprovalRequestCommand{
		ApprovalID: "approval-runtime-recovery", WorkspaceID: "ws-runtime-recovery", RunID: "run-runtime-recovery",
		DispatchID: "dispatch-runtime-recovery", AuthoritySnapshotID: "authority-runtime-recovery",
		DependencyKind: string(runtimecontract.GovernanceApproval), SubjectRef: "secret.txt", Reason: "sensitive write",
		RequestedAction: "file/write", RiskLevel: "high",
		AuthorizationContext: workorcheventpayload.New(workorcheventpayload.StringField("targetRef", "secret.txt")).Fields,
	}
	if err := approvalCommandError(rawHub, runtimecontract.RuntimeModuleOrchestrator, approvalevents.TopicOpenRuntimeApprovalRequest, open); err != nil {
		t.Fatal(err)
	}
	resolution := humanApprovalResolution(open.ApprovalID, open.WorkspaceID, open.RunID)
	if err := approvalCommandError(rawHub, approvalApplicationSource, approvalevents.TopicApprovalCommandEnsureApproved, resolution); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("invalid commit result err = %v, want invalid argument", err)
	}
	pending, err := repo.GetScoped(ctx, open.WorkspaceID, open.RunID, open.ApprovalID)
	if err != nil {
		t.Fatal(err)
	}
	if pending.State != common.ApprovalStatePending || pending.ResolutionProposal == nil || pending.ResolutionProposal.State != approvalmodel.ResolutionProposalPending {
		t.Fatalf("approval after invalid commit = %+v, want durable pending proposal", pending)
	}
	validCommit.Store(true)
	if err := owner.ReconcileApprovalDeliveries(ctx); err != nil {
		t.Fatalf("reconcile approval proposal: %v", err)
	}
	resolved, err := repo.GetScoped(ctx, open.WorkspaceID, open.RunID, open.ApprovalID)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.State != common.ApprovalStateApproved || resolved.EvidenceRecordedAt == "" || resolved.ResolutionNotificationPublishedAt == "" {
		t.Fatalf("reconciled approval = %+v, want terminal delivery markers", resolved)
	}
	if commitCount.Load() != 2 {
		t.Fatalf("runtime commit count = %d, want initial attempt plus recovery", commitCount.Load())
	}
	posted := hub.Posted()
	if len(posted) != 1 || posted[0].ID() != approvalevents.TopicApprovalResolved {
		t.Fatalf("resolved notifications = %+v, want one stable notification", posted)
	}
	if err := owner.ReconcileApprovalDeliveries(ctx); err != nil {
		t.Fatal(err)
	}
	if len(hub.Posted()) != 1 {
		t.Fatalf("duplicate reconciliation published %d notifications, want one", len(hub.Posted()))
	}
}

func TestApprovalFileOwnerRecoversDurableRuntimeProposalAfterReopen(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	open := approvalevents.OpenRuntimeApprovalRequestCommand{
		ApprovalID: "approval-file-proposal", WorkspaceID: "ws-file-proposal", RunID: "run-file-proposal",
		DispatchID: "dispatch-file-proposal", AuthoritySnapshotID: "authority-file-proposal",
		DependencyKind: string(runtimecontract.GovernanceApproval), SubjectRef: "secret-file.txt",
		Reason: "file-backed approval", RequestedAction: "file/write", RiskLevel: "high",
		AuthorizationContext: workorcheventpayload.New(workorcheventpayload.StringField("targetRef", "secret-file.txt")).Fields,
	}
	bound := common.ContextWithRuntimeWorkspaceID(ctx, open.WorkspaceID)

	firstRepositories := workorchrepo.NewWorkspaceScopedFileRepositorySet(root)
	firstHub := magicevent.NewHub(64)
	first, err := New(firstHub, nil, Dependencies{OwnerRepositories: firstRepositories.ApprovalOwner, EventHub: firstHub})
	if err != nil {
		t.Fatalf("configure first file owner: %v", err)
	}
	if err := first.Start(); err != nil {
		t.Fatalf("start first file owner: %v", err)
	}
	invalidCommit := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, firstHub)
	invalidCommit.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceApproval, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(struct{}{}, nil)
	})
	if err := approvalCommandError(firstHub, runtimecontract.RuntimeModuleOrchestrator, approvalevents.TopicOpenRuntimeApprovalRequest, open); err != nil {
		t.Fatalf("open file-backed approval: %v", err)
	}
	resolution := humanApprovalResolution(open.ApprovalID, open.WorkspaceID, open.RunID)
	if err := approvalCommandError(firstHub, approvalApplicationSource, approvalevents.TopicApprovalCommandEnsureApproved, resolution); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("invalid first commit result err = %v, want invalid argument", err)
	}
	pending, err := firstRepositories.Approvals.GetScoped(bound, open.WorkspaceID, open.RunID, open.ApprovalID)
	if err != nil || pending.State != common.ApprovalStatePending || pending.ResolutionProposal == nil || pending.ResolutionProposal.State != approvalmodel.ResolutionProposalPending {
		t.Fatalf("durable file proposal = %+v err=%v", pending, err)
	}
	invalidCommit.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceApproval)
	first.Teardown(ctx)
	firstHub.Terminate(ctx)

	secondRepositories := workorchrepo.NewWorkspaceScopedFileRepositorySet(root)
	secondHub := magicevent.NewHub(64)
	second, err := New(secondHub, nil, Dependencies{OwnerRepositories: secondRepositories.ApprovalOwner, EventHub: secondHub})
	if err != nil {
		t.Fatalf("configure reopened file owner: %v", err)
	}
	if err := second.Start(); err != nil {
		t.Fatalf("start reopened file owner: %v", err)
	}
	t.Cleanup(func() {
		second.Teardown(context.Background())
		secondHub.Terminate(context.Background())
	})
	commit := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, secondHub)
	commit.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceApproval, func(event magicevent.Event, result magicevent.Result) {
		command, ok := event.Data().(orchestratorevents.CommitRuntimeGovernanceApprovalResolutionCommand)
		if !ok {
			result.Set(struct{}{}, nil)
			return
		}
		result.Set(orchestratorevents.CommitRuntimeGovernanceResolutionResult{
			WorkspaceID: command.WorkspaceID, RunID: command.RunID, ResolutionID: command.ResolutionID,
			Disposition:   orchestratorevents.RuntimeGovernanceResolutionDispositionCommitted,
			StateRevision: 2, EvidenceRevision: "evidence-file-proposal",
			AuthoritySnapshotID: open.AuthoritySnapshotID, State: runtimecontract.ConvergenceStateNeedStateChange,
		}, nil)
	})
	t.Cleanup(func() { commit.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceApproval) })
	var evidenceValid atomic.Bool
	evidenceValid.Store(true)
	var resolutionValid atomic.Bool
	resolutionValid.Store(true)
	subscribeApprovalDeliveryResults(t, secondHub, &evidenceValid, &resolutionValid)

	if err := second.ReconcileApprovalDeliveries(bound); err != nil {
		t.Fatalf("reconcile reopened file proposal: %v", err)
	}
	recovered, err := secondRepositories.Approvals.GetScoped(bound, open.WorkspaceID, open.RunID, open.ApprovalID)
	if err != nil || recovered.State != common.ApprovalStateApproved || recovered.ResolutionProposal == nil || recovered.ResolutionProposal.State != approvalmodel.ResolutionProposalCommitted || recovered.EvidenceRecordedAt == "" || recovered.ResolutionNotificationPublishedAt == "" {
		t.Fatalf("recovered file approval = %+v err=%v", recovered, err)
	}
	if recovered.ResolutionPrincipal != resolution.Principal || !recovered.ResolutionContext.Equal(workorcheventpayload.New(resolution.ResolutionContext...)) {
		t.Fatalf("recovered file approval resolution drift = %+v", recovered)
	}
}

func TestApprovalFileOwnerRecoversTerminalDeliveryCheckpointsAfterReopen(t *testing.T) {
	for _, checkpoint := range []struct {
		name                  string
		id                    string
		firstEvidenceValid    bool
		firstResolutionValid  bool
		wantEvidencePersisted bool
	}{
		{name: "evidence pending", id: "evidence-pending", firstEvidenceValid: false, firstResolutionValid: true, wantEvidencePersisted: false},
		{name: "audit pending", id: "audit-pending", firstEvidenceValid: true, firstResolutionValid: false, wantEvidencePersisted: true},
	} {
		t.Run(checkpoint.name, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			workspaceID := common.WorkspaceID("ws-file-" + checkpoint.id)
			runID := common.RunID("run-file-" + checkpoint.id)
			approvalID := common.ApprovalID("approval-file-" + checkpoint.id)
			bound := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)

			firstRepositories := workorchrepo.NewWorkspaceScopedFileRepositorySet(root)
			if err := firstRepositories.Approvals.Create(bound, &approvalmodel.Approval{
				ID: approvalID, WorkspaceID: workspaceID, RunID: runID,
				Reason: "file delivery checkpoint", State: common.ApprovalStatePending,
			}); err != nil {
				t.Fatalf("seed approval: %v", err)
			}
			firstHub := magicevent.NewHub(64)
			first, err := New(firstHub, nil, Dependencies{OwnerRepositories: firstRepositories.ApprovalOwner, EventHub: firstHub})
			if err != nil {
				t.Fatalf("configure first owner: %v", err)
			}
			if err := first.Start(); err != nil {
				t.Fatalf("start first owner: %v", err)
			}
			var firstEvidenceValid atomic.Bool
			firstEvidenceValid.Store(checkpoint.firstEvidenceValid)
			var firstResolutionValid atomic.Bool
			firstResolutionValid.Store(checkpoint.firstResolutionValid)
			subscribeApprovalDeliveryResults(t, firstHub, &firstEvidenceValid, &firstResolutionValid)
			resolution := humanApprovalResolution(approvalID, workspaceID, runID)
			if err := approvalCommandError(firstHub, approvalApplicationSource, approvalevents.TopicApprovalCommandEnsureApproved, resolution); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("first checkpoint delivery err = %v, want invalid argument", err)
			}
			persisted, err := firstRepositories.Approvals.GetScoped(bound, workspaceID, runID, approvalID)
			if err != nil || persisted.State != common.ApprovalStateApproved || (persisted.EvidenceRecordedAt != "") != checkpoint.wantEvidencePersisted || persisted.ResolutionNotificationPublishedAt != "" {
				t.Fatalf("persisted checkpoint approval = %+v err=%v", persisted, err)
			}
			first.Teardown(ctx)
			firstHub.Terminate(ctx)

			secondRepositories := workorchrepo.NewWorkspaceScopedFileRepositorySet(root)
			secondHub := magicevent.NewHub(64)
			second, err := New(secondHub, nil, Dependencies{OwnerRepositories: secondRepositories.ApprovalOwner, EventHub: secondHub})
			if err != nil {
				t.Fatalf("configure reopened owner: %v", err)
			}
			if err := second.Start(); err != nil {
				t.Fatalf("start reopened owner: %v", err)
			}
			var secondEvidenceValid atomic.Bool
			secondEvidenceValid.Store(true)
			var secondResolutionValid atomic.Bool
			secondResolutionValid.Store(true)
			subscribeApprovalDeliveryResults(t, secondHub, &secondEvidenceValid, &secondResolutionValid)
			if err := second.ReconcileApprovalDeliveries(bound); err != nil {
				t.Fatalf("reconcile reopened checkpoint: %v", err)
			}
			recovered, err := secondRepositories.Approvals.GetScoped(bound, workspaceID, runID, approvalID)
			if err != nil || recovered.State != common.ApprovalStateApproved || recovered.EvidenceRecordedAt == "" || recovered.ResolutionNotificationPublishedAt == "" || recovered.ResolutionPrincipal != resolution.Principal {
				t.Fatalf("recovered checkpoint approval = %+v err=%v", recovered, err)
			}
			second.Teardown(ctx)
			secondHub.Terminate(ctx)
		})
	}
}

func TestApprovalTerminalEvidenceDeliveryRecoversWithoutReResolving(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(64)
	hub := &recordingApprovalHub{Hub: rawHub}
	t.Cleanup(func() { rawHub.Terminate(ctx) })
	repo := memory.NewApprovalRepository()
	owner := startApprovalBizForGovernanceTest(t, hub, repo)
	var evidenceValid atomic.Bool
	var resolutionValid atomic.Bool
	resolutionValid.Store(true)
	subscribeApprovalDeliveryResults(t, rawHub, &evidenceValid, &resolutionValid)

	const (
		workspaceID = common.WorkspaceID("ws-evidence-recovery")
		runID       = common.RunID("run-evidence-recovery")
		approvalID  = common.ApprovalID("approval-evidence-recovery")
	)
	if err := repo.Create(ctx, &approvalmodel.Approval{
		ID: approvalID, WorkspaceID: workspaceID, RunID: runID, Reason: "operator approval required",
		Context: workorcheventpayload.New(workorcheventpayload.StringField("targetRef", "resource-a")), State: common.ApprovalStatePending,
	}); err != nil {
		t.Fatal(err)
	}
	if err := approvalCommandError(rawHub, approvalApplicationSource, approvalevents.TopicApprovalCommandEnsureApproved, humanApprovalResolution(approvalID, workspaceID, runID)); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("invalid evidence ack err = %v, want invalid argument", err)
	}
	terminal, err := repo.GetScoped(ctx, workspaceID, runID, approvalID)
	if err != nil {
		t.Fatal(err)
	}
	if terminal.State != common.ApprovalStateApproved || terminal.EvidenceRecordedAt != "" || terminal.ResolutionNotificationPublishedAt != "" {
		t.Fatalf("approval after delivery failure = %+v, want durable terminal awaiting delivery", terminal)
	}
	evidenceValid.Store(true)
	if err := owner.ReconcileApprovalDeliveries(ctx); err != nil {
		t.Fatalf("recover terminal delivery: %v", err)
	}
	recovered, err := repo.GetScoped(ctx, workspaceID, runID, approvalID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.EvidenceRecordedAt == "" || recovered.ResolutionNotificationPublishedAt == "" {
		t.Fatalf("recovered approval = %+v, want completed delivery markers", recovered)
	}
	if len(hub.Posted()) != 1 {
		t.Fatalf("recovered notification count = %d, want one", len(hub.Posted()))
	}
}

func TestApprovalResolutionAuditAckFailureRecoversFromDurableTerminal(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(64)
	hub := &recordingApprovalHub{Hub: rawHub}
	t.Cleanup(func() { rawHub.Terminate(ctx) })
	repo := memory.NewApprovalRepository()
	owner := startApprovalBizForGovernanceTest(t, hub, repo)
	var evidenceValid atomic.Bool
	evidenceValid.Store(true)
	var resolutionValid atomic.Bool
	subscribeApprovalDeliveryResults(t, rawHub, &evidenceValid, &resolutionValid)

	const (
		workspaceID = common.WorkspaceID("ws-resolution-record-recovery")
		runID       = common.RunID("run-resolution-record-recovery")
		approvalID  = common.ApprovalID("approval-resolution-record-recovery")
	)
	if err := repo.Create(ctx, &approvalmodel.Approval{
		ID: approvalID, WorkspaceID: workspaceID, RunID: runID, Reason: "operator approval required",
		Context: workorcheventpayload.New(workorcheventpayload.StringField("targetRef", "resource-a")), State: common.ApprovalStatePending,
	}); err != nil {
		t.Fatal(err)
	}
	if err := approvalCommandError(rawHub, approvalApplicationSource, approvalevents.TopicApprovalCommandEnsureApproved, humanApprovalResolution(approvalID, workspaceID, runID)); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("invalid resolution record ack err = %v, want invalid argument", err)
	}
	terminal, err := repo.GetScoped(ctx, workspaceID, runID, approvalID)
	if err != nil {
		t.Fatal(err)
	}
	if terminal.State != common.ApprovalStateApproved || terminal.EvidenceRecordedAt == "" || terminal.ResolutionNotificationPublishedAt != "" {
		t.Fatalf("approval after audit failure = %+v, want evidence-recorded terminal awaiting audit", terminal)
	}
	resolutionValid.Store(true)
	if err := owner.ReconcileApprovalDeliveries(ctx); err != nil {
		t.Fatalf("recover resolution audit: %v", err)
	}
	recovered, err := repo.GetScoped(ctx, workspaceID, runID, approvalID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.ResolutionNotificationPublishedAt == "" || len(hub.Posted()) != 1 {
		t.Fatalf("recovered approval = %+v notifications=%d", recovered, len(hub.Posted()))
	}
}

func TestApprovalCommandErrorPreservesUnexpectedCause(t *testing.T) {
	hub := magicevent.NewHub(4)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	observer := magicevent.NewSimpleObserver(approvalevents.CommandDestination, hub)
	want := errors.New("owner failure")
	observer.Subscribe("/approval/test/error", func(_ magicevent.Event, result magicevent.Result) {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: want.Error(), Cause: want})
	})
	if err := approvalCommandError(hub, "test", "/approval/test/error", struct{}{}); !errors.Is(err, want) {
		t.Fatalf("command error = %v, want preserved cause", err)
	}
}
