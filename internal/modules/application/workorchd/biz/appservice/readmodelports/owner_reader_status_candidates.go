package readmodelports

func (c OwnerReaderCandidates) RunStatusReaders(owner OwnerReaders) RunStatusReaders {
	if c.Matches(owner) {
		return owner.RunStatusReaders()
	}
	return RunStatusReaders{
		Runs:        c.Runs,
		Steps:       c.Steps,
		Checkpoints: c.Checkpoints,
		Tickets:     c.Tickets,
		Approvals:   c.Approvals,
	}
}
