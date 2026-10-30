# Release recovery

## Maintainer recovery

If the annotated tag push succeeds but GitHub Release creation fails, do not
move or replace the tag. First verify that the local and remote annotated tags
resolve to the same immutable commit and that the push CI run for that exact SHA
succeeded. Then recover only the publication step:

~~~sh
make release-publish RELEASE_VERSION=__VERSION__
~~~

---

Publication requires an explicit owner instruction. Use the release Make targets;
never bypass exact-commit CI or replace an existing tag.
