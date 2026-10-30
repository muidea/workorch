package runtime

import (
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestEvidenceApplicabilityOwnerCompilerMatchesColdGate(t *testing.T) {
	base, _ := applicabilityTransitionFixture(t)
	c, _ := NewRuntimeController(&ownerQueryFallbackRepository{aggregate: base})
	for _, arm := range []string{"valid", "header", "receipt", "budget", "evidence", "scope", "duplicate", "first_error", "closed", "restart"} {
		t.Run(arm, func(t *testing.T) {
			// Warm the same complete raw image before independently mutating it.
			if _, _, _, err := c.CompileEvidenceApplicabilityRead(base); err != nil {
				t.Fatal(err)
			}
			a := base.CloneRaw()
			switch arm {
			case "header":
				a.ResultFeedbackVersion = 0
			case "receipt":
				a.OperationReceipts[0].AfterHash = "same identity altered receipt"
			case "budget":
				a.ExecutionBudget.Policy.PolicyRevision = "current drift"
			case "evidence":
				a.Facts.Evidence.Accepted[0].Summary += "raw drift"
			case "scope":
				a.Facts.WorkspaceID = "other-workspace"
			case "duplicate":
				a.OperationReceipts = append(a.OperationReceipts, a.OperationReceipts[0])
			case "first_error":
				a.ResultFeedbackVersion = 0
				a.OperationReceipts[0].AfterHash = "second error"
			case "closed":
				c.ReleaseHistoricalCompilation()
			case "restart":
				c, _ = NewRuntimeController(&ownerQueryFallbackRepository{aggregate: base})
			}
			before := a.CloneRaw()
			want, wantSource, wantRequired, wantErr := CompileRuntimeEvidenceApplicabilityRead(a)
			got, source, required, err := c.CompileEvidenceApplicabilityRead(a)
			if fmt.Sprint(err) != fmt.Sprint(wantErr) || required != wantRequired || !reflect.DeepEqual(got, want) || !reflect.DeepEqual(source, wantSource) {
				t.Fatalf("owner/cold complete result or first error differs: %v / %v", err, wantErr)
			}
			if err == nil && required {
				source.Operation.TargetRefs[0] = "returned mutation"
				source.OriginalDependencies.Entries[0].Revision = "returned mutation"
				source.ApplicabilitySource.OwnerRevisionSet.Entries[0].Revision = "returned mutation"
			}
			if !reflect.DeepEqual(a, before) {
				t.Fatal("compile or returned query data changed authority")
			}
		})
	}
	var absent *RuntimeController
	if _, _, _, err := absent.CompileEvidenceApplicabilityRead(base); err != common.ErrInvalidArgument {
		t.Fatal("nil compiler accepted")
	}
}

func TestEvidenceApplicabilityOwnerCompilerConcurrentWorkspaceIsolation(t *testing.T) {
	base, _ := applicabilityTransitionFixture(t)
	c, _ := NewRuntimeController(&ownerQueryFallbackRepository{aggregate: base})
	var group sync.WaitGroup
	for i := range 12 {
		group.Go(func() {
			a := base.CloneRaw()
			if i%2 != 0 {
				a.Facts.WorkspaceID = "unrelated-workspace"
				a.Facts.RunID = "unrelated-run"
			}
			want, wantSource, wantRequired, wantErr := CompileRuntimeEvidenceApplicabilityRead(a)
			got, source, required, err := c.CompileEvidenceApplicabilityRead(a)
			if fmt.Sprint(err) != fmt.Sprint(wantErr) || required != wantRequired || !reflect.DeepEqual(got, want) || !reflect.DeepEqual(source, wantSource) {
				t.Errorf("concurrent scope changed complete result: %v / %v", err, wantErr)
			}
		})
	}
	group.Wait()
}
