package events

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
)

func TestEventQueryNormalizedAppliesOwnerBounds(t *testing.T) {
	query, err := (EventQuery{
		Topic: " /run/state/changed ", ExcludeTopics: []string{" /a ", "/a", ""},
		OccurredAfter: " 2026-08-30T08:00:00+08:00 ", OccurredBefore: "2026-08-30T01:00:00Z",
	}).Normalized()
	if err != nil {
		t.Fatal(err)
	}
	if query.Topic != "/run/state/changed" || query.Limit != DefaultListLimit || len(query.ExcludeTopics) != 1 || query.ExcludeTopics[0] != "/a" ||
		query.OccurredAfter != "2026-08-30T00:00:00Z" || query.OccurredBefore != "2026-08-30T01:00:00Z" {
		t.Fatalf("normalized query = %+v", query)
	}
	for _, limit := range []int{-1, MaxListLimit + 1} {
		if _, err := (EventQuery{Limit: limit}).Normalized(); !errors.Is(err, common.ErrInvalidArgument) {
			t.Fatalf("limit %d err = %v, want invalid argument", limit, err)
		}
	}
}

func TestEventQueryNormalizedRejectsEveryUnboundedSurface(t *testing.T) {
	tooLong := strings.Repeat("x", MaxQueryStringBytes+1)
	queries := map[string]EventQuery{
		"workspace":       {WorkspaceID: common.WorkspaceID(tooLong)},
		"run":             {RunID: common.RunID(tooLong)},
		"topic":           {Topic: tooLong},
		"subject type":    {SubjectType: tooLong},
		"subject id":      {SubjectID: tooLong},
		"cursor":          {CursorAfterID: common.EventRecordID(tooLong)},
		"occurred after":  {OccurredAfter: tooLong},
		"occurred before": {OccurredBefore: tooLong},
		"excluded item":   {ExcludeTopics: []string{tooLong}},
	}
	for name, query := range queries {
		t.Run(name, func(t *testing.T) {
			if _, err := query.Normalized(); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("error = %v, want invalid argument", err)
			}
		})
	}
	if _, err := (EventQuery{ExcludeTopics: make([]string, MaxExcludeTopics+1)}).Normalized(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("exclude-topic count error = %v, want invalid argument", err)
	}
	if _, err := (EventQuery{Topic: string([]byte{0xff})}).Normalized(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("invalid UTF-8 error = %v, want invalid argument", err)
	}
}

func TestEventQueryNormalizedRejectsInvalidOrReversedTimeRange(t *testing.T) {
	for name, query := range map[string]EventQuery{
		"invalid after":  {OccurredAfter: "not-a-time"},
		"invalid before": {OccurredBefore: "2026-08-30"},
		"reversed":       {OccurredAfter: "2026-08-30T02:00:00Z", OccurredBefore: "2026-08-30T01:00:00Z"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := query.Normalized(); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("error = %v, want invalid argument", err)
			}
		})
	}
	if _, err := (EventQuery{OccurredAfter: "2026-08-30T01:00:00Z", OccurredBefore: "2026-08-30T01:00:00Z"}).Normalized(); err != nil {
		t.Fatalf("equal time bounds rejected: %v", err)
	}
}

func TestHighWatermarksCommandNormalizedCanonicalizesAndDeduplicates(t *testing.T) {
	command := HighWatermarksCommand{WorkspaceIDs: []common.WorkspaceID{" ws-b ", "ws-a", "ws-b"}}
	normalized, err := command.Normalized()
	if err != nil {
		t.Fatal(err)
	}
	want := []common.WorkspaceID{"ws-b", "ws-a"}
	if !slices.Equal(normalized.WorkspaceIDs, want) {
		t.Fatalf("normalized workspace IDs = %q, want %q", normalized.WorkspaceIDs, want)
	}
	if command.WorkspaceIDs[0] != " ws-b " {
		t.Fatalf("normalization mutated caller input = %q", command.WorkspaceIDs)
	}
	normalized.WorkspaceIDs[0] = "mutated"
	if command.WorkspaceIDs[0] != " ws-b " {
		t.Fatal("normalized workspace IDs alias caller input")
	}
}

func TestHighWatermarksCommandNormalizedRejectsInvalidItemsAndCount(t *testing.T) {
	tooMany := make([]common.WorkspaceID, MaxHighWatermarkScopes+1)
	for index := range tooMany {
		tooMany[index] = common.WorkspaceID(fmt.Sprintf("ws-%d", index))
	}
	for name, command := range map[string]HighWatermarksCommand{
		"empty list":    {},
		"blank item":    {WorkspaceIDs: []common.WorkspaceID{"ws-valid", " \t\n "}},
		"invalid UTF-8": {WorkspaceIDs: []common.WorkspaceID{common.WorkspaceID(string([]byte{0xff}))}},
		"oversized":     {WorkspaceIDs: []common.WorkspaceID{common.WorkspaceID(strings.Repeat("x", MaxHighWatermarkWorkspaceIDBytes+1))}},
		"too many":      {WorkspaceIDs: tooMany},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := command.Normalized(); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("error = %v, want invalid argument", err)
			}
		})
	}
}

func TestHighWatermarksResultValidateForRejectsScopeRevisionAndOrderDrift(t *testing.T) {
	command := HighWatermarksCommand{WorkspaceIDs: []common.WorkspaceID{"ws-b", "ws-a", "ws-b"}}
	valid := HighWatermarksResult{HighWatermarks: []WorkspaceHighWatermark{{WorkspaceID: "ws-a", Revision: 0}, {WorkspaceID: "ws-b", Revision: 7}}}
	if err := valid.ValidateFor(command); err != nil {
		t.Fatalf("valid high-watermark result rejected: %v", err)
	}
	for name, result := range map[string]HighWatermarksResult{
		"missing":      {HighWatermarks: valid.HighWatermarks[:1]},
		"extra":        {HighWatermarks: append(append([]WorkspaceHighWatermark(nil), valid.HighWatermarks...), WorkspaceHighWatermark{WorkspaceID: "ws-c", Revision: 1})},
		"duplicate":    {HighWatermarks: []WorkspaceHighWatermark{{WorkspaceID: "ws-a", Revision: 0}, {WorkspaceID: "ws-a", Revision: 7}}},
		"unrequested":  {HighWatermarks: []WorkspaceHighWatermark{{WorkspaceID: "ws-a", Revision: 0}, {WorkspaceID: "ws-c", Revision: 7}}},
		"negative":     {HighWatermarks: []WorkspaceHighWatermark{{WorkspaceID: "ws-a", Revision: -1}, {WorkspaceID: "ws-b", Revision: 7}}},
		"noncanonical": {HighWatermarks: []WorkspaceHighWatermark{{WorkspaceID: " ws-a ", Revision: 0}, {WorkspaceID: "ws-b", Revision: 7}}},
		"unsorted":     {HighWatermarks: []WorkspaceHighWatermark{{WorkspaceID: "ws-b", Revision: 7}, {WorkspaceID: "ws-a", Revision: 0}}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := result.ValidateFor(command); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("error = %v, want invalid argument", err)
			}
		})
	}
	clone := valid.Clone()
	clone.HighWatermarks[0].Revision = 99
	if valid.HighWatermarks[0].Revision != 0 {
		t.Fatal("high-watermark result clone aliases source")
	}
}

func TestApprovalResolutionRecordCommandIsEventstreamOwned(t *testing.T) {
	var contract eventcontract.Contract
	for _, candidate := range Contracts() {
		if candidate.Topic == TopicEventStreamCommandRecordApprovalResolution {
			contract = candidate
			break
		}
	}
	if contract.Owner != "eventstream" || !slices.Equal(contract.ReceiverOwners, []string{"eventstream"}) || contract.CommandClass != eventcontract.CommandClassWrite || contract.LanePolicy != eventcontract.LanePolicyState {
		t.Fatalf("approval resolution record contract = %+v", contract)
	}
	var message eventcontract.MessageContract
	for _, candidate := range MessageContracts() {
		if candidate.Topic == TopicEventStreamCommandRecordApprovalResolution {
			message = candidate
			break
		}
	}
	if message.Owner != contract.Owner || !slices.Equal(message.ReceiverOwners, contract.ReceiverOwners) || message.DefinitionPackage != definitionPackage {
		t.Fatalf("approval resolution record message = %+v", message)
	}
}

func TestApprovalResolutionFactRequiresTerminalOwnerObservation(t *testing.T) {
	valid := ApprovalResolutionFact{
		NotificationID: "notification-1", ResolutionID: "resolution-1", ApprovalID: "approval-1",
		WorkspaceID: "workspace-1", RunID: "run-1", PrincipalKind: "human_operator",
		PrincipalSubjectID: "operator", State: common.ApprovalStateApproved,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid approval resolution fact rejected: %v", err)
	}
	valid.State = common.ApprovalStatePending
	if err := valid.Validate(); err == nil {
		t.Fatal("non-terminal approval resolution fact accepted")
	}
}
