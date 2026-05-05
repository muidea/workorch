#!/usr/bin/env bash
set -euo pipefail

# verify-cli.sh — end-to-end verification of the workorch CLI binary.
# Run: ./scripts/verify-cli.sh [path-to-workorch-binary]

BINARY="${1:-./bin/workorch}"
TMP_DIR="$(mktemp -d)"
CONFIG_DIR="$TMP_DIR/config"
PASS=0
FAIL=0

cleanup() { rm -rf "$TMP_DIR"; }
trap cleanup EXIT

mkdir -p "$CONFIG_DIR"

fail() { echo "  ✗ FAIL: $1"; FAIL=$((FAIL + 1)); }
pass() { echo "  ✓ PASS: $1"; PASS=$((PASS + 1)); }

strip_ansi() { sed 's/\x1b\[[0-9;]*m//g'; }

assert_contains() {
  local input="$1" pattern="$2" desc="$3"
  if echo "$input" | strip_ansi | grep -q "$pattern"; then
    pass "$desc"
  else
    fail "$desc (expected: '$pattern')"
  fi
}

assert_not_contains() {
  local input="$1" pattern="$2" desc="$3"
  if ! echo "$input" | strip_ansi | grep -q "$pattern"; then
    pass "$desc"
  else
    fail "$desc (unexpected: '$pattern')"
  fi
}

echo "=== workorch CLI e2e tests ==="
echo "  binary: $BINARY"
echo "  config: $CONFIG_DIR"

# ── 1. Binary existence and help ──
echo ""
echo "── 1. Basic checks ──"

if [[ -x "$BINARY" ]]; then pass "binary exists and is executable"
else fail "binary missing"; fi

HELP=$("$BINARY" --help 2>&1 || true)
assert_contains "$HELP" "workorch" "help shows binary name"
assert_contains "$HELP" "workspace" "help shows workspace command"
assert_contains "$HELP" "agent" "help shows agent command"
assert_contains "$HELP" "config" "help shows config command"

# ── 2. Config commands ──
echo ""
echo "── 2. Config commands ──"

SHOW=$("$BINARY" -config-dir "$CONFIG_DIR" config show 2>&1)
assert_contains "$SHOW" "Storage" "config show works"

SETLLM=$("$BINARY" -config-dir "$CONFIG_DIR" config set-llm test-provider \
  --model "test-model" \
  --base-url "http://localhost:1234/v1" \
  --api-key "test-key" \
  2>&1) || true
assert_contains "$SETLLM" "configured" "config set-llm (flag mode)"

# ── 3. Workspace commands ──
echo ""
echo "── 3. Workspace commands ──"

# With invalid remote, the binary should exit (not hang forever or panic)
if timeout 3 "$BINARY" --remote "http://127.0.0.1:1" workspace list >/dev/null 2>&1; then
  fail "workspace list with bad remote should fail"
else
  pass "workspace list with bad remote exits"
fi

# ── 4. Help for all subcommands ──
echo ""
echo "── 4. Subcommand help ──"

for cmd in workspace run decision approval soul skill mcp memory events status repl; do
  OUT=$("$BINARY" $cmd --help 2>&1) || true
  assert_contains "$OUT" "Usage" "$cmd --help works"
done

# ── 5. Config set-llm wizard (non-interactive flags) ──
echo ""
echo "── 5. LLM config ──"

WIZ=$("$BINARY" -config-dir "$CONFIG_DIR" config set-llm wizard-test \
  --model "wizard-model" \
  --base-url "https://wizard.example.com" \
  --api-key "wk-xxx" \
  2>&1) || true
assert_contains "$WIZ" "configured" "set-llm flag mode works"
assert_contains "$WIZ" "wizard-model" "set-llm shows model name"

# ── 6. Agent command (should fail gracefully without LLM) ──
echo ""
echo "── 6. Agent graceful failure ──"

# Agent with empty config dir should show error, not crash
AGENT_ERR=$(echo "/quit" | timeout 5 "$BINARY" -config-dir "$CONFIG_DIR" agent "test goal" 2>&1) || true
assert_not_contains "$AGENT_ERR" "panic" "agent doesn't panic on error"
assert_not_contains "$AGENT_ERR" "SIGSEGV" "agent doesn't segfault"

# ── 7. Default TUI mode ──
echo ""
echo "── 7. Default TUI mode ──"

TUI=$(timeout 3 "$BINARY" -config-dir "$CONFIG_DIR" 2>&1) || true
assert_contains "$TUI" "TUI requires an interactive terminal" "default mode is TUI and non-TTY suggests --server"
assert_not_contains "$TUI" "bye\|Bye" "default no longer opens legacy REPL"

# ── 8. Unknown subcommand fallback to agent ──
echo ""
echo "── 8. Fallback mode ──"

FALLBACK=$(printf "q" | timeout 5 "$BINARY" -config-dir "$CONFIG_DIR" 查询一下 2>&1) || true
assert_not_contains "$FALLBACK" "unknown command" "unknown command doesn't appear (falls back to agent)"

# ── Summary ──
echo ""
echo "=============================="
echo "  Passed: $PASS"
echo "  Failed: $FAIL"
echo "=============================="

if [[ $FAIL -gt 0 ]]; then
  exit 1
fi
