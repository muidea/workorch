#!/usr/bin/env bash

# SkillHub discovery belongs to Application setup; Runtime receives only the
# typed, sealed skill selection in its bootstrap. This gate verifies that
# boundary without generating an alternative provider document.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

GOCACHE="${GOCACHE:-/tmp/workorch-go-build}"
export GOCACHE

echo "=== SkillHub selection and Runtime bootstrap verification ==="

go test ./internal/modules/application/workorchd/biz/appservice/contextregistrycore \
  -run 'Test(ListSkillHubSkillsForQueryAddsWorkspaceSkillsWhenAvailable|ListSkillHubSkillsForQueryKeepsGlobalWhenWorkspaceFails|ReadSkillHubSkillResolvesWorkspaceRefUsingEmbeddedWorkspace|ReadSkillHubInstructionAndMetadataReadsFirstAvailableSkillFile)$' \
  -count=1
go test ./internal/modules/application/workorchd/biz/appservice/contextregistryports \
  -run 'Test(ListSkillHubGlobalSkillsUsesCodexGlobalStatus|ListSkillHubWorkspaceSkillsRequiresWorkspaceSkillDirectoryBeforeStatus|ReadSkillViewFallsBackToSkillHubWhenRepositorySkillMissing|ListSkillsViewMergesRepositoryAndSkillHubSkills)$' \
  -count=1
go test ./internal/modules/application/workorchd/biz/appservice \
  -run 'Test(ListSkillsWithBackendIncludesSkillHubGlobalSkills|ListSkillsWithBackendIncludesWorkspaceSkillHubSkills|SelectedSkillsCanResolveSkillHubReferences|SkillHubCatalogRequiresEventHubForWorkspaceSkills)$' \
  -count=1
go test ./internal/modules/application/workorchd/biz/appservice/activationports \
  -run 'TestSelectWorkspaceExecutionSkills(AutoSelectsRepositorySkill|ResolvesExplicitRepositorySkill)$' \
  -count=1
go test ./internal/modules/application/workorchd/biz/appservice/activationcore \
  -run 'TestWorkspaceMCPSelectionContextAutoBindsRequiredRefs' \
  -count=1

echo "SkillHub selection and Runtime bootstrap verification passed"
