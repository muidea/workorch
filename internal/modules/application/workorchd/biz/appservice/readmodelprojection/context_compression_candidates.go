package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func ContextCompressionCandidates(bundle readmodelcore.ContextBundle) []readmodelcore.ContextCompressionCandidate {
	var ret []readmodelcore.ContextCompressionCandidate
	if len(bundle.RecentEvidence) > 8 {
		ret = append(ret, readmodelcore.ContextCompressionCandidate{
			SectionID:     "recent_evidence",
			Reason:        "recent evidence exceeds compact recovery window",
			Retention:     "retain latest evidence ids, states, summaries, and timestamps",
			SourceRefs:    ContextEvidenceRefs(bundle.RecentEvidence),
			SuggestedMode: "summarize",
		})
	}
	if len(bundle.TaskProgress.Items) > 10 {
		ret = append(ret, readmodelcore.ContextCompressionCandidate{
			SectionID:     "task_progress",
			Reason:        "task progress contains many low-level items",
			Retention:     "retain milestones, blockers, recovery, and accepted result refs",
			SourceRefs:    bundle.TaskProgress.EvidenceRefs,
			SuggestedMode: "milestone_summary",
		})
	}
	if candidate, ok := executionContextCompressionCandidate(bundle.Sections); ok {
		ret = append(ret, candidate)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}
