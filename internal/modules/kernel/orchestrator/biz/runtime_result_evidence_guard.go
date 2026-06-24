package orchestrator

import (
	"context"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (r *resultSubsystem) ensureAcceptedOutputEvidenceRefs(ctx context.Context, runID common.RunID, outputKind string, refs []string) error {
	if r == nil || r.evidence.EvidenceRecords == nil || runID == "" {
		return nil
	}
	refs = compactUniqueStrings(refs)
	if len(refs) == 0 {
		return nil
	}
	records, err := r.evidence.EvidenceRecords.ListByRun(ctx, runID)
	if err != nil {
		return err
	}
	rejected := rejectedEvidenceRefIndex(records)
	if len(rejected) == 0 {
		return nil
	}
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		if record, ok := rejected[ref]; ok {
			return fmt.Errorf("%w: rejected evidence ref %q cannot back accepted %s via %s", common.ErrInvalidArgument, ref, strings.TrimSpace(outputKind), record.SourceRef)
		}
	}
	return nil
}

func rejectedEvidenceRefIndex(records []*orchestratormodel.EvidenceRecord) map[string]*orchestratormodel.EvidenceRecord {
	ret := map[string]*orchestratormodel.EvidenceRecord{}
	for _, record := range records {
		if record == nil || record.Status != orchestratormodel.EvidenceStatusRejected {
			continue
		}
		indexRejectedEvidenceRef(ret, string(record.ID), record)
		indexRejectedEvidenceRef(ret, record.SourceRef, record)
		indexRejectedEvidenceRef(ret, string(record.StepID), record)
		indexRejectedEvidenceRef(ret, record.SubjectID, record)
	}
	return ret
}

func indexRejectedEvidenceRef(index map[string]*orchestratormodel.EvidenceRecord, ref string, record *orchestratormodel.EvidenceRecord) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return
	}
	if _, exists := index[ref]; !exists {
		index[ref] = record
	}
}
