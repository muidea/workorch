#!/usr/bin/env bash

# run_go_test executes a focused Go test command and rejects a successful
# command that did not select any tests. Go otherwise reports that case as a
# package-level success, which makes stale verification scripts silently green.
run_go_test() {
	local output
	if ! output="$(go test -v "$@" 2>&1)"; then
		printf '%s\n' "$output" >&2
		return 1
	fi
	printf '%s\n' "$output"
	if ! grep -q -E '^=== RUN[[:space:]]+Test' <<<"$output"; then
		printf 'verification error: focused go test selected no tests: go test %s\n' "$*" >&2
		return 1
	fi
}
