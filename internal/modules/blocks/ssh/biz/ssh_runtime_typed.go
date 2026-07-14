package ssh

import (
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type RuntimeCommandRequest struct {
	WorkspaceID                   common.WorkspaceID
	RunID                         common.RunID
	StepID                        common.StepID
	WorkspaceDeliverableDirectory string
	WorkspaceInternalDirectory    string
	Host                          string
	Port                          int64
	Username                      string
	Password                      string
	PrivateKey                    string
	PrivateKeyPath                string
	Command                       string
	TimeoutMillis                 int64
	EvidencePolicies              []runtimecontract.RuntimeEvidencePolicy
}

type RuntimeCommandResult struct {
	Status          string
	Host            string
	Port            int64
	Username        string
	Command         string
	Stdout          string
	Stderr          string
	ExitCode        int64
	DurationMillis  int64
	TimeoutMillis   int64
	AuthMethod      string
	OutputTruncated bool
	StdoutTruncated bool
	StderrTruncated bool
	StdoutBytes     int64
	StderrBytes     int64
	FailureCode     string
	FailureMessage  string
	Evidence        runtimecontract.RuntimeOperationEvidence
}
