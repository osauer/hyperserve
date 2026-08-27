#!/bin/sh
set -eu

cd "$(dirname "$0")/.."

root=$(pwd)
gate=$root/scripts/wait-exact-sha-ci.sh
fake=$root/scripts/testdata/fake-gh.sh
sha=0123456789abcdef0123456789abcdef01234567
run_id=987654
tmp=$(mktemp -d -t hyperserve-release-gate.XXXXXX)
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
mkdir "$tmp/bin"
ln -s "$fake" "$tmp/bin/gh"

run_fixture() {
  env \
    RELEASE_GATE_TEST_FIXTURE=1 \
    CI_MAX_POLLS=1 \
    CI_POLL_SECONDS=0 \
    GH_BIN="$fake" \
    GH_REPO=redirected/example \
    RELEASE_SHA="$sha" \
    FAKE_RUN_ID="$run_id" \
    FAKE_EXPECT_RUN_ID="$run_id" \
    "$gate"
}

expect_fixture_failure() {
  name=$1
  shift
  if env \
    RELEASE_GATE_TEST_FIXTURE=1 \
    CI_MAX_POLLS=1 \
    CI_POLL_SECONDS=0 \
    GH_BIN="$fake" \
    GH_REPO=redirected/example \
    RELEASE_SHA="$sha" \
    "$@" \
    "$gate" >/dev/null 2>&1; then
    echo "release-gate-test: expected failure: $name" >&2
    exit 1
  fi
}

run_fixture >/dev/null

# Non-canonical authority is available only when the script is called directly
# in explicit fixture mode.
env \
  RELEASE_GATE_TEST_FIXTURE=1 \
  RELEASE_REPOSITORY=fixture-owner/fixture-repo \
  RELEASE_MAIN_BRANCH=fixture-main \
  RELEASE_WORKFLOW=fixture.yml \
  'RELEASE_REQUIRED_JOBS=FixtureTest FixtureBenchmark' \
  CI_MAX_POLLS=1 \
  CI_POLL_SECONDS=0 \
  GH_BIN="$fake" \
  RELEASE_SHA="$sha" \
  FAKE_EXPECT_REPO=fixture-owner/fixture-repo \
  FAKE_EXPECT_BRANCH=fixture-main \
  FAKE_EXPECT_WORKFLOW=fixture.yml \
  FAKE_HEAD_BRANCH=fixture-main \
  'FAKE_JOBS=FixtureTest\tsuccess\nFixtureBenchmark\tsuccess\n' \
  "$gate" >/dev/null

expect_fixture_failure "missing run" FAKE_RUN_ID=
expect_fixture_failure "watch failure" FAKE_WATCH_EXIT=1
expect_fixture_failure "wrong SHA" FAKE_HEAD_SHA=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
expect_fixture_failure "wrong branch" FAKE_HEAD_BRANCH=feature
expect_fixture_failure "pull request run" FAKE_EVENT=pull_request
expect_fixture_failure "incomplete run" FAKE_STATUS=in_progress FAKE_CONCLUSION=
expect_fixture_failure "missing required job" 'FAKE_JOBS=Test\tsuccess\n'
expect_fixture_failure "duplicate required job" 'FAKE_JOBS=Test\tsuccess\nTest\tsuccess\nBenchmark\tsuccess\n'
expect_fixture_failure "failed job" 'FAKE_JOBS=Test\tsuccess\nBenchmark\tsuccess\nLint\tfailure\n'

if env \
  RELEASE_GATE_TEST_FIXTURE=1 \
  CI_MAX_POLLS=1 \
  CI_POLL_SECONDS=0 \
  GH_BIN="$fake" \
  RELEASE_SHA=01234567 \
  "$gate" >/dev/null 2>&1; then
  echo "release-gate-test: expected failure: abbreviated SHA" >&2
  exit 1
fi

# Production mode resolves the literal `gh` command and ignores every ambient
# authority override. The fake executable named gh asserts the complete query
# and every subsequent run ID/repository binding.
env \
  PATH="$tmp/bin:$PATH" \
  GH_REPO=redirected/example \
  GH_HOST=example.invalid \
  FAKE_RUN_ID="$run_id" \
  FAKE_EXPECT_RUN_ID="$run_id" \
  make --no-print-directory release-ci \
    RELEASE_SHA="$sha" \
    RELEASE_GATE_TEST_FIXTURE=1 \
    GH_BIN=/bin/false \
    RELEASE_REPOSITORY=redirected/example \
    RELEASE_MAIN_BRANCH=feature \
    RELEASE_WORKFLOW=redirected.yml \
    RELEASE_REQUIRED_JOBS=Test >/dev/null

# A command-line attempt to narrow the canonical required-job set must not turn
# a Test-only run into a passing release gate.
if env \
  PATH="$tmp/bin:$PATH" \
  GH_REPO=redirected/example \
  GH_HOST=example.invalid \
  FAKE_RUN_ID="$run_id" \
  FAKE_EXPECT_RUN_ID="$run_id" \
  'FAKE_JOBS=Test\tsuccess\n' \
  make --no-print-directory release-ci \
    RELEASE_SHA="$sha" \
    RELEASE_GATE_TEST_FIXTURE=1 \
    GH_BIN=/bin/false \
    RELEASE_REPOSITORY=redirected/example \
    RELEASE_MAIN_BRANCH=feature \
    RELEASE_WORKFLOW=redirected.yml \
    RELEASE_REQUIRED_JOBS=Test >/dev/null 2>&1; then
  echo "release-gate-test: command-line job narrowing unexpectedly passed" >&2
  exit 1
fi

# Both effective origin directions must stay inside the canonical repository.
make --no-print-directory release-authority-check >/dev/null
authority_repo=$tmp/authority-repo
git init -q "$authority_repo"
git -C "$authority_repo" remote add origin https://github.com/osauer/hyperserve.git
git -C "$authority_repo" remote set-url --add --push origin git@github.com:osauer/hyperserve.git
make -C "$authority_repo" -f "$root/Makefile" --no-print-directory release-authority-check >/dev/null

git -C "$authority_repo" remote set-url origin https://example.invalid/osauer/hyperserve.git
if make -C "$authority_repo" -f "$root/Makefile" --no-print-directory release-authority-check >/dev/null 2>&1; then
  echo "release-gate-test: non-canonical fetch URL unexpectedly passed" >&2
  exit 1
fi

git -C "$authority_repo" remote set-url origin https://github.com/osauer/hyperserve.git
git -C "$authority_repo" remote set-url --add --push origin https://example.invalid/osauer/hyperserve.git
if make -C "$authority_repo" -f "$root/Makefile" --no-print-directory release-authority-check >/dev/null 2>&1; then
  echo "release-gate-test: non-canonical push URL unexpectedly passed" >&2
  exit 1
fi

echo "release-gate-test: PASS"
