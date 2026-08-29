package workorchruntimecontract

import (
	"encoding/json"
	"testing"
)

func TestValidateRuntimeOperationGovernanceExecutionBindingsRequiresExactResumeProjection(t *testing.T) {
	binding, execution, _, _ := runtimeOperationGovernanceLineageFixture(t)

	if err := ValidateRuntimeOperationGovernanceExecutionBindings([]RuntimeOperationGovernanceBinding{binding}, []RuntimeOperationExecutionRecord{execution}); err != nil {
		t.Fatalf("valid governance resume projection: %v", err)
	}

	t.Run("missing resume", func(t *testing.T) {
		if err := ValidateRuntimeOperationGovernanceExecutionBindings([]RuntimeOperationGovernanceBinding{binding}, nil); err == nil {
			t.Fatal("granted governance binding without a resume execution was accepted")
		}
	})

	t.Run("identical copied projection", func(t *testing.T) {
		copied := execution.Clone()
		if err := ValidateRuntimeOperationGovernanceExecutionBindings(
			[]RuntimeOperationGovernanceBinding{binding},
			[]RuntimeOperationExecutionRecord{execution, copied},
		); err != nil {
			t.Fatalf("identical active/completed execution projections were rejected: %v", err)
		}
	})

	t.Run("inconsistent duplicate projection", func(t *testing.T) {
		inconsistent := execution.Clone()
		inconsistent.ClaimedAt = RuntimeOperationClaimMarkerGovernanceEffectStarted
		if err := inconsistent.Validate(); err != nil {
			t.Fatalf("inconsistent projection fixture: %v", err)
		}
		if err := ValidateRuntimeOperationGovernanceExecutionBindings(
			[]RuntimeOperationGovernanceBinding{binding},
			[]RuntimeOperationExecutionRecord{execution, inconsistent},
		); err == nil {
			t.Fatal("inconsistent projections of one governance execution identity were accepted")
		}
	})

	t.Run("orphan governance marker", func(t *testing.T) {
		if err := ValidateRuntimeOperationGovernanceExecutionBindings(nil, []RuntimeOperationExecutionRecord{execution}); err == nil {
			t.Fatal("governance execution marker without a granted binding was accepted")
		}
	})
}

func TestValidateRuntimeOperationGovernanceGrantLineageRejectsResolutionAndEvidenceDrift(t *testing.T) {
	binding, _, resolution, evidence := runtimeOperationGovernanceLineageFixture(t)
	bindings := []RuntimeOperationGovernanceBinding{binding}

	if err := ValidateRuntimeOperationGovernanceGrantLineage(bindings, []RuntimeGovernanceResolutionRecord{resolution}, []EvidenceFact{evidence}, nil); err != nil {
		t.Fatalf("valid governance grant lineage: %v", err)
	}
	if err := ValidateRuntimeOperationGovernanceGrantLineage(nil, []RuntimeGovernanceResolutionRecord{resolution}, []EvidenceFact{evidence}, nil); err != nil {
		t.Fatalf("valid non-operation governance resolution/evidence lineage: %v", err)
	}

	t.Run("missing resolution", func(t *testing.T) {
		if err := ValidateRuntimeOperationGovernanceGrantLineage(bindings, nil, []EvidenceFact{evidence}, nil); err == nil {
			t.Fatal("governance grant without a durable resolution was accepted")
		}
	})

	t.Run("resolution without accepted evidence", func(t *testing.T) {
		if err := ValidateRuntimeOperationGovernanceGrantLineage(nil, []RuntimeGovernanceResolutionRecord{resolution}, nil, nil); err == nil {
			t.Fatal("continue resolution without accepted governance evidence was accepted")
		}
	})

	t.Run("accepted evidence without resolution", func(t *testing.T) {
		if err := ValidateRuntimeOperationGovernanceGrantLineage(nil, nil, []EvidenceFact{evidence}, nil); err == nil {
			t.Fatal("accepted governance evidence without a continue resolution was accepted")
		}
	})

	t.Run("wrong resolution", func(t *testing.T) {
		wrong := resolution
		wrong.ResolutionID = "resolution-other"
		if err := ValidateRuntimeOperationGovernanceGrantLineage(bindings, []RuntimeGovernanceResolutionRecord{wrong}, []EvidenceFact{evidence}, nil); err == nil {
			t.Fatal("governance grant linked to a different resolution was accepted")
		}
	})

	t.Run("evidence refs drift", func(t *testing.T) {
		drifted := evidence
		drifted.EvidenceRefs = []string{"approval://other"}
		if err := ValidateRuntimeOperationGovernanceGrantLineage(bindings, []RuntimeGovernanceResolutionRecord{resolution}, []EvidenceFact{drifted}, nil); err == nil {
			t.Fatal("governance grant with drifted accepted evidence refs was accepted")
		}
	})

	t.Run("grant resolution hash drift", func(t *testing.T) {
		drifted := binding.Clone()
		drifted.Grant.ResolutionHash = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		if err := ValidateRuntimeOperationGovernanceGrantLineage([]RuntimeOperationGovernanceBinding{drifted}, []RuntimeGovernanceResolutionRecord{resolution}, []EvidenceFact{evidence}, nil); err == nil {
			t.Fatal("governance grant with a different resolution hash was accepted")
		}
	})

	t.Run("evidence resolution hash drift", func(t *testing.T) {
		drifted := evidence
		drifted.ContentHash = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		if err := ValidateRuntimeOperationGovernanceGrantLineage(bindings, []RuntimeGovernanceResolutionRecord{resolution}, []EvidenceFact{drifted}, nil); err == nil {
			t.Fatal("governance evidence with a different resolution hash was accepted")
		}
	})

	t.Run("coordinated grant and evidence replacement", func(t *testing.T) {
		driftedBinding := binding.Clone()
		driftedBinding.Grant.ResolutionHash = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		driftedEvidence := evidence
		driftedEvidence.ContentHash = driftedBinding.Grant.ResolutionHash
		if err := ValidateRuntimeOperationGovernanceGrantLineage([]RuntimeOperationGovernanceBinding{driftedBinding}, []RuntimeGovernanceResolutionRecord{resolution}, []EvidenceFact{driftedEvidence}, nil); err == nil {
			t.Fatal("grant and evidence replacement without the exact durable resolution was accepted")
		}
	})
}

func TestRuntimeGovernanceResolutionRecordSealsCompleteNormalizedResolution(t *testing.T) {
	_, _, record, _ := runtimeOperationGovernanceLineageFixture(t)
	if err := ValidateRuntimeGovernanceResolutionRecords([]RuntimeGovernanceResolutionRecord{record}); err != nil {
		t.Fatalf("valid governance resolution record: %v", err)
	}

	missingHash := record.Clone()
	missingHash.ResolutionHash = ""
	if err := ValidateRuntimeGovernanceResolutionRecords([]RuntimeGovernanceResolutionRecord{missingHash}); err == nil {
		t.Fatal("governance resolution record without a hash was accepted")
	}

	drifted := record.Clone()
	drifted.Summary = "replacement approval summary"
	if err := ValidateRuntimeGovernanceResolutionRecords([]RuntimeGovernanceResolutionRecord{drifted}); err == nil {
		t.Fatal("governance resolution record payload drift without a matching hash was accepted")
	}

	cloned := record.Clone()
	cloned.EvidenceRefs[0] = "approval://replacement"
	if record.EvidenceRefs[0] == cloned.EvidenceRefs[0] {
		t.Fatal("governance resolution record clone shared evidence refs")
	}

	payload, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded RuntimeGovernanceResolutionRecord
	if err := json.Unmarshal(payload, &reloaded); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRuntimeGovernanceResolutionRecords([]RuntimeGovernanceResolutionRecord{reloaded}); err != nil ||
		!SameRuntimeGovernanceResolutionRecord(record, reloaded) {
		t.Fatalf("governance resolution record did not survive persistence exactly: reloaded=%+v err=%v", reloaded, err)
	}
}

func runtimeOperationGovernanceLineageFixture(t *testing.T) (RuntimeOperationGovernanceBinding, RuntimeOperationExecutionRecord, RuntimeGovernanceResolutionRecord, EvidenceFact) {
	t.Helper()

	content := "governed content"
	operation := RuntimeOperation{
		ID:         "op-1",
		Kind:       StepKindFileWrite,
		TargetRefs: []string{"target.txt"},
		Parameters: []OperationParameter{{Name: "content", String: &content}},
	}
	execution := RuntimeOperationExecutionRecord{
		OperationID:         "op-1",
		OperationHash:       StableRuntimeOperationHash(operation),
		ExecutionKey:        "exec-1",
		ExpectedEffectScope: RuntimeEffectWorkspaceDurable,
		IdempotencyClass:    RuntimeOperationIdempotencyAtMostOnce,
		State:               OperationExecutionClaimed,
		Fence:               5,
		ClaimedAt:           RuntimeOperationClaimMarkerGovernanceGranted,
	}
	execution.AuthorityHash = StableRuntimeOperationExecutionAuthorityHash(execution)
	if err := execution.Validate(); err != nil {
		t.Fatalf("governance resume execution fixture: %v", err)
	}

	ownerResolution := RuntimeGovernanceResolution{
		ResolutionID:        "resolution-1",
		ApprovalID:          "approval-1",
		WorkspaceID:         "workspace-1",
		RunID:               "run-1",
		AuthoritySnapshotID: "governance-snapshot-1",
		DispatchID:          "governance-dispatch-1",
		DependencyKind:      string(GovernanceApproval),
		SubjectRef:          "target.txt",
		Outcome:             RuntimeGovernanceResolutionOutcomeContinue,
		Summary:             "approval granted",
		EvidenceRefs:        []string{"approval://1"},
	}
	resolution := RuntimeGovernanceResolutionRecordFromResolution(ownerResolution)
	grant := RuntimeOperationGovernanceGrant{
		ResolutionID:          ownerResolution.ResolutionID,
		ResolutionHash:        resolution.ResolutionHash,
		ApprovalID:            ownerResolution.ApprovalID,
		ExecutionKey:          execution.ExecutionKey,
		GovernanceFence:       4,
		ResumeFence:           execution.Fence,
		ResumeDispatchID:      "resume-dispatch-1",
		DependencyKind:        ownerResolution.DependencyKind,
		SubjectRef:            ownerResolution.SubjectRef,
		RequestedAction:       "file/write",
		RiskLevel:             "high",
		AuthorizationHash:     "authorization-hash",
		GovernanceReceiptHash: "governance-receipt-hash",
		EvidenceRefs:          append([]string(nil), ownerResolution.EvidenceRefs...),
	}
	binding := RuntimeOperationGovernanceBinding{
		ApprovalID:             grant.ApprovalID,
		GovernanceDispatchID:   ownerResolution.DispatchID,
		GovernanceSnapshotID:   ownerResolution.AuthoritySnapshotID,
		OperationDispatchID:    "operation-dispatch-1",
		OperationID:            execution.OperationID,
		ExecutionKey:           execution.ExecutionKey,
		Fence:                  grant.GovernanceFence,
		ExpectedEffectScope:    execution.ExpectedEffectScope,
		IdempotencyClass:       execution.IdempotencyClass,
		ExecutionAuthorityHash: execution.AuthorityHash,
		DependencyKind:         grant.DependencyKind,
		SubjectRef:             grant.SubjectRef,
		RequestedAction:        grant.RequestedAction,
		RiskLevel:              grant.RiskLevel,
		AuthorizationHash:      grant.AuthorizationHash,
		GovernanceReceiptHash:  grant.GovernanceReceiptHash,
		State:                  RuntimeOperationGovernanceBindingGranted,
		Grant:                  &grant,
	}
	if err := ValidateRuntimeOperationGovernanceBindings([]RuntimeOperationGovernanceBinding{binding}); err != nil {
		t.Fatalf("governance binding fixture: %v", err)
	}

	evidence, err := RuntimeGovernanceEvidenceFactForResolution(ownerResolution, "runtime-evidence-00000000000000000001")
	if err != nil {
		t.Fatalf("governance evidence fixture: %v", err)
	}
	return binding, execution, resolution, evidence
}
