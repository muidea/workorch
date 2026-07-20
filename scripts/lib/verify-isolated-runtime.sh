#!/usr/bin/env bash

# Guard destructive deployed-instance verification. Callers must run against a
# service created by verify-workorch-managed-full.sh, whose config and data live
# below the managed artifact directory.
workorch_verify_write_isolation_state() {
  local state_file="${1:?state file is required}"
  local state="${2:?state is required}"
  local base_url="${3:?base URL is required}"
  local pid="${4:-}"
  local config_dir="${5:?config directory is required}"
  local data_dir="${6:?data directory is required}"

  python3 - "$state_file" "$state" "$base_url" "$pid" "$config_dir" "$data_dir" <<'PY'
import json
import pathlib
import sys

path, state, base_url, pid, config_dir, data_dir = sys.argv[1:]
target = pathlib.Path(path)
target.parent.mkdir(parents=True, exist_ok=True)
payload = {
    "state": state,
    "baseUrl": base_url.rstrip("/"),
    "pid": int(pid) if pid.isdigit() else None,
    "configDir": config_dir,
    "dataDir": data_dir,
}
target.write_text(json.dumps(payload, indent=2, sort_keys=True) + "\n", encoding="utf-8")
PY
}

workorch_verify_require_isolated_runtime() {
  local base_url="${1:-}"
  local caller="${2:-verification script}"
  local state_file="${WORKORCH_VERIFY_ISOLATION_STATE_FILE:-}"

  if [[ -z "$base_url" ]]; then
    echo "$caller requires WORKORCH_BASE_URL from a managed verification runtime" >&2
    return 2
  fi
  if [[ -z "$state_file" ]]; then
    cat >&2 <<EOF
$caller refuses to mutate an unmanaged WorkOrch instance at $base_url.
Run it through scripts/verify-workorch-managed-full.sh so configuration,
workspace data, and generated artifacts remain inside a disposable runtime.
EOF
    return 2
  fi

  python3 - "$state_file" "$base_url" "$caller" <<'PY'
import json
import os
import pathlib
import sys

state_path = pathlib.Path(sys.argv[1]).expanduser().resolve()
requested_base_url = sys.argv[2].rstrip("/")
caller = sys.argv[3]

def reject(reason: str) -> None:
    print(f"{caller} refuses unmanaged runtime mutation: {reason}", file=sys.stderr)
    raise SystemExit(2)

if not state_path.is_file():
    reject(f"isolation state file does not exist: {state_path}")
if state_path.stat().st_uid != os.getuid():
    reject(f"isolation state file is not owned by the current user: {state_path}")

try:
    state = json.loads(state_path.read_text(encoding="utf-8"))
except (OSError, json.JSONDecodeError) as exc:
    reject(f"cannot read isolation state file {state_path}: {exc}")

if state.get("state") != "running":
    reject(f"managed runtime state is {state.get('state')!r}, want 'running'")

managed_base_url = str(state.get("baseUrl") or "").rstrip("/")
if not managed_base_url or managed_base_url != requested_base_url:
    reject(f"base URL {requested_base_url!r} does not match managed runtime {managed_base_url!r}")

pid = state.get("pid")
if not isinstance(pid, int) or pid <= 0:
    reject("managed runtime PID is missing")
try:
    os.kill(pid, 0)
except OSError as exc:
    reject(f"managed runtime PID {pid} is not alive: {exc}")

artifact_root = state_path.parent.resolve()
for field in ("configDir", "dataDir"):
    raw = state.get(field)
    if not isinstance(raw, str) or not raw.strip():
        reject(f"managed runtime {field} is missing")
    path = pathlib.Path(raw).expanduser().resolve()
    try:
        inside_artifacts = os.path.commonpath((str(artifact_root), str(path))) == str(artifact_root)
    except ValueError:
        inside_artifacts = False
    if not inside_artifacts:
        reject(f"managed runtime {field} escapes artifact root: {path}")

config_file = pathlib.Path(state["configDir"]).expanduser().resolve() / "application.toml"
if not config_file.is_file():
    reject(f"managed runtime configuration is missing: {config_file}")
PY
}
