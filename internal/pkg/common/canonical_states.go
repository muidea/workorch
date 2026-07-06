package common

func CanonicalRunState(state RunState) RunState {
	switch state {
	case RunStatePending:
		return RunStatePending
	case RunStateActive:
		return RunStateActive
	case RunStateBlockedOnDecision:
		return RunStateBlockedOnDecision
	case RunStateBlockedOnApproval:
		return RunStateBlockedOnApproval
	case RunStateCompleted:
		return RunStateCompleted
	case RunStateFailed:
		return RunStateFailed
	case RunStateCancelled:
		return RunStateCancelled
	case RunStateSuperseded:
		return RunStateSuperseded
	default:
		return state
	}
}

func CanonicalStepState(state StepState) StepState {
	switch state {
	case StepStateExecuting:
		return StepStateExecuting
	case StepStateSucceeded:
		return StepStateSucceeded
	case StepStatePending:
		return StepStatePending
	case StepStateFailed:
		return StepStateFailed
	case StepStateSkipped:
		return StepStateSkipped
	case StepStateCancelled:
		return StepStateCancelled
	default:
		return state
	}
}

func CanonicalWorkspaceOwnerState(state WorkspaceState) WorkspaceState {
	switch state {
	case WorkspaceStateBlockedOnApproval:
		return WorkspaceStateBlockedOnApproval
	case WorkspaceStateSuperseded:
		return WorkspaceStateSuperseded
	case WorkspaceStateActive:
		return WorkspaceStateActive
	default:
		return state
	}
}

func CanonicalDecisionTicketState(state DecisionTicketState) DecisionTicketState {
	switch state {
	case DecisionTicketStateOpen:
		return DecisionTicketStateOpen
	case DecisionTicketStateResolved:
		return DecisionTicketStateResolved
	case DecisionTicketStateSuperseded:
		return DecisionTicketStateSuperseded
	default:
		return state
	}
}

func CanonicalApprovalState(state ApprovalState) ApprovalState {
	switch state {
	case ApprovalStatePending:
		return ApprovalStatePending
	case ApprovalStateApproved:
		return ApprovalStateApproved
	case ApprovalStateRejected:
		return ApprovalStateRejected
	case ApprovalStateSuperseded:
		return ApprovalStateSuperseded
	default:
		return state
	}
}

func CanonicalResultState(state ResultState) ResultState {
	switch state {
	case ResultStateDraft:
		return ResultStateDraft
	case ResultStateAccepted:
		return ResultStateAccepted
	case ResultStateRejected:
		return ResultStateRejected
	case ResultStateSuperseded:
		return ResultStateSuperseded
	default:
		return state
	}
}

func CanonicalToolCallState(state ToolCallState) ToolCallState {
	switch state {
	case ToolCallStatePending:
		return ToolCallStatePending
	case ToolCallStateRunning:
		return ToolCallStateRunning
	case ToolCallStateCompleted:
		return ToolCallStateCompleted
	case ToolCallStateFailed:
		return ToolCallStateFailed
	default:
		return state
	}
}

func CanonicalPatchState(state PatchState) PatchState {
	switch state {
	case PatchStateCreated:
		return PatchStateCreated
	case PatchStatePreviewed:
		return PatchStatePreviewed
	case PatchStateApplied:
		return PatchStateApplied
	case PatchStateFailed:
		return PatchStateFailed
	case PatchStateReverted:
		return PatchStateReverted
	default:
		return state
	}
}

func CanonicalValidationResultState(state ValidationResultState) ValidationResultState {
	switch state {
	case ValidationResultStatePassed:
		return ValidationResultStatePassed
	case ValidationResultStateFailed:
		return ValidationResultStateFailed
	case ValidationResultStateInconclusive:
		return ValidationResultStateInconclusive
	case ValidationResultStateWarning:
		return ValidationResultStateWarning
	default:
		return state
	}
}

func CanonicalArtifactContinuationState(state ArtifactContinuationState) ArtifactContinuationState {
	switch state {
	case ArtifactContinuationStateActive:
		return ArtifactContinuationStateActive
	case ArtifactContinuationStateSealed:
		return ArtifactContinuationStateSealed
	case ArtifactContinuationStateAborted:
		return ArtifactContinuationStateAborted
	case ArtifactContinuationStatePlanning:
		return ArtifactContinuationStatePlanning
	case ArtifactContinuationStateChunking:
		return ArtifactContinuationStateChunking
	case ArtifactContinuationStateAssembling:
		return ArtifactContinuationStateAssembling
	case ArtifactContinuationStateApplying:
		return ArtifactContinuationStateApplying
	case ArtifactContinuationStateApplied:
		return ArtifactContinuationStateApplied
	case ArtifactContinuationStateFailed:
		return ArtifactContinuationStateFailed
	default:
		return state
	}
}
