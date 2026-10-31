# Releases

The current module has no major-version suffix. New releases use `v1.Y.Z`;
`v1.7.0` follows the old unsuffixed `v1.6.1` history. Do not change existing v2
tags or depend on sorting all repository tags numerically to choose this module.
At an authorized release, update the scaffold target and release entry, then
use `make release RELEASE_VERSION=v1.Y.Z`. The gate verifies generated consumers
against that version and exact-commit hosted CI before creating a tag.

A local checkout can be exercised without publication using
`make release-smoke RELEASE_VERSION=v1.7.0`, `make test-race` and `make fuzz-smoke`.
This does not establish public module retrieval. After publication, install the
CLI in a fresh module cache and build its generated app without a replacement.

## Maintainer recovery

If the annotated tag push succeeds but GitHub Release creation fails, do not
move or replace the tag. First verify that the local and remote annotated tags
resolve to the same immutable commit and that the push CI run for that exact SHA
succeeded. Then recover only the publication step:

~~~sh
make release-publish RELEASE_VERSION=v1.Y.Z
~~~

---

Publication requires an explicit owner instruction. Use the release Make targets;
never bypass exact-commit CI or replace an existing tag.
