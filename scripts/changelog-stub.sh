#!/bin/sh
# changelog-stub.sh — prepend a CHANGELOG.md entry skeleton for
# RELEASE_VERSION above the current topmost entry.
set -eu

ver=${RELEASE_VERSION:-}
[ -n "$ver" ] || {
  echo "changelog-stub: RELEASE_VERSION env required (e.g. v1.2.3)" >&2
  exit 1
}

if ! printf '%s\n' "$ver" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$'; then
  echo "changelog-stub: RELEASE_VERSION must look like vX.Y.Z (got '$ver')" >&2
  exit 1
fi

cd "$(dirname "$0")/.."

plain=${ver#v}
if grep -q "^## \\[$plain\\] " CHANGELOG.md; then
  echo "changelog-stub: $ver entry already exists in CHANGELOG.md" >&2
  exit 1
fi

ts=$(TZ="Europe/Berlin" date +"%Y-%m-%d %H:%M %Z")

stub_file=$(mktemp -t hyperserve-changelog-stub.XXXXXX)
tmp=$(mktemp -t hyperserve-changelog-out.XXXXXX)
trap 'rm -f "$stub_file" "$tmp"' EXIT

cat >"$stub_file" <<STUB_EOF
## [$plain] - $ts

Release summary in one or two plain-English sentences.

### What's new

<!--
  Three bullets max. Plain English. This section is promoted into the
  GitHub Release header by make release-publish.
  Mark Go-library breaking changes with **Breaking (Go library):** and
  reserve those for a future /v2 module path.
-->

- TODO:

### Changed

<!--
  One user-visible change per bullet. Frame the consumer-visible effect:
  what Go importers, MCP clients, SSE clients, API callers, scaffold users,
  or operators notice. Keep internal review IDs out of release prose.
-->

- TODO:

### Fixed

<!--
  One user-visible bug per bullet. "X no longer happens" or "Y now works
  as documented" is usually the right shape.
-->

- TODO:

### Documentation

<!-- Omit this section when there are no docs/example/scaffold changes. -->

- TODO:

### Verification

- \`go test ./...\`
- \`(cd examples/auth && go test ./...)\`
- \`make check\`
- scaffold smoke test
STUB_EOF

awk -v stub_file="$stub_file" '
  !inserted && /^## \[[0-9]/ {
    while ((getline line < stub_file) > 0) print line
    print ""
    close(stub_file)
    inserted = 1
  }
  { print }
' CHANGELOG.md > "$tmp"

if ! grep -q "^## \\[$plain\\] " "$tmp"; then
  echo "changelog-stub: could not find insertion point in CHANGELOG.md" >&2
  exit 1
fi

mv "$tmp" CHANGELOG.md

echo "changelog-stub: prepended skeleton for $ver"
echo "                edit CHANGELOG.md before running make changelog-lint"
