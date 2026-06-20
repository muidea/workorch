#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

cd "$ROOT_DIR"

go test ./internal/modules/kernel/orchestrator/biz \
  -run 'TestResultSubsystemRecordsAcceptedSliceResultWithoutFinalCompletion|TestEnsureRunCoverageWorkItemsBuildsAcceptanceSlicesFromGenericPolicy' \
  -count=1

go test ./internal/pkg/workorchdefaults \
  -run 'TestEmbeddedSoftwareDevTemplateDoDItemsCoverScopeEvidenceAndRisk' \
  -count=1

echo "acceptance slice result and final closure verification passed"
