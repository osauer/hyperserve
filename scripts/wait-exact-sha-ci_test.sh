#!/bin/sh
set -eu

cd "$(dirname "$0")/.."

gate=./scripts/wait-exact-sha-ci.sh
fake=./scripts/testdata/fake-gh.sh
sha=0123456789abcdef0123456789abcdef01234567

run_gate() {
  CI_MAX_POLLS=1 CI_POLL_SECONDS=0 GH_BIN="$fake" RELEASE_SHA="$sha" "$gate"
}

expect_failure() {
  name=$1
  shift
  if env "$@" CI_MAX_POLLS=1 CI_POLL_SECONDS=0 GH_BIN="$fake" RELEASE_SHA="$sha" "$gate" >/dev/null 2>&1; then
    echo "release-gate-test: expected failure: $name" >&2
    exit 1
  fi
}

run_gate >/dev/null
expect_failure "missing run" FAKE_RUN_ID=
expect_failure "watch failure" FAKE_WATCH_EXIT=1
expect_failure "wrong SHA" FAKE_HEAD_SHA=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
expect_failure "pull request run" FAKE_EVENT=pull_request
expect_failure "incomplete run" FAKE_STATUS=in_progress FAKE_CONCLUSION=
expect_failure "missing required job" 'FAKE_JOBS=Test\tsuccess\n'
expect_failure "failed job" 'FAKE_JOBS=Test\tsuccess\nBenchmark\tsuccess\nLint\tfailure\n'

echo "release-gate-test: PASS"
