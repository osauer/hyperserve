# ADR-0015: Maintain one current release

**Status:** Accepted
**Date:** 2026-09-05
**Deciders:** HyperServe maintainer
**Supersedes:** The ongoing warning-placement requirement in
[ADR-0014](./0014-root-package-and-concern-subpackages.md)

## Decision

Maintain only the latest stable release. Development happens on `main`; bug
fixes and security updates ship in the next release. Older tags remain
available for reproducible builds, with no backports or parallel maintenance
branches.

Remove the repeated v2.1.0 warning from the README and subsequent compatible
release notes. Keep the migration instructions and historical release notes
for applications upgrading from older versions. A new breaking release still
explains its own changes before the upgrade command.

The current `/v2` module path, public API, and semantic-versioning policy stay
in place. This decision changes support scope and documentation presentation;
it does not rewrite tags or introduce another package migration.
