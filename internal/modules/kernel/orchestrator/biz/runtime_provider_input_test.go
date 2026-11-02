package orchestrator

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type providerInputCarrierCut struct {
	repository.ArtifactContinuationRepository
	cut error
}

func (r *providerInputCarrierCut) Update(ctx context.Context, item *rc.ArtifactContinuation) error {
	if r.cut != nil && item.State == common.ArtifactContinuationStateApplied {
		err := r.cut
		r.cut = nil
		return err
	}
	return r.ArtifactContinuationRepository.Update(ctx, item)
}

func TestRuntimeFrozenInputRecoversCarrierAndSessionCASWindows(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	repo, a := runtimeSemanticPreflightFixture(t)
	m := newRuntimeSemanticPreflightModule(t, hub, repo)
	cut := &providerInputCarrierCut{ArtifactContinuationRepository: memory.NewArtifactContinuationRepository(), cut: errors.New("crash after assembled bytes")}
	m.evidence.ArtifactContinuations = cut
	calls := 0
	observer := magicevent.NewSimpleObserver(rc.RuntimeModuleLLM, hub)
	observer.Subscribe(llmevents.TopicPreflightRuntimeCandidate, func(e magicevent.Event, r magicevent.Result) {
		calls++
		c := e.Data().(llmevents.PreflightRuntimeCandidateCommand)
		r.Set(withRuntimePreflightSnapshotForTest(c, llmevents.PreflightRuntimeCandidateResult{DocumentKind: c.DocumentKind, Feedback: runtimePreflightFeedbackForTest(t, c, "")}), nil)
	})
	command := llmevents.PreflightRuntimeCandidateCommand{WorkspaceID: a.Facts.WorkspaceID, RunID: a.Facts.RunID, DocumentKind: rc.RuntimeProviderDocumentSemanticPlan}
	expected := rc.RuntimeExpectedDocumentContract{LogicalRequestID: "frozen-cut"}
	if err := m.persistRuntimeProviderInput(ctx, a, "frozen-cut", []byte(`{"full":"原始冻结内容"}`)); err == nil {
		t.Fatal("cut did not interrupt")
	}
	// No session exists yet: both the pre-session and assembled-before-carrier
	// crash windows must reuse the original, already durable bytes.
	id := rc.StableRuntimeCandidateRequestContentID(providerInputContentKey("frozen-cut"))
	_, original, err := cut.GetAssembled(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	restarted := newRuntimeSemanticPreflightModule(t, hub, repo)
	restarted.evidence.ArtifactContinuations = cut
	raw, ref, err := restarted.prepareRuntimeProviderInput(ctx, a, "frozen-cut", expected, llmevents.PreflightRuntimeCandidateCommand{})
	if err != nil || !bytes.Equal(raw, []byte(original)) || calls != 0 {
		t.Fatalf("re-rendered frozen source: calls=%d err=%v", calls, err)
	}
	carrier, err := cut.Get(ctx, id)
	if err != nil || carrier.State != common.ArtifactContinuationStateApplied {
		t.Fatal("carrier not completed", err)
	}
	a.CandidateRequestSessions = []rc.RuntimeCandidateRequestSession{{LogicalRequestID: "frozen-cut", InputRef: ref}}
	again, _, err := restarted.prepareRuntimeProviderInput(ctx, a, "frozen-cut", expected, command)
	if err != nil || !bytes.Equal(raw, again) || calls != 0 {
		t.Fatal("session replay changed input", err)
	}
	a.CandidateRequestSessions[0].InputRef = &rc.RuntimeProviderInputRef{Bytes: ref.Bytes, Hash: rc.RuntimeProviderInputHash([]byte("wrong"))}
	if _, _, err := restarted.prepareRuntimeProviderInput(ctx, a, "frozen-cut", expected, command); err == nil {
		t.Fatal("corrupt session input accepted")
	}
	if err := cut.Delete(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, _, err := restarted.prepareRuntimeProviderInput(ctx, a, "frozen-cut", expected, command); err == nil || !strings.Contains(err.Error(), "original frozen payload missing") || calls != 0 {
		t.Fatal("missing frozen input silently rebuilt", err)
	}
}

func TestRuntimeFrozenInputScopeAndDeletionFence(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	repo, a := runtimeSemanticPreflightFixture(t)
	m := newRuntimeSemanticPreflightModule(t, hub, repo)
	raw := []byte(`{"full":"原始输入"}`)
	if err := m.persistRuntimeProviderInput(ctx, a, "scoped", raw); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"workspace", "run", "authority"} {
		t.Run(field, func(t *testing.T) {
			other := a
			switch field {
			case "workspace":
				other.Facts.WorkspaceID = "other"
			case "run":
				other.Facts.RunID = "other"
			case "authority":
				other.Decision.AuthoritySnapshot.ID = "other"
			}
			if _, _, err := m.prepareRuntimeProviderInput(ctx, other, "scoped", rc.RuntimeExpectedDocumentContract{}, llmevents.PreflightRuntimeCandidateCommand{}); err == nil {
				t.Fatal("foreign input accepted")
			}
		})
	}
	m.quiescingWorkspaces = map[common.WorkspaceID]deletioncontract.WorkspaceRuntimeQuiescenceIntent{a.Facts.WorkspaceID: {}}
	if err := m.persistRuntimeProviderInput(ctx, a, "after-deletion", raw); !errors.Is(err, common.ErrNotFound) {
		t.Fatal("frozen input bypassed deletion fence", err)
	}
}

func TestRuntimeFrozenInputsDoNotShareAcrossWorkspaces(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	repo, a := runtimeSemanticPreflightFixture(t)
	m := newRuntimeSemanticPreflightModule(t, hub, repo)
	b := a
	b.Facts.WorkspaceID = "different-workspace"
	var wg sync.WaitGroup
	for _, item := range []struct {
		a        runtime.RuntimeAggregate
		id, body string
	}{{a, "logical-a", "A原文"}, {b, "logical-b", "B原文"}} {
		wg.Add(1)
		go func(a runtime.RuntimeAggregate, id, body string) {
			defer wg.Done()
			if err := m.persistRuntimeProviderInput(ctx, a, id, []byte(body)); err != nil {
				t.Error(err)
				return
			}
			raw, _, err := m.prepareRuntimeProviderInput(ctx, a, id, rc.RuntimeExpectedDocumentContract{}, llmevents.PreflightRuntimeCandidateCommand{})
			if err != nil || string(raw) != body {
				t.Errorf("scope input differs: %v", err)
			}
		}(item.a, item.id, item.body)
	}
	wg.Wait()
}
