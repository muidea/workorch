package repository_test

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	file "github.com/muidea/workorch/internal/pkg/repository/file"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

// 两种实际仓库均须隔离调用方副本；file reopen 只验证当前格式，不引入历史转换。
func TestDecisionTicketRepositoriesDetachPersistedBinding(t *testing.T) {
	for _, backend := range []string{"memory", "file"} {
		t.Run(backend, func(t *testing.T) {
			ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-ticket-clone")
			var repo repository.DecisionTicketRepository
			var reopen func() repository.DecisionTicketRepository
			if backend == "memory" {
				repo = memory.NewDecisionTicketRepository()
				reopen = func() repository.DecisionTicketRepository { return repo }
			} else {
				path := filepath.Join(t.TempDir(), "tickets")
				repo = file.NewDecisionTicketRepository(path)
				reopen = func() repository.DecisionTicketRepository { return file.NewDecisionTicketRepository(path) }
			}
			for _, kind := range []string{"clarification", "governance"} {
				t.Run(kind, func(t *testing.T) {
					item := &decisionmodel.Ticket{
						ID: common.TicketID("ticket-" + kind), WorkspaceID: "ws-ticket-clone", RunID: "run-ticket-clone",
						State: common.DecisionTicketStateOpen, Reason: "完整原因\r\n原文\x00",
						Context:                workorcheventpayload.New(workorcheventpayload.StringArrayField("refs", []string{"原始引用"})),
						AutoGovernance:         workorcheventpayload.New(workorcheventpayload.StringArrayField("reasons", []string{"原始说明"})),
						AllowedRecoveryActions: []string{"provide_missing_info"},
					}
					if kind == "clarification" {
						item.DecisionKind = "runtime_definition_clarification"
						item.DefinitionClarification = &decisionmodel.DefinitionClarificationBinding{
							Class: decisionmodel.DefinitionClarificationRuntime, SchemaVersion: 1,
							OpenBindingHash: "original-binding", DefinitionRevision: "original-definition", StartupInputHash: "original-startup",
							OffendingRefs: []string{"original-ref"},
						}
					} else {
						item.DecisionKind = "runtime_governance"
						item.RuntimeGovernanceProposal = &decisionmodel.RuntimeGovernanceResolutionProposal{
							ResolutionID: "original-resolution", PayloadHash: "original-payload", State: decisionmodel.RuntimeGovernanceResolutionProposalPending,
							Payload: workorcheventpayload.New(workorcheventpayload.StringArrayField("facts", []string{"原始事实"})),
						}
					}
					encode := func(value *decisionmodel.Ticket) []byte {
						t.Helper()
						body, err := json.Marshal(value)
						if err != nil {
							t.Fatal(err)
						}
						return body
					}
					mutate := func(value *decisionmodel.Ticket) {
						value.AllowedRecoveryActions[0] = "caller-action"
						value.Context.Fields[0].Value.Array[0].String = "caller-context"
						value.AutoGovernance.Fields[0].Value.Array[0].String = "caller-governance"
						if value.DefinitionClarification != nil {
							value.DefinitionClarification.OpenBindingHash = "caller-binding"
							value.DefinitionClarification.DefinitionRevision = "caller-definition"
							value.DefinitionClarification.OffendingRefs[0] = "caller-ref"
						}
						if value.RuntimeGovernanceProposal != nil {
							value.RuntimeGovernanceProposal.PayloadHash = "caller-payload"
							value.RuntimeGovernanceProposal.Payload.Fields[0].Value.Array[0].String = "caller-fact"
						}
					}
					if err := repo.Create(ctx, item); err != nil {
						t.Fatal(err)
					}
					expected := encode(item)
					assertStored := func(t *testing.T) {
						t.Helper()
						stored, err := repo.Get(ctx, item.ID)
						if err != nil {
							t.Fatal(err)
						}
						if !bytes.Equal(encode(stored), expected) {
							t.Fatal("调用方副本绕过 owner 写入改变持久 Ticket")
						}
					}
					mutate(item)
					assertStored(t)
					for _, read := range []struct {
						name string
						load func() ([]*decisionmodel.Ticket, error)
					}{
						{"get", func() ([]*decisionmodel.Ticket, error) {
							value, err := repo.Get(ctx, item.ID)
							return []*decisionmodel.Ticket{value}, err
						}},
						{"all", func() ([]*decisionmodel.Ticket, error) { return repo.List(ctx) }},
						{"run", func() ([]*decisionmodel.Ticket, error) { return repo.ListByRunID(ctx, item.RunID) }},
						{"workspace", func() ([]*decisionmodel.Ticket, error) { return repo.ListByWorkspaceID(ctx, item.WorkspaceID) }},
					} {
						t.Run(read.name, func(t *testing.T) {
							values, err := read.load()
							if err != nil {
								t.Fatal(err)
							}
							found := false
							for _, value := range values {
								if value.ID == item.ID {
									mutate(value)
									found = true
								}
							}
							if !found {
								t.Fatal("实际仓库查询未命中目标 Ticket")
							}
							assertStored(t)
						})
					}
					update, err := repo.Get(ctx, item.ID)
					if err != nil {
						t.Fatal(err)
					}
					update.Question = "正式更新\r\n完整保留"
					if err := repo.Update(ctx, update); err != nil {
						t.Fatal(err)
					}
					expected = encode(update)
					mutate(update)
					assertStored(t)
					repo = reopen()
					assertStored(t)
				})
			}
		})
	}
}
