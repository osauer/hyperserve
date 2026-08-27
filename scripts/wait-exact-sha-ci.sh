#!/bin/sh
# wait-exact-sha-ci.sh — prove the required push CI jobs for one immutable SHA.
set -eu

sha=${RELEASE_SHA:-}
max_polls=${CI_MAX_POLLS:-60}
poll_seconds=${CI_POLL_SECONDS:-10}

# Release authority is immutable in production. Tests may substitute a fake CLI
# and fixture values only through the explicit fixture switch; canonical Make
# targets force that switch off.
case "${RELEASE_GATE_TEST_FIXTURE:-0}" in
  0|'')
    repository=osauer/hyperserve
    main_branch=main
    workflow=ci.yml
    required_jobs='Test Benchmark'
    gh_bin=gh
    ;;
  1)
    repository=${RELEASE_REPOSITORY:-osauer/hyperserve}
    main_branch=${RELEASE_MAIN_BRANCH:-main}
    workflow=${RELEASE_WORKFLOW:-ci.yml}
    required_jobs=${RELEASE_REQUIRED_JOBS:-Test Benchmark}
    gh_bin=${GH_BIN:-gh}
    ;;
  *)
    echo "release-ci: RELEASE_GATE_TEST_FIXTURE must be 0 or 1" >&2
    exit 1
    ;;
esac

# gh otherwise treats these ambient variables as repository authority. Every
# invocation below also carries an explicit --repo binding.
unset GH_REPO GH_HOST

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
    --repo "$repository" \
    --workflow "$workflow" \
    --branch "$main_branch" \
    --event push \
    --commit "$sha" \
    --limit 20 \
    --json databaseId,headSha,headBranch,event,createdAt \
    --jq ".[] | select(.headSha == \"$sha\" and .headBranch == \"$main_branch\" and .event == \"push\") | .databaseId" |
    sed -n '1p')
  [ -n "$run_id" ] && break
  if [ "$poll" -lt "$max_polls" ]; then
    sleep "$poll_seconds"
  fi
  poll=$((poll + 1))
done

if [ -z "$run_id" ]; then
  echo "release-ci: no $repository $workflow push run on $main_branch found for $sha" >&2
  exit 1
fi

echo "release-ci: waiting for $repository $workflow run $run_id on $main_branch at $sha"
if ! "$gh_bin" run watch "$run_id" --repo "$repository" --exit-status; then
  echo "release-ci: run $run_id did not succeed" >&2
  exit 1
fi

actual_sha=$("$gh_bin" run view "$run_id" --repo "$repository" --json headSha --jq .headSha)
actual_branch=$("$gh_bin" run view "$run_id" --repo "$repository" --json headBranch --jq .headBranch)
event=$("$gh_bin" run view "$run_id" --repo "$repository" --json event --jq .event)
status=$("$gh_bin" run view "$run_id" --repo "$repository" --json status --jq .status)
conclusion=$("$gh_bin" run view "$run_id" --repo "$repository" --json conclusion --jq .conclusion)
url=$("$gh_bin" run view "$run_id" --repo "$repository" --json url --jq .url)

if [ "$actual_sha" != "$sha" ]; then
  echo "release-ci: run $run_id headSha is $actual_sha, expected $sha" >&2
  exit 1
fi
if [ "$actual_branch" != "$main_branch" ]; then
  echo "release-ci: run $run_id headBranch is $actual_branch, expected $main_branch" >&2
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

jobs=$("$gh_bin" run view "$run_id" --repo "$repository" --json jobs \
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

echo "release-ci: verified repo=$repository workflow=$workflow branch=$actual_branch run=$run_id headSha=$actual_sha event=$event status=$status conclusion=$conclusion"
echo "release-ci: $url"
