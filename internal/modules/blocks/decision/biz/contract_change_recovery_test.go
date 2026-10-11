package decision

import (
	"context"
	"errors"
	"sync"
	"testing"

	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
)

// 仅在真实持久写完成后丢响应，不能用未执行的 fake 写入证明恢复。
type contractChangeLostResponseRepository struct {
	repository.DecisionTicketRepository
	loseCreate bool
	loseUpdate bool
}

var errContractChangeResponseLost = errors.New("contract change response lost after persistence")

func (r *contractChangeLostResponseRepository) Create(ctx context.Context, item *decisionmodel.Ticket) error {
	if err := r.DecisionTicketRepository.Create(ctx, item); err != nil {
		return err
	}
	if r.loseCreate {
		r.loseCreate = false
		return errContractChangeResponseLost
	}
	return nil
}

func (r *contractChangeLostResponseRepository) Update(ctx context.Context, item *decisionmodel.Ticket) error {
	if err := r.DecisionTicketRepository.Update(ctx, item); err != nil {
		return err
	}
	if r.loseUpdate {
		r.loseUpdate = false
		return errContractChangeResponseLost
	}
	return nil
}

func TestContractChangeFileWriteResponseLossReopensOriginalVersion(t *testing.T) {
	root := t.TempDir()
	cmd := contractChangeCommandForTest()
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), cmd.Proposal.WorkspaceID)
	repos := workorchrepo.NewWorkspaceScopedFileRepositorySet(root)
	hub := newRecordingDecisionHub(t)
	owner, err := New(hub, nil, Dependencies{OwnerRepositories: repos.DecisionOwner, EventHub: hub})
	if err != nil {
		t.Fatal(err)
	}
	cut := &contractChangeLostResponseRepository{DecisionTicketRepository: repos.Tickets, loseCreate: true, loseUpdate: true}
	owner.tickets, owner.service.tickets = cut, cut
	if _, err := owner.registerContractChangeProposal(ctx, cmd); !errors.Is(err, errContractChangeResponseLost) {
		t.Fatalf("create cut=%v", err)
	}
	reopen := func() *Biz {
		t.Helper()
		r := workorchrepo.NewWorkspaceScopedFileRepositorySet(root)
		m, err := New(hub, nil, Dependencies{OwnerRepositories: r.DecisionOwner, EventHub: hub})
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	fresh := reopen()
	first, err := fresh.registerContractChangeProposal(ctx, cmd)
	if err != nil || first.Disposition != "duplicate" || first.ValidateFor(cmd) != nil {
		t.Fatalf("create恢复=%+v %v", first, err)
	}
	next := cmd
	next.RequestID = "second"
	next.ExpectedRevision, next.ExpectedHash = first.Revision, first.Hash
	next.Proposal = cmd.Proposal.Clone()
	next.Proposal.EvidenceGaps += "\r\n补充未知"
	if _, err := owner.registerContractChangeProposal(ctx, next); !errors.Is(err, errContractChangeResponseLost) {
		t.Fatalf("update cut=%v", err)
	}
	fresh = reopen()
	second, err := fresh.registerContractChangeProposal(ctx, next)
	if err != nil || second.Disposition != "duplicate" || second.ValidateFor(next) != nil {
		t.Fatalf("update恢复=%+v %v", second, err)
	}
	stored, err := fresh.tickets.Get(ctx, cmd.TicketID)
	if err != nil || len(stored.ContractChangeRevisions) != 2 || stored.ValidateContractChange() != nil {
		t.Fatalf("丢响应重复登记=%v", err)
	}
	if len(hub.EvidenceCommands()) != 0 {
		t.Fatal("登记产生Runtime evidence")
	}
}

func TestContractChangeConcurrentFileOwnersRejectStaleAppend(t *testing.T) {
	root := t.TempDir()
	cmd := contractChangeCommandForTest()
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), cmd.Proposal.WorkspaceID)
	hub := newRecordingDecisionHub(t)
	newOwner := func() *Biz {
		r := workorchrepo.NewWorkspaceScopedFileRepositorySet(root)
		m, err := New(hub, nil, Dependencies{OwnerRepositories: r.DecisionOwner, EventHub: hub})
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	firstOwner, secondOwner := newOwner(), newOwner()
	first, err := firstOwner.registerContractChangeProposal(ctx, cmd)
	if err != nil {
		t.Fatal(err)
	}
	commands := []decisionevents.RegisterContractChangeProposalCommand{cmd, cmd}
	for n := range commands {
		commands[n].ExpectedRevision, commands[n].ExpectedHash = first.Revision, first.Hash
		commands[n].Proposal = cmd.Proposal.Clone()
	}
	commands[0].RequestID = "competitor-a"
	commands[1].RequestID = "competitor-b"
	commands[0].Proposal.WhyChange += "甲"
	commands[1].Proposal.WhyChange += "乙"
	start := make(chan struct{})
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for n, owner := range []*Biz{firstOwner, secondOwner} {
		wg.Add(1)
		go func() { defer wg.Done(); <-start; _, errs[n] = owner.registerContractChangeProposal(ctx, commands[n]) }()
	}
	close(start)
	wg.Wait()
	if !(errs[0] == nil && errors.Is(errs[1], common.ErrConflict) || errs[1] == nil && errors.Is(errs[0], common.ErrConflict)) {
		t.Fatalf("竞争追加=%v", errs)
	}
	stored, err := newOwner().tickets.Get(ctx, cmd.TicketID)
	if err != nil || len(stored.ContractChangeRevisions) != 2 || stored.ValidateContractChange() != nil {
		t.Fatalf("竞争丢历史=%v", err)
	}
	stale := decisionmodel.CloneTicket(stored)
	stale.ContractChangeRevisions[0].Proposal.WhyChange = "覆盖原历史"
	stale.ContractChangeRevisions[0].Hash = stale.ContractChangeRevisions[0].Proposal.Hash()
	if err := firstOwner.tickets.Update(ctx, stale); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("覆盖历史=%v", err)
	}
}
