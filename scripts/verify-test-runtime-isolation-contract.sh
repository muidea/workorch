#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

helper='scripts/lib/verify-isolated-runtime.sh'
managed='scripts/verify-workorch-managed-full.sh'

if [[ ! -f "$helper" ]]; then
  echo "test runtime isolation helper is missing: $helper" >&2
  exit 1
fi
if ! grep -q 'export WORKORCH_VERIFY_ISOLATION_STATE_FILE=' "$managed"; then
  echo "managed verification does not export the runtime isolation state file" >&2
  exit 1
fi

mapfile -t mutators < <(python3 - <<'PY'
from pathlib import Path
import re

self_managed_or_explicit_tools = {
    "create-runtime-v2-bounded-positive-fixtures.sh",
    "verify-ai-proxy-primary-llm-provider-live.sh",
    "verify-test-runtime-isolation-contract.sh",
    "verify-tui-runtime-refresh-history-live.sh",
    "verify-workorch-container.sh",
    "verify-workorch-managed-full.sh",
}
for path in sorted(Path("scripts").glob("*.sh")):
    if path.name in self_managed_or_explicit_tools:
        continue
    text = path.read_text(encoding="utf-8")
    mutates_workorch = "/api/v1/workorch/" in text and re.search(r"-X\s+(?:POST|PUT|DELETE)", text)
    if mutates_workorch:
        print(path)
PY
)

if ((${#mutators[@]} == 0)); then
  echo "test runtime isolation contract found no deployed runtime mutators" >&2
  exit 1
fi

for script in "${mutators[@]}"; do
  if ! grep -q 'scripts/lib/verify-isolated-runtime.sh' "$script"; then
    echo "test runtime isolation violation: $script does not source the isolation helper" >&2
    exit 1
  fi
  if ! grep -q 'workorch_verify_require_isolated_runtime' "$script"; then
    echo "test runtime isolation violation: $script does not enforce managed runtime isolation" >&2
    exit 1
  fi
  if grep -q 'WORKORCH_BASE_URL:-http://127\.0\.0\.1:8080' "$script" &&
    ! grep -q 'mutation mode' "$script"; then
    echo "test runtime isolation violation: $script still defaults to the shared 127.0.0.1:8080 instance" >&2
    exit 1
  fi
done

tmp_dir="$(mktemp -d)"
cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT

mkdir -p "$tmp_dir/config" "$tmp_dir/data"
touch "$tmp_dir/config/application.toml"
python3 - "$tmp_dir/managed-runtime-state.json" "$tmp_dir/config" "$tmp_dir/data" "$$" <<'PY'
import json
import sys

path, config_dir, data_dir, pid = sys.argv[1:]
with open(path, "w", encoding="utf-8") as handle:
    json.dump({
        "state": "running",
        "baseUrl": "http://127.0.0.1:45123",
        "pid": int(pid),
        "configDir": config_dir,
        "dataDir": data_dir,
    }, handle)
PY

source "$helper"
export WORKORCH_VERIFY_ISOLATION_STATE_FILE="$tmp_dir/managed-runtime-state.json"
workorch_verify_require_isolated_runtime "http://127.0.0.1:45123" "isolation contract self-test"
if workorch_verify_require_isolated_runtime "http://127.0.0.1:8080" "isolation contract negative test" >/dev/null 2>&1; then
  echo "test runtime isolation helper accepted a mismatched runtime" >&2
  exit 1
fi

echo "test runtime isolation contract verified"
