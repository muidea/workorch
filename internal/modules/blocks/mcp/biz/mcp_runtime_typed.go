package mcp

import (
	"encoding/json"

	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type RuntimeOperationRequest struct {
	WorkspaceID                   common.WorkspaceID
	RunID                         common.RunID
	StepID                        common.StepID
	Kind                          string
	WorkspaceDeliverableDirectory string
	WorkspaceInternalDirectory    string
	TargetRefs                    []string
	Data                          orchestratorevents.MCPOperationData
	EvidencePolicies              []runtimecontract.RuntimeEvidencePolicy
}

type RuntimeOperationResult struct {
	Status         string
	Stage          string
	Operation      string
	ServerID       string
	Tool           string
	URI            string
	Prompt         string
	Summary        string
	Transport      string
	Endpoint       string
	DurationMillis int64
	StatusCode     int64
	Stderr         string
	FailureCode    string
	FailureMessage string
	TargetRefs     []string
	Response       MCPResponsePayload
	StreamEvents   []MCPStreamEvent
	Diagnostics    MCPCapabilityFailureDiagnostics
	Evidence       runtimecontract.RuntimeOperationEvidence
}

type MCPResponsePayload struct {
	JSON json.RawMessage
	Text string
}

type MCPStreamEvent struct {
	Method  string
	Params  json.RawMessage
	Payload MCPResponsePayload
}

type MCPCapabilityFailureDiagnostics struct {
	Operation      string
	ServerID       string
	Tool           string
	URI            string
	Prompt         string
	TargetRef      string
	FailureTargets []string
}
