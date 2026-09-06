"""Read-only comparison of owner-projected Runtime facts; never drive authority."""


def objects(value):
    return [item for item in value if isinstance(item, dict)] if isinstance(value, list) else []


def runtime_progress_facts(snapshot):
    workspace = snapshot.get("workspace") or {}
    run = snapshot.get("run") or {}
    workspace_id, run_id = workspace.get("id"), run.get("id")
    run_matches = bool(workspace_id and run_id and run.get("workspaceId") == workspace_id
                       and workspace.get("activeRunId") in (None, "", run_id))
    audit = (snapshot.get("acceptance") or {}).get("audit") or {}
    audit_matches = bool(run_matches and audit.get("workspaceId") == workspace_id
                         and audit.get("runId") == run_id)
    # Results in a workspace snapshot may include predecessor Runs. Never count
    # them as evidence that the currently observed Run is progressing/completed.
    accepted_results = sorted({item["id"] for item in objects(snapshot.get("results"))
                               if run_matches and item.get("id") and item.get("state") == "accepted"
                               and item.get("runId") == run_id and item.get("workspaceId") == workspace_id})
    coverage = objects(audit.get("coverage")) if audit_matches else []
    receipts = objects(audit.get("operationReceipts")) if audit_matches else []
    evidence = objects(audit.get("acceptedEvidence")) if audit_matches else []
    return {
        "workspaceId": workspace_id, "runId": run_id,
        "runScopeValid": run_matches, "auditScopeValid": audit_matches,
        "stateRevision": int(audit.get("stateRevision") or 0) if audit_matches else 0,
        "acceptedResultIds": accepted_results,
        "acceptedEvidenceIds": sorted({item["id"] for item in evidence if item.get("id")}),
        # Recovery fences/repeated receipt delivery for one execution are not
        # independent effects. Only owner-accepted durable changes count here.
        "durableEffectExecutionKeys": sorted({item["executionKey"] for item in receipts
            if item.get("executionKey") and item.get("outcome") == "accepted" and item.get("changed") is True
            and item.get("observedEffectScope") in {"workspace_durable", "remote_durable"}}),
        "satisfiedCWIIds": sorted({item["id"] for item in coverage
                                   if item.get("id") and item.get("state") == "satisfied"}),
        "openCWIIds": sorted({item["id"] for item in coverage
                              if item.get("id") and item.get("state") != "satisfied"}),
        "coverageClosed": bool(coverage) and all(item.get("id") and item.get("state") == "satisfied" for item in coverage),
    }


def evaluate_workspace_progress(summary, previous=None, min_accepted_results=1):
    workspace, run = summary.get("workspace") or {}, summary.get("run") or {}
    counts, blockers = summary.get("counts") or {}, summary.get("blockers") or {}
    heartbeat, convergence = summary.get("heartbeat") or {}, summary.get("convergenceStatus") or {}
    facts = summary.get("runtimeProgress") or {}
    before = (previous or {}).get("runtimeProgress") or {}
    workspace_id, run_id = summary.get("workspaceId"), run.get("id")
    state = run.get("state") or ""
    same_run = bool(previous and workspace_id == previous.get("workspaceId") and run_id
                    and run_id == (previous.get("run") or {}).get("id"))
    comparable = same_run and facts.get("auditScopeValid") is True and before.get("auditScopeValid") is True
    comparable = comparable and int(facts.get("stateRevision") or 0) >= int(before.get("stateRevision") or 0)
    fields = ("acceptedResultIds", "acceptedEvidenceIds", "durableEffectExecutionKeys", "satisfiedCWIIds")
    delta = {key: sorted(set(facts.get(key) or []) - set(before.get(key) or [])) if comparable else [] for key in fields}
    observed_progress = any(delta.values())
    previous_heartbeat = (previous or {}).get("heartbeat") or {}
    heartbeat_changed = same_run and any(heartbeat.get(key) and heartbeat.get(key) != previous_heartbeat.get(key)
        for key in ("latestEventRevision", "latestLLMUsageAt", "latestEffectiveProgressAt"))
    revision_delta = int(facts.get("stateRevision") or 0) - int(before.get("stateRevision") or 0) if comparable else 0
    activity_observed = state == "active" and (heartbeat_changed or revision_delta > 0)
    decision_blocker = bool(int(counts.get("openDecisions") or 0) > 0 and (
        workspace.get("blockingTicketId") or run.get("blockingTicketId") or blockers.get("openDecisions")
        or workspace.get("state") == "waiting_decision" or state == "blocked_on_decision"))
    approval_blocker = bool(int(counts.get("pendingApprovals") or 0) > 0 and (
        workspace.get("blockingApprovalId") or run.get("blockingApprovalId") or blockers.get("pendingApprovals")
        or workspace.get("state") == "waiting_approval" or state == "blocked_on_approval"))
    completed = (state == "completed" and facts.get("auditScopeValid") is True and facts.get("coverageClosed") is True
                 and len(facts.get("acceptedResultIds") or []) >= min_accepted_results
                 and not decision_blocker and not approval_blocker)
    unhealthy = any(heartbeat.get(key) or convergence.get(key) for key in ("stalled", "ineffectiveRetry"))
    issues = []
    if state in {"failed", "cancelled", "superseded"}:
        mode = "terminal_without_success"
        issues.append(f"current Run is {state}; prior effects/results do not establish continuing progress")
    elif unhealthy:
        mode = "stalled_or_ineffective_retry"
        issues.append("owner projection marks this Run stalled or ineffectiveRetry")
    elif completed:
        mode = "completed_with_accepted_evidence"
    elif decision_blocker:
        mode = "governed_decision_blocker"
    elif approval_blocker:
        mode = "governed_approval_blocker"
    elif state == "active" and observed_progress:
        mode = "positive_progress_with_owner_evidence"
    else:
        mode = "active_without_observed_progress" if activity_observed else "no_observed_positive_progress"
        issues.append("no same-Run owner-evidence/effect/CWI delta or fully accepted completion observed in this interval")
    if decision_blocker and convergence.get("decisionEligibilityDenied"):
        issues.append("open decision conflicts with owner-projected decision eligibility: "
                      + str(convergence.get("decisionNotEligibleReason") or ""))
    return {
        "workspaceId": workspace_id, "workspaceState": workspace.get("state"),
        "runId": run_id, "runState": state, "mode": mode,
        "comparisonAvailable": bool(comparable), "activityObserved": bool(activity_observed),
        "observedProgress": observed_progress, "completed": completed,
        "positiveProgress": bool(not issues and (completed or (state == "active" and observed_progress))),
        "delta": delta, "stateRevisionDelta": revision_delta,
        "decisionEligibility": {"denied": bool(convergence.get("decisionEligibilityDenied")),
            "reason": convergence.get("decisionNotEligibleReason"), "stepId": convergence.get("decisionNotEligibleStepId")},
        "pass": not issues, "issues": issues,
    }
