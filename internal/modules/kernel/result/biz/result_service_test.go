package result

import (
	"context"
	"errors"
	"testing"

	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestServiceRejectsInvalidResultAndArtifact(t *testing.T) {
	ctx := context.Background()
	service := NewService(memory.NewResultRepository(), memory.NewArtifactRepository())
	if err := service.CreateResult(ctx, &resultmodel.Result{}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("CreateResult err = %v, want invalid argument", err)
	}
	if err := service.CreateArtifact(ctx, &resultmodel.Artifact{}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("CreateArtifact err = %v, want invalid argument", err)
	}
}

func TestServiceCRUD(t *testing.T) {
	ctx := context.Background()
	service := NewService(memory.NewResultRepository(), memory.NewArtifactRepository())
	result := validTestResult()
	if err := service.CreateResult(ctx, result); err != nil {
		t.Fatalf("CreateResult: %v", err)
	}
	result.State = common.ResultStateSuperseded
	if err := service.UpdateResult(ctx, result); err != nil {
		t.Fatalf("UpdateResult: %v", err)
	}
	updated, err := service.GetResult(ctx, result.ID)
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if updated.State != common.ResultStateSuperseded {
		t.Fatalf("updated state = %q, want superseded", updated.State)
	}
	artifact := validTestArtifact(result.ID)
	if err := service.CreateArtifact(ctx, artifact); err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}
	artifact.State = common.ArtifactStateAvailable
	if err := service.UpdateArtifact(ctx, artifact); err != nil {
		t.Fatalf("UpdateArtifact: %v", err)
	}
	if err := service.DeleteArtifact(ctx, artifact.ID); err != nil {
		t.Fatalf("DeleteArtifact: %v", err)
	}
	if _, err := service.GetArtifact(ctx, artifact.ID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("GetArtifact after delete err = %v, want not found", err)
	}
	if err := service.DeleteResult(ctx, result.ID); err != nil {
		t.Fatalf("DeleteResult: %v", err)
	}
	if _, err := service.GetResult(ctx, result.ID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("GetResult after delete err = %v, want not found", err)
	}
}

func validTestResult() *resultmodel.Result {
	return &resultmodel.Result{
		ID:          "result-test",
		WorkspaceID: "ws-result-test",
		RunID:       "run-result-test",
		GoalID:      "goal-result-test",
		DoDID:       "dod-result-test",
		State:       common.ResultStateAccepted,
		Title:       "accepted result",
		Summary:     "accepted",
	}
}

func validTestArtifact(resultID common.ResultID) *resultmodel.Artifact {
	return &resultmodel.Artifact{
		ID:          "artifact-test",
		WorkspaceID: "ws-result-test",
		RunID:       "run-result-test",
		ResultID:    resultID,
		State:       common.ArtifactStateCreated,
		Name:        "artifact",
		Summary:     "artifact",
	}
}
