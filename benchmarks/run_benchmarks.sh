#!/usr/bin/env bash

set -Eeuo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
duration="${BENCH_DURATION:-5s}"
threads="${BENCH_THREADS:-4}"
connections="${BENCH_CONNECTIONS:-32}"
port="${BENCH_PORT:-18080}"
timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
results_dir="${BENCH_RESULTS_DIR:-${repo_root}/benchmarks/results/${timestamp}}"

fail() {
	printf 'benchmark-load: %s\n' "$*" >&2
	exit 2
}

for tool in go curl git; do
	command -v "$tool" >/dev/null 2>&1 || fail "required tool not found on PATH: ${tool}"
done

[[ "$duration" =~ ^[1-9][0-9]*(ms|s|m)$ ]] || fail "BENCH_DURATION must look like 500ms, 5s, or 1m"
[[ "$threads" =~ ^[1-9][0-9]*$ ]] || fail "BENCH_THREADS must be a positive integer"
[[ "$connections" =~ ^[1-9][0-9]*$ ]] || fail "BENCH_CONNECTIONS must be a positive integer"
[[ "$port" =~ ^[0-9]+$ ]] || fail "BENCH_PORT must be an integer"
((port >= 1024 && port <= 65535)) || fail "BENCH_PORT must be between 1024 and 65535"

run_tmp="$(mktemp -d "${TMPDIR:-/tmp}/hyperserve-benchmark.XXXXXX")"
server_pid=""

cleanup() {
	local status=$?
	trap - EXIT
	if [[ -n "$server_pid" ]] && kill -0 "$server_pid" 2>/dev/null; then
		kill "$server_pid" 2>/dev/null || true
		for _ in {1..40}; do
			kill -0 "$server_pid" 2>/dev/null || break
			sleep 0.05
		done
		if kill -0 "$server_pid" 2>/dev/null; then
			kill -KILL "$server_pid" 2>/dev/null || true
		fi
		wait "$server_pid" 2>/dev/null || true
	fi
	rm -rf "$run_tmp"
	exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
trap 'exit 129' HUP

mkdir -p "$results_dir"
[[ -z "$(ls -A "$results_dir")" ]] || fail "BENCH_RESULTS_DIR must be empty: $results_dir"

commit="$(git -C "$repo_root" rev-parse HEAD)"
tree_state="clean"
if [[ -n "$(git -C "$repo_root" status --porcelain)" ]]; then
	tree_state="dirty"
fi
go_version="$(go version)"
platform="$(uname -a)"

cat >"$results_dir/metadata.txt" <<EOF
HyperServe commit: $commit
Working tree: $tree_state
Go tool: $go_version
Platform: $platform
Load tool: benchmarks/load (Go standard library)
Duration per profile: $duration
Go execution parallelism (GOMAXPROCS): $threads
Concurrent workers (connections): $connections
Server address: 127.0.0.1:$port
Profile minimal: GET /minimal; default HyperServe middleware; request logs filtered at ERROR
Profile middleware: GET /middleware; defaults plus security headers and bearer authentication
EOF

printf 'Building maintained benchmark fixture and load tool...\n'
go -C "$repo_root" build -trimpath -o "$run_tmp/server" ./benchmarks/server
go -C "$repo_root" build -trimpath -o "$run_tmp/load" ./benchmarks/load

run_id="${run_tmp##*/}-$$"
"$run_tmp/server" -addr "127.0.0.1:$port" -run-id "$run_id" >"$results_dir/server.log" 2>&1 &
server_pid=$!

ready_url="http://127.0.0.1:$port/ready"
check_child() {
	kill -0 "$server_pid" 2>/dev/null ||
		fail "benchmark server exited; see $results_dir/server.log"
}
owns_listener() {
	[[ "$(curl --fail --silent --max-time 1 "$ready_url")" == "$run_id" ]]
}
for _ in {1..100}; do
	check_child
	if owns_listener; then
		break
	fi
	sleep 0.05
done
check_child
owns_listener || fail "benchmark child did not acquire the listener; see $results_dir/server.log"

validate_endpoints() {
	check_child
	owns_listener || fail "benchmark listener identity changed"
	[[ "$(curl --fail --silent --max-time 1 "http://127.0.0.1:$port/minimal")" == "OK" ]] || fail "unexpected minimal response"
	[[ "$(curl --fail --silent --max-time 1 -H 'Authorization: Bearer benchmark-token' "http://127.0.0.1:$port/middleware")" == "OK" ]] || fail "unexpected middleware response"
}

run_profile() {
	local name=$1
	local endpoint=$2
	shift 2
	validate_endpoints
	printf '\nRunning %s profile...\n' "$name"
	GOMAXPROCS="$threads" "$run_tmp/load" \
		-url "http://127.0.0.1:$port$endpoint" \
		-duration "$duration" \
		-workers "$connections" \
		"$@" >"$run_tmp/$name.txt"
	validate_endpoints
	sed -n '1,14p' "$run_tmp/$name.txt"
}

run_profile minimal /minimal
run_profile middleware /middleware -bearer-token benchmark-token
mv "$run_tmp/minimal.txt" "$run_tmp/middleware.txt" "$results_dir/"

printf '\nBenchmark results: %s\n' "$results_dir"
