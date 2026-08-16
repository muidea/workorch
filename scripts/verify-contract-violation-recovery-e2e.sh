#!/usr/bin/env bash

# Contract correction is an SM-3 transaction, not a provider-selected follow-up
# flow. Exercise the typed rejection, one bounded correction and fail-closed
# restart behavior directly.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

GOCACHE="${GOCACHE:-/tmp/workorch-go-build}"
export GOCACHE

echo "=== Runtime candidate contract correction verification ==="

go test ./internal/modules/blocks/llm/biz/runtime \
  -run 'Test(DecodeCandidatePayloadRejectsAuthorityFields|DecodeCandidatePayloadRejectsLegacyCandidateWrappers|RequestCandidateForwardsCompleteSchemaInvalidDocuments|RequestCandidateClassifiesOutputLimitSchemaTruncation)$' \
  -count=1
go test ./internal/modules/kernel/orchestrator/biz/runtime \
  -run 'Test(CandidateRejectionCodeDispositionMatrixIsTotal|SealCandidateRejectionCorrectableRequiresObjectives|SealCandidateRejectionUnknownCodeBecomesContractDefect|RuntimeAggregateRequiresReducerTransitionAfterCandidateAttemptOneFailure)$' \
  -count=1
go test ./internal/modules/kernel/orchestrator/biz \
  -run 'Test(RuntimeMainlineRetriesSameCandidateContractOnce|RuntimeFileBackedSemanticCandidateReplayExhaustionSurvivesCASBoundaries|DriveRuntimeCompleteSchemaFailureStopsAfterSameContractRetry|RuntimeMainlinePartialReplayClosesAttemptThenSealsCorrectedRetry)$' \
  -count=1

echo "Runtime candidate contract correction verification passed"
