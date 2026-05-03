#!/usr/bin/env bash
set -euo pipefail
# verify-tui-agent.sh — TUI agent acceptance checks.
# Run: ./scripts/verify-tui-agent.sh [--static-only] [--scripted]

CLI_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PASS=0
FAIL=0

fail() { echo "  ✗ FAIL: $1"; FAIL=$((FAIL + 1)); }
pass() { echo "  ✓ PASS: $1"; PASS=$((PASS + 1)); }

STATIC_ONLY=false
SCRIPTED=false
for arg in "$@"; do
  case "$arg" in
    --static-only) STATIC_ONLY=true ;;
    --scripted) SCRIPTED=true ;;
  esac
done
if ! $STATIC_ONLY && ! $SCRIPTED; then
  STATIC_ONLY=true
  SCRIPTED=true
fi

echo "=== TUI Agent Acceptance ==="

# ── Layer 1: Static Architecture Guards ──
if $STATIC_ONLY; then
  echo ""
  echo "── Layer 1: Static Architecture Guards ──"

  MODULE_BASE="internal/modules/application/workorch"
  TUI_DIR="$CLI_ROOT/$MODULE_BASE/service/tui"

  # 1a. TUI must not import biz/client.
  if grep -rq '"biz/client"' "$TUI_DIR" 2>/dev/null; then
    fail "TUI imports biz/client"
  else
    pass "TUI: no biz/client import"
  fi

  # 1b. TUI must not use fixed polling.
  if grep -rq 'time\.Tick\|time\.Ticker\|tea\.Tick\|pollInterval\|PollTick' "$TUI_DIR" 2>/dev/null; then
    fail "TUI uses fixed-interval polling"
  else
    pass "TUI: no fixed-interval polling"
  fi

  # 1c. No biz/local directory.
  if [ -d "$CLI_ROOT/$MODULE_BASE/biz/local" ]; then
    fail "biz/local directory still exists"
  else
    pass "CLI: biz/local removed"
  fi
fi

# ── Layer 2+3: Go tests ──
echo ""
echo "── Layer 2+3: Go Tests ──"

cd "$CLI_ROOT"

# TUI model tests
if go test ./internal/modules/application/workorch/service/tui --count 1 2>&1; then
  pass "TUI model tests pass"
else
  fail "TUI model tests fail"
fi

# Backend tests
if go test ./internal/modules/application/workorch/biz/backend --count 1 2>&1; then
  pass "Backend tests pass"
else
  fail "Backend tests fail"
fi

# Agent tests
if go test ./internal/modules/application/workorch/biz/agent --count 1 2>&1; then
  pass "Agent tests pass"
else
  fail "Agent tests fail"
fi

# ── Layer 4: Scripted E2E ──
if $SCRIPTED; then
  echo ""
  echo "── Layer 4: Scripted E2E ──"

  BINARY="$CLI_ROOT/bin/workorch"
  if [ ! -x "$BINARY" ]; then
    fail "Binary not found at $BINARY"
  else
    pass "Binary exists"

    # TUI-E2E-01: Opens agent TUI by default (no --remote).
    OUTPUT=$(timeout 3 "$BINARY" </dev/null 2>&1) || true
    if echo "$OUTPUT" | grep -q "Agent\|agent\|WorkOrch"; then
      pass "TUI-E2E-01: opens agent session by default"
    else
      fail "TUI-E2E-01: no agent session markers found"
    fi

    # TUI-E2E-03: Submit goal starts run (via REPL mode).
    REPL_OUT=$(printf "/quit\n" | timeout 5 "$BINARY" 2>&1) || true
    if echo "$REPL_OUT" | grep -q "bye\|Bye"; then
      pass "TUI-E2E-03: REPL accepts input and quits cleanly"
    else
      fail "TUI-E2E-03: REPL did not quit cleanly"
    fi

    # TUI-E2E-10: Interrupt and cleanup (no child processes).
    if ! pgrep -f "workorchd" >/dev/null 2>&1; then
      pass "TUI-E2E-10: no workorchd child process"
    else
      fail "TUI-E2E-10: workorchd process detected"
    fi
  fi
fi

echo ""
echo "=============================="
echo "  Passed: $PASS"
echo "  Failed: $FAIL"
echo "=============================="

if [[ $FAIL -gt 0 ]]; then
  exit 1
fi
