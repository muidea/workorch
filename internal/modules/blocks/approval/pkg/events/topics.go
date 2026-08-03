package events

import "github.com/muidea/workorch/internal/pkg/workorchownercontract"

const NotificationDestination = "/approval/consumers/#"

const (
	TopicApprovalResolved = "/approval/resolved"

	TopicApprovalCommandGet             = workorchownercontract.TopicApprovalCommandGet
	TopicApprovalCommandList            = "/approval/command/list"
	TopicApprovalCommandListByRun       = "/approval/command/list-by-run"
	TopicApprovalCommandListByWorkspace = "/approval/command/list-by-workspace"
	TopicApprovalCommandDelete          = "/approval/command/delete"
	TopicApprovalCommandEnsureApproved  = "/approval/command/ensure-approved"
	TopicApprovalCommandEnsureRejected  = "/approval/command/ensure-rejected"
	TopicApprovalCommandRecordEvidence  = "/approval/command/record-evidence"
)
