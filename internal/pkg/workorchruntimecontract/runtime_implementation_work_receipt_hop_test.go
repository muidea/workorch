package workorchruntimecontract

import (
	"strings"
	"testing"
)

func TestImplementationWorkReceiptEvidenceRequiresExactExecutionDispatchHop(t *testing.T) {
	ledger, _ := implementationWorkLedgerFixture(t)
	set := implementationWorkReceiptFixture(t, &ledger)
	round := ledger.Rounds[0].Clone()
	round.CompletedReceiptSetHash = ""
	round.GovernanceDispatchIDs = []string{"previous-authorized-dispatch"}
	round.Hash = StableRuntimeImplementationWorkRoundHash(round)

	member := set.Members[0]
	member.Evidence.DispatchID = round.GovernanceDispatchIDs[0]
	member.Execution.EvidenceFactHash = StableRuntimeEvidenceFactHash(member.Evidence)
	// 两个 hop 均在同轮授权历史中，且各自 hash 完整；证据仍必须来自该 execution 的 exact hop。
	if _, err := ValidateRuntimeImplementationWorkReceiptSet(round, []RuntimeImplementationWorkReceiptMember{member}); err == nil {
		t.Fatal("effect evidence borrowed another authorized dispatch hop")
	}

	member.Evidence.DispatchID = round.DispatchID
	member.Execution.EvidenceFactHash = StableRuntimeEvidenceFactHash(member.Evidence)
	if _, err := ValidateRuntimeImplementationWorkReceiptSet(round, []RuntimeImplementationWorkReceiptMember{member}); err != nil {
		t.Fatalf("exact current execution dispatch rejected: %v", err)
	}

	member.Execution.ExecutionKey = StableRuntimeOperationExecutionKey(round.WorkspaceID, round.RunID, round.GovernanceDispatchIDs[0], member.Operation.ID)
	member.Receipt.ExecutionKey = member.Execution.ExecutionKey
	member.Execution.OwnerReceiptHash = StableRuntimeOperationReceiptHash(member.Receipt)
	member.Execution.AuthorityHash = StableRuntimeOperationExecutionAuthorityHash(member.Execution)
	member.DispatchID = round.GovernanceDispatchIDs[0]
	member.Evidence.DispatchID = round.GovernanceDispatchIDs[0]
	member.Execution.EvidenceFactHash = StableRuntimeEvidenceFactHash(member.Evidence)
	if _, err := ValidateRuntimeImplementationWorkReceiptSet(round, []RuntimeImplementationWorkReceiptMember{member}); err != nil {
		t.Fatalf("exact original execution dispatch rejected: %v", err)
	}
}

func TestImplementationWorkReceiptProjectionDerivesHopFromExecutionNotEvidence(t *testing.T) {
	ledger, _ := implementationWorkLedgerFixture(t)
	set := implementationWorkReceiptFixture(t, &ledger)
	round := ledger.Rounds[0].Clone()
	round.CompletedReceiptSetHash = ""
	round.GovernanceDispatchIDs = []string{"previous-authorized-dispatch"}
	round.Hash = StableRuntimeImplementationWorkRoundHash(round)
	member := set.Members[0]
	member.Evidence.DispatchID = round.GovernanceDispatchIDs[0]
	member.Execution.EvidenceFactHash = StableRuntimeEvidenceFactHash(member.Evidence)
	dispatch := RuntimeDispatchRecord{ID: round.DispatchID, Candidate: RuntimeCandidate{StateChange: &StateChangeCandidate{Operations: []StateChangingOperation{{Operation: member.Operation}}}}, OperationExecutions: []RuntimeOperationExecutionRecord{member.Execution}}
	facts := RuntimeFacts{WorkspaceID: round.WorkspaceID, RunID: round.RunID, Evidence: EvidenceLedger{Accepted: []EvidenceFact{member.Evidence}}}
	members, err := RuntimeImplementationWorkReceiptMembers(dispatch, []RuntimeOperationReceipt{member.Receipt}, facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 1 || members[0].DispatchID != round.DispatchID {
		t.Fatal("projector copied effect dispatch from evidence instead of original execution")
	}
	if _, err := ValidateRuntimeImplementationWorkReceiptSet(round, members); err == nil {
		t.Fatal("coordinately rehashed evidence from another authorized hop accepted")
	}

	// 治理保留原 execution key；effect 归属于 exact resume fence 对应的授权 dispatch。
	member.Execution.ExecutionKey = StableRuntimeOperationExecutionKey(round.WorkspaceID, round.RunID, round.GovernanceDispatchIDs[0], member.Operation.ID)
	member.Execution.Fence = 2
	member.Receipt.ExecutionKey, member.Receipt.Fence = member.Execution.ExecutionKey, member.Execution.Fence
	member.Execution.OwnerReceiptHash = StableRuntimeOperationReceiptHash(member.Receipt)
	member.Execution.AuthorityHash = StableRuntimeOperationExecutionAuthorityHash(member.Execution)
	member.Evidence.DispatchID = round.DispatchID
	member.Execution.EvidenceFactHash = StableRuntimeEvidenceFactHash(member.Evidence)
	dispatch.OperationExecutions = []RuntimeOperationExecutionRecord{member.Execution}
	facts.Evidence.Accepted = []EvidenceFact{member.Evidence}
	grant := RuntimeOperationGovernanceGrant{ResolutionID: "resolution-hop", ResolutionHash: strings.Repeat("a", 64), ApprovalID: "approval-hop", ExecutionKey: member.Execution.ExecutionKey,
		GovernanceFence: 1, ResumeFence: 2, ResumeDispatchID: round.DispatchID, DependencyKind: string(GovernanceApproval), SubjectRef: "work-hop", RequestedAction: "execute", AuthorizationHash: "authorization-hop", GovernanceReceiptHash: "blocked-hop"}
	bindings := []RuntimeOperationGovernanceBinding{{State: RuntimeOperationGovernanceBindingGranted, OperationID: member.Operation.ID, ExecutionKey: member.Execution.ExecutionKey, Grant: &grant}}
	members, err = RuntimeImplementationWorkReceiptMembers(dispatch, []RuntimeOperationReceipt{member.Receipt}, facts, bindings)
	if err != nil || len(members) != 1 || members[0].DispatchID != round.DispatchID {
		t.Fatalf("exact resume hop projection: %+v err=%v", members, err)
	}
	if _, err := ValidateRuntimeImplementationWorkReceiptSet(round, members); err != nil {
		t.Fatalf("authorized original-key resumed effect rejected: %v", err)
	}
	members, err = RuntimeImplementationWorkReceiptMembers(dispatch, []RuntimeOperationReceipt{member.Receipt}, facts, append(bindings, bindings[0]))
	if err == nil {
		t.Fatal("duplicate original governance hop accepted")
	}
}
