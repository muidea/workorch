package workorchruntimecontract

import (
	"errors"
	"testing"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeProviderTransportBackoffAndDueAt(t *testing.T) {
	wantDelays := map[int]time.Duration{
		0: 2 * time.Second,
		1: 2 * time.Second,
		2: 4 * time.Second,
		3: 8 * time.Second,
		4: 16 * time.Second,
		5: 32 * time.Second,
		6: 60 * time.Second,
		7: 60 * time.Second,
		8: 60 * time.Second,
	}
	for attempt, want := range wantDelays {
		if got := RuntimeProviderTransportBackoff(attempt); got != want {
			t.Fatalf("attempt %d backoff = %v, want %v", attempt, got, want)
		}
	}

	if DefaultMaxProviderTransportAttempts != 32 || DefaultMaxProviderTransportAttemptsPerRequest != 8 {
		t.Fatalf("unexpected transport retry defaults: run=%d perRequest=%d", DefaultMaxProviderTransportAttempts, DefaultMaxProviderTransportAttemptsPerRequest)
	}

	observedAt := "2026-11-23T08:55:33.123456789Z"
	due, err := RuntimeProviderTransportRetryDueAt(observedAt, 3)
	if err != nil {
		t.Fatal(err)
	}
	if want := "2026-11-23T08:55:41.123456789Z"; due != want {
		t.Fatalf("dueAt = %q, want %q", due, want)
	}

	for _, tc := range []struct {
		name       string
		observedAt string
		attempt    int
	}{
		{name: "empty observedAt", attempt: 1},
		{name: "invalid observedAt", observedAt: "not-a-timestamp", attempt: 1},
		{name: "zero attempt", observedAt: observedAt, attempt: 0},
		{name: "negative attempt", observedAt: observedAt, attempt: -1},
	} {
		if _, err := RuntimeProviderTransportRetryDueAt(tc.observedAt, tc.attempt); err == nil {
			t.Fatalf("%s unexpectedly succeeded", tc.name)
		}
	}
}

func TestRuntimeCandidateRequestSessionTransportRetryDueAtValidationAndEquality(t *testing.T) {
	expected := RuntimeExpectedDocumentContract{
		OperationsContextEncoding: RuntimeOperationsContextEncoding,
		SchemaVersion:             RuntimeExpectedDocumentSchemaCurrent,
		DocumentKind:              RuntimeProviderDocumentOperations,
		ManifestHash:              "manifest-1",
		WireContractHash:          "wire-1",
		LogicalRequestID:          "logical-transport-1",
		StablePrefixHash:          "prefix-1",
		MaxOutputTokens:           1024,
		MaxRecoveryAttempts:       1,
	}
	expected.Hash = StableRuntimeExpectedDocumentHash(expected)
	dueAt := "2026-11-23T08:55:35.000000000Z"
	base := RuntimeCandidateRequestSession{
		LogicalRequestID:    expected.LogicalRequestID,
		DocumentKind:        expected.DocumentKind,
		ExpectedDocument:    &expected,
		Transaction:         AuthorityTransaction{AuthoritySnapshotID: "authority-1"},
		StablePrefixHash:    expected.StablePrefixHash,
		ManifestHash:        expected.ManifestHash,
		State:               RuntimeCandidateRequestSessionTransportPending,
		NextSequence:        2,
		TransportOutcomes:   1,
		TransportRetryDueAt: dueAt,
		LastPhysicalCallID:  "physical-1",
		LastOutcome: RuntimeCandidateRequestOutcome{
			Class:     RuntimeCandidateRequestOutcomeProviderTransient,
			Code:      "provider_http_status_503",
			Signature: "provider-http-status-503",
		},
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid transport pending session: %v", err)
	}

	missingDue := base.Clone()
	missingDue.TransportRetryDueAt = ""
	if err := missingDue.Validate(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("missing dueAt error = %v, want invalid argument", err)
	}

	invalidDue := base.Clone()
	invalidDue.TransportRetryDueAt = "bad-timestamp"
	if err := invalidDue.Validate(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("invalid dueAt error = %v, want invalid argument", err)
	}

	zeroOutcomes := base.Clone()
	zeroOutcomes.TransportOutcomes = 0
	zeroOutcomes.NextSequence = 1
	if err := zeroOutcomes.Validate(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("zero transport outcomes error = %v, want invalid argument", err)
	}

	for _, tc := range []struct {
		name  string
		build func(RuntimeCandidateRequestSession) RuntimeCandidateRequestSession
	}{
		{
			name: "prepared",
			build: func(s RuntimeCandidateRequestSession) RuntimeCandidateRequestSession {
				s.State = RuntimeCandidateRequestSessionPrepared
				s.NextSequence = 1
				s.TransportOutcomes = 0
				s.LastPhysicalCallID = ""
				s.LastOutcome = RuntimeCandidateRequestOutcome{}
				return s
			},
		},
		{
			name: "recovery_pending",
			build: func(s RuntimeCandidateRequestSession) RuntimeCandidateRequestSession {
				s.State = RuntimeCandidateRequestSessionRecoveryPending
				s.DocumentRecoveryOutcomes = 1
				s.TransportOutcomes = 0
				s.NextSequence = 2
				s.LastDocumentFraming = &RuntimeProviderDocumentFraming{
					Disposition:          RuntimeProviderDocumentIncomplete,
					ExpectedDocumentHash: expected.Hash,
					ResponseHash:         "resp-1",
					ResponseBytes:        4,
					Code:                 RuntimeProviderDocumentCodeIncompleteJSON,
					Signature:            RuntimeProviderDocumentSignatureIncompleteJSON,
				}
				s.LastOutcome = RuntimeCandidateRequestOutcome{
					Class:     RuntimeCandidateRequestOutcomeDocumentRecovery,
					Code:      RuntimeProviderDocumentCodeIncompleteJSON,
					Signature: RuntimeProviderDocumentSignatureIncompleteJSON,
				}
				return s
			},
		},
		{
			name: "document_complete",
			build: func(s RuntimeCandidateRequestSession) RuntimeCandidateRequestSession {
				s.State = RuntimeCandidateRequestSessionDocumentComplete
				s.ProviderSubmitted = true
				s.LastOutcome = RuntimeCandidateRequestOutcome{Class: RuntimeCandidateRequestOutcomeDocumentComplete}
				return s
			},
		},
		{
			name: "terminal",
			build: func(s RuntimeCandidateRequestSession) RuntimeCandidateRequestSession {
				s.State = RuntimeCandidateRequestSessionTerminal
				return s
			},
		},
	} {
		t.Run("non_pending_"+tc.name, func(t *testing.T) {
			candidate := tc.build(base.Clone())
			if err := candidate.Validate(); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("%s with non-empty TransportRetryDueAt error = %v, want invalid argument", tc.name, err)
			}
			candidate.TransportRetryDueAt = ""
			if err := candidate.Validate(); err != nil {
				t.Fatalf("%s without TransportRetryDueAt error = %v", tc.name, err)
			}
		})
	}

	other := base.Clone()
	other.TransportRetryDueAt = "2026-11-23T08:55:36.000000000Z"
	if equalRuntimeCandidateRequestSession(&base, &other) {
		t.Fatal("equalRuntimeCandidateRequestSession ignored TransportRetryDueAt difference")
	}
	if cloned := cloneRuntimeCandidateRequestSessionRaw(base); cloned.TransportRetryDueAt != dueAt {
		t.Fatalf("cloneRuntimeCandidateRequestSessionRaw lost TransportRetryDueAt: %q", cloned.TransportRetryDueAt)
	}
	if snap := snapshotRuntimeCandidateRequestSessionFromStored(base); snap.TransportRetryDueAt != dueAt {
		t.Fatalf("snapshotRuntimeCandidateRequestSessionFromStored lost TransportRetryDueAt: %q", snap.TransportRetryDueAt)
	}
}
