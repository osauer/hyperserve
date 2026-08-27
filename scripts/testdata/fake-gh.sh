#!/bin/sh
set -eu

fail() {
  echo "fake-gh: $*" >&2
  exit 2
}

require_pair() {
  flag=$1
  expected=$2
  shift 2
  while [ "$#" -gt 0 ]; do
    if [ "$1" = "$flag" ]; then
      [ "$#" -ge 2 ] || fail "$flag has no value"
      [ "$2" = "$expected" ] || fail "$flag is '$2', expected '$expected'"
      return 0
    fi
    shift
  done
  fail "missing $flag '$expected'"
}

require_flag() {
  expected=$1
  shift
  for arg in "$@"; do
    [ "$arg" = "$expected" ] && return 0
  done
  fail "missing $expected"
}

pair_value() {
  flag=$1
  shift
  while [ "$#" -gt 0 ]; do
    if [ "$1" = "$flag" ]; then
      [ "$#" -ge 2 ] || fail "$flag has no value"
      printf '%s\n' "$2"
      return 0
    fi
    shift
  done
  fail "missing $flag"
}

expected_repo=${FAKE_EXPECT_REPO-osauer/hyperserve}
expected_workflow=${FAKE_EXPECT_WORKFLOW-ci.yml}
expected_branch=${FAKE_EXPECT_BRANCH-main}
expected_sha=${FAKE_EXPECT_SHA-0123456789abcdef0123456789abcdef01234567}
expected_run_id=${FAKE_EXPECT_RUN_ID-${FAKE_RUN_ID-321}}

[ "$#" -ge 2 ] || fail "missing command"
command_name=$1
subcommand=$2
shift 2

case "$command_name $subcommand" in
  "run list")
    require_pair --repo "$expected_repo" "$@"
    require_pair --workflow "$expected_workflow" "$@"
    require_pair --branch "$expected_branch" "$@"
    require_pair --event push "$@"
    require_pair --commit "$expected_sha" "$@"
    require_pair --limit 20 "$@"
    require_pair --json databaseId,headSha,headBranch,event,createdAt "$@"
    expected_jq=".[] | select(.headSha == \"$expected_sha\" and .headBranch == \"$expected_branch\" and .event == \"push\") | .databaseId"
    require_pair --jq "$expected_jq" "$@"
    printf '%s\n' "${FAKE_RUN_ID-321}"
    ;;
  "run watch")
    [ "${1:-}" = "$expected_run_id" ] || fail "watch run ID is '${1:-}', expected '$expected_run_id'"
    require_pair --repo "$expected_repo" "$@"
    require_flag --exit-status "$@"
    exit "${FAKE_WATCH_EXIT:-0}"
    ;;
  "run view")
    [ "${1:-}" = "$expected_run_id" ] || fail "view run ID is '${1:-}', expected '$expected_run_id'"
    require_pair --repo "$expected_repo" "$@"
    json_field=$(pair_value --json "$@")
    case "$json_field" in
      headSha)
        require_pair --jq .headSha "$@"
        printf '%s\n' "${FAKE_HEAD_SHA-0123456789abcdef0123456789abcdef01234567}"
        ;;
      headBranch)
        require_pair --jq .headBranch "$@"
        printf '%s\n' "${FAKE_HEAD_BRANCH-main}"
        ;;
      event)
        require_pair --jq .event "$@"
        printf '%s\n' "${FAKE_EVENT-push}"
        ;;
      status)
        require_pair --jq .status "$@"
        printf '%s\n' "${FAKE_STATUS-completed}"
        ;;
      conclusion)
        require_pair --jq .conclusion "$@"
        printf '%s\n' "${FAKE_CONCLUSION-success}"
        ;;
      url)
        require_pair --jq .url "$@"
        printf '%s\n' "${FAKE_URL-https://example.invalid/actions/runs/321}"
        ;;
      jobs)
        require_pair --template '{{range .jobs}}{{printf "%s\t%s\n" .name .conclusion}}{{end}}' "$@"
        printf '%b' "${FAKE_JOBS-Test\tsuccess\nBenchmark\tsuccess\n}"
        ;;
      *)
        fail "unsupported run view JSON field: $json_field"
        ;;
    esac
    ;;
  *)
    fail "unsupported invocation: $command_name $subcommand $*"
    ;;
esac
