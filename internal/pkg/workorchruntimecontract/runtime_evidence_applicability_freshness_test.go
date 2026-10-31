package workorchruntimecontract

import "testing"

func TestEvidenceApplicabilityFreshConsumerSeparatesHistoryAndGeneration(t *testing.T) {
	set := *applicabilityIdentityObservation(10, "original").CurrentDependencies
	fact := EvidenceFact{ID: "evidence", ApplicabilityDependencies: &RuntimeEvidenceApplicabilityDependencies{ProtocolRef: RuntimeEvidenceApplicabilityDependenciesProtocol, OwnerRevisionSet: set}}
	original := applicabilityIdentityObservation(10, "original")
	original.Request.SourceEvidenceHash = StableRuntimeEvidenceFactHash(fact)
	original.Request.ReconciliationRequestHash = "generation-1"
	original.Request = original.Request.WithID()
	observations := []RuntimeEvidenceApplicabilityObservation{original}
	if !RuntimeEvidenceHasFreshApplicability(fact, observations, "1", "generation-1") ||
		RuntimeEvidenceHasFreshApplicability(fact, observations, "2", "generation-1") ||
		RuntimeEvidenceHasFreshApplicability(fact, observations, "1", "generation-2") ||
		RuntimeEvidenceHasFreshApplicability(fact, observations, "", "generation-1") {
		t.Fatal("freshness gate conflated old receipt/revision/Resume generation")
	}
	fresh := original.CloneRaw()
	fresh.Request.IssuedAtStateRevision = 20
	fresh.RecordedAtStateRevision = 21
	fresh.Request.ReconciliationRequestHash = "generation-2"
	fresh.Request = fresh.Request.WithID()
	observations = append(observations, fresh)
	if !RuntimeEvidenceHasFreshApplicability(fact, observations, "1", "generation-2") || StableRuntimeEvidenceApplicabilitySemanticHash(observations) != StableRuntimeEvidenceApplicabilitySemanticHash(observations[:1]) {
		t.Fatal("matching fresh read changed semantics or failed new-generation fence")
	}
	stale := fresh.CloneRaw()
	stale.Request.IssuedAtStateRevision = 30
	stale.RecordedAtStateRevision = 31
	stale.CurrentDependencies.Entries[0].Revision = "changed"
	*stale.CurrentDependencies = stale.CurrentDependencies.WithHash()
	stale.Request = stale.Request.WithID()
	observations = append(observations, stale, fresh)
	if RuntimeEvidenceHasFreshApplicability(fact, observations, "1", "generation-2") {
		t.Fatal("restoration revived previously stale source")
	}
	if !RuntimeEvidenceHasFreshApplicability(fact, observations[:1], "1", "generation-1") {
		t.Fatal("later stale result polluted original immutable assessment prefix")
	}
	if !RuntimeEvidenceHasFreshApplicability(EvidenceFact{}, nil, "", "") {
		t.Fatal("legacy receipt was silently upgraded to new observation protocol")
	}
}
