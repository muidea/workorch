package governance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	governanceevents "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/events"
	governancemodel "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/models"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func compactPromptKey(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(normalize(input)))
	return hex.EncodeToString(sum[:8])
}

func stableContextDigest(input workorcheventpayload.Payload) string {
	if len(input.Fields) == 0 {
		return ""
	}
	sum := sha256.Sum256([]byte(input.CanonicalString()))
	return hex.EncodeToString(sum[:8])
}

func containsRiskReason(input workorcheventpayload.Payload, target string) bool {
	target = normalize(target)
	if target == "" {
		return false
	}
	reasons, ok := input.StringSlice("riskReasons")
	if !ok {
		return false
	}
	for _, item := range reasons {
		if normalize(item) == target {
			return true
		}
	}
	return false
}

func containsAnyText(input string, terms []string) bool {
	input = normalize(input)
	for _, term := range terms {
		term = normalize(term)
		if term == "" {
			continue
		}
		if strings.Contains(input, term) {
			return true
		}
	}
	return false
}

func containsAnyToken(input []string, terms []string) bool {
	if len(input) == 0 || len(terms) == 0 {
		return false
	}
	set := map[string]struct{}{}
	for _, item := range input {
		item = normalize(item)
		if item != "" {
			set[item] = struct{}{}
		}
	}
	for _, term := range terms {
		term = normalize(term)
		if term == "" {
			continue
		}
		if _, ok := set[term]; ok {
			return true
		}
	}
	return false
}

func tokenSetFromText(input string) []string {
	input = normalize(input)
	if input == "" {
		return nil
	}
	parts := strings.FieldsFunc(input, func(r rune) bool {
		return r == ' ' || r == ',' || r == '，' || r == ';' || r == '；' || r == ':' || r == '：'
	})
	ret := make([]string, 0, len(parts))
	for _, item := range parts {
		item = strings.TrimSpace(item)
		if len(item) < 2 {
			continue
		}
		ret = append(ret, item)
	}
	return compactUniqueStrings(ret)
}

func compactUniqueStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := map[string]bool{}
	ret := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		ret = append(ret, item)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func (s *Service) loadGoalAndDoD(ctx context.Context, goalID common.GoalID, dodID common.DoDID) (*workspace.Goal, *workspace.DoD, error) {
	if s == nil || s.workspaceDefs == nil {
		return nil, nil, common.ErrInvalidArgument
	}
	var goal *workspace.Goal
	var dod *workspace.DoD
	if goalID != "" {
		item, err := s.workspaceDefs.GetGoal(ctx, goalID)
		if err != nil {
			return nil, nil, err
		}
		goal = item
	}
	if dodID != "" {
		item, err := s.workspaceDefs.GetDoD(ctx, dodID)
		if err != nil {
			return nil, nil, err
		}
		dod = item
	}
	return goal, dod, nil
}

func (s *Service) loadMemoryItems(ctx context.Context, workspaceID common.WorkspaceID) ([]*memorymodel.MemoryItem, error) {
	if s == nil || s.memoryItems == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	items, err := s.memoryItems.ListByWorkspaceID(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return items, nil
}

func (s *Service) publishGovernanceEvent(ctx context.Context, topic string, trace *governancemodel.AutoGovernanceTrace, escalationReason string) error {
	if s == nil || s.EventHub() == nil || trace == nil {
		return nil
	}
	cloned := *trace
	cloned.EvidenceRefs = append([]string(nil), trace.EvidenceRefs...)
	event := magicevent.NewEventWithContext(
		topic,
		"kernel/governance",
		governanceevents.NotificationDestination,
		magicevent.NewHeader(),
		ctx,
		governanceevents.GovernanceData{Trace: cloned, EscalationReason: escalationReason},
	)
	event.BindLaneKey(events.EventHubLaneKeyFor(governanceevents.Contracts(), topic, eventcontract.LaneScope{
		SubjectType: "auto_governance_trace",
		SubjectID:   string(trace.ID),
		WorkspaceID: string(trace.WorkspaceID),
		RunID:       string(trace.RunID),
		StepID:      string(trace.StepID),
	}))
	s.PostEvent(event)
	return nil
}

func isEnabled(value *bool) bool {
	return value != nil && *value
}

func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}
