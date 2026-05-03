#!/usr/bin/env bash
set -euo pipefail
# verify-cli-architecture.sh — static checks for forbidden imports and patterns.

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

# 1. TUI must not import biz/client.
check_no_import "service/tui" "biz/client" "TUI: no HTTP client import"

# 2. TUI must not use fixed polling (tracked in CLI-AGENT-07).
if grep -rq 'time\.Tick\|time\.Ticker\|tea\.Tick\|pollInterval\|PollTick' "$CLI_ROOT/$MODULE_BASE/service/tui" 2>/dev/null; then
  echo "  ⚠ WARN: TUI still uses polling (pending CLI-AGENT-07)"
else
  pass "TUI: no fixed-interval polling"
fi

# 3. Agent uses backend, not direct HTTP.
check_no_import "biz/agent" "biz/client" "Agent: no HTTP client import"

# 4. Production CLI code (service/cli) must not import biz/local.
if [ -d "$CLI_ROOT/$MODULE_BASE/biz/local" ]; then
  fail "CLI: biz/local directory still exists"
else
  pass "CLI: biz/local directory removed"
fi
check_no_import "service/cli" "biz/local" "CLI commands: no launcher import"

# 5. Cobra commands should use backend or client (not both for same operation).
# This is advisory — transition is in progress.

echo ""
echo "=============================="
echo "  Passed: $PASS"
echo "  Failed: $FAIL"
echo "=============================="

if [[ $FAIL -gt 0 ]]; then
  exit 1
fi
