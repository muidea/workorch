#!/usr/bin/env bash

# Long-output behavior is a pure read-model/view concern. Keep the check local
# to TUI state and avoid creating an unrelated Runtime provider fixture.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

GOCACHE="${GOCACHE:-/tmp/workorch-go-build}"
export GOCACHE

go test ./internal/services/workorch/service/tui \
  -run 'Test(ModelScrollableResultViewport|ModelScrollableResultKeepsWorkspacePanelFixed|TUIE2EViewportKeyboardScrollAndMouseSelectionContract|ModelWorkspaceSwitchContextResetsScrolledMainViewport|ModelTerminalOutcomeViaRuntimeViewRespectsUserScrolled|RuntimeViewFollowTailAndUserScrolledUnread)$' \
  -count=1

echo "TUI long-output viewport verification passed"
