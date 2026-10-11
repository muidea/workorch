package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

// 保留原完整匹配次序；不调用新身份预过滤路径。
func referenceAcceptedAttemptMatchesDispatch(attempt RuntimeCandidateAttemptRecord, dispatch RuntimeDispatchRecord) bool {
	if attempt.State != RuntimeCandidateAttemptStateAccepted ||
		ValidateRuntimeDispatchOperationTargetAuthorizationProjection(attempt.AdmissionTargetAuthorizations) != nil ||
		ValidateRuntimeDispatchOperationTargetAuthorizationProjection(dispatch.OperationTargetAuthorizations) != nil ||
		ValidateRuntimeDispatchOperationTargetAuthorizations(dispatch.Candidate, attempt.AdmissionTargetAuthorizations) != nil ||
		attempt.TargetAuthorizationHash != StableRuntimeDispatchOperationTargetAuthorizationsHash(attempt.AdmissionTargetAuthorizations) ||
		!RuntimeDispatchOperationTargetAuthorizationProjectionsEqual(attempt.AdmissionTargetAuthorizations, dispatch.OperationTargetAuthorizations) ||
		ValidateCoverageEvidenceBindingProjection(attempt.AdmissionCoverageBindings) != nil ||
		ValidateCoverageEvidenceBindingProjection(dispatch.CoverageBindings) != nil ||
		attempt.AdmissionCoverageBindingHash != StableRuntimeCoverageEvidenceBindingSnapshotHash(attempt.AdmissionCoverageBindings) ||
		dispatch.CoverageBindingHash != StableRuntimeCoverageEvidenceBindingSnapshotHash(dispatch.CoverageBindings) ||
		!RuntimeCoverageEvidenceBindingProjectionsEqual(attempt.AdmissionCoverageBindings, dispatch.CoverageBindings) ||
		!runtimeAcceptedCandidateAttemptValidationRecipeMatchesDispatch(attempt, dispatch) ||
		!runtimeImplementationWorkCommitmentsEqual(attempt.AdmissionImplementationWorkCommitment, dispatch.ImplementationWorkCommitment) ||
		!runtimeAcceptedCandidateAttemptRepairCommitmentMatchesDispatch(attempt, dispatch) {
		return false
	}
	return attempt.Transaction == dispatch.Transaction && attempt.DispatchID == dispatch.ID &&
		attempt.CandidateHash == dispatch.CandidateHash && attempt.ManifestHash == dispatch.CandidateManifestHash &&
		attempt.DispatchAuthorityHash == dispatch.DispatchAuthorityHash &&
		dispatch.DispatchAuthorityHash == StableRuntimeDispatchAuthorityHash(dispatch)
}

func admissionIdentityAttemptForDispatch(dispatch RuntimeDispatchRecord) RuntimeCandidateAttemptRecord {
	attempt := RuntimeCandidateAttemptRecord{
		Transaction: dispatch.Transaction, State: RuntimeCandidateAttemptStateAccepted,
		ManifestHash: dispatch.CandidateManifestHash, DispatchID: dispatch.ID, CandidateHash: dispatch.CandidateHash,
		AdmissionTargetAuthorizations: CloneRuntimeDispatchOperationTargetAuthorizations(dispatch.OperationTargetAuthorizations),
		TargetAuthorizationHash:       StableRuntimeDispatchOperationTargetAuthorizationsHash(dispatch.OperationTargetAuthorizations),
		AdmissionCoverageBindings:     CloneCoverageEvidenceBindings(dispatch.CoverageBindings),
		AdmissionCoverageBindingHash:  StableRuntimeCoverageEvidenceBindingSnapshotHash(dispatch.CoverageBindings),
		AdmissionValidationRecipes:    CloneRuntimeValidationRecipes(dispatch.ValidationRecipes),
		DispatchAuthorityHash:         dispatch.DispatchAuthorityHash,
	}
	if len(attempt.AdmissionValidationRecipes) != 0 {
		attempt.ValidationRecipeHash = StableRuntimeValidationRecipeSnapshotHash(attempt.AdmissionValidationRecipes)
	}
	return attempt
}

func assertAdmissionIdentityMatcherEquivalent(t testing.TB, attempt RuntimeCandidateAttemptRecord, dispatch RuntimeDispatchRecord) {
	t.Helper()
	before, err := json.Marshal(struct {
		Attempt  RuntimeCandidateAttemptRecord
		Dispatch RuntimeDispatchRecord
	}{attempt, dispatch})
	if err != nil {
		t.Fatal(err)
	}
	want := referenceAcceptedAttemptMatchesDispatch(attempt, dispatch)
	for range 2 {
		if got := RuntimeAcceptedCandidateAttemptMatchesDispatch(attempt, dispatch); got != want {
			t.Fatalf("匹配与原完整门禁不同: %t != %t", got, want)
		}
	}
	after, _ := json.Marshal(struct {
		Attempt  RuntimeCandidateAttemptRecord
		Dispatch RuntimeDispatchRecord
	}{attempt, dispatch})
	if !reflect.DeepEqual(before, after) {
		t.Fatal("只读 matcher 修改原输入")
	}
}

func mutateAdmissionIdentityMatcher(attempt *RuntimeCandidateAttemptRecord, dispatch *RuntimeDispatchRecord, value string, mode uint8) {
	switch mode % 13 {
	case 0:
		attempt.DispatchID = value
	case 1:
		attempt.Transaction.FocusID = value
	case 2:
		attempt.CandidateHash = value
	case 3:
		attempt.ManifestHash = value
	case 4:
		attempt.DispatchAuthorityHash = value
	case 5:
		attempt.TargetAuthorizationHash = value
	case 6:
		attempt.AdmissionTargetAuthorizations[0].OperationHash = value
	case 7:
		attempt.AdmissionCoverageBindingHash = value
	case 8:
		attempt.AdmissionCoverageBindings[0].DeclarationRef = value
	case 9:
		attempt.ValidationRecipeHash = value
	case 10:
		dispatch.CoverageBindingHash = value
	case 11:
		dispatch.ValidationRecipes[0].ID = value
	case 12:
		dispatch.DispatchAuthorityHash = value
		attempt.DispatchAuthorityHash = value
	}
}

func TestAdmissionIdentityPrefilterPreservesFullMatcher(t *testing.T) {
	for _, dispatch := range []RuntimeDispatchRecord{runtimeDispatchIntrinsicFixture(t, 0), runtimeManagedValidationDispatchIntrinsicFixture(t)} {
		attempt := admissionIdentityAttemptForDispatch(dispatch)
		if !referenceAcceptedAttemptMatchesDispatch(attempt, dispatch) || !RuntimeAcceptedCandidateAttemptMatchesDispatch(attempt, dispatch) {
			t.Fatal("有效精确配对未接受")
		}
		assertAdmissionIdentityMatcherEquivalent(t, attempt, dispatch)
		body, _ := json.Marshal(struct {
			Attempt  RuntimeCandidateAttemptRecord
			Dispatch RuntimeDispatchRecord
		}{attempt, dispatch})
		var reloaded struct {
			Attempt  RuntimeCandidateAttemptRecord
			Dispatch RuntimeDispatchRecord
		}
		if err := json.Unmarshal(body, &reloaded); err != nil {
			t.Fatal(err)
		}
		assertAdmissionIdentityMatcherEquivalent(t, reloaded.Attempt, reloaded.Dispatch)
	}
	dispatch := runtimeManagedValidationDispatchIntrinsicFixture(t)
	attempt := admissionIdentityAttemptForDispatch(dispatch)
	for mode := uint8(0); mode < 13; mode++ {
		changedAttempt, changedDispatch := attempt.Clone(), dispatch.Clone()
		mutateAdmissionIdentityMatcher(&changedAttempt, &changedDispatch, "wrong\x00原文", mode)
		assertAdmissionIdentityMatcherEquivalent(t, changedAttempt, changedDispatch)
		if RuntimeAcceptedCandidateAttemptMatchesDispatch(changedAttempt, changedDispatch) {
			t.Fatalf("非法配对被接受: %d", mode)
		}
	}
	assertAdmissionIdentityMatcherEquivalent(t, RuntimeCandidateAttemptRecord{}, RuntimeDispatchRecord{})
	for _, mutate := range []func(*RuntimeCandidateAttemptRecord, *RuntimeDispatchRecord){
		func(a *RuntimeCandidateAttemptRecord, _ *RuntimeDispatchRecord) {
			a.State = RuntimeCandidateAttemptStateFailed
		},
		func(a *RuntimeCandidateAttemptRecord, _ *RuntimeDispatchRecord) {
			a.AdmissionTargetAuthorizations = nil
		},
		func(a *RuntimeCandidateAttemptRecord, _ *RuntimeDispatchRecord) {
			a.AdmissionTargetAuthorizations = append(a.AdmissionTargetAuthorizations, a.AdmissionTargetAuthorizations[0])
		},
		func(a *RuntimeCandidateAttemptRecord, _ *RuntimeDispatchRecord) { a.AdmissionValidationRecipes = nil },
		func(_ *RuntimeCandidateAttemptRecord, d *RuntimeDispatchRecord) { d.ValidationRecipes = nil },
		func(a *RuntimeCandidateAttemptRecord, _ *RuntimeDispatchRecord) {
			a.AdmissionCoverageBindings = []CoverageEvidenceBinding{}
		},
		func(_ *RuntimeCandidateAttemptRecord, d *RuntimeDispatchRecord) { d.CoverageBindings = nil },
	} {
		changedAttempt, changedDispatch := attempt.Clone(), dispatch.Clone()
		mutate(&changedAttempt, &changedDispatch)
		assertAdmissionIdentityMatcherEquivalent(t, changedAttempt, changedDispatch)
		if RuntimeAcceptedCandidateAttemptMatchesDispatch(changedAttempt, changedDispatch) {
			t.Fatal("同身份的非法原投影绕过完整门禁")
		}
	}
	changed := attempt.Clone()
	changed.AdmissionCoverageBindings = append(changed.AdmissionCoverageBindings, changed.AdmissionCoverageBindings[0])
	assertAdmissionIdentityMatcherEquivalent(t, changed, dispatch)
	if RuntimeAcceptedCandidateAttemptMatchesDispatch(changed, dispatch) {
		t.Fatal("同身份的重复 raw binding 绕过完整门禁")
	}
	changed = attempt.Clone()
	changed.DispatchID = "later-changed"
	assertAdmissionIdentityMatcherEquivalent(t, changed, dispatch)
	if RuntimeAcceptedCandidateAttemptMatchesDispatch(changed, dispatch) {
		t.Fatal("后续调用复用上次成功")
	}
}

func referenceAdmissionIdentityLineage(active *RuntimeDispatchRecord, completed []RuntimeDispatchRecord, attempts []RuntimeCandidateAttemptRecord, bindings []RuntimeOperationGovernanceBinding) error {
	dispatches := append([]RuntimeDispatchRecord(nil), completed...)
	if active != nil {
		dispatches = append(dispatches, *active)
	}
	for _, dispatch := range dispatches {
		if dispatch.Candidate.Governance != nil {
			continue
		}
		accepted, resumed := 0, 0
		for _, attempt := range attempts {
			if referenceAcceptedAttemptMatchesDispatch(attempt, dispatch) {
				accepted++
			}
		}
		if dispatch.Candidate.StateChange != nil || dispatch.Candidate.Validation != nil {
			for _, raw := range bindings {
				binding := raw.Normalized()
				if binding.State == RuntimeOperationGovernanceBindingGranted && binding.Grant != nil &&
					binding.Grant.Normalized().ResumeDispatchID == dispatch.ID &&
					runtimeGovernanceResumeAdmissionInheritsAuthorityCommitments(binding, dispatch, dispatches) {
					resumed++
				}
			}
		}
		if accepted+resumed != 1 {
			return fmt.Errorf("runtime candidate dispatch %q requires exactly one accepted-attempt or operation governance-resume admission, got accepted=%d resume=%d", dispatch.ID, accepted, resumed)
		}
	}
	return nil
}

func TestAdmissionIdentityPrefilterPreservesLineageCountsAndFirstRejection(t *testing.T) {
	first := runtimeManagedValidationDispatchIntrinsicFixture(t)
	second := first.Clone()
	second.ID = "second-dispatch"
	second.DispatchAuthorityHash = StableRuntimeDispatchAuthorityHash(second)
	firstAttempt, secondAttempt := admissionIdentityAttemptForDispatch(first), admissionIdentityAttemptForDispatch(second)
	for _, attempts := range [][]RuntimeCandidateAttemptRecord{
		{firstAttempt, secondAttempt}, {secondAttempt, firstAttempt}, {firstAttempt}, {secondAttempt},
		{firstAttempt, firstAttempt, secondAttempt}, {firstAttempt, secondAttempt, secondAttempt}, nil,
	} {
		got := ValidateRuntimeDispatchAdmissionLineage(&second, []RuntimeDispatchRecord{first}, attempts, nil)
		want := referenceAdmissionIdentityLineage(&second, []RuntimeDispatchRecord{first}, attempts, nil)
		if cwiFocusBindingsError(got) != cwiFocusBindingsError(want) {
			t.Fatalf("lineage 拒绝原因／次序不同: %v != %v", got, want)
		}
	}
	_, _, _, blocked, _, binding := runtimeGovernanceDispatchHistoryFixture()
	source := first.Clone()
	source.State = RuntimeDispatchStateCommitted
	resume := source.Clone()
	resume.ID = "governance-resume"
	resume.State = RuntimeDispatchStateSealed
	resume.DispatchAuthorityHash = StableRuntimeDispatchAuthorityHash(resume)
	binding.OperationDispatchID = source.ID
	binding.Grant.ResumeDispatchID = resume.ID
	attempt := admissionIdentityAttemptForDispatch(source)
	got := ValidateRuntimeDispatchAdmissionLineage(&resume, []RuntimeDispatchRecord{source, blocked}, []RuntimeCandidateAttemptRecord{attempt}, []RuntimeOperationGovernanceBinding{binding})
	want := referenceAdmissionIdentityLineage(&resume, []RuntimeDispatchRecord{source, blocked}, []RuntimeCandidateAttemptRecord{attempt}, []RuntimeOperationGovernanceBinding{binding})
	if got != nil || want != nil {
		t.Fatalf("有效 governance resume lineage 未通过: %v / %v", got, want)
	}
}

func TestAdmissionIdentityPrefilterAvoidsUnrelatedPairAllocations(t *testing.T) {
	dispatch := runtimeManagedValidationDispatchIntrinsicFixture(t)
	attempt := admissionIdentityAttemptForDispatch(dispatch)
	attempt.DispatchID = "another-dispatch"
	original := testing.AllocsPerRun(20, func() { _ = referenceAcceptedAttemptMatchesDispatch(attempt, dispatch) })
	current := testing.AllocsPerRun(20, func() { _ = RuntimeAcceptedCandidateAttemptMatchesDispatch(attempt, dispatch) })
	if current != 0 || current >= original {
		t.Fatalf("无关配对未消除复制: %f >= %f", current, original)
	}
}

func FuzzAdmissionIdentityPrefilterMatchesOriginal(f *testing.F) {
	f.Add("", uint8(0))
	f.Add(" 原判据\r\n ", uint8(8))
	f.Add("\x00\xff", uint8(12))
	f.Fuzz(func(t *testing.T, value string, mode uint8) {
		dispatch := runtimeManagedValidationDispatchIntrinsicFixture(t)
		attempt := admissionIdentityAttemptForDispatch(dispatch)
		mutateAdmissionIdentityMatcher(&attempt, &dispatch, value, mode)
		assertAdmissionIdentityMatcherEquivalent(t, attempt, dispatch)
	})
}

func BenchmarkAdmissionIdentityPrefilter(b *testing.B) {
	// benchmark 使用相同完整 validation fixture；仅 ID 不同，原路径仍做全部投影复制。
	dispatch := runtimeManagedValidationDispatchIntrinsicFixture(b)
	attempt := admissionIdentityAttemptForDispatch(dispatch)
	attempt.DispatchID = "another-dispatch"
	for _, reference := range []bool{false, true} {
		name := "identity_prefilter"
		if reference {
			name = "reference"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				var matched bool
				if reference {
					matched = referenceAcceptedAttemptMatchesDispatch(attempt, dispatch)
				} else {
					matched = RuntimeAcceptedCandidateAttemptMatchesDispatch(attempt, dispatch)
				}
				if matched {
					b.Fatal("无关身份错误匹配")
				}
			}
		})
	}
}
