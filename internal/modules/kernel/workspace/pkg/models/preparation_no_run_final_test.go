package workspace

import (
	"strings"
	"testing"
)

func TestPreparationNoRunFinalRequiresWholeWorkspaceFence(t *testing.T) {
	for _, scenario := range []string{"exact", "version", "workspace", "revision", "active_run", "creation", "epoch", "pending", "settled"} {
		f := PreparationNoRunFinalFence{Version: PreparationNoRunFinalVersion, WorkspaceRevision: 2,
			Fence: WorkspacePreparationFence{Version: WorkspacePreparationFenceVersion, WorkspaceID: "ws", CreationEpoch: strings.Repeat("a", 32)}}
		switch scenario {
		case "version":
			f.Version = "future"
		case "workspace":
			f.Fence.WorkspaceID = "other"
		case "revision":
			f.WorkspaceRevision = 0
		case "active_run":
			f.ActiveRunID = "run"
		case "creation":
			f.Fence.CreationEpoch = ""
		case "epoch":
			f.Fence.PreparationEpoch = 1
		case "pending":
			f.Fence.Pending = &WorkspaceFreshIntentReservation{}
		case "settled":
			f.Fence.Settled = []WorkspaceFreshIntentSettlement{{}}
		}
		if f.validFor("ws") != (scenario == "exact") {
			t.Fatal("partial final fence accepted", scenario)
		}
	}
}

func TestPreparationNoRunFinalVersionWithoutMaterialIsRejected(t *testing.T) {
	w := preparationHistoryFixture()
	a := &w.StartupPreparation.Attempts[0]
	a.EvaluationVersion, a.ObservationSnapshotHash = PreparationCheckEvaluationVersion, "original-observation"
	a.NoRunEvaluationVersion = PreparationNoRunFinalVersion
	if a.Validate() == nil || w.StartupPreparation.Validate(w.ID) == nil || ValidatePreparationEvidencePersistence(w) == nil {
		t.Fatal("version alone substituted for complete seal/history/final-CAS proof")
	}
	// An entirely orphaned version must also fail the public persistence entry,
	// not just the archive loop (which has no entries to visit).
	a.RunHistoryEvidenceRef = ""
	w.PreparationRunHistoryEvidence = nil
	if ValidatePreparationEvidencePersistence(w) == nil {
		t.Fatal("orphan final version accepted")
	}
	if CompilePreparationNoRunFinalFence(nil, w, *a) != nil || CompilePreparationNoRunFinalFence(w, nil, *a) != nil {
		t.Fatal("missing actual CAS input fabricated final fence")
	}
}
