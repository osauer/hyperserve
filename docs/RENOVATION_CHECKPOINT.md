# V3 renovation checkpoint

2026-10-30 CET. Working branch: `codex/v3-renovation`, based on local main
`ba983b3131d428520d0ebe1861aad83e4d95f57e`. No publication authorized.

Owner decisions: v3 cleanup; consolidate the two broad examples; one document
catalogue shared by HTML/JSON/MCP/live updates; retain proprietary routed SSE in
an isolated compatibility path; near-zero dependencies and vertical integration.

Completed locally: module-path migration, initial safety regressions and fixes,
scaffold config packaging, current/history documentation distinction, MCP guide
split, workflow cleanup/pinning, and a prepared catalogue with explicit policy
input and focused tests. The old examples remain until replacement approval.

Consumer evidence: committed Desk revision
`1e3b264832ad56c47d608520a1c1b64ef0786b05` was archived to a temporary directory.
Only imports and the module selection changed. Its `TestLiveStream` tests pass
against this working tree, including HTTP/1 and HTTP/2. Desk's shared checkout,
local changes and running application were not changed. This is transport-test
evidence, not a live migration or proof of all Desk behavior.

Pending owner answers: approve the proposed catalogue demonstration policy
(10 MiB/file, 50 MiB total, 100 documents, in-memory), and replacement of the
old hardcoded feature cards, mock CPU/Memory chart, random-error button and
About page. No such policy file has been added as a default.

Next: visually inspect catalogue with a disposable synthetic policy; complete
approved consolidation; run normal/race/fuzz gates and disposable consumer
checks on the final tree; inspect the staged set and commit. Final publication,
GitHub branch protection changes and real Desk migration remain separate.

Logs are in `/private/tmp/hyperserve-v3-*.log` and
`/private/tmp/hyperserve-desk-v3.log`. No changelog entry is written before release.
