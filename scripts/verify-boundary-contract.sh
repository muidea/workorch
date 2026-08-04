#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

./scripts/verify-test-runtime-isolation-contract.sh

paths=(
  "internal/modules/kernel/orchestrator"
  "internal/modules/blocks/run"
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

# Component type is determined by responsibility and directory placement.
# Keep this inventory explicit so a new unit cannot silently bypass
# type/ownership review.
initiator_class_roots=(
  "internal/initiators/workorchruntime"
)
module_class_roots=(
  "internal/modules/application/workorchd"
  "internal/modules/kernel/governance"
  "internal/modules/kernel/orchestrator"
  "internal/modules/kernel/system"
)
block_class_roots=(
  "internal/modules/blocks/eventstream"
  "internal/modules/blocks/run"
  "internal/modules/blocks/approval"
  "internal/modules/blocks/decision"
  "internal/modules/blocks/file"
  "internal/modules/blocks/git"
  "internal/modules/blocks/llm"
  "internal/modules/blocks/mcp"
  "internal/modules/blocks/mcpregistry"
  "internal/modules/blocks/memory"
  "internal/modules/blocks/result"
  "internal/modules/blocks/shell"
  "internal/modules/blocks/skill"
  "internal/modules/blocks/soul"
  "internal/modules/blocks/ssh"
  "internal/modules/blocks/web"
  "internal/modules/blocks/workspace"
)

if ((${#initiator_class_roots[@]} != 1 || ${#module_class_roots[@]} != 4 || ${#block_class_roots[@]} != 17)); then
  echo "boundary contract violation: unit inventory must remain 1 Initiator / 4 Module / 17 Block" >&2
  exit 1
fi

for component_root in "${initiator_class_roots[@]}" "${module_class_roots[@]}" "${block_class_roots[@]}"; do
  if [[ ! -d "$component_root" ]]; then
    echo "boundary contract violation: declared component root does not exist: $component_root" >&2
    exit 1
  fi
done

# Every framework Module/Block in this repository currently participates in
# EventHub lifecycle. Its root is only a lifecycle bridge; the component-local
# Biz must be the sole holder of the Base/EventHub interaction. The repository
# provider remains an Initiator, so it is intentionally excluded here.
mapfile -t component_roots < <(find internal/modules -mindepth 2 -maxdepth 3 -name module.go -printf '%h\n' | sort)
mapfile -t declared_component_roots < <(printf '%s\n' "${module_class_roots[@]}" "${block_class_roots[@]}" | sort)
if [[ "$(printf '%s\n' "${component_roots[@]}")" != "$(printf '%s\n' "${declared_component_roots[@]}")" ]]; then
  cat >&2 <<EOF
Boundary contract violation: framework component inventory differs from the
declared 4 Module / 17 Block type baseline.

Discovered:
$(printf '%s\n' "${component_roots[@]}")

Declared:
$(printf '%s\n' "${declared_component_roots[@]}")

Classify the unit by responsibility before changing this inventory.
EOF
  exit 1
fi
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

# Workspace scope is per EventHub execution, never component-instance state.
# The state owners and Runtime drive must use the checked binder rather
# than silently replacing a pre-existing workspace context from a command.
workspace_scope_bindings=(
  "internal/modules/blocks/run/biz/owner.go"
  "internal/modules/blocks/run/biz/lifecycle_handler.go"
  "internal/modules/blocks/workspace/biz/owner.go"
  "internal/modules/blocks/result/biz/owner.go"
  "internal/modules/blocks/result/biz/lifecycle_handler.go"
  "internal/modules/blocks/decision/biz/owner.go"
  "internal/modules/blocks/approval/biz/owner.go"
  "internal/modules/blocks/memory/biz/owner.go"
  "internal/modules/blocks/eventstream/biz/event_stream_service.go"
  "internal/modules/kernel/orchestrator/biz/runtime_drive.go"
  "internal/modules/kernel/orchestrator/biz/runtime_operation_executor.go"
  "internal/modules/kernel/orchestrator/biz/governance_evidence_command.go"
  "internal/modules/kernel/orchestrator/biz/owner.go"
  "internal/modules/kernel/governance/biz/service.go"
  "internal/modules/application/workorchd/biz/owner.go"
)
for path in "${workspace_scope_bindings[@]}"; do
  if ! grep -q -E -- 'BindRuntimeWorkspaceContext|resultCommandContext' "$path"; then
    echo "boundary contract violation: workspace-scoped owner path must validate EventHub context: $path" >&2
    exit 1
  fi
done

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
mapfile -t event_contract_component_roots < <(printf '%s\n' "${event_contract_dirs[@]}" | sed 's#/pkg/events$##' | sort)
mapfile -t expected_event_contract_component_roots < <(
  printf '%s\n' "${module_class_roots[@]}" "${block_class_roots[@]}" |
    grep -v -F -- 'internal/modules/kernel/system' |
    sort
)
if [[ "$(printf '%s\n' "${event_contract_component_roots[@]}")" != "$(printf '%s\n' "${expected_event_contract_component_roots[@]}")" ]]; then
  cat >&2 <<EOF
Boundary contract violation: owner-local pkg/events coverage differs from the
declared event-owning units. The Initiator and kernel/system intentionally own
no EventHub contracts; every other Module/Block must define contracts locally.

Discovered:
$(printf '%s\n' "${event_contract_component_roots[@]}")

Expected:
$(printf '%s\n' "${expected_event_contract_component_roots[@]}")
EOF
  exit 1
fi

matches="$(source_matches 'github.com/muidea/workorch/internal/modules/' internal/pkg/events)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: internal/pkg/events must remain owner-neutral and
must not import component event packages to build a global topic index.

$matches
EOF
  exit 1
fi

# The shared EventHub helper must not grow back into a repository-wide topic,
# message, version or type registry. Owner packages provide Contracts()
# explicitly at each call site; cross-owner aggregation is test/CI-only.
shared_event_registry_pattern='eventHubContracts|eventHubMessageContracts|workorchEventTopics|workorchEventContracts|ResolveEventHubContract|ResolveEventHubMessageContract|EventHubContractInventory|WorkorchEventContract|ValidateWorkorchEventVersion|map\[string\](eventcontract\.)?(Contract|MessageContract)|map\[string\]reflect\.Type'
matches="$(source_matches "$shared_event_registry_pattern" internal/pkg/events)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: internal/pkg/events contains a production global
topic, message, version or type registry.

$matches

Keep owner contracts in the receiving component pkg/events and pass that
owner's Contracts() explicitly to owner-neutral helpers. Repository-wide
aggregation is allowed only in *_test.go or CI validation code.
EOF
  exit 1
fi

if [[ -d internal/modules/kernel/system/pkg/events ]]; then
  echo "boundary contract violation: System sender must consume receiver-owner contracts; it must not own cross-owner pkg/events" >&2
  exit 1
fi
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

# Runtime capability execution wire contracts belong to the receiving owner,
# not to the Orchestrator that sends them. File is the first closed migration
# slice: keep its topics, concrete commands/results and operation mapper in the
# File Block package and prevent the deleted Orchestrator surface from
# returning under an alias or a second topic name.
file_contract_root="internal/modules/blocks/file/pkg/events"
for required_file in topics.go contracts.go messages.go operation.go; do
  if [[ ! -f "$file_contract_root/$required_file" ]]; then
    echo "boundary contract violation: File owner execution contract is incomplete: $file_contract_root/$required_file" >&2
    exit 1
  fi
done

legacy_orchestrator_file_contract_pattern='TopicOrchestratorExecute(File|Patch)|type Execute(File|Patch)[A-Za-z0-9_]*(Command|Result)|type (File(List|Read|Edit|Write)|Patch(Apply|Preview))OperationData|func (File(List|Read|Edit|Write)|Patch(Apply|Preview))OperationDataFromRuntimeOperation'
matches="$(source_matches "$legacy_orchestrator_file_contract_pattern" internal/modules/kernel/orchestrator/pkg/events)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: File execution wire contract is defined by the
Orchestrator instead of the receiving File owner.

$matches

Keep File topics, concrete Command/Data/Result and pure operation mapping in
internal/modules/blocks/file/pkg/events. Do not add aliases, dual topics or a
forwarding compatibility bridge in kernel/orchestrator/pkg/events.
EOF
  exit 1
fi

legacy_orchestrator_file_query_pattern='TopicOrchestrator(AcquireTargetSnapshot|AcquireRuntimeTargetSelection|AcquireWorkspaceManifest|ConfirmRuntimeRepairSurface)|type (AcquireTargetSnapshot|AcquireRuntimeTargetSelection|AcquireWorkspaceManifest|ConfirmRuntimeRepairSurface)(Command|Result)'
matches="$(source_matches "$legacy_orchestrator_file_query_pattern" internal/modules/kernel/orchestrator/pkg/events)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: File prerequisite/query wire contract is defined
by the Orchestrator instead of the receiving File owner.

$matches

Keep target snapshot/selection, workspace manifest and repair-surface query
topics plus concrete Command/Result types in blocks/file/pkg/events. Do not
add aliases, old-topic forwarding or dual subscriptions.
EOF
  exit 1
fi

for required_symbol in \
  TopicAcquireTargetSnapshot \
  TopicAcquireRuntimeTargetSelection \
  TopicAcquireWorkspaceManifest \
  TopicConfirmRuntimeRepairSurface; do
  if ! grep -q -F -- "$required_symbol" "$file_contract_root/topics.go" ||
     ! grep -q -F -- "fileevents.$required_symbol" internal/modules/blocks/file/biz/owner.go; then
    echo "boundary contract violation: File owner query contract is not registered and consumed: $required_symbol" >&2
    exit 1
  fi
done

for result_name in AcquireTargetSnapshotResult AcquireRuntimeTargetSelectionResult AcquireWorkspaceManifestResult; do
  result_shape="$(sed -n "/^type ${result_name} struct {/,/^}/p" "$file_contract_root/messages.go")"
  if ! grep -q -E -- 'WorkspaceID[[:space:]]+common\.WorkspaceID' <<<"$result_shape" ||
     ! grep -q -E -- 'RunID[[:space:]]+common\.RunID' <<<"$result_shape" ||
     ! grep -q -E -- 'AuthoritySnapshotID[[:space:]]+string' <<<"$result_shape" ||
     ! grep -q -E -- 'Observation[[:space:]]+runtimecontract\.RuntimePrerequisiteObservation' <<<"$result_shape" ||
     grep -q -E -- 'PrerequisiteID[[:space:]]+' <<<"$result_shape"; then
    echo "boundary contract violation: File ${result_name} must be scoped and derive prerequisite identity from its required canonical observation/payload" >&2
    exit 1
  fi
done

if ! grep -q -F -- 'blocks/file/pkg/events' internal/modules/kernel/orchestrator/biz/prerequisite_event_adapter.go ||
   ! grep -q -F -- 'fileevents.TopicAcquireWorkspaceManifest' internal/modules/blocks/git/biz/owner.go; then
  echo "boundary contract violation: Orchestrator or Git does not consume the File owner query contract" >&2
  exit 1
fi

if ! grep -q -F -- 'blocks/file/pkg/events' internal/modules/kernel/orchestrator/biz/runtime_operation_executor.go ||
   ! grep -q -E -- 'fileevents\.TopicExecute(File|Patch)' internal/modules/kernel/orchestrator/biz/runtime_operation_executor.go; then
  echo "boundary contract violation: Orchestrator executor does not consume the File owner execution contract" >&2
  exit 1
fi

matches="$(source_matches 'TopicOrchestratorExecute(File|Patch)' internal/modules/blocks/file/biz)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: File Biz still consumes a deleted Orchestrator
File execution topic.

$matches

Subscribe only to the File owner topics declared in blocks/file/pkg/events.
EOF
  exit 1
fi

shell_contract_root="internal/modules/blocks/shell/pkg/events"
for required_file in topics.go contracts.go messages.go operation.go; do
  if [[ ! -f "$shell_contract_root/$required_file" ]]; then
    echo "boundary contract violation: Shell owner execution contract is incomplete: $shell_contract_root/$required_file" >&2
    exit 1
  fi
done

legacy_orchestrator_shell_contract_pattern='TopicOrchestratorExecuteShellOperation|type ExecuteShellOperation(Command|Result)|type ShellOperationData|func ShellOperationDataFromRuntimeOperation'
matches="$(source_matches "$legacy_orchestrator_shell_contract_pattern" internal/modules/kernel/orchestrator/pkg/events)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: Shell execution wire contract is defined by the
Orchestrator instead of the receiving Shell owner.

$matches

Keep Shell topic, concrete Command/Data/Result and operation mapping in
internal/modules/blocks/shell/pkg/events without aliases or forwarding.
EOF
  exit 1
fi

if ! grep -q -F -- 'blocks/shell/pkg/events' internal/modules/kernel/orchestrator/biz/runtime_operation_executor.go ||
   ! grep -q -F -- 'shellevents.TopicExecuteOperation' internal/modules/kernel/orchestrator/biz/runtime_operation_executor.go; then
  echo "boundary contract violation: Orchestrator executor does not consume the Shell owner execution contract" >&2
  exit 1
fi

matches="$(source_matches 'TopicOrchestratorExecuteShellOperation' internal/modules/blocks/shell/biz)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: Shell Biz still consumes a deleted Orchestrator
Shell execution topic.

$matches
EOF
  exit 1
fi

git_contract_root="internal/modules/blocks/git/pkg/events"
for required_file in topics.go contracts.go messages.go operation.go; do
  if [[ ! -f "$git_contract_root/$required_file" ]]; then
    echo "boundary contract violation: Git owner execution contract is incomplete: $git_contract_root/$required_file" >&2
    exit 1
  fi
done

legacy_orchestrator_git_contract_pattern='TopicOrchestratorExecuteGitOperation|type ExecuteGitOperation(Command|Result)|type GitOperationData|func GitOperationDataFromRuntimeOperation'
matches="$(source_matches "$legacy_orchestrator_git_contract_pattern" internal/modules/kernel/orchestrator/pkg/events)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: Git execution wire contract is defined by the
Orchestrator instead of the receiving Git owner.

$matches
EOF
  exit 1
fi

matches="$(source_matches 'TopicOrchestratorAcquireGitCapabilityState|type AcquireGitCapabilityState(Command|Result)' internal/modules/kernel/orchestrator/pkg/events)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: Git capability-state query contract is defined by
the Orchestrator instead of the receiving Git owner.

$matches
EOF
  exit 1
fi

if ! grep -q -F -- 'TopicAcquireCapabilityState' "$git_contract_root/topics.go" ||
   ! grep -q -F -- 'gitevents.TopicAcquireCapabilityState' internal/modules/blocks/git/biz/owner.go ||
   ! grep -q -F -- 'blocks/git/pkg/events' internal/modules/kernel/orchestrator/biz/prerequisite_event_adapter.go; then
  echo "boundary contract violation: Git capability-state owner contract is not registered or consumed" >&2
  exit 1
fi

if ! grep -q -F -- 'blocks/git/pkg/events' internal/modules/kernel/orchestrator/biz/runtime_operation_executor.go ||
   ! grep -q -F -- 'gitevents.TopicExecuteOperation' internal/modules/kernel/orchestrator/biz/runtime_operation_executor.go; then
  echo "boundary contract violation: Orchestrator executor does not consume the Git owner execution contract" >&2
  exit 1
fi

matches="$(source_matches 'TopicOrchestratorExecuteGitOperation' internal/modules/blocks/git/biz)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: Git Biz still consumes a deleted Orchestrator Git
execution topic.

$matches
EOF
  exit 1
fi

for owner in mcp ssh web; do
  owner_contract_root="internal/modules/blocks/$owner/pkg/events"
  for required_file in topics.go contracts.go messages.go operation.go; do
    if [[ ! -f "$owner_contract_root/$required_file" ]]; then
      echo "boundary contract violation: $owner owner execution contract is incomplete: $owner_contract_root/$required_file" >&2
      exit 1
    fi
  done
  case "$owner" in
    mcp) legacy_name="MCP" ;;
    ssh) legacy_name="SSH" ;;
    web) legacy_name="Web" ;;
  esac
  legacy_pattern="TopicOrchestratorExecute${legacy_name}Operation|type Execute${legacy_name}Operation(Command|Result)|type ${legacy_name}OperationData|func ${legacy_name}OperationDataFromRuntimeOperation"
  matches="$(source_matches "$legacy_pattern" internal/modules/kernel/orchestrator/pkg/events)"
  if [[ -n "$matches" ]]; then
    cat >&2 <<EOF
Boundary contract violation: $owner execution wire contract is defined by the
Orchestrator instead of the receiving owner.

$matches
EOF
    exit 1
  fi
  if ! grep -q -F -- "blocks/$owner/pkg/events" internal/modules/kernel/orchestrator/biz/runtime_operation_executor.go; then
    echo "boundary contract violation: Orchestrator executor does not import the $owner owner execution contract" >&2
    exit 1
  fi
  matches="$(source_matches "TopicOrchestratorExecute${legacy_name}Operation" "internal/modules/blocks/$owner/biz")"
  if [[ -n "$matches" ]]; then
    echo "boundary contract violation: $owner Biz still consumes a deleted Orchestrator execution topic" >&2
    echo "$matches" >&2
    exit 1
  fi
done

# Provider ceiling acquisition, request rendering preflight and provider
# submission are consumed by the LLM Block. Their concrete EventHub contract
# therefore belongs to blocks/llm/pkg/events, even though Orchestrator drives
# the persisted SM-3 request session around those calls.
llm_contract_root="internal/modules/blocks/llm/pkg/events"
for required_file in topics.go contracts.go messages.go; do
  if [[ ! -f "$llm_contract_root/$required_file" ]]; then
    echo "boundary contract violation: LLM owner Runtime provider contract is incomplete: $llm_contract_root/$required_file" >&2
    exit 1
  fi
done

legacy_orchestrator_llm_contract_pattern='TopicOrchestrator(AcquireProviderCeiling|PreflightRuntimeCandidate|RequestRuntimeCandidate)|type (AcquireProviderCeiling|PreflightRuntimeCandidate|RequestRuntimeCandidate)(Command|Result)'
matches="$(source_matches "$legacy_orchestrator_llm_contract_pattern" internal/modules/kernel/orchestrator/pkg/events)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: LLM Runtime provider wire contract is defined by
the Orchestrator instead of the receiving LLM owner.

$matches

Keep provider ceiling, preflight and request topics plus their concrete
Command/Result types in internal/modules/blocks/llm/pkg/events. Do not add an
alias, dual subscription or forwarding compatibility bridge.
EOF
  exit 1
fi

for adapter_path in \
  internal/modules/kernel/orchestrator/biz/prerequisite_event_adapter.go \
  internal/modules/kernel/orchestrator/biz/runtime_candidate_event_adapter.go; do
  if ! grep -q -F -- 'blocks/llm/pkg/events' "$adapter_path"; then
    echo "boundary contract violation: Orchestrator adapter does not import the LLM owner Runtime provider contract: $adapter_path" >&2
    exit 1
  fi
done

if ! grep -q -F -- 'llmevents.TopicAcquireProviderCeiling' internal/modules/blocks/llm/biz/owner.go ||
   ! grep -q -F -- 'llmevents.TopicPreflightRuntimeCandidate' internal/modules/blocks/llm/biz/owner.go ||
   ! grep -q -F -- 'llmevents.TopicRequestRuntimeCandidate' internal/modules/blocks/llm/biz/owner.go; then
  echo "boundary contract violation: LLM Biz does not consume all LLM owner Runtime provider topics" >&2
  exit 1
fi

provider_ceiling_result_shape="$(sed -n '/^type AcquireProviderCeilingResult struct {/,/^}/p' "$llm_contract_root/messages.go")"
if ! grep -q -E -- 'WorkspaceID[[:space:]]+common\.WorkspaceID' <<<"$provider_ceiling_result_shape" ||
   ! grep -q -E -- 'RunID[[:space:]]+common\.RunID' <<<"$provider_ceiling_result_shape" ||
   ! grep -q -E -- 'AuthoritySnapshotID[[:space:]]+string' <<<"$provider_ceiling_result_shape" ||
   ! grep -q -E -- 'Observation[[:space:]]+runtimecontract\.RuntimePrerequisiteObservation' <<<"$provider_ceiling_result_shape" ||
   grep -q -E -- 'PrerequisiteID[[:space:]]+' <<<"$provider_ceiling_result_shape"; then
  echo "boundary contract violation: LLM provider ceiling Result must be scoped and derive prerequisite identity from its required canonical observation" >&2
  exit 1
fi

git_capability_state_result_shape="$(sed -n '/^type AcquireCapabilityStateResult struct {/,/^}/p' internal/modules/blocks/git/pkg/events/messages.go)"
if ! grep -q -E -- 'WorkspaceID[[:space:]]+common\.WorkspaceID' <<<"$git_capability_state_result_shape" ||
   ! grep -q -E -- 'RunID[[:space:]]+common\.RunID' <<<"$git_capability_state_result_shape" ||
   ! grep -q -E -- 'AuthoritySnapshotID[[:space:]]+string' <<<"$git_capability_state_result_shape" ||
   ! grep -q -E -- 'Observation[[:space:]]+runtimecontract\.RuntimePrerequisiteObservation' <<<"$git_capability_state_result_shape" ||
   grep -q -E -- 'PrerequisiteID[[:space:]]+' <<<"$git_capability_state_result_shape"; then
  echo "boundary contract violation: Git capability-state Result must be scoped and derive prerequisite identity from its canonical snapshot/observation" >&2
  exit 1
fi

# Opening a governance ticket changes Decision-owned state. The concrete write
# contract therefore belongs to Decision even though Orchestrator supplies the
# sealed dependency fact and persists its own acknowledgement afterward.
matches="$(source_matches 'TopicOrchestratorOpenRuntimeGovernanceRequest|type OpenRuntimeGovernanceRequest(Command|Result)' internal/modules/kernel/orchestrator/pkg/events)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: Runtime governance request wire contract is
defined by Orchestrator instead of the receiving Decision owner.

$matches
EOF
  exit 1
fi

if ! grep -q -F -- 'TopicOpenRuntimeGovernanceRequest' internal/modules/blocks/decision/pkg/events/topics.go ||
   ! grep -q -F -- 'decisionevents.TopicOpenRuntimeGovernanceRequest' internal/modules/blocks/decision/biz/owner.go ||
   ! grep -q -F -- 'blocks/decision/pkg/events' internal/modules/kernel/orchestrator/biz/runtime_governance_request_adapter.go; then
  echo "boundary contract violation: Decision owner Runtime governance request contract is not registered or consumed" >&2
  exit 1
fi

# Runtime capability acquisition is a read-only owner fact, so every
# receiving capability owner defines and consumes its own concrete wire
# contract. Orchestrator may normalize the concrete results internally but
# must not retain a shared multi-receiver topic/type or forwarding bridge.
legacy_orchestrator_capability_pattern='TopicOrchestratorAcquireRuntimeCapabilities|type AcquireRuntimeCapabilities(Command|Result)|func ResolveRuntimeCapabilities|type RuntimeCapabilityUnsupportedError'
matches="$(source_matches "$legacy_orchestrator_capability_pattern" internal/modules/kernel/orchestrator/pkg/events)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: Runtime capability acquisition wire contract is
defined by Orchestrator instead of each receiving capability owner.

$matches
EOF
  exit 1
fi

for owner in decision file git llm mcp shell ssh web; do
  owner_contract_root="internal/modules/blocks/$owner/pkg/events"
  if [[ ! -f "$owner_contract_root/runtime_capability.go" ]] ||
     ! grep -q -F -- 'TopicAcquireRuntimeCapabilities' "$owner_contract_root/topics.go" ||
     ! grep -q -F -- 'AcquireRuntimeCapabilitiesCommand' "$owner_contract_root/runtime_capability.go" ||
     ! grep -q -F -- "${owner}events.TopicAcquireRuntimeCapabilities" "internal/modules/blocks/$owner/biz/owner.go"; then
    echo "boundary contract violation: $owner Runtime capability owner contract is not defined and consumed" >&2
    exit 1
  fi
  if ! grep -q -F -- "blocks/$owner/pkg/events" internal/modules/kernel/orchestrator/biz/prerequisite_event_adapter.go; then
    echo "boundary contract violation: Orchestrator prerequisite adapter does not import the $owner Runtime capability contract" >&2
    exit 1
  fi
  capability_result_shape="$(sed -n '/^type AcquireRuntimeCapabilitiesResult struct {/,/^}/p' "$owner_contract_root/runtime_capability.go")"
  if ! grep -q -E -- 'Acquisition[[:space:]]+runtimecontract\.RuntimeCapabilityAcquisition' <<<"$capability_result_shape" ||
     grep -q -E -- 'PrerequisiteID|Capabilities[[:space:]]+runtimecontract|Observation[[:space:]]+' <<<"$capability_result_shape"; then
    echo "boundary contract violation: $owner Runtime capability Result must expose only the canonical scoped acquisition" >&2
    exit 1
  fi
done

capability_acquisition_shape="$(sed -n '/^type RuntimeCapabilityAcquisition struct {/,/^}/p' internal/pkg/workorchruntimecontract/runtime_capability_acquisition.go)"
if ! grep -q -E -- 'WorkspaceID[[:space:]]+common\.WorkspaceID' <<<"$capability_acquisition_shape" ||
   ! grep -q -E -- 'RunID[[:space:]]+common\.RunID' <<<"$capability_acquisition_shape" ||
   ! grep -q -E -- 'AuthoritySnapshotID[[:space:]]+string' <<<"$capability_acquisition_shape" ||
   ! grep -q -E -- 'Observation[[:space:]]+RuntimePrerequisiteObservation' <<<"$capability_acquisition_shape" ||
   grep -q -E -- 'PrerequisiteID[[:space:]]+' <<<"$capability_acquisition_shape"; then
  echo "boundary contract violation: canonical Runtime capability acquisition must be scoped and derive prerequisite identity from its required observation" >&2
  exit 1
fi

legacy_orchestrator_admissibility_pattern='TopicOrchestratorAcquireRuntimeOperationAdmissibility|type AcquireRuntimeOperationAdmissibility(Command|Result)|func ValidateRuntimeOperationAdmissibilityCommand|func NewRuntimeOperationAdmissibility'
matches="$(source_matches "$legacy_orchestrator_admissibility_pattern" internal/modules/kernel/orchestrator/pkg/events)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: operation admissibility acquisition is defined by
Orchestrator instead of each receiving capability owner.

$matches
EOF
  exit 1
fi

for owner in file git mcp shell ssh web; do
  owner_contract_root="internal/modules/blocks/$owner/pkg/events"
  if [[ ! -f "$owner_contract_root/runtime_admissibility.go" ]] ||
     ! grep -q -F -- 'TopicAcquireRuntimeOperationAdmissibility' "$owner_contract_root/topics.go" ||
     ! grep -q -F -- 'AcquireRuntimeOperationAdmissibilityCommand' "$owner_contract_root/runtime_admissibility.go" ||
     ! grep -q -F -- "${owner}events.TopicAcquireRuntimeOperationAdmissibility" "internal/modules/blocks/$owner/biz/owner.go"; then
    echo "boundary contract violation: $owner operation admissibility owner contract is not defined and consumed" >&2
    exit 1
  fi
  if ! grep -q -F -- "blocks/$owner/pkg/events" internal/modules/kernel/orchestrator/biz/runtime_operation_admissibility_event_adapter.go; then
    echo "boundary contract violation: Orchestrator admissibility adapter does not import the $owner owner contract" >&2
    exit 1
  fi
  admissibility_result_shape="$(sed -n '/^type AcquireRuntimeOperationAdmissibilityResult struct {/,/^}/p' "$owner_contract_root/runtime_admissibility.go")"
  if ! grep -q -E -- 'Acquisition[[:space:]]+runtimecontract\.RuntimeOperationAdmissibilityAcquisition' <<<"$admissibility_result_shape" ||
     grep -q -E -- 'PrerequisiteID|Admissibility[[:space:]]+runtimecontract\.RuntimeOperationAdmissibility|Observation[[:space:]]+' <<<"$admissibility_result_shape"; then
    echo "boundary contract violation: $owner operation admissibility Result must expose only the canonical scoped acquisition" >&2
    exit 1
  fi
done

admissibility_acquisition_shape="$(sed -n '/^type RuntimeOperationAdmissibilityAcquisition struct {/,/^}/p' internal/pkg/workorchruntimecontract/runtime_operation_admissibility_acquisition.go)"
if ! grep -q -E -- 'WorkspaceID[[:space:]]+common\.WorkspaceID' <<<"$admissibility_acquisition_shape" ||
   ! grep -q -E -- 'RunID[[:space:]]+common\.RunID' <<<"$admissibility_acquisition_shape" ||
   ! grep -q -E -- 'AuthoritySnapshotID[[:space:]]+string' <<<"$admissibility_acquisition_shape" ||
   ! grep -q -E -- 'Observation[[:space:]]+RuntimePrerequisiteObservation' <<<"$admissibility_acquisition_shape" ||
   grep -q -E -- 'PrerequisiteID[[:space:]]+' <<<"$admissibility_acquisition_shape"; then
  echo "boundary contract violation: canonical operation admissibility acquisition must be scoped and derive prerequisite identity from its inventory/observation" >&2
  exit 1
fi

# Operation feasibility has one factual outcome authority. Feedback, stale and
# prerequisite values are outcome-specific detail. A parallel Feasible bool or
# FailureFact would recreate ambiguous outcome/failure authority in every owner
# adapter.
for owner in file git mcp shell ssh web; do
  feasibility_contract="internal/modules/blocks/$owner/pkg/events/runtime_admissibility.go"
  feasibility_shape="$(sed -n '/^type ProbeRuntimeOperationFeasibilityResult struct {/,/^}/p' "$feasibility_contract")"
  if ! grep -q -E -- 'Outcome[[:space:]]+runtimecontract\.RuntimeOwnerFeasibilityOutcome' <<<"$feasibility_shape" ||
     ! grep -q -E -- 'Feedback[[:space:]]+\*runtimecontract\.RuntimeFeedback' <<<"$feasibility_shape" ||
     grep -q -E -- 'Feasible[[:space:]]+bool|Failure[[:space:]]+\*runtimecontract\.FailureFact|Category[[:space:]]+runtimecontract\.RuntimeOwnerFeasibilityCategory' <<<"$feasibility_shape"; then
    echo "boundary contract violation: $owner operation feasibility must expose Outcome + Feedback without parallel Feasible/Failure/Category authority" >&2
    exit 1
  fi
done
neutral_feasibility_shape="$(sed -n '/^type RuntimeOperationFeasibilityObservation struct {/,/^}/p' internal/pkg/workorchruntimecontract/runtime_operation_admissibility_acquisition.go)"
if ! grep -q -E -- 'Outcome[[:space:]]+RuntimeOwnerFeasibilityOutcome' <<<"$neutral_feasibility_shape" ||
   ! grep -q -E -- 'Feedback[[:space:]]+\*RuntimeFeedback' <<<"$neutral_feasibility_shape" ||
   grep -q -E -- 'Feasible[[:space:]]+bool|Failure[[:space:]]+\*FailureFact|Category[[:space:]]+RuntimeOwnerFeasibilityCategory' <<<"$neutral_feasibility_shape"; then
  echo "boundary contract violation: owner-neutral operation feasibility must expose Outcome + Feedback without parallel Feasible/Failure/Category authority" >&2
  exit 1
fi
persisted_feasibility_shape="$(sed -n '/^type RuntimeOwnerFeasibilityObservation struct {/,/^}/p' internal/pkg/workorchruntimecontract/runtime_owner_feasibility.go)"
if ! grep -q -E -- 'Outcome[[:space:]]+RuntimeOwnerFeasibilityOutcome' <<<"$persisted_feasibility_shape" ||
   ! grep -q -E -- 'Feedback[[:space:]]+\*RuntimeFeedback' <<<"$persisted_feasibility_shape" ||
   grep -q -E -- '(Feasible[[:space:]]+bool|Failure[[:space:]]+\*FailureFact|Category[[:space:]]+RuntimeOwnerFeasibilityCategory|Code[[:space:]]+string|Signature[[:space:]]+string|Reason[[:space:]]+string|TargetRefs[[:space:]]+\[\]string)' <<<"$persisted_feasibility_shape"; then
  echo "boundary contract violation: persisted owner feasibility retains duplicate outcome or diagnostic authority" >&2
  exit 1
fi
candidate_feasibility_shape="$(sed -n '/^type RuntimeCandidateFeasibilityRecord struct {/,/^}/p' internal/pkg/workorchruntimecontract/runtime_state.go)"
if ! grep -q -E -- 'Observation[[:space:]]+\*RuntimeOwnerFeasibilityObservation' <<<"$candidate_feasibility_shape" ||
   grep -q -E -- 'Feasible[[:space:]]+bool|Failure[[:space:]]+\*' <<<"$candidate_feasibility_shape"; then
  echo "boundary contract violation: candidate feasibility ledger must persist only the canonical owner observation" >&2
  exit 1
fi
run_state_update_shape="$(sed -n '/^type RuntimeStateUpdateResult struct {/,/^}/p' internal/modules/blocks/run/pkg/events/messages.go)"
if ! grep -q -E -- 'Feedback[[:space:]]+runtimecontract\.RuntimeFeedback' <<<"$run_state_update_shape" ||
   grep -q -E -- '(Disposition|Status|Outcome)[[:space:]]+' <<<"$run_state_update_shape"; then
  echo "boundary contract violation: Run aggregate persistence Result must use one RuntimeFeedback stable outcome" >&2
  exit 1
fi
operation_receipt_shape="$(sed -n '/^type RuntimeOperationReceipt struct {/,/^}/p' internal/pkg/workorchruntimecontract/runtime_operation_receipt.go)"
if ! grep -q -E -- 'Outcome[[:space:]]+RuntimeOperationOutcome' <<<"$operation_receipt_shape" ||
   ! grep -q -E -- 'Feedback[[:space:]]+\*RuntimeFeedback' <<<"$operation_receipt_shape" ||
   grep -q -E -- '(Failure[[:space:]]+\*FailureFact|Status[[:space:]]+)' <<<"$operation_receipt_shape"; then
  echo "boundary contract violation: operation receipt must expose Outcome + Feedback without parallel Failure/Status authority" >&2
  exit 1
fi
for owner in file git mcp shell ssh web; do
  execute_result_shape="$(sed -n '/^type ExecuteOperationResult struct {/,/^}/p' "internal/modules/blocks/${owner}/pkg/events/messages.go")"
  if ! grep -q -E -- 'Receipt[[:space:]]+runtimecontract\.RuntimeOperationReceipt' <<<"$execute_result_shape" ||
     grep -q -E -- '(EventID|OperationID|EvidenceKind|SourceRef)[[:space:]]+' <<<"$execute_result_shape"; then
    echo "boundary contract violation: ${owner} execute Result must return only the canonical owner receipt without derived evidence identity echoes" >&2
    exit 1
  fi
done

legacy_orchestrator_feasibility_pattern='TopicOrchestratorProbeRuntimeOperationFeasibility|type ProbeRuntimeOperationFeasibility(Command|Result)|func (ValidateRuntimeOperationFeasibilityCommand|NewProbeRuntimeOperationFeasibilityResult|NormalizeProbeRuntimeOperationFeasibilityResult)'
matches="$(source_matches "$legacy_orchestrator_feasibility_pattern" internal/modules/kernel/orchestrator/pkg/events)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: operation feasibility is defined by Orchestrator
instead of each receiving capability owner.

$matches
EOF
  exit 1
fi

for owner in file git mcp shell ssh web; do
  owner_contract_root="internal/modules/blocks/$owner/pkg/events"
  if [[ ! -f "$owner_contract_root/runtime_admissibility.go" ]] ||
     ! grep -q -F -- 'TopicProbeRuntimeOperationFeasibility' "$owner_contract_root/topics.go" ||
     ! grep -q -F -- 'ProbeRuntimeOperationFeasibilityCommand' "$owner_contract_root/runtime_admissibility.go" ||
     ! grep -q -F -- "${owner}events.TopicProbeRuntimeOperationFeasibility" "internal/modules/blocks/$owner/biz/owner.go"; then
    echo "boundary contract violation: $owner operation feasibility owner contract is not defined and consumed" >&2
    exit 1
  fi
  if ! grep -q -F -- "${owner}events.TopicProbeRuntimeOperationFeasibility" internal/modules/kernel/orchestrator/biz/runtime_operation_admissibility_event_adapter.go; then
    echo "boundary contract violation: Orchestrator feasibility adapter does not dispatch the $owner owner contract" >&2
    exit 1
  fi
done

legacy_orchestrator_semantic_feasibility_pattern='TopicOrchestratorProbeRuntimeSemanticProofFeasibility|type ProbeRuntimeSemanticProofFeasibility(Command|Result)|func (ValidateRuntimeSemanticProofFeasibilityCommand|NewRuntimeSemanticProofFeasibilityResult|RuntimeSemanticFeasibilityDiagnostic|RuntimeSemanticProposalTargetRefs)'
matches="$(source_matches "$legacy_orchestrator_semantic_feasibility_pattern" internal/modules/kernel/orchestrator/pkg/events)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: semantic-proof feasibility is defined by
Orchestrator instead of each receiving capability owner.

$matches
EOF
  exit 1
fi

for owner in file git mcp shell ssh web; do
  owner_contract_root="internal/modules/blocks/$owner/pkg/events"
  if [[ ! -f "$owner_contract_root/runtime_admissibility.go" ]] ||
     ! grep -q -F -- 'TopicProbeRuntimeSemanticProofFeasibility' "$owner_contract_root/topics.go" ||
     ! grep -q -F -- 'ProbeRuntimeSemanticProofFeasibilityCommand' "$owner_contract_root/runtime_admissibility.go" ||
     ! grep -q -F -- "${owner}events.TopicProbeRuntimeSemanticProofFeasibility" "internal/modules/blocks/$owner/biz/owner.go"; then
    echo "boundary contract violation: $owner semantic-proof feasibility owner contract is not defined and consumed" >&2
    exit 1
  fi
  if ! grep -q -F -- "${owner}events.TopicProbeRuntimeSemanticProofFeasibility" internal/modules/kernel/orchestrator/biz/runtime_semantic_feasibility_event_adapter.go; then
    echo "boundary contract violation: Orchestrator semantic feasibility adapter does not dispatch the $owner owner contract" >&2
    exit 1
  fi
done

legacy_orchestrator_cwi_reconciliation_pattern='TopicOrchestratorQueryRuntimeCWIReconciliation|type QueryRuntimeCWIReconciliation(Command|Result)'
matches="$(source_matches "$legacy_orchestrator_cwi_reconciliation_pattern" internal/modules/kernel/orchestrator/pkg/events)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: CWI reconciliation query is defined by
Orchestrator instead of each receiving capability owner.

$matches
EOF
  exit 1
fi

for owner in file git mcp shell; do
  owner_contract_root="internal/modules/blocks/$owner/pkg/events"
  if [[ ! -f "$owner_contract_root/runtime_reconciliation.go" ]] ||
     ! grep -q -F -- 'TopicQueryRuntimeCWIReconciliation' "$owner_contract_root/topics.go" ||
     ! grep -q -F -- 'QueryRuntimeCWIReconciliationCommand' "$owner_contract_root/runtime_reconciliation.go" ||
     ! grep -q -F -- "${owner}events.TopicQueryRuntimeCWIReconciliation" "internal/modules/blocks/$owner/biz/owner.go"; then
    echo "boundary contract violation: $owner CWI reconciliation owner contract is not defined and consumed" >&2
    exit 1
  fi
  if ! grep -q -F -- "${owner}events.TopicQueryRuntimeCWIReconciliation" internal/modules/kernel/orchestrator/biz/runtime_reconciliation.go; then
    echo "boundary contract violation: Orchestrator reconciliation adapter does not dispatch the $owner owner contract" >&2
    exit 1
  fi
  reconciliation_result_shape="$(sed -n '/^type QueryRuntimeCWIReconciliationResult struct {/,/^}/p' "$owner_contract_root/runtime_reconciliation.go")"
  if ! grep -q -E -- 'Observation[[:space:]]+runtimecontract\.RuntimeCWIReconciliationObservation' <<<"$reconciliation_result_shape" ||
     grep -q -E -- 'OwnerReceiptRef|ReceiptRef|Failure|Feedback|Disposition' <<<"$reconciliation_result_shape"; then
    echo "boundary contract violation: $owner CWI reconciliation Result must expose only the canonical observation" >&2
    exit 1
  fi
done

reconciliation_batch_shape="$(sed -n '/^type RuntimeCWIReconciliationBatch struct {/,/^}/p' internal/pkg/workorchruntimecontract/runtime_cwi_reconciliation.go)"
if grep -q -E -- 'OwnerReceiptRefs|ReceiptRefs' <<<"$reconciliation_batch_shape"; then
  echo "boundary contract violation: CWI reconciliation batch must derive lineage refs from canonical observations" >&2
  exit 1
fi
matches="$(source_matches 'func (file|git|mcp|shell)ReconciliationReceiptRef|func reconciliationOwnerReceiptRef' internal/modules/blocks/file internal/modules/blocks/git internal/modules/blocks/mcp internal/modules/blocks/shell internal/modules/kernel/orchestrator)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: CWI reconciliation restored a synthetic owner
receipt helper beside the canonical observation.

$matches

Derive stable lineage with StableRuntimeCWIReconciliationObservationRef.
Operation execution owner receipts are a separate durable identity.
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

# Runtime contracts are immutable DTOs and pure value helpers.  Their
# ledgers are persisted facts, but all mutation belongs to the Orchestrator
# reducer (SM-2/SM-3), never to the shared contract package.
runtime_contract_mutator_pattern='BeginRepairCycle|MarkValidationOnlyRetry|NoteProgress|NoteGapProgress|RecordNoProgressOperationShape|ResetForFocus|NoteNoProgress|ApplyAcceptedStateChangeEvidence|func \([^)]*RuntimeRecoveryLedger[^)]*\) Consume'
matches="$(source_matches "$runtime_contract_mutator_pattern" internal/pkg/workorchruntimecontract)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: Runtime contract pkg contains aggregate state
transition logic.

$matches

Keep Runtime pkg limited to immutable DTOs, contracts and pure helpers.
Move ledger mutation and recovery-budget consumption into the Orchestrator
runtime Biz, the sole SM-2/SM-3 authority owner.
EOF
  exit 1
fi

# workorchruntimecontract is classified by its package boundary, not by a
# production registry of every exported type. Scan every production source
# file directly so new DTO/helper files are covered automatically without a
# global type index that could become a runtime dispatch authority.
runtime_contract_impurity_pattern='github\.com/muidea/magicCommon/(event|task)|github\.com/muidea/workorch/internal/modules/|github\.com/muidea/workorch/internal/pkg/repository|"(os|os/exec|net/http|database/sql)"|NewSimpleObserver|\.Subscribe\(|\.Unsubscribe\(|\.Send\(|\.Post\(|\.Save\(|CompareAndSwap|time\.(NewTicker|NewTimer|After|AfterFunc|Sleep)|go[[:space:]]+func'
matches="$(source_matches "$runtime_contract_impurity_pattern" internal/pkg/workorchruntimecontract)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: workorchruntimecontract contains owner lifecycle,
EventHub, repository, external I/O, timer or asynchronous execution logic.

$matches

This package is an owner-neutral pure-value boundary: DTOs, enums and pure
normalize/validate/clone/hash/compiler/evaluator helpers only. Do not replace
this structural scan with a production global type inventory.
EOF
  exit 1
fi

runtime_transition_root="internal/modules/kernel/orchestrator/biz/runtime"
runtime_internal_fact_sources=(
  internal_event_activation.go
  internal_event_authority.go
  internal_event_transaction.go
  internal_event_delivery.go
)
for runtime_internal_fact_source in "${runtime_internal_fact_sources[@]}"; do
  if [[ ! -f "$runtime_transition_root/$runtime_internal_fact_source" ]]; then
    echo "boundary contract violation: Orchestrator private fact group $runtime_internal_fact_source is missing" >&2
    exit 1
  fi
done
if [[ ! -f "$runtime_transition_root/internal_event_contract.go" ]]; then
  echo "boundary contract violation: Orchestrator owner-private fact classification is missing" >&2
  exit 1
fi
matches="$(source_matches 'map\[RuntimeInternalEventKind\]|map\[string\].*RuntimeInternalEvent|RuntimeInternalEventRegistry|RuntimeInternalEventInventory' "$runtime_transition_root")"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: Orchestrator private facts use a production
registry/index instead of an owner-private exhaustive classification.

$matches

Keep the classification as an exhaustive owner-local switch. It must not be
exported, shared, or used as a dispatcher; ApplyInternalEvent remains the only
mutation entry.
EOF
  exit 1
fi

# RUNTIME-SMO: SM-2/SM-3/SM-4 share one Orchestrator-owned aggregate.  Local
# reducers may construct a complete next value, but only the transition
# coordinator may cross the opaque Run-owner CAS boundary.
runtime_transition_root="internal/modules/kernel/orchestrator/biz/runtime"
matches="$(
  find "$runtime_transition_root" -type f -name '*.go' ! -name '*_test.go' ! -name 'transition_coordinator.go' -print0 |
    xargs -0 -r grep -n -E -- '\.repository\.Save\(' || true
)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: Runtime aggregate CAS bypasses the sole
Orchestrator transition coordinator.

$matches

Local SM reducers and use-cases must submit a complete aggregate through
RuntimeTransitionCoordinator.Commit. Only transition_coordinator.go may call
the opaque RuntimeStateRepository Save method.
EOF
  exit 1
fi

# RUNTIME-SMO SMO-2: mainline and inbound adapters may translate external
# typed facts into owner-local events, but they must not choose a controller
# transition facade themselves.  The one Biz entry is
# submitRuntimeInternalEvent ->
# RuntimeController.ApplyInternalEvent -> RuntimeTransitionCoordinator.Commit.
if [[ ! -f "$runtime_transition_root/internal_event.go" ]] ||
   ! grep -q -E -- 'func \(c \*RuntimeController\) ApplyInternalEvent\(' "$runtime_transition_root/internal_event.go" ||
   ! grep -q -E -- 'func \(m \*Biz\) submitRuntimeInternalEvent\(' "internal/modules/kernel/orchestrator/biz/runtime_internal_events.go"; then
  echo "boundary contract violation: Runtime internal-event transition coordinator is missing" >&2
  exit 1
fi
matches="$(
  find internal/modules/kernel/orchestrator/biz -maxdepth 1 -type f -name '*.go' ! -name '*_test.go' -print0 |
    xargs -0 -r grep -n -E -- '\.runtimeController\.[A-Z][A-Za-z0-9_]*\(' |
    grep -v -E -- '\.runtimeController\.(ApplyInternalEvent|Load|QueryOperationReceipt)\(' || true
)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: Orchestrator mainline/adapter bypasses the
owner-local Runtime internal-event coordinator.

$matches

Translate the typed owner result into RuntimeInternalEvent and submit it via
Biz.submitRuntimeInternalEvent. Do not select a controller transition facade
from drive, mainline, receipt, governance or delivery adapters.
EOF
  exit 1
fi

# Biz directly owns the controller and evidence dependencies.  Restoring a
# RuntimeOrchestrator shell would create a second apparent owner/mutation
# surface and hide which object crosses the Run-owner CAS boundary.
runtime_orchestrator_shell="$(
  find internal/modules/kernel/orchestrator -type f -name '*.go' ! -name '*_test.go' -print0 |
    xargs -0 -r grep -n -E -- 'type RuntimeOrchestrator|NewRuntimeOrchestrator|\*RuntimeOrchestrator' || true
)"
if [[ -n "$runtime_orchestrator_shell" ]]; then
  cat >&2 <<EOF
Boundary contract violation: RuntimeOrchestrator thin shell was restored.

$runtime_orchestrator_shell

Wire RuntimeController and RuntimeEvidenceRepositories directly into Biz.
Mutation must enter through Biz.submitRuntimeInternalEvent; reads may use the
controller's scoped read surface.
EOF
  exit 1
fi

# R3-F4: ApplyInternalEvent plus scoped reads are the complete controller
# export surface. All reducer/CAS mutation handlers are package-private.
runtime_controller_unreviewed_exports="$(
  grep -R -n -E --include='*.go' --exclude='*_test.go' -- '^func \(c \*RuntimeController\) [A-Z][A-Za-z0-9_]*\(' "$runtime_transition_root" |
    grep -v -E -- ' (ApplyInternalEvent|Load|QueryOperationReceipt)\(' || true
)"
if [[ -n "$runtime_controller_unreviewed_exports" ]]; then
  cat >&2 <<EOF
Boundary contract violation: RuntimeController exposes a second mutation or
unreviewed read surface.

$runtime_controller_unreviewed_exports

Add owner mutation as a concrete RuntimeInternalEvent and package-private
handler. The only exported methods are ApplyInternalEvent, Load and
QueryOperationReceipt.
EOF
  exit 1
fi
if ! grep -q -E -- 'func \(c \*RuntimeController\) Load\(' "$runtime_transition_root/controller.go" ||
   ! grep -q -E -- 'func \(c \*RuntimeController\) QueryOperationReceipt\(' "$runtime_transition_root/operation_receipt.go"; then
  echo "boundary contract violation: RuntimeController scoped read surface is incomplete" >&2
  exit 1
fi

# RUNTIME-SMO: canonical convergence-state/effect semantics have exactly one
# pure contract source.
# Every production consumer must call that source instead of reconstructing a
# local action switch or accepted-effect table. Keep the paths explicit so a
# file move or consumer replacement requires an ownership review here.
runtime_effect_policy_consumers=(
  "internal/pkg/workorchruntimecontract/candidate_contract_manifest.go:RuntimeConvergenceEffectPolicyFor"
  "internal/modules/blocks/llm/biz/runtime_candidate_provider.go:RuntimeConvergenceEffectInstruction"
  "internal/pkg/workorchruntimecontract/runtime_effect_scope.go:RuntimeConvergenceEffectPolicyFor"
  "internal/pkg/workorchruntimecontract/runtime_effect_resolve.go:RuntimeConvergenceEffectPolicyFor"
  "internal/pkg/workorchruntimecontract/runtime_operation_receipt.go:RuntimeConvergenceEffectPolicyFor"
  "internal/modules/kernel/orchestrator/biz/runtime/executor.go:RuntimeConvergenceEffectPolicyFor"
)
for consumer in "${runtime_effect_policy_consumers[@]}"; do
  consumer_path="${consumer%%:*}"
  policy_symbol="${consumer#*:}"
  if [[ ! -f "$consumer_path" ]] || ! grep -q -E -- "${policy_symbol}[[:space:]]*\\(" "$consumer_path"; then
    cat >&2 <<EOF
Boundary contract violation: Runtime convergence-state/effect consumer does
not use the single RuntimeConvergenceEffectPolicy source: $consumer_path

Manifest compilation, provider rendering, candidate evaluation, seal-time
effect resolution, receipt evidence admission and executor adjudication must
all derive their rules from internal/pkg/workorchruntimecontract.
EOF
    exit 1
  fi
done

# RUNTIME-SMO C7-D: RuntimeOperation.TargetRefs is the single cross-owner
# target authority. File mutation/read/edit/patch and shell command contracts
# must not reintroduce a second path/targetRef parameter authority. file/list
# is intentionally outside this range because its path is a read-only query
# scope rather than a durable operation target.
target_contract_source="internal/pkg/workorchruntimecontract/runtime_operation_contract.go"
target_parameter_aliases="$(
  sed -n '/case StepKindFileRead:/,/case StepKindGitRead, StepKindGitWrite:/p' "$target_contract_source" |
    grep -n -E -- 'Name: "(path|targetRef|targetRefs)"' || true
)"
if [[ -n "$target_parameter_aliases" ]]; then
  cat >&2 <<EOF
Boundary contract violation: canonical Runtime target operations retain a
duplicate path/targetRef parameter authority.

$target_parameter_aliases

File read/write/edit, patch apply/preview and shell command targets must be
declared only through RuntimeOperation.TargetRefs. Owner-local command payloads
may derive their path from the sealed outer target list.
EOF
  exit 1
fi

# A generic SM-2 failure transition cannot replace current authority while an
# SM-3 dispatch remains active. Receipt/candidate/dispatch-specific transitions
# must close or advance that dispatch in the same aggregate CAS.
if ! grep -q -F -- 'if aggregate.ActiveDispatch != nil {' "$runtime_transition_root/controller.go" ||
   ! grep -q -F -- 'active dispatch failures require a dispatch/receipt-specific transition' "$runtime_transition_root/controller.go"; then
  cat >&2 <<EOF
Boundary contract violation: RecordFailureFact lacks the active-dispatch fence.

Generic failure recording is only valid when SM-3 has no active dispatch.
Keep active receipt/candidate failures on their atomic dispatch transition.
EOF
  exit 1
fi

# RUNTIME-SMO: generic owner updates are not lifecycle reducers. Keep the
# check structural (rather than relying on a field use-site grep) so a new
# mapper cannot silently reintroduce a second state-write path.
struct_body() {
  local file="$1" name="$2"
  awk -v name="$name" '
    $0 ~ "type " name " struct[[:space:]]*\\{" { inside=1; next }
    inside && /^}/ { exit }
    inside { print }
  ' "$file"
}

forbidden_workspace_update_fields='(^|[[:space:]])(State|ActiveRunID)([[:space:]]|$)'
workspace_update_body="$(struct_body internal/modules/blocks/workspace/pkg/events/messages.go UpdateWorkspaceCommand)"
if grep -q -E -- "$forbidden_workspace_update_fields" <<<"$workspace_update_body"; then
  echo "boundary contract violation: generic Workspace update exposes lifecycle fields" >&2
  exit 1
fi

forbidden_run_update_fields='(^|[[:space:]])(State|BlockingTicketID|BlockingApprovalID|FailureReason|StartedAt|EndedAt|Lineage|Priority)([[:space:]]|$)'
run_update_body="$(struct_body internal/modules/blocks/run/pkg/events/messages.go UpdateRunCommand)"
if grep -q -E -- "$forbidden_run_update_fields" <<<"$run_update_body"; then
  echo "boundary contract violation: generic Run update exposes SM-1 lifecycle fields" >&2
  exit 1
fi

forbidden_decision_update_fields='(^|[[:space:]])(State|ResolvedAt|ResolvedBy|CreatedAt)([[:space:]]|$)'
decision_update_body="$(struct_body internal/modules/blocks/decision/pkg/events/messages.go UpdateTicketCommand)"
if grep -q -E -- "$forbidden_decision_update_fields" <<<"$decision_update_body"; then
	 echo "boundary contract violation: generic Decision update exposes lifecycle fields" >&2
	 exit 1
fi

forbidden_approval_update_fields='(^|[[:space:]])(State|ResolvedAt|ResolvedBy|CreatedAt)([[:space:]]|$)'
approval_update_body="$(struct_body internal/modules/blocks/approval/pkg/events/messages.go UpdateApprovalCommand)"
if grep -q -E -- "$forbidden_approval_update_fields" <<<"$approval_update_body"; then
	 echo "boundary contract violation: generic Approval update exposes lifecycle fields" >&2
	 exit 1
fi

result_terminal_body="$(struct_body internal/modules/blocks/result/pkg/events/lifecycle_messages.go ApplyRuntimeTerminalDeliveryCommand)"
if grep -q -E -- '(^|[[:space:]])Result[[:space:]]+\*' <<<"$result_terminal_body"; then
  echo "boundary contract violation: Result terminal command carries a mutable Result model" >&2
  exit 1
fi

# Run, Result, and Workspace own their lifecycle wire contracts. Orchestrator
# may send those concrete contracts but must not redefine them, and the owner
# Biz packages must not depend on an Orchestrator-defined lifecycle command.
for owner in result workspace; do
  matches="$(source_matches 'internal/modules/kernel/orchestrator/pkg/events' "internal/modules/blocks/$owner/biz")"
  if [[ -n "$matches" ]]; then
    echo "boundary contract violation: $owner lifecycle owner imports Orchestrator event contracts" >&2
    echo "$matches" >&2
    exit 1
  fi
done
if ! grep -q -F -- 'TopicRunCommandApplyTerminalDelivery' internal/modules/blocks/run/pkg/events/topics.go ||
   ! grep -q -F -- 'TopicResultCommandApplyTerminalDelivery' internal/modules/blocks/result/pkg/events/topics.go ||
   ! grep -q -F -- 'TopicWorkspaceCommandApplyTerminalDelivery' internal/modules/blocks/workspace/pkg/events/topics.go ||
   ! grep -q -F -- 'runevents.TopicRunCommandApplyTerminalDelivery' internal/modules/kernel/orchestrator/biz/runtime_result_projection.go ||
   ! grep -q -F -- 'resultevents.TopicResultCommandApplyTerminalDelivery' internal/modules/kernel/orchestrator/biz/runtime_result_projection.go ||
   ! grep -q -F -- 'workspaceevents.TopicWorkspaceCommandApplyTerminalDelivery' internal/modules/kernel/orchestrator/biz/runtime_result_projection.go; then
  echo "boundary contract violation: Runtime terminal delivery is not expressed through Run/Result/Workspace owner contracts" >&2
  exit 1
fi

matches="$(source_matches 'TopicRunCommand(Cancel|Supersede)|CancelRunCommand|SupersedeRunCommand|reduce(Cancel|Supersede)Run' internal/modules)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: administrative Run termination bypasses the
Orchestrator-persisted RuntimeTerminationRecord and SM-4 delivery saga.

$matches
EOF
  exit 1
fi

# The LLM Block is a transport/schema owner. It may decode a typed candidate,
# but it must not decide candidate acceptance, retry or terminal disposition.
llm_candidate_authority_pattern='ValidateWithActivation\(|CandidateDisposition|CandidateAttemptOutcome|RetrySameContract|CandidateFailureOutcome'
matches="$(source_matches "$llm_candidate_authority_pattern" internal/modules/blocks/llm/biz)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: LLM Block contains Runtime candidate authority.

$matches

LLM owns provider transport and strict wire decoding only. Candidate semantic
validation, retry eligibility, governance and terminal selection belong to the
Orchestrator reducer.
EOF
  exit 1
fi

# WorkOrchd is an application composition module. Production appservice and
# HTTP code must use typed owner ports, not reach into another owner's store.
matches="$(
  find internal/modules/application/workorchd -type f -name '*.go' ! -name '*_test.go' ! -path '*/biz/testkit/*' -print0 |
    xargs -0 -r grep -n -E -- 'internal/pkg/repository' || true
)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: WorkOrchd production code directly imports repository infrastructure.

$matches

Application composition must read or write owner state through typed EventHub
commands/results or narrow owner read ports. Repository fixtures remain test-only.
EOF
  exit 1
fi

# Governance persists and audits decision/approval evidence. It must never
# inject a generic Run continuation; the Orchestrator consumes the committed
# typed resolution and schedules the Runtime drive itself.
governance_resume_pattern='TopicOrchestratorCommandResumeRun|ResumeRunCommand|ResumeRunFunc|eventResumeRun'
matches="$(source_matches "$governance_resume_pattern" internal/modules/kernel/governance)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: Governance contains a generic Runtime resume path.

$matches

Governance may resolve a ticket or approve an operation, but must not resume a
Run. Runtime continuation is owned exclusively by the Orchestrator after it
consumes committed typed governance evidence.
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
# The assessment gate is a structural verifier.  Lexical comparison between a
# Goal/DoD claim and summaries, operation IDs, targets, test names or output
# makes coverage language-dependent and creates a second semantic evaluator.
assessment_lexical_alignment_pattern='runtimeEvidenceAlignsWithClaim|runtimeCoverageClaimText|runtimeClaimToken|runtimeNumericToken|unicode\.(IsLetter|IsDigit|IsNumber)|strings\.Fields'

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

# RUNTIME-SMO C6: projections are observers.  A presentation surface must not
# retain a second Runtime action authority, parse text into a Runtime choice,
# or call a Runtime drive/continuation path.  Runtime exposes the canonical
# convergence state as display-only data; legacy suggestion fields are marked
# separately and cannot be used as a drive input.
projection_runtime_alias_pattern='CurrentAction[[:space:]]+string|NextActionKind[[:space:]]+string|RuntimeAuditAuthority[[:space:]]*struct[[:space:]]*\{[^}]*Action'
matches="$(source_matches "$projection_runtime_alias_pattern" \
  internal/modules/application/workorchd/biz/appservice/readmodelcore \
  internal/modules/application/workorchd/biz/appservice/readmodelprojection \
  internal/pkg/workorchport internal/pkg/workorchclient \
  internal/services/workorch web/workorch-console/src)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: projection surface retains a duplicate Runtime action authority.

$matches

Project Runtime only as canonical convergenceState. A historical display
hint, if still required, must be named suggestedNextAction and must never be
used to select, resume, ticket, or drive Runtime work.
EOF
  exit 1
fi

projection_message_routing_pattern='Failure(Reason|Message).*strings\.(HasPrefix|Contains|EqualFold)|strings\.(HasPrefix|Contains|EqualFold).*Failure(Reason|Message)|Original(Message|Code).*strings\.(HasPrefix|Contains|EqualFold)|strings\.(HasPrefix|Contains|EqualFold).*Original(Message|Code)'
matches="$(source_matches "$projection_message_routing_pattern" \
  internal/modules/application/workorchd/biz/appservice/readmodelprojection \
  internal/modules/blocks/eventstream internal/services/workorch/service/tui \
  web/workorch-console/src)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: projection routes Runtime behavior from diagnostic text.

$matches

Use a structured stable category, terminal code, or canonical convergence
state for display. Original message and code are audit data only.
EOF
  exit 1
fi

projection_reverse_drive_pattern='(DriveRuntime|RequestRuntimeCandidate|OpenRuntimeGovernanceRequest|CommitRuntimeGovernanceResolution|ResumeRun)[[:space:]]*\('
matches="$(source_matches "$projection_reverse_drive_pattern" \
  internal/modules/application/workorchd/biz/appservice/readmodelprojection \
  internal/modules/blocks/eventstream)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: projection or eventstream code reverse-drives Runtime.

$matches

Projection and eventstream are audit/display consumers only. Runtime drive and
governance continuation remain Orchestrator owner-local work.
EOF
  exit 1
fi

# Managed and live provider fixtures are contract consumers too. They must
# select their deterministic response from the canonical C3 requiredState,
# never resurrect the retired RuntimeDecision Action field. Otherwise a green
# fixture can conceal a production prompt/contract drift until a later state
# (typically assessment) receives the wrong JSON shape.
runtime_candidate_fixture_action_pattern='"Action"|\["Action"\]|sealed Action|action_match'
matches="$(grep -R -n -E --include='*.sh' --include='*.py' --exclude='verify-boundary-contract.sh' -- "$runtime_candidate_fixture_action_pattern" scripts || true)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: Runtime provider fixture depends on retired
RuntimeDecision Action instead of canonical requiredState.

$matches

Provider fixtures must derive their canned candidate shape from the sealed C3
requiredState. RuntimeAction is not a Runtime prompt, persistence, or
fixture authority.
EOF
  exit 1
fi

# The live C7 verifiers are also contract consumers.  They may observe the
# canonical convergence state, but cannot recover a retired action-shaped
# authority from a read model.  Keep the scope explicit: unrelated archived
# V1 material is not a Runtime verifier and must not mask this boundary.
runtime_live_verifier_retired_action_pattern='nextActionKind|currentAction'
runtime_live_verifier_paths=(
  scripts/observe-workorch-runtime-closure.sh
  scripts/verify-runtime-positive-convergence-live.sh
  scripts/verify-runtime-restart-recovery-live.sh
  scripts/verify-runtime-positive-convergence-file.sh
  scripts/verify_runtime_live_contract.py
  scripts/verify_runtime_restart_contract.py
)
matches="$(grep -n -E -- "$runtime_live_verifier_retired_action_pattern" "${runtime_live_verifier_paths[@]}" || true)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: a Runtime live verifier reads a retired action
field instead of canonical convergenceState.

$matches

Use sealed/persisted convergenceState and typed audit facts for observation.
Live verifiers must not recreate a drive instruction from nextActionKind or
currentAction.
EOF
  exit 1
fi

# Every one of the declared 22 runtime units has the same pkg purity rule.
# In addition to infrastructure imports checked above, a pkg value/helper may
# not implement an owner transition, retry, lifecycle continuation, scheduler
# or drive operation. Those belong to the component Biz reducer.
component_pkg_transition_pattern='func \([^)]*\) (Transition|Advance|Retry|Resume|Fail|Complete|Schedule|Drive|Apply.*State|SetState)\('
matches="$(
  find "${initiator_class_roots[@]}" "${module_class_roots[@]}" "${block_class_roots[@]}" \
    -type f -path '*/pkg/*.go' ! -name '*_test.go' -print0 |
    xargs -0 -r grep -n -E -- "$component_pkg_transition_pattern" || true
)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: a component pkg contains state-transition logic.

$matches

All 22 component pkg directories are DTO/contract/pure-helper boundaries.
Move transition, retry, lifecycle, scheduling, and drive logic into the
single-owner component Biz.
EOF
  exit 1
fi

# Safe owner feedback must reach the audit projection structurally.  The
# projection may render only the bounded RuntimeFeedback metadata, never raw
# provider/tool payloads; removing this mapper would silently collapse the
# owner -> aggregate -> audit chain to a summary string.
if ! grep -q -E -- 'func runtimeAuditOperationReceipts\(' \
  internal/modules/application/workorchd/biz/appservice/readmodelprojection/runtime_audit.go ||
   ! grep -q -E -- 'OriginalDiagnostic' \
  internal/modules/application/workorchd/biz/appservice/readmodelprojection/runtime_audit.go ||
   ! grep -q -E -- 'OperationReceipts' \
  internal/modules/application/workorchd/biz/appservice/readmodelcore/runtime_audit_types.go; then
  cat >&2 <<EOF
Boundary contract violation: Runtime audit projection drops structured owner feedback.

Map RuntimeOperationReceipt.Feedback as bounded stable category and original
diagnostic metadata. Do not replace it with a message-only summary or raw
provider/tool payload.
EOF
  exit 1
fi
audit_receipt_shape="$(sed -n '/^type RuntimeAuditOperationReceipt struct {/,/^}/p' internal/modules/application/workorchd/biz/appservice/readmodelcore/runtime_audit_types.go)"
if ! grep -q -E -- 'OriginalDiagnostic[[:space:]]+\*runtimecontract\.RuntimeOriginalDiagnostic' <<<"$audit_receipt_shape" ||
   grep -q -E -- '(OwnerModuleID|OriginalClass|OriginalCode|OriginalMessage|Signature|ExpectedRevision|ObservedRevision|DetailsRef|DetailsHash|ContentType|Truncated|Redacted)[[:space:]]+' <<<"$audit_receipt_shape"; then
  echo "boundary contract violation: operation receipt audit must carry one nested canonical original diagnostic without flattened duplicates" >&2
  exit 1
fi
port_receipt_shape="$(sed -n '/^type RuntimeAuditOperationReceiptInfo struct {/,/^}/p' internal/pkg/workorchport/workspace.go)"
client_receipt_shape="$(sed -n '/^type RuntimeAuditOperationReceipt struct {/,/^}/p' internal/pkg/workorchclient/types.go)"
for receipt_shape in "$port_receipt_shape" "$client_receipt_shape"; do
  if ! grep -q -E -- 'OriginalDiagnostic[[:space:]]+\*runtimecontract\.RuntimeOriginalDiagnostic' <<<"$receipt_shape" ||
     grep -q -E -- '(OwnerModuleID|OriginalClass|OriginalCode|OriginalMessage|Signature|ExpectedRevision|ObservedRevision|DetailsRef|DetailsHash|ContentType|Truncated|Redacted)[[:space:]]+' <<<"$receipt_shape"; then
    echo "boundary contract violation: operation receipt port/client projection drifted from nested canonical original diagnostic" >&2
    exit 1
  fi
done

# The LLM owner transports provider request facts only. Candidate rejection is
# Orchestrator SM-3 authority and must never reappear on the EventHub Result.
llm_candidate_result_shape="$(sed -n '/^type RequestRuntimeCandidateResult struct {/,/^}/p' internal/modules/blocks/llm/pkg/events/messages.go)"
if ! grep -q -E -- 'Feedback[[:space:]]+\*runtimecontract\.RuntimeFeedback' <<<"$llm_candidate_result_shape" ||
   grep -q -E -- '(^|[[:space:]])(Failure[[:space:]]+\*runtimecontract\.RuntimeCandidateFailure|RequestID[[:space:]]+string|Status[[:space:]]+string)' <<<"$llm_candidate_result_shape"; then
  echo "boundary contract violation: LLM provider Result must carry RuntimeFeedback without candidate failure or duplicate audit authority" >&2
  exit 1
fi
llm_candidate_command_shape="$(sed -n '/^type RequestRuntimeCandidateCommand struct {/,/^}/p' internal/modules/blocks/llm/pkg/events/messages.go)"
llm_candidate_model_request_shape="$(sed -n '/^type RuntimeCandidateRequest struct {/,/^}/p' internal/modules/blocks/llm/pkg/models/runtime_candidate.go)"
llm_candidate_model_response_shape="$(sed -n '/^type RuntimeCandidateResponse struct {/,/^}/p' internal/modules/blocks/llm/pkg/models/runtime_candidate.go)"
for request_identity_shape in "$llm_candidate_command_shape" "$llm_candidate_result_shape" "$llm_candidate_model_request_shape" "$llm_candidate_model_response_shape"; do
  if ! grep -q -E -- 'RequestIdentity[[:space:]]+runtimecontract\.RuntimeCandidateRequestIdentity' <<<"$request_identity_shape" ||
     grep -q -E -- '(LogicalRequestID[[:space:]]+string|RequestSequence[[:space:]]+int|PhysicalCallID[[:space:]]+string)' <<<"$request_identity_shape"; then
    echo "boundary contract violation: Runtime provider request boundaries must carry one canonical RuntimeCandidateRequestIdentity without flattened identity fields" >&2
    exit 1
  fi
done
if grep -q -E -- '(RequestID|Status)[[:space:]]+string' <<<"$llm_candidate_model_response_shape"; then
  echo "boundary contract violation: LLM RuntimeCandidateResponse must use sealed audit/error provider identity and status without duplicate RequestID/Status fields" >&2
  exit 1
fi
for terminal_owner in run workspace result; do
  terminal_delivery_shape="$(sed -n '/^type ApplyRuntimeTerminalDeliveryCommand struct {/,/^}/p' "internal/modules/blocks/${terminal_owner}/pkg/events/lifecycle_messages.go")"
  if ! grep -q -E -- 'Outcome[[:space:]]+runtimecontract\.TerminalOutcome' <<<"$terminal_delivery_shape" ||
     grep -q -E -- 'FailureReason[[:space:]]+string' <<<"$terminal_delivery_shape"; then
    echo "boundary contract violation: ${terminal_owner} terminal delivery must carry one sealed TerminalOutcome without a parallel failure reason" >&2
    exit 1
  fi
done
for execution_intent_owner in run workspace; do
  execution_intent_result_shape="$(sed -n '/^type WorkspaceExecutionIntentResult struct {/,/^}/p' "internal/modules/blocks/${execution_intent_owner}/pkg/events/lifecycle_messages.go")"
  if ! grep -q -E -- 'Outcome[[:space:]]+runtimecontract\.RuntimeExecutionIntentOutcome' <<<"$execution_intent_result_shape" ||
     ! grep -q -E -- 'Reason[[:space:]]+runtimecontract\.RuntimeExecutionIntentReason' <<<"$execution_intent_result_shape" ||
     grep -q -E -- '(Disposition|Diagnostic)[[:space:]]+string' <<<"$execution_intent_result_shape"; then
    echo "boundary contract violation: ${execution_intent_owner} execution-intent Result must carry one typed admission outcome and factual reason without free-form disposition/diagnostic fields" >&2
    exit 1
  fi
done
for lifecycle_owner in run workspace result; do
  case "$lifecycle_owner" in
    run) lifecycle_result_name='RunLifecycleCommandResult' ;;
    workspace) lifecycle_result_name='WorkspaceLifecycleCommandResult' ;;
    result) lifecycle_result_name='ResultLifecycleCommandResult' ;;
  esac
  lifecycle_result_shape="$(sed -n "/^type ${lifecycle_result_name} struct {/,/^}/p" "internal/modules/blocks/${lifecycle_owner}/pkg/events/lifecycle_messages.go")"
  if grep -q -E -- 'Diagnostic[[:space:]]+string' <<<"$lifecycle_result_shape"; then
    echo "boundary contract violation: ${lifecycle_owner} lifecycle Result must expose owner state/ack facts without a parallel free-form diagnostic" >&2
    exit 1
  fi
done
drive_result_shape="$(sed -n '/^type DriveRuntimeResult struct {/,/^}/p' internal/modules/kernel/orchestrator/pkg/events/messages.go)"
if ! grep -q -E -- 'State[[:space:]]+runtimecontract\.ConvergenceState' <<<"$drive_result_shape" ||
   ! grep -q -E -- 'BlockingTicketID[[:space:]]+common\.TicketID' <<<"$drive_result_shape" ||
   ! grep -q -E -- 'Terminal[[:space:]]+\*runtimecontract\.RuntimeTerminalDecision' <<<"$drive_result_shape" ||
   grep -q -E -- 'Disposition[[:space:]]+' <<<"$drive_result_shape"; then
  echo "boundary contract violation: Runtime drive Result must derive governed wait or terminal delivery from sealed state facts without a parallel disposition" >&2
  exit 1
fi
governance_result_shape="$(sed -n '/^type CommitRuntimeGovernanceResolutionResult struct {/,/^}/p' internal/modules/kernel/orchestrator/pkg/events/messages.go)"
if ! grep -q -E -- 'WorkspaceID[[:space:]]+common\.WorkspaceID' <<<"$governance_result_shape" ||
   ! grep -q -E -- 'RunID[[:space:]]+common\.RunID' <<<"$governance_result_shape" ||
   ! grep -q -E -- 'ResolutionID[[:space:]]+common\.ResolutionID' <<<"$governance_result_shape" ||
   ! grep -q -E -- 'Disposition[[:space:]]+RuntimeGovernanceResolutionDisposition' <<<"$governance_result_shape" ||
   grep -q -E -- '(Diagnostic|Reason|Failure)[[:space:]]+' <<<"$governance_result_shape"; then
  echo "boundary contract violation: Runtime governance commit Result must preserve scoped CAS disposition without parallel diagnostic authority" >&2
  exit 1
fi
llm_preflight_result_shape="$(sed -n '/^type PreflightRuntimeCandidateResult struct {/,/^}/p' internal/modules/blocks/llm/pkg/events/messages.go)"
if ! grep -q -E -- 'Feedback[[:space:]]+runtimecontract\.RuntimeFeedback' <<<"$llm_preflight_result_shape" ||
   grep -q -E -- '(Ready[[:space:]]+bool|Reason[[:space:]]+string)' <<<"$llm_preflight_result_shape"; then
  echo "boundary contract violation: LLM provider preflight Result must carry one RuntimeFeedback outcome without Ready/Reason authority" >&2
  exit 1
fi
for provider_feedback_symbol in \
  RuntimeFeedbackDomainProviderPreflight \
  RuntimeFeedbackDomainProviderRequest \
  RuntimeFeedbackCategoryContextIntegrity \
  RuntimeFeedbackCategoryProviderAuthentication \
  RuntimeFeedbackCategoryProviderConfiguration \
  RuntimeFeedbackCategoryProviderTransient \
  RuntimeFeedbackCategoryProviderProtocol; do
  if ! grep -q -E -- "^[[:space:]]*${provider_feedback_symbol}[[:space:]]+RuntimeFeedback" internal/pkg/workorchruntimecontract/runtime_feedback.go; then
    echo "boundary contract violation: missing exhaustive provider feedback symbol ${provider_feedback_symbol}" >&2
    exit 1
  fi
done

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

matches="$(source_matches "$assessment_lexical_alignment_pattern" internal/modules/kernel/orchestrator/biz/runtime/assessment_evidence_gate.go)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: assessment evidence gate uses lexical claim alignment.

$matches

Assessment coverage is decided from accepted current-run evidence references,
coverage bindings, sealed operation/effect contracts and owner observations.
Goal/DoD or check-name text similarity belongs only to the provider assessment,
not to the Orchestrator reducer.
EOF
  exit 1
fi

# Decision and Approval own their lifecycle reducers. Orchestrator must use
# their typed intent/fact topics directly and must not recreate repository
# facades that accept caller-selected owner state.
matches="$(source_matches 'repository\.(DecisionTicketRepository|DecisionResolutionRepository|ApprovalRepository)|decisionEventTicketRepository|decisionEventResolutionRepository|approvalEventRepository' internal/modules/kernel/orchestrator)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: Orchestrator retains a Decision/Approval repository facade.

$matches

Decision and Approval state transitions belong to their owner Biz. Runtime
may publish typed intent and consume immutable facts, but it cannot inject or
wrap their repositories.
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

# C4 is a provider-facing diagnostic projection, not a Runtime aggregate
# mirror. Keep its struct limited to the LLM owner's narrow provider-safe
# values so owner actions, recovery continuation, raw facts and repair hints
# cannot reach a future candidate request by accident.
c4_projection_file="internal/modules/blocks/llm/biz/runtime_candidate_provider.go"
matches="$(
  sed -n '/^type runtimeCandidateExecutionTailProjection struct {/,/^}/p' "$c4_projection_file" |
    grep -n -E -- 'RuntimeAction|RuntimeRepairHint|RuntimeContinuationContext|FailureFact|EvidenceFact|RuntimeAssessmentFact|RuntimeRejectedOperationShape' || true
)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: provider-facing C4 execution tail leaks a raw
Runtime fact, action/recovery value, or owner retry guidance.

$matches

C4 may contain only its dedicated provider-safe projections, the sealed
CandidateRejection correction projection, and an output-continuation suffix.
Keep Runtime actions, repair hints, continuation/governance history, raw
failure messages and complete evidence/assessment facts with their owners.
EOF
  exit 1
fi

echo "boundary contract verification passed"
