#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASE_URL="${WORKORCH_TEMPLATE_SOUL_VERIFY_BASE_URL:-${WORKORCH_BASE_URL:-}}"
source "$ROOT_DIR/scripts/lib/verify-isolated-runtime.sh"
workorch_verify_require_isolated_runtime "$BASE_URL" "verify-workspace-template-soul"
TMP_DIR="${WORKORCH_TEMPLATE_SOUL_VERIFY_ARTIFACT_DIR:-$(mktemp -d)}"
KEEP_ARTIFACTS="${WORKORCH_TEMPLATE_SOUL_VERIFY_KEEP_ARTIFACTS:-0}"
VERIFY_SUFFIX="${WORKORCH_TEMPLATE_SOUL_VERIFY_SUFFIX:-$(date +%s)-$$}"
SOUL_ID="soul-template-e2e-$VERIFY_SUFFIX"
UPDATED_SOUL_NAME="Template E2E Soul Updated $VERIFY_SUFFIX"
TEMPLATE_ID="workspace-template-e2e-$VERIFY_SUFFIX"
WORKSPACE_TITLE="Workspace Template E2E $VERIFY_SUFFIX"

report_failure() {
  local status=$?
  if [[ $status -eq 0 ]]; then
    return
  fi
  echo "workspace template and soul e2e verification failed for deployed service $BASE_URL" >&2
  echo "artifacts=$TMP_DIR" >&2
  for file in "$TMP_DIR"/*; do
    [[ -f "$file" ]] || continue
    echo "--- $file ---" >&2
    if [[ "$file" == *.json ]]; then
      python3 -m json.tool "$file" >&2 || cat "$file" >&2
    elif [[ "$file" == *.js ]]; then
      wc -c "$file" >&2
    else
      cat "$file" >&2
    fi
  done
}

cleanup() {
  local status=$?
  if [[ "$KEEP_ARTIFACTS" != "1" && $status -eq 0 ]]; then
    rm -rf "$TMP_DIR"
  fi
}

json_get() {
  local file="$1"
  local expr="$2"
  python3 - "$file" "$expr" <<'PY'
import json
import sys

path, expr = sys.argv[1], sys.argv[2]
with open(path, "r", encoding="utf-8") as fh:
    value = json.load(fh)
for part in expr.split("."):
    if isinstance(value, list):
        value = value[int(part)]
    else:
        value = value[part]
print(value)
PY
}

assert_json() {
  local file="$1"
  local description="$2"
  local code="$3"
  python3 - "$file" "$description" "$code" <<'PY'
import json
import sys

path, description, code = sys.argv[1], sys.argv[2], sys.argv[3]
with open(path, "r", encoding="utf-8") as fh:
    data = json.load(fh)
if not eval(code, {"data": data}):
    raise SystemExit(f"{description} failed for {path}")
PY
}

download_console_assets() {
  local index_file="$TMP_DIR/workorch-console.js"
  curl -fsS "$BASE_URL/console/assets/workorch-console.js" >"$index_file"
  python3 - "$index_file" "$TMP_DIR/console-assets.txt" <<'PY'
import re
import sys

bundle_path, output_path = sys.argv[1:3]
with open(bundle_path, "r", encoding="utf-8") as fh:
    text = fh.read()
assets = sorted(set(re.findall(r'assets/[A-Za-z0-9._-]+\.js', text)))
with open(output_path, "w", encoding="utf-8") as fh:
    for asset in assets:
        fh.write(asset + "\n")
PY
  local asset
  while IFS= read -r asset; do
    [[ -n "$asset" ]] || continue
    curl -fsS "$BASE_URL/console/$asset" >"$TMP_DIR/$(basename "$asset")"
  done <"$TMP_DIR/console-assets.txt"
  cat "$TMP_DIR"/*.js >"$TMP_DIR/console-bundle-text.txt"
}

assert_console_text() {
  local text="$1"
  if ! grep -F "$text" "$TMP_DIR/console-bundle-text.txt" >/dev/null; then
    echo "console bundle missing text: $text" >&2
    exit 1
  fi
}

trap report_failure ERR
trap cleanup EXIT

mkdir -p "$TMP_DIR"

curl -fsS "$BASE_URL/healthz" >"$TMP_DIR/healthz.json"
assert_json "$TMP_DIR/healthz.json" "healthz status" 'data.get("status") == "ok"'
curl -fsS "$BASE_URL/readyz" >"$TMP_DIR/readyz.json"
assert_json "$TMP_DIR/readyz.json" "readyz status" 'data.get("status") == "ready"'

curl -fsS "$BASE_URL/api/v1/workorch/builtin-defaults" >"$TMP_DIR/builtin-defaults.json"
assert_json "$TMP_DIR/builtin-defaults.json" "builtin default owner inventory" 'len(data.get("defaults", [])) == 42 and sum(item.get("kind") == "soul" for item in data["defaults"]) == 36 and sum(item.get("kind") == "workspace_template" for item in data["defaults"]) == 6'
assert_json "$TMP_DIR/builtin-defaults.json" "builtin default current versus embedded content" 'all(item.get("id") and item.get("builtinContentHash") and item.get("present") == bool(item.get("currentContentHash")) and isinstance(item.get("upgradeAvailable"), bool) for item in data["defaults"])'

download_console_assets
assert_console_text '执行人格（Soul）'
assert_console_text '工作空间模板（Workspace Template）'
assert_console_text '执行人格定义已删除'
assert_console_text '删除工作空间模板'
assert_console_text '调整执行人格'
assert_console_text '执行人格配置已保存'

cat >"$TMP_DIR/soul-create.json" <<JSON
{
  "id": "$SOUL_ID",
  "name": "Template E2E Soul $VERIFY_SUFFIX",
  "description": "Soul used by deployed-instance workspace template verification.",
  "businessDomains": ["workorch", "template-e2e"],
  "soulMd": "# Template E2E Soul\\n\\nPlan, execute, decide, approve, validate, and produce evidence-backed artifacts.\\n\\n## Planning\\n\\nUse the workspace Goal and DoD.\\n\\n## Validation\\n\\nRequire formal evidence.\\n",
  "allowedSkillRefs": ["skill-template-e2e"],
  "allowedMCPServerRefs": ["mcp-template-e2e"],
  "allowedToolKinds": ["llm/chat", "shell/command"]
}
JSON

curl -fsS -X POST "$BASE_URL/api/v1/workorch/souls" \
  -H 'Content-Type: application/json' \
  -d @"$TMP_DIR/soul-create.json" >"$TMP_DIR/soul-created.json"
assert_json "$TMP_DIR/soul-created.json" "created soul id" 'data["soul"]["id"].startswith("soul-template-e2e-")'

python3 - "$TMP_DIR/soul-create.json" "$TMP_DIR/soul-update.json" "$UPDATED_SOUL_NAME" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as fh:
    soul = json.load(fh)
soul["name"] = sys.argv[3]
soul["description"] = "Updated through deployed-instance e2e verification."
soul["businessDomains"].append("updated")
with open(sys.argv[2], "w", encoding="utf-8") as fh:
    json.dump(soul, fh, ensure_ascii=False)
PY

curl -fsS -X PUT "$BASE_URL/api/v1/workorch/souls/$SOUL_ID" \
  -H 'Content-Type: application/json' \
  -d @"$TMP_DIR/soul-update.json" >"$TMP_DIR/soul-updated.json"
assert_json "$TMP_DIR/soul-updated.json" "updated soul name" 'data["soul"]["name"].startswith("Template E2E Soul Updated")'

curl -fsS "$BASE_URL/api/v1/workorch/souls" >"$TMP_DIR/souls.json"
assert_json "$TMP_DIR/souls.json" "soul listed" "any(item.get('id') == '$SOUL_ID' for item in data.get('souls', []))"
curl -fsS "$BASE_URL/api/v1/workorch/souls/$SOUL_ID" >"$TMP_DIR/soul-read.json"
assert_json "$TMP_DIR/soul-read.json" "soul.md exposed" '"soulMd" in data["soul"] and "Template E2E Soul" in data["soul"]["soulMd"]'

DELETE_SOUL_ID="soul-template-e2e-delete-$VERIFY_SUFFIX"
cat >"$TMP_DIR/soul-delete-create.json" <<JSON
{
  "id": "$DELETE_SOUL_ID",
  "name": "Template E2E Delete Soul $VERIFY_SUFFIX",
  "description": "Temporary Soul used to verify system configuration delete.",
  "soulMd": "# Template E2E Delete Soul\\n\\nTemporary delete verification.\\n"
}
JSON
curl -fsS -X POST "$BASE_URL/api/v1/workorch/souls" \
  -H 'Content-Type: application/json' \
  -d @"$TMP_DIR/soul-delete-create.json" >"$TMP_DIR/soul-delete-created.json"
delete_status="$(curl -sS -o "$TMP_DIR/soul-delete-response.json" -w '%{http_code}' \
  -X DELETE "$BASE_URL/api/v1/workorch/souls/$DELETE_SOUL_ID")"
if [[ "$delete_status" != "204" ]]; then
  echo "delete soul status=$delete_status, want 204" >&2
  exit 1
fi
deleted_read_status="$(curl -sS -o "$TMP_DIR/soul-delete-read-response.json" -w '%{http_code}' \
  "$BASE_URL/api/v1/workorch/souls/$DELETE_SOUL_ID")"
if [[ "$deleted_read_status" != "404" ]]; then
  echo "deleted soul read status=$deleted_read_status, want 404" >&2
  exit 1
fi

cat >"$TMP_DIR/template-invalid.json" <<JSON
{
  "id": "$TEMPLATE_ID-invalid",
  "name": "Invalid Template $VERIFY_SUFFIX",
  "goalText": "This template must be rejected.",
  "doDItems": ["invalid soul references are rejected"],
  "defaultSoulId": "missing-soul-$VERIFY_SUFFIX"
}
JSON
invalid_status="$(curl -sS -o "$TMP_DIR/template-invalid-response.json" -w '%{http_code}' \
  -X POST "$BASE_URL/api/v1/workorch/workspace-templates" \
  -H 'Content-Type: application/json' \
  -d @"$TMP_DIR/template-invalid.json")"
if [[ "$invalid_status" != "404" && "$invalid_status" != "400" ]]; then
  echo "invalid template status=$invalid_status, want 400 or 404" >&2
  exit 1
fi

DELETE_TEMPLATE_ID="workspace-template-e2e-delete-$VERIFY_SUFFIX"
cat >"$TMP_DIR/template-delete-create.json" <<JSON
{
  "id": "$DELETE_TEMPLATE_ID",
  "name": "Template E2E Delete Workspace $VERIFY_SUFFIX",
  "description": "Temporary workspace template used to verify delete.",
  "goalText": "This template is deleted by e2e.",
  "doDItems": ["Template can be deleted"],
  "defaultSoulId": "$SOUL_ID"
}
JSON
curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspace-templates" \
  -H 'Content-Type: application/json' \
  -d @"$TMP_DIR/template-delete-create.json" >"$TMP_DIR/template-delete-created.json"
delete_template_status="$(curl -sS -o "$TMP_DIR/template-delete-response.json" -w '%{http_code}' \
  -X DELETE "$BASE_URL/api/v1/workorch/workspace-templates/$DELETE_TEMPLATE_ID")"
if [[ "$delete_template_status" != "204" ]]; then
  echo "delete workspace template status=$delete_template_status, want 204" >&2
  exit 1
fi
curl -fsS "$BASE_URL/api/v1/workorch/workspace-templates" >"$TMP_DIR/templates-after-delete.json"
assert_json "$TMP_DIR/templates-after-delete.json" "deleted template not listed" "not any(item.get('id') == '$DELETE_TEMPLATE_ID' for item in data.get('workspaceTemplates', []))"

cat >"$TMP_DIR/template-create.json" <<JSON
{
  "id": "$TEMPLATE_ID",
  "name": "Template E2E Workspace $VERIFY_SUFFIX",
  "description": "Reusable workspace template verified through deployed-instance e2e.",
  "goalText": "Verify workspace template based creation and Soul binding for suffix $VERIFY_SUFFIX.",
  "doDItems": [
    "Workspace is created from template data",
    "Goal is defined from template data",
    "DoD is defined from template data",
    "Workspace Soul bindings are persisted and readable"
  ],
  "defaultSoulId": "$SOUL_ID",
  "soulBindings": {
    "planning": "$SOUL_ID",
    "execution": "$SOUL_ID",
    "decision": "$SOUL_ID",
    "approval": "$SOUL_ID",
    "validation": "$SOUL_ID",
    "artifact": "$SOUL_ID"
  }
}
JSON

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspace-templates" \
  -H 'Content-Type: application/json' \
  -d @"$TMP_DIR/template-create.json" >"$TMP_DIR/template-created.json"
assert_json "$TMP_DIR/template-created.json" "created template id" "data['workspaceTemplate']['id'] == '$TEMPLATE_ID'"

python3 - "$TMP_DIR/template-create.json" "$TMP_DIR/template-update.json" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as fh:
    template = json.load(fh)
template["description"] = "Updated deployed-instance template."
template["doDItems"].append("Template can be updated before reuse")
template["soulBindings"] = {
    "planning": template["defaultSoulId"],
    "execution": template["defaultSoulId"],
    "validation": template["defaultSoulId"],
}
with open(sys.argv[2], "w", encoding="utf-8") as fh:
    json.dump(template, fh, ensure_ascii=False)
PY

curl -fsS -X PUT "$BASE_URL/api/v1/workorch/workspace-templates/$TEMPLATE_ID" \
  -H 'Content-Type: application/json' \
  -d @"$TMP_DIR/template-update.json" >"$TMP_DIR/template-updated.json"
assert_json "$TMP_DIR/template-updated.json" "updated template validation binding" "data['workspaceTemplate']['soulBindings'].get('validation') == '$SOUL_ID'"

curl -fsS "$BASE_URL/api/v1/workorch/workspace-templates" >"$TMP_DIR/templates.json"
assert_json "$TMP_DIR/templates.json" "template listed" "any(item.get('id') == '$TEMPLATE_ID' for item in data.get('workspaceTemplates', []))"

python3 - "$TMP_DIR/templates.json" "$TEMPLATE_ID" "$TMP_DIR/workspace-create.json" "$WORKSPACE_TITLE" <<'PY'
import json
import sys

templates_path, template_id, output_path, title = sys.argv[1:5]
with open(templates_path, "r", encoding="utf-8") as fh:
    templates = json.load(fh).get("workspaceTemplates", [])
template = next(item for item in templates if item["id"] == template_id)
workspace = {
    "title": title,
    "description": template.get("description", ""),
    "templateId": template_id,
}
with open(output_path, "w", encoding="utf-8") as fh:
    json.dump(workspace, fh, ensure_ascii=False)
PY

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces" \
  -H 'Content-Type: application/json' \
  -d @"$TMP_DIR/workspace-create.json" >"$TMP_DIR/workspace-created.json"
workspace_id="$(json_get "$TMP_DIR/workspace-created.json" "workspace.id")"
if [[ -z "$workspace_id" || "$workspace_id" != ws-* ]]; then
  echo "workspace id is invalid: $workspace_id" >&2
  exit 1
fi

python3 - "$TMP_DIR/templates.json" "$TEMPLATE_ID" "$TMP_DIR/goal-create.json" "$TMP_DIR/dod-create.json" <<'PY'
import json
import sys

templates_path, template_id, goal_path, dod_path = sys.argv[1:5]
with open(templates_path, "r", encoding="utf-8") as fh:
    templates = json.load(fh).get("workspaceTemplates", [])
template = next(item for item in templates if item["id"] == template_id)
with open(goal_path, "w", encoding="utf-8") as fh:
    json.dump({"text": template["goalText"]}, fh, ensure_ascii=False)
with open(dod_path, "w", encoding="utf-8") as fh:
    json.dump({"items": template["doDItems"]}, fh, ensure_ascii=False)
PY

curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/goals" \
  -H 'Content-Type: application/json' \
  -d @"$TMP_DIR/goal-create.json" >"$TMP_DIR/goal-created.json"
goal_id="$(json_get "$TMP_DIR/goal-created.json" "goal.id")"
curl -fsS -X POST "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/dods" \
  -H 'Content-Type: application/json' \
  -d @"$TMP_DIR/dod-create.json" >"$TMP_DIR/dod-created.json"
dod_id="$(json_get "$TMP_DIR/dod-created.json" "dod.id")"

curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/soul-bindings" >"$TMP_DIR/bindings-read.json"
assert_json "$TMP_DIR/bindings-read.json" "workspace default soul from template" "data['soulBindings']['defaultSoulId'] == '$SOUL_ID'"
assert_json "$TMP_DIR/bindings-read.json" "workspace bindings readable" "data['soulBindings']['bindings'].get('execution') == '$SOUL_ID'"
assert_json "$TMP_DIR/bindings-read.json" "workspace validation soul from template" "data['soulBindings']['bindings'].get('validation') == '$SOUL_ID'"

curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/goals" >"$TMP_DIR/goals.json"
assert_json "$TMP_DIR/goals.json" "goal listed" "any(item.get('id') == '$goal_id' for item in data.get('goals', []))"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/dods" >"$TMP_DIR/dods.json"
assert_json "$TMP_DIR/dods.json" "dod listed" "any(item.get('id') == '$dod_id' for item in data.get('dods', []))"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/snapshot" >"$TMP_DIR/workspace-snapshot.json"
assert_json "$TMP_DIR/workspace-snapshot.json" "snapshot workspace id" "data['snapshot']['workspace']['id'] == '$workspace_id'"

echo "workspace template and soul e2e verification passed for deployed service $BASE_URL workspace=$workspace_id template=$TEMPLATE_ID soul=$SOUL_ID"
