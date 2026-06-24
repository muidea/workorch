package repository

import (
	"context"
	"errors"

	"github.com/muidea/workorch/internal/pkg/common"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func CreateOrUpdateEvidenceRecord(ctx context.Context, repo EvidenceRecordRepository, record *orchestratormodel.EvidenceRecord) error {
	if repo == nil || record == nil {
		return nil
	}
	if err := repo.Create(ctx, record); err != nil {
		if !errors.Is(err, common.ErrConflict) {
			return err
		}
		existing, getErr := repo.Get(ctx, record.ID)
		if getErr != nil {
			return getErr
		}
		record.CreatedAt = firstNonEmptyEvidenceRecordString(existing.CreatedAt, record.CreatedAt)
		if err := repo.Update(ctx, record); err != nil {
			return err
		}
	}
	return nil
}

func firstNonEmptyEvidenceRecordString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
