#!/bin/sh
set -eu

cd "$(dirname "$0")/.."
root=$(pwd)
check=$root/scripts/check-release-publish-source.sh
tmp=$(mktemp -d -t hyperserve-release-source.XXXXXX)
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
repo=$tmp/repo

git init -q "$repo"
git -C "$repo" config user.name "Release Gate Fixture"
git -C "$repo" config user.email "release-gate@example.invalid"
printf 'release\n' > "$repo/release.txt"
git -C "$repo" add release.txt
git -C "$repo" commit -qm "release source"
release_sha=$(git -C "$repo" rev-parse HEAD)
git -C "$repo" tag -a v1.2.3 -m "v1.2.3"

run_check() {
  (cd "$repo" && RELEASE_VERSION=v1.2.3 RELEASE_REMOTE_SHA="$release_sha" sh "$check")
}

expect_failure() {
  name=$1
  shift
  if "$@" >/dev/null 2>&1; then
    echo "release-source-test: expected failure: $name" >&2
    exit 1
  fi
}

run_check

printf 'dirty\n' >> "$repo/release.txt"
expect_failure "dirty checkout" run_check
git -C "$repo" restore release.txt

printf 'later\n' > "$repo/later.txt"
git -C "$repo" add later.txt
git -C "$repo" commit -qm "later source"
expect_failure "different HEAD" run_check

later_sha=$(git -C "$repo" rev-parse HEAD)
git -C "$repo" tag v1.2.4
expect_failure "lightweight tag" sh -c \
  'cd "$1" && RELEASE_VERSION=v1.2.4 RELEASE_REMOTE_SHA="$2" sh "$3"' \
  sh "$repo" "$later_sha" "$check"

echo "release-source-test: PASS"
