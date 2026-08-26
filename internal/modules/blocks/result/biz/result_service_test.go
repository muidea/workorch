package result

import (
	"context"
	"errors"
	"testing"

	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestServiceRejectsInvalidResultAndArtifact(t *testing.T) {
	ctx := context.Background()
	service := newService(memory.NewResultRepository(), memory.NewArtifactRepository())
	if err := service.CreateResult(ctx, &resultmodel.Result{}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("CreateResult err = %v, want invalid argument", err)
	}
	if err := service.CreateArtifact(ctx, &resultmodel.Artifact{}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("CreateArtifact err = %v, want invalid argument", err)
	}
	invalidCloseout := validTestResult()
	invalidCloseout.RuntimeCloseout = &runtimecontract.RuntimeResultCloseout{SchemaVersion: 2}
	if err := service.CreateResult(ctx, invalidCloseout); err == nil {
		t.Fatal("Result owner accepted unsupported Runtime closeout schema")
	}
}

func TestServiceCreateReadUpdate(t *testing.T) {
	ctx := context.Background()
	service := newService(memory.NewResultRepository(), memory.NewArtifactRepository())
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
	updatedArtifact, err := service.GetArtifact(ctx, artifact.ID)
	if err != nil || updatedArtifact.State != common.ArtifactStateAvailable {
		t.Fatalf("GetArtifact after update = %+v, err=%v", updatedArtifact, err)
	}
}

func TestServiceEnforcesArtifactParentScope(t *testing.T) {
	ctx := context.Background()
	resultRepo := memory.NewResultRepository()
	artifactRepo := memory.NewArtifactRepository()
	service := newService(resultRepo, artifactRepo)
	result := validTestResult()
	if err := service.CreateResult(ctx, result); err != nil {
		t.Fatal(err)
	}

	missingParent := validTestArtifact("missing-result")
	missingParent.ID = "artifact-missing-parent"
	if err := service.CreateArtifact(ctx, missingParent); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing parent artifact err = %v, want not found", err)
	}
	wrongScope := validTestArtifact(result.ID)
	wrongScope.ID = "artifact-wrong-scope"
	wrongScope.WorkspaceID = "ws-other"
	if err := service.CreateArtifact(ctx, wrongScope); !errors.Is(err, common.ErrNotFound) && !errors.Is(err, common.ErrConflict) {
		t.Fatalf("wrong scope artifact err = %v, want scoped rejection", err)
	}

	artifact := validTestArtifact(result.ID)
	if err := service.CreateArtifact(ctx, artifact); err != nil {
		t.Fatal(err)
	}
	stored, err := artifactRepo.Get(common.ContextWithRuntimeWorkspaceID(ctx, artifact.WorkspaceID), artifact.ID)
	if err != nil || stored.ResultID != result.ID {
		t.Fatalf("stored artifact = %+v, err=%v", stored, err)
	}
}

func TestResultTransitionMatrixRejectsTerminalRegression(t *testing.T) {
	ctx := context.Background()
	service := newService(memory.NewResultRepository(), memory.NewArtifactRepository())
	result := validTestResult()
	if err := service.CreateResult(ctx, result); err != nil {
		t.Fatal(err)
	}
	result.State = common.ResultStateDraft
	if err := service.UpdateResult(ctx, result); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("accepted -> draft err = %v, want conflict", err)
	}
	result.State = common.ResultStateSuperseded
	if err := service.UpdateResult(ctx, result); err != nil {
		t.Fatalf("accepted -> superseded: %v", err)
	}
	result.State = common.ResultStateRejected
	if err := service.UpdateResult(ctx, result); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("superseded -> rejected err = %v, want conflict", err)
	}
}

func TestServiceRejectsTerminalMaterialMutation(t *testing.T) {
	ctx := context.Background()
	service := newService(memory.NewResultRepository(), memory.NewArtifactRepository())
	item := validTestResult()
	item.TerminalMaterialHash = "result-material-sha256:immutable"
	item.TerminalDelivery = &runtimecontract.RuntimeTerminalDeliveryIdentity{TerminalDeliveryID: "delivery-immutable"}
	if err := service.CreateResult(ctx, item); err != nil {
		t.Fatal(err)
	}
	item.Summary = "mutated after terminal persistence"
	if err := service.UpdateResult(ctx, item); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("terminal material mutation err = %v, want conflict", err)
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
