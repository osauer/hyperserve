# Roadmap

This document records directions, not release promises. Issues and accepted
ADRs remain the authority for scheduled work.

## Current work

Maintenance focuses on tested HTTP/MCP composition, non-destructive file
examples, correct binding and static serving, one reference application, and
current documentation. See [ADR-0018](0018-single-owner-module.md).

Protocol conformance, fuzzing, public-module consumer checks and repeatable
performance measurements are continuing maintenance obligations, not features
promised for a future release. New APIs require a concrete application need.

## Deliberately out of scope

- ORM or database abstraction;
- browser-session management;
- identity-provider configuration inside the root package;
- application roles and resource authorization;
- automatic trust of `Forwarded` or `X-Forwarded-For`;
- a process-wide default limiter policy;
- a custom router solely for benchmark position;
- retaining duplicate public APIs for compatibility.

## How work enters the roadmap

A proposal should identify the user problem, authority boundary, public API
impact, expected dependency cost, and verification witness. Changes to package
direction or lifecycle require an ADR. Security-sensitive features need
fail-closed behavior and tests before they become examples or scaffold
defaults.
