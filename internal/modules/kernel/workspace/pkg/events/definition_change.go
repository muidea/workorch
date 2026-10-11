package events

import (
	"strings"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/pkg/common"
)

const TopicWorkspaceDefinitionChangePrepare = "/workspace/definition-change/prepare"
const TopicWorkspaceDefinitionChangeQuery = "/workspace/definition-change/query"

// 只携带原 Decision 身份；候选输入由 Workspace 从原文差异构造。
type PrepareDefinitionChangeCommand struct {
	WorkspaceID               common.WorkspaceID
	RequestID                 string
	ExpectedWorkspaceRevision uint64
	TicketID                  common.TicketID
	ProposalRevision          uint64
	ProposalHash              string
}

func (c PrepareDefinitionChangeCommand) Validate() error {
	if c.WorkspaceID == "" || c.TicketID == "" || strings.TrimSpace(c.RequestID) == "" || len(c.RequestID) > 256 || !utf8.ValidString(c.RequestID) ||
		c.ExpectedWorkspaceRevision == 0 || c.ExpectedWorkspaceRevision == ^uint64(0) || c.ProposalRevision == 0 || c.ProposalHash == "" {
		return common.ErrInvalidArgument
	}
	return nil
}

type DefinitionChangeCandidate struct {
	RequestID                 string
	ExpectedWorkspaceRevision uint64
	RecordedWorkspaceRevision uint64
	TicketID                  common.TicketID
	ProposalRevision          uint64
	ProposalHash              string
	BaseDefinitionRevision    string
	BaseStartupInputHash      string
	BaseInputHash             string
	Record                    DefinitionAuthoringRecord
}

func (c DefinitionChangeCandidate) Clone() DefinitionChangeCandidate {
	c.Record = c.Record.Clone()
	return c
}

type PrepareDefinitionChangeResult struct {
	WorkspaceID       common.WorkspaceID
	WorkspaceRevision uint64
	Candidate         DefinitionChangeCandidate
}

func (r PrepareDefinitionChangeResult) ValidateFor(c PrepareDefinitionChangeCommand) error {
	v := r.Candidate
	if c.Validate() != nil || r.WorkspaceID != c.WorkspaceID || r.WorkspaceRevision < v.RecordedWorkspaceRevision ||
		v.RequestID != c.RequestID || v.ExpectedWorkspaceRevision != c.ExpectedWorkspaceRevision || v.RecordedWorkspaceRevision != c.ExpectedWorkspaceRevision+1 ||
		v.TicketID != c.TicketID || v.ProposalRevision != c.ProposalRevision || v.ProposalHash != c.ProposalHash ||
		v.Record.Input.WorkspaceID != c.WorkspaceID || v.BaseInputHash == "" || v.Record.InputHash == v.BaseInputHash {
		return common.ErrConflict
	}
	record, err := v.Record.Model()
	if err != nil || record.ValidateView() != nil {
		return common.ErrConflict
	}
	return nil
}

type QueryDefinitionChangeCommand struct {
	WorkspaceID common.WorkspaceID
}

type QueryDefinitionChangeResult struct {
	WorkspaceID       common.WorkspaceID
	WorkspaceRevision uint64
	Candidate         *DefinitionChangeCandidate
}

func (r QueryDefinitionChangeResult) ValidateFor(c QueryDefinitionChangeCommand) error {
	if c.WorkspaceID == "" || r.WorkspaceID != c.WorkspaceID || r.WorkspaceRevision == 0 {
		return common.ErrConflict
	}
	if r.Candidate == nil {
		return nil
	}
	v := r.Candidate
	return (PrepareDefinitionChangeResult{WorkspaceID: r.WorkspaceID, WorkspaceRevision: r.WorkspaceRevision, Candidate: *v}).ValidateFor(PrepareDefinitionChangeCommand{
		WorkspaceID: c.WorkspaceID, RequestID: v.RequestID, ExpectedWorkspaceRevision: v.ExpectedWorkspaceRevision,
		TicketID: v.TicketID, ProposalRevision: v.ProposalRevision, ProposalHash: v.ProposalHash,
	})
}
