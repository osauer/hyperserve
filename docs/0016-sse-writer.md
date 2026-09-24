# ADR-0016: Standalone SSE writer

**Status:** Accepted
**Date:** 2026-09-24
**Extends:** [ADR-0014](./0014-root-package-and-concern-subpackages.md)

## Context

HTTP consumers repeat frame encoding, response-controller deadlines, and
flushing. Their event replay and slow-reader delivery policies differ, so those
policies cannot usefully be merged into a server-owned stream abstraction.

## Decision

Add `github.com/osauer/hyperserve/v2/sse`, depending only on the standard
library. `NewWriter` takes an ordinary response writer and a positive write
timeout. `Send`, `SendJSON`, and `Comment` validate, write, and flush one frame.
Transport failures stop further writes; encoding failures commit no bytes.
Deadlines are cleared after each frame so they do not expire an idle HTTP/2
stream. Missing deadline or flush support is an error, including through
middleware that does not expose its underlying response.

The application owns the handler loop, heartbeat timing, authorization, IDs,
replay, queues, and job lifetime. The writer starts no goroutines and retains
no event history. Existing root `SSEMessage` formatting remains compatible;
the new writer explicitly rejects invalid input and returns encoding errors.

## Verification

Focused tests cover framing, invalid input, partial writes, flush and deadline
failures, middleware unwrapping, stalled readers, and idle HTTP/2 streams.
Consumer acceptance requires disposable migrations of a durable event stream
and a latest-snapshot stream, preserving their cancellation and replay rules.
