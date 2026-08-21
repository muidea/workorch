package ssh

import (
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type RuntimeCommandRequest struct {
	WorkspaceID      common.WorkspaceID
	RunID            common.RunID
	StepID           common.StepID
	TargetRef        string
	SessionRef       string
	CredentialRef    string
	Command          string
	TimeoutMillis    int64
	EvidencePolicies []runtimecontract.RuntimeEvidencePolicy
}

type RuntimeCommandResult struct {
	Status          string
	TargetRef       string
	ObservationRef  string
	ExitCode        int64
	DurationMillis  int64
	TimeoutMillis   int64
	OutputTruncated bool
	StdoutTruncated bool
	StderrTruncated bool
	StdoutBytes     int64
	StderrBytes     int64
	OutputHash      string
	BeforeHash      string
	AfterHash       string
	Changed         bool
	ObservedScope   runtimecontract.RuntimeEffectScope
	FailureCode     string
	FailureMessage  string
	Evidence        runtimecontract.RuntimeOperationEvidence
}
