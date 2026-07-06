package events

import "github.com/muidea/workorch/internal/pkg/workorchownercontract"

const (
	TopicApprovalCreated    = "/approval/created"
	TopicApprovalApproved   = "/approval/approved"
	TopicApprovalRejected   = "/approval/rejected"
	TopicApprovalResolved   = "/approval/resolved"
	TopicApprovalSuperseded = "/approval/superseded"

	TopicApprovalCommandCreate          = "/approval/command/create"
	TopicApprovalCommandEnsure          = "/approval/command/ensure"
	TopicApprovalCommandGet             = workorchownercontract.TopicApprovalCommandGet
	TopicApprovalCommandList            = "/approval/command/list"
	TopicApprovalCommandListByRun       = "/approval/command/list-by-run"
	TopicApprovalCommandListByWorkspace = "/approval/command/list-by-workspace"
	TopicApprovalCommandUpdate          = "/approval/command/update"
	TopicApprovalCommandDelete          = "/approval/command/delete"
	TopicApprovalCommandEnsureApproved  = "/approval/command/ensure-approved"
	TopicApprovalCommandEnsureRejected  = "/approval/command/ensure-rejected"
)
