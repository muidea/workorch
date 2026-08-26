package repository_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	filerepo "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestCreateOrVerifyImmutableEvidenceRecordUsesJSONSemanticEquality(t *testing.T) {
	ctx := context.Background()
	repo := filerepo.NewEvidenceRecordRepository(filepath.Join(t.TempDir(), "evidence"))
	record := &runtimecontract.EvidenceRecord{
		ID:               "evidence-immutable",
		WorkspaceID:      "ws-immutable",
		RunID:            "run-immutable",
		Kind:             runtimecontract.EvidenceKindGovernance,
		Status:           runtimecontract.EvidenceStatusAccepted,
		SourceRef:        "decision-immutable",
		CoverageItemRefs: []string{},
		Supersedes:       []common.EvidenceRecordID{},
		CreatedAt:        "2026-08-26T00:00:00Z",
		UpdatedAt:        "2026-08-26T00:00:00Z",
		Metadata: workorcheventpayload.New(
			workorcheventpayload.StringField("alpha", "one"),
			workorcheventpayload.StringField("beta", "two"),
		),
	}
	if err := repository.CreateOrVerifyImmutableEvidenceRecord(ctx, repo, record); err != nil {
		t.Fatalf("create immutable evidence: %v", err)
	}

	duplicate := *record
	duplicate.CoverageItemRefs = nil
	duplicate.Supersedes = nil
	duplicate.Metadata = workorcheventpayload.New(
		workorcheventpayload.StringField("beta", "two"),
		workorcheventpayload.StringField("alpha", "one"),
	)
	if err := repository.CreateOrVerifyImmutableEvidenceRecord(ctx, repo, &duplicate); err != nil {
		t.Fatalf("semantically exact JSON round-trip duplicate must be idempotent: %v", err)
	}

	changed := duplicate
	changed.Status = runtimecontract.EvidenceStatusRejected
	if err := repository.CreateOrVerifyImmutableEvidenceRecord(ctx, repo, &changed); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("changed immutable evidence must conflict, got %v", err)
	}
	stored, err := repo.Get(ctx, record.ID)
	if err != nil {
		t.Fatalf("load stored immutable evidence: %v", err)
	}
	if stored.Status != runtimecontract.EvidenceStatusAccepted {
		t.Fatalf("conflicting replay rewrote immutable evidence: %#v", stored)
	}
}
