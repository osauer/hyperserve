#!/usr/bin/env bash
# Exercise local Torok revisions without editing consumers or fetching private code.
set -euo pipefail
if [ "$#" -lt 2 ]; then
  echo "usage: $0 /path/to/torok REF [REF ...]" >&2
  exit 2
fi
consumer_repo=$1
shift
producer_repo=$(cd "$(dirname "$0")/.." && pwd)
check_dir=$(mktemp -d "${TMPDIR:-/tmp}/hyperserve-consumers.XXXXXX")
trap 'rm -rf "$check_dir"' EXIT
export GOWORK=off
go -C "$producer_repo/tools" build -o "$check_dir/server" ./mcpconformance/testdata/stdio-server
for ref in "$@"; do
  revision=$(git -C "$consumer_repo" rev-parse --verify "${ref}^{commit}")
  archive_dir="$check_dir/$revision"
  if [ -d "$archive_dir" ]; then continue; fi
  mkdir -p "$archive_dir/source" "$archive_dir/probe"
  git -C "$consumer_repo" archive "$revision" | tar -x -C "$archive_dir/source"
  cp "$producer_repo/tools/mcpconformance/testdata/torok-client/main.go" "$archive_dir/probe/main.go"
  go -C "$archive_dir/probe" mod init hyperserve-consumer-check
  consumer_go=$(awk '$1 == "go" { print $2; exit }' "$archive_dir/source/go.mod")
  go -C "$archive_dir/probe" mod edit -go="$consumer_go" -require=github.com/osauer/torok@v0.0.0 -replace="github.com/osauer/torok=$archive_dir/source"
  go -C "$archive_dir/probe" mod tidy
  echo "Torok $revision"
  go -C "$archive_dir/probe" run . "$check_dir/server"
done
