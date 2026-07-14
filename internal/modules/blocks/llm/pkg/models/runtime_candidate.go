package models

import runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

type RuntimeCandidateRequest struct {
	Decision          runtimecontract.RuntimeDecision
	Transaction       runtimecontract.AuthorityTransaction
	CandidateAttempt  int
	PreviousFailure   *runtimecontract.RuntimeCandidateFailure
	StablePrefixHash  string
	DefinitionContext runtimecontract.RuntimeDefinitionContext
	CandidateContext  runtimecontract.RuntimeCandidateContext
}

type RuntimeCandidateResponse struct {
	Transaction runtimecontract.AuthorityTransaction
	Candidate   runtimecontract.RuntimeCandidate
}
