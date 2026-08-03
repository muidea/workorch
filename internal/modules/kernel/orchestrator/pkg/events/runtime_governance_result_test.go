package events

import (
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestCommitRuntimeGovernanceResolutionResultPreservesScopedCASFact(t *testing.T) {
	base := CommitRuntimeGovernanceResolutionResult{
		WorkspaceID: "ws-governance", RunID: "run-governance", ResolutionID: "resolution-governance",
		Disposition:   RuntimeGovernanceResolutionDispositionCommitted,
		StateRevision: 1, AuthoritySnapshotID: "authority-governance",
		State: runtimecontract.ConvergenceStateNeedStateChange,
	}
	tests := []struct {
		name    string
		mutate  func(*CommitRuntimeGovernanceResolutionResult)
		wantErr bool
	}{
		{name: "committed"},
		{name: "stale", mutate: func(r *CommitRuntimeGovernanceResolutionResult) {
			r.Disposition = RuntimeGovernanceResolutionDispositionStale
		}},
		{name: "duplicate", mutate: func(r *CommitRuntimeGovernanceResolutionResult) {
			r.Disposition = RuntimeGovernanceResolutionDispositionDuplicate
		}},
		{name: "missing workspace", wantErr: true, mutate: func(r *CommitRuntimeGovernanceResolutionResult) { r.WorkspaceID = "" }},
		{name: "missing run", wantErr: true, mutate: func(r *CommitRuntimeGovernanceResolutionResult) { r.RunID = "" }},
		{name: "unknown disposition", wantErr: true, mutate: func(r *CommitRuntimeGovernanceResolutionResult) { r.Disposition = "unknown" }},
		{name: "terminal state without detail", wantErr: true, mutate: func(r *CommitRuntimeGovernanceResolutionResult) { r.State = runtimecontract.ConvergenceStateComplete }},
		{name: "nonterminal state with detail", wantErr: true, mutate: func(r *CommitRuntimeGovernanceResolutionResult) {
			r.Terminal = &runtimecontract.RuntimeTerminalDecision{Code: "unexpected"}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := base
			if test.mutate != nil {
				test.mutate(&result)
			}
			err := result.Validate()
			if (err != nil) != test.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}
