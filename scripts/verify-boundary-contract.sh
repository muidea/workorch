#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

./scripts/verify-test-runtime-isolation-contract.sh

paths=(
  "internal/modules/kernel/orchestrator"
  "internal/modules/kernel/run"
  "internal/modules/blocks/llm"
  "internal/modules/blocks/mcp"
  "web/workorch-console/src"
)

for path in "${paths[@]}"; do
  if [[ ! -e "$path" ]]; then
    echo "boundary contract check path does not exist: $path" >&2
    exit 1
  fi
done

source_matches() {
  local pattern="$1"
  shift
  grep -R -n -E \
    --exclude='*_test.go' \
    --exclude-dir='.git' \
    --exclude-dir='dist' \
    --exclude-dir='node_modules' \
    -- "$pattern" "$@" || true
}

# Every framework Module/Block in this repository currently participates in
# EventHub lifecycle. Its root is only a lifecycle bridge; the component-local
# Biz must be the sole holder of the Base/EventHub interaction. The repository
# provider remains an Initiator, so it is intentionally excluded here.
mapfile -t component_roots < <(find internal/modules -mindepth 2 -maxdepth 3 -name module.go -printf '%h\n' | sort)
for component_root in "${component_roots[@]}"; do
  if [[ ! -d "$component_root/biz" ]]; then
    echo "boundary contract violation: component has no local biz directory: $component_root" >&2
    exit 1
  fi
  if ! grep -R -q -E --include='*.go' -- 'basebiz\.Base' "$component_root/biz"; then
    echo "boundary contract violation: component Biz does not embed basebiz.Base: $component_root" >&2
    exit 1
  fi
  if ! grep -R -q -E --include='*.go' -- 'func \(.*\) (Stop|Teardown|Close)\(' "$component_root/biz"; then
    echo "boundary contract violation: component Biz has no explicit teardown: $component_root" >&2
    exit 1
  fi
  if [[ "$component_root" == "internal/modules/application/workorchd" ]]; then
    if ! grep -q -E -- 'func \(m \*Module\) (Setup|Run|Teardown)\(' "$component_root/module.go"; then
      echo "boundary contract violation: application module lacks framework lifecycle bridge: $component_root" >&2
      exit 1
    fi
    continue
  fi
  if [[ ! -f "$component_root/plugin.go" ]] || ! grep -q -E -- 'pluginmodule\.Register\(New\(\)\)' "$component_root/plugin.go" || ! grep -q -E -- 'func \(m \*Module\) Setup\(' "$component_root/plugin.go" || ! grep -q -E -- 'func \(m \*Module\) Run\(' "$component_root/plugin.go" || ! grep -q -E -- 'func \(m \*Module\) Teardown\(' "$component_root/plugin.go"; then
    echo "boundary contract violation: component plugin lacks registration or full lifecycle bridge: $component_root" >&2
    exit 1
  fi
done

root_event_operations=()
for component_root in "${component_roots[@]}"; do
  # A component root may contain both module.go and plugin.go. Neither may
  # perform EventHub work: roots are lifecycle bridges only.
  if matches="$(
    find "$component_root" -maxdepth 1 -type f -name '*.go' ! -name '*_test.go' -print0 |
      xargs -0 -r grep -n -E -- 'NewSimpleObserver|\.Subscribe\(|\.Unsubscribe\(|\.Send\(|\.Post\(' || true
  )"; then
    if [[ -n "$matches" ]]; then
      root_event_operations+=("$matches")
    fi
  fi
done

if ((${#root_event_operations[@]} > 0)); then
  cat >&2 <<EOF
Boundary contract violation: Module roots must only bridge lifecycle and may
not create observers, subscribe, or publish EventHub events.

$(printf '%s\n' "${root_event_operations[@]}")
EOF
  exit 1
fi

matches="$(
  find internal/modules -type f -path '*/service/*.go' ! -name '*_test.go' -print0 |
    xargs -0 -r grep -n -E -- 'event\.Hub|magicevent\.Hub|event\.SimpleObserver|magicevent\.SimpleObserver|NewSimpleObserver|\.Subscribe\(|\.Unsubscribe\(|\.Send\(|\.Post\(' || true
)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: protocol service/adapter code must not retain or
operate on EventHub. Move the event lifecycle and typed handlers into the
component-local Biz.

$matches
EOF
  exit 1
fi

matches="$(
  find internal/initiators -type f -name '*.go' ! -name '*_test.go' -print0 |
    xargs -0 -r grep -n -E -- 'basebiz\.Base|NewSimpleObserver|\.Subscribe\(|\.Unsubscribe\(|\.Send\(|\.Post\(' || true
)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: Initiators provide one stateless infrastructure
capability and must not embed Base or participate in business EventHub traffic.

$matches
EOF
  exit 1
fi

mapfile -t event_contract_dirs < <(find internal/modules -type d -path '*/pkg/events' | sort)
matches=""
if ((${#event_contract_dirs[@]} > 0)); then
  matches="$(
    find "${event_contract_dirs[@]}" -type f -name '*.go' -print0 |
      xargs -0 -r grep -n -E -- 'map\[string\]any|\[\]any|\bany\b|<-chan|\bchan\b|internal/pkg/repository|io\.Writer|http\.ResponseWriter|\*.*Repository|Registry|Recorder' || true
  )"
fi
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: EventHub Command/Data/Result contracts must use
bounded concrete DTOs only. They may not leak maps, any, channels, repositories
or other mutable/I/O resources.

$matches
EOF
  exit 1
fi

# Component pkg/ is a contract/value boundary.  It may define DTOs, event
# contracts and pure normalization/validation helpers, but it must not become
# a hidden lifecycle, EventHub, repository or transport owner.  Keep this
# check independent of rg because the CI image intentionally only requires
# portable POSIX tooling.
matches="$(
  find internal/modules -type f -path '*/pkg/*.go' ! -name '*_test.go' -print0 |
    xargs -0 -r grep -n -E -- 'github\.com/muidea/magicCommon/(event|task)|"(os|os/exec|net/http|database/sql|io|io/fs|path/filepath)"|internal/pkg/repository|NewSimpleObserver|\.Subscribe\(|\.Unsubscribe\(|\.Send\(|\.Post\(' || true
)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: component pkg/ may only contain DTOs, contracts
and pure stateless helpers. It must not import EventHub/task, filesystem,
process, network, database or repository infrastructure, or publish/subscribe
events.

$matches

Move lifecycle, I/O and aggregate transition ownership into the component Biz
or a focused owner-local adapter. Keep pkg/ values and pure helpers only.
EOF
  exit 1
fi

# These terms are allowed in e2e scripts, tests, and docs, where a concrete
# acceptance dataset can describe a business scenario. They are forbidden in
# reusable runtime layers because those layers must not synthesize business
# outcomes from domain-specific goals, tools, or payload fields.
domain_specific_pattern='weather\.current|weatherDescription|temperatureC|humidityPercent|windSpeedKmh|Open-Meteo|Hangzhou|杭州|天气|气温|湿度|风速'
software_dev_bias_pattern='Code Change|code-change|go test \./|npm test|pytest|implementation planning|implementation path|affected files|verification commands|Verification commands|Patch targets|code_agent|target repository|new project|newproject|project root|projectlocation|workingdirectory|source_change|validation_report'
lsp_profile_pattern='lsp|LSP|lsp-bridge|semanticCodeNavigation'
deprecated_workspace_decision_endpoint_pattern='/api/v1/workorch/workspaces/[^[:space:]"]+/decisions|workspaces/\{workspaceId\}/decisions|workspaces/[^[:space:]"]+/decisions'
readmodel_presenter_import_pattern='internal/modules/application/workorchd/pkg/presenter'
orchestrator_patch_side_effect_pattern='PatchRuntimeAdapter|NewPatchRuntimeAdapter|applyWorkspacePatch|applyWorkspaceFileWrite|applyWorkspaceTextReplace|applyWorkspaceUnifiedDiff'
orchestrator_repair_target_snapshot_read_pattern='os\.ReadFile|ioutil\.ReadFile'

event_contract_violations=()
while IFS= read -r topics_file; do
  contracts_file="$(dirname "$topics_file")/contracts.go"
  if [[ ! -f "$contracts_file" ]]; then
    # runtimeevent/eventdefs keeps Contracts in topics.go because the dynamic
    # execute prefix and fixed notify topics are defined together.
    contracts_file="$topics_file"
  fi
  while IFS= read -r topic_name; do
    if [[ "$topic_name" == "TopicRuntimeBlockCommandExecutePrefix" ]]; then
      continue
    fi
    if ! grep -q -E -- "(^|[^[:alnum:]_])${topic_name}([^[:alnum:]_]|$)" "$contracts_file"; then
      event_contract_violations+=("${topics_file}: ${topic_name} is not declared in ${contracts_file}")
    fi
  done < <(grep -E -o -- 'Topic[A-Za-z0-9_]+[[:space:]]*=' "$topics_file" | sed 's/[[:space:]]*=//')
done < <(find internal/modules internal/initiators -path '*/pkg/events/topics.go' -o -path '*/pkg/runtimeevent/eventdefs/topics.go' | sort)

if ((${#event_contract_violations[@]} > 0)); then
  cat >&2 <<EOF
Boundary contract violation: every module/block event topic must be declared in its event contract metadata.

$(printf '%s\n' "${event_contract_violations[@]}")

Add the missing topic to the owner package Contracts() list with an explicit
command class and lane policy. Dynamic runtime block execute topics must remain
covered by TopicRuntimeBlockCommandExecutePrefix in the event contract resolver.
EOF
  exit 1
fi

matches="$(source_matches "$readmodel_presenter_import_pattern" \
  internal/modules/application/workorchd/biz/appservice/readmodelcore \
  internal/modules/application/workorchd/biz/appservice/readmodelprojection)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: read-model core/projection code must not import presenter mappers.

$matches

Keep presentation mappers in pkg/presenter or product adapters. Shared
read-model DTOs and projection logic must live in readmodelcore/readmodelprojection
without depending on display-layer helpers.
EOF
  exit 1
fi

matches="$(source_matches "$orchestrator_patch_side_effect_pattern" internal/modules/kernel/orchestrator)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: orchestrator must not own patch filesystem side-effect execution.

$matches

Patch preview/apply/revert technical filesystem work belongs to blocks/file.
Orchestrator may only perform approval gating, runtime block dispatch, formal
Patch evidence admission, coverage binding, and projection events.
EOF
  exit 1
fi

matches="$(source_matches "$orchestrator_repair_target_snapshot_read_pattern" internal/modules/kernel/orchestrator)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: orchestrator repair context must not read target snapshot files directly.

$matches

Target snapshot content/hash acquisition belongs to blocks/file via runtime
managed file/read. Orchestrator may request acquisition and consume successful
block output, but must not materialize workspace file content itself.
EOF
  exit 1
fi

matches="$(source_matches "$domain_specific_pattern" "${paths[@]}")"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: reusable runtime code must not contain concrete business fallback terms.

$matches

Move domain-specific scenario terms to e2e scripts, tests, or documentation.
Generic layers must expose formal evidence and DoD output instead of deriving
business outcomes from specific tools, cities, weather payloads, or provider
fields.
EOF
  exit 1
fi

matches="$(source_matches "$software_dev_bias_pattern" "${paths[@]}")"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: reusable runtime code must not contain software-development profile bias.

$matches

Move software-development-specific guidance, examples, artifact names, test
commands, or deliverable ids to workspace templates, Souls, profile contracts,
profile verifiers, e2e scripts, tests, or documentation. Generic runtime layers
must express only neutral stage, capability, evidence, target, result, and
repair contracts.
EOF
  exit 1
fi

matches="$(
  find internal/modules/kernel internal/modules/blocks internal/pkg -type f \
    ! -name '*_test.go' \
    ! -path 'internal/pkg/workorchdefaults/defaults/*' \
    ! -path '*/dist/*' \
    ! -path '*/node_modules/*' \
    ! -path '*/.git/*' \
    -print0 |
    xargs -0 -r grep -n -E -- "$lsp_profile_pattern" || true
)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: reusable runtime code must not contain LSP profile-specific logic.

$matches

Keep LSP Bridge integration in workspace templates, Souls, capabilities,
profile verifiers, e2e scripts, tests, or documentation. Generic runtime layers
may only handle neutral MCP servers, capabilities, tool calls, evidence,
failure, context, and recovery contracts.
EOF
  exit 1
fi

matches="$(
  find internal web scripts -type f \
    ! -name '*_test.go' \
    ! -path 'scripts/verify-boundary-contract.sh' \
    ! -path '*/dist/*' \
    ! -path '*/node_modules/*' \
    ! -path '*/.git/*' \
    -print0 |
    xargs -0 -r grep -n -E -- "$deprecated_workspace_decision_endpoint_pattern" || true
)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: deprecated workspace-scoped decision REST endpoint is not allowed in code.

$matches

Use /api/v1/workorch/decisions?workspaceId=... for list operations and
/api/v1/workorch/decisions/{ticketId}?workspaceId=... or
/api/v1/workorch/decisions/{ticketId}/resolve?workspaceId=... for scoped
decision detail and resolution operations.
EOF
  exit 1
fi

echo "boundary contract verification passed"
