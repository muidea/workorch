package repository

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// CreateOrVerifyImmutableEvidenceRecord persists an evidence identity once.
// An exact duplicate is idempotent; the same identity with any different
// content is a conflict. Governance evidence is an audit fact and must never
// be rewritten by a later delivery of the same owner command.
func CreateOrVerifyImmutableEvidenceRecord(ctx context.Context, repo EvidenceRecordRepository, record *orchestratormodel.EvidenceRecord) error {
	if repo == nil || record == nil {
		return common.ErrInvalidArgument
	}
	if err := repo.Create(ctx, record); err != nil {
		if !errors.Is(err, common.ErrConflict) {
			return err
		}
		existing, getErr := repo.Get(ctx, record.ID)
		if getErr != nil {
			return getErr
		}
		if !sameImmutableEvidenceRecord(existing, record) {
			return fmt.Errorf("%w: evidence record %q already exists with different immutable fields: %s", common.ErrConflict, record.ID, evidenceRecordDifference(existing, record))
		}
	}
	return nil
}

func sameImmutableEvidenceRecord(left, right *orchestratormodel.EvidenceRecord) bool {
	return left != nil && right != nil &&
		left.ID == right.ID && left.WorkspaceID == right.WorkspaceID && left.RunID == right.RunID && left.StepID == right.StepID &&
		left.DefinitionRevision == right.DefinitionRevision && left.Kind == right.Kind && left.Status == right.Status &&
		left.SourceType == right.SourceType && left.SourceRef == right.SourceRef && left.SubjectType == right.SubjectType && left.SubjectID == right.SubjectID &&
		slices.Equal(left.CoverageItemRefs, right.CoverageItemRefs) && slices.Equal(left.Supersedes, right.Supersedes) &&
		left.CreatedAt == right.CreatedAt && left.UpdatedAt == right.UpdatedAt && left.Metadata.Equal(right.Metadata)
}

func evidenceRecordDifference(left, right *orchestratormodel.EvidenceRecord) string {
	if left == nil || right == nil {
		return "record"
	}
	differences := make([]string, 0, 8)
	fields := []struct {
		name  string
		equal bool
	}{
		{"identity", left.ID == right.ID},
		{"scope", left.WorkspaceID == right.WorkspaceID && left.RunID == right.RunID && left.StepID == right.StepID},
		{"definitionRevision", left.DefinitionRevision == right.DefinitionRevision},
		{"classification", left.Kind == right.Kind && left.Status == right.Status},
		{"source", left.SourceType == right.SourceType && left.SourceRef == right.SourceRef},
		{"subject", left.SubjectType == right.SubjectType && left.SubjectID == right.SubjectID},
		{"coverage", slices.Equal(left.CoverageItemRefs, right.CoverageItemRefs)},
		{"supersedes", slices.Equal(left.Supersedes, right.Supersedes)},
		{"timestamps", left.CreatedAt == right.CreatedAt && left.UpdatedAt == right.UpdatedAt},
		{"metadata", left.Metadata.Equal(right.Metadata)},
	}
	for _, field := range fields {
		if !field.equal {
			differences = append(differences, field.name)
		}
	}
	if len(differences) == 0 {
		return "record"
	}
	return strings.Join(differences, ",")
}
