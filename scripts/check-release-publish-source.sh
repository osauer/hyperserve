#!/bin/sh
# Verify that release notes will be rendered from the exact immutable tag tree.
set -eu

ver=${RELEASE_VERSION:-}
remote_sha=${RELEASE_REMOTE_SHA:-}

[ -n "$ver" ] || {
  echo "release-publish: RELEASE_VERSION is required" >&2
  exit 1
}
printf '%s\n' "$ver" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$' || {
  echo "release-publish: RELEASE_VERSION must look like vX.Y.Z (got $ver)" >&2
  exit 1
}
printf '%s\n' "$remote_sha" | grep -Eq '^[0-9a-f]{40}$' || {
  echo "release-publish: RELEASE_REMOTE_SHA must be a full commit SHA" >&2
  exit 1
}

if [ -n "$(git status --porcelain=v1 --untracked-files=all)" ]; then
  echo "release-publish: checkout must be clean before rendering release notes" >&2
  exit 1
fi

tag_type=$(git cat-file -t "refs/tags/$ver" 2>/dev/null || true)
if [ "$tag_type" != "tag" ]; then
  echo "release-publish: local $ver must be an annotated tag" >&2
  exit 1
fi

local_sha=$(git rev-parse "$ver^{}" 2>/dev/null || true)
if [ "$local_sha" != "$remote_sha" ]; then
  echo "release-publish: local and remote $ver do not resolve to the same commit" >&2
  exit 1
fi

head_sha=$(git rev-parse HEAD)
if [ "$head_sha" != "$remote_sha" ]; then
  echo "release-publish: HEAD $head_sha is not tagged release source $remote_sha" >&2
  echo "                 check out the immutable tag commit before recovery" >&2
  exit 1
fi
