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
  "internal/pkg/workorchruntimecontract"
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

# Workspace is the sole deletion/retirement aggregate owner. Application has
# one Workspace command ingress and must not regain a cross-owner purge list,
# directory cleanup, or Run enumeration saga.
application_workspace_delete_orchestration="$({
  source_matches 'purgeDeletedWorkspaceView|PurgeWorkspaceRecords|PurgeWorkspaceEventStream|Topic(Orchestrator|Run|Memory|Result|Decision|Approval|EventStream)Command(PurgeWorkspace|PurgeRun)' \
    internal/modules/application/workorchd
  source_matches 'PurgeDirectories|os\.RemoveAll' \
    internal/modules/application/workorchd/biz/appservice/workspaceports \
    internal/modules/application/workorchd/biz/appservice/workspacecore
} | sed '/^[[:space:]]*$/d')"
if [[ -n "$application_workspace_delete_orchestration" ]]; then
  echo "boundary contract violation: Application regained cross-owner Workspace deletion orchestration" >&2
  echo "$application_workspace_delete_orchestration" >&2
  exit 1
fi

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

# Canonical DTO and lifecycle types must be consumed from their focused owner
# package. Type aliases recreate facade/compatibility surfaces, hide the actual
# contract owner, and let duplicate names return without a compile-time review.
type_aliases="$(
  find internal -type f -name '*.go' -print0 |
    xargs -0 -r grep -n -E -- '^type [A-Za-z_][A-Za-z0-9_]*[[:space:]]*=' || true
)"
if [[ -n "$type_aliases" ]]; then
  cat >&2 <<EOF
Boundary contract violation: internal code must not re-export canonical types
through facade or compatibility aliases. Import the focused owner contract
directly, or define a genuinely distinct protocol type with explicit mapping.

$type_aliases
EOF
  exit 1
fi

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

# Workorchd production transport is intentionally narrower than the Biz
# lifecycle owner. Test infrastructure may construct owner runtimes, but
# non-test server code receives typed ApplicationPorts only.
workorchd_server_transport_matches="$({
  source_matches 'runtimecore|magicCommon/event|magicevent\.Hub|NewSimpleObserver|\.(Subscribe|Unsubscribe|Send|Post)\(|"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"' \
    internal/modules/application/workorchd/service/server
} | sed '/^[[:space:]]*$/d')"
if [[ -n "$workorchd_server_transport_matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: Workorchd production server must not retain
runtimecore, EventHub, root appservice, observer, or direct event operations.
Inject typed ApplicationPorts from Biz.

$workorchd_server_transport_matches
EOF
  exit 1
fi

# The execution-admission use case owns focused application ports and must stay
# independent of its HTTP adapter. The Biz package root is the composition
# boundary and may wire both sides; biz/execution itself may not import server.
workorchd_execution_server_import_matches="$({
  find internal/modules/application/workorchd/biz/execution -type f -name '*.go' ! -name '*_test.go' -print0 |
    xargs -0 -r grep -Hn -F -- 'internal/modules/application/workorchd/service/server' || true
} | sed '/^[[:space:]]*$/d')"
if [[ -n "$workorchd_execution_server_import_matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: Workorchd biz/execution imports its downstream
HTTP service/server adapter. The execution use case must own focused typed
ports and remain independent of server handlers, metrics, and local state.

$workorchd_execution_server_import_matches
EOF
  exit 1
fi

workorchd_server_owner_implementation_matches="$({
  source_matches '"github.com/muidea/workorch/internal/modules/(blocks|kernel)/[^\"]+/(biz|service)(/|\")' \
    internal/modules/application/workorchd/service/server
} | sed '/^[[:space:]]*$/d')"
if [[ -n "$workorchd_server_owner_implementation_matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: Workorchd transport imports another owner Biz,
service, or capability executor implementation. Transport may consume only
application DTOs/ports and read-only owner contract values.

$workorchd_server_owner_implementation_matches
EOF
  exit 1
fi

# Workorchd may render owner-owned Result/Run/Orchestrator content, but it must
# obtain those values through the receiving owner's typed EventHub read
# contract. The protocol/content adapter must never reconstruct a repository
# path or read another owner's file/repository directly.
workorchd_content_boundary_files=(
  "internal/modules/application/workorchd/biz/appservice/content"
  "internal/modules/application/workorchd/biz/appservice/ownerreadports"
  "internal/modules/application/workorchd/biz/appservice/artifact_content_service.go"
  "internal/modules/application/workorchd/biz/appservice/runtime_raw_content_service.go"
)
workorchd_private_content_read_matches="$({
  find "${workorchd_content_boundary_files[@]}" -type f -name '*.go' ! -name '*_test.go' -print0 |
    xargs -0 -r grep -Hn -E -- '"(os|path/filepath)"|os\.(ReadFile|Open)|filepath\.|internal/pkg/repository|workorchrepo|internal/modules/(blocks|kernel)/[^\"]+/(biz|service)(/|\")' || true
} | sed '/^[[:space:]]*$/d')"
if [[ -n "$workorchd_private_content_read_matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: Workorchd content access bypasses a Result, Run,
or Orchestrator typed owner read and reaches private filesystem/repository or
owner implementation state directly.

$workorchd_private_content_read_matches
EOF
  exit 1
fi
workorchd_content_reader_contract="$(cat \
  internal/modules/application/workorchd/biz/appservice/content/artifact_reader.go \
  internal/modules/application/workorchd/biz/appservice/content/runtime_raw_reader.go \
  internal/modules/application/workorchd/biz/appservice/content/runtime_virtual_reader.go)"
for marker in \
  'ReadArtifactContentByResultOwnerCommand' \
  'ReadStepByOwnerCommand' \
  'ListToolCallsByRunOwnerCommand' \
  'ReadRuntimeCheckpointByOwnerCommand' \
  'ReadResultByOwnerCommand'; do
  if ! grep -q -F -- "$marker" <<<"$workorchd_content_reader_contract"; then
    echo "boundary contract violation: Workorchd content reader lost typed owner read $marker" >&2
    exit 1
  fi
done
if ! grep -q -F -- 'resultevents.TopicArtifactCommandReadContent' internal/modules/application/workorchd/biz/appservice/ownerreadports/artifact_content_read.go ||
   ! grep -q -F -- 'resultevents.TopicArtifactCommandReadContent,' internal/modules/blocks/result/biz/owner.go ||
   ! grep -q -F -- 'runevents.TopicRunCommandGetStep' internal/modules/application/workorchd/biz/appservice/ownerreadports/run_step_reads.go ||
   ! grep -q -F -- 'runevents.TopicRunCommandGetRuntimeCheckpoint' internal/modules/application/workorchd/biz/appservice/ownerreadports/run_checkpoint_reads.go ||
   ! grep -q -F -- 'runevents.TopicRunCommandGetStep,' internal/modules/blocks/run/biz/command_router.go ||
   ! grep -q -F -- 'runevents.TopicRunCommandGetRuntimeCheckpoint,' internal/modules/blocks/run/biz/command_router.go ||
   ! grep -q -F -- 'orchestratorevents.TopicOrchestratorCommandListToolCallsByRun' internal/modules/application/workorchd/biz/appservice/ownerreadports/orchestrator_reads.go ||
   ! grep -q -F -- 'orchestratorevents.TopicOrchestratorCommandListToolCallsByRun,' internal/modules/kernel/orchestrator/biz/owner.go; then
  echo "boundary contract violation: Workorchd content reads are not wired to their receiving owner topics" >&2
  exit 1
fi
if grep -R -q -E --include='*.go' --exclude='*_test.go' -- \
  'func (runtimeRawContentPath|artifactContentPath|artifactPath|runtimeRawPath)\(' \
  internal/modules/application/workorchd; then
  echo "boundary contract violation: Workorchd restored a private owner content-path builder" >&2
  exit 1
fi
for regression in \
  'TestReadArtifactUsesResultOwnerContentCommandWithURIWorkspaceScope' \
  'TestReadRuntimeRawContentUsesOwnerTypedReadsAcrossBackends' \
  'TestReadRuntimeRawContentReturnsTypedCheckpointAndResultDetailsAcrossBackends' \
  'TestReadArtifactContentByResultOwnerCommandValidatesScopeHashAndCopy'; do
  if ! grep -R -q -F --include='*_test.go' -- "$regression" internal/modules/application/workorchd; then
    echo "boundary contract violation: Workorchd typed content-read regression is missing ($regression)" >&2
    exit 1
  fi
done

workorchd_run_ports_shape="$(sed -n '/^type RunPorts struct {/,/^}/p' internal/modules/application/workorchd/service/server/application_ports.go)"
workorchd_run_mutation_ports="$(grep -E '^[[:space:]]+[A-Z][A-Za-z0-9_]*[[:space:]]' <<<"$workorchd_run_ports_shape" | grep -v -E '^[[:space:]]+(Read|List)[A-Za-z0-9_]*[[:space:]]' || true)"
if [[ -n "$workorchd_run_mutation_ports" ]]; then
  cat >&2 <<EOF
Boundary contract violation: ApplicationPorts.Run must remain read-only; Run
lifecycle mutation enters through Workspace execution admission only.

$workorchd_run_mutation_ports
EOF
  exit 1
fi

workorchd_run_authority_matches="$({
  find internal/modules/application/workorchd -type f -name '*.go' ! -name '*_test.go' ! -path '*/biz/testkit/*' -print0 |
    xargs -0 -r grep -n -E -- '(^|[^*[:alnum:]_])((runmodel\.)?ReplacementIntent|runmodel\.Run)[[:space:]]*\{' || true
} | sed '/^[[:space:]]*$/d')"
if [[ -n "$workorchd_run_authority_matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: Workorchd constructs Run or ReplacementIntent
owner state. Application may compile a typed bootstrap and submit Workspace
execution admission, but successor/replacement materialization belongs to Run.

$workorchd_run_authority_matches
EOF
  exit 1
fi

workorchd_server_compatibility_state_matches="$({
  source_matches 'sharedSession|runtimeSessionErr|hasSharedSession|withSharedSession|compatRead|workspaceProjectionObserver|workspaceProjectionLifecycle|manageWorkspaceProjectionLifecycle|newWorkspaceProjectionManager|productionWiring' \
    internal/modules/application/workorchd/service/server
} | sed '/^[[:space:]]*$/d')"
if [[ -n "$workorchd_server_compatibility_state_matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: Workorchd production server retains a Session,
cache-authority, projection-lifecycle, or dual-wiring compatibility path.

$workorchd_server_compatibility_state_matches
EOF
  exit 1
fi

retired_application_state_surface_matches="$({
  source_matches 'workorchd\.scheduler|WatchdogDiagnostics|ContinuationReadbackDiagnostic|LatestContinuation|RecentContinuations|schedulerDiagnosticState(NotScheduled|Accepted|Running|Failed|Completed|NoActionNeeded)|recordWatchdog|recordStaleRuntimeGapRecovery|ListRecoverableRuntimeGapRunCandidates|IsRecoverableRunningRun|LatestRuntimeCheckpointModel|TerminalStepModel|PreviousStepModelID|ResolvedDecisionBlockerContext|ApprovedApprovalBlockerContext|BlockedRunResumeCompressedContext|NextAction:[[:space:]]*"resume_run"|MaxConcurrentRuns|StaleDispatchedStepMinAge|runInFlight|runCreateRequests|runStartRequests|runResumeRequests|observedRepairAttempts|convergenceRepairAttempts|worker_lifecycle|worker_lost|workorch_run_requests_in_flight|workorch_server_max_concurrent_runs|workorch_stale_dispatched_step_min_age_seconds|workorch_runtime_event_publish_failures_total|workorch_runtime_heartbeat_timer_start_failures_total|workorch_scheduler_continuation_|workorch_watchdog_' \
    internal/modules/application/workorchd \
    internal/pkg/workorchconfig \
    internal/pkg/workorchconfigurationcontract \
    internal/pkg/workorchport \
    internal/pkg/workorchprojectioncontract \
    internal/services/workorch
} | sed '/^[[:space:]]*$/d')"
if [[ -n "$retired_application_state_surface_matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: a retired Workorchd scheduler identity/state,
watchdog recovery readback, projection field, or metric was reintroduced.
Execution admission may expose only an immutable admission outcome.

$retired_application_state_surface_matches
EOF
  exit 1
fi

# Deployed verifiers must read owner state and the immutable runCommand
# admission outcome.  They must not make removed Workorchd continuation
# diagnostics, scheduler counters, watchdog recovery, or an automatically
# created successor part of the passing contract.
retired_authority_verifier_matches="$({
  find scripts -maxdepth 1 -type f -name '*.sh' ! -name 'verify-boundary-contract.sh' -print0 |
    xargs -0 -r grep -Hn -E -- 'latestContinuation|recentContinuations|latestAdmissionFailure|recentAdmissionFailures|schedulerDiagnostics|scheduler_recent_items|governance_scheduler_diagnostics|has_failed_continuation_diagnostic|successor_or_failed_diagnostic_present|resolved_replan_without_successor_or_failed_diagnostic' || true
  grep -Hn -E -- 'WORKORCH_OBSERVE_REQUIRE_GOVERNANCE_CONTINUATION' Makefile || true
  source_matches 'scheduler_recent_items|governance_scheduler_diagnostics|has_failed_continuation_diagnostic|successor_or_failed_diagnostic_present|resolved_replan_without_successor_or_failed_diagnostic|workorch_scheduler_commands_total|workorch_scheduler_continuation_|workorch_watchdog_' \
    scripts/verify-authority-live-e2e-closure.sh
} | sed '/^[[:space:]]*$/d')"
if [[ -n "$retired_authority_verifier_matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: a deployed authority verifier requires retired
application scheduler/watchdog/continuation state or automatic successor
evidence. Verify owner readback and immutable runCommand admission outcomes.

$retired_authority_verifier_matches
EOF
  exit 1
fi

for verifier in scripts/verify-authority-live-e2e-closure.sh scripts/observe-workorch-runtime-closure.sh; do
  if ! grep -q -F -- 'runCommand' "$verifier"; then
    echo "boundary contract violation: $verifier does not inspect immutable runCommand admission outcomes" >&2
    exit 1
  fi
done

retired_application_metric_markers=(
  "workorch_scheduler_commands_total"
  "workorch_scheduler_continuation_failures_total"
  "workorch_scheduler_continuation_no_action_total"
  "workorch_watchdog_timer_registrations_total"
  "workorch_watchdog_interval_seconds"
  "workorch_watchdog_ticks_total"
  "workorch_watchdog_last_error_timestamp_seconds"
  "workorch_watchdog_stale_runtime_gap_recoveries_total"
  "workorch_run_requests_in_flight"
  "workorch_server_max_concurrent_runs"
  "workorch_stale_dispatched_step_min_age_seconds"
  "workorch_runtime_event_publish_failures_total"
  "workorch_runtime_heartbeat_timer_start_failures_total"
)
observer_negative_metric_block="$(sed -n '/^retired_application_state_metric_markers = \[/,/^\]/p' scripts/observe-workorch-runtime-closure.sh)"
for marker in "${retired_application_metric_markers[@]}"; do
  if ! grep -q -F -- "$marker" <<<"$observer_negative_metric_block"; then
    echo "boundary contract violation: closure observer lost the negative assertion for retired metric $marker" >&2
    exit 1
  fi
done

retired_metric_positive_usage="$({
  find scripts -maxdepth 1 -type f \( -name '*.sh' -o -name '*.py' \) \
    ! -name 'verify-boundary-contract.sh' \
    ! -name 'observe-workorch-runtime-closure.sh' -print0 |
    xargs -0 -r grep -Hn -E -- 'workorch_scheduler_commands_total|workorch_scheduler_continuation_|workorch_watchdog_|workorch_run_requests_in_flight|workorch_server_max_concurrent_runs|workorch_stale_dispatched_step_min_age_seconds|workorch_runtime_event_publish_failures_total|workorch_runtime_heartbeat_timer_start_failures_total|worker_lifecycle|worker_lost' || true
  grep -Hn -E -- 'workorch_scheduler_commands_total|workorch_scheduler_continuation_|workorch_watchdog_|workorch_run_requests_in_flight|workorch_server_max_concurrent_runs|workorch_stale_dispatched_step_min_age_seconds|workorch_runtime_event_publish_failures_total|workorch_runtime_heartbeat_timer_start_failures_total|worker_lifecycle|worker_lost' Makefile || true
  awk '
    /^retired_application_state_metric_markers = \[/ { negative = 1; next }
    negative && /^\]/ { negative = 0; next }
    /workorch_scheduler_commands_total|workorch_scheduler_continuation_|workorch_watchdog_|workorch_run_requests_in_flight|workorch_server_max_concurrent_runs|workorch_stale_dispatched_step_min_age_seconds|workorch_runtime_event_publish_failures_total|workorch_runtime_heartbeat_timer_start_failures_total|worker_lifecycle|worker_lost/ && !negative { print FNR ":" $0 }
  ' scripts/observe-workorch-runtime-closure.sh
} | sed '/^[[:space:]]*$/d')"
if [[ -n "$retired_metric_positive_usage" ]]; then
  cat >&2 <<EOF
Boundary contract violation: a production/verification target consumes a
retired application scheduler/watchdog metric. Those names may appear only in
the closure observer's explicit "must be absent" marker block.

$retired_metric_positive_usage
EOF
  exit 1
fi

retired_execution_scheduler_config_matches="$({
  find application config docs internal scripts -type f \
    \( -name '*.go' -o -name '*.toml' -o -name '*.md' -o -name '*.py' -o -name '*.sh' \) \
    ! -path 'scripts/verify-boundary-contract.sh' \
    ! -path 'internal/pkg/workorchconfig/retired_server_config_test.go' -print0 |
    xargs -0 -r grep -Hn -E -- 'maxConcurrentRuns|staleDispatchedStepMinAge|WORKORCH_MANAGED_MAX_CONCURRENT_RUNS|WORKORCH_MANAGED_STALE_DISPATCHED_STEP_MIN_AGE|WORKORCH_LOAD_EXPECT_MAX_CONCURRENT_RUNS|WORKORCH_OBSERVE_STALE_DISPATCHED_STEP_MIN_AGE_SECONDS|WORKORCH_OBSERVE_MAX_RUNTIME_EVENT_PUBLISH_FAILURES_DELTA' || true
} | sed '/^[[:space:]]*$/d')"
if [[ -n "$retired_execution_scheduler_config_matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: a retired Workorchd execution scheduler/watchdog
configuration surface was reintroduced. Client concurrency and owner Runtime
facts must be observed directly; Application must not expose a parallel run
limit or stale-dispatch recovery policy.

$retired_execution_scheduler_config_matches
EOF
  exit 1
fi

retired_server_config_test='internal/pkg/workorchconfig/retired_server_config_test.go'
for marker in 'maxConcurrentRuns' 'staleDispatchedStepMinAge' 'TestCurrentRejectsRetiredServerSchedulerFields' 'TestPersistedRejectsRetiredServerSchedulerFields'; do
  if ! grep -q -F -- "$marker" "$retired_server_config_test"; then
    echo "boundary contract violation: retired server config rejection test lost marker $marker" >&2
    exit 1
  fi
done

retired_web_execution_shadow_matches="$({
  source_matches 'export type RuntimeExecution[[:space:]]*=|runtimeExecution\?:[[:space:]]*RuntimeExecution|queuedAt\?:|workorch_run_queue_|workorch_server_max_concurrent_runs|workorch_run_requests_in_flight|schedulerDiagnostics|maxConcurrentRuns[?]?:|staleDispatchedStepMinAge[?]?:' \
    web/workorch-console/src
} | sed '/^[[:space:]]*$/d')"
if [[ -n "$retired_web_execution_shadow_matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: Web Console reintroduced the retired application
execution scheduler/worker projection or one of its queue/concurrency controls.
Render owner facts and immutable runCommand outcomes instead.

$retired_web_execution_shadow_matches
EOF
  exit 1
fi

# Eventstream is audit/display infrastructure. Goal/DoD selection, effective
# definition compilation, stale checks and definition consolidation feed SM-4
# Runtime bootstrap or owner writes and therefore must never read EventRecord.
eventstream_runtime_authority_matches="$({
  source_matches 'internal/modules/blocks/eventstream|EventRecord(Reader|Lister)|workorchquerycontract\.EventRecordQuery|eventstreamevents\.EventRecordData' \
    internal/modules/application/workorchd/biz/appservice/activationcore \
    internal/modules/application/workorchd/biz/appservice/activationports \
    internal/modules/application/workorchd/biz/appservice/definitionports \
    internal/modules/application/workorchd/biz/appservice/definitionquality
} | sed '/^[[:space:]]*$/d')"
if [[ -n "$eventstream_runtime_authority_matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: EventRecord/audit data participates in Runtime
bootstrap, definition selection/staleness, or Workspace definition writes.
Use Workspace owner Goal/DoD revision and lineage state only.

$eventstream_runtime_authority_matches
EOF
  exit 1
fi

numeric_projection_family_files="$(find internal/modules/application/workorchd/biz/appservice/readmodelprojection -maxdepth 1 -type f -name 'projection_family_[0-9]*.go' -print)"
if [[ -n "$numeric_projection_family_files" ]]; then
  cat >&2 <<EOF
Boundary contract violation: numeric read-model projection family filenames
must be replaced by semantic responsibility names.

$numeric_projection_family_files
EOF
  exit 1
fi

# Workspace scope is per EventHub execution, never component-instance state.
# The state owners and Runtime drive must use the checked binder rather
# than silently replacing a pre-existing workspace context from a command.
workspace_scope_bindings=(
  "internal/modules/blocks/run/biz/command_router.go"
  "internal/modules/blocks/run/biz/lifecycle_handler.go"
  "internal/modules/blocks/workspace/biz/command_router.go"
  "internal/modules/blocks/result/biz/owner.go"
  "internal/modules/blocks/result/biz/lifecycle_handler.go"
  "internal/modules/blocks/decision/biz/command_router.go"
  "internal/modules/blocks/approval/biz/command_router.go"
  "internal/modules/blocks/memory/biz/command_router.go"
  "internal/modules/blocks/eventstream/biz/event_stream_service.go"
  "internal/modules/kernel/orchestrator/biz/runtime_drive.go"
  "internal/modules/kernel/orchestrator/biz/runtime_operation_executor.go"
  "internal/modules/kernel/orchestrator/biz/governance_evidence_command.go"
  "internal/modules/kernel/orchestrator/biz/owner.go"
  "internal/modules/kernel/governance/biz/worker_reconciliation.go"
  "internal/modules/application/workorchd/biz/workspace_command_gateway.go"
)
for path in "${workspace_scope_bindings[@]}"; do
  if ! grep -q -E -- 'BindRuntimeWorkspaceContext|resultCommandContext' "$path"; then
    echo "boundary contract violation: workspace-scoped owner path must validate EventHub context: $path" >&2
    exit 1
  fi
done

# Resource lifecycle reducers are owner-private implementation details.  A
# production EventHub handler must translate a validated intent/fact into the
# owner's private event dispatcher before selecting a reducer or persistence
# path. Tests may call the reducer directly to exercise transition tables.
owner_private_reducer_matches="$({
  source_matches 'applyLifecycleLocked\(' internal/modules/blocks/run/biz |
    grep -v -F -- 'internal/modules/blocks/run/biz/lifecycle_internal_event.go' |
    grep -v -E -- 'func \(m \*Biz\) applyLifecycleLocked\(' || true
  source_matches '(applyTerminalDelivery|applyRunProjection|releaseFreshActivationRunSlot)\(' internal/modules/blocks/workspace/biz |
    grep -v -F -- 'internal/modules/blocks/workspace/biz/lifecycle_internal_event.go' |
    grep -v -E -- 'func \(m \*Biz\) (applyTerminalDelivery|applyRunProjection|releaseFreshActivationRunSlot)\(' || true
  source_matches 'applyTerminalDelivery\(' internal/modules/blocks/result/biz |
    grep -v -F -- 'internal/modules/blocks/result/biz/lifecycle_internal_event.go' |
    grep -v -E -- 'func \(m \*Biz\) applyTerminalDelivery\(' || true
	 source_matches '(\.State[[:space:]]*=[[:space:]]*[^=]|\.RuntimeGovernanceProposal\.State[[:space:]]*=[[:space:]]*[^=]|\.Update\()' internal/modules/blocks/decision/biz |
	   grep -v -F -- 'internal/modules/blocks/decision/biz/lifecycle_internal_event.go' || true
	 source_matches '(\.State[[:space:]]*=[[:space:]]*[^=]|\.Update\()' internal/modules/blocks/approval/biz |
	   grep -v -F -- 'internal/modules/blocks/approval/biz/lifecycle_internal_event.go' || true
} | sed '/^[[:space:]]*$/d')"
if [[ -n "$owner_private_reducer_matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: owner lifecycle reducer bypasses its private
internal-event dispatcher.

$owner_private_reducer_matches
EOF
  exit 1
fi

# Initial owner state is a lifecycle choice as well. Object literals used by
# public services historically bypassed the private dispatcher because the
# assignment form above only saw later mutations.
owner_initial_state_bypass_matches="$({
  source_matches 'State:[[:space:]]*common\.WorkspaceState' internal/modules/blocks/workspace/biz |
    grep -v -F -- 'internal/modules/blocks/workspace/biz/creation_internal_event.go' || true
  source_matches 'State:[[:space:]]*common\.RunState' internal/modules/blocks/run/biz |
    grep -v -F -- 'internal/modules/blocks/run/biz/creation_internal_event.go' || true
  source_matches 'State:[[:space:]]*common\.DecisionTicketState' internal/modules/blocks/decision/biz |
    grep -v -F -- 'internal/modules/blocks/decision/biz/lifecycle_internal_event.go' || true
  source_matches 'State:[[:space:]]*common\.ApprovalState' internal/modules/blocks/approval/biz |
    grep -v -F -- 'internal/modules/blocks/approval/biz/lifecycle_internal_event.go' || true
  source_matches 'State:[[:space:]]*decisionModel\.RuntimeGovernanceResolutionProposal' internal/modules/blocks/decision/biz |
    grep -v -F -- 'internal/modules/blocks/decision/biz/lifecycle_internal_event.go' || true
  source_matches 'State:[[:space:]]*approvalmodel\.ResolutionProposal' internal/modules/blocks/approval/biz |
    grep -v -F -- 'internal/modules/blocks/approval/biz/lifecycle_internal_event.go' || true
  source_matches 'LineageState:[[:space:]]*"active"' internal/modules/blocks/run/biz |
    grep -v -F -- 'internal/modules/blocks/run/biz/creation_internal_event.go' || true
  source_matches '(workspaces|runs|tickets|approvals)\.Create\(' \
    internal/modules/blocks/workspace/biz internal/modules/blocks/run/biz internal/modules/blocks/decision/biz internal/modules/blocks/approval/biz |
    grep -v -E -- '/(creation|lifecycle)_internal_event\.go:' || true
  source_matches '(binding|oldBinding)\.(ResolutionID|ResolutionBindingHash|DefinitionRevision|StartupInputHash|ReplacementIntentID|ReplacementIntentHash|SuccessorDefinitionRevision|SuccessorStartupInputHash)[[:space:]]*=[[:space:]]*[^=]' \
    internal/modules/blocks/decision/biz |
    grep -v -F -- 'internal/modules/blocks/decision/biz/lifecycle_internal_event.go' || true
} | sed '/^[[:space:]]*$/d')"
if [[ -n "$owner_initial_state_bypass_matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: owner creation state or clarification binding
bypasses its owner-private internal-event dispatcher.

$owner_initial_state_bypass_matches
EOF
  exit 1
fi

# Resource-owner Biz packages must not export or embed a generic mutation
# service. Cross-owner callers use typed EventHub commands; owner-local code
# reaches private helpers and private internal-event dispatchers only.
resource_owner_biz_roots=(
  internal/modules/blocks/workspace/biz
  internal/modules/blocks/run/biz
  internal/modules/blocks/result/biz
  internal/modules/blocks/decision/biz
  internal/modules/blocks/approval/biz
  internal/modules/blocks/memory/biz
  internal/modules/blocks/skill/biz
  internal/modules/blocks/soul/biz
  internal/modules/blocks/mcpregistry/biz
  internal/modules/blocks/eventstream/biz
)
owner_generic_service_facade_pattern='^type ([A-Z][A-Za-z0-9]*)?Service[[:space:]]+(struct|interface)|^func New([A-Z][A-Za-z0-9]*)?Service\('
owner_private_service_export_pattern='^func[[:space:]]+\([^)]*\*(service|[a-z][A-Za-z0-9]*Service)\)[[:space:]]+[A-Z][A-Za-z0-9_]*\('

if ((${#resource_owner_biz_roots[@]} != 10)); then
  echo "boundary contract violation: resource-owner facade scan must cover exactly 10 owner Biz roots" >&2
  exit 1
fi
for owner_root in "${resource_owner_biz_roots[@]}"; do
  if [[ ! -d "$owner_root" ]]; then
    echo "boundary contract violation: resource-owner facade scan path does not exist: $owner_root" >&2
    exit 1
  fi
done

# Inline negative fixtures keep both halves of the scanner executable. The
# labels also prove the four formerly omitted owners remain in the explicit
# ten-owner inventory rather than relying on an incidental glob.
owner_facade_guard_fixtures=(
  'memory|type Service struct {'
  'skill|func NewService(repository any) *service {'
  'soul|func (s *soulService) UpdateSoul() {'
  'mcpregistry|func (s *service) RegistrySnapshot() {'
)
for fixture in "${owner_facade_guard_fixtures[@]}"; do
  fixture_owner="${fixture%%|*}"
  fixture_source="${fixture#*|}"
  if ! printf '%s\n' "${resource_owner_biz_roots[@]}" | grep -q -x -F -- "internal/modules/blocks/$fixture_owner/biz"; then
    echo "boundary contract violation: resource-owner facade fixture is outside the scanned inventory: $fixture_owner" >&2
    exit 1
  fi
  if ! grep -q -E -- "$owner_generic_service_facade_pattern|$owner_private_service_export_pattern" <<<"$fixture_source"; then
    echo "boundary contract violation: resource-owner facade scanner missed its $fixture_owner negative fixture" >&2
    exit 1
  fi
done

owner_mutation_facade_matches="$({
  source_matches "$owner_generic_service_facade_pattern" "${resource_owner_biz_roots[@]}" || true
  source_matches "$owner_private_service_export_pattern" \
    internal/modules/blocks/memory/biz internal/modules/blocks/skill/biz \
    internal/modules/blocks/soul/biz internal/modules/blocks/mcpregistry/biz || true
  source_matches '^[[:space:]]*\*Service$|func \([^)]*\) (CreateRun|EnsureRun)\(' \
    internal/modules/blocks/run/biz internal/modules/blocks/result/biz || true
  source_matches '^type GoalDoDManager struct|^func NewGoalDoDManager' \
    internal/modules/blocks/workspace/biz || true
  source_matches 'PurgeWorkspaceEvents' \
    internal/modules/blocks/eventstream/biz || true
  source_matches 'eventstreambiz\.Service|eventstreambiz\.New([A-Z][A-Za-z0-9]*)?Service' \
    internal/modules/blocks/eventstream/module.go || true
  grep -R -n -E --include='*.go' -- \
    'github.com/muidea/workorch/internal/modules/blocks/approval/biz/contracts' internal \
    | grep -v -E '^internal/modules/blocks/approval/' || true
  grep -R -n -E --include='*.go' -- \
    'github.com/muidea/workorch/internal/modules/blocks/decision/biz/contracts' internal \
    | grep -v -E '^internal/modules/blocks/decision/' || true
} | sed '/^[[:space:]]*$/d')"
if [[ -n "$owner_mutation_facade_matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: a resource owner exposes a generic mutation
service/facade, Eventstream exposes direct purge, or a sibling imports
Approval/Decision owner-private use-case contracts, bypassing typed EventHub
ingress and the owner-private reducer.

$owner_mutation_facade_matches
EOF
  exit 1
fi

# Eventstream workspace deletion is one owner-private reducer reachable only
# from concrete synchronous EventHub commands sent by the Workspace owner.
# Physical cleanup uses the exact persisted fence sink; ordinary repository
# purge/delete cannot cross a sealed tombstone.
eventstream_lifecycle_file="internal/modules/blocks/eventstream/biz/event_stream_service.go"
eventstream_deletion_file="internal/modules/blocks/eventstream/biz/workspace_deletion_internal_event.go"
eventstream_exported_biz_surface_matches="$({
  source_matches '^func[[:space:]]+[A-Z][A-Za-z0-9_]*([[:space:]]*\[[^]]+\])?[[:space:]]*\(' internal/modules/blocks/eventstream/biz |
    grep -v -E -- ':func[[:space:]]+New\(' || true
  source_matches '^func[[:space:]]+\([^)]*\)[[:space:]]+[A-Z][A-Za-z0-9_]*\(' internal/modules/blocks/eventstream/biz |
    grep -v -E -- ':func[[:space:]]+\([^)]*\*?Biz\)[[:space:]]+(Start|Stop)\(' || true
} | sed '/^[[:space:]]*$/d')"
eventstream_deletion_ingress_implementation_matches="$(source_matches '^func[[:space:]]+\([^)]*\)[[:space:]]+executeWorkspaceDeletionCommand\(' internal/modules/blocks/eventstream/biz)"
eventstream_deletion_ingress_call_matches="$(source_matches '\.executeWorkspaceDeletionCommand([[:space:]]|\(|$)' internal/modules/blocks/eventstream/biz)"
eventstream_purge_implementation_matches="$(source_matches '^func[[:space:]]+\([^)]*\)[[:space:]]+purgeWorkspace\(' internal/modules/blocks/eventstream/biz)"
eventstream_purge_call_matches="$(source_matches '\.purgeWorkspace([[:space:]]|\(|$)' internal/modules/blocks/eventstream/biz)"
eventstream_exact_repository_purge_matches="$(source_matches '\.PurgeWorkspaceEventRecords([[:space:]]|\(|$)' internal/modules/blocks/eventstream/biz)"
eventstream_ordinary_repository_purge_matches="$(source_matches '\.PurgeWorkspace([[:space:]]|\(|$)' internal/modules/blocks/eventstream/biz)"
eventstream_deleted_receipt_audit_matches="$(source_matches 'workspaceevents\.TopicWorkspaceDeleted' "$eventstream_lifecycle_file")"
eventstream_legacy_application_purge_matches="$({
  source_matches 'eventstreamApplicationCommandSource|application/workorchd/appservice' internal/modules/blocks/eventstream/biz |
    grep -v -E -- '_test\.go:' || true
} | sed '/^[[:space:]]*$/d')"
if [[ -n "$eventstream_exported_biz_surface_matches" ]] ||
   ! grep -q -F -- 'func New(deps Dependencies) *Biz' "$eventstream_lifecycle_file" ||
   ! grep -q -F -- 'func (s *Biz) Start() error' "$eventstream_lifecycle_file" ||
   ! grep -q -F -- 'func (s *Biz) Stop()' "$eventstream_lifecycle_file" ||
   [[ "$(awk 'NF { count++ } END { print count + 0 }' <<<"$eventstream_deletion_ingress_implementation_matches")" -ne 1 ]] ||
   [[ "$(awk 'NF { count++ } END { print count + 0 }' <<<"$eventstream_deletion_ingress_call_matches")" -ne 1 ]] ||
   [[ "$(awk 'NF { count++ } END { print count + 0 }' <<<"$eventstream_purge_implementation_matches")" -ne 1 ]] ||
   [[ "$(awk 'NF { count++ } END { print count + 0 }' <<<"$eventstream_purge_call_matches")" -ne 1 ]] ||
   [[ "$(awk 'NF { count++ } END { print count + 0 }' <<<"$eventstream_exact_repository_purge_matches")" -ne 1 ]] ||
   [[ -n "$eventstream_ordinary_repository_purge_matches" ]] ||
   [[ -n "$eventstream_deleted_receipt_audit_matches" ]] ||
   [[ -n "$eventstream_legacy_application_purge_matches" ]] ||
   ! grep -q -F -- 'case eventstreamevents.TopicEventStreamCommandSealWorkspaceDeletion:' "$eventstream_deletion_file" ||
   ! grep -q -F -- 'case eventstreamevents.TopicEventStreamCommandPurgeWorkspace:' "$eventstream_deletion_file" ||
   ! grep -q -F -- 'cmd, ok := ev.Data().(eventstreamevents.SealWorkspaceEventStreamDeletionCommand)' "$eventstream_deletion_file" ||
   ! grep -q -F -- 'cmd, ok := ev.Data().(eventstreamevents.PurgeWorkspaceCommand)' "$eventstream_deletion_file" ||
   ! grep -q -F -- 'ev.Source() != workspaceDeletionCommandSource' "$eventstream_deletion_file"; then
  echo "boundary contract violation: Eventstream Biz may export only New/Start/Stop and deletion must use one Workspace-owned typed ingress plus the exact fenced purge sink" >&2
  if [[ -n "$eventstream_exported_biz_surface_matches" ]]; then
    echo "$eventstream_exported_biz_surface_matches" >&2
  fi
  if [[ -n "$eventstream_legacy_application_purge_matches" ]]; then
    echo "$eventstream_legacy_application_purge_matches" >&2
  fi
  if [[ -n "$eventstream_deleted_receipt_audit_matches" ]]; then
    echo "$eventstream_deleted_receipt_audit_matches" >&2
  fi
  exit 1
fi

# A Runtime PhysicalCallID reserves exactly one provider submission. The LLM
# HTTP owner must reject every redirect before net/http can replay a POST body;
# an injected CheckRedirect callback cannot authorize a second call.
provider_http_client_shape="$(sed -n '/^func providerHTTPClient(/,/^}/p' internal/modules/blocks/llm/biz/http_provider.go)"
if ! grep -q -F -- 'return providerProtocolError("redirect_rejected", 0)' <<<"$provider_http_client_shape" ||
   grep -q -E -- '(redirect_limit_exceeded|return previous\(|return nil)' <<<"$provider_http_client_shape"; then
  cat >&2 <<EOF
Boundary contract violation: the LLM provider HTTP client must reject every
redirect before a second physical submission under the same PhysicalCallID.
EOF
  exit 1
fi

# Application execution admission is synchronous with Workspace owner. Keep
# deleted application-local queue/state-owner surfaces from returning under a
# different call site, and keep setup draft state a response-only derivation.
application_execution_state_bypass_matches="$({
  source_matches '(runQueueWaiters|activeRunSlots|EnqueueWorkspaceExecution|RecordWorkspaceExecutionQueued|RecordWorkspaceExecutionEnqueueFailure|RecordWorkspaceExecutionTransportQueued|RecordWorkspaceExecutionTransportEnqueueFailure|RunCommandQueuedUpdate|ApplyRunCommandQueued|QueuedAt|recordCommandEnqueueFailure|scheduler_enqueue_failed|workorch_run_queue_)' \
    internal/modules/application/workorchd || true
  source_matches '(runQueueWaiting|workorch_run_queue_|workorch_scheduler_command_enqueue_failures_total|scheduler_enqueue_failed)' scripts |
    grep -v -F -- 'scripts/verify-boundary-contract.sh' || true
  source_matches 'session\.State[[:space:]]*=|State:[[:space:]]*workspaceSetupDraftState' \
    internal/modules/application/workorchd/service/server/workspace_setup_draft.go || true
  source_matches '(ApplyWorkspaceExecutionAdmission|applyProjectedWorkspaceExecutionAdmission)' \
    internal/modules/application/workorchd/service/server || true
  source_matches 'ApplyWorkspaceExecutionAdmission.*int64' \
    internal/modules/application/workorchd/biz || true
  source_matches 'IdempotencyKey|idempotencyKey[[:space:]]*[?=:]|idempotency[_-]key[[:space:]]*[:=]' \
    internal/modules/application/workorchd \
    internal/pkg/workorchprojectioncontract \
    internal/services/workorch/biz/backends \
    internal/services/workorch/service/tui \
    web/workorch-console/src \
    scripts |
    grep -v -F -- 'scripts/verify-boundary-contract.sh' || true
  grep -R -n -E --exclude-dir='90-archive' -- 'optional[-_]idempotency[-_]key|"idempotencyKey"[[:space:]]*:|"idempotency_key"[[:space:]]*:|"idempotency-key"[[:space:]]*:' docs || true
} | sed '/^[[:space:]]*$/d')"
if [[ -n "$application_execution_state_bypass_matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: Application owns an execution queue/state,
fabricates a run-command idempotency identity, lets transport write the
execution observation, lets a caller select its projection revision, or
persists a setup-draft state that must remain derived.

$application_execution_state_bypass_matches
EOF
  exit 1
fi

# Execution admission has no scheduler and therefore accepts no Run priority.
# Keep the retired caller field out of every cross-owner command and persisted
# Run identity while allowing independent acceptance/context/Soul priorities.
execution_priority_contract_matches="$({
  {
    sed -n '/^type WorkspaceExecutionCommandRequest struct {/,/^}/p' internal/pkg/workorchmanagementcontract/definition.go
    sed -n '/^type WorkspaceRunCommand struct {/,/^}/p' internal/pkg/workorchport/run.go
    sed -n '/^type WorkspaceRunRequestedCommand struct {/,/^}/p' internal/modules/application/workorchd/pkg/events/messages.go
    sed -n '/^type WorkspaceExecutionIntentCommand struct {/,/^}/p' internal/modules/blocks/workspace/pkg/events/lifecycle_messages.go
    sed -n '/^type WorkspaceExecutionIntentCommand struct {/,/^}/p' internal/modules/blocks/run/pkg/events/lifecycle_messages.go
    sed -n '/^type ReplacementIntent struct {/,/^}/p' internal/modules/blocks/run/pkg/models/run.go
    sed -n '/^type Run struct {/,/^}/p' internal/modules/blocks/run/pkg/models/run.go
    sed -n '/^type Run struct {/,/^}/p' internal/pkg/workorchprojectioncontract/resource.go
  } | grep -n -E -- 'Priority|json:"priority' || true
  source_matches 'runPriorityFromRequest|X-Workorch-Run-Priority|Flags\(\)\.Int\("priority"|requestBody\.priority' \
    internal/modules/application/workorchd \
    internal/services/workorch \
    web/workorch-console/src || true
  while IFS= read -r script_file; do
    awk '
      /\/execution-activations/ { remaining = 20 }
      remaining > 0 {
        if ($0 ~ /"priority"[[:space:]]*:/) {
          print FILENAME ":" FNR ":" $0
        }
        remaining--
      }
    ' "$script_file"
  done < <(find scripts -type f \( -name '*.sh' -o -name '*.py' \) | sort)
} | sed '/^[[:space:]]*$/d')"
if [[ -n "$execution_priority_contract_matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: retired caller-controlled Run priority returned
to an execution admission, cross-owner command, persisted Run, verifier, load
test, or UI surface.

$execution_priority_contract_matches
EOF
  exit 1
fi

# Secondary owner transitions and audit/read-model carriers follow the same
# rule: public/cross-owner handlers translate immutable facts, while the sole
# owner uses its private dispatchers to select and persist the next state.
# Multiple coordinated transition IDs are allowed only inside that same owner.
owner_secondary_state_bypass_matches="$({
  source_matches '\.(DefinitionAdmissionCandidates|StartupInput|ActivationAdmission)(\.[A-Za-z0-9_]+)?[[:space:]]*=[[:space:]]*[^=]' \
    internal/modules/blocks/workspace/biz |
    grep -v -F -- 'internal/modules/blocks/workspace/biz/definition_admission_internal_event.go' || true
  source_matches '\.(ReplacementIntent(\.[A-Za-z0-9_]+)?|Lineage\.SuccessorRunID)[[:space:]]*=[[:space:]]*[^=]|[A-Za-z0-9_]+\.State[[:space:]]*=[[:space:]]*runmodel\.ReplacementIntent|State:[[:space:]]*runmodel\.ReplacementIntent|updateReplacementIntent' \
    internal/modules/blocks/run/biz |
    grep -v -F -- 'internal/modules/blocks/run/biz/replacement_internal_event.go' || true
  source_matches '\.Status[[:space:]]*=[[:space:]]*[^=]|s\.traces\.Upsert\(' \
    internal/modules/kernel/governance/biz |
    grep -v -F -- 'internal/modules/kernel/governance/biz/trace_internal_event.go' || true
  source_matches '\.State[[:space:]]*=[[:space:]]*[^=]|\.Update\(' \
    internal/modules/kernel/orchestrator/biz/runtime_candidate_request_content.go || true
  source_matches 's\.state[[:space:]]*=[[:space:]]*[^=]' \
    internal/modules/application/workorchd/biz/appservice/workspaceviewcore |
    grep -v -F -- 'internal/modules/application/workorchd/biz/appservice/workspaceviewcore/store_lifecycle_internal_event.go' || true
} | sed '/^[[:space:]]*$/d')"
if [[ -n "$owner_secondary_state_bypass_matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: a secondary owner state machine, audit carrier,
or derived projection lifecycle bypasses its owner-private internal-event
dispatcher.

$owner_secondary_state_bypass_matches
EOF
  exit 1
fi

# Keep one verification-only inventory for every formal Runtime machine and
# every owner-private transition/effect gate.  This is deliberately not a Go
# production registry: reducers cannot use it to select transitions.  It makes
# owner identity, each private dispatcher, and its exact reducer/persistence
# sinks reviewable, and makes a newly added *_internal_event(s).go fail CI
# until it is classified here. One state type may reference coordinated
# transitions only when they retain one owner.
state_transition_owner_manifest="scripts/state-transition-owners.manifest"
if [[ ! -f "$state_transition_owner_manifest" ]]; then
  echo "boundary contract violation: state-transition owner manifest is missing: $state_transition_owner_manifest" >&2
  exit 1
fi
production_manifest_consumers="$(source_matches 'state-transition-owners\.manifest' application internal | grep -v -F -- 'internal/tools/stateownercheck/' || true)"
if [[ -n "$production_manifest_consumers" ]]; then
  cat >&2 <<EOF
Boundary contract violation: the verification-only state-transition manifest
is consumed by production code and can become a global reducer registry.

$production_manifest_consumers
EOF
  exit 1
fi

state_model_owner_manifest="scripts/state-model-owners.manifest"
if [[ ! -f "$state_model_owner_manifest" ]]; then
  echo "boundary contract violation: exhaustive state-model owner manifest is missing: $state_model_owner_manifest" >&2
  exit 1
fi
production_state_model_manifest_consumers="$(source_matches 'state-model-owners\.manifest' application internal | grep -v -F -- 'internal/tools/stateownercheck/' || true)"
if [[ -n "$production_state_model_manifest_consumers" ]]; then
  cat >&2 <<EOF
Boundary contract violation: the verification-only state-model inventory is
consumed by production code and can become a global state/reducer registry.

$production_state_model_manifest_consumers
EOF
  exit 1
fi

declare -A state_transition_ids=()
declare -A state_transition_owner_roots=()
declare -A state_transition_classifications=()
declare -A declared_internal_event_files=()
declare -A formal_runtime_machine_owners=()
manifest_line=0
while IFS='|' read -r transition_id classification owner_root internal_event_file dispatcher_file dispatcher_symbol internal_event_type transition_sinks unexpected; do
  manifest_line=$((manifest_line + 1))
  if [[ -z "$transition_id" || "$transition_id" == \#* ]]; then
    continue
  fi
  if [[ -n "${unexpected:-}" || -z "$classification" || -z "$owner_root" || -z "$internal_event_file" || -z "$dispatcher_file" || -z "$dispatcher_symbol" || -z "$internal_event_type" || -z "$transition_sinks" ]]; then
    echo "boundary contract violation: malformed state-transition owner manifest row $manifest_line" >&2
    exit 1
  fi
  if [[ ! "$transition_id" =~ ^[a-z0-9_.-]+$ || ! "$dispatcher_symbol" =~ ^[A-Za-z_][A-Za-z0-9_]*$ || ! "$internal_event_type" =~ ^[A-Za-z_][A-Za-z0-9_]*$ || ! "$transition_sinks" =~ ^[A-Za-z_][A-Za-z0-9_.]*(,[A-Za-z_][A-Za-z0-9_.]*)*$ ]]; then
    echo "boundary contract violation: invalid transition or dispatcher identity at manifest row $manifest_line" >&2
    exit 1
  fi
  if [[ -n "${state_transition_ids[$transition_id]+x}" ]]; then
    echo "boundary contract violation: duplicate state-transition identity in manifest: $transition_id" >&2
    exit 1
  fi
  state_transition_ids["$transition_id"]=1
  state_transition_owner_roots["$transition_id"]="$owner_root"
  state_transition_classifications["$transition_id"]="$classification"

  case "$classification" in
    formal_runtime_machine|resource_state_machine|audit_carrier|owner_private_transaction|owner_private_effect_gate|derived_projection_lifecycle) ;;
    *)
      echo "boundary contract violation: unknown state-transition classification at row $manifest_line: $classification" >&2
      exit 1
      ;;
  esac

  owner_declared=false
  for component_root in "${module_class_roots[@]}" "${block_class_roots[@]}"; do
    if [[ "$component_root" == "$owner_root" ]]; then
      owner_declared=true
      break
    fi
  done
  if [[ "$owner_declared" != true ]]; then
    echo "boundary contract violation: state-transition owner is not a declared Module/Block: $owner_root" >&2
    exit 1
  fi
  if [[ "$internal_event_file" != "$owner_root/biz/"* || "$dispatcher_file" != "$owner_root/biz/"* ]]; then
    echo "boundary contract violation: transition $transition_id escapes its owner-local biz boundary" >&2
    exit 1
  fi
  if [[ ! -f "$internal_event_file" || ! -f "$dispatcher_file" ]]; then
    echo "boundary contract violation: transition $transition_id references a missing internal-event or dispatcher file" >&2
    exit 1
  fi
  case "${internal_event_file##*/}" in
    *_internal_event.go|*_internal_events.go|internal_event.go) ;;
    *)
      echo "boundary contract violation: transition $transition_id does not use the owner-private internal-event file convention" >&2
      exit 1
      ;;
  esac
  if ! grep -q -E -- "^func (\([^)]*\) )?${dispatcher_symbol}\(" "$dispatcher_file"; then
    echo "boundary contract violation: transition $transition_id dispatcher is missing: $dispatcher_file:$dispatcher_symbol" >&2
    exit 1
  fi
  declared_internal_event_files["$internal_event_file"]=1
  case "${dispatcher_file##*/}" in
    *_internal_event.go|*_internal_events.go|internal_event.go)
      declared_internal_event_files["$dispatcher_file"]=1
      ;;
  esac

  if [[ "$classification" == "formal_runtime_machine" ]]; then
    case "$transition_id" in
      sm_1) expected_owner="internal/modules/blocks/run" ;;
      sm_2|sm_3|sm_4) expected_owner="internal/modules/kernel/orchestrator" ;;
      *)
        echo "boundary contract violation: unknown formal Runtime machine in manifest: $transition_id" >&2
        exit 1
        ;;
    esac
    if [[ "$owner_root" != "$expected_owner" ]]; then
      echo "boundary contract violation: $transition_id owner is $owner_root, want $expected_owner" >&2
      exit 1
    fi
    formal_runtime_machine_owners["$transition_id"]="$owner_root"
  fi
done < "$state_transition_owner_manifest"

# The transition-file inventory above catches private dispatcher additions.
# The AST inventory complements it by exhaustively classifying every named
# string State/Status/Phase/Disposition and every bare state-like string field
# in Block/Kernel owner pkg/models. Explicit immutable outcome/trace and derived
# projection fields remain non-authoritative; transition values must use a named
# state type and therefore cannot bypass the owner inventory.
go run ./internal/tools/stateownercheck \
  -root . \
  -inventory "$state_model_owner_manifest" \
  -transitions "$state_transition_owner_manifest"

for transition_id in sm_1 sm_2 sm_3 sm_4; do
  if [[ -z "${formal_runtime_machine_owners[$transition_id]+x}" ]]; then
    echo "boundary contract violation: formal Runtime machine is absent from owner manifest: $transition_id" >&2
    exit 1
  fi
done

# run.replacement is a Run-owner-private subordinate transaction of SM-1. It
# may coordinate clarification, termination handoff acknowledgement and
# successor materialization, but it is not a fifth state machine or a second
# owner of Run lifecycle.
if [[ "${state_transition_classifications[run.replacement]-}" != "owner_private_transaction" ||
      "${state_transition_owner_roots[run.replacement]-}" != "${formal_runtime_machine_owners[sm_1]-}" ]]; then
  echo "boundary contract violation: run.replacement must be an SM-1-owner-private subordinate transaction" >&2
  exit 1
fi

while IFS= read -r internal_event_file; do
  if [[ -z "${declared_internal_event_files[$internal_event_file]+x}" ]]; then
    cat >&2 <<EOF
Boundary contract violation: owner-private internal-event file is not present
in the sole state-transition owner manifest.

$internal_event_file

Classify the owner and dispatcher in $state_transition_owner_manifest. Do not
introduce a second production registry or a cross-owner reducer call.
EOF
    exit 1
  fi
done < <(find internal/modules -type f ! -name '*_test.go' \( -name '*_internal_event.go' -o -name '*_internal_events.go' -o -name 'internal_event.go' \) | sort)

# Targeted ingress regressions complement the manifest without defining a
# second owner inventory. These call sites historically bypassed, or are the
# only public translators into, their private dispatcher.
owner_secondary_ingress_dispatcher_assertions=(
  "internal/modules/blocks/workspace/biz/goal_dod_manager.go:submitWorkspaceCreationInternalEvent"
  "internal/modules/blocks/workspace/biz/definition_admission.go:submitDefinitionAdmissionInternalEvent"
  "internal/modules/blocks/run/biz/workspace_execution_intent.go:submitRunCreationInternalEvent"
  "internal/modules/blocks/run/biz/workspace_execution_intent.go:submitReplacementInternalEvent"
  "internal/modules/blocks/decision/biz/decision_service.go:applyLifecycleInternalEvent"
  "internal/modules/blocks/decision/biz/definition_clarification.go:applyLifecycleInternalEvent"
  "internal/modules/blocks/approval/biz/approval_service.go:applyLifecycleInternalEvent"
  "internal/modules/kernel/governance/biz/worker_reconciliation.go:persistEvaluatedGovernanceTrace"
  "internal/modules/kernel/governance/biz/worker_reconciliation.go:escalateGovernanceTrace"
  "internal/modules/kernel/governance/biz/worker_reconciliation.go:startGovernanceTraceResolution"
  "internal/modules/kernel/governance/biz/worker_reconciliation.go:blockGovernanceTrace"
  "internal/modules/kernel/governance/biz/worker_reconciliation.go:resolveGovernanceTrace"
  "internal/modules/kernel/orchestrator/biz/runtime_candidate_request_content.go:submitCandidateContentInternalEvent"
  "internal/modules/application/workorchd/biz/appservice/workspaceviewcore/store_mutation.go:submitProjectionLifecycleInternalEventLocked"
)
for entry in "${owner_secondary_ingress_dispatcher_assertions[@]}"; do
  path="${entry%%:*}"
  dispatcher="${entry#*:}"
  if ! grep -q -F -- "$dispatcher" "$path"; then
    echo "boundary contract violation: owner ingress does not use private dispatcher $dispatcher: $path" >&2
    exit 1
  fi
done

# Provider feedback category/class vocabulary belongs to LLM. Orchestrator may
# validate sealed Runtime scope, but must not maintain a second factual mapping.
if ! grep -q -F -- 'ValidateRuntimeProviderRequestFeedback(feedback)' internal/modules/kernel/orchestrator/biz/runtime_candidate_event_adapter.go ||
   ! grep -q -F -- 'RuntimeCandidateFailureClassProviderProtocol' internal/modules/blocks/llm/pkg/events/messages.go; then
  echo "boundary contract violation: Runtime provider feedback vocabulary is not owned exhaustively by LLM" >&2
  exit 1
fi

legacy_dod_resource_state_matches="$(source_matches 'DoDState(Draft|Satisfied)' internal || true)"
if [[ -n "$legacy_dod_resource_state_matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: Workspace DoD definitions only have the ready
resource state. Per-Run satisfied/unsatisfied assessment belongs to the
Orchestrator closeout ledger and must not become a second DoD state machine.

$legacy_dod_resource_state_matches
EOF
  exit 1
fi

if ! grep -q -F -- 'submitEffectInternalEvent' internal/modules/blocks/file/biz/owner.go ||
   source_matches 'runtimeAdapter\.ApplyPatch\(' internal/modules/blocks/file/biz/owner.go | grep -q .; then
  echo "boundary contract violation: File EventHub handler bypasses the owner-private effect dispatcher" >&2
  exit 1
fi

# Lifecycle transport names must describe immutable facts, not an instruction
# to apply a caller-selected state. Runtime aggregate persistence likewise uses
# the narrow opaque record command rather than a generic Run update surface.
legacy_lifecycle_contract_matches="$(source_matches 'ApplyRuntime(GovernanceWait|ContinuationEligible|TerminalDelivery|StateIncompatible)|ApplyRunProjectionFact|ReleaseFreshActivationRunSlotFact|Topic(Run|Workspace|Result)CommandApply|TopicRunCommandUpdate|UpdateRunCommand' internal || true)"
if [[ -n "$legacy_lifecycle_contract_matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: legacy apply/update lifecycle contract returned.
Use immutable owner facts and StoreRuntimeStateRecordCommand without aliases,
dual topics, or forwarding bridges.

$legacy_lifecycle_contract_matches
EOF
  exit 1
fi

# Schema v8 retains one output-token authority: the LLM-owner ProviderCeiling.
# Configuration, activation, policy and execution contracts must not regain
# stage-specific token fields or a selector mode.
legacy_stage_token_authority_matches="$(source_matches 'OutputTokenBudgetMode|semanticPlanMaxOutputTokens|candidateMaxOutputTokens|SemanticPlanMaxOutputTokens|CandidateMaxOutputTokens' internal application config web || true)"
if [[ -n "$legacy_stage_token_authority_matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: schema-v8 production code/config reintroduced
stage-specific output-token authority. Use persisted ProviderCeiling and the
sealed ExpectedDocument value.

$legacy_stage_token_authority_matches
EOF
  exit 1
fi

# Approval lifecycle input is an observed fact kind. A caller-selected target
# state or direct target assignment would bypass the owner-private reducer.
approval_target_state_matches="$({
  source_matches 'Target[[:space:]]+common\.ApprovalState' internal/modules/blocks/approval
  source_matches 'next\.State[[:space:]]*=[[:space:]]*(event|command|data)\.Target' internal/modules/blocks/approval
} | sed '/^[[:space:]]*$/d')"
if [[ -n "$approval_target_state_matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: Approval lifecycle input carries a receiver-owner
target state. Translate granted/rejected observations into an owner-private
fact kind and let the reducer select the state.

$approval_target_state_matches
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

matches="$(source_matches '^type Event(CommandClass|LanePolicy|HubLaneScope)[[:space:]]*=|Event(CommandClass|LanePolicy)(Read|Write|Execute|Notify|Facade|Topic|Query|State|Global)' internal/pkg/events)"
if [[ -n "$matches" ]]; then
	cat >&2 <<EOF
Boundary contract violation: internal/pkg/events recreates eventcontract facade aliases.

$matches

Callers must use eventcontract types and constants directly; internal/pkg/events
may retain only owner-neutral stateless EventHub helpers.
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
      xargs -0 -r grep -n -E -- 'map\[string\]any|\[\]any|\bany\b|<-chan|\bchan\b|internal/pkg/repository|io\.Writer|http\.ResponseWriter|\*.*(Repository|Registry)|Recorder' || true
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

# Strong cross-owner commands are defined by the receiving state/resource
# owner. Decision and Approval publish immutable governance facts into
# Orchestrator; Approval asks Eventstream to persist its resolution audit.
# None of those senders may retain a same-named command/topic alias.
orchestrator_event_topics="internal/modules/kernel/orchestrator/pkg/events/topics.go"
orchestrator_event_messages="internal/modules/kernel/orchestrator/pkg/events/messages.go"
eventstream_event_topics="internal/modules/blocks/eventstream/pkg/events/topics.go"
eventstream_event_messages="internal/modules/blocks/eventstream/pkg/events/messages.go"
if ! grep -q -E -- 'TopicOrchestratorCommandRecordDecisionEvidence[[:space:]]*=[[:space:]]*"/orchestrator/command/record-decision-evidence"' "$orchestrator_event_topics" ||
   ! grep -q -E -- 'TopicOrchestratorCommandRecordApprovalEvidence[[:space:]]*=[[:space:]]*"/orchestrator/command/record-approval-evidence"' "$orchestrator_event_topics" ||
   ! grep -q -F -- 'CommandTopic[RecordDecisionEvidenceCommand, RecordDecisionEvidenceResult]("orchestrator"' "$orchestrator_event_messages" ||
   ! grep -q -F -- 'CommandTopic[RecordApprovalEvidenceCommand, RecordApprovalEvidenceResult]("orchestrator"' "$orchestrator_event_messages" ||
   ! grep -q -F -- 'TopicOrchestratorCommandRecordDecisionEvidence, m.handleDecisionEvidenceCommand' internal/modules/kernel/orchestrator/biz/owner.go ||
   ! grep -q -F -- 'TopicOrchestratorCommandRecordApprovalEvidence, m.handleApprovalEvidenceCommand' internal/modules/kernel/orchestrator/biz/owner.go; then
  echo "boundary contract violation: governance evidence commands are not fully defined and consumed by Orchestrator" >&2
  exit 1
fi
if ! grep -q -E -- 'TopicEventStreamCommandRecordApprovalResolution[[:space:]]*=[[:space:]]*"/eventstream/command/record-approval-resolution"' "$eventstream_event_topics" ||
   ! grep -q -F -- 'CommandTopic[RecordApprovalResolutionCommand, RecordApprovalResolutionResult]("eventstream"' "$eventstream_event_messages" ||
   ! grep -q -F -- 'eventstreamevents.TopicEventStreamCommandRecordApprovalResolution,' internal/modules/blocks/eventstream/biz/event_stream_service.go ||
   ! grep -q -F -- 'ev.Source() != approvalcommon.ModuleID' internal/modules/blocks/eventstream/biz/event_stream_service.go; then
  echo "boundary contract violation: approval-resolution audit command is not fully owned and source-validated by Eventstream" >&2
  exit 1
fi
legacy_sender_owned_command_contracts="$({
  source_matches 'TopicDecisionCommandRecordEvidence|type RecordDecisionEvidence(Command|Result)|/decision/command/record-evidence' \
    internal/modules/blocks/decision/pkg/events
  source_matches 'TopicApprovalCommandRecord(Evidence|Resolution)|type RecordApproval(Evidence|Resolution)(Command|Result)|/(approval)/command/record-(evidence|resolution)' \
    internal/modules/blocks/approval/pkg/events
} | sed '/^[[:space:]]*$/d')"
if [[ -n "$legacy_sender_owned_command_contracts" ]]; then
  cat >&2 <<EOF
Boundary contract violation: a governance sender regained a command contract
owned by the receiving Orchestrator or Eventstream component.

$legacy_sender_owned_command_contracts
EOF
  exit 1
fi
if ! grep -q -F -- 'orchestratorevents.TopicOrchestratorCommandRecordDecisionEvidence' internal/modules/blocks/decision/biz/evidence_event.go ||
   ! grep -q -F -- 'orchestratorevents.TopicOrchestratorCommandRecordApprovalEvidence' internal/modules/blocks/approval/biz/evidence_event.go ||
   ! grep -q -F -- 'eventstreamevents.TopicEventStreamCommandRecordApprovalResolution' internal/modules/blocks/approval/biz/approval_notification.go ||
   ! grep -q -F -- 'TestGovernanceEvidenceCommandsAreOrchestratorOwned' internal/modules/kernel/orchestrator/pkg/events/governance_evidence_messages_test.go ||
   ! grep -q -F -- 'TestApprovalResolutionRecordCommandIsEventstreamOwned' internal/modules/blocks/eventstream/pkg/events/messages_test.go; then
  echo "boundary contract violation: receiver-owned governance event wiring or regressions are incomplete" >&2
  exit 1
fi

# LLM usage is an owner-authored notification, not an Eventstream command.
# Keep its unique event ID in the publishing owner namespace while Eventstream
# remains only a subscriber/audit sink.
if ! grep -q -E -- 'TopicLLMUsageRecorded[[:space:]]*=[[:space:]]*"/llm/usage/recorded"' internal/modules/blocks/llm/pkg/events/topics.go ||
   ! grep -q -F -- 'llmevents.TopicLLMUsageRecorded' internal/modules/blocks/eventstream/biz/event_stream_service.go ||
   ! grep -q -F -- 'TestLLMUsageRecordedTopicUsesOwnerNamespace' internal/modules/blocks/llm/pkg/events/runtime_provider_test.go ||
   grep -R -q -E --include='*.go' --exclude='*_test.go' -- 'TopicLLMUsageRecorded[[:space:]]*=[[:space:]]*"/llm/usage"' internal; then
  echo "boundary contract violation: LLM usage notification lost its unique owner-namespaced topic" >&2
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

# Deployed verifier providers are executable acceptance contracts. Keep every
# literal operation-producing branch under scripts/ on the same strict wire as
# production: providers propose semantics only; Orchestrator derives identity,
# dependency edges and coverage credit from the sealed manifest.
python3 scripts/verify_runtime_candidate_wire_scripts.py --self-test

# SM-3 owns the persisted candidate correction disposition/terminal state.
# The LLM owner receives only a detached, manifest-bound witness needed to
# render C4; neither its EventHub command/model nor C4 may mirror owner state,
# and the adapter must not rebuild a persisted directive from that input.
llm_candidate_command_shapes="$(sed -n \
	-e '/^type RequestRuntimeCandidateCommand struct {/,/^}/p' \
	-e '/^type PreflightRuntimeCandidateCommand struct {/,/^}/p' \
	"$llm_contract_root/messages.go")"
llm_candidate_correction_input_shape="$(sed -n '/^type RuntimeCandidateCorrectionInput struct {/,/^}/p' internal/modules/blocks/llm/pkg/models/runtime_candidate.go)"
for marker in \
	'CandidateCorrection[[:space:]]+\*llmmodel.RuntimeCandidateCorrectionInput' \
	'ManifestHash[[:space:]]+string' \
	'Objectives[[:space:]]+\[\]runtimecontract.RuntimeCandidateCorrectionObjective' \
	'Witness[[:space:]]+runtimecontract.RuntimeCandidateCorrectionWitness'; do
	if ! grep -q -E -- "$marker" <<<"$llm_candidate_command_shapes"$'\n'"$llm_candidate_correction_input_shape"; then
		echo "boundary contract violation: detached LLM candidate correction input lost $marker" >&2
		exit 1
	fi
done
llm_candidate_correction_authority_shapes="$({
	printf '%s\n' "$llm_candidate_command_shapes"
	printf '%s\n' "$llm_candidate_correction_input_shape"
	cat internal/modules/blocks/llm/biz/runtime_candidate_contract.go
})"
if grep -q -E -- '^[[:space:]]*(Disposition|TerminalCode)[[:space:]]|RuntimeCandidateCorrectionDirective' <<<"$llm_candidate_correction_authority_shapes"; then
	echo "boundary contract violation: LLM command/model/C4 regained candidate correction disposition, terminal state or persisted directive" >&2
	exit 1
fi
if grep -q -E -- 'runtimeCandidateCorrectionDirectiveFromInput|RuntimeCandidateCorrectionDirective[[:space:]]*\{' internal/modules/kernel/orchestrator/biz/runtime_candidate_event_adapter.go; then
	echo "boundary contract violation: Orchestrator adapter rebuilds persisted candidate correction authority from detached LLM input" >&2
	exit 1
fi

# Orchestrator must shrink persisted candidate/semantic failures before the
# EventHub call. Commands and the LLM owner model consume only the shared
# bounded observations; audit Reason/OriginalMessage and complete owner
# diagnostics may not be reconstructed or filtered after crossing.
llm_candidate_model_request_shape="$(sed -n '/^type RuntimeCandidateRequest struct {/,/^}/p' internal/modules/blocks/llm/pkg/models/runtime_candidate.go)"
llm_candidate_observation_shape="$(sed -n '/^type RuntimeCandidateCorrectionObservation struct {/,/^}/p' internal/pkg/workorchruntimecontract/runtime_correction_observation.go)"
llm_semantic_observation_shape="$(sed -n '/^type RuntimeSemanticCorrectionObservation struct {/,/^}/p' internal/pkg/workorchruntimecontract/runtime_correction_observation.go)"
for observation_request_shape in "$llm_candidate_command_shapes" "$llm_candidate_model_request_shape"; do
	for marker in \
		'CandidateCorrectionObservation[[:space:]]+\*runtimecontract\.RuntimeCandidateCorrectionObservation' \
		'SemanticCorrectionObservation[[:space:]]+\*runtimecontract\.RuntimeSemanticCorrectionObservation'; do
		if ! grep -q -E -- "$marker" <<<"$observation_request_shape"; then
			echo "boundary contract violation: Runtime LLM request lost bounded correction observation $marker" >&2
			exit 1
		fi
done
	if grep -q -E -- '\*runtimecontract\.(RuntimeCandidateFailure|RuntimeOriginalDiagnostic)|(^|[[:space:]])(PreviousFailure|PreviousSemanticRejection|Reason|OriginalMessage)[[:space:]]+' <<<"$observation_request_shape"; then
		echo "boundary contract violation: Runtime LLM request regained complete failure/audit diagnostic crossing" >&2
		exit 1
	fi
done
if grep -q -E -- '(^|[[:space:]])(Reason|ValidationDiagnostic|OriginalMessage|DetailsRef|DetailsHash|DetailsBytes|ContentType|OmittedReason|Truncated|Redacted|ValidationFailureObservation|GovernanceDependency)[[:space:]]+' <<<"$llm_candidate_observation_shape"$'\n'"$llm_semantic_observation_shape"; then
	echo "boundary contract violation: bounded correction observation regained owner-private audit/raw/validation/governance fields" >&2
	exit 1
fi

# The owner-neutral correction frontier may inspect only manifest-sealed typed
# semantic roles. Capability parameter/wire spellings stay centralized in the
# operation contract and must never become Orchestrator branching rules.
candidate_correction_frontier='internal/modules/kernel/orchestrator/biz/runtime/candidate_correction_frontier.go'
for marker in \
	'RuntimeOperationCorrectionFieldForRole' \
	'RuntimeOperationCorrectionFieldsForRole' \
	'RuntimeOperationKindCorrectionField' \
	'RuntimeOperationVariantCorrectionField'; do
	if ! grep -q -F -- "$marker" "$candidate_correction_frontier"; then
		echo "boundary contract violation: candidate correction frontier lost typed operation metadata API $marker" >&2
		exit 1
	fi
done
if grep -q -E -- 'recipeId|"path"|targetRef' "$candidate_correction_frontier"; then
	echo "boundary contract violation: candidate correction frontier infers capability semantics from parameter or wire-field spelling" >&2
	exit 1
fi
if grep -q -E -- 'TargetMirror|target_mirror|Retarget|retarget' \
	"$candidate_correction_frontier" internal/pkg/workorchruntimecontract/runtime_operation_correction.go; then
	echo "boundary contract violation: candidate correction contract regained an unused target-mirror/retarget authority role" >&2
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
   ! grep -q -F -- 'func (m *Biz) handleRuntimeGovernanceRequest' internal/modules/blocks/decision/biz/runtime_governance_adapter.go ||
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
for owner in decision file git llm mcp shell ssh web; do
  capability_contract="internal/modules/blocks/$owner/pkg/events/runtime_capability.go"
  if ! grep -q -E -- '^type AcquireRuntimeCapabilitiesCommand runtimecontract\.RuntimeCapabilityAcquisitionRequest$' "$capability_contract"; then
    echo "boundary contract violation: $owner capability command must use the canonical owner-neutral request value" >&2
    exit 1
  fi
done
for owner in file git mcp shell ssh web; do
  feasibility_contract="internal/modules/blocks/$owner/pkg/events/runtime_admissibility.go"
  if ! grep -q -E -- '^type AcquireRuntimeOperationAdmissibilityCommand runtimecontract\.RuntimeOperationAdmissibilityRequest$' "$feasibility_contract" ||
     ! grep -q -E -- '^type ProbeRuntimeOperationFeasibilityCommand runtimecontract\.RuntimeOperationFeasibilityRequest$' "$feasibility_contract" ||
     ! grep -q -E -- '^type ProbeRuntimeSemanticProofFeasibilityCommand runtimecontract\.RuntimeSemanticProofFeasibilityRequest$' "$feasibility_contract" ||
     ! grep -q -E -- '^type ProbeRuntimeSemanticProofFeasibilityResult runtimecontract\.RuntimeSemanticProofFeasibilityObservation$' "$feasibility_contract"; then
    echo "boundary contract violation: $owner Runtime owner commands/results must use canonical owner-neutral values" >&2
    exit 1
  fi
  feasibility_shape="$(grep -E -- '^type ProbeRuntimeOperationFeasibilityResult runtimecontract\.RuntimeOperationFeasibilityObservation$' "$feasibility_contract" || true)"
  if [[ -z "$feasibility_shape" ]] ||
     grep -q -E -- 'Feasible[[:space:]]+bool|Failure[[:space:]]+\*runtimecontract\.FailureFact|Category[[:space:]]+runtimecontract\.RuntimeOwnerFeasibilityCategory' <<<"$feasibility_shape"; then
    echo "boundary contract violation: $owner operation feasibility must use the canonical observation without parallel Feasible/Failure/Category authority" >&2
    exit 1
  fi
done
for owner in file git mcp shell; do
  reconciliation_contract="internal/modules/blocks/$owner/pkg/events/runtime_reconciliation.go"
  if ! grep -q -E -- '^type QueryRuntimeCWIReconciliationCommand runtimecontract\.RuntimeCWIOwnerReconciliationRequest$' "$reconciliation_contract"; then
    echo "boundary contract violation: $owner CWI reconciliation command must use the canonical owner-neutral request value" >&2
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

# File semantic and operation probes must execute the same owner-private target
# policy. Separate existence/create-only branches recreate the deployed v7
# contradiction where SM-2 accepted an operation that SM-3 always rejected.
file_policy="internal/modules/blocks/file/biz/file_owner_feasibility_policy.go"
if [[ ! -f "$file_policy" ]] ||
   ! grep -q -F -- 'evaluateFileOwnerFeasibilityPolicy' internal/modules/blocks/file/biz/owner.go ||
   ! grep -q -F -- 'evaluateFileOwnerFeasibilityPolicy' internal/modules/blocks/file/biz/file_runtime_feasibility.go; then
  echo "boundary contract violation: File semantic and operation feasibility do not share one owner policy" >&2
  exit 1
fi

# Equivalent-authority identity excludes request/lease/CAS bookkeeping and is
# persisted in both snapshot and candidate transaction. Attempt 0 must consult
# historical owner infeasibility before opening another provider session.
authority_snapshot_shape="$(sed -n '/^type AuthoritySnapshot struct {/,/^}/p' internal/pkg/workorchruntimecontract/canonical_runtime.go)"
authority_transaction_shape="$(sed -n '/^type AuthorityTransaction struct {/,/^}/p' internal/pkg/workorchruntimecontract/canonical_runtime.go)"
if ! grep -q -E -- 'AuthorityInputID[[:space:]]+string' <<<"$authority_snapshot_shape" ||
   ! grep -q -E -- 'AuthorityInput[[:space:]]+RuntimeEquivalentAuthorityInputIdentity' <<<"$authority_snapshot_shape" ||
   ! grep -q -E -- 'AuthorityInputID[[:space:]]+string' <<<"$authority_transaction_shape" ||
   ! grep -q -F -- 'equivalentOwnerInfeasibilityExists' internal/modules/kernel/orchestrator/biz/runtime/candidate_dispatch_transition.go; then
  echo "boundary contract violation: equivalent Runtime authority can bypass the no-progress fence" >&2
  exit 1
fi

# Runtime persistence has one current shape and no version negotiation,
# migration classifier, activation-status mirror, or old-data owner handoff.
if ! grep -q -F -- 'TestRuntimePersistenceRejectsRetiredAndUnknownFields' internal/pkg/workorchruntimecontract/runtime_state_schema_test.go ||
	! grep -q -F -- 'TestRuntimePolicyNormalizationDoesNotBackfillMissingPersistedFields' internal/pkg/workorchruntimecontract/runtime_state_schema_test.go ||
   ! grep -q -F -- 'TestRunPersistenceRejectsRetiredAndUnknownFields' internal/modules/blocks/run/pkg/models/replacement_intent_test.go ||
   ! grep -q -F -- 'TestGoalDoDManagerRejectsMissingLineageInsteadOfUpgrading' internal/modules/blocks/workspace/biz/goal_dod_manager_test.go ||
   ! grep -q -F -- 'TestReduceTerminalDeliveryRejectsMissingRunLineageInsteadOfUpgrading' internal/modules/blocks/run/biz/lifecycle_reducer_test.go ||
   ! grep -q -F -- 'TestPersistedRejectsRetiredGitCapabilityFlags' internal/pkg/workorchconfig/config_test.go; then
	echo "boundary contract violation: strict current Runtime/Run persistence regression is missing" >&2
	exit 1
fi
runtime_policy_backfill_matches="$({
	grep -n -E -- 'defaults[[:space:]]*:=[[:space:]]*DefaultRuntimeRecoveryPolicy|Max(ProviderTransport|OperationRecovery|CASReload|TerminalDelivery|DriveSchedule|PrerequisiteRefresh)[A-Za-z]*[[:space:]]*=[[:space:]]*defaults\.' internal/pkg/workorchruntimecontract/runtime_recovery_budget.go || true
	grep -n -E -- 'Max(RepairAttemptsPerFocus|ValidationOnlyRetriesPerFocus)[[:space:]]*=[[:space:]]*Default' internal/pkg/workorchruntimecontract/runtime_repair_ledger.go || true
	grep -n -F -- 'if strings.TrimSpace(b.ExecutionBudgetPolicy.PolicyRevision) == ""' internal/pkg/workorchruntimecontract/runtime_bootstrap.go || true
	grep -n -F -- 'if strings.TrimSpace(f.ExecutionBudgetPolicy.PolicyRevision) == ""' internal/pkg/workorchruntimecontract/canonical_runtime.go || true
	grep -n -E -- 'if c\.StateChangePolicy == ""|if p\.(CompletionPolicy|ResultPolicy) == ""|if s\.ProofApplicability == ""' internal/pkg/workorchruntimecontract/runtime_execution_contract.go internal/pkg/workorchruntimecontract/runtime_acceptance_slice.go internal/pkg/workorchruntimecontract/runtime_validation_surface.go || true
	grep -n -A2 -F -- 'value.OperationKind = strings.TrimSpace(value.OperationKind)' internal/pkg/workorchruntimecontract/runtime_activation.go | grep -F -- 'normalizeMCPRegistryKindToOperationKind' || true
})"
if [[ -n "$runtime_policy_backfill_matches" ]]; then
	echo "boundary contract violation: Runtime normalization backfills missing persisted policies" >&2
	echo "$runtime_policy_backfill_matches" >&2
	exit 1
fi
provider_capability_default_matches="$(source_matches 'func effectiveProviderCapabilities\(' internal/modules/blocks/llm)"
if [[ -n "$provider_capability_default_matches" ]] ||
	! grep -q -F -- 'TestRedactedProviderConfigDoesNotBackfillMissingCapabilities' internal/modules/blocks/llm/biz/provider_config_test.go; then
	echo "boundary contract violation: LLM provider capabilities regain an implicit empty-config default" >&2
	[[ -z "$provider_capability_default_matches" ]] || echo "$provider_capability_default_matches" >&2
	exit 1
fi
runtime_persistence_compatibility_matches="$(source_matches 'RuntimeStateSchemaVersionCurrent|RuntimeStateIncompatib|RuntimeActivation(Status|Disposition)|SchemaUnsupported|DataResetRequired|EquivalentReplacementIntentHash|legacyIdentityHashSlot|StepKindLLMWorkspacePlan|patchRequests|patch/request|file/write_file|GitRuntimeEnabled|GitRuntimeWriteEnabled' internal || true)"
if [[ -n "$runtime_persistence_compatibility_matches" ]]; then
	echo "boundary contract violation: Runtime/Run persisted-data compatibility surface returned" >&2
	echo "$runtime_persistence_compatibility_matches" >&2
	exit 1
fi
if grep -q -E -- 'LineageState[[:space:]]*==[[:space:]]*""' internal/modules/blocks/workspace/biz/goal_dod_manager.go internal/modules/blocks/run/biz/lifecycle_reducer.go; then
	echo "boundary contract violation: Workspace/Run owner silently upgrades a missing current lineage" >&2
	exit 1
fi

# Definition admission has one current immutable candidate shape and one
# public intent/fact ingress. Its pure compiler and transition-selecting result
# belong to the Workspace owner; Workorchd may consume that result and compose
# Decision-owner commands, but it cannot submit promotion/eligibility targets.
# Neither side may reseal caller/persisted bytes, and Restart must reject that
# shape before materializing managed files or sending an owner execution intent.
definition_candidate_hash_shape="$(sed -n '/^func (c DefinitionAdmissionCandidate) computeHash()/,/^}/p' internal/modules/blocks/workspace/pkg/models/definition_admission.go)"
for required_field in AdmissionPolicyBindings AcquisitionOperationVariantID AcquisitionTargetPolicy AcquisitionTargetSelectorRef; do
	if ! grep -q -F -- "$required_field" <<<"$definition_candidate_hash_shape"; then
		echo "boundary contract violation: Definition candidate hash omits $required_field" >&2
		exit 1
	fi
done
definition_compiler="internal/modules/blocks/workspace/pkg/definitionadmission/compiler.go"
definition_compiler_test="internal/modules/blocks/workspace/pkg/definitionadmission/compiler_test.go"
definition_intent_command_shape="$(sed -n '/^type DefinitionAdmissionIntentCommand struct {/,/^}/p' internal/modules/blocks/workspace/pkg/events/messages.go)"
definition_intent_result_shape="$(sed -n '/^type DefinitionAdmissionIntentResult struct {/,/^}/p' internal/modules/blocks/workspace/pkg/events/messages.go)"
definition_intent_reducer="$(sed -n '/^func reduceDefinitionAdmissionIntent(/,/^}/p' internal/modules/blocks/workspace/biz/definition_admission_internal_event.go)"
definition_legacy_target_commands="$(source_matches 'SubmitDefinitionAdmissionCandidate(Command|Result)|PromoteDefinitionAdmissionCandidate(Command|Result)|AcknowledgeWorkspaceDefinitionClarificationResolution(Command|Result)|TopicWorkspaceCommandSubmitDefinitionAdmissionCandidate|TopicWorkspaceCommandPromoteDefinitionAdmissionCandidate|TopicWorkspaceCommandAcknowledgeDefinitionClarification|/workspace/definition-admission/command/(submit-candidate|promote-candidate|acknowledge-clarification)' internal/modules || true)"
if [[ -f internal/modules/application/workorchd/biz/appservice/definitionadmissioncore/compiler.go ]] ||
	[[ ! -f "$definition_compiler" ]] ||
	! grep -q -E -- 'AdmissionResult[[:space:]]+workspacemodel\.DefinitionAdmissionResult' <<<"$definition_intent_result_shape" ||
	! grep -q -F -- 'ClarificationTicket' <<<"$definition_intent_command_shape" ||
	! grep -q -F -- 'ClarificationResolution' <<<"$definition_intent_command_shape" ||
	grep -q -E -- 'TargetState|RequestedState|PromotionState' <<<"$definition_intent_command_shape" ||
	[[ -n "$definition_legacy_target_commands" ]] ||
	! grep -q -F -- 'definitionadmission.Compile(candidate)' <<<"$definition_intent_reducer" ||
	grep -q -F -- 'candidate = candidate.WithHash()' "$definition_compiler" ||
	grep -q -F -- 'command.Candidate = command.Candidate.WithHash()' internal/modules/blocks/workspace/biz/definition_admission_internal_event.go ||
	! grep -q -F -- 'TestCompileRejectsCandidateHashDriftInsteadOfResealingCallerInput' "$definition_compiler_test" ||
	! grep -q -F -- 'TestDefinitionAdmissionOwnerDoesNotResealCallerCandidate' internal/modules/blocks/workspace/biz/definition_admission_test.go ||
	! grep -q -F -- 'TestDefinitionAdmissionIntentAutomaticallyPromotesEligibleCandidate' internal/modules/blocks/workspace/biz/definition_admission_test.go ||
	! grep -q -F -- 'TestDefinitionAdmissionIntentAloneSelectsTicketPromotionAndResolution' internal/modules/blocks/workspace/biz/definition_admission_test.go ||
	! grep -q -F -- 'TestWorkspaceExecutionRestartRequiresCurrentAdmissionCandidateBeforeRunOwner' internal/modules/blocks/workspace/biz/execution_intent_test.go; then
	echo "boundary contract violation: Definition admission escaped Workspace ownership, regained caller outcome/resealing, or lost strict Restart regressions" >&2
	exit 1
fi
restart_compiler_file="internal/modules/application/workorchd/biz/appservice/activationports/workspace_execution_intent.go"
restart_admission_line="$(grep -n -m1 -F -- 'EnsureWorkspaceDefinitionAdmission(' "$restart_compiler_file" | cut -d: -f1)"
restart_activation_line="$(grep -n -m1 -F -- 'workspaceRuntimeActivation(' "$restart_compiler_file" | cut -d: -f1)"
if [[ -z "$restart_admission_line" || -z "$restart_activation_line" || "$restart_admission_line" -ge "$restart_activation_line" ]] ||
	! grep -q -F -- 'workspacecore.EnsureManagedConfigs(' internal/modules/application/workorchd/biz/appservice/activationports/execution_context.go; then
	echo "boundary contract violation: Restart must validate Definition admission before managed-config materialization" >&2
	exit 1
fi

# Activation declarations and semantic-generation authority are distinct
# concrete types. RuntimeBootstrap/Facts retain immutable DefinitionFocuses;
# only SM-2 may create a CoverageFocus carrying target/operation/proof binding.
runtime_bootstrap_shape="$(sed -n '/^type RuntimeBootstrap struct {/,/^}/p' internal/pkg/workorchruntimecontract/runtime_bootstrap.go)"
runtime_focus_seed_shape="$(sed -n '/^func (f RuntimeDefinitionFocus) RuntimeFocusSeed()/,/^}/p' internal/pkg/workorchruntimecontract/runtime_definition_focus.go)"
if ! grep -q -E -- 'DefinitionFocuses[[:space:]]+\[\]RuntimeDefinitionFocus' <<<"$runtime_bootstrap_shape" ||
	grep -q -E -- '(^|[[:space:]])Focuses[[:space:]]+\[\]CoverageFocus' <<<"$runtime_bootstrap_shape" ||
	! grep -q -F -- 'RuntimeFocusSeedsFromDefinitionFocuses(b.DefinitionFocuses)' internal/pkg/workorchruntimecontract/runtime_bootstrap.go ||
	grep -q -E -- '(TargetRefs|TargetSelectorRef|TargetSelectorCandidateRefs|CoverageEvidenceBindings|Operations|ValidationRecipe|ProofBinding)' <<<"$runtime_focus_seed_shape" ||
	! grep -q -F -- 'TestRuntimeDefinitionFocusPreservesDeclarationWithoutGenerationAuthority' internal/pkg/workorchruntimecontract/runtime_bootstrap_test.go; then
	echo "boundary contract violation: activation DefinitionFocus and SM-2 generation Focus authority were merged" >&2
	exit 1
fi

# The target_grounding_unresolved view created from a binding-free focus during
# catalog retirement cannot cross SM-4. It remains owner-local until the
# current semantic generation is sealed or fails. Other sealed terminals stay
# authoritative. Startup recovery must apply the same convergence fence.
runtime_drive_transition_shape="$(sed -n '/^func (m \*Biz) driveOneRuntimeTransition(/,/^}/p' internal/modules/kernel/orchestrator/biz/runtime_drive.go)"
runtime_finish_transition_shape="$(sed -n '/^func finishRuntimeConvergenceTransition(/,/^}/p' internal/modules/kernel/orchestrator/biz/runtime_drive.go)"
runtime_startup_recovery_shape="$(sed -n '/^func (m \*Biz) recoverRuntimeStartup(/,/^}/p' internal/modules/kernel/orchestrator/biz/owner.go)"
if ! grep -q -F -- 'runtimeTerminalDeliveryReady(aggregate)' <<<"$runtime_drive_transition_shape" ||
	! grep -q -F -- 'runtimeTerminalDeliveryReady(aggregate)' <<<"$runtime_finish_transition_shape" ||
	! grep -q -F -- 'runtimeTerminalDeliveryReady(aggregate)' <<<"$runtime_startup_recovery_shape" ||
	! grep -q -F -- 'aggregate.Decision.Terminal.Code != "target_grounding_unresolved"' internal/modules/kernel/orchestrator/biz/runtime_drive.go ||
	! grep -q -F -- 'aggregate.Decision.Terminal.Code != "target_grounding_unresolved"' internal/modules/kernel/orchestrator/biz/runtime/candidate_request_session.go ||
	! grep -q -F -- 'TestRuntimeTerminalDeliveryWaitsForCurrentSemanticAuthority' internal/modules/kernel/orchestrator/biz/runtime_drive_result_test.go ||
	! grep -q -F -- 'TestSemanticRequestMayReplaceOnlyBindingFreeProvisionalTerminal' internal/modules/kernel/orchestrator/biz/runtime/candidate_request_session_test.go; then
	echo "boundary contract violation: target-grounding provisional terminal can bypass current semantic authority, or another sealed terminal can be reopened" >&2
	exit 1
fi

# File remains target-inventory owner. Orchestrator may forward only the
# persisted File catalog to Shell when reacquiring its validation catalog, and
# that bounded vocabulary must survive semantic-generation retirement.
runtime_shell_target_forwarding_shape="$(sed -n '/^func runtimeOperationAdmissibilityTargetRefs(/,/^}/p' internal/modules/kernel/orchestrator/biz/runtime_prerequisite_drive.go)"
if ! grep -q -F -- 'ownerModuleID != runtimecontract.RuntimeModuleShell' <<<"$runtime_shell_target_forwarding_shape" ||
	! grep -q -F -- 'aggregate.Facts.Resources.OperationAdmissibilities' <<<"$runtime_shell_target_forwarding_shape" ||
	grep -q -F -- 'aggregate.SemanticPlanning.Phase' <<<"$runtime_shell_target_forwarding_shape" ||
	! grep -q -F -- 'TestRuntimeOperationAdmissibilityTargetsExposeFileCatalogOnlyToShellCatalog' internal/modules/kernel/orchestrator/biz/runtime_mainline_semantic_test.go ||
	! grep -q -F -- 'TestDriveRuntimeFileBackedTwoFocusStateValidationAssessmentAndRestart' internal/modules/kernel/orchestrator/biz/runtime_drive_test.go; then
	echo "boundary contract violation: Shell validation catalog can lose or invent the File-owned target vocabulary" >&2
	exit 1
fi

# All file-backed owner data uses one current layout and strict JSON shape.
# A missing file may mean "not created"; an existing old/empty/unknown shape
# must fail instead of entering an import, timestamp synthesis or projection
# repair path.
if ! grep -q -F -- 'decoder.DisallowUnknownFields()' internal/pkg/repository/file/file.go ||
   ! grep -q -F -- 'TestWorkspaceRepositoryRejectsRetiredOrUnknownFields' internal/pkg/repository/file/file_test.go ||
   ! grep -q -F -- 'TestWorkspaceScopedFileStoreRejectsInvalidCurrentIndex' internal/pkg/workorchrepo/workspace_scoped_repositories_test.go ||
   ! grep -q -F -- 'TestCapabilityStateReceiptStoreRejectsUnknownPersistedFields' internal/modules/blocks/git/biz/capability_state_receipt_store_test.go; then
	echo "boundary contract violation: strict current file-persistence decoder/regression is missing" >&2
	exit 1
fi
repository_compatibility_matches="$(source_matches 'NewMCPRegistryRepositoryWithLegacy|loadLegacy|CapabilitySourceLegacyImport|SoulSourceLegacyOwned|legacy_owner_upgrade|fileModTimeTimestamp|FetchEndpoint|fetchEndpoint' internal || true)"
if [[ -n "$repository_compatibility_matches" ]]; then
	echo "boundary contract violation: persisted repository/config compatibility surface returned" >&2
	echo "$repository_compatibility_matches" >&2
	exit 1
fi
if grep -q -F -- 'AddHandler("/api/v1/workorch/memory/:memoryId"' internal/modules/application/workorchd/service/server/http_memory.go ||
	! grep -q -F -- 'TestWorkspaceScopedMemoryItemRoutesRejectUnscopedItemPaths' internal/modules/application/workorchd/service/server/http_test.go; then
	echo "boundary contract violation: unscoped Memory item compatibility route returned" >&2
	exit 1
fi
git_runtime_compatibility_matches="$(source_matches 'writeEnabled|NewRuntimeAdapter\((true|false)\)|NewRuntimeAdapterWithTimeout\((true|false)|NewRuntimeAdapterFromTimeoutString\((true|false)' internal/modules/blocks/git/biz internal/modules/application/workorchd internal/modules/kernel/orchestrator || true)"
if [[ -n "$git_runtime_compatibility_matches" ]]; then
	echo "boundary contract violation: Git Runtime adapter retained the retired enable/write-enable compatibility input" >&2
	echo "$git_runtime_compatibility_matches" >&2
	exit 1
fi
provider_failure_identity="$(sed -n '/^func providerFailureIdentity(/,/^}/p' internal/modules/blocks/llm/biz/runtime/candidate_boundary.go)"
if grep -q -E -- 'err\.Error\(|strings\.Contains\(' <<<"$provider_failure_identity" ||
	! grep -q -F -- 'RuntimeProviderFailureIdentity()' <<<"$provider_failure_identity" ||
	! grep -q -F -- 'TestProviderFailureIdentityDoesNotClassifyErrorText' internal/modules/blocks/llm/biz/runtime/provider_failure_identity_test.go; then
	echo "boundary contract violation: LLM provider failure identity regained an error-text compatibility classifier" >&2
	exit 1
fi
if grep -q -F -- 'ret.Payload = recordpayload.SanitizeAuditPayload(item.Payload)' internal/modules/blocks/eventstream/pkg/events/record_value.go ||
   ! grep -q -F -- 'TestEventRecordDataReadBoundaryRejectsUnsanitizedPayload' internal/modules/blocks/eventstream/pkg/events/record_value_test.go; then
	echo "boundary contract violation: Eventstream read boundary upgrades persisted payload instead of rejecting it" >&2
	exit 1
fi
# Run is the physical Runtime record owner, not the semantic owner. Lifecycle
# reduction may compare only its opaque lifecycle fence and envelope revision;
# decoding these aggregate fields would let SM-1 select transitions from
# SM-2/SM-3/SM-4 state.
run_runtime_decode_matches="$(source_matches 'RuntimeState\.(Facts|Decision|SemanticPlanning|Reconciliation|CandidateAttempts|ActiveDispatch|CompletedDispatches|GovernanceRequests|GovernanceResolutions|OperationReceipts|DeliveryAcknowledgements|DriveLease|Termination)' internal/modules/blocks/run/biz/lifecycle_reducer.go)"
if [[ -n "$run_runtime_decode_matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: Run lifecycle reducer decodes Orchestrator-owned
Runtime semantic state instead of comparing the opaque lifecycle fence.

$run_runtime_decode_matches
EOF
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

# File prospective-target admission is a post-binding, pre-effect create gate. Post-effect
# reconciliation must use a separate File-private observer; reusing admission
# turns the successfully created target into unsupported/unverifiable and can
# make SM-2 reopen the CWI for a second effect. The observer remains factual
# and must not acquire Orchestrator-private retain/reopen vocabulary.
file_reconciliation_observer="internal/modules/blocks/file/biz/file_runtime_reconciliation.go"
file_reconciliation_handler="$(sed -n '/^func (m \*Biz) handleRuntimeCWIReconciliationQuery(/,/^}/p' internal/modules/blocks/file/biz/owner.go)"
if [[ ! -f "$file_reconciliation_observer" ]] ||
   ! grep -q -F -- 'observeRuntimeReconciliationTargets' "$file_reconciliation_observer" ||
   ! grep -q -F -- 'observeRuntimeReconciliationTargets' <<<"$file_reconciliation_handler" ||
   grep -q -F -- 'AcquireRuntimeTargetSelection' <<<"$file_reconciliation_handler"; then
  echo "boundary contract violation: File post-effect reconciliation reuses prospective-target admission" >&2
  exit 1
fi
if source_matches 'RuntimeCWIReconciliationDisposition|"(retain|reopen)"' "$file_reconciliation_observer" | grep -q .; then
  echo "boundary contract violation: File reconciliation observer selects an Orchestrator-owned CWI disposition" >&2
  exit 1
fi

# The pre-effect unique selector is current-inventory cardinality only. A
# prospective path must be rejected before the generic manifest branch can
# preserve it as an absent/selected/ambiguous admission; otherwise post-effect
# reconciliation will correctly fail closed and SM-2 can be sent into a
# needless reopen loop.
file_target_admission="internal/modules/blocks/file/biz/file_runtime_target.go"
unique_target_admission="$(sed -n '/^func acquireUniqueWorkspaceEntryRuntimeTargetSelection(/,/^}/p' "$file_target_admission")"
if ! grep -q -F -- 'case runtimeFileTargetSelectorUniqueWorkspaceEntry:' "$file_target_admission" ||
   ! grep -q -F -- 'return acquireUniqueWorkspaceEntryRuntimeTargetSelection(request, manifest)' "$file_target_admission" ||
   ! grep -q -F -- 'request.prospectiveTargetRef != ""' <<<"$unique_target_admission" ||
   ! grep -q -F -- 'RuntimeTargetSelectionUnsupported' <<<"$unique_target_admission"; then
  echo "boundary contract violation: File unique-workspace selector can retain an unauthorized prospective target" >&2
  exit 1
fi

# A prospective File target must remain observable through the same bounded
# deliverable manifest after its effect. Admission may share the owner-private
# path eligibility predicate with reconciliation, but must not reuse the
# reconciliation decision path itself.
file_path_guard="internal/modules/blocks/file/biz/file_path_guard.go"
prospective_target_admission="$(sed -n '/^func acquireProspectiveRuntimeTargetSelection(/,/^}/p' "$file_target_admission")"
workspace_target_admission="$(sed -n '/^func acquireManifestRuntimeTargetSelection(/,/^}/p' "$file_target_admission")"
bounded_target_admission="$(sed -n '/^func acquireBoundedCandidateTargetSelection(/,/^}/p' "$file_target_admission")"
if ! grep -q -F -- 'func runtimeFileDeliverableTargetEligible(' "$file_path_guard" ||
   ! grep -q -F -- 'runtimeFileDeliverableTargetEligible' <<<"$prospective_target_admission" ||
   ! grep -q -F -- 'runtimeFileDeliverableTargetEligible' <<<"$workspace_target_admission" ||
   ! grep -q -F -- 'runtimeFileDeliverableTargetEligible' <<<"$bounded_target_admission" ||
   ! grep -q -F -- 'runtimeFileDeliverableTargetEligible' "$file_reconciliation_observer"; then
  echo "boundary contract violation: File prospective target can escape the deliverable manifest eligibility shared by factual reconciliation" >&2
  exit 1
fi
file_manifest_observation="$(sed -n '/^func (a \*RuntimeAdapter) acquireWorkspaceManifestObservation(/,/^}/p' internal/modules/blocks/file/biz/file_runtime_snapshot.go)"
file_capacity_guard="$(sed -n '/^func runtimeFileProspectiveTargetWouldOverflowManifest(/,/^}/p; /^func runtimeFileProspectiveTargetAdditionalEntries(/,/^}/p' "$file_target_admission")"
file_effect_capacity_guard="$(sed -n '/^func (a \*RuntimeAdapter) validateProspectivePatchManifestCapacity(/,/^}/p' internal/modules/blocks/file/biz/file_runtime_typed.go)"
if ! grep -q -F -- 'entryCount: len(result.Entries)' <<<"$file_manifest_observation" ||
	! grep -q -F -- 'runtimeFileProspectiveTargetAdditionalEntries' <<<"$file_capacity_guard" ||
	! grep -q -F -- 'entryCount+additional > defaultMaxEntries' <<<"$file_capacity_guard" ||
	! grep -q -F -- 'observation.entryCount+additional > defaultMaxEntries' <<<"$file_effect_capacity_guard"; then
	echo "boundary contract violation: File prospective capacity ignores directories or missing parent entries" >&2
	exit 1
fi
if ! grep -q -F -- 'TestRuntimeAdapterProspectiveAdmissionRejectsManifestExcludedTargetsWithoutMutation' internal/modules/blocks/file/biz/file_runtime_adapter_test.go ||
	! grep -q -F -- 'TestRuntimeAdapterProspectiveAdmissionDoesNotIgnoreNonParticipatingTarget' internal/modules/blocks/file/biz/file_runtime_adapter_test.go ||
	! grep -q -F -- 'TestRuntimeAdapterProspectiveAdmissionReservesManifestCapacity' internal/modules/blocks/file/biz/file_runtime_adapter_test.go ||
	! grep -q -F -- 'TestRuntimeAdapterCreateEffectRechecksAndSerializesManifestCapacity' internal/modules/blocks/file/biz/file_runtime_adapter_test.go ||
   ! grep -q -F -- 'TestRuntimeAdapterProfileProspectiveMustBelongToDeclaredCandidatesAcrossCardinality' internal/modules/blocks/file/biz/file_runtime_adapter_test.go ||
   ! grep -q -F -- 'TestRuntimeAdapterExistingProspectiveMustMatchCurrentInventory' internal/modules/blocks/file/biz/file_runtime_adapter_test.go ||
   ! grep -q -F -- 'TestRuntimeAdapterManifestFailsClosedOnIncompleteInventoryObservation' internal/modules/blocks/file/biz/file_runtime_adapter_test.go ||
   ! grep -q -F -- 'TestFileOwnerReconciliationKeepsManifestExcludedProspectiveTargetsUnverifiable' internal/modules/blocks/file/biz/owner_reconciliation_test.go ||
   ! grep -q -F -- 'TestRuntimeAdapterListAndManifestDistinguishExactBoundFromOverflow' internal/modules/blocks/file/biz/file_runtime_adapter_test.go; then
  echo "boundary contract violation: File deliverable eligibility or exact-bound regression coverage is missing" >&2
  exit 1
fi

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

# CWI-SO-14/15: final executable admission is one invocation-local pure
# contract and one Orchestrator-owned dispatcher. The admission input must not
# become an EventHub DTO or persisted aggregate, and production dispatch
# construction must not bypass admitRuntimeDispatch.
if [[ ! -f internal/pkg/workorchruntimecontract/runtime_complete_admission.go ]] ||
   ! grep -q -F -- 'func EvaluateRuntimeCompleteAdmission(' internal/pkg/workorchruntimecontract/runtime_complete_admission.go; then
  echo "boundary contract violation: pure Runtime complete-admission evaluator is missing" >&2
  exit 1
fi
runtime_complete_admission_consumers="$(
  grep -R -Hn -F --include='*.go' --exclude='*_test.go' -- 'RuntimeCompleteAdmissionInput' internal |
    grep -v -E -- 'internal/pkg/workorchruntimecontract/runtime_complete_admission\.go:|internal/modules/kernel/orchestrator/biz/runtime/candidate_dispatch_transition\.go:' || true
)"
if [[ -n "$runtime_complete_admission_consumers" ]]; then
  cat >&2 <<EOF
Boundary contract violation: Runtime complete-admission input escaped its pure
contract and sole Orchestrator assembler.

$runtime_complete_admission_consumers

Do not persist or publish this invocation-local detached view. Assemble it
only in the owner-local admission dispatcher from canonical aggregate facts.
EOF
  exit 1
fi
if ! grep -q -F -- 'func admitRuntimeDispatch(' internal/modules/kernel/orchestrator/biz/runtime/candidate_dispatch_transition.go ||
   ! grep -q -F -- 'EvaluateRuntimeCompleteAdmission(runtimecontract.RuntimeCompleteAdmissionInput{' internal/modules/kernel/orchestrator/biz/runtime/candidate_dispatch_transition.go; then
  echo "boundary contract violation: Orchestrator complete-admission dispatcher is missing" >&2
  exit 1
fi
runtime_dispatch_builder_bypasses="$(
  grep -R -Hn -F --include='*.go' --exclude='*_test.go' -- 'SealDispatchWithManifestAndFeasibilities(' internal/modules/kernel/orchestrator/biz/runtime |
    grep -v -E -- 'dispatch_commit\.go:.*func SealDispatchWithManifestAndFeasibilities|candidate_dispatch_transition\.go:' || true
)"
if [[ -n "$runtime_dispatch_builder_bypasses" ]]; then
  cat >&2 <<EOF
Boundary contract violation: production Runtime dispatch construction bypasses
the sole owner-local admission dispatcher.

$runtime_dispatch_builder_bypasses
EOF
  exit 1
fi

# Semantic providers select only sealed vocabulary. Artifact and metric proof
# authority remain declaration-owned and therefore must never be restored to
# the provider-authored proposal DTO.
semantic_proposal_shape="$(sed -n '/^type RuntimeSemanticProofProposal struct {/,/^}/p' internal/pkg/workorchruntimecontract/runtime_semantic_contract.go)"
if grep -q -E -- 'ArtifactRefs|MetricConstraints' <<<"$semantic_proposal_shape"; then
  echo "boundary contract violation: semantic provider proposal restored artifact or metric authority" >&2
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

# RuntimeController test facades must not compile into production. Direct
# component mutation is legal only inside the exhaustive owner-private event
# routes reached by ApplyInternalEvent; helpers elsewhere must submit a
# concrete RuntimeInternalEvent instead of selecting a component mutator.
if [[ -f "$runtime_transition_root/component_facades.go" ]]; then
	echo "boundary contract violation: RuntimeController component facade is compiled into production" >&2
	exit 1
fi
runtime_controller_component_mutator_facades="$(
  find "$runtime_transition_root" -type f -name '*.go' ! -name '*_test.go' ! -name 'internal_event_route_*.go' -print0 |
    xargs -0 -r grep -Hn -E -- '[A-Za-z_][A-Za-z0-9_]*\.(authority|planning|candidateDispatch|driveDelivery)\.[A-Za-z_][A-Za-z0-9_]*\(' || true
)"
if [[ -n "$runtime_controller_component_mutator_facades" ]]; then
  cat >&2 <<EOF
Boundary contract violation: a production RuntimeController facade calls a
private component mutator outside the owner-private internal-event routes.

$runtime_controller_component_mutator_facades

Submit a concrete RuntimeInternalEvent through ApplyInternalEvent. Direct
component dispatch remains legal only in internal_event_route_*.go.
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
  "internal/modules/blocks/llm/biz/runtime_candidate_contract.go:RuntimeConvergenceEffectInstruction"
  "internal/modules/blocks/llm/biz/runtime_candidate_schema.go:RuntimeConvergenceEffectInstruction"
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
if ! grep -q -F -- 'if aggregate.ActiveDispatch != nil {' "$runtime_transition_root/authority_transition.go" ||
   ! grep -q -F -- 'active dispatch failures require a dispatch/receipt-specific transition' "$runtime_transition_root/authority_transition.go"; then
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
run_update_body="$(struct_body internal/modules/blocks/run/pkg/events/messages.go StoreRuntimeStateRecordCommand)"
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

# Workspace pre-seal Definition clarifications are Decision-owned but have no
# Run scope. They must use their dedicated resolve/replay contract and must
# never be coerced into ordinary recovery or run-scoped Orchestrator evidence.
for decision_pre_seal_regression in \
	TestWorkspaceDefinitionClarificationOrphanResolutionWaitsForDedicatedReplay \
	TestWorkspacePreSealDefinitionResolutionSkipsRunScopedEvidenceAndReplaysAfterResponseLoss; do
	if ! grep -R -q -E --include='*_test.go' -- "^func ${decision_pre_seal_regression}\\(" internal; then
		echo "boundary contract violation: pre-seal Decision resolution regression is missing (${decision_pre_seal_regression})" >&2
		exit 1
	fi
done

forbidden_approval_update_fields='(^|[[:space:]])(State|ResolvedAt|ResolvedBy|CreatedAt)([[:space:]]|$)'
approval_update_body="$(struct_body internal/modules/blocks/approval/pkg/events/messages.go UpdateApprovalCommand)"
if grep -q -E -- "$forbidden_approval_update_fields" <<<"$approval_update_body"; then
	 echo "boundary contract violation: generic Approval update exposes lifecycle fields" >&2
	 exit 1
fi

result_terminal_body="$(struct_body internal/modules/blocks/result/pkg/events/lifecycle_messages.go RuntimeTerminalDeliveryFact)"
if grep -q -E -- '(^|[[:space:]])Result[[:space:]]+\*' <<<"$result_terminal_body"; then
  echo "boundary contract violation: Result terminal command carries a mutable Result model" >&2
  exit 1
fi

# Run, Result, and Workspace own their lifecycle wire contracts. Orchestrator
# may send those concrete contracts but must not redefine them, and lifecycle
# handlers must not depend on an Orchestrator-defined lifecycle command.
for owner in result; do
  matches="$(source_matches 'internal/modules/kernel/orchestrator/pkg/events' "internal/modules/blocks/$owner/biz")"
  if [[ -n "$matches" ]]; then
    echo "boundary contract violation: $owner lifecycle owner imports Orchestrator event contracts" >&2
    echo "$matches" >&2
    exit 1
  fi
done
# Workspace deletion is a distinct Workspace-owned state machine. Its sole
# coordinator is a legitimate typed sender of Orchestrator quiescence/purge
# commands; no other Workspace production file may import that target-owner
# package.
matches="$(source_matches 'internal/modules/kernel/orchestrator/pkg/events' internal/modules/blocks/workspace/biz | sed '/internal\/modules\/blocks\/workspace\/biz\/deletion_coordinator\.go:/d')"
if [[ -n "$matches" ]]; then
  echo "boundary contract violation: workspace lifecycle owner imports Orchestrator event contracts" >&2
  echo "$matches" >&2
  exit 1
fi
if ! grep -q -F -- 'TopicRunFactTerminalDelivery' internal/modules/blocks/run/pkg/events/topics.go ||
   ! grep -q -F -- 'TopicResultFactTerminalDelivery' internal/modules/blocks/result/pkg/events/topics.go ||
   ! grep -q -F -- 'TopicWorkspaceFactTerminalDelivery' internal/modules/blocks/workspace/pkg/events/topics.go ||
   ! grep -q -F -- 'runevents.TopicRunFactTerminalDelivery' internal/modules/kernel/orchestrator/biz/runtime_result_projection.go ||
   ! grep -q -F -- 'resultevents.TopicResultFactTerminalDelivery' internal/modules/kernel/orchestrator/biz/runtime_result_projection.go ||
   ! grep -q -F -- 'workspaceevents.TopicWorkspaceFactTerminalDelivery' internal/modules/kernel/orchestrator/biz/runtime_result_projection.go; then
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
# convergence state and exact owner blocker as display-only data. Navigation is
# kept separate from owner-required actions and cannot be used as a drive input.
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

Project Runtime only as canonical convergenceState plus the exact factual
blocker. A display navigation hint must never be used to select, resume,
ticket, or drive Runtime work.
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
  internal/pkg/workorchprojectioncontract/runtime_audit.go; then
  cat >&2 <<EOF
Boundary contract violation: Runtime audit projection drops structured owner feedback.

Map RuntimeOperationReceipt.Feedback as bounded stable category and original
diagnostic metadata. Do not replace it with a message-only summary or raw
provider/tool payload.
EOF
  exit 1
fi
audit_receipt_shape="$(sed -n '/^type RuntimeAuditOperationReceipt struct {/,/^}/p' internal/pkg/workorchprojectioncontract/runtime_audit.go)"
if ! grep -q -E -- 'OriginalDiagnostic[[:space:]]+\*runtimecontract\.RuntimeOriginalDiagnostic' <<<"$audit_receipt_shape" ||
   grep -q -E -- '(OwnerModuleID|OriginalClass|OriginalCode|OriginalMessage|Signature|ExpectedRevision|ObservedRevision|DetailsRef|DetailsHash|ContentType|Truncated|Redacted)[[:space:]]+' <<<"$audit_receipt_shape"; then
  echo "boundary contract violation: operation receipt audit must carry one nested canonical original diagnostic without flattened duplicates" >&2
  exit 1
fi

if ! grep -q -E -- 'Snapshot\(.*\*projectioncontract\.WorkspaceSnapshot' internal/pkg/workorchport/workspace.go ||
	 ! grep -q -E -- 'Audit[[:space:]]+\*RuntimeAudit' internal/pkg/workorchprojectioncontract/acceptance.go ||
	 ! grep -q -E -- 'Acceptance[[:space:]]+AcceptanceProjection' internal/pkg/workorchprojectioncontract/snapshot.go ||
	 ! grep -q -E -- 'GetRunSnapshot.*\*workorchprojectioncontract\.RunSnapshot' internal/pkg/workorchclient/run.go; then
	echo "boundary contract violation: port/client must consume the complete canonical Runtime audit projection" >&2
	exit 1
fi

# The LLM owner transports provider request facts only. Candidate rejection is
# Orchestrator SM-3 authority and must never reappear on the EventHub Result.
llm_candidate_result_shape="$(sed -n '/^type RequestRuntimeCandidateResult struct {/,/^}/p' internal/modules/blocks/llm/pkg/events/messages.go)"
llm_document_outcome_shape="$(sed -n '/^type RuntimeProviderDocumentOutcome struct {/,/^}/p' internal/modules/blocks/llm/pkg/events/messages.go)"
if ! grep -q -E -- 'Feedback[[:space:]]+\*runtimecontract\.RuntimeFeedback' <<<"$llm_candidate_result_shape" ||
   ! grep -q -E -- 'Stale[[:space:]]+\*runtimecontract\.RuntimePrerequisiteStale' <<<"$llm_candidate_result_shape" ||
   ! grep -q -E -- 'DocumentOutcome[[:space:]]+\*RuntimeProviderDocumentOutcome' <<<"$llm_candidate_result_shape" ||
   grep -q -E -- 'DocumentContent[[:space:]]+\[\]byte' <<<"$llm_candidate_result_shape" ||
   grep -q -E -- '(^|[[:space:]])(Failure[[:space:]]+\*runtimecontract\.RuntimeCandidateFailure|RequestID[[:space:]]+string|Status[[:space:]]+string)' <<<"$llm_candidate_result_shape"; then
  echo "boundary contract violation: LLM provider Result must be the concrete Stale|Feedback|DocumentOutcome union without content-only fallback, candidate failure, or duplicate audit authority" >&2
  exit 1
fi
if ! grep -q -E -- 'Content[[:space:]]+\[\]byte[[:space:]]+`json:"-"`' <<<"$llm_document_outcome_shape" ||
   grep -q -E -- 'Content[[:space:]]+\[\]byte' <<<"$llm_candidate_result_shape"; then
  echo "boundary contract violation: complete provider document content must remain volatile and nested only in the typed DocumentOutcome with json exclusion" >&2
  exit 1
fi
if ! grep -q -F -- 'result requires exactly one of stale, feedback, or document outcome' internal/modules/blocks/llm/pkg/events/messages.go; then
  echo "boundary contract violation: LLM provider Result no longer enforces its exactly-one factual union" >&2
  exit 1
fi
raw_provider_framing_reclassifiers="$({
  find internal/modules internal/services internal/initiators \
    -type f -name '*.go' ! -name '*_test.go' \
    ! -path 'internal/modules/blocks/llm/*' -print0 |
    xargs -0 -r grep -n -E -- 'InspectRuntimeCandidateDocumentFraming|RuntimeCandidateDocumentDispositionForError' || true
} | sed '/^[[:space:]]*$/d')"
if [[ -n "$raw_provider_framing_reclassifiers" ]]; then
  cat >&2 <<EOF
Boundary contract violation: a downstream component reclassifies raw provider
document framing owned by the LLM block.

$raw_provider_framing_reclassifiers
EOF
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
  terminal_delivery_shape="$(sed -n '/^type RuntimeTerminalDeliveryFact struct {/,/^}/p' "internal/modules/blocks/${terminal_owner}/pkg/events/lifecycle_messages.go")"
  if ! grep -q -E -- 'runtimecontract\.RuntimeTerminalDeliveryFact' <<<"$terminal_delivery_shape" ||
     grep -q -E -- 'FailureReason[[:space:]]+string' <<<"$terminal_delivery_shape"; then
    echo "boundary contract violation: ${terminal_owner} terminal delivery must carry one canonical immutable fact without a parallel failure reason" >&2
    exit 1
  fi
done
terminal_delivery_fact_shape="$(sed -n '/^type RuntimeTerminalDeliveryFact struct {/,/^}/p' internal/pkg/workorchruntimecontract/runtime_lifecycle_delivery.go)"
if ! grep -q -E -- 'RuntimeTerminalDeliveryIdentity' <<<"$terminal_delivery_fact_shape" ||
   ! grep -q -E -- 'SourceOwner[[:space:]]+string' <<<"$terminal_delivery_fact_shape" ||
   ! grep -q -E -- 'CausationID[[:space:]]+string' <<<"$terminal_delivery_fact_shape"; then
  echo "boundary contract violation: Runtime terminal delivery fact must preserve sealed identity, source owner and causation" >&2
  exit 1
fi
if grep -q -E -- '^type MemorySearchQuery struct' internal/pkg/workorchmanagementcontract/definition.go ||
   grep -q -E -- '^type ReadWorkspaceRuntimeViewRequest struct' internal/pkg/workorchport/workspace_runtime_view.go; then
  echo "boundary contract violation: application ports must use canonical owner-neutral query values without parallel request DTOs" >&2
  exit 1
fi
runtime_view_command_shape="$(sed -n '/^type ReadWorkspaceRuntimeViewCommand struct {/,/^}/p' internal/modules/application/workorchd/pkg/events/messages.go)"
memory_search_command_shape="$(sed -n '/^type CLIMemorySearchCommand struct {/,/^}/p' internal/modules/application/workorchd/pkg/events/cli_contracts.go)"
if ! grep -q -E -- 'Query[[:space:]]+workorchquerycontract\.WorkspaceRuntimeViewQuery' <<<"$runtime_view_command_shape" ||
   ! grep -q -E -- 'Query[[:space:]]+workorchquerycontract\.MemorySearchQuery' <<<"$memory_search_command_shape"; then
  echo "boundary contract violation: workorchd owner commands must wrap canonical query values" >&2
  exit 1
fi
if ! grep -q -E -- 'projectioncontract\.RunSnapshotOf\[appmodelcore\.Workspace, appmodelcore\.Run\]' internal/modules/application/workorchd/biz/appservice/readmodelcore/snapshot_state.go ||
   ! grep -q -E -- 'projectioncontract\.WorkspaceSnapshotOf\[appmodelcore\.Workspace, appmodelcore\.Run\]' internal/modules/application/workorchd/biz/appservice/readmodelcore/workspace_snapshot_state.go ||
   ! grep -q -E -- 'projectioncontract\.RunObservabilityOf\[appmodelcore\.Run\]' internal/modules/application/workorchd/biz/appservice/readmodelcore/observability_state.go; then
  echo "boundary contract violation: application read states must instantiate canonical projection layouts instead of duplicating their fields" >&2
  exit 1
fi
execution_intent_admission_shape="$(sed -n '/^type RuntimeExecutionIntentAdmission struct {/,/^}/p' internal/pkg/workorchruntimecontract/runtime_execution_intent.go)"
for required_field in \
  'WorkspaceID[[:space:]]+common\.WorkspaceID' \
  'RunID[[:space:]]+common\.RunID' \
  'RequestID[[:space:]]+string' \
  'Kind[[:space:]]+RuntimeExecutionIntentKind' \
  'Outcome[[:space:]]+RuntimeExecutionIntentOutcome' \
  'Reason[[:space:]]+RuntimeExecutionIntentReason'; do
  if ! grep -q -E -- "$required_field" <<<"$execution_intent_admission_shape"; then
    echo "boundary contract violation: canonical Runtime execution-intent admission is incomplete: $required_field" >&2
    exit 1
  fi
done
if grep -q -E -- '(Disposition|Diagnostic)[[:space:]]+string' <<<"$execution_intent_admission_shape"; then
  echo "boundary contract violation: canonical Runtime execution-intent admission must preserve typed facts without free-form disposition/diagnostic fields" >&2
  exit 1
fi

for execution_intent_owner in run workspace; do
  execution_intent_command_shape="$(sed -n '/^type WorkspaceExecutionIntentCommand struct {/,/^}/p' "internal/modules/blocks/${execution_intent_owner}/pkg/events/lifecycle_messages.go")"
  if ! grep -q -E -- 'Kind[[:space:]]+runtimecontract\.RuntimeExecutionIntentKind' <<<"$execution_intent_command_shape"; then
    echo "boundary contract violation: ${execution_intent_owner} execution-intent Command must carry the typed intent kind" >&2
    exit 1
  fi
done

if ! grep -q -E -- '^type WorkspaceExecutionIntentResult runtimecontract\.RuntimeExecutionIntentAdmission$' internal/modules/blocks/workspace/pkg/events/lifecycle_messages.go ||
   ! sed -n '/^type WorkspaceExecutionIntentResult struct {/,/^}/p' internal/modules/blocks/run/pkg/events/lifecycle_messages.go | grep -q -E -- 'runtimecontract\.RuntimeExecutionIntentAdmission' ||
   ! grep -q -E -- '^type WorkspaceRunRequestedResult runtimecontract\.RuntimeExecutionIntentAdmission$' internal/modules/application/workorchd/pkg/events/messages.go ||
   ! grep -q -E -- '^type RunCommandResult runtimecontract\.RuntimeExecutionIntentAdmission$' internal/pkg/workorchport/run.go; then
  echo "boundary contract violation: execution-intent Result roots must consume the canonical typed admission fact without duplicating its fields" >&2
  exit 1
fi
orchestrator_terminal_delivery_shape="$(sed -n '/^type RuntimeTerminalDeliveryRequestData struct {/,/^}/p' internal/modules/kernel/orchestrator/pkg/events/lifecycle_messages.go)"
if ! grep -q -E -- 'runtimecontract\.RuntimeTerminalDeliveryIdentity' <<<"$orchestrator_terminal_delivery_shape"; then
  echo "boundary contract violation: Orchestrator terminal delivery request must carry the canonical sealed identity" >&2
  exit 1
fi
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

# Owner models and focused application projection DTOs are canonical. Event
# contracts may reference those stable values, but must not recreate a second
# isomorphic struct plus field-by-field mappers inside the same owner.
duplicate_owner_dto_matches="$({
  grep -Hn -E '^type (Goal|DoD) struct' internal/modules/blocks/workspace/pkg/events/messages.go || true
  grep -Hn -E '^type (MemoryRecallHit|MemoryRecallSelectionScore) struct' internal/modules/blocks/memory/pkg/events/messages.go || true
  grep -Hn -E '^type ProviderUsageData struct' internal/modules/blocks/llm/pkg/events/messages.go || true
	grep -Hn -E '^type ProviderUsage(View)? struct' \
		internal/modules/blocks/llm/pkg/models/*.go \
		internal/pkg/workorchruntimecontract/*.go || true
	grep -Hn -E '^type (WorkspaceRefreshRevision|ProviderModelTarget) struct' internal/pkg/workorchclient/*.go || true
	grep -Hn -E '^type RuntimeSemanticFieldViolation(Info)? struct' \
		internal/pkg/workorchport/*.go \
		internal/modules/application/workorchd/biz/appservice/readmodelcore/*.go || true
	grep -Hn -E '^type RuntimeAuditEvidence(Info)? struct' \
		internal/pkg/workorchclient/*.go \
		internal/pkg/workorchport/*.go \
		internal/modules/application/workorchd/biz/appservice/readmodelcore/*.go || true
	grep -Hn -E '^type RuntimeAudit(CoverageBinding|CoverageProof|Dispatch|SealedOperation|OperationReceipt|Repair|Assessment|AssessmentItem|AssessmentGap|Failure) struct' \
		internal/pkg/workorchclient/*.go \
		internal/modules/application/workorchd/biz/appservice/readmodelcore/*.go || true
	grep -Hn -E '^type (Goal|DoD) struct' \
		internal/pkg/workorchclient/*.go \
		internal/modules/application/workorchd/biz/appservice/appmodelcore/*.go || true
	grep -Hn -E '^type (RunLineage|RuntimeActivationStatus) struct' \
		internal/pkg/workorchclient/*.go \
		internal/modules/application/workorchd/biz/appservice/appmodelcore/*.go || true
	grep -Hn -E '^type (LLMContextBudgetTrace|LLMContextBudgetSection|LLMContextBudgetSummary|LLMUsageTrace|LLMUsageSummary|LLMUsageProviderModelBreakdown) struct' \
		internal/pkg/workorchclient/*.go \
		internal/modules/application/workorchd/biz/appservice/readmodelcore/*.go || true
	grep -Hn -E '^type (ToolCall|Patch|ValidationResult|Artifact|ArtifactContinuation|RuntimeCheckpoint) struct' \
		internal/pkg/workorchclient/*.go \
		internal/modules/application/workorchd/biz/appservice/readmodelcore/*.go || true
	grep -Hn -E '^type (AcceptanceSlice|CoverageFocus|AcceptanceSummary|ConvergenceBudget|RuntimeRepairDirective|RuntimeRepairClosure|RuntimeOperationStatus|RuntimeCapabilityState|RuntimeGovernanceState|RuntimeSemanticPlanning|RuntimeSemanticPlanningItem|RuntimeSemanticFeasibilityGap|RuntimeSemanticDiagnostic) struct' \
		internal/pkg/workorchclient/*.go \
		internal/modules/application/workorchd/biz/appservice/readmodelcore/*.go || true
	grep -Hn -E '^type (ContinuationDiagnostic|ContinuationReadbackDiagnostic|ContinuationDiagnosticCount|WatchdogDiagnostics|ExecutionAdmissionFailureDiagnostic|ExecutionAdmissionFailureCount|SchedulerDiagnostics) struct' \
		internal/pkg/workorchclient/*.go \
		internal/modules/application/workorchd/biz/appservice/readmodelcore/*.go || true
	grep -Hn -E '^type (DoDAssessment|DoDAssessmentItem|DoDCoverage|DoDCoverageItem) struct' \
		internal/pkg/workorchclient/*.go \
		internal/modules/application/workorchd/biz/appservice/readmodelcore/*.go || true
	grep -Hn -E '^type (RuntimeProgress|RuntimePrerequisiteRefreshInvalidation|RuntimePrerequisiteResourceChange) struct' \
		internal/pkg/workorchclient/*.go \
		internal/modules/application/workorchd/biz/appservice/readmodelcore/*.go || true
	grep -Hn -E '^type RunHeartbeat struct' \
		internal/pkg/workorchclient/*.go \
		internal/modules/application/workorchd/biz/appservice/readmodelcore/*.go || true
	grep -Hn -E '^type (BlockingResourceRef|RecoveryPlan|AssessmentOutcome|ProgressMilestone) struct' \
		internal/pkg/workorchclient/*.go \
		internal/modules/application/workorchd/biz/appservice/readmodelcore/*.go || true
	grep -Hn -E '^type (AcceptanceProjection|RuntimeRecoveryBudget|EventRecord|GapBaseline|GapTodoItem|GapDiagnostic|MemoryRecallHit|RunStatus|ConvergenceStatus|RuntimeAudit|RuntimeAuditFocus|RuntimeAuditCoverage|RuntimeAuditGrounding|RuntimeAuditPrerequisite|RuntimeAuditProviderCeiling|RuntimeAuditTargetSelection|RuntimeAuditFailureTarget|RuntimeAuditTargetSnapshot|RuntimeAuditPrerequisiteObservation|RuntimeAuditFreshnessDependency|RuntimeAuditOperationAdmissibility|RuntimeAuditAuthority|RuntimeAuditCandidateAttempt|RuntimeAuditTerminal) struct' \
		internal/pkg/workorchclient/*.go \
		internal/pkg/workorchport/*.go \
		internal/modules/application/workorchd/biz/appservice/readmodelcore/*.go || true
	grep -Hn -E '^type RuntimeAudit[A-Za-z]*Info struct' internal/pkg/workorchport/*.go || true
	grep -Hn -E '^type (ProviderCatalogSummary|ProviderModelDescriptorView) struct' \
		internal/pkg/workorchclient/*.go \
		internal/modules/application/workorchd/biz/appservice/configcore/*.go || true
	grep -Hn -E '^type (Decision|Approval|AutoGovernanceSummary|RepairLoop|TaskProgress|ProgressItem|ProgressBlocker|RunOutcome|WorkspaceOutcome|Result|ResultSummary|MemoryRecallTrace|MemoryRecallSelectionScore|TaskDecomposition|TaskDecompositionNode|TaskPlan) struct' \
		internal/pkg/workorchclient/*.go \
		internal/modules/application/workorchd/biz/appservice/appmodelcore/*.go \
		internal/modules/application/workorchd/biz/appservice/governancecore/*.go \
		internal/modules/application/workorchd/biz/appservice/readmodelcore/*.go || true
	grep -Hn -E '^type (WorkspaceViewModel|WorkspaceViewModelSection|WorkspaceActivityItem|WorkspaceActivityFeed|WorkspaceRuntimeView|RunCommandStatus) struct' \
		internal/pkg/workorchclient/*.go \
		internal/modules/application/workorchd/biz/appservice/appmodelcore/*.go \
		internal/modules/application/workorchd/biz/appservice/readmodelcore/*.go || true
	grep -Hn -E '^type (ServiceConfiguration|RuntimeConfiguration|RuntimeGitConfiguration|LLMConfiguration|AIProxyConfigurationView|CustomProviderConfigurationView|CustomProviderModelView|WebSearchConfiguration|WebSearchProviderConfiguration|WebSearchProviderTemplate|GovernanceConfiguration|GovernanceAutoTicketConfiguration|GovernanceAutoApprovalConfiguration) struct' \
		internal/pkg/workorchclient/*.go \
		internal/modules/application/workorchd/biz/appservice/configcore/*.go || true
	grep -Hn -E '^type (Soul|Skill|MCPServer|MCPCapability|MCPTool|MCPResource|MCPPrompt|MemoryItem) struct' \
		internal/pkg/workorchclient/*.go \
		internal/modules/application/workorchd/biz/appservice/contextregistrycore/*.go || true
	grep -Hn -E '^type WorkspaceTemplate struct' \
		internal/pkg/workorchclient/*.go \
		internal/modules/application/workorchd/biz/appservice/workspacecore/*.go || true
	grep -Hn -E '^type WorkspaceTemplate struct' internal/pkg/repository/*.go || true
	grep -Hn -E '^type (Workspace|Run|WorkspaceSnapshot|RunSnapshot|RunObservability) struct' \
		internal/pkg/workorchclient/*.go || true
	grep -Hn -E '^type (WorkspaceSnapshot|RunSnapshot|RunObservability|WorkspaceExecutionActivation) struct' \
		internal/modules/application/workorchd/biz/appservice/readmodelcore/*.go || true
	grep -Hn -E '^[[:space:]]+(InternalDirectory|RuntimeState)[[:space:]]' \
		internal/pkg/workorchprojectioncontract/*.go || true
	grep -Hn -E '^type (Workspace|Decision|Approval|EventStream|Governance|MCP|Memory|Orchestrator|Result|Run|Skill|Soul)Repositories[[:space:]]*=' \
		internal/pkg/workorchdeps/*.go || true
	grep -Hn -E '^type (ApprovalEvidence|DecisionEvidence|Event)(ValueKind|Field|Value|Payload)[[:space:]]*=' \
		internal/modules/blocks/approval/pkg/events/*.go \
		internal/modules/blocks/decision/pkg/events/*.go \
		internal/modules/blocks/eventstream/pkg/events/*.go || true
  grep -Hn -E '^type (WorkspaceViewModel|WorkspaceViewModelSection) struct' internal/modules/application/workorchd/biz/appservice/readmodelcore/*.go || true
  grep -Hn -E '^type Repository(Configuration|Config) struct' \
    internal/modules/application/workorchd/biz/appservice/configcore/*.go \
    internal/modules/application/workorchd/biz/appservice/runtimecore/*.go || true
} )"
if [[ -n "$duplicate_owner_dto_matches" ]]; then
  cat >&2 <<EOF
Boundary contract violation: an owner-local event, read-model, or runtime
package recreated an existing canonical model/configuration DTO.

$duplicate_owner_dto_matches

Reuse the owning component model or focused application DTO directly. Keep
only conversions that detach mutable data, stabilize map ordering, or adapt a
real protocol/persistence boundary; do not restore isomorphic facade structs.
EOF
  exit 1
fi

if [[ -d internal/modules/blocks/approval/pkg/contracts || -d internal/modules/blocks/decision/pkg/contracts ]]; then
	echo "boundary contract violation: owner-local Approval/Decision use-case inputs must stay under biz, not public pkg/contracts" >&2
	exit 1
fi

if [[ -e internal/services/workorch/biz/backends/port_alias.go ]]; then
	echo "boundary contract violation: backend adapters must consume workorchport directly; broad port facade aliases are forbidden" >&2
	exit 1
fi

for owner_messages in \
	internal/modules/blocks/file/pkg/events/messages.go \
	internal/modules/blocks/git/pkg/events/messages.go \
	internal/modules/blocks/mcp/pkg/events/messages.go \
	internal/modules/blocks/shell/pkg/events/messages.go \
	internal/modules/blocks/ssh/pkg/events/messages.go \
	internal/modules/blocks/web/pkg/events/messages.go; do
	if ! grep -q -F 'runtimecontract.RuntimeOperationExecutionContext' "$owner_messages"; then
		echo "boundary contract violation: capability-owner execute commands must embed the canonical Runtime operation execution context: $owner_messages" >&2
		exit 1
	fi
done

mcp_create_capability_shape="$(sed -n '/^type CreateCapabilityCommand struct {/,/^}/p' internal/modules/blocks/mcpregistry/pkg/events/messages.go)"
if grep -q -F -- 'mcpmodel.Capability' <<<"$mcp_create_capability_shape" ||
	grep -q -E -- '(^|[[:space:]])Status[[:space:]]' <<<"$mcp_create_capability_shape"; then
  echo "boundary contract violation: MCP capability commands must carry configuration intent, not the owner entity or lifecycle status" >&2
  exit 1
fi
for required_field in CapabilityID ServerID Kind Name Schema Enabled; do
	if ! grep -q -E -- "^[[:space:]]*${required_field}[[:space:]]" <<<"$mcp_create_capability_shape"; then
		echo "boundary contract violation: MCP create capability command is missing typed field ${required_field}" >&2
		exit 1
	fi
done

if grep -q -E -- '^type MCPCommandResult struct' internal/modules/blocks/mcpregistry/pkg/events/messages.go; then
	echo "boundary contract violation: MCP Registry topics must use concrete result types, not MCPCommandResult" >&2
	exit 1
fi
if grep -q -E -- '^type WorkspaceCommandResult struct' internal/modules/blocks/workspace/pkg/events/messages.go; then
	echo "boundary contract violation: Workspace management topics must use concrete result types, not WorkspaceCommandResult" >&2
	exit 1
fi

# Workspace management result values cross an owner boundary only when the
# consumer binds them back to the exact concrete command. Checking near every
# production use prevents a well-typed but foreign identity/scope (including a
# foreign item hidden in a list) from being accepted after decoding.
workspace_management_result_pattern='workspaceevents\.(WorkspaceResult|WorkspaceListResult|WorkspaceLLMSelectionResult|GoalResult|GoalListResult|DeleteGoalResult|DoDResult|DoDListResult|DeleteDoDResult|WorkspaceTemplateResult|WorkspaceTemplateListResult|DeleteWorkspaceTemplateResult|EnsureWorkspaceTemplateDefaultsResult)'
workspace_management_result_uses="$({
	grep -R -Hn -E --include='*.go' --exclude='*_test.go' -- "$workspace_management_result_pattern" internal || true
} | grep -v -E '^internal/modules/blocks/workspace/(biz|pkg/events)/' || true)"
if [[ -z "$workspace_management_result_uses" ]]; then
	echo "boundary contract violation: Workspace management result consumers disappeared from command-bound validation audit" >&2
	exit 1
fi
workspace_management_unbound_uses=""
while IFS=: read -r source_file source_line _; do
	[[ -n "$source_file" && -n "$source_line" ]] || continue
	validation_window="$(sed -n "${source_line},$((source_line + 14))p" "$source_file")"
	if ! grep -q -F -- '.ValidateFor(' <<<"$validation_window"; then
		workspace_management_unbound_uses+="${source_file}:${source_line}"$'\n'
	fi
done <<<"$workspace_management_result_uses"
if [[ -n "$workspace_management_unbound_uses" ]]; then
	echo "boundary contract violation: Workspace management result caller does not bind the result to its concrete command with ValidateFor" >&2
	echo "$workspace_management_unbound_uses" >&2
	exit 1
fi
for sealed_method in workspaceResultWorkspaceID goalResultIdentity doDResultIdentity workspaceTemplateResultID; do
	if ! grep -q -E -- "^[[:space:]]*${sealed_method}\(\)" internal/modules/blocks/workspace/pkg/events/management_result_validation.go; then
		echo "boundary contract violation: Workspace command-bound result interface is no longer package-sealed: ${sealed_method}" >&2
		exit 1
	fi
done
for result_validation_test in \
	TestWorkspaceManagementResultsValidateExactCommandIdentity \
	TestWorkspaceManagementListResultsRejectForeignOrDuplicateOwnerIdentity \
	TestWorkspaceOwnerReadPortsRejectMismatchedResultIdentityAndScope \
	TestWorkspaceSnapshotReaderRejectsMismatchedOwnerIdentity; do
	if ! grep -R -q -E --include='*_test.go' -- "^func ${result_validation_test}\\(" internal; then
		echo "boundary contract violation: Workspace command-bound result regression is missing (${result_validation_test})" >&2
		exit 1
	fi
done
mcp_result_shapes="$(sed -n \
	-e '/^type CreateCapabilityResult struct {/,/^}/p' \
	-e '/^type GetCapabilityResult struct {/,/^}/p' \
	-e '/^type ListCapabilitiesResult struct {/,/^}/p' \
	-e '/^type UpdateCapabilityResult struct {/,/^}/p' \
	internal/modules/blocks/mcpregistry/pkg/events/messages.go)"
if ! grep -q -F -- 'mcpmodel.Capability' <<<"$mcp_result_shapes"; then
	echo "boundary contract violation: MCP capability results must reuse the canonical owner value" >&2
	exit 1
fi
if ! grep -q -F -- 'mcpmodel.CloneCapability' internal/modules/blocks/mcpregistry/biz/owner.go ||
	! grep -q -F -- 'mcpmodel.CloneCapabilities' internal/modules/blocks/mcpregistry/biz/owner.go; then
	echo "boundary contract violation: MCP capability results must detach owner values in the handler" >&2
	exit 1
fi

mcp_create_server_shape="$(sed -n '/^type CreateServerCommand struct {/,/^}/p' internal/modules/blocks/mcpregistry/pkg/events/messages.go)"
if ! grep -q -E -- '^[[:space:]]*EnvironmentRefs[[:space:]]' <<<"$mcp_create_server_shape" ||
	grep -q -E -- '^[[:space:]]*Env[[:space:]]' <<<"$mcp_create_server_shape"; then
	echo "boundary contract violation: MCP Registry server commands must carry reference-only EnvironmentRefs" >&2
	exit 1
fi
if ! grep -q -F -- 'repository.MCPRegistryRepository' internal/modules/blocks/mcpregistry/biz/owner.go ||
	grep -q -E -- 'repository\.(MCPServerRepository|MCPCapabilityRepository)' internal/modules/blocks/mcpregistry/biz/owner.go; then
	echo "boundary contract violation: MCP Registry must use one aggregate CAS repository" >&2
	exit 1
fi

management_entity_write_leaks="$(grep -RIn -E 'func .*\b(cmd|req|s|cap) \*?workorchmanagementcontract\.(Soul|Skill|MCPServer|MCPCapability)\b' \
	internal/pkg/workorchclient \
	internal/modules/application/workorchd/biz/appservice \
	internal/modules/application/workorchd/service/server \
	--include='*.go' --exclude='*_test.go' || true)"
if [[ -n "$management_entity_write_leaks" ]]; then
	cat >&2 <<EOF
boundary contract violation: management write paths must accept field-only
mutation DTOs, not complete owner read entities.

$management_entity_write_leaks
EOF
	exit 1
fi

management_port_mirrors="$(grep -RIn -E '^type (GoalInfo|DoDInfo|SoulInfo|SkillInfo|MCPServerInfo|MCPCapabilityInfo|MemoryItemInfo|TemplateInfo|DecisionInfo|ApprovalInfo|ResultInfo|ArtifactInfo|ArtifactContinuationInfo|PatchInfo|MemoryRecallTraceInfo|WebSourceTraceInfo|LLMContextBudgetTraceInfo|LLMUsageTraceInfo|LLMUsageSummaryInfo|EventRecordInfo|RecoveryPlanInfo|AssessmentOutcomeInfo|RuntimeProgressInfo|RuntimePrerequisiteRefreshInvalidationInfo|WorkspaceOutcomeInfo|RunHeartbeatInfo|RunCommandStatusInfo|WorkspaceActivityItemInfo|WorkspaceActivityFeedInfo|ConvergenceStatusInfo|TaskDecompositionNodeInfo|TaskDecompositionInfo|TaskPlanInfo|TaskProgressInfo|RepairLoopInfo|AcceptanceProjectionInfo|RuntimeSemanticPlanningInfo|RuntimeSemanticPlanningItemInfo|RuntimeSemanticFeasibilityGapInfo|RuntimeRepairDirectiveInfo|GapBaselineInfo|GapTodoItemInfo|GapDiagnosticInfo|StepViewInfo|RunStatusInfo|WorkspaceViewModelInfo|WorkspaceInfo|RunInfo) struct' internal/pkg/workorchport --include='*.go' || true)"
if [[ -n "$management_port_mirrors" ]]; then
	cat >&2 <<EOF
boundary contract violation: process ports must reuse canonical management read
DTOs and mutation contracts instead of recreating management mirrors.

$management_port_mirrors
EOF
	exit 1
fi

result_owner_write_leaks="$(source_matches 'Topic(Result|Artifact)Command(Create|Update)|type (Create|Update)(Result|Artifact)Command' internal/modules/blocks/result/pkg/events internal/modules/kernel/orchestrator/biz)"
if [[ -n "$result_owner_write_leaks" ]]; then
	cat >&2 <<EOF
boundary contract violation: Result/Artifact generic entity writes bypass the
typed Result-owner lifecycle contract.

$result_owner_write_leaks
EOF
	exit 1
fi

if ! grep -q -F -- 'ev.Source() != "kernel/orchestrator"' internal/modules/blocks/run/biz/runtime_state_commands.go; then
	echo "boundary contract violation: opaque Runtime aggregate updates must authenticate the Orchestrator source" >&2
	exit 1
fi

# C4 is a provider-facing diagnostic projection, not a Runtime aggregate
# mirror. Keep its struct limited to the LLM owner's narrow provider-safe
# values so owner actions, recovery continuation, raw facts and repair hints
# cannot reach a future candidate request by accident.
c4_projection_file="internal/modules/blocks/llm/biz/runtime_candidate_context.go"
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

# Owner-neutral queries have one canonical value definition. Owner EventHub
# packages may embed these values in their own Commands, but must not recreate
# the query or import repository contracts to obtain it.
duplicate_query_contracts="$(
	grep -R -Hn -E '^type (MemorySearchQuery|MemoryRecallTraceQuery|AutoGovernanceTraceQuery|EventRecordQuery) struct' \
		internal/pkg/repository \
		internal/modules/blocks/memory/pkg/events \
		internal/modules/blocks/eventstream/pkg/events \
		internal/modules/kernel/governance/pkg/events \
		internal/modules/application/workorchd/biz/appservice/readmodelcore \
		internal/pkg/workorchclient || true
)"
if [[ -n "$duplicate_query_contracts" ]]; then
	echo "boundary contract violation: owner-neutral query DTOs must use internal/pkg/workorchquerycontract" >&2
	echo "$duplicate_query_contracts" >&2
	exit 1
fi

query_contract_violations="$(
	grep -Hn -E 'internal/(modules|pkg/repository)|^type [A-Za-z0-9_]*(Command|Data|Result) struct|map\[string\](any|interface\{\})' \
		internal/pkg/workorchquerycontract/*.go || true
)"
if [[ -n "$query_contract_violations" ]]; then
	echo "boundary contract violation: workorchquerycontract must remain an owner-neutral pure value package" >&2
	echo "$query_contract_violations" >&2
	exit 1
fi

duplicate_management_dtos="$(
	grep -R -Hn -E '^type (WorkspaceSoulBindings|embeddedTemplate|embeddedMCPServer|TimelineEntry|ProviderCacheCapability|ProviderInferenceProfile|ProviderContextProfile) struct' \
		internal/pkg/workorchclient \
		internal/pkg/workorchdefaults \
		internal/modules/application/workorchd/biz/appservice/appmodelcore || true
)"
if [[ -n "$duplicate_management_dtos" ]]; then
	echo "boundary contract violation: client/default/application duplicated a canonical management or owner DTO" >&2
	echo "$duplicate_management_dtos" >&2
	exit 1
fi

legacy_management_projection_dtos="$(
	grep -R -Hn -E '^type (CreateWorkspaceRequest|CreateWorkspaceCommand|ActivationRequest|workspaceExecutionCommandRequest|CLIWorkspaceCreateRequest|ProjectionInfo|EventRecordPayloadInfo|ResultSummaryInfo|DecisionQuery|ApprovalQuery|StepQuery|ToolCallQuery|ResultQuery|PatchQuery|ValidationResultQuery|ArtifactQuery|RuntimeCheckpointQuery) struct' \
		internal/pkg/workorchport \
		internal/pkg/workorchclient \
		internal/modules/application/workorchd \
		internal/services/workorch || true
)"
if [[ -n "$legacy_management_projection_dtos" ]]; then
	echo "boundary contract violation: management, projection, payload, Result, or pagination facade DTO was reintroduced" >&2
	echo "$legacy_management_projection_dtos" >&2
	exit 1
fi

duplicate_result_values="$(
	grep -R -Hn -E '^type (ResultSummary|DoDAssessment|DoDAssessmentItem|DoDCoverage|DoDCoverageItem) struct' \
		internal/modules internal/pkg/workorchport internal/pkg/workorchprojectioncontract || true
)"
if [[ -n "$duplicate_result_values" ]]; then
	echo "boundary contract violation: already-judged Result values must use internal/pkg/workorchresultcontract" >&2
	echo "$duplicate_result_values" >&2
	exit 1
fi

result_contract_violations="$(
	grep -Hn -E 'internal/modules|^type [A-Za-z0-9_]*(Command|Data|Result) struct|map\[string\](any|interface\{\})' \
		internal/pkg/workorchresultcontract/*.go || true
)"
if [[ -n "$result_contract_violations" ]]; then
	echo "boundary contract violation: workorchresultcontract must remain an owner-neutral pure value package" >&2
	echo "$result_contract_violations" >&2
	exit 1
fi

# Runtime execution focus has one Orchestrator-owned field-writing reducer.
# Other helpers may compute detached projections, but production code must not
# select RemainingFocuses[0] or assign the canonical queue independently.
if ! grep -q -F -- 'func replaceRuntimeCWIFocusProjectionForSemanticRebinding(' internal/modules/kernel/orchestrator/biz/runtime/cwi_ledger.go ||
	! grep -q -F -- 'replaceRuntimeCWIFocusProjectionForSemanticRebinding(facts' internal/modules/kernel/orchestrator/biz/runtime/validation_failure_repair.go ||
	grep -q -F -- 'sanitizeRuntimeValidationFailureRepairPendingFocus' internal/modules/kernel/orchestrator/biz/runtime/validation_failure_repair.go; then
	echo "boundary contract violation: validation-failure semantic rebinding bypasses the CWI focus projection reducer" >&2
	exit 1
fi
if ! grep -R -q -F --include='*_test.go' -- 'TestValidationFailureRepairSemanticRebindingClearsExecutionAuthorityThroughFocusReducer' internal/modules/kernel/orchestrator/biz/runtime; then
	echo "boundary contract violation: validation-failure semantic rebinding focus transition regression is missing" >&2
	exit 1
fi
runtime_focus_writes="$(
	grep -R -Hn -E 'facts\.(Focus|RemainingFocuses|CompletedFocuses)[[:space:]]*=' \
		internal/modules/kernel/orchestrator/biz/runtime --include='*.go' --exclude='*_test.go' |
		grep -v 'internal/modules/kernel/orchestrator/biz/runtime/cwi_ledger.go:' || true
)"
runtime_queue_head_selection="$(
	grep -R -Hn -E 'RemainingFocuses\[0\]' \
		internal/modules/kernel/orchestrator/biz/runtime --include='*.go' --exclude='*_test.go' || true
)"
if [[ -n "$runtime_focus_writes" || -n "$runtime_queue_head_selection" ]]; then
	echo "boundary contract violation: Runtime execution focus must be installed only by the Orchestrator eligible-focus reducer" >&2
	echo "$runtime_focus_writes" >&2
	echo "$runtime_queue_head_selection" >&2
	exit 1
fi

# Runtime authority must not regain the pre-reducer command/snapshot surfaces.
# Display-only projection names are intentionally checked separately below.
forbidden_runtime_surface_matches="$(
	find \
		internal/pkg/workorchruntimecontract \
		internal/modules/kernel/orchestrator \
		internal/modules/application/workorchd/biz/appservice/activationports \
		-type f -name '*.go' ! -name '*_test.go' -print0 |
		xargs -0 -r grep -Hn -E -- 'type (StagePacket|NextAction|ExecutionContextPatch|ResumeRunCommand)\b|DispatchNextStep|NextActionForRun' || true
)"
if [[ -n "$forbidden_runtime_surface_matches" ]]; then
	echo "boundary contract violation: a forbidden pre-reducer Runtime authority surface was reintroduced" >&2
	echo "$forbidden_runtime_surface_matches" >&2
	exit 1
fi

# Embedded templates and Souls are provider input. Historical graph/step
# authority keys cannot be kept there as examples because they would be sent
# to the model as live instructions.
legacy_provider_contract_pattern='executionGraph|executionContext|thoughtSummary|completionState|graphFollowUp|previousStepOutput|mcpToolCalls|repairActionPackage|patchRequests|StagePacket|DispatchNextStep|ExecutionContextPatch'
legacy_default_contract_matches="$(
	grep -R -Hn -E \
		--include='*.json' --include='*.md' \
		-- "$legacy_provider_contract_pattern" internal/pkg/workorchdefaults/defaults || true
)"
if [[ -n "$legacy_default_contract_matches" ]]; then
	echo "boundary contract violation: embedded provider defaults contain a retired Runtime contract key" >&2
	echo "$legacy_default_contract_matches" >&2
	exit 1
fi

legacy_script_contract_matches="$(
	find scripts -maxdepth 1 -type f -name '*.sh' ! -name 'verify-boundary-contract.sh' -print0 |
		xargs -0 -r grep -Hn -E -- "$legacy_provider_contract_pattern" || true
)"
if [[ -n "$legacy_script_contract_matches" ]]; then
	echo "boundary contract violation: an active verifier still drives a retired provider contract" >&2
	echo "$legacy_script_contract_matches" >&2
	exit 1
fi

# Runtime read projections carry factual blockers and display-only navigation,
# never a compatibility action object or a repair-loop command mirror.
projection_next_action_matches="$(
	grep -R -Hn -E --include='*.go' --include='*.ts' --include='*.tsx' --exclude='*_test.go' -- \
		'\bGapNextAction\b|[Nn]extAction[[:space:]]*[:?]|RecommendedNextSteps|RuntimeAuditRepair[^}]*TerminalDeliveryAttempts' \
		internal/pkg/workorchprojectioncontract \
		internal/modules/application/workorchd/biz/appservice/readmodelprojection \
		internal/modules/application/workorchd/biz/appservice/runtimecore \
		web/workorch-console/src || true
)"
if [[ -n "$projection_next_action_matches" ]]; then
	echo "boundary contract violation: a Runtime read projection retains action or terminal-delivery shadow state" >&2
	echo "$projection_next_action_matches" >&2
	exit 1
fi

script_next_action_matches="$(
	find scripts -maxdepth 1 -type f -name '*.sh' \
		! -name 'verify-boundary-contract.sh' \
		-print0 |
		xargs -0 -r grep -Hn -E -- 'nextAction|NextAction' || true
)"
if [[ -n "$script_next_action_matches" ]]; then
	echo "boundary contract violation: an active verifier treats a legacy nextAction projection as executable authority" >&2
	echo "$script_next_action_matches" >&2
	exit 1
fi

# Runtime creation has one application ingress. Run endpoints are read-only;
# execution-activations is the only normal workspace execution write route.
if grep -q -E -- 'http\.MethodPost|registerRunWriteRoutes|handleWorkspaceScopedRunPost' \
	internal/modules/application/workorchd/service/server/http_run.go \
	internal/modules/application/workorchd/service/server/http_write.go; then
	echo "boundary contract violation: a direct Run write route was reintroduced" >&2
	exit 1
fi
if ! grep -q -E -- 'execution-activations".*, http\.MethodPost' \
	internal/modules/application/workorchd/service/server/http_workspace.go; then
	echo "boundary contract violation: the typed workspace execution-activation ingress is missing" >&2
	exit 1
fi
rich_activation_command_matches="$(
	grep -R -Hn -E \
		--include='*.go' \
		-- 'ActivateWorkspaceExecutionCommand|WorkspaceRuntimeActivation(Request|Build|ByIDRequest)|BuildWorkspaceRuntimeActivation|ReadWorkspaceRuntimeActivationInputs' \
		internal \
		|| true
)"
if [[ -n "$rich_activation_command_matches" ]]; then
	echo "boundary contract violation: restart bootstrap exposes per-run activation overrides instead of compiling owner state" >&2
	echo "$rich_activation_command_matches" >&2
	exit 1
fi

# These knobs belonged to removed graph/artifact-output paths. Keeping a
# configuration field after its behavior disappears creates a false contract.
legacy_llm_output_config_pattern='MaxPatchContentBytes|MaxGraphBytes|MaxGraphFileWrites|SchemaInvalidLargePreviewThreshold|MaxArtifactChunkBytes|MaxArtifactPlanAttempts|MaxArtifactReplanAttempts|maxPatchContentBytes|maxGraphBytes|maxGraphFileWrites|schemaInvalidLargePreviewThreshold|maxArtifactChunkBytes|maxArtifactPlanAttempts|maxArtifactReplanAttempts'
legacy_llm_output_config_matches="$(
	# Scan the repository contract surface, including non-ignored new files in
	# the worktree, without treating ignored runtime-local configuration (for
	# example config/application.toml) as versioned product authority.
	while IFS= read -r -d '' repository_file; do
		[[ -f "$repository_file" ]] || continue
		case "$repository_file" in
			*_test.go) ;;
			*.go|*.toml|*.ts|*.tsx)
				grep -Hn -E -- "$legacy_llm_output_config_pattern" "$repository_file" || true
				;;
		esac
	done < <(git ls-files -z --cached --others --exclude-standard -- internal config web/workorch-console/src)
)"
if [[ -n "$legacy_llm_output_config_matches" ]]; then
	echo "boundary contract violation: a retired graph/artifact LLM output setting was reintroduced" >&2
	echo "$legacy_llm_output_config_matches" >&2
	exit 1
fi

# Literal Go package paths in active scripts and Make targets must resolve.
# This catches renamed component packages before a verifier silently skips the
# intended ownership boundary.
mapfile -t verification_go_packages < <(
	grep -RhoE -- '\./(application|internal)/[A-Za-z0-9_./-]+' scripts Makefile |
		sed 's#/\.\.\.$##' |
		sort -u
)
for go_package in "${verification_go_packages[@]}"; do
	package_path="${go_package#./}"
	if [[ ! -d "$package_path" ]]; then
		echo "boundary contract violation: active verifier references a missing Go package: $go_package" >&2
		exit 1
	fi
done

# Focused closure verifiers must reject an empty -run selection rather than
# accepting Go's package-level '[no tests to run]' success.
for focused_verifier in \
	scripts/verify-acceptance-slice-result-e2e.sh \
	scripts/verify-acceptance-slice-result-deployed-e2e.sh \
	scripts/verify-workspace-file-diff-patch.sh \
	scripts/verify-auto-governance-ticket-approval.sh \
	scripts/verify-runtime-positive-convergence-file.sh; do
	if ! grep -q -F 'scripts/lib/go-test-must-run.sh' "$focused_verifier" ||
		! grep -q -F 'run_go_test ' "$focused_verifier"; then
		echo "boundary contract violation: focused verifier does not fail closed on an empty test selection: $focused_verifier" >&2
		exit 1
	fi
done

# Every named test-selector prefix in the deterministic positive-convergence
# verifier must still resolve to a current test. The runtime helper catches an
# entirely empty -run group; this guard also catches a stale alternative that
# would otherwise be hidden by another matching name in the same expression.
current_runtime_test_names="$(
	grep -RhoE --include='*_test.go' -- '^func Test[A-Za-z0-9_]+' internal |
		sed 's/^func //' |
		sort -u
)"
while IFS= read -r test_selector; do
	if ! grep -q -E "^${test_selector}" <<<"$current_runtime_test_names"; then
		echo "boundary contract violation: positive-convergence verifier references a stale test selector: $test_selector" >&2
		exit 1
	fi
done < <(
	grep -oE -- 'Test[A-Za-z0-9_]+' scripts/verify-runtime-positive-convergence-file.sh |
		sort -u
)

# Governance/application resolution consumes and displays owner state. It may
# not create a successor, resume a Run, or drive Runtime as a side effect.
governance_continuation_matches="$(
	find \
		internal/modules/kernel/governance \
		internal/modules/application/workorchd/biz/appservice/governancecore \
		internal/modules/application/workorchd/biz/appservice/governanceports \
		-type f -name '*.go' ! -name '*_test.go' -print0 |
		xargs -0 -r grep -Hn -E -- 'DriveRuntime|ResumeRunCommand|CreateRunCommand|StartRunCommand|ActivateWorkspaceExecution' || true
)"
if [[ -n "$governance_continuation_matches" ]]; then
	echo "boundary contract violation: governance resolution directly creates or continues Runtime work" >&2
	echo "$governance_continuation_matches" >&2
	exit 1
fi

# Provider budget settlement is Orchestrator owner-private. The retired shared
# observation API duplicated that reducer and would restore a second
# cross-owner budget transition surface.
runtime_provider_budget_shadow_matches="$(
	grep -R -Hn -E --include='*.go' -- \
		'RuntimeProviderBudgetObservation|RuntimeProviderConsumptionStatus|ApplyRuntimeProviderBudgetObservation' \
		internal/pkg/workorchruntimecontract || true
)"
if [[ -n "$runtime_provider_budget_shadow_matches" ]]; then
	echo "boundary contract violation: shared Runtime provider budget observation shadow returned" >&2
	echo "$runtime_provider_budget_shadow_matches" >&2
	exit 1
fi

# LLM physical transport observability carries the exact two-level request
# identity and has one bounded enum source. No persisted/public streaming
# compatibility boolean is retained.
if ! grep -q -F 'PhysicalResponseMode llmaudit.PhysicalResponseMode `json:"physicalResponseMode"`' \
	internal/pkg/workorchprojectioncontract/llm_observability.go ||
	! grep -q -F 'ProviderSubmitted    bool                          `json:"providerSubmitted"`' \
		internal/pkg/workorchprojectioncontract/llm_observability.go ||
	! grep -q -E 'LogicalRequestID[[:space:]]+string[[:space:]]+`json:"logicalRequestId,omitempty"`' \
		internal/pkg/workorchprojectioncontract/llm_observability.go ||
	! grep -q -E 'PhysicalCallID[[:space:]]+string[[:space:]]+`json:"physicalCallId,omitempty"`' \
		internal/pkg/workorchprojectioncontract/llm_observability.go ||
	! grep -q -F 'PayloadKeyLogicalRequestID        = "logicalRequestId"' \
		internal/modules/blocks/eventstream/pkg/recordpayload/payload.go ||
	! grep -q -F 'PayloadKeyPhysicalCallID          = "physicalCallId"' \
		internal/modules/blocks/eventstream/pkg/recordpayload/payload.go ||
	! grep -q -F 'LogicalRequestID:      data.LogicalRequestID' \
		internal/modules/blocks/eventstream/biz/event_stream_service.go ||
	! grep -q -F 'PhysicalCallID:        data.PhysicalCallID' \
		internal/modules/blocks/eventstream/biz/event_stream_service.go ||
	! grep -q -F 'LogicalRequestID:     logicalRequestID' \
		internal/modules/application/workorchd/biz/appservice/readmodelprojection/projection_contract_sections.go ||
	! grep -q -F 'PhysicalCallID:       physicalCallID' \
		internal/modules/application/workorchd/biz/appservice/readmodelprojection/projection_contract_sections.go ||
	! grep -q -F 'PhysicalResponseMode: physicalResponseMode' \
		internal/modules/application/workorchd/biz/appservice/readmodelprojection/projection_contract_sections.go; then
	echo "boundary contract violation: LLM usage projection lost canonical physical response/submission facts" >&2
	exit 1
fi
if grep -q -E 'PayloadKey(RequestBytes|Streaming)' \
	internal/modules/application/workorchd/biz/appservice/readmodelprojection/llm_usage_trace_identity.go ||
	grep -R -q -E --include='*.go' --include='*.ts' --include='*.tsx' \
		'PayloadKeyStreaming|json:"streaming|[[:space:]]Streaming[[:space:]]+bool|[[:space:]]streaming\??:[[:space:]]*boolean' \
		internal/modules/blocks/eventstream internal/modules/application/workorchd internal/pkg/workorchprojectioncontract web/workorch-console/src; then
	echo "boundary contract violation: LLM usage request identity is inferred from bytes or streaming" >&2
	exit 1
fi
if ! grep -q -F 'item.logicalRequestId' web/workorch-console/src/components/cards/LLMUsageCard.tsx ||
	! grep -q -F 'item.physicalCallId' web/workorch-console/src/components/cards/LLMUsageCard.tsx ||
	! grep -q -F 'item.logicalRequestId' web/workorch-console/src/helpers.tsx ||
	! grep -q -F 'item.physicalCallId' web/workorch-console/src/helpers.tsx; then
	echo "boundary contract violation: Web console no longer displays both LLM request identities" >&2
	exit 1
fi
llm_streaming_ui_matches="$(
	grep -R -Hn -E --include='*.ts' --include='*.tsx' -- "\\.streaming([^[:alnum:]_]|$)|dataIndex:[[:space:]]*['\"]streaming['\"]" web/workorch-console/src || true
)"
if [[ -n "$llm_streaming_ui_matches" ]]; then
	echo "boundary contract violation: Web console reads the compatibility streaming boolean instead of physicalResponseMode" >&2
	echo "$llm_streaming_ui_matches" >&2
	exit 1
fi
if grep -q -E -- 'expected_streaming|item\.get\("streaming"\)' scripts/verify-llm-workspace-execution.sh; then
	echo "boundary contract violation: deployed LLM verifier reads the compatibility streaming boolean" >&2
	exit 1
fi

# Workorchd projection may decode the opaque Runtime aggregate only as one
# mapper invocation's transient input. Long-lived application/store/transport
# values must remain bounded immutable DTOs and cannot become a second Runtime
# recovery source.
workorchd_runtime_shadow_matches="$({
	source_matches 'RuntimeStateRecord' \
		internal/modules/application/workorchd/biz/appservice/appmodelcore \
		internal/modules/application/workorchd/biz/appservice/readmodelcore \
		internal/modules/application/workorchd/biz/appservice/workspaceviewcore \
		internal/modules/application/workorchd/service/server
} | sed '/^[[:space:]]*$/d')"
if [[ -n "$workorchd_runtime_shadow_matches" ]]; then
	echo "boundary contract violation: Workorchd Store/HTTP cache retains RuntimeStateRecord" >&2
	echo "$workorchd_runtime_shadow_matches" >&2
	exit 1
fi
if grep -R -n -E --include='*.go' -- 'func[[:space:]]+workspaceProjectionState|RunStateBlockedOnApproval.*WorkspaceStateBlockedOnApproval' \
	internal/modules/application/workorchd; then
	echo "boundary contract violation: Workorchd regained a Run-to-Workspace owner state shadow mapping" >&2
	exit 1
fi

# Workspace has no governance blocker fields. Canonical blocker references are
# Run-owner facts; public/application/CLI/Web Workspace DTOs cannot retain an
# empty compatibility copy that can later become shadow authority.
application_workspace_shape="$(struct_body internal/modules/application/workorchd/biz/appservice/appmodelcore/workspace.go Workspace)"
public_workspace_shape="$(struct_body internal/pkg/workorchprojectioncontract/resource.go Workspace)"
cli_workspace_shape="$(struct_body internal/modules/application/workorchd/pkg/events/cli_workspace_dto.go CLIWorkspaceInfo)"
web_workspace_shape="$(sed -n '/^export type Workspace = {/,/^};/p' web/workorch-console/src/types.ts)"
if grep -q -E -- 'Blocking(Ticket|Approval)ID|blocking(Ticket|Approval)Id' <<<"$application_workspace_shape
$public_workspace_shape
$cli_workspace_shape
$web_workspace_shape"; then
	echo "boundary contract violation: Workspace DTO regained Run/governance blocker shadow fields" >&2
	exit 1
fi

# Snapshot/runtime-view outcome is explicitly a display-only value, with
# formal Result projected separately. Do not restore the old ambiguous outcome
# field/type alongside displayOutcome.
display_snapshot_files=(
	internal/pkg/workorchprojectioncontract/snapshot.go
	internal/pkg/workorchprojectioncontract/observability.go
	internal/pkg/workorchprojectioncontract/workspace_snapshot.go
	internal/pkg/workorchprojectioncontract/workspace_view.go
)
display_snapshot_shapes="$({
	sed -n '/^type RunSnapshotOf\[/,/^}/p' internal/pkg/workorchprojectioncontract/snapshot.go
	sed -n '/^type RunObservabilityOf\[/,/^}/p' internal/pkg/workorchprojectioncontract/observability.go
	sed -n '/^type WorkspaceSnapshotOf\[/,/^}/p' internal/pkg/workorchprojectioncontract/workspace_snapshot.go
	sed -n '/^type WorkspaceRuntimeView struct {/,/^}/p' internal/pkg/workorchprojectioncontract/workspace_view.go
	sed -n '/^export type RunObservability = {/,/^};/p; /^export type RunSnapshot = {/,/^};/p; /^export type WorkspaceSnapshot = {/,/^};/p' web/workorch-console/src/types.ts
})"
if grep -q -E -- 'json:"outcome(,|\")|(^|[^A-Za-z])(RunOutcome|WorkspaceOutcome)([^A-Za-z]|$)|^[[:space:]]*outcome\??:' <<<"$display_snapshot_shapes" ||
	grep -n -E -- '(^|[^A-Za-z])(RunOutcome|WorkspaceOutcome)([^A-Za-z]|$)' "${display_snapshot_files[@]}" web/workorch-console/src/types.ts; then
	echo "boundary contract violation: snapshot/runtime-view regained ambiguous outcome beside displayOutcome" >&2
	exit 1
fi
if ! grep -q -F 'DisplayOutcome' internal/pkg/workorchprojectioncontract/snapshot.go ||
	! grep -q -F 'DisplayOutcome' internal/pkg/workorchprojectioncontract/observability.go ||
	! grep -q -F 'DisplayOutcome' internal/pkg/workorchprojectioncontract/workspace_snapshot.go ||
	! grep -q -F 'DisplayOutcome' internal/pkg/workorchprojectioncontract/workspace_view.go; then
	echo "boundary contract violation: snapshot/runtime-view lost explicit displayOutcome contract" >&2
	exit 1
fi

# Display outcome/governance/consistency values may affect rendering only. The
# application execution use case and the Web/TUI intent constructors cannot use
# them to select or build Drive/Resume/Restart mutation intent.
display_mutation_matches="$({
	source_matches 'DisplayOutcome|GovernanceDisplay|ConsistencyDiagnostic' \
		internal/modules/application/workorchd/biz/execution
	sed -n '/^func ResumeRun(/,/^}/p; /^func RestartRun(/,/^}/p' \
		internal/services/workorch/service/tui/commands.go |
		grep -n -E -- 'DisplayOutcome|GovernanceDisplay|ConsistencyDiagnostic' || true
	sed -n '/^export function workspaceExecutionPrimaryIntent(/,/^}/p' \
		web/workorch-console/src/helpers.tsx |
		grep -n -E -- 'displayOutcome|governanceDisplay|consistencyDiagnostic' || true
} | sed '/^[[:space:]]*$/d')"
if [[ -n "$display_mutation_matches" ]]; then
	echo "boundary contract violation: display-only projection participates in execution mutation intent" >&2
	echo "$display_mutation_matches" >&2
	exit 1
fi

# RuntimeView may expose a concrete governance resource only through the
# canonical active Run's matching blocked state and exact blocker reference.
# GovernanceDisplay and a first-open resource from the same Run are not target
# authority.
workorchd_governance_resource_selection="$({
	sed -n '/^func selectBlockingDecision(/,$p' internal/modules/application/workorchd/biz/appservice/readmodelprojection/workspace_runtime_view.go
})"
if grep -q -E -- 'GovernanceDisplay|isOpenGovernanceState' <<<"$workorchd_governance_resource_selection" ||
	! grep -q -F 'canonicalActiveRun(snapshot, activeRunID)' <<<"$workorchd_governance_resource_selection" ||
	! grep -q -F 'run.BlockingTicketID' <<<"$workorchd_governance_resource_selection" ||
	! grep -q -F 'run.BlockingApprovalID' <<<"$workorchd_governance_resource_selection" ||
	! grep -q -F 'RunStateBlockedOnDecision' <<<"$workorchd_governance_resource_selection" ||
	! grep -q -F 'RunStateBlockedOnApproval' <<<"$workorchd_governance_resource_selection" ||
	! grep -q -F 'DecisionTicketStateOpen' <<<"$workorchd_governance_resource_selection" ||
	! grep -q -F 'ApprovalStatePending' <<<"$workorchd_governance_resource_selection"; then
	echo "boundary contract violation: Workorchd RuntimeView governance resource is not selected from the canonical Run blocker reference" >&2
	exit 1
fi

recovery_blocker_contract="$(cat \
	internal/pkg/workorchprojectioncontract/progress.go \
	internal/modules/application/workorchd/biz/appservice/readmodelprojection/governance_progress_projection.go \
	internal/modules/application/workorchd/biz/appservice/readmodelprojection/projection_readmodel_views.go)"
if ! grep -q -F 'type BlockingResourceRef struct' <<<"$recovery_blocker_contract" ||
	! grep -q -F 'AllowedUserActions: append([]string(nil), item.AllowedRecoveryActions...)' <<<"$recovery_blocker_contract" ||
	! grep -q -F 'selectRunBlockingDecision(snapshot.Run, snapshot.Decisions)' <<<"$recovery_blocker_contract" ||
	! grep -q -F 'selectRunBlockingApproval(snapshot.Run, snapshot.Approvals)' <<<"$recovery_blocker_contract" ||
	! grep -q -F 'SuggestedNavigation is display-only' <<<"$recovery_blocker_contract"; then
	echo "boundary contract violation: factual recovery blocker lost canonical owner selection or display-only navigation separation" >&2
	exit 1
fi

# Pre-seal admission must prove a complete owner-fact acquisition/reobserve
# closure; Runtime may consume that sealed fact but no application/capability
# projection may invent an action after Run creation.
definition_owner_fact_contract="$(cat \
	internal/modules/blocks/workspace/pkg/models/definition_admission.go \
	internal/modules/blocks/workspace/pkg/definitionadmission/compiler.go \
	internal/pkg/workorchruntimecontract/runtime_owner_fact.go)"
for marker in RequiredOwnerFactBindings OwnerFactConvergenceBindings RuntimeProfileContractsHash RuntimeOwnerFactMissingTrigger AcquisitionOperationVariantID AcquisitionTargetPolicy AcquisitionTargetSelectorRef MaxAcquisitionAttempts; do
	if ! grep -q -F "$marker" <<<"$definition_owner_fact_contract"; then
		echo "boundary contract violation: Definition owner-fact closure lost $marker" >&2
		exit 1
	fi
done
if ! grep -q -F 'declared_prerequisite_unresolved' <<<"$definition_owner_fact_contract"; then
	echo "boundary contract violation: incomplete owner-fact closure no longer fails before Run creation" >&2
	exit 1
fi
runtime_owner_fact_consumption="$(cat \
	internal/pkg/workorchruntimecontract/runtime_semantic_obligation.go \
	internal/pkg/workorchruntimecontract/runtime_semantic_binding.go \
	internal/pkg/workorchruntimecontract/runtime_semantic_payload.go \
	internal/modules/kernel/orchestrator/biz/runtime/semantic_planning.go \
	internal/modules/kernel/orchestrator/biz/runtime/assessment_owner_fact_replan.go)"
preseal_target_shapes=""
append_preseal_target_shape() {
	local type_name="$1"
	local source_file="$2"
	local shape
	shape="$(sed -n "/^type ${type_name} struct {/,/^}/p" "$source_file")"
	if [[ -z "$shape" ]]; then
		echo "boundary contract violation: cannot extract pre-seal type $type_name from $source_file" >&2
		return 1
	fi
	preseal_target_shapes+=$'\n'"$shape"
}
append_preseal_target_shape RuntimeProfileEvidenceRequirement internal/pkg/workorchruntimecontract/runtime_profile_contract.go
append_preseal_target_shape RuntimeBootstrapArtifact internal/pkg/workorchruntimecontract/runtime_profile_contract.go
append_preseal_target_shape RuntimeCoverageEvidenceRequirement internal/pkg/workorchruntimecontract/runtime_evidence_policy.go
append_preseal_target_shape RuntimeDefinitionFocus internal/pkg/workorchruntimecontract/runtime_definition_focus.go
append_preseal_target_shape RuntimeSemanticProofObligation internal/pkg/workorchruntimecontract/runtime_semantic_obligation.go
append_preseal_target_shape RuntimeSemanticOwnerFactAcquisition internal/pkg/workorchruntimecontract/runtime_semantic_obligation.go
append_preseal_target_shape RuntimeValidationFailureRepairConvergence internal/pkg/workorchruntimecontract/runtime_validation_failure_repair_convergence.go
append_preseal_target_shape RuntimeSemanticValidationFailureRepair internal/pkg/workorchruntimecontract/runtime_semantic_validation_failure_repair.go
if grep -q -i -E -- '[[:space:]](ProspectiveTargetRefs?|TargetRef)[[:space:]]|json:"[^"]*(prospectiveTargetRefs?|targetRef)' <<<"$preseal_target_shapes"; then
	echo "boundary contract violation: a pre-seal Definition/profile/semantic declaration regained an exact prospective target" >&2
	exit 1
fi
validation_repair_preseal_shapes="$({
	sed -n '/^type RuntimeValidationFailureRepairConvergence struct {/,/^}/p' internal/pkg/workorchruntimecontract/runtime_validation_failure_repair_convergence.go
	sed -n '/^type RuntimeSemanticValidationFailureRepair struct {/,/^}/p' internal/pkg/workorchruntimecontract/runtime_semantic_validation_failure_repair.go
})"
if grep -q -E -- '^[[:space:]]+[A-Za-z0-9_]*(Prospective|Exact)?TargetRefs?[[:space:]]+' <<<"$validation_repair_preseal_shapes" ||
	grep -q -E -- '^[[:space:]]*[A-Za-z0-9_]*(OneOf|AnyOf|Alternative|Quorum|Threshold|FirstSuccess)[A-Za-z0-9_]*[[:space:]]+' <<<"$validation_repair_preseal_shapes"; then
	echo "boundary contract violation: validation-failure repair declaration regained an exact target or alternative relation" >&2
	exit 1
fi
runtime_profile_requirement_shape="$(sed -n '/^type RuntimeProfileEvidenceRequirement struct {/,/^}/p' internal/pkg/workorchruntimecontract/runtime_profile_contract.go)"
if ! grep -q -E -- 'ValidationFailureRepairConvergence[[:space:]]+\*RuntimeValidationFailureRepairConvergence' <<<"$runtime_profile_requirement_shape" ||
	grep -q -E -- 'ValidationFailureRepairConvergences|\[\][[:space:]]*\*?RuntimeValidationFailureRepairConvergence' <<<"$runtime_profile_requirement_shape"; then
	echo "boundary contract violation: Profile validation-failure repair must remain one optional all-of convergence object" >&2
	exit 1
fi
runtime_coverage_focus_shape="$(sed -n '/^type CoverageFocus struct {/,/^}/p' internal/pkg/workorchruntimecontract/canonical_runtime.go)"
if grep -q -F -- 'ValidationFailureRepairConvergence' <<<"$runtime_coverage_focus_shape"; then
	echo "boundary contract violation: mutable CoverageFocus regained Profile validation-failure repair authority" >&2
	exit 1
fi
if ! grep -R -q -F --include='*_test.go' -- 'TestRuntimeValidationFailureRepairConvergenceIsSealedAcrossProfileAndDefinitionSurfaces' internal/pkg/workorchruntimecontract; then
	echo "boundary contract violation: immutable Definition/mutable CoverageFocus repair-convergence regression is missing" >&2
	exit 1
fi
validation_repair_profile_parser="$({
	sed -n '/^func runtimeValidationFailureRepairConvergence(/,/^}/p' internal/modules/application/workorchd/biz/appservice/activationports/runtime_activation.go
	sed -n '/^func runtimeValidationFailureRepairConvergenceHasExactShape(/,/^}/p' internal/modules/application/workorchd/biz/appservice/activationports/runtime_activation.go
})"
for marker in \
	'rawValue, present := input["validationFailureRepairConvergence"]' \
	'!present' \
	'!object' \
	'len(raw) == 0' \
	'default:' \
	'return false'; do
	if ! grep -q -F -- "$marker" <<<"$validation_repair_profile_parser"; then
		echo "boundary contract violation: validation-failure repair strict-current object parser lost $marker" >&2
		exit 1
	fi
done
if grep -q -E -- '"(oneOf|anyOf|alternative|alias)"' <<<"$validation_repair_profile_parser"; then
	echo "boundary contract violation: validation-failure repair strict-current whitelist accepts an alternative or alias field" >&2
	exit 1
fi
runtime_profile_requirement_normalization="$({
	sed -n '/^func ValidateRuntimeProfileContracts(/,/^}/p' internal/pkg/workorchruntimecontract/runtime_profile_contract.go
	sed -n '/^func normalizeRuntimeProfileEvidenceRequirements(/,/^}/p' internal/pkg/workorchruntimecontract/runtime_profile_contract.go
})"
for marker in \
	'duplicate evidence requirement scope+id' \
	'make([]RuntimeProfileEvidenceRequirement, 0, len(values))' \
	'ret = append(ret, value)' \
	'sort.SliceStable'; do
	if ! grep -q -F -- "$marker" <<<"$runtime_profile_requirement_normalization"; then
		echo "boundary contract violation: Profile evidence normalization no longer preserves duplicate scope+id for fail-closed admission ($marker)" >&2
		exit 1
	fi
done
if grep -q -F -- 'byKey[value.Scope+"|"+value.ID] = value' <<<"$runtime_profile_requirement_normalization"; then
	echo "boundary contract violation: Profile evidence normalization restored last-wins scope+id semantics" >&2
	exit 1
fi
runtime_bootstrap_artifact_parser="$(sed -n '/^func runtimeExecutionContract(/,/^}/p' internal/modules/application/workorchd/biz/appservice/activationports/runtime_activation.go)"
if ! grep -q -F 'runtimecontract.RuntimeBootstrapArtifact{' <<<"$runtime_bootstrap_artifact_parser" ||
	! grep -q -F 'AllowedOperationKinds: runtimeStrings(artifact["allowedOperationKinds"])' <<<"$runtime_bootstrap_artifact_parser" ||
	grep -q -i -E -- 'artifact\[[^]]*"(prospective)?targetRef"|[[:space:]](Prospective)?TargetRef[[:space:]]*:' <<<"$runtime_bootstrap_artifact_parser"; then
	echo "boundary contract violation: pre-seal profile parser regained an exact RuntimeBootstrapArtifact target" >&2
	exit 1
fi
runtime_bootstrap_artifact_compiler="$(sed -n '/^func compileRuntimeBootstrapArtifactPolicy(/,/^}/p' internal/modules/application/workorchd/biz/appservice/activationcore/runtime_focus_compiler.go)"
if ! grep -q -F 'file/workspace_deliverable' <<<"$runtime_bootstrap_artifact_compiler" ||
	grep -q -E -- 'file/profile_declared_candidates|TargetSelectorCandidateRefs:|ProspectiveTargetRef|artifact\.TargetRef' <<<"$runtime_bootstrap_artifact_compiler"; then
	echo "boundary contract violation: bootstrap artifact compilation regained a pre-seal exact target or closed candidate selector" >&2
	exit 1
fi
bootstrap_artifact_compiler_regression="$(sed -n '/^func TestCompileRuntimeCoverageFocusesPrependsMatchedBootstrapArtifactsInDependencyOrder(/,/^}/p' internal/modules/application/workorchd/biz/appservice/activationcore/runtime_focus_compiler_test.go)"
for marker in \
	'focus.TargetSelectorRef != "file/workspace_deliverable"' \
	'len(focus.ProspectiveTargetRefs) != 0' \
	'len(focus.TargetSelectorCandidateRefs) != 0'; do
	if ! grep -q -F "$marker" <<<"$bootstrap_artifact_compiler_regression"; then
		echo "boundary contract violation: target-free bootstrap artifact compiler regression is missing ($marker)" >&2
		exit 1
	fi
done
for marker in RuntimeSemanticProofObligation AcquisitionTargetSelectorRef DeclarationTargetSelectorCandidateRefs CompileRuntimeSemanticBindings runtimeOwnerFactAcquisitionFailureSignature; do
	if ! grep -q -F "$marker" <<<"$runtime_owner_fact_consumption"; then
		echo "boundary contract violation: SM-2 owner-fact acquisition lost bounded proposal selection or one-shot accounting ($marker)" >&2
		exit 1
	fi
done
if grep -q -F -- 'DeclarationProspectiveTargetRef' <<<"$runtime_owner_fact_consumption"; then
	echo "boundary contract violation: pre-seal semantic authority regained an exact prospective target" >&2
	exit 1
fi
if grep -R -n -E \
	--include='*.go' \
	--exclude='*_test.go' \
	-- 'CompileRuntimeSemanticBindingsWithSourceFocuses|ValidateRuntimeSemanticPlanSourceDeclarations(WithOwnerFacts)?' \
	internal/pkg/workorchruntimecontract >/dev/null; then
	echo "boundary contract violation: semantic binding restored a post-manifest DefinitionFocus compatibility path" >&2
	exit 1
fi
for regression in \
	'TestRuntimeSemanticOwnerFactAcquisitionSchemaUsesOperationSelectorNotDefinitionSelector' \
	'TestRuntimeSemanticOwnerFactAcquisitionSchemaBoundsProspectiveTargetByCandidateCardinality' \
	'TestCoverageEvidenceBindingProspectiveTargetRequiresBoundedSelectorSurface' \
	'TestRuntimeSemanticManifestRejectsAcquisitionSelectorDriftFromSealedCatalog' \
	'TestRuntimeSemanticProviderProjectsDefinitionLineageAndExactAcquisitionSelector' \
	'TestRuntimeCandidatePrefixChangesWithOwnerFactAcquisitionAuthorization' \
	'TestCompileRequiresExactStaticOwnerFactConvergenceBeforeAdmission'; do
	if ! grep -R -q -F --include='*_test.go' -- "$regression" internal; then
		echo "boundary contract violation: semantic selector separation regression is missing ($regression)" >&2
		exit 1
	fi
done
semantic_fixture_adapter="$(sed -n '/^func runtimeTestSemanticProofObligations(/,/^}/p' internal/modules/kernel/orchestrator/biz/runtime_test_support_test.go)"
if grep -q -E -- 'spec\.proposal.*ProspectiveTargetRef|DeclarationProspectiveTargetRef' <<<"$semantic_fixture_adapter"; then
	echo "boundary contract violation: test semantic fixture restored proposal-to-Definition target authority rebuild" >&2
	exit 1
fi
if ! grep -q -F 'TestCommitAssessmentFailsClosedAfterSingleSealedOwnerFactAcquisition' internal/modules/kernel/orchestrator/biz/runtime/assessment_owner_fact_replan_test.go; then
	echo "boundary contract violation: one-shot owner-fact acquisition exhaustion regression is missing" >&2
	exit 1
fi

# Definition admission owns the entire pre-seal input fence. Exact prospective
# targets cannot survive in a typed bootstrap declaration or hide recursively
# in raw Template/Workspace policy payloads. Policy normalization must be
# lossless per owner, and the final Runtime bootstrap must exactly match the
# Workspace policy projection sealed into the admission candidate.
runtime_bootstrap_validation="$(sed -n '/^func (b RuntimeBootstrap) Validate() error {/,/^}/p' internal/pkg/workorchruntimecontract/runtime_bootstrap.go)"
if ! grep -q -F 'requirement.ProspectiveTargetRef != ""' <<<"$runtime_bootstrap_validation" ||
	! grep -q -F 'runtime bootstrap requirement cannot carry a prospective target before semantic binding' <<<"$runtime_bootstrap_validation"; then
	echo "boundary contract violation: Runtime bootstrap no longer rejects a pre-seal exact prospective target" >&2
	exit 1
fi
runtime_preseal_input_guard="$({
	sed -n '/^func validateRuntimePresealTargetDeclarations(/,/^}/p' internal/modules/application/workorchd/biz/appservice/activationports/runtime_activation.go
	sed -n '/^func runtimeExecutionContractContainsBootstrapArtifactField(/,/^}/p' internal/modules/application/workorchd/biz/appservice/activationports/runtime_activation.go
	sed -n '/^func runtimeValueContainsField(/,/^}/p' internal/modules/application/workorchd/biz/appservice/activationports/runtime_activation.go
	sed -n '/^func runtimePayloadFieldsContainField(/,/^}/p' internal/modules/application/workorchd/biz/appservice/activationports/runtime_activation.go
	sed -n '/^func runtimePayloadValueContainsField(/,/^}/p' internal/modules/application/workorchd/biz/appservice/activationports/runtime_activation.go
})"
for marker in \
	'prospectiveTargetRef' \
	'targetRef' \
	'runtimeExecutionContractContainsBootstrapArtifactField' \
	'runtimeValueContainsField(child, fieldName)' \
	'runtimePayloadValueContainsField(field.Value, fieldName)' \
	'runtimePayloadValueContainsField(item, fieldName)' \
	'strings.EqualFold'; do
	if ! grep -q -F "$marker" <<<"$runtime_preseal_input_guard"; then
		echo "boundary contract violation: recursive pre-seal exact-target rejection lost $marker" >&2
		exit 1
	fi
done
runtime_evidence_policy_admission="$({
	sed -n '/^func validateRuntimeEvidencePolicyInputs(/,/^}/p' internal/modules/application/workorchd/biz/appservice/activationports/runtime_activation.go
	sed -n '/^func compileRuntimeEvidencePolicies(/,/^}/p' internal/modules/application/workorchd/biz/appservice/activationports/runtime_activation.go
})"
for marker in \
	'validateRuntimePresealTargetDeclarations(nil, items)' \
	'requires a non-empty string id' \
	'evidence policies duplicate id' \
	'policy.Validate()' \
	'len(normalized) != len(ret)' \
	'evidence policy normalization changed policy cardinality'; do
	if ! grep -q -F "$marker" <<<"$runtime_evidence_policy_admission"; then
		echo "boundary contract violation: Definition evidence-policy admission is no longer lossless ($marker)" >&2
		exit 1
	fi
done
definition_policy_seal="$(sed -n '/^func definitionAdmissionEvidencePolicyBindings(/,/^}/p' internal/modules/application/workorchd/biz/appservice/activationports/definition_admission.go)"
for marker in \
	'validateRuntimeEvidencePolicyInputs("Template ' \
	'validateRuntimeEvidencePolicyInputs("Workspace ' \
	'policy.CanonicalString()' \
	'PolicyRef:' \
	'Revision:' \
	'SnapshotHash:'; do
	if ! grep -q -F "$marker" <<<"$definition_policy_seal"; then
		echo "boundary contract violation: Definition candidate policy snapshot lost $marker" >&2
		exit 1
	fi
done
if ! grep -q -F 'AdmissionPolicyBindings: policyBindings' internal/modules/application/workorchd/biz/appservice/activationports/definition_admission.go; then
	echo "boundary contract violation: Definition candidate assembly omits the admitted policy snapshot bindings" >&2
	exit 1
fi
runtime_bootstrap_startup_seal="$(sed -n '/^func validateRuntimeBootstrapStartupSeal(/,/^}/p' internal/modules/application/workorchd/biz/appservice/activationports/workspace_execution_intent.go)"
if ! grep -q -F 'compileRuntimeEvidencePolicies("Workspace ' <<<"$runtime_bootstrap_startup_seal" ||
	! grep -q -F 'SameRuntimeEvidencePolicies(bootstrap.Activation.EvidencePolicies, expectedEvidencePolicies)' <<<"$runtime_bootstrap_startup_seal"; then
	echo "boundary contract violation: final Runtime bootstrap lost exact admitted Workspace evidence-policy projection" >&2
	exit 1
fi
for regression in \
	'TestDefinitionAdmissionRejectsRemovedPresealExactTargetField' \
	'TestDefinitionAdmissionRejectsBootstrapArtifactExactTarget' \
	'TestDefinitionAdmissionRejectsRemovedPresealExactTargetInEvidencePolicies' \
	'TestDefinitionAdmissionRejectsLossyEvidencePolicyInputsPerOwner' \
	'TestDefinitionAdmissionCandidateSealsWorkspaceEvidencePolicySnapshot' \
	'TestDefinitionAdmissionBlocksIncompleteValidationRepairContractBeforeRun' \
	'TestDefinitionAdmissionRejectsValidationRepairNonObjectAlternativeAndDuplicateShapesBeforeRuntimeWork' \
	'TestDefinitionAdmissionProfileContractInvalidHasNoRuntimeAdmission' \
	'TestRuntimeProfileEvidenceRequirementNormalizationPreservesCardinalityAndRejectsDuplicateScopeID' \
	'TestRuntimeBootstrapStartupSealRejectsTemplateAndProfileDrift' \
	'TestRuntimeActivationContextRejectsPresealExactProspectiveTarget' \
	'TestRuntimeActivationContextRejectsBootstrapArtifactExactTarget' \
	'post-binding bootstrap target uses open declaration selector'; do
	if ! grep -R -q -F --include='*_test.go' -- "$regression" internal; then
		echo "boundary contract violation: strict Definition/pre-seal policy regression is missing ($regression)" >&2
		exit 1
	fi
done

for regression in \
	'TestCompileRuntimeSemanticValidationFailureRepairSealsOneFileProofAndFreshValidationLineage' \
	'TestRejectedTypedValidationWithoutFailureTargetOpensProfileRepairGeneration' \
	'TestValidationFailureRepairMultiTargetSettlementUsesOneCompletedAllOfDispatch' \
	'TestRuntimeCandidateAttemptFailedRequiresManifestBoundTotalCorrection' \
	'TestCandidateCorrectionFrontierExhaustsSingleTargetAndSealsConcreteAlternative' \
	'TestRuntimeMainlineCandidateAuthorityExhaustionHasNoCorrectionProviderOrBudgetEffect' \
	'TestRuntimeFileBackedSemanticCandidateReplayExhaustionSurvivesCASBoundaries'; do
	if ! grep -R -q -F --include='*_test.go' -- "$regression" internal; then
		echo "boundary contract violation: validation-repair/candidate-correction convergence regression is missing ($regression)" >&2
		exit 1
	fi
done

# SM-2 seals one exact bounded target surface into both CoverageEvidenceBinding and its
# focus. Selector identity, candidate bounds, and the accepted prospective
# target must all match; partial equality would let either value become a
# shadow authority after restart.
coverage_binding_focus_validation="$(sed -n '/^func ValidateCoverageEvidenceBindingsForFocus(/,/^}/p' internal/pkg/workorchruntimecontract/runtime_coverage_evidence_binding.go)"
for marker in \
	'!sameStrings(binding.TargetRefs, focus.TargetRefs)' \
	'binding.TargetSelectorRef != focus.TargetSelectorRef' \
	'binding.TargetSelectorOwnerRef != focus.TargetSelectorOwnerRef' \
	'binding.TargetSelectorSubjectRef != focus.TargetSelectorSubjectRef' \
	'!sameStrings(binding.TargetSelectorCandidateRefs, focus.TargetSelectorCandidateRefs)' \
	'!sameStrings(binding.ProspectiveTargetRefs, focus.ProspectiveTargetRefs)'; do
	if ! grep -q -F "$marker" <<<"$coverage_binding_focus_validation"; then
		echo "boundary contract violation: Coverage binding/focus exact target surface lost $marker" >&2
		exit 1
	fi
done
if ! grep -q -F 'TestValidateCoverageEvidenceBindingsForFocusRequiresExactTargetSurface' internal/pkg/workorchruntimecontract/runtime_coverage_evidence_binding_test.go; then
	echo "boundary contract violation: Coverage binding/focus exact target surface regression is missing" >&2
	exit 1
fi

# Candidate decode/admission, owner feasibility, dispatch seal/load and receipt
# credit must interpret the three sealed target policies through one pure
# matcher. An exact binding is a bounded membership set; only the receipt and
# its sealed operation are compared for exact equality. Prospective all-of
# completion remains frozen by the separate collective gate below.
candidate_payload_target_matcher="$(sed -n '/^func validateRuntimeCandidatePayloadOperation(/,/^}/p' internal/pkg/workorchruntimecontract/runtime_candidate_payload_manifest.go)"
candidate_semantic_target_matcher="$(sed -n '/^func validateRuntimeCandidateOperationAgainstManifest(/,/^}/p' internal/pkg/workorchruntimecontract/candidate_contract_manifest_semantics.go)"
operation_feasibility_target_matcher="$(sed -n '/^func ValidateRuntimeOperationFeasibilityRequest(/,/^}/p' internal/pkg/workorchruntimecontract/runtime_operation_admissibility_acquisition.go)"
dispatch_target_matcher="$(sed -n '/^func ValidateRuntimeDispatchOperationTargetAuthorizations(/,/^}/p' internal/pkg/workorchruntimecontract/runtime_dispatch_target_authorization.go)"
receipt_target_matcher="$(sed -n '/^func MatchCoverageEvidenceBindingReceipt(/,/^}/p' internal/pkg/workorchruntimecontract/runtime_coverage_evidence_binding.go)"
for matcher_name in \
	candidate_payload_target_matcher \
	candidate_semantic_target_matcher \
	operation_feasibility_target_matcher \
	dispatch_target_matcher \
	receipt_target_matcher; do
	matcher_body="${!matcher_name}"
	matcher_calls="$(grep -c -F -- 'ValidateRuntimeOperationTargetsAgainstAuthorization' <<<"$matcher_body" || true)"
	if [[ "$matcher_calls" -ne 1 ]]; then
		echo "boundary contract violation: $matcher_name must call the canonical operation target matcher exactly once" >&2
		exit 1
	fi
done
for marker in \
	'RuntimeOperationTargetAuthorizationFromCoverageEvidenceBinding(binding)' \
	'!sameStrings(operation.TargetRefs, receipt.TargetRefs)'; do
	if ! grep -q -F -- "$marker" <<<"$receipt_target_matcher"; then
		echo "boundary contract violation: receipt target matching lost $marker" >&2
		exit 1
	fi
done
if grep -q -E -- 'binding\.(TargetRefs|ProspectiveTargetRefs)' <<<"$receipt_target_matcher"; then
	echo "boundary contract violation: receipt matching regained direct binding-set equality instead of canonical membership" >&2
	exit 1
fi
dispatch_manifest_target_seal="$(sed -n '/^func SealDispatchWithManifestAndOwnerAuthority(/,/^}/p' internal/modules/kernel/orchestrator/biz/runtime/dispatch_commit.go)"
dispatch_fresh_load_target_validation="$(sed -n '/^func (d RuntimeDispatchRecord) ValidateIntrinsic(/,/^}/p' internal/pkg/workorchruntimecontract/runtime_state.go)"
dispatch_authority_hash="$(sed -n '/^func StableRuntimeDispatchAuthorityHash(/,/^}/p' internal/pkg/workorchruntimecontract/runtime_state.go)"
if [[ "$(grep -c -F -- 'RuntimeDispatchOperationTargetAuthorizationsFromManifest(manifest, candidate)' <<<"$dispatch_manifest_target_seal" || true)" -ne 1 ]] ||
	! grep -q -F -- 'ValidateRuntimeDispatchOperationTargetAuthorizations' <<<"$dispatch_fresh_load_target_validation" ||
	! grep -q -F -- 'StableRuntimeDispatchOperationTargetAuthorizationsHash(dispatch.OperationTargetAuthorizations)' <<<"$dispatch_authority_hash"; then
	echo "boundary contract violation: final dispatch seal, fresh-load or authority hash lost sealed target authorization" >&2
	exit 1
fi
dispatch_target_snapshot_production_calls="$({
	source_matches 'RuntimeDispatchOperationTargetAuthorizationsFromManifest\(' internal
} | grep -v -F -- 'internal/pkg/workorchruntimecontract/runtime_dispatch_target_authorization.go:' | sed '/^[[:space:]]*$/d' || true)"
if [[ "$(wc -l <<<"$dispatch_target_snapshot_production_calls" | tr -d ' ')" -ne 1 ]] ||
	! grep -q -F -- 'internal/modules/kernel/orchestrator/biz/runtime/dispatch_commit.go:' <<<"$dispatch_target_snapshot_production_calls"; then
	echo "boundary contract violation: dispatch target snapshot must be created only by the final SM-3 manifest seal" >&2
	echo "$dispatch_target_snapshot_production_calls" >&2
	exit 1
fi
for regression in \
	'TestValidateRuntimeOperationTargetsAgainstAuthorizationTruthTable' \
	'TestCandidateTargetValidatorsShareExactTargetMembership' \
	'TestValidateRuntimeOperationFeasibilityRequestUsesSealedTargetAuthorization' \
	'TestCoverageEvidenceBindingReceiptUsesOperationTargetSubset' \
	'TestCoverageEvidenceBindingProspectiveReceiptRemainsPerOperationSingleton' \
	'TestRuntimeOperationTargetAuthorizationProofProjectionDoesNotInventConstraint' \
	'TestSealDispatchWithManifestRejectsTargetDriftAtFinalBoundary' \
	'TestRuntimeDispatchTargetAuthorizationFreshLoadRejectsTamper' \
	'TestCoverageEvidenceBindingMultiTargetExactRejectsReceiptOperationTargetDrift'; do
	if ! grep -R -q -F --include='*_test.go' -- "$regression" internal; then
		echo "boundary contract violation: canonical target authorization regression is missing ($regression)" >&2
		exit 1
	fi
done

production_test_target_authority="$({
	source_matches 'RuntimeOperationAuthorizationIdentity|identity-only-owner-feasibility' internal
} | sed '/^[[:space:]]*$/d')"
if [[ -n "$production_test_target_authority" ]]; then
	echo "boundary contract violation: production source regained fixture-only target authority" >&2
	echo "$production_test_target_authority" >&2
	exit 1
fi

# SM-3 semantic batches are forward-only. Provider document/schema correction
# happens while the batch remains Requesting; an owner feasibility result can
# never reopen the batch or authorize another target-selection attempt.
semantic_batch_reducer="internal/modules/kernel/orchestrator/biz/runtime/semantic_plan_batch_reducer.go"
semantic_batch_compiler="internal/modules/kernel/orchestrator/biz/internal/semanticplan/batch_compiler.go"
if [[ ! -f "$semantic_batch_reducer" || ! -f "$semantic_batch_compiler" ]]; then
	echo "boundary contract violation: SM-3 semantic batch reducer/compiler is not owner-private" >&2
	exit 1
fi
if grep -q -E 'func (AdvanceRuntimeSemanticPlanBatch|CompileRuntimeSemanticPlanBatch|CompileRuntimeSemanticPlanBatchFromOffset|CompileRuntimeSemanticPlanFocusBatch)\(' internal/pkg/workorchruntimecontract/runtime_semantic_batch.go ||
	grep -q -E 'State:[[:space:]]*RuntimeSemanticPlanBatchPending' internal/pkg/workorchruntimecontract/runtime_semantic_batch.go; then
	echo "boundary contract violation: shared Runtime contract regained semantic batch initialization or transition authority" >&2
	exit 1
fi
semantic_batch_advance="$(sed -n '/^func runtimeSemanticPlanBatchTransitionAllowed(/,/^}/p' "$semantic_batch_reducer")"
if grep -F 'RuntimeSemanticPlanBatchFeasibilityPending:' <<<"$semantic_batch_advance" | grep -q -F 'RuntimeSemanticPlanBatchRequesting'; then
	echo "boundary contract violation: SM-3 regained a feasibility-to-requesting correction back edge" >&2
	exit 1
fi
if grep -R -q -E --include='*.go' -- \
	'ReopenRuntimeSemanticPlanBatchForCorrection|DeriveRuntimeProspectiveSatisfactionCorrection|ValidateRuntimeProspectiveSemanticCorrectionDocument|prospective_owner_fact_unsatisfied' \
	internal/pkg/workorchruntimecontract internal/modules/kernel/orchestrator; then
	echo "boundary contract violation: retired owner-rejection target-correction authority returned" >&2
	exit 1
fi
if [[ -e internal/modules/kernel/orchestrator/biz/runtime/prospective_semantic_correction.go ]]; then
	echo "boundary contract violation: retired prospective semantic correction implementation returned" >&2
	exit 1
fi
orchestrator_semantic_batch_state_writes="$({
	source_matches '([A-Za-z0-9_]*[Bb]atch|Batches\[[^]]+\])\.State[[:space:]]*=[[:space:]]*[^=]' internal/modules/kernel/orchestrator/biz
	source_matches '\.State[[:space:]]*=[[:space:]]*runtimecontract\.RuntimeSemanticPlanBatch|State:[[:space:]]*runtimecontract\.RuntimeSemanticPlanBatch' internal/modules/kernel/orchestrator/biz
} | grep -v -F -- "$semantic_batch_reducer:" | sed '/^[[:space:]]*$/d' || true)"
orchestrator_semantic_batch_state_writes="$(grep -v -F -- "$semantic_batch_compiler:" <<<"$orchestrator_semantic_batch_state_writes" || true)"
if [[ -n "$orchestrator_semantic_batch_state_writes" ]]; then
	echo "boundary contract violation: Orchestrator writes RuntimeSemanticPlanBatch.State outside the canonical forward transition" >&2
	echo "$orchestrator_semantic_batch_state_writes" >&2
	exit 1
fi
semantic_batch_compiler_state_writes="$({
	source_matches '\.State[[:space:]]*=[[:space:]]*[^=]' "$semantic_batch_compiler"
	source_matches '(^|[^A-Za-z0-9_])State[[:space:]]*:' "$semantic_batch_compiler"
} | sed '/^[[:space:]]*$/d')"
if [[ "$(grep -c -E 'State:[[:space:]]*runtimecontract\.RuntimeSemanticPlanBatchPending' <<<"$semantic_batch_compiler_state_writes")" -ne 2 || "$(wc -l <<<"$semantic_batch_compiler_state_writes" | tr -d ' ')" -ne 2 ]]; then
	echo "boundary contract violation: owner-private semantic batch compiler must contain only its two canonical Pending initializations" >&2
	echo "$semantic_batch_compiler_state_writes" >&2
	exit 1
fi
semantic_batch_reducer_state_writes="$({
	source_matches '\.State[[:space:]]*=[[:space:]]*[^=]' "$semantic_batch_reducer"
	source_matches '(^|[^A-Za-z0-9_])State[[:space:]]*:' "$semantic_batch_reducer"
} | sed '/^[[:space:]]*$/d')"
if [[ "$(grep -c -F 'advanced.State = next' <<<"$semantic_batch_reducer_state_writes")" -ne 1 || "$(wc -l <<<"$semantic_batch_reducer_state_writes" | tr -d ' ')" -ne 1 ]]; then
	echo "boundary contract violation: owner-private SM-3 semantic batch reducer must contain exactly one state write, the canonical advanced.State = next assignment" >&2
	echo "$semantic_batch_reducer_state_writes" >&2
	exit 1
fi

# The persisted proof document and its exact proposal are revalidated against
# the manifest-sealed, Shell-authored all-of target constraint before any File
# or Shell owner probe. A matching target followed by would_not_satisfy is an
# owner-contract contradiction and terminates through the existing private
# semantic-contract event; it never opens a finite or open-selector retry.
semantic_proposal_preprobe="$(sed -n '/^func ValidateRuntimeSemanticProposalBeforeOwnerProbe(/,/^}/p' internal/modules/kernel/orchestrator/biz/runtime/prospective_satisfaction_contract.go)"
for marker in \
	'document.ValidateAgainstManifest(*ledger.Manifest)' \
	'StableRuntimeSemanticProofProposalHash(proposal)' \
	'matches != 1'; do
	if ! grep -q -F "$marker" <<<"$semantic_proposal_preprobe"; then
		echo "boundary contract violation: semantic proposal pre-probe guard lost $marker" >&2
		exit 1
	fi
done
semantic_owner_probe="$(sed -n '/^func (m \*Biz) probeRuntimeSemanticProposal(/,/^}/p' internal/modules/kernel/orchestrator/biz/runtime_semantic_drive.go)"
if ! grep -q -F 'runtime.ValidateRuntimeSemanticProposalBeforeOwnerProbe(ledger, proposal)' <<<"$semantic_owner_probe" ||
	! grep -q -F 'semantic_probe_proposal_invalid' <<<"$semantic_owner_probe"; then
	echo "boundary contract violation: semantic owner probe bypasses the persisted manifest/document preflight" >&2
	exit 1
fi
prospective_contract_violation="$(sed -n '/^func RuntimeProspectiveSatisfactionContractViolationDiagnostic(/,/^}/p' internal/modules/kernel/orchestrator/biz/runtime/prospective_satisfaction_contract.go)"
for marker in \
	'RuntimeOwnerFactProspectiveWouldNotSatisfy' \
	'prospective_satisfaction_contract_violated' \
	'StableRuntimeOwnerFactProspectiveSatisfactionHash(satisfaction)'; do
	if ! grep -q -F "$marker" <<<"$prospective_contract_violation"; then
		echo "boundary contract violation: prospective satisfaction contradiction mapping lost $marker" >&2
		exit 1
	fi
done
semantic_feasibility_drive="$(sed -n '/^func (m \*Biz) driveRuntimeSemanticFeasibility(/,/^}/p' internal/modules/kernel/orchestrator/biz/runtime_semantic_drive.go)"
if ! grep -q -F 'runtime.RuntimeProspectiveSatisfactionContractViolationDiagnostic(fact)' <<<"$semantic_feasibility_drive" ||
	! grep -q -F 'm.failRuntimeSemanticContract(ctx, aggregate, diagnostic.OriginalCode' <<<"$semantic_feasibility_drive" ||
	grep -q -F 'RuntimeInternalEventSemanticPlanCorrectionRequired' <<<"$semantic_feasibility_drive"; then
	echo "boundary contract violation: would_not_satisfy did not converge directly as an owner-contract violation" >&2
	exit 1
fi
for regression in \
	'TestRuntimeProspectiveProposalPreProbeRejectsTargetOutsideSealedConstraint' \
	'TestRuntimeProspectiveDriveTreatsConstraintValidOwnerRejectionAsContractViolation'; do
	if ! grep -R -q -F --include='*_test.go' -- "$regression" internal/modules/kernel/orchestrator; then
		echo "boundary contract violation: prospective all-of convergence regression is missing ($regression)" >&2
		exit 1
	fi
done

# The required-owner target fact is conjunctive only. Its provider projection
# carries the predicates and opaque provenance needed for admission, but must
# not leak the hidden dependency surface/recipe identity under an alias.
prospective_target_contract="$(cat internal/pkg/workorchruntimecontract/runtime_prospective_target_constraint.go)"
for marker in \
	'RuntimeProspectiveTargetSatisfiesConstraint' \
	'RuntimeProspectiveTargetsSatisfyConstraint' \
	'RuntimeProspectiveTargetClause' \
	'ResolveRuntimeProspectiveTargetSatisfactionConstraint'; do
	if ! grep -q -F "$marker" <<<"$prospective_target_contract"; then
		echo "boundary contract violation: prospective all-of target contract lost $marker" >&2
		exit 1
	fi
done
if ! grep -q -E -- 'AllOf[[:space:]]+\[\]RuntimeProspectiveTargetPredicate' <<<"$prospective_target_contract"; then
	echo "boundary contract violation: prospective all-of target contract lost AllOf" >&2
	exit 1
fi
if grep -q -E -- 'AnyOf|OneOf|Aggregation' <<<"$prospective_target_contract"; then
	echo "boundary contract violation: prospective target contract regained alternative aggregation" >&2
	exit 1
fi
if ! grep -q -E -- 'RuntimeProspectiveTargetRefsMaxCount[[:space:]]*=[[:space:]]*8' <<<"$prospective_target_contract"; then
	echo "boundary contract violation: bounded prospective acquisition lost its one shared max=8 constant" >&2
	exit 1
fi
for shared_bound_consumer in \
	internal/pkg/workorchruntimecontract/runtime_owner_fact_prospective_satisfaction.go \
	internal/pkg/workorchruntimecontract/runtime_semantic_contract.go \
	internal/pkg/workorchruntimecontract/runtime_semantic_payload.go \
	internal/pkg/workorchruntimecontract/runtime_coverage_evidence_binding.go \
	internal/pkg/workorchruntimecontract/candidate_contract_manifest.go \
	internal/modules/blocks/file/biz/owner.go \
	internal/modules/blocks/file/biz/file_runtime_target.go \
	internal/modules/blocks/shell/biz/shell_runtime_validation.go; do
	if ! grep -q -F -- 'RuntimeProspectiveTargetRefsMaxCount' "$shared_bound_consumer"; then
		echo "boundary contract violation: prospective acquisition does not consume the shared max=8 bound: $shared_bound_consumer" >&2
		exit 1
	fi
done
prospective_clause_shape="$(sed -n '/^type RuntimeProspectiveTargetClause struct {/,/^}/p' internal/pkg/workorchruntimecontract/runtime_prospective_target_constraint.go)"
prospective_set_evaluator="$(sed -n '/^func RuntimeProspectiveTargetsSatisfyConstraint(/,/^}/p' internal/pkg/workorchruntimecontract/runtime_prospective_target_constraint.go)"
if ! grep -q -E -- 'AllOf[[:space:]]+\[\]RuntimeProspectiveTargetPredicate' <<<"$prospective_clause_shape" ||
	! grep -q -F -- 'clauseMatchCounts := make([]int, len(constraint.AllOf))' <<<"$prospective_set_evaluator" ||
	! grep -q -F -- 'targetMatches := make([][]bool, len(targetRefs))' <<<"$prospective_set_evaluator" ||
	! grep -q -F -- 'if count == 0' <<<"$prospective_set_evaluator" ||
	! grep -q -F -- 'if !necessary' <<<"$prospective_set_evaluator"; then
	echo "boundary contract violation: prospective target evaluator no longer requires every all-of clause and rejects unrelated target padding" >&2
	exit 1
fi
for regression in \
	'TestRuntimeProspectiveTargetSatisfactionConstraintMatchesBoundedAllOfTargetSet' \
	'TestResolveRuntimeProspectiveTargetSatisfactionConstraintRejectsImplicitAnyOf' \
	'TestRuntimeProspectiveTargetSatisfactionConstraintEnforcesSharedBoundAndRelevantTargets'; do
	if ! grep -q -F -- "$regression" internal/pkg/workorchruntimecontract/runtime_prospective_target_constraint_test.go; then
		echo "boundary contract violation: bounded all-of prospective constraint regression is missing ($regression)" >&2
		exit 1
	fi
done

# File admits the complete hypothetical target batch against one manifest
# revision/capacity observation and never creates a directory or file during
# semantic feasibility. Both successful replay and rejection matrices freeze
# that zero-side-effect contract.
file_prospective_batch_admission="$(sed -n '/^func fileProspectiveSemanticTargetAdmission(/,/^}/p' internal/modules/blocks/file/biz/owner.go)"
for marker in \
	'RuntimeSemanticProposalTargetRefs(proposal)' \
	'RuntimeProspectiveTargetRefsMaxCount' \
	'for _, targetRef := range targetRefs' \
	'runtimeFileProspectiveTargetRefsAdditionalEntries' \
	'observation.entryCount+additional > defaultMaxEntries'; do
	if ! grep -q -F -- "$marker" <<<"$file_prospective_batch_admission"; then
		echo "boundary contract violation: File bounded batch admission lost $marker" >&2
		exit 1
	fi
done
file_batch_zero_effect_test="$(sed -n '/^func TestFileOwnerSemanticFeasibilityPublishesBoundedBatchDeltaWithoutMutation(/,/^}/p' internal/modules/blocks/file/prospective_workspace_manifest_delta_test.go)"
for marker in \
	'[]string{"go.mod", "internal/example/runtime_test.go"}' \
	'delta.ProspectiveTargetRefs' \
	'os.Lstat' \
	'reflect.DeepEqual(replayed, observation)'; do
	if ! grep -q -F -- "$marker" <<<"$file_batch_zero_effect_test"; then
		echo "boundary contract violation: File bounded batch zero-side-effect regression lost $marker" >&2
		exit 1
	fi
done
for regression in \
	'TestFileOwnerSemanticFeasibilityDoesNotPublishProspectiveDeltaForNonFeasibleInput' \
	'TestFileOwnerSemanticFeasibilityReusesCanonicalProspectiveAdmissionMatrixWithoutMutation'; do
	if ! grep -q -F -- "$regression" internal/modules/blocks/file/prospective_workspace_manifest_delta_test.go; then
		echo "boundary contract violation: File prospective admission/rejection zero-side-effect matrix is missing ($regression)" >&2
		exit 1
	fi
done

# Shell owns Go validation applicability. Freeze the four workspace readiness
# classes (empty/source-only/module-only/ready), package-root handling and the
# incomparable multi-root all-of case in its owner-local matrix.
shell_prospective_matrix="$(sed -n '/^func TestShellProspectiveTargetConstraintPublishesSufficientGoTestAllOfTargetSet(/,/^}/p' internal/modules/blocks/shell/biz/prospective_target_constraint_test.go)"
for marker in \
	'empty workspace' \
	'source only' \
	'module only' \
	'ready workspace' \
	'missing package root' \
	'existing package root' \
	'incomparable package roots' \
	'pkg/a/runtime_test.go' \
	'pkg/b/runtime.go'; do
	if ! grep -q -F -- "$marker" <<<"$shell_prospective_matrix"; then
		echo "boundary contract violation: Shell prospective Go validation matrix lost $marker" >&2
		exit 1
	fi
done
if ! grep -q -F -- 'TestShellProspectiveTargetConstraintCoversMultipleGoWorkspaceGapsInOneBoundedSet' internal/modules/blocks/shell/biz/prospective_target_constraint_test.go ||
	! grep -q -F -- 'TestShellOwnerProspectiveSatisfactionDuplicateIsPureValue' internal/modules/blocks/shell/biz/prospective_satisfaction_test.go ||
	! grep -q -F -- 'TestShellOwnerProspectiveSatisfactionRejectsRecipeIdentityDriftWithoutEffects' internal/modules/blocks/shell/biz/prospective_satisfaction_test.go; then
	echo "boundary contract violation: Shell bounded all-of, replay, or zero-effect prospective regressions are missing" >&2
	exit 1
fi

# SM-2/SM-3 must turn every target in the sealed prospective binding into one
# exact required operation. A candidate missing any target requirement, using
# another target, or trying to bind several targets to one requirement fails
# before dispatch; the provider schema carries the same exact singleton fence.
prospective_required_sequence="$(sed -n '/^func requiredStateChangeOperationSequence(/,/^}/p' internal/modules/kernel/orchestrator/biz/runtime/reducer.go)"
for marker in \
	'for _, binding := range facts.Focus.Normalized().CoverageEvidenceBindings' \
	'for index, targetRef := range binding.ProspectiveTargetRefs' \
	'StableCoverageEvidenceBindingHash(binding)' \
	'fmt.Sprintf("prospective-%s-target-%02d", bindingHash, index+1)' \
	'previousProspectiveRequirement' \
	'TargetRef: targetRef' \
	'complete the sealed prospective all-of target transaction'; do
	if ! grep -q -F -- "$marker" <<<"$prospective_required_sequence"; then
		echo "boundary contract violation: Orchestrator per-target RequiredOperationSequence lost $marker" >&2
		exit 1
	fi
done
required_operation_binding="$(sed -n '/^func ValidateRequiredOperationBindings(/,/^}/p' internal/pkg/workorchruntimecontract/runtime_candidate.go)"
if ! grep -q -F -- 'requirement.TargetRef != ""' <<<"$required_operation_binding" ||
	! grep -q -F -- '!sameStrings(operation.TargetRefs, []string{requirement.TargetRef})' <<<"$required_operation_binding"; then
	echo "boundary contract violation: candidate required-operation binding lost exact singleton target validation" >&2
	exit 1
fi
required_operation_schema="$(sed -n '/^func runtimeCandidateSchemaForManifest(/,/^}/p' internal/modules/blocks/llm/biz/runtime_candidate_schema.go)"
if ! grep -q -F -- 'for _, requirement := range manifest.RequiredOperationSequence' <<<"$required_operation_schema" ||
	! grep -q -F -- '"items": map[string]any{"const": requirement.TargetRef}' <<<"$required_operation_schema"; then
	echo "boundary contract violation: provider candidate schema lost exact per-target required operations" >&2
	exit 1
fi
if ! grep -q -F -- 'TestRequiredStateChangeOperationSequenceSealsCompleteProspectiveAllOfTargets' internal/modules/kernel/orchestrator/biz/runtime/reducer_test.go ||
	! grep -q -F -- 'TestValidateRequiredOperationBindingsRequiresExactProspectiveTarget' internal/pkg/workorchruntimecontract/runtime_operation_contract_test.go ||
	! grep -q -F -- 'operations[:1]' internal/pkg/workorchruntimecontract/runtime_operation_contract_test.go ||
	! grep -q -F -- 'partial prospective target set error' internal/pkg/workorchruntimecontract/runtime_operation_contract_test.go; then
	echo "boundary contract violation: prospective per-target sequence or partial-candidate rejection regression is missing" >&2
	exit 1
fi

# RequiredOperationSequence is sealed SM-3 authority, not a best-effort hint.
# Reject duplicate identities/targets and malformed dependency graphs before
# normalization can collapse ambiguity or a candidate can reach dispatch.
required_operation_sequence_validation="$(sed -n '/^func validateRequiredOperationVariants(/,/^}/p' internal/pkg/workorchruntimecontract/canonical_runtime.go)"
for marker in \
	'duplicates requirement id' \
	'duplicates exact target' \
	'depends on unknown requirement' \
	'duplicates dependency' \
	'dependency cycle'; do
	if ! grep -q -F -- "$marker" <<<"$required_operation_sequence_validation"; then
		echo "boundary contract violation: RequiredOperationSequence fail-closed validation lost $marker" >&2
		exit 1
	fi
done
if ! grep -q -F -- 'TestRequiredOperationSequenceValidationFailsClosed' internal/pkg/workorchruntimecontract/runtime_operation_contract_test.go ||
	! grep -q -F -- 'TestRuntimeCandidateSchemaBindsEachRequiredOperationToItsExactProspectiveTarget' internal/modules/blocks/llm/biz/runtime_candidate_provider_test.go; then
	echo "boundary contract violation: RequiredOperationSequence graph or exact provider-schema regression is missing" >&2
	exit 1
fi

# A multi-target prospective binding receives collective credit only after one
# sealed dispatch contains an evidence_committed exact accepted receipt for
# every target. Partial, rejected, stale, duplicate, or cross-dispatch members
# remain audit facts and cannot settle the CWI.
prospective_collective_matcher="$(sed -n '/^func MatchProspectiveCoverageReceiptSet(/,/^}/p' internal/modules/kernel/orchestrator/biz/internal/proof/prospective_receipt_set.go)"
for marker in \
	'OperationExecutionEvidenceCommitted' \
	'MatchCoverageEvidenceBindingReceipt' \
	'len(receipt.TargetRefs) != 1' \
	'finding.MissingTargetRefs'; do
	if ! grep -q -F -- "$marker" <<<"$prospective_collective_matcher"; then
		echo "boundary contract violation: prospective collective receipt matcher lost $marker" >&2
		exit 1
	fi
done
prospective_collective_runtime="$(cat \
	internal/modules/kernel/orchestrator/biz/runtime/prospective_receipt_set.go \
	internal/modules/kernel/orchestrator/biz/runtime/authority_assessment_transition.go \
	internal/modules/kernel/orchestrator/biz/runtime/aggregate_invariant.go)"
for marker in \
	'runtimeProspectiveCoverageKindComplete' \
	'validateRuntimeProspectiveCoverageCollectiveCredit' \
	'runtimeProspectiveCoverageReceiptSetFinding' \
	'validateRuntimeProspectiveCoverageCollectiveCredit(a)'; do
	if ! grep -q -F -- "$marker" <<<"$prospective_collective_runtime"; then
		echo "boundary contract violation: Orchestrator collective receipt credit lost $marker" >&2
		exit 1
	fi
done
for regression in \
	'TestMatchProspectiveCoverageReceiptSetRequiresCompleteAllOfOwnerEvidence' \
	'TestProspectiveAllOfReceiptSetSettlesExactlyOnceAfterRestart' \
	'TestProspectiveAllOfRejectedReceiptCannotSettleCWI' \
	'TestProspectiveAllOfAssessmentClosureUsesCollectiveReceiptGate'; do
	if ! grep -R -q -F --include='*_test.go' -- "$regression" internal/modules/kernel/orchestrator/biz; then
		echo "boundary contract violation: prospective collective receipt regression is missing ($regression)" >&2
		exit 1
	fi
done
provider_target_projection="$(sed -n '/^type runtimeSemanticProspectiveTargetConstraintProjection struct {/,/^}/p' internal/modules/blocks/llm/biz/runtime_semantic_projection.go)"
for marker in OwnerModuleID ProspectiveSatisfactionContractRef InputSchemaHash CatalogObservedRevision AllOf; do
	if ! grep -q -F "$marker" <<<"$provider_target_projection"; then
		echo "boundary contract violation: provider-safe prospective target projection lost $marker" >&2
		exit 1
	fi
done
if grep -q -E -- 'ValidationSurfaceID|ValidationRecipeID|validationSurfaceId|validationRecipeId' <<<"$provider_target_projection"; then
	echo "boundary contract violation: provider projection leaked dependency surface/recipe identity" >&2
	exit 1
fi
if ! grep -q -F 'strings.Contains(authority, prospectiveConstraint.ValidationRecipeID)' internal/modules/blocks/llm/biz/runtime_candidate_provider_test.go; then
	echo "boundary contract violation: dependency recipe identity value-isolation regression is missing" >&2
	exit 1
fi
# Run replacement recovery is one owner-private durable stage/generation
# ledger. Process-local tasks may only wake the persisted lease.
run_replacement_recovery_contract="$(cat \
	internal/modules/blocks/run/pkg/models/run.go \
	internal/modules/blocks/run/biz/owner.go \
	internal/modules/blocks/run/biz/replacement_internal_event.go \
	internal/modules/blocks/run/biz/workspace_execution_intent.go)"
for marker in ReplacementRecoveryLease ReplacementRecoveryClarificationResolution ReplacementRecoveryAdministrativeTermination ReplacementRecoverySuccessorMaterialization ReplacementRecoveryScheduled ReplacementRecoveryRunning; do
	if ! grep -q -F "$marker" <<<"$run_replacement_recovery_contract"; then
		echo "boundary contract violation: Run-owned replacement recovery lost $marker" >&2
		exit 1
	fi
done
for marker in replacementRecoveryCtx replacementRecoveryCancel replacementRecoveryWG beginReplacementRecoveryGeneration; do
	if ! grep -q -F "$marker" <<<"$run_replacement_recovery_contract"; then
		echo "boundary contract violation: Run replacement wakeup is not bound to owner Stop/drain ($marker)" >&2
		exit 1
	fi
done
if ! grep -q -F 'TestReplacementRecoveryStopCancelsWakeupAndFreshOwnerRemountsPersistedLease' internal/modules/blocks/run/biz/replacement_termination_delivery_test.go; then
	echo "boundary contract violation: Run replacement Stop/restart wakeup regression is missing" >&2
	exit 1
fi

# Workorchd is an admission adapter, not another governance state owner. New
# open/pending transitions must intersect the concrete resource with typed
# Workspace/Run owner reads and the exact canonical blocker before any owner
# write. Dedicated Definition tickets never enter generic mutation routes;
# terminal response-loss retries remain owner-adjudicated.
workorchd_governance_mutation_admission="$(cat internal/modules/application/workorchd/biz/appservice/governanceports/mutation_admission.go)"
workorchd_definition_classifier="$(sed -n '/^func definitionDecisionUsesDedicatedAdmission(/,/^}/p' internal/modules/application/workorchd/biz/appservice/governanceports/mutation_admission.go)"
if ! grep -q -F 'ReadWorkspaceByOwnerCommand' <<<"$workorchd_governance_mutation_admission" ||
	! grep -q -F 'ReadRunByWorkspaceOwnerCommand' <<<"$workorchd_governance_mutation_admission" ||
	! grep -q -F 'RunStateBlockedOnDecision' <<<"$workorchd_governance_mutation_admission" ||
	! grep -q -F 'run.BlockingTicketID != ticket.ID' <<<"$workorchd_governance_mutation_admission" ||
	! grep -q -F 'RunStateBlockedOnApproval' <<<"$workorchd_governance_mutation_admission" ||
	! grep -q -F 'run.BlockingApprovalID != item.ID' <<<"$workorchd_governance_mutation_admission" ||
	! grep -q -F 'definitionDecisionUsesDedicatedAdmission' <<<"$workorchd_governance_mutation_admission" ||
	! grep -q -F 'response-loss retry' <<<"$workorchd_governance_mutation_admission"; then
	echo "boundary contract violation: Workorchd governance mutation admission lost canonical owner intersection, dedicated Definition exclusion, or owner idempotency" >&2
	exit 1
fi
if ! grep -q -F 'ticket.DefinitionClarification != nil' <<<"$workorchd_definition_classifier" ||
	grep -q -E -- 'DecisionKind|FailureClass|Reason|Question|Context' <<<"$workorchd_definition_classifier"; then
	echo "boundary contract violation: Workorchd rebuilt Definition authority from text instead of the current typed binding" >&2
	exit 1
fi
for governance_file in \
	internal/modules/application/workorchd/biz/appservice/governanceports/decision_resolve_views.go \
	internal/modules/application/workorchd/biz/appservice/governanceports/decision_batch_resolve_views.go; do
	if ! grep -q -F 'admitDecisionMutation' "$governance_file"; then
		echo "boundary contract violation: Decision mutation bypasses canonical admission: $governance_file" >&2
		exit 1
	fi
done
for governance_file in \
	internal/modules/application/workorchd/biz/appservice/governanceports/approval_resolve_views.go \
	internal/modules/application/workorchd/biz/appservice/governanceports/approval_batch_resolve_views.go; do
	if ! grep -q -F 'admitApprovalMutation' "$governance_file"; then
		echo "boundary contract violation: Approval mutation bypasses canonical admission: $governance_file" >&2
		exit 1
	fi
done
if ! grep -q -F 'requireCanonicalDecisionMutationTargets' web/workorch-console/src/pages/DecisionQueuePage.tsx ||
	! grep -q -F 'requireCanonicalApprovalMutationTargets' web/workorch-console/src/pages/ApprovalQueuePage.tsx ||
	! grep -q -F 'canonicalDefinitionClarification(snapshot)' web/workorch-console/src/helpers.tsx ||
	grep -q -F 'governanceDisplay' < <(sed -n '/^export function canonicalDefinitionClarification(/,/^}/p' web/workorch-console/src/governanceMutationTarget.ts); then
	echo "boundary contract violation: Web governance/definition mutation target is not canonical or regained display authority" >&2
	exit 1
fi

# Definition clarification crosses Workorchd only as a bounded typed owner
# projection. Mutable refs must detach, and UI command admission must not be
# reconstructed from legacy text, display hints, or derived Goal/DoD freshness.
definition_projection_contract="$(sed -n '/^type DefinitionClarification struct {/,/^}/p' internal/pkg/workorchprojectioncontract/governance.go)"
definition_projection_clone="$(sed -n '/^func CloneDefinitionClarification(/,/^}/p' internal/pkg/workorchprojectioncontract/governance.go)"
definition_projection_mapper="$(sed -n '/^func mapDefinitionClarification(/,/^}/p' internal/modules/application/workorchd/biz/appservice/readmodelprojection/decision_projection.go)"
if ! grep -q -F 'DefinitionClarification *DefinitionClarification' internal/pkg/workorchprojectioncontract/governance.go ||
	! grep -q -F 'OffendingRefs' <<<"$definition_projection_contract" ||
	! grep -q -F 'AuthoritySnapshotID' <<<"$definition_projection_contract" ||
	! grep -q -F 'ClarificationDispatchID' <<<"$definition_projection_contract" ||
	! grep -q -F 'ret.OffendingRefs = append([]string(nil), item.OffendingRefs...)' <<<"$definition_projection_clone" ||
	! grep -q -F 'OffendingRefs:' <<<"$definition_projection_mapper" ||
	! grep -q -F 'mapDefinitionClarification(item.DefinitionClarification)' internal/modules/application/workorchd/biz/appservice/readmodelprojection/decision_projection.go ||
	! grep -q -F 'item.Decisions = projectioncontract.CloneDecisions(item.Decisions)' internal/modules/application/workorchd/biz/appservice/workspaceviewcore/store_clone.go ||
	! grep -q -F 'snapshot.Decisions = projectioncontract.CloneDecisions(source.Decisions)' internal/modules/application/workorchd/biz/appservice/workspaceviewcore/store_query.go ||
	! grep -q -F 'TestMapDecisionProjectsRuntimeDefinitionClarificationWithoutLoss' internal/modules/application/workorchd/biz/appservice/readmodelprojection/decision_projection_test.go ||
	! grep -q -F 'TestStoreDefinitionClarificationReadsDetachBindingAndOffendingRefs' internal/modules/application/workorchd/biz/appservice/workspaceviewcore/store_clone_test.go ||
	! grep -q -F 'TestDecisionReadEndpointsProjectTypedRuntimeDefinitionClarification' internal/modules/application/workorchd/service/server/http_test.go; then
	echo "boundary contract violation: Definition clarification lost its bounded typed/deep-cloned Workorchd read projection" >&2
	exit 1
fi
web_definition_selector="$(sed -n '/^export function canonicalDefinitionClarification(/,/^}/p' web/workorch-console/src/governanceMutationTarget.ts)"
web_definition_intent_gate="$(sed -n '/^export function workspaceExecutionIntentAllowed(/,/^}/p' web/workorch-console/src/governanceMutationTarget.ts)"
web_definition_classifier="$(sed -n '/^export function isDedicatedDefinitionDecision(/,/^}/p' web/workorch-console/src/governanceMutationTarget.ts)"
if grep -q -E -- 'governanceDisplay|decisionKind|failureClass|\.context' <<<"$web_definition_selector" ||
	grep -q -E -- 'decisionKind|failureClass|reason|question|context' <<<"$web_definition_classifier" ||
	! grep -q -F 'item?.definitionClarification' <<<"$web_definition_classifier" ||
	grep -q -E -- 'currentDefinitionRevision|\.goals|\.dods|governanceDisplay' <<<"$web_definition_intent_gate" ||
	! grep -q -F "return intent === 'restart'" <<<"$web_definition_intent_gate" ||
	! grep -q -F 'workspaceBoardExecutionIntentAllowed' web/workorch-console/src/helpers.tsx ||
	! grep -q -F 'onOpenWorkspace(workspace.id)' web/workorch-console/src/pages/WorkspaceBoardPage.tsx ||
	! grep -q -F "requestWorkspaceActivation('restart')" web/workorch-console/src/pages/WorkspaceDetailPage.tsx ||
	! grep -q -F 'Board always navigates for Definition clarification' web/workorch-console/tests/governanceMutationTarget.test.ts ||
	! grep -q -F 'display changes do not enable or disable explicit Restart' web/workorch-console/tests/governanceMutationTarget.test.ts; then
	echo "boundary contract violation: Web Definition navigation/explicit Restart boundary depends on display freshness or regained Board execution" >&2
	exit 1
fi
tui_definition_instruction="$(sed -n '/^func definitionClarificationInstruction(/,/^}/p' internal/services/workorch/service/tui/model.go)"
tui_definition_classifier="$(sed -n '/^func decisionRequiresDedicatedDefinitionFlow(/,/^}/p' internal/services/workorch/service/tui/model.go)"
if ! grep -q -F 'item.DefinitionClarification' <<<"$tui_definition_instruction" ||
	! grep -q -F 'binding.ViolationOrGapCode' <<<"$tui_definition_instruction" ||
	! grep -q -F 'binding.OffendingRefs' <<<"$tui_definition_instruction" ||
	! grep -q -F 'binding.RequiredAction' <<<"$tui_definition_instruction" ||
	grep -q -E -- 'item\.(Context|DecisionKind|FailureClass|Reason|Question)' <<<"$tui_definition_instruction"; then
	echo "boundary contract violation: TUI Definition instruction is not rendered exclusively from the typed binding" >&2
	exit 1
fi
if ! grep -q -F 'decisionHasTypedDefinitionClarification(item)' <<<"$tui_definition_classifier" ||
	grep -q -E -- 'DecisionKind|FailureClass|Reason|Question|Context' <<<"$tui_definition_classifier"; then
	echo "boundary contract violation: TUI rebuilt Definition authority from text instead of the current typed binding" >&2
	exit 1
fi

# Governance resolution already enters the owners' persisted continuation
# lane. UI success/help copy must wait for projection refresh and surface a
# delivery diagnostic instead of teaching users to submit a second Resume.
approval_queue_copy="$(cat web/workorch-console/src/pages/ApprovalQueuePage.tsx)"
model_selection_governance_copy="$(sed -n '/switch m.activeWorkspacePromptMode()/,/^\t}/p' internal/services/workorch/service/tui/model_selection.go)"
if grep -q -E -- '如关联运行.*恢复运行|唯一用户继续命令为.*恢复运行' <<<"$approval_queue_copy" ||
	grep -q -F -- 'then use /run resume' <<<"$model_selection_governance_copy" ||
	! grep -q -F 'governanceContinuationGuidance' <<<"$approval_queue_copy" ||
	! grep -q -F 'resolution delivery diagnostic' <<<"$model_selection_governance_copy" ||
	! grep -q -F 'TestModelSelectionGovernanceGuidanceWaitsForOwnerContinuation' internal/services/workorch/service/tui/model_selection_test.go ||
	! grep -q -F 'governance success guidance waits for owner continuation instead of scheduling Resume' web/workorch-console/tests/governanceMutationTarget.test.ts; then
	echo "boundary contract violation: governance UI copy regained a client-scheduled Resume continuation" >&2
	exit 1
fi

# A governance display handoff cannot select a concrete Decision/Approval
# mutation target. TUI resolution requires the exact RuntimeView-selected
# resource to remain present in the owner list and canonical Workspace
# activeRunId scope; it cannot fall back to the first same-Run resource.
tui_governance_target_selection="$({
	sed -n '/^func (m \*AppModel) activeBlockingDecision(/,/^}/p' internal/services/workorch/service/tui/model.go
	sed -n '/^func (m \*AppModel) activeBlockingApproval(/,/^}/p' internal/services/workorch/service/tui/model.go
	sed -n '/^func (m \*AppModel) decisionMatchesActiveScope(/,/^}/p' internal/services/workorch/service/tui/model.go
	sed -n '/^func (m \*AppModel) approvalMatchesActiveScope(/,/^}/p' internal/services/workorch/service/tui/model.go
})"
tui_governance_scope_source="$(sed -n '/^func (m \*AppModel) canonicalGovernanceMutationScope(/,/^}/p' internal/services/workorch/service/tui/model.go)"
if grep -q -E -- 'activeGovernanceDisplay|GovernanceDisplay|\.DecisionID|\.ApprovalID' <<<"$tui_governance_target_selection" ||
	grep -q -E -- 'filterOpenDecisions\(m\.decisions\)|filterPendingApprovals\(m\.approvals\)' <<<"$tui_governance_target_selection" ||
	! grep -q -F 'canonicalBlockingTicketID' <<<"$tui_governance_target_selection" ||
	! grep -q -F 'canonicalBlockingApprovalID' <<<"$tui_governance_target_selection" ||
	! grep -q -F 'containsOpenDecisionIdentity(m.decisions, *m.blockingDecision)' <<<"$tui_governance_target_selection" ||
	! grep -q -F 'containsPendingApprovalIdentity(m.approvals, *m.blockingApproval)' <<<"$tui_governance_target_selection" ||
	! grep -q -F 'canonicalGovernanceMutationScope()' <<<"$tui_governance_target_selection" ||
	! grep -q -F 'activeWorkspaceOverview()' <<<"$tui_governance_scope_source" ||
	! grep -q -F 'workspace.ActiveRunID' <<<"$tui_governance_scope_source"; then
	echo "boundary contract violation: TUI governance mutation target is not canonical owner scoped" >&2
	exit 1
fi
if source_matches 'func \(m \*AppModel\) setBlocking(Decision|Approval)\(' internal/services/workorch/service/tui | grep -q .; then
	echo "boundary contract violation: TUI synthesizes a concrete governance resource from a Run/display reference" >&2
	exit 1
fi
workorchd_tui_resume_gate="$({
	sed -n '/^func (m \*AppModel) activeWorkspaceBlockerProgress(/,/^}/p' internal/services/workorch/service/tui/workspace_state.go
	sed -n '/^func (m \*AppModel) blockActiveWorkspaceResume(/,/^}/p' internal/services/workorch/service/tui/workspace_state.go
})"
if grep -q -E -- 'activeGovernance|GovernanceDisplay|PromptMode|promptMode' <<<"$workorchd_tui_resume_gate" ||
	! grep -q -F 'activeBlockingDecision()' <<<"$workorchd_tui_resume_gate" ||
	! grep -q -F 'activeDefinitionClarificationDecision()' <<<"$workorchd_tui_resume_gate" ||
	! grep -q -F 'activeBlockingApproval()' <<<"$workorchd_tui_resume_gate"; then
	echo "boundary contract violation: display-only governance projection can suppress TUI Resume" >&2
	exit 1
fi
if ! grep -q -F 'TestModelGovernanceDisplayCannotSelectDecisionMutationTarget' internal/services/workorch/service/tui/model_test.go ||
	! grep -q -F 'TestModelGovernanceDisplayCannotSelectApprovalMutationTarget' internal/services/workorch/service/tui/model_test.go ||
	! grep -q -F 'TestModelGovernanceDisplayCannotBlockResumeWithoutCanonicalBlocker' internal/services/workorch/service/tui/model_test.go ||
	! grep -q -F 'TestModelDefinitionClarificationDisablesGenericMutationAndResume' internal/services/workorch/service/tui/model_test.go ||
	! grep -q -F 'TestEqualRevisionDefinitionClassificationRevokesGenericMutationTarget' internal/services/workorch/service/tui/runtime_view_test.go ||
	! grep -q -F 'TestDecisionMutationEndpointsRejectDefinitionTicketsWithoutMutation' internal/modules/application/workorchd/service/server/http_test.go ||
	! grep -q -F 'TestBatchResolveApprovalsWithBackendAcceptsExactResponseLossDuplicate' internal/modules/application/workorchd/biz/appservice/approval_service_test.go ||
	! grep -q -F 'TestWorkspaceRuntimeViewBlockersRequireCanonicalRunReference' internal/modules/application/workorchd/biz/appservice/readmodelprojection/workspace_runtime_view_test.go; then
	echo "boundary contract violation: TUI display-forgery governance mutation regressions are missing" >&2
	exit 1
fi
resolve_approval_tui="$(sed -n '/^func ResolveApproval(/,/^}/p' internal/services/workorch/service/tui/commands.go)"
if grep -q -F 'Approval().Get' <<<"$resolve_approval_tui" ||
	! grep -q -F 'TestResolveApprovalDoesNotReclassifyOwnerConflictFromTerminalReadback' internal/services/workorch/service/tui/model_test.go ||
	! grep -q -F 'TestModelProjectedApprovalHandoffWithoutResourceBlocksMutation' internal/services/workorch/service/tui/model_test.go; then
	echo "boundary contract violation: TUI re-adjudicates Approval conflict or bypasses the Workspace-owned approval handoff" >&2
	exit 1
fi

# Run state metrics/cache invalidation accepts only the concrete Run owner's
# validated notification. Eventstream replay and forged sources remain audit
# or display input only.
run_state_handler="$(sed -n '/^func (b \*Biz) handleRunStateChanged(/,/^}/p' internal/modules/application/workorchd/biz/owner.go)"
run_state_recorder="$(sed -n '/^func (s \*serverState) RecordRunStateChanged(/,/^}/p' internal/modules/application/workorchd/service/server/event_subscriptions.go)"
if ! grep -q -F 'event.Source() == "blocks/run"' <<<"$run_state_handler" ||
	! grep -q -F 'data.Validate() == nil' <<<"$run_state_handler" ||
	! grep -q -F 'data.Validate() != nil' <<<"$run_state_recorder"; then
	echo "boundary contract violation: RunStateChanged cache/metrics ingress lost source or typed validation gate" >&2
	exit 1
fi

# Orchestrator evidence repositories form one backend-owned capability set.
# Production callers may obtain that set only from the memory/file factories;
# injecting raw repositories or a differently rooted purger would separate the
# permanent tombstone from the resources it is meant to fence and purge.
orchestrator_evidence_constructor_files="$(grep -R -l -F --include='*.go' -- 'NewOrchestratorEvidenceOwnerRepositories(' internal | sort)"
expected_orchestrator_evidence_constructor_files="$(printf '%s\n' \
	internal/pkg/repository/memory/orchestrator_workspace_deletion.go \
	internal/pkg/repository/orchestrator_evidence_fence.go \
	internal/pkg/workorchrepo/workspace_scoped_repositories.go | sort)"
if [[ "$orchestrator_evidence_constructor_files" != "$expected_orchestrator_evidence_constructor_files" ]]; then
	echo "boundary contract violation: Orchestrator evidence owner set gained a non-canonical constructor callsite" >&2
	exit 1
fi
orchestrator_evidence_backend_files="$(grep -R -l -F --include='*.go' -- 'OrchestratorWorkspaceDeletions()' internal | sort)"
expected_orchestrator_evidence_backend_files="$(printf '%s\n' \
	internal/pkg/repository/memory/orchestrator_workspace_deletion.go \
	internal/pkg/repository/orchestrator_evidence_fence.go \
	internal/pkg/workorchrepo/workspace_scoped_repositories.go | sort)"
if [[ "$orchestrator_evidence_backend_files" != "$expected_orchestrator_evidence_backend_files" ]]; then
	echo "boundary contract violation: Orchestrator evidence owner set gained a non-canonical backend implementation" >&2
	exit 1
fi

# Result, Decision, Approval, Memory, and Eventstream owner repositories are
# admitted only as backend-minted opaque bundles. Production owner Biz inputs
# cannot accept separately assembled repositories, and bundle constructors,
# binding minting, and backend aggregate implementations stay confined to the
# repository contract plus the memory/file backend factories.
resource_owner_bundle_files="$({
	grep -R -l -E --include='*.go' -- 'New(Result|Decision|Approval|Memory|EventStream)OwnerRepositories\(' internal || true
} | grep -v '_test\.go$' | sort)"
expected_resource_owner_bundle_files="$(printf '%s\n' \
	internal/pkg/repository/memory/memory.go \
	internal/pkg/repository/resource_owner_repositories.go \
	internal/pkg/workorchrepo/workspace_scoped_repositories.go | sort)"
if [[ "$resource_owner_bundle_files" != "$expected_resource_owner_bundle_files" ]]; then
	echo "boundary contract violation: resource owner bundle gained a non-canonical production constructor callsite" >&2
	exit 1
fi
resource_owner_binding_files="$({
	grep -R -l -E --include='*.go' -- 'New(Result|Decision|Approval|Memory|EventStream)OwnerRepositoryBinding\(' internal || true
} | grep -v '_test\.go$' | sort)"
if [[ "$resource_owner_binding_files" != "$expected_resource_owner_bundle_files" ]]; then
	echo "boundary contract violation: resource owner binding gained a non-canonical production mint callsite" >&2
	exit 1
fi
resource_owner_backend_files="$({
	grep -R -l -E --include='*.go' -- '(ResultOwnerResults|DecisionOwnerTickets|ApprovalOwnerApprovals|MemoryOwnerItems|EventStreamOwnerEventRecords)\(\)' internal || true
} | grep -v '_test\.go$' | sort)"
if [[ "$resource_owner_backend_files" != "$expected_resource_owner_bundle_files" ]]; then
	echo "boundary contract violation: resource owner bundle gained a non-canonical production backend implementation" >&2
	exit 1
fi
if grep -R -n -E --include='*.go' --exclude='*_test.go' -- '^[[:space:]]+(Results|Artifacts|Tickets|Resolutions|Approvals|MemoryItems|MemoryRecallTraces|EventRecords|WorkspaceDeletions|WorkspacePurger)[[:space:]]+repository\.' \
	internal/modules/blocks/{result,decision,approval,memory,eventstream}/biz | grep -q .; then
	echo "boundary contract violation: resource owner Biz Dependencies accept independently supplied repositories" >&2
	exit 1
fi

echo "boundary contract verification passed"
