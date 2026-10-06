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

# 创建期合同要求每个目标包都实际运行并通过测试，不允许某包掩盖另一个空选择。
run_go_test_each_package() {
	local output
	local helper_dir
	helper_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
	if ! output="$(go test -json "$@" 2>&1)"; then
		printf '%s\n' "$output" >&2
		return 1
	fi
	printf '%s\n' "$output" | python3 "$helper_dir/go-test-each-package.py"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
	if [[ "${1:-}" != "--each-package" ]]; then
		printf 'usage: %s --each-package <go test arguments>\n' "$0" >&2
		exit 2
	fi
	shift
	run_go_test_each_package "$@"
fi
