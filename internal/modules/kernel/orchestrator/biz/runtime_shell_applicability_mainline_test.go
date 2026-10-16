package orchestrator

import (
	"context"
	"fmt"
	"testing"

	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
)

// Real Shell bytes/process/receipt, File/Run/Result/Workspace owners and the
// production drive/private SM-2 path. Provider documents are scripted; this is
// a recovery oracle, not a public authoring or live-model acceptance result.
func TestDriveRuntimeShellApplicabilityCASRecoveryUsesOriginalReceipt(t *testing.T) {
	baseline := runValidationInputReviewMainline(t, "consistent", "")
	for _, cut := range []string{"before_shell_applicability", "after_shell_applicability"} {
		t.Run(cut, func(t *testing.T) {
			if got := runValidationInputReviewMainline(t, "consistent", cut); got != baseline {
				t.Fatalf("current-query recovery repeated provider submission: %d != %d", got, baseline)
			}
		})
	}
}

func TestDriveRuntimeShellApplicabilityResumeSettlesSubmittedRequestBeforeSampling(t *testing.T) {
	for _, fault := range []string{"", "before_cas", "after_cas"} {
		t.Run("settlement/"+fault, func(t *testing.T) {
			runValidationInputReviewMainline(t, "consistent", "after_assessment_submission", validationInputReviewMainlineOptions{Adapter: shellcommon.ValidationRecipeAdapterVersionedEntrypoint, ValidationCount: 1, SubmittedSettlementFault: fault})
		})
	}
}

type runtimeSubmittedSettlementFaultRepository struct {
	delegate runtime.RuntimeStateRepository
	afterCAS bool
	fired    bool
}

func (r *runtimeSubmittedSettlementFaultRepository) Load(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (runtime.RuntimeAggregate, error) {
	return r.delegate.Load(ctx, workspaceID, runID)
}

func (r *runtimeSubmittedSettlementFaultRepository) Save(ctx context.Context, aggregate runtime.RuntimeAggregate, expectedRevision uint64) error {
	cut := !r.fired && aggregate.Decision.Terminal != nil && aggregate.Decision.Terminal.Code == "provider_submission_unknown"
	if cut && !r.afterCAS {
		r.fired = true
		return fmt.Errorf("test interruption before submitted settlement CAS")
	}
	if err := r.delegate.Save(ctx, aggregate, expectedRevision); err != nil {
		return err
	}
	if cut {
		r.fired = true
		return fmt.Errorf("test response lost after submitted settlement CAS")
	}
	return nil
}
