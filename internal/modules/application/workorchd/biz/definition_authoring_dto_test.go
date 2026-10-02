package biz

import (
	workspaceeventmapping "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/eventmapping"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

func authoringTestRecord(record workspace.DefinitionAuthoringRecord) *workspaceevents.DefinitionAuthoringRecord {
	dto := workspaceeventmapping.DefinitionAuthoringRecordDTO(record)
	return &dto
}
