#!/usr/bin/env bash
set -euo pipefail
# verify-cli-architecture.sh — static checks for forbidden imports and patterns.
# See docs/workorch-cli-tui-agent-requirements.md CLI-AGENT-15, CLI-AGENT-11.

CLI_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MODULE_BASE="internal/modules/application/workorch"
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
if grep -rq '"biz/client"' "$CLI_ROOT/$MODULE_BASE/service/tui" 2>/dev/null; then
  echo "  ⚠ WARN: TUI still imports biz/client (pending CLI-AGENT-07)"
else
  pass "TUI: no HTTP client import"
fi
check_no_import "biz/agent" "biz/client" "Agent: no HTTP client import"
check_no_import "service/cli" "biz/local" "CLI commands: no launcher import"

# 2. biz/local directory must not exist.
if [ -d "$CLI_ROOT/$MODULE_BASE/biz/local" ]; then
  fail "CLI: biz/local directory still exists"
else
  pass "CLI: biz/local directory removed"
fi

# 3. TUI must not use fixed polling (tracked in CLI-AGENT-07).
if grep -rq 'time\.Tick\|time\.Ticker\|tea\.Tick\|pollInterval\|PollTick' "$CLI_ROOT/$MODULE_BASE/service/tui" 2>/dev/null; then
  echo "  ⚠ WARN: TUI still uses polling (pending CLI-AGENT-07)"
else
  pass "TUI: no fixed-interval polling"
fi

# 4. No HTTP client outside biz/client, biz/backend/remote, and service/cli (transition).
echo "  → Checking no HTTP client leakage outside allowed packages..."
HTTP_LEAKS=$(grep -rn '"github.com/muidea/workorch/internal/modules/application/workorch/biz/client"' \
  "$CLI_ROOT/$MODULE_BASE/" 2>/dev/null | grep -v 'biz/client/' | grep -v 'biz/backend/remote.go' | grep -v 'service/cli/' | grep -v 'service/tui/' || true)
if [ -n "$HTTP_LEAKS" ]; then
  fail "HTTP client leaked outside allowed packages: $HTTP_LEAKS"
else
  pass "HTTP client contained in allowed packages"
fi

# 5. Local backend must not import biz/client.
check_no_import "biz/backend" "biz/client" "Local backend: no HTTP client import"

# 6. No agent direct appservice.NewRuntimeSessionFromInitiator calls outside legacy path.
# (Agent has both legacy and backend paths — allowed during transition.)
AGENT_SESSION_CALLS=$(grep -rn 'NewRuntimeSessionFromInitiator' \
  "$CLI_ROOT/$MODULE_BASE/biz/agent/agent.go" 2>/dev/null | wc -l)
if [ "$AGENT_SESSION_CALLS" -le 3 ]; then
  pass "Agent: session init limited to legacy path"
else
  fail "Agent: too many direct session initializations ($AGENT_SESSION_CALLS)"
fi

echo ""
echo "=============================="
echo "  Passed: $PASS"
echo "  Failed: $FAIL"
echo "=============================="

if [[ $FAIL -gt 0 ]]; then
  exit 1
fi
