package events

import (
	"fmt"
	"strings"
	"unicode/utf8"

	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

// Expected 为零只允许首次登记；修订必须绑定原持久版本，不携带 target state。
type RegisterContractChangeProposalCommand struct {
	TicketID         common.TicketID
	RequestID        string
	ExpectedRevision uint64
	ExpectedHash     string
	Proposal         decisionmodel.ContractChangeProposal
}

type RegisterContractChangeProposalResult struct {
	TicketID    common.TicketID
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	RequestID   string
	Revision    uint64
	Hash        string
	Disposition string
}

func (c RegisterContractChangeProposalCommand) Validate() error {
	if c.TicketID == "" || strings.TrimSpace(c.RequestID) == "" || !utf8.ValidString(c.RequestID) || len(c.RequestID) > 256 ||
		((c.ExpectedRevision == 0) != (c.ExpectedHash == "")) || c.ExpectedRevision >= decisionmodel.ContractChangeRevisionLimit {
		return fmt.Errorf("%w: contract change registration identity", common.ErrInvalidArgument)
	}
	return c.Proposal.Validate()
}

func (r RegisterContractChangeProposalResult) ValidateFor(c RegisterContractChangeProposalCommand) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if r.TicketID != c.TicketID || r.WorkspaceID != c.Proposal.WorkspaceID || r.RunID != c.Proposal.RunID ||
		r.RequestID != c.RequestID || r.Revision != c.ExpectedRevision+1 || r.Hash != c.Proposal.Hash() ||
		(r.Disposition != "registered" && r.Disposition != "duplicate") {
		return common.ErrConflict
	}
	return nil
}
