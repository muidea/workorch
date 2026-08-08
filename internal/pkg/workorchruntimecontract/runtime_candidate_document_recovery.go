package workorchruntimecontract

import "fmt"

// RuntimeCandidateDocumentRecoveryPolicy is the activation-sealed maximum
// number of complete-document retries after the initial provider response.
type RuntimeCandidateDocumentRecoveryPolicy struct {
	MaxAttempts int
}

func (p RuntimeCandidateDocumentRecoveryPolicy) Normalized() RuntimeCandidateDocumentRecoveryPolicy {
	return p
}

func (p RuntimeCandidateDocumentRecoveryPolicy) Validate() error {
	p = p.Normalized()
	if p.MaxAttempts < 0 || p.MaxAttempts > RuntimeRecoveryBudgetLimit {
		return fmt.Errorf("runtime candidate document recovery attempts must be in 0..%d", RuntimeRecoveryBudgetLimit)
	}
	return nil
}
