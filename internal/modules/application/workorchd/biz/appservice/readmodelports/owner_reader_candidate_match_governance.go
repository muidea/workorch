package readmodelports

func (c OwnerReaderCandidates) matchesGovernanceReaders(owner OwnerReaders) bool {
	tickets, ok := c.Tickets.(DecisionTicketReader)
	if !ok || tickets != owner.Tickets {
		return false
	}
	resolutions, ok := c.Resolutions.(DecisionResolutionReader)
	if !ok || resolutions != owner.Resolutions {
		return false
	}
	approvals, ok := c.Approvals.(ApprovalReader)
	return ok && approvals == owner.Approvals
}
