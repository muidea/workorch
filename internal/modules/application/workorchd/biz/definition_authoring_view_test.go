package biz

import (
	"testing"

	decision "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

// This predicate is only the delivery/lifecycle gate. The caller must still
// compare the complete ticket identity to the Workspace-owned candidate.
func TestAuthoringTicketDeliveryCompleteRejectsRetiredAndPartialResolution(t *testing.T) {
	fresh := func() *decision.Ticket {
		return &decision.Ticket{
			State:                          common.DecisionTicketStateOpen,
			CreatedNotificationPublishedAt: "2026-10-19T00:00:00Z",
			DefinitionClarification:        &decision.DefinitionClarificationBinding{},
		}
	}
	if !authoringTicketDeliveryComplete(fresh()) {
		t.Fatal("open ticket with owner delivery marker should pass the lifecycle gate")
	}
	if authoringTicketDeliveryComplete(nil) {
		t.Fatal("nil ticket cannot prove completed delivery")
	}
	for _, test := range []struct {
		name   string
		mutate func(*decision.Ticket)
	}{
		{"missing binding", func(v *decision.Ticket) { v.DefinitionClarification = nil }},
		{"missing delivery marker", func(v *decision.Ticket) { v.CreatedNotificationPublishedAt = "" }},
		{"resolved", func(v *decision.Ticket) { v.State = common.DecisionTicketStateResolved }},
		{"superseded", func(v *decision.Ticket) { v.State = common.DecisionTicketStateSuperseded }},
		{"unknown lifecycle", func(v *decision.Ticket) { v.State = common.DecisionTicketState("unknown") }},
		{"step scope", func(v *decision.Ticket) { v.StepID = "step" }},
		{"resolved time", func(v *decision.Ticket) { v.ResolvedAt = "time" }},
		{"resolved actor", func(v *decision.Ticket) { v.ResolvedBy = "actor" }},
		{"resolution notification", func(v *decision.Ticket) { v.ResolutionNotificationPublishedAt = "time" }},
		{"resolution id", func(v *decision.Ticket) { v.DefinitionClarification.ResolutionID = "resolution" }},
		{"resolution binding", func(v *decision.Ticket) { v.DefinitionClarification.ResolutionBindingHash = "hash" }},
		{"resolution candidate id", func(v *decision.Ticket) { v.DefinitionClarification.ResolutionCandidateID = "candidate" }},
		{"resolution candidate revision", func(v *decision.Ticket) { v.DefinitionClarification.ResolutionCandidateRevision = 1 }},
		{"resolution candidate hash", func(v *decision.Ticket) { v.DefinitionClarification.ResolutionCandidateHash = "hash" }},
		{"admission result", func(v *decision.Ticket) { v.DefinitionClarification.AdmissionResultHash = "hash" }},
		{"definition revision", func(v *decision.Ticket) { v.DefinitionClarification.DefinitionRevision = "revision" }},
		{"startup input", func(v *decision.Ticket) { v.DefinitionClarification.StartupInputHash = "hash" }},
		{"replacement id", func(v *decision.Ticket) { v.DefinitionClarification.ReplacementIntentID = "replacement" }},
		{"replacement hash", func(v *decision.Ticket) { v.DefinitionClarification.ReplacementIntentHash = "hash" }},
		{"successor revision", func(v *decision.Ticket) { v.DefinitionClarification.SuccessorDefinitionRevision = "revision" }},
		{"successor input", func(v *decision.Ticket) { v.DefinitionClarification.SuccessorStartupInputHash = "hash" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			ticket := fresh()
			test.mutate(ticket)
			if authoringTicketDeliveryComplete(ticket) {
				t.Fatal("retired or partially resolved ticket advertised as complete")
			}
		})
	}
}
