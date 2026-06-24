package repository

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type EvidenceRecordRepository interface {
	Create(context.Context, *orchestratormodel.EvidenceRecord) error
	Get(context.Context, common.EvidenceRecordID) (*orchestratormodel.EvidenceRecord, error)
	ListByRun(context.Context, common.RunID) ([]*orchestratormodel.EvidenceRecord, error)
	ListByStep(context.Context, common.StepID) ([]*orchestratormodel.EvidenceRecord, error)
	ListBySourceRef(context.Context, common.RunID, string) ([]*orchestratormodel.EvidenceRecord, error)
	Update(context.Context, *orchestratormodel.EvidenceRecord) error
	Delete(context.Context, common.EvidenceRecordID) error
}
