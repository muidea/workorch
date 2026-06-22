#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

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

# These terms are allowed in e2e scripts, tests, and docs, where a concrete
# acceptance dataset can describe a business scenario. They are forbidden in
# reusable runtime layers because those layers must not synthesize business
# outcomes from domain-specific goals, tools, or payload fields.
domain_specific_pattern='weather\.current|weatherDescription|temperatureC|humidityPercent|windSpeedKmh|Open-Meteo|Hangzhou|杭州|天气|气温|湿度|风速'
software_dev_bias_pattern='Code Change|code-change|go test \./|npm test|pytest|implementation planning|implementation path|affected files|verification commands|Verification commands|Patch targets|code_agent|target repository|new project|newproject|project root|projectlocation|workingdirectory|source_change|validation_report'
lsp_profile_pattern='lsp|LSP|lsp-bridge|semanticCodeNavigation'
deprecated_workspace_decision_endpoint_pattern='/api/v1/workorch/workspaces/[^[:space:]"]+/decisions|workspaces/\{workspaceId\}/decisions|workspaces/[^[:space:]"]+/decisions'
readmodel_presenter_import_pattern='internal/modules/application/workorchd/pkg/presenter'

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
    if ! rg -q "\b${topic_name}\b" "$contracts_file"; then
      event_contract_violations+=("${topics_file}: ${topic_name} is not declared in ${contracts_file}")
    fi
  done < <(rg -o 'Topic[A-Za-z0-9_]+\s*=' "$topics_file" | sed 's/[[:space:]]*=//')
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

if matches="$(
  rg -n --hidden -S "$readmodel_presenter_import_pattern" \
    internal/modules/application/workorchd/biz/appservice/readmodelcore \
    internal/modules/application/workorchd/biz/appservice/readmodelprojection \
    -g '!**/*_test.go' \
    -g '!**/.git/**'
)"; then
  cat >&2 <<EOF
Boundary contract violation: read-model core/projection code must not import presenter mappers.

$matches

Keep presentation mappers in pkg/presenter or product adapters. Shared
read-model DTOs and projection logic must live in readmodelcore/readmodelprojection
without depending on display-layer helpers.
EOF
  exit 1
fi

if matches="$(
  rg -n --hidden -S "$domain_specific_pattern" "${paths[@]}" \
    -g '!**/*_test.go' \
    -g '!**/dist/**' \
    -g '!**/node_modules/**' \
    -g '!**/.git/**'
)"; then
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

if matches="$(
  rg -n --hidden -S "$software_dev_bias_pattern" "${paths[@]}" \
    -g '!**/*_test.go' \
    -g '!**/dist/**' \
    -g '!**/node_modules/**' \
    -g '!**/.git/**'
)"; then
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

if matches="$(
  rg -n --hidden -S "$lsp_profile_pattern" internal/modules/kernel internal/modules/blocks internal/pkg \
    -g '!**/*_test.go' \
    -g '!internal/pkg/workorchdefaults/defaults/**' \
    -g '!**/dist/**' \
    -g '!**/node_modules/**' \
    -g '!**/.git/**'
)"; then
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

if matches="$(
  rg -n --hidden -S "$deprecated_workspace_decision_endpoint_pattern" internal web scripts \
    -g '!scripts/verify-boundary-contract.sh' \
    -g '!**/*_test.go' \
    -g '!**/dist/**' \
    -g '!**/node_modules/**' \
    -g '!**/.git/**'
)"; then
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
