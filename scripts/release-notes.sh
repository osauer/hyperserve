#!/bin/sh
# release-notes.sh — render GitHub Release notes from the canonical
# CHANGELOG.md entry for RELEASE_VERSION.
set -eu

ver=${RELEASE_VERSION:-}
[ -n "$ver" ] || {
  echo "release-notes: RELEASE_VERSION env required (e.g. v1.2.3)" >&2
  exit 1
}

if ! printf '%s\n' "$ver" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$'; then
  echo "release-notes: RELEASE_VERSION must look like vX.Y.Z (got '$ver')" >&2
  exit 1
fi

cd "$(dirname "$0")/.."

plain=${ver#v}
highlights=$(mktemp -t hyperserve-release-highlights.XXXXXX)
trimmed_highlights=$(mktemp -t hyperserve-release-highlights-trimmed.XXXXXX)
body=$(mktemp -t hyperserve-release-body.XXXXXX)
trap 'rm -f "$highlights" "$trimmed_highlights" "$body"' EXIT

awk -v plain="$plain" '
  /^## \[[0-9]/ { if (in_ver) exit; in_ver = ($0 ~ "^## \\["plain"\\] "); next }
  in_ver && /^### What.s new$/ { in_new = 1; next }
  in_ver && in_new && /^###/ { exit }
  in_new { print }
' CHANGELOG.md > "$highlights"

awk '
  NF { seen = 1 }
  seen { lines[++n] = $0 }
  END {
    while (n > 0 && lines[n] == "") n--
    for (i = 1; i <= n; i++) print lines[i]
  }
' "$highlights" > "$trimmed_highlights"

awk -v plain="$plain" '
  /^## \[[0-9]/ {
    if (in_section) exit
    in_section = ($0 ~ "^## \\["plain"\\] ")
    skip = 0
    if (in_section) next
  }
  in_section && /^### What.s new$/ { skip = 1; next }
  in_section && skip && /^### / { skip = 0 }
  in_section && !skip { print }
' CHANGELOG.md > "$body"

awk -v ver="$ver" -v hf="$trimmed_highlights" '
  {
    gsub(/__VERSION__/, ver)
  }
  /__HIGHLIGHTS__/ {
    while ((getline line < hf) > 0) print line
    close(hf)
    next
  }
  { print }
' .github/release-notes-template.md

cat "$body"
