#!/bin/sh
# wait-exact-sha-ci.sh — prove the required push CI jobs for one immutable SHA.
set -eu

sha=${RELEASE_SHA:-}
workflow=${RELEASE_WORKFLOW:-ci.yml}
required_jobs=${RELEASE_REQUIRED_JOBS:-Test Benchmark}
gh_bin=${GH_BIN:-gh}
max_polls=${CI_MAX_POLLS:-60}
poll_seconds=${CI_POLL_SECONDS:-10}

case "$sha" in
  ''|*[!0-9a-fA-F]*)
    echo "release-ci: RELEASE_SHA must be a full hexadecimal commit SHA" >&2
    exit 1
    ;;
esac

if [ "${#sha}" -ne 40 ]; then
  echo "release-ci: RELEASE_SHA must be a full 40-character commit SHA" >&2
  exit 1
fi

command -v "$gh_bin" >/dev/null 2>&1 || {
  echo "release-ci: gh CLI not on PATH; install gh" >&2
  exit 1
}

case "$max_polls" in
  ''|*[!0-9]*)
    echo "release-ci: CI_MAX_POLLS must be a positive integer" >&2
    exit 1
    ;;
esac
if [ "$max_polls" -le 0 ]; then
  echo "release-ci: CI_MAX_POLLS must be a positive integer" >&2
  exit 1
fi

run_id=
poll=1
while [ "$poll" -le "$max_polls" ]; do
  run_id=$("$gh_bin" run list \
    --workflow "$workflow" \
    --event push \
    --commit "$sha" \
    --limit 20 \
    --json databaseId,headSha,event,createdAt \
    --jq ".[] | select(.headSha == \"$sha\" and .event == \"push\") | .databaseId" |
    sed -n '1p')
  [ -n "$run_id" ] && break
  if [ "$poll" -lt "$max_polls" ]; then
    sleep "$poll_seconds"
  fi
  poll=$((poll + 1))
done

if [ -z "$run_id" ]; then
  echo "release-ci: no $workflow push run found for $sha" >&2
  exit 1
fi

echo "release-ci: waiting for $workflow run $run_id at $sha"
if ! "$gh_bin" run watch "$run_id" --exit-status; then
  echo "release-ci: run $run_id did not succeed" >&2
  exit 1
fi

actual_sha=$("$gh_bin" run view "$run_id" --json headSha --jq .headSha)
event=$("$gh_bin" run view "$run_id" --json event --jq .event)
status=$("$gh_bin" run view "$run_id" --json status --jq .status)
conclusion=$("$gh_bin" run view "$run_id" --json conclusion --jq .conclusion)
url=$("$gh_bin" run view "$run_id" --json url --jq .url)

if [ "$actual_sha" != "$sha" ]; then
  echo "release-ci: run $run_id headSha is $actual_sha, expected $sha" >&2
  exit 1
fi
if [ "$event" != push ]; then
  echo "release-ci: run $run_id event is $event, expected push" >&2
  exit 1
fi
if [ "$status" != completed ] || [ "$conclusion" != success ]; then
  echo "release-ci: run $run_id is $status/$conclusion, expected completed/success" >&2
  exit 1
fi

jobs=$("$gh_bin" run view "$run_id" --json jobs \
  --template '{{range .jobs}}{{printf "%s\t%s\n" .name .conclusion}}{{end}}')
if [ -z "$jobs" ]; then
  echo "release-ci: run $run_id returned no jobs" >&2
  exit 1
fi

bad_jobs=$(printf '%s\n' "$jobs" | awk -F '\t' '$2 != "success" { print $1 "=" $2 }')
if [ -n "$bad_jobs" ]; then
  echo "release-ci: run $run_id contains non-successful jobs:" >&2
  printf '%s\n' "$bad_jobs" >&2
  exit 1
fi

for required in $required_jobs; do
  matches=$(printf '%s\n' "$jobs" | awk -F '\t' -v required="$required" '
    $1 == required && $2 == "success" { count++ }
    END { print count + 0 }
  ')
  if [ "$matches" -ne 1 ]; then
    echo "release-ci: required job '$required' succeeded $matches times; expected exactly once" >&2
    exit 1
  fi
done

echo "release-ci: verified run=$run_id headSha=$actual_sha event=$event status=$status conclusion=$conclusion"
echo "release-ci: $url"
