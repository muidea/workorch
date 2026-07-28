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
# The state owners and Runtime V2 drive must use the checked binder rather
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
  "internal/modules/kernel/orchestrator/biz/runtime_v2_drive.go"
  "internal/modules/kernel/orchestrator/biz/runtime_v2_operation_executor.go"
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

# Runtime V2 contracts are immutable DTOs and pure value helpers.  Their
# ledgers are persisted facts, but all mutation belongs to the Orchestrator
# reducer (SM-2/SM-3), never to the shared contract package.
runtime_contract_mutator_pattern='BeginRepairCycle|MarkValidationOnlyRetry|NoteProgress|NoteGapProgress|RecordNoProgressOperationShape|ResetForFocus|NoteNoProgress|ApplyAcceptedStateChangeEvidence|func \([^)]*RuntimeRecoveryLedger[^)]*\) Consume'
matches="$(source_matches "$runtime_contract_mutator_pattern" internal/pkg/workorchruntimecontract)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: Runtime V2 contract pkg contains aggregate state
transition logic.

$matches

Keep Runtime V2 pkg limited to immutable DTOs, contracts and pure helpers.
Move ledger mutation and recovery-budget consumption into the Orchestrator
runtimev2 Biz, the sole SM-2/SM-3 authority owner.
EOF
  exit 1
fi

# RUNTIME-SMO: SM-2/SM-3/SM-4 share one Orchestrator-owned aggregate.  Local
# reducers may construct a complete next value, but only the transition
# coordinator may cross the opaque Run-owner CAS boundary.
runtime_transition_root="internal/modules/kernel/orchestrator/biz/runtimev2"
matches="$(
  find "$runtime_transition_root" -type f -name '*.go' ! -name '*_test.go' ! -name 'transition_coordinator.go' -print0 |
    xargs -0 -r grep -n -E -- '\.repository\.Save\(' || true
)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: Runtime V2 aggregate CAS bypasses the sole
Orchestrator transition coordinator.

$matches

Local SM reducers and use-cases must submit a complete aggregate through
RuntimeTransitionCoordinator.Commit. Only transition_coordinator.go may call
the opaque RuntimeStateRepository Save method.
EOF
  exit 1
fi

# RUNTIME-SMO: action/effect semantics have exactly one pure contract source.
# Every production consumer must call that source instead of reconstructing a
# local action switch or accepted-effect table. Keep the paths explicit so a
# file move or consumer replacement requires an ownership review here.
runtime_effect_policy_consumers=(
  "internal/pkg/workorchruntimecontract/candidate_contract_manifest.go:RuntimeActionEffectPolicyFor"
  "internal/modules/blocks/llm/biz/runtime_candidate_provider.go:RuntimeActionEffectInstruction"
  "internal/pkg/workorchruntimecontract/runtime_effect_scope.go:RuntimeActionEffectPolicyFor"
  "internal/pkg/workorchruntimecontract/runtime_effect_resolve.go:RuntimeActionEffectPolicyFor"
  "internal/pkg/workorchruntimecontract/runtime_operation_receipt.go:RuntimeActionEffectPolicyFor"
  "internal/modules/kernel/orchestrator/biz/runtimev2/executor.go:RuntimeActionEffectPolicyFor"
)
for consumer in "${runtime_effect_policy_consumers[@]}"; do
  consumer_path="${consumer%%:*}"
  policy_symbol="${consumer#*:}"
  if [[ ! -f "$consumer_path" ]] || ! grep -q -E -- "${policy_symbol}[[:space:]]*\\(" "$consumer_path"; then
    cat >&2 <<EOF
Boundary contract violation: Runtime V2 action/effect consumer does not use the
single RuntimeActionEffectPolicy source: $consumer_path

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
workspace_update_body="$(struct_body internal/modules/blocks/workspace/pkg/contracts/commands.go UpdateWorkspaceCommand)"
if grep -q -E -- "$forbidden_workspace_update_fields" <<<"$workspace_update_body"; then
  echo "boundary contract violation: generic Workspace update exposes lifecycle fields" >&2
  exit 1
fi

forbidden_run_update_fields='(^|[[:space:]])(State|BlockingTicketID|BlockingApprovalID|FailureReason|StartedAt|EndedAt|Lineage|Priority)([[:space:]]|$)'
run_update_body="$(struct_body internal/modules/blocks/run/pkg/contracts/commands.go UpdateRunCommand)"
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
Boundary contract violation: LLM Block contains Runtime V2 candidate authority.

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
# typed resolution and schedules the Runtime V2 drive itself.
governance_resume_pattern='TopicOrchestratorCommandResumeRun|ResumeRunCommand|ResumeRunFunc|eventResumeRun'
matches="$(source_matches "$governance_resume_pattern" internal/modules/kernel/governance)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: Governance contains a generic Runtime resume path.

$matches

Governance may resolve a ticket or approve an operation, but must not resume a
Run. Runtime V2 continuation is owned exclusively by the Orchestrator after it
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

# Decision and Approval own their lifecycle reducers. Orchestrator must use
# their typed intent/fact topics directly and must not recreate repository
# facades that accept caller-selected owner state.
matches="$(source_matches 'repository\.(DecisionTicketRepository|DecisionResolutionRepository|ApprovalRepository)|decisionEventTicketRepository|decisionEventResolutionRepository|approvalEventRepository' internal/modules/kernel/orchestrator)"
if [[ -n "$matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: Orchestrator retains a Decision/Approval repository facade.

$matches

Decision and Approval state transitions belong to their owner Biz. Runtime V2
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

echo "boundary contract verification passed"
