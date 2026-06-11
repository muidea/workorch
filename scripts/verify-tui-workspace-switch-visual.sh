#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="${WORKORCH_TUI_WORKSPACE_SWITCH_ARTIFACT_DIR:-$(mktemp -d)}"
KEEP_ARTIFACTS="${WORKORCH_TUI_WORKSPACE_SWITCH_KEEP_ARTIFACTS:-1}"

cleanup() {
  local status=$?
  if [[ "$KEEP_ARTIFACTS" != "1" && "$KEEP_ARTIFACTS" != "true" && $status -eq 0 ]]; then
    rm -rf "$TMP_DIR"
  else
    echo "tui workspace switch visual artifacts: $TMP_DIR" >&2
  fi
}

trap cleanup EXIT

mkdir -p "$TMP_DIR"

cd "$ROOT_DIR"

WORKORCH_TUI_WORKSPACE_SWITCH_ARTIFACT_DIR="$TMP_DIR" \
  go test ./internal/services/workorch/service/tui \
    -run '^TestTUIE2EWorkspaceSwitchProjectionRefreshVisualSnapshot$' \
    -count=1 \
    -timeout=60s

before="$TMP_DIR/01-before-switch.txt"
switched="$TMP_DIR/02-after-switch-before-refresh.txt"
refreshed="$TMP_DIR/03-after-projection-refresh.txt"
inactive="$TMP_DIR/04-after-inactive-projection-event.txt"

for file in "$before" "$switched" "$refreshed" "$inactive"; do
  if [[ ! -s "$file" ]]; then
    echo "missing TUI visual snapshot: $file" >&2
    exit 1
  fi
done

grep -q "task-a" "$before"
grep -q "Workspace switched; refreshing state" "$switched"
if grep -q "task-a" "$switched"; then
  echo "stale workspace A task leaked into switched workspace view" >&2
  exit 1
fi
grep -q "build workspace B" "$refreshed"
grep -q "Running - B is making progress" "$refreshed"
grep -q "ws ws-b" "$refreshed"
grep -q "run run-b" "$refreshed"
if grep -q "task-a" "$refreshed"; then
  echo "stale workspace A task leaked into refreshed workspace B view" >&2
  exit 1
fi
grep -q "build workspace B" "$inactive"
if grep -q "task-a" "$inactive"; then
  echo "inactive workspace A projection event polluted workspace B view" >&2
  exit 1
fi

echo "TUI workspace switch visual verification passed artifacts=$TMP_DIR"
