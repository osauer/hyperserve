#!/bin/sh
# check-changelog-entry.sh — assert the topmost CHANGELOG.md entry matches
# RELEASE_VERSION and is useful enough to become GitHub release notes.
set -eu

ver=${RELEASE_VERSION:-}
[ -n "$ver" ] || {
  echo "check-changelog-entry: RELEASE_VERSION env required" >&2
  exit 1
}

if ! printf '%s\n' "$ver" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$'; then
  echo "check-changelog-entry: RELEASE_VERSION must look like vX.Y.Z (got '$ver')" >&2
  exit 1
fi

cd "$(dirname "$0")/.."

plain=${ver#v}

# 1) Topmost release entry must match RELEASE_VERSION with a full local
# timestamp. HyperServe keeps Keep-a-Changelog link-style headings, so the
# tag is v1.2.3 while the changelog heading is ## [1.2.3] - ...
head=$(grep -m1 '^## \[[0-9]' CHANGELOG.md || true)
case "$head" in
  "## [$plain] - "*) ;;
  "")
    echo "check-changelog-entry: CHANGELOG.md has no '## [X.Y.Z]' entries" >&2
    exit 1
    ;;
  *)
    echo "check-changelog-entry: topmost CHANGELOG entry is '$head'" >&2
    echo "                       expected '## [$plain] - <YYYY-MM-DD HH:MM TZ>'" >&2
    exit 1
    ;;
esac

if ! printf '%s\n' "$head" | grep -Eq '^## \[[0-9]+\.[0-9]+\.[0-9]+\] - [0-9]{4}-[0-9]{2}-[0-9]{2} [0-9]{2}:[0-9]{2} [A-Z]{3,4}$'; then
  echo "check-changelog-entry: heading timestamp must include date, time, and timezone" >&2
  echo "                       got '$head'" >&2
  exit 1
fi

# 2) Matching entry must have a non-empty `### What's new` section. This is
# mechanically promoted into the top of the GitHub Release body.
if ! awk -v plain="$plain" '
  /^## \[[0-9]/ { if (in_ver) exit; in_ver = ($0 ~ "^## \\["plain"\\] "); next }
  in_ver && /^### What.s new$/ { in_new = 1; next }
  in_ver && in_new && /^###/ { exit }
  in_new && /^- / { found = 1 }
  END { exit !found }
' CHANGELOG.md; then
  echo "check-changelog-entry: $ver has no bullet in '### What's new'" >&2
  echo "                       (must follow the version heading; describes user-visible change)" >&2
  exit 1
fi

# 3) Matching entry must have at least one Keep-a-Changelog subsection.
has_kac=$(awk -v plain="$plain" '
  /^## \[[0-9]/ { if (in_ver) exit; in_ver = ($0 ~ "^## \\["plain"\\] "); next }
  in_ver && /^### (Added|Changed|Deprecated|Removed|Fixed|Security)$/ { print "yes"; exit }
' CHANGELOG.md)
[ "$has_kac" = yes ] || {
  echo "check-changelog-entry: $ver has no ### Added/Changed/Deprecated/Removed/Fixed/Security section" >&2
  exit 1
}

# 4) Matching entry must list verification commands. Release notes are not a
# wish; they should say what was actually exercised.
has_verification=$(awk -v plain="$plain" '
  /^## \[[0-9]/ { if (in_ver) exit; in_ver = ($0 ~ "^## \\["plain"\\] "); next }
  in_ver && /^### Verification$/ { in_verification = 1; next }
  in_ver && in_verification && /^###/ { exit }
  in_verification && /^- / { print "yes"; exit }
' CHANGELOG.md)
[ "$has_verification" = yes ] || {
  echo "check-changelog-entry: $ver has no non-empty ### Verification section" >&2
  exit 1
}

# 5) Placeholder text means the skeleton was not actually turned into a
# release note.
placeholder=$(awk -v plain="$plain" '
  /^## \[[0-9]/ { if (in_ver) exit; in_ver = ($0 ~ "^## \\["plain"\\] "); next }
  in_ver && /(TODO|TBD|__HIGHLIGHTS__|__VERSION__)/ { print "L"NR": "$0; exit }
' CHANGELOG.md)
if [ -n "$placeholder" ]; then
  echo "check-changelog-entry: $ver still contains placeholder text" >&2
  echo "                       $placeholder" >&2
  exit 1
fi

# 6) Optional Engineering notes must stay short and not become a second
# changelog inside the changelog.
eng_lines=$(awk -v plain="$plain" '
  /^## \[[0-9]/ { if (in_ver) exit; in_ver = ($0 ~ "^## \\["plain"\\] "); next }
  in_ver && /^### Engineering notes$/ { in_eng = 1; next }
  in_ver && in_eng && /^###/ { exit }
  in_eng { n++ }
  END { print n+0 }
' CHANGELOG.md)
if [ "$eng_lines" -gt 15 ]; then
  echo "check-changelog-entry: $ver '### Engineering notes' has $eng_lines lines (limit 15)" >&2
  echo "                       trim it or move user-visible facts into Changed/Fixed." >&2
  exit 1
fi

# 7) Internal review/finding IDs make public notes look like an exported
# scratchpad. Keep those handles in commits/issues, not release prose.
finding=$(awk -v plain="$plain" '
  /^## \[[0-9]/ { if (in_ver) exit; in_ver = ($0 ~ "^## \\["plain"\\] "); next }
  in_ver && /^### (Added|Changed|Deprecated|Removed|Fixed|Security)$/ { in_kac = 1; next }
  in_ver && in_kac && /^###/ { in_kac = 0 }
  in_ver && in_kac && /(\*\*F-[0-9]+|F#[0-9]+|finding-[0-9]+)/ { print "L"NR": "$0; exit }
' CHANGELOG.md)
if [ -n "$finding" ]; then
  echo "check-changelog-entry: $ver KaC bullet references an internal finding ID" >&2
  echo "                       $finding" >&2
  echo "                       Frame the bullet for users; keep review handles out of CHANGELOG.md." >&2
  exit 1
fi

echo "check-changelog-entry: $ver OK"
