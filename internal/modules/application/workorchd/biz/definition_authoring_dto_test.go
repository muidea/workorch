package biz

import (
	workspaceeventmapping "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/eventmapping"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
)

func authoringTestRecord(record workspace.DefinitionAuthoringRecord) *workspaceevents.DefinitionAuthoringRecord {
	dto := workspaceeventmapping.DefinitionAuthoringRecordDTO(record)
	return &dto
}
