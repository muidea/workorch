package runtime

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestOfflineOwnerAuditCompleteEventImagesMatchIndependentGates(t *testing.T) {
	for _, arm := range []string{"valid", "raw scope", "raw header", "converted header", "seed", "evidence", "budget", "dispatch", "owner before", "owner after", "nil empty", "revision"} {
		t.Run(arm, func(t *testing.T) {
			a := stateChangeAggregate(t)
			r := runtimeStateRecordForTerminalNonterminalRecoveryTest(t, a)
			originalR, originalA := r.Clone(), a.CloneRaw()
			for event := range 3 {
				err := runtimecontract.CompileRuntimeOwnerStateTransaction(func(gate func(runtimecontract.RuntimeOwnerStateSourceImage) error) error {
					// The owner invariant compiler is always new for each event.
					for phase := range 3 {
						raw, image := r.Clone(), a.CloneRaw()
						var record *runtimecontract.RuntimeStateRecord
						if phase != 1 {
							record = &raw
						} else {
							image.StateRevision++
						}
						if event == 1 && phase == 2 {
							switch arm {
							case "raw scope":
								raw.Facts.WorkspaceID = "other"
							case "raw header":
								raw.ResultFeedbackVersion = 0
							case "converted header":
								image.ResultFeedbackVersion = 0
							case "seed":
								image.SemanticPlanning.DefinitionSeed.PolicyHash = "same hash raw drift"
							case "evidence":
								image.EvidenceSequence++
							case "budget":
								image.ExecutionBudget.Policy.PolicyRevision = "drift"
							case "dispatch":
								image.CompletedDispatches = append(image.CompletedDispatches, runtimecontract.RuntimeDispatchRecord{})
							case "owner before":
								image.Decision.AuthoritySnapshot.RunID = "other"
							case "owner after":
								image.ProcessedEventIDs = []string{"duplicate", "duplicate"}
							case "nil empty":
								image.SemanticPlanning.CompletedBatchIDs = []string{}
							case "revision":
								image.StateRevision = 0
							}
						}
						got := gate(ownerStateSourceImageInCompiler(record, image))
						want := independentOwnerStateImage(record, image)
						if fmt.Sprint(got) != fmt.Sprint(want) {
							t.Fatalf("event %d phase %d first error differs: %v != %v", event, phase, got, want)
						}
						if got != nil {
							return got
						}
					}
					return nil
				})
				if (event != 1 || arm == "valid") && err != nil {
					t.Fatal(err)
				}
			}
			if !reflect.DeepEqual(originalR, r) || !reflect.DeepEqual(originalA, a) {
				t.Fatal("cross-event compile changed original images")
			}
		})
	}
}

func FuzzOfflineOwnerAuditCompleteEventImages(f *testing.F) {
	for arm := uint8(0); arm < 8; arm++ {
		f.Add(arm, "原文\x00")
	}
	f.Fuzz(func(t *testing.T, arm uint8, raw string) {
		if len(raw) > 128 {
			t.Skip()
		}
		a := stateChangeAggregate(t)
		r := runtimeStateRecordForTerminalNonterminalRecoveryTest(t, a)
		for event := range 2 {
			_ = runtimecontract.CompileRuntimeOwnerStateTransaction(func(gate func(runtimecontract.RuntimeOwnerStateSourceImage) error) error {
				image, record := a.CloneRaw(), r.Clone()
				if event == 1 {
					switch arm % 8 {
					case 0:
						record.Definition.StartupInputHash = raw
					case 1:
						record.Facts.DefinitionRevision = raw
					case 2:
						image.SemanticPlanning.DefinitionSeed.PolicyHash = raw
					case 3:
						image.ExecutionBudget.Policy.PolicyRevision = raw
					case 4:
						image.Decision.AuthoritySnapshot.RunID = common.RunID(raw)
					case 5:
						image.Facts.Coverage[0].TargetText = raw
					case 6:
						image.Facts.Focus.ID = raw
					case 7:
						image.ProcessedEventIDs = append(image.ProcessedEventIDs, raw)
					}
				}
				got := gate(ownerStateSourceImage(&record, image))
				want := independentOwnerStateImage(&record, image)
				if fmt.Sprint(got) != fmt.Sprint(want) {
					t.Fatalf("event fuzz differs: %v != %v", got, want)
				}
				return got
			})
		}
	})
}
