#!/usr/bin/env bash
set -euo pipefail
# verify-cli-architecture.sh — static checks for forbidden imports and patterns.
# See docs/workorch-cli-tui-agent-requirements.md CLI-AGENT-15, CLI-AGENT-11.

CLI_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MODULE_BASE="internal/services/workorch"
PASS=0
FAIL=0

fail() { echo "  ✗ FAIL: $1"; FAIL=$((FAIL + 1)); }
pass() { echo "  ✓ PASS: $1"; PASS=$((PASS + 1)); }

check_no_import() {
  local target="$1" forbidden="$2" desc="$3"
  if grep -rq "\"$forbidden\"" "$CLI_ROOT/$MODULE_BASE/$target" 2>/dev/null; then
    fail "$desc (found $forbidden in $target)"
    grep -rn "\"$forbidden\"" "$CLI_ROOT/$MODULE_BASE/$target"
  else
    pass "$desc"
  fi
}

echo "=== CLI Architecture Guards ==="

# 1. Forbidden imports in local-mode packages.
check_no_import "service/tui" "workorchclient" "TUI: no HTTP client import"
check_no_import "biz/agent" "workorchclient" "Agent: no HTTP client import"
check_no_import "service/cli" "workorchclient" "CLI commands: no HTTP client import"
check_no_import "service/cli" "biz/local" "CLI commands: no launcher import"

# 2. biz/local directory must not exist.
if [ -d "$CLI_ROOT/$MODULE_BASE/biz/local" ]; then
  fail "CLI: biz/local directory still exists"
else
  pass "CLI: biz/local directory removed"
fi

# 3. TUI must not use fixed polling.
if grep -rq 'time\.Tick\|time\.Ticker\|tea\.Tick\|pollInterval\|PollTick' "$CLI_ROOT/$MODULE_BASE/service/tui" 2>/dev/null; then
  fail "TUI: uses fixed-interval polling"
else
  pass "TUI: no fixed-interval polling"
fi

# 3b. TUI must preserve terminal-native mouse selection/copy by default.
if grep -rEq 'WithMouse|tea\.MouseMsg|MouseWheelEnabled[[:space:]]*=[[:space:]]*true' "$CLI_ROOT/$MODULE_BASE/service/cli" "$CLI_ROOT/$MODULE_BASE/service/tui" 2>/dev/null; then
	fail "TUI: enables mouse reporting and blocks terminal selection"
else
	pass "TUI: mouse selection/copy preserved"
fi

# 4. No HTTP client outside workorchclient and biz/backends/remote.
echo "  → Checking no HTTP client leakage outside allowed packages..."
HTTP_LEAKS=$(grep -rn '"github.com/muidea/workorch/internal/pkg/workorchclient"' \
  "$CLI_ROOT/$MODULE_BASE/" 2>/dev/null | grep -v 'workorchclient/' | grep -v 'biz/backends/remote.go' || true)
if [ -n "$HTTP_LEAKS" ]; then
  fail "HTTP client leaked outside allowed packages: $HTTP_LEAKS"
else
  pass "HTTP client contained in allowed packages"
fi

# 5. Local/eventhub backend must not import workorchclient; remote backend is the
# only backend transport allowed to use the HTTP client.
LOCAL_BACKEND_LEAKS=$(grep -rn '"github.com/muidea/workorch/internal/pkg/workorchclient"\|"workorchclient"' \
  "$CLI_ROOT/$MODULE_BASE/biz/backends/" 2>/dev/null | grep -v 'biz/backends/remote.go' || true)
if [ -n "$LOCAL_BACKEND_LEAKS" ]; then
  fail "Local/eventhub backend imports HTTP client: $LOCAL_BACKEND_LEAKS"
else
  pass "Local/eventhub backend: no HTTP client import"
fi

# 6. Agent must not create runtime sessions or use fixed-interval polling.
AGENT_SESSION_CALLS=$(grep -c 'NewRuntimeSessionFromInitiator' \
  "$CLI_ROOT/$MODULE_BASE/biz/agent/agent.go" 2>/dev/null || true)
if [ "$AGENT_SESSION_CALLS" -eq 0 ]; then
  pass "Agent: no direct runtime session initialization"
else
  fail "Agent: direct runtime session initialization found"
fi

if grep -rq 'time\.NewTicker\|time\.Ticker\|time\.Tick\|pollRun' "$CLI_ROOT/$MODULE_BASE/biz/agent" 2>/dev/null; then
  fail "Agent: uses fixed-interval polling"
else
  pass "Agent: no fixed-interval polling"
fi

if grep -rq 'startAgentREPL\|runAgentREPL' "$CLI_ROOT/$MODULE_BASE/service/cli" 2>/dev/null; then
  fail "CLI: legacy REPL entrypoint remains"
else
  pass "CLI: default entrypoint is TUI-backed"
fi

echo ""
echo "=============================="
echo "  Passed: $PASS"
echo "  Failed: $FAIL"
echo "=============================="

if [[ $FAIL -gt 0 ]]; then
  exit 1
fi
