package shell

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestProjectExplicitDeadlineUsesSharedOwnerBounds(t *testing.T) {
	for _, adapter := range []string{modulecommon.ValidationRecipeAdapterProjectConjunction, modulecommon.ValidationRecipeAdapterProjectFailureDiagnostic, modulecommon.ValidationRecipeAdapterProjectMultiFailureDiagnostic} {
		for _, ms := range []int64{0, 1000, 60000, 90000, 600000} {
			recipe := rc.RuntimeValidationRecipe{AdapterRef: adapter, InputReviewProtocolRef: rc.RuntimeValidationInputReviewProtocol, TimeoutMillis: ms}
			if !shellProjectRecipeDeclared(recipe) {
				t.Fatalf("declared deadline rejected: %s/%d", adapter, ms)
			}
		}
		for _, ms := range []int64{-1, 1, 999, 600001} {
			recipe := rc.RuntimeValidationRecipe{AdapterRef: adapter, InputReviewProtocolRef: rc.RuntimeValidationInputReviewProtocol, TimeoutMillis: ms}
			if shellProjectRecipeDeclared(recipe) {
				t.Fatalf("invalid deadline admitted: %s/%d", adapter, ms)
			}
		}
	}
}

func TestProjectNewPreparationDeclaresActualSharedDeadline(t *testing.T) {
	recipe, requirement, _ := conjunctiveSourceRequirement(t)
	recipe.AdapterRef = modulecommon.ValidationRecipeAdapterProjectMultiFailureDiagnostic
	root := t.TempDir()
	writeNativeStructureSource(t, root, "go.mod", "module fixture\ngo 1.26\n")
	manifest := shellTestWorkspaceManifestBinding("deadline", []string{"go.mod"})
	for _, ms := range []int64{0, 60000, 300000, 600000} {
		recipe.TimeoutMillis = ms
		surfaces := shellValidationSurfaces([]rc.RuntimeValidationRecipe{recipe}, []rc.RuntimeValidationRecipe{recipe}, nil, []rc.RuntimeValidationProofRequirement{requirement}, manifest, "policy", root)
		if len(surfaces) != 1 || len(surfaces[0].ProofRequirementPreparations) != 1 {
			t.Fatal("missing original input preparation")
		}
		prep := surfaces[0].ProofRequirementPreparations[0]
		want := fmt.Sprintf("timeoutMillis=%d", ms)
		if ms == 0 {
			want = "未显式声明 timeoutMillis（0）"
		}
		if !strings.Contains(prep.ContentRequirements, want) || len(prep.CheckAttestations) == 0 || !strings.Contains(prep.ContentRequirements, "all-of") {
			t.Fatal("preparation lost declared deadline or all-of obligations", prep)
		}
	}
}

func TestProjectNewMethodRetainsOldSealedDeadlineAndAllStages(t *testing.T) {
	request, execution := projectExecutionFixture(t, false)
	old := request
	oldRecipes := []rc.RuntimeValidationRecipe{{ID: request.ValidationRecipeID, Command: request.Command, Args: request.Args, TimeoutMillis: request.TimeoutMillis}}
	oldHash := rc.StableRuntimeValidationRecipeSnapshotHash(oldRecipes)
	request.TimeoutMillis = 300000
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	deadline, _ := ctx.Deadline()
	result, err := NewRuntimeAdapter().executeShellProjectConjunction(ctx, request, execution)
	if err != nil || result.Conjunction == nil || !result.Conjunction.Complete || len(result.Conjunction.Stages) != 4 || result.Result.TimeoutMillis != 300000 || !result.Deadline.Equal(deadline) {
		t.Fatalf("new bounded method failed to preserve physical all-of/parent: %+v err=%v", result, err)
	}
	if old.TimeoutMillis != 60000 || oldRecipes[0].TimeoutMillis != 60000 || rc.StableRuntimeValidationRecipeSnapshotHash(oldRecipes) != oldHash || !reflect.DeepEqual(request.Args, old.Args) {
		t.Fatal("new method rewrote historical recipe or dropped original stages")
	}
}

func TestProjectExplicitLongDeadlineDoesNotResetEarlierParent(t *testing.T) {
	request, execution := projectExecutionFixture(t, false)
	request.TimeoutMillis = 90000
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	parent, _ := ctx.Deadline()
	result, err := NewRuntimeAdapter().executeShellProjectConjunction(ctx, request, execution)
	if err != nil || result.Conjunction == nil || !result.Conjunction.Complete {
		t.Fatalf("legal explicit timeout rejected: %v", err)
	}
	if !result.Deadline.Equal(parent) || result.Result.TimeoutMillis != 90000 {
		t.Fatalf("earlier parent/declared budget lost: %+v", result)
	}
}
