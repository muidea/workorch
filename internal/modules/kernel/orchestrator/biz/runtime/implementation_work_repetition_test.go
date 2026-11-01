package runtime

import (
	"context"
	magicevent "github.com/muidea/magicCommon/event"
	filebiz "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"os"
	"path/filepath"
	"testing"
)

func TestImplementationRepeatedStateGuardUsesRealEffectsAcrossReload(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	file, err := filebiz.New(hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = file.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { file.Teardown(ctx) })
	directory := t.TempDir()
	if err = os.WriteFile(filepath.Join(directory, "target.txt"), []byte("state A"), 0600); err != nil {
		t.Fatal(err)
	}
	owner := &implementationWorkRealFileFixture{hub: hub, directory: directory, maxRepeatedStates: 2, contentForRound: func(n int) string {
		if n%2 == 0 {
			return "state A"
		}
		return "state B"
	}}
	var a RuntimeAggregate
	for n := 1; n <= 4; n++ {
		a = implementationWorkPrivatePipeline(t, rc.RuntimeImplementationWorkPurposeEffect, []string{"target.txt"}, owner)
		if a.Facts.Failure != nil || len(a.ImplementationWork.Rounds) != n {
			t.Fatal("guard counted effective ordinal", n)
		}
		a = refreshImplementationHistoryTestOwners(t, owner, a)
	}
	before := a.CloneRaw()
	failed := implementationWorkPrivatePipeline(t, rc.RuntimeImplementationWorkPurposeStalled, []string{"target.txt"}, owner)
	if len(failed.ImplementationWork.Rounds) != 4 || failed.Facts.Failure == nil || failed.Facts.Failure.Code != "implementation_work_repeated_state_exhausted" || failed.Facts.Failure.Class != rc.FailureClassNoProgressExhausted {
		t.Fatal("cycle did not stop before extra effect", failed.Facts.Failure)
	}
	if len(failed.OperationReceipts) != len(before.OperationReceipts) || failed.Decision.State != rc.ConvergenceStateFailed {
		t.Fatal("cycle guard created effects or lost terminal")
	}
	if err = failed.Validate(); err != nil {
		t.Fatal("reload lost original repetition sources", err)
	}
	tampered := failed.CloneRaw()
	for n := range tampered.SemanticPlanning.History {
		s := &tampered.SemanticPlanning.History[n].Snapshot
		if s.Manifest == nil {
			continue
		}
		for j := range s.Manifest.ProofObligations {
			w := s.Manifest.ProofObligations[j].ImplementationWork
			if w != nil && w.PurposeChoiceOnly && !w.EffectAvailable {
				w.RepeatedWorkStates = 0
				w.EffectAvailable = true
				s.Manifest.Hash = rc.StableRuntimeSemanticPlanManifestHash(*s.Manifest)
			}
		}
	}
	if err = tampered.Validate(); err == nil {
		t.Fatal("forged reset accepted")
	}
}
